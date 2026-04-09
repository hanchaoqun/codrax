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
// offloaded to a blob file and replaced with a head/tail preview in
// Summary. Below this size, output flows through inline so small
// results (the common case in tests and quick lookups) stay byte-for-
// byte unchanged.
const MaxInlineBytes = 4096

// previewHeadBytes / previewTailBytes split the truncation budget.
// Head dominates because tool output is typically front-loaded with
// the most useful information (file headers, first matches, command
// banners). Tail catches trailing errors and stack traces.
const (
	previewHeadBytes = 3072
	previewTailBytes = 512
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
