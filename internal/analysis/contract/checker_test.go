package contract

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hasViolation(r Result, kind string) bool {
	for _, v := range r.Violations {
		if v.Kind == kind {
			return true
		}
	}
	return false
}

func TestCheck_EmptyContract_AlwaysPasses(t *testing.T) {
	r := Check(Answer{Text: "anything"}, types.AnswerContract{})
	if !r.Passed {
		t.Fatalf("zero contract should never reject; got %+v", r)
	}
}

func TestCheck_Boolean_Shape(t *testing.T) {
	c := types.AnswerContract{RequiredAnswerShape: types.ShapeBoolean}
	if !Check(Answer{Text: "Yes, it does."}, c).Passed {
		t.Fatal("'Yes...' should pass boolean shape")
	}
	if !Check(Answer{Text: "是的"}, c).Passed {
		t.Fatal("Chinese 是 should pass boolean shape")
	}
	if Check(Answer{Text: "It depends on ..."}, c).Passed {
		t.Fatal("non-boolean leading should fail")
	}
}

func TestCheck_Value_ShapeTooLong(t *testing.T) {
	c := types.AnswerContract{RequiredAnswerShape: types.ShapeValue}
	long := strings.Repeat("x", 600)
	if Check(Answer{Text: long}, c).Passed {
		t.Fatal("long value answer should fail")
	}
	if !Check(Answer{Text: "42"}, c).Passed {
		t.Fatal("'42' should pass value shape")
	}
}

func TestCheck_Value_EmptyRejected(t *testing.T) {
	c := types.AnswerContract{RequiredAnswerShape: types.ShapeValue}
	if Check(Answer{Text: "   "}, c).Passed {
		t.Fatal("empty value answer should fail")
	}
}

func TestCheck_ListOfSymbols_Shape(t *testing.T) {
	c := types.AnswerContract{RequiredAnswerShape: types.ShapeListOfSymbols}
	bulleted := "- Foo\n- Bar\n- Baz"
	if !Check(Answer{Text: bulleted}, c).Passed {
		t.Fatal("bulleted list should pass")
	}
	fenced := "The answer is `Foo` and `Bar`."
	if !Check(Answer{Text: fenced}, c).Passed {
		t.Fatal("fenced identifiers should pass")
	}
	prose := "The explorer stops when it runs out of facts."
	if Check(Answer{Text: prose}, c).Passed {
		t.Fatal("prose without list markers should fail")
	}
}

func TestCheck_StepList_RequiresMultipleNumbered(t *testing.T) {
	c := types.AnswerContract{RequiredAnswerShape: types.ShapeStepList}
	good := "1. First locate the file.\n2. Then read the symbol.\n3. Finally validate."
	if !Check(Answer{Text: good}, c).Passed {
		t.Fatal("numbered steps should pass")
	}
	// Only one numbered item — not a step list.
	oneStep := "1. Just do this."
	if Check(Answer{Text: oneStep}, c).Passed {
		t.Fatal("single numbered line should not pass step_list")
	}
}

func TestCheck_ConfigValue_Shape(t *testing.T) {
	c := types.AnswerContract{RequiredAnswerShape: types.ShapeConfigValue}
	if !Check(Answer{Text: "default_agent = explorer"}, c).Passed {
		t.Fatal("key=value should pass")
	}
	if !Check(Answer{Text: "log_level: debug"}, c).Passed {
		t.Fatal("key: value should pass")
	}
	if Check(Answer{Text: "It's some config"}, c).Passed {
		t.Fatal("prose should fail config_value shape")
	}
}

func TestCheck_Explanation_TooShort(t *testing.T) {
	c := types.AnswerContract{RequiredAnswerShape: types.ShapeExplanation}
	if Check(Answer{Text: "short"}, c).Passed {
		t.Fatal("5-char explanation should fail")
	}
	if !Check(Answer{Text: "The explorer decides to stop when ERM conditions are satisfied."}, c).Passed {
		t.Fatal("substantive explanation should pass")
	}
}

func TestCheck_Citations_MinCount(t *testing.T) {
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
	}
	a := Answer{
		Text:      "Long enough explanation that passes the shape check because it is many characters.",
		Citations: []Citation{{File: "a.go", Line: 10}},
	}
	r := Check(a, c)
	if r.Passed {
		t.Fatal("1 citation should fail min=2")
	}
	if !hasViolation(r, "citation") {
		t.Fatal("should emit citation violation")
	}
}

func TestCheck_Citations_Granularity(t *testing.T) {
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
	}
	a := Answer{
		Text:      "Long enough explanation that passes the shape check because it is many characters.",
		Citations: []Citation{{File: "a.go", Line: 0}},
	}
	r := Check(a, c)
	if r.Passed {
		t.Fatal("zero line number must fail file_line granularity")
	}
}

func TestCheck_MustInclude(t *testing.T) {
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		MustInclude:         []string{"Explorer", "ShouldStop"},
	}
	a := Answer{Text: "The Explorer uses ShouldStop when evidence is sufficient. This is a longer answer."}
	if !Check(a, c).Passed {
		t.Fatal("answer containing both required terms should pass")
	}
	a2 := Answer{Text: "The Explorer stops when it's done, which is a long enough explanation."}
	r := Check(a2, c)
	if r.Passed {
		t.Fatal("missing ShouldStop should fail")
	}
	if !hasViolation(r, "must_include") {
		t.Fatal("should emit must_include violation")
	}
}

func TestCheck_MustExclude(t *testing.T) {
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		MustExclude:         []string{"TODO", "FIXME"},
	}
	a := Answer{Text: "This is a complete answer without forbidden tokens and of sufficient length."}
	if !Check(a, c).Passed {
		t.Fatal("clean answer should pass")
	}
	a2 := Answer{Text: "This answer has a TODO which is forbidden and of sufficient length to pass shape."}
	r := Check(a2, c)
	if r.Passed {
		t.Fatal("forbidden token must fail")
	}
	if !hasViolation(r, "must_exclude") {
		t.Fatal("should emit must_exclude violation")
	}
}

func TestCheck_Acceptance_ContainsSymbol(t *testing.T) {
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		AcceptanceTests:     []types.Criterion{{Kind: types.CritContainsSymbol, Expr: "rollback"}},
	}
	good := Answer{Text: "The plan includes a rollback strategy described here at length sufficient to pass."}
	if !Check(good, c).Passed {
		t.Fatal("contains_symbol should pass")
	}
	bad := Answer{Text: "The plan is irreversible but long enough to pass the shape check threshold entirely."}
	if Check(bad, c).Passed {
		t.Fatal("missing 'rollback' should fail acceptance")
	}
}

func TestCheck_Acceptance_RegexMatch(t *testing.T) {
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		AcceptanceTests:     []types.Criterion{{Kind: types.CritRegexMatch, Expr: `\b[A-Z][a-zA-Z]+Agent\b`}},
	}
	good := Answer{Text: "The ExplorerAgent handles the explore stage with sufficient context for shape check."}
	if !Check(good, c).Passed {
		t.Fatal("regex_match should pass with ExplorerAgent")
	}
	bad := Answer{Text: "The system runs the explore stage with sufficient context for the shape check entirely."}
	if Check(bad, c).Passed {
		t.Fatal("regex_match should fail when pattern absent")
	}
}

func TestCheck_Acceptance_CitationCountGE(t *testing.T) {
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		AcceptanceTests:     []types.Criterion{{Kind: types.CritCitationCountGE, Expr: "3"}},
	}
	a := Answer{
		Text:      "Long enough explanation that passes the shape check because it is many characters.",
		Citations: []Citation{{File: "a", Line: 1}, {File: "b", Line: 1}},
	}
	if Check(a, c).Passed {
		t.Fatal("2 citations should fail ≥3 acceptance")
	}
	a.Citations = append(a.Citations, Citation{File: "c", Line: 1})
	if !Check(a, c).Passed {
		t.Fatal("3 citations should pass ≥3 acceptance")
	}
}

func TestCheck_MultipleViolationsReported(t *testing.T) {
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		MustInclude:         []string{"Explorer"},
		CitationReq:         types.CitationReq{Required: true, MinCitations: 1},
	}
	a := Answer{Text: "just prose, no list markers, no Explorer mentioned, no citations"}
	r := Check(a, c)
	if r.Passed {
		t.Fatal("multi-failure answer must not pass")
	}
	if len(r.Violations) < 2 {
		t.Fatalf("expected multiple violations; got %+v", r.Violations)
	}
}

func TestCheck_RepairHintEmittedForRecoverable(t *testing.T) {
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		MustInclude:         []string{"Explorer"},
	}
	a := Answer{Text: "This answer is long enough to pass the shape check threshold but lacks a must-include term."}
	r := Check(a, c)
	if r.Passed {
		t.Fatal("should fail")
	}
	foundRepair := false
	for _, v := range r.Violations {
		if v.Kind == "must_include" && strings.Contains(v.Repair, "Explorer") {
			foundRepair = true
		}
	}
	if !foundRepair {
		t.Fatalf("repair hint missing; violations=%+v", r.Violations)
	}
}
