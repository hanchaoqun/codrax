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

type hmc375RepairAdapter struct {
	traceTeachingCaptureLLM
	window   int
	padding  int
	stop     error
	mutable  *types.MutableState
	messages [][]llm.Message
	tools    [][]llm.ToolSchema
}

func (a *hmc375RepairAdapter) MaxContextTokens() int { return a.window }
func (*hmc375RepairAdapter) MaxOutputTokens() int    { return 0 }
func (a *hmc375RepairAdapter) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	a.messages = append(a.messages, append([]llm.Message(nil), messages...))
	a.tools = append(a.tools, append([]llm.ToolSchema(nil), schemas...))
	switch len(a.messages) {
	case 1:
		// An existing rejected draft is legitimate in a retry dispatch. The
		// real empty-block rejection selects the patch tool without changing it.
		a.mutable.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
			DocumentModel: "v2", Citations: []types.Citation{{File: "x.go", Line: 10}},
			Blocks: []types.AnswerBlock{
				{ID: "s1", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "unchanged model-authored premise", FacetIDs: []string{string(types.FacetCurrentCodePath)}},
				{ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal, FacetIDs: []string{string(types.FacetCurrentCodePath)},
					ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}}, Items: []types.AnswerBlockItem{{ID: "hop", Label: "original rejected hop", CitationRef: 0}}},
			},
		})
		return llm.Response{Content: strings.Repeat("superseded first attempt notes ", 1050), ToolCalls: []llm.ToolCall{{
			ID: "full-emit-1", Name: "emit_answer_document", Params: json.RawMessage(`{"blocks":[]}`),
		}}}, nil
	case 2:
		// HMC376 re-base: the padding of the over-limit control used to sit in
		// this attempt's free text. Free text that accompanies an answer attempt
		// is now given up before a repair request is refused, so the control puts
		// its bulk where every repair rung must keep it: the submitted patch
		// arguments, which also become the staged draft. The assertions below are
		// unchanged and keep their meaning: a transaction that is still over the
		// allocation because of content that may not be dropped is neither sent
		// nor published.
		label, _ := json.Marshal("staged hop requiring a real edge" + strings.Repeat(" and one more retained clause", a.padding))
		return llm.Response{Content: strings.Repeat("most recent patch attempt notes ", 1600), ToolCalls: []llm.ToolCall{{
			ID: "live-patch-2", Name: "emit_answer_document_patch", Params: json.RawMessage(`{"unchanged_block_ids":["s1"],"replace_blocks":[{"id":"path","kind":"ordered_list","surface_role":"principal","facet_ids":["current_code_path"],"claim_uses":[{"claim_form":"call_edge"}],"items":[{"id":"hop","label":` + string(label) + `,"citation_ref":0}]}]}`),
		}}}, nil
	default:
		return llm.Response{}, a.stop
	}
}

func TestHMC375FinalizerPublicCumulativeRepairFromLiveGeneration(t *testing.T) {
	run := func(window, padding int) (*hmc375RepairAdapter, *types.MutableState, error) {
		mutable := types.NewMutableState("Describe the verified call chain from A to B")
		ctx := &types.AgentContext{Objective: mutable.Objective(), Stage: types.StageFinalize, AgentName: types.AgentFinalizer, Language: "en", Mutable: mutable,
			Constraints: []string{"Preserve the independent evidence constraint."},
			AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}}}}
		a := &hmc375RepairAdapter{window: window, padding: padding, mutable: mutable, stop: errors.New("latest staged patch reached actual adapter")}
		registry := tool.NewRegistry()
		registry.Register(&tool.EmitAnswerDocument{})
		registry.Register(&tool.EmitAnswerDocumentPatch{})
		base := NewFinalizerAgent(&Dependencies{LLM: a, Tools: registry, MaxIterations: 3,
			AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}})
		_, err := base.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
		return a, mutable, err
	}
	wide, state, err := run(1_000_000, 0)
	if !errors.Is(err, wide.stop) || len(wide.messages) != 3 || state.PendingAnswerDocumentPatchBase() == nil || state.AnswerDocumentV2() != nil {
		t.Fatalf("fixture must perform an actual staged patch rejection: requests=%d staged=%t err=%v", len(wide.messages), state.PendingAnswerDocumentPatchBase() != nil, err)
	}
	first := llm.EstimateRequestBytes(wide.messages[0], wide.tools[0])
	window := (first + 11000) * 2 / 7
	t.Logf("wide request bytes: %d / %d / %d; tight allocation=%d", first, llm.EstimateRequestBytes(wide.messages[1], wide.tools[1]), llm.EstimateRequestBytes(wide.messages[2], wide.tools[2]), window*4-window*4/8)
	tight, state, err := run(window, 0)
	if !errors.Is(err, tight.stop) || len(tight.messages) != 3 {
		if len(tight.messages) == 2 {
			t.Logf("round two static teaching change=%d", len(wide.messages[1][0].Content)-len(tight.messages[1][0].Content))
		}
		for _, result := range state.DispatchToolResults() {
			if result.Repair != nil {
				t.Logf("repair: tool=%s code=%s outcome=%s revalidated=%s optional=%d", result.ToolName, result.Repair.Code, result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome], result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchFullyRevalidated], len(result.OptionalCarrierOutcomes))
			} else {
				t.Logf("tool=%s has no typed repair, success=%t", result.ToolName, result.Success)
			}
		}
		t.Fatalf("latest staged repair failed to reach the real adapter: first=%d allocation=%d requests=%d err=%v", first, window*4-window*4/8, len(tight.messages), err)
	}
	if !reflect.DeepEqual(tight.messages[0], wide.messages[0]) || !reflect.DeepEqual(tight.tools, wide.tools) {
		t.Fatal("recovery changed the initial context or any tool authority")
	}
	last, _ := json.Marshal(tight.messages[2])
	for _, want := range []string{"unchanged model-authored premise", "staged hop requiring a real edge", "live-patch-2", "Preserve the independent evidence constraint."} {
		if !strings.Contains(string(last), want) {
			t.Errorf("latest request lost %q", want)
		}
	}
	for _, stale := range []string{"full-emit-1", "superseded first attempt notes", "original rejected hop"} {
		if strings.Contains(string(last), stale) {
			t.Errorf("latest request replayed stale material %q", stale)
		}
	}
	if llm.EstimateRequestBytes(tight.messages[2], tight.tools[2]) > window*4-window*4/8 || state.AnswerDocumentV2() != nil ||
		state.LastRejectedAnswerDocumentV2().Blocks[1].Items[0].Label != "original rejected hop" || state.PendingAnswerDocumentPatchBase().Blocks[1].Items[0].Label != "staged hop requiring a real edge" {
		t.Fatal("request recovery changed durable state or bypassed allocation")
	}
	blocked, state, err := run(window, 4000)
	var budgetErr *AnswerContextByteBudgetError
	if len(blocked.messages) != 2 || !errors.As(err, &budgetErr) || budgetErr.ActualBytes <= budgetErr.AllocatedBytes || state.AnswerDocumentV2() != nil || state.PendingAnswerDocumentPatchBase() == nil {
		t.Fatalf("still-over-limit current transaction was sent or published: calls=%d err=%v", len(blocked.messages), err)
	}
}

func TestHMC375RepairHistoryNeedsExactWholeDraftAndPairedResults(t *testing.T) {
	for _, mode := range []string{"current", "changed draft", "rolled back", "dependency stage", "duplicate id", "intervening exchange", "optional outcome"} {
		t.Run(mode, func(t *testing.T) {
			m := types.NewMutableState("question stays byte-exact")
			base := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "case", Kind: types.BlockSummary, Text: "current staged contents"}}}
			m.SetPendingAnswerDocumentPatchBase(base)
			pair := func(id, name string) []llm.Message {
				return []llm.Message{{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: id, Name: name}}}, {Role: "tool", ToolCallID: id, Content: id + " result"}}
			}
			const full, patch = "emit_answer_document", "emit_answer_document_patch"
			history := finalizerRepairHistory{}
			messages := append([]llm.Message{{Role: "user", Content: m.Objective()}}, pair("old", full)...)
			first := types.ToolResult{ToolName: full, Repair: &types.ToolRepair{}}
			if mode == "optional outcome" {
				first.OptionalCarrierOutcomes = []types.OptionalCarrierOutcome{{}}
			}
			history.observe(m, messages, 1, -1, []types.ToolResult{first}, nil)
			latestID := "new"
			if mode == "duplicate id" {
				latestID = "old"
			}
			messages = append(messages, pair(latestID, patch)...)
			latest := types.ToolResult{ToolName: patch, Repair: &types.ToolRepair{Metadata: map[string]string{
				types.ToolRepairMetaAnswerDocumentPatchOutcome: types.AnswerDocumentPatchOutcomeStagedForRetry, types.ToolRepairMetaAnswerDocumentPatchFullyRevalidated: "true",
			}}}
			if mode == "rolled back" {
				latest.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] = types.AnswerDocumentPatchOutcomeNotStaged
			}
			if mode == "dependency stage" {
				delete(latest.Repair.Metadata, types.ToolRepairMetaAnswerDocumentPatchFullyRevalidated)
			}
			history.observe(m, messages, 3, -1, []types.ToolResult{latest}, nil)
			surface := llm.Message{Role: "system", Content: "exact current tool surface"}
			messages = append(messages, surface)
			switch mode {
			case "changed draft":
				base.Blocks[0].Text = "another staged generation"
				m.SetPendingAnswerDocumentPatchBase(base)
			case "intervening exchange":
				messages = append(messages, pair("read", "read_file")...)
			}
			before, _ := json.Marshal(messages)
			got := history.project(m, messages)
			if mode == "current" {
				if len(got) != 5 || !reflect.DeepEqual(got[0], messages[0]) || got[1].ToolCalls[0].ID != "new" || got[2].ToolCallID != "new" ||
					!strings.Contains(got[3].Content, "current staged contents") || !reflect.DeepEqual(got[4], surface) {
					t.Fatalf("exact current base did not replace only its superseded exchange: %+v", got)
				}
			} else if got != nil {
				t.Fatalf("unproved supersession compacted request history: %+v", got)
			}
			after, _ := json.Marshal(messages)
			if string(after) != string(before) {
				t.Fatal("projection changed original history")
			}
		})
	}
}
