package types

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestEvidenceClosureProductionMutationsUseReducer guards the D1-F8f reducer
// ownership boundary. Production code may read EvidenceClosure freely, but
// read/handoff-state writes should flow through typed EvidenceReducerInput
// lanes so controller, snapshot, replay, and audit code share one authority.
func TestEvidenceClosureProductionMutationsUseReducer(t *testing.T) {
	root := reducerOwnershipRepoRoot(t)
	banned := map[string]bool{
		"SetReadSet":                       true,
		"AddReadSet":                       true,
		"SetReadRanges":                    true,
		"AddReadRanges":                    true,
		"SetFileTotalLines":                true,
		"RecordFileTotalLines":             true,
		"AppendAcceptedEvidenceItems":      true,
		"AppendAcceptedEvidenceRefs":       true,
		"RecordSourceInventoryObservation": true,
		"SetLatestProgressDecision":        true,
		"SetNodeExecStatus":                true,
	}
	allow := map[string]map[string]bool{
		"internal/types/evidence_round.go": {
			"SetReadSet":                       true,
			"AddReadSet":                       true,
			"SetReadRanges":                    true,
			"AddReadRanges":                    true,
			"SetFileTotalLines":                true,
			"RecordFileTotalLines":             true,
			"AppendAcceptedEvidenceRefs":       true,
			"RecordSourceInventoryObservation": true,
			"SetLatestProgressDecision":        true,
			"SetNodeExecStatus":                true,
		},
		"internal/types/evidence_closure.go": {
			"AppendAcceptedEvidenceRefs": true,
		},
		"internal/orchestrator/scheduler_status.go": {
			"SetNodeExecStatus": true,
		},
	}

	var violations []string
	fset := token.NewFileSet()
	internalRoot := filepath.Join(root, "internal")
	err := filepath.WalkDir(internalRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || !banned[sel.Sel.Name] {
				return true
			}
			if allow[rel][sel.Sel.Name] {
				return true
			}
			pos := fset.Position(sel.Sel.Pos())
			violations = append(violations, rel+":"+strconv.Itoa(pos.Line)+" "+sel.Sel.Name)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan production files: %v", err)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("production EvidenceClosure mutations must route through typed reducer inputs:\n  %s", strings.Join(violations, "\n  "))
	}
}

func reducerOwnershipRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatalf("could not find repo root from %s", wd)
		}
		wd = parent
	}
}
