package skill_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual registry -> context renderer -> LLM message boundary.
// These are system-teaching pins, not keyword gates on user/model text.
func b1683DiagramPrompt(t *testing.T, language string, kind types.DiagramKind, runtimeTrace bool) string {
	t.Helper()
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	cfg, err := registry.Get("answer-document-skill")
	if err != nil {
		t.Fatal(err)
	}
	ac := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Language:  language,
		Objective: "Explain the requested component relationship without inventing missing connections.",
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				Scenario:      types.ScenarioArchitectureExplain,
				PredicateAxis: types.AxisFlow,
				DiagramHint:   &types.DiagramHint{Kind: kind, Required: true},
			},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{Required: true, RequiredKind: kind}},
		},
	}
	if language == "zh" {
		ac.Objective = "说明所请求的业务组件关系，缺乏证据的连接不要补造。"
	}
	if runtimeTrace {
		ac.Objective = "Explain worker-17 in capture.systrace during 12.000–12.040 s; retain chain-local IO and scheduling evidence."
		ac.AnalysisIR.RequestModel.Intent = types.IntentRootCause
		ac.AnalysisIR.RequestModel.Scenario = types.ScenarioGeneric
		ac.AnalysisIR.RequestModel.LogTriage = &types.LogBundle{}
		ac.RuntimeArtifactPreflight = types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "capture.systrace", Carrier: "request_path",
			}},
		})
	}
	before, err := json.Marshal(ac)
	if err != nil {
		t.Fatal(err)
	}
	var rendered strings.Builder
	for _, message := range promptcontext.ToMessages(promptcontext.BuildPromptContext(ac, cfg)) {
		rendered.WriteString(message.Content)
		rendered.WriteByte('\n')
	}
	after, err := json.Marshal(ac)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rendering teaching changed the request/context: err=%v", err)
	}
	if !strings.Contains(rendered.String(), ac.Objective) {
		t.Fatal("rendering teaching lost the user's objective")
	}
	return rendered.String()
}

func TestB1683DiagramTeachingSeparatesDisplaySourceAndRelation(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		for _, kind := range []types.DiagramKind{types.DiagramFlow, types.DiagramArchitecture, types.DiagramSequence, types.DiagramCallDAG} {
			t.Run(fmt.Sprintf("%s/%s", language, kind), func(t *testing.T) {
				prompt := b1683DiagramPrompt(t, language, kind, false)
				for _, stale := range []string{
					"a diagram of role nodes connected by edges is unconstrained",
					"A label that contains a cited file path is treated as grounded as a whole",
					"the bare identifiers nested inside inherit that grounding",
					"EMBED a cited file:line inside the SAME label text",
					"A separate validator flags any multi-word CamelCase/snake_case token",
					"use a role label rather than inventing a file-shaped node",
					"PREFERRED diagram form is a fenced code block",
				} {
					if strings.Contains(prompt, stale) {
						t.Errorf("rendered prompt still teaches unsupported authority: %q", stale)
					}
				}
				for _, want := range []string{
					"keep visible labels, source locators, and relation evidence separate",
					"concise business/domain names in the answer language",
					"neither a role label nor a cited file:line proves an edge",
					"never add file:line to a label just to confer grounding",
					"disclose that boundary or keep the nodes disconnected",
					"changing a file-shaped label into a role label never proves the missing relationship",
					types.GroundedSourceDiagramEdgeOwnershipContract,
					types.GroundedSourceDiagramRelationEvidenceContract,
					"`diagram.body` is the RAW Mermaid source only",
					"Do NOT add opening/closing Markdown fences",
					"legacy fallback where Mermaid is embedded directly inside a summary/section `text` field",
				} {
					if !strings.Contains(prompt, want) {
						t.Errorf("rendered prompt lacks authority/presentation boundary %q", want)
					}
				}
			})
		}
	}
}

func TestB1683DiagramTeachingPreservesIndependentTraceAuthority(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		t.Run(language, func(t *testing.T) {
			prompt := b1683DiagramPrompt(t, language, types.DiagramFlow, true)
			for _, want := range []string{
				"Runtime Trace diagrams retain their separate typed causal authority",
				"explicit capture/target/time-window scope",
				"adjacent or background information is not promoted to an on-chain root cause",
				"Display choices neither create nor remove typed evidence, projection, or automatic completion authority",
				"Runtime/root-cause trace diagrams use their separate typed causal-relation authority",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("rendered Trace prompt lacks scoped causal boundary %q", want)
				}
			}
		})
	}
}

func TestB1683LegacyDiagramTeachingCannotShedTypedOwnership(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		t.Run(language, func(t *testing.T) {
			prompt := b1683DiagramPrompt(t, language, types.DiagramArchitecture, false)
			for _, stale := range []string{
				"LEGACY ALTERNATIVE outside the strict grounded call-chain contract",
				"Outside a strict grounded source call-chain diagram, an UNLABELLED edge",
				"A label that matches NONE of the keywords above also counts as label-free and does not require an edge anchor",
				"Typed declarations always count toward the minimum; label-only declarations also count",
			} {
				if strings.Contains(prompt, stale) {
					t.Errorf("legacy teaching grants a broader exception than the typed contract: %q", stale)
				}
			}
			for _, want := range []string{
				"LEGACY ALTERNATIVE for presentation-only arrows outside mandatory typed relation ownership",
				"no `relation_kind` or other typed relation claim is declared",
				"Only in that presentation-only legacy lane, an UNLABELLED edge",
				"Removing a label or metadata cannot convert a requested source relation into presentation-only decoration",
				"recognized display-relation labels may count toward its relation-kind minimum",
				"Such counting is not evidence and cannot satisfy typed ownership",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("legacy teaching lacks the original permission boundary: %q", want)
				}
			}
		})
	}
}

func TestB1683LogDiagramTeachingPreservesSameStackCallsWithoutPeerBridges(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		for _, peer := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/peer=%t", language, peer), func(t *testing.T) {
				bundle := &types.LogBundle{
					Meta: types.LogMeta{Lang: "go", Signals: []types.LogSignal{types.SignalPanic, types.SignalCrash}},
					Errors: []types.LogError{{Type: "PrimaryError", Frames: []types.LogFrame{
						{File: "worker.go", Line: 10, Func: "loadItem", Confidence: 1},
						{File: "worker.go", Line: 20, Func: "serveRequest", Confidence: 1},
					}}},
					ResolvedFiles: []string{"worker.go"}, Coverage: 1,
				}
				if peer {
					bundle.Errors = append(bundle.Errors, types.LogError{Type: "PeerError", Frames: []types.LogFrame{
						{File: "peer.go", Line: 30, Func: "flushQueue", Confidence: 1},
						{File: "peer.go", Line: 40, Func: "runPeer", Confidence: 1},
					}})
					bundle.ResolvedFiles = append(bundle.ResolvedFiles, "peer.go")
				}
				registry := skill.NewRegistry()
				skill.RegisterDefaults(registry)
				cfg, err := registry.Get("answer-document-skill")
				if err != nil {
					t.Fatal(err)
				}
				ac := &types.AgentContext{AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
					Language: language, Objective: "Explain the observed error stacks and their proven relationships.", LogTriage: bundle}
				before, err := json.Marshal(bundle)
				if err != nil {
					t.Fatal(err)
				}
				var rendered strings.Builder
				for _, message := range promptcontext.ToMessages(promptcontext.BuildPromptContext(ac, cfg)) {
					rendered.WriteString(message.Content)
					rendered.WriteByte('\n')
				}
				prompt := rendered.String()
				after, err := json.Marshal(bundle)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("prompt rendering changed stack authority: err=%v", err)
				}
				for _, want := range []string{
					"Grounded log frames preserve their recorded identities and within-stack order",
					"any stack/call relationships established by the artifact within that same stack",
					"without inventing cross-stack continuations or additional caller/callee links",
					"### Call chain (innermost → outer)",
					"The panic / crash frames above describe the call chain",
					"loadItem", "serveRequest", "caller frame 1",
				} {
					if !strings.Contains(prompt, want) {
						t.Errorf("public LogBundle prompt lacks scoped stack relationship %q", want)
					}
				}
				if strings.Contains(prompt, "Grounded log frames support only their recorded identities and ordering") {
					t.Error("generic diagram teaching negates the same artifact's established stack/call relationships")
				}
				if peer {
					for _, want := range []string{
						"flushQueue", "runPeer", "PeerError",
						"no explicit cause-chain marker connects one stack to another",
						"do not establish a cross-error call/causal edge",
					} {
						if !strings.Contains(prompt, want) {
							t.Errorf("peer-stack observation or non-bridging boundary lost: %q", want)
						}
					}
					chain := prompt[strings.Index(prompt, "### Call chain (innermost → outer)"):]
					fenceStart := strings.Index(chain, "```mermaid\n")
					if fenceStart < 0 {
						t.Fatal("missing same-stack diagram floor")
					}
					chain = chain[fenceStart+len("```mermaid\n"):]
					fenceEnd := strings.Index(chain, "```")
					if fenceEnd < 0 {
						t.Fatal("missing diagram floor fence end")
					}
					if strings.Contains(chain[:fenceEnd], "peer.go") || strings.Contains(chain[:fenceEnd], "flushQueue") {
						t.Fatal("independent peer stack was joined into the primary stack diagram")
					}
				}
			})
		}
	}
}
