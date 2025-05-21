package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/epikur-io/go-pool"
	lua "github.com/epikur-io/gopher-lua"
)

var luaCode = `
	print("Hello world!")

	function some_global_function(name)
		print("Hello:", name)
	end
`

func main() {
	factory := func() *lua.LState {
		lvm := lua.NewState()
		return lvm
	}
	pool := pool.NewPool(5000, factory)

	// get an entry:
	entry := pool.Acquire()
	entry.DoString(luaCode)

	// release entry
	pool.Release(entry)

	wg := sync.WaitGroup{}
	for i := range 200000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// get an entry or timeout after 1 second:
			entry, err := pool.AcquireWithTimeout(time.Second * 5)
			if err != nil {
				log.Printf("error acquiring from pool (#%d): %s", i, err)
				return
			}

			// call some lua function
			entry.DoString(luaCode)
			if err := entry.CallByParam(lua.P{
				Fn:      entry.GetGlobal("some_global_function"),
				NRet:    1,
				Protect: true,
			}, lua.LString("epikur #"+fmt.Sprint(i))); err != nil {
				panic(err)
			}

			// release a new entry to te pool and drop the old one
			fmt.Printf("## Finished iteration #%d\n", i)
			pool.Release(nil)
		}()
	}
	wg.Wait()
}
