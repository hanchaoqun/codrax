package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc377Capture is one pinned whole original and the question window the
// batch asked about it.
type hmc377Capture struct {
	name, env, sha256 string
	size              int64
	tid               int
	start, end        float64
}

var hmc377Captures = []hmc377Capture{
	{"honeybird", "CODRAX_HMC365_HONEYBIRD", "21280a96618781b37be37f9289be9ba286d6a85d358e60ff72d9c73c827cf30e", 106896120, 64156, 11343.248, 11343.310},
	{"netease_reboot5", "CODRAX_HMC376_NETEASE_REBOOT_FRAME", "48cf2fd8b91a81f2276a16a5988ef14729c078e5d1f23615eb166d2f6e2c8cbc", 104144939, 11077, 153.763595, 153.887340},
	{"douyin_reboot5", "CODRAX_HMC377_DOUYIN_REBOOT", "8087138a5810347675fe7764974f5103cad4a9a71e56d7d1702d65da2256892e", 138438125, 11105, 140.911304, 141.835458},
}

func hmc377PinnedCapture(t *testing.T, c hmc377Capture) string {
	t.Helper()
	path := os.Getenv(c.env)
	if path == "" {
		t.Skipf("requires the pinned complete capture in %s", c.env)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if n, err := io.Copy(h, f); err != nil || n != c.size || hex.EncodeToString(h.Sum(nil)) != c.sha256 {
		t.Fatalf("%s is not the pinned original (%d bytes, %v)", c.env, n, err)
	}
	return path
}

// hmc377CaptureFinalizerContext runs the batch's own explorer query sequence
// at the public trace_query entry and hands the results to the finalizer the
// way the explore stage does.
func hmc377CaptureFinalizerContext(t *testing.T, c hmc377Capture, path string) *types.AgentContext {
	t.Helper()
	root := t.TempDir()
	start, end := c.start, c.end
	request := types.RequestModel{Language: "zh", Intent: types.IntentRootCause, Scenario: types.ScenarioPerformanceBottleneck, PerfTrace: &types.PerfBundle{},
		RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "tid"},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: c.tid, Source: "user_explicit"}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "window"},
	}
	ctx := &types.AgentContext{RepoRoot: root, WorkDir: root, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("main thread time and ranked causes in the asked window"), AnalysisIR: &types.AnalysisIR{RequestModel: request}}
	ctx.Objective = ctx.Mutable.Objective()
	ctx.Mutable.SetRequestModel(request)
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "critical_blocking_calls"} {
		params, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": c.tid, "time_start": start, "time_end": end})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !result.Success {
			t.Fatalf("%s %s: %v %s", c.name, view, err, result.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	return ctx
}

// hmc377CaptureFirstRequest sends the actual first finalizer request of that
// context to a capturing adapter (enforced byte policy, an 800,000-byte
// window as in the batch) and returns it.
func hmc377CaptureFirstRequest(t *testing.T, ctx *types.AgentContext) ([]llm.Message, []llm.ToolSchema) {
	t.Helper()
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	stop := errors.New("captured the first finalizer request")
	adapter := &hmc376Adapter{window: 200000}
	adapter.script = func(int) (llm.Response, error) { return llm.Response{}, stop }
	base := NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: registry, MaxIterations: 1,
		AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}}).(*BaseAgent)
	if _, err := base.Execute(ctx, traceTeachingSkill(t, "answer-document-skill")); !errors.Is(err, stop) || len(adapter.messages) != 1 {
		t.Fatalf("expected exactly the first finalizer request: calls=%d err=%v", len(adapter.messages), err)
	}
	return adapter.messages[0], adapter.tools[0]
}

// The exact candidate roster is in the first request on every capture: in the
// finalizer context and as the schema enum of trace_root_causes, the same set.
// With CODRAX_HMC377_FIRST_REQUEST_DUMP set the request is written there for
// a byte comparison between two builds.
func TestHMC377WholeOriginalFirstRequestCarriesTheExactRoster(t *testing.T) {
	for _, c := range hmc377Captures {
		t.Run(c.name, func(t *testing.T) {
			path := hmc377PinnedCapture(t, c)
			ctx := hmc377CaptureFinalizerContext(t, c, path)
			messages, tools := hmc377CaptureFirstRequest(t, ctx)
			contract := ctx.Mutable.TraceFindingContract()
			selectable := tracefinding.SelectableRootCauseCandidates(contract)
			if contract == nil || !contract.RootCauseReportEnabled || len(selectable) == 0 {
				t.Fatalf("no selectable roster: %+v", contract)
			}
			var context strings.Builder
			for _, message := range messages {
				context.WriteString(message.Content)
			}
			var enum []string
			for _, schema := range tools {
				if schema.Name != hmc376Full {
					continue
				}
				var object struct {
					Properties map[string]struct {
						Properties map[string]struct {
							Items struct {
								Properties map[string]struct {
									Enum []string `json:"enum"`
								} `json:"properties"`
							} `json:"items"`
						} `json:"properties"`
					} `json:"properties"`
				}
				if err := json.Unmarshal(schema.Parameters, &object); err != nil {
					t.Fatal(err)
				}
				enum = object.Properties["trace_root_causes"].Properties["root_causes"].Items.Properties["candidate_id"].Enum
			}
			if len(enum) != len(selectable) {
				t.Fatalf("schema enum has %d ids for %d selectable candidates", len(enum), len(selectable))
			}
			for i, candidate := range selectable {
				id := candidate.Decision.CandidateID
				if enum[i] != id || !strings.Contains(context.String(), `"candidate_id": "`+id+`"`) {
					t.Fatalf("candidate %s is not offered exactly in both the context roster and the schema", id)
				}
				t.Logf("%s roster[%d] %s %s %s %.3f %s", c.name, i, id, candidate.Decision.Token.Token, candidate.Decision.SubjectName, candidate.Decision.Magnitude.Value, candidate.Decision.Magnitude.Unit)
			}
			t.Logf("%s first request: %d messages B + %d tools B", c.name, llm.EstimateMessagesBytes(messages), llm.EstimateToolSchemasBytes(tools))
			if dir := os.Getenv("CODRAX_HMC377_FIRST_REQUEST_DUMP"); dir != "" {
				raw, err := json.MarshalIndent(struct {
					Messages []llm.Message    `json:"messages"`
					Tools    []llm.ToolSchema `json:"tools"`
				}{messages, tools}, "", " ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, c.name+".first-request.json"), raw, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
