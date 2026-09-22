package skill_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

var summaryScopeTeachingCases = []string{
	"finite_trace", "finite_log", "source_mechanism", "finite_with_source", "bounded_effect", "causal_trace",
}

func summaryScopeTeachingContext(name, language string) *types.AgentContext {
	ac := &types.AgentContext{
		AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Language: language,
		Objective: "Answer the requested facts and independently requested explanation dimensions.",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, Language: language,
		}},
	}
	if language == "zh" {
		ac.Objective = "回答所请求的事实，以及独立明确要求解释的维度。"
	}
	rm := &ac.AnalysisIR.RequestModel
	rm.RawRequest = ac.Objective
	if name == "source_mechanism" {
		rm.AnalyzerHints.Kind = string(types.ReqMechanism)
		return ac
	}
	rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration},
	}
	if name == "finite_log" {
		rm.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}
		ac.LogTriage = &types.LogBundle{Errors: []types.LogError{
			{Type: "FirstError", Frames: []types.LogFrame{{File: "first.go", Line: 11, Func: "failFirst"}, {File: "first.go", Line: 22, Func: "callFirst"}}},
			{Type: "PeerError", Frames: []types.LogFrame{{File: "peer.go", Line: 33, Func: "failPeer"}, {File: "peer.go", Line: 44, Func: "callPeer"}}},
		}}
		rm.LogTriage = ac.LogTriage
		return ac
	}
	start, end := 12.0, 12.04
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end,
	}
	rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 17, Thread: "worker-17", Source: "user_explicit"}}
	ac.RuntimeArtifactPreflight = types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
		SourceNavigationOptional: true,
		Artifacts:                []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace", Carrier: "request_path"}},
	})
	switch name {
	case "bounded_effect":
		rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeBoundedEffectVerdict
	case "causal_trace":
		rm.Intent = types.IntentRootCause
		rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
		rm.RuntimeQuestionProfile.FactFamilies = nil
		rm.RuntimeQuestionProfile.FrameCausalityRequested = true
		rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested = true
	case "finite_with_source":
		// A finite runtime dimension does not cancel an independent precise
		// source requirement. No request-word inference creates this fixture.
		rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions:          []types.RequestedAnswerDimension{{Index: 1, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true}},
		}
		rm.AnalyzerHints = types.AnalyzerHints{
			Kind:              string(types.ReqMechanism),
			RequiredFileHints: []types.RequiredFileHint{{Path: "worker.go", Confidence: 1, RequestedDimensionIndices: []int{1}}},
		}
		rm.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{ac.Objective},
		}
	}
	return ac
}

// Exercise the registry -> production context renderer -> actual system
// message boundary. These are teaching regression pins, not runtime scans of
// user text or model answers. User-side evidence cannot satisfy a system pin.
func summaryScopeSystemTeaching(t *testing.T, ac *types.AgentContext) string {
	t.Helper()
	r := skill.NewRegistry()
	skill.RegisterDefaults(r)
	cfg, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(ac)
	if err != nil {
		t.Fatal(err)
	}
	var system, user strings.Builder
	for _, message := range promptcontext.ToMessages(promptcontext.BuildPromptContext(ac, cfg)) {
		switch message.Role {
		case "system":
			system.WriteString(message.Content)
			system.WriteByte('\n')
		case "user":
			user.WriteString(message.Content)
			user.WriteByte('\n')
		}
	}
	after, err := json.Marshal(ac)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rendering teaching changed typed scope, source requirements, or evidence: err=%v", err)
	}
	if !strings.Contains(user.String(), ac.Objective) {
		t.Fatal("actual user message lost the objective")
	}
	if system.Len() == 0 {
		t.Fatal("missing system teaching message")
	}
	return system.String()
}

func summaryScopeTeachingParagraphs(t *testing.T, system string) map[string]string {
	t.Helper()
	paragraphs := make(map[string]string)
	for _, line := range strings.Split(system, "\n") {
		// The later Visual structure bullet also says "For a summary-only";
		// do not let it replace the independently numbered Workflow rule.
		if strings.Contains(line, "For a summary-only") && !strings.HasPrefix(strings.TrimSpace(line), "- ") {
			paragraphs["workflow"] = line
		}
		if strings.HasPrefix(strings.TrimSpace(line), "- `summary` as the only principal block") {
			paragraphs["output_format"] = line
		}
	}
	if len(paragraphs) != 2 {
		t.Fatalf("both independently rendered summary teaching surfaces must remain, found %v", paragraphs)
	}
	return paragraphs
}

func TestSummaryScopeTeachingRejectsAutomaticMechanismExpansion(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		for _, name := range summaryScopeTeachingCases {
			t.Run(language+"/"+name, func(t *testing.T) {
				paragraphs := summaryScopeTeachingParagraphs(t, summaryScopeSystemTeaching(t, summaryScopeTeachingContext(name, language)))
				stale := map[string]string{
					"workflow":      "Include mechanism details with inline `code` references and cross-file relationships.",
					"output_format": "Write a thorough multi-paragraph explanation that fully addresses the user's question: mechanism details, code-level specifics, cross-file relationships.",
				}
				for surface, phrase := range stale {
					if strings.Contains(paragraphs[surface], phrase) {
						t.Errorf("%s still turns the summary carrier into an unconditional mechanism/cross-file requirement: %q", surface, phrase)
					}
				}
			})
		}
	}
}

func TestSummaryScopeTeachingUsesRequestAndEvidenceOnBothSurfaces(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		for _, name := range summaryScopeTeachingCases {
			t.Run(language+"/"+name, func(t *testing.T) {
				paragraphs := summaryScopeTeachingParagraphs(t, summaryScopeSystemTeaching(t, summaryScopeTeachingContext(name, language)))
				// Each summary rule must carry the entire scope/evidence boundary;
				// a corrected Workflow must not mask stale OutputFormat (or vice
				// versa), and deleting either rule is not a sufficient repair.
				for surface, paragraph := range paragraphs {
					for _, want := range []string{
						"is the answer body, not a request for extra mechanisms",
						"Match depth to the resolved request scope and required facets",
						"Explain mechanisms, code-level details, and cross-file relationships when needed to answer those facets and supported by accepted evidence",
						"For finite facts or bounded effect verdicts",
						"preserve the requested facts, measurement scope, sources, uncertainty, and reasoning needed for the verdict",
						"do not add an unrequested root-cause investigation or source relationship",
						"when the question needs them",
					} {
						if !strings.Contains(paragraph, want) {
							t.Errorf("%s lost summary scope/evidence teaching %q", surface, want)
						}
					}
				}
				for _, want := range []string{
					"Open with a plain-prose core conclusion",
					"When the contract requires a `diagram`, emit a separate `diagram` block, not Mermaid inside summary `text`",
				} {
					if !strings.Contains(paragraphs["workflow"], want) {
						t.Errorf("summary Workflow lost reader/diagram carrier guidance %q", want)
					}
				}
				if !strings.Contains(paragraphs["output_format"], "a deep explanation must still be thorough") {
					t.Error("scope matching must not become a blanket brevity rule for genuine explanations")
				}
			})
		}
	}
}

func TestSummaryScopeTeachingPreservesIndependentContracts(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		for _, name := range summaryScopeTeachingCases {
			t.Run(language+"/"+name, func(t *testing.T) {
				ac := summaryScopeTeachingContext(name, language)
				system := summaryScopeSystemTeaching(t, ac)
				summaryScopeTeachingParagraphs(t, system)
				for _, want := range []string{
					"projected `emit_answer_document` tool schema",
					"Do not reconstruct a second schema from prose guidance",
					"Required Answer Blocks",
					"Evidence-entailment boundary (all code explanations)",
					"When an explanation crosses functions or stages, cite separate evidence for each hop",
					"keep visible labels, source locators, and relation evidence separate",
					types.GroundedSourceDiagramEdgeOwnershipContract,
					types.GroundedSourceDiagramRelationEvidenceContract,
					"Display choices neither create nor remove typed evidence, projection, or automatic completion authority",
				} {
					if !strings.Contains(system, want) {
						t.Errorf("scope-specific style change lost independent system teaching %q", want)
					}
				}
				switch name {
				case "source_mechanism", "finite_with_source":
					if !strings.Contains(system, "Describe what the code DOES and why it matters") {
						t.Error("source explanations lost mechanism depth guidance")
					}
					if name == "finite_with_source" && types.RuntimeSourceRequestCurrentSourceRequirementPrecision(&ac.AnalysisIR.RequestModel, ac.TurnRouteHint) != types.RuntimeSourceRequirementPrecise {
						t.Fatal("mixed fixture must retain a precise independent source requirement")
					}
				case "bounded_effect":
					for _, want := range []string{
						"Put the verdict and the core reasoning together using the schema-selected verdict carrier and rationale fields",
						"Explain the supporting observations, conditions, and evidence boundary at the depth the question needs",
						"include mechanism details when needed and evidenced, not merely because this is a decision",
					} {
						if !strings.Contains(system, want) {
							t.Errorf("bounded-effect answers lost evidence-scoped reasoning permission %q", want)
						}
					}
				case "causal_trace":
					for _, want := range []string{
						"When frame/deadline causality is proven, state the causal chain",
						"adjacent or background information is not promoted to an on-chain root cause",
						"explicit capture/target/time-window scope",
						"S alone proves no wait mechanism, and an absent IO marker does not prove non-IO waiting",
					} {
						if !strings.Contains(system, want) {
							t.Errorf("causal diagnosis lost independent evidence authority %q", want)
						}
					}
				case "finite_log":
					for _, want := range []string{
						"Grounded log frames preserve their recorded identities and within-stack order",
						"without inventing cross-stack continuations or additional caller/callee links",
					} {
						if !strings.Contains(system, want) {
							t.Errorf("finite log answer lost stack evidence boundary %q", want)
						}
					}
				}
			})
		}
	}
}
