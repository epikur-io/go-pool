package pool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

var (
	ErrFailedToRelease        = fmt.Errorf("failed to release to pool")
	ErrMissingFactoryFunction = fmt.Errorf("missing factory function")
	ErrPoolClosed             = fmt.Errorf("pool is closed")
	ErrNilItem                = fmt.Errorf("cannot release nil item; use Replace() to swap in a fresh one")
)

// Pooler is the interface satisfied by Pool[T].
type Pooler[T any] interface {
	Len() int
	Cap() int

	Acquire() (*T, error)
	AcquireWithTimeout(time.Duration) (*T, error)
	AcquireWithContext(context.Context) (*T, error)

	Release(*T) error
	ReleaseWithContext(context.Context, *T) error
	TryRelease(*T) error
	TryReleaseWithContext(context.Context, *T) error

	// Replace discards v (which may be nil/broken) and returns a fresh item
	// to the pool. It is the explicit, unambiguous way to handle a bad item.
	Replace(v *T) error

	Run(func(*T) error) error
	RunWithContext(context.Context, func(context.Context, *T) error) error

	Close() error
}

var _ Pooler[any] = &Pool[any]{}

// Pool is a bounded, goroutine-safe resource pool.
type Pool[T any] struct {
	size        int
	factoryFunc func() *T
	closeFunc   func(*T) // optional; called on each item when the pool closes
	pool        chan *T
	closeOnce   sync.Once
	closed      chan struct{}
}

// Options holds optional configuration for NewPool.
type Options[T any] struct {
	// CloseFunc is called for every item when Close() is invoked.
	// Use it to release underlying resources (e.g. L.Close() for Lua VMs).
	CloseFunc func(*T)
}

// NewPool creates a new pool of size cap, filling it eagerly via factoryFunc.
// Panics if factoryFunc is nil, size < 1, or factoryFunc returns nil.
//
// Pool lifecycle:
//   - Created via NewPool (pre-populated with items).
//   - Used via Acquire/Release cycles.
//   - Closed via Close (idempotent, safe to call multiple times).
//
// After Close:
//   - All Acquire calls return ErrPoolClosed.
//   - Release/Replace calls close the item and return ErrPoolClosed.
//   - Items in-flight at close time are closed when next returned to the pool.
func NewPool[T any](size int, factoryFunc func() *T, opts ...Options[T]) *Pool[T] {
	if factoryFunc == nil {
		panic(ErrMissingFactoryFunction)
	}
	if size < 1 {
		panic("pool: size must be >= 1")
	}

	p := &Pool[T]{
		size:        size,
		factoryFunc: factoryFunc,
		pool:        make(chan *T, size),
		closed:      make(chan struct{}),
	}
	if len(opts) > 0 {
		p.closeFunc = opts[0].CloseFunc
	}

	for i := 0; i < size; i++ {
		item := factoryFunc()
		if item == nil {
			panic("pool: factory function returned nil")
		}
		p.pool <- item
	}
	return p
}

// Len returns the approximate number of items currently idle in the pool.
// The value may be stale by the time it is read; treat it as a hint only.
func (p *Pool[T]) Len() int { return len(p.pool) }

// Cap returns the maximum pool capacity.
func (p *Pool[T]) Cap() int { return cap(p.pool) }

// Acquire blocks until an item is available or the pool is closed.
func (p *Pool[T]) Acquire() (*T, error) {
	return p.AcquireWithContext(context.Background())
}

// AcquireWithTimeout blocks until an item is available or the timeout elapses.
func (p *Pool[T]) AcquireWithTimeout(d time.Duration) (*T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return p.AcquireWithContext(ctx)
}

// AcquireWithContext blocks until an item is available, ctx is cancelled, or
// the pool is closed. A cancelled context is checked eagerly before blocking
// so a pre-cancelled context never returns an item.
func (p *Pool[T]) AcquireWithContext(ctx context.Context) (*T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Eagerly check: don't hand out an item to an already-cancelled caller.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-p.closed:
		return nil, ErrPoolClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-p.pool:
		return v, nil
	}
}

// Release returns a healthy item to the pool. v must not be nil; use Replace
// to handle a broken item.
func (p *Pool[T]) Release(v *T) error {
	if v == nil {
		return ErrNilItem
	}
	select {
	case <-p.closed:
		p.closeItem(v)
		return ErrPoolClosed
	case p.pool <- v:
		return nil
	}
}

// ReleaseWithContext returns a healthy item to the pool, blocking until space
// is available, the context is cancelled, or the pool is closed.
// The context is checked eagerly so a pre-cancelled context never blocks.
func (p *Pool[T]) ReleaseWithContext(ctx context.Context, v *T) error {
	if v == nil {
		return ErrNilItem
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-p.closed:
		p.closeItem(v)
		return ErrPoolClosed
	case <-ctx.Done():
		return ctx.Err()
	case p.pool <- v:
		return nil
	}
}

// TryRelease is a non-blocking Release. Returns ErrFailedToRelease if the
// pool is full.
func (p *Pool[T]) TryRelease(v *T) error {
	if v == nil {
		return ErrNilItem
	}
	select {
	case <-p.closed:
		p.closeItem(v)
		return ErrPoolClosed
	case p.pool <- v:
		return nil
	default:
		return ErrFailedToRelease
	}
}

// TryReleaseWithContext is a non-blocking Release that also respects ctx.
// The context is checked eagerly; the select's default branch means ctx.Done()
// inside the select would be unreachable anyway (a full channel hits default
// before ctx.Done() is evaluated).
func (p *Pool[T]) TryReleaseWithContext(ctx context.Context, v *T) error {
	if v == nil {
		return ErrNilItem
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-p.closed:
		p.closeItem(v)
		return ErrPoolClosed
	case p.pool <- v:
		return nil
	default:
		return ErrFailedToRelease
	}
}

// Replace closes v (if a CloseFunc was provided), creates a fresh item via
// factoryFunc, and returns it to the pool. Pass the item you acquired when it
// is broken or otherwise unfit for reuse.
func (p *Pool[T]) Replace(v *T) error {
	p.closeItem(v) // safe with nil
	fresh := p.factoryFunc()
	if fresh == nil {
		panic("pool: factory function returned nil during Replace")
	}
	select {
	case <-p.closed:
		p.closeItem(fresh)
		return ErrPoolClosed
	case p.pool <- fresh:
		return nil
	}
}

// Run acquires an item, calls fn, then returns or replaces it depending on
// whether fn returned an error.
func (p *Pool[T]) Run(fn func(*T) error) error {
	return p.RunWithContext(context.Background(), func(_ context.Context, v *T) error {
		return fn(v)
	})
}

// RunWithContext is like Run but propagates ctx to both acquisition and fn.
// ctx is checked eagerly for consistency; AcquireWithContext would catch it
// anyway, but this makes the early-exit behaviour obvious without tracing deeper.
func (p *Pool[T]) RunWithContext(ctx context.Context, fn func(context.Context, *T) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := p.AcquireWithContext(ctx)
	if err != nil {
		return err
	}

	fnErr := fn(ctx, v)
	if fnErr != nil {
		// Item may be in a bad state; replace it rather than returning it.
		// If Replace fails (pool already closed), close the item directly so
		// it is not leaked — the primary fnErr is still what we return.
		if replaceErr := p.Replace(v); replaceErr != nil {
			p.closeItem(v)
		}
		return fnErr
	}

	return p.Release(v)
}

// Close shuts the pool down. All idle items are closed immediately via
// CloseFunc (if set). Items that are currently acquired will be closed when
// they are returned via Release/Replace.
func (p *Pool[T]) Close() error {
	p.closeOnce.Do(func() {
		close(p.closed)
		// Drain and close all idle items.
		for {
			select {
			case v := <-p.pool:
				p.closeItem(v)
			default:
				return
			}
		}
	})
	return nil
}

func (p *Pool[T]) closeItem(v *T) {
	if p.closeFunc != nil && v != nil {
		p.closeFunc(v)
	}
}
