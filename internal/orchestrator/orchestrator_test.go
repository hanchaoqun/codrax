package orchestrator

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// This file hosts the mock agent + registry helpers the DAG tests
// (orchestrator_dag_test.go, two_turn_e2e_test.go,
// answer_document_e2e_test.go, apply_stage_output_dedup_test.go)
// share.

// ---------------------------------------------------------------------------
// Mock agent
// ---------------------------------------------------------------------------

type mockAgent struct {
	name   types.AgentName
	execFn func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error)
}

func (m *mockAgent) Name() types.AgentName { return m.name }

func (m *mockAgent) Execute(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sk)
	}
	return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
}

func newMockAgent(name types.AgentName, fn func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error)) *mockAgent {
	return &mockAgent{name: name, execFn: fn}
}

// buildRegistries creates agent and skill registries with mock
// entries matching the 4-stage read-only pipeline plus the
// log_triage pre-stage. Skill names match the hardcoded topology
// in topology.go. The log_triager mock is wired even for tests
// that do not set AttachedLog, so a test that flips on the
// attachment mid-setup does not trip on a missing agent lookup.
func buildRegistries(agentFns map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error)) (*agent.Registry, *skill.Registry, *agent.SubAgentRegistry) {
	ar := agent.NewRegistry()
	names := []types.AgentName{
		types.AgentLogTriager,
		types.AgentPerfTriager,
		types.AgentAnalyzer,
		types.AgentExplorer,
		types.AgentExtractor,
		types.AgentFinalizer,
		// B0 write-mode agents. Registered here so tests exercising
		// Mode=ModePlan / ModeApply / ModeVerify do not crash in
		// dispatchStage with an "agent not registered" error. Tests
		// that care about the write-mode behavior either supply an
		// agentFns entry to drive the mock or rely on the default
		// zero-value StageOutput (which runPlanPhase then interprets
		// as "no ChangePlan produced" and surfaces fail-loud).
		types.AgentWriteAnalyzer,
		types.AgentWriteController,
		types.AgentPlanner,
		types.AgentCoder,
		types.AgentVerifier,
	}
	for _, n := range names {
		var fn func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error)
		if agentFns != nil {
			fn = agentFns[n]
		}
		if fn == nil && n == types.AgentWriteController {
			fn = defaultTestWriteController
		}
		ar.Register(newMockAgent(n, fn))
	}

	sr := skill.NewRegistry()
	skillNames := []string{
		"log-triage-skill",
		"log-segmentation-skill",
		"analysis-skill",
		"explore-skill",
		"extract-skill",
		"answer-document-skill",
		// B0 write-mode skills (mirror topology.go entries).
		"write-analysis-skill",
		"write-controller-skill",
		"change-plan-skill",
		"code-write-skill",
		"test-execute-skill",
	}
	for _, s := range skillNames {
		sr.Register(&skill.Config{Name: s, Goal: s + " goal"})
	}

	sar := agent.NewSubAgentRegistry()

	return ar, sr, sar
}

func defaultTestWriteController(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
	decision := writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish, ReasonCode: "test_complete"}
	switch ctx.Mode {
	case types.ModePlan:
		if ctx.Mutable.ChangePlan() == nil {
			decision = writeflow.WriteWorkflowDecision{
				Action: writeflow.ActionPlanBatch,
				Batch:  &writeflow.WriteBatchPlan{ID: "batch-1", Goal: types.StripConversationPrefix(ctx.Mutable.Objective())},
			}
		}
	case types.ModeApply:
		switch {
		case ctx.Mutable.ChangePlan() == nil:
			decision = writeflow.WriteWorkflowDecision{
				Action: writeflow.ActionPlanBatch,
				Batch:  &writeflow.WriteBatchPlan{ID: "batch-1", Goal: types.StripConversationPrefix(ctx.Mutable.Objective())},
			}
		case ctx.Mutable.ChangeReport() != nil && ctx.Mutable.ChangeReport().Passed:
			decision = writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish, ReasonCode: "test_complete"}
		case ctx.Mutable.ChangeReport() != nil:
			decision = writeflow.WriteWorkflowDecision{
				Action: writeflow.ActionReplanBatch,
				Batch:  &writeflow.WriteBatchPlan{ID: "batch-1", Goal: types.StripConversationPrefix(ctx.Mutable.Objective())},
			}
		case len(ctx.Mutable.WriteClosure().AppliedSet()) > 0:
			decision = writeflow.WriteWorkflowDecision{Action: writeflow.ActionVerifyBatch, ReasonCode: "test_verify"}
		default:
			decision = writeflow.WriteWorkflowDecision{Action: writeflow.ActionApplyPlan, ReasonCode: "test_apply"}
		}
	case types.ModeVerify:
		if ctx.Mutable.ChangeReport() == nil {
			decision = writeflow.WriteWorkflowDecision{Action: writeflow.ActionVerifyBatch, ReasonCode: "test_verify"}
		}
	}
	raw, err := json.Marshal(decision)
	if err != nil {
		return nil, err
	}
	return &agent.StageOutput{MissingPiece: types.MissingNone, Data: raw}, nil
}
