package agent

import (
	"strings"
	"testing"

	agentctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_prompt_test.go is the regression fence for the Skill /
// Evaluator boundary on the analyze stage. The analyzer used to
// hardcode ~60 lines of enum tables and hard rules into
// BuildInitialPrompt, duplicating the analysis-skill's declarative
// contract. Two commits removed that duplication — one moved the SSOT
// into internal/skill/analysis_contract.go and one pinned
// BuildInitialPrompt at "". But neither test file asserts the PROMPT
// SHAPE directly, which is how the duplication went unnoticed for so
// long in the first place.
//
// These tests lock three invariants:
//
//  1. The evaluator's BuildInitialPrompt output never contains any of
//     the distinctive phrases from the removed hardcoded prompt, even
//     when ctx is populated with every dynamic field the builder
//     uses. A regression here means a future refactor has started
//     re-seeding static contract text into the evaluator.
//  2. The analysis-skill config returned by skill.BuildAnalysisSkill
//     carries all the classification content — Goal, Workflow,
//     per-field enum tables, Prohibitions — so the LLM still sees
//     the full contract from the skill path. Catches the inverse
//     regression: someone stripped the SSOT thinking it was
//     redundant.
//  3. End-to-end, given a populated AgentContext and the real skill,
//     BuildPromptContext produces a message stream that carries every
//     dynamic section (user request / retry hint / language
//     preference) from the builder side and every static section
//     from the skill side.

// bannedStaticPromptPhrases lists substrings taken verbatim from the
// removed hardcoded BuildInitialPrompt. Every phrase is distinctive
// enough that it cannot plausibly appear in any legitimate dynamic
// supplement — they all come from the old field-description /
// hard-rule blocks. `rg` on the live source confirms each phrase is
// absent before this test was written; any future commit that
// re-introduces one will fail the TestAnalyzerPrompt_NoStaticContractText
// assertions below.
var bannedStaticPromptPhrases = []string{
	// Old markdown header for the field block.
	"Required emit_analysis fields",
	// Full descriptive phrase that identified the question_kind field
	// in the old hardcoded text.
	"ERM predicate selector",
	// Full descriptive phrase that identified the answer_shape field.
	"anti-hallucination selector",
	// Old sentence pattern for the intent description ("— the task
	// intent. Pick one:"). Case-sensitive so it catches the old text
	// but not the skill's lower-case "— pick one:" header pattern.
	"task intent. Pick one",
	// Old hardcoded reference to ERM's predicate whitelist.
	"predicate whitelist",
}

// TestAnalyzerPrompt_NoStaticContractText asserts that no banned
// phrase from the removed hardcoded prompt reappears in the
// evaluator's output. Runs across three ctx shapes — nil, empty, and
// fully populated — so the future maintainer cannot sneak static
// content into a "only when ctx has field X" branch.
func TestAnalyzerPrompt_NoStaticContractText(t *testing.T) {
	eval := &analyzerEvaluator{}
	sk := skill.BuildAnalysisSkill()

	cases := []struct {
		name string
		ctx  *types.AgentContext
	}{
		{"nil-ctx", nil},
		{"empty-ctx", &types.AgentContext{}},
		{
			"populated-ctx",
			&types.AgentContext{
				AgentName:    types.AgentAnalyzer,
				Stage:        types.StageAnalyze,
				Objective:    "trace the analyze dispatch end-to-end",
				MissingPiece: types.MissingFacts,
				Preferences:  []string{"Respond to the user in Simplified Chinese (zh)."},
				RetryHint:    "previous attempt emitted 0 times; retry with keywords",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := eval.BuildInitialPrompt(tc.ctx, sk)
			for _, phrase := range bannedStaticPromptPhrases {
				if strings.Contains(got, phrase) {
					t.Errorf(
						"BuildInitialPrompt output contains banned static phrase %q — "+
							"someone put the old hardcoded contract text back; it belongs "+
							"in internal/skill/analysis_contract.go, not the evaluator.\n"+
							"Full output:\n%s",
						phrase, got)
				}
			}
		})
	}
}

// TestAnalyzerPrompt_SkillOwnsContractText is the inverse regression
// guard: every piece of content we removed from BuildInitialPrompt
// must still be carried by the analysis-skill, otherwise the LLM
// sees neither side. Checks the declarative Goal / Workflow /
// OutputFormat / Prohibitions are non-empty and the OutputFormat
// carries an enum-table header for every classification field.
func TestAnalyzerPrompt_SkillOwnsContractText(t *testing.T) {
	sk := skill.BuildAnalysisSkill()

	if sk == nil {
		t.Fatal("skill.BuildAnalysisSkill returned nil")
	}
	if sk.Name != "analysis-skill" {
		t.Errorf("skill Name = %q, want %q", sk.Name, "analysis-skill")
	}
	if strings.TrimSpace(sk.Goal) == "" {
		t.Error("skill Goal must not be empty")
	}
	if len(sk.Workflow) == 0 {
		t.Error("skill Workflow must not be empty")
	}
	if strings.TrimSpace(sk.OutputFormat) == "" {
		t.Fatal("skill OutputFormat must not be empty")
	}
	if len(sk.Prohibitions) == 0 {
		t.Error("skill Prohibitions must not be empty")
	}

	// Every classification field must have its enum table rendered
	// into OutputFormat via renderEnumTable. The header format is
	// "<field> — pick one:" (em-dash; see renderEnumTable in
	// internal/skill/analysis_contract.go).
	for _, field := range []string{
		"intent",
		"scenario",
		"complexity",
		"question_kind",
		"answer_shape",
	} {
		header := field + " — pick one:"
		if !strings.Contains(sk.OutputFormat, header) {
			t.Errorf("skill OutputFormat missing enum-table header %q — "+
				"SSOT is incomplete; the LLM will no longer see this field's enum.",
				header)
		}
	}

	// Tool suggestions must still scope the LLM to emit_analysis only.
	// A leak here would allow the analyzer to call arbitrary tools.
	if len(sk.ToolSuggestions) != 1 || sk.ToolSuggestions[0] != "emit_analysis" {
		t.Errorf("skill ToolSuggestions = %v, want [emit_analysis]", sk.ToolSuggestions)
	}
}

// TestAnalyzerPrompt_DynamicContentInjectedViaBuilder assembles the
// full prompt pipeline — BuildPromptContext + ToMessages — with a
// fully-populated AgentContext and the real analysis-skill, then
// asserts every dynamic supplement the analyzer relies on lands in
// the rendered messages. This is the test that catches "the
// evaluator is empty but so is the system" regressions, where
// removing BuildInitialPrompt content accidentally drops a piece of
// dynamic context that no other layer replaces.
//
// Dynamic supplements checked:
//   - User Request user section (from ctx.Objective)
//   - User Preferences system section (from ctx.Preferences)
//   - Retry Directive user section (from ctx.RetryHint)
//
// The test embeds unique sentinel strings in each field so a partial
// match somewhere else in the prompt cannot mask a missing injection.
func TestAnalyzerPrompt_DynamicContentInjectedViaBuilder(t *testing.T) {
	const (
		objectivePin  = "SENTINEL_OBJECTIVE_7FQX: trace the analyze dispatch"
		preferencePin = "SENTINEL_PREFERENCE_9KJP: respond in Simplified Chinese"
		retryPin      = "SENTINEL_RETRY_4MWL: previous dispatch emitted 0 times"
	)

	ac := &types.AgentContext{
		AgentName:   types.AgentAnalyzer,
		Stage:       types.StageAnalyze,
		Objective:   objectivePin,
		Preferences: []string{preferencePin},
		RetryHint:   retryPin,
	}
	sk := skill.BuildAnalysisSkill()

	pc := agentctx.BuildPromptContext(ac, sk)
	msgs := agentctx.ToMessages(pc)

	if len(msgs) < 2 {
		t.Fatalf("ToMessages produced %d messages, want system + user", len(msgs))
	}
	sysMsg, userMsg := msgs[0].Content, msgs[1].Content

	if !strings.Contains(userMsg, objectivePin) {
		t.Errorf("user message missing objective sentinel %q:\n%s", objectivePin, userMsg)
	}
	if !strings.Contains(sysMsg, preferencePin) {
		t.Errorf("system message missing language-preference sentinel %q:\n%s", preferencePin, sysMsg)
	}
	if !strings.Contains(userMsg, retryPin) {
		t.Errorf("user message missing retry-directive sentinel %q:\n%s", retryPin, userMsg)
	}

	// The evaluator's supplement must still be empty after all this —
	// BuildPromptContext carries everything, so there is nothing left
	// for BuildInitialPrompt to contribute. This guard catches the
	// "nobody noticed the evaluator was re-injecting duplicates"
	// regression from two angles.
	if got := (&analyzerEvaluator{}).BuildInitialPrompt(ac, sk); got != "" {
		t.Errorf("analyzer BuildInitialPrompt must be empty because the builder carries "+
			"all per-dispatch context; got %d bytes:\n%s", len(got), got)
	}
}

// TestAnalyzerPrompt_NoDuplicateSkillTitles is a second-layer
// boundary guard: IF a future refactor legitimately adds dynamic
// content to BuildInitialPrompt (the rule is "dynamic content only",
// not "empty forever"), the supplement must still not restate any of
// the titled sections the builder already renders. Section-title
// collision is how the two layers silently drift into
// contradictory directives even when the content itself is different.
//
// The canonical title list is duplicated here with a comment naming
// its single source of truth (canonicalSystemSectionOrder +
// canonicalUserSectionOrder in internal/context/builder.go) so a
// grep on either file reaches both copies.
func TestAnalyzerPrompt_NoDuplicateSkillTitles(t *testing.T) {
	ac := &types.AgentContext{
		AgentName:   types.AgentAnalyzer,
		Stage:       types.StageAnalyze,
		Objective:   "pin the no-duplicate-titles contract",
		Preferences: []string{"Respond to the user in English."},
		RetryHint:   "retry-hint",
	}
	sk := skill.BuildAnalysisSkill()

	evalOut := (&analyzerEvaluator{}).BuildInitialPrompt(ac, sk)
	if evalOut == "" {
		// Happy case — nothing to duplicate. The companion test
		// TestAnalyzerPrompt_DynamicContentInjectedViaBuilder already
		// asserts this explicitly, so here we just short-circuit.
		return
	}

	// Mirrors canonicalSystemSectionOrder + canonicalUserSectionOrder
	// from internal/context/builder.go. Kept minimal and grep-visible.
	canonicalSectionHeaders := []string{
		"## Skill Goal",
		"## Workflow",
		"## Output Format",
		"## Prohibitions",
		"## User Preferences",
		"## User Request",
		"## Retry Directive",
		"## Agent Identity",
		"## Reasoning Hygiene",
	}
	for _, header := range canonicalSectionHeaders {
		if strings.Contains(evalOut, header) {
			t.Errorf("evaluator BuildInitialPrompt duplicates builder-owned section header %q — "+
				"supplements must be strictly additive new titles, never re-renders of the canonical set",
				header)
		}
	}
}
