package tool

import (
	"encoding/json"
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

type structuredEditDiagnostic struct {
	ReasonCode       string   `json:"reason_code"`
	Path             string   `json:"path,omitempty"`
	EditIndex        int      `json:"edit_index,omitempty"`
	FileLineCount    int      `json:"file_line_count,omitempty"`
	StartLine        int      `json:"start_line,omitempty"`
	EndLine          int      `json:"end_line,omitempty"`
	AnchorLine       int      `json:"anchor_line,omitempty"`
	CurrentBytes     string   `json:"current_bytes,omitempty"`
	SuppliedOldText  string   `json:"supplied_old_text,omitempty"`
	ExpectedOldText  string   `json:"expected_old_text,omitempty"`
	CurrentByteLen   int      `json:"current_byte_len,omitempty"`
	SuppliedByteLen  int      `json:"supplied_byte_len,omitempty"`
	RetryInstruction string   `json:"retry_instruction,omitempty"`
	SafeEditKinds    []string `json:"safe_edit_kinds,omitempty"`
}

type structuredEditDiagnosticError struct {
	message    string
	diagnostic structuredEditDiagnostic
}

func (e *structuredEditDiagnosticError) Error() string {
	if e == nil {
		return ""
	}
	b, err := json.Marshal(e.diagnostic)
	if err != nil {
		return e.message
	}
	return e.message + "; diagnostic=" + string(b)
}

func newStructuredEditDiagnosticError(message string, diagnostic structuredEditDiagnostic) error {
	return &structuredEditDiagnosticError{message: message, diagnostic: diagnostic}
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
				if relocatedStart, relocatedEnd, ok := uniqueStructuredOldTextRange(lines, edit.OldText); ok {
					edit.StartLine = relocatedStart + 1
					endLine = relocatedEnd
				} else {
					msg := fmt.Sprintf("structured edit builder: change %q edits[%d] has invalid line range %d-%d for %d-line file", path, i, edit.StartLine, endLine, lineCount)
					return nil, newStructuredEditDiagnosticError(msg, structuredEditDiagnostic{
						ReasonCode:    "invalid_line_range",
						Path:          path,
						EditIndex:     i,
						FileLineCount: lineCount,
						StartLine:     edit.StartLine,
						EndLine:       endLine,
						SafeEditKinds: structuredEditSafeInsertKinds(lines),
					})
				}
			}
			start := edit.StartLine - 1
			end := endLine
			if edit.OldText != "" {
				got := strings.Join(lines[start:end], "")
				if !structuredEditOldTextMatches(got, edit.OldText) {
					if relocatedStart, relocatedEnd, ok := uniqueStructuredOldTextRange(lines, edit.OldText); ok {
						start = relocatedStart
						end = relocatedEnd
						edit.StartLine = relocatedStart + 1
						endLine = relocatedEnd
					} else {
						msg := fmt.Sprintf(
							"structured edit builder: change %q edits[%d] old_text mismatch at lines %d-%d; current bytes are %s — copy diagnostic.expected_old_text exactly into old_text and resend the same bounded edit",
							path, i, edit.StartLine, endLine, boundedByteQuote(got, 160))
						return nil, newStructuredEditDiagnosticError(msg, structuredEditDiagnostic{
							ReasonCode:       "old_text_mismatch",
							Path:             path,
							EditIndex:        i,
							FileLineCount:    lineCount,
							StartLine:        edit.StartLine,
							EndLine:          endLine,
							CurrentBytes:     got,
							SuppliedOldText:  edit.OldText,
							ExpectedOldText:  got,
							CurrentByteLen:   len(got),
							SuppliedByteLen:  len(edit.OldText),
							RetryInstruction: "resend this edit with old_text exactly equal to expected_old_text; keep start_line/end_line aligned to that snippet, omit end_line for a single-line edit, or omit old_text when the line numbers came from the latest read_file view",
							SafeEditKinds:    structuredEditSafeInsertKinds(lines),
						})
					}
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
			if edit.StartLine < 1 {
				msg := fmt.Sprintf("structured edit builder: change %q edits[%d] %s requires start_line for line-addressed insertion", path, i, kind)
				return nil, newStructuredEditDiagnosticError(msg, structuredEditDiagnostic{
					ReasonCode:    "missing_start_line",
					Path:          path,
					EditIndex:     i,
					FileLineCount: lineCount,
					SafeEditKinds: structuredEditSafeInsertKinds(lines),
				})
			}
			start, err := insertionIndex(kind, edit.StartLine, lineCount)
			if err != nil {
				msg := fmt.Sprintf("structured edit builder: change %q edits[%d] %v", path, i, err)
				return nil, newStructuredEditDiagnosticError(msg, structuredEditDiagnostic{
					ReasonCode:    "invalid_insert_anchor",
					Path:          path,
					EditIndex:     i,
					FileLineCount: lineCount,
					StartLine:     edit.StartLine,
					SafeEditKinds: structuredEditSafeInsertKinds(lines),
				})
			}
			if edit.OldText != "" {
				anchor := ""
				if edit.StartLine >= 1 && edit.StartLine <= lineCount {
					anchor = lines[edit.StartLine-1]
				}
				if !structuredEditOldTextMatches(anchor, edit.OldText) {
					if relocatedStart, _, ok := uniqueStructuredOldTextRange(lines, edit.OldText); ok {
						edit.StartLine = relocatedStart + 1
						start, err = insertionIndex(kind, edit.StartLine, lineCount)
						if err != nil {
							msg := fmt.Sprintf("structured edit builder: change %q edits[%d] %v", path, i, err)
							return nil, newStructuredEditDiagnosticError(msg, structuredEditDiagnostic{
								ReasonCode:    "invalid_insert_anchor",
								Path:          path,
								EditIndex:     i,
								FileLineCount: lineCount,
								StartLine:     edit.StartLine,
								SafeEditKinds: structuredEditSafeInsertKinds(lines),
							})
						}
					} else {
						msg := fmt.Sprintf(
							"structured edit builder: change %q edits[%d] old_text mismatch at anchor line %d; current bytes are %s — copy diagnostic.expected_old_text exactly into old_text and resend the same bounded edit",
							path, i, edit.StartLine, boundedByteQuote(anchor, 160))
						return nil, newStructuredEditDiagnosticError(msg, structuredEditDiagnostic{
							ReasonCode:       "old_text_mismatch",
							Path:             path,
							EditIndex:        i,
							FileLineCount:    lineCount,
							StartLine:        edit.StartLine,
							AnchorLine:       edit.StartLine,
							CurrentBytes:     anchor,
							SuppliedOldText:  edit.OldText,
							ExpectedOldText:  anchor,
							CurrentByteLen:   len(anchor),
							SuppliedByteLen:  len(edit.OldText),
							RetryInstruction: "resend this insertion with old_text exactly equal to expected_old_text for the anchor line, or omit old_text when line anchoring alone is acceptable",
							SafeEditKinds:    structuredEditSafeInsertKinds(lines),
						})
					}
				}
			}
			key := fmt.Sprintf("%s:%d", kind, start)
			if prev, dup := insertPoints[key]; dup {
				return nil, fmt.Errorf("structured edit builder: change %q edits[%d] duplicates insertion point from edits[%d]", path, i, prev)
			}
			insertPoints[key] = i
			out = append(out, compiledStructuredEdit{kind: kind, start: start, end: start, insert: splitContentLines(edit.Content), sourceIdx: i})
		case "insert_at_eof", "insert_before_final_brace":
			if edit.Content == "" {
				return nil, fmt.Errorf("structured edit builder: change %q edits[%d] %s requires non-empty content", path, i, kind)
			}
			start := lineCount
			if kind == "insert_before_final_brace" {
				var ok bool
				start, ok = finalBraceInsertionIndex(lines)
				if !ok {
					msg := fmt.Sprintf("structured edit builder: change %q edits[%d] could not find a final standalone closing brace for insert_before_final_brace", path, i)
					return nil, newStructuredEditDiagnosticError(msg, structuredEditDiagnostic{
						ReasonCode:    "final_brace_not_found",
						Path:          path,
						EditIndex:     i,
						FileLineCount: lineCount,
						SafeEditKinds: []string{"insert_at_eof"},
					})
				}
			}
			key := fmt.Sprintf("%s:%d", kind, start)
			if prev, dup := insertPoints[key]; dup {
				return nil, fmt.Errorf("structured edit builder: change %q edits[%d] duplicates insertion point from edits[%d]", path, i, prev)
			}
			insertPoints[key] = i
			out = append(out, compiledStructuredEdit{kind: kind, start: start, end: start, insert: splitContentLines(edit.Content), sourceIdx: i})
		default:
			return nil, fmt.Errorf("structured edit builder: change %q edits[%d] has illegal kind %q (must be replace|delete|insert_before|insert_after|insert_at_eof|insert_before_final_brace)", path, i, edit.Kind)
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
		if !isStructuredInsertKind(edit.kind) {
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

func uniqueStructuredOldTextRange(lines []string, oldText string) (start, end int, ok bool) {
	if oldText == "" || len(lines) == 0 {
		return 0, 0, false
	}
	needleLines := splitContentLines(oldText)
	if len(needleLines) == 0 || len(needleLines) > len(lines) {
		return 0, 0, false
	}
	matchStart := -1
	matchEnd := -1
	for i := 0; i+len(needleLines) <= len(lines); i++ {
		j := i + len(needleLines)
		if !structuredEditOldTextMatches(strings.Join(lines[i:j], ""), oldText) {
			continue
		}
		if matchStart >= 0 {
			return 0, 0, false
		}
		matchStart = i
		matchEnd = j
	}
	if matchStart < 0 {
		return 0, 0, false
	}
	return matchStart, matchEnd, true
}

func isStructuredInsertKind(kind string) bool {
	switch kind {
	case "insert_before", "insert_after", "insert_at_eof", "insert_before_final_brace":
		return true
	default:
		return false
	}
}

func finalBraceInsertionIndex(lines []string) (int, bool) {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if strings.TrimSpace(lines[i]) == "}" {
			return i, true
		}
		return 0, false
	}
	return 0, false
}

func structuredEditSafeInsertKinds(lines []string) []string {
	out := []string{"insert_at_eof"}
	if _, ok := finalBraceInsertionIndex(lines); ok {
		out = append(out, "insert_before_final_brace")
	}
	return out
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
