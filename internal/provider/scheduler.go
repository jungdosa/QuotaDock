package provider

import (
	"context"
	"sync"
	"time"

	"github.com/jungdosa/QuotaDock/internal/diagnostics"
)

// Scheduler owns at most one refresh timer. Start is idempotent until Stop.
type Scheduler struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func (s *Scheduler) Start(parent context.Context, interval time.Duration, refresh func(context.Context)) bool {
	if interval <= 0 || refresh == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return false
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.done = make(chan struct{})
	done := s.done
	diagnostics.Go("refresh_scheduler", func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				refresh(ctx)
			case <-ctx.Done():
				return
			}
		}
	})
	return true
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done = nil, nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}
