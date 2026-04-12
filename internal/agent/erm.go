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
func extractEvidenceRequirements(question string) []EvidenceRequirement {
	entities := extractRankingEntities(question)
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
	for _, kw := range []string{"how many", "list all", "list each", "what are the"} {
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
	isRegistration := false
	for _, kw := range []string{"register", "registered", "registry"} {
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
	// Triggered when question asks about matching, identity, or naming
	for _, kw := range []string{"name", "type", "which", "what", "名称", "类型", "哪个", "什么"} {
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
	for _, kw := range []string{"when", "condition", "under what", "条件", "什么时候", "何时"} {
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

// ermAutoSatisfyUnresolvable marks requirements as "satisfied" when none
// of their entities match any symbol in the codebase. This prevents
// generic English words (from analyzer-rewritten tasks) from creating
// unsatisfiable requirements that block the pipeline indefinitely.
// This is a data-driven filter (checked against the repo's symbol table),
// not a hardcoded stopword list.
func ermAutoSatisfyUnresolvable(reqs []EvidenceRequirement, graph *repomap.Graph) []EvidenceRequirement {
	if graph == nil || len(reqs) == 0 {
		return reqs
	}
	for i := range reqs {
		req := &reqs[i]
		if req.Status == "satisfied" {
			continue
		}
		// Check if ANY entity in this requirement matches ANY symbol in the repo.
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
func identifyAnswerChains(question string, evidence []types.EvidenceItem, maxChains int, whitelist answerPredicateWhitelist) []string {
	entities := extractRankingEntities(question)
	if len(entities) == 0 || len(evidence) == 0 {
		return nil
	}

	type scored struct {
		text  string
		score float64
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

		text := normalizeForMatch(ev.Summary + " " + ev.Subject + " " + ev.Object)
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

		candidates = append(candidates, scored{
			text:  display,
			score: float64(overlap) / float64(len(entities)) * bonus,
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Dedup by text content
	seen := make(map[string]bool)
	var result []string
	for _, c := range candidates {
		if seen[c.text] {
			continue
		}
		seen[c.text] = true
		result = append(result, c.text)
		if len(result) >= maxChains {
			break
		}
	}
	return result
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
