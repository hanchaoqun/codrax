package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC256PublicNativeFactRetryRepairsExactPair(t *testing.T) {
	for _, mode := range []string{"unknown_id", "wrong_view", "duplicate", "missing_interpretation", "empty_selection", "event_selection", "missing_handoff", "selection_limit_16"} {
		t.Run(mode, func(t *testing.T) {
			ctx := deliveryHandoffContextHMC255(t)
			bus := types.ToolBusContext(ctx, types.AgentExplorer)
			choices := tool.NativeFactHandoffChoices(bus)
			if len(choices) < 1 {
				t.Fatal("actual exported queries have no native choices")
			}
			selection := map[string]any{"observation_id": choices[0].ObservationID, "view": choices[0].View}
			const explanation = "Business clue: cache reservation deserves inspection; a leak and causal connection remain unproven."
			handoff := map[string]any{"runtime_measurements": []any{selection}, "interpretation": explanation}
			wantField := "native_fact_handoff.runtime_measurements[0]"
			switch mode {
			case "unknown_id":
				selection["observation_id"] = "guessed-observation"
			case "wrong_view":
				selection["view"] = "guessed-view"
			case "duplicate":
				handoff["runtime_measurements"] = []any{selection, selection}
				wantField = "native_fact_handoff.runtime_measurements[1]"
			case "missing_interpretation":
				delete(handoff, "interpretation")
				wantField = "native_fact_handoff.interpretation"
			case "empty_selection":
				handoff["runtime_measurements"] = []any{}
				wantField = "native_fact_handoff.runtime_measurements"
			case "event_selection":
				selection["event_selection"] = map[string]any{}
			case "selection_limit_16":
				items := make([]any, 17)
				for i := range items {
					items[i] = selection
				}
				handoff["runtime_measurements"] = items
				wantField = "native_fact_handoff.runtime_measurements"
			case "missing_handoff":
				wantField = "native_fact_handoff"
			}
			payload := map[string]any{"reason": "Lifecycle rationale only.", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": handoff}
			if mode == "missing_handoff" {
				delete(payload, "native_fact_handoff")
			}
			raw, _ := json.Marshal(payload)
			result, err := (&tool.EmitInvestigationComplete{}).Execute(bus, raw)
			if err != nil || result.Success || result.Repair == nil || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil || ctx.Mutable.IsInvestigationComplete() {
				t.Fatalf("invalid selector must fail without acceptance: %v %+v", err, result)
			}
			if !strings.Contains(result.Summary, wantField) || !strings.Contains(result.Summary, mode) {
				t.Errorf("repair lacks exact mismatch: %s", result.Summary)
			}
			var offered []types.AnswerRuntimeMeasurementReceipt
			if err := json.Unmarshal([]byte(result.Repair.Metadata["current_runtime_measurements"]), &offered); err != nil || !reflect.DeepEqual(offered, choices) {
				t.Fatalf("repair must supply current copyable paired choices: %v %+v", err, result.Repair)
			}
			pairJSON, _ := json.Marshal(offered[0])
			if !strings.Contains(result.Summary, string(pairJSON)) {
				t.Fatal("model-visible summary omitted copyable exact pair")
			}
			// Retry copies the offered pair as one object. No ID/view reconstruction.
			handoff["runtime_measurements"], handoff["interpretation"] = offered[:1], explanation
			payload["native_fact_handoff"] = handoff
			raw, _ = json.Marshal(payload)
			result, err = (&tool.EmitInvestigationComplete{}).Execute(bus, raw)
			accepted := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
			if err != nil || !result.Success || accepted == nil || accepted.Interpretation != explanation {
				t.Fatalf("exact offered retry lost independent explanation or failed: %v %+v", err, result)
			}
		})
	}
}

type nativeFactPublicationLLMHMC256 struct {
	traceTeachingCaptureLLM
	first []llm.Message
}

func (l *nativeFactPublicationLLMHMC256) Chat(ctx context.Context, messages []llm.Message, schemas []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	if l.calls == 0 {
		l.calls++
		l.first = append([]llm.Message(nil), messages...)
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "publish-native", Name: "trace_query", Params: json.RawMessage(`{"path":"captures/capture.data","view":"resource_stack","pid":202,"time_start":10,"time_end":10.05}`)}}}, nil
	}
	return l.traceTeachingCaptureLLM.Chat(ctx, messages, schemas, opts)
}

func TestHMC256ActualExplorerLaterPublicationKeepsConditionalOwnership(t *testing.T) {
	ctx := deliveryHandoffContextHMC255(t)
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	ctx.Mutable.ResetDispatchToolResults()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
	if tool.ManagedDeliveryRequiresNativeFactHandoff(types.ToolBusContext(ctx, types.AgentExplorer)) {
		t.Fatal("fixture already has native facts")
	}
	reg := tool.NewRegistry()
	tool.RegisterDefaults(reg)
	reg.Register(&tool.EmitInvestigationComplete{})
	capture := &nativeFactPublicationLLMHMC256{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("capture after real native publication")}}
	sk := traceTeachingSkill(t, "explore-skill")
	sk.ToolSuggestions = append(sk.ToolSuggestions, "emit_investigation_complete", "trace_query")
	_, err := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 3}).Execute(ctx, sk)
	if !errors.Is(err, capture.stop) || capture.calls != 2 {
		t.Fatalf("actual two-round publication path failed: %v %d", err, capture.calls)
	}
	for _, messages := range [][]llm.Message{capture.first, capture.messages} {
		var text strings.Builder
		for _, m := range messages {
			text.WriteString(m.Content)
		}
		if !strings.Contains(text.String(), "after later tool results change") || !strings.Contains(text.String(), "use native_fact_handoff.interpretation when the current completion schema requires") {
			t.Fatal("initial legacy teaching survived publication without a conditional owner")
		}
		if strings.Contains(text.String(), "make `reason` the concise investigation conclusion") && !strings.Contains(text.String(), "legacy reason guidance applies only while native_fact_handoff is neither required nor selected") {
			t.Fatal("legacy-only paragraph has no conditional boundary")
		}
	}
	if !tool.ManagedDeliveryRequiresNativeFactHandoff(types.ToolBusContext(ctx, types.AgentExplorer)) {
		bus := types.ToolBusContext(ctx, types.AgentExplorer)
		requests, requestErr := tool.AuthorizedPendingTraceCatalogExports(bus)
		t.Fatalf("real trace_query did not activate current native contract: choices=%+v requests=%+v err=%v", tool.NativeFactHandoffChoices(bus), requests, requestErr)
	}
	found := false
	for _, s := range capture.tools {
		if s.Name != "emit_investigation_complete" {
			continue
		}
		var schema struct {
			Required []string `json:"required"`
		}
		json.Unmarshal(s.Parameters, &schema)
		for _, f := range schema.Required {
			found = found || f == "native_fact_handoff"
		}
	}
	if !found {
		t.Fatal("second outgoing schema did not require current handoff")
	}
}

func TestHMC256NativeFactRepairRefreshesEligibility(t *testing.T) {
	for _, mode := range []string{"no_facts", "standalone", "cancelled", "revoked"} {
		t.Run(mode, func(t *testing.T) {
			ctx := deliveryHandoffContextHMC255(t)
			choices := tool.NativeFactHandoffChoices(types.ToolBusContext(ctx, types.AgentExplorer))
			switch mode {
			case "no_facts":
				ctx.Mutable.ResetDispatchToolResults()
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
			case "standalone":
				ctx.PipelineExports = nil
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Ctx = c
			case "revoked":
				ctx.RepoRoot, ctx.UserPinnedFiles = t.TempDir(), nil
			}
			choices[0].View = "unpublished"
			raw, _ := json.Marshal(map[string]any{"reason": "keep independent uncertainty", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{"runtime_measurements": choices[:1], "interpretation": "independent business direction"}})
			r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || r.Success || r.Repair == nil || r.Repair.Code != "native_fact_handoff" || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
				t.Fatalf("optional/current-eligibility repair upgraded authority: %v %+v", err, r)
			}
			var offered []types.AnswerRuntimeMeasurementReceipt
			json.Unmarshal([]byte(r.Repair.Metadata["current_runtime_measurements"]), &offered)
			if !reflect.DeepEqual(offered, tool.NativeFactHandoffChoices(types.ToolBusContext(ctx, types.AgentExplorer))) || strings.Contains(r.Summary, "A named delivery is already accepted") {
				t.Fatal("stale managed requirement or stale pair roster")
			}
		})
	}
}

func TestHMC256ActualExplorerCompletionTeaching(t *testing.T) {
	ctx := deliveryHandoffContextHMC255(t)
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	reg := tool.NewRegistry()
	tool.RegisterDefaults(reg)
	reg.Register(&tool.EmitInvestigationComplete{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("capture HMC256 completion contract")}
	sk := traceTeachingSkill(t, "explore-skill")
	sk.ToolSuggestions = append(sk.ToolSuggestions, "emit_investigation_complete")
	_, err := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, sk)
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("actual adapter not captured: %v %d", err, capture.calls)
	}
	var outgoing strings.Builder
	for _, m := range capture.messages {
		outgoing.WriteString(m.Content)
	}
	checkHMC256NarrativeTeaching(t, outgoing.String())
	found := false
	for _, schema := range capture.tools {
		if schema.Name != "emit_investigation_complete" {
			continue
		}
		found = true
		var s struct {
			Properties map[string]struct {
				Properties map[string]struct {
					Items struct {
						Enum []types.AnswerRuntimeMeasurementReceipt `json:"enum"`
					} `json:"items"`
				} `json:"properties"`
			} `json:"properties"`
		}
		if json.Unmarshal(schema.Parameters, &s) != nil {
			t.Fatal("invalid outgoing schema")
		}
		got := s.Properties["native_fact_handoff"].Properties["runtime_measurements"].Items.Enum
		if !reflect.DeepEqual(got, tool.NativeFactHandoffChoices(types.ToolBusContext(ctx, types.AgentExplorer))) {
			t.Fatal("outgoing schema offers different pairs from completion repair")
		}
	}
	if !found {
		t.Fatal("actual outgoing completion schema missing")
	}
}

func checkHMC256NarrativeTeaching(t *testing.T, text string) {
	t.Helper()
	if !strings.Contains(text, "native_fact_handoff") || !strings.Contains(text, "interpretation") {
		t.Errorf("current completion contract missing: %s", text)
	}
	for _, old := range []string{"make `reason` the concise investigation conclusion", "put them in `reason`", "narrative conclusions belong in `reason`", "concise conclusion and boundary in reason", "important boundary in `reason`", "keep it in `reason` / `aggregate_facts`"} {
		if strings.Contains(text, old) {
			t.Errorf("conflicting legacy teaching %q remains", old)
		}
	}
}

func TestHMC256SoftStopAndCheckpointUseCurrentContract(t *testing.T) {
	ctx := deliveryHandoffContextHMC255(t)
	e := &explorerEvaluator{phase: 1, mutable: ctx.Mutable, analysisIR: ctx.AnalysisIR}
	signal := e.Observe(ctx, LoopObservation{Phase: PhaseSoftStop, Iteration: 4, Response: llm.Response{Content: "The bounded investigation is ready."}})
	if !signal.HintRequested || signal.HintKey != "explorer.completion-tool-reminder" {
		t.Fatalf("expected actual soft-stop completion reminder: %+v", signal)
	}
	checkHMC256NarrativeTeaching(t, signal.Hint)
	checkHMC256NarrativeTeaching(t, e.buildDurableProgressContinuationInstruction(ctx))
	// No managed delivery still offers the optional same-native contract; no
	// native facts retains the independent legacy narrative lane unchanged.
	ctx.PipelineExports = nil
	checkHMC256NarrativeTeaching(t, e.buildDurableProgressContinuationInstruction(ctx))
	ctx.Mutable = types.NewMutableState("legacy independent source investigation")
	legacy := e.buildDurableProgressContinuationInstruction(ctx)
	if !strings.Contains(legacy, "keep it in `reason` / `aggregate_facts`") {
		t.Fatal("legacy recovery lane was removed")
	}
}
