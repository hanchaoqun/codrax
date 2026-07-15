package tracewire

import (
	"strings"
	"testing"
)

func TestPerfKVCanonicalQuoteRoundTripCannotReopenKeys(t *testing.T) {
	values := []string{
		`Hot" tid=999 cpu=7 sample_weight=999`,
		`C:\Program Files\鸿蒙\libfoo.dll`,
		"类验证\t阶段\n下一层",
		`"edge quotes"`,
	}
	for _, value := range values {
		wire := "pid=12 tid=34 cpu=5 sample_weight=11 symbol=" + QuotePerfKVValue(value)
		fields, err := ParsePerfKV(wire)
		if err != nil {
			t.Fatalf("ParsePerfKV(%q): %v", value, err)
		}
		if len(fields) != 5 {
			t.Fatalf("quoted metadata reopened a key boundary: fields=%+v", fields)
		}
		if got := fields[4]; got.Key != "symbol" || got.Value != strings.TrimSpace(value) || !got.Quoted {
			t.Fatalf("round-trip drift for %q: %+v", value, got)
		}
	}
}

func TestPerfKVRejectsUnknownBoundariesWithoutResynchronizing(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		reason string
	}{
		{name: "invalid escape", body: `pid=1 symbol="bad\q" tid=2`, reason: "invalid_escape"},
		{name: "unclosed quote", body: `pid=1 symbol="bad tid=2`, reason: "unclosed_quote"},
		{name: "quote tail", body: `pid=1 symbol="ok"x tid=2`, reason: "trailing_bytes_after_value"},
		{name: "naked token", body: `pid=1 orphan tid=2`, reason: "missing_equals"},
		{name: "bare quote", body: `pid=1 symbol=bad"tail tid=2`, reason: "quote_in_bare_value"},
		{name: "raw newline", body: "pid=1 symbol=\"bad\nvalue\" tid=2", reason: "raw_line_break_in_literal"},
		{name: "single quote escaped newline", body: "pid=1 symbol='bad\\\nvalue' tid=2", reason: "raw_line_break_in_literal"},
		{name: "bare raw control", body: "pid=1 symbol=bad\x00value tid=2", reason: "raw_control_in_value"},
		{name: "single quote raw control", body: "pid=1 symbol='bad\tvalue' tid=2", reason: "raw_control_in_literal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields, err := ParsePerfKV(tc.body)
			if err == nil || err.Reason != tc.reason {
				t.Fatalf("fields=%+v err=%+v want reason=%s", fields, err, tc.reason)
			}
			if fields != nil {
				t.Fatalf("malformed body exposed a trusted prefix: %+v", fields)
			}
		})
	}
}

func TestPerfKVBoundsAndLegacySingleQuote(t *testing.T) {
	fields, err := ParsePerfKV(`symbol='it\'s \\ exact' pid=1`)
	if err != nil || len(fields) != 2 || fields[0].Value != `it's \ exact` {
		t.Fatalf("bounded legacy single quote drift: fields=%+v err=%v", fields, err)
	}
	if _, err := ParsePerfKV(strings.Repeat("x", MaxPerfKVBodyBytes+1)); err == nil || err.Reason != "body_too_long" {
		t.Fatalf("body budget not enforced: %v", err)
	}
	if _, err := ParsePerfKV("symbol=\"" + strings.Repeat("x", MaxPerfKVEncodedValueBytes) + "\""); err == nil || err.Reason != "value_too_long" {
		t.Fatalf("value budget not enforced: %v", err)
	}
	if _, err := ParsePerfKV(strings.Repeat("k", MaxPerfKVKeyBytes+1) + "=1"); err == nil || err.Reason != "key_too_long" || err.Field != "" {
		t.Fatalf("key budget not enforced without retaining hostile key: %v", err)
	}
	parts := make([]string, MaxPerfKVFields+1)
	for i := range parts {
		parts[i] = "k=1"
	}
	if _, err := ParsePerfKV(strings.Join(parts, " ")); err == nil || err.Reason != "field_count_exceeded" {
		t.Fatalf("field budget not enforced: %v", err)
	}
}
