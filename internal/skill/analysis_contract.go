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
	{string(types.IntentRootCause), "user is debugging: asks \"why does X fail\" — OR pasted a runtime log excerpt (panic, exception trace, sanitizer diagnostic, traceback) and wants the code location / cause of the failure"},
	{string(types.IntentTrace), "follow a data flow or call chain end to end"},
	{string(types.IntentEnumerate), "list every X matching a predicate — the answer is a SET of names (\"list all X that do Y\", \"X matching pattern Y\"). Do NOT pick this when the user wants a count/size/total; that is return_value."},
	{string(types.IntentConfigQuery), "look up what a config key controls"},
	{string(types.IntentReturnValue), "asks for a single scalar answer: a function return, a literal name, a count / size / total / version number (\"how many X\", \"size of Y\", \"what does X return\"). One value, not a list."},
	{string(types.IntentUnknown), "genuinely ambiguous — the system's deterministic fallback will decide"},
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
//
// Descriptions carry TWO signals so the LLM can pick a level at
// analyze-time — before any file is read. File-count estimates alone
// are brittle (the LLM guesses) so each level also lists the
// question-shape cues that reliably predict investigation breadth.
// Language-neutral: cues are semantic patterns, not syntax.
var analysisComplexities = []AnalysisEnumChoice{
	{string(types.ComplexitySimple), "1 entity, 1-2 files. Question-shape cues: \"what is X\", \"where is X defined\", \"does X exist\", \"X 是什么\", \"X 在哪定义\", literal lookups, single-symbol return/boolean queries."},
	{string(types.ComplexityModerate), "1 entity / 1 component, 3-5 files. Question-shape cues: \"how does X work\", \"what does X do\", \"explain X\", \"X 怎么工作\", \"X 的作用\", single-component mechanism/explanation questions with no cross-system comparison."},
	{string(types.ComplexityComplex), "2+ entities OR cross-component reasoning, 6+ files. Question-shape cues: \"compare A and B\", \"how does X affect Y\", \"trace flow from A to B across M and N\", \"对比 A 和 B\", \"从 A 到 B 如何传递\", multi-symbol diffs, control-flow / dataflow spanning 2+ components."},
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
	{string(types.ShapeListOfSymbols), "answer is a set of identifier names found in the investigation's evidence"},
	{string(types.ShapeStepList), "ordered steps of a mechanism"},
	{string(types.ShapeValue), "a single literal / returned value"},
	{string(types.ShapeBoolean), "yes/no"},
	{string(types.ShapeConfigValue), "a resolved config key value"},
	{string(types.ShapeExplanation), "long prose explanation"},
	{string(types.ShapeNone), "no structured shape applies"},
}

// analysisAnswerSubjects is the canonical answer_subject.kind enum.
// Values match types.AnswerSubjectKind constants. Used by the chain
// ranker (subject-aware scoring), shape reconciler (config_value →
// value when subject is a source-code literal), and retry-hint
// renderer (rebind_subject directive). Zero-value (empty string =
// SubjectUnknown) is the explicit "I don't know" sentinel — the
// analyzer agent SHOULD pick a kind when the question has a clear
// answer-literal type, but missing the field is non-fatal because
// inferAnswerSubject in analyzer_intent.go provides a deterministic
// fallback from the cue list and question_kind enum.
var analysisAnswerSubjects = []AnalysisEnumChoice{
	{string(types.SubjectFunctionName), "answer is a function or method name"},
	{string(types.SubjectTypeName), "answer is a type / struct / class / enum name"},
	{string(types.SubjectInterface), "answer is an interface / trait name"},
	{string(types.SubjectHandlerRoute), "answer is an HTTP route or handler path"},
	{string(types.SubjectConfigKey), "answer is a YAML/JSON/TOML config key"},
	{string(types.SubjectReturnValue), "answer is what a function returns (literal or symbol)"},
	{string(types.SubjectFilePath), "answer is a repo-relative file path"},
	{string(types.SubjectStringLiteral), "answer is a quoted string constant"},
	{string(types.SubjectNumeric), "answer is a number / count / size"},
	{string(types.SubjectEnumValue), "answer is an enum constant / ALL_CAPS name"},
	{string(types.SubjectStructField), "answer is a struct/object field name"},
	{string(types.SubjectGeneric), "answer is heterogeneous (e.g. enumeration result)"},
	{string(types.SubjectUnknown), "no clear answer-literal kind — let the system infer"},
}

// AnalysisAnswerSubjectChoices returns the canonical answer_subject.kind enum table.
func AnalysisAnswerSubjectChoices() []AnalysisEnumChoice { return analysisAnswerSubjects }

// AnalysisAnswerSubjectValues returns the answer_subject.kind enum values
// in canonical order. The emit_analysis JSON schema reads this slice.
func AnalysisAnswerSubjectValues() []string { return enumValues(analysisAnswerSubjects) }

// analysisPredicateAxes is the canonical predicate_axis enum. Values
// match types.PredicateAxis constants. The LLM emits this directly in
// emit_analysis.predicate_axis (schema v4) — no more prose verb
// matching in Go code. Empty string means "no clear verb cue", which
// disables the axis-aware evidence-ranker boost.
var analysisPredicateAxes = []AnalysisEnumChoice{
	{string(types.AxisCall), "question is about who CALLs / invokes something"},
	{string(types.AxisRegister), "question is about REGISTRATION / binding / wiring"},
	{string(types.AxisDefine), "question is about DEFINITIONS / declarations"},
	{string(types.AxisReturn), "question is about RETURN values / outputs"},
	{string(types.AxisConfigure), "question is about CONFIGURATION / settings"},
	{string(types.AxisCondition), "question is about CONDITIONS / when something fires"},
	{string(types.AxisImplement), "question is about IMPLEMENTATION of an interface / contract"},
	{string(types.AxisUnknown), "no clear verb cue in the question"},
}

// AnalysisPredicateAxisChoices returns the canonical predicate_axis enum table.
func AnalysisPredicateAxisChoices() []AnalysisEnumChoice { return analysisPredicateAxes }

// AnalysisPredicateAxisValues returns the predicate_axis enum values
// in canonical order. The emit_analysis JSON schema reads this slice.
func AnalysisPredicateAxisValues() []string { return enumValues(analysisPredicateAxes) }

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
// The pre-scan workflow (Round 1 / Round 2 / the ClassificationGrep
// exception) lives in the Workflow slice in BuildAnalysisSkill, not
// here — duplicating it with absolute prohibitions used to create
// contradictions (Prohibitions said "ALWAYS files_only=true" while
// the Round 2 ClassificationGrep explicitly permits files_only=false
// for declarative-file verification). Only the one genuinely
// absolute rule survives here: no content-reading tools ever.
var AnalysisHardRules = []string{
	"every field in emit_analysis is REQUIRED (keywords and entities may be empty arrays); missing required fields rejects the call",
	"entities come from the user's ORIGINAL text only — \"ContinuationPrompt\" stays as \"ContinuationPrompt\", not \"continuation prompt\" or \"continuation_prompt\"",
	"do not invent an intent by stretching a category; if two fit equally, pick the one that matches the user's verb; if none fit, use \"unknown\"",
	"answer_shape=list_of_symbols ONLY when the user asks for the NAMES of items in a SET (\"list all X that match Y\"); if the user asks for a COUNT / SIZE / TOTAL (\"how many X\", \"统计…数量\", \"total of Y\"), the shape is value and the intent is return_value — a scalar cannot satisfy the list_of_symbols shape contract; \"is X registered\" is boolean; \"explain X\" is step_list or explanation",
	"call emit_analysis EXACTLY ONCE — multiple calls trigger a warning (or a hard reject when analysis_reject_multiple_emit=true) and only the last write is effective",
	"do not defer emit_analysis by writing open-ended analysis prose — the moment you have enough information to classify, call the tool; brief reasoning paired with pre-scan tool calls is fine, a standalone \"let me think about this\" paragraph is not",
	"do not translate or re-case entities — copy them verbatim from the user's text",
	"do not make assumptions about code structure — classify only what the user's text plus the pre-scan results support",
	"do NOT call read_file, exec_command, or any tool that reads file CONTENT — the analyze stage is existence + location verification only; the explore stage owns deep reading",
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

	// OutputFormat is the emit_analysis output contract — what the LLM
	// produces, not what it does. The workflow (how to pre-scan, when
	// Round 2 is allowed, which tools are forbidden) lives in the
	// Workflow slice above so the two surfaces do not drift or
	// contradict each other, and so the reader can find pre-scan
	// guidance in one place.
	of.WriteString("## emit_analysis contract\n\n")
	of.WriteString("Call emit_analysis EXACTLY ONCE. The required fields are:\n")
	of.WriteString("- intent, scenario, complexity, question_kind, answer_shape — one enum value each (tables below).\n")
	of.WriteString("- keywords, entities — string arrays (may be empty).\n")
	of.WriteString("- intent_confidence, complexity_confidence, kind_confidence, shape_confidence — floats in [0.0, 1.0].\n")
	of.WriteString("- predicates — object with five required booleans (see Semantic predicates below).\n\n")
	of.WriteString("Optional fields: sub_topics (array), answer_subject (object), predicate_axis (enum), language.\n\n")
	of.WriteString("Everything downstream — the search plan, the evidence plan, the hypothesis set, the quality checks — is derived automatically from your input; do not provide them.\n\n")
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
	of.WriteString(renderEnumTable("answer_subject.kind", analysisAnswerSubjects))
	of.WriteString("\n")
	of.WriteString("answer_subject is OPTIONAL — it tells downstream stages WHAT KIND of source-code literal the answer should be (a skill name, an agent name, a config key, ...). When the question clearly resolves to one of the listed kinds, set kind explicitly so the chain ranker can demote chains whose terminal token is the wrong kind. When unsure, leave the field unset; the system has a deterministic fallback that infers from question_kind. entity_axes is a short array describing the relational shape (e.g. [\"agent → skill\"] for \"what skill does the explorer agent use\").\n\n")
	of.WriteString(renderEnumTable("predicate_axis", analysisPredicateAxes))
	of.WriteString("\n")
	of.WriteString("predicate_axis names the action verb of the question (\"how is X CALLed\" → call; \"how is X REGISTERed\" → register). Pick the value that matches the user's verb regardless of language; leave empty when no clear verb cue exists.\n\n")
	of.WriteString("## Confidence\n\n")
	of.WriteString("For intent / complexity / question_kind / answer_shape, also emit a confidence float in [0.0, 1.0]:\n")
	of.WriteString("- 0.9+ when the user's wording unambiguously dictates the value\n")
	of.WriteString("- 0.5-0.7 when a plausible alternative exists\n")
	of.WriteString("- below 0.5 when you are genuinely guessing\n\n")
	of.WriteString("## Semantic predicates (REQUIRED, all five fields)\n\n")
	of.WriteString("In `predicates`, judge the user's question along five language-neutral axes. Every field MUST be present and MUST be true OR false (no missing fields). Read the user's wording in whatever language they wrote it:\n")
	of.WriteString("- `is_scalar_answer`: true when the answer is a single scalar (a number, a literal, a path), not a set or sequence\n")
	of.WriteString("- `is_count_question`: true when the answer is a single number that must be computed by aggregating values across multiple source units (counting items, summing lines of code, summing file sizes, totalling bytes across a directory tree). Implies is_scalar_answer. False when the answer is a number that already exists as a single source-code literal (const declaration, default value, enum ordinal) — that case is is_scalar_answer=true without is_count_question\n")
	of.WriteString("- `is_cross_component`: true when the user is comparing or relating two distinct subsystems / components / types\n")
	of.WriteString("- `is_relational_lookup`: true when the user is filtering set X by a relationship to Y ('functions that return Z', 'agents that use skill Y')\n")
	of.WriteString("- `is_category_enumeration`: true when the user is asking 'what kinds / types / categories of X exist'\n\n")
	of.WriteString("These predicates replace the system's old prose-keyword tables. Be honest — `false` is a valid answer. The system uses these to pick the right downstream behaviour; a wrong predicate produces a wrong answer downstream.\n\n")
	of.WriteString("Entities: CamelCase/snake_case symbol names copied VERBATIM from the user's wording. Do NOT translate, re-case, pluralise, or paraphrase. Generic nouns (count, function, thing, agent, handler, module) MUST NOT appear here — they degrade the downstream evidence search. Leave empty only when the question has no identifier-looking tokens. The pre-scan confirms whether these entities exist; presence in the repo is not a filter, just a sanity check.\n\n")
	of.WriteString("IMPORTANT — disambiguate from Prior Conversation: if the current request relies on Prior Conversation to resolve a pronoun or demonstrative (\"它\", \"那个\", \"它们\", \"this\", \"them\"), extract the concrete identifier from Prior and write THAT identifier verbatim into the entities array. The analyzer is the only stage that sees Prior Conversation by default; downstream stages only see the fields you emit here, so any Prior-derived disambiguation MUST land in entities or the downstream stages will lose the subject.\n\n")
	of.WriteString("Keyword generation — target ≥8 diverse stems. For each concept, generate multiple variants:\n")
	of.WriteString("- Word roots and inflections (e.g. send/sending/sent)\n")
	of.WriteString("- Synonyms (e.g. send → emit, dispatch, publish, write)\n")
	of.WriteString("- Antonyms when relevant (e.g. lock → unlock, start → stop)\n")
	of.WriteString("- Abbreviations and full forms (e.g. config → configuration, ctx → context)\n")
	of.WriteString("- CamelCase and snake_case identifiers (e.g. getUser, get_user)\n")
	of.WriteString("- Compound identifiers that cross-combine core terms\n")
	of.WriteString("For non-English questions, include BOTH the original language AND English programming equivalents. The system auto-expands each keyword into case variants, so produce diverse STEMS rather than repeating the same word. Validate a handful of stems via pre-scan grep to avoid wasting downstream search on non-existent terms.\n\n")
	of.WriteString("## Sub-topic detection (sub_topics field)\n\n")
	of.WriteString("When the user's question contains multiple independently-answerable sub-topics, list each in sub_topics. Rules:\n")
	of.WriteString("- Each sub_topic has a one-sentence summary and its own entities\n")
	of.WriteString("- Do NOT split topics that depend on each other (e.g. \"X是什么，它怎么影响Y\" → one topic)\n")
	of.WriteString("- DO split genuinely independent questions (e.g. \"快速排序的平均时间复杂度是多少？它是稳定排序吗？\" → two topics — one asks for a complexity class, the other asks for a boolean property)\n")
	of.WriteString("- When sub_topics is non-empty, answer_shape MUST be explanation\n")
	of.WriteString("- When unsure, do NOT split (empty array is safe)\n")
	of.WriteString("- Maximum 5 sub-topics\n\n")
	of.WriteString("After emit_analysis succeeds, you may add a one-paragraph rationale for the trace log. This text is captured but does not drive any agent — the structured fields are what matter.")

	return &Config{
		Name: "analysis-skill",
		Goal: "You are a CLASSIFIER, not an investigator. Classify the user request along seven fields (intent, scenario, complexity, keywords, entities, question_kind, answer_shape), then call emit_analysis exactly once. The explorer stage does the actual investigation — your job is only to verify entity existence and classify.",
		Workflow: []string{
			"Detect the request's language.",
			"Round 1 pre-scan: in a single response, issue parallel calls to repo_map and grep(files_only=true) for each candidate entity. A 'round' is one LLM response — multiple tool calls per response are normal and expected.",
			"If Round 1 resolved every entity, call emit_analysis now — skip Round 2.",
			"Round 2 is allowed at most once, when Round 1 ended ambiguous on either signal: (a) a key entity came back empty → broaden keyword stems / variants (still files_only=true); or (b) classification remains uncertain AND Round 1 surfaced a declarative file (e.g. a name matching topology / defaults / registry / routes / wire / init / manifest / schema / enum patterns) → a single response MAY include up to 3 grep(files_only=false, max_count=20) calls targeting those declarative files, to peek at the literal forms inside map/struct/enum bodies. Then call emit_analysis regardless of the Round 2 result.",
			"If the request spans multiple independent sub-topics, fill sub_topics (at most 5) and set answer_shape=explanation.",
			"Call emit_analysis exactly once. Required fields: intent, scenario, complexity, keywords, entities, question_kind, answer_shape, the four confidence floats (intent_confidence, complexity_confidence, kind_confidence, shape_confidence), and the predicates object (five booleans: is_scalar_answer, is_count_question, is_cross_component, is_relational_lookup, is_category_enumeration). Optional: sub_topics, answer_subject, predicate_axis, language.",
		},
		ToolSuggestions: append([]string(nil), AnalysisToolSuggestions...),
		OutputFormat:    of.String(),
		Prohibitions:    append([]string(nil), AnalysisHardRules...),
	}
}
