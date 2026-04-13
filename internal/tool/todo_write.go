package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TodoWrite is the agent-facing tool for updating the working
// task list. It writes directly to BusContext.Mutable, which is the
// only region of pipeline state tools are allowed to mutate during
// the ReAct loop.
//
// API: full-list replacement. The caller must send the complete
// desired state every call; the tool does not merge with the existing
// list. This matches Claude Code's TodoWrite convention and avoids
// merge-logic complexity.
//
// Classified ReadOnly because IsWrite() is the filesystem-write
// boundary; mutating BusContext is not a filesystem write.
type TodoWrite struct {
	ReadOnly
	NonEvidenceTool
}

type todoWriteParams struct {
	Tasks           []todoWriteTaskParam       `json:"tasks"`
	Edges           []types.TaskGraphEdge      `json:"edges,omitempty"`
	ExecutionPolicy *types.TaskExecutionPolicy `json:"execution_policy,omitempty"`
}

type todoWriteTaskParam struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	Writing         bool     `json:"writing,omitempty"`
	HighRisk        bool     `json:"high_risk,omitempty"`
	Complexity      string   `json:"complexity,omitempty"`
	Keywords        []string `json:"keywords,omitempty"`
	Entities        []string `json:"entities,omitempty"`
	QuestionKind    string   `json:"question_kind,omitempty"`
	AnswerShape     string   `json:"answer_shape,omitempty"`
	NodeType        string   `json:"type,omitempty"`
	Objective       string   `json:"objective,omitempty"`
	Inputs          []string `json:"inputs,omitempty"`
	Outputs         []string `json:"outputs,omitempty"`
	SuccessCriteria []string `json:"success_criteria,omitempty"`
	EntryConditions []string `json:"entry_conditions,omitempty"`
	ExitArtifacts   []string `json:"exit_artifacts,omitempty"`
	Status          string   `json:"status,omitempty"`
}

func (t *TodoWrite) Name() string { return "todo_write" }

func (t *TodoWrite) Description() string {
	return "Update the working todolist that tracks progress on the user's request. " +
		"Pass the complete list of tasks every call — this tool replaces the existing " +
		"list, it does not merge. Use it to record initial decomposition, mark items " +
		"in_progress as you start them, and mark items done as you finish."
}

func (t *TodoWrite) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "tasks": {
      "type": "array",
      "description": "Complete desired state of the todolist (full replacement, not merge)",
      "items": {
        "type": "object",
        "properties": {
          "id":          {"type": "string", "description": "Stable identifier; auto-assigned if missing"},
          "title":       {"type": "string", "description": "Short user-facing label"},
          "description": {"type": "string", "description": "Optional details"},
          "writing":     {"type": "boolean", "description": "True if this task may mutate files (picks an implementation-class policy)"},
          "high_risk":   {"type": "boolean", "description": "True if this task needs design/code review (only meaningful when writing is true)"},
          "complexity":  {"type": "string", "enum": ["simple", "moderate", "complex"], "description": "Investigation depth: simple (lookup/count), moderate (single-component), complex (cross-component architecture). Defaults to moderate if omitted."},
          "keywords":    {"type": "array", "items": {"type": "string"}, "description": "Search terms for the explorer to grep. Include CamelCase symbols, snake_case identifiers, and conceptual synonyms. Minimum 8 keywords."},
          "entities":    {"type": "array", "items": {"type": "string"}, "description": "CamelCase/snake_case symbol names copied VERBATIM from the user's original wording. Do NOT translate, re-case, or paraphrase. These drive ERM entity extraction for the explorer; adding generic English nouns (e.g. 'count','that','function') has caused ranking regressions. Leave empty only if the user's question contains no identifier-looking tokens."},
          "question_kind": {"type": "string", "enum": ["registration","mechanism","return_value","conditional","config_mapping","enumeration","call_chain","unknown"], "description": "Evidence-requirement shape. registration = 'which/how many X register/bind Y'. mechanism = 'how does X work / explain process'. return_value = 'what does X return / X's name/type'. conditional = 'when/under what condition'. config_mapping = 'what does config key K do'. enumeration = 'list/count all X'. call_chain = 'which X calls Y'. Use 'unknown' only if genuinely ambiguous — ERM will fall back to keyword inference. Picking the right kind here directly drives which evidence predicates the downstream pipeline opens."},
          "answer_shape":  {"type": "string", "enum": ["list_of_symbols","step_list","value","boolean","config_value","none"], "description": "Expected final-answer structure. list_of_symbols = answer is a set of identifier names (the finalizer will forbid out-of-evidence symbols). step_list = ordered steps of a mechanism. value = a single literal/return. boolean = yes/no. config_value = a config key's resolved value. none = no structured shape applies."},
          "objective":   {"type": "string", "description": "Node-level objective for AnalysisIR.TaskGraph.nodes[].objective. Defaults to title when omitted."},
          "inputs":      {"type": "array", "items": {"type": "string"}, "description": "Node-level required inputs consumed by this task."},
          "outputs":     {"type": "array", "items": {"type": "string"}, "description": "Node-level outputs produced by this task."},
          "success_criteria": {"type": "array", "items": {"type": "string"}, "description": "Concrete conditions that define successful completion."},
          "entry_conditions": {"type": "array", "items": {"type": "string"}, "description": "Required preconditions for scheduling this node. Must not be empty."},
          "exit_artifacts": {"type": "array", "items": {"type": "string"}, "description": "Artifacts that must exist when this node exits. Must not be empty."},
          "status":      {"type": "string", "enum": ["pending", "in_progress", "done", "blocked", "failed"]}
        },
        "required": ["title"]
      }
    },
    "edges": {
      "type": "array",
      "description": "TaskGraph edges. edge_type must be one of hard_dependency|soft_dependency|validation_feedback.",
      "items": {
        "type": "object",
        "properties": {
          "from": {"type": "string"},
          "to": {"type": "string"},
          "edge_type": {"type": "string", "enum": ["hard_dependency", "soft_dependency", "validation_feedback"]}
        },
        "required": ["from", "to", "edge_type"]
      }
    },
    "execution_policy": {
      "type": "object",
      "properties": {
        "max_parallelism": {"type": "integer"},
        "critical_path": {"type": "array", "items": {"type": "string"}},
        "retry_budget": {"type": "integer"}
      }
    }
  },
  "required": ["tasks"]
}`)
}

func (t *TodoWrite) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "todo_write requires BusContext.Mutable; the caller did not provide one",
			Timestamp: time.Now(),
		}, nil
	}

	var p todoWriteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("invalid params: %v", err),
			Timestamp: time.Now(),
		}, err
	}
	if len(p.Tasks) == 0 {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "empty task list rejected; mark items as failed/blocked instead of removing them",
			Timestamp: time.Now(),
		}, nil
	}

	items, err := buildTodoItems(p.Tasks)
	if err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   err.Error(),
			Timestamp: time.Now(),
		}, nil
	}

	// Preserve the existing Objective when replacing the list — the
	// LLM rarely re-sends it, and losing it would break system prompt
	// rebuilding. Other fields (Tasks, CurrentTaskID) are owned by
	// the tool's view of the world and get rewritten.
	existing := ctx.Mutable.TaskList()
	newList := types.TaskList{
		Objective:     existing.Objective,
		Tasks:         items,
		CurrentTaskID: pickCurrentTaskID(items),
	}
	ctx.Mutable.SetTaskList(newList)
	ctx.AnalysisIR = buildAnalysisIR(p, newList)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   renderTodoSummary(items),
		Timestamp: time.Now(),
	}, nil
}

func buildAnalysisIR(p todoWriteParams, tl types.TaskList) *types.AnalysisIR {
	var current *todoWriteTaskParam
	for i := range p.Tasks {
		id := strings.TrimSpace(p.Tasks[i].ID)
		if id == "" {
			id = fmt.Sprintf("task-%d", i+1)
		}
		if id == tl.CurrentTaskID {
			current = &p.Tasks[i]
			break
		}
	}
	if current == nil && len(p.Tasks) > 0 {
		current = &p.Tasks[0]
	}
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			UserIntent: tl.Objective,
		},
		TaskGraph: types.TaskGraph{
			Nodes: make([]types.TaskGraphNode, 0, len(tl.Tasks)),
		},
		QualityGate: types.QualityGate{
			FailureActions: []string{"re-analyze"},
		},
	}
	for _, task := range tl.Tasks {
		rawNode := findRawNodeByID(p.Tasks, task.ID)
		objective := task.Title
		var inputs, outputs, successCriteria, entryConditions, exitArtifacts []string
		if rawNode != nil && strings.TrimSpace(rawNode.Objective) != "" {
			objective = strings.TrimSpace(rawNode.Objective)
		}
		if rawNode != nil {
			inputs = trimStringSlice(rawNode.Inputs)
			outputs = trimStringSlice(rawNode.Outputs)
			successCriteria = trimStringSlice(rawNode.SuccessCriteria)
			entryConditions = trimStringSlice(rawNode.EntryConditions)
			exitArtifacts = trimStringSlice(rawNode.ExitArtifacts)
		}
		ir.TaskGraph.Nodes = append(ir.TaskGraph.Nodes, types.TaskGraphNode{
			ID:              task.ID,
			Type:            normalizeNodeType(rawNode, task),
			Objective:       objective,
			Inputs:          inputs,
			Outputs:         outputs,
			SuccessCriteria: successCriteria,
			EntryConditions: ensureNonEmpty(entryConditions, "task "+task.ID+" has pending dependencies resolved"),
			ExitArtifacts:   ensureNonEmpty(exitArtifacts, "task "+task.ID+" completion report"),
		})
	}
	ir.TaskGraph.Edges = normalizeEdges(p.Edges)
	if p.ExecutionPolicy != nil {
		ir.TaskGraph.ExecutionPolicy = *p.ExecutionPolicy
	}
	if ir.TaskGraph.ExecutionPolicy.MaxParallelism <= 0 {
		ir.TaskGraph.ExecutionPolicy.MaxParallelism = 1
	}
	if ir.TaskGraph.ExecutionPolicy.RetryBudget <= 0 {
		ir.TaskGraph.ExecutionPolicy.RetryBudget = 1
	}
	if current != nil {
		ir.RequestModel.Entities = trimStringSlice(current.Entities)
		ir.RequestModel.QuestionKind = normalizeQuestionKind(current.QuestionKind)
		ir.EvidencePlan = types.EvidencePlan{
			Complexity: normalizeComplexity(current.Complexity),
			Keywords:   trimStringSlice(current.Keywords),
			SourceMix: map[string]float64{
				"repo_code": 0.7,
				"config":    0.3,
			},
			StopConditions: []string{"evidence_budget_exhausted", "acceptance_criteria_met"},
		}
		ir.AnswerContract = types.AnswerContract{
			OutputShape:        normalizeAnswerShape(current.AnswerShape),
			AcceptanceCriteria: []string{"all_must_items_satisfied", "no_forbidden_items_present"},
		}
	}
	return ir
}

func findRawNodeByID(raw []todoWriteTaskParam, id string) *todoWriteTaskParam {
	for i := range raw {
		candidate := strings.TrimSpace(raw[i].ID)
		if candidate == "" {
			candidate = fmt.Sprintf("task-%d", i+1)
		}
		if candidate == id {
			return &raw[i]
		}
	}
	return nil
}

func normalizeNodeType(raw *todoWriteTaskParam, task types.TaskItem) string {
	if raw != nil && strings.TrimSpace(raw.NodeType) != "" {
		return strings.TrimSpace(raw.NodeType)
	}
	if task.Writing {
		return "implementation"
	}
	return "analysis"
}

func ensureNonEmpty(in []string, fallback string) []string {
	if len(in) > 0 {
		return in
	}
	return []string{fallback}
}

func normalizeEdges(in []types.TaskGraphEdge) []types.TaskGraphEdge {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.TaskGraphEdge, 0, len(in))
	for _, edge := range in {
		from := strings.TrimSpace(edge.From)
		to := strings.TrimSpace(edge.To)
		if from == "" || to == "" {
			continue
		}
		edgeType := strings.ToLower(strings.TrimSpace(edge.EdgeType))
		switch edgeType {
		case "hard_dependency", "soft_dependency", "validation_feedback":
		default:
			edgeType = "hard_dependency"
		}
		out = append(out, types.TaskGraphEdge{
			From:     from,
			To:       to,
			EdgeType: edgeType,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildTodoItems normalizes raw params into TaskItem values, assigning
// synthetic IDs where missing and rejecting duplicates outright.
func buildTodoItems(raw []todoWriteTaskParam) ([]types.TaskItem, error) {
	items := make([]types.TaskItem, 0, len(raw))
	seen := make(map[string]bool, len(raw))

	for i, r := range raw {
		title := strings.TrimSpace(r.Title)
		if title == "" {
			return nil, fmt.Errorf("task[%d]: title is required", i)
		}

		id := strings.TrimSpace(r.ID)
		if id == "" {
			id = fmt.Sprintf("task-%d", i+1)
		}
		if seen[id] {
			return nil, fmt.Errorf("task[%d]: duplicate id %q", i, id)
		}
		seen[id] = true

		items = append(items, types.TaskItem{
			ID:          id,
			Title:       title,
			Description: strings.TrimSpace(r.Description),
			Writing:     r.Writing,
			HighRisk:    r.HighRisk,
			Complexity:  normalizeComplexity(r.Complexity),
			Status:      normalizeTodoStatus(r.Status),
		})
	}
	return items, nil
}

// pickCurrentTaskID selects the task that should drive routing for
// the next decision point. Preference order: first in_progress, then
// first pending, then first overall.
func pickCurrentTaskID(items []types.TaskItem) string {
	if len(items) == 0 {
		return ""
	}
	for _, item := range items {
		if item.Status == types.TaskInProgress {
			return item.ID
		}
	}
	for _, item := range items {
		if item.Status == types.TaskPending {
			return item.ID
		}
	}
	return items[0].ID
}

// normalizeTodoStatus maps LLM-produced status strings to canonical
// TaskStatus values. Unknown values fall back to Pending.
func normalizeTodoStatus(s string) types.TaskStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "in_progress", "in-progress", "active", "working":
		return types.TaskInProgress
	case "done", "completed", "complete":
		return types.TaskDone
	case "blocked":
		return types.TaskBlocked
	case "failed", "error":
		return types.TaskFailed
	default:
		return types.TaskPending
	}
}

// normalizeQuestionKind maps LLM-produced question_kind strings to the
// canonical set used by ERM (EvidenceRequirement.Kind). Unknown and
// empty values both fall back to "unknown" so downstream callers can
// cleanly distinguish "analyzer did not classify" from a valid Kind.
// The analyzer's prompt is the primary enforcement of the enum; this
// function is a defensive normalizer only.
func normalizeQuestionKind(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "registration", "register":
		return "registration"
	case "mechanism", "process", "flow":
		return "mechanism"
	case "return_value", "return-value", "return", "value_of":
		return "return_value"
	case "conditional", "condition":
		return "conditional"
	case "config_mapping", "config-mapping", "config":
		return "config_mapping"
	case "enumeration", "enumerate", "list", "count":
		return "enumeration"
	case "call_chain", "call-chain", "callchain", "calls":
		return "call_chain"
	case "", "unknown", "other":
		return "unknown"
	default:
		return "unknown"
	}
}

// normalizeAnswerShape maps LLM-produced answer_shape strings to the
// canonical set consumed by the finalizer. Empty maps to "none" so the
// finalizer can early-return its shape switch.
func normalizeAnswerShape(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "list_of_symbols", "symbol_list", "list-of-symbols":
		return "list_of_symbols"
	case "step_list", "steps", "step-list":
		return "step_list"
	case "value", "literal", "return_value":
		return "value"
	case "boolean", "yes_no", "yes/no":
		return "boolean"
	case "config_value", "config-value":
		return "config_value"
	default:
		return "none"
	}
}

// trimStringSlice drops empty/whitespace-only entries and trims each
// remaining element. Returns nil for an empty input so the field stays
// omitempty-friendly.
func trimStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// normalizeComplexity maps LLM-produced complexity strings to canonical
// values. Unknown values fall back to "moderate".
func normalizeComplexity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "simple", "easy", "trivial":
		return "simple"
	case "complex", "hard", "deep":
		return "complex"
	default:
		return "moderate"
	}
}

// renderTodoSummary builds the markdown summary that the tool returns
// to the LLM so subsequent iterations can see the latest state from
// the conversation history.
func renderTodoSummary(items []types.TaskItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "todolist updated (%d items)\n", len(items))
	for _, item := range items {
		icon := "[ ]"
		switch item.Status {
		case types.TaskInProgress:
			icon = "[~]"
		case types.TaskDone:
			icon = "[x]"
		case types.TaskBlocked:
			icon = "[!]"
		case types.TaskFailed:
			icon = "[F]"
		}
		fmt.Fprintf(&b, "  %s %s: %s\n", icon, item.ID, item.Title)
	}
	return b.String()
}
