package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the real explore handoff, dedicated supplement ledger and finalizer
// instruction. A wider model query remains evidence, but is not relabeled as
// the requested right-open population when the system obtains that population.
func TestPrimaryWindowReplayActualFinalizerPublic(t *testing.T) {
	fixture, err := os.ReadFile("../../eval/fixtures/hmosperf_rendering_candidates/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.05
	request := "分析附件中1.000到1.050秒的渲染框架与线程角色线索。"
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		PerfTrace:            &types.PerfBundle{},
		RuntimeTargetProfile: &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNoNamedTarget},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis,
			FrameCausalityRequested: true},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000到1.050秒", Confidence: 1},
	}
	dir := t.TempDir()
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AttachedHitrace: string(fixture),
		Mutable: types.NewMutableState(request), AnalysisIR: &types.AnalysisIR{RequestModel: rm,
			AnswerContract: types.AnswerContract{Language: "zh"}}}
	bus.Mutable.SetRequestModel(rm)
	params, err := json.Marshal(map[string]any{"source": "attached_trace", "view": "rendering_candidates", "time_start": start, "time_end": 1.051})
	if err != nil {
		t.Fatal(err)
	}
	original, err := (&tool.TraceQuery{}).Execute(bus, params)
	if err != nil || !original.Success {
		t.Fatalf("original public query: %v %s", err, original.Summary)
	}
	bus.ToolResults = []types.ToolResult{original}
	bus.Mutable.AppendDispatchToolResult(original)
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{original}})
	modelBefore, err := json.Marshal([]any{bus.ToolResults, bus.Mutable.DispatchToolResults(), bus.Mutable.TurnAArtifacts()})
	if err != nil {
		t.Fatal(err)
	}
	wideBoundarySeen := false
	for _, record := range original.Observations {
		if p, ok := tool.DecodeTraceRenderingCandidates(record); ok {
			for _, candidate := range p.Candidates {
				wideBoundarySeen = wideBoundarySeen || candidate.OwnerID == 900
			}
		}
	}
	if !wideBoundarySeen {
		t.Fatal("fixture did not expose the wider query's right-boundary candidate")
	}
	beforeCtx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	if got := renderAnswerDocRenderingCandidates(beforeCtx, answerDocObservationLedger(beforeCtx)); got != "" {
		t.Fatalf("wide model query already claimed the requested population: %s", got)
	}

	outcome := tool.RunTraceQuerySystemSupplement(bus)
	if len(outcome.Executed) == 0 || outcome.Executed[0] != "rendering_candidates" {
		t.Fatalf("primary window was not supplemented: %+v", outcome)
	}
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	input := types.ObservationLedgerInputFromAgentContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	if len(input.ToolResults) != 1 || len(input.SystemTraceSupplementResults) == 0 {
		t.Fatalf("supplement did not retain its dedicated lane: model=%d system=%d", len(input.ToolResults), len(input.SystemTraceSupplementResults))
	}
	ledger := answerDocObservationLedger(ctx)
	primarySeen, originalSeen := false, false
	for _, record := range ledger.Records {
		p, ok := tool.DecodeTraceRenderingCandidates(record)
		if !ok {
			continue
		}
		if p.Window.StartTs == start && p.Window.EndTs == 1.051 && !record.SystemSupplement {
			originalSeen = true
		}
		if p.Window.StartTs != start || p.Window.EndTs != end {
			continue
		}
		primarySeen = true
		if !record.SystemSupplement || p.Window.EndInclusive || len(p.Candidates) == 0 {
			t.Fatalf("wrong primary-window authority or population: system=%v %+v", record.SystemSupplement, p)
		}
		for _, candidate := range p.Candidates {
			if candidate.OwnerID == 900 {
				t.Fatal("PID 900 at the right endpoint entered the primary-window members")
			}
		}
	}
	if !primarySeen || !originalSeen {
		t.Fatalf("actual finalizer ledger lost a query: primary=%v original=%v", primarySeen, originalSeen)
	}
	navigation := renderAnswerDocRenderingCandidates(ctx, ledger)
	for _, want := range []string{"### 渲染框架与线程角色候选", "仅供后续排查导航", "ArkUI", "Flutter", "PID=600"} {
		if !strings.Contains(navigation, want) {
			t.Errorf("primary-window navigation lost %q: %s", want, navigation)
		}
	}
	if strings.Contains(navigation, "PID=900") || strings.Count(navigation, "查询记录=") != 1 {
		t.Fatalf("primary navigation borrowed the wider query: %s", navigation)
	}
	ledgerBefore, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if navigation == "" || !strings.Contains(instruction, navigation) {
		t.Fatal("actual finalizer instruction omitted the source-bound primary-window navigation")
	}
	ledgerAfter, _ := json.Marshal(answerDocObservationLedger(ctx))
	modelAfter, _ := json.Marshal([]any{bus.ToolResults, bus.Mutable.DispatchToolResults(), bus.Mutable.TurnAArtifacts()})
	if !bytes.Equal(ledgerBefore, ledgerAfter) || !bytes.Equal(modelBefore, modelAfter) {
		t.Fatal("supplement/finalizer rewrote the model exploration or original evidence")
	}
}
