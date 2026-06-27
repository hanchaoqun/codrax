package types

import "strings"

func normalizeSourceInventoryFollowupCursor(d SourceInventoryFollowupDebt) SourceInventoryFollowupDebt {
	if d.ReasonCode == SourceInventoryFollowupDebtPagination {
		return d
	}
	d.Query.Cursor = ""
	d.Query.Offset = 0
	return d
}

func normalizeSourceInventoryFollowupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		value := strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`), "/")
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
