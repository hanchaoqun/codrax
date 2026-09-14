package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// B1675 adds two immediate, per-call scalar integrity operations. Their method
// values and the private field remain forbidden escape surfaces; no ledger,
// callback, or mutable error field is handed to another owner.
func TestB1675OptionalCarrierIntegrityMethodOwnership(t *testing.T) {
	run := func(body string) optionalCarrierCensus {
		t.Helper()
		source := `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func Execute(raw []byte) (result types.ToolResult, err error) {
 carriers := newOptionalCarrierLedger("emit_answer_document")
 defer func() { result = carriers.finalize(result) }()
 ` + body + `
 return result, nil
}`
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fixture.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		return auditOptionalCarrierFiles(fset, map[string]*ast.File{"fixture.go": file})
	}
	if c := run(`carriers.captureTraceRootCauseParamIntegrity(raw)
err = carriers.traceRootCauseParamIntegrityError()`); len(c.offenders) != 0 || len(c.creators) != 1 {
		t.Fatalf("direct per-call integrity access must retain the existing ownership construction: %+v", c)
	}
	for _, body := range []string{
		`capture := carriers.captureTraceRootCauseParamIntegrity; _ = capture`,
		`getter := carriers.traceRootCauseParamIntegrityError; _ = getter`,
		`save(carriers.captureTraceRootCauseParamIntegrity)`,
		`save(carriers.traceRootCauseParamIntegrityError)`,
		`err = carriers.traceRootCauseParamAmbiguity`,
		`carriers.traceRootCauseParamAmbiguity = nil`,
		`carriers.unknownIntegrityOperation(raw)`,
		`carriers.captureTraceRootCauseParamIntegrity(raw, raw)`,
		`carriers.traceRootCauseParamIntegrityError(raw)`,
	} {
		if c := run(body); len(c.offenders) == 0 || !strings.Contains(strings.Join(c.offenders, "\n"), "escapes the call") {
			t.Fatalf("integrity interface escape was not rejected: %s: %+v", body, c)
		}
	}
}
