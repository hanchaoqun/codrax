package types

import (
	"strings"
	"testing"
	"time"
)

func typedTraceToolResult(id string) ToolResult {
	return ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Summary: strings.Join([]string{
			"[trace_query params: view=root_cause_rank source=attached_trace path=/traces/app.systrace origin=runtime_artifact artifact_id=attached_trace artifact_kind=trace payload_ref=/blobs/p.json]",
			"# Trace Query: root_cause_rank",
			"",
			"## Root cause rank",
			"- rank=1 tier=primary type=binder_wait thread=app-20 impact=12.500ms score=0.910 confidence=0.88 lines=10-20 source=wakeup_chain — binder reply stalled the frame",
		}, "\n"),
		RawRef: "/blobs/raw.txt",
		Observations: []ObservationRecord{{
			ID:              id,
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: ClaimGroundingHard,
			ProvenanceLane:  ObservationProvenanceObservedDirectCause,
			SourceRef: ObservationSourceRef{
				Kind:         ObservationSourceRuntimeArtifact,
				Path:         "/traces/app.systrace",
				ArtifactID:   "attached_trace",
				ArtifactKind: "trace",
				PayloadRef:   "/blobs/p.json",
				RawRef:       "/blobs/raw.txt",
			},
			Span:      ObservationSpan{LineStart: 10, LineEnd: 20},
			ClaimKey:  "root_cause_primary",
			Subject:   "app-20",
			Predicate: "root_cause_primary",
			Object:    "binder_wait",
			Value:     "12.500",
			Unit:      "ms",
			Summary:   "binder reply stalled the frame",
		}},
		Timestamp: time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
	}
}

// TestCompileObservationLedgerPrefersTypedToolRowsOverSummaryReparse pins the
// A1 consumer contract: a trace_query ToolResult that carries typed rows is
// compiled from those rows and the line-by-line Summary re-parse is skipped
// for that result, so the same fact cannot appear once typed and once parsed.
func TestCompileObservationLedgerPrefersTypedToolRowsOverSummaryReparse(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		ToolResults: []ToolResult{typedTraceToolResult("trace_query:p.json#root_cause_rank:1")},
	})
	foundTyped := false
	for _, record := range ledger.Records {
		if record.ID == "trace_query:p.json#root_cause_rank:1" {
			foundTyped = true
			if record.Origin != AnswerEvidenceOriginRuntimeArtifact {
				t.Fatalf("typed row origin drifted: %q", record.Origin)
			}
		}
		if strings.Contains(record.ID, "#trace_query:root_cause_rank") {
			t.Fatalf("summary re-parse ran despite typed rows: %s", record.ID)
		}
	}
	if !foundTyped {
		t.Fatalf("typed row missing from ledger: %+v", ledger.Records)
	}
}

// TestCompileObservationLedgerKeepsSummaryReparseFallback pins the fallback:
// the same result WITHOUT typed rows still compiles via the summary re-parse.
func TestCompileObservationLedgerKeepsSummaryReparseFallback(t *testing.T) {
	result := typedTraceToolResult("unused")
	result.Observations = nil
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}})
	for _, record := range ledger.Records {
		if strings.Contains(record.ID, "#trace_query:root_cause_rank:1") {
			return
		}
	}
	t.Fatalf("summary re-parse fallback did not run: %+v", ledger.Records)
}

// TestCompileObservationLedgerTypedRowIDLevelDedup pins ID-level dedup:
// duplicate copies of the same typed-row-bearing result (e.g. dispatch buffer
// plus snapshot history) compile the row exactly once, by exact ID equality.
func TestCompileObservationLedgerTypedRowIDLevelDedup(t *testing.T) {
	first := typedTraceToolResult("trace_query:p.json#root_cause_rank:1")
	second := typedTraceToolResult("trace_query:p.json#root_cause_rank:1")
	// Different summaries so the input-side result merge cannot collapse them.
	second.Summary += "\ncaveat=duplicate window"
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{first, second}})
	count := 0
	for _, record := range ledger.Records {
		if record.ID == "trace_query:p.json#root_cause_rank:1" {
			count++
		}
		if strings.Contains(record.ID, "#trace_query:root_cause_rank") {
			t.Fatalf("summary re-parse ran for a typed-covered duplicate: %s", record.ID)
		}
	}
	if count != 1 {
		t.Fatalf("typed row compiled %d times, want exactly 1", count)
	}
}

// TestCompileObservationLedgerRejectsCurrentSourceTypedToolRows pins the
// origin red line: a producer-attached row claiming current-source origin is
// dropped — tool-published rows can never enter the citation lane.
func TestCompileObservationLedgerRejectsCurrentSourceTypedToolRows(t *testing.T) {
	result := typedTraceToolResult("trace_query:p.json#root_cause_rank:1")
	result.Observations = append(result.Observations, ObservationRecord{
		ID:        "trace_query:p.json#bogus_current_source:1",
		Origin:    AnswerEvidenceOriginCurrentSource,
		Subject:   "internal/agent/agent.go",
		Predicate: "defines",
		Summary:   "must never be accepted from a tool-published row",
	})
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}})
	for _, record := range ledger.Records {
		if record.ID == "trace_query:p.json#bogus_current_source:1" {
			t.Fatalf("current-source typed tool row leaked into the ledger")
		}
	}
}

func TestCompileObservationLedgerCommandMeasurementUsesTypedCarrier(t *testing.T) {
	result := ToolResult{
		ToolName: "exec_command",
		Success:  true,
		Summary: strings.Join([]string{
			"[exec_command: evidence_origin=vcs_metadata count=999]",
			"rendered summary must not become an observation origin",
		}, "\n"),
		RawRef: "blob://raw/command.txt",
		CommandMeasurement: &ToolCommandMeasurement{
			Kind:        ToolCommandMeasurementKindCount,
			Value:       7,
			Origin:      AnswerEvidenceOriginCommandMeasurement,
			ProofSource: "exec_command",
			Command:     "find internal -name '*.go' | wc -l",
		},
		Timestamp: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC),
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}})
	got := findTypedToolObservationRecord(t, ledger, "tool:0#command_measurement")
	if got.Origin != AnswerEvidenceOriginCommandMeasurement ||
		got.SourceRef.Kind != ObservationSourceCommand ||
		got.SourceRef.Command != "find internal -name '*.go' | wc -l" ||
		got.ResultCount == nil ||
		*got.ResultCount != 7 ||
		got.Summary == "rendered summary must not become an observation origin" {
		t.Fatalf("command carrier was not projected from typed fields: %+v", got)
	}
	for _, record := range ledger.Records {
		if record.ID != "tool:0#command_measurement" {
			t.Fatalf("summary fallback leaked alongside typed command carrier: %+v", ledger.Records)
		}
	}
}

func TestCompileObservationLedgerVCSHistoryUsesTypedCarrier(t *testing.T) {
	commits := []string{
		"1111111111111111111111111111111111111111",
		"2222222222222222222222222222222222222222",
	}
	result := ToolResult{
		ToolName: "git_log",
		Success:  true,
		Summary: strings.Join([]string{
			"[git_log: evidence_origin=runtime_artifact artifact_id=trace count=99]",
			"commit deadbeef rendered text must not define the ledger",
		}, "\n"),
		RawRef: "blob://raw/git-log.txt",
		VCSHistory: &ToolVCSHistory{
			Kind:     ToolVCSHistoryKindGitLog,
			Commits:  commits,
			Ref:      "HEAD",
			Pathspec: "internal/types",
		},
		Timestamp: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC),
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}})
	got := findTypedToolObservationRecord(t, ledger, "tool:0#vcs_metadata")
	if got.Origin != AnswerEvidenceOriginVCSMetadata ||
		got.SourceRef.Kind != ObservationSourceVCSMetadata ||
		got.SourceRef.Range != "ref=HEAD count=2" ||
		got.SourceRef.Pathspec != "internal/types" ||
		got.ResultCount == nil ||
		*got.ResultCount != 2 ||
		got.Value != commits[0] ||
		len(got.SurfaceTerms) != 2 {
		t.Fatalf("vcs carrier was not projected from typed fields: %+v", got)
	}
	for _, record := range ledger.Records {
		if record.ID != "tool:0#vcs_metadata" {
			t.Fatalf("summary fallback leaked alongside typed vcs carrier: %+v", ledger.Records)
		}
	}
}

func findTypedToolObservationRecord(t *testing.T, ledger ObservationLedger, id string) ObservationRecord {
	t.Helper()
	for _, record := range ledger.Records {
		if record.ID == id {
			return record
		}
	}
	t.Fatalf("record %s not found in ledger: %+v", id, ledger.Records)
	return ObservationRecord{}
}
