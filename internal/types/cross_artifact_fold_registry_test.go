package types_test

// cross_artifact_fold_registry_test.go — V1-4 (colleague_merge_audit §40.26 ③,
// 2026-09-03; §40.48 fold-in): 「任何跨工件折叠的公开集合必须保留分区键」. Every
// producer that folds a multi-artifact TraceCausalProjectionSet into a row
// collection (and every ledger-fed per-artifact authority row, plus the
// public sidecar wire item) is registered here with its row type, and each
// row type must carry the partition key as an `ArtifactLabel` string field
// (json tag `artifact_label` where the row is a wire shape). A go/ast census
// over the producing packages fails on any NEW fold producer — a function OR
// method whose parameter or receiver is a TraceCausalProjectionSet (by value
// or pointer, bare or package-qualified) and whose result is not a bare bool
// gate — that is not registered, so the next fold surface cannot silently
// lose the key the way the tracefinding contract did.
//
// The reflect check is a SHAPE tie only; the behavioural witnesses that the
// key is actually stamped and propagated live beside each producer
// (answer_relation_claim_artifact_label_test.go for the relation fold,
// artifact_partition_test.go for the candidate contract).
//
// External test package: tracefinding imports types, so the registry can
// name the compiler only from types_test.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

type crossArtifactFoldRow struct {
	producer string
	// row is the folded row type carrying the partition key; nil marks a
	// producer that publishes the PARTITION ROSTER itself (the labels, not
	// rows) and therefore has no row to check.
	row reflect.Type
}

// crossArtifactFoldRegistry — producer function name → row type carrying the
// partition key. Set-fed producers are what the census discovers; ledger-fed
// authorities and the wire item are listed so their rows join the reflect
// check even though no TraceCausalProjectionSet parameter names them.
func crossArtifactFoldRegistry() []crossArtifactFoldRow {
	return []crossArtifactFoldRow{
		{"CompileTraceAnswerRelationAuthorities", reflect.TypeOf(types.AnswerRelationAuthority{})},
		{"BuildTraceRankRosterAuthorities", reflect.TypeOf(types.TraceRankRosterAuthority{})},
		{"BuildTraceTargetStateScopeAuthorities", reflect.TypeOf(types.TraceTargetStateScopeAuthority{})},
		{"CompileCandidateContract", reflect.TypeOf(types.TraceCauseDecision{})},
		// The partition roster itself (labels in first-appearance order).
		{"TraceCausalProjectionSetArtifactLabels", nil},
		// Ledger-fed per-artifact authorities (partition key already a row field).
		{"BuildTraceWakeupEdgeRoleAuthorities", reflect.TypeOf(types.TraceWakeupEdgeRoleAuthority{})},
		{"BuildTraceBlockingWallClockAuthorities", reflect.TypeOf(types.TraceBlockingWallClockAuthority{})},
		{"BuildTraceValueOccurrenceAuthorities", reflect.TypeOf(types.TraceValueOccurrenceAuthority{})},
		{"BuildTraceIPCRequestCensusAuthorities", reflect.TypeOf(types.TraceIPCRequestCensusAuthority{})},
		{"BuildTraceTargetWaitSummaryAuthorities", reflect.TypeOf(types.TraceTargetWaitSummaryAuthority{})},
		// The public sidecar wire item (bound from TraceCauseDecision) and the
		// model-copyable relation claim (projected from the relation authority).
		{"BindRootCauseReportSelection", reflect.TypeOf(types.TraceRootCauseItemV2{})},
		{"AnswerRelationClaimForAuthority", reflect.TypeOf(types.AnswerRelationClaim{})},
	}
}

func TestCrossArtifactFoldSurfacesCarryPartitionKey(t *testing.T) {
	// Keep the compiler referenced so the registry names a real symbol.
	_ = tracefinding.CompileCandidateContract
	_ = types.TraceCausalProjectionSetArtifactLabels
	for _, entry := range crossArtifactFoldRegistry() {
		if entry.row == nil {
			continue
		}
		field, ok := entry.row.FieldByName("ArtifactLabel")
		if !ok || field.Type.Kind() != reflect.String {
			t.Errorf("%s row %s has no string ArtifactLabel partition key (§40.26 ③)", entry.producer, entry.row)
			continue
		}
		if tag, tagged := field.Tag.Lookup("json"); tagged && strings.Split(tag, ",")[0] != "artifact_label" {
			t.Errorf("%s row %s: json tag %q must be artifact_label (the same key on every wire)", entry.producer, entry.row, tag)
		}
	}
}

func TestCrossArtifactFoldProducersAreRegistered(t *testing.T) {
	registered := map[string]bool{}
	for _, entry := range crossArtifactFoldRegistry() {
		registered[entry.producer] = true
	}
	found, err := crossArtifactFoldCensus([]string{".", "../analysis/tracefinding"})
	if err != nil {
		t.Fatal(err)
	}
	for _, producer := range found {
		if !registered[producer.name] {
			t.Errorf("%s: %s folds a TraceCausalProjectionSet but is not in crossArtifactFoldRegistry — register it with a row type that carries ArtifactLabel (§40.26 ③)", producer.pos, producer.name)
		}
	}
	if len(found) < 5 {
		t.Fatalf("census looks broken: only %d set-fed fold producer(s) found", len(found))
	}
}

// Self-red: every producer SHAPE the census claims to cover is discovered on
// a scratch package, and the one typed exemption (a bare-bool gate) is not.
func TestCrossArtifactFoldCensusCoversEveryProducerShape(t *testing.T) {
	dir := t.TempDir()
	src := `package scratch

type TraceCausalProjectionSet struct{}
type row struct{ ArtifactLabel string }

func ByValue(set TraceCausalProjectionSet) []row            { return nil }
func ByPointer(set *TraceCausalProjectionSet) []row         { return nil }
func Qualified(set types.TraceCausalProjectionSet) []row    { return nil }
func QualifiedPtr(set *types.TraceCausalProjectionSet) []row { return nil }
func Parenthesized(set (TraceCausalProjectionSet)) []row     { return nil }
func Second(a int, set TraceCausalProjectionSet) []row       { return nil }
func (s TraceCausalProjectionSet) ValueReceiver() []row      { return nil }
func (s *TraceCausalProjectionSet) PointerReceiver() []row   { return nil }
func Gate(set TraceCausalProjectionSet) bool                 { return false }
func (s TraceCausalProjectionSet) GateMethod() bool          { return false }
func Unrelated(x int) []row                                  { return nil }
`
	if err := os.WriteFile(filepath.Join(dir, "scratch.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := crossArtifactFoldCensus([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, producer := range found {
		got[producer.name] = true
	}
	for _, want := range []string{"ByValue", "ByPointer", "Qualified", "QualifiedPtr", "Parenthesized", "Second", "ValueReceiver", "PointerReceiver"} {
		if !got[want] {
			t.Errorf("census misses producer shape %s (an evasion): %v", want, found)
		}
	}
	for _, exempt := range []string{"Gate", "GateMethod", "Unrelated"} {
		if got[exempt] {
			t.Errorf("census must not count %s (bare-bool gate / unrelated): %v", exempt, found)
		}
	}
}

type crossArtifactFoldProducer struct {
	name string
	pos  string
}

// crossArtifactFoldCensus walks the production .go files under dirs and
// returns every function or method that takes a TraceCausalProjectionSet
// (parameter or receiver; by value or pointer; bare or package-qualified) and
// whose result is not a bare bool gate.
func crossArtifactFoldCensus(dirs []string) ([]crossArtifactFoldProducer, error) {
	fset := token.NewFileSet()
	var found []crossArtifactFoldProducer
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || !crossArtifactFoldTakesProjectionSet(fn) || crossArtifactFoldIsBoolGate(fn) {
					continue
				}
				found = append(found, crossArtifactFoldProducer{name: fn.Name.Name, pos: fset.Position(fn.Pos()).String()})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return found, nil
}

func crossArtifactFoldTakesProjectionSet(fn *ast.FuncDecl) bool {
	var fields []*ast.Field
	if fn.Recv != nil {
		fields = append(fields, fn.Recv.List...)
	}
	if fn.Type.Params != nil {
		fields = append(fields, fn.Type.Params.List...)
	}
	for _, field := range fields {
		if crossArtifactFoldNamesProjectionSet(field.Type) {
			return true
		}
	}
	return false
}

// crossArtifactFoldNamesProjectionSet unwraps pointer/paren wrappers and
// matches bare or package-qualified TraceCausalProjectionSet.
func crossArtifactFoldNamesProjectionSet(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.StarExpr:
		return crossArtifactFoldNamesProjectionSet(x.X)
	case *ast.ParenExpr:
		return crossArtifactFoldNamesProjectionSet(x.X)
	case *ast.Ident:
		return x.Name == "TraceCausalProjectionSet"
	case *ast.SelectorExpr:
		return x.Sel.Name == "TraceCausalProjectionSet"
	}
	return false
}

// crossArtifactFoldIsBoolGate — a function whose only result is a bare bool
// is a materialization gate, not a fold: it publishes no rows.
func crossArtifactFoldIsBoolGate(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return false
	}
	ident, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
	return ok && ident.Name == "bool"
}
