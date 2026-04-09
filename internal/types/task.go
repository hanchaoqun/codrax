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

	Status    TaskStatus `json:"status" yaml:"status"`
	DependsOn []string   `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Result    string     `json:"result,omitempty" yaml:"result,omitempty"`
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

// UpdateTask updates the status and result of a task by ID.
func (tl *TaskList) UpdateTask(id string, status TaskStatus, result string) {
	for i := range tl.Tasks {
		if tl.Tasks[i].ID == id {
			tl.Tasks[i].Status = status
			tl.Tasks[i].Result = result
			return
		}
	}
}

// NextPendingTask returns a pointer to the first task with pending status, or nil.
func (tl *TaskList) NextPendingTask() *TaskItem {
	for i := range tl.Tasks {
		if tl.Tasks[i].Status == TaskPending {
			return &tl.Tasks[i]
		}
	}
	return nil
}

// AllDone returns true if every task is either done or failed.
func (tl *TaskList) AllDone() bool {
	for i := range tl.Tasks {
		s := tl.Tasks[i].Status
		if s != TaskDone && s != TaskFailed {
			return false
		}
	}
	return true
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
