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
	res := runContractCheck(out, types.AnswerContract{}, nil)
	if !res.Passed {
		t.Errorf("empty contract should pass; got %+v", res)
	}
}

func TestRunContractCheck_NilOutputPasses(t *testing.T) {
	res := runContractCheck(nil, types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
	}, nil)
	if !res.Passed {
		t.Errorf("nil output should pass (caller decides separately); got %+v", res)
	}
}

func TestRunContractCheck_FailingShape(t *testing.T) {
	out := &agent.StageOutput{FinalAnswer: "no bullets here"}
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
	}
	res := runContractCheck(out, c, nil)
	if res.Passed {
		t.Errorf("plain prose should fail list_of_symbols shape")
	}
	if len(res.Violations) == 0 {
		t.Error("want at least one violation")
	}
}

func TestRunContractCheck_CitationGap_SoftDegrade(t *testing.T) {
	// Contract requires 3 file_line citations; answer has 1.
	// Soft degradation: 1 > 0 → pass.
	out := &agent.StageOutput{
		FinalAnswer: "- foo\n- bar\nSee internal/agent/explorer.go:42",
	}
	c := types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 3,
		},
	}
	res := runContractCheck(out, c, nil)
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
		RequiredAnswerShape: types.ShapeListOfSymbols,
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 3,
		},
	}
	res := runContractCheck(out, c, nil)
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
		RequiredAnswerShape: types.ShapeListOfSymbols,
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 2,
		},
	}
	res := runContractCheck(out, c, nil)
	if !res.Passed {
		t.Errorf("expected pass, got violations: %+v", res.Violations)
	}
}

func TestRenderViolations_Ordering(t *testing.T) {
	res := contract.Result{
		Passed: false,
		Violations: []contract.Violation{
			{Kind: contract.ViolShape, Detail: "no bullets"},
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

func TestAppendViolationsToAnswer_FailLoudPattern(t *testing.T) {
	original := "the answer body"
	res := contract.Result{Passed: false, Violations: []contract.Violation{
		{Kind: contract.ViolShape, Detail: "wrong"},
	}}
	out := appendViolationsToAnswer(original, res)
	if !strings.HasPrefix(out, "⚠️ answer-contract validation exhausted") {
		t.Errorf("want fail-loud prefix; got %q", out)
	}
	if !strings.Contains(out, original) {
		t.Errorf("want original answer preserved beneath warning; got %q", out)
	}
}

func TestAppendViolationsToAnswer_PassedKeepsOriginal(t *testing.T) {
	original := "good answer"
	res := contract.Result{Passed: true}
	if got := appendViolationsToAnswer(original, res); got != original {
		t.Errorf("passed: want original unchanged; got %q", got)
	}
}

// TestIsJustifiedAbsenceAnswer_ShapeTieredGate pins the shape-tiered
// trust check: shallow shapes pass on any single investigation tool,
// deep shapes require a content read. Zero-tool or wrong-shape
// answers never pass.
func TestIsJustifiedAbsenceAnswer_ShapeTieredGate(t *testing.T) {
	mkMut := func(doc *types.AnswerDocument, toolResults []types.ToolResult) *types.MutableState {
		m := types.NewMutableState("")
		m.SetAnswerDocument(doc)
		m.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: toolResults})
		return m
	}
	absentValue := &types.AnswerDocument{
		Shape: types.ShapeValue,
		Value: &types.AnswerValue{Literal: "0", CitationRef: -1},
	}
	absentList := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessComplete,
	}
	cases := []struct {
		name string
		doc  *types.AnswerDocument
		tool types.ToolResult
		want bool
	}{
		// Shallow shape — any investigation tool is enough.
		{"value + list_files", absentValue,
			types.ToolResult{ToolName: "list_files", Success: true}, true},
		{"value + exec_command", absentValue,
			types.ToolResult{ToolName: "exec_command", Success: true, Summary: "0"}, true},
		{"value + grep files_only", absentValue,
			types.ToolResult{ToolName: "grep", Success: true, Summary: "[grep: 0 matching files]"}, true},

		// Deep shape — need content read.
		{"list_of_symbols + read_file OK", absentList,
			types.ToolResult{ToolName: "read_file", Success: true, Summary: "file content"}, true},
		{"list_of_symbols + content grep OK", absentList,
			types.ToolResult{ToolName: "grep", Success: true, Summary: "file.go:1: found line"}, true},
		{"list_of_symbols + only list_files REJECT", absentList,
			types.ToolResult{ToolName: "list_files", Success: true}, false},
		{"list_of_symbols + only grep files_only REJECT", absentList,
			types.ToolResult{ToolName: "grep", Success: true, Summary: "[grep: 3 matching files]"}, false},

		// Universal: no tools at all fails both shape tiers.
		{"value + no tools REJECT", absentValue,
			types.ToolResult{}, false},
		// Non-investigation tool: emit_evidence does not count.
		{"value + only emit_evidence REJECT", absentValue,
			types.ToolResult{ToolName: "emit_evidence", Success: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var trs []types.ToolResult
			if c.tool.ToolName != "" {
				trs = []types.ToolResult{c.tool}
			}
			mut := mkMut(c.doc, trs)
			if got := isJustifiedAbsenceAnswer(mut); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestIsJustifiedAbsenceAnswer_ShapeCheck pins shape detection:
// lower_bound completeness is not an absence claim, non-zero literals
// are not absence, boolean true is not absence.
func TestIsJustifiedAbsenceAnswer_ShapeCheck(t *testing.T) {
	mkMut := func(doc *types.AnswerDocument) *types.MutableState {
		m := types.NewMutableState("")
		m.SetAnswerDocument(doc)
		m.SetTurnAArtifacts(types.TurnAArtifacts{
			ToolResults: []types.ToolResult{{ToolName: "list_files", Success: true}},
		})
		return m
	}
	cases := []struct {
		name string
		doc  *types.AnswerDocument
		want bool
	}{
		{"value=0 accepts", &types.AnswerDocument{
			Shape: types.ShapeValue, Value: &types.AnswerValue{Literal: "0", CitationRef: -1}}, true},
		{"value=none accepts", &types.AnswerDocument{
			Shape: types.ShapeValue, Value: &types.AnswerValue{Literal: "none", CitationRef: -1}}, true},
		{"value=42 rejects", &types.AnswerDocument{
			Shape: types.ShapeValue, Value: &types.AnswerValue{Literal: "42", CitationRef: -1}}, false},
		{"boolean=false accepts", &types.AnswerDocument{
			Shape: types.ShapeBoolean, Boolean: &types.AnswerBoolean{Decision: false, CitationRef: -1}}, true},
		{"boolean=true rejects", &types.AnswerDocument{
			Shape: types.ShapeBoolean, Boolean: &types.AnswerBoolean{Decision: true, CitationRef: -1}}, false},
		{"empty symbols + complete accepts", &types.AnswerDocument{
			Shape: types.ShapeListOfSymbols, SymbolsCompleteness: types.CompletenessComplete}, false}, // rejected here because list_files alone is not content-read for deep shape
		{"non-empty symbols rejects", &types.AnswerDocument{
			Shape:   types.ShapeListOfSymbols,
			Symbols: []types.AnswerSymbol{{Name: "Foo", File: "a.go", Line: 1}},
			SymbolsCompleteness: types.CompletenessComplete}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isJustifiedAbsenceAnswer(mkMut(c.doc)); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
