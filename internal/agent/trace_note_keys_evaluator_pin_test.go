package agent

// trace_note_keys_evaluator_pin_test.go — NKR evaluator-side consumer pins
// (F2). The evaluator's trace_query supplement lane consumes rich notes two
// ways: it PARSES perf_quality / perf_quality_caveats back off the wire, and
// it SELECTS notes for pass-through via a prefix table — and pass-through
// selection is consumption too: a drifted table entry means the supplement
// row silently loses that note (the soft-consumer failure mode; no gate, no
// test failure, just a vanished line).
//
// Both pins below carry an IN-TEST MUTATION drill: each one first proves the
// checker fires on the mutated shape it exists to catch, then asserts the
// real surface is clean. Key literals in THIS file are deliberate verbatim
// wire pins — do not replace them with the constants.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// traceNoteKeyPrefixTableViolations reports the entries of a supplement
// pass-through prefix table that are not "<registered key>=" shaped.
func traceNoteKeyPrefixTableViolations(prefixes []string) []string {
	var out []string
	for _, prefix := range prefixes {
		key, ok := strings.CutSuffix(prefix, "=")
		if !ok || !types.TraceNoteKeyRegistered(key) {
			out = append(out, prefix)
		}
	}
	return out
}

// TestTraceQueryObservationSupplementAllowedPrefixesRegistered pins the
// supplement pass-through table ⊆ registry. Table drift (a typo'd or renamed
// entry) silently drops the supplement note instead of failing — exactly the
// soft-consumer failure mode — so membership is checked structurally here.
func TestTraceQueryObservationSupplementAllowedPrefixesRegistered(t *testing.T) {
	// In-test mutation drill: a stale/typo'd entry MUST be flagged, or this
	// pin is checking nothing.
	mutated := append(append([]string(nil), traceQueryObservationSupplementAllowedNotePrefixes...), "perf_quality_caveatz=")
	if got := traceNoteKeyPrefixTableViolations(mutated); len(got) != 1 || got[0] != "perf_quality_caveatz=" {
		t.Fatalf("mutation drill failed: violations on typo'd table = %v, want exactly [perf_quality_caveatz=] — the ⊆-registry checker is broken", got)
	}
	if got := traceNoteKeyPrefixTableViolations([]string{"window"}); len(got) != 1 {
		t.Fatalf("mutation drill failed: entry without '=' suffix not flagged — the ⊆-registry checker is broken")
	}
	if violations := traceNoteKeyPrefixTableViolations(traceQueryObservationSupplementAllowedNotePrefixes); len(violations) != 0 {
		t.Errorf("traceQueryObservationSupplementAllowedNotePrefixes carries entries outside the trace_note_keys.go registry: %v — register the key first (change protocol), never let a selection entry drift", violations)
	}
	// SG 批: the per-type priority tables are selection surfaces too (their
	// pass picks notes directly), so they carry the same ⊆-registry pin.
	for name, table := range map[string][]string{
		"blocking": traceQueryObservationSupplementBlockingPriorityPrefixes,
		"binder":   traceQueryObservationSupplementBinderPriorityPrefixes,
		"periodic": traceQueryObservationSupplementPeriodicPriorityPrefixes,
	} {
		if violations := traceNoteKeyPrefixTableViolations(table); len(violations) != 0 {
			t.Errorf("supplement %s priority table carries entries outside the trace_note_keys.go registry: %v — register the key first (change protocol), never let a selection entry drift", name, violations)
		}
	}
	// SG 批: the blocking-family trio the mandate added to the ALLOWED table
	// must stay there — table-level exclusion was the harder half of the
	// §10-A3/C4/Q4-K 修3 common cause (no ordering can admit an excluded key).
	allowed := map[string]bool{}
	for _, prefix := range traceQueryObservationSupplementAllowedNotePrefixes {
		allowed[prefix] = true
	}
	for _, want := range []string{
		types.TraceNoteKeyBlockingKind + "=",
		types.TraceNoteKeyHolderSite + "=",
		types.TraceNoteKeyWaiters + "=",
	} {
		if !allowed[want] {
			t.Errorf("allowed-prefix table lost the blocking-family entry %q (SG 批 Q4-K 修3)", want)
		}
	}
	// BLK-2 P1 指代翻转修复: the holder-subject folded rank row's twin state
	// breakdown is ported under subject_state_*, so both the allowed table and
	// the blocking priority table must carry the subject_state pair alongside
	// the peer_state pair — dropping either silently regresses the
	// holder-subject audit window back to the pre-BLK-2 omission behavior.
	blockingPriority := map[string]bool{}
	for _, prefix := range traceQueryObservationSupplementBlockingPriorityPrefixes {
		blockingPriority[prefix] = true
	}
	for _, want := range []string{"subject_state_dominant=", "subject_state_sleep="} {
		if !allowed[want] {
			t.Errorf("allowed-prefix table lost the holder-subject entry %q (BLK-2 P1)", want)
		}
		if !blockingPriority[want] {
			t.Errorf("blocking priority table lost the holder-subject entry %q (BLK-2 P1) — must coexist with the peer_state pair, not replace it", want)
		}
	}
	// The peer_state pair must SURVIVE in the blocking priority table: the same
	// table still serves waiter-subject critical_blocking rows.
	for _, want := range []string{"peer_state_dominant=", "peer_state_sleep="} {
		if !blockingPriority[want] {
			t.Errorf("blocking priority table lost the waiter-subject entry %q (BLK-2 must ADD subject_state, not replace peer_state)", want)
		}
	}
}

// TestRuntimeTracePerfQualityNoteTracksRegistry pins the evaluator's
// perf-quality parse lane to the registry constants: notes BUILT FROM the
// registry keys must round-trip, so a registry-protocol rename of
// perf_quality / perf_quality_caveats that leaves any evaluator spot on the
// stale literal fails here (the stale spot would no longer parse the renamed
// wire note this test constructs).
func TestRuntimeTracePerfQualityNoteTracksRegistry(t *testing.T) {
	// In-test mutation drill: the pre-rename/typo wire shape must NOT parse —
	// exact-prefix parsing is what makes drift silent on the wire, and what
	// makes the constant-built round-trip below a real tripwire.
	if key, _, ok := runtimeTracePerfQualityNote("perf_quality_caveatz=cpu_unknown"); ok {
		t.Fatalf("mutation drill failed: typo'd note parsed as %q — exact-prefix contract broken, this pin can no longer catch renames", key)
	}
	for _, want := range []string{types.TraceNoteKeyPerfQuality, types.TraceNoteKeyPerfQualityCaveats} {
		key, value, ok := runtimeTracePerfQualityNote(want + "=cpu_unknown")
		if !ok || key != want || value != "cpu_unknown" {
			t.Errorf("runtimeTracePerfQualityNote(%q+\"=cpu_unknown\") = (%q, %q, %v), want (%q, \"cpu_unknown\", true) — an evaluator spot drifted off the registry constant", want, key, value, ok, want)
		}
	}
	// Wire-format double-write: the registry constants themselves stay pinned
	// to the verbatim wire keys the producer renders.
	if types.TraceNoteKeyPerfQuality != "perf_quality" || types.TraceNoteKeyPerfQualityCaveats != "perf_quality_caveats" {
		t.Errorf("perf-quality registry constants drifted from the wire literals: %q / %q — walk the trace_note_keys.go change protocol (golden + producer + consumers in one commit)", types.TraceNoteKeyPerfQuality, types.TraceNoteKeyPerfQualityCaveats)
	}
}
