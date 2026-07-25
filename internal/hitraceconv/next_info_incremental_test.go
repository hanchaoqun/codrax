package hitraceconv

import "testing"

// next_info_incremental_test.go — NEXTINFO P1 硬伤C pins (2026-07-25): the
// customer doc declares next_info an INCREMENTAL format. The string-typed
// producer lane must not drop the whole next_info token on the first field
// the converter doesn't know; validated decimal tails pass through verbatim.

func TestCanonicalHarmonySchedInfoTextIncrementalTail(t *testing.T) {
	for _, tc := range []struct {
		raw         string
		includeCGID bool
		want        string
	}{
		// Exact known widths keep their bytes.
		{"f,10,2,1,3", false, "f,10,2,1,3"},
		{"f,10,2,1,3,17", true, "f,10,2,1,3,17"},
		// Incremental tails survive verbatim past the validated prefix.
		{"f,10,2,1,3,17,4", true, "f,10,2,1,3,17,4"},
		{"e,166,3,0,0,1,42,7", true, "e,166,3,0,0,1,42,7"},
		{"f,10,2,1,3,9", false, "f,10,2,1,3,9"},
		// Short payloads and malformed tails still fail closed.
		{"f,10,2,1", false, ""},
		{"f,10,2,1,3,", true, ""},
		{"f,10,2,1,3,17,x", true, ""},
		{"f,10,2,1,3,17, 4", true, ""},
	} {
		if got := canonicalHarmonySchedInfoText(tc.raw, tc.includeCGID); got != tc.want {
			t.Fatalf("canonicalHarmonySchedInfoText(%q, cgid=%v) = %q, want %q",
				tc.raw, tc.includeCGID, got, tc.want)
		}
	}
}
