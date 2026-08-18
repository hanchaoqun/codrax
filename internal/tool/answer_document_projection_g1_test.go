package tool

// answer_document_projection_g1_test.go — G1 跨车道对账 display pins
// (§27.2-G1, 2026-07-09, real_trace_campaign_20260705.md), FULL-CHAIN engine
// -minted fixtures (§28.7 复核纪律②: fixture 应取引擎实铸形): trace text →
// tracequery.Run(frame_root_cause_bundle) → traceQueryTypedObservations →
// CompileTraceCausalProjectionSet → tree model → fence / detail / evidence
// faces.
//
// End-to-end shape (opendir_79 form): the ×6 io_latency family (raw sum
// 2.858ms, four udk-irq peers) renders as ONE family row; the six absorbed
// critical_blocking peer rows hold NO tree/stanza seats; the family detail
// stanza carries the 链上并入 note with the absorbed rows' E# list; every
// absorbed E# stays registered on the evidence index with its absorbed_into
// audit token (信息守恒三面: E# 索引 / 原始观测照发 / 审计 token).
//
// MUTATION self-checks:
//   - dropping the model's absorbed attach/registration pass reds
//     TestG1DisplayEndToEndOpendirShape (链上并入 + E# assertions);
//   - dropping the stanza add() reds the 链上并入 assertion;
//   - suppressing rows WITHOUT the family-present join (noisy relaxation)
//     reds TestG1DisplayNoFamilyKeepsPeerRowSeats.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// g1DisplayOpendirTrace mirrors the tracequery-side g1OpendirShapeTrace
// verbatim (intentional cross-package double-write of the witness shape).
const g1DisplayOpendirTrace = `
        work-500   (  500) [001] .... 10.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=work next_pid=500 next_prio=100
        work-500   (  500) [001] .... 10.100000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [work]
      udk-irq-0-71   (    2) [000] .... 10.100500: block_rq_complete: 8,0 R () 1000 + 8 [0]
        work-500   (  500) [001] .... 10.110000: block_rq_issue: 8,0 R 4096 () 1008 + 8 [work]
      udk-irq-1-72   (    2) [000] .... 10.110500: block_rq_complete: 8,0 R () 1008 + 8 [0]
        work-500   (  500) [001] .... 10.120000: block_rq_issue: 8,0 R 4096 () 1016 + 8 [work]
      udk-irq-2-73   (    2) [000] .... 10.120500: block_rq_complete: 8,0 R () 1016 + 8 [0]
        work-500   (  500) [001] .... 10.130000: block_rq_issue: 8,0 R 4096 () 1024 + 8 [work]
      udk-irq-3-74   (    2) [000] .... 10.130500: block_rq_complete: 8,0 R () 1024 + 8 [0]
        work-500   (  500) [001] .... 10.140000: block_rq_issue: 8,0 R 4096 () 1032 + 8 [work]
      udk-irq-0-71   (    2) [000] .... 10.140500: block_rq_complete: 8,0 R () 1032 + 8 [0]
        work-500   (  500) [001] .... 10.150000: block_rq_issue: 8,0 R 4096 () 1040 + 8 [work]
      udk-irq-1-72   (    2) [000] .... 10.150358: block_rq_complete: 8,0 R () 1040 + 8 [0]
        work-500   (  500) [001] .... 10.900000: sched_switch: prev_comm=work prev_pid=500 prev_prio=100 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func g1DisplayCompileOpendir(t *testing.T) (types.TraceCausalProjection, []types.ObservationRecord) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "g1_display.systrace")
	if err := os.WriteFile(path, []byte(g1DisplayOpendirTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "frame_root_cause_bundle", PID: 500, TimeStart: 10.0, TimeEnd: 11.0})
	records := traceQueryTypedObservations(result, "g1_display.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	if len(records) == 0 {
		t.Fatal("engine fixture minted no observation records")
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	return set.Projections[0], records
}

func g1DisplayModelRows(model runtimeTraceProjTreeModel) []runtimeTraceProjTreeRow {
	var rows []runtimeTraceProjTreeRow
	for _, group := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		rows = append(rows, group...)
	}
	return rows
}

func TestG1DisplayEndToEndOpendirShape(t *testing.T) {
	projection, records := g1DisplayCompileOpendir(t)
	// 观测照发不删: all six critical_blocking observations still published.
	published := 0
	for _, record := range records {
		if record.ClaimKey == "critical_blocking:io_latency" {
			published++
			if !strings.Contains(strings.Join(record.RichNotes, "\n"), "absorbed_by_rank_family=true") {
				t.Fatalf("published absorbed observation must carry the typed marker: %+v", record.RichNotes)
			}
		}
	}
	if published != 6 {
		t.Fatalf("all 6 critical_blocking observations must keep publishing, got %d", published)
	}
	if len(projection.AbsorbedChainRows) != 6 {
		t.Fatalf("compile must relocate the 6 absorbed rows, got %d", len(projection.AbsorbedChainRows))
	}
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, evidence, true)
	var family *runtimeTraceProjTreeRow
	rows := g1DisplayModelRows(model)
	for i := range rows {
		row := &rows[i]
		if row.Node.AbsorbedByRankFamily {
			t.Fatalf("absorbed peer row must hold no tree/stanza seat: %+v", row.Node)
		}
		if row.Node.RankFamilyKey != "" {
			if family != nil {
				t.Fatalf("expected exactly one rendered family row, found a second: %+v", row.Node)
			}
			family = row
		}
	}
	if family == nil {
		t.Fatal("the ×6 family row must render")
	}
	if family.Node.FamilyMemberCount != 6 {
		t.Fatalf("family row must carry member_count=6, got %d", family.Node.FamilyMemberCount)
	}
	if len(family.AbsorbedChainPeers) != 6 {
		t.Fatalf("family row must carry the 6 absorbed peers, got %d", len(family.AbsorbedChainPeers))
	}
	// The detail stanza's 链上并入 disclosure with the E# roster.
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "链上并入") || !strings.Contains(detail, "链上通道 6 条同源观测已并入本行(") {
		t.Fatalf("family detail stanza must carry the 链上并入 note:\n%s", detail)
	}
	// Every absorbed peer's E# is registered and cited in the note.
	for _, peer := range family.AbsorbedChainPeers {
		tag := strings.TrimSpace(peer.EvidenceTag)
		if tag == "" {
			t.Fatal("absorbed peer must carry a registered evidence tag")
		}
		if !strings.Contains(detail, tag) {
			t.Fatalf("链上并入 note must cite %s:\n%s", tag, detail)
		}
	}
	// The evidence index keeps the absorption meaning, while the exact family
	// pointer remains in diagnostics rather than the customer panel.
	evidenceIntro, evidenceItems := runtimeTraceProjEvidenceBlockParts(evidence, true)
	var evidenceText strings.Builder
	evidenceText.WriteString(evidenceIntro)
	for _, item := range evidenceItems {
		evidenceText.WriteString("\n" + item.Label + " " + item.Text)
	}
	if !strings.Contains(evidenceText.String(), "已并入同一根因族，避免重复计数") ||
		strings.Contains(evidenceText.String(), "absorbed_into=") {
		t.Fatalf("evidence index must carry reader-facing absorption without the raw key:\n%s", evidenceText.String())
	}
	// The fence renders the family row once; no absorbed peer row beside it.
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "6次") {
		t.Fatalf("fence must render the 6次 family row:\n%s", fence)
	}
	if strings.Count(fence, "udk-irq-") > 0 {
		t.Fatalf("fence must not seat absorbed peer rows:\n%s", fence)
	}
	// EN face parity for the stanza note.
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	detailEN := runtimeTraceProjDetailFullText(modelEN, false)
	if !strings.Contains(detailEN, "on-chain-channel same-source observation(s) absorbed into this row") {
		t.Fatalf("EN family stanza must carry the absorbed note:\n%s", detailEN)
	}
}

// g1DisplayHuadongTrace mirrors the tracequery-side g1HuadongShapeTrace
// verbatim (×8 io_latency family, raw sum 15.156ms, five udk-irq peers).
const g1DisplayHuadongTrace = `
        hmfs-600   (  600) [002] .... 20.010000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hmfs next_pid=600 next_prio=100
        hmfs-600   (  600) [002] .... 20.100000: block_rq_issue: 12,48 RS 4096 () 2000 + 8 [hmfs]
        hmfs-600   (  600) [002] .... 20.100100: sched_switch: prev_comm=hmfs prev_pid=600 prev_prio=100 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      udk-irq-0-81   (    2) [000] .... 20.102000: block_rq_complete: 12,48 RS () 2000 + 8 [0]
      udk-irq-0-81   (    2) [000] .... 20.102015: sched_wakeup: comm=hmfs pid=600 prio=100 target_cpu=002
        hmfs-600   (  600) [002] .... 20.102020: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hmfs next_pid=600 next_prio=100
        hmfs-600   (  600) [002] .... 20.110000: block_rq_issue: 12,48 RS 4096 () 2008 + 8 [hmfs]
        hmfs-600   (  600) [002] .... 20.110100: sched_switch: prev_comm=hmfs prev_pid=600 prev_prio=100 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      udk-irq-1-82   (    2) [000] .... 20.112000: block_rq_complete: 12,48 RS () 2008 + 8 [0]
      udk-irq-1-82   (    2) [000] .... 20.112015: sched_wakeup: comm=hmfs pid=600 prio=100 target_cpu=002
        hmfs-600   (  600) [002] .... 20.112020: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hmfs next_pid=600 next_prio=100
        hmfs-600   (  600) [002] .... 20.120000: block_rq_issue: 12,48 RS 4096 () 2016 + 8 [hmfs]
        hmfs-600   (  600) [002] .... 20.120100: sched_switch: prev_comm=hmfs prev_pid=600 prev_prio=100 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      udk-irq-2-83   (    2) [000] .... 20.122000: block_rq_complete: 12,48 RS () 2016 + 8 [0]
      udk-irq-2-83   (    2) [000] .... 20.122015: sched_wakeup: comm=hmfs pid=600 prio=100 target_cpu=002
        hmfs-600   (  600) [002] .... 20.122020: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hmfs next_pid=600 next_prio=100
        hmfs-600   (  600) [002] .... 20.130000: block_rq_issue: 12,48 RS 4096 () 2024 + 8 [hmfs]
        hmfs-600   (  600) [002] .... 20.130100: sched_switch: prev_comm=hmfs prev_pid=600 prev_prio=100 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      udk-irq-3-84   (    2) [000] .... 20.132000: block_rq_complete: 12,48 RS () 2024 + 8 [0]
      udk-irq-3-84   (    2) [000] .... 20.132015: sched_wakeup: comm=hmfs pid=600 prio=100 target_cpu=002
        hmfs-600   (  600) [002] .... 20.132020: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hmfs next_pid=600 next_prio=100
        hmfs-600   (  600) [002] .... 20.140000: block_rq_issue: 12,48 RS 4096 () 2032 + 8 [hmfs]
        hmfs-600   (  600) [002] .... 20.140100: sched_switch: prev_comm=hmfs prev_pid=600 prev_prio=100 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      udk-irq-4-85   (    2) [000] .... 20.142000: block_rq_complete: 12,48 RS () 2032 + 8 [0]
      udk-irq-4-85   (    2) [000] .... 20.142015: sched_wakeup: comm=hmfs pid=600 prio=100 target_cpu=002
        hmfs-600   (  600) [002] .... 20.142020: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hmfs next_pid=600 next_prio=100
        hmfs-600   (  600) [002] .... 20.150000: block_rq_issue: 12,48 RS 4096 () 2040 + 8 [hmfs]
        hmfs-600   (  600) [002] .... 20.150100: sched_switch: prev_comm=hmfs prev_pid=600 prev_prio=100 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      udk-irq-0-81   (    2) [000] .... 20.152000: block_rq_complete: 12,48 RS () 2040 + 8 [0]
      udk-irq-0-81   (    2) [000] .... 20.152015: sched_wakeup: comm=hmfs pid=600 prio=100 target_cpu=002
        hmfs-600   (  600) [002] .... 20.152020: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hmfs next_pid=600 next_prio=100
        hmfs-600   (  600) [002] .... 20.160000: block_rq_issue: 12,48 RS 4096 () 2048 + 8 [hmfs]
        hmfs-600   (  600) [002] .... 20.160100: sched_switch: prev_comm=hmfs prev_pid=600 prev_prio=100 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      udk-irq-1-82   (    2) [000] .... 20.162000: block_rq_complete: 12,48 RS () 2048 + 8 [0]
      udk-irq-1-82   (    2) [000] .... 20.162015: sched_wakeup: comm=hmfs pid=600 prio=100 target_cpu=002
        hmfs-600   (  600) [002] .... 20.162020: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hmfs next_pid=600 next_prio=100
        hmfs-600   (  600) [002] .... 20.170000: block_rq_issue: 12,48 RS 4096 () 2056 + 8 [hmfs]
        hmfs-600   (  600) [002] .... 20.170100: sched_switch: prev_comm=hmfs prev_pid=600 prev_prio=100 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      udk-irq-2-83   (    2) [000] .... 20.171156: block_rq_complete: 12,48 RS () 2056 + 8 [0]
      udk-irq-2-83   (    2) [000] .... 20.171171: sched_wakeup: comm=hmfs pid=600 prio=100 target_cpu=002
        hmfs-600   (  600) [002] .... 20.171176: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hmfs next_pid=600 next_prio=100
        hmfs-600   (  600) [002] .... 20.900000: sched_switch: prev_comm=hmfs prev_pid=600 prev_prio=100 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
`

func TestG1DisplayEndToEndHuadongShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "g1_huadong.systrace")
	if err := os.WriteFile(path, []byte(g1DisplayHuadongTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "frame_root_cause_bundle", PID: 600, TimeStart: 20.0, TimeEnd: 21.0})
	records := traceQueryTypedObservations(result, "g1_huadong.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	// EVOLUTION RECORD (SELF-ALL §29.61.2, 2026-07-13; BIO-WAKE-1 fixture
	// strengthening 2026-08-16): pre-SELF-ALL the
	// target's own ×8 io_latency family sat on the ◇ adjacent channel, where
	// ONE critical publication same-fact-folded into the family node as its
	// native carrier (7 relocated + 1 folded). The family now takes the
	// on-chain channel on the typed self wall-clock basis (the target's own IO
	// seat may be crowned the ranked root cause), so all EIGHT chain
	// publications relocate into the absorbed audit lane beside the seated
	// family row — 观测照发不删, the disclosure note counts all eight. Every
	// member now carries an actual issue→blocking-switch→completion→issuer-
	// wake closure; target identity/request overlap alone is no longer enough.
	if len(projection.AbsorbedChainRows) != 8 {
		t.Fatalf("huadong shape must relocate all 8 chain publications beside the on-chain family row, got %d", len(projection.AbsorbedChainRows))
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var family *runtimeTraceProjTreeRow
	for i, rows := 0, g1DisplayModelRows(model); i < len(rows); i++ {
		if rows[i].Node.AbsorbedByRankFamily {
			t.Fatalf("huadong absorbed peer row must hold no seat: %+v", rows[i].Node)
		}
		if rows[i].Node.RankFamilyKey != "" {
			family = &rows[i]
		}
	}
	if family == nil || family.Node.FamilyMemberCount != 8 || len(family.AbsorbedChainPeers) != 8 {
		t.Fatalf("huadong family row must render ×8 with 8 absorbed peers: %+v", family)
	}
	// BIO-WAKE-1 lane pin: the family's on-chain identity is the typed
	// completion→issuer wake closure, not a fabricated target-self interval
	// basis. The old SELF-ALL fallback is intentionally absent when the
	// stronger directed credential exists.
	if family.Node.ChainRelevance != "on_chain" || !family.Node.ResourceCompletionClosure || family.Node.OnChainBasis != "" {
		t.Fatalf("huadong family must ride the on-chain channel on the strict completion-wake basis: %+v", family.Node)
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "链上通道 8 条同源观测已并入本行(") {
		t.Fatalf("huadong family stanza must disclose the 8 relocated peers:\n%s", detail)
	}
	// The real sched_wakeup closures add legitimate IRQ wakeup-chain nodes.
	// Do not confuse those first-class causal nodes with the absorbed duplicate
	// critical publications guarded by the row-level checks above.
	if !strings.Contains(runtimeTraceProjTreeFence(model, true), "udk-irq-") {
		t.Fatal("huadong fence must retain the completion IRQ wakeup-chain nodes")
	}
}

// TestG1DisplayNoFamilyKeepsPeerRowSeats is the display-level 负向保护 pin: a
// projection whose absorbed markers matched no family (compile left the rows
// in their buckets) renders the peer rows exactly as before — suppression is
// join-conditioned, never marker-alone.
func TestG1DisplayNoFamilyKeepsPeerRowSeats(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "E4",
		Subject: "work-500", Predicate: "critical_blocking", Object: "udk-irq-0-71",
		AbsorbedByRankFamily: true, AbsorbedInto: "io_latency|pid:500|on_chain|10.000000..11.000000",
		ChainRelevance: "on_chain", ImpactMS: 0.5, Confidence: 0.86,
		LineStart: 3, LineEnd: 3,
	}
	projection := types.TraceCausalProjection{
		SupportingHops: []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	seated := false
	for _, row := range g1DisplayModelRows(model) {
		if row.Node.EvidenceID == "E4" {
			seated = true
		}
	}
	if !seated {
		t.Fatal("without a rendered family row the absorbed-marked peer row must keep its seat")
	}
}

// TestG1BannerFaceCarriesAbsorptionMarker pins the result text face: an
// absorbed blocking row tells the model inline that its events already count
// inside the named family (soft guidance twin of the typed notes).
func TestG1BannerFaceCarriesAbsorptionMarker(t *testing.T) {
	result := tracequery.Result{
		View: "critical_blocking_calls",
		CriticalBlocking: &tracequery.CriticalBlockingResult{
			Items: []tracequery.CriticalBlockingCandidate{{
				Type: "io_latency", Thread: tracequery.ThreadRef{Comm: "work", PID: 500},
				Peer:                 tracequery.ThreadRef{Comm: "udk-irq-0", PID: 71},
				AbsorbedByRankFamily: true,
				AbsorbedIntoFamily:   "io_latency|pid:500|on_chain|10.000000..11.000000",
				DurationMs:           0.5, LineStart: 3, LineEnd: 3, Confidence: 0.86,
				Summary: "block IO 8,0 R sector=1000 len=8 took 0.500ms",
			}},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "critical_blocking_calls"}, "path", "/tmp/payload.json")
	if !strings.Contains(summary, "absorbed_by_rank_family=true absorbed_into=io_latency|pid:500|on_chain|10.000000..11.000000") {
		t.Fatalf("banner face must carry the absorption marker:\n%s", summary)
	}
}

// TestG1DisplayAbsorbedNoteRosterOverflow pins the counted-overflow roster
// discipline (§24.7.1 ①): more than 8 absorbed peers fold the E# roster with
// an explicit total, never a silent cut.
func TestG1DisplayAbsorbedNoteRosterOverflow(t *testing.T) {
	peers := make([]runtimeTraceProjAbsorbedChainPeer, 0, 10)
	for i := 1; i <= 10; i++ {
		peers = append(peers, runtimeTraceProjAbsorbedChainPeer{EvidenceTag: "E" + string(rune('0'+i%10)) + "0"})
	}
	note := runtimeTraceProjAbsorbedChainNote(peers, true)
	if !strings.HasPrefix(note, "链上通道 10 条同源观测已并入本行(") {
		t.Fatalf("note must carry the full count, got %q", note)
	}
	if !strings.Contains(note, "等共10条") {
		t.Fatalf("overflow must disclose the counted total, got %q", note)
	}
}

// --- 收尾 P1/P2 pins (对抗复核 SHIP-WITH-FIXES, 2026-07-09) -------------------

// g1DisplayBackgroundFamilyTrace is the P1 REPRO shape: the target app-700
// has a REAL wakeup chain (waker-600 wakes it), and a THIRD thread io-800
// issues two OFF-CHAIN block IOs (completers not on the chain either) — the
// io_latency family row then classifies background and seats in the ▒ stanza,
// NOT the tree. Pre-P1 the absorbed attach ran BEFORE the stanza loops were
// populated, so this family row could never receive its 链上并入 note (dead
// code on exactly the stanza lanes).
const g1DisplayBackgroundFamilyTrace = `
       waker-600   (  600) [000] .... 30.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=600 next_prio=100
         app-700   (  700) [001] .... 30.005000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=700 next_prio=100
         app-700   (  700) [001] .... 30.010000: sched_switch: prev_comm=app prev_pid=700 prev_prio=100 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       waker-600   (  600) [000] .... 30.200000: sched_wakeup: comm=app pid=700 prio=100 target_cpu=001
         app-700   (  700) [001] .... 30.210000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=700 next_prio=100
          io-800   (  800) [002] .... 30.300000: block_rq_issue: 8,0 R 4096 () 3000 + 8 [io]
      udk-irq-0-90   (    2) [000] .... 30.301000: block_rq_complete: 8,0 R () 3000 + 8 [0]
          io-800   (  800) [002] .... 30.400000: block_rq_issue: 8,0 R 4096 () 3008 + 8 [io]
      udk-irq-1-91   (    2) [000] .... 30.401500: block_rq_complete: 8,0 R () 3008 + 8 [0]
         app-700   (  700) [001] .... 30.900000: sched_switch: prev_comm=app prev_pid=700 prev_prio=100 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func TestG1DisplayBackgroundFamilyStanzaReachableP1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "g1_bgfam.systrace")
	if err := os.WriteFile(path, []byte(g1DisplayBackgroundFamilyTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "frame_root_cause_bundle", PID: 700, TimeStart: 30.0, TimeEnd: 31.0})
	records := traceQueryTypedObservations(result, "g1_bgfam.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	if len(projection.AbsorbedChainRows) != 2 {
		t.Fatalf("REPRO shape must relocate the 2 absorbed off-chain rows, got %d", len(projection.AbsorbedChainRows))
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var family *runtimeTraceProjTreeRow
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for i := range rows {
			if rows[i].Node.AbsorbedByRankFamily {
				t.Fatalf("absorbed off-chain row must hold no ▒/◇ seat: %+v", rows[i].Node)
			}
			if rows[i].Node.RankFamilyKey != "" {
				family = &rows[i]
			}
		}
	}
	if family == nil {
		t.Fatal("the off-chain io family row must render")
	}
	// The REPRO essence: the family row seats in a STANZA lane (the P1 dead
	// zone), not the chain tree.
	if family.Kind != runtimeTraceProjTreeRowBackground && family.Kind != runtimeTraceProjTreeRowAdjacent {
		t.Fatalf("REPRO family row must seat in a stanza lane, got kind %q", family.Kind)
	}
	if len(family.AbsorbedChainPeers) != 2 {
		t.Fatalf("stanza family row must carry BOTH absorbed peers (P1 attach order), got %d", len(family.AbsorbedChainPeers))
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "链上通道 2 条同源观测已并入本行(") {
		t.Fatalf("stanza family row's 链上并入 note must be reachable (P1):\n%s", detail)
	}
}

// TestG1DisplayLaneSplitFamiliesOwnPeersP2a: two family rows with DISTINCT
// lane-carrying keys each receive exactly their own absorbed peers — the
// pre-P2a shared key let the first-claimed row swallow both families' peers.
func TestG1DisplayLaneSplitFamiliesOwnPeersP2a(t *testing.T) {
	keyOn := "io_latency|pid:800|on_chain|30.000000..31.000000"
	keyBg := "io_latency|pid:800|background|30.000000..31.000000"
	famOn := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E1",
		Subject: "io-800", Predicate: "root_cause_secondary", Object: "io_latency",
		ChainRelevance: "on_chain", ChainDepth: 1, Rank: 2,
		RankFamilyKey: keyOn, FamilyMemberCount: 2, FamilyFoldCaliber: "sum_disjoint",
		ImpactMS: 2.0, CumulativeImpactMS: 2.0, Confidence: 0.86, LineStart: 5, LineEnd: 9,
	}
	famBg := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E2",
		Subject: "io-800", Predicate: "root_cause_tertiary", Object: "io_latency",
		ChainRelevance: "background", Rank: 5,
		RankFamilyKey: keyBg, FamilyMemberCount: 2, FamilyFoldCaliber: "sum_disjoint",
		ImpactMS: 1.0, CumulativeImpactMS: 1.0, Confidence: 0.86, LineStart: 15, LineEnd: 19,
	}
	projection := types.TraceCausalProjection{
		OnChainCauses:    []types.TraceCausalProjectionNode{famOn},
		BackgroundCauses: []types.TraceCausalProjectionNode{famBg},
		AbsorbedChainRows: []types.TraceCausalProjectionNode{
			{EvidenceID: "E3", Subject: "io-800", Predicate: "critical_blocking", Object: "udk-irq-0", AbsorbedByRankFamily: true, AbsorbedInto: keyOn, LineStart: 6, LineEnd: 6},
			{EvidenceID: "E4", Subject: "io-800", Predicate: "critical_blocking", Object: "udk-irq-1", AbsorbedByRankFamily: true, AbsorbedInto: keyBg, LineStart: 16, LineEnd: 16},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	counts := map[string]int{}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for i := range rows {
			if key := rows[i].Node.RankFamilyKey; key != "" {
				counts[key] = len(rows[i].AbsorbedChainPeers)
			}
		}
	}
	if counts[keyOn] != 1 || counts[keyBg] != 1 {
		t.Fatalf("each family row must carry exactly ITS OWN peer (P2-a partition), got %v", counts)
	}
}

// TestG1DisplayMergedCarrierStanzaNoteP2b: when the family contender merged
// into an R2 ×N carrier (family grammar cleared, RankFamilyKey preserved),
// the 链上并入 note still renders — it keys on the attached peers, never on
// the family-row grammar arm.
func TestG1DisplayMergedCarrierStanzaNoteP2b(t *testing.T) {
	key := "io_latency|pid:800|background|30.000000..31.000000"
	carrier := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E5",
		Subject: "io-800", Predicate: "root_cause_secondary", Object: "io_latency",
		ChainRelevance: "background",
		RankFamilyKey:  key, // FamilyMemberCount == 0: the F-1 post-merge shape
		MergedCount:    3, MergedMinMS: 1.0, MergedMaxMS: 2.5,
		ImpactMS: 5.0, CumulativeImpactMS: 5.0, Confidence: 0.8, LineStart: 10, LineEnd: 30,
	}
	projection := types.TraceCausalProjection{
		BackgroundCauses: []types.TraceCausalProjectionNode{carrier},
		AbsorbedChainRows: []types.TraceCausalProjectionNode{
			{EvidenceID: "E6", Subject: "io-800", Predicate: "critical_blocking", Object: "udk-irq-0", AbsorbedByRankFamily: true, AbsorbedInto: key, LineStart: 11, LineEnd: 11},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "链上通道 1 条同源观测已并入本行(") {
		t.Fatalf("×N carrier row must still render the 链上并入 note (P2-b):\n%s", detail)
	}
	// Chimera control: no family grammar may render off the carrier.
	if strings.Contains(detail, "家族合并") {
		t.Fatalf("×N carrier must not render family grammar (F-1):\n%s", detail)
	}
}
