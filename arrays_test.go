package co_test

import (
	"reflect"
	"strings"
	"testing"

	co "github.com/tj/co-go"
)

// Port of test/arrays.js — co(* -> yield []) > should aggregate several promises
func TestShouldAggregateSeveralPromises(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		a := readFilePromise(fileA)
		b := readFilePromise(fileB)
		c := readFilePromise(fileC)

		raw, err := y.Yield([]any{a, b, c})
		if err != nil {
			return nil, err
		}

		res, ok := raw.([]any)
		if !ok {
			t.Errorf("result is %T, want []any", raw)
			return nil, nil
		}
		if len(res) != 3 {
			t.Errorf("len = %d, want 3", len(res))
			return nil, nil
		}
		if !strings.Contains(res[0].(string), needleA) {
			t.Errorf("res[0] missing %q", needleA)
		}
		if !strings.Contains(res[1].(string), needleB) {
			t.Errorf("res[1] missing %q", needleB)
		}
		if !strings.Contains(res[2].(string), needleC) {
			t.Errorf("res[2] missing %q", needleC)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Port of test/arrays.js — co(* -> yield []) > should noop with no args
func TestShouldNoopWithNoArgs(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		raw, err := y.Yield([]any{})
		if err != nil {
			return nil, err
		}
		res, ok := raw.([]any)
		if !ok {
			t.Errorf("result is %T, want []any", raw)
			return nil, nil
		}
		if len(res) != 0 {
			t.Errorf("len = %d, want 0", len(res))
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Port of test/arrays.js — co(* -> yield []) > should support an array of generators
func TestShouldSupportAnArrayOfGenerators(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		inner := co.Gen(func(*co.Yielder) (any, error) { return 1, nil })

		val, err := y.Yield([]any{inner})
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(val, []any{1}) {
			t.Errorf("val = %#v, want []any{1}", val)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}
