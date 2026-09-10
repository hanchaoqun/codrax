package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1649ActualCall(t *testing.T) (*types.BusContext, types.EvidenceItem) {
	t.Helper()
	bus := b1649ActualCalls(t, 1)
	return bus, bus.EvidenceItems[0]
}

func b1649ActualCalls(t *testing.T, count int) *types.BusContext {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "calls.go"), []byte("package p\nfunc Callee() {}\nfunc Caller() {\n"+strings.Repeat(" Callee()\n", count)+"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("B1649 actual call")}
	read, err := (&ReadFile{}).Execute(bus, json.RawMessage(`{"path":"calls.go","offset":0,"limit":20}`))
	if err != nil || !read.Success {
		t.Fatalf("read: %v %+v", err, read)
	}
	bus.ToolResults = []types.ToolResult{read}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: bus.ToolResults})
	var items []json.RawMessage
	for i := 0; i < count; i++ {
		items = append(items, json.RawMessage(fmt.Sprintf(`{"scope":"line","evidence_kind":"mechanism","subject":"p.Caller","predicate":"calls","object":"Callee","source":"calls.go","line_start":%d,"anchor_kind":"call","anchor_symbol":"Callee"}`, 4+i)))
	}
	params, _ := json.Marshal(map[string]any{"items": items})
	emit, err := (&EmitEvidence{}).Execute(bus, params)
	if err != nil || !emit.Success {
		t.Fatalf("emit evidence: %v %+v", err, emit)
	}
	for _, ev := range bus.Mutable.EmittedEvidence() {
		if ev.Producer == EmitEvidenceProducer && types.ClaimFormOf(ev) == types.ClaimCallEdge && ev.Subject == "p.Caller" {
			if !ev.IsCitable() || ev.GroundingStatus != types.GroundingGrounded || ev.Source != "calls.go" || ev.LineStart < 4 || ev.LineStart >= 4+count || ev.LineEnd != ev.LineStart || !strings.Contains(ev.Snippet, "Callee()") {
				t.Fatalf("actual grounded receipt lost: %+v", ev)
			}
			bus.EvidenceItems = append(bus.EvidenceItems, ev)
		}
	}
	if len(bus.EvidenceItems) != count {
		t.Fatalf("qualified calls missing from actual producer: %+v", bus.Mutable.EmittedEvidence())
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall}}
	return bus
}

func b1649Diagram(from, to string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "The caller invokes the callee."},
		{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n    participant A as " + from + "\n    participant B as " + to + "\n    A->>B: invoke callee\n"}},
	}}
}

func TestB1649ActualGroundedShortCallPublishesAttach(t *testing.T) {
	for _, label := range []string{"Caller", "p.Caller"} {
		t.Run(label, func(t *testing.T) { b1649AssertActualAttach(t, label) })
	}
}

func b1649AssertActualAttach(t *testing.T, label string) {
	t.Helper()
	bus, ev := b1649ActualCall(t)
	doc := b1649Diagram(label, "Callee")
	existing := b1649Diagram("p.Caller", "Callee").Blocks[1]
	existing.ID = "existing-diagram"
	existing.EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", VisibleLabel: "invoke callee", FromIdentity: "p.Caller", ToIdentity: "Callee", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge}}
	doc.Blocks = append(doc.Blocks, existing)
	issues := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, bus.EvidenceItems)
	if len(issues) != 1 || issues[0].Issue != diagramCallEdgeIssueMissingGroundedAnchor {
		t.Fatalf("premise: existing gate must already recognize grounded short call: %+v", issues)
	}
	raw, _ := json.Marshal(doc)
	result, err := (&EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success || result.Repair == nil {
		t.Fatalf("expected structural repair, not a fabricated accepted answer: err=%v result=%+v", err, result)
	}
	var delta diagramRelationRepairDelta
	if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatalf("actual emitted delta: %v %+v", err, result.Repair)
	}
	if len(delta.Failures) != 1 || len(delta.AllowedAdditions) != 1 {
		t.Fatalf("gate-confirmed call must have an executable source-backed choice, not remove-only: failures=%+v additions=%+v", delta.Failures, delta.AllowedAdditions)
	}
	addition := delta.AllowedAdditions[0]
	if addition.EvidenceID != ev.ID || addition.Source != "calls.go:4" || addition.FromIdentity != "p.Caller" || addition.ToIdentity != "Callee" {
		t.Fatalf("repair borrowed/lost exact source: %+v", addition)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(doc, delta.Failures, delta.AllowedAdditions)
	if lease == nil || len(lease.Failures) != 1 || !lease.Failures[0].AllowsAction("attach") {
		t.Fatalf("source-backed addition must execute against this exact visible occurrence: %+v", lease)
	}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	schema := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: bus.Mutable})
	if !strings.Contains(string(schema), "attach") || !strings.Contains(string(schema), lease.Failures[0].FailureRef) {
		t.Fatalf("actual schema did not publish the executable attach choice: %s", schema)
	}
	params := json.RawMessage(fmt.Sprintf(`{"unchanged_block_ids":["summary","existing-diagram"],"diagram_edge_edits":[{"action":"attach","failure_ref":%q,"addition_ref":%q,"edge":{"from_node":"A","to_node":"B","visible_label":"invoke callee"}}]}`, lease.Failures[0].FailureRef, lease.AllowedAdditions[0].AdditionRef))
	patched, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || !patched.Success {
		t.Fatalf("published attach cannot execute: %v %+v", err, patched)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) < 3 || got.Blocks[1].Diagram.Body != doc.Blocks[1].Diagram.Body || !reflect.DeepEqual(got.Blocks[0], doc.Blocks[0]) || !reflect.DeepEqual(got.Blocks[2], existing) {
		t.Fatalf("attach rewrote model-owned body/summary: %+v", got)
	}
	for _, block := range got.Blocks[3:] {
		if block.SystemGeneratedKind == "" {
			t.Fatalf("only the existing source supplement may follow preserved model blocks: %+v", block)
		}
	}
	if issues := DiagramCallEdgeEvidenceMismatches(got, &types.AnswerSemanticView{Family: types.QFCallChain}, bus.EvidenceItems); len(issues) != 0 {
		t.Fatalf("ordinary gate rejected chosen exact repair: %+v", issues)
	}
	after, _ := json.Marshal(doc)
	if string(after) != string(raw) {
		t.Fatal("repair changed immutable input")
	}
}

func TestB1649ShortRepairDoesNotChooseAmongOwners(t *testing.T) {
	bus, base := b1649ActualCall(t)
	other := base
	other.ID, other.Subject, other.Source = "other-owner", "other.Caller", "other/calls.go"
	for _, pool := range [][]types.EvidenceItem{{base, other}, {other, base}} {
		issues := DiagramCallEdgeEvidenceMismatches(b1649Diagram("Caller", "Callee"), &types.AnswerSemanticView{Family: types.QFCallChain}, pool)
		if len(issues) != 1 || issues[0].Issue != diagramCallEdgeIssueMissingAnchor {
			t.Fatalf("full-pool ambiguity must retain original ungrounded diagnosis: %+v", issues)
		}
		if got := preEmitStandaloneRelationRepairCandidates(issues, pool, 1); len(got) != 0 {
			t.Fatalf("a candidate-sized singleton pool must not invent uniqueness: %+v", got)
		}
		doc := b1649Diagram("p.Caller", "Callee")
		issues = DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, pool)
		var delta diagramRelationRepairDelta
		if err := json.Unmarshal([]byte(diagramRelationRepairDeltaJSON(doc, bus, issues, pool, nil)), &delta); err != nil {
			t.Fatal(err)
		}
		if len(delta.AllowedAdditions) != 1 || delta.AllowedAdditions[0].EvidenceID != base.ID {
			t.Fatalf("qualified selection must retain only its original row: %+v", delta)
		}
	}
}

func TestB1649MatcherReceiptKeepsDirectionKindAndExactSource(t *testing.T) {
	_, ev := b1649ActualCall(t)
	issues := DiagramCallEdgeEvidenceMismatches(b1649Diagram("Caller", "Callee"), &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{ev})
	if len(issues) != 1 {
		t.Fatal(issues)
	}
	original := preEmitStandaloneRelationCandidatesFromEvidence(ev)[0]
	if !issues[0].matchesCallRepairCandidate(original) {
		t.Fatal("actual source must match its own receipt")
	}
	for _, tc := range []struct {
		name string
		edit func(*preEmitStandaloneRelationRepairCandidate)
	}{
		{"different-id", func(c *preEmitStandaloneRelationRepairCandidate) { c.evidenceID += "-other" }},
		{"different-file", func(c *preEmitStandaloneRelationRepairCandidate) { c.source = "other/calls.go:4" }},
		{"different-line", func(c *preEmitStandaloneRelationRepairCandidate) { c.source = "calls.go:5" }},
		{"different-range", func(c *preEmitStandaloneRelationRepairCandidate) { c.source = "calls.go:4-5" }},
		{"different-owner", func(c *preEmitStandaloneRelationRepairCandidate) { c.from = "other.Caller" }},
		{"reverse", func(c *preEmitStandaloneRelationRepairCandidate) { c.from, c.to = c.to, c.from }},
		{"different-kind", func(c *preEmitStandaloneRelationRepairCandidate) { c.relation = types.DiagramRelCallback }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := original
			tc.edit(&candidate)
			if issues[0].matchesCallRepairCandidate(candidate) {
				t.Fatalf("receipt borrowed another row or relation: %+v", candidate)
			}
		})
	}
	for _, issue := range []string{diagramCallEdgeIssueMissingAnchor, diagramCallEdgeIssueMissingRelationAnchor, diagramCallEdgeIssueNoEvidence} {
		mismatch := issues[0]
		mismatch.Issue = issue
		if mismatch.matchesCallRepairCandidate(original) {
			t.Fatalf("call receipt escaped its original diagnosis: %s", issue)
		}
		if _, _, ok := mismatch.uniqueCallRepairPair(); ok {
			t.Fatalf("non-grounded failure identity was rewritten: %s", issue)
		}
	}
}

func TestB1649RepairPairUniquenessPrecedesCandidateBudget(t *testing.T) {
	bus, ev := b1649ActualCall(t)
	// Synthetic ambiguity controls: the old exact AnchorSymbol lane admits
	// both rows, but their complete callee identities disagree.
	ev.Subject, ev.Object = "Caller", "one.Callee"
	other := ev
	other.ID, other.Object, other.LineStart, other.LineEnd = "second-call", "two.Callee", 5, 5
	for _, pool := range [][]types.EvidenceItem{{ev, other}, {other, ev}} {
		doc := b1649Diagram("Caller", "Callee")
		issues := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, pool)
		if len(issues) != 1 || issues[0].Issue != diagramCallEdgeIssueMissingGroundedAnchor {
			t.Fatalf("old exact-anchor gate must be unchanged: %+v", issues)
		}
		if got := preEmitStandaloneRelationRepairCandidates(issues, pool, 1); len(got) != 1 {
			t.Fatalf("budget still displays one choice: %+v", got)
		}
		if _, _, ok := issues[0].uniqueCallRepairPair(); ok {
			t.Fatal("budget must not manufacture a unique complete identity pair")
		}
		var delta diagramRelationRepairDelta
		if err := json.Unmarshal([]byte(diagramRelationRepairDeltaJSON(doc, bus, issues, pool, nil)), &delta); err != nil {
			t.Fatal(err)
		}
		if delta.Failures[0].ToIdentity != "Callee" {
			t.Fatalf("ambiguous failure chosen automatically: %+v", delta.Failures)
		}
		lease := types.NewAnswerDiagramRelationRepairLease(doc, delta.Failures, delta.AllowedAdditions)
		if lease == nil || lease.Failures[0].AllowsAction("attach") {
			t.Fatalf("ambiguous pair gained implicit attach ownership: %+v", lease)
		}
	}
	// A matched AnchorSymbol must not donate its identity to an unrelated
	// Object. Nor may filtering that row make the other row uniquely proved.
	other.Object = "two.Unrelated"
	for _, pool := range [][]types.EvidenceItem{{ev, other}, {other, ev}} {
		receipts := diagramCallRepairEvidenceForMismatch(diagramCallEdgeIssueMissingGroundedAnchor, pool, "Caller", "Callee")
		m := DiagramCallEdgeEvidenceMismatch{Issue: diagramCallEdgeIssueMissingGroundedAnchor, matchedCallEvidence: receipts}
		if _, _, ok := m.uniqueCallRepairPair(); ok {
			t.Fatal("discarding a contradictory matching row fabricated uniqueness")
		}
		candidate := preEmitStandaloneRelationCandidatesFromEvidence(other)[0]
		if m.matchesCallRepairCandidate(candidate) {
			t.Fatal("AnchorSymbol was used to nominate an unrelated Object")
		}
	}
}

// Freeze the old two-lane boolean as a parity oracle, not a new producer.
func b1649OriginalRowBackedCallGate(pool []types.EvidenceItem, from, to string) bool {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" {
		return false
	}
	for _, ev := range pool {
		if ev.IsCitable() && types.ClaimFormOf(ev) == types.ClaimCallEdge &&
			diagramCallEvidenceEndpointMatches(ev, ev.Subject, from) &&
			(diagramCallEvidenceEndpointMatches(ev, ev.Object, to) || diagramCallEvidenceEndpointMatches(ev, ev.AnchorSymbol, to)) {
			return true
		}
	}
	return diagramRelationEdgeHasExactOrUniqueShortProjection(pool, from, to,
		func(ev types.EvidenceItem) bool { return types.ClaimFormOf(ev) == types.ClaimCallEdge },
		func(ev types.EvidenceItem) []string { return []string{ev.Subject} },
		func(ev types.EvidenceItem) []string { return []string{ev.Object, ev.AnchorSymbol} })
}

func TestB1649SharedMatcherPreservesBooleanAndExactEarlyReturn(t *testing.T) {
	_, base := b1649ActualCall(t)
	other := base
	other.ID, other.Subject = "other-owner", "other.Caller"
	invalid := base
	invalid.GroundingStatus = types.GroundingUngrounded
	callback := base
	callback.Predicate, callback.AnchorKind = "registers", types.AnchorDefinition
	pools := [][]types.EvidenceItem{nil, {base}, {base, other}, {other, base}, {invalid}, {callback}}
	for _, pool := range pools {
		for _, from := range []string{"", "Caller", "p.Caller", "other.Caller", "Callee", "Unknown"} {
			for _, to := range []string{"", "Callee", "p.Callee", "Caller", "Unknown"} {
				want := b1649OriginalRowBackedCallGate(pool, from, to)
				got, noRows := diagramCallEdgeExactOrUniqueShortEvidence(pool, from, to, false)
				collected, rows := diagramCallEdgeExactOrUniqueShortEvidence(pool, from, to, true)
				if got != want || collected != want || (len(rows) > 0) != want || len(noRows) != 0 {
					t.Fatalf("boolean gate changed for %q -> %q: want=%v got=%v collected=%v rows=%d", from, to, want, got, collected, len(rows))
				}
			}
		}
	}
	pool := []types.EvidenceItem{base, other}
	before := testing.AllocsPerRun(100, func() { b1649OriginalRowBackedCallGate(pool, "p.Caller", "Callee") })
	after := testing.AllocsPerRun(100, func() { diagramCallEdgeHasTypedEvidence(pool, nil, "p.Caller", "Callee", "") })
	if after > before {
		t.Fatalf("normal exact gate added allocations: old=%v new=%v", before, after)
	}
}

func TestB1649ActualDistinctCallSitesKeepOriginalOccurrenceBudget(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprintf("source_sites_%d", count), func(t *testing.T) {
			bus := b1649ActualCalls(t, count)
			doc := b1649Diagram("Caller", "Callee")
			doc.Blocks[1].Diagram.Body += "    A->>B: invoke again\n"
			issues := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, bus.EvidenceItems)
			if len(issues) != 2 {
				t.Fatalf("need two actual visible missing anchors: %+v", issues)
			}
			var delta diagramRelationRepairDelta
			if err := json.Unmarshal([]byte(diagramRelationRepairDeltaJSON(doc, bus, issues, bus.EvidenceItems, nil)), &delta); err != nil {
				t.Fatal(err)
			}
			if len(delta.AllowedAdditions) != count {
				t.Fatalf("distinct native call-site rows lost or duplicated: %+v", delta.AllowedAdditions)
			}
			for _, candidate := range delta.AllowedAdditions {
				found := false
				for _, ev := range bus.EvidenceItems {
					found = found || (candidate.EvidenceID == ev.ID && candidate.Source == currentSourceEvidenceLocation(ev))
				}
				if !found {
					t.Fatalf("candidate lost exact call-site provenance: %+v", candidate)
				}
			}
			lease := types.NewAnswerDiagramRelationRepairLease(doc, delta.Failures, delta.AllowedAdditions)
			if lease == nil || len(lease.Failures) != 2 {
				t.Fatalf("lost visible occurrence locators: %+v", lease)
			}
			bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
			bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
			var edits []map[string]any
			for i, f := range lease.Failures {
				if !f.AllowsAction("attach") || f.BodyOccurrence != i+1 {
					t.Fatalf("candidate erased exact occurrence: %+v", f)
				}
				label := "invoke callee"
				if i == 1 {
					label = "invoke again"
				}
				edits = append(edits, map[string]any{"action": "attach", "failure_ref": f.FailureRef,
					"addition_ref": lease.AllowedAdditions[i%count].AdditionRef,
					"edge":         map[string]string{"from_node": "A", "to_node": "B", "visible_label": label}})
			}
			if count == 1 {
				params, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits})
				result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
				if err != nil || result.Success || !strings.Contains(result.Summary, "each live allowed addition may be selected at most once") {
					t.Fatalf("one source choice was reused for two messages: err=%v result=%+v", err, result)
				}
			} else {
				// One exact pair anchor owns the pair; the FULL evidence pool
				// still has to supply both static call-site occurrences. A second
				// redundant attach is not needed to express this same-method case.
				params, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits[:1]})
				result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
				if err != nil || !result.Success {
					t.Fatalf("two-site pair must be repairable with one exact pair anchor: err=%v result=%+v", err, result)
				}
				got := bus.Mutable.AnswerDocumentV2()
				if got.Blocks[1].Diagram.Body != doc.Blocks[1].Diagram.Body || len(got.Blocks[1].EdgeAnchors) != 1 {
					t.Fatalf("single selected attach lost either model message: %+v", got.Blocks[1])
				}
			}
			// Independently test the original ordinary gate, without treating
			// the existing pair-level multi-attach executor as a new capability.
			// These are explicit model-owned anchors, not auto-added production.
			var authored types.AnswerDocumentV2
			snapshot, _ := json.Marshal(doc)
			if err := json.Unmarshal(snapshot, &authored); err != nil {
				t.Fatal(err)
			}
			for _, label := range []string{"invoke callee", "invoke again"} {
				authored.Blocks[1].EdgeAnchors = append(authored.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{
					FromNode: "A", ToNode: "B", FromIdentity: "p.Caller", ToIdentity: "Callee", VisibleLabel: label,
					RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge})
			}
			issues = DiagramCallEdgeEvidenceMismatches(&authored, &types.AnswerSemanticView{Family: types.QFCallChain}, bus.EvidenceItems)
			if count == 1 && (len(issues) != 1 || issues[0].Issue != diagramCallEdgeIssueOccurrenceUnproven) {
				t.Fatalf("one source site must not authorize two calls: %+v", issues)
			}
			if count == 2 && len(issues) != 0 {
				t.Fatalf("two actual source sites must retain ordinary authority: %+v", issues)
			}
		})
	}
}

func TestB1649UnprovenOrMissingSourceCannotMintReceipt(t *testing.T) {
	_, base := b1649ActualCall(t)
	for _, tc := range []struct {
		name string
		edit func(*types.EvidenceItem)
	}{
		{"ungrounded", func(e *types.EvidenceItem) { e.GroundingStatus = types.GroundingUngrounded }},
		{"no-id", func(e *types.EvidenceItem) { e.ID = "" }},
		{"no-file", func(e *types.EvidenceItem) { e.Source = "" }},
		{"no-line", func(e *types.EvidenceItem) { e.LineStart, e.LineEnd = 0, 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := base
			tc.edit(&ev)
			m := DiagramCallEdgeEvidenceMismatch{Issue: diagramCallEdgeIssueMissingGroundedAnchor,
				matchedCallEvidence: diagramCallRepairEvidenceForMismatch(diagramCallEdgeIssueMissingGroundedAnchor, []types.EvidenceItem{ev}, "Caller", "Callee")}
			if _, _, ok := m.uniqueCallRepairPair(); ok {
				t.Fatal("unproved or incomplete source row minted exact repair ownership")
			}
			for _, candidate := range preEmitStandaloneRelationCandidatesFromEvidence(ev) {
				if m.matchesCallRepairCandidate(candidate) {
					t.Fatal("missing precise source still selected a repair")
				}
			}
		})
	}
}
