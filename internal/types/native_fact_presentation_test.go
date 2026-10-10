package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func presentationLogRecord(source string, line int) ObservationRecord {
	id := fmt.Sprintf("%s:L%d-%d", source, line, line)
	value, _ := json.Marshal(map[string]any{
		"record_id": id, "source_id": source, "kind": "hilog", "status": "malformed",
		"first_line": line, "last_line": line, "byte_start": 0, "byte_end": 70,
		"pid": 27599, "tid": nil, "cpu": nil, "level": "E", "tag": "AssetLoad", "comm": "",
		"wall_timestamp_raw": "99-09 09:00:00.004", "boot_timestamp_raw": "", "boot_timestamp_ns": "",
		"clock_domain": "wall_year_and_timezone_unknown", "parse_error": "invalid_wall_timestamp",
		"message_preview": "malformed timestamp", "raw_text_preview": "99-09 09:00:00.004 27599 27612 E AssetLoad: malformed timestamp",
		"preview_only": true, "text_fields_truncated": false,
	})
	return ObservationRecord{ID: "log_query:query#" + id, Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "log_query", Role: AnswerAggregateRoleSupportingCoverage, GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane: ObservationProvenanceArtifactSpan, ClaimAuthority: ObservationClaimAuthorityDirectObservation,
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactID: source, ArtifactKind: "runtime_log", Path: "/logs/" + source,
			QueryScopeID: "query", PayloadRef: "/payload/query.json", TimeDomain: "wall_year_and_timezone_unknown", ClockAlignment: "unmapped"},
		Span: ObservationSpan{LineStart: line, LineEnd: line}, Subject: source, Object: id,
		Predicate: "log_record", ClaimKey: "log_record", Value: string(value), Summary: "Recorded malformed log entry",
		RichNotes: []string{"source_generation=strong:sample", "source_sha256=" + strings.Repeat("a", 64), "decoded_sha256=" + strings.Repeat("a", 64), "line_coordinates=decoded_original_physical_lines"}}
}

func TestNativeFactPresentationCompleteFieldsAndImmutableLedger(t *testing.T) {
	record := presentationLogRecord("source-a", 4)
	original, _ := json.Marshal(record)
	opts := DefaultObservationPromptProjectionOptions(1)
	opts.ValueMaxLen, opts.NoteLimit, opts.OriginSpecificSupportingNoteLimit = 96, 1, 3
	got := ProjectObservationPromptRecords([]ObservationRecord{record}, nil, nil, opts)
	if len(got) != 1 {
		t.Fatal(got)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(got[0].Value), &fields); err != nil {
		t.Fatalf("native fields were reduced to a broken JSON prefix: %s", got[0].Value)
	}
	for key, want := range map[string]any{"record_id": record.Object, "source_id": "source-a", "first_line": float64(4), "last_line": float64(4), "status": "malformed", "wall_timestamp_raw": "99-09 09:00:00.004", "pid": float64(27599), "tid": nil, "cpu": nil, "parse_error": "invalid_wall_timestamp", "clock_domain": "wall_year_and_timezone_unknown", "tag": "AssetLoad", "level": "E"} {
		if value, exists := fields[key]; !exists || !reflect.DeepEqual(value, want) {
			t.Errorf("native field %s: got %#v exists=%t want %#v", key, value, exists, want)
		}
	}
	if strings.Contains(strings.Join(got[0].Notes, " "), "sha256=") {
		t.Fatal("per-row hashes still crowd out native fields", got[0].Notes)
	}
	if !strings.Contains(strings.Join(got[0].Notes, " "), "source_generation=strong:sample") {
		t.Fatal("independent prompt consumer lost source generation", got[0].Notes)
	}
	after, _ := json.Marshal(record)
	if string(original) != string(after) {
		t.Fatal("presentation mutated accepted ledger")
	}
}

func TestNativeFactPresentationRejectsUnboundOrMalformedCarriers(t *testing.T) {
	for name, alter := range map[string]func(*ObservationRecord){
		"model producer": func(r *ObservationRecord) { r.Producer = "log_triage" },
		"no authority":   func(r *ObservationRecord) { r.ClaimAuthority = "" },
		"no grounding":   func(r *ObservationRecord) { r.GroundingPolicy = "" },
		"no payload":     func(r *ObservationRecord) { r.SourceRef.PayloadRef = "" },
		"no scope":       func(r *ObservationRecord) { r.SourceRef.QueryScopeID = "" },
		"wrong source":   func(r *ObservationRecord) { r.SourceRef.ArtifactID = "another" },
		"wrong line":     func(r *ObservationRecord) { r.Span.LineEnd++ },
		"wrong domain":   func(r *ObservationRecord) { r.SourceRef.TimeDomain = "trace" },
		"duplicate keys": func(r *ObservationRecord) {
			r.Value = strings.Replace(r.Value, `"pid":27599`, `"pid":27599,"pid":1`, 1)
		},
		"missing unknown identity": func(r *ObservationRecord) { r.Value = strings.Replace(r.Value, `"tid":null,`, "", 1) },
		"case folded field":        func(r *ObservationRecord) { r.Value = strings.Replace(r.Value, `"pid":`, `"PID":`, 1) },
		"unknown field":            func(r *ObservationRecord) { r.Value = strings.TrimSuffix(r.Value, "}") + `,"extra":1}` },
		"trailing data":            func(r *ObservationRecord) { r.Value += " {}" },
	} {
		t.Run(name, func(t *testing.T) {
			record := presentationLogRecord("source-a", 1)
			alter(&record)
			if got := NativeRuntimeFactPresentationKind(record); got != "" {
				t.Fatalf("carrier gained display authority: %s", got)
			}
		})
	}
}

func TestNativeFactPresentationExactBootTimeAndUnknownIdentity(t *testing.T) {
	record := presentationLogRecord("kernel", 1)
	var value map[string]any
	if err := json.Unmarshal([]byte(record.Value), &value); err != nil {
		t.Fatal(err)
	}
	value["kind"], value["status"], value["parse_error"] = "kmsg", "parsed", ""
	value["pid"], value["tid"], value["cpu"], value["comm"] = nil, nil, 0, "kernel-worker"
	value["wall_timestamp_raw"], value["boot_timestamp_raw"], value["boot_timestamp_ns"] = "", "9007199.254740993", "9007199254740993"
	value["clock_domain"], value["last_line"] = "source_boot_unmapped", 2
	record.SourceRef.TimeDomain, record.Span.LineEnd = "source_boot_unmapped", 2
	data, _ := json.Marshal(value)
	record.Value = string(data)
	projected := ProjectObservationPromptRecords([]ObservationRecord{record}, nil, nil, DefaultObservationPromptProjectionOptions(1))[0]
	var got map[string]any
	if err := json.Unmarshal([]byte(projected.Value), &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"boot_timestamp_ns": "9007199254740993", "boot_timestamp_raw": "9007199.254740993", "wall_timestamp_raw": nil, "pid": nil, "tid": nil, "cpu": float64(0), "comm": "kernel-worker", "first_line": float64(1), "last_line": float64(2)} {
		if !reflect.DeepEqual(got[key], want) {
			t.Errorf("%s: got %#v want %#v", key, got[key], want)
		}
	}
	if RuntimeObservationProducerIsDeterministicQuery(record.Producer) || (ObservationLedger{Records: []ObservationRecord{record}}).HasDeterministicRuntimeQueryObservation() {
		t.Fatal("display eligibility silently expanded trace causal/window authority")
	}
}

func TestNativeFactPresentationReceiptKindsAndScopeSeparation(t *testing.T) {
	record := presentationLogRecord("source-a", 1)
	if got := NativeRuntimeFactPresentationKind(record); got != "log_record" {
		t.Fatal(got)
	}
	coverage := record
	coverage.ClaimKey, coverage.Predicate, coverage.SourceRef.ArtifactKind = "log_query_coverage", "log_query_coverage", "log_query_result"
	if got := NativeRuntimeFactPresentationKind(coverage); got != "log_query_coverage" {
		t.Fatal(got)
	}
	trace := record
	trace.Producer = "trace_query:run2"
	if got := NativeRuntimeFactPresentationKind(trace); got != "trace_record" {
		t.Fatal(got)
	}
	for name, alter := range map[string]func(*ObservationRecord){
		"query":      func(r *ObservationRecord) { r.SourceRef.QueryScopeID = "query2" },
		"generation": func(r *ObservationRecord) { r.RichNotes = []string{"source_generation=other"} },
		"window": func(r *ObservationRecord) {
			r.SourceRef.QueryWindowKnown = true
			r.SourceRef.QueryWindowStartTs = 2
			r.SourceRef.QueryWindowEndTs = 3
		},
	} {
		t.Run(name, func(t *testing.T) {
			other := record
			alter(&other)
			if nativePresentationParent(record) == nativePresentationParent(other) {
				t.Fatal("independent scope merged")
			}
		})
	}
}

func TestNativeFactPresentationRebalanceKeepsOtherOriginsAndDoesNotMutate(t *testing.T) {
	var records []ObservationRecord
	for i := 1; i <= 8; i++ {
		records = append(records, presentationLogRecord("source-a", i))
	}
	for i := 1; i <= 3; i++ {
		records = append(records, presentationLogRecord("source-b", i))
	}
	current := ObservationRecord{ID: "source:anchor", Origin: AnswerEvidenceOriginCurrentSource, SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "reader.go"}}
	selected := append([]ObservationRecord{current}, records[:6]...)
	before, _ := json.Marshal(records)
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 8; i++ {
				got := rebalanceNativePresentationRecords(records, selected, nil, nil)
				if len(got) != 7 || got[0].ID != current.ID {
					t.Errorf("non-native survival slot changed: %+v", got)
				}
				counts := map[string]int{}
				for _, row := range got[1:] {
					counts[row.SourceRef.ArtifactID]++
				}
				if counts["source-a"] != 3 || counts["source-b"] != 3 {
					t.Errorf("source coverage changed: %v", counts)
				}
				ProjectObservationPromptRecords(records, nil, nil, DefaultObservationPromptProjectionOptions(6))
			}
		}()
	}
	wg.Wait()
	after, _ := json.Marshal(records)
	if string(before) != string(after) || selected[1].SourceRef.ArtifactID != "source-a" {
		t.Fatal("projection mutated accepted records or caller selection")
	}
}

func TestNativeFactPresentationSourceCoverageAheadOfTriage(t *testing.T) {
	var records []ObservationRecord
	for i := 1; i <= 8; i++ {
		records = append(records, presentationLogRecord("source-a", i))
	}
	for i := 1; i <= 3; i++ {
		records = append(records, presentationLogRecord("source-b", i))
	}
	for i := 0; i < 6; i++ {
		records = append(records, ObservationRecord{ID: fmt.Sprintf("log:observation:%d", i), Origin: AnswerEvidenceOriginRuntimeArtifact,
			Producer: "log_triage", Role: AnswerAggregateRolePrincipalAnswer, ProvenanceLane: ObservationProvenanceObservedErrorOccurrence,
			Summary: "model-authored event", RawExcerpt: "observed words"})
	}
	for _, input := range [][]ObservationRecord{records, append(append([]ObservationRecord(nil), records[8:]...), records[:8]...)} {
		got := ProjectObservationPromptRecords(input, nil, nil, DefaultObservationPromptProjectionOptions(6))
		counts := map[string]int{}
		for _, r := range got {
			if r.Producer != "log_query" {
				t.Fatalf("native facts displaced by older triage: %+v", got)
			}
			if strings.Contains(r.Source, "source-a") {
				counts["a"]++
			} else if strings.Contains(r.Source, "source-b") {
				counts["b"]++
			}
		}
		if counts["a"] != 3 || counts["b"] != 3 {
			t.Fatalf("first source consumed the shared presentation budget: %v", counts)
		}
	}
}
