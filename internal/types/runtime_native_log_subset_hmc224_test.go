package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func hmc224LogResult(query string, sourceIDs ...string) ToolResult {
	var records []ObservationRecord
	var sources []nativeLogSourceReceipt
	for _, sourceID := range sourceIDs {
		record := presentationLogRecord(sourceID, 1)
		record.ID = "log_query:" + query + "#" + record.Object
		record.SourceRef.QueryScopeID, record.SourceRef.PayloadRef = query, "/payload/"+query+".json"
		records = append(records, record)
		sources = append(sources, nativeLogSourceReceipt{ID: sourceID, Path: record.SourceRef.Path, Generation: "strong:sample", Complete: true})
	}
	coverage := records[0]
	coverage.ID, coverage.Object = "log_query:"+query+"#coverage", query
	coverage.ClaimKey, coverage.Predicate = "log_query_coverage", "log_query_coverage"
	coverage.SourceRef.Path, coverage.SourceRef.ArtifactID, coverage.SourceRef.ArtifactKind = "", "log-query:"+query, "log_query_result"
	coverage.SourceRef.TimeDomain = ""
	coverage.Value = fmt.Sprint(len(records))
	encoded, _ := json.Marshal(sources)
	coverage.RichNotes = []string{`query_filters={"limit":50}`, "source_generations=" + string(encoded), "source_reads_complete=true"}
	return ToolResult{ToolName: "log_query", Success: true, Observations: append(records, coverage)}
}

func hmc224AlterValue(r *ObservationRecord, field string, value any) {
	var fields map[string]any
	_ = json.Unmarshal([]byte(r.Value), &fields)
	fields[field] = value
	encoded, _ := json.Marshal(fields)
	r.Value = string(encoded)
}

func hmc224AlterSource(result *ToolResult, mutate func(*nativeLogSourceReceipt)) {
	coverage := &result.Observations[len(result.Observations)-1]
	sources, _ := nativeLogPresentationSources(*coverage)
	mutate(&sources[0])
	encoded, _ := json.Marshal(sources)
	coverage.RichNotes[1] = "source_generations=" + string(encoded)
}

func TestHMC224NativeLogSubsetExactBoundaries(t *testing.T) {
	for name, alter := range map[string]func(*ToolResult){
		"generation": func(r *ToolResult) {
			hmc224AlterSource(r, func(s *nativeLogSourceReceipt) { s.Generation = "strong:replaced" })
			r.Observations[0].RichNotes[0] = "source_generation=strong:replaced"
		},
		"same filename other path": func(r *ToolResult) {
			hmc224AlterSource(r, func(s *nativeLogSourceReceipt) { s.Path = "/other/source-a" })
			r.Observations[0].SourceRef.Path = "/other/source-a"
		},
		"source name": func(r *ToolResult) { hmc224AlterSource(r, func(s *nativeLogSourceReceipt) { s.Name = "renamed" }) },
		"bytes":       func(r *ToolResult) { hmc224AlterValue(&r.Observations[0], "byte_start", 1) },
		"line range": func(r *ToolResult) {
			hmc224AlterValue(&r.Observations[0], "last_line", 2)
			r.Observations[0].Span.LineEnd = 2
		},
		"record identity": func(r *ToolResult) {
			hmc224AlterValue(&r.Observations[0], "record_id", "other-record")
			r.Observations[0].Object = "other-record"
		},
		"null versus zero": func(r *ToolResult) { hmc224AlterValue(&r.Observations[0], "tid", 0) },
		"format":           func(r *ToolResult) { hmc224AlterValue(&r.Observations[0], "kind", "kmsg") },
		"text":             func(r *ToolResult) { hmc224AlterValue(&r.Observations[0], "message_preview", "different") },
		"clock": func(r *ToolResult) {
			hmc224AlterValue(&r.Observations[0], "clock_domain", "other-clock")
			r.Observations[0].SourceRef.TimeDomain = "other-clock"
		},
		"window": func(r *ToolResult) {
			for i := range r.Observations {
				r.Observations[i].SourceRef.QueryWindowKnown = true
				r.Observations[i].SourceRef.QueryWindowStartTs = 1
				r.Observations[i].SourceRef.QueryWindowEndTs = 2
			}
		},
		"omission": func(r *ToolResult) { r.Observations[len(r.Observations)-1].Value = "2" },
		"empty result": func(r *ToolResult) {
			r.Observations = r.Observations[1:]
			r.Observations[0].Value = "0"
		},
		"source reads not complete": func(r *ToolResult) {
			r.Observations[len(r.Observations)-1].RichNotes[2] = "source_reads_complete=false"
		},
		"duplicate native identity": func(r *ToolResult) {
			duplicate := r.Observations[0]
			duplicate.ID += "-duplicate"
			r.Observations = append([]ObservationRecord{duplicate}, r.Observations...)
			r.Observations[len(r.Observations)-1].Value = "2"
		},
		"duplicate filters": func(r *ToolResult) {
			r.Observations[len(r.Observations)-1].RichNotes[0] = `query_filters={"limit":50,"limit":50}`
		},
		"duplicate coverage note": func(r *ToolResult) {
			c := &r.Observations[len(r.Observations)-1]
			c.RichNotes = append(c.RichNotes, c.RichNotes[2])
		},
		"duplicate filter note": func(r *ToolResult) {
			c := &r.Observations[len(r.Observations)-1]
			c.RichNotes = append(c.RichNotes, c.RichNotes[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			large, small := hmc224LogResult("all", "source-a", "source-b"), hmc224LogResult("one", "source-a")
			alter(&small)
			tables := buildNativeLogPresentationTables(ObservationLedgerInput{ToolResults: []ToolResult{large, small}})
			if len(tables) != 2 || !tables[0].DefaultPresentation || !tables[1].DefaultPresentation {
				t.Fatalf("distinct, incomplete or ambiguous query was hidden: %+v", tables)
			}
		})
	}
	for _, filter := range []string{`"pid":0`, `"tid":1`, `"kinds":["hilog"]`, `"contains":"malformed"`, `"first_line":1`, `"last_line":2`, `"offset":1`, `"future_filter":true`} {
		t.Run(filter, func(t *testing.T) {
			large, small := hmc224LogResult("all", "source-a", "source-b"), hmc224LogResult("one", "source-a")
			small.Observations[1].RichNotes[0] = `query_filters={"limit":50,` + filter + "}"
			tables := buildNativeLogPresentationTables(ObservationLedgerInput{ToolResults: []ToolResult{large, small}})
			if len(tables) != 2 || !tables[1].DefaultPresentation {
				t.Fatal("independent filter scope was hidden")
			}
		})
	}
}

func TestHMC224NativeLogDisplayPopulationQualification(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(*ToolResult, *ObservationLedgerInput)
		want  bool
	}{
		{name: "complete", want: true},
		{name: "truncated text", alter: func(r *ToolResult, _ *ObservationLedgerInput) {
			hmc224AlterValue(&r.Observations[0], "text_fields_truncated", true)
		}},
		{name: "empty", alter: func(r *ToolResult, _ *ObservationLedgerInput) {
			r.Observations = r.Observations[1:]
			r.Observations[0].Value = "0"
		}},
		{name: "omitted", alter: func(r *ToolResult, _ *ObservationLedgerInput) { r.Observations[1].Value = "2" }},
		{name: "failed source", alter: func(r *ToolResult, _ *ObservationLedgerInput) {
			r.Observations[1].RichNotes[2] = "source_reads_complete=false"
		}},
		{name: "explicit unmapped window", alter: func(_ *ToolResult, input *ObservationLedgerInput) {
			start, end := 1.0, 2.0
			input.RequestModel = &RequestModel{RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1 to 2 seconds"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := hmc224LogResult("one", "source-a")
			input := ObservationLedgerInput{}
			if tc.alter != nil {
				tc.alter(&result, &input)
			}
			input.ToolResults = []ToolResult{result}
			tables := buildNativeLogPresentationTables(input)
			if len(tables) != 1 || tables[0].HasCompleteNativeDisplayPopulation() != tc.want {
				t.Fatalf("wrong display-only qualification: %+v", tables)
			}
			table := tables[0]
			if table.Clone().HasCompleteNativeDisplayPopulation() != tc.want || table.MemberSet != nil || table.coverageScope != nil {
				t.Fatal("clone lost display receipt or gained completion authority")
			}
			raw, _ := json.Marshal(table)
			var restored RuntimeMeasurementTable
			_ = json.Unmarshal(raw, &restored)
			if restored.HasCompleteNativeDisplayPopulation() {
				t.Fatal("JSON recreated display receipt")
			}
		})
	}
}

func TestHMC224NativeLogSubsetRetainsFailureAndConflictDisclosures(t *testing.T) {
	large, small := hmc224LogResult("all", "source-a", "source-b"), hmc224LogResult("one", "source-a")
	coverage := &large.Observations[len(large.Observations)-1]
	sources, _ := nativeLogPresentationSources(*coverage)
	sources = append(sources, nativeLogSourceReceipt{ID: "failed", Path: "/logs/failed", Error: "unavailable"})
	encoded, _ := json.Marshal(sources)
	coverage.RichNotes[1], coverage.RichNotes[2] = "source_generations="+string(encoded), "source_reads_complete=false"
	tables := buildNativeLogPresentationTables(ObservationLedgerInput{ToolResults: []ToolResult{large, small}})
	if len(tables) != 2 || !tables[1].DefaultPresentation || !strings.Contains(strings.Join(tables[0].Notes, " "), "unavailable") {
		t.Fatal("partially failed superset hid healthy source or failure disclosure")
	}
	large = hmc224LogResult("all", "source-a", "source-b")
	conflict := hmc224LogResult("all", "source-a", "source-b")
	hmc224AlterValue(&conflict.Observations[0], "message_preview", "conflicting payload")
	contract := BuildRuntimeMeasurementContract(ObservationLedgerInput{ToolResults: []ToolResult{large, small, conflict}})
	choices := contract.Choices()
	if len(choices) != 1 || !choices[0].DefaultPresentation || len(choices[0].Rows) != 1 {
		t.Fatal("conflicting superset suppressed the remaining valid default")
	}
	// The formatted table does not contain original byte offsets. Equal
	// visible text therefore cannot conceal a conflicting physical roster.
	conflict = hmc224LogResult("all", "source-a", "source-b")
	hmc224AlterValue(&conflict.Observations[0], "byte_start", 3)
	contract = BuildRuntimeMeasurementContract(ObservationLedgerInput{ToolResults: []ToolResult{large, small, conflict}})
	choices = contract.Choices()
	if len(choices) != 2 {
		t.Fatal("physical conflict removed an original table")
	}
	for _, choice := range choices {
		if !choice.DefaultPresentation || len(choice.Rows) == 2 && choice.HasCompleteNativeDisplayPopulation() {
			t.Fatal("equal text overruled conflicting hidden physical fields")
		}
	}
}
