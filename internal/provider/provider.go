// Package provider contains shared provider orchestration and probe helpers.
package provider

import (
	"context"
	"errors"
	"github.com/jungdosa/QuotaDock/internal/model"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrNotInstalled = errors.New("provider CLI not installed")
	ErrNotLoggedIn  = errors.New("provider is not logged in")
)

func VersionAtLeast(version, minimum string) bool {
	parse := func(value string) []int {
		value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
		fields := strings.SplitN(value, "-", 2)
		parts := strings.Split(fields[0], ".")
		numbers := make([]int, 3)
		for i := 0; i < len(parts) && i < 3; i++ {
			numbers[i], _ = strconv.Atoi(parts[i])
		}
		return numbers
	}
	actual, wanted := parse(version), parse(minimum)
	for i := 0; i < 3; i++ {
		if actual[i] > wanted[i] {
			return true
		}
		if actual[i] < wanted[i] {
			return false
		}
	}
	return true
}

type Outcome struct {
	Snapshot model.UsageSnapshot
	Err      error
}
type Coordinator struct {
	Providers map[model.ProviderID]model.Provider
}

func (c Coordinator) RefreshAll(ctx context.Context) map[model.ProviderID]Outcome {
	output := make(map[model.ProviderID]Outcome, len(c.Providers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for id, implementation := range c.Providers {
		id, implementation := id, implementation
		wg.Add(1)
		go func() {
			defer wg.Done()
			snapshot, err := implementation.Refresh(ctx)
			mu.Lock()
			output[id] = Outcome{Snapshot: snapshot, Err: err}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return output
}
func (c Coordinator) Close() error {
	var combined error
	for _, implementation := range c.Providers {
		combined = errors.Join(combined, implementation.Close())
	}
	return combined
}
