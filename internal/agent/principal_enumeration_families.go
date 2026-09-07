package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This display projection uses the same complete typed family set as row
// rendering and explicit-family validation. Memberships may overlap; neither
// this count nor the prompt limit changes row identity, role, or eligibility.
// The caller has already selected/canonicalized its accepted principal rows.
func answerDocPrincipalEnumerationSurfaceFamilyCounts(sets []types.EnumerationDisplaySet) (string, int, int) {
	counts := map[string]int{}
	covered := 0
	total := 0
	for _, set := range sets {
		for _, row := range set.Rows {
			total++
			families := types.SourceInventorySurfaceFamilyKeys(row.SurfaceTerms)
			if len(families) == 0 {
				continue
			}
			covered++
			for _, family := range families {
				counts[family]++
			}
		}
	}
	if len(counts) == 0 {
		return "", covered, total
	}
	families := make([]string, 0, len(counts))
	for family := range counts {
		families = append(families, family)
	}
	sort.Strings(families)
	const maxCounts = 32
	shown := min(len(families), maxCounts)
	parts := make([]string, 0, shown+1)
	for _, family := range families[:shown] {
		parts = append(parts, fmt.Sprintf("%s:%d", renderSourceInventorySurfaceFamilies([]string{family}), counts[family]))
	}
	if len(families) > shown {
		parts = append(parts, fmt.Sprintf("+%d family counts omitted", len(families)-shown))
	}
	return strings.Join(parts, ", "), covered, total
}
