package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// preEmitCitationSameExtent is citation transport identity, not source proof.
// Sharing a start line does not make a point, a range and a section the same
// selection. Legacy line scopes may carry an explicit end; preserve that end
// without rewriting the model's pool entry, quote or display metadata.
func preEmitCitationSameExtent(a, b types.Citation) bool {
	endA, lineA := preEmitCitationLineExtent(a)
	endB, lineB := preEmitCitationLineExtent(b)
	if lineA && lineB {
		return a.Line == b.Line && endA == endB &&
			preEmitPathMatches(a.File, b.File) &&
			strings.TrimSpace(a.SectionPath) == strings.TrimSpace(b.SectionPath) &&
			a.FileRoleLabel == b.FileRoleLabel &&
			strings.TrimSpace(a.CrossfileSummary) == strings.TrimSpace(b.CrossfileSummary) &&
			strings.TrimSpace(a.NegativePattern) == strings.TrimSpace(b.NegativePattern)
	}
	return equivalentAnswerCitation(a, b)
}

func preEmitCitationLineExtent(cit types.Citation) (int, bool) {
	if strings.TrimSpace(cit.File) == "" || cit.Line <= 0 || (cit.LineEnd > 0 && cit.LineEnd < cit.Line) {
		return 0, false
	}
	switch cit.Scope {
	case "", types.ScopeLine, types.ScopeLineRange:
		if cit.LineEnd > cit.Line {
			return cit.LineEnd, true
		}
		return cit.Line, true
	default:
		return 0, false
	}
}

// findPreEmitCitation prefers an exact file identity. A legacy suffix alias is
// usable only if the pool contains one matching source, never the first of two
// same-named files. Existing entries and their indexes are always left intact.
func findPreEmitCitation(pool []types.Citation, cit types.Citation) int {
	file := preEmitCitationPoolFile(cit.File)
	if file == "" {
		return -1
	}
	exactFilePresent := false
	for i, existing := range pool {
		if preEmitCitationPoolFile(existing.File) == file {
			exactFilePresent = true
			if preEmitCitationSameExtent(existing, cit) {
				return i
			}
		}
	}
	// A known exact source with a different extent must not borrow a range
	// from a same-named sibling through the legacy path alias fallback.
	if exactFilePresent {
		return -1
	}
	matched, matchedFile := -1, ""
	for i, existing := range pool {
		if !preEmitCitationSameExtent(existing, cit) {
			continue
		}
		candidateFile := preEmitCitationPoolFile(existing.File)
		if matched >= 0 && candidateFile != matchedFile {
			return -1
		}
		if matched < 0 {
			matched, matchedFile = i, candidateFile
		}
	}
	return matched
}

func preEmitCitationPoolFile(file string) string {
	return strings.TrimPrefix(normalizePatchCitationFile(file), "./")
}
