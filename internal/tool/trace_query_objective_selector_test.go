package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// trace_query_objective_selector_test.go — G6-tracediag #0 (colleague_merge_audit
// §40.58 合流复核收编): the objective-exact-token caveat reads the call's full
// literal OR set (pattern ∪ patterns) through one selector helper. Before this
// pin the caveat built its selector from pattern/span_name only, so a call
// whose `patterns` already carried the requested token was told
// `selector "" does not include requested token "173073"` and instructed to
// rerun with the literal it had just sent.

func objectiveSelectorFrameTrace(t *testing.T, withRequested bool) (dir, name string) {
	t.Helper()
	dir = t.TempDir()
	name = "frame_selector.systrace"
	lines := []string{
		`app-20 (20) [001] .... 1.100000: print: B|20|Choreographer#doFrame 170048`,
		`app-20 (20) [001] .... 1.120000: print: E|20`,
	}
	if withRequested {
		lines = append(lines,
			`app-20 (20) [001] .... 2.100000: print: B|20|Choreographer#doFrame 173073`,
			`app-20 (20) [001] .... 2.120000: print: E|20`,
		)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, name
}

func objectiveSelectorExecute(t *testing.T, dir, name string, params map[string]any) types.ToolResult {
	t.Helper()
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		Mutable:  types.NewMutableState(`分析 Choreographer#doFrame 173073 这一帧丢帧的深层次原因`),
	}
	base := map[string]any{"source": "path", "path": name, "view": "event_search"}
	for k, v := range params {
		base[k] = v
	}
	raw, _ := json.Marshal(base)
	res, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// patterns-only: the requested token is in the OR set, the limit cuts its
// row — no caveat may claim the selector omitted it.
func TestTraceQueryObjectiveTokenCaveatReadsPatternsOnlySelector(t *testing.T) {
	dir, name := objectiveSelectorFrameTrace(t, true)
	res := objectiveSelectorExecute(t, dir, name, map[string]any{
		"patterns": []string{"Choreographer#doFrame", "173073"},
		"limit":    1,
	})
	if strings.Contains(res.Summary, "objective_exact_frame_hint") {
		t.Fatalf("patterns-only call carrying the requested token must not be told its selector omitted it:\n%s", res.Summary)
	}
	if strings.Contains(res.Summary, `selector ""`) {
		t.Fatalf("caveat rendered an empty selector for a patterns call:\n%s", res.Summary)
	}
}

// pattern+patterns: the token rides in `patterns` next to a broad `pattern`;
// the union is the selector.
func TestTraceQueryObjectiveTokenCaveatReadsPatternPlusPatternsSelector(t *testing.T) {
	dir, name := objectiveSelectorFrameTrace(t, true)
	res := objectiveSelectorExecute(t, dir, name, map[string]any{
		"pattern":  "Choreographer#doFrame",
		"patterns": []string{"173073"},
		"limit":    1,
	})
	if strings.Contains(res.Summary, "objective_exact_frame_hint") {
		t.Fatalf("pattern+patterns call carrying the requested token must not be told its selector omitted it:\n%s", res.Summary)
	}
}

// genuine absence: the OR set really omits the token — the caveat fires,
// names the `patterns` carrier and renders the full set it judged, and the
// same holds with the union spelled as pattern+patterns.
func TestTraceQueryObjectiveTokenCaveatNamesFullSetOnGenuineAbsence(t *testing.T) {
	dir, name := objectiveSelectorFrameTrace(t, false)
	res := objectiveSelectorExecute(t, dir, name, map[string]any{
		"patterns": []string{"Choreographer#doFrame", "VerifyClass"},
		"limit":    50,
	})
	for _, want := range []string{
		"objective_exact_frame_hint",
		`this event_search patterns ["Choreographer#doFrame","VerifyClass"] does not include requested token "173073"`,
		`trace_query(view="frame_window", pattern="173073"`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("genuine absence caveat missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, `selector ""`) {
		t.Fatalf("caveat rendered an empty selector for a patterns call:\n%s", res.Summary)
	}
	res = objectiveSelectorExecute(t, dir, name, map[string]any{
		"pattern":  "Choreographer#doFrame",
		"patterns": []string{"VerifyClass"},
		"limit":    50,
	})
	if !strings.Contains(res.Summary, `this event_search pattern+patterns ["Choreographer#doFrame","VerifyClass"] does not include requested token "173073"`) {
		t.Fatalf("pattern+patterns absence caveat must render the union:\n%s", res.Summary)
	}
}

// The selector helper itself: one reader, every carrier (pattern, patterns,
// span_name — a token sent through any of them is a sent selector), quoted
// text matches the pinned single-pattern wording.
func TestTraceQueryObjectiveSelectorHelper(t *testing.T) {
	for _, tc := range []struct {
		name     string
		params   traceQueryParams
		wantName string
		wantText string
		wantSet  []string
	}{
		{"none", traceQueryParams{}, "selector", `""`, nil},
		{"pattern", traceQueryParams{Pattern: " Choreographer#doFrame "}, "pattern", `"Choreographer#doFrame"`, []string{"Choreographer#doFrame"}},
		{"span", traceQueryParams{SpanName: "DecodeBitmap"}, "span_name", `"DecodeBitmap"`, []string{"DecodeBitmap"}},
		{"patterns_one", traceQueryParams{Patterns: []string{"173073"}}, "patterns", `["173073"]`, []string{"173073"}},
		{"patterns", traceQueryParams{Patterns: []string{"a", " b "}}, "patterns", `["a","b"]`, []string{"a", "b"}},
		{"union", traceQueryParams{Pattern: "a", Patterns: []string{"b"}}, "pattern+patterns", `["a","b"]`, []string{"a", "b"}},
		{"patterns_with_span", traceQueryParams{SpanName: "x", Patterns: []string{"b"}}, "patterns+span_name", `["b","x"]`, []string{"b", "x"}},
		{"pattern_with_span", traceQueryParams{Pattern: "a", SpanName: "x"}, "pattern+span_name", `["a","x"]`, []string{"a", "x"}},
		{"all_carriers", traceQueryParams{Pattern: "a", Patterns: []string{"b"}, SpanName: "x"}, "pattern+patterns+span_name", `["a","b","x"]`, []string{"a", "b", "x"}},
	} {
		name, text, set := traceQueryObjectiveSelector(tc.params)
		if name != tc.wantName || text != tc.wantText || strings.Join(set, "|") != strings.Join(tc.wantSet, "|") {
			t.Errorf("%s: got (%q, %s, %q), want (%q, %s, %q)", tc.name, name, text, set, tc.wantName, tc.wantText, tc.wantSet)
		}
	}
	if !traceQuerySelectorSetContainsObjectiveToken([]string{"Choreographer#doFrame", "173073"}, "173073") {
		t.Fatal("token carried in the set must count as selected")
	}
	if traceQuerySelectorSetContainsObjectiveToken(nil, "173073") || traceQuerySelectorSetContainsObjectiveToken([]string{"doFrame"}, "173073") {
		t.Fatal("absent token must not count as selected")
	}
}
