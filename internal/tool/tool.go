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

	// Confidence reports how much weight a fact produced by this tool
	// should carry. Evidence tools (grep, read_file, exec_command, …)
	// return a high value; navigation indexes (repo_map) return a low
	// value; orchestration/state tools (propose_sub_agents, todo_write)
	// return 0 because they do not produce repo facts at all.
	//
	// The explorer agent uses this both to tag RepoFact.Confidence and
	// to decide whether this tool counts toward the "enough evidence
	// sources" floor. Implementations should embed one of the provided
	// mixins: EvidenceTool (0.8), NavigationTool (0.3), or
	// NonEvidenceTool (0.0).
	Confidence() float64
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

// EvidenceTool is an embeddable mixin for tools whose output constitutes
// direct evidence from the codebase (grep, read_file, exec_command, …).
type EvidenceTool struct{}

// Confidence returns 0.8 — high, because the output comes from
// reading/running against the real codebase.
func (EvidenceTool) Confidence() float64 { return 0.8 }

// NavigationTool is an embeddable mixin for tools whose output is a
// cached or derived index (repo_map). Useful for deciding where to
// look, but not citable as evidence.
type NavigationTool struct{}

// Confidence returns 0.3 — low, because the output is a derived
// index, not a live read of the source files.
func (NavigationTool) Confidence() float64 { return 0.3 }

// NonEvidenceTool is an embeddable mixin for tools that do not produce
// repo facts at all (propose_sub_agents, todo_write).
type NonEvidenceTool struct{}

// Confidence returns 0.0 — this tool does not produce factual claims
// about the codebase.
func (NonEvidenceTool) Confidence() float64 { return 0.0 }

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
