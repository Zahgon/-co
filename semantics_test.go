package co_test

import (
	"errors"
	"math"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	co "github.com/tj/co-go"
)

// TestInvalidYieldMessageCoercion pins the String() coercion used to build the
// invalid-yield TypeError, the one user-visible string this port must not drift
// on.
func TestInvalidYieldMessageCoercion(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		wantStr string
	}{
		{"nil", nil, "null"},
		{"false", false, "false"},
		{"zero int", 0, "0"},
		{"empty string", "", ""},
		{"NaN", math.NaN(), "NaN"},
		{"non-empty string", "something", "something"},
		{"int", 42, "42"},
		{"float", 1.5, "1.5"},
		{"true", true, "true"},
		{"infinity", math.Inf(1), "Infinity"},
		{"negative infinity", math.Inf(-1), "-Infinity"},

		// ECMAScript switches to exponential notation only outside
		// 1e-7..1e21, and never zero-pads the exponent. Go's %g does neither,
		// so this band is where a naive port drifts.
		{"one million", 1e6, "1000000"},
		{"nine digits", 123456789.0, "123456789"},
		{"1e20", 1e20, "100000000000000000000"},
		{"1e21", 1e21, "1e+21"},
		{"1e-6", 1e-6, "0.000001"},
		{"1e-7", 1e-7, "1e-7"},
		{"tenth", 0.1, "0.1"},
		{"fraction", 123.456, "123.456"},
		{"negative", -1.5, "-1.5"},
		{"negative zero", math.Copysign(0, -1), "0"},
		{"smallest denormal", 5e-324, "5e-324"},
		{"max float", math.MaxFloat64, "1.7976931348623157e+308"},

		{"error", errors.New("boom"), "Error: boom"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := tc.value
			err := mustReject(t, co.Co(func(y *co.Yielder) (any, error) {
				_, err := y.Yield(value)
				return nil, err
			}))

			want := `You may only yield a function, promise, generator, array, or object, ` +
				`but the following object was passed: "` + tc.wantStr + `"`
			if err.Error() != want {
				t.Errorf("err  = %q\nwant = %q", err.Error(), want)
			}
			if _, ok := co.AsTypeError(err); !ok {
				t.Errorf("err is %T, want *co.TypeError", err)
			}
		})
	}
}

// TestObjectKeyOrder pins JavaScript's Object.keys() ordering rule: canonical
// array indices ascending, then the remaining keys in insertion order.
func TestObjectKeyOrder(t *testing.T) {
	obj := co.ObjectOf(
		"zebra", 1,
		"2", 2,
		"apple", 3,
		"0", 4,
		"10", 5,
		"01", 6,
	)

	got := obj.Keys()
	want := []string{"0", "2", "10", "zebra", "apple", "01"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
}

// TestObjectSetPreservesFirstPosition checks that overwriting a key keeps its
// original slot, which is what lets a pre-defined placeholder hold position.
func TestObjectSetPreservesFirstPosition(t *testing.T) {
	obj := co.ObjectOf("a", 1, "b", 2, "c", 3)
	obj.Set("a", 99)

	if got, want := obj.Keys(), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
	if v, _ := obj.Get("a"); v != 99 {
		t.Errorf("a = %v, want 99", v)
	}
}

// TestPromiseSettlesOnce covers the guarantee the thunk contract leans on: the
// first settlement wins and later ones are ignored.
func TestPromiseSettlesOnce(t *testing.T) {
	p := co.New(func(resolve func(any), reject func(error)) {
		resolve(1)
		resolve(2)
		reject(errors.New("late"))
	})

	if got := mustResolve(t, p); got != 1 {
		t.Errorf("got = %v, want 1", got)
	}
}

// TestPromiseAdoptsThenable covers thenable adoption, which is what makes
// co(fn) resolve through a returned promise.
func TestPromiseAdoptsThenable(t *testing.T) {
	t.Run("fulfilled", func(t *testing.T) {
		if got := mustResolve(t, co.Resolve(co.Resolve(7))); got != 7 {
			t.Errorf("got = %v, want 7", got)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		err := mustReject(t, co.Resolve(co.Reject(errBoom)))
		if err.Error() != "boom" {
			t.Errorf("err = %v, want boom", err)
		}
	})
}

// TestAllFailsFast checks that a rejected entry rejects the aggregate.
func TestAllFailsFast(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		_, err := y.Yield([]any{
			co.Resolve(1),
			co.Reject(errBoom),
			co.Resolve(3),
		})
		return nil, err
	}

	if err := mustReject(t, co.Co(body)); err.Error() != "boom" {
		t.Errorf("err = %v, want boom", err)
	}
}

// TestThrowFromGeneratorBody covers co.Throw as the port of an uncaught throw.
func TestThrowFromGeneratorBody(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		y.MustYield(get(1, errBoom, nil))
		t.Error("MustYield returned instead of throwing")
		return nil, nil
	}

	if err := mustReject(t, co.Co(body)); err.Error() != "boom" {
		t.Errorf("err = %v, want boom", err)
	}
}

// TestConcurrentCo runs many independent trampolines at once, checking that the
// shared microtask queue keeps them isolated and race-free.
func TestConcurrentCo(t *testing.T) {
	const workers = 64

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()

			body := func(y *co.Yielder) (any, error) {
				res, err := y.Yield([]any{
					get(n, nil, nil),
					co.Resolve(n),
					co.ObjectOf("v", get(n, nil, nil)),
				})
				if err != nil {
					return nil, err
				}
				return res, nil
			}

			raw, err := co.Co(body).Await()
			if err != nil {
				t.Errorf("worker %d: %v", n, err)
				return
			}

			res := raw.([]any)
			if res[0] != n || res[1] != n {
				t.Errorf("worker %d: got %v", n, res)
			}
			if v, _ := res[2].(*co.Object).Get("v"); v != n {
				t.Errorf("worker %d: object value = %v", n, v)
			}
		}(i)
	}

	wg.Wait()
}

// TestNoGoroutineLeak checks that completed trampolines leave no generator
// goroutines behind. Generators suspended on a promise that never settles are
// excluded: they are unreachable in JavaScript too, and co.Generator exposes
// Close for that case.
func TestNoGoroutineLeak(t *testing.T) {
	run := func() {
		body := func(y *co.Yielder) (any, error) {
			if _, err := y.Yield(get(1, nil, nil)); err != nil {
				return nil, err
			}
			if _, err := y.Yield([]any{co.Resolve(1), work}); err != nil {
				return nil, err
			}
			_, err := y.Yield(get(1, errBoom, nil))
			if err == nil {
				t.Error("expected a rejection")
			}
			return nil, nil
		}
		mustResolve(t, co.Co(body))
	}

	run() // warm up the scheduler and any lazily started goroutines
	settle()
	before := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		run()
	}
	settle()

	if after := runtime.NumGoroutine(); after > before+2 {
		t.Errorf("goroutines grew from %d to %d", before, after)
	}
}

// settle gives finished goroutines a moment to exit before they are counted.
func settle() {
	for i := 0; i < 20; i++ {
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)
	}
}

// TestGeneratorCloseReleasesSuspendedBody checks the Go-only escape hatch for
// abandoning a suspended generator.
func TestGeneratorCloseReleasesSuspendedBody(t *testing.T) {
	settle()
	before := runtime.NumGoroutine()

	gen := co.Gen(func(y *co.Yielder) (any, error) {
		_, err := y.Yield(co.New(func(func(any), func(error)) {}))
		return nil, err
	})

	if _, err := gen.Next(nil); err != nil {
		t.Fatalf("Next: %v", err)
	}

	gen.Close()

	settle()
	if after := runtime.NumGoroutine(); after > before+1 {
		t.Errorf("goroutines grew from %d to %d after Close", before, after)
	}
}

// TestGeneratorAfterDone checks the terminal-state rules: further Next calls
// report done, and Throw re-raises.
func TestGeneratorAfterDone(t *testing.T) {
	gen := co.Gen(func(*co.Yielder) (any, error) { return "value", nil })

	res, err := gen.Next(nil)
	if err != nil || !res.Done || res.Value != "value" {
		t.Fatalf("first Next = %+v, %v", res, err)
	}

	res, err = gen.Next(nil)
	if err != nil || !res.Done || res.Value != nil {
		t.Errorf("second Next = %+v, %v", res, err)
	}

	if _, err := gen.Throw(errBoom); err == nil || err.Error() != "boom" {
		t.Errorf("Throw = %v, want boom", err)
	}
}

// TestNonThunkFunctionYieldsTypeError checks that a function whose shape is not
// a thunk falls through to the invalid-yield error rather than being called.
func TestNonThunkFunctionYieldsTypeError(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		_, err := y.Yield(func(int, string) bool { return true })
		return nil, err
	}

	err := mustReject(t, co.Co(body))
	if !strings.Contains(err.Error(), "You may only yield") {
		t.Errorf("err = %q, want the invalid-yield TypeError", err)
	}
}

// TestFalsyYieldsAreRejected covers the falsy passthrough in toPromise: every
// falsy value reaches the TypeError instead of being converted.
func TestFalsyYieldsAreRejected(t *testing.T) {
	for _, value := range []any{nil, false, 0, "", math.NaN()} {
		v := value
		err := mustReject(t, co.Co(func(y *co.Yielder) (any, error) {
			_, err := y.Yield(v)
			return nil, err
		}))
		if _, ok := co.AsTypeError(err); !ok {
			t.Errorf("yield %#v: err is %T, want *co.TypeError", v, err)
		}
	}
}

// TestEmptyYieldablesResolveEmpty checks that an empty array and an empty
// object are truthy and resolve to empty results.
func TestEmptyYieldablesResolveEmpty(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		arr, err := y.Yield([]any{})
		if err != nil {
			return nil, err
		}
		if len(arr.([]any)) != 0 {
			t.Errorf("array = %#v, want empty", arr)
		}

		obj, err := y.Yield(co.NewObject())
		if err != nil {
			return nil, err
		}
		if obj.(*co.Object).Len() != 0 {
			t.Errorf("object = %#v, want empty", obj)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}
