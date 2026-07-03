package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// prompt_assembler_test.go pins the three-way split of the prompt
// initialization pipeline. The old single buildInitialMessages method
// mixed three concerns (PromptContext construction, context.Message
// → llm.Message conversion, evaluator supplement append); this file
// tests each primitive in isolation plus the orchestration shell on
// top of them.
//
// Test matrix:
//
//   - defaultPromptAssembler.AssembleContext — forwards to the
//     context package, delegation-only.
//   - defaultPromptAssembler.RenderMessages — nil/empty handling,
//     context.Message → llm.Message conversion fidelity.
//   - AppendDynamicInstruction — empty supplement, non-empty
//     supplement, nil evaluator.
//   - BaseAgent.buildInitialMessages — end-to-end orchestration
//     against a mock PromptAssembler + mock Evaluator so the test
//     proves "buildInitialMessages has no business logic of its own".
//
// The mocks are local to this file so they never leak into
// production code and stay narrow (exactly the methods the three-
// way split needs).

// -----------------------------------------------------------------------------
// Mocks
// -----------------------------------------------------------------------------

// recordingAssembler captures every input to AssembleContext and
// RenderMessages and returns preset outputs. It implements the
// PromptAssembler interface so it can be injected via
// Dependencies.PromptAssembler.
type recordingAssembler struct {
	// inputs seen by the two methods — populated on each call so a
	// test can assert call order and argument identity.
	assembleCtx   *types.AgentContext
	assembleSkill *skill.Config
	renderContext *types.PromptContext
	assembleCalls int
	renderCalls   int

	// outputs to return. nilPromptContext and nilMessages let a test
	// exercise the "empty pipeline" branches without threading a
	// nil check through buildInitialMessages.
	returnPrompt   *types.PromptContext
	returnMessages []llm.Message
}

func (r *recordingAssembler) AssembleContext(ctx *types.AgentContext, sk *skill.Config) *types.PromptContext {
	r.assembleCtx = ctx
	r.assembleSkill = sk
	r.assembleCalls++
	return r.returnPrompt
}

func (r *recordingAssembler) RenderMessages(pc *types.PromptContext) []llm.Message {
	r.renderContext = pc
	r.renderCalls++
	return append([]llm.Message(nil), r.returnMessages...)
}

// recordingEvaluator is a zero-logic Evaluator that returns a fixed
// BuildInitialInstruction string and no-ops for the other three methods.
// buildInitialMessages only calls BuildInitialInstruction, so the other
// three methods are pure stubs.
type recordingEvaluator struct {
	instruction string
	gotCtx      *types.AgentContext
	gotSkill    *skill.Config
	calls       int
}

func (r *recordingEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
	r.gotCtx = ctx
	r.gotSkill = sk
	r.calls++
	return r.instruction
}
func (r *recordingEvaluator) ShouldStop(llm.Response, int) bool { return true }
func (r *recordingEvaluator) ParseOutput(*types.AgentContext, []llm.Message, []types.ToolResult, []types.MCPResponse) (*StageOutput, error) {
	return nil, nil
}
func (r *recordingEvaluator) DetermineMissingPiece(*types.AgentContext, *StageOutput) types.MissingPiece {
	return types.MissingNone
}

// -----------------------------------------------------------------------------
// defaultPromptAssembler tests
// -----------------------------------------------------------------------------

// TestDefaultPromptAssembler_AssembleContext verifies the default
// implementation delegates to the context package and returns a
// non-nil PromptContext for a minimally populated AgentContext +
// Skill. Asserts the structural forwarding without re-testing every
// BuildPromptContext branch (those live in
// internal/context/builder_test.go).
func TestDefaultPromptAssembler_AssembleContext(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "test the default assembler",
	}
	sk := skill.BuildAnalysisSkill()

	pc := DefaultPromptAssembler().AssembleContext(ac, sk)

	if pc == nil {
		t.Fatal("AssembleContext returned nil PromptContext")
	}
	if pc.AgentName != types.AgentAnalyzer {
		t.Errorf("PromptContext.AgentName = %q, want analyzer", pc.AgentName)
	}
	if pc.SkillName != "analysis-skill" {
		t.Errorf("PromptContext.SkillName = %q, want analysis-skill", pc.SkillName)
	}
}

// TestDefaultPromptAssembler_RenderMessages_NilContext asserts the
// nil-safe branch: passing a nil PromptContext must yield a nil
// slice so the caller needs no defensive branch of its own.
func TestDefaultPromptAssembler_RenderMessages_NilContext(t *testing.T) {
	msgs := DefaultPromptAssembler().RenderMessages(nil)
	if msgs != nil {
		t.Errorf("RenderMessages(nil) = %v, want nil", msgs)
	}
}

// TestDefaultPromptAssembler_RenderMessages_RoundTrip checks the
// context.Message → llm.Message conversion preserves Role and
// Content for both the system and user sections. The test uses the
// real context package's BuildPromptContext so it proves the whole
// default pipeline works together.
func TestDefaultPromptAssembler_RenderMessages_RoundTrip(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "UNIQUE_RENDER_PIN",
	}
	sk := &skill.Config{
		Name:         "test-skill",
		Goal:         "test goal",
		Workflow:     []string{"step one"},
		OutputFormat: "output shape",
	}
	pa := DefaultPromptAssembler()

	pc := pa.AssembleContext(ac, sk)
	msgs := pa.RenderMessages(pc)

	if len(msgs) < 2 {
		t.Fatalf("expected system + user messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("msgs[0].Role = %q, want system", msgs[0].Role)
	}
	if msgs[1].Role != "user" {
		t.Errorf("msgs[1].Role = %q, want user", msgs[1].Role)
	}
	if !strings.Contains(msgs[1].Content, "UNIQUE_RENDER_PIN") {
		t.Errorf("user message did not carry objective sentinel, got: %s", msgs[1].Content)
	}
}

// -----------------------------------------------------------------------------
// AppendDynamicInstruction tests
// -----------------------------------------------------------------------------

// TestAppendDynamicInstruction_EmptyInstructionIsDropped pins the
// contract that an empty evaluator supplement does NOT append a
// bare user-role message. The analyzer relies on this: its
// BuildInitialInstruction returns "" by design.
func TestAppendDynamicInstruction_EmptyInstructionIsDropped(t *testing.T) {
	eval := &recordingEvaluator{instruction: ""}
	base := []llm.Message{{Role: "system", Content: "base"}}

	out := AppendDynamicInstruction(base, eval, &types.AgentContext{}, nil)

	if len(out) != 1 {
		t.Fatalf("empty supplement must not append, got %d messages", len(out))
	}
	if eval.calls != 1 {
		t.Errorf("BuildInitialInstruction should have been called exactly once, got %d", eval.calls)
	}
}

// TestAppendDynamicInstruction_NonEmptyInstructionAppends asserts a
// non-empty supplement lands as a user-role message AFTER the
// existing ones, preserving the base slice's order.
func TestAppendDynamicInstruction_NonEmptyInstructionAppends(t *testing.T) {
	eval := &recordingEvaluator{instruction: "dynamic supplement body"}
	base := []llm.Message{
		{Role: "system", Content: "system block"},
		{Role: "user", Content: "user block"},
	}

	out := AppendDynamicInstruction(base, eval, &types.AgentContext{}, nil)

	if len(out) != 3 {
		t.Fatalf("non-empty supplement should append one message, got %d", len(out))
	}
	if out[2].Role != "user" {
		t.Errorf("supplement role = %q, want user", out[2].Role)
	}
	if out[2].Content != "dynamic supplement body" {
		t.Errorf("supplement content = %q, want 'dynamic supplement body'", out[2].Content)
	}
	// Order of prior messages must be preserved.
	if out[0].Content != "system block" || out[1].Content != "user block" {
		t.Errorf("prior messages mutated: %+v", out[:2])
	}
}

// TestAppendDynamicInstruction_NilEvaluator is a defensive branch for
// tests (and a sanity check for future callers): passing nil must be
// a no-op so a caller that wants to skip the supplement step can do
// so without threading a conditional.
func TestAppendDynamicInstruction_NilEvaluator(t *testing.T) {
	base := []llm.Message{{Role: "system", Content: "hi"}}
	out := AppendDynamicInstruction(base, nil, nil, nil)
	if len(out) != 1 {
		t.Errorf("nil evaluator must be a no-op, got %d messages", len(out))
	}
}

// TestAppendDynamicInstruction_NilBaseWithInstruction covers the
// edge where the base slice is nil and the evaluator contributes an
// instruction. The result must be a non-nil one-element slice so
// the downstream LLM caller never sees "no messages".
func TestAppendDynamicInstruction_NilBaseWithInstruction(t *testing.T) {
	eval := &recordingEvaluator{instruction: "from nothing"}

	out := AppendDynamicInstruction(nil, eval, nil, nil)

	if len(out) != 1 {
		t.Fatalf("nil base + supplement should yield 1 message, got %d", len(out))
	}
	if out[0].Role != "user" || out[0].Content != "from nothing" {
		t.Errorf("result[0] = %+v, want user/'from nothing'", out[0])
	}
}

// -----------------------------------------------------------------------------
// BaseAgent.buildInitialMessages orchestration tests
// -----------------------------------------------------------------------------

// TestBuildInitialMessages_DefaultAssemblerInstalled pins the
// NewBaseAgent wiring: a Dependencies value with a nil
// PromptAssembler must still produce a BaseAgent that holds the
// default implementation. This catches a regression where a future
// constructor change forgets the "nil means default" branch.
func TestBuildInitialMessages_DefaultAssemblerInstalled(t *testing.T) {
	base := NewBaseAgent(types.AgentAnalyzer, &Dependencies{}, &recordingEvaluator{})
	if base.deps.PromptAssembler == nil {
		t.Fatal("NewBaseAgent must install DefaultPromptAssembler when deps.PromptAssembler is nil")
	}
	// The default implementation is an empty struct — a sanity check
	// is enough to confirm we got *a* non-nil value.
}

// TestBuildInitialMessages_CallsPrimitivesInOrder injects a
// recording assembler and a recording evaluator, then asserts
// buildInitialMessages calls them in the prescribed order and
// returns the exact concatenation. This is the central guard for
// the orchestration contract: if buildInitialMessages ever grows
// business logic of its own, the comparison against the expected
// output will start drifting.
func TestBuildInitialMessages_CallsPrimitivesInOrder(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "test orchestration",
	}
	sk := &skill.Config{Name: "test-skill"}

	// The assembler returns three canned messages with distinctive
	// content so the test can spot any reordering or mutation.
	rec := &recordingAssembler{
		returnPrompt: &types.PromptContext{SkillName: "test-skill"},
		returnMessages: []llm.Message{
			{Role: "system", Content: "A-system"},
			{Role: "user", Content: "B-user"},
		},
	}
	evalRec := &recordingEvaluator{instruction: "C-dynamic"}

	base := NewBaseAgent(types.AgentAnalyzer, &Dependencies{PromptAssembler: rec}, evalRec)

	msgs := base.buildInitialMessages(ac, sk)

	// 1. Assemble step ran exactly once with the exact arguments.
	if rec.assembleCalls != 1 {
		t.Errorf("AssembleContext called %d times, want 1", rec.assembleCalls)
	}
	if rec.assembleCtx != ac {
		t.Errorf("AssembleContext got ctx %p, want %p", rec.assembleCtx, ac)
	}
	if rec.assembleSkill != sk {
		t.Errorf("AssembleContext got skill %p, want %p", rec.assembleSkill, sk)
	}

	// 2. Render step ran exactly once and received the PromptContext
	// produced by step 1 — not a fresh build.
	if rec.renderCalls != 1 {
		t.Errorf("RenderMessages called %d times, want 1", rec.renderCalls)
	}
	if rec.renderContext != rec.returnPrompt {
		t.Error("RenderMessages did not receive the PromptContext produced by AssembleContext")
	}

	// 3. Evaluator supplement ran with the same ctx + skill.
	if evalRec.calls != 1 {
		t.Errorf("BuildInitialInstruction called %d times, want 1", evalRec.calls)
	}
	if evalRec.gotCtx != ac || evalRec.gotSkill != sk {
		t.Error("BuildInitialInstruction did not receive the same ctx/skill as the assembler")
	}

	// 4. Output equals [assembler messages..., dynamic supplement].
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3 (2 assembler + 1 supplement)", len(msgs))
	}
	if msgs[0].Content != "A-system" || msgs[1].Content != "B-user" || msgs[2].Content != "C-dynamic" {
		t.Errorf("message order drifted: %+v", msgs)
	}
	if msgs[2].Role != "user" {
		t.Errorf("supplement role = %q, want user", msgs[2].Role)
	}
}

// TestBuildInitialMessages_EmptySupplementDropsAppend wires a
// recording evaluator whose BuildInitialInstruction returns "" and
// asserts buildInitialMessages returns only the assembler output.
// This pins the analyzer's empty-supplement path end-to-end.
func TestBuildInitialMessages_EmptySupplementDropsAppend(t *testing.T) {
	rec := &recordingAssembler{
		returnPrompt: &types.PromptContext{},
		returnMessages: []llm.Message{
			{Role: "system", Content: "only this"},
		},
	}
	evalRec := &recordingEvaluator{instruction: ""}

	base := NewBaseAgent(types.AgentAnalyzer, &Dependencies{PromptAssembler: rec}, evalRec)
	msgs := base.buildInitialMessages(&types.AgentContext{}, &skill.Config{})

	if len(msgs) != 1 {
		t.Fatalf("empty supplement should leave the assembler output unchanged, got %d messages", len(msgs))
	}
	if msgs[0].Content != "only this" {
		t.Errorf("assembler message mutated: %+v", msgs[0])
	}
	// The evaluator was still consulted — dropping the supplement is
	// a post-consultation decision, not a skip-the-call optimization.
	if evalRec.calls != 1 {
		t.Errorf("BuildInitialInstruction calls = %d, want 1 (empty supplement is still a consultation)", evalRec.calls)
	}
}

// TestBuildInitialMessages_NilAssemblerOutputIsHandled covers the
// edge where the assembler returns a nil PromptContext (and the
// downstream RenderMessages call therefore returns nil too). The
// orchestration shell must still call the evaluator and, if it
// emits a supplement, return a one-element slice — never nil.
func TestBuildInitialMessages_NilAssemblerOutputIsHandled(t *testing.T) {
	rec := &recordingAssembler{
		returnPrompt:   nil,
		returnMessages: nil,
	}
	evalRec := &recordingEvaluator{instruction: "still there"}

	base := NewBaseAgent(types.AgentAnalyzer, &Dependencies{PromptAssembler: rec}, evalRec)
	msgs := base.buildInitialMessages(&types.AgentContext{}, &skill.Config{})

	if len(msgs) != 1 {
		t.Fatalf("nil assembler output + non-empty supplement should yield 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "still there" {
		t.Errorf("supplement not surfaced: %+v", msgs[0])
	}
}
