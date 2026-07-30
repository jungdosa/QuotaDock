//go:build !windows

package update

import "errors"

type ProcessLauncher struct{}

func (ProcessLauncher) Launch(string, string) error {
	return errors.New("automatic update installation is only supported on Windows")
}
