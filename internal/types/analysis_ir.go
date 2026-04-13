package types

// AnalysisIR is the analyzer's canonical structured output.
// It centralizes intent/evidence/answer semantics so downstream
// stages don't depend on duplicated TaskItem text fields.
type AnalysisIR struct {
	RequestModel   RequestModel   `json:"request_model" yaml:"request_model"`
	TaskGraph      TaskGraph      `json:"task_graph" yaml:"task_graph"`
	HypothesisSet  []Hypothesis   `json:"hypothesis_set,omitempty" yaml:"hypothesis_set,omitempty"`
	EvidencePlan   EvidencePlan   `json:"evidence_plan" yaml:"evidence_plan"`
	AnswerContract AnswerContract `json:"answer_contract" yaml:"answer_contract"`
	QualityGate    QualityGate    `json:"quality_gate" yaml:"quality_gate"`
}

type RequestModel struct {
	UserIntent   string    `json:"user_intent,omitempty" yaml:"user_intent,omitempty"`
	Scenario     string    `json:"scenario,omitempty" yaml:"scenario,omitempty"`
	Risk         string    `json:"risk,omitempty" yaml:"risk,omitempty"`
	Ambiguities  []string  `json:"ambiguities,omitempty" yaml:"ambiguities,omitempty"`
	Entities     []string  `json:"entities,omitempty" yaml:"entities,omitempty"`
	QuestionKind string    `json:"question_kind,omitempty" yaml:"question_kind,omitempty"`
	TermGraph    TermGraph `json:"term_graph,omitempty" yaml:"term_graph,omitempty"`
}

type TermGraph struct {
	Nodes   []TermNode      `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Aliases []TermAliasEdge `json:"aliases,omitempty" yaml:"aliases,omitempty"`
}

type TermNode struct {
	ID        string `json:"id" yaml:"id"`
	Surface   string `json:"surface" yaml:"surface"`
	Canonical string `json:"canonical" yaml:"canonical"`
	Language  string `json:"language,omitempty" yaml:"language,omitempty"`
	POS       string `json:"pos,omitempty" yaml:"pos,omitempty"`
	Domain    string `json:"domain,omitempty" yaml:"domain,omitempty"`
	Kind      string `json:"kind,omitempty" yaml:"kind,omitempty"`
}

type TermAliasEdge struct {
	From       string  `json:"from" yaml:"from"`
	To         string  `json:"to" yaml:"to"`
	Relation   string  `json:"relation,omitempty" yaml:"relation,omitempty"`
	Confidence float64 `json:"confidence,omitempty" yaml:"confidence,omitempty"`
}

type TaskGraph struct {
	Nodes           []TaskGraphNode     `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Edges           []TaskGraphEdge     `json:"edges,omitempty" yaml:"edges,omitempty"`
	ExecutionPolicy TaskExecutionPolicy `json:"execution_policy,omitempty" yaml:"execution_policy,omitempty"`
}

type TaskGraphNode struct {
	ID               string   `json:"id" yaml:"id"`
	Title            string   `json:"title" yaml:"title"`
	HypothesisRefs   []string `json:"hypothesis_refs,omitempty" yaml:"hypothesis_refs,omitempty"`
	IsBaselineProbe  bool     `json:"is_baseline_probe,omitempty" yaml:"is_baseline_probe,omitempty"`
	HypothesisStatus string   `json:"hypothesis_status,omitempty" yaml:"hypothesis_status,omitempty"`
}

type Hypothesis struct {
	ID                     string   `json:"id,omitempty" yaml:"id,omitempty"`
	Statement              string   `json:"statement" yaml:"statement"`
	RequiredEvidence       []string `json:"required_evidence,omitempty" yaml:"required_evidence,omitempty"`
	FalsificationCondition string   `json:"falsification_condition,omitempty" yaml:"falsification_condition,omitempty"`
	Priority               int      `json:"priority,omitempty" yaml:"priority,omitempty"`
	Status                 string   `json:"status,omitempty" yaml:"status,omitempty"`
}

type TaskGraphEdge struct {
	From     string `json:"from" yaml:"from"`
	To       string `json:"to" yaml:"to"`
	EdgeType string `json:"edge_type,omitempty" yaml:"edge_type,omitempty"`
}

type TaskExecutionPolicy struct {
	MaxParallelism int      `json:"max_parallelism,omitempty" yaml:"max_parallelism,omitempty"`
	CriticalPath   []string `json:"critical_path,omitempty" yaml:"critical_path,omitempty"`
	RetryBudget    int      `json:"retry_budget,omitempty" yaml:"retry_budget,omitempty"`
}

type EvidencePlan struct {
	Complexity     string             `json:"complexity,omitempty" yaml:"complexity,omitempty"`
	Keywords       []string           `json:"keywords,omitempty" yaml:"keywords,omitempty"`
	EvidenceBudget int                `json:"evidence_budget,omitempty" yaml:"evidence_budget,omitempty"`
	SourceMix      map[string]float64 `json:"source_mix,omitempty" yaml:"source_mix,omitempty"`
	StopConditions []string           `json:"stop_conditions,omitempty" yaml:"stop_conditions,omitempty"`
}

type AnswerContract struct {
	OutputShape        string   `json:"output_shape,omitempty" yaml:"output_shape,omitempty"`
	MustInclude        []string `json:"must_include,omitempty" yaml:"must_include,omitempty"`
	Forbidden          []string `json:"forbidden,omitempty" yaml:"forbidden,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty" yaml:"acceptance_criteria,omitempty"`
}

type QualityGate struct {
	SelfCheckThresholds map[string]float64 `json:"self_check_thresholds,omitempty" yaml:"self_check_thresholds,omitempty"`
	FailureActions      []string           `json:"failure_actions,omitempty" yaml:"failure_actions,omitempty"`
}
