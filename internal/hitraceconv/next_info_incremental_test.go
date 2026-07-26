package hitraceconv

import "testing"

// next_info_incremental_test.go — NEXTINFO P1 硬伤C pins (2026-07-25): the
// customer doc declares next_info an INCREMENTAL format. The string-typed
// producer lane must not drop the whole next_info token on the first field
// the converter doesn't know; validated decimal tails pass through verbatim.

func TestCanonicalHarmonySchedInfoTextIncrementalTail(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		// Every kernel version is accepted from the same stable five-field
		// prefix; fields are appended in order and preserved.
		{"f,10,2,1,3", "f,10,2,1,3"},
		{"f,10,2,1,3,17", "f,10,2,1,3,17"},
		{"f,10,2,1,3,17,4", "f,10,2,1,3,17,4"},
		{"e,166,3,0,0,1,42,7", "e,166,3,0,0,1,42,7"},
		{"e,166,3,0,0,1,42,7,9", "e,166,3,0,0,1,42,7,9"},
		// Short payloads and malformed tails still fail closed.
		{"f,10,2,1", ""},
		{"f,10,2,1,3,", ""},
		{"f,10,2,1,3,17,x", ""},
		{"f,10,2,1,3,17, 4", ""},
	} {
		if got := canonicalHarmonySchedInfoText(tc.raw); got != tc.want {
			t.Fatalf("canonicalHarmonySchedInfoText(%q) = %q, want %q",
				tc.raw, got, tc.want)
		}
	}
}
