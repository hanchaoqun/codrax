package types

import "testing"

// Merging occurrence rows keeps an account only when every row names it. The
// row the merge was seeded from never speaks for the others.
func TestMeasuredStateFactMergedRowsKeepOnlyAUnanimousOccurrenceAccount(t *testing.T) {
	row := func(key string) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Subject: "dep-21", StateKind: "runnable", ImpactMS: 2, RunnableMS: 2,
			MeasuredOccurrenceGroupKey: key,
		}
	}
	for _, tc := range []struct {
		name string
		keys []string
		want string
	}{
		{"every row names one account", []string{"a", " a "}, "a"},
		{"rows name different accounts", []string{"a", "b"}, ""},
		{"a later row names none", []string{"a", ""}, ""},
		{"the seed names none", []string{"", "a"}, ""},
	} {
		rows := make([]TraceCausalProjectionNode, len(tc.keys))
		for i, key := range tc.keys {
			rows[i] = row(key)
		}
		if got := TraceCausalProjectionMergeOccurrenceRows(rows).MeasuredOccurrenceGroupKey; got != tc.want {
			t.Errorf("%s: merged account=%q, want %q", tc.name, got, tc.want)
		}
	}
}
