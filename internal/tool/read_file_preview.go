package tool

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// readFileCompletePreviewLines uses the producer's exact banner length and
// rendered row boundaries, not a parser over model prose or JSON contents.
func readFileCompletePreviewLines(content, banner string, selectedLines int) int {
	head := cutHeadAtBoundary([]byte(content), PreviewHeadBytesValue())
	if len(head) == len(content) {
		return selectedLines
	}
	if len(head) <= len(banner) {
		return 0
	}
	// An omitted suffix means only newline-terminated rows are complete.
	return strings.Count(head[len(banner):], "\n")
}

func readFileOversizedLineResult(ctx *types.BusContext, requestedPath, fsPath, ref string, line, total int) types.ToolResult {
	hint := "The file is readable, but this physical line together with its read metadata does not fit the inline byte budget. line_offset/limit select whole lines, not bytes; advancing the line offset would skip unseen content. Use a bounded structured query or field extraction from the same file."
	if ref != "" {
		hint += " The raw artifact retains the selected bytes; its wrapper line numbers are not source coordinates."
	} else {
		hint += " The selected bytes could not be persisted as a raw artifact; the original file remains the source."
	}
	// Reading bytes succeeded. The zero range retains source identity without
	// certifying any visible line, unlike a complete, known-empty enumeration.
	var coverage *types.ToolReadCoverage
	if sourcePath := readFileTypedSourcePath(ctx, requestedPath, fsPath); sourcePath != "" {
		coverage = &types.ToolReadCoverage{Path: sourcePath, TotalLines: total, RawRef: strings.TrimSpace(ref)}
	}
	return types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  fmt.Sprintf("[%s: no complete lines displayed; physical line %d exceeds the inline byte budget; %d total lines]\n%s", requestedPath, line, total, hint),
		RawRef:   ref,
		Repair: &types.ToolRepair{
			Code: "read_file_physical_line_exceeds_byte_budget", Hint: hint,
			Fields: []string{"path"},
		},
		EnumerationAuthority: &types.ToolEnumerationAuthority{
			Status: "incomplete",
			Boundaries: []types.ToolEnumerationBoundary{{
				Scope: requestedPath, Dimension: "lines", Emitted: 0,
				Total: total, TotalKnown: true, Reason: "physical_line_exceeds_byte_budget",
			}},
		},
		RuntimeArtifactRead: readFileRuntimeArtifactMarker(ctx, requestedPath, fsPath, ref, 0, 0, total),
		ReadCoverage:        coverage,
		Timestamp:           time.Now(),
	}
}
