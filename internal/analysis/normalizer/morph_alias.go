package normalizer

import "strings"

// MorphAliasCandidates returns bounded ASCII-only lexical candidates for a
// plain English surface. Candidates are advisory until a SymbolResolver confirms
// them against the repo symbol table.
func MorphAliasCandidates(surface string) []string {
	s := strings.TrimSpace(surface)
	if len(s) < 6 || !isASCIIAlphaString(s) {
		return nil
	}
	lower := strings.ToLower(s)
	seen := make(map[string]bool, 4)
	out := make([]string, 0, 4)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if len(candidate) < 4 || candidate == lower || !isASCIIAlphaString(candidate) || seen[candidate] {
			return
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	addStemForms := func(stem string) {
		add(stem)
		if !strings.HasSuffix(stem, "e") {
			add(stem + "e")
		}
		if hasDoubleFinalASCIIConsonant(stem) {
			add(stem[:len(stem)-1])
		}
	}
	switch {
	case strings.HasSuffix(lower, "er"):
		addStemForms(lower[:len(lower)-2])
	case strings.HasSuffix(lower, "or"):
		addStemForms(lower[:len(lower)-2])
	case strings.HasSuffix(lower, "ing"):
		addStemForms(lower[:len(lower)-3])
	case strings.HasSuffix(lower, "ed"):
		addStemForms(lower[:len(lower)-2])
	case strings.HasSuffix(lower, "ion"):
		addStemForms(lower[:len(lower)-3])
	}
	return out
}

func isASCIIAlphaString(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

func hasDoubleFinalASCIIConsonant(s string) bool {
	if len(s) < 2 || s[len(s)-1] != s[len(s)-2] {
		return false
	}
	switch s[len(s)-1] {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	default:
		return true
	}
}
