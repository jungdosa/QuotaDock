//go:build windows

package process

import (
	"context"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type platformTreeController struct {
	mu   sync.Mutex
	jobs map[int]windows.Handle
}

func (*platformTreeController) Prepare(*exec.Cmd) error { return nil }
func (c *platformTreeController) Attach(cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return err
	}
	err = windows.AssignProcessToJobObject(job, process)
	windows.CloseHandle(process)
	if err != nil {
		windows.CloseHandle(job)
		return err
	}
	c.mu.Lock()
	if c.jobs == nil {
		c.jobs = make(map[int]windows.Handle)
	}
	c.jobs[cmd.Process.Pid] = job
	c.mu.Unlock()
	return nil
}
func (c *platformTreeController) Terminate(cmd *exec.Cmd) error {
	if cmd != nil && cmd.Process != nil {
		c.mu.Lock()
		job := c.jobs[cmd.Process.Pid]
		delete(c.jobs, cmd.Process.Pid)
		c.mu.Unlock()
		if job != 0 {
			return windows.CloseHandle(job)
		}
	}
	pid := pidArgument(cmd)
	if pid == "" {
		return nil
	}
	killer := exec.CommandContext(context.Background(), "taskkill.exe", "/T", "/F", "/PID", pid)
	configureCommand(killer)
	_ = killer.Run()
	return nil
}
