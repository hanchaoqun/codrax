package repomap

import (
	"fmt"
	"strings"
)

// normalizeRepoMapSourceInventoryScopesForSelectedPath separates the selected
// graph's execution coordinate from the repository-root coordinate the model
// may redundantly repeat in scope/scopes. The repair is exact path-prefix
// arithmetic; it does not inspect request prose or infer a scope from names.
func normalizeRepoMapSourceInventoryScopesForSelectedPath(p *repoMapParams, coordinateRoot, resolvedRoot string) []string {
	if p == nil || p.View != "source_inventory" || strings.TrimSpace(coordinateRoot) == "" || strings.TrimSpace(resolvedRoot) == "" {
		return nil
	}
	selected, ok := repoMapRelPathWithinRoot(coordinateRoot, resolvedRoot)
	selected = strings.Trim(strings.ReplaceAll(strings.TrimSpace(selected), `\`, `/`), "/")
	if !ok || selected == "" || selected == "." {
		return nil
	}
	var advisories []string
	normalize := func(field, raw string) string {
		value := strings.TrimPrefix(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)), "./")
		value = strings.Trim(value, "/")
		next := value
		switch {
		case value == selected:
			next = "."
		case strings.HasPrefix(value, selected+"/"):
			next = strings.TrimPrefix(value, selected+"/")
		}
		if next != value {
			advisories = append(advisories, fmt.Sprintf("%s: `%s` -> `%s`", field, value, next))
		}
		return next
	}
	p.Scope = normalize("scope", p.Scope)
	for i := range p.Scopes {
		p.Scopes[i] = normalize(fmt.Sprintf("scopes[%d]", i), p.Scopes[i])
	}
	return advisories
}

func prependRepoMapSourceInventoryCoordinateAdvisory(output string, advisories []string) string {
	if len(advisories) == 0 {
		return output
	}
	var b strings.Builder
	b.WriteString("## Source inventory coordinate advisory\n\n")
	b.WriteString("`scope`/`scopes` are relative to the selected `path`; repo_map removed an exactly repeated repository-root path prefix while preserving that root coordinate in typed query provenance:\n")
	for _, advisory := range advisories {
		b.WriteString("- ")
		b.WriteString(advisory)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String() + output
}
