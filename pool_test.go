package pool

import (
	"fmt"
	"testing"
	"time"
)

type poolItem any

func poolFactory() *poolItem {
	return nil
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
	pool.LockedRun(func(p *Pool[poolItem]) error {
		for i := 0; i < p.Cap(); i++ {
			// empty the Pool
			p.Acquire()
		}
		for i := 0; i < p.Cap(); i++ {
			// fill the Pool
			p.Release(nil)
		}

		return nil
	})
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
	pool.LockedRun(func(p *Pool[poolItem]) error {
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
	})

	if removedInstances != 0 {
		t.Errorf("expected %d removed instances but got %d", pool.Len(), removedInstances)
	}
	if updatedInstances != 0 {
		t.Errorf("expected %d updated instances but got %d", pool.Len(), updatedInstances)
	}
}
