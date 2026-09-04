package tool

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strings"
	"testing"
)

// answer_block_normalize_anchor_census_test.go — V10-4 structural tripwire
// (colleague_merge_audit §40.57; template: the V4-1 / V4-3 censuses).
//
// The G2 converter (answer_block_normalize.go) is the single seat every
// emit path (full emit, patch emit, text recovery, recovered blocks) uses
// to turn a model block into the typed AnswerBlock. It may validate, reject
// or re-home model edge_anchors, but it must never EDIT one: no endpoint,
// identity, relation kind, claim form or label of a model anchor may be
// assigned, no anchor may be minted, and the anchor array may only flow
// through a registered shape. B698 (commit 60f89da79) silently swapped a
// reversed classDiagram type_relation anchor here; that rewrite is retired
// and the contradiction is now reported by the diagram evidence gate.
//
// Rules, bound by syntax over every occurrence (fail loud on any shape the
// registry does not recognise, §40.50):
//
//  1. no assignment whose LHS selects an anchor field
//     (FromNode / ToNode / FromIdentity / ToIdentity / RelationKind /
//     ClaimForm / VisibleLabel);
//  2. every `.EdgeAnchors` selector occurrence sits in a registered shape:
//     (a) the value of a composite-literal `EdgeAnchors:` key (verbatim
//     passthrough / re-home into a split half), (b) an argument of the
//     builtin len, (c) the LHS of `x.EdgeAnchors = nil` (re-home: the
//     visible half drops the array the diagram half keeps);
//  3. no composite literal of DiagramEdgeAnchor (the converter never mints
//     an anchor);
//  4. positive anchor: NormalizeEmitAnswerBlock holds exactly one
//     `EdgeAnchors: raw.EdgeAnchors` passthrough, so the census cannot go
//     green by the seat disappearing.

const anchorCensusFile = "answer_block_normalize.go"

var anchorCensusAnchorFields = map[string]bool{
	"FromNode": true, "ToNode": true, "FromIdentity": true, "ToIdentity": true,
	"RelationKind": true, "ClaimForm": true, "VisibleLabel": true,
}

type anchorCensusReport struct {
	offenders   []string
	passthrough int
}

func anchorCensusPrint(fset *token.FileSet, n ast.Node) string {
	var b bytes.Buffer
	_ = printer.Fprint(&b, fset, n)
	return b.String()
}

func anchorCensusWalk(root ast.Node, visit func(n, parent ast.Node)) {
	var stack []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		var parent ast.Node
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		visit(n, parent)
		stack = append(stack, n)
		return true
	})
}

func answerBlockNormalizeAnchorCensus(src string) (anchorCensusReport, error) {
	var report anchorCensusReport
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, anchorCensusFile, src, 0)
	if err != nil {
		return report, err
	}
	pos := func(n ast.Node) string { return fset.Position(n.Pos()).String() }
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		anchorCensusWalk(fn.Body, func(n, parent ast.Node) {
			switch node := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range node.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && anchorCensusAnchorFields[sel.Sel.Name] {
						report.offenders = append(report.offenders,
							pos(node)+" "+fn.Name.Name+" assigns anchor field "+anchorCensusPrint(fset, lhs))
					}
				}
			case *ast.CompositeLit:
				if node.Type != nil && strings.HasSuffix(anchorCensusPrint(fset, node.Type), "DiagramEdgeAnchor") {
					report.offenders = append(report.offenders,
						pos(node)+" "+fn.Name.Name+" mints an anchor literal "+anchorCensusPrint(fset, node.Type))
				}
			case *ast.SelectorExpr:
				if node.Sel.Name != "EdgeAnchors" {
					return
				}
				switch p := parent.(type) {
				case *ast.KeyValueExpr:
					if key, ok := p.Key.(*ast.Ident); ok && key.Name == "EdgeAnchors" && p.Value == n {
						if fn.Name.Name == "NormalizeEmitAnswerBlock" && anchorCensusPrint(fset, n) == "raw.EdgeAnchors" {
							report.passthrough++
						}
						return
					}
				case *ast.CallExpr:
					if callee, ok := p.Fun.(*ast.Ident); ok && callee.Name == "len" {
						return
					}
				case *ast.AssignStmt:
					if len(p.Lhs) == 1 && p.Lhs[0] == n && len(p.Rhs) == 1 {
						if rhs, ok := p.Rhs[0].(*ast.Ident); ok && rhs.Name == "nil" {
							return
						}
					}
				}
				parentText := "<nil>"
				if parent != nil {
					parentText = anchorCensusPrint(fset, parent)
				}
				report.offenders = append(report.offenders,
					pos(node)+" "+fn.Name.Name+" unrecognized EdgeAnchors shape: "+parentText)
			}
		})
	}
	return report, nil
}

func TestNormalizeEmitAnswerBlock_NeverMutatesModelEdgeAnchors(t *testing.T) {
	body, err := os.ReadFile(anchorCensusFile)
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	report, err := answerBlockNormalizeAnchorCensus(src)
	if err != nil {
		t.Fatalf("census parse failed (a silent green would defeat the tripwire): %v", err)
	}
	if len(report.offenders) > 0 {
		t.Fatalf("the G2 converter must never edit, mint or re-route a model edge anchor: %v", report.offenders)
	}
	if report.passthrough != 1 {
		t.Fatalf("positive anchor missing: NormalizeEmitAnswerBlock must hold exactly one `EdgeAnchors: raw.EdgeAnchors` passthrough, found %d", report.passthrough)
	}

	// Self-red: every historical or conceivable rewrite shape must be
	// reported when injected into the live source. S2 is the retired B698
	// swap seat verbatim.
	const beforeBlock = "\tblk := types.AnswerBlock{"
	const afterBlock = "\tif raw.RuntimeWorkRelation != nil {"
	shapes := []struct {
		name, marker, inject, want string
	}{
		{"S1 tuple swap through element pointer", beforeBlock,
			"\tfor i := range raw.EdgeAnchors {\n\t\ta := &raw.EdgeAnchors[i]\n\t\ta.FromNode, a.ToNode = a.ToNode, a.FromNode\n\t}\n",
			"assigns anchor field a.FromNode"},
		{"S2 retired B698 helper reassignment", beforeBlock,
			"\tif raw.Diagram != nil {\n\t\traw.EdgeAnchors = normalizeClassDiagramTypeRelationAnchorDirections(raw.Diagram.Body, raw.EdgeAnchors)\n\t}\n",
			"unrecognized EdgeAnchors shape"},
		{"S3 in-place index assignment", beforeBlock,
			"\traw.EdgeAnchors[0].FromIdentity = \"\"\n",
			"assigns anchor field raw.EdgeAnchors[0].FromIdentity"},
		{"S4 address escape to a helper", beforeBlock,
			"\trealignAnchors(&raw.EdgeAnchors)\n",
			"unrecognized EdgeAnchors shape"},
		{"S5 post-construction relation rewrite", afterBlock,
			"\tblk.EdgeAnchors[0].RelationKind = types.DiagramRelCall\n",
			"assigns anchor field blk.EdgeAnchors[0].RelationKind"},
		{"S6 unregistered clone/call consumer", beforeBlock,
			"\t_ = types.CloneDiagramEdgeAnchors(raw.EdgeAnchors)\n",
			"unrecognized EdgeAnchors shape"},
		{"S7 minted anchor literal", beforeBlock,
			"\traw.EdgeAnchors = append(raw.EdgeAnchors, types.DiagramEdgeAnchor{FromNode: \"A\", ToNode: \"B\"})\n",
			"mints an anchor literal"},
		{"S8 range copy re-route", beforeBlock,
			"\tfor _, a := range raw.EdgeAnchors {\n\t\t_ = a\n\t}\n",
			"unrecognized EdgeAnchors shape"},
	}
	for _, shape := range shapes {
		mutated := strings.Replace(src, shape.marker, shape.inject+shape.marker, 1)
		if mutated == src {
			t.Fatalf("%s: injection marker %q not found", shape.name, shape.marker)
		}
		got, err := answerBlockNormalizeAnchorCensus(mutated)
		if err != nil {
			t.Fatalf("%s: %v", shape.name, err)
		}
		found := false
		for _, o := range got.offenders {
			found = found || strings.Contains(o, shape.want)
		}
		if !found {
			t.Fatalf("%s: census must report %q, got %v", shape.name, shape.want, got.offenders)
		}
	}
	// Self-red for the positive anchor: removing the passthrough seat is
	// reported rather than silently passing.
	mutated := strings.Replace(src, "\t\tEdgeAnchors:           raw.EdgeAnchors,\n", "", 1)
	if mutated == src {
		t.Fatal("passthrough seat not found for the positive-anchor self-red")
	}
	got, err := answerBlockNormalizeAnchorCensus(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if got.passthrough != 0 {
		t.Fatalf("positive-anchor self-red: expected 0 passthroughs, got %d", got.passthrough)
	}
}
