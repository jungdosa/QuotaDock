//go:build !windows

package process

import "os/exec"

func configureCommand(*exec.Cmd) {}
