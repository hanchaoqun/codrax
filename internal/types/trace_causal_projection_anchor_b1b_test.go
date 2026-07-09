package types

// B1-b anchor-election pins (§22 B1-b, huadong_01 CHAIN-PATH audit 2026-07-07).
//
// EVOLUTION RECORD (evolves the §10-B1/§12.3-裁定3 pins in
// trace_causal_projection_anchor_b1_test.go — bar NOT lowered): the original
// B1 predicate matched a path record's END element only. The huadong_01 audit
// (维度B B-1 / 维度D P0|CHAIN-PATH, both verdict-CONFIRMED) proved the
// producer's flattened multi-branch walk overshoots chain.Target when
// nil-impact transit nodes (depth 0) sort last — the END slot holds an
// artifact transit node, the user thread sits mid-path, and the end-only
// predicate is STRUCTURALLY unsatisfiable on that shape (tree root fell back
// to candidates[0] = VSyncGenerator + 免责横幅 even WITH B1 installed:
// "B1-would-NOT-fix").
//
// B1-b election law (each clause pinned below):
//   - ANY-POSITION match: the typed entity may match any path element
//     (comparator traceCausalProjectionAnchorLabelMatchesEntity unchanged —
//     tid-first, typed/prose provenance arms, cursor exclusion all intact);
//   - TRUNCATION anchoring: the elected path is cut at the matched position
//     (LAST occurrence — later elements are walk overshoot; earlier
//     duplicates are the ↺ cycle shape), so an elected WakeupPath still ends
//     at the user entity. Compile-root, never a display re-root.
//   - position-0 hits are NOT electable (the original ≥2-element rule: a root
//     with no upstream chain cannot root a tree);
//   - multi-hit tie-break (裁定, reasons in the function doc): typed Subject
//     match (the record's Subject IS threadLabel(chain.Target)) outranks a
//     position-only hit > deepest matched position (longest resolved
//     upstream trunk; path records carry no typed impact scalar so
//     "影响最大" has no precise carrier) > publication order (pin ④'s
//     original rule, kept as the terminal tie-break);
//   - no entities / no match: candidates[0] legacy lane, byte-stable.
//
// MUTATION self-check (task acceptance): reverting the any-position predicate
// to end-only matching reds TestTraceCausalProjectionAnchorElectsMidPathUserEntityB1b
// (the specimen shape's END element is the artifact transit node).
//
// P0-E CHAIN-PATH EVOLUTION RECORD (ledger §22.1, 2026-07-09): the producer's
// flattened multi-branch walk these pins were written against is RETIRED from
// engine results — per-branch TRUE path records (typed branch= note) are the
// election pool now (pins: trace_causal_projection_branch_p0e_test.go). The
// records built here carry NO branch note, so every pin below keeps
// exercising the legacy identity-less lane byte-stably; the B1-b election law
// itself (any-position match / truncation / tie ladder) is UNCHANGED and now
// picks a real branch on the main lane — semantics only strengthened.

import (
	"reflect"
	"testing"
)

// huadong specimen shape: the user thread mid-path, the END slot hijacked by
// zero-impact transit nodes (tppmgr / gpu-pm), the record Subject carrying the
// typed chain target.
func anchorB1bHuadongRecord() ObservationRecord {
	return anchorB1PathRecord("path-huadong", "oney.hmn.berlin-42591",
		"VSyncGenerator-2270 -> oney.hmn.berlin-42591 -> tppmgr-idle-7-189 -> gpu-pm-re-327")
}

// 任意位选举正向 + 截断锚定: the mid-path user entity elects and the elected
// path is truncated at the matched position — root = user entity, artifact
// tail dropped from the trunk (its records keep their own seats).
func TestTraceCausalProjectionAnchorElectsMidPathUserEntityB1b(t *testing.T) {
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{anchorB1bHuadongRecord()}, []string{"42591"})
	want := []string{"VSyncGenerator-2270", "oney.hmn.berlin-42591"}
	if !reflect.DeepEqual(got.WakeupPath, want) {
		t.Fatalf("B1-b: mid-path user entity must elect + truncate, got %v want %v", got.WakeupPath, want)
	}
	if !got.WakeupPathUserElected {
		t.Fatalf("B1-b: WakeupPathUserElected must be true on a mid-path election")
	}

	// Negative control: an entity matching nothing keeps the legacy full path
	// unelected (banner lane preserved, byte-stable with the no-entity compile).
	miss := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{anchorB1bHuadongRecord()}, []string{"99999"})
	if miss.WakeupPathUserElected || len(miss.WakeupPath) != 4 {
		t.Fatalf("B1-b: a no-match entity must keep the legacy full path unelected, got %v elected=%v",
			miss.WakeupPath, miss.WakeupPathUserElected)
	}
	if !reflect.DeepEqual(miss, TraceCausalProjectionFromObservationRecords(
		[]ObservationRecord{anchorB1bHuadongRecord()})) {
		t.Fatalf("B1-b: the no-match lane must stay byte-stable with the legacy compile")
	}
}

// ↺ repeated occurrence: the LAST occurrence is the cut point — earlier
// duplicates stay on the trunk as the legitimate cycle shape, only the
// post-target overshoot drops.
func TestTraceCausalProjectionAnchorTruncatesAtLastOccurrenceB1b(t *testing.T) {
	record := anchorB1PathRecord("path-cycle", "oney.hmn.berlin-42591",
		"oney.hmn.berlin-42591 -> waker-7 -> oney.hmn.berlin-42591 -> transit-9")
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{record}, []string{"42591"})
	want := []string{"oney.hmn.berlin-42591", "waker-7", "oney.hmn.berlin-42591"}
	if !got.WakeupPathUserElected || !reflect.DeepEqual(got.WakeupPath, want) {
		t.Fatalf("B1-b: the LAST occurrence must anchor (↺ prefix kept, overshoot dropped), got %v elected=%v",
			got.WakeupPath, got.WakeupPathUserElected)
	}
}

// Position-0 hits stay un-electable (the original ≥2-element rule): a root
// with no upstream chain cannot root a tree; the legacy first-candidate lane
// keeps the anchor and the honest banner.
func TestTraceCausalProjectionAnchorPositionZeroNotElectableB1b(t *testing.T) {
	record := anchorB1PathRecord("path-headhit", "other-77",
		"oney.hmn.berlin-42591 -> mid-5 -> other-77")
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{record}, []string{"42591"})
	if got.WakeupPathUserElected {
		t.Fatalf("B1-b: a position-0 hit must not elect, got %v", got.WakeupPath)
	}
	if len(got.WakeupPath) != 3 {
		t.Fatalf("B1-b: the legacy candidate keeps its full path, got %v", got.WakeupPath)
	}
	// F2 carrier: the hit still surfaces on the typed hits lane so the display
	// fold layer can force-expand the user entity (deepest-trunk position).
	if !reflect.DeepEqual(got.WakeupPathUserEntityHits, []int{0}) {
		t.Fatalf("B1-b F2: the position-0 hit must surface on WakeupPathUserEntityHits, got %v",
			got.WakeupPathUserEntityHits)
	}
}

// 光标负向 (F1a preserved on the ANY-POSITION arm): the model's exploration
// cursor never elects — not even when its thread sits mid-path where the new
// predicate would otherwise hit. The cursor exclusion runs at the entity
// source (observationLedgerAnchorEntities), so the election never sees it.
func TestTraceCausalProjectionAnchorCursorMidPathNeverElectsB1b(t *testing.T) {
	rm := &RequestModel{}
	rm.RuntimeTargets = []RuntimeTarget{{
		Kind: RuntimeTargetKindThread, PID: 2270, Thread: "VSyncGenerator",
		Source: RuntimeTargetSourceExplicitToolCall,
	}}
	records := []ObservationRecord{anchorB1PathRecord("path-cursor-mid", "other-88",
		"waker-5 -> VSyncGenerator-2270 -> transit-9 -> other-88")}
	ledger := CompileObservationLedger(ObservationLedgerInput{
		RequestModel: rm,
		ToolResults:  []ToolResult{{ToolName: "trace_query", Success: true, Observations: records}},
	})
	set := CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	p := set.Projections[0]
	if p.WakeupPathUserElected {
		t.Fatalf("B1-b F1a: the exploration cursor must never elect via the any-position arm, got %v", p.WakeupPath)
	}
	if len(p.WakeupPath) != 4 {
		t.Fatalf("B1-b F1a: the cursor-only ledger keeps the legacy full path, got %v", p.WakeupPath)
	}
	if p.WakeupPathUserEntityHits != nil {
		t.Fatalf("B1-b F1a: cursor threads are not user entities — no hits, got %v", p.WakeupPathUserEntityHits)
	}
}

// 多命中 tie 裁定 (reasons in the traceCausalProjectionSelectWakeupPath doc):
//  1. typed Subject match (the engine declared the chain's target to BE the
//     user entity) outranks a position-only hit — a mid-path hit may be the
//     user as transit in ANOTHER thread's chain;
//  2. then the DEEPEST matched position (longest resolved upstream trunk);
//  3. then publication order (pin ④'s original rule, terminal tie-break —
//     re-asserted by TestTraceCausalProjectionAnchorTieBreakFirstMatchingPath).
func TestTraceCausalProjectionAnchorMultiHitTieBreakB1b(t *testing.T) {
	// Subject match beats a position-only hit even when the position-only
	// candidate published FIRST and carries the DEEPER position.
	positionOnly := anchorB1PathRecord("path-position-only", "other-77",
		"a-1 -> b-2 -> c-3 -> oney.hmn.berlin-42591 -> other-77")
	subjectMatch := anchorB1PathRecord("path-subject", "oney.hmn.berlin-42591",
		"waker-5 -> oney.hmn.berlin-42591 -> transit-9")
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{positionOnly, subjectMatch}, []string{"42591"})
	want := []string{"waker-5", "oney.hmn.berlin-42591"}
	if !got.WakeupPathUserElected || !reflect.DeepEqual(got.WakeupPath, want) {
		t.Fatalf("B1-b tie 1: subject-match must outrank a position-only hit, got %v", got.WakeupPath)
	}

	// Within the same class (both subject-match), the DEEPEST matched position
	// wins regardless of publication order.
	shallow := anchorB1PathRecord("path-shallow", "oney.hmn.berlin-42591",
		"waker-5 -> oney.hmn.berlin-42591 -> transit-9")
	deep := anchorB1PathRecord("path-deep", "oney.hmn.berlin-42591",
		"w-1 -> w-2 -> w-3 -> oney.hmn.berlin-42591 -> transit-9")
	deeper := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{shallow, deep}, []string{"42591"})
	wantDeep := []string{"w-1", "w-2", "w-3", "oney.hmn.berlin-42591"}
	if !deeper.WakeupPathUserElected || !reflect.DeepEqual(deeper.WakeupPath, wantDeep) {
		t.Fatalf("B1-b tie 2: the deepest matched position must win, got %v", deeper.WakeupPath)
	}
}

// F2 carrier pin: WakeupPathUserEntityHits lists the FINAL path's typed
// user-entity positions (ascending) — the elected root AND any other user
// entity left on the trunk — and stays nil on the no-signal lane (the
// DeepEqual byte-stability pins ②/③ depend on that nil).
func TestTraceCausalProjectionWakeupPathUserEntityHitsB1b(t *testing.T) {
	record := anchorB1PathRecord("path-two-entities", "oney.hmn.berlin-42591",
		"w-1 -> workerB-77 -> w-2 -> oney.hmn.berlin-42591 -> transit-9")
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{record}, []string{"42591", "77"})
	wantPath := []string{"w-1", "workerB-77", "w-2", "oney.hmn.berlin-42591"}
	if !got.WakeupPathUserElected || !reflect.DeepEqual(got.WakeupPath, wantPath) {
		t.Fatalf("B1-b: priority entity elects + truncates, got %v", got.WakeupPath)
	}
	if !reflect.DeepEqual(got.WakeupPathUserEntityHits, []int{1, 3}) {
		t.Fatalf("B1-b F2: hits must list every user-entity position on the FINAL path, got %v",
			got.WakeupPathUserEntityHits)
	}

	// No entities → nil hits (byte-stability of the no-signal lane).
	legacy := TraceCausalProjectionFromObservationRecords([]ObservationRecord{record})
	if legacy.WakeupPathUserEntityHits != nil {
		t.Fatalf("B1-b F2: the no-entity lane must carry nil hits, got %v", legacy.WakeupPathUserEntityHits)
	}
}
