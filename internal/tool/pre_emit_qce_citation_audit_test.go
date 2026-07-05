package tool

// QCE batch #61 GAP-A/GAP-B pins (2026-07-05, tightened per the adversarial
// review's 19 confirmed findings).
//
// Specimen: eval/results/qf_architecture-20260705-025038. The finalizer
// emitted three pre-stage bullet items whose labels reproduce the accepted
// supporting_coverage member_set verbatim and whose citation_refs
// (internal/types/enums.go:15/24/31) reproduce that fact's explicit
// per-member support_refs verbatim — all three verified correct against the
// run-time HEAD. The pre-emit chain then (1) detached two of the three refs
// on noisy evidence-text matching (agent-name spelling luck), (2) pruned all
// three items as "extraneous" against the principal-only display sets, and
// (3) disclosed "条目内容保留" while the content was gone.
//
// Pinned invariants:
//   - a citation whose item LABEL names a member of an accepted
//     content-bearing member_set (principal_answer / supporting_coverage)
//     and matches that member's explicit support_ref location is never
//     detached; item prose text is never a protection signal;
//   - enumeration pruning keeps label-named members of content-bearing
//     member_sets outside the strict source-inventory lane; hallucinated
//     rows (including text name-drops), audit_ledger-backed rows, and
//     strict-lane leaks are still pruned;
//   - decorated members ("Base (qualifier)") bind to their NAMED
//     support_refs by base identity through the single cross-package naming
//     authority; same-base siblings do not cross-bind a base-named ref;
//   - the detach disclosure caveat is materialized at the persist
//     chokepoint (after the LAST content-mutating pass) from each item's
//     actual final presence, with block-scoped identity — "content kept" is
//     never claimed for removed content;
//   - quoteless system/model citations get their source-line quote
//     backfilled by the gated passes (chain end + pre-persist) (GAP-B).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func qceSpecimenEvidence() []types.EvidenceItem {
	mk := func(subject, symbol, snippet, summary string, line, lineEnd int, terms ...string) types.EvidenceItem {
		return types.EvidenceItem{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/enums.go",
			LineStart:       line,
			LineEnd:         lineEnd,
			AnchorKind:      types.AnchorDefinition,
			Subject:         subject,
			AnchorSymbol:    symbol,
			Snippet:         snippet,
			Summary:         summary,
			SurfaceTerms:    terms,
			GroundingStatus: types.GroundingGrounded,
		}
	}
	return []types.EvidenceItem{
		mk("StageAnalyze", "StageAnalyze", `StageAnalyze  PipelineStage = "analyze"`, "core read-pipeline stage 1 — analyze", 33, 0, "analyze"),
		mk("StageExplore", "StageExplore", `StageExplore  PipelineStage = "explore"`, "core read-pipeline stage 2 — explore", 34, 0, "explore"),
		mk("StageExtract", "StageExtract", `StageExtract  PipelineStage = "extract"`, "core read-pipeline stage 3 — extract", 35, 0, "extract"),
		mk("StageFinalize", "StageFinalize", `StageFinalize PipelineStage = "finalize"`, "core read-pipeline stage 4 — finalize (terminal)", 36, 0, "finalize"),
		mk("AllMainStages", "AllMainStages", "func AllMainStages() []PipelineStage { return []PipelineStage{\n StageAnalyze,\n StageExplore,\n StageExtract,\n StageFinalize,\n } }", "the canonical ordering of the unconditional 4-stage read pipeline", 117, 124, "AllMainStages", "analyze explore extract finalize"),
		mk("AllStages", "AllStages", "func AllStages() []PipelineStage {\n return []PipelineStage{\n StageLogTriage,\n StagePerfTriage,\n StageMultiRepoFocus,\n StageAnalyze,\n StageExplore,\n StageExtract,\n StageFinalize,\n }\n }", "all stages incl. three optional pre-stages prefixed to the four-stage core", 102, 112, "AllStages"),
		mk("StageLogTriage", "StageLogTriage", `StageLogTriage PipelineStage = "log_triage"`, "conditional pre-stage — runs only when an attached log is present", 15, 0, "log_triage"),
		mk("StagePerfTriage", "StagePerfTriage", `StagePerfTriage PipelineStage = "perf_triage"`, "conditional pre-stage — runs only when an attached perf/htrace/systrace is present", 24, 0, "perf_triage"),
		mk("StageMultiRepoFocus", "StageMultiRepoFocus", `StageMultiRepoFocus PipelineStage = "multi_repo_focus"`, "conditional pre-stage — multi-sub-repo focus selector before analyze", 31, 0, "multi_repo_focus"),
		mk("IsTerminal", "IsTerminal", "func (s PipelineStage) IsTerminal() bool {\n return s == StageFinalize\n }", "only the finalize stage is terminal in the read pipeline", 66, 68, "finalize"),
		mk("AgentName", "AgentAnalyzer", "AgentAnalyzer       AgentName = \"analyzer\"\n AgentExplorer       AgentName = \"explorer\"\n AgentExtractor      AgentName = \"extractor\"\n AgentFinalizer      AgentName = \"finalizer\"\n AgentLogTriager     AgentName = \"log_triager\"\n AgentPerfTriager    AgentName = \"perf_triager\"\n AgentMultiRepoFocus AgentName = \"multi_repo_focus_selector\"", "agent name enum — one agent per read pipeline stage", 130, 137, "analyzer", "explorer", "extractor", "finalizer"),
	}
}

func qceSpecimenAggregateFacts() []types.AnswerAggregateFact {
	return []types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateMemberSet,
			Label: "read-mode pipeline 主链 4 stage",
			Value: "4",
			Unit:  "stages",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{
				"StageAnalyze (analyzer, classifier + repo_map + AnalysisIR)",
				"StageExplore (explorer, dependency-graph traversal)",
				"StageExtract (extractor, factual extraction + grounding)",
				"StageFinalize (finalizer, terminal answer document)",
			},
			MemberNotes: []string{
				"第 1 阶段，做任务/代码理解分析，输出 AnalysisIR",
				"第 2 阶段，沿依赖关系图定向深挖证据",
				"第 3 阶段，从 explore 结果里抽取事实并 grounding，筛入最终答案",
				"第 4 阶段（终态，IsTerminal），产出 final answer / answer_document_v2",
			},
			SupportRefs: []string{
				"StageAnalyze: internal/types/enums.go:33",
				"StageExplore: internal/types/enums.go:34",
				"StageExtract: internal/types/enums.go:35",
				"StageFinalize: internal/types/enums.go:36",
			},
		},
		{
			Kind:  types.AnswerAggregateMemberSet,
			Label: "read-mode 可选 pre-stage",
			Value: "3",
			Unit:  "pre_stages",
			Role:  types.AnswerAggregateRoleSupportingCoverage,
			Members: []string{
				"StageLogTriage (log_triager)",
				"StagePerfTriage (perf_triager)",
				"StageMultiRepoFocus (multi_repo_focus_selector)",
			},
			MemberNotes: []string{
				"仅当 BusContext.AttachedLog 非空时触发，结果为 LogBundle 写到 MutableState，给下游 stage 当 read-only hint",
				"仅当 AttachedHitrace 非空时触发（HarmonyOS-HiTrace / Android-systrace），输出 jank spans + main-thread stalls + cold-start timing 的 PerfBundle",
				"仅当 workspace 含多个 sub-repo 且用户未显式 pin focus 时触发，给出 typed focus 建议",
			},
			SupportRefs: []string{
				"StageLogTriage: internal/types/enums.go:15",
				"StagePerfTriage: internal/types/enums.go:24",
				"StageMultiRepoFocus: internal/types/enums.go:31",
			},
		},
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "read-pipeline 一对一绑定的 4 个 agent",
			Value:   "4",
			Unit:    "agents",
			Role:    types.AnswerAggregateRoleSupportingCoverage,
			Members: []string{"AgentAnalyzer", "AgentExplorer", "AgentExtractor", "AgentFinalizer"},
			SupportRefs: []string{
				"internal/types/enums.go:130",
				"internal/types/enums.go:131",
				"internal/types/enums.go:132",
				"internal/types/enums.go:133",
			},
		},
	}
}

func qceSpecimenDoc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "main_stages",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "主链 4 stage（按 `AllMainStages()` 固定顺序）",
				FacetIDs:    []string{string(types.FacetEnumerationItem)},
				ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact, FacetID: string(types.FacetEnumerationItem)}},
				Items: []types.AnswerBlockItem{
					{ID: "st1", Label: "StageAnalyze (analyzer, classifier + repo_map + AnalysisIR)", Text: "第 1 阶段，由 analyzer agent 承担；做任务理解与代码分类（classifier），通过 repo_map 抓取仓库结构，最终输出 AnalysisIR 作为后续 explore 的入口。", CitationRef: 0},
					{ID: "st2", Label: "StageExplore (explorer, dependency-graph traversal)", Text: "第 2 阶段，由 explorer agent 承担；从 AnalysisIR 出发沿依赖关系图定向深挖证据，为 extract 准备 grounded 候选。", CitationRef: 1},
					{ID: "st3", Label: "StageExtract (extractor, factual extraction + grounding)", Text: "第 3 阶段，由 extractor agent 承担；从 explore 的产物里抽取事实并做 grounding 校验，筛入最终答案。", CitationRef: 2},
					{ID: "st4", Label: "StageFinalize (finalizer, terminal answer document)", Text: "第 4 阶段（终态），由 finalizer agent 承担；合成 final answer / answer_document_v2，pipeline 终止（IsTerminal() == true）。", CitationRef: 3},
				},
			},
			{
				ID:       "pre_stages",
				Kind:     types.BlockBulletList,
				Title:    "可选的 3 个 conditional pre-stage（仅在对应 trigger 存在时插入到 analyze 之前）",
				FacetIDs: []string{string(types.FacetEnumerationItem)},
				Items: []types.AnswerBlockItem{
					{ID: "pre1", Label: "StageLogTriage (log_triager)", Text: "仅当 `BusContext.AttachedLog` 非空时触发；dispatch log_triager agent 产出 LogBundle，写到 MutableState 供下游 stage 作为 read-only hint 消费。", CitationRef: 4},
					{ID: "pre2", Label: "StagePerfTriage (perf_triager)", Text: "仅当 `AttachedHitrace` 非空（HarmonyOS-HiTrace / Android-systrace）时触发；产出 jank spans、main-thread stalls、cold-start timing 等 PerfBundle 信息。", CitationRef: 5},
					{ID: "pre3", Label: "StageMultiRepoFocus (multi_repo_focus_selector)", Text: "仅当 workspace 包含多个 sub-repo 且用户未显式 pin focus 时触发；做 sub-repo 焦点选择，避免 analyze 在多仓环境下发散。", CitationRef: 6},
				},
			},
		},
		Citations: []types.Citation{
			{File: "internal/types/enums.go", Line: 33},
			{File: "internal/types/enums.go", Line: 34},
			{File: "internal/types/enums.go", Line: 35},
			{File: "internal/types/enums.go", Line: 36},
			{File: "internal/types/enums.go", Line: 15},
			{File: "internal/types/enums.go", Line: 24},
			{File: "internal/types/enums.go", Line: 31},
			{File: "internal/types/enums.go", Line: 117},
		},
	}
}

func qceSpecimenCtx() *types.BusContext {
	mu := types.NewMutableState("请介绍 codrax read-mode pipeline 的整体架构")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: qceSpecimenEvidence()})
	mu.SetInvestigationAggregateFacts(qceSpecimenAggregateFacts())
	mu.RetainInvestigationAggregateFacts()
	return &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
}

func qceFindBlock(t *testing.T, doc *types.AnswerDocumentV2, id string) *types.AnswerBlock {
	t.Helper()
	if doc == nil {
		return nil
	}
	for i := range doc.Blocks {
		if doc.Blocks[i].ID == id {
			return &doc.Blocks[i]
		}
	}
	return nil
}

// GAP-A pin 1 — detach layer: a citation whose item label names an accepted
// supporting member_set member and matches that member's explicit
// support_ref location must never be detached (specimen: 2 of 3 isomorphic
// correct refs were detached on qualifier-spelling luck).
func TestQCESpecimen_SupportingMemberSetRefsSurviveDetach(t *testing.T) {
	ctx := qceSpecimenCtx()
	doc := qceSpecimenDoc()
	if fixed := detachInvalidItemCitationRefsWithoutSafeCandidateWithContext(doc, nil, ctx, nil); fixed != 0 {
		t.Fatalf("typed support-ref-backed citations must survive detach, got fixed=%d", fixed)
	}
	pre := qceFindBlock(t, doc, "pre_stages")
	for i, wantRef := range []int{4, 5, 6} {
		if got := pre.Items[i].CitationRef; got != wantRef {
			t.Fatalf("pre_stages item %d citation_ref = %d, want %d", i, got, wantRef)
		}
	}
}

// GAP-A pin 2 — prune layer: enumeration pruning must not delete items whose
// labels name accepted supporting member_set members (content retention).
func TestQCESpecimen_PrunePreservesSupportingMemberSetItems(t *testing.T) {
	ctx := qceSpecimenCtx()
	doc := qceSpecimenDoc()
	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	pre := qceFindBlock(t, doc, "pre_stages")
	if pre == nil {
		t.Fatal("pre_stages block disappeared")
	}
	if len(pre.Items) != 3 {
		t.Fatalf("supporting member_set items must be preserved, got %d items: %+v", len(pre.Items), pre.Items)
	}
	for i, wantLabel := range []string{
		"StageLogTriage (log_triager)",
		"StagePerfTriage (perf_triager)",
		"StageMultiRepoFocus (multi_repo_focus_selector)",
	} {
		if pre.Items[i].Label != wantLabel {
			t.Fatalf("pre_stages item %d label = %q, want %q", i, pre.Items[i].Label, wantLabel)
		}
		if strings.TrimSpace(pre.Items[i].Text) == "" {
			t.Fatalf("pre_stages item %d lost its text content", i)
		}
	}
}

// Review finding 1 (P1), detach face — a hallucinated LABEL citing an
// evidence-backed line is still detached even when its prose TEXT name-drops
// a real member: item text is not a protection signal.
func TestQCEDetach_TextNameDropDoesNotProtectBogusLabel(t *testing.T) {
	ctx := qceSpecimenCtx()
	doc := qceSpecimenDoc()
	doc.Blocks[1].Items = append(doc.Blocks[1].Items, types.AnswerBlockItem{
		ID:          "bogus",
		Label:       "StageBogus (bogus_agent)",
		Text:        "该阶段配合 StageLogTriage (log_triager) 工作，见 log_triage 说明。",
		CitationRef: 4,
	})
	if fixed := detachInvalidItemCitationRefsWithoutSafeCandidateWithContext(doc, nil, ctx, nil); fixed != 1 {
		t.Fatalf("hallucinated label with member name-drop in text must still be detached, got fixed=%d", fixed)
	}
	if got := doc.Blocks[1].Items[3].CitationRef; got != -1 {
		t.Fatalf("bogus item should be detached, got ref=%d", got)
	}
	for i, wantRef := range []int{4, 5, 6} {
		if got := doc.Blocks[1].Items[i].CitationRef; got != wantRef {
			t.Fatalf("typed-backed item %d must keep ref %d, got %d", i, wantRef, got)
		}
	}
}

// Review finding 1 (P1), prune face — a hallucinated row whose TEXT
// name-drops real members is still pruned; label-only matching decides.
func TestQCEPrune_TextNameDropDoesNotRescueHallucinatedItem(t *testing.T) {
	ctx := qceSpecimenCtx()
	doc := qceSpecimenDoc()
	doc.Blocks[1].Items = append(doc.Blocks[1].Items, types.AnswerBlockItem{
		ID:          "bogus",
		Label:       "StageBogus (bogus_agent)",
		Text:        "该阶段配合 StageLogTriage (log_triager) 与 StagePerfTriage (perf_triager) 工作。",
		CitationRef: -1,
	})
	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	pre := qceFindBlock(t, doc, "pre_stages")
	if pre == nil {
		t.Fatal("pre_stages block disappeared")
	}
	if len(pre.Items) != 3 {
		t.Fatalf("text name-drop must not rescue a hallucinated row, got %d items", len(pre.Items))
	}
	for _, item := range pre.Items {
		if item.ID == "bogus" {
			t.Fatal("hallucinated item survived enumeration pruning via text name-drop")
		}
	}
}

// Review finding 2 — audit_ledger member_sets are not protection sources:
// neither the detach keep-gate nor the prune keep-gate accepts them.
func TestQCERoleWhitelist_AuditLedgerMemberSetDoesNotProtect(t *testing.T) {
	ctx := qceSpecimenCtx()
	ctx.Mutable.SetInvestigationAggregateFacts(append(qceSpecimenAggregateFacts(), types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "inspection audit trail",
		Value:       "1",
		Role:        types.AnswerAggregateRoleAuditLedger,
		Members:     []string{"AuditStage (audit_agent)"},
		SupportRefs: []string{"AuditStage: internal/types/enums.go:15"},
	}))
	ctx.Mutable.RetainInvestigationAggregateFacts()

	doc := qceSpecimenDoc()
	doc.Blocks[1].Items = append(doc.Blocks[1].Items, types.AnswerBlockItem{
		ID: "audit", Label: "AuditStage (audit_agent)", Text: "审计行。", CitationRef: 4,
	})
	if fixed := detachInvalidItemCitationRefsWithoutSafeCandidateWithContext(doc, nil, ctx, nil); fixed != 1 {
		t.Fatalf("audit_ledger-backed citation must not be kept by the member-set keep-gate, got fixed=%d", fixed)
	}
	if got := doc.Blocks[1].Items[3].CitationRef; got != -1 {
		t.Fatalf("audit item should be detached, got ref=%d", got)
	}

	doc2 := qceSpecimenDoc()
	doc2.Blocks[1].Items = append(doc2.Blocks[1].Items, types.AnswerBlockItem{
		ID: "audit", Label: "AuditStage (audit_agent)", Text: "审计行。", CitationRef: -1,
	})
	normalizePrincipalEnumerationRowBlocks(doc2, ctx)
	pre := qceFindBlock(t, doc2, "pre_stages")
	for _, item := range pre.Items {
		if item.ID == "audit" {
			t.Fatal("audit_ledger member must not rescue an enumeration row from pruning")
		}
	}
}

// GAP-A negative control — prune still removes rows with no typed member
// backing at all.
func TestQCEPrune_StillRemovesHallucinatedItems(t *testing.T) {
	ctx := qceSpecimenCtx()
	doc := qceSpecimenDoc()
	doc.Blocks[1].Items = append(doc.Blocks[1].Items, types.AnswerBlockItem{
		ID: "bogus", Label: "StageBogus (bogus_agent)", Text: "不存在的阶段。", CitationRef: -1,
	})
	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	pre := qceFindBlock(t, doc, "pre_stages")
	if pre == nil {
		t.Fatal("pre_stages block disappeared")
	}
	if len(pre.Items) != 3 {
		t.Fatalf("hallucinated item must still be pruned while typed-backed items stay, got %d items", len(pre.Items))
	}
}

// Review finding 3 — the STRICT source-inventory lane keeps its exclusive
// semantics: a row that leaked into the strict principal carrier is pruned
// even when a supporting_coverage member_set names it. (Shape from the
// adversarial review's Cangjie fixture: "Item" is a public struct leaked
// into a "public class" strict inventory.)
func TestQCEPrune_StrictSourceInventoryLaneNotRescued(t *testing.T) {
	mu := types.NewMutableState("列出 public class 声明")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateMemberSet,
			Label: "public class 声明",
			Value: "2",
			Role:  types.AnswerAggregateRoleSupportingCoverage,
			Members: []string{
				"Bridge @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:15 (package demo.bridge)",
				"Item @ eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:6 (package demo.cart)",
			},
			SupportRefs: []string{
				"Bridge: eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:15",
				"Item: eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:6",
			},
		},
		{
			Kind:       types.AnswerAggregateMemberSet,
			Label:      "source inventory principal rows",
			Value:      "2",
			Role:       types.AnswerAggregateRolePrincipalAnswer,
			Provenance: types.SourceInventoryPrincipalRowSetAggregateProvenance,
			Members: []string{
				"Bridge @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:15 (package demo.bridge)",
				"Cart @ eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14 (package demo.cart)",
			},
			SupportRefs: []string{
				"Bridge: eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:15",
				"Cart: eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14",
			},
		},
	})
	mu.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleType,
			Complete: false,
			Members: []types.SourceInventoryObservationMember{
				{
					Name: "Bridge", Role: types.AnswerCandidateRoleType,
					File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15,
					Language: "cangjie", SurfaceTerms: []string{"public class", "public class Bridge"},
					CoverageState: types.SourceInventoryCoverageObserved,
					Attributes:    []types.SourceInventoryObservationAttribute{{Role: types.AnswerCandidateRolePackage, Name: "demo.bridge"}},
				},
				{
					Name: "Cart", Role: types.AnswerCandidateRoleType,
					File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 14,
					Language: "cangjie", SurfaceTerms: []string{"public class", "public class Cart"},
					CoverageState: types.SourceInventoryCoverageObserved,
					Attributes:    []types.SourceInventoryObservationAttribute{{Role: types.AnswerCandidateRolePackage, Name: "demo.cart"}},
				},
			},
		}},
	})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentEnumerate,
			Language:   "zh",
			Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
				SourceQuotes:      []string{"public class"},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
					types.SourceInventoryFieldPackage,
				},
				Confidence: 0.95,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 15, Quote: "public class Bridge {"},
			{File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 14, Quote: "public class Cart {"},
			{File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 6, Quote: "public struct Item {"},
		},
		Blocks: []types.AnswerBlock{{
			ID:          "public-class",
			Kind:        types.BlockOrderedList,
			Title:       "public class 声明",
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{
				{ID: "bridge", Label: "Bridge", Text: "public class Bridge，package=demo.bridge", CitationRef: 0},
				{ID: "cart", Label: "Cart", Text: "public class Cart，package=demo.cart", CitationRef: 1},
				{ID: "item", Label: "Item", Text: "public class Item，package=demo.cart", CitationRef: 2},
			},
		}},
	}
	_ = materializeRequiredModelSurfaceTerms(doc, ctx)
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		t.Fatal("no surface plan")
	}
	sets := types.CompileEnumerationDisplaySets(&ctx.AnalysisIR.RequestModel, plan)
	if len(sets) == 0 {
		t.Fatal("no display sets compiled")
	}
	if _, strict := principalEnumerationPruneRowsForBlockAtWithMode(doc, 0, sets); !strict {
		t.Fatal("fixture must exercise the strict source-inventory lane")
	}
	prunePrincipalEnumerationExtraneousItems(doc, ctx, sets)
	for _, item := range doc.Blocks[0].Items {
		if item.ID == "item" {
			t.Fatal("supporting member_set naming must not rescue a leak inside the strict source-inventory lane")
		}
	}
	if len(doc.Blocks[0].Items) != 2 {
		t.Fatalf("strict lane should keep exactly the scoped rows, got %d items", len(doc.Blocks[0].Items))
	}
}

// Review findings 7 + 11 — the single cross-package naming authority binds
// decorated members to base-named refs, never binds generic/empty labels,
// and same-base siblings do not cross-bind a base-named ref (full decorated
// surface refs still bind exactly).
func TestQCESupportRefNamingAuthority_ContractMatrix(t *testing.T) {
	if !types.AnswerAggregateNamedSupportRefLabelDescribesMember("StageLogTriage", "StageLogTriage (log_triager)") {
		t.Fatal("base-named ref label must describe its decorated member")
	}
	if types.AnswerAggregateNamedSupportRefLabelDescribesMember("StageOther", "StageLogTriage (log_triager)") {
		t.Fatal("a different base must not describe this member")
	}
	if types.AnswerAggregateNamedSupportRefLabelDescribesMember("", "StageLogTriage (log_triager)") {
		t.Fatal("empty ref label must never bind by name")
	}

	fact := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "same-base siblings",
		Value: "2",
		Role:  types.AnswerAggregateRoleSupportingCoverage,
		Members: []string{
			"Foo (fast path)",
			"Foo (slow path)",
		},
		SupportRefs: []string{
			"Foo: internal/pkg/a.go:10",
			"Foo: internal/pkg/a.go:20",
		},
	}
	if _, _, _, ok := preEmitAggregateMemberSupportLocationClassified(fact, 0, fact.Members[0]); ok {
		t.Fatal("base-named ref must not bind when two members share the base (ambiguous)")
	}
	exact := fact
	exact.SupportRefs = []string{
		"Foo (fast path): internal/pkg/a.go:10",
		"Foo (slow path): internal/pkg/a.go:20",
	}
	source, line, kind, ok := preEmitAggregateMemberSupportLocationClassified(exact, 1, exact.Members[1])
	if !ok || kind != preEmitAggregateSupportExplicit || source != "internal/pkg/a.go" || line != 20 {
		t.Fatalf("full decorated-surface ref must bind exactly: source=%q line=%d kind=%d ok=%v", source, line, kind, ok)
	}
}

// GAP-A pin 3 — support-ref parsing face: a decorated member with a unique
// base binds its NAMED support_ref by base identity (explicit lane), and
// does not bind a ref naming a different base or a wrong location.
func TestQCEAggregateSupportRefClassifier_DecoratedMemberBindsNamedRefByBase(t *testing.T) {
	fact := types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "read-mode 可选 pre-stage",
		Value:       "1",
		Role:        types.AnswerAggregateRoleSupportingCoverage,
		Members:     []string{"StageLogTriage (log_triager)"},
		SupportRefs: []string{"StageLogTriage: internal/types/enums.go:15"},
	}
	source, line, kind, ok := preEmitAggregateMemberSupportLocationClassified(fact, 0, fact.Members[0])
	if !ok || kind != preEmitAggregateSupportExplicit || source != "internal/types/enums.go" || line != 15 {
		t.Fatalf("decorated member must bind its named support_ref by base identity: source=%q line=%d kind=%d ok=%v", source, line, kind, ok)
	}
	if !preEmitAggregateMemberCitationMatchesExplicit(fact, 0, fact.Members[0], types.Citation{File: "internal/types/enums.go", Line: 15}) {
		t.Fatal("explicit citation match must accept the member-bound support_ref location")
	}
	if preEmitAggregateMemberCitationMatchesExplicit(fact, 0, fact.Members[0], types.Citation{File: "internal/types/enums.go", Line: 24}) {
		t.Fatal("explicit citation match must reject a location the member's support_ref does not name")
	}
	other := fact
	other.SupportRefs = []string{"StageOther: internal/types/enums.go:99"}
	if _, _, _, ok := preEmitAggregateMemberSupportLocationClassified(other, 0, other.Members[0]); ok {
		t.Fatal("a support_ref naming a different base must not bind to this member")
	}
}

// Review finding 10 — the decorated-base explicit binding also activates
// the PRINCIPAL aggregate keep lane (preEmitCitationSupportsAggregateItem):
// principal decorated members now bind their named refs, and a sibling
// member's location is still rejected.
func TestQCEPrincipalDecoratedMemberExplicitBindingActive(t *testing.T) {
	ctx := qceSpecimenCtx()
	pctx := newPreEmitCheckContext(ctx)
	label := "StageAnalyze (analyzer, classifier + repo_map + AnalysisIR)"
	if !preEmitCitationSupportsAggregateItemWithContext(pctx, label, "", types.Citation{File: "internal/types/enums.go", Line: 33}) {
		t.Fatal("principal decorated member must bind its named support_ref location explicitly")
	}
	if preEmitCitationSupportsAggregateItemWithContext(pctx, label, "", types.Citation{File: "internal/types/enums.go", Line: 34}) {
		t.Fatal("principal member must not accept a sibling member's location")
	}
}

// GAP-A pin 4 — disclosure wording and disposal share one typed signal.
// MUTATION PIN: any pass that deletes a detached item flips its presence,
// and the caveat wording must flip with it — "content kept" may only be
// claimed for items still visible in the final document.
func TestQCEDetachedCitationDisclosure_WordingTracksActualDisposal(t *testing.T) {
	ctx := qceSpecimenCtx()

	keptDoc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "b1",
		Kind: types.BlockBulletList,
		Items: []types.AnswerBlockItem{
			{ID: "i1", Label: "StageLogTriage (log_triager)", Text: "内容仍在。", CitationRef: -1},
		},
	}}}
	materializeDetachedCitationRefCaveats(keptDoc, ctx, []types.DetachedCitationDisclosure{
		{BlockID: "b1", ItemID: "i1", Label: "StageLogTriage (log_triager)"},
	})
	if len(keptDoc.Caveats) != 1 {
		t.Fatalf("kept scenario should add exactly one caveat, got %v", keptDoc.Caveats)
	}
	if !strings.Contains(keptDoc.Caveats[0], "条目内容保留") {
		t.Fatalf("visible item disclosure must state retention, got %q", keptDoc.Caveats[0])
	}
	if strings.Contains(keptDoc.Caveats[0], "一并移除") {
		t.Fatalf("visible item disclosure must not claim removal, got %q", keptDoc.Caveats[0])
	}

	removedDoc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "b1",
		Kind:  types.BlockBulletList,
		Items: nil,
	}}}
	materializeDetachedCitationRefCaveats(removedDoc, ctx, []types.DetachedCitationDisclosure{
		{BlockID: "b1", ItemID: "i1", Label: "StageBogus (bogus_agent)"},
	})
	if len(removedDoc.Caveats) != 1 {
		t.Fatalf("removed scenario should add exactly one caveat, got %v", removedDoc.Caveats)
	}
	if strings.Contains(removedDoc.Caveats[0], "条目内容保留") {
		t.Fatalf("disclosure must never claim retention for removed content, got %q", removedDoc.Caveats[0])
	}
	if !strings.Contains(removedDoc.Caveats[0], "一并移除") {
		t.Fatalf("removed item disclosure must state removal, got %q", removedDoc.Caveats[0])
	}

	mixedDoc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "b1",
		Kind: types.BlockBulletList,
		Items: []types.AnswerBlockItem{
			{ID: "i1", Label: "StageLogTriage (log_triager)", Text: "内容仍在。", CitationRef: -1},
		},
	}}}
	materializeDetachedCitationRefCaveats(mixedDoc, ctx, []types.DetachedCitationDisclosure{
		{BlockID: "b1", ItemID: "i1", Label: "StageLogTriage (log_triager)"},
		{BlockID: "b1", ItemID: "i2", Label: "StageBogus (bogus_agent)"},
	})
	if len(mixedDoc.Caveats) != 2 {
		t.Fatalf("mixed scenario should add two caveats, got %v", mixedDoc.Caveats)
	}
	if !strings.Contains(mixedDoc.Caveats[0], "1 处") || !strings.Contains(mixedDoc.Caveats[0], "条目内容保留") {
		t.Fatalf("mixed kept caveat wrong: %q", mixedDoc.Caveats[0])
	}
	if !strings.Contains(mixedDoc.Caveats[1], "1 处") || !strings.Contains(mixedDoc.Caveats[1], "一并移除") {
		t.Fatalf("mixed removed caveat wrong: %q", mixedDoc.Caveats[1])
	}
}

// Review finding 5 — item identity is block-scoped on BOTH legs: an
// unrelated same-ID item in another block must not fabricate a "content
// kept" claim for a removed item.
func TestQCEDetachedCitationDisclosure_CrossBlockIDCollisionDisclosesRemoval(t *testing.T) {
	ctx := qceSpecimenCtx()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "b1",
			Kind: types.BlockBulletList,
			// The detached item was pruned: b1 no longer holds ID "1".
			Items: []types.AnswerBlockItem{{ID: "2", Label: "survivor in b1"}},
		},
		{
			ID:   "b2",
			Kind: types.BlockBulletList,
			// Unrelated item that happens to reuse per-block ID "1".
			Items: []types.AnswerBlockItem{{ID: "1", Label: "unrelated item in b2"}},
		},
	}}
	materializeDetachedCitationRefCaveats(doc, ctx, []types.DetachedCitationDisclosure{
		{BlockID: "b1", ItemID: "1", Label: "removed label"},
	})
	if len(doc.Caveats) != 1 {
		t.Fatalf("expected one caveat, got %v", doc.Caveats)
	}
	if strings.Contains(doc.Caveats[0], "条目内容保留") {
		t.Fatalf("cross-block ID collision fabricated a retention claim: %q", doc.Caveats[0])
	}
	if !strings.Contains(doc.Caveats[0], "一并移除") {
		t.Fatalf("removed item must be disclosed as removed, got %q", doc.Caveats[0])
	}
}

// GAP-A pin 4b — English wording branch obeys the same typed coupling.
func TestQCEDetachedCitationDisclosure_EnglishWording(t *testing.T) {
	mu := types.NewMutableState("architecture question")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "en",
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "b1", Kind: types.BlockBulletList}}}
	materializeDetachedCitationRefCaveats(doc, ctx, []types.DetachedCitationDisclosure{
		{BlockID: "b1", ItemID: "gone", Label: "Removed (thing)"},
	})
	if len(doc.Caveats) != 1 {
		t.Fatalf("expected one caveat, got %v", doc.Caveats)
	}
	if strings.Contains(doc.Caveats[0], "text kept") || !strings.Contains(doc.Caveats[0], "removed together with their content") {
		t.Fatalf("english removed disclosure must state removal and not retention, got %q", doc.Caveats[0])
	}
}

// Review finding 6 — the disclosure is materialized at the PERSIST
// chokepoint, not at chain end: after the pre-emit chain the doc carries no
// detach caveat yet; ApplyAndPersistMutation (which still runs the
// pre-persist enumeration normalization + block dedupe) materializes it
// from the final merged state. The specimen failure shape — retention
// wording surviving a later content deletion — is structurally impossible
// because no wording exists until the last content mutation has run.
func TestQCEFullChain_DisclosureMaterializedAtPersistPoint(t *testing.T) {
	ctx := qceSpecimenCtx()
	doc := qceSpecimenDoc()
	doc.Blocks[1].Items = append(doc.Blocks[1].Items, types.AnswerBlockItem{
		ID: "bogus", Label: "StageBogus (bogus_agent)", Text: "不存在的阶段。", CitationRef: 4,
	})
	normalizeAnswerDocumentForPreEmit("qce_test", doc, &types.AnswerSemanticView{}, ctx, nil)

	for _, caveat := range doc.Caveats {
		if strings.Contains(caveat, "条目内容保留") || strings.Contains(caveat, "一并移除") {
			t.Fatalf("no detach disclosure may exist before the persist point, got %q", caveat)
		}
	}

	res, err := ApplyAndPersistMutation(ctx, "emit_answer_document", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist failed: res=%+v err=%v", res, err)
	}
	persisted := ctx.Mutable.AnswerDocumentV2()
	if persisted == nil {
		t.Fatal("no persisted document")
	}
	pre := qceFindBlock(t, persisted, "pre_stages")
	if pre == nil || len(pre.Items) != 3 {
		t.Fatalf("typed-backed items must survive the full chain while the bogus row is pruned: %+v", pre)
	}
	for i, wantRef := range []int{4, 5, 6} {
		if got := pre.Items[i].CitationRef; got != wantRef {
			t.Fatalf("full chain: typed-backed item %d must keep ref %d, got %d", i, wantRef, got)
		}
	}
	var removalCaveat string
	for _, caveat := range persisted.Caveats {
		if strings.Contains(caveat, "条目内容保留") {
			t.Fatalf("persisted disclosure claims retention for removed content: %q", caveat)
		}
		if strings.Contains(caveat, "一并移除") {
			removalCaveat = caveat
		}
	}
	if removalCaveat == "" {
		t.Fatalf("detached-then-pruned item must be disclosed as removed at persist, caveats=%v", persisted.Caveats)
	}
}

// GAP-B pin — precise gate for the post-repair quote passes: only bare
// single-line current-source citations open it; runtime-artifact citations
// never keep it open.
func TestQCEQuotelessCitationGate(t *testing.T) {
	if answerDocumentHasQuotelessCurrentSourceCitation(nil, nil) {
		t.Fatal("nil doc must not trigger the backfill pass")
	}
	doc := &types.AnswerDocumentV2{Citations: []types.Citation{
		{File: "a.go", Line: 3, Quote: "quoted line"},
		{File: "a.go", Line: 4, LineEnd: 9},                       // ranged: pass-ineligible
		{File: "a.go", Line: 5, NegativePattern: "not-found-pat"}, // negative: pass-ineligible
	}}
	if answerDocumentHasQuotelessCurrentSourceCitation(doc, nil) {
		t.Fatal("no eligible quoteless citation → gate must stay closed (no source re-reads)")
	}
	doc.Citations = append(doc.Citations, types.Citation{File: "a.go", Line: 6})
	if !answerDocumentHasQuotelessCurrentSourceCitation(doc, nil) {
		t.Fatal("bare file:line citation must open the gate")
	}

	artifactDoc := &types.AnswerDocumentV2{Citations: []types.Citation{
		{File: "traces/big.systrace", Line: 12},
	}}
	artifactCtx := &types.BusContext{AttachedHitraceSource: "traces/big.systrace"}
	if answerDocumentHasQuotelessCurrentSourceCitation(artifactDoc, artifactCtx) {
		t.Fatal("a runtime-artifact citation must not keep the backfill gate open")
	}
}

// GAP-B pin — a quoteless citation minted after the first quote pass (the
// repair-chain shape) is backfilled with the source-line quote by the gated
// pass at the end of the pre-emit chain.
func TestQCERepairChainMintedCitationGetsQuoteBackfill(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "enums.go"),
		[]byte("package pkg\nconst StageLogTriage = \"log_triage\"\nconst StagePerfTriage = \"perf_triage\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("architecture question")
	ctx := &types.BusContext{RepoRoot: dir, Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "b1",
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{
				{ID: "i1", Label: "StageLogTriage", Text: "pre-stage", CitationRef: 0},
			},
		}},
		Citations: []types.Citation{
			// Simulates appendOrReusePreEmitCitation output: file:line, no quote.
			{File: "pkg/enums.go", Line: 2},
		},
	}
	normalizeAnswerDocumentForPreEmit("qce_test", doc, &types.AnswerSemanticView{}, ctx, nil)
	if got := strings.TrimSpace(doc.Citations[0].Quote); got != `const StageLogTriage = "log_triage"` {
		t.Fatalf("quoteless citation not backfilled at chain end, got %q", got)
	}
}

// GAP-B pin — the pre-persist row normalization can mint quoteless
// citations too; persistMergedAnswerDocument backfills before persist.
func TestQCEPrePersistQuoteBackfill(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "enums.go"),
		[]byte("package pkg\nconst StageLogTriage = \"log_triage\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("architecture question")
	ctx := &types.BusContext{RepoRoot: dir, Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:    "b1",
			Kind:  types.BlockBulletList,
			Items: []types.AnswerBlockItem{{ID: "i1", Label: "StageLogTriage", Text: "pre-stage", CitationRef: 0}},
		}},
		Citations: []types.Citation{{File: "pkg/enums.go", Line: 2}},
	}
	res, err := ApplyAndPersistMutation(ctx, "emit_answer_document", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist failed: res=%+v err=%v", res, err)
	}
	persisted := ctx.Mutable.AnswerDocumentV2()
	if persisted == nil || len(persisted.Citations) == 0 {
		t.Fatal("no persisted document/citations")
	}
	if got := strings.TrimSpace(persisted.Citations[0].Quote); got != `const StageLogTriage = "log_triage"` {
		t.Fatalf("pre-persist backfill missing, got %q", got)
	}
}
