package writeflow

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// PreflightDiagnostic is the language-neutral shape for syntax/static/build
// diagnostics that can be scoped to a patch surface. Producers may be Python
// symtable, TypeScript, javac, rustc, or any future parser, as long as they
// provide a file and 1-based line number.
type PreflightDiagnostic struct {
	Path    string
	Line    int
	Column  int
	Code    string
	Message string
}

// FilterPreflightDiagnosticsToChangedLines returns only diagnostics whose
// file:line lies on a line introduced or replaced by this ChangePlan. When the
// plan lacks a precise line surface for the file, diagnostics are preserved
// fail-closed so hard gates still catch newly authored full-file rewrites.
func FilterPreflightDiagnosticsToChangedLines(plan *types.ChangePlan, path, newContent string, diagnostics []PreflightDiagnostic) []PreflightDiagnostic {
	if len(diagnostics) == 0 {
		return nil
	}
	changed, precise := ChangedLinesForPlanPath(plan, path, newContent)
	if !precise {
		return clonePreflightDiagnostics(diagnostics)
	}
	var out []PreflightDiagnostic
	for _, diag := range diagnostics {
		if diag.Line <= 0 {
			out = append(out, diag)
			continue
		}
		if _, ok := changed[diag.Line]; ok {
			out = append(out, diag)
		}
	}
	return out
}

func ChangedLinesForPlanPath(plan *types.ChangePlan, path, newContent string) (map[int]struct{}, bool) {
	lines := map[int]struct{}{}
	if plan == nil {
		return lines, false
	}
	normPath := normalizePreflightPath(path)
	matched := false
	precise := true
	for _, change := range plan.Changes {
		if normalizePreflightPath(change.Path) != normPath {
			continue
		}
		matched = true
		switch strings.TrimSpace(change.Kind) {
		case "create", "modify":
			addLineRange(lines, 1, countTextLines(firstNonEmptyPreflight(change.NewContent, newContent)))
		case "patch", "":
			if strings.TrimSpace(change.Patch) != "" {
				addPatchAddedLines(lines, change.Path, change.Patch)
				continue
			}
			if len(change.Edits) > 0 {
				addStructuredEditChangedLines(lines, change.Edits)
				continue
			}
			precise = false
		case "delete":
			// A delete cannot introduce a new undefined-name/static diagnostic
			// in the post-change file.
		default:
			precise = false
		}
	}
	if !matched {
		return lines, false
	}
	return lines, precise
}

func addPatchAddedLines(lines map[int]struct{}, path, diff string) {
	record := PatchEffectRecordFromUnifiedDiff("", "", "change_plan", "", "", diff)
	if len(record.Files) == 0 && strings.TrimSpace(path) != "" {
		p := normalizePreflightPath(path)
		record = PatchEffectRecordFromUnifiedDiff("", "", "change_plan", "", "", "diff --git a/"+p+" b/"+p+"\n"+diff)
	}
	for _, file := range record.Files {
		for _, hunk := range file.Hunks {
			for _, line := range hunk.AddedLineNumbers {
				if line > 0 {
					lines[line] = struct{}{}
				}
			}
		}
	}
}

func addStructuredEditChangedLines(lines map[int]struct{}, edits []types.StructuredEdit) {
	for _, edit := range edits {
		start := edit.StartLine
		if start < 1 {
			start = 1
		}
		switch strings.TrimSpace(edit.Kind) {
		case "delete":
			continue
		case "insert_after":
			start++
		case "insert_at_eof":
			if edit.StartLine > 0 {
				start = edit.StartLine
			}
		}
		count := countTextLines(edit.Content)
		if count <= 0 {
			count = 1
		}
		addLineRange(lines, start, count)
	}
}

func addLineRange(lines map[int]struct{}, start, count int) {
	if start < 1 {
		start = 1
	}
	for i := 0; i < count; i++ {
		lines[start+i] = struct{}{}
	}
}

func countTextLines(text string) int {
	if text == "" {
		return 0
	}
	count := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		count++
	}
	return count
}

func firstNonEmptyPreflight(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizePreflightPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.ToSlash(filepath.Clean(path))
	path = strings.TrimPrefix(path, "./")
	return strings.TrimPrefix(path, "b/")
}

func clonePreflightDiagnostics(in []PreflightDiagnostic) []PreflightDiagnostic {
	if len(in) == 0 {
		return nil
	}
	out := append([]PreflightDiagnostic(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Column < out[j].Column
	})
	return out
}
