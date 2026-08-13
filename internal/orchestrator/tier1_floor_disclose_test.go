package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// tier1_floor_disclose_test.go pins the §29.60 (2026-07-13) terminal contract
// of the pre-finalize floor arms plus the 件1/件2/件4 修复轮:
//   - an ACCEPTED model completion is terminal: DETECT → DISCLOSE, never
//     requeue (件4 completed direction);
//   - exit paths WITHOUT a completion signal keep the old bounded recovery
//     requeue (件4 no-completion direction, §29.21 CSP-RM F-1 / cmp_792);
//   - the disclosure speaks the firing arm's own truth (件1);
//   - the softened completion-caveat lanes reach the answer surface (件2).

// TestRunTaskGraph_Tier1FloorDetectionDisclosesInsteadOfRequeue (件4,
// completed direction): the model declared a typed completion, but the
// evidence is all GroundingRecovered so the Tier-1 ratio floor fails. The
// completion is terminal — no requeue, one finalize, typed floor-degraded
// disclosure with the ratio arm recorded.
func TestRunTaskGraph_Tier1FloorDetectionDisclosesInsteadOfRequeue(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0.5})
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	var explorerCalls, finalizeCalls int
	ir := dagIR(types.AnswerContract{Language: "en"})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			// The model's typed completion decision (production shape:
			// accepted by the emit_investigation_complete gates).
			ctx.Mutable.SetInvestigationComplete("模型已明确完成探索")
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{
					ID: "ev-recovered", Kind: types.EvidenceDirect,
					Source: "src.go", LineStart: 1,
					Subject: "Foo", Object: "bar",
					// Recovered-only grounding: Tier-1 ratio = 0 < 0.5 floor.
					GroundingStatus: types.GroundingRecovered,
				}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (src.go:1)",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(30)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !busCtx.TaskState.IsTerminal {
		t.Error("want terminal")
	}
	// Flip pin: ONE dispatch — the accepted-closure boundary auto-completes
	// the remaining windows, and the floor failure must NOT re-dispatch
	// exploration once the model completed.
	if explorerCalls != 1 {
		t.Errorf("explorer calls: want 1 (accepted closure, no floor requeue), got %d", explorerCalls)
	}
	if finalizeCalls != 1 {
		t.Errorf("finalize calls: want 1, got %d", finalizeCalls)
	}
	// Disclosure pin (件1): the typed termination profile carries the
	// degradation AND the firing arm.
	tp := busCtx.Mutable.TerminationProfile()
	if tp == nil || !tp.FloorDegraded {
		t.Fatalf("TerminationProfile.FloorDegraded = %+v, want degraded disclosure", tp)
	}
	if tp.FloorArm != types.TerminationFloorArmTier1Ratio {
		t.Fatalf("TerminationProfile.FloorArm = %q, want tier1_ratio", tp.FloorArm)
	}
}

// TestRunTaskGraph_Tier1FloorNoCompletionRecoversOnceThenDiscloses (件4,
// no-completion direction): the explorer exits WITHOUT any completion signal
// — §29.60 does not apply, so the old bounded recovery requeue runs once
// (RetryBudget=1 → one full re-collection pass), then the still-failing
// floor discloses and the answer ships.
func TestRunTaskGraph_Tier1FloorNoCompletionRecoversOnceThenDiscloses(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0.5})
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	var explorerCalls, finalizeCalls int
	ir := dagIR(types.AnswerContract{Language: "en"})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			// NO completion signal — ShouldStop/idle-style exit. Each call
			// emits a DISTINCT recovered-only item so the CGEC I4 stall
			// detector (identical fingerprints → hard stall) stays out of
			// the picture and the recovery lane is what gets exercised.
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{
					ID: fmt.Sprintf("ev-recovered-%d", explorerCalls), Kind: types.EvidenceDirect,
					Source: "src.go", LineStart: explorerCalls,
					GroundingStatus: types.GroundingRecovered,
				}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (src.go:1)",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(30)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !busCtx.TaskState.IsTerminal {
		t.Error("want terminal")
	}
	// Recovery pin: one bounded requeue re-runs the 3 explore windows
	// (RetryBudget=1), then the budget is exhausted and the run discloses.
	if explorerCalls != 6 {
		t.Errorf("explorer calls: want 6 (3 windows + one bounded recovery pass), got %d", explorerCalls)
	}
	if finalizeCalls != 1 {
		t.Errorf("finalize calls: want 1, got %d", finalizeCalls)
	}
	tp := busCtx.Mutable.TerminationProfile()
	if tp == nil || !tp.FloorDegraded {
		t.Fatalf("TerminationProfile = %+v, want floor-degraded disclosure after exhausted recovery", tp)
	}
}

// TestRunTaskGraph_StrictPolicyValidateShortCircuitsAfterAcceptedCompletion
// pins the §29.60 validate-SC soften: even under the opt-in strict policy, a
// validate node whose SuccessCriteria fail AFTER the model declared a typed
// completion routes through the inconclusive-injection escape (markDone)
// instead of requeueing the upstream evidence nodes.
func TestRunTaskGraph_StrictPolicyValidateShortCircuitsAfterAcceptedCompletion(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	// Capture the run's debug log: the short-circuit branch's INFO line is
	// the direct witness that the validate failure routed through the
	// accepted-completion escape (call counts alone are ambiguous — other
	// scheduler escapes can also stop at 3 dispatches).
	logDir := t.TempDir()
	lg, err := logging.NewFromFlags(logDir, "debug", false)
	if err != nil {
		t.Fatalf("logging: %v", err)
	}
	prevLg := logging.Default
	logging.SetDefault(lg)
	t.Cleanup(func() { logging.SetDefault(prevLg) })

	var explorerCalls, finalizeCalls int
	ir := dagIR(types.AnswerContract{Language: "en"})
	// A validate criterion that deterministically fails on this run's env
	// (no citations exist), so the validate node reaches the requeue branch.
	ir.TaskGraph.Nodes[2].SuccessCriteria = []types.Criterion{
		{Kind: types.CritCitationCountGE, Expr: "99"},
	}
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			// The model's typed completion decision (as accepted by the
			// emit_investigation_complete gates in production).
			ctx.Mutable.SetInvestigationComplete("模型已明确完成探索")
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{
					ID: "ev", Kind: types.EvidenceDirect, Source: "src.go", LineStart: 1,
					GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
				}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (src.go:1)",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{
		Agent: types.AgentSettings{InvestigationCompletePolicy: types.ICPolicyStrict},
	}, ar, sr, sar)
	o.SetMaxSteps(30)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !busCtx.TaskState.IsTerminal {
		t.Error("want terminal")
	}
	// Flip pin: the failed validate SC must not re-open evidence collection
	// once the model has declared completion — 3 windows, no
	// validation-feedback requeue.
	if explorerCalls != 3 {
		t.Errorf("explorer calls: want 3 (validate short-circuit, no feedback requeue), got %d", explorerCalls)
	}
	if finalizeCalls != 1 {
		t.Errorf("finalize calls: want 1, got %d", finalizeCalls)
	}
	// Direct witness: the accepted-completion short-circuit branch ran for
	// the failed validate node.
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}
	shortCircuited := false
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(logDir, e.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "short-circuited after accepted completion") {
			shortCircuited = true
		}
	}
	if !shortCircuited {
		t.Fatalf("expected the validate short-circuit witness line in the run log")
	}
}

// TestDegradedTerminationSystemCaveat_ArmSpecificWording (件1): the
// disclosure speaks the firing arm's own truth — the follow-up arm never
// measured a ratio, so it must not claim one; the ratio arm keeps the
// established wording byte-stable (legacy empty arm included).
func TestDegradedTerminationSystemCaveat_ArmSpecificWording(t *testing.T) {
	run := func(lang string, arm types.TerminationFloorArm) string {
		mu := types.NewMutableState("q")
		mu.MarkTerminationFloorDegradedArm(arm, "detail")
		o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: lang}}
		return degradedTerminationSystemCaveat(o)
	}

	// Follow-up arm: no ratio claim; says the follow-up steps were skipped.
	zhFollowup := run("zh", types.TerminationFloorArmFollowupCoverage)
	if strings.Contains(zhFollowup, "比例") || !strings.Contains(zhFollowup, "未执行") {
		t.Fatalf("zh follow-up arm must not claim a ratio and must name unexecuted steps, got: %q", zhFollowup)
	}
	enFollowup := run("en", types.TerminationFloorArmFollowupCoverage)
	if strings.Contains(enFollowup, "ratio") || !strings.Contains(enFollowup, "not executed") {
		t.Fatalf("en follow-up arm must not claim a ratio, got: %q", enFollowup)
	}

	// Ratio arm and legacy empty arm: established ratio wording.
	for _, arm := range []types.TerminationFloorArm{types.TerminationFloorArmTier1Ratio, ""} {
		zh := run("zh", arm)
		if !strings.Contains(zh, "已核实证据的比例低于配置的最低标准") {
			t.Fatalf("zh ratio wording missing for arm %q: %q", arm, zh)
		}
		en := run("en", arm)
		if !strings.Contains(en, "verified-evidence ratio below the configured floor") {
			t.Fatalf("en ratio wording missing for arm %q: %q", arm, en)
		}
	}
}

// TestCompletionCaveatLanesReachAnswerSurface (件2): the three softened
// completion-gate lanes render through the appendSystemCaveatsToAnswer
// chokepoint — detect→disclose is only real when the caveat reaches the
// user-facing answer. Unrelated lanes must not leak a disclosure.
func TestCompletionCaveatLanesReachAnswerSurface(t *testing.T) {
	mu := types.NewMutableState("q")
	closure := mu.EvidenceClosure()
	closure.AppendCompletionCaveat(types.CompletionCaveat{
		Lane: types.DowngradeLaneWakeupChainDrilldown, ReasonCode: "r1", Reason: "x",
	})
	closure.AppendCompletionCaveat(types.CompletionCaveat{
		Lane: types.DowngradeLaneExactResolvedDefiningProof, ReasonCode: "r2", Reason: "x",
	})
	closure.AppendCompletionCaveat(types.CompletionCaveat{
		Lane: types.DowngradeLaneForcedReadCoverage, ReasonCode: "r3", Reason: "x",
	})
	// Control: a convergence-lane caveat has its own surfaces and must not
	// render here.
	closure.AppendCompletionCaveat(types.CompletionCaveat{
		Lane: types.DowngradeLaneCompletionForm, ReasonCode: "r4", Reason: "x",
	})

	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "zh"}}
	answer := o.appendSystemCaveatsToAnswer("正文")
	for _, want := range []string{"未定位到上游唤醒者", "定义位置", "未读取"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer missing lane disclosure %q, got: %q", want, answer)
		}
	}
	if strings.Count(answer, "未定位到上游唤醒者") != 1 {
		t.Fatalf("lane disclosure must render exactly once, got: %q", answer)
	}

	// English rendering of the same lanes.
	o2 := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "en"}}
	answerEN := o2.appendSystemCaveatsToAnswer("body")
	for _, want := range []string{"upstream waker", "defining location", "were not read"} {
		if !strings.Contains(answerEN, want) {
			t.Fatalf("en answer missing lane disclosure %q, got: %q", want, answerEN)
		}
	}

	// Idempotent replay register: appending twice must not duplicate.
	again := o.appendSystemCaveatsToAnswer(answer)
	if strings.Count(again, "未定位到上游唤醒者") != 1 {
		t.Fatalf("replay register must keep the disclosure single, got: %q", again)
	}
}

func TestBoundedRuntimeFactSuppressesOnlyCausalCoverageDebt(t *testing.T) {
	mu := types.NewMutableState("bounded runtime fact")
	mu.MarkTerminationFloorDegradedArm(types.TerminationFloorArmFollowupCoverage, "unused causal drill")
	closure := mu.EvidenceClosure()
	closure.AppendCompletionCaveat(types.CompletionCaveat{
		Lane: types.DowngradeLaneWakeupChainDrilldown, ReasonCode: "wakeup", Reason: "not requested",
	})
	closure.AppendCompletionCaveat(types.CompletionCaveat{
		Lane: types.DowngradeLaneExactResolvedDefiningProof, ReasonCode: "definition", Reason: "control",
	})
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, PID: 59566, Thread: "app-59566", Source: "user_explicit",
		}},
	}}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, AnalysisIR: ir, Language: "zh"}}
	if got := degradedTerminationSystemCaveat(o); got != "" {
		t.Fatalf("bounded fact leaked generic follow-up debt: %q", got)
	}
	caveats := completionCaveatLaneSystemCaveats(o)
	joined := strings.Join(caveats, "\n")
	if strings.Contains(joined, "唤醒者") || !strings.Contains(joined, "定义位置") {
		t.Fatalf("bounded fact must suppress only causal drill debt: %q", joined)
	}

	start, end := 10.0, 10.1
	ir.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "10.0..10.1",
	}
	if got := degradedTerminationSystemCaveat(o); got != "" {
		t.Fatalf("explicit-window bounded facts must not inherit causal follow-up disclosure: %q", got)
	}
	if got := strings.Join(completionCaveatLaneSystemCaveats(o), "\n"); strings.Contains(got, "唤醒者") {
		t.Fatalf("explicit-window bounded facts must suppress wakeup-drill disclosure: %q", got)
	}
}
