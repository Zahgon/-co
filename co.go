package co

// Default is an alias for Co, standing in for the JavaScript module's
// self-referential exports (`module.exports = co['default'] = co.co = co`,
// index.js:12) so that every import style has a Go counterpart.
var Default = Co

// Co runs a generator, generator function or plain function and returns a
// promise for its completion value. Extra arguments are forwarded to the
// function, mirroring `co(gen, ...args)`.
func Co(gen any, args ...any) *Promise {
	return CoCtx(nil, gen, args...)
}

// CoCtx is Co with an explicit receiver, the port of `co.call(ctx, gen)`.
//
// The receiver reaches generator bodies through Yielder.Ctx, thunks through the
// CtxThunk signature, and nested yieldables by propagation — exactly the reach
// `this` has in the original.
//
// This ports index.js:43-106 in full. Everything runs inside a single promise
// executor rather than a chain, preserving the original's fix for the memory
// leak reported in tj/co#180.
func CoCtx(ctx any, gen any, args ...any) *Promise {
	return New(func(resolve func(any), reject func(error)) {
		target := gen

		// `if (typeof gen === 'function') gen = gen.apply(ctx, args)`
		//
		// A *Wrapped is a callable in JavaScript, so co applies it and adopts
		// the promise it returns. It has to be matched explicitly here because
		// Go models it as a struct rather than a func.
		switch t := target.(type) {
		case *Wrapped:
			target = t.CallCtx(ctx, args...)
		default:
			switch {
			case isGeneratorLike(target):
				if _, live := target.(Generator); !live {
					target = newGeneratorFromValue(ctx, target, args)
				}
			case isFuncKind(target):
				target = applyPlain(ctx, target, args)
			}
		}

		// `if (!gen || typeof gen.next !== 'function') return resolve(gen)`
		if isFalsy(target) {
			resolve(target)
			return
		}
		iter, ok := target.(Generator)
		if !ok {
			resolve(target)
			return
		}

		drive(ctx, iter, resolve, reject)
	})
}

// drive is the trampoline: it steps the generator, converts each yielded value
// into a promise and feeds the settlement back in.
func drive(ctx any, iter Generator, resolve func(any), reject func(error)) {
	var onFulfilled func(any)
	var onRejected func(error)
	var next func(Result)

	// Settling the outer promise abandons the generator, so release it first.
	// This is always safe: every settle below happens after Next/Throw has
	// returned, meaning the body is suspended or finished, never running.
	// Close on a finished generator is a no-op.
	settleResolve := func(v any) {
		iter.Close()
		resolve(v)
	}
	settleReject := func(err error) {
		iter.Close()
		reject(err)
	}

	// index.js:62-71
	onFulfilled = func(res any) {
		ret, err := iter.Next(res)
		if err != nil {
			settleReject(err)
			return
		}
		next(ret)
	}

	// index.js:79-87
	onRejected = func(err error) {
		ret, thrownErr := iter.Throw(err)
		if thrownErr != nil {
			settleReject(thrownErr)
			return
		}
		next(ret)
	}

	// index.js:98-104
	next = func(ret Result) {
		if ret.Done {
			settleResolve(ret.Value)
			return
		}

		value := toPromise(ctx, ret.Value)
		if th, ok := asThenable(value); ok {
			th.Then(
				func(v any) any {
					guard(settleReject, func() { onFulfilled(v) })
					return nil
				},
				func(e error) any {
					guard(settleReject, func() { onRejected(e) })
					return nil
				},
			)
			return
		}

		// Note: the error is thrown back INTO the generator rather than
		// rejecting, so a generator can catch it and keep running.
		onRejected(newInvalidYieldError(ret.Value))
	}

	guard(settleReject, func() { onFulfilled(nil) })
}

// guard runs fn, turning an unexpected panic into a rejection of the outer
// promise instead of losing it inside a derived one.
func guard(reject func(error), fn func()) {
	defer func() {
		if r := recover(); r != nil {
			reject(asError(r))
		}
	}()
	fn()
}

// Wrapped is a generator function converted into a promise-returning function,
// the port of the closure returned by `co.wrap` (index.js:26-32).
type Wrapped struct {
	fn any
}

// Wrap converts a generator function into a regular function returning a
// promise.
func Wrap(fn any) *Wrapped {
	return &Wrapped{fn: fn}
}

// Call invokes the wrapped function with no receiver.
func (w *Wrapped) Call(args ...any) *Promise {
	return w.CallCtx(nil, args...)
}

// CallCtx invokes the wrapped function with an explicit receiver.
func (w *Wrapped) CallCtx(ctx any, args ...any) *Promise {
	return CoCtx(ctx, w.fn, args...)
}

// GeneratorFunction returns the wrapped function, the port of the
// `__generatorFunction__` property the original attaches to its closure.
func (w *Wrapped) GeneratorFunction() any {
	return w.fn
}
