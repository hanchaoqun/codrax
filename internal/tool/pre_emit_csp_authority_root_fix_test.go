package tool

// CSP #63 (2026-07-05) — authority-lane root fix + blob spelling case-fold pins.
//
// Companion to pre_emit_cpd_pool_detach_test.go (CPD #58). That file pins the
// DISPLAY-LAYER arm (typed user boundary short-circuits the derived authority
// veto). This file pins the ROOT fix underneath it:
//
//  1. Donghu shape (model aggregate facts only, zero source reads): the
//     compiled authority itself reports CurrentSourceSatisfied=false and the
//     runtime citation cleanup gate opens through the DERIVED authority lane —
//     the CPD arm is a redundant defense line for this shape, not the only
//     thing keeping the customer answer clean. (The arm stays: the CPD #58
//     evidence-item shape — a genuine incidental source read — still relies
//     on it, and an explicit user boundary must keep outranking derived
//     authority regardless.)
//  2. Case-fold (CPD 核验 P3): the typed blob spelling set matches
//     case-variant artifact citations exactly like the path-shape lane
//     (RuntimeArtifactPathKind lowercases), so the three faces that consult
//     only the typed set — citation quote normalize (read + quoteless gate)
//     and current-source metadata surface terms — skip `Attached_Trace.txt`
//     variants instead of reading them as source files, while case-variant
//     REAL source paths keep full processing.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// cspDonghuToolAggregateFacts mirrors the specimen handoff: trace-derived
// facts, no origin dims, no support refs (see the types-side twin fixture
// cspDonghuAggregateFacts; duplicated here because the tool package cannot
// import types test fixtures).
func cspDonghuToolAggregateFacts() []types.AnswerAggregateFact {
	return []types.AnswerAggregateFact{
		{Kind: types.AnswerAggregateScalar, Label: "帧窗口总时长", Value: "114.94 ms"},
		{Kind: types.AnswerAggregateMemberSet, Label: "主线程唤醒链节点", Value: "4",
			Members: []string{"ThreadPoolForeg-60555", "NetworkService-60595", "CookieMonsterCl-59843", "com.baidu.tieba-59566"}},
		{Kind: types.AnswerAggregateScalar, Label: "主线程最大单次唤醒延迟", Value: "11.103 ms"},
		{Kind: types.AnswerAggregateScalar, Label: "CPU 0 runnable 等待总时长", Value: "389.746 ms"},
		{Kind: types.AnswerAggregateScalar, Label: "IO 最长延迟", Value: "110.660 ms"},
		{Kind: types.AnswerAggregateMemberSet, Label: "优先级反转候选", Value: "2",
			Members: []string{"CookieMonsterCl-59843 (prio=20)", "NetworkService-60595 (prio=20)"}},
		{Kind: types.AnswerAggregateBucketCount, Label: "主线程在窗口内的状态段数量", Value: "12"},
		{Kind: types.AnswerAggregateScalar, Label: "ThreadPoolForeg-60555 D-sleep 时长", Value: "6.768 ms"},
		{Kind: types.AnswerAggregateScalar, Label: "主线程与 Binder 的同步 binder 等待", Value: "11.103 ms"},
		{Kind: types.AnswerAggregateBucketCount, Label: "主线程被唤醒次数", Value: "34"},
	}
}

// Pin 1: donghu shape — the derived authority lane ALONE opens the pool
// cleanup after the root fix. No incidental current-source record is seeded;
// the model's aggregate facts are the only ledger producers, exactly like the
// specimen run (0 evidence, 0 readFiles, trace_query only).
func TestRuntimeCitationCleanup_DonghuShapeOpensThroughDerivedAuthority(t *testing.T) {
	ctx := cpdTypedExcludeTraceBusContext()
	ctx.Mutable.SetInvestigationAggregateFacts(cspDonghuToolAggregateFacts())
	ctx.Mutable.RetainInvestigationAggregateFacts()

	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if !authority.Active {
		t.Fatalf("donghu shape must keep the authority active: %+v", authority)
	}
	if authority.CurrentSourceRecordCount != 0 || authority.CurrentSourceSatisfied {
		t.Fatalf("exclude-run aggregate facts polluted the current-source lane again (CSP #63 regression): %+v", authority)
	}
	if authority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("derived authority still vetoes cleanup for the donghu shape: %+v", authority)
	}
	// The derived-authority gate arms — the exact pair the CPD arm had to
	// short-circuit pre-fix — now open by themselves.
	if !runtimeSourceAuthorityAppliesToArtifactCitationCleanup(ctx, authority) {
		t.Fatalf("authority cleanup lane must apply to the donghu shape: %+v", authority)
	}
	if !runtimeSourceAuthorityAllowsArtifactCitationCleanup(authority) {
		t.Fatalf("authority cleanup lane must allow the donghu cleanup (arm redundancy): %+v", authority)
	}

	// Full replay: every blob-path pseudo-citation drops, refs detach.
	doc := cpdSpecimenDoc()
	if fixed := normalizeRuntimeArtifactCitationRefsWithContext(doc, ctx, newPreEmitCheckContext(ctx)); fixed == 0 {
		t.Fatal("donghu replay cleanup gate did not open")
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("blob pseudo-citations must not survive: %+v", doc.Citations)
	}
	for _, item := range doc.Blocks[0].Items {
		if item.CitationRef != -1 {
			t.Fatalf("item %s citation_ref = %d, want -1", item.ID, item.CitationRef)
		}
	}
}

// Pin 2a: the typed spelling set is case-folded on both sides, aligned with
// the shape lane; real source paths never match.
func TestRuntimeArtifactCitationPathSet_CaseFoldAlignsWithShapeLane(t *testing.T) {
	set := runtimeArtifactCitationPathSet(&types.BusContext{AttachedHitraceSource: "../../customlogs/xxx_all.systrace"})
	for _, variant := range []string{
		"Attached_Trace.txt",
		"ATTACHED_TRACE.TXT",
		".codrax/blob/20260703-111820-000-5208/Attached_Trace.txt",
		"/abs/repo/.codrax/blob/s/ATTACHED_HITRACE.TXT",
		"Attached_Log.TXT",
		"../../customlogs/XXX_All.SYSTRACE",
	} {
		if !citationFileIsRuntimeArtifact(set, variant) {
			t.Fatalf("typed spelling lane must case-fold %q (shape lane already lowercases)", variant)
		}
	}
	for _, source := range []string{
		"internal/tool/builtin.go",
		"Internal/Tool/Builtin.GO",
		"cmd/Root.go",
	} {
		if citationFileIsRuntimeArtifact(set, source) {
			t.Fatalf("real source path %q must never match the artifact spelling set", source)
		}
	}
}

func cspCaseFoldRepoContext(t *testing.T) (*types.BusContext, string) {
	t.Helper()
	root := t.TempDir()
	// A case-variant of the reserved blob basename materialized inside the
	// repo, plus a case-variant real source file.
	if err := os.WriteFile(filepath.Join(root, "Attached_Trace.txt"), []byte("@TraceMeta\ntrace body line two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "UTIL.Go"), []byte("@Component\nfunc CaseVariant() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mut := types.NewMutableState("q")
	ctx := &types.BusContext{
		Mutable:  mut,
		RepoRoot: root,
		// Non-exclude mixed posture: the current-source read faces are live.
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}},
	}
	return ctx, root
}

// Pin 2b: all three consult-only faces skip case-variant blob citations and
// keep processing case-variant real source files.
func TestCurrentSourceReadFaces_SkipCaseVariantBlobCitations(t *testing.T) {
	ctx, _ := cspCaseFoldRepoContext(t)

	// Face 1 — normalizeCurrentSourceCitationQuotes (:48): the blob-variant
	// quote stays model-authored (skipped), the source-variant quote is
	// repaired from the file.
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "Attached_Trace.txt", Line: 2, Quote: "model quote"},
			{File: "UTIL.Go", Line: 2, Quote: "stale quote"},
		},
	}
	fixed := normalizeCurrentSourceCitationQuotes(doc, ctx)
	if doc.Citations[0].Quote != "model quote" {
		t.Fatalf("case-variant blob citation was read as source (face 1): %+v", doc.Citations[0])
	}
	if fixed != 1 || doc.Citations[1].Quote != "func CaseVariant() {}" {
		t.Fatalf("case-variant real source file must keep quote repair: fixed=%d cit=%+v", fixed, doc.Citations[1])
	}

	// Face 2 — answerDocumentHasQuotelessCurrentSourceCitation (:158): a
	// quoteless blob-variant citation must not hold the gate open; a
	// quoteless source-variant citation must.
	blobOnly := &types.AnswerDocumentV2{Citations: []types.Citation{{File: ".codrax/blob/s/Attached_Trace.TXT", Line: 3}}}
	if answerDocumentHasQuotelessCurrentSourceCitation(blobOnly, ctx) {
		t.Fatal("case-variant blob citation held the quoteless gate open (face 2)")
	}
	sourceOnly := &types.AnswerDocumentV2{Citations: []types.Citation{{File: "UTIL.Go", Line: 1}}}
	if !answerDocumentHasQuotelessCurrentSourceCitation(sourceOnly, ctx) {
		t.Fatal("case-variant real source citation must keep the quoteless gate open (face 2)")
	}

	// Face 3 — currentSourceMetadataSurfaceTermEvidence (pre_emit_check:8439):
	// the blob-variant citation contributes no metadata evidence even though
	// its first line is metadata-shaped; the source-variant citation does.
	blobDoc := &types.AnswerDocumentV2{Citations: []types.Citation{{File: "Attached_Trace.txt", Line: 1}}}
	if rows := currentSourceMetadataSurfaceTermEvidence(ctx, blobDoc); len(rows) != 0 {
		t.Fatalf("case-variant blob citation produced metadata evidence (face 3): %+v", rows)
	}
	sourceDoc := &types.AnswerDocumentV2{Citations: []types.Citation{{File: "UTIL.Go", Line: 1}}}
	if rows := currentSourceMetadataSurfaceTermEvidence(ctx, sourceDoc); len(rows) == 0 {
		t.Fatal("case-variant real source citation must keep metadata evidence (face 3)")
	}
}
