package co_test

import (
	"reflect"
	"testing"

	co "github.com/tj/co-go"
)

// Port of test/generator-functions.js — co(fn*) > should wrap with co()
//
// This yields the generator FUNCTION rather than a live generator, exercising
// the generator-function branch of toPromise, which must be tried before the
// plain-function (thunk) branch.
func TestShouldWrapWithCo(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		a, err := y.Yield(work)
		if err != nil {
			return nil, err
		}
		b, err := y.Yield(work)
		if err != nil {
			return nil, err
		}
		c, err := y.Yield(work)
		if err != nil {
			return nil, err
		}

		if a != "yay" {
			t.Errorf("a = %v, want yay", a)
		}
		if b != "yay" {
			t.Errorf("b = %v, want yay", b)
		}
		if c != "yay" {
			t.Errorf("c = %v, want yay", c)
		}

		res, err := y.Yield([]any{work, work, work})
		if err != nil {
			return nil, err
		}
		want := []any{"yay", "yay", "yay"}
		if !reflect.DeepEqual(res, want) {
			t.Errorf("res = %#v, want %#v", res, want)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Port of test/generator-functions.js — co(fn*) > should catch errors
func TestShouldCatchErrors(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		_, err := y.Yield(func(*co.Yielder) (any, error) { return nil, errBoom })
		return nil, err
	}

	if err := mustReject(t, co.Co(body)); err.Error() != "boom" {
		t.Errorf("err = %v, want boom", err)
	}
}
