package co

import "reflect"

// Thunk is a function whose only argument is a node-style callback.
//
// Thunk support exists purely for parity with the JavaScript original, where
// it is documented as legacy (Readme.md:127-131). Sixteen of the ported tests
// depend on it, so it is a first-class yieldable here too.
type Thunk func(done func(err error, res ...any))

// CtxThunk is a Thunk that also receives the co receiver, the port of a thunk
// invoked as `fn.call(ctx, cb)` (index.js:137).
type CtxThunk func(ctx any, done func(err error, res ...any))

var (
	thunkType    = reflect.TypeOf(Thunk(nil))
	ctxThunkType = reflect.TypeOf(CtxThunk(nil))
)

// isThunkValue reports whether v has a callable thunk shape.
//
// JavaScript accepts ANY function as a thunk; Go must check the signature.
// Functions that do not match fall through toPromise unchanged and end up
// producing the invalid-yield TypeError.
func isThunkValue(v any) bool {
	if v == nil {
		return false
	}
	t := reflect.TypeOf(v)
	if t.Kind() != reflect.Func {
		return false
	}
	if reflect.ValueOf(v).IsNil() {
		return false
	}
	return t.ConvertibleTo(thunkType) || t.ConvertibleTo(ctxThunkType)
}

// thunkToPromise ports index.js:134-143.
//
// Three behaviours are load-bearing and each has a dedicated test:
//   - a truthy error rejects;
//   - a callback invoked with more than one result resolves with the slice of
//     results (`arguments.length > 2` in the original);
//   - the first settlement wins, so calling back and then panicking rejects
//     with the callback's error.
func thunkToPromise(ctx any, fn any) *Promise {
	return New(func(resolve func(any), reject func(error)) {
		done := func(err error, res ...any) {
			if err != nil {
				reject(err)
				return
			}
			switch len(res) {
			case 0:
				resolve(nil)
			case 1:
				resolve(res[0])
			default:
				resolve(append([]any(nil), res...))
			}
		}

		// A panic raised synchronously by the thunk escapes into New's
		// executor recovery and becomes a rejection.
		callThunk(ctx, fn, done)
	})
}

// callThunk invokes fn with the callback, passing the receiver when the
// signature asks for it.
func callThunk(ctx any, fn any, done func(err error, res ...any)) {
	t := reflect.TypeOf(fn)

	if t.ConvertibleTo(thunkType) {
		reflect.ValueOf(fn).Convert(thunkType).Interface().(Thunk)(done)
		return
	}

	reflect.ValueOf(fn).Convert(ctxThunkType).Interface().(CtxThunk)(ctx, done)
}
