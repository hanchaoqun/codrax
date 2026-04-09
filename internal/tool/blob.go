package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/types"
)

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
const MaxInlineBytes = 32 * 1024

// previewHeadBytes / previewTailBytes split the truncation budget for
// the head+tail preview mode (used by exec_command-style tools where
// startup banner + trailing error are both useful). read_file uses a
// head-only preview instead via StoreBlobHeadOnly — see its docs.
const (
	previewHeadBytes = 24 * 1024
	previewTailBytes = 4 * 1024
)

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
