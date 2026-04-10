package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ---------------------------------------------------------------------------
// ExecCommand
// ---------------------------------------------------------------------------

// ExecCommand runs a shell command with a configurable timeout.
//
// Classified as read-only by intent so that explorer/planner/verifier
// skills can keep using it. A shell can of course mutate the filesystem
// (`cat > foo`, `rm`, `mv`); restricting that requires a separate
// command allowlist or sandbox, not the requires_write boundary.
type ExecCommand struct{ ReadOnly }

type execCommandParams struct {
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

func (t *ExecCommand) Name() string        { return "exec_command" }
func (t *ExecCommand) Description() string { return "Run a shell command with configurable timeout" }

func (t *ExecCommand) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command":    {"type": "string",  "description": "Shell command to execute"},
    "timeout_ms": {"type": "integer", "description": "Timeout in milliseconds (default 30000)"}
  },
  "required": ["command"]
}`)
}

func (t *ExecCommand) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p execCommandParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}

	timeout := 30 * time.Second
	if p.TimeoutMs > 0 {
		timeout = time.Duration(p.TimeoutMs) * time.Millisecond
	}

	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "sh", "-c", p.Command)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()

	if execCtx.Err() == context.DeadlineExceeded {
		preview, ref := StoreBlob(ctx, t.Name()+"-timeout", output)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("command timed out after %v\n%s", timeout, preview),
			RawRef:    ref,
			Timestamp: time.Now(),
		}, fmt.Errorf("command timed out after %v", timeout)
	}

	if err != nil {
		preview, ref := StoreBlob(ctx, t.Name()+"-fail", output)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("command failed: %v\n%s", err, preview),
			RawRef:    ref,
			Timestamp: time.Now(),
		}, nil
	}

	summary, ref := StoreBlob(ctx, t.Name(), output)
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

// ---------------------------------------------------------------------------
// GrepTool
// ---------------------------------------------------------------------------

// GrepTool searches file contents by pattern using grep.
type GrepTool struct{ ReadOnly }

type grepToolParams struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Include    string `json:"include,omitempty"`
	IgnoreCase *bool  `json:"ignore_case,omitempty"`
}

func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return "Search file contents by regex pattern. Use this to find where a symbol or string appears. Case handling is smart by default: if the pattern contains no uppercase characters it is matched case-insensitively (so `subagent` finds `SubAgent`, `SUBAGENT`, etc.); if the pattern contains any uppercase character it is matched exactly. Pass ignore_case explicitly to override. Do NOT use the result to count matches by eye — pipe `grep -c` or `grep ... | wc -l` through exec_command instead, and treat that number as authoritative."
}

func (t *GrepTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern":     {"type": "string",  "description": "Regex pattern to search for"},
    "path":        {"type": "string",  "description": "Directory or file to search (default: current directory)"},
    "include":     {"type": "string",  "description": "File glob filter, e.g. *.go"},
    "ignore_case": {"type": "boolean", "description": "Case-insensitive match. Omit for smart-case (insensitive iff pattern has no uppercase). Set true/false to force."}
  },
  "required": ["pattern"]
}`)
}

func (t *GrepTool) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p grepToolParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}

	searchPath := p.Path
	if searchPath == "" {
		searchPath = "."
	}

	// Smart-case: if the LLM didn't pass ignore_case explicitly, fall
	// back to ripgrep's smart-case rule — case-insensitive iff the
	// pattern contains no uppercase character. The motivating failure:
	// the LLM grep'd "subagent" expecting to find the CamelCase
	// SubAgent / SubAgentRegistry / RegisterDefaultSubAgents symbols,
	// got 4 noise hits (an error string and three log lines), then
	// deflected its investigation into struct-listing and head-only
	// reads of the wrong files. The right answer was one targeted
	// grep away the whole time. Smart-case matches LLM intuition (a
	// lowercase query is conceptually case-blind) without forcing the
	// model to remember a flag.
	caseInsensitive := false
	if p.IgnoreCase != nil {
		caseInsensitive = *p.IgnoreCase
	} else {
		caseInsensitive = !strings.ContainsAny(p.Pattern, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	}

	args := []string{"-rn"}
	if caseInsensitive {
		args = append(args, "-i")
	}
	args = append(args, p.Pattern)
	if p.Include != "" {
		args = append(args, "--include="+p.Include)
	}
	args = append(args, searchPath)

	cmd := exec.Command("grep", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()

	// grep returns exit code 1 when no matches found — not a real error.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   true,
				Summary:   "no matches found",
				Timestamp: time.Now(),
			}, nil
		}
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("grep failed: %v: %s", err, stderr.String()),
			Timestamp: time.Now(),
		}, nil
	}

	summary, ref := StoreBlob(ctx, t.Name(), output)
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

// ---------------------------------------------------------------------------
// ReadFile
// ---------------------------------------------------------------------------

// ReadFile reads file contents with optional line-based offset and limit.
type ReadFile struct{ ReadOnly }

type readFileParams struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func (t *ReadFile) Name() string { return "read_file" }
func (t *ReadFile) Description() string {
	return "Read file contents. The offset/limit parameters exist for paging files too large to inline (>32KB); for files smaller than that, the whole file is always returned regardless of offset/limit, because slicing a small file silently hides the answer (e.g. asking for offset=0,limit=20 on a 66-line file). The slice/total banner at the top of the result tells you exactly which lines you saw out of how many."
}

func (t *ReadFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":   {"type": "string",  "description": "Path to the file to read"},
    "offset": {"type": "integer", "description": "Starting line number (0-based, optional)"},
    "limit":  {"type": "integer", "description": "Maximum number of lines to return (optional)"}
  },
  "required": ["path"]
}`)
}

func (t *ReadFile) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p readFileParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}

	data, err := os.ReadFile(p.Path)
	if err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("read failed: %v", err), Timestamp: time.Now()}, nil
	}

	content := string(data)
	allLines := strings.Split(content, "\n")
	totalLines := len(allLines)

	// Small-file passthrough: if the whole file fits in the inline
	// budget, ignore offset/limit and return everything. The LLM
	// routinely passes a foolish limit (e.g. limit=20) on a 66-line
	// source file because "20" is a popular default in its training
	// distribution; on a small file that limit can hide the answer
	// just past the cutoff. The offset/limit knobs exist for paging
	// files too large to inline, so honoring them on a tiny file is
	// strictly worse than ignoring them. The banner records that we
	// did so, in case the slice was meaningful to the caller.
	sliceStart := 0
	sliceEnd := totalLines
	overrode := false
	if (p.Offset > 0 || p.Limit > 0) && len(data) > MaxInlineBytes {
		sliceStart = p.Offset
		if sliceStart > totalLines {
			sliceStart = totalLines
		}
		sliceEnd = totalLines
		if p.Limit > 0 && sliceStart+p.Limit < sliceEnd {
			sliceEnd = sliceStart + p.Limit
		}
		content = strings.Join(allLines[sliceStart:sliceEnd], "\n")
	} else if p.Offset > 0 || p.Limit > 0 {
		overrode = true
	}

	// Always prepend a slice/total banner so the LLM cannot mistake
	// a partial read for the whole file. The previous design returned
	// the requested slice with no metadata, so a 40-line read of a
	// 330-line file looked identical to a 40-line read of a 40-line
	// file — and the model would routinely conclude after the head
	// that it had seen everything. Banner is plain text, no prescription.
	var banner string
	if overrode {
		banner = fmt.Sprintf("[%s: showing all %d lines (%d bytes); offset/limit ignored because the file fits inline]\n",
			p.Path, totalLines, len(data))
	} else {
		banner = fmt.Sprintf("[%s: showing lines %d-%d of %d total]\n", p.Path, sliceStart+1, sliceEnd, totalLines)
	}
	content = banner + content

	summary, ref := StoreBlobHeadOnly(ctx, t.Name(), content)
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

// ---------------------------------------------------------------------------
// ListFiles
// ---------------------------------------------------------------------------

// ListFiles lists files in a directory, optionally recursively.
type ListFiles struct{ ReadOnly }

type listFilesParams struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

func (t *ListFiles) Name() string { return "list_files" }
func (t *ListFiles) Description() string {
	return "List files in a directory. Use this for navigation and discovery only. Do NOT use the result to count files, filter by extension, or sort — for any of those, run a shell pipeline through exec_command (e.g. `find . -name '*.go' | wc -l`) and trust its output."
}

func (t *ListFiles) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":      {"type": "string",  "description": "Directory path to list"},
    "recursive": {"type": "boolean", "description": "List recursively (default false)"}
  },
  "required": ["path"]
}`)
}

func (t *ListFiles) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p listFilesParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}

	var files []string

	if p.Recursive {
		err := filepath.WalkDir(p.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// Skip well-known noise directories that explode the walk
			// without yielding useful entries (VCS metadata, dependency
			// caches, hidden tooling). Mirrors RepoMap's behavior.
			if d.IsDir() && path != p.Path {
				name := d.Name()
				if name == ".git" || name == "node_modules" || name == "vendor" || name == ".venv" || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("walk failed: %v", err), Timestamp: time.Now()}, nil
		}
	} else {
		entries, err := os.ReadDir(p.Path)
		if err != nil {
			return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("readdir failed: %v", err), Timestamp: time.Now()}, nil
		}
		for _, e := range entries {
			files = append(files, filepath.Join(p.Path, e.Name()))
		}
	}

	summary, ref := StoreBlob(ctx, t.Name(), strings.Join(files, "\n"))
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

// ---------------------------------------------------------------------------
// RepoMap
// ---------------------------------------------------------------------------

// RepoMap generates a repository structure tree.
type RepoMap struct{ ReadOnly }

type repoMapParams struct {
	Path     string `json:"path"`
	MaxDepth int    `json:"max_depth,omitempty"`
}

func (t *RepoMap) Name() string        { return "repo_map" }
func (t *RepoMap) Description() string { return "Generate a repository structure tree" }

func (t *RepoMap) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":      {"type": "string",  "description": "Root path for the tree"},
    "max_depth": {"type": "integer", "description": "Maximum directory depth (default unlimited)"}
  },
  "required": ["path"]
}`)
}

func (t *RepoMap) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p repoMapParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}

	rootPath := p.Path
	maxDepth := p.MaxDepth

	var buf strings.Builder
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		rel, relErr := filepath.Rel(rootPath, path)
		if relErr != nil {
			return nil
		}

		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(os.PathSeparator)) + 1
		}

		if maxDepth > 0 && depth > maxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden directories like .git
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && rel != "." {
			return filepath.SkipDir
		}

		indent := strings.Repeat("  ", depth)
		name := info.Name()
		if info.IsDir() {
			name += "/"
		}
		buf.WriteString(indent + name + "\n")
		return nil
	})
	if err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("walk failed: %v", err), Timestamp: time.Now()}, nil
	}

	summary, ref := StoreBlob(ctx, t.Name(), buf.String())
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

// ---------------------------------------------------------------------------
// RunTests
// ---------------------------------------------------------------------------

// RunTests runs an explicit test command. There is no language- or
// build-system default: the caller must always pass a command. The
// previous design fell back to "go test ./..." on empty input, which
// quietly produced confusing failures in non-Go repositories and gave
// the LLM a free pass to skip thinking about which tests are
// actually relevant. Forcing the command makes both errors loud.
//
// Classified as read-only by intent: its purpose is observation, not
// mutation. The verifier (a read-only agent) depends on it. Test code
// could in principle write files, but that risk is shared with any
// language runtime and is out of scope for this boundary.
type RunTests struct{ ReadOnly }

type runTestsParams struct {
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
}

func (t *RunTests) Name() string { return "run_tests" }
func (t *RunTests) Description() string {
	return "Run a test command. The command parameter is required — pick the test invocation that matches the project's build system (e.g. 'go test ./...', 'pytest', 'npm test'). Working directory is optional."
}

func (t *RunTests) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {"type": "string", "description": "Test command to run, e.g. 'go test ./...', 'pytest -q', 'npm test'. Required."},
    "path":    {"type": "string", "description": "Working directory for the command (optional)."}
  },
  "required": ["command"]
}`)
}

func (t *RunTests) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p runTestsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}

	command := strings.TrimSpace(p.Command)
	if command == "" {
		// Surface as a tool failure rather than a hard error so the
		// agent receives an actionable message in the next observation
		// step and can retry with a real command.
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "run_tests requires the 'command' parameter — there is no default. Pass the test invocation for this project (e.g. 'go test ./...', 'pytest -q', 'npm test').",
			Timestamp: time.Now(),
		}, nil
	}

	cmd := exec.Command("sh", "-c", command)
	if p.Path != "" {
		cmd.Dir = p.Path
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()

	if err != nil {
		preview, ref := StoreBlob(ctx, t.Name()+"-fail", output)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("tests failed: %v\n%s", err, preview),
			RawRef:    ref,
			Timestamp: time.Now(),
		}, nil
	}

	summary, ref := StoreBlob(ctx, t.Name(), output)
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

// ---------------------------------------------------------------------------
// GitDiff
// ---------------------------------------------------------------------------

// GitDiff runs git diff with optional staging and ref options.
type GitDiff struct{ ReadOnly }

type gitDiffParams struct {
	Path   string `json:"path,omitempty"`
	Staged bool   `json:"staged,omitempty"`
	Ref    string `json:"ref,omitempty"`
}

func (t *GitDiff) Name() string        { return "git_diff" }
func (t *GitDiff) Description() string { return "Run git diff" }

func (t *GitDiff) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":   {"type": "string",  "description": "Repository path (working directory)"},
    "staged": {"type": "boolean", "description": "Show staged changes (--cached)"},
    "ref":    {"type": "string",  "description": "Git ref to diff against"}
  }
}`)
}

func (t *GitDiff) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p gitDiffParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}

	args := []string{"diff"}
	if p.Staged {
		args = append(args, "--cached")
	}
	if p.Ref != "" {
		args = append(args, p.Ref)
	}

	cmd := exec.Command("git", args...)
	if p.Path != "" {
		cmd.Dir = p.Path
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("git diff failed: %v: %s", err, stderr.String()),
			Timestamp: time.Now(),
		}, nil
	}

	summary, ref := StoreBlob(ctx, t.Name(), stdout.String())
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

// ---------------------------------------------------------------------------
// GitLog
// ---------------------------------------------------------------------------

// GitLog runs git log with configurable count and format.
type GitLog struct{ ReadOnly }

type gitLogParams struct {
	Path   string `json:"path,omitempty"`
	Count  int    `json:"count,omitempty"`
	Format string `json:"format,omitempty"`
}

func (t *GitLog) Name() string        { return "git_log" }
func (t *GitLog) Description() string { return "Run git log" }

func (t *GitLog) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":   {"type": "string",  "description": "Repository path (working directory)"},
    "count":  {"type": "integer", "description": "Number of commits to show (default 10)"},
    "format": {"type": "string",  "description": "Log format: oneline, short, medium, full (default oneline)"}
  }
}`)
}

func (t *GitLog) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p gitLogParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}

	count := p.Count
	if count <= 0 {
		count = 10
	}
	format := p.Format
	if format == "" {
		format = "oneline"
	}

	args := []string{"log", fmt.Sprintf("-n%d", count), fmt.Sprintf("--format=%s", format)}

	cmd := exec.Command("git", args...)
	if p.Path != "" {
		cmd.Dir = p.Path
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("git log failed: %v: %s", err, stderr.String()),
			Timestamp: time.Now(),
		}, nil
	}

	summary, ref := StoreBlob(ctx, t.Name(), stdout.String())
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}
