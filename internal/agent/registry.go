package agent

import (
	"fmt"
	"sync"

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

// RegisterDefaults registers all 6 agent types with the given dependencies.
func RegisterDefaults(r *Registry, deps *Dependencies) {
	r.Register(NewPlannerAgent(deps))
	r.Register(NewExplorerAgent(deps))
	r.Register(NewImplementerAgent(deps))
	r.Register(NewReviewerAgent(deps))
	r.Register(NewVerifierAgent(deps))
	r.Register(NewFinalizerAgent(deps))
}
