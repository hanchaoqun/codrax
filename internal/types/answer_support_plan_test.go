package types

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuildAnswerSupportPlan_RootCauseTraceCompilesTypedLanes(t *testing.T) {
	plan := &AnswerSurfacePlan{
		ExternalObservationSeeds: []ExternalObservationSeed{
			{Kind: "error_type", Raw: "panic: runtime error"},
			{Kind: "frame", Raw: "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR(0x0)", File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR", AnchoredFile: "internal/agent/analyzer.go", AnchoredLine: 861},
		},
		LogSourceDriftAnchors: []LogSourceDriftAnchor{
			{File: "internal/agent/analyzer.go", Func: "buildAnalysisIR", ObservedLine: 250, AnchoredLine: 861},
		},
		DriftBoundedSurfaceItems: []EvidenceItem{
			{
				Kind:         EvidenceRelationship,
				Source:       "internal/agent/analyzer.go",
				LineStart:    651,
				AnchorKind:   AnchorCall,
				Subject:      "ParseOutput",
				Object:       "buildAnalysisIR",
				AnchorSymbol: "buildAnalysisIR",
			},
			{
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    861,
				AnchorKind:   AnchorCondition,
				AnchorSymbol: "buildAnalysisIR",
				Condition:    "ctx == nil || ctx.Mutable == nil",
			},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent: IntentRootCause,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "panic: runtime error"}},
		},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	if got.Family != QFRootCauseTrace {
		t.Fatalf("family = %q, want %q", got.Family, QFRootCauseTrace)
	}
	if len(got.Lanes) < 3 {
		t.Fatalf("expected at least 3 support lanes, got %d", len(got.Lanes))
	}
	var sawObserved, sawPath, sawMechanism, sawBoundary bool
	var observedTexts []string
	var boundaryTexts []string
	for _, lane := range got.Lanes {
		switch lane.Kind {
		case SupportLaneObservedArtifact:
			sawObserved = len(lane.Entries) > 0
			for _, entry := range lane.Entries {
				observedTexts = append(observedTexts, entry.Text)
			}
		case SupportLaneCurrentCodePath:
			sawPath = len(lane.Entries) > 0
		case SupportLaneNearestMechanism:
			sawMechanism = len(lane.Entries) > 0
		case SupportLaneUncertaintyBound:
			sawBoundary = len(lane.Entries) > 0
			for _, entry := range lane.Entries {
				boundaryTexts = append(boundaryTexts, entry.Text)
			}
		}
	}
	if !sawObserved || !sawPath || !sawBoundary {
		t.Fatalf("missing compiled lanes: observed=%v path=%v mechanism=%v boundary=%v", sawObserved, sawPath, sawMechanism, sawBoundary)
	}
	if sawMechanism {
		t.Fatal("guard-only drift case should not promote a dedicated nearest_mechanism lane")
	}
	joinedObserved := strings.Join(observedTexts, "\n")
	if !strings.Contains(joinedObserved, `runtime artifact includes stack frame "buildAnalysisIR" at observed internal/agent/analyzer.go:250`) {
		t.Fatalf("observed artifact lane should surface structured frame text, got:\n%s", joinedObserved)
	}
	if strings.Contains(joinedObserved, "buildAnalysisIR(0x0)") {
		t.Fatalf("observed artifact lane should not expose raw stack arguments, got:\n%s", joinedObserved)
	}
	var observedGuidance string
	var observedAllowed []string
	var boundaryAllowed []string
	for _, lane := range got.Lanes {
		if lane.Kind == SupportLaneObservedArtifact {
			observedGuidance = lane.Guidance
			observedAllowed = append(observedAllowed, lane.AllowedBlocks...)
		}
		if lane.Kind == SupportLaneUncertaintyBound {
			boundaryAllowed = append(boundaryAllowed, lane.AllowedBlocks...)
			break
		}
	}
	if !strings.Contains(observedGuidance, "they do not prove caller-side provenance, source-parameter mapping") {
		t.Fatalf("observed artifact guidance missing raw-argument boundary: %q", observedGuidance)
	}
	if got, want := strings.Join(observedAllowed, ","), "summary,caveat"; got != want {
		t.Fatalf("observed lane allowed blocks = %q, want %q", got, want)
	}
	if got, want := strings.Join(boundaryAllowed, ","), "caveat,summary"; got != want {
		t.Fatalf("boundary lane allowed blocks = %q, want %q", got, want)
	}
	var boundaryGuidance string
	for _, lane := range got.Lanes {
		if lane.Kind == SupportLaneUncertaintyBound {
			boundaryGuidance = lane.Guidance
			break
		}
	}
	if !strings.Contains(boundaryGuidance, "do not identify a specific variable, receiver field, or caller-provided value as the likely cause") {
		t.Fatalf("weak boundary guidance should forbid variable-level speculation, got: %q", boundaryGuidance)
	}
	joinedBoundary := strings.Join(boundaryTexts, "\n")
	if !strings.Contains(joinedBoundary, "current grounded code exposes only a protective guard") {
		t.Fatalf("weak guard-only case should surface a boundary-only note, got:\n%s", joinedBoundary)
	}
	if strings.Contains(joinedBoundary, "ctx == nil || ctx.Mutable == nil") {
		t.Fatalf("boundary-only weak guard note should not leak raw guard variables, got:\n%s", joinedBoundary)
	}
}

func TestBuildAnswerSupportPlan_ExternalOnlyObservedArtifactCanBePrincipalList(t *testing.T) {
	plan := &AnswerSurfacePlan{
		RuntimeGroundingDisposition: &RuntimeGroundingDisposition{
			Source:         RuntimeGroundingSystemDetected,
			Reason:         EvidenceFloorWaiverExternalLog,
			Rationale:      "structured log errors were extracted, but no stack frame resolved to a current repository file",
			CitationPolicy: RuntimeGroundingCitationRuntimeObservation,
		},
		ExternalObservationSeeds: []ExternalObservationSeed{
			{Kind: "error_message", Raw: "index out of bounds: index=5, size=3"},
			{Kind: "frame", Func: "demo.bridge.ohSum", File: "src/bridge/Bridge.cj", Line: 18},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Errors: []LogError{{Type: "panic"}}},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	var observed *AnswerSupportLane
	for i := range got.Lanes {
		if got.Lanes[i].Kind == SupportLaneObservedArtifact {
			observed = &got.Lanes[i]
			break
		}
	}
	if observed == nil {
		t.Fatal("expected observed artifact lane")
	}
	if got, want := strings.Join(observed.AllowedBlocks, ","), "summary,ordered_list,bullet_list,caveat"; got != want {
		t.Fatalf("external-only observed lane allowed blocks = %q, want %q", got, want)
	}
	if !strings.Contains(observed.Guidance, "principal answer list") ||
		!strings.Contains(observed.Guidance, "Do not substitute current-repo analysis helpers") {
		t.Fatalf("external-only observed guidance should forbid helper substitution, got: %q", observed.Guidance)
	}
}

func TestBuildAnswerSupportPlan_ExternalOnlyObservedArtifactFiltersPrincipalFramesBySubTopics(t *testing.T) {
	plan := &AnswerSurfacePlan{
		RuntimeGroundingDisposition: &RuntimeGroundingDisposition{
			Source:         RuntimeGroundingSystemDetected,
			Reason:         EvidenceFloorWaiverExternalLog,
			Rationale:      "attached log frames did not resolve to current repo files",
			CitationPolicy: RuntimeGroundingCitationRuntimeObservation,
		},
		ExternalObservationSeeds: []ExternalObservationSeed{
			{Kind: "error_message", Raw: "Cangjie native call failed: index out of bounds"},
			{Kind: "error_message", Raw: "index out of bounds: index=5, size=3"},
			{Kind: "log_frame", Role: "error_head_frame", Lang: "arkts", Func: "NativeBridge.invokeOhSum", File: "entry/src/main/ets/bridges/NativeBridge.ets", Line: 33},
			{Kind: "log_frame", Role: "caller_frame", Lang: "arkts", Func: "HomePage.computeTotal", File: "entry/src/main/ets/pages/Home.ets", Line: 54},
			{Kind: "log_frame", Role: "error_head_frame", Lang: "cangjie", Func: "demo.bridge.ohSum", File: "src/bridge/Bridge.cj", Line: 18},
			{Kind: "log_frame", Role: "caller_frame", Lang: "cangjie", Func: "demo.bridge.checkout", File: "src/bridge/Bridge.cj", Line: 42},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Errors: []LogError{{Type: "Error"}}},
		SubTopics: []SubTopic{
			{Summary: "ArkTS frame", Entities: []string{"NativeBridge.invokeOhSum"}},
			{Summary: "Cangjie frame", Entities: []string{"demo.bridge.ohSum"}},
		},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	var observed *AnswerSupportLane
	for i := range got.Lanes {
		if got.Lanes[i].Kind == SupportLaneObservedArtifact {
			observed = &got.Lanes[i]
			break
		}
	}
	if observed == nil {
		t.Fatal("expected observed artifact lane")
	}
	joined := ""
	for _, entry := range observed.Entries {
		joined += entry.Text + "\n"
	}
	for _, want := range []string{
		`structured runtime error message "Cangjie native call failed: index out of bounds"`,
		`structured runtime error message "index out of bounds: index=5, size=3"`,
		`runtime artifact identifies error head frame "NativeBridge.invokeOhSum"`,
		`runtime artifact identifies error head frame "demo.bridge.ohSum"`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("observed lane missing principal-safe entry %q:\n%s", want, joined)
		}
	}
	for _, notWant := range []string{"HomePage.computeTotal", "demo.bridge.checkout"} {
		if strings.Contains(joined, notWant) {
			t.Fatalf("observed lane should keep non-target support frame %q out of principal-safe entries:\n%s", notWant, joined)
		}
	}
	if !strings.Contains(observed.Guidance, "only frame entries listed in this lane are principal-safe") {
		t.Fatalf("focused observed lane should warn about support-only frames, got: %q", observed.Guidance)
	}
}

func TestBuildAnswerSupportPlan_CallChainCompilesCurrentPathLaneFromStepBackbone(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StepBackbone: []StepSurfaceAnchor{
			{Name: "Index.ets.render", File: "entry/src/main/ets/pages/Index.ets", Line: 42, SurfaceText: "Index.ets.render dispatches the UI action"},
			{Name: "native_bridge_sum", File: "src/native/sum.cpp", Line: 18, SurfaceText: "native_bridge_sum calls into the native bridge"},
			{Name: "demo.bridge.ohSum", File: "src/bridge/Bridge.cj", Line: 27, SurfaceText: "demo.bridge.ohSum returns the bridged result"},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentTrace}, plan)
	if got == nil {
		t.Fatal("expected call-chain support plan")
	}
	if got.Family != QFCallChain {
		t.Fatalf("family = %q, want %q", got.Family, QFCallChain)
	}
	var path *AnswerSupportLane
	for i := range got.Lanes {
		if got.Lanes[i].Kind == SupportLaneCurrentCodePath {
			path = &got.Lanes[i]
			break
		}
	}
	if path == nil {
		t.Fatalf("missing current path lane: %+v", got.Lanes)
	}
	if got, want := strings.Join(path.AllowedBlocks, ","), "summary,ordered_list,diagram"; got != want {
		t.Fatalf("call-chain path allowed blocks = %q, want %q", got, want)
	}
	joined := ""
	for _, entry := range path.Entries {
		joined += entry.Text + " @ " + entry.Location + "\n"
	}
	for _, want := range []string{
		"Index.ets.render",
		"entry/src/main/ets/pages/Index.ets:42",
		"native_bridge_sum",
		"src/native/sum.cpp:18",
		"demo.bridge.ohSum",
		"src/bridge/Bridge.cj:27",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("call-chain lane missing %q:\n%s", want, joined)
		}
	}
	if !strings.Contains(path.Guidance, "search-hint subjects, prior-turn subjects, or runtime frames") {
		t.Fatalf("call-chain guidance should bound non-principal context, got: %q", path.Guidance)
	}
}

func TestBuildAnswerSupportPlan_CallChainKeepsObservationsOutOfPrincipalPath(t *testing.T) {
	plan := &AnswerSurfacePlan{
		ExternalObservationSeeds: []ExternalObservationSeed{
			{Kind: "perf_frame", Raw: "trace span: RenderList"},
		},
		StepBackbone: []StepSurfaceAnchor{
			{Name: "RenderList", File: "ui/list.ets", Line: 88, SurfaceText: "RenderList schedules cell rendering"},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentTrace}, plan)
	if got == nil {
		t.Fatal("expected call-chain support plan")
	}
	var observed *AnswerSupportLane
	var path *AnswerSupportLane
	for i := range got.Lanes {
		switch got.Lanes[i].Kind {
		case SupportLaneObservedArtifact:
			observed = &got.Lanes[i]
		case SupportLaneCurrentCodePath:
			path = &got.Lanes[i]
		}
	}
	if observed == nil || path == nil {
		t.Fatalf("expected observed + path lanes, got: %+v", got.Lanes)
	}
	if strings.Contains(strings.Join(observed.AllowedBlocks, ","), "ordered_list") {
		t.Fatalf("non-external-only observation lane must not allow principal hop items: %+v", observed.AllowedBlocks)
	}
	if !strings.Contains(path.Guidance, "runtime frames as additional principal hops") {
		t.Fatalf("path lane must forbid observation-frame promotion, got: %q", path.Guidance)
	}
}

func TestBuildAnswerSupportPlan_CallChainPrefersSurfaceEvidenceWhenStepBackboneMissesEndpoint(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StepBackbone: []StepSurfaceAnchor{
			{Name: "buildAnalysisIR", File: "internal/agent/analyzer.go", Line: 1039, SurfaceText: "ParseOutput calls buildAnalysisIR"},
			{Name: "reconcileScenario", File: "internal/agent/analyzer.go", Line: 1534, SurfaceText: "reconcileScenario updates the request scenario"},
			{Name: "analyzerSymbolResolver", File: "internal/agent/analyzer.go", Line: 1624, SurfaceText: "analyzerSymbolResolver builds the resolver"},
		},
		SurfaceEvidence: []EvidenceItem{
			{
				Kind:            EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1625,
				AnchorKind:      AnchorCall,
				AnchorSymbol:    "normalizer.Normalize",
				Subject:         "buildAnalysisIR",
				Object:          "normalizer.Normalize",
				Summary:         "normalizer.Normalize builds the TermGraph",
				GroundingStatus: GroundingGrounded,
			},
			{
				Kind:            EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1818,
				AnchorKind:      AnchorCall,
				AnchorSymbol:    "compiler.Compile",
				Subject:         "buildAnalysisIR",
				Object:          "compiler.Compile",
				Summary:         "compiler.Compile builds TaskGraph and EvidencePlan",
				GroundingStatus: GroundingGrounded,
			},
			{
				Kind:            EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1825,
				AnchorKind:      AnchorCall,
				AnchorSymbol:    "hdp.Plan",
				Subject:         "buildAnalysisIR",
				Object:          "hdp.Plan",
				Summary:         "hdp.Plan builds hypotheses",
				GroundingStatus: GroundingGrounded,
			},
			{
				Kind:            EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1844,
				AnchorKind:      AnchorCall,
				AnchorSymbol:    "binder.BindByRelevance",
				Subject:         "buildAnalysisIR",
				Object:          "binder.BindByRelevance",
				Summary:         "binder.BindByRelevance binds hypotheses",
				GroundingStatus: GroundingGrounded,
			},
			{
				Kind:            EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1970,
				AnchorKind:      AnchorCall,
				AnchorSymbol:    "gate.RunWith",
				Subject:         "buildAnalysisIR",
				Object:          "gate.RunWith",
				Summary:         "gate.RunWith runs the quality gate",
				GroundingStatus: GroundingGrounded,
			},
		},
	}
	var earlyNoise []EvidenceItem
	for i := 0; i < 10; i++ {
		earlyNoise = append(earlyNoise, EvidenceItem{
			Kind:            EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1320 + i,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "reconcileStep",
			Subject:         "buildAnalysisIR",
			Object:          "reconcileStep",
			Summary:         "early reconcile helper",
			GroundingStatus: GroundingGrounded,
		})
	}
	plan.SurfaceEvidence = append(earlyNoise, plan.SurfaceEvidence...)

	got := BuildAnswerSupportPlan(RequestModel{
		Intent: IntentTrace,
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"buildAnalysisIR", "gate.Run", "analyzer.go"},
		},
	}, plan)
	if got == nil {
		t.Fatal("expected call-chain support plan")
	}
	var path *AnswerSupportLane
	for i := range got.Lanes {
		if got.Lanes[i].Kind == SupportLaneCurrentCodePath {
			path = &got.Lanes[i]
			break
		}
	}
	if path == nil {
		t.Fatalf("missing current path lane: %+v", got.Lanes)
	}
	joined := ""
	for _, entry := range path.Entries {
		joined += entry.Text + "\n"
	}
	for _, want := range []string{"normalizer.Normalize", "compiler.Compile", "hdp.Plan", "binder.BindByRelevance", "gate.RunWith"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("surface evidence endpoint chain missing %q:\n%s", want, joined)
		}
	}
	for _, notWant := range []string{"reconcileScenario", "analyzerSymbolResolver"} {
		if strings.Contains(joined, notWant) {
			t.Fatalf("stale step backbone entry %q should not win over endpoint-covering surface evidence:\n%s", notWant, joined)
		}
	}
}

func TestCallChainCondenseSupportEntriesPreservesTerminalTailWhenEndpointIsBroad(t *testing.T) {
	var entries []AnswerSupportEntry
	for i := 0; i < 20; i++ {
		entries = append(entries, AnswerSupportEntry{
			Text: fmt.Sprintf("hop %02d calls gate.RunWith", i),
		})
	}

	got := callChainCondenseSupportEntries(entries, []string{"gate.RunWith"}, 12)
	if len(got) != 12 {
		t.Fatalf("expected condensed lane at limit, got %d: %+v", len(got), got)
	}
	joined := ""
	for _, entry := range got {
		joined += entry.Text + "\n"
	}
	for _, want := range []string{"hop 14", "hop 15", "hop 16", "hop 17", "hop 18", "hop 19"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("terminal tail entry %q was dropped from condensed call chain:\n%s", want, joined)
		}
	}
}

func TestCallChainCondenseSupportEntriesKeepsModerateExplicitChainWhole(t *testing.T) {
	var entries []AnswerSupportEntry
	for i := 0; i < 13; i++ {
		entries = append(entries, AnswerSupportEntry{
			Text: fmt.Sprintf("hop %02d", i),
		})
	}

	got := callChainCondenseSupportEntries(entries, []string{"hop 12"}, callChainSupportEntryLimit)
	if len(got) != len(entries) {
		t.Fatalf("moderate explicit chain should remain whole, got %d entries want %d", len(got), len(entries))
	}
	for i := range entries {
		if got[i].Text != entries[i].Text {
			t.Fatalf("entry %d = %q, want %q", i, got[i].Text, entries[i].Text)
		}
	}
}

func TestBuildAnswerSupportPlanForAgentContext_CurrentStatusAddsVerdictLane(t *testing.T) {
	ctx := &AgentContext{
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent: IntentRootCause,
				LogTriage: &LogBundle{
					Errors:        []LogError{{Type: "panic"}},
					ResolvedFiles: []string{"internal/agent/analyzer.go"},
				},
			},
			AnswerContract: AnswerContract{
				CurrentStatusDiagnostic: &CurrentStatusDiagnosticContract{
					Required: true,
					AllowedVerdicts: []CurrentStatusVerdict{
						CurrentStatusStillPresent,
						CurrentStatusFixed,
						CurrentStatusNotEnoughEvidence,
					},
				},
			},
		},
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "panic", Frames: []LogFrame{{
				File: "internal/agent/analyzer.go",
				Line: 250,
				Func: "buildAnalysisIR",
			}}}},
			ResolvedFiles: []string{"internal/agent/analyzer.go"},
		},
		Mutable: NewMutableState(""),
		EvidenceItems: []EvidenceItem{
			{
				Kind:         EvidenceRelationship,
				Source:       "internal/agent/analyzer.go",
				LineStart:    743,
				AnchorKind:   AnchorCall,
				Subject:      "ParseOutput",
				Object:       "buildAnalysisIR",
				AnchorSymbol: "buildAnalysisIR",
			},
			{
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    978,
				AnchorKind:   AnchorCondition,
				AnchorSymbol: "buildAnalysisIR",
				Condition:    "ctx == nil || ctx.Mutable == nil",
			},
		},
	}

	got := BuildAnswerSupportPlanForAgentContext(ctx)
	if got == nil {
		t.Fatal("expected support plan")
	}
	var verdictLane *AnswerSupportLane
	var observedAllowed []string
	for i := range got.Lanes {
		switch got.Lanes[i].Kind {
		case SupportLaneCurrentVerdict:
			verdictLane = &got.Lanes[i]
		case SupportLaneObservedArtifact:
			observedAllowed = append(observedAllowed, got.Lanes[i].AllowedBlocks...)
		}
	}
	if verdictLane == nil {
		t.Fatalf("current-status diagnostic should add verdict lane; lanes=%+v", got.Lanes)
	}
	if strings.Join(verdictLane.AllowedBlocks, ",") != "decision" {
		t.Fatalf("verdict lane allowed blocks = %v, want [decision]", verdictLane.AllowedBlocks)
	}
	if len(verdictLane.Entries) == 0 {
		t.Fatal("verdict lane should carry located evidence entries for citation routing")
	}
	if got, want := strings.Join(observedAllowed, ","), "summary,caveat"; got != want {
		t.Fatalf("observed lane boundaries should remain narrow, got %q want %q", got, want)
	}
}

func TestBuildAnswerSupportPlan_RootCauseTraceKeepsGuardProtectedAccessOutOfNearestMechanism(t *testing.T) {
	plan := &AnswerSurfacePlan{
		DriftBoundedSurfaceItems: []EvidenceItem{
			{
				Kind:         EvidenceRelationship,
				Source:       "internal/agent/analyzer.go",
				LineStart:    743,
				AnchorKind:   AnchorCall,
				Subject:      "ParseOutput",
				Object:       "buildAnalysisIR",
				AnchorSymbol: "buildAnalysisIR",
			},
			{
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    978,
				AnchorKind:   AnchorCondition,
				AnchorSymbol: "buildAnalysisIR",
				Snippet:      "if ctx == nil || ctx.Mutable == nil {",
			},
			{
				Kind:         EvidenceDirect,
				Source:       "internal/agent/analyzer.go",
				LineStart:    981,
				AnchorKind:   AnchorAssignment,
				AnchorSymbol: "buildAnalysisIR",
				Snippet:      "raw := ctx.Mutable.RequestModel()",
			},
		},
	}
	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Errors: []LogError{{Type: "panic: runtime error"}}},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	var mechanismLane *AnswerSupportLane
	var boundaryLane *AnswerSupportLane
	for i := range got.Lanes {
		if got.Lanes[i].Kind == SupportLaneNearestMechanism {
			mechanismLane = &got.Lanes[i]
		}
		if got.Lanes[i].Kind == SupportLaneUncertaintyBound {
			boundaryLane = &got.Lanes[i]
		}
	}
	if mechanismLane != nil && len(mechanismLane.Entries) > 0 {
		t.Fatalf("guard-protected post-check access should not be promoted into nearest_mechanism: %+v", mechanismLane.Entries)
	}
	if boundaryLane == nil || len(boundaryLane.Entries) == 0 {
		t.Fatal("expected uncertainty boundary lane when only a guard survives as grounded mechanism support")
	}
	if !strings.Contains(boundaryLane.Entries[len(boundaryLane.Entries)-1].Text, "only a protective guard") {
		t.Fatalf("weak guard-only case should stay in boundary lane, got %+v", boundaryLane.Entries)
	}
}

func TestBuildAnswerSupportPlan_RootCauseTracePathLanePrefersObservedFrameTransition(t *testing.T) {
	plan := &AnswerSurfacePlan{
		LogObservedAnchors: []LogSourceDriftAnchor{
			{File: "internal/agent/analyzer.go", Func: "buildAnalysisIR", ObservedLine: 250, AnchoredLine: 978},
			{File: "internal/agent/analyzer.go", Func: "(*analyzerEvaluator).ParseOutput", ObservedLine: 320, AnchoredLine: 743},
		},
		DriftBoundedSurfaceItems: []EvidenceItem{
			{
				Kind:         EvidenceRelationship,
				Source:       "internal/agent/analyzer.go",
				LineStart:    743,
				AnchorKind:   AnchorCall,
				Subject:      "ParseOutput",
				Object:       "buildAnalysisIR",
				AnchorSymbol: "buildAnalysisIR",
			},
			{
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    978,
				AnchorKind:   AnchorCondition,
				AnchorSymbol: "buildAnalysisIR",
				Snippet:      "if ctx == nil || ctx.Mutable == nil {",
			},
			{
				Kind:         EvidenceDirect,
				Source:       "internal/agent/analyzer.go",
				LineStart:    981,
				AnchorKind:   AnchorCall,
				Subject:      "buildAnalysisIR",
				Object:       "RequestModel",
				AnchorSymbol: "RequestModel",
				Snippet:      "raw := ctx.Mutable.RequestModel()",
			},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Errors: []LogError{{Type: "panic: runtime error"}}},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}

	var pathLane *AnswerSupportLane
	for i := range got.Lanes {
		if got.Lanes[i].Kind == SupportLaneCurrentCodePath {
			pathLane = &got.Lanes[i]
			break
		}
	}
	if pathLane == nil {
		t.Fatal("expected current_code_path lane")
	}
	joined := make([]string, 0, len(pathLane.Entries))
	for _, entry := range pathLane.Entries {
		joined = append(joined, entry.Text)
	}
	body := strings.Join(joined, "\n")
	if !strings.Contains(body, "ParseOutput") || !strings.Contains(body, "buildAnalysisIR") {
		t.Fatalf("path lane should keep the observed frame transition, got:\n%s", body)
	}
	if strings.Contains(body, "RequestModel") {
		t.Fatalf("path lane should not elevate intra-function helper calls into the principal path, got:\n%s", body)
	}
}

func TestBuildAnswerSupportPlan_RootCauseTracePromotesIndependentMechanismCompanion(t *testing.T) {
	plan := &AnswerSurfacePlan{
		DriftBoundedSurfaceItems: []EvidenceItem{
			{
				Kind:         EvidenceRelationship,
				Source:       "internal/agent/analyzer.go",
				LineStart:    743,
				AnchorKind:   AnchorCall,
				Subject:      "ParseOutput",
				Object:       "buildAnalysisIR",
				AnchorSymbol: "buildAnalysisIR",
			},
			{
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    978,
				AnchorKind:   AnchorCondition,
				AnchorSymbol: "buildAnalysisIR",
				Condition:    "ctx == nil || ctx.Mutable == nil",
			},
			{
				Kind:         EvidenceDirect,
				Source:       "internal/agent/analyzer.go",
				LineStart:    1000,
				AnchorKind:   AnchorAssignment,
				AnchorSymbol: "buildAnalysisIR",
				Snippet:      "rm.AnalyzerHints.PrimaryEntities = append([]string(nil), rm.AnalyzerHints.Entities...)",
			},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Errors: []LogError{{Type: "panic: runtime error"}}},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	var mechanismLane *AnswerSupportLane
	for i := range got.Lanes {
		if got.Lanes[i].Kind == SupportLaneNearestMechanism {
			mechanismLane = &got.Lanes[i]
			break
		}
	}
	if mechanismLane == nil || len(mechanismLane.Entries) == 0 {
		t.Fatal("independent inner companion should still promote nearest_mechanism lane")
	}
	if !strings.Contains(strings.Join(func() []string {
		out := make([]string, 0, len(mechanismLane.Entries))
		for _, entry := range mechanismLane.Entries {
			out = append(out, entry.Text)
		}
		return out
	}(), "\n"), "PrimaryEntities") {
		t.Fatalf("mechanism lane should keep the independent companion, got %+v", mechanismLane.Entries)
	}
}

func TestBuildAnswerSupportPlan_RootCauseTraceKeepsControlHeaderAssignmentsOutOfNearestMechanism(t *testing.T) {
	plan := &AnswerSurfacePlan{
		DriftBoundedSurfaceItems: []EvidenceItem{
			{
				Kind:         EvidenceRelationship,
				Source:       "internal/agent/analyzer.go",
				LineStart:    743,
				AnchorKind:   AnchorCall,
				Subject:      "ParseOutput",
				Object:       "buildAnalysisIR",
				AnchorSymbol: "buildAnalysisIR",
			},
			{
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    978,
				AnchorKind:   AnchorCondition,
				AnchorSymbol: "buildAnalysisIR",
				Snippet:      "if ctx == nil || ctx.Mutable == nil {",
			},
			{
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    1011,
				AnchorKind:   AnchorAssignment,
				AnchorSymbol: "reconcileEnumerationBoundaryScope",
				Snippet:      "if resolved, reason := reconcileEnumerationBoundaryScope(rm, graph); reason != \"\" {",
			},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Errors: []LogError{{Type: "panic: runtime error"}}},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}

	for _, lane := range got.Lanes {
		if lane.Kind != SupportLaneNearestMechanism {
			continue
		}
		joined := make([]string, 0, len(lane.Entries))
		for _, entry := range lane.Entries {
			joined = append(joined, entry.Text)
		}
		if strings.Contains(strings.Join(joined, "\n"), "reconcileEnumerationBoundaryScope") {
			t.Fatalf("control-header assignment should not enter nearest_mechanism, got %+v", lane.Entries)
		}
	}
}

func TestBuildAnswerSupportPlan_RootCauseTraceKeepsReturnFactsOutOfNearestMechanism(t *testing.T) {
	plan := &AnswerSurfacePlan{
		DriftBoundedSurfaceItems: []EvidenceItem{
			{
				Kind:         EvidenceRelationship,
				Source:       "internal/agent/analyzer.go",
				LineStart:    743,
				AnchorKind:   AnchorCall,
				Subject:      "ParseOutput",
				Object:       "buildAnalysisIR",
				AnchorSymbol: "buildAnalysisIR",
			},
			{
				Kind:         EvidenceDirect,
				Source:       "internal/agent/analyzer.go",
				LineStart:    983,
				AnchorKind:   AnchorReturn,
				AnchorSymbol: "buildAnalysisIR",
				Snippet:      `return nil, errors.New("analyzer: emit_analysis was not called")`,
			},
		},
		// Mark the plan as runtime-artifact-sourced so
		// AnswerSurfacePlan.IsCrashSourcedRootCause returns true and
		// the AnchorReturn exclusion stays active. Real-world plans
		// produced by BuildAnswerSurfacePlanForBusContext always
		// populate this slice from the LogBundle's resolved frames;
		// the manual fixture has to set it explicitly.
		LogObservedAnchors: []LogSourceDriftAnchor{{File: "internal/agent/analyzer.go", ObservedLine: 983}},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Errors: []LogError{{Type: "panic: runtime error"}}},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	for _, lane := range got.Lanes {
		if lane.Kind != SupportLaneNearestMechanism {
			continue
		}
		joined := make([]string, 0, len(lane.Entries))
		for _, entry := range lane.Entries {
			joined = append(joined, entry.Text)
		}
		if strings.Contains(strings.Join(joined, "\n"), "emit_analysis was not called") {
			t.Fatalf("return facts should not be promoted into nearest_mechanism, got %+v", lane.Entries)
		}
	}
}

// TestRootCauseMechanismItemEligible_ReturnGatedByCrashSource confirms
// AnchorReturn is excluded from the nearest_mechanism lane only when
// the surface plan is sourced from a runtime artifact (panic/exception
// /perf jank). For non-crash root-cause questions ("Foo returns nil —
// why?") the early-return statement legitimately IS the mechanism and
// must remain eligible.
func TestRootCauseMechanismItemEligible_ReturnGatedByCrashSource(t *testing.T) {
	returnItem := EvidenceItem{
		Kind:       EvidenceDirect,
		Source:     "internal/foo/loader.go",
		LineStart:  88,
		AnchorKind: AnchorReturn,
		Snippet:    "return defaultValue",
	}

	t.Run("crash-sourced plan excludes AnchorReturn", func(t *testing.T) {
		plan := &AnswerSurfacePlan{
			LogObservedAnchors: []LogSourceDriftAnchor{
				{File: "internal/foo/loader.go", ObservedLine: 88},
			},
		}
		if rootCauseMechanismItemEligible(plan, returnItem) {
			t.Error("crash-sourced plan must exclude AnchorReturn from mechanism lane")
		}
	})

	t.Run("non-crash-sourced plan keeps AnchorReturn eligible", func(t *testing.T) {
		plan := &AnswerSurfacePlan{
			// No LogObservedAnchors, no ExternalObservationSeeds —
			// "Foo returns nil — why?" style code-only debug.
		}
		if !rootCauseMechanismItemEligible(plan, returnItem) {
			t.Error("non-crash root-cause plan must keep AnchorReturn eligible — early-return IS the mechanism")
		}
	})

	t.Run("non-crash control-header companion still excluded", func(t *testing.T) {
		plan := &AnswerSurfacePlan{}
		controlHeader := EvidenceItem{
			Kind:       EvidenceDirect,
			Source:     "internal/foo/loader.go",
			LineStart:  90,
			AnchorKind: AnchorReturn,
			Snippet:    "if v == nil { return nil }",
		}
		if rootCauseMechanismItemEligible(plan, controlHeader) {
			t.Error("control-header companion (snippet starts with 'if ') must be filtered even on non-crash plans")
		}
	})
}

// TestIsCrashSourcedRootCause confirms the helper's structural
// signal sources (ExternalObservationSeeds + LogObservedAnchors).
func TestIsCrashSourcedRootCause(t *testing.T) {
	cases := []struct {
		name string
		plan *AnswerSurfacePlan
		want bool
	}{
		{name: "nil plan", plan: nil, want: false},
		{name: "empty plan", plan: &AnswerSurfacePlan{}, want: false},
		{
			name: "log observed anchors set",
			plan: &AnswerSurfacePlan{LogObservedAnchors: []LogSourceDriftAnchor{{File: "x.go", ObservedLine: 1}}},
			want: true,
		},
		{
			name: "external observation seeds set",
			plan: &AnswerSurfacePlan{ExternalObservationSeeds: []ExternalObservationSeed{{Kind: "log_frame", File: "x.go", Line: 1}}},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.plan.IsCrashSourcedRootCause(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
