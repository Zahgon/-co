# Migration notes: tj/co (JavaScript) → co (Go)

This records how each construct in the 239-line `index.js` was carried across,
and where the target language forced a decision.

## Source → target file map

| JavaScript | Go | Contents |
| --- | --- | --- |
| `index.js:12` | `co.go` | `module.exports = co['default'] = co.co = co` → `Co` + `Default` |
| `index.js:26-32` | `co.go` | `co.wrap` → `Wrap` / `Wrapped` |
| `index.js:43-106` | `co.go` | the trampoline → `CoCtx` + `drive` |
| `index.js:116-124` | `yieldable.go` | `toPromise` |
| `index.js:134-143` | `thunk.go` | `thunkToPromise` |
| `index.js:154-156` | `yieldable.go` | `arrayToPromise` |
| `index.js:167-188` | `yieldable.go` | `objectToPromise` |
| `index.js:198-239` | `yieldable.go`, `apply.go` | `isPromise`, `isGenerator`, `isGeneratorFunction`, `isObject` |
| host `Promise` | `promise.go`, `microtask.go` | Promises/A+ state machine + microtask queue |
| host generators | `generator.go` | goroutine-backed coroutine |
| host objects | `object.go` | insertion-ordered string map |
| host `String()`, `TypeError` | `errors.go` | coercion + error types |

## The four decisions

### 1. Generators → goroutine coroutines

JavaScript generators suspend a call frame. Go has no such primitive, so a
generator body runs on its own goroutine joined to the driver by two
**unbuffered** channels. Unbuffered is the point: exactly one side runs at a
time, so the body observes the same strictly interleaved, single-threaded
execution it had in V8.

`Yield` sends a value out and blocks for the resumption; `Next` and `Throw` send
a resumption in and block for the next step. `run` always emits exactly one
terminal step, so sends and receives stay balanced and neither side can wedge.

**Consequence.** An abandoned generator is a blocked goroutine, whereas an
abandoned JavaScript generator is simply collected. Three cases:

- a generator that runs to completion or throws releases its goroutine;
- a generator `co` abandons — because the trampoline settled early, or because a
  reaction panicked — is closed by `drive` before it resolves or rejects, so it
  releases too;
- a generator you drive yourself and then drop while it is suspended keeps its
  goroutine until you call `Generator.Close()`.

`Close` unwinds a suspended body by raising an internal sentinel panic through
the `Yield` point, and is a no-op on a finished one. There is no equivalent in
the original because none is needed there.

### 2. `this` → an explicit `ctx any`

`co.call(this, gen)` threads a receiver through the trampoline into thunks and
nested generators. Go has no dynamic receiver, so `ctx any` becomes a real
parameter: `CoCtx(ctx, ...)`, `y.Ctx()`, `CtxThunk`.

`Co(...)` is `CoCtx(nil, ...)`, matching a call with no receiver.

**This is not `context.Context`.** It is an opaque value; cancellation and
deadlines are not part of the original's semantics and are not added here.

### 3. Rejection reasons → `error`

JavaScript can reject with any value; Go rejections are `error`. Non-error
reasons travel inside `*co.Rejection`:

```go
err := mustReject(co.Co(func() any { return co.Reject(co.Reason(1)) }))

var r *co.Rejection
errors.As(err, &r) // r.Value == 1
```

`panic(err)` inside a generator body or a thunk is the port of `throw`; it is
recovered and converted to a rejection. `co.Throw(err)` is the explicit form.

### 4. JavaScript objects → `*co.Object`

`objectToPromise` depends on `Object.keys` order, and Go maps are unordered and
deliberately randomised. `*co.Object` stores keys alongside values and reports
them in JavaScript's order: canonical array indices ascending, then the rest in
insertion order.

Any map with a string key kind is also accepted — `map[string]any`,
`map[string]int`, a named string type — and always returns a `map[string]any`,
because `index.js:168` builds `new obj.constructor()` and `isObject`
(`index.js:238`) only admits plain objects, so the result in JavaScript is
always a plain `{}` too. The key order inside `toPromise` is made deterministic
(indices numerically, then the rest lexicographically) so behaviour is
reproducible even though the map itself cannot carry order.

## Semantics preserved exactly

These were the traps, and each has a test.

**Everything happens inside one promise executor.** `index.js:43` wraps the
whole trampoline in a single `new Promise`, deliberately, to avoid the chaining
memory leak of tj/co#180. `CoCtx` does the same, and `drive`'s reactions are
wrapped in `guard` so an unexpected panic rejects the *outer* promise rather
than vanishing into a derived one.

**An invalid yield is thrown back into the generator.** `index.js:104` calls
`onRejected(new TypeError(...))`, not `reject(...)`. The generator can catch it
and continue; two consecutive invalid yields produce two recoverable errors.
Ported verbatim, message included:

```
You may only yield a function, promise, generator, array, or object, but the following object was passed: "<value>"
```

`errors.go` reproduces JavaScript's `String()` coercion so the message is
byte-identical: `null`, `NaN`, `Infinity`, `[object Object]`, comma-joined
arrays, `Error: <message>` for errors, and — the part that is easy to get wrong
— `Number::toString` (ECMA-262 6.1.6.1.20) rather than `strconv.FormatFloat`.
The two disagree across a wide band: `1000000` is `1e+06` to Go and `1000000` to
V8, `1e20` is `1e+20` to Go and `100000000000000000000` to V8, `1e-7` is `1e-07`
to Go and `1e-7` to V8. `jsNumber` implements the spec's five layouts from the
shortest round-tripping digit string.

`TestInvalidYieldMessageCoercion` pins the message and
`TestJSNumberMatchesV8` pins the numbers against values captured from Node. Both
were also checked by differential fuzz: 20,000 random `float64`s formatted by
`jsNumber` and by V8's `String()`, zero mismatches.

**`toPromise` dispatch order.** falsy → promise → generator/generator function →
`*Wrapped` → function (thunk) → array → object → passthrough. Order matters: a
generator function is also a function, so it must be tested first. `*Wrapped` is
the one insertion, and it sits where JavaScript would have found a plain
function; see *`co.wrap` is callable everywhere* below.

**Falsy values are not converted.** `index.js:117` returns falsy values
untouched, and `index.js:99` then rejects them as invalid. `isFalsy` mirrors JS
truthiness — but nil maps and nil slices are deliberately **not** falsy, since
`[]` and `{}` are truthy objects in JavaScript and must resolve to empty
results.

**Thunk result arity.** `index.js:141` slices `arguments` when
`arguments.length > 2`. So: no results → `nil`, one → the value, two or more →
`[]any`. `done(nil, "stdout", "stderr")` yields `["stdout", "stderr"]`.

**First settlement wins.** A thunk that calls `done(err)` and then throws
rejects with the first error. Guaranteed by `Promise.locked`.

**Object key order survives.** `index.js:181-187` pre-defines `results[key]`
before awaiting, which is what holds the slot. `objectToPromise` writes a nil
placeholder for the same reason, and `Object.Set` preserves a key's original
position when overwritten.

**The first `next` receives nothing.** `index.js:60` calls `onFulfilled()` with
no argument. `Generator.Next` discards its argument on the first call.

**`co.wrap` exposes the source function.** `__generatorFunction__` →
`Wrapped.GeneratorFunction()`.

**`co(co.wrap(fn))` works.** In JavaScript `co.wrap(fn)` is an ordinary
function, so `co` applies it and adopts the promise it returns. `*Wrapped` is a
struct and would otherwise have missed every branch and been resolved as a plain
value — a silently wrong result. `CoCtx` calls it explicitly instead.

**A `[]byte` is not an array.** JavaScript rejects a `Buffer` with the invalid
yield `TypeError`, because a Buffer is neither `Array.isArray` nor
`Object == val.constructor`. Go's `asSlice` would happily have converted one
element-wise, turning a loud error into quiet garbage, so slices and arrays of
`uint8` are excluded and fall through to the same `TypeError`. To resolve binary
data element-wise on purpose, yield a `[]any`.

## Deliberate divergences

Two, both narrow, both improvements on behaviour the original does not defend.

**`co.wrap` is callable everywhere.** `yield co.wrap(fn)` **hangs forever** in
upstream co: the wrapper is a plain function, so `toPromise` classifies it as a
thunk, hands it the completion callback as its first argument, and the callback
is never invoked. Verified against the real `index.js` under Node. The port
treats a yielded `*Wrapped` as what it is — a generator function bound to a
receiver — calls it, and adopts its promise. Reproducing a deadlock is not
fidelity worth having.

**A panicking thenable rejects.** Promises/A+ 2.3.3.2 requires that a `then`
that throws while being retrieved or called rejects the adopting promise.
`Promise.resolve` recovers around adoption and settles as rejected, unless the
thenable already settled it first (2.3.3.3.4).

## Signature mapping

| JavaScript | Go |
| --- | --- |
| `co(gen)` | `co.Co(gen)` |
| `co(gen, a, b)` | `co.Co(gen, a, b)` |
| `co.call(ctx, gen)` | `co.CoCtx(ctx, gen)` |
| `co.wrap(fn)` | `co.Wrap(fn)` |
| `co.wrap(fn)(a, b)` | `co.Wrap(fn).Call(a, b)` |
| `co(co.wrap(fn), a)` | `co.Co(co.Wrap(fn), a)` |
| `fn.__generatorFunction__` | `wrapped.GeneratorFunction()` |
| `function *(){ }` | `func(y *co.Yielder) (any, error)` |
| `yield v` | `y.Yield(v)` → `(any, error)` |
| `yield v` (uncaught) | `y.MustYield(v)` |
| `try { yield v } catch (e) {}` | `v, err := y.Yield(v); if err != nil {}` |
| `throw e` | `return nil, e` or `co.Throw(e)` |
| `return v` | `return v, nil` |
| `p.then(f, r)` | `p.Then(f, r)` |
| `p.catch(r)` | `p.Catch(r)` |
| `await p` | `p.Await()` → `(any, error)` |
| `{ a: x }` | `co.ObjectOf("a", x)` or `map[string]any{"a": x}` |
| `[x, y]` | `[]any{x, y}` |
| `function (done) {}` | `co.Thunk(func(done func(error, ...any)) {})` |

## Test suite

All 43 cases from the 11 Mocha files are ported one-for-one. Each `it()` becomes
its own top-level `func TestXxx(t *testing.T)` rather than a `t.Run` subtest, so
that the case inventory of the two suites lines up one-to-one and every case is
independently addressable with `go test -run`.

Mocha gets its uniqueness from the enclosing `describe`, so several titles repeat
verbatim — `should work` appears six times and `should throw` twice. Go has no
enclosing scope to borrow, and the names still have to differ, so the duplicates
are distinguished with words that carry no meaning of their own: `TestShouldWork`,
`TestItWorks`, `TestWorks`, `TestItShouldWork`, `TestCanWork`, `TestDoesWork`.
Each of those carries a doc comment naming the exact `describe > it` it came
from, because the function name alone can no longer say it.

The three fixture files the originals read through `mz/fs` are remapped to files
that exist in this module:

| Original | Needle | Port | Needle |
| --- | --- | --- | --- |
| `index.js` | `exports` | `co.go` | `package co` |
| `LICENSE` | `MIT` | `LICENSE` | `MIT` |
| `package.json` | `devDependencies` | `go.mod` | `module` |

Two originals — `test/generators.js` and `test/generator-functions.js` — are
byte-identical and both yield the generator *function*. The port differentiates
them: `generators_test.go` yields live generators and
`generator_functions_test.go` yields generator functions, covering both branches
of the dispatch that the originals left half-tested.

`semantics_test.go` adds coverage for what Go could drift on and JavaScript
could not: `String()` coercion of the error message, `Object.keys` ordering,
single settlement, thenable adoption, goroutine accounting under repeated runs,
`Generator.Close`, terminal-state generator behaviour, and 64-way concurrent
trampolines under `-race`.

`regression_test.go` pins each divergence that a differential audit against Node
turned up: `*Wrapped` as an argument and as a yieldable, `[]byte` rejected,
typed maps accepted, a panicking thenable rejecting, 200 abandoned generators
leaving no goroutines behind, and the `Await` guard. `errors_internal_test.go`
pins `jsNumber` and `jsString` against values captured from V8.

The Readme's examples live in `example_test.go` as `Example`, `ExampleWrap` and
`ExampleObject`. Go runs those under `go test` and compares their stdout against
the `// Output:` comment, so a documented example that stops being true fails the
build.

## Deliberate non-goals

- No `context.Context` cancellation. Not in the original.
- No generics. The original is untyped; `any` is the faithful mapping, and
  generic wrappers would constrain the heterogeneous arrays and objects the
  tests require.
- No dependencies. The original has none at runtime.
