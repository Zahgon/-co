package co_test

import (
	"testing"

	co "github.com/tj/co-go"
)

// Port of test/arguments.js — co(gen, args) > should pass the rest of the arguments
func TestShouldPassTheRestOfTheArguments(t *testing.T) {
	body := func(y *co.Yielder, num int, str string, arr []any, obj map[string]any, fun func()) (any, error) {
		if num != 42 {
			t.Errorf("num = %v, want 42", num)
		}
		if str != "forty-two" {
			t.Errorf("str = %q, want \"forty-two\"", str)
		}
		if len(arr) != 1 || arr[0] != 42 {
			t.Errorf("arr = %v, want [42]", arr)
		}
		if obj["value"] != 42 {
			t.Errorf("obj.value = %v, want 42", obj["value"])
		}
		if fun == nil {
			t.Errorf("fun is nil, want a function")
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body, 42, "forty-two", []any{42}, map[string]any{"value": 42}, func() {}))
}

// Additional coverage: the canonical GeneratorFunc signature reaches the same
// arguments through Yielder.Args.
func TestShouldExposeTheArgumentsViaYielderArgs(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		args := y.Args()
		if len(args) != 2 {
			t.Errorf("len(args) = %d, want 2", len(args))
			return nil, nil
		}
		if args[0] != 42 {
			t.Errorf("args[0] = %v, want 42", args[0])
		}
		if args[1] != "forty-two" {
			t.Errorf("args[1] = %v, want forty-two", args[1])
		}
		return nil, nil
	}

	mustResolve(t, co.Co(co.GeneratorFunc(body), 42, "forty-two"))
}
