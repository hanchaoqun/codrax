package tracequery

// chain_credential_census_structure_test.go — CENSAME-1 件7②
// (CHAINGUARD P4/R8 structural guard, 2026-07-24). Production behavior is
// unchanged; these tests force every future chain-admission field to be
// consciously reconciled with the single credential census mint point.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var chainCredentialCensusProbeFields = map[string]bool{
	"OnChainBasis":                 true,
	"Causality":                    true,
	"SubjectIsAnalysisTarget":      true,
	"ChainIdentityInheritance":     true,
	"ChainCredentialEnvelopeLevel": true,
	"ChainAnchoredMs":              true,
	"Source":                       true,
	"OverlapMs":                    true,
	"StartTs":                      true,
	"EndTs":                        true,
}

func TestChainCredentialCensusProbeReadSetClosed(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "chain_credential_census.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "censusChainSeatCredential" {
			continue
		}
		found = true
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if ok && ident.Name == "item" {
				got[sel.Sel.Name] = true
			}
			return true
		})
	}
	if !found {
		t.Fatal("censusChainSeatCredential production mint point not found")
	}
	if !reflect.DeepEqual(got, chainCredentialCensusProbeFields) {
		t.Fatalf("censusChainSeatCredential item-field read set drifted.\n got: %v\nwant: %v\n"+
			"Classify the new/removed field as a credential probe, update the mint logic, and then update this closed roster.",
			sortedCensusStructureSet(got), sortedCensusStructureSet(chainCredentialCensusProbeFields))
	}
}

type chainCredentialFieldDisposition struct {
	kind   string
	reason string
}

const (
	chainCredentialDispositionProbe  = "census_probe"
	chainCredentialDispositionOutput = "census_output"
	chainCredentialDispositionExempt = "exempt"
)

var chainCredentialFieldDispositions = map[string]chainCredentialFieldDisposition{
	"ChainAnchoredMs":              {chainCredentialDispositionProbe, "measured anchored share is an interval credential"},
	"ChainIdentityInheritance":     {chainCredentialDispositionProbe, "typed member-inheritance admission record"},
	"ChainCredentialEnvelopeLevel": {chainCredentialDispositionProbe, "typed envelope-level interval credential"},
	"Source":                       {chainCredentialDispositionProbe, "constructive wakeup_chain source prefix is a credential"},
	"Causality":                    {chainCredentialDispositionProbe, "self causality tokens are target credentials"},
	"OnChainBasis":                 {chainCredentialDispositionProbe, "self and host-edge basis tokens are credentials"},
	"OverlapMs":                    {chainCredentialDispositionProbe, "positive measured overlap joins a valid interval"},
	"SubjectIsAnalysisTarget":      {chainCredentialDispositionProbe, "R8 typed target identity is the self credential"},

	"ChainCredentialCensus": {chainCredentialDispositionOutput, "closed census verdict minted by the guarded function"},

	"ChainAnchorFullMs":                 {chainCredentialDispositionExempt, "full-window value account, not chain admission"},
	"ChainAnchorRemainderSeat":          {chainCredentialDispositionExempt, "post-accounting adjacent remainder marker"},
	"ChainAnchorOwnershipDivergent":     {chainCredentialDispositionExempt, "ownership-account disclosure, not admission"},
	"ChainAnchorChainLaneMs":            {chainCredentialDispositionExempt, "chain-lane value account only"},
	"ChainAnchorCensusMs":               {chainCredentialDispositionExempt, "census value account only"},
	"ChainCredentialLaneDemoted":        {chainCredentialDispositionExempt, "lane-demotion disposition, not a credential"},
	"ChainAnchorRepresentedByChainSeat": {chainCredentialDispositionExempt, "duplicate-seat representation marker"},
	"ChainRelevance":                    {chainCredentialDispositionExempt, "ordinal-channel output consumed before census"},
	"ChainDepth":                        {chainCredentialDispositionExempt, "display depth metadata"},
	"ChainBranch":                       {chainCredentialDispositionExempt, "display branch metadata"},
}

func TestRootCauseRankChainAdmissionFieldsHaveCensusDisposition(t *testing.T) {
	typ := reflect.TypeOf(RootCauseRankItem{})
	present := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !chainCredentialDispositionCandidate(name) {
			continue
		}
		present[name] = true
		disposition, ok := chainCredentialFieldDispositions[name]
		if !ok {
			t.Errorf("%s is a new chain-admission-family field with no census disposition; choose census_probe (and wire it into censusChainSeatCredential), census_output, or exempt with a concrete reason", name)
			continue
		}
		switch disposition.kind {
		case chainCredentialDispositionProbe:
			if !chainCredentialCensusProbeFields[name] {
				t.Errorf("%s is classified census_probe but the guarded mint point does not read it", name)
			}
		case chainCredentialDispositionOutput:
			if name != "ChainCredentialCensus" {
				t.Errorf("%s is classified census_output; review whether the closed census really owns another output", name)
			}
		case chainCredentialDispositionExempt:
			if strings.TrimSpace(disposition.reason) == "" {
				t.Errorf("%s exemption must state why it is not a chain credential", name)
			}
		default:
			t.Errorf("%s has unknown census disposition %q", name, disposition.kind)
		}
	}
	for name := range chainCredentialFieldDispositions {
		if !present[name] {
			t.Errorf("stale census disposition %s: RootCauseRankItem no longer has that chain-admission-family field", name)
		}
	}
}

func chainCredentialDispositionCandidate(name string) bool {
	if strings.HasPrefix(name, "Chain") || strings.HasPrefix(name, "OnChain") {
		return true
	}
	switch name {
	case "Causality", "SubjectIsAnalysisTarget", "OverlapMs", "Source":
		return true
	default:
		return false
	}
}

func sortedCensusStructureSet(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
