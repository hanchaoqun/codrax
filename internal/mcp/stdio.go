package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	defaultStartupTimeout  = 3 * time.Second
	defaultCallTimeout     = 10 * time.Second
	defaultMaxResponseSize = 4 * 1024 * 1024
	defaultProtocolVersion = "2025-03-26"
)

var defaultToolParameters = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":true}`)

// StdioServer implements Server for stdio-based MCP servers.
type StdioServer struct {
	name    string
	command string
	args    []string
	env     map[string]string

	inheritEnv      bool
	startupTimeout  time.Duration
	callTimeout     time.Duration
	maxResponseSize int

	mu        sync.Mutex
	writeMu   sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	scanner   *bufio.Scanner
	started   bool
	healthy   bool
	waitErr   chan error
	pending   map[int64]chan rpcResponse
	nextID    atomic.Int64
	tools     []ToolSchema
	toolsOnce bool
}

// NewStdioServer creates a new StdioServer with default limits.
func NewStdioServer(name, command string, args ...string) *StdioServer {
	return &StdioServer{
		name:            strings.TrimSpace(name),
		command:         strings.TrimSpace(command),
		args:            append([]string(nil), args...),
		startupTimeout:  defaultStartupTimeout,
		callTimeout:     defaultCallTimeout,
		maxResponseSize: defaultMaxResponseSize,
	}
}

// NewStdioServerFromConfig creates a stdio server from codrax.yaml settings.
func NewStdioServerFromConfig(cfg types.MCPServerConfig) (*StdioServer, error) {
	name := strings.TrimSpace(cfg.Name)
	if !validMCPName(name) {
		return nil, fmt.Errorf("invalid mcp server name %q: use letters, digits, underscore, or dash", name)
	}
	transport := cfg.Transport
	if transport == "" {
		transport = types.TransportStdio
	}
	if transport != types.TransportStdio {
		return nil, fmt.Errorf("mcp server %s: unsupported transport %q", name, transport)
	}
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return nil, fmt.Errorf("mcp server %s: command is required", name)
	}
	s := NewStdioServer(name, command, cfg.Args...)
	s.env = cloneStringMap(cfg.Env)
	if cfg.InheritEnv != nil {
		s.inheritEnv = *cfg.InheritEnv
	}
	if cfg.StartupTimeoutMS != nil && *cfg.StartupTimeoutMS > 0 {
		s.startupTimeout = time.Duration(*cfg.StartupTimeoutMS) * time.Millisecond
	}
	if cfg.CallTimeoutMS != nil && *cfg.CallTimeoutMS > 0 {
		s.callTimeout = time.Duration(*cfg.CallTimeoutMS) * time.Millisecond
	}
	if cfg.MaxResponseBytes != nil && *cfg.MaxResponseBytes > 0 {
		s.maxResponseSize = *cfg.MaxResponseBytes
	}
	return s, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Start launches the child process and starts the JSON-RPC reader.
func (s *StdioServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("stdio server %s already started", s.name)
	}
	if s.command == "" {
		return fmt.Errorf("stdio server %s has no command", s.name)
	}

	cmd := exec.Command(s.command, s.args...)
	cmd.Env = s.buildEnv()
	prepareCommandProcessGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	maxBuffer := s.maxResponseSize
	if maxBuffer < 1024*1024 {
		maxBuffer = 1024 * 1024
	}
	scanner.Buffer(make([]byte, 64*1024), maxBuffer)

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("failed to start command %s: %w", s.command, err)
	}

	s.cmd = cmd
	s.stdin = stdin
	s.scanner = scanner
	s.started = true
	s.healthy = true
	s.pending = make(map[int64]chan rpcResponse)
	s.waitErr = make(chan error, 1)
	s.tools = nil
	s.toolsOnce = false

	go s.readLoop()
	go s.waitLoop(cmd, s.waitErr)
	go s.drainStderr(stderr)
	return nil
}

func (s *StdioServer) buildEnv() []string {
	var env []string
	if s.inheritEnv {
		env = append(env, os.Environ()...)
	} else {
		for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "PATHEXT"} {
			if value := os.Getenv(key); strings.TrimSpace(value) != "" {
				env = append(env, key+"="+value)
			}
		}
	}
	for k, v := range s.env {
		k = strings.TrimSpace(k)
		if k == "" || strings.Contains(k, "=") {
			continue
		}
		env = append(env, k+"="+v)
	}
	return env
}

func (s *StdioServer) readLoop() {
	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" {
			continue
		}
		if len(line) > s.maxResponseSize {
			s.failPending(fmt.Errorf("mcp server %s response exceeded %d bytes", s.name, s.maxResponseSize))
			return
		}
		var resp rpcResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			logging.Warning("[mcp %s] ignored invalid JSON-RPC line: %v", s.name, err)
			continue
		}
		if resp.ID == nil {
			logging.Debug("[mcp %s] notification: %s", s.name, line)
			continue
		}
		ch := s.takePending(*resp.ID)
		if ch == nil {
			logging.Debug("[mcp %s] dropped late JSON-RPC response id=%d", s.name, *resp.ID)
			continue
		}
		ch <- resp
		close(ch)
	}
	if err := s.scanner.Err(); err != nil {
		s.failPending(fmt.Errorf("mcp server %s reader error: %w", s.name, err))
		return
	}
	s.failPending(fmt.Errorf("mcp server %s stdout closed", s.name))
}

func (s *StdioServer) waitLoop(cmd *exec.Cmd, waitErr chan error) {
	if cmd == nil {
		return
	}
	err := cmd.Wait()
	s.mu.Lock()
	if waitErr != nil {
		waitErr <- err
		close(waitErr)
	}
	s.started = false
	s.healthy = false
	s.mu.Unlock()
}

func (s *StdioServer) drainStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 16*1024), 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			logging.Debug("[mcp %s stderr] %s", s.name, line)
		}
	}
}

func (s *StdioServer) takePending(id int64) chan rpcResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := s.pending[id]
	delete(s.pending, id)
	return ch
}

func (s *StdioServer) failPending(err error) {
	logging.Warning("[mcp %s] %v", s.name, err)
	s.mu.Lock()
	s.healthy = false
	for id, ch := range s.pending {
		delete(s.pending, id)
		close(ch)
	}
	s.mu.Unlock()
}

// Initialize performs the MCP handshake and caches the server tool list.
func (s *StdioServer) Initialize(ctx context.Context) error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), s.startupTimeout)
		defer cancel()
	}
	params := map[string]any{
		"protocolVersion": defaultProtocolVersion,
		"capabilities": map[string]any{
			"roots": map[string]any{"listChanged": false},
		},
		"clientInfo": map[string]any{
			"name":    "codrax",
			"version": "0.1",
		},
	}
	if _, err := s.request(ctx, "initialize", params); err != nil {
		return fmt.Errorf("mcp initialize %s: %w", s.name, err)
	}
	if err := s.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("mcp initialized notification %s: %w", s.name, err)
	}
	tools, err := s.fetchTools(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.tools = tools
	s.toolsOnce = true
	s.mu.Unlock()
	return nil
}

func (s *StdioServer) fetchTools(ctx context.Context) ([]ToolSchema, error) {
	raw, err := s.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("mcp tools/list %s: %w", s.name, err)
	}
	var decoded struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("mcp tools/list %s invalid result: %w", s.name, err)
	}
	out := make([]ToolSchema, 0, len(decoded.Tools))
	seenNamespaced := map[string]string{}
	for _, t := range decoded.Tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		namespaced := NamespacedToolName(s.name, name)
		if prev, exists := seenNamespaced[namespaced]; exists {
			return nil, fmt.Errorf("mcp server %s tool names %q and %q collide after namespacing as %q", s.name, prev, name, namespaced)
		}
		seenNamespaced[namespaced] = name
		params := t.InputSchema
		if len(bytesTrimSpace(params)) == 0 {
			params = defaultToolParameters
		}
		out = append(out, ToolSchema{
			Name:        name,
			Description: strings.TrimSpace(t.Description),
			Parameters:  params,
		})
	}
	return out, nil
}

// Name returns the server name.
func (s *StdioServer) Name() string { return s.name }

// Transport returns the transport type.
func (s *StdioServer) Transport() types.TransportType { return types.TransportStdio }

// ListTools returns the initialized tool cache.
func (s *StdioServer) ListTools() []ToolSchema {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ToolSchema(nil), s.tools...)
}

// CallTool invokes a tool on the MCP server.
func (s *StdioServer) CallTool(name string, params json.RawMessage) (types.MCPResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.callTimeout)
	defer cancel()
	raw, err := s.request(ctx, "tools/call", map[string]json.RawMessage{
		"name":      mustMarshalRaw(name),
		"arguments": params,
	})
	if err != nil {
		return types.MCPResponse{
			ServerName: s.name,
			Method:     "tools/call:" + name,
			Summary:    err.Error(),
			Success:    false,
			Timestamp:  time.Now(),
		}, err
	}
	summary, mime, isErr := decodeToolCallResult(raw)
	resp := types.MCPResponse{
		ServerName: s.name,
		Method:     "tools/call:" + name,
		Summary:    summary,
		MIMEType:   mime,
		Success:    !isErr,
		Timestamp:  time.Now(),
	}
	if isErr {
		return resp, fmt.Errorf("mcp tool %s returned isError", name)
	}
	return resp, nil
}

func (s *StdioServer) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if !s.started || !s.healthy || s.stdin == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("stdio server %s not connected", s.name)
	}
	id := s.nextID.Add(1)
	ch := make(chan rpcResponse, 1)
	s.pending[id] = ch
	s.mu.Unlock()

	paramsRaw, err := json.Marshal(params)
	if err != nil {
		s.removePending(id)
		return nil, err
	}
	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: paramsRaw}
	line, err := json.Marshal(req)
	if err != nil {
		s.removePending(id)
		return nil, err
	}

	s.writeMu.Lock()
	_, err = s.stdin.Write(append(line, '\n'))
	s.writeMu.Unlock()
	if err != nil {
		s.removePending(id)
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case <-ctx.Done():
		s.removePending(id)
		return nil, ctx.Err()
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("stdio server %s disconnected", s.name)
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		if len(resp.Result) > s.maxResponseSize {
			return nil, fmt.Errorf("mcp response exceeded %d bytes", s.maxResponseSize)
		}
		return resp.Result, nil
	}
}

func (s *StdioServer) removePending(id int64) {
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
}

func (s *StdioServer) notify(method string, params any) error {
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	line, err := json.Marshal(rpcNotification{JSONRPC: "2.0", Method: method, Params: paramsRaw})
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.stdin.Write(append(line, '\n'))
	return err
}

// Close terminates the child process and cleans up resources.
func (s *StdioServer) Close() error {
	s.mu.Lock()
	if !s.started && s.cmd == nil {
		s.mu.Unlock()
		return nil
	}
	stdin := s.stdin
	cmd := s.cmd
	waitErr := s.waitErr
	s.started = false
	s.healthy = false
	s.stdin = nil
	s.cmd = nil
	for id, ch := range s.pending {
		delete(s.pending, id)
		close(ch)
	}
	s.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		killCommandProcessGroup(cmd)
	}
	if waitErr != nil {
		select {
		case <-waitErr:
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}

func decodeToolCallResult(raw json.RawMessage) (summary, mime string, isErr bool) {
	var decoded struct {
		Content []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			Data     string          `json:"data"`
			MIMEType string          `json:"mimeType"`
			Resource json.RawMessage `json:"resource"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw), "", false
	}
	var parts []string
	for _, c := range decoded.Content {
		if mime == "" {
			mime = strings.TrimSpace(c.MIMEType)
		}
		switch {
		case c.Text != "":
			parts = append(parts, c.Text)
		case c.Data != "":
			parts = append(parts, c.Data)
		case len(c.Resource) > 0:
			parts = append(parts, string(c.Resource))
		default:
			b, _ := json.Marshal(c)
			parts = append(parts, string(b))
		}
	}
	if len(parts) == 0 {
		return string(raw), mime, decoded.IsError
	}
	return strings.Join(parts, "\n"), mime, decoded.IsError
}

func mustMarshalRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func bytesTrimSpace(raw json.RawMessage) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}
