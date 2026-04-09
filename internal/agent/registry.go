package agent

import (
	"fmt"
	"sync"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/types"
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

// RegisterDefaults registers all 7 agent types.
// If resolver is non-nil, each agent gets its own LLM adapter;
// otherwise all agents share deps.LLM.
func RegisterDefaults(r *Registry, deps *Dependencies, resolver LLMResolver) {
	agents := []types.AgentName{
		types.AgentPlanner,
		types.AgentExplorer,
		types.AgentImplementer,
		types.AgentDesignReviewer,
		types.AgentCodeReviewer,
		types.AgentVerifier,
		types.AgentFinalizer,
	}

	constructors := map[types.AgentName]func(*Dependencies) Agent{
		types.AgentPlanner:        func(d *Dependencies) Agent { return NewPlannerAgent(d) },
		types.AgentExplorer:       func(d *Dependencies) Agent { return NewExplorerAgent(d) },
		types.AgentImplementer:    func(d *Dependencies) Agent { return NewImplementerAgent(d) },
		types.AgentDesignReviewer: func(d *Dependencies) Agent { return NewDesignReviewerAgent(d) },
		types.AgentCodeReviewer:   func(d *Dependencies) Agent { return NewCodeReviewerAgent(d) },
		types.AgentVerifier:       func(d *Dependencies) Agent { return NewVerifierAgent(d) },
		types.AgentFinalizer:      func(d *Dependencies) Agent { return NewFinalizerAgent(d) },
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
