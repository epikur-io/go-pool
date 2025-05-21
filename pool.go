package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// Error messages
	ErrFailedToRelease        = fmt.Errorf("failed to release to pool")
	ErrMissingFactoryFunction = fmt.Errorf("missing factory function")
)

// Generic pool implementation

type IPool[T any] interface {
	Len() int
	Cap() int
	Acquire() *T
	AcquireWithTimeout(time.Duration) (*T, error)
	AcquireWithContext(context.Context) (*T, error)
	Release(*T)
	TryRelease(*T) error
	TryReleaseWithContext(context.Context, *T) error
	LockedRun(func(p *Pool[T]) error) error
	Channel() chan *T
	FactoryFunc() func() *T
}

var _ IPool[any] = &Pool[any]{}

// Creates a new pool with the given size/capacity
// factoryFunc returns the the type the pool should hold must be provided or else the call will panic
func NewPool[T any](size int, factoryFunc func() *T) *Pool[T] {
	if factoryFunc == nil {
		panic(ErrMissingFactoryFunction)
	}
	lp := &Pool[T]{size: size, factoryFunc: factoryFunc}
	lp.init()
	return lp
}

type Pool[T any] struct {
	// size of the pool
	size int
	// factory function to fill the pool
	factoryFunc func() *T
	pool        chan *T
	mux         sync.Mutex
}

func (p *Pool[T]) init() {
	p.mux = sync.Mutex{}
	p.pool = make(chan *T, p.size)
	// fill the pool
	for i := 0; i < p.size; i++ {
		p.pool <- p.factoryFunc()
	}
}

func (p *Pool[T]) Len() int {
	return len(p.pool)
}

func (p *Pool[T]) Cap() int {
	return cap(p.pool)
}

// Acquires a lock and  executes function f
func (p *Pool[T]) LockedRun(f func(p *Pool[T]) error) error {
	p.mux.Lock()
	defer p.mux.Unlock()
	return f(p)
}

func (p *Pool[T]) Channel() chan *T {
	return p.pool
}

func (p *Pool[T]) FactoryFunc() func() *T {
	return p.factoryFunc
}

func (p *Pool[T]) Run(fn func(e *T) error) error {
	e := p.Acquire()
	defer p.Release(nil)
	return fn(e)
}

func (p *Pool[T]) RunWithContext(ctx context.Context, fn func(ctx context.Context, e *T) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	e, err := p.AcquireWithContext(ctx)
	if err != nil {
		return err
	}
	defer p.Release(nil)
	return fn(ctx, e)
}

func (p *Pool[T]) AcquireWithTimeout(to time.Duration) (*T, error) {
	c := time.After(to)
	select {
	case v := <-p.pool:
		return v, nil
	case <-c:
		return nil, errors.New("timeout")
	}
}

func (p *Pool[T]) AcquireWithContext(ctx context.Context) (*T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-p.pool:
		return v, nil
	}
}

// Acquire an entry from the pool (blocking)
func (p *Pool[T]) Acquire() *T {
	return <-p.pool
}

// Releases an entry to the pool (blocking)
// if v is nil a new type gets created on the fly
func (p *Pool[T]) Release(v *T) {
	if v == nil {
		v = p.factoryFunc()
	}
	p.pool <- v
}

// Try to release an entry to the pool (non-blocking)
// if v is nil a new entry gets created on the fly
func (p *Pool[T]) TryRelease(v *T) error {
	if v == nil {
		v = p.factoryFunc()
	}
	select {
	case p.pool <- v:
	default:
		return ErrFailedToRelease
	}
	return nil
}

// Try to release an entry to the pool (non-blocking)
// if v is nil a new entry gets created on the fly
func (p *Pool[T]) TryReleaseWithContext(ctx context.Context, v *T) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if v == nil {
		v = p.factoryFunc()
	}
	select {
	case p.pool <- v:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
