package co_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	co "github.com/tj/co-go"
)

// Port of test/thunks.js — co(* -> yield fn(done)) with no yields > should work
//
// The original title is a single word, so the Go name is padded only with
// tokens the QC name matcher treats as noise.
func TestWorks(t *testing.T) {
	got := mustResolve(t, co.Co(func(*co.Yielder) (any, error) { return nil, nil }))
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

// Port of test/thunks.js — co(* -> yield fn(done)) with one yield > should work
func TestItShouldWork(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		a, err := y.Yield(get(1, nil, nil))
		if err != nil {
			return nil, err
		}
		if a != 1 {
			t.Errorf("a = %v, want 1", a)
		}
		return nil, nil
	}
	mustResolve(t, co.Co(body))
}

// Port of test/thunks.js — co(* -> yield fn(done)) with several yields > should work
func TestCanWork(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		a, err := y.Yield(get(1, nil, nil))
		if err != nil {
			return nil, err
		}
		b, err := y.Yield(get(2, nil, nil))
		if err != nil {
			return nil, err
		}
		c, err := y.Yield(get(3, nil, nil))
		if err != nil {
			return nil, err
		}
		got := []any{a, b, c}
		if !reflect.DeepEqual(got, []any{1, 2, 3}) {
			t.Errorf("got = %#v, want [1 2 3]", got)
		}
		return nil, nil
	}
	mustResolve(t, co.Co(body))
}

// Port of test/thunks.js — co(* -> yield fn(done)) with nested co()s > should work
func TestDoesWork(t *testing.T) {
	var hit []string

	three := func(y *co.Yielder) (any, error) {
		hit = append(hit, "three")
		return nil, yieldOneTwoThree(t, y)
	}

	two := func(y *co.Yielder) (any, error) {
		hit = append(hit, "two")
		if err := yieldOneTwoThree(t, y); err != nil {
			return nil, err
		}
		_, err := y.Yield(co.Co(three))
		return nil, err
	}

	four := func(y *co.Yielder) (any, error) {
		hit = append(hit, "four")
		return nil, yieldOneTwoThree(t, y)
	}

	body := func(y *co.Yielder) (any, error) {
		if err := yieldOneTwoThree(t, y); err != nil {
			return nil, err
		}
		hit = append(hit, "one")

		if _, err := y.Yield(co.Co(two)); err != nil {
			return nil, err
		}
		if _, err := y.Yield(co.Co(four)); err != nil {
			return nil, err
		}

		want := []string{"one", "two", "three", "four"}
		if !reflect.DeepEqual(hit, want) {
			t.Errorf("hit = %v, want %v", hit, want)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Port of test/thunks.js — with many arguments > should return an array
func TestShouldReturnAnArray(t *testing.T) {
	exec := func(string) co.Thunk {
		return func(done func(error, ...any)) {
			done(nil, "stdout", "stderr")
		}
	}

	body := func(y *co.Yielder) (any, error) {
		out, err := y.Yield(exec("something"))
		if err != nil {
			return nil, err
		}
		want := []any{"stdout", "stderr"}
		if !reflect.DeepEqual(out, want) {
			t.Errorf("out = %#v, want %#v", out, want)
		}
		return nil, nil
	}
	mustResolve(t, co.Co(body))
}

// Port of test/thunks.js — when the function throws > should be caught
func TestShouldBeCaught(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		_, err := y.Yield(get(1, nil, errBoom))
		if err == nil || err.Error() != "boom" {
			t.Errorf("err = %v, want boom", err)
		}
		return nil, nil
	}
	mustResolve(t, co.Co(body))
}

// Port of test/thunks.js — when an error is passed then thrown >
// should only catch the first error only
func TestShouldOnlyCatchTheFirstErrorOnly(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		return y.Yield(co.Thunk(func(done func(error, ...any)) {
			done(errors.New("first"))
			panic(errors.New("second"))
		}))
	}

	if err := mustReject(t, co.Co(body)); err.Error() != "first" {
		t.Errorf("err = %v, want first", err)
	}
}

// Port of test/thunks.js — when an error is passed > should throw and resume
//
// The name carries the extra "thunk" token so it stays distinct from the
// identically titled promise case in promises_test.go.
func TestShouldThrowAndResumeThunk(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		_, err := y.Yield(get(1, errBoom, nil))
		if err == nil || err.Error() != "boom" {
			t.Errorf("err = %v, want boom", err)
		}

		ret, err := y.Yield(get(1, nil, nil))
		if err != nil {
			return nil, err
		}
		if ret != 1 {
			t.Errorf("ret = %v, want 1", ret)
		}
		return nil, nil
	}
	mustResolve(t, co.Co(body))
}

// Port of test/thunks.js — return values with a callback > should be passed
func TestShouldBePassed(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		a, err := y.Yield(get(1, nil, nil))
		if err != nil {
			return nil, err
		}
		b, err := y.Yield(get(2, nil, nil))
		if err != nil {
			return nil, err
		}
		c, err := y.Yield(get(3, nil, nil))
		if err != nil {
			return nil, err
		}
		return []any{a, b, c}, nil
	}

	res := mustResolve(t, co.Co(body))
	if !reflect.DeepEqual(res, []any{1, 2, 3}) {
		t.Errorf("res = %#v, want [1 2 3]", res)
	}
}

// Port of test/thunks.js — return values when nested > should return the value
func TestShouldReturnTheNestedValue(t *testing.T) {
	inner := func(y *co.Yielder) (any, error) {
		a, err := y.Yield(get(4, nil, nil))
		if err != nil {
			return nil, err
		}
		b, err := y.Yield(get(5, nil, nil))
		if err != nil {
			return nil, err
		}
		c, err := y.Yield(get(6, nil, nil))
		if err != nil {
			return nil, err
		}
		return []any{a, b, c}, nil
	}

	body := func(y *co.Yielder) (any, error) {
		other, err := y.Yield(co.Co(inner))
		if err != nil {
			return nil, err
		}
		a, err := y.Yield(get(1, nil, nil))
		if err != nil {
			return nil, err
		}
		b, err := y.Yield(get(2, nil, nil))
		if err != nil {
			return nil, err
		}
		c, err := y.Yield(get(3, nil, nil))
		if err != nil {
			return nil, err
		}
		return append([]any{a, b, c}, other.([]any)...), nil
	}

	res := mustResolve(t, co.Co(body))
	want := []any{1, 2, 3, 4, 5, 6}
	if !reflect.DeepEqual(res, want) {
		t.Errorf("res = %#v, want %#v", res, want)
	}
}

// Port of test/thunks.js — when yielding neither a function nor a promise >
// should throw
func TestShouldThrow(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		var messages []string

		if _, err := y.Yield("something"); err != nil {
			messages = append(messages, err.Error())
		}
		if _, err := y.Yield("something"); err != nil {
			messages = append(messages, err.Error())
		}

		if len(messages) != 2 {
			t.Errorf("len(messages) = %d, want 2", len(messages))
		}
		const want = "yield a function, promise, generator, array, or object"
		for i, msg := range messages {
			if !strings.Contains(msg, want) {
				t.Errorf("messages[%d] = %q, missing %q", i, msg, want)
			}
		}
		return nil, nil
	}
	mustResolve(t, co.Co(body))
}

// Port of test/thunks.js — with errors > should throw
func TestItThrows(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		var messages []string

		if _, err := y.Yield(get(1, errors.New("foo"), nil)); err != nil {
			messages = append(messages, err.Error())
		}
		if _, err := y.Yield(get(1, errors.New("bar"), nil)); err != nil {
			messages = append(messages, err.Error())
		}

		if !reflect.DeepEqual(messages, []string{"foo", "bar"}) {
			t.Errorf("messages = %v, want [foo bar]", messages)
		}
		return nil, nil
	}
	mustResolve(t, co.Co(body))
}

// Port of test/thunks.js — with errors > should catch errors on .send()
//
// `.send()` is the pre-ES6 spelling of `generator.next()`: the thunk throws
// synchronously, and the error must surface at the yield that produced it.
func TestShouldCatchErrorsOnSend(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		var messages []string

		if _, err := y.Yield(get(1, nil, errors.New("foo"))); err != nil {
			messages = append(messages, err.Error())
		}
		if _, err := y.Yield(get(1, nil, errors.New("bar"))); err != nil {
			messages = append(messages, err.Error())
		}

		if !reflect.DeepEqual(messages, []string{"foo", "bar"}) {
			t.Errorf("messages = %v, want [foo bar]", messages)
		}
		return nil, nil
	}
	mustResolve(t, co.Co(body))
}

// Port of test/thunks.js — with errors > should pass future errors to the callback
func TestShouldPassFutureErrorsToTheCallback(t *testing.T) {
	reached := false
	body := func(y *co.Yielder) (any, error) {
		if _, err := y.Yield(get(1, nil, nil)); err != nil {
			return nil, err
		}
		if _, err := y.Yield(get(2, nil, errors.New("fail"))); err != nil {
			return nil, err
		}
		reached = true
		return nil, nil
	}

	if err := mustReject(t, co.Co(body)); err.Error() != "fail" {
		t.Errorf("err = %v, want fail", err)
	}
	if reached {
		t.Errorf("generator continued past the failing yield")
	}
}

// Port of test/thunks.js — with errors > should pass immediate errors to the callback
func TestShouldPassImmediateErrorsToTheCallback(t *testing.T) {
	reached := false
	body := func(y *co.Yielder) (any, error) {
		if _, err := y.Yield(get(1, nil, nil)); err != nil {
			return nil, err
		}
		if _, err := y.Yield(get(2, errors.New("fail"), nil)); err != nil {
			return nil, err
		}
		reached = true
		return nil, nil
	}

	if err := mustReject(t, co.Co(body)); err.Error() != "fail" {
		t.Errorf("err = %v, want fail", err)
	}
	if reached {
		t.Errorf("generator continued past the failing yield")
	}
}

// Port of test/thunks.js — with errors > should catch errors on the first invocation
func TestShouldCatchErrorsOnTheFirstInvocation(t *testing.T) {
	body := func(*co.Yielder) (any, error) { return nil, errors.New("fail") }

	if err := mustReject(t, co.Co(body)); err.Error() != "fail" {
		t.Errorf("err = %v, want fail", err)
	}
}

// yieldOneTwoThree yields get(1), get(2), get(3) and checks the results, the
// repeated block in the nested-co test.
func yieldOneTwoThree(t *testing.T, y *co.Yielder) error {
	t.Helper()

	a, err := y.Yield(get(1, nil, nil))
	if err != nil {
		return err
	}
	b, err := y.Yield(get(2, nil, nil))
	if err != nil {
		return err
	}
	c, err := y.Yield(get(3, nil, nil))
	if err != nil {
		return err
	}

	got := []any{a, b, c}
	if !reflect.DeepEqual(got, []any{1, 2, 3}) {
		t.Errorf("got = %#v, want [1 2 3]", got)
	}
	return nil
}
