package tool

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Tool defines the interface for all local tools.
//
// IsWrite reports whether the tool's primary purpose is to mutate the
// filesystem. It backs the requires_write permission boundary: read-only
// agents must not be granted access to tools that return true here.
// Implementations should embed ReadOnly or WriteCapable to satisfy this
// method without boilerplate.
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error)
	IsWrite() bool
}

// ReadOnly is an embeddable mixin that marks a Tool as non-mutating.
// Embed it in a tool struct to satisfy IsWrite() with a constant false.
type ReadOnly struct{}

// IsWrite returns false. ReadOnly tools never touch the filesystem.
func (ReadOnly) IsWrite() bool { return false }

// WriteCapable is an embeddable mixin that marks a Tool as filesystem-mutating.
// Embed it in a tool struct to satisfy IsWrite() with a constant true.
type WriteCapable struct{}

// IsWrite returns true. WriteCapable tools require write permission.
func (WriteCapable) IsWrite() bool { return true }

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

// IsWrite reports whether the named tool is filesystem-mutating. Unknown
// tool names return false: callers that need a strict allow/deny decision
// should first check Get() to distinguish "missing" from "read-only".
func (r *Registry) IsWrite(name string) bool {
	t, err := r.Get(name)
	if err != nil {
		return false
	}
	return t.IsWrite()
}
