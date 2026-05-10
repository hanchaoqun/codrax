package agent

// explorer_erm.go — Evidence Requirement Model (ERM).
//
// All types and functions in this file serve the explorer agent's
// Turn A investigation loop (internal/agent/explorer.go). ERM is the
// bookkeeping layer that turns the analyzer's AnalysisIR entities +
// question_kind into concrete "have I collected enough evidence to
// stop reading files" predicates. Every symbol here has exactly one
// caller — the main explorer evaluator — and no other agent imports
// this file's surface. The file is named with the explorer_ prefix
// so `ls internal/agent/explorer_*` surfaces the full explorer
// codebase in one shot.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
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
	// Kind is the typed question-side classification. See
	// internal/types/requirement_kind.go for the canonical enum and
	// the RequirementToEvidenceKinds map that binds each Kind to the
	// EvidenceKinds the ERM layer accepts as contributing evidence.
	Kind     types.RequirementKind
	Entities []string // key entities from the question this requirement relates to
	Reason   string   // human-readable reason for this requirement
	Status   string   // "unsatisfied", "partial", "satisfied"

	// DependsOn lists the IDs (Kind + entity key) of requirements
	// that must be satisfied before this requirement can be
	// evaluated. Used by the sequential step ordering: the explorer
	// only pushes the LLM to work on a requirement whose
	// dependencies are all satisfied. Empty = ready immediately.
	//
	// This is the first of two "real increment" additions from the
	// investigation planner audit (issues/investigation-planner-
	// duplicates-erm.md). The second is RetryCount below.
	DependsOn []string

	// RetryCount tracks how many times the explorer's soft-stop
	// loop pushed back on this requirement without it advancing
	// from its current status. When RetryCount exceeds a threshold
	// (replanRetryThreshold), the explorer splits this requirement
	// into per-entity sub-requirements so each entity gets its own
	// satisfaction check — the "retry-triggered refinement" from
	// the planner audit.
	RetryCount int
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
	return extractEvidenceRequirementsWithEntities(question, extractRankingEntitiesWithGraph(question, nil))
}

// extractEvidenceRequirementsWithModel is the analyzer-driven primary
// path. Semantic requirement selection should come from the
// analyzer's structured classification; the legacy keyword tables are
// a fallback only when no usable structured signal exists.
func extractEvidenceRequirementsWithModel(question string, entities []string, declaredKindRaw string, preds types.SemanticPredicates, rm *types.RequestModel) []EvidenceRequirement {
	if rm != nil {
		if reqs := extractEvidenceRequirementsFromStructuredSignals(entities, *rm); len(reqs) > 0 {
			return reqs
		}
	}
	return extractEvidenceRequirementsWithHint(question, entities, declaredKindRaw, preds)
}

// extractEvidenceRequirementsWithHint is the analyzer-aware entry
// point used by the explorer. Phase 6 stage 16 (2026-05-03):
// keyword inference is REMOVED. RequirementKind comes ONLY from the
// analyzer's typed emit (`question_kind` declared by emit_analysis,
// or inferred deterministically from the typed RequestModel fields
// via inferPrimaryRequirementKindFromSignals). Empty / unknown
// declared kind no longer falls through to a keyword path; it
// returns just the typed-secondary set from `preds`.
//
// Per the user's directive "意图分类是分析器的工作", explore must
// not re-tokenize the question to infer intent — the analyzer
// already classified it; ERM consumes the typed result.
func extractEvidenceRequirementsWithHint(question string, entities []string, declaredKindRaw string, preds types.SemanticPredicates) []EvidenceRequirement {
	_ = question
	declaredKind := types.NormalizeRequirementKind(declaredKindRaw)
	if declaredKind == types.ReqUnknown {
		// No declared kind → no keyword fallback. Secondary kinds
		// from typed SemanticPredicates only.
		return appendSecondaryKinds(nil, entities, preds)
	}

	var reqs []EvidenceRequirement

	// Track whether the declared kind already appeared in the keyword
	// path — if so, the keyword path has already covered it.
	declaredPresent := false
	for _, r := range reqs {
		if r.Kind == declaredKind {
			declaredPresent = true
			break
		}
	}

	if !declaredPresent {
		// The declared kind was missed by keyword inference — add it
		// explicitly. This is the "analyzer saves us" path: e.g. a
		// Chinese mechanism question whose English rewrite used idioms
		// the keyword tables don't cover.
		reason := fmt.Sprintf("analyzer declared question_kind=%s", declaredKind)
		switch declaredKind {
		case types.ReqRegistration, types.ReqReturnValue, types.ReqHistory:
			// Registration and return_value requirements are per-entity
			// in the keyword path; match that convention so downstream
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
		default:
			// mechanism / conditional / config_mapping / enumeration /
			// call_chain all take the entity set as a single group.
			reqs = append(reqs, EvidenceRequirement{
				Kind: declaredKind, Entities: append([]string(nil), entities...),
				Reason: reason, Status: "unsatisfied",
			})
		}
	}

	return appendSecondaryKinds(reqs, entities, preds)
}

func extractEvidenceRequirementsFromStructuredSignals(entities []string, rm types.RequestModel) []EvidenceRequirement {
	declaredKind := types.NormalizeRequirementKind(rm.AnalyzerHints.Kind)
	if declaredKind == types.ReqUnknown {
		declaredKind = inferPrimaryRequirementKindFromSignals(rm)
	}
	if declaredKind == types.ReqUnknown {
		return nil
	}
	reqs := requirementsForKind(declaredKind, entities, structuredRequirementReason(declaredKind, rm))
	return appendSecondaryKinds(reqs, entities, rm.Predicates)
}

func inferPrimaryRequirementKindFromSignals(rm types.RequestModel) types.RequirementKind {
	if rm.Predicates.IsHistoryLookup {
		return types.ReqHistory
	}
	if rm.Scenario == types.ScenarioConfigTrace ||
		rm.AnswerSubject.Kind == types.SubjectConfigKey ||
		rm.PredicateAxis == types.AxisConfigure {
		return types.ReqConfigMapping
	}
	switch rm.PredicateAxis {
	case types.AxisRegister:
		return types.ReqRegistration
	case types.AxisCondition:
		return types.ReqConditional
	case types.AxisCall:
		return types.ReqCallChain
	case types.AxisReturn:
		if !rm.Predicates.IsCountQuestion {
			return types.ReqReturnValue
		}
	}
	if isEnumerationRequestModel(rm) {
		return types.ReqEnumeration
	}
	if rm.Intent == types.IntentReturnValue && !rm.Predicates.IsCountQuestion {
		return types.ReqReturnValue
	}
	switch rm.Intent {
	case types.IntentTrace:
		return types.ReqCallChain
	case types.IntentExplain, types.IntentRootCause:
		if !rm.Predicates.IsScalarAnswer {
			return types.ReqMechanism
		}
	}
	return types.ReqUnknown
}

func structuredRequirementReason(kind types.RequirementKind, rm types.RequestModel) string {
	parts := []string{}
	if declared := types.NormalizeRequirementKind(rm.AnalyzerHints.Kind); declared == kind {
		parts = append(parts, "analyzer declared question_kind="+string(kind))
	}
	if rm.PredicateAxis != types.AxisUnknown {
		switch kind {
		case types.ReqRegistration:
			if rm.PredicateAxis == types.AxisRegister {
				parts = append(parts, "predicate_axis=register")
			}
		case types.ReqConditional:
			if rm.PredicateAxis == types.AxisCondition {
				parts = append(parts, "predicate_axis=condition")
			}
		case types.ReqCallChain:
			if rm.PredicateAxis == types.AxisCall {
				parts = append(parts, "predicate_axis=call")
			}
		case types.ReqConfigMapping:
			if rm.PredicateAxis == types.AxisConfigure {
				parts = append(parts, "predicate_axis=configure")
			}
		case types.ReqReturnValue:
			if rm.PredicateAxis == types.AxisReturn {
				parts = append(parts, "predicate_axis=return")
			}
		}
	}
	switch kind {
	case types.ReqHistory:
		if rm.Predicates.IsHistoryLookup {
			parts = append(parts, "predicates.is_history_lookup=true")
		}
	case types.ReqEnumeration:
		if isEnumerationRequestModel(rm) {
			parts = append(parts, "structured enumeration intent")
		}
	case types.ReqConfigMapping:
		if rm.Scenario == types.ScenarioConfigTrace {
			parts = append(parts, "scenario=config_trace")
		}
		if rm.AnswerSubject.Kind == types.SubjectConfigKey {
			parts = append(parts, "answer_subject=config_key")
		}
	case types.ReqReturnValue:
		if rm.Intent == types.IntentReturnValue {
			parts = append(parts, "intent=return_value")
		}
	case types.ReqCallChain:
		if rm.Intent == types.IntentTrace {
			parts = append(parts, "intent=trace")
		}
	case types.ReqMechanism:
		if rm.Intent == types.IntentExplain || rm.Intent == types.IntentRootCause {
			parts = append(parts, "intent="+string(rm.Intent))
		}
	}
	if len(parts) == 0 {
		return "structured analyzer signals"
	}
	return strings.Join(parts, ", ")
}

func requirementsForKind(kind types.RequirementKind, entities []string, reason string) []EvidenceRequirement {
	var reqs []EvidenceRequirement
	add := func(kind types.RequirementKind, reason string, ents ...string) {
		reqs = append(reqs, EvidenceRequirement{
			Kind:     kind,
			Entities: append([]string(nil), ents...),
			Reason:   reason,
			Status:   "unsatisfied",
		})
	}
	switch kind {
	case types.ReqRegistration, types.ReqReturnValue, types.ReqHistory:
		if len(entities) == 0 {
			add(kind, reason)
			return reqs
		}
		for _, ent := range entities {
			add(kind, reason+" (per-entity)", ent)
		}
	default:
		add(kind, reason, entities...)
	}
	return reqs
}

func isEnumerationRequestModel(rm types.RequestModel) bool {
	if types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) == types.ReqEnumeration {
		return true
	}
	if rm.Intent == types.IntentEnumerate || rm.Predicates.IsCategoryEnumeration {
		return true
	}
	return types.ResolveQuestionFamily(rm) == types.QFEnumeration
}

// appendSecondaryKinds consults inferSecondaryKinds for structural
// hybrid-kind cues (e.g. category-enumeration questions that the
// analyzer tagged as scalar return_value but which also need
// enumeration thresholds) and appends missing kinds. De-duped
// against whatever primary / keyword-inferred kinds already sit in
// reqs — a question that already has ReqEnumeration from the
// analyzer or keyword path gets no duplicate.
func appendSecondaryKinds(reqs []EvidenceRequirement, entities []string, preds types.SemanticPredicates) []EvidenceRequirement {
	secondary := inferSecondaryKinds(preds)
	if len(secondary) == 0 {
		return reqs
	}
	present := make(map[types.RequirementKind]bool, len(reqs))
	for _, r := range reqs {
		present[r.Kind] = true
	}
	for _, k := range secondary {
		if present[k] {
			continue
		}
		reqs = append(reqs, EvidenceRequirement{
			Kind:     k,
			Entities: append([]string(nil), entities...),
			Reason:   fmt.Sprintf("secondary kind inferred from structural cues (hybrid question)"),
			Status:   "unsatisfied",
		})
		present[k] = true
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
// extractEvidenceRequirementsWithEntities was retired 2026-05-03
// (Phase 6 stage 16). The function classified the user's question
// via 15 hardcoded EN+ZH keyword tables (enumeration / call_chain
// / registration / return_value / history_noun / history_intent /
// config_mapping / conditional / mechanism). Per the user's
// architectural directive — "intent classification is the
// analyzer's job, NOT explore's; keyword tables driving intent
// classification are forbidden" — the entire keyword path is
// removed. RequirementKind now derives ONLY from typed analyzer
// signals (RequestModel.AnalyzerHints.Kind / Intent / Predicates /
// PredicateAxis / AnswerSubject) via
// extractEvidenceRequirementsFromStructuredSignals. When the
// analyzer's emit_analysis tool call returns ReqUnknown for a
// question, ERM stays empty rather than fabricating a kind from
// re-tokenizing the question text.
//
// The function is preserved as a no-op for callers that haven't
// migrated yet, returning an empty slice. Callers that produced
// useful output through the keyword path now produce nothing —
// the analyzer's structured emit is the single source of truth.
func extractEvidenceRequirementsWithEntities(question string, entities []string) []EvidenceRequirement {
	// Phase 6 stage 16 (2026-05-03) — keyword-based intent
	// classification REMOVED. Intent classification is the
	// analyzer's job (emit_analysis tool call); ERM consumes the
	// result via extractEvidenceRequirementsFromStructuredSignals.
	// The retired path scanned 15 hardcoded EN+ZH keyword tables
	// against the user's question prose to fabricate a
	// RequirementKind — that violated the "no custom keyword
	// matching" red line by making explore the intent classifier.
	// Returning an empty slice routes callers through the typed
	// analyzer signals.
	_ = question
	_ = entities
	return nil
}

// ermThresholds carries the "satisfied" and "partial" floors for a
// single requirement kind at a given analyzer-classified complexity.
// Returned by thresholdForKind so every branch in
// checkRequirementSatisfaction reads the same SSOT.
type ermThresholds struct {
	Satisfied int
	Partial   int
}

// thresholdForKind returns the count floors for a requirement kind
// at the given complexity. Complex questions (cross-component, 6+
// files) raise the "satisfied" floor so a handful of isolated
// mechanism/config/enumeration facts cannot declare the requirement
// resolved while the dispatch-path files are still unread.
//
// Mapping (satisfied / partial):
//
//	Kind           | simple  | moderate | complex |
//	---------------|---------|----------|---------|
//	mechanism      | 2 / 1   | 2 / 1    | 4 / 2   |
//	enumeration    | 3 / 1   | 3 / 1    | 5 / 2   |
//	config_mapping | 2 / 1   | 2 / 1    | 3 / 2   |
//	conditional    | 1 / 0   | 1 / 0    | 2 / 1   |
//	call_chain     | 2 / 1   | 2 / 1    | 3 / 2   |
//	return_value   | 1 / 0   | 1 / 0    | 1 / 0   |
//	registration   | 1 / 0   | 1 / 0    | 1 / 0   |  (not scalar — per-entity match)
//
// Unknown complexity (zero value or any future enum variant) falls
// through to the moderate row so the pre-T1c behavior is preserved
// byte-for-byte.
//
// Registration and return_value are per-entity rather than scalar
// count based, so their thresholds are placeholders that the
// respective cases don't actually read — kept in the table for
// completeness and future-proofing.
func thresholdForKind(kind types.RequirementKind, complexity types.Complexity) ermThresholds {
	complex := complexity == types.ComplexityComplex
	switch kind {
	case types.ReqMechanism:
		if complex {
			return ermThresholds{Satisfied: 4, Partial: 2}
		}
		return ermThresholds{Satisfied: 2, Partial: 1}
	case types.ReqEnumeration:
		if complex {
			return ermThresholds{Satisfied: 5, Partial: 2}
		}
		return ermThresholds{Satisfied: 3, Partial: 1}
	case types.ReqConfigMapping:
		if complex {
			return ermThresholds{Satisfied: 3, Partial: 2}
		}
		return ermThresholds{Satisfied: 2, Partial: 1}
	case types.ReqConditional:
		if complex {
			return ermThresholds{Satisfied: 2, Partial: 1}
		}
		return ermThresholds{Satisfied: 1, Partial: 0}
	case types.ReqCallChain:
		if complex {
			return ermThresholds{Satisfied: 3, Partial: 2}
		}
		return ermThresholds{Satisfied: 2, Partial: 1}
	case types.ReqHistory:
		return ermThresholds{Satisfied: 1, Partial: 0}
	default:
		return ermThresholds{Satisfied: 1, Partial: 0}
	}
}

// checkRequirementSatisfaction scans investigation notes and structured
// evidence against ERM requirements. Returns the updated requirements
// with status set to "satisfied", "partial", or "unsatisfied".
//
// T1c: `complexity` scales the count thresholds via thresholdForKind
// so complex cross-component questions (6+ files) need more evidence
// to satisfy a requirement than simple lookups. Callers that do not
// have a classified complexity (e.g. unit tests, analyze-phase
// failures) pass the zero value, which maps to the historical
// moderate-complexity thresholds.
// checkRequirementSatisfaction (2026-05-03, Phase 6 stage 14) —
// the `notes` parameter is now unused. Pre-stage-14 the function
// scanned the joined ReAct-loop notes for bracket tags
// ([direct]/[registration]/[relationship]/[mechanism]/[conditional])
// and hardcoded keyword tables ({"new", "only", "default", "\""},
// {"returns", "return"}, {"commit", "history", "提交", "历史",
// "blame"}). Both signals were token-overlap heuristics: bracket
// tags depended on the LLM voluntarily wrapping its prose in
// brackets, and keyword tables didn't generalise to other
// languages or naming conventions. The new path counts typed
// EvidenceKind enums on the evidence pool — the structural source
// of truth that the LLM emits when it calls emit_evidence.
//
// Signature is preserved (notes still accepted for back-compat
// callers); the parameter is intentionally ignored.
func checkRequirementSatisfaction(reqs []EvidenceRequirement, notes []string, evidence []types.EvidenceItem, complexity types.Complexity) []EvidenceRequirement {
	if len(reqs) == 0 {
		return reqs
	}
	_ = notes

	for i := range reqs {
		req := &reqs[i]
		if req.Status == "satisfied" {
			continue
		}
		th := thresholdForKind(req.Kind, complexity)
		switch req.Kind {
		case types.ReqEnumeration:
			// Satisfied if the evidence pool carries enough
			// enumeration-bearing items naming the requirement
			// entities. Accept LLM-tagged kinds (Direct,
			// Registration) AND deterministic kinds (Concrete,
			// DataflowPath); both are first-class.
			count := countEvidenceByKinds(evidence, req.Entities,
				types.EvidenceDirect, types.EvidenceRegistration,
				types.EvidenceConcrete, types.EvidenceDataflowPath)
			if count >= th.Satisfied {
				req.Status = "satisfied"
			} else if count >= th.Partial && th.Partial > 0 {
				req.Status = "partial"
			} else if count >= 1 && th.Partial == 0 {
				req.Status = "partial"
			}

		case types.ReqCallChain:
			// Satisfied when typed Relationship / Mechanism /
			// Concrete kinds ALL describe the requirement
			// entities AND ≥2 entities each appear in some
			// EvidenceItem's typed slots.
			hasRelationship := countEvidenceByKinds(evidence, req.Entities,
				types.EvidenceRelationship, types.EvidenceMechanism, types.EvidenceConcrete) > 0
			hasRelationship = hasRelationship || countEvidenceForRequirement(evidence, req.Entities, types.ReqCallChain) > 0
			entitiesFound := 0
			for _, ent := range req.Entities {
				for _, ev := range evidence {
					if matchEvidenceSlotsByEntity(ev, ent) {
						entitiesFound++
						break
					}
				}
			}
			if hasRelationship && entitiesFound >= 2 {
				req.Status = "satisfied"
			} else if hasRelationship || entitiesFound >= 1 {
				req.Status = "partial"
			}

		case types.ReqRegistration:
			// Satisfied when typed EvidenceRegistration items
			// (or binds-shape EvidenceConcrete via
			// isRegistrationShape) name the requirement entity
			// in their typed identifier slots. The retired path
			// scanned bracket-tagged note lines and used a
			// {"new", "\"", "only", "default"} keyword table on
			// raw line text to confirm "specific value" — those
			// markers don't generalise across languages. The
			// new path treats any EvidenceRegistration / binds-
			// shape EvidenceConcrete that names the entity as
			// satisfied; partial-vs-satisfied distinction is now
			// driven by EvidenceKind precedence (Registration
			// outranks bare Concrete).
			for _, ent := range req.Entities {
				for _, ev := range evidence {
					if ev.Kind == types.EvidenceRegistration && matchEvidenceSlotsByEntity(ev, ent) {
						req.Status = "satisfied"
						break
					}
				}
				if req.Status == "satisfied" {
					break
				}
				for _, ev := range evidence {
					if !isRegistrationShape(ev) {
						continue
					}
					if matchEvidenceSlotsByEntity(ev, ent) {
						req.Status = "satisfied"
						break
					}
				}
				if req.Status == "satisfied" {
					break
				}
			}

		case types.ReqReturnValue:
			// Satisfied when EvidenceConcrete with non-empty
			// Object names the entity in its Subject slot AND
			// the item is NOT a binds-shape (registration)
			// concrete value. Subject-only match preserves the
			// pre-stage-14 contract that "return value belongs
			// to its method/function via Subject"; the
			// !isRegistrationShape filter keeps binds-shape
			// concrete items (Predicate="binds ONLY", etc.) out
			// of return-value satisfaction so cross-Kind leak
			// from registration evidence is prevented (per the
			// TestCheckRequirementSatisfaction_ReturnValueUnaffectedByBinds
			// contract). The retired notes-prose
			// `(returns|return)` keyword fallback is dropped —
			// typed slot equality + isRegistrationShape filter
			// is the precise signal.
			for _, ent := range req.Entities {
				for _, ev := range evidence {
					if ev.Kind != types.EvidenceConcrete {
						continue
					}
					if ev.Object == "" {
						continue
					}
					if isRegistrationShape(ev) {
						continue
					}
					subjectLow := normalizeForMatch(ev.Subject)
					needle := normalizeForMatch(ent)
					if subjectLow == "" || needle == "" {
						continue
					}
					if strings.Contains(subjectLow, needle) {
						req.Status = "satisfied"
						break
					}
				}
				if req.Status == "satisfied" {
					break
				}
			}

		case types.ReqHistory:
			// Satisfied when EvidenceConcrete with non-empty
			// Object names the entity in typed slots. The
			// retired notes-prose {"commit", "history", "blame",
			// "提交", "历史"} keyword scan is dropped: it was a
			// dual-language hardcoded table that didn't
			// generalise to other locales, and concrete-value
			// evidence (the typed signal that the explorer
			// captured git-log / git-blame output) IS the
			// precise marker. Empty-entities branch is dropped
			// too — without entities to bind the proof to, "any
			// concrete value" is too coarse.
			if len(req.Entities) == 0 {
				break
			}
			for _, ent := range req.Entities {
				for _, ev := range evidence {
					if ev.Kind == types.EvidenceConcrete &&
						ev.Object != "" &&
						matchEvidenceSlotsByEntity(ev, ent) {
						req.Status = "satisfied"
						break
					}
				}
				if req.Status == "satisfied" {
					break
				}
			}

		case types.ReqConfigMapping:
			count := countEvidenceForRequirement(evidence, req.Entities, types.ReqConfigMapping)
			if count >= th.Satisfied {
				req.Status = "satisfied"
			} else if count >= 1 {
				req.Status = "partial"
			}

		case types.ReqConditional:
			// Same simplification as ReqMechanism — typed
			// EvidenceConditional Kind is already covered by
			// countEvidenceForRequirement via RequirementAcceptsEvidenceKind.
			count := countEvidenceForRequirement(evidence, req.Entities, types.ReqConditional)
			if count >= th.Satisfied {
				req.Status = "satisfied"
			} else if count >= 1 && th.Partial > 0 {
				req.Status = "partial"
			}

		case types.ReqMechanism:
			// Satisfied via the structural-carrier counter, which
			// already covers EvidenceMechanism + EvidenceRelationship
			// via RequirementAcceptsEvidenceKind plus the
			// AnchorCall+call-like-Predicate / AnchorCondition
			// fallbacks. The retired notes [mechanism] bracket-tag
			// count is dropped — that was a parallel signal on
			// LLM-prose; the typed equivalent is already counted
			// here.
			//
			// T1c (preserved): th.Satisfied scales with
			// complexity — complex cross-package mechanism
			// questions (6+ files) need ≥4 evidence items so the
			// explorer reads orchestrator / dispatch files in
			// addition to entry-point implementation.
			count := countEvidenceForRequirement(evidence, req.Entities, types.ReqMechanism)
			if count >= th.Satisfied {
				req.Status = "satisfied"
			} else if count >= 1 {
				req.Status = "partial"
			}
		}
	}
	return reqs
}

// replanRetryThreshold is the number of soft-stop retries on a
// single requirement before the explorer's refinement logic kicks
// in. Set to 3 so the LLM gets 3 attempts before the system
// intervenes.
const replanRetryThreshold = 3

// ermReadyRequirements returns the requirements whose DependsOn
// prerequisites are all satisfied. Used by the explorer's
// observeSoftStop to determine which requirements to push the LLM
// toward — only ready requirements generate gap hints.
func ermReadyRequirements(reqs []EvidenceRequirement) []EvidenceRequirement {
	satisfied := make(map[string]bool, len(reqs))
	for _, r := range reqs {
		if r.Status == "satisfied" {
			satisfied[ermRequirementID(r)] = true
		}
	}
	var ready []EvidenceRequirement
	for _, r := range reqs {
		if r.Status == "satisfied" {
			continue
		}
		allDeps := true
		for _, dep := range r.DependsOn {
			if !satisfied[dep] {
				allDeps = false
				break
			}
		}
		if allDeps {
			ready = append(ready, r)
		}
	}
	return ready
}

// ermRequirementID produces a stable string key for a requirement,
// used by DependsOn references. Format: "kind:entity1,entity2".
func ermRequirementID(r EvidenceRequirement) string {
	return string(r.Kind) + ":" + strings.Join(r.Entities, ",")
}

// ermMaybeRefine checks whether any stalled requirement should be
// split into per-entity sub-requirements. Returns true if any
// refinement was performed (caller should re-read the requirements).
//
// Refinement rule: when a multi-entity requirement (e.g.
// mechanism(explorer,subagent)) has RetryCount >= replanRetryThreshold
// and Status is still "unsatisfied" or "partial", split it into
// per-entity sub-requirements that each track independently. This
// is the "retry-triggered refinement" from the investigation
// planner audit — it reuses the existing Kind and thresholdForKind
// so no new enum or threshold table is needed.
func ermMaybeRefine(reqs []EvidenceRequirement) ([]EvidenceRequirement, bool) {
	refined := false
	var result []EvidenceRequirement
	for _, r := range reqs {
		if r.RetryCount >= replanRetryThreshold &&
			r.Status != "satisfied" &&
			len(r.Entities) > 1 {
			// Split into per-entity sub-requirements.
			for _, ent := range r.Entities {
				result = append(result, EvidenceRequirement{
					Kind:     r.Kind,
					Entities: []string{ent},
					Reason:   r.Reason + " (refined per-entity after " + fmt.Sprintf("%d", r.RetryCount) + " retries)",
					Status:   "unsatisfied",
				})
			}
			refined = true
			logging.Debug("[erm] refined %s(%s) into %d per-entity sub-requirements after %d retries",
				r.Kind, strings.Join(r.Entities, ","), len(r.Entities), r.RetryCount)
		} else {
			result = append(result, r)
		}
	}
	return result, refined
}

// normalizeForMatch is the package-local alias for the canonical
// identifier-key normaliser. Single-pass builder implementation
// lives in analysis/normalizer.NormalizeCodeKey — this file used to
// carry its own 3-pass ReplaceAll copy; now it is one function with
// one home and two callers (analysis/normalizer/canonicalize.go and
// the 15+ ERM matching sites in this file).
var normalizeForMatch = normalizer.NormalizeCodeKey

// countEvidenceTags was deleted 2026-05-03 (Phase 6 stage 14).
// The function counted bracket-tag substrings ([direct] /
// [registration] / [relationship] / [mechanism] / [conditional])
// in joined ReAct-loop notes. Replacement: countEvidenceByKinds
// reads typed EvidenceKind enum equality on the structured
// evidence pool. Notes-prose bracket scanning depended on the LLM
// voluntarily wrapping its prose in brackets; the typed-kind
// counter reads what the LLM emitted via emit_evidence's typed
// channel.

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
		if evidenceMatchesRequirementEntities(ev, entities) {
			count++
		}
	}
	return count
}

// evidenceMatchesRequirementEntities reports whether `ev` names any
// of the requested entities in a typed identifier slot — Subject,
// Object, or AnchorSymbol — via case-insensitive equality with
// optional NormalizedSurfaceSymbolTail equality. Phase 6 stage 14
// (2026-05-03) replaced the prior `Subject+Object+Summary` substring
// scan with this typed-slot equality. Substring on the concatenated
// blob falsely matched any prose mention of an entity name in the
// Summary string (where Summary is free-form LLM-emitted prose).
// The new path requires the entity to appear as a typed identifier
// slot value, mirroring the stage 13 criterion-side migration.
func evidenceMatchesRequirementEntities(ev types.EvidenceItem, entities []string) bool {
	if len(entities) == 0 {
		return true
	}
	for _, ent := range entities {
		if matchEvidenceSlotsByEntity(ev, ent) {
			return true
		}
	}
	return false
}

// matchEvidenceSlotsByEntity is the shared per-slot match helper
// for ERM (Evidence Requirement Matrix) bookkeeping. Reports
// whether `entity` appears as a substring of any typed identifier
// slot (Subject / Object / AnchorSymbol) in `ev`, case-insensitive,
// after normalizeForMatch.
//
// IMPORTANT: substring (not equality) is intentional here.
// ERM is a BREADTH HEURISTIC, not a grounding gate. Entities come
// from user prose / analyzer Entities ("subagent"), but code
// identifiers are typed but loosely named ("RegisterDefaultSubAgents",
// "SubAgentRegistry", "NewSubExplorer"). Strict equality would
// drop most legitimate matches and stall the ERM satisfaction
// loop. Per the precise-signals-for-hard-gates / noisy-signals-
// for-soft-guidance red line, ERM (breadth bookkeeping) is the
// "soft guidance" tier where substring on typed slots is the
// correct precision.
//
// The Summary field is intentionally NOT consulted — Summary is
// free-form LLM-emitted prose, NOT an identifier slot. Phase 6
// stage 14 (2026-05-03) dropped the Subject+Object+Summary
// concatenation that pre-stage-14 callers used; the new helper
// reads only typed identifier slots.
func matchEvidenceSlotsByEntity(ev types.EvidenceItem, entity string) bool {
	entity = strings.TrimSpace(entity)
	if entity == "" {
		return false
	}
	needle := normalizeForMatch(entity)
	if needle == "" {
		return false
	}
	checkSlot := func(slot string) bool {
		if slot == "" {
			return false
		}
		return strings.Contains(normalizeForMatch(slot), needle)
	}
	return checkSlot(ev.Subject) || checkSlot(ev.Object) || checkSlot(ev.AnchorSymbol)
}

func countEvidenceForRequirement(evidence []types.EvidenceItem, entities []string, reqKind types.RequirementKind) int {
	count := 0
	for _, ev := range evidence {
		if !types.EvidenceStructurallyMatchesRequirement(ev, reqKind) {
			continue
		}
		if evidenceMatchesRequirementEntities(ev, entities) {
			count++
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
				if isRegistrationLikeName(sym.Name, nil) {
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

// isRegistrationLikeName reports whether `name` matches any token
// in `tokens`. Phase 6 stage 15 (2026-05-03): the previous
// hardcoded inline 7-token list was unified with the
// concrete_values producer's 12-token list and made yaml-tunable
// via codrax.yaml :: explore.registration_function_name_tokens
// (DefaultExploreHeuristics().RegistrationFunctionNameTokens).
// Empty `tokens` falls back to defaults so unit tests with the
// zero-value ExploreHeuristics still observe legacy behaviour.
func isRegistrationLikeName(name string, tokens []string) bool {
	if len(tokens) == 0 {
		tokens = types.DefaultExploreHeuristics().RegistrationFunctionNameTokens
	}
	return symbolNameMatchesRegistrationTokens(name, tokens)
}

// symbolNameMatchesRegistrationTokens performs a case-aware
// substring match of `name` against the operator-tunable token
// list. Tokens that are entirely lower-case are matched
// case-insensitively against `strings.ToLower(name)`; tokens
// containing any upper-case rune are matched case-sensitively
// against `name`. This preserves the pre-stage-15 behaviour of
// the inline list where most tokens were lowercase substrings
// (register / config / etc.) but "Map" was case-sensitive (to
// avoid noise on words like "remap").
func symbolNameMatchesRegistrationTokens(name string, tokens []string) bool {
	if name == "" || len(tokens) == 0 {
		return false
	}
	lower := strings.ToLower(name)
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		if isAllLower(tok) {
			if strings.Contains(lower, tok) {
				return true
			}
			continue
		}
		if strings.Contains(name, tok) {
			return true
		}
	}
	return false
}

// isAllLower reports whether every byte in `s` is in [a-z0-9_-].
// Used to decide whether a registration token should be matched
// case-insensitively (lowercase token) or case-sensitively (mixed
// or upper-case token).
func isAllLower(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
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
// themselves registrable. Guards against the t1 explorer self-
// dispatch bug from the 2026-04-13 latency audit.
func hasConcreteRegistrationTarget(entities []string, graph *repomap.Graph) bool {
	if graph == nil {
		return false
	}
	// Build lower→original map once; the helper inverts the loops so
	// the full graph is walked in one pass. Registration targets
	// check lives in isRegistrationTargetKind which takes the full
	// defs slice — so we short-circuit on the first matching symName
	// from inside the visitor by tracking which symNames we already
	// dispatched.
	entMap := entitiesSliceToLowerMap(entities)
	if len(entMap) == 0 {
		return false
	}
	dispatched := make(map[string]bool)
	found := false
	for symName, defs := range graph.SymbolDefs {
		if dispatched[symName] {
			continue
		}
		if _, hit := entMap[strings.ToLower(symName)]; !hit {
			continue
		}
		dispatched[symName] = true
		if isRegistrationTargetKind(defs) {
			found = true
			break
		}
	}
	return found
}

// entitiesSliceToLowerMap builds the lower→original map that
// forEachMatchingDef expects, filtering empty entries. Callers that
// already have a map (most of the explorer.go sites do) pass it in
// directly without going through this helper.
func entitiesSliceToLowerMap(entities []string) map[string]string {
	if len(entities) == 0 {
		return nil
	}
	out := make(map[string]string, len(entities))
	for _, ent := range entities {
		if ent == "" {
			continue
		}
		out[strings.ToLower(ent)] = ent
	}
	return out
}

// forEachMatchingDef iterates over graph definitions whose symbol
// name case-insensitively matches an entity in the supplied
// lower→original map. For each matching (symName, def) pair the
// callback receives (entLower, entOrig, symName, def) with def
// guaranteed non-nil. Returning false aborts iteration early.
//
// Replaces the hand-rolled
//
//	for entLower := range entities {
//	    for symName, defs := range graph.SymbolDefs {
//	        if strings.ToLower(symName) != entLower { continue }
//	        for _, d := range defs { ... }
//	    }
//	}
//
// pattern that appeared four times in explorer.go (primaryEntityFiles,
// buildPrimaryTargetBanner × 2 phases) plus once in this file
// (hasConcreteRegistrationTarget). The outer-loop inversion turns an
// O(|entities| × |SymbolDefs|) scan into O(|SymbolDefs|) while
// preserving behaviour — both forms visit each (symName, def) pair at
// most once per matching entity.
func forEachMatchingDef(
	entities map[string]string,
	graph *repomap.Graph,
	visit func(entLower, entOrig, symName string, d *repomap.Symbol) bool,
) {
	if graph == nil || len(entities) == 0 {
		return
	}
	// Multi-language qualified-name expansion (2026-05-10).
	//
	// LLM-emitted entities can carry package / namespace / scope
	// qualifiers — Go "gate.Run", Rust "mod::Type::method", Ruby
	// "Foo::Bar#baz", Java "com.foo.Bar.method", etc. — but the
	// graph stores symbols by bare name. The legacy single-key
	// lookup `entities[strings.ToLower(symName)]` silently missed
	// every qualified entity, narrowing primaryEntityFiles to
	// whatever side-entity happened to be bare and dragging the
	// evidence filter onto the wrong file. See qualified_name.go
	// for the full failure forensics (s1a 2026-05-10).
	//
	// Build a multi-key index: each lookup key (alias) maps back
	// to the original entity strings that produced it, plus the
	// prefix segments the caller's symbolMatchesQualifier checks
	// against Symbol.Receiver / Symbol.Parent / FileInfo.Package.
	type entityAlias struct {
		orig    string
		entLow  string // pre-normalisation lower
		prefix  []string
	}
	keyToAliases := make(map[string][]entityAlias, len(entities))
	for entLower, entOrig := range entities {
		keys, prefix := expandEntityNameAliases(entOrig)
		for _, k := range keys {
			keyToAliases[k] = append(keyToAliases[k], entityAlias{
				orig:   entOrig,
				entLow: entLower,
				prefix: prefix,
			})
		}
	}
	if len(keyToAliases) == 0 {
		return
	}
	for symName, defs := range graph.SymbolDefs {
		nameLower := strings.ToLower(symName)
		matches, ok := keyToAliases[nameLower]
		if !ok {
			continue
		}
		for _, m := range matches {
			for _, d := range defs {
				if d == nil {
					continue
				}
				// Disambiguate qualified entities. When the entity
				// carried a prefix (e.g. "gate.Run" → prefix ["gate"]),
				// require the symbol to plausibly belong to that
				// scope — Receiver / Parent / FileInfo.Package /
				// directory basename. Single-segment entities (no
				// prefix) skip this check unconditionally.
				if !symbolMatchesQualifier(d, m.prefix, graph) {
					continue
				}
				if !visit(m.entLow, m.orig, symName, d) {
					return
				}
			}
		}
	}
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
//     symbols but are not registrable (2026-04-13 latency audit).
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
		if req.Kind == types.ReqRegistration && len(req.Entities) > 0 {
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
		case types.ReqConditional:
			w.allowConditional = true
		case types.ReqCallChain:
			w.allowRelationshipCall = true
		case types.ReqConfigMapping:
			w.allowMechanismConfig = true
			w.allowConditional = true
		case types.ReqMechanism:
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
// Returns up to maxChains typed AnswerChain records, sorted by
// relevance. Rendering the underlying EvidenceItem to a display string
// happens at prompt-assembly time in context/builder.go — this
// producer stays purely structured per the architecture principle
// "prose only at the LLM boundary".
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
// a fallback safety net with StrictOK=false; they simply never
// outrank a passing chain. Callers may pass nil for both to opt out
// and preserve legacy ranking behaviour (used by older tests).
func identifyAnswerChains(question string, evidence []types.EvidenceItem, maxChains int, whitelist answerPredicateWhitelist, reqs []EvidenceRequirement, graph *repomap.Graph) []types.AnswerChain {
	entities := extractRankingEntitiesWithGraph(question, graph)
	if len(entities) == 0 || len(evidence) == 0 {
		return nil
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

		// Session 11 R3 axis-aware chain demote. When the question's
		// primary entity is named as the chain's terminal literal
		// (e.g. "Explorer.Name() returns \"explorer\"" for the
		// question "which skill does explorer use"), the chain is
		// self-referential — it resolves primary entity → primary
		// entity's own name — not primary entity → its property.
		// Demote by ×0.2, matching the terminal-predicate demote
		// factor so the two reasons are symmetric.
		if isChain && len(entities) > 0 {
			primary := entities[0]
			if primary != "" && chainTerminalIsSelfRef(ev.Summary, primary) {
				bonus *= 0.2
				// Session 11 G7 R3 — ledger hookup via the
				// ambient closure fn (set by the caller before
				// identifyAnswerChains runs). When unset the
				// demote is log-only, preserving the legacy
				// test paths that do not wire a closure. The
				// F2 aggregator picks up ViolChainDemoted events
				// to correlate self-ref signal across chains +
				// evidence.
				if f := activeLedgerHook; f != nil {
					f(types.Violation{
						Kind:       types.ViolChainDemoted,
						Detail:     fmt.Sprintf("chain terminal equals primary entity %q (self-ref)", primary),
						ClusterKey: types.SymbolClusterKey(primary, "answer_subject.kind"),
						Stage:      string(types.StageExplore),
						SuspectedRoot: types.SuspectedRoot{
							IRField:    "answer_subject.kind",
							Reason:     "chain terminal is primary_entity self-name — not an attribute lookup",
							Confidence: 0.80,
						},
					})
				}
				logging.Debug("[erm] R3 axis-aware demote: chain terminal == primary entity %q", primary)
			}
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
		return nil
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

	// Build a single unified AnswerChain slice. Dedup by the
	// identity tuple (Summary, Source, LineStart, Subject, Predicate,
	// Object) — matches the legacy "two items with identical display
	// text are the same chain" semantic while still using structured
	// fields instead of the pre-rendered string.
	//
	// Cap semantics preserve legacy behaviour: up to maxChains
	// loose-slot admissions, plus additional strict items until the
	// strict cap also hits maxChains. In the common case (top-ranked
	// items are strict) the two caps converge on a single maxChains-
	// sized slice. Under heavy demotion (top maxChains are all
	// demoted) the slice extends past maxChains to ensure strict items
	// still reach downstream.
	seen := make(map[string]bool)
	var out []types.AnswerChain
	looseFilled := 0
	strictFilled := 0
	for _, c := range candidates {
		key := fmt.Sprintf("%s|%s|%d|%s|%s|%s",
			c.src.Summary, c.src.Source, c.src.LineStart,
			c.src.Subject, c.src.Predicate, c.src.Object)
		if seen[key] {
			continue
		}
		admit := false
		if looseFilled < maxChains {
			admit = true
			looseFilled++
		}
		if c.strictOK && strictFilled < maxChains {
			admit = true
			strictFilled++
		}
		if !admit {
			continue
		}
		seen[key] = true
		out = append(out, types.AnswerChain{
			Item:     c.src,
			Score:    c.score,
			StrictOK: c.strictOK,
		})
	}
	return out
}

// hasTerminalEvidence reports whether any item in the strict subset
// carries a structurally single-symbol terminal shape. Called by
// Turn A ParseOutput to compute terminalEvidenceCount (β) which
// becomes the cardinality baseline Turn B (extractor) cross-checks
// the emitted answer-symbol slate against.
//
// `decorates` and `maps` Concrete evidence are recognised as
// terminal shapes alongside registration chains. Both carry an
// `X → Y` hop pair where the terminal Y is a handler/value.
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

func hasGroundedTerminalEvidence(items []types.EvidenceItem) bool {
	filtered := make([]types.EvidenceItem, 0, len(items))
	for _, ev := range items {
		switch ev.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered:
			filtered = append(filtered, ev)
		}
	}
	return hasTerminalEvidence(filtered)
}

func hasGroundedRequirementCarrier(items []types.EvidenceItem, reqs []EvidenceRequirement) bool {
	if len(items) == 0 || len(reqs) == 0 {
		return false
	}
	for _, ev := range items {
		switch ev.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered, "":
		default:
			continue
		}
		for _, req := range reqs {
			if types.EvidenceStructurallyMatchesRequirement(ev, req.Kind) &&
				evidenceMatchesRequirementEntities(ev, req.Entities) {
				return true
			}
		}
	}
	return false
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
var terminalPredicateByKind = map[types.RequirementKind]terminalPredicate{
	types.ReqRegistration: terminalIsConcreteSymbolRef,
	types.ReqCallChain:    terminalIsConcreteSymbolRef,
	types.ReqReturnValue:  terminalIsConcreteLiteral,
	types.ReqHistory:      terminalIsConcreteLiteral,
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
var originPredicateByKind = map[types.RequirementKind]terminalPredicate{
	types.ReqRegistration: chainOriginIsRegistrationLinkage,
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
func predicatesFor(reqs []EvidenceRequirement, table map[types.RequirementKind]terminalPredicate) []terminalPredicate {
	if len(reqs) == 0 || len(table) == 0 {
		return nil
	}
	seen := make(map[types.RequirementKind]bool, len(table))
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

// normalizedChainTerminal returns the chain's rightmost hop with two
// noise sources stripped: surrounding whitespace, and a trailing
// ` (file:line)` source locator some render paths append. This is the
// canonical "answer terminal" form used by:
//
//   - β (TerminalEvidenceCount) dedup — two chains pointing at the
//     same answer produce identical strings.
//   - terminalIsConcreteSymbolRef — shape-match on method calls /
//     identifier references.
//   - endsWithShortLiteralReturn — shape-match on `returns "..."`.
//
// Normalisation is deliberately minimal — anything more aggressive
// (e.g. collapsing all "returns X" terminals regardless of X) risks
// false-positive merges that would DROP distinct answers.
func normalizedChainTerminal(chainText string) string {
	t := extractTerminalSegment(chainText)
	if p := strings.LastIndex(t, " ("); p >= 0 && strings.HasSuffix(t, ")") {
		t = strings.TrimSpace(t[:p])
	}
	return t
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
	terminal := normalizedChainTerminal(chainText)
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
	seg := normalizedChainTerminal(chain)
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

// activeLedgerHook is a package-level pointer to a closure that
// wraps EvidenceClosure.AppendViolation. Installed by the caller
// (explorer.go) via setLedgerHook BEFORE identifyAnswerChains
// runs and cleared immediately after. Session 11 G7 — the hook
// lets R3's axis-aware demote write ViolChainDemoted entries
// without threading a closure pointer through all the ranking
// helpers. Test paths that do not install a hook get log-only
// behaviour, preserving legacy test determinism.
var activeLedgerHook func(types.Violation)

// SetLedgerHook replaces the ambient ledger writer. Call with nil
// to clear. Safe to call from the main explorer goroutine because
// identifyAnswerChains is sequential within one explore window;
// parallel explore dispatches are not supported.
func SetLedgerHook(h func(types.Violation)) {
	activeLedgerHook = h
}

// chainTerminalIsSelfRef reports whether a chain summary
// terminates in a quoted literal equal to the question's primary
// entity name. Used by Session 11 R3 to demote self-referential
// chains when the question axis implies a real relational answer
// (e.g. "which skill does X use" should not resolve to X.Name()
// returning "X" itself).
//
// Heuristic: look for `returns "<primary>"` or `returns '<primary>'`
// anywhere in the Summary (after normalising case on the literal
// but NOT on the verb — chain summaries consistently use the
// lowercase "returns" verb). False positives are acceptable because
// the demote is non-lethal (×0.2, not ×0); real self-name chains
// with no better candidate still win.
func chainTerminalIsSelfRef(summary, primaryEntity string) bool {
	if summary == "" || primaryEntity == "" {
		return false
	}
	lit1 := `returns "` + primaryEntity + `"`
	lit2 := `returns '` + primaryEntity + `'`
	lower := strings.ToLower(summary)
	lowerEntity := strings.ToLower(primaryEntity)
	return strings.Contains(lower, `returns "`+lowerEntity+`"`) ||
		strings.Contains(lower, `returns '`+lowerEntity+`'`) ||
		strings.Contains(summary, lit1) || strings.Contains(summary, lit2)
}
