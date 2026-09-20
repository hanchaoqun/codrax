package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const queriedIOSemanticsHeading = "### Already-Queried IO Measurement Meanings"

func queriedIOSemanticsPublicContext(t *testing.T) *types.AgentContext {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_io_request_latency_distribution", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 14.0
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", Stage: types.StageFinalize,
		AgentName: types.AgentFinalizer, Mutable: types.NewMutableState("Describe the queried measurements"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: "zh", PerfTrace: &types.PerfBundle{},
			RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000s to 14.000s"},
		}},
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("public query failed: %v (success=%t)", err, result.Success)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	return ctx
}

func queriedIOSemanticsSection(prompt string) string {
	_, section, present := strings.Cut(prompt, queriedIOSemanticsHeading)
	if !present {
		return ""
	}
	section, _, _ = strings.Cut(strings.TrimLeft(section, "\n"), "\n\n")
	return section
}

func TestQueriedIOMeaningsReachPublicFinalizerWithGenericCountFamily(t *testing.T) {
	ctx := queriedIOSemanticsPublicContext(t)
	ledger := answerDocObservationLedger(ctx)
	beforeLedger, _ := json.Marshal(ledger)
	beforeRequest, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	section := queriedIOSemanticsSection(prompt)
	if section == "" {
		t.Fatal("generic count/duration classification lost the already-queried IO measurement meanings")
	}
	for _, want := range []string{
		"Accepted complete pairs=17", "query-selected details=8", "paired details beyond cap=9",
		"not current prompt row counts", "not capture or scan completeness", "independently of this detail cap",
		"query_window=`1.000000..14.000000`", "observed_interval=`12.000000..12.046000`",
		"not a requested fact family", "not target blocking time", "no containment or subtraction",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("queried IO semantics lost %q", want)
		}
	}
	for _, group := range []struct{ identity, numbers, endpoints string }{
		{"event=block_rq dev=12,80 op=R", "samples=11 min=1.000 mean=6.000 max=11.000 p50=6.000 p90=10.000 p95=10.500 p99=10.900", "block_rq_issue→block_rq_complete"},
		{"event=block_rq dev=12,80 op=W", "samples=3 min=5.000 mean=15.000 max=25.000 p50=15.000 p90=23.000 p95=24.000 p99=24.800", "block_rq_issue→block_rq_complete"},
		{"event=block_bio dev=12,80 op=R", "samples=3 min=2.000 mean=4.000 max=6.000 p50=4.000 p90=5.600 p95=5.800 p99=5.960", "block_bio_queue→block_bio_complete"},
	} {
		found := false
		for _, record := range ledger.Records {
			if record.Predicate != "storage_latency_by_layer" || !strings.Contains(traceQueryObservationSupplementNoteValue(record, "storage_request_group"), group.identity) {
				continue
			}
			for _, line := range strings.Split(section, "\n") {
				if !strings.Contains(line, "id=`"+record.ID+"`") {
					continue
				}
				found = true
				for _, want := range []string{group.identity, group.numbers, group.endpoints, "issuers=all", record.SourceRef.Path, "query_scope=`" + record.SourceRef.QueryScopeID + "`", "query_window=`1.000000..14.000000`"} {
					if !strings.Contains(line, want) {
						t.Errorf("same group %s lost %q", record.ID, want)
					}
				}
			}
		}
		if !found {
			t.Errorf("no scoped source record for %s", group.identity)
		}
	}
	afterLedger, _ := json.Marshal(answerDocObservationLedger(ctx))
	afterRequest, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	if string(beforeLedger) != string(afterLedger) || string(beforeRequest) != string(afterRequest) {
		t.Fatal("prompt-only meanings mutated observations or the model-owned request classification")
	}
}

func queriedIOSemanticsUnitContext() *types.AgentContext {
	return &types.AgentContext{
		AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		}},
	}
}

func TestQueriedIOMeaningsRequirePublishedDeterministicIO(t *testing.T) {
	ctx := queriedIOSemanticsUnitContext()
	for _, tc := range []struct {
		name   string
		mutate func(*types.ObservationRecord)
	}{
		{"model_origin", func(r *types.ObservationRecord) { r.Origin = types.AnswerEvidenceOriginSystemInference }},
		{"pretriage", func(r *types.ObservationRecord) { r.Producer = "perf_trace" }},
		{"fake_producer", func(r *types.ObservationRecord) { r.Producer = "model:trace_query" }},
		{"producer_suffix_lookalike", func(r *types.ObservationRecord) { r.Producer = "trace_query_model" }},
		{"soft", func(r *types.ObservationRecord) { r.GroundingPolicy = types.ClaimGroundingSoft }},
		{"prose_only", func(r *types.ObservationRecord) {
			r.Predicate = "model_summary"
			r.Summary = "io_latency_coverage Accepted complete pairs=23"
		}},
		{"other_measurement", func(r *types.ObservationRecord) { r.Predicate = "target_window_wait_occurrences" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := ioDetailCoverageTestRecord("5", "23", "18")
			tc.mutate(&r)
			if got := renderAnswerDocQueriedIOMeasurementSemantics(ctx, types.ObservationLedger{Records: []types.ObservationRecord{r}}); got != "" {
				t.Fatalf("non-native IO manufactured a semantics card: %s", got)
			}
		})
	}
	if got := renderAnswerDocQueriedIOMeasurementSemantics(ctx, types.ObservationLedger{}); got != "" {
		t.Fatal("empty ledger manufactured IO context")
	}
	r := ioDetailCoverageTestRecord("5", "23", "18")
	r.Producer = "trace_query:run2"
	if got := renderAnswerDocQueriedIOMeasurementSemantics(ctx, types.ObservationLedger{Records: []types.ObservationRecord{r}}); !strings.Contains(got, "Accepted complete pairs=23") {
		t.Fatal("legitimate run-suffixed producer lost its original IO counts")
	}
}

func TestQueriedIOMeaningsDoNotRepeatExistingDedicatedLanes(t *testing.T) {
	ctx := queriedIOSemanticsPublicContext(t)
	for _, tc := range []struct {
		name    string
		profile types.RuntimeQuestionProfile
		want    string
	}{
		{"io", types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency}}, "### Requested Runtime Fact Authority"},
		{"causal", types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}, "### IO Measurements For Causal Interpretation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &tc.profile
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if strings.Contains(prompt, queriedIOSemanticsHeading) || !strings.Contains(prompt, tc.want) {
				t.Fatalf("existing %s lane was replaced, dropped, or redundantly re-explained", tc.name)
			}
		})
	}
}

func TestQueriedIOMeaningsPreserveExplicitWindowExclusion(t *testing.T) {
	ctx := queriedIOSemanticsPublicContext(t)
	start, end := 2.0, 3.0
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "2.000s to 3.000s",
	}
	if prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil); strings.Contains(prompt, queriedIOSemanticsHeading) {
		t.Fatalf("wider published query leaked into the narrower explicit window through the new card: %s", queriedIOSemanticsSection(prompt))
	}
	// The existing causal IO lane also consults the producer's QueryWindow
	// receipt when selected_window notes are absent; this lane must do the same.
	r := ioDetailCoverageTestRecord("5", "23", "18")
	r.RichNotes = []string{types.TraceNoteKeyIOCoverageEmitted + "=5", types.TraceNoteKeyTotal + "=23", types.TraceNoteKeyIOOverflowPairs + "=18"}
	r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs = 1, 14
	if got := renderAnswerDocQueriedIOMeasurementSemantics(ctx, types.ObservationLedger{Records: []types.ObservationRecord{r}}); got != "" {
		t.Fatal("known wider SourceRef query window was ignored without a selected_window note")
	}
}

func TestQueriedIOMeaningsKeepIndependentReceiptsAndUnknownCounts(t *testing.T) {
	ctx := queriedIOSemanticsUnitContext()
	for _, field := range []string{"source", "query", "payload", "window", "line_range", "target", "generation"} {
		t.Run(field, func(t *testing.T) {
			a := ioDetailCoverageTestRecord("5", "23", "18")
			b := a
			var distinguish string
			switch field {
			case "source":
				b.SourceRef.Path = "/different-capture/io.ftrace"
				distinguish = `source_path="/different-capture/io.ftrace"`
			case "query":
				b.SourceRef.QueryScopeID = "query:second"
				distinguish = "query_scope=`query:second`"
			case "payload":
				b.SourceRef.PayloadRef = "/results/second.json"
				distinguish = `query_result="/results/second.json"`
			case "window":
				b.SourceRef.QueryWindowStartTs, b.SourceRef.QueryWindowEndTs = 20, 30
				distinguish = "query_window=`20.000000..30.000000`"
			case "line_range":
				b.SourceRef.QueryLineStart, b.SourceRef.QueryLineEnd = 10, 90
				distinguish = "query_lines=`10..90`"
			case "target":
				b.SourceRef.QueryTargetPID, b.SourceRef.QueryTargetThread, b.SourceRef.QueryTargetScope = 72, "other-72", "thread"
				distinguish = `query_target_pid=72; query_target_thread="other-72"; query_target_scope="thread"`
			case "generation":
				a.ObservedAt, b.ObservedAt = "first", "later"
				distinguish = `observed_at="later"`
			}
			for _, records := range [][]types.ObservationRecord{{a, b}, {b, a}} {
				got := renderAnswerDocQueriedIOMeasurementSemantics(ctx, types.ObservationLedger{Records: records})
				if strings.Count(got, "Accepted complete pairs=23") != 2 || strings.Contains(got, "pairs=46") {
					t.Fatalf("independent %s receipts were merged or added: %s", field, got)
				}
				if strings.Count(got, distinguish) != 1 {
					t.Errorf("different %s receipt not distinguishable on its own row (%q): %s", field, distinguish, got)
				}
				for _, line := range strings.Split(got, "\n") {
					if strings.Contains(line, distinguish) && !strings.Contains(line, "Accepted complete pairs=23") {
						t.Errorf("%s scope and count were split across records: %s", field, line)
					}
				}
			}
		})
	}
	for _, counts := range [][3]string{{"5", "", "18"}, {"", "23", "18"}, {"5", "23", ""}, {"5", "23", "17"}} {
		r := ioDetailCoverageTestRecord(counts[0], counts[1], counts[2])
		got := renderAnswerDocQueriedIOMeasurementSemantics(ctx, types.ObservationLedger{Records: []types.ObservationRecord{r}})
		if strings.Contains(got, "Accepted complete pairs=") || !strings.Contains(got, "counts are incomplete or inconsistent") || !strings.Contains(got, "not capture or scan completeness") {
			t.Errorf("incomplete coverage %v invented a known/complete population: %s", counts, got)
		}
	}
}

func TestQueriedIOMeaningsDoNotInventEndpointsOrCrossFamilyBlocking(t *testing.T) {
	ctx := queriedIOSemanticsUnitContext()
	for _, caliber := range []string{"", "unknown_future_caliber"} {
		r := b1618IOCaptureRecord("request", "/capture/io.ftrace", "io_latency")
		r.Object, r.Value, r.Unit = "block_rq", "35.000", "ms"
		r.Summary = "block_rq_issue to block_rq_complete proves blocking 35ms"
		r.RichNotes = append(r.RichNotes, types.TraceNoteKeyIORequestResidenceCaliber+"="+caliber)
		other := b1618IOCaptureRecord("scheduler", "/capture/io.ftrace", "target_window_wait_occurrences")
		other.Value = "900"
		got := renderAnswerDocQueriedIOMeasurementSemantics(ctx, types.ObservationLedger{Records: []types.ObservationRecord{r, other}})
		if !strings.Contains(got, "request_endpoints=unknown") || strings.Contains(got, "block_rq_issue→block_rq_complete") || strings.Contains(got, "id=`scheduler`") || strings.Contains(got, "proves blocking 35ms") {
			t.Errorf("unproven caliber or another family manufactured IO scope/causality: %s", got)
		}
	}
	for _, tc := range []struct{ caliber, endpoints string }{
		{"block_rq_issue_to_complete", "block_rq_issue→block_rq_complete"},
		{"block_bio_queue_to_complete", "block_bio_queue→block_bio_complete"},
	} {
		r := b1618IOCaptureRecord("closed-request", "/capture/io.ftrace", "io_latency")
		r.RichNotes = append(r.RichNotes,
			types.TraceNoteKeyIORequestResidenceCaliber+"="+tc.caliber,
			types.TraceNoteKeyIORequestResidence+"=35.000",
			types.TraceNoteKeyIOCompletionWokeIssuer+"=true",
			types.TraceNoteKeyIOIssuerBlockedState+"=S",
			types.TraceNoteKeyIOIssuerBlocked+"=31.000",
		)
		got := renderAnswerDocQueriedIOMeasurementSemantics(ctx, types.ObservationLedger{Records: []types.ObservationRecord{r}})
		for _, want := range []string{tc.endpoints, "request_residence=`35.000`", "issuer_blocked=`31.000`", "issuer_blocked_state=`S`", "cross_ruler_addition=`forbidden`"} {
			if !strings.Contains(got, want) {
				t.Errorf("original independent ruler lost %q: %s", want, got)
			}
		}
	}
}

func TestQueriedIOMeaningsUnknownSourceAndQueryRemainUnknown(t *testing.T) {
	ctx := queriedIOSemanticsUnitContext()
	r := ioDetailCoverageTestRecord("5", "23", "18")
	r.SourceRef = types.ObservationSourceRef{}
	r.Span = types.ObservationSpan{StartTs: 20, EndTs: 21}
	r.Summary = "source=/invented/from-prose query_window=20..21"
	got := renderAnswerDocQueriedIOMeasurementSemantics(ctx, types.ObservationLedger{Records: []types.ObservationRecord{r}})
	for _, want := range []string{`source_path=""`, `query_result=""`, "query_window=`unknown`", "observed_interval=`20.000000..21.000000`"} {
		if !strings.Contains(got, want) {
			t.Errorf("unknown scope lost %q: %s", want, got)
		}
	}
	for _, forbidden := range []string{"/invented/from-prose", "query_window=`20.000000..21.000000`", "query_lines=`unrestricted`", "query_target_pid=0"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("unknown scope was reconstructed as %q: %s", forbidden, got)
		}
	}
}
