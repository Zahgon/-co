package co_test

import (
	"errors"
	"strings"
	"testing"

	co "github.com/tj/co-go"
)

// Port of test/invalid.js — yield <invalid> > should throw an error
func TestShouldThrowAnError(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		_, err := y.Yield(nil)
		if err == nil {
			return nil, errors.New("lol")
		}
		if _, isTypeError := co.AsTypeError(err); !isTypeError {
			t.Errorf("err is %T, want *co.TypeError", err)
		}
		if !strings.Contains(err.Error(), "You may only yield") {
			t.Errorf("err = %q, want it to contain \"You may only yield\"", err)
		}
		return nil, nil
	}

	mustResolve(t, co.Co(body))
}

// Additional coverage: the message is byte-identical to index.js:102-103.
func TestInvalidYieldReproducesTheOriginalMessageVerbatim(t *testing.T) {
	body := func(y *co.Yielder) (any, error) {
		_, err := y.Yield("something")
		return nil, err
	}

	err := mustReject(t, co.Co(body))
	want := `You may only yield a function, promise, generator, array, or object, ` +
		`but the following object was passed: "something"`
	if err.Error() != want {
		t.Errorf("err  = %q\nwant = %q", err.Error(), want)
	}
}
