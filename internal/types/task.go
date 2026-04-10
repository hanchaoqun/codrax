package types

// TaskItem represents a single task in the task list.
//
// The two boolean capability fields (Writing, HighRisk) jointly
// determine which task policy the orchestrator selects for this
// task. Writing=false picks the analysis policy (read-only answer),
// Writing=true picks the implementation policy, and HighRisk=true
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

	// Complexity indicates the investigation depth the task requires.
	// Set by the analyzer to guide the explorer's evidence thresholds:
	//   "simple"  — lookup / yes-no / count (low threshold)
	//   "moderate" — single-component explanation (medium threshold)
	//   "complex" — cross-component architecture / flow (high threshold)
	// Empty string is treated as "moderate" (the default).
	Complexity string `json:"complexity,omitempty" yaml:"complexity,omitempty"`

	// Keywords are search terms extracted by the analyzer for the
	// explorer's Phase 1 breadth scan. Should include both domain
	// terms (CamelCase symbols, snake_case identifiers) and conceptual
	// synonyms so grep casts a wide net.
	Keywords []string `json:"keywords,omitempty" yaml:"keywords,omitempty"`

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

	// RetryHint is set when a stage self-loops (orchestrator picks
	// the same stage as next). It carries the previous dispatch's own
	// diagnosis of why it could not progress, so the next dispatch
	// sees concrete "do this differently" guidance instead of being
	// re-run with an unchanged prompt. The orchestrator clears it on
	// any forward transition. The prompt builder renders it as the
	// most prominent user section.
	RetryHint string `json:"retry_hint,omitempty"`
}
