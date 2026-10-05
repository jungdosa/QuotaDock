//go:build !windows

package antigravity

import "testing"

func TestCLIDisabledOutsideWindows(t *testing.T) {
	if defaultCLIClient() != nil || NewLocalClient().cli != nil {
		t.Fatal("CLI enabled outside Windows")
	}
}
