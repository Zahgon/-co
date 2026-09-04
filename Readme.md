# co

Generator-based control flow for Go — a functionally equivalent port of
[tj/co](https://github.com/tj/co) v4.6.0.

`co` lets you write asynchronous code that reads sequentially. You yield
"yieldables" — promises, thunks, generators, arrays, objects — and get their
results back as plain values. The library resolves them and resumes you.

```go
package main

import (
	"fmt"
	"time"

	co "github.com/tj/co-go"
)

func main() {
	p := co.Co(func(y *co.Yielder) (any, error) {
		result, err := y.Yield(co.Thunk(func(done func(error, ...any)) {
			time.AfterFunc(10*time.Millisecond, func() { done(nil, "hello") })
		}))
		if err != nil {
			return nil, err
		}
		return result.(string) + " world", nil
	})

	value, err := p.Await()
	fmt.Println(value, err) // hello world <nil>
}
```

## Installation

```sh
go get github.com/tj/co-go
```

## API

### `co.Co(gen any, args ...any) *Promise`

Runs `gen` and returns a promise for its return value. `gen` may be a generator
function, a live generator, or a plain function. Extra arguments are forwarded.
Equivalent to `co(gen, ...args)`.

### `co.CoCtx(ctx any, gen any, args ...any) *Promise`

Same, with an explicit receiver. `ctx` is what `y.Ctx()` returns inside the body
and what context-aware thunks receive. Equivalent to `co.call(this, gen, ...)`.

### `co.Wrap(fn any) *Wrapped`

Converts a generator function into a reusable, callable unit.

```go
run := co.Wrap(func(y *co.Yielder, id int) (any, error) {
	return y.Yield(fetch(id))
})

value, err := run.Call(42).Await()
```

`Wrapped.Call(args...)`, `Wrapped.CallCtx(ctx, args...)` and
`Wrapped.GeneratorFunction()` port `fn(...)`, `fn.call(this, ...)` and
`fn.__generatorFunction__`.

### `co.Gen(fn) / co.GenCtx(ctx, fn, args...)`

Creates a live generator you can drive yourself with `Next` / `Throw`, or yield
into another `co` body.

## Yieldables

| Kind | Go type | Behaviour |
| --- | --- | --- |
| Promise | `*co.Promise`, any `co.Thenable` | awaited |
| Thunk | `co.Thunk`, `co.CtxThunk` | called with a completion callback |
| Generator function | `func(*co.Yielder) (any, error)` and typed variants | run with `co` |
| Generator | `co.Generator` | run with `co` |
| Wrapped | `*co.Wrapped` | called, its promise adopted |
| Array | `[]any` and any slice/array except `[]byte` | resolved in parallel, order preserved |
| Object | `*co.Object`, any string-keyed map | values resolved in parallel, keys preserved |

`[]byte` is excluded on purpose: the original rejects a `Buffer`, and silently
resolving one element-wise would turn an error into garbage. Yield a `[]any` if
that is what you meant.

Anything else raises a `*co.TypeError` **inside the generator**, so it is
recoverable:

```go
_, err := y.Yield(42)
// err: You may only yield a function, promise, generator, array, or object,
//      but the following object was passed: "42"
```

### Thunks

A thunk is a function taking a completion callback:

```go
func read(name string) co.Thunk {
	return func(done func(err error, res ...any)) {
		data, err := os.ReadFile(name)
		done(err, string(data))
	}
}
```

Result arity matches the original: no results yield `nil`, one result yields the
value, two or more yield a `[]any`.

### Arrays and objects

```go
res, err := y.Yield([]any{read("a.txt"), read("b.txt")})
// res.([]any) -> ["...", "..."]

obj, err := y.Yield(co.ObjectOf(
	"a", read("a.txt"),
	"b", read("b.txt"),
))
// obj.(*co.Object) -> {a: "...", b: "..."} with key order preserved
```

Both nest arbitrarily.

## Errors

Yield returns `(value, error)`. Handle it, or ignore it and return it — that is
the port of `try/catch` around `yield`:

```go
value, err := y.Yield(mightFail())
if err != nil {
	return "fallback", nil // recovered, the generator continues
}
```

`y.MustYield(v)` throws instead of returning an error, ending the generator and
rejecting the promise.

## Promises

`co.Promise` follows the Promises/A+ state machine: single settlement, thenable
adoption, and reactions delivered serially on a microtask queue.

```go
p.Then(onFulfilled, onRejected)
p.Catch(onRejected)
p.Finally(fn)
p.Await()          // (any, error) — blocks
<-p.Done()         // settle signal
p.State()          // Pending | Fulfilled | Rejected
```

**Never call `Await` from inside a generator body or a `Then` / `Catch` /
`Finally` handler.** Both run on goroutines the queue depends on, so blocking
one stalls every promise in the program. `Await` detects this and panics with an
explanation rather than hanging silently; yield the promise with `y.Yield(p)`,
or return it from the handler and let it be adopted. Awaiting a promise that has
already settled is always fine.

## Differences from the JavaScript original

Everything observable is preserved. These are the mechanical consequences of the
target language and are documented in full in [MIGRATION.md](MIGRATION.md):

- Generators are goroutines. `co` closes the ones it abandons, but a generator
  you drive yourself and drop while suspended holds its goroutine until you call
  `Close()`.
- `this` becomes an explicit `ctx any` parameter. It is unrelated to
  `context.Context`.
- Rejection reasons are `error`. A non-error reason travels inside
  `*co.Rejection`, reachable with `errors.As`.
- `*co.Object` provides the ordered string-keyed map that JavaScript objects
  give for free.

One behaviour is deliberately *not* reproduced: `yield co.wrap(fn)` hangs
forever in the original, because the wrapper is mistaken for a thunk. Here it is
called and its promise adopted.

## Tests

```sh
go test -race ./...
```

The suite is a case-for-case port of the original Mocha suite — one top-level
`Test` function per `it()` — plus tests pinning the semantics that Go could
otherwise drift on. The runnable examples in `example_test.go` are checked
against their `// Output:` comments by the same command.

## License

MIT — see [LICENSE](LICENSE). Original work copyright (c) 2014 TJ Holowaychuk.
