//go:build windows

package windows

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSingleInstanceMutexBlocksSecondOwnerAndReleases(t *testing.T) {
	name := fmt.Sprintf("QuotaDock_Test_%d_%d", os.Getpid(), time.Now().UnixNano())
	first, exists, err := acquireSingleInstance(name)
	if err != nil || exists || first == nil {
		t.Fatalf("first acquire guard=%v exists=%v err=%v", first, exists, err)
	}
	defer first.Close()

	second, exists, err := acquireSingleInstance(name)
	if err != nil || !exists || second != nil {
		t.Fatalf("second acquire guard=%v exists=%v err=%v", second, exists, err)
	}

	first.Close()
	third, exists, err := acquireSingleInstance(name)
	if err != nil || exists || third == nil {
		t.Fatalf("reacquire guard=%v exists=%v err=%v", third, exists, err)
	}
	third.Close()
}
