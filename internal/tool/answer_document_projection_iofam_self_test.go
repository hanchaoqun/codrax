package tool

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_projection_iofam_self_test.go — IOFAM-SELF pins (CAL-1 件②,
// ledger §29.39② + §29.47.4①, 2026-07-12): the self/on-chain lane's IO facet
// rows of ONE physical IO episode (interval-connected) collapse into ONE
// family seat — the 64414 witness rendered five flat rows (io_latency 3.670 /
// block_io 2.694+2.116 / io_wait 1.347+1.248) with THREE ❶. Post-fix: the
// wall-clock lead holds the single seat, members ride the LAYERED roster
// (调度等待/完成端到端/块设备层), the composite score never prints bare ms on
// the chain lane, and the seat's [E#(+N)] absorbs the members' evidence ids.

func iofamSelfNode(id, typ string, impact float64, lineStart, lineEnd int) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		EvidenceID:         id,
		Subject:            ".ugc.aweme.lite-17267",
		Object:             typ,
		TypeToken:          typ,
		Predicate:          "critical_blocking",
		Role:               types.TraceCausalRoleCausalHop,
		Causality:          "on_wakeup_chain",
		ChainRelevance:     "on_chain",
		ChainDepth:         1,
		ImpactMS:           impact,
		CumulativeImpactMS: impact,
		LineStart:          lineStart,
		LineEnd:            lineEnd,
		SupportRefs:        []string{fmt.Sprintf("donghu.ftrace:%d-%d", lineStart, lineEnd)},
		Confidence:         0.8,
	}
}

// iofamSelfProjection mirrors the 64414 witness geometry: five overlapping
// same-subject IO facet rows on the chain lane (wall-clock io_latency /
// io_wait ×2 + composite block_io ×2).
func iofamSelfProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"udk-irq-12-92", ".ugc.aweme.lite-17267"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{
			iofamSelfNode("iofam-lat", "io_latency", 3.670, 8188, 9936),
			iofamSelfNode("iofam-blk1", "block_io_by_inode", 2.694, 8200, 9900),
			iofamSelfNode("iofam-blk2", "block_io_by_inode", 2.116, 8300, 9800),
			iofamSelfNode("iofam-wait1", "io_wait", 1.347, 8400, 9700),
			iofamSelfNode("iofam-wait2", "io_wait", 1.248, 8500, 9600),
		},
	}
}

// TestIOFAMSelfFiveFacetsOneSeatLayeredRoster — the witness five-flat-row
// shape folds to one seat with the layered roster and merged evidence.
func TestIOFAMSelfFiveFacetsOneSeatLayeredRoster(t *testing.T) {
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(iofamSelfProjection(), evidence, true)
	fence := runtimeTraceProjTreeFence(model, true)
	// One seat: the max wall-clock facet (io_latency 3.670) leads; the other
	// four facets render no sibling rows (their values appear exactly once —
	// inside the roster note).
	if !strings.Contains(fence, "3.670ms") {
		t.Fatalf("the wall-clock lead must hold the family seat:\n%s", fence)
	}
	for _, v := range []string{"2.694", "2.116", "1.347", "1.248"} {
		if got := strings.Count(fence, v); got != 1 {
			t.Fatalf("family member %s must render exactly once (roster only), got %d:\n%s", v, got, fence)
		}
	}
	// Layered roster: each member wears its measuring-layer word; the
	// composite score never prints bare ms on the chain lane. The note may
	// T3-wrap at atom boundaries, so the pins run over the whitespace-folded
	// fence (rail/indent bytes removed).
	folded := strings.NewReplacer("\n", "", "│", "", " ", "").Replace(fence)
	if !strings.Contains(folded, "块设备层·块设备IO(inode)（block_io_by_inode）2.694/2.116(分数,非墙钟)") {
		t.Fatalf("the composite members must ride the 块设备层 roster with the 分数,非墙钟 disclosure:\n%s", fence)
	}
	if !strings.Contains(folded, "调度等待·iowait（io_wait）1.347/1.248ms") {
		t.Fatalf("the io_wait members must ride the 调度等待 roster:\n%s", fence)
	}
	// E# 并 merged_ids: the seat tag absorbs the four members.
	if !strings.Contains(fence, "[E1(+4)]") {
		t.Fatalf("the seat's evidence tag must absorb the members' ids:\n%s", fence)
	}
	// 徽章单点权威: at most one ❶ in the whole fence (members hold no seats).
	if got := strings.Count(fence, "❶"); got > 1 {
		t.Fatalf("family members must never each wear a lead badge, got %d ❶:\n%s", got, fence)
	}
	// Every facet keeps its evidence-index entry (lossless).
	if got := len(evidence.order); got != 5 {
		t.Fatalf("all five facet observations must stay on the evidence index, got %d", got)
	}
}

// TestIOFAMSelfNonOverlappingMemberNoLongerVetoesGroup — 修复轮 P2-2 (复核
// overlay 实证): a same-subject page_cache_churn member ELSEWHERE in the
// window (non-overlapping lines) must not veto the overlapping io pair's
// fold — the fold works per overlap connected component; the distant churn
// row keeps its own independent seat.
func TestIOFAMSelfNonOverlappingMemberNoLongerVetoesGroup(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"udk-irq-12-92", ".ugc.aweme.lite-17267"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{
			iofamSelfNode("iofam-lat", "io_latency", 3.670, 8188, 9936),
			iofamSelfNode("iofam-wait", "io_wait", 1.347, 8400, 9700),
			// The distant churn member: valid lines, zero overlap with the pair.
			iofamSelfNode("iofam-churn", "page_cache_churn", 0.600, 20000, 20100),
		},
	}
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	fence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(projection, evidence, true), true)
	if !strings.Contains(fence, "同段IO另有") || !strings.Contains(fence, "调度等待·iowait（io_wait） 1.347ms") {
		t.Fatalf("the overlapping pair must still fold (component-scoped veto):\n%s", fence)
	}
	if got := strings.Count(fence, "1.347"); got != 1 {
		t.Fatalf("the folded io_wait member must render exactly once, got %d:\n%s", got, fence)
	}
	if !strings.Contains(fence, "0.600") {
		t.Fatalf("the distant churn member keeps its own independent seat:\n%s", fence)
	}
	if strings.Contains(fence, "页缓存层·页缓存抖动") {
		t.Fatalf("the non-overlapping churn member must NOT be folded into the pair's roster:\n%s", fence)
	}
}

// TestIOFAMSelfInvalidIntervalMemberStaysStandalone — 修复轮 P2-2 fail-closed
// arm: a member without a valid line interval joins no component (keeps its
// own row) while the valid overlapping pair still folds.
func TestIOFAMSelfInvalidIntervalMemberStaysStandalone(t *testing.T) {
	invalid := iofamSelfNode("iofam-noline", "io_burst_episode", 2.222, 0, 0)
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"udk-irq-12-92", ".ugc.aweme.lite-17267"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			iofamSelfNode("iofam-lat", "io_latency", 3.670, 8188, 9936),
			iofamSelfNode("iofam-wait", "io_wait", 1.347, 8400, 9700),
			invalid,
		},
	}
	fence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if !strings.Contains(fence, "同段IO另有") {
		t.Fatalf("the valid pair must fold beside an interval-less member:\n%s", fence)
	}
	if !strings.Contains(fence, "2.222") {
		t.Fatalf("the interval-less member must keep its own row (fail-closed):\n%s", fence)
	}
	if strings.Contains(fence, "完成端到端·IO突发（io_burst_episode） 2.222") {
		t.Fatalf("an interval-less member must never enter a roster:\n%s", fence)
	}
}

// TestIOFAMSelfCompositeOnlyGroupFailsOpen — a group with no wall-clock facet
// never folds (the composite/count rows keep their independent honest form —
// the V2-P0 ⌗ lane words them).
func TestIOFAMSelfCompositeOnlyGroupFailsOpen(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"udk-irq-12-92", ".ugc.aweme.lite-17267"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			iofamSelfNode("iofam-blk1", "block_io_by_inode", 2.694, 8200, 9900),
			iofamSelfNode("iofam-blk2", "block_io_by_inode", 2.116, 8300, 9800),
		},
	}
	fence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if strings.Contains(fence, "同段IO另有") {
		t.Fatalf("a composite-only group must fail open (no wall-clock seat holder):\n%s", fence)
	}
	for _, v := range []string{"2.694", "2.116"} {
		if !strings.Contains(fence, v) {
			t.Fatalf("unfolded composite row %s must keep rendering:\n%s", v, fence)
		}
	}
}
