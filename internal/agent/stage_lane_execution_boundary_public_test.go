package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Capture the actual default-skill finalizer request, not just the stage
// renderer. The verified checkout fixture supplies membership/order, not an
// execution history. No model response, dispatch receipt, or answer is invented.
func stageLaneExecutionBoundaryMessages(t *testing.T, lang string, endpoints []string, mutate func(*types.BusContext)) string {
	t.Helper()
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	const request = "Explain the selected workflow and its state carriers."
	rm := types.RequestModel{RawRequest: request, Intent: types.IntentExplain, PredicateAxis: types.AxisFlow, Language: lang,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true}}
	for _, endpoint := range endpoints {
		rm.DiagramHint.Participants = append(rm.DiagramHint.Participants, types.DiagramParticipantHint{
			Identity: endpoint, Role: types.DiagramParticipantIncidentRequired,
		})
	}
	mu := types.NewMutableState(request)
	mu.SetRequestModel(rm)
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Original model-owned answer."}}})
	bus := &types.BusContext{RepoRoot: repo, Mode: types.ModeRead, Language: lang, Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm},
		EvidenceItems: []types.EvidenceItem{{ID: "membership", Kind: types.EvidenceDirect,
			Source: types.ReadModePipelineStageBindingFile, LineStart: 3, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "StageBinding", GroundingStatus: types.GroundingGrounded}}}
	if mutate != nil {
		mutate(bus)
	}
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	snapshot := func() string {
		data, err := json.Marshal([]any{ctx.AnalysisIR, ctx.EvidenceItems, bus.StageReports, mu.AnswerDocumentV2()})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	before := snapshot()
	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.EmitAnswerDocument{})
	reg.Register(&toolpkg.EmitAnswerDocumentPatch{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured stage execution boundary request")}
	finalizer := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
	_, err := finalizer.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if capture.calls != 1 || !errors.Is(err, capture.stop) {
		t.Fatalf("HARNESS: expected one real adapter request, calls=%d err=%v", capture.calls, err)
	}
	if before != snapshot() {
		t.Fatal("teaching mutated membership, evidence, execution history, or model-owned answer")
	}
	var message strings.Builder
	for _, row := range capture.messages {
		if row.Role == "system" || row.Role == "user" {
			message.WriteString(row.Content)
			message.WriteByte('\n')
		}
	}
	return message.String()
}

func TestStageLaneExecutionBoundaryActualFinalizerMessages(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name, sequence string
			endpoints      []string
			count          int
		}{
			{"full_membership", "analyze -> explore -> extract -> finalize", []string{"analyze", "explorer", "extractor", "finalizer"}, 3},
			{"bounded_endpoint_span", "explore -> extract -> finalize", []string{"explorer", "finalizer"}, 2},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				message := stageLaneExecutionBoundaryMessages(t, lang, tc.endpoints, nil)
				for _, want := range []string{
					"canonical_read_main_sequence=`" + tc.sequence + "`",
					"stage membership and logical precedence, not a receipt that every stage runs in every request or exactly once",
					"Conditions, skipped stages, retries, and reuse of accepted artifacts require grounded control-flow evidence or actual execution receipts",
					"stage/binding/namespace/catalog/function declarations alone do not prove them",
					"A zero-dispatch observation does not remove a stage from this capability roster or prove that its artifacts are absent",
					"when a selected stage is executed, its listed agent owns the model-authored work",
					"capability catalog proves existence/availability only",
					"language model authors only the request classification",
					"AgentExtractor authors the structured extraction",
					"The Mermaid arrow is presentation syntax; the typed anchor keeps it an ordering relation rather than a function call",
				} {
					if !strings.Contains(message, want) {
						t.Errorf("actual finalizer request missing boundary %q", want)
					}
				}
				if strings.Contains(message, "every selected stage is executed by its listed agent") {
					t.Error("availability was still taught as an unconditional execution receipt")
				}
				if count := strings.Count(message, "- stage_precedence["); count != tc.count {
					t.Errorf("stage capability/order changed: recipes=%d want=%d", count, tc.count)
				}
			})
		}
	}
}

func TestStageLaneExecutionBoundaryActualFinalizerMessagesStayScoped(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*types.BusContext)
	}{
		{"trace", func(bus *types.BusContext) { bus.AnalysisIR.RequestModel.Intent = types.IntentTrace }},
		{"write", func(bus *types.BusContext) { bus.Mode = types.ModeApply }},
		{"unverified_source", func(bus *types.BusContext) { bus.EvidenceItems[0].GroundingStatus = types.GroundingUngrounded }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := stageLaneExecutionBoundaryMessages(t, "en", []string{"analyze", "finalizer"}, tc.mutate)
			if strings.Contains(message, "## Current Run Stage-Lane Authority") || strings.Contains(message, "- stage_precedence[") {
				t.Fatal("canonical read-stage teaching widened into another authority scope")
			}
		})
	}
}
