package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gate_teaching_hint_census_test.go — EVALFIX-2A Tripwire B. The
// analyzer retry-hint channel (MutableState.SetAnalyzerRetryHint) is
// the closed conduit for "teaching sent back after a burned round":
// any instructional text flowing through it must be single-sourced in
// a skill.GateTeaching so Tripwire A can force the initial prompt to
// carry the same lesson. This census makes the obligation static: a
// NEW hard gate wired to a bare instructional retry hint turns this
// test red with zero new test code — the fix is a data row (register
// a GateTeaching, or land an exemption with a rationale ruling the
// teaching repair-time-only).
//
// Scope note: the reference check is function-body-scoped (the spec's
// "directly or via a local variable in the same function"), so two
// call sites inside ONE function share a verdict; exemptions and
// failures are keyed "file:function" accordingly.

// retryHintTeachingExemptions — call sites whose hint deliberately
// carries no GateTeaching reference. Key is "<internal-relative
// file>:<enclosing function>"; the rationale must state why the text
// is repair-time-only. Stale entries (site gone, or site now
// referencing a teaching) are a red test.
var retryHintTeachingExemptions = map[string]string{
	"agent/analyzer.go:ParseOutput": "the read analyzer's gate-failure hints are composed per-failure from the quality gate's own check Detail strings (coherence contradictions, per-file confidence advice) — dynamic diagnostics, not a static escape-lane enumeration; the skill prompt already teaches the gate axes, so there is no fixed sentence to single-source",
}

func TestEveryAnalyzerRetryHintCallSiteReferencesGateTeachingOrIsExempted(t *testing.T) {
	sites := analyzerRetryHintCallSitesFromSource(t)
	if len(sites) == 0 {
		t.Fatal("no SetAnalyzerRetryHint call sites found — the census scan is broken and the tripwire vacuous")
	}
	seen := map[string]bool{}
	for _, site := range sites {
		seen[site.key] = true
		rationale, exempted := retryHintTeachingExemptions[site.key]
		switch {
		case exempted && strings.TrimSpace(rationale) == "":
			t.Errorf("retry-hint call site %s exemption has no rationale", site.key)
		case exempted && site.referencesTeaching:
			t.Errorf("retry-hint call site %s is exempted AND references a skill.GateTeaching — remove the stale exemption", site.key)
		case !exempted && !site.referencesTeaching:
			t.Errorf("retry-hint call site %s sends instructional text through the analyzer retry channel without referencing any skill.GateTeaching — single-source the teaching (register a GateTeaching var and splice its Text) or declare an exemption with a rationale", site.key)
		}
	}
	for key := range retryHintTeachingExemptions {
		if !seen[key] {
			t.Errorf("exemption %q matches no SetAnalyzerRetryHint call site — remove the stale entry", key)
		}
	}
}

type retryHintCallSite struct {
	key                string
	referencesTeaching bool
}

// analyzerRetryHintCallSitesFromSource walks every non-test Go file
// under internal/ and collects each SetAnalyzerRetryHint call with a
// function-body-scoped check for skill.GateTeaching* /
// skill.AllGateTeachings references. The method DEFINITION
// (types/context.go) is a FuncDecl, not a CallExpr, and is naturally
// excluded.
func analyzerRetryHintCallSitesFromSource(t *testing.T) []retryHintCallSite {
	t.Helper()
	internalRoot := ".."
	var sites []retryHintCallSite
	err := filepath.WalkDir(internalRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		rel, relErr := filepath.Rel(internalRoot, path)
		if relErr != nil {
			t.Fatalf("relativize %s: %v", path, relErr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			calls := 0
			refs := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CallExpr:
					if sel, ok := node.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetAnalyzerRetryHint" {
						calls++
					}
				case *ast.SelectorExpr:
					if strings.HasPrefix(node.Sel.Name, "GateTeaching") || node.Sel.Name == "AllGateTeachings" {
						refs = true
					}
				}
				return true
			})
			if calls > 0 {
				sites = append(sites, retryHintCallSite{
					key:                filepath.ToSlash(rel) + ":" + fn.Name.Name,
					referencesTeaching: refs,
				})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", internalRoot, err)
	}
	return sites
}
