package skill

// glossary.go is the single source of truth for LLM-facing prompt
// hygiene. The lists below drive:
//
//   - glossary_test.go's three lint tests (prompts, tool schemas,
//     retry/mid-loop hints) that fail the build when any listed token
//     surfaces in a rendered LLM prompt;
//   - the planned four-batch prompt-cleanup refactor described in
//     docs/prompt_glossary.md; entries graduate from "report-only"
//     (t.Log) to "hard-fail" (t.Fatal) as batches 2/3/4 land.
//
// Batch 1 ships the file + lint skeleton in REPORT-ONLY mode. Later
// batches purge the terms and flip the switches.

// InternalTermsBlocklist is the set of implementation-jargon tokens
// that must NEVER appear in LLM-facing prompt text. Each entry is a
// plain substring; the lint does a case-sensitive Contains check on
// every rendered string literal, so acronyms (e.g. "CGEC", "ERM")
// catch as-is while concepts that have multiple surface forms appear
// once per form.
//
// Guidance for writers: if you need to reference an internal concept
// in a prompt, either
//   (a) rephrase in user-facing language (see docs/prompt_glossary.md
//       for the replacement table), or
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
