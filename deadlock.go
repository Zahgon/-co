package co

import (
	"bytes"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
)

// This file implements the guard that turns the library's one silent failure
// mode into a loud one.
//
// Reactions run on a single microtask goroutine, and a generator body only
// makes progress while some driver is blocked waiting for it. Blocking either
// of those goroutines on Promise.Await stops the scheduler that would have
// settled the promise being awaited: the process hangs with no panic, and Go's
// deadlock detector stays quiet because other goroutines are still runnable.
// Detecting it up front costs one stack read on a path that was about to block
// anyway.

var (
	microtaskGoroutine atomic.Int64
	bodyGoroutines     sync.Map
)

// goid extracts the calling goroutine's id from the runtime stack header,
// which always begins "goroutine <id> [".
func goid() int64 {
	var buf [40]byte
	trace := buf[:runtime.Stack(buf[:], false)]

	trace = bytes.TrimPrefix(trace, []byte("goroutine "))
	end := bytes.IndexByte(trace, ' ')
	if end < 0 {
		return 0
	}

	id, err := strconv.ParseInt(string(trace[:end]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func registerBodyGoroutine() int64 {
	id := goid()
	bodyGoroutines.Store(id, struct{}{})
	return id
}

func releaseBodyGoroutine(id int64) {
	bodyGoroutines.Delete(id)
}

// assertAwaitable panics if blocking the calling goroutine would deadlock the
// scheduler. It is only consulted when Await is actually about to block.
func assertAwaitable() {
	id := goid()

	if id != 0 && microtaskGoroutine.Load() == id {
		panic("co: Promise.Await called from a promise reaction, which would " +
			"deadlock the microtask queue; return the promise from the handler " +
			"instead so it is adopted")
	}

	if _, ok := bodyGoroutines.Load(id); ok {
		panic("co: Promise.Await called from inside a generator body, which " +
			"would deadlock the microtask queue; use y.Yield(promise) instead")
	}
}
