package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newIntPool(size int) *Pool[int] {
	n := 0
	return NewPool(size, func() *int {
		n++
		v := n
		return &v
	})
}

func TestAcquireRelease(t *testing.T) {
	p := newIntPool(2)
	defer p.Close()

	v, err := p.Acquire()
	if err != nil || v == nil {
		t.Fatalf("Acquire() = %v, %v; want non-nil, nil", v, err)
	}
	if p.Len() != 1 {
		t.Fatalf("Len() = %d; want 1", p.Len())
	}
	if err := p.Release(v); err != nil {
		t.Fatalf("Release() = %v; want nil", err)
	}
	if p.Len() != 2 {
		t.Fatalf("Len() = %d; want 2", p.Len())
	}
}

func TestReleaseNilReturnsError(t *testing.T) {
	p := newIntPool(2)
	defer p.Close()

	if err := p.Release(nil); err == nil {
		t.Fatal("Release(nil) should return an error")
	}
}

func TestRun_HealthyItemReturned(t *testing.T) {
	p := newIntPool(1)
	defer p.Close()

	var seen *int
	err := p.Run(func(v *int) error {
		seen = v
		return nil
	})
	if err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}
	if p.Len() != 1 {
		t.Fatalf("pool should be full after Run; Len() = %d", p.Len())
	}

	// The exact same pointer should be back in the
	v, _ := p.Acquire()
	if v != seen {
		t.Fatal("Run() did not return the original item to the pool")
	}
}

func TestRun_BrokenItemReplaced(t *testing.T) {
	created := 0
	p := NewPool(1, func() *int {
		created++
		v := created
		return &v
	})
	defer p.Close()

	_ = p.Run(func(v *int) error {
		return errors.New("something broke")
	})

	if created != 2 {
		t.Fatalf("expected 2 items created (1 initial + 1 replacement), got %d", created)
	}
	if p.Len() != 1 {
		t.Fatalf("pool should still be full after failed Run; Len() = %d", p.Len())
	}
}

func TestAcquireWithContext_AlreadyCancelled(t *testing.T) {
	p := newIntPool(1)
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before acquire

	v, err := p.AcquireWithContext(ctx)
	if v != nil || err == nil {
		t.Fatalf("expected error from pre-cancelled ctx, got v=%v err=%v", v, err)
	}
}

func TestAcquireWithTimeout_Expires(t *testing.T) {
	p := newIntPool(1)
	defer p.Close()

	// Exhaust the
	_, _ = p.Acquire()

	_, err := p.AcquireWithTimeout(20 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestReplace(t *testing.T) {
	created := 0
	p := NewPool(1, func() *int {
		created++
		v := created
		return &v
	})
	defer p.Close()

	v, _ := p.Acquire()
	if err := p.Replace(v); err != nil {
		t.Fatalf("Replace() = %v; want nil", err)
	}
	if created != 2 {
		t.Fatalf("expected 2 items after Replace, got %d", created)
	}
	if p.Len() != 1 {
		t.Fatalf("Len() = %d; want 1", p.Len())
	}
}

func TestClose_IdempotentAndBlocksAcquire(t *testing.T) {
	p := newIntPool(2)
	p.Close()
	p.Close() // must not panic

	_, err := p.Acquire()
	if err != ErrPoolClosed {
		t.Fatalf("Acquire on closed pool = %v; want ErrPoolClosed", err)
	}
}

func TestClose_CallsCloseFunc(t *testing.T) {
	closed := 0
	p := NewPool(3, func() *int {
		v := 0
		return &v
	}, Options[int]{
		CloseFunc: func(v *int) { closed++ },
	})
	p.Close()
	if closed != 3 {
		t.Fatalf("CloseFunc called %d times; want 3", closed)
	}
}

func TestConcurrentStress(t *testing.T) {
	const goroutines = 100
	const iters = 200

	p := newIntPool(10)
	defer p.Close()

	var wg sync.WaitGroup
	var errs atomic.Int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				if err := p.Run(func(v *int) error { return nil }); err != nil {
					errs.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	if errs.Load() != 0 {
		t.Fatalf("%d errors during stress test", errs.Load())
	}
	if p.Len() != p.Cap() {
		t.Fatalf("pool not fully returned after stress: Len=%d Cap=%d", p.Len(), p.Cap())
	}
}

func BenchmarkPool_Run(b *testing.B) {
	p := newIntPool(8)
	defer p.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.Run(func(v *int) error { return nil })
		}
	})
}
