package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func deliveryHandoffContextHMC255(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx := pendingAnswerContextHMC253(t) // Actual catalog queries and accepted public export.
	prior := ctx.AnalysisIR.RequestModel
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentReturnValue, Predicates: types.SemanticPredicates{IsScalarAnswer: true},
		RuntimeArtifactScopeProfile: prior.RuntimeArtifactScopeProfile,
		RuntimeTargets:              prior.RuntimeTargets, RuntimeTargetProfile: prior.RuntimeTargetProfile,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
	}}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
	return ctx
}

func TestHMC255ManagedDeliveryRequiresSeparatedCompletion(t *testing.T) {
	ctx := deliveryHandoffContextHMC255(t)
	const oldReason = "HMC255_LEGACY_MIXED: cache business clue; delivery pending_publication"
	result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"reason":"`+oldReason+`","confidence":"high","result_kind":"resolved"}`))
	if err != nil || result.Success || result.Repair == nil || result.Repair.Code != "managed_delivery_native_handoff" || ctx.Mutable.IsInvestigationComplete() || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatalf("mixed legacy completion must request structured separation before accepting: err=%v result=%+v", err, result)
	}
	for _, want := range []string{"native_fact_handoff", "interpretation", "runtime_measurements"} {
		if !strings.Contains(result.Summary, want) {
			t.Errorf("repair omitted %q: %s", want, result.Summary)
		}
	}
	// The producer retries using the existing contract, preserving independent
	// source/business conclusions rather than the framework editing its prose.
	var selections []types.AnswerRuntimeMeasurementReceipt
	for _, choice := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if choice.View == types.RuntimeMeasurementSummary {
			selections = append(selections, types.AnswerRuntimeMeasurementReceipt{ObservationID: choice.ObservationID, View: choice.View})
		}
	}
	if len(selections) == 0 {
		t.Fatal("actual queries did not produce summary views")
	}
	const lifecycle = "HMC255_LIFECYCLE_ONLY: delivery pending_publication"
	const interpretation = "HMC255_BUSINESS: cache reservation is an investigation clue; a causal relation and allocator leak remain unproven. Independent source explanation remains necessary."
	const independent = "HMC255_INDEPENDENT: a negative search is limited to the inspected checkout."
	ta := ctx.Mutable.TurnAArtifacts()
	ta.InvestigationNotes = []string{lifecycle, independent}
	ctx.Mutable.SetTurnAArtifacts(*ta)
	args, _ := json.Marshal(map[string]any{"reason": lifecycle, "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{"runtime_measurements": selections, "interpretation": interpretation}})
	result, err = (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
		t.Fatalf("separated public retry failed: %v %s", err, result.Summary)
	}
	ctx = ctx.ShallowClone()
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, lifecycle) {
		t.Fatal("separated lifecycle re-entered the finalizer after actual native publication")
	}
	for _, want := range []string{interpretation, independent, `"target":"capture-review"`, "Verified measurement tables"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("finalizer lost independent or native lane %q", want)
		}
	}
	if ctx.Mutable.StableInvestigationCompleteReason() != lifecycle {
		t.Fatal("presentation rewrote the original audit reason")
	}
	const model = "HMC255 final analysis remains byte preserved."
	final := FinalizeTraceCatalogExports(ctx, model, "en")
	if !strings.HasPrefix(final, model+"\n\n") || !strings.Contains(final, "matches current registered execution") {
		t.Fatalf("terminal publication lost ownership: %s", final)
	}
}

func TestHMC255ManagedDeliverySchemaAndNegativeBoundaries(t *testing.T) {
	for _, mode := range []string{"pending", "published", "no_request", "unmanaged", "no_facts", "revoked", "cancelled", "failed", "json_only"} {
		t.Run(mode, func(t *testing.T) {
			ctx := deliveryHandoffContextHMC255(t)
			switch mode {
			case "published":
				FinalizeTraceCatalogExports(ctx, "Prior analysis.", "en")
			case "no_request":
				ctx.PipelineExports = tracecatalog.NewPipelineExports(ctx.DeliveryRoot)
			case "unmanaged":
				ctx.PipelineExports = nil
			case "no_facts":
				ctx.Mutable.ResetDispatchToolResults()
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
			case "revoked", "failed":
				ctx.RepoRoot, ctx.UserPinnedFiles = t.TempDir(), nil
				if mode == "failed" {
					FinalizeTraceCatalogExports(ctx, "Prior analysis.", "en")
				}
			case "cancelled":
				cancelled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Ctx = cancelled
				FinalizeTraceCatalogExports(ctx, "Prior analysis.", "en")
			case "json_only":
				wire, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				var restored types.TurnAArtifacts
				json.Unmarshal(wire, &restored)
				ctx.Mutable = types.NewMutableState(ctx.Objective)
				ctx.Mutable.SetTurnAArtifacts(restored)
			}
			want := mode == "pending" || mode == "published"
			schemas := projectExplorerNativeFactHandoffSchema(ctx, []llm.ToolSchema{{Name: "emit_investigation_complete", Parameters: (&tool.EmitInvestigationComplete{}).Parameters()}})
			var schema struct {
				Required   []string                   `json:"required"`
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if err := json.Unmarshal(schemas[0].Parameters, &schema); err != nil {
				t.Fatal(err)
			}
			required := false
			for _, field := range schema.Required {
				required = required || field == "native_fact_handoff"
			}
			if required != want {
				t.Errorf("schema required=%t want=%t", required, want)
			}
			result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"reason":"Keep independent analysis unchanged.","confidence":"high","result_kind":"resolved"}`))
			actual := result.Repair != nil && result.Repair.Code == "managed_delivery_native_handoff"
			if err != nil || actual != want {
				t.Errorf("runtime required=%t want=%t err=%v result=%+v", actual, want, err, result)
			}
		})
	}
}

func TestHMC255ManagedDeliveryActualExplorerSchema(t *testing.T) {
	ctx := deliveryHandoffContextHMC255(t)
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	reg := tool.NewRegistry()
	tool.RegisterDefaults(reg)
	reg.Register(&tool.EmitInvestigationComplete{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("capture HMC255 delivery schema")}
	sk := traceTeachingSkill(t, "explore-skill")
	sk.ToolSuggestions = append(sk.ToolSuggestions, "emit_investigation_complete")
	_, err := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, sk)
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("actual explorer schema not captured: %v calls=%d", err, capture.calls)
	}
	for _, exposed := range capture.tools {
		if exposed.Name != "emit_investigation_complete" {
			continue
		}
		var schema struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(exposed.Parameters, &schema); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, name := range schema.Required {
			found = found || name == "native_fact_handoff"
		}
		if !found || !strings.Contains(schema.Properties["reason"].Description, "Lifecycle rationale only") ||
			strings.Contains(schema.Properties["reason"].Description, "legacy reason contract") ||
			!strings.Contains(schema.Properties["native_fact_handoff"].Description, "terminal component") {
			t.Fatal("actual outgoing schema contradicts required lifecycle separation")
		}
		return
	}
	t.Fatal("actual explorer omitted completion tool")
}

func TestHMC255ManagedDeliveryKeepsUnpublishedOrRejectedAnalysis(t *testing.T) {
	for _, mode := range []string{"unknown_id", "missing_interpretation", "unpublished", "long_interpretation", "json_restart"} {
		t.Run(mode, func(t *testing.T) {
			ctx := deliveryHandoffContextHMC255(t)
			choices := tool.NativeFactHandoffChoices(types.ToolBusContext(ctx, types.AgentExplorer))
			if len(choices) == 0 {
				t.Fatal("no actual native choices")
			}
			interpretation := "HMC255_KEEP_BUSINESS_BOUNDARY"
			if mode == "long_interpretation" {
				interpretation += strings.Repeat(" independent analysis", 100)
			}
			handoff := map[string]any{"runtime_measurements": choices[:1], "interpretation": interpretation}
			if mode == "unknown_id" {
				handoff["runtime_measurements"] = []types.AnswerRuntimeMeasurementReceipt{{ObservationID: "unknown", View: types.RuntimeMeasurementSummary}}
			}
			if mode == "missing_interpretation" {
				delete(handoff, "interpretation")
			}
			const reason = "HMC255_KEEP_ORIGINAL_LIFECYCLE"
			raw, _ := json.Marshal(map[string]any{"reason": reason, "confidence": "high", "result_kind": "resolved", "native_fact_handoff": handoff})
			result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if mode == "unknown_id" || mode == "missing_interpretation" {
				if err != nil || result.Success || ctx.Mutable.IsInvestigationComplete() || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil || result.Repair == nil || result.Repair.Code != "managed_delivery_native_handoff" || strings.Contains(result.Summary, "Omit the optional") {
					t.Fatalf("invalid required handoff lacks consistent repair: %v %+v", err, result)
				}
				return
			}
			if err != nil || !result.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
				t.Fatalf("public separated completion failed: %v %s", err, result.Summary)
			}
			if mode == "json_restart" {
				wire, _ := json.Marshal(types.TurnAArtifacts{AcceptedClosureReason: reason, AcceptedNativeFactHandoff: ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), ToolResults: ctx.Mutable.TurnAArtifacts().ToolResults})
				var restored types.TurnAArtifacts
				json.Unmarshal(wire, &restored)
				ctx.Mutable = types.NewMutableState(ctx.Objective)
				ctx.Mutable.SetTurnAArtifacts(restored)
				ctx.Mutable.SetInvestigationComplete(reason)
				ctx.Mutable.SetInvestigationResultKind("resolved")
			}
			ctx = ctx.ShallowClone()
			var published *answerDocNativePresentationReceipt
			if mode != "unpublished" {
				published = &answerDocNativePresentationReceipt{}
				renderAnswerDocRuntimeMeasurementChoices(ctx, published)
			}
			text := renderAnswerDocInvestigationNarrativeHandoffWithPublication(ctx, published)
			if !strings.Contains(text, reason) || !strings.Contains(text, "HMC255_KEEP_BUSINESS_BOUNDARY") {
				t.Fatalf("incomplete or uncredentialed display discarded original context: %s", text)
			}
		})
	}
}
