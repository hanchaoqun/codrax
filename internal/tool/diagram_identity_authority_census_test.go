package tool

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// diagram_identity_authority_census_test.go — V4-1 structural tripwire
// (colleague_merge_audit §40.9 → §40.34; template: the session-ANY census).
// Every emit-side hard / normalizing arm that judges whether a typed entity,
// identity or endpoint is named inside a quote MUST go through the single
// authority entityNamedInQuote. The quote-anchoring helpers
// (quoteVerbatimInRequest / sourceQuotePresentInCurrentRequest) may be
// called only from the allowlisted quote-anchoring functions, and never with
// an identity-named argument inside the registered diagram arms.

type diagramIdentityCensusKey struct{ file, fn string }

var diagramIdentityCensusQuoteAnchorAllowlist = map[diagramIdentityCensusKey]bool{
	// ① quote-vs-request anchoring (the helper's only legitimate job).
	{"emit_analysis.go", "quoteVerbatimInRequest"}:                                        true,
	{"emit_analysis.go", "parseDiagramHint"}:                                              true,
	{"emit_analysis.go", "parseErrorGranularityProfile"}:                                  true,
	{"emit_analysis.go", "parseExternalObservationPolicy"}:                                true,
	{"emit_analysis.go", "parseAnswerExclusionPolicy"}:                                    true,
	{"emit_analysis.go", "parseAnswerRoleProfile"}:                                        true,
	{"emit_analysis.go", "parseAnswerVisibilityProfile"}:                                  true,
	{"emit_analysis.go", "parseFieldValueProfile"}:                                        true,
	{"emit_analysis.go", "parseHistorySelectionProfile"}:                                  true,
	{"emit_analysis.go", "parseRuntimeArtifactScopeProfile"}:                              true,
	{"emit_analysis.go", "parseRuntimeQuestionProfile"}:                                   true,
	{"emit_analysis.go", "parseRuntimeTargetProfile"}:                                     true,
	{"emit_analysis.go", "parseSourceInventoryProfile"}:                                   true,
	{"emit_analysis.go", "parseSourceScopeProfile"}:                                       true,
	{"emit_analysis.go", "sourceInventoryProfileRepairSourceQuotes"}:                      true,
	{"emit_analysis_call_chain_wire.go", "reconcileRuntimeSelectionProfile"}:              true,
	{"emit_analysis_call_chain_wire.go", "validateCallChainRuntimeSelectionDeclaration"}:  true,
	{"emit_analysis_source_inventory_prescan.go", "sourceInventoryAnalyzerPrescanQuotes"}: true,
}

var diagramIdentityCensusHardArms = map[string]bool{
	"validateRequiredFlowDiagramParticipantProvenance":    true,
	"reconcileDiagramParticipantsWithClosedRelationScope": true,
	"parseDiagramHint": true,
}

var diagramIdentityCensusIdentityArgs = map[string]bool{
	"entity": true, "rawEntity": true, "identity": true, "endpoint": true, "symbol": true, "participant": true, "candidate": true,
}

func diagramIdentityAuthorityCensus(files map[string]string) (offenders []string, seenAllow map[diagramIdentityCensusKey]bool, err error) {
	seenAllow = map[diagramIdentityCensusKey]bool{}
	fset := token.NewFileSet()
	positive := map[string]bool{}
	for name, src := range files {
		file, perr := parser.ParseFile(fset, name, src, 0)
		if perr != nil {
			return nil, nil, perr
		}
		base := filepath.Base(name)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := diagramIdentityCensusKey{base, fn.Name.Name}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee := ""
				switch f := call.Fun.(type) {
				case *ast.Ident:
					callee = f.Name
				case *ast.SelectorExpr:
					if x, ok := f.X.(*ast.Ident); ok {
						callee = x.Name + "." + f.Sel.Name
					}
				}
				if callee == "entityNamedInQuote" && diagramIdentityCensusHardArms[fn.Name.Name] {
					positive[fn.Name.Name] = true
				}
				switch callee {
				case "quoteVerbatimInRequest", "sourceQuotePresentInCurrentRequest":
					if diagramIdentityCensusQuoteAnchorAllowlist[key] {
						seenAllow[key] = true
					} else {
						offenders = append(offenders, name+":"+fn.Name.Name+" calls "+callee)
					}
					if diagramIdentityCensusHardArms[fn.Name.Name] && diagramIdentityCensusHasIdentityArg(fset, call) {
						offenders = append(offenders, name+":"+fn.Name.Name+" passes an identity to "+callee)
					}
				case "strings.Contains", "strings.Index", "strings.HasPrefix", "strings.HasSuffix":
					if diagramIdentityCensusHardArms[fn.Name.Name] && diagramIdentityCensusHasIdentityArg(fset, call) {
						offenders = append(offenders, name+":"+fn.Name.Name+" substring-matches an identity via "+callee)
					}
				}
				return true
			})
		}
	}
	for arm := range diagramIdentityCensusHardArms {
		if !positive[arm] {
			offenders = append(offenders, "hard arm "+arm+" no longer calls entityNamedInQuote")
		}
	}
	return offenders, seenAllow, nil
}

func diagramIdentityCensusHasIdentityArg(fset *token.FileSet, call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		switch a := arg.(type) {
		case *ast.Ident:
			if diagramIdentityCensusIdentityArgs[a.Name] {
				return true
			}
		case *ast.SelectorExpr:
			var b strings.Builder
			_ = printer.Fprint(&b, fset, a)
			printed := b.String()
			if strings.HasSuffix(printed, ".Identity") || strings.HasSuffix(printed, ".Entities") || strings.HasSuffix(printed, ".MentionedEntities") {
				return true
			}
		}
	}
	return false
}

func TestDiagramIdentityAuthorityCensus(t *testing.T) {
	files := map[string]string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, rerr := os.ReadFile(e.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		files[e.Name()] = string(body)
	}
	if len(files) < 50 {
		t.Fatalf("census walked only %d files", len(files))
	}
	offenders, seen, err := diagramIdentityAuthorityCensus(files)
	if err != nil {
		t.Fatalf("census parse failed (a silent green would defeat the tripwire): %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("identity-membership authority violated: %v", offenders)
	}
	for key := range diagramIdentityCensusQuoteAnchorAllowlist {
		if key.fn == "quoteVerbatimInRequest" {
			continue // the helper itself calls sourceQuotePresentInCurrentRequest
		}
		if !seen[key] {
			t.Fatalf("stale allowlist entry %s:%s (no quote-anchoring call found) — prune it", key.file, key.fn)
		}
	}
	// Self-red: a hard arm that reverts to the substring helper must be reported.
	mutated := map[string]string{"emit_analysis.go": strings.Replace(files["emit_analysis.go"],
		"!entityNamedInQuote(rm.DiagramHint.RelationScopeQuote, entity)",
		"!quoteVerbatimInRequest(rm.DiagramHint.RelationScopeQuote, entity)", 1)}
	if mutated["emit_analysis.go"] == files["emit_analysis.go"] {
		t.Fatal("self-red fixture did not find the hard-arm call to mutate")
	}
	offenders, _, err = diagramIdentityAuthorityCensus(mutated)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range offenders {
		found = found || strings.Contains(o, "validateRequiredFlowDiagramParticipantProvenance")
	}
	if !found {
		t.Fatalf("census must flag the reverted hard arm, got %v", offenders)
	}
}
