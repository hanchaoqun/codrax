package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_intent.go holds a deterministic sanity rule that patches
// the most common intent misclassification we see in production: the
// LLM treats a "count / how many / 统计" question as IntentEnumerate
// and downstream the compiler picks ShapeListOfSymbols with a
// citation_count_ge=1 floor (see
// internal/analysis/compiler/templates.go:516-517). The scalar answer
// that the investigation legitimately produces cannot satisfy either
// the bulleted-symbols shape check or the file:line citation floor,
// and the contract-retry loop burns its budget on a mismatch no
// amount of re-investigation can fix.
//
// The rule mirrors the discipline of analyzer_complexity.go: fire only
// on strong structural signals, leave the LLM's choice untouched in
// every borderline case. Specifically:
//
//   - Triggers ONLY when the LLM picked IntentEnumerate; other intents
//     are left alone (the rule cannot create an enumerate false negative).
//   - Triggers ONLY when the request's leading clause matches a count-
//     verb cue — not any position mid-prompt. "list handlers that
//     count requests" is not touched because "list" is the prefix;
//     "count" appears mid-clause.
//   - Triggers ONLY when Complexity is simple (after reconcileComplexity
//     has already had its say). Moderate / complex enumerations can
//     legitimately include counting language ("list all files and
//     report sizes") and downgrading would lose the enumeration half.

// isMeasurementScalarRequest reports whether the request is asking for
// a single scalar produced by a tool query — "how many X", "统计 …",
// "total of Y" — where the answer has no file:line to cite.
//
// The check is independent of whether the LLM initially picked
// IntentEnumerate (then reconcileIntent downgrades) or IntentReturnValue
// (already correct) — both populations need the three-citation-gate
// carve-out in buildAnalysisIR. Previously the carve-out keyed off
// "reconcileIntent fired" which missed the case where the LLM nailed
// the intent directly; the citation gates stayed enabled and the
// retry budget looped the same way as the original bug.
//
// Three gates in force, all fire equally for both populations:
//
//   1. ComplexitySimple  — moderate/complex count-style questions
//                          can have legitimate file:line evidence
//                          ("list all files > N lines and their
//                          counts") so we don't strip gates there.
//   2. Intent in {enumerate, return_value} — other intents
//                          (explain / trace / root_cause) are never
//                          measurement-scalar even if the prose
//                          starts with a count verb.
//   3. Leading count-verb prefix — "list handlers that count requests"
//                          does not fire because "list" is the prefix;
//                          only first-verb cues count.
func isMeasurementScalarRequest(rm types.RequestModel, rawRequest string) bool {
	if rm.Complexity != types.ComplexitySimple {
		return false
	}
	if rm.Intent != types.IntentEnumerate && rm.Intent != types.IntentReturnValue {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(rawRequest))
	lower = stripPolitenessPrefix(lower)
	return hasLeadingCountVerb(lower)
}

// reconcileIntent returns the intent that should travel downstream and
// a short reason string. When resolved == declared the rule did not
// fire and reason is empty. Runs AFTER reconcileComplexity so the
// complexity input is post-reconcile.
//
// This is the strict downgrade contract (enumerate → return_value on
// count-verb prefix). For the broader "is this a measurement scalar
// question regardless of declared intent" check that drives the
// citation-gate carve-out, see isMeasurementScalarRequest above.
func reconcileIntent(declared types.Intent, rawRequest string, complexity types.Complexity) (types.Intent, string) {
	if declared != types.IntentEnumerate {
		return declared, ""
	}
	if complexity != types.ComplexitySimple {
		return declared, ""
	}
	lower := strings.ToLower(strings.TrimSpace(rawRequest))
	lower = stripPolitenessPrefix(lower)
	if !hasLeadingCountVerb(lower) {
		return declared, ""
	}
	return types.IntentReturnValue,
		"leading count-verb cue (how many / 统计 / count / total) — answer is a scalar, not a list"
}

// countVerbPrefixes are matched at the start of the (trimmed,
// politeness-stripped) request. HasPrefix matching is what keeps
// "list handlers that count requests" safe — "list" is the prefix,
// "count" is mid-clause and does not fire.
//
// Keep this list short on purpose — every entry must be a phrase a
// real user has been observed to ask as a count question. Curation
// discipline: do NOT add a token because it "could" be count-like;
// add it when a failing eval run shows a miss. This matches the
// curation pattern of simpleLookupCues / crossComponentCues in
// analyzer_complexity.go.
var countVerbPrefixes = []string{
	// Chinese — note no trailing space: Chinese tokens are not
	// space-delimited, so HasPrefix against the concatenated verb
	// is the correct shape.
	"统计",
	"数一下",
	"数下",
	"算一下",
	"算下",
	"有多少",
	"多少个",
	"多少行",
	"总共",
	"一共",
	// English — trailing space forces whole-word prefix match.
	"how many ",
	"how much ",
	"count ",
	"number of ",
	"total ",
	"size of ",
	"tally ",
	"what's the total",
	"what is the total",
	"what's the count",
	"what is the count",
	"what's the size",
	"what is the size",
}

func hasLeadingCountVerb(lower string) bool {
	for _, p := range countVerbPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// politenessPrefixes are stripped once before the leading-verb check
// so "please count X" and "请统计 X" still fire. Kept deliberately
// minimal — no recursive strip (if a user writes "please could you
// count ..." we accept the miss rather than chain strips).
var politenessPrefixes = []string{
	"please ",
	"could you ",
	"can you ",
	"请帮我",
	"请帮",
	"请",
	"帮我",
	"帮忙",
	"麻烦",
}

func stripPolitenessPrefix(lower string) string {
	for _, p := range politenessPrefixes {
		if strings.HasPrefix(lower, p) {
			return strings.TrimSpace(lower[len(p):])
		}
	}
	return lower
}

// enumerationCuePrefixes extend countVerbPrefixes with leading cues
// that ask for a count but typically carry a relational tail ("有几个
// X 可以 Y") — cues where the count alone is uninteresting and the Y
// side is the real question. Kept disjoint from countVerbPrefixes:
// the latter drives the strict intent downgrade (enumerate →
// return_value) and its list is curated to avoid false-positive
// downgrades. This set is consulted ONLY from reconcileComplexity's
// Rule 6, which upgrades complexity without touching intent, so a
// looser membership is safe.
var enumerationCuePrefixes = []string{
	"有几个",
	"几个",
	"哪几个",
	"哪些",
	"which ",
}

// hasLeadingEnumerationCue reports whether the request starts with a
// count-style cue — either the strict count-verb set from
// countVerbPrefixes or the broader enumerationCuePrefixes list.
// Prefix-matching (post politeness strip) keeps "list handlers that
// count requests" from tripping the rule.
func hasLeadingEnumerationCue(lower string) bool {
	if hasLeadingCountVerb(lower) {
		return true
	}
	for _, p := range enumerationCuePrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// relationalVerbCues are verbs that imply the enumeration target is
// defined by its relationship to another symbol, not by intrinsic
// attributes. Pure "how many files over 100 lines" has no relational
// verb and stays simple; "how many agents CAN INVOKE subagents"
// lifts to moderate because the answer depends on a cross-file
// relationship trace.
//
// Curation discipline mirrors crossComponentCues / simpleLookupCues
// in analyzer_complexity.go — add a verb here when a failing run
// shows the rule missed, not preemptively.
var relationalVerbCues = []string{
	// English — whole-word padded matches. "uses" / "calls" etc.
	// need spaces to avoid firing inside nouns ("abuses", "callback").
	" can ", " could ", " may ",
	" calls ", " call ", " calling ",
	" invokes ", " invoke ", " invoking ",
	" dispatches ", " dispatch ", " dispatching ",
	" registers ", " register ", " registered ",
	" implements ", " implement ",
	" extends ", " extend ",
	" embeds ", " embed ",
	" contains ", " contain ",
	" depends on ", " depend on ",
	" triggers ", " trigger ",
	" references ", " reference ",
	" binds ", " bind ",
	" wires ", " wire ",
	" provides ", " provide ",
	" returns ", " return ",
	" handles ", " handle ",
	" uses ", " use ",
	// Chinese — no space delimiters, so bare substring is the right
	// shape. Tokens are chosen so they cannot appear inside unrelated
	// compounds (e.g. 调用 is unambiguous; 用 alone would match 使用
	// 范围 and similar noise).
	"可以",
	"能够",
	"调用",
	"注册",
	"实现",
	"继承",
	"包含",
	"依赖",
	"触发",
	"引用",
	"绑定",
	"处理",
	"分发",
	"调度",
}

// containsRelationalVerbCue reports whether any relational verb
// appears anywhere in the (lowercased, trimmed) request. Substring
// match for Chinese; whole-word padded match for English is baked
// into the cue list itself.
func containsRelationalVerbCue(lower string) bool {
	padded := " " + lower + " "
	for _, cue := range relationalVerbCues {
		if strings.Contains(padded, cue) {
			return true
		}
	}
	return false
}

// logIntentReconcile is the twin of logComplexityReconcile — one
// warning line when the rule overrode the LLM's pick, silent no-op
// otherwise. Matching log levels let operators grep a single trace
// for "[analyzer] * reconciled:" to find every automatic override.
func logIntentReconcile(before, after types.Intent, reason string) {
	if before == after || reason == "" {
		return
	}
	logging.Warning("[analyzer] intent reconciled: %s → %s (%s)", before, after, reason)
}

// ── AnswerSubject inference + Shape reconciliation ───────────────────
//
// inferAnswerSubject and reconcileShape are the CGEC additions to the
// analyzer's deterministic post-processing chain. They mirror the
// reconcileIntent / reconcileComplexity pattern: fire only on strong
// signals, log every override under "[analyzer] * reconciled:" so
// operators can grep one trace for every automatic decision, and
// leave the LLM's choice untouched in every borderline case.
//
// Why both rules:
//
//   inferAnswerSubject classifies WHAT KIND of code-literal the
//   answer is supposed to be (skill_name, agent_name, config_key,
//   ...). The chain ranker uses this to demote chains whose terminal
//   token is the wrong kind ("SubExplorer.Name() returns 'explorer'"
//   should not rank highest when the question asks for a SKILL).
//
//   reconcileShape handles the corner where the LLM picked
//   ShapeConfigValue (key=value pair) but the actual answer is a Go
//   struct-field literal — e.g. the topology.go map literal that
//   binds "explore-skill" to the explorer agent. Forcing config_value
//   shape on a Go literal manufactures a fake "key" the LLM has to
//   invent, which downstream contract checks then reject.

// answerSubjectCue maps a leading prose pattern to an expected
// AnswerSubjectKind. Curated tightly: each entry must come from a
// real eval-trace miss, not speculation. Patterns are case-folded
// substring matches against the politeness-stripped raw request.
type answerSubjectCue struct {
	pattern string
	kind    types.AnswerSubjectKind
	axes    []string
}

// answerSubjectCues fires before the question_kind fallback. Order
// matters: the first match wins, so the more specific patterns come
// first. Bilingual entries to match the codrax convention (zh + en).
var answerSubjectCues = []answerSubjectCue{
	// SkillName — skill of agent / 默认 skill / 用哪个 skill
	{"默认用哪个skill", types.SubjectSkillName, []string{"agent → skill"}},
	{"默认用哪个 skill", types.SubjectSkillName, []string{"agent → skill"}},
	{"默认的 skill", types.SubjectSkillName, []string{"agent → skill"}},
	{"默认skill", types.SubjectSkillName, []string{"agent → skill"}},
	{"哪个skill", types.SubjectSkillName, []string{"agent → skill"}},
	{"哪个 skill", types.SubjectSkillName, []string{"agent → skill"}},
	{"什么skill", types.SubjectSkillName, []string{"agent → skill"}},
	{"什么 skill", types.SubjectSkillName, []string{"agent → skill"}},
	{"default skill", types.SubjectSkillName, []string{"agent → skill"}},
	{"which skill", types.SubjectSkillName, []string{"agent → skill"}},
	{"what skill", types.SubjectSkillName, []string{"agent → skill"}},

	// AgentName — agent of stage / 默认 agent
	{"默认agent", types.SubjectAgentName, []string{"stage → agent"}},
	{"默认 agent", types.SubjectAgentName, []string{"stage → agent"}},
	{"哪个agent", types.SubjectAgentName, []string{"stage → agent"}},
	{"哪个 agent", types.SubjectAgentName, []string{"stage → agent"}},
	{"什么agent", types.SubjectAgentName, []string{"stage → agent"}},
	{"什么 agent", types.SubjectAgentName, []string{"stage → agent"}},
	{"default agent", types.SubjectAgentName, []string{"stage → agent"}},
	{"which agent", types.SubjectAgentName, []string{"stage → agent"}},
	{"what agent", types.SubjectAgentName, []string{"stage → agent"}},

	// ConfigKey — explicit config key / 配置项 / 配置键
	{"配置项", types.SubjectConfigKey, []string{"key → value"}},
	{"配置键", types.SubjectConfigKey, []string{"key → value"}},
	{"config key", types.SubjectConfigKey, []string{"key → value"}},
	{"yaml key", types.SubjectConfigKey, []string{"key → value"}},

	// ReturnValue — what does X return / X 返回什么
	{"返回什么", types.SubjectReturnValue, []string{"function → value"}},
	{"返回值是", types.SubjectReturnValue, []string{"function → value"}},
	{"what does", types.SubjectReturnValue, []string{"function → value"}},
	{"what is the return", types.SubjectReturnValue, []string{"function → value"}},

	// FunctionName — what function / 哪个函数 / which function
	{"哪个函数", types.SubjectFunctionName, []string{"behavior → function"}},
	{"什么函数", types.SubjectFunctionName, []string{"behavior → function"}},
	{"which function", types.SubjectFunctionName, []string{"behavior → function"}},
	{"what function", types.SubjectFunctionName, []string{"behavior → function"}},

	// TypeName — what type / 什么类型 / which struct
	{"什么类型", types.SubjectTypeName, []string{"value → type"}},
	{"哪个类型", types.SubjectTypeName, []string{"value → type"}},
	{"which type", types.SubjectTypeName, []string{"value → type"}},
	{"what type", types.SubjectTypeName, []string{"value → type"}},
	{"which struct", types.SubjectTypeName, []string{"value → type"}},

	// FilePath — which file / 哪个文件
	{"哪个文件", types.SubjectFilePath, []string{"behavior → file"}},
	{"在哪个文件", types.SubjectFilePath, []string{"behavior → file"}},
	{"which file", types.SubjectFilePath, []string{"behavior → file"}},
}

// inferAnswerSubject derives AnswerSubject when the LLM left it zero.
// Returns the resolved subject + a short reason for the log line. An
// empty Kind in the result means no rule fired and the caller should
// keep the LLM's value (or accept SubjectUnknown).
//
// Two-step inference:
//
//  1. Cue match against the politeness-stripped lower-case request.
//     The first matching cue wins. Bilingual patterns curated above.
//  2. Fallback by question_kind enum. The LLM's question_kind is a
//     stable signal even when the prose lacks an obvious cue:
//
//       config_mapping  → SubjectConfigKey
//       registration    → SubjectAgentName  (registries usually bind agents)
//       return_value    → SubjectReturnValue
//       enumeration     → SubjectGeneric    (lists are heterogenous)
//       call_chain      → SubjectFunctionName
//       mechanism       → SubjectGeneric
//       conditional     → SubjectGeneric
//
// Confidence is 0.8 for cue matches (unambiguous prose) and 0.4 for
// question_kind fallbacks (the kind is right but the prose did not
// pin a specific subject).
func inferAnswerSubject(rm types.RequestModel, rawRequest string) (types.AnswerSubject, string) {
	// Cue match runs FIRST, with high (0.8) confidence — curated
	// explicit bilingual patterns are more reliable than the LLM's
	// own AnswerSubject classification, which has been observed to
	// mis-label "agent → skill" questions as SubjectConfigKey
	// (log 2026-04-18T14:25 — "explorer agent 默认用哪个 skill?"
	// → LLM picked config_key → reconcileShape skipped → finalizer
	// invented a fake config key). A matching cue overrides the
	// LLM-supplied kind; entity_axes prefers the LLM's value if
	// provided (the LLM often gets the relation shape right even
	// when the subject kind is wrong).
	lower := strings.ToLower(strings.TrimSpace(rawRequest))
	lower = stripPolitenessPrefix(lower)
	for _, cue := range answerSubjectCues {
		if strings.Contains(lower, cue.pattern) {
			// Preserve LLM-supplied entity_axes when present so we
			// don't discard useful relational annotation.
			axes := append([]string(nil), cue.axes...)
			if len(rm.AnswerSubject.EntityAxes) > 0 {
				axes = append([]string(nil), rm.AnswerSubject.EntityAxes...)
			}
			reason := "cue match: " + cue.pattern
			// Flag the override in the log when LLM disagreed, so
			// operators can tell which cases cue rescued.
			if rm.AnswerSubject.Kind != types.SubjectUnknown && rm.AnswerSubject.Kind != cue.kind {
				reason = fmt.Sprintf("cue match: %s (overriding LLM=%s)", cue.pattern, rm.AnswerSubject.Kind)
			}
			return types.AnswerSubject{
				Kind:       cue.kind,
				EntityAxes: axes,
				Confidence: 0.8,
			}, reason
		}
	}
	// No cue matched — honour LLM-supplied subject if present.
	if rm.AnswerSubject.Kind != types.SubjectUnknown {
		return rm.AnswerSubject, ""
	}
	// Fallback by question_kind. AnalyzerHints.Kind is the LLM's
	// raw question_kind value (registration / mechanism / config_mapping /
	// enumeration / call_chain / return_value / conditional / unknown).
	switch rm.AnalyzerHints.Kind {
	case "config_mapping":
		return types.AnswerSubject{Kind: types.SubjectConfigKey, EntityAxes: []string{"key → value"}, Confidence: 0.4},
			"question_kind=config_mapping → ConfigKey"
	case "registration":
		return types.AnswerSubject{Kind: types.SubjectAgentName, EntityAxes: []string{"stage → agent"}, Confidence: 0.4},
			"question_kind=registration → AgentName"
	case "return_value":
		return types.AnswerSubject{Kind: types.SubjectReturnValue, EntityAxes: []string{"function → value"}, Confidence: 0.4},
			"question_kind=return_value → ReturnValue"
	case "call_chain":
		return types.AnswerSubject{Kind: types.SubjectFunctionName, EntityAxes: []string{"behavior → function"}, Confidence: 0.4},
			"question_kind=call_chain → FunctionName"
	case "enumeration":
		return types.AnswerSubject{Kind: types.SubjectGeneric, Confidence: 0.3},
			"question_kind=enumeration → Generic"
	case "mechanism", "conditional":
		return types.AnswerSubject{Kind: types.SubjectGeneric, Confidence: 0.2},
			"question_kind=" + rm.AnalyzerHints.Kind + " → Generic"
	}
	// CGEC E1 hard-fallback: no cue matched AND no question_kind
	// fallback fired. Before this commit the function returned the
	// zero value (SubjectUnknown), which neutered every downstream
	// consumer — chain ranker (C2) skips ranking when Unknown,
	// reconcileShape does nothing, rankChainsBySubject returns the
	// unsorted input, RepairRebindSubject never fires. That made
	// every helper in the AnswerSubject layer dead code whenever
	// the LLM fell outside the cue table.
	//
	// Hard-fallback to SubjectGeneric with low confidence. Generic
	// is the weakest-constraint kind (accepts any source-code token
	// via subject.Score), so it does not cause false narrowing, but
	// it DOES switch the subject-aware downstream consumers from
	// "off" to "passive-on" — reconcileShape won't flip ConfigValue
	// (the subject is not a source-code literal kind), rankChainsBySubject
	// still preserves insertion order (match=0 on all chains), and
	// pre-complete check (f) stays inactive. Operator observability
	// gets a log line so the fallback is grep-able in trace.
	return types.AnswerSubject{Kind: types.SubjectGeneric, Confidence: 0.1},
		"hard fallback: no cue match and question_kind missing — defaulting to Generic (weakest kind)"
}

// reconcileShape swaps AnswerShape from ShapeConfigValue to
// ShapeValue when the AnswerSubject is a source-code literal that
// has no YAML "key" surface.
//
// The LLM frequently picks ShapeConfigValue for any "what is the
// X for Y" question because the schema enum lists config_value
// before value, but most "X for Y" questions in real code resolve
// to a Go map literal / struct field, NOT a YAML key. Forcing the
// config_value shape on these questions makes the finalizer invent
// a fake key like "explorerAgent.defaultSkill" to satisfy the
// schema, which the contract checker then rejects with
// "config_value answer must express a key=value or key: value pair".
//
// Returns the resolved shape + a short reason for the log line.
// Empty reason means no rule fired.
func reconcileShape(declared types.AnswerShape, subject types.AnswerSubject) (types.AnswerShape, string) {
	if declared != types.ShapeConfigValue {
		return declared, ""
	}
	switch subject.Kind {
	case types.SubjectSkillName, types.SubjectAgentName,
		types.SubjectFunctionName, types.SubjectTypeName,
		types.SubjectInterface, types.SubjectHandlerRoute,
		types.SubjectReturnValue:
		return types.ShapeValue,
			"answer subject is source-code literal (" + string(subject.Kind) + ") not a YAML config key"
	}
	return declared, ""
}

// logSubjectInferred + logShapeReconciled are the structural twins of
// logIntentReconcile. One warning line per actual override. Silent
// when the rule did not fire.
func logSubjectInferred(subject types.AnswerSubject, reason string) {
	if subject.Kind == types.SubjectUnknown || reason == "" {
		return
	}
	logging.Warning("[CGEC] E1 subject_inferred: kind=%s axes=%v conf=%.2f (%s)",
		subject.Kind, subject.EntityAxes, subject.Confidence, reason)
}

func logShapeReconciled(before, after types.AnswerShape, reason string) {
	if before == after || reason == "" {
		return
	}
	logging.Warning("[analyzer] shape reconciled: %s → %s (%s)", before, after, reason)
}
