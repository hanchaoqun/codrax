package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC273NativeNarrativeRoleActualConsumers(t *testing.T) {
	for _, mode := range []string{"restatement", "independent", "legacy", "restored", "stale", "near_copy", "empty"} {
		t.Run(mode, func(t *testing.T) {
			ctx := nativeFactHandoffContextHMC247(t, true)
			choices := tool.NativeFactHandoffChoices(types.ToolBusContext(ctx, types.AgentExplorer))
			const narrative = "HMC273_SAME_PAYLOAD peak overlap 2 ms; render member 101"
			const note = "HMC273_INDEPENDENT_SOURCE_BUSINESS_AND_NEGATIVE_FINDING"
			const reason = "HMC273_LIFECYCLE_ONLY"
			interpretation := narrative
			if mode == "empty" {
				interpretation = ""
			}
			handoff := map[string]any{"runtime_measurements": choices[:1], "interpretation": interpretation}
			if mode != "legacy" {
				role := "native_fact_restatement"
				if mode == "independent" {
					role = "independent_analysis"
				}
				handoff["interpretation_role"] = role
			}
			ta := ctx.Mutable.TurnAArtifacts()
			ta.AcceptedClosureReason, ta.AcceptedResultKind = reason, "resolved"
			ta.InvestigationNotes = []string{narrative, note}
			if mode == "near_copy" {
				ta.InvestigationNotes = []string{narrative + "; preserve independent hypothesis", note}
			} else if mode == "empty" {
				ta.InvestigationNotes = []string{note}
			}
			ctx.Mutable.SetTurnAArtifacts(*ta)
			raw, _ := json.Marshal(map[string]any{"reason": reason, "confidence": "high", "result_kind": "resolved", "native_fact_handoff": handoff})
			r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !r.Success {
				t.Fatalf("completion: %v %s", err, r.Summary)
			}
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			if mode == "restored" {
				wire, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				var restored types.TurnAArtifacts
				if err := json.Unmarshal(wire, &restored); err != nil {
					t.Fatal(err)
				}
				ctx.Mutable = types.NewMutableState("restart")
				ctx.Mutable.SetTurnAArtifacts(restored)
				ctx.Mutable.SetInvestigationComplete(reason)
				ctx.Mutable.SetInvestigationResultKind("resolved")
			} else if mode == "stale" {
				ta := ctx.Mutable.TurnAArtifacts()
				for i := range ta.ToolResults {
					for j := range ta.ToolResults[i].Observations {
						ta.ToolResults[i].Observations[j].SourceRef.Path += "-stale"
					}
				}
				ctx.Mutable.ResetDispatchToolResults()
				ctx.Mutable.SetTurnAArtifacts(*ta)
			}
			ctx = ctx.ShallowClone()
			extractCtx := ctx.ShallowClone()
			extractCtx.AgentName, extractCtx.Stage = types.AgentExtractor, types.StageExtract
			reg := tool.NewRegistry()
			tool.RegisterDefaults(reg)
			capture := &traceTeachingCaptureLLM{stop: errors.New("capture HMC273 extraction")}
			_, err = NewExtractorAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(extractCtx, traceTeachingSkill(t, "extract-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("actual extractor: %v calls=%d", err, capture.calls)
			}
			var extracted strings.Builder
			for _, m := range capture.messages {
				extracted.WriteString(m.Content)
			}
			final := dependencyObservationMessages(t, ctx)
			wantNarrative := mode == "independent" || mode == "legacy" || mode == "near_copy"
			for name, prompt := range map[string]string{"extract": extracted.String(), "final": final} {
				if strings.Contains(prompt, narrative) != wantNarrative || !strings.Contains(prompt, note) {
					t.Errorf("%s role projection: narrative=%t want=%t independent note=%t", name, strings.Contains(prompt, narrative), wantNarrative, strings.Contains(prompt, note))
				}
			}
			projection := answerDocSeparatedNativeFactHandoff(ctx, nil)
			if projection.ownsCurrentNarrative != (mode != "restored" && mode != "stale") {
				t.Error("presentation role changed private/fresh narrative ownership")
			}
			if mode != "restored" && ctx.Mutable.AcceptedInvestigationNativeFactHandoff().Interpretation != interpretation {
				t.Error("audit original changed")
			}
		})
	}
}

func TestHMC273NativeNarrativeRoleActualSchema(t *testing.T) {
	ctx := nativeFactHandoffContextHMC247(t, true).ShallowClone()
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	reg := tool.NewRegistry()
	tool.RegisterDefaults(reg)
	reg.Register(&tool.EmitInvestigationComplete{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("capture role schema")}
	sk := traceTeachingSkill(t, "explore-skill")
	sk.ToolSuggestions = append(sk.ToolSuggestions, "emit_investigation_complete")
	_, err := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, sk)
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("actual explorer: %v", err)
	}
	for _, schema := range capture.tools {
		if schema.Name != "emit_investigation_complete" {
			continue
		}
		var root struct {
			Properties struct {
				Handoff struct {
					Required   []string `json:"required"`
					Properties map[string]struct {
						Enum        []string `json:"enum"`
						Description string   `json:"description"`
					} `json:"properties"`
				} `json:"native_fact_handoff"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(schema.Parameters, &root); err != nil {
			t.Fatal(err)
		}
		h := root.Properties.Handoff
		if !strings.Contains(strings.Join(h.Required, ","), "interpretation_role") || len(h.Properties["interpretation_role"].Enum) != 2 || h.Properties["interpretation_role"].Description != types.NativeFactInterpretationRoleTeaching {
			t.Fatal("actual schema lost explicit current role or shared teaching")
		}
		return
	}
	t.Fatal("completion schema missing")
}

func TestHMC273NativeNarrativeRoleRejectsUnknownEnum(t *testing.T) {
	ctx := nativeFactHandoffContextHMC247(t, true)
	choices := tool.NativeFactHandoffChoices(types.ToolBusContext(ctx, types.AgentExplorer))
	raw, _ := json.Marshal(map[string]any{"reason": "done", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{
		"runtime_measurements": choices[:1], "interpretation": "keep all independent evidence", "interpretation_role": "verified_causal_proof",
	}})
	r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err == nil && r.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("unknown role accepted")
	}
}
