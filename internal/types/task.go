package types

// TaskItem represents a single task in the task list.
//
// The two boolean capability fields (Writing, HighRisk) jointly
// determine which task policy the orchestrator selects for this task.
// They replaced the earlier TaskType enum: Writing=false maps to the
// old Analysis, Writing=true to Implementation, and HighRisk=true
// escalates an implementation task to the high_risk_implementation
// policy with review stages.
type TaskItem struct {
	ID          string     `json:"id" yaml:"id"`
	Title       string     `json:"title" yaml:"title"`
	Description string     `json:"description" yaml:"description"`

	// Writing indicates whether this task may mutate the filesystem.
	// It is the per-task analogue of StageConfig.RequiresWrite and
	// drives the same capability boundary.
	Writing bool `json:"writing" yaml:"writing"`

	// HighRisk indicates whether this task needs design/code review
	// before and after implementation. When true, the orchestrator
	// picks the high_risk_implementation policy (if Writing is also
	// true). The global Policy.RequireReview acts as an OR on top of
	// this flag, so operators can force review for an entire Run.
	HighRisk bool `json:"high_risk" yaml:"high_risk"`

	Status TaskStatus `json:"status" yaml:"status"`
	Result string     `json:"result,omitempty" yaml:"result,omitempty"`
}

// TaskList holds the objective and all tasks for a pipeline run.
type TaskList struct {
	Objective     string     `json:"objective" yaml:"objective"`
	Tasks         []TaskItem `json:"tasks" yaml:"tasks"`
	CurrentTaskID string     `json:"current_task_id" yaml:"current_task_id"`
}

// CurrentTask returns a pointer to the task matching CurrentTaskID, or nil.
func (tl *TaskList) CurrentTask() *TaskItem {
	for i := range tl.Tasks {
		if tl.Tasks[i].ID == tl.CurrentTaskID {
			return &tl.Tasks[i]
		}
	}
	return nil
}

// TaskState captures the current pipeline execution state.
type TaskState struct {
	Stage        PipelineStage `json:"stage"`
	Missing      MissingPiece  `json:"missing"`
	Completed    []string      `json:"completed"`
	Remaining    []string      `json:"remaining"`
	LastDecision string        `json:"last_decision"`
	LastError    string        `json:"last_error,omitempty"`
	IsTerminal   bool          `json:"is_terminal"`
}
