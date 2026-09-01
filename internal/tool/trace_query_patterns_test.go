package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryPatternsTypedAlternativeCarrier(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.systrace")
	trace := "app-20 (20) [001] .... 1.000000: tracing_mark_write: B|20|VerifyClass Demo\n" +
		"app-20 (20) [001] .... 1.001000: tracing_mark_write: E|20\n" +
		"jit-30 (30) [002] .... 1.002000: tracing_mark_write: B|30|JIT compile Demo\n"
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"source":"path","path":"multi.systrace","view":"event_search","patterns":["VerifyClass","JIT"],"event_types":["trace_mark"],"limit":10}`))
	if err != nil || !res.Success {
		t.Fatalf("typed patterns call failed: err=%v result=%+v", err, res)
	}
	for _, want := range []string{"patterns=VerifyClass,JIT", "matched_total=2"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryPatternsValidation(t *testing.T) {
	tool := &TraceQuery{}
	for _, tc := range []struct {
		name    string
		payload string
		want    string
	}{
		{"wrong view", `{"view":"window_stats","patterns":["VerifyClass"]}`, "only valid for view=event_search"},
		{"empty literal", `{"view":"event_search","patterns":[" "]}`, "empty after trimming"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Execute(&types.BusContext{}, json.RawMessage(tc.payload))
			if err != nil {
				t.Fatal(err)
			}
			if res.Success || !strings.Contains(res.Summary, tc.want) {
				t.Fatalf("validation mismatch: %+v", res)
			}
		})
	}
}

func TestTraceQueryPatternDescriptionKeepsPipeLiteral(t *testing.T) {
	schema := string((&TraceQuery{}).Parameters())
	for _, want := range []string{"\"patterns\"", "combined with OR", "'|' remains an ordinary literal character"} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema missing %q", want)
		}
	}
}
