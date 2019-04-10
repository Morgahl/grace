package grace

import (
	"context"
	"os"
	"os/signal"
	"sync"
)

// Grace is a concurrent safe graceful cleanup handler for use in long running tasks.
type Grace struct {
	mu      sync.RWMutex
	ctx     context.Context
	can     context.CancelFunc
	sig     chan os.Signal
	cleanup []interface{}
	invalid bool
}

// New instantiates an instance of Grace, if nil context.Context is passed it will use context.Background() internally.
func New(ctx context.Context, sigs ...os.Signal) *Grace {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, can := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, sigs...)

	return &Grace{
		ctx: ctx,
		can: can,
		sig: sig,
	}
}

// Cancel stops the Grace object from recieving signals and performs graceful cleanup.
func (g *Grace) Cancel() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.invalid {
		return
	}

	g.runCleanup()
}

// Context returns a reference to the internal context.Context for propigation of downstream cancellation when triggered
func (g *Grace) Context() context.Context {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.invalid {
		return context.Background()
	}

	return g.ctx
}

// RegisterCleanup will register a function to be run for cleanup.
// Operations are expected to complete syncronously.
// Cleanup will be performed in FILO order once triggered.
func (g *Grace) RegisterCleanup(fn func()) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.invalid {
		return
	}

	g.cleanup = append(g.cleanup, fn)
}

// RegisterCleanupError will register a function to be run for cleanup.
// Operations are expected to complete syncronously.
// Cleanup will be performed in FILO order once triggered.
func (g *Grace) RegisterCleanupError(fn func() error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.invalid {
		return
	}

	g.cleanup = append(g.cleanup, fn)
}

// Wait will only return once a configured os.Signal is received or the base contex is cancelled.
func (g *Grace) Wait() {
	select {
	case <-g.ctx.Done():
	case <-g.sig:
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	g.runCleanup()
}

// runCleanup MUST be called from a function holding the recieving Grace object's mutex lock.
func (g *Grace) runCleanup() {
	if g.invalid {
		return
	}
	g.invalid = true

	signal.Stop(g.sig)
	g.can()

	for i := len(g.cleanup) - 1; i >= 0; i-- {
		switch fnType := g.cleanup[i].(type) {
		case func():
			fnType()
		case func() error:
			fnType()
		}
	}
	g.cleanup = nil
}
