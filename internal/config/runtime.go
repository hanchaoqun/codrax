package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RuntimeSettings holds the per-process knobs that the codrax binary
// exposes on the command line. The division of responsibility across
// the three YAML files in config/ is:
//
//   - orchestrator.yaml — pipeline topology (stages, transitions,
//     policies, agents, skills, per-stage limits). Shared across users.
//   - providers.yaml    — LLM provider credentials and per-agent model
//     routing. Per-user secrets, never committed.
//   - codrax.yaml       — this file. Runtime knobs that describe how
//     the binary should run on this machine/in this invocation: log
//     sink, memory sink, language, per-run step budget, target repo,
//     and pointers to the two files above.
//
// Nothing in this struct should duplicate orchestrator.yaml or
// providers.yaml keys: OrchestratorConfig and ProvidersConfig are
// paths, not contents; PipelineMaxSteps is a global Run() budget,
// not a per-stage limit like pipeline_max_stage_visits.
//
// Every field is a pointer so the merge logic in main.go can tell
// "user omitted this key in the YAML file" (nil) from "user set it
// to the zero value" (non-nil pointer to the zero value). This matters
// for LogStdout in particular: a default of false and a config file
// with `log_stdout: false` must both be allowed to override each
// other based on precedence.
type RuntimeSettings struct {
	// Log + memory + language (per-process UX).
	LogDir    *string `yaml:"log_dir"`
	LogLevel  *string `yaml:"log_level"`
	LogStdout *bool   `yaml:"log_stdout"`
	MemoryDir *string `yaml:"memory_dir"`
	Lang      *string `yaml:"lang"`

	// Per-invocation defaults.
	Repo   *string `yaml:"repo"`
	Branch *string `yaml:"branch"`

	// Tool blob sizing knobs. Flat-prefixed `blob_*` to keep the
	// namespace obvious without nesting. All three accept any
	// positive integer; non-positive (or omitted) means "use the
	// code default in internal/tool/blob.go".
	BlobMaxInlineBytes   *int `yaml:"blob_max_inline_bytes"`
	BlobPreviewHeadBytes *int `yaml:"blob_preview_head_bytes"`
	BlobPreviewTailBytes *int `yaml:"blob_preview_tail_bytes"`

	// Pipeline behavior. Flat-prefixed `pipeline_*`. The toggles and
	// per-stage budgets used to live in orchestrator.yaml's
	// `pipeline_settings:` block; they have moved here because they
	// are runtime/operator concerns, not pipeline topology. The
	// orchestrator.yaml block is still loaded for backward
	// compatibility and acts as a fallback layer beneath these — see
	// main.go for the precedence merge. PipelineMaxSteps is the
	// global Run() step budget; it never lived in orchestrator.yaml,
	// so its precedence chain is just code default → codrax.yaml →
	// CLI flag.
	PipelineMaxSteps              *int  `yaml:"pipeline_max_steps"`
	PipelineMaxRetriesPerStage    *int  `yaml:"pipeline_max_retries_per_stage"`
	PipelineMaxStageVisits        *int  `yaml:"pipeline_max_stage_visits"`
	PipelineEnableVerify          *bool `yaml:"pipeline_enable_verify"`
	PipelineRequireReview         *bool `yaml:"pipeline_require_review"`
	PipelineAllowSkipPlanForSmall *bool `yaml:"pipeline_allow_skip_plan_for_small_change"`

	// Pointers to the other two config files. Nested here so a single
	// `CODRAX_SETTINGS=path/to/codrax.yaml` bootstraps an entire
	// environment (dev, staging, prod) from one entry point.
	OrchestratorConfig *string `yaml:"orchestrator_config"`
	ProvidersConfig    *string `yaml:"providers_config"`
}

// LoadRuntimeSettings reads path as a YAML document into a
// RuntimeSettings. The empty-but-present case (a file with no keys)
// is valid and returns a zero-value struct, which the caller treats
// as "inherit all defaults". A missing file is signaled by
// os.ErrNotExist so the caller can decide whether silence (default
// path) or a loud error (explicit --settings path) is appropriate.
func LoadRuntimeSettings(path string) (*RuntimeSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s RuntimeSettings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse runtime settings: %w", err)
	}
	return &s, nil
}

// IsNotExist is re-exported so main.go can keep its runtime-config
// branching self-contained without importing os just for this check.
func IsNotExist(err error) bool { return errors.Is(err, os.ErrNotExist) }
