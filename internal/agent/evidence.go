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

// MergeEvidenceItems is the exported variant of the internal merger,
// added so the orchestrator can deduplicate BusContext.EvidenceItems
// on stage self-loops without duplicating the ID-based merge logic.
// See memory/project_applystage_dedup.md for context.
func MergeEvidenceItems(groups ...[]types.EvidenceItem) []types.EvidenceItem {
	return mergeEvidenceItems(groups...)
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

// MergeFlowFindings is the exported variant of the internal merger,
// symmetric with MergeEvidenceItems — used by the orchestrator for
// BusContext dedup on stage self-loops.
func MergeFlowFindings(groups ...[]types.FlowFindingDigest) []types.FlowFindingDigest {
	return mergeFlowFindings(groups...)
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

// rankEvidenceByRelevance scores and sorts evidence items by their
// relevance to the user's question. The ranking is question-aware,
// using entity overlap, evidence kind weight, source weight, and
// bridge detection. A diversity constraint limits items from the
// same (source, subject) to avoid redundancy.
func rankEvidenceByRelevance(question string, items []types.EvidenceItem, readFiles map[string]bool) []types.EvidenceItem {
	if len(items) == 0 {
		return items
	}
	entities := extractRankingEntities(question)
	if len(entities) == 0 {
		return items // no entities to rank by — preserve original order
	}

	type scored struct {
		item  types.EvidenceItem
		score float64
	}
	scored_items := make([]scored, 0, len(items))
	for _, item := range items {
		s := evidenceRelevanceScore(item, entities, readFiles)
		scored_items = append(scored_items, scored{item: item, score: s})
	}
	sort.SliceStable(scored_items, func(i, j int) bool {
		return scored_items[i].score > scored_items[j].score
	})

	// Diversity: same (source_file, subject) max 2 entries.
	type dedupKey struct{ source, subject string }
	counts := make(map[dedupKey]int)
	result := make([]types.EvidenceItem, 0, len(items))
	for _, si := range scored_items {
		key := dedupKey{si.item.Source, si.item.Subject}
		if counts[key] >= 2 {
			continue
		}
		counts[key]++
		result = append(result, si.item)
	}
	return result
}

func evidenceRelevanceScore(item types.EvidenceItem, entities []string, readFiles map[string]bool) float64 {
	// 1. Entity overlap: how many question entities appear in item fields.
	text := strings.ToLower(item.Subject + " " + item.Object + " " + item.Summary + " " + item.Predicate)
	overlap := 0
	for _, ent := range entities {
		if strings.Contains(text, ent) {
			overlap++
		}
	}
	entityScore := float64(overlap) / float64(len(entities))

	// 2. Kind weight: concrete facts are most valuable.
	kindWeight := 0.5
	switch item.Kind {
	case types.EvidenceConcrete:
		kindWeight = 1.0
	case types.EvidenceRegistration:
		kindWeight = 0.95
	case types.EvidenceRelationship:
		kindWeight = 0.8
	case types.EvidenceMechanism:
		kindWeight = 0.7
	case types.EvidenceConditional:
		kindWeight = 0.6
	case types.EvidenceAbsent:
		kindWeight = 0.55
	case types.EvidenceDataflowPath:
		kindWeight = 0.75
	case types.EvidenceUnresolved:
		kindWeight = 0.3
	case types.EvidenceTruncated:
		kindWeight = 0.1
	}

	// 3. Source weight: files the explorer read are more relevant.
	sourceWeight := 0.5
	if readFiles != nil && readFiles[item.Source] {
		sourceWeight = 1.0
	}

	// 4. Bridge bonus: if subject and object match different entities.
	bridgeBonus := 1.0
	if item.Subject != "" && item.Object != "" {
		subjectLower := strings.ToLower(item.Subject)
		objectLower := strings.ToLower(item.Object)
		subjectHit, objectHit := false, false
		for _, ent := range entities {
			if strings.Contains(subjectLower, ent) {
				subjectHit = true
			}
			if strings.Contains(objectLower, ent) {
				objectHit = true
			}
		}
		if subjectHit && objectHit {
			bridgeBonus = 2.0
		}
	}

	return entityScore * kindWeight * sourceWeight * bridgeBonus
}

// rankFindingsByRelevance scores and sorts dataflow findings by
// relevance to the user's question, preferring findings whose path
// nodes overlap with question entities and shorter, more specific chains.
func rankFindingsByRelevance(question string, findings []types.FlowFindingDigest) []types.FlowFindingDigest {
	if len(findings) == 0 {
		return findings
	}
	entities := extractRankingEntities(question)
	if len(entities) == 0 {
		return findings
	}

	type scored struct {
		finding types.FlowFindingDigest
		score   float64
	}
	scored_items := make([]scored, 0, len(findings))
	for _, f := range findings {
		s := findingRelevanceScore(f, entities)
		scored_items = append(scored_items, scored{finding: f, score: s})
	}
	sort.SliceStable(scored_items, func(i, j int) bool {
		return scored_items[i].score > scored_items[j].score
	})

	result := make([]types.FlowFindingDigest, 0, len(findings))
	for _, si := range scored_items {
		result = append(result, si.finding)
	}
	return result
}

func findingRelevanceScore(f types.FlowFindingDigest, entities []string) float64 {
	// 1. Path entity overlap.
	allText := strings.ToLower(strings.Join(f.Path, " ") + " " +
		strings.Join(f.Sources, " ") + " " + strings.Join(f.Sinks, " "))
	overlap := 0
	for _, ent := range entities {
		if strings.Contains(allText, ent) {
			overlap++
		}
	}
	if overlap == 0 {
		return 0.0 // completely irrelevant finding
	}
	entityScore := float64(overlap) / float64(len(entities))

	// 2. Chain brevity: shorter chains are more precise.
	brevity := 1.0
	if len(f.Path) > 1 {
		brevity = 1.0 / float64(len(f.Path))
	}

	// 3. Original confidence as a multiplier.
	confidence := f.Confidence
	if confidence <= 0 {
		confidence = 0.5
	}

	return entityScore * brevity * confidence
}

// extractRankingEntities extracts question entities for relevance
// scoring. More aggressive than extractQuestionEntities: also extracts
// plain alphanumeric words ≥4 chars (lowercased) to handle questions
// like "有多少个agent可以调用subagent?" where there are no CamelCase tokens.
func extractRankingEntities(question string) []string {
	seen := make(map[string]bool)
	var entities []string
	add := func(s string) {
		s = strings.ToLower(strings.Trim(s, "(){}[]?!.,;:'\""))
		if len(s) < 4 || seen[s] {
			return
		}
		seen[s] = true
		entities = append(entities, s)
	}

	// Backtick-quoted identifiers.
	rest := question
	for {
		start := strings.Index(rest, "`")
		if start < 0 {
			break
		}
		end := strings.Index(rest[start+1:], "`")
		if end < 0 {
			break
		}
		add(rest[start+1 : start+1+end])
		rest = rest[start+1+end+1:]
	}

	// All runs of [A-Za-z0-9_.] ≥ 4 chars — captures both CamelCase
	// and lowercase identifiers like "agent", "subagent".
	inIdent := false
	identStart := 0
	for i := 0; i <= len(question); i++ {
		var c byte
		if i < len(question) {
			c = question[i]
		}
		isIdent := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '.'
		if isIdent && !inIdent {
			identStart = i
			inIdent = true
		} else if !isIdent && inIdent {
			token := question[identStart:i]
			add(token)
			// Also add dotted parts: "Foo.Bar" → "Foo", "Bar"
			if strings.Contains(token, ".") {
				for _, part := range strings.Split(token, ".") {
					add(part)
				}
			}
			inIdent = false
		}
	}
	return entities
}

// DataflowIntent encodes how much dataflow analysis a question needs.
//
//   - IntentNone: no dataflow at all (current bool=false case).
//   - IntentLookup: single-hop / identity / enumeration. Lowering and
//     per-file evidence are useful, but the multi-hop buildFindings pass
//     would be wasted compute. Maps to dataflow.Options{SkipFindings:true}.
//   - IntentPropagate: multi-hop, "X flows to Y" / "value propagates
//     across files". Full pipeline.
//
// Three-level enum (T1.2) replaces the bool needsDataflowAnalysis. Avoids
// the high-recall low-precision trap of `strings.Contains` keyword
// matching against a single boolean. The resolution rule is structural,
// not query-specific:
//
//  1. Explicit propagation phrases ("propagate" / "flow" / "传播" /
//     "流向") → Propagate.
//  2. Otherwise, any keyword or evidence kind that the original
//     needsDataflowAnalysis would have triggered on → Lookup.
//  3. No triggers → None.
type DataflowIntent int

const (
	IntentNone DataflowIntent = iota
	IntentLookup
	IntentPropagate
)

func (d DataflowIntent) String() string {
	switch d {
	case IntentNone:
		return "none"
	case IntentLookup:
		return "lookup"
	case IntentPropagate:
		return "propagate"
	}
	return "unknown"
}

// propagateKeywords are phrases that explicitly indicate the user is
// asking about cross-file value propagation, not single-hop identity or
// enumeration. Kept tight on purpose: anything not on this list falls
// through to Lookup.
var propagateKeywords = []string{
	// English
	"flow", "flows", "propagate", "propagates", "through",
	"where does", "call chain",
	// Chinese
	"传播", "流向", "怎么到", "如何到", "如何传",
}

// lookupKeywords are phrases that the original needsDataflowAnalysis
// triggered on but are single-hop in nature (registration, identity,
// enumeration, configuration lookup). Listed verbatim from the legacy
// table minus the propagate items above.
var lookupKeywords = []string{
	// English
	"path", "trigger",
	"which value", "what value", "who gets", "who is",
	"condition", "configured", "config", "registered", "route", "handler",
	"invoke", "dispatch", "how many",
	// Chinese
	"调用", "注册", "触发", "配置", "条件",
	"路由", "处理器", "绑定", "分发", "哪些", "多少", "列出",
	"哪个", "谁会", "谁能", "怎么",
}

func dataflowIntent(question string, items []types.EvidenceItem) DataflowIntent {
	lower := strings.ToLower(question)
	for _, needle := range propagateKeywords {
		if strings.Contains(lower, needle) {
			return IntentPropagate
		}
	}
	for _, needle := range lookupKeywords {
		if strings.Contains(lower, needle) {
			return IntentLookup
		}
	}
	for _, item := range items {
		switch item.Kind {
		case types.EvidenceConditional, types.EvidenceRelationship, types.EvidenceMechanism, types.EvidenceRegistration:
			return IntentLookup
		}
	}
	return IntentNone
}

// needsDataflowAnalysis is a backward-compatible wrapper around
// dataflowIntent. Returns true for both Lookup and Propagate. Kept so
// sub_explorer and existing tests don't have to change.
func needsDataflowAnalysis(question string, items []types.EvidenceItem) bool {
	return dataflowIntent(question, items) != IntentNone
}
