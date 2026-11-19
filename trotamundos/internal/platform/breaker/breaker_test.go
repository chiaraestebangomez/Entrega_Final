package breaker

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestHalfOpenAllowsSingleProbe(t *testing.T) {
	b := New(1, time.Minute)
	now := time.Unix(1000, 0)
	b.SetNow(func() time.Time { return now })

	if err := b.Allow(); err != nil {
		t.Fatalf("primer Allow: %v", err)
	}
	b.Failure()
	if got := b.State(); got != Open {
		t.Fatalf("esperaba Open, obtuve %v", got)
	}

	now = now.Add(2 * time.Minute)
	if got := b.State(); got != HalfOpen {
		t.Fatalf("esperaba HalfOpen, obtuve %v", got)
	}
	if err := b.Allow(); err != nil {
		t.Fatalf("sonda en HalfOpen: %v", err)
	}
	if err := b.Allow(); !errors.Is(err, ErrOpen) {
		t.Fatalf("la segunda sonda debía ser rechazada, obtuve %v", err)
	}
	b.Success()
	if err := b.Allow(); err != nil {
		t.Fatalf("tras Success el circuito debe estar Closed: %v", err)
	}
}

func TestSetNowAndAllowAreRaceSafe(t *testing.T) {
	b := New(5, time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			b.SetNow(func() time.Time { return time.Unix(int64(i), 0) })
		}(i)
		go func() {
			defer wg.Done()
			_ = b.Allow()
			b.Success()
		}()
	}
	wg.Wait()
}
