package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// trace_query_supplyfold_vs2b_display_test.go — VS-2b/VS-2c (§7.10,
// docs/design/customer_dead_session_audit_20260703.md) display-wiring pins
// for the supply-fold rich notes: fmax ladder provenance, the throttling
// finding clause, and the cluster-lane corroboration caveat. All three are
// pure renderings of typed SupplyFoldBasis fields — no display-side
// computation.

func TestTraceQuerySupplyFoldRichNotesFmaxProvenance(t *testing.T) {
	notes := traceQueryTypedSupplyFoldRichNotes(&tracequery.SupplyFoldBasis{
		KnownMs: 20, FmaxKHz: 2000000, FmaxSource: tracequery.SupplyFoldFmaxSourceObserved,
	}, 5, 15)
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "fold_fmax=2.000GHz,source=observed") {
		t.Fatalf("observed-fallback provenance must publish, got:\n%s", joined)
	}
	if strings.Contains(joined, "fold_fmax_finding=") || strings.Contains(joined, "fold_cluster_lane_caveat=") {
		t.Fatalf("no throttling / no divergent lane → no extra clauses, got:\n%s", joined)
	}
}

func TestTraceQuerySupplyFoldRichNotesThrottledFindingAndLaneCaveat(t *testing.T) {
	notes := traceQueryTypedSupplyFoldRichNotes(&tracequery.SupplyFoldBasis{
		KnownMs:              10,
		FmaxKHz:              2000000,
		FmaxSource:           tracequery.SupplyFoldFmaxSourceObserved,
		LimitThrottled:       true,
		PolicyCeilingKHz:     1500000,
		TraceObservedMaxKHz:  2000000,
		ClusterLaneName:      "cpu_freq",
		ClusterLaneMaxKHz:    3000000,
		ClusterLaneDivergent: true,
	}, 3.333, 6.667)
	joined := strings.Join(notes, "\n")
	for _, want := range []string{
		"fold_fmax=2.000GHz,source=observed",
		"fold_fmax_finding=大核策略频率上限 1.50 GHz 低于全程实测峰值 2.00 GHz(仅证明策略上限存在,不单独证明热机制或实际绑定影响)",
		// 2026-07-04 review: divergent lane value renders RAW and unit-hedged
		// (单位不明) — the flag now means no unit hypothesis matched, so the
		// display must not assert a GHz reading or a direction.
		"fold_cluster_lane_caveat=簇泳道 cpu_freq 最高原始值 3000000(单位不明)在原值/千分/百万分单位假设下均与折算 fmax 2.00 GHz 相差 >10%,泳道名与单位均为厂商自由词汇仅旁证",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
}

func TestTraceQuerySupplyFoldRichNotesThrottledFindingRequiresComparisonPair(t *testing.T) {
	// Persisted pre-B37 records may carry the boolean and observed endpoint but
	// not the policy endpoint. Refuse a numeric sentence instead of borrowing
	// FmaxKHz, which can itself be the observed max-over-lanes winner.
	notes := traceQueryTypedSupplyFoldRichNotes(&tracequery.SupplyFoldBasis{
		KnownMs: 10, FmaxKHz: 2000000, FmaxSource: tracequery.SupplyFoldFmaxSourceObserved,
		LimitThrottled: true, TraceObservedMaxKHz: 2000000,
	}, 3.333, 6.667)
	if joined := strings.Join(notes, "\n"); strings.Contains(joined, "fold_fmax_finding=") {
		t.Fatalf("an incomplete comparison pair must not fabricate a policy endpoint:\n%s", joined)
	}
}

// CFR (#75 簇共频): a basis carrying cluster-reuse pairs publishes the short
// typed provenance note — and ONLY then (no reuse → no note). Verbatim
// wire-format literal below is a deliberate double-write (registry protocol).
func TestTraceQuerySupplyFoldRichNotesClusterFreqReuseDisclosure(t *testing.T) {
	notes := traceQueryTypedSupplyFoldRichNotes(&tracequery.SupplyFoldBasis{
		KnownMs: 20, FmaxKHz: 2000000, FmaxSource: tracequery.SupplyFoldFmaxSourceObserved,
		ClusterFreqReuse: []tracequery.SupplyFoldClusterReuse{{CPU: 3, DonorCPU: 2}, {CPU: 6, DonorCPU: 4}},
	}, 5, 15)
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "fold_cluster_freq_reuse=cpu3 频点=同簇 cpu2;cpu6 频点=同簇 cpu4(簇共频复用,显式拓扑)") {
		t.Fatalf("cluster-reuse provenance must publish, got:\n%s", joined)
	}
	notes = traceQueryTypedSupplyFoldRichNotes(&tracequery.SupplyFoldBasis{
		KnownMs: 20, FmaxKHz: 2000000, FmaxSource: tracequery.SupplyFoldFmaxSourceObserved,
	}, 5, 15)
	if strings.Contains(strings.Join(notes, "\n"), "fold_cluster_freq_reuse=") {
		t.Fatalf("no reuse → no note, got %v", notes)
	}
	// CFR-2 (#80) 披露区分: a derived-membership basis renders the derived
	// suffix variant instead of the explicit one.
	notes = traceQueryTypedSupplyFoldRichNotes(&tracequery.SupplyFoldBasis{
		KnownMs: 20, FmaxKHz: 2000000, FmaxSource: tracequery.SupplyFoldFmaxSourceObserved,
		ClusterFreqReuse:       []tracequery.SupplyFoldClusterReuse{{CPU: 1, DonorCPU: 3}},
		ClusterFreqReuseSource: tracequery.ClusterFreqSourceDerived,
	}, 5, 15)
	joined = strings.Join(notes, "\n")
	if !strings.Contains(joined, "fold_cluster_freq_reuse=cpu1 频点=同簇 cpu3(簇共频复用,频点变化点推导)") {
		t.Fatalf("derived-source provenance must publish the derived suffix, got:\n%s", joined)
	}
	if strings.Contains(joined, "显式拓扑") {
		t.Fatalf("derived reuse must not claim the explicit-topology suffix, got:\n%s", joined)
	}
}

// Zero-fmax basis (all-unknown fold or member-summed aggregate) publishes
// only the base accounting — no provenance, no clauses, and the lane caveat
// stays keyed on the typed divergence flag alone (一致时不加注).
func TestTraceQuerySupplyFoldRichNotesAggregateAndConsistentLaneQuiet(t *testing.T) {
	notes := traceQueryTypedSupplyFoldRichNotes(&tracequery.SupplyFoldBasis{
		KnownMs: 28, UnknownMs: 2,
	}, 7.5, 22.5)
	if len(notes) != 3 {
		t.Fatalf("zero-fmax basis must publish the base accounting only, got %v", notes)
	}
	notes = traceQueryTypedSupplyFoldRichNotes(&tracequery.SupplyFoldBasis{
		KnownMs: 10, FmaxKHz: 2000000, FmaxSource: tracequery.SupplyFoldFmaxSourceObserved,
		ClusterLaneName: "cpu_freq", ClusterLaneMaxKHz: 2000000, ClusterLaneDivergent: false,
	}, 0, 10)
	if strings.Contains(strings.Join(notes, "\n"), "fold_cluster_lane_caveat=") {
		t.Fatalf("consistent lane must not render the caveat, got %v", notes)
	}
}
