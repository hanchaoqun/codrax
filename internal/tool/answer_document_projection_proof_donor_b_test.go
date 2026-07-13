package tool

// answer_document_projection_proof_donor_b_test.go — 修复轮二 件B display
// pins (v5 P1 批, 2026-07-13): the refined 「D-state」 word's proof donor is
// dispatch-independent — the window_stats face records (top_d_state) carry
// the per-GROUP typed proof themselves, so the ×N raw-state fold row wears
// the refined word and the 等待对象 symbol with OR without a rank-family
// view in the ledger.
//
// EVOLUTION RECORD (h2 banned-word HEAD parity 实录, 2026-07-13): the golden
// h2 oracle's banned form 「自身·D-state/iowait」 was reachable on clean HEAD
// whenever the model's dispatch carried no rank/bundle-family view (the only
// proof donor then) — deterministic probe: the r3 dispatch set
// (window_stats+wakeup_chain+critical_blocking_calls) rendered the banned
// merged word BYTE-IDENTICALLY on HEAD and on the v5 P1 tree, and adding
// root_cause_rank refined BOTH. 件B sinks the donor to the stats face; these
// pins hold the word REACHABLE per dispatch shape, and the negative pin
// keeps the merged word honest when NO donor exists anywhere.
//
// MUTATION self-checks (修复轮三 F1 勘正: the no-marker negative has ALL
// members unproven, where OR≡AND — it can NOT catch the OR flip; the mixed
// pin below is the discriminating witness):
//   - dropping the critical-face proof emission reds
//     TestProofDonorStatsOnlyDispatchRefined;
//   - flipping the R2 ×N merge to OR (over-claim) reds
//     TestProofDonorMixedMembersVetoBothLanes (2真1假 members would mint the
//     refined word);
//   - removing the caller unanimity veto is caught at the merge authority
//     (types.TestMergeSameKindProofFamilyANDAndUnanimity — the tool
//     fixture's marker-seed election can mask that arm; 实测 2026-07-13).

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

func proofDonorFence(t *testing.T, tracePath string, thread string, start, end float64, views []string) string {
	t.Helper()
	idx, err := tracequery.BuildIndex(context.Background(), tracePath)
	if err != nil {
		t.Fatal(err)
	}
	var records []types.ObservationRecord
	at := time.Unix(1751600000, 0).UTC()
	for _, view := range views {
		result := tracequery.Run(idx, tracequery.Query{View: view, Thread: thread, TimeStart: start, TimeEnd: end})
		records = append(records, traceQueryTypedObservations(result, filepath.Base(tracePath), "p-"+view, "r", "", at)...)
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) == 0 {
		t.Fatal("no projection")
	}
	model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	return runtimeTraceProjTreeFence(model, true)
}

const proofDonorDonghu = "../../eval/fixtures/real_traces/donghu.ftrace"

// TestProofDonorStatsOnlyDispatchRefined — the r3 banned dispatch shape (no
// rank/bundle view anywhere): the ×4 raw-state fold row must now wear the
// refined word + the unanimous 等待对象 symbol from the stats-face donor.
func TestProofDonorStatsOnlyDispatchRefined(t *testing.T) {
	if _, err := os.Stat(proofDonorDonghu); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	fence := proofDonorFence(t, proofDonorDonghu, "CompThread_0-2955", 13762.791708, 13763.024898,
		[]string{"window_stats", "wakeup_chain", "critical_blocking_calls"})
	if !strings.Contains(fence, "自身·D-state(对端未解析) 36.757ms 4次(3.774~16.064ms)") {
		t.Fatalf("stats-only dispatch must refine the ×4 fold row:\n%s", fence)
	}
	if strings.Contains(fence, "自身·D-state/iowait") {
		t.Fatalf("the banned merged word must not survive a proven shape:\n%s", fence)
	}
	if !strings.Contains(fence, "等待对象 dma_fence_default_w") {
		t.Fatalf("the unanimous wait object must ride the stats-face donor:\n%s", fence)
	}
}

// TestProofDonorWithRankDispatchRefined — the rank-family donor path stays
// first-class: adding root_cause_rank keeps the refined word (and never
// resurrects the banned form).
func TestProofDonorWithRankDispatchRefined(t *testing.T) {
	if _, err := os.Stat(proofDonorDonghu); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	fence := proofDonorFence(t, proofDonorDonghu, "CompThread_0-2955", 13762.791708, 13763.024898,
		[]string{"window_stats", "wakeup_chain", "critical_blocking_calls", "root_cause_rank"})
	if strings.Contains(fence, "自身·D-state/iowait") {
		t.Fatalf("the banned merged word must not appear with the rank donor present:\n%s", fence)
	}
	if !strings.Contains(fence, "自身·D-state") || !strings.Contains(fence, "36.757") {
		t.Fatalf("the refined seat must render with the rank donor present:\n%s", fence)
	}
	if !strings.Contains(fence, "等待对象 dma_fence_default_w") {
		t.Fatalf("the wait object must ride with the rank donor present:\n%s", fence)
	}
}

// TestProofDonorNoMarkerKeepsMergedWord — 负向 (诚实词存续): a D shape with
// NO markers anywhere yields no donor on any face — the merged
// 「D-state/iowait」 word must persist and no wait object may be fabricated.
func TestProofDonorNoMarkerKeepsMergedWord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proof_donor_neg.systrace")
	if err := os.WriteFile(path, []byte(case1DisplayPureDTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	fence := proofDonorFence(t, path, "app-100", 3.0, 3.2, []string{"window_stats", "root_cause_rank", "critical_blocking_calls"})
	if !strings.Contains(fence, "D-state/iowait") {
		t.Fatalf("an unproven D shape must keep the honest merged word:\n%s", fence)
	}
	if strings.Contains(fence, "等待对象") {
		t.Fatalf("no donor may fabricate a wait object:\n%s", fence)
	}
}


// TestProofDonorMixedMembersVetoBothLanes — 修复轮三 F1: the discriminating
// mixed shape (engine-minted, dioPartitionDisplayTrace without a rank view →
// CASE-1 absorption cannot fire and the THREE chain-lane rows ×3-merge):
// members = D 20ms proven dma_fence_default_wait + io 30ms proven same
// symbol + D 10ms unproven. AND semantics ⇒ merged word persists; unanimity
// ⇒ no 等待对象 on the family (one memberless symbol vetoes).
func TestProofDonorMixedMembersVetoBothLanes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proof_donor_mixed.systrace")
	if err := os.WriteFile(path, []byte(proofDonorMixedTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	fence := proofDonorFence(t, path, "app-100", 3.0, 3.2, []string{"window_stats", "wakeup_chain", "critical_blocking_calls"})
	workerMerged := false
	for _, line := range strings.Split(fence, "\n") {
		if !strings.Contains(line, "worker-200") {
			continue
		}
		if strings.Contains(line, "等待对象") {
			t.Fatalf("a non-unanimous ×N family must not wear a wait object:\n%s", fence)
		}
		if strings.Contains(line, "D-state/iowait") {
			workerMerged = true
		}
	}
	if !workerMerged {
		t.Fatalf("one unproven member must keep the honest merged word (AND, never OR):\n%s", fence)
	}
	if strings.Contains(fence, "等待对象 dma_fence_default_wait") {
		t.Fatalf("the seed symbol must not survive the unanimity veto:\n%s", fence)
	}
}

// proofDonorMixedTrace — the F1 discriminating shape: the PROVEN D fragment
// is the largest (40ms → it seeds the ×N critical merge with refined=true +
// caller), the proven-io (5ms, same symbol) and the unproven D (10ms) follow.
// AND ⇒ the family word stays the honest merged 「D-state/iowait」; unanimity
// ⇒ the seed's dma symbol is vetoed by the callerless member.
const proofDonorMixedTrace = `
        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.050000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
       peer-300 (300) [003] .... 3.050000: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842
     worker-200 (200) [003] .... 3.051000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.055000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.060000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=004
       peer-300 (300) [003] .... 3.060000: sched_blocked_reason: pid=200 iowait=1 caller=dma_fence_default_wait+0x88/0x160[sysmgr.elf] delay=913
     worker-200 (200) [004] .... 3.061000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [004] .... 3.065000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.075000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=004
     worker-200 (200) [004] .... 3.076000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [004] .... 3.119000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (200) [004] .... 3.119500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

// TestProofDonorStatsFaceEmissionPinned — 修复轮三 F3 (前提勘正实测
// 2026-07-13): the tree's refined word never consumes the window_stats-face
// notes — window_stats top records mint no nodes (RN-12 side-channel), and
// in the h2 230637-r2 dispatch shape the donor is the RANK row's own
// family-mint proof (mutating the stats-face emission off left that fence
// BYTE-IDENTICAL). The stats-face notes are the AUDIT/EVIDENCE face's
// honest enrichment, so their pin lives at the RECORD level: a stats-only
// dispatch must emit the per-group proof on every fully-proven top_d_state
// record. MUTATION self-check: removing the 件B stats-face emission
// (trace_query.go traceQueryTypedThreadDurationObservations) reds THIS test
// while the critical/rank-face pins stay green (the two donors are no
// longer mutual cover for it).
func TestProofDonorStatsFaceEmissionPinned(t *testing.T) {
	if _, err := os.Stat(proofDonorDonghu); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), proofDonorDonghu)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "window_stats", Thread: "CompThread_0-2955", TimeStart: 13762.791708, TimeEnd: 13763.024898})
	records := traceQueryTypedObservations(result, "donghu.ftrace", "p", "r", "", time.Unix(1751600000, 0).UTC())
	proven := 0
	for _, r := range records {
		if r.ClaimKey != "d_state_or_io_wait:CompThread_0-2955" {
			continue
		}
		notes := strings.Join(r.RichNotes, ";")
		if !strings.Contains(notes, "dstate_all_noniowait=true") ||
			!strings.Contains(notes, "blocked_reason_caller=dma_fence_default_w") {
			t.Fatalf("a fully-proven top_d_state record must carry the proof notes: %v", r.RichNotes)
		}
		proven++
	}
	if proven != 4 {
		t.Fatalf("all four CompThread groups must publish proven records, got %d", proven)
	}
}
