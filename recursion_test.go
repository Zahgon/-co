package co_test

import (
	"strings"
	"testing"

	co "github.com/tj/co-go"
)

// Port of test/recursion.js — co() recursion > should aggregate arrays within arrays
func TestShouldAggregateArraysWithinArrays(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		a := readFilePromise(fileA)
		b := readFilePromise(fileB)
		c := readFilePromise(fileC)

		raw, err := y.Yield([]any{a, []any{b, c}})
		if err != nil {
			return nil, err
		}

		res := raw.([]any)
		if len(res) != 2 {
			t.Errorf("len = %d, want 2", len(res))
			return nil, nil
		}
		if !strings.Contains(res[0].(string), needleA) {
			t.Errorf("res[0] missing %q", needleA)
		}

		nested := res[1].([]any)
		if len(nested) != 2 {
			t.Errorf("nested len = %d, want 2", len(nested))
			return nil, nil
		}
		if !strings.Contains(nested[0].(string), needleB) {
			t.Errorf("nested[0] missing %q", needleB)
		}
		if !strings.Contains(nested[1].(string), needleC) {
			t.Errorf("nested[1] missing %q", needleC)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Port of test/recursion.js — co() recursion > should aggregate objects within objects
func TestShouldAggregateObjectsWithinObjects(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		a := readFilePromise(fileA)
		b := readFilePromise(fileB)
		c := readFilePromise(fileC)

		raw, err := y.Yield(co.ObjectOf(
			"0", a,
			"1", co.ObjectOf("0", b, "1", c),
		))
		if err != nil {
			return nil, err
		}

		res := raw.(*co.Object)
		zero, _ := res.Get("0")
		if !strings.Contains(zero.(string), needleA) {
			t.Errorf("res[0] missing %q", needleA)
		}

		one, _ := res.Get("1")
		nested := one.(*co.Object)
		nb, _ := nested.Get("0")
		nc, _ := nested.Get("1")
		if !strings.Contains(nb.(string), needleB) {
			t.Errorf("res[1][0] missing %q", needleB)
		}
		if !strings.Contains(nc.(string), needleC) {
			t.Errorf("res[1][1] missing %q", needleC)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}
