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

// analysisQuestionKinds is the canonical question_kind enum. Values
// are sourced from types.AllRequirementKinds() so adding a new
// question kind requires editing internal/types/requirement_kind.go
// and nothing else — the skill prompt, the emit_analysis schema, and
// the ERM predicate selector all pick up the change automatically.
// Descriptions live here because they are user-facing prompt copy.
//
// The table is built once at package init; the mutable rendering is
// done through AnalysisQuestionKindChoices()/Values() accessors.
var analysisQuestionKinds = buildAnalysisQuestionKinds()

// questionKindDescriptions pairs each RequirementKind with the
// prompt-copy string the LLM sees. Kinds without an entry fall back
// to string(kind) for the description. Keeping descriptions here
// (not in internal/types) avoids leaking prompt copy into the type
// definition.
var questionKindDescriptions = map[types.RequirementKind]string{
	types.ReqRegistration:  "\"which/how many X register/bind Y\", \"X 是在哪注册的\"",
	types.ReqMechanism:     "\"how does X work\", \"explain the process of X\", \"X 怎么实现\"",
	types.ReqReturnValue:   "\"what does X return\", \"X.Name() 是什么\"",
	types.ReqConditional:   "\"when does X fire\", \"under what condition\", \"什么时候\"",
	types.ReqConfigMapping: "\"what does config key K control\"",
	types.ReqEnumeration:   "\"list all X\", \"count of X\"",
	types.ReqCallChain:     "\"which X calls Y\", \"从 A 到 B 怎么调用的\"",
}

func buildAnalysisQuestionKinds() []AnalysisEnumChoice {
	kinds := types.AllRequirementKinds()
	out := make([]AnalysisEnumChoice, 0, len(kinds)+1)
	for _, k := range kinds {
		desc, ok := questionKindDescriptions[k]
		if !ok {
			desc = string(k)
		}
		out = append(out, AnalysisEnumChoice{Value: string(k), Desc: desc})
	}
	// "unknown" is the analyzer-facing fallback when the LLM cannot
	// classify. It is NOT a RequirementKind value (ReqUnknown is the
	// zero value and has empty string form), so we append it here as
	// a rendering-only tail entry.
	out = append(out, AnalysisEnumChoice{Value: "unknown", Desc: "genuinely ambiguous"})
	return out
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
//
// The evidence-lite pre-scan rules (no read_file, grep must use
// files_only=true, no deep content reasoning) are grouped together
// at the end so the contract between the analyzer and the explorer
// stays grep-visible: the analyzer verifies that symbols and terms
// EXIST in the repo; the explorer does the content reading.
var AnalysisHardRules = []string{
	"every field in emit_analysis is REQUIRED (keywords and entities may be empty arrays); missing required fields rejects the call",
	"entities come from the user's ORIGINAL text only — \"ContinuationPrompt\" stays as \"ContinuationPrompt\", not \"continuation prompt\" or \"continuation_prompt\"",
	"do not invent an intent by stretching a category; if two fit equally, pick the one that matches the user's verb; if none fit, use \"unknown\"",
	"answer_shape=list_of_symbols ONLY when the user is asking for a SET of names they want listed — \"how many agents call X\" is list_of_symbols, \"is X registered\" is boolean, \"explain X\" is step_list or explanation",
	"call emit_analysis EXACTLY ONCE — multiple calls trigger a warning (or a hard reject when analysis_reject_multiple_emit=true) and only the last write is effective",
	"do NOT write free-form prose before the emit_analysis call beyond what the evidence-lite pre-scan requires — emit_analysis is the final output channel of the analyze stage",
	"do not translate or re-case entities — copy them verbatim from the user's text",
	"do not make assumptions about code structure — classify only what the user's text plus the evidence-lite pre-scan support",
	// Evidence-lite pre-scan boundary rules.
	"EVIDENCE-LITE BOUNDARY: do NOT call read_file, exec_command, or any tool that reads file CONTENT — the analyze stage is existence + location verification only, not content inspection; the explore stage owns deep reading",
	"EVIDENCE-LITE BOUNDARY: when calling grep, ALWAYS pass files_only=true — line-level results are noise at the analyze stage and will overflow the budget",
	"EVIDENCE-LITE BOUNDARY: pre-scan is limited to 1-2 rounds of lightweight calls (repo_map, grep files_only=true, list_files) before emit_analysis; do NOT loop on pre-scan tools — two rounds is the hard ceiling",
	"EVIDENCE-LITE BOUNDARY: the pre-scan answers \"does X exist / in which files does term Y appear\" and nothing else; do not draw conclusions about how code WORKS from file paths alone — save that for the explorer",
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

// AnalysisToolSuggestions is the allowlist of tools the analyze
// stage's BaseAgent exposes to the LLM. The analyzer is a "pure
// language classifier" by default and used to see only emit_analysis
// — which meant every entity it extracted was guesswork against a
// repo it had never touched, and that guesswork flowed straight into
// the explorer's seed keywords. The evidence-lite pre-scan adds
// three read-only navigation tools so the analyzer can verify
// "does this symbol exist? in which files does this term appear?"
// before committing to a classification:
//
//   - repo_map    — structural index (tasks view), for discovering
//     which files are relevant to a term
//   - grep        — MUST be called with files_only=true (line-level
//     results are noise at the analyze stage and blow the budget)
//   - list_files  — directory listing for when grep / repo_map come
//     back empty and the analyzer wants to know what's even there
//
// read_file, exec_command, and the explorer-owned emit_evidence
// channel are intentionally absent: the analyze stage is existence
// + location verification, not content inspection. The explorer
// owns deep reading. BaseAgent.buildToolSchemas enforces this by
// only adding tools listed here to the LLM's schema set, so the
// analyzer's LLM has no tool it can call to break the boundary.
var AnalysisToolSuggestions = []string{
	"emit_analysis",
	"repo_map",
	"grep",
	"list_files",
}

// BuildAnalysisSkill returns the single analysis-skill Config the
// analyze stage binds to. All static analyzer-contract text — field
// descriptions, enum tables, hard rules — is assembled from the
// tables above, so the only way to change the contract is to edit
// this file.
func BuildAnalysisSkill() *Config {
	var of strings.Builder

	// Evidence-lite pre-scan preamble. Runs BEFORE the field-enum
	// tables so the LLM sees the workflow framing first: pre-scan,
	// then classify, then emit_analysis.
	of.WriteString("## Evidence-lite pre-scan (1-2 rounds, then emit_analysis)\n\n")
	of.WriteString("Spend 1-2 rounds verifying that the entities you plan to extract from the user's wording actually exist in this repository and that the terms you plan to put into keywords appear somewhere relevant. Use ONLY these low-cost navigation tools:\n\n")
	of.WriteString("  - `repo_map` — structural index of the repo, for discovering which files are relevant to a term.\n")
	of.WriteString("  - `grep` — MUST be called with `files_only=true`. Line-level results are too noisy for the analyze stage and will overflow the budget. `files_only=true` returns just the file paths that contain matches, which is what you need.\n")
	of.WriteString("  - `list_files` — fall back here when grep / repo_map come back empty and you want to know what's even in a directory.\n\n")
	of.WriteString("You are FORBIDDEN from calling `read_file`, `exec_command`, or any tool that reads file CONTENT — the analyze stage is existence + location verification, not content inspection. The explorer stage runs next and owns deep reading. Do NOT draw conclusions about how code WORKS from file paths alone; the pre-scan answers \"does X exist / in which files does term Y appear\" and nothing else.\n\n")
	of.WriteString("Pre-scan budget: at most two rounds. If round 1 confirmed your entities and keywords, go straight to emit_analysis. If round 1 came back empty on a symbol, spend round 2 on broader search (strip camelCase, try stems, drop qualifiers) — then emit_analysis regardless of whether the symbol was found. An entity that does not appear in the repo still belongs in the `entities` array exactly as the user wrote it; the downstream ranking layer will handle non-existent terms.\n\n")
	of.WriteString("A runtime gate enforces the 2-round ceiling: after two pre-scan rounds a call to `repo_map` / `grep` / `list_files` triggers a hard stop and the pipeline falls back to a zero-value RequestModel, so the budget is the real constraint even if you forget.\n\n")
	of.WriteString("## emit_analysis contract\n\n")
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
	of.WriteString("Entities: CamelCase/snake_case symbol names copied VERBATIM from the user's wording. Do NOT translate, re-case, pluralise, or paraphrase. Generic nouns (count, function, thing, agent, handler, module) MUST NOT appear here — they poison ERM ranking. Leave empty only when the question has no identifier-looking tokens. The pre-scan confirms whether these entities exist; presence in the repo is not a filter, just a sanity check.\n\n")
	of.WriteString("Keyword generation (example approach — adapt to the question's complexity): (1) Core — extract every domain noun and verb from the question in both original and identifier forms (e.g. CamelCase, snake_case). (2) Compound — cross-combine core terms into plausible multi-word identifiers (e.g. CacheStore, store_config). (3) Synonyms — for each verb add 2-3 programming synonyms (e.g. send → emit/dispatch/publish). Target ≥8 diverse stems; simple questions may need fewer rounds, complex ones may need more. For Chinese questions include BOTH Chinese and English forms (the codebase is English). The system auto-expands each keyword into case variants, so produce diverse STEMS rather than repeating words. The pre-scan is a good place to validate at least a handful of these stems appear in the repo, so downstream search time is not wasted on terms that never match.\n\n")
	of.WriteString("After emit_analysis succeeds, you may add a one-paragraph rationale for the trace log. This text is captured but does not drive any agent — the structured fields are what matter.")

	return &Config{
		Name: "analysis-skill",
		Goal: "Classify the user request into a RequestModel (intent, scenario, complexity, keywords, entities, question_kind, answer_shape). Use 1-2 rounds of evidence-lite pre-scan (repo_map, grep files_only=true, list_files) to verify entities/terms exist in the repo, then call emit_analysis exactly once.",
		Workflow: []string{
			"read the user input and detect its language",
			"round 1 pre-scan: call repo_map and/or grep(files_only=true) to check whether the entities from the user's wording exist in the repo and which files they live in",
			"round 2 pre-scan (optional): broaden the search if round 1 was empty on a key symbol — strip camelCase, try stems, drop qualifiers — or use list_files on a relevant directory",
			"pick intent from the intent enum",
			"pick scenario from the scenario enum",
			"pick complexity from the complexity enum (use the \"how many files\" hints)",
			"extract entities VERBATIM from the user's text — CamelCase/snake_case only, no generic nouns",
			"generate ≥8 keywords (e.g. core terms, compound identifiers, action synonyms — adapt rounds to complexity)",
			"pick question_kind and answer_shape from their enums",
			"call emit_analysis EXACTLY ONCE with the classified fields",
		},
		ToolSuggestions: append([]string(nil), AnalysisToolSuggestions...),
		OutputFormat:    of.String(),
		Prohibitions:    append([]string(nil), AnalysisHardRules...),
	}
}
