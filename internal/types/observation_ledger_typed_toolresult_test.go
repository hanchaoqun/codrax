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

// TestCompileObservationLedgerKeepsSummaryReparseFallback pins the remaining
// legacy fallback: the same result WITHOUT typed rows still compiles via the
// summary re-parse, but only as low-confidence audit context.
func TestCompileObservationLedgerKeepsSummaryReparseFallback(t *testing.T) {
	result := typedTraceToolResult("unused")
	result.Observations = nil
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}})
	for _, record := range ledger.Records {
		if strings.Contains(record.ID, "#trace_query:root_cause_rank:1") {
			if record.Role != AnswerAggregateRoleAuditLedger ||
				record.GroundingPolicy != ClaimGroundingDisplayOnly ||
				record.Confidence != 0.2 ||
				!observationRecordHasNote(record, "legacy_summary_fallback=true; not_answer_grade=true") {
				t.Fatalf("legacy summary fallback should be audit-only: %+v", record)
			}
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

func TestCompileObservationLedgerPathDiscoveryUsesTypedCarrier(t *testing.T) {
	result := ToolResult{
		ToolName: "list_files",
		Success:  true,
		Summary: strings.Join([]string{
			"[list_files: evidence_origin=runtime_artifact artifact_id=trace count=99]",
			"rendered summary must not become a discovery observation",
		}, "\n"),
		RawRef: "blob://raw/list-files.txt",
		PathDiscovery: &ToolPathDiscovery{
			Kind:                    ToolPathDiscoveryKindListFiles,
			Path:                    "src",
			Recursive:               true,
			ResultCount:             2,
			CandidateFiles:          []string{"src/a.ts", "src/b.ts"},
			CandidateFilesTruncated: true,
		},
		Timestamp: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC),
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}})
	got := findTypedToolObservationRecord(t, ledger, "tool:0#path_discovery")
	if got.Origin != AnswerEvidenceOriginCurrentSource ||
		got.SourceRef.Kind != ObservationSourceCurrentSource ||
		got.SourceRef.Path != "src" ||
		got.ResultCount == nil ||
		*got.ResultCount != 2 ||
		len(got.SurfaceTerms) != 2 ||
		!observationRecordHasNote(got, "candidate_files_truncated=true; narrow path/include/type before using this as inventory guidance") {
		t.Fatalf("path discovery carrier was not projected from typed fields: %+v", got)
	}
	for _, record := range ledger.Records {
		if record.ID != "tool:0#path_discovery" {
			t.Fatalf("summary fallback leaked alongside typed path discovery carrier: %+v", ledger.Records)
		}
	}
}

func TestCompileObservationLedgerReadCoverageUsesTypedCarrier(t *testing.T) {
	result := ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: strings.Join([]string{
			"[read_file: evidence_origin=vcs_metadata count=1]",
			"rendered summary must not become a coverage observation",
		}, "\n"),
		RawRef: "blob://raw/read-file.txt",
		ReadCoverage: &ToolReadCoverage{
			Path:       "internal/types/context.go",
			LineStart:  10,
			LineEnd:    20,
			TotalLines: 200,
			RawRef:     "blob://raw/coverage.txt",
		},
		Timestamp: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC),
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}})
	got := findTypedToolObservationRecord(t, ledger, "tool:0#read_coverage")
	if got.Origin != AnswerEvidenceOriginCurrentSource ||
		got.SourceRef.Kind != ObservationSourceCurrentSource ||
		got.SourceRef.Path != "internal/types/context.go" ||
		got.SourceRef.RawRef != "blob://raw/coverage.txt" ||
		got.Span.LineStart != 10 ||
		got.Span.LineEnd != 20 ||
		got.ResultCount == nil ||
		*got.ResultCount != 11 ||
		len(got.SupportRefs) != 0 {
		t.Fatalf("read coverage carrier was not projected from typed fields: %+v", got)
	}
	for _, record := range ledger.Records {
		if record.ID != "tool:0#read_coverage" {
			t.Fatalf("summary fallback leaked alongside typed read coverage carrier: %+v", ledger.Records)
		}
	}
}

func TestCompileObservationLedgerSourceInventoryUsesToolCarrier(t *testing.T) {
	result := ToolResult{
		ToolName: "repo_map",
		Success:  true,
		Summary: strings.Join([]string{
			"[repo_map: evidence_origin=runtime_artifact artifact_id=trace count=99]",
			"rendered summary must not become a source-inventory observation",
		}, "\n"),
		RawRef: "blob://raw/source-inventory.txt",
		SourceInventory: &SourceInventoryObservation{
			Active:     true,
			Scopes:     []string{"internal/types"},
			Provenance: []string{"repo_map"},
			Sets: []SourceInventoryObservationSet{{
				Role:     AnswerCandidateRoleType,
				Complete: true,
				Members: []SourceInventoryObservationMember{{
					Name:          "AgentName",
					Key:           "AgentName",
					File:          "internal/types/enums.go",
					Line:          127,
					Role:          AnswerCandidateRoleType,
					SupportRef:    "internal/types/enums.go:127",
					CoverageState: SourceInventoryCoverageObserved,
				}},
			}},
		},
		Timestamp: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC),
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}})
	set := findTypedToolObservationRecord(t, ledger, "tool:0#source_inventory:set:0")
	member := findTypedToolObservationRecord(t, ledger, "tool:0#source_inventory:0:0")
	if set.SourceRef.ToolCallID != "repo_map[0]" ||
		set.SourceRef.RawRef != "blob://raw/source-inventory.txt" ||
		member.SourceRef.Path != "internal/types/enums.go" ||
		member.Span.LineStart != 127 ||
		len(member.SupportRefs) != 1 {
		t.Fatalf("source-inventory carrier was not projected from typed fields: set=%+v member=%+v", set, member)
	}
	for _, record := range ledger.Records {
		if strings.HasPrefix(record.ID, "tool:0#source_inventory") {
			continue
		}
		t.Fatalf("summary fallback leaked alongside typed source-inventory carrier: %+v", ledger.Records)
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

func observationRecordHasNote(record ObservationRecord, note string) bool {
	for _, existing := range record.RichNotes {
		if existing == note {
			return true
		}
	}
	return false
}
