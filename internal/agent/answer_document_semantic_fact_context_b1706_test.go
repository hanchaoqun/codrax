package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1706SemanticFactContext(t *testing.T) (*types.AgentContext, types.ToolResult) {
	t.Helper()
	dir := t.TempDir()
	trace := "# tracer: nop\n" +
		"idle-0 (0) [000] .... 10.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=100 next_prio=120\n" +
		"client-100 (100) [000] .... 10.000500: sched_switch: prev_comm=client prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=worker next_pid=300 next_prio=120\n" +
		"compiler-200 (100) [001] .... 10.001000: tracing_mark_write: B|100|JIT compiling void business.Widget.first()\n" +
		"compiler-200 (100) [001] .... 10.002781: tracing_mark_write: E|100\n" +
		"compiler-200 (100) [001] .... 10.004000: tracing_mark_write: B|100|JIT compiling void business.Decoder.second()\n" +
		"compiler-200 (100) [001] .... 10.004607: tracing_mark_write: E|100\n" +
		"loader-400 (100) [002] .... 10.006000: tracing_mark_write: B|100|VerifyClass customer.billing.Invoice\n" +
		"loader-400 (100) [002] .... 10.006250: tracing_mark_write: E|100\n" +
		"worker-300 (300) [000] .... 10.012000: sched_wakeup: comm=client pid=100 prio=120 target_cpu=0\n" +
		"worker-300 (300) [000] .... 10.013000: sched_switch: prev_comm=worker prev_pid=300 prev_prio=120 prev_state=R ==> next_comm=client next_pid=100 next_prio=120\n" +
		"client-100 (100) [000] .... 10.020000: sched_switch: prev_comm=client prev_pid=100 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n"
	path := filepath.Join(dir, "capture.ftrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 10.0, 10.020
	bus := &types.BusContext{RepoRoot: dir, WorkDir: filepath.Join(dir, ".codrax", "blob", "b1706"),
		AttachedHitrace: trace, AttachedHitraceSource: "generic_ftrace", Mutable: types.NewMutableState("Bounded trace facts"),
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "attachment"}}},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactOccurrenceTime}},
			RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "client-100", Source: "user_explicit"}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10.000000..10.020000"},
		}},
	}
	params := json.RawMessage(`{"source":"attached_trace","view":"root_cause_rank","pid":100,"time_start":10,"time_end":10.020}`)
	result, err := (&tool.TraceQuery{}).Execute(bus, params)
	if err != nil || !result.Success || result.TraceQuerySourceRead.Path() == "" {
		t.Fatalf("native attached trace query must publish a physical-source receipt: %v %+v", err, result)
	}
	bus.Mutable.AppendDispatchToolResult(result)
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "MODEL OWNED WORDS"}}})
	return promptcontext.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize), result
}

func b1706SemanticRecords(result types.ToolResult) []types.ObservationRecord {
	var out []types.ObservationRecord
	for _, record := range result.Observations {
		if record.Predicate == "trace_semantic_span" {
			out = append(out, record)
		}
	}
	return out
}

func b1706ReplaceResult(ctx *types.AgentContext, result types.ToolResult) {
	ctx.Mutable.ResetDispatchToolResults()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
}

func TestB1706SemanticFactsRejectUnboundSourcesAndScopes(t *testing.T) {
	for name, mutate := range map[string]func(*types.AgentContext, *types.ToolResult){
		"no native receipt": func(_ *types.AgentContext, r *types.ToolResult) {
			r.TraceQuerySourceRead = types.TraceQuerySourceReadRef{}
		},
		"failed query": func(_ *types.AgentContext, r *types.ToolResult) { r.Success = false },
		"missing reference": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].SourceRef = types.ObservationSourceRef{}
			}
		},
		"same basename foreign source": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].SourceRef.Path = filepath.Join("/other-capture", filepath.Base(r.Observations[i].SourceRef.Path))
			}
		},
		"unknown query scope": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].SourceRef.QueryScopeID = ""
			}
		},
		"unknown query window": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].SourceRef.QueryWindowKnown = false
			}
		},
		"other window": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].SourceRef.QueryWindowEndTs = 10.030
			}
		},
		"other target": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].SourceRef.QueryTargetPID = 200
			}
		},
		"missing query target": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].SourceRef.QueryTargetPID = 0
				r.Observations[i].SourceRef.QueryTargetThread = ""
			}
		},
		"process scope": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].SourceRef.QueryTargetScope = "process"
			}
		},
		"model producer": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].Producer = "model"
			}
		},
		"soft observation": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].GroundingPolicy = types.ClaimGroundingSoft
			}
		},
		"source origin": func(_ *types.AgentContext, r *types.ToolResult) {
			for i := range r.Observations {
				r.Observations[i].Origin = types.AnswerEvidenceOriginCurrentSource
			}
		},
		"no explicit window": func(ctx *types.AgentContext, _ *types.ToolResult) {
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = nil
		},
		"no user target": func(ctx *types.AgentContext, _ *types.ToolResult) { ctx.AnalysisIR.RequestModel.RuntimeTargets = nil },
		"unrequested fact family": func(ctx *types.AgentContext, _ *types.ToolResult) {
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactRecordedReason}
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, result := b1706SemanticFactContext(t)
			result.Observations = b1706SemanticRecords(result)
			mutate(ctx, &result)
			b1706ReplaceResult(ctx, result)
			if got := renderAnswerDocTraceDecisionHandoff(ctx); strings.Contains(got, "Bounded Semantic Span Facts") {
				t.Fatalf("unbound semantic inventory borrowed another query/attachment:\n%s", got)
			}
		})
	}
}

func TestB1706SemanticFactsKeepOtherDiagnosticsAndFullReportSeparate(t *testing.T) {
	ctx, result := b1706SemanticFactContext(t)
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	for _, forbidden := range []string{"Axis A", "Axis B", "root_cause_primary", "effective_impact", "target_window_states", "repair_direction"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("unrelated diagnostic entered finite facts: %s", forbidden)
		}
	}
	// Presence of ordinary rank/state records is a positive fixture premise.
	if len(result.Observations) <= len(b1706SemanticRecords(result)) {
		t.Fatal("fixture lacks other diagnostics")
	}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
	before := b1670ScopeSnapshot(t, ctx)
	got = renderAnswerDocTraceDecisionHandoff(ctx)
	if !strings.Contains(got, "## Trace Decision Inputs") || !strings.Contains(got, "business.Widget.first()") || strings.Contains(got, "## Bounded Semantic Span Facts") {
		t.Fatalf("existing full-report branch changed:\n%s", got)
	}
	if before != b1670ScopeSnapshot(t, ctx) {
		t.Fatal("full-report rendering mutated source authority")
	}
}

func TestB1706SemanticFactLabelsAndInventoryAreBounded(t *testing.T) {
	ctx, result := b1706SemanticFactContext(t)
	// A renderer-capacity fixture, derived from the real native record shape.
	// These synthetic rows do not stand in for the public-query positive test.
	base := b1706SemanticRecords(result)[0]
	result.Observations = nil
	label := strings.Repeat("界\n`<unsafe>\x00", 100)
	for i := 0; i < 12; i++ {
		row := base
		row.ID, row.Subject = fmt.Sprintf("capacity-%d", i), fmt.Sprintf("worker-%d", i+200)
		row.Span.LineStart, row.Span.LineEnd = i*30+1, i*30+29
		row.RichNotes = []string{"semantic_class=runtime_compile", "chain_relevance=background", "span_name=" + label, "member_count=12", "selected_window=10.000000..10.020000"}
		var names, ranges, values []string
		for member := 0; member < 12; member++ {
			names = append(names, label)
			ranges = append(ranges, fmt.Sprintf("%d..%d", row.Span.LineStart+member*2, row.Span.LineStart+member*2+1))
			values = append(values, "0.100")
		}
		row.RichNotes = append(row.RichNotes, "member_roster="+strings.Join(names, " | "), "member_line_ranges="+strings.Join(ranges, "|"), "member_wall_ms="+strings.Join(values, "|"))
		result.Observations = append(result.Observations, row)
	}
	b1706ReplaceResult(ctx, result)
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	if !utf8.ValidString(got) || len(got) > traceSemanticFactByteLimit {
		t.Fatalf("invalid or unbounded UTF-8 output: %d bytes", len(got))
	}
	for _, want := range []string{"families_available=12", "families_omitted=", "members_omitted=4", "[truncated]", `\n\u0060\u003cunsafe\u003e\x00`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing bounded-label/inventory disclosure %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "member_9 ") || strings.Contains(got, "<unsafe>") || strings.Contains(got, "\x00") || strings.Contains(got, "\n`<unsafe>") {
		t.Fatal("member limit or label escaping failed")
	}
	for _, input := range []string{"short exact name", strings.Repeat("界", 1000), strings.Repeat("\n\x00`", 1000), string([]byte{0xff, 0xfe})} {
		quoted := traceSemanticFactQuoted(input)
		if !utf8.ValidString(quoted) || len(quoted) > 512 || strings.Contains(quoted, "\n") || strings.Contains(quoted, "`") {
			t.Fatalf("unsafe/big scalar %q", quoted)
		}
	}
	if got := traceSemanticFactQuoted("业务.Widget.method(arg)"); got != `"业务.Widget.method(arg)"` {
		t.Fatalf("short original names must remain exact: %s", got)
	}
}

func TestB1706SemanticFactFamilyCountCapIsDisclosed(t *testing.T) {
	ctx, result := b1706SemanticFactContext(t)
	base := b1706SemanticRecords(result)[0]
	result.Observations = nil
	for i := 0; i < 12; i++ {
		row := base
		row.ID, row.Subject = fmt.Sprintf("family-%d", i), fmt.Sprintf("compiler-%d", i+200)
		result.Observations = append(result.Observations, row)
	}
	b1706ReplaceResult(ctx, result)
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	if !strings.Contains(got, "families_shown=8; families_available=12; families_omitted=4") || strings.Count(got, "  host=") != 8 {
		t.Fatalf("family cap lost exact shown/available/omitted counts:\n%s", got)
	}
}

func TestB1706ActualQueryDispatchBuilderSuppliesBoundedSemanticFacts(t *testing.T) {
	ctx, result := b1706SemanticFactContext(t)
	ledger := answerDocObservationLedger(ctx)
	set := types.CompileTraceCausalProjectionSet(ledger)
	if !ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
		t.Fatal("fixture needs a validated typed requested window")
	}
	if types.RuntimeTraceReportMaterializationAllowed(&ctx.AnalysisIR.RequestModel, set) {
		t.Fatal("fixture must not authorize a full causal report")
	}
	semantic := 0
	for _, row := range ledger.Records {
		if row.Predicate == "trace_semantic_span" {
			semantic++
		}
	}
	if semantic < 2 {
		t.Fatalf("real query must publish JIT and another semantic family, got %d", semantic)
	}
	before := b1670ScopeSnapshot(t, ctx)
	beforeSelection, _ := json.Marshal([]any{ctx.Mutable.TraceRootCauseReport(), ctx.Mutable.TraceFindingContract(), ctx.Mutable.TraceRootCauseSelectorRejected()})
	for name, got := range map[string]string{
		"handoff":     renderAnswerDocTraceDecisionHandoff(ctx),
		"instruction": (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil),
	} {
		for _, want := range []string{"Bounded Semantic Span Facts", types.AnswerControlMetadataVisibilityGuide, "10.000000..10.020000", "compiler-200", "business.Widget.first()", "business.Decoder.second()", "1.781ms", "0.607ms", "2.388ms", "customer.billing.Invoice", "0.250ms", result.TraceQuerySourceRead.Path(), "lines=4..5", "lines=6..7", "relationship_to_target=unknown", "not a root-cause or eliminable-time claim"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s lost bounded fact %q:\n%s", name, want, got)
			}
		}
		if strings.Contains(got, "## Trace Decision Inputs") || strings.Contains(got, "Axis B") {
			t.Errorf("%s broadened bounded facts into full causal synthesis", name)
		}
	}
	if before != b1670ScopeSnapshot(t, ctx) {
		t.Fatal("prompt changed source facts, model answer, request or causal authority")
	}
	afterSelection, _ := json.Marshal([]any{ctx.Mutable.TraceRootCauseReport(), ctx.Mutable.TraceFindingContract(), ctx.Mutable.TraceRootCauseSelectorRejected()})
	if string(beforeSelection) != string(afterSelection) {
		t.Fatal("noncausal prompt changed root-cause sidecar selection/authority")
	}
}

func TestB1706CaptureIdentityCannotBorrowPhysicalQueryReceipt(t *testing.T) {
	ctx, _ := b1706SemanticFactContext(t)
	ledger := answerDocObservationLedger(ctx)
	for i := range ledger.Records {
		ledger.Records[i].SourceRef.CaptureIdentityPath = "/foreign/capture.ftrace"
	}
	if got := renderAnswerDocBoundedSemanticFacts(ctx, ledger); got != "" {
		t.Fatalf("same physical path/query key borrowed foreign capture identity:\n%s", got)
	}
	// Public native query against a different actual file, with the same basename,
	// target and time window, must not bind to the attached capture.
	other := filepath.Join(t.TempDir(), "capture.ftrace")
	if err := os.WriteFile(other, []byte(ctx.AttachedHitrace), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": other, "view": "root_cause_rank", "pid": 100, "time_start": 10, "time_end": 10.020})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success || result.TraceQuerySourceRead.Path() == "" || len(b1706SemanticRecords(result)) == 0 {
		t.Fatalf("foreign actual query fixture failed: %v %+v", err, result)
	}
	b1706ReplaceResult(ctx, result)
	if got := renderAnswerDocTraceDecisionHandoff(ctx); strings.Contains(got, "Bounded Semantic Span Facts") {
		t.Fatalf("same-basename foreign capture borrowed attachment scope:\n%s", got)
	}
}

func TestB1706InvalidSemanticMemberArraysAreNotPrinted(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"member_wall_ms", "NaN|0.607"}, {"member_wall_ms", "+Inf|0.607"},
		{"member_wall_ms", "-1.781|0.607"}, {"member_wall_ms", "1.781"},
		{"member_line_ranges", "0..5|6..7"}, {"member_line_ranges", "5..4|6..7"},
		{"member_line_ranges", "4..5|6..700"}, {"member_roster", "only one member"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			ctx, result := b1706SemanticFactContext(t)
			result.Observations = b1706SemanticRecords(result)[:1]
			row := &result.Observations[0]
			for i, note := range row.RichNotes {
				if strings.HasPrefix(note, tc.key+"=") {
					row.RichNotes[i] = tc.key + "=" + tc.value
				}
			}
			b1706ReplaceResult(ctx, result)
			got := renderAnswerDocTraceDecisionHandoff(ctx)
			if !strings.Contains(got, "total=2.388ms") || !strings.Contains(got, "member_detail=not_available") || strings.Contains(got, "member_1 ") {
				t.Fatalf("corrupt member list must keep the valid family value without fake detail:\n%s", got)
			}
		})
	}
}
