package co

import (
	"errors"
	"math"
	"testing"
	"time"
)

// Every expectation below is the literal output of String(v) in Node, captured
// by running the same values through V8. jsNumber must reproduce them exactly,
// because they feed the invalid-yield message that tj/co exposes to callers.
func TestJSNumberMatchesV8(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1e6, "1000000"},
		{123456789, "123456789"},
		{1e20, "100000000000000000000"},
		{1e21, "1e+21"},
		{1e-6, "0.000001"},
		{1e-7, "1e-7"},
		{0.1, "0.1"},
		{123.456, "123.456"},
		{-1.5, "-1.5"},
		{math.Copysign(0, -1), "0"},
		{5e-324, "5e-324"},
		{1e-323, "1e-323"},
		{math.MaxFloat64, "1.7976931348623157e+308"},
		{1.5, "1.5"},
		{42, "42"},
		{0, "0"},
		{9007199254740993, "9007199254740992"},
		{0.000001234, "0.000001234"},
		{1234567890123456789012, "1.2345678901234568e+21"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
	}

	for _, tc := range cases {
		if got := jsNumber(tc.in); got != tc.want {
			t.Errorf("jsNumber(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// stringerOnly is a fmt.Stringer that is not an error, so it exercises the
// Stringer branch of jsString rather than the error branch above it.
type stringerOnly struct{}

func (stringerOnly) String() string { return "stringer" }

// TestJSStringCoercesEveryKind walks every branch of jsString. The expectations
// are what JavaScript's String() produces for the nearest equivalent value.
func TestJSStringCoercesEveryKind(t *testing.T) {
	var nilPtr *stringerOnly
	var nilIface any

	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, "null"},
		{"nil interface", nilIface, "null"},
		{"nil pointer", nilPtr, "null"},
		{"string", "hi", "hi"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"type error", &TypeError{Message: "bad"}, "TypeError: bad"},
		{"rejection of a value", &Rejection{Value: 7}, "7"},
		{"rejection of an error", &Rejection{Value: errors.New("nope")}, "Error: nope"},
		{"error", errors.New("boom"), "Error: boom"},
		{"stringer", stringerOnly{}, "stringer"},
		{"duration", 2 * time.Second, "2s"},
		{"int", 42, "42"},
		{"negative int", int8(-3), "-3"},
		{"uint", uint16(7), "7"},
		{"float", 1.5, "1.5"},
		{"float32", float32(0.5), "0.5"},
		{"byte slice", []byte("hi"), "hi"},
		{"byte array", [2]byte{'h', 'i'}, "hi"},
		{"slice", []any{1, "a", true}, "1,a,true"},
		{"slice with a hole", []any{nil, 1}, ",1"},
		{"array", [2]int{1, 2}, "1,2"},
		{"nested slice", []any{[]any{1, 2}, 3}, "1,2,3"},
		{"map", map[string]any{"a": 1}, "[object Object]"},
		{"struct", struct{ A int }{1}, "[object Object]"},
		{"pointer", &[]any{1}, "1"},
		{"func", func() {}, "function"},
		{"channel", make(chan int), ""},
	}

	for _, tc := range cases {
		got := jsString(tc.in)
		if tc.name == "channel" {
			// A channel has no JavaScript counterpart; only the fallback path
			// matters, so assert it produced something rather than panicking.
			if got == "" {
				t.Errorf("jsString(chan) = %q, want a non-empty fallback", got)
			}
			continue
		}
		if got != tc.want {
			t.Errorf("jsString(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestReasonPassesErrorsThrough keeps Reason from double-wrapping: an error
// reason is already an error and must survive untouched.
func TestReasonPassesErrorsThrough(t *testing.T) {
	inner := errors.New("inner")
	if got := Reason(inner); got != inner {
		t.Errorf("Reason(err) = %v, want the same error back", got)
	}
	if Reason(nil) == nil {
		t.Errorf("Reason(nil) = nil, want a rejection carrying the nil reason")
	}

	wrapped := Reason(1)
	var rejection *Rejection
	if !errors.As(wrapped, &rejection) {
		t.Fatalf("Reason(1) is %T, want *Rejection", wrapped)
	}
	if rejection.Value != 1 {
		t.Errorf("Value = %v, want 1", rejection.Value)
	}
	if rejection.Unwrap() != nil {
		t.Errorf("Unwrap() = %v, want nil", rejection.Unwrap())
	}

	nested := Reason(inner)
	if !errors.Is(nested, inner) {
		t.Errorf("errors.Is(Reason(inner), inner) = false, want true")
	}
}
