package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/hanchaoqun/codrax/internal/types"
)

type compiledStructuredEdit struct {
	kind      string
	start     int
	end       int
	insert    []string
	sourceIdx int
}

func compileStructuredEditsToPatch(repoRoot string, change *types.FileChange) (string, error) {
	if change == nil {
		return "", fmt.Errorf("structured edit builder: nil change")
	}
	path := filepath.ToSlash(strings.TrimSpace(change.Path))
	if path == "" {
		return "", fmt.Errorf("structured edit builder: empty path")
	}
	if strings.TrimSpace(change.Kind) != "patch" {
		return "", fmt.Errorf("structured edit builder: change %q uses edits with kind=%s; edits are only valid for kind=patch", path, change.Kind)
	}
	if len(change.Edits) == 0 {
		return "", fmt.Errorf("structured edit builder: change %q has no edits", path)
	}
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return "", fmt.Errorf("structured edit builder: repo root is empty")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("structured edit builder: resolve repo root: %w", err)
	}
	absPath := filepath.Join(absRoot, filepath.FromSlash(path))
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("structured edit builder: resolve %s: %w", path, err)
	}
	if rel, err := filepath.Rel(absRoot, absPath); err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("structured edit builder: path %q escapes repo root", path)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("structured edit builder: read %s: %w", path, err)
	}
	oldContent := string(data)
	lines := splitContentLines(oldContent)
	compiled, err := normalizeStructuredEdits(path, lines, change.Edits)
	if err != nil {
		return "", err
	}
	newLines := append([]string(nil), lines...)
	sort.SliceStable(compiled, func(i, j int) bool {
		if compiled[i].start != compiled[j].start {
			return compiled[i].start > compiled[j].start
		}
		return compiled[i].sourceIdx > compiled[j].sourceIdx
	})
	for _, edit := range compiled {
		newLines = spliceLines(newLines, edit.start, edit.end, edit.insert)
	}
	// Line-integrity normalization. Structured edits are LINE-based and
	// the schema tolerates a missing final "\n" on old_text as a quoting
	// variation; mirror the same byte-level tolerance on the content
	// side. Without this, a mid-file replacement whose content omits the
	// trailing "\n" fuses with the following line at join time (observed
	// live: a one-line typo fix absorbed the next line's bare `}`,
	// synthesizing a -2/+1 line-joining diff the model never asked for).
	// Every element except the last must end with "\n"; the last element
	// keeps the original file's EOF-newline convention.
	for i := 0; i < len(newLines)-1; i++ {
		if !strings.HasSuffix(newLines[i], "\n") {
			newLines[i] += "\n"
		}
	}
	if n := len(newLines); n > 0 && strings.HasSuffix(oldContent, "\n") && !strings.HasSuffix(newLines[n-1], "\n") {
		newLines[n-1] += "\n"
	}
	newContent := strings.Join(newLines, "")
	if newContent == oldContent {
		return "", fmt.Errorf("structured edit builder: change %q is a no-op", path)
	}
	patch := udiff.Unified("a/"+path, "b/"+path, oldContent, newContent)
	if strings.TrimSpace(patch) == "" {
		return "", fmt.Errorf("structured edit builder: change %q produced an empty diff", path)
	}
	return patch, nil
}

func normalizeStructuredEdits(path string, lines []string, edits []types.StructuredEdit) ([]compiledStructuredEdit, error) {
	out := make([]compiledStructuredEdit, 0, len(edits))
	lineCount := len(lines)
	rangeEdits := []compiledStructuredEdit{}
	insertPoints := map[string]int{}
	for i, edit := range edits {
		kind := strings.TrimSpace(edit.Kind)
		if kind == "" {
			return nil, fmt.Errorf("structured edit builder: change %q edits[%d] has empty kind", path, i)
		}
		switch kind {
		case "replace", "delete":
			endLine := edit.EndLine
			// Omitted end_line means a single-line edit. The schema
			// documents the default so the model never has to copy
			// line arithmetic for the common one-line replace.
			if endLine == 0 {
				endLine = edit.StartLine
			}
			if edit.StartLine < 1 || endLine < edit.StartLine || endLine > lineCount {
				return nil, fmt.Errorf("structured edit builder: change %q edits[%d] has invalid line range %d-%d for %d-line file", path, i, edit.StartLine, endLine, lineCount)
			}
			start := edit.StartLine - 1
			end := endLine
			if edit.OldText != "" {
				got := strings.Join(lines[start:end], "")
				if !structuredEditOldTextMatches(got, edit.OldText) {
					return nil, fmt.Errorf(
						"structured edit builder: change %q edits[%d] old_text mismatch at lines %d-%d; current bytes are %s — re-read the file and resend old_text matching the current content",
						path, i, edit.StartLine, endLine, boundedByteQuote(got, 160))
				}
			}
			insert := []string(nil)
			if kind == "replace" {
				if edit.Content == "" {
					return nil, fmt.Errorf("structured edit builder: change %q edits[%d] replace requires non-empty content; use kind=delete to remove lines", path, i)
				}
				insert = splitContentLines(edit.Content)
			}
			compiled := compiledStructuredEdit{kind: kind, start: start, end: end, insert: insert, sourceIdx: i}
			rangeEdits = append(rangeEdits, compiled)
			out = append(out, compiled)
		case "insert_before", "insert_after":
			if edit.Content == "" {
				return nil, fmt.Errorf("structured edit builder: change %q edits[%d] %s requires non-empty content", path, i, kind)
			}
			start, err := insertionIndex(kind, edit.StartLine, lineCount)
			if err != nil {
				return nil, fmt.Errorf("structured edit builder: change %q edits[%d] %v", path, i, err)
			}
			if edit.OldText != "" {
				anchor := ""
				if edit.StartLine >= 1 && edit.StartLine <= lineCount {
					anchor = lines[edit.StartLine-1]
				}
				if !structuredEditOldTextMatches(anchor, edit.OldText) {
					return nil, fmt.Errorf(
						"structured edit builder: change %q edits[%d] old_text mismatch at anchor line %d; current bytes are %s — re-read the file and resend old_text matching the current content",
						path, i, edit.StartLine, boundedByteQuote(anchor, 160))
				}
			}
			key := fmt.Sprintf("%s:%d", kind, start)
			if prev, dup := insertPoints[key]; dup {
				return nil, fmt.Errorf("structured edit builder: change %q edits[%d] duplicates insertion point from edits[%d]", path, i, prev)
			}
			insertPoints[key] = i
			out = append(out, compiledStructuredEdit{kind: kind, start: start, end: start, insert: splitContentLines(edit.Content), sourceIdx: i})
		default:
			return nil, fmt.Errorf("structured edit builder: change %q edits[%d] has illegal kind %q (must be replace|delete|insert_before|insert_after)", path, i, edit.Kind)
		}
	}
	sort.SliceStable(rangeEdits, func(i, j int) bool {
		if rangeEdits[i].start != rangeEdits[j].start {
			return rangeEdits[i].start < rangeEdits[j].start
		}
		return rangeEdits[i].end < rangeEdits[j].end
	})
	for i := 1; i < len(rangeEdits); i++ {
		prev := rangeEdits[i-1]
		cur := rangeEdits[i]
		if cur.start < prev.end {
			return nil, fmt.Errorf("structured edit builder: change %q edits[%d] overlaps edits[%d]", path, cur.sourceIdx, prev.sourceIdx)
		}
	}
	for _, edit := range out {
		if edit.kind != "insert_before" && edit.kind != "insert_after" {
			continue
		}
		for _, rng := range rangeEdits {
			if edit.start >= rng.start && edit.start <= rng.end {
				return nil, fmt.Errorf("structured edit builder: change %q edits[%d] inserts inside replaced/deleted range from edits[%d]", path, edit.sourceIdx, rng.sourceIdx)
			}
		}
	}
	return out, nil
}

func insertionIndex(kind string, line, lineCount int) (int, error) {
	switch kind {
	case "insert_before":
		if line < 1 || line > lineCount+1 {
			return 0, fmt.Errorf("insert_before start_line %d is outside 1..%d", line, lineCount+1)
		}
		return line - 1, nil
	case "insert_after":
		if line < 1 || line > lineCount {
			return 0, fmt.Errorf("insert_after start_line %d is outside 1..%d", line, lineCount)
		}
		return line, nil
	default:
		return 0, fmt.Errorf("unsupported insertion kind %q", kind)
	}
}

func splitContentLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func spliceLines(lines []string, start, end int, insert []string) []string {
	next := make([]string, 0, len(lines)-(end-start)+len(insert))
	next = append(next, lines[:start]...)
	next = append(next, insert...)
	next = append(next, lines[end:]...)
	return next
}

// structuredEditOldTextMatches compares the model-supplied old_text against
// the current bytes of the target range. Exact byte equality is the rule;
// the single documented normalization is the final trailing newline — the
// model frequently quotes a range without (or with) the terminal "\n" that
// SplitAfter preserves. This is a byte-level structural tolerance, never a
// fuzzy match.
func structuredEditOldTextMatches(got, oldText string) bool {
	if got == oldText {
		return true
	}
	if got == oldText+"\n" || got+"\n" == oldText {
		return true
	}
	return false
}

// boundedByteQuote renders the current bytes for a mismatch diagnostic,
// quoted and capped so the repair message stays readable while showing the
// model exactly what the file holds now.
func boundedByteQuote(s string, max int) string {
	runes := []rune(s)
	if max > 0 && len(runes) > max {
		return fmt.Sprintf("%q (truncated, %d bytes total)", string(runes[:max]), len(s))
	}
	return fmt.Sprintf("%q", s)
}
