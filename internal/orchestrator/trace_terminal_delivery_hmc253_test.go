package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Real Run and public tools: requesting delivery before exploration must not
// freeze the package before subsequent real query attempts finish.
func TestHMC253TerminalExportWaitsForFinalExecution(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolCitation)})
	for _, lane := range []string{"normal", "recovered", "raw", "backtrack"} {
		t.Run(lane, func(t *testing.T) {
			repo, anchor, dumps := t.TempDir(), t.TempDir(), t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			body, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, "capture.data"), body, 0600); err != nil {
				t.Fatal(err)
			}
			ir := dagIR(types.AnswerContract{Language: "en"})
			if lane == "backtrack" {
				ir.AnswerContract.CitationReq = types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 2}
			}
			queries, finalizers := 0, 0
			requested := false
			fns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: dagAnalyzerFn(ir),
				types.AgentExplorer: func(ac *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					bus := types.ToolBusContext(ac, types.AgentExplorer)
					if !requested {
						r, err := (&tool.TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}]}`))
						if err != nil || !r.Success {
							t.Fatalf("discover: %v %s", err, r.Summary)
						}
						c := bus.Mutable.TraceCatalogs()[0]
						args, _ := json.Marshal(map[string]any{"action": "export", "catalog_id": c.ID(), "export": map[string]string{"target": "capture-review"}})
						r, err = (&tool.TraceCatalog{}).Execute(bus, args)
						if err != nil || !r.Success {
							t.Fatalf("request export: %v %s", err, r.Summary)
						}
						var accepted map[string]any
						if err := json.Unmarshal([]byte(r.Summary), &accepted); err != nil {
							t.Fatal(err)
						}
						if accepted["status"] != "pending_publication" || accepted["physical_success"] != nil || accepted["manifest_path"] != nil {
							t.Fatalf("early request must not publish a frozen prefix: %s", r.Summary)
						}
						requested = true
					}
					for _, c := range bus.Mutable.TraceCatalogs() {
						for _, q := range c.Snapshot().Queries {
							args, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": q.ID}})
							r, err := (&tool.TraceQuery{}).Execute(bus, args)
							if err != nil || !r.Success {
								t.Fatalf("query: %v %s", err, r.Summary)
							}
							queries++
						}
					}
					return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
				},
				types.AgentFinalizer: func(ac *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					finalizers++
					found, _ := filepath.Glob(filepath.Join(anchor, "deliveries", "*", "evidence-exports", "*", "manifest.json"))
					if len(found) != 0 {
						t.Fatalf("finalizer dispatch is not terminal publication: %v", found)
					}
					answer := "Original analysis stays unchanged."
					if lane == "backtrack" && finalizers > 1 {
						answer += "\n- `Foo` at file.go:1\n- `Bar` at file.go:2"
					}
					doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: answer}}}
					if lane == "raw" {
						// A stale structured draft must not lend ownership to raw.
						doc.Blocks[0].Text = "Superseded structured draft."
						agent.RenderAnswerDocumentWithLastMileSupplements(ac, doc, nil, "en")
					} else {
						ac.Mutable.RewriteAcceptedAnswerDocumentV2(doc)
						answer = agent.RenderAnswerDocumentWithLastMileSupplements(ac, doc, nil, "en")
					}
					return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: answer, AnswerDegraded: lane == "recovered" || lane == "raw", SkipAnswerChecks: lane == "raw"}, nil
				},
			}
			ar, sr, sar := buildRegistries(fns)
			o := New(types.PipelineSettings{}, ar, sr, sar)
			o.SetTraceRuntimeAnchor(anchor)
			// Keep producer references for the current-match assertion. The
			// separate lifecycle test exercises disposable WorkDir cleanup.
			session := filepath.Join(anchor, "session")
			if err := os.MkdirAll(session, 0700); err != nil {
				t.Fatal(err)
			}
			o.SetBlobSessionDir(session)
			o.SetOutputDump(dumps, 10)
			o.SetMaxSteps(20)
			bus, err := o.Run("Inspect the capture and export its evidence as capture-review", repo, "main")
			if err != nil {
				t.Fatal(err)
			}
			wantFinalizers := 1
			if lane == "backtrack" {
				wantFinalizers = 2
			}
			if queries < 2 || finalizers != wantFinalizers {
				t.Fatalf("fixture did not exercise later attempts: %d/%d", queries, finalizers)
			}
			rows, err := tool.AuthorizedTraceCatalogDeliveries(bus)
			if err != nil || len(rows) != 1 || !rows[0].PackageValid || !rows[0].CurrentMatch {
				t.Fatalf("final delivery: %+v %v", rows, err)
			}
			r := rows[0].Receipt
			if r.Target != "capture-review" || r.ExecutionCoverage.Attempts != queries {
				t.Fatalf("original target/final union: %+v queries=%d", r, queries)
			}
			assertHMC252PackageReadable(t, r)
			answer := bus.Mutable.Result()
			if !strings.HasPrefix(answer, "Original analysis stays unchanged.") || strings.Count(answer, "### System delivery:") != 1 || !strings.Contains(answer, r.ManifestPath) {
				t.Fatalf("terminal answer: %s", answer)
			}
			md, err := os.ReadFile(bus.Mutable.FinalAnswerMarkdownPath())
			if err != nil || !strings.Contains(string(md), answer) {
				t.Fatalf("dump not from same answer: %v", err)
			}
			raw, err := os.ReadFile(outputdump.AnswerSurfacesPathForMarkdown(bus.Mutable.FinalAnswerMarkdownPath()))
			if err != nil {
				t.Fatal(err)
			}
			var surface outputdump.AnswerSurfaceArtifact
			if err := json.Unmarshal(raw, &surface); err != nil {
				t.Fatal(err)
			}
			if lane == "raw" {
				if surface.Status != "unavailable" || surface.Primary != "" {
					t.Fatalf("raw borrowed stale model ownership: %+v", surface)
				}
			} else if surface.Status != "available" || !strings.Contains(surface.Primary, "Original analysis stays unchanged.") || strings.Contains(surface.Primary, r.ManifestPath) || strings.Contains(surface.Principal, r.ManifestPath) {
				t.Fatalf("terminal delivery changed model ownership: %+v", surface)
			}
		})
	}
}

func TestHMC253TerminalRecordDoesNotReplayBoundRawCaveat(t *testing.T) {
	m := types.NewMutableState("audit")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: m, Language: "en"}, outputDumpDir: t.TempDir(), emit: func(render.Event) {}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Original model answer."}}}
	answer := o.renderFinalAnswerWithLastMileSupplements(doc, nil)
	answer = o.appendRegisteredAnswerCaveatRawSection(answer, "System-only raw disclosure.")
	o.recordTaskFinalize(&agent.StageOutput{FinalAnswer: answer})
	raw, err := os.ReadFile(outputdump.AnswerSurfacesPathForMarkdown(m.FinalAnswerMarkdownPath()))
	if err != nil {
		t.Fatal(err)
	}
	var surface outputdump.AnswerSurfaceArtifact
	if err := json.Unmarshal(raw, &surface); err != nil {
		t.Fatal(err)
	}
	if m.Result() != answer || surface.Status != "available" || !strings.Contains(surface.Primary, "Original model answer.") || strings.Contains(surface.Primary, "raw disclosure") {
		t.Fatalf("terminal no-op lost or changed bound ownership: %+v", surface)
	}
}
