package main

import (
	"log"
	"time"

	"github.com/epikur-io/go-pool"
)

type PoolEntry struct{}

func (pe *PoolEntry) DoSomeWork() {
	log.Println("Do some work...")
	time.Sleep(time.Second * 1)
}

func main() {
	factory := func() *PoolEntry {
		return &PoolEntry{}
	}
	pool := pool.NewPool(10, factory)

	// get an entry:
	entry := pool.Acquire()
	entry.DoSomeWork()

	// release entry
	pool.Release(entry)

	// get an entry or timeout after 1 second:
	entry, err := pool.AcquireWithTimeout(time.Second * 1)
	if err != nil {
		log.Fatalln("error:", err)
	}

	entry.DoSomeWork()

	// release entry
	pool.Release(entry)
}
