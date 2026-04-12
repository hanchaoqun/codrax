package agent

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzerEvaluator customizes the ReAct loop for the analyzer agent,
// which is responsible only for the analyze stage: classify the user
// request and produce the initial task list.
//
// The analyzer drives task list creation through the todo_write tool
// (see internal/tool/todo_write.go) — it does not parse the LLM's
// free-form output anymore. The tool mutates BusContext.Mutable
// directly, so by the time ParseOutput runs the working task list is
// already in place. ParseOutput's only remaining job is to capture
// the assistant's natural-language summary of what it decided.
type analyzerEvaluator struct{}

func (e *analyzerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	// The analyzer is the first LLM in the pipeline to see the raw
	// user request, so it has strictly more context than any downstream
	// post-hoc extractor. Every field it writes via todo_write feeds a
	// deterministic consumer:
	//
	//   keywords   -> keywordSearch / grep IDF ranking
	//   entities   -> ERM entity set (explorer.go prefers this over regex)
	//   question_kind -> ERM predicate whitelist (skips keyword inference)
	//   answer_shape -> finalizer anti-hallucination constraint
	//
	// Vague output here (generic keywords, missing entities, wrong
	// kind) cascades into wasted breadth scans, spurious ERM requirements,
	// and hallucinated final answers. The prompt is therefore written
	// as an explicit contract.
	return `You decompose the user request into a working task list by calling todo_write.
Your output feeds a deterministic evidence pipeline — downstream stages rely
on the STRUCTURED fields below, not your free-form explanation.

# Required fields on every task

title        — short imperative label in the user's language
writing      — true only if the task may mutate files; leave false for
               any read/explain/audit request
high_risk    — true only when writing=true AND the change needs review
complexity   — "simple" (single lookup/count), "moderate" (single-component
               explanation, the default), "complex" (cross-component flow)
keywords     — ≥8 search terms for grep. MUST include every CamelCase and
               snake_case identifier the user mentioned, PLUS conceptual
               synonyms. For Chinese questions, include BOTH Chinese and
               English forms (the codebase is English).
entities     — CamelCase/snake_case symbol names copied VERBATIM from the
               user's wording. Do NOT translate, pluralise, re-case, or
               paraphrase. Leave empty only if the question has no
               identifier-looking tokens. Generic nouns (count, function,
               thing, agent, handler, module) MUST NOT appear here — they
               poison ERM ranking.
question_kind — pick one:
   registration  — "which/how many X register/bind Y", "X 是在哪注册的"
   mechanism     — "how does X work", "explain the process of X", "X 怎么实现"
   return_value  — "what does X return", "X 的 Name() 是什么"
   conditional   — "when does X fire", "under what condition", "什么时候"
   config_mapping — "what does config key K control", key → behaviour
   enumeration   — "list all X", "count of X", pure set membership
   call_chain    — "which X calls Y", "从 A 到 B 怎么调用的"
   unknown       — genuinely ambiguous; ERM will fall back to keyword inference
answer_shape — pick one:
   list_of_symbols — answer is a set of identifier names (triggers
                     finalizer's anti-hallucination: forbids symbols
                     not present in Ground Truth evidence)
   step_list       — ordered steps of a mechanism
   value           — a single literal / returned value
   boolean         — yes/no
   config_value    — a resolved config key value
   none            — no structured shape applies

# Hard rules

1. entities come from the user's ORIGINAL text only. If the user wrote
   "ContinuationPrompt", put "ContinuationPrompt" — NOT "continuation
   prompt", NOT "continuation_prompt", NOT "prompt".
2. Do not invent a question_kind by stretching one of the categories.
   If two fit equally, choose the one that directly matches the
   user's verb. If none fit, use "unknown".
3. answer_shape=list_of_symbols ONLY when the user is asking for a
   SET of names they want to see listed. "How many agents call X" is
   list_of_symbols (they want the names even if phrased as a count).
   "Is X registered" is boolean. "Explain X" is step_list or none.
4. After calling todo_write, briefly explain your classification choices
   (question_kind and answer_shape) in one paragraph. This text is
   captured for the trace but does not drive any agent — the structured
   fields are what matter.`
}

func (e *analyzerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *analyzerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	// Extract the last assistant message as the human-readable summary.
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	// If the LLM never called todo_write, fall back to a single-item
	// analysis-typed task so the orchestrator still has something to
	// route on. The fallback uses the user's original request as the
	// task title and writes through Mutable so we go through the same
	// channel a tool call would.
	if ctx.Mutable != nil {
		if tl := ctx.Mutable.TaskList(); len(tl.Tasks) == 0 {
			// Fail-safe default: the user request as a single
			// non-writing, non-risky task. Picks the analysis policy
			// so the pipeline answers the user instead of guessing
			// code changes.
			tl.Tasks = []types.TaskItem{{
				ID:       "task-1",
				Title:    tl.Objective,
				Writing:  false,
				HighRisk: false,
				Status:   types.TaskPending,
			}}
			tl.CurrentTaskID = "task-1"
			ctx.Mutable.SetTaskList(tl)
		}
	}

	// Assemble a structured StageOutput.Data so the trace records the
	// analyzer's classification machine-readably (in addition to the
	// natural-language rationale). Downstream consumers and post-run
	// audits can compare the analyzer's declared question_kind against
	// ERM's eventual inference without grepping free text.
	data := map[string]any{
		"result": lastContent,
	}
	if ctx.Mutable != nil {
		tl := ctx.Mutable.TaskList()
		if current := tl.CurrentTask(); current != nil {
			data["question_kind"] = current.QuestionKind
			data["answer_shape"] = current.AnswerShape
			data["complexity"] = current.Complexity
			data["entity_count"] = len(current.Entities)
			data["keyword_count"] = len(current.Keywords)
		}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		// Marshal of a map[string]any with string/int values can only
		// fail on pathological content; fall back to the legacy shape.
		raw = json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent))
	}

	return &StageOutput{
		Data: raw,
	}, nil
}

func (e *analyzerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingFacts
}

// NewAnalyzerAgent creates the analyzer agent (used in the analyze stage).
func NewAnalyzerAgent(deps *Dependencies) Agent {
	eval := &analyzerEvaluator{}
	return NewBaseAgent(types.AgentAnalyzer, deps, eval)
}
