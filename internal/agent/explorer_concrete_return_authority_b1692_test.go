package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This exercises the actual parser, source reader, model evidence emitter,
// Explorer handoff, ledger compiler, and finalizer prompt. No returned-value
// evidence or parser node is supplied by the fixture itself.
func b1692PublicReturnHandoff(t *testing.T, source, owner string, declarationLine int) ([]types.EvidenceItem, types.ObservationLedger, string) {
	t.Helper()
	return b1692PublicReturnHandoffFile(t, "matcher.rs", source, owner, "is_match", declarationLine)
}

func TestB1692ReturnReceiptReadAndCacheBoundaries(t *testing.T) {
	const source = "package p\nfunc make() string {\n return \"actual\"\n}\n"
	graph, repo := b1627ParsedGraph(t, map[string]string{"factory.go": source})
	fi := graph.FileIndex["factory.go"]
	var sym repotypes.Symbol
	for _, s := range fi.Symbols {
		if s.Name == "make" {
			sym = s
		}
	}
	reader := repotypes.NewCallableReturnReader(fi, []byte(source))
	if len(reader.For(sym)) != 1 {
		t.Fatal("real parser prerequisite: expected one return receipt")
	}
	for _, tc := range []struct {
		name            string
		read, candidate bool
		anchor          types.AnchorKind
		want            bool
	}{
		{"unread_no_evidence", false, false, "", false},
		{"unread_exact_definition", false, false, types.AnchorDefinition, true},
		{"unread_candidate_definition", false, true, types.AnchorDefinition, false},
		{"unread_exact_assignment", false, false, types.AnchorAssignment, true},
		{"unread_candidate_assignment", false, true, types.AnchorAssignment, false},
		{"unread_exact_argument", false, false, types.AnchorArgument, true},
		{"unread_candidate_argument", false, true, types.AnchorArgument, false},
		{"read_plus_candidate", true, true, types.AnchorDefinition, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			closure := types.NewEvidenceClosure(repo)
			if tc.read {
				closure.AddReadSet(map[string]bool{"factory.go": true})
				closure.AddReadRanges(map[string][]types.LineRange{"factory.go": {{Start: 3, End: 3}}})
			}
			var evidence []types.EvidenceItem
			if tc.anchor != "" {
				evidence = []types.EvidenceItem{{ID: "line", Source: "factory.go", LineStart: 3, LineEnd: 3, Scope: types.ScopeLine, AnchorKind: tc.anchor, GroundingStatus: types.GroundingGrounded, DerivationCandidate: tc.candidate}}
			}
			before, _ := json.Marshal(evidence)
			got := concreteCallableReturnValues(fi, sym, reader, nil, closure, evidence)
			if (len(got) == 1) != tc.want {
				t.Fatalf("read/precise-evidence admission=%+v want one=%t", got, tc.want)
			}
			after, _ := json.Marshal(evidence)
			if !bytes.Equal(before, after) {
				t.Fatal("read adapter mutated evidence")
			}
		})
	}
	for _, mutation := range []string{"receipt_removed", "source_changed", "source_missing", "owner_changed"} {
		t.Run("warm_"+mutation, func(t *testing.T) {
			g, root := b1627ParsedGraph(t, map[string]string{"factory.go": source})
			eval := &explorerEvaluator{searchResult: &keywordSearchResult{Graph: g}}
			read := map[string]bool{"factory.go": true}
			countReturns := func(r concreteValuesResult) int {
				n := 0
				for _, item := range r.evidence {
					if item.Producer == "concrete_values" && item.Predicate == "returns" && !types.EvidenceIsDerivationCandidate(item) {
						n++
					}
				}
				return n
			}
			first := eval.getConcreteValuesCached(context.Background(), root, read, nil)
			if countReturns(first) != 1 {
				t.Fatalf("cold parser return prerequisite missing: %+v", first.evidence)
			}
			cached := eval.cachedConcreteValues
			if countReturns(eval.getConcreteValuesCached(context.Background(), root, read, nil)) != 1 || eval.cachedConcreteValues != cached {
				t.Fatal("unchanged snapshot failed to use warm cache")
			}
			switch mutation {
			case "receipt_removed":
				g.FileIndex["factory.go"].CallableReturnExpressions = nil
			case "source_changed":
				if err := os.WriteFile(filepath.Join(root, "factory.go"), []byte(strings.Replace(source, "actual", "change", 1)), 0600); err != nil {
					t.Fatal(err)
				}
			case "source_missing":
				if err := os.Remove(filepath.Join(root, "factory.go")); err != nil {
					t.Fatal(err)
				}
			case "owner_changed":
				for i := range g.FileIndex["factory.go"].Symbols {
					if g.FileIndex["factory.go"].Symbols[i].Name == "make" {
						g.FileIndex["factory.go"].Symbols[i].Parent = "foreign"
					}
				}
			}
			fresh := eval.getConcreteValuesCached(context.Background(), root, read, nil)
			if countReturns(fresh) != 0 || eval.cachedConcreteValues == cached {
				t.Fatalf("warm cache kept obsolete return authority: %+v", fresh.evidence)
			}
			if mutation != "source_missing" {
				lead := false
				for _, item := range fresh.evidence {
					if item.Producer == "concrete_values" && item.Predicate == concreteValueSourceExpression && types.EvidenceIsDerivationCandidate(item) && item.AnchorKind != types.AnchorReturn {
						lead = true
					}
				}
				if !lead {
					t.Fatalf("unproved current source lost its inspection lead: %+v", fresh.evidence)
				}
			}
		})
	}
}

func TestB1692ReturnReceiptRequiresCompleteReadSpan(t *testing.T) {
	const source = "package p\nfunc make() string {\n return join(\n  \"actual\",\n )\n}\n"
	graph, repo := b1627ParsedGraph(t, map[string]string{"factory.go": source})
	fi := graph.FileIndex["factory.go"]
	var sym repotypes.Symbol
	for _, candidate := range fi.Symbols {
		if candidate.Name == "make" {
			sym = candidate
		}
	}
	reader := repotypes.NewCallableReturnReader(fi, []byte(source))
	receipts := reader.For(sym)
	if len(receipts) != 1 || receipts[0].LineStart != 3 || receipts[0].LineEnd != 5 {
		t.Fatalf("real parser prerequisite: expected exact multiline return, got %+v", receipts)
	}
	for _, tc := range []struct {
		name      string
		end       int
		candidate bool
		want      bool
	}{
		{"start_only", 3, false, false},
		{"missing_end", 4, false, false},
		{"complete", 5, false, true},
		{"candidate_cannot_supply_end", 4, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			closure := types.NewEvidenceClosure(repo)
			closure.AddReadSet(map[string]bool{"factory.go": true})
			closure.AddReadRanges(map[string][]types.LineRange{"factory.go": {{Start: 3, End: tc.end}}})
			var evidence []types.EvidenceItem
			if tc.candidate {
				evidence = []types.EvidenceItem{{ID: "end", Source: "factory.go", LineStart: 5, LineEnd: 5, Scope: types.ScopeLine, AnchorKind: types.AnchorReturn, GroundingStatus: types.GroundingGrounded, DerivationCandidate: true}}
			}
			got := concreteCallableReturnValues(fi, sym, reader, nil, closure, evidence)
			if (len(got) == 1) != tc.want {
				t.Fatalf("incomplete source span acquired return authority: %+v want one=%t", got, tc.want)
			}
			if tc.want && (got[0].value != receipts[0].Expression || got[0].line != 3 || concreteValueLastLine(got[0]) != 5) {
				t.Fatalf("complete multiline expression changed in projection: %+v", got)
			}
		})
	}
}

func TestB1692PublicMultilineReturnPreservesExpressionAndSpan(t *testing.T) {
	const source = "package p\nfunc make() string {\n return join(\n  \"actual\",\n )\n}\nfunc invoke() { make() }\n"
	const expression = "join(\n  \"actual\",\n )"
	rows, ledger, _ := b1692PublicReturnHandoffFile(t, "factory.go", source, "make", "make", 2)
	foundRow, foundRecord := false, false
	for _, row := range rows {
		if row.Producer == "concrete_values" && row.Subject == "make" && row.Predicate == "returns" && row.Object == expression {
			if row.LineStart != 3 || row.LineEnd != 5 || row.ValidateScope() != nil || types.EvidenceIsDerivationCandidate(row) {
				t.Fatalf("public multiline return changed extent or authority: %+v", row)
			}
			foundRow = true
		}
	}
	for _, record := range ledger.Records {
		if record.Producer == "concrete_values" && record.Subject == "make" && record.Predicate == "returns" && record.Object == expression {
			if record.Span.LineStart != 3 || record.Span.LineEnd != 5 || record.ClaimAuthority != types.ObservationClaimAuthorityIndependentlyProven {
				t.Fatalf("multiline source scope was lost in the ledger: %+v", record)
			}
			foundRecord = true
		}
	}
	if !foundRow || !foundRecord {
		t.Fatalf("public parser return missing after handoff: row=%t record=%t rows=%+v", foundRow, foundRecord, rows)
	}
}

func TestB1692WarmReturnCacheTracksExactEvidenceQualification(t *testing.T) {
	const source = "package p\nfunc make() string {\n return \"actual\"\n}\n"
	for _, mutation := range []string{"candidate", "moved", "range", "uncitable", "self_producer"} {
		t.Run(mutation, func(t *testing.T) {
			graph, repo := b1627ParsedGraph(t, map[string]string{"factory.go": source})
			closure := types.NewEvidenceClosure(repo)
			read := map[string]bool{"factory.go": true}
			closure.SetReadSet(read)
			closure.AddReadRanges(map[string][]types.LineRange{"factory.go": {{Start: 2, End: 2}}})
			item := types.EvidenceItem{ID: "exact", Source: "factory.go", Subject: "make", LineStart: 3, LineEnd: 3, Scope: types.ScopeLine, AnchorKind: types.AnchorReturn, GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerExplorerEmitEvidence}
			eval := &explorerEvaluator{searchResult: &keywordSearchResult{Graph: graph}, structuredEvidence: []types.EvidenceItem{item}}
			count := func(result concreteValuesResult) int {
				n := 0
				for _, row := range result.evidence {
					if row.Producer == "concrete_values" && row.Predicate == "returns" && !types.EvidenceIsDerivationCandidate(row) {
						n++
					}
				}
				return n
			}
			if count(eval.getConcreteValuesCached(context.Background(), repo, read, closure)) != 1 {
				t.Fatal("cold exact source coordinate must independently admit one return")
			}
			switch mutation {
			case "candidate":
				item.DerivationCandidate = true
			case "moved":
				item.LineStart, item.LineEnd = 4, 4
			case "range":
				item.LineEnd = 4
			case "uncitable":
				item.GroundingStatus = types.GroundingUngrounded
			case "self_producer":
				item.Producer = "concrete_values"
			}
			eval.structuredEvidence = []types.EvidenceItem{item}
			fresh := &explorerEvaluator{searchResult: &keywordSearchResult{Graph: graph}, structuredEvidence: []types.EvidenceItem{item}}
			warmCount := count(eval.getConcreteValuesCached(context.Background(), repo, read, closure))
			freshCount := count(fresh.getConcreteValuesCached(context.Background(), repo, read, closure))
			if warmCount != 0 || freshCount != 0 {
				t.Fatalf("changed exact coordinate retained return proof: warm=%d fresh=%d", warmCount, freshCount)
			}
		})
	}
}

func TestB1692ReturnEnrichmentCannotSignItsOwnReadCoverage(t *testing.T) {
	for _, producer := range []string{"concrete_values", "bridge_literal", "bridge_literal_terminal", types.EvidenceProducerRepoMapDynamicSelectorAssignment, types.EvidenceProducerRepoMapDynamicSelectorReturn, types.EvidenceProducerRepoMapDynamicSelectorArgument} {
		t.Run(producer, func(t *testing.T) {
			item := types.EvidenceItem{Source: "factory.go", LineStart: 3, LineEnd: 3, Scope: types.ScopeLine, AnchorKind: types.AnchorReturn, GroundingStatus: types.GroundingGrounded, Producer: producer}
			if runtimeTargetReadOrExactEvidenceLineAllowed("factory.go", 3, nil, nil, []types.EvidenceItem{item}) {
				t.Fatal("cached enrichment signed its own unread source coordinate")
			}
			if !runtimeTargetReadOrExactEvidenceLineAllowed("factory.go", 3, map[string]bool{"factory.go": true}, nil, []types.EvidenceItem{item}) {
				t.Fatal("independent real read was blocked by a same-coordinate enrichment")
			}
		})
	}
}

func b1692PublicReturnHandoffFile(t *testing.T, file, source, owner, anchor string, declarationLine int) ([]types.EvidenceItem, types.ObservationLedger, string) {
	t.Helper()
	graph, repo := b1627ParsedGraph(t, map[string]string{file: source})
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Language: "en",
		Mutable: types.NewMutableState("Explain the matching operation and its results"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
			AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			PredicateAxis:            types.AxisCall,
			CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: owner, SinkMode: types.CallChainSinkResolutionDiscoverTerminal},
		}},
	}
	bus.Mutable.SetSearchGraph(graph)
	readParams, _ := json.Marshal(map[string]any{"path": file, "line_offset": 0, "limit": 100})
	read, err := (&tool.ReadFile{}).Execute(bus, readParams)
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("actual source read failed: %v %+v", err, read)
	}
	bus.Mutable.AppendDispatchToolResult(read)
	item := map[string]any{
		"scope": "line", "evidence_kind": "direct", "subject": owner,
		"predicate": "defined", "source": file, "line_start": declarationLine,
		"anchor_kind": "definition", "anchor_symbol": anchor, "summary": "The operation is defined here.",
	}
	if declarationLine < 0 {
		// Use a separate actual call-site witness when declaration and return
		// share a line. This avoids the unrelated legacy same-line evidence
		// merger absorbing a return into the definition carrier.
		item["evidence_kind"], item["subject"], item["predicate"], item["object"] = "relationship", "invoke", "calls", owner
		item["line_start"], item["anchor_kind"], item["summary"] = -declarationLine, "call", "The operation is invoked here."
	}
	raw, _ := json.Marshal(map[string]any{"items": []map[string]any{item}})
	emit, err := (&tool.EmitEvidence{}).Execute(bus, raw)
	if err != nil || !emit.Success || len(bus.Mutable.EmittedEvidence()) == 0 || !bus.Mutable.EmittedEvidence()[0].IsCitable() {
		t.Fatalf("actual grounded owner prerequisite failed: %v %+v evidence=%+v", err, emit, bus.Mutable.EmittedEvidence())
	}
	bus.Mutable.AppendDispatchToolResult(emit)
	ctx := promptcontext.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	explorer := &explorerEvaluator{}
	explorer.BuildInitialInstruction(ctx, nil)
	explorer.searchResult = &keywordSearchResult{Graph: graph}
	out, err := explorer.ParseOutput(ctx, nil, []types.ToolResult{read, emit}, nil)
	if err != nil || out == nil {
		t.Fatalf("actual Explorer handoff failed: %v %+v", err, out)
	}
	if file == "matcher.rs" {
		for _, receipt := range graph.FileIndex[file].CallableReturnExpressions {
			t.Logf("parser receipt: owner=%s line=%d value=%q", receipt.CallableName, receipt.LineStart, receipt.Expression)
		}
		if explorer.cachedConcreteValues != nil {
			for _, row := range explorer.cachedConcreteValues.evidence {
				if row.Predicate == "returns" {
					t.Logf("premerge enrichment: owner=%s line=%d value=%q", row.Subject, row.LineStart, row.Object)
				}
			}
		}
	}
	bus.EvidenceItems = out.EvidenceItems
	before, _ := json.Marshal(out.EvidenceItems)
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
	finalCtx := promptcontext.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(finalCtx, nil)
	after, _ := json.Marshal(out.EvidenceItems)
	currentSource, err := os.ReadFile(filepath.Join(repo, file))
	if err != nil || !bytes.Equal(currentSource, []byte(source)) || !bytes.Equal(before, after) {
		t.Fatal("handoff/ledger/prompt changed source bytes or accepted evidence")
	}
	return out.EvidenceItems, ledger, prompt
}

func TestB1692FinalizerCandidateReturnDoesNotBorrowProof(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, sharedID := range []bool{false, true} {
			t.Run(fmt.Sprintf("reverse=%t/shared_id=%t", reverse, sharedID), func(t *testing.T) {
				proven := types.EvidenceItem{ID: "value", Kind: types.EvidenceConcrete, Producer: "concrete_values", Subject: "Choose", Predicate: "returns", Object: `"actual"`, Source: "chooser.go", LineStart: 2, LineEnd: 2, Scope: types.ScopeLine, AnchorKind: types.AnchorReturn}
				candidate := proven
				candidate.ID, candidate.Object, candidate.DerivationCandidate = "candidate", `factory(input)`, true
				if sharedID {
					candidate.ID = proven.ID
				}
				rows := []types.EvidenceItem{proven, candidate}
				if reverse {
					rows[0], rows[1] = rows[1], rows[0]
				}
				ctx := &types.AgentContext{Language: "en", EvidenceItems: rows, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "Choose", SinkMode: types.CallChainSinkResolutionDiscoverTerminal}}}}
				before, _ := json.Marshal(ctx.EvidenceItems)
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				for _, line := range strings.Split(prompt, "\n") {
					if strings.Contains(line, "family=`value_or_factory_flow`") && (strings.Contains(line, "factory(input)") || sharedID) {
						t.Errorf("candidate became a proven return in the finalizer capsule: %s", line)
					}
				}
				if !sharedID && !strings.Contains(prompt, "object=`\"actual\"`") {
					t.Fatal("independent source return must retain its capsule capability")
				}
				after, _ := json.Marshal(ctx.EvidenceItems)
				if !bytes.Equal(before, after) {
					t.Fatal("prompt consumer changed accepted evidence")
				}
			})
		}
	}
}

func TestB1692PublicRustBranchAssignmentIsNotReturnAuthority(t *testing.T) {
	const source = `struct PatternMatcher;
impl PatternMatcher {
    fn is_match(&self, line: &str) -> bool {
        let mut rest = line;
        for token in ["a", "b"] {
            match rest.find(token) {
                Some(i) => rest = &rest[i + token.len()..],
                None => return false,
            }
        }
        true
    }
}
`
	rows, ledger, prompt := b1692PublicReturnHandoff(t, source, "PatternMatcher.is_match", 3)
	if !strings.Contains(prompt, "Typed relation capsule") {
		t.Fatal("public finalizer must enter the actual typed relation capsule lane")
	}
	var foundOwner bool
	proved := map[string]bool{}
	var assignmentLead bool
	for _, row := range rows {
		if row.Subject == "PatternMatcher.is_match" && row.AnchorKind == types.AnchorDefinition && row.IsCitable() {
			foundOwner = true
		}
		if row.Producer == "concrete_values" && row.Predicate == "returns" && strings.Contains(row.Object, "rest =") && !types.EvidenceIsDerivationCandidate(row) {
			t.Errorf("branch assignment was minted as independent owner return: %+v", row)
		}
		if row.Producer == "concrete_values" && row.Subject == "PatternMatcher.is_match" && row.Predicate == "returns" && !types.EvidenceIsDerivationCandidate(row) {
			proved[row.Object] = true
		}
		if row.Source == "matcher.rs" && row.LineStart == 7 && strings.Contains(row.Object, "Some(i) => rest =") && types.EvidenceIsDerivationCandidate(row) && row.AnchorKind != types.AnchorReturn {
			assignmentLead = true
		}
	}
	if !foundOwner {
		t.Fatal("public owner authority disappeared before the counterexample")
	}
	if !proved["false"] || !proved["true"] {
		t.Errorf("actual explicit return and terminal tail must survive with the exact owning method: %+v", proved)
		for _, row := range rows {
			if row.Predicate == "returns" || row.LineStart == 11 {
				t.Logf("return/terminal carrier: producer=%s subject=%s line=%d kind=%s anchor=%s object=%q", row.Producer, row.Subject, row.LineStart, row.Kind, row.AnchorKind, row.Object)
			}
		}
	}
	if !assignmentLead {
		t.Error("unproved lexical branch must remain an original-source inspection lead, not a return anchor")
	}
	ledgerLead := false
	for _, record := range ledger.Records {
		if record.Producer == "concrete_values" && record.AnchorKind == types.AnchorReturn && record.Span.LineStart == 7 && record.ClaimAuthority == types.ObservationClaimAuthorityIndependentlyProven {
			t.Errorf("ledger upgraded branch assignment to proven return: %+v", record)
		}
		if record.Producer == "concrete_values" && record.Span.LineStart == 7 && record.Predicate == concreteValueSourceExpression && strings.Contains(record.Object, "Some(i) => rest =") && record.AnchorKind != types.AnchorReturn {
			ledgerLead = true
		}
	}
	if !ledgerLead || !strings.Contains(prompt, "rest[i + token.len()..]") {
		t.Fatal("original branch source must remain available through the ledger and finalizer as an inspection lead")
	}
	for _, line := range strings.Split(prompt, "\n") {
		if strings.Contains(line, "family=`value_or_factory_flow`") && strings.Contains(line, "rest =") {
			t.Errorf("finalizer received false typed return capsule: %s", line)
		}
	}
	if !strings.Contains(prompt, "PatternMatcher.is_match") {
		t.Fatal("owner/source context must remain available")
	}
}

func TestB1692PublicCallableReturnLanguagesAndNestedOwners(t *testing.T) {
	for _, tc := range []struct {
		name, file, source, owner, anchor, value string
		line                                     int
	}{
		{"javascript_arrow", "factory.js", "export const make = () => \"actual\";\nfunction invoke() { return make(); }\n", "make", "make", `"actual"`, -2},
		{"typescript_arrow", "factory.ts", "export const make = (): string => \"actual\";\nfunction invoke() { return make(); }\n", "make", "make", `"actual"`, -2},
		{"arkts_arrow", "factory.ets", "export const make = (): string => \"actual\";\nfunction invoke() { return make(); }\n", "make", "make", `"actual"`, -2},
		{"javascript_nested_same_line", "factory.js", "export function make() { const inner = () => \"nested\"; return \"actual\"; }\nfunction invoke() { return make(); }\n", "make", "make", `"actual"`, -2},
		{"rust_nested_same_line", "factory.rs", "fn make() -> &'static str { let inner = || { return \"nested\"; }; \"actual\" }\nfn invoke() { make(); }\n", "make", "make", `"actual"`, -2},
		{"rust_tail", "factory.rs", "fn make() -> bool {\n true\n}\n", "make", "make", "true", 1},
		{"go_explicit", "factory.go", "package sample\nfunc make() string { return \"actual\" }\nfunc invoke() { make() }\n", "make", "make", `"actual"`, -3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, ledger, prompt := b1692PublicReturnHandoffFile(t, tc.file, tc.source, tc.owner, tc.anchor, tc.line)
			found := false
			for _, row := range rows {
				if row.Producer != "concrete_values" || row.Subject != tc.owner || row.Predicate != "returns" || types.EvidenceIsDerivationCandidate(row) {
					continue
				}
				if row.Object == tc.value {
					found = true
				}
				if strings.Contains(row.Object, "nested") {
					t.Errorf("nested callable return borrowed outer owner: %+v", row)
				}
			}
			if !found {
				t.Errorf("true language return disappeared: want %s -> %s; rows=%+v", tc.owner, tc.value, rows)
			}
			for _, record := range ledger.Records {
				if record.Producer == "concrete_values" && record.Subject == tc.owner && record.AnchorKind == types.AnchorReturn && record.ClaimAuthority == types.ObservationClaimAuthorityIndependentlyProven && strings.Contains(record.Object, "nested") {
					t.Errorf("nested return became independently proven outer result: %+v", record)
				}
			}
			for _, line := range strings.Split(prompt, "\n") {
				if strings.Contains(line, "family=`value_or_factory_flow`") && strings.Contains(line, "subject=`"+tc.owner+"`") && strings.Contains(line, "nested") {
					t.Errorf("nested return leaked as outer typed capsule: %s", line)
				}
			}
		})
	}
}
