package agent

import (
	agentctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// PromptAssembler is the extension point BaseAgent uses to turn an
// AgentContext + Skill config into the initial message list for the
// ReAct loop. It is split into TWO halves (AssembleContext and
// RenderMessages) rather than a single "BuildInitialMessages" because
// each half is independently useful:
//
//   - AssembleContext produces a structured PromptContext that
//     tests and diagnostics can inspect without committing to a
//     particular message encoding. Prompt-shape regression tests
//     (see internal/context/builder_test.go and
//     internal/agent/analyzer_prompt_test.go) exercise this level.
//   - RenderMessages flattens the PromptContext into the llm.Message
//     slice the LLM client consumes. An alternate renderer (for
//     example one that emits OpenAI-style tool_choice hints or a
//     debug-tagged variant) can replace this half without touching
//     AssembleContext.
//
// The evaluator's per-dispatch dynamic supplement is intentionally
// NOT part of this interface — see AppendDynamicInstruction for the
// reason. Splitting the three steps (assemble / render / append)
// lets BaseAgent.buildInitialMessages stay a pure orchestration
// shell with no business logic of its own.
//
// A future test that wants to capture the rendered PromptContext for
// inspection can implement this interface and install it via
// Dependencies.PromptAssembler; the default path remains zero-alloc
// and free of indirection for production.
type PromptAssembler interface {
	AssembleContext(ctx *types.AgentContext, sk *skill.Config) *types.PromptContext
	RenderMessages(pc *types.PromptContext) []llm.Message
}

// defaultPromptAssembler is the canonical PromptAssembler: it
// delegates AssembleContext to internal/context.BuildPromptContext
// and RenderMessages to internal/context.ToMessages plus a
// context.Message → llm.Message conversion.
//
// Kept as an empty struct rather than a package-level var so a
// caller that takes a concrete value (as opposed to the interface)
// cannot accidentally mutate shared state.
type defaultPromptAssembler struct{}

// DefaultPromptAssembler returns the package's canonical
// PromptAssembler implementation. Every BaseAgent created via
// NewBaseAgent uses this unless Dependencies.PromptAssembler is
// overridden by a test or a specialized caller.
func DefaultPromptAssembler() PromptAssembler { return defaultPromptAssembler{} }

// AssembleContext wraps context.BuildPromptContext. The wrapper keeps
// the internal/context package a leaf dependency the agent package
// never tangles with outside of PromptAssembler and
// AppendDynamicInstruction, so a future refactor that splits or
// renames BuildPromptContext has exactly one call site to update
// here instead of N call sites inside BaseAgent.
func (defaultPromptAssembler) AssembleContext(ctx *types.AgentContext, sk *skill.Config) *types.PromptContext {
	return agentctx.BuildPromptContext(ctx, sk)
}

// RenderMessages flattens a PromptContext into the llm.Message slice
// BaseAgent hands to the LLM client. A nil PromptContext yields a
// nil result so the caller does not need a defensive branch.
//
// The context.Message → llm.Message conversion is a plain struct
// copy today, but keeping it behind this method means a future
// addition to llm.Message (tool-call metadata, token budgets, ...)
// does not force an edit to every caller of buildInitialMessages.
func (defaultPromptAssembler) RenderMessages(pc *types.PromptContext) []llm.Message {
	if pc == nil {
		return nil
	}
	ctxMsgs := agentctx.ToMessages(pc)
	if len(ctxMsgs) == 0 {
		return nil
	}
	out := make([]llm.Message, 0, len(ctxMsgs))
	for _, m := range ctxMsgs {
		out = append(out, llm.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	return out
}

// AppendDynamicInstruction appends the evaluator's per-dispatch
// supplement — when non-empty — as a user-role message after the
// assembler's output. Three design notes:
//
//  1. Exists as a top-level function rather than a PromptAssembler
//     method because the supplement comes from the Evaluator, not the
//     assembler. Folding it into PromptAssembler would force every
//     assembler implementation to also know about evaluators, which
//     cross-wires two orthogonal extension points.
//  2. An empty instruction is dropped, not appended as an empty user
//     message. The analyzer (see analyzer_prompt_test.go) relies on
//     this: its BuildInitialInstruction returns "" by design, and the LLM
//     must not see a bare user-role message with no content.
//  3. A nil evaluator is a no-op so tests that only care about the
//     assemble+render pipeline can skip the dynamic step by passing
//     nil. Production code always has a non-nil evaluator.
//
// The function returns the (possibly mutated) slice so the caller
// can chain it in a single assignment.
func AppendDynamicInstruction(messages []llm.Message, eval Evaluator, ctx *types.AgentContext, sk *skill.Config) []llm.Message {
	if eval == nil {
		return messages
	}
	instruction := eval.BuildInitialInstruction(ctx, sk)
	if instruction == "" {
		return messages
	}
	return append(messages, llm.Message{
		Role:    "user",
		Content: instruction,
	})
}
