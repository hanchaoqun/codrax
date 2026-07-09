package tracediag

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// tracediagImportWhitelist is the CLOSED import universe of this package's
// non-test files (whitelist style after event_promotion_pin_test): the Go
// standard library, the yaml dependency, the tracequery engine (read-only
// consumption) and internal/types (shared rune-safe truncation only).
//
// The zero-LLM / pure-read red line of §28.12 is enforced structurally:
// internal/llm, internal/agent, internal/orchestrator and internal/tool can
// never slip in without turning this pin red.
var tracediagImportWhitelist = map[string]bool{
	"gopkg.in/yaml.v3": true,
	"github.com/hanchaoqun/codrax/internal/tracequery": true,
	"github.com/hanchaoqun/codrax/internal/types":      true,
}

func TestTraceDiagImportBoundary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquote %s: %v", name, imp.Path.Value, err)
			}
			if isStdlibImport(path) || tracediagImportWhitelist[path] {
				continue
			}
			t.Errorf("%s imports %q — outside the tracediag whitelist (stdlib + yaml.v3 + tracequery + types). The §28.12 zero-LLM/pure-read boundary forbids llm/agent/orchestrator/tool here.", name, path)
		}
	}
	if checked == 0 {
		t.Fatal("no non-test files scanned — the boundary pin is vacuous")
	}
}

// isStdlibImport: stdlib import paths have no dot in their first segment.
func isStdlibImport(path string) bool {
	first := path
	if idx := strings.IndexByte(path, '/'); idx >= 0 {
		first = path[:idx]
	}
	return !strings.Contains(first, ".")
}
