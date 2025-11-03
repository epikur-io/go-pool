# go-pool - A generic pool implementation in Go

*A lightweight, generic object pool implementation in Go*

[![Go Reference](https://pkg.go.dev/badge/github.com/epikur-io/go-pool.svg)](https://pkg.go.dev/github.com/epikur-io/go-pool)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Overview

`go-pool` is a generic, efficient object pool for Go designed to manage reusable, expensive-to-create objects such as virtual machines, interpreters, or database connections.

Instead of repeatedly allocating and destroying large objects, you can use this pool to **acquire**, **reuse**, and **release** instances safely and efficiently.

## Features

- Generic API – works with any object type
- Optimized for performance and low contention
- Simple, idiomatic interface
- Supports timeouts for acquiring objects
- Optional helper for automatic acquire/release management

## Use Cases

- Embedding scripting engines (Lua, JS, etc.)
- Managing reusable network connections or sessions
- Pooling pre-initialized workers or buffers
- Any case where object creation is expensive

## Usage Example

```go
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

	// or use function wrapper to automatically acquire and release entry
	err := pool.Run(func(entry *PoolEntry) error {
		entry.DoSomeWork()
		return nil
	})
}
```