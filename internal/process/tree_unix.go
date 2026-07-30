//go:build !windows

package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type platformTreeController struct{}

func (platformTreeController) Prepare(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}
func (platformTreeController) Attach(*exec.Cmd) error { return nil }
func (platformTreeController) Terminate(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
