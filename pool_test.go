package pool

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type poolItem any

func poolFactory() *poolItem {
	return nil
}

func TestFactoryFunc(t *testing.T) {
	pool := NewPool(2, poolFactory)
	if pool.FactoryFunc() == nil {
		t.Errorf("missing factory function")
	}
}

func TestAqcuireAndRelease(t *testing.T) {
	pool := NewPool(2, poolFactory)
	entries := []*poolItem{}
	for range 2 {
		entry := pool.Acquire()
		entries = append(entries, entry)
	}

	plen := pool.Len()
	if plen != 0 {
		t.Errorf("pool expected to be empty but %d instances remained", plen)
	}

	// should timeout since pool is now empty
	_, err := pool.AcquireWithTimeout(1 * time.Second)
	if err == nil {
		t.Errorf("expected timout error but got %v", err)
	}

	for _, entry := range entries {
		pool.Release(entry)
	}

	plen = pool.Len()
	if plen != 2 {
		t.Errorf("pool expected to be full but got %d instances", plen)
	}

	_, err = pool.AcquireWithTimeout(1 * time.Second)
	if err != nil {
		t.Errorf("unexpected error: %e", err)
	}
}

func TestAqcuireAndReleaseWithContext(t *testing.T) {
	pool := NewPool(2, poolFactory)
	entries := []*poolItem{}
	for range 2 {
		entry := pool.Acquire()
		entries = append(entries, entry)
	}

	plen := pool.Len()
	if plen != 0 {
		t.Errorf("pool expected to be empty but %d instances remained", plen)
	}

	// should timeout since pool is now empty
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	_, err := pool.AcquireWithContext(ctx)
	cancel()
	if err == nil {
		t.Errorf("expected timout error but got %v", err)
	}
	for _, entry := range entries {
		pool.Release(entry)
	}

	plen = pool.Len()
	if plen != 2 {
		t.Errorf("pool expected to be full but got %d instances", plen)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
	_, err = pool.AcquireWithContext(ctx)
	cancel()
	if err != nil {
		t.Errorf("unexpected error: %e", err)
	}
}

func TestTryRelease(t *testing.T) {
	pool := NewPool(2, poolFactory)
	err := pool.TryRelease(nil)
	if err == nil {
		t.Errorf("error expected but got %v", err)
	}

	entry := pool.Acquire()
	err = pool.TryRelease(entry)
	if err != nil {
		t.Errorf("unexpected error: %e", err)
	}
}
func TestTryReleaseWithContext(t *testing.T) {
	pool := NewPool(2, poolFactory)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	err := pool.TryReleaseWithContext(ctx, nil)
	cancel()
	if err == nil {
		t.Errorf("error expected but got %v", err)
	}

	entry := pool.Acquire()
	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
	err = pool.TryReleaseWithContext(ctx, entry)
	cancel()
	if err != nil {
		t.Errorf("unexpected error: %e", err)
	}
}

func TestUpdate(t *testing.T) {
	pool := NewPool(2, poolFactory)
	entries := []*poolItem{}
	for range 2 {
		entry := pool.Acquire()
		entries = append(entries, entry)
	}
	delay := 1 * time.Second
	go func() {
		time.Sleep(delay)
		for _, entry := range entries {
			pool.Release(entry)
		}
	}()
	start := time.Now()
	if err := pool.LockedRun(func(p *Pool[poolItem]) error {
		for i := 0; i < p.Cap(); i++ {
			// empty the Pool
			p.Acquire()
		}
		for i := 0; i < p.Cap(); i++ {
			// fill the Pool
			p.Release(nil)
		}

		return nil
	}); err != nil {
		t.Error(err)
	}
	duration := time.Since(start)

	if duration < delay {
		// check if channel was blocking
		t.Errorf("expected pool to block for 1 second but got %v", duration)
	}

}

func TestUpdateTimeout(t *testing.T) {
	pool := NewPool(3, poolFactory)
	for range 3 {
		pool.Acquire()
	}

	removedInstances := 0
	updatedInstances := 0

	tc := time.After(1 * time.Second)
	if err := pool.LockedRun(func(p *Pool[poolItem]) error {
		for i := 0; i < p.Cap(); i++ {
			// empty the Pool
			select {
			case <-p.Channel():
				removedInstances++
			case <-tc:
				return fmt.Errorf("timeout")
			}
		}
		factoryFunc := p.FactoryFunc()
		for i := 0; i < p.Cap(); i++ {
			// fill the Pool
			select {
			case p.Channel() <- factoryFunc():
				updatedInstances++
			case <-tc:
				return fmt.Errorf("timeout")
			}
		}

		return nil
	}); err == nil {
		t.Errorf("error expected but got %v", err)
	}

	if removedInstances != 0 {
		t.Errorf("expected %d removed instances but got %d", pool.Len(), removedInstances)
	}
	if updatedInstances != 0 {
		t.Errorf("expected %d updated instances but got %d", pool.Len(), updatedInstances)
	}
}
