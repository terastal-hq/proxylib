package lifecycle

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCloserRunsOnlyOnce(t *testing.T) {
	var closer Closer
	var calls atomic.Int32
	expectedErr := errors.New("cleanup failed")

	cleanup := func() error {
		calls.Add(1)
		return expectedErr
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := closer.Close(cleanup)
			if !errors.Is(err, expectedErr) {
				t.Errorf("got error %v, want %v", err, expectedErr)
			}
		}()
	}
	wg.Wait()

	if calls.Load() != 1 {
		t.Fatalf("cleanup called %d times, want 1", calls.Load())
	}

	// Repeated call after wait
	if err := closer.Close(cleanup); !errors.Is(err, expectedErr) {
		t.Fatalf("repeated call error = %v, want %v", err, expectedErr)
	}
	if calls.Load() != 1 {
		t.Fatalf("cleanup called again, count = %d", calls.Load())
	}
}

func TestCloserNilCleanup(t *testing.T) {
	var closer Closer
	if err := closer.Close(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
