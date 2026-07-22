package tool

// answer_document_projection_ispgap_test.go — ISPGAP-1 (§29.202 立案 +
// §29.204 CHAINGUARD 三路审计定谳, 2026-07-21; customer witness
// /Users/han/opt/customlogs/cust_runnable2_cli.txt:230-232: isplogcat-1225
// 整窗 D-state 144.504ms 以三无席形 — 裸「链上」无层级/深度未解析+对端未解析/
// 无板锚/零凭证 chip — 加冕 ➊ 主根因).
//
// 定谳机制(审计 F1-CHAINLESS-CROWN, HEAD 产线复现):三层 fail-open 复合 —
// 无目标(或 span 未解析 frame bundle)的无链板铸 rank 行时 Causality/
// ChainRelevance 全空且不走背景折算;①引擎序数道 rootCauseOrdinalChannel 空
// relevance→chain 通道发 Rank(chainless 板既裁设计,零动);②显示 primary/rank
// 车道(types 投影编译)按 predicate 准入、无 relevance 门;③树面 depthless 分流
// 只拦 background/adjacent,空 token 直入 ⛓ 冠冕;chip 完备臂却要求显式
// on_chain → 零凭证词。修=显示消费端三点(空 relevance 的 rank 车道行默认
// ▒ 背景席,与 hop 车道 [P1 修正轮 2026-07-06] 既有形对齐;引擎零动)。
//
// FULL-CHAIN engine-minted fixtures (§28.7 复核纪律②): trace text →
// tracequery.Run → traceQueryTypedObservations →
// CompileTraceCausalProjectionSet → tree model → fence/detail faces.

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

// ispgapChainlessDTrace — customer-shape probe (audit probes 段构造要点):
// app-100 target + worker-200 real chain edge + isplogd-1300 a daemon that
// enters D BEFORE the window and never emits an in-window event (zero wakeup
// edges, zero blocked_reason, zero peers) — the head-state prefix scan seats
// it as a whole-window 150ms D account.
const ispgapChainlessDTrace = `
     isplogd-1300 (1300) [003] .... 0.940000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=isplogd next_pid=1300 next_prio=20
     isplogd-1300 (1300) [003] .... 0.945000: sched_switch: prev_comm=isplogd prev_pid=1300 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 1.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 1.020000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (200) [002] .... 1.021000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.022000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 1.150000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
`

// ispgapChainlessRunnableTrace — the whole-window RUNNABLE twin form (audit
// F1: 整窗 runnable 同构形同样加冕 ➊ ⧖): the daemon's last pre-window switch
// leaves it runnable (prev_state=R) and it never runs in-window.
const ispgapChainlessRunnableTrace = `
     isplogr-1400 (1400) [003] .... 0.940000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=isplogr next_pid=1400 next_prio=20
     isplogr-1400 (1400) [003] .... 0.945000: sched_switch: prev_comm=isplogr prev_pid=1400 prev_prio=20 prev_state=R ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 1.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 1.020000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (200) [002] .... 1.021000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.022000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 1.150000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
`

func ispgapChainlessRecords(t *testing.T, trace, name string) []types.ObservationRecord {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	// The chainless entrance: an UNTARGETED root_cause_rank (no pid, no
	// thread) — the engine board carries no causal chain, mints every row
	// with empty Causality/ChainRelevance at FULL value, and issues chain-
	// channel ordinals (the adjudicated chainless fail-open, untouched).
	result := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", TimeStart: 1.0, TimeEnd: 1.15, MinDurationMs: 0.05})
	records := traceQueryTypedObservations(result, name, "payload-ref", "raw-ref", "", time.Unix(1753100000, 0).UTC())
	if len(records) == 0 {
		t.Fatal("engine fixture minted no observation records")
	}
	return records
}

// ispgapAssertBackgroundOnly asserts the post-fix customer-shape landing: the
// daemon holds exactly its honest ▒ background seat — never a ⛓/depthless
// chain row, never a rank-board crown, never the primary bucket.
func ispgapAssertBackgroundOnly(t *testing.T, records []types.ObservationRecord, daemon string) {
	t.Helper()
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	for _, node := range projection.PrimaryRootCauses {
		if strings.Contains(node.Subject, daemon) {
			t.Fatalf("件2' 主臂: the undeclared-relevance chainless row must not enter PrimaryRootCauses: %+v", node)
		}
	}
	for _, node := range projection.OnChainCauses {
		if strings.Contains(node.Subject, daemon) {
			t.Fatalf("件2' 主臂: the undeclared-relevance chainless row must not enter OnChainCauses: %+v", node)
		}
	}
	backgroundSeat := false
	for _, node := range projection.BackgroundCauses {
		if strings.Contains(node.Subject, daemon) {
			backgroundSeat = true
			if node.ChainRelevance != "background" {
				t.Fatalf("件2' 主臂: the demoted seat must carry the explicit background token, got %q", node.ChainRelevance)
			}
		}
	}
	if !backgroundSeat {
		t.Fatalf("件2' PTS 臂: the daemon's account must keep its honest ▒ seat (never a silent drop); background bucket: %+v", projection.BackgroundCauses)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.TreeRows {
		if strings.Contains(row.Node.Subject, daemon) {
			t.Fatalf("件2' 树臂: the daemon must hold no chain/depthless tree row (kind=%s):\n%+v", row.Kind, row.Node)
		}
	}
	seated := false
	for _, row := range model.Background {
		if strings.Contains(row.Node.Subject, daemon) {
			seated = true
		}
	}
	if !seated {
		t.Fatalf("件2' PTS 臂: the daemon must render in the ▒ background stanza")
	}
	fence := runtimeTraceProjTreeFence(model, true)
	for _, line := range strings.Split(fence, "\n") {
		if !strings.Contains(line, daemon) {
			continue
		}
		for _, banned := range []string{"➊", "➋", "➌", "➍", "➎", "链上", "根因排序#"} {
			if strings.Contains(line, banned) {
				t.Fatalf("件2' 冠冕负臂: the daemon line must not wear %q:\n%s\nfull fence:\n%s", banned, line, fence)
			}
		}
	}
	if !strings.Contains(fence, daemon) {
		t.Fatalf("件2' PTS 臂: the daemon must stay visible on the fence:\n%s", fence)
	}
}

// TestISPGAPChainlessWholeWindowDReturnsToBackground — 件1'/件2' customer
// shape, D form: the whole-window D daemon on a chainless board lands in ▒
// (the pre-fix crown was └─链上·深度未解析─ ➊ ⛓ 150.000ms(全额) with zero
// credential chip — the witness E10 form byte-for-feature).
func TestISPGAPChainlessWholeWindowDReturnsToBackground(t *testing.T) {
	records := ispgapChainlessRecords(t, ispgapChainlessDTrace, "ispgap_chainless_d.systrace")
	ispgapAssertBackgroundOnly(t, records, "isplogd-1300")
}

// TestISPGAPChainlessWholeWindowRunnableReturnsToBackground — the runnable
// twin form (pre-fix: ➊ ⧖ 调度延迟 150ms(全额)).
func TestISPGAPChainlessWholeWindowRunnableReturnsToBackground(t *testing.T) {
	records := ispgapChainlessRecords(t, ispgapChainlessRunnableTrace, "ispgap_chainless_r.systrace")
	ispgapAssertBackgroundOnly(t, records, "isplogr-1400")
}

// TestISPGAPUnresolvedFrameBundleReturnsToBackground — the customer's ACTUAL
// entrance (audit F1 second variant): a frame_root_cause_bundle whose span
// pattern resolves to nothing (doFrame 76795 无对应命名 span) runs its
// internal rank on the same chainless form — the daemon must land in ▒, not
// on the ⛓ crown lane.
func TestISPGAPUnresolvedFrameBundleReturnsToBackground(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ispgap_bundle.systrace")
	if err := os.WriteFile(path, []byte(ispgapChainlessDTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{
		View: "frame_root_cause_bundle", SpanName: "Choreographer#doFrame 76795",
		TimeStart: 1.0, TimeEnd: 1.15, MinDurationMs: 0.05,
	})
	records := traceQueryTypedObservations(result, "ispgap_bundle.systrace", "payload-ref", "raw-ref", "", time.Unix(1753100000, 0).UTC())
	if len(records) == 0 {
		t.Skip("bundle minted no records on the unresolved-span form (entrance fail-closed); the untargeted-rank pins own the mechanism")
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) == 0 {
		t.Skip("bundle records compiled no projection (entrance fail-closed); the untargeted-rank pins own the mechanism")
	}
	projection := set.Projections[0]
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.TreeRows {
		if strings.Contains(row.Node.Subject, "isplogd-1300") {
			t.Fatalf("件2' bundle 入口: the daemon must hold no chain/depthless tree row:\n%+v", row.Node)
		}
	}
	fence := runtimeTraceProjTreeFence(model, true)
	for _, line := range strings.Split(fence, "\n") {
		if !strings.Contains(line, "isplogd-1300") {
			continue
		}
		for _, banned := range []string{"➊", "链上", "根因排序#"} {
			if strings.Contains(line, banned) {
				t.Fatalf("件2' bundle 入口冠冕负臂: the daemon line must not wear %q:\n%s", banned, line)
			}
		}
	}
}

// TestISPGAPUnionTruncatedBoardKeepsBackgroundSeat — 件3' (audit
// F5-UNION-SILENT-DROP 伴随回归面): when a TARGETED board's records merge with
// a chainless board's records and the targeted board carries NO twin row for
// the daemon (the Limit-truncation shape), the chainless record must keep its
// ▒ seat — never crown, never silently vanish (PTS 永不静默丢).
func TestISPGAPUnionTruncatedBoardKeepsBackgroundSeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ispgap_union.systrace")
	if err := os.WriteFile(path, []byte(ispgapChainlessDTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	targeted := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 100, TimeStart: 1.0, TimeEnd: 1.15, MinDurationMs: 0.05})
	targetedRecords := traceQueryTypedObservations(targeted, "ispgap_union.systrace", "payload-ref", "raw-ref", "", time.Unix(1753100000, 0).UTC())
	// Simulate the Limit-truncated targeted board: drop every daemon row the
	// targeted board minted (the audit variant used Limit=4; the filter is
	// the deterministic equivalent of "the targeted board has no twin").
	kept := targetedRecords[:0]
	for _, record := range targetedRecords {
		if strings.Contains(record.Subject, "isplogd-1300") {
			continue
		}
		kept = append(kept, record)
	}
	chainless := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", TimeStart: 1.0, TimeEnd: 1.15, MinDurationMs: 0.05})
	chainlessRecords := traceQueryTypedObservations(chainless, "ispgap_union.systrace", "payload-ref-2", "raw-ref-2", "", time.Unix(1753100060, 0).UTC())
	union := append(append([]types.ObservationRecord{}, kept...), chainlessRecords...)
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: union})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.TreeRows {
		if strings.Contains(row.Node.Subject, "isplogd-1300") {
			t.Fatalf("件3' 冠冕负臂: the merged chainless record must not take a chain/depthless seat:\n%+v", row.Node)
		}
	}
	seated := false
	for _, group := range [][]runtimeTraceProjTreeRow{model.Adjacent, model.Background} {
		for _, row := range group {
			if strings.Contains(row.Node.Subject, "isplogd-1300") {
				seated = true
			}
		}
	}
	if !seated {
		t.Fatalf("件3' PTS 臂: the truncated-board union must keep the daemon's stanza seat (never a silent zero):\nBackground=%+v", model.Background)
	}
}

// TestISPGAPEmptyRelevanceRankLaneNegativeArms — 件2'/件4' typed negative
// arms on the two display consumers:
//   - the display ordinal-channel classifier: an EMPTY-relevance rank-lane
//     node resolves to the background channel; a by-construction chain-view
//     node (causal_hop role, e.g. the root_evidence audit family that carries
//     no relevance note by design) keeps the chain fallback;
//   - the tree depthless fork: a stale/cross-query chain-universe copy with
//     an empty token must not enter the 链上·深度未解析 lane — it lands on
//     the ▒ stray seat.
func TestISPGAPEmptyRelevanceRankLaneNegativeArms(t *testing.T) {
	rankLane := types.TraceCausalProjectionNode{
		Role:    types.TraceCausalRolePrimaryRootCause,
		Subject: "isplogd-1300",
	}
	if got := runtimeTraceProjNodeOrdinalChannel(rankLane); got != runtimeTraceProjOrdinalChannelBackground {
		t.Fatalf("件4': empty-relevance rank-lane node must classify background, got %q", got)
	}
	rankLane.Role = types.TraceCausalRoleRootCauseContext
	if got := runtimeTraceProjNodeOrdinalChannel(rankLane); got != runtimeTraceProjOrdinalChannelBackground {
		t.Fatalf("件4': empty-relevance root-cause-context node must classify background, got %q", got)
	}
	hop := types.TraceCausalProjectionNode{
		Role:    types.TraceCausalRoleCausalHop,
		Subject: "worker-200",
	}
	if got := runtimeTraceProjNodeOrdinalChannel(hop); got != runtimeTraceProjOrdinalChannelChain {
		t.Fatalf("件4' 保真臂: the by-construction chain-view hop keeps the chain fallback, got %q", got)
	}
	declared := types.TraceCausalProjectionNode{
		Role:           types.TraceCausalRolePrimaryRootCause,
		Subject:        "worker-200",
		ChainRelevance: "on_chain",
	}
	if got := runtimeTraceProjNodeOrdinalChannel(declared); got != runtimeTraceProjOrdinalChannelChain {
		t.Fatalf("件4' 保真臂: a declared on_chain seat keeps the chain channel, got %q", got)
	}
	// Tree-fork negative arm: hand-built projection simulating a stale or
	// cross-query chain-universe copy whose relevance token is empty — the
	// depthless ⛓ lane must reject it and the ▒ stray sweep must seat it.
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-200", "app-100"},
		WindowStartTs: 1.0, WindowEndTs: 1.15,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "ispgap-stale-1",
				Subject: "isplogd-1300", Predicate: "root_cause_primary",
				Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
				Summary: "whole-window D account", Value: "150.000", Unit: "ms",
				Rank: 1, EffectiveImpactMS: 150, ImpactMS: 150, CumulativeImpactMS: 150,
				StateKind: "d_state_or_io_wait", Tier: "primary", Confidence: 0.82,
				QueryWindowStartTs: 1.0, QueryWindowEndTs: 1.15,
				LineStart: 2, LineEnd: 2,
			},
			{
				Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "ispgap-stale-2",
				Subject: "worker-200", Predicate: "root_cause_secondary",
				Object: "runnable_wait", TypeToken: "runnable_wait",
				Summary: "runnable wait", Value: "12.000", Unit: "ms",
				Rank: 2, EffectiveImpactMS: 12, ImpactMS: 12, CumulativeImpactMS: 12,
				ChainRelevance: "on_chain", ChainDepth: 1, StateKind: "runnable",
				Tier: "primary", Confidence: 0.72,
				QueryWindowStartTs: 1.0, QueryWindowEndTs: 1.15,
				LineStart: 4, LineEnd: 4,
			},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.TreeRows {
		if strings.Contains(row.Node.Subject, "isplogd-1300") {
			t.Fatalf("件2' 分流负臂: the empty-token chain-universe copy must not take a tree seat:\n%+v", row.Node)
		}
	}
	seated := false
	for _, row := range model.Background {
		if strings.Contains(row.Node.Subject, "isplogd-1300") {
			seated = true
		}
	}
	if !seated {
		t.Fatalf("件2' 分流负臂: the rejected copy keeps its ▒ stray seat (PTS): %+v", model.Background)
	}
}

// TestISPGAPChainedBoardZeroChangeWitness — 四旗舰同族 A/B 缩影(chained
// boards move zero): a TARGETED query on the same trace keeps the daemon on
// its engine-declared background lane with the explicit token — the fix's
// empty-relevance defaults never fire on a chained board (every rank row is
// normalize-stamped), so declared lanes are byte-identical by construction.
func TestISPGAPChainedBoardZeroChangeWitness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ispgap_chained.systrace")
	if err := os.WriteFile(path, []byte(ispgapChainlessDTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 100, TimeStart: 1.0, TimeEnd: 1.15, MinDurationMs: 0.05})
	records := traceQueryTypedObservations(result, "ispgap_chained.systrace", "payload-ref", "raw-ref", "", time.Unix(1753100000, 0).UTC())
	empties := 0
	for _, record := range records {
		// The RANK-ROW family only (ClaimKey "root_cause_<tier>"): the
		// valueless evidence_fact mirrors (ClaimKey "evidence_fact:…") carry
		// no notes by design and are covered by the compile/tree gates, not
		// by the engine normalize authority this witness pins.
		if !strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_") {
			continue
		}
		if !ispgapRecordDeclaresRelevance(record) {
			empties++
			t.Logf("undeclared rank-row record on a chained board: id=%s %s claim=%s", record.ID, record.Subject, record.ClaimKey)
		}
	}
	if empties != 0 {
		t.Fatalf("chained-board rank rows must all carry declared relevance (normalize authority): %d undeclared", empties)
	}
}

// ispgapRecordDeclaresRelevance reads the RAW typed wire notes (never the
// compile round-trip, which now backfills the background default): a chained-
// board rank record must carry a chain_relevance or causality note.
func ispgapRecordDeclaresRelevance(record types.ObservationRecord) bool {
	for _, note := range record.RichNotes {
		if v, ok := strings.CutPrefix(note, types.TraceNoteKeyChainRelevance+"="); ok && strings.TrimSpace(v) != "" {
			return true
		}
		if v, ok := strings.CutPrefix(note, types.TraceNoteKeyCausality+"="); ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

const ispgapTiebaCarve = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"

// ispgapSameSegmentMirrorProjection is the representative ×N 第六式 shape for
// the revisit76 bidirectional legend harness (复核 F-B, §29.207): one ▒ row
// whose two cross-record members overlap inside ONE window — the merged seat
// carries the 同段镜像 caliber (member MAX as the honest lower bound) with
// the lossless raw Σ beside it.
func ispgapSameSegmentMirrorProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WindowStartTs: 1.0,
		WindowEndTs:   1.15,
		WakeupPath:    []string{"worker-200", "app-100"},
		BackgroundCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ispgap-mirror-1",
			Subject: "isplogd-1300", Object: "d_state_or_io_wait", StateKind: "d_state_or_io_wait",
			ChainRelevance: "background",
			ImpactMS:       150.000, CumulativeImpactMS: 150.000,
			StartTs: 1.0, EndTs: 1.15,
			QueryWindowStartTs: 1.0, QueryWindowEndTs: 1.15,
			MergedCount: 2, MergedMinMS: 52.500, MergedMaxMS: 150.000,
			MergedSameSegmentMirror: true, MergedSumMS: 202.500,
			MergedEvidenceIDs: []string{"ispgap-mirror-2"},
			Confidence:        0.8,
		}},
	}
}

// TestISPGAPMirrorExemptChainedBoardByteIdentical — 复核 F-A (P1) 带镜像有链板
// A/B pin on the committed customer carve: the valueless evidence_fact mirror
// records must stay INVISIBLE display carriers — compiling the targeted-board
// ledger WITH them renders byte-identically to compiling WITHOUT them (the
// first-round background backfill surfaced them as 0.000ms ▒ twins and, worse,
// made them ambiguous AXIOM-V2 overlap partners, killing the 互指句 — the
// both-or-neither arm). The 互指句在场 half pins the cross-direction mutual
// clauses the reviewer watched die (b1: 与[E4]/与[E8]同段重叠…收益不叠加).
func TestISPGAPMirrorExemptChainedBoardByteIdentical(t *testing.T) {
	if _, err := os.Stat(ispgapTiebaCarve); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), ispgapTiebaCarve)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 59843, TimeStart: 34579.450627, TimeEnd: 34579.595131, MinDurationMs: 0.05})
	records := traceQueryTypedObservations(result, "donghu_tieba_frame.systrace", "payload-ref", "raw-ref", "", time.Unix(1753100000, 0).UTC())
	mirrors := 0
	var withoutMirrors []types.ObservationRecord
	for _, record := range records {
		if strings.HasPrefix(record.ClaimKey, "evidence_fact:root_cause_") {
			mirrors++
			continue
		}
		withoutMirrors = append(withoutMirrors, record)
	}
	if mirrors == 0 {
		t.Fatal("the engine board must mint evidence_fact rank mirrors (fixture premise)")
	}
	render := func(recs []types.ObservationRecord) (string, string) {
		set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: recs})
		if len(set.Projections) != 1 {
			t.Fatalf("expected one projection, got %d", len(set.Projections))
		}
		model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		return runtimeTraceProjTreeFence(model, true), runtimeTraceProjDetailFullText(model, true)
	}
	fenceWith, detailWith := render(records)
	fenceWithout, detailWithout := render(withoutMirrors)
	if fenceWith != fenceWithout {
		t.Fatalf("F-A: the mirror records must be invisible on the fence (WITH vs WITHOUT must be byte-identical)\n--- with ---\n%s\n--- without ---\n%s", fenceWith, fenceWithout)
	}
	if detailWith != detailWithout {
		t.Fatalf("F-A: the mirror records must be invisible on the detail face")
	}
	// 互指句在场: the AXIOM-V2 cross-direction mutual clauses render (the
	// mirror-partner ambiguity killed them both-or-neither).
	combined := fenceWith + "\n" + detailWith
	if !strings.Contains(combined, "同段重叠") || !strings.Contains(combined, "收益不叠加") {
		t.Fatalf("F-A 互指句在场: the cross-direction mutual clauses must render on the chained board:\n%s", combined)
	}
}

// TestISPGAPUnionFullMergeSameSegmentMirror — 复核 F-B (P1, §29.207 裁定) u2
// 全并集形 pin: the multi-call union WITH the targeted board's ▒ twin present
// (independent payload refs — the reviewer's c5/c6 ID-collision caveat) must
// seat the daemon ONCE at ≤ 100% of the window under the 同段镜像 caliber —
// never the 52.500+150.000=202.500ms(135%) SUM.
func TestISPGAPUnionFullMergeSameSegmentMirror(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ispgap_union_full.systrace")
	if err := os.WriteFile(path, []byte(ispgapChainlessDTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	targeted := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 100, TimeStart: 1.0, TimeEnd: 1.15, MinDurationMs: 0.05})
	targetedRecords := traceQueryTypedObservations(targeted, "ispgap_union_full.systrace", "payload-ref", "raw-ref", "", time.Unix(1753100000, 0).UTC())
	chainless := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", TimeStart: 1.0, TimeEnd: 1.15, MinDurationMs: 0.05})
	chainlessRecords := traceQueryTypedObservations(chainless, "ispgap_union_full.systrace", "payload-ref-2", "raw-ref-2", "", time.Unix(1753100060, 0).UTC())
	union := append(append([]types.ObservationRecord{}, targetedRecords...), chainlessRecords...)
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: union})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	windowMS := (1.15 - 1.0) * 1000
	seats := 0
	for _, node := range projection.BackgroundCauses {
		if !strings.Contains(node.Subject, "isplogd-1300") {
			continue
		}
		seats++
		if node.ImpactMS > windowMS+0.001 {
			t.Fatalf("F-B: the merged ▒ seat must stay ≤ the window (%.3fms), got %.3fms: %+v", windowMS, node.ImpactMS, node)
		}
		if node.MergedCount > 1 {
			if !node.MergedSameSegmentMirror {
				t.Fatalf("F-B: the overlapping cross-record merge must wear the 同段镜像 caliber: %+v", node)
			}
			if node.MergedSumMS <= node.ImpactMS {
				t.Fatalf("F-B: the lossless raw Σ must ride beside the deduplicated value: sum=%.3f value=%.3f", node.MergedSumMS, node.ImpactMS)
			}
		}
	}
	if seats != 1 {
		t.Fatalf("F-B 单席: the daemon must hold exactly ONE ▒ seat after the union merge, got %d:\n%+v", seats, projection.BackgroundCauses)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "202.500") {
		t.Fatalf("F-B 禁求和: the 202.500ms sum face must never render:\n%s", fence)
	}
	mirrorLine := false
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "同段镜像") && strings.Contains(line, "不可相加") {
			mirrorLine = true
		}
	}
	if !mirrorLine {
		t.Fatalf("F-B 话术随行: the ▒ merged row must wear the 同段镜像…不可相加 word:\n%s", fence)
	}
}

// TestISPGAPChainlessConclusionLineSpeaks — 复核 F-C (P2): the chainless
// board's crown vacuum must SPEAK — the conclusion line renders the honest
// 「窗口内未定位到链上主根因，见背景压力段」 sentence instead of the legacy
// silent "" (rank data observed, primary bucket empty, ▒ stanza seated).
func TestISPGAPChainlessConclusionLineSpeaks(t *testing.T) {
	records := ispgapChainlessRecords(t, ispgapChainlessDTrace, "ispgap_chainless_c.systrace")
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "窗口内未定位到链上主根因") || !strings.Contains(line, "见背景压力段") {
		t.Fatalf("F-C: the crown vacuum must speak the honest headline, got %q", line)
	}
	lineEN := runtimeTraceProjConclusionLine(projection, buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false), false)
	if !strings.Contains(lineEN, "no on-chain primary root cause was located in the window") {
		t.Fatalf("F-C EN mirror: got %q", lineEN)
	}
}
