package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC224ComparisonScopePublicBinding(t *testing.T) {
	const request = "Compare `/captures/a.data` from 1 to 2 seconds and `/captures/b.data` from 4 to 4.5 seconds."
	ctx := &types.BusContext{Mutable: types.NewMutableState(request)}
	scope := map[string]any{"requested_scope": "explicit_time_window", "confidence": 1, "comparison": map[string]any{
		"baseline": map[string]any{"path": "/captures/a.data", "time_start": 1, "time_end": 2, "source_quote": "`/captures/a.data` from 1 to 2 seconds"},
		"current":  map[string]any{"path": "/captures/b.data", "time_start": 4, "time_end": 4.5, "source_quote": "`/captures/b.data` from 4 to 4.5 seconds"},
	}}
	out, err := (&EmitAnalysis{}).Execute(ctx, boundedScopeAuthorityPublicParams(t, scope, request))
	if err != nil || !out.Success {
		t.Fatalf("independent source-owned windows were not accepted: %s %v", out.Summary, err)
	}
	raw, err := json.Marshal(ctx.Mutable.RequestModel().RuntimeArtifactScopeProfile)
	if err != nil {
		t.Fatal(err)
	}
	var retained map[string]json.RawMessage
	if err := json.Unmarshal(raw, &retained); err != nil {
		t.Fatal(err)
	}
	if len(retained["comparison"]) == 0 {
		t.Fatal("accepted source roles were dropped from the actual request model")
	}
}

func TestHMC224ComparisonScopeExactPathAndJSON(t *testing.T) {
	for _, mutation := range []string{"prefix_overlap", "wrong_side_quote", "mixed_windows", "duplicate", "unknown_field"} {
		t.Run(mutation, func(t *testing.T) {
			const first = "/captures/a.data"
			const second = "/captures/a.data.backup"
			q1, q2 := "`"+first+"` from 1 to 2 seconds", "`"+second+"` from 4 to 4.5 seconds"
			request := "Compare " + q1 + " and " + q2 + "."
			scope := map[string]any{"requested_scope": "explicit_time_window", "confidence": 1, "comparison": map[string]any{
				"baseline": map[string]any{"path": first, "time_start": 1, "time_end": 2, "source_quote": q1},
				"current":  map[string]any{"path": second, "time_start": 4, "time_end": 4.5, "source_quote": q2},
			}}
			if mutation == "mixed_windows" {
				scope["time_windows"] = []any{}
			}
			if mutation == "wrong_side_quote" {
				scope["comparison"].(map[string]any)["baseline"].(map[string]any)["source_quote"] = q2
			}
			raw := boundedScopeAuthorityPublicParams(t, scope, request)
			if mutation == "duplicate" {
				raw = json.RawMessage(strings.Replace(string(raw), `"path":"`+first+`"`, `"path":"`+first+`","path":"`+second+`"`, 1))
			}
			if mutation == "unknown_field" {
				raw = json.RawMessage(strings.Replace(string(raw), `"path":"`+first+`"`, `"path":"`+first+`","device":"same"`, 1))
			}
			out, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: types.NewMutableState(request)}, raw)
			accepted := err == nil && out.Success
			if accepted != (mutation == "prefix_overlap") {
				t.Fatalf("accepted=%v err=%v out=%+v", accepted, err, out)
			}
		})
	}
}
