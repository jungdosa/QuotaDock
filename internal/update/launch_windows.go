//go:build windows

package update

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

const updateHelperScript = `$target = Get-Process -Id ([int]$env:QUOTADOCK_UPDATE_PID) -ErrorAction SilentlyContinue
if ($null -ne $target) { $target.WaitForExit() }
$installer = Start-Process -FilePath $env:QUOTADOCK_UPDATE_INSTALLER -ArgumentList @('/SP-','/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART') -Wait -PassThru
Remove-Item -LiteralPath $env:QUOTADOCK_UPDATE_INSTALLER -Force -ErrorAction SilentlyContinue
if ($installer.ExitCode -eq 0) { Start-Process -FilePath $env:QUOTADOCK_UPDATE_EXECUTABLE }`

type ProcessLauncher struct{}

func (ProcessLauncher) Launch(installerPath, executablePath string) error {
	if !filepath.IsAbs(installerPath) || !filepath.IsAbs(executablePath) || !isSafeSetupName(filepath.Base(installerPath)) {
		return errors.New("update helper paths are invalid")
	}
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		return errors.New("Windows system root is unavailable")
	}
	powershell := filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	command := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", updateHelperScript)
	command.Env = append(os.Environ(),
		"QUOTADOCK_UPDATE_PID="+strconv.Itoa(os.Getpid()),
		"QUOTADOCK_UPDATE_INSTALLER="+installerPath,
		"QUOTADOCK_UPDATE_EXECUTABLE="+executablePath,
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
