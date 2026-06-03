package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const DefaultMaxServers = 8

// LoadServers starts and registers configured MCP servers. It is all-or-nothing:
// a startup failure closes every server it already started.
func LoadServers(reg *Registry, cfgs []types.MCPServerConfig, maxServers int) error {
	if reg == nil || len(cfgs) == 0 {
		return nil
	}
	if maxServers <= 0 {
		maxServers = DefaultMaxServers
	}
	if len(cfgs) > maxServers {
		return fmt.Errorf("mcp_servers has %d entries, exceeds mcp_max_servers=%d", len(cfgs), maxServers)
	}
	seen := map[string]bool{}
	for _, cfg := range cfgs {
		name := strings.TrimSpace(cfg.Name)
		if seen[name] {
			return fmt.Errorf("duplicate mcp server name: %s", name)
		}
		seen[name] = true
	}

	started := make([]Server, 0, len(cfgs))
	for _, cfg := range cfgs {
		server, err := StartServerFromConfig(cfg)
		if err != nil {
			closeServers(started)
			return err
		}
		started = append(started, server)
		if err := reg.Register(server); err != nil {
			closeServers(started)
			return err
		}
	}
	return nil
}

func StartServerFromConfig(cfg types.MCPServerConfig) (*StdioServer, error) {
	server, err := NewStdioServerFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	if err := server.Start(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), server.startupTimeout)
	err = server.Initialize(ctx)
	cancel()
	if err != nil {
		_ = server.Close()
		return nil, err
	}
	return server, nil
}

func closeServers(servers []Server) {
	for _, s := range servers {
		_ = s.Close()
	}
}
