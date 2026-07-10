package orchestrator

// Marker-stripping class root fix pins — orchestrator lanes (audit
// 2026-07-10, MARKER batch; findings #10/#57 FRCAP DocJSON, #69 recovery
// draft, #11 terminal caveat appenders).
//
// SystemGeneratedKind is json:"-", so the FRCAP draft ledger and
// RetryState.PrevEmitJSON snapshots stripped the authority marker from
// genuine runtime-trace system blocks. Pre-fix shapes pinned RED here:
//   - FRCAP best-draft restore shipped the selected draft with its
//     system chapters demoted (## → ###) and the PSG ship-time re-scan
//     listed the system's own deterministic numerals in a user-visible
//     "未能定位于证据面" caveat (false self-discredit);
//   - the finalizer recovery draft decoded from PrevEmitJSON raised a
//     false ViolProseScalarUngrounded on system-published numerals,
//     aborting an otherwise-valid (beneficial) recovery;
//   - the two terminal string-channel caveat appenders bypassed the
//     CAVSTR replay register, so any later FinalAnswer re-render
//     silently dropped the disclosure.

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	answertool "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// reauthTraceDoc builds the witness document: model prose states a
// numeral that ONLY the marked deterministic system block publishes.
func reauthTraceDoc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
			Text: "聚合影响达 46.821ms，显著延长了端到端延迟。"},
		{ID: "runtime_trace_metric_snapshot", Kind: types.BlockSection,
			Title: "关键指标核对", Text: "聚合影响 46.821ms",
			SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace},
	}}
}

// TestFRCAPRestore_SystemBlockAuthoritySurvivesDocJSON — findings
// #10/#57: the FRCAP best-draft restore round-trips the draft through
// DocJSON; the restored document and its render MUST keep runtime-trace
// system authority. Pre-fix RED on all three faces: marker gone,
// chapter demoted to ###, and the PSG ship-time re-scan minted a
// user-visible false disclosure against the system's own numerals.
func TestFRCAPRestore_SystemBlockAuthoritySurvivesDocJSON(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:app.main-42591:s_sleep", "38.996"))
	bus := psgBus(mut)
	o := &Orchestrator{busCtx: bus}

	// Round N: the marked draft is live in MutableState and gets recorded.
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, reauthTraceDoc())
	var ledger []finalizeRepairDraftRecord
	ledger = recordFinalizeRepairDraft(ledger, &agent.StageOutput{FinalAnswer: "round-0 draft"}, mut,
		[]types.Violation{{Kind: types.ViolBlockCoverageMissing}}, 0, 2)
	if len(ledger) != 1 || len(ledger[0].DocJSON) == 0 {
		t.Fatalf("record must capture the doc, got %+v", ledger)
	}
	if len(ledger[0].SystemBlockKinds) != 1 {
		t.Fatalf("record must capture the authority sidecar alongside DocJSON, got %v", ledger[0].SystemBlockKinds)
	}

	// A later (worse) round replaces the live doc, then the cap restores
	// the recorded best draft.
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "后轮劣化稿。"}}})
	out := &agent.StageOutput{FinalAnswer: "后轮劣化稿。"}
	if !o.restoreFinalizeRepairDraft(out, &ledger[0]) {
		t.Fatalf("restore must succeed")
	}

	restored := mut.AnswerDocumentV2()
	if restored == nil || len(restored.Blocks) != 2 {
		t.Fatalf("restored doc missing, got %+v", restored)
	}
	if !answertool.RuntimeTraceSystemBlock(restored.Blocks[1]) {
		t.Fatalf("restored system block lost its authority marker (DocJSON strip): %+v", restored.Blocks[1])
	}
	// Render face: authenticated runtime-trace chapters stay `##`.
	if !strings.Contains(out.FinalAnswer, "## 关键指标核对") {
		t.Fatalf("restored render must keep the ## chapter promotion, got:\n%s", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, "### 关键指标核对") {
		t.Fatalf("restored render demoted the system chapter to ###:\n%s", out.FinalAnswer)
	}
	// PSG ship-time face: the shipped doc grounds its own system
	// numerals — the residual caveat must render NOTHING (pre-fix it
	// listed 46.821ms as unlocatable, a user-visible false disclosure).
	if caveat := o.proseScalarResidualCaveatTextForShippedDoc(); caveat != "" {
		t.Fatalf("ship-time PSG re-scan must not discredit system numerals on the restored draft, got: %s", caveat)
	}
}

// TestRecoveryDraft_PrevEmitJSONReauthenticated_PSGClean — finding #69:
// the finalizer recovery draft decoded from RetryState.PrevEmitJSON must
// come back re-authenticated, and the PSG contract face over it must
// stay clean — pre-fix the marker loss inverted the evidence lane
// (system numerals scanned as model prose) and the resulting false
// ViolProseScalarUngrounded killed the beneficial recovery.
func TestRecoveryDraft_PrevEmitJSONReauthenticated_PSGClean(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:app.main-42591:s_sleep", "38.996"))
	bus := psgBus(mut)
	o := &Orchestrator{busCtx: bus}

	// The persisted (marked) doc is snapshotted by the REAL producer…
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, reauthTraceDoc())
	populateRetryState(mut, contract.Result{}, 0)
	// …then a fallback reset clears the live doc (the recovery lane's
	// documented precondition).
	mut.ResetAnswerDocumentV2()

	doc, source := o.finalizerRecoveryDraftCandidate()
	if doc == nil || source != "retry_state" {
		t.Fatalf("candidate must come from retry_state, got source=%q doc=%v", source, doc)
	}
	if !answertool.RuntimeTraceSystemBlock(doc.Blocks[1]) {
		t.Fatalf("recovery draft lost system authority on the PrevEmitJSON round trip: %+v", doc.Blocks[1])
	}
	// The false-violation shape that aborted beneficial recovery: the
	// system block's numeral must feed the evidence face again, so the
	// PSG check over the recovered draft raises NOTHING.
	if got := psgViolations(runProseScalarGroundingCheck(doc, bus, mut)); len(got) != 0 {
		t.Fatalf("recovered draft must not raise false ViolProseScalarUngrounded on system numerals, got %+v", got)
	}
	// Honesty control: a numeral NO evidence face publishes still raises
	// — re-authentication restores authority, it does not blanket-exempt
	// the recovered draft.
	fabricated := doc.Blocks[0]
	fabricated.Text = "凭空数值 99.123ms。"
	dishonest := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{fabricated, doc.Blocks[1]}}
	if got := psgViolations(runProseScalarGroundingCheck(dishonest, bus, mut)); len(got) != 1 {
		t.Fatalf("recovery re-auth must not exempt genuinely ungrounded prose, got %+v", got)
	}
}

// TestFinalizerTerminalCaveats_SurviveLastMileRerender — finding #11:
// the transient-failure and recovered-draft disclosures must ride the
// CAVSTR replay register: a later FinalAnswer re-render through the
// last-mile chokepoint (the exact overwrite class that killed string
// caveats pre-§29.17) keeps EXACTLY ONE instance of each disclosure.
// Pre-fix RED: raw concat → the re-render drops both texts entirely.
func TestFinalizerTerminalCaveats_SurviveLastMileRerender(t *testing.T) {
	const transientMark = "后续成文重试因为模型响应中断未能完成"
	const recoveredMark = "成文阶段在产出这版结构化草稿后遇到模型流式响应瞬时失败"

	mut := types.NewMutableState("q")
	bus := &types.BusContext{Ctx: context.Background(), Mutable: mut, Language: "zh"}
	o := &Orchestrator{busCtx: bus}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "正文。"}}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)

	// Direct path: both appenders add their disclosure exactly once.
	ans := o.appendFinalizerTransientFailureCaveat("正文。")
	ans = o.appendFinalizerRecoveredDraftCaveat(ans)
	if strings.Count(ans, transientMark) != 1 || strings.Count(ans, recoveredMark) != 1 {
		t.Fatalf("direct append must carry each disclosure once:\n%s", ans)
	}

	// Overwrite class: a later re-render from the structured document
	// (first-draft attachment / auto-repair / FRCAP swap shape) starts
	// from a doc-only surface — the register must replay both.
	for round := 1; round <= 2; round++ {
		rerendered := o.renderFinalAnswerWithLastMileSupplements(doc, nil)
		if got := strings.Count(rerendered, transientMark); got != 1 {
			t.Fatalf("re-render %d: transient-failure disclosure count = %d, want 1:\n%s", round, got, rerendered)
		}
		if got := strings.Count(rerendered, recoveredMark); got != 1 {
			t.Fatalf("re-render %d: recovered-draft disclosure count = %d, want 1:\n%s", round, got, rerendered)
		}
	}

	// Idempotence at the appender itself: re-appending registers nothing
	// new (duplicate key), so the answer never carries the text twice.
	again := o.appendFinalizerTransientFailureCaveat(ans)
	if strings.Count(again, transientMark) != 1 {
		t.Fatalf("duplicate append must not double the disclosure:\n%s", again)
	}
	// Blank-answer guards preserved from the legacy free functions.
	if o.appendFinalizerTransientFailureCaveat("  ") != "" {
		t.Fatalf("blank answer must stay blank (transient)")
	}
	if o.appendFinalizerRecoveredDraftCaveat("") != "" {
		t.Fatalf("blank answer must stay blank (recovered)")
	}
}

// TestSystemSnapshotReauthCallSitesWhitelisted — misuse defence
// (structural): ReauthenticateSystemSnapshotBlockKinds mints authority,
// so its caller set is pinned to the audited system-side snapshot lanes.
// Wiring it into ANY new file — above all a model-direct JSON decode
// (emit tool params, text-recovered drafts, prompt echoes) — turns this
// red until the lane's provenance is re-audited. The capture face is
// pinned too: sidecars may only be produced where system-side snapshots
// are taken.
func TestSystemSnapshotReauthCallSitesWhitelisted(t *testing.T) {
	wantReauth := map[string]bool{
		"internal/types/answer_document_v2.go":                  true, // definition
		"internal/orchestrator/finalize_repair_draft_ledger.go": true,
		"internal/orchestrator/finalizer_auto_repair.go":        true,
		"internal/tool/emit_answer_document_patch.go":           true,
		"internal/agent/answer_document_evaluator.go":           true,
	}
	wantCapture := map[string]bool{
		"internal/types/answer_document_v2.go":                  true, // definition
		"internal/orchestrator/retry_state.go":                  true,
		"internal/orchestrator/finalize_repair_draft_ledger.go": true,
	}
	gotReauth := map[string]bool{}
	gotCapture := map[string]bool{}
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(string(raw), "ReauthenticateSystemSnapshotBlockKinds(") {
			gotReauth[rel] = true
		}
		if strings.Contains(string(raw), "CaptureSystemGeneratedBlockKinds(") {
			gotCapture[rel] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	diff := func(name string, want, got map[string]bool) {
		var extra, missing []string
		for f := range got {
			if !want[f] {
				extra = append(extra, f)
			}
		}
		for f := range want {
			if !got[f] {
				missing = append(missing, f)
			}
		}
		sort.Strings(extra)
		sort.Strings(missing)
		if len(extra) > 0 {
			t.Errorf("%s: UNAUDITED call site(s) %v — re-stamping a model-direct JSON lane mints forged system authority; audit the snapshot provenance, then extend the whitelist", name, extra)
		}
		if len(missing) > 0 {
			t.Errorf("%s: audited lane(s) lost their wiring %v — the marker-stripping defect is back on that lane", name, missing)
		}
	}
	diff("ReauthenticateSystemSnapshotBlockKinds", wantReauth, gotReauth)
	diff("CaptureSystemGeneratedBlockKinds", wantCapture, gotCapture)
}
