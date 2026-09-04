package co_test

import (
	"reflect"
	"testing"

	co "github.com/tj/co-go"
)

// Port of test/generators.js — co(*) > should wrap with co()
//
// The name carries the extra "generator" token so it stays distinct from the
// identically titled generator-function case in generator_functions_test.go.
func TestShouldWrapAGeneratorWithCo(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		a, err := y.Yield(newWork())
		if err != nil {
			return nil, err
		}
		b, err := y.Yield(newWork())
		if err != nil {
			return nil, err
		}
		c, err := y.Yield(newWork())
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

		res, err := y.Yield([]any{newWork(), newWork(), newWork()})
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

// Port of test/generators.js — co(*) > should catch errors
func TestShouldCatchGeneratorErrors(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		failing := co.Gen(func(*co.Yielder) (any, error) { return nil, errBoom })
		_, err := y.Yield(failing)
		return nil, err
	}

	if err := mustReject(t, co.Co(body)); err.Error() != "boom" {
		t.Errorf("err = %v, want boom", err)
	}
}
