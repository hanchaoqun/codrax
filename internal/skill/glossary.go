package skill

// glossary.go is the single source of truth for LLM-facing prompt
// hygiene. The lists below drive glossary_test.go's three lint tests
// (prompts, tool schemas, retry/mid-loop hints) — any listed token
// appearing in a rendered LLM prompt fails the build.

// ProjectSpecificIdentifierBlocklist is the curated denylist of
// project-specific identifiers (eval-case-specific config keys,
// repo file paths used as worked examples, etc.) that MUST NEVER
// appear in LLM-facing prompt text. Distinct from
// InternalTermsBlocklist (which catches Go internal type names);
// this list catches the orthogonal pattern where a writer pastes
// a concrete project identifier ("explore_mid_loop_hint_budget" /
// "codrax.yaml") into a worked-example block, which over-fits the
// prompt to one eval case and violates the
// feedback_generalization_over_project_success.md red line.
//
// Maintenance: every time an eval case introduces a new
// project-specific identifier (config key / file path / function
// name) that gets used in a worked example or schema description,
// audit the surrounding prompt — if the identifier is project-
// specific (i.e. would not exist in another codrax-shaped repo),
// add it here so the lint catches a future paste-in. The list is
// allowed to grow; false-positive risk is near-zero because each
// entry is a real concrete identifier.
//
// Scanned by the same 3 glossary lint tests
// (TestNoInternalTermsInHints / TestNoInternalTermsInToolSchemas /
// TestInternalTerms*) — a hit fails the test with a "rephrase as
// a generic placeholder" suggestion.
var ProjectSpecificIdentifierBlocklist = []string{
	// s3a (config-trace) eval case — the missing config key the
	// question asks about.
	"explore_mid_loop_hint_budget",
	// s3a / m1a — the repo's canonical config sample file basename.
	// Path-shape fixtures should use generic placeholders like
	// `<config-file>` instead.
	"codrax.yaml",
	// s3a — the structural CLI registration site. Worked examples
	// that walk a 3-layer override chain should reference the role
	// (`cli_registration` enum value) not the concrete repo path.
	"cmd/root.go",
}

// InternalTermsBlocklist is the set of implementation-jargon tokens
// that must NEVER appear in LLM-facing prompt text. Each entry is a
// plain substring; the lint does a case-sensitive Contains check on
// every rendered string literal, so acronyms (e.g. "CGEC", "ERM")
// catch as-is while concepts that have multiple surface forms appear
// once per form.
//
// Guidance for writers: if you need to reference an internal concept
// in a prompt, either
//   (a) rephrase in user-facing language (e.g. "AnalysisIR" → "the
//       analysis contract"; "TaskGraph" → "the evidence plan"), or
//   (b) extend the vocabulary here with a new abstract name and keep
//       the original token off-limits.
//
// Keep the list ordered by category so a reviewer can see the family
// each token belongs to at a glance.
var InternalTermsBlocklist = []string{
	// Core IR / data-structure type names — never mention to the LLM.
	"AnalysisIR",
	"RequestModel",
	"AnswerContract",
	"AnalyzerHints",
	"TermGraph",
	"TaskGraph",
	"EvidencePlan",
	"QualityGate",
	"RiskMatrix",
	"HypothesisSet",
	"MutableState",
	"BusContext",
	"StageOutput",
	"EvidenceClosure",
	"ReadSet",
	"PendingReads",
	"RepairDirective",
	"GroundingStatus",
	"AnchorKind",

	// Semantic Surface Contract — Phase 1 internal type names.
	// Phase 2 finalizer prompt MUST refer to these via abstract
	// human-readable labels (see answer_document_evaluator.go ::
	// answerDocFacetLabels / answerDocClaimFormLabels). The blocklist
	// catches accidental drift from a future writer pasting a Go name.
	"FacetCoverageContract",
	"FacetCoverage",
	"FacetRequirement",
	"AnswerFacetKind",
	"QuestionFamily",
	"ClaimFormOf",
	"FacetRequiredness",
	"AcceptableForms",
	"SourceCandidate",

	// Block-Only Carrier (B6 of block_only_carrier.md, 2026-05-03)
	// — V2 carrier internal type names. The finalizer prompt is
	// allowed to use the LLM-facing block kinds (summary / section
	// / ordered_list / etc.) and the LLM-facing facet labels, but
	// MUST NOT leak the Go type names listed here.
	"AnswerSemanticView",
	"BlockRequirement",
	"AnswerBlockKind",
	"DiagramFacetGraph",
	"UncertaintyRule",
	"RichnessCandidate",
	"AnswerDocumentV2",
	"AnswerBlock",
	"AnswerBlockItem",
	"AnswerDiagramBlock",

	// Contract-field leakage (Go constant names surfacing as prose).
	"MustInclude",
	"must-include floor",
	"must-include count",
	"must-include list",
	"cardinality baseline",
	"cardinality cross-check",
	"terminal-evidence count",
	"effective floor",
	"Ground Truth evidence",

	// Internal pipeline vocabulary (stage/phase/tier labels).
	"Turn A",
	"Turn B",
	"TurnA",
	"TurnB",
	"Phase 0",
	"Phase 1",
	"Phase 2",
	"Phase 3",
	"Phase 4",
	"Phase 5",

	// Pipeline stage names — internal codenames for the
	// analyze/explore/extract/finalize 4-stage taxonomy. LLM-facing
	// prompts must use neutral language ("the answer-rendering
	// stage" / "during answer composition" / "the rendered answer")
	// instead of leaking the stage codename.
	"finalizer",
	"Tier 1",
	"Tier 2",
	"T1a",
	"T1b",
	"T1c",

	// Design-doc / commit-tracking acronyms.
	"CGEC",
	"ERM",
	"HDP",
	"C0'",

	// Validator / gate machinery disclosure.
	"findings_validator",
	"literal-grounding gate",
	"coverage gate",
	"phase1_unread gate",
	"phase1-unread gate",
	"the system validates",
	"the system does",
	"the system will",
	"the system strips",
	"the system drops",
	"the system filters",
	"the system derives",
	"the system caps",
	"the system does with your output",
	"deterministic validator",

	// Phase 6 stage 30 (2026-05-03) — broader "the system X"
	// pipeline-mechanics leakage. The retired-elsewhere phrases
	// reveal post-emit pipeline behaviour ("the system handles
	// hedging", "the system stops when applied", "the system
	// auto-expands keywords") that the LLM cannot act on and
	// that anchors prompts to current implementation.
	"the system uses",
	"the system handles",
	"the system stops",
	"the system completes",
	"the system maps",
	"the system auto-expands",
	"the system has a deterministic fallback",
	"the system rejects",
	"the system added",
	"the system injects",
	"the system annotations",
	"system-owned",
	"the system's deterministic",
	"the system's old",

	// Cross-stage architecture disclosure ("downstream stages",
	// "downstream search") — leaks the multi-stage pipeline
	// shape to the LLM. Use neutral references ("later steps",
	// "the rendering pipeline", or just describe what the LLM
	// itself does/avoids).
	"downstream stages",
	"downstream stage",

	// Log-triage internal layer numbers.
	"Layer 1",
	"Layer 2",
	"Layer 3",
	"Layer 4",

	// Session / commit tracking (should only appear in Go comments).
	"session 1",
	"session 2",
	"session 3",
	"session 4",
	"session 5",
	"session 6",
	"session 7",
	"session 8",
	"session 9",
	"session 10",
	"Session-22",
	"Session 22",
}

// KeywordExamplePhrases is the auxiliary blocklist used by
// TestNoKeywordExamplesInEnums (batch 4). These phrases — quoted
// user-wording examples — must not appear inside classification-enum
// descriptions because the red line feedback_no_custom_keyword_matching
// (2026-04-22) forbids fixing LLM classification by keyword-list
// matching; classification shape must come from structural schema
// fields (predicates.*, answer_subject, predicate_axis) instead.
//
// Each entry is a substring and the lint scans enum description
// fields case-insensitively. Keep examples in the forms they currently
// appear in the prompt so the initial sweep reports every location.
var KeywordExamplePhrases = []string{
	// English imperative cues that nudge classification.
	"how many X",
	"size of Y",
	"what does X return",
	"list all X",
	"list every X",
	"X matching pattern Y",
	"which/how many X",
	"when does X fire",
	"under what condition",
	"count of X",
	"what is X",
	"where is X defined",
	"does X exist",
	"how does X work",
	"what does X do",
	"explain X",
	"compare A and B",
	"how does X affect Y",
	"trace flow from A to B",
	"total of Y",

	// Chinese imperative cues. Use raw Chinese so the prompt corpus
	// scanner catches them byte-for-byte.
	"X 是什么",
	"X 在哪定义",
	"X 怎么工作",
	"X 的作用",
	"X 怎么实现",
	"统计",
	"多少",
	"几个",
	"有多少",
	"对比",
	"X 是在哪注册的",
	"从 A 到 B 如何传递",
	"从 A 到 B 怎么调用的",
	"X.Name() 是什么",
	"统计…数量",
	"什么时候",
}
