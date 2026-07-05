package tool

// CPD #58 (2026-07-05) — citation POOL detach downgrade batch pins.
//
// Specimen: eval/results/trace_query_donghu_real_frame_multicausal-20260703-111818
// ("只分析这份 trace，不分析代码"): the model emitted 4 pseudo-citations naming the
// blob materialization path `.codrax/blob/<session>/attached_trace.txt:<line>`;
// an incidental current-source ledger record set CurrentSourceSatisfied, the
// derived authority projection vetoed pool cleanup, and the final answer
// rendered a source bibliography of machine-local trace paths directly above a
// system supplement declaring they are NOT repository source citations.
//
// Pinned here:
//  1. Typed user boundary (ExcludesCurrentSource) + artifact context opens the
//     pool cleanup regardless of the derived authority projection, blob-path
//     pseudo-citations are fully dropped, and each detached item rides the QCE
//     §7.13 disclosure ferry on the runtime_artifact wording lane.
//  2. Ruling pin: MIXED runtime+current-source runs do NOT detach — real
//     source citations and even artifact-spelled pool entries stay untouched
//     without the typed exclude boundary (citation-floor interaction is real
//     there; see docs/design/citation_pool_detach_evaluation_20260705.md §7).
//  3. Lane alignment: the typed spelling set recognizes the reserved blob
//     basenames, so the read-skip lane and the pool-cleanup lane can no longer
//     diverge on blob-path citations.
//  4. Persist-time wording: runtime_artifact records disclose with artifact
//     wording (kept vs removed variants), coexisting with — never rewording —
//     the legacy unverifiable-source lane.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const cpdBlobCitationPath = ".codrax/blob/20260703-111820-000-5208/attached_trace.txt"

func cpdTypedExcludeTraceBusContext() *types.BusContext {
	return &types.BusContext{
		Mutable:               types.NewMutableState("q"),
		AttachedHitrace:       "tracer: nop\n<idle>-0 [000] 34579.472865: sched_switch ...",
		AttachedHitraceSource: "../../customlogs/xxx_all.systrace",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode:    types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:        types.ExternalObservationSourceExclusionExplicitUserBoundary,
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				SourceQuotes:         []string{"只分析这份 trace，不分析代码"},
			},
		}},
	}
}

// cpdSeedIncidentalCurrentSourceRecord reproduces the specimen's gate-closure
// mechanism: an incidental current-source ledger record in a typed-exclude
// run sets authority.CurrentSourceSatisfied (run log:
// `current_source_lane=excluded; current_source_required=false;
// current_source_satisfied=true`), and KeepsCurrentSourceLaneLoadBearing then
// vetoes every derived cleanup arm. The CPD #58 typed-boundary arm must
// outrank exactly this veto.
func cpdSeedIncidentalCurrentSourceRecord(t *testing.T, ctx *types.BusContext) {
	t.Helper()
	evidence := []types.EvidenceItem{{
		ID:              "incidental-current-source",
		Kind:            types.EvidenceDirect,
		Origin:          types.ClaimOriginCurrentRepo,
		Source:          "internal/runtime/init.go",
		LineStart:       44,
		LineEnd:         52,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "init",
		Subject:         "init",
		Summary:         "incidental current-source record in a 不分析代码 run",
		GroundingStatus: types.GroundingGrounded,
	}}
	ctx.Mutable.AppendEvidence(evidence)
	ctx.EvidenceItems = evidence
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if !authority.Active || !authority.CurrentSourceSatisfied {
		t.Fatalf("fixture must reproduce the specimen veto (Active + CurrentSourceSatisfied), got %+v", authority)
	}
}

func cpdSpecimenDoc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: cpdBlobCitationPath, Line: 2917, Quote: "CookieMonsterCl-59843 was runnable for 25.847ms (cpu=0)"},
			// The specimen's model-typo'd year variant — still a reserved
			// blob basename, still runtime provenance.
			{File: ".codrax/blob/20270703-111820-000-5208/attached_trace.txt", Line: 15620, Quote: "print: B|60194|Choreographer#onVsync 76797"},
			{File: cpdBlobCitationPath, Line: 8712},
			{File: cpdBlobCitationPath, Line: 6673},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "chain",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{ID: "hop1", Label: "ThreadPoolForeg-60555", Text: "链上 D/io_wait 主因。", CitationRef: 2},
				{ID: "hop2", Label: "NetworkService-60595", Text: "链上 runnable 主因。", CitationRef: 3},
				{ID: "hop3", Label: "CookieMonsterCl-59843", Text: "直接唤醒者。", CitationRef: 0},
				{ID: "hop4", Label: "com.baidu.tieba-59566（主线程）", Text: "被阻塞帧主体。", CitationRef: 1},
			},
		}},
	}
}

// Pin 1: specimen replay — typed exclude boundary drops every blob-path
// pseudo-citation from the pool, detaches the refs, and records one
// runtime_artifact disclosure per affected item. With the pool empty, the
// rendered answer has no citation bibliography — zero machine-local paths.
func TestNormalizeRuntimeArtifactCitationRefs_TypedExcludeBoundaryDropsBlobPseudoCitations(t *testing.T) {
	ctx := cpdTypedExcludeTraceBusContext()
	cpdSeedIncidentalCurrentSourceRecord(t, ctx)
	doc := cpdSpecimenDoc()
	pctx := newPreEmitCheckContext(ctx)

	fixed := normalizeRuntimeArtifactCitationRefsWithContext(doc, ctx, pctx)
	if fixed == 0 {
		t.Fatal("typed exclude boundary must open the pool cleanup (CPD #58 arm); fixed=0")
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("blob pseudo-citations must not remain in the pool: %+v", doc.Citations)
	}
	for _, item := range doc.Blocks[0].Items {
		if item.CitationRef != -1 {
			t.Fatalf("item %s citation_ref = %d, want -1", item.ID, item.CitationRef)
		}
		if item.Text == "" || item.Label == "" {
			t.Fatalf("item %s visible content must be preserved", item.ID)
		}
	}

	recs := pctx.detachedCitationDisclosures()
	if len(recs) != 4 {
		t.Fatalf("disclosure records = %d, want 4 (one per detached item)", len(recs))
	}
	for _, rec := range recs {
		if rec.Kind != types.DetachedCitationKindRuntimeArtifact {
			t.Fatalf("disclosure kind = %q, want runtime_artifact", rec.Kind)
		}
		if rec.BlockID != "chain" || rec.ItemID == "" {
			t.Fatalf("disclosure identity incomplete: %+v", rec)
		}
	}

	// Persist-time wording: items still visible → artifact "kept" wording,
	// and the disclosure never claims an unverifiable source.
	materializeDetachedCitationRefCaveats(doc, ctx, recs)
	joined := strings.Join(doc.Caveats, "\n")
	if !strings.Contains(joined, "指向附件运行时材料") || !strings.Contains(joined, "条目内容保留") {
		t.Fatalf("runtime-artifact kept wording missing: %q", joined)
	}
	if strings.Contains(joined, "无法对应到任何已验证来源") {
		t.Fatalf("artifact detach must not ride the unverifiable-source wording lane: %q", joined)
	}
}

// Pin 2 (ruling): mixed runtime+current-source runs make NO pool detach —
// neither the real source citation nor the artifact-spelled entry moves, and
// nothing is recorded on the disclosure ferry. The CPD #58 arm requires the
// typed exclude boundary; derived mixed posture alone never mutates the pool
// (citation-floor interaction is live in mixed runs — evaluation §6/§7).
func TestNormalizeRuntimeArtifactCitationRefs_MixedRunMakesNoPoolDetach(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "timeout"}},
	})
	ctx := &types.BusContext{
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
		},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/llm/openai.go", Line: 608},
			{File: cpdBlobCitationPath, Line: 2917},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "当前源码说明 first byte watchdog 如何取消请求。",
			Items: []types.AnswerBlockItem{
				{ID: "current", Label: "watchdog", CitationRef: 0},
				{ID: "observed", Label: "运行时观测", CitationRef: 1},
			},
		}},
	}
	pctx := newPreEmitCheckContext(ctx)

	if fixed := normalizeRuntimeArtifactCitationRefsWithContext(doc, ctx, pctx); fixed != 0 {
		t.Fatalf("mixed run must not detach pool entries, fixed=%d doc=%+v", fixed, doc)
	}
	if len(doc.Citations) != 2 {
		t.Fatalf("mixed-run pool must stay untouched: %+v", doc.Citations)
	}
	if doc.Blocks[0].Items[0].CitationRef != 0 || doc.Blocks[0].Items[1].CitationRef != 1 {
		t.Fatalf("mixed-run citation_refs must stay untouched: %+v", doc.Blocks[0].Items)
	}
	if recs := pctx.detachedCitationDisclosures(); len(recs) != 0 {
		t.Fatalf("mixed run must not record detach disclosures: %+v", recs)
	}
}

// Pin 3: lane alignment — the typed spelling set recognizes the reserved blob
// basenames exactly like the path-shape lane, so a blob-path citation can no
// longer be skipped by one artifact lane and missed by the other.
func TestRuntimeArtifactCitationPathSet_IncludesReservedBlobBasenames(t *testing.T) {
	set := runtimeArtifactCitationPathSet(&types.BusContext{AttachedHitraceSource: "../../customlogs/xxx_all.systrace"})
	blobPaths := []string{
		"/abs/repo/.codrax/blob/20260703-111820-000-5208/attached_trace.txt",
		".codrax/blob/s/attached_hitrace.txt",
		"blob/attached_atrace.txt",
		"work/attached_log.txt",
	}
	for _, p := range blobPaths {
		if !citationFileIsRuntimeArtifact(set, p) {
			t.Fatalf("typed spelling lane must recognize reserved blob path %q", p)
		}
		if !types.LooksLikeRuntimeArtifactPath(p) {
			t.Fatalf("shape lane regressed on reserved blob path %q", p)
		}
	}
	if !citationFileIsRuntimeArtifact(set, "../../customlogs/xxx_all.systrace") {
		t.Fatal("user attachment spelling must stay recognized")
	}
	if citationFileIsRuntimeArtifact(set, "internal/tool/builtin.go") {
		t.Fatal("repo source path must never match the artifact spelling set")
	}
}

// Pin 4: persist-time wording flips with the item's FINAL presence on the
// runtime_artifact lane too, and the two wording lanes coexist without
// rewording each other.
func TestMaterializeDetachedCitationRefCaveats_RuntimeArtifactWordingLanes(t *testing.T) {
	ctx := cpdTypedExcludeTraceBusContext()

	// (a) item removed by a later structural pass → "removed" wording.
	gone := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "chain", Kind: types.BlockOrderedList}}}
	materializeDetachedCitationRefCaveats(gone, ctx, []types.DetachedCitationDisclosure{{
		BlockID: "chain", ItemID: "hop1", Label: "ThreadPoolForeg-60555",
		Kind: types.DetachedCitationKindRuntimeArtifact,
	}})
	joined := strings.Join(gone.Caveats, "\n")
	if !strings.Contains(joined, "指向附件运行时材料") || !strings.Contains(joined, "一并移除") {
		t.Fatalf("runtime-artifact removed wording missing: %q", joined)
	}
	if strings.Contains(joined, "条目内容保留") {
		t.Fatalf("removed item must not claim kept content (GAP-A red line): %q", joined)
	}

	// (b) mixed kinds → both lanes disclose with their own wording.
	both := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "chain",
		Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{
			{ID: "hop1", Label: "ThreadPoolForeg-60555", Text: "kept"},
			{ID: "legacy", Label: "watchdog", Text: "kept"},
		},
	}}}
	materializeDetachedCitationRefCaveats(both, ctx, []types.DetachedCitationDisclosure{
		{BlockID: "chain", ItemID: "hop1", Kind: types.DetachedCitationKindRuntimeArtifact},
		{BlockID: "chain", ItemID: "legacy"},
	})
	joined = strings.Join(both.Caveats, "\n")
	if !strings.Contains(joined, "指向附件运行时材料") {
		t.Fatalf("runtime-artifact lane missing in mixed disclosure: %q", joined)
	}
	if !strings.Contains(joined, "无法对应到任何已验证来源") {
		t.Fatalf("legacy lane wording must stay byte-stable in mixed disclosure: %q", joined)
	}
}
