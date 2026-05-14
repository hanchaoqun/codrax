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
	{string(types.IntentExplain), "the user wants to understand how something works — the answer describes mechanism or behaviour"},
	{string(types.IntentRootCause), "the user is debugging a failure and wants its cause, OR has attached a runtime log (panic / exception trace / sanitizer diagnostic / traceback) and wants the code location responsible"},
	{string(types.IntentTrace), "the user wants a data flow or call chain followed end to end"},
	{string(types.IntentEnumerate), "the answer is a set of distinct named items, each independently valid — mutually exclusive with return_value (see predicates.is_count_question for the scalar/set boundary)"},
	{string(types.IntentConfigQuery), "the subject is a configuration key and the answer describes what the key controls"},
	{string(types.IntentReturnValue), "the answer is a single scalar: a count, a size, a function return, a literal value, a version number — one value, not a set"},
	{string(types.IntentUnknown), "genuinely ambiguous — leave for an automatic fallback to decide"},
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
// types.Complexity constants. Descriptions describe the structural
// shape of the subject (entity count + component count + file-count
// estimate) rather than the surface wording of the question — the
// latter was a keyword-matching prompt hint that fought schema
// predicates when the two disagreed.
var analysisComplexities = []AnalysisEnumChoice{
	{string(types.ComplexitySimple), "single entity, single component, 1-2 files; a literal lookup or single-symbol return / boolean query that does not cross subsystems"},
	{string(types.ComplexityModerate), "single entity or single component, 3-5 files; a mechanism / explanation question contained within one subsystem, no cross-component comparison"},
	{string(types.ComplexityComplex), "two or more entities OR cross-component reasoning, 6+ files; control flow or dataflow spanning two or more subsystems, or a comparison between distinct entities (predicates.is_cross_component is typically true here)"},
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
	types.ReqRegistration:  "the subject is a registration / binding relationship between two entities (which X registers / binds / wires Y)",
	types.ReqMechanism:     "the subject is the internal mechanism of a named component (how it works, how it is implemented)",
	types.ReqReturnValue:   "the subject is the return value of a named function / method / expression",
	types.ReqHistory:       "the subject is repository history / authorship metadata about a named path or symbol (which commit introduced it, when it was added, who changed it)",
	types.ReqConditional:   "the subject is the guard or condition under which a named behaviour fires",
	types.ReqConfigMapping: "the subject is a configuration key and the answer describes what the key controls",
	types.ReqEnumeration:   "the subject is a set of distinct named items; the answer enumerates the set",
	types.ReqCallChain:     "the subject is a caller → callee relationship between two named entities",
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
	{string(types.SubjectUnknown), "no clear answer-literal kind — leave for deterministic inference"},
}

// AnalysisAnswerSubjectChoices returns the canonical answer_subject.kind enum table.
func AnalysisAnswerSubjectChoices() []AnalysisEnumChoice { return analysisAnswerSubjects }

// AnalysisAnswerSubjectValues returns the answer_subject.kind enum values
// in canonical order. The emit_analysis JSON schema reads this slice.
func AnalysisAnswerSubjectValues() []string { return enumValues(analysisAnswerSubjects) }

// analysisDiagramKinds is the canonical diagram_hint.kind enum. The
// analyzer LLM may suggest a diagram family, but the deterministic
// compiler still derives the final DiagramContract from stronger
// structural signals (shape / intent / predicate_axis / log-triage /
// cross-component predicates). Omit the field when there is no clear
// diagram preference.
var analysisDiagramKinds = []AnalysisEnumChoice{
	{string(types.DiagramCallDAG), "call chain / dispatch / fan-out / root-to-callee structure"},
	{string(types.DiagramFlow), "flow / branch / guard / fallback / retry logic"},
	{string(types.DiagramSequence), "actor-to-actor temporal ordering over time"},
	{string(types.DiagramArchitecture), "component / layer / subsystem relationship view"},
}

var analysisExactContextRoles = []AnalysisEnumChoice{
	{string(types.EvidenceDiagramRoleDefault), "code-default / compiled-in baseline layer"},
	{string(types.EvidenceDiagramRoleConfig), "repo or user config-file layer (YAML / JSON / TOML / INI / properties / env-like config files)"},
	{string(types.EvidenceDiagramRoleRuntime), "runtime-resolved struct / in-memory config layer"},
	{string(types.EvidenceDiagramRoleOverride), "override channel such as CLI / env / explicit runtime override"},
}

// AnalysisDiagramKindChoices returns the canonical diagram_hint.kind enum table.
func AnalysisDiagramKindChoices() []AnalysisEnumChoice { return analysisDiagramKinds }

// AnalysisDiagramKindValues returns the diagram_hint.kind enum values
// in canonical order. The emit_analysis JSON schema reads this slice.
func AnalysisDiagramKindValues() []string { return enumValues(analysisDiagramKinds) }

func AnalysisExactContextRoleValues() []string { return enumValues(analysisExactContextRoles) }

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

// AnalysisIntentValues returns the intent enum values in canonical
// order. This is the slice the emit_analysis JSON schema reads.
func AnalysisIntentValues() []string { return enumValues(analysisIntents) }

// AnalysisScenarioValues returns the scenario enum values in canonical order.
func AnalysisScenarioValues() []string { return enumValues(analysisScenarios) }

// AnalysisComplexityValues returns the complexity enum values in canonical order.
func AnalysisComplexityValues() []string { return enumValues(analysisComplexities) }

// AnalysisQuestionKindValues returns the question_kind enum values in canonical order.
func AnalysisQuestionKindValues() []string { return enumValues(analysisQuestionKinds) }

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
// The pre-scan workflow lives in the Workflow slice in BuildAnalysisSkill.
// Keep the files_only=true grep boundary explicit here too because the
// runtime gate enforces it before dispatching the tool.
var AnalysisHardRules = []string{
	"every field in emit_analysis is REQUIRED (keywords and entities may be empty arrays); missing required fields rejects the call",
	"entities come from the user's ORIGINAL text only — \"ContinuationPrompt\" stays as \"ContinuationPrompt\", not \"continuation prompt\" or \"continuation_prompt\"",
	"do not invent an intent by stretching a category; if two fit equally, pick the one that matches the user's verb; if none fit, use \"unknown\"",
	"call emit_analysis EXACTLY ONCE — multiple calls trigger a warning (or a hard reject when analysis_reject_multiple_emit=true) and only the last write is effective",
	"do not defer emit_analysis by writing open-ended analysis prose — the moment you have enough information to classify, call the tool; brief reasoning paired with pre-scan tool calls is fine, a standalone \"let me think about this\" paragraph is not",
	"do not translate or re-case entities — copy them verbatim from the user's text",
	"do not make assumptions about code structure — classify only what the user's text plus the pre-scan results support",
	"when calling grep in analyze, ALWAYS pass files_only=true — line-level matches are exploration evidence and are rejected at runtime in this stage",
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
	// produces, not what it does. The workflow (how to pre-scan and which
	// tools are forbidden) lives in the Workflow slice above so the two
	// surfaces do not drift or contradict each other, and so the reader can
	// find pre-scan guidance in one place.
	of.WriteString("## emit_analysis contract\n\n")
	of.WriteString("Call emit_analysis EXACTLY ONCE. The required fields are:\n")
	of.WriteString("- intent, scenario, complexity, question_kind — one enum value each (tables below).\n")
	of.WriteString("- keywords, entities — string arrays (may be empty).\n")
	of.WriteString("- intent_confidence, complexity_confidence, kind_confidence — floats in [0.0, 1.0].\n")
	of.WriteString("- predicates — object with eight required booleans (see Semantic predicates below).\n\n")
	of.WriteString("- diagnostic_profile — object with four required booleans plus confidence (see Diagnostic profile below).\n\n")
	of.WriteString("- answer_role_profile — object with is_role_binding_requested plus confidence; set false when no positive answer-row role is requested (see Answer role profile below).\n\n")
	of.WriteString("- error_granularity_profile — object with is_granularity_question plus confidence; set false when no failure-scope verdict is requested (see Error granularity profile below).\n\n")
	of.WriteString("Optional fields: sub_topics (array), answer_subject (object), predicate_axis (enum), diagram_hint (object), enumeration_boundary (object), completeness_obligation (object), buckets (array), exact_targets (array), exact_context_terms (array), exact_context_roles (array), source_scope_profile (object), change_impact_profile (object), field_value_profile (object), answer_exclusion_policy (object), language, required_files (array), irrelevant_files (array).\n\n")
	of.WriteString("Everything downstream — the search plan, the evidence plan, the hypothesis set, the quality checks — is derived automatically from your input; do not provide them.\n\n")
	// Top-level "current-question primacy" rule (2026-05-10 Issue B).
	// Generalises the per-field cross-turn discipline rules below
	// (sub_topics line ~387, pronoun-disambiguation line ~370) so the
	// LLM cannot interpret the per-field rules as opt-in. EVERY intent
	// field is current-question only; Prior Conversation participates
	// ONLY for pronoun / demonstrative resolution.
	of.WriteString("## Current-question primacy (every intent field)\n\n")
	of.WriteString("EVERY field you emit — intent, scenario, complexity, question_kind, intent_confidence, complexity_confidence, kind_confidence, keywords, entities, predicates (every flag), diagnostic_profile (every flag), " +
		"sub_topics, answer_subject, predicate_axis, exact_targets, exact_context_terms, exact_context_roles, source_scope_profile, change_impact_profile, field_value_profile, answer_exclusion_policy, answer_role_profile, error_granularity_profile, " +
		"diagram_hint, language, completeness_obligation, enumeration_boundary, buckets, required_files, irrelevant_files — MUST be derived from the CURRENT request's text only. " +
		"The CURRENT request is the user-typed input that drove this dispatch. " +
		"Prior Conversation entries (visible in the \"Prior Conversation\" section above when present), agent memory recalled by `recall_memory`, and any other historical material " +
		"are REFERENCE-ONLY context — they MUST NOT be the primary basis for any field's value.\n\n")
	of.WriteString("The single exception: when the current request leans on a pronoun / demonstrative (\"它\", \"那个\", \"它们\", \"this\", \"that\", \"them\", \"上次\", \"刚才\", \"the same\", \"continue with\") " +
		"that has no antecedent inside the current request itself, you MAY consult Prior Conversation to resolve the antecedent and copy the resolved CONCRETE identifier verbatim into the relevant field (most often `entities`). " +
		"This is disambiguation, not lifting: only the resolved name crosses over, never the framing / topic / structural shape of the prior turn.\n\n")
	of.WriteString("Lifting any of the following from Prior Conversation / memory / historical material is a STRUCTURAL ERROR — the answer the user actually receives is computed from the CURRENT request:\n" +
		"- A topic / framing / sub-topic from a prior turn (an earlier UI tweak, layout change, performance discussion, design debate)\n" +
		"- A predicate flag value (`is_cross_component`, `is_category_enumeration`, etc.) inferred from prior-turn shape\n" +
		"- An entity name that appeared in a prior turn but is NOT named or pronoun-referenced by the current request\n" +
		"- A predicate axis / answer subject / diagram kind shaped by what the prior turn was asking, not by what the current request asks\n" +
		"- A sub-repo scope or PrimaryEntity inferred from prior-turn focus rather than the current request\n\n")
	of.WriteString("Runtime artifacts do not change that rule. If the current request asks for diagnosis, treat symbols and error names found only inside an attached log / trace as evidence tokens for that diagnostic answer, not as the user's requested architecture subject. If the current request asks how a log / trace feature itself is implemented, then classify the feature mechanism normally and keep `is_diagnostic_question=false` even if an artifact is attached.\n\n")
	of.WriteString("If the current request is fully self-contained (no pronoun / demonstrative needing resolution), Prior Conversation has NO bearing on any field. Read the current request, classify from its text, and emit. " +
		"Treat memory-recalled material the same way: REFERENCE for understanding the user's broader context, never a source of intent fields for THIS dispatch.\n\n")
	of.WriteString("Field enums:\n\n")
	of.WriteString(renderEnumTable("intent", analysisIntents))
	of.WriteString("\n")
	of.WriteString(renderEnumTable("scenario", analysisScenarios))
	of.WriteString("\n")
	of.WriteString(renderEnumTable("complexity", analysisComplexities))
	of.WriteString("\n")
	of.WriteString(renderEnumTable("question_kind", analysisQuestionKinds))
	of.WriteString("\n")
	of.WriteString(renderEnumTable("answer_subject.kind", analysisAnswerSubjects))
	of.WriteString("\n")
	of.WriteString("answer_subject is OPTIONAL in general, but for define-axis single-target questions you should usually set it explicitly — that is how the rest of the pipeline tells apart \"explain this named entity\" from \"locate the function / file / route / config key that plays this role\". It declares WHAT KIND of source-code literal the answer should be (a skill name, an agent name, a config key, ...). When the question clearly resolves to one of the listed kinds, set kind explicitly so the chain ranker can demote chains whose terminal token is the wrong kind. When unsure, leave the field unset; an automatic fallback infers from question_kind. entity_axes is a short array describing the relational shape (e.g. [\"component → role\"] for \"what role does component X play in module Y\").\n\n")
	of.WriteString("**Narrative-answer guard**: distinguish the SUBJECT the user names (a function, an error, a field, a config value, a route, a runtime symptom...) from the ANSWER the user wants. When the user asks for a multi-step explanation — causation, mechanism, sequence, layering, decomposition, walk-through, comparison — the answer is the explanation itself, NOT any single name they happened to mention. In those cases do NOT set `answer_subject.kind` to a scalar value (`string_literal`, `numeric`, `return_value`). Either leave the field unset (preferred — `question_kind` already classifies the answer shape), or pick a non-scalar kind only when the question literally asks for ONE named target (e.g. 'which function does X' → `function_name`; 'which config key controls Y' → `config_key`). The system rejects scalar kinds for explanation-shape answers and wastes a retry round otherwise.\n\n")
	of.WriteString("**Config-vs-code qualification guard**: separators do not decide the subject kind. A dotted / namespace-qualified / module-qualified surface may be a code symbol (`package.function`, `Type.method`, `Namespace::method`, `Module.member`) or a real config key (`section.option`). Use `intent=config_query`, `scenario=config_trace`, `question_kind=config_mapping`, `predicate_axis=configure`, and `answer_subject.kind=config_key` only when the user is asking about a configuration key / precedence / override / effective value. When the user asks to compare, explain, trace, or measure named code functions/templates/types/methods, classify by that answer surface (`mechanism`, `call_chain`, `return_value`, `architecture_explain`, buckets for comparisons) even if the code names contain dots or namespace separators.\n\n")
	of.WriteString(renderEnumTable("predicate_axis", analysisPredicateAxes))
	of.WriteString("\n")
	of.WriteString("predicate_axis names the action verb of the question (\"how is X CALLed\" → call; \"how is X REGISTERed\" → register). Pick the value that matches the user's verb regardless of language; leave empty when no clear verb cue exists.\n\n")
	of.WriteString(renderEnumTable("diagram_hint.kind", analysisDiagramKinds))
	of.WriteString("\n")
	of.WriteString("diagram_hint is OPTIONAL. Emit it only when the CURRENT request explicitly asks for a diagram / drawing / visual representation, or when the answer would be structurally incomplete without a visual. Use `call_dag` for call chains / dispatch / fan-out, `flow` for branches / guards / fallback / retry, `sequence` for actor-to-actor ordering over time, `architecture` for component / layer / subsystem relationships. Omit it for ordinary code, log, or trace questions where prose/list/table blocks directly answer the user.\n\n")
	of.WriteString("exact_targets is OPTIONAL. Use it when the user is asking about a SPECIFIC named target but the request also mentions nearby context items (for example, a config key plus neighboring files / layers / defaults). Put ONLY the exact user-asked target(s) here, copied verbatim from the current request text. Omit the field when unsure. The system validates every item against the current request before turning it into an exact-resolution contract.\n\n")
	of.WriteString("exact_context_terms is OPTIONAL. Leave it unset by default. Use it only alongside exact_targets when nearby context truly needs one narrow identifier family / module scope, and only when you can copy 1-2 stems directly from the exact target lane itself. Example: if the exact target is a multi-segment snake_case identifier (`<family>_<sub>_<sub>`), a good term is the first / family-naming segment; if the exact target is a path under `<repo>/<package>/...`, good terms are path segments or identifier stems that come from that exact target lane. Do NOT invent new family names, layer labels, precedence words, or generic context terms. The system validates every term against the request-mentioned exact-target lane and silently drops invalid ones downstream.\n\n")
	of.WriteString(renderEnumTable("exact_context_roles[]", analysisExactContextRoles))
	of.WriteString("\n")
	of.WriteString("exact_context_roles is OPTIONAL. Use it only alongside exact_targets when the user explicitly asks for precedence / lineage layers of a config-like exact target (for example code default vs config file vs CLI/env override). Emit only the abstract roles the user actually asked about, using the enum values above. Do not guess file names or repo-specific layer names here; the rendering maps grounded evidence onto these abstract roles automatically.\n\n")
	of.WriteString("## Confidence\n\n")
	of.WriteString("For intent / complexity / question_kind, also emit a confidence float in [0.0, 1.0]:\n")
	of.WriteString("- 0.9+ when the user's wording unambiguously dictates the value\n")
	of.WriteString("- 0.5-0.7 when a plausible alternative exists\n")
	of.WriteString("- below 0.5 when you are genuinely guessing\n\n")
	of.WriteString("## Semantic predicates (REQUIRED, all eight fields)\n\n")
	of.WriteString("In `predicates`, judge the user's question along eight language-neutral axes. Every field MUST be present and MUST be true OR false (no missing fields). Read the user's wording in whatever language they wrote it:\n")
	of.WriteString("- `is_scalar_answer`: true when the answer is a single scalar (a number, a literal, a path), not a set or sequence\n")
	of.WriteString("- `is_role_locate_lookup`: true when the request names a clue / output / context entity, but the answer is a DIFFERENT single literal that plays a role relative to that clue (entry function, defining file, route, config key, owner symbol, etc.). When this is true, also set `is_scalar_answer=true` and set `answer_subject.kind` explicitly\n")
	of.WriteString("- `is_count_question`: true when the answer is a single number that must be computed by aggregating values across multiple source units (counting items, summing lines of code, summing file sizes, totalling bytes across a directory tree). Implies is_scalar_answer. False when the answer is a number that already exists as a single source-code literal (const declaration, default value, enum ordinal) — that case is is_scalar_answer=true without is_count_question\n")
	of.WriteString("  - Long-form-explanation carve-out: a count question may carry a long-form explanation surface (the derivation of the count, the source units it iterates) alongside its numeric answer. Set BOTH `is_count_question=true` AND `is_scalar_answer=true` even when the answer body explains how the count was derived. Without `is_count_question=true`, the long-form-explanation surface conflicts with a numeric subject and the question is rejected as a wrong shape (the system reads it as 'long-form prose pretending to be a single literal'). With `is_count_question=true` the combination is recognised as 'derived numeric scalar with explanation' and proceeds.\n")
	of.WriteString("- `is_cross_component`: true when the user is comparing or relating two distinct subsystems / components / types. Leave it false for a single ordered source-to-sink call/flow trace even if that one chain crosses files/packages, and false for a single exact target that merely needs nearby context / precedence layers\n")
	of.WriteString("- `is_relational_lookup`: true when the user is filtering set X by a relationship to Y ('functions that return Z', 'agents that use skill Y')\n")
	of.WriteString("- `is_category_enumeration`: true when the user is asking the system to enumerate a closed set defined by a structural relation. Three concrete shapes count as category enumeration: (a) 'what kinds / types / categories of X exist'; (b) 'list all implementers / implementations / subclasses of <interface | trait | protocol | abstract class>' (each implementer is one category) — same shape applies to overrides of an abstract method, conformers of a protocol, and registrations against a registry / dispatch table; (c) 'list all members / variants / cases of <enum | sum-type>'. The defining test: the answer set is closed (every member can in principle be enumerated from the source) AND each member is a NAMED entity, not a free-form description.\n")
	of.WriteString("  - Cardinality consistency — IMPORTANT: when you set `is_category_enumeration=true`, your `entities` field MUST list the actual ENUMERATED MEMBERS (e.g. each implementer name, each enum case name, each registered handler name), not the enclosing TYPE / INTERFACE / REGISTRY name alone. The enclosing type is the wrapper of the answer; the enumerated members ARE the answer. If you can only name the wrapper, set `is_category_enumeration=false` and let the question route as a regular type-name lookup. The downstream gate rejects `is_category_enumeration=true` paired with a single-entity emit because that combination is structurally self-contradictory (you cannot enumerate one thing).\n")
	of.WriteString("  - Relational-lookup carve-out: if the question asks you to filter a SET by a relationship to a named target (e.g. 'which packages import X', 'list files that depend on Y', 'list X's exports', 'enumerate Z's overrides', 'X 的子包' / 'X 下的子目录' / 'list APIs exported by X' / 'X 包导出哪些'), the entity legitimately IS the relation target — the actual values (importing packages / dependent files / exported symbols / overriding methods / child packages) are NOT yet known at classification time and will be discovered when the codebase is investigated. For this shape, set BOTH `is_category_enumeration=true` AND `is_relational_lookup=true`. The cardinality rule above is satisfied by the relational shape; your emit only needs to name the relation target. A single-entity emit with `is_category_enumeration=true` but `is_relational_lookup=false` reads as a categorical-wrapper mistake (you named the wrapper, not its contents); add `is_relational_lookup=true` to mark it as a relational lookup instead.\n")
	of.WriteString("- `is_history_lookup`: true when the literal answer should come from repository history / authorship metadata (git log / blame / commit history), not from a repo file:line\n")
	of.WriteString("- `is_diagnostic_question`: true when the CURRENT request asks to diagnose a failure, regression, runtime symptom, observed bad behaviour, or whether a similar problem still exists. This is about the answer the user wants: cause / current-risk / remediation analysis. It applies with an attached log, an attached trace, or no attachment at all. Leave it false for ordinary architecture explanations, code walkthroughs, parser/formatter mechanism questions, or questions about how a log/trace feature itself is implemented. When true, align `intent=root_cause`; use `scenario=root_cause` unless the diagnostic is specifically performance-hotspot oriented, in which case use `scenario=performance_bottleneck`.\n\n")
	of.WriteString("Attached runtime artifacts may include structured `observations[]` such as retry cycles, topical mismatch, line-mapping drift, contract/review failures, or state changes. Treat those observations as evidence about what happened in the artifact, not as the answer subject by themselves. If the CURRENT request asks whether such an observed issue still exists, set `is_diagnostic_question=true` and make the answer verify current code separately from the historical/artifact observation. If the CURRENT request instead asks how the log/trace feature or a named tool/type is implemented, keep `is_diagnostic_question=false` and classify the named implementation subject normally.\n\n")
	of.WriteString("These predicates are the structural intent signal — be honest, `false` is a valid answer. A wrong predicate produces a wrong answer because every later choice keys on these typed flags rather than re-reading the question text.\n\n")
	of.WriteString("## Diagnostic profile (REQUIRED)\n\n")
	of.WriteString("Emit `diagnostic_profile` on every call with fields `is_diagnostic`, `current_risk`, `historical_regression`, `current_version_check`, and `confidence`. Every boolean MUST be present and true OR false.\n")
	of.WriteString("- `is_diagnostic`: true when the CURRENT request expects diagnosis, cause, remediation, or risk analysis. Keep it aligned with `predicates.is_diagnostic_question`.\n")
	of.WriteString("- `current_risk`: true when the user asks whether a known / observed issue can still happen in the current checkout.\n")
	of.WriteString("- `historical_regression`: true when the user references a prior log / trace / previous run / earlier answer and asks whether the same class of problem remains or regressed.\n")
	of.WriteString("- `current_version_check`: true only for diagnostic current-status questions where the answer must verify still-present / fixed / not-enough-evidence against current code rather than only explaining the historical observation. It is NOT a generic 'read the current repo' flag; leave it false for ordinary exact/config/value/location/look-up questions even though those also require reading current code.\n")
	of.WriteString("- `observation_summary`: optional but strongly recommended for diagnostic/current-version checks. Write a compact description of the historical or user-described symptom, using the current request plus Prior Conversation when the user refers to \"that issue\" / \"the previous problem\" / equivalent. Leave empty only when there is no identifiable observed symptom.\n")
	of.WriteString("- `confidence`: your confidence in this profile in [0,1].\n\n")
	of.WriteString("If `is_diagnostic`, `current_risk`, or `historical_regression` is true, align `intent=root_cause`; use `scenario=root_cause` unless the diagnostic is specifically performance-hotspot oriented, in which case use `scenario=performance_bottleneck`. `current_version_check=true` must be paired with one of those diagnostic signals or `predicates.is_diagnostic_question=true`; using it alone is structurally invalid because current-code lookup is not the same as current-status diagnosis. Do NOT set these flags merely because a log or trace is attached: artifact presence is evidence context, not user intent. For current-version checks, the later answer must keep two lanes separate: historical/artifact observation and current-code verification.\n\n")
	of.WriteString("Entities: CamelCase/snake_case symbol names copied VERBATIM from the user's wording. Do NOT translate, re-case, pluralise, or paraphrase. Generic nouns (count, function, thing, agent, handler, module) MUST NOT appear here — they degrade the downstream evidence search. Leave empty only when the question has no identifier-looking tokens. The pre-scan confirms whether these entities exist; presence in the repo is not a filter, just a sanity check.\n\n")
	of.WriteString("IMPORTANT — disambiguate from Prior Conversation: if the current request relies on Prior Conversation to resolve a pronoun or demonstrative (\"它\", \"那个\", \"它们\", \"this\", \"them\"), extract the concrete identifier from Prior and write THAT identifier verbatim into the entities array. Prior Conversation is visible only at this point in the pipeline; later steps only see the fields you emit here, so any Prior-derived disambiguation MUST land in entities or the subject is lost.\n\n")
	of.WriteString("conversation_reference_profile is OPTIONAL. Emit it only when the CURRENT request relies on Prior Conversation to identify the subject and there is no attached log/trace observation being diagnosed. This is the typed provenance lane for ordinary follow-up questions. Set `requires_prior_context=true`, `needs_repo_verification=true` when repository reading is still needed, `ambiguity=none|ambiguous|missing`, and list `resolved_subjects` with `surface`, `kind`, `source=prior_context|mixed`, `role`, `use_as_exact_target`, and `confidence`. Set `use_as_exact_target=true` only when the prior-derived subject is the exact config key / file path / route / symbol / literal the answer must resolve. Keep `exact_targets` restricted to targets copied from the CURRENT request text; do not place prior-only subjects there.\n\n")
	of.WriteString("source_scope_profile is OPTIONAL. Emit it when the CURRENT request makes tests, docs, fixtures, examples, or all repo material part of the principal answer scope. Use `requested_scope=production` for explicit production-only requests, `test` for test files, `documentation` for docs, `auxiliary` for fixture/example/support material, `all` when both production and auxiliary material should be principal, and `unknown` only when the user's scope is genuinely unclear. Set `include_auxiliary_as_principal=true` only when auxiliary roles may be answer members, not merely supporting context. For ordinary code-behavior, architecture, config, relation, call-chain, scalar, and count questions that do not explicitly ask about auxiliary material, omit the profile or use production; later stages will still show auxiliary matches as context, not production proof.\n\n")
	of.WriteString("change_impact_profile is OPTIONAL. Emit it when the CURRENT request asks for the affected surface of a target change: which files, sites, symbols, APIs, config locations, build edges, generated artifacts, or downstream consumers would need updates if a named symbol/type/field/function/config key/module/API changed shape or contract. This profile is independent of diagnostics: use it for migration / compatibility / affected-site enumeration, not for root-cause current-status checks. Set `is_change_impact=true`, copy the changed target into `target`, choose `target_kind` when clear, set `scope` to production/test/all/unknown, set `requested_output` to files/sites/symbols/steps/unknown, and include every affected site role you expect to be principal in `affected_site_kinds` (definition, assignment, read, guard, call, return, import, construction, validation, serialization, config, generated, documentation, build). Direct assignments are only one possible affected site role; if readers, guards, validators, serializers, builders, import edges, or call adapters would also need updates, include those roles too. Leave the profile omitted for ordinary enumeration of existing members, mechanism explanation, scalar lookup, or call-chain trace.\n\n")
	of.WriteString("field_value_profile is OPTIONAL. Emit it when the CURRENT request explicitly binds a named field/member/config surface to an exact literal value, especially count/scalar questions such as how many production sites set `Owner.Field = false` or which sites set `server.port: 8080`. Set `is_field_value_lookup=true`, copy the owner-qualified target into `target`, copy the exact literal into `literal`, choose `literal_kind` (bool, nil, number, string, enum, or empty when genuinely unknown), and set `source_quote` to the verbatim phrase in the current request that contains both target and literal. This typed carrier is the only signal downstream hard gates use for field/value coverage. Do NOT rely on `keywords` such as true/false/nil/null, and do not emit the profile for unrelated counts like Top 3, first 5, line counts, or generic mechanism questions.\n\n")
	of.WriteString("answer_exclusion_policy is OPTIONAL. Emit it when the CURRENT request explicitly says a candidate category must be excluded from the principal answer surface, for example excluding variables, tests, generated files, private helpers, documentation, examples, fixtures, tool names, import paths, scalar role classes, or other role classes. Set `is_exclusion_requested=true`, put language-neutral enum values in `excluded_candidate_roles` (function, method, type, constant, variable, field, package, file, test, generated, private, documentation, example, fixture, helper, tool_name, config_key, route, import_path, literal_value, commit_hash, budget_cap, attempt_counter, guard_condition, other), and set `source_quotes[]` to verbatim phrase(s) in the current request that state the exclusion. This is an intent carrier only; downstream enforcement compares it with `emit_answer_document.blocks[].items[].candidate_role`. Do not infer exclusions from ordinary category words, and do not emit it for positive scope requests such as \"production only\" unless the user explicitly names a category to exclude.\n\n")
	of.WriteString("Emit `answer_role_profile` on every call with fields `is_role_binding_requested` and `confidence`. Set `is_role_binding_requested=false` when no positive answer-row role is requested. Set it true when the CURRENT request positively asks which role-bearing answer should win among nearby alternatives: examples of role classes include a configuration limit, attempt counter, guard condition, tool name, config key, route, import path, commit hash, or literal value. This profile names the REQUESTED ROLE CLASS, not the final identifier, so do not wait for exploration to prove the exact symbol before emitting it. Choose `required_candidate_roles` from the same candidate_role enum as answer_exclusion_policy by semantic role: `budget_cap` is the upper-bound or limit role that controls how many tries are allowed, `attempt_counter` is the observed or current try index role, `guard_condition` is the branch predicate role, and `tool_name` is the API/tool identifier role. When true, put the required answer-row enum value(s) in `required_candidate_roles`, and set `source_quotes[]` to verbatim phrase(s) in the current request that state the positive role binding. This is the complement of answer_exclusion_policy: exclusion says which row roles must stay out; answer_role_profile says which row roles must be present. Downstream enforcement compares only this typed profile with `emit_answer_document.blocks[].items[].candidate_role`; it must not infer the requested role from prose.\n\n")
	of.WriteString("Emit `error_granularity_profile` on every call with fields `is_granularity_question` and `confidence`. Set `is_granularity_question=false` when the CURRENT request does not ask for a canonical failure-scope verdict. Set it true when the request asks how an error, failure, or rejection is scoped across items, records, calls, batches, transactions, or sibling entries: whether one bad member is rejected while others continue, whether the whole batch or call fails, whether partial success is returned, whether processing stops at the first failure, or whether errors are collected for reporting. This profile only says the answer requires a canonical verdict after exploration; it does NOT predict the verdict in analyze. When true, set `source_quotes[]` to verbatim phrase(s) in the current request that state the failure-scope question. If the CURRENT request explicitly contrasts verdict alternatives, fill `requested_verdict_options[]` with the corresponding enum values from: `per_item_rejection` (bad item/record rejected while valid siblings continue), `whole_batch_failure` (one failure rejects the whole call/batch), `partial_success` (mixed success/failure result is itself the requested alternative), `fail_fast` (processing stops at first failure), and `collect_errors` (errors are accumulated for reporting). Do not put `not_enough_evidence` in requested_verdict_options; that value is reserved for the final answer when evidence cannot decide. When this profile is active and `predicates.is_count_question=false` plus `predicates.is_category_enumeration=false`, count-like phrases are scenario context, not an `enumeration_boundary`. Downstream enforcement compares only this typed profile with `emit_answer_document.blocks[].error_granularity_verdict`; it must not infer the verdict requirement from prose.\n\n")
	of.WriteString("Sub-repo scopes (multi-repo workspaces): when the workspace section above lists multiple sub-repos under \"Multi-Repo Active Set\", a sub-repo name (the slash-separated path shown in that list) is a legitimate `entities` value even if it is all-lowercase or otherwise outside the CamelCase / snake_case shape. The system recognises sub-repo names against the active list verbatim; cross-sub-repo questions (\"compare A and B\" / \"how does A do X / how does B do X\") should put each named sub-repo in `entities`, and each independent sub-topic that focuses on a single sub-repo should include that sub-repo's name in its own `sub_topics[].entities`. The structural-coherence check accepts sub-repo names anchored this way without requiring them to resolve as code symbols.\n\n")
	of.WriteString("Keyword generation — target ≥8 diverse stems. For each concept, generate multiple variants:\n")
	of.WriteString("- Word roots and inflections (e.g. send/sending/sent)\n")
	of.WriteString("- Synonyms (e.g. send → emit, dispatch, publish, write)\n")
	of.WriteString("- Antonyms when relevant (e.g. lock → unlock, start → stop)\n")
	of.WriteString("- Abbreviations and full forms (e.g. config → configuration, ctx → context)\n")
	of.WriteString("- CamelCase and snake_case identifiers (e.g. getUser, get_user)\n")
	of.WriteString("- Compound identifiers that cross-combine core terms\n")
	of.WriteString("For non-English questions, include BOTH the original language AND English programming equivalents. Each keyword is auto-expanded into case variants downstream, so produce diverse STEMS rather than repeating the same word. Validate a handful of stems via pre-scan grep to avoid wasting later search on non-existent terms.\n\n")
	of.WriteString("## Sub-topic detection (sub_topics field)\n\n")
	of.WriteString("When the user's question contains multiple independently-answerable sub-topics, list each in sub_topics. Rules:\n")
	of.WriteString("- Each sub_topic has a one-sentence summary and its own entities\n")
	of.WriteString("- Do NOT split topics when the second depends on the first (e.g. a definition followed by its downstream effects belongs to one topic, not two)\n")
	of.WriteString("- DO split when the topics are structurally independent – different answer subjects, different predicate axes, or different subsystems – and each could stand as its own separate question\n")
	of.WriteString("- When unsure, do NOT split (empty array is safe)\n")
	of.WriteString("- Maximum 5 sub-topics\n\n")
	of.WriteString("Cross-turn discipline: sub_topics MUST be extracted from the CURRENT question's text only. Prior conversation entries (visible in the \"Prior Conversation\" section above) are reference material for pronoun / demonstrative disambiguation in Chinese or English (\"它\" / \"那个\" / \"它们\" / \"this\" / \"that\" / \"them\") — they are NOT a source of sub-topic content. Lifting a prior turn's UI / formatting / framing topic (e.g. an earlier display-tweak discussion or a layout adjustment) into the current emit's sub_topics is a structural error: the answer the user actually receives is computed from the CURRENT question.\n\n")
	of.WriteString("Field names that appear inside structural-contradiction retry hints (e.g. \"is_cross_component\", \"sub_topics\", \"primary_entities\") describe the SHAPE of your previous emit_analysis call — they are NOT repository code identifiers and MUST NOT enter `entities` or `sub_topics[].entities`. Treat them as instructions for what to fix in the next emit, not as new things to grep.\n\n")
	of.WriteString("## Question structural obligations\n\n")
	of.WriteString("The user's question may carry one or more structural obligations that the answer must respect. Each axis is independent — emit each one that applies, omit those that don't. Source quotes are validated against the current request; a fabricated quote is dropped silently.\n\n")
	of.WriteString("### Count axis (`enumeration_boundary` field)\n\n")
	of.WriteString("When the user's request states an explicit count number that names or constrains the answer set, emit `enumeration_boundary` with:\n")
	of.WriteString("- `declared_count`: the same user-stated number\n")
	of.WriteString("- `source_quote`: the exact phrase copied verbatim from the current request text\n")
	of.WriteString("Trigger PRINCIPLES (apply, do not pattern-match — examples below are abstract shapes, not the answer):\n")
	of.WriteString("- (a) the count and a plural noun appear together AND that noun-phrase is what the question asks ABOUT — the count constrains how many items belong to the answer set. The question form (list / compare / order / explain how each works / what relationship between them) is irrelevant; what matters is whether the count quantifies the answer's enumerated members.\n")
	of.WriteString("- (b) the count is a fact the user states about the set being asked about, not a count of unrelated context. Test: if you removed the count from the question, would the user's intent change about how many items the answer should cover? If yes, the count is a boundary.\n")
	of.WriteString("Do NOT emit when:\n")
	of.WriteString("- the count appears in the question but names something OTHER than the answer set (e.g. a count of files searched, a count of error log lines, a version number, a line number)\n")
	of.WriteString("- the count is a vague approximation ('around N', 'a few', 'several', 'about K')\n")
	of.WriteString("- there is no count in the request at all\n")
	of.WriteString("Do NOT invent a count from the repository — this field preserves a user-stated count, not a system-derived one.\n\n")
	of.WriteString("### Completeness axis (`completeness_obligation` field)\n\n")
	of.WriteString("When the user demands an EXHAUSTIVE answer — phrases that signal universal coverage of the answer set — emit `completeness_obligation`. The trigger surface includes: (a) universal quantifiers in any language ('all'/'every'/'every single'/'each' / '所有' / '全部' / '每一个' / '每个' / 'tous' / 'todos' / 'tutti'); (b) explicit completeness markers ('complete list'/'exhaustive'/'full inventory'/'完整列表'/'穷举'); (c) plural-relation enumeration phrasings that closed-set semantics imply implicit completeness — 'list all implementers / implementations / subclasses / overrides / conformers / handlers / registrations of <interface | trait | protocol | abstract class | hook>' (any language; the plural noun + closed structural relation is itself the completeness trigger because partial enumeration would mislead the user into thinking the listed set is the full set). Emit:\n")
	of.WriteString("- `required: true`\n")
	of.WriteString("- `source_quote`: the verbatim trigger phrase from the current request\n")
	of.WriteString("Decision rule: if the user would NOT consider the answer complete while one valid item is still missing, set required=true. Distinct from enumeration_boundary which carries a count: completeness demands every item without naming a number. A single question can carry BOTH (e.g. 'all 5 of the X' has both axes). Omit when the user is satisfied with a representative subset / top-N / examples.\n\n")
	of.WriteString("### Bucket-partition axis (`buckets` array)\n\n")
	of.WriteString("When the user EXPLICITLY partitions the answer into named groups — phrases that pair multiple labels with parallel asks (e.g. 'X for A, Y for B' / 'A 和 B 分别...' / 'list ... separately for A and B' / 'compare A vs B') — emit `buckets[]` with one entry per named group:\n")
	of.WriteString("- `label`: the user's verbatim label for this bucket (the section header in the rendered answer will reuse this exact string)\n")
	of.WriteString("- `anchors`: the entities you resolved as belonging to this bucket\n")
	of.WriteString("- `index`: 1-based ordinal in the order labels appear in the question\n")
	of.WriteString("Decision rule: if the answer would naturally split into N sections each titled by a user-named label, emit one bucket per label. Omit for single-topic questions or for multi-topic questions where the user did NOT name the partition (use sub_topics for those — sub_topics is investigator-mental decomposition, buckets is user-stated partition). Buckets requires ≥2 entries (a single bucket is not a partition).\n\n")
	of.WriteString("After emit_analysis succeeds, you may add a one-paragraph rationale for the trace log. This text is captured but does not drive any agent – the structured fields are what matter.")

	return &Config{
		Name: "analysis-skill",
		Goal: "You are a CLASSIFIER, not an investigator. Classify the user request into the structured emit_analysis fields, including intent, scenario, complexity, question_kind, keywords, entities, confidence scores, semantic predicates, and diagnostic_profile, then call emit_analysis exactly once. The explorer stage does the actual investigation — your job is only to verify entity existence and classify.",
		Workflow: []string{
			"Detect the request's language.",
			"Round 1 pre-scan: in a single response, issue parallel calls to repo_map and grep(files_only=true) for each candidate entity. A 'round' is one LLM response — multiple tool calls per response are normal and expected.",
			"If Round 1 resolved every entity, call emit_analysis now — skip Round 2.",
			"Round 2 is allowed at most once, when Round 1 ended ambiguous: broaden keyword stems / variants, but keep every grep(files_only=true). Then call emit_analysis regardless of the Round 2 result. Do not peek at source lines in analyze; line-level grep belongs to explore.",
			"For count / size / total / measurement-scalar questions: Round 1 confirms the subject exists (the directory / file / symbol the question is asking about), then immediately call emit_analysis with intent=return_value + predicates.is_count_question=true + predicates.is_scalar_answer=true. Do NOT attempt to compute the answer literal yourself in pre-scan — the explore stage is the one that runs wc / find / grep -c and reads file content. Trying to retrieve source lines in analyze will exhaust the pre-scan budget and fail-loud the analyze stage.",
			"If the request spans multiple independent sub-topics, fill sub_topics (at most 5).",
			"Call emit_analysis exactly once. Required fields: intent, scenario, complexity, keywords, entities, question_kind, the three confidence floats (intent_confidence, complexity_confidence, kind_confidence), the predicates object (eight booleans: is_scalar_answer, is_role_locate_lookup, is_count_question, is_cross_component, is_relational_lookup, is_category_enumeration, is_history_lookup, is_diagnostic_question), diagnostic_profile (is_diagnostic, current_risk, historical_regression, current_version_check, confidence), answer_role_profile (is_role_binding_requested, confidence), and error_granularity_profile (is_granularity_question, confidence). Optional: sub_topics, answer_subject, predicate_axis, diagram_hint, enumeration_boundary, completeness_obligation, buckets, exact_targets, exact_context_terms, exact_context_roles, source_scope_profile, change_impact_profile, field_value_profile, answer_exclusion_policy, language, required_files.",
			"When you can identify specific files structurally needed to answer the question (e.g. you read the user's wording carefully + saw exact symbol matches in the prescan), populate the optional `required_files` array with `{path, confidence ∈ [0,1], rationale}` entries. The system threshold-bands them: confidence ≥ 0.8 makes the file a primary file AND eligible for prompt pre-read; 0.5 ≤ conf < 0.8 makes it a soft pre-read hint; below 0.5 the entry is dropped — leave the recommendation to the deterministic resolver. Use repo-relative POSIX paths copied verbatim from the prescan results. Empty list is fine: omit when you do not have file-level conviction.",
			"When the prescan pulled in candidate files that you have READ and judged off-topic for the user's question (e.g. you saw the file content in the prescan output and it does NOT match the user's subject), list those paths in the optional `irrelevant_files` array. The system will respect the exclusion across pre-read pools, mid-loop reading suggestions, and primary-file selection — saves prompt tokens and prevents the system from contradicting your judgment. Cap of 10 entries; only include files you actually inspected. Empty list is fine when no candidates need explicit exclusion.",
		},
		ToolSuggestions: append([]string(nil), AnalysisToolSuggestions...),
		OutputFormat:    of.String(),
		Prohibitions:    append([]string(nil), AnalysisHardRules...),
	}
}
