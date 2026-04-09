package types

// StageConfig is the resolved configuration for a pipeline stage.
type StageConfig struct {
	Name          PipelineStage `yaml:"name"`
	DefaultAgent  AgentName     `yaml:"default_agent"`
	DefaultSkill  string        `yaml:"default_skill"`
	Terminal      bool          `yaml:"terminal"`
	RequiresWrite bool          `yaml:"requires_write"`
}

// Transition defines a possible stage transition with priority.
type Transition struct {
	From     PipelineStage `yaml:"-" json:"from"`
	To       PipelineStage `yaml:"to" json:"to"`
	Priority int           `yaml:"priority" json:"priority"`
}

// TaskPolicyConfig defines which stages a task type is allowed to use.
type TaskPolicyConfig struct {
	Name          string          `yaml:"-"`
	AllowedStages []PipelineStage `yaml:"allowed_stages"`
}

// PipelineSettings controls optional pipeline features.
type PipelineSettings struct {
	EnableVerify                bool `yaml:"enable_verify"`
	RequireReview               bool `yaml:"require_review"`
	MaxRetriesPerStage          int  `yaml:"max_retries_per_stage"`
	AllowSkipPlanForSmallChange bool `yaml:"allow_skip_plan_for_small_change"`

	// MaxStageVisits caps how many times any single stage may be
	// entered during one Run before the orchestrator gives up and
	// forces finalize. A value <= 0 falls back to DefaultMaxStageVisits.
	// This is the second guard against runaway loops (in addition to
	// max-steps) and fires earlier when the pipeline is bouncing
	// between the same stages without making progress.
	MaxStageVisits int `yaml:"max_stage_visits"`
}

// DefaultMaxStageVisits is the fallback value used when
// PipelineSettings.MaxStageVisits is zero or negative.
const DefaultMaxStageVisits = 4

// AgentConfig is the YAML configuration for a single agent.
type AgentConfig struct {
	Name          AgentName       `yaml:"name"`
	Stages        []PipelineStage `yaml:"stages"`
	RequiresWrite bool            `yaml:"requires_write"`
}

// SkillConfigYAML is the YAML representation of a skill.
type SkillConfigYAML struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// StageConfigYAML is the YAML representation of a stage configuration.
type StageConfigYAML struct {
	DefaultAgent  string `yaml:"default_agent"`
	DefaultSkill  string `yaml:"default_skill"`
	Terminal      bool   `yaml:"terminal"`
	RequiresWrite bool   `yaml:"requires_write"`
}

// OrchestratorConfig is the top-level YAML configuration for the orchestrator.
type OrchestratorConfig struct {
	Stages       map[string]StageConfigYAML  `yaml:"stages"`
	Transitions  map[string][]Transition     `yaml:"transitions"`
	TaskPolicies map[string]TaskPolicyConfig `yaml:"task_policies"`
	PipelineSettings PipelineSettings                `yaml:"pipeline_settings"`
	Agents       []AgentConfig               `yaml:"agents"`
	Skills       []SkillConfigYAML           `yaml:"skills"`
}
