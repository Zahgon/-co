package co_test

import (
	"strings"
	"testing"
	"time"

	co "github.com/tj/co-go"
)

// pet ports the `Pet` constructor from test/objects.js:83-86: a non-plain
// object, which must be copied into the result untouched.
type pet struct {
	Name      string
	Something func()
}

// Port of test/objects.js — co(* -> yield {}) > should aggregate several promises
//
// The name carries the extra "object" token so it stays distinct from the
// identically titled array case in arrays_test.go.
func TestShouldAggregateSeveralObjectPromises(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		raw, err := y.Yield(co.ObjectOf(
			"a", readFilePromise(fileA),
			"b", readFilePromise(fileB),
			"c", readFilePromise(fileC),
		))
		if err != nil {
			return nil, err
		}

		res, ok := raw.(*co.Object)
		if !ok {
			t.Errorf("result is %T, want *co.Object", raw)
			return nil, nil
		}
		if res.Len() != 3 {
			t.Errorf("len = %d, want 3", res.Len())
			return nil, nil
		}

		a, _ := res.Get("a")
		b, _ := res.Get("b")
		c, _ := res.Get("c")
		if !strings.Contains(a.(string), needleA) {
			t.Errorf("a missing %q", needleA)
		}
		if !strings.Contains(b.(string), needleB) {
			t.Errorf("b missing %q", needleB)
		}
		if !strings.Contains(c.(string), needleC) {
			t.Errorf("c missing %q", needleC)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Port of test/objects.js — co(* -> yield {}) > should noop with no args
func TestShouldNoopWithNoObjectArgs(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		raw, err := y.Yield(co.NewObject())
		if err != nil {
			return nil, err
		}
		res, ok := raw.(*co.Object)
		if !ok {
			t.Errorf("result is %T, want *co.Object", raw)
			return nil, nil
		}
		if res.Len() != 0 {
			t.Errorf("len = %d, want 0", res.Len())
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Port of test/objects.js — co(* -> yield {}) > should ignore non-thunkable properties
func TestShouldIgnoreNonThunkableProperties(t *testing.T) {
	now := time.Now()
	tobi := &pet{Name: "tobi", Something: func() {}}

	body := func(y *co.Yielder) (any, error) {
		foo := co.ObjectOf(
			"name", map[string]any{"first": "tobi"},
			"age", 2,
			"address", readFilePromise(fileA),
			"tobi", tobi,
			"now", now,
			"falsey", false,
			"nully", nil,
		)

		raw, err := y.Yield(foo)
		if err != nil {
			return nil, err
		}
		res := raw.(*co.Object)

		// A nested plain object is itself a yieldable, so it is resolved
		// rather than copied — same as the original.
		name, _ := res.Get("name")
		if name.(map[string]any)["first"] != "tobi" {
			t.Errorf("name.first = %v, want tobi", name)
		}

		if age, _ := res.Get("age"); age != 2 {
			t.Errorf("age = %v, want 2", age)
		}

		gotPet, _ := res.Get("tobi")
		if gotPet.(*pet).Name != "tobi" {
			t.Errorf("tobi.Name = %v, want tobi", gotPet)
		}

		if gotNow, _ := res.Get("now"); gotNow != any(now) {
			t.Errorf("now = %v, want %v", gotNow, now)
		}

		if falsey, _ := res.Get("falsey"); falsey != false {
			t.Errorf("falsey = %v, want false", falsey)
		}

		if nully, _ := res.Get("nully"); nully != nil {
			t.Errorf("nully = %v, want nil", nully)
		}

		address, _ := res.Get("address")
		if !strings.Contains(address.(string), needleA) {
			t.Errorf("address missing %q", needleA)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Port of test/objects.js — co(* -> yield {}) > should preserve key order
func TestShouldPreserveKeyOrder(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		before := co.ObjectOf(
			"sun", timedThunk(30*time.Millisecond),
			"rain", timedThunk(20*time.Millisecond),
			"moon", timedThunk(10*time.Millisecond),
		)

		raw, err := y.Yield(before)
		if err != nil {
			return nil, err
		}
		after := raw.(*co.Object)

		orderBefore := strings.Join(before.Keys(), ",")
		orderAfter := strings.Join(after.Keys(), ",")
		if orderBefore != orderAfter {
			t.Errorf("order = %q, want %q", orderAfter, orderBefore)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Additional coverage: a plain Go map is accepted and returns a map.
func TestAPlainMapIsAcceptedAndResolvesToAMap(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		raw, err := y.Yield(map[string]any{
			"a": co.Resolve(1),
			"b": 2,
		})
		if err != nil {
			return nil, err
		}
		res, ok := raw.(map[string]any)
		if !ok {
			t.Errorf("result is %T, want map[string]any", raw)
			return nil, nil
		}
		if res["a"] != 1 {
			t.Errorf("a = %v, want 1", res["a"])
		}
		if res["b"] != 2 {
			t.Errorf("b = %v, want 2", res["b"])
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Additional coverage: Object.Map exposes the underlying key/value pairs.
func TestObjectMapExposesEveryPair(t *testing.T) {
	obj := co.ObjectOf("a", 1, "b", 2)
	obj.Set("a", 3)

	m := obj.Map()
	if len(m) != 2 {
		t.Fatalf("len = %d, want 2", len(m))
	}
	if m["a"] != 3 {
		t.Errorf("a = %v, want 3", m["a"])
	}
	if m["b"] != 2 {
		t.Errorf("b = %v, want 2", m["b"])
	}

	// The snapshot must not alias the object's storage.
	m["a"] = 99
	if got, _ := obj.Get("a"); got != 3 {
		t.Errorf("Get(a) = %v after mutating the snapshot, want 3", got)
	}
}
