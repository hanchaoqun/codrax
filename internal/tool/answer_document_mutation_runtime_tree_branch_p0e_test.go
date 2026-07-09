package tool

// P0-E CHAIN-PATH display pins (ledger §22.1, huadong_78 §24.11-A witness,
// 2026-07-09): the tree's depth attach carries a CHAIN DOMAIN — the elected
// trunk is ONE real branch, and a node measured in a DIFFERENT branch never
// fabricates a trunk position off its same-valued depth (the fake-L26/L27
// family's attach half); it keeps its honest 未接入树 seat. On a truncated
// mid-branch election the attach re-bases engine depths by the elected
// rootDepth, and trunk rows publish the engine's TRUE depth (chip == detail
// 层级, §24.12 C6 discipline).
//
// MUTATION self-checks:
//   - dropping the `node.ChainBranch != electedBranch → continue` arm reds
//     TestRuntimeTraceProjTreeBranchDomainAttachP0E (the foreign-branch node
//     re-attaches under the elected trunk);
//   - dropping the rootDepth re-base reds
//     TestRuntimeTraceProjTreeTruncatedElectionRebasesDepthAttachP0E;
//   - re-flattening the publication (reverting the per-branch record switch)
//     reds TestRuntimeTraceProjTreeHuadongEndToEndRealBranchP0E (the ladder
//     and the fake deep chips return).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func branchP0EToolPathRecord(id, target, path string, branch, branches int) types.ObservationRecord {
	record := anchorB1ToolPathRecord(id, target, path)
	record.RichNotes = []string{
		"path=" + path,
		"target=" + target,
		"branch=" + itoaToolP0E(branch),
		"branches=" + itoaToolP0E(branches),
	}
	return record
}

func itoaToolP0E(n int) string {
	digits := ""
	if n == 0 {
		return "0"
	}
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// Branch-domain attach: same-branch depth-1 node attaches under the elected
// trunk; a foreign-branch node of the SAME depth keeps the honest unattached
// (深度/未接入树) lane instead of a fabricated trunk seat.
func TestRuntimeTraceProjTreeBranchDomainAttachP0E(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:       []string{"RSUniRenderThre-1963", "oney.hmn.berlin-42591"},
		WakeupPathBranch: 8,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E1",
				Subject: "same-branch-dep-77", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", ChainDepth: 1, ChainBranch: 8,
				ImpactMS: 4.0, CumulativeImpactMS: 4.0, Confidence: 0.9},
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E2",
				Subject: "other-branch-dep-88", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", ChainDepth: 1, ChainBranch: 3,
				ImpactMS: 3.0, CumulativeImpactMS: 3.0, Confidence: 0.9},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var sameKind, otherKind string
	found := 0
	for _, row := range model.TreeRows {
		switch row.Node.Subject {
		case "same-branch-dep-77":
			sameKind = row.Kind
			found++
		case "other-branch-dep-88":
			otherKind = row.Kind
			found++
		}
	}
	if found != 2 {
		t.Fatalf("both cause nodes must keep a tree seat (永不静默丢), rows=%d", found)
	}
	if sameKind != runtimeTraceProjTreeRowChain {
		t.Fatalf("same-branch depth-1 node must attach as a chain row, got kind %v", sameKind)
	}
	if otherKind != runtimeTraceProjTreeRowDepthless {
		t.Fatalf("foreign-branch node must keep the honest unattached lane, got kind %v", otherKind)
	}
}

// P0-E 复核收尾① EVOLUTION RECORD (对抗复核 REPRO, 2026-07-09 — evolves the
// former TestRuntimeTraceProjTreeLegacyNodesKeepDepthAttachP0E, bar RAISED):
// on a BRANCH projection an identity-less (ChainBranch==0) node takes the
// honest 未接入树 seat instead of the legacy depth attach. Rationale: the
// engine honestly stamps ChainBranch=0 on CROSS-BRANCH aggregates and the
// chain_branch note zero-drops, so a display-side 0 conflates "no identity"
// with "known cross-branch" — the huadong VSync×7 aggregate fake-attached to
// the elected trunk claiming a wakeup relation (有损信号禁作硬门). TRUE
// legacy nodes are unreachable inside a branch projection (per-artifact
// partition: one compile = one producer version), so nothing legitimate is
// lost. The legacy-projection lane (electedBranch==0) keeps the legacy
// attach byte-stably — pinned by the control below.
func TestRuntimeTraceProjTreeCrossBranchAggregateNeverFakesTrunkSeatP0E(t *testing.T) {
	// REPRO shape: elected branch-8 trunk + a cross-branch aggregate row
	// (engine ChainBranch=0, zero-dropped note) at depth 1.
	projection := types.TraceCausalProjection{
		WakeupPath:       []string{"RSUniRenderThre-1963", "oney.hmn.berlin-42591"},
		WakeupPathBranch: 8,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "E1",
				Subject: "VSyncGenerator-2270", Object: "s_sleep", StateKind: "s_sleep",
				Predicate:      "wakeup_causal_aggregate",
				ChainRelevance: "on_chain", ChainDepth: 1, // ChainBranch 0: cross-branch
				ImpactMS: 8.4, CumulativeImpactMS: 8.4, Confidence: 0.82},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.TreeRows {
		if row.Node.Subject != "VSyncGenerator-2270" {
			continue
		}
		if row.Kind == runtimeTraceProjTreeRowChain {
			t.Fatalf("REPRO: a cross-branch aggregate (ChainBranch=0) must never fake a trunk wakeup seat on a branch projection, got %+v", row)
		}
		if row.Kind != runtimeTraceProjTreeRowDepthless {
			t.Fatalf("the cross-branch aggregate keeps the honest unattached seat, got kind %v", row.Kind)
		}
		return
	}
	t.Fatal("aggregate row lost its seat entirely (永不静默丢)")
}

// Legacy-projection control: with NO elected branch (legacy pool), the
// pre-P0-E depth attach stays byte-stable for identity-less nodes.
func TestRuntimeTraceProjTreeLegacyProjectionKeepsDepthAttachP0E(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"RSUniRenderThre-1963", "oney.hmn.berlin-42591"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E1",
				Subject: "legacy-dep-99", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 4.0, CumulativeImpactMS: 4.0, Confidence: 0.9},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.TreeRows {
		if row.Node.Subject == "legacy-dep-99" {
			if row.Kind != runtimeTraceProjTreeRowChain {
				t.Fatalf("legacy projection must keep the legacy depth attach, got kind %v", row.Kind)
			}
			return
		}
	}
	t.Fatal("legacy node lost its seat")
}

// Truncated mid-branch election: rootDepth re-bases the attach — an engine
// depth-3 node of the elected branch sits at trunk position 1 (displayed
// Depth stays the TRUE engine depth 3), and an engine depth-1 node (below the
// displayed root) never attaches.
func TestRuntimeTraceProjTreeTruncatedElectionRebasesDepthAttachP0E(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:            []string{"gpu-pm-release-327", "oney.hmn.berlin-42591"},
		WakeupPathUserElected: true,
		WakeupPathBranch:      2,
		WakeupPathRootDepth:   2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E1",
				Subject: "deep-dep-55", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", ChainDepth: 3, ChainBranch: 2,
				ImpactMS: 4.0, CumulativeImpactMS: 4.0, Confidence: 0.9},
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E2",
				Subject: "below-root-dep-66", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", ChainDepth: 1, ChainBranch: 2,
				ImpactMS: 3.0, CumulativeImpactMS: 3.0, Confidence: 0.9},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var deepRow, belowRow *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		switch model.TreeRows[i].Node.Subject {
		case "deep-dep-55":
			deepRow = &model.TreeRows[i]
		case "below-root-dep-66":
			belowRow = &model.TreeRows[i]
		}
	}
	if deepRow == nil || deepRow.Kind != runtimeTraceProjTreeRowChain {
		t.Fatalf("elected-branch engine-depth-3 node must attach on the re-based trunk, got %+v", deepRow)
	}
	if deepRow.Depth != 3 {
		t.Fatalf("attached row must publish the TRUE engine depth (chip==detail 层级), got %d want 3", deepRow.Depth)
	}
	if belowRow == nil || belowRow.Kind == runtimeTraceProjTreeRowChain {
		t.Fatalf("a node BELOW the displayed root must not attach to the trunk, got %+v", belowRow)
	}
	// Trunk row itself publishes the true engine depth (rel 1 + rootDepth 2).
	for _, row := range model.TreeRows {
		if row.Kind == runtimeTraceProjTreeRowChain && row.Node.Subject == "gpu-pm-release-327" {
			if row.Depth != 3 {
				t.Fatalf("truncated-election trunk row must carry the engine depth 3, got %d", row.Depth)
			}
			return
		}
	}
	t.Fatal("trunk row missing")
}

// End-to-end huadong shape (§24.11-A witness, replay acceptance §22.1): the
// per-branch records compile into a projection whose trunk is the ELECTED
// REAL branch — the deep chain renders at true depths L1..L3, the seven
// ping-pong branches never serialize into a trunk ladder (no 省略 fold, no ↺
// cycle row — the LAD folding is naturally inert on a real branch), and a
// foreign-branch rank row keeps its honest unattached seat instead of a fake
// deep chip.
func TestRuntimeTraceProjTreeHuadongEndToEndRealBranchP0E(t *testing.T) {
	oney := "oney.hmn.berlin-42591"
	records := []types.ObservationRecord{}
	for branch := 1; branch <= 7; branch++ {
		records = append(records, branchP0EToolPathRecord(
			"branch-"+itoaToolP0E(branch), oney,
			"VSyncGenerator-2270 -> "+oney, branch, 8))
	}
	records = append(records, branchP0EToolPathRecord("branch-8", oney,
		"tppmgr-idle-0-185 -> gpu-pm-release-327 -> RSUniRenderThre-1963 -> "+oney, 8, 8))
	// E7 analog: the deep branch's depth-1 dependency (subject ON the elected
	// trunk) — pre-P0-E this row surfaced at the flattened position 链上L26.
	rsuni := anchorB1ToolPathRecord("rank-e7", "RSUniRenderThre-1963", "runnable_wait")
	rsuni.Predicate = "root_cause_primary"
	rsuni.ClaimKey = "root_cause_primary:RSUniRenderThre-1963"
	rsuni.RichNotes = []string{"rank=9", "type=runnable_wait", "chain_relevance=on_chain",
		"chain_depth=1", "chain_branch=8", "impact_ms=4.115", "cumulative_impact_ms=4.115"}
	// Ping-pong branch rank row: VSync at depth 1 of branch 2 — must NOT
	// attach to the elected branch-8 trunk at L1.
	vsync := anchorB1ToolPathRecord("rank-vsync", "VSyncGenerator-2270", "sleep_wait")
	vsync.Predicate = "root_cause_secondary"
	vsync.ClaimKey = "root_cause_secondary:VSyncGenerator-2270"
	vsync.RichNotes = []string{"rank=10", "type=sleep_wait", "chain_relevance=on_chain",
		"chain_depth=1", "chain_branch=2", "impact_ms=1.2", "cumulative_impact_ms=1.2"}
	// 复核收尾① REPRO half: the VSync ping-pong CROSS-BRANCH aggregate — the
	// engine stamps ChainBranch=0 and the note zero-drops; it must not fake a
	// trunk seat off its depth-1 value.
	vsyncAgg := anchorB1ToolPathRecord("agg-vsync", "VSyncGenerator-2270", "s_sleep")
	vsyncAgg.Predicate = "wakeup_causal_aggregate"
	vsyncAgg.ClaimKey = "wakeup_causal_aggregate:VSyncGenerator-2270"
	vsyncAgg.RichNotes = []string{"depth=1", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"occurrences=7", "impact=8.4", "total=8.4"}
	records = append(records, rsuni, vsync, vsyncAgg)

	projection := anchorB1ToolProjection(t, []string{"42591"}, records...)
	want := []string{"tppmgr-idle-0-185", "gpu-pm-release-327", "RSUniRenderThre-1963", oney}
	if len(projection.WakeupPath) != 4 {
		t.Fatalf("the deep REAL branch must elect (deepest entity hit), got %v", projection.WakeupPath)
	}
	for i, label := range want {
		if projection.WakeupPath[i] != label {
			t.Fatalf("elected path[%d] = %q, want %q (%v)", i, projection.WakeupPath[i], label, projection.WakeupPath)
		}
	}
	if projection.WakeupPathBranch != 8 {
		t.Fatalf("elected branch must be 8, got %d", projection.WakeupPathBranch)
	}

	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if model.Target != oney {
		t.Fatalf("🎯 root must be the user thread, got %q", model.Target)
	}
	// Real trunk at true depths — the fake flattened ladder is gone (the LAD
	// cycle fold is naturally inert on a real branch: nothing repeats).
	if strings.Contains(fence, "省略") || strings.Contains(fence, "↺ 循环") {
		t.Fatalf("a real 3-hop trunk needs no fold ladder and no cycle row:\n%s", fence)
	}
	// E7 analog surfaces at its TRUE depth chip 链上L1 (pre-P0-E: the
	// flattened trunk seated it at 链上L26); the deep transits nest as plain
	// trunk rows (no-data rows carry no chip by display law).
	if !strings.Contains(fence, "链上L1\n") && !strings.Contains(fence, "·链上L1") {
		t.Fatalf("E7-analog trunk row must carry its TRUE depth chip 链上L1:\n%s", fence)
	}
	if !strings.Contains(fence, "◦ tppmgr-idle-0-185 中转") {
		t.Fatalf("the deep branch's depth-3 transit must nest on the real trunk:\n%s", fence)
	}
	if strings.Contains(fence, "链上L9") || strings.Contains(fence, "链上L26") || strings.Contains(fence, "链上L27") {
		t.Fatalf("no fabricated deep chips may survive:\n%s", fence)
	}
	// The foreign-branch VSync rank row keeps an honest unattached seat.
	if !strings.Contains(fence, "链上L1(未接入树)") {
		t.Fatalf("foreign-branch VSync row must disclose its unattached seat:\n%s", fence)
	}
	for _, row := range model.TreeRows {
		if row.Node.Subject == "VSyncGenerator-2270" {
			if row.Kind == runtimeTraceProjTreeRowChain {
				t.Fatalf("foreign-branch VSync row must not fabricate a trunk seat: %+v", row)
			}
		}
	}
}
