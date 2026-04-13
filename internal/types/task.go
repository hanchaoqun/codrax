package types

// TaskItem represents a single task in the task list.
//
// The LLM classification carrier (writing, high_risk, complexity,
// keywords, entities, question_kind, answer_shape) used to live on
// this struct but was moved to MutableState.Classification() in batch
// B5b-β so the analyzer's post-processing pipeline reads from a
// single canonical source instead of seven per-task scalars.
type TaskItem struct {
	ID          string     `json:"id" yaml:"id"`
	Title       string     `json:"title" yaml:"title"`
	Description string     `json:"description" yaml:"description"`
	Status      TaskStatus `json:"status" yaml:"status"`
	Result      string     `json:"result,omitempty" yaml:"result,omitempty"`
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

	// RetryHint is set when a stage self-loops (orchestrator picks
	// the same stage as next). It carries the previous dispatch's own
	// diagnosis of why it could not progress, so the next dispatch
	// sees concrete "do this differently" guidance instead of being
	// re-run with an unchanged prompt. The orchestrator clears it on
	// any forward transition. The prompt builder renders it as the
	// most prominent user section.
	RetryHint string `json:"retry_hint,omitempty"`
}
