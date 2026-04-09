package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/design/internal/types"
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
type TodoWrite struct{ ReadOnly }

type todoWriteParams struct {
	Tasks []todoWriteTaskParam `json:"tasks"`
}

type todoWriteTaskParam struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Status      string `json:"status,omitempty"`
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
          "type":        {"type": "string", "enum": ["analysis", "implementation"], "description": "Drives pipeline policy when this is the current task"},
          "status":      {"type": "string", "enum": ["pending", "in_progress", "done", "blocked", "failed"]}
        },
        "required": ["title"]
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

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   renderTodoSummary(items),
		Timestamp: time.Now(),
	}, nil
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
			Type:        normalizeTodoType(r.Type),
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

// normalizeTodoType maps LLM-produced type strings to canonical
// TaskType values. Unknown values fall back to Analysis (fail-safe).
func normalizeTodoType(s string) types.TaskType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "implementation", "implement", "code", "feature", "fix", "bug":
		return types.TaskTypeImplementation
	default:
		return types.TaskTypeAnalysis
	}
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
