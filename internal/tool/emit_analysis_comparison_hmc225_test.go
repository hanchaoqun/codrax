package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
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
			want := mode == "none" || mode == "one" || mode == "implicit_full"
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

func TestHMC225ComparisonFullScopeQuoteAuthority(t *testing.T) {
	const a, b = "/captures/a.data", "/captures/b.data"
	const qa, qb = "`/captures/a.data`", "`/captures/b.data`"
	const request = "Show all original records from " + qa + " and " + qb + "."
	for _, mode := range []string{"missing", "empty", "unanchored", "anchored", "wrong_local_quote", "same_path", "pooled", "scalar", "partial", "explicit_without_windows", "full_with_window"} {
		t.Run(mode, func(t *testing.T) {
			currentRequest := request
			baseline := map[string]any{"path": a, "source_quote": qa}
			current := map[string]any{"path": b, "source_quote": qb}
			scope := map[string]any{"requested_scope": "full_artifact", "confidence": 0.91, "rationale": "independent original records", "comparison": map[string]any{"baseline": baseline, "current": current}}
			switch mode {
			case "empty":
				scope["source_quote"] = ""
			case "unanchored":
				scope["source_quote"] = "the entire captures including every event"
			case "anchored":
				scope["source_quote"] = request
			case "wrong_local_quote":
				baseline["source_quote"] = qb
			case "same_path":
				current["path"] = a
			case "pooled":
				scope["time_windows"] = []any{}
			case "scalar":
				scope["time_start"], scope["time_end"] = 1, 2
			case "partial":
				baseline["time_start"] = 1
			case "explicit_without_windows":
				scope["requested_scope"] = "explicit_time_window"
			case "full_with_window":
				baseline["time_start"], baseline["time_end"] = 1, 2
				baseline["source_quote"] = qa + " from 1 to 2 seconds"
				currentRequest = "Show records from " + qa + " from 1 to 2 seconds and " + qb + "."
			}
			bus := &types.BusContext{Mutable: types.NewMutableState(currentRequest)}
			result, err := (&EmitAnalysis{}).Execute(bus, boundedScopeAuthorityPublicParams(t, scope, currentRequest))
			want := mode == "missing" || mode == "empty" || mode == "unanchored" || mode == "anchored"
			if err != nil || result.Success != want {
				t.Fatalf("accepted=%v want=%v: %v %s", result.Success, want, err, result.Summary)
			}
			if !want {
				if !strings.Contains(result.Summary, "preserving valid comparison members") {
					t.Fatal("hard error lost the valid source-binding repair path")
				}
				return
			}
			accepted := bus.Mutable.RequestModel().RuntimeArtifactScopeProfile
			if accepted.Comparison.Baseline.Path != a || accepted.Comparison.Current.Path != b || accepted.Comparison.Baseline.SourceQuote != qa || accepted.Comparison.Current.SourceQuote != qb || accepted.Confidence != 0.91 || accepted.Rationale != "independent original records" {
				t.Fatalf("repair changed valid sibling fields: %+v", accepted)
			}
			if accepted.HasExplicitTimeWindows() || len(accepted.TimeWindows) != 0 || accepted.TimeStart != nil || accepted.TimeEnd != nil {
				t.Fatal("full-scope repair invented numeric authority")
			}
			if mode == "anchored" {
				if accepted.RequestedScope != types.RuntimeArtifactScopeFullArtifact || accepted.SourceQuote != request {
					t.Fatal("valid whole-artifact scope was lost")
				}
			} else if accepted.RequestedScope != types.RuntimeArtifactScopeUnspecified || accepted.SourceQuote != "" || !strings.Contains(result.Summary, "runtime_artifact_scope_profile.source_quote") || !strings.Contains(result.Summary, "comparison.baseline/current") {
				t.Fatalf("missing precise leaf normalization with retained comparison: %+v / %s", accepted, result.Summary)
			}
		})
	}
}

func TestHMC225ArtifactScopeSchemaSharesSingleAndDualSourceTeaching(t *testing.T) {
	var schema map[string]any
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	profile := schema["properties"].(map[string]any)["runtime_artifact_scope_profile"].(map[string]any)
	if !strings.Contains(profile["description"].(string), skill.AnalysisRuntimeArtifactScopeTeaching) {
		t.Fatal("schema lost shared single-/dual-source branch selection")
	}
	comparisonTeaching, _ := json.Marshal(skill.AnalysisRuntimeComparisonTeaching)
	if strings.Count(string(raw), strings.Trim(string(comparisonTeaching), "\"")) != 1 {
		t.Fatal("schema repeats the full comparison teaching across parent and child descriptions")
	}
	properties := profile["properties"].(map[string]any)
	if !strings.Contains(properties["time_windows"].(map[string]any)["description"].(string), "Without comparison") {
		t.Fatal("pooled window schema omitted its single-capture boundary")
	}
}
