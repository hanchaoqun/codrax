package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/safety"
	"github.com/hanchaoqun/codrax/internal/textfmt"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
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

func boolBannerValue(v bool) string {
	if v {
		return "true"
	}
	return ""
}

func positiveIntBannerValue(v int) string {
	if v > 0 {
		return fmt.Sprintf("%d", v)
	}
	return ""
}

func ctxRepoRoot(ctx *types.BusContext) string {
	if ctx == nil {
		return ""
	}
	return ctx.RepoRoot
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
//   - p absolute → return as-is, except during an active write-mode
//     worktree loop where an absolute path under MainRepoRoot is
//     remapped to the same repo-relative path under WorktreePath. This
//     lets replan/verify tools inspect the current mutable checkout
//     even when prior logs or prompt context carried main-checkout
//     absolute paths.
//   - p relative → join under ctx.RepoRoot. If the model prepends the active
//     repo directory name (for example `my-repo/internal/foo.go`), strip that
//     prefix only when the original path is missing and the stripped suffix
//     exists inside ctx.RepoRoot.
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
		if rewritten, ok := rewriteMainRepoAbsolutePathToActiveWorktree(ctx, p); ok {
			return rewritten
		}
		return p
	}
	if rewritten, ok := stripActiveRepoLabelPrefix(ctx, p); ok {
		return filepath.Join(ctx.RepoRoot, rewritten)
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
		if rewritten, ok := rewriteMainRepoAbsolutePathToActiveWorktree(ctx, raw); ok {
			raw = rewritten
		}
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
	if rewritten, ok := stripActiveRepoLabelPrefix(ctx, raw); ok {
		return filepath.Join(root, rewritten), ""
	}
	return filepath.Join(root, raw), ""
}

func rewriteMainRepoAbsolutePathToActiveWorktree(ctx *types.BusContext, p string) (string, bool) {
	if ctx == nil || !ctx.Mode.Normalize().IsWrite() {
		return "", false
	}
	activeRoot := strings.TrimSpace(ctx.RepoRoot)
	mainRoot := strings.TrimSpace(ctx.MainRepoRoot)
	worktreeRoot := strings.TrimSpace(ctx.WorktreePath)
	if activeRoot == "" || mainRoot == "" || worktreeRoot == "" {
		return "", false
	}
	activeRoot = filepath.Clean(normalizeToolAbsolutePath(activeRoot))
	mainRoot = filepath.Clean(normalizeToolAbsolutePath(mainRoot))
	worktreeRoot = filepath.Clean(normalizeToolAbsolutePath(worktreeRoot))
	if !toolPathsEqual(activeRoot, worktreeRoot) || toolPathsEqual(mainRoot, worktreeRoot) {
		return "", false
	}
	abs := filepath.Clean(normalizeToolAbsolutePath(p))
	if !toolPathIsAbs(abs) {
		return "", false
	}
	if workDir := strings.TrimSpace(ctx.WorkDir); workDir != "" {
		workDir = filepath.Clean(normalizeToolAbsolutePath(workDir))
		if _, inWorkDir := repoRelativePathWithinRoot(workDir, abs); inWorkDir {
			return "", false
		}
	}
	if _, alreadyWorktree := repoRelativePathWithinRoot(worktreeRoot, abs); alreadyWorktree {
		return "", false
	}
	rel, underMain := repoRelativePathWithinRoot(mainRoot, abs)
	if !underMain {
		return "", false
	}
	if writeModeMainRuntimeArtifactRel(rel) {
		return "", false
	}
	var rewritten string
	if rel == "" {
		rewritten = worktreeRoot
	} else {
		rewritten = filepath.Join(worktreeRoot, filepath.FromSlash(rel))
	}
	logging.Debug("[tool] remapped write-mode main-repo absolute path to worktree raw=%q rewritten=%q main=%q worktree=%q", p, rewritten, mainRoot, worktreeRoot)
	return rewritten, true
}

func writeModeMainRuntimeArtifactRel(rel string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	return cleaned == ".codrax" || strings.HasPrefix(cleaned, ".codrax/")
}

func stripActiveRepoLabelPrefix(ctx *types.BusContext, raw string) (string, bool) {
	if ctx == nil {
		return "", false
	}
	root := strings.TrimSpace(ctx.RepoRoot)
	if root == "" {
		return "", false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "." {
		return "", false
	}
	if normalized, ok := normalizeWindowsPOSIXPath(raw); ok {
		raw = normalized
	}
	if toolPathIsAbs(raw) {
		return "", false
	}
	cleaned := filepath.ToSlash(filepath.Clean(raw))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	parts := strings.Split(cleaned, "/")
	if len(parts) < 2 {
		return "", false
	}
	repoLabel := filepath.Base(filepath.Clean(root))
	if repoLabel == "" || repoLabel == "." || parts[0] != repoLabel {
		return "", false
	}
	original := filepath.Join(root, cleaned)
	if fileExists(original) {
		return "", false
	}
	stripped := strings.Join(parts[1:], "/")
	if stripped == "" || stripped == "." || stripped == ".." || strings.HasPrefix(stripped, "../") {
		return "", false
	}
	candidate := filepath.Join(root, stripped)
	if _, ok := repoRelativePathWithinRoot(root, candidate); !ok {
		return "", false
	}
	if !fileExists(candidate) {
		return "", false
	}
	logging.Debug("[tool] normalized repo-label path prefix raw=%q rewritten=%q repo_label=%q", raw, stripped, repoLabel)
	return stripped, true
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
	return "Run a read-only shell command with configurable timeout. In read mode it starts in the repository root, so use repo-relative paths and avoid absolute cd/git -C guesses; large output is saved to a blob that can be paged with read_file. For log/trace search/filter pipelines that will support line evidence, preserve original line numbers (for example grep -n or awk printing NR) so read_file can ground the selected range."
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
	if _, decodeFailure, err := decodeStrictToolParams(t.Name(), params, t.Parameters(), &p, nil); err != nil {
		return *decodeFailure, err
	}
	command := p.Command
	if evalGitHistoryDisabled(ctx) && execCommandUsesGitHistoryForEval(command) {
		return evalGitHistoryDisabledToolResult(t.Name(), fmt.Sprintf("$ %s", sanitizeForBanner(command))), nil
	}
	var compatibilityNote string
	if shouldGateExecCommandAsReadOnly(ctx) {
		command, compatibilityNote = normalizeReadOnlyExecCommand(command)
		if err := validateReadOnlyExecCommand(command); err != nil {
			result := readOnlyExecRefusal(ctx, command, err)
			result.Timestamp = time.Now()
			return result, nil
		}
		if result := readModeExecSourceInventoryDebtRepair(ctx, command); result != nil {
			result.Timestamp = time.Now()
			return *result, nil
		}
		if result := readModeExecFileDiscoveryRepair(ctx, command); result != nil {
			result.Timestamp = time.Now()
			return *result, nil
		}
	}
	if shouldGateExecCommandAsWriteReadOnly(ctx) {
		command, compatibilityNote = normalizeReadOnlyExecCommand(command)
		decision := decideWriteModeExecPermission(command)
		if decision.Action != safety.PermissionAllow {
			result := writeModeExecRefusal(ctx, command, fmt.Errorf("%s: %s", decision.ReasonCode, decision.Reason))
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
		payload := output + execCommandFileDiscoveryAdvisory(command)
		preview, ref := StoreBlob(ctx, t.Name()+"-timeout", payload)
		return types.ToolResult{
			ToolName:   t.Name(),
			Success:    false,
			Summary:    banner + fmt.Sprintf("command timed out after %v\n%s", timeout, preview),
			RawRef:     ref,
			Refinement: execCommandRefinement(ctx, command, "exec_command_timeout", len(payload)),
			Timestamp:  time.Now(),
		}, fmt.Errorf("command timed out after %v", timeout)
	case SupervisedExitOOM:
		preview, ref := StoreBlob(ctx, t.Name()+"-oom", output)
		return types.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Summary: banner + fmt.Sprintf("command killed by memory cap (%d MiB) — adjust verify_mem_limit_mb if your repo legitimately needs more RAM\n%s",
				caps.MemoryLimitBytes/(1024*1024), preview),
			RawRef:     ref,
			Refinement: execCommandRefinement(ctx, command, "exec_command_oom", len(output)),
			Timestamp:  time.Now(),
		}, fmt.Errorf("command killed by memory cap")
	case SupervisedExitCPULimit:
		preview, ref := StoreBlob(ctx, t.Name()+"-cpu", output)
		return types.ToolResult{
			ToolName: t.Name(),
			Success:  false,
			Summary: banner + fmt.Sprintf("command killed by CPU-time cap (%ds)\n%s",
				caps.CPULimitSeconds, preview),
			RawRef:     ref,
			Refinement: execCommandRefinement(ctx, command, "exec_command_cpu_limit", len(output)),
			Timestamp:  time.Now(),
		}, fmt.Errorf("command killed by CPU-time cap")
	}

	if err != nil {
		payload := output + execCommandSearchShapeAdvisory(command, output, err) + execCommandFileDiscoveryAdvisory(command)
		preview, ref := StoreBlob(ctx, t.Name()+"-fail", payload)
		return types.ToolResult{
			ToolName:   t.Name(),
			Success:    false,
			Summary:    banner + fmt.Sprintf("command failed: %v\n%s", err, preview),
			RawRef:     ref,
			Refinement: execCommandRefinement(ctx, command, "exec_command_failed", len(payload)),
			Timestamp:  time.Now(),
		}, nil
	}

	payload := execCommandPayloadWithTypedOrigins(banner, command, output)
	if advisory := execCommandSearchShapeAdvisory(command, output, nil); advisory != "" {
		payload += advisory
	}
	if advisory := execCommandFileDiscoveryAdvisory(command); advisory != "" {
		payload += advisory
	}
	summary, ref := StoreBlob(ctx, t.Name(), payload)
	return types.ToolResult{
		ToolName:   t.Name(),
		Success:    true,
		Summary:    summary,
		RawRef:     ref,
		Refinement: execCommandRefinement(ctx, command, "", len(payload)),
		Timestamp:  time.Now(),
	}, nil
}

func execCommandRefinement(ctx *types.BusContext, command, outcomeReason string, payloadBytes int) *types.ToolRefinementHint {
	broadFind := readModeExecFileDiscoveryShouldRepair(command)
	broadContent := execCommandLooksLikeBroadRepoContentEnumeration(command)
	largeOutput := payloadBytes > MaxInlineBytes
	if strings.TrimSpace(outcomeReason) == "" && !largeOutput && !broadFind && !broadContent {
		return nil
	}
	reasonCode := strings.TrimSpace(outcomeReason)
	if reasonCode == "" {
		switch {
		case broadFind:
			reasonCode = "exec_command_broad_file_discovery"
		case broadContent:
			reasonCode = "exec_command_broad_content_enumeration"
		default:
			reasonCode = "exec_command_large_output"
		}
	}
	hint := types.ToolRefinementHint{
		ReasonCode:      reasonCode,
		ResultTruncated: largeOutput || execCommandOutcomeTruncated(outcomeReason),
	}
	switch {
	case broadFind && execCommandActiveSourceInventoryProfile(ctx):
		hint.PreferredNextTool = "repo_map"
		hint.PreferredParams = map[string]string{
			"view":               "source_inventory",
			"include_counts":     "true",
			"include_attributes": "false",
		}
		hint.RequiredFields = []string{"scope", "roles"}
	case broadFind:
		hint.PreferredNextTool = "list_files"
		hint.PreferredParams = map[string]string{
			"path":      ".",
			"recursive": "true",
		}
		hint.RequiredFields = []string{"include", "file_type"}
	case broadContent || execCommandLooksLikeGrepPipeline(command):
		hint.PreferredNextTool = "grep"
		hint.PreferredParams = map[string]string{
			"context_lines": "3",
			"files_only":    "false",
		}
		if execCommandTargetsRuntimeTextArtifact(command) {
			hint.PreferredParams["context_lines"] = "0"
		}
		hint.RequiredFields = []string{"path", "pattern"}
	default:
		hint.PreferredNextTool = "exec_command"
		hint.RequiredFields = []string{"command"}
	}
	out := types.NormalizeToolRefinementHint(hint)
	if out.Empty() {
		return nil
	}
	return &out
}

func execCommandOutcomeTruncated(outcomeReason string) bool {
	switch strings.TrimSpace(outcomeReason) {
	case "exec_command_timeout", "exec_command_oom", "exec_command_cpu_limit":
		return true
	default:
		return false
	}
}

func execCommandFileDiscoveryAdvisory(command string) string {
	if !execCommandLooksLikeFindDiscovery(command) {
		return ""
	}
	return "[exec_command advisory: broad file discovery through `find` can be slow or over-broad inside repository worktrees. Prefer `list_files` with `recursive=true` and typed `include`/`file_type` filters for repo-local path discovery; use `include_auxiliary=true` only when the typed answer scope includes all/auxiliary repo-owned corpus material or a production-only scan was empty.]\n"
}

func readModeExecFileDiscoveryRepair(ctx *types.BusContext, command string) *types.ToolResult {
	if !readModeExecFileDiscoveryShouldRepair(command) {
		return nil
	}
	hint := "Use list_files with recursive=true plus typed include/file_type filters for repo-local path discovery."
	metadata := map[string]string{
		"reason_code":    "broad_file_discovery",
		"preferred_tool": "list_files",
	}
	if execCommandActiveSourceInventoryProfile(ctx) {
		hint = "Use repo_map with view=source_inventory for inventory membership, or list_files with recursive=true plus typed include/file_type filters for path discovery."
		metadata["source_inventory"] = "true"
		metadata["alternative_tool"] = "repo_map"
		metadata["alternative_view"] = "source_inventory"
	}
	return &types.ToolResult{
		ToolName: "exec_command",
		Success:  false,
		Summary: fmt.Sprintf(
			"exec_command refused: broad repo-local file discovery through `find` is not executed in read mode because typed bounded tools can produce the same path set with clearer scope and lower runtime risk. %s The rejected command was: %s",
			hint, sanitizeForBanner(command)),
		Repair: &types.ToolRepair{
			Code:     "exec_command_file_discovery_use_typed_tools",
			Hint:     hint,
			Fields:   []string{"path", "recursive", "include", "file_type", "include_auxiliary"},
			Metadata: metadata,
		},
	}
}

func readModeExecSourceInventoryDebtRepair(ctx *types.BusContext, command string) *types.ToolResult {
	if !execCommandActiveSourceInventoryDebt(ctx) {
		return nil
	}
	reasonCode := ""
	switch {
	case execCommandLooksLikeFileSetMeasurement(command):
		reasonCode = "source_inventory_debt_command_measurement"
	case execCommandLooksLikeBroadRepoContentEnumeration(command):
		reasonCode = "source_inventory_debt_broad_content_enumeration"
	default:
		return nil
	}
	hint := "Use repo_map with view=source_inventory and the scheduler/requested typed scope to close source-inventory debt; file-set shell counts do not prove membership coverage."
	return &types.ToolResult{
		ToolName: "exec_command",
		Success:  false,
		Summary: fmt.Sprintf(
			"exec_command refused: active source-inventory completion debt requires typed inventory coverage, not broad shell file-set measurement. %s The rejected command was: %s",
			hint, sanitizeForBanner(command)),
		Repair: &types.ToolRepair{
			Code:   "exec_command_source_inventory_debt_use_typed_inventory",
			Hint:   hint,
			Fields: []string{"view", "scope", "scopes", "roles", "include_counts"},
			Metadata: map[string]string{
				"reason_code":    reasonCode,
				"preferred_tool": "repo_map",
				"preferred_view": "source_inventory",
			},
		},
	}
}

func readModeExecFileDiscoveryShouldRepair(command string) bool {
	if !execCommandLooksLikeFindDiscovery(command) {
		return false
	}
	if execCommandLooksLikeCountOnly(command) {
		return false
	}
	return true
}

func execCommandActiveSourceInventoryProfile(ctx *types.BusContext) bool {
	return ctx != nil &&
		ctx.AnalysisIR != nil &&
		ctx.AnalysisIR.RequestModel.SourceInventoryProfile != nil &&
		ctx.AnalysisIR.RequestModel.SourceInventoryProfile.Active()
}

func execCommandActiveSourceInventoryDebt(ctx *types.BusContext) bool {
	if !execCommandActiveSourceInventoryProfile(ctx) {
		return false
	}
	if types.NormalizeSourceInventoryFollowupDebt(ctx.SourceInventoryFollowupDebt).IsActive() {
		return true
	}
	if ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return false
	}
	observation := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	if !observation.IsActive() {
		return false
	}
	authority := types.BuildSourceInventoryCompletionAuthority(observation, ctx.AnalysisIR.RequestModel, false)
	return authority.IsBlocking()
}

func execCommandLooksLikeFindDiscovery(command string) bool {
	for _, segment := range shellCommandSegments(command) {
		for _, token := range shellWordsForOrigin(segment) {
			if path.Base(strings.ToLower(token)) == "find" {
				return true
			}
		}
	}
	return false
}

func execCommandLooksLikeFileSetMeasurement(command string) bool {
	if !execCommandLooksLikeCountOnly(command) {
		return false
	}
	for _, segment := range shellCommandSegments(command) {
		tokens := shellWordsForOrigin(segment)
		if len(tokens) == 0 {
			continue
		}
		cmd := path.Base(strings.ToLower(tokens[0]))
		switch cmd {
		case "find", "ls":
			return true
		case "git":
			if firstGitSubcommand(tokens[1:]) == "ls-files" {
				return true
			}
		case "rg":
			for _, token := range tokens[1:] {
				if token == "--files" {
					return true
				}
			}
		}
	}
	return false
}

func execCommandLooksLikeBroadRepoContentEnumeration(command string) bool {
	if execCommandLooksLikeCountOnly(command) {
		return false
	}
	for _, segment := range shellCommandSegments(command) {
		tokens := shellWordsForOrigin(segment)
		if len(tokens) == 0 {
			continue
		}
		if execCommandSegmentLooksLikeBroadGrepEnumeration(tokens) {
			return true
		}
	}
	return false
}

func execCommandSegmentLooksLikeBroadGrepEnumeration(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	cmd := path.Base(strings.ToLower(tokens[0]))
	switch cmd {
	case "grep", "egrep", "fgrep":
		return execCommandGrepTokensTargetRepoRoot(tokens[1:])
	case "rg":
		return execCommandRGTokensTargetRepoRoot(tokens[1:])
	default:
		return false
	}
}

func execCommandGrepTokensTargetRepoRoot(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	sawRecursive := false
	nonFlag := 0
	for i := 0; i < len(tokens); i++ {
		token := strings.TrimSpace(tokens[i])
		if token == "" {
			continue
		}
		switch {
		case token == "--":
			for _, rest := range tokens[i+1:] {
				if execCommandTokenIsRepoRoot(rest) {
					return true
				}
			}
			return nonFlag <= 1
		case token == "-R" || token == "-r" || strings.Contains(token, "R") && strings.HasPrefix(token, "-") && !strings.HasPrefix(token, "--"):
			sawRecursive = true
			continue
		case strings.HasPrefix(token, "--include=") || strings.HasPrefix(token, "--exclude=") ||
			strings.HasPrefix(token, "--exclude-dir=") || strings.HasPrefix(token, "--include-dir="):
			continue
		case token == "--include" || token == "--exclude" || token == "--exclude-dir" || token == "--include-dir" ||
			token == "-e" || token == "-f" || token == "-m" || token == "-A" || token == "-B" || token == "-C":
			i++
			continue
		case strings.HasPrefix(token, "-"):
			continue
		}
		nonFlag++
		if execCommandTokenIsRepoRoot(token) {
			return true
		}
	}
	return sawRecursive && nonFlag <= 1
}

func execCommandRGTokensTargetRepoRoot(tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	nonFlag := 0
	for i := 0; i < len(tokens); i++ {
		token := strings.TrimSpace(tokens[i])
		if token == "" {
			continue
		}
		switch {
		case token == "--":
			for _, rest := range tokens[i+1:] {
				if execCommandTokenIsRepoRoot(rest) {
					return true
				}
			}
			return nonFlag <= 1
		case token == "-e" || token == "-f" || token == "-g" || token == "--glob" ||
			token == "--type" || token == "-t" || token == "--type-not" || token == "-T" ||
			token == "-m" || token == "-A" || token == "-B" || token == "-C":
			i++
			continue
		case strings.HasPrefix(token, "-"):
			continue
		}
		nonFlag++
		if execCommandTokenIsRepoRoot(token) {
			return true
		}
	}
	return nonFlag <= 1
}

func execCommandTokenIsRepoRoot(token string) bool {
	token = strings.Trim(strings.TrimSpace(strings.ReplaceAll(token, `\`, `/`)), `"'`)
	return token == "" || token == "." || token == "./"
}

func execCommandPayloadWithTypedOrigins(banner, command, output string) string {
	payload := banner + output
	originLine := execCommandTypedOriginLine(command, output)
	if originLine == "" {
		return payload
	}
	return banner + originLine + output
}

func execCommandSearchShapeAdvisory(command, output string, err error) string {
	looksLikeGrepPipeline := execCommandLooksLikeGrepPipeline(command)
	if !looksLikeGrepPipeline && !execCommandLooksLikeRuntimeSearchFilter(command) {
		return ""
	}
	if err != nil {
		if looksLikeGrepPipeline && strings.Contains(err.Error(), "exit status 1") {
			if execCommandTargetsRuntimeTextArtifact(command) {
				return "[exec_command advisory: grep exited 1, which usually means zero matches rather than a broken command. Split combined log/trace patterns into one literal/timestamp/thread/event first, preserve the observed field order, and rerun with original line numbers such as `grep -n` before using read_file for evidence.]\n"
			}
			return "[exec_command advisory: grep exited 1, which usually means zero matches rather than a broken command. Simplify or split the pattern if the absence is surprising; preserve line numbers when the result will support line evidence.]\n"
		}
		return ""
	}
	if strings.TrimSpace(output) == "" || execCommandLooksLikeCountOnly(command) || execCommandOutputLooksScalarOnly(output) || !execCommandTargetsRuntimeTextArtifact(command) {
		return ""
	}
	var advisories []string
	if !execCommandOutputHasLineNumbers(output) {
		advisories = append(advisories, "[exec_command advisory: this log/trace search/filter output has no original line numbers. For line-scope evidence, rerun the search with line numbers (for example `grep -n ...` or `awk '... { printf \"%d:%s\\n\", NR, $0 }' ...`) and then use read_file around the selected range.]")
	}
	if execCommandRuntimeSearchUsesBroadAlternation(command) {
		advisories = append(advisories, "[exec_command advisory: this runtime/log/trace search uses a broad OR/alternation pattern. For time-window or chain tracing, OR searches can return early unrelated rows; prefer conjunctive filtering or numeric filtering that preserves original line numbers, then read_file the selected line window.]")
	}
	if len(advisories) == 0 {
		return ""
	}
	return strings.Join(advisories, "\n") + "\n"
}

func execCommandLooksLikeRuntimeSearchFilter(command string) bool {
	for _, segment := range shellCommandSegments(command) {
		for _, token := range shellWordsForOrigin(segment) {
			base := path.Base(strings.ToLower(token))
			switch base {
			case "grep", "egrep", "fgrep", "rg",
				"awk", "gawk", "mawk", "nawk",
				"sed", "perl", "python", "python3", "ruby":
				return true
			}
		}
	}
	return false
}

func execCommandLooksLikeGrepPipeline(command string) bool {
	for _, segment := range shellCommandSegments(command) {
		for _, token := range shellWordsForOrigin(segment) {
			base := path.Base(strings.ToLower(token))
			if base == "grep" || base == "egrep" || base == "fgrep" || base == "rg" {
				return true
			}
		}
	}
	return false
}

func execCommandTargetsRuntimeTextArtifact(command string) bool {
	if ok, hadReadableCandidate := execCommandTargetsRuntimeTextArtifactByContent(command); ok || hadReadableCandidate {
		return ok
	}
	lower := strings.ToLower(command)
	for _, suffix := range []string{".systrace", ".htrace", ".atrace", ".perfetto", ".perftrace", ".tracebundle.json", ".trace", ".log"} {
		if strings.Contains(lower, suffix) {
			return true
		}
	}
	return false
}

func execCommandTargetsRuntimeTextArtifactByContent(command string) (bool, bool) {
	hadReadableCandidate := false
	for _, segment := range shellCommandSegments(command) {
		for _, token := range shellWordsForOrigin(segment) {
			for _, candidate := range execCommandRuntimeArtifactPathCandidates(token) {
				if candidate == "" {
					continue
				}
				if execCommandPathContentLooksRuntimeArtifact(candidate) {
					return true, true
				}
				if execCommandPathIsReadableRegularFile(candidate) {
					hadReadableCandidate = true
				}
			}
		}
	}
	return false, hadReadableCandidate
}

func execCommandRuntimeArtifactPathCandidates(token string) []string {
	token = strings.TrimSpace(token)
	if token == "" || token == "-" || strings.HasPrefix(token, "-") {
		return nil
	}
	if strings.ContainsAny(token, "*?[") || strings.Contains(token, "=") {
		return nil
	}
	token = strings.Trim(token, " \t\r\n\"'`,;")
	if token == "" || token == "." || token == ".." {
		return nil
	}
	return []string{token}
}

func execCommandPathContentLooksRuntimeArtifact(rawPath string) bool {
	path := execCommandReadablePath(rawPath)
	if path == "" {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 64*1024)
	n, _ := f.Read(buf)
	if n <= 0 {
		return false
	}
	raw := string(buf[:n])
	lower := strings.ToLower(raw)
	return strings.HasPrefix(raw, "PERFILE2") ||
		strings.HasPrefix(raw, "SIMPLEPERF") ||
		strings.Contains(raw, "perf_sample:") ||
		strings.Contains(raw, "sched_switch") ||
		strings.Contains(raw, "sched_wakeup") ||
		strings.Contains(raw, "tracing_mark_write:") ||
		strings.Contains(raw, "binder_transaction:") ||
		strings.Contains(raw, "block_rq_issue:") ||
		strings.Contains(raw, "block_rq_complete:") ||
		strings.Contains(raw, "android_fs_") ||
		strings.Contains(raw, "f2fs_") ||
		strings.Contains(raw, "mm_filemap_") ||
		strings.Contains(raw, "scsi_") ||
		strings.Contains(raw, "# tracer:") ||
		(strings.Contains(lower, `"artifacts"`) && strings.Contains(lower, `"tracebundle"`))
}

func execCommandPathIsReadableRegularFile(rawPath string) bool {
	return execCommandReadablePath(rawPath) != ""
}

func execCommandReadablePath(rawPath string) string {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if !filepath.IsAbs(path) {
		if cwd, err := os.Getwd(); err == nil && cwd != "" {
			path = filepath.Join(cwd, path)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return ""
	}
	return path
}

func execCommandRuntimeSearchUsesBroadAlternation(command string) bool {
	if strings.Contains(command, `\|`) {
		return true
	}
	for _, quoted := range shellQuotedSegments(command) {
		if strings.Contains(quoted, "|") {
			return true
		}
	}
	return false
}

func shellQuotedSegments(command string) []string {
	var out []string
	for i := 0; i < len(command); i++ {
		quote := command[i]
		if quote != '\'' && quote != '"' {
			continue
		}
		start := i + 1
		escaped := false
		for j := start; j < len(command); j++ {
			c := command[j]
			if quote == '"' && c == '\\' && !escaped {
				escaped = true
				continue
			}
			if c == quote && !escaped {
				out = append(out, command[start:j])
				i = j
				break
			}
			escaped = false
		}
	}
	return out
}

func execCommandLooksLikeCountOnly(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, "wc -l") ||
		strings.Contains(lower, "grep -c") ||
		strings.Contains(lower, "rg -c") ||
		strings.Contains(lower, "--count")
}

func execCommandOutputLooksScalarOnly(output string) bool {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	nonEmpty := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		nonEmpty++
		if nonEmpty > 3 || !execCommandOutputLineLooksScalar(line) {
			return false
		}
	}
	return nonEmpty > 0
}

func execCommandOutputLineLooksScalar(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	for _, r := range line {
		switch {
		case r >= '0' && r <= '9':
			continue
		case r == '.' || r == '-' || r == '+' || r == ',' || r == '_' || r == ':' || r == '%' || r == ' ' || r == '\t':
			continue
		default:
			return false
		}
	}
	return true
}

func execCommandOutputHasLineNumbers(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		if strings.HasPrefix(line, "--") {
			continue
		}
		if execCommandOutputLineStartsWithNumber(line) {
			return true
		}
		if _, n, _, ok := parseGrepOutputLineLocation(line); ok && n > 0 {
			return true
		}
	}
	return false
}

func execCommandOutputLineStartsWithNumber(line string) bool {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i < len(line) && line[i] == ':'
}

func execCommandTypedOriginLine(command, output string) string {
	metadata, diff := execCommandGitOrigins(command)
	proofSummary := fmt.Sprintf("[exec_command: $ %s]\n%s", sanitizeForBanner(command), output)
	_, measurement := types.DeterministicCountProofInteger(proofSummary)
	if !measurement {
		_, measurement = deterministicHistoryCountProofInteger(proofSummary)
	}
	if !metadata && !diff && !measurement {
		return ""
	}
	var kv []string
	switch {
	case metadata:
		kv = append(kv, "evidence_origin", string(types.AnswerEvidenceOriginVCSMetadata))
	case diff:
		kv = append(kv, "evidence_origin", string(types.AnswerEvidenceOriginVCSDiff))
	case measurement:
		kv = append(kv, "evidence_origin", string(types.AnswerEvidenceOriginCommandMeasurement))
	}
	if diff && metadata {
		kv = append(kv, "diff_origin", string(types.AnswerEvidenceOriginVCSDiff))
	}
	if measurement && (metadata || diff) {
		kv = append(kv, "measurement_origin", string(types.AnswerEvidenceOriginCommandMeasurement))
	}
	if measurement {
		kv = append(kv, "measurement", "count")
	}
	return kvBanner("exec_command", kv...)
}

func execCommandGitOrigins(command string) (metadata bool, diff bool) {
	for _, segment := range shellCommandSegments(command) {
		tokens := shellWordsForOrigin(segment)
		if len(tokens) == 0 {
			continue
		}
		gitIdx := effectiveGitTokenIndex(tokens)
		if gitIdx < 0 {
			continue
		}
		subIdx, subcmd := gitSubcommand(tokens[gitIdx+1:])
		if subcmd == "" {
			continue
		}
		subTokens := tokens[gitIdx+1+subIdx:]
		switch subcmd {
		case "diff", "diff-tree", "diff-index", "diff-files", "format-patch":
			diff = true
		case "show":
			metadata = true
			if !gitShowMetadataOnly(subTokens) {
				diff = true
			}
		case "log":
			metadata = true
			if gitLogIncludesDiff(subTokens) {
				diff = true
			}
		case "rev-list", "rev-parse", "branch", "tag", "describe", "merge-base", "status", "ls-files", "blame", "shortlog", "for-each-ref":
			metadata = true
		}
	}
	return metadata, diff
}

func execCommandHasGitHistoryCommand(command string) bool {
	for _, segment := range shellCommandSegments(command) {
		tokens := shellWordsForOrigin(segment)
		if len(tokens) == 0 {
			continue
		}
		gitIdx := effectiveGitTokenIndex(tokens)
		if gitIdx < 0 {
			continue
		}
		_, subcmd := gitSubcommand(tokens[gitIdx+1:])
		if subcmd == "log" || subcmd == "rev-list" {
			return true
		}
	}
	return false
}

func execCommandUsesGitHistoryForEval(command string) bool {
	for _, segment := range shellCommandSegments(command) {
		tokens := shellWordsForOrigin(segment)
		if len(tokens) == 0 {
			continue
		}
		gitIdx := effectiveGitTokenIndex(tokens)
		if gitIdx < 0 {
			continue
		}
		_, subcmd := gitSubcommand(tokens[gitIdx+1:])
		if gitHistorySubcommandForEval(subcmd) {
			return true
		}
	}
	return false
}

func gitHistorySubcommandForEval(subcmd string) bool {
	switch subcmd {
	case "show", "log", "rev-list", "rev-parse", "branch", "tag", "describe",
		"merge-base", "shortlog", "for-each-ref", "blame",
		"diff", "diff-tree", "diff-index", "format-patch":
		return true
	default:
		return false
	}
}

func evalGitHistoryDisabled(ctx *types.BusContext) bool {
	return ctx != nil && ctx.EvalDisableGitHistory
}

func evalGitHistoryDisabledToolResult(toolName, operation string) types.ToolResult {
	operation = strings.TrimSpace(operation)
	prefix := toolName
	if operation != "" {
		prefix = fmt.Sprintf("%s %s", toolName, operation)
	}
	return types.ToolResult{
		ToolName:  toolName,
		Success:   false,
		Summary:   fmt.Sprintf("%s refused: git history is disabled by --eval-disable-git-history for fair evaluation; use current checkout files or working-tree-only diff evidence instead.", prefix),
		Timestamp: time.Now(),
	}
}

func shellCommandSegments(command string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	escaped := false
	skipNextAmp := false
	flush := func() {
		if s := strings.TrimSpace(b.String()); s != "" {
			out = append(out, s)
		}
		b.Reset()
	}
	for i, r := range command {
		if skipNextAmp {
			skipNextAmp = false
			continue
		}
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			b.WriteRune(r)
			escaped = true
			continue
		}
		if quote != 0 {
			b.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			b.WriteRune(r)
		case '|', ';', '\n':
			flush()
		case '&':
			if i+1 < len(command) && command[i+1] == '&' {
				flush()
				skipNextAmp = true
			} else {
				b.WriteRune(r)
			}
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

func shellWordsForOrigin(segment string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range segment {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t', '\r', '\n':
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

func effectiveGitTokenIndex(tokens []string) int {
	i := 0
	for i < len(tokens) && strings.Contains(tokens[i], "=") && !strings.HasPrefix(tokens[i], "-") {
		i++
	}
	for i < len(tokens) {
		switch tokens[i] {
		case "env", "command":
			i++
			continue
		}
		break
	}
	if i < len(tokens) && tokens[i] == "git" {
		return i
	}
	return -1
}

func gitSubcommand(tokens []string) (int, string) {
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == "" {
			continue
		}
		if strings.HasPrefix(tok, "--git-dir=") ||
			strings.HasPrefix(tok, "--work-tree=") ||
			strings.HasPrefix(tok, "--namespace=") ||
			strings.HasPrefix(tok, "--exec-path=") {
			continue
		}
		switch tok {
		case "-C", "-c", "--git-dir", "--work-tree", "--namespace", "--exec-path", "--config-env":
			i++
			continue
		case "--no-pager", "--literal-pathspecs", "--glob-pathspecs", "--noglob-pathspecs", "--icase-pathspecs":
			continue
		}
		if strings.HasPrefix(tok, "-") {
			continue
		}
		return i, tok
	}
	return -1, ""
}

func gitShowMetadataOnly(tokens []string) bool {
	for _, tok := range tokens[1:] {
		switch tok {
		case "--no-patch", "--quiet", "-s":
			return true
		}
	}
	return false
}

func gitLogIncludesDiff(tokens []string) bool {
	for _, tok := range tokens[1:] {
		switch tok {
		case "-p", "-u", "--patch", "--stat", "--numstat", "--shortstat", "--summary", "--name-only", "--name-status", "--raw":
			return true
		}
		if strings.HasPrefix(tok, "--stat=") || strings.HasPrefix(tok, "--diff-filter=") {
			return true
		}
	}
	return false
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
	FixedString  bool   `json:"fixed_string,omitempty"`
	LineStart    int    `json:"line_start,omitempty"`
	LineEnd      int    `json:"line_end,omitempty"`
}

func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return "Search file contents by pattern. Use this to find where a symbol, string, config key, log event, or trace row appears. By default `pattern` is a regex. Case handling is smart by default: if the pattern contains no uppercase characters it is matched case-insensitively (so `subagent` finds `SubAgent`, `SUBAGENT`, etc.); if the pattern contains any uppercase character it is matched exactly. Pass ignore_case explicitly to override. Use fixed_string=true for exact literal text with punctuation (recommended for log/trace event labels, thread names, error strings, config keys, and any text containing []()|#.: that you do not want treated as regex). Use file_type to filter by language (e.g. \"go\", \"py\", \"js\", \"ts\", \"java\", \"yaml\") — this is preferred over include because it covers all relevant extensions (e.g. type \"ts\" matches both *.ts and *.tsx). Do not set file_type/include when path is already one concrete log/trace artifact. Use context_lines to include surrounding lines around each match, which saves a follow-up read_file call; avoid context_lines on the first broad search of a large log/trace and instead narrow to a timestamp/thread/event, then read_file around returned line numbers. Use line_start/line_end only with a single file when you already know a relevant line vicinity. Directory scans may skip very large runtime artifacts for safety; if the result lists skipped_large_candidates, set path to that candidate and grep the file explicitly rather than reading from the file head. For large log/trace/systrace files, search one exact timestamp, thread id/name, or event literal first; regex is order-sensitive, so if a combined pattern returns no matches, inspect the line format with a simpler literal before combining fields. For trace marker spans, a B|pid|name begin is normally closed by unnamed E|pid or bare E on the same ftrace thread stack, so do not grep for E|pid|name; use trace_query(view=\"span_window\", span_name=\"...\") to resolve the end. Do NOT use the result to count matches by eye — pipe `grep -c` or `grep ... | wc -l` through exec_command instead, and treat that number as authoritative."
}

func grepShouldUseNativeBackend(searchBackend string, hasRepoRoot, searchPathIsFile bool) bool {
	switch searchBackend {
	case "native":
		return true
	case "grep":
		// Keep the Go walker for broad repo-root/directory searches when
		// ripgrep is unavailable because it preserves SearchDirFilter's
		// root-only exclusion semantics. Explicit single-file searches
		// should use the detected system grep: large runtime artifacts
		// (systrace/log/trace/perfetto) are often exactly what the user
		// asked to inspect, and shell grep streams them efficiently.
		return hasRepoRoot && !searchPathIsFile
	default:
		return false
	}
}

func grepPatternHasCommonRegexShorthand(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '\\' {
			continue
		}
		runStart := i
		for i < len(pattern) && pattern[i] == '\\' {
			i++
		}
		if (i-runStart)%2 == 0 || i >= len(pattern) {
			i--
			continue
		}
		switch pattern[i] {
		case 'd', 'D', 's', 'S', 'w', 'W':
			return true
		}
	}
	return false
}

func grepLineUnit(contextLines int) string {
	if contextLines > 0 {
		return "matching lines (including context output lines)"
	}
	return "matching lines"
}

func grepCountBanner(entries int, filesOnly bool, contextLines int) string {
	unit := "matching lines"
	if filesOnly {
		unit = "matching files"
	} else {
		unit = grepLineUnit(contextLines)
	}
	return fmt.Sprintf("[grep: %d %s]\n", entries, unit)
}

func configuredSearchExcludeRootsRefinement(ctx *types.BusContext, targetPath, preferredTool string) *types.ToolRefinementHint {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return nil
	}
	roots := SearchRuntimeArtifactRoots()
	if len(roots) == 0 {
		return nil
	}
	rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, targetPath)
	if !ok || rel != "" {
		return nil
	}
	hint := types.NormalizeToolRefinementHint(types.ToolRefinementHint{
		UniverseExcludedReason: "configured_search_exclude_roots",
		ExcludedRoots:          roots,
		PreferredNextTool:      preferredTool,
		RequiredFields:         []string{"path"},
	})
	if hint.Empty() {
		return nil
	}
	return &hint
}

func mergeToolRefinementHints(a, b *types.ToolRefinementHint) *types.ToolRefinementHint {
	switch {
	case a == nil && b == nil:
		return nil
	case a == nil:
		out := types.NormalizeToolRefinementHint(*b)
		if out.Empty() {
			return nil
		}
		return &out
	case b == nil:
		out := types.NormalizeToolRefinementHint(*a)
		if out.Empty() {
			return nil
		}
		return &out
	}
	left := types.NormalizeToolRefinementHint(*a)
	right := types.NormalizeToolRefinementHint(*b)
	if left.ReasonCode == "" {
		left.ReasonCode = right.ReasonCode
	}
	left.ResultTruncated = left.ResultTruncated || right.ResultTruncated
	left.CandidateBudgetTruncated = left.CandidateBudgetTruncated || right.CandidateBudgetTruncated
	if left.UniverseExcludedReason == "" {
		left.UniverseExcludedReason = right.UniverseExcludedReason
	}
	left.SkippedLargeCandidates = append(left.SkippedLargeCandidates, right.SkippedLargeCandidates...)
	left.ExcludedRoots = append(left.ExcludedRoots, right.ExcludedRoots...)
	left.TopSourceClasses = append(left.TopSourceClasses, right.TopSourceClasses...)
	if left.NextCursor == "" {
		left.NextCursor = right.NextCursor
	}
	if left.PreferredNextTool == "" {
		left.PreferredNextTool = right.PreferredNextTool
	}
	if left.PreferredParams == nil && len(right.PreferredParams) > 0 {
		left.PreferredParams = map[string]string{}
	}
	for k, v := range right.PreferredParams {
		if _, ok := left.PreferredParams[k]; !ok {
			left.PreferredParams[k] = v
		}
	}
	left.RequiredFields = append(left.RequiredFields, right.RequiredFields...)
	out := types.NormalizeToolRefinementHint(left)
	if out.Empty() {
		return nil
	}
	return &out
}

func promoteSourceToolRefinementToRepoMap(ctx *types.BusContext, hint types.ToolRefinementHint) types.ToolRefinementHint {
	hint = types.NormalizeToolRefinementHint(hint)
	if ctx == nil || ctx.AnalysisIR == nil || hint.Empty() {
		return hint
	}
	policy := types.CompileRepoMapNavigationPolicy(ctx.AnalysisIR.RequestModel, &ctx.AnalysisIR.AnswerContract, ctx.ExploreLanePlan)
	step, ok := policy.FirstHopStep()
	if !ok {
		if ctx.AnalysisIR.RequestModel.SourceInventoryProfile == nil || !ctx.AnalysisIR.RequestModel.SourceInventoryProfile.Active() {
			return hint
		}
		step = types.RepoMapNavigationStep{
			Route:   types.RepoMapNavigationRouteSourceInventory,
			Purpose: types.RepoMapNavigationPurposeInventory,
			Params:  []string{"scope", "roles", "include_attributes=false"},
		}
	}
	params := repoMapPreferredParamsFromNavigationStep(step, policy, ctx.AnalysisIR.RequestModel)
	if len(params) == 0 {
		return hint
	}
	hint.PreferredNextTool = "repo_map"
	hint.PreferredParams = params
	hint.RequiredFields = repoMapRequiredFieldsFromNavigationStep(step, params)
	return types.NormalizeToolRefinementHint(hint)
}

func repoMapPreferredParamsFromNavigationStep(step types.RepoMapNavigationStep, policy types.RepoMapNavigationPolicy, rm types.RequestModel) map[string]string {
	route := strings.TrimSpace(string(step.Route))
	if route == "" {
		return nil
	}
	params := map[string]string{"view": route}
	if query := strings.Join(policy.QueryTermList(4), " "); strings.TrimSpace(query) != "" {
		params["query"] = strings.TrimSpace(query)
	}
	if step.Route == types.RepoMapNavigationRouteSourceInventory {
		params["include_attributes"] = "false"
		if rm.SourceInventoryProfile != nil {
			if roles := answerCandidateRolesParam(rm.SourceInventoryProfile.PrincipalTargetRoles()); roles != "" {
				params["roles"] = roles
			}
		}
	}
	if len(step.RelationHint) > 0 {
		if kinds := typedRelationKindsParam(step.RelationHint); kinds != "" {
			params["relation_kinds"] = kinds
		}
	}
	return params
}

func repoMapRequiredFieldsFromNavigationStep(step types.RepoMapNavigationStep, params map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(field string) {
		field = strings.TrimSpace(field)
		if field == "" || strings.Contains(field, "=") || seen[field] {
			return
		}
		if _, alreadySuggested := params[field]; alreadySuggested {
			return
		}
		seen[field] = true
		out = append(out, field)
	}
	for _, field := range step.Params {
		add(field)
	}
	return out
}

func answerCandidateRolesParam(in []types.AnswerCandidateRole) string {
	var out []string
	seen := map[string]bool{}
	for _, role := range in {
		value := strings.TrimSpace(string(role))
		if value == "" || value == string(types.AnswerCandidateRoleUnknown) || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return strings.Join(out, ",")
}

func typedRelationKindsParam(in []types.TypedRelationKind) string {
	var out []string
	seen := map[string]bool{}
	for _, kind := range in {
		value := strings.TrimSpace(string(kind))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return strings.Join(out, ",")
}

func grepNoMatchRefinement(ctx *types.BusContext, params grepToolParams, searchPath string, base *types.ToolRefinementHint) *types.ToolRefinementHint {
	return mergeToolRefinementHints(base, configuredSearchExcludeRootsRefinement(ctx, searchPath, "grep"))
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
    "files_only":    {"type": "boolean", "description": "If true, return only file paths that contain matches (like grep -l), not the matching lines. Useful for breadth scans to discover which files are relevant without flooding the output."},
    "fixed_string":  {"type": "boolean", "description": "Treat pattern as literal text, not regex. Use for exact log/trace/event/config strings with punctuation, or when regex escaping is not needed."},
    "line_start":    {"type": "integer", "description": "1-based inclusive start line for a single-file search window. Use only after discovering a line vicinity."},
    "line_end":      {"type": "integer", "description": "1-based inclusive end line for a single-file search window. Use only with path set to one file."}
  },
  "required": ["pattern"]
}`)
}

func (t *GrepTool) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p grepToolParams
	if _, decodeFailure, err := decodeStrictToolParams(t.Name(), params, t.Parameters(), &p, nil); err != nil {
		return *decodeFailure, err
	}

	// L1 negative-knowledge gate (R3 second-axis enforcement):
	// the search PATH or the search PATTERN was already shown to be
	// absent in the current repository by an input-side validator.
	// Refuse with the generic per-class reason.
	if ctx != nil {
		if denial, denied := ctx.TypedDenials.FirstPathDenial(p.Path); denied {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   denial.HumanRefusalReason("grep"),
				Timestamp: time.Now(),
			}, nil
		}
		if denial, denied := ctx.TypedDenials.FirstSymbolDenial(p.Pattern); denied {
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
	searchPathIsFile := false
	if info, err := os.Stat(searchPath); err == nil {
		searchPathIsFile = !info.IsDir()
	}
	lineWindow := p.LineStart > 0 || p.LineEnd > 0
	if p.LineStart < 0 || p.LineEnd < 0 {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "invalid params: grep line_start/line_end must be positive 1-based line numbers",
			Timestamp: time.Now(),
		}, nil
	}
	if lineWindow {
		if !searchPathIsFile {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   "invalid params: grep line_start/line_end require path to be one explicit file; omit the line window for repository or directory searches",
				Timestamp: time.Now(),
			}, nil
		}
		if p.LineStart == 0 {
			p.LineStart = 1
		}
		if p.LineEnd > 0 && p.LineEnd < p.LineStart {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("invalid params: grep line_end (%d) must be >= line_start (%d)", p.LineEnd, p.LineStart),
				Timestamp: time.Now(),
			}, nil
		}
	}
	dirFilter := NewSearchDirFilterWithOptions(ctx.RepoRoot, searchPath, SearchDirFilterOptions{
		IncludeAuxiliarySource: grepShouldAutoIncludeAuxiliary(ctx, p),
	})
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
	searchBackend := SearchCommand()
	useNativeBackend := grepShouldUseNativeBackend(searchBackend, ctx != nil && ctx.RepoRoot != "", searchPathIsFile)
	if grepShouldAutoIncludeAuxiliary(ctx, p) {
		useNativeBackend = true
	}
	if lineWindow {
		useNativeBackend = true
	}
	if searchBackend == "grep" && searchPathIsFile && !p.FixedString && grepPatternHasCommonRegexShorthand(p.Pattern) {
		useNativeBackend = true
	}
	if useNativeBackend {
		var shouldSkip func(path string, d fs.DirEntry) bool
		if ctx != nil && ctx.RepoRoot != "" {
			shouldSkip = func(path string, d fs.DirEntry) bool {
				rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, path)
				return ok && dirFilter.ExcludesRepoRelativePath(rel)
			}
		}
		var includeGlobs []string
		if strings.TrimSpace(p.FileType) != "" {
			includeGlobs = fileTypeToGlobs(p.FileType)
		}
		nres, nerr := NativeGrep(searchCtx, NativeGrepOpts{
			Pattern:      p.Pattern,
			Root:         searchPath,
			IgnoreCase:   caseInsensitive,
			FixedString:  p.FixedString,
			FilesOnly:    p.FilesOnly,
			ContextLines: contextLines,
			LineStart:    p.LineStart,
			LineEnd:      p.LineEnd,
			Include:      p.Include,
			IncludeGlobs: includeGlobs,
			ExcludeDirs:  dirFilter.AnyLevelPatterns(),
			ShouldSkip:   shouldSkip,
			DisplayRoot:  ctxRepoRoot(ctx),
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
			"fixed_string", boolBannerValue(p.FixedString),
			"line_start", positiveIntBannerValue(p.LineStart),
			"line_end", positiveIntBannerValue(p.LineEnd),
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
				ToolName:   t.Name(),
				Success:    false,
				Summary:    paramsBanner + "grep timed out after 60s — pattern may be too broad or repo too large",
				Refinement: configuredSearchExcludeRootsRefinement(ctx, searchPath, "grep"),
				Timestamp:  time.Now(),
			}, nil
		}
		if nres.Matches == 0 {
			suffix := grepNoMatchBody(ctx, p)
			var refinement *types.ToolRefinementHint
			if nres.SkippedLargeFiles > 0 {
				suffix = grepSkippedLargeFilesNoMatchBody(nres.SkippedLargeFilePaths, nres.SkippedLargeFiles)
				refinement = grepSkippedLargeRefinement(nres.SkippedLargeFilePaths, nres.SkippedLargeFiles)
			}
			refinement = grepNoMatchRefinement(ctx, p, searchPath, refinement)
			return types.ToolResult{
				ToolName:   t.Name(),
				Success:    true,
				Summary:    paramsBanner + suffix,
				Refinement: refinement,
				Timestamp:  time.Now(),
			}, nil
		}
		countBanner := grepCountBanner(nres.Matches, p.FilesOnly, contextLines)
		summary, ref, refinement := finalizeGrepOutput(ctx, p, countBanner, paramsBanner, nres.Output)
		return types.ToolResult{
			ToolName:   t.Name(),
			Success:    true,
			Summary:    summary,
			RawRef:     ref,
			Refinement: refinement,
			Timestamp:  time.Now(),
		}, nil
	}

	if searchBackend == "rg" {
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
		if p.FixedString {
			args = append(args, "-F")
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
			for _, glob := range fileTypeToGlobs(p.FileType) {
				args = append(args, "--glob", glob)
			}
		}
		if p.Include != "" {
			args = append(args, "--glob", p.Include)
		}
		args = append(args, p.Pattern, commandSearchPath)
		cmd = exec.CommandContext(searchCtx, SearchExecutable(), args...)
	} else {
		// GNU grep fallback: -E for ERE unless fixed_string=true, -I to skip binary files,
		// -H forces filename prefix (see ripgrep comment above for why
		// single-file paths break extractFileCoverage without it).
		args := []string{"-rnEIH"}
		if p.FilesOnly {
			args = []string{"-rlEI"}
		}
		if p.FixedString {
			if p.FilesOnly {
				args = []string{"-rlFI"}
			} else {
				args = []string{"-rnFIH"}
			}
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
		"fixed_string", boolBannerValue(p.FixedString),
		"line_start", positiveIntBannerValue(p.LineStart),
		"line_end", positiveIntBannerValue(p.LineEnd),
	)

	if grepParamsTargetRuntimeArtifactFile(ctx, p) && !p.FilesOnly {
		capture, err := runRuntimeArtifactGrepStream(ctx, cmd, p.FilesOnly)
		defer capture.cleanup()

		if searchCtx.Err() == context.DeadlineExceeded {
			return types.ToolResult{
				ToolName:   t.Name(),
				Success:    false,
				Summary:    paramsBanner + "grep timed out after 60s — pattern may be too broad or repo too large",
				Refinement: configuredSearchExcludeRootsRefinement(ctx, searchPath, "grep"),
				Timestamp:  time.Now(),
			}, nil
		}
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				return types.ToolResult{
					ToolName:   t.Name(),
					Success:    true,
					Summary:    paramsBanner + grepNoMatchBody(ctx, p),
					Refinement: grepNoMatchRefinement(ctx, p, searchPath, nil),
					Timestamp:  time.Now(),
				}, nil
			}
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   paramsBanner + fmt.Sprintf("grep failed: %v: %s", err, capture.Stderr),
				Timestamp: time.Now(),
			}, nil
		}
		if capture.Lines == 0 {
			return types.ToolResult{
				ToolName:   t.Name(),
				Success:    true,
				Summary:    paramsBanner + grepNoMatchBody(ctx, p),
				Refinement: grepNoMatchRefinement(ctx, p, searchPath, nil),
				Timestamp:  time.Now(),
			}, nil
		}
		countBanner := grepCountBanner(capture.Lines, false, contextLines)
		if capture.FullInMemory && capture.Lines <= grepGovernorLineEntryThreshold && capture.Bytes <= grepGovernorByteThreshold {
			summary, ref, refinement := finalizeGrepOutput(ctx, p, countBanner, paramsBanner, capture.InlineOutput)
			return types.ToolResult{
				ToolName:   t.Name(),
				Success:    true,
				Summary:    summary,
				RawRef:     ref,
				Refinement: refinement,
				Timestamp:  time.Now(),
			}, nil
		}
		summary, ref := compactStreamedRuntimeArtifactGrepOutput(ctx, p, countBanner, paramsBanner, capture)
		refinement := grepBroadResultRefinement(ctx, p, strings.Join(capture.PreviewLines, "\n"))
		return types.ToolResult{
			ToolName:   t.Name(),
			Success:    true,
			Summary:    summary,
			RawRef:     ref,
			Refinement: refinement,
			Timestamp:  time.Now(),
		}, nil
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()

	// Timeout produces a context.DeadlineExceeded — surface it clearly.
	if searchCtx.Err() == context.DeadlineExceeded {
		return types.ToolResult{
			ToolName:   t.Name(),
			Success:    false,
			Summary:    paramsBanner + "grep timed out after 60s — pattern may be too broad or repo too large",
			Refinement: configuredSearchExcludeRootsRefinement(ctx, searchPath, "grep"),
			Timestamp:  time.Now(),
		}, nil
	}

	// grep returns exit code 1 when no matches found — not a real error.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return types.ToolResult{
				ToolName:   t.Name(),
				Success:    true,
				Summary:    paramsBanner + grepNoMatchBody(ctx, p),
				Refinement: grepNoMatchRefinement(ctx, p, searchPath, nil),
				Timestamp:  time.Now(),
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
		countBanner = grepCountBanner(lines, p.FilesOnly, contextLines)
	}
	summary, ref, refinement := finalizeGrepOutput(ctx, p, countBanner, paramsBanner, output)
	return types.ToolResult{
		ToolName:   t.Name(),
		Success:    true,
		Summary:    summary,
		RawRef:     ref,
		Refinement: refinement,
		Timestamp:  time.Now(),
	}, nil
}

const (
	grepGovernorLineEntryThreshold = 80
	grepGovernorFileEntryThreshold = 120
	grepGovernorByteThreshold      = 16 * 1024
	grepGovernorLineProductionCap  = 48
	grepGovernorFileProductionCap  = 80
	grepGovernorAuxiliaryCap       = 24
	grepGovernorOtherCap           = 16
	grepLineWindowHintMax          = 4
	grepLineWindowHalfSpan         = 20
)

type grepLineWindow struct {
	Path  string
	Line  int
	First int
	Last  int
	Count int
}

func finalizeGrepOutput(ctx *types.BusContext, params grepToolParams, countBanner, paramsBanner, rawOutput string) (summary, ref string, refinement *types.ToolRefinementHint) {
	annotated := annotateGrepOutputByRelevance(ctx, params, rawOutput)
	if compacted, rawRef, ok := compactBroadGrepOutput(ctx, params, countBanner, paramsBanner, rawOutput, annotated); ok {
		refinement := grepBroadResultRefinement(ctx, params, rawOutput)
		refinement = mergeToolRefinementHints(refinement, configuredSearchExcludeRootsRefinement(ctx, resolveToolPath(ctx, params.Path), "grep"))
		return compacted, rawRef, refinement
	}
	payload := countBanner + paramsBanner
	if grepParamsTargetRuntimeArtifactFile(ctx, params) {
		payload += grepRuntimeArtifactTraceSpanAdvisory(params, rawOutput)
	}
	payload += annotated
	summary, ref = StoreBlob(ctx, "grep", payload)
	return summary, ref, nil
}

func compactBroadGrepOutput(ctx *types.BusContext, params grepToolParams, countBanner, paramsBanner, rawOutput, annotatedOutput string) (summary, rawRef string, ok bool) {
	production, auxiliary, other, relevanceStats := partitionGrepOutputByRelevance(ctx, params, rawOutput)
	entryCount := len(production) + len(auxiliary) + len(other)
	threshold := grepGovernorLineEntryThreshold
	prodCap := grepGovernorLineProductionCap
	mode := "line_output"
	if params.FilesOnly {
		threshold = grepGovernorFileEntryThreshold
		prodCap = grepGovernorFileProductionCap
		mode = "files_only"
	}
	if entryCount <= threshold && len(annotatedOutput) <= grepGovernorByteThreshold {
		return "", "", false
	}

	fullPayload := countBanner + paramsBanner + annotatedOutput
	if ctx != nil && strings.TrimSpace(ctx.WorkDir) != "" {
		rawRef = StoreBlobArtifact(ctx.WorkDir, "grep", "grep-full.txt", fullPayload)
	}

	productionHeader, otherHeader, auxiliaryHeader := grepPathRoleSectionHeaders(ctx)
	var b strings.Builder
	b.WriteString(countBanner)
	b.WriteString(paramsBanner)
	b.WriteString("[grep retrieval governor]\n")
	fmt.Fprintf(&b, "decision=broad_result_compacted mode=%s entries=%d production=%d auxiliary=%d other=%d\n",
		mode, entryCount, len(production), len(auxiliary), len(other))
	fmt.Fprintf(&b, "inline_caps production=%d auxiliary=%d other=%d\n", prodCap, grepGovernorAuxiliaryCap, grepGovernorOtherCap)
	if relevanceStats != "" {
		fmt.Fprintf(&b, "relevance=%s\n", relevanceStats)
	}
	if rawRef != "" {
		fmt.Fprintf(&b, "full_raw_saved=%s\n", rawRef)
	} else {
		b.WriteString("full_raw_saved=unavailable (no workdir configured)\n")
	}
	if grepParamsTargetRuntimeArtifactFile(ctx, params) {
		if advisory := grepRuntimeArtifactParamAdvisory(params); advisory != "" {
			b.WriteString(advisory)
		}
		b.WriteString("next_shape=single large runtime artifact matched too broadly; narrow with one exact timestamp/literal/thread id, then read_file around the returned line numbers for evidence. For numeric time-window filtering, use a deterministic command that preserves original line numbers (for example `grep -n` before awk/head), then read_file the selected line range before emitting line-scope evidence.\n")
		if advisory := grepRuntimeArtifactTraceSpanAdvisory(params, strings.Join(production, "\n")); advisory != "" {
			b.WriteString(advisory)
		}
		if hint := grepLineWindowHint(production, params.FilesOnly); hint != "" {
			b.WriteString(hint)
		}
	} else if params.FilesOnly {
		b.WriteString("next_shape=read_file a top production path for exact evidence, or re-run grep with a narrower path/file_type.\n")
	} else {
		b.WriteString("next_shape=for exact line evidence, re-run grep with path set to one top production file and context_lines<=3; for discovery use files_only=true.\n")
	}
	if hint := grepBroadResultRelationNavigationHint(ctx, params, production); hint != "" {
		b.WriteString(hint)
	}

	writeCappedGrepSection(&b, productionHeader, production, prodCap, "no non-auxiliary matches found")
	writeCappedGrepSection(&b, auxiliaryHeader, auxiliary, grepGovernorAuxiliaryCap, "")
	writeCappedGrepSection(&b, otherHeader, other, grepGovernorOtherCap, "")
	return b.String(), rawRef, true
}

func grepBroadResultRefinement(ctx *types.BusContext, params grepToolParams, rawOutput string) *types.ToolRefinementHint {
	production, auxiliary, other, _ := partitionGrepOutputByRelevance(ctx, params, rawOutput)
	entryCount := len(production) + len(auxiliary) + len(other)
	threshold := grepGovernorLineEntryThreshold
	if params.FilesOnly {
		threshold = grepGovernorFileEntryThreshold
	}
	if entryCount <= threshold && len(rawOutput) <= grepGovernorByteThreshold {
		return nil
	}
	ordered := make([]string, 0, len(production)+len(auxiliary)+len(other))
	ordered = append(ordered, production...)
	ordered = append(ordered, auxiliary...)
	ordered = append(ordered, other...)
	topPath := firstGrepOutputPath(ordered, params.FilesOnly)
	hint := types.ToolRefinementHint{
		ReasonCode:      "grep_result_truncated",
		ResultTruncated: true,
	}
	if params.FilesOnly {
		hint.PreferredNextTool = "read_file"
		if topPath != "" {
			hint.PreferredParams = map[string]string{"path": topPath}
		}
		out := promoteSourceToolRefinementToRepoMap(ctx, hint)
		return &out
	}
	hint.PreferredNextTool = "grep"
	hint.PreferredParams = map[string]string{
		"files_only":    "false",
		"context_lines": "3",
	}
	if topPath != "" {
		hint.PreferredParams["path"] = topPath
	} else if strings.TrimSpace(params.Path) != "" && strings.TrimSpace(params.Path) != "." {
		hint.PreferredParams["path"] = strings.TrimSpace(params.Path)
	}
	if grepParamsTargetRuntimeArtifactFile(ctx, params) {
		hint.PreferredParams["context_lines"] = "0"
		hint.RequiredFields = []string{"pattern"}
		return &hint
	}
	if strings.TrimSpace(params.Pattern) != "" {
		hint.PreferredParams["pattern"] = strings.TrimSpace(params.Pattern)
	}
	if strings.TrimSpace(params.FileType) != "" && topPath == "" {
		hint.PreferredParams["file_type"] = strings.TrimSpace(params.FileType)
	}
	if strings.TrimSpace(params.Include) != "" && topPath == "" {
		hint.PreferredParams["include"] = strings.TrimSpace(params.Include)
	}
	out := promoteSourceToolRefinementToRepoMap(ctx, hint)
	return &out
}

func firstGrepOutputPath(lines []string, filesOnly bool) string {
	for _, line := range lines {
		if path, ok := grepOutputLinePath(line, filesOnly); ok {
			return path
		}
	}
	return ""
}

func grepSkippedLargeRefinement(paths []string, skippedTotal int) *types.ToolRefinementHint {
	if skippedTotal <= 0 && len(paths) == 0 {
		return nil
	}
	candidates := normalizeGrepSkippedLargeCandidates(paths, skippedTotal)
	hint := types.ToolRefinementHint{
		ReasonCode:             "skipped_large_candidates",
		SkippedLargeCandidates: candidates,
		PreferredNextTool:      "grep",
		PreferredParams: map[string]string{
			"files_only":    "false",
			"context_lines": "0",
		},
		RequiredFields: []string{"pattern"},
	}
	if nextPath := firstNonEmptyString(paths); nextPath != "" {
		hint.PreferredParams["path"] = nextPath
	}
	return &hint
}

func normalizeGrepSkippedLargeCandidates(paths []string, skippedTotal int) []string {
	if len(paths) == 0 {
		return nil
	}
	limit := len(paths)
	if skippedTotal > 0 && skippedTotal < limit {
		limit = skippedTotal
	}
	out := make([]string, 0, limit)
	seen := map[string]bool{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
		if len(out) >= limit {
			break
		}
	}
	return out
}

type runtimeArtifactGrepCapture struct {
	InlineOutput string
	PreviewLines []string
	LineWindows  []grepLineWindow
	TempPath     string
	Stderr       string
	Lines        int
	Bytes        int
	FullInMemory bool
}

func (c runtimeArtifactGrepCapture) cleanup() {
	if strings.TrimSpace(c.TempPath) != "" {
		_ = os.Remove(c.TempPath)
	}
}

func runRuntimeArtifactGrepStream(ctx *types.BusContext, cmd *exec.Cmd, filesOnly bool) (runtimeArtifactGrepCapture, error) {
	var capture runtimeArtifactGrepCapture
	capture.FullInMemory = true

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return capture, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	var temp *os.File
	if ctx != nil && strings.TrimSpace(ctx.WorkDir) != "" {
		if err := os.MkdirAll(ctx.WorkDir, 0o755); err == nil {
			if f, err := os.CreateTemp(ctx.WorkDir, "grep-stream-*.tmp"); err == nil {
				temp = f
				capture.TempPath = f.Name()
			}
		}
	}

	if err := cmd.Start(); err != nil {
		if temp != nil {
			_ = temp.Close()
		}
		capture.Stderr = stderr.String()
		return capture, err
	}

	reader := bufio.NewReaderSize(stdout, 64*1024)
	var inline strings.Builder
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			capture.Lines++
			capture.Bytes += len(line)
			if temp != nil {
				_, _ = temp.WriteString(line)
			}
			if capture.FullInMemory {
				if inline.Len()+len(line) <= grepGovernorByteThreshold {
					inline.WriteString(line)
				} else {
					capture.FullInMemory = false
				}
			}
			if len(capture.PreviewLines) < grepGovernorLineProductionCap {
				capture.PreviewLines = append(capture.PreviewLines, strings.TrimRight(line, "\r\n"))
			}
			if !filesOnly {
				capture.LineWindows = appendGrepLineWindow(capture.LineWindows, strings.TrimRight(line, "\r\n"), true, grepLineWindowHintMax)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			waitErr := cmd.Wait()
			if temp != nil {
				_ = temp.Close()
			}
			capture.Stderr = stderr.String()
			if waitErr != nil {
				return capture, waitErr
			}
			return capture, readErr
		}
	}
	if temp != nil {
		_ = temp.Close()
	}
	err = cmd.Wait()
	capture.Stderr = stderr.String()
	if capture.FullInMemory {
		capture.InlineOutput = inline.String()
	}
	return capture, err
}

func compactStreamedRuntimeArtifactGrepOutput(ctx *types.BusContext, params grepToolParams, countBanner, paramsBanner string, capture runtimeArtifactGrepCapture) (summary, rawRef string) {
	if ctx != nil && strings.TrimSpace(ctx.WorkDir) != "" && strings.TrimSpace(capture.TempPath) != "" {
		rawRef = StoreBlobArtifactFromFile(ctx.WorkDir, "grep", "grep-full.txt", capture.TempPath, countBanner+paramsBanner)
	}

	var b strings.Builder
	b.WriteString(countBanner)
	b.WriteString(paramsBanner)
	b.WriteString("[grep retrieval governor]\n")
	fmt.Fprintf(&b, "decision=broad_result_compacted mode=line_output entries=%d production=%d auxiliary=0 other=0\n", capture.Lines, capture.Lines)
	fmt.Fprintf(&b, "inline_caps production=%d auxiliary=0 other=0\n", grepGovernorLineProductionCap)
	fmt.Fprintf(&b, "relevance=explicit_file=%d\n", capture.Lines)
	if rawRef != "" {
		fmt.Fprintf(&b, "full_raw_saved=%s\n", rawRef)
	} else {
		b.WriteString("full_raw_saved=unavailable (no workdir configured)\n")
	}
	if advisory := grepRuntimeArtifactParamAdvisory(params); advisory != "" {
		b.WriteString(advisory)
	}
	b.WriteString("next_shape=single large runtime artifact matched too broadly; narrow with one exact timestamp/literal/thread id, then read_file around the returned line numbers for evidence. For numeric time-window filtering, use a deterministic command that preserves original line numbers (for example `grep -n` before awk/head), then read_file the selected line range before emitting line-scope evidence.\n")
	if advisory := grepRuntimeArtifactTraceSpanAdvisory(params, strings.Join(capture.PreviewLines, "\n")); advisory != "" {
		b.WriteString(advisory)
	}
	windows := capture.LineWindows
	if len(windows) == 0 {
		windows = collectGrepLineWindows(capture.PreviewLines, params.FilesOnly, grepLineWindowHintMax)
	}
	if hint := renderGrepLineWindowHints(windows); hint != "" {
		b.WriteString(hint)
	}
	writeCappedGrepSection(&b, "[grep production matches]", capture.PreviewLines, grepGovernorLineProductionCap, "no runtime-artifact matches returned")
	if omitted := capture.Lines - len(capture.PreviewLines); omitted > 0 {
		fmt.Fprintf(&b, "... omitted %d output line(s) from inline preview; page the full_raw_saved artifact or narrow the grep.\n", omitted)
	}
	return b.String(), rawRef
}

func grepNoMatchBody(ctx *types.BusContext, params grepToolParams) string {
	body := "no matches found"
	lineWindowNote := ""
	if params.LineStart > 0 || params.LineEnd > 0 {
		lineWindowNote = "line_window_note=search was limited to the requested line window; omit or widen line_start/line_end if the target may be outside it.\n"
	}
	if grepParamsTargetRuntimeArtifactFile(ctx, params) {
		var b strings.Builder
		b.WriteString("no matches found\n")
		b.WriteString(lineWindowNote)
		if advisory := grepRuntimeArtifactParamAdvisory(params); advisory != "" {
			b.WriteString(advisory)
		}
		if advisory := grepRuntimeArtifactTraceSpanAdvisory(params, ""); advisory != "" {
			b.WriteString(advisory)
		}
		if advisory := grepFixedStringRegexAdvisory(params); advisory != "" {
			b.WriteString(advisory)
		}
		b.WriteString("no_match_advisory=single runtime/log/trace artifact searched; this only means the exact pattern matched zero lines. Regex is line-order-sensitive: split combined patterns into one exact literal/timestamp/thread id/event name first, inspect one returned line to preserve the observed field order, then recombine only if needed. Use fixed_string=true for punctuation-heavy text. If you know the vicinity, use line_start/line_end on this file. For numeric time ranges, a deterministic command can filter the interval, but preserve original line numbers (for example `grep -n`) so read_file can ground the selected lines.\n")
		if !params.FixedString && grepPatternHasCommonRegexShorthand(params.Pattern) {
			b.WriteString("regex_compatibility_note=pattern contains \\d/\\s/\\w-style shorthand; codrax normalizes common single-file cases, but a simpler literal or fixed_string=true is safer for artifact discovery.\n")
		}
		return b.String()
	}
	if lineWindowNote != "" {
		body += "\n" + lineWindowNote
	}
	if !params.FixedString && grepPatternHasCommonRegexShorthand(params.Pattern) {
		body += "\nregex_compatibility_note=pattern contains \\d/\\s/\\w-style shorthand; regex support varies by backend. Try an explicit character class such as [0-9] or fixed_string=true when searching literal text.\n"
	}
	if advisory := grepFixedStringRegexAdvisory(params); advisory != "" {
		body += "\n" + strings.TrimRight(advisory, "\n") + "\n"
	}
	if advisory := grepPathDiscoveryAdvisory(params); advisory != "" {
		body += "\n" + advisory
	}
	return body
}

func grepPathDiscoveryAdvisory(params grepToolParams) string {
	if strings.TrimSpace(params.Include) == "" && strings.TrimSpace(params.FileType) == "" {
		return ""
	}
	return "path_discovery_advisory=grep searches file contents, not file names. If the goal is file-path/glob discovery, call list_files with recursive=true plus include/file_type; add include_auxiliary=true only when the typed scope includes all/auxiliary repo-owned corpus material or a production-only scan was empty.\n"
}

func grepFixedStringRegexAdvisory(params grepToolParams) string {
	if !params.FixedString {
		return ""
	}
	markers := grepFixedStringRegexMarkers(params.Pattern)
	if len(markers) == 0 {
		return ""
	}
	return "fixed_string_regex_note=fixed_string=true treats regex syntax as literal text; pattern contains " + strings.Join(markers, ", ") + ". If you intended regex matching, re-run with fixed_string=false; if you intended exact text, remove regex escapes/metacharacters from the literal.\n"
}

func grepFixedStringRegexMarkers(pattern string) []string {
	checks := []string{`.*`, `.+`, `\d`, `\s`, `\w`, `\b`, `\.`}
	var out []string
	for _, marker := range checks {
		if strings.Contains(pattern, marker) {
			out = append(out, marker)
		}
	}
	for _, marker := range []string{`[0-9`, `[A-Z`, `[a-z`} {
		if strings.Contains(pattern, marker) {
			out = append(out, marker+"]")
		}
	}
	return out
}

func grepRuntimeArtifactParamAdvisory(params grepToolParams) string {
	if strings.TrimSpace(params.FileType) == "" && strings.TrimSpace(params.Include) == "" && (params.ContextLines == nil || *params.ContextLines <= 0) {
		return ""
	}
	var notes []string
	if strings.TrimSpace(params.FileType) != "" || strings.TrimSpace(params.Include) != "" {
		notes = append(notes, "path is already one concrete runtime/log/trace file; file_type/include filters are redundant and may distract from narrowing by timestamp/thread/event")
	}
	if params.ContextLines != nil && *params.ContextLines > 0 && params.LineStart <= 0 && params.LineEnd <= 0 {
		notes = append(notes, "context_lines expands broad artifact searches; first narrow to a small vicinity, then use read_file for surrounding context")
	}
	if len(notes) == 0 {
		return ""
	}
	return "artifact_search_advisory=" + strings.Join(notes, "; ") + ".\n"
}

type grepTraceMarkerBegin struct {
	Path     string
	Line     int
	Action   string
	SpanPID  string
	SpanName string
	Cookie   string
}

func grepRuntimeArtifactTraceSpanAdvisory(params grepToolParams, output string) string {
	var b strings.Builder
	if spanName, ok := grepNamedTraceEndPattern(params.Pattern); ok {
		b.WriteString("trace_marker_span_end_shape=pattern appears to search a named B/E end marker")
		if spanName != "" {
			fmt.Fprintf(&b, " for span_name=%q", spanName)
		}
		b.WriteString("; ftrace/atrace synchronous span ends normally appear as unnamed E|<pid> or bare E on the same ftrace thread stack. A zero match or broad regex result for E|<pid>|<span_name> is not evidence that the span never ended. Use trace_query(view=\"span_window\"")
		if path := grepTraceQueryPathHint(params, ""); path != "" {
			fmt.Fprintf(&b, ", path=%q", path)
		}
		if spanName != "" {
			fmt.Fprintf(&b, ", span_name=%q", spanName)
		} else {
			b.WriteString(", span_name=\"<begin span label>\"")
		}
		b.WriteString(") to resolve the end by stack pairing.\n")
	}
	begin, ok := firstGrepTraceMarkerBegin(output)
	if !ok {
		return b.String()
	}
	b.WriteString("trace_marker_span_begin_hint=matched a trace marker begin row")
	if begin.Action == "S" {
		b.WriteString(" (async S/F)")
	} else {
		b.WriteString(" (sync B/E)")
	}
	if begin.SpanName != "" {
		fmt.Fprintf(&b, " span_name=%q", begin.SpanName)
	}
	if begin.SpanPID != "" {
		fmt.Fprintf(&b, " marker_pid=%s", begin.SpanPID)
	}
	if begin.Cookie != "" {
		fmt.Fprintf(&b, " cookie=%s", begin.Cookie)
	}
	b.WriteString("; do not infer the end by grepping for E|<pid>|<span_name>. Next call: trace_query(view=\"span_window\"")
	if path := grepTraceQueryPathHint(params, begin.Path); path != "" {
		fmt.Fprintf(&b, ", path=%q", path)
	}
	if begin.SpanName != "" {
		fmt.Fprintf(&b, ", span_name=%q", begin.SpanName)
	}
	if begin.Line > 0 {
		fmt.Fprintf(&b, ", line_start=%d", begin.Line)
	}
	b.WriteString("); omit line_end unless you intentionally want to stop the stack search before a later E row.\n")
	return b.String()
}

func grepNamedTraceEndPattern(pattern string) (spanName string, ok bool) {
	normalized := strings.TrimSpace(pattern)
	normalized = strings.ReplaceAll(normalized, `\|`, "|")
	normalized = strings.Trim(normalized, `"'`)
	idx := strings.Index(normalized, "E|")
	if idx < 0 {
		return "", false
	}
	parts := strings.Split(normalized[idx:], "|")
	if len(parts) < 3 || strings.TrimSpace(parts[0]) != "E" {
		return "", false
	}
	pid := strings.Trim(strings.TrimSpace(parts[1]), `^$"'`)
	if pid == "" {
		return "", false
	}
	span := cleanGrepTraceSpanHintLabel(parts[2])
	if span == "" {
		return "", false
	}
	return span, true
}

func firstGrepTraceMarkerBegin(output string) (grepTraceMarkerBegin, bool) {
	for _, line := range strings.Split(output, "\n") {
		fields, ok := grepTraceMarkerFields(line)
		if !ok {
			continue
		}
		parts := strings.Split(fields, "|")
		if len(parts) < 3 {
			continue
		}
		action := strings.TrimSpace(parts[0])
		if action != "B" && action != "S" {
			continue
		}
		path, lineNo, _, _ := parseGrepOutputLineLocation(line)
		begin := grepTraceMarkerBegin{
			Path:     path,
			Line:     lineNo,
			Action:   action,
			SpanPID:  strings.TrimSpace(parts[1]),
			SpanName: cleanGrepTraceSpanHintLabel(parts[2]),
		}
		if action == "S" && len(parts) >= 4 {
			begin.Cookie = cleanGrepTraceSpanHintLabel(parts[3])
		}
		if begin.SpanName == "" {
			continue
		}
		return begin, true
	}
	return grepTraceMarkerBegin{}, false
}

func grepTraceMarkerFields(line string) (string, bool) {
	for _, marker := range []string{"tracing_mark_write:", "print:"} {
		idx := strings.Index(line, marker)
		if idx < 0 {
			continue
		}
		fields := strings.TrimSpace(line[idx+len(marker):])
		if strings.HasPrefix(fields, "B|") || strings.HasPrefix(fields, "S|") {
			return fields, true
		}
	}
	return "", false
}

func grepTraceQueryPathHint(params grepToolParams, observed string) string {
	if path := strings.TrimSpace(params.Path); path != "" {
		return path
	}
	return strings.TrimSpace(observed)
}

func cleanGrepTraceSpanHintLabel(raw string) string {
	label := strings.TrimSpace(raw)
	label = strings.Trim(label, `"'`)
	label = strings.TrimPrefix(label, "^")
	label = strings.TrimSuffix(label, "$")
	label = strings.TrimSuffix(label, ".*")
	label = strings.TrimSuffix(label, ".+")
	return strings.TrimSpace(label)
}

func grepBroadResultRelationNavigationHint(ctx *types.BusContext, params grepToolParams, production []string) string {
	if ctx == nil || ctx.AnalysisIR == nil || len(production) == 0 {
		return ""
	}
	if grepParamsTargetRuntimeArtifactFile(ctx, params) {
		return ""
	}
	policy := types.CompileRepoMapNavigationPolicy(ctx.AnalysisIR.RequestModel, &ctx.AnalysisIR.AnswerContract, ctx.ExploreLanePlan)
	if !policy.HasRoute(types.RepoMapNavigationRouteRelationMap) && !policy.HasRoute(types.RepoMapNavigationRouteCallPath) {
		return ""
	}
	sources := grepProductionSourceCandidates(production, params.FilesOnly, 3)
	if len(sources) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("relation_navigation_hint=grep result was broad/compacted and this typed request has relation/call-flow shape; consider `repo_map(view=\"relation_map\", sources=[")
	b.WriteString(quoteStringListForToolHint(sources))
	b.WriteString("]")
	if kinds := relationKindCandidatesForToolHint(policy, 4); len(kinds) > 0 {
		b.WriteString(", relation_kinds=[")
		b.WriteString(quoteStringListForToolHint(kinds))
		b.WriteString("]")
	}
	b.WriteString(")` before reading many files. This is only a navigation hint: if grep already pinpointed the exact file/range, go straight to `read_file`; if `relation_map` is empty, that is not absence proof and targeted grep/read_file remains valid.\n")
	return b.String()
}

func grepParamsTargetRuntimeArtifactFile(ctx *types.BusContext, params grepToolParams) bool {
	explicit, ok := explicitGrepPath(ctx, params.Path)
	if !ok || !explicit.isFile {
		return false
	}
	return grepPathLooksLikeRuntimeArtifact(explicit.path)
}

func grepPathLooksLikeRuntimeArtifact(p string) bool {
	if types.LooksLikeRuntimeArtifactPath(p) {
		return true
	}
	normalized := strings.ToLower(filepath.ToSlash(strings.TrimSpace(p)))
	base := path.Base(normalized)
	if filepath.Ext(base) == ".txt" {
		return strings.Contains(base, "log") || strings.Contains(base, "trace") || strings.Contains(base, "perf")
	}
	return strings.Contains(base, "systrace") || strings.Contains(base, "perfetto") || strings.Contains(base, "perftrace")
}

func LooksLikeRuntimeArtifactPath(p string) bool {
	return grepPathLooksLikeRuntimeArtifact(p)
}

func grepProductionSourceCandidates(production []string, filesOnly bool, limit int) []string {
	if limit <= 0 {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]string, 0, limit)
	for _, line := range production {
		file, ok := grepOutputLinePath(line, filesOnly)
		if !ok {
			continue
		}
		file = strings.TrimSpace(filepath.ToSlash(file))
		if file == "" || strings.HasPrefix(file, "[") || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, file)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func relationKindCandidatesForToolHint(policy types.RepoMapNavigationPolicy, limit int) []string {
	if limit <= 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, step := range policy.Steps {
		if step.Route != types.RepoMapNavigationRouteRelationMap {
			continue
		}
		for _, kind := range step.RelationHint {
			s := strings.TrimSpace(string(kind))
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func quoteStringListForToolHint(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return strings.Join(quoted, ", ")
}

type grepRelevanceTier int

const (
	grepRelevanceExplicitFile grepRelevanceTier = iota
	grepRelevanceRequiredFile
	grepRelevanceReadOrEvidenceFile
	grepRelevancePhase1RankedFile
	grepRelevanceExplicitScope
	grepRelevanceProduction
	grepRelevanceAuxiliary
)

const grepRelevanceNoOrder = int(^uint(0) >> 1)

type grepRelevanceRank struct {
	tier  grepRelevanceTier
	order int
}

type grepRelevanceIndex struct {
	paths          map[string]grepRelevanceRank
	explicitScopes []string
	nextOrder      int
}

type grepOutputEntry struct {
	raw     string
	path    string
	role    types.SourcePathRole
	rank    grepRelevanceRank
	ordinal int
}

type grepRelevanceStats struct {
	explicitFile  int
	required      int
	readEvidence  int
	phase1Ranked  int
	explicitScope int
	production    int
	auxiliary     int
}

func partitionGrepOutputByRelevance(ctx *types.BusContext, params grepToolParams, output string) (production, auxiliary, other []string, stats string) {
	idx := buildGrepRelevanceIndex(ctx, params)
	var productionEntries []grepOutputEntry
	var auxiliaryEntries []grepOutputEntry
	var stat grepRelevanceStats
	ordinal := 0
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if relPath, ok := grepOutputLinePath(line, params.FilesOnly); ok {
			path := canonicalGrepResultPath(ctx, relPath)
			role := types.ClassifySourcePathRole(path)
			rank := idx.rank(path, role)
			entry := grepOutputEntry{raw: raw, path: path, role: role, rank: rank, ordinal: ordinal}
			ordinal++
			addGrepRelevanceStat(&stat, rank)
			if types.SourcePathRoleIsAuxiliary(role) {
				auxiliaryEntries = append(auxiliaryEntries, entry)
			} else {
				productionEntries = append(productionEntries, entry)
			}
			continue
		}
		other = append(other, raw)
	}
	sortGrepEntries(productionEntries)
	sortGrepEntries(auxiliaryEntries)
	production = grepEntryLines(productionEntries)
	auxiliary = grepEntryLines(auxiliaryEntries)
	return production, auxiliary, other, stat.String()
}

func partitionGrepOutputByPathRole(ctx *types.BusContext, filesOnly bool, output string) (production, auxiliary, other []string) {
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
		other = append(other, raw)
	}
	return production, auxiliary, other
}

func buildGrepRelevanceIndex(ctx *types.BusContext, params grepToolParams) grepRelevanceIndex {
	idx := grepRelevanceIndex{paths: make(map[string]grepRelevanceRank)}
	if ctx == nil {
		return idx
	}
	if explicit, ok := explicitGrepPath(ctx, params.Path); ok {
		if explicit.isFile {
			idx.addPath(ctx, explicit.path, grepRelevanceExplicitFile)
		} else if explicit.path != "" {
			idx.explicitScopes = append(idx.explicitScopes, explicit.path)
		}
	}
	if ctx.AnalysisIR != nil {
		idx.addPaths(ctx, ctx.AnalysisIR.EvidencePlan.RequiredFiles, grepRelevanceRequiredFile)
	}
	if ctx.Mutable != nil {
		idx.addPaths(ctx, ctx.Mutable.ExactContextRequiredFiles(), grepRelevanceRequiredFile)
		if ranked := ctx.Mutable.Phase1Ranking(); len(ranked) > 0 {
			for _, item := range ranked {
				idx.addPath(ctx, item.Path, grepRelevancePhase1RankedFile)
			}
		}
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			idx.addPaths(ctx, ta.ReadFiles, grepRelevanceReadOrEvidenceFile)
			idx.addEvidenceSources(ctx, ta.EvidenceItems)
			idx.addReadFilesFromToolResults(ctx, ta.ToolResults)
		}
		idx.addEvidenceSources(ctx, ctx.Mutable.EmittedEvidence())
		idx.addReadFilesFromToolResults(ctx, ctx.Mutable.DispatchToolResults())
	}
	idx.addEvidenceSources(ctx, ctx.EvidenceItems)
	idx.addEvidenceSources(ctx, types.AnswerChainItems(ctx.AnswerChains))
	idx.addReadFilesFromToolResults(ctx, ctx.ToolResults)
	return idx
}

type explicitGrepPathResult struct {
	path   string
	isFile bool
}

func explicitGrepPath(ctx *types.BusContext, raw string) (explicitGrepPathResult, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "." || raw == "./" {
		return explicitGrepPathResult{}, false
	}
	canon := canonicalGrepResultPath(ctx, raw)
	if canon == "" || canon == "." || canon == "/" {
		return explicitGrepPathResult{}, false
	}
	res := explicitGrepPathResult{path: canon}
	resolved := resolveToolPath(ctx, raw)
	if resolved != "" {
		if info, err := os.Stat(resolved); err == nil {
			res.isFile = !info.IsDir()
			return res, true
		}
	}
	base := path.Base(canon)
	if base != "." && base != "/" && strings.Contains(base, ".") && !strings.HasSuffix(canon, "/") {
		res.isFile = true
	}
	return res, true
}

func (idx *grepRelevanceIndex) addPaths(ctx *types.BusContext, files []string, tier grepRelevanceTier) {
	for _, file := range files {
		idx.addPath(ctx, file, tier)
	}
}

func (idx *grepRelevanceIndex) addEvidenceSources(ctx *types.BusContext, items []types.EvidenceItem) {
	for _, item := range items {
		idx.addPath(ctx, item.Source, grepRelevanceReadOrEvidenceFile)
	}
}

func (idx *grepRelevanceIndex) addReadFilesFromToolResults(ctx *types.BusContext, results []types.ToolResult) {
	for _, result := range results {
		if result.ToolName != "read_file" || strings.TrimSpace(result.Summary) == "" {
			continue
		}
		if file, _, _, ok := ground.ParseReadFileBanner(result.Summary); ok {
			idx.addPath(ctx, file, grepRelevanceReadOrEvidenceFile)
		}
	}
}

func (idx *grepRelevanceIndex) addPath(ctx *types.BusContext, raw string, tier grepRelevanceTier) {
	if idx == nil {
		return
	}
	path := canonicalGrepResultPath(ctx, raw)
	if path == "" {
		return
	}
	if idx.paths == nil {
		idx.paths = make(map[string]grepRelevanceRank)
	}
	rank := grepRelevanceRank{tier: tier, order: idx.nextOrder}
	idx.nextOrder++
	if prev, ok := idx.paths[path]; ok {
		if prev.tier < rank.tier || (prev.tier == rank.tier && prev.order <= rank.order) {
			return
		}
	}
	idx.paths[path] = rank
}

func (idx grepRelevanceIndex) rank(path string, role types.SourcePathRole) grepRelevanceRank {
	if idx.paths != nil {
		if rank, ok := idx.paths[path]; ok {
			return rank
		}
	}
	for _, scope := range idx.explicitScopes {
		if grepPathWithinScope(path, scope) {
			return grepRelevanceRank{tier: grepRelevanceExplicitScope, order: grepRelevanceNoOrder}
		}
	}
	if types.SourcePathRoleIsAuxiliary(role) {
		return grepRelevanceRank{tier: grepRelevanceAuxiliary, order: grepRelevanceNoOrder}
	}
	return grepRelevanceRank{tier: grepRelevanceProduction, order: grepRelevanceNoOrder}
}

func sortGrepEntries(entries []grepOutputEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i].rank
		right := entries[j].rank
		if left.tier != right.tier {
			return left.tier < right.tier
		}
		if left.order != right.order {
			return left.order < right.order
		}
		return false
	})
}

func grepEntryLines(entries []grepOutputEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.raw)
	}
	return out
}

func addGrepRelevanceStat(stat *grepRelevanceStats, rank grepRelevanceRank) {
	if stat == nil {
		return
	}
	switch rank.tier {
	case grepRelevanceExplicitFile:
		stat.explicitFile++
	case grepRelevanceRequiredFile:
		stat.required++
	case grepRelevanceReadOrEvidenceFile:
		stat.readEvidence++
	case grepRelevancePhase1RankedFile:
		stat.phase1Ranked++
	case grepRelevanceExplicitScope:
		stat.explicitScope++
	case grepRelevanceAuxiliary:
		stat.auxiliary++
	default:
		stat.production++
	}
}

func (s grepRelevanceStats) String() string {
	var parts []string
	if s.explicitFile > 0 {
		parts = append(parts, fmt.Sprintf("explicit_file=%d", s.explicitFile))
	}
	if s.required > 0 {
		parts = append(parts, fmt.Sprintf("required=%d", s.required))
	}
	if s.readEvidence > 0 {
		parts = append(parts, fmt.Sprintf("read_or_evidence=%d", s.readEvidence))
	}
	if s.phase1Ranked > 0 {
		parts = append(parts, fmt.Sprintf("phase1_ranked=%d", s.phase1Ranked))
	}
	if s.explicitScope > 0 {
		parts = append(parts, fmt.Sprintf("explicit_scope=%d", s.explicitScope))
	}
	if s.production > 0 {
		parts = append(parts, fmt.Sprintf("production_rest=%d", s.production))
	}
	if s.auxiliary > 0 {
		parts = append(parts, fmt.Sprintf("auxiliary_rest=%d", s.auxiliary))
	}
	return strings.Join(parts, " ")
}

func canonicalGrepResultPath(ctx *types.BusContext, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	return ground.CanonicalRepoRelative(raw, repoRoot)
}

func grepPathWithinScope(filePath, scope string) bool {
	filePath = strings.Trim(strings.TrimSpace(filePath), "/")
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if filePath == "" || scope == "" || scope == "." {
		return false
	}
	return filePath == scope || strings.HasPrefix(filePath, scope+"/")
}

func writeCappedGrepSection(b *strings.Builder, header string, lines []string, cap int, emptyLine string) {
	if len(lines) == 0 && emptyLine == "" {
		return
	}
	b.WriteString(header)
	b.WriteByte('\n')
	if len(lines) == 0 {
		b.WriteString(emptyLine)
		b.WriteByte('\n')
		return
	}
	limit := cap
	if limit > len(lines) {
		limit = len(lines)
	}
	for _, line := range lines[:limit] {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if omitted := len(lines) - limit; omitted > 0 {
		fmt.Fprintf(b, "... omitted %d additional entries from this section; see full_raw_saved above.\n", omitted)
	}
}

func grepSkippedLargeFilePathHint(paths []string, skippedTotal int) string {
	if len(paths) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		quoted = append(quoted, fmt.Sprintf("%q", path))
	}
	if len(quoted) == 0 {
		return ""
	}
	suffix := ""
	if skippedTotal > len(quoted) {
		suffix = fmt.Sprintf(", ... +%d more", skippedTotal-len(quoted))
	}
	return " Skipped large file path(s): " + strings.Join(quoted, ", ") + suffix + "."
}

func grepSkippedLargeFilesNoMatchBody(paths []string, skippedTotal int) string {
	var b strings.Builder
	b.WriteString("no matches found in scanned files\n")
	fmt.Fprintf(&b, "searched_subset_no_matches=true skipped_large_files=%d\n", skippedTotal)
	if hint := grepSkippedLargeFilePathHint(paths, skippedTotal); hint != "" {
		b.WriteString(strings.TrimSpace(hint))
		b.WriteByte('\n')
	}
	if candidates := grepSkippedLargeCandidatesValue(paths, skippedTotal); candidates != "" {
		b.WriteString("skipped_large_candidates=")
		b.WriteString(candidates)
		b.WriteByte('\n')
	}
	b.WriteString("directory_scan_safety_skip=the skipped large file candidates were not searched in this directory scan; this is not absence proof and does not mean grep cannot search those files.\n")
	if nextPath := firstSkippedLargeRuntimeArtifactPath(paths); nextPath != "" {
		fmt.Fprintf(&b, "single_file_grep_supported=true next_call=grep(path=%q, pattern=\"<one exact timestamp/thread/event literal>\", files_only=false, context_lines=0)\n", nextPath)
		b.WriteString("runtime_artifact_recovery=do not start read_file at the file head; first run explicit single-file grep on the skipped artifact, then read_file around returned line numbers for evidence.\n")
	} else if nextPath := firstNonEmptyString(paths); nextPath != "" {
		fmt.Fprintf(&b, "single_file_grep_supported=true next_call=grep(path=%q, pattern=\"<one exact target literal>\", files_only=false, context_lines=0)\n", nextPath)
		b.WriteString("large_file_recovery=if a skipped file is the target, re-run grep with path set to that exact file before broadening or paging from the beginning.\n")
	} else {
		b.WriteString("large_file_recovery=if the target is a specific large file, re-run grep with path set to that exact file before broadening or paging from the beginning.\n")
	}
	return b.String()
}

func grepSkippedLargeCandidatesValue(paths []string, skippedTotal int) string {
	if len(paths) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(paths)+1)
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		quoted = append(quoted, fmt.Sprintf("%q", path))
	}
	if len(quoted) == 0 {
		return ""
	}
	if skippedTotal > len(quoted) {
		quoted = append(quoted, fmt.Sprintf("+%d_more", skippedTotal-len(quoted)))
	}
	return strings.Join(quoted, ",")
}

func firstSkippedLargeRuntimeArtifactPath(paths []string) string {
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p != "" && grepPathLooksLikeRuntimeArtifact(p) {
			return p
		}
	}
	return ""
}

func firstNonEmptyString(values []string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func grepLineWindowHint(lines []string, filesOnly bool) string {
	return renderGrepLineWindowHints(collectGrepLineWindows(lines, filesOnly, grepLineWindowHintMax))
}

func collectGrepLineWindows(lines []string, filesOnly bool, maxWindows int) []grepLineWindow {
	if filesOnly {
		return nil
	}
	if maxWindows <= 0 {
		maxWindows = grepLineWindowHintMax
	}
	var windows []grepLineWindow
	for _, line := range lines {
		windows = appendGrepLineWindow(windows, line, true, maxWindows)
	}
	if len(windows) > 0 {
		return windows
	}
	for _, line := range lines {
		windows = appendGrepLineWindow(windows, line, false, maxWindows)
	}
	return windows
}

func appendGrepLineWindow(windows []grepLineWindow, line string, requireMatch bool, maxWindows int) []grepLineWindow {
	path, lineNo, match, ok := parseGrepOutputLineLocation(line)
	if !ok || lineNo <= 0 || strings.TrimSpace(path) == "" {
		return windows
	}
	if requireMatch && !match {
		return windows
	}
	start := lineNo - grepLineWindowHalfSpan
	if start < 1 {
		start = 1
	}
	end := lineNo + grepLineWindowHalfSpan
	for i := range windows {
		if windows[i].Path != path || start > windows[i].Last+grepLineWindowHalfSpan {
			continue
		}
		if start < windows[i].First {
			windows[i].First = start
		}
		if end > windows[i].Last {
			windows[i].Last = end
		}
		windows[i].Count++
		return windows
	}
	if maxWindows > 0 && len(windows) >= maxWindows {
		return windows
	}
	return append(windows, grepLineWindow{Path: path, Line: lineNo, First: start, Last: end, Count: 1})
}

func renderGrepLineWindowHints(windows []grepLineWindow) string {
	if len(windows) == 0 {
		return ""
	}
	first := windows[0]
	lineNo := first.Line
	if lineNo <= 0 {
		lineNo = first.First
	}
	start := lineNo - grepLineWindowHalfSpan
	if start < 1 {
		start = 1
	}
	end := lineNo + grepLineWindowHalfSpan
	limit := end - start + 1
	offset := start - 1
	var b strings.Builder
	fmt.Fprintf(&b, "line_window_hint=first returned match is %s:%d; next use `read_file` with path=%q line_offset=%d limit=%d, or re-run grep on this same file with line_start=%d line_end=%d.\n",
		first.Path, lineNo, first.Path, offset, limit, start, end)
	if len(windows) > 1 {
		var parts []string
		for _, w := range windows {
			parts = append(parts, fmt.Sprintf("%s:%d-%d(matches=%d)", w.Path, w.First, w.Last, w.Count))
		}
		fmt.Fprintf(&b, "line_windows=%s\n", strings.Join(parts, "; "))
	}
	return b.String()
}

func firstGrepOutputLineLocation(lines []string, requireMatch bool) (string, int, bool) {
	for _, line := range lines {
		path, lineNo, match, ok := parseGrepOutputLineLocation(line)
		if !ok {
			continue
		}
		if requireMatch && !match {
			continue
		}
		return path, lineNo, true
	}
	return "", 0, false
}

func parseGrepOutputLineLocation(line string) (path string, lineNo int, match bool, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || line == "--" || strings.HasPrefix(line, "[") {
		return "", 0, false, false
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
		if digitEnd == 0 || digitEnd >= len(rest) {
			continue
		}
		if rest[digitEnd] != ':' && rest[digitEnd] != '-' {
			continue
		}
		if grepLineLocationLooksLikePathContinuation(rest[digitEnd+1:]) {
			continue
		}
		path = strings.TrimSpace(line[:i])
		if path == "" {
			continue
		}
		n, err := strconv.Atoi(rest[:digitEnd])
		if err != nil || n <= 0 {
			continue
		}
		return path, n, r == ':' && rest[digitEnd] == ':', true
	}
	return "", 0, false, false
}

func grepLineLocationLooksLikePathContinuation(afterLineSeparator string) bool {
	token := strings.TrimLeft(afterLineSeparator, " \t")
	if token == "" {
		return false
	}
	for i, r := range token {
		if r == ' ' || r == '\t' {
			token = token[:i]
			break
		}
	}
	return strings.ContainsAny(token, `/\`)
}

func annotateGrepOutputByRelevance(ctx *types.BusContext, params grepToolParams, output string) string {
	if ctx == nil || strings.TrimSpace(output) == "" {
		return output
	}
	production, auxiliary, passthrough, _ := partitionGrepOutputByRelevance(ctx, params, output)
	if len(production) == 0 && len(auxiliary) == 0 {
		return output
	}
	if len(auxiliary) == 0 && len(passthrough) == 0 && grepLinesEqual(production, grepNonEmptyLines(output)) {
		return output
	}
	return renderGrepPathRoleSections(ctx, production, auxiliary, passthrough)
}

func renderGrepPathRoleSections(ctx *types.BusContext, production, auxiliary, passthrough []string) string {
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

func grepNonEmptyLines(output string) []string {
	var lines []string
	for _, raw := range strings.Split(output, "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		lines = append(lines, raw)
	}
	return lines
}

func grepLinesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
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
	return annotateGrepOutputByRelevance(ctx, grepToolParams{FilesOnly: filesOnly}, output)
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

// fileTypeToGlobs maps a language/file type name to glob patterns shared by
// every search backend. Do not pass repo-map-only languages straight to
// `rg --type`: local ripgrep builds do not know ArkTS/Cangjie and would reject
// otherwise valid searches before the model gets any useful evidence.
// Unknown types fall back to a best-effort "*.type" glob.
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
// returns the whole file with an override banner. Any non-zero
// lineOffset is a strong signal of deliberate paging and is always honored —
// only the "first read, lazy default limit" shape is suppressed.
//
// Rationale: LLMs routinely emit line_offset=0+limit=20/50/100 from
// their training distribution, not because they know the file layout.
// On a 66-line source file, limit=20 silently hides the answer past
// line 20. A re-read that deliberately wants a slice picks a non-zero
// line_offset (see the paging loop around the banner).
//
// Default 100 catches the common lazy values (10/20/50/100) without
// forcing back the whole file when the LLM picks an odd size like
// 150 or 200 (almost always a deliberate keyhole read). Set to 0 to
// disable the override entirely — line_offset/limit is then honored on
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
	Path       string   `json:"path"`
	LineOffset *FlexInt `json:"line_offset,omitempty"`
	Offset     FlexInt  `json:"offset,omitempty"` // legacy backend-only alias for line_offset
	Limit      FlexInt  `json:"limit,omitempty"`
}

func (t *ReadFile) Name() string { return "read_file" }
func (t *ReadFile) Description() string {
	return "Read file contents. Use line_offset/limit to page files too large to inline (>32KB) and to read targeted line windows. line_offset is a zero-based line coordinate, not a byte or character offset: line_offset=100 starts at source line 101. Pass any non-zero line_offset (for example line_offset=100, limit=30) for a deliberate keyhole read and the slice is honored exactly. On files that fit inline, line_offset=0 combined with a small limit (default <= 100 lines, configurable via readfile_small_limit_threshold) is treated as a lazy default and expanded to the whole file, because slicing a small file silently hides the answer past the cutoff; the override banner reports this when it fires. The first line of the result is a banner shaped `[path: showing lines X-Y of Z total]` telling you which slice you got; every subsequent line is prefixed with its absolute 1-based line number followed by `│`. When citing a file location in your investigation notes, use only numbers taken verbatim from the gutter of a line you actually read — never estimate or interpolate line numbers between two lines you saw."
}

func (t *ReadFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":        {"type": "string",  "description": "Path to the file to read"},
    "line_offset": {"type": "integer", "description": "Preferred starting zero-based LINE offset. line_offset=100 starts at source line 101. This is not a byte or character offset."},
    "limit":       {"type": "integer", "description": "Maximum number of lines to return (optional)"}
  },
  "required": ["path"]
}`)
}

func readFileDecodeParameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":        {"type": "string",  "description": "Path to the file to read"},
    "line_offset": {"type": "integer", "description": "Preferred starting zero-based LINE offset. line_offset=100 starts at source line 101. This is not a byte or character offset."},
    "offset":      {"type": "integer", "description": "Deprecated backend-only alias for line_offset. Kept for compatibility; model-facing schema must use line_offset."},
    "limit":       {"type": "integer", "description": "Maximum number of lines to return (optional)"}
  },
  "required": ["path"]
}`)
}

func decodeReadFileParams(raw json.RawMessage) (readFileParams, bool, *types.ToolResult, error) {
	var p readFileParams
	normalized, decodeFailure, err := decodeStrictToolParams("read_file", raw, readFileDecodeParameters(), &p, nil)
	if err != nil {
		return p, false, decodeFailure, err
	}
	if p.LineOffset != nil {
		p.Offset = *p.LineOffset
		return p, false, nil, nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(normalized, &probe); err == nil {
		if _, ok := probe["offset"]; ok {
			return p, true, nil, nil
		}
	}
	return p, false, nil, nil
}

// Denial lookups for refusal rendering live on TypedDenialSet itself
// (FirstPathDenial / FirstSymbolDenial) — one implementation of the
// match rules, one lock acquisition, shared with the repo_map gate.

func (t *ReadFile) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	p, usedLegacyOffset, decodeFailure, err := decodeReadFileParams(params)
	if err != nil {
		return *decodeFailure, err
	}
	lineOffset := p.Offset.Int()
	limit := p.Limit.Int()
	if lineOffset < 0 || limit < 0 {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "invalid params: line_offset and limit must be non-negative line counts; line_offset is zero-based and line-based, not byte-based", Timestamp: time.Now()}, nil
	}

	// L1 negative-knowledge gate (R3 second-axis enforcement):
	// when an input-side validator has already determined the named
	// path is not present / not corroborated in the current repo,
	// refuse the read with the per-class user-facing reason from
	// HumanRefusalReason — no internal pipeline terminology, no
	// fixture-specific examples, generic enough to apply to any
	// attached log / trace / pasted artifact.
	if ctx != nil {
		if denial, denied := ctx.TypedDenials.FirstPathDenial(p.Path); denied {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   denial.HumanRefusalReason("read"),
				Timestamp: time.Now(),
			}, nil
		}
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
		hint := ""
		if errors.Is(err, fs.ErrNotExist) {
			hint = sourceInventoryReadFilePathMissHint(ctx, p.Path)
		}
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: fmt.Sprintf("read failed: %v%s", err, hint), Timestamp: time.Now()}, nil
	}

	content := string(data)
	allLines := strings.Split(content, "\n")
	totalLines := len(allLines)

	// Slice vs. full-file policy:
	//
	//   - No line_offset/limit passed → return the whole file.
	//   - line_offset > 0 → deliberate paging. Honor the slice verbatim,
	//     regardless of file size. Any non-zero line_offset is a strong
	//     signal that the LLM knows what section it wants.
	//   - Large file (bytes > MaxInlineBytes) → honor the slice, the
	//     whole file cannot fit inline anyway.
	//   - Small file with line_offset==0 and 0 < Limit <=
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
	if lineOffset > 0 || limit > 0 {
		isLazyDefault := lineOffset == 0 &&
			limit > 0 &&
			ReadFileSmallLimitThreshold > 0 &&
			limit <= ReadFileSmallLimitThreshold &&
			limit < totalLines &&
			len(data) <= MaxInlineBytes
		if isLazyDefault {
			overrode = true
		} else {
			sliceStart = lineOffset
			sliceEnd = totalLines
			if limit > 0 && sliceStart+limit < sliceEnd {
				sliceEnd = sliceStart + limit
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
				"read_file empty range: line_offset=%d starts after the last readable line of %s (%d total line(s)); line_offset is a zero-based LINE offset, not a byte or character offset, so the last valid line_offset is %d. Retry with line_offset<=%d, or omit line_offset to read from the beginning.",
				lineOffset, p.Path, totalLines, lastOffset, lastOffset),
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
		banner = fmt.Sprintf("[%s: showing all %d lines (%d bytes); limit=%d expanded to full file (inline-sized; pass line_offset>0 for explicit paging)]\n",
			p.Path, totalLines, len(data), limit)
	} else {
		banner = fmt.Sprintf("[%s: showing lines %d-%d of %d total]\n", p.Path, sliceStart+1, sliceEnd, totalLines)
	}
	if usedLegacyOffset {
		banner += "[read_file notice: legacy offset was accepted and interpreted as zero-based line_offset, not as a byte or character offset]\n"
	} else if lineOffset > 0 || p.LineOffset != nil {
		banner += "[read_file coordinates: line_offset is a zero-based line offset, not a byte or character offset; gutter line numbers are 1-based]\n"
	}
	content = banner + content

	summary, ref := StoreBlobHeadOnly(ctx, t.Name(), content)
	now := time.Now()
	return types.ToolResult{
		ToolName:     t.Name(),
		Success:      true,
		Summary:      summary,
		RawRef:       ref,
		Observations: readFileTypedObservations(ctx, p.Path, fsPath, ref, sliceStart+1, sliceEnd, totalLines, now),
		Timestamp:    now,
	}, nil
}

func readFileTypedObservations(ctx *types.BusContext, requestedPath, fsPath, rawRef string, lineStart, lineEnd, totalLines int, observedAt time.Time) []types.ObservationRecord {
	sourcePath := strings.TrimSpace(requestedPath)
	if ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		if rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, fsPath); ok && rel != "" {
			sourcePath = rel
		}
	}
	sourcePath = strings.TrimSpace(strings.ReplaceAll(sourcePath, "\\", "/"))
	if sourcePath == "" || strings.HasPrefix(sourcePath, "/") || sourcePath == "." || strings.HasPrefix(sourcePath, "../") {
		return nil
	}
	if lineStart <= 0 {
		lineStart = 1
	}
	if lineEnd < lineStart {
		lineEnd = lineStart
	}
	scope := types.ScopeLineRange
	switch {
	case totalLines > 0 && lineStart == 1 && lineEnd >= totalLines:
		scope = types.ScopeFile
	case lineStart == lineEnd:
		scope = types.ScopeLine
	}
	role := types.AnswerAggregateRoleSupportingCoverage
	origin := types.AnswerEvidenceOriginCurrentSource
	return []types.ObservationRecord{{
		ID:              fmt.Sprintf("read_file:%s:%d:%d", sourcePath, lineStart, lineEnd),
		Origin:          origin,
		Producer:        "read_file",
		Role:            role,
		GroundingPolicy: types.AnswerClaimBindingGroundingPolicy(origin, role),
		SourceRef: types.ObservationSourceRef{
			Kind:   types.ObservationSourceCurrentSource,
			Path:   sourcePath,
			RawRef: strings.TrimSpace(rawRef),
		},
		Span: types.ObservationSpan{
			LineStart: lineStart,
			LineEnd:   lineEnd,
		},
		EvidenceKind:    types.EvidenceDirect,
		EvidenceScope:   scope,
		GroundingStatus: types.GroundingGrounded,
		Summary:         "read_file observed current source bytes",
		ObservedAt:      observedAt.Format("2006-01-02T15:04:05Z07:00"),
		Confidence:      1,
	}}
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
	return textfmt.LineGutter(lines, startLineNo)
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
	Path             string `json:"path"`
	Recursive        bool   `json:"recursive,omitempty"`
	Include          string `json:"include,omitempty"`
	FileType         string `json:"file_type,omitempty"`
	IncludeAuxiliary bool   `json:"include_auxiliary,omitempty"`
}

func (t *ListFiles) Name() string { return "list_files" }
func (t *ListFiles) Description() string {
	return "List files in a directory. Use this for navigation and discovery only. Use include or file_type for file-path/glob discovery (for example file_type=\"arkts\" for *.ets, \"cangjie\" for *.cj, \"cpp\" for C++). Use include_auxiliary=true only when the typed scope is all/auxiliary or a production scan was empty and repo-owned auxiliary/corpus trees need to be inspected. Do NOT count or sort by eye; for exact numeric counts, use a deterministic command or the tool-reported exact list size."
}

func (t *ListFiles) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":              {"type": "string",  "description": "Directory path to list"},
    "recursive":         {"type": "boolean", "description": "List recursively (default false)"},
    "include":           {"type": "string",  "description": "Optional file glob filter, e.g. *.go or */Index.ets. Filters path names, unlike grep which searches file contents."},
    "file_type":         {"type": "string",  "description": "Optional language/file type filter: go, py, js, ts, java, kotlin, arkts, cangjie, rust, ruby, c, cpp, yaml, json, toml, markdown, config, etc. Reuses the same cross-language glob mapping as grep."},
    "include_auxiliary": {"type": "boolean", "description": "If true, re-include repo-owned auxiliary/corpus trees that broad navigation normally hides, while still keeping dependency/cache/VCS noise filtered. Use only when the typed answer scope includes auxiliary/all material or a production-only scan returned empty."}
  },
  "required": ["path"]
}`)
}

func (t *ListFiles) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p listFilesParams
	if _, decodeFailure, err := decodeStrictToolParams(t.Name(), params, t.Parameters(), &p, nil); err != nil {
		return *decodeFailure, err
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
	if !p.IncludeAuxiliary && listFilesShouldAutoIncludeAuxiliary(ctx, p) {
		p.IncludeAuxiliary = true
	}

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

	files, err := listFilesCollect(ctx, fsPath, displayPath, p)
	if err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: listFilesBanner(p) + err.Error(), Timestamp: time.Now()}, nil
	}
	if len(files) == 0 && listFilesShouldFallbackRecursive(ctx, p) {
		p.Recursive = true
		files, err = listFilesCollect(ctx, fsPath, displayPath, p)
		if err != nil {
			return types.ToolResult{ToolName: t.Name(), Success: false, Summary: listFilesBanner(p) + err.Error(), Timestamp: time.Now()}, nil
		}
	}
	if len(files) == 0 && !p.IncludeAuxiliary && listFilesShouldFallbackIncludeAuxiliary(ctx, p) {
		p.IncludeAuxiliary = true
		files, err = listFilesCollect(ctx, fsPath, displayPath, p)
		if err != nil {
			return types.ToolResult{ToolName: t.Name(), Success: false, Summary: listFilesBanner(p) + err.Error(), Timestamp: time.Now()}, nil
		}
	}

	banner := listFilesBanner(p)
	payload := banner + strings.Join(files, "\n")
	summary, ref := StoreBlob(ctx, t.Name(), payload)
	return types.ToolResult{
		ToolName: t.Name(),
		Success:  true,
		Summary:  summary,
		RawRef:   ref,
		Refinement: mergeToolRefinementHints(
			configuredSearchExcludeRootsRefinement(ctx, fsPath, t.Name()),
			listFilesBroadResultRefinement(ctx, p, files, len(payload)),
		),
		Timestamp: time.Now(),
	}, nil
}

func listFilesBanner(p listFilesParams) string {
	recursiveStr := ""
	if p.Recursive {
		recursiveStr = "true"
	}
	includeAuxiliaryStr := ""
	if p.IncludeAuxiliary {
		includeAuxiliaryStr = "true"
	}
	return kvBanner("list_files",
		"path", p.Path,
		"recursive", recursiveStr,
		"include", p.Include,
		"file_type", p.FileType,
		"include_auxiliary", includeAuxiliaryStr,
	)
}

func listFilesCollect(ctx *types.BusContext, fsPath, displayPath string, p listFilesParams) ([]string, error) {
	var files []string
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	dirFilter := NewSearchDirFilterWithOptions(repoRoot, fsPath, SearchDirFilterOptions{
		IncludeAuxiliarySource: p.IncludeAuxiliary,
	})
	filter := listFilesFilter{
		ctx:              ctx,
		dirFilter:        dirFilter,
		includeAuxiliary: p.IncludeAuxiliary,
		globs:            listFilesGlobs(p.Include, p.FileType),
	}

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
				if filter.shouldSkipDir(path, d.Name()) {
					return filepath.SkipDir
				}
				if IsWindowsReservedDevicePath(d.Name()) {
					return filepath.SkipDir
				}
			}
			if !d.IsDir() && IsWindowsReservedDevicePath(d.Name()) {
				return nil
			}
			if !d.IsDir() && !filter.matchesPath(path) {
				return nil
			}
			if d.IsDir() && len(filter.globs) > 0 {
				return nil
			}
			rel, relErr := filepath.Rel(fsPath, path)
			if relErr != nil || rel == "." {
				if len(filter.globs) > 0 && !d.IsDir() {
					return nil
				}
				files = append(files, displayPath)
			} else {
				files = append(files, filepath.Join(displayPath, rel))
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk failed: %v", err)
		}
	} else {
		entries, err := os.ReadDir(fsPath)
		if err != nil {
			return nil, fmt.Errorf("readdir failed: %v", err)
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
			entryPath := filepath.Join(fsPath, e.Name())
			if e.IsDir() {
				if filter.shouldSkipDir(entryPath, e.Name()) {
					continue
				}
			}
			if IsWindowsReservedDevicePath(e.Name()) {
				continue
			}
			if !e.IsDir() && !filter.matchesPath(entryPath) {
				continue
			}
			if e.IsDir() && len(filter.globs) > 0 {
				continue
			}
			files = append(files, filepath.Join(displayPath, e.Name()))
		}
	}
	return files, nil
}

func listFilesBroadResultRefinement(ctx *types.BusContext, p listFilesParams, files []string, payloadBytes int) *types.ToolRefinementHint {
	if len(files) <= grepGovernorFileEntryThreshold && payloadBytes <= grepGovernorByteThreshold {
		return nil
	}
	hint := types.ToolRefinementHint{
		ReasonCode:        "list_files_result_truncated",
		ResultTruncated:   true,
		PreferredNextTool: "list_files",
		PreferredParams: map[string]string{
			"recursive": strconv.FormatBool(p.Recursive),
		},
	}
	if pathValue := strings.TrimSpace(p.Path); pathValue != "" {
		hint.PreferredParams["path"] = pathValue
	}
	if fileType := strings.TrimSpace(p.FileType); fileType != "" {
		hint.PreferredParams["file_type"] = fileType
	}
	if include := strings.TrimSpace(p.Include); include != "" {
		hint.PreferredParams["include"] = include
	}
	if p.IncludeAuxiliary {
		hint.PreferredParams["include_auxiliary"] = "true"
	}
	if nextPath := listFilesPreferredNarrowPath(ctx, files); nextPath != "" {
		hint.PreferredParams["path"] = nextPath
	}
	if strings.TrimSpace(p.FileType) == "" && strings.TrimSpace(p.Include) == "" {
		hint.RequiredFields = []string{"include", "file_type"}
		if _, ok := hint.PreferredParams["recursive"]; !ok {
			hint.PreferredParams["recursive"] = "true"
		}
	}
	out := promoteSourceToolRefinementToRepoMap(ctx, hint)
	if out.Empty() {
		return nil
	}
	return &out
}

func listFilesPreferredNarrowPath(ctx *types.BusContext, files []string) string {
	dirFallback := ""
	for _, candidate := range listFilesPreferredPathCandidates(files) {
		if candidate == "" || candidate == "." {
			continue
		}
		fsPath := resolveToolPath(ctx, candidate)
		if info, err := os.Stat(fsPath); err == nil {
			if info.IsDir() {
				dirFallback = deeperListFilesPath(dirFallback, candidate)
				continue
			}
			if parent := path.Dir(strings.ReplaceAll(candidate, `\`, `/`)); parent != "" && parent != "." {
				return parent
			}
			continue
		}
		if parent := path.Dir(strings.ReplaceAll(candidate, `\`, `/`)); parent != "" && parent != "." {
			return parent
		}
	}
	return dirFallback
}

func deeperListFilesPath(current, candidate string) string {
	candidate = strings.Trim(strings.ReplaceAll(candidate, `\`, `/`), "/")
	if candidate == "" || candidate == "." {
		return current
	}
	current = strings.Trim(strings.ReplaceAll(current, `\`, `/`), "/")
	if current == "" || strings.Count(candidate, "/") > strings.Count(current, "/") {
		return candidate
	}
	return current
}

func listFilesPreferredPathCandidates(files []string) []string {
	production := make([]string, 0, len(files))
	auxiliary := make([]string, 0)
	other := make([]string, 0)
	for _, raw := range files {
		candidate := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
		if candidate == "" {
			continue
		}
		switch role := types.ClassifySourcePathRole(candidate); {
		case role == types.SourcePathRoleUnknown:
			other = append(other, candidate)
		case types.SourcePathRoleIsAuxiliary(role):
			auxiliary = append(auxiliary, candidate)
		default:
			production = append(production, candidate)
		}
	}
	out := make([]string, 0, len(files))
	out = append(out, production...)
	out = append(out, other...)
	out = append(out, auxiliary...)
	return out
}

func listFilesShouldAutoIncludeAuxiliary(ctx *types.BusContext, p listFilesParams) bool {
	if ctx == nil || ctx.AnalysisIR == nil || !p.Recursive {
		return false
	}
	if strings.TrimSpace(p.Include) == "" && strings.TrimSpace(p.FileType) == "" {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if SourceInventoryHasExplicitAuxiliaryExclusion(rm) {
		return false
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return true
	}
	if rm.SourceScopeProfile != nil && rm.SourceScopeProfile.AllowsAuxiliaryPrincipal() {
		return true
	}
	return types.IsTypedSourceEnumerationShape(rm)
}

func grepShouldAutoIncludeAuxiliary(ctx *types.BusContext, p grepToolParams) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if strings.TrimSpace(p.Include) == "" && strings.TrimSpace(p.FileType) == "" {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if SourceInventoryHasExplicitAuxiliaryExclusion(rm) {
		return false
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return true
	}
	if rm.SourceScopeProfile != nil && rm.SourceScopeProfile.AllowsAuxiliaryPrincipal() {
		return true
	}
	return types.IsTypedSourceEnumerationShape(rm)
}

func listFilesShouldFallbackRecursive(ctx *types.BusContext, p listFilesParams) bool {
	if p.Recursive {
		return false
	}
	if strings.TrimSpace(p.Include) == "" && strings.TrimSpace(p.FileType) == "" {
		return false
	}
	pathValue := strings.TrimSpace(p.Path)
	if pathValue == "" || pathValue == "." || pathValue == "./" {
		return true
	}
	if ctx != nil && ctx.RepoRoot != "" {
		if rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, resolveToolPath(ctx, p.Path)); ok {
			rel = strings.Trim(strings.ReplaceAll(rel, `\`, `/`), "/")
			return rel == "" || rel == "."
		}
	}
	return false
}

func listFilesShouldFallbackIncludeAuxiliary(ctx *types.BusContext, p listFilesParams) bool {
	if !p.Recursive {
		return false
	}
	if strings.TrimSpace(p.Include) == "" && strings.TrimSpace(p.FileType) == "" {
		return false
	}
	if ctx != nil && ctx.AnalysisIR != nil && SourceInventoryHasExplicitAuxiliaryExclusion(ctx.AnalysisIR.RequestModel) {
		return false
	}
	return true
}

type listFilesFilter struct {
	ctx              *types.BusContext
	dirFilter        SearchDirFilter
	includeAuxiliary bool
	globs            []string
}

func listFilesGlobs(include, fileType string) []string {
	var out []string
	for _, raw := range splitListFilesIncludeGlobs(include) {
		out = append(out, raw)
	}
	if strings.TrimSpace(fileType) != "" {
		for _, raw := range fileTypeToGlobs(fileType) {
			raw = strings.TrimSpace(raw)
			if raw != "" {
				out = append(out, raw)
			}
		}
	}
	return dedupStrings(out)
}

func splitListFilesIncludeGlobs(include string) []string {
	var out []string
	for _, raw := range strings.FieldsFunc(include, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\t'
	}) {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			out = append(out, raw)
		}
	}
	return out
}

func (f listFilesFilter) shouldSkipDir(pathValue, baseName string) bool {
	if IsWindowsReservedDevicePath(baseName) {
		return true
	}
	if f.ctx != nil && f.ctx.RepoRoot != "" {
		if rel, ok := repoRelativePathWithinRoot(f.ctx.RepoRoot, pathValue); ok {
			if f.includeAuxiliary && listFilesAuxiliaryDirAllowed(rel) {
				return false
			}
			return f.dirFilter.ExcludesRepoRelativePath(rel)
		}
	}
	if f.includeAuxiliary && listFilesAuxiliaryDirNameAllowed(baseName) {
		return false
	}
	return IsExcludedDirName(baseName)
}

func (f listFilesFilter) matchesPath(pathValue string) bool {
	if len(f.globs) == 0 {
		return true
	}
	display := strings.ReplaceAll(pathValue, `\`, `/`)
	base := path.Base(display)
	if f.ctx != nil && f.ctx.RepoRoot != "" {
		if rel, ok := repoRelativePathWithinRoot(f.ctx.RepoRoot, pathValue); ok {
			display = rel
			base = path.Base(rel)
		}
	}
	for _, glob := range f.globs {
		if listFilesGlobMatches(glob, base, display) {
			return true
		}
	}
	return false
}

func listFilesGlobMatches(glob, base, rel string) bool {
	glob = strings.TrimSpace(strings.ReplaceAll(glob, `\`, `/`))
	if glob == "" {
		return false
	}
	if matched, err := path.Match(glob, base); err == nil && matched {
		return true
	}
	if strings.Contains(glob, "/") {
		if matched, err := path.Match(glob, rel); err == nil && matched {
			return true
		}
	}
	return false
}

func listFilesAuxiliaryDirAllowed(rel string) bool {
	return searchAuxiliarySourceRelAllowed(rel) && !searchAuxiliarySourceHardExcludedRel(rel)
}

func listFilesAuxiliaryDirNameAllowed(name string) bool {
	return searchAuxiliarySourceRelAllowed(name) && !searchAuxiliarySourceHardExcludedRel(name)
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

func (t *GitDiff) Name() string { return "git_diff" }
func (t *GitDiff) Description() string {
	return "Show a working-tree/staged/range git diff safely. Use this for diff-only and diff+current-code questions before deciding whether current source must also be read. Diff hunks are VCS diff evidence, not current checkout file:line evidence; do not mirror old/deleted diff lines through emit_evidence."
}

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
	if _, decodeFailure, err := decodeStrictToolParams(t.Name(), params, t.Parameters(), &p, nil); err != nil {
		return *decodeFailure, err
	}
	if evalGitHistoryDisabled(ctx) && strings.TrimSpace(p.Ref) != "" {
		return evalGitHistoryDisabledToolResult(t.Name(), "ref/range diff"), nil
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
		"evidence_origin", string(types.AnswerEvidenceOriginVCSDiff),
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
	return "Show a commit/ref safely. Use this instead of shelling out to git show; no_patch gives metadata only, stat gives --stat plus exact_changed_paths, name_only gives changed paths, and the default includes the patch without using unsupported --no-stat. Copy exact paths from exact_changed_paths or name_only output; do not expand abbreviated --stat paths containing `...`."
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
	if _, decodeFailure, err := decodeStrictToolParams(t.Name(), params, t.Parameters(), &p, nil); err != nil {
		return *decodeFailure, err
	}
	if evalGitHistoryDisabled(ctx) {
		return evalGitHistoryDisabledToolResult(t.Name(), "commit/ref show"), nil
	}
	ref, refErr := normalizeGitRefParam(p.Ref, true)
	if refErr != "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: refErr, Timestamp: time.Now()}, nil
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
	bannerKV := []string{
		"repo_path", p.RepoPath,
		"ref", ref,
		"path", p.Path,
		"format", format,
		"no_patch", fmt.Sprintf("%t", p.NoPatch),
		"stat", fmt.Sprintf("%t", p.Stat),
		"name_only", fmt.Sprintf("%t", p.NameOnly),
		"evidence_origin", string(types.AnswerEvidenceOriginVCSMetadata),
	}
	if !p.NoPatch {
		bannerKV = append(bannerKV, "diff_origin", string(types.AnswerEvidenceOriginVCSDiff))
	}
	banner := kvBanner("git_show", bannerKV...)
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
	body := banner
	if p.Stat {
		body += gitExactChangedPathsSection(dir, []string{ref}, pathspec)
	}
	body += stdout.String()
	summary, refBlob := StoreBlob(ctx, t.Name(), body)
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
	Path        string `json:"path,omitempty"`
	Pathspec    string `json:"pathspec,omitempty"`
	Ref         string `json:"ref,omitempty"`
	Count       int    `json:"count,omitempty"`
	Format      string `json:"format,omitempty"`
	Stat        bool   `json:"stat,omitempty"`
	NameOnly    bool   `json:"name_only,omitempty"`
	MergesOnly  bool   `json:"merges_only,omitempty"`
	NoMerges    bool   `json:"no_merges,omitempty"`
	FirstParent bool   `json:"first_parent,omitempty"`
}

func (t *GitLog) Name() string { return "git_log" }
func (t *GitLog) Description() string {
	return "List recent git commits safely. Use this for latest/last-N history questions; for “最近 N 次提交做了什么/影响是什么” use count=N with format=medium and stat=true (or name_only=true for only changed paths). stat=true output includes an exact_changed_paths section; copy exact paths from that section or name_only output, and do not expand Git's abbreviated --stat paths that contain `...`. For “最近一次合入/merge” use merges_only=true, first_parent=true, count=1. Use ref to start from a branch/tag/commit. Then optionally git_show one commit for full patch detail. Git metadata is VCS provenance, not source file:line evidence; do not emit_evidence for commit hashes or subjects."
}

func (t *GitLog) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":      {"type": "string",  "description": "Optional repository working directory, resolved under the active repo root; omit for the active repo root. This is not a file pathspec."},
    "pathspec":  {"type": "string",  "description": "Optional repo-relative pathspec to limit commit history, e.g. internal/tool/"},
    "ref":        {"type": "string",  "description": "Optional git revision/ref to start from, e.g. HEAD, main, origin/main, a tag, or a commit hash. Omit for HEAD."},
    "count":      {"type": "integer", "description": "Number of commits to show (default 10, max 100)"},
    "format":     {"type": "string",  "description": "Log format: oneline, short, medium, full (default oneline). Use medium with stat=true for commit-impact summaries."},
    "stat":       {"type": "boolean", "description": "Include --stat changed-file summary for each commit; useful for recent-N feature/impact summaries."},
    "name_only":  {"type": "boolean", "description": "Include --name-only changed paths for each commit without patch text."},
    "merges_only":{"type": "boolean", "description": "Only show merge commits (--merges). Use with first_parent=true for recent integration/合入 questions."},
    "no_merges":  {"type": "boolean", "description": "Exclude merge commits (--no-merges). Mutually exclusive with merges_only."},
    "first_parent":{"type": "boolean", "description": "Follow only the first-parent history (--first-parent), useful for integration branch summaries and latest merge questions."}
  }
}`)
}

func (t *GitLog) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p gitLogParams
	if _, decodeFailure, err := decodeStrictToolParams(t.Name(), params, t.Parameters(), &p, nil); err != nil {
		return *decodeFailure, err
	}
	if evalGitHistoryDisabled(ctx) {
		return evalGitHistoryDisabledToolResult(t.Name(), "history log"), nil
	}

	count := p.Count
	if count <= 0 {
		count = 10
	}
	if count > 100 {
		count = 100
	}
	format := p.Format
	if format == "" {
		format = "oneline"
	}
	switch format {
	case "oneline", "short", "medium", "full":
	default:
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "invalid params: format must be oneline, short, medium, or full", Timestamp: time.Now()}, nil
	}
	ref, refErr := normalizeGitRefParam(p.Ref, false)
	if refErr != "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: refErr, Timestamp: time.Now()}, nil
	}
	if ref == "" {
		ref = "HEAD"
	}

	banner := kvBanner("git_log",
		"path", p.Path,
		"pathspec", p.Pathspec,
		"ref", ref,
		"count", fmt.Sprintf("%d", count),
		"format", format,
		"stat", fmt.Sprintf("%t", p.Stat),
		"name_only", fmt.Sprintf("%t", p.NameOnly),
		"merges_only", fmt.Sprintf("%t", p.MergesOnly),
		"no_merges", fmt.Sprintf("%t", p.NoMerges),
		"first_parent", fmt.Sprintf("%t", p.FirstParent),
		"evidence_origin", string(types.AnswerEvidenceOriginVCSMetadata),
	)
	if p.Stat && p.NameOnly {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + "invalid params: stat and name_only are mutually exclusive", Timestamp: time.Now()}, nil
	}
	if p.MergesOnly && p.NoMerges {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + "invalid params: merges_only and no_merges are mutually exclusive", Timestamp: time.Now()}, nil
	}

	// Same RepoRoot anchoring as GitDiff above.
	dir, pathErr := resolveRepoScopedToolDir(ctx, p.Path)
	if pathErr != "" {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + pathErr, Timestamp: time.Now()}, nil
	}
	pathspec := normalizeGitHistoryPathspec(p.Pathspec, dir)
	if path.IsAbs(pathspec) {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: banner + "invalid params: pathspec must be repo-relative or an absolute path under path", Timestamp: time.Now()}, nil
	}

	args := []string{"log", fmt.Sprintf("-n%d", count), fmt.Sprintf("--format=%s", format)}
	if p.FirstParent {
		args = append(args, "--first-parent")
	}
	if p.MergesOnly {
		args = append(args, "--merges")
	}
	if p.NoMerges {
		args = append(args, "--no-merges")
	}
	if p.Stat {
		args = append(args, "--stat")
	}
	if p.NameOnly {
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
			Summary:   banner + fmt.Sprintf("git log failed: %v: %s", err, stderr.String()),
			Timestamp: time.Now(),
		}, nil
	}

	body := banner
	if p.Stat {
		if commits, errSummary := gitLogCommitHashes(dir, ref, count, pathspec, p.FirstParent, p.MergesOnly, p.NoMerges); errSummary == "" {
			body += gitExactChangedPathsSection(dir, commits, pathspec)
		}
	}
	body += stdout.String()
	summary, ref := StoreBlob(ctx, t.Name(), body)
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
	return "Search a bounded git history window and count commits whose diff contains a fixed string. Use order=oldest for first-introduced / earliest-occurrence questions; order=recent is the default for latest / last-N questions. The result is VCS metadata/diff provenance, not current-source file:line evidence; carry it through aggregate_facts/reason unless you separately read current source."
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
	if _, decodeFailure, err := decodeStrictToolParams(t.Name(), params, t.Parameters(), &p, nil); err != nil {
		return *decodeFailure, err
	}
	if evalGitHistoryDisabled(ctx) {
		return evalGitHistoryDisabledToolResult(t.Name(), "history search"), nil
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
		"evidence_origin", string(types.AnswerEvidenceOriginVCSMetadata),
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

func normalizeGitRefParam(s string, required bool) (string, string) {
	ref := strings.TrimSpace(s)
	if ref == "" {
		if required {
			return "", "invalid params: ref is required"
		}
		return "", ""
	}
	if strings.HasPrefix(ref, "-") {
		return "", "invalid params: ref must not start with '-'"
	}
	if strings.ContainsAny(ref, " \t\r\n") {
		return "", "invalid params: ref must be a single git revision token"
	}
	for _, r := range ref {
		if unicode.IsControl(r) {
			return "", "invalid params: ref must not contain control characters"
		}
	}
	return ref, ""
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

const (
	gitExactChangedPathsMaxCommits = 20
	gitExactChangedPathsMaxPaths   = 24
)

func gitLogCommitHashes(dir, ref string, count int, pathspec string, firstParent, mergesOnly, noMerges bool) ([]string, string) {
	if count <= 0 {
		count = 10
	}
	if count > gitExactChangedPathsMaxCommits {
		count = gitExactChangedPathsMaxCommits
	}
	args := []string{"log", fmt.Sprintf("-n%d", count), "--format=%H"}
	if firstParent {
		args = append(args, "--first-parent")
	}
	if mergesOnly {
		args = append(args, "--merges")
	}
	if noMerges {
		args = append(args, "--no-merges")
	}
	if strings.TrimSpace(ref) == "" {
		ref = "HEAD"
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
		return nil, fmt.Sprintf("git log exact changed paths failed: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	var hashes []string
	for _, line := range strings.Split(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n") {
		hash := strings.TrimSpace(line)
		if hash == "" {
			continue
		}
		hashes = append(hashes, hash)
	}
	return hashes, ""
}

func gitExactChangedPathsSection(dir string, commits []string, pathspec string) string {
	if len(commits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("exact_changed_paths: copy these exact paths; do not expand abbreviated --stat paths containing `...`.\n")
	for i, hash := range commits {
		if i >= gitExactChangedPathsMaxCommits {
			fmt.Fprintf(&b, "exact_path_omitted commits=%d\n", len(commits)-i)
			break
		}
		paths, errSummary := gitChangedPathsForCommit(dir, hash, pathspec)
		if errSummary != "" {
			fmt.Fprintf(&b, "exact_path_unavailable commit=%s reason=%q\n", shortGitHash(hash), errSummary)
			continue
		}
		for j, changedPath := range paths {
			if j >= gitExactChangedPathsMaxPaths {
				fmt.Fprintf(&b, "exact_path_omitted commit=%s paths=%d\n", shortGitHash(hash), len(paths)-j)
				break
			}
			fmt.Fprintf(&b, "exact_path commit=%s path=%q\n", shortGitHash(hash), changedPath)
		}
	}
	b.WriteByte('\n')
	return b.String()
}

func gitChangedPathsForCommit(dir, hash, pathspec string) ([]string, string) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return nil, "empty commit"
	}
	args := []string{"show", "--format=", "--name-only", "--no-ext-diff", hash}
	if strings.TrimSpace(pathspec) != "" {
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
		return nil, fmt.Sprintf("git show %s name-only failed: %v: %s", shortGitHash(hash), err, strings.TrimSpace(stderr.String()))
	}
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n") {
		changedPath := strings.TrimSpace(line)
		if changedPath == "" || seen[changedPath] {
			continue
		}
		seen[changedPath] = true
		out = append(out, filepath.ToSlash(changedPath))
	}
	return out, ""
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
