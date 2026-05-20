package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/types"
)

// toolBannerMaxValueLen caps each k=v value in the per-tool params
// banner so a pathological command / pattern does not blow the
// banner into pages. 200 is roomy for typical shell pipelines and
// regex patterns while still fitting on one terminal line.
const toolBannerMaxValueLen = 200

// sanitizeForBanner renders a param value as a single banner-safe
// token: control chars become spaces, the result is truncated to
// toolBannerMaxValueLen and any truncation is marked with an
// ellipsis. The banner lives at the top of ToolResult.Summary and is
// the ONLY path through which Turn A call provenance reaches Turn B
// (extractor / finalizer) — see internal/context/builder.go's
// formatRawToolSummary, which only renders ToolName + Summary.
func sanitizeForBanner(s string) string {
	if s == "" {
		return ""
	}
	// Replace each control / newline character with a single space so
	// the banner stays on one line; collapsing runs of spaces is not
	// necessary because downstream parsers don't care about extra
	// whitespace and the LLM tolerates it fine.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r < 0x20 {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if len(out) > toolBannerMaxValueLen {
		out = out[:toolBannerMaxValueLen-3] + "..."
	}
	return out
}

// resolveToolPath maps an LLM-supplied path to a real filesystem path
// rooted at ctx.RepoRoot. The LLM treats the repo root as its own
// CWD ("." means "the repo I'm investigating"), but the codrax process
// CWD is wherever the user invoked the binary — typically a different
// directory. Without this resolution, a tool call like
// `repo_map(path=".")` or `read_file(path="internal/foo.go")` would
// scan or open the wrong tree, and the LLM would faithfully cite
// content from the wrong repo (the Q2 glamour-vs-codrax confusion).
//
// Rules:
//   - ctx.RepoRoot empty (zero-value BusContext in unit tests) →
//     return p unchanged so existing tests keep working without a fake
//     RepoRoot.
//   - p empty or "." → return ctx.RepoRoot.
//   - p absolute → return as-is. The LLM occasionally pastes a path
//     it saw in earlier output (already absolute); re-rooting it would
//     produce a nonsense `RepoRoot/abs/path/...`.
//   - p relative → join under ctx.RepoRoot.
func resolveToolPath(ctx *types.BusContext, p string) string {
	if ctx == nil || ctx.RepoRoot == "" {
		return normalizeToolAbsolutePath(p)
	}
	if p == "" || p == "." {
		return ctx.RepoRoot
	}
	if normalized, ok := normalizeWindowsPOSIXPath(p); ok {
		return normalized
	}
	if toolPathIsAbs(p) {
		return p
	}
	return filepath.Join(ctx.RepoRoot, p)
}

func resolveRepoScopedToolDir(ctx *types.BusContext, p string) (string, string) {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return resolveToolPath(ctx, p), ""
	}
	root := normalizeToolAbsolutePath(ctx.RepoRoot)
	raw := strings.TrimSpace(p)
	if raw == "" || raw == "." {
		return root, ""
	}
	if normalized, ok := normalizeWindowsPOSIXPath(raw); ok {
		raw = normalized
	}
	if toolPathIsAbs(raw) {
		abs := normalizeToolAbsolutePath(raw)
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") {
			return "", fmt.Sprintf("invalid params: path %q is outside repository root %q; use a repo-relative path", p, root)
		}
		return abs, ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(raw))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Sprintf("invalid params: path %q escapes repository root %q; use a repo-relative path", p, root)
	}
	return filepath.Join(root, raw), ""
}

func normalizeToolAbsolutePath(p string) string {
	if normalized, ok := normalizeWindowsPOSIXPath(p); ok {
		return normalized
	}
	return p
}

type attachmentReadPolicy struct {
	blobPath      string
	blobName      string
	artifactLabel string
	emitTools     string
}

func readFileAttachmentPolicy(ctx *types.BusContext) (attachmentReadPolicy, bool) {
	if ctx == nil {
		return attachmentReadPolicy{}, false
	}
	switch {
	case ctx.ActiveAgent == types.AgentLogTriager || ctx.PipelineStage == types.StageLogTriage:
		return attachmentReadPolicy{
			blobPath:      attachedArtifactBlobPath(ctx.WorkDir, promptctx.AttachedLogBlobName),
			blobName:      promptctx.AttachedLogBlobName,
			artifactLabel: "runtime log",
			emitTools:     "emit_log_triage / emit_log_segmentation",
		}, true
	case ctx.ActiveAgent == types.AgentPerfTriager || ctx.PipelineStage == types.StagePerfTriage:
		return attachmentReadPolicy{
			blobPath:      attachedArtifactBlobPath(ctx.WorkDir, promptctx.AttachedTraceBlobName),
			blobName:      promptctx.AttachedTraceBlobName,
			artifactLabel: "performance trace",
			emitTools:     "emit_perf_trace / emit_perf_segmentation",
		}, true
	default:
		return attachmentReadPolicy{}, false
	}
}

func attachedArtifactBlobPath(workDir, blobName string) string {
	if strings.TrimSpace(workDir) == "" || strings.TrimSpace(blobName) == "" {
		return ""
	}
	path := filepath.Join(workDir, blobName)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

func resolveAttachmentScopedReadPath(ctx *types.BusContext, requested string, policy attachmentReadPolicy) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return resolveToolPath(ctx, requested)
	}
	trimmed := strings.ReplaceAll(requested, "\\", "/")
	if strings.EqualFold(path.Base(trimmed), policy.blobName) {
		return policy.blobPath
	}
	return resolveToolPath(ctx, requested)
}

func toolPathsEqual(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func resolveReadFilePath(ctx *types.BusContext, requested string) (string, *types.ToolResult) {
	if policy, restricted := readFileAttachmentPolicy(ctx); restricted {
		if policy.blobPath == "" {
			hint := fmt.Sprintf("This dispatch may only paginate the attached %s blob, but no blob path is available. Decide from the inline attached %s section and emit %s directly instead of reading repository files.", policy.artifactLabel, policy.artifactLabel, policy.emitTools)
			return "", &types.ToolResult{
				ToolName: "read_file",
				Success:  false,
				Summary:  fmt.Sprintf("read_file rejected: this dispatch may only paginate the attached %s blob, but no blob path is available. Decide from the inline attached %s section and emit the structured triage tool instead of reading repository files.", policy.artifactLabel, policy.artifactLabel),
				Repair: &types.ToolRepair{
					Code:   "attachment_only_read_file",
					Hint:   hint,
					Fields: []string{"path"},
					Metadata: map[string]string{
						"artifact":   policy.artifactLabel,
						"emit_tools": policy.emitTools,
					},
				},
				Timestamp: time.Now(),
			}
		}
		resolved := resolveAttachmentScopedReadPath(ctx, requested, policy)
		if !toolPathsEqual(resolved, policy.blobPath) {
			hint := fmt.Sprintf("This dispatch may only read the attached %s blob at %s. Re-issue `read_file` on that blob path (or its basename `%s`) if you need pagination; do not read repository source files during triage.", policy.artifactLabel, policy.blobPath, policy.blobName)
			return "", &types.ToolResult{
				ToolName: "read_file",
				Success:  false,
				Summary:  "read_file rejected: triage-stage attachment paging is restricted to the attached artifact blob",
				Repair: &types.ToolRepair{
					Code:   "attachment_only_read_file",
					Hint:   hint,
					Fields: []string{"path"},
					Metadata: map[string]string{
						"allowed_path":     policy.blobPath,
						"allowed_basename": policy.blobName,
						"artifact":         policy.artifactLabel,
						"emit_tools":       policy.emitTools,
					},
				},
				Timestamp: time.Now(),
			}
		}
		return policy.blobPath, nil
	}
	return resolveToolPath(ctx, requested), nil
}

func normalizeWindowsPOSIXPath(p string) (string, bool) {
	if runtime.GOOS != "windows" {
		return "", false
	}
	slash := strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if len(slash) >= 4 && slash[0] == '/' && isASCIIDriveLetter(slash[1]) && slash[2] == '/' {
		return string([]byte{asciiUpper(slash[1]), ':'}) + filepath.FromSlash(slash[2:]), true
	}
	lower := strings.ToLower(slash)
	if strings.HasPrefix(lower, "/mnt/") && len(slash) >= 8 && isASCIIDriveLetter(slash[5]) && slash[6] == '/' {
		return string([]byte{asciiUpper(slash[5]), ':'}) + filepath.FromSlash(slash[6:]), true
	}
	return "", false
}

func isASCIIDriveLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func asciiUpper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - ('a' - 'A')
	}
	return b
}

func toolPathIsAbs(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	slash := strings.ReplaceAll(p, "\\", "/")
	if path.IsAbs(slash) {
		return true
	}
	if len(p) >= 3 &&
		((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) &&
		p[1] == ':' &&
		(p[2] == '\\' || p[2] == '/') {
		return true
	}
	return strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, `//`)
}

// kvBanner renders a tool call's parameters as a single-line banner
// of the shape `[<name>: k1=v1 k2=v2]\n`. Empty values are dropped so
// the banner stays tight. Values are sanitised via sanitizeForBanner.
// Callers pass alternating key/value strings; if an odd count is
// passed the trailing key is ignored.
func kvBanner(name string, kv ...string) string {
	var b strings.Builder
	b.WriteByte('[')
	b.WriteString(name)
	first := true
	for i := 0; i+1 < len(kv); i += 2 {
		v := sanitizeForBanner(kv[i+1])
		if v == "" {
			continue
		}
		if first {
			b.WriteString(": ")
			first = false
		} else {
			b.WriteByte(' ')
		}
		b.WriteString(kv[i])
		b.WriteByte('=')
		b.WriteString(v)
	}
	b.WriteString("]\n")
	return b.String()
}

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

func (t *ExecCommand) Name() string { return "exec_command" }
func (t *ExecCommand) Description() string {
	return "Run a read-only shell command with configurable timeout. In read mode it starts in the repository root, so use repo-relative paths and avoid absolute cd/git -C guesses; large output is saved to a blob that can be paged with read_file."
}

func (t *ExecCommand) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command":    {"type": "string",  "description": "Shell command to execute from the repository root in read mode; use repo-relative paths"},
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
	command := p.Command
	var compatibilityNote string
	if shouldGateExecCommandAsReadOnly(ctx) {
		command, compatibilityNote = normalizeReadOnlyExecCommand(command)
		if err := validateReadOnlyExecCommand(command); err != nil {
			result := readOnlyExecRefusal(ctx, command, err)
			result.Timestamp = time.Now()
			return result, nil
		}
	}

	// Multi-repo active-set gate: the path gate covers read_file /
	// grep / list_files / repo_map but exec_command is the free-form
	// shell surface. Without this branch the LLM can `cat
	// <inactive-sub-repo>/path/file.go` to bypass the workspace scope
	// the user pinned via /repos focus. Single-repo bypass lives
	// inside ResolveActiveSetCommand (m.IsSingle()).
	if ctx != nil && ctx.MultiGraph != nil {
		if gater, ok := ctx.MultiGraph.(types.MultiRepoActiveSetGater); ok && gater != nil {
			gate := gater.ResolveActiveSetCommand(ctx, t.Name(), command)
			if !gate.Allowed {
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   gate.RefusalProse,
					Timestamp: time.Now(),
				}, nil
			}
		}
	}

	timeout := 30 * time.Second
	if p.TimeoutMs > 0 {
		timeout = time.Duration(p.TimeoutMs) * time.Millisecond
	}

	// The `$ <cmd>` banner is the single path through which the
	// executed command reaches Turn B (extractor / finalizer). Without
	// it, a `wc -l` scalar arrives in the Raw Tool Outputs section as
	// just a number, stripped of its provenance.
	banner := fmt.Sprintf("[exec_command: $ %s]\n", sanitizeForBanner(command))
	if compatibilityNote != "" {
		banner = fmt.Sprintf("[exec_command: $ %s]\n[exec_command compatibility: %s]\n", sanitizeForBanner(command), compatibilityNote)
	}

	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Wrap exec_command through the same supervisor + resource-cap
	// path run_tests uses. Two reasons even for a "free-form" tool:
	//   - process-tree teardown on cancel (the OOM event proved
	//     timeout-only kill of the sh wrapper leaks grandchildren)
	//   - memory + CPU caps so an LLM-typed `find / -exec` style
	//     blunder can't tip a low-RAM host into OOM
	caps := verifyResourceCaps()
	wrappedCmd := wrapShellCommandWithCaps(command, caps)
	cmd := NewShellCommandContext(execCtx, wrappedCmd)
	// Anchor the shell at the repo root so `find .`, `wc -l *.go`,
	// `git ls-files` etc. operate on the user's --repo target rather
	// than the codrax process CWD. exec_command has no path param —
	// the LLM expresses the working directory through its command
	// (e.g. `cd subpkg && ...`) and that `cd` is relative to RepoRoot.
	if ctx != nil && ctx.RepoRoot != "" {
		cmd.Dir = ctx.RepoRoot
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	supRes := SupervisedRun(execCtx, cmd, caps)
	err := supRes.Err
	output := buf.String()

	switch supRes.ExitKind {
	case SupervisedExitTimeout:
		preview, ref := StoreBlob(ctx, t.Name()+"-timeout", output)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   banner + fmt.Sprintf("command timed out after %v\n%s", timeout, preview),
			RawRef:    ref,
			Timestamp: time.Now(),
		}, fmt.Errorf("command timed out after %v", timeout)
	case SupervisedExitOOM:
		preview, ref := StoreBlob(ctx, t.Name()+"-oom", output)
		return types.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Summary: banner + fmt.Sprintf("command killed by memory cap (%d MiB) — adjust verify_mem_limit_mb if your repo legitimately needs more RAM\n%s",
				caps.MemoryLimitBytes/(1024*1024), preview),
			RawRef:    ref,
			Timestamp: time.Now(),
		}, fmt.Errorf("command killed by memory cap")
	case SupervisedExitCPULimit:
		preview, ref := StoreBlob(ctx, t.Name()+"-cpu", output)
		return types.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Summary: banner + fmt.Sprintf("command killed by CPU-time cap (%ds)\n%s",
				caps.CPULimitSeconds, preview),
			RawRef:    ref,
			Timestamp: time.Now(),
		}, fmt.Errorf("command killed by CPU-time cap")
	}

	if err != nil {
		preview, ref := StoreBlob(ctx, t.Name()+"-fail", output)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   banner + fmt.Sprintf("command failed: %v\n%s", err, preview),
			RawRef:    ref,
			Timestamp: time.Now(),
		}, nil
	}

	summary, ref := StoreBlob(ctx, t.Name(), banner+output)
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
    "file_type":     {"type": "string",  "description": "Language/file type filter: go, py, js, ts, java, kotlin, arkts, cangjie, rust, ruby, c, cpp, yaml, json, toml, markdown, config, etc. Covers all relevant extensions (e.g. ts matches *.ts and *.tsx; kotlin matches *.kt and *.kts; arkts matches *.ets; cangjie matches *.cj). Preferred over include for language filtering."},
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

	// L1 negative-knowledge gate (R3 second-axis enforcement):
	// the search PATH or the search PATTERN was already shown to be
	// absent in the current repository by an input-side validator.
	// Refuse with the generic per-class reason.
	if ctx != nil {
		if ctx.TypedDenials.IsPathDenied(p.Path) {
			denial := findFirstDenial(&ctx.TypedDenials, p.Path, true)
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   denial.HumanRefusalReason("grep"),
				Timestamp: time.Now(),
			}, nil
		}
		if ctx.TypedDenials.IsSymbolDenied(p.Pattern) {
			denial := findFirstDenial(&ctx.TypedDenials, p.Pattern, false)
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   denial.HumanRefusalReason("grep"),
				Timestamp: time.Now(),
			}, nil
		}

		// L1 multi-repo active-set gate (Phase 1.L1, 2026-05-08): in
		// multi-repo posture, refuse parent-wide ("" / ".") greps that
		// would span all sub-repos, refuse paths inside inactive sub-
		// repos, and reject ambiguous bare paths that resolve into
		// multiple active sub-repos. fileExists=nil because grep
		// targets directories — multi-active-sub-repo bare paths
		// resolve as ambiguous and force the LLM to specify a prefix.
		if gater, ok := ctx.MultiGraph.(types.MultiRepoActiveSetGater); ok && gater != nil {
			gate := gater.ResolveActiveSetPath(ctx, t.Name(), p.Path, nil)
			if !gate.Allowed {
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   gate.RefusalProse,
					Timestamp: time.Now(),
				}, nil
			}
			p.Path = gate.ResolvedPath
		}
	}

	// Resolve relative / "." paths against ctx.RepoRoot so a search
	// rooted at the LLM's notion of "the repo" hits the actual --repo
	// target. The banner below still echoes the LLM-supplied p.Path
	// (relative form) because downstream extractFileCoverage and the
	// path canonicaliser key on repo-relative paths, not absolute.
	searchPath := resolveToolPath(ctx, p.Path)
	if searchPath == "" {
		searchPath = "."
	}
	dirFilter := NewSearchDirFilter(ctx.RepoRoot, searchPath)
	commandDir := ""
	commandSearchPath := searchPath
	if ctx != nil && ctx.RepoRoot != "" {
		if rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, searchPath); ok {
			commandDir = ctx.RepoRoot
			if rel == "" {
				commandSearchPath = "."
			} else {
				commandSearchPath = filepath.FromSlash(rel)
			}
		}
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

	// Native Go fallback — runs when neither ripgrep nor grep is on
	// PATH (distroless / scratch containers / stripped CI images).
	// Output format matches grep -rnH / grep -rl so the same count
	// banner + params banner wrapper works unchanged below.
	if UseNativeGrep() || (!UseRipgrep() && ctx != nil && ctx.RepoRoot != "") {
		var shouldSkip func(path string, d fs.DirEntry) bool
		if ctx != nil && ctx.RepoRoot != "" {
			shouldSkip = func(path string, d fs.DirEntry) bool {
				rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, path)
				return ok && dirFilter.ExcludesRepoRelativePath(rel)
			}
		}
		include := p.Include
		if include == "" && p.FileType != "" {
			// Best-effort: native scan applies the first glob only —
			// keeps parity with single-glob Include semantics.
			if globs := fileTypeToGlobs(p.FileType); len(globs) > 0 {
				include = globs[0]
			}
		}
		nres, nerr := NativeGrep(searchCtx, NativeGrepOpts{
			Pattern:      p.Pattern,
			Root:         searchPath,
			IgnoreCase:   caseInsensitive,
			FilesOnly:    p.FilesOnly,
			ContextLines: contextLines,
			Include:      include,
			ExcludeDirs:  dirFilter.AnyLevelPatterns(),
			ShouldSkip:   shouldSkip,
		})
		paramsBanner := kvBanner("grep params",
			"pattern", p.Pattern,
			"path", p.Path,
			"file_type", p.FileType,
			"include", p.Include,
			"context_lines", func() string {
				if contextLines > 0 {
					return fmt.Sprintf("%d", contextLines)
				}
				return ""
			}(),
			"case_insensitive", func() string {
				if caseInsensitive {
					return "true"
				}
				return ""
			}(),
			"files_only", func() string {
				if p.FilesOnly {
					return "true"
				}
				return ""
			}(),
			"backend", "native",
		)
		if nerr != nil && searchCtx.Err() != context.DeadlineExceeded {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   paramsBanner + fmt.Sprintf("native grep failed: %v", nerr),
				Timestamp: time.Now(),
			}, nil
		}
		if searchCtx.Err() == context.DeadlineExceeded {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   paramsBanner + "grep timed out after 60s — pattern may be too broad or repo too large",
				Timestamp: time.Now(),
			}, nil
		}
		if nres.Matches == 0 {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   true,
				Summary:   paramsBanner + "no matches found",
				Timestamp: time.Now(),
			}, nil
		}
		unit := "matching lines"
		if p.FilesOnly {
			unit = "matching files"
		}
		countBanner := fmt.Sprintf("[grep: %d %s]\n", nres.Matches, unit)
		matchOutput := annotateGrepOutputByPathRole(ctx, p.FilesOnly, nres.Output)
		summary, ref := StoreBlob(ctx, t.Name(), countBanner+paramsBanner+matchOutput)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   true,
			Summary:   summary,
			RawRef:    ref,
			Timestamp: time.Now(),
		}, nil
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
		for _, glob := range dirFilter.RipgrepGlobs() {
			args = append(args, "--glob", glob)
		}
		for _, glob := range ReservedDeviceRipgrepGlobs() {
			args = append(args, "--glob", "!"+glob)
		}
		if p.FileType != "" {
			args = append(args, "--type", p.FileType)
		}
		if p.Include != "" {
			args = append(args, "--glob", p.Include)
		}
		args = append(args, p.Pattern, commandSearchPath)
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
		for _, dir := range dirFilter.AnyLevelPatterns() {
			args = append(args, "--exclude-dir="+dir)
		}
		for _, glob := range ReservedDeviceGrepExcludes() {
			args = append(args, "--exclude="+glob)
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
		args = append(args, commandSearchPath)
		cmd = exec.CommandContext(searchCtx, SearchExecutable(), args...)
	}
	if commandDir != "" {
		cmd.Dir = commandDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()

	// Params banner — added AFTER the count banner (or at the top
	// when there is no count banner) so `strings.HasPrefix(Summary,
	// "[grep:")` and `strings.Contains(Summary, "matching files]")`
	// contracts in contract_check.go and explorer.go survive. Records
	// the resolved flags, not the raw LLM inputs (e.g. smart-case
	// already folded into case_insensitive).
	caseStr := ""
	if caseInsensitive {
		caseStr = "true"
	}
	filesOnlyStr := ""
	if p.FilesOnly {
		filesOnlyStr = "true"
	}
	contextStr := ""
	if contextLines > 0 {
		contextStr = fmt.Sprintf("%d", contextLines)
	}
	paramsBanner := kvBanner("grep params",
		"pattern", p.Pattern,
		"path", p.Path,
		"file_type", p.FileType,
		"include", p.Include,
		"context_lines", contextStr,
		"case_insensitive", caseStr,
		"files_only", filesOnlyStr,
	)

	// Timeout produces a context.DeadlineExceeded — surface it clearly.
	if searchCtx.Err() == context.DeadlineExceeded {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   paramsBanner + "grep timed out after 60s — pattern may be too broad or repo too large",
			Timestamp: time.Now(),
		}, nil
	}

	// grep returns exit code 1 when no matches found — not a real error.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   true,
				Summary:   paramsBanner + "no matches found",
				Timestamp: time.Now(),
			}, nil
		}
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   paramsBanner + fmt.Sprintf("grep failed: %v: %s", err, stderr.String()),
			Timestamp: time.Now(),
		}, nil
	}

	// Count banner first (preserves the `[grep:` prefix contract that
	// contract_check.go and explorer.go key on), params banner
	// second. Without the count banner the LLM cannot tell whether
	// a blob was truncated; without the params banner Turn B loses
	// the call provenance.
	lines := strings.Count(output, "\n")
	if output != "" && !strings.HasSuffix(output, "\n") {
		lines++ // last line has no trailing newline
	}
	var countBanner string
	if lines > 0 {
		unit := "matching lines"
		if p.FilesOnly {
			unit = "matching files"
		}
		countBanner = fmt.Sprintf("[grep: %d %s]\n", lines, unit)
	}
	matchOutput := annotateGrepOutputByPathRole(ctx, p.FilesOnly, output)
	output = countBanner + paramsBanner + matchOutput

	summary, ref := StoreBlob(ctx, t.Name(), output)
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

func annotateGrepOutputByPathRole(ctx *types.BusContext, filesOnly bool, output string) string {
	if ctx == nil || strings.TrimSpace(output) == "" {
		return output
	}
	var production []string
	var auxiliary []string
	var passthrough []string
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if path, ok := grepOutputLinePath(line, filesOnly); ok {
			if types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(path)) {
				auxiliary = append(auxiliary, raw)
			} else {
				production = append(production, raw)
			}
			continue
		}
		passthrough = append(passthrough, raw)
	}
	if len(auxiliary) == 0 {
		return output
	}
	productionHeader, otherHeader, auxiliaryHeader := grepPathRoleSectionHeaders(ctx)
	var b strings.Builder
	if len(production) > 0 {
		b.WriteString(productionHeader)
		b.WriteByte('\n')
		for _, line := range production {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	} else {
		b.WriteString(productionHeader)
		b.WriteByte('\n')
		b.WriteString("no non-auxiliary matches found\n")
	}
	if len(passthrough) > 0 {
		b.WriteString(otherHeader)
		b.WriteByte('\n')
		for _, line := range passthrough {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	b.WriteString(auxiliaryHeader)
	b.WriteByte('\n')
	for _, line := range auxiliary {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func grepPathRoleSectionHeaders(ctx *types.BusContext) (production, other, auxiliary string) {
	if ctx != nil && ctx.PipelineStage == types.StageAnalyze {
		return "[prescan production matches]",
			"[prescan other output]",
			"[prescan auxiliary matches - not production proof]"
	}
	return "[grep production matches]",
		"[grep other output]",
		"[grep auxiliary matches - supporting context, not production proof]"
}

// annotateAnalyzerPrescanGrepOutput is kept as a small compatibility wrapper
// for older tests and call sites. The implementation now partitions grep
// results by typed path role for every stage, not only analyzer prescan.
func annotateAnalyzerPrescanGrepOutput(ctx *types.BusContext, filesOnly bool, output string) string {
	return annotateGrepOutputByPathRole(ctx, filesOnly, output)
}

func grepOutputLinePath(line string, filesOnly bool) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "[") {
		return "", false
	}
	if filesOnly {
		if strings.Contains(line, "/") || strings.Contains(line, `\`) || strings.Contains(line, ".") {
			return line, true
		}
		return "", false
	}
	for i, r := range line {
		if r != ':' && r != '-' {
			continue
		}
		rest := line[i+1:]
		digitEnd := 0
		for digitEnd < len(rest) && rest[digitEnd] >= '0' && rest[digitEnd] <= '9' {
			digitEnd++
		}
		if digitEnd == 0 {
			continue
		}
		if digitEnd >= len(rest) || (rest[digitEnd] != ':' && rest[digitEnd] != '-') {
			continue
		}
		path := strings.TrimSpace(line[:i])
		if path == "" {
			continue
		}
		return path, true
	}
	return "", false
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
	case "kotlin", "kt":
		return []string{"*.kt", "*.kts"}
	case "arkts", "ets":
		// ArkTS also lives in .ts inside HarmonyOS projects but we
		// can't distinguish at grep time without a project probe;
		// returning just .ets is the honest subset. Users who want
		// every ArkTS file in a mixed project should combine with
		// type=ts.
		return []string{"*.ets"}
	case "cangjie", "cj":
		return []string{"*.cj"}
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

// ReadFileSmallLimitThreshold is the upper bound on LLM-provided Limit
// values that read_file treats as a "training-distribution default"
// on inline-sized files. When Offset==0 and 0 < Limit <= threshold on
// a file of bytes <= MaxInlineBytes, read_file ignores the slice and
// returns the whole file with an override banner. Any non-zero Offset
// is a strong signal of deliberate paging and is always honored —
// only the "first read, lazy default limit" shape is suppressed.
//
// Rationale: LLMs routinely emit offset=0+limit=20/50/100 from their
// training distribution, not because they know the file layout. On a
// 66-line source file, limit=20 silently hides the answer past line
// 20. A re-read that deliberately wants a slice picks a non-zero
// offset (see the paging loop around the banner).
//
// Default 100 catches the common lazy values (10/20/50/100) without
// forcing back the whole file when the LLM picks an odd size like
// 150 or 200 (almost always a deliberate keyhole read). Set to 0 to
// disable the override entirely — offset/limit is then honored on
// any file size.
var ReadFileSmallLimitThreshold = 100

// SetReadFileSmallLimitThreshold overrides the package default from
// cmd/root.go at startup. Non-positive values disable the override
// (0 is canonical for "disabled"; negatives are clamped to 0).
func SetReadFileSmallLimitThreshold(n int) {
	if n < 0 {
		n = 0
	}
	ReadFileSmallLimitThreshold = n
}

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
	return "Read file contents. The offset/limit parameters page files too large to inline (>32KB) and can also page inline-sized files — pass any non-zero offset (e.g. offset=100, limit=30) for a deliberate keyhole read and the slice is honored exactly. On files that fit inline, an offset=0 combined with a small limit (default <= 100 lines, configurable via readfile_small_limit_threshold) is treated as a lazy default and expanded to the whole file, because slicing a small file silently hides the answer past the cutoff; the override banner reports this when it fires. The first line of the result is a banner shaped `[path: showing lines X-Y of Z total]` telling you which slice you got; every subsequent line is prefixed with its absolute line number followed by `│`. When citing a file location in your investigation notes, use only numbers taken verbatim from the gutter of a line you actually read — never estimate or interpolate line numbers between two lines you saw."
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

// findFirstDenial returns the first TypedDenial in s whose token
// matches `tok`. pathShaped=true selects path-class denials (exact /
// suffix match, mirroring IsPathDenied); pathShaped=false selects
// symbol-class denials (exact match). Returns a zero-value denial
// when no match — caller's HumanRefusalReason then renders the
// generic "unverified token" fallback.
func findFirstDenial(s *types.TypedDenialSet, tok string, pathShaped bool) types.TypedDenial {
	if s == nil || tok == "" {
		return types.TypedDenial{}
	}
	for _, d := range s.Denials {
		if pathShaped {
			if !s.IsPathDenied(d.Token) {
				continue
			}
			// Substring/suffix-aware membership check via IsPathDenied
			// the other direction (does tok match this denial's token?).
			if d.Token == tok || matchesPath(tok, d.Token) {
				return d
			}
		} else {
			if !s.IsSymbolDenied(d.Token) {
				continue
			}
			if d.Token == tok {
				return d
			}
		}
	}
	return types.TypedDenial{}
}

// matchesPath mirrors TypedDenialSet.IsPathDenied's suffix logic.
func matchesPath(query, denial string) bool {
	if query == denial {
		return true
	}
	// query absolute, denial repo-relative
	if len(query) > len(denial) && query[len(query)-len(denial)-1] == '/' && query[len(query)-len(denial):] == denial {
		return true
	}
	// reverse direction
	if len(denial) > len(query) && denial[len(denial)-len(query)-1] == '/' && denial[len(denial)-len(query):] == query {
		return true
	}
	return false
}

func (t *ReadFile) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p readFileParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}

	// L1 negative-knowledge gate (R3 second-axis enforcement):
	// when an input-side validator has already determined the named
	// path is not present / not corroborated in the current repo,
	// refuse the read with the per-class user-facing reason from
	// HumanRefusalReason — no internal pipeline terminology, no
	// fixture-specific examples, generic enough to apply to any
	// attached log / trace / pasted artifact.
	if ctx != nil && ctx.TypedDenials.IsPathDenied(p.Path) {
		denial := findFirstDenial(&ctx.TypedDenials, p.Path, true)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   denial.HumanRefusalReason("read"),
			Timestamp: time.Now(),
		}, nil
	}

	// L1 multi-repo active-set gate (Phase 1.L1, 2026-05-08): in
	// multi-repo posture refuse paths inside inactive sub-repos and
	// auto-prefix bare paths into the unique-matching active sub-repo.
	// Uses os.Stat as fileExists so the bare-path branch resolves
	// only to a sub-repo where the file actually exists; ambiguous
	// or no-match cases surface as a typed refusal.
	//
	// Reached via the types.MultiRepoActiveSetGater interface
	// (BusContext.MultiGraph any → typed cast) so this package
	// doesn't import the multigraph package directly — that would
	// re-introduce the tool→multigraph→topology→tool cycle.
	if ctx != nil {
		if gater, ok := ctx.MultiGraph.(types.MultiRepoActiveSetGater); ok && gater != nil {
			gate := gater.ResolveActiveSetPath(ctx, t.Name(), p.Path, func(abs string) bool {
				info, err := os.Stat(abs)
				return err == nil && !info.IsDir()
			})
			if !gate.Allowed {
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   gate.RefusalProse,
					Timestamp: time.Now(),
				}, nil
			}
			p.Path = gate.ResolvedPath
		}
	}

	// Resolve relative paths against ctx.RepoRoot. The LLM cites paths
	// in repo-relative form (the canonicaliser also stores them that
	// way); the banner below echoes p.Path verbatim so those downstream
	// keys stay aligned.
	fsPath, reject := resolveReadFilePath(ctx, p.Path)
	if reject != nil {
		return *reject, nil
	}
	data, err := os.ReadFile(fsPath)
	if err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("read failed: %v", err), Timestamp: time.Now()}, nil
	}

	content := string(data)
	allLines := strings.Split(content, "\n")
	totalLines := len(allLines)

	// Slice vs. full-file policy:
	//
	//   - No offset/limit passed → return the whole file.
	//   - Offset > 0 → deliberate paging. Honor the slice verbatim,
	//     regardless of file size. Any non-zero offset is a strong
	//     signal that the LLM knows what section it wants.
	//   - Large file (bytes > MaxInlineBytes) → honor the slice, the
	//     whole file cannot fit inline anyway.
	//   - Small file with Offset==0 and 0 < Limit <=
	//     ReadFileSmallLimitThreshold and Limit < totalLines → treat
	//     as a training-distribution default (limit=20 on a 66-line
	//     file would silently hide the tail) and expand to the whole
	//     file. Banner records the override.
	//   - Small file with any other slice (Limit > threshold, or
	//     Limit == 0, or Limit >= totalLines) → honor the slice.
	//
	// The threshold is configurable via codrax.yaml's
	// readfile_small_limit_threshold; setting it to 0 disables the
	// override entirely.
	sliceStart := 0
	sliceEnd := totalLines
	overrode := false
	if p.Offset > 0 || p.Limit > 0 {
		isLazyDefault := p.Offset == 0 &&
			p.Limit > 0 &&
			ReadFileSmallLimitThreshold > 0 &&
			p.Limit <= ReadFileSmallLimitThreshold &&
			p.Limit < totalLines &&
			len(data) <= MaxInlineBytes
		if isLazyDefault {
			overrode = true
		} else {
			sliceStart = p.Offset
			sliceEnd = totalLines
			if p.Limit > 0 && sliceStart+p.Limit < sliceEnd {
				sliceEnd = sliceStart + p.Limit
			}
		}
	}
	if !overrode && sliceStart >= totalLines {
		lastOffset := totalLines - 1
		if lastOffset < 0 {
			lastOffset = 0
		}
		return types.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Summary: fmt.Sprintf(
				"read_file empty range: offset=%d starts after the last readable line of %s (%d total line(s)); offsets are zero-based, so the last valid offset is %d. Retry with offset<=%d, or omit offset to read from the beginning.",
				p.Offset, p.Path, totalLines, lastOffset, lastOffset),
			Timestamp: time.Now(),
		}, nil
	}

	// Render the selected slice with a per-line gutter carrying the
	// absolute 1-based line number. This exists because LLMs cannot
	// reliably count lines inside a multi-line blob — when asked to
	// cite `file:line` for an extracted fact, they guess monotonically
	// across the read window, producing citations that are off by
	// tens of lines (see memory/project_fake_green_audit_2026_04_14.md
	// Pattern 2). With a gutter, the correct line number is already
	// visible beside every statement and the LLM can copy it verbatim.
	//
	// Gutter format: a right-aligned 6-digit line number, a U+2502
	// separator, and a single space. The separator is unambiguous
	// (not a code-punctuation character) so the LLM cannot confuse
	// the gutter with source content. Per-line overhead is ~10 bytes.
	content = renderWithLineGutter(allLines[sliceStart:sliceEnd], sliceStart+1)

	// Clamp sliceEnd so the banner matches what StoreBlobHeadOnly
	// will actually show. Without this, the banner claims "showing
	// lines X-Y" but blob truncation silently drops lines past the
	// head budget, and the LLM pages from Y+1 — skipping the
	// truncated lines entirely. Clamping runs on the rendered (gutter-
	// prefixed) bytes because that is what actually ships to the LLM.
	if !overrode && len(content) > MaxInlineBytes {
		headBudget := PreviewHeadBytesValue()
		// Find the largest line count N such that rendering
		// allLines[sliceStart:sliceStart+N] fits in headBudget. The
		// rendering is monotone in line count (each extra line only
		// appends bytes), so a linear shrink from the current slice
		// is cheap enough — no binary search needed for typical
		// 24 KB budgets and ~100-byte lines.
		clamped := content
		if len(clamped) > headBudget {
			clamped = clamped[:headBudget]
		}
		// Walk back to the last complete gutter-prefixed line. The
		// resulting byte count may still be under budget; that is
		// fine because we only need a conservative estimate.
		if idx := strings.LastIndex(clamped, "\n"); idx > 0 {
			clamped = clamped[:idx]
		}
		visibleLines := strings.Count(clamped, "\n") + 1
		if visibleLines < sliceEnd-sliceStart {
			sliceEnd = sliceStart + visibleLines
			content = renderWithLineGutter(allLines[sliceStart:sliceEnd], sliceStart+1)
		}
	}

	// Always prepend a slice/total banner so the LLM cannot mistake
	// a partial read for the whole file. The previous design returned
	// the requested slice with no metadata, so a 40-line read of a
	// 330-line file looked identical to a 40-line read of a 40-line
	// file — and the model would routinely conclude after the head
	// that it had seen everything. Banner is plain text, no prescription.
	//
	// The banner format is load-bearing for three downstream parsers
	// in explorer.go (extractFileCoverage, detectTruncatedUngrepped,
	// detectPartiallyReadSymbols) — keep the `[path: showing lines
	// X-Y of Z total]` / `[path: showing all N lines (B bytes); ...]`
	// shapes in sync with the string matchers in those functions.
	var banner string
	if overrode {
		banner = fmt.Sprintf("[%s: showing all %d lines (%d bytes); limit=%d expanded to full file (inline-sized; pass offset>0 for explicit paging)]\n",
			p.Path, totalLines, len(data), p.Limit)
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

// renderWithLineGutter joins a slice of source lines into a single
// string where every line is prefixed by its absolute 1-based line
// number followed by a U+2502 visual separator and a space. The
// first element of `lines` is numbered `startLineNo`.
//
// The gutter exists so the LLM can cite file locations verbatim from
// the numbers it sees on each line, instead of estimating them from
// its position within the blob — an estimation LLMs do badly, with
// drifts of +20 to +40 lines empirically observed on ~400-line reads
// (see memory/project_fake_green_audit_2026_04_14.md Pattern 2).
//
// Width of the number column is 6 so line numbers up to 999999 fit
// without realignment; longer files simply overflow the column, which
// is still unambiguous because the `│` separator marks the boundary.
// The separator is intentionally not a code-punctuation character so
// the LLM cannot confuse gutter with content.
//
// Edge cases:
//   - strings.Split of a file ending in `\n` yields a trailing empty
//     element. We preserve it so the joined output round-trips when
//     a downstream consumer splits on `\n` again, and we still number
//     it (an empty line is still a line in the file).
//   - If `lines` is empty, returns "" unchanged.
func renderWithLineGutter(lines []string, startLineNo int) string {
	if len(lines) == 0 {
		return ""
	}
	// Pre-estimate buffer size: ~10 gutter bytes + average line length.
	avg := 0
	for _, l := range lines {
		avg += len(l)
	}
	avg /= len(lines)
	if avg < 1 {
		avg = 1
	}
	var b strings.Builder
	b.Grow(len(lines) * (avg + 11))
	for i, l := range lines {
		// Gutter: right-aligned 6-digit line number, U+2502, space.
		// Using Fprintf avoids allocating a throwaway intermediate
		// string per line.
		fmt.Fprintf(&b, "%6d│ %s", startLineNo+i, l)
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
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

	// L1 multi-repo active-set gate (Phase 1.L1, 2026-05-08): refuse
	// parent-wide listings that would span all sub-repos, refuse
	// listings inside inactive sub-repos, and refuse ambiguous bare
	// paths. fileExists=nil — list_files targets directories.
	if ctx != nil {
		if gater, ok := ctx.MultiGraph.(types.MultiRepoActiveSetGater); ok && gater != nil {
			gate := gater.ResolveActiveSetPath(ctx, t.Name(), p.Path, nil)
			if !gate.Allowed {
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   gate.RefusalProse,
					Timestamp: time.Now(),
				}, nil
			}
			p.Path = gate.ResolvedPath
		}
	}

	recursiveStr := ""
	if p.Recursive {
		recursiveStr = "true"
	}
	banner := kvBanner("list_files",
		"path", p.Path,
		"recursive", recursiveStr,
	)

	// Resolve "." / relative paths against ctx.RepoRoot. The LLM walks
	// the repo by saying `list_files(path=".")`, which without resolution
	// would scan the codrax process CWD instead of the user's --repo.
	// Output paths keep the LLM's relative form (or "." when no prefix
	// was supplied) so downstream parsers and citation matching stay
	// repo-relative.
	fsPath := resolveToolPath(ctx, p.Path)
	displayPath := p.Path
	if displayPath == "" {
		displayPath = "."
	}
	dirFilter := NewSearchDirFilter(ctx.RepoRoot, fsPath)

	var files []string

	if p.Recursive {
		err := filepath.WalkDir(fsPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// Skip well-known noise directories that explode the walk
			// without yielding useful entries (VCS metadata, dependency
			// caches, hidden tooling). Sourced from the central
			// tool.ExcludeDirsAnyLevelSet so list_files / grep / repomap
			// agree on the same set — earlier this filter was inline
			// and SHORTER than the central set, missing target/dist/
			// build/__pycache__/.codrax which then leaked into LLM
			// view of the repo.
			if d.IsDir() && path != fsPath {
				if ctx != nil && ctx.RepoRoot != "" {
					if rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, path); ok && dirFilter.ExcludesRepoRelativePath(rel) {
						return filepath.SkipDir
					}
				} else if IsExcludedDirName(d.Name()) {
					return filepath.SkipDir
				}
				if IsWindowsReservedDevicePath(d.Name()) {
					return filepath.SkipDir
				}
			}
			if !d.IsDir() && IsWindowsReservedDevicePath(d.Name()) {
				return nil
			}
			rel, relErr := filepath.Rel(fsPath, path)
			if relErr != nil || rel == "." {
				files = append(files, displayPath)
			} else {
				files = append(files, filepath.Join(displayPath, rel))
			}
			return nil
		})
		if err != nil {
			return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + fmt.Sprintf("walk failed: %v", err), Timestamp: time.Now()}, nil
		}
	} else {
		entries, err := os.ReadDir(fsPath)
		if err != nil {
			return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + fmt.Sprintf("readdir failed: %v", err), Timestamp: time.Now()}, nil
		}
		for _, e := range entries {
			// Apply the SAME noise-dir filter to non-recursive listing.
			// Earlier the non-recursive branch had no filter at all, so
			// `list_files path=. recursive=false` returned `.git` /
			// `.codrax` / `node_modules` as legitimate-looking entries
			// — the LLM then either thought the repo was empty (when
			// real source files were drowned by noise) or treated
			// codrax's own state dir as project content. Symmetric
			// filtering with the recursive path is the load-bearing
			// fix; the central set drives both.
			if e.IsDir() {
				if ctx != nil && ctx.RepoRoot != "" {
					entryPath := filepath.Join(fsPath, e.Name())
					if rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, entryPath); ok && dirFilter.ExcludesRepoRelativePath(rel) {
						continue
					}
				} else if IsExcludedDirName(e.Name()) {
					continue
				}
			}
			if IsWindowsReservedDevicePath(e.Name()) {
				continue
			}
			files = append(files, filepath.Join(displayPath, e.Name()))
		}
	}

	summary, ref := StoreBlob(ctx, t.Name(), banner+strings.Join(files, "\n"))
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
// GitDiff
// ---------------------------------------------------------------------------

// GitDiff runs git diff with optional staging and ref options.
type GitDiff struct {
	ReadOnly
	EvidenceTool
}

type gitDiffParams struct {
	Path     string `json:"path,omitempty"`
	Staged   bool   `json:"staged,omitempty"`
	Ref      string `json:"ref,omitempty"`
	Stat     bool   `json:"stat,omitempty"`
	NameOnly bool   `json:"name_only,omitempty"`
}

func (t *GitDiff) Name() string        { return "git_diff" }
func (t *GitDiff) Description() string { return "Run git diff" }

func (t *GitDiff) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":   {"type": "string",  "description": "Repository path (working directory)"},
    "staged": {"type": "boolean", "description": "Show staged changes (--cached)"},
    "ref":    {"type": "string",  "description": "Git ref or range to diff against"},
    "stat":   {"type": "boolean", "description": "Show --stat instead of the full patch"},
    "name_only": {"type": "boolean", "description": "Show --name-only instead of the full patch"}
  }
}`)
}

func (t *GitDiff) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p gitDiffParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}

	stagedStr := ""
	if p.Staged {
		stagedStr = "true"
	}
	banner := kvBanner("git_diff",
		"path", p.Path,
		"ref", p.Ref,
		"staged", stagedStr,
		"stat", fmt.Sprintf("%t", p.Stat),
		"name_only", fmt.Sprintf("%t", p.NameOnly),
	)

	args := []string{"diff"}
	if p.Staged {
		args = append(args, "--cached")
	}
	if p.Stat && p.NameOnly {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   banner + "invalid params: stat and name_only are mutually exclusive",
			Timestamp: time.Now(),
		}, nil
	}
	if p.Stat {
		args = append(args, "--stat")
	}
	if p.NameOnly {
		args = append(args, "--name-only")
	}
	if p.Ref != "" {
		args = append(args, p.Ref)
	}

	cmd, cancel := NewGitLongCommand(nil, args...)
	defer cancel()
	// Anchor at ctx.RepoRoot when the LLM didn't specify a path.
	// Otherwise resolve the LLM-supplied path against RepoRoot so a
	// relative `path: "subpkg"` works. Unlike generic file tools,
	// git tool path overrides are command roots, so reject paths that
	// escape the active repo instead of silently inspecting another
	// checkout.
	dir, pathErr := resolveRepoScopedToolDir(ctx, p.Path)
	if pathErr != "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + pathErr, Timestamp: time.Now()}, nil
	}
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   banner + fmt.Sprintf("git diff failed: %v: %s", err, stderr.String()),
			Timestamp: time.Now(),
		}, nil
	}

	summary, ref := StoreBlob(ctx, t.Name(), banner+stdout.String())
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

// ---------------------------------------------------------------------------
// GitShow
// ---------------------------------------------------------------------------

// GitShow shows a single commit/ref through a structured read-only tool.
type GitShow struct {
	ReadOnly
	EvidenceTool
}

type gitShowParams struct {
	RepoPath string `json:"repo_path,omitempty"`
	Ref      string `json:"ref"`
	Path     string `json:"path,omitempty"`
	Format   string `json:"format,omitempty"`
	NoPatch  bool   `json:"no_patch,omitempty"`
	Stat     bool   `json:"stat,omitempty"`
	NameOnly bool   `json:"name_only,omitempty"`
}

func (t *GitShow) Name() string { return "git_show" }
func (t *GitShow) Description() string {
	return "Show a commit/ref safely. Use this instead of shelling out to git show; no_patch gives metadata only, stat gives --stat, name_only gives changed paths, and the default includes the patch without using unsupported --no-stat."
}

func (t *GitShow) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "repo_path": {"type": "string", "description": "Optional repository working directory, resolved like other path tools"},
    "ref":       {"type": "string", "description": "Commit hash or git ref to show"},
    "path":      {"type": "string", "description": "Optional repo-relative pathspec within the commit"},
    "format":    {"type": "string", "description": "Commit header format: oneline, short, medium, full (default medium)"},
    "no_patch":  {"type": "boolean", "description": "Show commit metadata only (--no-patch)"},
    "stat":      {"type": "boolean", "description": "Show --stat"},
    "name_only": {"type": "boolean", "description": "Show --name-only"}
  },
  "required": ["ref"]
}`)
}

func (t *GitShow) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p gitShowParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}
	ref := strings.TrimSpace(p.Ref)
	if ref == "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "invalid params: ref is required", Timestamp: time.Now()}, nil
	}
	if strings.HasPrefix(ref, "-") {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "invalid params: ref must not start with '-'", Timestamp: time.Now()}, nil
	}
	format := strings.TrimSpace(p.Format)
	if format == "" {
		format = "medium"
	}
	switch format {
	case "oneline", "short", "medium", "full":
	default:
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "invalid params: format must be oneline, short, medium, or full", Timestamp: time.Now()}, nil
	}
	modeCount := 0
	for _, enabled := range []bool{p.NoPatch, p.Stat, p.NameOnly} {
		if enabled {
			modeCount++
		}
	}
	dir, pathErr := resolveRepoScopedToolDir(ctx, p.RepoPath)
	banner := kvBanner("git_show",
		"repo_path", p.RepoPath,
		"ref", ref,
		"path", p.Path,
		"format", format,
		"no_patch", fmt.Sprintf("%t", p.NoPatch),
		"stat", fmt.Sprintf("%t", p.Stat),
		"name_only", fmt.Sprintf("%t", p.NameOnly),
	)
	if pathErr != "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + pathErr, Timestamp: time.Now()}, nil
	}
	if modeCount > 1 {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + "invalid params: no_patch, stat, and name_only are mutually exclusive", Timestamp: time.Now()}, nil
	}
	pathspec := normalizeGitHistoryPathspec(p.Path, dir)
	if path.IsAbs(pathspec) {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + "invalid params: path must be repo-relative or an absolute path under repo_path", Timestamp: time.Now()}, nil
	}

	args := []string{"show", "--no-ext-diff", fmt.Sprintf("--format=%s", format)}
	switch {
	case p.NoPatch:
		args = append(args, "--no-patch")
	case p.Stat:
		args = append(args, "--stat")
	case p.NameOnly:
		args = append(args, "--name-only")
	}
	args = append(args, ref)
	if pathspec != "" {
		args = append(args, "--", pathspec)
	}
	cmd, cancel := NewGitLongCommand(nil, args...)
	defer cancel()
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   banner + fmt.Sprintf("git show failed: %v: %s", err, strings.TrimSpace(stderr.String())),
			Timestamp: time.Now(),
		}, nil
	}
	summary, refBlob := StoreBlob(ctx, t.Name(), banner+stdout.String())
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    refBlob,
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

	banner := kvBanner("git_log",
		"path", p.Path,
		"count", fmt.Sprintf("%d", count),
		"format", format,
	)

	args := []string{"log", fmt.Sprintf("-n%d", count), fmt.Sprintf("--format=%s", format)}

	cmd, cancel := NewGitLongCommand(nil, args...)
	defer cancel()
	// Same RepoRoot anchoring as GitDiff above.
	dir, pathErr := resolveRepoScopedToolDir(ctx, p.Path)
	if pathErr != "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + pathErr, Timestamp: time.Now()}, nil
	}
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   banner + fmt.Sprintf("git log failed: %v: %s", err, stderr.String()),
			Timestamp: time.Now(),
		}, nil
	}

	summary, ref := StoreBlob(ctx, t.Name(), banner+stdout.String())
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

// ---------------------------------------------------------------------------
// GitHistorySearch
// ---------------------------------------------------------------------------

// GitHistorySearch deterministically filters a bounded commit window by
// fixed-string presence in each commit's diff. It exists so history/count
// questions do not need shell loops or model-side list intersection.
type GitHistorySearch struct {
	ReadOnly
	EvidenceTool
}

type gitHistorySearchParams struct {
	RepoPath        string `json:"repo_path,omitempty"`
	WindowPath      string `json:"window_path"`
	WindowCount     int    `json:"window_count,omitempty"`
	Order           string `json:"order,omitempty"`
	DiffPath        string `json:"diff_path,omitempty"`
	Contains        string `json:"contains"`
	CaseInsensitive bool   `json:"case_insensitive,omitempty"`
}

const gitHistorySearchMaxWindowCount = 100

func (t *GitHistorySearch) Name() string { return "git_history_search" }
func (t *GitHistorySearch) Description() string {
	return "Search a bounded git history window and count commits whose diff contains a fixed string. Use order=oldest for first-introduced / earliest-occurrence questions; order=recent is the default for latest / last-N questions."
}

func (t *GitHistorySearch) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "repo_path": {"type": "string", "description": "Optional repository working directory, resolved like other path tools"},
    "window_path": {"type": "string", "description": "Repo-relative pathspec that defines the recent commit window, e.g. internal/orchestrator/"},
    "window_count": {"type": "integer", "description": "For order=recent, number of recent commits to inspect; for order=oldest, maximum matching commits to return while scanning from the earliest commit (default 20, max 100)"},
    "order": {"type": "string", "enum": ["recent", "oldest"], "description": "History window direction. recent (default) starts from HEAD for latest/last-N questions. oldest starts from the earliest commit for first-introduced / earliest-occurrence questions."},
    "diff_path": {"type": "string", "description": "Optional repo-relative pathspec to inspect inside each commit diff; defaults to window_path"},
    "contains": {"type": "string", "description": "Fixed string that must appear in a commit diff for the commit to count"},
    "case_insensitive": {"type": "boolean", "description": "Whether contains matching ignores case (default false)"}
  },
  "required": ["window_path", "contains"]
}`)
}

func (t *GitHistorySearch) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p gitHistorySearchParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: %v", err), Timestamp: time.Now()}, err
	}
	dir, pathErr := resolveRepoScopedToolDir(ctx, p.RepoPath)
	if pathErr != "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: pathErr, Timestamp: time.Now()}, nil
	}
	windowPath := normalizeGitHistoryPathspec(p.WindowPath, dir)
	diffPath := normalizeGitHistoryPathspec(p.DiffPath, dir)
	if diffPath == "" {
		diffPath = windowPath
	}
	needle := strings.TrimSpace(p.Contains)
	if windowPath == "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "invalid params: window_path is required", Timestamp: time.Now()}, nil
	}
	if path.IsAbs(windowPath) || path.IsAbs(diffPath) {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "invalid params: paths must be repo-relative or absolute paths under repo_path", Timestamp: time.Now()}, nil
	}
	if needle == "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "invalid params: contains is required", Timestamp: time.Now()}, nil
	}
	count := p.WindowCount
	if count <= 0 {
		count = 20
	}
	if count > gitHistorySearchMaxWindowCount {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("invalid params: window_count max %d", gitHistorySearchMaxWindowCount), Timestamp: time.Now()}, nil
	}
	order := strings.ToLower(strings.TrimSpace(p.Order))
	if order == "" {
		order = "recent"
	}
	if order != "recent" && order != "oldest" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "invalid params: order must be recent or oldest", Timestamp: time.Now()}, nil
	}

	banner := kvBanner("git_history_search",
		"window_path", windowPath,
		"window_count", fmt.Sprintf("%d", count),
		"order", order,
		"diff_path", diffPath,
		"contains", needle,
	)

	commits, errSummary := gitHistorySearchWindow(dir, count, windowPath, order)
	if errSummary != "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + errSummary, Timestamp: time.Now()}, nil
	}
	matches := make([]gitHistorySearchCommit, 0, len(commits))
	inspected := 0
	for _, commit := range commits {
		inspected++
		patch, errSummary := gitHistorySearchPatch(dir, commit.Hash, diffPath)
		if errSummary != "" {
			return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + errSummary, Timestamp: time.Now()}, nil
		}
		if gitHistoryPatchContains(patch, needle, p.CaseInsensitive) {
			matches = append(matches, commit)
			if order == "oldest" && len(matches) >= count {
				break
			}
		}
	}

	var b strings.Builder
	b.WriteString(banner)
	fmt.Fprintf(&b, "window_size=%d\n", inspected)
	fmt.Fprintf(&b, "answer_count=%d\n", len(matches))
	b.WriteString("matched_commits:\n")
	if len(matches) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, commit := range matches {
			fmt.Fprintf(&b, "- %s %s\n", shortGitHash(commit.Hash), commit.Subject)
		}
	}
	fmt.Fprintf(&b, "unmatched=%d\n", inspected-len(matches))

	summary, ref := StoreBlob(ctx, t.Name(), b.String())
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: summary, RawRef: ref, Timestamp: time.Now()}, nil
}

type gitHistorySearchCommit struct {
	Hash    string
	Subject string
}

func normalizeGitHistoryPathspec(s string, repoRoot string) string {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return ""
	}
	if repoRoot != "" && filepath.IsAbs(raw) {
		if rel, err := filepath.Rel(repoRoot, raw); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			raw = rel
		}
	}
	normalized := strings.TrimSpace(filepath.ToSlash(raw))
	for strings.HasPrefix(normalized, "./") {
		normalized = strings.TrimPrefix(normalized, "./")
	}
	if normalized == "" {
		return ""
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." && normalized != "." {
		return normalized
	}
	return cleaned
}

func gitHistorySearchWindow(dir string, count int, windowPath, order string) ([]gitHistorySearchCommit, string) {
	args := []string{"log", "--format=%H%x09%s"}
	if order == "oldest" {
		args = append(args, "--reverse")
	} else {
		args = append(args, fmt.Sprintf("-n%d", count))
	}
	args = append(args, "--", windowPath)
	cmd, cancel := NewGitLongCommand(nil, args...)
	defer cancel()
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Sprintf("git log window failed: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	lines := strings.Split(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n")
	out := make([]gitHistorySearchCommit, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		commit := gitHistorySearchCommit{Hash: strings.TrimSpace(parts[0])}
		if len(parts) > 1 {
			commit.Subject = strings.TrimSpace(parts[1])
		}
		if commit.Hash != "" {
			out = append(out, commit)
		}
	}
	return out, ""
}

func gitHistorySearchPatch(dir, hash, diffPath string) (string, string) {
	args := []string{"show", "--format=", "--no-ext-diff", "-U0", hash, "--", diffPath}
	cmd, cancel := NewGitLongCommand(nil, args...)
	defer cancel()
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Sprintf("git show %s failed: %v: %s", shortGitHash(hash), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), ""
}

func gitHistoryPatchContains(patch, needle string, caseInsensitive bool) bool {
	if caseInsensitive {
		return strings.Contains(strings.ToLower(patch), strings.ToLower(needle))
	}
	return strings.Contains(patch, needle)
}

func shortGitHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
