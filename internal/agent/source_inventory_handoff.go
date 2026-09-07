package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The caller selects the mechanical landing lane using its existing shared
// snapshot predicate. This function only publishes that snapshot's facts.
func (e *explorerEvaluator) renderMechanicalSourceInventoryHandoff(ctx *types.AgentContext) string {
	view := types.BuildSourceInventoryAnswerAuthorityView(e.sourceInventoryAuthoritySnapshot(ctx))
	var b strings.Builder
	b.WriteString("## Mechanical source inventory handoff\n\n")
	b.WriteString("The following typed declaration rows are information for your structured completion, not final answer text. They support member identity, location, role, construct markers, counts, languages, source classes, and row-local attributes; they do not establish implementation behavior or control-flow.\n")
	b.WriteString("Preserve row-local construct families and source locations: the same name can identify different declarations, and one declaration can carry several independent markers. Prompt omissions below do not mean those rows or attributes are absent.\n\n")
	renderSourceInventoryAuthorityView(&b, view)
	return b.String()
}

// This renderer consumes the existing authority view without recomputing
// eligibility, inventory membership, or answer conclusions. Explore and
// finalize use the same row fields and explicit prompt-omission boundaries.
func renderSourceInventoryAuthorityView(b *strings.Builder, view types.SourceInventoryAnswerAuthorityView) {
	if b == nil || view.PrincipalTotal <= 0 {
		return
	}
	if len(view.PrincipalRoles) > 0 {
		roles := make([]string, 0, len(view.PrincipalRoles))
		for _, role := range view.PrincipalRoles {
			roles = append(roles, string(role))
		}
		fmt.Fprintf(b, "- principal_roles: `%s`\n", strings.Join(roles, "`, `"))
	}
	if view.PrincipalScope != "" {
		fmt.Fprintf(b, "- principal_scope: `%s`", view.PrincipalScope)
		if view.RepoWidePrincipal {
			b.WriteString(" (repo-wide typed inventory)")
		}
		b.WriteByte('\n')
	}
	if len(view.SourceClasses) > 0 {
		parts := make([]string, 0, len(view.SourceClasses))
		for _, class := range view.SourceClasses {
			parts = append(parts, fmt.Sprintf("%s:%d", class.Role, class.Count))
		}
		fmt.Fprintf(b, "- source_classes: %s\n", strings.Join(parts, ", "))
	}
	if view.RequestedUniverse.Active {
		renderSourceInventoryRequestedUniverse(b, view.RequestedUniverse)
	}
	fmt.Fprintf(b, "- row_lanes: principal=%d", view.PrincipalTotal)
	if view.PrincipalHiddenCount > 0 {
		fmt.Fprintf(b, " (+%d hidden)", view.PrincipalHiddenCount)
	}
	fmt.Fprintf(b, ", support=%d", view.SupportTotal)
	if view.SupportHiddenCount > 0 {
		fmt.Fprintf(b, " (+%d hidden)", view.SupportHiddenCount)
	}
	fmt.Fprintf(b, ", audit=%d", view.AuditTotal)
	if view.AuditHiddenCount > 0 {
		fmt.Fprintf(b, " (+%d hidden)", view.AuditHiddenCount)
	}
	b.WriteString("\n\n")
	b.WriteString("### Principal candidate rows\n\n")
	b.WriteString("These rows are selected from the full typed observation before prompt row limits, balanced by role/source-class/language family. Treat them as the candidate universe handoff, not as final prose.\n\n")
	renderSourceInventoryRows(b, view.PrincipalRows, false)
	if view.PrincipalHiddenCount > 0 {
		fmt.Fprintf(b, "- ... %d additional principal row(s) are preserved in the typed observation but omitted from this bounded prompt view.\n", view.PrincipalHiddenCount)
	}
	b.WriteString("\n")
	if view.SupportTotal > 0 {
		b.WriteString("### Support/navigation rows\n\n")
		b.WriteString("These rows are outside the principal scope or lack direct source location; use them for navigation or qualification only, not as replacement answer members.\n\n")
		renderSourceInventoryRows(b, view.SupportRows, true)
		if view.SupportHiddenCount > 0 {
			fmt.Fprintf(b, "- ... %d additional support row(s) omitted from this bounded prompt view.\n", view.SupportHiddenCount)
		}
		b.WriteString("\n")
	}
	if view.AuditTotal > 0 {
		b.WriteString("### Audit-only rows\n\n")
		b.WriteString("These rows are non-principal roles under the current typed source-inventory profile. Keep them out of the answer slate unless another typed carrier makes them principal.\n\n")
		renderSourceInventoryRows(b, view.AuditRows, true)
		if view.AuditHiddenCount > 0 {
			fmt.Fprintf(b, "- ... %d additional audit row(s) omitted from this bounded prompt view.\n", view.AuditHiddenCount)
		}
		b.WriteString("\n")
	}
}

func renderSourceInventoryRequestedUniverse(b *strings.Builder, universe types.SourceInventoryRequestedUniverseView) {
	if b == nil || !universe.Active {
		return
	}
	var parts []string
	if len(universe.Languages) > 0 {
		parts = append(parts, "languages=`"+strings.Join(universe.Languages, "`, `")+"`")
	}
	if len(universe.SourceClasses) > 0 {
		classes := make([]string, 0, len(universe.SourceClasses))
		for _, class := range universe.SourceClasses {
			classes = append(classes, string(class))
		}
		parts = append(parts, "source_classes=`"+strings.Join(classes, "`, `")+"`")
	}
	if len(universe.SurfaceFamilies) > 0 {
		parts = append(parts, "surface_families=`"+strings.Join(universe.SurfaceFamilies, "`, `")+"`")
	}
	if len(parts) > 0 {
		fmt.Fprintf(b, "- requested_universe: %s\n", strings.Join(parts, "; "))
	}
	if universe.InventoryOutOfUniverseRowsSuppressed > 0 {
		fmt.Fprintf(b, "- inventory_out_of_universe_rows_suppressed: %d", universe.InventoryOutOfUniverseRowsSuppressed)
		if len(universe.ReasonCodes) > 0 {
			fmt.Fprintf(b, " (`%s`)", strings.Join(universe.ReasonCodes, "`, `"))
		}
		b.WriteByte('\n')
	}
}

func renderSourceInventoryRows(b *strings.Builder, rows []types.SourceInventoryRow, includeReason bool) {
	for _, row := range rows {
		member := row.Member
		name := strings.TrimSpace(member.Name)
		if name == "" {
			name = strings.TrimSpace(member.Key)
		}
		if name == "" {
			continue
		}
		fmt.Fprintf(b, "- member=`%s`, role=%s", name, row.Role)
		if row.SourceClass != "" {
			fmt.Fprintf(b, ", source_class=%s", row.SourceClass)
		}
		if lang := strings.TrimSpace(row.Language); lang != "" {
			fmt.Fprintf(b, ", language=%s", lang)
		}
		if family := strings.TrimSpace(row.SurfaceFamily); family != "" {
			fmt.Fprintf(b, ", surface_family=`%s`", family)
		}
		if families := types.SourceInventorySurfaceFamilyKeys(member.SurfaceTerms); len(families) > 0 {
			fmt.Fprintf(b, ", surface_families=%s", renderSourceInventorySurfaceFamilies(families))
		}
		if ref := strings.TrimSpace(member.SupportRef); ref != "" {
			fmt.Fprintf(b, ", support_ref=`%s`", ref)
		}
		if file := strings.TrimSpace(member.File); file != "" {
			if member.Line > 0 {
				fmt.Fprintf(b, ", location=`%s:%d`", file, member.Line)
			} else {
				fmt.Fprintf(b, ", location=`%s`", file)
			}
		}
		if state := strings.TrimSpace(string(member.CoverageState)); state != "" {
			fmt.Fprintf(b, ", coverage_state=%s", state)
		}
		if attrs := renderSourceInventoryRowAttributes(member.Attributes); attrs != "" {
			fmt.Fprintf(b, ", attributes=%s", attrs)
		}
		if includeReason && strings.TrimSpace(row.ReasonCode) != "" {
			fmt.Fprintf(b, ", lane=%s reason=%s", row.Lane, row.ReasonCode)
		}
		b.WriteByte('\n')
	}
}

// Bound only the newly displayed family list, never the underlying row or its
// membership. The existing primary SurfaceFamily remains a separate field.
func renderSourceInventorySurfaceFamilies(families []string) string {
	const maxFamilies = 8
	const maxFamilyRunes = 96
	parts := make([]string, 0, min(len(families), maxFamilies)+1)
	for _, family := range families[:min(len(families), maxFamilies)] {
		runes := []rune(family)
		if len(runes) > maxFamilyRunes {
			parts = append(parts, fmt.Sprintf("`%s…` (+%d characters omitted)", string(runes[:maxFamilyRunes]), len(runes)-maxFamilyRunes))
		} else {
			parts = append(parts, "`"+family+"`")
		}
	}
	if len(families) > maxFamilies {
		parts = append(parts, fmt.Sprintf("+%d families omitted", len(families)-maxFamilies))
	}
	return strings.Join(parts, ", ")
}

func renderSourceInventoryRowAttributes(attrs []types.SourceInventoryObservationAttribute) string {
	const maxAttrs = 4
	if len(attrs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		name := strings.TrimSpace(attr.Name)
		if name == "" {
			name = strings.TrimSpace(attr.Key)
		}
		if name == "" {
			continue
		}
		role := strings.TrimSpace(string(attr.Role))
		if role == "" {
			role = "attribute"
		}
		item := role + ":" + name
		if loc := sourceInventoryAttributeLocation(attr); loc != "" {
			item += " @ " + loc
		}
		if attr.Ambiguity != "" {
			item += " ambiguity=" + strings.TrimSpace(attr.Ambiguity)
		}
		parts = append(parts, "`"+strings.ReplaceAll(item, "`", "'")+"`")
		if len(parts) >= maxAttrs {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if hidden := len(attrs) - len(parts); hidden > 0 {
		parts = append(parts, fmt.Sprintf("+%d more", hidden))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func sourceInventoryAttributeLocation(attr types.SourceInventoryObservationAttribute) string {
	if _, loc, ok := types.ParseAnswerSupportRefMemberLocation(attr.SupportRef); ok {
		file := strings.TrimSpace(strings.ReplaceAll(loc.File, `\`, `/`))
		if file != "" && loc.LineStart > 0 {
			return fmt.Sprintf("%s:%d", file, loc.LineStart)
		}
	}
	file := strings.TrimSpace(strings.ReplaceAll(attr.File, `\`, `/`))
	if file == "" || attr.Line <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", file, attr.Line)
}
