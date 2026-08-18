package tool

// answer_document_projection_case1_dio_test.go — CASE-1 display half
// 通电验证 (§29.52 独立立案 CASE-1 → v5 P1 批, 2026-07-13): with the engine
// minting {d_state_or_io_wait, io_wait}² absorption markers (gaps a/b/c —
// see internal/tracequery/rank_family_recon_case1_test.go), the SMR-1-era
// display arm that was 已就位待命 (the generic G1 链上并入 relocation joined
// on verbatim RankFamilyKey == AbsorbedInto) energizes for the D/IO shape:
// the chain-lane critical rows hold NO tree/stanza seats, the ONE family
// seat carries the 链上并入 disclosure, and 观测照发不删 (evidence index /
// published observations / audit tokens lossless). blocked_reason 消费义务
// (CR-3 P10) rides the surviving single seat unchanged.
//
// FULL-CHAIN engine-minted fixture (§28.7 复核纪律②): trace text →
// tracequery.Run(frame_root_cause_bundle) → traceQueryTypedObservations →
// CompileTraceCausalProjectionSet → tree model → detail/evidence faces.

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

// case1DisplayPureDTrace mirrors the tracequery-side case1PureDTrace verbatim
// (intentional cross-package double-write of the witness shape): worker-200
// blocks in D twice on two cpus → two per-(thread,cpu) ledger groups →
// window_stats family ×2 (20ms + 30ms) + two chain-lane critical rows.
const case1DisplayPureDTrace = `
        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.030000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.031000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.040000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.070000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.071000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.072000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func TestCase1DisplayDIOAbsorptionEndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case1_display.systrace")
	if err := os.WriteFile(path, []byte(case1DisplayPureDTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "frame_root_cause_bundle", PID: 100, TimeStart: 3.0, TimeEnd: 3.2, MinDurationMs: 0.05})
	records := traceQueryTypedObservations(result, "case1_display.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	if len(records) == 0 {
		t.Fatal("engine fixture minted no observation records")
	}
	// 观测照发不删: the absorbed chain-lane observations keep publishing with
	// the typed marker.
	published := 0
	for _, record := range records {
		if record.ClaimKey == "critical_blocking:d_state_or_io_wait" {
			published++
			if !strings.Contains(strings.Join(record.RichNotes, "\n"), "absorbed_by_rank_family=true") {
				t.Fatalf("published absorbed observation must carry the typed marker: %+v", record.RichNotes)
			}
		}
	}
	if published != 2 {
		t.Fatalf("both chain-lane D/IO observations must keep publishing, got %d", published)
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	if len(projection.AbsorbedChainRows) != 2 {
		t.Fatalf("compile must relocate the 2 absorbed D/IO rows, got %d", len(projection.AbsorbedChainRows))
	}
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, evidence, true)
	var family *runtimeTraceProjTreeRow
	var rows []runtimeTraceProjTreeRow
	for _, group := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		rows = append(rows, group...)
	}
	for i := range rows {
		row := &rows[i]
		if row.Node.AbsorbedByRankFamily {
			t.Fatalf("absorbed D/IO peer row must hold no tree/stanza seat: %+v", row.Node)
		}
		if row.Node.RankFamilyKey != "" {
			if family != nil {
				t.Fatalf("expected exactly one rendered D/IO family row, found a second: %+v", row.Node)
			}
			family = row
		}
	}
	if family == nil {
		t.Fatal("the D/IO family row must render")
	}
	if len(family.AbsorbedChainPeers) != 2 {
		t.Fatalf("family row must carry the 2 absorbed peers, got %d", len(family.AbsorbedChainPeers))
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "链上并入") || !strings.Contains(detail, "链上通道 2 条同源观测已并入本行(") {
		t.Fatalf("family detail stanza must carry the 链上并入 note:\n%s", detail)
	}
	for _, peer := range family.AbsorbedChainPeers {
		tag := strings.TrimSpace(peer.EvidenceTag)
		if tag == "" {
			t.Fatal("absorbed peer must carry a registered evidence tag")
		}
		if !strings.Contains(detail, tag) {
			t.Fatalf("链上并入 note must cite %s:\n%s", tag, detail)
		}
	}
	// Reader face: the family-absorption fact stays visible without exposing
	// the internal absorbed_into key/family identity.
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
}
