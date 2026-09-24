package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func ioDetailCoverageTestRecord(emitted, total, omitted string) types.ObservationRecord {
	r := b1618IOCaptureRecord("coverage", "/capture/io.ftrace", "io_latency_coverage")
	r.Subject, r.Value, r.Unit = "block_request_pairs", total, "requests"
	r.SourceRef.Kind = types.ObservationSourceRuntimeArtifact
	r.SourceRef.QueryScopeID = "query:first"
	r.SourceRef.PayloadRef = "/results/first.json"
	r.SourceRef.QueryWindowKnown = true
	r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs = 10, 10.010
	r.SourceRef.QueryLineRangeKnown = true
	for _, kv := range [][2]string{{types.TraceNoteKeyIOCoverageEmitted, emitted}, {types.TraceNoteKeyTotal, total}, {types.TraceNoteKeyIOOverflowPairs, omitted}} {
		if kv[1] != "" {
			r.RichNotes = append(r.RichNotes, kv[0]+"="+kv[1])
		}
	}
	r.RichNotes = append(r.RichNotes, "io_latency_coverage_status=capacity_truncated", "io_latency_overflow_request_ms=0.000")
	return r
}

func TestIODetailCoveragePopulationAndDisplayStaySeparate(t *testing.T) {
	ctx := b1618IOCaptureContext()
	for _, counts := range [][3]int{{5, 23, 18}, {2, 2, 0}, {0, 0, 0}, {0, 41, 41}} {
		t.Run(fmt.Sprint(counts), func(t *testing.T) {
			r := ioDetailCoverageTestRecord(fmt.Sprint(counts[0]), fmt.Sprint(counts[1]), fmt.Sprint(counts[2]))
			ledger := types.ObservationLedger{Records: []types.ObservationRecord{r}}
			before, _ := json.Marshal(ledger)
			for name, got := range map[string]string{
				"bridge": renderAnswerDocIOMeasurementRelationBridge(ctx, ledger, ledger.Records),
				"row":    answerDocBoundedRuntimeFactAuthorityRow(r, &ctx.AnalysisIR.RequestModel, "en"),
			} {
				wants := []string{
					fmt.Sprintf("Accepted complete pairs=%d", counts[1]),
					fmt.Sprintf("query-selected details=%d", counts[0]),
					fmt.Sprintf("paired details beyond cap=%d", counts[2]),
					"not capture or scan completeness",
					"not failed/missing pairs",
				}
				if name == "bridge" {
					wants = append(wants, "not current prompt row counts", "not failed or missing-endpoint pairs", "independently of this detail cap")
				}
				for _, want := range wants {
					if !strings.Contains(got, want) {
						t.Errorf("%s lacks %q: %s", name, want, got)
					}
				}
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("display projection mutated the original ledger")
			}
		})
	}
}

func TestIODetailCoverageUnknownAndConflictingCountsDoNotCreatePopulation(t *testing.T) {
	ctx := b1618IOCaptureContext()
	for _, counts := range [][3]string{{"5", "", "18"}, {"", "23", "18"}, {"5", "23", ""}, {"5", "23", "17"}, {"-1", "23", "24"}, {"2.5", "23", "20.5"}} {
		t.Run(strings.Join(counts[:], "/"), func(t *testing.T) {
			r := ioDetailCoverageTestRecord(counts[0], counts[1], counts[2])
			ledger := types.ObservationLedger{Records: []types.ObservationRecord{r}}
			for name, got := range map[string]string{
				"bridge": renderAnswerDocIOMeasurementRelationBridge(ctx, ledger, ledger.Records),
				"row":    answerDocBoundedRuntimeFactAuthorityRow(r, &ctx.AnalysisIR.RequestModel, "en"),
			} {
				if !strings.Contains(got, "Request-detail counts are incomplete or inconsistent") || strings.Contains(got, "Accepted complete pairs=") {
					t.Errorf("%s fabricated a reconciled population: %s", name, got)
				}
				if !strings.Contains(got, "not capture or scan completeness") {
					t.Errorf("%s lost the independent scan/capture boundary: %s", name, got)
				}
			}
		})
	}
}

func TestIODetailCoverageDistinctQueryReceiptsRemainSeparate(t *testing.T) {
	ctx := b1618IOCaptureContext()
	for _, field := range []string{"query", "payload", "line_range", "line_presence", "query_window", "window_presence", "target", "target_thread", "target_scope", "generation", "unknown_result"} {
		t.Run(field, func(t *testing.T) {
			a := ioDetailCoverageTestRecord("5", "23", "18")
			b := a
			b.ID = "second-coverage"
			switch field {
			case "query":
				b.SourceRef.QueryScopeID = "query:second"
			case "payload":
				b.SourceRef.PayloadRef = "/results/later-generation.json"
			case "line_range":
				b.SourceRef.QueryLineStart, b.SourceRef.QueryLineEnd = 5, 20
			case "line_presence":
				b.SourceRef.QueryLineRangeKnown = false
			case "query_window":
				b.SourceRef.QueryWindowEndTs = 10.009
			case "window_presence":
				b.SourceRef.QueryWindowKnown = false
			case "target":
				b.SourceRef.QueryTargetPID = 9
			case "target_thread":
				b.SourceRef.QueryTargetThread = "other-9"
			case "target_scope":
				b.SourceRef.QueryTargetScope = "process"
			case "generation":
				a.ObservedAt, b.ObservedAt = "first", "later"
			case "unknown_result":
				a.SourceRef.QueryScopeID, b.SourceRef.QueryScopeID = "", ""
			}
			for _, records := range [][]types.ObservationRecord{{a, b}, {b, a}} {
				ledger := types.ObservationLedger{Records: records}
				got := renderAnswerDocIOMeasurementRelationBridge(ctx, ledger, records)
				if strings.Contains(got, "Global selected-window block-request coverage:") || strings.Contains(got, "Accepted complete pairs=") {
					t.Fatalf("different query receipts got one population merely because counts matched: %s", got)
				}
				for _, record := range records {
					row := answerDocBoundedRuntimeFactAuthorityRow(record, &ctx.AnalysisIR.RequestModel, "en")
					if !strings.Contains(row, "Accepted complete pairs=23") {
						t.Fatalf("original independently scoped coverage disappeared: %s", row)
					}
				}
			}
		})
	}
}

func TestPublishedIODetailCoverageRetainsQueryAndPopulationMeanings(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_io_request_latency_distribution", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", Stage: types.StageFinalize,
		AgentName: types.AgentFinalizer, Mutable: types.NewMutableState("Summarize accepted IO pairs and group measurements"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: "zh", PerfTrace: &types.PerfBundle{},
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency}},
		}},
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 1, "time_end": 14})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("public query failed: %v %+v", err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal(ledger)
	finalizer := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for name, content := range map[string]string{"explorer tool": result.Summary, "finalizer": finalizer} {
		for _, want := range []string{"Accepted complete pairs=17", "query-selected details=8", "paired details beyond cap=9", "not current prompt row counts", "not capture or scan completeness", "not failed or missing-endpoint pairs", "independently of this detail cap"} {
			if !strings.Contains(content, want) {
				t.Errorf("%s lost %q: %s", name, want, content)
			}
		}
	}
	semantic := types.ProjectObservationPromptRecords(ledger.Records, &ctx.AnalysisIR.RequestModel, nil,
		types.SemanticReviewObservationPromptProjectionOptions(types.ObservationPromptRecordLimit))
	found := 0
	for _, record := range ledger.Records {
		if record.Predicate != "io_latency_coverage" {
			continue
		}
		found++
		if record.SourceRef.Path != path || !record.SourceRef.QueryWindowKnown || record.SourceRef.QueryWindowStartTs != 1 || record.SourceRef.QueryWindowEndTs != 14 || record.SourceRef.QueryScopeID == "" {
			t.Fatalf("coverage query provenance missing: %+v", record.SourceRef)
		}
		var projected *types.ObservationPromptRecord
		for i := range semantic {
			if semantic[i].ID == record.ID {
				projected = &semantic[i]
				break
			}
		}
		if projected == nil {
			t.Errorf("semantic compact omitted coverage record %s", record.ID)
			continue
		}
		for _, want := range []string{"Accepted complete pairs=17", "query-selected details=8", "paired details beyond cap=9", "not failed/missing pairs"} {
			if !strings.Contains(projected.Summary, want) {
				t.Errorf("same-record semantic summary lost %q: %s", want, projected.Summary)
			}
		}
	}
	if found == 0 {
		t.Fatal("public query did not publish coverage")
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("display projection modified original ledger")
	}
}

func TestIODetailCoverageSameQueryDuplicateCanCoalesce(t *testing.T) {
	ctx := b1618IOCaptureContext()
	a := ioDetailCoverageTestRecord("5", "23", "18")
	b := a
	b.ID = "same-result-second-publication"
	records := []types.ObservationRecord{a, b}
	got := renderAnswerDocIOMeasurementRelationBridge(ctx, types.ObservationLedger{Records: records}, records)
	if strings.Count(got, "Accepted complete pairs=23") != 1 {
		t.Fatalf("one exact result's duplicate coverage must retain one honest count: %s", got)
	}
}

func TestIORuntimeFactQueryWindowIsNotObservedInterval(t *testing.T) {
	ctx := b1618IOCaptureContext()
	for _, predicate := range []string{"io_latency", "io_latency_coverage", "storage_latency_by_layer"} {
		t.Run(predicate, func(t *testing.T) {
			r := ioDetailCoverageTestRecord("5", "23", "18")
			r.Predicate = predicate
			r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs = 0, 20
			r.Span = types.ObservationSpan{StartTs: 12, EndTs: 12.046}
			got := answerDocBoundedRuntimeFactAuthorityRow(r, &ctx.AnalysisIR.RequestModel, "en")
			for _, want := range []string{"query_window=`0.000000..20.000000`", "query_lines=`unrestricted`", "observed_interval=`12.000000..12.046000`", "query_scope=`query:first`"} {
				if !strings.Contains(got, want) {
					t.Errorf("row lost separate query/occurrence coordinate %q: %s", want, got)
				}
			}
			// Actual query execution gives line bounds precedence; retain its
			// submitted time arguments as audit metadata, not a denominator.
			r.SourceRef.QueryLineStart, r.SourceRef.QueryLineEnd = 10, 50
			got = answerDocBoundedRuntimeFactAuthorityRow(r, &ctx.AnalysisIR.RequestModel, "en")
			for _, want := range []string{"query_window=`unknown`", "query_time_arguments=`0..20`", "line selection takes precedence", "query_lines=`10..50`", "observed_interval=`12.000000..12.046000`"} {
				if !strings.Contains(got, want) {
					t.Errorf("line-selected row lost %q: %s", want, got)
				}
			}
			r.SourceRef.QueryWindowKnown = false
			r.SourceRef.QueryLineRangeKnown = false
			got = answerDocBoundedRuntimeFactAuthorityRow(r, &ctx.AnalysisIR.RequestModel, "en")
			if !strings.Contains(got, "query_window=`unknown`") || strings.Contains(got, "query_window=`12.000000..12.046000`") || strings.Contains(got, "query_lines=`10..50`") {
				t.Errorf("unknown query bounds were inferred from stale fields or observed span: %s", got)
			}
		})
	}
}
