package types

import (
	"path/filepath"
	"sort"
	"strings"
)

// patch_effect_line_remap.go — V5-1 (colleague_merge_audit §40.10 item 2):
// binding a pre-apply `file:line` evidence reference to the post-apply file
// through the applied PatchEffect hunk tables. A source-text witness may be
// read only at a MAPPED line; every other outcome is a typed disclosure and
// never a satisfied observation. Pure function over typed hunk facts — no
// prose, no fuzzy matching.

// PatchEffectLineRemapStatus is the closed outcome set of RemapPatchEffectOldLine.
type PatchEffectLineRemapStatus string

const (
	// PatchEffectLineMapped: the old line survived the patch; Line is its
	// post-apply number and Path the post-apply path.
	PatchEffectLineMapped PatchEffectLineRemapStatus = "mapped"
	// PatchEffectLineRemovedByPatch: the old line is a removed ("-") line.
	PatchEffectLineRemovedByPatch PatchEffectLineRemapStatus = "removed_by_patch"
	// PatchEffectLineFileNotInPatch: the applied diff did not touch the file;
	// the caller decides whether an identity mapping is safe.
	PatchEffectLineFileNotInPatch PatchEffectLineRemapStatus = "file_not_in_patch"
	// PatchEffectLineFileCreated: the file did not exist before the patch, so
	// a pre-apply line reference cannot name anything.
	PatchEffectLineFileCreated PatchEffectLineRemapStatus = "file_created"
	// PatchEffectLineFileDeleted: the file no longer exists.
	PatchEffectLineFileDeleted PatchEffectLineRemapStatus = "file_deleted"
	// PatchEffectLineHunkUnmapped: the line falls inside a hunk whose per-line
	// tables are incomplete, so its survival cannot be decided precisely.
	PatchEffectLineHunkUnmapped PatchEffectLineRemapStatus = "hunk_unmapped"
	// PatchEffectLineNoPatchEffect: no PatchEffect record is attached.
	PatchEffectLineNoPatchEffect PatchEffectLineRemapStatus = "no_patch_effect"
	// PatchEffectLineInvalid: a non-positive line or empty path.
	PatchEffectLineInvalid PatchEffectLineRemapStatus = "invalid_line"
	// PatchEffectLineBaseDivergent: the file was dirty in the checkout the
	// evidence was taken from, so no base tree holds its numbering.
	PatchEffectLineBaseDivergent PatchEffectLineRemapStatus = "base_divergent"
)

// PatchEffectLineRemap is the typed result of binding one old line.
type PatchEffectLineRemap struct {
	Path   string
	Line   int
	Status PatchEffectLineRemapStatus
}

// Mapped reports whether the line was bound to a post-apply line.
func (r PatchEffectLineRemap) Mapped() bool {
	return r.Status == PatchEffectLineMapped && r.Line > 0 && strings.TrimSpace(r.Path) != ""
}

// RemapPatchEffectOldLine binds the pre-apply line `oldLine` of `path` to its
// post-apply position using the hunk tables of `effect`.
func RemapPatchEffectOldLine(effect *PatchEffectRecord, path string, oldLine int) PatchEffectLineRemap {
	want := patchEffectRemapPath(path)
	if oldLine <= 0 || want == "" {
		return PatchEffectLineRemap{Status: PatchEffectLineInvalid}
	}
	if effect == nil {
		return PatchEffectLineRemap{Status: PatchEffectLineNoPatchEffect}
	}
	var file *PatchEffectFile
	for i := range effect.Files {
		candidate := &effect.Files[i]
		if patchEffectRemapPath(candidate.OldPath) == want || patchEffectRemapPath(candidate.Path) == want {
			file = candidate
			break
		}
	}
	if file == nil {
		return PatchEffectLineRemap{Path: want, Line: oldLine, Status: PatchEffectLineFileNotInPatch}
	}
	newPath := patchEffectRemapPath(file.Path)
	if newPath == "" {
		newPath = patchEffectRemapPath(file.OldPath)
	}
	switch strings.TrimSpace(file.Status) {
	case "created":
		return PatchEffectLineRemap{Path: newPath, Status: PatchEffectLineFileCreated}
	case "deleted":
		return PatchEffectLineRemap{Status: PatchEffectLineFileDeleted}
	}
	if file.Binary {
		// Content changed without a line table: nothing can be located.
		return PatchEffectLineRemap{Path: newPath, Status: PatchEffectLineHunkUnmapped}
	}
	hunks := append([]PatchEffectHunk(nil), file.Hunks...)
	sort.SliceStable(hunks, func(i, j int) bool { return hunks[i].OldStart < hunks[j].OldStart })
	shift := 0
	for _, hunk := range hunks {
		if hunk.OldLines > 0 && oldLine >= hunk.OldStart && oldLine < hunk.OldStart+hunk.OldLines {
			return patchEffectRemapInsideHunk(hunk, oldLine, newPath)
		}
		// A zero-length old range ("-L,0") is an insertion AFTER old line L:
		// line L itself is untouched, lines beyond it shift.
		before := oldLine >= hunk.OldStart+hunk.OldLines
		if hunk.OldLines == 0 {
			before = oldLine > hunk.OldStart
		}
		if !before {
			break
		}
		shift += hunk.NewLines - hunk.OldLines
	}
	return PatchEffectLineRemap{Path: newPath, Line: oldLine + shift, Status: PatchEffectLineMapped}
}

// patchEffectRemapInsideHunk decides a line that falls inside a hunk's old
// range: a removed line is gone; a context line is located by its ordinal
// among the hunk's kept old lines, matched against the kept new lines. The
// decision needs complete per-line tables (every removed line numbered, every
// added line numbered) — otherwise the line stays unmapped.
func patchEffectRemapInsideHunk(hunk PatchEffectHunk, oldLine int, newPath string) PatchEffectLineRemap {
	complete := len(hunk.RemovedLineTexts) == hunk.RemovedLines &&
		len(hunk.AddedLineNumbers) == hunk.AddedLines &&
		hunk.OldLines-hunk.RemovedLines == hunk.NewLines-hunk.AddedLines
	removedBefore := 0
	for _, removed := range hunk.RemovedLineTexts {
		if removed.Line <= 0 {
			complete = false
			continue
		}
		if removed.Line == oldLine {
			return PatchEffectLineRemap{Path: newPath, Status: PatchEffectLineRemovedByPatch}
		}
		if removed.Line < oldLine {
			removedBefore++
		}
	}
	if !complete {
		return PatchEffectLineRemap{Path: newPath, Status: PatchEffectLineHunkUnmapped}
	}
	added := make(map[int]bool, len(hunk.AddedLineNumbers))
	for _, line := range hunk.AddedLineNumbers {
		if line <= 0 {
			return PatchEffectLineRemap{Path: newPath, Status: PatchEffectLineHunkUnmapped}
		}
		added[line] = true
	}
	ordinal := (oldLine - hunk.OldStart) - removedBefore
	kept := 0
	for line := hunk.NewStart; line < hunk.NewStart+hunk.NewLines; line++ {
		if added[line] {
			continue
		}
		if kept == ordinal {
			return PatchEffectLineRemap{Path: newPath, Line: line, Status: PatchEffectLineMapped}
		}
		kept++
	}
	return PatchEffectLineRemap{Path: newPath, Status: PatchEffectLineHunkUnmapped}
}

func patchEffectRemapPath(raw string) string {
	path := filepath.ToSlash(strings.TrimSpace(raw))
	path = strings.TrimPrefix(path, "./")
	if path == "." || path == "/dev/null" {
		return ""
	}
	return path
}
