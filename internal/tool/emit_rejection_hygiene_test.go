package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Rejection-message hygiene: every read-pipeline structured emit tool's
// rejection Summary is model-facing text. It must not leak internal
// vocabulary (Go types, pipeline internals, validator names) — the model
// can only act on schema-level field language. The wordlist below is a
// TEST oracle; production code never routes on keywords.
func TestEmitRejectionSummaries_NoInternalJargon(t *testing.T) {
	banned := []string{
		"Mutable", "BusContext", "StageOutput", "graphState", "ViolKind",
		"AnalysisIR", "TurnAArtifacts", "EvidenceClosure", "json.Unmarshal",
		"*types.", "interface {}", "struct {",
	}
	mu := types.NewMutableState("hygiene")
	ctx := &types.BusContext{Mutable: mu}
	cases := []struct {
		name string
		tool interface {
			Execute(*types.BusContext, json.RawMessage) (types.ToolResult, error)
		}
		payload string
	}{
		{"emit_log_triage", &EmitLogTriage{}, `{"errors": 42}`},
		{"emit_perf_trace", &EmitPerfTrace{}, `{"stalls": 42}`},
		{"emit_evidence", &EmitEvidence{}, `{"items": "not-an-array"}`},
		{"emit_hypothesis_verdict", &EmitHypothesisVerdict{}, `{"items":[{"hypothesis_id":"h1","status":"confirmed","citation":"???"}]}`},
		{"emit_change_plan", &EmitChangePlan{}, `{"summary": 42}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, _ := tc.tool.Execute(ctx, json.RawMessage(tc.payload))
			if res.Success {
				t.Skipf("payload unexpectedly accepted; rejection text not exercised")
			}
			for _, word := range banned {
				if strings.Contains(res.Summary, word) {
					t.Fatalf("rejection summary leaks internal vocabulary %q: %s", word, res.Summary)
				}
			}
		})
	}
}
