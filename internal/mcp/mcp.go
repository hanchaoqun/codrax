package mcp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ToolSchema describes a tool exposed by an MCP server.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ResourceSchema describes a resource exposed by an MCP server.
type ResourceSchema struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

// PromptSchema describes a prompt advertised by an MCP server.
type PromptSchema struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Server defines the interface for MCP servers.
type Server interface {
	Name() string
	Transport() types.TransportType
	ListTools() []ToolSchema
	ListResources() []ResourceSchema
	ReadResource(uri string) (types.MCPResponse, error)
	ListPrompts() []PromptSchema
	CallTool(name string, params json.RawMessage) (types.MCPResponse, error)
	Close() error
}

// Registry manages MCP server connections.
type Registry struct {
	mu      sync.RWMutex
	servers map[string]Server
}

func NewRegistry() *Registry {
	return &Registry{servers: make(map[string]Server)}
}

func (r *Registry) Register(s Server) error {
	if s == nil {
		return fmt.Errorf("mcp server is nil")
	}
	name := s.Name()
	if !validMCPName(name) {
		return fmt.Errorf("invalid mcp server name %q: use letters, digits, underscore, or dash", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.servers[name]; exists {
		return fmt.Errorf("duplicate mcp server name: %s", name)
	}
	r.servers[name] = s
	return nil
}

func (r *Registry) Get(name string) (Server, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.servers[name]
	if !ok {
		return nil, fmt.Errorf("mcp server not found: %s", name)
	}
	return s, nil
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.servers))
	for name := range r.servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) ListAllTools() []ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []ToolSchema
	serverNames := make([]string, 0, len(r.servers))
	for name := range r.servers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)
	for _, serverName := range serverNames {
		s := r.servers[serverName]
		for _, t := range s.ListTools() {
			namespaced := NamespacedToolName(serverName, t.Name)
			if namespaced == "" {
				continue
			}
			all = append(all, ToolSchema{
				Name:        namespaced,
				Description: fmt.Sprintf("[MCP:%s] %s", serverName, t.Description),
				Parameters:  t.Parameters,
			})
		}
	}
	return all
}

func (r *Registry) CallTool(serverName, toolName string, params json.RawMessage) (types.MCPResponse, error) {
	s, err := r.Get(serverName)
	if err != nil {
		return types.MCPResponse{ServerName: serverName, Success: false}, err
	}
	return s.CallTool(toolName, params)
}

// ResolveNamespaced maps a model-visible MCP tool name back to the exact
// server/tool pair that advertised it. This is the hard routing signal for MCP:
// no substring matching, no best-effort prefix guesses.
func (r *Registry) ResolveNamespaced(name string) (Server, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for serverName, s := range r.servers {
		for _, t := range s.ListTools() {
			if NamespacedToolName(serverName, t.Name) == name {
				return s, t.Name, true
			}
		}
	}
	return nil, "", false
}

// SchemaForNamespaced returns the JSON schema for a model-visible MCP tool.
func (r *Registry) SchemaForNamespaced(name string) (json.RawMessage, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for serverName, s := range r.servers {
		for _, t := range s.ListTools() {
			if NamespacedToolName(serverName, t.Name) == name {
				return t.Parameters, true
			}
		}
	}
	return nil, false
}

func (r *Registry) ListAllResources() []ResourceSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []ResourceSchema
	serverNames := make([]string, 0, len(r.servers))
	for name := range r.servers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)
	for _, serverName := range serverNames {
		for _, res := range r.servers[serverName].ListResources() {
			res.Name = firstNonEmpty(res.Name, res.URI)
			out = append(out, res)
		}
	}
	return out
}

func (r *Registry) HasResources() bool {
	return len(r.ListAllResources()) > 0
}

func (r *Registry) ReadResource(uri string) (types.MCPResponse, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return types.MCPResponse{Success: false}, fmt.Errorf("mcp resource uri is required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, serverName := range r.ListLocked() {
		s := r.servers[serverName]
		for _, res := range s.ListResources() {
			if strings.TrimSpace(res.URI) == uri {
				return s.ReadResource(uri)
			}
		}
	}
	return types.MCPResponse{Success: false}, fmt.Errorf("mcp resource uri is not advertised by any server: %s", uri)
}

func (r *Registry) ListAllPrompts() []PromptSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []PromptSchema
	for _, serverName := range r.ListLocked() {
		for _, p := range r.servers[serverName].ListPrompts() {
			if p.Name == "" {
				continue
			}
			p.Name = serverName + "." + p.Name
			out = append(out, p)
		}
	}
	return out
}

func (r *Registry) ListLocked() []string {
	names := make([]string, 0, len(r.servers))
	for name := range r.servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NamespacedToolName returns the provider-safe tool name shown to the model.
func NamespacedToolName(serverName, toolName string) string {
	server := sanitizeNameComponent(serverName)
	tool := sanitizeNameComponent(toolName)
	if server == "" || tool == "" {
		return ""
	}
	return server + "__" + tool
}

func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var lastErr error
	for _, s := range r.servers {
		if err := s.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

var safeMCPNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func validMCPName(name string) bool {
	return safeMCPNamePattern.MatchString(name)
}

func sanitizeNameComponent(s string) string {
	out := make([]rune, 0, len(s))
	lastUnderscore := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '_' || r == '-'
		if ok {
			out = append(out, r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			out = append(out, '_')
			lastUnderscore = true
		}
	}
	for len(out) > 0 && out[0] == '_' {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == '_' {
		out = out[:len(out)-1]
	}
	return string(out)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
