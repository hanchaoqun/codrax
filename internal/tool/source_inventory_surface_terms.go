package tool

import (
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryCandidateForSymbol(sym *repotypes.Symbol, role types.AnswerCandidateRole, graph *repotypes.Graph) sourceInventoryCandidate {
	language := sourceInventoryGraphLanguageForFile(graph, sym.File)
	key := aggregateMemberKey(sym.Name)
	if role == types.AnswerCandidateRoleRoute {
		// Route names are "<VERB> <path>"; key routes on the full name so the
		// shared symbol-tail canonicaliser does not collapse every GET route.
		key = strings.ToLower(strings.Join(strings.Fields(sym.Name), " "))
	}
	note, surfaceTerms := sourceInventoryCandidateNoteAndSurfaceTermsFromGraph(sym, language)
	return sourceInventoryCandidate{
		member:       strings.TrimSpace(sym.Name),
		key:          key,
		supportRef:   strings.TrimSpace(sym.Name) + ": " + aggregateSupportLocationKey(sym.File, sym.Line),
		note:         note,
		surfaceTerms: surfaceTerms,
		role:         role,
		exported:     sym.Exported,
		file:         sym.File,
		line:         sym.Line,
		language:     language,
	}
}

func sourceInventoryCandidateNote(candidate sourceInventoryCandidate) string {
	return sourceInventoryCompactNote(candidate.note)
}

func sourceInventoryCandidateSurfaceTerms(candidate sourceInventoryCandidate) []string {
	if len(candidate.surfaceTerms) == 0 {
		return nil
	}
	out := make([]string, 0, len(candidate.surfaceTerms))
	seen := map[string]bool{}
	for _, raw := range candidate.surfaceTerms {
		term := strings.TrimSpace(raw)
		if term == "" {
			continue
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, term)
	}
	return out
}

func sourceInventoryCandidateNoteFromGraph(sym *repotypes.Symbol, language string) string {
	note, _ := sourceInventoryCandidateNoteAndSurfaceTermsFromGraph(sym, language)
	return note
}

func sourceInventoryCandidateNoteAndSurfaceTermsFromGraph(sym *repotypes.Symbol, language string) (string, []string) {
	if sym == nil {
		return "", nil
	}
	note := sourceInventoryCompactNote(sym.Doc)
	if terms := sourceInventorySurfaceTermsFromGraphNote(note); len(terms) > 0 {
		return "surface=" + strings.Join(terms, " "), terms
	}
	if !sourceInventoryGraphDocDescribesSymbol(note, sym.Name, language) {
		return "", nil
	}
	return note, nil
}

func sourceInventorySurfaceTermsFromGraphNote(note string) []string {
	note = strings.TrimSpace(note)
	if !strings.HasPrefix(note, "@") {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, raw := range strings.Fields(note) {
		term := strings.Trim(raw, " \t\r\n,;")
		if !strings.HasPrefix(term, "@") {
			continue
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, term)
	}
	return out
}
