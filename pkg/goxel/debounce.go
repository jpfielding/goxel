package goxel

import (
	"sync"
	"time"
)

type debouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	duration time.Duration
}

func newDebouncer(d time.Duration) *debouncer {
	return &debouncer{duration: d}
}

func (d *debouncer) call(fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.duration, fn)
}
