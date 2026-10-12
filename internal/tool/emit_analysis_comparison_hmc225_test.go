package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC225ComparisonOptionalWindowPublicBinding(t *testing.T) {
	for _, mode := range []string{"none", "one", "partial", "invented", "pooled", "wrong_side", "implicit_full"} {
		t.Run(mode, func(t *testing.T) {
			q1, q2 := "`/captures/a.data`", "`/captures/b.data` from 4 to 4.5 seconds"
			request := "Show original records from " + q1 + " and " + q2 + "."
			a := map[string]any{"path": "/captures/a.data", "source_quote": q1}
			b := map[string]any{"path": "/captures/b.data", "source_quote": q2}
			scope := map[string]any{"requested_scope": "unspecified", "confidence": 1, "comparison": map[string]any{"baseline": a, "current": b}}
			if mode == "one" {
				scope["requested_scope"] = "explicit_time_window"
				b["time_start"], b["time_end"] = 4, 4.5
			}
			if mode == "partial" {
				b["time_start"] = 4
			}
			if mode == "invented" {
				scope["requested_scope"] = "explicit_time_window"
				a["time_start"], a["time_end"] = 0, 1
			}
			if mode == "pooled" {
				scope["time_windows"] = []any{}
			}
			if mode == "wrong_side" {
				a["source_quote"] = q2
			}
			if mode == "implicit_full" {
				scope["requested_scope"] = "full_artifact"
			}
			bus := &types.BusContext{Mutable: types.NewMutableState(request)}
			result, err := (&EmitAnalysis{}).Execute(bus, boundedScopeAuthorityPublicParams(t, scope, request))
			want := mode == "none" || mode == "one"
			if (err == nil && result.Success) != want {
				t.Fatalf("accepted=%v want=%v: %v %s", result.Success, want, err, result.Summary)
			}
			if want {
				accepted := bus.Mutable.RequestModel().RuntimeArtifactScopeProfile.Comparison
				if accepted.Baseline.HasExplicitWindow() || accepted.Baseline.TimeStart != nil || accepted.Baseline.TimeEnd != nil {
					t.Fatal("unbounded side acquired numeric endpoints")
				}
				if accepted.Current.HasExplicitWindow() != (mode == "one") {
					t.Fatal("one-side explicit scope changed")
				}
			}
		})
	}
}
