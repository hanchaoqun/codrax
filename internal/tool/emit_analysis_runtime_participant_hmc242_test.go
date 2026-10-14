package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeParticipantCoListedInputHMC242(t *testing.T, identity string, entities []any) (*types.BusContext, map[string]any) {
	t.Helper()
	const quote = hmc242ParticipantQuote + "和 observer"
	question := "看看附件中 1.000 到 1.020 秒 " + quote + "被谁唤醒了，再画一张唤醒时序图。"
	ctx, payload, _ := requestedThreadAnalysisInputHMC242(t, question, func(payload map[string]any) {
		payload["entities"] = entities
		payload["runtime_target_profile"].(map[string]any)["source_quote"] = quote
		diagram := payload["diagram_hint"].(map[string]any)
		diagram["relation_scope_quote"] = quote + "被谁唤醒了"
		diagram["participants"] = []any{map[string]any{"identity": identity, "source_quote": quote, "role": "incident_required"}}
	})
	return ctx, payload
}

func TestHMC242RuntimeParticipantPublicOmittedActorRejected(t *testing.T) {
	for _, identity := range []string{"observer", hmc242ParticipantQuote + "和 observer"} {
		t.Run(identity, func(t *testing.T) {
			ctx, payload := runtimeParticipantCoListedInputHMC242(t, identity, []any{"target", "observer"})
			ctx.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "trace_query", Success: true, Summary: "retained prior native result"})
			before := ctx.Mutable.DispatchToolResults()
			raw, _ := json.Marshal(payload)
			out, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil || out.Success || !strings.Contains(out.Summary, "ambiguous native thread binding") || ctx.Mutable.RequestModel() != nil {
				t.Fatalf("co-listed actor acquired selected target identity: err=%v success=%t summary=%s request=%+v", err, out.Success, out.Summary, ctx.Mutable.RequestModel())
			}
			if !reflect.DeepEqual(before, ctx.Mutable.DispatchToolResults()) {
				t.Fatal("invalid analyzer declaration changed existing native observations")
			}
		})
	}
}

func TestHMC242RuntimeParticipantPublicIdentityCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, quote, identity, thread string
		entities                      []any
	}{
		{"short_name", hmc242ParticipantQuote, "target", "target-42", []any{"target"}},
		{"whole_quote", hmc242ParticipantQuote, hmc242ParticipantQuote, "target-42", []any{hmc242ParticipantQuote}},
		{"whole_quote_name_and_id", hmc242ParticipantQuote, hmc242ParticipantQuote, "target-42", []any{"target", "42"}},
		{"whole_quote_name_and_tid", hmc242ParticipantQuote, hmc242ParticipantQuote, "target-42", []any{"target", "tid 42"}},
		{"numeric_participant", hmc242ParticipantQuote, "42", "target-42", []any{"target", "42"}},
		{"canonical_tid_participant", hmc242ParticipantQuote, "tid 42", "target-42", []any{"target", "tid 42"}},
		{"numeric_participant_broad_quote", hmc242ParticipantQuote + "和 observer", "42", "target-42", []any{"target", "observer", "42"}},
		{"canonical_tid_broad_quote", hmc242ParticipantQuote + "和 observer", "tid 42", "target-42", []any{"target", "observer"}},
		{"alias", "TargetWorker 线程（tid 42）", "target_worker", "TargetWorker-42", []any{"TargetWorker", "target_worker", "42"}},
		{"same_named_selector", "target (42)", "target", "target-42", []any{"target", "target (42)", "42"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			question := "看看附件中 1.000 到 1.020 秒 " + tc.quote + "被谁唤醒了，再画一张唤醒时序图。"
			ctx, payload, _ := requestedThreadAnalysisInputHMC242(t, question, func(payload map[string]any) {
				payload["entities"] = tc.entities
				payload["runtime_target_profile"].(map[string]any)["source_quote"] = tc.quote
				payload["runtime_targets"] = []any{runtimeTargetProvenanceRowHMC237("thread", 42, tc.thread)}
				diagram := payload["diagram_hint"].(map[string]any)
				diagram["relation_scope_quote"] = tc.quote + "被谁唤醒了"
				diagram["participants"] = []any{map[string]any{"identity": tc.identity, "source_quote": tc.quote, "role": "incident_required"}}
			})
			raw, _ := json.Marshal(payload)
			out, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil || !out.Success {
				t.Fatalf("one exact selected identity was split into actors: %v %s", err, out.Summary)
			}
			rm := ctx.Mutable.RequestModel()
			if tid, ok := requestedRuntimeParticipantThread(rm, rm.DiagramHint.Participants[0]); !ok || tid != 42 {
				t.Fatalf("compatible identity lost binding: %+v", rm)
			}
		})
	}
}

func TestHMC242RuntimeParticipantPublicNarrowQuoteRepair(t *testing.T) {
	ctx, payload := runtimeParticipantCoListedInputHMC242(t, "observer", []any{"target", "observer"})
	raw, _ := json.Marshal(payload)
	bad, err := (&EmitAnalysis{}).Execute(ctx, raw)
	if err != nil || bad.Success {
		t.Fatalf("expected original ambiguity before repair: %v %+v", err, bad)
	}
	payload["runtime_target_profile"].(map[string]any)["source_quote"] = hmc242ParticipantQuote
	participant := payload["diagram_hint"].(map[string]any)["participants"].([]any)[0].(map[string]any)
	participant["identity"], participant["source_quote"] = "target", hmc242ParticipantQuote
	raw, _ = json.Marshal(payload)
	good, err := (&EmitAnalysis{}).Execute(ctx, raw)
	if err != nil || !good.Success {
		t.Fatalf("narrow typed quote did not repair declaration: %v %+v", err, good)
	}
	if got := ctx.Mutable.RequestModel().AnalyzerHints.Entities; !reflect.DeepEqual(got, []string{"target", "observer"}) {
		t.Fatalf("repair discarded original entity roster: %v", got)
	}
}

func TestHMC242RuntimeParticipantPublicDifferentSelectorRejected(t *testing.T) {
	for _, identity := range []string{"43", "tid 43", "observer-43"} {
		t.Run(identity, func(t *testing.T) {
			quote := "tid 42 和 " + identity
			question := "看看附件中 1.000 到 1.020 秒 " + quote + "被谁唤醒了，再画一张唤醒时序图。"
			ctx, payload, _ := requestedThreadAnalysisInputHMC242(t, question, func(payload map[string]any) {
				payload["entities"] = []any{identity}
				payload["runtime_target_profile"].(map[string]any)["source_quote"] = quote
				payload["runtime_targets"] = []any{runtimeTargetProvenanceRowHMC237("thread", 42, "")}
				diagram := payload["diagram_hint"].(map[string]any)
				diagram["relation_scope_quote"] = quote + "被谁唤醒了"
				diagram["participants"] = []any{map[string]any{"identity": identity, "source_quote": quote, "role": "incident_required"}}
			})
			raw, _ := json.Marshal(payload)
			out, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil || out.Success || !strings.Contains(out.Summary, "explicit thread coordinate 43 differs from the selected thread 42") {
				t.Fatalf("typed participant coordinate borrowed different target: %v %+v", err, out)
			}
		})
	}
}

func TestHMC242RuntimeParticipantOriginalRosterAndTargetNameBoundaries(t *testing.T) {
	ctx, rows := requestedThreadFixtureHMC242(t)
	rm := *ctx.Mutable.RequestModel()
	const quote = hmc242ParticipantQuote + "和 observer"
	rm.RuntimeTargetProfile.SourceQuote = quote
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{Identity: "observer", SourceQuote: quote, Role: types.DiagramParticipantIncidentRequired}}
	rm.AnalyzerHints.Entities, rm.AnalyzerHints.PrimaryEntities = []string{"observer"}, []string{"observer"}
	rm.RuntimeTargets[0].Thread = ""
	if issue := validateRequiredFlowDiagramParticipantProvenance(rm, freezeModelEntityRoster([]string{"target", "observer"})); !strings.Contains(issue, "ambiguous native thread binding") {
		t.Fatalf("mutable narrowing erased original actor: %q", issue)
	}
	rm.AnalyzerHints.Entities, rm.AnalyzerHints.PrimaryEntities = []string{"target", "observer"}, []string{"target", "observer"}
	if issue := validateRequiredFlowDiagramParticipantProvenance(rm, freezeModelEntityRoster([]string{"observer"})); issue != "" {
		t.Fatalf("later expanded entity became original-actor authority: %q", issue)
	}
	rm.RuntimeTargets[0].Thread = "target"
	if issue := validateRequiredFlowDiagramParticipantProvenance(rm, freezeModelEntityRoster([]string{"observer"})); !strings.Contains(issue, "ambiguous native thread binding") {
		t.Fatalf("quoted target name did not veto conflicting display join: %q", issue)
	}
	rm.RuntimeTargets[0].Thread = "not_in_quote"
	if issue := validateRequiredFlowDiagramParticipantProvenance(rm, freezeModelEntityRoster([]string{"observer"})); issue != "" {
		t.Fatalf("unquoted display name acquired authority: %q", issue)
	}
	if len(rows) != 3 {
		t.Fatal("native source fixture lost its three events")
	}
}

func TestHMC242RuntimeParticipantNoBindingPathsUnchanged(t *testing.T) {
	ctx, _ := requestedThreadFixtureHMC242(t)
	baseJSON, _ := json.Marshal(ctx.Mutable.RequestModel())
	for _, mode := range []string{"no_diagram", "optional", "multiple_targets", "no_target", "process", "other_quote", "context_only", "business_role", "different_tid"} {
		t.Run(mode, func(t *testing.T) {
			var rm types.RequestModel
			_ = json.Unmarshal(baseJSON, &rm)
			rm.RuntimeTargetProfile.SourceQuote = hmc242ParticipantQuote + "和 observer tid 11"
			rm.DiagramHint.Participants = []types.DiagramParticipantHint{{Identity: "observer", SourceQuote: rm.RuntimeTargetProfile.SourceQuote, Role: types.DiagramParticipantIncidentRequired}}
			switch mode {
			case "no_diagram":
				rm.DiagramHint = nil
			case "optional":
				rm.DiagramHint.Required = false
			case "multiple_targets":
				rm.RuntimeTargets = append(rm.RuntimeTargets, types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 11, Thread: "observer", Source: "user_explicit"})
			case "no_target":
				rm.RuntimeTargets = nil
			case "process":
				rm.RuntimeTargets[0].Kind = types.RuntimeTargetKindProcess
			case "other_quote":
				rm.DiagramHint.Participants[0].SourceQuote = "observer"
			case "context_only":
				rm.DiagramHint.Participants[0].Role = types.DiagramParticipantContextOnly
			case "business_role":
				rm.DiagramHint.Participants[0].Identity = "business owner"
			case "different_tid":
				rm.RuntimeTargets[0].PID = 77
			}
			if issue := validateRequiredFlowDiagramParticipantProvenance(rm, freezeModelEntityRoster([]string{"target", "observer"})); issue != "" {
				t.Fatalf("nonbinding request inherited a new obligation: %q", issue)
			}
		})
	}
}

func TestHMC242RuntimeParticipantPublicNoBindingPaths(t *testing.T) {
	for _, mode := range []string{"multiple_targets", "no_required_diagram", "other_quote", "context_only", "name_only"} {
		t.Run(mode, func(t *testing.T) {
			const quote = hmc242ParticipantQuote + "和 observer 线程（tid 11）"
			question := "看看附件中 1.000 到 1.020 秒 " + quote + "被谁唤醒了，再画一张唤醒时序图。"
			ctx, payload, _ := requestedThreadAnalysisInputHMC242(t, question, func(payload map[string]any) {
				payload["entities"] = []any{"target", "observer"}
				payload["runtime_target_profile"].(map[string]any)["source_quote"] = quote
				diagram := payload["diagram_hint"].(map[string]any)
				diagram["relation_scope_quote"] = quote + "被谁唤醒了"
				participant := map[string]any{"identity": "observer", "source_quote": quote, "role": "incident_required"}
				diagram["participants"] = []any{participant}
				switch mode {
				case "multiple_targets":
					payload["runtime_targets"] = []any{runtimeTargetProvenanceRowHMC237("thread", 42, "target-42"), runtimeTargetProvenanceRowHMC237("thread", 11, "observer-11")}
				case "no_required_diagram":
					diagram["required"] = false
					payload["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": false, "confidence": .95, "dimensions": []any{}}
				case "other_quote":
					participant["source_quote"] = "observer"
				case "context_only":
					participant["role"] = "context_only"
				case "name_only":
					payload["runtime_targets"] = []any{runtimeTargetProvenanceRowHMC237("thread", 0, "observer")}
				}
			})
			raw, _ := json.Marshal(payload)
			result, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("nonbinding public request inherited actor completeness gate: %v %+v", err, result)
			}
		})
	}
}

func TestHMC242RuntimeParticipantIdentityKeyPreservesDifferentNamedSelectors(t *testing.T) {
	for _, tc := range []struct {
		identity string
		key      string
		selector bool
	}{
		{"42", "", true}, {"tid 42", "", true}, {"0042", "", true},
		{"target-42", "flat:target", false}, {"observer-42", "flat:observer", false},
		{"target-43", "exact:target43", false}, {"target_worker", "flat:targetworker", false},
	} {
		key, selector := runtimeParticipantBindingIdentityKey(tc.identity, 42)
		if key != tc.key || selector != tc.selector {
			t.Fatalf("%q: key=%q selector=%t", tc.identity, key, selector)
		}
	}
}
