package types

// observation_ledger_rankdis_a1_test.go — RANKDIS-EXT A1+A3 read-side compat
// (§29.104.16/.16.1, 2026-07-16): the state_drilldown TEXT word is now
// drill_rank, but persisted artifacts and replayed old tool summaries still
// spell it rank= — the legacy text re-parse lane keeps a fail-open arm and
// both spellings mint the SAME record shape, on the DEDICATED state_rank
// note lane (A3: the borrowed causal `rank` key let the projection read a
// state ordinal as a board seat).
//
// MUTATION self-check: dropping the drill_rank parse reds the new-face arm;
// dropping the legacy fallback reds the old-face arm; minting back onto the
// causal `rank` key reds the note-lane pins.

import (
	"strings"
	"testing"
)

func rankdisA1DrilldownLedger(t *testing.T, line string) ObservationLedger {
	t.Helper()
	return CompileObservationLedger(ObservationLedgerInput{
		ToolResults: []ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Summary: strings.Join([]string{
				"[trace_query params: view=window_stats source=attached_trace path=/tmp/a.systrace origin=runtime_artifact artifact_id=attached_trace artifact_kind=trace payload_ref=/tmp/trace-query.json]",
				"## State drilldown",
				line,
			}, "\n"),
		}},
	})
}

func TestRankdisA1StateDrilldownParsesNewAndLegacyRankWords(t *testing.T) {
	const tail = "thread=com.app-42 state=S impact=21.000ms total=30.000ms source=top_sleep chain_required=true recursive=true window_proportion=0.7000 significant=true recommended_views=wakeup_chain,root_cause_rank lines=121-130 — com.app-42 long sleep requires wakeup-chain drilldown"
	newFace := rankdisA1DrilldownLedger(t, "- state_drilldown drill_rank=2 "+tail)
	oldFace := rankdisA1DrilldownLedger(t, "- state_drilldown rank=2 "+tail)

	// RANKDIS-EXT C8 (M23): the ID tail is the row POSITION — this fixture's
	// single line is position 1 even though its drill_rank is 2, pinning the
	// unified position semantics (the retired shape minted the rank digit
	// here while the typed lane minted position).
	fetch := func(ledger ObservationLedger, label string) ObservationRecord {
		for _, record := range ledger.Records {
			if record.ID == "tool:0#trace_query:state_drilldown:1" {
				return record
			}
		}
		t.Fatalf("%s: drilldown record at position 1 missing: %+v", label, ledger.Records)
		return ObservationRecord{}
	}
	newRecord := fetch(newFace, "new face")
	oldRecord := fetch(oldFace, "legacy face")

	for label, record := range map[string]ObservationRecord{"new face": newRecord, "legacy face": oldRecord} {
		if record.Subject != "com.app-42" || record.Object != "S" || record.Value != "21.000" {
			t.Fatalf("%s: drilldown identity fields must survive the word change: %+v", label, record)
		}
		// A3: the ordinal mints on the dedicated state_rank lane regardless
		// of the text spelling; the causal `rank` key never appears.
		if !observationLedgerTestContainsString(record.RichNotes, "state_rank=2") {
			t.Fatalf("%s: the ordinal note must ride the state_rank lane: %+v", label, record.RichNotes)
		}
		for _, note := range record.RichNotes {
			if strings.HasPrefix(strings.TrimSpace(note), "rank=") {
				t.Fatalf("%s: a drilldown record must not mint the causal rank key: %+v", label, record.RichNotes)
			}
		}
	}
	// Fail-open equivalence: both spellings produce identical note sets.
	if strings.Join(newRecord.RichNotes, "\n") != strings.Join(oldRecord.RichNotes, "\n") {
		t.Fatalf("new-face and legacy-face drilldown notes diverged:\nnew: %+v\nold: %+v", newRecord.RichNotes, oldRecord.RichNotes)
	}
}

func TestRankdisA1StateDrilldownFallbackSummaryWearsScopedWord(t *testing.T) {
	// A head-only line (no — summary tail) forces the fallback Summary mint;
	// the fallback wears the scoped word like every other current face.
	ledger := rankdisA1DrilldownLedger(t, "- state_drilldown drill_rank=1 thread=com.app-42 state=S impact=21.000ms total=30.000ms source=top_sleep lines=121-130")
	for _, record := range ledger.Records {
		if record.ID != "tool:0#trace_query:state_drilldown:1" {
			continue
		}
		if !strings.Contains(record.Summary, "state_drilldown drill_rank=1") {
			t.Fatalf("fallback summary must wear the scoped drill_rank word: %q", record.Summary)
		}
		if strings.Contains(record.Summary, "state_drilldown rank=") {
			t.Fatalf("fallback summary must not resurrect the bare rank= word: %q", record.Summary)
		}
		return
	}
	t.Fatalf("drilldown record missing: %+v", ledger.Records)
}
