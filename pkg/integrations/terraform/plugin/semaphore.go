package plugin

import (
	"context"
	"runtime"
	"sync"
)

const startupOrgLimiterBucket = "__terraform_startup__"

type Semaphore struct {
	ch chan struct{}
}

func NewSemaphore(limit int) *Semaphore {
	if limit <= 0 {
		limit = runtime.NumCPU() * 2
	}
	return &Semaphore{ch: make(chan struct{}, limit)}
}

func (s *Semaphore) Acquire(ctx context.Context) (func(), error) {
	select {
	case s.ch <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-s.ch })
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type OrgRateLimiter struct {
	mu     sync.Mutex
	limits map[string]chan struct{}
	max    int
	global *Semaphore
}

func NewOrgRateLimiter(maxPerOrg int, global *Semaphore) *OrgRateLimiter {
	if maxPerOrg <= 0 {
		maxPerOrg = 4
	}
	if global == nil {
		global = NewSemaphore(0)
	}
	return &OrgRateLimiter{
		limits: map[string]chan struct{}{},
		max:    maxPerOrg,
		global: global,
	}
}

func (l *OrgRateLimiter) Acquire(ctx context.Context, orgID string) (func(), error) {
	if orgID == "" {
		orgID = startupOrgLimiterBucket
	}
	globalRelease, err := l.global.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	orgLimit := l.orgLimit(orgID)
	select {
	case orgLimit <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() {
				<-orgLimit
				globalRelease()
			})
		}, nil
	case <-ctx.Done():
		globalRelease()
		return nil, ctx.Err()
	}
}

func (l *OrgRateLimiter) orgLimit(orgID string) chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()

	limit, ok := l.limits[orgID]
	if !ok {
		limit = make(chan struct{}, l.max)
		l.limits[orgID] = limit
	}
	return limit
}
