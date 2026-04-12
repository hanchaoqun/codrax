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

// defaultExcludeDirs is an alias for the shared ExcludeDirs list in
// search.go, used by the grep tool and keyword search.
var defaultExcludeDirs = ExcludeDirs

// ---------------------------------------------------------------------------
// ExecCommand
// ---------------------------------------------------------------------------

// ExecCommand runs a shell command with a configurable timeout.
//
// Classified as read-only by intent so that explorer/planner/verifier
// skills can keep using it. A shell can of course mutate the filesystem
// (`cat > foo`, `rm`, `mv`); restricting that requires a separate
// command allowlist or sandbox, not the requires_write boundary.
type ExecCommand struct {
	ReadOnly
	EvidenceTool
}

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

	cmd := NewShellCommandContext(execCtx, p.Command)
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
type GrepTool struct {
	ReadOnly
	EvidenceTool
}

type grepToolParams struct {
	Pattern      string `json:"pattern"`
	Path         string `json:"path,omitempty"`
	Include      string `json:"include,omitempty"`
	FileType     string `json:"file_type,omitempty"`
	ContextLines *int   `json:"context_lines,omitempty"`
	IgnoreCase   *bool  `json:"ignore_case,omitempty"`
	FilesOnly    bool   `json:"files_only,omitempty"`
}

func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return "Search file contents by regex pattern. Use this to find where a symbol or string appears. Case handling is smart by default: if the pattern contains no uppercase characters it is matched case-insensitively (so `subagent` finds `SubAgent`, `SUBAGENT`, etc.); if the pattern contains any uppercase character it is matched exactly. Pass ignore_case explicitly to override. Use file_type to filter by language (e.g. \"go\", \"py\", \"js\", \"ts\", \"java\", \"yaml\") — this is preferred over include because it covers all relevant extensions (e.g. type \"ts\" matches both *.ts and *.tsx). Use context_lines to include surrounding lines around each match, which saves a follow-up read_file call. Do NOT use the result to count matches by eye — pipe `grep -c` or `grep ... | wc -l` through exec_command instead, and treat that number as authoritative."
}

func (t *GrepTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern":       {"type": "string",  "description": "Regex pattern to search for"},
    "path":          {"type": "string",  "description": "Directory or file to search (default: current directory)"},
    "include":       {"type": "string",  "description": "File glob filter, e.g. *.go. Prefer file_type when filtering by language."},
    "file_type":     {"type": "string",  "description": "Language/file type filter: go, py, js, ts, java, rust, ruby, c, cpp, yaml, json, toml, markdown, config, etc. Covers all relevant extensions (e.g. ts matches *.ts and *.tsx). Preferred over include for language filtering."},
    "context_lines": {"type": "integer", "description": "Number of lines to show before and after each match (like grep -C). Use this to see surrounding code without a separate read_file call. Ignored when files_only is true."},
    "ignore_case":   {"type": "boolean", "description": "Case-insensitive match. Omit for smart-case (insensitive iff pattern has no uppercase). Set true/false to force."},
    "files_only":    {"type": "boolean", "description": "If true, return only file paths that contain matches (like grep -l), not the matching lines. Useful for breadth scans to discover which files are relevant without flooding the output."}
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

	// All search commands are guarded by a 60 s timeout to prevent
	// hangs on large repos or pathological regex patterns.
	searchCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var cmd *exec.Cmd

	// Resolve context_lines: only meaningful for content mode, clamped to [0,20].
	contextLines := 0
	if p.ContextLines != nil && !p.FilesOnly {
		contextLines = *p.ContextLines
		if contextLines < 0 {
			contextLines = 0
		} else if contextLines > 20 {
			contextLines = 20
		}
	}

	if UseRipgrep() {
		// ripgrep: ERE-compatible by default, auto-skips binary files,
		// respects .gitignore (auto-excludes logs/, memory/, etc.).
		// -H forces filename prefix on every line — required because
		// ripgrep otherwise DROPS filenames when the search path is a
		// single file, producing `158:content` instead of
		// `file.go:158:content`. Downstream extractFileCoverage then
		// parses the lineno as if it were a path, inflating discovered
		// files with dozens of bogus entries per grep call.
		args := []string{"-n", "-H"}
		if p.FilesOnly {
			args = []string{"-l"}
		}
		if contextLines > 0 {
			args = append(args, "-C", fmt.Sprintf("%d", contextLines))
		}
		if caseInsensitive {
			args = append(args, "-i")
		} else {
			args = append(args, "--case-sensitive")
		}
		// Add manual exclusions for dirs not in .gitignore.
		for _, dir := range defaultExcludeDirs {
			args = append(args, "--glob", "!"+dir+"/")
		}
		if p.FileType != "" {
			args = append(args, "--type", p.FileType)
		}
		if p.Include != "" {
			args = append(args, "--glob", p.Include)
		}
		args = append(args, p.Pattern, searchPath)
		cmd = exec.CommandContext(searchCtx, SearchExecutable(), args...)
	} else {
		// GNU grep fallback: -E for ERE, -I to skip binary files,
		// -H forces filename prefix (see ripgrep comment above for why
		// single-file paths break extractFileCoverage without it).
		args := []string{"-rnEIH"}
		if p.FilesOnly {
			args = []string{"-rlEI"}
		}
		if contextLines > 0 {
			args = append(args, fmt.Sprintf("-C%d", contextLines))
		}
		if caseInsensitive {
			args = append(args, "-i")
		}
		for _, dir := range defaultExcludeDirs {
			args = append(args, "--exclude-dir="+dir)
		}
		args = append(args, p.Pattern)
		// file_type → --include globs for GNU grep (no --type support).
		if p.FileType != "" {
			for _, glob := range fileTypeToGlobs(p.FileType) {
				args = append(args, "--include="+glob)
			}
		}
		if p.Include != "" {
			args = append(args, "--include="+p.Include)
		}
		args = append(args, searchPath)
		cmd = exec.CommandContext(searchCtx, SearchExecutable(), args...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()

	// Timeout produces a context.DeadlineExceeded — surface it clearly.
	if searchCtx.Err() == context.DeadlineExceeded {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "grep timed out after 60s — pattern may be too broad or repo too large",
			Timestamp: time.Now(),
		}, nil
	}

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

	// Prepend a count banner so the LLM knows whether the blob was
	// truncated. Without this, a blob-trimmed result looks identical
	// to a short result and the model cannot judge completeness.
	lines := strings.Count(output, "\n")
	if output != "" && !strings.HasSuffix(output, "\n") {
		lines++ // last line has no trailing newline
	}
	if lines > 0 {
		unit := "matching lines"
		if p.FilesOnly {
			unit = "matching files"
		}
		output = fmt.Sprintf("[grep: %d %s]\n%s", lines, unit, output)
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

// fileTypeToGlobs maps a language/file type name to glob patterns,
// mirroring ripgrep's built-in --type definitions for the GNU grep
// fallback. Only the most commonly used types are listed; unknown
// types fall back to a best-effort "*.type" glob.
func fileTypeToGlobs(ft string) []string {
	switch strings.ToLower(ft) {
	case "go":
		return []string{"*.go"}
	case "py", "python":
		return []string{"*.py"}
	case "js", "javascript":
		return []string{"*.js", "*.jsx", "*.vue"}
	case "ts", "typescript":
		return []string{"*.ts", "*.tsx"}
	case "java":
		return []string{"*.java", "*.jsp", "*.properties"}
	case "rust", "rs":
		return []string{"*.rs"}
	case "ruby", "rb":
		return []string{"*.rb", "*.gemspec"}
	case "c":
		return []string{"*.[ch]"}
	case "cpp":
		return []string{"*.cpp", "*.hpp", "*.cc", "*.hh", "*.cxx", "*.hxx"}
	case "yaml":
		return []string{"*.yaml", "*.yml"}
	case "json":
		return []string{"*.json"}
	case "toml":
		return []string{"*.toml"}
	case "markdown", "md":
		return []string{"*.md", "*.markdown"}
	case "config":
		return []string{"*.cfg", "*.conf", "*.config", "*.ini"}
	case "css":
		return []string{"*.css", "*.scss"}
	case "html":
		return []string{"*.html", "*.htm"}
	case "sh", "shell", "bash":
		return []string{"*.sh", "*.bash"}
	case "sql":
		return []string{"*.sql"}
	case "proto", "protobuf":
		return []string{"*.proto"}
	default:
		return []string{"*." + ft}
	}
}

// ---------------------------------------------------------------------------
// ReadFile
// ---------------------------------------------------------------------------

// ReadFile reads file contents with optional line-based offset and limit.
type ReadFile struct {
	ReadOnly
	EvidenceTool
}

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
		// Clamp sliceEnd so the banner matches what StoreBlobHeadOnly
		// will actually show. Without this, the banner claims "showing
		// lines X-Y" but blob truncation silently drops lines past the
		// head budget, and the LLM pages from Y+1 — skipping the
		// truncated lines entirely.
		if len(content) > MaxInlineBytes {
			headBudget := PreviewHeadBytesValue()
			clamped := content
			if len(clamped) > headBudget {
				clamped = clamped[:headBudget]
			}
			// Walk back to the last complete line.
			if idx := strings.LastIndex(clamped, "\n"); idx > 0 {
				clamped = clamped[:idx]
			}
			visibleLines := strings.Count(clamped, "\n") + 1
			sliceEnd = sliceStart + visibleLines
			content = strings.Join(allLines[sliceStart:sliceEnd], "\n")
		}
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
type ListFiles struct {
	ReadOnly
	EvidenceTool
}

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
// RepoMap — moved to internal/tool/repomap/ package (tree-sitter powered).

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
type RunTests struct {
	ReadOnly
	EvidenceTool
}

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

	cmd := NewShellCommandContext(context.Background(), command)
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
type GitDiff struct {
	ReadOnly
	EvidenceTool
}

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
type GitLog struct {
	ReadOnly
	EvidenceTool
}

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
