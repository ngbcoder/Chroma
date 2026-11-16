package app

import (
	"context"
	"log"
	"runtime/debug"
	"sync"
	"time"
)

const (
	workerPanicRetryBaseDelay = 100 * time.Millisecond
	workerPanicRetryMaxDelay  = 2 * time.Second
	workerPanicRetryLimit     = 5
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
// If a worker panics, it is restarted with a bounded exponential backoff.
func (l *Lifecycle) Go(fn func(ctx context.Context)) {
	if fn == nil {
		return
	}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.runWorker(fn)
	}()
}

func (l *Lifecycle) runWorker(fn func(ctx context.Context)) {
	for attempt := 0; ; attempt++ {
		if l.ctx.Err() != nil {
			return
		}

		panicValue := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					panicValue = true
					if attempt >= workerPanicRetryLimit {
						log.Printf("worker panic exhausted retries: %v\n%s", r, debug.Stack())
						return
					}
					log.Printf("worker panic recovered: %v\n%s", r, debug.Stack())
				}
			}()
			fn(l.ctx)
		}()

		if !panicValue {
			return
		}

		if attempt >= workerPanicRetryLimit {
			return
		}

		delay := workerPanicRetryBaseDelay << min(attempt, 5)
		if delay > workerPanicRetryMaxDelay {
			delay = workerPanicRetryMaxDelay
		}

		timer := time.NewTimer(delay)
		select {
		case <-l.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// Shutdown cancels all workers and waits for completion.
func (l *Lifecycle) Shutdown() {
	l.cancel()
	l.wg.Wait()
}
