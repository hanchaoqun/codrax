package orchestrator

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Canned model decisions, but the classifier parser, guards, Run, Explorer
// tool surface, selected preparation and native query are the public paths.
// No trace attachment, PerfTrace, preflight profile or prepared file is seeded.
type hmc264NamedZIPAdapter struct {
	response llm.Response
	requests [][]llm.Message
	visible  map[string]bool
}

func (a *hmc264NamedZIPAdapter) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	a.requests = append(a.requests, append([]llm.Message(nil), messages...))
	a.visible = map[string]bool{}
	for _, schema := range schemas {
		a.visible[schema.Name] = true
	}
	if len(a.requests) > 1 {
		return llm.Response{}, fmt.Errorf("bounded test stops after one actual tool call")
	}
	return a.response, nil
}
func (*hmc264NamedZIPAdapter) ModelID() string               { return "hmc264-scripted-public-entry" }
func (*hmc264NamedZIPAdapter) MaxContextTokens() int         { return 128000 }
func (*hmc264NamedZIPAdapter) MaxOutputTokens() int          { return 4096 }
func (*hmc264NamedZIPAdapter) RequestTimeout() time.Duration { return 0 }
func (*hmc264NamedZIPAdapter) RetryMaxAttempts() int         { return 0 }

func hmc264ZIP(t *testing.T, unsafe bool) (string, []byte) {
	t.Helper()
	const headerSize, attrSize = 104, 48
	data := make([]byte, headerSize+attrSize)
	copy(data, "PERFILE2")
	for offset, value := range map[int]uint64{8: headerSize, 16: attrSize, 24: headerSize, 32: attrSize, 40: headerSize + attrSize, 48: 48} {
		binary.LittleEndian.PutUint64(data[offset:], value)
	}
	binary.LittleEndian.PutUint32(data[headerSize+4:], 40)
	binary.LittleEndian.PutUint64(data[headerSize+24:], 1<<0|1<<1|1<<2|1<<7|1<<8)
	record := make([]byte, 48)
	binary.LittleEndian.PutUint32(record, 9)
	binary.LittleEndian.PutUint16(record[6:], 48)
	binary.LittleEndian.PutUint64(record[8:], 0x1234)
	binary.LittleEndian.PutUint32(record[16:], 42)
	binary.LittleEndian.PutUint32(record[20:], 42)
	binary.LittleEndian.PutUint64(record[24:], 1_000_000_000)
	binary.LittleEndian.PutUint32(record[32:], 3)
	binary.LittleEndian.PutUint64(record[40:], 99)
	data = append(data, record...)
	var raw bytes.Buffer
	w := zip.NewWriter(&raw)
	names := []string{"captures/device capture.data", "other/独立原件"}
	if unsafe {
		names = append(names, "../escape.data")
	}
	for _, name := range names {
		member, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "采样资料.zip")
	if err := os.WriteFile(path, raw.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path, raw.Bytes()
}

func hmc264Messages(messages []llm.Message) string {
	var b strings.Builder
	for _, m := range messages {
		b.WriteString(m.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestHMC264NamedZIPClassifierToRunToNativeQuery(t *testing.T) {
	for _, test := range []struct {
		name   string
		unsafe bool
		parent bool
	}{{"native_selected", false, false}, {"unsafe", true, false}, {"external_parent", false, true}} {
		t.Run(test.name, func(t *testing.T) {
			unsafe := test.unsafe
			repo, anchor := t.TempDir(), t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			path, original := hmc264ZIP(t, unsafe)
			request := "请看 `" + path + "` 里的 captures/device capture.data，在0.9到1.1秒内线程42的采样记录。"
			classifierModel := &hmc264NamedZIPAdapter{response: llm.Response{ToolCalls: []llm.ToolCall{{ID: "classify", Name: "emit_turn_policy", Params: json.RawMessage(`{"route":"repo","needs_repo_access":true,"current_source_evidence_mode":"optional","operation":"investigate","source":"artifact","confidence":0.95,"reason":"read native measurements","requires_diagram":false,"required_outcomes":["measurement"]}`)}}}}
			classifier := repl.NewChitchatClassifier(classifierModel).(repl.SingleShotTurnPolicyClassifier)
			policy, err := classifier.ClassifyPolicySingleShot(repl.WithNamedInputRoutingContext(t.Context(), request, repo, anchor), request, "", false)
			if err != nil {
				t.Fatal(err)
			}
			policy = repl.ApplyTurnPolicyGuards(policy, false, false)
			if policy.Source != "artifact" || policy.CurrentSourceEvidenceMode != types.TurnRouteCurrentSourceEvidenceOptional {
				t.Fatalf("candidate rewrote accepted domain: %+v", policy)
			}
			classifierPrompt := hmc264Messages(classifierModel.requests[0])
			if !unsafe && (!strings.Contains(classifierPrompt, `"archive_candidates"`) || !strings.Contains(classifierPrompt, `"other/独立原件"`)) {
				t.Error("actual classifier did not receive the safe complete member navigation")
			}
			if unsafe && strings.Contains(classifierPrompt, `"archive_candidates"`) {
				t.Error("unsafe ZIP supplied candidate authority")
			}

			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "archive_member": "captures/device capture.data", "view": "event_search", "time_start": 0.9, "time_end": 1.1})
			toolName := "trace_query"
			if test.parent {
				toolName = "trace_catalog"
				params, _ = json.Marshal(map[string]any{"root": filepath.Dir(path)})
			}
			model := &hmc264NamedZIPAdapter{response: llm.Response{ToolCalls: []llm.ToolCall{{ID: "selected-native-query", Name: toolName, Params: params}}}}
			registry := tool.NewRegistry()
			registry.Register(&tool.TraceQuery{})
			registry.Register(&tool.TraceCatalog{})
			explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: model, Tools: registry, MaxIterations: 1, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
			ir := dagIR(types.AnswerContract{})
			ir.RequestModel = types.RequestModel{RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
				ExternalObservationPolicy: &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceAllow, ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly}}
			ir.TaskGraph = types.TaskGraph{Nodes: []types.TaskNode{{ID: "native-query", Type: types.NodeEvidence, Objective: request, OneShot: true}}, ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1}}
			ir.EvidencePlan.Budget.MaxReactIters = 1
			var output *agent.StageOutput
			var native *tracequery.Result
			ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					if ctx.RuntimeArtifactPreflight.HasTraceArtifact() || len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
						t.Error("ordinary ZIP was prefilled as admitted trace before accepted analysis")
					}
					return &agent.StageOutput{AnalysisIR: ir}, nil
				},
				types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					if len(ctx.TraceInputPreparer.PreparedMaterials()) != 0 {
						t.Error("unselected candidates manufactured ready material")
					}
					var err error
					output, err = explorer.Execute(ctx, &skill.Config{Name: "explore-skill", ToolSuggestions: []string{"trace_query", "trace_catalog"}})
					// The public Run cleans its temporary working directory. Read
					// its real publication while this execution still owns it.
					if output != nil {
						for _, result := range output.ToolResults {
							if result.ToolName != "trace_query" || !result.Success {
								continue
							}
							ref := result.RawRef
							for _, observation := range result.Observations {
								if observation.SourceRef.PayloadRef != "" {
									ref = observation.SourceRef.PayloadRef
									break
								}
							}
							body, readErr := os.ReadFile(ref)
							if readErr != nil {
								t.Error(readErr)
								continue
							}
							var payload tracequery.Result
							if json.Unmarshal(body, &payload) == nil {
								native = &payload
							}
						}
					}
					return output, err
				},
			})
			o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
			o.SetTraceRuntimeAnchor(anchor)
			o.SetTurnRouteHint(repl.TurnRouteHintFromPolicy(policy))
			bus, runErr := o.Run(request, repo, "main")
			if bus == nil || output == nil || len(model.requests) == 0 {
				t.Fatalf("did not reach actual explorer: %v", runErr)
			}
			if unsafe {
				if model.visible["trace_query"] || len(bus.TraceInputPreparer.PreparedMaterials()) != 0 {
					t.Fatal("unsafe candidates entered native tool surface or preparation")
				}
				return
			}
			if test.parent {
				for _, result := range output.ToolResults {
					if result.ToolName == "trace_catalog" {
						if result.Success || len(bus.Mutable.TraceCatalogs()) != 0 || len(bus.TraceInputPreparer.PreparedMaterials()) != 0 {
							t.Fatal("ZIP candidate navigation expanded directory authority")
						}
						return
					}
				}
				t.Fatal("external directory was not tested through actual tool dispatch")
			}
			prompt := hmc264Messages(model.requests[0])
			for _, want := range []string{`"other/独立原件"`, `"candidate_count":2`, `"members_complete":true`, `archive_member`, `not instructions`} {
				if !strings.Contains(prompt, want) {
					t.Errorf("actual Explorer lost navigation %q", want)
				}
			}
			if native == nil || native.EventCount != 1 || len(native.Events) != 1 || native.Events[0].Type != tracequery.EventPerfSample {
				t.Fatalf("normal named selection did not produce native facts: visible=%v output=%+v run=%v", model.visible, output.ToolResults, runErr)
			}
			if len(bus.TraceInputPreparer.PreparedMaterials()) != 1 || bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
				t.Fatal("selected query failed preparation identity or replaced attachment state")
			}
			material := bus.TraceInputPreparer.PreparedMaterials()[0]
			if !material.MatchesSelection(path, "captures/device capture.data") || material.MatchesPath(path) || material.QueryPath() != native.SourcePath {
				t.Fatal("source/member/query identity was not retained")
			}
			if current, err := os.ReadFile(path); err != nil || !bytes.Equal(current, original) {
				t.Fatal("named original modified", err)
			}
		})
	}
}
