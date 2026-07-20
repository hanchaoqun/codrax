package types

// trace_note_keys_p3measure_test.go — P3MEASURE-1 registry red-line pin
// (§29.169, 2026-07-20): the five p3m_* silent-measurement keys are
// registered and their carrier IS display_only — advisory-only by
// mechanical enforcement (supply_pressure 分离先例). Promoting any of them
// to a parsed carrier (the prerequisite of any future gate or face) reddens
// THIS pin and the registry golden together; that red is the review surface
// the §29.169 stage-two ruling must pass through (词面须「见证下界」语义,
// 禁比值形, new user ruling required).

import "testing"

func TestP3MeasureKeysStayDisplayOnly(t *testing.T) {
	keys := []string{
		TraceNoteKeyP3MCounterfactualValidMS,
		TraceNoteKeyP3MCounterfactualInvalidMS,
		TraceNoteKeyP3MEdgeWitnessedMS,
		TraceNoteKeyP3MDisposition,
		TraceNoteKeyP3MCoverage,
	}
	for _, key := range keys {
		row, ok := TraceNoteKeyLookup(key)
		if !ok {
			t.Fatalf("p3m key %q must be registered", key)
		}
		if row.Carrier != TraceNoteCarrierDisplayOnly {
			t.Fatalf("advisory-only red line: %q carrier is %q — the silent measurement admits NO parsing consumer without a new user ruling (§29.169)",
				key, row.Carrier)
		}
		if row.Family != "causal_rank" {
			t.Fatalf("p3m key %q must ride the causal_rank family, got %q", key, row.Family)
		}
	}
}
