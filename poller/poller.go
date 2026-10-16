// Package poller provides an in-memory background polling worker with staleness detection.
package poller

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/terastal-hq/proxylib/lifecycle"
)

// PollInfo describes background polling attempts, successes, and errors.
type PollInfo struct {
	IntervalSeconds int64      `json:"interval_seconds"`
	LastAttemptAt   *time.Time `json:"last_attempt_at"`
	LastSuccessAt   *time.Time `json:"last_success_at"`
	LastError       string     `json:"last_error,omitempty"`
}

// Poller runs a periodic background fetcher, maintains the latest observation in memory,
// and provides staleness calculations without disk I/O.
type Poller[T any] struct {
	mu        sync.RWMutex
	interval  time.Duration
	fetch     func(context.Context) (T, error)
	latest    T
	hasLatest bool
	polling   PollInfo

	trigger chan struct{}

	started    bool
	closed     bool
	cancel     context.CancelFunc
	workerDone chan struct{}
	closer     lifecycle.Closer
}

// New constructs a Poller for the given interval and fetch function.
func New[T any](interval time.Duration, fetch func(context.Context) (T, error)) (*Poller[T], error) {
	if interval <= 0 {
		return nil, errors.New("poll interval must be positive")
	}
	if fetch == nil {
		return nil, errors.New("fetch function is nil")
	}
	return &Poller[T]{
		interval: interval,
		fetch:    fetch,
		trigger:  make(chan struct{}, 1),
		polling:  PollInfo{IntervalSeconds: int64(interval / time.Second)},
	}, nil
}

// Start begins background polling. Repeated calls or nil context return an error.
func (p *Poller[T]) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("polling context is nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("poller is closed")
	}
	if p.started {
		return errors.New("poller is already started")
	}
	owned, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.started = true
	p.workerDone = make(chan struct{})

	go p.run(owned)
	return nil
}

func (p *Poller[T]) run(ctx context.Context) {
	defer close(p.workerDone)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		case <-p.trigger:
			p.poll(ctx)
		}
	}
}

// Poll attempts one fetch and updates the in-memory state.
func (p *Poller[T]) Poll(ctx context.Context) {
	p.poll(ctx)
}

func (p *Poller[T]) poll(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	attempt := time.Now().UTC()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.polling.IntervalSeconds = int64(p.interval / time.Second)
	p.polling.LastAttemptAt = &attempt
	p.mu.Unlock()

	sample, err := p.fetch(ctx)

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		p.polling.LastError = err.Error()
		return
	}
	p.latest = sample
	p.hasLatest = true
	now := time.Now().UTC()
	p.polling.LastSuccessAt = &now
	p.polling.LastError = ""
}

// Latest returns the most recent in-memory observation, an availability flag, and independent polling metadata.
func (p *Poller[T]) Latest() (T, bool, PollInfo) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	info := p.pollingInfoLocked()
	if !p.hasLatest {
		var zero T
		return zero, false, info
	}
	return p.latest, true, info
}

// PollingInfo returns independent polling metadata.
func (p *Poller[T]) PollingInfo() PollInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pollingInfoLocked()
}

func (p *Poller[T]) pollingInfoLocked() PollInfo {
	info := p.polling
	info.LastAttemptAt = CloneTime(info.LastAttemptAt)
	info.LastSuccessAt = CloneTime(info.LastSuccessAt)
	return info
}

// TriggerPoll signals an immediate polling attempt if running.
func (p *Poller[T]) TriggerPoll() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.started || p.closed {
		return
	}
	select {
	case p.trigger <- struct{}{}:
	default:
	}
}

// Stale reports whether the observation is older than two polling intervals or has an error.
func (p *Poller[T]) Stale(observedAt time.Time, info PollInfo) bool {
	return IsStale(observedAt, p.interval, info.LastError, time.Now())
}

// IsStale reports whether observedAt is older than two polling intervals or has an error.
func IsStale(observedAt time.Time, interval time.Duration, lastError string, now time.Time) bool {
	return lastError != "" || now.After(observedAt.Add(interval).Add(interval))
}

// Close gracefully stops the worker and waits for it to complete.
func (p *Poller[T]) Close() error {
	return p.closer.Close(func() error {
		p.mu.Lock()
		p.closed = true
		if p.cancel != nil {
			p.cancel()
		}
		p.mu.Unlock()

		if p.workerDone != nil {
			<-p.workerDone
		}
		return nil
	})
}

// CloneTime safely duplicates a timestamp pointer.
func CloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}
