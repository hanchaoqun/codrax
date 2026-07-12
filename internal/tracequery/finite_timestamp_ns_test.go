package tracequery

import "testing"

func TestParseLineTimestampNSUsesExactUint64Boundary(t *testing.T) {
	line := func(ts string) string {
		return "worker-7  ( 7) [001] ....  " + ts + ": tracing_mark_write: B|7|frame"
	}
	tests := []struct {
		name string
		ts   string
		want uint64
		ok   bool
	}{
		{name: "ordinary microseconds", ts: "5.000001", want: 5_000_001_000, ok: true},
		{name: "uint64 max nanosecond", ts: "18446744073.709551615", want: ^uint64(0), ok: true},
		{name: "one nanosecond over uint64", ts: "18446744073.709551616", ok: false},
		{name: "whole second overflow", ts: "18446744074", ok: false},
		{name: "sub nanosecond precision", ts: "5.0000000000", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ParseLineTimestampNS(line(test.ts))
			if ok != test.ok || got != test.want {
				t.Fatalf("ParseLineTimestampNS(%q)=(%d,%t), want (%d,%t)", test.ts, got, ok, test.want, test.ok)
			}
		})
	}
	if _, ok := ParseLineTimestampNS("payload mentions 5.000000: tracing_mark_write: B|7|not-a-header"); ok {
		t.Fatal("timestamp-looking payload text must not satisfy the anchored ftrace header")
	}
}
