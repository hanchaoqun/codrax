package config

import (
	"fmt"
	"os"

	"github.com/hanchaoqun/design/internal/types"
	"gopkg.in/yaml.v3"
)

// Load reads and parses the orchestrator YAML configuration file.
func Load(path string) (*types.OrchestratorConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return Parse(data)
}

// Parse parses raw YAML bytes into an OrchestratorConfig.
func Parse(data []byte) (*types.OrchestratorConfig, error) {
	var cfg types.OrchestratorConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// ResolvedConfig holds the fully resolved, ready-to-use configuration
// with typed stage configs and transitions indexed by source stage.
type ResolvedConfig struct {
	Stages       map[types.PipelineStage]*types.StageConfig
	Transitions  map[types.PipelineStage][]types.Transition
	TaskPolicies map[string]*types.TaskPolicyConfig
	PipelineSettings types.PipelineSettings
	Agents       map[types.AgentName]*types.AgentConfig
	Skills       map[string]*types.SkillConfigYAML
}

// Resolve converts a raw OrchestratorConfig into a ResolvedConfig
// with properly typed keys and indexed lookups.
func Resolve(raw *types.OrchestratorConfig) (*ResolvedConfig, error) {
	rc := &ResolvedConfig{
		Stages:       make(map[types.PipelineStage]*types.StageConfig),
		Transitions:  make(map[types.PipelineStage][]types.Transition),
		TaskPolicies: make(map[string]*types.TaskPolicyConfig),
		PipelineSettings: raw.PipelineSettings,
		Agents:       make(map[types.AgentName]*types.AgentConfig),
		Skills:       make(map[string]*types.SkillConfigYAML),
	}

	// Resolve stages
	for name, s := range raw.Stages {
		stage := types.PipelineStage(name)
		rc.Stages[stage] = &types.StageConfig{
			Name:          stage,
			DefaultAgent:  types.AgentName(s.DefaultAgent),
			DefaultSkill:  s.DefaultSkill,
			Terminal:      s.Terminal,
			RequiresWrite: s.RequiresWrite,
		}
	}

	// Resolve transitions — attach the From field
	for from, transitions := range raw.Transitions {
		stage := types.PipelineStage(from)
		resolved := make([]types.Transition, len(transitions))
		for i, t := range transitions {
			resolved[i] = types.Transition{
				From:     stage,
				To:       t.To,
				Priority: t.Priority,
			}
		}
		rc.Transitions[stage] = resolved
	}

	// Resolve task policies
	for name, p := range raw.TaskPolicies {
		policy := p // copy
		policy.Name = name
		rc.TaskPolicies[name] = &policy
	}

	// Resolve agents
	for _, a := range raw.Agents {
		agent := a // copy
		rc.Agents[agent.Name] = &agent
	}

	// Resolve skills
	for _, s := range raw.Skills {
		skill := s // copy
		rc.Skills[skill.Name] = &skill
	}

	// Invariant: any stage that requires write permission must be served by a
	// default agent that also declares requires_write. This catches misconfigured
	// pipelines where a read-only agent is bound to a write-capable stage.
	for _, stage := range rc.Stages {
		if !stage.RequiresWrite {
			continue
		}
		agent, ok := rc.Agents[stage.DefaultAgent]
		if !ok {
			return nil, fmt.Errorf("stage %q requires write but its default agent %q is not declared", stage.Name, stage.DefaultAgent)
		}
		if !agent.RequiresWrite {
			return nil, fmt.Errorf("stage %q requires write but default agent %q does not declare requires_write", stage.Name, stage.DefaultAgent)
		}
	}

	return rc, nil
}

// LoadAndResolve is a convenience function that loads and resolves in one step.
func LoadAndResolve(path string) (*ResolvedConfig, error) {
	raw, err := Load(path)
	if err != nil {
		return nil, err
	}
	return Resolve(raw)
}

// GetStageConfig returns the resolved stage config or an error.
func (rc *ResolvedConfig) GetStageConfig(stage types.PipelineStage) (*types.StageConfig, error) {
	sc, ok := rc.Stages[stage]
	if !ok {
		return nil, fmt.Errorf("unknown stage: %s", stage)
	}
	return sc, nil
}

// GetTransitions returns all outgoing transitions from a stage, sorted by priority (desc).
func (rc *ResolvedConfig) GetTransitions(stage types.PipelineStage) []types.Transition {
	ts := rc.Transitions[stage]
	// Return a copy sorted by priority descending
	sorted := make([]types.Transition, len(ts))
	copy(sorted, ts)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Priority > sorted[i].Priority {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

// GetTaskPolicy returns the named task policy or an error.
func (rc *ResolvedConfig) GetTaskPolicy(name string) (*types.TaskPolicyConfig, error) {
	p, ok := rc.TaskPolicies[name]
	if !ok {
		return nil, fmt.Errorf("unknown task policy: %s", name)
	}
	return p, nil
}
