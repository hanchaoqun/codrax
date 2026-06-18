package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_prompt_test.go is the regression fence for the Skill /
// Evaluator boundary on the analyze stage. The analyzer used to
// hardcode ~60 lines of enum tables and hard rules into
// BuildInitialInstruction, duplicating the analysis-skill's declarative
// contract. Two commits removed that duplication — one moved the SSOT
// into internal/skill/analysis_contract.go and one pinned
// BuildInitialInstruction at "". But neither test file asserts the PROMPT
// SHAPE directly, which is how the duplication went unnoticed for so
// long in the first place.
//
// These tests lock three invariants:
//
//  1. The evaluator's BuildInitialInstruction output never contains any of
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
// removed hardcoded BuildInitialInstruction. Every phrase is distinctive
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
			got := eval.BuildInitialInstruction(tc.ctx, sk)
			for _, phrase := range bannedStaticPromptPhrases {
				if strings.Contains(got, phrase) {
					t.Errorf(
						"BuildInitialInstruction output contains banned static phrase %q — "+
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
// guard: every piece of content we removed from BuildInitialInstruction
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
	} {
		header := field + " — pick one:"
		if !strings.Contains(sk.OutputFormat, header) {
			t.Errorf("skill OutputFormat missing enum-table header %q — "+
				"SSOT is incomplete; the LLM will no longer see this field's enum.",
				header)
		}
	}

	// Tool suggestions scope the analyzer's LLM to exactly the
	// evidence-lite pre-scan surface: emit_analysis (the exit
	// channel) plus three read-only navigation tools (repo_map,
	// grep, list_files). BaseAgent.buildToolSchemas only exposes
	// tools named here to the LLM, so a leak (read_file, etc.)
	// would surface as the schema set mismatching this list.
	wantTools := map[string]bool{
		"emit_analysis": true,
		"repo_map":      true,
		"grep":          true,
		"list_files":    true,
	}
	if len(sk.ToolSuggestions) != len(wantTools) {
		t.Errorf("skill ToolSuggestions = %v, want exactly %d tools",
			sk.ToolSuggestions, len(wantTools))
	}
	got := make(map[string]bool, len(sk.ToolSuggestions))
	for _, name := range sk.ToolSuggestions {
		got[name] = true
	}
	for name := range wantTools {
		if !got[name] {
			t.Errorf("skill ToolSuggestions missing %q", name)
		}
	}
	// Heavy / content-reading tools must stay OUT of the analyzer's
	// allowlist. This is the complement of the wantTools check: any
	// future PR that mistakenly adds one surfaces here.
	for _, forbidden := range []string{
		"read_file",
		"exec_command",
		"emit_evidence",
		"emit_answer_symbol",
		"emit_answer_document",
		"emit_hypothesis_verdict",
	} {
		if got[forbidden] {
			t.Errorf("skill ToolSuggestions must NOT contain %q (evidence-lite boundary)", forbidden)
		}
	}
}

// TestAnalyzerPrompt_DynamicContentInjectedViaBuilder assembles the
// full prompt pipeline — BuildPromptContext + ToMessages — with a
// fully-populated AgentContext and the real analysis-skill, then
// asserts every dynamic supplement the analyzer relies on lands in
// the rendered messages. This is the test that catches "the
// evaluator is empty but so is the system" regressions, where
// removing BuildInitialInstruction content accidentally drops a piece of
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
	// for BuildInitialInstruction to contribute. This guard catches the
	// "nobody noticed the evaluator was re-injecting duplicates"
	// regression from two angles.
	if got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, sk); got != "" {
		t.Errorf("analyzer BuildInitialInstruction must be empty because the builder carries "+
			"all per-dispatch context; got %d bytes:\n%s", len(got), got)
	}
}

// TestAnalyzerPrompt_RuntimeObservationOnlyShortcut pins the narrow
// dynamic supplement used when an attached log/trace is known to be
// outside the current checkout. In that case the analyzer already has
// structured runtime facts and should classify immediately instead of
// spending a round searching the repository for files that cannot
// exist there.
func TestAnalyzerPrompt_RuntimeObservationOnlyShortcut(t *testing.T) {
	mut := types.NewMutableState("分析这个外部 panic 日志说明了什么，只分析日志")
	mut.SetRequestModel(types.RequestModel{
		LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "panic"}}},
		ExternalObservationPolicy: &types.ExternalObservationPolicy{
			CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
			SourceQuotes:      []string{"只分析日志"},
			Confidence:        0.9,
		},
	})
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "分析这个外部 panic 日志说明了什么，只分析日志",
		Mutable:   mut,
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{Type: "panic"}},
			// ResolvedFiles intentionally empty: external runtime artifact.
		},
	}
	sk := skill.BuildAnalysisSkill()

	got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, sk)
	for _, want := range []string{
		"Runtime Artifact Classification Shortcut",
		"external to the current checkout",
		"Do not run repo pre-scan just to classify stack-frame or trace literals",
		"default mixed external-observation plus current-source lane",
		"required_files / exact_targets",
		"mechanism explanations backed by current code",
		"Do not collapse a mixed artifact + current-code request into observation-only",
		"diagnostic_profile.current_risk/current_version_check/historical_regression=false",
		"call `emit_analysis` now",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime observation shortcut missing %q in:\n%s", want, got)
		}
	}
	for _, forbidden := range bannedStaticPromptPhrases {
		if strings.Contains(got, forbidden) {
			t.Fatalf("runtime observation shortcut reintroduced static contract phrase %q:\n%s", forbidden, got)
		}
	}
	if strings.Contains(got, "Repository overview") {
		t.Fatalf("runtime observation shortcut must skip precomputed repo overview; got:\n%s", got)
	}
}

func TestAnalyzerPrompt_RuntimeSourceOptionalSkipsRepoOverview(t *testing.T) {
	mut := types.NewMutableState("分析这份 trace 的卡顿原因")
	rm := types.RequestModel{
		Intent:   types.IntentRootCause,
		Scenario: types.ScenarioPerformanceBottleneck,
		PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{
			Kind:    "attached_trace",
			Subject: "com.baidu.tieba-59566",
			Summary: "attached runtime trace artifact is present",
		}}},
		ExternalObservationPolicy: &types.ExternalObservationPolicy{
			CurrentSourceMode: types.ExternalObservationCurrentSourceDefault,
			Confidence:        0.8,
		},
	}
	mut.SetRequestModel(rm)
	mut.SetPerfTrace(rm.PerfTrace)
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "分析这份 trace 的卡顿原因",
		RepoRoot:  ".",
		Mutable:   mut,
		PerfTrace: rm.PerfTrace,
	}

	got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, skill.BuildAnalysisSkill())
	for _, want := range []string{
		"Runtime Artifact Source-Optional Classification Shortcut",
		"current checkout/source evidence is not required",
		"Do not run repo pre-scan just to classify trace/log literals",
		"Keep current-source analysis allowed by default",
		"call `emit_analysis` now",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime source-optional shortcut missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Repository overview") || strings.Contains(got, "Task Map") {
		t.Fatalf("runtime source-optional shortcut must skip precomputed repo overview; got:\n%s", got)
	}
}

func TestAnalyzerPrompt_RuntimeSourceOptionalKeepsSourceWhenRequired(t *testing.T) {
	mut := types.NewMutableState("结合当前代码分析这份 trace")
	rm := types.RequestModel{
		Intent:   types.IntentRootCause,
		Scenario: types.ScenarioPerformanceBottleneck,
		PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{
			Kind:    "attached_trace",
			Subject: "com.baidu.tieba-59566",
		}}},
		AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{
			Path:       "internal/tracequery/query.go",
			Confidence: 0.9,
		}}},
	}
	mut.SetRequestModel(rm)
	mut.SetPerfTrace(rm.PerfTrace)
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "结合当前代码分析这份 trace",
		Mutable:   mut,
		PerfTrace: rm.PerfTrace,
	}

	got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, skill.BuildAnalysisSkill())
	if strings.Contains(got, "Runtime Artifact Source-Optional Classification Shortcut") {
		t.Fatalf("current-source-required runtime request must not take source-optional shortcut:\n%s", got)
	}
}

func TestAnalyzerPrompt_ExternalObservationTurnHintSkipsRepoOverview(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "根据 MCP 返回的外部观测解释现象",
		RepoRoot:  ".",
		TurnRouteHint: types.TurnRouteHint{
			Route:           "repo",
			Source:          "external_tool",
			Operation:       "external_skill_workflow",
			NeedsRepoAccess: true,
			Confidence:      0.85,
		},
	}
	sk := skill.BuildAnalysisSkill()

	got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, sk)
	for _, want := range []string{
		"External Observation First Classification Shortcut",
		"source=external_tool",
		"external observation evidence",
		"Do not run repo pre-scan merely to classify",
		"artifact_citation_mode=\"external_only\"",
		"Keep current-source analysis allowed by default",
		"do not set `current_source_mode=exclude`",
		"normal mixed external-observation plus current-source lane",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("external-observation turn hint shortcut missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Repository overview") || strings.Contains(got, "Task Map") {
		t.Fatalf("external-observation turn hint shortcut must skip precomputed repo overview; got:\n%s", got)
	}
}

func TestAnalyzerPrompt_ExplicitTracePathDoesNotSuppressSourceByDefault(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: `这个trace里面 "record_trace_20260526174055.systrace", "com.tencent.mm-36379" 从 2942.124416 到 2942.260210 sleep 的原因`,
		RepoRoot:  ".",
	}
	sk := skill.BuildAnalysisSkill()

	got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, sk)
	for _, want := range []string{
		"Explicit Runtime Artifact Path Classification Shortcut",
		"call `emit_analysis` now",
		"keep the mixed runtime-artifact plus current-source lane",
		"do not collapse mixed artifact + current-code requests into artifact-only",
		"diagnostic_profile.current_risk/current_version_check/historical_regression=false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("explicit trace path shortcut missing %q in:\n%s", want, got)
		}
	}
	if explicitRuntimeTraceArtifactOnlyRequest(ac) {
		t.Fatalf("explicit trace path must not become typed artifact-only without emitted policy")
	}
	if strings.Contains(got, "Repository overview") || strings.Contains(got, "Task Map") {
		t.Fatalf("explicit runtime artifact path shortcut must skip precomputed repo overview; got:\n%s", got)
	}
}

func TestAnalyzerPrompt_ExplicitTracePathWithCurrentSourceCueKeepsSourceLane(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: `结合当前源码分析 record_trace_20260526174055.systrace 里的 com.tencent.mm-36379 sleep 原因`,
	}
	sk := skill.BuildAnalysisSkill()

	got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, sk)
	for _, want := range []string{
		"Explicit Runtime Artifact Path Classification Shortcut",
		"keep the mixed runtime-artifact plus current-source lane",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mixed trace+source request should keep source-lane guidance %q in:\n%s", want, got)
		}
	}
	if explicitRuntimeTraceArtifactOnlyRequest(ac) {
		t.Fatalf("mixed trace+source request should not be classified as explicit-runtime-only")
	}
}

func TestAnalyzerPrompt_BareRuntimeExtensionDoesNotTriggerArtifactPathShortcut(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "分析当前代码里 .log 路径识别的实现流程",
	}
	sk := skill.BuildAnalysisSkill()

	got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, sk)
	if strings.Contains(got, "Explicit Runtime Artifact Path Classification Shortcut") {
		t.Fatalf("bare .log extension discussion must remain an ordinary source question:\n%s", got)
	}
}

func TestAnalyzerPrompt_BarePerfDataDiscussionDoesNotTriggerArtifactPathShortcut(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "分析当前代码里 raw perf.data fallback parser 的实现流程",
	}
	sk := skill.BuildAnalysisSkill()

	got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, sk)
	if strings.Contains(got, "Explicit Runtime Artifact Path Classification Shortcut") {
		t.Fatalf("bare perf.data parser discussion must remain an ordinary source question:\n%s", got)
	}
}

func TestAnalyzerPrompt_ExistingSuffixlessTracePathTriggersArtifactShortcut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture")
	body := "# tracer: nop\napp-1 (1) [000] .... 1.000000: sched_switch: prev_comm=app prev_pid=1 prev_state=S ==> next_comm=idle next_pid=0\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "只分析 ./capture 这个文件里的调度问题",
		RepoRoot:  dir,
	}
	sk := skill.BuildAnalysisSkill()

	got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, sk)
	if !strings.Contains(got, "Explicit Runtime Artifact Path Classification Shortcut") {
		t.Fatalf("existing suffixless trace-like path should trigger artifact shortcut:\n%s", got)
	}
}

func TestAnalyzerPrompt_BareRelativeSuffixlessTracePathTriggersArtifactShortcut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture")
	body := "app-1 (1) [000] .... 1.000000: sched_switch prev_comm=app prev_pid=1 prev_state=S ==> next_comm=idle next_pid=0\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "只分析 capture 这个文件里的调度问题",
		RepoRoot:  dir,
	}
	sk := skill.BuildAnalysisSkill()

	got := (&analyzerEvaluator{}).BuildInitialInstruction(ac, sk)
	if !strings.Contains(got, "Explicit Runtime Artifact Path Classification Shortcut") {
		t.Fatalf("existing bare relative suffixless trace-like path should trigger artifact shortcut:\n%s", got)
	}
}

// TestAnalyzerPrompt_NoDuplicateSkillTitles is a second-layer
// boundary guard: IF a future refactor legitimately adds dynamic
// content to BuildInitialInstruction (the rule is "dynamic content only",
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

	evalOut := (&analyzerEvaluator{}).BuildInitialInstruction(ac, sk)
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
			t.Errorf("evaluator BuildInitialInstruction duplicates builder-owned section header %q — "+
				"supplements must be strictly additive new titles, never re-renders of the canonical set",
				header)
		}
	}
}

// -----------------------------------------------------------------------------
// Evidence-lite pre-scan contract tests
// -----------------------------------------------------------------------------
//
// The analyze stage runs a 1-2 round "existence + location" pre-scan
// before emit_analysis so the entities and keywords it extracts are
// grounded in the actual repo, not pure language-level guesswork.
// These tests pin the four concrete pieces of that contract so a
// future edit to internal/skill/analysis_contract.go cannot silently
// break the boundary:
//
//  1. ToolSuggestions exposes exactly the four allowed tools.
//  2. The heavy content tools (read_file, exec_command) are NEVER
//     in ToolSuggestions.
//  3. The skill's OutputFormat + Prohibitions text instructs the
//     LLM to use files_only=true on every grep call and forbids
//     read_file / content inspection by name.
//  4. AnalysisToolSuggestions is the single source the builder
//     reads, not a stray fork hiding inside BuildAnalysisSkill.

// TestAnalysisSkill_EvidenceLiteToolsExposedExactly asserts the four
// tools we want and only those four. Mirrors the check inside
// TestAnalyzerPrompt_SkillOwnsContractText but as a dedicated test
// so a future split or rename produces a clearly named failure.
func TestAnalysisSkill_EvidenceLiteToolsExposedExactly(t *testing.T) {
	sk := skill.BuildAnalysisSkill()

	want := []string{"emit_analysis", "grep", "list_files", "repo_map"}
	got := append([]string(nil), sk.ToolSuggestions...)
	// Sort independently for a stable equality check.
	sortStrings := func(in []string) []string {
		out := append([]string(nil), in...)
		for i := 1; i < len(out); i++ {
			for j := i; j > 0 && out[j-1] > out[j]; j-- {
				out[j-1], out[j] = out[j], out[j-1]
			}
		}
		return out
	}
	gotSorted := sortStrings(got)
	wantSorted := sortStrings(want)
	if len(gotSorted) != len(wantSorted) {
		t.Fatalf("ToolSuggestions count = %d, want %d; got=%v want=%v",
			len(gotSorted), len(wantSorted), gotSorted, wantSorted)
	}
	for i := range wantSorted {
		if gotSorted[i] != wantSorted[i] {
			t.Errorf("ToolSuggestions[%d] = %q, want %q (sorted)",
				i, gotSorted[i], wantSorted[i])
		}
	}
}

// TestAnalysisSkill_HeavyToolsNeverAllowed is the negative guard.
// It lists every tool that would break the evidence-lite boundary
// and asserts none of them appear in the allowlist. The list is
// deliberately broader than "just read_file" so a future new tool
// with similar semantics (edit_file, apply_patch, ...) surfaces
// here on the day it is added to internal/tool/.
func TestAnalysisSkill_HeavyToolsNeverAllowed(t *testing.T) {
	sk := skill.BuildAnalysisSkill()
	allowed := make(map[string]bool, len(sk.ToolSuggestions))
	for _, name := range sk.ToolSuggestions {
		allowed[name] = true
	}

	forbidden := []string{
		// Content-reading tools — reserved for the explore stage.
		"read_file",
		// Shell execution — never appropriate at analyze time.
		"exec_command",
		// Other stages' emit channels — strictly out of scope.
		"emit_evidence",
		"emit_answer_symbol",
		"emit_answer_document",
		"emit_hypothesis_verdict",
		// Git tools — content-ish, no place in classification.
		"git_diff",
		"git_show",
		"git_log",
	}
	for _, name := range forbidden {
		if allowed[name] {
			t.Errorf("analyze stage must NOT allow %q (evidence-lite boundary)", name)
		}
	}
}

// TestAnalysisSkill_PromptDocumentsFilesOnlyGuard pins the
// distinctive phrasing that tells the LLM to always pass
// files_only=true when calling grep. This is a text-level check,
// paired with validateAnalyzerPrescanToolCall's runtime enforcement.
// The phrase "files_only=true" must appear in the rendered prompt
// (either in the workflow, the output format section, or the
// prohibitions).
func TestAnalysisSkill_PromptDocumentsFilesOnlyGuard(t *testing.T) {
	sk := skill.BuildAnalysisSkill()

	// Collect every text surface the builder renders into the LLM
	// prompt: OutputFormat is the big one, Workflow is a slice, and
	// Prohibitions is where the hard rule lives.
	var corpus strings.Builder
	corpus.WriteString(sk.Goal)
	corpus.WriteString("\n")
	for _, w := range sk.Workflow {
		corpus.WriteString(w)
		corpus.WriteString("\n")
	}
	corpus.WriteString(sk.OutputFormat)
	corpus.WriteString("\n")
	for _, p := range sk.Prohibitions {
		corpus.WriteString(p)
		corpus.WriteString("\n")
	}

	if !strings.Contains(corpus.String(), "files_only=true") {
		t.Errorf("analysis-skill prompt must instruct the LLM to call grep with files_only=true")
	}
	// The phrase "do NOT call read_file" / equivalent should also
	// appear so the LLM cannot claim "the prompt didn't say no".
	if !strings.Contains(corpus.String(), "read_file") {
		t.Errorf("analysis-skill prompt must name read_file explicitly in its prohibition")
	}
}

// TestAnalysisSkill_PromptDocumentsPreScanBudget pins the pre-scan
// budget ceiling somewhere the LLM actually reads. Batch 3A moved
// the pre-scan workflow out of OutputFormat into Workflow, so the
// assertion scans the full rendered corpus (Goal + Workflow +
// OutputFormat + Prohibitions) instead of OutputFormat alone.
//
// Without a round-limit phrase the LLM will loop on repo_map /
// grep calls forever, burning the pre-scan budget without ever
// emitting.
func TestAnalysisSkill_PromptDocumentsPreScanBudget(t *testing.T) {
	sk := skill.BuildAnalysisSkill()

	var corpus strings.Builder
	corpus.WriteString(sk.Goal)
	corpus.WriteString("\n")
	for _, w := range sk.Workflow {
		corpus.WriteString(w)
		corpus.WriteString("\n")
	}
	corpus.WriteString(sk.OutputFormat)
	corpus.WriteString("\n")
	for _, p := range sk.Prohibitions {
		corpus.WriteString(p)
		corpus.WriteString("\n")
	}
	rendered := corpus.String()

	phrases := []string{
		"Round 2 is allowed at most once", // the canonical batch-3A phrasing
		"at most one round of pre-scan",
		"2 rounds",
		"two rounds",
		"2 pre-scan rounds",
	}
	for _, p := range phrases {
		if strings.Contains(rendered, p) {
			return
		}
	}
	t.Errorf("analysis-skill prompt must document the pre-scan budget ceiling somewhere (Workflow / OutputFormat / Prohibitions); searched for any of: %v", phrases)
}

func TestAnalysisSkill_PromptTeachesRepoMapFirstForStructuralLocationPrescan(t *testing.T) {
	sk := skill.BuildAnalysisSkill()
	rendered := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	for _, want := range []string{
		"mechanism, architecture, call-chain",
		"start with `repo_map` as the navigation pass",
		"use `grep(files_only=true)` only after repo_map is unavailable, empty/ambiguous",
		"do not treat relation rows as proof",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("analysis-skill prompt must teach typed repo_map first-hop; missing %q in:\n%s", want, rendered)
		}
	}
}

func TestAnalysisSkill_PromptDocumentsDirectHistoryClassification(t *testing.T) {
	sk := skill.BuildAnalysisSkill()
	rendered := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	for _, want := range []string{
		"repository-history / git questions",
		"do NOT run a source-code pre-scan",
		"predicates.is_history_lookup=true",
		"choose the answer shape separately",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("analysis-skill prompt must teach direct VCS/history classification; missing %q in:\n%s", want, rendered)
		}
	}
}

func TestAnalysisSkill_PromptDocumentsExternalRuntimeDirectClassification(t *testing.T) {
	sk := skill.BuildAnalysisSkill()
	rendered := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	for _, want := range []string{
		"external-source log / trace",
		"explicit runtime artifact paths",
		".log",
		"resolved_files=0",
		"do NOT run a source-code pre-scan",
		"External observations default to mixed external + current-source analysis",
		"external_observation_policy.current_source_mode=exclude",
		"copy the exact user phrase into source_quotes",
		"diagnostic_profile.current_risk/current_version_check/historical_regression=false",
		"keep the default current-source lane available",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("analysis-skill prompt must teach direct external-runtime classification; missing %q in:\n%s", want, rendered)
		}
	}
}

func TestAnalysisSkill_WorkflowGuidesRepoMapWithoutSourceInventoryExpansion(t *testing.T) {
	sk := skill.BuildAnalysisSkill()
	workflowCorpus := strings.Join(sk.Workflow, "\n")
	for _, want := range []string{
		"repo_map",
		"overview/task_map/file_map",
		`repo_map(view="relation_map")`,
		"sources=[...]",
		"relation_kinds=[...]",
		`repo_map(view="source_inventory")`,
		"source_inventory_profile",
		"call `emit_analysis`",
	} {
		if !strings.Contains(workflowCorpus, want) {
			t.Fatalf("analysis workflow missing repo-map/source-inventory boundary guidance %q:\n%s", want, workflowCorpus)
		}
	}
	for _, forbidden := range []string{
		"for each candidate entity",
		"line-level grep belongs to explore",
		"explore stage",
		"in analyze",
	} {
		if strings.Contains(workflowCorpus, forbidden) {
			t.Fatalf("analysis workflow should not carry misleading/internal phrasing %q:\n%s", forbidden, workflowCorpus)
		}
	}
}

// TestAnalysisSkill_RequiredFieldsEnumeratedEverywhere is the batch
// 3A 3-way consistency gate: every top-level required field in the
// emit_analysis JSON schema must also be named in the skill's
// Workflow text and in the OutputFormat text, so the LLM sees the
// same required-field set from three independent surfaces. Previously
// the three surfaces drifted: Workflow step 6 listed sub_topics as
// required, OutputFormat omitted the four confidence floats, and
// the JSON schema was the only complete list. A field missing from
// two of three surfaces is why the LLM sometimes emits partial
// classifications without the predicates object.
func TestAnalysisSkill_RequiredFieldsEnumeratedEverywhere(t *testing.T) {
	sk := skill.BuildAnalysisSkill()

	workflowCorpus := strings.Join(sk.Workflow, "\n")

	// Canonical required set — matches emit_analysis.go schema
	// "required" array (verified by TestEmitAnalysisSchemaMatchesContract
	// in internal/tool). Keep this list in sync if the schema changes.
	required := []string{
		"intent", "scenario", "complexity",
		"keywords", "entities",
		"question_kind",
		"intent_confidence", "complexity_confidence",
		"kind_confidence",
		"predicates",
		"diagnostic_profile",
		"answer_role_profile",
		"error_granularity_profile",
	}
	for _, f := range required {
		if !strings.Contains(workflowCorpus, f) {
			t.Errorf("Workflow does not name required emit_analysis field %q — the LLM reads Workflow first and will omit it", f)
		}
		if !strings.Contains(sk.OutputFormat, f) {
			t.Errorf("OutputFormat does not name required emit_analysis field %q — the contract section must enumerate every required field", f)
		}
	}
}

// TestAnalysisToolSuggestions_IsTheSingleSourceOfTruth verifies
// that BuildAnalysisSkill uses the exported AnalysisToolSuggestions
// slice rather than a separate literal. Two sources would be the
// exact kind of duplication we just finished eliminating elsewhere
// in this package.
func TestAnalysisToolSuggestions_IsTheSingleSourceOfTruth(t *testing.T) {
	sk := skill.BuildAnalysisSkill()
	if len(sk.ToolSuggestions) != len(skill.AnalysisToolSuggestions) {
		t.Fatalf("BuildAnalysisSkill ToolSuggestions len = %d, AnalysisToolSuggestions len = %d",
			len(sk.ToolSuggestions), len(skill.AnalysisToolSuggestions))
	}
	for i := range skill.AnalysisToolSuggestions {
		if sk.ToolSuggestions[i] != skill.AnalysisToolSuggestions[i] {
			t.Errorf("ToolSuggestions[%d] = %q, AnalysisToolSuggestions[%d] = %q (drift)",
				i, sk.ToolSuggestions[i], i, skill.AnalysisToolSuggestions[i])
		}
	}
}
