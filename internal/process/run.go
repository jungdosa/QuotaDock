package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/jungdosa/QuotaDock/internal/security"
)

// RunOutput executes a fixed command without a shell and returns bounded stdout.
func (r Runner) RunOutput(ctx context.Context, spec CommandSpec) ([]byte, error) {
	if err := ValidateCommand(spec); err != nil {
		return nil, err
	}
	runCtx := ctx
	cancel := func() {}
	if r.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, r.Timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, spec.Name, spec.Args...)
	configureCommand(cmd)
	_, outputLimit, stderrLimit := r.limits()
	stdout := &limitedBuffer{limit: outputLimit}
	stderr := &limitedBuffer{limit: stderrLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	tree := r.Tree
	if tree == nil {
		tree = &platformTreeController{}
	}
	if err := tree.Prepare(cmd); err != nil {
		return nil, fmt.Errorf("prepare process tree: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}
	if err := tree.Attach(cmd); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("attach process tree: %w", err)
	}
	waitErr := cmd.Wait()
	_ = tree.Terminate(cmd)
	if stderr.Len() > 0 && r.Log != nil {
		r.Log(security.MaskSecrets(stderr.String()))
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return nil, ErrTimeout
	}
	if stdout.exceeded {
		return nil, ErrOutputLimit
	}
	if waitErr != nil {
		return nil, ErrProcessExited
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}
