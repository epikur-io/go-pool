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
	Update()
	UpdateWithTimeout(time.Duration) (int, int)
	Acquire() *T
	AcquireWithTimeout(time.Duration) (*T, error)
	AcquireWithContext(context.Context) (*T, error)
	Release(*T)
	TryRelease(*T) error
	TryReleaseWithContext(context.Context, *T) error
}

var _ IPool[any] = &Pool[any]{}

// Creates a new pool with the given size/capacity
// factoryFunc returns the the type the pool should hold must be provided or else the call will panic
func NewPool[T any](size int, factoryFunc func() *T) *Pool[T] {
	if factoryFunc == nil {
		panic(ErrMissingFactoryFunction)
	}
	lp := &Pool[T]{size: size, creator: factoryFunc}
	lp.init()
	return lp
}

type Pool[T any] struct {
	// size of the pool
	size int
	// factory function to fill the pool
	creator func() *T
	pool    chan *T
	mux     sync.Mutex
}

func (p *Pool[T]) init() {
	p.mux = sync.Mutex{}
	p.pool = make(chan *T, p.size)
	// fill the pool
	for i := 0; i < p.size; i++ {
		p.pool <- p.creator()
	}
}

func (p *Pool[T]) Len() int {
	return len(p.pool)
}

func (p *Pool[T]) Cap() int {
	return cap(p.pool)
}

// Updates the pool and fills it with fresh newly instantiated entries
func (p *Pool[T]) Update() {
	// Make sure the pool is empty so we don't miss a entry because
	// it was acquired by an other function
	// So this loop can take a while if some entries are already acquired and busy.
	p.mux.Lock()
	defer p.mux.Unlock()

	for i := 0; i < cap(p.pool); i++ {
		// empty the Pool
		<-p.pool
	}
	for i := 0; i < cap(p.pool); i++ {
		// fill the Pool
		p.pool <- p.creator()
	}
}

// Updates the pool and fills it with fresh newly instantiated entries or thows a timout error
// this can leave the pool not or only partially filled. Use it with caution!
func (p *Pool[T]) UpdateWithTimeout(to time.Duration) (removedInstanceCount int, newInstanceCount int) {
	p.mux.Lock()
	defer p.mux.Unlock()

	c := time.After(to)
	for i := 0; i < cap(p.pool); i++ {
		// try to empty the Pool
		select {
		case <-p.pool:
			removedInstanceCount++
		case <-c:
			return
		}
	}
	for i := 0; i < cap(p.pool); i++ {
		// try to fill the Pool
		select {
		case p.pool <- p.creator():
			newInstanceCount++
		case <-c:
			return
		}

	}
	return
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
		p.pool <- p.creator()
		return
	}
	p.pool <- v
}

// Try to release an entry to the pool (non-blocking)
// if v is nil a new entry gets created on the fly
func (p *Pool[T]) TryRelease(v *T) error {
	if v == nil {
		v = p.creator()
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
		v = p.creator()
	}
	select {
	case p.pool <- v:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
