package types

// trace_causal_projection_epub_test.go — EPUB 批 pins (ledger
// real_trace_campaign_20260705.md §29.31 立案, 2026-07-11): the typed
// effective-published marker (EffectiveImpactPublished) that distinguishes a
// PUBLISHED effective attribution of 0 from an UNPUBLISHED (absent) one on
// the float64 wire. Mint = note presence at the record decode single point
// (复核 M1: the root_evidence: sentinel family exempt); propagation = R1
// same-fact merge (OR-monotone) + ×N fold (Σ-published on all-periodic,
// deliberate un-publication on the mixed clear and — 复核 L1 — on the plain
// fold's group-first inherited copy).

import "testing"

func epubEffectiveRecord(notes ...string) ObservationRecord {
	return ObservationRecord{
		ID:              "epub-root",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary:running",
		Subject:         "worker-20",
		Object:          "running",
		Value:           "3.000",
		Unit:            "ms",
		Span:            ObservationSpan{LineStart: 120, LineEnd: 140},
		RichNotes: append([]string{
			"tier=primary",
			"rank=1",
			"impact_ms=3.000",
			"cumulative_impact_ms=3.000",
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=1",
		}, notes...),
		Confidence: 0.9,
	}
}

// TestEPUBEffectivePublishedMintFromNotePresence pins the mint: the marker is
// note PRESENCE (an explicit 0.000 is present; the legacy effective_impact
// alias counts), never the parsed value — and absence stays unpublished.
func TestEPUBEffectivePublishedMintFromNotePresence(t *testing.T) {
	cases := []struct {
		name          string
		notes         []string
		wantPublished bool
		wantMS        float64
	}{
		{"explicit zero publishes", []string{"effective_impact_ms=0.000"}, true, 0},
		{"positive publishes", []string{"effective_impact_ms=4.600"}, true, 4.6},
		{"legacy alias publishes", []string{"effective_impact=2.500"}, true, 2.5},
		{"absent stays unpublished", nil, false, 0},
	}
	for _, tc := range cases {
		got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
			epubEffectiveRecord(tc.notes...),
		})
		if len(got.PrimaryRootCauses) != 1 {
			t.Fatalf("%s: fixture must compile one primary root, got %+v", tc.name, got)
		}
		node := got.PrimaryRootCauses[0]
		if node.EffectiveImpactPublished != tc.wantPublished {
			t.Fatalf("%s: published marker want %v, got %+v", tc.name, tc.wantPublished, node)
		}
		if node.EffectiveImpactMS != tc.wantMS {
			t.Fatalf("%s: effective value want %.3f, got %+v", tc.name, tc.wantMS, node)
		}
	}
}

// TestEPUBRootEvidenceSentinelZeroNeverMintsPublished pins the 复核 M1
// exemption at the mint itself: the root_evidence: audit family's
// effective_impact_ms=0.000 is a ranking-exclusion SENTINEL (the reduced-shape
// wakeup witness carries no CAP/gated/state-union provenance), so decode must
// NOT mint published from its note presence — with or without the note the
// family stays unpublished, while non-family records with the same note keep
// minting (the exemption is the claim-key prefix, nothing wider — pinned by
// TestEPUBEffectivePublishedMintFromNotePresence above).
func TestEPUBRootEvidenceSentinelZeroNeverMintsPublished(t *testing.T) {
	build := func(claimKey string, notes ...string) TraceCausalProjectionNode {
		record := ObservationRecord{
			ID:              "epub-rootev",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "missing_wakeup",
			ClaimKey:        claimKey,
			Subject:         "worker-20",
			Value:           "3.000",
			Unit:            "ms",
			Span:            ObservationSpan{LineStart: 120, LineEnd: 140},
			RichNotes:       append([]string{"tier=context_only"}, notes...),
			Confidence:      0.9,
		}
		got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{record})
		if len(got.SupportingHops) != 1 {
			t.Fatalf("fixture must compile one supporting hop, got %+v", got)
		}
		return got.SupportingHops[0]
	}
	if node := build("root_evidence:missing_wakeup", "effective_impact_ms=0.000"); node.EffectiveImpactPublished {
		t.Fatalf("the sentinel zero must not mint published on the root_evidence family: %+v", node)
	}
	if node := build("root_evidence:missing_wakeup"); node.EffectiveImpactPublished {
		t.Fatalf("note-less root_evidence must stay unpublished too: %+v", node)
	}
}

// TestEPUBSameFactMergePublishedFollowsTheFact pins the R1 propagation: when
// the non-periodic backfill arm fires, the marker travels OR-monotone with
// the fact — a published-0 survivor is never demoted to "unpublished" by a
// silent twin, a silent survivor adopts the twin's published value+marker,
// and two silent views stay silent. The periodic F6(b) arm is untouched
// (loser fields never copied over a periodic survivor).
func TestEPUBSameFactMergePublishedFollowsTheFact(t *testing.T) {
	build := func(survivorPub, loserPub bool, loserMS float64) TraceCausalProjectionNode {
		survivor := periodicVS1FoldMember("E10", 100, 36.256, 0, 0)
		survivor.PeriodicSource = false
		survivor.DetectedPeriodMS = 0
		survivor.EffectiveImpactPublished = survivorPub
		twin := periodicVS1FoldMember("E99", 100, 36.256, loserMS, 0)
		twin.PeriodicSource = false
		twin.DetectedPeriodMS = 0
		twin.EffectiveImpactPublished = loserPub
		projection := TraceCausalProjection{OnChainCauses: []TraceCausalProjectionNode{survivor, twin}}
		traceCausalProjectionMergeSameFacts(&projection)
		if len(projection.OnChainCauses) != 1 {
			t.Fatalf("same-fact twins must merge, got %+v", projection.OnChainCauses)
		}
		return projection.OnChainCauses[0]
	}
	if merged := build(false, true, 36.256); !merged.EffectiveImpactPublished || merged.EffectiveImpactMS != 36.256 {
		t.Fatalf("silent survivor must adopt the twin's published value+marker: %+v", merged)
	}
	if merged := build(true, false, 0); !merged.EffectiveImpactPublished || merged.EffectiveImpactMS != 0 {
		t.Fatalf("a published-0 survivor must stay published against a silent twin: %+v", merged)
	}
	if merged := build(false, false, 0); merged.EffectiveImpactPublished {
		t.Fatalf("two silent views must stay unpublished: %+v", merged)
	}
	// Periodic survivor: the F6(b) authoritative-0 arm keeps value AND marker
	// exactly as the survivor published them.
	survivor := periodicVS1FoldMember("E10", 100, 36.256, 0, 0)
	survivor.EffectiveImpactPublished = true
	twin := periodicVS1FoldMember("E99", 100, 36.256, 36.256, 0)
	twin.PeriodicSource = false
	twin.DetectedPeriodMS = 0
	projection := TraceCausalProjection{OnChainCauses: []TraceCausalProjectionNode{survivor, twin}}
	traceCausalProjectionMergeSameFacts(&projection)
	if merged := projection.OnChainCauses[0]; !merged.EffectiveImpactPublished || merged.EffectiveImpactMS != 0 {
		t.Fatalf("periodic survivor's published-0 must survive the merge untouched: %+v", merged)
	}
}

// TestEPUBSameKindFoldPublishedPropagation pins the ×N fold arms: an
// all-periodic fold Σ over published member discounts stays published; the
// mixed clear is a DELIBERATE un-publication (the engine never published a
// fold effective for the part-cadence sum — a dangling published-0 would
// wrongly refuse the ×N crown downstream).
func TestEPUBSameKindFoldPublishedPropagation(t *testing.T) {
	members := periodicVS1FoldMembers()
	for i := range members {
		members[i].EffectiveImpactPublished = true
	}
	folded := traceCausalProjectionAggregateSameKind(members)
	if len(folded) != 1 || !folded[0].EffectiveImpactPublished {
		t.Fatalf("all-periodic fold over published members must stay published: %+v", folded)
	}
	mixed := periodicVS1FoldMembers()[:2]
	for i := range mixed {
		mixed[i].EffectiveImpactPublished = true
	}
	plain := periodicVS1FoldMember("E20", 200, 7.000, 0, 0)
	plain.PeriodicSource = false
	plain.DetectedPeriodMS = 0
	plain.PeriodicLatenessMS = 0
	mixed = append(mixed, plain)
	folded = traceCausalProjectionAggregateSameKind(mixed)
	if len(folded) != 1 || folded[0].EffectiveImpactPublished {
		t.Fatalf("the mixed clear must un-publish with the inherited discount: %+v", folded)
	}
	if folded[0].EffectiveImpactMS != 0 || folded[0].PeriodicSource {
		t.Fatalf("mixed clear raw semantics drifted: %+v", folded[0])
	}
	// 复核 L1 (defense-in-depth): a PLAIN (all-non-periodic) fold's effective
	// slot is a group-first inherited copy — never an engine-published fold
	// effective — so a published-0 seed (the M1 sentinel-transplant family's
	// only remaining path) must not leave the ×N row published.
	plainMembers := make([]TraceCausalProjectionNode, 0, 3)
	for i, id := range []string{"E30", "E31", "E32"} {
		m := periodicVS1FoldMember(id, 300+10*i, 5.000, 0, 0)
		m.PeriodicSource = false
		m.DetectedPeriodMS = 0
		m.PeriodicLatenessMS = 0
		plainMembers = append(plainMembers, m)
	}
	plainMembers[0].EffectiveImpactPublished = true
	folded = traceCausalProjectionAggregateSameKind(plainMembers)
	if len(folded) != 1 || folded[0].MergedCount != 3 {
		t.Fatalf("expected one plain ×3 fold row, got %+v", folded)
	}
	if folded[0].EffectiveImpactPublished {
		t.Fatalf("L1: the plain fold must clear the inherited published marker: %+v", folded[0])
	}
}
