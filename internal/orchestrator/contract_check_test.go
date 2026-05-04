package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// contract_check_test.go locks the orchestrator's contract-checker
// glue: citation extraction, the violation-rendering helper, and the
// fail-loud answer-augmentation pattern. The contract checker logic
// itself has its own tests under internal/analysis/contract.

func TestExtractCitations_BasicFileLine(t *testing.T) {
	text := `The handler is dispatched in internal/agent/explorer.go:1234 and
returns through internal/agent/finalizer.go:567.`
	cits := extractCitationsFromAnswer(text)
	if len(cits) != 2 {
		t.Fatalf("want 2 citations, got %d: %+v", len(cits), cits)
	}
	if cits[0].File != "internal/agent/explorer.go" || cits[0].Line != 1234 {
		t.Errorf("cit[0]: %+v", cits[0])
	}
	if cits[1].File != "internal/agent/finalizer.go" || cits[1].Line != 567 {
		t.Errorf("cit[1]: %+v", cits[1])
	}
}

func TestExtractCitations_RangeForm(t *testing.T) {
	text := "see file.go:100-150 for the full block."
	cits := extractCitationsFromAnswer(text)
	if len(cits) != 1 {
		t.Fatalf("want 1, got %d", len(cits))
	}
	if cits[0].File != "file.go" || cits[0].Line != 100 {
		t.Errorf("file/line: %+v", cits[0])
	}
	if len(cits[0].Lines) != 2 || cits[0].Lines[0] != 100 || cits[0].Lines[1] != 150 {
		t.Errorf("Lines: %+v", cits[0].Lines)
	}
}

func TestExtractCitations_DeduplicatesIdenticalRefs(t *testing.T) {
	text := "x.go:5 ... x.go:5 again ... x.go:5 once more"
	cits := extractCitationsFromAnswer(text)
	if len(cits) != 1 {
		t.Errorf("want 1 dedup'd citation, got %d", len(cits))
	}
}

func TestExtractCitations_IgnoresProseColons(t *testing.T) {
	// "step 3:" and "10:30" must not register as citations because
	// neither has a `path.ext:line` shape.
	text := "Step 3: at 10:30 we observed nothing."
	cits := extractCitationsFromAnswer(text)
	if len(cits) != 0 {
		t.Errorf("prose colons should not match; got %+v", cits)
	}
}

func TestExtractCitations_BareFileWithExtension(t *testing.T) {
	// README.md:1 is a legitimate citation (no slash but real
	// extension + line number).
	text := "see README.md:1 for context"
	cits := extractCitationsFromAnswer(text)
	if len(cits) != 1 {
		t.Fatalf("want 1, got %d: %+v", len(cits), cits)
	}
	if cits[0].File != "README.md" || cits[0].Line != 1 {
		t.Errorf("got %+v", cits[0])
	}
}

func TestExtractCitations_EmptyText(t *testing.T) {
	if cits := extractCitationsFromAnswer(""); cits != nil {
		t.Errorf("empty text should yield nil, got %+v", cits)
	}
}

func TestRunContractCheck_EmptyContractAlwaysPasses(t *testing.T) {
	out := &agent.StageOutput{FinalAnswer: "anything goes"}
	res := runContractCheck(out, types.AnswerContract{}, nil, nil)
	if !res.Passed {
		t.Errorf("empty contract should pass; got %+v", res)
	}
}

func TestRunContractCheck_NilOutputPasses(t *testing.T) {
	res := runContractCheck(nil, types.AnswerContract{
	}, nil, nil)
	if !res.Passed {
		t.Errorf("nil output should pass (caller decides separately); got %+v", res)
	}
}

// V1 shape-text-heuristic check (checkShape) has been retired per
// docs/migration/answer_shape_retirement.md; the V2 block oracles in
// contract_check_block.go own the read-mode block contract on the
// typed AnswerDocumentV2 carrier, not on rendered prose. The
// TestRunContractCheck_FailingShape coverage moves to V2 block-level
// tests in contract_check_block_test.go.

func TestRunContractCheck_CitationGap_SoftDegrade(t *testing.T) {
	// Contract requires 3 file_line citations; answer has 1.
	// Soft degradation: 1 > 0 → pass.
	out := &agent.StageOutput{
		FinalAnswer: "- foo\n- bar\nSee internal/agent/explorer.go:42",
	}
	c := types.AnswerContract{
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 3,
		},
	}
	res := runContractCheck(out, c, nil, nil)
	if !res.Passed {
		t.Errorf("1 citation with min=3 should soft-pass; got violations: %+v", res.Violations)
	}
}

func TestRunContractCheck_CitationGap_ZeroRejects(t *testing.T) {
	// Contract requires 3 file_line citations; answer has 0 → reject.
	out := &agent.StageOutput{
		FinalAnswer: "- foo\n- bar\nNo citations at all",
	}
	c := types.AnswerContract{
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 3,
		},
	}
	res := runContractCheck(out, c, nil, nil)
	if res.Passed {
		t.Error("0 citations should hard-reject")
	}
}

func TestRunContractCheck_PassingCase(t *testing.T) {
	out := &agent.StageOutput{
		FinalAnswer: `- ` + "`" + `Foo` + "`" + ` at internal/a.go:1
- ` + "`" + `Bar` + "`" + ` at internal/b.go:2`,
	}
	c := types.AnswerContract{
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 2,
		},
	}
	res := runContractCheck(out, c, nil, nil)
	if !res.Passed {
		t.Errorf("expected pass, got violations: %+v", res.Violations)
	}
}

func TestRenderViolations_Ordering(t *testing.T) {
	res := contract.Result{
		Passed: false,
		Violations: []contract.Violation{
			{Kind: contract.ViolFamilyMismatch, Detail: "no bullets"},
			{Kind: contract.ViolCitation, Detail: "0/3 citations", Repair: "add 3 anchors"},
		},
	}
	got := renderViolations(res)
	if !strings.Contains(got, "no bullets") || !strings.Contains(got, "0/3 citations") {
		t.Errorf("missing details: %q", got)
	}
	if !strings.Contains(got, "add 3 anchors") {
		t.Errorf("repair missing: %q", got)
	}
	if !strings.Contains(got, ";") {
		t.Errorf("expected joiner; got %q", got)
	}
}

func TestRenderViolations_EmptyOnPassed(t *testing.T) {
	if got := renderViolations(contract.Result{Passed: true}); got != "" {
		t.Errorf("passed result should yield empty; got %q", got)
	}
}

// TestFormatViolationsForLogger_FailLoudPattern verifies the digest
// formatter (renamed from appendViolationsToAnswer in B4-F3,
// 2026-05-04). The function now returns the digest string only;
// the channel decision (user panel vs operator log) lives in
// orchestrator.go::applyContractViolations.
func TestFormatViolationsForLogger_FailLoudPattern(t *testing.T) {
	res := contract.Result{Passed: false, Violations: []contract.Violation{
		{Kind: contract.ViolFamilyMismatch, Detail: "wrong"},
	}}
	got := formatViolationsForLogger(res)
	if !strings.HasPrefix(got, "· answer-contract validation exhausted") {
		t.Errorf("want digest prefix; got %q", got)
	}
	if !strings.Contains(got, "wrong") {
		t.Errorf("want violation detail in digest; got %q", got)
	}
}

func TestFormatViolationsForLogger_PassedReturnsEmpty(t *testing.T) {
	if got := formatViolationsForLogger(contract.Result{Passed: true}); got != "" {
		t.Errorf("passed: want empty digest; got %q", got)
	}
}

func TestFormatViolationsForLogger_NoViolationsReturnsEmpty(t *testing.T) {
	if got := formatViolationsForLogger(contract.Result{Passed: false}); got != "" {
		t.Errorf("no violations: want empty digest; got %q", got)
	}
}

// TestIsJustifiedAbsenceAnswer_ShapeTieredGate pins the shape-tiered
// trust check. Shallow shapes (value / boolean / config_value) and
// EMPTY list_of_symbols pass on any single investigation tool. Only
// NON-empty list_of_symbols (behaviour-enumeration positive answers)
// require a content read — and those are not absence cases. Zero-tool
// runs never pass.
//