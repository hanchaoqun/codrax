package types

// trace_causal_projection_g1_test.go — G1 跨车道对账 compile pins (§27.2-G1,
// 2026-07-09, real_trace_campaign_20260705.md): the projection compile parses
// the engine's typed absorption markers and relocates matched
// critical_blocking nodes out of the render buckets into AbsorbedChainRows —
// BEFORE aggregation, so no ×N chimera of absorbed rows can form.
//
// Wire-note literals below are deliberate verbatim pins (the registry
// double-write discipline) — do not replace them with the constants.
//
// MUTATION self-checks:
//   - dropping the family-presence gate (relocate on the absorbed marker
//     alone) reds TestG1CompileNoFamilyKeepsBucketSeats;
//   - loosening the verbatim key join to any fuzzy match reds
//     TestG1CompileKeyMismatchKeepsBucketSeats;
//   - moving the fold AFTER aggregation reds
//     TestG1CompileNoAggregateChimera (the ×3 group would R2-merge first).

import (
	"fmt"
	"strings"
	"testing"
)

const g1CompileFamilyKey = "io_latency|pid:500|on_chain|10.000000..11.000000"

func g1CompileFamilyRecord() ObservationRecord {
	return ObservationRecord{
		ID:              "trace_query:g1#root_cause_rank:1",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: ClaimGroundingHard,
		ClaimKey:        "root_cause_primary",
		Subject:         "work-500",
		Predicate:       "root_cause_primary",
		Object:          "io_latency",
		Value:           "2.858",
		Unit:            "ms",
		RichNotes: []string{
			"rank=1", "tier=primary", "type=io_latency",
			"chain_relevance=on_chain",
			"impact_ms=2.858", "cumulative_impact_ms=2.858",
			"member_count=6", "member_fold_caliber=sum_disjoint",
			"rank_family_key=" + g1CompileFamilyKey,
			"selected_window=10.000000..11.000000",
		},
		Span:       ObservationSpan{LineStart: 2, LineEnd: 13},
		Confidence: 0.86,
	}
}

func g1CompileAbsorbedRecord(i int, relevance string) ObservationRecord {
	return ObservationRecord{
		ID:              fmt.Sprintf("trace_query:g1#critical_blocking:%d", i),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ClaimKey:        "critical_blocking:io_latency",
		Subject:         "work-500",
		Predicate:       "critical_blocking",
		Object:          fmt.Sprintf("udk-irq-%d-7%d", i, i),
		Value:           "0.500",
		Unit:            "ms",
		RichNotes: []string{
			"type=io_latency",
			"chain_relevance=" + relevance,
			"absorbed_by_rank_family=true",
			"absorbed_into=" + g1CompileFamilyKey,
			"selected_window=10.000000..11.000000",
		},
		SupportRefs: []string{fmt.Sprintf("g1.systrace:%d-%d", 3+i, 3+i)},
		Span:        ObservationSpan{LineStart: 3 + i, LineEnd: 3 + i},
		Confidence:  0.86,
	}
}

func g1CompileBucketHasAbsorbed(p TraceCausalProjection) bool {
	for _, bucket := range [][]TraceCausalProjectionNode{
		p.PrimaryRootCauses, p.OnChainCauses, p.AdjacentCauses, p.BackgroundCauses, p.SupportingHops,
	} {
		for _, node := range bucket {
			if node.AbsorbedByRankFamily {
				return true
			}
		}
	}
	return false
}

func TestG1CompileParsesAbsorptionMarkers(t *testing.T) {
	fam := traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, g1CompileFamilyRecord())
	if fam.RankFamilyKey != g1CompileFamilyKey {
		t.Fatalf("rank_family_key note must parse verbatim, got %q", fam.RankFamilyKey)
	}
	absorbed := traceCausalProjectionNodeFromRecord(TraceCausalRoleCausalHop, g1CompileAbsorbedRecord(1, "on_chain"))
	if !absorbed.AbsorbedByRankFamily || absorbed.AbsorbedInto != g1CompileFamilyKey {
		t.Fatalf("absorbed markers must parse: %+v", absorbed)
	}
}

func TestG1CompileRelocatesAbsorbedRows(t *testing.T) {
	// One on-chain absorbed row (buckets TWICE: hop copy + classified copy)
	// and one background absorbed row — every seat relocates, deduped by
	// EvidenceID, and the family row keeps its bucket seat.
	projection := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{
		g1CompileFamilyRecord(),
		g1CompileAbsorbedRecord(1, "on_chain"),
		g1CompileAbsorbedRecord(2, "background"),
	}})
	if len(projection.AbsorbedChainRows) != 2 {
		t.Fatalf("expected 2 absorbed rows relocated (EvidenceID-deduped), got %d: %+v",
			len(projection.AbsorbedChainRows), projection.AbsorbedChainRows)
	}
	if g1CompileBucketHasAbsorbed(projection) {
		t.Fatalf("absorbed rows must vacate every render bucket: %+v", projection)
	}
	famSeated := false
	for _, bucket := range [][]TraceCausalProjectionNode{projection.PrimaryRootCauses, projection.OnChainCauses} {
		for _, node := range bucket {
			if node.RankFamilyKey == g1CompileFamilyKey {
				famSeated = true
			}
		}
	}
	if !famSeated {
		t.Fatal("the absorbing family row must keep its bucket seat")
	}
	for _, node := range projection.AbsorbedChainRows {
		if node.AbsorbedInto != g1CompileFamilyKey {
			t.Fatalf("relocated node must keep its family pointer: %+v", node)
		}
	}
}

// TestG1CompileNoFamilyKeepsBucketSeats is the 负向保护 pin: absorbed markers
// whose family row is ABSENT from the compile universe change nothing — the
// rows keep their honest (duplicate) seats rather than silently vanishing.
func TestG1CompileNoFamilyKeepsBucketSeats(t *testing.T) {
	projection := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{
		g1CompileAbsorbedRecord(1, "on_chain"),
		g1CompileAbsorbedRecord(2, "background"),
	}})
	if len(projection.AbsorbedChainRows) != 0 {
		t.Fatalf("no family present → nothing relocates, got %+v", projection.AbsorbedChainRows)
	}
	if !g1CompileBucketHasAbsorbed(projection) {
		t.Fatal("marker-carrying rows must KEEP their bucket seats when the family is absent")
	}
}

// TestG1CompileKeyMismatchKeepsBucketSeats: the join is VERBATIM string
// equality on the engine-rendered key — a family under a different key never
// captures the absorbed rows.
func TestG1CompileKeyMismatchKeepsBucketSeats(t *testing.T) {
	fam := g1CompileFamilyRecord()
	for i, note := range fam.RichNotes {
		if strings.HasPrefix(note, "rank_family_key=") {
			fam.RichNotes[i] = "rank_family_key=io_latency|pid:501|on_chain|10.000000..11.000000"
		}
	}
	projection := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{
		fam, g1CompileAbsorbedRecord(1, "on_chain"),
	}})
	if len(projection.AbsorbedChainRows) != 0 || !g1CompileBucketHasAbsorbed(projection) {
		t.Fatalf("key mismatch must not relocate: %+v", projection.AbsorbedChainRows)
	}
}

// TestG1CompileNoAggregateChimera pins the fold's position BEFORE R1/R2/V4:
// three absorbed publications of one (subject, object) would reach the R2 ×N
// threshold — after the G1 relocation the aggregation must never see them
// (no merged ×N critical row in any bucket, three individual rows on the
// absorbed carrier).
func TestG1CompileNoAggregateChimera(t *testing.T) {
	one := g1CompileAbsorbedRecord(1, "on_chain")
	two := g1CompileAbsorbedRecord(1, "on_chain")
	two.ID = "trace_query:g1#critical_blocking:2"
	two.Span = ObservationSpan{LineStart: 20, LineEnd: 20}
	two.SupportRefs = []string{"g1.systrace:20-20"}
	three := g1CompileAbsorbedRecord(1, "on_chain")
	three.ID = "trace_query:g1#critical_blocking:3"
	three.Span = ObservationSpan{LineStart: 30, LineEnd: 30}
	three.SupportRefs = []string{"g1.systrace:30-30"}
	projection := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{
		g1CompileFamilyRecord(), one, two, three,
	}})
	if len(projection.AbsorbedChainRows) != 3 {
		t.Fatalf("all three publications must relocate individually, got %d", len(projection.AbsorbedChainRows))
	}
	for _, bucket := range [][]TraceCausalProjectionNode{
		projection.OnChainCauses, projection.SupportingHops,
	} {
		for _, node := range bucket {
			if node.Predicate == "critical_blocking" && node.MergedCount > 1 {
				t.Fatalf("absorbed rows must never form a post-fold ×N chimera: %+v", node)
			}
		}
	}
	for _, node := range projection.AbsorbedChainRows {
		if node.MergedCount > 1 {
			t.Fatalf("relocated nodes are pre-aggregation records, never merged rows: %+v", node)
		}
	}
}

// TestG1SameKindMergePreservesRankFamilyKeyOnlyP2b (收尾 P2-b, 对抗复核
// 2026-07-09): a family contender that merges into an R2 ×N row as a
// NON-first member must hand its RankFamilyKey to the carrier — the key is
// the 链上并入 disclosure's join identity, not family grammar — while the F-1
// chimera clear keeps wiping the family NINE fields (the merged row stays a
// plain R2 ×N row; no family wording may render off it).
func TestG1SameKindMergePreservesRankFamilyKeyOnlyP2b(t *testing.T) {
	plain := func(id string, ms float64, line int) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Role: TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: "io-800", Predicate: "root_cause_secondary", Object: "io_latency",
			ImpactMS: ms, CumulativeImpactMS: ms, Confidence: 0.8,
			LineStart: line, LineEnd: line,
		}
	}
	fam := plain("E9", 2.5, 30)
	fam.RankFamilyKey = "io_latency|pid:800|background|30.000000..31.000000"
	fam.FamilyMemberCount = 2
	fam.FamilyFoldCaliber = "sum_disjoint"
	fam.FamilyMemberRoster = []string{"dev=8:0 op=R sector=3000 1.000ms"}
	// Values >3% apart and disjoint lines — the V4 near-duplicate lane must
	// not fold them before R2 sees the ×3 group. The family row is the THIRD
	// member (non-first — the swallowed shape).
	nodes := []TraceCausalProjectionNode{plain("E7", 1.0, 10), plain("E8", 1.5, 20), fam}
	merged := traceCausalProjectionAggregateSameKind(nodes)
	if len(merged) != 1 || merged[0].MergedCount != 3 {
		t.Fatalf("expected one ×3 merged row, got %+v", merged)
	}
	if merged[0].RankFamilyKey != fam.RankFamilyKey {
		t.Fatalf("the merged carrier must keep the non-first member's RankFamilyKey, got %q", merged[0].RankFamilyKey)
	}
	// F-1 chimera guard intact: key ONLY — no family grammar survives.
	if merged[0].FamilyMemberCount != 0 || merged[0].FamilyFoldCaliber != "" || merged[0].FamilyMemberRoster != nil {
		t.Fatalf("family grammar fields must stay cleared on the ×N carrier (F-1): %+v", merged[0])
	}
}
