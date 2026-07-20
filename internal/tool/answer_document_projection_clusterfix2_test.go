package tool

// answer_document_projection_clusterfix2_test.go — CLUSTER-FIX-2 件1 (S1)
// display pins: the freq_only capability clause forks ONLY on the typed
// single-cluster cause token (audit 底稿 cluster_audit_code_20260718.md S1:
// 「不可判」措辞失实 for the container single-policy capture — the structure
// IS judged, the missing piece is CROSS-cluster capability information);
// every other reason and every reason-less record keeps the ruled legacy
// wording byte-identically, and the gated lane (no reason twin this batch)
// is byte-identical by construction.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Wire-token drift pin (the CAP mirror precedent).
func TestClusterFix2ReasonTokenMirrorsEngine(t *testing.T) {
	if runtimeTraceCapabilityFreqOnlyReasonSingleCluster != tracequery.CoreCapabilityFreqOnlyReasonSingleCluster {
		t.Fatalf("display single-cluster reason token drifted from the engine constant (core_capability.go)")
	}
}

// The clause single point: single_cluster forks; every other member of the
// closed reason set renders the legacy bytes EXACTLY (byte-identity pinned
// against the reason-less form).
func TestClusterFix2SingleClusterClauseFork(t *testing.T) {
	const freqOnly = runtimeTraceCapabilitySourceFreqOnly
	wantZH := "仅单簇有频点采样,无跨簇算力信息,按纯频率比折算(单簇内等价)"
	wantEN := "only one cluster carries frequency samples — no cross-cluster capability information, frequency-ratio fold only (equivalent within the single cluster)"
	if got := runtimeTraceProjCapabilityCaliberClauseReason(freqOnly, "", runtimeTraceCapabilityFreqOnlyReasonSingleCluster, true); got != wantZH {
		t.Fatalf("zh single-cluster clause = %q, want %q", got, wantZH)
	}
	if got := runtimeTraceProjCapabilityCaliberClauseReason(freqOnly, "", runtimeTraceCapabilityFreqOnlyReasonSingleCluster, false); got != wantEN {
		t.Fatalf("EN single-cluster clause = %q, want %q", got, wantEN)
	}
	// Non-single-cluster reasons and absence: byte-identical to the legacy
	// reason-less clause (the pre-batch bytes) in both languages.
	for _, reason := range []string{"", "no_domains", "no_sampled_cluster", "cluster_overflow", "fmax_tie", "comove_floor", "comove_floor_single_burst"} {
		if reason == runtimeTraceCapabilityFreqOnlyReasonSingleCluster {
			continue
		}
		for _, zh := range []bool{true, false} {
			legacy := runtimeTraceProjCapabilityCaliberClauseTopo(freqOnly, "", zh)
			if got := runtimeTraceProjCapabilityCaliberClauseReason(freqOnly, "", reason, zh); got != legacy {
				t.Fatalf("reason %q (zh=%v) must keep the legacy bytes %q, got %q", reason, zh, legacy, got)
			}
		}
	}
	// The judged/default arms are reason-transparent (no fork outside
	// freq_only) — spot-pin the default form.
	if runtimeTraceProjCapabilityCaliberClauseReason(runtimeTraceCapabilitySourceDefault, "", runtimeTraceCapabilityFreqOnlyReasonSingleCluster, true) !=
		runtimeTraceProjCapabilityCaliberClauseTopo(runtimeTraceCapabilitySourceDefault, "", true) {
		t.Fatalf("a stray reason on a judged record must not fork the default wording")
	}
	// The legend seat is unchanged: the single-cluster phrase keeps the
	// taught 按纯频率比折算 term, so it teaches through the SAME freq_only
	// legend entry.
	if !strings.Contains(wantZH, "按纯频率比折算") {
		t.Fatalf("the single-cluster wording must keep the legend-taught term")
	}
	if mark, ok := runtimeTraceProjCapabilityCaliberMarkTopo(freqOnly, ""); !ok || mark != runtimeTraceProjMarkCaliberFreqOnlyCapability {
		t.Fatalf("the freq_only legend seat must be unchanged")
	}
}

// End-to-end through the supply-fold clause body: the dominant deficit form
// and the compressed no-deficit form both fork on the node's typed reason —
// and stay byte-identical without it.
func TestClusterFix2SupplyFoldClauseForkEndToEnd(t *testing.T) {
	node := capClauseNode(5, 15, 20, 0, 5, runtimeTraceCapabilitySourceFreqOnly)
	node.SupplyFoldCapabilityFreqOnlyReason = runtimeTraceCapabilityFreqOnlyReasonSingleCluster
	clause, _, ok := runtimeTraceProjSupplyFoldClause(node, 0, true)
	if !ok || !strings.Contains(clause, "仅单簇有频点采样,无跨簇算力信息,按纯频率比折算(单簇内等价)") {
		t.Fatalf("the dominant deficit clause must carry the single-cluster wording:\n%s", clause)
	}
	if strings.Contains(clause, "簇结构不可判") {
		t.Fatalf("the single-cluster form must not also claim 簇结构不可判:\n%s", clause)
	}
	// UXR-1 §29.36.4 ② stands: still no core-class word, still the
	// 全域最高频 basis word.
	if strings.Contains(clause, "大核") || !strings.Contains(clause, "按全域最高频折算") {
		t.Fatalf("core-class honesty gate / basis word must be unchanged:\n%s", clause)
	}
	// Compressed no-deficit form.
	noDeficit := capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceFreqOnly)
	noDeficit.SupplyFoldCapabilityFreqOnlyReason = runtimeTraceCapabilityFreqOnlyReasonSingleCluster
	compressed, _, ok := runtimeTraceProjSupplyFoldClause(noDeficit, 0, true)
	if !ok || !strings.Contains(compressed, "已按全域最高频(或接近)运行·无供给折算(仅单簇有频点采样,按频率比)") {
		t.Fatalf("the compressed no-deficit form must fork with the clause single point:\n%s", compressed)
	}
	compressedEN, _, ok := runtimeTraceProjSupplyFoldClause(noDeficit, 0, false)
	if !ok || !strings.Contains(compressedEN, "no supply fold (single-cluster samples only, frequency-ratio basis)") {
		t.Fatalf("the EN compressed form must fork too:\n%s", compressedEN)
	}
	// Without the reason both forms keep the pre-batch bytes (the CAP pin's
	// exact strings — absence preserves every legacy surface).
	legacy, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceFreqOnly), 0, true)
	if !ok || !strings.Contains(legacy, "已按全域最高频(或接近)运行·无供给折算(簇结构不可判,按频率比)") {
		t.Fatalf("the reason-less compressed form must keep the legacy bytes:\n%s", legacy)
	}
}

// Deliberate batch boundary: the GATED lane carries no reason twin — its
// freq_only wording is the reason-less legacy form by construction (recorded
// as a 显示小批 candidate in the node-field doc).
func TestClusterFix2GatedLaneKeepsLegacyWording(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		GatedCapabilitySource: runtimeTraceCapabilitySourceFreqOnly,
	}
	suffix := runtimeTraceProjCapabilityCaliberSuffixTopo(node.GatedCapabilitySource, node.GatedTopologySource, true)
	if suffix != ",簇结构不可判,按纯频率比折算" {
		t.Fatalf("the gated lane must keep the legacy freq_only suffix bytes, got %q", suffix)
	}
}
