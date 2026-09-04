package co

import (
	"math"
	"reflect"
	"sync"
)

// toPromise converts a yielded value into a promise, porting index.js:116-124.
//
// The dispatch ORDER is part of the contract — in particular generator
// functions are recognised before plain functions, so a generator function is
// delegated to rather than called as a thunk.
//
//  1. falsy            -> returned unchanged
//  2. thenable         -> returned unchanged
//  3. generator (fn)   -> delegated via CoCtx
//  4. *Wrapped         -> called, yielding its promise
//  5. thunk            -> thunkToPromise
//  6. slice/array      -> arrayToPromise  (parallel)
//  7. object/map       -> objectToPromise (parallel)
//  8. anything else    -> returned unchanged, which raises the TypeError
func toPromise(ctx any, obj any) any {
	if isFalsy(obj) {
		return obj
	}
	if _, ok := asThenable(obj); ok {
		return obj
	}
	if isGeneratorLike(obj) {
		return CoCtx(ctx, obj)
	}
	if w, ok := obj.(*Wrapped); ok {
		return w.CallCtx(ctx)
	}
	if isThunkValue(obj) {
		return thunkToPromise(ctx, obj)
	}
	if arr, ok := asSlice(obj); ok {
		return arrayToPromise(ctx, arr)
	}
	if o, ok := obj.(*Object); ok {
		return objectToPromise(ctx, o)
	}
	if m, ok := asStringMap(obj); ok {
		return mapToPromise(ctx, m)
	}
	return obj
}

// asStringMap exposes any map with string keys as map[string]any, so that
// map[string]int is as yieldable as map[string]any — matching JavaScript,
// where the value type of an object literal is irrelevant.
//
// The result is always map[string]any because index.js:168 builds the output
// with `new obj.constructor()`, and isObject (index.js:238) only admits plain
// objects, so the output shape is always a plain object.
func asStringMap(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, false
	}

	out := make(map[string]any, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		out[iter.Key().String()] = iter.Value().Interface()
	}
	return out, true
}

// arrayToPromise ports index.js:154-156: every element is converted and the
// whole slice resolves in parallel, preserving order and failing fast.
func arrayToPromise(ctx any, arr []any) *Promise {
	converted := make([]any, len(arr))
	for i, v := range arr {
		converted[i] = toPromise(ctx, v)
	}
	return All(converted)
}

// objectToPromise ports index.js:167-188.
//
// Every key is written into the result in source order — pending ones with a
// nil placeholder — before any value is awaited. That pre-definition is what
// keeps key order stable when values settle out of order.
func objectToPromise(ctx any, obj *Object) *Promise {
	results := NewObject()
	var pending []any
	var mu sync.Mutex

	for _, key := range obj.Keys() {
		original, _ := obj.Get(key)
		converted := toPromise(ctx, original)

		th, ok := asThenable(converted)
		if !ok {
			mu.Lock()
			results.Set(key, original)
			mu.Unlock()
			continue
		}

		// Pre-define the key, preserving its position. Already-settled values
		// can call back on the microtask goroutine while this loop is still
		// running, so every access to results is guarded.
		mu.Lock()
		results.Set(key, nil)
		mu.Unlock()

		k := key
		pending = append(pending, th.Then(func(res any) any {
			mu.Lock()
			results.Set(k, res)
			mu.Unlock()
			return nil
		}, nil))
	}

	return All(pending).Then(func(any) any { return results }, nil)
}

// mapToPromise is objectToPromise for a plain Go map. The result is a map
// again, mirroring `new obj.constructor()` returning the same shape.
func mapToPromise(ctx any, obj map[string]any) *Promise {
	results := make(map[string]any, len(obj))
	var pending []any
	var mu sync.Mutex

	for _, key := range objectKeysOf(obj) {
		original := obj[key]
		converted := toPromise(ctx, original)

		th, ok := asThenable(converted)
		if !ok {
			mu.Lock()
			results[key] = original
			mu.Unlock()
			continue
		}

		k := key
		pending = append(pending, th.Then(func(res any) any {
			mu.Lock()
			results[k] = res
			mu.Unlock()
			return nil
		}, nil))
	}

	return All(pending).Then(func(any) any { return results }, nil)
}

// asThenable reports whether v behaves like a promise (index.js:198-200).
func asThenable(v any) (Thenable, bool) {
	if isFalsy(v) {
		return nil, false
	}
	th, ok := v.(Thenable)
	return th, ok
}

// isGeneratorLike covers both a live generator and a generator function,
// merging index.js:210-227 into one predicate.
func isGeneratorLike(v any) bool {
	if _, ok := v.(Generator); ok {
		return true
	}
	return isGeneratorFuncValue(v)
}

// isGeneratorFuncValue reports whether v is a function whose first parameter is
// a *Yielder — the Go signal that stands in for JavaScript's GeneratorFunction
// constructor check.
func isGeneratorFuncValue(v any) bool {
	if v == nil {
		return false
	}
	if _, ok := v.(GeneratorFunc); ok {
		return true
	}
	t := reflect.TypeOf(v)
	if t.Kind() != reflect.Func || t.NumIn() == 0 {
		return false
	}
	if reflect.ValueOf(v).IsNil() {
		return false
	}
	return t.In(0) == yielderType
}

var yielderType = reflect.TypeOf((*Yielder)(nil))

// asSlice exposes a slice or array as []any so it can be treated as a
// JavaScript array (Array.isArray in index.js:121).
//
// Byte slices and byte arrays are excluded: they are the Go analogue of a
// Buffer or Uint8Array, and Array.isArray rejects both, so yielding one must
// raise the invalid-yield TypeError rather than resolve to a list of numbers.
// Yield []any if you want element-wise resolution of binary-looking data.
func asSlice(v any) ([]any, bool) {
	if arr, ok := v.([]any); ok {
		return arr, true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
	default:
		return nil, false
	}
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		return nil, false
	}

	out := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

// isFalsy ports JavaScript truthiness for the values that can reach a yield.
//
// Go has no coercion, so this is an explicit table: nil, false, zero numbers,
// NaN and the empty string are falsy. Nil maps and slices are NOT treated as
// falsy, because an empty array/object is truthy in JavaScript and yielding
// one must resolve to an empty result rather than raise a TypeError.
func isFalsy(v any) bool {
	if v == nil {
		return true
	}

	switch x := v.(type) {
	case bool:
		return !x
	case string:
		return x == ""
	case float64:
		return x == 0 || math.IsNaN(x)
	case float32:
		return x == 0 || math.IsNaN(float64(x))
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() == 0
	case reflect.Ptr, reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return rv.IsNil()
	}

	return false
}
