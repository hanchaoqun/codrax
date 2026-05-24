package types

import (
	"reflect"
	"strings"
	"testing"
)

// TestHasContextAroundRegion_FullyCoveredAcrossOneRange exercises
// the simplest happy path: a single merged range fully contains the
// requested [start - window, end + window] interval.
func TestHasContextAroundRegion_FullyCoveredAcrossOneRange(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 100, End: 200}},
	})
	if !c.HasContextAroundRegion("a.go", 110, 190, 5) {
		t.Errorf("[110, 190]±5 inside [100, 200] must be covered")
	}
}

// TestHasContextAroundRegion_FullyCoveredAcrossAdjacentRanges checks
// that two abutting merged ranges (mergeLineRanges leaves [1,50] and
// [51,100] as one because they're adjacent — but we test the logic
// against a deliberately non-merged input via direct construction).
func TestHasContextAroundRegion_FullyCoveredAcrossAdjacentRanges(t *testing.T) {
	c := NewEvidenceClosure("")
	// SetReadRanges runs mergeLineRanges, so [1,50] + [51,100] becomes
	// [1,100]. That's the canonical state. Verify cover.
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 50}, {Start: 51, End: 100}},
	})
	if !c.HasContextAroundRegion("a.go", 25, 75, 0) {
		t.Errorf("[25, 75] inside merged [1, 100] must be covered")
	}
}

// TestHasContextAroundRegion_GapBetweenRangesUncovered: when the
// merged ranges have a real gap inside the requested interval, the
// region is NOT fully covered.
func TestHasContextAroundRegion_GapBetweenRangesUncovered(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 50}, {Start: 80, End: 200}},
	})
	if c.HasContextAroundRegion("a.go", 40, 90, 0) {
		t.Errorf("gap at lines 51-79 must make [40, 90] uncovered")
	}
}

// TestHasContextAroundRegion_WindowExpands grows the requested
// interval by `window` on both sides — a strict ±0 might match while
// ±20 might not.
func TestHasContextAroundRegion_WindowExpands(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 95, End: 105}},
	})
	if !c.HasContextAroundRegion("a.go", 100, 100, 0) {
		t.Errorf("single line 100 with window 0 inside [95, 105] must be covered")
	}
	if c.HasContextAroundRegion("a.go", 100, 100, 20) {
		t.Errorf("single line 100 with window 20 → [80, 120] is NOT inside [95, 105]")
	}
}

// TestHasContextAroundRegion_WindowClampsToOne checks the
// "expansion below line 1 stays at line 1" guard: if line 5 is
// requested with window 100, want.Start becomes 1, not -95.
func TestHasContextAroundRegion_WindowClampsToOne(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 200}},
	})
	if !c.HasContextAroundRegion("a.go", 5, 10, 100) {
		t.Errorf("window expanding below line 1 must clamp to 1, not negative")
	}
}

// TestHasContextAroundRegion_NilOrUnreadFile returns false defensively.
func TestHasContextAroundRegion_NilOrUnreadFile(t *testing.T) {
	if (*EvidenceClosure)(nil).HasContextAroundRegion("a.go", 1, 10, 0) {
		t.Error("nil closure must return false")
	}
	c := NewEvidenceClosure("")
	if c.HasContextAroundRegion("a.go", 1, 10, 0) {
		t.Error("never-read file must return false")
	}
}

// TestHasContextAroundRegion_MalformedInput returns false on
// invalid coordinates rather than silently treating them as covered.
func TestHasContextAroundRegion_MalformedInput(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 200}},
	})
	if c.HasContextAroundRegion("a.go", 0, 10, 0) {
		t.Error("start <= 0 must return false")
	}
	if c.HasContextAroundRegion("a.go", 10, 5, 0) {
		t.Error("end < start must return false")
	}
	if c.HasContextAroundRegion("", 1, 10, 0) {
		t.Error("empty file must return false")
	}
}

// TestMissingContextRegions_FullyCoveredReturnsEmpty: every requested
// region is already covered → no demand needed.
func TestMissingContextRegions_FullyCoveredReturnsEmpty(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 500}},
	})
	got := c.MissingContextRegions("a.go", []int{100, 200, 300, 400}, 10)
	if len(got) != 0 {
		t.Errorf("fully covered regions must produce empty demand, got %+v", got)
	}
}

// TestMissingContextRegions_PartiallyCoveredReturnsSlivers: a
// requested region overlapping a covered range produces demand for
// the uncovered slivers only — not the whole requested span.
func TestMissingContextRegions_PartiallyCoveredReturnsSlivers(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 50, End: 100}},
	})
	// Requested [40-110] window 0 → covered slice is [50-100], missing
	// is [40-49] + [101-110].
	got := c.MissingContextRegions("a.go", []int{40, 110}, 0)
	want := []LineRange{{Start: 40, End: 49}, {Start: 101, End: 110}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("missing slivers = %+v, want %+v", got, want)
	}
}

// TestMissingContextRegions_MultipleRegionsMerged: two overlapping
// requested regions produce a single merged demand.
func TestMissingContextRegions_MultipleRegionsMerged(t *testing.T) {
	c := NewEvidenceClosure("")
	// Empty closure → every requested region's expanded window is
	// uncovered.
	got := c.MissingContextRegions("a.go", []int{100, 105, 108, 115}, 5)
	// Expanded windows: [95, 110] and [103, 120]. Merged: [95, 120].
	want := []LineRange{{Start: 95, End: 120}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged missing = %+v, want %+v", got, want)
	}
}

// TestMissingContextRegions_WholeRegionUncovered: when nothing in
// the file is read yet, the demand is the expanded window verbatim.
func TestMissingContextRegions_WholeRegionUncovered(t *testing.T) {
	c := NewEvidenceClosure("")
	got := c.MissingContextRegions("a.go", []int{200, 220}, 5)
	want := []LineRange{{Start: 195, End: 225}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("uncovered demand = %+v, want %+v", got, want)
	}
}

// TestMissingContextRegions_MalformedSkipped: malformed pairs are
// silently dropped (consistent with the package's nil-tolerant API
// elsewhere) — they don't pollute the demand.
func TestMissingContextRegions_MalformedSkipped(t *testing.T) {
	c := NewEvidenceClosure("")
	got := c.MissingContextRegions("a.go", []int{0, 10, -5, 20, 100, 50}, 0)
	// All three pairs are malformed (start<=0, start<=0, end<start) → empty.
	if len(got) != 0 {
		t.Errorf("malformed pairs must be skipped, got %+v", got)
	}
}

// TestMissingContextRegions_OddLengthIgnoresTrailing: odd-length
// input drops the last unpaired int silently.
func TestMissingContextRegions_OddLengthIgnoresTrailing(t *testing.T) {
	c := NewEvidenceClosure("")
	got := c.MissingContextRegions("a.go", []int{100, 110, 200}, 0)
	want := []LineRange{{Start: 100, End: 110}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("odd trailing must be ignored, got %+v want %+v", got, want)
	}
}

// TestDrainSatisfiedPendingReads_LegacyFileLevelStillWorks pins the
// backward-compat half: a PendingRead with NO LineRanges (legacy
// file-level demand from chain promotion / phase1_unread / etc.) is
// drained as soon as the file enters readSet, byte-identical to the
// pre-2026-05-01 behaviour.
func TestDrainSatisfiedPendingReads_LegacyFileLevelStillWorks(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{File: "a.go", Origin: "phase1_unread", Rationale: "legacy demand"})
	c.SetReadSet(map[string]bool{"a.go": true})

	if got := c.DrainSatisfiedPendingReads(); got != 1 {
		t.Fatalf("legacy file-level PendingRead should drain when file enters readSet; got %d drained", got)
	}
	if got := c.PendingReads(); len(got) != 0 {
		t.Errorf("PendingReads should be empty after legacy drain; got %+v", got)
	}
}

// TestDrainSatisfiedPendingReads_SurgicalDemandSurvivesIncomplete pins
// the load-bearing surgical-aware drain contract. A PendingRead
// carrying LineRanges=[100,130] must NOT be drained when the LLM
// has only read [1,50] of the file — even though readSet[file]=true,
// the actual demand (lines 100-130) remains unmet. Without this the
// gate's surgical demand was silently lost on every refresh-then-
// drain cycle.
func TestDrainSatisfiedPendingReads_SurgicalDemandSurvivesIncomplete(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{
		File:       "a.go",
		Origin:     "pre_complete.multi_path_anchor",
		Rationale:  "symbol fnX region [100, 130] unread",
		LineRanges: []LineRange{{Start: 100, End: 130}},
	})
	// LLM read a different range entirely.
	c.SetReadSet(map[string]bool{"a.go": true})
	c.SetReadRanges(map[string][]LineRange{"a.go": {{Start: 1, End: 50}}})

	if got := c.DrainSatisfiedPendingReads(); got != 0 {
		t.Fatalf("surgical PendingRead with uncovered LineRanges must NOT drain; got %d drained", got)
	}
	if got := c.PendingReads(); len(got) != 1 {
		t.Fatalf("surgical PendingRead should survive; got %+v", got)
	}
	if got := c.PendingReads()[0].LineRanges; len(got) != 1 || got[0].Start != 100 || got[0].End != 130 {
		t.Errorf("LineRanges must survive the drain pass intact; got %+v", got)
	}
}

// TestDrainSatisfiedPendingReads_SurgicalDrainsWhenFullyCovered: the
// other half of the surgical contract — once the LLM has read every
// demanded range (with merge across multiple reads), the surgical
// PendingRead drains.
func TestDrainSatisfiedPendingReads_SurgicalDrainsWhenFullyCovered(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{
		File:       "a.go",
		Origin:     "pre_complete.multi_path_anchor",
		Rationale:  "two regions",
		LineRanges: []LineRange{{Start: 100, End: 130}, {Start: 200, End: 230}},
	})
	c.SetReadSet(map[string]bool{"a.go": true})
	// Both ranges are covered (with extra slack on each side).
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 90, End: 140}, {Start: 195, End: 240}},
	})

	if got := c.DrainSatisfiedPendingReads(); got != 1 {
		t.Fatalf("surgical PendingRead should drain when every LineRange is fully covered; got %d drained", got)
	}
}

// TestDrainSatisfiedPendingReads_SurgicalDrainsOnPartialAcrossReads
// confirms the merge-across-reads case: two separate read calls
// cover two separate demanded regions; both ranges qualify.
func TestDrainSatisfiedPendingReads_SurgicalDrainsOnPartialAcrossReads(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{
		File:       "a.go",
		Origin:     "pre_complete.multi_path_anchor",
		Rationale:  "two regions",
		LineRanges: []LineRange{{Start: 100, End: 130}, {Start: 200, End: 230}},
	})
	c.SetReadSet(map[string]bool{"a.go": true})
	// Only one range covered; the second is missing.
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 90, End: 140}},
	})

	if got := c.DrainSatisfiedPendingReads(); got != 0 {
		t.Fatalf("partial coverage of surgical demand must NOT drain; got %d drained", got)
	}
}

// TestRepairDirectiveBridge_PropagatesLineRanges: future-proofing.
// AddRepair with Kind=RepairReadFile + LineRanges populated must
// propagate the LineRanges into the auto-bridged PendingRead so any
// future enforcer wiring surgical demand through AddRepair (instead
// of the direct AddPendingRead path multipath uses) gets the same
// surgical recovery in runForcedReads. Without this propagation the
// bridge silently downgrades surgical demand to full-file reads.
func TestRepairDirectiveBridge_PropagatesLineRanges(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddRepair(RepairDirective{
		Kind:       RepairReadFile,
		Files:      []string{"a.go"},
		Rationale:  "surgical via bridge",
		Origin:     "future_enforcer",
		LineRanges: []LineRange{{Start: 50, End: 80}, {Start: 100, End: 120}},
	})
	got := c.PendingReads()
	if len(got) != 1 {
		t.Fatalf("AddRepair(RepairReadFile) bridge must produce exactly one PendingRead; got %d", len(got))
	}
	if got[0].File != "a.go" {
		t.Errorf("File = %q, want %q", got[0].File, "a.go")
	}
	if got[0].Origin != "auto_bridge.future_enforcer" {
		t.Errorf("Origin = %q, want auto_bridge.future_enforcer", got[0].Origin)
	}
	if len(got[0].LineRanges) != 2 {
		t.Fatalf("bridge must propagate LineRanges (length); got %+v", got[0].LineRanges)
	}
	if got[0].LineRanges[0] != (LineRange{Start: 50, End: 80}) {
		t.Errorf("LineRanges[0] = %+v, want {50, 80}", got[0].LineRanges[0])
	}
	if got[0].LineRanges[1] != (LineRange{Start: 100, End: 120}) {
		t.Errorf("LineRanges[1] = %+v, want {100, 120}", got[0].LineRanges[1])
	}
	repairs := c.ActiveRepairs()
	if len(repairs) != 1 {
		t.Fatalf("ActiveRepairs length = %d, want 1", len(repairs))
	}
	if len(repairs[0].LineRanges) != 2 {
		t.Fatalf("ActiveRepairs must preserve surgical line ranges; got %+v", repairs[0])
	}
	if repairs[0].LineRanges[0] != (LineRange{Start: 50, End: 80}) ||
		repairs[0].LineRanges[1] != (LineRange{Start: 100, End: 120}) {
		t.Fatalf("ActiveRepairs line ranges = %+v, want original surgical ranges", repairs[0].LineRanges)
	}
}

// TestRepairDirectiveBridge_NilLineRangesNoOp: bridging a
// RepairReadFile without LineRanges must produce a PendingRead with
// nil LineRanges so runForcedReads still picks the legacy full-file
// fallback for the historical chain-promotion / grounder-reject path.
func TestRepairDirectiveBridge_NilLineRangesNoOp(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddRepair(RepairDirective{
		Kind:      RepairReadFile,
		Files:     []string{"legacy.go"},
		Rationale: "no surgical info",
		Origin:    "chain_promotion",
	})
	got := c.PendingReads()
	if len(got) != 1 {
		t.Fatalf("expected 1 PendingRead; got %d", len(got))
	}
	if len(got[0].LineRanges) != 0 {
		t.Errorf("nil LineRanges must propagate as nil/empty (legacy fallback); got %+v", got[0].LineRanges)
	}
}

// TestHasContextAroundRegion_EOFClampGrantsCoverage: when
// FileTotalLines is known, a context window padded past EOF (e.g.
// symbol at line 8 + window 15 in a 10-line file → asks [1, 23])
// must NOT block coverage just because lines [11, 23] are
// unreadable. The clamp treats want.End as min(want.End,
// totalLines), so reading [1, 10] of a 10-line file fully satisfies
// the demand. Without this clamp the demand is uncoverable forever
// and the gate's drain → re-raise loop never converges.
func TestHasContextAroundRegion_EOFClampGrantsCoverage(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"tiny.go": {{Start: 1, End: 10}},
	})
	c.SetFileTotalLines(map[string]int{"tiny.go": 10})
	// Symbol at line 8, window 15 → asks [-7, 23] → clamped to
	// [1, 10]. File covers [1, 10]. Should be covered.
	if !c.HasContextAroundRegion("tiny.go", 8, 8, 15) {
		t.Errorf("EOF-clamped region must be considered covered when the in-file portion is read")
	}
}

// TestHasContextAroundRegion_EOFClampNoTotal preserves the legacy
// semantic: when FileTotalLines is unknown (banner missing /
// total == 0), no clamp applies and an over-EOF window genuinely
// fails coverage. This is the "we cannot prove it's covered"
// degraded mode.
func TestHasContextAroundRegion_EOFClampNoTotal(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"unknown.go": {{Start: 1, End: 10}},
	})
	// No SetFileTotalLines call.
	if c.HasContextAroundRegion("unknown.go", 8, 8, 15) {
		t.Errorf("without total, over-EOF window should NOT be coverable (legacy fallback)")
	}
}

// TestMissingContextRegions_EOFClampShrinksDemand: the demanded
// LineRange returned to the multipath gate must be clamped to
// FileTotalLines so the surgical demand never names lines past EOF.
// Without this, the gate would request unreadable lines and the
// closure could never drain the demand even after an honest
// forced-read consumed every available line.
func TestMissingContextRegions_EOFClampShrinksDemand(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetFileTotalLines(map[string]int{"tiny.go": 10})
	// Symbol [8, 8] with window 15 against an empty closure.
	got := c.MissingContextRegions("tiny.go", []int{8, 8}, 15)
	if len(got) != 1 {
		t.Fatalf("expected one demand range, got %+v", got)
	}
	if got[0].Start != 1 || got[0].End != 10 {
		t.Errorf("EOF-clamped demand = [%d, %d], want [1, 10]", got[0].Start, got[0].End)
	}
}

// TestAddPendingRead_SurgicalRefreshOnMatchingKey pins the
// surgical-aware dedup: when a NEW PendingRead has the same
// (File, Origin) as an existing one but DIFFERENT LineRanges, the
// existing entry's LineRanges and Rationale are REPLACED by the
// new ones. Without this, two consecutive emit_investigation_complete
// calls within one dispatch (LLM retries without intervening reads)
// would silently drop the second gate's refined demand, leaving
// the closure with a stale LineRanges that no longer matches what
// the gate just computed.
func TestAddPendingRead_SurgicalRefreshOnMatchingKey(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{
		File:       "a.go",
		Origin:     "pre_complete.multi_path_anchor",
		Rationale:  "demand v1: lines 100-130",
		LineRanges: []LineRange{{Start: 100, End: 130}},
	})
	// Second emit (same key, refined demand).
	c.AddPendingRead(PendingRead{
		File:       "a.go",
		Origin:     "pre_complete.multi_path_anchor",
		Rationale:  "demand v2: lines 100-130 + 200-230",
		LineRanges: []LineRange{{Start: 100, End: 130}, {Start: 200, End: 230}},
	})

	got := c.PendingReads()
	if len(got) != 1 {
		t.Fatalf("dedup-by-(File,Origin) must keep one entry; got %d", len(got))
	}
	if !strings.Contains(got[0].Rationale, "v2") {
		t.Errorf("Rationale must be refreshed to the latest demand; got %q", got[0].Rationale)
	}
	if len(got[0].LineRanges) != 2 {
		t.Errorf("LineRanges must be replaced with the latest (2 entries); got %+v", got[0].LineRanges)
	}
}

// TestAddPendingRead_LegacyNoLineRanges_FirstWriteWins preserves
// the historical "first add wins" behaviour for callers that never
// populate LineRanges (chain promotion, grounder reject, etc.) so
// no legacy emitter regresses on the new refresh logic.
func TestAddPendingRead_LegacyNoLineRanges_FirstWriteWins(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{File: "a.go", Origin: "chain_promotion", Rationale: "first"})
	c.AddPendingRead(PendingRead{File: "a.go", Origin: "chain_promotion", Rationale: "second (should be dropped)"})

	got := c.PendingReads()
	if len(got) != 1 {
		t.Fatalf("expected 1 entry; got %d", len(got))
	}
	if got[0].Rationale != "first" {
		t.Errorf("legacy nil-LineRanges path should preserve first-write-wins; got rationale=%q", got[0].Rationale)
	}
}

// TestDrainSatisfiedPendingReads_EOFClampDrainsSurgical pins the
// drain-side EOF clamp: a PendingRead whose LineRanges include the
// over-EOF tail of a previously-clamped demand must drain once the
// in-file portion is covered. This is the round-trip integration:
// gate produces clamped demand → forced-read covers what exists →
// drain succeeds → no infinite re-raise.
func TestDrainSatisfiedPendingReads_EOFClampDrainsSurgical(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{
		File:       "tiny.go",
		Origin:     "pre_complete.multi_path_anchor",
		Rationale:  "symbol fnX at line 8, window padded past EOF",
		LineRanges: []LineRange{{Start: 1, End: 23}}, // raw demand including over-EOF tail
	})
	c.SetReadSet(map[string]bool{"tiny.go": true})
	c.SetReadRanges(map[string][]LineRange{
		"tiny.go": {{Start: 1, End: 10}}, // every readable line covered
	})
	c.SetFileTotalLines(map[string]int{"tiny.go": 10})

	if got := c.DrainSatisfiedPendingReads(); got != 1 {
		t.Fatalf("EOF-clamped surgical PendingRead must drain when in-file portion is covered; got %d drained", got)
	}
}
