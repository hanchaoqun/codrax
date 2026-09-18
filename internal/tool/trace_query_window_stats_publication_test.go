package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQueryWindowStatsOptionPublicationScopeUsesFactoryDefault(t *testing.T) {
	const (
		path       = "/captures/window-stats-option.systrace"
		payloadRef = "/results/same.json"
		rawRef     = "/results/same.txt"
	)
	// Keep the result and both content-addressed references identical. In
	// particular, relation-scoped results can lack WindowStats regardless of
	// this option, so differing payload hashes cannot protect query identity.
	result := tracequery.Result{
		View:       "wakeup_chain",
		SourcePath: path,
		TimeStart:  1,
		TimeEnd:    2,
	}
	cases := []struct {
		name      string
		params    string
		wantStats bool
	}{
		{"omitted", `{"view":"wakeup_chain","pid":42,"time_start":1,"time_end":2}`, true},
		{"false", `{"view":"wakeup_chain","pid":42,"time_start":1,"time_end":2,"include_window_stats":false}`, false},
		{"true", `{"view":"wakeup_chain","pid":42,"time_start":1,"time_end":2,"include_window_stats":true}`, true},
	}
	scopes := make(map[string]string, len(cases))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p traceQueryParams
			if err := json.Unmarshal([]byte(tc.params), &p); err != nil {
				t.Fatal(err)
			}
			// The publication path retains this factory value: engine Run
			// normalizes its own copy, not the query later hashed by the tool.
			// Do not call Run or normalize before inspecting or hashing q.
			q := traceQueryBuildQuery(nil, p, "path", path, p.TimeStart.Seconds(), p.TimeEnd.Seconds())
			if q.IncludeWindowStats != tc.wantStats {
				t.Fatalf("factory IncludeWindowStats = %v, want %v before engine normalization", q.IncludeWindowStats, tc.wantStats)
			}
			scope := traceQueryPublicationScope(result, payloadRef, rawRef, "", q)
			if scope == "" {
				t.Fatal("factory query must have a publication scope")
			}
			if again := traceQueryPublicationScope(result, payloadRef, rawRef, "", q); again != scope {
				t.Fatalf("same query and references changed publication scope: %q != %q", again, scope)
			}
			scopes[tc.name] = scope
		})
	}
	if len(scopes) != len(cases) {
		t.Fatal("all three factory variants must reach publication")
	}
	if scopes["omitted"] != scopes["true"] {
		t.Fatalf("omitted default and explicit true must publish the same effective query: %q != %q", scopes["omitted"], scopes["true"])
	}
	if scopes["false"] == scopes["omitted"] || scopes["false"] == scopes["true"] {
		t.Fatalf("explicit false must remain distinguishable with identical result/references: %#v", scopes)
	}
}
