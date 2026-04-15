package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RuntimeSettings holds the per-process knobs that the codrax binary
// exposes on the command line. The division of responsibility across
// the two YAML files in config/ is:
//
//   - providers.yaml — LLM provider credentials and per-agent model
//     routing. Per-user secrets, never committed.
//   - codrax.yaml    — this file. Runtime knobs that describe how
//     the binary should run on this machine/in this invocation: log
//     sink, memory sink, language, per-run step budget, target repo,
//     and a pointer to providers.yaml.
//
// The pipeline topology (4 stages × 4 agents) is hardcoded in
// internal/orchestrator/topology.go and has no YAML counterpart.
//
// Every field is a pointer so the merge logic in cmd/root.go can tell
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
	Repo     *string `yaml:"repo"`
	Branch   *string `yaml:"branch"`
	CacheDir *string `yaml:"cache_dir"`

	// Tool blob sizing knobs. Flat-prefixed `blob_*` to keep the
	// namespace obvious without nesting. All three accept any
	// positive integer; non-positive (or omitted) means "use the
	// code default in internal/tool/blob.go".
	BlobMaxInlineBytes   *int `yaml:"blob_max_inline_bytes"`
	BlobPreviewHeadBytes *int `yaml:"blob_preview_head_bytes"`
	BlobPreviewTailBytes *int `yaml:"blob_preview_tail_bytes"`

	// emit_analysis runtime validation knobs. Flat-prefixed
	// `analysis_*` so the namespace stays visible alongside the
	// `blob_*` and `pipeline_*` groups. All three are optional:
	// absent = inherit the code default in
	// internal/tool/analysis_limits.go.
	//
	//   - analysis_warn_below_keywords: soft floor; the tool
	//     attaches a "[warn: ...]" tag to the ToolResult.Summary
	//     when the analyzer emits fewer keywords. 0 disables the
	//     warning. Default 8.
	//   - analysis_reject_below_keywords: hard floor; the tool
	//     fails the emit call with Success=false when the keyword
	//     count is below this value. 0 disables rejection (only
	//     the warning fires). Default 0.
	//   - analysis_generic_entity_blocklist: lowercase words the
	//     validator drops from the entities slice because they
	//     poison ERM ranking. Empty disables the filter. Default
	//     lives in DefaultAnalysisLimits().
	//   - analysis_max_prescan_rounds: runtime hard-enforcement of
	//     the "1-2 rounds then emit_analysis" ceiling in the analyze
	//     stage. After N successful pre-scan rounds (iterations
	//     whose last-executed tool is repo_map / grep / list_files),
	//     the analyzer's LoopController force-stops the dispatch
	//     and the ParseOutput failsafe synthesises a zero-value
	//     RequestModel. 0 disables the gate entirely. Default 2.
	AnalysisWarnBelowKeywords      *int     `yaml:"analysis_warn_below_keywords"`
	AnalysisRejectBelowKeywords    *int     `yaml:"analysis_reject_below_keywords"`
	AnalysisGenericEntityBlocklist []string `yaml:"analysis_generic_entity_blocklist"`
	AnalysisRejectMultipleEmit     *bool    `yaml:"analysis_reject_multiple_emit"`
	AnalysisMaxPrescanRounds       *int     `yaml:"analysis_max_prescan_rounds"`

	// Pipeline budget knobs. Flat-prefixed `pipeline_*`. Precedence:
	// code default → codrax.yaml → CLI flag.
	PipelineMaxSteps           *int `yaml:"pipeline_max_steps"`
	PipelineMaxRetriesPerStage *int `yaml:"pipeline_max_retries_per_stage"`
	PipelineMaxStageVisits     *int `yaml:"pipeline_max_stage_visits"`

	// Pointer to providers.yaml. A single
	// `CODRAX_SETTINGS=path/to/codrax.yaml` bootstraps an entire
	// environment (dev, staging, prod) from one entry point.
	ProvidersConfig *string `yaml:"providers_config"`
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
