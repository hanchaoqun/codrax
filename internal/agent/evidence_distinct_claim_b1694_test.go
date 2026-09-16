package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type b1694PublicEvidence struct {
	definition types.EvidenceItem
	concrete   []types.EvidenceItem
	rows       []types.EvidenceItem
	ledger     types.ObservationLedger
	prompt     string
}

// Reuse B1692's real parser fixture and its ReadFile -> EmitEvidence ->
// Explorer -> ledger -> finalizer path, but emit the actual declaration even
// when it shares a line with the return. No alternate call-site witness avoids
// the colliding coordinate, and no fixture supplies a deterministic claim.
func b1694PublicSameCoordinate(t *testing.T, file, source string, declarationLine int) b1694PublicEvidence {
	t.Helper()
	graph, repo := b1627ParsedGraph(t, map[string]string{file: source})
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Language: "en", Mutable: types.NewMutableState("Explain make and its results"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)}, PredicateAxis: types.AxisCall,
			CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "make", SinkMode: types.CallChainSinkResolutionDiscoverTerminal},
		}},
	}
	bus.Mutable.SetSearchGraph(graph)
	params, _ := json.Marshal(map[string]any{"path": file, "line_offset": 0, "limit": 100})
	read, err := (&tool.ReadFile{}).Execute(bus, params)
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("real source read prerequisite failed: %v %+v", err, read)
	}
	bus.Mutable.AppendDispatchToolResult(read)
	params, _ = json.Marshal(map[string]any{"items": []map[string]any{{
		"scope": "line", "evidence_kind": "direct", "subject": "make", "predicate": "defined",
		"source": file, "line_start": declarationLine, "anchor_kind": "definition", "anchor_symbol": "make",
		"summary": "The operation is defined here.",
	}}})
	emit, err := (&tool.EmitEvidence{}).Execute(bus, params)
	if err != nil || !emit.Success {
		t.Fatalf("real grounded declaration prerequisite failed: %v %+v rows=%+v", err, emit, bus.Mutable.EmittedEvidence())
	}
	out := b1694PublicEvidence{}
	definitions := 0
	for _, item := range bus.Mutable.EmittedEvidence() {
		if item.Subject == "make" && item.AnchorKind == types.AnchorDefinition && item.IsCitable() {
			out.definition = item
			definitions++
		}
	}
	if definitions != 1 || out.definition.LineStart != declarationLine {
		t.Fatalf("declaration witness moved away from return coordinate: %+v", out.definition)
	}
	bus.Mutable.AppendDispatchToolResult(emit)
	ctx := promptcontext.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	explorer := &explorerEvaluator{}
	explorer.BuildInitialInstruction(ctx, nil)
	explorer.searchResult = &keywordSearchResult{Graph: graph}
	handoff, err := explorer.ParseOutput(ctx, nil, []types.ToolResult{read, emit}, nil)
	if err != nil || handoff == nil || explorer.cachedConcreteValues == nil {
		t.Fatalf("real Explorer return-enrichment prerequisite failed: %v %+v", err, handoff)
	}
	for _, row := range explorer.cachedConcreteValues.evidence {
		if row.Subject == "make" && row.Producer == "concrete_values" && row.Predicate == "returns" && !types.EvidenceIsDerivationCandidate(row) {
			if row.LineStart != declarationLine || row.LineEnd != declarationLine || row.AnchorKind != types.AnchorReturn {
				t.Fatalf("parser-derived return did not share the declaration coordinate: %+v", row)
			}
			out.concrete = append(out.concrete, row)
		}
	}
	if len(out.concrete) == 0 {
		t.Fatalf("real parser failed to produce owner-bound concrete returns: %+v", graph.FileIndex[file].CallableReturnExpressions)
	}
	out.rows = handoff.EvidenceItems
	bus.EvidenceItems = out.rows
	before, _ := json.Marshal(out.rows)
	out.ledger = types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
	finalCtx := promptcontext.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	out.prompt = (&answerDocumentEvaluator{}).BuildInitialInstruction(finalCtx, nil)
	after, _ := json.Marshal(out.rows)
	current, err := os.ReadFile(filepath.Join(repo, file))
	if err != nil || !bytes.Equal(current, []byte(source)) || !bytes.Equal(before, after) {
		t.Fatal("handoff/ledger/finalizer changed source or accepted model evidence")
	}
	return out
}

func TestB1694PublicSameCoordinateDefinitionKeepsIndependentReturns(t *testing.T) {
	for _, tc := range []struct {
		name, file, source string
		line               int
	}{
		{"javascript_arrow", "factory.js", "export const make = () => \"actual\";\nfunction invoke() { return make(); }\n", 1},
		{"typescript_arrow", "factory.ts", "export const make = (): string => \"actual\";\nfunction invoke() { return make(); }\n", 1},
		{"arkts_arrow", "factory.ets", "export const make = (): string => \"actual\";\nfunction invoke() { return make(); }\n", 1},
		{"rust_tail", "factory.rs", "fn make() -> &'static str { \"actual\" }\nfn invoke() { make(); }\n", 1},
		{"go_explicit", "factory.go", "package sample\nfunc make() string { return \"actual\" }\nfunc invoke() { make() }\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := b1694PublicSameCoordinate(t, tc.file, tc.source, tc.line)
			b1694AssertDistinctReturnCarriers(t, got, []string{`"actual"`})
		})
	}
}

func b1694AssertDistinctReturnCarriers(t *testing.T, got b1694PublicEvidence, values []string) {
	t.Helper()
	definitions, dataflowReturns := 0, 0
	returns := map[string]types.EvidenceItem{}
	for _, row := range got.rows {
		if strings.HasPrefix(row.Producer, "dataflow") {
			t.Logf("separate dataflow carrier: kind=%s predicate=%s subject=%s object=%q", row.Kind, row.Predicate, row.Subject, row.Object)
			if row.Predicate == "returns" {
				dataflowReturns++
			}
		}
		if row.Subject != "make" {
			continue
		}
		if row.AnchorKind == types.AnchorDefinition && row.IsCitable() {
			definitions++
			t.Logf("definition carrier: predicate=%s summary=%q", row.Predicate, row.Summary)
		}
		if row.Predicate == "returns" && row.Producer == "concrete_values" && !types.EvidenceIsDerivationCandidate(row) {
			returns[row.Object] = row
		}
	}
	t.Logf("independent concrete before=%d after=%d; separate dataflow returns=%d", len(got.concrete), len(returns), dataflowReturns)
	if dataflowReturns == 0 {
		t.Error("the independent dataflow return lane must remain present; this regression concerns the concrete_values claim")
	}
	if definitions != 1 {
		t.Errorf("grounded definition must remain independently visible, got %d", definitions)
	}
	for _, value := range values {
		row, ok := returns[value]
		if !ok {
			t.Errorf("same-coordinate definition swallowed independent concrete returns %s; summary prose/dataflow are not that claim", value)
		} else if row.AnchorKind != types.AnchorReturn || row.ID == got.definition.ID {
			t.Errorf("return borrowed definition identity: %+v", row)
		}
		found := false
		for _, record := range got.ledger.Records {
			if record.Producer == "concrete_values" && record.Subject == "make" && record.Predicate == "returns" && record.Object == value && record.AnchorKind == types.AnchorReturn && record.ClaimAuthority == types.ObservationClaimAuthorityIndependentlyProven {
				found = true
			}
		}
		if !found {
			t.Errorf("independent concrete returns %s missing from compiled ledger", value)
		}
		for _, original := range got.concrete {
			if original.Object != value {
				continue
			}
			if original.ID == "" {
				t.Fatal("native concrete return needs its own stable identity")
			}
			foundCapsule := false
			for _, line := range strings.Split(got.prompt, "\n") {
				if strings.Contains(line, "family=`value_or_factory_flow`") && strings.Contains(line, "relation=`returns`") && strings.Contains(line, "subject=`make`") && strings.Contains(line, "object=`"+value+"`") && strings.Contains(line, "["+original.ID+"]") {
					foundCapsule = true
				}
			}
			if !foundCapsule {
				t.Errorf("independent concrete return identity %s missing from finalizer typed value capsule", original.ID)
			}
		}
	}
}

func TestB1694ActualCarriersKeepDistinctClaimsUnderReversalAndDuplicates(t *testing.T) {
	const source = "package sample\nfunc make(flag bool) string { if flag { return \"actual\" }; return \"other\" }\nfunc invoke() { make(true) }\n"
	got := b1694PublicSameCoordinate(t, "factory.go", source, 2)
	values := map[string]bool{}
	for _, row := range got.concrete {
		values[row.Object] = true
	}
	if len(got.concrete) != 2 || !values[`"actual"`] || !values[`"other"`] {
		t.Fatalf("real parser must publish two different same-line return expressions before merging: %+v", got.concrete)
	}
	b1694AssertDistinctReturnCarriers(t, got, []string{`"actual"`, `"other"`})
	for _, reverse := range []bool{false, true} {
		for _, repeat := range []bool{false, true} {
			t.Run(fmt.Sprintf("reverse=%t/repeat=%t", reverse, repeat), func(t *testing.T) {
				rows := append([]types.EvidenceItem{got.definition}, got.concrete...)
				if reverse {
					for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
						rows[i], rows[j] = rows[j], rows[i]
					}
				}
				if repeat {
					rows = append(rows, rows...)
				}
				before, _ := json.Marshal(rows)
				merged := MergeEvidenceItems(rows)
				after, _ := json.Marshal(rows)
				if !bytes.Equal(before, after) {
					t.Fatal("claim merge mutated its accepted input carriers")
				}
				if len(merged) != 3 {
					t.Errorf("distinct definition and both return values require 3 carriers, got %d: %+v", len(merged), merged)
				}
				found := map[string]int{}
				for _, row := range merged {
					found[string(row.AnchorKind)+"/"+row.Predicate+"/"+row.Object]++
				}
				for _, key := range []string{"definition/defined/", `return/returns/"actual"`, `return/returns/"other"`} {
					if found[key] != 1 {
						t.Errorf("exact semantic claim %q appeared %d times, want 1", key, found[key])
					}
				}
			})
		}
	}
}
