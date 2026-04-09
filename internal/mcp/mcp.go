package mcp

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ToolSchema describes a tool exposed by an MCP server.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Server defines the interface for MCP servers.
type Server interface {
	Name() string
	Transport() types.TransportType
	ListTools() []ToolSchema
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

func (r *Registry) Register(s Server) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.servers[s.Name()] = s
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
	return names
}

func (r *Registry) ListAllTools() []ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []ToolSchema
	for _, s := range r.servers {
		all = append(all, s.ListTools()...)
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
