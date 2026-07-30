package windows

import "sync"

type Lifecycle struct {
	mu      sync.Mutex
	exiting bool
	Hide    func()
	Quit    func()
}

func (l *Lifecycle) CloseRequested() {
	l.mu.Lock()
	exiting := l.exiting
	l.mu.Unlock()
	if !exiting && l.Hide != nil {
		l.Hide()
	}
}
func (l *Lifecycle) ExitRequested() {
	l.mu.Lock()
	if l.exiting {
		l.mu.Unlock()
		return
	}
	l.exiting = true
	l.mu.Unlock()
	if l.Quit != nil {
		l.Quit()
	}
}
func (l *Lifecycle) Exiting() bool { l.mu.Lock(); defer l.mu.Unlock(); return l.exiting }
