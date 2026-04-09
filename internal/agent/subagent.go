package agent

import (
	"fmt"
	"sync"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SubAgent is a focused worker that executes a sub-task in parallel with other SubAgents.
// It shares read access to BusContext and writes only to its own SubAgentResult.
type SubAgent interface {
	// Name returns the registered name of this SubAgent.
	Name() string

	// Run executes the sub-task described by req.
	// req.ReadView provides shared read access to BusContext.
	// The returned SubAgentResult is the isolated write buffer.
	Run(req *types.SubAgentRequest) (*types.SubAgentResult, error)
}

// SubAgentRegistry manages available SubAgent implementations.
type SubAgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]SubAgent
}

// NewSubAgentRegistry creates a new SubAgent registry.
func NewSubAgentRegistry() *SubAgentRegistry {
	return &SubAgentRegistry{agents: make(map[string]SubAgent)}
}

// Register adds a SubAgent to the registry.
func (r *SubAgentRegistry) Register(sa SubAgent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[sa.Name()] = sa
}

// Get returns the SubAgent with the given name.
func (r *SubAgentRegistry) Get(name string) (SubAgent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sa, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("subagent not found: %s", name)
	}
	return sa, nil
}

// Names returns all registered SubAgent names (used to generate LLM tool schema enum).
func (r *SubAgentRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	return names
}

// RegisterDefaultSubAgents registers the default set of SubAgent implementations.
func RegisterDefaultSubAgents(r *SubAgentRegistry, deps *Dependencies) {
	r.Register(NewSubExplorer(deps))
}
