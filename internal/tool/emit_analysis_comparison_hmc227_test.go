package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC227ComparisonSharedWindowPublicBinding(t *testing.T) {
	for _, mode := range []string{"seconds", "milliseconds", "mixed", "partial", "forged_quote", "missing_quote", "pooled", "wrong_scope", "local_quote_forged"} {
		t.Run(mode, func(t *testing.T) {
			qa, qb, interval := "`/captures/a.data`", "`/captures/b.data`", "1–2 seconds"
			if mode == "milliseconds" {
				interval = "1000–2000 ms"
			}
			request := "Compare the original records in " + qa + " and " + qb + " during " + interval + "."
			a := map[string]any{"path": "/captures/a.data", "source_quote": qa}
			b := map[string]any{"path": "/captures/b.data", "source_quote": qb}
			scope := map[string]any{"requested_scope": "explicit_time_window", "time_start": 1, "time_end": 2, "source_quote": interval, "confidence": 1, "comparison": map[string]any{"baseline": a, "current": b}}
			switch mode {
			case "milliseconds":
				scope["time_start"], scope["time_end"] = 1000, 2000
			case "mixed":
				a["time_start"], a["time_end"] = 1, 2
			case "partial":
				delete(scope, "time_end")
			case "forged_quote":
				scope["source_quote"] = "3–4 seconds"
			case "missing_quote":
				delete(scope, "source_quote")
			case "pooled":
				scope["time_windows"] = []any{}
			case "wrong_scope":
				scope["requested_scope"] = "full_artifact"
			case "local_quote_forged":
				a["source_quote"] = qa + " during " + interval
			}
			bus := &types.BusContext{Mutable: types.NewMutableState(request)}
			result, err := (&EmitAnalysis{}).Execute(bus, boundedScopeAuthorityPublicParams(t, scope, request))
			want := mode == "seconds" || mode == "milliseconds"
			if err != nil || result.Success != want {
				t.Fatalf("accepted=%v want=%v: %v %s", result.Success, want, err, result.Summary)
			}
			if !want {
				return
			}
			accepted := bus.Mutable.RequestModel().RuntimeArtifactScopeProfile
			if accepted.TimeStart == nil || accepted.TimeEnd == nil || *accepted.TimeStart != 1 || *accepted.TimeEnd != 2 || accepted.TimeWindows != nil || accepted.SourceQuote != interval {
				t.Fatalf("shared declaration was not preserved and normalized: %+v", accepted)
			}
			for _, side := range accepted.Comparison.Sides() {
				if side.HasExplicitWindow() || side.TimeStart != nil || side.TimeEnd != nil || side.SourceQuote != "`"+side.Path+"`" {
					t.Fatalf("common interval was copied into fabricated local evidence: %+v", side)
				}
				projected, ok := types.RuntimeArtifactDisplayScopeForOriginal(accepted, side.Path, "")
				if !ok || !projected.ContainsExplicitTimeWindow(1, 2) || accepted.ComparisonWindowAllows(side.Path, "", 0, 3) {
					t.Fatalf("effective source window was lost or widened: %+v", projected)
				}
			}
		})
	}
}
