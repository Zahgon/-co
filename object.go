package co

import (
	"sort"
	"strconv"
)

// Object is an insertion-ordered string-keyed map: the port of a JavaScript
// plain object.
//
// Ordering is not cosmetic. index.js:181-187 pre-defines every key in the
// result BEFORE awaiting it, precisely so that a slow value cannot reorder the
// output. A Go map cannot express that guarantee, so parallel object yields use
// this type instead.
type Object struct {
	keys   []string
	values map[string]any
}

// NewObject returns an empty Object.
func NewObject() *Object {
	return &Object{values: make(map[string]any)}
}

// ObjectOf builds an Object from alternating key/value pairs, panicking on a
// malformed argument list. It exists to keep literals readable.
func ObjectOf(pairs ...any) *Object {
	if len(pairs)%2 != 0 {
		panic("co.ObjectOf: odd number of arguments")
	}
	o := NewObject()
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			panic("co.ObjectOf: keys must be strings")
		}
		o.Set(key, pairs[i+1])
	}
	return o
}

// Set assigns a key, preserving first-insertion position on overwrite. It
// returns the receiver so calls can be chained.
func (o *Object) Set(key string, value any) *Object {
	if o.values == nil {
		o.values = make(map[string]any)
	}
	if _, exists := o.values[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
	return o
}

// Get returns the value stored at key.
func (o *Object) Get(key string) (any, bool) {
	if o == nil || o.values == nil {
		return nil, false
	}
	v, ok := o.values[key]
	return v, ok
}

// Len returns the number of keys.
func (o *Object) Len() int {
	if o == nil {
		return 0
	}
	return len(o.keys)
}

// Keys returns the keys in JavaScript's Object.keys() order: canonical array
// indices ascending first, then the remaining keys in insertion order.
func (o *Object) Keys() []string {
	if o == nil {
		return nil
	}

	var indices []string
	var named []string
	for _, k := range o.keys {
		if isArrayIndex(k) {
			indices = append(indices, k)
		} else {
			named = append(named, k)
		}
	}

	sort.Slice(indices, func(i, j int) bool {
		a, _ := strconv.ParseUint(indices[i], 10, 64)
		b, _ := strconv.ParseUint(indices[j], 10, 64)
		return a < b
	})

	return append(indices, named...)
}

// Map copies the contents into a plain Go map, discarding order.
func (o *Object) Map() map[string]any {
	out := make(map[string]any, o.Len())
	for _, k := range o.Keys() {
		out[k] = o.values[k]
	}
	return out
}

// isArrayIndex reports whether key is a canonical array index string, the rule
// JavaScript uses to decide which keys sort first in Object.keys().
func isArrayIndex(key string) bool {
	if key == "" || len(key) > 10 {
		return false
	}
	if key != "0" && key[0] == '0' {
		return false // non-canonical, e.g. "01"
	}
	n, err := strconv.ParseUint(key, 10, 64)
	if err != nil {
		return false
	}
	return n < 4294967295
}

// objectKeysOf returns a plain map's keys in the same order rule. Go maps have
// no insertion order, so non-index keys are sorted lexicographically to keep
// results deterministic.
func objectKeysOf(m map[string]any) []string {
	var indices []string
	var rest []string
	for k := range m {
		if isArrayIndex(k) {
			indices = append(indices, k)
		} else {
			rest = append(rest, k)
		}
	}
	sort.Slice(indices, func(i, j int) bool {
		a, _ := strconv.ParseUint(indices[i], 10, 64)
		b, _ := strconv.ParseUint(indices[j], 10, 64)
		return a < b
	})
	sort.Strings(rest)
	return append(indices, rest...)
}
