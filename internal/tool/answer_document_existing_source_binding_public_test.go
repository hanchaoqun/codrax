package tool_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

type existingSourceBindingFixture struct {
	file, source, language, caller, callee, owner, displayedCallee string
	lines                                                          []int
}

func existingSourceBindingPublicFixture(t *testing.T, options ...existingSourceBindingFixture) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	cfg := existingSourceBindingFixture{"gate.go", "package gate\nfunc Run() {\n RunWith()\n}\nfunc RunWith() {}\n", rmtypes.LangGo, "Run", "RunWith", "gate.Run", "gate.RunWith", []int{3}}
	if len(options) > 0 {
		cfg = options[0]
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("show the selected call"), PresentationDiagramRequired: true}
	if err := os.WriteFile(filepath.Join(repo, cfg.file), []byte(cfg.source), 0600); err != nil {
		t.Fatal(err)
	}
	files := index.ParseFiles([]index.FileEntry{{RelPath: cfg.file, AbsPath: filepath.Join(bus.RepoRoot, cfg.file), Language: cfg.language}}, bus.RepoRoot)
	bus.Mutable.SetSearchGraph(index.BuildGraph(bus.RepoRoot, files))
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(fmt.Sprintf(`{"path":%q,"offset":0,"limit":30}`, cfg.file)))
	if err != nil || !read.Success {
		t.Fatalf("actual read: %v %+v", err, read)
	}
	bus.Mutable.AppendDispatchToolResult(read)
	bus.ToolResults = []types.ToolResult{read}
	var items []map[string]any
	for _, line := range cfg.lines {
		items = append(items, map[string]any{"scope": "line", "evidence_kind": "mechanism", "subject": cfg.caller, "predicate": "calls", "object": cfg.callee, "source": cfg.file, "line_start": line, "anchor_kind": "call", "anchor_symbol": cfg.callee})
	}
	params, _ := json.Marshal(map[string]any{"items": items})
	emit, err := (&tool.EmitEvidence{}).Execute(bus, params)
	if err != nil || !emit.Success {
		t.Fatalf("actual evidence: %v %+v", err, emit)
	}
	for _, ev := range bus.Mutable.EmittedEvidence() {
		if types.ClaimFormOf(ev) == types.ClaimCallEdge && ev.Producer == tool.EmitEvidenceProducer {
			if !ev.IsCitable() || ev.OwnerSymbol != cfg.owner || ev.Source != cfg.file {
				t.Fatalf("source-owned receipt missing: %+v", ev)
			}
			bus.EvidenceItems = append(bus.EvidenceItems, ev)
		}
	}
	if len(bus.EvidenceItems) != len(cfg.lines) {
		t.Fatalf("actual call was not grounded: %+v", bus.Mutable.EmittedEvidence())
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true,
			Participants: []types.DiagramParticipantHint{{Identity: cfg.owner, Role: types.DiagramParticipantIncidentRequired, SourceQuote: cfg.owner}}}}}
	bus.AnalysisIR.AnswerContract.Diagram = &types.DiagramContract{Required: true, PreferredKinds: []types.DiagramKind{types.DiagramSequence}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "The caller invokes the callee."},
		{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: fmt.Sprintf("sequenceDiagram\n participant A as %q\n participant B as %q\n A->>B: invoke callee", cfg.owner, cfg.displayedCallee)}},
	}}
	doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge, VisibleLabel: "invoke callee"}}
	return bus, doc
}

func TestExistingSourceBindingPublicNodeOnlyAnchorDoesNotRequestDuplicate(t *testing.T) {
	bus, doc := existingSourceBindingPublicFixture(t)
	existingSourceBindingAssertAccepted(t, bus, doc)
}

func existingSourceBindingAssertAccepted(t *testing.T, bus *types.BusContext, doc *types.AnswerDocumentV2) {
	t.Helper()
	body, anchors := doc.Blocks[1].Diagram.Body, append([]types.DiagramEdgeAnchor(nil), doc.Blocks[1].EdgeAnchors...)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if len(view.DiagramParticipantObligations) != 1 {
		t.Fatalf("missing typed participant obligation: %+v", view)
	}
	if issues := tool.DiagramCallEdgeEvidenceMismatches(doc, types.BuildAnswerSemanticViewForBusContext(bus), bus.EvidenceItems); len(issues) != 0 {
		t.Fatalf("ordinary source gate must already prove the existing message: %+v; evidence=%+v", issues, bus.EvidenceItems)
	}
	raw, _ := json.Marshal(doc)
	evidenceBefore, _ := json.Marshal(bus.Mutable.EmittedEvidence())
	rawBefore := string(raw)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("source-proved existing message must satisfy the request without an addition: %+v", result)
	}
	evidenceAfter, _ := json.Marshal(bus.Mutable.EmittedEvidence())
	if string(evidenceBefore) != string(evidenceAfter) || string(raw) != rawBefore {
		t.Fatal("validator mutated evidence or model input")
	}
	got := bus.Mutable.AnswerDocumentV2().Blocks[1]
	if got.Diagram.Body != body || !reflect.DeepEqual(got.EdgeAnchors, anchors) {
		t.Fatalf("binding changed model-owned body or anchors: body=%q want=%q anchors=%+v", got.Diagram.Body, body, got.EdgeAnchors)
	}
}

func TestExistingSourceBindingPublicPreservesRepeatedMessagesBranchesAndReturn(t *testing.T) {
	bus, doc := existingSourceBindingPublicFixture(t)
	doc.Blocks[1].Diagram.Body += "\n B-->>A: result\n alt another invocation\n A->>B: invoke callee\n B-->>A: result\n else no invocation\n Note over A: retain prior result\n end"
	existingSourceBindingAssertAccepted(t, bus, doc)
	if strings.Count(bus.Mutable.AnswerDocumentV2().Blocks[1].Diagram.Body, "A->>B: invoke callee") != 2 {
		t.Fatal("legitimate repeat was removed or duplicated")
	}
}

func TestExistingSourceBindingPublicRenamedAndOtherLanguage(t *testing.T) {
	for _, cfg := range []existingSourceBindingFixture{
		{"dispatch.go", "package dispatch\nfunc Submit() {\n Deliver()\n}\nfunc Deliver() {}\n", rmtypes.LangGo, "Submit", "Deliver", "dispatch.Submit", "dispatch.Deliver", []int{3}},
		{"Gateway.java", "class Gateway {\n void run() {\n  deliver();\n }\n void deliver() {}\n}\n", rmtypes.LangJava, "run", "deliver", "Gateway.run", "deliver", []int{3}},
	} {
		t.Run(cfg.language, func(t *testing.T) {
			bus, doc := existingSourceBindingPublicFixture(t, cfg)
			existingSourceBindingAssertAccepted(t, bus, doc)
		})
	}
}

func TestExistingSourceBindingPublicDoesNotBorrowUnselectedSource(t *testing.T) {
	bus, doc := existingSourceBindingPublicFixture(t)
	evidence := append([]types.EvidenceItem(nil), bus.EvidenceItems...)
	evidence[0].Producer = "repomap.background"
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if issues := tool.DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(issues) != 0 {
		t.Fatalf("background call still truthful: %+v", issues)
	}
	if issues := tool.DiagramParticipantCoverageMismatches(doc, view, bus.AnalysisIR.RequestModel, evidence); len(issues) == 0 {
		t.Fatal("background-only relation entered requested participant graph")
	}
}

func TestExistingSourceBindingPublicRejectsAmbiguityAndExplicitConflict(t *testing.T) {
	for _, mode := range []string{"multiple_call_sites", "wrong_owner", "different_source_same_short_pair", "explicit_conflict", "reverse"} {
		t.Run(mode, func(t *testing.T) {
			var bus *types.BusContext
			var doc *types.AnswerDocumentV2
			if mode == "multiple_call_sites" {
				bus, doc = existingSourceBindingPublicFixture(t, existingSourceBindingFixture{"gate.go", "package gate\nfunc Run() {\n RunWith()\n RunWith()\n}\nfunc RunWith() {}\n", rmtypes.LangGo, "Run", "RunWith", "gate.Run", "gate.RunWith", []int{3, 4}})
			} else {
				bus, doc = existingSourceBindingPublicFixture(t)
			}
			switch mode {
			case "wrong_owner":
				bus.EvidenceItems[0].OwnerSymbol = "other.Run"
			case "different_source_same_short_pair":
				other := bus.EvidenceItems[0]
				other.Source = "other.go"
				other.ID = "other-source"
				bus.EvidenceItems = append(bus.EvidenceItems, other)
			case "explicit_conflict":
				doc.Blocks[1].EdgeAnchors[0].FromIdentity = "other.Run"
				doc.Blocks[1].EdgeAnchors[0].ToIdentity = "other.RunWith"
			case "reverse":
				doc.Blocks[1].Diagram.Body = strings.ReplaceAll(doc.Blocks[1].Diagram.Body, "A->>B", "B->>A")
				doc.Blocks[1].EdgeAnchors[0].FromNode = "B"
				doc.Blocks[1].EdgeAnchors[0].ToNode = "A"
			}
			view := types.BuildAnswerSemanticViewForBusContext(bus)
			if issues := tool.DiagramCallEdgeEvidenceMismatches(doc, view, bus.EvidenceItems); len(issues) == 0 {
				t.Fatal("ambiguous/conflicting source call became proved")
			}
			if issues := tool.DiagramParticipantCoverageMismatches(doc, view, bus.AnalysisIR.RequestModel, bus.EvidenceItems); len(issues) == 0 {
				t.Fatal("ambiguous/conflicting existing message satisfied participant debt")
			}
		})
	}
}

func existingSourceBindingDefinitionFixture(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	bus, doc := existingSourceBindingPublicFixture(t, existingSourceBindingFixture{"gate.go", "package gate\nfunc Run() {\n RunWith()\n}\nfunc RunWith() {\n Sink()\n}\nfunc Sink() {}\n", rmtypes.LangGo, "Run", "RunWith", "gate.Run", "gate.RunWith", []int{3}})
	emit, err := (&tool.EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"direct","subject":"gate","predicate":"defines","object":"Run","source":"gate.go","line_start":2,"anchor_kind":"definition","anchor_symbol":"Run"},{"scope":"line","evidence_kind":"mechanism","subject":"gate.RunWith","predicate":"calls","object":"Sink","source":"gate.go","line_start":6,"anchor_kind":"call","anchor_symbol":"Sink"}]}`))
	if err != nil || !emit.Success {
		t.Fatalf("actual definition/callee support: %v %+v", err, emit)
	}
	if err := os.WriteFile(filepath.Join(bus.RepoRoot, "client.go"), []byte("package client\nimport \"example/gate\"\nfunc Start() {\n gate.RunWith()\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"client.go","offset":0,"limit":30}`))
	if err != nil || !read.Success {
		t.Fatalf("client read: %v %+v", err, read)
	}
	bus.Mutable.AppendDispatchToolResult(read)
	bus.ToolResults = append(bus.ToolResults, read)
	emit, err = (&tool.EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"mechanism","subject":"Start","predicate":"calls","object":"gate.RunWith","source":"client.go","line_start":4,"anchor_kind":"call","anchor_symbol":"gate.RunWith"}]}`))
	if err != nil || !emit.Success {
		t.Fatalf("qualified callee receipt: %v %+v", err, emit)
	}
	legacy := append([]types.EvidenceItem(nil), bus.Mutable.EmittedEvidence()...)
	for i := range legacy {
		if legacy[i].Subject == "Run" {
			legacy[i].OwnerSymbol = ""
			legacy[i].OwnerIdentity = ""
		}
	}
	// Replay a legacy serialization without the later parser owner stamp;
	// every relation and definition still came from actual public grounding.
	mu := types.NewMutableState("legacy source receipt replay")
	mu.AppendEvidence(legacy)
	bus = &types.BusContext{RepoRoot: bus.RepoRoot, WorkDir: bus.WorkDir, Mutable: mu, EvidenceItems: legacy, AnalysisIR: bus.AnalysisIR, ToolResults: bus.ToolResults, PresentationDiagramRequired: true}
	bus.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"gate.Run"}
	bus.AnalysisIR.AnswerContract.MustIncludeTerms = []types.ContractTerm{{Text: "gate.Run", Kind: types.ContractTermSymbol}}
	return bus, doc
}

func TestExistingSourceBindingPublicDefinitionBackedLegacyReceipt(t *testing.T) {
	bus, doc := existingSourceBindingDefinitionFixture(t)
	existingSourceBindingAssertAccepted(t, bus, doc)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if len(view.RequiredMechanismAnchors) != 1 {
		t.Fatal("legacy test did not exercise required-anchor proof")
	}
	for _, mode := range []string{"missing_definition", "different_source_definition", "ambiguous_definition", "nonselected_call"} {
		t.Run(mode, func(t *testing.T) {
			evidence := append([]types.EvidenceItem(nil), bus.EvidenceItems...)
			for i := range evidence {
				if types.ClaimFormOf(evidence[i]) == types.ClaimDefinitionFact {
					switch mode {
					case "missing_definition":
						evidence[i].GroundingStatus = types.GroundingUngrounded
					case "different_source_definition":
						evidence[i].Source = "other.go"
					case "ambiguous_definition":
						other := evidence[i]
						other.ID = "other-definition"
						other.Source = "other.go"
						evidence = append(evidence, other)
					}
				}
				if mode == "nonselected_call" && evidence[i].Subject == "Run" {
					evidence[i].Producer = "repomap.background"
				}
			}
			if issues := tool.DiagramParticipantCoverageMismatches(doc, view, bus.AnalysisIR.RequestModel, evidence); len(issues) == 0 {
				t.Fatal("legacy binding borrowed missing/ambiguous/out-of-scope authority")
			}
		})
	}
}

func TestExistingSourceBindingPublicRepairPublishesOnlyMissingJoin(t *testing.T) {
	bus, doc := existingSourceBindingDefinitionFixture(t)
	for _, identity := range []string{"gate.RunWith", "Sink"} {
		bus.AnalysisIR.RequestModel.DiagramHint.Participants = append(bus.AnalysisIR.RequestModel.DiagramHint.Participants, types.DiagramParticipantHint{Identity: identity, Role: types.DiagramParticipantIncidentRequired, SourceQuote: identity})
	}
	doc.Blocks[1].Diagram.Body = strings.Replace(doc.Blocks[1].Diagram.Body, " A->>B:", " participant C as \"Sink\"\n A->>B:", 1)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if issues := tool.DiagramCallEdgeEvidenceMismatches(doc, view, bus.EvidenceItems); len(issues) != 0 {
		t.Fatalf("existing message premise: %+v", issues)
	}
	raw, _ := json.Marshal(doc)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success || result.Repair == nil {
		t.Fatalf("missing requested join must require repair: %v %+v", err, result)
	}
	var delta struct {
		AllowedAdditions []types.AnswerDiagramRelationRepairCandidate `json:"allowed_additions"`
	}
	if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatalf("executable repair absent: %v %+v", err, result.Repair)
	}
	if len(delta.AllowedAdditions) == 0 {
		t.Fatalf("no executable missing join: %+v", result.Repair)
	}
	for _, candidate := range delta.AllowedAdditions {
		if candidate.FromIdentity != "RunWith" || candidate.ToIdentity != "Sink" {
			t.Fatalf("repair reintroduced an already rendered source relation: %+v", candidate)
		}
	}
	for _, key := range []string{types.ToolRepairMetaDiagramParticipantRepairDeltaJSON} {
		if guidance := result.Repair.Metadata[key]; strings.Contains(guidance, `from_identity:\"Run\"`) {
			t.Fatalf("compact guidance disagrees with existing-source binding: %s", guidance)
		}
	}
}

func TestExistingSourceBindingPublicPartialIdentityRequiresMetadataRepair(t *testing.T) {
	for _, side := range []string{"from", "to"} {
		for _, correct := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/correct=%t", side, correct), func(t *testing.T) {
				bus, doc := existingSourceBindingPublicFixture(t)
				prefix := "other."
				if correct {
					prefix = "gate."
				}
				if side == "from" {
					doc.Blocks[1].EdgeAnchors[0].FromIdentity = prefix + "Run"
				} else {
					doc.Blocks[1].EdgeAnchors[0].ToIdentity = prefix + "RunWith"
				}
				raw, _ := json.Marshal(doc)
				result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
				if err != nil || result.Success {
					t.Fatalf("partial identity must stay model-owned and repairable, never ignored: %v %+v", err, result)
				}
				if bus.Mutable.AnswerDocumentV2() != nil {
					t.Fatal("rejected partial metadata replaced accepted state")
				}
			})
		}
	}
}

func TestExistingSourceBindingPublicSameActorsKeepDistinctOperations(t *testing.T) {
	bus, doc := existingSourceBindingPublicFixture(t, existingSourceBindingFixture{"gate.go", "package gate\nfunc Run() {\n RunWith()\n Other()\n}\nfunc RunWith() {}\nfunc Other() {}\n", rmtypes.LangGo, "Run", "RunWith", "gate.Run", "gate.RunWith", []int{3}})
	emit, err := (&tool.EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"mechanism","subject":"Run","predicate":"calls","object":"Other","source":"gate.go","line_start":4,"anchor_kind":"call","anchor_symbol":"Other"}]}`))
	if err != nil || !emit.Success {
		t.Fatalf("second actual operation: %v %+v", err, emit)
	}
	bus.EvidenceItems = bus.Mutable.EmittedEvidence()
	doc.Blocks[1].Diagram.Body = "sequenceDiagram\n participant A as \"gate.Run\"\n participant B as \"gate\"\n A->>B: invoke callee\n A->>B: invoke other"
	doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "A", ToNode: "B", FromIdentity: "gate.Run", ToIdentity: "gate.RunWith", VisibleLabel: "invoke callee", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		{FromNode: "A", ToNode: "B", FromIdentity: "gate.Run", ToIdentity: "gate.Other", VisibleLabel: "invoke other", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
	}
	existingSourceBindingAssertAccepted(t, bus, doc)
	doc.Blocks[1].Diagram.Body = strings.TrimSuffix(doc.Blocks[1].Diagram.Body, "\n A->>B: invoke other")
	raw, _ := json.Marshal(doc)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success {
		t.Fatalf("hidden second operation cannot borrow the first occurrence: %v %+v", err, result)
	}
}
