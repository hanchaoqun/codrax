package skill

import (
	"fmt"
	"sync"
)

// Config defines the strategy for a specific type of work.
type Config struct {
	Name            string   `json:"name" yaml:"name"`
	Goal            string   `json:"goal" yaml:"goal"`
	Workflow        []string `json:"workflow" yaml:"workflow"`
	ToolSuggestions []string `json:"tool_suggestions" yaml:"tool_suggestions"`
	OutputFormat    string   `json:"output_format" yaml:"output_format"`
	Prohibitions    []string `json:"prohibitions" yaml:"prohibitions"`
}

// Registry manages skill configurations.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Config
}

func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]*Config)}
}

func (r *Registry) Register(cfg *Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[cfg.Name] = cfg
}

func (r *Registry) Get(name string) (*Config, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	if !ok {
		return nil, fmt.Errorf("skill not found: %s", name)
	}
	return s, nil
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	return names
}
