package poller

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type testSample struct {
	Value      string
	ObservedAt time.Time
}

func TestPollerLifecycleAndStaleness(t *testing.T) {
	var count atomic.Int32
	var fail atomic.Bool

	fetch := func(ctx context.Context) (testSample, error) {
		if fail.Load() {
			return testSample{}, errors.New("upstream failed")
		}
		_ = count.Add(1)
		return testSample{
			Value:      "sample",
			ObservedAt: time.Now().UTC(),
		}, nil
	}

	p, err := New(50*time.Millisecond, fetch, func(s testSample) time.Time {
		return s.ObservedAt
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Before start
	_, exists, info := p.Latest()
	if exists || info.LastAttemptAt != nil {
		t.Fatalf("unexpected state before start: exists=%v, info=%+v", exists, info)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Repeated start fails
	if err := p.Start(ctx); err == nil {
		t.Fatal("expected error on duplicate Start")
	}

	// Wait for first poll to complete
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		sample, exists, info := p.Latest()
		if exists && sample.Value == "sample" && info.LastError == "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	sample, exists, info := p.Latest()
	if !exists || sample.Value != "sample" {
		t.Fatalf("expected sample, got exists=%v sample=%+v", exists, sample)
	}

	// Fresh sample is not stale
	if p.Stale(sample.ObservedAt, info) {
		t.Fatal("fresh observation marked stale")
	}

	// Simulate error on next poll
	fail.Store(true)
	p.TriggerPoll()

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		_, _, info := p.Latest()
		if info.LastError != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	_, _, errInfo := p.Latest()
	if errInfo.LastError != "upstream failed" {
		t.Fatalf("expected error info, got %+v", errInfo)
	}
	if !p.Stale(sample.ObservedAt, errInfo) {
		t.Fatal("errored poller was not marked stale")
	}

	// Close
	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("idempotent Close failed: %v", err)
	}
}

func TestIsStaleBoundaries(t *testing.T) {
	now := time.Now()
	interval := time.Minute

	// Within 2 intervals
	if IsStale(now.Add(-90*time.Second), interval, "", now) {
		t.Fatal("observation within 2 intervals marked stale")
	}

	// Past 2 intervals
	if !IsStale(now.Add(-130*time.Second), interval, "", now) {
		t.Fatal("observation past 2 intervals was not marked stale")
	}

	// Error makes it stale immediately
	if !IsStale(now, interval, "some error", now) {
		t.Fatal("error state was not marked stale")
	}
}

func TestPollerManualPollSerializationAndClose(t *testing.T) {
	fetchEntered := make(chan int, 2)
	release1 := make(chan struct{})
	fetch2Canceled := make(chan struct{})
	var fetch2Finished atomic.Bool

	var fetchIndex atomic.Int32
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	fetch := func(ctx context.Context) (testSample, error) {
		idx := int(fetchIndex.Add(1))
		c := concurrent.Add(1)
		for {
			cur := maxConcurrent.Load()
			if c <= cur || maxConcurrent.CompareAndSwap(cur, c) {
				break
			}
		}
		defer concurrent.Add(-1)

		fetchEntered <- idx

		if idx == 1 {
			select {
			case <-ctx.Done():
				return testSample{}, ctx.Err()
			case <-release1:
				return testSample{Value: "fetch1", ObservedAt: time.Now().UTC()}, nil
			}
		}

		// idx == 2: wait for cancel via Close
		select {
		case <-ctx.Done():
			close(fetch2Canceled)
			time.Sleep(50 * time.Millisecond)
			fetch2Finished.Store(true)
			return testSample{}, ctx.Err()
		}
	}

	p, err := New(time.Hour, fetch)
	if err != nil {
		t.Fatal(err)
	}

	done1 := make(chan struct{})
	done2 := make(chan struct{})

	// Start first manual poll
	go func() { p.Poll(context.Background()); close(done1) }()
	first := <-fetchEntered
	if first != 1 {
		t.Fatalf("expected fetch 1, got %d", first)
	}

	// Start second manual poll while first is still running
	go func() { p.Poll(context.Background()); close(done2) }()

	// Verify that second fetch does not enter because first holds the gate
	select {
	case unexpected := <-fetchEntered:
		t.Fatalf("fetch %d entered concurrently before fetch 1 was released", unexpected)
	case <-time.After(50 * time.Millisecond):
		// Expected: fetch 2 is blocked on fetchGate
	}

	if max := maxConcurrent.Load(); max != 1 {
		t.Fatalf("concurrent fetches observed: %d, want 1", max)
	}

	// Release first fetch; second fetch can now acquire the gate
	close(release1)
	<-done1

	// Wait until second fetch has entered
	second := <-fetchEntered
	if second != 2 {
		t.Fatalf("expected fetch 2, got %d", second)
	}

	// Close must cancel the in-flight second fetch and wait for it to return
	closed := make(chan struct{})
	go func() {
		_ = p.Close()
		close(closed)
	}()

	select {
	case <-fetch2Canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch 2 was not canceled by Close")
	}

	select {
	case <-closed:
		if !fetch2Finished.Load() {
			t.Fatal("Close returned before fetch 2 finished its execution")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after fetch canceled")
	}

	<-done2
}
