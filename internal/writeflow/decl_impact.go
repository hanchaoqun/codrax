package writeflow

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// planChangeTouchedOldLines computes the set of pre-image (old-file) line
// numbers a FileChange modifies or deletes. It is a deterministic structural
// parse of the plan's own typed content — unified-diff hunk bodies for
// kind=patch and StructuredEdit line ranges — never of request or rationale
// prose. Pure insertions contribute nothing: adding lines cannot alter an
// existing declaration line, and additive API surface stays in the softer
// medium-grade lane.
func planChangeTouchedOldLines(change types.FileChange) map[int]bool {
	touched := map[int]bool{}
	for _, edit := range change.Edits {
		kind := strings.TrimSpace(edit.Kind)
		if kind != "replace" && kind != "delete" {
			continue
		}
		end := edit.EndLine
		if end < edit.StartLine {
			end = edit.StartLine
		}
		for ln := edit.StartLine; ln <= end; ln++ {
			if ln > 0 {
				touched[ln] = true
			}
		}
	}
	if strings.TrimSpace(change.Patch) != "" {
		for ln := range unifiedDiffRemovedOldLines(change.Patch) {
			touched[ln] = true
		}
	}
	return touched
}

var unifiedHunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+\d+(?:,\d+)? @@`)

// unifiedDiffRemovedOldLines walks every hunk of a unified diff and returns
// the old-file line numbers of '-' rows — the lines the patch modifies or
// deletes. Context rows advance the old counter; '+' rows do not.
func unifiedDiffRemovedOldLines(patch string) map[int]bool {
	removed := map[int]bool{}
	scanner := bufio.NewScanner(strings.NewReader(patch))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	oldLine := 0
	inHunk := false
	for scanner.Scan() {
		line := scanner.Text()
		if m := unifiedHunkHeaderRe.FindStringSubmatch(line); m != nil {
			start, err := strconv.Atoi(m[1])
			if err != nil {
				inHunk = false
				continue
			}
			oldLine = start
			inHunk = true
			continue
		}
		if !inHunk || len(line) == 0 {
			continue
		}
		switch line[0] {
		case '-':
			if strings.HasPrefix(line, "---") {
				continue
			}
			removed[oldLine] = true
			oldLine++
		case '+':
			if strings.HasPrefix(line, "+++") {
				continue
			}
			// new-file line only; old counter unchanged
		case ' ':
			oldLine++
		case '\\':
			// "\ No newline at end of file"
		default:
			// Diff garbage / file headers between hunks end the hunk.
			inHunk = false
		}
	}
	return removed
}

// assessExportedDeclImpact adds a RiskMedium reason for every plan change whose
// modified/deleted old lines intersect an exported declaration line reported
// by the typed graph. Returns true when any intersection was found. Files the
// source cannot vouch for (ok=false) contribute nothing — the LLM risk axes
// then stand at their medium grade. Only kind=patch changes are inspected:
// create has no pre-image, full modify/delete are graded by change kind and
// path class already. The declaration-line signal is precise but not
// automatically dangerous; high/critical approval remains reserved for typed
// blast-radius and safety policy signals.
func assessExportedDeclImpact(a *RiskAssessment, plan *types.ChangePlan, decls types.DeclSpanSource) bool {
	if a == nil || plan == nil || decls == nil {
		return false
	}
	found := false
	for _, change := range plan.Changes {
		if strings.ToLower(strings.TrimSpace(change.Kind)) != "patch" {
			continue
		}
		touched := planChangeTouchedOldLines(change)
		if len(touched) == 0 {
			continue
		}
		declLines, ok := decls.ExportedDeclLines(strings.TrimSpace(change.Path))
		if !ok {
			continue
		}
		for _, ln := range declLines {
			if !touched[ln] {
				continue
			}
			found = true
			a.add(RiskMedium, "public_decl_line_changed",
				fmt.Sprintf("patch modifies an exported declaration line (%s:%d)", change.Path, ln),
				change.Path)
			break
		}
	}
	return found
}
