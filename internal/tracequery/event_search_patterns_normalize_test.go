package tracequery

import (
	"reflect"
	"strings"
	"testing"
)

// NormalizeEventSearchPatterns is the one boundary both the LLM tool and the
// deterministic tracediag script call (V11-2, colleague_merge_audit §40.58).
// Error strings are asserted by the tool's own pins, so they are pinned here
// verbatim as the shared contract.
func TestNormalizeEventSearchPatternsSharedBoundary(t *testing.T) {
	if got, err := NormalizeEventSearchPatterns("window_stats", nil); got != nil || err != nil {
		t.Fatalf("absent carrier is the escape lane on every view: got=%q err=%v", got, err)
	}
	for _, view := range []string{"", "event_search", " event_search "} {
		got, err := NormalizeEventSearchPatterns(view, []string{"VerifyClass", " jit ", "verifyclass", "JIT"})
		if err != nil || !reflect.DeepEqual(got, []string{"VerifyClass", "jit"}) {
			t.Fatalf("view=%q: got=%q err=%v, want [VerifyClass jit]", view, got, err)
		}
	}
	tooMany := make([]string, EventSearchPatternLimit+1)
	for i := range tooMany {
		tooMany[i] = "l" + strings.Repeat("x", i)
	}
	for name, tc := range map[string]struct {
		view     string
		patterns []string
		want     string
	}{
		"wrong view":    {"window_stats", []string{"a"}, "patterns is only valid for view=event_search, got view=window_stats"},
		"aliased view":  {"frame_bundle", []string{"a"}, "got view=frame_root_cause_bundle"},
		"unknown view":  {"format_census", []string{"a"}, "got view=format_census"},
		"empty literal": {"event_search", []string{"a", " "}, "literal 2 is empty after trimming"},
		"over limit":    {"event_search", tooMany, "received 17 literals; maximum is 16"},
	} {
		got, err := NormalizeEventSearchPatterns(tc.view, tc.patterns)
		if err == nil || got != nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got=%q err=%v, want error containing %q", name, got, err, tc.want)
		}
	}
	if EventSearchPatternLimit != 16 {
		t.Fatalf("EventSearchPatternLimit=%d changed; the tool schema description and tracediag docs read it through the placeholder — verify both faces", EventSearchPatternLimit)
	}
}
