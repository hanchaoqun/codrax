package tool

// P2.1 Session 1 Phase 3 — emit_hypothesis_verdict structural tests.
//
// Pin every branch of the validator that ships in P2.1 because the
// extractor (Turn B) drains this buffer through MutableState.MarkHypothesis
// (the D7 carve-out on the AnalysisIR header invariant). Anything we
// silently let through here ends up in the IR's HypothesisSet[*].Status
// field which downstream consumers (finalizer prompt builder, contract
// checker) read as authoritative.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func newVerdictCtx() *types.BusContext {
	return &types.BusContext{Mutable: types.NewMutableState("")}
}

func TestEmitHypothesisVerdict_AcceptsValidBatch(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{
        "items": [
          {"hypothesis_id": "H1", "status": "confirmed", "rationale": "explorer.go:42 returns true literally", "citation": "internal/agent/explorer.go:42"},
          {"hypothesis_id": "H2", "status": "rejected", "citation": "internal/agent/finalizer.go:100-105"},
          {"hypothesis_id": "H3", "status": "inconclusive", "rationale": "no concrete return literal in the read window"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 3 {
		t.Fatalf("want 3 verdicts in buffer, got %d", len(got))
	}
	if got[0].Status != types.HypConfirmed {
		t.Errorf("[0].Status = %q, want %q", got[0].Status, types.HypConfirmed)
	}
	if got[1].Status != types.HypRejected || got[1].Citation != "internal/agent/finalizer.go:100-105" {
		t.Errorf("[1] mismatch: %+v", got[1])
	}
	if got[2].Status != types.HypInconclusive || got[2].Citation != "" {
		t.Errorf("[2] inconclusive should preserve empty citation: %+v", got[2])
	}
}

func TestEmitHypothesisVerdict_RejectsCitationLineDriftWhenBetterAnchorVisible(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	lines := make([]string, 55)
	for i := range lines {
		lines[i] = "// unrelated"
	}
	lines[100-98] = "func AllMainStages() []PipelineStage {"
	lines[146-98] = "func AllAgentNames() []AgentName {"
	lines[147-98] = "\treturn []AgentName{AgentAnalyzer, AgentExplorer}"
	seedReadFileHistory(ctx, "internal/types/enums.go", 98, lines...)

	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"AllAgentNames() returns AgentAnalyzer and AgentExplorer entries","citation":"internal/types/enums.go:100"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("drifted citation should be rejected when a better visible anchor exists")
	}
	if !strings.Contains(res.Summary, "does not corroborate") {
		t.Fatalf("expected corroboration diagnosis, got: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/types/enums.go:146") {
		t.Fatalf("expected precise candidate anchor in retry hint, got: %q", res.Summary)
	}
}

func TestEmitHypothesisVerdict_AcceptsGroundedCitationWhenIdentifierMatchesReadWindow(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	lines := []string{
		"func AllAgentNames() []AgentName {",
		"\treturn []AgentName{AgentAnalyzer, AgentExplorer}",
		"}",
	}
	seedReadFileHistory(ctx, "internal/types/enums.go", 146, lines...)

	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"AllAgentNames() returns AgentAnalyzer and AgentExplorer entries","citation":"internal/types/enums.go:146"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("grounded citation should be accepted, got: %s", res.Summary)
	}
}

func TestEmitHypothesisVerdict_ResolvesEvidenceIDToCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	seedReadFileHistory(ctx, "internal/orchestrator/scheduler.go", 3922,
		"func runReadSchedulerLoop(ctx context.Context) error {",
		"\treturn nil",
		"}")
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:              "ev-run-loop",
		Kind:            types.EvidenceDirect,
		Subject:         "runReadSchedulerLoop",
		Source:          "internal/orchestrator/scheduler.go",
		LineStart:       3922,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "runReadSchedulerLoop",
		GroundingStatus: types.GroundingGrounded,
	}})

	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"runReadSchedulerLoop is defined","evidence_id":"ev-run-loop"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("evidence_id should resolve to grounded citation, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "internal/orchestrator/scheduler.go:3922" {
		t.Fatalf("citation = %q, want scheduler definition", got[0].Citation)
	}
}

func TestEmitHypothesisVerdict_EvidenceIDBypassesUnrelatedReadHistory(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	seedReadFileHistory(ctx, "internal/agent/unrelated.go", 10,
		"func Other() {}")
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:              "ev-socket-send",
		Kind:            types.EvidenceDirect,
		Subject:         "sock_sendmsg_nosec",
		Predicate:       "dispatches",
		Object:          "inet_sendmsg",
		Source:          "net/socket.c",
		LineStart:       785,
		AnchorKind:      types.AnchorCall,
		AnchorSymbol:    "sock_sendmsg_nosec",
		GroundingStatus: types.GroundingGrounded,
	}})

	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"sock_sendmsg_nosec dispatches into the protocol stack","evidence_id":"ev-socket-send"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("grounded evidence_id should not require a redundant read_file of the evidence source, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "net/socket.c:785" {
		t.Fatalf("citation = %q, want resolved evidence citation", got[0].Citation)
	}
}

func TestEmitHypothesisVerdict_NormalizesMultiAnchorCitationToGroundedEvidence(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	seedReadFileHistory(ctx, "internal/agent/unrelated.go", 10, "func Other() {}")
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:              "ev-parse-trace-mark",
		Kind:            types.EvidenceDirect,
		Subject:         "parseTraceMark",
		Predicate:       "parses",
		Object:          "tracing_mark_write payload",
		Source:          "internal/tracequery/parse.go",
		LineStart:       788,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "parseTraceMark",
		GroundingStatus: types.GroundingGrounded,
	}})

	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"parseTraceMark parses tracing_mark_write payloads","citation":"internal/agent/unrelated.go:10, internal/tracequery/parse.go:788"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("multi-anchor citation should normalize to grounded evidence, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "internal/tracequery/parse.go:788" {
		t.Fatalf("citation = %q, want grounded evidence citation", got[0].Citation)
	}
}

func TestEmitHypothesisVerdict_NormalizesExternalLogFrameCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	logBundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "rust", Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				Lang:       "rust",
				File:       "./src/config.rs",
				Line:       42,
				Func:       "my_app::config::load",
				Raw:        "src/config.rs:42",
				Confidence: 0.95,
			}},
		}},
		IntentHint: types.IntentRootCause,
	}
	mut := types.NewMutableState("")
	mut.SetLogTriage(logBundle)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Language:  "zh",
				Intent:    types.IntentRootCause,
				LogTriage: logBundle,
			},
		},
	}
	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"日志帧显示配置加载阶段 panic","citation":"src/config.rs:42"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("external artifact citation should be accepted as runtime-frame context, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "" {
		t.Fatalf("external frame must not leak into repo citation field, got %q", got[0].Citation)
	}
	if !strings.Contains(got[0].Rationale, "外部运行时帧：src/config.rs:42") {
		t.Fatalf("rationale should preserve artifact frame location, got %q", got[0].Rationale)
	}
}

func TestEmitHypothesisVerdict_NormalizesWrappedExternalLogFrameCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	logBundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "cangjie", Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				Lang:       "cangjie",
				File:       "",
				Line:       78,
				Func:       "demo.cart.Cart.itemAt",
				Raw:        "at demo.cart.Cart.itemAt(src/cart/Cart.cj:78)",
				Confidence: 0.95,
			}},
		}},
		IntentHint: types.IntentRootCause,
	}
	mut := types.NewMutableState("")
	mut.SetLogTriage(logBundle)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: logBundle,
			},
		},
	}
	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"日志帧显示 itemAt 越界 panic","citation":"runtime artifact frame: demo.cart.Cart.itemAt(src/cart/Cart.cj:78)"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("wrapped external artifact citation should be accepted, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "" {
		t.Fatalf("external frame must not leak into repo citation field, got %q", got[0].Citation)
	}
	if !strings.Contains(got[0].Rationale, "外部运行时帧：src/cart/Cart.cj:78") {
		t.Fatalf("rationale should preserve artifact frame location, got %q", got[0].Rationale)
	}
}

func TestEmitHypothesisVerdict_NormalizesArtifactLocalLogLineCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	logBundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "text", Signals: []types.LogSignal{types.SignalValidation}},
		Observations: []types.LogObservation{{
			Kind:       types.LogObservationRuntimeEvent,
			Summary:    "WARN appears on the third attached-log line",
			LineStart:  3,
			LineEnd:    3,
			Diagnostic: true,
			Confidence: 0.95,
		}},
		IntentHint: types.IntentReturnValue,
	}
	mut := types.NewMutableState("")
	mut.SetLogTriage(logBundle)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Language:  "zh",
				Intent:    types.IntentReturnValue,
				LogTriage: logBundle,
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
		},
	}
	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"WARN 出现在附件日志第 3 行","citation":"runtime_artifact:3"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("artifact-local log line citation should be accepted, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "" {
		t.Fatalf("artifact-local line must not leak into repo citation field, got %q", got[0].Citation)
	}
	if !strings.Contains(got[0].Rationale, "附件日志行：3") {
		t.Fatalf("rationale should preserve artifact-local line, got %q", got[0].Rationale)
	}
}

func TestEmitHypothesisVerdict_AcceptsRationaleOnlyRuntimeArtifactVerdict(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	logBundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "text", Signals: []types.LogSignal{types.SignalValidation}},
		Observations: []types.LogObservation{{
			Kind:       types.LogObservationRuntimeEvent,
			Summary:    "WARN appears on the third attached-log line",
			LineStart:  3,
			Diagnostic: true,
			Confidence: 0.95,
		}},
		IntentHint: types.IntentReturnValue,
	}
	mut := types.NewMutableState("")
	mut.SetLogTriage(logBundle)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Language:  "zh",
				Intent:    types.IntentReturnValue,
				LogTriage: logBundle,
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
		},
	}
	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"WARN 出现在附件日志第 3 行。"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("rationale-only runtime artifact verdict should be accepted, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "" {
		t.Fatalf("runtime artifact rationale must not become repo citation, got %q", got[0].Citation)
	}
}

func TestEmitHypothesisVerdict_AcceptsRationaleOnlyRuntimeArtifactVerdictWhenSourceOptional(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	perfBundle := &types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace", Signals: []string{"jank"}},
		Observations: []types.PerfObservation{{
			Kind:       "trace_query",
			Subject:    "Choreographer#doFrame 1254842",
			Summary:    "trace_query identified scheduler latency as the frame root cause",
			LineStart:  1139180,
			DurationMs: 123,
			Confidence: 0.95,
		}},
	}
	mut := types.NewMutableState("")
	mut.SetPerfTrace(perfBundle)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Language:  "zh",
				Intent:    types.IntentRootCause,
				PerfTrace: perfBundle,
			},
		},
	}
	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"trace_query 已定位 Choreographer#doFrame 1254842 的丢帧根因。"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("source-optional runtime artifact verdict should be accepted, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "" {
		t.Fatalf("runtime artifact rationale must not become repo citation, got %q", got[0].Citation)
	}
}

func TestEmitHypothesisVerdict_NormalizesArtifactLocalTraceLineCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	perfBundle := &types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace", Signals: []string{"gc-pause"}},
		Observations: []types.PerfObservation{{
			Kind:       "span",
			Subject:    "H:GC:Collect",
			Summary:    "GC span starts on trace line 5 and ends on line 6",
			LineStart:  5,
			LineEnd:    6,
			DurationMs: 8,
			Confidence: 0.95,
		}},
		IntentHint: string(types.IntentReturnValue),
	}
	mut := types.NewMutableState("")
	mut.SetPerfTrace(perfBundle)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Language:  "zh",
				Intent:    types.IntentReturnValue,
				PerfTrace: perfBundle,
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					SourceQuotes:      []string{"只分析 trace"},
					Confidence:        0.9,
				},
			},
		},
	}
	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"GC span 位于 trace 第 5-6 行","citation":"trace:5-6"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("artifact-local trace line citation should be accepted, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "" {
		t.Fatalf("artifact-local trace line must not leak into repo citation field, got %q", got[0].Citation)
	}
	if !strings.Contains(got[0].Rationale, "附件 trace 行：5-6") {
		t.Fatalf("rationale should preserve artifact-local trace line, got %q", got[0].Rationale)
	}
}

func TestEmitHypothesisVerdict_NormalizesVCSOriginSpecificCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	mut := types.NewMutableState("latest commit")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "git_log",
			Success:  true,
			Summary:  "[git_log: evidence_origin=vcs_metadata count=1]\ne645a3f1 fix: scope convergence finalizer telemetry",
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Language: "zh",
				Intent:   types.IntentExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
			},
		},
	}
	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"最近一次合入来自 git_log 结果","citation":"git_log: commit e645a3f1"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("typed VCS citation should be accepted as origin-specific context, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "" {
		t.Fatalf("VCS ref must not leak into repo citation field, got %q", got[0].Citation)
	}
	if !strings.Contains(got[0].Rationale, "外部证据锚点：git_log: commit e645a3f1") {
		t.Fatalf("rationale should preserve VCS source ref, got %q", got[0].Rationale)
	}
}

func TestEmitHypothesisVerdict_NormalizesCommandOriginSpecificCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	mut := types.NewMutableState("line count")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "exec_command",
			Success:  true,
			Summary:  "[exec_command: $ find internal/tool -name '*.go' | wc -l]\n[exec_command: evidence_origin=command_measurement]\n70693 total",
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Language: "zh",
				Intent:   types.IntentReturnValue,
				Predicates: types.SemanticPredicates{
					IsCountQuestion: true,
				},
			},
		},
	}
	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"计数来自 exec_command 输出","citation":"exec_command: line-count result"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("typed command citation should be accepted as origin-specific context, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 || got[0].Citation != "" {
		t.Fatalf("command ref must be preserved as external context, got %+v", got)
	}
	if !strings.Contains(got[0].Rationale, "外部证据锚点：exec_command: line-count result") {
		t.Fatalf("rationale should preserve command source ref, got %q", got[0].Rationale)
	}
}

func TestEmitHypothesisVerdict_NormalizesOriginSpecificCitationsAcrossLedgerOrigins(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	tests := []struct {
		name     string
		origin   types.AnswerEvidenceOrigin
		citation string
		dims     []types.AnswerAggregateDimension
	}{
		{
			name:     "cross repo index",
			origin:   types.AnswerEvidenceOriginCrossRepoIndex,
			citation: "repo_map: frameworks/base overview",
			dims: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginCrossRepoIndex)},
				{Name: "repo", Value: "frameworks/base"},
				{Name: "view", Value: "overview"},
			},
		},
		{
			name:     "external document",
			origin:   types.AnswerEvidenceOriginExternalDocument,
			citation: "external_document: drive://doc/123#p2",
			dims: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginExternalDocument)},
				{Name: "resource_uri", Value: "drive://doc/123"},
				{Name: "paragraph", Value: "2"},
			},
		},
		{
			name:     "web page",
			origin:   types.AnswerEvidenceOriginWebPage,
			citation: "web_page: https://example.test/spec#api",
			dims: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginWebPage)},
				{Name: "url", Value: "https://example.test/spec"},
				{Name: "selector", Value: "#api"},
			},
		},
		{
			name:     "mcp resource",
			origin:   types.AnswerEvidenceOriginMCPResource,
			citation: "mcp_resource: mcp://docs/spec#/items/0/title",
			dims: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginMCPResource)},
				{Name: "server", Value: "docs"},
				{Name: "resource_uri", Value: "mcp://docs/spec"},
				{Name: "json_pointer", Value: "/items/0/title"},
			},
		},
		{
			name:     "connector resource",
			origin:   types.AnswerEvidenceOriginConnectorResource,
			citation: "connector_resource: jira://ISSUE-7 row 2",
			dims: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginConnectorResource)},
				{Name: "connector", Value: "jira"},
				{Name: "resource_uri", Value: "jira://ISSUE-7"},
				{Name: "row", Value: "2"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mut := types.NewMutableState(tt.name)
			mut.SetTurnAArtifacts(types.TurnAArtifacts{
				AcceptedAggregateFacts: []types.AnswerAggregateFact{{
					Kind:       types.AnswerAggregateScalar,
					Label:      tt.name,
					Value:      "supported externally",
					Role:       types.AnswerAggregateRolePrincipalAnswer,
					Dimensions: tt.dims,
				}},
			})
			ctx := &types.BusContext{
				Mutable: mut,
				AnalysisIR: &types.AnalysisIR{
					Version: types.AnalysisIRVersion,
					RequestModel: types.RequestModel{
						Language: "zh",
						Intent:   types.IntentExplain,
					},
				},
			}
			params := json.RawMessage(fmt.Sprintf(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"结论由 %s 支撑","citation":%q}]}`, tt.origin, tt.citation))

			res, err := tool.Execute(ctx, params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Success {
				t.Fatalf("%s citation should be accepted as origin-specific context, got: %s", tt.origin, res.Summary)
			}
			got := ctx.Mutable.EmittedHypothesisVerdicts()
			if len(got) != 1 || got[0].Citation != "" {
				t.Fatalf("%s ref must not leak into repo citation field, got %+v", tt.origin, got)
			}
			if !strings.Contains(got[0].Rationale, "外部证据锚点："+tt.citation) {
				t.Fatalf("rationale should preserve %s source ref, got %q", tt.origin, got[0].Rationale)
			}
		})
	}
}

func TestEmitHypothesisVerdict_DoesNotNormalizeUnknownExternalOriginCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"pretend git result","citation":"git_log: commit e645a3f1"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("origin-specific citation without typed ledger support must not be accepted")
	}
	if !strings.Contains(res.Summary, "does not look like 'path:line'") {
		t.Fatalf("expected ordinary malformed-citation failure, got %q", res.Summary)
	}
}

func TestEmitHypothesisVerdict_DoesNotNormalizeRepoCitationWithoutExternalArtifact(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"h1","status":"confirmed","rationale":"AllAgentNames is defined","citation":"internal/types/enums.go:146"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo citation should remain accepted when no grounding context is present, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedHypothesisVerdicts()
	if len(got) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got))
	}
	if got[0].Citation != "internal/types/enums.go:146" {
		t.Fatalf("repo citation was unexpectedly normalized: %+v", got[0])
	}
}

func TestEmitHypothesisVerdict_ConfirmedRequiresCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"confirmed","rationale":"trust me"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("confirmed without citation must be rejected")
	}
	if !strings.Contains(res.Summary, "requires a citation") {
		t.Errorf("expected citation diagnosis, got: %q", res.Summary)
	}
}

func TestEmitHypothesisVerdict_RejectedRequiresCitation(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"rejected"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("rejected without citation must be rejected")
	}
}

func TestEmitHypothesisVerdict_InconclusiveCitationOptional(t *testing.T) {
	// The whole point of inconclusive is that you investigated but
	// could not point at a definitive cite. A bare inconclusive
	// without rationale is allowed but lossy; pin that it parses.
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H7","status":"inconclusive"}]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("inconclusive without citation should be accepted, got: %s", res.Summary)
	}
}

func TestEmitHypothesisVerdict_RejectsUnknownStatus(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"maybe"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("unknown status must be rejected")
	}
	if !strings.Contains(res.Summary, "unknown status") {
		t.Errorf("expected status enum diagnosis, got: %q", res.Summary)
	}
}

func TestEmitHypothesisVerdict_RejectsMissingHypothesisID(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"","status":"inconclusive"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("missing hypothesis_id must be rejected")
	}
}

func TestEmitHypothesisVerdict_RejectsMalformedCitation(t *testing.T) {
	cases := []struct {
		name string
		cite string
	}{
		{"prose colon", "the function explorer.go: returns true"},
		{"line zero", "internal/agent/explorer.go:0"},
		{"non-numeric", "internal/agent/explorer.go:abc"},
		{"missing line", "internal/agent/explorer.go:"},
		{"reversed range", "internal/agent/explorer.go:50-x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &EmitHypothesisVerdict{}
			ctx := newVerdictCtx()
			params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"confirmed","citation":"` + tc.cite + `"}]}`)
			res, _ := tool.Execute(ctx, params)
			if res.Success {
				t.Errorf("malformed citation %q should be rejected", tc.cite)
			}
		})
	}
}

func TestEmitHypothesisVerdict_RejectsUnknownFields(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"inconclusive","note":"extra"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("unknown field must be rejected")
	}
}

func TestEmitHypothesisVerdict_RejectsEmptyItems(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := newVerdictCtx()
	params := json.RawMessage(`{"items":[]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("empty items must be rejected")
	}
}

func TestEmitHypothesisVerdict_RequiresMutable(t *testing.T) {
	tool := &EmitHypothesisVerdict{}
	ctx := &types.BusContext{}
	params := json.RawMessage(`{"items":[{"hypothesis_id":"H1","status":"inconclusive"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure when Mutable is nil")
	}
}

// looksLikeCitation unit boundary: exercised through the tool above
// for the failure cases; this table covers positive and edge cases
// directly so future refactors of the helper cannot diverge from
// internal/orchestrator/contract_check.go's regex.
func TestLooksLikeCitation_Boundary(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"internal/agent/explorer.go:42", true},
		{"README.md:1", true},
		{"path/with-dashes/file.go:1234", true},
		{"path/file.go:50-75", true},
		{"path/file.go:50-50", true},
		{"a.b.c:10", true},
		{"path/file.go:0", false},
		{"path/file.go:01", true}, // leading-zero tolerated for parity with contract_check.go's `\d{1,6}` regex
		{"path/file.go", false},
		{"path/file.go:", false},
		{":42", false},
		{"prose with colon: 42", false},
		{"path/file.go:abc", false},
		{"path/file.go:10-", false},
		{"path/file.go:-10", false}, // dash at index 0 not handled as range
	}
	for _, tc := range cases {
		t.Run(tc.s, func(t *testing.T) {
			if got := looksLikeCitation(tc.s); got != tc.want {
				t.Errorf("looksLikeCitation(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}
