package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func ioDetailCoverageRecord(emitted, total, omitted string) ObservationRecord {
	return ObservationRecord{
		Predicate: "io_latency_coverage", Subject: "block_request_pairs", Unit: "requests", Value: total,
		RichNotes: []string{TraceNoteKeyIOCoverageEmitted + "=" + emitted, TraceNoteKeyTotal + "=" + total, TraceNoteKeyIOOverflowPairs + "=" + omitted},
	}
}

func TestTraceIODetailCoverageExactCountsAndZeroPresence(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, counts := range [][3]int{{5, 23, 18}, {2, 2, 0}, {0, 0, 0}, {0, 41, 41}, {1, maxInt, maxInt - 1}} {
		t.Run(fmt.Sprint(counts), func(t *testing.T) {
			r := ioDetailCoverageRecord(fmt.Sprint(counts[0]), fmt.Sprint(counts[1]), fmt.Sprint(counts[2]))
			before, _ := json.Marshal(r)
			got := TraceIODetailCoverageFromObservation(r).PromptMeaning()
			for _, want := range []string{fmt.Sprintf("Accepted complete pairs=%d", counts[1]), fmt.Sprintf("query-selected details=%d", counts[0]), fmt.Sprintf("paired details beyond cap=%d", counts[2]), "not current prompt row counts", "not capture or scan completeness", "not failed or missing-endpoint pairs", "independently of this detail cap", "absent distributions remain unknown", "neither pair count nor request residence proves target blocking or a root cause"} {
				if !strings.Contains(got, want) {
					t.Errorf("lost %q: %s", want, got)
				}
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) {
				t.Fatal("display helper changed source observation")
			}
		})
	}
}

func TestTraceIODetailCoverageDoesNotInferMissingOrConflictingCounters(t *testing.T) {
	for _, field := range []string{"emitted", "total", "overflow", "bad_balance", "negative", "decimal", "integer_overflow", "duplicate_total", "duplicate_emitted", "duplicate_overflow", "value", "result_count", "other_predicate", "other_subject", "other_unit"} {
		t.Run(field, func(t *testing.T) {
			r := ioDetailCoverageRecord("5", "23", "18")
			switch field {
			case "emitted":
				r.RichNotes = r.RichNotes[1:]
			case "total":
				r.RichNotes = []string{r.RichNotes[0], r.RichNotes[2]}
			case "overflow":
				r.RichNotes = r.RichNotes[:2]
			case "bad_balance":
				r.RichNotes[2] = TraceNoteKeyIOOverflowPairs + "=17"
			case "negative":
				r.RichNotes[0] = TraceNoteKeyIOCoverageEmitted + "=-1"
			case "decimal":
				r.RichNotes[0] = TraceNoteKeyIOCoverageEmitted + "=5.0"
			case "integer_overflow":
				r.RichNotes[1] = TraceNoteKeyTotal + "=999999999999999999999999999"
			case "duplicate_total":
				r.RichNotes = append(r.RichNotes, TraceNoteKeyTotal+"=24")
			case "duplicate_emitted":
				r.RichNotes = append(r.RichNotes, TraceNoteKeyIOCoverageEmitted+"=6")
			case "duplicate_overflow":
				r.RichNotes = append(r.RichNotes, TraceNoteKeyIOOverflowPairs+"=17")
			case "value":
				r.Value = "24"
			case "result_count":
				n := 24
				r.ResultCount = &n
			case "other_predicate":
				r.Predicate = "storage_latency_by_layer"
			case "other_subject":
				r.Subject = "issuer-41"
			case "other_unit":
				r.Unit = "ms"
			}
			r.Object, r.Summary = "complete", "complete capture; emitted=5 total=23 overflow=18"
			got := TraceIODetailCoverageFromObservation(r).PromptMeaning()
			if !strings.Contains(got, "Request-detail counts are incomplete or inconsistent") || strings.Contains(got, "Accepted complete pairs=") {
				t.Fatalf("invented a population from contradictory/absent counters: %s", got)
			}
		})
	}
}
