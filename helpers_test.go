package co_test

import (
	"errors"
	"os"
	"testing"
	"time"

	co "github.com/tj/co-go"
)

// The JavaScript suite reads index.js / LICENSE / package.json through mz/fs
// and asserts on a substring of each. The Go port reads the equivalent files
// from this module and asserts on an equivalent substring.
//
//	index.js     -> co.go   ("exports"         -> "package co")
//	LICENSE      -> LICENSE ("MIT"             -> "MIT")
//	package.json -> go.mod  ("devDependencies" -> "module")
const (
	fileA, needleA = "co.go", "package co"
	fileB, needleB = "LICENSE", "MIT"
	fileC, needleC = "go.mod", "module"
)

// awaitTimeout bounds every test so a stuck trampoline fails loudly instead of
// hanging the suite.
const awaitTimeout = 10 * time.Second

// await blocks until p settles, failing the test if it never does.
func await(t *testing.T, p *co.Promise) (any, error) {
	t.Helper()
	select {
	case <-p.Done():
	case <-time.After(awaitTimeout):
		t.Fatalf("timed out after %s waiting for promise", awaitTimeout)
	}
	return p.Await()
}

// mustResolve asserts the promise fulfils and returns its value.
func mustResolve(t *testing.T, p *co.Promise) any {
	t.Helper()
	value, err := await(t, p)
	if err != nil {
		t.Fatalf("expected fulfilment, got rejection: %v", err)
	}
	return value
}

// mustReject asserts the promise rejects and returns its reason.
func mustReject(t *testing.T, p *co.Promise) error {
	t.Helper()
	value, err := await(t, p)
	if err == nil {
		t.Fatalf("expected rejection, got fulfilment: %#v", value)
	}
	return err
}

// readFilePromise is the port of mz/fs readFile: it returns a promise, so a
// single read can be created once and yielded later, as the originals do.
func readFilePromise(name string) *co.Promise {
	return co.New(func(resolve func(any), reject func(error)) {
		go func() {
			data, err := os.ReadFile(name)
			if err != nil {
				reject(err)
				return
			}
			resolve(string(data))
		}()
	})
}

// sleep ports `function sleep(ms) { return function(done){ setTimeout(done, ms) } }`.
func sleep(d time.Duration) co.Thunk {
	return func(done func(err error, res ...any)) {
		time.AfterFunc(d, func() { done(nil) })
	}
}

// get ports test/thunks.js:6-13.
//
//	get(v)            -> resolves v after 10ms
//	get(v, err)       -> rejects with err after 10ms
//	get(v, nil, boom) -> panics immediately, i.e. `throw error`
func get(val any, err error, thrown error) co.Thunk {
	return func(done func(error, ...any)) {
		if thrown != nil {
			panic(thrown)
		}
		time.AfterFunc(10*time.Millisecond, func() {
			if err != nil {
				done(err)
				return
			}
			done(nil, val)
		})
	}
}

// timedThunk resolves after d without producing a value.
func timedThunk(d time.Duration) co.Thunk {
	return func(done func(error, ...any)) {
		time.AfterFunc(d, func() { done(nil) })
	}
}

// work ports `function *work() { yield sleep(50); return 'yay' }`.
func work(y *co.Yielder) (any, error) {
	if _, err := y.Yield(sleep(50 * time.Millisecond)); err != nil {
		return nil, err
	}
	return "yay", nil
}

// newWork returns a fresh live generator for the same body.
func newWork() co.Generator {
	return co.Gen(work)
}

var errBoom = errors.New("boom")
