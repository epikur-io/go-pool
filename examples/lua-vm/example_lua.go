package main

import (
	"log"
	"time"

	"github.com/epikur-io/go-pool"
	lua "github.com/yuin/gopher-lua"
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
	pool := pool.NewPool(10, factory)

	// get an entry:
	entry := pool.Acquire()
	entry.DoString(luaCode)

	// release entry
	pool.Release(entry)

	// get an entry or timeout after 1 second:
	entry, err := pool.AcquireWithTimeout(time.Second * 1)
	if err != nil {
		log.Println("error:", err)
	} else {
		// call some lua function
		entry.DoString(luaCode)
		if err := entry.CallByParam(lua.P{
			Fn:      entry.GetGlobal("some_global_function"),
			NRet:    1,
			Protect: true,
		}, lua.LString("epikur")); err != nil {
			panic(err)
		}

		// release a new entry to te pool and drop the old one
		pool.Release(nil)
	}
}
