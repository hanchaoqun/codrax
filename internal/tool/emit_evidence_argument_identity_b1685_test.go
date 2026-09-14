package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1685ReadSources(t *testing.T, sources map[string]string) *types.BusContext {
	t.Helper()
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("argument identity")}
	for file, source := range sources {
		if err := os.WriteFile(filepath.Join(repo, file), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		params, _ := json.Marshal(map[string]any{"path": file, "line_offset": 0, "limit": 100})
		read, err := (&ReadFile{}).Execute(bus, params)
		if err != nil || !read.Success || read.ReadCoverage == nil {
			t.Fatalf("actual source read: %v %+v", err, read)
		}
		bus.Mutable.AppendDispatchToolResult(read)
	}
	return bus
}

func b1685ArgumentItem(file string, line int, argument, receiver string) map[string]any {
	return map[string]any{"scope": "line", "evidence_kind": "relationship", "source": file,
		"line_start": line, "anchor_kind": "argument", "anchor_symbol": argument,
		"subject": argument, "predicate": "passes argument", "object": receiver,
		"summary": "The original model-authored handoff description."}
}

func b1685Emit(t *testing.T, bus *types.BusContext, items ...map[string]any) types.ToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), params...)
	result, err := (&EmitEvidence{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("public evidence emit: %v %+v", err, result)
	}
	if !bytes.Equal(before, params) {
		t.Fatal("evidence tool modified the raw model arguments")
	}
	bus.Mutable.AppendDispatchToolResult(result)
	return result
}

func TestB1685ArgumentAliasesCanonicalizeAtActualSourceBeforePublication(t *testing.T) {
	for _, tc := range []struct {
		language, file, source, short, full string
		line                                int
	}{
		{"go", "flow.go", "package p\nfunc Run(ac Context) bool {\n return engine.Check(ac)\n}\n", "Check", "engine.Check", 3},
		{"python", "flow.py", "def run(ac):\n    return engine.check(ac)\n", "check", "engine.check", 2},
		{"java", "Flow.java", "class Flow {\n boolean run(Context ac) {\n  return engine.check(ac);\n }\n}\n", "check", "engine.check", 3},
		{"arkts", "Flow.ets", "function run(ac: Context): boolean {\n return engine.check(ac);\n}\n", "check", "engine.check", 2},
		{"cangjie", "flow.cj", "func run(ac: Context): Bool {\n return engine.check(ac)\n}\n", "check", "engine.check", 2},
		{"cpp", "flow.cc", "bool run(Context ac) {\n return engine.check(ac);\n}\n", "check", "engine.check", 2},
		{"rust", "flow.rs", "fn run(ac: Context) -> bool {\n return engine.check(ac);\n}\n", "check", "engine.check", 2},
		{"swift", "Flow.swift", "func run(ac: Context) -> Bool {\n return engine.check(ac)\n}\n", "check", "engine.check", 2},
	} {
		for _, batch := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/same_batch=%t/reverse=%t", tc.language, batch, reverse), func(t *testing.T) {
					bus := b1685ReadSources(t, map[string]string{tc.file: tc.source})
					canonicalBus := b1685ReadSources(t, map[string]string{tc.file: tc.source})
					b1685Emit(t, canonicalBus, b1685ArgumentItem(tc.file, tc.line, "ac", tc.full))
					canonicalID := canonicalBus.Mutable.EmittedEvidence()[0].ID
					items := []map[string]any{b1685ArgumentItem(tc.file, tc.line, "ac", tc.short), b1685ArgumentItem(tc.file, tc.line, "ac", tc.full)}
					if reverse {
						items[0], items[1] = items[1], items[0]
					}
					if batch {
						b1685Emit(t, bus, items...)
					} else {
						for _, item := range items {
							b1685Emit(t, bus, item)
						}
					}
					rows := bus.Mutable.EmittedEvidence()
					for _, row := range rows {
						if !row.IsCitable() || row.AnchorKind != types.AnchorArgument || row.GroundingStatus != types.GroundingGrounded {
							t.Fatalf("premise: both accepted syntax aliases must already be grounded: %+v", row)
						}
					}
					if len(rows) != 1 {
						t.Fatalf("one actual invocation/argument must not publish duplicate alias facts: %+v", rows)
					}
					row := rows[0]
					if row.Object != tc.full || row.Subject != "ac" || row.AnchorSymbol != "ac" || row.Predicate != "passes argument" ||
						row.Source != tc.file || row.LineStart != tc.line || row.LineEnd != tc.line || row.Summary != items[0]["summary"] ||
						row.GroundingTier != types.TierLineText || row.ID != canonicalID {
						t.Fatalf("normalization changed more than the proved receiving expression: %+v", row)
					}
					// A third emit remains idempotent, including its explanatory note.
					b1685Emit(t, bus, b1685ArgumentItem(tc.file, tc.line, "ac", tc.short))
					if again := bus.Mutable.EmittedEvidence(); len(again) != 1 || again[0].ID != row.ID {
						t.Fatalf("canonical source identity did not survive repeated batches: %+v", again)
					}
				})
			}
		}
	}
}

func TestB1685ArgumentIdentityPreservesIndependentCoordinatesAndValues(t *testing.T) {
	bus := b1685ReadSources(t, map[string]string{
		"a.go": "package p\nfunc Run(ac, other Context) {\n engine.Check(ac, other)\n peer.Check(ac)\n engine.Check(ac)\n}\n",
		"b.go": "package p\nfunc Run(ac Context) {\n engine.Check(ac)\n}\n",
	})
	b1685Emit(t, bus,
		b1685ArgumentItem("a.go", 3, "ac", "Check"),
		b1685ArgumentItem("a.go", 3, "other", "Check"),
		b1685ArgumentItem("a.go", 4, "ac", "Check"),
		b1685ArgumentItem("a.go", 5, "ac", "Check"),
		b1685ArgumentItem("b.go", 3, "ac", "Check"))
	rows := bus.Mutable.EmittedEvidence()
	if len(rows) != 5 {
		t.Fatalf("different sources, lines, arguments and receivers must remain distinct: %+v", rows)
	}
	for _, row := range rows {
		if !row.IsCitable() || !strings.Contains(row.Object, ".Check") {
			t.Errorf("canonical source receiver missing: %+v", row)
		}
	}
}

func TestB1685ArgumentIdentityDoesNotGuessUnprovedTuple(t *testing.T) {
	for _, file := range []string{"flow.go", "flow.ets", "flow.cj"} {
		for _, tc := range []struct{ name, source, subject, object string }{
			{"different_receivers", "engine.Check(ac); peer.Check(ac)", "ac", "Check"},
			{"two_invocations", "engine.Check(ac); engine.Check(other)", "ac", "Check"},
			{"duplicate_argument_slots", "engine.Check(ac, ac)", "ac", "Check"},
			{"partial_argument", "engine.Check(ac.Context)", "ac", "Check"},
			{"wrong_receiver", "engine.Check(ac)", "ac", "peer.Check"},
			{"reverse_direction", "engine.Check(ac)", "Check", "ac"},
			{"quoted_call", `message := "engine.Check(ac)"`, "ac", "Check"},
			{"comment_call", "// engine.Check(ac)", "ac", "Check"},
		} {
			t.Run(file+"/"+tc.name, func(t *testing.T) {
				bus := b1685ReadSources(t, map[string]string{file: "func run() {\n" + tc.source + "\n}\n"})
				b1685Emit(t, bus, b1685ArgumentItem(file, 2, tc.subject, tc.object))
				rows := bus.Mutable.EmittedEvidence()
				if len(rows) != 1 || rows[0].Object != tc.object || rows[0].Subject != tc.subject ||
					strings.Contains(rows[0].GroundingNote, "argument receiver normalized") {
					t.Fatalf("ambiguous/unproved source tuple was rewritten: %+v", rows)
				}
			})
		}
	}
}

func TestB1685ArgumentIdentityRequiresCurrentGroundingAndReadScope(t *testing.T) {
	base := types.EvidenceItem{Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		AnchorKind: types.AnchorArgument, AnchorSymbol: "ac", Subject: "ac", Object: "Check",
		Source: "flow.go", LineStart: 3, LineEnd: 3, GroundingStatus: types.GroundingGrounded}
	gc := &ground.Context{LineIndex: map[string]map[int]string{"flow.go": {3: "engine.Check(ac)"}}}
	for _, tc := range []struct {
		name    string
		change  func(*types.EvidenceItem)
		context *ground.Context
	}{
		{"nil_context", func(*types.EvidenceItem) {}, nil},
		{"no_read", func(*types.EvidenceItem) {}, &ground.Context{}},
		{"other_source", func(i *types.EvidenceItem) { i.Source = "other.go" }, gc},
		{"unread_line", func(i *types.EvidenceItem) { i.LineStart = 4 }, gc},
		{"legacy_no_status", func(i *types.EvidenceItem) { i.GroundingStatus = "" }, gc},
		{"recovered_only", func(i *types.EvidenceItem) { i.GroundingStatus = types.GroundingRecovered }, gc},
		{"ungrounded", func(i *types.EvidenceItem) { i.GroundingStatus = types.GroundingUngrounded }, gc},
		{"line_range", func(i *types.EvidenceItem) { i.Scope = types.ScopeLineRange }, gc},
		{"another_relation", func(i *types.EvidenceItem) { i.AnchorKind = types.AnchorCall }, gc},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := base
			tc.change(&item)
			before, _ := json.Marshal(item)
			if normalizeArgumentFlowEvidenceReceiver(&item, tc.context) {
				t.Fatal("out-of-scope item normalized")
			}
			after, _ := json.Marshal(item)
			if !bytes.Equal(before, after) {
				t.Fatalf("out-of-scope evidence mutated: %s", after)
			}
		})
	}
}

func TestB1685ArgumentIdentityPreservesPredicateAndExactReceiverSeparation(t *testing.T) {
	bus := b1685ReadSources(t, map[string]string{"flow.go": "func run() {\n engine.Check(ac); peer.Check(ac)\n}\n"})
	one := b1685ArgumentItem("flow.go", 2, "ac", "engine.Check")
	two := b1685ArgumentItem("flow.go", 2, "ac", "peer.Check")
	three := b1685ArgumentItem("flow.go", 2, "ac", "engine.Check")
	three["predicate"] = "supplies state"
	b1685Emit(t, bus, one, two, three)
	rows := bus.Mutable.EmittedEvidence()
	if len(rows) != 3 {
		t.Fatalf("distinct receiver or model predicate was merged: %+v", rows)
	}
	ids := map[string]bool{}
	for _, row := range rows {
		if !row.IsCitable() || ids[row.ID] {
			t.Fatalf("distinct proved tuple lost authority or identity: %+v", rows)
		}
		ids[row.ID] = true
	}
}

func TestB1685ArgumentIdentityUsesFinalRecoveredCoordinate(t *testing.T) {
	bus := b1685ReadSources(t, map[string]string{"flow.go": "package p\ntype Context struct{}\ntype Checker interface { Check(Context) }\nfunc Run(ac Context, engine Checker) {\n\n engine.Check(ac)\n}\n"})
	b1685Emit(t, bus, b1685ArgumentItem("flow.go", 5, "ac", "Check"))
	rows := bus.Mutable.EmittedEvidence()
	if len(rows) != 1 || !rows[0].IsCitable() || rows[0].LineStart != 6 || rows[0].LineEnd != 6 || rows[0].Object != "engine.Check" {
		t.Fatalf("post-grounding coordinates did not own the canonical receiver: %+v", rows)
	}
}

func TestB1685ArgumentCanonicalizationClosesOnlyExactCompletionRepair(t *testing.T) {
	const file = "pipeline.go"
	bus := b1685ReadSources(t, map[string]string{file: "package pipeline\ntype Pipeline struct { bus BusContext }\nfunc (p *Pipeline) run() {\n ctx := builder.Build(p.bus, types.AgentExtractor)\n return agent.Execute(ctx)\n}\n"})
	bus.AnalysisIR = flowOperationCompletionContext(nil).AnalysisIR
	bus.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "Extractor", ResolvedAs: "types.AgentExtractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
	}
	bus.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired}}}
	// Controlled exact index for completion navigation; the separate agent
	// test also runs the actual scanner/parser and endpoint-binding producer.
	bus.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{file: {
		RelPath: file, Language: repotypes.LangGo, Package: "pipeline",
		Symbols: []repotypes.Symbol{{Name: "bus", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 2},
			{Name: "run", Kind: "method", Receiver: "Pipeline", Line: 3, EndLine: 6}},
		Relations: []repotypes.Relation{
			{Kind: "call", File: file, Line: 4, FromEP: repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 4}, ToEP: repotypes.RelationEndpoint{Name: "Build", Receiver: "builder", Line: 4}},
			{Kind: "call", File: file, Line: 5, FromEP: repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 5}, ToEP: repotypes.RelationEndpoint{Name: "Execute", Receiver: "agent", Line: 5}},
		},
	}}))
	bus.Mutable.EvidenceClosure().SetReadSet(map[string]bool{file: true})
	bus.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{file: {{Start: 1, End: 6}}})
	assignment := b1685ArgumentItem(file, 4, "ctx", "builder.Build")
	assignment["anchor_kind"] = "assignment"
	b1685Emit(t, bus, assignment, b1685ArgumentItem(file, 4, "p.bus", "builder.Build"), b1685ArgumentItem(file, 4, "types.AgentExtractor", "builder.Build"))
	before, err := (&EmitInvestigationComplete{}).Execute(bus, flowOperationCompletionParams(t))
	if err != nil || !before.Success || before.Repair == nil || before.Repair.Code != "flow_value_consumer_evidence" ||
		!strings.Contains(before.Repair.Hint, `object="agent.Execute"`) {
		t.Fatalf("premise: exact source consumer must be current public completion debt: %v %+v", err, before)
	}
	b1685Emit(t, bus, b1685ArgumentItem(file, 5, "ctx", "Execute"))
	if !flowNavigationArgumentFlowAlreadyEmitted(bus.Mutable.EmittedEvidence(), file, 5, "ctx", "agent.Execute") {
		t.Fatal("source-canonical accepted handoff is not recognized by completion navigation")
	}
	after, err := (&EmitInvestigationComplete{}).Execute(bus, flowOperationCompletionParams(t))
	if err != nil || !after.Success || (after.Repair != nil && after.Repair.Code == "flow_value_consumer_evidence") {
		t.Fatalf("same source operation was requested again after successful short-name emit: %v %+v", err, after)
	}
	for _, miss := range []struct {
		source, argument, receiver string
		line                       int
	}{
		{"elsewhere.go", "ctx", "agent.Execute", 5}, {file, "ctx", "agent.Execute", 4},
		{file, "other", "agent.Execute", 5}, {file, "ctx", "peer.Execute", 5},
	} {
		if flowNavigationArgumentFlowAlreadyEmitted(bus.Mutable.EmittedEvidence(), miss.source, miss.line, miss.argument, miss.receiver) {
			t.Fatalf("normalization discharged a different exact operation: %+v", miss)
		}
	}
}
