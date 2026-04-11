package agent

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func parseEvidenceItems(notes []string, producer string) []types.EvidenceItem {
	var items []types.EvidenceItem
	for _, note := range notes {
		source := ""
		for _, rawLine := range strings.Split(note, "\n") {
			line := strings.TrimSpace(rawLine)
			if strings.HasPrefix(line, "## Evidence from ") {
				source = parseEvidenceHeaderSource(line)
				continue
			}
			item, ok := parseEvidenceLine(line, source, producer)
			if ok {
				items = append(items, item)
			}
		}
	}
	return mergeEvidenceItems(items)
}

func parseEvidenceHeaderSource(line string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "## Evidence from "))
	rest = strings.Trim(rest, "[]")
	if idx := strings.Index(rest, ":"); idx > 0 && looksLikePath(rest[:idx]) {
		return rest[:idx]
	}
	if looksLikePath(rest) {
		return rest
	}
	return ""
}

func parseEvidenceLine(line, source, producer string) (types.EvidenceItem, bool) {
	if !strings.HasPrefix(line, "- [") {
		return types.EvidenceItem{}, false
	}
	close := strings.Index(line, "]")
	if close < 3 {
		return types.EvidenceItem{}, false
	}
	tag := strings.TrimSpace(line[3:close])
	rest := strings.TrimSpace(line[close+1:])
	if rest == "" {
		return types.EvidenceItem{}, false
	}

	kind := evidenceKindFromTag(tag)
	lineStart, lineEnd := parseEvidenceLineRange(rest)
	subject, object := parseEvidenceSubjectObject(tag, rest)
	predicate := strings.ToLower(tag)
	condition := ""
	summary := rest

	if idx := strings.Index(rest, " IF "); idx >= 0 {
		condition = strings.TrimSpace(rest[idx+4:])
		rest = strings.TrimSpace(rest[:idx])
	}
	if colon := strings.Index(rest, ":"); colon >= 0 {
		object = strings.TrimSpace(rest[colon+1:])
		if subject == "" {
			subject = strings.TrimSpace(rest[:colon])
		}
	}
	if tag == "RELATIONSHIP" && predicate == "relationship" {
		predicate = "relates_to"
		if strings.Contains(rest, " calls ") {
			predicate = "calls"
		} else if strings.Contains(rest, " references ") {
			predicate = "references"
		} else if strings.Contains(rest, " uses ") {
			predicate = "uses"
		}
	}
	if tag == "ABSENT" {
		predicate = "absent"
		if subject == "" {
			subject = summary
		}
		object = ""
	}

	item := types.EvidenceItem{
		Kind:       kind,
		Subject:    strings.Trim(subject, "`"),
		Predicate:  predicate,
		Object:     strings.Trim(object, "`"),
		Summary:    summary,
		Condition:  condition,
		Source:     source,
		LineStart:  lineStart,
		LineEnd:    lineEnd,
		Confidence: 0.78,
		Producer:   producer,
	}
	item.ID = types.StableEvidenceID(item.Kind, item.Subject, item.Predicate, item.Object, item.Condition, item.Source, item.LineStart, item.LineEnd)
	return item, true
}

func parseEvidenceSubjectObject(tag, rest string) (string, string) {
	if tag == "RELATIONSHIP" {
		first, remainder := extractBacktickToken(rest)
		second, _ := extractBacktickToken(remainder)
		return first, second
	}
	subject, _ := extractBacktickToken(rest)
	return subject, ""
}

func extractBacktickToken(s string) (string, string) {
	start := strings.Index(s, "`")
	if start < 0 {
		return "", s
	}
	end := strings.Index(s[start+1:], "`")
	if end < 0 {
		return "", s
	}
	token := s[start+1 : start+1+end]
	remainder := s[start+1+end+1:]
	return token, remainder
}

func parseEvidenceLineRange(rest string) (int, int) {
	lineIdx := strings.Index(rest, " line ")
	if lineIdx < 0 {
		return 0, 0
	}
	after := rest[lineIdx+6:]
	end := 0
	for end < len(after) && after[end] >= '0' && after[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, 0
	}
	start, err := strconv.Atoi(after[:end])
	if err != nil {
		return 0, 0
	}
	return start, start
}

func evidenceKindFromTag(tag string) types.EvidenceKind {
	switch tag {
	case "DIRECT":
		return types.EvidenceDirect
	case "CONDITIONAL":
		return types.EvidenceConditional
	case "REGISTRATION":
		return types.EvidenceRegistration
	case "MECHANISM":
		return types.EvidenceMechanism
	case "RELATIONSHIP":
		return types.EvidenceRelationship
	case "ABSENT":
		return types.EvidenceAbsent
	default:
		return types.EvidenceDirect
	}
}

func mergeEvidenceItems(groups ...[]types.EvidenceItem) []types.EvidenceItem {
	merged := make(map[string]types.EvidenceItem)
	for _, group := range groups {
		for _, item := range group {
			if item.ID == "" {
				item.ID = types.StableEvidenceID(item.Kind, item.Subject, item.Predicate, item.Object, item.Condition, item.Source, item.LineStart, item.LineEnd)
			}
			if existing, ok := merged[item.ID]; ok {
				if existing.Summary == "" && item.Summary != "" {
					existing.Summary = item.Summary
				}
				if existing.Source == "" {
					existing.Source = item.Source
				}
				if existing.EvidenceRef == "" {
					existing.EvidenceRef = item.EvidenceRef
				}
				if item.Confidence > existing.Confidence {
					existing.Confidence = item.Confidence
				}
				existing.DerivedFrom = mergeStrings(existing.DerivedFrom, item.DerivedFrom)
				merged[item.ID] = existing
				continue
			}
			item.DerivedFrom = mergeStrings(item.DerivedFrom, nil)
			merged[item.ID] = item
		}
	}
	result := make([]types.EvidenceItem, 0, len(merged))
	for _, item := range merged {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Source != result[j].Source {
			return result[i].Source < result[j].Source
		}
		if result[i].LineStart != result[j].LineStart {
			return result[i].LineStart < result[j].LineStart
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func mergeFlowFindings(groups ...[]types.FlowFindingDigest) []types.FlowFindingDigest {
	merged := make(map[string]types.FlowFindingDigest)
	for _, group := range groups {
		for _, item := range group {
			if item.ID == "" {
				item.ID = types.StableFlowFindingID(item.Path, item.Conditions, item.Sources, item.Sinks)
			}
			if existing, ok := merged[item.ID]; ok {
				existing.Confidence = max(existing.Confidence, item.Confidence)
				existing.EvidenceIDs = mergeStrings(existing.EvidenceIDs, item.EvidenceIDs)
				if existing.UnsupportedReason == "" {
					existing.UnsupportedReason = item.UnsupportedReason
				}
				merged[item.ID] = existing
				continue
			}
			item.Path = mergeStrings(item.Path, nil)
			item.Conditions = mergeStrings(item.Conditions, nil)
			item.Sources = mergeStrings(item.Sources, nil)
			item.Sinks = mergeStrings(item.Sinks, nil)
			item.Hops = mergeStrings(item.Hops, nil)
			item.EvidenceIDs = mergeStrings(item.EvidenceIDs, nil)
			merged[item.ID] = item
		}
	}
	result := make([]types.FlowFindingDigest, 0, len(merged))
	for _, item := range merged {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Confidence > result[j].Confidence
	})
	return result
}

func mergeStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	var out []string
	for _, item := range append(append([]string{}, a...), b...) {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func buildCrossReferenceMapFromEvidence(items []types.EvidenceItem, findings []types.FlowFindingDigest) string {
	if len(items) == 0 && len(findings) == 0 {
		return ""
	}
	var lines []string
	for _, finding := range findings {
		if len(finding.Path) < 2 {
			continue
		}
		line := fmt.Sprintf("- **%s**", strings.Join(finding.Path, " -> "))
		if len(finding.Conditions) > 0 {
			line += " IF " + strings.Join(finding.Conditions, " AND ")
		}
		if finding.UnsupportedReason != "" {
			line += " [uncertain: " + finding.UnsupportedReason + "]"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Cross-References Between Evidence Sets\n\n")
	b.WriteString("These bridges are derived from structured evidence and dataflow findings:\n\n")
	for _, line := range lines {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func formatEvidenceSection(items []types.EvidenceItem, kind types.EvidenceKind, title string, limit int) string {
	var selected []types.EvidenceItem
	for _, item := range items {
		if item.Kind == kind {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		return ""
	}
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}
	var b strings.Builder
	b.WriteString("## " + title + "\n\n")
	for _, item := range selected {
		line := item.Summary
		if line == "" {
			line = fmt.Sprintf("%s %s %s", item.Subject, item.Predicate, item.Object)
		}
		b.WriteString("- " + line + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func formatFlowFindingsSection(findings []types.FlowFindingDigest, title string, limit int, includeUnsupported bool) string {
	if len(findings) == 0 {
		return ""
	}
	var selected []types.FlowFindingDigest
	for _, finding := range findings {
		if finding.UnsupportedReason != "" && !includeUnsupported {
			continue
		}
		if finding.UnsupportedReason == "" && includeUnsupported {
			continue
		}
		selected = append(selected, finding)
	}
	if len(selected) == 0 {
		return ""
	}
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}
	var b strings.Builder
	b.WriteString("## " + title + "\n\n")
	for _, finding := range selected {
		line := strings.Join(finding.Path, " -> ")
		if line == "" {
			line = finding.ID
		}
		if len(finding.Conditions) > 0 {
			line += " IF " + strings.Join(finding.Conditions, " AND ")
		}
		if finding.UnsupportedReason != "" {
			line += " [uncertain: " + finding.UnsupportedReason + "]"
		}
		b.WriteString("- " + line + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func needsDataflowAnalysis(question string, items []types.EvidenceItem) bool {
	lower := strings.ToLower(question)
	for _, needle := range []string{
		"flow", "flows", "path", "propagate", "through", "trigger",
		"which value", "what value", "where does", "who gets", "who is",
		"condition", "configured", "config", "registered", "route", "handler",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	for _, item := range items {
		switch item.Kind {
		case types.EvidenceConditional, types.EvidenceRelationship, types.EvidenceMechanism, types.EvidenceRegistration:
			return true
		}
	}
	return false
}
