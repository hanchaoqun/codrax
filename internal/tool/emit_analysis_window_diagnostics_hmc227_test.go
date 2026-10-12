package tool

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC227WindowDeclarationDiagnosticsIdentifyExactField(t *testing.T) {
	for _, form := range []string{"scalar", "members", "shared", "side"} {
		for _, problem := range []string{"source_quote", "endpoints"} {
			t.Run(form+"/"+problem, func(t *testing.T) {
				const request = "Compare `/a.data` during 1..2 seconds and `/b.data`, including OpenDocument."
				member := map[string]any{"time_start": 1, "time_end": 2, "source_quote": "1..2 seconds"}
				field := "runtime_artifact_scope_profile"
				if form == "side" {
					member["path"], member["source_quote"] = "/a.data", "`/a.data` during 1..2 seconds"
					field += ".comparison.baseline"
				}
				validQuote := member["source_quote"]
				if problem == "source_quote" {
					member["source_quote"] = "0.999–1.051 seconds"
				} else {
					member["time_end"] = 0
				}
				scope := map[string]any{"requested_scope": "explicit_time_window", "confidence": 1}
				leaf := member
				switch form {
				case "scalar", "shared":
					for key, value := range member {
						scope[key] = value
					}
					leaf = scope
				case "members":
					scope["time_windows"] = []any{member}
					field += ".time_windows[0]"
				}
				if form == "shared" || form == "side" {
					first := map[string]any{"path": "/a.data", "source_quote": "`/a.data`"}
					if form == "side" {
						first = member
					}
					scope["comparison"] = map[string]any{"baseline": first, "current": map[string]any{"path": "/b.data", "source_quote": "`/b.data`"}}
				}
				bus := &types.BusContext{Mutable: types.NewMutableState(request), AttachedHitrace: "attachment window 0.999–1.051 seconds"}
				bus.Mutable.SetRequestModel(types.RequestModel{RawRequest: "previous accepted request"})
				before, _ := json.Marshal(bus.Mutable.RequestModel())
				result, err := (&EmitAnalysis{}).Execute(bus, boundedScopeAuthorityPublicParams(t, scope, request))
				if err != nil || result.Success {
					t.Fatalf("invalid scope was accepted: %+v %v", result, err)
				}
				if problem == "source_quote" {
					for _, want := range []string{field + ".source_quote", "CURRENT user request", "Attachment, tool and pre-triage"} {
						if !strings.Contains(result.Summary, want) {
							t.Errorf("source error omitted %q: %s", want, result.Summary)
						}
					}
					if strings.Contains(result.Summary, "finite") || strings.Contains(result.Summary, "precision") {
						t.Errorf("source error incorrectly directs numerical repair: %s", result.Summary)
					}
					if (form == "scalar" || form == "members") && !strings.Contains(result.Summary, "bounded_selector") {
						t.Error("missing executable current-selector repair")
					}
				} else if !strings.Contains(result.Summary, field+".time_start/time_end") || strings.Contains(result.Summary, "Attachment, tool and pre-triage") || strings.Contains(result.Summary, "bounded_selector") {
					t.Errorf("endpoint error blamed a valid user quote: %s", result.Summary)
				}
				after, _ := json.Marshal(bus.Mutable.RequestModel())
				if !bytes.Equal(before, after) {
					t.Fatal("diagnostic automatically changed accepted scope")
				}
				// The diagnosis must not assume the request lacks an interval:
				// fixing exactly the named leaf preserves the legitimate lane.
				leaf["source_quote"], leaf["time_end"] = validQuote, 2
				repaired, err := (&EmitAnalysis{}).Execute(bus, boundedScopeAuthorityPublicParams(t, scope, request))
				if err != nil || !repaired.Success || !bus.Mutable.RequestModel().RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
					t.Fatalf("precise leaf repair lost valid user interval: %+v %v", repaired, err)
				}
			})
		}
	}
}

func TestHMC227WindowSourceRepairUsesCurrentSelectorWithoutCoordinates(t *testing.T) {
	const request = "Explain OpenDocument in the attached trace."
	bus := &types.BusContext{Mutable: types.NewMutableState(request), AttachedHitrace: "OpenDocument 0.999–1.051 seconds"}
	scope := map[string]any{"requested_scope": "explicit_time_window", "time_start": 0.999, "time_end": 1.051, "source_quote": "0.999–1.051 seconds", "confidence": 1}
	result, err := (&EmitAnalysis{}).Execute(bus, boundedScopeAuthorityPublicParams(t, scope, request))
	if err != nil || result.Success || !strings.Contains(result.Summary, "CURRENT user request") || !strings.Contains(result.Summary, "bounded_selector") {
		t.Fatalf("attachment coordinates lack a precise executable repair: %+v %v", result, err)
	}
	if bus.Mutable.RequestModel() != nil {
		t.Fatal("rejection auto-elected a replacement scope")
	}
	repair := map[string]any{"requested_scope": "bounded_selector", "source_quote": "OpenDocument", "confidence": 1}
	result, err = (&EmitAnalysis{}).Execute(bus, boundedScopeAuthorityPublicParams(t, repair, request))
	if err != nil || !result.Success {
		t.Fatalf("current-selector repair failed: %+v %v", result, err)
	}
	accepted := bus.Mutable.RequestModel().RuntimeArtifactScopeProfile
	if accepted.RequestedScope != types.RuntimeArtifactScopeBoundedSelector || accepted.SourceQuote != "OpenDocument" || accepted.HasExplicitTimeWindows() || accepted.TimeStart != nil || accepted.TimeEnd != nil || accepted.TimeWindows != nil {
		t.Fatalf("selector repair minted user time bounds: %+v", accepted)
	}
}

func TestHMC227WindowLiteralDiagnosticsSeparateMissingFromAmbiguous(t *testing.T) {
	for quote, want := range map[string]string{"OpenDocument": "source_quote contains no supported literal interval", "1..2 and 4..5": "quote does not uniquely bind the declared endpoints"} {
		request := "Inspect " + quote
		bus := &types.BusContext{Mutable: types.NewMutableState(request)}
		scope := map[string]any{"requested_scope": "explicit_time_window", "time_start": 1.1, "time_end": 2.1, "source_quote": quote, "confidence": 1}
		result, err := (&EmitAnalysis{}).Execute(bus, boundedScopeAuthorityPublicParams(t, scope, request))
		if err != nil || result.Success || !strings.Contains(result.Summary, want) {
			t.Fatalf("wrong literal-source diagnostic: %+v %v", result, err)
		}
	}
}

func TestHMC227ComparisonLiteralRepairRetainsIndependentBinding(t *testing.T) {
	for _, form := range []string{"shared", "side"} {
		t.Run(form, func(t *testing.T) {
			const request = "Compare `/a.data` during OpenDocument and `/b.data`."
			a := map[string]any{"path": "/a.data", "source_quote": "`/a.data`"}
			scope := map[string]any{"requested_scope": "explicit_time_window", "confidence": 1, "comparison": map[string]any{"baseline": a, "current": map[string]any{"path": "/b.data", "source_quote": "`/b.data`"}}}
			leaf := scope
			if form == "side" {
				leaf = a
				leaf["source_quote"] = "`/a.data` during OpenDocument"
			} else {
				leaf["source_quote"] = "OpenDocument"
			}
			leaf["time_start"], leaf["time_end"] = 1, 2
			bus := &types.BusContext{Mutable: types.NewMutableState(request)}
			result, err := (&EmitAnalysis{}).Execute(bus, boundedScopeAuthorityPublicParams(t, scope, request))
			if err != nil || result.Success || !strings.Contains(result.Summary, "no supported literal interval") || !strings.Contains(result.Summary, "Preserve comparison path/source_quote bindings") || strings.Contains(result.Summary, "bounded_selector") {
				t.Fatalf("comparison was given an unsupported single-source repair: %+v %v", result, err)
			}
			delete(leaf, "time_start")
			delete(leaf, "time_end")
			scope["requested_scope"] = "unspecified"
			result, err = (&EmitAnalysis{}).Execute(bus, boundedScopeAuthorityPublicParams(t, scope, request))
			if err != nil || !result.Success {
				t.Fatalf("existing comparison repair path failed: %+v %v", result, err)
			}
			accepted := bus.Mutable.RequestModel().RuntimeArtifactScopeProfile
			if !accepted.Comparison.Valid() || accepted.HasExplicitTimeWindows() || accepted.RequestedScope != types.RuntimeArtifactScopeUnspecified {
				t.Fatalf("repair lost binding or invented range: %+v", accepted)
			}
		})
	}
}
