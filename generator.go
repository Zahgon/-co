package co

import "sync"

// Result mirrors the `{ value, done }` object returned by a JavaScript
// iterator's next()/throw().
type Result struct {
	Value any
	Done  bool
}

// Generator is the Go counterpart of a JavaScript generator object.
//
// index.js:211 identifies a generator as "has a callable next and a callable
// throw"; this interface is that predicate, expressed in the type system.
//
// A non-nil error returned from Next or Throw means an exception escaped the
// generator body: the generator is finished and the error must be propagated,
// exactly as a `throw` out of gen.next() is in index.js:64-68.
// Close has no JavaScript counterpart — there an abandoned generator is simply
// collected. A Go body suspended in Yield owns a blocked goroutine, so Close is
// part of the interface: co calls it whenever it abandons a generator, and
// callers driving one by hand must call it too.
type Generator interface {
	Next(v any) (Result, error)
	Throw(err error) (Result, error)
	Close()
}

// GeneratorFunc is the canonical signature of a generator body.
//
// The returned value becomes the generator's completion value (the `return`
// value of a JavaScript generator); a non-nil error behaves like a `throw`
// escaping the body.
type GeneratorFunc func(y *Yielder) (any, error)

// resumeMsg travels from the driver into the suspended generator body.
type resumeMsg struct {
	value  any
	err    error
	closed bool
}

// stepMsg travels from the generator body back to the driver.
type stepMsg struct {
	value any
	done  bool
	err   error
}

// genClosed is the panic payload used to unwind an abandoned generator body.
type genClosed struct{}

// Yielder is the handle a generator body uses to suspend itself.
//
// It replaces JavaScript's `yield` keyword, which Go has no equivalent for.
// Each Yield hands a value to the driver and blocks until the driver resumes
// the body with a result or an error.
type Yielder struct {
	ctx  any
	args []any
	in   chan resumeMsg
	out  chan stepMsg
}

// Ctx returns the receiver the generator was started with.
//
// This is the port of JavaScript's `this` inside a `co`-driven generator
// (index.js:44, index.js:51). Go has no dynamic receiver, so it is threaded
// through explicitly.
func (y *Yielder) Ctx() any { return y.ctx }

// Args returns the extra arguments passed to co.Co / co.Wrap.
//
// Bodies declared with typed parameters receive them directly instead; Args is
// the escape hatch for bodies using the canonical GeneratorFunc signature.
func (y *Yielder) Args() []any { return y.args }

// Yield suspends the body, handing v to the driver.
//
// It returns the value the driver resumes with, or a non-nil error when the
// driver throws back in — the direct analogue of wrapping a `yield` in
// try/catch.
func (y *Yielder) Yield(v any) (any, error) {
	y.out <- stepMsg{value: v}
	r := <-y.in
	if r.closed {
		panic(genClosed{})
	}
	return r.value, r.err
}

// MustYield is Yield for bodies that do not want to handle the error locally.
// A thrown error is re-raised as a panic, which unwinds the body and surfaces
// as a rejection — matching an uncaught `yield` in JavaScript.
func (y *Yielder) MustYield(v any) any {
	res, err := y.Yield(v)
	if err != nil {
		Throw(err)
	}
	return res
}

// generator drives a GeneratorFunc running on its own goroutine.
//
// The goroutine plus the two unbuffered channels form a coroutine: exactly one
// of {driver, body} runs at any moment, so the body sees the same
// single-threaded execution a JavaScript generator does.
type generator struct {
	fn GeneratorFunc
	y  *Yielder

	mu      sync.Mutex
	started bool
	done    bool
}

// Gen creates a generator from a body.
func Gen(fn GeneratorFunc) Generator {
	return GenCtx(nil, fn)
}

// GenCtx creates a generator bound to a receiver and a set of arguments.
func GenCtx(ctx any, fn GeneratorFunc, args ...any) Generator {
	return &generator{
		fn: fn,
		y: &Yielder{
			ctx:  ctx,
			args: args,
			in:   make(chan resumeMsg),
			out:  make(chan stepMsg),
		},
	}
}

// Next resumes the body with v and runs it until the next Yield or completion.
//
// The first Next starts the body; its argument is discarded, matching
// JavaScript, where the value passed to the first next() has nothing to bind
// to (index.js:54 calls onFulfilled() with no argument).
func (g *generator) Next(v any) (Result, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.done {
		return Result{Done: true}, nil
	}

	if !g.started {
		g.started = true
		go g.run()
	} else {
		g.y.in <- resumeMsg{value: v}
	}

	return g.receive()
}

// Throw injects err at the generator's suspension point, mirroring gen.throw().
func (g *generator) Throw(err error) (Result, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Throwing into a generator that has finished — or has not started — simply
	// re-raises the error at the call site, as it does in JavaScript.
	if g.done || !g.started {
		g.done = true
		return Result{Done: true}, err
	}

	g.y.in <- resumeMsg{err: err}
	return g.receive()
}

// Close abandons a suspended generator so its goroutine can exit.
//
// This has no JavaScript counterpart (there, an abandoned generator is simply
// collected). In Go the body would otherwise block on its channel forever, so
// Close exists to keep long-lived programs leak-free.
func (g *generator) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.done {
		return
	}
	if !g.started {
		g.done = true
		return
	}

	g.y.in <- resumeMsg{closed: true}
	<-g.y.out
	g.done = true
}

// receive reads one step from the body. Callers must hold g.mu.
func (g *generator) receive() (Result, error) {
	s := <-g.y.out
	if s.done || s.err != nil {
		g.done = true
	}
	if s.err != nil {
		return Result{Done: true}, s.err
	}
	return Result{Value: s.value, Done: s.done}, nil
}

// run executes the body on its own goroutine and always emits exactly one
// terminal step, so the driver's send/receive pairs stay balanced.
func (g *generator) run() {
	bodyID := registerBodyGoroutine()
	defer releaseBodyGoroutine(bodyID)

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(genClosed); ok {
				g.y.out <- stepMsg{done: true}
				return
			}
			g.y.out <- stepMsg{done: true, err: asError(r)}
		}
	}()

	value, err := g.fn(g.y)
	g.y.out <- stepMsg{value: value, done: true, err: err}
}
