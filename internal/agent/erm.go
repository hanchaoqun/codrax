package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EvidenceRequirement represents a specific type of evidence needed to
// answer the user's question. The explorer tracks which requirements
// are satisfied during investigation and directs file reads to fill gaps.
//
// noSourceLineSentinel is used by identifyAnswerChains' multi-key
// sort when an EvidenceItem has no LineStart. Sorted ascending,
// items with real line numbers (small positive integers) come
// first and items without come last. Using a large but bounded
// integer (not math.MaxInt) keeps the key inside int32 range so
// the comparator stays well-defined on 32-bit platforms.
const noSourceLineSentinel = 1 << 30

type EvidenceRequirement struct {
	Kind     string   // "enumeration", "call_chain", "registration", "return_value", "config_mapping", "conditional"
	Entities []string // key entities from the question this requirement relates to
	Reason   string   // human-readable reason for this requirement
	Status   string   // "unsatisfied", "partial", "satisfied"
}

// extractEvidenceRequirements analyzes the user's question and produces
// a set of evidence requirements. This is deterministic (no LLM call)
// and drives the entire investigation: file prioritization, continuation
// prompts, quality gates, and dataflow candidate selection.
// extractEvidenceRequirements is the convenience entry point that
// derives entities from the same string used for keyword detection.
// Callers that want entities and keyword source to differ (e.g. the
// explorer needs CamelCase identifiers from the original user request
// but English idioms from the analyzer's rewrite) should use
// extractEvidenceRequirementsWithEntities directly.
func extractEvidenceRequirements(question string) []EvidenceRequirement {
	return extractEvidenceRequirementsWithEntities(question, extractRankingEntities(question))
}

// extractEvidenceRequirementsWithHint is the analyzer-aware entry
// point used by the explorer. When the analyzer declared a concrete
// question_kind via todo_write (analyzer.go contract), we trust it
// directly and emit the matching EvidenceRequirement without running
// keyword inference — the analyzer has strictly more context than a
// post-hoc regex over the same string. Empty or "unknown" kind falls
// through to the legacy keyword-based path.
//
// This closes the regression loop documented in
// project_erm_english_keyword_gap: the keyword tables existed because
// the analyzer's output was opaque. With a typed kind, we no longer
// need to reverse-engineer it from the rewritten English.
//
// The keyword path is still run as a SUPPLEMENT when the declared
// kind is present — it may surface additional requirements (e.g. a
// mechanism question that also needs a registration lookup) that the
// analyzer did not think to declare. The declared kind is always
// represented at least once in the output.
func extractEvidenceRequirementsWithHint(question string, entities []string, declaredKind string) []EvidenceRequirement {
	declaredKind = strings.ToLower(strings.TrimSpace(declaredKind))
	if declaredKind == "" || declaredKind == "unknown" {
		return extractEvidenceRequirementsWithEntities(question, entities)
	}

	// Build the keyword-inferred set first so we can merge.
	reqs := extractEvidenceRequirementsWithEntities(question, entities)

	// Check whether the declared kind already appears. If so, the
	// keyword path has already covered it; nothing more to do.
	for _, r := range reqs {
		if r.Kind == declaredKind {
			return reqs
		}
	}

	// The declared kind was missed by keyword inference — add it
	// explicitly. This is the "analyzer saves us" path: e.g. a
	// Chinese mechanism question whose English rewrite used idioms
	// the keyword tables don't cover.
	reason := fmt.Sprintf("analyzer declared question_kind=%s", declaredKind)
	switch declaredKind {
	case "registration":
		// Registration requirements are per-entity in the keyword
		// path; match that convention so downstream
		// checkRequirementSatisfaction works uniformly.
		if len(entities) == 0 {
			reqs = append(reqs, EvidenceRequirement{
				Kind: declaredKind, Reason: reason, Status: "unsatisfied",
			})
		} else {
			for _, ent := range entities {
				reqs = append(reqs, EvidenceRequirement{
					Kind: declaredKind, Entities: []string{ent},
					Reason: reason + " (per-entity)", Status: "unsatisfied",
				})
			}
		}
	case "return_value":
		if len(entities) == 0 {
			reqs = append(reqs, EvidenceRequirement{
				Kind: declaredKind, Reason: reason, Status: "unsatisfied",
			})
		} else {
			for _, ent := range entities {
				reqs = append(reqs, EvidenceRequirement{
					Kind: declaredKind, Entities: []string{ent},
					Reason: reason + " (per-entity)", Status: "unsatisfied",
				})
			}
		}
	default:
		// mechanism / conditional / config_mapping / enumeration /
		// call_chain all take the entity set as a single group.
		reqs = append(reqs, EvidenceRequirement{
			Kind: declaredKind, Entities: append([]string(nil), entities...),
			Reason: reason, Status: "unsatisfied",
		})
	}

	return reqs
}

// extractEvidenceRequirementsWithEntities lets the caller supply the
// entity list separately from the keyword-detection text. This is the
// primary entry point for the explorer, which needs to:
//
//   - run keyword detection over the union of the original Chinese
//     question (for Chinese trigger words like 怎么/多少) and the
//     analyzer's English rewrite (for "Determine the number of..."
//     idioms)
//   - extract entities from the original ONLY, because the analyzer's
//     rewrite tends to add generic English nouns ("count", "agents",
//     "that") that pollute the entity set, inflate the requirement
//     count, and degrade answer-chain ranking
//
// This separation was added after an integration test (df1 5x, commit
// c04298f) caught a regression where joining the two strings before
// entity extraction made answer_chain[0] flip from the canonical
// `RegisterDefaultSubAgents → SubExplorer` to the spurious `RegisterDefaults → GrepTool.Description`
// chain — the tool registry matched MORE of the polluted entity set
// than the correct answer.
func extractEvidenceRequirementsWithEntities(question string, entities []string) []EvidenceRequirement {
	lower := strings.ToLower(question)

	var reqs []EvidenceRequirement
	seen := make(map[string]bool)
	add := func(kind, reason string, ents ...string) {
		key := kind + ":" + strings.Join(ents, ",")
		if seen[key] {
			return
		}
		seen[key] = true
		reqs = append(reqs, EvidenceRequirement{
			Kind:     kind,
			Entities: ents,
			Reason:   reason,
			Status:   "unsatisfied",
		})
	}

	// --- Enumeration: "how many", "list all", "哪些", "多少", "列出" ---
	//
	// English set is broad enough to survive the analyzer's question
	// rewriting. Original Chinese "有多少 / 哪些" gets rewritten by the
	// analyzer to phrases like "Determine the number of X", "Count
	// all X", "Find all instances of X", "List the X" — none of which
	// the original {"how many", "list all", ...} set caught. All
	// additions are multi-word phrases to keep false-positive risk
	// low (bare "count" / "all" / "list" would over-trigger).
	for _, kw := range []string{
		"how many", "list all", "list each", "list the", "what are the",
		"the number of", "count the", "count of",
		"determine the number", "determine all",
		"find all", "find every", "identify all",
		"all instances of", "enumerate",
	} {
		if strings.Contains(lower, kw) {
			add("enumeration", fmt.Sprintf("question asks to enumerate (%s)", kw), entities...)
			break
		}
	}
	for _, kw := range []string{"哪些", "多少", "列出", "哪几", "有几个", "分别"} {
		if strings.Contains(question, kw) {
			add("enumeration", fmt.Sprintf("question asks to enumerate (%s)", kw), entities...)
			break
		}
	}

	// --- Call chain: "calls", "invoke", "调用", "dispatch" ---
	isCallChain := false
	for _, kw := range []string{"call", "invoke", "dispatch", "calls"} {
		if strings.Contains(lower, kw) {
			isCallChain = true
			break
		}
	}
	for _, kw := range []string{"调用", "分发", "触发"} {
		if strings.Contains(question, kw) {
			isCallChain = true
			break
		}
	}
	if isCallChain && len(entities) >= 2 {
		add("call_chain",
			fmt.Sprintf("need to trace how %s invokes/calls %s", entities[0], entities[1]),
			entities...)
	} else if isCallChain && len(entities) >= 1 {
		add("call_chain",
			fmt.Sprintf("need to trace call relationships of %s", entities[0]),
			entities...)
	}

	// --- Registration: "registered", "注册", or call_chain implies it ---
	//
	// "register" is a stem that already covers "registers/registered/
	// registering/registry" via strings.Contains. Adding "bind"-family
	// terms catches the analyzer's other common rewrite for 注册/绑定
	// ("X is bound to Y", "binding for X").
	isRegistration := false
	for _, kw := range []string{"register", "registered", "registry", "bound to", "binding"} {
		if strings.Contains(lower, kw) {
			isRegistration = true
			break
		}
	}
	for _, kw := range []string{"注册", "绑定"} {
		if strings.Contains(question, kw) {
			isRegistration = true
			break
		}
	}
	// Call chains imply registration: "which X can call Y?" requires knowing what Y is registered
	if isCallChain || isRegistration {
		for _, ent := range entities {
			add("registration",
				fmt.Sprintf("need to find where %s is registered/bound", ent),
				ent)
		}
	}

	// --- Return value: for each entity, we may need its concrete value ---
	// Triggered when question asks about matching, identity, or naming.
	//
	// "return value" / "returned by" added because the analyzer rewrites
	// "X 的方法返回什么?" → "Identify the return value of X" — and the
	// original {name,type,which,what} set missed it (no name/type/which/
	// what in that phrasing). Both additions are 2+ word phrases to
	// avoid the false-positive trap of bare "return" (matches "early
	// return", "return statement", code-structure questions).
	for _, kw := range []string{
		"name", "type", "which", "what",
		"return value", "returned by",
		"名称", "类型", "哪个", "什么",
	} {
		if strings.Contains(lower, kw) || strings.Contains(question, kw) {
			for _, ent := range entities {
				add("return_value",
					fmt.Sprintf("need concrete return values from %s (for matching/identity)", ent),
					ent)
			}
			break
		}
	}

	// --- Config mapping: "config", "configured", "配置" ---
	for _, kw := range []string{"config", "configured", "configuration", "配置", "yaml", "json"} {
		if strings.Contains(lower, kw) || strings.Contains(question, kw) {
			add("config_mapping", "need to trace config keys to runtime behavior", entities...)
			break
		}
	}

	// --- Conditional: "when", "if", "condition", "条件", "什么时候" ---
	//
	// "triggered when" / "fires when" added to catch the analyzer's
	// typical rewrite of "什么时候 X 触发?" → "Identify when X is
	// triggered" / "Determine the conditions under which X fires".
	// Bare "if" / "triggered" are NOT added because they over-trigger
	// (every conditional code question contains "if").
	for _, kw := range []string{
		"when", "condition", "under what",
		"triggered when", "fires when", "conditions under",
		"条件", "什么时候", "何时",
	} {
		if strings.Contains(lower, kw) || strings.Contains(question, kw) {
			add("conditional", "need to resolve conditions under which behavior occurs", entities...)
			break
		}
	}

	// --- Mechanism (T2.1): "how does X work", "process", "原理", "机制", "怎么" ---
	// English keyword set is intentionally broad enough to survive the
	// analyzer's question rewriting (which often turns "X 怎么工作?" into
	// "Explain how X works" or "Describe the process of X"). The Chinese
	// set covers direct user phrasing.
	//
	// Detection avoids overlap with conditional ("when") and call_chain
	// ("call/invoke") by requiring an explicit mechanism marker — we
	// don't trigger on bare "how" because the analyzer uses "how" loosely.
	for _, kw := range []string{
		// English (analyzer-rewrite friendly)
		"how does", "how is", "how do", "how the",
		"explain how", "describe how", "describe the process",
		"step by step", "steps involved", "mechanism of", "process of", "flow of",
		"walk through", "walkthrough",
	} {
		if strings.Contains(lower, kw) {
			add("mechanism", fmt.Sprintf("need to trace mechanism (%s)", kw), entities...)
			break
		}
	}
	for _, kw := range []string{
		// Chinese (direct user phrasing)
		"怎么工作", "怎么实现", "如何实现", "如何工作", "如何处理",
		"原理", "机制", "工作流程", "步骤", "过程",
	} {
		if strings.Contains(question, kw) {
			add("mechanism", fmt.Sprintf("need to trace mechanism (%s)", kw), entities...)
			break
		}
	}

	return reqs
}

// checkRequirementSatisfaction scans investigation notes and structured
// evidence against ERM requirements. Returns the updated requirements
// with status set to "satisfied", "partial", or "unsatisfied".
func checkRequirementSatisfaction(reqs []EvidenceRequirement, notes []string, evidence []types.EvidenceItem) []EvidenceRequirement {
	if len(reqs) == 0 {
		return reqs
	}
	notesJoined := normalizeForMatch(strings.Join(notes, "\n"))

	for i := range reqs {
		req := &reqs[i]
		if req.Status == "satisfied" {
			continue
		}
		switch req.Kind {
		case "enumeration":
			// Satisfied if notes contain a list of items (multiple [DIRECT]/[REGISTRATION] tags)
			count := countEvidenceTags(notesJoined, []string{"[direct]", "[registration]"})
			count += countEvidenceByKinds(evidence, req.Entities, types.EvidenceDirect, types.EvidenceRegistration)
			if count >= 3 {
				req.Status = "satisfied"
			} else if count >= 1 {
				req.Status = "partial"
			}

		case "call_chain":
			// Satisfied if notes describe call relationships between entities
			hasRelationship := countEvidenceTags(notesJoined, []string{"[relationship]", "[mechanism]"}) > 0
			hasRelationship = hasRelationship || countEvidenceByKinds(evidence, req.Entities, types.EvidenceRelationship) > 0
			// Also check if entities appear together in a call context
			entitiesInNotes := 0
			for _, ent := range req.Entities {
				if strings.Contains(notesJoined, strings.ToLower(ent)) {
					entitiesInNotes++
				}
			}
			if hasRelationship && entitiesInNotes >= 2 {
				req.Status = "satisfied"
			} else if hasRelationship || entitiesInNotes >= 1 {
				req.Status = "partial"
			}

		case "registration":
			// Satisfied if notes contain a [REGISTRATION] tag mentioning the entity,
			// AND the registration mentions a SPECIFIC value (not just the interface).
			for _, ent := range req.Entities {
				entLower := normalizeForMatch(ent)
				// Look for "[registration]" lines that mention this entity with specific values
				for _, line := range strings.Split(notesJoined, "\n") {
					if strings.Contains(line, "[registration]") && strings.Contains(line, entLower) {
						// Check if it mentions a specific value (NewXxx, "xxx", etc.)
						if strings.Contains(line, "new") || strings.Contains(line, "\"") ||
							strings.Contains(line, "only") || strings.Contains(line, "default") {
							req.Status = "satisfied"
						} else if req.Status != "satisfied" {
							req.Status = "partial"
						}
					}
				}
				// Also check structured evidence
				for _, ev := range evidence {
					if ev.Kind == types.EvidenceRegistration &&
						strings.Contains(normalizeForMatch(ev.Subject+ev.Object+ev.Summary), entLower) {
						if strings.Contains(normalizeForMatch(ev.Object+ev.Summary), "new") ||
							strings.Contains(ev.Object, "\"") {
							req.Status = "satisfied"
						} else if req.Status != "satisfied" {
							req.Status = "partial"
						}
					}
				}
				// T1.1 follow-up: also accept binds-shape Concrete Values
				// produced by the deterministic extractor. The
				// `RegisterDefaultSubAgents binds ONLY NewSubExplorer` chain
				// surfaces here as EvidenceConcrete{Predicate: "binds ONLY"},
				// not as EvidenceRegistration. Without this branch the
				// satisfaction check is blind to the strongest deterministic
				// evidence the system already produces.
				//
				// Entity matching uses the same normalizeForMatch logic as
				// the [REGISTRATION] branch above so precision is identical.
				for _, ev := range evidence {
					if !isRegistrationShape(ev) {
						continue
					}
					if strings.Contains(normalizeForMatch(ev.Subject+" "+ev.Object+" "+ev.Summary), entLower) {
						req.Status = "satisfied"
						break
					}
				}
				if req.Status == "satisfied" {
					break
				}
			}

		case "return_value":
			// Satisfied if concrete values exist for the entity
			for _, ent := range req.Entities {
				entLower := normalizeForMatch(ent)
				for _, ev := range evidence {
					if ev.Kind == types.EvidenceConcrete &&
						strings.Contains(normalizeForMatch(ev.Subject), entLower) &&
						ev.Object != "" {
						req.Status = "satisfied"
						break
					}
				}
				if req.Status == "satisfied" {
					break
				}
				// Check notes for return patterns
				if strings.Contains(notesJoined, normalizeForMatch(ent)) &&
					(strings.Contains(notesJoined, "returns") || strings.Contains(notesJoined, "return")) {
					if req.Status != "satisfied" {
						req.Status = "partial"
					}
				}
			}

		case "config_mapping":
			count := countEvidenceByKinds(evidence, req.Entities, types.EvidenceConcrete, types.EvidenceMechanism)
			if count >= 2 {
				req.Status = "satisfied"
			} else if count >= 1 {
				req.Status = "partial"
			}

		case "conditional":
			count := countEvidenceTags(notesJoined, []string{"[conditional]"})
			count += countEvidenceByKinds(evidence, req.Entities, types.EvidenceConditional)
			if count >= 1 {
				req.Status = "satisfied"
			}

		case "mechanism":
			// Mechanism requirements need either LLM-tagged [MECHANISM]
			// notes or structured EvidenceMechanism items mentioning the
			// requirement entities. Reuses the same per-Kind counter as
			// conditional/config_mapping for consistency. Two evidence
			// items are required for "satisfied" because mechanism
			// answers usually need at least an entry point + a step
			// inside the function body — one isolated [MECHANISM] tag
			// is rarely enough for a usable explanation. Falls through
			// to "partial" with one item.
			count := countEvidenceTags(notesJoined, []string{"[mechanism]"})
			count += countEvidenceByKinds(evidence, req.Entities, types.EvidenceMechanism)
			// Also accept relationship items mentioning the entity —
			// "calls / writes_field / reads_field" are mechanism-shaped
			// when they appear together. T2.2 (mechanism scan pipeline)
			// will produce richer EvidenceMechanism directly; until then
			// the dataflow lowering's relationships fill the gap.
			count += countEvidenceByKinds(evidence, req.Entities, types.EvidenceRelationship)
			if count >= 2 {
				req.Status = "satisfied"
			} else if count >= 1 {
				req.Status = "partial"
			}
		}
	}
	return reqs
}

// normalizeForMatch lowercases and strips hyphens/underscores so that
// "sub-agent", "sub_agent", and "subagent" all match.
func normalizeForMatch(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

func countEvidenceTags(text string, tags []string) int {
	count := 0
	for _, tag := range tags {
		count += strings.Count(text, tag)
	}
	return count
}

func countEvidenceByKinds(evidence []types.EvidenceItem, entities []string, kinds ...types.EvidenceKind) int {
	count := 0
	for _, ev := range evidence {
		kindMatch := false
		for _, k := range kinds {
			if ev.Kind == k {
				kindMatch = true
				break
			}
		}
		if !kindMatch {
			continue
		}
		if len(entities) == 0 {
			count++
			continue
		}
		text := normalizeForMatch(ev.Subject + " " + ev.Object + " " + ev.Summary)
		for _, ent := range entities {
			if strings.Contains(text, normalizeForMatch(ent)) {
				count++
				break
			}
		}
	}
	return count
}

// ermUnsatisfiedGaps returns a human-readable prompt section describing
// which evidence requirements are still unsatisfied, suitable for
// injection into ContinuationPrompt.
func ermUnsatisfiedGaps(reqs []EvidenceRequirement) string {
	var gaps []string
	for _, req := range reqs {
		if req.Status == "satisfied" {
			continue
		}
		prefix := "MISSING"
		if req.Status == "partial" {
			prefix = "INCOMPLETE"
		}
		gaps = append(gaps, fmt.Sprintf("- [%s] %s: %s (entities: %s)",
			prefix, req.Kind, req.Reason, strings.Join(req.Entities, ", ")))
	}
	if len(gaps) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Evidence Gaps (from question analysis)\n\n")
	b.WriteString("The following evidence requirements are NOT YET satisfied. ")
	b.WriteString("Prioritize reading files and extracting evidence that fills these gaps:\n\n")
	for _, g := range gaps {
		b.WriteString(g + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// ermFileScore scores a file by how well its symbols match ERM requirements.
// Higher score = file more likely to contain evidence that fills gaps.
func ermFileScore(fi *repomap.FileInfo, reqs []EvidenceRequirement) float64 {
	if fi == nil || len(reqs) == 0 {
		return 0
	}
	score := 0.0
	// Build a set of all entity names from unsatisfied requirements
	var unsatisfiedEntities []string
	for _, req := range reqs {
		if req.Status == "satisfied" {
			continue
		}
		unsatisfiedEntities = append(unsatisfiedEntities, req.Entities...)
	}
	if len(unsatisfiedEntities) == 0 {
		return 0
	}

	// Check file path for entity mentions
	pathLower := strings.ToLower(fi.RelPath)
	for _, ent := range unsatisfiedEntities {
		if strings.Contains(pathLower, strings.ToLower(ent)) {
			score += 2.0
		}
	}

	// Check symbol names for entity mentions
	for _, sym := range fi.Symbols {
		symLower := strings.ToLower(sym.Name)
		for _, ent := range unsatisfiedEntities {
			entLower := strings.ToLower(ent)
			if strings.Contains(symLower, entLower) || strings.Contains(entLower, symLower) {
				score += 1.0
				// Bonus for registration-like function names
				if isRegistrationLikeName(sym.Name) {
					score += 2.0
				}
				// Bonus for Name()/String() methods (return_value requirement)
				if sym.Kind == "method" && (sym.Name == "Name" || sym.Name == "String" || sym.Name == "Type") {
					score += 1.5
				}
			}
		}
	}

	return score
}

func isRegistrationLikeName(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range []string{"register", "bind", "setup", "init", "default", "provide", "subscribe"} {
		if strings.Contains(lower, prefix) {
			return true
		}
	}
	return false
}

// ermSuggestFiles returns files the ERM thinks should be read to fill
// evidence gaps, based on symbol table matching. Returns up to maxFiles
// suggestions with reasons.
func ermSuggestFiles(graph *repomap.Graph, reqs []EvidenceRequirement, readSet map[string]bool, maxFiles int) []ermFileSuggestion {
	if graph == nil || len(reqs) == 0 {
		return nil
	}

	type scored struct {
		path   string
		score  float64
		reason string
	}
	var candidates []scored

	for _, fi := range graph.Files {
		if readSet[fi.RelPath] {
			continue // already read
		}
		s := ermFileScore(fi, reqs)
		if s <= 0 {
			continue
		}
		// Build reason from matching symbols
		var matchedSyms []string
		for _, sym := range fi.Symbols {
			for _, req := range reqs {
				if req.Status == "satisfied" {
					continue
				}
				for _, ent := range req.Entities {
					if strings.Contains(strings.ToLower(sym.Name), strings.ToLower(ent)) {
						matchedSyms = append(matchedSyms, sym.Name)
						break
					}
				}
			}
		}
		reason := fmt.Sprintf("contains symbols: %s", strings.Join(matchedSyms, ", "))
		candidates = append(candidates, scored{path: fi.RelPath, score: s, reason: reason})
	}

	// Sort by score descending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	if len(candidates) > maxFiles {
		candidates = candidates[:maxFiles]
	}

	result := make([]ermFileSuggestion, len(candidates))
	for i, c := range candidates {
		result[i] = ermFileSuggestion{Path: c.path, Score: c.score, Reason: c.reason}
	}
	return result
}

type ermFileSuggestion struct {
	Path   string
	Score  float64
	Reason string
}

// registrationEligibleKinds lists Symbol.Kind values that can
// legitimately be the target of a literal registration call like
// `Register(NewFoo)` or `registry[key] = Foo{}`. Interfaces, methods,
// fields, packages, and traits are excluded because they are not
// directly registrable — an interface's concrete implementer is the
// registration target, not the interface itself; a method is reached
// via its parent type; a field/package/trait is structural. Kinds are
// matched case-insensitively against the canonical values emitted by
// the tree-sitter extractors in internal/tool/repomap/extract_*.go.
var registrationEligibleKinds = map[string]bool{
	"function": true,
	"struct":   true,
	"class":    true,
	"type":     true,
	"const":    true,
	"var":      true,
	"enum":     true,
}

// isRegistrationTargetKind reports whether any of the given symbol
// definitions has a Kind that could be a literal registration target.
func isRegistrationTargetKind(defs []*repomap.Symbol) bool {
	for _, d := range defs {
		if d == nil {
			continue
		}
		if registrationEligibleKinds[strings.ToLower(d.Kind)] {
			return true
		}
	}
	return false
}

// hasConcreteRegistrationTarget reports whether any entity in the
// requirement refers to a graph symbol that could be a registration
// target. Matching is exact-name (case-insensitive) against
// graph.SymbolDefs — the permissive substring match used for other
// Kinds is not safe here because words like `synthesis` / `continuation`
// substring-hit unrelated symbol names and file paths but are not
// themselves registrable. See docs/latency-analysis-2026-04-13.md §2.1
// for the t1 self-dispatch bug this guards against.
func hasConcreteRegistrationTarget(entities []string, graph *repomap.Graph) bool {
	if graph == nil {
		return false
	}
	for _, ent := range entities {
		if ent == "" {
			continue
		}
		entLower := strings.ToLower(ent)
		for symName, defs := range graph.SymbolDefs {
			if strings.ToLower(symName) != entLower {
				continue
			}
			if isRegistrationTargetKind(defs) {
				return true
			}
		}
	}
	return false
}

// ermAutoSatisfyUnresolvable marks requirements as "satisfied" when
// they can never be resolved by the evidence pipeline. Two layers:
//
//  1. Registration-specific gate: if req.Kind == "registration" and
//     no entity in the req has an exact-name symbol with a
//     registration-eligible Kind (function / struct / class / type /
//     const / var / enum), auto-satisfy. This kills the explorer
//     self-dispatch loop caused by interface-method names
//     (`SynthesizingEvaluator`) or abstract concept verbs
//     (`synthesis`, `continuation`) that substring-hit unrelated
//     symbols but are not registrable. Analyzed in
//     docs/latency-analysis-2026-04-13.md §2.
//
//  2. Generic fallback: if no entity substring-matches any symbol
//     name or file path, the entity is simply not present in the
//     codebase and the requirement is "not applicable". This
//     preserves the original filter for generic English words from
//     analyzer-rewritten tasks (e.g. "list", "count", "agents").
//
// Both filters are data-driven — checked against the repo's symbol
// table and file index — not hardcoded stopword lists.
func ermAutoSatisfyUnresolvable(reqs []EvidenceRequirement, graph *repomap.Graph) []EvidenceRequirement {
	if graph == nil || len(reqs) == 0 {
		return reqs
	}
	for i := range reqs {
		req := &reqs[i]
		if req.Status == "satisfied" {
			continue
		}
		// Layer 1: registration-specific gate.
		if req.Kind == "registration" && len(req.Entities) > 0 {
			if !hasConcreteRegistrationTarget(req.Entities, graph) {
				req.Status = "satisfied"
				continue
			}
		}
		// Layer 2: generic substring fallback — does ANY entity
		// appear anywhere in the codebase (symbol name or file path)?
		hasCodeMatch := false
		for _, ent := range req.Entities {
			entLower := strings.ToLower(ent)
			// Check symbol definitions
			for symName := range graph.SymbolDefs {
				if strings.Contains(strings.ToLower(symName), entLower) {
					hasCodeMatch = true
					break
				}
			}
			if hasCodeMatch {
				break
			}
			// Check file paths
			for _, fi := range graph.Files {
				if strings.Contains(strings.ToLower(fi.RelPath), entLower) {
					hasCodeMatch = true
					break
				}
			}
			if hasCodeMatch {
				break
			}
		}
		if !hasCodeMatch {
			req.Status = "satisfied" // not applicable — entity doesn't exist in codebase
		}
	}
	return reqs
}

// ermAllSatisfied returns true if all requirements are satisfied.
func ermAllSatisfied(reqs []EvidenceRequirement) bool {
	for _, req := range reqs {
		if req.Status != "satisfied" {
			return false
		}
	}
	return true
}

// formatERMStatuses renders a compact one-line summary of a
// []EvidenceRequirement suitable for a single debug log entry. Each
// requirement becomes `kind(ent1,ent2)=status`, joined by `; `. Used
// by the explorer's S1 soft-stop diagnostics to collapse what used to
// be a ~5-line multi-entry dump into a single line per check.
func formatERMStatuses(reqs []EvidenceRequirement) string {
	if len(reqs) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(reqs))
	for _, r := range reqs {
		parts = append(parts, fmt.Sprintf("%s(%s)=%s",
			r.Kind, strings.Join(r.Entities, ","), r.Status))
	}
	return strings.Join(parts, "; ")
}

// isRegistrationShape reports whether an EvidenceItem matches the
// canonical "registration linkage" shape — an EvidenceConcrete whose
// predicate contains "binds" (e.g. "binds ONLY", "binds first").
//
// Single source of truth used by both `identifyAnswerChains` (which
// classifies these as candidate Ground Truth answer chains) and the
// `case "registration"` branch of `checkRequirementSatisfaction` (which
// uses them to satisfy registration requirements without depending on
// LLM-tagged [REGISTRATION] notes). Keeping the predicate in one helper
// prevents the two consumers from drifting apart as the Concrete Values
// extractor evolves.
func isRegistrationShape(ev types.EvidenceItem) bool {
	return ev.Kind == types.EvidenceConcrete && strings.Contains(ev.Predicate, "binds")
}

// answerPredicateWhitelist controls which evidence kinds/predicates
// `identifyAnswerChains` will consider as candidate answers. The base
// set (chains + binds + returns) is always on. ERM-Kind-specific slots
// are opened by buildAnswerWhitelist so questions about conditions,
// call chains, config mappings, etc. can land structured evidence into
// Ground Truth instead of being filtered out.
type answerPredicateWhitelist struct {
	allowConditional      bool // EvidenceConditional (any predicate)
	allowRelationshipCall bool // EvidenceRelationship + predicate "calls"
	allowMechanismConfig  bool // EvidenceMechanism + predicate "reads_config"
	allowMechanismAny     bool // EvidenceMechanism (any predicate)
	allowRelationshipAny  bool // EvidenceRelationship (any predicate)
}

// buildAnswerWhitelist derives predicate-opening flags from the ERM
// requirements active for the current question. Mapping is one-way:
// each ERM Kind opens the predicates it can be answered by. Kinds with
// no mapping leave the whitelist at the base set.
func buildAnswerWhitelist(reqs []EvidenceRequirement) answerPredicateWhitelist {
	var w answerPredicateWhitelist
	for _, r := range reqs {
		switch r.Kind {
		case "conditional":
			w.allowConditional = true
		case "call_chain":
			w.allowRelationshipCall = true
		case "config_mapping":
			w.allowMechanismConfig = true
			w.allowConditional = true
		case "mechanism":
			// Reserved for T2.1 (mechanism Kind). Opens broad mechanism
			// + relationship slots so the future mechanism scanner has
			// a delivery channel into Ground Truth.
			w.allowMechanismAny = true
			w.allowRelationshipAny = true
		}
	}
	return w
}

// identifyAnswerChains scores resolution chains and concrete values
// against the user's question and returns the ones that most directly
// answer it. These are deterministic ground-truth facts that should be
// presented to the finalizer with priority, not mixed into the general
// evidence pool.
//
// A chain is "answer-relevant" if its text mentions entities from the
// question. The score is the fraction of question entities matched.
// Returns up to maxChains formatted strings, sorted by relevance.
//
// `whitelist` opens additional evidence kinds/predicates beyond the base
// set (resolution_chain + binds + returns) per ERM Kind. See
// buildAnswerWhitelist.
//
// `reqs` and `graph` enable L0-1 terminal verification: a post-rank
// discriminative check that demotes (×0.2) chains whose terminal
// segment is structurally incompatible with the question's Kind —
// e.g. chains ending at a Go `range` loop header when the question
// asks for a registered symbol. Demoted chains are still returned as
// a fallback safety net; they simply never outrank a passing chain.
// Callers may pass nil for both to opt out and preserve legacy
// ranking behaviour (used by older tests).
func identifyAnswerChains(question string, evidence []types.EvidenceItem, maxChains int, whitelist answerPredicateWhitelist, reqs []EvidenceRequirement, graph *repomap.Graph) ([]string, []types.EvidenceItem) {
	entities := extractRankingEntitiesWithGraph(question, graph)
	if len(entities) == 0 || len(evidence) == 0 {
		return nil, nil
	}

	// L0-1: pre-compute terminal + origin predicates once per call.
	// Empty slices when no active kind has a predicate, in which case
	// the per-candidate checks below become no-ops.
	terminalPreds := terminalPredicatesFor(reqs)
	originPreds := originPredicatesFor(reqs)

	// Candidates are scored into two aligned fields: `text` for the
	// display-rendered chain (loose, demote-not-drop) and `src` for
	// the underlying EvidenceItem (strict, only items passing ALL
	// applicable predicates). Both paths share the same scoring so
	// callers can treat them as two views of one ranked list.
	//
	// Sort keys for multi-key stable ordering (2026-04-12 user-
	// requested ordering discipline, see memory/project_answer_chain_stable_sort.md):
	//   1. score         — descending (primary relevance)
	//   2. strictOK      — true first (L0-1 predicate-passing items
	//                      win ties against demoted ones)
	//   3. confidence    — descending (from ev.Confidence)
	//   4. chainLength   — ascending (shorter chains are more
	//                      precise / less indirection)
	//   5. sourceLine    — ascending (earlier code wins ties, with
	//                      a sentinel for items without a line)
	//   6. summary       — lexicographic tie-break final key,
	//                      guarantees a total order so results are
	//                      deterministic across Go runtime hash seeds
	//                      and call orderings.
	type scored struct {
		text        string
		score       float64
		src         types.EvidenceItem // untouched source item
		strictOK    bool               // passed all applicable predicates
		confidence  float64            // mirror of src.Confidence, cached for sort
		chainLength int                // hop count: arrow count + 1, min 1
		sourceLine  int                // src.LineStart, or noSourceLine sentinel
		summary     string             // lex tie-break final key
	}
	var candidates []scored

	for _, ev := range evidence {
		// Base set: resolution chains and concrete registrations/returns.
		isChain := ev.Kind == types.EvidenceDataflowPath && ev.Predicate == "resolution_chain"
		isRegistration := isRegistrationShape(ev)
		isConcreteReturn := ev.Kind == types.EvidenceConcrete && ev.Predicate == "returns"
		// ERM-Kind-opened slots (T1.3).
		isCondition := whitelist.allowConditional && ev.Kind == types.EvidenceConditional
		isCallRel := whitelist.allowRelationshipCall && ev.Kind == types.EvidenceRelationship && ev.Predicate == "calls"
		isConfigMech := whitelist.allowMechanismConfig && ev.Kind == types.EvidenceMechanism && ev.Predicate == "reads_config"
		isMechAny := whitelist.allowMechanismAny && ev.Kind == types.EvidenceMechanism
		isRelAny := whitelist.allowRelationshipAny && ev.Kind == types.EvidenceRelationship
		if !isChain && !isRegistration && !isConcreteReturn &&
			!isCondition && !isCallRel && !isConfigMech && !isMechAny && !isRelAny {
			continue
		}

		// Strip file-path locators before substring matching — see
		// memory/project_next_session_kickoff_filepath_entity_bug.md.
		// Without this, a short lowercase entity that names a package
		// directory (e.g. `agent`) matches every chain whose Summary
		// embeds `internal/agent/...`, so package layout trumps
		// semantic relevance during ranking.
		text := normalizeForMatch(stripPathTokens(ev.Summary + " " + ev.Subject + " " + ev.Object))
		overlap := 0
		for _, ent := range entities {
			if strings.Contains(text, normalizeForMatch(ent)) {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}

		display := ev.Summary
		if display == "" {
			display = fmt.Sprintf("[%s] %s %s %s", ev.Kind, ev.Subject, ev.Predicate, ev.Object)
		}
		if ev.Source != "" {
			display += fmt.Sprintf(" (%s", ev.Source)
			if ev.LineStart > 0 {
				display += fmt.Sprintf(":%d", ev.LineStart)
			}
			display += ")"
		}

		// Chains get a bonus because they contain multi-hop reasoning
		bonus := 1.0
		if isChain {
			bonus = 2.0
		}
		// Shape-based bonus: chains whose rightmost segment ends in a
		// short literal `returns "x"` (Name/Type/Kind-style identity
		// returns) are canonical resolved answers — as opposed to
		// chains ending in long description strings, constructor
		// returns, or assignments. This breaks ties between chains
		// with equal entity overlap deterministically, without
		// depending on chain iteration order.
		if isChain && endsWithShortLiteralReturn(ev.Summary) {
			bonus *= 1.5
		}
		// Additional shape-based bonus: chains whose first segment is
		// a `binds` verb (registration linkage) are stronger answers
		// to "which X does Y?" questions than chains starting with a
		// constructor (`returns &Foo{`). Combined with the short-literal
		// bonus, this disambiguates `Register(NewFoo) → Foo.Name() returns "x"`
		// from `NewFoo() returns &Foo{} → Foo.Name() returns "x"` — both
		// end in a short literal but the register-linked one is the
		// canonical registration-driven answer shape.
		if isChain && firstSegmentIsBinds(ev.Summary) {
			bonus *= 1.3
		}

		// L0-1: predicate checks. strictOK tracks whether the item
		// passed ALL applicable predicates; used later to build the
		// strict subset for L0-2 consumption. Failing items are
		// still kept in the loose list (demote-not-drop) for the
		// Ground Truth display.
		strictOK := true
		if len(terminalPreds) > 0 {
			for _, p := range terminalPreds {
				if !p(ev.Summary, graph) {
					bonus *= 0.2
					strictOK = false
					preview := ev.Summary
					if len(preview) > 120 {
						preview = preview[:120] + "..."
					}
					logging.Debug("[erm] L0-1 terminal predicate demoted chain: %s", preview)
					break
				}
			}
		}
		if strictOK && len(originPreds) > 0 {
			for _, p := range originPreds {
				if !p(ev.Summary, graph) {
					bonus *= 0.1
					strictOK = false
					preview := ev.Summary
					if len(preview) > 120 {
						preview = preview[:120] + "..."
					}
					logging.Debug("[erm] L0-1 origin predicate demoted chain: %s", preview)
					break
				}
			}
		}

		// Chain length: number of hops. Counted as (→ arrows + 1).
		// An arrow-less Summary is 1 hop (a bare Subject-predicate-
		// Object triple), a 1-arrow chain is 2 hops, etc. Items with
		// empty Summary get chainLength=1 since the {Subject, Object}
		// pair functions as a single hop. Lower is more precise —
		// fewer intermediate indirections between the question entity
		// and the terminal answer.
		chainLen := strings.Count(ev.Summary, "→") + 1
		if chainLen < 1 {
			chainLen = 1
		}

		// Source line sentinel: items without a line number sort
		// AFTER items with a line number, so a chain anchored at a
		// concrete source location wins ties against a floating
		// chain built from LLM notes that never resolved a line.
		srcLine := ev.LineStart
		if srcLine <= 0 {
			srcLine = noSourceLineSentinel
		}

		candidates = append(candidates, scored{
			text:        display,
			score:       float64(overlap) / float64(len(entities)) * bonus,
			src:         ev,
			strictOK:    strictOK,
			confidence:  ev.Confidence,
			chainLength: chainLen,
			sourceLine:  srcLine,
			summary:     ev.Summary,
		})
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Stable multi-key sort. SliceStable (not Slice) keeps equal-
	// keyed candidates in their original insertion order, so the
	// final tie-break defaults to "came from evidence[] first" —
	// relevant when two items share ALL sort keys including the
	// lex-ordered summary. See memory/project_answer_chain_stable_sort.md.
	sort.SliceStable(candidates, func(i, j int) bool {
		ci, cj := candidates[i], candidates[j]
		// 1. score descending
		if ci.score != cj.score {
			return ci.score > cj.score
		}
		// 2. strictOK=true first (L0-1 passing items beat demoted)
		if ci.strictOK != cj.strictOK {
			return ci.strictOK
		}
		// 3. confidence descending
		if ci.confidence != cj.confidence {
			return ci.confidence > cj.confidence
		}
		// 4. chainLength ascending (shorter = more precise)
		if ci.chainLength != cj.chainLength {
			return ci.chainLength < cj.chainLength
		}
		// 5. sourceLine ascending (earlier code wins)
		if ci.sourceLine != cj.sourceLine {
			return ci.sourceLine < cj.sourceLine
		}
		// 6. summary lexicographic — deterministic final tie-break
		//    so the result is stable across Go runtime hash seeds
		//    and iteration-order noise.
		return ci.summary < cj.summary
	})

	// Build two aligned outputs:
	//  - chains: loose list of display strings (includes demoted
	//    candidates for Ground Truth fallback safety)
	//  - strictItems: filtered list of EvidenceItems that passed
	//    all applicable predicates, for L0-2 structured extraction
	// Both are deduplicated independently: chains by text, strictItems
	// by (Subject, Predicate, Object, Source, LineStart).
	seenText := make(map[string]bool)
	var chains []string
	seenEv := make(map[string]bool)
	var strictItems []types.EvidenceItem
	for _, c := range candidates {
		if !seenText[c.text] {
			seenText[c.text] = true
			if len(chains) < maxChains {
				chains = append(chains, c.text)
			}
		}
		if c.strictOK {
			key := fmt.Sprintf("%s|%s|%s|%s|%d", c.src.Subject, c.src.Predicate, c.src.Object, c.src.Source, c.src.LineStart)
			if !seenEv[key] {
				seenEv[key] = true
				if len(strictItems) < maxChains {
					strictItems = append(strictItems, c.src)
				}
			}
		}
	}
	return chains, strictItems
}

// AnswerRole classifies which hop of an evidence chain should be
// extracted as the answer. Added 2026-04-12 (Phase 2 of the
// extractAnswerSymbols audit; see
// memory/project_answer_symbol_extraction_audit.md). Pre-audit code
// always extracted the same role by evidence shape, silently giving
// the wrong symbol for reverse-reference and link-identity questions.
type AnswerRole int

const (
	// RoleTerminal is the legacy default. The answer is at the
	// rightmost/terminal position, extracted per evidence shape:
	// the registered class for `binds`, the receiver type for
	// `returns`, the callee for `calls`, the rightmost hop for
	// `resolution_chain`. This matches pre-Phase-2 behavior and is
	// correct for forward-reference questions ("what does X
	// register / return / call").
	RoleTerminal AnswerRole = iota
	// RoleOrigin points to the leftmost hop's caller. Used for
	// reverse-reference questions ("who calls X", "who registers
	// Y", "谁调用 X", "which component initializes Z").
	RoleOrigin
	// RoleAnchor prefers a string literal found anywhere in the
	// chain, walking right-to-left. Used when the answer IS the
	// bridging literal: return-value questions ("what does X
	// return"), name-of questions ("X 的名称是什么"), and reverse
	// enumeration over a relationship ("how many agents can call
	// sub-agents" — answer is the name literal that bridges caller
	// and registry).
	RoleAnchor
)

// classifyAnswerRole reads the user's original question (and the
// analyzer-declared kind for completeness) to decide which part of
// the evidence chain the answer lives in. Keyword-based, covering
// both English and Chinese. Empty question → RoleTerminal (legacy
// compatibility — unit tests that don't supply a question still
// exercise the pre-Phase-2 extraction).
//
// Over-fit audit: all rules are structural (verb / pronoun
// patterns). No eval case's specific entities (SubExplorer,
// propose_sub_agents, etc.) appear in the rules — deleting any one
// eval case does not weaken or strengthen any rule. Removing a
// single rule loses a class of questions, not one eval case, which
// is the definition of structural coverage.
func classifyAnswerRole(question, declaredKind string) AnswerRole {
	if question == "" {
		return RoleTerminal
	}
	lower := strings.ToLower(question)

	// Declared-kind fast path (2026-04-12 REPL audit finding): the
	// analyzer sets `question_kind="enumeration"` for "how many X can
	// Y" / "哪些 X 能 Y" questions regardless of how it rewrites the
	// title. When the analyzer classified enumeration AND the question
	// text contains a relationship verb, route directly to RoleAnchor
	// — the literal bridging the two entities is the answer.
	//
	// Real-scenario trigger: "how many agents can invoke subagent"
	// got rewritten to title="List agents that can invoke a subagent".
	// The title's "List agents" doesn't match my count-cue list
	// ("list the", "list all"), so countEn stayed false and the
	// classifier defaulted to RoleTerminal — picking the callee
	// class `SubExplorer` instead of the caller literal `explorer`.
	// The declaredKind hint short-circuits this: enumeration +
	// relationship verb ⇒ RoleAnchor, bypassing keyword fragility.
	if declaredKind == "enumeration" {
		relVerbs := []string{"call", "invoke", "use", "read", "write",
			"register", "handle", "dispatch", "bind", "consume"}
		for _, v := range relVerbs {
			if strings.Contains(lower, v) {
				return RoleAnchor
			}
		}
	}

	// RoleOrigin cues (English): reverse-reference verb patterns.
	// Pattern: "who/which <noun>? <reverse-verb>".
	reverseVerbs := []string{
		"calls", "call ", "calling",
		"invokes", "invoke ", "invoking",
		"uses", "use ", "using",
		"reads", "read ", "reading",
		"writes", "write ", "writing",
		"registers", "register ", "registering",
		"imports", "import ", "importing",
		"references", "referencing",
		"creates", "create ", "creating",
		"initializes", "initialize", "initializing",
		"instantiates", "instantiate",
		"binds", "bind ", "binding",
		"handles", "handle ", "handling",
		"dispatches", "dispatch ",
	}
	if strings.HasPrefix(lower, "who ") || strings.HasPrefix(lower, "which ") || strings.Contains(lower, "where is ") {
		for _, v := range reverseVerbs {
			if strings.Contains(lower, v) {
				return RoleOrigin
			}
		}
	}
	// RoleOrigin cues (Chinese): 谁 + verb.
	if strings.Contains(question, "谁") {
		return RoleOrigin
	}

	// RoleAnchor cues: return-value + name-of questions.
	// English: "what does X return/yield", "what is the name of X".
	if strings.Contains(lower, "what does") && (strings.Contains(lower, "return") || strings.Contains(lower, "yield")) {
		return RoleAnchor
	}
	if strings.Contains(lower, "what is the name") || strings.Contains(lower, "what's the name") {
		return RoleAnchor
	}
	// Chinese: 返回什么, X 的名称, X 返回的值
	if strings.Contains(question, "返回") || strings.Contains(question, "的名称") || strings.Contains(question, "名字是") {
		return RoleAnchor
	}

	// RoleAnchor cues: reverse enumeration over a relationship.
	// The answer is the shared identity that makes the relationship
	// hold — "how many X can call Y" / "多少 X 可以调用 Y".
	// English count/enumeration cues. Broadened to match the
	// analyzer's typical rewrites of 多少/哪些 questions into formal
	// English ("Determine the number of X", "Count the X", "List
	// the X"), mirroring the Chinese list above.
	countEn := strings.Contains(lower, "how many") ||
		strings.Contains(lower, "count of") ||
		strings.Contains(lower, "count the") ||
		strings.Contains(lower, "determine the number") ||
		strings.Contains(lower, "the number of") ||
		strings.Contains(lower, "list the") ||
		strings.Contains(lower, "list all") ||
		strings.Contains(lower, "find all") ||
		strings.Contains(lower, "enumerate")
	relVerbEn := false
	for _, v := range []string{"call", "invoke", "use", "read", "register", "handle", "dispatch"} {
		if strings.Contains(lower, v) {
			relVerbEn = true
			break
		}
	}
	if countEn && relVerbEn {
		return RoleAnchor
	}
	// Chinese count/enumeration cues. The list must cover BOTH the
	// raw user phrasing (多少, 几个, 哪些) AND the analyzer's typical
	// rewrite into more formal Chinese (数量, 统计, 计算, 列出, 哪几).
	// The real-scenario re-test on 2026-04-12 hit this: the user
	// typed "有多少个agent可以调用subagent" but the analyzer task title
	// was "统计可以调用subagent的agent数量" — 数量/统计 had to be in the
	// list or classifyAnswerRole would silently fall through to
	// RoleTerminal.
	countZh := strings.Contains(question, "多少") ||
		strings.Contains(question, "几个") ||
		strings.Contains(question, "有几") ||
		strings.Contains(question, "数量") ||
		strings.Contains(question, "统计") ||
		strings.Contains(question, "计算") ||
		strings.Contains(question, "列出") ||
		strings.Contains(question, "哪几") ||
		strings.Contains(question, "哪些")
	relVerbZh := strings.Contains(question, "调用") || strings.Contains(question, "使用") ||
		strings.Contains(question, "注册") || strings.Contains(question, "可以") ||
		strings.Contains(question, "能够") || strings.Contains(question, "处理")
	if countZh && relVerbZh {
		return RoleAnchor
	}

	// Default: forward-reference, preserves legacy behavior.
	return RoleTerminal
}

// splitHops splits an evidence's chain representation into an
// ordered list of hop segments. If Summary contains `→` (U+2192),
// split by it. Otherwise synthesize a 2-hop list from Subject +
// Object (the natural interpretation of a Subject→Object triple).
// Empty segments are dropped.
func splitHops(ev types.EvidenceItem) []string {
	const arrow = "→"
	if strings.Contains(ev.Summary, arrow) {
		parts := strings.Split(ev.Summary, arrow)
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	var out []string
	if ev.Subject != "" {
		out = append(out, ev.Subject)
	}
	if ev.Object != "" {
		out = append(out, ev.Object)
	}
	return out
}

// extractCallerName scans a chain hop for the first "<Ident>(" call
// expression and returns the identifier, stripped of a leading
// "New" prefix for Go constructors. Treats backticks as non-ident
// so that hop text like "`NewOrchestrator()` binds ..." picks up
// NewOrchestrator correctly. Minimum length 3 to avoid noise.
func extractCallerName(hop string) string {
	start := -1
	for i := 0; i < len(hop); i++ {
		c := hop[i]
		if start < 0 {
			if isIdentStart(c) {
				start = i
			}
			continue
		}
		if isIdentChar(c) {
			continue
		}
		if c == '(' {
			name := hop[start:i]
			if len(name) >= 3 {
				return stripNewPrefix(name)
			}
		}
		start = -1
	}
	return ""
}

// extractQuotedLiteral returns the first string literal found in a
// hop, stripped of surrounding quotes. Prefers a literal following
// an explicit `returns ` anchor (to avoid picking up unrelated
// literals in the hop prose); falls back to the first quoted run
// anywhere in the hop. Accepts both " and ' to cover Python / JS /
// Ruby single-quoted strings.
func extractQuotedLiteral(hop string) string {
	if idx := strings.Index(hop, "returns "); idx >= 0 {
		rest := strings.TrimSpace(hop[idx+len("returns "):])
		if len(rest) >= 2 {
			q := rest[0]
			if q == '"' || q == '\'' {
				if end := strings.IndexByte(rest[1:], q); end >= 0 {
					return rest[1 : 1+end]
				}
			}
		}
	}
	for i := 0; i < len(hop); i++ {
		q := hop[i]
		if q != '"' && q != '\'' {
			continue
		}
		if end := strings.IndexByte(hop[i+1:], q); end >= 0 {
			lit := hop[i+1 : i+1+end]
			if lit != "" {
				return lit
			}
		}
	}
	return ""
}

// pickHop walks an evidence chain according to role and returns
// the extracted symbol/literal. See memory/project_answer_symbol_extraction_audit.md
// for the design rationale. Returns "" when no symbol can be
// determined so the caller can drop the item.
func pickHop(ev types.EvidenceItem, role AnswerRole, graph *repomap.Graph) string {
	switch role {
	case RoleOrigin:
		for _, hop := range splitHops(ev) {
			if name := extractCallerName(hop); name != "" {
				return name
			}
		}
		return ""
	case RoleAnchor:
		// Walk right-to-left for an IDENTIFIER-shaped string literal.
		// Strict: if no identifier-shaped literal is found we return
		// "" (the caller drops this item) rather than falling through
		// to pickTerminalLegacy.
		//
		// Why strict: for an anchor question, the answer IS a
		// literal (a name, a path, an ID). An evidence item with no
		// literal in any hop doesn't carry an answer-shaped fact —
		// falling back to the legacy class-name pick would re-
		// introduce the bug the audit was opened to fix. Concretely,
		// the 2026-04-12 real-scenario repro had two evidence items
		// for the same registration: one WITH the `returns "explorer"`
		// trailing hop (literal present, correctly yields "explorer"),
		// and one WITHOUT it (literal absent; legacy would pick
		// "SubExplorer", re-polluting the answer set with the
		// callee class). Dropping the second item produces a clean
		// single-symbol result.
		//
		// Why identifier-shaped (2026-04-13 Phase 4 repro): when the
		// Phase 4 gate promotes a batch to RoleAnchor, that batch
		// may contain unrelated evidence whose terminal hop carries
		// a long narrative literal — e.g. `GrepTool.Description()
		// returns "Search file contents..."`. These are legitimate
		// `returns "literal"` shapes but NOT the bridging identity
		// the user asked for. The analyzer's list_of_symbols
		// contract explicitly scopes the answer to "a SET of
		// IDENTIFIER NAMES"; filtering here to identifier-shaped
		// literals keeps the walker aligned with that contract
		// without adding any NL/keyword logic. Non-identifier
		// literals are skipped in the same walk; if an item has no
		// identifier literal in ANY hop, it falls out via the
		// strict-drop path unchanged.
		hops := splitHops(ev)
		for i := len(hops) - 1; i >= 0; i-- {
			lit := extractQuotedLiteral(hops[i])
			if lit == "" || !isIdentifierLiteral(lit) {
				continue
			}
			return lit
		}
		return ""
	case RoleTerminal:
		fallthrough
	default:
		return pickTerminalLegacy(ev, graph)
	}
}

// firstIdent returns the first identifier-shaped token in seg,
// accepting both CamelCase/PascalCase (Go/Java) and snake_case or
// lowercase-first camelCase (Python/Ruby/JS/YAML). Structural
// filter: token must be [A-Za-z_][A-Za-z0-9_]{2,} — minimum 3
// characters to keep noise (single-letter receivers, 2-char Go
// idioms like `io`) out of the picker.
//
// This is the Phase 3 complement to firstUppercaseIdent. The
// legacy picker stays in the Go-specific extraction paths
// (registration, resolution chains with Go symbols) where
// uppercase-only is the right safety net. firstIdent is only
// used where the producing evidence shape is language-agnostic:
// the decorator case (Python / Java / JS annotations), the map
// case (any language's key→value literal), and the returns-case
// fallback (snake_case receiver types for Python / Ruby).
func firstIdent(seg string) string {
	n := len(seg)
	for i := 0; i < n; i++ {
		c := seg[i]
		if i > 0 && isIdentChar(seg[i-1]) {
			continue
		}
		if !isIdentStart(c) {
			continue
		}
		j := i
		for j < n && isIdentChar(seg[j]) {
			j++
		}
		if j-i >= 3 {
			return seg[i:j]
		}
		i = j - 1
	}
	return ""
}

// rightmostArrowHop returns the trimmed substring after the last
// U+2192 ("→") in s. Used by the decorator / map cases to pull the
// "target" side of a `key → value` or `@decorator(args) → target`
// expression when the hop list is embedded in a single Object
// field rather than in Summary.
func rightmostArrowHop(s string) string {
	const arrow = "→"
	if idx := strings.LastIndex(s, arrow); idx >= 0 {
		return strings.TrimSpace(s[idx+len(arrow):])
	}
	return strings.TrimSpace(s)
}

// pickTerminalLegacy is the per-shape extraction routed by
// evidence Predicate/Kind. Preserved as the RoleTerminal default
// so forward-reference questions and all existing unit tests
// continue to produce the same symbol they produced before the
// audit. Phase 3 additions: `decorates` and `maps` predicates
// (previously unrouted), plus a snake_case fallback on the
// returns branch for Python/Ruby lowercase receivers.
func pickTerminalLegacy(ev types.EvidenceItem, graph *repomap.Graph) string {
	switch {
	case isRegistrationShape(ev):
		return stripNewPrefix(firstUppercaseIdent(ev.Object))
	case ev.Kind == types.EvidenceConcrete && ev.Predicate == "returns":
		sub := ev.Subject
		if dot := strings.Index(sub, "."); dot > 0 {
			sub = sub[:dot]
		}
		// Try the strict Go-style picker first (back-compat with
		// TestAnswerSymbolFromEvidence_ReturnsMethod and every other
		// existing Go-returns test). Fall back to language-agnostic
		// firstIdent only when the strict picker draws a blank —
		// that's the "list_users.name" snake_case path that Phase 3
		// added coverage for.
		if name := firstUppercaseIdent(sub); name != "" {
			return name
		}
		return firstIdent(sub)
	case ev.Kind == types.EvidenceConcrete && ev.Predicate == "decorates":
		// `@app.route("/api/users") → list_users`
		// `@GetMapping("/api/foo") → getFoo`
		// The concrete-values extractor puts the whole hop-pair
		// into Object. Rightmost-arrow hop is the decorated
		// function, which is language-agnostic so it gets
		// firstIdent (handles both snake_case list_users and
		// lowercase-first camelCase getFoo).
		target := rightmostArrowHop(ev.Object)
		return firstIdent(target)
	case ev.Kind == types.EvidenceConcrete && ev.Predicate == "maps":
		// `"/api/users" → NewUserHandler()`
		// `types.AgentExplorer → NewExplorerAgent`
		// `"/foo" → getFoo`
		// Rightmost-arrow hop is the map value. Try
		// firstUppercaseIdent first (matches Go constructors
		// like NewUserHandler → UserHandler), fall back to
		// firstIdent for snake_case / lowercase-first handlers.
		val := rightmostArrowHop(ev.Object)
		if name := firstUppercaseIdent(val); name != "" {
			return stripNewPrefix(name)
		}
		return firstIdent(val)
	case ev.Kind == types.EvidenceRegistration:
		return stripNewPrefix(firstUppercaseIdent(ev.Object))
	case ev.Kind == types.EvidenceRelationship && ev.Predicate == "calls":
		return firstUppercaseIdent(ev.Object)
	case ev.Kind == types.EvidenceDataflowPath && ev.Predicate == "resolution_chain":
		terminal := extractTerminalSegment(ev.Summary)
		if p := strings.LastIndex(terminal, " ("); p >= 0 && strings.HasSuffix(terminal, ")") {
			terminal = strings.TrimSpace(terminal[:p])
		}
		return firstUppercaseIdent(terminal)
	case ev.Kind == types.EvidenceMechanism:
		return firstUppercaseIdent(ev.Subject)
	}
	return ""
}

// extractAnswerSymbols translates a pre-filtered slice of EvidenceItems
// into a structured AnswerSymbol list. The L0-2 translation step:
// runs AFTER identifyAnswerChains has produced the strict subset
// (the second return value) and BEFORE the finalizer is invoked. The
// caller is expected to pass only items that passed all applicable
// L0-1 predicates; this function does NOT re-apply them.
//
// Phase 2 (2026-04-12): classify the answer role from the user's
// question once, pass it into answerSymbolFromEvidence so reverse-
// reference and link-identity questions can pick the correct hop.
// Legacy (empty question / RoleTerminal) path is untouched. See
// memory/project_answer_symbol_extraction_audit.md.
//
// Phase 4 (2026-04-13): answer-shape-gated literal-termination
// dominance. The Phase 2 NL classifier reads analyzer-rewritten
// surface forms and can miss reverse-reference questions when the
// analyzer picks a rewrite that doesn't match any keyword cue (real
// 2026-04-13 repro: "which agents can invoke subagent" → analyzer
// title "Identify subagent-invoking agents" + question_kind
// "call_chain" → classifier defaulted to RoleTerminal → picked
// callee class SubExplorer instead of bridging literal "explorer").
//
// The fix is a STRUCTURAL gate independent of question-text keyword
// matching: when the analyzer declared answer_shape=list_of_symbols
// AND the classifier's default would be RoleTerminal AND at least
// one evidence item carries a terminal string literal, promote the
// role to RoleAnchor. The promoted role's strict-drop semantics
// ensures items without literals are discarded rather than falling
// back to shape-dispatched class picks that would re-pollute the
// answer set.
//
// Why answer_shape: the analyzer prompt (analyzer.go hard-rule 3)
// defines list_of_symbols as "user is asking for a SET of names
// they want to see listed. 'How many agents call X' is
// list_of_symbols (they want the names even if phrased as a
// count)". This is a documented contract on the analyzer's
// structured output and is INDEPENDENT of how the analyzer rewrites
// the title — unlike question_kind which drifted between
// call_chain/enumeration across LLM runs on the same question.
//
// Over-fit audit: the rule is purely shape- and schema-driven. No
// identifier names, no question keywords, no language tokens. The
// gate fires only when the analyzer contractually declared
// "answer is a set of names" AND the evidence graph actually
// carries a literal name to surface. Removing any single eval case
// does not weaken the rule.
//
// S2 trigger (unchanged): L0-2 no longer gates on analyzer-declared
// questionKind. The gate is EVIDENCE-DRIVEN — extract only if at
// least one item carries a single-symbol terminal shape.
//
// Extraction reads STRUCTURED fields (Subject / Object / Source /
// LineStart) or parses the chain via splitHops/pickHop. The
// pre-Phase-2 per-shape behaviour is preserved under RoleTerminal
// so forward-reference questions and all pre-existing unit tests
// continue to produce the same symbol.
func extractAnswerSymbols(items []types.EvidenceItem, questionKind, question, answerShape string, graph *repomap.Graph) []types.AnswerSymbol {
	if len(items) == 0 {
		return nil
	}
	// Mechanism questions never produce a symbol list.
	//
	// Mechanism answers are prose descriptions of HOW something works
	// — ordered sequences of steps each anchored by a file:line. Forcing
	// the finalizer into "Translation mode" (see finalizer.go §Translation
	// mode) on a mechanism question tells the LLM "mention EXACTLY these
	// symbols, no more no less," which degrades a step-by-step explanation
	// into a flat name list.
	//
	// Concrete df3 failure (docs/df3-file-selection-drift 2026-04-13):
	// the question "explorerEvaluator 的 ContinuationPrompt 是怎么实现的?
	// 有哪几种 push 策略?" is mechanism kind + step_list shape. The
	// pipeline produced [REGISTRATION]-shape and [MECHANISM]-shape
	// evidence items, hasTerminalEvidence returned true, and this
	// function extracted {subExplorerEvaluator, ContinuationPrompt}
	// as answer symbols. Translation mode then forced the finalizer
	// to write "the answer is subExplorerEvaluator and ContinuationPrompt"
	// — skipping every actual push strategy (partial-read, unread
	// high-priority, enumeration completeness, cross-reference, idle
	// streak). Returning nil here routes the finalizer to the
	// shape-based step_list prompt which asks for citations and
	// ordered steps — the right container for a how-does-X-work
	// answer.
	//
	// The early return is keyed on questionKind, NOT on the specific
	// entity or answer_shape, so it generalises across any mechanism
	// question in any language and doesn't depend on the analyzer's
	// specific wording.
	if strings.EqualFold(strings.TrimSpace(questionKind), "mechanism") {
		logging.Debug("[erm] extractAnswerSymbols: mechanism kind, skipping symbol extraction (finalizer will use shape prompt)")
		return nil
	}
	if !hasTerminalEvidence(items) {
		return nil
	}

	role := classifyAnswerRole(question, questionKind)

	// Phase 4 gate: promote Terminal → Anchor when the analyzer
	// declared the answer is a set of names AND the evidence set
	// contains an actual literal to surface. Anchor's strict-drop
	// walk then picks the bridging literal from any item that has
	// one, and discards items that only carry a class-name shape.
	if role == RoleTerminal && strings.EqualFold(answerShape, "list_of_symbols") && hasTerminalLiteral(items) {
		logging.Debug("[erm] Phase 4 gate: answer_shape=list_of_symbols and literal present; promoting role Terminal → Anchor")
		role = RoleAnchor
	}

	var out []types.AnswerSymbol
	seen := make(map[string]bool)
	for _, ev := range items {
		sym := answerSymbolFromEvidence(ev, questionKind, role, graph)
		if sym.Name == "" || seen[sym.Name] {
			continue
		}
		seen[sym.Name] = true
		out = append(out, sym)
	}
	return out
}

// hasTerminalEvidence reports whether any item in the strict subset
// carries a structurally single-symbol shape that answerSymbolFrom-
// Evidence can extract. Used by extractAnswerSymbols as the S2
// evidence-driven gate replacing the old question_kind whitelist.
//
// Phase 3 additions (2026-04-12): `decorates` and `maps` Concrete
// evidence are now recognised as terminal shapes. Both carry an
// `X → Y` hop pair where the terminal Y is a handler/value the
// pipeline can surface as an AnswerSymbol. See
// memory/project_answer_symbol_extraction_audit.md.
func hasTerminalEvidence(items []types.EvidenceItem) bool {
	for _, ev := range items {
		if isRegistrationShape(ev) {
			return true
		}
		if ev.Kind == types.EvidenceConcrete && ev.Predicate == "returns" {
			return true
		}
		if ev.Kind == types.EvidenceConcrete && ev.Predicate == "decorates" {
			return true
		}
		if ev.Kind == types.EvidenceConcrete && ev.Predicate == "maps" {
			return true
		}
		if ev.Kind == types.EvidenceRegistration {
			return true
		}
		if ev.Kind == types.EvidenceRelationship && ev.Predicate == "calls" {
			return true
		}
		if ev.Kind == types.EvidenceDataflowPath && ev.Predicate == "resolution_chain" {
			// Multi-hop chains are extractable if they contain an
			// arrow and the terminal segment has a structural symbol
			// reference. The chain already passed L0-1 predicates
			// upstream (strict subset), so it is terminal-shaped.
			return true
		}
	}
	return false
}

// isIdentifierLiteral reports whether a quoted string literal is
// shaped like an identifier name — the form the analyzer's
// `list_of_symbols` answer_shape contract asks us to surface. The
// rule is purely structural: no whitespace and length ≤ 64. This
// accepts real identifiers ("explorer", "propose_sub_agents",
// "widget-handler", "/api/v1/users", snake_case, kebab-case, route
// paths) and rejects docstrings, help text, tool descriptions, and
// any other narrative string that happens to live in a `returns
// "..."` hop. No hardcoded keywords, no name-specific patterns.
//
// Why 64: empirically, the longest well-formed identifier-ish
// literals in Go/Python/Java/TS/Ruby codebases (compound snake_case
// names, namespaced route paths) stay under 64 characters. A
// docstring or help text is typically hundreds of characters and
// always contains whitespace — either filter alone would catch it;
// both together make the filter defense-in-depth.
func isIdentifierLiteral(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			return false
		}
	}
	return true
}

// hasTerminalLiteral reports whether any item's evidence chain
// contains an IDENTIFIER-SHAPED quoted string literal in any hop
// (not just the last one — the bridging literal can be produced by
// a pre-terminal Name()/Key()/Type() call). Used by
// extractAnswerSymbols' Phase 4 gate to decide whether promoting
// the role to RoleAnchor would actually surface a usable literal —
// if every literal in the batch is a long docstring or help text,
// promotion would degrade to garbage, so we stay on the legacy
// Terminal path instead.
//
// Implementation is purely structural: for each item, splitHops the
// chain and scan each hop for a quoted literal using the same
// extractor pickHop(RoleAnchor) uses, then filter via
// isIdentifierLiteral. No identifier/keyword matching.
func hasTerminalLiteral(items []types.EvidenceItem) bool {
	for _, ev := range items {
		for _, hop := range splitHops(ev) {
			if lit := extractQuotedLiteral(hop); lit != "" && isIdentifierLiteral(lit) {
				return true
			}
		}
	}
	return false
}

// answerSymbolFromEvidence extracts a single AnswerSymbol from one
// EvidenceItem via pickHop(role). Returns an empty AnswerSymbol
// (Name=="") when no symbol can be determined so the caller can
// skip it.
//
// Phase 2: all per-shape routing moved into pickTerminalLegacy
// (which is what RoleTerminal falls back to). RoleOrigin and
// RoleAnchor override this with direction-aware hop selection; see
// pickHop's doc and memory/project_answer_symbol_extraction_audit.md.
func answerSymbolFromEvidence(ev types.EvidenceItem, questionKind string, role AnswerRole, graph *repomap.Graph) types.AnswerSymbol {
	sym := types.AnswerSymbol{
		File:  ev.Source,
		Line:  ev.LineStart,
		Kind:  questionKind,
		Chain: ev.Summary,
	}
	sym.Name = pickHop(ev, role, graph)

	// Graph-anchored fallback for source locator when EvidenceItem
	// didn't carry Source. Receiver-aware: when multiple
	// definitions share the same name (the drift corpus —
	// `Execute`, `Name`, `String`, …), pick the one whose Receiver
	// or Parent matches ev.Subject. Refuse to guess when the hint
	// cannot disambiguate, leaving File/Line empty rather than
	// drifting to an arbitrary defs[0].
	if sym.File == "" && graph != nil && sym.Name != "" {
		if def := resolveDefWithReceiver(graph, sym.Name, ev.Subject); def != nil {
			sym.File = def.File
			sym.Line = def.Line
		}
	}
	return sym
}

// resolveDefWithReceiver looks up a symbol by name in the graph's
// SymbolDefs index with receiver-aware disambiguation.
//
//   - 0 matches → nil
//   - 1 match   → return it (no ambiguity)
//   - >1 match  → if receiverHint is non-empty, filter by matching
//     Receiver or Parent and return the unique survivor; otherwise
//     return nil to avoid defs[0] drift.
//
// Receiver matching is a case-sensitive exact comparison against the
// hint. Tree-sitter extractors already strip leading pointers (`*T`
// → `T`) so direct equality is safe.
//
// Introduced to close the first of the two B-bucket drift sites
// documented in memory/project_repomap_refactor_plan.md. The
// contract is deliberately conservative: when in doubt, return nil
// so downstream consumers see the same empty result they would see
// if the evidence had no Source at all, rather than a plausible-
// looking but wrong file.
func resolveDefWithReceiver(graph *repomap.Graph, name, receiverHint string) *repomap.Symbol {
	defs, ok := graph.SymbolDefs[name]
	if !ok || len(defs) == 0 {
		return nil
	}
	if len(defs) == 1 {
		return defs[0]
	}
	if receiverHint == "" {
		return nil
	}
	var match *repomap.Symbol
	for _, d := range defs {
		if d.Receiver == receiverHint || d.Parent == receiverHint {
			if match != nil {
				// Two receivers collide on the same hint (rare: an
				// embedding chain or a duplicated type name across
				// packages). Refuse to guess.
				return nil
			}
			match = d
		}
	}
	return match
}

// stripNewPrefix removes a leading "New" from a constructor-style
// identifier. "NewSubExplorer" → "SubExplorer"; "Foo" → "Foo".
// Requires the character after "New" to be uppercase so we don't
// mangle legitimate names starting with "new" lowercase.
func stripNewPrefix(name string) string {
	if len(name) > 3 && strings.HasPrefix(name, "New") {
		if c := name[3]; c >= 'A' && c <= 'Z' {
			return name[3:]
		}
	}
	return name
}

// firstUppercaseIdent returns the first capitalised identifier token
// in the segment, where "identifier" is [A-Za-z_][A-Za-z0-9_]* and
// "capitalised" means first byte in [A-Z]. Tokens must be at least
// 3 characters long — shorter tokens are usually primitives or
// single-letter receivers (e.g. `r.tools` where `r` is noise).
func firstUppercaseIdent(seg string) string {
	n := len(seg)
	for i := 0; i < n; i++ {
		c := seg[i]
		// Token boundary: start of string or previous char was not an
		// identifier character.
		if i > 0 && isIdentChar(seg[i-1]) {
			continue
		}
		if c < 'A' || c > 'Z' {
			continue
		}
		// Scan forward while we have identifier chars.
		j := i
		for j < n && isIdentChar(seg[j]) {
			j++
		}
		if j-i >= 3 {
			return seg[i:j]
		}
		// Advance past this short token so we don't retry its chars.
		i = j - 1
	}
	return ""
}

// terminalPredicate reports whether a candidate answer chain's terminal
// segment (the right-hand side of its last hop) is structurally
// compatible with the question kind's answer shape. Used by
// identifyAnswerChains as a post-rank discriminative filter to demote
// chains whose terminal cannot possibly be a concrete answer (e.g. a
// Go `range` loop header when the question asks for a registered
// symbol). See project_L0_1_terminal_verification_design.md.
type terminalPredicate func(chainText string, graph *repomap.Graph) bool

// terminalPredicateByKind maps ERM Kind to the predicate its candidate
// chains must satisfy. Kinds without an entry (mechanism, enumeration,
// conditional, config_mapping) have no terminal requirement — they are
// verified by other means. Keeping this map small is deliberate:
// predicates are only for kinds whose answer is a SINGLE concrete
// symbol or literal.
var terminalPredicateByKind = map[string]terminalPredicate{
	"registration": terminalIsConcreteSymbolRef,
	"call_chain":   terminalIsConcreteSymbolRef,
	"return_value": terminalIsConcreteLiteral,
}

// originPredicateByKind maps ERM Kind to an ORIGIN predicate on the
// chain's leftmost segment. This is the complement of the terminal
// predicate — together they bracket a chain at both ends to verify
// it structurally represents the kind of resolution the question
// asks about. Only registration currently has one: a registration
// chain must start at a binding verb (`binds`) or a function whose
// name contains `Register`. Constructor-originated chains like
// `NewFoo() returns &Foo{} → Foo.Name()` are NOT registration chains
// even though their terminal looks valid.
//
// This closes the df1 post-L0-1 regression where chains like
// `NewProposeSubAgents() → ProposeSubAgents.Name() returns "..."`
// and `NewBaseAgent() → BaseAgent.buildToolSchemas()` passed the
// terminal predicate (valid method call shape), outscored the correct
// chain on question-entity overlap (because `propose_sub_agents`
// contains both "subagent" and "agent" as substrings, giving 2/2
// overlap while the correct `RegisterDefaultSubAgents → SubExplorer`
// chain only matches once), and fed BaseAgent / ProposeSubAgents
// into L0-2's AnswerSymbols list.
var originPredicateByKind = map[string]terminalPredicate{
	"registration": chainOriginIsRegistrationLinkage,
}

// chainOriginIsRegistrationLinkage reports whether a chain's leftmost
// segment represents a registration point. Two acceptance paths,
// designed to cover the Go ecosystem broadly without picking up the
// concrete-values extractor's generic `binds ONLY <signature>` output
// for every function in the codebase:
//
//  1. Function name contains `Register` (Go naming convention).
//     Matches `RegisterDefaultSubAgents()`, `RegisterHandlers()`,
//     etc. directly by substring.
//
//  2. First segment contains `binds ONLY` FOLLOWED BY a call
//     expression `<CapitalizedIdent>(` — a CONSTRUCTOR/CALL, not a
//     parameter list. This structurally distinguishes
//     `RegisterX() binds ONLY NewFoo(deps)` (registration linkage
//     via a call) from `NewBaseAgent() binds ONLY name types.Agent-
//     Name, deps *Dependencies` (concrete-values signature format,
//     no call after "binds ONLY"). The call-after-binds check means
//     codebases using non-Register naming (e.g. `BindHandlers()`,
//     `InstallRoutes()`, `ProvideDefaults()`) still pass as long as
//     the chain body shows a constructed instance.
//
// Earlier versions of this predicate accepted a bare " binds "
// substring — that matched both registration linkage and every
// constructor's parameter list, which turned out to be the dominant
// false positive on df1 run 3 (see eval/results/df1-20260412-093913).
// The compound check eliminates that false positive while preserving
// non-Go-convention registration coverage.
//
// Over-fit audit: the `Register` path is structurally named (Go
// naming rule, not tied to a specific symbol), and the `binds ONLY
// <call>` path is structurally defined (call expression vs parameter
// list) rather than verb-list-enumerated. Neither path was chosen by
// looking at df1's ground truth.
//
// Graph argument is unused but kept for signature uniformity with
// terminalPredicate.
func chainOriginIsRegistrationLinkage(chainText string, _ *repomap.Graph) bool {
	const arrow = "→"
	first := chainText
	if idx := strings.Index(chainText, arrow); idx >= 0 {
		first = chainText[:idx]
	}
	// Path 1: function name contains Register.
	if strings.Contains(first, "Register") {
		return true
	}
	// Path 2: `binds ONLY` followed by a call expression.
	bindsIdx := strings.Index(first, "binds ONLY ")
	if bindsIdx < 0 {
		return false
	}
	rest := first[bindsIdx+len("binds ONLY "):]
	return firstTokenIsCallExpression(rest)
}

// firstTokenIsCallExpression reports whether the first non-whitespace
// token of seg is an uppercase identifier followed by a `(` (a
// CONSTRUCTOR/CALL like `NewFoo(` or `CreateHandler(`). This is how
// we distinguish registration-linkage `binds ONLY NewFoo(deps)` from
// signature `binds ONLY name types.Type`: the former starts with a
// call, the latter with a parameter identifier + type.
func firstTokenIsCallExpression(seg string) bool {
	seg = strings.TrimLeft(seg, " \t")
	if seg == "" {
		return false
	}
	// Must start with an uppercase letter (Go exported identifier /
	// constructor convention). Lowercase starts are parameter names
	// ("name types.AgentName"), not exported calls.
	if seg[0] < 'A' || seg[0] > 'Z' {
		return false
	}
	// Walk to the first non-ident char; if it's `(`, it is a call.
	i := 0
	for i < len(seg) && isIdentChar(seg[i]) {
		i++
	}
	return i < len(seg) && seg[i] == '('
}

// terminalPredicatesFor returns the set of predicates applicable to the
// active ERM requirements, deduped so a single Kind's predicate is only
// evaluated once even when the requirement set contains multiple
// entries of that Kind. Returns nil when no active kind has a
// predicate, which is the signal for identifyAnswerChains to skip
// terminal verification entirely.
func terminalPredicatesFor(reqs []EvidenceRequirement) []terminalPredicate {
	return predicatesFor(reqs, terminalPredicateByKind)
}

// originPredicatesFor returns the origin predicates applicable to the
// active ERM requirements. Same dedup semantics as terminalPredicatesFor.
func originPredicatesFor(reqs []EvidenceRequirement) []terminalPredicate {
	return predicatesFor(reqs, originPredicateByKind)
}

// predicatesFor is the shared lookup helper for any Kind → predicate
// table.
func predicatesFor(reqs []EvidenceRequirement, table map[string]terminalPredicate) []terminalPredicate {
	if len(reqs) == 0 || len(table) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(table))
	var out []terminalPredicate
	for _, r := range reqs {
		if seen[r.Kind] {
			continue
		}
		if p, ok := table[r.Kind]; ok {
			out = append(out, p)
			seen[r.Kind] = true
		}
	}
	return out
}

// extractTerminalSegment returns the substring after the last U+2192
// ("→") arrow in a resolution chain text. This is the chain's
// rightmost hop — the terminal symbol that ends up being the answer.
// Chains with no arrow return the entire string (defensive; chains
// should always contain at least one hop).
func extractTerminalSegment(chainText string) string {
	const arrow = "→"
	if idx := strings.LastIndex(chainText, arrow); idx >= 0 {
		return strings.TrimSpace(chainText[idx+len(arrow):])
	}
	return strings.TrimSpace(chainText)
}

// terminalIsConcreteSymbolRef reports whether a chain's terminal
// segment names a concrete symbol (function call, method receiver,
// type reference) rather than a Go-language control-flow construct.
// Used by registration and call_chain kinds.
//
// The rejection list is structural: Go keywords and builtins that
// cannot be an "answer" under any registration or call-chain question,
// regardless of the specific entities involved. The list is derived
// from Go semantics, not from any eval case's ground truth — reversing
// it would break an entire class of questions, not just df1.
func terminalIsConcreteSymbolRef(chainText string, graph *repomap.Graph) bool {
	terminal := extractTerminalSegment(chainText)
	// Strip a trailing source locator like ` (file:line)` so the
	// literal shape matchers see the raw expression.
	if p := strings.LastIndex(terminal, " ("); p >= 0 && strings.HasSuffix(terminal, ")") {
		terminal = strings.TrimSpace(terminal[:p])
	}
	if terminal == "" {
		return false
	}
	badPatterns := []string{
		"range ",       // loop header: `range r.tools`, `range m`
		"for _, ",      // generic iteration
		"for k, v :=",  // generic iteration
		"make(",        // builtin constructor for generic containers
		"append(",      // builtin slice op
		"len(", "cap(", // builtin size queries
		"assigns name :=", // internal marker from concrete-values loop scan
	}
	for _, bad := range badPatterns {
		if strings.Contains(terminal, bad) {
			return false
		}
	}
	if hasMethodCallShape(terminal) {
		return true
	}
	if hasReturnsLiteralShape(terminal) {
		return true
	}
	if graph != nil && containsGraphSymbol(terminal, graph) {
		return true
	}
	return false
}

// terminalIsConcreteLiteral reports whether a chain's terminal segment
// ends at a concrete literal value (string, number, bool, nil). Used
// by the return_value kind, whose answer is a single literal rather
// than a symbol reference.
func terminalIsConcreteLiteral(chainText string, graph *repomap.Graph) bool {
	return hasReturnsLiteralShape(extractTerminalSegment(chainText))
}

// hasMethodCallShape reports whether the segment contains a method-call
// pattern like `X.Y(` — a capitalised (or at least identifier-like)
// receiver followed by a dotted call. This is the canonical shape of a
// concrete symbol reference in a chain terminal.
func hasMethodCallShape(seg string) bool {
	// Find the first '.' that is followed by an identifier and '(',
	// with an identifier character just before the dot. This is a
	// cheap structural check, not a full Go parser.
	for i := 1; i < len(seg)-2; i++ {
		if seg[i] != '.' {
			continue
		}
		prev := seg[i-1]
		next := seg[i+1]
		if !isIdentChar(prev) || !isIdentStart(next) {
			continue
		}
		// Scan forward for an opening paren within ~40 chars.
		end := i + 40
		if end > len(seg) {
			end = len(seg)
		}
		for j := i + 1; j < end; j++ {
			if seg[j] == '(' {
				return true
			}
			if !isIdentChar(seg[j]) {
				break
			}
		}
	}
	return false
}

// hasReturnsLiteralShape reports whether the segment contains a
// `returns "x"` / `returns 'x'` pattern, the canonical concrete-return
// shape produced by the concrete-values extractor. This is a subset of
// endsWithShortLiteralReturn (which additionally enforces a length
// cap); here we accept any length because the predicate's role is
// "is there a literal at all", not "is it a short identity return".
func hasReturnsLiteralShape(seg string) bool {
	idx := strings.Index(seg, "returns ")
	if idx < 0 {
		return false
	}
	after := strings.TrimSpace(seg[idx+len("returns "):])
	if after == "" {
		return false
	}
	q := after[0]
	if q == '"' || q == '\'' {
		// Quoted: require a closing quote. Len >= 2 enforced implicitly
		// by IndexByte over the suffix — a missing close yields -1.
		return len(after) >= 2 && strings.IndexByte(after[1:], q) >= 0
	}
	// Non-quoted literals: true/false/nil and numeric prefixes. A
	// bare digit is already a valid literal ("returns 0").
	for _, lit := range []string{"true", "false", "nil"} {
		if strings.HasPrefix(after, lit) {
			return true
		}
	}
	if after[0] >= '0' && after[0] <= '9' {
		return true
	}
	return false
}

// containsGraphSymbol reports whether the segment mentions the name of
// any symbol defined in the repo graph. This is the fallback path for
// terminalIsConcreteSymbolRef when the method-call and literal shape
// checks both miss — the terminal might be a bare type reference like
// `SubExplorer` with no dotted access.
func containsGraphSymbol(seg string, graph *repomap.Graph) bool {
	if graph == nil || len(graph.SymbolDefs) == 0 {
		return false
	}
	// Only check symbols at least 4 chars to avoid trivial matches.
	// Uppercase-first symbols are the overwhelming majority of Go
	// exported identifiers; skipping lowercase ones keeps this cheap.
	for name := range graph.SymbolDefs {
		if len(name) < 4 || !isIdentStart(name[0]) {
			continue
		}
		if name[0] < 'A' || name[0] > 'Z' {
			continue
		}
		if strings.Contains(seg, name) {
			return true
		}
	}
	return false
}

// isIdentStart is a local helper for the terminal-predicate shape
// checks. isIdentChar already exists in explorer.go and is reused as
// the "ident continuation" predicate.
func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

// firstSegmentIsBinds reports whether a resolution-chain text's
// leftmost segment uses a `binds` verb, i.e. the chain starts with a
// registration linkage rather than a constructor or generic return.
// This is a canonical shape for registration-driven answers.
func firstSegmentIsBinds(chain string) bool {
	const arrow = "→"
	seg := chain
	if idx := strings.Index(chain, arrow); idx >= 0 {
		seg = chain[:idx]
	}
	return strings.Contains(seg, " binds ")
}

// endsWithShortLiteralReturn reports whether a resolution-chain text's
// rightmost segment ends with `returns "x"` or `returns 'x'` where x is
// a short literal (≤ 20 chars). This is the canonical shape of a
// resolved identity answer (Name/Type/Kind methods), as opposed to
// descriptions (long strings), constructors (`returns &Foo{`), or
// assignments. The caller uses this as a deterministic tie-breaker
// bonus when ranking answer chains.
func endsWithShortLiteralReturn(chain string) bool {
	// Take the last segment after the last arrow (U+2192). If there is
	// no arrow (single-item chain, shouldn't happen for isChain but be
	// defensive), consider the whole text.
	const arrow = "→"
	idx := strings.LastIndex(chain, arrow)
	seg := chain
	if idx >= 0 {
		seg = chain[idx+len(arrow):]
	}
	seg = strings.TrimSpace(seg)
	// Drop a trailing source locator like ` (file:line)` so it doesn't
	// push the literal away from the end of the string.
	if p := strings.LastIndex(seg, " ("); p >= 0 && strings.HasSuffix(seg, ")") {
		seg = strings.TrimSpace(seg[:p])
	}
	// Require `returns ` somewhere in the segment.
	rIdx := strings.Index(seg, "returns ")
	if rIdx < 0 {
		return false
	}
	after := strings.TrimSpace(seg[rIdx+len("returns "):])
	if len(after) < 2 {
		return false
	}
	q := after[0]
	if q != '"' && q != '\'' {
		return false
	}
	// Find the matching closing quote.
	end := strings.IndexByte(after[1:], q)
	if end < 0 {
		return false
	}
	// Short literal: 0..20 chars between quotes. Also require the
	// literal to be the TAIL of the segment — nothing meaningful after
	// it — otherwise we may have matched a `returns "x" + something`.
	closeIdx := 1 + end
	tail := strings.TrimSpace(after[closeIdx+1:])
	if tail != "" {
		return false
	}
	return end <= 20
}

// logERM logs the current ERM state at debug level.
func logERM(reqs []EvidenceRequirement) {
	if len(reqs) == 0 {
		return
	}
	for _, req := range reqs {
		logging.Debug("[erm] %s(%s) = %s — %s",
			req.Kind, strings.Join(req.Entities, ","), req.Status, req.Reason)
	}
}
