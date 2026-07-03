package tool

// §7.30.3 D-round renderer pins (docs/design/
// trace_layered_root_cause_methodology_audit_20260701.md §7.30.3):
//   D1 — lock-contention rows render typed semantics + parsed holder instead
//        of "未定位线程 <ms>"; a duration is never a bare number;
//   D2 — tree/cause rows show concise zh type labels, the lossless detail
//        table gains a "类型" column with the raw English token;
//   D3 — priority-inversion rows split the gated composition and use the
//        dedicated "反转影响" state label instead of a single-state claim;
//   D4 — the lead "主根因:" label is bold and narrative-lane cause names use
//        the zh（token）combined format.
// ZH and EN are pinned symmetrically.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// dRoundContentionObs mirrors the aweme customer shape: two critical_blocking
// rows produced from ART monitor-contention prints — one with a parsed owner,
// one ownerless — both with unresolved subjects.
func dRoundContentionObs() []types.ObservationRecord {
	owned := projV3Obs("cb-owned", "critical_blocking", "critical_blocking:blocking_span",
		"unknown-thread", "NetworkKit_AssetsUtil_Operate_0-42067", "112.223", 112.223, 45696, 45696,
		"type=blocking_span", "peer=NetworkKit_AssetsUtil_Operate_0-42067",
		"blocking_kind=monitor_contention",
		"holder_site=java.lang.String[] android.content.res.AssetManager.list(java.lang.String)(AssetManager.java:1258)",
		"waiters=1", "chain_relevance=on_chain", "causality=on_wakeup_chain")
	ownerless := projV3Obs("cb-bare", "critical_blocking", "critical_blocking:blocking_span",
		"unknown-thread", "unknown-thread", "45.000", 45.0, 45697, 45697,
		"type=blocking_span", "peer=unknown-thread", "blocking_kind=lock_contention",
		"chain_relevance=on_chain", "causality=on_wakeup_chain")
	return []types.ObservationRecord{owned, ownerless}
}

func TestTraceProjectionD1ContentionRowsCarryOwnerAndSemanticsZH(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), dRoundContentionObs(), "")
	// The parsed holder renders on the row; the duration carries the typed
	// blocked-wait label; the unattributed-thread sentinel disappears.
	for _, want := range []string{
		"锁竞争等待(持有者 NetworkKit_AssetsUtil_Operate_0-42067)",
		"锁竞争·阻塞",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("D1 contention rendering missing %q:\n%s", want, md)
		}
	}
	// 2026-07-03: the sentinel wording moved from "未定位线程" to the typed
	// unresolved-peer family — a typed BlockingKind row must render its lock
	// semantics, never ANY bare unresolved-subject label (old or new).
	for _, banned := range []string{"未定位线程", "对端线程未解析"} {
		if strings.Contains(md, banned) {
			t.Fatalf("contention rows must not render as %q:\n%s", banned, md)
		}
	}
	// The ownerless row keeps the labeled fallback form (no holder suffix).
	if strings.Count(md, "锁竞争等待") < 2 {
		t.Fatalf("ownerless contention row must keep the semantic label:\n%s", md)
	}
}

// dRoundTypeObs: a primary inversion row plus an on-chain hop, with a wakeup
// path so the tree anchors — exercises the three-tier D2/D4 fidelity.
func dRoundTypeObs() []types.ObservationRecord {
	inversion := projV3Obs("root-inv", "root_cause_primary", "root_cause_primary:inv",
		"dep-200", "priority_inversion_candidate", "16.000", 16.0, 100, 200,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=running")
	hop := projV3Obs("hop-io", "wakeup_causal_impact", "wakeup_causal_impact:io",
		"io-500", "io_latency", "4.000", 4.0, 300, 400,
		"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=2", "dominant_state=io_wait")
	path := types.ObservationRecord{
		ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
		Object: "io-500 -> dep-200 -> app-1",
	}
	return []types.ObservationRecord{inversion, hop, path}
}

func TestTraceProjectionD2TypeLabelsThreeTierFidelityZH(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), dRoundTypeObs(), "")
	// D4: bold lead label + 中文（token） combined narrative format.
	if !strings.Contains(md, "**主根因:** dep-200 优先级反转候选（priority_inversion_candidate）") {
		t.Fatalf("D4 lead must use bold label + combined zh(token) cause:\n%s", md)
	}
	// D2: tree rows show the concise zh label only.
	if !strings.Contains(md, "dep-200 · 优先级反转候选") {
		t.Fatalf("D2 tree row must show the concise zh label:\n%s", md)
	}
	// D2: the detail table keeps the raw tokens in the 类型 column.
	if !strings.Contains(md, "| 类型 |") {
		t.Fatalf("detail table must carry the 类型 column:\n%s", md)
	}
	for _, want := range []string{"| priority_inversion_candidate |", "| io_latency |", "io-500 / IO延迟"} {
		if !strings.Contains(md, want) {
			t.Fatalf("D2 detail table missing raw token cell %q:\n%s", want, md)
		}
	}
}

func TestTraceProjectionD2TypeLabelsKeepRawTokensEN(t *testing.T) {
	md := audit730Render(t, audit730Bus("en"), dRoundTypeObs(), "en")
	// EN tree keeps raw tokens; the Type column mirrors them for audit parity.
	for _, want := range []string{
		"**Primary root cause:** dep-200 priority_inversion_candidate",
		"dep-200 · priority_inversion_candidate",
		"| Type |",
		"| priority_inversion_candidate |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("EN D2 rendering missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "优先级反转候选") {
		t.Fatalf("EN surface must not carry zh labels:\n%s", md)
	}
}

// dRoundInversionObs: a gated-composite inversion row whose dominant state is
// running — exactly the misleading customer shape D3 fixes.
func dRoundInversionObs() []types.ObservationRecord {
	inversion := projV3Obs("root-inv", "root_cause_primary", "root_cause_primary:inv",
		"dep-200", "priority_inversion_candidate", "16.000", 16.0, 100, 200,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=running",
		"gated_runnable=10.000", "gated_running_deficit=6.000")
	path := types.ObservationRecord{
		ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
		Object: "dep-200 -> app-1",
	}
	return []types.ObservationRecord{inversion, path}
}

func TestTraceProjectionD3InversionCompositionSplitZH(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), dRoundInversionObs(), "")
	for _, want := range []string{
		"反转影响",
		"影响构成: 可运行等待 10.000ms + 运行折算 6.000ms",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("D3 inversion composition missing %q:\n%s", want, md)
		}
	}
	// The composite row must not claim the single-state 运行占用 tag (the
	// legend sentence naming the tag family is not a row).
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "优先级反转候选") && strings.Contains(line, "运行占用") {
			t.Fatalf("inversion row must not claim a single running state:\n%s", line)
		}
	}
}

func TestTraceProjectionD3InversionCompositionSplitEN(t *testing.T) {
	md := audit730Render(t, audit730Bus("en"), dRoundInversionObs(), "en")
	for _, want := range []string{
		"inversion impact",
		"composition: runnable 10.000ms + discounted running 6.000ms",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("EN D3 inversion composition missing %q:\n%s", want, md)
		}
	}
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "priority_inversion_candidate") && strings.Contains(line, "| running") {
			t.Fatalf("EN inversion row must not claim a single running state:\n%s", line)
		}
	}
}

func TestTraceProjectionD1ContentionRowsCarryOwnerAndSemanticsEN(t *testing.T) {
	md := audit730Render(t, audit730Bus("en"), dRoundContentionObs(), "en")
	for _, want := range []string{
		"lock contention wait (owner NetworkKit_AssetsUtil_Operate_0-42067)",
		"lock contention · blocked",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("EN D1 contention rendering missing %q:\n%s", want, md)
		}
	}
	// 2026-07-03: sentinel wording renamed (see ZH twin) — ban both eras.
	for _, banned := range []string{"unattributed thread", "unresolved wait peer"} {
		if strings.Contains(md, banned) {
			t.Fatalf("EN contention rows must not render as %q:\n%s", banned, md)
		}
	}
	if strings.Count(md, "lock contention wait") < 2 {
		t.Fatalf("EN ownerless contention row must keep the semantic label:\n%s", md)
	}
}

// TestTraceProjectionSelfRowsCarryImpactShapeInTree pins the lock_001
// customer fix (2026-07-03): the TARGET's own status rows inside the tree
// fence must carry their impact-shape label (锁竞争·阻塞 for typed
// lock-contention rows) — not just the detail table's 影响形态 column.
// The tree legend promises "rows without a dominant state keep their
// impact-shape value" and every other row family already renders it.
func TestTraceProjectionSelfRowsCarryImpactShapeInTree(t *testing.T) {
	// The lock_001 shape: a contention span whose SUBJECT is the focus
	// thread itself renders as the target's own status row under 🎯 —
	// plus an anchored wakeup path so the tree does not fall flat.
	selfContention := projV3Obs("cb-self", "critical_blocking", "critical_blocking:blocking_span",
		"app-1", "NetworkKit_AssetsUtil_Operate_0-42067", "91.500", 91.5, 45696, 45696,
		"type=blocking_span", "peer=NetworkKit_AssetsUtil_Operate_0-42067",
		"blocking_kind=monitor_contention",
		"chain_relevance=on_chain", "causality=on_wakeup_chain")
	obs := append([]types.ObservationRecord{selfContention}, dRoundTypeObs()...)
	md := audit730Render(t, audit730Bus(""), obs, "")
	fence := ""
	if start := strings.Index(md, "```text"); start >= 0 {
		if end := strings.Index(md[start+7:], "```"); end >= 0 {
			fence = md[start : start+7+end]
		}
	}
	if fence == "" {
		t.Fatalf("projection tree fence missing:\n%s", md)
	}
	var contentionLine string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "│") && strings.Contains(line, "91.500ms") {
			contentionLine = line
			break
		}
	}
	if contentionLine == "" {
		t.Fatalf("contention self row missing from tree fence:\n%s", fence)
	}
	if !strings.Contains(contentionLine, "锁竞争·阻塞") {
		t.Fatalf("contention self row must carry the impact-shape label at a glance:\n%s", contentionLine)
	}
}
