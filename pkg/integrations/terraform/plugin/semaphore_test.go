package plugin

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSemaphore(t *testing.T) {
	sem := NewSemaphore(1)
	release, err := sem.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	waiting := make(chan struct{})
	acquired := make(chan func())
	go func() {
		close(waiting)
		release, err := sem.Acquire(context.Background())
		if err == nil {
			acquired <- release
		}
	}()
	<-waiting
	select {
	case <-acquired:
		t.Fatal("acquired while semaphore was full")
	case <-time.After(20 * time.Millisecond):
	}

	release()
	select {
	case secondRelease := <-acquired:
		secondRelease()
	case <-time.After(time.Second):
		t.Fatal("waiting acquire did not proceed after release")
	}
}

func TestSemaphoreContextCancellation(t *testing.T) {
	sem := NewSemaphore(1)
	release, err := sem.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = sem.Acquire(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire() error = %v, want context.Canceled", err)
	}
}

func TestOrgRateLimiter(t *testing.T) {
	limiter := NewOrgRateLimiter(1, NewSemaphore(4))
	releaseA, err := limiter.Acquire(context.Background(), "org-a")
	if err != nil {
		t.Fatalf("Acquire(org-a) error = %v", err)
	}

	releaseB, err := limiter.Acquire(context.Background(), "org-b")
	if err != nil {
		t.Fatalf("Acquire(org-b) error = %v", err)
	}
	releaseB()

	acquiredA := make(chan func())
	go func() {
		release, err := limiter.Acquire(context.Background(), "org-a")
		if err == nil {
			acquiredA <- release
		}
	}()
	select {
	case <-acquiredA:
		t.Fatal("same org acquired while at limit")
	case <-time.After(20 * time.Millisecond):
	}

	releaseA()
	select {
	case secondReleaseA := <-acquiredA:
		secondReleaseA()
	case <-time.After(time.Second):
		t.Fatal("same org did not acquire after release")
	}
}

func TestOrgRateLimiterAllowsStartupWithoutOrgID(t *testing.T) {
	limiter := NewOrgRateLimiter(1, NewSemaphore(2))
	release, err := limiter.Acquire(context.Background(), "")
	if err != nil {
		t.Fatalf("Acquire(empty org) error = %v", err)
	}
	release()
}

func TestOrgRateLimiterGlobalLimit(t *testing.T) {
	limiter := NewOrgRateLimiter(4, NewSemaphore(1))
	releaseA, err := limiter.Acquire(context.Background(), "org-a")
	if err != nil {
		t.Fatalf("Acquire(org-a) error = %v", err)
	}
	defer releaseA()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = limiter.Acquire(ctx, "org-b")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire(org-b) error = %v, want context deadline", err)
	}
}

func TestOrgRateLimiterCancellationAndIdempotentRelease(t *testing.T) {
	limiter := NewOrgRateLimiter(1, NewSemaphore(4))
	release, err := limiter.Acquire(context.Background(), "org-a")
	if err != nil {
		t.Fatalf("Acquire(org-a) error = %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = limiter.Acquire(ctx, "org-a")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire(org-a) error = %v, want context deadline", err)
	}

	release()
	release()
	reacquired, err := limiter.Acquire(context.Background(), "org-a")
	if err != nil {
		t.Fatalf("Acquire after idempotent release error = %v", err)
	}
	reacquired()
}
