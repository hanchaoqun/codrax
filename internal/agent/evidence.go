// Several functions in this file (parseEvidenceItems, groundEvidenceItems,
// mergeEvidenceItems, rankEvidenceByRelevance, scrubSiblingEvidenceBlocks)
// form filters F4..F7 and F9 of the post-hoc filtering pipeline. Their
// required execution order and fail-open behaviour are documented in
// docs/filtering-pipeline.md — update that doc in the same commit when
// adding, moving, or removing a filter here.
//
// P1.1 (2026-04-14): parseEvidenceItems is no longer the only feeder of
// the Evidence channel. The structured emit_evidence tool
// (internal/tool/emit_evidence.go) is now the preferred channel under
// evidence_tool_mode=on; ensureStructuredEvidence in explorer.go merges
// its output with parseEvidenceItems via mergeEvidenceItems before
// grounding. parseEvidenceItems stays as the fallback path and the
// default; do NOT delete it without flipping the default first and
// updating docs/filtering-pipeline.md row F4.
package agent

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ungroundedSuffix is appended to Producer for evidence items whose
// LineStart failed every validation tier. Used by downstream
// consumers (finalizer, evidence rendering) to demote or filter
// these items. The suffix is stable enough to grep for; downstream
// checks use strings.HasSuffix to detect it.
const ungroundedSuffix = "/ungrounded"

// gutterLinePrefix matches the per-line gutter emitted by
// tool.renderWithLineGutter: optional spaces, 1-6 digits, U+2502,
// single space. The captured group is the line number.
//
// Keep the format in sync with internal/tool/builtin.go's
// renderWithLineGutter. If the gutter format changes, update this
// regex or grounding silently degrades to the read-window fallback.
var gutterLineRe = regexp.MustCompile(`^\s*(\d{1,6})│ (.*)$`)

// identifierTokenRe extracts identifier-shaped tokens of length ≥3
// from a free-text evidence summary. Used to check whether the
// code line cited by an evidence entry actually contains any of the
// identifiers the entry talks about.
var identifierTokenRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

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

// groundEvidenceItems validates the LineStart field of every LLM-
// parsed evidence item against two deterministic signals drawn from
// this session's state:
//
//  1. The actual text of the line at (Source, LineStart) as seen
//     by the LLM, reconstructed from the per-line gutter that
//     tool.renderWithLineGutter now emits in every read_file
//     result. If the evidence entry claims Subject=Foo at line N,
//     that line (and its immediate neighbours) should mention
//     "Foo" — otherwise the LLM either hallucinated the number or
//     is paraphrasing a different location.
//
//  2. The tree-sitter symbol table in graph.SymbolsInFile(Source).
//     If Subject is a symbol name and LineStart is exactly that
//     symbol's declaration Line, the cite is grounded even when the
//     LLM did not read the file (it legitimately pulled the number
//     from a grep or repo_map result).
//
// Items whose LineStart fails BOTH tiers have their LineStart and
// LineEnd cleared to zero and their Producer suffixed with
// "/ungrounded". The no-rewrite policy is intentional: fabricating
// a plausible-looking "nearest symbol" line would silently replace
// a known hallucination with a wrong cite, recreating the fake-
// green failure mode at a new layer. Downstream consumers decide
// what to do with ungrounded items (demote, drop, render without
// a line cite). See memory/feedback_honesty_over_cleverness.md.
//
// Items with no Source or no LineStart pass through unchanged —
// grounding only fires when the LLM committed to a citation.
//
// Grounding is a NET, not a substitute for the gutter-prefix
// signal in tool.renderWithLineGutter. An LLM that carefully
// copies gutter numbers will rarely trip the grounder; an LLM that
// guesses will trip it on anything egregious (wrong file, line
// outside any read window, line that doesn't mention the symbol).
// In-body paraphrase drift — citing line 1018 of a function that
// runs from 976 to 1413 while describing a branch actually at 980
// — is partially caught here (if line 1018 doesn't mention the
// subject) and partially the gutter's job (if line 1018 does).
func groundEvidenceItems(items []types.EvidenceItem, graph *repomap.Graph, history []types.ToolResult) []types.EvidenceItem {
	if len(items) == 0 {
		return items
	}
	lineIndex := buildReadFileLineIndex(history)

	ungroundedCount := 0
	out := make([]types.EvidenceItem, len(items))
	for i, it := range items {
		out[i] = it
		if it.Source == "" || it.LineStart == 0 {
			continue
		}
		if evidenceLineGrounded(it, graph, lineIndex) {
			continue
		}
		// Ungrounded: clear line fields, tag producer, rebuild ID.
		out[i].LineStart = 0
		out[i].LineEnd = 0
		if !strings.HasSuffix(out[i].Producer, ungroundedSuffix) {
			out[i].Producer = out[i].Producer + ungroundedSuffix
		}
		out[i].ID = types.StableEvidenceID(
			out[i].Kind, out[i].Subject, out[i].Predicate,
			out[i].Object, out[i].Condition, out[i].Source,
			out[i].LineStart, out[i].LineEnd,
		)
		ungroundedCount++
		logging.Debug("[evidence-ground] ungrounded: subject=%q source=%s line=%d (tier1=line_text, tier2=symbol_start — both failed)",
			it.Subject, it.Source, it.LineStart)
	}
	if ungroundedCount > 0 {
		logging.Debug("[evidence-ground] cleared %d/%d evidence items", ungroundedCount, len(items))
	}
	return out
}

// evidenceLineGrounded runs the two-tier grounding check described
// on groundEvidenceItems. Returns true when at least one tier
// accepts the item; false when both tiers fail or neither applies.
func evidenceLineGrounded(it types.EvidenceItem, graph *repomap.Graph, lineIndex map[string]map[int]string) bool {
	// Tier 1: cross-reference LineStart text with the evidence's
	// own identifiers. Uses the gutter-reconstructed line text, so
	// we only get here if the LLM actually read the relevant slice
	// in this session.
	if fileLines, ok := lineIndex[it.Source]; ok {
		if lineText, exists := lookupLineWithNeighbours(fileLines, it.LineStart, 2); exists {
			if lineCorroboratesEvidence(lineText, it.Subject, it.Object, it.Condition, graph) {
				return true
			}
		}
	}
	// Tier 2: symbol-table declaration match. Handles the "cited
	// from grep output, never opened the file" case — the line
	// number is the declaration of a symbol whose name matches the
	// evidence subject.
	if graph != nil && it.Subject != "" {
		name := lastDotSegment(it.Subject)
		for _, sym := range graph.SymbolsInFile(it.Source) {
			if sym.Name != name {
				continue
			}
			if sym.Line > 0 && it.LineStart == sym.Line {
				return true
			}
		}
	}
	return false
}

// buildReadFileLineIndex walks tool history and reconstructs, for
// every read_file result that carries a gutter, a map from
// absolute line number → line text (without the gutter prefix).
// Subsequent read_file calls on the same Source merge into the
// existing map so the latest content for each line wins.
func buildReadFileLineIndex(history []types.ToolResult) map[string]map[int]string {
	out := make(map[string]map[int]string)
	for _, r := range history {
		if !r.Success || r.ToolName != "read_file" {
			continue
		}
		// Banner is the first line; skip it and walk gutter-
		// prefixed lines below. See renderWithLineGutter for the
		// exact format contract.
		body := r.Summary
		if strings.HasPrefix(body, "[") {
			if idx := strings.Index(body, "\n"); idx >= 0 {
				// Path is the substring up to the first ':' inside
				// the bracket. We use the banner-derived path as
				// the map key so two different reads of the same
				// file reconcile.
				path := parseBannerPath(body[:idx])
				if path == "" {
					continue
				}
				fileMap, ok := out[path]
				if !ok {
					fileMap = make(map[int]string)
					out[path] = fileMap
				}
				// Walk the body line by line, matching gutter
				// prefixes. Non-matching lines are skipped — they
				// might be the tail hint from StoreBlobHeadOnly.
				for _, gutterLine := range strings.Split(body[idx+1:], "\n") {
					m := gutterLineRe.FindStringSubmatch(gutterLine)
					if m == nil {
						continue
					}
					n, err := strconv.Atoi(m[1])
					if err != nil {
						continue
					}
					fileMap[n] = m[2]
				}
			}
		}
	}
	return out
}

// parseBannerPath extracts the file path from a read_file banner
// of the form `[path: showing ...]`. Returns "" on malformed input.
func parseBannerPath(banner string) string {
	if !strings.HasPrefix(banner, "[") {
		return ""
	}
	banner = strings.TrimPrefix(banner, "[")
	colon := strings.Index(banner, ": ")
	if colon < 1 {
		return ""
	}
	return banner[:colon]
}

// lookupLineWithNeighbours returns the concatenated text of lines
// [n-radius, n+radius] from the per-file line map. Missing lines
// are skipped without error. Returns (text, true) when at least
// the target line `n` exists, (""/false) otherwise. The radius
// window exists so a one-off off-by-one on an LLM cite still
// corroborates when the neighbour line mentions the subject.
func lookupLineWithNeighbours(fileLines map[int]string, n, radius int) (string, bool) {
	if _, ok := fileLines[n]; !ok {
		return "", false
	}
	var b strings.Builder
	for i := n - radius; i <= n+radius; i++ {
		if text, ok := fileLines[i]; ok {
			b.WriteString(text)
			b.WriteByte('\n')
		}
	}
	return b.String(), true
}

// lineCorroboratesEvidence checks whether the cited line text (and
// its neighbours) contain at least one code-shaped identifier that
// the evidence claims to be describing. Matching is on whole
// identifiers only — substring matching on CamelCase fragments is
// intentionally avoided (it invites "Handle" matching "HandleFoo"
// and generates false positives).
//
// "Code-shaped" is decided structurally, without any hard-coded
// stopword list:
//   - CamelCase (has both upper and lower case letters) — e.g.
//     `ContinuationPrompt`, `subExplorer`
//   - Contains an underscore — e.g. `read_set`, `user_question`
//   - OR appears in the repo graph as a known symbol name — the
//     authoritative source for "this token names something real"
//
// Any candidate token that satisfies none of these is treated as
// prose (`return`, `when`, `line`, `function`, `method`, …) and
// ignored. This keeps the grounder cross-lingual and removes the
// maintenance burden of curating an English stopword list — the
// approach memory/feedback_overfitting_audit_stopwords.md warns
// against.
//
// Returns true when at least one code-shaped identifier from the
// union of Subject / Object / Condition shows up as a whole word
// in lineText. Returns false when the evidence has no code-shaped
// identifiers or when none of them match the line — in that case
// Tier 1 cannot judge and the caller falls to Tier 2 or declares
// ungrounded.
func lineCorroboratesEvidence(lineText, subject, object, condition string, graph *repomap.Graph) bool {
	want := extractCodeIdentifiers(subject+" "+object+" "+condition, graph)
	if len(want) == 0 {
		return false
	}
	// Rebuild a word-boundary token set of the line so we compare
	// whole identifiers only.
	have := make(map[string]bool)
	for _, tok := range identifierTokenRe.FindAllString(lineText, -1) {
		have[tok] = true
	}
	for _, w := range want {
		if have[w] {
			return true
		}
	}
	return false
}

// extractCodeIdentifiers walks a free-text evidence fragment and
// returns the subset of identifier-shaped tokens that look like
// real code identifiers — CamelCase, snake_case, or known to the
// repo's symbol graph. Lowercase common English words ("return",
// "when", "method") are rejected purely on structural grounds, no
// curated stopword list. Deduplicated, preserves first-seen order.
func extractCodeIdentifiers(text string, graph *repomap.Graph) []string {
	tokens := identifierTokenRe.FindAllString(text, -1)
	if len(tokens) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(tokens))
	var out []string
	for _, t := range tokens {
		if seen[t] {
			continue
		}
		if !looksLikeCodeIdentifier(t, graph) {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// looksLikeCodeIdentifier is the structural + graph-based filter
// that decides whether a token qualifies as "a name the evidence
// could be citing" vs. a prose word. The three accept rules are
// orthogonal; a token passing any one is accepted:
//
//  1. Mixed case (at least one upper and one lower letter): catches
//     CamelCase (`ContinuationPrompt`), lowerCamelCase (`readSet`),
//     and PascalCase. Over-fit-safe because it's a structural
//     property of the string, not a dictionary lookup.
//  2. Contains `_`: catches snake_case (`read_file`), SCREAMING_SNAKE,
//     and Python/C naming conventions.
//  3. Appears in the repo's symbol graph as a Name: catches
//     single-lowercase-word identifiers that are legitimate in the
//     repo (`explorer`, `agent`, `grep`) but indistinguishable from
//     prose without domain knowledge — and lets the repo itself
//     decide what that knowledge is, no hard-coded list.
func looksLikeCodeIdentifier(tok string, graph *repomap.Graph) bool {
	hasUpper, hasLower, hasUnderscore := false, false, false
	for _, r := range tok {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r == '_':
			hasUnderscore = true
		}
	}
	if hasUpper && hasLower {
		return true
	}
	if hasUnderscore {
		return true
	}
	if graph == nil {
		return false
	}
	// Graph-based acceptance: the token is a known symbol name in
	// this repository, regardless of file. This is the authoritative
	// over-fit-safe escape hatch — if the repo defines `explorer` as
	// a symbol, we treat `explorer` as a real identifier.
	if defs, ok := graph.SymbolDefs[tok]; ok && len(defs) > 0 {
		return true
	}
	return false
}

// lastDotSegment returns the substring after the final `.` in s,
// or s itself when no `.` is present. Used to normalise subjects
// like "Receiver.Method" down to "Method" for symbol-table lookup.
func lastDotSegment(s string) string {
	if idx := strings.LastIndex(s, "."); idx >= 0 && idx < len(s)-1 {
		return s[idx+1:]
	}
	return s
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
	// File-path locators are stripped first — otherwise an entity that
	// names a package directory (e.g. `agent` in `internal/agent/...`)
	// trivially matches every item sourced from that directory. See
	// memory/project_next_session_kickoff_filepath_entity_bug.md.
	text := stripPathTokens(strings.ToLower(item.Subject + " " + item.Object + " " + item.Summary + " " + item.Predicate))
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
	// Same path-strip discipline as the overlap text above — a file
	// path embedded in Subject/Object must not count as a hit.
	bridgeBonus := 1.0
	if item.Subject != "" && item.Object != "" {
		subjectLower := stripPathTokens(strings.ToLower(item.Subject))
		objectLower := stripPathTokens(strings.ToLower(item.Object))
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
// scoring. Wrapper around extractRankingEntitiesWithGraph with no
// symbol table; callers that have access to the repo's repomap.Graph
// should prefer extractRankingEntitiesWithGraph so lowercase tokens
// that verbatim match real symbol names survive the tighter filter.
func extractRankingEntities(question string) []string {
	return extractRankingEntitiesWithGraph(question, nil)
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
