package co_test

import (
	"runtime"
	"testing"

	co "github.com/tj/co-go"
)

// The tests below pin behaviours that a differential audit against Node found
// diverging from tj/co. Each names the JavaScript expression it mirrors.

// co(co.wrap(fn)) resolves with the generator's return value, because in
// JavaScript co.wrap returns an ordinary function that co applies.
func TestCoAcceptsWrapped(t *testing.T) {
	wrapped := co.Wrap(func(y *co.Yielder) (any, error) {
		return 42, nil
	})

	if got := mustResolve(t, co.Co(wrapped)); got != 42 {
		t.Errorf("Co(Wrap(fn)) = %#v, want 42", got)
	}
}

func TestCoAcceptsWrappedWithArgs(t *testing.T) {
	wrapped := co.Wrap(func(y *co.Yielder) (any, error) {
		args := y.Args()
		return args[0].(int) + args[1].(int), nil
	})

	if got := mustResolve(t, co.Co(wrapped, 1, 2)); got != 3 {
		t.Errorf("Co(Wrap(fn), 1, 2) = %#v, want 3", got)
	}
}

func TestCoPropagatesWrappedRejection(t *testing.T) {
	wrapped := co.Wrap(func(y *co.Yielder) (any, error) {
		return nil, errBoom
	})

	if err := mustReject(t, co.Co(wrapped)); err != errBoom {
		t.Errorf("err = %v, want %v", err, errBoom)
	}
}

func TestYieldWrapped(t *testing.T) {
	inner := co.Wrap(func(y *co.Yielder) (any, error) {
		return "hello", nil
	})

	got := mustResolve(t, co.Co(func(y *co.Yielder) (any, error) {
		return y.Yield(inner)
	}))

	if got != "hello" {
		t.Errorf("yield Wrap(fn) = %#v, want \"hello\"", got)
	}
}

// A []byte is the Go analogue of a Buffer, which Array.isArray rejects, so it
// must raise the invalid-yield TypeError rather than resolve element-wise.
func TestYieldByteSliceIsInvalid(t *testing.T) {
	err := mustReject(t, co.Co(func(y *co.Yielder) (any, error) {
		_, err := y.Yield([]byte("hi"))
		return nil, err
	}))

	if _, ok := co.AsTypeError(err); !ok {
		t.Fatalf("err is %T, want *co.TypeError", err)
	}
}

// Object literals are yieldable regardless of their value type, matching
// JavaScript, where {a: 1} carries no type information at all.
func TestYieldTypedMap(t *testing.T) {
	got := mustResolve(t, co.Co(func(y *co.Yielder) (any, error) {
		return y.Yield(map[string]int{"a": 1, "b": 2})
	}))

	out, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("result is %T, want map[string]any", got)
	}
	if out["a"] != 1 || out["b"] != 2 {
		t.Errorf("result = %#v, want a:1 b:2", out)
	}
}

func TestYieldTypedMapResolvesValues(t *testing.T) {
	got := mustResolve(t, co.Co(func(y *co.Yielder) (any, error) {
		return y.Yield(map[string]co.Thunk{"greeting": get("hi", nil, nil)})
	}))

	out, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("result is %T, want map[string]any", got)
	}
	if out["greeting"] != "hi" {
		t.Errorf("result = %#v, want greeting:hi", out)
	}
}

// Promises/A+ 2.3.3.2: a thenable whose then throws rejects the promise
// adopting it, instead of leaving it pending forever.
func TestPanickingThenableRejects(t *testing.T) {
	if err := mustReject(t, co.Resolve(explodingThenable{})); err == nil {
		t.Error("want a rejection")
	}
}

func TestYieldPanickingThenableRejects(t *testing.T) {
	err := mustReject(t, co.Co(func(y *co.Yielder) (any, error) {
		_, err := y.Yield(co.Resolve(explodingThenable{}))
		return nil, err
	}))

	if err == nil {
		t.Error("want a rejection")
	}
}

type explodingThenable struct{}

func (explodingThenable) Then(func(any) any, func(error) any) *co.Promise {
	panic(errBoom)
}

// co abandons a generator whenever it settles its promise, so a body suspended
// on a yield that can never resume must not strand its goroutine.
func TestAbandonedGeneratorsDoNotLeak(t *testing.T) {
	settle()
	before := runtime.NumGoroutine()

	for i := 0; i < 200; i++ {
		gen := co.Gen(func(y *co.Yielder) (any, error) {
			_, err := y.Yield(co.New(func(func(any), func(error)) {}))
			return nil, err
		})
		if _, err := gen.Next(nil); err != nil {
			t.Fatalf("Next: %v", err)
		}
		gen.Close()
	}

	settle()
	if after := runtime.NumGoroutine(); after > before+5 {
		t.Errorf("goroutines grew from %d to %d over 200 generators", before, after)
	}
}

// A panic raised while co is processing a yield settles the outer promise, and
// must release the generator that is still suspended behind it.
func TestPanicDuringYieldReleasesGenerator(t *testing.T) {
	settle()
	before := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		mustReject(t, co.Co(func(y *co.Yielder) (any, error) {
			_, err := y.Yield(explodingThenable{})
			return nil, err
		}))
	}

	settle()
	if after := runtime.NumGoroutine(); after > before+5 {
		t.Errorf("goroutines grew from %d to %d over 50 generators", before, after)
	}
}

// Await on a live promise from a generator body would hang the scheduler with
// no panic and no deadlock-detector trip, so it is refused loudly instead.
func TestAwaitFromGeneratorBodyPanics(t *testing.T) {
	err := mustReject(t, co.Co(func(y *co.Yielder) (any, error) {
		co.New(func(func(any), func(error)) {}).Await()
		return nil, nil
	}))

	if err == nil {
		t.Fatal("want a rejection")
	}
}

func TestAwaitFromReactionPanics(t *testing.T) {
	err := mustReject(t, co.Resolve(1).Then(func(any) any {
		co.New(func(func(any), func(error)) {}).Await()
		return nil
	}, nil))

	if err == nil {
		t.Fatal("want a rejection")
	}
}

// The guard must not fire when nothing would block: a settled promise is
// readable from any goroutine, including the ones the scheduler owns.
func TestAwaitSettledPromiseFromGeneratorBodyIsAllowed(t *testing.T) {
	got := mustResolve(t, co.Co(func(y *co.Yielder) (any, error) {
		if _, err := y.Yield(get("tick", nil, nil)); err != nil {
			return nil, err
		}
		return co.Resolve("done").Await()
	}))

	if got != "done" {
		t.Errorf("result = %#v, want \"done\"", got)
	}
}

// String(Buffer.from('hi')) is "hi", not the char codes, so the invalid-yield
// message for a byte slice must carry the text.
func TestByteSliceInvalidYieldMessage(t *testing.T) {
	err := mustReject(t, co.Co(func(y *co.Yielder) (any, error) {
		_, err := y.Yield([]byte("hi"))
		return nil, err
	}))

	want := `You may only yield a function, promise, generator, array, or object, ` +
		`but the following object was passed: "hi"`
	if err.Error() != want {
		t.Errorf("err  = %q\nwant = %q", err.Error(), want)
	}
}
