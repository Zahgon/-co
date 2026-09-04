package co_test

import (
	"testing"

	co "github.com/tj/co-go"
)

type receiver struct {
	some string
}

// Port of test/context.js — co.call(this) > should pass the context
func TestShouldPassTheContext(t *testing.T) {
	ctx := &receiver{some: "thing"}

	body := func(y *co.Yielder) (any, error) {
		if y.Ctx() != any(ctx) {
			t.Errorf("y.Ctx() = %#v, want %#v", y.Ctx(), ctx)
		}
		return nil, nil
	}

	mustResolve(t, co.CoCtx(ctx, body))
}

// Additional coverage: the receiver reaches a nested yieldable and a thunk,
// matching the reach of `this` in the original.
func TestShouldPropagateTheReceiverIntoNestedYieldables(t *testing.T) {
	ctx := &receiver{some: "thing"}

	thunk := co.CtxThunk(func(got any, done func(error, ...any)) {
		if got != any(ctx) {
			t.Errorf("thunk ctx = %#v, want %#v", got, ctx)
		}
		done(nil, "ok")
	})

	inner := func(y *co.Yielder) (any, error) {
		if y.Ctx() != any(ctx) {
			t.Errorf("nested ctx = %#v, want %#v", y.Ctx(), ctx)
		}
		return y.Yield(thunk)
	}

	body := func(y *co.Yielder) (any, error) {
		return y.Yield(inner)
	}

	if got := mustResolve(t, co.CoCtx(ctx, body)); got != "ok" {
		t.Errorf("got = %v, want ok", got)
	}
}
