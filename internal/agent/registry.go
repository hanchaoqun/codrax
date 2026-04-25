package agent

import (
	"fmt"
	"sync"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Registry manages available agents.
type Registry struct {
	mu     sync.RWMutex
	agents map[types.AgentName]Agent
}

// NewRegistry creates a new agent registry.
func NewRegistry() *Registry {
	return &Registry{agents: make(map[types.AgentName]Agent)}
}

// Register adds an agent to the registry.
func (r *Registry) Register(a Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[a.Name()] = a
}

// Get returns the agent with the given name.
func (r *Registry) Get(name types.AgentName) (Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	return a, nil
}

// List returns all registered agent names.
func (r *Registry) List() []types.AgentName {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]types.AgentName, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	return names
}

// LLMResolver returns an LLM adapter for the given agent name.
// If it returns nil, the default adapter from Dependencies is used.
type LLMResolver func(name types.AgentName) llm.Adapter

// RegisterDefaults registers all agent types. After the 2026-04-14
// simplification the codrax pipeline is read-only: four agents
// drive the analyze → explore → extract → finalize flow. The
// log_triager is additionally registered as a conditional pre-stage
// agent whose Guard in internal/orchestrator/topology.go decides
// per Run whether it dispatches (fires only when AttachedLog is
// non-empty). If resolver is non-nil, each agent gets its own LLM
// adapter; otherwise all agents share deps.LLM.
//
// triageSettings carries the log_triager's per-stage tuning. Pass a
// zero-value LogTriageSettings to inherit DefaultLogTriageSettings;
// cmd/root.go merges codrax.yaml's log_triage_* knobs and passes the
// resolved struct here.
func RegisterDefaults(r *Registry, deps *Dependencies, resolver LLMResolver, triageSettings LogTriageSettings) {
	agents := []types.AgentName{
		types.AgentAnalyzer,
		types.AgentExplorer,
		types.AgentExtractor,
		types.AgentFinalizer,
		types.AgentLogTriager,
		types.AgentPerfTriager,
		// Write-mode agents. All three are real LLM-backed agents:
		// planner emits a structured ChangePlan via emit_change_plan;
		// coder walks plan.Changes via per-unit apply_patch calls
		// inside the orchestrator-provisioned worktree; verifier
		// drives run_tests (deterministic 4-language parser) and
		// persists a ChangeReport. All three stay inert for
		// read-mode Runs because those stages only fire when
		// BusContext.Mode is Plan / Apply / Verify.
		types.AgentPlanner,
		types.AgentCoder,
		types.AgentVerifier,
	}

	constructors := map[types.AgentName]func(*Dependencies) Agent{
		types.AgentAnalyzer:   func(d *Dependencies) Agent { return NewAnalyzerAgent(d) },
		types.AgentExplorer:   func(d *Dependencies) Agent { return NewExplorerAgent(d) },
		types.AgentExtractor:  func(d *Dependencies) Agent { return NewExtractorAgent(d) },
		types.AgentFinalizer:  func(d *Dependencies) Agent { return NewFinalizerAgent(d) },
		types.AgentLogTriager:  func(d *Dependencies) Agent { return NewLogTriagerAgent(d, triageSettings) },
		types.AgentPerfTriager: func(d *Dependencies) Agent { return NewPerfTriagerAgent(d) },
		types.AgentPlanner:    func(d *Dependencies) Agent { return NewPlannerAgent(d) },
		types.AgentCoder:      func(d *Dependencies) Agent { return NewCoderAgent(d) },
		types.AgentVerifier:   func(d *Dependencies) Agent { return NewVerifierAgent(d) },
	}

	for _, name := range agents {
		d := deps
		if resolver != nil {
			if adapter := resolver(name); adapter != nil {
				// Create a copy of deps with the agent-specific adapter
				agentDeps := *deps
				agentDeps.LLM = adapter
				d = &agentDeps
			}
		}
		r.Register(constructors[name](d))
	}
}
