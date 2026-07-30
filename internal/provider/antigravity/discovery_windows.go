//go:build windows

package antigravity

import (
	"errors"

	"path/filepath"
	"sort"
	"unsafe"

	"golang.org/x/sys/windows"
)

const processCommandLineInformation = 60

var (
	ntQueryInformationProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationProcess")
	getExtendedTCPTable       = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")
)

type tcpRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPID  uint32
}

func discoverLocalEndpoints() ([]endpointCandidate, error) {
	listeners, err := loopbackListenersByPID()
	if err != nil {
		return nil, err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, ErrLocalRequest
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, ErrLocalRequest
	}
	var candidates []endpointCandidate
	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		ports := listeners[entry.ProcessID]
		if IsAllowedExecutable(name) && len(ports) > 0 {
			if path, commandLine, ok := processIdentity(entry.ProcessID); ok && validateProcessIdentity(path, commandLine) {
				for _, token := range csrfTokens(commandLine) {
					for _, port := range ports {
						candidates = append(candidates, endpointCandidate{pid: entry.ProcessID, port: port, token: token, executable: filepath.Base(path), verified: true})
					}
				}
			}
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, ErrLocalRequest
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].pid != candidates[j].pid {
			return candidates[i].pid < candidates[j].pid
		}
		return candidates[i].port < candidates[j].port
	})
	return candidates, nil
}

func processIdentity(pid uint32) (string, string, bool) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return "", "", false
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return "", "", false
	}
	path := windows.UTF16ToString(buffer[:size])
	commandLine, ok := queryCommandLine(handle)
	return path, commandLine, ok
}

func queryCommandLine(handle windows.Handle) (string, bool) {
	var size uint32
	_, _, _ = ntQueryInformationProcess.Call(uintptr(handle), processCommandLineInformation, 0, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 || size > 1<<20 {
		return "", false
	}
	buffer := make([]byte, size)
	status, _, _ := ntQueryInformationProcess.Call(
		uintptr(handle),
		processCommandLineInformation,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(size),
		uintptr(unsafe.Pointer(&size)),
	)
	if int32(status) < 0 {
		return "", false
	}
	value := (*windows.NTUnicodeString)(unsafe.Pointer(&buffer[0]))
	if value.Buffer == nil || value.Length == 0 || int(value.Length) > len(buffer) {
		return "", false
	}
	units := unsafe.Slice(value.Buffer, int(value.Length)/2)
	return windows.UTF16ToString(units), true
}

func loopbackListenersByPID() (map[uint32][]uint16, error) {
	var size uint32
	result, _, _ := getExtendedTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 3, 0)
	if result != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) || size < 4 || size > 16<<20 {
		return nil, ErrLocalRequest
	}
	buffer := make([]byte, size)
	result, _, _ = getExtendedTCPTable.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 3, 0)
	if result != 0 {
		return nil, ErrLocalRequest
	}
	count := *(*uint32)(unsafe.Pointer(&buffer[0]))
	rowSize := int(unsafe.Sizeof(tcpRowOwnerPID{}))
	if int(count) > (len(buffer)-4)/rowSize {
		return nil, ErrLocalRequest
	}
	const loopback uint32 = 0x0100007f
	output := make(map[uint32][]uint16)
	for index := 0; index < int(count); index++ {
		offset := 4 + index*rowSize
		row := (*tcpRowOwnerPID)(unsafe.Pointer(&buffer[offset]))
		if row.LocalAddr != loopback {
			continue
		}
		portWord := uint16(row.LocalPort)
		port := portWord<<8 | portWord>>8
		if port != 0 {
			output[row.OwningPID] = append(output[row.OwningPID], port)
		}
	}
	return output, nil
}
