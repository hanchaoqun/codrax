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
	paddedDoc := " " + doc + " "
	switch {
	case kind == "extend" || kind == "foreign func" || kind == "operator":
		return []string{kind, kind + " " + name}
	case sym.Exported && (kind == "class" || kind == "struct" || kind == "interface" || kind == "enum") &&
		(strings.Contains(paddedDoc, " public ") || strings.Contains(paddedDoc, " open ")):
		var terms []string
		hasPublic := strings.Contains(paddedDoc, " public ")
		if hasPublic {
			terms = append(terms, "public "+kind, "public "+kind+" "+name)
		}
		if strings.Contains(paddedDoc, " open ") && !hasPublic {
			terms = append(terms, "open "+kind, "open "+kind+" "+name)
		}
		terms = append(terms, strings.TrimSpace(doc+" "+kind), strings.TrimSpace(doc+" "+kind+" "+name))
		return sourceInventoryDedupSurfaceTerms(terms)
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

func sourceInventoryAppendCandidateNote(note, extra string) string {
	note, extra = strings.TrimSpace(note), strings.TrimSpace(extra)
	switch {
	case extra == "" || strings.Contains(note, extra):
		return note
	case note == "":
		return extra
	default:
		return note + "; " + extra
	}
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
