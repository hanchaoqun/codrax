package tool

// answer_document_projection_f_round_test.go — F-round pins (adversarial
// review of the V1–V5 fixes, 2026-07-03). Each test reproduces the review
// probe's exact shape:
//   F1 — the target-symptom denominator sums ONLY the target's state-view rows
//        in the sleep/D/blocked wait family: a blocked-wait hop view nested
//        inside a state segment never double-counts (10ms sleep + 8ms binder
//        inside it = 10ms symptom, 80% attributed — never 18ms/44%), and a
//        running/runnable self row never enters the sleep/blocked denominator;
//   F3 — the whole-window idle annotation is gated on the typed wait-family
//        StateKind on BOTH surfaces (tree stanza tag + detail table share one
//        helper): a whole-window running CPU hog or a stateless aggregate row
//        never reads as "等待/空闲";
//   F4 — rank TIES in the conclusion-line selection break on the
//        single-instance value, never on bucket order (the primary bucket
//        sorts by cumulative = R2 SUM, which re-admitted the ×N SUM);
//   F5 — the evidence-index audit detail exposes duplicate_publications=N so
//        the third surface can tell the dedup fold from the R1/R2 merges.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- F1: symptom denominator = state-view wait rows only --------------------------

func fRoundSymptomModel(selfRows []runtimeTraceProjTreeRow) runtimeTraceProjTreeModel {
	return runtimeTraceProjTreeModel{
		Target:   "app_main-100",
		WindowMS: 100.0,
		SelfRows: selfRows,
		TreeRows: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "waker-1", CumulativeImpactMS: 8.0}},
		},
	}
}

func TestRuntimeTraceProjTargetSymptomExcludesNestedBlockingViewRow(t *testing.T) {
	// Review probe: a 10ms sleep state row plus the 8ms binder blocked-wait
	// view INSIDE it (critical_blocking hop view of the same wall clock). The
	// hop row deliberately carries a wait-family StateKind so ONLY the typed
	// Role exclusion can keep it out of the denominator.
	model := fRoundSymptomModel([]runtimeTraceProjTreeRow{
		{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
			Node: types.TraceCausalProjectionNode{Subject: "app_main-100", ImpactMS: 10.0, StateKind: "s_sleep"}},
		{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
			Node: types.TraceCausalProjectionNode{Subject: "app_main-100", ImpactMS: 8.0, StateKind: "s_sleep",
				Role: types.TraceCausalRoleCausalHop, Predicate: "critical_blocking", TypeToken: "binder_wait"}},
	})
	if got := runtimeTraceProjTargetSymptomMS(model); got != 10.0 {
		t.Fatalf("nested blocking view must not double-count the symptom: got %.3f, want 10.000", got)
	}
	projection := types.TraceCausalProjection{WindowStartTs: 100.000, WindowEndTs: 100.100}
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "目标睡眠/阻塞 10.000ms 中 on-chain 已归因 8.000ms(80%),未归因 2.000ms(20%)") {
		t.Fatalf("denominator must be the 10ms state segment (80%% attributed):\n%s", line)
	}
	if strings.Contains(line, "18.000") || strings.Contains(line, "44%") {
		t.Fatalf("the 18ms double-count / 44%% share must be gone:\n%s", line)
	}
	en := runtimeTraceProjWindowLine(projection, model, false)
	if !strings.Contains(en, "Of the target's 10.000ms sleep/blocked time, on-chain attributed 8.000ms (80%)") {
		t.Fatalf("EN surface must fork the same way:\n%s", en)
	}
}

func TestRuntimeTraceProjTargetSymptomExcludesRunningAndRunnableSelfRows(t *testing.T) {
	// A running (or runnable) self row is not sleep/blocked time — it must not
	// inflate the "目标睡眠/阻塞" denominator.
	model := fRoundSymptomModel([]runtimeTraceProjTreeRow{
		{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
			Node: types.TraceCausalProjectionNode{Subject: "app_main-100", ImpactMS: 90.0, StateKind: "running"}},
		{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
			Node: types.TraceCausalProjectionNode{Subject: "app_main-100", ImpactMS: 70.0, StateKind: "runnable"}},
		{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
			Node: types.TraceCausalProjectionNode{Subject: "app_main-100", ImpactMS: 10.0, StateKind: "s_sleep"}},
	})
	if got := runtimeTraceProjTargetSymptomMS(model); got != 10.0 {
		t.Fatalf("running/runnable self rows must not enter the denominator: got %.3f, want 10.000", got)
	}
	// Only non-wait self rows → no symptom → the pre-V2 whole-window fallback
	// wording (never a running-time denominator masquerading as sleep/blocked).
	onlyRunning := fRoundSymptomModel([]runtimeTraceProjTreeRow{
		{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
			Node: types.TraceCausalProjectionNode{Subject: "app_main-100", ImpactMS: 90.0, StateKind: "running"}},
	})
	if got := runtimeTraceProjTargetSymptomMS(onlyRunning); got != 0 {
		t.Fatalf("a running-only self view has no sleep/blocked symptom: got %.3f", got)
	}
	projection := types.TraceCausalProjection{WindowStartTs: 100.000, WindowEndTs: 100.100}
	line := runtimeTraceProjWindowLine(projection, onlyRunning, true)
	if strings.Contains(line, "目标睡眠/阻塞") || !strings.Contains(line, "未归因残差") {
		t.Fatalf("running-only self rows must fall back to the whole-window wording:\n%s", line)
	}
}

// --- F3: idle annotation requires the typed wait-family StateKind -----------------

func TestRuntimeTraceProjIdleAnnotationRequiresWaitFamilyStateKind(t *testing.T) {
	mkRow := func(state string) runtimeTraceProjTreeRow {
		return runtimeTraceProjTreeRow{
			Kind: runtimeTraceProjTreeRowBackground, HasData: true,
			Node: types.TraceCausalProjectionNode{
				Subject: "hog-9", Object: "unknown-thread",
				ImpactMS: 101.0, ChainRelevance: "background", StateKind: state,
			},
		}
	}
	// Review probe: a whole-window RUNNING CPU hog once rendered "运行占用" and
	// "整窗等待(疑似空闲)" on the same row — mutually exclusive claims.
	running := runtimeTraceProjStanzaRowLine(mkRow("running"), runtimeTraceProjTreeLabelWidth, 101.0, true, true)
	if strings.Contains(running, "疑似空闲") {
		t.Fatalf("a whole-window running row must not be annotated idle:\n%s", running)
	}
	if !strings.Contains(running, "运行占用") {
		t.Fatalf("the running row keeps its state tag:\n%s", running)
	}
	if runnable := runtimeTraceProjStanzaRowLine(mkRow("runnable"), runtimeTraceProjTreeLabelWidth, 101.0, true, true); strings.Contains(runnable, "疑似空闲") {
		t.Fatalf("a whole-window runnable row must not be annotated idle:\n%s", runnable)
	}
	// Stateless rows (e.g. a cpu·ms aggregate that happens to ≈ the window)
	// never fabricate an idle claim.
	if stateless := runtimeTraceProjStanzaRowLine(mkRow(""), runtimeTraceProjTreeLabelWidth, 101.0, true, true); strings.Contains(stateless, "疑似空闲") {
		t.Fatalf("a stateless whole-window row must not be annotated idle:\n%s", stateless)
	}
	// Sleep whole-window rows keep the V3 annotation.
	if sleep := runtimeTraceProjStanzaRowLine(mkRow("s_sleep"), runtimeTraceProjTreeLabelWidth, 101.0, true, true); !strings.Contains(sleep, "整窗等待(疑似空闲)") {
		t.Fatalf("a whole-window sleep row must keep the idle annotation:\n%s", sleep)
	}
	// The detail table consumes the SAME shared judgment — both surfaces fork
	// identically (the drift between the two hand-synced copies was the bug).
	model := runtimeTraceProjTreeModel{
		WindowMS:   101.0,
		Background: []runtimeTraceProjTreeRow{mkRow("running"), mkRow("s_sleep")},
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	if len(rows) != 2 {
		t.Fatalf("expected both rows on the detail table: %+v", rows)
	}
	if strings.Contains(rows[0].Cells[5], "疑似空闲") {
		t.Fatalf("detail table running row must not be annotated idle: %q", rows[0].Cells[5])
	}
	if !strings.Contains(rows[1].Cells[5], "整窗等待(疑似空闲)") {
		t.Fatalf("detail table sleep row must keep the idle annotation: %q", rows[1].Cells[5])
	}
	en := runtimeTraceProjStanzaRowLine(mkRow("running"), runtimeTraceProjTreeLabelWidth, 101.0, true, false)
	if strings.Contains(en, "likely idle") {
		t.Fatalf("EN running row must not be annotated idle:\n%s", en)
	}
}

// --- F4: rank ties break on the single-instance value -----------------------------

func TestRuntimeTraceProjLeadRankTieNeverPicksMergedSumByBucketOrder(t *testing.T) {
	// Review probe: BOTH primaries carry rank=1. The ×7 merged row (SUM 13.324)
	// sorts FIRST in the bucket (cumulative-major), so the old first-wins tie
	// path selected it — the single instance of 4.115 must win (its
	// single-instance value 4.115 > the merged per-instance max 1.940).
	projection := custom1gProjection(1, 1)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "RSUniRenderThre-1963") || !strings.Contains(line, "4.115ms") {
		t.Fatalf("rank tie must break on the single-instance value:\n%s", line)
	}
	for _, banned := range []string{"hmfs_discard", "13.324"} {
		if strings.Contains(line, banned) {
			t.Fatalf("the merged SUM must not win a rank tie (%q):\n%s", banned, line)
		}
	}
	en := runtimeTraceProjConclusionLine(projection, buildRuntimeTraceProjTreeModel(projection, nil, false), false)
	if !strings.Contains(en, "RSUniRenderThre-1963") || strings.Contains(en, "13.324") {
		t.Fatalf("EN conclusion must fork the same way:\n%s", en)
	}
}

func TestRuntimeTraceProjLeadRankTieFullTieKeepsBucketOrderDeterministic(t *testing.T) {
	// Same rank AND same single-instance value → the earlier bucket row wins
	// (deterministic; no re-ordering beyond the typed tie-break).
	projection := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{
			{EvidenceID: "E1", Subject: "aaa-1", Object: "running", Rank: 1,
				ImpactMS: 4.115, CumulativeImpactMS: 4.115, EffectiveImpactMS: 4.115, StateKind: "running"},
			{EvidenceID: "E2", Subject: "bbb-2", Object: "running", Rank: 1,
				ImpactMS: 4.115, CumulativeImpactMS: 4.115, EffectiveImpactMS: 4.115, StateKind: "running"},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "aaa-1") || strings.Contains(line, "bbb-2") {
		t.Fatalf("a full tie keeps the earlier bucket row:\n%s", line)
	}
}

// --- F5: audit detail exposes the duplicate-publication count ----------------------

func TestRuntimeTraceProjAuditDetailCarriesDuplicatePublications(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Predicate: "root_cause_context", DuplicatePublications: 3,
		MergedEvidenceIDs: []string{"E13b", "E13c"},
	}
	detail := runtimeTraceCausalProjectionAuditDetail(node, true, false)
	if !strings.Contains(detail, "duplicate_publications=3") {
		t.Fatalf("audit detail must expose the dedup fold count: %q", detail)
	}
	if !strings.Contains(detail, "merged_ids=E13b,E13c") {
		t.Fatalf("merged ids stay on the audit line: %q", detail)
	}
	// The R2 SUM aggregate keeps merged_count= and never claims the dedup field.
	sum := types.TraceCausalProjectionNode{Predicate: "root_cause_context", MergedCount: 3}
	sumDetail := runtimeTraceCausalProjectionAuditDetail(sum, true, false)
	if !strings.Contains(sumDetail, "merged_count=3") || strings.Contains(sumDetail, "duplicate_publications") {
		t.Fatalf("sum aggregate audit detail must not claim the dedup field: %q", sumDetail)
	}
}
