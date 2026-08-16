package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// enableSummaryCapsForTest flips summary-cap master switch on for the
// duration of a test and restores the default (Enabled=false) on
// cleanup. Needed by the shrinkage-salvage cap-trim cases — they
// expect the trimmer to land at SummaryCapFor(shape, itemCount),
// which returns SummaryCapUnlimited when the switch is off.
func enableSummaryCapsForTest(t *testing.T) {
	t.Helper()
	cfg := types.DefaultSummaryCapConfig()
	cfg.Enabled = true
	types.SetSummaryCapConfig(cfg)
	t.Cleanup(func() { types.SetSummaryCapConfig(types.DefaultSummaryCapConfig()) })
}

func TestAnswerDocPrincipalEnumerationSetAuthorityLabel_PrefersTypedSelectionFamily(t *testing.T) {
	set := types.EnumerationDisplaySet{
		SelectionFamily: "public class",
		Label:           "public class (excluding abstract/sealed)",
	}
	if got := answerDocPrincipalEnumerationSetAuthorityLabel(set); got != "public class" {
		t.Fatalf("authority label = %q, want typed selection family", got)
	}
	set.SelectionFamily = ""
	if got := answerDocPrincipalEnumerationSetAuthorityLabel(set); got != set.Label {
		t.Fatalf("fallback authority label = %q, want model display label %q", got, set.Label)
	}
}

// TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersBlockContract
// pins that the dynamic prompt surfaces the V2 block contract
// (Required Answer Blocks section with at least the Summary block
// requirement). B8-T1 part 3 deleted the `## Target answer shape`
// section + resolveAnswerDocShape; the V2 block contract carries
// the equivalent guidance via Family-aware block requirements.
//
// The STATIC contract — tool name, required fields, forbidden fields
// — still lives in answer-document-skill.OutputFormat (rendered by
// context/builder.go as a system section), NOT here. Asserting those
// substrings in the dynamic prompt would resurrect the pre-cleanup
// contradiction.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersBlockContract(t *testing.T) {
	// V2 carrier renders the block contract from QuestionFamily.
	// Iterating Intent/Scenario combinations exercises the same
	// surface the pre-PR5 shape-driven test covered: the contract
	// section must surface the "Required Answer Blocks" header
	// plus a summary block requirement.
	intents := []types.Intent{
		types.IntentExplain,
		types.IntentRootCause,
		types.IntentTrace,
		types.IntentEnumerate,
		types.IntentConfigQuery,
		types.IntentReturnValue,
	}
	for _, intent := range intents {
		t.Run(string(intent), func(t *testing.T) {
			ctx := &types.AgentContext{
				AnalysisIR: &types.AnalysisIR{
					RequestModel:   types.RequestModel{Intent: intent},
					AnswerContract: types.AnswerContract{},
				},
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, "## Required Answer Blocks") {
				t.Errorf("intent=%s: prompt missing V2 block contract header: %q", intent, prompt)
			}
			if !strings.Contains(prompt, "summary") {
				t.Errorf("intent=%s: prompt missing summary block requirement: %q", intent, prompt)
			}
			// Guard against drift back to the pre-cleanup pattern:
			// the static contract MUST NOT resurface here.
			for _, banned := range []string{"emit_answer_document", "Prohibitions", "Citation pool"} {
				if strings.Contains(prompt, banned) {
					t.Errorf("intent=%s: dynamic prompt leaked static contract substring %q — "+
						"that content belongs in answer-document-skill, not the evaluator", intent, banned)
				}
			}
			// B8-T1 part 3: the legacy "## Target answer shape"
			// section is deleted; pin its absence so it can't drift back.
			if strings.Contains(prompt, "## Target answer shape") {
				t.Errorf("intent=%s: pre-B8 ## Target answer shape section resurfaced: %q", intent, prompt)
			}
		})
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRequestedCandidateRoles(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
				AnswerRoleProfile: &types.AnswerRoleProfile{
					IsRoleBindingRequested: true,
					RequiredCandidateRoles: []types.AnswerCandidateRole{
						types.AnswerCandidateRoleBudgetCap,
					},
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Typed Answer Role Contract") {
		t.Fatalf("prompt missing typed answer-role contract:\n%s", prompt)
	}
	for _, want := range []string{"budget_cap", "items[].candidate_role", "prose-only"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q in typed answer-role contract:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_ObserveHintsMissingRequestedDimensions(t *testing.T) {
	mut := types.NewMutableState("说明日志线索、当前关键代码、影响和边界")
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true,
					Dimensions: []types.RequestedAnswerDimension{
						{Label: "日志线索", Role: types.RequestedAnswerDimensionEvidenceSource, Required: true, Index: 1},
						{Label: "当前关键代码", Role: types.RequestedAnswerDimensionCurrentKeyCode, Required: true, Index: 2},
						{Label: "影响", Role: types.RequestedAnswerDimensionImpact, Required: true, Index: 3},
						{Label: "边界", Role: types.RequestedAnswerDimensionBoundary, Required: true, Index: 4},
					},
				},
			},
		},
		Mutable: mut,
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "日志线索和当前关键代码已经说明，但还没有展开影响。",
		}},
	})

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.HintRequested {
		t.Fatalf("missing requested dimensions should trigger repair hint, got %+v", sig)
	}
	if sig.StopRequested {
		t.Fatalf("dimension repair hint must not stop in the same observation: %+v", sig)
	}
	for _, want := range []string{"边界", "answer_document_patch", "do not re-open searches"} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("hint missing %q:\n%s", want, sig.Hint)
		}
	}
}

func TestRequestedSourceLocationDimensionRequiresLocationOnEveryTypedRelationMemberRow(t *testing.T) {
	mut := types.NewMutableState("show each implementation source location")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "implementers",
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Provenance: types.TypedRelationPrincipalMemberSetAggregateProvenance,
		Members:    []string{"Alpha", "Beta"},
		SupportRefs: []string{
			"Alpha @ internal/alpha.go:10",
			"Beta @ internal/beta.go:20",
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label: "source locations", Role: types.RequestedAnswerDimensionSourceLocation,
					Required: true, Index: 1,
				}},
			},
		}},
	}
	missing := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		Kind: types.BlockTable,
		Items: []types.AnswerBlockItem{
			{Label: "Alpha", Text: "first implementation"},
			{Label: "Beta", Text: "second implementation"},
		},
	}}}
	if answerDocumentCoversTypedPerMemberSourceLocations(ctx, missing) {
		t.Fatal("citations/typed support refs must not substitute for visible per-member locations")
	}
	complete := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		Kind: types.BlockTable,
		Items: []types.AnswerBlockItem{
			{Label: "Alpha", Cells: []string{"internal/alpha.go", "first implementation"}},
			{Label: "Beta", Cells: []string{"internal/beta.go", "second implementation"}},
		},
	}}}
	if !answerDocumentCoversTypedPerMemberSourceLocations(ctx, complete) {
		t.Fatal("every typed member row carries its own visible source path")
	}
}

func TestRequestedSourceInventoryLocationAndAttributeDimensionsUseExactTypedRows(t *testing.T) {
	mut := types.NewMutableState("list file and package per declaration")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "public class",
		Value:      "2",
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Provenance: types.SourceInventoryPrincipalRowSetAggregateProvenance,
		Members: []string{
			"Alpha @ src/alpha.cj:10",
			"Beta @ src/beta.cj:20",
		},
		SupportRefs: []string{
			"Alpha @ src/alpha.cj:10",
			"Beta @ src/beta.cj:20",
		},
	}})
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active: true, Complete: true,
		Sets: []types.SourceInventoryObservationSet{{
			Role: types.AnswerCandidateRoleType, Complete: true, Count: 2, Total: 2,
			Members: []types.SourceInventoryObservationMember{
				{Name: "Alpha", Role: types.AnswerCandidateRoleType, File: "src/alpha.cj", Line: 10, Attributes: []types.SourceInventoryObservationAttribute{{Role: types.AnswerCandidateRolePackage, Name: "demo.alpha"}}},
				{Name: "Beta", Role: types.AnswerCandidateRoleType, File: "src/beta.cj", Line: 20, Attributes: []types.SourceInventoryObservationAttribute{{Role: types.AnswerCandidateRolePackage, Name: "demo.beta"}}},
			},
		}},
	})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentEnumerate,
			Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
					types.SourceInventoryFieldPackage,
				},
			},
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{
					{Index: 1, Label: "file", Role: types.RequestedAnswerDimensionSourceLocation, Required: true},
					{Index: 2, Label: "package", Role: types.RequestedAnswerDimensionSourceAttribute, Required: true},
				},
			},
		}},
	}
	sets := answerDocPrincipalEnumerationSets(ctx, answerSurfacePlan(ctx))
	if len(sets) != 1 || len(sets[0].Rows) != 2 {
		t.Fatalf("typed source-inventory rows unavailable: %+v", sets)
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{Kind: types.BlockSection}}}
	for _, row := range sets[0].Rows {
		if len(row.Attributes) != 1 {
			t.Fatalf("row attributes unavailable: %+v", row)
		}
		doc.Blocks[0].Items = append(doc.Blocks[0].Items, types.AnswerBlockItem{
			Label:                row.DisplayLabel,
			Text:                 row.Location + "; package=" + row.Attributes[0].Name,
			SourceInventoryRowID: row.RowID,
		})
	}
	if !answerDocumentCoversTypedPerMemberSourceLocations(ctx, doc) {
		t.Fatal("exact row identities with visible typed paths should cover source_location")
	}
	if !answerDocumentCoversTypedPerMemberSourceAttributes(ctx, doc) {
		t.Fatal("exact row identities with visible typed package values should cover source_attribute")
	}
	doc.Blocks[0].Items[1].Text = sets[0].Rows[1].Location
	if answerDocumentCoversTypedPerMemberSourceAttributes(ctx, doc) {
		t.Fatal("omitting one exact row-local package value must remain a soft presentation miss")
	}
}

func TestAnswerDocumentEvaluator_ObserveStopsWhenPresentationOnlyDimensionMissing(t *testing.T) {
	mut := types.NewMutableState("说明影响")
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true,
					Dimensions: []types.RequestedAnswerDimension{{
						Label:    "影响",
						Role:     types.RequestedAnswerDimensionImpact,
						Required: true,
						Index:    1,
					}},
				},
			},
		},
		Mutable: mut,
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "核心机制已经说明。",
		}},
	})

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.StopRequested || sig.HintRequested {
		t.Fatalf("presentation-only requested dimensions should not trigger a second finalizer round, got %+v", sig)
	}
}

func TestAnswerDocumentEvaluator_ObserveStopsWhenEvidenceSourceDimensionHasTypedCarrier(t *testing.T) {
	mut := types.NewMutableState("说明证据边界说明")
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true,
					Dimensions: []types.RequestedAnswerDimension{{
						Label:    "证据边界说明",
						Role:     types.RequestedAnswerDimensionEvidenceSource,
						Required: true,
						Index:    1,
					}},
				},
			},
		},
		Mutable: mut,
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "summary",
				Kind:        types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "当前源码说明了解析机制；attached trace 说明了运行时耗时。",
				ClaimUses: []types.RenderedClaimUse{{
					ClaimForm: types.ClaimExternalObservation,
				}},
			},
			{
				ID:   "scope",
				Kind: types.BlockCaveat,
				Text: "边界：运行时观察不映射到当前 checkout 的源码行。",
			},
		},
		Citations: []types.Citation{{
			File: "internal/tracequery/parse.go",
			Line: 1980,
		}},
	})

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.StopRequested || sig.HintRequested {
		t.Fatalf("typed evidence carrier should satisfy evidence_source dimension without retry, got %+v", sig)
	}
}

func TestAnswerDocumentEvaluator_ObserveHintsMissingEvidenceSourceCarrier(t *testing.T) {
	mut := types.NewMutableState("说明证据边界说明")
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true,
					Dimensions: []types.RequestedAnswerDimension{{
						Label:    "证据边界说明",
						Role:     types.RequestedAnswerDimensionEvidenceSource,
						Required: true,
						Index:    1,
					}},
				},
			},
		},
		Mutable: mut,
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "当前结论是性能异常。",
		}},
	})

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.HintRequested || sig.HintKey != "answer_doc.requested_dimensions" {
		t.Fatalf("missing typed evidence carrier should still get one precise repair hint, got %+v", sig)
	}
}

func TestAnswerDocumentEvaluator_ObserveStopsWhenRequestedDimensionsVisible(t *testing.T) {
	mut := types.NewMutableState("说明日志线索、当前关键代码、影响和边界")
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true,
					Dimensions: []types.RequestedAnswerDimension{
						{Label: "日志线索", Role: types.RequestedAnswerDimensionEvidenceSource, Required: true, Index: 1},
						{Label: "当前关键代码", Role: types.RequestedAnswerDimensionCurrentKeyCode, Required: true, Index: 2},
						{Label: "影响", Role: types.RequestedAnswerDimensionImpact, Required: true, Index: 3},
						{Label: "边界", Role: types.RequestedAnswerDimensionBoundary, Required: true, Index: 4},
					},
				},
			},
		},
		Mutable: mut,
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:    "t1",
			Kind:  types.BlockTable,
			Title: "维度核对",
			Columns: []string{
				"日志线索",
				"当前关键代码",
				"影响",
				"边界",
			},
			Items: []types.AnswerBlockItem{{
				ID:    "r1",
				Cells: []string{"log", "code", "impact", "scope"},
			}},
		}},
	})

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.StopRequested || sig.HintRequested {
		t.Fatalf("complete requested dimensions should stop without hint, got %+v", sig)
	}
}

func TestAnswerDocumentEvaluator_ObserveStopsWhenTypedCountAndMemberSetShapesPresent(t *testing.T) {
	mut := types.NewMutableState("给出总数和完整成员名")
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsCountQuestion:       true,
				},
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true,
					Dimensions: []types.RequestedAnswerDimension{
						{Label: "总数", Role: types.RequestedAnswerDimensionCount, Required: true, Index: 1},
						{Label: "完整成员名", Role: types.RequestedAnswerDimensionMemberSet, Required: true, Index: 2},
					},
				},
			},
		},
		Mutable: mut,
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "members",
				Kind:  types.BlockOrderedList,
				Items: []types.AnswerBlockItem{{ID: "explorer", Label: "explorer"}},
			},
			{ID: "count", Kind: types.BlockScalar, Text: "1"},
		},
	})

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.StopRequested || sig.HintRequested {
		t.Fatalf("typed count/member-set structural carriers should stop without dimension repair hint, got %+v", sig)
	}
}

func TestAnswerDocumentEvaluator_ObserveStopsWhenBoundaryDimensionHasTypedBoundaryCarrier(t *testing.T) {
	mut := types.NewMutableState("说明窗口统计和平台时基")
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true,
					Dimensions: []types.RequestedAnswerDimension{
						{Label: "窗口统计", Role: types.RequestedAnswerDimensionStageWorkflow, SourceQuote: "窗口统计", Required: true, Index: 1},
						{Label: "平台时基", Role: types.RequestedAnswerDimensionBoundary, SourceQuote: "平台时基", Required: true, Index: 2},
					},
				},
			},
		},
		Mutable: mut,
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "s1",
				Kind: types.BlockSummary,
				Text: "窗口统计显示时间戳单位为秒，窗口跨度约 0.452ms。",
			},
			{
				ID:       "b1",
				Kind:     types.BlockCaveat,
				Text:     "边界：这个结论只覆盖当前 trace 窗口和已观测线程。",
				FacetIDs: []string{string(types.FacetUncertaintyBoundary)},
			},
		},
	})

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.StopRequested || sig.HintRequested {
		t.Fatalf("typed boundary carrier should satisfy boundary presentation dimension, got %+v", sig)
	}
}

func TestAnswerDocumentEvaluator_ObserveHintsMissingExternalObservationSelectorValue(t *testing.T) {
	mut := types.NewMutableState("explain mcp line 12")
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}},
		Mutable:    mut,
		MCPResponses: []types.MCPResponse{{
			ServerName:  "fixture",
			Method:      "tools/call:lookup_trace_fact",
			Success:     true,
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
			MIMEType:    "application/vnd.codrax.observation+json",
			Observations: []types.MCPTypedObservation{{
				Summary:     "helper wakes target",
				ResourceURI: "mcp://fixture/trace/sleep-wakeup",
				LineStart:   12,
				LineEnd:     12,
				Selector:    "pid=4242 event=sched_wakeup waker=helper",
				RawRef:      "mcp://fixture/trace/sleep-wakeup#L12",
			}},
		}},
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "line 12 是 sched_wakeup，worker-100 唤醒 pid=4242。",
		}},
	})

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.HintRequested {
		t.Fatalf("missing external selector value should trigger repair hint, got %+v", sig)
	}
	for _, want := range []string{"waker=helper", "helper", "mcp://fixture/trace/sleep-wakeup"} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("selector hint missing %q:\n%s", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_ObserveStopsWhenExternalObservationSelectorValueVisible(t *testing.T) {
	mut := types.NewMutableState("explain mcp line 12")
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}},
		Mutable:    mut,
		MCPResponses: []types.MCPResponse{{
			ServerName:  "fixture",
			Method:      "tools/call:lookup_trace_fact",
			Success:     true,
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
			MIMEType:    "application/vnd.codrax.observation+json",
			Observations: []types.MCPTypedObservation{{
				Summary:     "helper wakes target",
				ResourceURI: "mcp://fixture/trace/sleep-wakeup",
				LineStart:   12,
				LineEnd:     12,
				Selector:    "pid=4242 event=sched_wakeup waker=helper",
				RawRef:      "mcp://fixture/trace/sleep-wakeup#L12",
			}},
		}},
	}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "line 12 是 sched_wakeup，helper 唤醒 pid=4242。",
		}},
	})

	sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !sig.StopRequested || sig.HintRequested {
		t.Fatalf("complete external selector values should stop without hint, got %+v", sig)
	}
}

func TestSelectorCoverageValues_ExcludesTraceArtifactProvenanceCoordinates(t *testing.T) {
	selector := "pid=2000 event=span_window;artifact_spans=/private/work/.codrax/blob/trace.txt:5-6[trace_seconds]"
	got := selectorCoverageValues(selector)
	joined := strings.Join(got, "|")
	for _, want := range []string{"span_window"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("semantic selector value %q missing from %v", want, got)
		}
	}
	for _, forbidden := range []string{"/private/work", "trace.txt", "trace_seconds"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("provenance coordinate %q must not become visible-answer coverage: %v", forbidden, got)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_ExclusionPolicyHidesConcreteCandidates(t *testing.T) {
	mut := types.NewMutableState("list public symbols")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:     types.AnswerAggregateExcluded,
		Label:    "excluded variables",
		Value:    "2",
		Excluded: []string{"registered", "defaultExternalArtifactFloor"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
					IsExclusionRequested: true,
					ExcludedCandidateRoles: []types.AnswerCandidateRole{
						types.AnswerCandidateRoleVariable,
					},
				},
			},
		},
		Mutable: mut,
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Typed Exclusion Policy",
		"Do not name concrete excluded candidates anywhere in the visible answer",
		"excluded_candidates=omitted_by_typed_exclusion_policy",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q for typed exclusion policy:\n%s", want, prompt)
		}
	}
	for _, banned := range []string{"registered", "defaultExternalArtifactFloor"} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("prompt leaked concrete excluded candidate %q:\n%s", banned, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersSourceInventoryHandoff(t *testing.T) {
	mut := types.NewMutableState("list module entrypoints")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:     true,
		Complete:   true,
		Scopes:     []string{"services"},
		Provenance: []string{"repo_map.source_inventory"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRolePackage,
			Complete: true,
			Count:    2,
			Members: []types.SourceInventoryObservationMember{{
				Name:          "billing",
				File:          "services/billing/index.ts",
				Line:          12,
				Language:      "typescript",
				CoverageState: types.SourceInventoryCoverageObserved,
				Attributes: []types.SourceInventoryObservationAttribute{{
					Name:          "createBillingService",
					File:          "services/billing/index.ts",
					Line:          16,
					Language:      "typescript",
					CoverageState: types.SourceInventoryCoverageObserved,
				}},
			}, {
				Name:          "identity",
				File:          "services/identity/main.py",
				Line:          7,
				Language:      "python",
				CoverageState: types.SourceInventoryCoverageObserved,
			}},
		}},
	})
	ctx := &types.AgentContext{
		Objective:             "use trace_query window_stats to analyse sched state_churn for app-20",
		AttachedHitrace:       "sched_switch app-20 rival-30",
		AttachedHitraceSource: "harmony_hitrace",
		Mutable:               mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
				},
				Confidence: 0.95,
			},
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Repo Lens Candidate Universe Handoff",
		"verifies navigation facts",
		"not final answer text",
		types.SourceInventoryMechanicalFactBoundary,
		"model-selected slate",
		"principal_roles: `package`",
		"row_lanes: principal=2, support=0, audit=0",
		"### Principal candidate rows",
		"member=`billing`, role=package, source_class=production, language=typescript, location=`services/billing/index.ts:12`, coverage_state=observed, attributes=[`attribute:createBillingService @ services/billing/index.ts:16`]",
		"member=`identity`, role=package, source_class=production, language=python, location=`services/identity/main.py:7`, coverage_state=observed",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("source-inventory handoff missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersPrincipalRowAttributes(t *testing.T) {
	mut := types.NewMutableState("list cangjie entrypoints and packages")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "entrypoints",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Unit:        "function",
		Members:     []string{"extend Cart"},
		SupportRefs: []string{"extend Cart @ src/cart/cart.cj:30"},
	}})
	mut.SetInvestigationComplete("structured source inventory member set accepted")
	mut.SetInvestigationResultKind("resolved")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"src"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Count:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:          "extend Cart",
				File:          "src/cart/cart.cj",
				Line:          30,
				Language:      "cangjie",
				SurfaceTerms:  []string{"extend", "extend Cart"},
				CoverageState: types.SourceInventoryCoverageObserved,
				Attributes: []types.SourceInventoryObservationAttribute{{
					Name:          "demo.cart",
					Role:          types.AnswerCandidateRolePackage,
					File:          "src/cart/cart.cj",
					Language:      "cangjie",
					CoverageState: types.SourceInventoryCoverageObserved,
				}},
			}},
		}},
	})
	ctx := &types.AgentContext{
		Objective: "list cangjie entrypoints and packages",
		Mutable:   mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
				},
				Confidence: 0.95,
			},
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Principal Enumeration Rows",
		"member=`extend Cart`",
		"surface_family=`extend`",
		"source_inventory_family",
		"attributes=[`package:demo.cart`]",
		"do not infer them from paths",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q for row attributes:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersTurnASourceInventoryAdvisoryHandoff(t *testing.T) {
	mut := types.NewMutableState("explain configuration routes")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		SourceInventoryAdvisory: types.SourceInventoryAdvisory{
			Active:       true,
			AdvisoryOnly: true,
			Complete:     true,
			Scopes:       []string{"routes"},
			Provenance:   []string{"repo_lens:tool_query"},
			Sets: []types.SourceInventoryAdvisorySet{{
				Role:     types.AnswerCandidateRoleRoute,
				Complete: true,
				Candidates: []types.SourceInventoryAdvisoryCandidate{{
					Member:     "GET /v1/users",
					SupportRef: "GET /v1/users: routes/users.ts:42",
					File:       "routes/users.ts",
					Line:       42,
					Language:   "typescript",
				}},
			}},
		},
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Repo Lens Candidate Universe Handoff",
		"scopes: `routes`",
		"provenance: `repo_lens:tool_query`",
		"authority_reason_codes: `inactive`",
		"role `route` (route): count=1 len(members)=1 complete=true",
		"member=`GET /v1/users` @ `routes/users.ts:42`, language=typescript, coverage_state=observed",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("TurnA source-inventory advisory handoff missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_PrincipalMemberSetSuppressesClosureProse(t *testing.T) {
	mut := types.NewMutableState("list public symbols")
	mut.SetInvestigationComplete("complete public set; variables such as defaultExternalArtifactFloor were excluded")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "public functions",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Eval"},
		SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15"},
	}, {
		Kind:     types.AnswerAggregateExcluded,
		Label:    "excluded variables",
		Value:    "1",
		Excluded: []string{"defaultExternalArtifactFloor"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
					IsExclusionRequested: true,
					ExcludedCandidateRoles: []types.AnswerCandidateRole{
						types.AnswerCandidateRoleVariable,
					},
				},
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
		Mutable: mut,
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "defaultExternalArtifactFloor") {
		t.Fatalf("finalizer prompt should not project unstructured closure prose candidates when principal member_set exists:\n%s", prompt)
	}
	if !strings.Contains(prompt, "model-authored closure set-level summary") ||
		!strings.Contains(prompt, "[excluded candidate omitted]") ||
		!strings.Contains(prompt, "typed `aggregate_facts.member_set` rows/counts below remain the authoritative member carrier") {
		t.Fatalf("finalizer prompt should preserve sanitized tool-call closure prose as set-level advisory context:\n%s", prompt)
	}
	if !strings.Contains(prompt, "member=`Eval`") ||
		!strings.Contains(prompt, "members_rendered_in=authoritative_principal_member_rows") {
		t.Fatalf("principal member_set should render once as authoritative rows with compact metadata:\n%s", prompt)
	}
	if strings.Contains(prompt, "members=[`Eval`]") {
		t.Fatalf("structured aggregate metadata must not duplicate principal member rows:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_HistoryMemberSetKeepsClosureProse(t *testing.T) {
	mut := types.NewMutableState("最近 10 次提交都做了什么")
	mut.SetInvestigationComplete("ae1dd6b 统一 VCS 证据通道；3ae8465 调整重试路由。")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "最近10次提交",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"ae1dd6b", "3ae8465"},
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "evidence_origin", Value: "vcs_metadata"},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
			},
		},
		Mutable: mut,
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "model-authored closure reason:") ||
		!strings.Contains(prompt, "统一 VCS 证据通道") {
		t.Fatalf("VCS history member_set may be identity-only; finalizer should still see rich closure prose:\n%s", prompt)
	}
	if strings.Contains(prompt, "model-authored closure reason omitted") {
		t.Fatalf("history member_set must not suppress closure prose:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRequiredMechanismAnchors(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				AnalyzerHints: types.AnalyzerHints{
					ExactTargets:      []string{"runTaskGraph"},
					MentionedEntities: []string{"runTaskGraph"},
				},
			},
			AnswerContract: types.AnswerContract{
				MustIncludeTerms: []types.ContractTerm{{
					Text: "runTaskGraph",
					Kind: types.ContractTermSymbol,
				}},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Typed Mechanism Anchor Contract") {
		t.Fatalf("prompt missing typed mechanism-anchor contract:\n%s", prompt)
	}
	if !strings.Contains(prompt, "## Preferred Visible Anchors (Pre-flight Guide)") {
		t.Fatalf("visible-anchor guide missing:\n%s", prompt)
	}
	for _, forbidden := range []string{
		"## Visible-Anchor Whitelist (Authoritative)",
		"COMPLETE pre-flight set",
		"Surfaces outside this slate will fail",
		"Surfaces NOT in this slate",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("visible-anchor guide should not frame the bounded prompt slate as an exclusive whitelist (%q):\n%s", forbidden, prompt)
		}
	}
	for _, want := range []string{"runTaskGraph", "blocks[].items[].label", "edge_anchors", "prose-only"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q in typed mechanism-anchor contract:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocDynamicTrace_AppendSectionPreservesMarkdownBoundary(t *testing.T) {
	var b strings.Builder
	trace := newAnswerDocDynamicTrace(&types.AgentContext{Stage: types.StageFinalize})
	if !trace.appendSection(&b, "first", func() string { return "candidate-only guidance." }) {
		t.Fatal("first section unexpectedly stopped")
	}
	if !trace.appendSection(&b, "second", func() string { return "## Preferred Visible Anchors" }) {
		t.Fatal("second section unexpectedly stopped")
	}
	if got, want := b.String(), "candidate-only guidance.\n\n## Preferred Visible Anchors"; got != want {
		t.Fatalf("dynamic prompt section boundary = %q, want %q", got, want)
	}

	var existingBoundary strings.Builder
	existingBoundary.WriteString("first\n")
	(*answerDocDynamicTrace)(nil).appendSection(&existingBoundary, "second", func() string {
		return "\n## Second"
	})
	if got, want := existingBoundary.String(), "first\n\n## Second"; got != want {
		t.Fatalf("existing section newlines should be normalized to one blank line: got %q, want %q", got, want)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersTypedCallChainEndpointBoundary(t *testing.T) {
	mut := types.NewMutableState("typed no-directed-path boundary")
	mut.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
		Reason:    types.PrincipalSpanWaiverNoDirectedPath,
		Rationale: "buildAnalysisIR reaches gate.RunWith while gate.Run calls RunWith",
	})
	const stalePathClaim = "STALE_PATH_CLAIM_buildAnalysisIR_reaches_gate_Run"
	mut.SetInvestigationComplete(stalePathClaim)
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   stalePathClaim,
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"buildAnalysisIR", "gate.Run"},
	}})
	mut.RetainInvestigationAggregateFacts()
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		InvestigationNotes:    []string{"Previous accepted closure reason (preserved advisory, not a citation): " + stalePathClaim},
		AcceptedClosureReason: stalePathClaim,
		EvidenceItems: []types.EvidenceItem{
			{ID: "E-call-source", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2666, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
			{ID: "E-call-sink", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "gate.Run", Object: "RunWith", AnchorSymbol: "RunWith", Source: "internal/analysis/gate/gate.go", LineStart: 134, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
			{ID: "E-sibling", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Object: "normalizer.Normalize", AnchorSymbol: "normalizer.Normalize", Source: "internal/agent/analyzer.go", LineStart: 2321, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		},
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   stalePathClaim,
			Value:   "2",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"buildAnalysisIR", "gate.Run"},
		}},
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:                   types.IntentTrace,
			PredicateAxis:            types.AxisCall,
			CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "buildAnalysisIR", Sink: "gate.Run"},
			AnalyzerHints: types.AnalyzerHints{
				Kind:         string(types.ReqCallChain),
				ExactTargets: []string{"buildAnalysisIR", "gate.Run"},
			},
		}},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Typed Call-Chain Endpoint Boundary",
		"disposition=`no_directed_path`",
		"source_endpoint=`buildAnalysisIR`",
		"requested_sink=`gate.Run`",
		"call_graph_status=`shared_callee_boundary`",
		"grounded_call_edge_count=`3`",
		"source_endpoint_existence_proof=`call_edge`",
		"requested_sink_existence_proof=`call_edge`",
		"shared_callee=`gate.RunWith`",
		"directed_topology_shape=`buildAnalysisIR -> ... -> gate.RunWith <- ... <- gate.Run`",
		"each have a grounded same-direction call path ending at the same callee",
		"source_path: `buildAnalysisIR` -> `gate.RunWith` [E-call-source] @ internal/agent/analyzer.go:2666",
		"requested_sink_path: `gate.Run` -> `gate.RunWith` [E-call-sink] @ internal/analysis/gate/gate.go:134",
		"not a reachable-chain member declaration",
		"reverse or shared-callee typed calls",
		"both arrowheads end at the same callee",
		"do not prove parallel execution, convergence, a join",
		"Never rewrite `source -> ... -> shared_callee <- ... <- requested_sink`",
		"principal_relation_scope=`typed_endpoint_boundary`",
		"supporting_directed_relations_outside_boundary=`1`",
		"separate supporting item/block",
		"model owns the conclusion",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("typed endpoint-boundary prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "node_alias[n4]=`RunWith`") {
		t.Fatalf("one canonical call endpoint must not be republished under a short duplicate identity:\n%s", prompt)
	}
	if strings.Contains(prompt, "node_alias[n4]=`normalizer.Normalize`") ||
		strings.Contains(prompt, "node_alias[n3]=`normalizer.Normalize`") {
		t.Fatalf("same-caller sibling must stay outside the principal endpoint-boundary recipe:\n%s", prompt)
	}
	if !strings.Contains(prompt, "verified_relation_component_count=1") ||
		!strings.Contains(prompt, "node_alias[n2]=`gate.RunWith`") ||
		!strings.Contains(prompt, `"to_identity":"RunWith"`) {
		t.Fatalf("relation recipe did not reuse the shared canonical endpoint identity:\n%s", prompt)
	}
	if strings.Contains(prompt, "buildAnalysisIR reaches gate.RunWith while gate.Run calls RunWith") {
		t.Fatalf("free-form waiver rationale must remain audit-only, not prompt authority:\n%s", prompt)
	}
	if strings.Contains(prompt, stalePathClaim) {
		t.Fatalf("no-directed-path finalizer context must omit contradictory model path rosters/reasons while retaining them in audit state:\n%s", prompt)
	}
	if got := mut.StableInvestigationAggregateFacts(); len(got) != 1 || got[0].Label != stalePathClaim {
		t.Fatalf("answer-authority projection must not delete accepted audit aggregates: %+v", got)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersCallChainTargetDiscoveryBoundary(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentTrace,
		PredicateAxis: types.AxisCall,
		CallChainEndpointProfile: &types.CallChainEndpointProfile{
			Source: "run_pipeline", SinkMode: types.CallChainSinkResolutionDiscover,
		},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
	}}}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Call-chain runtime target discovery",
		"grounded source endpoint: `run_pipeline`",
		"destination mode: `discover`",
		"registration/binding is not a source-level call",
		"runtime class from the class or mixin that owns an inherited method definition",
		"model owns the final destination conclusion",
		"If the dynamic boundary cannot be drawn without turning a binding, return, inheritance, or method-owner relation into a call arrow, omit the diagram",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("target-discovery prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Typed Call-Chain Endpoint Boundary") {
		t.Fatalf("discover mode must not fabricate an exact no-directed-path endpoint boundary:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_RuntimeSelectionWithoutEndpointPairUsesNeutralBoundary(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		CallChainEndpointProfile: &types.CallChainEndpointProfile{
			SinkMode:                    types.CallChainSinkResolutionExact,
			RuntimeSelectionRequired:    true,
			RuntimeSelectionSourceQuote: "initial/full-output attempt versus a retry/error/patch attempt",
		},
	}}}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Runtime selection evidence",
		"does not declare a source-to-sink call-chain endpoint pair",
		"Do not invent endpoint identities",
		"model owns the final destination conclusion",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("runtime-only answer boundary missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "exact requested endpoints: `` -> ``") || strings.Contains(prompt, "## Call-chain runtime selection evidence") {
		t.Fatalf("runtime-only selection must not render synthetic empty endpoints:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_DiscoverPathReceivesTypedRelationCompositionWithoutEndpointAuthority(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			CallChainEndpointProfile: &types.CallChainEndpointProfile{
				SinkMode: types.CallChainSinkResolutionDiscoverPath,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
		EvidenceItems: []types.EvidenceItem{
			{ID: "E-static", Kind: types.EvidenceRelationship, Subject: "Logger.log", Predicate: "calls", Object: "Sink.write", Source: "src/logger.cpp", LineStart: 36, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
			{ID: "E-type", Kind: types.EvidenceRelationship, Subject: "ConsoleSink", Predicate: "inheritance", Object: "Sink", Source: "include/logx/console_sink.hpp", LineStart: 8, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerRepoMapStructuralRelation},
			{ID: "E-impl", Kind: types.EvidenceRelationship, Subject: "ConsoleSink.write", Predicate: "calls", Object: "std::fputs", Source: "include/logx/console_sink.hpp", LineStart: 11, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
			{ID: "E-return", Kind: types.EvidenceConcrete, Subject: "SinkRegistry.create", Predicate: "returns", Object: "std::make_unique<ConsoleSink>()", Source: "src/registry.cpp", LineStart: 18, Scope: types.ScopeLine, AnchorKind: types.AnchorReturn, Producer: "concrete_values"},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Call-chain role-bound relation composition",
		"endpoint mode: `discover_path`",
		"not preselected code endpoints",
		"do not promote a parser candidate into an endpoint, required hop, or final conclusion",
		"### Typed relation capsule (bounded, no synthetic edges)",
		"family=`declared_type_relation` relation=`inheritance` subject=`ConsoleSink` object=`Sink` [E-type] @ include/logx/console_sink.hpp:8",
		"### Typed dynamic-dispatch compositions (candidate-only)",
		"static_call=`Logger.log -> Sink.write` [E-static] @ src/logger.cpp:36",
		"type_relation=`ConsoleSink -> Sink` [E-type] @ include/logx/console_sink.hpp:8",
		"implementation_body_call=`ConsoleSink.write -> std::fputs` [E-impl] @ include/logx/console_sink.hpp:11",
		"runtime_selection_status=`conditional`",
		"keep `static_call` and `implementation_body_call` as separate call edges",
		"The model decides whether the binding and current request prove a selected runtime target",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("discover_path relation composition missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"## Call-chain runtime target discovery",
		"grounded source endpoint:",
		"## Typed Call-Chain Endpoint Boundary",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("discover_path must not inherit endpoint authority %q:\n%s", forbidden, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_TargetDiscoveryRendersTypedRelationFamiliesWithoutSyntheticCall(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			CallChainEndpointProfile: &types.CallChainEndpointProfile{
				Source: "run_pipeline", SinkMode: types.CallChainSinkResolutionDiscover,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
		EvidenceItems: []types.EvidenceItem{
			{ID: "E-call", Kind: types.EvidenceRelationship, Subject: "run_pipeline", Predicate: "calls", Object: "resolve", Source: "pipeline/runner.py", LineStart: 15, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
			{ID: "E-bind", Kind: types.EvidenceRegistration, Subject: `REGISTRY["json"]`, Predicate: "binds", Object: "JsonPlugin", Source: "pipeline/plugins.py", LineStart: 7, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition},
			{ID: "E-bind-sparse", Kind: types.EvidenceRegistration, Predicate: "binds", Object: "YamlPlugin", Source: "pipeline/plugins.py", LineStart: 9, Scope: types.ScopeLine, AnchorKind: types.AnchorStringLiteral, AnchorSymbol: "yaml"},
			{ID: "E-inherit-3", Kind: types.EvidenceRelationship, Subject: "JsonPlugin", Predicate: "inheritance", Object: "BasePlugin", Source: "pipeline/plugins.py", LineStart: 8, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, Producer: "repomap_structural_relation", RelationOrdinal: 3},
			{ID: "E-inherit-1", Kind: types.EvidenceRelationship, Subject: "JsonPlugin", Predicate: "inheritance", Object: "TimestampMixin", Source: "pipeline/plugins.py", LineStart: 8, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, Producer: "repomap_structural_relation", RelationOrdinal: 1},
			{ID: "E-inherit-2", Kind: types.EvidenceRelationship, Subject: "JsonPlugin", Predicate: "inheritance", Object: "ValidationMixin", Source: "pipeline/plugins.py", LineStart: 8, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, Producer: "repomap_structural_relation", RelationOrdinal: 2},
			{ID: "E-return", Kind: types.EvidenceConcrete, Subject: "resolve", Predicate: "returns", Object: "cls()", Source: "pipeline/registry.py", LineStart: 19, Scope: types.ScopeLine, AnchorKind: types.AnchorReturn, Producer: "concrete_values"},
			{ID: "E-noise", Kind: types.EvidenceConcrete, Subject: "CsvPlugin.content_type", Predicate: "returns", Object: `"text/csv"`, Source: "pipeline/plugins.py", LineStart: 14, Scope: types.ScopeLine, AnchorKind: types.AnchorReturn, Producer: "concrete_values"},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"### Typed relation capsule (bounded, no synthetic edges)",
		"family=`registration_or_binding` relation=`binds` subject=`REGISTRY[\"json\"]` object=`JsonPlugin` [E-bind] @ pipeline/plugins.py:7",
		"family=`registration_or_binding` relation=`binds` subject=`yaml` object=`YamlPlugin` [E-bind-sparse] @ pipeline/plugins.py:9",
		"family=`declared_type_relation` relation=`inheritance` subject=`JsonPlugin` object=`TimestampMixin` [E-inherit-1] @ pipeline/plugins.py:8 declared_order=`1`",
		"family=`declared_type_relation` relation=`inheritance` subject=`JsonPlugin` object=`ValidationMixin` [E-inherit-2] @ pipeline/plugins.py:8 declared_order=`2`",
		"family=`declared_type_relation` relation=`inheritance` subject=`JsonPlugin` object=`BasePlugin` [E-inherit-3] @ pipeline/plugins.py:8 declared_order=`3`",
		"family=`value_or_factory_flow` relation=`returns` subject=`resolve` object=`cls()` [E-return] @ pipeline/registry.py:19 expression_form=`call_result`",
		"family=`static_call` relation=`calls` subject=`run_pipeline` object=`resolve` [E-call] @ pipeline/runner.py:15",
		"only `family=static_call` is a source-level invocation edge",
		"neither alone proves that the registry selected that type at runtime",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("target-discovery relation capsule missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"run_pipeline` -> `JsonPlugin",
		"resolve` -> `JsonPlugin",
		"JsonPlugin` -> `TimestampMixin",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("typed non-call relations must not be fused into a synthetic call path %q:\n%s", forbidden, prompt)
		}
	}
	capsule := renderAnswerDocRuntimeTargetRelationCapsule(ctx)
	if first, second, third := strings.Index(capsule, "object=`TimestampMixin`"), strings.Index(capsule, "object=`ValidationMixin`"), strings.Index(capsule, "object=`BasePlugin`"); first < 0 || second <= first || third <= second {
		t.Fatalf("typed declared relation roster must render in parser/source order despite ranked input order:\n%s", capsule)
	}
	for _, noise := range []string{"CsvPlugin.content_type", "text/csv"} {
		if strings.Contains(capsule, noise) {
			t.Fatalf("unconnected value fact consumed runtime-target capsule budget %q:\n%s", noise, capsule)
		}
	}
}

func TestAnswerDocReturnExpressionForm_CrossLanguageCallResults(t *testing.T) {
	for _, expression := range []string{
		"cls()",                    // Python / Ruby
		"new JsonPlugin()",         // Java / Kotlin / ArkTS / TypeScript
		"std::make_unique<Sink>()", // C++
		"Box::new(worker)",         // Rust
		"await factory.create()",   // ArkTS / JavaScript
		"Handler()",                // Cangjie / Swift / Go-style factory
	} {
		item := types.EvidenceItem{AnchorKind: types.AnchorReturn, Object: expression}
		if got := answerDocReturnExpressionForm(item); got != "call_result" {
			t.Fatalf("return expression %q classified as %q, want call_result", expression, got)
		}
	}
	for _, expression := range []string{"JsonPlugin", "handler", `"json"`, "42", "(left, right)"} {
		item := types.EvidenceItem{AnchorKind: types.AnchorReturn, Object: expression}
		if got := answerDocReturnExpressionForm(item); got != "" {
			t.Fatalf("non-call return expression %q must remain unclassified, got %q", expression, got)
		}
	}
}

func TestRenderAnswerDocTypedExplorationEnrichment_ReturnKeepsExpressionAndForm(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentReturnValue,
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "E-return", Kind: types.EvidenceConcrete, Subject: "Registry.resolve",
			Predicate: "returns", Object: "cls()", Source: "pipeline/registry.py",
			LineStart: 19, Scope: types.ScopeLine, AnchorKind: types.AnchorReturn,
			Producer: "concrete_values",
		}},
	}
	got := renderAnswerDocTypedExplorationEnrichment(ctx, false)
	for _, want := range []string{
		"lane=value_fact label=`Registry.resolve` @ pipeline/registry.py:19",
		"Registry.resolve returns cls()",
		"expression_form=`call_result`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed return context missing %q:\n%s", want, got)
		}
	}
}

func TestAnswerDocumentEvaluator_TargetDiscoveryRendersTypedDynamicDispatchComposition(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			CallChainEndpointProfile: &types.CallChainEndpointProfile{
				Source: "Facade.run", SinkMode: types.CallChainSinkResolutionDiscover,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
		EvidenceItems: []types.EvidenceItem{
			{ID: "E-static", Kind: types.EvidenceRelationship, Subject: "Facade.run", OwnerSymbol: "Facade.run", Predicate: "calls", Object: "BaseSink.write", Source: "src/facade.ext", LineStart: 20, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
			{ID: "E-type", Kind: types.EvidenceRelationship, Subject: "ConsoleSink", Predicate: "implements", Object: "BaseSink", Source: "src/console.ext", LineStart: 4, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerRepoMapImplementerRelation},
			{ID: "E-body", Kind: types.EvidenceRelationship, Subject: "ConsoleSink.write", OwnerSymbol: "ConsoleSink.write", Predicate: "calls", Object: "IO.write", Source: "src/console.ext", LineStart: 9, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
			{ID: "E-bind", Kind: types.EvidenceRegistration, Subject: "SinkFactory.create", Predicate: "binds", Object: "ConsoleSink", Source: "src/factory.ext", LineStart: 11, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"### Typed dynamic-dispatch compositions (candidate-only)",
		"static_call=`Facade.run -> BaseSink.write` [E-static] @ src/facade.ext:20",
		"type_relation=`ConsoleSink -> BaseSink` [E-type] @ src/console.ext:4",
		"implementation_body_call=`ConsoleSink.write -> IO.write` [E-body] @ src/console.ext:9",
		"binding_candidate=`SinkFactory.create -> ConsoleSink` [E-bind] @ src/factory.ext:11",
		"runtime_selection_status=`conditional`",
		"keep `static_call` and `implementation_body_call` as separate call edges",
		"`relation_kind=type_relation`",
		"The model decides whether the binding and current request prove a selected runtime target",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("typed dispatch composition missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"BaseSink.write -> ConsoleSink.write",
		"Facade.run -> ConsoleSink.write",
		"runtime_selection_status=`proven`",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("candidate composition minted an unsupported dispatch edge/status %q:\n%s", forbidden, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_TargetDiscoveryRendersTypedCooperativeMethodRoster(t *testing.T) {
	ctx := &types.AgentContext{EvidenceItems: []types.EvidenceItem{
		{ID: "E-register", Kind: types.EvidenceRegistration, Subject: `@register("json")`, Predicate: "registers", Object: "JsonPlugin", Source: "pipeline/plugins.py", LineStart: 17, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition},
		{ID: "E-base-3", Kind: types.EvidenceRelationship, Subject: "JsonPlugin", Predicate: "inheritance", Object: "BasePlugin", Source: "pipeline/plugins.py", LineStart: 18, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerRepoMapStructuralRelation, RelationOrdinal: 3},
		{ID: "E-base-1", Kind: types.EvidenceRelationship, Subject: "JsonPlugin", Predicate: "inheritance", Object: "TimestampMixin", Source: "pipeline/plugins.py", LineStart: 18, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerRepoMapStructuralRelation, RelationOrdinal: 1},
		{ID: "E-base-2", Kind: types.EvidenceRelationship, Subject: "JsonPlugin", Predicate: "inheritance", Object: "ValidationMixin", Source: "pipeline/plugins.py", LineStart: 18, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerRepoMapStructuralRelation, RelationOrdinal: 2},
		{ID: "E-def-base", Kind: types.EvidenceMechanism, Subject: "BasePlugin.handle", Source: "pipeline/base.py", LineStart: 15, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition},
		{ID: "E-def-validation", Kind: types.EvidenceMechanism, Subject: "ValidationMixin.handle", Source: "pipeline/base.py", LineStart: 30, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition},
		{ID: "E-def-timestamp", Kind: types.EvidenceMechanism, Subject: "TimestampMixin.handle", Source: "pipeline/base.py", LineStart: 39, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition},
		{ID: "E-super-validation", Kind: types.EvidenceRelationship, Subject: "ValidationMixin.handle", OwnerSymbol: "ValidationMixin.handle", Predicate: "cooperative_super_call", Object: "super.handle", Source: "pipeline/base.py", LineStart: 33, Scope: types.ScopeLine, AnchorKind: types.AnchorCall, Producer: types.EvidenceProducerRepoMapCooperativeCall},
		{ID: "E-super-timestamp", Kind: types.EvidenceRelationship, Subject: "TimestampMixin.handle", OwnerSymbol: "TimestampMixin.handle", Predicate: "cooperative_super_call", Object: "super.handle", Source: "pipeline/base.py", LineStart: 42, Scope: types.ScopeLine, AnchorKind: types.AnchorCall, Producer: types.EvidenceProducerRepoMapCooperativeCall},
	}}

	got := renderAnswerDocRuntimeTargetRelationCapsule(ctx)
	for _, want := range []string{
		"family=`cooperative_delegation` relation=`cooperative_super_call` subject=`ValidationMixin.handle` object=`super.handle` [E-super-validation] @ pipeline/base.py:33",
		"### Typed cooperative-method rosters (candidate-only)",
		"declared_type=`JsonPlugin`; operation=`handle`; runtime_mro_status=`unproven`",
		"binding_candidate=`@register(\"json\") -> JsonPlugin` [E-register] @ pipeline/plugins.py:17",
		"1. declared_owner=`TimestampMixin`",
		"2. declared_owner=`ValidationMixin`",
		"3. declared_owner=`BasePlugin`",
		"cooperative_delegation=`TimestampMixin.handle -> super.handle` [E-super-timestamp] @ pipeline/base.py:42",
		"typed_super_delegations=`2/2`; cooperative_path_status=`candidate_only`",
		"Do not draw `BaseA.operation -> BaseB.operation` as a direct call unless a separate typed call edge names those exact owners",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed cooperative roster missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"TimestampMixin.handle -> ValidationMixin.handle",
		"ValidationMixin.handle -> BasePlugin.handle",
		"runtime_mro_status=`proven`",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("cooperative roster minted unsupported concrete MRO edge/status %q:\n%s", forbidden, got)
		}
	}
}

func TestAnswerDocumentEvaluator_SeparatesDecoratorSelectorFromMethodReturnValue(t *testing.T) {
	ctx := &types.AgentContext{EvidenceItems: []types.EvidenceItem{
		{ID: "E-decorator", Kind: types.EvidenceRelationship, Subject: `@register("json")`, Predicate: "decorator_selector_application", Object: "JsonPlugin", Source: "pipeline/plugins.py", LineStart: 17, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerRepoMapDecoratorApplication},
		{ID: "E-content-type", Kind: types.EvidenceConcrete, Subject: "JsonPlugin.content_type", Predicate: "returns", Object: `"application/json"`, Source: "pipeline/plugins.py", LineStart: 23, Scope: types.ScopeLine, AnchorKind: types.AnchorReturn, Producer: "concrete_values"},
	}}

	got := renderAnswerDocRuntimeTargetRelationCapsule(ctx)
	for _, want := range []string{
		"family=`decorator_application` relation=`decorator_selector_application` subject=`@register(\"json\")` object=`JsonPlugin` [E-decorator] @ pipeline/plugins.py:17",
		"family=`value_or_factory_flow` relation=`returns` subject=`JsonPlugin.content_type` object=`\"application/json\"` [E-content-type] @ pipeline/plugins.py:23",
		"### Typed selector/value roles (do not substitute)",
		"decorator_application=`@register(\"json\")` [E-decorator] @ pipeline/plugins.py:17",
		"independent_method_return=`JsonPlugin.content_type -> \"application/json\"` [E-content-type] @ pipeline/plugins.py:23",
		"Whether the selector is a registry key still depends on separate grounded evidence from the decorator implementation",
		"The model retains ownership of the final binding/dispatch conclusion",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed selector/value role separation missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"registration_key=`json`",
		"registration_key=`application/json`",
		"runtime_selection_status=`proven`",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("role-separation context invented a binding conclusion %q:\n%s", forbidden, got)
		}
	}
}

func TestAnswerDocumentEvaluator_RendersTypedSameOwnerLexicalOrderWithoutInventingGuardScope(t *testing.T) {
	ctx := &types.AgentContext{EvidenceItems: []types.EvidenceItem{
		{ID: "E-flush", Kind: types.EvidenceRelationship, Subject: "log", OwnerSymbol: "log", Predicate: "calls", Object: "Sink.flush", Source: "src/logger.cpp", LineStart: 38, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
		{ID: "E-guard", Kind: types.EvidenceConcrete, Subject: "Logger.log", Predicate: "conditional", Object: "level >= Level::kError", Source: "src/logger.cpp", LineStart: 37, Scope: types.ScopeLine, AnchorKind: types.AnchorCondition},
		{ID: "E-write", Kind: types.EvidenceRelationship, Subject: "log", OwnerSymbol: "log", Predicate: "calls", Object: "Sink.write", Source: "src/logger.cpp", LineStart: 36, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
		{ID: "E-unrelated", Kind: types.EvidenceRelationship, Subject: "ConsoleSink.write", OwnerSymbol: "ConsoleSink.write", Predicate: "calls", Object: "IO.write", Source: "src/logger.cpp", LineStart: 11, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
	}}

	got := renderAnswerDocLocalFactOrderCapsule(ctx)
	for _, want := range []string{
		"## Typed same-owner lexical order (advisory)",
		"owner=`Logger.log` source=`src/logger.cpp`",
		"1. line=`36` claim_form=`call_edge` fact=`log -> Sink.write` [E-write]",
		"2. line=`37` claim_form=`guard_condition` fact=`level >= Level::kError` [E-guard]",
		"3. line=`38` claim_form=`call_edge` fact=`log -> Sink.flush` [E-flush]",
		"do not by themselves prove branch containment or causality",
		"only when a separate typed control-scope/containment relation proves that ownership",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed lexical-order capsule missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ConsoleSink.write") || strings.Contains(got, "guard_controls") {
		t.Fatalf("lexical-order capsule must not add unrelated rows or mint guard containment:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_LocalFactOrderUsesCompiledSupportScope(t *testing.T) {
	ctx := &types.AgentContext{EvidenceItems: []types.EvidenceItem{
		{ID: "E-stage-write", Kind: types.EvidenceRelationship, Subject: "StageBinding", OwnerSymbol: "StageBinding", Predicate: "assigns", Object: "StageAnalyze", Source: "internal/types/stage_binding.go", LineStart: 10, Scope: types.ScopeLine, AnchorKind: types.AnchorAssignment},
		{ID: "E-stage-guard", Kind: types.EvidenceConcrete, Subject: "StageBinding", OwnerSymbol: "StageBinding", Predicate: "conditional", Object: "stage != StageUnknown", Source: "internal/types/stage_binding.go", LineStart: 11, Scope: types.ScopeLine, AnchorKind: types.AnchorCondition},
		{ID: "E-stage-return", Kind: types.EvidenceConcrete, Subject: "StageBinding", OwnerSymbol: "StageBinding", Predicate: "returns", Object: "stage", Source: "internal/types/stage_binding.go", LineStart: 12, Scope: types.ScopeLine, AnchorKind: types.AnchorReturn},
		{ID: "E-cache-get", Kind: types.EvidenceRelationship, Subject: "explorerSearchCache", OwnerSymbol: "explorerSearchCache", Predicate: "calls", Object: "cache.Get", Source: "internal/agent/explorer.go", LineStart: 100, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
		{ID: "E-cache-guard", Kind: types.EvidenceConcrete, Subject: "explorerSearchCache", OwnerSymbol: "explorerSearchCache", Predicate: "conditional", Object: "cache != nil", Source: "internal/agent/explorer.go", LineStart: 101, Scope: types.ScopeLine, AnchorKind: types.AnchorCondition},
	}}
	supportScope := supportLaneScopeFromPlan(&types.AnswerSupportPlan{
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLanePrincipalEvidence,
			Entries: []types.AnswerSupportEntry{
				{
					EvidenceID:   "support-stage-binding",
					Source:       "internal/types/stage_binding.go",
					LineStart:    10,
					AnchorSymbol: "StageBinding",
					Subject:      "StageBinding",
				},
				{
					EvidenceID:   "support-explorer-component",
					Source:       "internal/agent/explorer.go",
					LineStart:    69,
					AnchorSymbol: "explorerEvaluator",
					Subject:      "explorerEvaluator",
				},
			},
		}},
	}, true, extractorValueRankComparison)

	got := renderAnswerDocLocalFactOrderCapsuleWithScope(ctx, supportScope)
	if !strings.Contains(got, "owner=`StageBinding` source=`internal/types/stage_binding.go`") {
		t.Fatalf("support-connected lexical group was lost:\n%s", got)
	}
	if strings.Contains(got, "explorerSearchCache") || strings.Contains(got, "internal/agent/explorer.go") {
		t.Fatalf("unrelated lexical group escaped the compiled support scope:\n%s", got)
	}
}

func TestSupportLaneScopeFromDiagramPlan_UsesVisiblePrincipalFloor(t *testing.T) {
	plan := &types.AnswerSupportPlan{Lanes: []types.AnswerSupportLane{
		{
			Kind:          types.SupportLanePrincipalEvidence,
			AllowedBlocks: []string{string(types.BlockDiagram)},
			Entries: []types.AnswerSupportEntry{{
				EvidenceID:   "support-stage-output",
				Text:         "StageOutput",
				Source:       "internal/agent/agent.go",
				LineStart:    36,
				AnchorSymbol: "StageOutput",
				Subject:      "StageOutput",
			}, {
				EvidenceID:   "support-bus-context",
				Text:         "BusContext",
				Source:       "internal/types/context.go",
				LineStart:    45,
				AnchorSymbol: "BusContext",
				Subject:      "BusContext",
			}},
		},
		{
			Kind:          types.SupportLaneNearestMechanism,
			AllowedBlocks: []string{string(types.BlockSection)},
			Entries: []types.AnswerSupportEntry{{
				EvidenceID:   "support-write-policy",
				Text:         "validateWriteAnalyzerToolPolicy",
				Source:       "internal/agent/agent.go",
				LineStart:    3248,
				AnchorSymbol: "validateWriteAnalyzerToolPolicy",
				Subject:      "validateWriteAnalyzerToolPolicy",
			}},
		},
	}}
	supportScope := supportLaneScopeFromDiagramPlan(plan, answerDocDiagramSupportSeedLimit, extractorValueRankComparison)
	if supportScope == nil || !supportScope.hasAnchor("StageOutput") {
		t.Fatal("diagram principal support entry did not reach advisory scope")
	}
	if supportScope.hasAnchor("validateWriteAnalyzerToolPolicy") {
		t.Fatal("section-only sibling support widened the diagram advisory scope")
	}

	got := renderAnswerDocDiagramFlowSeed([]types.FlowFindingDigest{
		{ID: "unrelated", Path: []string{"BaseAgent.executeTool", "validateWriteAnalyzerToolPolicy"}, Sources: []string{"internal/agent/agent.go"}},
		{ID: "relevant", Path: []string{"StageOutput", "BusContext"}},
		{ID: "left-only", Path: []string{"StageOutput", "TraceAdmission"}},
		{ID: "right-only", Path: []string{"WritePolicy", "BusContext"}},
		{ID: "same-file-only", Path: []string{"internal/agent/agent.go:SiblingReader", "internal/agent/agent.go:SiblingWriter"}},
		{ID: "exact-id", Path: []string{"EvidenceBound", "ExactReplay"}, EvidenceIDs: []string{"support-stage-output"}},
	}, supportScope)
	if !strings.Contains(got, "StageOutput -> BusContext") ||
		!strings.Contains(got, "EvidenceBound -> ExactReplay") ||
		strings.Contains(got, "validateWriteAnalyzerToolPolicy") ||
		strings.Contains(got, "TraceAdmission") ||
		strings.Contains(got, "WritePolicy") ||
		strings.Contains(got, "SiblingReader") {
		t.Fatalf("diagram flow seed did not follow the visible principal support floor:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_LocalFactOrderDoesNotGuessAmbiguousShortOwner(t *testing.T) {
	ctx := &types.AgentContext{EvidenceItems: []types.EvidenceItem{
		{ID: "E-short", Kind: types.EvidenceRelationship, Subject: "log", OwnerSymbol: "log", Predicate: "calls", Object: "Sink.write", Source: "src/logger.cpp", LineStart: 36, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
		{ID: "E-a-guard", Kind: types.EvidenceConcrete, Subject: "A.log", Predicate: "conditional", Object: "aReady", Source: "src/logger.cpp", LineStart: 37, Scope: types.ScopeLine, AnchorKind: types.AnchorCondition},
		{ID: "E-b-guard", Kind: types.EvidenceConcrete, Subject: "B.log", Predicate: "conditional", Object: "bReady", Source: "src/logger.cpp", LineStart: 38, Scope: types.ScopeLine, AnchorKind: types.AnchorCondition},
	}}

	if got := renderAnswerDocLocalFactOrderCapsule(ctx); got != "" {
		t.Fatalf("ambiguous leaf owner must not be merged into either qualified owner:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_LocalFactOrderStandsDownWithoutGuard(t *testing.T) {
	ctx := &types.AgentContext{EvidenceItems: []types.EvidenceItem{
		{ID: "E-a", Kind: types.EvidenceRelationship, Subject: "A.run", Predicate: "calls", Object: "B.run", Source: "a.go", LineStart: 10, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
		{ID: "E-b", Kind: types.EvidenceRelationship, Subject: "A.run", Predicate: "calls", Object: "C.run", Source: "a.go", LineStart: 11, Scope: types.ScopeLine, AnchorKind: types.AnchorCall},
	}}
	if got := renderAnswerDocLocalFactOrderCapsule(ctx); got != "" {
		t.Fatalf("same-owner calls without a guard should not add prompt noise:\n%s", got)
	}
}

func TestRenderAnswerDocRuntimeDispatchCompositions_NoTypedOwnerJoinStandsDown(t *testing.T) {
	calls := []answerDocRuntimeTargetRelationRow{{family: "static_call", item: types.EvidenceItem{
		ID: "E-dynamic", Kind: types.EvidenceRelationship, Subject: "Facade.run", Predicate: "calls", Object: "receiver.write",
		Source: "src/facade.ext", LineStart: 20, Scope: types.ScopeLine, AnchorKind: types.AnchorCall,
	}}}
	structural := []answerDocRuntimeTargetRelationRow{{family: "declared_type_relation", item: types.EvidenceItem{
		ID: "E-type", Kind: types.EvidenceRelationship, Subject: "ConsoleSink", Predicate: "implements", Object: "BaseSink",
		Source: "src/console.ext", LineStart: 4, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition,
	}}}
	if got := renderAnswerDocRuntimeDispatchCompositions(nil, structural, calls); got != "" {
		t.Fatalf("unresolved source receiver must not be guessed into a typed dispatch composition:\n%s", got)
	}
}

func TestRenderAnswerDocCallChainEndpointBoundary_DefinitionOnlyDoesNotClaimLeaf(t *testing.T) {
	view := &types.AnswerSemanticView{CallChainEndpointBoundary: &types.CallChainEndpointBoundary{
		Disposition:    types.CallChainEndpointNoDirectedPath,
		SourceEndpoint: "buildAnalysisIR",
		RequestedSink:  "gate.Run",
		EvidenceCapsule: &types.CallChainEndpointEvidenceCapsule{
			Status:             types.CallChainEndpointEvidenceEndpointUnresolved,
			EdgeCount:          1,
			SourceProof:        types.CallChainEndpointExistenceCallEdge,
			RequestedSinkProof: types.CallChainEndpointExistenceDefinitionOnly,
			SourceFrontier:     []types.CallChainEvidenceEdge{{From: "buildAnalysisIR", To: "gate.RunWith", EvidenceID: "E1", Source: "analyzer.go", LineStart: 10}},
		},
	}}
	got := renderAnswerDocCallChainEndpointBoundary(view)
	for _, want := range []string{
		"requested_sink_existence_proof=`definition_only`",
		"requested_sink_incident_call_evidence=`not_emitted`",
		"does not prove the endpoint is a leaf",
		"Keep that local topology unproven",
		"not as the last hop of the principal directed ordered_list",
		"No principal endpoint-boundary call edge is available",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("definition-only topology boundary missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "`gate.Run` ->") || strings.Contains(got, "-> `gate.Run`") {
		t.Fatalf("definition-only proof must not fabricate an incident edge:\n%s", got)
	}
	if strings.Contains(got, "source_frontier:") {
		t.Fatalf("unresolved endpoint must not publish arbitrary source siblings as principal boundary rows:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_CallChainBoundaryUsesExplorerHandoffDefinition(t *testing.T) {
	mut := types.NewMutableState("typed no-directed-path handoff boundary")
	mut.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
		Reason:    types.PrincipalSpanWaiverNoDirectedPath,
		Rationale: "audit-only rationale",
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
			CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "buildAnalysisIR", Sink: "gate.Run"},
			AnalyzerHints: types.AnalyzerHints{
				Kind:         string(types.ReqCallChain),
				ExactTargets: []string{"buildAnalysisIR", "gate.Run"},
			},
		}},
		EvidenceItems: []types.EvidenceItem{
			{ID: "E1", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2666, GroundingStatus: types.GroundingGrounded},
			{ID: "D1", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, Subject: "gate.Run", AnchorSymbol: "Run", Source: "internal/analysis/gate/gate.go", LineStart: 134, GroundingStatus: types.GroundingGrounded},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"call_graph_status=`endpoint_unresolved`",
		"requested_sink_existence_proof=`definition_only`",
		"requested_sink_incident_call_evidence=`not_emitted`",
		"describes only resolution inside the grounded call-edge graph",
		"Do not extend it to the requested sink",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("finalizer prompt lost explorer handoff authority %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "requested_sink_existence_proof=`unproven`") {
		t.Fatalf("finalizer prompt must not regress accepted endpoint existence to unproven:\n%s", prompt)
	}
	if strings.Contains(prompt, "source_frontier: `buildAnalysisIR` -> `gate.RunWith`") {
		t.Fatalf("definition-only endpoint must not promote the source frontier into the principal prompt:\n%s", prompt)
	}
	seed := buildRetryCallChainEndpointBoundarySeed(ctx, types.DiagramSequence)
	for _, want := range []string{`participant p0 as "buildAnalysisIR"`, `participant p1 as "gate.Run"`} {
		if !strings.Contains(seed.Fence, want) {
			t.Fatalf("endpoint-only sequence seed missing %q:\n%s", want, seed.Fence)
		}
	}
	if strings.Contains(seed.Fence, "->>") || strings.Contains(seed.Fence, "gate.RunWith") {
		t.Fatalf("endpoint-only sequence seed invented or promoted an unrelated edge:\n%s", seed.Fence)
	}
}

func TestRenderAnswerDocFirstPassDiagramSkeleton_ReusesValidatorAlignedTypedCarrier(t *testing.T) {
	mut := types.NewMutableState("qualified display endpoint with short call-site identity")
	mut.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
		Reason:    types.PrincipalSpanWaiverNoDirectedPath,
		Rationale: "typed shared-callee boundary",
	})
	evidence := []types.EvidenceItem{
		{ID: "E-source", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", OwnerSymbol: "agent.buildAnalysisIR", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2722, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{ID: "E-sink", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "Run", OwnerSymbol: "gate.Run", Object: "RunWith", AnchorSymbol: "RunWith", Source: "internal/analysis/gate/gate.go", LineStart: 135, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
	}
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: evidence})
	ctx := &types.AgentContext{
		Mutable:       mut,
		EvidenceItems: evidence,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
				CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "buildAnalysisIR", Sink: "gate.Run"},
				AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"buildAnalysisIR", "gate.Run"}},
			},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				Required: true, RequiredKind: types.DiagramSequence,
				PreferredKinds: []types.DiagramKind{types.DiagramSequence},
			}},
		},
	}

	got := renderAnswerDocFirstPassDiagramSkeleton(ctx)
	for _, want := range []string{
		"validator-aligned `edge_anchors_json`",
		"participant n1 as agent.buildAnalysisIR",
		`"from_identity":"buildAnalysisIR"`,
		`"to_identity":"gate.RunWith"`,
		`"from_identity":"Run"`,
		`"to_identity":"RunWith"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("first-pass reference lost validator-aligned carrier %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `participant p0 as "agent.buildAnalysisIR"`) {
		t.Fatalf("first-pass reference must not publish the legacy body-only carrier beside the typed carrier:\n%s", got)
	}
	repairCarrier := answerDocMechanismCopyReadyRepairPayload(ctx)
	if repairCarrier == "" || !strings.Contains(got, repairCarrier) {
		t.Fatalf("initial and repair teaching must reuse one typed carrier:\ninitial=%s\nrepair=%s", got, repairCarrier)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"source_endpoint_existence_proof=`call_edge`",
		"requested_sink_existence_proof=`call_edge`",
		"requested_sink_path: `gate.Run` -> `gate.RunWith`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("finalizer prompt lost owner-qualified endpoint existence %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "requested_sink_incident_call_evidence=`not_emitted`") ||
		strings.Contains(prompt, "requested_sink_existence_proof=`definition_only`") {
		t.Fatalf("one prompt must not deny the requested-sink call edge published by its own capsule:\n%s", prompt)
	}
}

func TestRenderAnswerDocCallChainEndpointBoundary_DirectedEvidenceDisclosesStateConflict(t *testing.T) {
	view := &types.AnswerSemanticView{CallChainEndpointBoundary: &types.CallChainEndpointBoundary{
		Disposition:    types.CallChainEndpointNoDirectedPath,
		SourceEndpoint: "Source.run",
		RequestedSink:  "Sink.run",
		EvidenceCapsule: &types.CallChainEndpointEvidenceCapsule{
			Status:     types.CallChainEndpointEvidenceDirectedPathPresent,
			EdgeCount:  1,
			SourcePath: []types.CallChainEvidenceEdge{{From: "Source.run", To: "Sink.run", EvidenceID: "E1", Source: "main.go", LineStart: 9}},
		},
	}}
	got := renderAnswerDocCallChainEndpointBoundary(view)
	for _, want := range []string{"call_graph_status=`directed_path_present`", "conflicts with the retained no-directed-path boundary", "model owns the conclusion"} {
		if !strings.Contains(got, want) {
			t.Fatalf("conflicting typed carriers must be disclosed with model ownership (%q):\n%s", want, got)
		}
	}
	if strings.Contains(got, "accepted investigation did not prove") {
		t.Fatalf("renderer must not state a no-path fact beside a grounded directed path:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersErrorGranularityContract(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
				ErrorGranularityProfile: &types.ErrorGranularityProfile{
					IsGranularityQuestion: true,
					RequestedVerdictOptions: []types.ErrorGranularityVerdict{
						types.ErrorGranularityPerItemRejection,
						types.ErrorGranularityWholeBatch,
					},
					Confidence: 0.9,
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Typed Error Granularity Contract") {
		t.Fatalf("prompt missing typed error-granularity contract:\n%s", prompt)
	}
	for _, want := range []string{"error_granularity_verdict", "per_item_rejection", "whole_batch_failure", "not_enough_evidence", "requested verdict options", "prose-only"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q in typed error-granularity contract:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `"partial_success", "fail_fast"`) {
		t.Fatalf("prompt should narrow allowed verdicts to requested options plus fallback:\n%s", prompt)
	}
}

// TestRenderAnswerDocFacetCoverage_NilOrEmptyReturnsEmpty pins
// byte-identical behaviour for shapes whose AnswerSurfacePlan
// produces no FacetCoverageContract — Phase 2 must not change pre-P1
// prompt output for those cases.
func TestRenderAnswerDocFacetCoverage_NilOrEmptyReturnsEmpty(t *testing.T) {
	if got := renderAnswerDocFacetCoverage(nil); got != "" {
		t.Errorf("nil ctx should return empty; got %q", got)
	}
	// ctx with no AnalysisIR — answerSurfacePlan returns nil.
	emptyCtx := &types.AgentContext{}
	if got := renderAnswerDocFacetCoverage(emptyCtx); got != "" {
		t.Errorf("ctx without AnalysisIR should return empty; got %q", got)
	}
}

// TestRenderAnswerDocFacetCoverage_EmitsHardSoftOptionalLabels pins
// the rendered surface format. The ConfigPrecedence family fires
// HARD on FacetConfigPrecedenceRole + FacetResolvedLiteralOrSymbol,
// SOFT on FacetUncertaintyBoundary, OPTIONAL on FacetDiagramSpine.
// Phase 1 fallback degrades HARD-without-candidates to SOFT, so we
// also cover that path by NOT supplying surface evidence.
func TestRenderAnswerDocFacetCoverage_EmitsHardSoftOptionalLabels(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentConfigQuery,
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	for _, want := range []string{
		"## Required Answer Facets",
		"**SOFT**", // Phase 1 degrades HARD without evidence
		"Config precedence role",
		"Uncertainty boundary",
		"Optional richness facets:",
		// v3 B5 (2026-05-04): "Diagram spine" / "structural backbone"
		// language replaced with neutral "Diagram facet" — lint
		// catches the old wording in InternalTermsBlocklist.
		"Diagram facet",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered prompt missing %q\n----\n%s", want, got)
		}
	}
}

// TestRenderAnswerDocFacetCoverage_EvidenceCountAnnotation pins
// Phase 5-E2: every facet line ends with `(evidence: N)` where N is
// len(SourceCandidate). The annotation surfaces the typed evidence input so
// the LLM understands when SOFT facets have no evidence to surface (N=0).
func TestRenderAnswerDocFacetCoverage_EvidenceCountAnnotation(t *testing.T) {
	// IntentRootCause + LogBundle drives QFRootCauseTrace, which has
	// FacetObservedArtifactFact as HARD. Provide one matching surface
	// item so SourceCandidate is non-empty (count = 1).
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: &types.LogBundle{},
			},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	// Every facet line MUST carry the (evidence: N) marker.
	if !strings.Contains(got, "(evidence:") {
		t.Errorf("missing (evidence: N) annotation; got\n%s", got)
	}
	// Header prose MUST explain the gate semantic.
	if !strings.Contains(got, "(evidence: N)") ||
		!strings.Contains(got, "SOFT facets, N=0") {
		t.Errorf("header prose missing E1 gate explanation; got\n%s", got)
	}
}

func TestRenderAnswerDocTypedExplorationEnrichment_RendersStructuredRowsAndFlow(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentExplain},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "cfg-default",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/config/runtime.go",
			LineStart:       42,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "DefaultLimit",
			Subject:         "DefaultLimit",
			Object:          "16",
			Snippet:         "DefaultLimit: 16,",
			Summary:         "raw thinking note that must not be surfaced",
			SurfaceTerms:    []string{"DefaultLimit"},
			GroundingStatus: types.GroundingGrounded,
		}},
		FlowFindings: []types.FlowFindingDigest{{
			ID:      "flow-1",
			Path:    []string{"loadConfig", "normalize", "compile"},
			Sources: []string{"codrax.yaml"},
			Sinks:   []string{"RuntimeConfig"},
		}},
	}
	got := renderAnswerDocTypedExplorationEnrichment(ctx, false)
	for _, want := range []string{
		"## Typed Exploration Enrichment Facts",
		"model-emitted `EvidenceItems`, deterministic evidence projections, and `FlowFindings`",
		"lane=value_fact",
		"DefaultLimit: 16,",
		"### Flow/source-sink rows",
		"path=loadConfig -> normalize -> compile",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed enrichment prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "raw thinking note") {
		t.Fatalf("typed enrichment must not replay non-load-bearing free-form summary:\n%s", got)
	}
}

func TestRenderAnswerDocTypedExplorationEnrichmentCarriesTextAuthority(t *testing.T) {
	got := answerDocEvidenceRoleTag(types.EvidenceItem{
		ID: "policy-text", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
		Source: "internal/skill/defaults.go", LineStart: 1203,
		AnchorKind: types.AnchorTextReference, AnchorSymbol: "B/E pairing teaching",
		GroundingStatus: types.GroundingGrounded,
	})
	for _, want := range []string{
		"claim_form=text_reference_fact",
		"source_shape_authority=visible_text_only",
		"executable_mechanism=unproven",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed enrichment lost %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocTypedExplorationEnrichment_TraceKeepsExpandedFlowRows(t *testing.T) {
	flows := make([]types.FlowFindingDigest, 0, 14)
	for i := 1; i <= 14; i++ {
		flows = append(flows, types.FlowFindingDigest{
			ID:   fmt.Sprintf("flow-%02d", i),
			Path: []string{fmt.Sprintf("thread-%02d", i), "ui-main"},
		})
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
		},
		FlowFindings: flows,
	}
	got := renderAnswerDocTypedExplorationEnrichment(ctx, false)
	if count := strings.Count(got, "\n- id=flow-"); count != answerDocMaxFlowEnrichmentFacts {
		t.Fatalf("trace flow supplement should keep %d typed rows, got %d:\n%s", answerDocMaxFlowEnrichmentFacts, count, got)
	}
	if !strings.Contains(got, "id=flow-12") {
		t.Fatalf("expanded trace flow supplement should preserve the 12th row:\n%s", got)
	}
	if strings.Contains(got, "id=flow-13") {
		t.Fatalf("trace flow supplement must remain bounded, got:\n%s", got)
	}
}

func TestRenderAnswerDocTypedExplorationEnrichment_ContextDoesNotBecomePrincipal(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "nearby-helper",
			Kind:            types.EvidenceRelationship,
			Scope:           types.ScopeLine,
			Source:          "internal/agent/helper.go",
			LineStart:       77,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "helper",
			Subject:         "NearbyHelper",
			Object:          "Target",
			Snippet:         "NearbyHelper()",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	got := renderAnswerDocTypedExplorationEnrichment(ctx, true)
	for _, want := range []string{
		"Because typed support lanes are present, those lanes remain the principal boundary",
		"They are not a member slate by themselves",
		"Do not promote an enrichment fact into a new principal ordered-list member",
		"lane=context_enrichment_fact",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed enrichment boundary prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "### Principal Member Obligations") {
		t.Fatalf("enrichment rows must not create principal member obligations:\n%s", got)
	}
}

func TestAnswerDocTypedEnrichmentEvidencePoolBoundsLargeContext(t *testing.T) {
	evidence := make([]types.EvidenceItem, answerDocMaxEnrichmentCandidateFacts+50)
	for i := range evidence {
		evidence[i] = types.EvidenceItem{
			ID:         fmt.Sprintf("ev-%04d", i),
			Kind:       types.EvidenceDirect,
			Source:     "internal/example.go",
			LineStart:  i + 1,
			AnchorKind: types.AnchorAssignment,
			Subject:    fmt.Sprintf("Subject%d", i),
			Snippet:    fmt.Sprintf("Subject%d := value", i),
		}
	}
	ctx := &types.AgentContext{EvidenceItems: evidence}

	got := answerDocTypedEnrichmentEvidencePool(ctx, answerDocMaxEnrichmentCandidateFacts)
	if len(got) != answerDocMaxEnrichmentCandidateFacts {
		t.Fatalf("bounded enrichment pool len=%d, want %d", len(got), answerDocMaxEnrichmentCandidateFacts)
	}
	if got[len(got)-1].ID != fmt.Sprintf("ev-%04d", answerDocMaxEnrichmentCandidateFacts-1) {
		t.Fatalf("bounded enrichment pool should preserve ranked prefix, last=%q", got[len(got)-1].ID)
	}
}

func TestAnswerDocTypedEnrichmentEvidencePoolKeepsLaterTypedCarrierCorrection(t *testing.T) {
	const durableID = "ev-finalizer-line-30"
	prior := types.EvidenceItem{
		ID: durableID, Kind: types.EvidenceDirect, Scope: types.ScopeLine,
		Source: "internal/agent/finalizer.go", LineStart: 30, LineEnd: 30,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "NewBaseAgent",
		GroundingStatus: types.GroundingGrounded,
		Producer:        types.EvidenceProducerExplorerEmitEvidence,
	}
	corrected := types.EvidenceItem{
		ID: durableID, Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Source: "internal/agent/finalizer.go", LineStart: 30, LineEnd: 30,
		AnchorKind: types.AnchorCall, AnchorSymbol: "NewBaseAgent",
		Subject: "NewFinalizerAgent", Predicate: "calls", Object: "NewBaseAgent",
		GroundingStatus: types.GroundingGrounded,
		Producer:        types.EvidenceProducerExplorerEmitEvidence,
	}
	mu := types.NewMutableState("")
	mu.AppendEvidence([]types.EvidenceItem{corrected})
	ctx := &types.AgentContext{
		AnalysisIR:    &types.AnalysisIR{RequestModel: types.RequestModel{PredicateAxis: types.AxisFlow}},
		EvidenceItems: []types.EvidenceItem{prior},
		Mutable:       mu,
	}

	got := answerDocTypedEnrichmentEvidencePool(ctx, answerDocMaxEnrichmentCandidateFacts)
	if len(got) != 1 {
		t.Fatalf("same-ID correction should remain one answer-grade row, got %d: %+v", len(got), got)
	}
	if got[0].Kind != types.EvidenceRelationship ||
		types.ClaimFormOf(got[0]) != types.ClaimCallEdge ||
		got[0].Subject != "NewFinalizerAgent" || got[0].Object != "NewBaseAgent" {
		t.Fatalf("later grounded call carrier was lost behind stale first-ID row: %+v", got[0])
	}
	_, edges, acceptedFacts, callsiteFacts := answerDocCurrentSourceMechanismRelations(ctx)
	if acceptedFacts != 1 || callsiteFacts != 1 || len(edges) != 1 {
		t.Fatalf("corrected call must reach finalizer relation authority, facts=%d calls=%d edges=%+v",
			acceptedFacts, callsiteFacts, edges)
	}
	if edges[0].relation != types.DiagramRelCall ||
		edges[0].from != "NewFinalizerAgent" || edges[0].to != "NewBaseAgent" {
		t.Fatalf("corrected call authority direction changed: %+v", edges[0])
	}
}

func TestSelectAnswerDocTypedEnrichmentFactsTruncatesLargeSurface(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}}
	longSnippet := strings.Repeat("x", answerDocMaxEnrichmentSurfaceBytes+200)
	got := selectAnswerDocTypedEnrichmentFacts(ctx, []types.EvidenceItem{{
		ID:              "large-surface",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/example.go",
		LineStart:       12,
		AnchorKind:      types.AnchorAssignment,
		Subject:         "LargeSurface",
		Snippet:         longSnippet,
		GroundingStatus: types.GroundingGrounded,
	}}, false, nil)
	if len(got) != 1 {
		t.Fatalf("expected one enrichment fact, got %+v", got)
	}
	if len(got[0].surface) > answerDocMaxEnrichmentSurfaceBytes+len("…[truncated]") {
		t.Fatalf("surface was not bounded: len=%d", len(got[0].surface))
	}
	if !strings.Contains(got[0].surface, "…[truncated]") {
		t.Fatalf("surface should carry truncation marker, got %q", got[0].surface)
	}
}

func TestSelectAnswerDocTypedEnrichmentFacts_RejectsUngroundedValueAuthority(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}}
	evidence := []types.EvidenceItem{
		{
			ID:              "rejected-declaration",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/types/context.go",
			LineStart:       18,
			AnchorKind:      types.AnchorInitializer,
			AnchorSymbol:    "Mutable",
			Subject:         "Mutable",
			Snippet:         "Mutable *MutableState",
			Authority:       types.AuthorityIllustrative,
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			ID:              "recovered-assignment",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/orchestrator/runtime.go",
			LineStart:       24,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "Mutable",
			Subject:         "ctx.Mutable",
			Object:          "next",
			Snippet:         "ctx.Mutable = next",
			GroundingStatus: types.GroundingRecovered,
		},
	}

	got := selectAnswerDocTypedEnrichmentFacts(ctx, evidence, false, nil)
	if len(got) != 1 || got[0].item.ID != "recovered-assignment" {
		t.Fatalf("explicitly ungrounded evidence must not enter factual enrichment; got %+v", got)
	}
	if got[0].lane != "value_fact" {
		t.Fatalf("recovered assignment lane=%q, want value_fact", got[0].lane)
	}
	rendered := renderAnswerDocTypedExplorationEnrichment(&types.AgentContext{
		AnalysisIR:    ctx.AnalysisIR,
		EvidenceItems: evidence,
	}, false)
	if strings.Contains(rendered, "rejected-declaration") || strings.Contains(rendered, "Mutable initializer anchor") {
		t.Fatalf("ungrounded initializer leaked into finalizer handoff:\n%s", rendered)
	}
}

func TestSelectAnswerDocTypedEnrichmentFacts_SalienceLockedSurvivesScoreFilter(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: "needle",
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentExplain, RawRequest: "needle"},
		},
	}
	evidence := []types.EvidenceItem{
		{
			ID:              "scored",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/scored.go",
			LineStart:       10,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "Scored",
			Subject:         "Scored",
			Object:          "needle",
			Snippet:         "Scored = \"needle\"",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "locked",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/locked.go",
			LineStart:       11,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "LockedDefinition",
			Subject:         "LockedDefinition",
			Snippet:         "type LockedDefinition struct{}",
			Salience:        types.SalienceLoadBearing,
			GroundingStatus: types.GroundingGrounded,
		},
	}
	got := selectAnswerDocTypedEnrichmentFacts(ctx, evidence, false, nil)
	if len(got) != 2 {
		t.Fatalf("expected scored + locked facts, got %+v", got)
	}
	if got[0].item.ID != "locked" {
		t.Fatalf("locked salience row should sort before context rows, got first=%q", got[0].item.ID)
	}
}

func TestSelectAnswerDocTypedEnrichmentFacts_PrincipalDefinitionRowsPreserveDetails(t *testing.T) {
	mut := types.NewMutableState("list functions")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/types/grammar.go"},
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "functions",
			Value:   "1",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"RegisteredKinds"},
		}},
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				SourceScopeProfile: &types.SourceScopeProfile{
					RequestedScope: types.SourceScopeProduction,
				},
			},
		},
	}
	evidence := []types.EvidenceItem{
		{
			ID:              "context",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/types/grammar.go",
			LineStart:       10,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "helper",
			Subject:         "helper",
			Summary:         "context call",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:                 "registered-kinds",
			Kind:               types.EvidenceDirect,
			Scope:              types.ScopeLine,
			Source:             "internal/types/grammar.go",
			LineStart:          106,
			AnchorKind:         types.AnchorDefinition,
			AnchorSymbol:       "RegisteredKinds",
			Subject:            "RegisteredKinds",
			Summary:            "returns all registered kinds with stable ordering",
			LoadBearingSummary: true,
			GroundingStatus:    types.GroundingGrounded,
		},
	}

	got := selectAnswerDocTypedEnrichmentFacts(ctx, evidence, false, nil)
	if len(got) == 0 {
		t.Fatal("expected enrichment rows")
	}
	if got[0].item.ID != "registered-kinds" {
		t.Fatalf("principal definition detail should sort first, got %+v", got)
	}
	if got[0].lane != "principal_definition_fact" {
		t.Fatalf("lane=%q, want principal_definition_fact", got[0].lane)
	}
	if !strings.Contains(got[0].surface, "stable ordering") {
		t.Fatalf("definition summary detail was not preserved: %q", got[0].surface)
	}
}

func TestAnswerDocEnrichmentDisplayLimit_WidensForLockedSalience(t *testing.T) {
	evidence := make([]types.EvidenceItem, 0, answerDocMaxEnrichmentFacts)
	for i := 0; i < answerDocMaxEnrichmentFacts; i++ {
		evidence = append(evidence, types.EvidenceItem{Salience: types.SalienceExhaustListed})
	}
	limit := answerDocEnrichmentDisplayLimit(nil, extractorValueRankGeneric, evidence)
	if limit != answerDocMaxEnrichmentFacts {
		t.Fatalf("limit=%d, want max cap %d", limit, answerDocMaxEnrichmentFacts)
	}
	for i := range evidence {
		evidence[i].Salience = types.SalienceUnset
	}
	limit = answerDocEnrichmentDisplayLimit(nil, extractorValueRankGeneric, evidence)
	if limit != answerDocDefaultEnrichmentFacts {
		t.Fatalf("unset salience must keep default limit, got %d", limit)
	}
}

func TestSelectAnswerDocTypedEnrichmentFacts_DiagnosticSupportScopeFiltersUnrelatedRows(t *testing.T) {
	supportScope := supportLaneScopeFromPlan(&types.AnswerSupportPlan{
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLaneCurrentCodePath,
			Entries: []types.AnswerSupportEntry{{
				EvidenceID:    "support-build",
				Source:        "internal/agent/analyzer.go",
				LineStart:     1271,
				AnchorKind:    types.AnchorCall,
				AnchorSymbol:  "buildAnalysisIR",
				Subject:       "buildAnalysisIR",
				SurfaceTerms:  []string{"analysis IR builder"},
				GroundingTier: types.TierLineText,
			}},
		}},
	}, true, extractorValueRankDiagnostic)
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentRootCause,
				Scenario: types.ScenarioRootCause,
			},
		},
	}
	evidence := []types.EvidenceItem{
		{
			ID:              "relevant-build",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1280,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "buildAnalysisIR",
			Object:          "request model",
			Snippet:         "raw := ctx.Mutable.RequestModel()",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "unrelated-multigraph",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/tool/repomap/multigraph/build.go",
			LineStart:       42,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "New",
			Subject:         "MultiGraph.New",
			Object:          "graph construction",
			Snippet:         "return MultiGraph.New(nodes)",
			GroundingStatus: types.GroundingGrounded,
		},
	}

	got := selectAnswerDocTypedEnrichmentFacts(ctx, evidence, true, supportScope)
	if len(got) != 1 {
		t.Fatalf("diagnostic support scope should keep only linked rows, got %d: %+v", len(got), got)
	}
	if got[0].item.ID != "relevant-build" {
		t.Fatalf("diagnostic support scope kept wrong row: %+v", got[0].item)
	}
}

func TestSelectAnswerDocTypedEnrichmentFacts_SupportScopeFiltersUnrelatedRowsAcrossProfiles(t *testing.T) {
	supportScope := supportLaneScopeFromPlan(&types.AnswerSupportPlan{
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLaneCurrentCodePath,
			Entries: []types.AnswerSupportEntry{{
				EvidenceID:   "support-build",
				Source:       "internal/agent/analyzer.go",
				LineStart:    1271,
				AnchorSymbol: "buildAnalysisIR",
				Subject:      "buildAnalysisIR",
			}},
		}},
	}, true, extractorValueRankComparison)
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
			},
		},
	}
	evidence := []types.EvidenceItem{{
		ID:              "architecture-context",
		Kind:            types.EvidenceRelationship,
		Scope:           types.ScopeLine,
		Source:          "internal/tool/repomap/multigraph/build.go",
		LineStart:       42,
		AnchorKind:      types.AnchorCall,
		AnchorSymbol:    "New",
		Subject:         "MultiGraph.New",
		Object:          "graph construction",
		Snippet:         "return MultiGraph.New(nodes)",
		GroundingStatus: types.GroundingGrounded,
	}}

	got := selectAnswerDocTypedEnrichmentFacts(ctx, evidence, true, supportScope)
	if len(got) != 0 {
		t.Fatalf("support-rendered enrichment should stay inside support lanes across profiles, got %+v", got)
	}
}

func TestSelectAnswerDocTypedEnrichmentFacts_SourceLocationBeatsSameAnchor(t *testing.T) {
	supportScope := supportLaneScopeFromPlan(&types.AnswerSupportPlan{
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLanePrincipalEvidence,
			Entries: []types.AnswerSupportEntry{{
				EvidenceID:   "support-cart-extension",
				Source:       "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
				LineStart:    30,
				AnchorSymbol: "Cart",
				Subject:      "Cart",
			}},
		}},
	}, true, extractorValueRankEnumeration)
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
	}
	evidence := []types.EvidenceItem{
		{
			ID:              "same-anchor-wrong-location",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/thirdparty/tree-sitter-cangjie/corpus/sources/03_struct_interface.cj",
			LineStart:       30,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Cart",
			Subject:         "Cart",
			Snippet:         `println("Circle at ...")`,
			SurfaceTerms:    []string{"extend Cart"},
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "exact-cart-extension",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
			LineStart:       30,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Cart",
			Subject:         "Cart",
			Snippet:         "extend Cart {",
			SurfaceTerms:    []string{"extend Cart"},
			GroundingStatus: types.GroundingGrounded,
		},
	}

	got := selectAnswerDocTypedEnrichmentFacts(ctx, evidence, true, supportScope)
	if len(got) != 1 {
		t.Fatalf("support scope should keep only exact source-location evidence, got %+v", got)
	}
	if got[0].item.ID != "exact-cart-extension" {
		t.Fatalf("same-anchor wrong-location evidence leaked into enrichment: %+v", got)
	}
}

func TestSelectAnswerDocFlowEnrichmentLines_DiagnosticSupportScopeFiltersUnrelatedFlow(t *testing.T) {
	supportScope := supportLaneScopeFromPlan(&types.AnswerSupportPlan{
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLaneCurrentCodePath,
			Entries: []types.AnswerSupportEntry{{
				EvidenceID:   "support-build",
				Source:       "internal/agent/analyzer.go",
				LineStart:    1271,
				AnchorSymbol: "buildAnalysisIR",
				Subject:      "buildAnalysisIR",
			}},
		}},
	}, true, extractorValueRankDiagnostic)
	findings := []types.FlowFindingDigest{
		{
			ID:          "flow-relevant",
			Path:        []string{"parseAnalyzerPayload", "buildAnalysisIR"},
			EvidenceIDs: []string{"support-build"},
		},
		{
			ID:   "flow-unrelated",
			Path: []string{"MultiGraph.New", "Builder"},
		},
		{
			ID:      "flow-same-file-only",
			Path:    []string{"BaseAgent.executeTool", "MutableState.ArmTraceInputAdmissionTerminal"},
			Sources: []string{"internal/agent/analyzer.go"},
		},
	}

	got := selectAnswerDocFlowEnrichmentLines(findings, 10, supportScope)
	if len(got) != 1 {
		t.Fatalf("diagnostic support scope should keep only linked flow rows, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0], "flow-relevant") {
		t.Fatalf("diagnostic support scope kept wrong flow row: %+v", got)
	}
}

func TestSupportLaneScope_FlowFileFallbackRequiresAnchorlessSupport(t *testing.T) {
	supportScope := supportLaneScopeFromPlan(&types.AnswerSupportPlan{
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLaneCurrentCodePath,
			Entries: []types.AnswerSupportEntry{{
				EvidenceID: "support-file-only",
				Source:     "internal/agent/analyzer.go",
				LineStart:  1271,
			}},
		}},
	}, true, extractorValueRankDiagnostic)
	finding := types.FlowFindingDigest{
		ID:      "flow-file-linked",
		Path:    []string{"parseAnalyzerPayload", "buildAnalysisIR"},
		Sources: []string{"internal/agent/analyzer.go"},
	}

	if !supportScope.allowsFlowFinding(finding) {
		t.Fatal("file-only support without typed endpoint anchors should retain its compatibility flow")
	}
}

func TestRenderAnswerDocTypedExplorationEnrichment_CoversQuestionFamilies(t *testing.T) {
	cases := []struct {
		name string
		rm   types.RequestModel
		item types.EvidenceItem
		want string
	}{
		{
			name: "scalar",
			rm: types.RequestModel{
				Intent:        types.IntentReturnValue,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectNumeric},
			},
			item: types.EvidenceItem{Kind: types.EvidenceDirect, AnchorKind: types.AnchorReturn, AnchorSymbol: "limit", Subject: "limit", Snippet: "return 16"},
			want: "lane=value_fact",
		},
		{
			name: "config",
			rm: types.RequestModel{
				Intent:        types.IntentConfigQuery,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			item: types.EvidenceItem{Kind: types.EvidenceDirect, AnchorKind: types.AnchorInitializer, AnchorSymbol: "llm_stream", Subject: "llm_stream", Snippet: "llm_stream: true"},
			want: "lane=value_fact",
		},
		{
			name: "trace",
			rm:   types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall},
			item: types.EvidenceItem{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "Analyze", Object: "Compile", Snippet: "Compile(ir)"},
			want: "lane=chain_or_intermediate_fact",
		},
		{
			name: "diagnostic",
			rm:   types.RequestModel{Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause},
			item: types.EvidenceItem{Kind: types.EvidenceConflict, AnchorKind: types.AnchorCondition, Subject: "guard", Condition: "ctx == nil", Snippet: "if ctx == nil { return }"},
			want: "lane=boundary_or_exclusion_fact",
		},
		{
			name: "comparison",
			rm:   types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain, SubTopics: []types.SubTopic{{Summary: "A"}, {Summary: "B"}}},
			item: types.EvidenceItem{Kind: types.EvidenceDirect, AnchorKind: types.AnchorImport, Subject: "agent", Object: "internal/types", Snippet: "import \"github.com/hanchaoqun/codrax/internal/types\""},
			want: "lane=chain_or_intermediate_fact",
		},
		{
			name: "enumeration",
			rm:   types.RequestModel{Intent: types.IntentEnumerate, Predicates: types.SemanticPredicates{IsCategoryEnumeration: true}},
			item: types.EvidenceItem{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, Subject: "ExplorerAgent", Object: "agent implementation", SurfaceTerms: []string{"ExplorerAgent"}},
			want: "lane=principal_definition_fact",
		},
		{
			name: "generic",
			rm:   types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioGeneric},
			item: types.EvidenceItem{Kind: types.EvidenceAbsent, Scope: types.ScopeNegative, Source: "internal/agent/agent.go", NegativeQuery: &types.NegativeQuery{File: "internal/agent/agent.go", Pattern: "typed handoff"}, NegativeScope: types.NegativeScopeFile, Subject: "typed handoff"},
			want: "lane=boundary_or_exclusion_fact",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := tc.item
			item.ID = "ev-" + tc.name
			if item.Scope == "" {
				item.Scope = types.ScopeLine
			}
			if item.Source == "" {
				item.Source = "internal/agent/example.go"
			}
			if item.LineStart == 0 && item.Scope == types.ScopeLine {
				item.LineStart = 10
			}
			if item.GroundingStatus == "" {
				item.GroundingStatus = types.GroundingGrounded
			}
			ctx := &types.AgentContext{
				AnalysisIR:    &types.AnalysisIR{RequestModel: tc.rm, AnswerContract: types.AnswerContract{}},
				EvidenceItems: []types.EvidenceItem{item},
			}
			got := renderAnswerDocTypedExplorationEnrichment(ctx, false)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("%s enrichment missing %q:\n%s", tc.name, tc.want, got)
			}
		})
	}
}

func TestRenderAnswerDocFacetCoverage_UsesCuratedPrincipalEvidenceCount(t *testing.T) {
	modelItem := types.EvidenceItem{
		ID:              "literal-model",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1903,
		AnchorKind:      types.AnchorAssignment,
		Subject:         "CitationReq.Required",
		Object:          "false",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
	}
	evidence := []types.EvidenceItem{modelItem}
	for i := 0; i < 20; i++ {
		evidence = append(evidence, types.EvidenceItem{
			ID:              fmt.Sprintf("noise-%02d", i),
			Kind:            types.EvidenceConcrete,
			Scope:           types.ScopeLine,
			Source:          "internal/tool/noise.go",
			LineStart:       i + 1,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    fmt.Sprintf("helper%d", i),
			Producer:        "concrete_values",
			GroundingStatus: types.GroundingGrounded,
		})
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
			},
		},
		EvidenceItems: evidence,
	}

	got := renderAnswerDocFacetCoverage(ctx)
	if !strings.Contains(got, `facet_id: "resolved_literal_or_symbol"`) ||
		!strings.Contains(got, "Resolved literal or symbol") ||
		!strings.Contains(got, "(evidence: 1)") {
		t.Fatalf("facet prompt should render curated principal evidence count, got:\n%s", got)
	}
	if strings.Contains(got, "(evidence: 21)") {
		t.Fatalf("facet prompt should not expose broad raw candidate count as answer-grade evidence:\n%s", got)
	}
}

// SOFT facets with typed evidence available carry an
// `(evidence available)` marker after the evidence count, signalling to the
// LLM that the answer may naturally cover the facet without making the
// metadata an emit-time hard gate.
func TestRenderAnswerDocFacetCoverage_EvidenceAvailableMarkerOnPromotedSoft(t *testing.T) {
	// IntentTrace → QFCallChain template has FacetCurrentCodePath
	// as SOFT requirement, AcceptableForms=[ClaimDefinitionFact].
	// FacetBranchGuard is SOFT with AcceptableForms=[ClaimGuardCondition]
	// — leave it WITHOUT matching evidence so it stays unpromoted.
	// AnchorDefinition projects to ClaimDefinitionFact, which matches
	// FacetCurrentCodePath's AcceptableForms.
	defEvidence := types.EvidenceItem{
		Kind:       types.EvidenceDirect,
		Subject:    "DefSym",
		Source:     "x.go",
		LineStart:  10,
		LineEnd:    10,
		AnchorKind: types.AnchorDefinition,
	}
	callEvidence := types.EvidenceItem{
		Kind:       types.EvidenceDirect,
		Subject:    "Caller",
		Object:     "Callee",
		Source:     "x.go",
		LineStart:  20,
		LineEnd:    20,
		AnchorKind: types.AnchorCall,
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
			},
		},
		EvidenceItems: []types.EvidenceItem{defEvidence, callEvidence},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	// Header prose MUST explain the evidence-available marker.
	if !strings.Contains(got, "evidence available") {
		t.Errorf("header prose missing evidence-available explanation; got\n%s", got)
	}
	// At least one promoted SOFT line should carry `(evidence available)` — the
	// header occurrence is wrapped in backticks; the bare marker on
	// a facet line is the typed signal.
	lines := strings.Split(got, "\n")
	bareMarkerCount := 0
	for _, line := range lines {
		// Skip header lines (they describe the marker via backticks)
		if !strings.HasPrefix(strings.TrimSpace(line), "-") {
			continue
		}
		if strings.Contains(line, " (evidence available)") {
			bareMarkerCount++
		}
	}
	if bareMarkerCount < 1 {
		t.Errorf("expected ≥1 facet line with bare `(evidence available)`; got %d\n%s", bareMarkerCount, got)
	}
}

// TestRenderAnswerDocFacetCoverage_MustDeclareContractSurfacesHardFacets
// pins the rendered prompt's must-declare set to emit-hard facets only.
// Evidence-supported SOFT facets may be useful, but they must not be
// described as emit-time rejection requirements.
func TestRenderAnswerDocFacetCoverage_MustDeclareContractSurfacesHardFacets(t *testing.T) {
	// IntentTrace + AnchorDefinition evidence → FacetCurrentCodePath is
	// evidence-available SOFT, while the principal path edge facet is
	// template-hard.
	defEvidence := types.EvidenceItem{
		Kind:       types.EvidenceDirect,
		Subject:    "DefSym",
		Source:     "x.go",
		LineStart:  10,
		LineEnd:    10,
		AnchorKind: types.AnchorDefinition,
	}
	callEvidence := types.EvidenceItem{
		Kind:       types.EvidenceDirect,
		Subject:    "Caller",
		Object:     "Callee",
		Source:     "x.go",
		LineStart:  20,
		LineEnd:    20,
		AnchorKind: types.AnchorCall,
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
			},
		},
		EvidenceItems: []types.EvidenceItem{defEvidence, callEvidence},
	}
	got := renderAnswerDocFacetCoverage(ctx)

	// (1) Header MUST list at least one must-declare facet_id.
	if !strings.Contains(got, "Must declare (emit-time rejection if any are missing") {
		t.Errorf("header missing 'Must declare' summary line; got\n%s", got)
	}

	// (2) At least one hard facet line MUST carry the
	// "→ **MUST declare**" contract suffix.
	lines := strings.Split(got, "\n")
	mustDeclareLineCount := 0
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "-") {
			continue
		}
		if strings.Contains(line, "→ **MUST declare** via `block.facet_ids[]` or `claim_uses[].facet_id`") {
			mustDeclareLineCount++
		}
	}
	if mustDeclareLineCount < 1 {
		t.Errorf("expected ≥1 facet line with MUST-declare contract suffix; got %d\n%s",
			mustDeclareLineCount, got)
	}

	// (3) MUST-declare suffix MUST NOT leak onto SOFT/OPTIONAL
	// facet lines (those would mislead the LLM into declaring
	// non-enforced facets and bloating the doc).
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "- **OPTIONAL**") &&
			!strings.HasPrefix(trim, "- **SOFT**") {
			continue
		}
		if strings.Contains(line, "MUST declare") {
			t.Errorf("SOFT/OPTIONAL facet line carries MUST declare suffix (should be hard-only): %q", line)
		}
	}
}

// TestRenderAnswerDocFacetCoverage_NoMustDeclareWhenNoPromoted pins
// that the must-declare header is OMITTED when no facet is promoted.
// Avoids leaking an empty "Must declare: ." line.
func TestRenderAnswerDocFacetCoverage_NoMustDeclareWhenNoPromoted(t *testing.T) {
	// IntentTrace WITHOUT any matching evidence → all SOFT facets stay
	// unpromoted. The FacetCallChainSpine HARD facet's gate behavior
	// depends on family-specific resolution; this test focuses on the
	// SOFT-without-evidence case.
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
			},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	// When there are no emit-hard facets, the must-declare header MUST
	// be absent (we cannot list zero entries cleanly).
	if got == "" {
		return // section omitted entirely is fine
	}
	if strings.Contains(got, "Must declare (emit-time rejection if any are missing):") &&
		!strings.Contains(got, "facet_id") {
		t.Errorf("must-declare header present but no facet_id listed; got\n%s", got)
	}
}

// SOFT facet line WITHOUT typed evidence (SourceCandidate=0)
// MUST NOT carry the evidence-available bare marker.
func TestRenderAnswerDocFacetCoverage_NoEvidenceAvailableWhenNoEvidence(t *testing.T) {
	// IntentTrace → QFCallChain. Provide NO evidence at all so every
	// SOFT facet has SourceCandidate=0; expect no bare evidence-available marker.
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
			},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "-") {
			continue
		}
		if strings.Contains(line, " (evidence available)") {
			t.Errorf("no-evidence SOFT facet must NOT carry bare evidence-available marker; got line %q", line)
		}
	}
}

// TestRenderAnswerDocPrincipalMemberSetContract_RendersMustVerbatimList
// pins P1A (2026-05-17): when the investigator hands off a principal
// member_set fact with decorated members, the new prompt section MUST
// surface those members verbatim under a "MUST appear verbatim" contract
// statement so the LLM does not paraphrase them away into iter=0 failure.
func TestRenderAnswerDocPrincipalMemberSetContract_RendersMustVerbatimList(t *testing.T) {
	mut := types.NewMutableState("compare codrax and opencode")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "codrax 四层",
		Value: "4",
		Members: []string{
			"SelfConsistencyReviewer",
			"SemanticQualityReviewer",
			"gate.Run (9 checks)",
			"contract_check (violRegistry)",
		},
		SupportRefs: []string{
			"SelfConsistencyReviewer @ internal/a.go:10",
			"SemanticQualityReviewer @ internal/b.go:20",
			"gate.Run (9 checks) @ internal/gate.go:30",
			"contract_check (violRegistry) @ internal/contract.go:40",
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsCrossComponent:      true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind: string(types.ReqEnumeration),
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "compare codrax and opencode",
				},
			},
		},
		Mutable: mut,
	}

	got := renderAnswerDocPrincipalMemberSetContract(ctx)
	if got == "" {
		t.Fatal("expected non-empty contract section for principal member_set with non-path members")
	}

	// (1) Section header MUST be present.
	if !strings.Contains(got, "## Required Principal Member Set") {
		t.Errorf("section header missing; got\n%s", got)
	}

	// (2) MUST-appear-verbatim contract MUST appear.
	if !strings.Contains(got, "MUST appear verbatim") {
		t.Errorf("contract clause missing; got\n%s", got)
	}

	// (3) The contract must point at the single rich principal row list
	// instead of duplicating the member body here.
	if !strings.Contains(got, "Principal Enumeration Rows") ||
		!strings.Contains(got, "Every `row.member` there MUST appear verbatim") {
		t.Errorf("compact principal-row contract missing; got\n%s", got)
	}
	for _, member := range []string{
		"`SelfConsistencyReviewer`",
		"`SemanticQualityReviewer`",
		"`gate.Run (9 checks)`",
		"`contract_check (violRegistry)`",
	} {
		if strings.Contains(got, member) {
			t.Errorf("contract should not duplicate member %s when principal rows exist; got\n%s", member, got)
		}
	}

	// (4) Label header MUST appear so the LLM knows which set the
	// members belong to.
	if !strings.Contains(got, `principal set "codrax 四层"`) {
		t.Errorf("missing principal set label; got\n%s", got)
	}

	// (5) The no-dup contract still makes the oracle behavior explicit.
	if !strings.Contains(got, "do not paraphrase or abbreviate") {
		t.Errorf("missing paraphrase guard; got\n%s", got)
	}
}

func TestRenderAnswerDocPrincipalMemberSetContractPreservesCaseDistinctSymbols(t *testing.T) {
	got := answerDocDistinctPrincipalMembers([]string{
		" isTraceMarkPayload ",
		"IsTraceMarkPayload",
		"isTraceMarkPayload",
		"",
	})
	want := []string{"isTraceMarkPayload", "IsTraceMarkPayload"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("case-distinct typed symbols must survive exact dedup: got=%q want=%q", got, want)
	}
}

func TestRenderAnswerDocAggregateFacts_SourceOperationSiteCitationGuidance(t *testing.T) {
	question := "当前代码仓进程切换cgroup分组，其pid和tid是在哪儿写入cgroup分组下面的文件里的，都有哪些写入点？"
	mut := types.NewMutableState(question)
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "cgroup 写入点",
		Value: "2",
		Members: []string{
			"SetPidToCgroup @ awarecpu/aware_cpuctl.c:131",
			"SetCgroup dispatch @ awarecpu/aware_cpuctl.c:822",
		},
		MemberNotes: []string{
			"target path: /dev/cpuctl/cgroup.procs",
			"dispatches to background/root cpuctl write helpers",
		},
		SupportRefs: []string{
			"awarecpu/aware_cpuctl.c:131",
			"awarecpu/aware_cpuctl.c:822",
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: question,
				Intent:     types.IntentRootCause,
				Predicates: types.SemanticPredicates{
					HasPerMemberTable: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleMethod},
				},
			},
		},
		Mutable: mut,
	}

	contract := renderAnswerDocPrincipalMemberSetContract(ctx)
	if !strings.Contains(contract, "source operation-site set") ||
		!strings.Contains(contract, "must not replace the citation for the function/call/write site") {
		t.Fatalf("source operation-site principal contract missing citation guidance:\n%s", contract)
	}
	prompt := renderAnswerDocAggregateFacts(ctx)
	if !strings.Contains(prompt, "Source operation-site contract") ||
		!strings.Contains(prompt, "Do not borrow a nearby constant/path citation") {
		t.Fatalf("aggregate fact prompt missing source operation-site citation guidance:\n%s", prompt)
	}
}

// TestRenderAnswerDocPrincipalMemberSetContract_EmptyWhenNoPrincipalFacts
// pins that the section is omitted entirely when no principal member_set
// is on the bus, so the prompt budget is not consumed by an empty header.
func TestRenderAnswerDocPrincipalMemberSetContract_EmptyWhenNoPrincipalFacts(t *testing.T) {
	// nil ctx
	if got := renderAnswerDocPrincipalMemberSetContract(nil); got != "" {
		t.Errorf("nil ctx must produce empty section; got %q", got)
	}
	// ctx without AnalysisIR
	ctxNoIR := &types.AgentContext{}
	if got := renderAnswerDocPrincipalMemberSetContract(ctxNoIR); got != "" {
		t.Errorf("ctx without AnalysisIR must produce empty section; got %q", got)
	}
	// ctx with mutable but no aggregate facts
	mut := types.NewMutableState("test")
	ctxNoFacts := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
		},
		Mutable: mut,
	}
	if got := renderAnswerDocPrincipalMemberSetContract(ctxNoFacts); got != "" {
		t.Errorf("no facts must produce empty section; got %q", got)
	}
}

// TestRenderAnswerDocPrincipalMemberSetContract_SkipsScalarCountSupport
// pins that the contract section is OMITTED for scalar-count questions
// where the member_set is just supporting evidence for the count
// (e.g. "how many X are there?"). In that case the count IS the answer
// and verbatim members are not required — matches the oracle filter
// AggregateMemberSetIsScalarCountSupport.
func TestRenderAnswerDocPrincipalMemberSetContract_SkipsScalarCountSupport(t *testing.T) {
	mut := types.NewMutableState("how many checks are there")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "checks",
		Value:   "9",
		Members: []string{"checkA", "checkB", "checkC"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				Predicates: types.SemanticPredicates{
					IsCountQuestion: true,
					IsScalarAnswer:  true,
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectNumeric},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "how many",
				},
			},
		},
		Mutable: mut,
	}
	got := renderAnswerDocPrincipalMemberSetContract(ctx)
	if got != "" {
		t.Errorf("scalar count support must skip section; got\n%s", got)
	}
}

func TestRenderAnswerDocPrincipalMemberSetContract_SkipsNoHitSearchedWindowSupport(t *testing.T) {
	mut := types.NewMutableState("最近 10 次提交有没有改 ResSchedClient")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateNegativeObservation,
			Label: "recent commits do not touch ResSchedClient",
			Value: "0",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: "vcs_metadata"},
				{Name: "target", Value: "ResSchedClient"},
				{Name: "commit_range", Value: "HEAD~10..HEAD"},
				{Name: "result_count", Value: "0"},
				{Name: "tool_result", Value: "git-history-search-001"},
			},
		},
		{
			Kind:  types.AnswerAggregateMemberSet,
			Label: "searched commits",
			Value: "3",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: "vcs_metadata"},
				{Name: "commit_range", Value: "HEAD~10..HEAD"},
				{Name: "window_count", Value: "10"},
				{Name: "unmatched", Value: "10"},
				{Name: "tool_result", Value: "git-history-search-001"},
			},
			Members: []string{"c1 fix docs", "c2 tune search", "c3 update tests"},
		},
	})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
		Mutable: mut,
	}

	if got := renderAnswerDocPrincipalMemberSetContract(ctx); got != "" {
		t.Fatalf("no-hit searched-window ledger must not become a required principal member contract:\n%s", got)
	}
}

func TestRenderAnswerDocPrincipalMemberSetContract_HistoryList(t *testing.T) {
	mut := types.NewMutableState("recent commit impact rollup")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "最近10次提交",
		Value: "2",
		Members: []string{
			"abc1234 (docs: plan contract)",
			"def5678 (agent: preserve VCS summaries)",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "vcs_metadata"}},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentExplain,
			Predicates: types.SemanticPredicates{IsHistoryLookup: true},
		}},
		Mutable: mut,
	}

	contract := renderAnswerDocPrincipalMemberSetContract(ctx)
	if contract == "" {
		t.Fatal("history list member_set should produce a visible preservation contract")
	}
	if !strings.Contains(contract, `principal set "最近10次提交"`) ||
		!strings.Contains(contract, "Principal Enumeration Rows") {
		t.Fatalf("history principal contract should point at the single rich row surface:\n%s", contract)
	}
	rows := renderAnswerDocAggregateFacts(ctx)
	for _, want := range []string{
		"member=`abc1234 (docs: plan contract)`",
		"member=`def5678 (agent: preserve VCS summaries)`",
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("history principal rows missing %q:\n%s", want, rows)
		}
	}
}

// TestRenderAnswerDocPrincipalMemberSetContract_BuildInitialInstructionWiring
// pins that the new section is wired into BuildInitialInstruction so the
// finalizer prompt actually sees it.
func TestRenderAnswerDocPrincipalMemberSetContract_BuildInitialInstructionWiring(t *testing.T) {
	mut := types.NewMutableState("compare X and Y")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "X layers",
		Value:   "2",
		Members: []string{"LayerA", "LayerB (boundary)"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsCrossComponent:      true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind: string(types.ReqEnumeration),
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "compare X and Y",
				},
			},
		},
		Mutable: mut,
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Advisory Model-Inferred Member Sets") {
		t.Errorf("BuildInitialInstruction must expose ungrounded member sets as advisory context; got\n%s", prompt)
	}
	if strings.Contains(prompt, "## Required Principal Member Set") ||
		strings.Contains(prompt, "Use this lane as the principal member slate") {
		t.Errorf("pure system_inference must not become a required contract or principal support lane; got\n%s", prompt)
	}
	for _, want := range []string{
		"fact_authority=`advisory_model_inference`",
		"principal_contract=`not_authorized`",
		"advisory set \"X layers\"",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("advisory member-set prompt missing %q; got\n%s", want, prompt)
		}
	}
}

// TestRenderAnswerDocFacetCoverage_NoGoInternalNamesLeak pins the
// glossary-lint contract at the function-output level. None of the
// Phase 1 internal type names may appear in the rendered LLM-facing
// prompt, no matter what facets fire.
func TestRenderAnswerDocFacetCoverage_NoGoInternalNamesLeak(t *testing.T) {
	// Drive every family to maximise facet variety in the rendered
	// output. Use four distinct ctxs and concatenate.
	cases := []*types.AgentContext{
		{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause, LogTriage: &types.LogBundle{}}}},
		{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentConfigQuery}}},
		{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate}}},
		{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentExplain,
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName, Confidence: 0.8}}}},
	}
	var combined strings.Builder
	for _, ctx := range cases {
		combined.WriteString(renderAnswerDocFacetCoverage(ctx))
	}
	got := combined.String()
	for _, banned := range []string{
		"FacetCoverageContract", "FacetCoverage", "FacetRequirement",
		"AnswerFacetKind", "QuestionFamily", "ClaimFormOf",
		"FacetRequiredness", "AcceptableForms", "SourceCandidate",
		"ClaimDefinitionFact", "ClaimCallEdge", "ClaimExternalObservation",
		"QFRootCauseTrace", "QFConfigPrecedence",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("rendered prompt leaked Go-internal token %q\n----\n%s", banned, got)
		}
	}
}

// TestRenderAnswerDocFacetCoverage_OptionalSectionOmittedWhenEmpty
// pins that the "Optional richness facets" sub-heading only renders
// when there's at least one optional facet — keeps prompt lean.
// QFCallChain template (Intent=Trace, no obligation, no log) has
// HARD/SOFT-only entries with zero FacetOptional rows.
func TestRenderAnswerDocFacetCoverage_OptionalSectionOmittedWhenEmpty(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
			},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	if got == "" {
		t.Fatal("QFCallChain should produce a non-empty contract")
	}
	if strings.Contains(got, "Optional richness facets:") {
		t.Errorf("QFCallChain template has no optional facets; section must be omitted; got:\n%s", got)
	}
	if !strings.Contains(got, "Principal path edge") {
		t.Errorf("QFCallChain principal-path-edge facet missing; got:\n%s", got)
	}
}

// TestRenderAnswerDocFacetCoverage_BuildInitialInstructionWiring
// pins that BuildInitialInstruction picks up the section in its
// per-dispatch output (covers the orchestration plumb between
// answerSurfacePlan and the prompt builder).
func TestRenderAnswerDocFacetCoverage_BuildInitialInstructionWiring(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 3, SourceQuote: "3 X",
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Required Answer Facets") {
		t.Errorf("BuildInitialInstruction did not surface facet section; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Enumeration item") {
		t.Errorf("enumeration_item facet expected for IntentEnumerate + EnumerationBoundary; got:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SingleTopicExplanationLeavesSymbolsEmpty(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentExplain},
			AnswerContract: types.AnswerContract{},
		},
		Mutable: types.NewMutableState(""),
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Do NOT add an enumeration anchor skeleton block unless the prompt explicitly attached an Anchor skeleton section") {
		t.Fatalf("single-topic explanation checklist must forbid anchor skeleton noise:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_HistorySubTopicsDoNotEchoKeyAnchors(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: "最近三次合入分别做了什么？",
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqHistory)},
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				SubTopics: []types.SubTopic{
					{Summary: "最近合入"},
					{Summary: "影响范围"},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		Mutable: types.NewMutableState(""),
		AnswerSymbols: []types.AnswerSymbol{{
			Name:      "scalar",
			File:      "internal/agent/explorer.go",
			Line:      3572,
			Rationale: "support-only stale symbol from extraction",
		}},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "Key Anchors block beneath") ||
		strings.Contains(prompt, "关键锚点 block beneath") {
		t.Fatalf("history answers must not promote support symbols into a visible key-anchor block:\n%s", prompt)
	}
}

func TestAnswerDocKeyAnchorsTitleLocalizesChinese(t *testing.T) {
	if got := answerDocKeyAnchorsTitle("zh-CN"); got != "关键锚点" {
		t.Fatalf("Chinese key-anchor title = %q", got)
	}
	if got := answerDocKeyAnchorsTitle("en"); got != "Key Anchors" {
		t.Fatalf("English key-anchor title = %q", got)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_ResolvesAbsentConfigValueToExplanation(t *testing.T) {
	mut := types.NewMutableState("")
	mut.SetInvestigationResultKind("absence")
	mut.SetAbsenceJustification("repo-wide search found no exact key")
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					AllowAbsence: true,
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	// Pre-PR5 the prompt embedded the literal "explanation" shape
	// label; with shape retired, the absence-narrative path is
	// indicated by the V2 BlockSummary contract — assert that the
	// contract section is rendered (not by tag) for an absent
	// exact-config-key dispatch.
	if !strings.Contains(prompt, "## Required Answer Blocks") {
		t.Fatalf("absent exact config-key dispatch should still render block contract: %q", prompt)
	}
}

// TestAnswerDocumentEvaluator_BuildInitialInstruction_SurfacesCardinalityBaseline
// checks that when MustInclude is populated and the answer needs an
// enumeration slate, the dynamic prompt renders the grounded
// required-member floor so the finalizer preserves those members in
// V2 blocks instead of inventing a retired completeness field.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_SurfacesCardinalityBaseline(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"Alpha", "Beta"},
			},
		},
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "Alpha", File: "a.go", Line: 10},
			{Name: "Beta", File: "b.go", Line: 20},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Alpha") || !strings.Contains(prompt, "Beta") {
		t.Errorf("prior slate not surfaced: %q", prompt)
	}
	if !strings.Contains(prompt, "Required-term floor: **2 term(s)**") {
		t.Errorf("required-term floor not surfaced: %q", prompt)
	}
	if !strings.Contains(prompt, "Alpha (symbol)") || !strings.Contains(prompt, "Beta (symbol)") {
		t.Errorf("required-term labels not surfaced: %q", prompt)
	}
	if !strings.Contains(prompt, "must preserve all 2 grounded term(s)") {
		t.Errorf("required-member preservation guidance not surfaced: %q", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_TypedMustIncludeTerms(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"providers.yaml"},
				MustIncludeTerms: []types.ContractTerm{
					{Text: "emit_evidence", Kind: types.ContractTermToolName},
					{Text: "providers.yaml", Kind: types.ContractTermFileStem},
					{Text: "raw prompt", Kind: types.ContractTermUserPhrase},
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"emit_evidence (tool name)",
		"providers.yaml (file stem)",
		"raw prompt (phrase)",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("typed must-include term %q missing from prompt:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Required-symbol floor") {
		t.Fatalf("typed must-include prompt should not call every term a symbol:\n%s", prompt)
	}
	if strings.Contains(prompt, "providers.yaml (symbol)") {
		t.Fatalf("typed file-stem term should override duplicate legacy symbol lane:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_MustIncludeTermsAreNotPrincipalFloorWithoutSlate(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"SubAgentRuntime", "ProposeSubAgents"},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Required answer-term coverage:") {
		t.Fatalf("must_include terms should still be surfaced as answer coverage:\n%s", prompt)
	}
	if strings.Contains(prompt, "must preserve all 2 grounded term(s)") {
		t.Fatalf("must_include terms without answer-symbol slate must not become principal item floor:\n%s", prompt)
	}
	if !strings.Contains(prompt, "do not turn a helper, tool, file stem, attribute, or mechanism component into a principal list member") {
		t.Fatalf("prompt should guard against context helper promotion:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersCallChainSupportLanesFromStepBackbone(t *testing.T) {
	mut := types.NewMutableState("")
	syms := []types.AnswerSymbol{
		{Name: "RequestModel", File: "internal/agent/analyzer.go", Line: 616, Rationale: "在 buildAnalysisIR 内部获取 LLM 输出的 RequestModel，是后续步骤的输入基础"},
		{Name: "gate.Run", File: "internal/agent/analyzer.go", Line: 1062, Rationale: "执行质量门检查，生成最终 gate 结果"},
	}
	mut.SetEmittedAnswerSymbols(syms, types.CompletenessLowerBound)
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentTrace},
			AnswerContract: types.AnswerContract{},
		},
		Mutable:                  mut,
		AnswerSymbols:            syms,
		AnswerSymbolCompleteness: types.CompletenessLowerBound,
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Typed Answer Support Lanes",
		"### Current grounded call chain",
		"### Cross-language call-diagram semantics",
		"exact callee operation",
		"Do not manufacture a self-call",
		"FFI, JNI, PyO3",
		"show an unproved binding transition as a `Note over`",
		"compound condition can still contain a real invocation",
		"never replace the callee with an abstract guard node",
		"JavaScript/TypeScript/ArkTS",
		"Cangjie",
		"Declarative imports, inheritance/implements edges, annotations, and Proto/RPC declarations",
		"presentation instructions, not conclusion authority",
		"Allowed block kinds: summary, ordered_list, diagram",
		"[entry_role=`typed_step_backbone`]",
		"`RequestModel`",
		"internal/agent/analyzer.go:616",
		"`gate.Run`",
		"internal/agent/analyzer.go:1062",
		"runtime frames as additional principal hops",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("call-chain support prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Resolved Step Sequence") {
		t.Fatalf("call-chain support lanes should replace legacy step sequence prompt:\n%s", prompt)
	}
}

func TestRenderAnswerDocCallChainDiagramSemanticsGuide_CoversAllExecutableLanguageFamiliesWithoutHardGate(t *testing.T) {
	guide := renderAnswerDocCallChainDiagramSemanticsGuide()
	for _, language := range []string{
		"Go", "Java", "Kotlin", "JavaScript/TypeScript/ArkTS", "C/C++", "Rust",
		"Python", "Ruby", "Swift", "Lua", "Cangjie",
	} {
		if !strings.Contains(guide, language) {
			t.Fatalf("cross-language diagram guide missing %q:\n%s", language, guide)
		}
	}
	for _, want := range []string{
		"exact callee operation",
		"One grounded invocation proves one direct caller-to-callee edge",
		"Calls with the same caller are sibling call sites",
		"source-ordered stages/call sites",
		"do not rename the fan-out as several independent paths converging on the last callee",
		"A sequence diagram's top-to-bottom message order is itself an ordering claim",
		"prefer a call-DAG/flow view or omit the optional diagram",
		"never label sibling calls as parallel/concurrent merely because they share a caller",
		"`Note`, `alt`/`else`, or `opt`",
		"registration_binding_fact",
		"going through the registered binding, never as bypassing it",
		"does not prove execution of the registered callable",
		"compound condition can still contain a real invocation",
		"dynamic dispatch is unresolved",
		"not executable calls unless a separately grounded invocation proves that edge",
		"not conclusion authority",
		"prefer repository and business vocabulary",
	} {
		if !strings.Contains(guide, want) {
			t.Fatalf("cross-language diagram guide missing semantic boundary %q:\n%s", want, guide)
		}
	}
	for _, forbidden := range []string{"user question contains", "answer text contains", "reject the answer"} {
		if strings.Contains(strings.ToLower(guide), forbidden) {
			t.Fatalf("soft diagram guide must not introduce raw-prose hard gates %q:\n%s", forbidden, guide)
		}
	}
}

func TestRenderAnswerDocCallChainRelationTopology_ClassifiesSiblingCallsWithoutInventingAPath(t *testing.T) {
	plan := &types.AnswerSupportPlan{Family: types.QFCallChain, Lanes: []types.AnswerSupportLane{{
		Kind: types.SupportLaneCurrentCodePath,
		Entries: []types.AnswerSupportEntry{
			{EvidenceID: "e1", ClaimForm: types.ClaimCallEdge, Subject: "buildAnalysisIR", Object: "normalizer.Normalize", Source: "internal/agent/analyzer.go", LineStart: 2321, Location: "internal/agent/analyzer.go:2321"},
			{EvidenceID: "e2", ClaimForm: types.ClaimCallEdge, Subject: "buildAnalysisIR", Object: "compiler.Compile", Source: "internal/agent/analyzer.go", LineStart: 2528, Location: "internal/agent/analyzer.go:2528"},
			{EvidenceID: "e3", ClaimForm: types.ClaimCallEdge, Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2722, Location: "internal/agent/analyzer.go:2722"},
		},
	}}}

	got := renderAnswerDocCallChainRelationTopology(plan)
	for _, want := range []string{
		"grounded_direct_edge_count=`3`",
		"connected_edge_transition_count=`0`",
		"connected_path_status=`no callee_to_next_caller transition",
		"sibling_callsite_group: caller=`buildAnalysisIR`",
		"normalizer.Normalize @ internal/agent/analyzer.go:2321 | compiler.Compile @ internal/agent/analyzer.go:2528 | gate.RunWith @ internal/agent/analyzer.go:2722",
		"same caller fan-out; source order is not callee chaining",
		"model owns the explanation and visible diagram",
	} {
		if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
			t.Fatalf("typed relation topology missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocCallChainRelationTopology_RecognizesExactConnectedTransition(t *testing.T) {
	plan := &types.AnswerSupportPlan{Family: types.QFCallChain, Lanes: []types.AnswerSupportLane{{
		Kind: types.SupportLaneCurrentCodePath,
		Entries: []types.AnswerSupportEntry{
			{EvidenceID: "e1", ClaimForm: types.ClaimCallEdge, Subject: "cli.run", Object: "service.fetch", Location: "cli.ts:10"},
			{EvidenceID: "e2", ClaimForm: types.ClaimCallEdge, Subject: "service.fetch", Object: "transport.send", Location: "service.ts:20"},
		},
	}}}

	got := renderAnswerDocCallChainRelationTopology(plan)
	if !strings.Contains(got, "connected_edge_transition_count=`1`") {
		t.Fatalf("callee-to-next-caller transition should be recognized:\n%s", got)
	}
	if strings.Contains(got, "connected_path_status=`no callee_to_next_caller") || strings.Contains(got, "sibling_callsite_group") {
		t.Fatalf("connected chain must not be labeled disconnected sibling fan-out:\n%s", got)
	}
}

func TestRenderAnswerDocCallChainRelationTopology_DoesNotMergeSameNamedCallersAcrossFiles(t *testing.T) {
	plan := &types.AnswerSupportPlan{Family: types.QFCallChain, Lanes: []types.AnswerSupportLane{{
		Kind: types.SupportLaneCurrentCodePath,
		Entries: []types.AnswerSupportEntry{
			{EvidenceID: "e1", ClaimForm: types.ClaimCallEdge, Subject: "Handler.run", Object: "a.send", Source: "src/a.java", LineStart: 10, Location: "src/a.java:10"},
			{EvidenceID: "e2", ClaimForm: types.ClaimCallEdge, Subject: "Handler.run", Object: "b.send", Source: "src/b.java", LineStart: 20, Location: "src/b.java:20"},
		},
	}}}

	got := renderAnswerDocCallChainRelationTopology(plan)
	if strings.Contains(got, "sibling_callsite_group") {
		t.Fatalf("same-named callers in different source owners must fail open instead of gaining one source order:\n%s", got)
	}
}

func TestRenderAnswerDocCallChainRelationTopology_DoesNotConnectQualifiedAndShortSameTail(t *testing.T) {
	plan := &types.AnswerSupportPlan{Family: types.QFCallChain, Lanes: []types.AnswerSupportLane{{
		Kind: types.SupportLaneCurrentCodePath,
		Entries: []types.AnswerSupportEntry{
			{EvidenceID: "e1", ClaimForm: types.ClaimCallEdge, Subject: "cli.run", Object: "a.Handler.send", Source: "cli.ts", LineStart: 10, Location: "cli.ts:10"},
			{EvidenceID: "e2", ClaimForm: types.ClaimCallEdge, Subject: "send", Object: "transport.write", Source: "other.ts", LineStart: 20, Location: "other.ts:20"},
		},
	}}}

	got := renderAnswerDocCallChainRelationTopology(plan)
	if !strings.Contains(got, "connected_edge_transition_count=`0`") {
		t.Fatalf("short tail must not stand for a qualified owner in topology joins:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersCallChainSupportLanesFromEvidence(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentTrace},
			AnswerContract: types.AnswerContract{},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       127,
				LineEnd:         130,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "checkCoverage",
				Summary:         "checkCoverage is appended as the first gate check",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       128,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "checkDAGClosure",
				Summary:         "checkDAGClosure is appended next",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       129,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "checkBudgetSanity",
				Summary:         "checkBudgetSanity is appended after DAG closure",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Typed Answer Support Lanes",
		"### Current grounded call chain",
		"`checkCoverage`",
		"internal/analysis/gate/gate.go:127",
		"equivalent typed anchors: `internal/analysis/gate/gate.go:130`",
		"`checkDAGClosure`",
		"internal/analysis/gate/gate.go:128",
		"`checkBudgetSanity`",
		"internal/analysis/gate/gate.go:129",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("fallback call-chain support prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Resolved Step Sequence") {
		t.Fatalf("call-chain support lanes should replace legacy fallback step sequence prompt:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_PrincipalSupportLaneBacktickBoundary(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				AnswerSubject: types.AnswerSubject{
					Kind:       types.SubjectNumeric,
					Confidence: 0.95,
				},
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				ID:              "required-false-1",
				Kind:            types.EvidenceDirect,
				Scope:           types.ScopeLine,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1903,
				AnchorKind:      types.AnchorAssignment,
				AnchorSymbol:    "CitationReq.Required",
				Subject:         "AnswerContract.CitationReq.Required",
				Object:          "false",
				Snippet:         "out.AnswerContract.CitationReq.Required = false",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Typed Answer Support Lanes",
		"inline backticks around code / file / config surfaces only when that exact surface is visible in a lane entry",
		"Names that appear only in `Evidence note`, retry diagnostics, raw tool output, search hints, or nearby context are background",
		"If the user's scalar / count question also asks for concrete members, files, paths, or line numbers",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("principal support prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersPrincipalMemberObligations(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
			},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{{
			ID:              "enum-intent",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       642,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Intent",
			Subject:         "Intent",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"### Principal Member Obligations",
		"Intent",
		"internal/types/analysis_ir.go:642",
		"item_id=support-intent",
		"citation_key=internal/types/analysis_ir.go:642",
		"evidence_id=enum-intent",
		"fresh `citations[]` pool",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("principal member obligation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestRenderAnswerDocPrincipalMemberObligations_DedupesEnumerationRowsByLocationAndLabel(t *testing.T) {
	coverage := answerDocPrincipalEnumerationRowCoverage{
		byEvidenceID: map[string]bool{},
		byRowKey:     map[string]bool{},
		rows: []types.EnumerationDisplayRow{{
			RowID:        "source-inventory-public-class-cart",
			Member:       "Cart",
			DisplayLabel: "Cart",
			Source:       "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
			LineStart:    14,
			Location:     "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14",
			CitationKey:  "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14",
		}},
		rowCount: 1,
	}
	for _, key := range answerDocPrincipalEnumerationRowKeys(coverage.rows[0]) {
		coverage.byRowKey[key] = true
	}
	plan := &types.AnswerSupportPlan{
		Family:                  types.QFEnumeration,
		PrincipalMemberCoverage: types.PrincipalMemberCoveragePolicyRequired,
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLanePrincipalEvidence,
			Entries: []types.AnswerSupportEntry{{
				Text:       "Cart",
				Location:   "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14",
				Source:     "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
				LineStart:  14,
				EvidenceID: "different-support-lane-id",
				ClaimForm:  types.ClaimDefinitionFact,
			}},
		}},
	}

	got := renderAnswerDocPrincipalMemberObligations(plan, coverage)
	for _, want := range []string{
		"1 answer-grade member obligation(s) are already rendered once in `Principal Enumeration Rows` above",
		"The typed principal lane contains 1 answer-grade member(s). Render each member from `Principal Enumeration Rows`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("deduped obligation prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "different-support-lane-id") ||
		strings.Contains(got, "still need explicit obligation rows") {
		t.Fatalf("same location+label support row should be covered by principal enumeration rows:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_FileImpactObligationsUseFileCount(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				ChangeImpactProfile: &types.ChangeImpactProfile{
					IsChangeImpact:  true,
					RequestedOutput: types.ImpactOutputFiles,
					Target:          "CitationReq.Required",
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
			},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				ID:              "builder-required",
				Kind:            types.EvidenceConditional,
				Scope:           types.ScopeLine,
				Source:          "internal/context/builder.go",
				LineStart:       1729,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "Required",
				Subject:         "CitationReq.Required",
				Condition:       "if c.CitationReq.Required",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
			{
				ID:              "gate-required",
				Kind:            types.EvidenceConditional,
				Scope:           types.ScopeLine,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       301,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "Required",
				Subject:         "CitationReq.Required",
				Condition:       "if c.CitationReq.Required && c.CitationReq.MinCitations < 0",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"This is a change-impact file enumeration",
		"the 2 member(s) below are the file set",
		"Use each file path as the item label",
		"Do not copy numeric file counts from unstructured closure prose",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("file-impact obligation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersEquivalentPrincipalAnchors(t *testing.T) {
	mut := types.NewMutableState("all implementers")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "LoopController implementers",
		Value:   "1",
		Members: []string{"analyzerEvaluator (internal/agent/analyzer.go:887)"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"analyzerEvaluator"},
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "all implementers"},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
			},
		},
		Mutable: mut,
		EvidenceItems: []types.EvidenceItem{{
			ID:              "analyzer-definition",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/agent/analyzer.go",
			LineStart:       46,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "analyzerEvaluator",
			Subject:         "analyzerEvaluator",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"equivalent typed anchors",
		"internal/agent/analyzer.go:887",
		"internal/agent/analyzer.go:46",
		"one of internal/agent/analyzer.go:887, internal/agent/analyzer.go:46",
		"Anchors are equivalent only for the visible claims they both actually prove",
		"prefer one grounded proof anchor that carries both the member endpoint and that visible second axis",
		"a definition-only line is not equivalent for that two-axis row",
		"Do not churn `citation_ref`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("principal equivalent-anchor prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersStructuredAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateTotalCount,
			Label: "production assignment locations",
			Value: "4",
			Unit:  "locations",
			Members: []string{
				"internal/agent/analyzer.go:1903",
				"internal/orchestrator/contract_check.go:63",
			},
		},
		{
			Kind:  types.AnswerAggregateUniqueCount,
			Label: "unique files",
			Value: "3",
			Unit:  "files",
			Members: []string{
				"internal/agent/analyzer.go",
				"internal/orchestrator/contract_check.go",
				"internal/orchestrator/orchestrator.go",
			},
		},
	})
	mut.SetInvestigationComplete("deterministic count and classification complete")
	mut.SetInvestigationResultKind("resolved")
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				AnswerSubject: types.AnswerSubject{
					Kind:       types.SubjectNumeric,
					Confidence: 0.95,
				},
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		Mutable: mut,
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Structured Aggregate Facts",
		"emit_investigation_complete.aggregate_facts",
		"kind=`total_count`, label=production assignment locations, value=`4`",
		"kind=`unique_count`, label=unique files, value=`3`",
		"internal/orchestrator/orchestrator.go",
		"create or reuse a matching `citations[]` entry",
		"Do not recompute new aggregate values in finalization",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("aggregate prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersPrincipalEnumerationRowsWithNotes(t *testing.T) {
	mut := types.NewMutableState("")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "公开函数",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Unit:        "函数",
		Members:     []string{"Eval"},
		SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15"},
	}})
	mut.SetInvestigationComplete("enumeration complete")
	mut.SetInvestigationResultKind("resolved")
	mut.RetainInvestigationAggregateFacts()
	evidence := []types.EvidenceItem{{
		ID:              "ev-eval",
		Kind:            types.EvidenceDirect,
		Subject:         "Eval",
		AnchorSymbol:    "Eval",
		AnchorKind:      types.AnchorDefinition,
		Source:          "internal/analysis/criterion/eval.go",
		LineStart:       15,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
		Summary:         "Eval 对单个 Criterion 进行求值并返回 Result。" + strings.Repeat(" 保留富描述", 90) + " END_MARKER",
	}}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		Mutable:       mut,
		EvidenceItems: evidence,
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Principal Enumeration Rows",
		"row_id=`enum-set-公开函数-row-eval`",
		"member=`Eval`",
		"location=`internal/analysis/criterion/eval.go:15`",
		"citation_key=`internal/analysis/criterion/eval.go:15`",
		"note: Eval 对单个 Criterion 进行求值并返回 Result。",
		"END_MARKER",
		"Use `display_label`, `location`, and `citation_key` to build clear table cells",
		"For EVERY structured source-inventory item, copy that row's exact `row_id` to `source_inventory_row_id`",
		"even when labels are unique, decorated for display, or repeated across files",
		"use a required bucket `section`'s `items[]` when the row belongs to that bucket",
		"do not add a second global list/table merely to repeat rows already carried by sections",
		"render that note on the same row as a concise description/说明 column",
		"members_rendered_in=authoritative_principal_member_rows",
		"Entries already rendered in `Principal Enumeration Rows`: 1",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("principal enumeration row prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "members=[`Eval`]") {
		t.Fatalf("principal member rows should not be duplicated as dry structured members:\n%s", prompt)
	}
	if strings.Contains(prompt, "Render these rows as the actual principal `ordered_list`, `bullet_list`, or `table` blocks") {
		t.Fatalf("principal row teaching retained the carrier list that excludes section.items[]:\n%s", prompt)
	}
}

func TestRenderAnswerDocSourceInventoryRowGuidance_LocationIsVisibleRowField(t *testing.T) {
	mut := types.NewMutableState("enumerate ArkTS entries")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "@Entry pages", Value: "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Index (struct)"},
		SupportRefs: []string{"Index (struct) @ src/Index.ets:5"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
			RequestedFields: []types.SourceInventoryRequestedField{
				types.SourceInventoryFieldName,
				types.SourceInventoryFieldLocation,
			},
			Confidence: 0.95,
		},
	}}, Mutable: mut}
	got := renderAnswerDocSourceInventoryRowGuidance(ctx)
	for _, want := range []string{
		"`location` is a user-visible row field",
		"same item's text/cells",
		"does not replace the requested visible file path",
		"does not by itself prove inheritance, implementation, execution, ownership",
		"When `summary` was not requested",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("location row guidance missing %q:\n%s", want, got)
		}
	}
	contract := renderAnswerDocPrincipalMemberSetContract(ctx)
	if !strings.Contains(contract, "exact `display_label`") ||
		!strings.Contains(contract, "exact base `Index`") ||
		strings.Contains(contract, "Every `row.member` there MUST appear verbatim") ||
		strings.Contains(contract, "do not paraphrase or abbreviate any row member") {
		t.Fatalf("source-inventory member contract did not publish its row-id/display split:\n%s", contract)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersPrincipalBoundaryForPriorContext(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				AnswerSubject: types.AnswerSubject{
					Kind:       types.SubjectConfigKey,
					Confidence: 0.9,
				},
				ConversationReferenceProfile: &types.ConversationReferenceProfile{
					RequiresPriorContext:  true,
					NeedsRepoVerification: true,
					Ambiguity:             types.ConversationReferenceAmbiguityNone,
					ResolvedSubjects: []types.ResolvedConversationSubject{{
						Surface:          "explore_mid_loop_hint_budget",
						Kind:             types.SubjectConfigKey,
						Source:           types.ConversationReferenceSourcePriorContext,
						Role:             "asked_default_value",
						UseAsExactTarget: true,
					}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Principal Answer Boundary",
		"key-file / related-file / 关键文件 section",
		"do not promote them into the principal file list",
		"Resolved prior-context focus:",
		"not an authorization to add neighboring subjects as answer members",
		"surface `explore_mid_loop_hint_budget`",
		"kind `config_key`",
		"eligible exact target",
		"For role lookups, answer with the single role-bearing literal",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("principal boundary prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_PrincipalBoundaryBindsDiagramsToUserIntent(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramSequence},
				},
			},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "entry/src/main/ets/pages/Index.ets",
				LineStart:       42,
				AnchorKind:      types.AnchorCall,
				Subject:         "Index.ets.render",
				Object:          "NativeBridge.invokeOhSum",
				AnchorSymbol:    "NativeBridge.invokeOhSum",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceRelationship,
				Source:          "src/native/sum.cpp",
				LineStart:       18,
				AnchorKind:      types.AnchorCall,
				Subject:         "NativeBridge.invokeOhSum",
				Object:          "demo.bridge.ohSum",
				AnchorSymbol:    "demo.bridge.ohSum",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceRelationship,
				Source:          "src/bridge/Bridge.cj",
				LineStart:       27,
				AnchorKind:      types.AnchorCall,
				Subject:         "demo.bridge.ohSum",
				Object:          "Bridge.sum",
				AnchorSymbol:    "Bridge.sum",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Principal Answer Boundary",
		"Any diagram must obey the same principal boundary as the prose blocks",
		"For call-chain answers, the principal ordered list and any sequence diagram contain only the requested chain hops",
		"### Current grounded call chain",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("diagram boundary prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SuppressesStepBackboneWhenTypedSupportPlanExists(t *testing.T) {
	mut := types.NewMutableState("")
	syms := []types.AnswerSymbol{
		{Name: "ParseOutput", File: "internal/agent/analyzer.go", Line: 698, Rationale: "stack trace shows all args are nil"},
		{Name: "buildAnalysisIR", File: "internal/agent/analyzer.go", Line: 743, Rationale: "panic must come from deeper nil path"},
	}
	mut.SetEmittedAnswerSymbols(syms, types.CompletenessLowerBound)
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				LogTriage: &types.LogBundle{
					Errors:        []types.LogError{{Type: "panic"}},
					ResolvedFiles: []string{"internal/agent/analyzer.go"},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		LogTriage:                &types.LogBundle{Errors: []types.LogError{{Type: "panic", Frames: []types.LogFrame{{Raw: "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR(0x0)\n\tinternal/agent/analyzer.go:250 +0x1e", File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"}}}}, ResolvedFiles: []string{"internal/agent/analyzer.go"}},
		Mutable:                  mut,
		AnswerSymbols:            syms,
		AnswerSymbolCompleteness: types.CompletenessLowerBound,
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       743,
				AnchorKind:      types.AnchorCall,
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				AnchorSymbol:    "buildAnalysisIR",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/analyzer.go",
				LineStart:       978,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "buildAnalysisIR",
				Condition:       "ctx == nil || ctx.Mutable == nil",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Typed Answer Support Lanes") {
		t.Fatalf("typed support lanes missing:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Allowed block kinds: summary, caveat") {
		t.Fatalf("typed support lanes should expose block-kind boundaries:\n%s", prompt)
	}
	if !strings.Contains(prompt, "If a lane does not list `ordered_list`, do not turn its entries into principal hop items") {
		t.Fatalf("typed support instructions should forbid boundary/observation lanes from becoming principal hops:\n%s", prompt)
	}
	if !strings.Contains(prompt, "If the **Nearest grounded mechanism** lane is absent, do NOT invent a likely internal cause") {
		t.Fatalf("typed support instructions should forbid speculative cause promotion when mechanism lane is absent:\n%s", prompt)
	}
	if strings.Contains(prompt, "## Resolved Step Sequence") {
		t.Fatalf("legacy step backbone should be suppressed when typed support lanes exist:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_CurrentStatusDecisionLane(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				LogTriage: &types.LogBundle{
					Errors:        []types.LogError{{Type: "panic"}},
					ResolvedFiles: []string{"internal/agent/analyzer.go"},
				},
			},
			AnswerContract: types.AnswerContract{
				CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{
					Required: true,
					AllowedVerdicts: []types.CurrentStatusVerdict{
						types.CurrentStatusStillPresent,
						types.CurrentStatusFixed,
						types.CurrentStatusNotEnoughEvidence,
					},
				},
			},
		},
		LogTriage: &types.LogBundle{
			Errors:        []types.LogError{{Type: "panic", Frames: []types.LogFrame{{Raw: "frame", File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"}}}},
			ResolvedFiles: []string{"internal/agent/analyzer.go"},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       743,
				AnchorKind:      types.AnchorCall,
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				AnchorSymbol:    "buildAnalysisIR",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/analyzer.go",
				LineStart:       978,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "buildAnalysisIR",
				Condition:       "ctx == nil || ctx.Mutable == nil",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "typed `decision` verdict only from the lanes below") {
		t.Fatalf("current-status support instructions should name the decision verdict lane:\n%s", prompt)
	}
	if !strings.Contains(prompt, "current_status_verdict") {
		t.Fatalf("current-status prompt should require typed verdict field:\n%s", prompt)
	}
	if !strings.Contains(prompt, "The typed verdict field is the decision carrier") {
		t.Fatalf("current-status prompt should make the typed verdict the decision carrier:\n%s", prompt)
	}
	if strings.Contains(prompt, "current_status_verdict` set to the canonical status enum. Put only the rationale and evidence boundary in block `text`; do not repeat a second canonical status token there. Attach a one-element `items=[{id:\"d\", citation_ref:N}]` when you need a citation anchor, and attach block-level `claim_uses=") {
		t.Fatalf("current-status checklist should not force decision claim_uses:\n%s", prompt)
	}
	if !strings.Contains(prompt, "`not_enough_evidence`: current cited code cannot decide") {
		t.Fatalf("current-status prompt should define not_enough_evidence narrowly:\n%s", prompt)
	}
	if strings.Contains(prompt, "verdict at the START of block `text`") {
		t.Fatalf("current-status prompt must not require a second prose verdict token:\n%s", prompt)
	}
	if !strings.Contains(prompt, "### Current status verdict synthesis") {
		t.Fatalf("current-status verdict lane missing from support plan:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Allowed block kinds: decision") {
		t.Fatalf("current-status verdict lane should explicitly allow decision blocks:\n%s", prompt)
	}
	if !strings.Contains(prompt, "- **decision** (exactly 1)") {
		t.Fatalf("semantic view should still require summary, path, and decision blocks:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SuppressesMultiTopicStructureWhenTypedSupportPlanExists(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "panic"}}},
				SubTopics: []types.SubTopic{
					{Summary: "panic 的直接触发点"},
					{Summary: "外层调用链"},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "panic", Frames: []types.LogFrame{{Raw: "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR(0x0)\n\tinternal/agent/analyzer.go:250 +0x1e", File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"}}}}},
		Mutable:   types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       743,
				AnchorKind:      types.AnchorCall,
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				AnchorSymbol:    "buildAnalysisIR",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/analyzer.go",
				LineStart:       978,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "buildAnalysisIR",
				Condition:       "ctx == nil || ctx.Mutable == nil",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "## Answer Structure (multi-topic)") {
		t.Fatalf("multi-topic scaffold should be suppressed when typed support lanes are authoritative:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRequestedEnumerationBoundary(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 7,
					SourceQuote:   "7 checks",
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	// Post-shape-retirement: an EnumerationBoundary obligation routes
	// the family to QFEnumeration (symbols-slate principal payload).
	for _, want := range []string{
		"## Requested Set Boundary",
		"`7 checks` (7 item(s))",
		"Keep the principal `ordered_list` block's `items[]` slate to 7 item(s)",
		"do not silently blend them into the principal set",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("requested set boundary prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_AttributeBearingEnumerationGuidance(t *testing.T) {
	mut := types.NewMutableState("q")
	syms := []types.AnswerSymbol{{
		Name:      "aggregator",
		File:      "internal/analysis/aggregator/aggregator.go",
		Line:      1,
		Kind:      types.KindPackage,
		Rationale: "entry function New at internal/analysis/aggregator/aggregator.go:112",
	}}
	mut.SetEmittedAnswerSymbols(syms, types.CompletenessComplete)
	ctx := &types.AgentContext{
		Mutable:                  mut,
		AnswerSymbols:            syms,
		AnswerSymbolCompleteness: types.CompletenessComplete,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "list all packages and each package entry point",
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				PredicateAxis: types.AxisDefine,
				AnalyzerHints: types.AnalyzerHints{
					PrimaryEntities: []string{"packages"},
					Entities:        []string{"aggregator", "compiler"},
				},
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 1,
					SourceQuote:   "all packages",
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all packages",
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"attribute-bearing enumeration",
		"separate the principal member set from per-member attributes",
		"Do not reduce the principal item count just because an attribute is missing",
		"Two-axis enumeration label rule",
		"the principal `ordered_list.items[].label` is the enumerated member itself",
		"Do not use an AnswerSymbol name as the item label when that symbol is the per-member attribute",
		"entry function New at internal/analysis/aggregator/aggregator.go:112",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("attribute-bearing finalizer prompt missing %q:\n%s", want, prompt)
		}
	}
}

// TestAnswerDocumentEvaluator_BuildInitialInstruction_NoFloorWithoutMustInclude
// checks the other branch: when MustInclude is empty, the prompt
// says there is no explicit required-member floor, so the finalizer
// keeps the rendered list aligned with the prior slate / evidence
// rather than inventing a retired completeness field.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_NoFloorWithoutMustInclude(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Required-member floor is empty") {
		t.Errorf("no-floor branch missing: %q", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SlateDoesNotReplaceRichLanes(t *testing.T) {
	syms := []types.AnswerSymbol{{
		Name:      "Eval",
		File:      "internal/analysis/criterion/eval.go",
		Line:      15,
		Kind:      types.KindFunction,
		Rationale: "core evaluator",
	}}
	mut := types.NewMutableState("公开函数")
	mut.SetEmittedAnswerSymbols(syms, types.CompletenessComplete)
	ctx := &types.AgentContext{
		Mutable:                  mut,
		AnswerSymbols:            syms,
		AnswerSymbolCompleteness: types.CompletenessComplete,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Prior slate from the extraction pipeline",
		"identity/citation skeleton",
		"not the complete explanation surface",
		"Do not let it replace richer per-member notes",
		"Principal Enumeration Rows",
		"Typed Answer Support Lanes",
		"same-row Evidence notes",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("finalizer prompt missing slate boundary %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_StopsOnCanceledContext(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := &types.AgentContext{
		Ctx: base,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "structural emit-time constraints") {
		t.Fatalf("canceled prompt should keep the schema pointer, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Required Answer Blocks") || strings.Contains(prompt, "Expected principal-item floor") {
		t.Fatalf("canceled prompt should stop before expensive dynamic sections, got:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersDecoratedSymbolHeaderContext(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:         types.EvidenceDirect,
		Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		LineStart:    7,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: "Index",
		Subject:      "Index",
		Producer:     "explorer.emit_evidence",
		SurfaceTerms: []string{"Index.ets", "@Entry"},
	}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
		AnswerSymbols: []types.AnswerSymbol{{
			Name:      "Index",
			File:      "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			Line:      7,
			Kind:      types.KindStruct,
			Rationale: "@Entry 页面入口",
		}},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"surface_terms",
		"preserve those exact structured terms",
		"Model-Emitted Surface Terms",
		"Index.ets, @Entry",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets:7",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("decorated symbol header context missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_EnumerationKeepsMoreThanSixSurfaceTerms(t *testing.T) {
	mut := types.NewMutableState("q")
	var evidence []types.EvidenceItem
	for i := 1; i <= 8; i++ {
		evidence = append(evidence, types.EvidenceItem{
			ID:           fmt.Sprintf("import-%02d", i),
			Kind:         types.EvidenceDirect,
			Scope:        types.ScopeLine,
			Source:       "internal/agent/explorer.go",
			LineStart:    10 + i,
			AnchorKind:   types.AnchorImport,
			AnchorSymbol: fmt.Sprintf("pkg%d", i),
			Producer:     "explorer.emit_evidence",
			SurfaceTerms: []string{fmt.Sprintf("github.com/acme/project/internal/pkg%d", i)},
		})
	}
	mut.AppendEvidence(evidence)
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for i := 1; i <= 8; i++ {
		want := fmt.Sprintf("github.com/acme/project/internal/pkg%d", i)
		if !strings.Contains(prompt, want) {
			t.Fatalf("enumeration surface terms should not be silently capped below complete small sets; missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_DropsUnrelatedPathSurfaceTermContext(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:         types.EvidenceDirect,
		Source:       "internal/agent/analyzer.go",
		LineStart:    1322,
		AnchorKind:   types.AnchorCall,
		AnchorSymbol: "analyzerGraphForNormalize",
		Subject:      "buildAnalysisIR",
		Object:       "analyzerGraphForNormalize",
		Producer:     "explorer.emit_evidence",
		SurfaceTerms: []string{"internal/types/enumeration_boundary.go"},
	}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "internal/types/enumeration_boundary.go") {
		t.Fatalf("irrelevant source-path surface term should not be rendered into finalizer prompt:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersDiagramContractAndSeeds(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					RequiredKind:   types.DiagramCallDAG,
					PreferredKinds: []types.DiagramKind{types.DiagramCallDAG},
					ScopeHint:      types.DiagramScopeOverall,
					Reasons:        []string{"axis_call"},
					Participants: []types.DiagramParticipantHint{
						{Identity: "Pipeline::Analyzer", Role: types.DiagramParticipantIncidentRequired},
						{Identity: "SharedContext", Role: types.DiagramParticipantContextOnly},
					},
				},
			},
		},
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Frames: []types.LogFrame{
					{File: "internal/a.go", Line: 10, Func: "inner"},
					{File: "internal/b.go", Line: 20, Func: "outer"},
				},
			}},
		},
		FlowFindings: []types.FlowFindingDigest{{
			Path:       []string{"Dispatch", "Handler"},
			Sources:    []string{"internal/a.go"},
			Conditions: []string{"kind == call"},
		}},
		AnswerChains: []types.AnswerChain{{
			StrictOK: true,
			Item: types.EvidenceItem{
				Summary:   "Dispatch routes to Handler",
				Source:    "internal/a.go",
				LineStart: 10,
			},
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Diagram Contract",
		"Required: yes",
		"Required kind: call_dag",
		"Preferred kinds: call_dag",
		"Typed participant obligations",
		"`Pipeline::Analyzer`: `incident_required`",
		"`SharedContext`: `context_only`",
		"Participant obligations guide investigation and honest coverage; they are not source evidence and cannot mint an edge",
		"EDGE DECISION FIRST",
		"USER-FACING DISPLAY LAYER",
		"structured evidence metadata, not visible diagram copy",
		"use concise domain/business actions for visible node labels and edge messages",
		"explain the semantic relationship in domain terms such as implements, calls, hands off, or precedes",
		"do not explain literal Mermaid operator tokens or arrow glyphs",
		"compatibility renderer may normalize an equivalent syntax family before display",
		"A display alias never proves an endpoint or relation",
		"This guidance supplies no labels, nodes, or edges; you remain their author",
		"stage/process/workflow order is `precedence/precedence_role`",
		"a conditional trigger is `guard/guard_condition`",
		"`call/call_edge` is only a direct invocation",
		"`register/registration_edge`",
		"`contain` has no edge-level claim_form",
		types.GroundedSourceDiagramRelationEvidenceContract,
		"Avoid invented enumeration labels like `Level 1`, `Round 2`, or `Step 3`",
		"async `-)` / `--)`, and lost-message `-x` / `--x`",
		"not decoration or an escape from edge evidence",
		"use the canonical identity grounded by topology/stage binding",
		"A repair/validation/helper function that only contains the stage name is not itself the component boundary",
		"an exact field/property ownership declaration grounds only the structural containment it states",
		"nesting the exact field/type node or subgroup inside the exact owner subgraph",
		"containment/grouping is not a call, transfer, execution order",
		"language-neutral across fields, properties, members, slots",
		"an exact typed participant may be the stable Mermaid endpoint node ID while its visible label uses concise business language",
		"The proved edge must terminate on that same node ID",
		"Do not draw the technical method as one endpoint and then invent a second method-to-component bridge",
		"One merge function does not prove that every source field is stored in the destination",
		"Do not synthesize bare line-number aliases such as `L877`, `Line 42`",
		"## Diagram Seeds",
		"### Grounded Labeling",
		"### Diagram Node Allowlist",
		"`internal/a.go`",
		"`internal/b.go`",
		"### Log Triage",
		"## First-Pass Diagram Reference",
		"Supporting anchor locations belong OUTSIDE the fence",
		// Log seed is now Mermaid (was bare-fence ASCII "innermost
		// failure:" prose). Assert on the grounded label that
		// survives both the Mermaid wrap and the later RenderMermaidBlocks
		// transformation.
		"innermost:",
		"Do not invent shorthand labels from citation line numbers",
		"A call-site citation proves only caller -> callee, not the callee's internal behavior or stage ordering",
		"split the hop and cite each line that actually proves that part",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "boundary_recipe[") {
		t.Fatalf("Trace lane must not receive non-runtime participant-boundary recipes:\n%s", prompt)
	}
	if strings.Contains(prompt, "Dispatch -> Handler") {
		t.Fatalf("unlinked flow finding must not widen the visible artifact support floor:\n%s", prompt)
	}
}

func TestRenderAnswerDocDiagramContractPublishesTypedUncoveredParticipantRecipesForFlow(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
	}}}
	dc := &types.DiagramContract{
		Required: true, RequiredKind: types.DiagramFlow,
		Participants: []types.DiagramParticipantHint{
			{Identity: "analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "surrounding system", Role: types.DiagramParticipantContextOnly},
		},
	}
	got := renderAnswerDocDiagramContract(ctx, dc)
	for _, want := range []string{
		"Typed uncovered-participant recipes",
		"boundary_recipe[1]: participant_identity=\"analyzer\"; visible_disconnected_node_first_line_identity=\"analyzer\"; boundary_row={\"participant\":\"analyzer\",\"status\":\"unproven\"}; edge_action=`none`",
		"boundary_recipe[2]: participant_identity=\"BusContext\"; visible_disconnected_node_first_line_identity=\"BusContext\"; boundary_row={\"participant\":\"BusContext\",\"status\":\"unproven\"}; edge_action=`none`",
		"choose any Mermaid-safe node ID",
		"copy `participant_identity` byte-for-byte",
		"Do not connect the node merely to satisfy coverage",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagram contract missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "participant_identity=\"surrounding system\"") {
		t.Fatalf("context_only participant must not receive an unproven boundary recipe:\n%s", got)
	}
}

func TestRenderAnswerDocDiagramContract_UserFacingDisplayGuidanceCoversEveryDiagramKind(t *testing.T) {
	for _, kind := range types.AllDiagramKinds() {
		t.Run(string(kind), func(t *testing.T) {
			got := renderAnswerDocDiagramContract(&types.AgentContext{}, &types.DiagramContract{
				Required:     true,
				RequiredKind: kind,
			})
			for _, want := range []string{
				"USER-FACING DISPLAY LAYER",
				"structured evidence metadata, not visible diagram copy",
				"Keep Mermaid node IDs stable and preserve producer-supplied exact source/stage endpoint selectors in `edge_anchors`",
				"use concise domain/business actions",
				"explain the semantic relationship in domain terms such as implements, calls, hands off, or precedes",
				"do not explain literal Mermaid operator tokens or arrow glyphs",
				"compatibility renderer may normalize an equivalent syntax family before display",
				"A display alias never proves an endpoint or relation",
				"This guidance supplies no labels, nodes, or edges; you remain their author",
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("%s diagram contract missing %q:\n%s", kind, want, got)
				}
			}
		})
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersConfigTraceDiagramSeed(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
					ScopeHint:      types.DiagramScopeOverall,
					Reasons:        []string{"config_lineage"},
				},
			},
		},
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token: "explore_mid_loop_hint_budget",
			Kind:  "symbol",
		}},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, Subject: "DefaultExploreHeuristics", Summary: "code defaults", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleDefault},
			{Source: "codrax.yaml.example", LineStart: 20, Subject: "ExploreHeuristics", Summary: "yaml precedence comment", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleYAML},
			{Source: "internal/config/runtime.go", LineStart: 194, Subject: "ExploreMidLoopMinIteration", Summary: "runtime yaml binding", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration", DiagramRole: types.EvidenceDiagramRoleRuntime},
			{Source: "cmd/root.go", LineStart: 1381, Summary: "CLI override applies when non-nil", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "OverrideLayer", DiagramRole: types.EvidenceDiagramRoleOverride},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"### Config Trace Precedence",
		"## First-Pass Diagram Reference",
		"## Precedence Role Coverage",
		"`code default` [default] → `internal/types/config.go:707`",
		"`config file` [config] → `codrax.yaml.example:20`",
		"`runtime binding` [runtime] → `internal/config/runtime.go:194`",
		"`operator override` [override] → `cmd/root.go:1381`",
		"codrax.yaml.example:20",
		"internal/types/config.go:707",
		"cmd/root.go:1381",
		"prefer the validated role-labeled precedence chain below",
		"compiled role abstraction backed by grounded evidence",
		"highest precedence at the top to lowest precedence at the bottom",
		// Pre-2026-04-30 the prompt named the literal `CLI` as the
		// concrete `override` example, which over-fit the s3a eval
		// case (eval/cases/s3a.case literally asks about "code
		// default / codrax.yaml / CLI"). Generalised to "operator-
		// supplied layer (CLI flag / env override / SDK setter /
		// RPC override — all the same tier)" so the binding rule
		// is structural, not vocabulary-specific.
		"highest-precedence operator-supplied layer",
		"all the same tier",
		"`runtime` is the binding / merge code path",
		"grounded FLOOR",
		"add additional grounded precedence layers when your investigation supports a richer chain",
		"The support locations listed above stay outside the fence",
		// New two-channel rule replaces the old "rename to a bare
		// `CLI` or tier label" prohibition. The validator works
		// off these structural channels (data-driven from the
		// EvidenceDiagramRole enum), not from prompt-side keyword
		// examples.
		"three structural grounding channels",
		"role label EXACTLY as listed",
		"<content> (<role-marker>)",
		"override` / `config` / `runtime` / `default",
		"concrete file path or symbol that appears in `citations[]`",
		"Drop those nodes from the fence and explain the concept in prose",
		"## Submission Checklist",
		"FLOOR you can extend, not a verbatim ceiling",
		"every fenced-diagram node must have its own grounded citation",
		"### Diagram Node Allowlist",
		"`codrax.yaml.example`",
		"`internal/config/runtime.go`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

// TestAnswerDocumentEvaluator_BuildInitialInstruction_NoLiteralCLIBlacklist
// is the dedicated guard against re-introducing the s3a over-fit:
// the prompt MUST NOT carry phrases that name `CLI` (or other
// vocabulary tokens from the s3a question) as a forbidden bucket
// label. The structural three-channel rule replaces the old literal
// blacklist, so neither the rejected old phrase nor a new
// equivalent should slip back in.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_NoLiteralCLIBlacklist(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Scenario: types.ScenarioConfigTrace},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleDefault},
			{Source: "codrax.yaml.example", LineStart: 20, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleConfig},
			{Source: "internal/config/runtime.go", LineStart: 194, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration", DiagramRole: types.EvidenceDiagramRoleRuntime},
			{Source: "cmd/root.go", LineStart: 1381, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "OverrideLayer", DiagramRole: types.EvidenceDiagramRoleOverride},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	// Specific over-fitted phrases that MUST NOT reappear.
	forbidden := []string{
		"a bare `CLI`",
		"the literal `CLI`",
		"renaming it to `Layer N`",
		"abstract bucket name (for example a generic step number, a bare `CLI`",
	}
	for _, ff := range forbidden {
		if strings.Contains(prompt, ff) {
			t.Errorf("prompt regressed to s3a-overfit phrase %q. Use the structural three-channel rule instead.", ff)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_DowngradesHardDiagramWithoutGroundedStructure(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramArchitecture, types.DiagramFlow},
				},
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					TargetLabel:  "config key",
					Targets:      []string{"missing_key"},
					AllowAbsence: true,
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Source:          "internal/types/config.go",
				LineStart:       707,
				GroundingStatus: types.GroundingGrounded,
				ContextRole:     types.EvidenceContextRoleAbsenceSupport,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "DefaultExploreHeuristics",
			},
		},
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token: "explore_mid_loop_hint_budget",
			Kind:  "symbol",
		}},
	}
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.SetAbsenceJustification("missing_key is absent from the current repo state")
	e := &answerDocumentEvaluator{}
	text := e.BuildInitialInstruction(ctx, nil)
	if strings.Contains(text, "## Diagram Contract") {
		t.Fatalf("hard diagram contract should downgrade when grounded structure is incomplete, got: %s", text)
	}
	if !strings.Contains(text, "## Diagram Preference") {
		t.Fatalf("downgraded diagram requirement should still leave a preference note, got: %s", text)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_ConfigTraceSeedWarnsWhenOverrideAnchorMissing(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
					ScopeHint:      types.DiagramScopeOverall,
					Reasons:        []string{"config_lineage"},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, Subject: "DefaultExploreHeuristics", Summary: "code defaults", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleDefault},
			{Source: "codrax.yaml.example", LineStart: 20, Subject: "ExploreHeuristics", Summary: "yaml precedence comment", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleYAML},
			{Source: "internal/config/runtime.go", LineStart: 194, Subject: "ExploreMidLoopMinIteration", Summary: "runtime yaml binding", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration", DiagramRole: types.EvidenceDiagramRoleRuntime},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Current grounded evidence does NOT include anchor(s) for these precedence role(s): override") {
		t.Fatalf("prompt missing generic missing-role warning:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_ConfigTraceSeedWarnsWhenYAMLAnchorMissing(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
					ScopeHint:      types.DiagramScopeOverall,
					Reasons:        []string{"config_lineage"},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, Subject: "DefaultExploreHeuristics", Summary: "code defaults", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleDefault},
			{Source: "internal/config/runtime.go", LineStart: 194, Subject: "ExploreMidLoopMinIteration", Summary: "runtime yaml binding", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration", DiagramRole: types.EvidenceDiagramRoleRuntime},
			{Source: "codrax.yaml.example", LineStart: 20, Subject: "ExploreHeuristics", Summary: "same-family background without a validated diagram role", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ExploreHeuristics"},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Current grounded evidence does NOT include anchor(s) for these precedence role(s): override, config") {
		t.Fatalf("prompt missing generic missing-role warning:\n%s", prompt)
	}
	if strings.Contains(prompt, "### Diagram Node Allowlist") && strings.Contains(prompt, "`codrax.yaml.example`") {
		t.Fatalf("diagram node allowlist must exclude non-diagram evidence labels when that role is missing:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SanitizesIllustrativeAbsenceJustification(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAbsenceJustification("searched the repo and found the token only in internal/skill/analysis_contract.go:367 comment examples")
	mu.SetInvestigationResultKind("absence")
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					TargetLabel:  "config key",
					Targets:      []string{"explore_mid_loop_hint_budget"},
					AllowAbsence: true,
				},
			},
		},
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token: "explore_mid_loop_hint_budget",
			Kind:  "symbol",
		}},
		EvidenceItems: []types.EvidenceItem{{
			Source:          "internal/skill/analysis_contract.go",
			LineStart:       367,
			Subject:         "explore_mid_loop_hint_budget",
			Summary:         "comment example only",
			ContextRole:     types.EvidenceContextRoleIllustrativeOnly,
			GroundingStatus: types.GroundingGrounded,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	contractStart := strings.Index(prompt, "## Exact Resolution Contract")
	if contractStart < 0 {
		t.Fatalf("prompt missing exact-resolution contract:\n%s", prompt)
	}
	contractEnd := strings.Index(prompt[contractStart+1:], "\n## ")
	contractBody := prompt[contractStart:]
	if contractEnd > 0 {
		contractBody = prompt[contractStart : contractStart+1+contractEnd]
	}
	if strings.Contains(contractBody, "internal/skill/analysis_contract.go:367") {
		t.Fatalf("exact-resolution contract should not echo illustrative-only source details into absence justification:\n%s", contractBody)
	}
	if !strings.Contains(prompt, "doc/test/example/comment-only mentions are illustrative only") {
		t.Fatalf("prompt should carry sanitized absence wording:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersExactResolutionContract(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	mu.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mu.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Subject:         "RuntimeSettings",
			Predicate:       "binds",
			Object:          "explore_midloop_min_iteration",
			Summary:         "RuntimeSettings exposes the YAML/runtime binding layer.",
			Source:          "internal/config/runtime.go",
			LineStart:       231,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "defines",
			Object:          "ExploreHeuristics defaults",
			Summary:         "DefaultExploreHeuristics defines the code defaults for explorer heuristics.",
			Source:          "internal/types/config.go",
			LineStart:       707,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "ExploreHeuristics",
			Predicate:       "documents",
			Object:          "heuristics config layer",
			Summary:         "codrax.yaml.example documents the three-layer precedence rule.",
			Source:          "codrax.yaml.example",
			LineStart:       25,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleYAML,
			GroundingStatus: types.GroundingRecovered,
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "ExploreBudget",
			Predicate:       "defines",
			Object:          "runtime budget counter",
			Summary:         "ExploreBudget is a runtime counter, not a config lineage anchor.",
			Source:          "internal/types/explore_budget.go",
			LineStart:       40,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:              types.SubjectConfigKey,
					TargetLabel:             "config key",
					Targets:                 []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:            true,
					RequireTargetMention:    true,
					AliasRequiresProof:      true,
					RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
					RelatedContextScopeHint: "same namespace / prefix family",
				},
			},
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "config_mapping",
					PrimaryEntities: []string{"explore_mid_loop_hint_budget"},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
		},
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token: "explore_mid_loop_hint_budget",
			Kind:  "symbol",
		}},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceMechanism,
				Subject:         "DefaultExploreHeuristics",
				Predicate:       "defines",
				Object:          "ExploreHeuristics defaults",
				Summary:         "DefaultExploreHeuristics defines the code defaults for explorer heuristics.",
				Source:          "internal/types/config.go",
				LineStart:       520,
				AnchorKind:      types.AnchorDefinition,
				DiagramRole:     types.EvidenceDiagramRoleDefault,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Exact Resolution Contract",
		"explore_mid_loop_hint_budget",
		"Absence-only is acceptable",
		"same namespace / prefix family",
		"you MUST set `exact_resolution.context_mode=\"grounded_context_only\"`",
		"Locked exact-resolution output",
		"Do not speculate about hypothetical parser / runtime behavior",
		"renderer will insert the exact-absence lead before `summary`",
		"treat `summary` as the follow-on grounded-context block only",
		"Keep the exact target name in the renderer-generated lead only",
		"Preferred exact_resolution object for this dispatch",
		"{\"status\":\"absent\",\"context_mode\":\"grounded_context_only\"}",
		"Summary surface mode: follow-on grounded context only",
		"does NOT license an invented field inventory",
		"Do not add a separate paragraph about the effect of supplying the absent target",
		"Surface-allowed nearby context is not automatically citation-grade",
		"only create a separate numbered step when that layer has its own grounded repo anchor",
		"repo-wide search result, aggregate absence conclusion, or test-only proof step usually has no single corroborating production line",
		"## Citation-Grade Grounded Context Anchors",
		"## Prose-Only Grounded Context Anchors",
		"Other surface-allowed anchors may still appear in `summary`, but only as uncited prose",
		"## Diagram-Grade Context Anchors",
		"## Related Context Citation Candidates",
		"## Background-Only Anchors",
		"## Exact Resolution Seeds",
		"DefaultExploreHeuristics",
		"codrax.yaml.example:25",
		"internal/types/config.go:520",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if !strings.Contains(prompt, "ExploreBudget") {
		t.Fatalf("background-only same-family anchors should be surfaced explicitly in the background-only section:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersCapabilitySurfaceAuthority(t *testing.T) {
	question := "Explorer stage 之前的 analyzer stage 里是否允许调用 read_file？"
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(question),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
			RequestModel: types.RequestModel{
				RawRequest: question,
				Scenario:   types.ScenarioGeneric,
				AnalyzerHints: types.AnalyzerHints{
					CapabilitySurface: &types.CapabilitySurfaceHint{
						Binding: types.StageBinding{
							Stage: types.StageAnalyze,
							Agent: types.AgentAnalyzer,
							Skill: "analysis-skill",
						},
						Tool: "read_file",
					},
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Capability Surface Authority",
		"`read_file`",
		"`analysis-skill` skill",
		"`ToolSuggestions`",
		"`buildToolSchemas`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "### Answer Chains") {
		t.Fatalf("diagram prompt should not expose legacy AnswerChains as a diagram seed:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_DiagramSeedsIgnoreLegacyAnswerChains(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RelatedContextTerms:  []string{"explore"},
				},
			},
		},
		AnswerChains: []types.AnswerChain{{
			StrictOK: true,
			Item: types.EvidenceItem{
				Kind:            types.EvidenceMechanism,
				Subject:         "DefaultExploreHeuristics",
				Predicate:       "explains",
				Object:          "nearby precedence baseline",
				Summary:         "This item names explore_mid_loop_hint_budget only in explanatory context; do NOT repair this item.",
				Source:          "internal/types/config.go",
				LineStart:       707,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "DefaultExploreHeuristics",
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				GroundingStatus: types.GroundingGrounded,
			},
		}},
	}

	seed := renderAnswerDocDiagramSupportSeed(ctx)
	if seed != "" {
		t.Fatalf("support seed should not be derived from legacy AnswerChains alone:\n%s", seed)
	}
	ctx.AnalysisIR.AnswerContract.Diagram = &types.DiagramContract{
		Required:       true,
		PreferredKinds: []types.DiagramKind{types.DiagramArchitecture},
	}
	seeds := renderAnswerDocDiagramSeeds(ctx, ctx.AnalysisIR.AnswerContract.Diagram)
	if strings.Contains(seeds, "### Answer Chains") {
		t.Fatalf("diagram prompt should not render legacy AnswerChains seed:\n%s", seeds)
	}
	if strings.Contains(seeds, "do NOT repair this item") {
		t.Fatalf("diagram seed leaked operational repair prose from legacy AnswerChains:\n%s", seeds)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersProseOnlyNearbyContextPolicy(t *testing.T) {
	mu := types.NewMutableState("")
	target := "explore_mid_loop_hint_budget"
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	mu.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mu.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Subject:         "RuntimeSettings",
			Predicate:       "binds",
			Object:          "explore_midloop_min_iteration",
			Source:          "internal/config/runtime.go",
			LineStart:       231,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "defines",
			Object:          "ExploreHeuristics defaults",
			Source:          "internal/types/config.go",
			LineStart:       707,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{target},
					AllowAbsence:         true,
					RequireTargetMention: true,
					AliasRequiresProof:   true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Nearby Context Citation Policy",
		"validated nearby grounded context is prose-only",
		"Do NOT place nearby grounded context anchors into `citations[]` or fenced diagrams",
		"Keep `citations[]` on the primary exact-proof / absence-proof anchors only",
		"`exact_resolution.context_mode=\"grounded_context_only\"`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Surface-Allowed Grounded Context Anchors") {
		t.Fatalf("surface-allowed section should be suppressed when nearby context is prose-only:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopExactResolutionLockedRejectUsesMetadata(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:exact_resolution] exact-resolution contract violated: the upstream investigation already closed as absence",
			Repair: &types.ToolRepair{
				Code: "exact_resolution",
				Metadata: map[string]string{
					"locked_status":          "absent",
					"preferred_context_mode": "grounded_context_only",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("locked exact-resolution reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		// G4 (post_v2_runtime_gap_remediation, 2026-05-04): "upstream
		// state" was R4-cleaned out of the LLM-facing hint. Lock the
		// remaining contract phrasing ("the status is already locked")
		// without the pipeline-shape leak.
		"the status is already locked",
		"`exact_resolution.status=\"absent\"`",
		"`exact_resolution.context_mode=\"grounded_context_only\"`",
		"Do not switch to `exact_match` or `alias_match`",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("locked exact-resolution hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestRenderAnswerDocDiagramGradeExactContextAnchors_UsesAnswerChainPool(t *testing.T) {
	mu := types.NewMutableState("explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？")
	mu.SetInvestigationResultKind("absence")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/config/runtime.go",
		LineStart:       32,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "RuntimeSettings",
		ContextRole:     types.EvidenceContextRoleAbsenceSupport,
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					AliasRequiresProof:   true,
					RequireTargetMention: true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
		AnswerChains: []types.AnswerChain{{
			Item: types.EvidenceItem{
				Kind:            types.EvidenceDirect,
				Source:          "internal/types/config.go",
				LineStart:       707,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "DefaultExploreHeuristics",
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				DiagramRole:     types.EvidenceDiagramRoleDefault,
				GroundingStatus: types.GroundingGrounded,
			},
		}},
	}
	got := renderAnswerDocDiagramGradeExactContextAnchors(ctx, ctx.AnalysisIR.AnswerContract.ExactResolution)
	if !strings.Contains(got, "internal/types/config.go:707") {
		t.Fatalf("diagram-grade anchors should include answer-chain precedence anchors, got:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopExactContextSurfaceRejectUsesMetadata(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2, diagramRequired: true}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:exact_context_surface] summary leaked background-only anchors",
			Repair: &types.ToolRepair{
				Code: "exact_context_surface",
				Metadata: map[string]string{
					"repeated_target":   "`explore_mid_loop_hint_budget`",
					"forbidden_anchors": "ExploreBudget, internal/config/runtime.go",
					"allowed_anchors":   "DefaultExploreHeuristics(), codrax.yaml.example",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("exact-context-surface reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"Do NOT restate `explore_mid_loop_hint_budget` in the `summary` block's text",
		"renderer already prints the exact-target lead",
		"`ExploreBudget`, `internal/config/runtime.go`",
		"`DefaultExploreHeuristics()`, `codrax.yaml.example`",
		"A grounded diagram is still required for this dispatch",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("exact-context-surface hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_PatchRejectCardinalityUsesTypedRepair(t *testing.T) {
	mut := types.NewMutableState("patch cardinality")
	mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "existing"},
			{ID: "table-stage-detail", Kind: types.BlockTable, Text: "existing"},
			{ID: "table-stage-detail_diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram"}},
			{ID: "diagram-stage", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram"}},
		},
	})
	ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mut}
	e := &answerDocumentEvaluator{mu: mut}
	result := &types.ToolResult{
		ToolName: "emit_answer_document_patch",
		Success:  false,
		Summary:  "patch rejected: rendered text is audit only",
		Repair: &types.ToolRepair{
			Code:   "answer_doc_pre_emit_contract",
			Fields: []string{"blocks[].kind=diagram"},
			Metadata: map[string]string{
				"violation_kinds": string(types.ViolBlockCoverageMissing),
				types.ToolRepairMetaBlockCardinalityRelation: "over_max",
				types.ToolRepairMetaOffendingBlockKinds:      string(types.BlockDiagram),
			},
		},
	}

	sig := e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
	if !sig.HintRequested || sig.HintKey != "answer_doc.patch_cardinality" {
		t.Fatalf("typed patch cardinality repair should trigger cardinality lane, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "exceeds the typed maximum for `kind=diagram`") ||
		!strings.Contains(sig.Hint, "explicitly delete surplus block ids with `remove_block_ids`") {
		t.Fatalf("cardinality hint should preserve merge guidance, got %q", sig.Hint)
	}
	for _, want := range []string{
		`{"id":"summary","kind":"summary"}`,
		`{"id":"table-stage-detail","kind":"table"}`,
		`{"id":"table-stage-detail_diagram","kind":"diagram"}`,
		`{"id":"diagram-stage","kind":"diagram"}`,
		`Existing ` + "`kind=diagram`" + ` block ids: ` + "`[\"table-stage-detail_diagram\",\"diagram-stage\"]`",
		"The model must choose which content to retain; the system does not choose or remove a block",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("cardinality hint missing exact patch-base roster %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocPatchBaseBlockRosterHintMirrorsPatchBasePrecedence(t *testing.T) {
	mut := types.NewMutableState("patch base precedence")
	mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "rejected", Kind: types.BlockDiagram}}})
	mut.SetRetryState(&types.RetryState{PrevEmitJSON: json.RawMessage(`{"blocks":[{"id":"retry","kind":"table"}]}`)})
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "live", Kind: types.BlockSummary}}})

	hint := answerDocPatchBaseBlockRosterHint(&types.AgentContext{Mutable: mut}, mut, types.BlockSummary)
	if !strings.Contains(hint, `[{"id":"live","kind":"summary"}]`) || strings.Contains(hint, "retry") || strings.Contains(hint, "rejected") {
		t.Fatalf("roster must mirror live > retry > rejected patch-base precedence: %q", hint)
	}
}

func TestAnswerDocumentEvaluator_PatchRejectSummaryOnlyDoesNotSelectCardinalityLane(t *testing.T) {
	mut := types.NewMutableState("patch cardinality summary only")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "existing"}},
	})
	ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mut}
	e := &answerDocumentEvaluator{mu: mut}
	result := &types.ToolResult{
		ToolName: "emit_answer_document_patch",
		Success:  false,
		Summary:  "patch rejected: blocks[].kind=section must reduce kind=section blocks",
	}

	sig := e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
	if !sig.HintRequested {
		t.Fatalf("summary-only patch reject should still get a generic patch correction hint, got %+v", sig)
	}
	if sig.HintKey == "answer_doc.patch_cardinality" {
		t.Fatalf("summary-only patch reject must not select cardinality lane, got %+v", sig)
	}
	if sig.HintKey != "answer_doc.patch_correct" {
		t.Fatalf("summary-only patch reject should use generic correction lane, got %+v", sig)
	}
}

func TestAnswerDocumentEvaluator_PatchRejectCardinalityIsBlockKindGeneric(t *testing.T) {
	mut := types.NewMutableState("ordered-list patch cardinality")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "existing"}},
	})
	ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mut}
	e := &answerDocumentEvaluator{mu: mut}
	result := &types.ToolResult{
		ToolName: "emit_answer_document_patch",
		Success:  false,
		Repair: &types.ToolRepair{
			Code:   "answer_doc_pre_emit_contract",
			Fields: []string{"blocks[].kind=ordered_list"},
			Metadata: map[string]string{
				"violation_kinds": string(types.ViolBlockCoverageMissing),
				types.ToolRepairMetaBlockCardinalityRelation: "over_max",
				types.ToolRepairMetaOffendingBlockKinds:      string(types.BlockOrderedList),
			},
		},
	}

	sig := e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: result})
	if !sig.HintRequested || sig.HintKey != "answer_doc.patch_cardinality" ||
		!strings.Contains(sig.Hint, "`kind=ordered_list`") {
		t.Fatalf("typed cardinality lane must be generic across block kinds: %+v", sig)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersScalarLookupDiscipline(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioGeneric,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Submission Checklist",
		"`scalar` block with the literal in block `text`",
		"Keep the complete scalar answer inside the answer document blocks",
		"names the subject being measured",
		"## Scalar Lookup Discipline",
		"one named source-code literal",
		"Do not expand into adjacent helpers",
		"Every non-negative `citation_ref` on a scalar payload must point at a real entry in `citations[]`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, banned := range []string{
		"value{literal, citation_ref}",
		"value{key, literal, citation_ref}",
		"boolean{decision, rationale, citation_ref}",
	} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("prompt must not teach retired V1 payload %q:\n%s", banned, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRoleLocateScalarDiscipline(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioGeneric,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
				AnalyzerHints: types.AnalyzerHints{
					Kind: "return_value",
				},
				PredicateAxis: types.AxisReturn,
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"role-locate lookup",
		"Do not promote the clue itself into the exact target lane",
		"answer with the located literal and its file:line first",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersLogTriageAndDiagramChecklist(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramSequence},
				},
			},
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
			},
		},
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Type:    "runtime error: invalid memory address or nil pointer dereference",
				Message: "nil pointer dereference while parsing analyzer output",
				Frames: []types.LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
					{File: "internal/agent/analyzer.go", Line: 320, Func: "ParseOutput"},
				},
			}},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Submission Checklist",
		"name each structured log error type or exception identifier from Log Triage",
		"the exact structured log error type(s) you must mention literally in `summary` are: `runtime error: invalid memory address or nil pointer dereference`",
		"Because the typed request profile marks this as a diagnostic / root-cause artifact question, preserve these structured log error message(s) verbatim in `summary` or body: `nil pointer dereference while parsing analyzer output`",
		"Every file/path node you keep inside a fenced diagram must also be grounded by `citations[]` or by attached Log Triage frames",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersLogSourceDriftGuidance(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
		},
		LogTriage: &types.LogBundle{
			ResolvedFiles: []string{"internal/agent/analyzer.go"},
			Errors: []types.LogError{{
				Frames: []types.LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				},
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       651,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "buildAnalysisIR",
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/analyzer.go",
				LineStart:       861,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "buildAnalysisIR",
				Condition:       "ctx == nil || ctx.Mutable == nil",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Log Source Drift",
		"older or shifted build snapshot",
		"Do not claim that the current cited line is the exact crashing line from the log",
		"## Typed Answer Support Lanes",
		"it does NOT by itself prove the runtime artifact actually passed the guard and reached the dereference path",
		"do NOT fill the gap with generic language-runtime guesses such as nil-map write, nil-slice index, field dereference",
		"### Observed artifact facts",
		"current grounded code exposes only a protective guard near the observed site",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "### Nearest grounded mechanism") {
		t.Fatalf("guard-only drift prompt should not surface a dedicated nearest-mechanism lane:\n%s", prompt)
	}
}

func TestCollectExactResolutionSeeds_FiltersDifferentConfigFamilies(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	ctx := &types.AgentContext{
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Subject:         "DefaultExploreHeuristics",
				Predicate:       "defines",
				Object:          "ExploreHeuristics defaults",
				Summary:         "DefaultExploreHeuristics defines the code defaults for explorer heuristics.",
				Source:          "internal/types/config.go",
				LineStart:       707,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Subject:         "DefaultLoopPolicy",
				Predicate:       "defines",
				Object:          "loop policy defaults",
				Summary:         "DefaultLoopPolicy returns loop-level defaults such as MaxMidLoopInjects=6.",
				Source:          "internal/agent/loop_policy.go",
				LineStart:       118,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	seeds := collectExactResolutionSeeds(ctx, contract)
	if len(seeds) == 0 {
		t.Fatal("expected exact-resolution seeds")
	}
	joined := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		joined = append(joined, seed.Text)
	}
	text := strings.Join(joined, "\n")
	if !strings.Contains(text, "DefaultExploreHeuristics") {
		t.Fatalf("expected same-family explore seed, got: %s", text)
	}
	if strings.Contains(text, "DefaultLoopPolicy") {
		t.Fatalf("different config family should not survive exact-resolution seeds, got: %s", text)
	}
}

func TestCollectExactResolutionSeeds_ConfigTraceRequiresDiagramRoleForNearbyContext(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Subject:         "ExploreBudget",
				Predicate:       "defines",
				Object:          "runtime budget counter",
				Summary:         "ExploreBudget is a runtime counter, not a config lineage anchor.",
				Source:          "internal/types/explore_budget.go",
				LineStart:       40,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Subject:         "DefaultExploreHeuristics",
				Predicate:       "defines",
				Object:          "ExploreHeuristics defaults",
				Summary:         "DefaultExploreHeuristics defines the code defaults for explorer heuristics.",
				Source:          "internal/types/config.go",
				LineStart:       707,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				DiagramRole:     types.EvidenceDiagramRoleDefault,
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Subject:         "RuntimeSettings",
				Predicate:       "binds",
				Object:          "explore_midloop_min_iteration",
				Summary:         "RuntimeSettings binds the YAML override layer.",
				Source:          "internal/config/runtime.go",
				LineStart:       231,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				DiagramRole:     types.EvidenceDiagramRoleRuntime,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	seeds := collectExactResolutionSeeds(ctx, contract)
	if len(seeds) == 0 {
		t.Fatal("expected exact-resolution seeds")
	}
	var joined []string
	for _, seed := range seeds {
		joined = append(joined, seed.Text)
	}
	text := strings.Join(joined, "\n")
	if strings.Contains(text, "ExploreBudget") {
		t.Fatalf("same-family symbol without validated diagram role should be filtered in config-trace, got: %s", text)
	}
	for _, want := range []string{"DefaultExploreHeuristics", "RuntimeSettings"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected config-lineage seed %q, got: %s", want, text)
		}
	}
}

func TestAnswerDocumentAuthoritativeSurfacesIgnoreOrdinarySummary(t *testing.T) {
	ordinary := types.EvidenceItem{
		Kind:            types.EvidenceDirect,
		Source:          "internal/example/thing.go",
		LineStart:       7,
		Summary:         "ordinary summary claims the target is fully proven",
		GroundingStatus: types.GroundingGrounded,
	}

	if got := formatExactResolutionSeed(ordinary); strings.Contains(got, "ordinary summary claims") {
		t.Fatalf("exact-resolution seed leaked ordinary summary: %q", got)
	}
	if got := formatExactResolutionSurfaceSeed(ordinary); strings.Contains(got, "ordinary summary claims") {
		t.Fatalf("surface seed leaked ordinary summary: %q", got)
	}
	row, ok := answerDocRelationSurfaceRowForEvidence(nil, types.EvidenceItem{
		Kind:            types.EvidenceDataflowPath,
		Producer:        "consumer_gate",
		Subject:         "handoff",
		Source:          "internal/example/flow.go",
		LineStart:       12,
		Summary:         "ordinary summary invents the receiving component",
		GroundingStatus: types.GroundingGrounded,
	}, 0)
	if !ok {
		t.Fatal("relation surface should keep typed subject-only rows")
	}
	if strings.Contains(row.surface, "ordinary summary invents") {
		t.Fatalf("relation surface leaked ordinary summary: %+v", row)
	}

	ctx := &types.AgentContext{
		EvidenceItems: []types.EvidenceItem{ordinary},
	}
	if rows := answerDocumentFallbackEvidenceRows(ctx, 8, 200); len(rows) != 0 {
		t.Fatalf("degraded fallback evidence rows must not use ordinary summary-only evidence: %+v", rows)
	}
}

func TestAnswerDocumentAuthoritativeSurfacesAllowLoadBearingSummary(t *testing.T) {
	item := types.EvidenceItem{
		Kind:               types.EvidenceDirect,
		Source:             "internal/example/version.go",
		LineStart:          9,
		Summary:            "version stamp v1.2.3",
		LoadBearingSummary: true,
		GroundingStatus:    types.GroundingGrounded,
	}

	if got := formatExactResolutionSeed(item); !strings.Contains(got, "v1.2.3") {
		t.Fatalf("load-bearing exact-resolution seed lost summary scalar: %q", got)
	}
	if got := formatExactResolutionSurfaceSeed(item); !strings.Contains(got, "v1.2.3") {
		t.Fatalf("load-bearing surface seed lost summary scalar: %q", got)
	}
	ctx := &types.AgentContext{
		EvidenceItems: []types.EvidenceItem{item},
	}
	rows := answerDocumentFallbackEvidenceRows(ctx, 8, 200)
	if len(rows) != 1 || !strings.Contains(rows[0].text, "v1.2.3") {
		t.Fatalf("load-bearing fallback evidence row lost summary scalar: %+v", rows)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_UsesStableAbsenceStateAfterWindowReset(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetInvestigationComplete("all three nearby precedence layers were already traced before the window reset")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	mu.ResetInvestigationComplete()
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:              types.SubjectConfigKey,
					TargetLabel:             "config key",
					Targets:                 []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:            true,
					RequireTargetMention:    true,
					AliasRequiresProof:      true,
					RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
					RelatedContextScopeHint: "same namespace / prefix family",
				},
			},
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:         "config_mapping",
					ExactTargets: []string{"explore_mid_loop_hint_budget"},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
		},
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token: "explore_mid_loop_hint_budget",
			Kind:  "symbol",
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Investigation state: the exact target is currently absent in the repo / branch under inspection.",
		"no config key named `explore_mid_loop_hint_budget` exists in the repo",
		"## Accepted Closure Status",
		"Treat this closure as a structured exploration handoff",
		"model-authored closure reason: all three nearby precedence layers were already traced before the window reset",
		"Emit `exact_resolution.status=\"absent\"`",
		"do NOT emit a principal scalar block with a synthetic literal",
		"grounded same-scope anchors may appear in `summary` even when they do not carry a validated diagram role",
		"Prefer a summary-led explanation",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q after reset:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersSanitizedInvestigationNarrativeHandoff(t *testing.T) {
	mu := types.NewMutableState("compare repos")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		InvestigationNotes: []string{
			"early stale note",
			"<think>internal reasoning</think>\n<minimax:tool_call>{\"name\":\"grep\"}</minimax:tool_call>\n调查完成：`frameworks/base` 与 `hm_z/foundation/resourceschedule/ressched` 没有直接接口交互；一个仓是 Android framework，另一个仓是 OHOS 资源调度子系统。",
		},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Investigation Narrative Handoff",
		"advisory synthesis only",
		"cross-bucket / cross-repository distinctions",
		"没有直接接口交互",
		"Android framework",
		"OHOS 资源调度子系统",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, banned := range []string{"<think>", "<minimax:tool_call>", "{\"name\":\"grep\"}"} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("prompt leaked sanitized scaffold %q:\n%s", banned, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersPrimaryAbsenceProofCitationSeeds(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetInvestigationComplete("bounded search already confirmed the exact key is absent")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:      types.EvidenceAbsent,
				Subject:   "explore_mid_loop_hint_budget",
				Predicate: "absent_in",
				Object:    "cmd/root.go",
				Source:    "cmd/root.go",
				Scope:     types.ScopeNegative,
				NegativeQuery: &types.NegativeQuery{
					File:    "cmd/root.go",
					Pattern: "ExploreMidLoopHintBudget|ExploreMidLoop.*Budget",
				},
				NegativeScope:   types.NegativeScopeFile,
				ContextRole:     types.EvidenceContextRoleAbsenceSupport,
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Subject:         "DefaultExploreHeuristics",
				Predicate:       "defines",
				Object:          "ExploreHeuristics defaults",
				Source:          "internal/types/config.go",
				LineStart:       707,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				DiagramRole:     types.EvidenceDiagramRoleDefault,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Primary Exact-Proof / Absence-Proof Citation Seeds",
		"`{\"file\":\"cmd/root.go\",\"negative_pattern\":\"ExploreMidLoopHintBudget|ExploreMidLoop.*Budget\",\"scope\":\"negative\"}`",
		"In `exact_resolution.status=\"absent\"` mode, at least one cited seed MUST be an absence-proof object",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_ConfigTraceAbsenceAsksForExplicitMissingLayerWording(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetInvestigationComplete("bounded search already confirmed the exact key is absent")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RequestedContextRoles: []types.EvidenceDiagramRole{
						types.EvidenceDiagramRoleDefault,
						types.EvidenceDiagramRoleConfig,
						types.EvidenceDiagramRoleOverride,
					},
				},
			},
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
				Buckets: []types.QuestionBucket{
					{Label: "code default", Index: 1},
					{Label: "codrax.yaml", Index: 2},
					{Label: "CLI", Index: 3},
				},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"say that layer absence explicitly",
		"`no config-file key matches this target`",
		"`no CLI flag binds this key`",
		"instead of vague placeholders like `N/A` / `不适用`",
		"Populate document-level `missing_requested_roles[]` with the missing requested layers below",
		"## Explicit Missing-Layer Wording",
		"`CLI`, prefer explicit absence wording such as `CLI 层未绑定该键` or `no CLI flag binds this key`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_DoesNotMentionRetiredTopLevelScalarPayloads(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
			},
		},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFRoleLookup,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, SurfaceRoleHint: types.SurfacePrincipal},
			{Kind: types.BlockScalar, Required: true, SurfaceRoleHint: types.SurfacePrincipal},
			{Kind: types.BlockDecision, Required: true, SurfaceRoleHint: types.SurfacePrincipal},
		},
	}

	prompt := renderAnswerDocSubmissionChecklist(ctx, view, false)
	for _, forbidden := range []string{"value{", "boolean{", "retired top-level scalar payload", "retired top-level decision payload"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("submission checklist must not mention retired top-level payload %q:\n%s", forbidden, prompt)
		}
	}
	for _, want := range []string{
		"Keep the complete scalar answer inside the answer document blocks",
		"Keep the complete decision answer inside the answer document blocks",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("submission checklist missing positive guidance %q:\n%s", want, prompt)
		}
	}
}

func TestRenderAnswerDocSubmissionChecklist_SectionOwnsStructuredRowsAndCitations(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Buckets: []types.QuestionBucket{{Label: "A", Index: 1}, {Label: "B", Index: 2}},
	}}}
	view := &types.AnswerSemanticView{
		Family: types.QFComparison,
		RequiredBlocks: []types.BlockRequirement{{
			Kind: types.BlockSection, Required: true, SurfaceRoleHint: types.SurfacePrincipal,
		}},
	}

	got := renderAnswerDocSubmissionChecklist(ctx, view, false)
	for _, want := range []string{
		"A section may also carry structured `items[]`",
		"primary citation_ref and optional additional citation_refs",
		"put those rows once in the section's items[]",
		"instead of duplicating the roster in a separate global list or table",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("section JSON teaching missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Section blocks have no built-in citation field") {
		t.Fatalf("section JSON teaching retained the false citation-carrier instruction:\n%s", got)
	}
}

func TestRenderAnswerDocSubmissionChecklist_EnumerationUsesMultiCitationCarrierWithoutMandate(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentEnumerate}}}
	view := &types.AnswerSemanticView{
		Family: types.QFEnumeration,
		RequiredBlocks: []types.BlockRequirement{{
			Kind: types.BlockOrderedList, Required: true, SurfaceRoleHint: types.SurfacePrincipal,
		}},
	}

	got := renderAnswerDocSubmissionChecklist(ctx, view, false)
	for _, want := range []string{
		"primary `citation_ref=N`",
		"optional additional `citation_refs=[...]`",
		"only when that same item states separately supported facts from several already-selected anchors",
		"Never add an unselected anchor merely to fill the array",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("enumeration checklist missing soft multi-citation guidance %q:\n%s", want, got)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildRetryInstruction_UsesPluralBlockClaimUsesPath(t *testing.T) {
	mut := types.NewMutableState("")
	mut.SetRetryState(&types.RetryState{
		Attempt: 1,
		PrevEmitSummary: types.RetryStateSummary{
			BlockSummaries: []types.RetryBlockSummary{{
				ID:   "b1",
				Kind: types.BlockSummary,
			}},
		},
		ActiveViolations: []types.ScoredViolation{{
			Severity:  types.SeverityHigh,
			Kind:      types.ViolPrincipalClaimUseMissing,
			FieldPath: "blocks[0].claim_uses",
			Detail:    "principal block missing claim use",
			Repair:    "add block-level claim_uses",
			Layer:     "finalize",
		}},
	})
	hint := renderAnswerDocRetryState(&types.AgentContext{Mutable: mut})
	if !strings.Contains(hint, "`blocks[].claim_uses[]`") {
		t.Fatalf("retry instruction must mention blocks[].claim_uses[] path:\n%s", hint)
	}
	if strings.Contains(hint, "`blocks[].claim_use`") {
		t.Fatalf("retry instruction must not mention stale blocks[].claim_use path:\n%s", hint)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_DoesNotHardRequireLogMessagesForNonDiagnosticIntent(t *testing.T) {
	ctx := &types.AgentContext{
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Type:    "panic",
				Message: "index out of bounds: index=5, size=3",
			}},
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "preserve these structured log error message(s) verbatim") {
		t.Fatalf("non-diagnostic intent must not hard-require log message literals:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Structured log error type") {
		t.Fatalf("log artifact should still be available as soft context:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersExternalObservationSeeds(t *testing.T) {
	ctx := &types.AgentContext{
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Type: "runtime error: invalid memory address or nil pointer dereference",
				Frames: []types.LogFrame{
					{
						File: "internal/agent/analyzer.go",
						Line: 320,
						Func: "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput",
						Raw:  "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput(0x0, 0x0, 0x0)",
					},
				},
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       651,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "buildAnalysisIR",
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				GroundingStatus: types.GroundingGrounded,
			},
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Typed Answer Support Lanes",
		"### Observed artifact facts",
		"structured runtime error type",
		`runtime artifact identifies error head stack frame "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput" at observed internal/agent/analyzer.go:320`,
		"internal/agent/analyzer.go:651",
		"Items rendered under the **Observed artifact facts** lane are runtime trace observations",
		"possible upstream investigation direction",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersHarmonyTracePriorityReminder(t *testing.T) {
	mut := types.NewMutableState("这是一段 OpenHarmony/鸿蒙 bytrace 文本，分析 sched_wakeup 调度和优先级")
	mut.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{{
			Kind:    "priority_semantics",
			Subject: "HarmonyOS priority semantics",
			Summary: "Harmony priority semantics: prio=120/ohos_rt observed in attached trace",
			Tags:    []string{"harmony_priority", "prio=120/ohos_rt"},
		}},
	})
	ctx := &types.AgentContext{
		Objective:             "这是一段 OpenHarmony/鸿蒙 bytrace 文本，分析 sched_wakeup 调度和优先级",
		AttachedHitraceSource: "harmony_hitrace",
		Mutable:               mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Harmony trace priority reminder",
		"数值越大优先级越高",
		"prio=20 is CFS",
		"prio=41, prio=140, and prio=159 are RT",
		"prio=160 and prio=301 are raw system/kernel scheduler tokens",
		"Recompute every concrete `prio=N` classification",
		"Runtime trace presentation hint",
		"Scheduler state authority hint",
		"`sched_switch prev_state=S`",
		"does not by itself prove RT preemption",
		"Only `R`/`R+` supports a still-runnable preemption candidate",
		"running slices is not a wakeup count",
		"never authority for `all`, `only`, `total`, exact `N`, `max`, or `min` claims",
		"Scheduler transition interval hint",
		"t_sleep→t_wake is sleep/blocking until wake",
		"t_wake→t_run is runnable scheduling delay",
		"Never label the total non-running interval as wakeup latency",
		"do not invent an unrequested duration",
		"Scheduler evidence-absence hint",
		"unavailable scheduler residency, not proven continuous On-CPU execution",
		"cannot prove running time, no blocking, no runnable delay, or a CPU-bound bottleneck",
		"scheduler decomposition is unavailable",
		"Scheduler residency and cross-subject causality hint",
		"S, D, or `io_wait` is not occupying a CPU",
		"typed wakeup/IPC/lock/flow/dependency connector",
		"keep their relationship unproven",
		"Thread role authority hint",
		"does not prove main-thread, UI-thread, render-thread, or render-service ownership",
		"`kind=thread_role`",
		"Current span/name-derived frame rows do not provide",
		"`frame_marker_role` and `pipeline_stage_role` describe the marker/item stage",
		"Runtime metric snapshot hint",
		"one metric snapshot line",
		"Runtime root-cause layering hint",
		"direct scheduler wait",
		"upstream dependency-chain IO/D-state",
		"auxiliary context",
		"Runtime priority-inversion authority hint",
		"does NOT prove that inversion occurred",
		"no priority-inversion impact was measured in this window",
		"Do not narrow measured inversion to same-CPU preemption",
		"cross-CPU weak-core/compute-supply running deficit",
		"`own·runnable`",
		"Runtime direct-blocking hint",
		"direct blocking surface",
		"`sync_like`",
		"`blocking_candidate`",
		"peer/on-chain thread state",
		"Runtime Binder direction hint",
		"thread on a `binder_transaction` row is the emitter",
		"`call_semantics=reply`",
		"typed `call_semantics=sync_request`",
		"Runtime trace handoff hint",
		"`cumulative_impact_ms`",
		"`occurrence_windows`",
		"representative repeated windows",
		"compare same-chain primary rows by cumulative impact before score",
		"preserve that next-step guidance visibly",
		"prefer the bounded `trace_query` facts",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_DoesNotRenderRuntimeTraceGuidanceFromRawQuestionWords(t *testing.T) {
	mut := types.NewMutableState("这是一段 OpenHarmony/鸿蒙 bytrace 文本，分析 sched_wakeup 调度和优先级")
	ctx := &types.AgentContext{
		Objective:             "这是一段 OpenHarmony/鸿蒙 bytrace 文本，分析 sched_wakeup 调度和优先级",
		AttachedHitraceSource: "harmony_hitrace",
		Mutable:               mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, forbidden := range []string{
		"## Runtime Trace Answer Guidance",
		"Harmony trace priority reminder",
		"Runtime trace presentation hint",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("raw question / attachment wording must not trigger runtime trace guidance %q:\n%s", forbidden, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRequestedAnswerDimensions(t *testing.T) {
	ctx := &types.AgentContext{
		Language: "zh",
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true,
					Dimensions: []types.RequestedAnswerDimension{
						{Label: "diff 线索", Role: types.RequestedAnswerDimensionDiffClue, SourceQuote: "diff 线索", Required: true, Index: 1},
						{Label: "当前关键代码", Role: types.RequestedAnswerDimensionCurrentKeyCode, SourceQuote: "当前关键代码", Required: true, Index: 2},
						{Label: "作用和影响", Role: types.RequestedAnswerDimensionImpact, SourceQuote: "作用和影响", Required: true, Index: 3},
					},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## 用户要求的答案维度",
		"diff 线索",
		"当前关键代码",
		"作用和影响",
		"展示契约，不是新的证据来源",
		"每个主体下面都应尽量显式保留这些维度标签",
		"不要为了套表格而删除、替换或压扁更丰富的说明",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestRenderAnswerDocTargetWaitOccurrenceAuthorityBypassesLedgerAndRepairBudgets(t *testing.T) {
	count := 3
	observation := types.ObservationRecord{
		ID:              "trace_query:window#target_window_wait_occurrences",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		Subject:         "main-59566",
		Predicate:       "target_window_wait_occurrences",
		Object:          "complete",
		Value:           "3",
		ResultCount:     &count,
		RichNotes: []string{
			types.TraceNoteKeyTargetWaitOccurrencePrompt + "=status=complete,emitted=3,total=3",
			types.TraceNoteKeyTargetWaitOccurrencePromptSum + "=0.635",
			types.TraceNoteKeyTargetWaitOccurrence + "=#1 state=io_wait 34579.451701..34579.451839 duration=0.138ms iowait=1 caller=sync_buffer_read_wi lines=1-2 reason_line=3",
			types.TraceNoteKeyTargetWaitOccurrence + "=#2 state=io_wait 34579.452934..34579.453081 duration=0.147ms iowait=1 caller=sync_buffer_read_wi lines=4-5 reason_line=6",
			types.TraceNoteKeyTargetWaitOccurrence + "=#3 state=io_wait 34579.471372..34579.471722 duration=0.350ms iowait=1 caller=sync_buffer_read_wi lines=7-8 reason_line=9",
		},
	}
	mut := types.NewMutableState("q")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: []types.ObservationRecord{observation},
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeTargets: []types.RuntimeTarget{{
				Kind:   types.RuntimeTargetKindThread,
				PID:    59566,
				Thread: "main-59566",
				Source: "user_explicit",
			}},
		}},
	}
	out := renderAnswerDocTargetWaitOccurrenceAuthority(ctx)
	for _, want := range []string{
		"## Typed Target Wait Occurrence Authority",
		"count=3; sum_ms=0.635",
		"34579.451701..34579.451839 duration=0.138ms",
		"34579.452934..34579.453081 duration=0.147ms",
		"34579.471372..34579.471722 duration=0.350ms",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dedicated target occurrence authority missing %q:\n%s", want, out)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRuntimeGroundingDisposition(t *testing.T) {
	mut := types.NewMutableState("q")
	// A runtime artifact MUST be attached for the disposition to
	// activate — the disposition's whole vocabulary ("the attached
	// log / trace") presumes an artifact in scope. The 2026-05-16
	// finalize-loop bug fix gates the waiver→disposition projection
	// on this. Without the LogTriage below, the prompt section would
	// (correctly) stay empty.
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{
			Func: "synthetic_frame",
		}}}},
	})
	mut.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason:    types.EvidenceFloorWaiverNoRepoIntersection,
		Rationale: "synthetic frames represent a different deployed build",
	})
	mut.RetainEvidenceFloorWaiver(true)
	mut.SetInvestigationComplete("IndexPage supplied the undefined user object, but the artifact directly shows only the RuntimeError frame")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedClosureReason: "IndexPage supplied the undefined user object, but the artifact directly shows only the RuntimeError frame",
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:  types.ScenarioRootCause,
				Intent:    types.IntentRootCause,
				LogTriage: mut.LogTriage(),
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Runtime Grounding Disposition",
		"`no_repo_intersection`",
		"synthetic frames represent a different deployed build",
		"runtime/artifact provenance lane",
		"This dispatch is runtime-artifact scoped",
		"Do not emit `current_status_verdict`",
		"trace-observed cause/risk",
		"model-authored closure reason omitted from this authority section",
		"Accepted runtime closure reason (advisory only",
		"possible upstream investigation direction",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "model-authored closure reason: IndexPage supplied") {
		t.Fatalf("runtime observation-only closure reason must not appear as authority text:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_RuntimeOnlyMultiTopicDoesNotDemandRepoCitations(t *testing.T) {
	mut := types.NewMutableState("runtime artifact only")
	mut.SetLogTriage(&types.LogBundle{Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{Func: "synthetic_frame"}}}}})
	mut.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason: types.EvidenceFloorWaiverNoRepoIntersection, Rationale: "different deployed build",
	})
	mut.RetainEvidenceFloorWaiver(true)
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:  types.ScenarioGeneric,
				Intent:    types.IntentExplain,
				LogTriage: mut.LogTriage(),
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					Confidence:        0.9,
				},
				SubTopics: []types.SubTopic{{Summary: "runtime symptom"}, {Summary: "runtime cause boundary"}},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Answer Structure (multi-topic)",
		"model-authored planning labels, not evidence or accepted conclusions",
		"recompute every quantity, interval, state, causal claim, and implementation fact",
		"preserve the attached-artifact provenance in each section",
		"do not invent current-repo citations",
		"only when a separate typed current-source anchor directly supports that section",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("runtime-only multi-topic citation boundary missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "\nProvide citations for each section.\n") {
		t.Fatalf("runtime-only multi-topic prompt retained contradictory repo citation demand:\n%s", prompt)
	}
	for _, forbidden := range []string{"## Trace Decision Inputs", "## Final Trace Decision Boundary"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("log-only multi-topic prompt leaked trace-only contract %q:\n%s", forbidden, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_PreciseRuntimeSourceDoesNotRenderObservationOnly(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{
			Func: "synthetic_frame",
		}}}},
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:  types.ScenarioRootCause,
				Intent:    types.IntentRootCause,
				LogTriage: mut.LogTriage(),
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: true,
					Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
					SourceQuotes:                        []string{"internal/tracequery/parse.go:42"},
					Confidence:                          0.9,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	if runtimeObservationOnlyForAnswerDoc(ctx) {
		t.Fatal("precise current-source obligation must not render observation-only runtime disposition")
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, forbidden := range []string{
		"This dispatch is runtime-artifact scoped: current checkout/source evidence is not required",
		"Do not emit `current_status_verdict`, and do not use visible `still_present` / `fixed` status wording",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("precise runtime/source prompt should not contain observation-only wording %q:\n%s", forbidden, prompt)
		}
	}
	if !strings.Contains(prompt, "Current repository citations may still be used") {
		t.Fatalf("precise runtime/source prompt should preserve current citation allowance:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SourceOptionalTraceSkipsCurrentStatus(t *testing.T) {
	perf := &types.PerfBundle{
		Observations: []types.PerfObservation{{
			Kind:      "state_churn",
			Subject:   "app-20",
			Summary:   "state_churn app-20 dominant_state=runnable next_step=compare rival-30 on same CPU",
			LineStart: 3,
			LineEnd:   23,
		}},
	}
	mut := types.NewMutableState("trace window stats")
	mut.SetPerfTrace(perf)
	ctx := &types.AgentContext{
		Objective:             "use trace_query window_stats to analyse sched state_churn for app-20",
		AttachedHitrace:       "sched_switch app-20 rival-30",
		AttachedHitraceSource: "harmony_hitrace",
		Mutable:               mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:  types.ScenarioPerformanceBottleneck,
				Intent:    types.IntentRootCause,
				PerfTrace: perf,
				AnalyzerHints: types.AnalyzerHints{
					ExactTargets: []string{"app-20", "11.0s-11.008s"},
				},
				Predicates: types.SemanticPredicates{IsDiagnosticQuestion: true},
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
					CurrentRisk:  true,
					Confidence:   0.95,
				},
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
					CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
					Confidence:           0.95,
				},
				SourceScopeProfile: &types.SourceScopeProfile{
					RequestedScope: types.SourceScopeProduction,
					Confidence:     0.9,
				},
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true,
					Dimensions: []types.RequestedAnswerDimension{{
						Label:    "dominant_state",
						Role:     types.RequestedAnswerDimensionStageWorkflow,
						Required: true,
						Index:    1,
					}},
					Confidence: 0.9,
				},
			},
			AnswerContract: types.AnswerContract{
				CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Runtime Grounding Disposition",
		"Runtime root-cause layering hint",
		"Runtime priority-inversion authority hint",
		"lower_priority_waker",
		"cross-CPU weak-core/compute-supply running deficit",
		"Runtime direct-blocking hint",
		"`sync_like`",
		"`blocking_candidate`",
		"Runtime trace handoff hint",
		"preserve that next-step guidance visibly",
		"This dispatch is runtime-artifact scoped",
		"Do not emit `current_status_verdict`",
		"trace-observed cause/risk",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"## Current Status Diagnostic",
		"Verdict: emit a principal `decision` block with `current_status_verdict`",
		`**Must declare (emit-time rejection if any are missing from every block's ` + "`facet_ids`" + ` and ` + "`claim_uses[].facet_id`" + `):** "` + string(types.FacetCurrentCodePath) + `"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("source-optional trace prompt should not contain %q:\n%s", forbidden, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_TraceQueryLedgerSkipsCurrentStatus(t *testing.T) {
	mut := types.NewMutableState("trace_query root cause rank")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		RawRef:   "[trace_query params: view=root_cause_rank source=path path=/tmp/donghu.systrace origin=runtime_artifact artifact_kind=trace]",
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:root_cause_rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				ArtifactID:   "donghu_trace",
				ArtifactKind: "trace",
				Path:         "/tmp/donghu.systrace",
			},
			Subject:   "ThreadPoolForeg-59566",
			Predicate: "root_cause_primary",
			Object:    "sleep_wait",
			Summary:   "root_cause_rank rank=1 chain_relevance=on_chain impact=63.0ms",
		}},
	})
	ctx := &types.AgentContext{
		Objective: "只分析这份 trace，不分析代码，说明主线程卡顿根因",
		Mutable:   mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioPerformanceBottleneck,
				Intent:   types.IntentRootCause,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
					CurrentRisk:  true,
					Confidence:   0.95,
				},
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
					CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
					Confidence:           0.95,
				},
			},
			AnswerContract: types.AnswerContract{
				CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Runtime Grounding Disposition",
		"Do not emit `current_status_verdict`",
		"runtime-observed cause/risk",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"## Current Status Diagnostic",
		"Verdict: emit a principal `decision` block with `current_status_verdict`",
		"blocks[kind=decision].current_status_verdict",
		`**Must declare (emit-time rejection if any are missing from every block's ` + "`facet_ids`" + ` and ` + "`claim_uses[].facet_id`" + `):** "` + string(types.FacetCurrentCodePath) + `"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("trace_query runtime-only prompt should not contain %q:\n%s", forbidden, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRuntimeClosureReasonWithoutTurnAArtifacts(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{
			Func: "synthetic_frame",
		}}}},
	})
	mut.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason:    types.EvidenceFloorWaiverNoRepoIntersection,
		Rationale: "synthetic frames represent a different deployed build",
	})
	mut.RetainEvidenceFloorWaiver(true)
	mut.SetInvestigationComplete("artifact line 12 shows RuntimeError; current checkout was not part of this observation-only answer")
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:  types.ScenarioRootCause,
				Intent:    types.IntentRootCause,
				LogTriage: mut.LogTriage(),
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"model-authored closure reason omitted from this authority section",
		"Accepted runtime closure reason (advisory only",
		"artifact line 12 shows RuntimeError",
		"possible upstream investigation direction",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "model-authored closure reason: artifact line 12") {
		t.Fatalf("runtime observation-only closure reason must remain advisory narrative, not authority text:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SkipsRuntimeClosureNarrativeWhenTraceQueryRowsPresent(t *testing.T) {
	perf := &types.PerfBundle{
		Observations: []types.PerfObservation{{
			Kind:      "priority_semantics",
			Subject:   "HarmonyOS priority semantics",
			Summary:   "Harmony priority semantics: prio=53/ohos_rt.",
			LineStart: 1,
			LineEnd:   2,
		}},
	}
	mut := types.NewMutableState("trace query window stats")
	mut.SetPerfTrace(perf)
	mut.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason:    types.EvidenceFloorWaiverNoRepoIntersection,
		Rationale: "attached trace is an external runtime artifact",
	})
	mut.RetainEvidenceFloorWaiver(true)
	mut.SetInvestigationComplete("trace_query finished; earlier runtime closure says prio=53 CFS")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedClosureReason: "trace_query finished; earlier runtime closure says prio=53 CFS",
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:  "trace_query",
			Role:      types.AnswerAggregateRolePrincipalAnswer,
			Subject:   "app-20",
			Predicate: "state_churn",
			Summary:   "dominant_state=runnable; prio=53/ohos_rt",
		}},
	})
	ctx := &types.AgentContext{
		Objective: "use trace_query window_stats to analyse app-20",
		Mutable:   mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:  types.ScenarioPerformanceBottleneck,
				Intent:    types.IntentRootCause,
				PerfTrace: perf,
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					Confidence:        0.9,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"model-authored closure reason omitted from this authority section",
		"trace_query",
		"dominant_state=runnable",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"Accepted runtime closure reason (advisory only",
		"prio=53 CFS",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt should omit deterministic-query-shadowed runtime closure narrative %q:\n%s", forbidden, prompt)
		}
	}
}

func TestAnswerDocumentFallbackEvidenceRows_RuntimeObservationOnlySkipsCurrentRepoRows(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{
			Func: "external_frame",
		}}}},
	})
	mut.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason:    types.EvidenceFloorWaiverNoRepoIntersection,
		Rationale: "artifact frames are from a different deployed build",
	})
	mut.RetainEvidenceFloorWaiver(true)
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:  types.ScenarioRootCause,
				Intent:    types.IntentRootCause,
				LogTriage: mut.LogTriage(),
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/helper.go",
			LineStart:       12,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "helper",
			Summary:         "repo helper that should not become fallback evidence for an external-only artifact",
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	rows := answerDocumentFallbackEvidenceRows(ctx, 8, 200)
	if len(rows) == 0 {
		t.Fatal("runtime observation-only fallback should preserve accepted runtime facts")
	}
	for _, row := range rows {
		if strings.Contains(row.loc, "internal/agent/helper.go") || strings.Contains(row.text, "repo helper") {
			t.Fatalf("runtime observation-only fallback must not surface current-repo helper rows: %+v", rows)
		}
	}
}

func TestAnswerDocumentEvaluator_MixedRuntimeCurrentSourceDoesNotRenderObservationOnly(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "timeout"}},
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentExplain,
				Scenario:  types.ScenarioArchitectureExplain,
				LogTriage: mut.LogTriage(),
				AnalyzerHints: types.AnalyzerHints{
					RequiredFileHints: []types.RequiredFileHint{{
						Path:       "internal/llm/openai.go",
						Confidence: 0.9,
						Rationale:  "current-source mechanism requested by the user",
					}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "This dispatch is observation-only") {
		t.Fatalf("mixed runtime+current-source prompt must not claim observation-only:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Current repository citations may still be used") {
		t.Fatalf("mixed runtime+current-source prompt missing current citation allowance:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_CurrentSourceExplanationProfileSoftAuthorityRendersCaveatGuidance(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "timeout"}},
	})
	ctx := &types.AgentContext{
		Language: "zh",
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentExplain,
				Scenario:  types.ScenarioArchitectureExplain,
				Language:  "zh",
				LogTriage: mut.LogTriage(),
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: true,
					Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
					SourceQuotes:                        []string{"当前源码解释"},
					TargetTerms:                         []string{"调度链路"},
					Confidence:                          0.9,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## 当前源码解释请求",
		"soft/caveat-only",
		"不要为了这个 soft lane 扩宽答案",
		"`explain_current_mechanism`",
		"当前源码解释",
		"调度链路",
		"精确符号、函数、配置键、错误类型或字面量锚点",
		"无法追到完整当前源码调用链",
		"多条源码事实不会自动组成一条顺序流水线",
		"alternative/dispatch 分支",
		"wrapper/public entry 只证明委托",
		"只有引用 gutter 中实际可见的操作才能进入机制结论",
		"source_shape_authority=definition_site_only",
		"aggregate fact、completion reason 和 evidence summary 是模型整理层",
		"不能提高源码权限",
		"Current repository citations may still be used",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "请同时使用外部观察 lane 和 current-source lane") {
		t.Fatalf("soft profile-backed authority must not render strong both-lane wording:\n%s", prompt)
	}
	for _, forbidden := range []string{
		"This dispatch is observation-only",
		"This dispatch is runtime-artifact scoped: current checkout/source evidence is not required",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("profile-backed mixed runtime+current-source prompt must not claim observation-only %q:\n%s", forbidden, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_CurrentSourceExplanationProfilePreciseAuthorityRendersBothLaneGuidance(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "timeout"}},
	})
	ctx := &types.AgentContext{
		Language: "zh",
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentExplain,
				Scenario:  types.ScenarioArchitectureExplain,
				Language:  "zh",
				LogTriage: mut.LogTriage(),
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: true,
					Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
					SourceQuotes:                        []string{"internal/tracequery/parse.go:42"},
					TargetTerms:                         []string{"span parser"},
					Confidence:                          0.9,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## 当前源码解释请求",
		"current-source 义务是精确的",
		"请同时使用外部观察 lane 和 current-source lane",
		"internal/tracequery/parse.go:42",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "soft/caveat-only") {
		t.Fatalf("precise profile-backed authority must not render soft caveat wording:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_CurrentSourceExplanationProfileCombinedProofRendersBothLaneGuidance(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "ev-current-trace-parser",
		Kind:            types.EvidenceDirect,
		Subject:         "current parser mechanism",
		Source:          "internal/tracequery/parse.go",
		LineStart:       42,
		LineEnd:         45,
		Scope:           types.ScopeLineRange,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "parseSpan",
		GroundingStatus: types.GroundingGrounded,
		Summary:         "current parser mechanism is grounded in source",
	}})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName:     "trace_query",
		Success:      true,
		Observations: []types.ObservationRecord{answerDocRuntimeAuthorityRuntimeRecord()},
	})
	ctx := &types.AgentContext{
		Language:      "en",
		Mutable:       mut,
		TurnRouteHint: types.TurnRouteHint{Source: "mixed", NeedsRepoAccess: true},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Language: "en",
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: true,
					Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
					SourceQuotes:                        []string{"current parser mechanism"},
					Confidence:                          0.9,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Current-Source Explanation Request",
		"current-source evidence has already landed",
		"Use both the external observation lane and the current-source lane",
		"current_source_satisfied=true",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "soft/caveat-only") {
		t.Fatalf("combined-proof authority must not render soft caveat wording:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_RuntimeObservationOnlySuppressesRepoEnrichment(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{
			Func: "load_config",
		}}}},
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: mut.LogTriage(),
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
				},
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{{
			ID:           "repo-helper",
			Source:       "internal/env/cache/disk_cache.go",
			LineStart:    179,
			Kind:         types.EvidenceConcrete,
			Subject:      "Cache.load",
			AnchorKind:   types.AnchorReturn,
			AnchorSymbol: "Cache.load",
			Origin:       types.ClaimOriginCurrentRepo,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, banned := range []string{
		"lane=value_fact label=`Cache.load`",
		"internal/env/cache/disk_cache.go:179",
		"`current_code_path\"`. (evidence: 1)",
	} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("observation-only runtime prompt leaked repo enrichment %q:\n%s", banned, prompt)
		}
	}
	for _, want := range []string{
		"This dispatch is runtime-artifact scoped",
		"Do not emit `current_status_verdict`",
		"trace-observed cause/risk",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing runtime-artifact boundary %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_RuntimeCausalDiagnosisUsesPostOverrideFacetAndLayeredCarrier(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{Errors: []types.LogError{{Type: "trace observation"}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:                 types.IntentRootCause,
			Scenario:               types.ScenarioRootCause,
			LogTriage:              mut.LogTriage(),
			DiagnosticProfile:      types.DiagnosticIntentProfile{IsDiagnostic: true},
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"trace only"},
				Confidence:        1,
			},
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"**section/ordered_list/table/bullet_list** (1+)",
		"Present the principal runtime diagnosis in the clearest grounded carrier",
		"Allowed block kinds: summary, section, ordered_list, bullet_list, table, caveat",
		"Build the principal answer blocks only from the lanes below",
		"exact allowed forms: [\"external_observation\"]",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("runtime causal JSON teaching missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"**Must declare (emit-time rejection if any are missing from every block's `facet_ids` and `claim_uses[].facet_id`):** `\"current_code_path\"`",
		"**HARD**: Current code path",
		"Emit the principal `ordered_list` block with `items[]` of ordered logical hops",
		"claim_uses=[{claim_form=definition_fact}]",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("runtime-only causal JSON teaching retained contradictory source/hop obligation %q:\n%s", forbidden, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_LogSourceDriftHonorsRuntimeDisposition(t *testing.T) {
	plan := &types.AnswerSurfacePlan{
		RuntimeGroundingDisposition: &types.RuntimeGroundingDisposition{
			Source:         types.RuntimeGroundingModelDeclared,
			Reason:         types.EvidenceFloorWaiverNoRepoIntersection,
			Rationale:      "different deployed build",
			CitationPolicy: types.RuntimeGroundingCitationRuntimeObservation,
		},
		LogSourceDriftAnchors: []types.LogSourceDriftAnchor{{
			File:         "a.go",
			ObservedLine: 20,
			AnchoredLine: 40,
			Func:         "Run",
		}},
	}
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Scenario: types.ScenarioRootCause, Intent: types.IntentRootCause},
			AnswerContract: types.AnswerContract{},
		},
	}
	ctx.Mutable.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason:    plan.RuntimeGroundingDisposition.Reason,
		Rationale: plan.RuntimeGroundingDisposition.Rationale,
	})
	ctx.Mutable.RetainEvidenceFloorWaiver(true)
	ctx.EvidenceItems = []types.EvidenceItem{{
		Source:          "a.go",
		LineStart:       40,
		AnchorSymbol:    "Run",
		GroundingStatus: types.GroundingGrounded,
		Authority:       types.AuthorityConditional,
		Origin:          types.ClaimOriginLog,
		DriftReason:     types.DriftReasonLineDrift,
	}}
	ctx.LogTriage = &types.LogBundle{Errors: []types.LogError{{Frames: []types.LogFrame{{File: "a.go", Line: 20, Func: "Run", Raw: "a.go:20"}}}}}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "observation-first for citation purposes") {
		t.Fatalf("drift section must honor runtime disposition:\n%s", prompt)
	}
	if strings.Contains(prompt, "Treat the current repo as the authoritative explanation surface") {
		t.Fatalf("drift section must not contradict runtime disposition:\n%s", prompt)
	}
}

// TestAnswerDocumentEvaluator_LanguageCapture keeps the hard language
// priority model pinned: project/CLI config wins, analyzer-emitted
// language is only a fallback when the agent context did not carry a
// concrete configured language.
func TestAnswerDocumentEvaluator_LanguageCapture(t *testing.T) {
	ctx := &types.AgentContext{Language: "zh"}
	e := &answerDocumentEvaluator{}
	e.BuildInitialInstruction(ctx, nil)
	if e.language != "zh" {
		t.Errorf("language = %q, want zh", e.language)
	}

	ctx2 := &types.AgentContext{Language: "en"}
	e2 := &answerDocumentEvaluator{}
	e2.BuildInitialInstruction(ctx2, nil)
	if e2.language != "en" {
		t.Errorf("language = %q, want en", e2.language)
	}

	ctx3 := &types.AgentContext{} // no language set
	e3 := &answerDocumentEvaluator{}
	e3.BuildInitialInstruction(ctx3, nil)
	if e3.language != "en" {
		t.Errorf("default language = %q, want en", e3.language)
	}

	ctx4 := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{Language: "zh"},
			RequestModel:   types.RequestModel{Language: "zh"},
		},
	}
	e4 := &answerDocumentEvaluator{}
	prompt4 := e4.BuildInitialInstruction(ctx4, nil)
	if e4.language != "zh" {
		t.Errorf("analysis fallback language = %q, want zh", e4.language)
	}
	if !strings.Contains(prompt4, "structured analyzer language") ||
		!strings.Contains(prompt4, "Simplified Chinese") {
		t.Errorf("analysis fallback language contract missing:\n%s", prompt4)
	}

	ctx5 := &types.AgentContext{
		Language: "en",
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{Language: "zh"},
			RequestModel:   types.RequestModel{Language: "zh"},
		},
	}
	e5 := &answerDocumentEvaluator{}
	prompt5 := e5.BuildInitialInstruction(ctx5, nil)
	if e5.language != "en" {
		t.Errorf("configured language should win: got %q, want en", e5.language)
	}
	if !strings.Contains(prompt5, "Project configuration locks the answer language to English") {
		t.Errorf("configured language contract missing:\n%s", prompt5)
	}

	ctx6 := &types.AgentContext{
		Language: "off",
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{Language: "zh"},
			RequestModel:   types.RequestModel{Language: "zh"},
		},
	}
	e6 := &answerDocumentEvaluator{}
	prompt6 := e6.BuildInitialInstruction(ctx6, nil)
	if e6.language != "en" {
		t.Errorf("disabled language should fall back renderer to en: got %q", e6.language)
	}
	if strings.Contains(prompt6, "## Response Language") {
		t.Errorf("lang=off must not inject response-language contract:\n%s", prompt6)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstructionResetsDispatchState(t *testing.T) {
	staleMu := types.NewMutableState("")
	freshMu := types.NewMutableState("")
	e := &answerDocumentEvaluator{
		mu:                     staleMu,
		retriesUsed:            2,
		proseFallbackRequested: true,
		rejectHintsUsed:        3,
		emitFullDocFailStreak:  4,
		emitPatchNudgeFired:    true,
		forceFullEmitNext:      true,
		preferPatchNext:        true,
		diagramRequired:        true,
		diagramMinimum:         1,
		diagramKinds:           []types.DiagramKind{types.DiagramSequence},
		configTraceDiagram:     true,
	}
	e.BuildInitialInstruction(&types.AgentContext{
		Mutable: freshMu,
	}, nil)
	if e.mu != freshMu {
		t.Fatalf("mutable state should be rebound per dispatch")
	}
	if e.retriesUsed != 0 || e.proseFallbackRequested || e.rejectHintsUsed != 0 ||
		e.emitFullDocFailStreak != 0 || e.emitPatchNudgeFired || e.forceFullEmitNext || e.preferPatchNext {
		t.Fatalf("dispatch counters/latches not reset: retries=%d prose=%t reject=%d fullStreak=%d patchNudge=%t forceFull=%t preferPatch=%t",
			e.retriesUsed, e.proseFallbackRequested, e.rejectHintsUsed, e.emitFullDocFailStreak, e.emitPatchNudgeFired, e.forceFullEmitNext, e.preferPatchNext)
	}
	if e.diagramRequired || e.diagramMinimum != 0 || len(e.diagramKinds) != 0 || e.configTraceDiagram {
		t.Fatalf("dispatch presentation state not reset: required=%t min=%d kinds=%v configTrace=%t",
			e.diagramRequired, e.diagramMinimum, e.diagramKinds, e.configTraceDiagram)
	}
}

// softStopObs builds a minimal PhaseSoftStop LoopObservation for the
// Observe tests — all the answer-document evaluator cares about is
// Phase; the rest of the fields can stay zero.
func softStopObs(continuationCount int) LoopObservation {
	return LoopObservation{
		Phase:             PhaseSoftStop,
		Iteration:         0,
		ContinuationsUsed: continuationCount,
	}
}

// TestAnswerDocumentEvaluator_Observe_EmptyNoToolFallsBackToIsolatedProse
// pins the single-model failure mode from noanswer.log: after one
// structured-tool nudge, another empty required-tool response should switch
// to a strictly isolated no-tool prose pass instead of burning the full
// finalizer correction budget on the same prompt.
func TestAnswerDocumentEvaluator_Observe_EmptyNoToolFallsBackToIsolatedProse(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: types.DefaultAgentSettings().FinalizerMaxCorrectionRetries}
	first := e.Observe(nil, softStopObs(0))
	if !first.HintRequested || first.DisableToolsNextTurn || first.IsolateNextPrompt {
		t.Fatalf("first empty soft-stop should request the structured tool once, got %+v", first)
	}
	second := e.Observe(nil, softStopObs(1))
	if !second.HintRequested || !second.DisableToolsNextTurn || !second.IsolateNextPrompt {
		t.Fatalf("second empty soft-stop should switch to isolated no-tool prose fallback, got %+v", second)
	}
	if second.HintKey != "answer_doc.prose_fallback" {
		t.Fatalf("isolated fallback hint key = %q", second.HintKey)
	}
	third := e.Observe(nil, softStopObs(2))
	if !third.StopRequested || third.HintRequested {
		t.Fatalf("empty isolated fallback result should stop and let ParseOutput show evidence fallback, got %+v", third)
	}
}

func TestAnswerDocumentEvaluator_Observe_AcceptsVisibleProseAfterOneStructuredNudge(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: types.DefaultAgentSettings().FinalizerMaxCorrectionRetries}
	first := e.Observe(nil, LoopObservation{
		Phase:    PhaseSoftStop,
		Response: llm.Response{Content: "rich prose answer"},
	})
	if !first.HintRequested {
		t.Fatalf("first visible no-tool answer should still get one structured emit nudge, got %+v", first)
	}
	second := e.Observe(nil, LoopObservation{
		Phase:    PhaseSoftStop,
		Response: llm.Response{Content: "rich prose answer after one nudge"},
	})
	if !second.StopRequested || second.HintRequested {
		t.Fatalf("second visible no-tool answer should be accepted as degraded prose fallback, got %+v", second)
	}
}

// TestAnswerDocumentEvaluator_Observe_RetriesWhenDocMissing is the
// complement: no doc in Mutable → Observe returns HintRequested.
func TestAnswerDocumentEvaluator_Observe_RetriesWhenDocMissing(t *testing.T) {
	mu := types.NewMutableState("") // empty Mutable
	e := &answerDocumentEvaluator{mu: mu, maxRetries: types.DefaultAgentSettings().FinalizerMaxCorrectionRetries}
	sig := e.Observe(nil, softStopObs(0))
	if !sig.HintRequested {
		t.Error("doc missing: HintRequested = false, want true")
	}
	if !sig.BypassThrottle {
		t.Fatalf("missing-document correction must bypass throttle so finalizer retries cannot be swallowed, got %+v", sig)
	}
	if !sig.BypassBudget {
		t.Fatalf("missing-document correction must bypass ordinary hint budget so finalizer delivery remains bounded by maxRetries, got %+v", sig)
	}
	if e.retriesUsed != 1 {
		t.Errorf("retriesUsed = %d, want 1", e.retriesUsed)
	}
}

func TestAnswerDocumentEvaluator_MissingDocumentHintDoesNotInventBlankDraft(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: types.DefaultAgentSettings().FinalizerMaxCorrectionRetries}
	sig := e.Observe(nil, LoopObservation{
		Phase:    PhaseSoftStop,
		Response: llm.Response{Content: "\n\n"},
	})
	if !sig.HintRequested {
		t.Fatalf("blank no-tool finalizer response should request correction")
	}
	if strings.Contains(sig.Hint, "You already drafted the answer") {
		t.Fatalf("blank response must not be described as an existing draft: %q", sig.Hint)
	}
	if !strings.Contains(sig.Hint, "did not contain a usable visible draft") {
		t.Fatalf("blank response hint should name the actual condition: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_MissingDocumentHintPreservesVisibleDraft(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: types.DefaultAgentSettings().FinalizerMaxCorrectionRetries}
	sig := e.Observe(nil, LoopObservation{
		Phase:    PhaseSoftStop,
		Response: llm.Response{Content: "Here is a rich draft answer."},
	})
	if !sig.HintRequested {
		t.Fatalf("visible no-tool finalizer draft should request structured emit")
	}
	if !strings.Contains(sig.Hint, "You already drafted the answer") {
		t.Fatalf("visible draft should still get preservation guidance: %q", sig.Hint)
	}
	if strings.Contains(sig.Hint, "did not contain a usable visible draft") {
		t.Fatalf("visible draft should not use blank-response guidance: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopSummaryCapRejectRequestsTargetedHint(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "typed summary cap reject",
			Repair: &types.ToolRepair{
				Code: "summary_cap",
				Metadata: map[string]string{
					"summary_length": "2782",
					"summary_cap":    "2500",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("summary-cap reject should request a correction hint, got %+v", sig)
	}
	if !sig.BypassThrottle {
		t.Fatalf("summary-cap reject hint should bypass throttle, got %+v", sig)
	}
	if !sig.BypassBudget {
		t.Fatalf("summary-cap reject hint should bypass the ordinary mid-loop budget, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "2500") || !strings.Contains(sig.Hint, "2782") {
		t.Fatalf("targeted summary-cap hint missing cap detail: %q", sig.Hint)
	}
	if !strings.Contains(sig.Hint, "emit_answer_document") {
		t.Fatalf("targeted summary-cap hint must tell the model to re-emit the tool call: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopSummaryCapRejectPreservesRequiredDiagram(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2, diagramRequired: true}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "typed summary cap reject",
			Repair: &types.ToolRepair{
				Code: "summary_cap",
				Metadata: map[string]string{
					"summary_length": "2782",
					"summary_cap":    "2500",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("summary-cap reject should request a correction hint, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "Preserve the required grounded diagram") {
		t.Fatalf("diagram-required summary-cap hint must preserve the diagram: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopMissingDiagramRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "diagram required for this dispatch (preferred kinds: call_dag); summary must include at least 1 grounded triple-backtick diagram block. This obligation is independent of answer shape.",
			Repair: &types.ToolRepair{
				Code:   "answer_doc_pre_emit_contract",
				Fields: []string{"blocks[].kind=diagram"},
				Metadata: map[string]string{
					"expected_block_kinds": "diagram",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("missing-diagram reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"grounded `diagram` block",
		"emit_answer_document",
		"diagram.kind",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("missing-diagram hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopSummaryOnlyRejectDoesNotSelectSpecialLane(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:exact_resolution] diagram required for this dispatch; value.literal is not corroborated by citations[0]",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("summary-only reject should still get a generic correction hint, got %+v", sig)
	}
	for _, forbidden := range []string{
		"grounded `diagram` block",
		"exact_resolution{status,anchor?,context_mode}",
		"LITERAL-GROUNDING",
	} {
		if strings.Contains(sig.Hint, forbidden) {
			t.Fatalf("summary-only reject must not select special lane %q:\n%s", forbidden, sig.Hint)
		}
	}
	if !strings.Contains(sig.Hint, "Re-emit a complete `emit_answer_document` payload") {
		t.Fatalf("summary-only reject should remain generic diagnostic guidance: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_Observe_UnexpectedFinalizerToolBypassesThrottle(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 2,
		Response: llm.Response{ToolCalls: []llm.ToolCall{{
			Name: "read_file",
		}}},
	})
	if !sig.HintRequested {
		t.Fatalf("unexpected finalizer tool should request a protocol correction, got %+v", sig)
	}
	if !sig.BypassThrottle {
		t.Fatalf("unexpected finalizer tool correction must bypass throttle, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "read_file") || !strings.Contains(sig.Hint, "emit_answer_document") {
		t.Fatalf("unexpected tool hint missing tool/protocol detail: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopRejectHintIgnoresSummaryFieldActions(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary: "The answer document does not yet meet the structural contract for this question.\n\n" +
				"  1. Field: `citations[]`\n" +
				"     Action: preserve / emit a top-level citations[] pool with at least 9 entries\n" +
				"  2. Field: `blocks[].items[].label`\n" +
				"     Action: structured answer anchor label(s) must preserve: explorer\n",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("tool reject should request a correction hint, got %+v", sig)
	}
	for _, forbidden := range []string{
		"citations[] pool with at least 9 entries",
		"structured answer anchor label(s)",
		"`citations[]`: preserve / emit",
		"`blocks[].items[].label`: structured answer anchor",
	} {
		if strings.Contains(sig.Hint, forbidden) {
			t.Fatalf("summary-only reject hint must stay generic and not parse %q:\n%s", forbidden, sig.Hint)
		}
	}
	if !strings.Contains(sig.Hint, "Re-emit a complete `emit_answer_document` payload") {
		t.Fatalf("summary-only reject should still get generic repair guidance:\n%s", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopMissingDiagramRejectIncludesConfigTraceSeed(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, Summary: "code defaults", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleDefault},
			{Source: "codrax.yaml.example", LineStart: 20, Subject: "ExploreHeuristics", Summary: "yaml precedence comment", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleYAML},
			{Source: "internal/config/runtime.go", LineStart: 194, Summary: "runtime yaml binding", Kind: types.EvidenceDirect, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration", DiagramRole: types.EvidenceDiagramRoleRuntime},
		},
	}
	e := &answerDocumentEvaluator{maxRetries: 2, configTraceDiagram: true}
	sig := e.Observe(ctx, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "diagram required for this dispatch (preferred kinds: architecture, flow); summary must include at least 1 grounded triple-backtick diagram block(s). This obligation is independent of answer shape.",
			Repair: &types.ToolRepair{
				Code:   "answer_doc_pre_emit_contract",
				Fields: []string{"blocks[].kind=diagram"},
				Metadata: map[string]string{
					"expected_block_kinds": "diagram",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("missing-diagram reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		// Hint reframed alongside the seed-extension authorisation:
		// the precedence chain is a FLOOR, not a verbatim ceiling.
		// The grounded reference fence is Mermaid and keeps only the
		// role labels inside the fence; supporting file:line anchors
		// stay elsewhere in the prompt so retries do not copy them
		// into node labels.
		"the seeded grounded precedence chain is the FLOOR",
		"```mermaid",
		"flowchart",
		"runtime binding",
		"code default",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("missing-diagram config-trace hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestRenderRetryDiagramSeedFence_UsesLogSeedForCallDAG(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramCallDAG},
				},
			},
		},
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Frames: []types.LogFrame{
					{File: "internal/agent/analyzer.go", Line: 320, Func: "buildAnalysisIR"},
					{File: "internal/orchestrator/orchestrator.go", Line: 101, Func: "Run"},
				},
			}},
		},
	}
	got := renderRetryDiagramSeedFence(ctx)
	// Seed is now a ```mermaid``` flowchart (was ASCII art with
	// "innermost failure:" / "  ->" prose). Assert on the grounded
	// labels themselves — those survive the format change.
	for _, want := range []string{
		"```mermaid",
		"flowchart",
		"internal/agent/analyzer.go:320",
		"buildAnalysisIR",
		"internal/orchestrator/orchestrator.go:101",
		"Run",
		"-->",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("call_dag retry seed missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRetryDiagramSeedFence_UsesFlowFindingSeedForFlow(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		FlowFindings: []types.FlowFindingDigest{{
			Path: []string{"config.handlers.explorer", "NewExplorer", "Register"},
		}},
	}
	got := renderRetryDiagramSeedFence(ctx)
	for _, want := range []string{
		"```mermaid",
		"flowchart",
		"config.handlers.explorer",
		"NewExplorer",
		"Register",
		"-->",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("flow retry seed missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRetryDiagramSeedFenceForRepair_SequencePrefersSupportLaneOverFlowNoise(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentTrace,
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					RequiredKind:   types.DiagramSequence,
					PreferredKinds: []types.DiagramKind{types.DiagramSequence},
				},
			},
		},
		FlowFindings: []types.FlowFindingDigest{{
			Path: []string{
				`finding.Conditions, " AND ", "## Cross-References"`,
				`"Evidence Gaps" prompt scaffold`,
			},
		}},
		EvidenceItems: []types.EvidenceItem{
			{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "ClientRequest.Start", Object: "Coordinator.Dispatch", Source: "internal/a.go", LineStart: 10, GroundingStatus: types.GroundingGrounded},
			{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "Coordinator.Dispatch", Object: "WorkerRuntime.Run", Source: "internal/b.go", LineStart: 20, GroundingStatus: types.GroundingGrounded},
			{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "WorkerRuntime.Run", Object: "LeafWorker.Execute", Source: "internal/c.go", LineStart: 30, GroundingStatus: types.GroundingGrounded},
		},
	}

	got := renderRetryDiagramSeedFenceForRepair(ctx, &types.ToolRepair{})
	firstPass := renderRetryDiagramSeedFenceForRepair(ctx, nil)
	for _, want := range []string{
		"```mermaid",
		"sequenceDiagram",
		"ClientRequest.Start",
		"Coordinator.Dispatch",
		"WorkerRuntime.Run",
		"LeafWorker.Execute",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sequence retry seed missing %q:\n%s", want, got)
		}
		if !strings.Contains(firstPass, want) {
			t.Fatalf("first-pass sequence seed missing %q:\n%s", want, firstPass)
		}
	}
	if strings.Contains(got, "Evidence Gaps") || strings.Contains(got, "Cross-References") {
		t.Fatalf("sequence retry seed should not let generic flow-finding prose shadow support-lane nodes:\n%s", got)
	}
	if strings.Contains(firstPass, "Evidence Gaps") || strings.Contains(firstPass, "Cross-References") {
		t.Fatalf("first-pass sequence seed should not let generic flow-finding prose shadow support-lane nodes:\n%s", firstPass)
	}
}

func TestRenderRetryDiagramSeedFenceForRepair_SequenceFallbackKeepsSequenceSyntax(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Scenario: types.ScenarioArchitectureExplain},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					RequiredKind:   types.DiagramSequence,
					PreferredKinds: []types.DiagramKind{types.DiagramSequence},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{{
			Source:          "mm/page_alloc.c",
			LineStart:       3943,
			GroundingStatus: types.GroundingGrounded,
			AnchorKind:      types.AnchorCall,
			Subject:         "get_page_from_freelist",
			Object:          "rmqueue",
		}},
	}

	got := renderRetryDiagramSeedFenceForRepair(ctx, nil)
	if !strings.Contains(got, "sequenceDiagram") {
		t.Fatalf("explicit sequence retry seed must keep sequence syntax:\n%s", got)
	}
	if strings.Contains(got, "flowchart TD") {
		t.Fatalf("explicit sequence retry seed must not hand finalizer a flowchart:\n%s", got)
	}
}

func TestRenderRetryDiagramSeedFence_UsesEvidenceSeedForArchitecture(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramArchitecture},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Alpha", Source: "internal/a.go", LineStart: 10, GroundingStatus: types.GroundingGrounded},
			{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Beta", Source: "internal/b.go", LineStart: 20, GroundingStatus: types.GroundingGrounded},
		},
	}
	got := renderRetryDiagramSeedFence(ctx)
	for _, want := range []string{
		"```mermaid",
		"flowchart",
		"Alpha",
		"Beta",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("architecture retry seed missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "-->") {
		t.Fatalf("definition membership does not prove an architecture edge:\n%s", got)
	}
}

func TestRenderRetryDiagramSeedFence_FlowDefinitionsDoNotMintEdges(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Scenario: types.ScenarioArchitectureExplain},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				Required:       true,
				RequiredKind:   types.DiagramFlow,
				PreferredKinds: []types.DiagramKind{types.DiagramFlow},
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "StageAnalyze", Source: "internal/types/enums.go", LineStart: 33, GroundingStatus: types.GroundingGrounded},
			{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "StageFinalize", Source: "internal/types/enums.go", LineStart: 36, GroundingStatus: types.GroundingGrounded},
			{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "MutableState.WriteExplorationRequest", Source: "internal/types/context.go", LineStart: 1781, GroundingStatus: types.GroundingGrounded},
		},
	}

	got := renderRetryDiagramSeedFence(ctx)
	for _, want := range []string{"flowchart TD", "StageAnalyze", "StageFinalize", "MutableState.WriteExplorationRequest"} {
		if !strings.Contains(got, want) {
			t.Fatalf("node-only flow seed missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "-->") {
		t.Fatalf("unordered definition/support nodes must not be linearly connected:\n%s", got)
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Node membership alone does not prove adjacency, order, containment, or direction",
		"an unconnected node set is intentional",
		"never connect nodes merely because they are listed next to each other or appear in collection order",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("finalizer prompt missing relation-authority boundary %q", want)
		}
	}
}

func TestRenderRetryDiagramSeedFenceForRepair_ConfigTraceRejectKeepsValidatedPrecedenceSeed(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, DiagramRole: types.EvidenceDiagramRoleDefault, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics"},
			{Source: "internal/config/runtime.go", LineStart: 231, DiagramRole: types.EvidenceDiagramRoleRuntime, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration"},
		},
		AnswerChains: []types.AnswerChain{
			{Item: types.EvidenceItem{Source: "cmd/root.go", LineStart: 2036, GroundingStatus: types.GroundingGrounded}},
			{Item: types.EvidenceItem{Source: "internal/analysis/declarative/classifier.go", LineStart: 66, GroundingStatus: types.GroundingGrounded}},
		},
	}
	repair := &types.ToolRepair{
		Code: "config_trace_context_citation",
		Metadata: map[string]string{
			"allowed_citations": "internal/config/runtime.go:231, internal/types/config.go:707",
		},
	}
	got := renderRetryDiagramSeedFenceForRepair(ctx, repair)
	for _, want := range []string{
		"runtime binding",
		"code default",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("config-trace repair seed missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{
		"internal/config/runtime.go:231",
		"internal/types/config.go:707",
	} {
		if strings.Contains(got, banned) {
			t.Fatalf("config-trace repair seed should keep support locations outside the fence, but found %q:\n%s", banned, got)
		}
	}
	for _, banned := range []string{
		"cmd/root.go:2036",
		"internal/analysis/declarative/classifier.go:66",
	} {
		if strings.Contains(got, banned) {
			t.Fatalf("config-trace repair seed should not fall back to unrelated answer chain %q:\n%s", banned, got)
		}
	}
}

func TestRenderRetryDiagramSeedFenceForRepair_ConfigTraceRejectOmitsUnrelatedFallbackWhenNoValidatedPrecedenceSeed(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/config/runtime.go", LineStart: 231, DiagramRole: types.EvidenceDiagramRoleRuntime, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
		},
		AnswerChains: []types.AnswerChain{
			{Item: types.EvidenceItem{Source: "cmd/root.go", LineStart: 2036, GroundingStatus: types.GroundingGrounded}},
			{Item: types.EvidenceItem{Source: "internal/analysis/declarative/classifier.go", LineStart: 66, GroundingStatus: types.GroundingGrounded}},
		},
	}
	repair := &types.ToolRepair{
		Code: "config_trace_context_citation",
		Metadata: map[string]string{
			"allowed_citations": "internal/config/runtime.go:231",
		},
	}
	if got := renderRetryDiagramSeedFenceForRepair(ctx, repair); got != "" {
		t.Fatalf("expected no repair seed when native precedence chain is incomplete, got:\n%s", got)
	}
}

func TestRenderRetryDiagramSeedFenceForRepair_FallsBackToAllowedCitationsWhenScenarioSeedsDoNotMatch(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramCallDAG},
				},
			},
		},
		FlowFindings: []types.FlowFindingDigest{
			{Path: []string{"internal/agent/agent.go:1786", "internal/agent/agent.go:1814"}},
		},
	}
	repair := &types.ToolRepair{
		Code: "diagram_codename",
		Metadata: map[string]string{
			"allowed_citations": "internal/agent/analyzer.go:651, internal/agent/analyzer.go:861",
		},
	}
	got := renderRetryDiagramSeedFenceForRepair(ctx, repair)
	for _, want := range []string{
		"internal/agent/analyzer.go:651",
		"internal/agent/analyzer.go:861",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("repair seed should fall back to current allowed citations %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "internal/agent/agent.go:1786") {
		t.Fatalf("repair seed should not reuse unrelated scenario seed when it falls outside the allowed citation set:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopDiagramGroundingRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "summary fenced code block references file(s) not present in citations[] or attached-log frames: codrax.yaml. ASCII diagrams are structural claims.",
			Repair: &types.ToolRepair{
				Code: "diagram_grounding",
				Hint: "Re-emit `emit_answer_document` with the same grounded answer, but inside fenced diagrams keep file/path node labels to the exact grounded allowlist for this dispatch. If a node has no grounded label, remove it from the fence and explain that relationship in prose instead.",
				Metadata: map[string]string{
					"allowed_labels": "cmd/root.go, internal/config/runtime.go, internal/types/config.go",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("diagram-grounding reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"DIAGRAM-GROUNDING",
		"reuse the exact grounded file / symbol / path labels",
		"Diagram Node Allowlist",
		"`cmd/root.go`, `internal/config/runtime.go`, `internal/types/config.go`",
		"Do NOT normalize one grounded label into a different spelling",
		"Keep `citations[]` byte-identical while repairing this gate",
		"Do NOT call `read_file`, `grep`, or any other tool",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("diagram-grounding hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopConfigTraceContextCitationRejectSurfacesAllowedAndForbiddenAnchors(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2, configTraceDiagram: true}
	sig := e.Observe(&types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Scenario: types.ScenarioConfigTrace},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/config/runtime.go", LineStart: 231, DiagramRole: types.EvidenceDiagramRoleRuntime, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
			{Source: "internal/types/config.go", LineStart: 707, DiagramRole: types.EvidenceDiagramRoleDefault, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
		},
	}, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:config_trace_context_citation] exact-absent config-trace answers may cite only precedence-capable lineage anchors.",
			Repair: &types.ToolRepair{
				Code: "config_trace_context_citation",
				Hint: "Re-emit `emit_answer_document` with the same exact-absence conclusion, but if `summary` continues to explain nearby precedence / lineage context, keep at least one grounded precedence anchor in `citations[]`.",
				Metadata: map[string]string{
					"allowed_citations": "internal/config/runtime.go:231, internal/types/config.go:707",
					"allowed_anchors":   "DefaultExploreHeuristics, internal/config/runtime.go",
					"forbidden_anchors": "ExploreBudget",
					"drop_citations":    "internal/types/explore_budget.go:40",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("config-trace context-citation reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"Only these grounded file:line anchors may appear in `citations[]` or fenced diagrams",
		"`internal/config/runtime.go:231`, `internal/types/config.go:707`",
		"Visible nearby context may only use this validated anchor set",
		"`DefaultExploreHeuristics`, `internal/config/runtime.go`",
		"Being visible does NOT make every anchor citation-grade",
		"Drop any prose / diagram node whose only support comes from these background-only anchors",
		"`ExploreBudget`",
		"Drop these invalid citation(s) from `citations[]`",
		"`internal/types/explore_budget.go:40`",
		"Choose one valid repair path now",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("config-trace context-citation hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopConfigTraceContextCitationRejectPreservesProseOnlyAnchors(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2, configTraceDiagram: true}
	sig := e.Observe(&types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Scenario: types.ScenarioConfigTrace},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/config/runtime.go", LineStart: 231, DiagramRole: types.EvidenceDiagramRoleRuntime, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
			{Source: "internal/types/config.go", LineStart: 707, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
		},
	}, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:config_trace_context_citation] exact-absent config-trace answers may cite only precedence-capable lineage anchors.",
			Repair: &types.ToolRepair{
				Code: "config_trace_context_citation",
				Hint: "Re-emit `emit_answer_document` with the same exact-absence conclusion, but remove this anchor from `citations[]` and from any fenced diagram nodes. You may keep it in `summary` as prose-only grounded nearby context, but if the user-visible answer still explains precedence / lineage, cite at least one validated default/config/runtime/override anchor.",
				Metadata: map[string]string{
					"allowed_citations":            "internal/config/runtime.go:231",
					"allowed_anchors":              "DefaultExploreHeuristics, internal/config/runtime.go",
					"prose_only_anchors":           "DefaultExploreHeuristics",
					"drop_citations":               "internal/types/config.go:707",
					"nearby_context_citation_mode": "prose_only",
					"preferred_context_mode":       "grounded_context_only",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("config-trace prose-only citation reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"treat the nearby grounded context as prose-only for this dispatch",
		"Only these grounded file:line anchors may appear in `citations[]` or fenced diagrams",
		"`internal/config/runtime.go:231`",
		"Visible nearby context may only use this validated anchor set",
		"`DefaultExploreHeuristics`, `internal/config/runtime.go`",
		"Being visible does NOT make every anchor citation-grade",
		"`exact_resolution.context_mode=\"grounded_context_only\"`",
		"may stay on the user-visible answer surface as uncited prose-only grounded context",
		"`DefaultExploreHeuristics`",
		"Drop these invalid citation(s) from `citations[]`",
		"`internal/types/config.go:707`",
		"Choose one valid repair path now",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("config-trace prose-only context-citation hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopConfigTraceContextCitationRejectKeepsFollowOnContextVisible(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	mu.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Subject:         "DefaultExploreHeuristics",
		Predicate:       "defines",
		Object:          "ExploreHeuristics defaults",
		Source:          "internal/types/config.go",
		LineStart:       707,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
	}})
	e := &answerDocumentEvaluator{maxRetries: 2, configTraceDiagram: true}
	sig := e.Observe(&types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
	}, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:config_trace_context_citation] exact-absent config-trace answers may cite only precedence-capable lineage anchors.",
			Repair: &types.ToolRepair{
				Code: "config_trace_context_citation",
				Hint: "Re-emit `emit_answer_document` with the same exact-absence conclusion, but remove this anchor from `citations[]`.",
				Metadata: map[string]string{
					"allowed_anchors":              "DefaultExploreHeuristics",
					"drop_citations":               "internal/types/config.go:707",
					"nearby_context_citation_mode": "prose_only",
					"preferred_context_mode":       "grounded_context_only",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("follow-on grounded-context citation reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"follow-on grounded-context mode",
		"Keep the nearby grounded context visible as uncited prose-only explanation instead of collapsing to the exact-absence lead alone",
		"Keep the nearby grounded context visible as uncited prose-only explanation",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("follow-on grounded-context hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopFollowOnGroundedContextRejectUsesAllowedAnchors(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:follow_on_grounded_context] exact-absent answers in follow-on grounded-context mode collapsed to the lead only.",
			Repair: &types.ToolRepair{
				Code: "follow_on_grounded_context",
				Hint: "Re-emit `emit_answer_document` with the same exact-absence conclusion, but keep the grounded nearby context visible after the renderer-generated lead.",
				Metadata: map[string]string{
					"allowed_anchors":        "DefaultExploreHeuristics, ResolvedExploreHeuristics",
					"preferred_context_mode": "grounded_context_only",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("follow-on grounded-context reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"`DefaultExploreHeuristics`, `ResolvedExploreHeuristics`",
		"`exact_resolution.context_mode=\"grounded_context_only\"`",
		"Do not collapse the answer to the renderer-generated exact-absence lead alone",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("follow-on grounded-context hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopDiagramCodenameRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2, configTraceDiagram: true}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "summary introduces codename label(s) not present in any citation's ±3-line window: Level 1, Level 2.",
			Repair: &types.ToolRepair{
				Code: "diagram_codename",
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("diagram-codename reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"CODENAME-GROUNDING",
		"Level 1",
		"Label the diagram directly with grounded files, functions, config keys",
		"Do NOT call `read_file`, `grep`, or any other tool",
		"defaults / config-file load / runtime binding / operator override",
		"move that explanation into prose outside the fenced diagram",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("diagram-codename hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopExactResolutionRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:exact_resolution] exact-resolution contract violated: summary must explicitly name the requested exact config key and lead with its absence before any nearby context.",
			Repair: &types.ToolRepair{
				Code: "exact_resolution",
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("exact-resolution reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"absence-only is acceptable",
		"requested exact target",
		"related context",
		"equivalent, alias, or substitute",
		"Do NOT call `read_file`, `grep`, or any other tool",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("exact-resolution hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopUnexpectedReadToolRequestsSynthesisOnlyRetry(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 3,
		Response: llm.Response{
			ToolCalls: []llm.ToolCall{{ID: "1", Name: "read_file"}},
		},
		LastToolResult: &types.ToolResult{
			ToolName: "read_file",
			Success:  false,
			Summary:  "invalid params",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("unexpected read tool should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"pure answer synthesizer",
		"Do NOT call `read_file`",
		"emit_answer_document",
		"Diagram Node Allowlist",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("unexpected-tool hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopGenericRejectSurfacesToolError(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "symbols[0].file is required when symbols[0].line is set\nextra detail follows",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("generic reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"emit_answer_document",
		// No previous structured draft exists in this path, so the
		// repair must ask for a complete fresh payload rather than
		// an impossible byte-identical paste from a missing prior
		// emit. Separate patch-base tests cover the preserve wording.
		"complete `emit_answer_document` payload",
		"Build the full document from the already-provided evidence",
		"Do not write free-form prose outside the tool call",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("generic reject hint missing %q: %q", want, sig.Hint)
		}
	}
	if strings.Contains(sig.Hint, "symbols[0].file is required") || strings.Contains(sig.Hint, "extra detail follows") {
		t.Fatalf("summary-only generic reject must not surface rendered tool detail: %q", sig.Hint)
	}
}

// TestComputeAnswerDocAttemptShape locks the attempt-shape capture so
// the regression detector's input is well-defined for tests.
func TestComputeAnswerDocAttemptShape(t *testing.T) {
	// Cannot import emit_answer_document params type from here —
	// asserting via the public detector path is enough. The
	// regression detector is the load-bearing consumer.
	prior := &types.AnswerDocAttemptShape{
		CitationsCount: 18, StepsCount: 16, SymbolsCount: 0, SummaryRunes: 4250,
		HasValue: false, HasBoolean: false, HasExactResolution: true,
	}
	if !shapeIndicatesContent(prior) {
		t.Errorf("prior with substantial payload must register as content-bearing")
	}
}

func shapeIndicatesContent(s *types.AnswerDocAttemptShape) bool {
	if s == nil {
		return false
	}
	return s.CitationsCount+s.StepsCount+s.SymbolsCount+s.SummaryRunes >= 16
}

func TestAnswerDocumentEvaluator_Observe_MidLoopStructuredRepairHintUsesRepairMetadata(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "validation failed",
			Repair: &types.ToolRepair{
				Code:   "scalar_summary_required",
				Fields: []string{"summary", "value.literal"},
				Hint:   "Re-emit `emit_answer_document` with the same scalar payload, keep the grounded literal and citation unchanged, and expand `summary` so it names the measured subject and how the value was obtained. Do not reopen files or change the answer shape.",
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("structured repair should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"summary",
		"value.literal",
		"same scalar payload",
		"Do not reopen files or change the answer shape",
		"Do not write free-form prose outside the tool call",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("structured repair hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopCitationRefRepairUsesStructuredHint(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:citation_ref_range] steps[3].citation_ref 6 is out of range (citations pool has 6 entries)",
			Repair: &types.ToolRepair{
				Code:   "citation_ref_range",
				Fields: []string{"steps[3].citation_ref"},
				Hint:   "Re-emit `emit_answer_document` and fix ONLY `steps[3].citation_ref`: citation_ref is zero-based. The current citations[] pool has 6 entries, so valid indices are `0` through `5`, or `-1` for 'no citation'. Keep the existing grounded evidence and renumber only the offending citation_ref fields; do not reopen files.",
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("structured citation_ref repair should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"steps[3].citation_ref",
		"zero-based",
		"`0` through `5`",
		"Do not write free-form prose outside the tool call",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("citation_ref repair hint missing %q: %q", want, sig.Hint)
		}
	}
	for _, forbidden := range []string{"-1 for 'no citation'", "`-1` for 'no citation'", "citation_ref=-1", "citation_ref = -1"} {
		if strings.Contains(sig.Hint, forbidden) {
			t.Fatalf("citation_ref repair hint should not teach no-citation sentinel %q: %q", forbidden, sig.Hint)
		}
	}
	if !strings.Contains(sig.Hint, "omitting the field for no current-repo citation") {
		t.Fatalf("citation_ref repair hint should prefer omitted-field wording: %q", sig.Hint)
	}
}

// TestAnswerDocumentEvaluator_Observe_MidLoopLiteralGroundingRejectSurfacesAction
// pins the session-22 in-dispatch self-correction nudge: when the
// literal-grounding gate rejects a value-shape citation, the
// mid-loop reject hint must surface the single-action fix
// ("leave uncited + summary caveat") at the TOP of the hint so
// the LLM stops trying more fabrications and reaches for the
// no-current-repo-citation escape. Without this special-case, the generic "fix the exact
// validation error" hint buried the action behind diagnostic
// prose and the LLM burned the full retry budget on fresh
// fabrications before the dispatch exited (observed: 16 min on
// the partial eval case).
func TestAnswerDocumentEvaluator_Observe_MidLoopLiteralGroundingRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 3}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  `ShouldNotLeak value.literal "processRequest" is not corroborated by citations[0].`,
			Repair: &types.ToolRepair{
				Code:   "literal_grounding",
				Fields: []string{"value.citation_ref"},
				Hint:   "Your last `emit_answer_document` call was rejected by the LITERAL-GROUNDING gate: keep `value.citation_ref` left uncited when the literal comes from an external source, and explain that evidence boundary in summary instead of borrowing a nearby repo citation.",
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("literal-grounding reject should request a correction hint, got %+v", sig)
	}
	actionIdx := strings.Index(sig.Hint, "`value.citation_ref` left uncited")
	if actionIdx < 0 {
		t.Fatalf("uncited action must come from typed repair hint; hint:\n%s", sig.Hint)
	}
	if strings.Contains(sig.Hint, "citation_ref=-1") || strings.Contains(sig.Hint, "citation_ref = -1") {
		t.Fatalf("literal-grounding hint should not teach visible sentinel wording:\n%s", sig.Hint)
	}
	if strings.Contains(sig.Hint, "ShouldNotLeak") || strings.Contains(sig.Hint, "processRequest") {
		t.Fatalf("literal-grounding hint must not parse rendered Summary diagnostics:\n%s", sig.Hint)
	}
	if !strings.Contains(sig.Hint, "LITERAL-GROUNDING") {
		t.Errorf("hint should name the gate so operators can trace the signal: %q", sig.Hint)
	}
	if !strings.Contains(sig.Hint, "external source") {
		t.Errorf("hint should surface the 'external source' escape rationale: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopLiteralGroundingRejectSurfacesStepAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 3}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  `ShouldNotLeak steps[0].description "searched the repo and found no production definition" is not corroborated by citations[0].`,
			Repair: &types.ToolRepair{
				Code:   "literal_grounding",
				Fields: []string{"steps[0].citation_ref", "steps[0].description"},
				Hint:   "Your last `emit_answer_document` call was rejected by the LITERAL-GROUNDING gate: keep `steps[0].citation_ref` left uncited when `steps[0].description` summarizes a repo-wide search, aggregate absence, test-only proof, or other non-line-local conclusion. Do NOT try to borrow a nearby file:line just to satisfy the schema.",
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("step literal-grounding reject should request a correction hint, got %+v", sig)
	}
	actionIdx := strings.Index(sig.Hint, "`steps[0].citation_ref` left uncited")
	if actionIdx < 0 {
		t.Fatalf("steps[0] uncited action must come from typed repair hint; hint:\n%s", sig.Hint)
	}
	if strings.Contains(sig.Hint, "citation_ref=-1") || strings.Contains(sig.Hint, "citation_ref = -1") {
		t.Fatalf("step literal-grounding hint should not teach visible sentinel wording:\n%s", sig.Hint)
	}
	if strings.Contains(sig.Hint, "ShouldNotLeak") || strings.Contains(sig.Hint, "searched the repo") {
		t.Fatalf("step literal-grounding hint must not parse rendered Summary diagnostics:\n%s", sig.Hint)
	}
	for _, want := range []string{
		"LITERAL-GROUNDING",
		"`steps[0].description`",
		"repo-wide search, aggregate absence, test-only proof",
		"Do NOT try to borrow a nearby file:line",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("step literal-grounding hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopRejectStopsHintingAfterBudget(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 1}
	obs := LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "summary length 2782 exceeds cap 2500 for shape=explanation — shorten the summary",
		},
	}
	if sig := e.Observe(nil, obs); !sig.HintRequested {
		t.Fatalf("first reject should request a correction hint, got %+v", sig)
	}
	if sig := e.Observe(nil, obs); !sig.HintRequested {
		t.Fatalf("tool-level rejects should continue surfacing repair hints beyond the first correction budget round, got %+v", sig)
	}
	for i := 0; i < e.rejectHintBudget()-2; i++ {
		if sig := e.Observe(nil, obs); !sig.HintRequested {
			t.Fatalf("repair hint %d should still be available before the extended budget is exhausted, got %+v", i+3, sig)
		}
	}
	if sig := e.Observe(nil, obs); sig.HintRequested {
		t.Fatalf("after the extended reject-hint budget, evaluator should stay silent, got %+v", sig)
	}
}

func TestAnswerDocumentEvaluator_RejectSignal_DoesNotRequireSummaryText(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 1}
	obs := LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "",
		},
	}
	sig := e.emitAnswerDocumentRejectSignal(nil, obs)
	if !sig.HintRequested {
		t.Fatalf("failed emit_answer_document should request generic repair even with empty summary, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "Re-emit") || !strings.Contains(sig.Hint, "`emit_answer_document`") {
		t.Fatalf("generic repair hint missing emit instruction: %q", sig.Hint)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_Happy — a fully-populated
// AnswerDocument in Mutable is rendered into FinalAnswer.

// TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_FailLoud covers
// the fail-loud path: no document, retries exhausted, ParseOutput
// surfaces a warning banner prefixed to the raw content.
func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_FailLoud(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	messages := []llm.Message{
		{Role: "assistant", Content: "raw fallback text"},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "answer_document emission missing") {
		t.Errorf("fail-loud warning missing: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "raw fallback text") {
		t.Errorf("raw content lost: %q", out.FinalAnswer)
	}
	if !out.AnswerDegraded || !out.SkipAnswerChecks || out.DegradeReason != "answer_document_missing" {
		t.Fatalf("missing-doc raw fallback must be marked degraded and skip structured checks, got degraded=%t skip=%t reason=%q",
			out.AnswerDegraded, out.SkipAnswerChecks, out.DegradeReason)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_FailLoudZh(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	messages := []llm.Message{
		{Role: "assistant", Content: "原始兜底内容"},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "answer_document emission missing") {
		t.Errorf("zh fail-loud warning should be localized: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "未能生成结构化答案") {
		t.Errorf("zh fail-loud warning missing: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "原始兜底内容") {
		t.Errorf("raw content lost: %q", out.FinalAnswer)
	}
	if !out.AnswerDegraded || !out.SkipAnswerChecks || out.DegradeReason != "answer_document_missing" {
		t.Fatalf("missing-doc raw fallback must be marked degraded and skip structured checks, got degraded=%t skip=%t reason=%q",
			out.AnswerDegraded, out.SkipAnswerChecks, out.DegradeReason)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_RecoversAnswerDocumentJSONContent(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	messages := []llm.Message{{
		Role: "assistant",
		Content: `{
  "blocks": [
    {
      "id": "summary",
      "kind": "summary",
      "surface_role": "principal",
      "text": "## 系统架构概述\n\n可见答案应从 block text 渲染，而不是展示原始 JSON。"
    },
    {
      "id": "diagram",
      "kind": "diagram",
      "diagram": {
        "kind": "sequence",
        "language": "mermaid",
        "body": "sequenceDiagram\nUser->>System: ask"
      }
    }
  ],
  "citations": [
    {"file": "internal/agent/agent.go", "line": 859}
  ]
}`,
	}}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "answer_document emission missing") {
		t.Fatalf("recovered answer should not use raw-content missing-doc banner: %q", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, `"blocks"`) || strings.Contains(out.FinalAnswer, `"citations"`) {
		t.Fatalf("recovered answer leaked raw JSON: %q", out.FinalAnswer)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("lossless recovered answer document should be restored to Mutable for contract validation")
	}
	if len(doc.Blocks) != 2 || len(doc.Citations) != 1 {
		t.Fatalf("recovered Mutable document shape = blocks:%d citations:%d, want 2/1", len(doc.Blocks), len(doc.Citations))
	}
	for _, want := range []string{
		"系统架构概述",
		"可见答案应从 block text 渲染",
		"```mermaid",
		"sequenceDiagram",
		"internal/agent/agent.go:859",
		"已从模型写在文本中的 answer_document JSON 恢复本答案",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("recovered answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_RecoversPreservedNoToolDraftAfterIsolatedFallback(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	ctx.Mutable.AppendFinalizerNoToolAnswerDraft(`{
	  "blocks": [
	    {
	      "id": "summary",
	      "kind": "summary",
	      "surface_role": "principal",
	      "text": "首轮 JSON 草稿保留了 timeout 与成文校验失败的区别。"
	    },
	    {
	      "id": "details",
	      "kind": "ordered_list",
	      "surface_role": "principal",
	      "items": [
	        {"id": "i1", "label": "模型响应超时", "text": "由流级错误判定，允许重试。", "citation_ref": 0},
	        {"id": "i2", "label": "成文校验失败", "text": "属于内容/结构校验，不应按流级超时重试。", "citation_ref": 1}
	      ]
	    }
	  ],
	  "citations": [
	    {"file": "internal/llm/retryable_error.go", "line": 117},
	    {"file": "internal/orchestrator/orchestrator.go", "line": 5714}
	  ],
	  "exact_resolution": {"status": "resolved", "anchor": "internal/orchestrator/orchestrator.go:5714", "context_mode": "clear"}
	}`)
	messages := []llm.Message{
		{Role: "user", Content: "isolated fallback prompt"},
		{Role: "assistant", Content: "降级散文兜底很短，已经丢失关键区分。"},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "未能生成结构化答案") ||
		strings.Contains(out.FinalAnswer, "answer_document emission missing") {
		t.Fatalf("preserved draft recovery should avoid missing-doc fallback:\n%s", out.FinalAnswer)
	}
	for _, want := range []string{
		"timeout 与成文校验失败的区别",
		"模型响应超时",
		"成文校验失败",
		"internal/llm/retryable_error.go:117",
		"internal/orchestrator/orchestrator.go:5714",
		"已从模型写在文本中的 answer_document JSON 恢复本答案",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
	if strings.Contains(out.FinalAnswer, "降级散文兜底很短") {
		t.Fatalf("weaker isolated fallback should not override the preserved structured draft:\n%s", out.FinalAnswer)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Citations) != 2 {
		t.Fatalf("lossless preserved draft should be restored to Mutable, doc=%+v", doc)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_FallsBackToPriorModelSurfaceDraft(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
		PriorReports: []types.StageReport{{
			Stage: types.StageExtract,
			Agent: types.AgentExtractor,
			Findings: "Model-authored visible surface draft (advisory only):\n\n" +
				"| 子包 | 入口 |\n| --- | --- |\n| alpha | Run |\n| beta | Execute |",
		}},
	}
	messages := []llm.Message{
		{Role: "assistant", Content: "{{PLACEHOLDER_REASONING_PLACEHOLDER}}"},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{"未能生成结构化答案", "已保留的模型草稿", "| alpha | Run |", "| beta | Execute |"} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
	if strings.Contains(out.FinalAnswer, "PLACEHOLDER_REASONING_PLACEHOLDER") ||
		strings.Contains(out.FinalAnswer, "Model-authored visible surface draft") {
		t.Fatalf("fallback leaked placeholder or prompt preamble:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_FallsBackToPreservedFinalizerSurfaceDraft(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	ctx.Mutable.AppendFinalizerNoToolAnswerDraft("## 已完成的模型草稿\n\n" +
		"| 阶段 | 区分 |\n| --- | --- |\n| 模型响应超时 | 传输层事件 |\n| 成文校验失败 | 结构化内容事件 |\n\n" +
		"```mermaid\nflowchart TD\n    A[timeout] --> B[retry]\n```")
	messages := []llm.Message{
		{Role: "assistant", Content: "{{PLACEHOLDER_REASONING_PLACEHOLDER}}"},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"未能生成结构化答案",
		"已保留的模型草稿",
		"| 模型响应超时 | 传输层事件 |",
		"```mermaid",
		"flowchart TD",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
	if strings.Contains(out.FinalAnswer, "PLACEHOLDER_REASONING_PLACEHOLDER") {
		t.Fatalf("fallback leaked placeholder:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_DoesNotAppendVerifiedStageBindingSupplement(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	mu := types.NewMutableState("")
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:           "stage-binding",
		Kind:         types.EvidenceRelationship,
		Subject:      "StageBinding",
		Predicate:    "maps stage to agent",
		Object:       "AgentExplorer",
		Source:       "internal/types/stage_binding.go",
		LineStart:    18,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: "builtinStageBindings",
	}})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "模型成文只概述了 pipeline，会在最后一公里补充已核验的阶段绑定。",
		}},
		Citations: []types.Citation{{
			File: "internal/types/stage_binding.go",
			Line: 18,
		}},
	})
	ctx := &types.AgentContext{RepoRoot: repo, Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "模型成文只概述了 pipeline") {
		t.Fatalf("model-authored answer was not preserved:\n%s", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：阶段绑定核对") ||
		strings.Contains(out.FinalAnswer, "System supplement: verified stage bindings") {
		t.Fatalf("stage authority belongs in the Finalizer prompt, not a system-authored answer suffix:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_DoesNotDuplicateFullyCitedStageBindings(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "模型已经逐项解释并引用四个 read-mode 主 stage。",
		}},
		Citations: []types.Citation{
			{File: "internal/types/stage_binding.go", Line: 13},
			{File: "internal/types/stage_binding.go", Line: 14},
			{File: "internal/types/stage_binding.go", Line: 15},
			{File: "internal/types/stage_binding.go", Line: 16},
		},
	})
	ctx := &types.AgentContext{RepoRoot: repo, Mutable: mu}
	out, err := (&answerDocumentEvaluator{language: "zh"}).ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：阶段绑定核对") {
		t.Fatalf("fully cited canonical rows must not trigger a duplicate system answer:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_DoesNotAppendStageBindingForAmbientEvidenceOnly(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	mu := types.NewMutableState("")
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:           "stage-binding",
		Kind:         types.EvidenceRelationship,
		Subject:      "StageBinding",
		Predicate:    "maps stage to agent",
		Object:       "AgentExplorer",
		Source:       "internal/types/stage_binding.go",
		LineStart:    18,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: "builtinStageBindings",
	}})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "主答案没有请求 stage workflow 维度，也没有引用 stage_binding.go。",
		}},
	})
	ctx := &types.AgentContext{RepoRoot: repo, Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：阶段绑定核对") {
		t.Fatalf("ambient evidence alone must not append stage binding supplement:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_DoesNotAppendStageBindingWithoutGroundedSource(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "没有引用或落地 stage binding 源码时，不追加系统关系核对表。",
		}},
	})
	ctx := &types.AgentContext{RepoRoot: repo, Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：阶段绑定核对") {
		t.Fatalf("supplement should be gated by grounded/cited source:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_DoesNotAppendStageBindingForRequestedWorkflowDimension(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "模型成文已经解释了 read-mode pipeline，但压缩了中间阶段。",
		}},
		Citations: []types.Citation{{
			File: "internal/orchestrator/orchestrator.go",
			Line: 1536,
		}},
	})
	ctx := &types.AgentContext{
		RepoRoot: repo,
		Mutable:  mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Index:       1,
					Label:       "每个 stage 的输入、输出和主要状态载体",
					SourceQuote: "每个 stage 的输入、输出和主要状态载体",
					Required:    true,
					Role:        types.RequestedAnswerDimensionStageWorkflow,
				}},
				Confidence: 0.9,
			},
		}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "模型成文已经解释了 read-mode pipeline") {
		t.Fatalf("model-authored answer was not preserved:\n%s", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：阶段绑定核对") ||
		strings.Contains(out.FinalAnswer, "System supplement: verified stage bindings") {
		t.Fatalf("requested workflow authority must guide the model without becoming a system-authored answer suffix:\n%s", out.FinalAnswer)
	}
}

func TestRenderAnswerDocCurrentRunStageLaneAuthoritySeparatesReadAndWriteStages(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	ctx := &types.AgentContext{
		Mode:     types.ModeRead,
		RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Index: 1, Label: "stage workflow", SourceQuote: "stage workflow", Required: true,
					Role: types.RequestedAnswerDimensionStageWorkflow,
				}},
			},
		}},
		EvidenceItems: []types.EvidenceItem{{
			Source: "internal/types/stage_binding.go", LineStart: 1,
			GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition,
			AnchorSymbol: "ReadModeConditionalPreStageBindings",
		}},
	}
	got := renderAnswerDocCurrentRunStageLaneAuthority(ctx)
	for _, want := range []string{
		"## Current Run Stage-Lane Authority",
		"current_mode=`read`",
		"canonical_read_main_sequence=`analyze -> explore -> extract -> finalize`",
		"conditional_pre_stages=`log_triage, perf_triage`",
		"cross-mode/background evidence",
		"declaration namespace",
		"cannot widen the active membership",
		"### Verified stage responsibilities and artifact provenance",
		"language model authors only the request classification",
		"deterministic code then normalizes and compiles",
		"do not attribute deterministically derived artifacts directly to the language model",
		"principal authority for the selected read workflow's stage responsibilities",
		"merely reuses a word such as analyze, explore, extract, finalize",
		"Do not use a homonymous helper to redefine the stage row",
		"stage_binding[1]: stage=`StageAnalyze` (`analyze`)",
		"agent=`AgentAnalyzer` (`analyzer`)",
		"primary_artifacts=`AnalysisIR, TaskGraph, EvidencePlan, AnswerContract, HypothesisSet, QualityGate`",
		"stage_binding[4]: stage=`StageFinalize` (`finalize`)",
		"source=`internal/types/stage_binding.go:",
		"### Verified shared state-carrier fields",
		"field-ownership facts, not stage edges or per-stage read/write claims",
		"place the field/type node or subgroup inside the owner subgraph with no arrow and no edge_anchor",
		"retain its `unproven` participant boundary",
		"boundary limits direct-flow claims and does not erase the proved containment",
		"state_carrier[1]: owner=`BusContext`; field=`Mutable`; type=`*MutableState`; source=`internal/types/context.go:",
		"owner=`BusContext`; field=`AnalysisIR`; type=`*AnalysisIR`",
		"owner=`MutableState`; field=`answerDocumentV2`; type=`*AnswerDocumentV2`",
		"does not authorize the system to replace the model's conclusion or answer",
		"### User-facing workflow display guidance",
		"responsibility` above is the wording basis",
		"business action first and the exact stage identity second",
		"do not label an arrow merely `precedence`, `call`, or `data_flow`",
		"Business wording is display-only and cannot authorize a new edge",
		"### Verified stage-order edge recipes",
		"one complete, checkout-verified authoring recipe",
		"preserve that exact pair in `edge_anchors.from_identity/to_identity`, and set `relation_kind=precedence`",
		"stage_precedence[1]: from_stage=`StageAnalyze` (`analyze`); from_agent=`AgentAnalyzer` (`analyzer`); to_stage=`StageExplore` (`explore`); to_agent=`AgentExplorer` (`explorer`); relation_kind=`precedence`",
		"stage_precedence[2]: from_stage=`StageExplore` (`explore`); from_agent=`AgentExplorer` (`explorer`); to_stage=`StageExtract` (`extract`); to_agent=`AgentExtractor` (`extractor`); relation_kind=`precedence`",
		"stage_precedence[3]: from_stage=`StageExtract` (`extract`); from_agent=`AgentExtractor` (`extractor`); to_stage=`StageFinalize` (`finalize`); to_agent=`AgentFinalizer` (`finalizer`); relation_kind=`precedence`",
		"source=`internal/types/enums.go:",
		"They do not prove `call`, `data_flow`, artifact transfer, shared-state participant connectivity, or runtime causality",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stage-lane authority missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "write_analyze") || strings.Contains(got, "StageWriteAnalyze") {
		t.Fatalf("write-only stage must not enter canonical read membership:\n%s", got)
	}
	if count := strings.Count(got, "- stage_precedence["); count != 3 {
		t.Fatalf("read authority must publish exactly three adjacent precedence recipes, got %d:\n%s", count, got)
	}

	ctx.Mode = types.ModeApply
	if got := renderAnswerDocCurrentRunStageLaneAuthority(ctx); got != "" {
		t.Fatalf("read authority must stay inactive in apply mode:\n%s", got)
	}
}

func TestAnswerDocVerifiedStagePrecedenceUsesSameRequiredWorkflowAuthority(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("partial analyzer stage slate")
	mu.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: types.ReadModePipelineOrchestratorFile, LineStart: 1685,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Orchestrator.Run",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.AgentContext{
		RepoRoot: repoRoot, Mode: types.ModeRead, Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true,
				Participants: []types.DiagramParticipantHint{{Identity: "Orchestrator.Run", Role: types.DiagramParticipantIncidentRequired}}},
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Index: 1, Label: "stage", SourceQuote: "stage", Required: true,
					Role: types.RequestedAnswerDimensionStageWorkflow,
				}},
			},
		}},
	}
	if got := answerDocVerifiedReadModeStagePrecedenceForRequest(ctx); len(got) != 3 {
		t.Fatalf("finalizer recipe authority=%d, want 3 from shared typed workflow admission", len(got))
	}
}

func TestRenderAnswerDocCurrentRunStageLaneAuthorityUsesTypedEndpointSpan(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("typed stage endpoint span")
	mu.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: types.ReadModePipelineOrchestratorFile, LineStart: 1685,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "runReadSchedulerLoop",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.AgentContext{
		RepoRoot: repoRoot, Mode: types.ModeRead, Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "codrax", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "analyze", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mermaid", Role: types.DiagramParticipantIncidentRequired},
				}},
		}},
	}
	got := renderAnswerDocCurrentRunStageLaneAuthority(ctx)
	if !strings.Contains(got, "canonical_read_main_sequence=`analyze -> explore -> extract -> finalize`") ||
		strings.Count(got, "- stage_precedence[") != 3 {
		t.Fatalf("typed endpoint span must publish the checkout-verified contiguous stage recipes:\n%s", got)
	}
	if prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil); !strings.Contains(prompt, "## Current Run Stage-Lane Authority") {
		t.Fatalf("endpoint-span authority must reach the real finalizer prompt:\n%s", prompt)
	}
}

func TestRenderAnswerDocCurrentRunStageLaneAuthorityUsesGroundedCanonicalEvidenceSpan(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("required sequence with omitted analyzer participants")
	mu.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: types.ReadModePipelineEnumsFile, LineStart: 34,
			Scope: types.ScopeLine, AnchorKind: types.AnchorStringLiteral,
			Subject: "StageAnalyze", AnchorSymbol: "StageAnalyze", GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceDirect, Source: types.ReadModePipelineEnumsFile, LineStart: 37,
			Scope: types.ScopeLine, AnchorKind: types.AnchorStringLiteral,
			Subject: "StageFinalize", AnchorSymbol: "StageFinalize", GroundingStatus: types.GroundingGrounded,
		},
		{
			ID: "cross-mode-support", Kind: types.EvidenceRelationship,
			Source: "internal/orchestrator/orchestrator.go", LineStart: 2952,
			Scope: types.ScopeLine, AnchorKind: types.AnchorCall, AnchorSymbol: "dispatchStage",
			Subject: "Orchestrator.runWriteAnalyzePhase", Object: "Orchestrator.dispatchStage",
			Producer: types.EvidenceProducerExplorerEmitEvidence, GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.AgentContext{
		RepoRoot: repoRoot, Mode: types.ModeRead, Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			DiagramHint:   &types.DiagramHint{Kind: types.DiagramSequence, Required: true},
		}},
	}
	got := renderAnswerDocCurrentRunStageLaneAuthority(ctx)
	if !strings.Contains(got, "canonical_read_main_sequence=`analyze -> explore -> extract -> finalize`") ||
		strings.Count(got, "- stage_precedence[") != 3 {
		t.Fatalf("grounded endpoints must reach the shared finalizer stage authority:\n%s", got)
	}
	if relations := answerDocVerifiedReadModeStagePrecedenceForRequest(ctx); len(relations) != 3 {
		t.Fatalf("prompt and relation authority drifted: %+v", relations)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Current Run Stage-Lane Authority",
		"stage_precedence[1]",
		"stage_precedence[2]",
		"stage_precedence[3]",
		"They do not prove `call`, `data_flow`, artifact transfer, shared-state participant connectivity, or runtime causality",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("finalizer prompt missing %q:\n%s", want, prompt)
		}
	}
	if firstPass := renderAnswerDocFirstPassDiagramSkeleton(ctx); firstPass != "" {
		t.Fatalf("grounded canonical endpoints must keep cross-mode support out of the generic first-pass floor:\n%s", firstPass)
	}
	if payload := answerDocMechanismPrincipalRelationRepairPayload(ctx); payload == "" ||
		strings.Contains(payload, "runWriteAnalyzePhase") || strings.Contains(payload, "relation_kind=`call`") {
		t.Fatalf("principal repair must retain only the recovered canonical stage span:\n%s", payload)
	}
}

func TestRenderAnswerDocCurrentRunStageLaneAuthorityUsesExactGroundedMembershipWithoutOptionalDimension(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	ctx := &types.AgentContext{
		Mode:     types.ModeRead,
		RepoRoot: repo,
		EvidenceItems: []types.EvidenceItem{{
			Source:          "internal/types/stage_binding.go",
			LineStart:       21,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ReadModeConditionalPreStageBindings",
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	got := renderAnswerDocCurrentRunStageLaneAuthority(ctx)
	if !strings.Contains(got, "conditional_pre_stages=`log_triage, perf_triage`") {
		t.Fatalf("exact grounded membership must activate stage-lane guidance without an optional dimension:\n%s", got)
	}

	ctx.EvidenceItems[0].AnchorSymbol = "AllStages"
	if got := renderAnswerDocCurrentRunStageLaneAuthority(ctx); got != "" {
		t.Fatalf("broad declaration universe alone must not activate Codrax stage authority:\n%s", got)
	}
}

func TestRenderAnswerDocCurrentRunStageLaneAuthorityUsesTypedRequiredStageParticipants(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	ctx := &types.AgentContext{
		Mode:     types.ModeRead,
		RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentExplain,
			PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Entities: []string{"Analyzer", "Explorer", "Extractor", "Finalizer", "BusContext", "Mutable"}},
			DiagramHint: &types.DiagramHint{
				Kind:     types.DiagramFlow,
				Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "Analyzer agent", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Explorer agent", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Extractor agent", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Finalizer agent", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable (in BusContext)", Role: types.DiagramParticipantIncidentRequired},
				},
			},
		}},
		EvidenceItems: []types.EvidenceItem{{
			Source:          "internal/orchestrator/orchestrator.go",
			LineStart:       1536,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "dispatchStage",
			GroundingStatus: types.GroundingGrounded,
		}},
	}

	got := renderAnswerDocCurrentRunStageLaneAuthority(ctx)
	if !strings.Contains(got, "canonical_read_main_sequence=`analyze -> explore -> extract -> finalize`") ||
		!strings.Contains(got, "language model authors only the request classification") {
		t.Fatalf("typed full stage participant slate must activate current-run authority:\n%s", got)
	}
	for _, want := range []string{
		"### Verified stage-artifact grouping recipes",
		"stage_artifact_group_recipe[1]: owner_stage=`StageAnalyze` (`analyze`); owner_agent=`AgentAnalyzer` (`analyzer`); members=`AnalysisIR, TaskGraph, EvidencePlan, AnswerContract, HypothesisSet, QualityGate`",
		"stage_artifact_group_recipe[2]: owner_stage=`StageExplore` (`explore`); owner_agent=`AgentExplorer` (`explorer`); members=`EvidenceItems, AnswerChains, StageReport, aggregate facts`",
		"stage_artifact_group_recipe[4]: owner_stage=`StageFinalize` (`finalize`); owner_agent=`AgentFinalizer` (`finalizer`); members=`AnswerDocumentV2, FinalAnswer, Citations, AnswerContract validation`",
		"authorizes only a no-arrow visual grouping",
		"ownership_group_recipe[1]: owner=`BusContext`; member=`Mutable`; type=`*MutableState`",
		"representation=`mermaid_subgraph_or_group_no_arrow`",
		"not a system-authored diagram and not a directed-flow claim",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed carrier ownership recipe missing %q:\n%s", want, got)
		}
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Current Run Stage-Lane Authority") {
		t.Fatalf("stage participant authority must reach the real finalizer prompt:\n%s", prompt)
	}
}

func TestVerifiedStageAuthorityFeedsRelationCapsuleAndOnlyLeavesRealBoundaries(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
	}
	ctx := &types.AgentContext{
		Mode: types.ModeRead, RepoRoot: repo,
		Mutable: types.NewMutableState("explain the read pipeline"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			DiagramHint:   &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: participants},
		}},
	}
	_ = (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !ctx.Mutable.FinalizerTypedRelationRecipeAvailable() {
		t.Fatal("the prompt compiler must stamp a typed receipt when exact relation recipes were emitted")
	}
	receipt := ctx.Mutable.FinalizerTypedRelationRecipeAnchors()
	if len(receipt) != 3 || receipt[0].FromNode != "n1" || receipt[0].ToNode != "n2" ||
		receipt[0].FromIdentity != "Analyzer" || receipt[0].ToIdentity != "Explorer" ||
		receipt[0].RelationKind != types.DiagramRelPrecedence {
		t.Fatalf("prompt receipt must preserve the exact schema-native recipe anchors: %+v", receipt)
	}
	authority := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"explicit_typed_directed_relations=3",
		"typed_named_participant_relation_coverage: request_scoped_incident=[Analyzer Explorer Extractor Finalizer]; local_typed_incident_only=[]; no_incident_typed_relation=[BusContext]",
		"verified_relation_component_count=1",
		"inter_component_bridge_status=`not_applicable_single_component`",
		"node_alias[n1]=`Analyzer`",
		"node_alias[n4]=`Finalizer`",
		"edge_recipe[1]=`n1 -> n2`; relation_kind=`precedence`",
		"edge_recipe[3]=`n3 -> n4`; relation_kind=`precedence`",
	} {
		if !strings.Contains(authority, want) {
			t.Fatalf("shared stage authority missing from mechanism relation capsule %q:\n%s", want, authority)
		}
	}

	diagramContract := &types.DiagramContract{Required: true, RequiredKind: types.DiagramFlow, Participants: participants}
	contract := renderAnswerDocDiagramContract(ctx, diagramContract)
	if strings.Contains(contract, `participant_identity="Analyzer"`) ||
		strings.Contains(contract, `participant_identity="Explorer"`) ||
		strings.Contains(contract, `participant_identity="Extractor"`) ||
		strings.Contains(contract, `participant_identity="Finalizer"`) {
		t.Fatalf("verified stage participants must not receive false unproven recipes:\n%s", contract)
	}
	if !strings.Contains(contract, `participant_identity="BusContext"`) || strings.Count(contract, "boundary_recipe[") != 1 {
		t.Fatalf("only the genuinely unproved carrier should retain a boundary recipe:\n%s", contract)
	}
}

func TestVerifiedStageAuthorityMarksRequestedSpineApartFromDisconnectedSupport(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
	}
	ctx := &types.AgentContext{
		Mode: types.ModeRead, RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			DiagramHint:   &types.DiagramHint{Kind: types.DiagramSequence, Required: true, Participants: participants},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "supporting-call", Kind: types.EvidenceRelationship,
			Source: "internal/orchestrator/orchestrator.go", LineStart: 2485,
			Scope: types.ScopeLine, AnchorKind: types.AnchorCall, AnchorSymbol: "dispatchStage",
			Subject: "Orchestrator.runAnalyzePhase", Object: "Orchestrator.dispatchStage",
			Producer:        types.EvidenceProducerExplorerEmitEvidence,
			GroundingStatus: types.GroundingGrounded,
		}},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"verified_relation_component_count=2",
		"answer_role=`requested_relation_spine`",
		"answer_role=`supporting_grounded_segment`",
		"requested_relation_spine_component_count=1",
		"principal_diagram_recipe_source=`request_scoped_typed_authority`; exact_edge_count=3",
		"principal_node_alias[n1]=`Analyzer`",
		"principal_node_alias[n4]=`Finalizer`",
		"principal_edge_recipe[1]=`n1 -> n2`; relation_kind=`precedence`",
		"principal_edge_recipe[3]=`n3 -> n4`; relation_kind=`precedence`",
		"edge_anchor_json=`{\"from_node\":\"n1\",\"to_node\":\"n2\",\"from_identity\":\"Analyzer\",\"to_identity\":\"Explorer\",\"relation_kind\":\"precedence\"}`",
		"edge_anchor_json=`{\"from_node\":\"n3\",\"to_node\":\"n4\",\"from_identity\":\"Extractor\",\"to_identity\":\"Finalizer\",\"relation_kind\":\"precedence\"}`",
		"supporting_recipe_policy=`optional_prose_or_separate_visual`",
		"make the `requested_relation_spine` component the principal visual",
		"additional evidence, not a missing hop",
		"Never add an inter-component arrow merely to make one picture",
		"The model still authors the visible diagram",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("request-spine presentation boundary missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Principal-diagram display guidance (soft)") {
		t.Fatalf("an already proved requested spine must not receive the unproven-spine display guidance:\n%s", got)
	}
	if firstPass := renderAnswerDocFirstPassDiagramSkeleton(ctx); firstPass != "" {
		t.Fatalf("a complete request spine must suppress the competing generic first-pass support floor:\n%s", firstPass)
	}
	principalRepair := answerDocMechanismPrincipalRelationRepairPayload(ctx)
	for _, want := range []string{
		"principal_diagram_recipe_source=`request_scoped_typed_authority`",
		"principal_edge_recipe[1]=`n1 -> n2`; relation_kind=`precedence`",
		"principal_edge_recipe[3]=`n3 -> n4`; relation_kind=`precedence`",
		"supporting_recipe_policy=`optional_prose_or_separate_visual`",
	} {
		if !strings.Contains(principalRepair, want) {
			t.Fatalf("principal-only required-repair payload missing %q:\n%s", want, principalRepair)
		}
	}
	for _, forbidden := range []string{"Orchestrator.runAnalyzePhase", "Orchestrator.dispatchStage", "relation_kind=`call`"} {
		if strings.Contains(principalRepair, forbidden) {
			t.Fatalf("supporting relation %q must stay outside the principal repair payload:\n%s", forbidden, principalRepair)
		}
	}
	repairHint, ok := answerDocRequiredDiagramRelationBoundaryPatchHint(ctx, false)
	if !ok || !strings.Contains(repairHint, "request-scoped typed provider already proves the complete principal relation spine") {
		t.Fatalf("required repair must select the principal-only lane:\n%s", repairHint)
	}
	if strings.Contains(repairHint, "Orchestrator.runAnalyzePhase") || strings.Contains(repairHint, "Orchestrator.dispatchStage") {
		t.Fatalf("required principal repair must not re-promote supporting calls:\n%s", repairHint)
	}
	principalBlockStart := strings.Index(got, "principal_diagram_recipe_source=")
	if principalBlockStart < 0 {
		t.Fatalf("compact principal recipe block missing:\n%s", got)
	}
	principalBlockEnd := strings.Index(got[principalBlockStart:], "supporting_recipe_policy=")
	if principalBlockEnd < 0 {
		t.Fatalf("compact principal recipe boundary missing:\n%s", got)
	}
	principalBlockEnd += principalBlockStart
	principalBlock := got[principalBlockStart:principalBlockEnd]
	for _, forbidden := range []string{"Orchestrator.runAnalyzePhase", "Orchestrator.dispatchStage", "relation_kind=`call`"} {
		if strings.Contains(principalBlock, forbidden) {
			t.Fatalf("supporting relation %q must stay outside compact request-spine recipe:\n%s", forbidden, principalBlock)
		}
	}
}

func TestVerifiedStageAuthorityDoesNotPromoteStageSubsetAcrossRequestedCarriers(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	ctx := &types.AgentContext{
		Mode: types.ModeRead, RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{
				Kind:     string(types.ReqMechanism),
				Entities: []string{"Analyzer", "Explorer", "Extractor", "Finalizer", "BusContext", "Mutable"},
			},
			DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: participants},
		}},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"requested_relation_spine_status=`unproven`",
		"request_scoped_typed_relation_subset_count=3",
		"does not cover every typed incident participant",
		"do not call it the complete requested flow",
		"this authority adds no edge",
		"Principal-diagram display guidance (soft)",
		"prioritize the requested participant identities as visible nodes",
		"Keep disconnected local implementation operations in prose or a separate bounded support visual",
		"Use concise repository/domain/business wording for visible labels",
		"Preserve uncovered requested participants as visible disconnected nodes",
		"the model still authors every visible label, node, edge, diagram, and conclusion",
		"`request_scoped_incident` is covered by the exact request-scoped typed provider",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("strict-subset authority boundary missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"answer_role=`requested_relation_spine`",
		"principal_diagram_recipe_source=`request_scoped_typed_authority`",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("stage-only subset must not be promoted via %q:\n%s", forbidden, got)
		}
	}
	if gotCount := strings.Count(got, "answer_role=`supporting_grounded_segment`"); gotCount != 1 {
		t.Fatalf("the truthful stage subset should remain one supporting component, got %d:\n%s", gotCount, got)
	}
}

func TestVerifiedStageAuthorityKeepsLocalCarrierOperationAndRequestedBoundarySeparate(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	ctx := &types.AgentContext{
		Mode: types.ModeRead, RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{
				Kind:     string(types.ReqMechanism),
				Entities: []string{"Analyzer", "Explorer", "Extractor", "Finalizer", "Mutable"},
			},
			DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: participants},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "mutable-call", Producer: types.EvidenceProducerExplorerEmitEvidence,
			Kind: types.EvidenceRelationship, Subject: "analyzerEvaluator.BuildInitialInstruction", Predicate: "calls",
			Object: "ctx.Mutable.ResetPrescanSummary", Source: "internal/agent/analyzer.go", LineStart: 89,
			AnchorKind: types.AnchorCall, AnchorSymbol: "ctx.Mutable.ResetPrescanSummary",
			Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
		}},
	}

	authority := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"request_scoped_incident=[Analyzer Explorer Extractor Finalizer]",
		"local_typed_incident_only=[Mutable]",
		"source_operation_missing=[]; request_visible_boundary_only=[]",
		"local_operation_binding[Mutable][1]",
		"to_identity=`ctx.Mutable.ResetPrescanSummary`",
		"representation=`exact_technical_endpoint_inside_participant_group`",
		"requested_relation_closure=`unproven`; retain_participant_boundary=true",
		"do not retarget the operation directly to the abstract participant node",
		"system supplies no visible node, edge, diagram, or conclusion",
	} {
		if !strings.Contains(authority, want) {
			t.Fatalf("local carrier/requested-boundary split missing %q:\n%s", want, authority)
		}
	}
	if strings.Contains(authority, "source_operation_missing=[Mutable]") {
		t.Fatalf("an already grounded local operation must not trigger duplicate source search:\n%s", authority)
	}

	contract := renderAnswerDocDiagramContract(ctx, &types.DiagramContract{
		Required: true, RequiredKind: types.DiagramFlow, Participants: participants,
	})
	if !strings.Contains(contract, "Typed unproven requested-relation recipes") ||
		!strings.Contains(contract, `participant_identity="Mutable"`) ||
		strings.Count(contract, `participant_identity="Mutable"`) != 1 {
		t.Fatalf("local carrier must retain exactly one requested-relation boundary:\n%s", contract)
	}
	for _, forbidden := range []string{
		`participant_identity="Analyzer"`,
		`participant_identity="Explorer"`,
		`participant_identity="Extractor"`,
		`participant_identity="Finalizer"`,
	} {
		if strings.Contains(contract, forbidden) {
			t.Fatalf("request-scoped incident participant received false boundary %s:\n%s", forbidden, contract)
		}
	}
}

func TestBuildInitialInstructionResetsTypedRelationRecipeReceiptWhenNoRecipeEmitted(t *testing.T) {
	mut := types.NewMutableState("summarize the repository")
	mut.SetFinalizerTypedRelationRecipeAvailable(true)
	mut.SetFinalizerTypedRelationRecipeAnchors([]types.DiagramEdgeAnchor{{
		FromNode: "n1", ToNode: "n2", FromIdentity: "A", ToIdentity: "B", RelationKind: types.DiagramRelCall,
	}})
	ctx := &types.AgentContext{Mutable: mut}
	_ = (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if mut.FinalizerTypedRelationRecipeAvailable() {
		t.Fatal("a fresh prompt without a typed relation recipe must clear the prior dispatch receipt")
	}
	if got := mut.FinalizerTypedRelationRecipeAnchors(); len(got) != 0 {
		t.Fatalf("a fresh prompt without recipes must clear stale exact anchors: %+v", got)
	}
}

func TestRenderAnswerDocCurrentRunStageLaneAuthorityTypedParticipantTriggerFailsClosed(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	base := func() *types.AgentContext {
		return &types.AgentContext{
			Mode:     types.ModeRead,
			RepoRoot: repo,
			AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				PredicateAxis: types.AxisFlow,
				DiagramHint: &types.DiagramHint{
					Kind:     types.DiagramFlow,
					Required: true,
					Participants: []types.DiagramParticipantHint{
						{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
						{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
						{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
						{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
					},
				},
			}},
			EvidenceItems: []types.EvidenceItem{{
				Source:          "internal/orchestrator/topology.go",
				LineStart:       31,
				Scope:           types.ScopeLine,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "pipelineTopology",
				GroundingStatus: types.GroundingGrounded,
			}},
		}
	}

	tests := []struct {
		name   string
		mutate func(*types.AgentContext)
	}{
		{name: "only one stage endpoint", mutate: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.DiagramHint.Participants =
				ctx.AnalysisIR.RequestModel.DiagramHint.Participants[:1]
		}},
		{name: "second stage is context only", mutate: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.DiagramHint.Participants =
				ctx.AnalysisIR.RequestModel.DiagramHint.Participants[:2]
			ctx.AnalysisIR.RequestModel.DiagramHint.Participants[1].Role = types.DiagramParticipantContextOnly
		}},
		{name: "wrong relation axis", mutate: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisCall
		}},
		{name: "trace intent", mutate: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
		}},
		{name: "optional diagram", mutate: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.DiagramHint.Required = false
		}},
		{name: "uncitable authority source", mutate: func(ctx *types.AgentContext) {
			ctx.EvidenceItems[0].GroundingStatus = types.GroundingUngrounded
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := base()
			tc.mutate(ctx)
			if got := renderAnswerDocCurrentRunStageLaneAuthority(ctx); got != "" {
				t.Fatalf("typed trigger must fail closed:\n%s", got)
			}
		})
	}
}

func TestRenderAnswerDocCurrentRunStageLaneAuthorityRejectsLookalikeCheckout(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	path := filepath.Join(repo, "internal", "types", "stage_binding.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data),
		"[]PipelineStage{StageLogTriage, StagePerfTriage}",
		"[]PipelineStage{StageLogTriage, StagePerfTriage, StageMultiRepoFocus}", 1))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		Mode:     types.ModeRead,
		RepoRoot: repo,
		EvidenceItems: []types.EvidenceItem{{
			Source:          "internal/types/stage_binding.go",
			LineStart:       21,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ReadModeConditionalPreStageBindings",
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	if got := renderAnswerDocCurrentRunStageLaneAuthority(ctx); got != "" {
		t.Fatalf("lookalike customer checkout must not receive compiled Codrax authority:\n%s", got)
	}
}

func TestRenderAnswerDocCurrentRunStageLaneAuthorityRejectsLookalikeBindingSemantics(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	path := filepath.Join(repo, "internal", "types", "stage_binding.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := types.ReadModeMainStageBindings()[0].Responsibility
	data = []byte(strings.Replace(string(data), strconv.Quote(want), strconv.Quote("lookalike analyzer responsibility"), 1))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		Mode:     types.ModeRead,
		RepoRoot: repo,
		EvidenceItems: []types.EvidenceItem{{
			Source:          "internal/types/stage_binding.go",
			LineStart:       21,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ReadModeMainStageBindings",
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	if got := renderAnswerDocCurrentRunStageLaneAuthority(ctx); got != "" {
		t.Fatalf("same-named checkout with different producer semantics must fail closed:\n%s", got)
	}
}

func TestRenderAnswerDocCurrentRunStageLaneAuthorityAcceptsFormattingRefactor(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	path := filepath.Join(repo, "internal", "types", "stage_binding.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data),
		"stages := []PipelineStage{StageLogTriage, StagePerfTriage}",
		"stages := []PipelineStage{\n\t\t// Runtime attachments are conditional.\n\t\tStageLogTriage,\n\t\tStagePerfTriage,\n\t}", 1))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		Mode:     types.ModeRead,
		RepoRoot: repo,
		EvidenceItems: []types.EvidenceItem{{
			Source:          "internal/types/stage_binding.go",
			LineStart:       21,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ReadModeConditionalPreStageBindings",
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	got := renderAnswerDocCurrentRunStageLaneAuthority(ctx)
	if !strings.Contains(got, "conditional_pre_stages=`log_triage, perf_triage`") {
		t.Fatalf("format-only refactor must preserve typed stage-lane authority:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_AppendsRequestedDimensionSourceQuotes(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "模型答案已经给出图和表，但压缩了用户原话里的展示细节。",
		}},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{
					{Index: 1, Label: "时序图", SourceQuote: "必须给一张时序图", Required: true, Role: types.RequestedAnswerDimensionOther},
					{Index: 2, Label: "阶段状态表", SourceQuote: "再给一张表列出每个阶段的输入、输出和状态载体，例如 甲/乙", Required: true, Role: types.RequestedAnswerDimensionOther},
				},
				Confidence: 0.9,
			},
		}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"系统补充：输出维度核对",
		"第 1 维：时序图；用户原话：必须给一张时序图",
		"第 2 维：阶段状态表；用户原话：再给一张表列出每个阶段的输入、输出和状态载体，例如 甲/乙",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
}

func TestMissingRequestedAnswerDimensions_DiagramRoleUsesTypedBlockShape(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "类型关系如下。"},
			{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramArchitecture, Language: "mermaid", Body: "flowchart TD\n  Impl --> Contract",
			}},
		},
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []types.RequestedAnswerDimension{{
				Index: 1, Label: "关系图", SourceQuote: "关系图", Required: true,
				Role: types.RequestedAnswerDimensionDiagram,
			}},
		},
	}}}
	if missing := missingRequestedAnswerDimensionsInDocument(ctx, doc); len(missing) != 0 {
		t.Fatalf("typed diagram block must cover diagram dimension without prose-label matching: %+v", missing)
	}
	doc.Blocks = doc.Blocks[:1]
	missing := missingRequestedAnswerDimensionsInDocument(ctx, doc)
	if len(missing) != 1 || missing[0].Role != types.RequestedAnswerDimensionDiagram {
		t.Fatalf("absent diagram block must leave diagram dimension missing: %+v", missing)
	}
}

func TestRenderReadOwnerAnchorSupplement_RendersOwnerRows(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "模型答案回答了调用关系，但压缩了源码定位依据。",
		}},
		ReadOwnerAnchors: []types.OwnerAnchorViewItem{{
			Path:         "internal/agent/subagent_runtime.go",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			Priority:     types.WriteContextP1,
			OwnerSymbol:  "SubAgentRuntime.Run",
			AnchorSymbol: "Run",
			EvidenceRef: &types.WriteExplorationEvidenceRef{
				ID:        "ev-owner",
				Source:    "internal/agent/subagent_runtime.go",
				LineStart: 218,
			},
		}, {
			Path:     "internal/agent/subagent_runtime.go",
			Kind:     types.SourceLocalizationAnchorReadFile,
			Strength: types.SourceLocalizationAnchorObserved,
		}},
	}
	got := renderReadOwnerAnchorSupplement(nil, doc, "zh")
	for _, want := range []string{
		"系统补充：源码定位锚点核对",
		"`internal/agent/subagent_runtime.go`",
		"`SubAgentRuntime.Run` / `Run`",
		"`internal/agent/subagent_runtime.go:218` (`ev-owner`)",
		"`owner`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("supplement missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "observed") {
		t.Fatalf("observed read_file-only anchors must not render as owner proof:\n%s", got)
	}
}

func TestRenderReadOwnerAnchorSupplement_ProjectsOnePreciseRowPerPath(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		ReadOwnerAnchors: []types.OwnerAnchorViewItem{{
			Path:        "pkg/main.go",
			Kind:        types.SourceLocalizationAnchorGroundedEvidence,
			Strength:    types.SourceLocalizationAnchorOwner,
			OwnerSymbol: "broadOwner",
			EvidenceRef: &types.WriteExplorationEvidenceRef{
				ID: "ev-broad", Source: "pkg/main.go", LineStart: 10, LineEnd: 80,
			},
		}, {
			Path:         "pkg/main.go",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			OwnerSymbol:  "exactOwner",
			AnchorSymbol: "exactCall",
			EvidenceRef: &types.WriteExplorationEvidenceRef{
				ID: "ev-exact", Source: "pkg/main.go", LineStart: 42,
			},
		}, {
			Path:         "pkg/other.go",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			OwnerSymbol:  "otherOwner",
			AnchorSymbol: "otherCall",
			EvidenceRef: &types.WriteExplorationEvidenceRef{
				ID: "ev-other", Source: "pkg/other.go", LineStart: 7,
			},
		}},
	}
	got := renderReadOwnerAnchorSupplement(nil, doc, "zh")
	if count := strings.Count(got, "| `pkg/main.go` |"); count != 1 {
		t.Fatalf("supplement must render one localization row per path, got %d:\n%s", count, got)
	}
	for _, want := range []string{
		"`exactOwner` / `exactCall`",
		"`pkg/main.go:42` (`ev-exact`)",
		"`pkg/other.go:7` (`ev-other`)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("supplement missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ev-broad") {
		t.Fatalf("broader same-path anchor must not leak into final supplement:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_DoesNotAppendReadOwnerAnchorSupplementForObservedOnly(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "模型答案不应因为只读过文件就显示 owner 锚点。",
		}},
		ReadOwnerAnchors: []types.OwnerAnchorViewItem{{
			Path:     "pkg/observed.py",
			Kind:     types.SourceLocalizationAnchorReadFile,
			Strength: types.SourceLocalizationAnchorObserved,
		}},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Source:      "read_turn_a",
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"pkg/observed.py"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:     "pkg/observed.py",
				Kind:     types.SourceLocalizationAnchorReadFile,
				Strength: types.SourceLocalizationAnchorObserved,
			}},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：源码定位锚点核对") {
		t.Fatalf("observed-only localization must not append owner supplement:\n%s", out.FinalAnswer)
	}
	for _, want := range []string{
		"系统补充：源码定位状态",
		"`observed_only`",
		"`localization_observed_without_owner`",
		"`pkg/observed.py`",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("observed-only localization status missing %q:\n%s", want, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_SuppressesResolvedReadLastMileSupplements(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "principal",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:          "runtime",
				Label:       "SubAgentRuntime.Run",
				Text:        "Orchestrator 调用 SubAgentRuntime.Run 调度 SubAgent。",
				CitationRef: 0,
			}},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}},
		}},
		Citations: []types.Citation{{
			File:  "internal/agent/subagent_runtime.go",
			Line:  218,
			Quote: "Run is the single entry point for the Orchestrator",
		}},
		ReadOwnerAnchors: []types.OwnerAnchorViewItem{{
			Path:         "internal/agent/subagent_runtime.go",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			OwnerSymbol:  "SubAgentRuntime.Run",
			AnchorSymbol: "Run",
			EvidenceRef: &types.WriteExplorationEvidenceRef{
				ID:        "ev-owner",
				Source:    "internal/agent/subagent_runtime.go",
				LineStart: 218,
			},
		}},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Source:              "read_turn_a",
			Status:              types.SourceLocalizationSupported,
			SourcePaths:         []string{"internal/agent/subagent_runtime.go"},
			SupportedPaths:      []string{"internal/agent/subagent_runtime.go"},
			OwnerSupportedPaths: []string{"internal/agent/subagent_runtime.go"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:         "internal/agent/subagent_runtime.go",
				Kind:         types.SourceLocalizationAnchorGroundedEvidence,
				Strength:     types.SourceLocalizationAnchorOwner,
				OwnerSymbol:  "SubAgentRuntime.Run",
				AnchorSymbol: "Run",
			}},
		},
		ReadNavigationCoverage: &types.RepoMapNavigationCoverage{
			State:          types.RepoMapNavigationCoveragePartial,
			ReasonCode:     "repo_map_navigation_partial",
			RequiredRoutes: []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap, types.RepoMapNavigationRouteRelationMap},
			CoveredRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap},
			MissingRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteRelationMap},
			EvidenceRefs:   []string{"blob://repo-map-task"},
		},
		ReadLocalizerFollowup: &types.ReadLocalizerFollowup{
			State:                types.ReadLocalizerFollowupNeeded,
			ReasonCode:           "read_localizer_navigation_missing",
			CandidatePaths:       []string{"internal/agent/subagent_runtime.go"},
			MissingRoutes:        []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteRelationMap},
			EvidenceRequirements: []string{"repo_map_navigation_requirement route=relation_map required=repo_map_navigation_route"},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "SubAgentRuntime.Run") {
		t.Fatalf("principal answer lost:\n%s", out.FinalAnswer)
	}
	for _, banned := range []string{
		"系统补充：源码定位状态",
		"系统补充：repo_map 导航覆盖",
		"系统补充：读模式定位补充请求",
		"系统补充：源码定位锚点核对",
	} {
		if strings.Contains(out.FinalAnswer, banned) {
			t.Fatalf("resolved principal answer should not append %q:\n%s", banned, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_SuppressesReadAuditSupplementsWhenPrincipalCoversAuditPaths(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "principal",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:          "handler",
				Label:       "handle_request",
				Text:        "请求入口由 handle_request 处理。",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{
			File: "pkg/handler.py",
			Line: 42,
		}},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Source:              "read_turn_a",
			Status:              types.SourceLocalizationObserved,
			SourcePaths:         []string{"pkg/handler.py"},
			OwnerSupportedPaths: []string{"pkg/handler.py"},
		},
		ReadNavigationCoverage: &types.RepoMapNavigationCoverage{
			State:          types.RepoMapNavigationCoveragePartial,
			ReasonCode:     "repo_map_navigation_partial",
			RequiredRoutes: []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap, types.RepoMapNavigationRouteRelationMap},
			CoveredRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap},
			MissingRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteRelationMap},
		},
		ReadLocalizerFollowup: &types.ReadLocalizerFollowup{
			State:          types.ReadLocalizerFollowupNeeded,
			ReasonCode:     "read_localizer_navigation_missing",
			CandidatePaths: []string{"pkg/handler.py"},
			MissingRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteRelationMap},
		},
		ReadOwnerAnchors: []types.OwnerAnchorViewItem{{
			Path:         "pkg/handler.py",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			OwnerSymbol:  "handle_request",
			AnchorSymbol: "handle_request",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "handle_request") {
		t.Fatalf("principal answer lost:\n%s", out.FinalAnswer)
	}
	for _, banned := range []string{
		"系统补充：源码定位状态",
		"系统补充：repo_map 导航覆盖",
		"系统补充：读模式定位补充请求",
		"系统补充：源码定位锚点核对",
	} {
		if strings.Contains(out.FinalAnswer, banned) {
			t.Fatalf("principal citation covers audit paths; should not append %q:\n%s", banned, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_RoutesNonCriticalReadAuditSupplementsOutOfFinalAnswer(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "principal",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:          "router",
				Label:       "Router",
				Text:        "路由层在 Router 中分发。",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{
			File: "pkg/router.py",
			Line: 12,
		}},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Source:              "read_turn_a",
			Status:              types.SourceLocalizationSupported,
			SourcePaths:         []string{"pkg/handler.py"},
			OwnerSupportedPaths: []string{"pkg/handler.py"},
		},
		ReadLocalizerFollowup: &types.ReadLocalizerFollowup{
			State:          types.ReadLocalizerFollowupNeeded,
			ReasonCode:     "read_localizer_owner_missing",
			CandidatePaths: []string{"pkg/handler.py"},
		},
		ReadOwnerAnchors: []types.OwnerAnchorViewItem{{
			Path:         "pkg/handler.py",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			OwnerSymbol:  "handle_request",
			AnchorSymbol: "handle_request",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "Router") {
		t.Fatalf("principal answer lost:\n%s", out.FinalAnswer)
	}
	for _, banned := range []string{
		"系统补充：源码定位状态",
		"系统补充：读模式定位补充请求",
		"系统补充：源码定位锚点核对",
	} {
		if strings.Contains(out.FinalAnswer, banned) {
			t.Fatalf("non-critical audit path should route to status/report, not final answer %q:\n%s", banned, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_DoesNotPublishSuccessfulLocalizationForVisibleUncitedPath(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID: "principal", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
			Text: "配置值来自 src/main/resources/application.properties。",
		}},
		Citations: []types.Citation{{File: "src/main/java/example/Config.java", Line: 21}},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Source:         "read_turn_a",
			Status:         types.SourceLocalizationSupported,
			SourcePaths:    []string{"src/main/resources/application.properties"},
			SupportedPaths: []string{"src/main/resources/application.properties"},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：源码定位状态") {
		t.Fatalf("successful localization is operator telemetry and must not append a system-authored answer table:\n%s", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "application.properties") {
		t.Fatalf("model-authored answer was lost:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_KeepsAnswerCriticalReadAuditSupplementForVisibleUncitedPath(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "principal",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:          "handler",
				Label:       "handle_request",
				Text:        "真正入口在 pkg/handler.py 的 handle_request，但当前引用还指向路由层。",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{
			File: "pkg/router.py",
			Line: 12,
		}},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Source:              "read_turn_a",
			Status:              types.SourceLocalizationWeak,
			SourcePaths:         []string{"pkg/handler.py", "pkg/unrelated.py"},
			OwnerMissingPaths:   []string{"pkg/handler.py"},
			OwnerSupportedPaths: []string{"pkg/router.py", "pkg/unrelated.py"},
		},
		ReadNavigationCoverage: &types.RepoMapNavigationCoverage{
			State:          types.RepoMapNavigationCoveragePartial,
			ReasonCode:     "repo_map_navigation_partial",
			RequiredRoutes: []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap, types.RepoMapNavigationRouteRelationMap},
			CoveredRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap},
			MissingRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteRelationMap},
		},
		ReadLocalizerFollowup: &types.ReadLocalizerFollowup{
			State:          types.ReadLocalizerFollowupNeeded,
			ReasonCode:     "read_localizer_owner_missing",
			CandidatePaths: []string{"pkg/handler.py"},
		},
		ReadOwnerAnchors: []types.OwnerAnchorViewItem{{
			Path:         "pkg/handler.py",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			OwnerSymbol:  "handle_request",
			AnchorSymbol: "handle_request",
		}, {
			Path:         "pkg/unrelated.py",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			OwnerSymbol:  "unrelated_owner",
			AnchorSymbol: "unrelated_owner",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"系统补充：源码定位状态",
		"系统补充：读模式定位补充请求",
		"系统补充：源码定位锚点核对",
		"`pkg/handler.py`",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("answer-critical visible uncited path should preserve audit supplement %q:\n%s", want, out.FinalAnswer)
		}
	}
	if strings.Contains(out.FinalAnswer, "系统补充：repo_map 导航覆盖") {
		t.Fatalf("navigation coverage is status/report material, not answer-critical final-answer supplement:\n%s", out.FinalAnswer)
	}
	for _, banned := range []string{
		"`pkg/unrelated.py`",
		"unrelated_owner",
	} {
		if strings.Contains(out.FinalAnswer, banned) {
			t.Fatalf("non-critical audit rows should not leak into final answer %q:\n%s", banned, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_SuppressesReadSupplementsForGroundedEnumerationSection(t *testing.T) {
	mu := types.NewMutableState("grounded source inventory")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "entries",
			Kind:        types.BlockSection,
			Title:       "@Entry 页面入口",
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{{
				ID:          "index",
				Label:       "Index",
				Text:        "页面入口组件。",
				CitationRef: 0,
			}, {
				ID:          "list",
				Label:       "ListPage",
				Text:        "列表页面入口组件。",
				CitationRef: 1,
			}},
		}},
		Citations: []types.Citation{
			{File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", Line: 7},
			{File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/05_foreach_lazyforeach.ets", Line: 32},
		},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Source:      "read_turn_a",
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:     "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
				Kind:     types.SourceLocalizationAnchorReadFile,
				Strength: types.SourceLocalizationAnchorObserved,
			}},
		},
		ReadNavigationCoverage: &types.RepoMapNavigationCoverage{
			State:          types.RepoMapNavigationCoverageMissing,
			ReasonCode:     "repo_map_navigation_missing",
			RequiredRoutes: []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteSourceInventory},
			MissingRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteSourceInventory},
		},
		ReadLocalizerFollowup: &types.ReadLocalizerFollowup{
			State:          types.ReadLocalizerFollowupNeeded,
			ReasonCode:     "read_localizer_owner_and_navigation_missing",
			CandidatePaths: []string{"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"},
			MissingRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteSourceInventory},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "Index") || !strings.Contains(out.FinalAnswer, "ListPage") {
		t.Fatalf("principal enumeration answer lost:\n%s", out.FinalAnswer)
	}
	for _, banned := range []string{
		"系统补充：源码定位状态",
		"系统补充：repo_map 导航覆盖",
		"系统补充：读模式定位补充请求",
		"系统补充：源码定位锚点核对",
	} {
		if strings.Contains(out.FinalAnswer, banned) {
			t.Fatalf("grounded principal enumeration should not append %q:\n%s", banned, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_SuppressesReadSupplementsForCompleteSourceInventoryMemberSet(t *testing.T) {
	mu := types.NewMutableState("grounded source inventory")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "Cangjie declarations",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Value: "2",
		Members: []string{
			"native_add (package demo.bridge) @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
			"native_add (package demo.ffi) @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
		},
		SupportRefs: []string{
			"native_add: eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
			"native_add: internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "principal",
			Kind:        types.BlockSection,
			SurfaceRole: types.SurfacePrincipal,
			Title:       "Cangjie declarations",
			Text: strings.Join([]string{
				"foreign func declarations:",
				"1. native_add — eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj — demo.bridge",
				"2. native_add — internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj — demo.ffi",
			}, "\n"),
		}},
		Citations: []types.Citation{
			{File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 6},
			{File: "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj", Line: 6},
		},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Source:      "read_turn_a",
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:     "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
				Kind:     types.SourceLocalizationAnchorReadFile,
				Strength: types.SourceLocalizationAnchorObserved,
			}},
		},
		ReadNavigationCoverage: &types.RepoMapNavigationCoverage{
			State:          types.RepoMapNavigationCoveragePartial,
			ReasonCode:     "repo_map_navigation_partial",
			RequiredRoutes: []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteSourceInventory, types.RepoMapNavigationRouteRelationMap},
			CoveredRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteSourceInventory},
			MissingRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteRelationMap},
			EvidenceRefs:   []string{"repo_map:source_inventory"},
		},
		ReadOwnerAnchors: []types.OwnerAnchorViewItem{{
			Path:         "internal/tool/repomap/relation/typed_relation.go",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			OwnerSymbol:  "extendsCandidates",
			AnchorSymbol: "extendsCandidates",
			EvidenceRef: &types.WriteExplorationEvidenceRef{
				ID:        "ev-owner",
				Source:    "internal/tool/repomap/relation/typed_relation.go",
				LineStart: 60,
			},
		}},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
					types.SourceInventoryFieldSummary,
				},
				Confidence: 0.95,
			},
		}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "demo.bridge") || !strings.Contains(out.FinalAnswer, "demo.ffi") {
		t.Fatalf("principal source-inventory answer lost:\n%s", out.FinalAnswer)
	}
	for _, banned := range []string{
		"系统补充：源码定位状态",
		"系统补充：repo_map 导航覆盖",
		"系统补充：读模式定位补充请求",
		"系统补充：源码定位锚点核对",
	} {
		if strings.Contains(out.FinalAnswer, banned) {
			t.Fatalf("complete source-inventory member_set should not append %q:\n%s", banned, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_DoesNotAppendRequestedDimensionWhenQuoteEqualsLabel(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "模型答案自然覆盖了简短展示维度。",
		}},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{
					{Index: 1, Label: "影响", SourceQuote: "影响", Required: true, Role: types.RequestedAnswerDimensionImpact},
				},
				Confidence: 0.9,
			},
		}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：输出维度核对") {
		t.Fatalf("supplement should not repeat dimensions whose source quote equals the label:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_DoesNotAppendCoveredRequestedDimensionSourceQuotes(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "summary",
				Kind:        types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "时序图已经覆盖核心路径。",
			},
			{
				ID:          "table",
				Kind:        types.BlockTable,
				SurfaceRole: types.SurfacePrincipal,
				Title:       "阶段状态表",
				Columns:     []string{"阶段", "输入", "输出"},
				Items: []types.AnswerBlockItem{{
					ID:    "r1",
					Label: "分析阶段",
					Cells: []string{"analyze", "request", "AnalysisIR"},
				}},
			},
		},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{
					{Index: 1, Label: "时序图", SourceQuote: "必须给一张时序图", Required: true, Role: types.RequestedAnswerDimensionOther},
					{Index: 2, Label: "阶段状态表", SourceQuote: "再给一张表列出每个阶段的输入、输出和状态载体，例如 甲/乙", Required: true, Role: types.RequestedAnswerDimensionOther},
				},
				Confidence: 0.9,
			},
		}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：输出维度核对") {
		t.Fatalf("covered requested dimensions should not get a source-quote supplement:\n%s", out.FinalAnswer)
	}
	for _, want := range []string{"时序图", "阶段状态表"} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer should preserve covered dimension %q:\n%s", want, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_TreatsMetricAnchorsAsCoveredRequestedDimension(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "summary",
				Kind:        types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "app-20 dominant_state=runnable，running累计3.5ms，runnable累计5.0ms；下一步应追查 rival-30 来源以确认主因。",
			},
			{
				ID:          "metrics",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Items: []types.AnswerBlockItem{
					{ID: "s1", Label: "sleep累计", Text: "0.0ms"},
					{ID: "s2", Label: "d_state累计", Text: "0.0ms"},
					{ID: "s3", Label: "io_wait累计", Text: "0.0ms"},
					{ID: "s4", Label: "fragments", Text: "21"},
					{ID: "s5", Label: "switches", Text: "20"},
					{ID: "s6", Label: "max_segment", Text: "0.5ms"},
					{ID: "s7", Label: "p95_segment", Text: "0.5ms"},
				},
			},
		},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{
					{Index: 1, Label: "state_churn 统计", SourceQuote: "输出 state_churn 的 dominant_state、running/runnable/sleep/d_state/io_wait 累计、fragments、switches、max_segment、p95_segment", Required: true, Role: types.RequestedAnswerDimensionStageWorkflow},
					{Index: 2, Label: "下一步查主因", SourceQuote: "说明下一步应该如何往下查主因", Required: true, Role: types.RequestedAnswerDimensionFunctionOrPurpose},
				},
				Confidence: 0.9,
			},
		}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：输出维度核对") {
		t.Fatalf("metric anchor coverage should suppress requested-dimension supplement:\n%s", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, "### state_churn 统计") || strings.Contains(out.FinalAnswer, "### 下一步查主因") {
		t.Fatalf("covered dimensions should not require synthetic heading patches:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_DoesNotRepublishModelAggregateFactsAsSystemAuthority(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:       types.AnswerAggregateScalar,
			Label:      "app-20 max_segment",
			Value:      "0.5",
			Unit:       "ms",
			Role:       types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: "runtime_artifact"}},
		},
		{
			Kind:       types.AnswerAggregateScalar,
			Label:      "app-20 p95_segment",
			Value:      "0.5",
			Unit:       "ms",
			Role:       types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: "runtime_artifact"}},
		},
		{
			Kind:       types.AnswerAggregateScalar,
			Label:      "app-20 running",
			Value:      "3.5",
			Unit:       "ms",
			Role:       types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: "runtime_artifact"}},
		},
		{
			Kind:       types.AnswerAggregateScalar,
			Label:      "app-20 dominant_state",
			Value:      "runnable",
			Unit:       "state",
			Role:       types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: "runtime_artifact"}},
		},
		{
			Kind:       types.AnswerAggregateScalar,
			Label:      "app-20 refresh_rate",
			Value:      "16552213 ns ≈ 16.55 ms ≈ 60.4 Hz",
			Unit:       "ns",
			Role:       types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: "runtime_artifact"}},
		},
	})
	mu.RetainInvestigationAggregateFacts()
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "app-20 dominant_state=runnable，running累计3.5ms，max_segment为0.5ms，p95_segment为0.5ms，refresh_rate约60.4 Hz；下一步应追查 rival-30 来源以确认主因。",
		}},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{
					{Index: 1, Label: "state_churn 统计", SourceQuote: "输出 state_churn 的 dominant_state、running/runnable/sleep/d_state/io_wait 累计、fragments、switches、max_segment、p95_segment", Required: true, Role: types.RequestedAnswerDimensionStageWorkflow},
					{Index: 2, Label: "下一步查主因", SourceQuote: "说明下一步应该如何往下查主因", Required: true, Role: types.RequestedAnswerDimensionFunctionOrPurpose},
					{Index: 3, Label: "refresh_rate", SourceQuote: "输出 refresh_rate", Required: true, Role: types.RequestedAnswerDimensionFunctionOrPurpose},
				},
				Confidence: 0.9,
			},
		}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	// The model-authored facts remain available to the finalizer. This fix
	// removes only the later system-publication path; it does not erase model
	// evidence or take over the model's conclusion.
	aggregatePrompt := renderAnswerDocAggregateFacts(ctx)
	if !strings.Contains(aggregatePrompt, "app-20 max_segment") ||
		!strings.Contains(aggregatePrompt, "16552213 ns ≈ 16.55 ms ≈ 60.4 Hz") {
		t.Fatalf("model aggregate facts were removed from finalizer context:\n%s", aggregatePrompt)
	}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "系统补充：结构化指标摘录") ||
		strings.Contains(out.FinalAnswer, "max_segment=0.5ms；p95_segment=0.5ms") {
		t.Fatalf("model-authored aggregate facts were republished as system authority:\n%s", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "app-20 dominant_state=runnable") {
		t.Fatalf("model-owned answer body was unexpectedly removed:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_AppendsTraceQueryPerfQualityMetricSupplement(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Observations: []types.ObservationRecord{{
				ID:              "trace_query:perf:1",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
				Span:            types.ObservationSpan{LineStart: 4, LineEnd: 4},
				ClaimKey:        "runtime_metric:RenderPipeline::draw",
				Subject:         "RenderPipeline::draw",
				Predicate:       "runtime_metric",
				Object:          "libui.so",
				Value:           "12000",
				Unit:            "sample_weight",
				Summary:         "perf samples symbol=RenderPipeline::draw dso=libui.so source=simpleperf_report_sample symbolization_status=symbolized",
				RichNotes: []string{
					"symbol=RenderPipeline::draw",
					"dso=libui.so",
					"perf_quality=cpu_known=1,cpu_unknown=0,sample_cpu_scope=known,source=simpleperf_report_sample,symbolization=symbolized,sample_kind=on_cpu,weight_unit=cycles,clock=record,clock_confidence=assumed,callchain_status=symbolized",
					"perf_quality_caveats=perf period/sample_weight values are event/sample weights, not elapsed duration or expected sample density unless explicit sampling configuration plus calibrated CPU frequency are available",
				},
				SupportRefs: []string{"attached_trace.txt:4"},
			}},
		}},
	})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "ui-31 的 perf 采样命中 RenderPipeline::draw，证据质量中等。",
		}},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	want := "perf_quality=cpu_known=1,cpu_unknown=0,sample_cpu_scope=known,source=simpleperf_report_sample,symbolization=symbolized,sample_kind=on_cpu,weight_unit=cycles"
	if !strings.Contains(out.FinalAnswer, "系统补充：结构化指标摘录") || !strings.Contains(out.FinalAnswer, want) {
		t.Fatalf("final answer missing perf quality metric supplement %q:\n%s", want, out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "perf_quality_caveats=perf period/sample_weight values are event/sample weights") {
		t.Fatalf("final answer missing perf quality caveat supplement:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_AppendsTraceQueryObservationSupplement(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Observations: []types.ObservationRecord{
				{
					ID:              "trace_query:root:1",
					Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
					Producer:        "trace_query",
					Role:            types.AnswerAggregateRolePrincipalAnswer,
					GroundingPolicy: types.ClaimGroundingHard,
					SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
					Span:            types.ObservationSpan{LineStart: 13417, LineEnd: 15158},
					ClaimKey:        "root_cause_primary",
					Subject:         "CookieMonsterCl-59843",
					Predicate:       "root_cause_primary",
					Object:          "runnable",
					Value:           "25.847",
					Unit:            "ms",
					RichNotes:       []string{"occurrence_windows=34579.525319..34579.534164,state=runnable,total=8.800ms;34579.546416..34579.553415,state=runnable,total=7.000ms", "cumulative_impact_ms=25.847", "chain_depth=1", "priority_relation=lower_wakes_higher"},
					SupportRefs:     []string{"attached_trace.txt:13417-15158"},
				},
				{
					ID:              "trace_query:root:duplicate-id",
					Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
					Producer:        "trace_query",
					Role:            types.AnswerAggregateRolePrincipalAnswer,
					GroundingPolicy: types.ClaimGroundingHard,
					SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
					Span:            types.ObservationSpan{LineStart: 13417, LineEnd: 15158},
					ClaimKey:        "root_cause_primary",
					Subject:         "CookieMonsterCl-59843",
					Predicate:       "root_cause_primary",
					Object:          "runnable",
					Value:           "25.847",
					Unit:            "ms",
					RichNotes:       []string{"chain_depth=1", "priority_relation=lower_wakes_higher"},
					SupportRefs:     []string{"attached_trace.txt:13417-15158"},
				},
				{
					ID:              "trace_query:blocking:1",
					Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
					Producer:        "trace_query",
					Role:            types.AnswerAggregateRoleSupportingCoverage,
					GroundingPolicy: types.ClaimGroundingHard,
					SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
					Span:            types.ObservationSpan{LineStart: 11666, LineEnd: 11670},
					ClaimKey:        "critical_blocking:binder_wait",
					Subject:         "com.baidu.tieba-59566",
					Predicate:       "critical_blocking",
					Object:          "Binder:43397_19-23088",
					Value:           "11.103",
					Unit:            "ms",
					RichNotes:       []string{"type=binder_wait", "peer=Binder:43397_19-23088", "chain_relevance=on_chain"},
					SupportRefs:     []string{"attached_trace.txt:11666-11670"},
				},
				{
					ID:              "trace_query:wakeup:path:1",
					Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
					Producer:        "trace_query",
					Role:            types.AnswerAggregateRoleSupportingCoverage,
					GroundingPolicy: types.ClaimGroundingHard,
					SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
					ClaimKey:        "wakeup_chain:path",
					Predicate:       "wakeup_chain",
					Object:          "ThreadPoolForeg-60555 -> NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566",
				},
			},
		}},
	})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "Binder 同步等待需要继续下钻到唤醒链和上游线程状态。",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: single-artifact
	// blocks now hoist the basename into the grouped intro line
	// (本块全部坐标位于 `<base>`) and per-row locators show 行 X–Y (en-dash).
	for _, want := range []string{
		"系统补充：trace_query 关键观测核对",
		"本块全部坐标位于 `attached_trace.txt`",
		"root_cause_primary：CookieMonsterCl-59843 -> runnable",
		"occurrence_windows=34579.525319..34579.534164",
		"cumulative_impact_ms=25.847",
		"critical_blocking:binder_wait：com.baidu.tieba-59566 -> Binder:43397_19-23088",
		"行 11666–11670",
		"chain_relevance=on_chain",
		"wakeup_chain:path：ThreadPoolForeg-60555 -> NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing trace_query supplement fragment %q:\n%s", want, out.FinalAnswer)
		}
	}
	if strings.Contains(out.FinalAnswer, "not_enough_evidence") {
		t.Fatalf("trace_query runtime supplement must not introduce source-status verdicts:\n%s", out.FinalAnswer)
	}
	if got := strings.Count(out.FinalAnswer, "root_cause_primary：CookieMonsterCl-59843 -> runnable"); got != 1 {
		t.Fatalf("trace_query supplement should content-dedupe repeated typed rows, got %d occurrences:\n%s", got, out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_TraceQuerySupplementNormalizesLocatorAndWindowBasis(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Observations: []types.ObservationRecord{{
				ID:              "trace_query:root:1",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRolePrincipalAnswer,
				GroundingPolicy: types.ClaimGroundingHard,
				SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "berlin.systrace"},
				Span: types.ObservationSpan{
					LineStart: 824646, LineEnd: 1624260,
					StartTs: 6793222.031, EndTs: 6793225.370,
				},
				ClaimKey:    "root_cause_primary",
				Subject:     "worker-2",
				Predicate:   "root_cause_primary",
				Object:      "running",
				Value:       "2029.609",
				Unit:        "ms",
				RichNotes:   []string{"impact=2029.609ms", "dominant_state=running", "actual_impact_ms=2677.636"},
				SupportRefs: []string{`D:\temp\南海\xiongqing\berlin.systrace:824646-1624260`},
			}},
		}},
	})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "主导状态为 running,需要保留结构化核对。",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	// §7.30 裁定6: display locator = basename + the record's own time window; the
	// raw customer path and the 800k-line range stay in the raw record. Values
	// coexisting with actual_* carry the selected-window basis label.
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: basename hoisted to
	// the grouped intro (本块全部坐标位于); 选定窗→查询窗; the impact= note is now
	// skipped as a 值= duplicate (impact_ms=≡值 dedupe) — the magnitude stays on
	// the rendered 值= field.
	for _, want := range []string{
		"系统补充：trace_query 关键观测核对",
		"本块全部坐标位于 `berlin.systrace`",
		"[6793222.031~6793225.370s]",
		"值=2029.609ms",
		"窗口基准=查询窗",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("supplement missing %q:\n%s", want, out.FinalAnswer)
		}
	}
	for _, banned := range []string{`D:\temp`, "xiongqing", ":824646-1624260"} {
		if strings.Contains(out.FinalAnswer, banned) {
			t.Fatalf("raw customer locator %q must not render:\n%s", banned, out.FinalAnswer)
		}
	}
}

// TestAnswerDocumentEvaluator_TraceQuerySupplementWindowBasisEndpoints pins
// NEW-8 (账本 §7.6): when the record's own typed selected_window note parses,
// the renderer-invented window-basis token names the endpoints inline
// ("窗口基准=选定窗 X.XXXs–Y.YYYs"); the observation data itself stays
// untouched and the note-less legacy token is pinned by the test above.
func TestAnswerDocumentEvaluator_TraceQuerySupplementWindowBasisEndpoints(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Observations: []types.ObservationRecord{{
				ID:              "trace_query:root:1",
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRolePrincipalAnswer,
				GroundingPolicy: types.ClaimGroundingHard,
				SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "berlin.systrace"},
				Span: types.ObservationSpan{
					LineStart: 824646, LineEnd: 1624260,
					StartTs: 6793222.031, EndTs: 6793225.370,
				},
				ClaimKey:  "root_cause_primary",
				Subject:   "worker-2",
				Predicate: "root_cause_primary",
				Object:    "running",
				Value:     "2029.609",
				Unit:      "ms",
				RichNotes: []string{
					"impact=2029.609ms", "dominant_state=running", "actual_impact_ms=2677.636",
					"selected_window=6793222.031000..6793225.370000",
				},
				SupportRefs: []string{"berlin.systrace:824646-1624260"},
			}},
		}},
	})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "主导状态为 running,需要保留结构化核对。",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 选定窗→查询窗 (窗族).
	if !strings.Contains(out.FinalAnswer, "窗口基准=查询窗 6793222.031s~6793225.370s") {
		t.Fatalf("supplement window-basis token must render the selected-window endpoints:\n%s", out.FinalAnswer)
	}
}

// TestTraceQueryObservationSupplementNotes_SelectedWindowEndpoints pins the
// NEW-8 token shape on both language surfaces plus the malformed-note fallback
// (shared strict parser; legacy token stays byte-identical).
func TestTraceQueryObservationSupplementNotes_SelectedWindowEndpoints(t *testing.T) {
	record := types.ObservationRecord{
		RichNotes: []string{
			"impact=2029.609ms", "dominant_state=running", "actual_impact_ms=2677.636",
			"selected_window=6793222.031000..6793225.370000",
		},
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: zh 选定窗→查询窗
	// (EN token unchanged).
	if zh := traceQueryObservationSupplementNotes(record, true); !strings.Contains(zh, "窗口基准=查询窗 6793222.031s~6793225.370s") {
		t.Fatalf("ZH supplement token must carry endpoints: %s", zh)
	}
	if en := traceQueryObservationSupplementNotes(record, false); !strings.Contains(en, "window_basis=selected_window 6793222.031s~6793225.370s") {
		t.Fatalf("EN supplement token must carry endpoints: %s", en)
	}
	record.RichNotes[3] = "selected_window=..6793225.370000"
	zh := traceQueryObservationSupplementNotes(record, true)
	if !strings.Contains(zh, "窗口基准=查询窗") || strings.Contains(zh, "窗口基准=查询窗 ") {
		t.Fatalf("malformed note must fall back to the bare ZH token: %s", zh)
	}
	en := traceQueryObservationSupplementNotes(record, false)
	if !strings.Contains(en, "window_basis=selected_window") || strings.Contains(en, "window_basis=selected_window ") {
		t.Fatalf("malformed note must fall back to the bare EN token: %s", en)
	}
}

// TestTraceQueryObservationSupplementNotes_PerTypePriority pins the SG-batch
// per-type note priority inside the 4-note window (§10-A3/C4 + Q4-K 修3 共因,
// 一修三收): binder_wait rows front the peer_state pair, blocking/monitor rows
// front blocking_kind/holder_site, periodic-source rows front the discounted
// effective_impact_ms + periodic_source flag — and in every case the row's
// identity/raw keys keep the remaining slots so the raw figure stays visible
// as the comparison value. Classification is typed (exact type= / periodic_
// source= note match), never prose.
func TestTraceQueryObservationSupplementNotes_PerTypePriority(t *testing.T) {
	notesOf := func(record types.ObservationRecord) string {
		return traceQueryObservationSupplementNotes(record, false)
	}
	// binder_wait: peer_state_dominant/sleep displace chain_relevance/edge_count.
	binder := types.ObservationRecord{RichNotes: []string{
		"type=binder_wait", "peer=OS_IPC_peer-43722", "chain_relevance=on_chain", "edge_count=3",
		"peer_state_dominant=sleep", "peer_state_sleep=31.600ms",
	}}
	got := notesOf(binder)
	want := "notes=peer_state_dominant=sleep, peer_state_sleep=31.600ms, type=binder_wait, peer=OS_IPC_peer-43722"
	if got != want {
		t.Fatalf("binder_wait priority selection:\n got %q\nwant %q", got, want)
	}
	// blocking_span carrying the parsed lock payload: kind + holder site lead;
	// type/peer fill; waiters is table-admitted but capped out here.
	blocking := types.ObservationRecord{RichNotes: []string{
		"type=blocking_span", "peer=Holder_Operate_0-42067", "blocking_kind=monitor_contention",
		"holder_site=SomeManager.list(SomeManager.java:1258)", "waiters=2", "chain_relevance=on_chain",
	}}
	got = notesOf(blocking)
	want = "notes=blocking_kind=monitor_contention, holder_site=SomeManager.list(SomeManager.java:1258), type=blocking_span, peer=Holder_Operate_0-42067"
	if got != want {
		t.Fatalf("blocking_span priority selection:\n got %q\nwant %q", got, want)
	}
	// blocking_span WITHOUT a parsed payload falls back to the peer_state pair.
	blockingNoPayload := types.ObservationRecord{RichNotes: []string{
		"type=blocking_span", "peer=unknown-thread", "chain_relevance=on_chain", "edge_count=1",
		"peer_state_dominant=running", "peer_state_sleep=0.000ms",
	}}
	got = notesOf(blockingNoPayload)
	want = "notes=peer_state_dominant=running, peer_state_sleep=0.000ms, type=blocking_span, peer=unknown-thread"
	if got != want {
		t.Fatalf("payload-less blocking_span priority selection:\n got %q\nwant %q", got, want)
	}
	// BLK-2 P1: the holder-subject folded rank row carries the twin state
	// breakdown under subject_state_* (the subject IS the holder). It must reach
	// the 4-note window exactly like the waiter-subject peer_state_* pair above —
	// same slot-2 fallback when no holder_site parsed — otherwise the audit
	// window silently regresses to the pre-BLK-2 omission.
	holderSubjectRank := types.ObservationRecord{RichNotes: []string{
		"type=blocking_span", "peer=Waiter-99", "chain_relevance=on_chain", "edge_count=1",
		"subject_state_dominant=running", "subject_state_sleep=0.000ms",
	}}
	got = notesOf(holderSubjectRank)
	want = "notes=subject_state_dominant=running, subject_state_sleep=0.000ms, type=blocking_span, peer=Waiter-99"
	if got != want {
		t.Fatalf("holder-subject subject_state priority selection (BLK-2):\n got %q\nwant %q", got, want)
	}
	// Realistic folded-row shape: the fold condition guarantees blocking_kind is
	// non-empty on every holder-subject rank row, so the cap=2 priority window
	// splits 1+1 — blocking_kind leads, and the scan pointer passes over the
	// absent holder_site/peer_state_* prefixes to reach subject_state_dominant.
	holderSubjectFolded := types.ObservationRecord{RichNotes: []string{
		"type=blocking_span", "peer=Waiter-99", "blocking_kind=monitor_contention",
		"chain_relevance=on_chain", "subject_state_dominant=running", "subject_state_sleep=0.000ms",
	}}
	got = notesOf(holderSubjectFolded)
	want = "notes=blocking_kind=monitor_contention, subject_state_dominant=running, type=blocking_span, peer=Waiter-99"
	if got != want {
		t.Fatalf("folded holder-subject rank row priority selection (BLK-2):\n got %q\nwant %q", got, want)
	}
	// periodic-source row: the discounted pair leads, the raw impact keeps a
	// slot as the comparison figure (never silently displaced).
	periodic := types.ObservationRecord{RichNotes: []string{
		"impact_ms=42.600", "dominant_state=sleep", "occurrences=12",
		"periodic_source=true", "detected_period_ms=16.667", "lateness_ms=0.208", "effective_impact_ms=0.208",
	}}
	got = notesOf(periodic)
	want = "notes=effective_impact_ms=0.208, periodic_source=true, impact_ms=42.600, dominant_state=sleep"
	if got != want {
		t.Fatalf("periodic-source priority selection:\n got %q\nwant %q", got, want)
	}
	// Unclassified rows keep the legacy first-4 RichNotes-order selection —
	// including NOT selecting a stray effective_impact_ms (no priority family,
	// and the key is priority-lane-only by design).
	legacy := types.ObservationRecord{RichNotes: []string{
		"type=runnable_wait", "impact_ms=10.000", "effective_impact_ms=8.000",
		"dominant_state=runnable", "occurrences=2", "prio=120",
	}}
	got = notesOf(legacy)
	want = "notes=type=runnable_wait, impact_ms=10.000, dominant_state=runnable, occurrences=2"
	if got != want {
		t.Fatalf("unclassified row must keep the legacy selection:\n got %q\nwant %q", got, want)
	}
	// waiters= is a first-class allowed key now (Q4-K 修3): with no competing
	// priority notes it survives the regular scan.
	waiters := types.ObservationRecord{RichNotes: []string{
		"type=priority_inversion_runnable_wait", "peer=some-peer-7", "waiters=3", "impact_ms=5.000",
	}}
	got = notesOf(waiters)
	want = "notes=type=priority_inversion_runnable_wait, peer=some-peer-7, waiters=3, impact_ms=5.000"
	if got != want {
		t.Fatalf("waiters must pass the allowed table:\n got %q\nwant %q", got, want)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_AppendsTraceStateDrilldownSupplement(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Observations: []types.ObservationRecord{
				{
					ID:              "trace_query:drilldown:top-sleep",
					Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
					Producer:        "trace_query",
					Role:            types.AnswerAggregateRoleSupportingCoverage,
					GroundingPolicy: types.ClaimGroundingHard,
					SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
					Span:            types.ObservationSpan{LineStart: 210, LineEnd: 230},
					ClaimKey:        "state_drilldown:main-1:S",
					Subject:         "main-1",
					Predicate:       "state_drilldown",
					Object:          "S",
					Value:           "21.000",
					Unit:            "ms",
					RichNotes: []string{
						"source=top_sleep",
						"recommended_views=wakeup_chain,root_cause_rank",
						"chain_required=true",
						"recursive=true",
						"window=1.000000..1.200000",
					},
					SupportRefs: []string{"attached_trace.txt:210-230"},
				},
				{
					ID:              "trace_query:drilldown:fragmented-sleep",
					Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
					Producer:        "trace_query",
					Role:            types.AnswerAggregateRoleSupportingCoverage,
					GroundingPolicy: types.ClaimGroundingHard,
					SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
					Span:            types.ObservationSpan{LineStart: 240, LineEnd: 260},
					ClaimKey:        "state_drilldown:main-1:S:fragmented",
					Subject:         "main-1",
					Predicate:       "state_drilldown",
					Object:          "S",
					Value:           "18.000",
					Unit:            "ms",
					RichNotes: []string{
						"source=state_churn",
						"recommended_views=thread_timeline,interaction_stats,window_stats",
						"chain_required=false",
						"recursive=false",
					},
					SupportRefs: []string{"attached_trace.txt:240-260"},
				},
			},
		}},
	})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "主线程阻塞需要保留状态下钻结论。",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: intro sentence
	// "以下条目来自 trace_query 发布的结构化运行时观测…" → "以下为 trace_query 输出的
	// 逐条观测记录(字段 token 保留原文)，可对照 trace 原文核对…"; the "main-1 -> S"
	// half is now claimKey-echo-suppressed (subject/object are both full
	// claimKey segments).
	for _, want := range []string{
		"系统补充：trace_query 关键观测核对",
		"以下为 trace_query 输出的逐条观测记录(字段 token 保留原文)，可对照 trace 原文核对；所有坐标都指向 trace 文件本身，不是源码位置。",
		"state_drilldown:main-1:S；值=21.000ms",
		"值=21.000ms",
		"source=top_sleep",
		"recommended_views=wakeup_chain,root_cause_rank",
		"chain_required=true",
		"recursive=true",
		"state_drilldown:main-1:S:fragmented；值=18.000ms",
		"值=18.000ms",
		"source=state_churn",
		"recommended_views=thread_timeline,interaction_stats,window_stats",
		"chain_required=false",
		"recursive=false",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing trace state-drilldown supplement fragment %q:\n%s", want, out.FinalAnswer)
		}
	}
	// §7.30 裁定5 review follow-up: renderer-invented labels and the guide
	// sentence are all-Chinese on the ZH face; only the raw key=value note
	// pairs (the 裁定6 audit carrier) stay verbatim.
	for _, banned := range []string{"value=21.000ms", "notes=", "结构化 runtime observation", "artifact-local"} {
		if strings.Contains(out.FinalAnswer, banned) {
			t.Fatalf("ZH supplement must not carry mixed-language renderer label %q:\n%s", banned, out.FinalAnswer)
		}
	}
	if strings.Contains(out.FinalAnswer, "not_enough_evidence") ||
		strings.Contains(out.FinalAnswer, "current-repo source citations") {
		t.Fatalf("state_drilldown supplement must stay runtime-observation only:\n%s", out.FinalAnswer)
	}
}

func TestTraceQueryObservationSupplementWakeupPathDoesNotPrependTarget(t *testing.T) {
	record := types.ObservationRecord{
		ClaimKey:  "wakeup_chain:path#1",
		Subject:   "app-100",
		Predicate: "wakeup_chain",
		Object:    "worker-200 -> app-100",
	}
	for _, tc := range []struct {
		zh   bool
		want string
	}{
		{zh: true, want: "wakeup_chain:path#1：worker-200 -> app-100"},
		{zh: false, want: "wakeup_chain:path#1: worker-200 -> app-100"},
	} {
		got := traceQueryObservationSupplementText(record, tc.zh)
		if !strings.HasPrefix(got, tc.want) {
			t.Fatalf("wakeup path must render the producer's complete path verbatim, zh=%t got=%q", tc.zh, got)
		}
		if strings.Contains(got, "app-100 -> worker-200 -> app-100") {
			t.Fatalf("target was prepended to an already complete path, zh=%t got=%q", tc.zh, got)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_TraceQueryObservationSupplementExpandsBeyondOldEightRowCap(t *testing.T) {
	observations := make([]types.ObservationRecord, 0, 32)
	for i := 1; i <= 32; i++ {
		observations = append(observations, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:root:%02d", i),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
			Span:            types.ObservationSpan{LineStart: 100 + i, LineEnd: 100 + i},
			ClaimKey:        fmt.Sprintf("root_cause_primary:thread-%02d", i),
			Subject:         fmt.Sprintf("thread-%02d", i),
			Predicate:       "root_cause_primary",
			Object:          "runnable",
			Value:           fmt.Sprintf("%d.000", i),
			Unit:            "ms",
			RichNotes:       []string{"chain_relevance=on_chain"},
			SupportRefs:     []string{fmt.Sprintf("attached_trace.txt:%d", 100+i)},
		})
	}

	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: observations,
	}}})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "trace_query 已给出多层 on-chain 观测。",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: single-artifact
	// locator "attached_trace.txt:132" → grouped intro + per-row "行 132".
	for _, want := range []string{
		"系统补充：trace_query 关键观测核对",
		"本块全部坐标位于 `attached_trace.txt`",
		"root_cause_primary:thread-01：thread-01 -> runnable",
		"root_cause_primary:thread-32：thread-32 -> runnable",
		"行 132；",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("expanded trace_query supplement missing %q:\n%s", want, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_PrioritizesTraceQueryOccurrenceWindowsSupplement(t *testing.T) {
	observations := make([]types.ObservationRecord, 0, 9)
	for i := 0; i < 8; i++ {
		observations = append(observations, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:root:%d", i),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
			ClaimKey:        "root_cause_primary",
			Subject:         fmt.Sprintf("background-%d", i),
			Predicate:       "root_cause_primary",
			Object:          "compute_supply",
			Value:           "1.000",
			Unit:            "ms",
			RichNotes:       []string{"chain_relevance=background"},
			SupportRefs:     []string{fmt.Sprintf("attached_trace.txt:%d", 100+i)},
		})
	}
	observations = append(observations, types.ObservationRecord{
		ID:              "trace_query:aggregate:threadpool",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
		ClaimKey:        "wakeup_causal_aggregate:ThreadPoolForeg-60555",
		Subject:         "ThreadPoolForeg-60555",
		Predicate:       "wakeup_causal_aggregate",
		Object:          "d_sleep",
		Value:           "17.442",
		Unit:            "ms",
		RichNotes: []string{
			"occurrence_windows=34579.525319..34579.534164,state=d_sleep,total=7.562ms;34579.546416..34579.553415,state=d_sleep,total=6.620ms;34579.576702..34579.587805,state=d_sleep,total=10.249ms",
			"path=ThreadPoolForeg-60555 -> NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566",
			"chain_relevance=on_chain",
		},
		SupportRefs: []string{"attached_trace.txt:8712-15131"},
	})

	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: observations,
	}}})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "ThreadPoolForeg 聚合链路需要保留代表性窗口。",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"wakeup_causal_aggregate:ThreadPoolForeg-60555",
		"occurrence_windows=34579.525319..34579.534164",
		"34579.546416..34579.553415",
		"34579.576702..34579.587805",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing prioritized occurrence-window fragment %q:\n%s", want, out.FinalAnswer)
		}
	}
}

func TestRenderAnswerDocObservationLedger_IncludesTraceObservationCoverage(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			stageReportTraceObservation("root", "trace_query[0]", "root_cause_primary", "root_cause_primary", "main-1", "runnable", "11.000", []string{"chain_relevance=on_chain"}, types.ObservationSpan{StartTs: 1.0, EndTs: 1.2}),
			stageReportTraceObservation("drill", "trace_query[0]", "state_drilldown", "state_drilldown:main-1:S", "main-1", "S", "21.000", []string{"source=top_sleep", "recommended_views=wakeup_chain,root_cause_rank", "chain_required=true", "recursive=true"}, types.ObservationSpan{StartTs: 1.0, EndTs: 1.2}),
			stageReportTraceObservation("fragmented", "trace_query[0]", "state_drilldown", "state_drilldown:main-1:S:fragmented", "main-1", "S", "18.000", []string{"source=state_churn", "recommended_views=thread_timeline,interaction_stats,window_stats", "chain_required=false", "recursive=false"}, types.ObservationSpan{StartTs: 1.0, EndTs: 1.2}),
		},
	}}})
	ctx := &types.AgentContext{Mutable: mu}

	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### Trace Observation Coverage",
		"trace_query_calls=1; trace_observations=3",
		"dimensions: root_cause_rank:1(on=1 adjacent=0 background=0), state_drilldown:2",
		"soft_followup_candidates: `wakeup_chain`, `critical_blocking_calls`, `window_stats_resource_pressure`",
		"top[1] dimension=`root_cause_rank`; id=`root`",
		"chain_relevance=`on_chain`",
		"window=1.000000..1.200000",
		"support_refs=`trace.systrace:10-20`",
		"top[2] dimension=`state_drilldown`; id=`drill`; window=1.000000..1.200000",
		"drilldown_source=`top_sleep`; recommended_views=`wakeup_chain`, `root_cause_rank`; chain_required=true; recursive=true",
		"top[3] dimension=`state_drilldown`; id=`fragmented`; window=1.000000..1.200000",
		"drilldown_source=`state_churn`; recommended_views=`thread_timeline`, `interaction_stats`, `window_stats`; chain_required=false; recursive=false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation ledger missing trace coverage fragment %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hard_block=true") || strings.Contains(got, "completion_blocker=true") {
		t.Fatalf("trace coverage should remain soft handoff, not hard blocker:\n%s", got)
	}
}

func TestRenderAnswerDocObservationLedger_IncludesTraceShardAggregates(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			stageReportTraceObservation("root1", "trace_query[1]", "root_cause_primary", "root_cause_primary", "main-1", "running", "12.000", []string{"chain_relevance=on_chain"}, types.ObservationSpan{StartTs: 1.000, EndTs: 1.100}),
			stageReportTraceObservation("root2", "trace_query[2]", "root_cause_primary", "root_cause_primary", "main-1", "running", "9.000", []string{"chain_relevance=on_chain"}, types.ObservationSpan{StartTs: 1.100, EndTs: 1.200}),
			stageReportTraceObservation("state1", "trace_query[1]", "state_drilldown", "state_drilldown:main:S", "main-1", "S", "18.000", []string{"source=top_sleep", "recommended_views=wakeup_chain,root_cause_rank", "chain_required=true", "recursive=true", "significant=true"}, types.ObservationSpan{StartTs: 1.000, EndTs: 1.100}),
			stageReportTraceObservation("state2", "trace_query[2]", "state_drilldown", "state_drilldown:main:S", "main-1", "S", "14.000", []string{"source=top_sleep", "recommended_views=wakeup_chain,root_cause_rank", "chain_required=true", "recursive=true", "significant=true"}, types.ObservationSpan{StartTs: 1.100, EndTs: 1.200}),
		},
	}}})
	ctx := &types.AgentContext{Mutable: mu}

	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"shard_aggregates: bounded shard summaries below are soft parent-window handoff, not completion blockers.",
		"shard[1] subject=`main-1`; object=`running`; chain_relevance=`on_chain`; shards=2; total_impact=21.000ms; max_shard=12.000ms",
		"cross_shard_additivity=`disjoint_windows`",
		"window=1.000000..1.200000",
		"example_windows=`1.000000..1.100000`, `1.100000..1.200000`",
		"support_refs=`trace.systrace:10-20`",
		"state_shard_aggregates: bounded state/window-stats shard summaries below are soft parent-window handoff, not completion blockers.",
		"state_shard[1] dimension=`state_drilldown`; subject=`main-1`; object=`S`; shards=2; significant_shards=2; total_impact=32.000ms; max_shard=18.000ms",
		"source=`top_sleep`",
		"recommended_views=`wakeup_chain`, `root_cause_rank`",
		"chain_required=true; recursive=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation ledger missing shard aggregate fragment %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hard_block=true") || strings.Contains(got, "completion_blocker=true") {
		t.Fatalf("trace shard aggregates should remain soft handoff:\n%s", got)
	}
}

func TestRenderAnswerDocObservationLedger_OverlappingTraceShardsWithdrawTotal(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			stageReportTraceObservation("block1", "trace_query[1]", "block_io_by_inode", "block_io_by_inode:block", "block", "block_rq", "4.262", nil, types.ObservationSpan{StartTs: 1.000, EndTs: 1.150}),
			stageReportTraceObservation("block2", "trace_query[2]", "block_io_by_inode", "block_io_by_inode:block", "block", "block_rq", "3.100", nil, types.ObservationSpan{StartTs: 1.100, EndTs: 1.200}),
		},
	}}})
	got := renderAnswerDocObservationLedger(&types.AgentContext{Mutable: mu})
	for _, want := range []string{
		"state_shard[1] dimension=`resource_pressure`",
		"total_impact=`unavailable`",
		"max_shard=4.262ms",
		"cross_shard_additivity=`forbidden_overlapping_windows`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("overlapping shard finalizer handoff missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "total_impact=7.362ms") {
		t.Fatalf("overlapping shard windows fabricated an additive total:\n%s", got)
	}
}

func writeStageBindingFixture(t *testing.T, repo string) {
	t.Helper()
	dir := filepath.Join(repo, "internal", "types")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir stage binding fixture: %v", err)
	}
	content := strings.Join([]string{
		"package types",
		"",
		"type StageBinding struct {",
		"\tStage string",
		"\tAgent string",
		"\tSkill string",
		"\tTerminal bool",
		"\tResponsibility string",
		"\tPrimaryArtifacts []string",
		"}",
		"",
		"var builtinStageBindings = []StageBinding{",
		"\t{Stage: StageLogTriage, Agent: AgentLogTriager, Skill: \"log-triage-skill\"},",
		"\t{Stage: StagePerfTriage, Agent: AgentPerfTriager, Skill: \"perf-triage-skill\"},",
	}, "\n")
	for _, binding := range types.ReadModeMainStageBindings() {
		stageIdent, agentIdent, ok := readModeStageBindingIdentifiers(binding)
		if !ok {
			t.Fatalf("unexpected main stage binding: %+v", binding)
		}
		artifacts := make([]string, 0, len(binding.PrimaryArtifacts))
		for _, artifact := range binding.PrimaryArtifacts {
			artifacts = append(artifacts, strconv.Quote(artifact))
		}
		content += fmt.Sprintf("\n\t{Stage: %s, Agent: %s, Skill: %q, Terminal: %t, Responsibility: %q, PrimaryArtifacts: []string{%s}},",
			stageIdent, agentIdent, binding.Skill, binding.Terminal, binding.Responsibility, strings.Join(artifacts, ", "))
	}
	content += "\n" + strings.Join([]string{
		"}",
		"",
		"func ReadModeConditionalPreStageBindings() []StageBinding {",
		"\tstages := []PipelineStage{StageLogTriage, StagePerfTriage}",
		"\t_ = stages",
		"\treturn nil",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "stage_binding.go"), []byte(content), 0o644); err != nil {
		t.Fatalf("write stage binding fixture: %v", err)
	}
	mainStageIdents := make([]string, 0, len(types.ReadModeMainStageBindings()))
	for _, binding := range types.ReadModeMainStageBindings() {
		stageIdent, _, ok := readModeStageBindingIdentifiers(binding)
		if !ok {
			t.Fatalf("unexpected main stage binding: %+v", binding)
		}
		mainStageIdents = append(mainStageIdents, stageIdent)
	}
	enums := "package types\n\ntype PipelineStage string\n\nfunc AllMainStages() []PipelineStage {\n\treturn []PipelineStage{" + strings.Join(mainStageIdents, ", ") + "}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "enums.go"), []byte(enums), 0o644); err != nil {
		t.Fatalf("write stage sequence fixture: %v", err)
	}
	contextSource := `package types
type MutableState struct { answerDocumentV2 *AnswerDocumentV2 }
type BusContext struct {
 Mutable *MutableState
 PipelineStage PipelineStage
 ActiveAgent AgentName
 EvidenceItems []EvidenceItem
 AnswerChains []AnswerChain
 AnswerSymbols []AnswerSymbol
 StageReports []StageReport
 AnalysisIR *AnalysisIR
}`
	if err := os.WriteFile(filepath.Join(dir, "context.go"), []byte(contextSource), 0o644); err != nil {
		t.Fatalf("write state carrier fixture: %v", err)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_RendersRetryStateDraftWithoutFailedRepairContent(t *testing.T) {
	prevDoc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "上一版结构化答案正文",
		}},
		Citations: []types.Citation{{File: "internal/agent/agent.go", Line: 969}},
	}
	prevJSON, err := json.Marshal(prevDoc)
	if err != nil {
		t.Fatalf("marshal prev doc: %v", err)
	}
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	ctx.Mutable.SetRetryState(&types.RetryState{
		Attempt:      2,
		PrevEmitJSON: prevJSON,
	})
	messages := []llm.Message{
		{Role: "assistant", Content: "【审计员注】patch 仍未通过。"},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "answer_document emission missing") {
		t.Fatalf("retry-state draft recovery should avoid raw-only missing-doc banner: %q", out.FinalAnswer)
	}
	for _, want := range []string{
		"上一版结构化答案正文",
		"internal/agent/agent.go:969",
		"最终重试未能产出有效的 answer_document",
		"失败修补回合的临时推理未包含在答案中",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
	if strings.Contains(out.FinalAnswer, "模型最后一轮原文") || strings.Contains(out.FinalAnswer, "【审计员注】patch 仍未通过。") {
		t.Fatalf("failed repair-turn content must not be appended beside recovered draft:\n%s", out.FinalAnswer)
	}
	if doc := ctx.Mutable.AnswerDocumentV2(); doc != nil {
		t.Fatalf("retry-state recovery should not mark rejected draft as accepted mutable document: %+v", doc)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_RetryStateDiagramDropsOlderRejectedDiagramAttachment(t *testing.T) {
	prevDoc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "恢复正文"},
			{
				ID:   "current-diagram",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramSequence,
					Body: "sequenceDiagram\n  Current->>Target: current",
				},
			},
		},
	}
	prevJSON, err := json.Marshal(prevDoc)
	if err != nil {
		t.Fatalf("marshal prev doc: %v", err)
	}
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	ctx.Mutable.SetRetryState(&types.RetryState{Attempt: 2, PrevEmitJSON: prevJSON})
	ctx.Mutable.SetAnswerDisplayAttachments([]types.AnswerDisplayAttachment{{
		Kind:   types.AnswerDisplayAttachmentDiagram,
		Body:   "sequenceDiagram\n  Old->>Wrong: rejected",
		Source: "emit_answer_document.rejected_payload",
	}})
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, []llm.Message{{Role: "assistant", Content: "最终 patch 未通过"}}, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "Current->>Target: current") {
		t.Fatalf("retry-state diagram was lost:\n%s", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, "Old->>Wrong: rejected") || strings.Contains(out.FinalAnswer, "系统保留内容") {
		t.Fatalf("older rejected diagram must not re-enter beside the shipped retry-state diagram:\n%s", out.FinalAnswer)
	}
	if got := ctx.Mutable.AnswerDisplayAttachments(); len(got) != 0 {
		t.Fatalf("stale retry attachment must be cleared from mutable state: %+v", got)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_RetryStateInvalidDiagramFallsBackToText(t *testing.T) {
	prevDoc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "恢复正文"},
			{
				ID: "broken-diagram", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramFlow, Language: "mermaid",
					Body: "flowchart TD\n ] -->|\n codraxNode1[broken] | codraxNode2[target]",
				},
			},
		},
	}
	prevJSON, err := json.Marshal(prevDoc)
	if err != nil {
		t.Fatalf("marshal prev doc: %v", err)
	}
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	ctx.Mutable.SetRetryState(&types.RetryState{Attempt: 6, PrevEmitJSON: prevJSON})

	// Single-shot markdown output keeps valid Mermaid source for downstream
	// viewers, so pin the degraded-only guard under that real production mode.
	render.SetMermaidRenderingEnabled(false)
	t.Cleanup(func() { render.SetMermaidRenderingEnabled(true) })
	out, err := (&answerDocumentEvaluator{language: "zh"}).ParseOutput(
		ctx, []llm.Message{{Role: "assistant", Content: "最终 patch 未通过"}}, nil, nil,
	)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"恢复正文",
		"```text\n# ⚠ 恢复稿中的 Mermaid 未通过语法校验",
		"] -->|",
		"最终重试未能产出有效的 answer_document",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("degraded output missing %q:\n%s", want, out.FinalAnswer)
		}
	}
	if strings.Contains(out.FinalAnswer, "```mermaid") {
		t.Fatalf("invalid recovered diagram must not ship as Mermaid:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_RendersLastRejectedDraft(t *testing.T) {
	rejected := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "最近一版结构化草稿正文",
		}},
		Citations: []types.Citation{{File: "internal/agent/agent.go", Line: 970}},
	}
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	ctx.Mutable.SetLastRejectedAnswerDocumentV2(rejected)
	messages := []llm.Message{
		{Role: "assistant", Content: "<think>仍在修 citation_ref</think>"},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "未能生成结构化答案") {
		t.Fatalf("last rejected draft recovery should avoid raw-only missing-doc banner:\n%s", out.FinalAnswer)
	}
	for _, want := range []string{
		"最近一版结构化草稿正文",
		"internal/agent/agent.go:970",
		"最终重试未能产出有效的 answer_document",
		"失败修补回合的临时推理未包含在答案中",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
	if strings.Contains(out.FinalAnswer, "<think>") || strings.Contains(out.FinalAnswer, "仍在修 citation_ref") ||
		strings.Contains(out.FinalAnswer, "模型最后一轮原文") {
		t.Fatalf("thinking/repair scratch must not leak from last rejected turn:\n%s", out.FinalAnswer)
	}
	if doc := ctx.Mutable.AnswerDocumentV2(); doc != nil {
		t.Fatalf("last rejected draft recovery should not mark rejected draft as accepted mutable document: %+v", doc)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_TextRecoveryKeepsRetryStateDiagram(t *testing.T) {
	prevDoc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "summary",
				Kind: types.BlockSummary,
				Text: "上一版结构化答案正文",
			},
			{
				ID:    "diagram",
				Kind:  types.BlockDiagram,
				Title: "执行图",
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramFlow,
					Language: "mermaid",
					Body:     "flowchart TD\n  A[Agent] --> B[Tool]",
				},
			},
		},
	}
	prevJSON, err := json.Marshal(prevDoc)
	if err != nil {
		t.Fatalf("marshal prev doc: %v", err)
	}
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	ctx.Mutable.SetRetryState(&types.RetryState{
		Attempt:      2,
		PrevEmitJSON: prevJSON,
	})
	messages := []llm.Message{{
		Role: "assistant",
		Content: `{
			"blocks": [
				{"id": "summary", "kind": "summary", "text": "文本恢复正文"}
			]
		}`,
	}}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"文本恢复正文",
		"执行图",
		"```mermaid",
		"flowchart TD",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_SanitizesFallback(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	messages := []llm.Message{
		{
			Role: "assistant",
			Content: "<think>internal reasoning</think>\n\n" +
				"Grounded user-facing answer.\n\n" +
				"<minimax:tool_call>\n" +
				"{\"shape\":\"explanation\"}\n" +
				"</minimax:tool_call>\n",
		},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "<think>") || strings.Contains(out.FinalAnswer, "<minimax:tool_call>") || strings.Contains(out.FinalAnswer, "\"shape\"") {
		t.Fatalf("fail-loud fallback leaked internal scaffolding: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "Grounded user-facing answer.") {
		t.Fatalf("sanitized fallback lost user-facing content: %q", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_SynthesizesAuthorityCaveatFromContextEvidence(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{{
			ID:        "ev1",
			Source:    "internal/agent/analyzer.go",
			LineStart: 981,
			Authority: types.AuthorityHistorical,
		}},
	}
	messages := []llm.Message{
		{Role: "assistant", Content: "raw fallback text"},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, render.AuthorityCaveatTag()) {
		t.Fatalf("fallback output leaked private authority tag: %q", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, "Authority:") {
		t.Fatalf("fallback output should hide authority caveat from user surface: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "raw fallback text") {
		t.Fatalf("fallback output lost raw answer body: %q", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_EmptyResponseUsesEvidenceFallback(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev1",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/agent/answer_document_evaluator.go",
			LineStart:       7000,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ParseOutput",
			Subject:         "ParseOutput",
			Summary:         "成文输出缺失时只展示已落地证据，不补写结论",
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, []llm.Message{{Role: "assistant"}}, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"模型未完成成文",
		"internal/agent/answer_document_evaluator.go:7000",
		"ParseOutput",
		"不补写结论",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("fallback answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
	if !out.AnswerDegraded || !out.SkipAnswerChecks || out.DegradeReason != "answer_document_missing" {
		t.Fatalf("empty-response evidence fallback must be marked degraded and skip structured checks, got degraded=%t skip=%t reason=%q",
			out.AnswerDegraded, out.SkipAnswerChecks, out.DegradeReason)
	}
}

func TestAnswerDocumentEvaluator_ParseOutputV2_AuthorityCaveatUsesMergedEvidencePool(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{{
			ID:        "ev1",
			Source:    "internal/agent/analyzer.go",
			LineStart: 981,
			Authority: types.AuthorityHistorical,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary body"},
		},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out := &StageOutput{}
	got, err := e.parseOutputV2(ctx, doc, out)
	if err != nil {
		t.Fatalf("parseOutputV2 err: %v", err)
	}
	if strings.Contains(got.FinalAnswer, render.AuthorityCaveatTag()) {
		t.Fatalf("rendered V2 answer leaked private authority tag:\n%s", got.FinalAnswer)
	}
	if strings.Contains(got.FinalAnswer, "Authority:") {
		t.Fatalf("rendered V2 answer should hide authority caveat from user surface:\n%s", got.FinalAnswer)
	}
	if !strings.Contains(got.FinalAnswer, "summary body") {
		t.Fatalf("rendered V2 answer lost summary body:\n%s", got.FinalAnswer)
	}
	foundStructuredCaveat := false
	for _, blk := range doc.Blocks {
		if blk.Kind == types.BlockCaveat && strings.Contains(blk.Text, render.AuthorityCaveatTag()) {
			foundStructuredCaveat = true
			break
		}
	}
	if !foundStructuredCaveat {
		t.Fatalf("authority hedging should remain on the structured doc for downstream checks; doc=%+v", doc.Blocks)
	}
}

func TestRenderReadNavigationCoverageSupplement(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		ReadNavigationCoverage: &types.RepoMapNavigationCoverage{
			State:          types.RepoMapNavigationCoveragePartial,
			ReasonCode:     "repo_map_navigation_partial",
			RequiredRoutes: []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap, types.RepoMapNavigationRouteRelationMap},
			ObservedRoutes: []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap},
			CoveredRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap},
			MissingRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteRelationMap},
			EvidenceRefs:   []string{"blob://repo-map-task"},
		},
	}
	got := renderReadNavigationCoverageSupplement(nil, doc, "en")
	for _, want := range []string{
		"repo_map navigation coverage",
		"`partial`",
		"`task_map`",
		"`relation_map`",
		"blob://repo-map-task",
		"not semantic source citation",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("navigation coverage supplement missing %q:\n%s", want, got)
		}
	}
}

func TestRenderReadLocalizerFollowupSupplement(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		ReadLocalizerFollowup: &types.ReadLocalizerFollowup{
			State:                types.ReadLocalizerFollowupNeeded,
			ReasonCode:           "read_localizer_owner_and_navigation_missing",
			CandidatePaths:       []string{"pkg/handler.py"},
			MissingRoutes:        []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteRelationMap},
			EvidenceRequirements: []string{"localization_requirement path=pkg/handler.py kind=owner_evidence required=typed_owner_localization_anchor"},
		},
	}
	got := renderReadLocalizerFollowupSupplement(nil, doc, "en")
	for _, want := range []string{
		"read localizer follow-up",
		"`needed`",
		"pkg/handler.py",
		"`relation_map`",
		"typed_owner_localization_anchor",
		"does not replace the answer",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("localizer follow-up supplement missing %q:\n%s", want, got)
		}
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_CardinalityDowngrade is the
// critical P2.2 test: when Shape == list_of_symbols + Completeness ==
// complete + len(Symbols) < baseline, ParseOutput downgrades to
// lower_bound and appends a caveat. Reuses the same validator as
// extractorEvaluator so this test pins the cross-stage contract.

// TestAnswerDocumentEvaluator_ParseOutput_NoDowngrade — completeness
// complete with enough symbols passes through unchanged.

// richDraftProse is a ~1500-char explanation the LLM writes as plain
// prose in its first attempt — the kind of answer the shrinkage
// salvage must preserve when the correction retry emits a compressed
// paraphrase. Kept as a test helper so every shrinkage case shares
// one fixture and the length floor matches the 400-char threshold.
func richDraftProse() string {
	return strings.Repeat(
		"The dispatcher walks the handler chain and delegates to the registered listener. ",
		20) // 20 * ~78 chars = ~1560 chars, well above the 400-char floor
}

func TestSanitizePriorDraftForSummary_StripsInternalScaffolding(t *testing.T) {
	cleanTail := strings.Repeat("入口函数负责把请求整理成结构化 IR。", 40)
	in := `<think>internal reasoning</think>

Translation: explain the answer

` + "```json\n{\"shape\":\"value\",\"summary\":\"x\",\"citations\":[]}\n```" + `

<minimax:tool_call>
<invoke name="emit_answer_document">
<parameter name="shape">value</parameter>
</invoke>
</minimax:tool_call>

` + cleanTail
	got := sanitizePriorDraftForSummary(in)
	if strings.Contains(got, "<think>") || strings.Contains(got, "emit_answer_document") ||
		strings.Contains(got, "\"shape\":") || strings.Contains(got, "<minimax:tool_call>") {
		t.Fatalf("sanitizePriorDraftForSummary leaked internal scaffolding: %q", got)
	}
	if !strings.Contains(got, "结构化 IR") {
		t.Fatalf("sanitizePriorDraftForSummary dropped user-facing tail: %q", got)
	}
}

func TestSanitizePriorDraftForSummary_PreservesNaturalLanguageSelfTalk(t *testing.T) {
	in := "Translation: explain the answer\n\nI need to emit exactly one emit_answer_document tool call.\n\n用户可见答案。"
	got := sanitizePriorDraftForSummary(in)
	for _, want := range []string{"Translation:", "I need to emit", "emit_answer_document", "用户可见答案"} {
		if !strings.Contains(got, want) {
			t.Fatalf("sanitizePriorDraftForSummary should preserve natural-language paragraph %q, got %q", want, got)
		}
	}
}

func TestLooksLikeInternalDraftParagraph_StripsStructuredJSONOnly(t *testing.T) {
	if !looksLikeInternalDraftParagraph(`{"shape":"value","summary":"x","citations":[]}`) {
		t.Fatal("answer-document JSON payload should be filtered")
	}
	if looksLikeInternalDraftParagraph(`{"unrelated":"shape","summary":"domain term"}`) {
		t.Fatal("unrelated JSON object must not be filtered as answer-document payload")
	}
	if looksLikeInternalDraftParagraph("The response structure is part of this codebase's public API.") {
		t.Fatal("natural-language prose must not be filtered by schema-ish keywords")
	}
}

// TestFindLastPreToolCallDraft_IgnoresToolCallTurns — the helper
// must SKIP assistant messages that have tool calls, since those
// represent "tool call fired" turns, not pre-tool-call drafts. The
// target is the draft from BEFORE the emit landed.
func TestFindLastPreToolCallDraft_IgnoresToolCallTurns(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "q"},
		{Role: "assistant", Content: "pre-tool-call draft"}, // no ToolCalls
		{Role: "user", Content: "hint"},
		{Role: "assistant", Content: "tool-call turn preamble", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit"}}},
	}
	got := findLastPreToolCallDraft(messages)
	if got != "pre-tool-call draft" {
		t.Errorf("findLastPreToolCallDraft = %q, want pre-tool-call draft", got)
	}
}

// TestAnswerDocumentEvaluator_DetermineMissingPiece — always returns
// MissingNone, matching the legacy finalizer contract.
func TestAnswerDocumentEvaluator_DetermineMissingPiece(t *testing.T) {
	e := &answerDocumentEvaluator{}
	if got := e.DetermineMissingPiece(nil, nil); got != types.MissingNone {
		t.Errorf("DetermineMissingPiece = %q, want MissingNone", got)
	}
}

// TestAnswerDocumentSkill_DeclaresEmitTool pins the P2.2 cleanup
// contract: the declarative answer-document-skill in
// internal/skill/defaults.go MUST declare emit_answer_document in
// its ToolSuggestions. This replaces the pre-cleanup approach of
// patching the two legacy finalize skills' ToolSuggestions at runtime
// in cmd/root.go, which would have left their contradictory
// Answer/Evidence markdown OutputFormat in the prompt.
func TestAnswerDocumentSkill_DeclaresEmitTool(t *testing.T) {
	reg := skill.NewRegistry()
	skill.RegisterDefaults(reg)
	sk, err := reg.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("answer-document-skill not registered by RegisterDefaults: %v", err)
	}
	found := false
	for _, name := range sk.ToolSuggestions {
		if name == "emit_answer_document" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("answer-document-skill.ToolSuggestions missing emit_answer_document: %v",
			sk.ToolSuggestions)
	}
	// Sanity checks: the skill must NOT accidentally declare the
	// legacy finalize skills' tools (todo_write etc.) which would
	// reintroduce the prose-writing pathway. The expanded set is
	// {emit_answer_document, emit_answer_document_patch} — patch
	// is the protocol-level retry preservation tool used on retry
	// paths only.
	allowed := map[string]bool{
		"emit_answer_document":       true,
		"emit_answer_document_patch": true,
	}
	for _, name := range sk.ToolSuggestions {
		if !allowed[name] {
			t.Errorf("answer-document-skill ToolSuggestions has unexpected entry %q (allowed: %v)",
				name, allowed)
		}
	}
}

// TestAnswerDocAttachEscalation pins B2-F4's retry escalation
// contract: same-issue retry text must visibly differ between
// attempts so the LLM knows it's being re-prompted on a
// persisted failure rather than reading a fresh issue.
//
//	attempt 1 → no escalation (first time hitting this issue)
//	attempt 2 → "RETRY ... your previous fix did not address it"
//	attempt 3+ → "FINAL RETRY ... fix NOW or the answer ships with violation"
//
// The dedup-key contract (rejectHintsUsed embedded in HintKey) is
// orthogonal — it ensures each retry actually delivers the hint;
// this test ensures the hint TEXT escalates.
func TestAnswerDocAttachEscalation(t *testing.T) {
	hint := "Attach claim_use to block X."
	cases := []struct {
		attempt        int
		mustEqual      string
		mustNotContain []string
		mustContain    []string
	}{
		{
			attempt:        1,
			mustEqual:      hint, // no escalation
			mustNotContain: []string{"RETRY", "FINAL RETRY"},
		},
		{
			attempt:        2,
			mustContain:    []string{"RETRY", "2nd attempt", "previous fix did not address"},
			mustNotContain: []string{"FINAL RETRY"},
		},
		{
			attempt:     3,
			mustContain: []string{"FINAL RETRY", "fix the named field NOW", "ships with the violation"},
		},
		{
			attempt:     5,
			mustContain: []string{"FINAL RETRY", "attempt #5"},
		},
	}
	for _, tc := range cases {
		got := answerDocAttachEscalation(hint, tc.attempt)
		if tc.mustEqual != "" && got != tc.mustEqual {
			t.Errorf("attempt=%d: got %q, want %q", tc.attempt, got, tc.mustEqual)
		}
		for _, want := range tc.mustContain {
			if !strings.Contains(got, want) {
				t.Errorf("attempt=%d: missing %q in %q", tc.attempt, want, got)
			}
		}
		for _, banned := range tc.mustNotContain {
			if strings.Contains(got, banned) {
				t.Errorf("attempt=%d: leaks %q in %q", tc.attempt, banned, got)
			}
		}
	}
}

// ── R7 typed-set 字段值 verbatim 渲染 (post_shape_residual_audit
// 2026-05-04) ─────────────────────────────────────────────────────

// TestRenderAnswerDocBlockRequirement_VerbatimTypedSets pins R7's
// fix: BlockRequirement.FacetIDs / AcceptableClaimForms /
// SurfaceRoleHint must each render as JSON-ready string lists the
// LLM can copy verbatim into block.facet_ids[] / block.claim_uses[].claim_form
// / surface_role.
//
// Pre-R7 the prose only said "covers facet(s): X" without
// instructing the LLM to copy "X" into the emit's typed field, so
// the LLM 100% missed HARD facets in eval verification (4/4 runs).
func TestRenderAnswerDocBlockRequirement_VerbatimTypedSets(t *testing.T) {
	req := types.BlockRequirement{
		Kind:                 types.BlockSection,
		MinCount:             1,
		MaxCount:             1,
		Required:             true,
		Rationale:            "the principal explanation surface",
		FacetIDs:             []string{"current_code_path", "component_relation"},
		AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge},
		SurfaceRoleHint:      types.SurfacePrincipal,
	}
	var b strings.Builder
	renderAnswerDocBlockRequirement(&b, req, nil, true)
	got := b.String()

	// Verbatim FacetID strings (LLM copies into block.facet_ids[]).
	for _, want := range []string{
		"`block.facet_ids` MUST include",
		`"current_code_path"`,
		`"component_relation"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing verbatim facet_ids token %q in:\n%s", want, got)
		}
	}
	// Verbatim ClaimForm strings.
	for _, want := range []string{
		"`block.claim_uses[]` entry's `claim_form` MUST be one of",
		`"definition_fact"`,
		`"call_edge"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing verbatim claim_form token %q in:\n%s", want, got)
		}
	}
	// Verbatim SurfaceRole.
	if !strings.Contains(got, `"principal"`) {
		t.Errorf("missing verbatim surface_role 'principal' in:\n%s", got)
	}
	if !strings.Contains(got, "`block.surface_role`") {
		t.Errorf("missing surface_role field name in:\n%s", got)
	}
}

func TestRenderAnswerDocBlockRequirement_RendersAlternativeKinds(t *testing.T) {
	req := types.BlockRequirement{
		Kind:             types.BlockOrderedList,
		AlternativeKinds: []types.AnswerBlockKind{types.BlockTable, types.BlockBulletList},
		MinCount:         1,
		Required:         true,
		Rationale:        "enumeration carrier",
	}
	var b strings.Builder
	renderAnswerDocBlockRequirement(&b, req, nil, true)
	got := b.String()
	if !strings.Contains(got, "**ordered_list/table/bullet_list**") {
		t.Fatalf("required-block prompt should render equivalent carrier kinds, got:\n%s", got)
	}
	if !strings.Contains(got, "enumeration carrier") {
		t.Fatalf("rationale should still render, got:\n%s", got)
	}
}

func TestRenderAnswerDocBlockRequirement_TypedDecisionCarrierDoesNotRequireClaimUse(t *testing.T) {
	req := types.BlockRequirement{
		Kind:                 types.BlockDecision,
		MinCount:             1,
		MaxCount:             1,
		Required:             true,
		AcceptableClaimForms: []types.ClaimForm{types.ClaimGuardCondition, types.ClaimDefinitionFact},
		SurfaceRoleHint:      types.SurfacePrincipal,
	}
	view := &types.AnswerSemanticView{
		CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
	}
	var b strings.Builder
	renderAnswerDocBlockRequirement(&b, req, view, true)
	got := b.String()
	if strings.Contains(got, "`block.claim_uses[]` entry's `claim_form` MUST be one of") {
		t.Fatalf("typed decision carrier should not force claim_uses:\n%s", got)
	}
	for _, want := range []string{
		"The active typed decision verdict field is the carrier",
		"`block.claim_uses[]` is optional",
		`"guard_condition"`,
		`"definition_fact"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed decision requirement missing %q:\n%s", want, got)
		}
	}
}

// TestRenderAnswerDocBlockRequirement_OmitsTypedSetsWhenEmpty
// confirms zero-value fields are NOT rendered (no empty
// "MUST include []" lines).
func TestRenderAnswerDocBlockRequirement_OmitsTypedSetsWhenEmpty(t *testing.T) {
	req := types.BlockRequirement{
		Kind:     types.BlockSummary,
		MinCount: 1, MaxCount: 1,
		Rationale: "lead-in",
	}
	var b strings.Builder
	renderAnswerDocBlockRequirement(&b, req, nil, true)
	got := b.String()
	for _, banned := range []string{
		"`block.facet_ids` MUST include",
		"`block.claim_uses[]` entry's `claim_form` MUST be one of",
		"`block.surface_role` SHOULD be",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("empty-field requirement should not render %q in:\n%s", banned, got)
		}
	}
}

// TestRenderAnswerDocFacetCoverage_VerbatimFacetIDAndOwnerBlock
// confirms each facet line carries `facet_id: "X"` verbatim AND
// the reverse-lookup "Place on block kind: ..." hint when the
// view's BlockRequirements list which block covers the facet.
func TestRenderAnswerDocFacetCoverage_VerbatimFacetIDAndOwnerBlock(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
			},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	// verbatim facet_id string
	if !strings.Contains(got, `facet_id: "current_code_path"`) {
		t.Errorf("missing verbatim facet_id for current_code_path:\n%s", got)
	}
	// preamble teaching that the value is what to copy into emit
	if !strings.Contains(got, "`block.facet_ids` (or `block.claim_uses[j].facet_id`)") {
		t.Errorf("missing R7 declarative preamble:\n%s", got)
	}
	// reverse-lookup hint (architecture family has BlockRequirement
	// with FacetIDs, so the reverse map is populated)
	if strings.Contains(got, "Place on block kind:") == false {
		t.Errorf("missing R7 reverse-lookup hint:\n%s", got)
	}
}

// TestRenderQuotedList_FormatStability pins the JSON-ready output
// shape (LLM-friendly format).
func TestRenderQuotedList_FormatStability(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"a"}, `["a"]`},
		{[]string{"a", "b"}, `["a", "b"]`},
		{[]string{"  a  ", "b"}, `["a", "b"]`}, // trim whitespace
		{[]string{"", "a"}, `["a"]`},           // skip empty
	}
	for _, tc := range cases {
		if got := renderQuotedList(tc.in); got != tc.want {
			t.Errorf("renderQuotedList(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ── R14-c9 retry-state rendering lock tests
// (post_shape_residual_audit.md, 2026-05-04) ──────────────────────

// TestRenderAnswerDocRetryState_NilOrEmpty pins the byte-identity
// invariant: fresh dispatches (no retry state on Mutable) must
// render empty so the prompt is byte-identical to pre-R14.
func TestRenderAnswerDocRetryState_NilOrEmpty(t *testing.T) {
	if got := renderAnswerDocRetryState(nil); got != "" {
		t.Errorf("nil ctx: got %q, want empty", got)
	}
	ctx := &types.AgentContext{}
	if got := renderAnswerDocRetryState(ctx); got != "" {
		t.Errorf("ctx without Mutable: got %q, want empty", got)
	}
	ctx = &types.AgentContext{Mutable: &types.MutableState{}}
	if got := renderAnswerDocRetryState(ctx); got != "" {
		t.Errorf("Mutable without RetryState: got %q, want empty", got)
	}
	mut := &types.MutableState{}
	mut.SetRetryState(&types.RetryState{Attempt: 0})
	ctx = &types.AgentContext{Mutable: mut}
	if got := renderAnswerDocRetryState(ctx); got != "" {
		t.Errorf("Attempt=0 RetryState: got %q, want empty", got)
	}
}

func TestRenderAnswerDocRetryState_NoPreviousEmitAsksFullDocument(t *testing.T) {
	mut := &types.MutableState{}
	mut.SetRetryState(&types.RetryState{
		Attempt: 1,
		ActiveViolations: []types.ScoredViolation{{
			Kind:      types.ViolBlockCoverageMissing,
			Severity:  types.SeverityHigh,
			Layer:     "v2_oracle",
			FieldPath: "blocks[]",
			Detail:    "blocks[] is required and must be non-empty",
			Repair:    "emit a complete document",
		}},
	})
	ctx := &types.AgentContext{Mutable: mut}
	got := renderAnswerDocRetryState(ctx)
	for _, want := range []string{
		"## Hard Rule (retry attempt 1)",
		"did not leave a usable structured answer document",
		"complete `emit_answer_document` payload",
		"Do NOT use patch operations",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("retry state missing %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{
		"starting from your previous payload",
		"byte-identical to your Previous Emit",
		"## Previous Emit",
	} {
		if strings.Contains(got, bad) {
			t.Fatalf("retry state without previous emit should not mention %q:\n%s", bad, got)
		}
	}
}

// TestRenderAnswerDocRetryState_FullPayload pins the 4 required
// sections + key invariants:
//   - Hard Rule referenced first
//   - Required Changes groups actionable fixes by repair phase
//   - Active Violations groups by Severity (incl Soft as telemetry)
//   - Previous Emit shows block-level claim_use + facet_ids verbatim
func TestRenderAnswerDocRetryState_FullPayload(t *testing.T) {
	rs := &types.RetryState{
		Attempt: 2,
		PrevEmitSummary: types.RetryStateSummary{
			CitationsCount: 5,
			CitationFiles:  []string{"a.go", "b.go"},
			BlockSummaries: []types.RetryBlockSummary{
				{
					ID: "lifecycle", Kind: types.BlockSection,
					SurfaceRole: types.SurfacePrincipal,
					FacetIDs:    []string{"current_code_path"},
					HasClaimUse: true,
					ClaimForm:   types.ClaimDefinitionFact,
				},
				{
					ID: "items_block", Kind: types.BlockOrderedList,
					HasItems: true, ItemCount: 3,
					ItemsWithCitation: 3,
				},
			},
			HasExactResolution: false,
		},
		ActiveViolations: []types.ScoredViolation{
			{
				Kind:      types.ViolPrincipalClaimUseMissing,
				Detail:    `principal block id="lifecycle" kind=section has no claim_use`,
				Repair:    "emit claim_use on the block",
				Severity:  types.SeverityCritical,
				Layer:     "v2_oracle",
				BlockID:   "lifecycle",
				FieldPath: `blocks[id="lifecycle"].claim_use`,
			},
			{
				Kind:      types.ViolFacetUncovered,
				Detail:    `required facet "diagram_spine" not covered`,
				Repair:    `declare facet_id="diagram_spine"`,
				Severity:  types.SeverityHigh,
				Layer:     "v2_oracle",
				FieldPath: "blocks[*].facet_ids",
			},
			{
				Kind:     types.ViolRichnessRegression,
				Detail:   `optional facet "uncertainty_boundary" not surfaced`,
				Severity: types.SeveritySoft,
				Layer:    "v2_oracle",
			},
		},
	}
	mut := &types.MutableState{}
	mut.SetRetryState(rs)
	ctx := &types.AgentContext{Mutable: mut}
	got := renderAnswerDocRetryState(ctx)

	// 1. Hard Rule
	for _, want := range []string{
		"## Hard Rule (retry attempt 2)",
		"If `emit_answer_document_patch` is available, prefer it",
		"`unchanged_block_ids`",
		"byte-identical to your Previous Emit",
		"Do NOT regenerate from scratch",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Hard Rule missing %q in:\n%s", want, got)
		}
	}

	// 2. Required Changes — phase partitioned; Soft NOT in this section
	cIdx := strings.Index(got, "## Required Changes")
	if cIdx < 0 {
		t.Fatal("Required Changes section missing")
	}
	shapeIdx := strings.Index(got, "**Payload shape**")
	coverageIdx := strings.Index(got, "**Requested topic and coverage**")
	if shapeIdx <= 0 || coverageIdx <= 0 || shapeIdx > coverageIdx {
		t.Errorf("Payload shape fixes must precede requested-topic fixes in Required Changes")
	}
	if !strings.Contains(got, `blocks[id="lifecycle"].claim_use`) {
		t.Errorf("missing typed FieldPath in Required Changes:\n%s", got[cIdx:cIdx+800])
	}
	// Soft not in Required Changes section (between Required and Active sections).
	requiredSection := got[cIdx:strings.Index(got, "## Active Violations")]
	if strings.Contains(requiredSection, "uncertainty_boundary") {
		t.Errorf("Soft violation must not appear in Required Changes:\n%s", requiredSection)
	}

	// 3. Active Violations
	if !strings.Contains(got, "## Active Violations (typed, by severity + layer)") {
		t.Error("Active Violations section header missing")
	}
	// Soft IS shown in Active Violations with telemetry tag
	if !strings.Contains(got, "uncertainty_boundary") {
		t.Error("Soft violation missing in Active Violations")
	}
	if !strings.Contains(got, "*(telemetry only)*") {
		t.Error("Soft violation must carry telemetry-only tag")
	}

	// 4. Previous Emit
	for _, want := range []string{
		"## Previous Emit (preserve every field below byte-identical",
		`Block id="lifecycle" kind=section surface_role="principal"`,
		`facet_ids: ["current_code_path"]`,
		`claim_use: present (claim_form="definition_fact")`,
		`Block id="items_block" kind=ordered_list`,
		"items: 3 total, 3 with citation",
		"Citations: 5 entries (top files: a.go, b.go)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Previous Emit missing %q in:\n%s", want, got)
		}
	}
}

// TestRenderRetryRequiredChanges_GroupsByPhaseThenSeverity verifies
// phase partitioning, per-phase Critical -> High -> Medium ordering,
// and Soft exclusion.
func TestRenderRetryRequiredChanges_GroupsByPhaseThenSeverity(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{Kind: types.ViolMustInclude, Severity: types.SeverityMedium, Detail: "consistency med"},
			{Kind: types.ViolBlockCoverageMissing, Severity: types.SeverityMedium, Detail: "shape med"},
			{Kind: types.ViolPrincipalClaimUseMissing, Severity: types.SeverityCritical, Detail: "shape crit"},
			{Kind: types.ViolFacetUncovered, Severity: types.SeverityHigh, Detail: "coverage high"},
			{Kind: types.ViolRichnessGlaringGap, Severity: types.SeverityMedium, Detail: "richness med"},
			{Kind: types.ViolRichnessRegression, Severity: types.SeveritySoft, Detail: "soft"},
		},
	}
	got := renderRetryRequiredChanges(rs)
	shapeIdx := strings.Index(got, "**Payload shape**")
	consistencyIdx := strings.Index(got, "**Grounding and consistency**")
	coverageIdx := strings.Index(got, "**Requested topic and coverage**")
	richnessIdx := strings.Index(got, "**Supported enrichment**")
	if shapeIdx < 0 || consistencyIdx < 0 || coverageIdx < 0 || richnessIdx < 0 {
		t.Fatalf("missing phase sections; got:\n%s", got)
	}
	if !(shapeIdx < consistencyIdx && consistencyIdx < coverageIdx && coverageIdx < richnessIdx) {
		t.Errorf("phase ordering broken: shape=%d consistency=%d coverage=%d richness=%d", shapeIdx, consistencyIdx, coverageIdx, richnessIdx)
	}
	critIdx := strings.Index(got, "shape crit")
	medIdx := strings.Index(got, "shape med")
	if critIdx < 0 || medIdx < 0 || critIdx > medIdx {
		t.Errorf("severity ordering inside phase broken:\n%s", got)
	}
	if strings.Contains(got, "soft") {
		t.Errorf("Soft must not appear in Required Changes:\n%s", got)
	}
}

func TestRenderRetryRequiredChanges_TopicMismatchSuppressesEnrichment(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{Kind: types.ViolAnswerTopicMismatch, Severity: types.SeverityHigh, Detail: "wrong subject"},
			{Kind: types.ViolAnswerSemanticUnderfilled, Severity: types.SeverityMedium, Detail: "thin explanation"},
			{Kind: types.ViolRichnessGlaringGap, Severity: types.SeverityMedium, Detail: "missing optional detail"},
			{Kind: types.ViolPrincipalClaimUseMissing, Severity: types.SeverityCritical, Detail: "missing claim use"},
		},
	}
	got := renderRetryRequiredChanges(rs)
	topicIdx := strings.Index(got, "wrong subject")
	shapeIdx := strings.Index(got, "missing claim use")
	if topicIdx < 0 {
		t.Fatalf("topic mismatch must stay actionable:\n%s", got)
	}
	if shapeIdx < 0 {
		t.Fatalf("critical structural blocker must stay actionable:\n%s", got)
	}
	if topicIdx > shapeIdx {
		t.Errorf("topic mismatch should lead the repair when present:\n%s", got)
	}
	for _, banned := range []string{"thin explanation", "missing optional detail"} {
		if strings.Contains(got, banned) {
			t.Errorf("topic-mismatch repair must not bundle enrichment %q:\n%s", banned, got)
		}
	}
}

// G7 (2026-06-12): a HARD facet downgraded to SOFT renders an
// explicit downgrade annotation when the typed facet_softened
// telemetry names its kind — and stays silent for born-SOFT facets.
func TestRenderAnswerDocFacetCoverage_SoftenedAnnotation(t *testing.T) {
	// Non-empty evidence surface whose rows match none of the
	// config-precedence acceptable forms — the real compile path
	// downgrades the HARD facet to SOFT and records the typed
	// facet_softened telemetry this annotation reads.
	mu := types.NewMutableState("q")
	mu.AppendEvidence([]types.EvidenceItem{
		{Kind: types.EvidenceRegistration, Subject: "Register", Object: "X", Source: "a.go", LineStart: 1},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentConfigQuery},
			AnswerContract: types.AnswerContract{},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	if !strings.Contains(got, "downgraded from HARD") {
		t.Fatalf("softened facet must carry the downgrade annotation\n----\n%s", got)
	}

	// Without the telemetry signal the annotation must not appear.
	plain := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentConfigQuery},
			AnswerContract: types.AnswerContract{},
		},
	}
	if strings.Contains(renderAnswerDocFacetCoverage(plain), "downgraded from HARD") {
		t.Fatalf("born-SOFT facets must not be annotated as downgraded")
	}
}

func TestRenderAnswerDocFacetCoverage_ExplicitDiagramCarrierDoesNotPublishStaleSoftening(t *testing.T) {
	mu := types.NewMutableState("q")
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:         "definition-only",
		Source:     "pipeline.go",
		LineStart:  1,
		AnchorKind: types.AnchorDefinition,
	}})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentTrace,
				Scenario:      types.ScenarioArchitectureExplain,
				PredicateAxis: types.AxisCall,
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
				DiagramHint: &types.DiagramHint{
					Kind:     types.DiagramSequence,
					Required: true,
				},
			},
		},
	}

	got := renderAnswerDocFacetCoverage(ctx)
	diagramLine := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, `facet_id: "diagram_spine"`) {
			diagramLine = line
			break
		}
	}
	if diagramLine == "" || !strings.Contains(diagramLine, "**HARD**") {
		t.Fatalf("explicit diagram carrier must remain visible as HARD:\n%s", got)
	}
	if strings.Contains(diagramLine, "downgraded from HARD") {
		t.Fatalf("explicit diagram carrier must not publish stale softening:\n%s", got)
	}
	for _, sig := range mu.RichnessTelemetry() {
		if sig.Kind == "facet_softened" && sig.FacetKind == string(types.FacetDiagramSpine) {
			t.Fatalf("explicit diagram carrier must not enter degradation telemetry: %+v", sig)
		}
	}
}

// TestAnswerDocumentEvaluator_EmptyBlocksRejectBreaker pins the F7
// same-cause breaker: three identical answer_doc_blocks_required rejects
// are hinted normally, the fourth force-stops the loop so the recovery
// chain ships the retained snapshot; any success or different reject code
// resets the streak; the empty-blocks reject itself is never accepted by
// the breaker (it only stops paying retries).
func TestAnswerDocumentEvaluator_EmptyBlocksRejectBreaker(t *testing.T) {
	mut := types.NewMutableState("q")
	ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mut}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)

	emptyReject := func() LoopObservation {
		return LoopObservation{
			Phase: PhaseMidLoop,
			LastToolResult: &types.ToolResult{
				ToolName: "emit_answer_document",
				Success:  false,
				Repair:   &types.ToolRepair{Code: types.ToolRepairCodeAnswerDocBlocksRequired},
			},
		}
	}

	for i := 1; i <= 3; i++ {
		sig := e.Observe(ctx, emptyReject())
		if sig.StopRequested {
			t.Fatalf("reject #%d must not trip the breaker yet: %+v", i, sig)
		}
	}
	sig := e.Observe(ctx, emptyReject())
	if !sig.StopRequested || !strings.Contains(sig.StopReason, "empty-blocks reject breaker") {
		t.Fatalf("4th identical empty-blocks reject must trip the breaker, got %+v", sig)
	}

	// A different reject code resets the streak.
	e2 := &answerDocumentEvaluator{}
	_ = e2.BuildInitialInstruction(ctx, nil)
	for i := 0; i < 3; i++ {
		_ = e2.Observe(ctx, emptyReject())
	}
	_ = e2.Observe(ctx, LoopObservation{
		Phase: PhaseMidLoop,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Repair:   &types.ToolRepair{Code: "missing_diagram"},
		},
	})
	if sig := e2.Observe(ctx, emptyReject()); sig.StopRequested {
		t.Fatalf("streak must reset on a different reject code, got %+v", sig)
	}

	// A snapshot appearing mid-streak changes the fingerprint and
	// restarts the count (situation changed — patch recovery is now
	// possible and worth hinting about).
	e3 := &answerDocumentEvaluator{}
	_ = e3.BuildInitialInstruction(ctx, nil)
	for i := 0; i < 3; i++ {
		_ = e3.Observe(ctx, emptyReject())
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "retained"}},
	})
	// NOTE: with an accepted doc on Mutable the MidLoop fast path stops
	// with "emit_answer_document called" before the breaker — which is
	// correct loop behavior (a valid doc exists). Assert the breaker
	// state was not what stopped it.
	sig = e3.Observe(ctx, emptyReject())
	if sig.StopRequested && strings.Contains(sig.StopReason, "empty-blocks reject breaker") {
		t.Fatalf("fingerprint change must restart the streak, got breaker trip: %+v", sig)
	}
}

// --- PTV5 #68 (用户裁定 2026-07-05) supplement pins ---------------------------

// TestPTV5TraceQuerySupplementTruncationDisclosure (C38): the supplement block
// self-describes as the auditable-fact keeper — when the 40-row cap trims the
// list, the tail states the full count; under the cap the block stays
// byte-identical (突变形态).
func TestPTV5TraceQuerySupplementTruncationDisclosure(t *testing.T) {
	build := func(n int) *types.AgentContext {
		mu := types.NewMutableState("")
		var obs []types.ObservationRecord
		for i := 0; i < n; i++ {
			obs = append(obs, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:sup:%d", i),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				Role:            types.AnswerAggregateRoleSupportingCoverage,
				GroundingPolicy: types.ClaimGroundingHard,
				SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
				Span:            types.ObservationSpan{LineStart: 100 + i*10, LineEnd: 105 + i*10},
				ClaimKey:        fmt.Sprintf("state_drilldown:t%d:S", i),
				Subject:         fmt.Sprintf("t%d-1", i),
				Predicate:       "state_drilldown",
				Object:          "S",
				Value:           "21.000",
				Unit:            "ms",
				RichNotes:       []string{"source=top_sleep"},
				SupportRefs:     []string{fmt.Sprintf("attached_trace.txt:%d-%d", 100+i*10, 105+i*10)},
			})
		}
		mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
			ToolName: "trace_query", Success: true, Observations: obs,
		}}})
		return &types.AgentContext{Mutable: mu}
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "结论。",
	}}}
	// PTV6-C ruling C (#73): the trim tail states the omitted rows' trace
	// line envelope (all fixture rows share one artifact) — never the retired
	// intermediate-record pointer.
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: the artifact
	// basename moved from the trim tail into the grouped intro line
	// (本块全部坐标位于 `<base>`); the tail keeps 行 X–Y (en-dash inside).
	over := renderTraceQueryObservationSupplement(build(traceQueryObservationSupplementMaxRows+5), doc, "zh")
	if !strings.Contains(over, "本块全部坐标位于 `attached_trace.txt`") ||
		!strings.Contains(over, fmt.Sprintf("(共 %d 条,仅列前 %d 条;其余 5 条位于行 ",
			traceQueryObservationSupplementMaxRows+5, traceQueryObservationSupplementMaxRows)) ||
		!strings.Contains(over, " 区间)") {
		t.Fatalf("over-cap supplement must disclose the trim count with the trace envelope:\n%s", over)
	}
	if strings.Contains(over, "见原始 trace_query 记录") {
		t.Fatalf("retired intermediate-record pointer resurfaced:\n%s", over)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: EN tail drops the
	// basename too (grouped intro carries it): "… sit within lines X–Y".
	overEN := renderTraceQueryObservationSupplement(build(traceQueryObservationSupplementMaxRows+5), doc, "en")
	if !strings.Contains(overEN, fmt.Sprintf("(%d rows total; only the first %d are listed; the other 5 sit within lines ",
		traceQueryObservationSupplementMaxRows+5, traceQueryObservationSupplementMaxRows)) {
		t.Fatalf("EN over-cap supplement must disclose the trim count:\n%s", overEN)
	}
	if strings.Contains(overEN, "remain in the raw trace_query records") {
		t.Fatalf("EN retired intermediate-record pointer resurfaced:\n%s", overEN)
	}
	under := renderTraceQueryObservationSupplement(build(3), doc, "zh")
	if under == "" || strings.Contains(under, "仅列前") {
		t.Fatalf("under-cap supplement must stay disclosure-free:\n%s", under)
	}
}

// TestPTV6CSupplementCoordinateSameSourceOnly pins the 修正轮 Med holes on the
// ruling-C envelope coordinate (2026-07-06): a coordinate pair is same-source
// only (one SupportRef carrying BOTH halves, or the record's own
// SourceRef+Span) — a SupportRef basename never splices onto SourceRef Span
// line numbers — and missing_wakeup synthetic-line records (projection-side
// EvidenceCoordinateTail 同判据) never fabricate a line coordinate.
func TestPTV6CSupplementCoordinateSameSourceOnly(t *testing.T) {
	// Same-source lane 1: basename + line suffix from ONE SupportRef.
	ref := types.ObservationRecord{
		SupportRefs: []string{"attached_trace.txt:100-105"},
		SourceRef:   types.ObservationSourceRef{ArtifactID: "other.systrace"},
		Span:        types.ObservationSpan{LineStart: 900, LineEnd: 950},
	}
	if base, ls, le := traceQueryObservationSourceCoordinate(ref); base != "attached_trace.txt" || ls != 100 || le != 105 {
		t.Fatalf("lane-1 coordinate must take BOTH halves from the SupportRef: %q %d-%d", base, ls, le)
	}
	// Splice ban: a suffix-less SupportRef basename must NOT pair with the
	// record Span; with no SourceRef base either, the coordinate is refused.
	splice := types.ObservationRecord{
		SupportRefs: []string{"attached_trace.txt"},
		Span:        types.ObservationSpan{LineStart: 100, LineEnd: 105},
	}
	if base, ls, _ := traceQueryObservationSourceCoordinate(splice); base != "" || ls != 0 {
		t.Fatalf("cross-source splice fabricated a locator: %q %d", base, ls)
	}
	// Same-source lane 2: the record's OWN SourceRef + Span.
	own := types.ObservationRecord{
		SourceRef: types.ObservationSourceRef{ArtifactID: "trace.systrace"},
		Span:      types.ObservationSpan{LineStart: 300, LineEnd: 320},
	}
	if base, ls, le := traceQueryObservationSourceCoordinate(own); base != "trace.systrace" || ls != 300 || le != 320 {
		t.Fatalf("lane-2 coordinate must pair the record's own SourceRef+Span: %q %d-%d", base, ls, le)
	}
	// Synthetic-line guard: missing_wakeup interval bookkeeping never claims
	// a trace row (both the predicate and the claim-key form).
	for _, synthetic := range []types.ObservationRecord{
		{Predicate: "missing_wakeup", SupportRefs: []string{"attached_trace.txt:44-44"}},
		{ClaimKey: "root_evidence:missing_wakeup", SourceRef: types.ObservationSourceRef{ArtifactID: "trace.systrace"}, Span: types.ObservationSpan{LineStart: 44, LineEnd: 44}},
	} {
		if base, ls, _ := traceQueryObservationSourceCoordinate(synthetic); base != "" || ls != 0 {
			t.Fatalf("synthetic-line record fabricated a coordinate: %q %d", base, ls)
		}
	}
}

// TestPTV5TraceQueryObservationLocationDropsToolCallID (C40): a record with no
// path and no line span shows NO locator (the caller skips the empty part) —
// an internal tool-call id is not a locator.
func TestPTV5TraceQueryObservationLocationDropsToolCallID(t *testing.T) {
	record := types.ObservationRecord{
		SourceRef: types.ObservationSourceRef{ToolCallID: "call-abc123"},
	}
	if got := traceQueryObservationLocation(record); got != "" {
		t.Fatalf("locator-less records must render nothing, got %q", got)
	}
	text := traceQueryObservationSupplementText(record, true)
	if strings.Contains(text, "call-abc123") {
		t.Fatalf("the tool-call id must stay off the panel: %s", text)
	}
}

func TestRecoveredAnswerDocumentCaveat_DisclosesMalformedVisibleStringSalvage(t *testing.T) {
	rec := tool.AnswerDocumentTextRecovery{Mode: "content_json_visible_string_salvage"}
	zh := recoveredAnswerDocumentCaveat("zh", rec)
	for _, want := range []string{"模型返回", "畸形", "仅提取", "可能缺段或失序"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh malformed-json disclosure missing %q: %s", want, zh)
		}
	}
	en := recoveredAnswerDocumentCaveat("en", rec)
	for _, want := range []string{"model returned malformed", "extracted only", "incomplete or out of order"} {
		if !strings.Contains(strings.ToLower(en), want) {
			t.Fatalf("en malformed-json disclosure missing %q: %s", want, en)
		}
	}
}
