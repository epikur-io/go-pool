package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/epikur-io/go-pool"
)

type PoolEntry struct{}

func (pe *PoolEntry) DoSomeWork(input string) {
	log.Printf("Do some work... Input: %v\n", input)
	time.Sleep(time.Second * 1)
}

func main() {
	factory := func() *PoolEntry {
		return &PoolEntry{}
	}
	pool := pool.NewPool(10, factory)

	// get an entry:
	{
		entry := pool.Acquire()
		// release entry
		defer pool.Release(entry)
		entry.DoSomeWork("A")
	}
	{
		// get an entry with the given context (for timeouts and deadlines):
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
		defer cancel()
		entry, err := pool.AcquireWithContext(ctx)
		// release entry, since entry is nil, a new entry will be created and put into the pool
		defer pool.Release(nil)
		entry.DoSomeWork("B")
		if err != nil {
			log.Fatalln("error:", err)
		}
	}
	{
		// automatically release entries back to the pool
		err := pool.RunWithContext(context.Background(), func(ctx context.Context, e *PoolEntry) error {
			// a freshly created entry will be automatically released on function exit using: `pool.Release(nil)`
			e.DoSomeWork("C")
			return fmt.Errorf("dummy error")
		})
		if err != nil {
			log.Fatalln("error:", err)
		}
	}
}
