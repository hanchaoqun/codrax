package tool

import (
	"reflect"
	"testing"
)

// strict_decode_census_gapr1_test.go — GAP-EVAL-R1 (eval audit 2026-07-26):
// the analyzer burned a full retry round on required_/requested_ field-name
// confusion (single unknown key → the census returned silently with no
// guidance) plus a string-encoded nested object. Both burn shapes now teach
// on the FIRST reject; the suggestion is generic (bounded edit distance
// against the reflected schema level), never a hand table.

func TestStrictDecodeNearestFieldSuggestionBounds(t *testing.T) {
	type sample struct {
		RequestedAnswerDimensions string `json:"requested_answer_dimensions"`
		Complexity                string `json:"complexity"`
	}
	fields := strictDecodeStructJSONFields(reflect.TypeOf(sample{}))
	if got := strictDecodeNearestFieldSuggestion(fields, "required_answer_dimensions"); got != "requested_answer_dimensions" {
		t.Fatalf("the R1 near-miss must suggest the real field: %q", got)
	}
	// Short keys and far keys never false-suggest (noise discipline).
	if got := strictDecodeNearestFieldSuggestion(fields, "zzzzz"); got != "" {
		t.Fatalf("short keys must not suggest: %q", got)
	}
	if got := strictDecodeNearestFieldSuggestion(fields, "totally_unrelated_field_name"); got != "" {
		t.Fatalf("far keys must not suggest: %q", got)
	}
}

func TestStrictDecodeBoundedEditDistance(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		limit int
		want  int
	}{
		{"required_answer_dimensions", "requested_answer_dimensions", 3, 3},
		{"abc", "abc", 3, 0},
		{"abc", "abd", 3, 1},
		{"abcdef", "zzzzzz", 3, -1},
	} {
		if got := strictDecodeBoundedEditDistance(tc.a, tc.b, tc.limit); got != tc.want {
			t.Fatalf("distance(%q,%q,limit=%d)=%d want %d", tc.a, tc.b, tc.limit, got, tc.want)
		}
	}
}
