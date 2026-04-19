package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDropCitationCountGE verifies the helper preserves non-matching
// criteria in order, removes every matching one, and never returns
// the caller's backing array.
func TestDropCitationCountGE(t *testing.T) {
	in := []types.Criterion{
		{Kind: types.CritContainsSymbol, Expr: "Foo"},
		{Kind: types.CritCitationCountGE, Expr: "1"},
		{Kind: types.CritRegexMatch, Expr: `\d+`},
		{Kind: types.CritCitationCountGE, Expr: "3"},
	}
	got := dropCitationCountGE(in)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Kind != types.CritContainsSymbol || got[1].Kind != types.CritRegexMatch {
		t.Errorf("unexpected surviving kinds: %+v", got)
	}

	// nil / empty passthrough.
	if out := dropCitationCountGE(nil); out != nil {
		t.Errorf("nil input should return nil, got %+v", out)
	}
	if out := dropCitationCountGE([]types.Criterion{}); len(out) != 0 {
		t.Errorf("empty input should stay empty, got %+v", out)
	}
}

// TestBuildAnalysisIR_CountQuestionStripsAllThreeGates verifies that
// a "how many X" / "统计 …" question with an LLM-declared enumerate
// intent ends up with zero citation gate anywhere in the IR:
//
//   1. AnswerContract.CitationReq.Required == false
//   2. No CritCitationCountGE entries in AnswerContract.AcceptanceTests
//   3. No CritCitationCountGE entries on any TaskNode.SuccessCriteria
//
// Regression guard for the three-gate carve-out in buildAnalysisIR.
func TestBuildAnalysisIR_CountQuestionStripsAllThreeGates(t *testing.T) {
	mut := types.NewMutableState("统计一下这个项目下的 go 代码量")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "统计一下这个项目下的 go 代码量",
		Intent:     types.IntentEnumerate, // the mis-classification under test
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"go", "lines", "count"},
			Entities: []string{},
			Shape:    string(types.ShapeListOfSymbols), // stale hint the LLM usually co-emits
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}

	if ir.RequestModel.Intent != types.IntentReturnValue {
		t.Errorf("Intent = %q, want return_value (reconcileIntent did not fire)", ir.RequestModel.Intent)
	}
	if ir.AnswerContract.CitationReq.Required {
		t.Error("AnswerContract.CitationReq.Required should be false")
	}
	if got := ir.AnswerContract.CitationReq.MinCitations; got != 0 {
		t.Errorf("AnswerContract.CitationReq.MinCitations = %d, want 0", got)
	}
	for _, a := range ir.AnswerContract.AcceptanceTests {
		if a.Kind == types.CritCitationCountGE {
			t.Errorf("AcceptanceTests still carries CritCitationCountGE: %+v", a)
		}
	}
	for _, n := range ir.TaskGraph.Nodes {
		for _, c := range n.SuccessCriteria {
			if c.Kind == types.CritCitationCountGE {
				t.Errorf("TaskNode %q SuccessCriteria still carries CritCitationCountGE: %+v", n.ID, c)
			}
		}
	}
}

// TestBuildAnalysisIR_CountQuestionLLMPickedReturnValue covers the
// critical case where the LLM already picked IntentReturnValue on a
// leading count-verb question (so reconcileIntent has nothing to
// downgrade). The carve-out must still fire because the isMeasurement
// Scalar signal is question-level, not reconcile-event-level.
func TestBuildAnalysisIR_CountQuestionLLMPickedReturnValue(t *testing.T) {
	mut := types.NewMutableState("how many X are in this project")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "how many X are in this project",
		Intent:     types.IntentReturnValue, // LLM got it right — nothing to reconcile
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"how", "many", "X"},
			Entities: []string{"X"},
			Shape:    string(types.ShapeValue),
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if ir.AnswerContract.CitationReq.Required {
		t.Error("CitationReq.Required must be false when LLM picks return_value on a count question")
	}
	if got := ir.AnswerContract.CitationReq.MinCitations; got != 0 {
		t.Errorf("MinCitations = %d, want 0", got)
	}
	for _, a := range ir.AnswerContract.AcceptanceTests {
		if a.Kind == types.CritCitationCountGE {
			t.Errorf("AcceptanceTests still carries CritCitationCountGE: %+v", a)
		}
	}
	for _, n := range ir.TaskGraph.Nodes {
		for _, c := range n.SuccessCriteria {
			if c.Kind == types.CritCitationCountGE {
				t.Errorf("TaskNode %q SuccessCriteria still carries CritCitationCountGE: %+v", n.ID, c)
			}
		}
	}
}

// TestBuildAnalysisIR_ReturnValueWithoutCountCue_KeepsGates is the
// other negative control — a regular return_value question ("what
// does function F return") has a file:line to cite (the return
// statement), so the carve-out must NOT strip its gates.
func TestBuildAnalysisIR_ReturnValueWithoutCountCue_KeepsGates(t *testing.T) {
	mut := types.NewMutableState("what does function F return")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "what does function F return",
		Intent:     types.IntentReturnValue,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"function", "F", "return"},
			Entities: []string{"F"},
			Shape:    string(types.ShapeValue),
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}
	if !ir.AnswerContract.CitationReq.Required {
		t.Error("CitationReq.Required must stay true for non-count return_value questions")
	}
}

// TestBuildAnalysisIR_NonCountQuestionKeepsGates is the negative
// control — a regular enumerate question (no leading count-verb cue)
// must NOT have the citation gates stripped, otherwise the rule would
// over-fit and regress the list_of_symbols happy path.
func TestBuildAnalysisIR_NonCountQuestionKeepsGates(t *testing.T) {
	mut := types.NewMutableState("list all X that match Y")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "list all X that match Y",
		Intent:     types.IntentEnumerate,
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"list", "X", "Y"},
			Entities: []string{"X", "Y"},
			Shape:    string(types.ShapeListOfSymbols),
		},
	})
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	ir, err := buildAnalysisIR(ctx)
	if err != nil {
		t.Fatalf("buildAnalysisIR: %v", err)
	}

	if ir.RequestModel.Intent != types.IntentEnumerate {
		t.Errorf("Intent = %q, want enumerate (rule over-fired)", ir.RequestModel.Intent)
	}
	if !ir.AnswerContract.CitationReq.Required {
		t.Error("AnswerContract.CitationReq.Required must remain true for legit enumerate")
	}
	foundAT := false
	for _, a := range ir.AnswerContract.AcceptanceTests {
		if a.Kind == types.CritCitationCountGE {
			foundAT = true
		}
	}
	if !foundAT {
		t.Error("AcceptanceTests should still carry CritCitationCountGE on legit enumerate")
	}
	foundSC := false
	for _, n := range ir.TaskGraph.Nodes {
		for _, c := range n.SuccessCriteria {
			if c.Kind == types.CritCitationCountGE {
				foundSC = true
			}
		}
	}
	if !foundSC {
		t.Error("At least one TaskNode.SuccessCriteria should still carry CritCitationCountGE")
	}
}

// TestReconcileIntent covers the downgrade rule, the three hard gates
// (intent must be enumerate, complexity must be simple, leading clause
// must start with a count-verb cue), and a representative set of
// Chinese/English positives and would-be false positives.
func TestReconcileIntent(t *testing.T) {
	type row struct {
		name       string
		declared   types.Intent
		request    string
		complexity types.Complexity
		want       types.Intent
		wantReason bool
	}
	cases := []row{
		// ── Gate: non-enumerate intents pass through ─────────────
		{"explain passes through", types.IntentExplain,
			"统计 go 代码量", types.ComplexitySimple,
			types.IntentExplain, false},
		{"return_value passes through (rule never over-picks)", types.IntentReturnValue,
			"how many go files", types.ComplexitySimple,
			types.IntentReturnValue, false},
		{"root_cause passes through", types.IntentRootCause,
			"how many times does the login fail", types.ComplexitySimple,
			types.IntentRootCause, false},

		// ── Gate: non-simple complexity pass through ─────────────
		{"moderate enumerate is left alone", types.IntentEnumerate,
			"统计 go 代码量", types.ComplexityModerate,
			types.IntentEnumerate, false},
		{"complex enumerate is left alone", types.IntentEnumerate,
			"how many handlers do X and also list them", types.ComplexityComplex,
			types.IntentEnumerate, false},

		// ── Positives: Chinese count-verb prefixes ───────────────
		{"统计 prefix downgrades to return_value", types.IntentEnumerate,
			"统计一下这个项目下的 go 代码量", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"数一下 prefix downgrades", types.IntentEnumerate,
			"数一下有多少个 handler", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"有多少 prefix downgrades", types.IntentEnumerate,
			"有多少个 go 文件", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"一共 prefix downgrades", types.IntentEnumerate,
			"一共有多少 agent", types.ComplexitySimple,
			types.IntentReturnValue, true},

		// ── Positives: English count-verb prefixes ───────────────
		{"how many prefix downgrades", types.IntentEnumerate,
			"how many Go files are in this repo", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"count prefix downgrades", types.IntentEnumerate,
			"count the lines in main.go", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"number of prefix downgrades", types.IntentEnumerate,
			"number of registered agents", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"total lines prefix downgrades", types.IntentEnumerate,
			"total lines of Go code", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"size of prefix downgrades", types.IntentEnumerate,
			"size of the binary", types.ComplexitySimple,
			types.IntentReturnValue, true},

		// ── Politeness strip: leading "please / 请 / 帮我" ────────
		{"please-count prefix downgrades", types.IntentEnumerate,
			"please count the handlers", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"请统计 prefix downgrades", types.IntentEnumerate,
			"请统计 go 文件数量", types.ComplexitySimple,
			types.IntentReturnValue, true},
		{"帮我统计 prefix downgrades", types.IntentEnumerate,
			"帮我统计代码行数", types.ComplexitySimple,
			types.IntentReturnValue, true},

		// ── Negatives: cue appears mid-clause, NOT at start ──────
		{"list that mentions count mid-sentence is NOT downgraded", types.IntentEnumerate,
			"list all handlers that count requests", types.ComplexitySimple,
			types.IntentEnumerate, false},
		{"统计 as a noun mid-sentence is NOT downgraded", types.IntentEnumerate,
			"列出所有做 统计 的模块", types.ComplexitySimple,
			types.IntentEnumerate, false},
		{"which uses of total is NOT downgraded", types.IntentEnumerate,
			"which handlers use the total variable", types.ComplexitySimple,
			types.IntentEnumerate, false},
		{"where is size_of registered is NOT downgraded (not a prefix match)", types.IntentEnumerate,
			"where is size_of registered", types.ComplexitySimple,
			types.IntentEnumerate, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := reconcileIntent(c.declared, c.request, c.complexity)
			if got != c.want {
				t.Fatalf("intent = %q, want %q (request=%q)", got, c.want, c.request)
			}
			if c.wantReason && reason == "" {
				t.Fatalf("expected non-empty reason, got empty (request=%q)", c.request)
			}
			if !c.wantReason && reason != "" {
				t.Fatalf("expected empty reason, got %q (request=%q)", reason, c.request)
			}
		})
	}
}

// TestStripPolitenessPrefix covers the single-strip contract: the
// helper strips at most one leading politeness token, and returns
// the input unchanged when none applies.
func TestStripPolitenessPrefix(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"please count files", "count files"},
		{"could you count files", "count files"},
		{"can you count files", "count files"},
		{"请统计代码", "统计代码"},
		{"请帮我统计代码", "统计代码"},
		{"帮我统计代码", "统计代码"},
		{"麻烦统计代码", "统计代码"},
		{"count files", "count files"}, // identity
		{"", ""},                        // empty identity
	}
	for _, c := range cases {
		got := stripPolitenessPrefix(c.in)
		if got != c.want {
			t.Errorf("stripPolitenessPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHasLeadingCountVerb is a tight round-trip on the prefix matcher
// itself — separate from reconcileIntent so a future regression on
// the prefix set is easy to bisect.
func TestHasLeadingCountVerb(t *testing.T) {
	positives := []string{
		"统计 go 代码",
		"数一下文件",
		"how many go files",
		"count the lines",
		"number of agents",
		"total lines in main.go",
	}
	negatives := []string{
		"list handlers that count requests",
		"which files total more than 100 lines",
		"explain how many ways to configure",
		"where is count defined",
		"",
	}
	for _, p := range positives {
		if !hasLeadingCountVerb(p) {
			t.Errorf("expected positive, got false: %q", p)
		}
	}
	for _, n := range negatives {
		if hasLeadingCountVerb(n) {
			t.Errorf("expected negative, got true: %q", n)
		}
	}
}

// TestReconcileShape pins the two reconcile rules: conditional
// enumeration lift and config_value → value for source-code
// literals.
func TestReconcileShape(t *testing.T) {
	type row struct {
		name     string
		declared types.AnswerShape
		subject  types.AnswerSubject
		raw      string
		want     types.AnswerShape
		wantFire bool
	}
	cases := []row{
		// Rule 1: conditional-enumeration lift (value → list_of_symbols).
		{"有几个 + relational verb 调用", types.ShapeValue, types.AnswerSubject{},
			"有几个agent可以调用subagent", types.ShapeListOfSymbols, true},
		{"which + relational verb implement", types.ShapeValue, types.AnswerSubject{},
			"which handlers implement the Closer interface", types.ShapeListOfSymbols, true},
		{"config_value + enumeration cue + relational verb", types.ShapeConfigValue,
			types.AnswerSubject{Kind: types.SubjectFunctionName},
			"哪些 agent 注册了 propose_sub_agents", types.ShapeListOfSymbols, true},
		// Rule 1 negatives.
		{"有多少 without relational verb stays value", types.ShapeValue, types.AnswerSubject{},
			"有多少 python 文件", types.ShapeValue, false},
		{"relational verb without enumeration cue stays value", types.ShapeValue, types.AnswerSubject{},
			"explain how the router calls handlers", types.ShapeValue, false},
		{"non-value shape untouched", types.ShapeExplanation, types.AnswerSubject{},
			"有几个 agent 可以调用 subagent", types.ShapeExplanation, false},
		// Rule 2: config_value → value for source-code literal subjects.
		{"config_value + function subject → value", types.ShapeConfigValue,
			types.AnswerSubject{Kind: types.SubjectFunctionName},
			"what function binds the handler", types.ShapeValue, true},
		{"config_value + type subject → value", types.ShapeConfigValue,
			types.AnswerSubject{Kind: types.SubjectTypeName},
			"what is the receiver type", types.ShapeValue, true},
		{"config_value + return subject → value", types.ShapeConfigValue,
			types.AnswerSubject{Kind: types.SubjectReturnValue},
			"what does Name return", types.ShapeValue, true},
		// Rule 2 negatives.
		{"config_value + config-key subject stays", types.ShapeConfigValue,
			types.AnswerSubject{Kind: types.SubjectConfigKey},
			"what is log_dir set to", types.ShapeConfigValue, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := reconcileShape(c.declared, c.subject, c.raw)
			if got != c.want {
				t.Errorf("shape = %q, want %q (reason=%q)", got, c.want, reason)
			}
			if c.wantFire && reason == "" {
				t.Errorf("expected rule to fire with a non-empty reason")
			}
			if !c.wantFire && reason != "" {
				t.Errorf("unexpected fire: reason=%q", reason)
			}
		})
	}
}
