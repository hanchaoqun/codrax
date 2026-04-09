package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

// analyzerEvaluator customizes the ReAct loop for the analyzer agent,
// which is responsible only for the analyze stage: classify the user
// request and produce the initial task list. It used to live inside
// plannerEvaluator behind an `if ctx.Stage == StageAnalyze` branch;
// splitting it out gives every stage a 1:1 agent mapping.
type analyzerEvaluator struct{}

func (e *analyzerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	return "Please follow the workflow steps and produce output in the specified format."
}

func (e *analyzerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *analyzerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	// Extract the last assistant message as output
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}
	return &StageOutput{
		Data:           json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
		TaskListUpdate: parseAnalyzeOutput(lastContent),
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

// parseAnalyzeOutput tries to extract a TaskList from the analyzer's
// LLM response. The skill's OutputFormat instructs the LLM to emit a
// JSON document whose field names match types.TaskList directly, so
// the parser can unmarshal in one shot.
//
// Any parse failure (missing JSON, malformed JSON, empty task list)
// returns a safe default that classifies the request as analysis —
// fail-safe means "answer the user" rather than "start mutating code".
func parseAnalyzeOutput(raw string) *types.TaskList {
	payload, ok := extractJSONObject(raw)
	if !ok {
		return defaultAnalysisTaskList()
	}

	var tl types.TaskList
	if err := json.Unmarshal([]byte(payload), &tl); err != nil {
		return defaultAnalysisTaskList()
	}
	if len(tl.Tasks) == 0 {
		return defaultAnalysisTaskList()
	}

	// Normalize: assign synthetic IDs where missing, map task type
	// synonyms / unknown values to a canonical TaskType, force Status
	// to Pending, and pick the first task as current.
	for i := range tl.Tasks {
		if tl.Tasks[i].ID == "" {
			tl.Tasks[i].ID = fmt.Sprintf("task-%d", i+1)
		}
		tl.Tasks[i].Type = normalizeTaskType(tl.Tasks[i].Type)
		tl.Tasks[i].Status = types.TaskPending
	}
	tl.CurrentTaskID = tl.Tasks[0].ID

	return &tl
}

// extractJSONObject pulls the first balanced JSON object out of an LLM
// response, tolerating ```json fences, leading prose, and trailing prose.
func extractJSONObject(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	// Strip ```json … ``` or ``` … ``` fences
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return "", false
	}
	return s[start : end+1], true
}

// normalizeTaskType maps LLM-produced strings to one of the two
// canonical TaskType values, accepting common synonyms. Anything that
// isn't clearly an implementation request lands in Analysis (fail-safe
// direction: prefer answering the user over editing code).
func normalizeTaskType(t types.TaskType) types.TaskType {
	switch strings.ToLower(strings.TrimSpace(string(t))) {
	case "implementation", "implement", "code", "feature", "fix", "bug":
		return types.TaskTypeImplementation
	default:
		return types.TaskTypeAnalysis
	}
}

// defaultAnalysisTaskList is the fail-safe TaskList used when the LLM
// output cannot be parsed. It declares a single Analysis task so the
// orchestrator picks the analysis policy and the pipeline tries to
// answer the user instead of starting an implementation.
func defaultAnalysisTaskList() *types.TaskList {
	return &types.TaskList{
		Tasks: []types.TaskItem{{
			ID:     "task-1",
			Title:  "user request",
			Type:   types.TaskTypeAnalysis,
			Status: types.TaskPending,
		}},
		CurrentTaskID: "task-1",
	}
}
