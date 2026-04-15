package skill

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// analysis_contract.go is the single source of truth for the analyzer
// classification contract. Everything the analyzer agent, the
// emit_analysis tool schema, and the "analysis-skill" system prompt
// need to agree on about field names, enum values, enum descriptions,
// and hard rules lives here.
//
// Before this file existed the same contract was triplicated across
// internal/skill/defaults.go (task-analysis-skill), internal/agent/
// analyzer.go (BuildInitialInstruction hardcoded text), and internal/tool/
// emit_analysis.go (Description + Parameters JSON). Any change to an
// enum value or wording required editing three files in two packages
// and nothing enforced consistency, so values drifted. Now:
//
//   - analyzer agent's BuildInitialInstruction returns "" and the skill
//     sections rendered from BuildAnalysisSkill() are the only static
//     prompt text the analyzer sees;
//   - emit_analysis.go builds its JSON schema enum arrays from
//     AnalysisIntentValues() / AnalysisScenarioValues() / ... so the
//     tool schema literally cannot drift from the skill text;
//   - a consistency test in internal/tool/ deep-equals the two sides.

// AnalysisEnumChoice is one row in a classification enum table:
// the canonical value string plus a one-line description that the
// LLM sees next to the value.
type AnalysisEnumChoice struct {
	Value string
	Desc  string
}

// analysisIntents is the canonical intent enum. Values match
// types.Intent constants — keep in sync with
// internal/types/analysis_ir.go.
var analysisIntents = []AnalysisEnumChoice{
	{string(types.IntentExplain), "user wants to understand how something works"},
	{string(types.IntentRootCause), "user is debugging, asks \"why does X fail\""},
	{string(types.IntentTrace), "follow a data flow or call chain end to end"},
	{string(types.IntentEnumerate), "list every X, count Xs (also for \"which agents call Y\")"},
	{string(types.IntentConfigQuery), "look up what a config key controls"},
	{string(types.IntentReturnValue), "asks what a specific function returns or its literal name"},
	{string(types.IntentUnknown), "genuinely ambiguous (ERM will fall back to keyword inference)"},
}

// analysisScenarios is the canonical scenario enum. Values match
// types.Scenario constants.
var analysisScenarios = []AnalysisEnumChoice{
	{string(types.ScenarioArchitectureExplain), "explain mechanism / code / flow"},
	{string(types.ScenarioRootCause), "debug a failure"},
	{string(types.ScenarioConfigTrace), "trace config → behaviour"},
	{string(types.ScenarioPerformanceBottleneck), "find a perf hotspot"},
	{string(types.ScenarioGeneric), "none of the above (safe fallback)"},
}

// analysisComplexities is the canonical complexity enum. Values match
// types.Complexity constants.
var analysisComplexities = []AnalysisEnumChoice{
	{string(types.ComplexitySimple), "single lookup/count, 1-2 files"},
	{string(types.ComplexityModerate), "single component, 3-5 files"},
	{string(types.ComplexityComplex), "cross-component, 6+ files"},
}

// analysisQuestionKinds is the canonical question_kind enum. Unlike
// the others, question_kind has no typed Go constant — it is the ERM
// predicate selector consumed downstream as a raw string. This table
// is the only place the values live.
var analysisQuestionKinds = []AnalysisEnumChoice{
	{"registration", "\"which/how many X register/bind Y\", \"X 是在哪注册的\""},
	{"mechanism", "\"how does X work\", \"explain the process of X\", \"X 怎么实现\""},
	{"return_value", "\"what does X return\", \"X.Name() 是什么\""},
	{"conditional", "\"when does X fire\", \"under what condition\", \"什么时候\""},
	{"config_mapping", "\"what does config key K control\""},
	{"enumeration", "\"list all X\", \"count of X\""},
	{"call_chain", "\"which X calls Y\", \"从 A 到 B 怎么调用的\""},
	{"unknown", "genuinely ambiguous"},
}

// analysisAnswerShapes is the canonical answer_shape enum. Values
// match types.AnswerShape constants (plus the "none" sentinel).
var analysisAnswerShapes = []AnalysisEnumChoice{
	{string(types.ShapeListOfSymbols), "answer is a set of identifier names (forbids symbols not in Ground Truth evidence)"},
	{string(types.ShapeStepList), "ordered steps of a mechanism"},
	{string(types.ShapeValue), "a single literal / returned value"},
	{string(types.ShapeBoolean), "yes/no"},
	{string(types.ShapeConfigValue), "a resolved config key value"},
	{string(types.ShapeExplanation), "long prose explanation"},
	{string(types.ShapeNone), "no structured shape applies"},
}

// AnalysisIntentChoices returns the canonical intent enum table.
// Callers must not mutate the returned slice.
func AnalysisIntentChoices() []AnalysisEnumChoice { return analysisIntents }

// AnalysisScenarioChoices returns the canonical scenario enum table.
func AnalysisScenarioChoices() []AnalysisEnumChoice { return analysisScenarios }

// AnalysisComplexityChoices returns the canonical complexity enum table.
func AnalysisComplexityChoices() []AnalysisEnumChoice { return analysisComplexities }

// AnalysisQuestionKindChoices returns the canonical question_kind enum
// table.
func AnalysisQuestionKindChoices() []AnalysisEnumChoice { return analysisQuestionKinds }

// AnalysisAnswerShapeChoices returns the canonical answer_shape enum
// table.
func AnalysisAnswerShapeChoices() []AnalysisEnumChoice { return analysisAnswerShapes }

// AnalysisIntentValues returns the intent enum values in canonical
// order. This is the slice the emit_analysis JSON schema reads.
func AnalysisIntentValues() []string { return enumValues(analysisIntents) }

// AnalysisScenarioValues returns the scenario enum values in canonical order.
func AnalysisScenarioValues() []string { return enumValues(analysisScenarios) }

// AnalysisComplexityValues returns the complexity enum values in canonical order.
func AnalysisComplexityValues() []string { return enumValues(analysisComplexities) }

// AnalysisQuestionKindValues returns the question_kind enum values in canonical order.
func AnalysisQuestionKindValues() []string { return enumValues(analysisQuestionKinds) }

// AnalysisAnswerShapeValues returns the answer_shape enum values in canonical order.
func AnalysisAnswerShapeValues() []string { return enumValues(analysisAnswerShapes) }

func enumValues(choices []AnalysisEnumChoice) []string {
	out := make([]string, len(choices))
	for i, c := range choices {
		out[i] = c.Value
	}
	return out
}

// AnalysisHardRules is the list of non-negotiable rules the analyzer
// agent must obey. Rendered verbatim into the analysis-skill's
// Prohibitions section by BuildAnalysisSkill so the analyzer agent
// never needs to restate them in its own prompt.
var AnalysisHardRules = []string{
	"every field in emit_analysis is REQUIRED (keywords and entities may be empty arrays); missing required fields rejects the call",
	"entities come from the user's ORIGINAL text only — \"ContinuationPrompt\" stays as \"ContinuationPrompt\", not \"continuation prompt\" or \"continuation_prompt\"",
	"do not invent an intent by stretching a category; if two fit equally, pick the one that matches the user's verb; if none fit, use \"unknown\"",
	"answer_shape=list_of_symbols ONLY when the user is asking for a SET of names they want listed — \"how many agents call X\" is list_of_symbols, \"is X registered\" is boolean, \"explain X\" is step_list or explanation",
	"do NOT call any tool other than emit_analysis",
	"do NOT write free-form prose before the emit_analysis call — emit_analysis is the only output channel for the analyze stage",
	"do not translate or re-case entities — copy them verbatim from the user's text",
	"do not make assumptions about code structure — classify only what the user's text supports",
}

// renderEnumTable formats one enum table as the bulleted block the
// skill's OutputFormat section shows to the LLM. Pure formatting,
// zero logic — kept local to this file so every enum section has a
// uniform look and one grep reaches both sides of the contract.
func renderEnumTable(field string, choices []AnalysisEnumChoice) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — pick one:\n", field)
	for _, c := range choices {
		fmt.Fprintf(&b, "  %s — %s\n", c.Value, c.Desc)
	}
	return b.String()
}

// BuildAnalysisSkill returns the single analysis-skill Config the
// analyze stage binds to. All static analyzer-contract text — field
// descriptions, enum tables, hard rules — is assembled from the
// tables above, so the only way to change the contract is to edit
// this file.
func BuildAnalysisSkill() *Config {
	var of strings.Builder
	of.WriteString("Call emit_analysis EXACTLY ONCE with all required fields: intent, scenario, complexity, keywords, entities, question_kind, answer_shape. " +
		"The system synthesises the TermGraph, TaskGraph, RiskMatrix, EvidencePlan, AnswerContract, and Hypotheses deterministically from your input — do not provide them.\n\n")
	of.WriteString("Field enums:\n\n")
	of.WriteString(renderEnumTable("intent", analysisIntents))
	of.WriteString("\n")
	of.WriteString(renderEnumTable("scenario", analysisScenarios))
	of.WriteString("\n")
	of.WriteString(renderEnumTable("complexity", analysisComplexities))
	of.WriteString("\n")
	of.WriteString(renderEnumTable("question_kind", analysisQuestionKinds))
	of.WriteString("\n")
	of.WriteString(renderEnumTable("answer_shape", analysisAnswerShapes))
	of.WriteString("\n")
	of.WriteString("Entities: CamelCase/snake_case symbol names copied VERBATIM from the user's wording. Do NOT translate, re-case, pluralise, or paraphrase. Generic nouns (count, function, thing, agent, handler, module) MUST NOT appear here — they poison ERM ranking. Leave empty only when the question has no identifier-looking tokens.\n\n")
	of.WriteString("Keyword generation (3 rounds): (1) Core — extract every domain noun and verb from the question in both original and identifier forms (CamelCase, snake_case). (2) Compound — cross-combine core terms into plausible multi-word identifiers (CacheStore, store_config). (3) Synonyms — for each verb add 2-3 programming synonyms (send → emit/dispatch/publish). Target ≥8 diverse stems. For Chinese questions include BOTH Chinese and English forms (the codebase is English). The system auto-expands each keyword into case variants, so produce diverse STEMS rather than repeating words.\n\n")
	of.WriteString("After emit_analysis succeeds, you may add a one-paragraph rationale for the trace log. This text is captured but does not drive any agent — the structured fields are what matter.")

	return &Config{
		Name: "analysis-skill",
		Goal: "Classify the user request into a RequestModel (intent, scenario, complexity, keywords, entities, question_kind, answer_shape) via a single emit_analysis tool call.",
		Workflow: []string{
			"read the user input and detect its language",
			"pick intent from the intent enum",
			"pick scenario from the scenario enum",
			"pick complexity from the complexity enum (use the \"how many files\" hints)",
			"extract entities VERBATIM from the user's text — CamelCase/snake_case only, no generic nouns",
			"generate ≥8 keywords in three rounds — core terms, compound identifiers, action synonyms",
			"pick question_kind and answer_shape from their enums",
			"call emit_analysis EXACTLY ONCE with the classified fields",
		},
		ToolSuggestions: []string{"emit_analysis"},
		OutputFormat:    of.String(),
		Prohibitions:    append([]string(nil), AnalysisHardRules...),
	}
}
