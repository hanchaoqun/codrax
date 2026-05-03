package contract

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hasViolation(r Result, kind ViolationKind) bool {
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

// V1 shape-text-heuristic checks (boolean prefix detection, list-bullet
// regex, numbered-step parser, config-value pair detection,
// explanation min-length floor) have been retired with checkShape per
// docs/migration/answer_shape_retirement.md. The V2 block oracles in
// internal/orchestrator/contract_check_block.go own these obligations
// against the typed AnswerDocumentV2 carrier; contract.Check remains
// only for citation / acceptance / pre-flight checks.

func TestCheck_Citations_MinCount_SoftDegrade(t *testing.T) {
	// 1 citation with min=2: soft degradation accepts because
	// citations > 0. Only 0 citations is a hard reject.
	c := types.AnswerContract{
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
	}
	a := Answer{
		Text:      "Long enough explanation that passes the shape check because it is many characters.",
		Citations: []Citation{{File: "a.go", Line: 10}},
	}
	r := Check(a, c)
	if !r.Passed {
		t.Fatal("1 citation with min=2 should soft-pass (>0 citations present)")
	}
}

func TestCheck_Citations_MinCount_ZeroRejects(t *testing.T) {
	// 0 citations with min=2: hard reject.
	c := types.AnswerContract{
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
	}
	a := Answer{
		Text:      "Long enough explanation that passes the shape check because it is many characters.",
		Citations: nil,
	}
	r := Check(a, c)
	if r.Passed {
		t.Fatal("0 citations should hard-reject")
	}
	if !hasViolation(r, ViolCitation) {
		t.Fatal("should emit citation violation")
	}
}

func TestCheck_Citations_Granularity(t *testing.T) {
	c := types.AnswerContract{
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
	if !hasViolation(r, ViolMustInclude) {
		t.Fatal("should emit must_include violation")
	}
}

func TestCheck_MustExclude(t *testing.T) {
	c := types.AnswerContract{
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
	if !hasViolation(r, ViolMustExclude) {
		t.Fatal("should emit must_exclude violation")
	}
}

func TestCheck_Acceptance_ContainsSymbol(t *testing.T) {
	c := types.AnswerContract{
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

func TestCheck_Acceptance_CitationCountGE_SoftDegrade(t *testing.T) {
	c := types.AnswerContract{
		AcceptanceTests:     []types.Criterion{{Kind: types.CritCitationCountGE, Expr: "3"}},
	}
	// 2 citations with threshold 3: soft-pass (>0).
	a := Answer{
		Text:      "Long enough explanation that passes the shape check because it is many characters.",
		Citations: []Citation{{File: "a", Line: 1}, {File: "b", Line: 1}},
	}
	if !Check(a, c).Passed {
		t.Fatal("2 citations with threshold 3 should soft-pass (>0 present)")
	}
	// 3 citations: hard pass.
	a.Citations = append(a.Citations, Citation{File: "c", Line: 1})
	if !Check(a, c).Passed {
		t.Fatal("3 citations should pass ≥3 acceptance")
	}
}

func TestCheck_Acceptance_CitationCountGE_ZeroRejects(t *testing.T) {
	c := types.AnswerContract{
		AcceptanceTests:     []types.Criterion{{Kind: types.CritCitationCountGE, Expr: "3"}},
	}
	a := Answer{
		Text:      "Long enough explanation that passes the shape check because it is many characters.",
		Citations: nil,
	}
	if Check(a, c).Passed {
		t.Fatal("0 citations should hard-reject")
	}
}

func TestCheck_MultipleViolationsReported(t *testing.T) {
	c := types.AnswerContract{
		MustInclude: []string{"Explorer"},
		CitationReq: types.CitationReq{Required: true, MinCitations: 1},
	}
	a := Answer{Text: "just prose without the required term, no citations"}
	r := Check(a, c)
	if r.Passed {
		t.Fatal("multi-failure answer must not pass")
	}
	if len(r.Violations) < 2 {
		t.Fatalf("expected multiple violations (must_include + citation); got %+v", r.Violations)
	}
}

func TestCheck_RepairHintEmittedForRecoverable(t *testing.T) {
	c := types.AnswerContract{
		MustInclude:         []string{"Explorer"},
	}
	a := Answer{Text: "This answer is long enough to pass the shape check threshold but lacks a must-include term."}
	r := Check(a, c)
	if r.Passed {
		t.Fatal("should fail")
	}
	foundRepair := false
	for _, v := range r.Violations {
		if v.Kind == ViolMustInclude && strings.Contains(v.Repair, "Explorer") {
			foundRepair = true
		}
	}
	if !foundRepair {
		t.Fatalf("repair hint missing; violations=%+v", r.Violations)
	}
}
