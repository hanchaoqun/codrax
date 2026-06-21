package tool

import (
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func sourceInventoryConstructSurfaceTerms(sym *repotypes.Symbol) []string {
	kind := sourceInventoryConstructKindSurface(sym)
	name := strings.TrimSpace(sym.Name)
	if kind == "" || name == "" {
		return nil
	}
	doc := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(sym.Doc))), " ")
	switch {
	case kind == "extend" || kind == "foreign func" || kind == "operator":
		return []string{kind, kind + " " + name}
	case sym.Exported && sourceInventoryConstructKindIsType(kind) &&
		(strings.Contains(" "+doc+" ", " public ") || strings.Contains(" "+doc+" ", " open ")):
		return []string{strings.TrimSpace(doc + " " + kind), strings.TrimSpace(doc + " " + kind + " " + name)}
	default:
		return nil
	}
}

func sourceInventoryConstructKindSurface(sym *repotypes.Symbol) string {
	if sym == nil {
		return ""
	}
	return strings.Join(strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(strings.ToLower(strings.TrimSpace(sym.Kind)))), " ")
}

func sourceInventoryConstructKindIsType(kind string) bool {
	switch kind {
	case "class", "struct", "interface", "enum":
		return true
	default:
		return false
	}
}

func sourceInventoryAppendCandidateNote(note, extra string) string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return note
	}
	if note = strings.TrimSpace(note); note == "" {
		return extra
	}
	if strings.Contains(note, extra) {
		return note
	}
	return note + "; " + extra
}

func sourceInventoryDedupSurfaceTerms(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		term := strings.TrimSpace(raw)
		key := strings.ToLower(term)
		if term != "" && !seen[key] {
			seen[key] = true
			out = append(out, term)
		}
	}
	return out
}
