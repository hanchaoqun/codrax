package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// StdioServer implements Server for stdio-based MCP servers.
// It manages a child process and communicates via JSON-RPC over stdin/stdout.
type StdioServer struct {
	name    string
	command string
	args    []string

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	started bool
}

// NewStdioServer creates a new StdioServer with the given command and arguments.
func NewStdioServer(name, command string, args ...string) *StdioServer {
	return &StdioServer{
		name:    name,
		command: command,
		args:    args,
	}
}

// Start launches the child process and sets up stdin/stdout communication.
func (s *StdioServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("stdio server %s already started", s.name)
	}

	s.cmd = exec.Command(s.command, s.args...)

	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	s.stdin = stdin

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	s.scanner = bufio.NewScanner(stdout)
	s.scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command %s: %w", s.command, err)
	}

	s.started = true
	return nil
}

// Name returns the server name.
func (s *StdioServer) Name() string {
	return s.name
}

// Transport returns the transport type (stdio).
func (s *StdioServer) Transport() types.TransportType {
	return types.TransportStdio
}

// ListTools returns the tools available from this server.
// Returns an empty list if the process has not been started.
func (s *StdioServer) ListTools() []ToolSchema {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	// TODO: Send tools/list JSON-RPC request to the child process
	// and parse the response. For now, return an empty list.
	return nil
}

// CallTool invokes a tool on the MCP server.
// Returns an error if the process has not been started.
func (s *StdioServer) CallTool(name string, params json.RawMessage) (types.MCPResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return types.MCPResponse{
			ServerName: s.name,
			Success:    false,
			Timestamp:  time.Now(),
		}, fmt.Errorf("stdio server %s not connected", s.name)
	}

	// TODO: Send tools/call JSON-RPC request and read the response.
	// For now, return a placeholder response.
	return types.MCPResponse{
		ServerName: s.name,
		Method:     "tools/call",
		Summary:    fmt.Sprintf("called tool %s", name),
		Success:    false,
		Timestamp:  time.Now(),
	}, fmt.Errorf("tool invocation not yet implemented")
}

// Close terminates the child process and cleans up resources.
func (s *StdioServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	s.started = false

	if s.stdin != nil {
		s.stdin.Close()
	}

	if s.cmd != nil && s.cmd.Process != nil {
		if err := s.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		// Wait to avoid zombies; ignore error since we killed it.
		_ = s.cmd.Wait()
	}

	return nil
}
