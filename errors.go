package co

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// TypeError is the Go counterpart of JavaScript's TypeError.
//
// `co` raises exactly one: the invalid-yield error from index.js:102-103.
type TypeError struct {
	Message string
}

func (e *TypeError) Error() string { return e.Message }

// invalidYieldMessage reproduces index.js:102-103 byte for byte.
const invalidYieldPrefix = "You may only yield a function, promise, generator, array, or object, " +
	"but the following object was passed: "

// newInvalidYieldError builds the TypeError thrown back into the generator
// when a non-yieldable value is yielded.
func newInvalidYieldError(v any) *TypeError {
	return &TypeError{Message: invalidYieldPrefix + `"` + jsString(v) + `"`}
}

// AsTypeError reports whether err is (or wraps) a co.TypeError.
func AsTypeError(err error) (*TypeError, bool) {
	var te *TypeError
	if errors.As(err, &te) {
		return te, true
	}
	return nil, false
}

// Rejection carries a non-error rejection reason.
//
// JavaScript can `throw` or reject with any value; Go rejections are typed as
// `error`. Rejection is the bridge: co.Reason wraps arbitrary values so they
// survive a round trip through the promise machinery unchanged.
type Rejection struct {
	Value any
}

func (r *Rejection) Error() string { return jsString(r.Value) }

// Unwrap exposes an inner error when the rejection reason happens to be one.
func (r *Rejection) Unwrap() error {
	if err, ok := r.Value.(error); ok {
		return err
	}
	return nil
}

// Reason converts an arbitrary JavaScript-style rejection reason into an error.
// Errors pass through untouched; everything else is wrapped in a *Rejection.
func Reason(v any) error {
	if v == nil {
		return &Rejection{Value: nil}
	}
	if err, ok := v.(error); ok {
		return err
	}
	return &Rejection{Value: v}
}

// thrown is the panic payload used by co.Throw so that a deliberate throw can
// be told apart from an accidental panic.
type thrown struct {
	err error
}

// Throw raises err as a JavaScript-style exception.
//
// Inside a generator body or a promise reaction this behaves like `throw`: the
// surrounding machinery converts it into a rejection.
func Throw(err error) {
	panic(thrown{err: err})
}

// asError normalises a recovered panic value into an error.
func asError(r any) error {
	switch v := r.(type) {
	case thrown:
		return v.err
	case error:
		return v
	default:
		return Reason(v)
	}
}

// The string forms ECMAScript's String() produces for values that have no
// direct Go spelling. Naming them keeps the spec tokens stated exactly once.
const (
	jsNull        = "null"
	jsTrue        = "true"
	jsFalse       = "false"
	jsFunction    = "function"
	jsObjectTag   = "[object Object]"
	jsNaN         = "NaN"
	jsInfinity    = "Infinity"
	jsNegInfinity = "-Infinity"
	jsZero        = "0"

	typeErrorTag = "TypeError: "
	errorTag     = "Error: "
)

// jsString reproduces JavaScript's String() coercion closely enough for the
// invalid-yield message. Go has no `undefined`, so nil renders as "null".
func jsString(v any) string {
	if v == nil {
		return jsNull
	}

	// A typed nil pointer is a non-nil interface, so the type switch below
	// would happily select its String() or Error() method and then panic
	// dereferencing it. JavaScript has a single null, so render one.
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Ptr && rv.IsNil() {
		return jsNull
	}

	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return jsTrue
		}
		return jsFalse
	case *TypeError:
		return typeErrorTag + x.Message
	case *Rejection:
		// A Rejection is a Go-side envelope, not a JavaScript value; render
		// the reason it carries.
		return jsString(x.Value)
	case error:
		// String(new Error('boom')) === 'Error: boom'
		return errorTag + x.Error()
	case fmt.Stringer:
		return x.String()
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return jsNumber(rv.Float())
	case reflect.Slice, reflect.Array:
		// A byte slice stands in for a Buffer, which stringifies as its text
		// rather than as a comma-separated list of char codes.
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return string(bytesOf(rv))
		}

		parts := make([]string, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			el := rv.Index(i).Interface()
			if el == nil {
				parts[i] = "" // String([null]) === ""
				continue
			}
			parts[i] = jsString(el)
		}
		return strings.Join(parts, ",")
	case reflect.Map, reflect.Struct:
		return jsObjectTag
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return jsNull
		}
		return jsString(rv.Elem().Interface())
	case reflect.Func:
		return jsFunction
	}

	return fmt.Sprint(v)
}

// bytesOf reads a byte slice or byte array out of rv, copying only when the
// value is an unaddressable array that cannot be sliced in place.
func bytesOf(rv reflect.Value) []byte {
	if rv.Kind() == reflect.Slice {
		return rv.Bytes()
	}

	out := make([]byte, rv.Len())
	for i := range out {
		out[i] = byte(rv.Index(i).Uint())
	}
	return out
}

// jsNumber implements ECMAScript Number::toString (ECMA-262 6.1.6.1.20).
//
// strconv's %g is NOT equivalent and cannot be made so by tuning precision:
// JavaScript switches to exponential notation only outside 1e-7..1e21 and
// never zero-pads the exponent, so %g renders 1e6 as "1e+06" where JavaScript
// gives "1000000", and 1e-7 as "1e-07" where JavaScript gives "1e-7".
//
// The spec defines s (the shortest digit string that round-trips) and n (its
// decimal exponent plus one), then selects one of five layouts from them.
// FormatFloat(f, 'e', -1, 64) produces exactly that s and n.
func jsNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return jsNaN
	case math.IsInf(f, 1):
		return jsInfinity
	case math.IsInf(f, -1):
		return jsNegInfinity
	case f == 0:
		return jsZero // also covers -0, which JavaScript renders as "0"
	case f < 0:
		return "-" + jsNumber(-f)
	}

	s, exp := shortestDigits(f)
	k := len(s)
	n := exp + 1

	switch {
	case k <= n && n <= 21:
		return s + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return s[:n] + "." + s[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + s
	}

	e := n - 1
	sign := "+"
	if e < 0 {
		sign, e = "-", -e
	}
	if k == 1 {
		return s + "e" + sign + strconv.Itoa(e)
	}
	return s[:1] + "." + s[1:] + "e" + sign + strconv.Itoa(e)
}

// shortestDigits returns the shortest round-tripping decimal digits of f with
// the point removed, plus the decimal exponent.
func shortestDigits(f float64) (digits string, exp int) {
	mantissa, exponent, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e")
	exp, _ = strconv.Atoi(exponent)
	return strings.Replace(mantissa, ".", "", 1), exp
}
