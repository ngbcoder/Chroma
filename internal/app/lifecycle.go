package app

import (
	"context"
	"sync"
)

// Lifecycle manages worker goroutines with a root context.
type Lifecycle struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewLifecycle() *Lifecycle {
	ctx, cancel := context.WithCancel(context.Background())
	return &Lifecycle{ctx: ctx, cancel: cancel}
}

func (l *Lifecycle) Context() context.Context { return l.ctx }

// Go launches a worker goroutine. The fn should respect ctx.Done().
func (l *Lifecycle) Go(fn func(ctx context.Context)) {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		fn(l.ctx)
	}()
}

// Shutdown cancels all workers and waits for completion.
func (l *Lifecycle) Shutdown() {
	l.cancel()
	l.wg.Wait()
}
