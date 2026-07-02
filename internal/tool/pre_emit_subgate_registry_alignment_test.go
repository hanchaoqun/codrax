// Package tool — pre_emit_subgate_registry_alignment_test.go (F5,
// 2026-07-03).
//
// Registry-alignment scan suite for the pre-emit chokepoint. Pins the
// CURRENT routing state of every subgate in runPreEmitChecksWithContext
// against the declarative preEmitSubgateRouteTable and the central
// ViolKindSpec registry:
//
//   - every routed kind is registered (closes the unregistered →
//     legacy-Medium → hard-by-accident fallback lane structurally);
//   - advisory rows route advisory and hard-lane rows route hard
//     through the REAL preEmitHintHardByDefault;
//   - the table's kind set equals the kinds actually passed to
//     appendPreEmitHints/tagPreEmitHints in the checker body
//     (go/parser scan — self-updating, no manual sync);
//   - exactly one ForceHard producer site exists in the checker file.
//
// Ping-pong guard: the advisory rows pin the settled post-D1-G95
// state (D1-F7w hardened citation carriers, D1-G95 reverted them).
// Do NOT "fix" an advisory row to hard here — that would be round
// three. Only the two typed policy-row lanes are hard.
//
// The go/parser scans are TEST-ONLY; production code never inspects
// source text.
package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const preEmitCheckerSourceFile = "answer_document_pre_emit_check.go"

func TestPreEmitSubgateRouteTableKindsRegistered(t *testing.T) {
	rows := preEmitSubgateRouteTable()
	if len(rows) == 0 {
		t.Fatal("route table must not be empty")
	}
	seenSubgate := map[string]bool{}
	for _, row := range rows {
		if strings.TrimSpace(row.Subgate) == "" {
			t.Fatalf("route row with empty subgate name: %+v", row)
		}
		if seenSubgate[row.Subgate] {
			t.Fatalf("duplicate subgate name %q", row.Subgate)
		}
		seenSubgate[row.Subgate] = true
		if _, ok := types.ViolKindSpecFor(row.ViolationKind); !ok {
			t.Errorf("subgate %q routes kind %q which is NOT registered in ViolKindSpec — the unregistered fallback lane must stay structurally unreachable", row.Subgate, row.ViolationKind)
		}
	}
}

// Every advisory row must route advisory through the REAL gate split;
// every hard-lane row must (a) route hard for its typed exception
// shape and (b) name a lane present in preEmitSameTurnHardPolicyRows.
func TestPreEmitSubgateRouteTableRoutesMatchGateSplit(t *testing.T) {
	policyRows := map[preEmitSameTurnHardPolicyRow]bool{}
	for _, row := range preEmitSameTurnHardPolicyRows() {
		policyRows[row] = true
	}
	for _, row := range preEmitSubgateRouteTable() {
		plain := emitFixHint{
			Kind:          row.ViolationKind,
			Field:         "test/" + row.Subgate,
			ExpectedShape: "synthetic",
			Reason:        "route-table alignment probe",
		}
		switch row.HardLane {
		case "":
			if preEmitHintHardByDefault(plain) {
				t.Errorf("subgate %q (kind %q) is declared advisory but a plain tagged hint routes HARD", row.Subgate, row.ViolationKind)
			}
		case preEmitHardSignalTypedRequiredBlockKind:
			if !policyRows[preEmitSameTurnHardPolicyRow{Kind: row.ViolationKind, Signal: row.HardLane}] {
				t.Errorf("subgate %q hard lane %q has no policy row", row.Subgate, row.HardLane)
			}
			// The typed exception shape (required diagram block).
			typed := plain
			typed.ExpectedBlockKinds = []types.AnswerBlockKind{types.BlockDiagram}
			if !preEmitHintHardByDefault(typed) {
				t.Errorf("subgate %q typed required-block hint must route hard", row.Subgate)
			}
			// Without the typed shape the same kind stays advisory.
			if preEmitHintHardByDefault(plain) {
				t.Errorf("subgate %q kind %q without typed block kinds must stay advisory", row.Subgate, row.ViolationKind)
			}
		case preEmitHardSignalCompletePrincipalMemberSet:
			if !policyRows[preEmitSameTurnHardPolicyRow{Kind: row.ViolationKind, Signal: row.HardLane}] {
				t.Errorf("subgate %q hard lane %q has no policy row", row.Subgate, row.HardLane)
			}
			forced := plain
			forced.ForceHard = true
			if !preEmitHintHardByDefault(forced) {
				t.Errorf("subgate %q ForceHard member-set hint must route hard", row.Subgate)
			}
			if preEmitHintHardByDefault(plain) {
				t.Errorf("subgate %q kind %q without ForceHard must stay advisory", row.Subgate, row.ViolationKind)
			}
		default:
			t.Errorf("subgate %q declares unknown hard lane %q", row.Subgate, row.HardLane)
		}
	}
}

// The table's kind set must equal the set of types.Viol* identifiers
// the checker body actually passes to appendPreEmitHints /
// tagPreEmitHints — a new call site (or a removed one) fails this
// test until the table is updated, so the two can never drift.
func TestPreEmitSubgateRouteTableMatchesCheckerBody(t *testing.T) {
	scanned := scanPreEmitCheckerRoutedKindIdents(t)
	if len(scanned) == 0 {
		t.Fatal("scan found no routed kinds — parser drift?")
	}
	kindValueByIdent := violationKindValuesByIdent(t)
	scannedValues := map[types.ViolationKind]bool{}
	for ident := range scanned {
		value, ok := kindValueByIdent[ident]
		if !ok {
			t.Fatalf("checker body routes types.%s which is not a declared ViolationKind constant", ident)
		}
		scannedValues[value] = true
	}
	tableValues := map[types.ViolationKind]bool{}
	for _, row := range preEmitSubgateRouteTable() {
		tableValues[row.ViolationKind] = true
	}
	for kind := range scannedValues {
		if !tableValues[kind] {
			t.Errorf("checker body routes kind %q but preEmitSubgateRouteTable has no row for it — add the row (and keep it advisory unless a typed policy row exists)", kind)
		}
	}
	for kind := range tableValues {
		if !scannedValues[kind] {
			t.Errorf("preEmitSubgateRouteTable declares kind %q but no checker call site routes it — remove the stale row", kind)
		}
	}
}

// Exactly ONE ForceHard producer site may exist in the checker file:
// the complete-principal-member-set hint. A second producer would
// widen the same-turn hard surface outside the policy-row governance.
func TestPreEmitForceHardSingleProducerSite(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, preEmitCheckerSourceFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", preEmitCheckerSourceFile, err)
	}
	producers := 0
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "ForceHard" {
				producers++
			}
		}
		return true
	})
	if producers != 1 {
		t.Fatalf("expected exactly 1 ForceHard producer site in %s, found %d — new hard producers must go through a preEmitSameTurnHardPolicyRows ruling", preEmitCheckerSourceFile, producers)
	}
}

// F5-T2 regression: a citation-carrier advisory (out-of-range
// citation_ref) must NOT short-circuit the later hard subgates. A doc
// with both an out-of-range visible citation_ref AND a missing
// complete principal member set gets the member-set hint, and that
// hint stays hard.
func TestPreEmitCarrierAdvisoryDoesNotSkipMemberSetHardLane(t *testing.T) {
	members := []string{"AnalyzerAgent", "ExplorerAgent", "ExtractorAgent", "FinalizerAgent"}
	mu := types.NewMutableState("step-0 append-and-continue guard")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "agent list",
		Value:   "4",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: members,
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "members",
		Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{
			{ID: "i1", Label: "AnalyzerAgent", CitationRef: 7}, // out-of-range: empty citation pool
			{ID: "i2", Label: "ExplorerAgent"},
			{ID: "i3", Label: "ExtractorAgent"},
			// FinalizerAgent omitted → member-set coverage gap.
		},
	}}}

	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx)
	var carrierHints, memberSetHints []emitFixHint
	for _, hint := range hints {
		switch {
		case hint.Kind == types.ViolCitation:
			carrierHints = append(carrierHints, hint)
		case hint.Kind == types.ViolExhaustiveMemberSetCoverageDrift &&
			strings.Contains(hint.ExpectedShape, `member="FinalizerAgent"`):
			memberSetHints = append(memberSetHints, hint)
		}
	}
	if len(carrierHints) == 0 {
		t.Fatalf("out-of-range citation_ref must still produce the carrier repair hint, got %+v", hints)
	}
	if len(memberSetHints) != 1 {
		t.Fatalf("member-set subgate must run despite the carrier advisory (pre-F5 the step-0 early return skipped it), got %+v", hints)
	}
	if hints[0].Kind != types.ViolCitation {
		t.Fatalf("carrier repair must lead the fix-list ordering, got %+v", hints[0])
	}
	hard, _ := splitPreEmitHintsByGate(memberSetHints)
	if len(hard) != 1 {
		t.Fatalf("member-set omission must stay hard in the same emit as a carrier advisory, got %+v", memberSetHints)
	}
	hardAll, _ := splitPreEmitHintsByGate(carrierHints)
	if len(hardAll) != 0 {
		t.Fatalf("carrier hints must stay advisory (D1-G95 settled state), got %+v", hardAll)
	}
}

// F5-T3 / F8 parity: materializeRequiredCaveatWhenOnlyMissing keys on
// the typed caveat-field hint, so enum-label hints firing alongside
// an uncertainty-block hint must not change caveat materialization.
func TestMaterializeRequiredCaveatUnaffectedByEnumLabelHints(t *testing.T) {
	view := &types.AnswerSemanticView{
		UncertaintyRules: []types.UncertaintyRule{{ExpectedBlockKind: types.BlockCaveat}},
	}
	caveatHint := emitFixHint{
		Kind:          types.ViolUncertaintyBlockMissing,
		Field:         "blocks[].kind=caveat",
		ExpectedShape: "add a caveat block",
		Reason:        "uncertainty contract requires a caveat block",
	}
	enumHint := emitFixHint{
		Kind:          types.ViolEnumerationLabelHallucinated,
		Field:         `blocks[id="l1"].items[].label`,
		ExpectedShape: "replace fabricated identifiers",
		Reason:        "labels must ground in the codebase",
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "l1", Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{ID: "i1", Label: "someLabel"}},
	}}}
	if fixed := materializeRequiredCaveatWhenOnlyMissing(doc, view, []emitFixHint{caveatHint, enumHint}, &types.BusContext{}); fixed != 1 {
		t.Fatalf("caveat materialization must fire despite co-firing enum-label hints, got %d", fixed)
	}
	docNoCaveatHint := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "l1", Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{ID: "i1", Label: "someLabel"}},
	}}}
	if fixed := materializeRequiredCaveatWhenOnlyMissing(docNoCaveatHint, view, []emitFixHint{enumHint}, &types.BusContext{}); fixed != 0 {
		t.Fatalf("enum-label hints alone must not materialize a caveat block, got %d", fixed)
	}
}

// scanPreEmitCheckerRoutedKindIdents extracts the types.Viol*
// identifier names passed to appendPreEmitHints (2nd arg) and
// tagPreEmitHints (1st arg) inside runPreEmitChecksWithContext.
func scanPreEmitCheckerRoutedKindIdents(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, preEmitCheckerSourceFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", preEmitCheckerSourceFile, err)
	}
	var body *ast.BlockStmt
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "runPreEmitChecksWithContext" {
			continue
		}
		body = fn.Body
	}
	if body == nil {
		t.Fatal("runPreEmitChecksWithContext not found")
	}
	out := map[string]bool{}
	collect := func(expr ast.Expr) {
		sel, ok := expr.(*ast.SelectorExpr)
		if !ok {
			return
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "types" || !strings.HasPrefix(sel.Sel.Name, "Viol") {
			return
		}
		out[sel.Sel.Name] = true
	}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		switch fn.Name {
		case "appendPreEmitHints":
			if len(call.Args) >= 2 {
				collect(call.Args[1])
			}
		case "tagPreEmitHints":
			if len(call.Args) >= 1 {
				collect(call.Args[0])
			}
		}
		return true
	})
	return out
}

// violationKindValuesByIdent parses the internal/types package
// sources and returns the ViolationKind constant table
// (identifier name → typed value). Test-only source scan.
func violationKindValuesByIdent(t *testing.T) map[string]types.ViolationKind {
	t.Helper()
	out := map[string]types.ViolationKind{}
	matches, err := filepath.Glob(filepath.Join("..", "types", "*.go"))
	if err != nil {
		t.Fatalf("glob types sources: %v", err)
	}
	fset := token.NewFileSet()
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(src), "ViolationKind = ") {
			continue
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				typeIdent, ok := vs.Type.(*ast.Ident)
				if !ok || typeIdent.Name != "ViolationKind" {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					value, err := strconv.Unquote(lit.Value)
					if err != nil {
						continue
					}
					out[name.Name] = types.ViolationKind(value)
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no ViolationKind constants found in ../types — scan drift?")
	}
	return out
}
