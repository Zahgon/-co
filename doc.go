// Package co is a Go port of the Node.js library co@4.6.0
// (https://github.com/tj/co): generator-based control flow using promises.
//
// # Model
//
// co drives a coroutine. Each value the coroutine yields is converted into a
// promise; when that promise settles, the result is fed back in. Six kinds of
// value are yieldable:
//
//   - promises  (any co.Thenable)
//   - thunks    (co.Thunk, co.CtxThunk)
//   - slices    (resolved in parallel)
//   - objects   (co.Object or map[string]any, resolved in parallel)
//   - generators and generator functions (delegated to)
//
// Yieldables nest arbitrarily.
//
//	p := co.Co(func(y *co.Yielder) (any, error) {
//		res, err := y.Yield([]any{
//			co.Resolve(1),
//			co.Resolve(2),
//		})
//		if err != nil {
//			return nil, err
//		}
//		return res, nil // []any{1, 2}
//	})
//	value, err := p.Await()
//
// # Language mapping
//
// Three JavaScript features have no Go equivalent, so each is modelled
// explicitly:
//
// Generators. Go has no resumable functions, so a generator is a goroutine
// joined to its driver by two unbuffered channels. Exactly one side runs at a
// time, which reproduces the single-threaded execution a JavaScript generator
// sees. Yielder.Yield replaces the yield keyword.
//
// Promises. co.Promise implements the JavaScript semantics co depends on:
// settle-once, thenable adoption, and asynchronous delivery of reactions. All
// reactions run serially on one internal microtask goroutine, so ordering is
// deterministic and reactions never race each other. Never call Promise.Await
// from inside a reaction — it would block that goroutine.
//
// The receiver. JavaScript's dynamic `this` becomes an explicit ctx value
// threaded through co.CoCtx, Yielder.Ctx and co.CtxThunk. It is unrelated to
// context.Context and carries no cancellation.
//
// # Errors
//
// Rejections are Go errors. Where JavaScript would throw, a generator body
// returns a non-nil error or calls co.Throw; either way the enclosing promise
// rejects. Rejecting with a non-error value is possible via co.Reason, which
// wraps it in a *co.Rejection.
//
// See MIGRATION.md for a line-by-line mapping to index.js and the full list of
// deliberate deviations.
package co
