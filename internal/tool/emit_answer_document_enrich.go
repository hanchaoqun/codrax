package tool

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_answer_document_enrich.go — render-only augmentation for
// the Snippets field. The LLM's emit_answer_document call lands the
// structured answer (summary, citations, steps, etc.), and
// extractCodeSnippets runs immediately afterward to populate up to
// N short code excerpts extracted from the read_file gutter index
// at each cited file:line. Adjacent citations in the same file
// collapse into one snippet so cite(:63) + cite(:65) render as a
// single 6-line block, not two near-duplicates.
//
// The field is deterministic over ctx.Mutable state and therefore
// independent of LLM wording — it survives re-runs and answer
// cache invalidations without drift.

// snippetContextLines is the symmetric line window around each
// citation line. 2 before + 2 after gives the reader enough context
// to recognize the code without burying the cited line in noise.
const snippetContextLines = 2

// snippetMaxLines caps the total line span of one rendered snippet
// block so even a tightly-clustered cite pool doesn't overwhelm the
// answer. Adjacent citations collapse into one snippet only when
// the merged span stays under this cap.
const snippetMaxLines = 12

// extractCodeSnippets returns up to maxN code excerpts, one per
// (file, line-range) cluster, derived from the citations pool. Each
// snippet pulls ±snippetContextLines around the citation line from
// the read_file gutter index; citations in the same file whose line
// windows overlap or abut are merged into a single range. Citations
// whose file was never read (no gutter entry) are skipped — the
// snippet would have no real text.
//
// Ordering mirrors the citation pool, so the first snippet maps to
// the most important citation.
func extractCodeSnippets(ctx *types.BusContext, doc *types.AnswerDocument, maxN int) []types.CodeSnippet {
	if ctx == nil || doc == nil || len(doc.Citations) == 0 || maxN <= 0 {
		return nil
	}
	gc := ground.BuildContext(ctx)
	if gc == nil || len(gc.LineIndex) == 0 {
		return nil
	}

	type rangeEntry struct {
		file      string
		startLine int
		endLine   int
		order     int // position in doc.Citations for stable ordering
	}

	// Collect one window per citation. Stable by citation order.
	windows := make([]rangeEntry, 0, len(doc.Citations))
	for i, c := range doc.Citations {
		if _, ok := gc.LineIndex[c.File]; !ok {
			continue
		}
		start := c.Line - snippetContextLines
		if start < 1 {
			start = 1
		}
		end := c.Line + snippetContextLines
		windows = append(windows, rangeEntry{
			file:      c.File,
			startLine: start,
			endLine:   end,
			order:     i,
		})
	}
	if len(windows) == 0 {
		return nil
	}

	// Cluster same-file adjacent windows. Sort by (file, start) so
	// merges can fold consecutively; after merging, re-sort by the
	// earliest citation order of each cluster so the output mirrors
	// citation priority.
	sort.SliceStable(windows, func(i, j int) bool {
		if windows[i].file != windows[j].file {
			return windows[i].file < windows[j].file
		}
		return windows[i].startLine < windows[j].startLine
	})

	type cluster struct {
		file      string
		startLine int
		endLine   int
		firstOrd  int
	}
	clusters := make([]cluster, 0, len(windows))
	for _, w := range windows {
		if len(clusters) > 0 {
			last := &clusters[len(clusters)-1]
			if last.file == w.file && w.startLine <= last.endLine+1 &&
				w.endLine-last.startLine+1 <= snippetMaxLines {
				if w.endLine > last.endLine {
					last.endLine = w.endLine
				}
				if w.order < last.firstOrd {
					last.firstOrd = w.order
				}
				continue
			}
		}
		clusters = append(clusters, cluster{
			file:      w.file,
			startLine: w.startLine,
			endLine:   w.endLine,
			firstOrd:  w.order,
		})
	}
	sort.SliceStable(clusters, func(i, j int) bool {
		return clusters[i].firstOrd < clusters[j].firstOrd
	})

	out := make([]types.CodeSnippet, 0, maxN)
	for _, cl := range clusters {
		if len(out) >= maxN {
			break
		}
		code := renderSnippetBody(gc.LineIndex[cl.file], cl.startLine, cl.endLine)
		if strings.TrimSpace(code) == "" {
			continue
		}
		out = append(out, types.CodeSnippet{
			File:      cl.file,
			StartLine: cl.startLine,
			EndLine:   cl.endLine,
			Language:  languageFromPath(cl.file),
			Code:      code,
		})
	}
	return out
}

// renderSnippetBody pulls the given line range out of the gutter
// index and returns a gutter-free, \n-joined string. Missing lines
// (outside the read range) are rendered as empty so the snippet
// stays contiguous in display even when the read window didn't
// cover the full ±2.
func renderSnippetBody(fileLines map[int]string, start, end int) string {
	if fileLines == nil {
		return ""
	}
	var b strings.Builder
	for ln := start; ln <= end; ln++ {
		text, ok := fileLines[ln]
		if !ok {
			// Hit the edge of the read window — stop rather than
			// pad with blank lines that misrepresent the file.
			break
		}
		b.WriteString(text)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// languageFromPath maps a repo-relative file extension to a
// language tag for fenced code blocks. Covers the ten languages
// codrax's keyword/grep path routinely indexes; unknown extensions
// return "" so the renderer emits an untagged fence.
func languageFromPath(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".rb":
		return "ruby"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "cpp"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".sh":
		return "sh"
	case ".md":
		return "markdown"
	}
	return ""
}

