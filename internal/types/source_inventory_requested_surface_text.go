package types

import "strings"

func sourceInventoryRequestedSurfaceQuoteKeys(quotes []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, quote := range quotes {
		key := sourceInventoryRequestedSurfaceTextKey(quote)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func sourceInventoryRequestedSurfaceTextKey(raw string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(raw))), " ")
}
