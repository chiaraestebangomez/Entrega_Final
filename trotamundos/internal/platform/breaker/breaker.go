package breaker

import (
	"errors"
	"sync"
	"time"
)

var ErrOpen = errors.New("circuit breaker is open")

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

type Breaker struct {
	mu          sync.Mutex
	failures    int
	threshold   int
	openFor     time.Duration
	state       State
	openedUntil time.Time
	now         func() time.Time
}

func New(threshold int, openFor time.Duration) *Breaker {
	if threshold <= 0 {
		threshold = 5
	}
	if openFor <= 0 {
		openFor = 30 * time.Second
	}
	return &Breaker{threshold: threshold, openFor: openFor, now: time.Now}
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()
	return b.state
}

func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()
	if b.state == Open {
		return ErrOpen
	}
	return nil
}

func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.state = Closed
	b.openedUntil = time.Time{}
}

func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == HalfOpen {
		b.state = Open
		b.openedUntil = b.now().Add(b.openFor)
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.state = Open
		b.openedUntil = b.now().Add(b.openFor)
	}
}

func (b *Breaker) refresh() {
	if b.state == Open && !b.openedUntil.IsZero() && b.now().After(b.openedUntil) {
		b.state = HalfOpen
	}
}

func (b *Breaker) SetNow(fn func() time.Time) { b.now = fn }
