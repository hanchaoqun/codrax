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
	if strings.Contains(md, "未定位线程") {
		t.Fatalf("contention rows must not render as 未定位线程:\n%s", md)
	}
	// The ownerless row keeps the labeled fallback form (no holder suffix).
	if strings.Count(md, "锁竞争等待") < 2 {
		t.Fatalf("ownerless contention row must keep the semantic label:\n%s", md)
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
	if strings.Contains(md, "unattributed thread") {
		t.Fatalf("EN contention rows must not render as unattributed thread:\n%s", md)
	}
	if strings.Count(md, "lock contention wait") < 2 {
		t.Fatalf("EN ownerless contention row must keep the semantic label:\n%s", md)
	}
}
