package co_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	co "github.com/tj/co-go"
)

func exampleReadFile(name string) co.Thunk {
	return func(done func(err error, res ...any)) {
		go func() {
			data, err := os.ReadFile(name)
			done(err, string(data))
		}()
	}
}

func exampleSleep(d time.Duration) co.Thunk {
	return func(done func(err error, res ...any)) {
		time.AfterFunc(d, func() { done(nil) })
	}
}

func exampleDelayed(value any, d time.Duration) *co.Promise {
	return co.New(func(resolve func(any), reject func(error)) {
		time.AfterFunc(d, func() { resolve(value) })
	})
}

func exampleGreet(y *co.Yielder, name string) (any, error) {
	if _, err := y.Yield(exampleSleep(10 * time.Millisecond)); err != nil {
		return nil, err
	}
	return "hello " + name, nil
}

// Example demonstrates every yieldable kind co supports, in one generator.
func Example() {
	value, err := co.Co(func(y *co.Yielder) (any, error) {
		// A thunk: a function taking a completion callback.
		text, err := y.Yield(exampleReadFile("go.mod"))
		if err != nil {
			return nil, err
		}
		fmt.Printf("thunk:     %v\n", strings.HasPrefix(text.(string), "module "))

		// A promise.
		v, err := y.Yield(exampleDelayed(42, 10*time.Millisecond))
		if err != nil {
			return nil, err
		}
		fmt.Printf("promise:   %v\n", v)

		// A generator function, run with co and given its arguments.
		greeting, err := y.Yield(co.Wrap(exampleGreet).Call("world"))
		if err != nil {
			return nil, err
		}
		fmt.Printf("generator: %v\n", greeting)

		// An array: resolved in parallel, order preserved.
		batch, err := y.Yield([]any{
			exampleDelayed("a", 30*time.Millisecond),
			exampleDelayed("b", 20*time.Millisecond),
			exampleDelayed("c", 10*time.Millisecond),
		})
		if err != nil {
			return nil, err
		}
		fmt.Printf("array:     %v\n", batch)

		// An object: values resolved in parallel, keys preserved.
		obj, err := y.Yield(co.ObjectOf(
			"sun", exampleDelayed("hot", 30*time.Millisecond),
			"rain", exampleDelayed("wet", 20*time.Millisecond),
			"moon", exampleDelayed("cold", 10*time.Millisecond),
		))
		if err != nil {
			return nil, err
		}
		fmt.Printf("object:    keys in order %v\n", obj.(*co.Object).Keys())

		// A rejection comes back as an error and can be recovered from.
		if _, err := y.Yield(co.Reject(errors.New("expected"))); err != nil {
			fmt.Printf("recovered: %v\n", err)
		}

		// An invalid yield is thrown back in, so it is recoverable too.
		if _, err := y.Yield(42); err != nil {
			fmt.Printf("invalid:   %s...\n", strings.SplitN(err.Error(), ",", 2)[0])
		}

		return "done", nil
	}).Await()

	fmt.Printf("result:    %v (err: %v)\n", value, err)

	// Output:
	// thunk:     true
	// promise:   42
	// generator: hello world
	// array:     [a b c]
	// object:    keys in order [sun rain moon]
	// recovered: expected
	// invalid:   You may only yield a function...
	// result:    done (err: <nil>)
}

// ExampleWrap shows co.Wrap turning a generator function into a reusable
// promise-returning function, the port of co.wrap(fn*).
func ExampleWrap() {
	double := co.Wrap(func(y *co.Yielder, v int) (any, error) {
		if _, err := y.Yield(exampleSleep(time.Millisecond)); err != nil {
			return nil, err
		}
		return v * 2, nil
	})

	first, _ := double.Call(2).Await()
	second, _ := double.Call(21).Await()
	fmt.Println(first, second)

	// Output: 4 42
}

// ExampleObject shows why the port needs an ordered object: JavaScript object
// keys are yielded and returned in a defined order, which a Go map cannot hold.
func ExampleObject() {
	obj := co.ObjectOf(
		"2", "two",
		"zebra", "z",
		"0", "zero",
		"apple", "a",
	)

	// Canonical array indices sort ascending and come first; the rest keep
	// insertion order.
	fmt.Println(obj.Keys())

	// Output: [0 2 zebra apple]
}
