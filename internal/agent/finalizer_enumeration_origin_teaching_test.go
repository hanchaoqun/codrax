package agent

import (
	"encoding/json"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The enumeration rationale is rendered into the actual finalizer user
// message. Correcting only answer-document-skill's system teaching cannot
// remove a source-only demand that arrives again through Required Blocks.
func TestFinalizerEnumerationOriginTeachingUsesActualRequiredBlockMessage(t *testing.T) {
	for _, external := range []bool{false, true} {
		for _, lang := range []string{"zh", "en"} {
			name := "source/" + lang
			if external {
				name = "external/" + lang
			}
			t.Run(name, func(t *testing.T) {
				rm := types.RequestModel{RawRequest: "List the supported members.", Language: lang, Intent: types.IntentEnumerate, Scenario: types.ScenarioGeneric,
					AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)}}
				mu := types.NewMutableState(rm.RawRequest)
				bus := &types.BusContext{Language: lang, Mutable: mu}
				if external {
					rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeSystemOverview}
					rm.PerfTrace = &types.PerfBundle{}
					bus.RuntimeArtifactPreflight = types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
						SourceNavigationOptional: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace", Carrier: "request_path"}},
					})
					mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{Success: true, Observations: []types.ObservationRecord{{
						ID: "trace_query:enumeration#thread:17", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
						GroundingPolicy: types.ClaimGroundingHard, SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "capture.systrace", PayloadRef: "native-result.json"},
						Predicate: "thread_summary", Subject: "worker-17", Object: "recorded thread", Value: "1", Unit: "thread",
						Span: types.ObservationSpan{StartTs: 10, EndTs: 10.02, LineStart: 1, LineEnd: 2},
					}}}}})
				} else {
					bus.EvidenceItems = []types.EvidenceItem{{ID: "source-worker", Kind: types.EvidenceDirect, Source: "worker.go", LineStart: 7, LineEnd: 7,
						AnchorKind: types.AnchorDefinition, Snippet: "type Worker struct{}", GroundingStatus: types.GroundingGrounded}}
				}
				mu.SetRequestModel(rm)
				bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}}
				ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
				view := types.BuildAnswerSemanticViewForAgentContext(ctx)
				if view == nil || view.Family != types.QFEnumeration {
					t.Fatalf("fixture must retain enumeration semantics: %+v", view)
				}
				var principal *types.BlockRequirement
				for i := range view.RequiredBlocks {
					block := &view.RequiredBlocks[i]
					if block.Kind == types.BlockOrderedList {
						principal = block
						break
					}
				}
				if principal == nil || !principal.Required || principal.MinCount != 1 || principal.SurfaceRoleHint != types.SurfacePrincipal {
					t.Fatalf("enumeration must retain its required principal member carrier: %+v", principal)
				}
				for _, want := range []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge, types.ClaimAssignmentFact, types.ClaimExternalObservation} {
					found := false
					for _, form := range principal.AcceptableClaimForms {
						found = found || form == want
					}
					if !found {
						t.Errorf("legal source/external claim form removed: %s", want)
					}
				}
				before, _ := json.Marshal([]any{ctx.AnalysisIR, ctx.EvidenceItems, answerDocObservationLedger(ctx), mu.AnswerDocumentV2()})
				instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if !strings.Contains(instruction, "## Required Answer Blocks") || !strings.Contains(instruction, principal.Rationale) ||
					!strings.Contains(instruction, "`block.facet_ids` MUST include") || !strings.Contains(instruction, "`claim_form` MUST be one of") ||
					!strings.Contains(instruction, "external_observation") || !strings.Contains(instruction, "definition_fact") {
					t.Fatal("actual user message lost required members, rationale, facets or claim forms")
				}
				const stale = "Each item names the member with its authoritative file:line"
				if strings.Contains(instruction, stale) {
					t.Errorf("actual finalizer user message imposes repo file:line on every legal enumeration origin: %q", principal.Rationale)
				}
				for _, phrase := range []string{
					"current-source claims need grounded source locations",
					"external observations retain artifact provenance without invented repo lines",
				} {
					if !strings.Contains(instruction, phrase) {
						t.Errorf("actual user message lost source-specific evidence obligation %q", phrase)
					}
				}
				after, _ := json.Marshal([]any{ctx.AnalysisIR, ctx.EvidenceItems, answerDocObservationLedger(ctx), mu.AnswerDocumentV2()})
				if string(before) != string(after) {
					t.Fatal("rendering teaching changed typed scope, source evidence, runtime observations or model document")
				}
			})
		}
	}
}
