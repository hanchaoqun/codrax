package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Independent counterexamples use the public native producer and stop at the
// real adapter boundary. Equal values are not a source/population identity.
func hmc267ReviewContext(t *testing.T) (*types.AgentContext, string) {
	t.Helper()
	dir := t.TempDir()
	start, end := 1., 2.
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, PerfTrace: &types.PerfBundle{},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到2秒"}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("比较采样原始记录和调用路径"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx.Mutable.SetRequestModel(rm)
	data, err := os.ReadFile("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "original.perftrace")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return ctx, path
}

func hmc267ReviewQuery(t *testing.T, ctx *types.AgentContext, path string, changes map[string]any) types.ToolResult {
	t.Helper()
	args := map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "pid": 11, "time_start": 1, "time_end": 2}
	for key, value := range changes {
		args[key] = value
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !result.Success {
		t.Fatalf("public native query must succeed before presentation: %v %+v", err, result)
	}
	return result
}

func hmc267ReviewInstall(ctx *types.AgentContext, results ...types.ToolResult) {
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
}

func TestHMC267ReviewIndependentNativePopulationsStaySeparate(t *testing.T) {
	for _, variation := range []string{"same_bytes_other_source", "different_window", "different_denominator", "same_name_other_thread", "different_depth"} {
		t.Run(variation, func(t *testing.T) {
			ctx, path := hmc267ReviewContext(t)
			otherPath, changes := path, map[string]any{}
			if variation == "same_name_other_thread" {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				otherThread := strings.NewReplacer("tid=11 ", "tid=12 ", "document-worker-11", "document-worker-12").Replace(string(data))
				if otherThread == string(data) {
					t.Fatal("independent thread fixture needs a genuinely different native tid")
				}
				var interleaved strings.Builder
				originalLines, otherLines := strings.Split(string(data), "\n"), strings.Split(otherThread, "\n")
				for i, line := range originalLines {
					if line == "" {
						continue
					}
					interleaved.WriteString(line + "\n")
					if !strings.HasPrefix(line, "#") {
						interleaved.WriteString(otherLines[i] + "\n")
					}
				}
				if err := os.WriteFile(path, []byte(interleaved.String()), 0600); err != nil {
					t.Fatal(err)
				}
				changes["pid"] = 12
			}
			switch variation {
			case "same_bytes_other_source":
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				otherPath = filepath.Join(ctx.RepoRoot, "independent.perftrace")
				if err := os.WriteFile(otherPath, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "different_window":
				changes["time_end"] = 1.95 // Same samples, distinct requested window.
			case "different_denominator":
				changes["time_end"] = 1.25 // Same cycles cohort has weight 30, not 35.
			case "different_depth":
				changes["sampling_depth"] = 2
			}
			first := hmc267ReviewQuery(t, ctx, path, nil)
			second := hmc267ReviewQuery(t, ctx, otherPath, changes)
			if variation == "same_name_other_thread" {
				for _, result := range []types.ToolResult{first, second} {
					tree, ok := tool.DecodeTracePerfSamplingTree(result.Observations[0])
					if !ok || tree.SampleCount != 5 || tree.CohortCount != 3 {
						t.Fatalf("independent same-name thread must retain its own 5 records/3 cohorts: %+v", tree)
					}
				}
			}
			hmc267ReviewInstall(ctx, first, second)
			before := hmc266NegativeSnapshot(t, ctx)
			prompt := hmc266NegativePrompt(t, ctx, "finalize")
			if got := strings.Count(prompt, "Sampling prefix tree:"); got != 2 {
				t.Fatalf("distinct %s was merged or dropped: native display count=%d", variation, got)
			}
			for _, value := range []string{"total_weight=35", "total_weight=7", "instructions"} {
				if !strings.Contains(prompt, value) {
					t.Errorf("independent cohort facts lost %q", value)
				}
			}
			if variation == "different_denominator" && !strings.Contains(prompt, "total_weight=30") {
				t.Fatal("the narrower cycles cohort lost its independent denominator 30")
			}
			if before != hmc266NegativeSnapshot(t, ctx) {
				t.Fatal("presentation changed original receipts, denominators, or causal authority")
			}
		})
	}
}

func TestHMC267ReviewDisplaySourceKeyRequiresCurrentPrivateOrigin(t *testing.T) {
	for _, variation := range []string{"json_restoration", "memo", "failed_result", "foreign_record", "same_bytes_new_inode", "new_turn"} {
		t.Run(variation, func(t *testing.T) {
			ctx, path := hmc267ReviewContext(t)
			result := hmc267ReviewQuery(t, ctx, path, nil)
			hmc267ReviewInstall(ctx, result)
			input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
			record := result.Observations[0]
			key := types.RuntimeObservationPresentationSourceKey(input, result, record)
			if key == "" {
				t.Fatal("positive actual native call must prove its held source for display equivalence")
			}
			switch variation {
			case "json_restoration":
				raw, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				result = types.ToolResult{}
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatal(err)
				}
				record = result.Observations[0]
			case "memo":
				result.ReusedFromRunMemo = true
			case "failed_result":
				result.Success = false
			case "foreign_record":
				record.SourceRef.Path += ".other"
			case "same_bytes_new_inode":
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				replacement := filepath.Join(ctx.RepoRoot, "replacement.perftrace")
				if err := os.WriteFile(replacement, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			case "new_turn":
				ctx.Mutable.ResetTurnAArtifacts()
			}
			if got := types.RuntimeObservationPresentationSourceKey(input, result, record); got != "" {
				t.Fatal("missing/stale private source became an equivalence witness")
			}
		})
	}
}

func TestHMC267ReviewFilteredPopulationDoesNotBorrowFullTree(t *testing.T) {
	ctx, path := hmc267ReviewContext(t)
	full := hmc267ReviewQuery(t, ctx, path, nil)
	filtered := hmc267ReviewQuery(t, ctx, path, map[string]any{"view": "perf_stats", "pattern": "/app/B.so"})
	hmc267ReviewInstall(ctx, full, filtered)
	ledger := answerDocObservationLedger(ctx)
	publication := &answerDocNativePresentationReceipt{}
	renderAnswerDocPerfSamplingTrees(ctx, ledger, publication)
	found := false
	for _, record := range filtered.Observations {
		if _, valid, _ := tool.TraceQueryFilteredPerfMemberSelection(record); !valid {
			continue
		}
		found = true
		if publication.queries[nativePresentationQueryKey(record)] {
			t.Fatal("unfiltered tree certified a distinct filtered member population before those members were displayed")
		}
	}
	if !found {
		t.Fatal("fixture must have a real independently filtered native member result")
	}
	before := hmc266NegativeSnapshot(t, ctx)
	prompt := hmc266NegativePrompt(t, ctx, "finalize")
	if !strings.Contains(prompt, "/app/B.so") || !strings.Contains(prompt, "total_weight=35") {
		t.Fatal("independent filtered member or full population disappeared")
	}
	if before != hmc266NegativeSnapshot(t, ctx) {
		t.Fatal("display changed filter/source/raw facts")
	}
}

func TestHMC267ReviewNativeMirrorKeepsEveryOriginalSummary(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	path := ctx.AttachedHitraceSource
	result := hmc267ReviewQuery(t, ctx, path, map[string]any{"view": "perf_stats"})
	hmc267ReviewInstall(ctx, result)
	ledger := answerDocObservationLedger(ctx)
	var originals []types.ObservationRecord
	bare, structured := 0, 0
	for _, record := range ledger.Records {
		if record.Predicate != "perf_sample_top_symbol" {
			continue
		}
		originals = append(originals, record)
		if record.Value == "" && len(record.RichNotes) == 0 {
			bare++
		} else {
			structured++
		}
	}
	if bare == 0 || structured == 0 {
		t.Fatal("public producer must exercise both actual summary-only and structured native records")
	}
	before := hmc266NegativeSnapshot(t, ctx)
	publication := &answerDocNativePresentationReceipt{}
	renderAnswerDocPerfSamplingTrees(ctx, ledger, publication)
	flat := renderAnswerDocScopedPerfMemberFacts(ctx, ledger, ledger, publication)
	noteDirectory := map[string]string{}
	resolvedRows := 0
	for _, line := range strings.Split(flat, "\n") {
		if strings.HasPrefix(line, "native_note_ref=") {
			parts := strings.SplitN(strings.TrimPrefix(line, "native_note_ref="), " value=", 2)
			if len(parts) != 2 {
				t.Fatal("native note catalog lost an atomic record")
			}
			id, err := strconv.Unquote(parts[0])
			if err != nil {
				t.Fatal(err)
			}
			var value string
			if err := json.Unmarshal([]byte(parts[1]), &value); err != nil {
				t.Fatal("native note catalog clipped its value: ", err)
			}
			if _, exists := noteDirectory[id]; exists {
				t.Fatal("native note directory reused an ID for another declaration")
			}
			noteDirectory[id] = value
		}
		var row map[string]json.RawMessage
		if json.Unmarshal([]byte(line), &row) == nil && row["recorded_symbol"] != nil && string(row["value"]) == `""` {
			t.Fatal("summary-only mirror was published as an empty structured flat value")
		}
		if row["recorded_symbol"] != nil && row["native_fields_refs"] != nil {
			var summary string
			var refs, resolved []string
			if err := json.Unmarshal(row["native_summary"], &summary); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(row["native_fields_refs"], &refs); err != nil {
				t.Fatal(err)
			}
			for _, ref := range refs {
				value, found := noteDirectory[ref]
				if !found {
					t.Fatalf("row references a note that was not actually published before it: %s", ref)
				}
				resolved = append(resolved, value)
			}
			found := false
			for _, original := range originals {
				if original.Summary == summary && reflect.DeepEqual(original.RichNotes, resolved) {
					found = true
					break
				}
			}
			if !found {
				t.Fatal("literal note reuse changed or borrowed another member's native fields")
			}
			resolvedRows++
		}
	}
	if resolvedRows == 0 || len(noteDirectory) == 0 || len(flat) > 32<<10 {
		t.Fatalf("expected actual atomic native notes within the existing flat budget: rows=%d notes=%d bytes=%d", resolvedRows, len(noteDirectory), len(flat))
	}
	for _, record := range originals {
		if publication.samplingMirrorReferences(ledger)[record.ID] != "" {
			encoded, err := json.Marshal(record.Summary)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(flat, `"native_summary":`+string(encoded)) {
				t.Fatalf("another member's flat fields replaced this record without its own complete summary: %s", record.ID)
			}
		}
	}
	t.Logf("native_summary_records=%d summary_only=%d structured=%d flat_bytes=%d", len(originals), bare, structured, len(flat))
	prompt := hmc266NegativePrompt(t, ctx, "finalize")
	for _, record := range originals {
		encoded, err := json.Marshal(record.Summary)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(prompt, record.Summary) && !strings.Contains(prompt, string(encoded)) {
			t.Errorf("actual finalizer lost native original summary: %s", record.ID)
		}
	}
	if !strings.Contains(prompt, "sample_weight=15 denominator=35 value_basis=recorded_sample_fields_not_prefix_node") {
		t.Fatal("generic original's independent flat weight/denominator was replaced by a tree or empty member")
	}
	if before != hmc266NegativeSnapshot(t, ctx) {
		t.Fatal("display changed the preserved summary-only original")
	}
}

func TestHMC267ReviewTableReceiptDoesNotEraseUndisplayedRecordSummary(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	result := hmc267ReviewQuery(t, ctx, ctx.AttachedHitraceSource, map[string]any{"view": "perf_stats"})
	hmc267ReviewInstall(ctx, result)
	before := hmc266NegativeSnapshot(t, ctx)
	ledger := answerDocObservationLedger(ctx)
	publication := &answerDocNativePresentationReceipt{}
	text := renderAnswerDocRuntimeMeasurementChoices(ctx, publication)
	if !strings.Contains(text, "Verified measurement tables") {
		t.Fatal("public producer must provide real native table choices")
	}
	mirrors := publication.samplingMirrorReferences(ledger)
	completeRecords := 0
	for _, record := range ledger.Records {
		if record.Predicate != types.RuntimeSamplingMeasurementPredicate || !publication.records[traceEventPublicationRecordKey(record)] {
			continue
		}
		completeRecords++
		if !strings.Contains(text, record.Summary) && mirrors[record.ID] != "" {
			t.Fatal("complete table rows falsely certified publication of an undisplayed raw record summary")
		}
	}
	if completeRecords == 0 {
		t.Fatal("positive prerequisite: at least one native sampling fact table must actually be completely displayed")
	}
	if before != hmc266NegativeSnapshot(t, ctx) {
		t.Fatal("table presentation altered native originals or causal authority")
	}
}

func TestHMC267ReviewUnqualifiedTreeCannotAcquireDisplayReceipt(t *testing.T) {
	for _, variation := range []string{"missing_source", "foreign_source", "model_origin", "model_producer"} {
		t.Run(variation, func(t *testing.T) {
			ctx, path := hmc267ReviewContext(t)
			result := hmc267ReviewQuery(t, ctx, path, nil)
			if len(result.Observations) != 1 {
				t.Fatalf("expected single native tree, got %d", len(result.Observations))
			}
			record := result.Observations[0]
			switch variation {
			case "missing_source":
				record.SourceRef.Path = ""
			case "foreign_source":
				record.SourceRef.Path += ".unrelated"
			case "model_origin":
				record.Origin = types.AnswerEvidenceOriginUnknown
			case "model_producer":
				record.Producer = "model"
			}
			ledger := types.ObservationLedger{Records: []types.ObservationRecord{record}}
			before, _ := json.Marshal(ledger)
			publication := &answerDocNativePresentationReceipt{}
			if text := renderAnswerDocPerfSamplingTrees(ctx, ledger, publication); strings.Contains(text, "Sampling prefix tree:") {
				t.Fatal("unqualified model/missing source borrowed native display identity")
			}
			if publication.queries[nativePresentationQueryKey(record)] {
				t.Fatal("unqualified record acquired a complete native presentation receipt")
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("display repaired/rewrote an unqualified source")
			}
		})
	}
}

func TestHMC267ReviewOmittedTreeNotCompleteAndExternalSourcesSurvive(t *testing.T) {
	ctx, path := hmc267ReviewContext(t)
	var results []types.ToolResult
	for i := 0; i < 5; i++ {
		results = append(results, hmc267ReviewQuery(t, ctx, path, map[string]any{"time_start": 1. + float64(i)/1000}))
	}
	mcp := []types.MCPResponse{{ServerName: "fixture", Method: "tools/call:status", Success: true, ResourceURI: "mcp://hmc267/status", MIMEType: "application/vnd.codrax.observation+json",
		Observations: []types.MCPTypedObservation{{Summary: "HMC267_remote_actual_status", ResourceURI: "mcp://hmc267/status", LineStart: 12, LineEnd: 12, Selector: "jobs=zero"}}}}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, MCPResponses: mcp})
	ctx.MCPResponses = mcp
	ctx.LogTriage = &types.LogBundle{Observations: []types.LogObservation{{Kind: types.LogObservationRetryCycle, Subject: "HMC267_log_original", Summary: "HMC267_logged_actual_status", Evidence: "pending_jobs=0", LineStart: 12, LineEnd: 12}}}
	ctx.Mutable.SetLogTriage(ctx.LogTriage)
	ledger := answerDocObservationLedger(ctx)
	publication := &answerDocNativePresentationReceipt{}
	text := renderAnswerDocPerfSamplingTrees(ctx, ledger, publication)
	if !strings.Contains(text, "另省略采样树查询=1") || strings.Count(text, "Sampling prefix tree:") != 4 {
		t.Fatalf("query-budget omission needs truthful receipt, got %s", text)
	}
	for _, record := range results[4].Observations {
		if publication.queries[nativePresentationQueryKey(record)] || publication.samplingSelections[nativePresentationQueryKey(record)] {
			t.Fatal("a budget-omitted native query acquired full-display authority")
		}
	}
	before := hmc266NegativeSnapshot(t, ctx)
	prompt := hmc266NegativePrompt(t, ctx, "finalize")
	for _, value := range []string{"HMC267_remote_actual_status", "mcp://hmc267/status", "HMC267_logged_actual_status", "pending_jobs=0"} {
		if !strings.Contains(prompt, value) {
			t.Errorf("shared native display budget erased independent log/MCP original %q", value)
		}
	}
	if before != hmc266NegativeSnapshot(t, ctx) {
		t.Fatal("budget changed underlying source snapshots or causal authority")
	}
}

// This is a bounded diagnostic measurement, not a timing gate or an LLM run.
// Native queries are completed before the timed dispatch-local projection.
func TestHMC267ReviewPresentationAssemblyCost(t *testing.T) {
	for _, queries := range []int{1, 3, 12} {
		t.Run(fmt.Sprint(queries), func(t *testing.T) {
			ctx, path := hmc267ReviewContext(t)
			var results []types.ToolResult
			records, candidates, worstComparisons := 0, 0, 0
			for i := 0; i < queries; i++ {
				result := hmc267ReviewQuery(t, ctx, path, map[string]any{"view": "window_stats", "time_start": 1. + float64(i)/1000})
				native := 0
				for _, record := range result.Observations {
					if nativePresentationFamily(record) != "" {
						native++
					}
				}
				records += len(result.Observations)
				candidates += native
				worstComparisons += native * len(result.Observations)
				results = append(results, result)
			}
			hmc267ReviewInstall(ctx, results...)
			before := hmc266NegativeSnapshot(t, ctx)
			started := time.Now()
			for i := 0; i < 3; i++ {
				publication := &answerDocNativePresentationReceipt{}
				publication.prepareSamplingIdentities(ctx)
			}
			elapsed := time.Since(started)
			started = time.Now()
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			assembly := time.Since(started)
			t.Logf("actual_queries=%d records=%d candidate_records=%d per_pass_linear_membership_comparisons=%d identity_build_3_passes=%s finalizer_assembly=%s prompt_bytes=%d", queries, records, candidates, worstComparisons, elapsed, assembly, len(prompt))
			if before != hmc266NegativeSnapshot(t, ctx) {
				t.Fatal("cost measurement changed native evidence or causal authority")
			}
		})
	}
}
