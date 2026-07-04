package types

// trace_state_kinds_scan.go — the SINGLE StateKind pin scanner + rule checker
// shared by the types-side pin (trace_state_kinds_pin_test.go) and the
// tool-side pin (internal/tool/answer_document_projection_state_kind_pin_test
// .go). TSH review F6: the two pins used to carry ~115-line private copies of
// this scanner ("used verbatim by the tool-side twin" — which was false) and
// had already grown two UNRECORDED semantic forks: the types copy alone
// understood classifier-func param switches, and the tool copy alone silently
// skipped tagged switches with zero registered cases. Guard code that watches
// twin-copy drift must not itself be a twin copy. The forks are now explicit
// parameters/behavior of ONE implementation:
//
//   - classifierFuncs: functions whose switch tag is a StateKind-carrying
//     PARAMETER (tag-text detection cannot see ".StateKind"); callers that
//     have none pass nil.
//   - empty tagged switches REGISTER as sites (the tool-side silent skip is
//     gone): a switch on .StateKind whose cases are all unregistered still
//     shows up in the golden and must be accounted for.
//   - aliasLedger: consumer-only case words that are NOT universe members,
//     word → recorded divergence rationale. Scan reports which ledger words
//     were actually SEEN (case or comparison), and the rule checker fails on
//     orphans (TSH review F5: deleting the aliased case plus refreshing the
//     golden used to leave the ledger row rotting silently).
//
// This lives in a non-test file ONLY because Go test files cannot be imported
// across packages; nothing in production paths calls it. Findings come back
// as data (Issues strings) so the file never imports "testing".

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// TraceStateKindConsumerSite is one switch whose tag derives from .StateKind
// (direct selector text, a derived local, or a classifier func's param).
type TraceStateKindConsumerSite struct {
	Key        string // file:enclosingFunc#ordinal
	Pos        string
	Handled    map[string]bool // registered universe words
	Aliases    map[string]bool // ledgered consumer-only alias words
	HasDefault bool
}

// TraceStateKindFallthroughDecl is one no-default switch's EXPLICIT
// declaration of the universe words it deliberately lets fall through, with
// rationale — the typed escape lane (§1.6): silence is red, a declared skip
// is green and reviewable.
type TraceStateKindFallthroughDecl struct {
	Missing string
	Why     string
}

// TraceStateKindScan is the scanner output.
type TraceStateKindScan struct {
	Sites       []TraceStateKindConsumerSite
	Comparisons int             // string-literal ==/!= against StateKind-derived expressions
	AliasSeen   map[string]bool // aliasLedger words actually consumed (case or comparison)
	Issues      []string        // per-site violations found during the scan
}

// ScanTraceStateKindConsumers scans the given files (relative to the calling
// package's directory — each pin parses its OWN package files) for (a)
// switches whose tag derives from .StateKind and (b) string-literal
// comparisons against StateKind-derived expressions.
func ScanTraceStateKindConsumers(files []string, classifierFuncs map[string]bool, aliasLedger map[string]string) (TraceStateKindScan, error) {
	scan := TraceStateKindScan{AliasSeen: map[string]bool{}}
	for _, name := range files {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			return scan, fmt.Errorf("parse %s: %w", name, err)
		}
		exprText := func(expr ast.Expr) string {
			var buf bytes.Buffer
			if err := printer.Fprint(&buf, fset, expr); err != nil {
				return ""
			}
			return buf.String()
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			// Derived idents: locals assigned (anywhere in the function) from
			// an expression mentioning .StateKind.
			derived := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				fromStateKind := false
				for _, rhs := range assign.Rhs {
					if strings.Contains(exprText(rhs), ".StateKind") {
						fromStateKind = true
					}
				}
				if !fromStateKind {
					return true
				}
				for _, lhs := range assign.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						derived[ident.Name] = true
					}
				}
				return true
			})
			isStateKindExpr := func(expr ast.Expr) bool {
				text := exprText(expr)
				if strings.Contains(text, ".StateKind") {
					return true
				}
				ident, ok := expr.(*ast.Ident)
				return ok && derived[ident.Name]
			}
			ordinal := 0
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.SwitchStmt:
					if node.Tag == nil {
						return true
					}
					tagged := isStateKindExpr(node.Tag)
					if !tagged && classifierFuncs[fn.Name.Name] {
						tagged = true
					}
					if !tagged {
						return true
					}
					handled := map[string]bool{}
					aliases := map[string]bool{}
					hasDefault := false
					for _, stmt := range node.Body.List {
						clause, ok := stmt.(*ast.CaseClause)
						if !ok {
							continue
						}
						if clause.List == nil {
							hasDefault = true
							continue
						}
						for _, expr := range clause.List {
							lit, ok := expr.(*ast.BasicLit)
							if !ok || lit.Kind != token.STRING {
								scan.Issues = append(scan.Issues, fmt.Sprintf("%s: StateKind switch case must be a plain registered string literal, got %s", fset.Position(expr.Pos()), exprText(expr)))
								continue
							}
							word, err := strconv.Unquote(lit.Value)
							if err != nil {
								scan.Issues = append(scan.Issues, fmt.Sprintf("%s: unquote %s: %v", fset.Position(expr.Pos()), lit.Value, err))
								continue
							}
							switch {
							case TraceStateKindRegistered(word):
								handled[word] = true
							case aliasLedger[word] != "":
								aliases[word] = true
								scan.AliasSeen[word] = true
							default:
								scan.Issues = append(scan.Issues, fmt.Sprintf("%s: StateKind switch case %q is not a registered state-kind word (trace_state_kinds.go) nor a ledgered consumer alias", fset.Position(expr.Pos()), word))
							}
						}
					}
					ordinal++
					scan.Sites = append(scan.Sites, TraceStateKindConsumerSite{
						Key:        fmt.Sprintf("%s:%s#%d", name, fn.Name.Name, ordinal),
						Pos:        fset.Position(node.Pos()).String(),
						Handled:    handled,
						Aliases:    aliases,
						HasDefault: hasDefault,
					})
				case *ast.BinaryExpr:
					if node.Op != token.EQL && node.Op != token.NEQ {
						return true
					}
					var lit *ast.BasicLit
					var other ast.Expr
					if l, ok := node.X.(*ast.BasicLit); ok {
						lit, other = l, node.Y
					} else if r, ok := node.Y.(*ast.BasicLit); ok {
						lit, other = r, node.X
					} else {
						return true
					}
					if lit.Kind != token.STRING || !isStateKindExpr(other) {
						return true
					}
					word, err := strconv.Unquote(lit.Value)
					if err != nil {
						scan.Issues = append(scan.Issues, fmt.Sprintf("%s: unquote %s: %v", fset.Position(lit.Pos()), lit.Value, err))
						return true
					}
					scan.Comparisons++
					switch {
					case word == "" || TraceStateKindRegistered(word):
					case aliasLedger[word] != "":
						scan.AliasSeen[word] = true
					default:
						scan.Issues = append(scan.Issues, fmt.Sprintf("%s: comparison against unregistered state-kind literal %q — register it or ledger the alias", fset.Position(lit.Pos()), word))
					}
				}
				return true
			})
		}
	}
	return scan, nil
}

// CheckTraceStateKindConsumerRules applies the shared rule set to a scan:
// exact site-golden parity, universe coverage (handle / default / ledgered
// fall-through with rationale), ledger hygiene (stale rows, orphan keys), and
// alias-ledger orphan tracking (TSH F5). Returned strings are violations.
func CheckTraceStateKindConsumerRules(scan TraceStateKindScan, siteGolden map[string]string, ledger map[string]TraceStateKindFallthroughDecl, aliasLedger map[string]string) []string {
	var issues []string
	got := map[string]string{}
	for _, site := range scan.Sites {
		rendered := renderTraceStateKindWordSet(site.Handled)
		var aliasList []string
		for alias := range site.Aliases {
			aliasList = append(aliasList, alias)
		}
		sort.Strings(aliasList)
		if len(aliasList) > 0 {
			rendered += "+alias:" + strings.Join(aliasList, ",")
		}
		if site.HasDefault {
			rendered += "|default"
		}
		got[site.Key] = rendered
	}
	if !reflect.DeepEqual(got, siteGolden) {
		var lines []string
		for key, value := range got {
			lines = append(lines, fmt.Sprintf("\t%q: %q,", key, value))
		}
		sort.Strings(lines)
		issues = append(issues, fmt.Sprintf("StateKind consumer switch sites drifted from the golden — review every change (a lost case is a silent misclassification), then update. Current scan:\n%s", strings.Join(lines, "\n")))
	}
	known := map[string]bool{}
	for _, site := range scan.Sites {
		known[site.Key] = true
		decl, hasDecl := ledger[site.Key]
		if site.HasDefault {
			if hasDecl {
				issues = append(issues, fmt.Sprintf("%s (%s): has an explicit default AND a fall-through ledger entry — remove the stale row", site.Key, site.Pos))
			}
			continue
		}
		declared := map[string]bool{}
		if hasDecl {
			for _, name := range strings.Split(decl.Missing, ",") {
				word := strings.TrimSpace(name)
				if !TraceStateKindRegistered(word) {
					issues = append(issues, fmt.Sprintf("%s: ledger declares unknown word %q", site.Key, word))
					continue
				}
				if site.Handled[word] {
					issues = append(issues, fmt.Sprintf("%s (%s): ledger declares %q as fall-through but the switch HANDLES it — stale row", site.Key, site.Pos, word))
				}
				declared[word] = true
			}
		}
		for _, word := range TraceStateKindUniverse {
			if site.Handled[word] || declared[word] {
				continue
			}
			issues = append(issues, fmt.Sprintf("%s (%s): universe word %q is neither handled nor declared as deliberate fall-through — add the case or a ledger declaration with rationale", site.Key, site.Pos, word))
		}
	}
	for key := range ledger {
		if !known[key] {
			issues = append(issues, fmt.Sprintf("fall-through ledger entry %q matches no scanned switch — remove or rekey it", key))
		}
	}
	// TSH F5: alias-ledger orphan hygiene — a recorded divergence whose
	// consumer vanished (case deleted + golden refreshed) must go red instead
	// of rotting as dead documentation.
	var aliasWords []string
	for word := range aliasLedger {
		aliasWords = append(aliasWords, word)
	}
	sort.Strings(aliasWords)
	for _, word := range aliasWords {
		if strings.TrimSpace(aliasLedger[word]) == "" {
			issues = append(issues, fmt.Sprintf("alias ledger entry %q has no recorded rationale", word))
		}
		if !scan.AliasSeen[word] {
			issues = append(issues, fmt.Sprintf("alias ledger entry %q matches no scanned case/comparison — the divergence it records is gone; remove the entry or restore the consumer", word))
		}
	}
	return issues
}

func renderTraceStateKindWordSet(handled map[string]bool) string {
	var parts []string
	for _, word := range TraceStateKindUniverse {
		if handled[word] {
			parts = append(parts, word)
		}
	}
	return strings.Join(parts, ",")
}
