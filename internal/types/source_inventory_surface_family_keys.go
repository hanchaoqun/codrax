package types

import "strings"

// SourceInventorySurfaceFamilyKey returns the first typed row-local surface
// family. Call SourceInventorySurfaceFamilyKeys when independent marker
// families on the same row must remain distinguishable.
func SourceInventorySurfaceFamilyKey(terms []string) string {
	families := SourceInventorySurfaceFamilyKeys(terms)
	if len(families) > 0 {
		return families[0]
	}
	return ""
}

// SourceInventorySurfaceFamilyKeys derives every independent construct family
// from parser/lens SurfaceTerms only. It never reads user or model prose.
func SourceInventorySurfaceFamilyKeys(terms []string) []string {
	keys := SourceInventorySurfaceTermKeys(terms)
	seen := map[string]bool{}
	var out []string
	add := func(key string) {
		key = SourceInventorySurfaceTermKey(key)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, key)
	}
	// Exact parser markers precede symbol-derived declaration families.
	for _, key := range keys {
		if strings.HasPrefix(key, "@") && !strings.Contains(key, " ") {
			marker := key
			if idx := strings.IndexAny(marker, "([{"); idx > 0 {
				marker = marker[:idx]
			}
			add(marker)
		}
	}
	for _, candidate := range keys {
		for _, other := range keys {
			if other != candidate && strings.HasPrefix(other, candidate+" ") {
				add(candidate)
				break
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	// Preserve the historical conservative fallback for one structured phrase.
	for _, key := range keys {
		if idx := strings.LastIndex(key, " "); idx > 0 {
			add(strings.TrimSpace(key[:idx]))
			break
		}
	}
	return out
}

func SourceInventorySurfaceTermKeys(terms []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, term := range terms {
		key := SourceInventorySurfaceTermKey(term)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func SourceInventorySurfaceTermKey(term string) string {
	key := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(strings.ReplaceAll(term, `\`, `/`))), " "))
	return strings.Trim(key, "` \t\r\n")
}
