package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Blob sizing knobs. These are package-level vars (not consts) so
// cmd/root.go can override them at startup from codrax.yaml's
// blob_max_inline_bytes / blob_preview_head_bytes /
// blob_preview_tail_bytes keys. The defaults below are the historical
// constants and remain in effect when codrax.yaml leaves them unset.
//
// Why vars and not a struct passed through ctx: blob.go is called
// from every tool's Execute, and threading a config object through
// every call site would be a noisy refactor for a value that does
// not change after startup. The vars are written exactly once from
// main.go before any tool runs, and read-only thereafter, so the
// data race window is empty in practice.

// MaxInlineBytes is the threshold above which a tool result is
// offloaded to a blob file and replaced with a preview in Summary.
// Below this size, output flows through inline so small results stay
// byte-for-byte unchanged.
//
// 32 KB comfortably fits the vast majority of source files in a Go
// repository (this repo's biggest files are ~20 KB), so a typical
// `read_file` returns the whole thing and the LLM never has to guess
// what was hidden in the middle. The previous 4 KB threshold was a
// silent footgun: any file over ~88 lines lost its middle to
// head+tail truncation, which is exactly where most source files put
// the interesting logic.
var MaxInlineBytes = 32 * 1024

// previewHeadBytes / previewTailBytes split the truncation budget for
// the head+tail preview mode (used by exec_command-style tools where
// startup banner + trailing error are both useful). read_file uses a
// head-only preview instead via StoreBlobHeadOnly — see its docs.
var (
	previewHeadBytes = 24 * 1024
	previewTailBytes = 4 * 1024
)

// SetBlobLimits is the controlled entry point for overriding the
// blob sizing knobs at startup. main.go calls it once after loading
// codrax.yaml; tests call it via t.Cleanup to restore defaults. A
// non-positive value means "leave the current value alone", so a
// caller can override one knob without touching the others.
func SetBlobLimits(maxInline, previewHead, previewTail int) {
	if maxInline > 0 {
		MaxInlineBytes = maxInline
	}
	if previewHead > 0 {
		previewHeadBytes = previewHead
	}
	if previewTail > 0 {
		previewTailBytes = previewTail
	}
}

// PreviewHeadBytesValue / PreviewTailBytesValue expose the (otherwise
// package-private) preview-window vars to other packages that need
// to log or display the resolved values. Used by main.go's startup
// banner. Kept as functions instead of exported vars so we don't
// invite mutation from outside this package.
func PreviewHeadBytesValue() int { return previewHeadBytes }
func PreviewTailBytesValue() int { return previewTailBytes }

// StoreBlob conditionally offloads large tool output to disk and
// returns the (summary, ref) pair a ToolResult should carry.
//
// Behavior:
//
//   - Output ≤ MaxInlineBytes: returned unchanged, ref = "". Caller
//     should leave RawRef empty.
//   - Output > MaxInlineBytes but no WorkDir configured (e.g. unit
//     tests with a zero-value BusContext, or a startup error in the
//     orchestrator): summary is a head/tail preview with no ref —
//     content the LLM can act on without crashing the pipeline, but
//     the full bytes are gone.
//   - Output > MaxInlineBytes with a WorkDir: full content is written
//     to <WorkDir>/<toolName>-<sha8>.txt, summary is a preview that
//     names the path and tells the LLM how to retrieve more via
//     read_file (offset/limit) or grep.
//
// The hint embedded in the preview is the only contract by which a
// downstream agent learns about the ref — keep it explicit.
func StoreBlob(ctx *types.BusContext, toolName string, output string) (summary, ref string) {
	if len(output) <= MaxInlineBytes {
		return output, ""
	}

	data := []byte(output)

	if ctx == nil || ctx.WorkDir == "" {
		return buildPreview(data, ""), ""
	}

	if err := os.MkdirAll(ctx.WorkDir, 0o755); err != nil {
		return buildPreview(data, ""), ""
	}

	sum := sha256.Sum256(data)
	name := fmt.Sprintf("%s-%s.txt", sanitizeToolName(toolName), hex.EncodeToString(sum[:4]))
	path := filepath.Join(ctx.WorkDir, name)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return buildPreview(data, ""), ""
	}

	return buildPreview(data, path), path
}

// StoreBlobHeadOnly is the file-aware variant used by tools like
// read_file where the content is line-structured and a hidden middle
// would silently lose information the LLM cannot recover. Instead of
// the head+tail preview StoreBlob produces, this returns ONLY the
// head plus an explicit line-aware notice telling the LLM exactly
// how many lines it saw out of how many total, and how to page the
// remainder via offset/limit.
//
// Behavior mirrors StoreBlob for the small/no-workdir cases.
func StoreBlobHeadOnly(ctx *types.BusContext, toolName string, output string) (summary, ref string) {
	if len(output) <= MaxInlineBytes {
		return output, ""
	}

	data := []byte(output)

	if ctx == nil || ctx.WorkDir == "" {
		return buildHeadPreview(data, "", ""), ""
	}

	if err := os.MkdirAll(ctx.WorkDir, 0o755); err != nil {
		return buildHeadPreview(data, "", ""), ""
	}

	sum := sha256.Sum256(data)
	name := fmt.Sprintf("%s-%s.txt", sanitizeToolName(toolName), hex.EncodeToString(sum[:4]))
	path := filepath.Join(ctx.WorkDir, name)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return buildHeadPreview(data, "", ""), ""
	}

	return buildHeadPreview(data, path, toolName), path
}

// LoadBlobText reads a local blob reference written by StoreBlob,
// StoreBlobHeadOnly, or StoreBlobArtifact. It is intentionally small and
// conservative: URI-like refs are rejected, directories are rejected, and
// callers must provide a byte ceiling. Downstream control-plane code uses this
// to consume tool-owned payload refs instead of reparsing rendered summaries.
func LoadBlobText(ref string, maxBytes int) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "://") {
		return "", false
	}
	if maxBytes <= 0 {
		maxBytes = MaxInlineBytes
	}
	path := filepath.Clean(ref)
	if path == "." || path == "" {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > int64(maxBytes) {
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
	if err != nil || len(data) > maxBytes {
		return "", false
	}
	return string(data), true
}

// StoreBlobArtifact writes structured auxiliary payloads that are meant to be
// referenced from another prompt surface rather than displayed as a tool result
// preview. It reuses the same WorkDir/session directory and filename sanitizing
// policy as StoreBlob, but always writes when WorkDir is available.
func StoreBlobArtifact(workDir, toolName, nameHint, output string) string {
	if strings.TrimSpace(output) == "" || strings.TrimSpace(workDir) == "" {
		return ""
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(output))
	rawName := firstNonEmptyBlobName(nameHint, toolName, "artifact")
	ext := filepath.Ext(rawName)
	if ext == "" {
		ext = ".txt"
	}
	stem := sanitizeToolName(strings.TrimSuffix(rawName, filepath.Ext(rawName)))
	if stem == "" {
		stem = sanitizeToolName(firstNonEmptyBlobName(toolName, "artifact"))
	}
	path := filepath.Join(workDir, fmt.Sprintf("%s-%s%s", stem, hex.EncodeToString(sum[:4]), ext))
	if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
		return ""
	}
	return path
}

// StoreBlobArtifactFromFile writes an auxiliary blob from an already-spooled
// file without loading the full payload into memory. prefix, when non-empty,
// is prepended to the stored artifact and included in the content hash.
func StoreBlobArtifactFromFile(workDir, toolName, nameHint, inputPath, prefix string) string {
	if strings.TrimSpace(workDir) == "" || strings.TrimSpace(inputPath) == "" {
		return ""
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return ""
	}
	in, err := os.Open(inputPath)
	if err != nil {
		return ""
	}
	defer in.Close()

	h := sha256.New()
	if prefix != "" {
		_, _ = h.Write([]byte(prefix))
	}
	if _, err := io.Copy(h, in); err != nil {
		return ""
	}
	sum := h.Sum(nil)

	rawName := firstNonEmptyBlobName(nameHint, toolName, "artifact")
	ext := filepath.Ext(rawName)
	if ext == "" {
		ext = ".txt"
	}
	stem := sanitizeToolName(strings.TrimSuffix(rawName, filepath.Ext(rawName)))
	if stem == "" {
		stem = sanitizeToolName(firstNonEmptyBlobName(toolName, "artifact"))
	}
	path := filepath.Join(workDir, fmt.Sprintf("%s-%s%s", stem, hex.EncodeToString(sum[:4]), ext))

	if _, err := in.Seek(0, 0); err != nil {
		return ""
	}
	out, err := os.Create(path)
	if err != nil {
		return ""
	}
	if prefix != "" {
		if _, err := out.WriteString(prefix); err != nil {
			_ = out.Close()
			_ = os.Remove(path)
			return ""
		}
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(path)
		return ""
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(path)
		return ""
	}
	return path
}

func firstNonEmptyBlobName(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "artifact"
}

// buildHeadPreview returns the leading slice of data with a line-aware
// truncation notice. It deliberately omits the tail because hiding a
// middle of source code without flagging it has caused real failures
// (the LLM sees what looks like a complete snippet and misses the
// section relevant to its question).
func buildHeadPreview(data []byte, ref, toolName string) string {
	total := len(data)

	headEnd := previewHeadBytes
	if headEnd > total {
		headEnd = total
	}
	head := string(data[:headEnd])

	totalLines := countLines(data)
	headLines := countLines(data[:headEnd])

	hint := ""
	if ref != "" && toolName == "read_file" {
		hint = fmt.Sprintf(
			"\n\n…[showing lines 1-%d of %d. The remaining %d lines were NOT returned. Call read_file again with offset=%d (and an appropriate limit) to read further, or grep this same path for specific patterns. Full content also saved to %s.]",
			headLines, totalLines, totalLines-headLines, headLines, ref,
		)
	} else if ref != "" {
		hint = fmt.Sprintf(
			"\n\n…[showing first %d of %d bytes (%d of %d lines). Full content saved to %s — call read_file with that path (use offset/limit to page) or grep it for specific patterns.]",
			headEnd, total, headLines, totalLines, ref,
		)
	} else {
		hint = fmt.Sprintf(
			"\n\n…[showing first %d of %d bytes (%d of %d lines). Remaining content unavailable.]",
			headEnd, total, headLines, totalLines,
		)
	}

	return head + hint
}

func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := 1
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}

// buildPreview assembles a head + tail snippet annotated with retrieval
// instructions. When ref is empty the trailing hint is dropped because
// the caller had no way to persist the full content.
func buildPreview(data []byte, ref string) string {
	total := len(data)

	headEnd := previewHeadBytes
	if headEnd > total {
		headEnd = total
	}
	head := string(data[:headEnd])

	// Head and tail must not overlap; if the data is barely over the
	// threshold, just return the head with a truncation note.
	tailStart := total - previewTailBytes
	if tailStart <= headEnd {
		if ref == "" {
			return fmt.Sprintf("%s\n\n…[truncated, %d bytes total, full content unavailable]", head, total)
		}
		return fmt.Sprintf(
			"%s\n\n…[truncated, %d bytes total. Full content saved to %s — call read_file with that path (use offset/limit to page) or grep it for specific patterns.]",
			head, total, ref,
		)
	}

	tail := string(data[tailStart:])
	middle := total - headEnd - previewTailBytes

	if ref == "" {
		return fmt.Sprintf("%s\n…[truncated %d bytes, full content unavailable]…\n%s", head, middle, tail)
	}
	return fmt.Sprintf(
		"%s\n…[truncated %d bytes. Full content at %s — call read_file with that path (use offset/limit to page) or grep it for specific patterns.]…\n%s",
		head, middle, ref, tail,
	)
}

// --- Blob session directory helpers ----------------------------------
//
// The blob subsystem is WorkDir-agnostic at write time: StoreBlob just
// writes into ctx.WorkDir. cmd/root.go owns the decision of what that
// directory should be — by default <CWD>/.codrax/blob/<session>/, where
// <session> is one per-process directory. These helpers give cmd a
// single place to pick the session name and prune old sessions, so the
// policy lives next to the other blob knobs rather than spread across
// cmd/root.go and the orchestrator.

// SessionDirName returns the current process's session subdirectory
// name, formatted as "<timestamp>-<pid>". The timestamp layout is
// aligned with logging.FileTimeLayout so an operator who already
// parsed a log filename can spot the matching blob session visually.
func SessionDirName() string {
	return fmt.Sprintf("%s-%d",
		time.Now().Format(logging.FileTimeLayout),
		os.Getpid())
}

// PruneBlobSessions deletes the oldest session subdirectories in base
// until at most maxSessions remain. Sessions whose embedded PID is
// still a running process are skipped, so a concurrent peer's
// working set is never yanked out from under it (mirrors the log
// retention contract). maxSessions <= 0 is a no-op; the session
// layout itself is disabled earlier in cmd/root.go.
//
// Best-effort: any filesystem error aborts the sweep silently.
// Retention is hygiene, not correctness.
func PruneBlobSessions(base string, maxSessions int) {
	if maxSessions <= 0 || base == "" {
		return
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if pidFromSessionDirName(e.Name()) == 0 {
			// Not a session directory (random subdir dropped by an
			// operator, or legacy). Leave it alone.
			continue
		}
		names = append(names, e.Name())
	}
	// Names sort lexicographically the same way the timestamps sort
	// chronologically (same property as log filenames).
	sort.Strings(names)

	selfPid := os.Getpid()
	for len(names) > maxSessions {
		removed := false
		for i, name := range names {
			pid := pidFromSessionDirName(name)
			if pid != 0 && pid != selfPid && logging.IsPidAlive(pid) {
				continue // owned by a live peer
			}
			if err := os.RemoveAll(filepath.Join(base, name)); err == nil {
				names = append(names[:i], names[i+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			return // all remaining are live peers; back off
		}
	}
}

// pidFromSessionDirName parses "<timestamp>-<pid>" and returns the pid.
// The timestamp is three dash-separated parts (YYYYMMDD, HHMMSS, mmm)
// from logging.FileTimeLayout, so a well-formed session name has
// exactly four segments. Returns 0 on any mismatch, which callers
// treat as "not a session directory" and skip without pruning.
func pidFromSessionDirName(name string) int {
	parts := strings.Split(name, "-")
	if len(parts) != 4 {
		return 0
	}
	pid, err := strconv.Atoi(parts[3])
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

// sanitizeToolName makes a tool name safe to embed in a filename.
// Anything outside [A-Za-z0-9_-] becomes '_'. An empty result is
// replaced with "tool" so the filename always has a non-empty stem.
func sanitizeToolName(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "tool"
	}
	return string(out)
}
