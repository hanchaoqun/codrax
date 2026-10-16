package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/textfmt"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This projection sees the bytes already shown, not a second file read or a
// fabricated read_file result. Generic source coverage remains line-based.
type preReadNavigation func(path string, data []byte, complete bool) string

func preReadCatalogNavigation(ctx *types.AgentContext) preReadNavigation {
	if ctx == nil {
		return nil
	}
	return func(path string, data []byte, complete bool) string {
		// Reuse the ordinary tool projection so host pins, active-set state,
		// cancellation and worktree path anchors cannot drift between readers.
		bus := types.ToolBusContext(ctx, types.AgentExplorer)
		nav := tool.BuildTraceCatalogReadNavigation(bus, path, data, complete)
		if nav == nil {
			return ""
		}
		return nav.Summary + "\n" + strings.Join(types.ToolRefinementPromptFields(nav.Refinement, types.ToolRefinementPromptFieldOptions{QuoteValues: true}), " ") + "\n\n"
	}
}

func preReadRequiredFilesWithObserver(repoRoot string, files []string, maxFiles, maxLines int, excludeRead map[string]bool, observe func(file string, totalLines int, lines []string), navigation ...preReadNavigation) string {
	if repoRoot == "" || len(files) == 0 || maxFiles <= 0 {
		return ""
	}
	var b strings.Builder
	injected := 0
	for _, f := range files {
		if injected >= maxFiles {
			break
		}
		if excludeRead != nil && excludeRead[canonicalExplorerPath(f)] {
			continue
		}
		absPath := filepath.Join(repoRoot, f)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		lines := textfmt.PhysicalLines(string(content))
		totalLines := len(lines)
		truncated := false
		if totalLines > maxLines {
			lines = lines[:maxLines]
			truncated = true
		}
		fmt.Fprintf(&b, "#### `%s`", f)
		if truncated {
			fmt.Fprintf(&b, " (first %d of %d total lines — issue read_file with line_offset to see the rest)", maxLines, totalLines)
		} else {
			fmt.Fprintf(&b, " (full file, %d lines)", totalLines)
		}
		b.WriteString("\n```\n")
		for i, line := range lines {
			fmt.Fprintf(&b, "%d\t%s\n", i+1, line)
		}
		b.WriteString("```\n\n")
		if observe != nil {
			observe(f, totalLines, append([]string(nil), lines...))
		}
		// Use the same cap that rendered the file. Never navigate from a
		// hidden tail, reconstructed gutter text, or a reread.
		for _, project := range navigation {
			if project != nil {
				b.WriteString(project(absPath, content, !truncated))
			}
		}
		injected++
	}
	return b.String()
}
