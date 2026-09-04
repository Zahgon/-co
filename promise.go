package co

import (
	"strconv"
	"sync"
)

// PromiseState mirrors the three states of a JavaScript promise.
type PromiseState int

// Promise states.
const (
	Pending PromiseState = iota
	Fulfilled
	Rejected
)

var promiseStateNames = [...]string{
	Pending:   "pending",
	Fulfilled: "fulfilled",
	Rejected:  "rejected",
}

// String renders the state name.
func (s PromiseState) String() string {
	if s < 0 || int(s) >= len(promiseStateNames) {
		return "PromiseState(" + strconv.Itoa(int(s)) + ")"
	}
	return promiseStateNames[s]
}

// Thenable is the Go equivalent of JavaScript's structural `.then` duck-type.
//
// `co` treats any value implementing Thenable as a promise, exactly as
// index.js:198-200 (`'function' == typeof obj.then`) does.
type Thenable interface {
	Then(onFulfilled func(any) any, onRejected func(error) any) *Promise
}

// Promise is a JavaScript-semantics promise.
//
// It settles at most once, adopts the state of a thenable it is resolved with,
// and delivers every reaction asynchronously on the shared microtask queue.
type Promise struct {
	mu        sync.Mutex
	state     PromiseState
	value     any
	err       error
	locked    bool // resolution procedure has begun; further resolve/reject are no-ops
	callbacks []func()
	done      chan struct{}
}

func newPending() *Promise {
	return &Promise{done: make(chan struct{})}
}

// New builds a promise from an executor, mirroring `new Promise(fn)`.
//
// The executor runs SYNCHRONOUSLY on the calling goroutine, as in JavaScript.
// A panic escaping the executor is converted into a rejection; this is what
// makes co.Co reject (rather than crash) when a generator function panics
// while being applied — see index.js:50-51.
func New(executor func(resolve func(any), reject func(error))) *Promise {
	p := newPending()
	func() {
		defer func() {
			if r := recover(); r != nil {
				p.reject(asError(r))
			}
		}()
		executor(p.resolve, p.reject)
	}()
	return p
}

// Resolve returns a promise fulfilled with v, adopting v if it is a thenable.
func Resolve(v any) *Promise {
	p := newPending()
	p.resolve(v)
	return p
}

// Reject returns a promise rejected with err.
func Reject(err error) *Promise {
	p := newPending()
	p.reject(err)
	return p
}

// resolve implements the promise resolution procedure.
func (p *Promise) resolve(v any) {
	if same, ok := v.(*Promise); ok && same == p {
		p.reject(&TypeError{Message: "Chaining cycle detected for promise"})
		return
	}

	p.mu.Lock()
	if p.locked {
		p.mu.Unlock()
		return
	}
	p.locked = true
	p.mu.Unlock()

	if th, ok := asThenable(v); ok {
		// Promises/A+ 2.3.3.2: if calling `then` throws, reject with the
		// reason. settle is called directly rather than reject because the
		// resolution procedure has already locked this promise; its
		// already-settled guard also implements 2.3.3.3.4, where a throw
		// after the promise has been settled is ignored.
		defer func() {
			if r := recover(); r != nil {
				p.settle(Rejected, nil, asError(r))
			}
		}()

		th.Then(
			func(x any) any { p.settle(Fulfilled, x, nil); return nil },
			func(e error) any { p.settle(Rejected, nil, e); return nil },
		)
		return
	}

	p.settle(Fulfilled, v, nil)
}

func (p *Promise) reject(err error) {
	p.mu.Lock()
	if p.locked {
		p.mu.Unlock()
		return
	}
	p.locked = true
	p.mu.Unlock()

	p.settle(Rejected, nil, err)
}

// settle transitions to a final state and flushes pending reactions. It is
// only ever reached once per promise because `locked` gates every entry point.
func (p *Promise) settle(state PromiseState, value any, err error) {
	p.mu.Lock()
	if p.state != Pending {
		p.mu.Unlock()
		return
	}
	p.state = state
	p.value = value
	p.err = err
	cbs := p.callbacks
	p.callbacks = nil
	close(p.done)
	p.mu.Unlock()

	for _, cb := range cbs {
		schedule(cb)
	}
}

// Then registers reactions and returns a derived promise, mirroring
// `promise.then(onFulfilled, onRejected)`.
//
// Either handler may be nil, in which case the corresponding settlement passes
// straight through. A handler's return value resolves the derived promise
// (adopting thenables); a panic — including co.Throw — rejects it.
func (p *Promise) Then(onFulfilled func(any) any, onRejected func(error) any) *Promise {
	derived := newPending()

	react := func() {
		p.mu.Lock()
		state, value, err := p.state, p.value, p.err
		p.mu.Unlock()

		defer func() {
			if r := recover(); r != nil {
				derived.reject(asError(r))
			}
		}()

		if state == Fulfilled {
			if onFulfilled == nil {
				derived.resolve(value)
				return
			}
			derived.resolve(onFulfilled(value))
			return
		}

		if onRejected == nil {
			derived.reject(err)
			return
		}
		derived.resolve(onRejected(err))
	}

	p.mu.Lock()
	if p.state == Pending {
		p.callbacks = append(p.callbacks, react)
		p.mu.Unlock()
	} else {
		p.mu.Unlock()
		schedule(react)
	}

	return derived
}

// Catch registers a rejection handler, mirroring `promise.catch(fn)`.
func (p *Promise) Catch(onRejected func(error) any) *Promise {
	return p.Then(nil, onRejected)
}

// Finally runs fn on settlement without altering the settled value.
func (p *Promise) Finally(fn func()) *Promise {
	return p.Then(
		func(v any) any { fn(); return v },
		func(e error) any { fn(); panic(thrown{err: e}) },
	)
}

// Await blocks the calling goroutine until the promise settles.
//
// It must not be called from a promise reaction or from inside a generator
// body: both run on goroutines the scheduler needs in order to settle anything,
// so blocking one hangs the process. Doing so panics rather than deadlocking.
// Inside a generator body use y.Yield(promise); inside a reaction return the
// promise and let it be adopted.
//
// Awaiting an already-settled promise is always safe and never panics.
func (p *Promise) Await() (any, error) {
	select {
	case <-p.done:
	default:
		assertAwaitable()
		<-p.done
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	return p.value, p.err
}

// Done exposes a channel closed when the promise settles, for use with select.
func (p *Promise) Done() <-chan struct{} {
	return p.done
}

// State reports the current state.
func (p *Promise) State() PromiseState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// All resolves with a slice of settled values once every entry has settled,
// and rejects with the first rejection it sees.
//
// Entries that are not thenables are passed through unchanged, matching
// `Promise.all` (index.js:155, index.js:177).
func All(values []any) *Promise {
	return New(func(resolve func(any), reject func(error)) {
		results := make([]any, len(values))
		if len(values) == 0 {
			resolve(results)
			return
		}

		var mu sync.Mutex
		remaining := len(values)

		settleOne := func(i int, v any) {
			mu.Lock()
			results[i] = v
			remaining--
			finished := remaining == 0
			mu.Unlock()
			if finished {
				resolve(results)
			}
		}

		for i, v := range values {
			th, ok := asThenable(v)
			if !ok {
				settleOne(i, v)
				continue
			}
			index := i
			th.Then(
				func(x any) any { settleOne(index, x); return nil },
				func(e error) any { reject(e); return nil },
			)
		}
	})
}
