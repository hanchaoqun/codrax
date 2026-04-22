// Several functions in this file (parseEvidenceItems, groundEvidenceItems,
// mergeEvidenceItems, rankEvidenceByRelevance, scrubSiblingEvidenceBlocks)
// form filters F4..F7 and F9 of the post-hoc filtering pipeline. The
// required execution order is: parseEvidenceItems → groundEvidenceItems
// (F5 grounding) → mergeEvidenceItems (F6 structured + prose merge) →
// rankEvidenceByRelevance (F7) → scrubSiblingEvidenceBlocks (F9).
// Every filter is fail-open — a zero-survivor outcome passes the
// input set through unchanged rather than silently dropping evidence.
//
// parseEvidenceItems is no longer the only feeder of the Evidence
// channel. The structured emit_evidence tool (internal/tool/
// emit_evidence.go) is the primary channel; ensureStructuredEvidence
// in explorer.go merges its output with parseEvidenceItems via
// mergeEvidenceItems before grounding. parseEvidenceItems stays as
// a secondary channel for LLMs that write markdown blocks anyway,
// and is directly used by the explorer's S1 check and by
// sub_explorer which doesn't route through emit_evidence.
package agent

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/axis"
	"github.com/hanchaoqun/codrax/internal/analysis/subject"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Grounding helpers (ungroundedSuffix, gutterLineRe, identifierTokenRe,
// buildReadFileLineIndex, parseBannerPath, lookupLineWithNeighbours,
// lineCorroboratesEvidence, extractCodeIdentifiers, looksLikeCodeIdentifier,
// lastDotSegment, groundEvidenceItems) moved to internal/tool/ground/
// in the 2026-04-17 redesign. Grounding now runs synchronously inside
// emit_evidence.Execute with three-state GroundingStatus output
// (grounded / recovered / ungrounded), replacing the old post-hoc
// "/ungrounded" Producer suffix.

// pathTokenRe matches file-path-shaped substrings — two or more
// `word/word` segments where word chars include `_` and `.`. Captures
// e.g. `internal/agent/foo.go`, `pkg/handler/bar.py`,
// `src/components/Header.tsx`. Used by stripPathTokens to scrub
// locator metadata out of relevance-scoring text so package-layout
// substrings (e.g. `agent` inside `internal/agent/...`) cannot
// trivially match a question entity and dominate ranking.
var pathTokenRe = regexp.MustCompile(`[A-Za-z0-9_.]+(?:/[A-Za-z0-9_.]+)+`)

// stripPathTokens replaces every path-shaped substring in s with a
// single space. Tokens like `internal/agent/registry.go:Registry.Register`
// become ` :Registry.Register` — the path vanishes, the post-colon
// symbol name survives. Entity-overlap scoring must run against the
// stripped text so file-path locators are treated as metadata, not
// as matchable semantic content.
func stripPathTokens(s string) string {
	if !strings.Contains(s, "/") {
		return s
	}
	return pathTokenRe.ReplaceAllString(s, " ")
}

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
	// Strip both the legacy `[path]` wrapper the original format
	// expected and the markdown `` `path` `` wrapper LLMs emit when
	// they format the header as prose. A leaked wrapper turns the
	// parsed Source into a key that downstream consumers
	// (filterEvidenceByPrimaryFiles, grounder lineIndex lookup,
	// SynthesisPrompt scrub) cannot match against the repo path
	// set — causing silent over-filtering. See df1-20260414-011749
	// where 17/18 evidence items were wrongly ungrounded because
	// the LLM wrote `` ## Evidence from `internal/agent/x.go` ``
	// and the Source carried the backticks through.
	rest = strings.Trim(rest, "[]`")
	if idx := strings.Index(rest, ":"); idx > 0 && looksLikePath(rest[:idx]) {
		return rest[:idx]
	}
	if looksLikePath(rest) {
		return rest
	}
	return ""
}

func parseEvidenceLine(line, source, producer string) (types.EvidenceItem, bool) {
	line, ok := normalizeEvidenceLine(line)
	if !ok {
		return types.EvidenceItem{}, false
	}
	close := strings.Index(line, "]")
	if close < 3 {
		return types.EvidenceItem{}, false
	}
	tag := strings.TrimSpace(line[1:close])
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

func normalizeEvidenceLine(line string) (string, bool) {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "- ["):
		return strings.TrimSpace(strings.TrimPrefix(line, "-")), true
	case strings.HasPrefix(line, "* ["):
		return strings.TrimSpace(strings.TrimPrefix(line, "*")), true
	case strings.HasPrefix(line, "["):
		return line, true
	default:
		return "", false
	}
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
				// Merge Producer too: when two producers contribute to the
				// same item, prefer the question-relevant one (non-dataflow)
				// so the rank below still treats the merged item as LLM-
				// emitted. Without this, a dataflow.* item that happens to
				// collide with an LLM emit could downgrade the merged
				// entry's rank.
				if evidenceSortRank(existing) > evidenceSortRank(item) {
					existing.Producer = item.Producer
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
		// Rank first: question-relevant items (LLM emit_evidence,
		// concrete_values, bridge_literal, consumer_gate) come before
		// dataflow-derived items. Dataflow scans reachable files in
		// graph order and produces alphabetically-early sources like
		// cmd/root.go that would otherwise fill the downstream top-18
		// Structured Evidence slot before any on-topic file appears.
		// Within each rank, keep the historical (Source, LineStart,
		// ID) tiebreakers for deterministic output and test stability.
		ri, rj := evidenceSortRank(result[i]), evidenceSortRank(result[j])
		if ri != rj {
			return ri < rj
		}
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

// evidenceSortRank maps an item to a band used as the primary sort
// key. Rank 0 items are authored to answer the current question
// (LLM emit_evidence, explorer-side deterministic analysis that
// targets the specific entities/intents); rank 1 items are
// structural background the dataflow engine auto-harvested from
// whatever files happened to be in the candidate set.
//
// Promoting rank 0 keeps the Structured Evidence top-N rendering
// on-topic even when dataflow contributes thousands of bullets from
// alphabetically-early files (cmd/*, api/*) that the question does
// not ask about.
func evidenceSortRank(item types.EvidenceItem) int {
	if strings.HasPrefix(item.Producer, "dataflow.") {
		return 1
	}
	return 0
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

// rankEvidenceByRelevance scores and sorts evidence items by their
// relevance to the user's question. The ranking is question-aware,
// using entity overlap, evidence kind weight, source weight, and
// bridge detection. A diversity constraint limits items from the
// same (source, subject) to avoid redundancy.
func rankEvidenceByRelevance(question string, items []types.EvidenceItem, readFiles map[string]bool) []types.EvidenceItem {
	return rankEvidenceByRelevanceWithSubject(question, items, readFiles, types.AnswerSubject{}, nil, types.AxisUnknown)
}

// evidenceSubjectBoost scores how well an evidence item's answer
// token (Object field, falling back to the tail of Summary) matches
// the expected AnswerSubject kind. Delegates the per-kind judge to
// internal/analysis/subject so the evidence ranker and the chain
// ranker use the same scoring logic. Empty Object + empty Summary
// tail returns 0 (no boost applied).
func evidenceSubjectBoost(item types.EvidenceItem, expected types.AnswerSubject, graph *repomap.Graph) float64 {
	token := strings.TrimSpace(item.Object)
	if token == "" {
		// Summary often ends with the literal ("... returns
		// 'foo'"). Grab the last quoted substring as a fallback
		// token. Cheap heuristic; subject.Score tolerates misses.
		token = tailQuotedToken(item.Summary)
	}
	if token == "" {
		return 0
	}
	return subject.Score(token, expected.Kind, graph)
}

// tailQuotedToken returns the content of the LAST quoted substring in
// s (single or double quotes). Returns "" when no quoted token
// exists. Used as a conservative fallback when an evidence item's
// Object is empty but the Summary contains the literal in quotes.
func tailQuotedToken(s string) string {
	for _, q := range []byte{'"', '\''} {
		if end := strings.LastIndexByte(s, q); end >= 0 {
			if start := strings.LastIndexByte(s[:end], q); start >= 0 && start < end-1 {
				return s[start+1 : end]
			}
		}
	}
	return ""
}

// rankEvidenceByRelevanceWithSubject is the subject-aware variant of
// rankEvidenceByRelevance. When expected.Kind is a source-code literal
// kind AND the item's Object field (or Summary tail) matches that
// kind via subject.Score, the score gets a subject boost so the
// literal-carrying evidence out-ranks generic concrete-value chains
// that happen to have similar entity overlap.
//
// The predicateAxis parameter adds a second independent boost from
// the axis × anchor_kind affinity matrix (internal/analysis/axis).
// When the user's question verb ("how does X CALL Y" → AxisCall)
// matches an evidence item's AnchorKind, the item is boosted; when
// they misalign (AxisCall × AnchorDefinition), the item is demoted.
// Orthogonal to subject: both boosts compose multiplicatively and
// either can be the decisive ordering signal. AxisUnknown disables
// the axis boost; SubjectUnknown disables the subject boost.
//
// The zero-value combination (SubjectUnknown AND AxisUnknown) with
// zero entities extracted returns the input unchanged.
//
// Fixes the post-Session-10 bug where the explorer emitted
// `Config assigns "explore-skill"` at defaults.go:14 but the evidence
// ranker demoted it below unrelated `NewExplorerAgent → ...` chains,
// leaving the finalizer's curated Primary Evidence (top-12) without
// the literal the answer needed. Also addresses the 2026-04-18
// "how does X call Y" class of bugs where call-anchored evidence was
// buried under registration-anchored evidence in the top-N slate.
func rankEvidenceByRelevanceWithSubject(question string, items []types.EvidenceItem, readFiles map[string]bool, expected types.AnswerSubject, graph *repomap.Graph, predicateAxis types.PredicateAxis) []types.EvidenceItem {
	if len(items) == 0 {
		return items
	}
	entities := extractRankingEntitiesWithGraph(question, nil)
	if len(entities) == 0 && expected.Kind == types.SubjectUnknown && predicateAxis == types.AxisUnknown {
		return items // no entities, no subject, AND no axis → nothing to rank by
	}

	type scored struct {
		item  types.EvidenceItem
		score float64
	}
	scored_items := make([]scored, 0, len(items))
	for _, item := range items {
		s := evidenceRelevanceScore(item, entities, readFiles)
		// Subject boost: additive multiplier when the item's Object
		// / Summary carries a token that matches the expected
		// AnswerSubject kind. The alpha is the same 2.0 used by
		// rankChainsBySubject so downstream ordering stays
		// consistent between the chain ranker and the evidence
		// ranker.
		if expected.Kind != types.SubjectUnknown {
			boost := evidenceSubjectBoost(item, expected, graph)
			if boost > 0 {
				const alpha = 2.0
				s *= (1.0 + alpha*boost)
			}
		}
		// Axis boost: lookup into the static PredicateAxis × AnchorKind
		// affinity matrix. The raw matrix weight w is in [0.7, 1.6]
		// (see internal/analysis/axis/matrix.go for the curation
		// discipline). On the boost side (w > 1.0) we amplify with
		// the same beta=2.0 factor the subject boost uses, so a 1.5
		// affinity becomes a 2.0x score multiplier — strong enough to
		// flip ordering when entity overlap favours the wrong-axis
		// item by 0.5. On the demote side (w < 1.0) we apply the raw
		// weight directly — a 0.7 demote becomes a 0.7x multiplier,
		// not 0.4x, to avoid starving the top-N when every item is a
		// non-matching anchor kind.
		if predicateAxis != types.AxisUnknown && item.AnchorKind != "" {
			if w := axis.Affinity(predicateAxis, item.AnchorKind); w != 1.0 {
				if w > 1.0 {
					const beta = 2.0
					s *= 1.0 + beta*(w-1.0)
				} else {
					s *= w
				}
			}
		}
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
	// File-path locators are stripped first — otherwise an entity that
	// names a package directory (e.g. `agent` in `internal/agent/...`)
	// trivially matches every item sourced from that directory. See
	// memory/project_next_session_kickoff_filepath_entity_bug.md.
	//
	// Underscores and hyphens are collapsed before matching so that
	// snake_case identifiers like `sub_agents` register as a hit for
	// the entity `subagent`. Without this, `b.deps.SubAgents.Get(
	// "propose_sub_agents")` (a concrete value extracted by the
	// programmatic layer) missed the `subagent` entity check even
	// though it is THE cross-file join the extractor needs to see —
	// the literal `sub_agents` token breaks the contiguous substring
	// match that the pre-2026-04-17 scorer relied on.
	text := stripPathTokens(strings.ToLower(item.Subject + " " + item.Object + " " + item.Summary + " " + item.Predicate))
	normText := normalizeEntityHaystack(text)
	overlap := 0
	for _, ent := range entities {
		if entityHits(normText, ent) {
			overlap++
		}
	}
	entityScore := float64(overlap) / float64(len(entities))

	// 2. Kind weight. LLM-emittable kinds (direct, conditional,
	// registration, mechanism, relationship, absent) rank ABOVE
	// deterministic-only kinds (concrete_value, dataflow_path) because
	// the LLM extracted them with intent — they answer the question.
	// Concrete values are quantity-heavy but often low-relevance
	// ("Name() returns X") and should not dominate the top-N.
	kindWeight := 0.5
	switch item.Kind {
	case types.EvidenceDirect:
		kindWeight = 1.0
	case types.EvidenceRegistration:
		kindWeight = 0.95
	case types.EvidenceMechanism:
		kindWeight = 0.90
	case types.EvidenceConditional:
		kindWeight = 0.85
	case types.EvidenceRelationship:
		kindWeight = 0.80
	case types.EvidenceAbsent:
		kindWeight = 0.75
	case types.EvidenceDataflowPath:
		kindWeight = 0.60
	case types.EvidenceConcrete:
		kindWeight = 0.50
	case types.EvidenceUnresolved:
		kindWeight = 0.3
	case types.EvidenceTruncated:
		kindWeight = 0.1
	}

	// 2a. Mechanism-concrete promotion. A concrete_value whose Object
	// describes a registry lookup / gate / binding (e.g.
	// `b.deps.SubAgents.Get(string(b.name))`) is the deterministic
	// equivalent of an LLM-emit mechanism item — it names exactly the
	// cross-file join the extractor needs to reach the answer. Without
	// this promotion, the item scores at concrete's 0.5 kindWeight and
	// 1.0 producerBoost and gets buried below LLM-emit noise items
	// that happen to pass a superficial entity overlap check.
	//
	// Detection: Kind==Concrete + Producer=="concrete_values" + Object
	// contains one of the registry-call patterns + at least one
	// entity hit in the already-computed text. The pattern list is
	// intentionally narrow (Get/Register/Bind/Inject/Lookup/Find);
	// generic Add/Set are excluded to avoid matching `list.add(x)`
	// style unrelated calls. The entity-hit prerequisite ensures
	// irrelevant concrete values don't get promoted.
	//
	// Effect: kindWeight raised to 0.90 (parity with EvidenceMechanism),
	// producerBoost raised to 1.5 (parity with LLM-emit boost). The
	// combined effect is just enough to let a well-scoped mechanism
	// concrete value compete with an LLM-emit conditional that has
	// full entity overlap and bridgeBonus.
	isMechanismConcrete := looksLikeMechanismConcreteValue(item, normText, entities)
	if isMechanismConcrete {
		kindWeight = 0.90
	}

	// 2b. Producer boost: evidence emitted by the LLM via
	// emit_evidence was extracted with intent — it directly answers
	// the question. Deterministic evidence (concrete values, dataflow)
	// is broad and often misses the point. Boost LLM-produced items.
	// Mechanism concrete values also qualify: they are programmatic
	// extractions that name the exact gate/registry mechanism the
	// question is about.
	producerBoost := 1.0
	if item.Kind.IsLLMEmittable() && item.Producer != "" && !strings.HasSuffix(item.Producer, "/ungrounded") {
		producerBoost = 1.5
	} else if isMechanismConcrete {
		producerBoost = 1.5
	}

	// 3. Source weight: files the explorer read are more relevant.
	// Exception: concrete_values are mechanically extracted from
	// source text — their correctness does not depend on the LLM
	// having chosen to read the file. Penalising them for living in
	// an unread file defeats the programmatic layer's purpose, which
	// is exactly to supply evidence for files the LLM missed.
	sourceWeight := 0.5
	if readFiles != nil && readFiles[item.Source] {
		sourceWeight = 1.0
	} else if item.Producer == "concrete_values" {
		sourceWeight = 1.0
	}

	// 4. Bridge bonus: if subject and object match different entities.
	// Same path-strip + normalization discipline as the overlap text
	// above — a file path embedded in Subject/Object must not count,
	// and snake_case identifiers must register for the snake_case
	// form of an entity.
	bridgeBonus := 1.0
	if item.Subject != "" && item.Object != "" {
		subjectLower := normalizeEntityHaystack(stripPathTokens(strings.ToLower(item.Subject)))
		objectLower := normalizeEntityHaystack(stripPathTokens(strings.ToLower(item.Object)))
		subjectHit, objectHit := false, false
		for _, ent := range entities {
			if entityHits(subjectLower, ent) {
				subjectHit = true
			}
			if entityHits(objectLower, ent) {
				objectHit = true
			}
		}
		if subjectHit && objectHit {
			bridgeBonus = 2.0
		}
	}

	return entityScore * kindWeight * sourceWeight * bridgeBonus * producerBoost
}

// normalizeEntityHaystack strips underscores and hyphens from a
// haystack string so that snake_case / kebab-case identifiers match
// entity tokens like `subagent`. Applied to the haystack only; the
// entity list itself is normalised at construction time.
func normalizeEntityHaystack(s string) string {
	if !strings.ContainsAny(s, "_-") {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '_' || s[i] == '-' {
			continue
		}
		b = append(b, s[i])
	}
	return string(b)
}

// entityHits reports whether the normalised haystack contains the
// entity token. Thin wrapper kept separate from strings.Contains so
// future refinements (word-boundary checks, synonym expansion) live
// in one place.
func entityHits(haystack, entity string) bool {
	return strings.Contains(haystack, entity)
}

// mechanismConcretePatterns lists the lowercase method-call
// fragments that identify a registry / gate / binding mechanism in
// a concrete_value's Object field. The patterns are deliberately
// narrow — generic call fragments like `.add(` or `.set(` are
// excluded because they match too many unrelated data-structure
// operations. Each fragment is case-insensitive via the caller's
// strings.ToLower on the haystack.
var mechanismConcretePatterns = []string{
	".get(",
	".register(",
	".bind(",
	".inject(",
	".lookup(",
	".find(",
}

// looksLikeMechanismConcreteValue reports whether a concrete_value
// item is describing a cross-file mechanism (registry lookup /
// binding / gate) rather than a plain return value. normText is the
// lowercased, path-stripped, underscore-normalised combination of
// Subject + Object + Summary + Predicate — passed in by the caller
// so we reuse the work already done for entity scoring.
//
// The check requires BOTH a mechanism-call pattern AND at least one
// entity overlap. The entity prerequisite prevents us from boosting
// unrelated registry calls elsewhere in the codebase (e.g. a
// logging framework's `logger.Register(handler)` has nothing to do
// with the user's question and must score normally).
func looksLikeMechanismConcreteValue(item types.EvidenceItem, normText string, entities []string) bool {
	if item.Kind != types.EvidenceConcrete {
		return false
	}
	if item.Producer != "concrete_values" {
		return false
	}
	hasEntity := false
	for _, ent := range entities {
		if entityHits(normText, ent) {
			hasEntity = true
			break
		}
	}
	if !hasEntity {
		return false
	}
	// Check raw Object + Summary (not normText) so `.Get(` survives
	// the underscore strip — the pattern needs its parenthesis to
	// disambiguate method calls from bare identifiers.
	haystack := strings.ToLower(item.Object + " " + item.Summary)
	for _, p := range mechanismConcretePatterns {
		if strings.Contains(haystack, p) {
			return true
		}
	}
	return false
}

// rankFindingsByRelevance scores and sorts dataflow findings by
// relevance to the user's question, preferring findings whose path
// nodes overlap with question entities and shorter, more specific chains.
func rankFindingsByRelevance(question string, findings []types.FlowFindingDigest) []types.FlowFindingDigest {
	if len(findings) == 0 {
		return findings
	}
	entities := extractRankingEntitiesWithGraph(question, nil)
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

// extractRankingEntitiesWithGraph is the graph-aware variant. When
// graph is non-nil, pure-lowercase tokens shorter than 8 chars are
// accepted only if their lowercased form exactly matches a symbol
// name in the repo. This rule replaced the blanket "≥ 4 chars" gate
// that used to pull generic English words (`many`, `invoke`,
// `agents`) into the ERM entity set for questions like
// "how many agents can invoke subagent".
//
// Tokens that contain an uppercase letter, underscore, or dot are
// always accepted — they are structural identifiers (CamelCase,
// snake_case, qualified names) and never prose. Tokens with
// length ≥ 8 are also always accepted; compound lowercase
// identifiers like `subagent`, `explorer`, `handlers` stand on
// length alone.
func extractRankingEntitiesWithGraph(question string, graph *repomap.Graph) []string {
	var symSet map[string]bool
	if graph != nil {
		symSet = make(map[string]bool, len(graph.SymbolDefs))
		for name := range graph.SymbolDefs {
			symSet[strings.ToLower(name)] = true
		}
	}
	seen := make(map[string]bool)
	var entities []string
	add := func(raw string) {
		trimmed := strings.Trim(raw, "(){}[]?!.,;:'\"")
		if !entityQualifies(trimmed, symSet) {
			return
		}
		lowered := strings.ToLower(trimmed)
		if seen[lowered] {
			return
		}
		seen[lowered] = true
		entities = append(entities, lowered)
	}

	// Both sources (backtick + run) go through the same add() — the
	// policy lives entirely in entityQualifies + the trim set. The
	// one source-specific rule is that dotted run tokens get split
	// into parts ("Foo.Bar" → "Foo", "Bar") so the ranking can score
	// individual tokens as well as the qualified form.
	scanQuestionTokens(question, func(tok string, src tokenSource) {
		add(tok)
		if src == tokenRun && strings.Contains(tok, ".") {
			for _, part := range strings.Split(tok, ".") {
				add(part)
			}
		}
	})
	return entities
}

// entityQualifies reports whether raw is admissible as a question
// ranking entity. See extractRankingEntitiesWithGraph for the rule
// summary. Exported only within the package for unit-test visibility
// of the filter logic independent of the extraction loop.
func entityQualifies(raw string, symSet map[string]bool) bool {
	if len(raw) < 4 {
		return false
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if (c >= 'A' && c <= 'Z') || c == '_' || c == '.' {
			return true
		}
	}
	if len(raw) >= 8 {
		return true
	}
	if symSet != nil && symSet[strings.ToLower(raw)] {
		return true
	}
	return false
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
