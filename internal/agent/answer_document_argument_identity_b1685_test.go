package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1685ActualArgumentAliasesPublishOneFinalizerRecipe(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", language, reverse), func(t *testing.T) {
				repo := t.TempDir()
				if err := os.WriteFile(filepath.Join(repo, "flow.go"), []byte("package p\nfunc Run(ac Context) bool {\n return engine.Check(ac)\n}\n"), 0600); err != nil {
					t.Fatal(err)
				}
				bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Language: language,
					Mutable: types.NewMutableState("Explain the state handoff"),
					AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
						Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain, PredicateAxis: types.AxisFlow,
						AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
						DiagramHint:   &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true},
					}},
				}
				read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"flow.go","line_offset":0,"limit":30}`))
				if err != nil || !read.Success || read.ReadCoverage == nil {
					t.Fatalf("actual read: %v %+v", err, read)
				}
				bus.Mutable.AppendDispatchToolResult(read)
				receivers := []string{"Check", "engine.Check"}
				if reverse {
					receivers[0], receivers[1] = receivers[1], receivers[0]
				}
				for _, receiver := range receivers {
					params, _ := json.Marshal(map[string]any{"items": []map[string]any{{
						"scope": "line", "evidence_kind": "relationship", "subject": "ac", "predicate": "passes argument",
						"object": receiver, "source": "flow.go", "line_start": 3, "anchor_kind": "argument", "anchor_symbol": "ac",
						"summary": "The state is passed to the receiving operation.",
					}}})
					result, err := (&tool.EmitEvidence{}).Execute(bus, params)
					if err != nil || !result.Success {
						t.Fatalf("actual emit: %v %+v", err, result)
					}
					bus.Mutable.AppendDispatchToolResult(result)
				}
				bus.EvidenceItems = bus.Mutable.EmittedEvidence()
				for _, ev := range bus.EvidenceItems {
					if !ev.IsCitable() || types.ClaimFormOf(ev) != types.ClaimArgumentFlow {
						t.Fatalf("actual source proof absent: %+v", ev)
					}
				}
				ctx := promptcontext.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if !strings.Contains(prompt, "edge_recipe[1]") {
					t.Fatalf("public finalizer path did not publish relation recipes: %s", prompt)
				}
				anchors := bus.Mutable.FinalizerTypedRelationRecipeAnchors()
				if len(anchors) != 1 || anchors[0].RelationKind != types.DiagramRelArgumentFlow || anchors[0].FromIdentity != "ac" || anchors[0].ToIdentity != "engine.Check" {
					t.Fatalf("one source operation became multiple or changed typed recipe endpoints: %+v", anchors)
				}
				if strings.Contains(prompt, "node_alias[n3]") || strings.Contains(prompt, "edge_recipe[2]") {
					t.Fatal("short/qualified receiver aliases produced a false extra node or recipe")
				}
			})
		}
	}
}

func TestB1685ActualParserReceiverBindingConsumesCanonicalRecoveredEndpoint(t *testing.T) {
	repo := t.TempDir()
	const source = "package p\ntype Context struct{}\ntype Checker interface { Check(Context) }\nfunc Run(ac Context, engine Checker) {\n\n engine.Check(ac)\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "flow.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := repomap.ScanFiles(repo)
	if err != nil || len(entries) != 1 {
		t.Fatalf("actual scan: %v %+v", err, entries)
	}
	files := repomap.ParseFiles(entries, repo)
	if len(files) != 1 {
		t.Fatalf("actual parser failed to retain source: %+v", files)
	}
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("Explain the handoff")}
	bus.Mutable.SetSearchGraph(repomap.BuildGraph(repo, files))
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"flow.go","line_offset":0,"limit":30}`))
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("actual source read: %v %+v", err, read)
	}
	bus.Mutable.AppendDispatchToolResult(read)
	result, err := (&tool.EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"ac","predicate":"passes argument","object":"Check","source":"flow.go","line_start":5,"anchor_kind":"argument","anchor_symbol":"ac","summary":"The state is passed to the receiving operation."}]}`))
	if err != nil || !result.Success {
		t.Fatalf("actual evidence emit: %v %+v", err, result)
	}
	rows := bus.Mutable.EmittedEvidence()
	if len(rows) != 1 || !rows[0].IsCitable() || rows[0].LineStart != 6 || rows[0].LineEnd != 6 || rows[0].Object != "engine.Check" || rows[0].OwnerIdentity == "" {
		t.Fatalf("actual parser/read/recovery pipeline lost exact endpoint: %+v", rows)
	}
	bound := false
	for _, binding := range rows[0].DeclaredIdentityBindings {
		if binding.Binding == rows[0].OwnerIdentity+".engine" && binding.Type == "Checker" {
			bound = true
		}
	}
	if !bound {
		t.Fatalf("source-normalization ran after typed endpoint binding: %+v", rows[0])
	}
}
