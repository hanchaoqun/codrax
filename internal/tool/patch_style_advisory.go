package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Patch line-structure advisory: a purely STRUCTURAL comparison of a
// unified diff's removed vs added blocks, flagging edits that compress
// the original's multi-line structure onto single lines (the class
// observed live: an `if (...) {` line absorbing the `return ...;`
// statement the original kept on its own line). Style is a noisy
// signal, so per the architecture red line this NEVER gates: the note
// rides the SUCCESSFUL emit summary (the same compat-note channel as
// payload salvage) and the model may re-emit voluntarily. The detector
// reads only brace/semicolon/newline arrangement — language-agnostic,
// no keywords, and it only fires relative to the removed side, so
// files that already use single-line style are never flagged.

// analyzePatchLineCompression returns bounded advisory notes for one
// file's unified diff, empty when no compression is detected.
func analyzePatchLineCompression(path, patch string) []string {
	var notes []string
	removedBraceEndsLine := false
	removedMaxSemis := 0
	flush := func() {
		removedBraceEndsLine = false
		removedMaxSemis = 0
	}
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			flush()
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			body := strings.TrimSpace(line[1:])
			if strings.HasSuffix(body, "{") {
				removedBraceEndsLine = true
			}
			if n := strings.Count(body, ";"); n > removedMaxSemis {
				removedMaxSemis = n
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			body := strings.TrimSpace(line[1:])
			if removedBraceEndsLine {
				if idx := strings.IndexByte(body, '{'); idx >= 0 {
					tail := strings.TrimSpace(body[idx+1:])
					if tail != "" && tail != "}" {
						notes = append(notes, fmt.Sprintf(
							"%s: an added line joins `{` and a statement on one line where the original kept them on separate lines", path))
						flush()
						continue
					}
				}
			}
			if removedMaxSemis <= 1 && strings.Count(body, ";") >= 2 {
				notes = append(notes, fmt.Sprintf(
					"%s: an added line merges multiple `;`-terminated statements the original kept on separate lines", path))
				flush()
			}
		}
	}
	if len(notes) > 3 {
		notes = notes[:3]
	}
	return notes
}

// patchStyleAdvisoryNote composes the bounded compat-style note for a
// whole plan, empty when nothing was flagged.
func patchStyleAdvisoryNote(changes []types.FileChange) string {
	var all []string
	for i := range changes {
		if strings.TrimSpace(changes[i].Patch) == "" {
			continue
		}
		all = append(all, analyzePatchLineCompression(changes[i].Path, changes[i].Patch)...)
	}
	if len(all) == 0 {
		return ""
	}
	if len(all) > 4 {
		all = all[:4]
	}
	return "\n[style: " + strings.Join(all, "; ") +
		" — the plan is accepted as-is; re-emit with the original line structure if you want a cleaner diff]"
}
