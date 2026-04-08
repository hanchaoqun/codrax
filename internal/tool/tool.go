package tool

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hanchaoqun/design/internal/types"
)

// Tool defines the interface for all local tools.
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error)
}

// Registry manages available tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return t, nil
}

// List returns the names of all registered tools.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Execute looks up a tool by name and executes it.
func (r *Registry) Execute(ctx *types.BusContext, name string, params json.RawMessage) (types.ToolResult, error) {
	t, err := r.Get(name)
	if err != nil {
		return types.ToolResult{ToolName: name, Success: false, Summary: err.Error()}, err
	}
	return t.Execute(ctx, params)
}
