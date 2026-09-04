package tool

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// diagram_relation_issue_vocabulary_census_test.go — V10-4 R2' tripwire
// (colleague_merge_audit §40.57). The diagram relation gate mints a closed
// issue vocabulary (string constants in answer_document_diagram_evidence.go
// and answer_document_pre_emit_check.go); the repair lease in internal/types
// decides, per issue, which atomic carrier/actions the retrying model may
// execute. A new issue added without a lease decision silently lands in the
// lease's `default` arm (prior_anchor, remove-only) and may publish an
// unexecutable capability. This census binds the two closed sets:
//
//  1. every producer issue constant has an explicit lease decision — an
//     explicit case label / comparison in
//     answerDiagramRelationRepairFailureCapabilities, a replacement grant in
//     answerDiagramRelationRepairIssueAllowsReplacement, or a recorded row in
//     diagramIssueDefaultLaneDecisions below (default arm by decision);
//  2. every issue string the lease switches name exists in the producer
//     vocabulary (no typo'd or orphaned lease label);
//  3. diagramIssueDefaultLaneDecisions cannot rot: each row must be a live
//     producer constant and must not also be an explicit lease label.
//
// Fail loud (§40.50): a case label or const value in a shape the walker does
// not recognise is an offender, never a skip.

var diagramIssueVocabularyProducerFiles = []string{
	"answer_document_diagram_evidence.go",
	"answer_document_pre_emit_check.go",
}

const diagramIssueVocabularyLeaseFile = "../types/answer_document_relation_repair_lease.go"

// diagramIssueDefaultLaneDecisions records, per issue, that the lease's
// default arm (unique prior_anchor → remove; replace only through the
// AllowsReplacement grant; visible occurrence → visible_body_edge remove) is
// the deliberate capability. Adding a producer issue without a row here or an
// explicit lease case fails the census.
var diagramIssueDefaultLaneDecisions = map[string]string{
	"duplicate_participant_identity":                       "alias declaration defect; no single anchor carrier — default arm publishes nothing executable by design",
	"standalone_relation_endpoint_identity_missing":        "list/table anchor lacks identities; prior_anchor remove-only",
	"call_edge_unproven":                                   "evidence-negative; remove-only",
	"call_edge_occurrence_unproven":                        "evidence-negative occurrence; remove-only",
	"typed_relation_tuple_reused_across_visible_endpoints": "structural clone; remove-only on the exact prior anchor",
	"registration_edge_unproven":                           "evidence-negative; remove-only",
	"type_relation_edge_unproven":                          "evidence-negative; remove-only",
	"assignment_edge_unproven":                             "evidence-negative; remove-only",
	"data_flow_edge_unproven":                              "evidence-negative; remove-only",
	"return_edge_unproven":                                 "evidence-negative; remove-only",
	"callback_handoff_unproven":                            "evidence-negative; remove-only",
	"argument_flow_unproven":                               "evidence-negative; remove-only",
	"semantic_relation_edge_unproven":                      "evidence-negative; remove-only",
	"standalone_principal_path_missing_relation_owner":     "standalone list/table structural defect; no anchor carrier",
	"standalone_relation_claim_has_no_anchor":              "standalone list/table structural defect; no anchor carrier",
	"standalone_relation_anchor_has_no_claim":              "standalone list/table structural defect; prior_anchor remove-only",
	"standalone_relation_missing_visible_label":            "standalone list/table label defect; prior_anchor remove-only",
	"standalone_semantic_handoff_missing":                  "standalone list/table structural defect; no anchor carrier",
}

type diagramIssueVocabularyCensus struct {
	producer      map[string]string // value → declaring file:const
	explicitLease map[string]bool   // values named by capabilities cases / comparisons
	replaceGrant  map[string]bool   // values named by AllowsReplacement
	relationCases map[string]bool   // values named by EffectiveRelation
	offenders     []string
}

func diagramIssueCensusPrint(fset *token.FileSet, n ast.Node) string {
	var b bytes.Buffer
	_ = printer.Fprint(&b, fset, n)
	return b.String()
}

// diagramIssueCensusStringConsts collects every package-level string constant
// of a parsed file, resolving `types.X` / bare identifiers against the given
// lookup. An unresolvable const initializer is reported, not skipped.
func diagramIssueCensusStringConsts(fset *token.FileSet, file *ast.File, name string, lookup map[string]string, offenders *[]string, strict bool) map[string]string {
	report := func(msg string) {
		if strict {
			*offenders = append(*offenders, msg)
		}
	}
	out := map[string]string{}
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
			for i, ident := range vs.Names {
				if i >= len(vs.Values) {
					continue // iota / implicit repetition: not a string vocabulary entry
				}
				if vs.Type != nil {
					continue // typed constants (enums with their own closed sets) are not issue strings
				}
				switch v := vs.Values[i].(type) {
				case *ast.BasicLit:
					if v.Kind == token.STRING {
						out[ident.Name] = strings.Trim(v.Value, "`\"")
					}
				case *ast.Ident:
					if val, ok := lookup[v.Name]; ok {
						out[ident.Name] = val
					} else {
						report(name + ": const " + ident.Name + " references unknown identifier " + v.Name)
					}
				case *ast.SelectorExpr:
					if val, ok := lookup[v.Sel.Name]; ok {
						out[ident.Name] = val
					} else {
						report(name + ": const " + ident.Name + " references unknown selector " + diagramIssueCensusPrint(fset, v))
					}
				case *ast.BinaryExpr, *ast.CallExpr:
					// concatenations / conversions are not issue tokens
				default:
					report(name + ": const " + ident.Name + " has unrecognized initializer shape " + diagramIssueCensusPrint(fset, v))
				}
			}
		}
	}
	return out
}

func diagramIssueCensusTypesConsts(fset *token.FileSet, offenders *[]string) (map[string]string, *ast.File, error) {
	entries, err := os.ReadDir("../types")
	if err != nil {
		return nil, nil, err
	}
	lookup := map[string]string{}
	var lease *ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join("../types", e.Name())
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil, nil, rerr
		}
		file, perr := parser.ParseFile(fset, path, src, 0)
		if perr != nil {
			return nil, nil, perr
		}
		if filepath.Base(path) == filepath.Base(diagramIssueVocabularyLeaseFile) {
			lease = file
		}
		// The package-wide lookup only resolves identifiers; unknown or
		// non-string initializers elsewhere in internal/types are not issue
		// vocabulary and are not reported. The lease file itself is strict.
		for k, v := range diagramIssueCensusStringConsts(fset, file, path, lookup, offenders, false) {
			lookup[k] = v
		}
	}
	if lease != nil {
		for k, v := range diagramIssueCensusStringConsts(fset, lease, diagramIssueVocabularyLeaseFile, lookup, offenders, true) {
			lookup[k] = v
		}
	}
	return lookup, lease, nil
}

// diagramIssueCensusCaseStrings resolves every case label of every switch in
// fn to issue strings. Comparisons of the form `issue == X` are collected as
// well. Unrecognised label shapes are offenders.
func diagramIssueCensusCaseStrings(fset *token.FileSet, fn *ast.FuncDecl, lookup map[string]string, offenders *[]string) map[string]bool {
	out := map[string]bool{}
	resolve := func(e ast.Expr) (string, bool) {
		switch v := e.(type) {
		case *ast.BasicLit:
			if v.Kind == token.STRING {
				return strings.Trim(v.Value, "`\""), true
			}
		case *ast.Ident:
			if val, ok := lookup[v.Name]; ok {
				return val, true
			}
		case *ast.SelectorExpr:
			if val, ok := lookup[v.Sel.Name]; ok {
				return val, true
			}
		}
		return "", false
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CaseClause:
			for _, e := range node.List {
				if val, ok := resolve(e); ok {
					out[val] = true
				} else {
					*offenders = append(*offenders, fn.Name.Name+": unrecognized case label "+diagramIssueCensusPrint(fset, e))
				}
			}
		case *ast.BinaryExpr:
			if node.Op != token.EQL {
				return true
			}
			left, ok := node.X.(*ast.Ident)
			if !ok || left.Name != "issue" {
				return true
			}
			if val, ok := resolve(node.Y); ok {
				out[val] = true
			} else {
				*offenders = append(*offenders, fn.Name.Name+": unrecognized issue comparison "+diagramIssueCensusPrint(fset, node))
			}
		}
		return true
	})
	return out
}

func runDiagramIssueVocabularyCensus(producerSources map[string]string, leaseOverride string, decisions map[string]string) (*diagramIssueVocabularyCensus, error) {
	c := &diagramIssueVocabularyCensus{
		producer: map[string]string{}, explicitLease: map[string]bool{},
		replaceGrant: map[string]bool{}, relationCases: map[string]bool{},
	}
	fset := token.NewFileSet()
	lookup, lease, err := diagramIssueCensusTypesConsts(fset, &c.offenders)
	if err != nil {
		return nil, err
	}
	if leaseOverride != "" {
		lease, err = parser.ParseFile(fset, diagramIssueVocabularyLeaseFile, leaseOverride, 0)
		if err != nil {
			return nil, err
		}
		for k, v := range diagramIssueCensusStringConsts(fset, lease, diagramIssueVocabularyLeaseFile, lookup, &c.offenders, true) {
			lookup[k] = v
		}
	}
	if lease == nil {
		return nil, os.ErrNotExist
	}
	for _, name := range diagramIssueVocabularyProducerFiles {
		file, perr := parser.ParseFile(fset, name, producerSources[name], 0)
		if perr != nil {
			return nil, perr
		}
		for ident, val := range diagramIssueCensusStringConsts(fset, file, name, lookup, &c.offenders, true) {
			if strings.ContainsAny(val, " .:/\\") {
				continue // prose / wording constants are not issue tokens
			}
			if prev, dup := c.producer[val]; dup && prev != name+":"+ident {
				c.offenders = append(c.offenders, "issue value "+val+" declared twice: "+prev+" and "+name+":"+ident)
			}
			c.producer[val] = name + ":" + ident
		}
	}
	found := map[string]bool{}
	for _, decl := range lease.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		switch fn.Name.Name {
		case "answerDiagramRelationRepairFailureCapabilities":
			c.explicitLease = diagramIssueCensusCaseStrings(fset, fn, lookup, &c.offenders)
		case "answerDiagramRelationRepairIssueAllowsReplacement":
			c.replaceGrant = diagramIssueCensusCaseStrings(fset, fn, lookup, &c.offenders)
		case "AnswerDiagramRelationRepairFailureEffectiveRelation":
			c.relationCases = diagramIssueCensusCaseStrings(fset, fn, lookup, &c.offenders)
		default:
			continue
		}
		found[fn.Name.Name] = true
	}
	for _, name := range []string{"answerDiagramRelationRepairFailureCapabilities", "answerDiagramRelationRepairIssueAllowsReplacement", "AnswerDiagramRelationRepairFailureEffectiveRelation"} {
		if !found[name] {
			c.offenders = append(c.offenders, "lease decision function "+name+" not found (census cannot bind the closed set)")
		}
	}
	// Rule 1: every producer issue has a lease decision.
	for val, where := range c.producer {
		if c.explicitLease[val] || c.replaceGrant[val] {
			continue
		}
		if _, ok := decisions[val]; ok {
			continue
		}
		c.offenders = append(c.offenders, "producer issue "+val+" ("+where+") has no lease decision: add an explicit case in answerDiagramRelationRepairFailureCapabilities or record its default-lane decision")
	}
	// Rule 2: every lease label is a producer issue.
	for _, set := range []map[string]bool{c.explicitLease, c.replaceGrant, c.relationCases} {
		for val := range set {
			if _, ok := c.producer[val]; !ok {
				// Lease-native issues (participant mapping) are declared in the
				// lease file itself and resolved through the types lookup.
				if strings.HasPrefix(val, "participant_") {
					continue
				}
				c.offenders = append(c.offenders, "lease names issue "+val+" that no producer declares")
			}
		}
	}
	// Rule 3: decision rows cannot rot.
	for val := range decisions {
		if _, ok := c.producer[val]; !ok {
			c.offenders = append(c.offenders, "stale default-lane decision "+val+" (no producer constant) — prune it")
		}
		if c.explicitLease[val] || c.replaceGrant[val] {
			c.offenders = append(c.offenders, "default-lane decision "+val+" is also an explicit lease label — keep one")
		}
	}
	sort.Strings(c.offenders)
	return c, nil
}

func TestDiagramRelationIssueVocabularyCensus(t *testing.T) {
	sources := map[string]string{}
	for _, name := range diagramIssueVocabularyProducerFiles {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		sources[name] = string(body)
	}
	c, err := runDiagramIssueVocabularyCensus(sources, "", diagramIssueDefaultLaneDecisions)
	if err != nil {
		t.Fatalf("census parse failed (a silent green would defeat the tripwire): %v", err)
	}
	if len(c.offenders) > 0 {
		t.Fatalf("diagram relation issue vocabulary and lease decisions drifted:\n  %s", strings.Join(c.offenders, "\n  "))
	}
	if len(c.producer) < 20 || !c.explicitLease["typed_anchor_reversed_against_visible_edge"] ||
		!c.explicitLease["typed_anchor_without_visible_edge"] || !c.replaceGrant["edge_anchor_node_identity_conflict"] {
		t.Fatalf("census did not bind the live sets: producer=%d explicit=%v replace=%v", len(c.producer), c.explicitLease, c.replaceGrant)
	}

	// Self-red 1: a producer issue minted without a lease decision.
	mutated := map[string]string{}
	for k, v := range sources {
		mutated[k] = v
	}
	mutated["answer_document_diagram_evidence.go"] = strings.Replace(sources["answer_document_diagram_evidence.go"],
		"\tdiagramCallEdgeIssueDuplicateParticipant  = \"duplicate_participant_identity\"\n",
		"\tdiagramCallEdgeIssueDuplicateParticipant  = \"duplicate_participant_identity\"\n\tdiagramCallEdgeIssueUnlisted = \"unlisted_probe_issue\"\n", 1)
	if mutated["answer_document_diagram_evidence.go"] == sources["answer_document_diagram_evidence.go"] {
		t.Fatal("self-red 1 injection marker not found")
	}
	got, err := runDiagramIssueVocabularyCensus(mutated, "", diagramIssueDefaultLaneDecisions)
	if err != nil {
		t.Fatal(err)
	}
	if !diagramIssueCensusHas(got.offenders, "producer issue unlisted_probe_issue") {
		t.Fatalf("self-red 1: undecided producer issue must be reported, got %v", got.offenders)
	}

	// Self-red 2: a lease label naming an issue no producer mints (typo).
	leaseSrc, err := os.ReadFile(diagramIssueVocabularyLeaseFile)
	if err != nil {
		t.Fatal(err)
	}
	typo := strings.Replace(string(leaseSrc), "\tcase \"typed_anchor_without_visible_edge\":\n", "\tcase \"typed_anchor_without_visible_edgee\":\n", 1)
	if typo == string(leaseSrc) {
		t.Fatal("self-red 2 injection marker not found")
	}
	got, err = runDiagramIssueVocabularyCensus(sources, typo, diagramIssueDefaultLaneDecisions)
	if err != nil {
		t.Fatal(err)
	}
	if !diagramIssueCensusHas(got.offenders, "lease names issue typed_anchor_without_visible_edgee") ||
		!diagramIssueCensusHas(got.offenders, "producer issue typed_anchor_without_visible_edge") {
		t.Fatalf("self-red 2: orphaned lease label and its now-undecided producer issue must both be reported, got %v", got.offenders)
	}

	// Self-red 3: a decision row that duplicates an explicit lease case, and
	// one that names no producer constant.
	rotten := map[string]string{}
	for k, v := range diagramIssueDefaultLaneDecisions {
		rotten[k] = v
	}
	rotten["typed_anchor_reversed_against_visible_edge"] = "duplicate"
	rotten["ghost_issue"] = "gone"
	got, err = runDiagramIssueVocabularyCensus(sources, "", rotten)
	if err != nil {
		t.Fatal(err)
	}
	if !diagramIssueCensusHas(got.offenders, "default-lane decision typed_anchor_reversed_against_visible_edge is also an explicit lease label") ||
		!diagramIssueCensusHas(got.offenders, "stale default-lane decision ghost_issue") {
		t.Fatalf("self-red 3: rotten decision rows must be reported, got %v", got.offenders)
	}

	// Self-red 4: an unrecognized case-label shape is an offender, not a skip.
	shape := strings.Replace(string(leaseSrc), "\tcase \"typed_anchor_without_visible_edge\":\n", "\tcase strings.TrimSpace(\"typed_anchor_without_visible_edge\"):\n", 1)
	got, err = runDiagramIssueVocabularyCensus(sources, shape, diagramIssueDefaultLaneDecisions)
	if err != nil {
		t.Fatal(err)
	}
	if !diagramIssueCensusHas(got.offenders, "unrecognized case label") {
		t.Fatalf("self-red 4: unrecognized label shape must fail loud, got %v", got.offenders)
	}
}

func diagramIssueCensusHas(offenders []string, needle string) bool {
	for _, o := range offenders {
		if strings.Contains(o, needle) {
			return true
		}
	}
	return false
}
