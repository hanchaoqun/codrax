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
		FmaxKHz:              1500000,
		FmaxSource:           tracequery.SupplyFoldFmaxSourceLimit,
		LimitThrottled:       true,
		TraceObservedMaxKHz:  2000000,
		ClusterLaneName:      "cpu_freq",
		ClusterLaneMaxKHz:    3000000,
		ClusterLaneDivergent: true,
	}, 3.333, 6.667)
	joined := strings.Join(notes, "\n")
	for _, want := range []string{
		"fold_fmax=1.500GHz,source=limit",
		"fold_fmax_finding=大核受策略/温控限频至 1.50 GHz,缺口部分源于压频(全程观测最高 2.00 GHz)",
		// 2026-07-04 review: divergent lane value renders RAW and unit-hedged
		// (单位不明) — the flag now means no unit hypothesis matched, so the
		// display must not assert a GHz reading or a direction.
		"fold_cluster_lane_caveat=簇泳道 cpu_freq 最高原始值 3000000(单位不明)在原值/千分/百万分单位假设下均与折算 fmax 1.50 GHz 相差 >10%,泳道名与单位均为厂商自由词汇仅旁证",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
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
