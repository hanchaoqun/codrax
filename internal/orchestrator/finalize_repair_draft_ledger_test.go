package orchestrator

// FRCAP pins (§29.12 ruling, docs/design/real_trace_campaign_20260705.md,
// 2026-07-10). Covers:
//
//	best-draft selection — fewest hard violations wins; ties select the
//	  EARLIEST draft (user ruling: later small-model rounds are not
//	  presumed better);
//	ledger bound — at most hardCap+1 retained drafts; empty answers are
//	  never recorded;
//	restore — the selected draft's rendered answer ships and the
//	  structured document is restored into MutableState so downstream
//	  re-renders read the SAME draft;
//	telemetry — the per-answer "finalize repair rounds used=N cap=M"
//	  line format is pinned (revisit transcripts grep for it);
//	config migration — the formerly hardcoded same-error-class /
//	  per-root caps keep their shipped defaults byte-for-byte and
//	  clamp defensively.

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func frcapLedgerWith(counts ...int) []finalizeRepairDraftRecord {
	out := make([]finalizeRepairDraftRecord, 0, len(counts))
	for i, c := range counts {
		out = append(out, finalizeRepairDraftRecord{
			Round:          i,
			Answer:         "draft",
			HardViolations: c,
		})
	}
	return out
}

func TestFRCAPSelectBestDraft_FewestHardViolations(t *testing.T) {
	if got := selectBestFinalizeRepairDraft(frcapLedgerWith(3, 1, 2)); got != 1 {
		t.Fatalf("fewest hard violations must win, got index %d", got)
	}
	if got := selectBestFinalizeRepairDraft(nil); got != -1 {
		t.Fatalf("empty ledger must select nothing, got %d", got)
	}
}

// TestFRCAPSelectBestDraft_TieEarliest — §29.12 item 3: on equal hard
// violation counts, the EARLIEST draft ships. All-equal ledgers select
// round 0. Mutating the comparison to <= (tie → latest) turns this red.
func TestFRCAPSelectBestDraft_TieEarliest(t *testing.T) {
	if got := selectBestFinalizeRepairDraft(frcapLedgerWith(2, 1, 1)); got != 1 {
		t.Fatalf("tie must select the earliest of the tied drafts, got index %d", got)
	}
	if got := selectBestFinalizeRepairDraft(frcapLedgerWith(2, 2, 2)); got != 0 {
		t.Fatalf("all-equal ledger must select the first draft, got index %d", got)
	}
}

// TestFRCAPRecordDraft_BoundAndEmptySkip — the ledger is structurally
// bounded at hardCap+1 entries and never records an empty answer.
func TestFRCAPRecordDraft_BoundAndEmptySkip(t *testing.T) {
	mut := types.NewMutableState("q")
	var ledger []finalizeRepairDraftRecord
	vio := []types.Violation{{Kind: types.ViolBlockCoverageMissing}}
	for round := 0; round < 6; round++ {
		out := &agent.StageOutput{FinalAnswer: "draft"}
		ledger = recordFinalizeRepairDraft(ledger, out, mut, vio, round, 2)
	}
	if len(ledger) != 3 {
		t.Fatalf("hardCap=2 must bound the ledger at 3 drafts, got %d", len(ledger))
	}
	if got := recordFinalizeRepairDraft(ledger[:0], &agent.StageOutput{FinalAnswer: "   "}, mut, vio, 0, 2); len(got) != 0 {
		t.Fatalf("blank answers must not be recorded, got %d", len(got))
	}
	// Violation slice is deep-copied (a later round mutating its result
	// must not rewrite an earlier record's ledger).
	src := []types.Violation{{Kind: types.ViolBlockCoverageMissing, Detail: "a"}}
	got := recordFinalizeRepairDraft(nil, &agent.StageOutput{FinalAnswer: "x"}, mut, src, 0, 2)
	src[0].Detail = "mutated"
	if got[0].Violations[0].Detail != "a" {
		t.Fatalf("recorded violations must be a deep copy")
	}
	if got[0].HardViolations != 1 || got[0].Hash == "" {
		t.Fatalf("record must carry the typed count and the audit hash: %+v", got[0])
	}
}

// TestFRCAPRestoreDraft_StringFallbackAndDocRestore — restore without a
// stored document falls back to the recorded rendered answer verbatim;
// restore WITH a document writes it back into MutableState (downstream
// re-renders must read the same draft) and re-renders the answer
// through the last-mile chokepoint.
func TestFRCAPRestoreDraft_StringFallbackAndDocRestore(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Ctx: context.Background(), Mutable: mut, Language: "zh"}
	o := &Orchestrator{busCtx: bus}

	// String fallback (no DocJSON).
	out := &agent.StageOutput{FinalAnswer: "latest draft"}
	rec := &finalizeRepairDraftRecord{Round: 0, Answer: "earliest draft"}
	if !o.restoreFinalizeRepairDraft(out, rec) {
		t.Fatalf("restore must succeed on the string fallback")
	}
	if out.FinalAnswer != "earliest draft" {
		t.Fatalf("string fallback must ship the recorded answer, got %q", out.FinalAnswer)
	}

	// Doc restore: the best draft's document lands back in MutableState.
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
		Text: "最优稿正文。",
	}}}
	var ledger []finalizeRepairDraftRecord
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	ledger = recordFinalizeRepairDraft(ledger, &agent.StageOutput{FinalAnswer: "最优稿正文。"}, mut,
		[]types.Violation{{Kind: types.ViolBlockCoverageMissing}}, 0, 2)
	if len(ledger) != 1 || len(ledger[0].DocJSON) == 0 {
		t.Fatalf("record must capture the structured doc, got %+v", ledger)
	}
	// A later round replaces the doc in MutableState…
	later := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
		Text: "后轮劣化稿。",
	}}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, later)
	// …and the restore swaps the earlier draft back.
	out2 := &agent.StageOutput{FinalAnswer: "后轮劣化稿。"}
	if !o.restoreFinalizeRepairDraft(out2, &ledger[0]) {
		t.Fatalf("doc restore must succeed")
	}
	if !strings.Contains(out2.FinalAnswer, "最优稿正文。") {
		t.Fatalf("restored answer must render the best draft, got %q", out2.FinalAnswer)
	}
	restored := mut.AnswerDocumentV2()
	if restored == nil || len(restored.Blocks) == 0 || !strings.Contains(restored.Blocks[0].Text, "最优稿正文。") {
		t.Fatalf("the best draft's document must be restored into MutableState, got %+v", restored)
	}
}

// TestFinalizeRepairRoundsTelemetryFormatPinned — §29.12 item 4: every
// answer logs one auditable repair-budget line; revisit transcript
// tooling greps for this exact prefix. P3c wording: the counter is the
// cross-stage scheduler retry budget (the exact value the P6 cap
// compares), so the label must not overclaim a finalize-only count.
func TestFinalizeRepairRoundsTelemetryFormatPinned(t *testing.T) {
	const want = "[orchestrator] finalize repair budget used=%d cap=%d (cross-stage)"
	if finalizeRepairRoundsTelemetryFormat != want {
		t.Fatalf("telemetry format drifted:\nwant %q\ngot  %q", want, finalizeRepairRoundsTelemetryFormat)
	}
}

// TestSetSameErrorClassRetryCap — FRCAP config migration: default 1
// (byte-identical to the retired hardcoded literal), 0 = explicit
// disable, negative clamps back to the default.
func TestSetSameErrorClassRetryCap(t *testing.T) {
	t.Cleanup(func() { SetSameErrorClassRetryCap(sameErrorClassRetryCapDefault) })
	if got := sameErrorClassRetryCap(); got != 1 {
		t.Fatalf("shipped default must stay 1, got %d", got)
	}
	SetSameErrorClassRetryCap(3)
	if got := sameErrorClassRetryCap(); got != 3 {
		t.Fatalf("setter(3) → %d", got)
	}
	SetSameErrorClassRetryCap(0)
	if got := sameErrorClassRetryCap(); got != 0 {
		t.Fatalf("explicit 0 disables the class governor, got %d", got)
	}
	SetSameErrorClassRetryCap(-2)
	if got := sameErrorClassRetryCap(); got != sameErrorClassRetryCapDefault {
		t.Fatalf("negative must clamp to default, got %d", got)
	}
}

// TestSetMaxRepairAttemptsPerRoot — FRCAP config migration: default is
// the types constant (3); non-positive clamps back (a 0 cap would
// declare every root exhausted on its first attempt — never allowed).
func TestSetMaxRepairAttemptsPerRoot(t *testing.T) {
	t.Cleanup(func() { SetMaxRepairAttemptsPerRoot(types.MaxRepairAttemptsPerRoot) })
	if maxRepairAttemptsPerRootValue != types.MaxRepairAttemptsPerRoot {
		t.Fatalf("shipped default must stay types.MaxRepairAttemptsPerRoot (%d), got %d",
			types.MaxRepairAttemptsPerRoot, maxRepairAttemptsPerRootValue)
	}
	SetMaxRepairAttemptsPerRoot(5)
	if maxRepairAttemptsPerRootValue != 5 {
		t.Fatalf("setter(5) → %d", maxRepairAttemptsPerRootValue)
	}
	SetMaxRepairAttemptsPerRoot(0)
	if maxRepairAttemptsPerRootValue != types.MaxRepairAttemptsPerRoot {
		t.Fatalf("0 must clamp to the types default, got %d", maxRepairAttemptsPerRootValue)
	}

	// Behaviour equivalence at the default: IncrementAttemptsAndCheckExhausted
	// exhausts a root exactly at the types constant, as before the migration.
	h := &types.RepairAttemptHistory{}
	vio := []types.Violation{{Kind: types.ViolBlockCoverageMissing, ClusterKey: "k"}}
	for i := 1; i < types.MaxRepairAttemptsPerRoot; i++ {
		if IncrementAttemptsAndCheckExhausted(vio, h) {
			t.Fatalf("attempt %d must not exhaust yet", i)
		}
	}
	if !IncrementAttemptsAndCheckExhausted(vio, h) {
		t.Fatalf("attempt %d must exhaust the root", types.MaxRepairAttemptsPerRoot)
	}
}
