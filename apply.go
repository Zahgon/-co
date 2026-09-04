package co

import "reflect"

// Func is the signature of a receiver-aware plain function, the port of a
// non-generator `fn` passed to co and invoked as `fn.apply(ctx, args)`
// (index.js:51).
type Func func(ctx any, args ...any) any

var (
	funcType          = reflect.TypeOf(Func(nil))
	generatorFuncType = reflect.TypeOf(GeneratorFunc(nil))
	errorType         = reflect.TypeOf((*error)(nil)).Elem()
)

// isFuncKind reports whether v is a non-nil function value.
func isFuncKind(v any) bool {
	if v == nil {
		return false
	}
	t := reflect.TypeOf(v)
	return t.Kind() == reflect.Func && !reflect.ValueOf(v).IsNil()
}

// newGeneratorFromValue turns a generator function value into a live generator
// bound to ctx and args, the analogue of calling a GeneratorFunction.
//
// Bodies may either use the canonical GeneratorFunc signature and read
// co.Yielder.Args, or declare typed parameters after the *Yielder and receive
// the arguments directly.
func newGeneratorFromValue(ctx any, v any, args []any) Generator {
	t := reflect.TypeOf(v)

	if t.ConvertibleTo(generatorFuncType) {
		fn := reflect.ValueOf(v).Convert(generatorFuncType).Interface().(GeneratorFunc)
		return GenCtx(ctx, fn, args...)
	}

	fnValue := reflect.ValueOf(v)
	body := func(y *Yielder) (any, error) {
		in := buildCallArgs(t, append([]any{y}, args...))
		return normaliseResults(fnValue.Call(in))
	}
	return GenCtx(ctx, body, args...)
}

// applyPlain invokes a non-generator function, mirroring `fn.apply(ctx, args)`.
//
// A returned error is re-raised as a throw so that co's enclosing executor
// converts it into a rejection, matching JavaScript, where the only way for an
// applied function to fail is by throwing.
func applyPlain(ctx any, v any, args []any) any {
	t := reflect.TypeOf(v)

	if t.ConvertibleTo(funcType) {
		return reflect.ValueOf(v).Convert(funcType).Interface().(Func)(ctx, args...)
	}

	result, err := normaliseResults(reflect.ValueOf(v).Call(buildCallArgs(t, args)))
	if err != nil {
		Throw(err)
	}
	return result
}

// buildCallArgs adapts a slice of dynamically typed values to a function's
// parameter list.
//
// JavaScript ignores surplus arguments and passes `undefined` for missing
// ones; the same forgiveness is reproduced here by truncating extras and
// zero-filling gaps.
func buildCallArgs(t reflect.Type, values []any) []reflect.Value {
	numIn := t.NumIn()
	variadic := t.IsVariadic()

	fixed := numIn
	if variadic {
		fixed = numIn - 1
	}

	in := make([]reflect.Value, 0, max(numIn, len(values)))

	for i := 0; i < fixed; i++ {
		var value any
		if i < len(values) {
			value = values[i]
		}
		in = append(in, coerce(value, t.In(i)))
	}

	if variadic {
		elem := t.In(numIn - 1).Elem()
		for i := fixed; i < len(values); i++ {
			in = append(in, coerce(values[i], elem))
		}
	}

	return in
}

// coerce converts value to target, zero-filling nil and panicking with a
// TypeError when the value simply does not fit.
func coerce(value any, target reflect.Type) reflect.Value {
	if value == nil {
		return reflect.Zero(target)
	}

	rv := reflect.ValueOf(value)
	switch {
	case rv.Type() == target, rv.Type().AssignableTo(target):
		return rv
	case rv.Type().ConvertibleTo(target):
		return rv.Convert(target)
	default:
		panic(&TypeError{Message: "cannot pass " + rv.Type().String() + " as " + target.String()})
	}
}

// normaliseResults maps a Go return tuple onto JavaScript's single completion
// value plus an optional thrown error.
//
//	()               -> (nil, nil)
//	(error)          -> (nil, error)
//	(T)              -> (T, nil)
//	(T, error)       -> (T, error)
//	(T, U, ...)      -> (T, nil)
func normaliseResults(out []reflect.Value) (any, error) {
	switch len(out) {
	case 0:
		return nil, nil
	case 1:
		if out[0].Type() == errorType || out[0].Type().Implements(errorType) {
			return nil, toError(out[0])
		}
		return out[0].Interface(), nil
	default:
		if out[1].Type() == errorType || out[1].Type().Implements(errorType) {
			return out[0].Interface(), toError(out[1])
		}
		return out[0].Interface(), nil
	}
}

func toError(v reflect.Value) error {
	if !v.IsValid() || (v.Kind() == reflect.Interface || v.Kind() == reflect.Ptr) && v.IsNil() {
		return nil
	}
	err, _ := v.Interface().(error)
	return err
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
