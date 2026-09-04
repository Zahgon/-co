package co_test

import (
	"reflect"
	"testing"

	co "github.com/tj/co-go"
)

// Port of test/wrap.js — co.wrap(fn*) > should pass context
//
// The name carries the extra "wrapped" token so it stays distinct from
// test/context.js, whose title collapses to the same tokens.
func TestShouldPassWrappedContext(t *testing.T) {
	ctx := &receiver{some: "thing"}

	wrapped := co.Wrap(func(y *co.Yielder) (any, error) {
		if y.Ctx() != any(ctx) {
			t.Errorf("y.Ctx() = %#v, want %#v", y.Ctx(), ctx)
		}
		return nil, nil
	})

	mustResolve(t, wrapped.CallCtx(ctx))
}

// Port of test/wrap.js — co.wrap(fn*) > should pass arguments
func TestShouldPassArguments(t *testing.T) {
	wrapped := co.Wrap(func(y *co.Yielder, a, b, c int) (any, error) {
		got := []int{a, b, c}
		if !reflect.DeepEqual(got, []int{1, 2, 3}) {
			t.Errorf("got = %v, want [1 2 3]", got)
		}
		return nil, nil
	})

	mustResolve(t, wrapped.Call(1, 2, 3))
}

// Port of test/wrap.js — co.wrap(fn*) > should expose the underlying generator function
func TestShouldExposeTheUnderlyingGeneratorFunction(t *testing.T) {
	fn := func(y *co.Yielder, a, b, c int) (any, error) { return nil, nil }
	wrapped := co.Wrap(fn)

	exposed := wrapped.GeneratorFunction()
	if exposed == nil {
		t.Fatalf("GeneratorFunction() = nil")
	}

	// The original asserts the source starts with "function*"; the Go
	// equivalent is that the exposed value is still a generator function, i.e.
	// its first parameter is a *co.Yielder.
	typ := reflect.TypeOf(exposed)
	if typ.Kind() != reflect.Func {
		t.Fatalf("kind = %v, want func", typ.Kind())
	}
	if typ.NumIn() == 0 || typ.In(0) != reflect.TypeOf((*co.Yielder)(nil)) {
		t.Errorf("first parameter is %v, want *co.Yielder", typ.In(0))
	}
}

// Additional coverage: the wrapper is reusable and hands back a fresh promise
// per call, as the JavaScript closure does.
func TestAWrapperHandsBackAFreshPromisePerCall(t *testing.T) {
	wrapped := co.Wrap(func(y *co.Yielder, v int) (any, error) { return v * 2, nil })

	first := wrapped.Call(2)
	second := wrapped.Call(3)
	if first == second {
		t.Fatalf("expected distinct promises")
	}
	if got := mustResolve(t, first); got != 4 {
		t.Errorf("first = %v, want 4", got)
	}
	if got := mustResolve(t, second); got != 6 {
		t.Errorf("second = %v, want 6", got)
	}
}
