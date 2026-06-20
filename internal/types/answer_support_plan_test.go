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
	if !strings.Contains(observedGuidance, "variable/parameter") ||
		!strings.Contains(observedGuidance, "possible upstream investigation directions") {
		t.Fatalf("observed artifact guidance should distinguish direct observations from inferred upstream provenance: %q", observedGuidance)
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
			{Kind: "frame", Raw: "at demo.bridge.ohSum (src/bridge/Bridge.cj:18)", Func: "demo.bridge.ohSum", File: "src/bridge/Bridge.cj", Line: 18},
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
	if len(observed.Entries) < 2 || !strings.Contains(observed.Entries[1].Detail, "src/bridge/Bridge.cj:18") {
		t.Fatalf("observed frame should preserve raw artifact detail separately from the principal label, got %+v", observed.Entries)
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
			{Kind: "error_chain", Raw: `runtime cause chain: upstream panic("index out of bounds: index=5, size=3") is wrapped / re-raised as top-level Error("Cangjie native call failed: index out of bounds")`},
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
		`runtime cause chain: upstream panic("index out of bounds: index=5, size=3") is wrapped / re-raised as top-level Error("Cangjie native call failed: index out of bounds")`,
		`structured runtime error message: Cangjie native call failed: index out of bounds`,
		`runtime artifact identifies error head stack frame "NativeBridge.invokeOhSum"`,
		`runtime artifact identifies error head stack frame "demo.bridge.ohSum"`,
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

func TestBuildAnswerSupportPlan_ObservedArtifactRuntimeMessageKeepsVisibleQuotes(t *testing.T) {
	plan := &AnswerSurfacePlan{
		RuntimeGroundingDisposition: &RuntimeGroundingDisposition{
			Source:         RuntimeGroundingSystemDetected,
			Reason:         EvidenceFloorWaiverExternalLog,
			CitationPolicy: RuntimeGroundingCitationRuntimeObservation,
		},
		ExternalObservationSeeds: []ExternalObservationSeed{
			{Kind: "error_message", Raw: `Cannot invoke "User.getId()" because "user" is null`},
		},
	}
	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Errors: []LogError{{Type: "java.lang.NullPointerException"}}},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	var joined string
	for _, lane := range got.Lanes {
		if lane.Kind != SupportLaneObservedArtifact {
			continue
		}
		for _, entry := range lane.Entries {
			joined += entry.Text + "\n"
		}
	}
	if !strings.Contains(joined, `structured runtime error message: Cannot invoke "User.getId()" because "user" is null`) {
		t.Fatalf("runtime message should render as visible user text, got:\n%s", joined)
	}
	if strings.Contains(joined, `\"User.getId()\"`) {
		t.Fatalf("support lane must not teach serialized quote syntax, got:\n%s", joined)
	}
}

func TestBuildAnswerSupportPlan_ObservedArtifactPrioritizesLineObservationOverGenericSignals(t *testing.T) {
	plan := &AnswerSurfacePlan{
		ExternalObservationSeeds: []ExternalObservationSeed{
			{Kind: "signal", Raw: "timeout"},
			{Kind: "signal", Raw: "performance"},
			{Kind: "log_observation", Raw: "log_line=3 first_byte_timeout exceeded after 40s", Func: "LLM 首字节响应超时", Line: 3},
			{Kind: "log_observation", Raw: "log_line=4 ⟳ 1/4 模型响应出错,正在重新理解问题", Func: "render 层发起第 1/4 次重试", Line: 4},
		},
	}
	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Observations: []LogObservation{{Kind: LogObservationPerformance, Summary: "first byte timeout", LineStart: 3}}},
		SubTopics: []SubTopic{
			{Summary: "line-specific timeout observation", Entities: []string{"first_byte_timeout"}},
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
	var joined string
	for _, entry := range observed.Entries {
		joined += entry.Text + "\n"
	}
	for _, want := range []string{
		"log_line=3 first_byte_timeout exceeded after 40s",
		"log_line=4",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("line-bearing observations should win the budget over generic signals; missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, `structured runtime signal "timeout"`) ||
		strings.Contains(joined, `structured runtime signal "performance"`) {
		t.Fatalf("generic signal seeds should not displace exact artifact lines in a focused support lane:\n%s", joined)
	}
}

func TestBuildAnswerSupportPlan_ExternalOnlyObservedArtifactKeepsRepresentativeFramesForErrorTargets(t *testing.T) {
	plan := &AnswerSurfacePlan{
		RuntimeGroundingDisposition: &RuntimeGroundingDisposition{
			Source:         RuntimeGroundingSystemDetected,
			Reason:         EvidenceFloorWaiverExternalLog,
			Rationale:      "attached Python traceback frames did not resolve to current repo files",
			CitationPolicy: RuntimeGroundingCitationRuntimeObservation,
		},
		ExternalObservationSeeds: []ExternalObservationSeed{
			{Kind: "error_chain", Raw: `runtime cause chain: upstream KeyError("KeyError: 'database'") is wrapped / re-raised as top-level RuntimeError("config unavailable")`},
			{Kind: "error_message", Raw: "config unavailable"},
			{Kind: "error_message", Raw: "'database'"},
			{Kind: "log_frame", Role: "error_head_frame", Lang: "python", Func: "load", File: "/app/src/config.py", Line: 42},
			{Kind: "log_frame", Role: "caller_frame", Lang: "python", Func: "safe_load", File: "/usr/lib/python3.11/yaml/__init__.py", Line: 125},
			{Kind: "log_frame", Role: "error_head_frame", Lang: "python", Func: "load_config", File: "/app/src/loader.py", Line: 25},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Meta: LogMeta{Lang: "python"}, Errors: []LogError{{Type: "RuntimeError"}}},
		SubTopics: []SubTopic{
			{Summary: "outer exception", Entities: []string{"RuntimeError"}},
			{Summary: "inner exception", Entities: []string{"KeyError"}},
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
		`runtime cause chain: upstream KeyError("KeyError: 'database'") is wrapped / re-raised as top-level RuntimeError("config unavailable")`,
		`structured runtime error message: config unavailable`,
		`traceback frame "load"`,
		`traceback frame "load_config"`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("observed lane missing %q:\n%s", want, joined)
		}
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

func TestBuildAnswerSupportPlan_CallChainExcludesConcreteValueSideEvidence(t *testing.T) {
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{
			{
				Kind:            EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1818,
				AnchorKind:      AnchorCall,
				Subject:         "buildAnalysisIR",
				Object:          "compiler.Compile",
				GroundingStatus: GroundingGrounded,
			},
			{
				Kind:            EvidenceConcrete,
				Source:          "internal/agent/answer_document_evaluator.go",
				LineStart:       2102,
				AnchorKind:      AnchorCall,
				Subject:         "renderAnswerDocDiagramContract",
				Object:          `"## Diagram Contract\n\n"`,
				GroundingStatus: GroundingGrounded,
			},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent: IntentTrace,
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"buildAnalysisIR", "gate.Run"},
		},
	}, plan)
	if got == nil {
		t.Fatal("expected call-chain support plan")
	}
	var joined string
	for _, lane := range got.Lanes {
		if lane.Kind != SupportLaneCurrentCodePath {
			continue
		}
		for _, entry := range lane.Entries {
			joined += entry.Text + "\n"
		}
	}
	if !strings.Contains(joined, "compiler.Compile") {
		t.Fatalf("expected principal call edge to remain, got:\n%s", joined)
	}
	if strings.Contains(joined, "renderAnswerDocDiagramContract") || strings.Contains(joined, "Diagram Contract") {
		t.Fatalf("concrete-value side evidence must not enter principal call-chain lane:\n%s", joined)
	}
}

func TestBuildAnswerSupportPlan_CallChainSortsSameFileEvidenceAndCarriesSummaryDetail(t *testing.T) {
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{
			{
				Kind:            EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       30,
				AnchorKind:      AnchorCall,
				AnchorSymbol:    "third",
				Subject:         "buildAnalysisIR",
				Object:          "third",
				Summary:         "third records final gate preparation",
				GroundingStatus: GroundingGrounded,
			},
			{
				Kind:            EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       10,
				AnchorKind:      AnchorCall,
				AnchorSymbol:    "first",
				Subject:         "buildAnalysisIR",
				Object:          "first",
				Summary:         "first prepares the normalized request",
				GroundingStatus: GroundingGrounded,
			},
			{
				Kind:            EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       20,
				AnchorKind:      AnchorCall,
				AnchorSymbol:    "second",
				Subject:         "buildAnalysisIR",
				Object:          "second",
				Summary:         "second compiles the task graph",
				GroundingStatus: GroundingGrounded,
			},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentTrace}, plan)
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
	if len(path.Entries) != 3 {
		t.Fatalf("entries = %d, want 3: %+v", len(path.Entries), path.Entries)
	}
	if gotOrder := []string{path.Entries[0].Text, path.Entries[1].Text, path.Entries[2].Text}; !strings.Contains(gotOrder[0], "first") || !strings.Contains(gotOrder[1], "second") || !strings.Contains(gotOrder[2], "third") {
		t.Fatalf("same-file call-chain evidence should be line ordered, got %+v", gotOrder)
	}
	if !strings.Contains(path.Entries[0].Detail, "normalized request") || !strings.Contains(path.Entries[1].Detail, "task graph") {
		t.Fatalf("model-authored summaries should survive as per-hop detail, got %+v", path.Entries)
	}
}

func TestBuildAnswerSupportPlan_FacetFamiliesCompilePrincipalEvidenceLane(t *testing.T) {
	tests := []struct {
		name           string
		rm             RequestModel
		family         QuestionFamily
		facet          AnswerFacetKind
		item           EvidenceItem
		allowed        string
		wantText       string
		wantGuidance   string
		wantDetailTerm string
	}{
		{
			name:   "config precedence",
			rm:     RequestModel{Intent: IntentConfigQuery},
			family: QFConfigPrecedence,
			facet:  FacetConfigPrecedenceRole,
			item: EvidenceItem{
				ID:              "config-role",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "internal/config/runtime.go",
				LineStart:       112,
				AnchorKind:      AnchorAssignment,
				AnchorSymbol:    "PipelineMaxSteps",
				Snippet:         "PipelineMaxSteps: 50",
				Summary:         "default runtime budget is overridden by config and then by CLI",
				GroundingStatus: GroundingGrounded,
			},
			allowed:        "summary,scalar,table,ordered_list",
			wantText:       "PipelineMaxSteps: 50",
			wantGuidance:   "real default/config/CLI/runtime layer anchors",
			wantDetailTerm: "overridden by config",
		},
		{
			name:   "role lookup",
			rm:     RequestModel{AnswerSubject: AnswerSubject{Kind: SubjectFunctionName, Confidence: 0.9}},
			family: QFRoleLookup,
			facet:  FacetResolvedLiteralOrSymbol,
			item: EvidenceItem{
				ID:              "role-result",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "internal/orchestrator/topology.go",
				LineStart:       31,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "runReadSchedulerLoop",
				Summary:         "runReadSchedulerLoop is the read-mode scheduler entrypoint",
				GroundingStatus: GroundingGrounded,
			},
			allowed:        "summary,scalar,section",
			wantText:       "runReadSchedulerLoop",
			wantGuidance:   "do not add uncited helper names",
			wantDetailTerm: "scheduler entrypoint",
		},
		{
			name:   "enumeration",
			rm:     RequestModel{Intent: IntentEnumerate},
			family: QFEnumeration,
			facet:  FacetEnumerationItem,
			item: EvidenceItem{
				ID:              "enum-item",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "internal/types/facet_plan.go",
				LineStart:       54,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "QFCallChain",
				Summary:         "QFCallChain is one enumerated question family",
				GroundingStatus: GroundingGrounded,
			},
			allowed:        "summary,ordered_list,table,bullet_list,section",
			wantText:       "QFCallChain",
			wantGuidance:   "do not invent missing members",
			wantDetailTerm: "enumerated question family",
		},
		{
			name:   "architecture",
			rm:     RequestModel{Intent: IntentExplain, Scenario: ScenarioArchitectureExplain},
			family: QFArchitecture,
			facet:  FacetComponentRelation,
			item: EvidenceItem{
				ID:              "architecture-edge",
				Kind:            EvidenceRelationship,
				Scope:           ScopeLine,
				Source:          "internal/orchestrator/topology.go",
				LineStart:       62,
				AnchorKind:      AnchorCall,
				Subject:         "scheduler",
				Object:          "explorer",
				Summary:         "scheduler delegates repository investigation to explorer",
				GroundingStatus: GroundingGrounded,
			},
			allowed:        "summary,section,ordered_list,bullet_list,diagram",
			wantText:       "scheduler calls explorer",
			wantGuidance:   "unrelated helper calls",
			wantDetailTerm: "repository investigation",
		},
		{
			name: "comparison",
			rm: RequestModel{Buckets: []QuestionBucket{
				{Label: "read mode", Index: 1},
				{Label: "write mode", Index: 2},
			}},
			family: QFComparison,
			facet:  FacetCurrentCodePath,
			item: EvidenceItem{
				ID:              "comparison-anchor",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "internal/orchestrator/topology.go",
				LineStart:       88,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "runWriteControllerWorkflow",
				Summary:         "runWriteControllerWorkflow is the write-mode controller entry point",
				GroundingStatus: GroundingGrounded,
			},
			allowed:        "summary,section,table,ordered_list,bullet_list",
			wantText:       "runWriteControllerWorkflow",
			wantGuidance:   "bucket labels",
			wantDetailTerm: "write-mode controller",
		},
		{
			name:   "generic",
			rm:     RequestModel{Intent: IntentExplain, Scenario: ScenarioGeneric},
			family: QFGeneric,
			facet:  FacetCurrentCodePath,
			item: EvidenceItem{
				ID:              "generic-anchor",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1818,
				AnchorKind:      AnchorCall,
				Subject:         "analyzer",
				Object:          "compiler.Compile",
				Summary:         "compiler.Compile builds the initial TaskGraph and EvidencePlan",
				GroundingStatus: GroundingGrounded,
			},
			allowed:        "summary,section,ordered_list,bullet_list,diagram",
			wantText:       "analyzer calls compiler.Compile",
			wantGuidance:   "do not add uncited helper names",
			wantDetailTerm: "TaskGraph and EvidencePlan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &AnswerSurfacePlan{
				SurfaceEvidence: []EvidenceItem{tt.item},
				FacetCoverage: &FacetCoverageContract{
					Family: tt.family,
					Required: []FacetRequirement{{
						Kind:            tt.facet,
						SourceCandidate: []string{tt.item.ID},
					}},
				},
			}
			got := BuildAnswerSupportPlan(tt.rm, plan)
			if got == nil {
				t.Fatal("expected support plan")
			}
			if got.Family != tt.family {
				t.Fatalf("family = %q, want %q", got.Family, tt.family)
			}
			lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
			if lane == nil {
				t.Fatalf("missing principal evidence lane: %+v", got.Lanes)
			}
			if got, want := strings.Join(lane.AllowedBlocks, ","), tt.allowed; got != want {
				t.Fatalf("allowed blocks = %q, want %q", got, want)
			}
			if !strings.Contains(lane.Guidance, tt.wantGuidance) {
				t.Fatalf("guidance missing %q: %q", tt.wantGuidance, lane.Guidance)
			}
			if len(lane.Entries) != 1 {
				t.Fatalf("entries = %+v, want exactly one source-candidate entry", lane.Entries)
			}
			if !strings.Contains(lane.Entries[0].Text, tt.wantText) {
				t.Fatalf("entry text missing %q: %+v", tt.wantText, lane.Entries[0])
			}
			if !strings.Contains(lane.Entries[0].Detail, tt.wantDetailTerm) {
				t.Fatalf("entry detail should preserve model-authored enrichment %q: %+v", tt.wantDetailTerm, lane.Entries[0])
			}
			if !strings.Contains(lane.Entries[0].Detail, "typed_surface: claim_form=") {
				t.Fatalf("entry detail should carry typed surface metadata for finalizer enrichment: %+v", lane.Entries[0])
			}
		})
	}
}

func TestPrincipalEvidenceSupportLane_AllowsDecisionForErrorGranularity(t *testing.T) {
	item := EvidenceItem{
		ID:              "decision-anchor",
		Kind:            EvidenceConditional,
		Scope:           ScopeLine,
		Source:          "internal/tool/emit_evidence.go",
		LineStart:       531,
		AnchorKind:      AnchorCondition,
		AnchorSymbol:    "Execute",
		Summary:         "per-item validation failure is recorded and the loop continues",
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{item},
		FacetCoverage: &FacetCoverageContract{
			Family: QFGeneric,
			Required: []FacetRequirement{{
				Kind:            FacetCurrentCodePath,
				SourceCandidate: []string{item.ID},
			}},
		},
	}
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioGeneric,
		ErrorGranularityProfile: &ErrorGranularityProfile{
			IsGranularityQuestion: true,
			Confidence:            0.9,
		},
	}
	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil {
		t.Fatalf("missing principal evidence lane: %+v", got)
	}
	for _, kind := range lane.AllowedBlocks {
		if kind == string(BlockDecision) {
			return
		}
	}
	t.Fatalf("error-granularity decision blocks should be allowed to cite principal evidence, got %v", lane.AllowedBlocks)
}

func TestBuildAnswerSupportPlan_EnumerationMechanismFallbackUsesEnrichmentPolicy(t *testing.T) {
	item := EvidenceItem{
		ID:              "template-budget",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/analysis/compiler/templates.go",
		LineStart:       354,
		AnchorKind:      AnchorInitializer,
		AnchorSymbol:    "RetryBudget",
		Subject:         "templateRootCause",
		Object:          "TmplRetryBudgetVeryHigh",
		Summary:         "templateRootCause sets the retry budget for root-cause answers",
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{item},
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: []string{item.ID},
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent:        IntentEnumerate,
		Scenario:      ScenarioArchitectureExplain,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	if got.PrincipalMemberCoverage != PrincipalMemberCoveragePolicyEnrichmentOnly {
		t.Fatalf("mechanism-shaped enumerate fallback policy = %q, want enrichment_only", got.PrincipalMemberCoverage)
	}
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("expected principal evidence lane entry for enrichment, got %+v", got)
	}
	if !strings.Contains(lane.Guidance, "not a one-row-per-entry member slate") {
		t.Fatalf("guidance should preserve enrichment-only boundary, got %q", lane.Guidance)
	}
	if obligations := PrincipalSupportMemberObligations(got); len(obligations) != 0 {
		t.Fatalf("enrichment-only enumeration fallback must not force one member per support entry, got %+v", obligations)
	}
}

func TestBuildAnswerSupportPlan_CategoryEnumerationKeepsRequiredMemberPolicy(t *testing.T) {
	item := EvidenceItem{
		ID:              "import-edge",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "entry/src/main/ets/pages/Index.ets",
		LineStart:       3,
		AnchorKind:      AnchorImport,
		Subject:         "Index.ets",
		Object:          "@kit.ArkUI",
		SurfaceTerms:    []string{"Index.ets", "@kit.ArkUI"},
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{item},
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: []string{item.ID},
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqEnumeration)},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	if got.PrincipalMemberCoverage != PrincipalMemberCoveragePolicyRequired {
		t.Fatalf("category enumeration policy = %q, want required", got.PrincipalMemberCoverage)
	}
	obligations := PrincipalSupportMemberObligations(got)
	if len(obligations) != 1 {
		t.Fatalf("set-valued enumeration should require one cited principal row, got %+v", obligations)
	}
	if !strings.Contains(obligations[0].Location, "entry/src/main/ets/pages/index.ets:3") {
		t.Fatalf("obligation should preserve typed citation location, got %+v", obligations[0])
	}
}

func TestBuildAnswerSupportPlan_FacetPrincipalLanePreservesDisplaySurfaceMetadata(t *testing.T) {
	item := EvidenceItem{
		ID:              "import-edge",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "entry/src/main/ets/pages/Index.ets",
		LineStart:       3,
		AnchorKind:      AnchorImport,
		Subject:         "Index.ets",
		Object:          "@kit.ArkUI",
		Producer:        "explorer.emit_evidence",
		SurfaceTerms:    []string{"@kit.ArkUI"},
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{item},
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: []string{item.ID},
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentEnumerate}, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("expected one principal import-edge entry, got %+v", got)
	}
	detail := lane.Entries[0].Detail
	for _, want := range []string{
		"claim_form=import_edge",
		"label_surface=display_label",
		"anchor_kind=import",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("display-surface metadata missing %q from detail %q", want, detail)
		}
	}
	if !strings.Contains(lane.Guidance, "preserve each entry's own snippet/operator") {
		t.Fatalf("principal lane guidance should protect per-entry visible syntax, got %q", lane.Guidance)
	}
}

func TestBuildAnswerSupportPlan_FacetPrincipalLaneUsesOnlySourceCandidates(t *testing.T) {
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "requested-item",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "internal/types/facet_plan.go",
				LineStart:       54,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "QFCallChain",
				Summary:         "QFCallChain is one requested family",
				GroundingStatus: GroundingGrounded,
			},
			{
				ID:              "nearby-helper",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "internal/types/facet_plan.go",
				LineStart:       565,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "ResolveQuestionFamily",
				Summary:         "ResolveQuestionFamily is nearby context but not an enumeration item",
				GroundingStatus: GroundingGrounded,
			},
		},
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: []string{"requested-item"},
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentEnumerate}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil {
		t.Fatalf("missing principal evidence lane: %+v", got.Lanes)
	}
	joined := answerSupportLaneText(lane)
	if !strings.Contains(joined, "QFCallChain") {
		t.Fatalf("requested source candidate missing:\n%s", joined)
	}
	if strings.Contains(joined, "ResolveQuestionFamily") {
		t.Fatalf("non-candidate nearby helper must not enter principal lane:\n%s", joined)
	}
}

func TestBuildAnswerSupportPlan_EnumerationBackboneSeparatesPrincipalFromSupportContext(t *testing.T) {
	principal := EvidenceItem{
		ID:              "agent-explorer",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/types/enums.go",
		LineStart:       117,
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "AgentExplorer",
		Subject:         "AgentExplorer",
		Object:          "explorer",
		Snippet:         `AgentExplorer AgentName = "explorer"`,
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	mechanism := EvidenceItem{
		ID:              "registration-helper",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/agent/subagent.go",
		LineStart:       63,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "RegisterDefaultSubAgents",
		Subject:         "RegisterDefaultSubAgents",
		Snippet:         "func RegisterDefaultSubAgents(...)",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	decoder := EvidenceItem{
		ID:              "proposal-decoder",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/orchestrator/orchestrator.go",
		LineStart:       6204,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "extractSubAgentProposal",
		Subject:         "extractSubAgentProposal",
		Snippet:         "func extractSubAgentProposal(...)",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		StepBackbone: []StepSurfaceAnchor{{
			Name: "AgentExplorer",
			File: "internal/types/enums.go",
			Line: 117,
		}},
		StepBackboneCompleteness: CompletenessComplete,
		SurfaceEvidence:          []EvidenceItem{principal, mechanism, decoder},
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: []string{principal.ID, mechanism.ID, decoder.ID},
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentEnumerate}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	principalLane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if principalLane == nil {
		t.Fatalf("missing principal lane: %+v", got.Lanes)
	}
	principalText := answerSupportLaneText(principalLane)
	if !strings.Contains(principalText, "AgentExplorer") {
		t.Fatalf("backbone-matched principal evidence missing:\n%s", principalText)
	}
	for _, helper := range []string{"RegisterDefaultSubAgents", "extractSubAgentProposal"} {
		if strings.Contains(principalText, helper) {
			t.Fatalf("support helper %q must not become a principal enumeration member:\n%s", helper, principalText)
		}
	}
	contextLane := answerSupportLaneByKind(got, SupportLaneNearestMechanism)
	if contextLane == nil {
		t.Fatalf("expected support context lane for non-principal proof helpers: %+v", got.Lanes)
	}
	contextText := answerSupportLaneText(contextLane)
	for _, helper := range []string{"RegisterDefaultSubAgents", "extractSubAgentProposal"} {
		if !strings.Contains(contextText, helper) {
			t.Fatalf("support helper %q should remain available for explanation context:\n%s", helper, contextText)
		}
	}
	curated := PrincipalSupportEvidenceItemsForFamily(QFEnumeration, plan)
	if len(curated) != 1 || curated[0].ID != principal.ID {
		t.Fatalf("principal evidence helper should return only the backbone principal, got %+v", curated)
	}
}

func TestBuildAnswerSupportPlan_EnumerationOrientsImportEdgesToSourceMembers(t *testing.T) {
	const targetPkg = "github.com/hanchaoqun/codrax/internal/tool/ground"
	importA := EvidenceItem{
		ID:              "emit-evidence-import",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/tool/emit_evidence.go",
		LineStart:       18,
		AnchorKind:      AnchorImport,
		Subject:         "EmitEvidence",
		Object:          targetPkg,
		SurfaceTerms:    []string{targetPkg},
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	importB := EvidenceItem{
		ID:              "explorer-import",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/agent/explorer.go",
		LineStart:       22,
		AnchorKind:      AnchorImport,
		Subject:         "explorer",
		Object:          targetPkg,
		SurfaceTerms:    []string{targetPkg},
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	helper := EvidenceItem{
		ID:              "same-file-helper",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/tool/emit_investigation_complete.go",
		LineStart:       2250,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "multiPathToolHistory",
		Subject:         "multiPathToolHistory",
		Snippet:         "func multiPathToolHistory(...)",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{importA, importB, helper},
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: []string{importA.ID, importB.ID, helper.ID},
			}},
		},
	}

	ApplyPrincipalEvidenceStepBackbone(plan, &AnalysisIR{RequestModel: RequestModel{Intent: IntentEnumerate}})
	if len(plan.StepBackbone) != 2 {
		t.Fatalf("source-location import edges should build a two-member backbone, got %+v", plan.StepBackbone)
	}
	for _, anchor := range plan.StepBackbone {
		if strings.Contains(anchor.Name, "multiPathToolHistory") || strings.Contains(anchor.Name, targetPkg) {
			t.Fatalf("principal backbone should use importing source files, got %+v", plan.StepBackbone)
		}
		if !strings.HasSuffix(anchor.Name, ".go") {
			t.Fatalf("source-location member should be a source path label, got %+v", plan.StepBackbone)
		}
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentEnumerate}, plan)
	principalLane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if principalLane == nil || len(principalLane.Entries) != 2 {
		t.Fatalf("expected only import-edge principal entries, got %+v", got)
	}
	for _, entry := range principalLane.Entries {
		if entry.MemberSurface != PrincipalMemberSurfaceSourceLocation {
			t.Fatalf("reverse import-edge member should be source_location, got %+v", entry)
		}
		if strings.Contains(entry.Text, "multiPathToolHistory") {
			t.Fatalf("context helper must not be promoted into the principal lane: %+v", principalLane.Entries)
		}
	}
	contextLane := answerSupportLaneByKind(got, SupportLaneNearestMechanism)
	if contextLane == nil || !strings.Contains(answerSupportLaneText(contextLane), "multiPathToolHistory") {
		t.Fatalf("outlier helper should remain available as support context, got %+v", got)
	}

	obligations := PrincipalSupportMemberObligations(got)
	if len(obligations) != 2 {
		t.Fatalf("expected one obligation per importing source file, got %+v", obligations)
	}
	for _, ob := range obligations {
		if !strings.HasSuffix(ob.Label, ".go") || strings.Contains(ob.Label, "->") || strings.Contains(ob.Label, targetPkg) {
			t.Fatalf("source-location obligation label should be the importing file path, got %+v", ob)
		}
	}
}

func TestBuildAnswerSupportPlan_ChangeImpactKeepsHeterogeneousAffectedSites(t *testing.T) {
	def := EvidenceItem{
		ID:              "definition",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       1235,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "CitationReq",
		Subject:         "CitationReq",
		Summary:         "CitationReq defines the changed field",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	assign := EvidenceItem{
		ID:              "assignment",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1903,
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		Subject:         "CitationReq.Required",
		Summary:         "analyzer assigns CitationReq.Required",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	guard := EvidenceItem{
		ID:              "guard",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/analysis/gate/gate.go",
		LineStart:       414,
		AnchorKind:      AnchorCondition,
		AnchorSymbol:    "CitationReq.Required",
		Subject:         "CitationReq.Required",
		Condition:       "req.Required && req.MinCitations > 0",
		Summary:         "gate reads CitationReq.Required in a guard",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	call := EvidenceItem{
		ID:              "call",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/context/builder.go",
		LineStart:       288,
		AnchorKind:      AnchorCall,
		AnchorSymbol:    "CitationReq.Required",
		Subject:         "CitationReq.Required",
		Object:          "BuildContext",
		Summary:         "context builder consumes CitationReq.Required",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	unrelatedRequired := EvidenceItem{
		ID:              "required-block",
		Kind:            EvidenceConditional,
		Scope:           ScopeLine,
		Source:          "internal/tool/answer_document_pre_emit_check.go",
		LineStart:       1115,
		AnchorKind:      AnchorCondition,
		AnchorSymbol:    "Required",
		Object:          "req.Required",
		Condition:       "!req.Required",
		Summary:         "Required belongs to RequiredBlock, not CitationReq.Required",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	mechanismComment := EvidenceItem{
		ID:              "comment-only",
		Kind:            EvidenceMechanism,
		Scope:           ScopeLine,
		Source:          "internal/orchestrator/orchestrator.go",
		LineStart:       6327,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "CitationReq",
		Subject:         "CitationReq",
		Summary:         "comment describes CitationReq.Required fallback behavior",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	evidence := []EvidenceItem{def, assign, guard, call, unrelatedRequired, mechanismComment}
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsRelationalLookup:    true,
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqEnumeration)},
		ChangeImpactProfile: &ChangeImpactProfile{
			IsChangeImpact:  true,
			Target:          "CitationReq.Required",
			TargetKind:      SubjectStructField,
			Scope:           ImpactScopeProduction,
			RequestedOutput: ImpactOutputFiles,
			AffectedSiteKinds: []ImpactAffectedSiteKind{
				ImpactSiteDefinition,
				ImpactSiteAssignment,
				ImpactSiteGuard,
				ImpactSiteCall,
			},
			Confidence: 0.92,
		},
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence:     evidence,
		ChangeImpactProfile: rm.ChangeImpactProfile,
		FacetCoverage:       CompileFacetCoverage(rm, evidence),
	}

	got := BuildAnswerSupportPlan(rm, plan)
	principalLane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if principalLane == nil {
		t.Fatalf("missing principal lane: %+v", got)
	}
	if len(principalLane.Entries) != 4 {
		t.Fatalf("change-impact principal lane should keep heterogeneous affected sites, got %+v", principalLane.Entries)
	}
	var locations []string
	for _, entry := range principalLane.Entries {
		locations = append(locations, entry.Location)
	}
	locationText := strings.Join(locations, "\n")
	for _, want := range []string{
		"analysis_ir.go",
		"analyzer.go",
		"gate.go",
		"builder.go",
	} {
		if !strings.Contains(locationText, want) {
			t.Fatalf("principal lane missing affected site %q:\n%s", want, locationText)
		}
	}
	if strings.Contains(locationText, "answer_document_pre_emit_check.go") {
		t.Fatalf("owner-qualified change-impact target should not promote a different Required field:\n%s", locationText)
	}
	if strings.Contains(locationText, "orchestrator.go") {
		t.Fatalf("mechanism/comment-only evidence should remain support context for production code-site impact answers:\n%s", locationText)
	}
	if !strings.Contains(principalLane.Guidance, "Direct assignments are only one affected-site role") {
		t.Fatalf("impact guidance should warn against assignment-only narrowing: %q", principalLane.Guidance)
	}
	contextLane := answerSupportLaneByKind(got, SupportLaneNearestMechanism)
	if contextLane == nil || !supportLaneHasSource(contextLane, "internal/orchestrator/orchestrator.go") {
		t.Fatalf("filtered mechanism evidence should remain available as support context, got %+v", got)
	}

	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/agent/analyzer.go", Line: 1903}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "internal/agent/analyzer.go",
				Text:        "direct assignment site for CitationReq.Required",
				CitationRef: 0,
			}},
		}},
	}
	narrowing := ChangeImpactPrincipalNarrowing(doc, got)
	if narrowing == nil {
		t.Fatalf("assignment-only rendered answer should be diagnosed as change-impact narrowing")
	}
	if len(narrowing.MissingForms) == 0 {
		t.Fatalf("narrowing diagnostic should name non-assignment missing forms: %+v", narrowing)
	}
}

func TestBuildAnswerSupportPlan_ChangeImpactSimpleTargetFiltersSameFamilyEvidence(t *testing.T) {
	profile := &ChangeImpactProfile{
		IsChangeImpact:  true,
		Target:          "ShapeValue",
		TargetKind:      SubjectEnumValue,
		Scope:           ImpactScopeProduction,
		RequestedOutput: ImpactOutputFiles,
		AffectedSiteKinds: []ImpactAffectedSiteKind{
			ImpactSiteDefinition,
			ImpactSiteRead,
		},
		Confidence: 0.92,
	}
	exact := EvidenceItem{
		ID:              "shapevalue-definition",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       1235,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "ShapeValue",
		Subject:         "ShapeValue",
		Snippet:         "ShapeValue AnswerShape = \"value\"",
		Summary:         "ShapeValue defines the requested exact target.",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	sameFamily := EvidenceItem{
		ID:              "shape-step-list",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/types/answer_surface_plan.go",
		LineStart:       295,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "ShapeStepList",
		Subject:         "ShapeStepList",
		Snippet:         "case ShapeStepList:",
		Summary:         "ShapeStepList is a nearby answer-shape value, not ShapeValue.",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	evidence := []EvidenceItem{sameFamily, exact}
	rm := RequestModel{
		Intent:              IntentEnumerate,
		AnalyzerHints:       AnalyzerHints{Kind: string(ReqEnumeration)},
		ChangeImpactProfile: profile,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence:     evidence,
		ChangeImpactProfile: profile,
		FacetCoverage:       CompileFacetCoverage(rm, evidence),
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("simple exact target should keep only exact-target principal evidence, got %+v", got)
	}
	if !strings.Contains(lane.Entries[0].Location, "analysis_ir.go") {
		t.Fatalf("principal lane should keep ShapeValue evidence, got %+v", lane.Entries)
	}
	if strings.Contains(answerSupportLaneText(lane), "ShapeStepList") ||
		strings.Contains(answerSupportLaneText(lane), "answer_surface_plan.go") {
		t.Fatalf("same-family symbol leaked into principal lane: %+v", lane.Entries)
	}
}

func TestBuildAnswerSupportPlan_ChangeImpactStableAbsenceSuppressesAggregateSameFamilyMembers(t *testing.T) {
	profile := &ChangeImpactProfile{
		IsChangeImpact:  true,
		Target:          "ShapeValue",
		TargetKind:      SubjectEnumValue,
		Scope:           ImpactScopeProduction,
		RequestedOutput: ImpactOutputFiles,
		AffectedSiteKinds: []ImpactAffectedSiteKind{
			ImpactSiteDefinition,
			ImpactSiteRead,
		},
		Confidence: 0.92,
	}
	exact := EvidenceItem{
		ID:              "shapevalue-comment",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       1235,
		AnchorKind:      AnchorTextReference,
		AnchorSymbol:    "ShapeValue",
		Subject:         "ShapeValue",
		Snippet:         "// ShapeValue legacy mention",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	sameFamily := EvidenceItem{
		ID:              "shape-step-list",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/types/answer_surface_plan.go",
		LineStart:       295,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "ShapeStepList",
		Subject:         "ShapeStepList",
		Snippet:         "case ShapeStepList:",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	evidence := []EvidenceItem{sameFamily, exact}
	rm := RequestModel{
		Intent:              IntentEnumerate,
		AnalyzerHints:       AnalyzerHints{Kind: string(ReqEnumeration)},
		ChangeImpactProfile: profile,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	plan := &AnswerSurfacePlan{
		StableAbsent:        true,
		SurfaceEvidence:     evidence,
		ChangeImpactProfile: profile,
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "affected files",
			Members:     []string{"internal/types/answer_surface_plan.go", "internal/types/analysis_ir.go"},
			SupportRefs: []string{"internal/types/answer_surface_plan.go:295", "internal/types/analysis_ir.go:1235"},
		}},
		FacetCoverage: CompileFacetCoverage(rm, evidence),
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("stable absence should keep only aggregate rows with exact target support, got %+v", got)
	}
	if !strings.Contains(lane.Entries[0].Location, "analysis_ir.go") {
		t.Fatalf("exact-supported aggregate row should survive, got %+v", lane.Entries)
	}
	if strings.Contains(answerSupportLaneText(lane), "answer_surface_plan.go") ||
		strings.Contains(answerSupportLaneText(lane), "ShapeStepList") {
		t.Fatalf("stable absence resurrected same-family aggregate row: %+v", lane.Entries)
	}
}

func supportLaneHasSource(lane *AnswerSupportLane, source string) bool {
	if lane == nil {
		return false
	}
	for _, entry := range lane.Entries {
		if entry.Source == source {
			return true
		}
	}
	return false
}

func TestBuildAnswerSupportPlan_ChangeImpactInitializerIsAssignmentLike(t *testing.T) {
	target := "CitationReq.Required"
	initializer := EvidenceItem{
		ID:              "initializer",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/orchestrator/orchestrator.go",
		LineStart:       6362,
		AnchorKind:      AnchorInitializer,
		AnchorSymbol:    target,
		Subject:         target,
		Snippet:         "CitationReq: types.CitationReq{Required: false},",
		Summary:         "orchestrator initializes CitationReq.Required",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsRelationalLookup:    true,
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqEnumeration)},
		ChangeImpactProfile: &ChangeImpactProfile{
			IsChangeImpact:    true,
			Target:            target,
			TargetKind:        SubjectStructField,
			Scope:             ImpactScopeProduction,
			RequestedOutput:   ImpactOutputFiles,
			AffectedSiteKinds: []ImpactAffectedSiteKind{ImpactSiteAssignment},
			Confidence:        0.92,
		},
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence:     []EvidenceItem{initializer},
		ChangeImpactProfile: rm.ChangeImpactProfile,
		FacetCoverage:       CompileFacetCoverage(rm, []EvidenceItem{initializer}),
	}

	got := BuildAnswerSupportPlan(rm, plan)
	principalLane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if principalLane == nil || len(principalLane.Entries) != 1 {
		t.Fatalf("expected initializer principal evidence lane, got %+v", got)
	}
	entry := principalLane.Entries[0]
	if entry.AnchorKind != AnchorInitializer {
		t.Fatalf("entry anchor kind = %s, want initializer: %+v", entry.AnchorKind, entry)
	}
	if entry.ClaimForm != ClaimAssignmentFact {
		t.Fatalf("initializer must project to assignment_fact, got %s", entry.ClaimForm)
	}
	if !strings.Contains(entry.Text, "CitationReq") || !strings.Contains(entry.Text, "Required") {
		t.Fatalf("initializer target surface was not preserved: %+v", entry)
	}
}

func TestBuildAnswerSupportPlan_ChangeImpactDocumentationCanPromoteMechanismEvidence(t *testing.T) {
	docEvidence := EvidenceItem{
		ID:              "doc",
		Kind:            EvidenceMechanism,
		Scope:           ScopeLine,
		Source:          "docs/api.md",
		LineStart:       24,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "CitationReq",
		Subject:         "CitationReq",
		Summary:         "documentation describes CitationReq.Required",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	rm := RequestModel{
		Intent:        IntentEnumerate,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqEnumeration)},
		ChangeImpactProfile: &ChangeImpactProfile{
			IsChangeImpact:    true,
			Target:            "CitationReq.Required",
			RequestedOutput:   ImpactOutputFiles,
			AffectedSiteKinds: []ImpactAffectedSiteKind{ImpactSiteDocumentation},
			Confidence:        0.9,
		},
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence:     []EvidenceItem{docEvidence},
		ChangeImpactProfile: rm.ChangeImpactProfile,
		FacetCoverage:       CompileFacetCoverage(rm, []EvidenceItem{docEvidence}),
	}
	got := BuildAnswerSupportPlan(rm, plan)
	principalLane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if principalLane == nil || len(principalLane.Entries) != 1 {
		t.Fatalf("documentation impact requests may promote mechanism evidence, got %+v", got)
	}
	if principalLane.Entries[0].Source != "docs/api.md" {
		t.Fatalf("unexpected promoted documentation evidence: %+v", principalLane.Entries[0])
	}
}

func TestChangeImpactPrincipalTargetSurfaceGaps_FindsUnderstructuredSourceLine(t *testing.T) {
	profile := &ChangeImpactProfile{
		IsChangeImpact:  true,
		Target:          "CitationReq.Required",
		RequestedOutput: ImpactOutputFiles,
	}
	understructured := EvidenceItem{
		ID:              "understructured",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1915,
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "Required",
		Condition:       "isMeasurementScalar || isHistoryLookup",
		Summary:         "sets CitationReq.Required to false",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	structured := understructured
	structured.ID = "structured"
	structured.Source = "internal/orchestrator/contract_check.go"
	structured.LineStart = 63
	structured.AnchorSymbol = "CitationReq.Required"
	structured.Snippet = "c.CitationReq.Required = false"
	unrelated := understructured
	unrelated.ID = "unrelated"
	unrelated.Source = "internal/tool/answer_document_pre_emit_check.go"
	unrelated.LineStart = 400
	unrelated.Summary = "Required belongs to another owner, not CitationReq.Required"
	items := []EvidenceItem{understructured, structured, unrelated}

	gaps := ChangeImpactPrincipalTargetSurfaceGaps(profile, items, func(item EvidenceItem) string {
		switch item.ID {
		case "understructured":
			return "out.AnswerContract.CitationReq.Required = false"
		case "structured":
			return "c.CitationReq.Required = false"
		case "unrelated":
			return "if !req.Required { return nil }"
		default:
			return ""
		}
	})
	if len(gaps) != 1 {
		t.Fatalf("expected one under-structured target handoff gap, got %+v", gaps)
	}
	if gaps[0].ID != "understructured" {
		t.Fatalf("unexpected gap item: %+v", gaps[0])
	}
}

func TestPrincipalSupportMemberObligations_ChangeImpactFilesCoalesceByFile(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		ChangeImpactProfile: &ChangeImpactProfile{
			IsChangeImpact:  true,
			RequestedOutput: ImpactOutputFiles,
		},
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{
				{
					Text:          "template requires citations",
					Location:      "internal/analysis/compiler/templates.go:256",
					ClaimForm:     ClaimAssignmentFact,
					MemberSurface: PrincipalMemberSurfaceSourceLocation,
					AnchorSymbol:  "CitationReq",
					SurfaceTerms:  []string{"CitationReq"},
					Source:        "internal/analysis/compiler/templates.go",
					LineStart:     256,
				},
				{
					Text:          "another template requires citations",
					Location:      "internal/analysis/compiler/templates.go:366",
					ClaimForm:     ClaimAssignmentFact,
					MemberSurface: PrincipalMemberSurfaceSourceLocation,
					AnchorSymbol:  "CitationReq",
					SurfaceTerms:  []string{"CitationReq"},
					Source:        "internal/analysis/compiler/templates.go",
					LineStart:     366,
				},
				{
					Text:          "gate checks citation requirement",
					Location:      "internal/analysis/gate/gate.go:301",
					ClaimForm:     ClaimGuardCondition,
					MemberSurface: PrincipalMemberSurfaceSourceLocation,
					AnchorSymbol:  "CitationReq.Required",
					SurfaceTerms:  []string{"CitationReq.Required"},
					Source:        "internal/analysis/gate/gate.go",
					LineStart:     301,
				},
			},
		}},
	}

	obligations := PrincipalSupportMemberObligations(plan)
	if len(obligations) != 2 {
		t.Fatalf("file-output impact answers should require unique files, got %+v", obligations)
	}
	if obligations[0].Label != "internal/analysis/compiler/templates.go" {
		t.Fatalf("first file obligation should label the file path, got %+v", obligations[0])
	}
	if !strings.Contains(obligations[0].LocationHint(), "internal/analysis/compiler/templates.go:366") {
		t.Fatalf("coalesced file obligation should retain equivalent site anchors, got %q", obligations[0].LocationHint())
	}

	doc := &AnswerDocumentV2{
		Citations: []Citation{
			{File: "internal/analysis/compiler/templates.go", Line: 256},
			{File: "internal/analysis/gate/gate.go", Line: 301},
		},
		Blocks: []AnswerBlock{{
			ID:          "files",
			Kind:        BlockTable,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{
				{Label: "internal/analysis/compiler/templates.go", Text: "template construction sites", CitationRef: 0},
				{Label: "internal/analysis/gate/gate.go", Text: "guard site", CitationRef: 1},
			},
		}},
	}
	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("one cited row per file should satisfy file-output obligations, got %+v", got)
	}
}

func TestChangeImpactFileOutputLabelDrift_RequiresFileLabels(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		ChangeImpactProfile: &ChangeImpactProfile{
			IsChangeImpact:  true,
			RequestedOutput: ImpactOutputFiles,
		},
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{
				{
					Text:          "builder guard reads Required",
					Location:      "internal/context/builder.go:1729",
					ClaimForm:     ClaimGuardCondition,
					MemberSurface: PrincipalMemberSurfaceSourceLocation,
					Subject:       "CitationReq.Required",
					SurfaceTerms:  []string{"CitationReq.Required"},
					Source:        "internal/context/builder.go",
					LineStart:     1729,
				},
				{
					Text:          "gate guard reads Required",
					Location:      "internal/analysis/gate/gate.go:301",
					ClaimForm:     ClaimGuardCondition,
					MemberSurface: PrincipalMemberSurfaceSourceLocation,
					Subject:       "CitationReq.Required",
					SurfaceTerms:  []string{"CitationReq.Required"},
					Source:        "internal/analysis/gate/gate.go",
					LineStart:     301,
				},
			},
		}},
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{
			{File: "internal/context/builder.go", Line: 1729},
			{File: "internal/analysis/gate/gate.go", Line: 301},
		},
		Blocks: []AnswerBlock{{
			ID:          "files",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{
				{Label: "internal/context/builder.go:1729", Text: "guard site in the affected file", CitationRef: 0},
				{Label: "internal/analysis/gate/gate.go", Text: "guard site in the affected file", CitationRef: 1},
			},
		}},
	}
	diag := ChangeImpactFileOutputLabelDrift(doc, plan)
	if diag == nil || len(diag.MissingLabels) != 1 {
		t.Fatalf("file-output answer should reject site-shaped principal labels, got %+v", diag)
	}
	if diag.MissingLabels[0].Source != "internal/context/builder.go" {
		t.Fatalf("missing file label = %+v", diag.MissingLabels[0])
	}
	doc.Blocks[0].Items[0].Label = "internal/context/builder.go"
	if got := ChangeImpactFileOutputLabelDrift(doc, plan); got != nil {
		t.Fatalf("file-path labels should satisfy file-output principal surface, got %+v", got)
	}
}

func TestCompileFacetCoverage_OrdinaryEnumerationDoesNotPromoteGuardSites(t *testing.T) {
	assign := EvidenceItem{
		ID:              "assignment",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "x.go",
		LineStart:       10,
		AnchorKind:      AnchorAssignment,
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	guard := EvidenceItem{
		ID:              "guard",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "x.go",
		LineStart:       20,
		AnchorKind:      AnchorCondition,
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	rm := RequestModel{
		Intent:        IntentEnumerate,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqEnumeration)},
	}
	coverage := CompileFacetCoverage(rm, []EvidenceItem{assign, guard})
	if coverage == nil || len(coverage.Required) == 0 {
		t.Fatalf("missing coverage: %+v", coverage)
	}
	var candidates []string
	for _, req := range coverage.Required {
		if req.Kind == FacetEnumerationItem {
			candidates = req.SourceCandidate
		}
	}
	if len(candidates) != 1 || candidates[0] != "assignment" {
		t.Fatalf("ordinary enumeration should not make guard principal, candidates=%v", candidates)
	}
}

func TestBuildAnswerSupportPlan_GenericScalarKeepsModelAuthoredAssignments(t *testing.T) {
	modelAssignment := EvidenceItem{
		ID:              "model-assignment",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/orchestrator/orchestrator.go",
		LineStart:       6362,
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "CitationReq",
		Subject:         "AnswerContract",
		Predicate:       "sets",
		Object:          "false",
		Snippet:         "CitationReq: types.CitationReq{Required: false}",
		Summary:         "orchestrator.go:6362 initializes AnswerContract.CitationReq.Required to false",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	concreteNoise := EvidenceItem{
		ID:              "concrete-noise",
		Kind:            EvidenceConcrete,
		Scope:           ScopeLine,
		Source:          "internal/types/evidence_closure.go",
		LineStart:       1870,
		AnchorKind:      AnchorCall,
		AnchorSymbol:    "BumpViolationsLogged",
		Subject:         "EvidenceClosure.BumpViolationsLogged",
		Predicate:       "calls",
		Object:          "c.bumpStat",
		Producer:        "concrete_values",
		GroundingStatus: GroundingGrounded,
	}
	rm := RequestModel{
		Intent:        IntentReturnValue,
		AnswerSubject: AnswerSubject{Kind: SubjectNumeric, Confidence: 0.95},
		Predicates: SemanticPredicates{
			IsScalarAnswer:  true,
			IsCountQuestion: true,
		},
	}
	evidence := []EvidenceItem{concreteNoise, modelAssignment}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: evidence,
		FacetCoverage:   CompileFacetCoverage(rm, evidence),
	}

	got := BuildAnswerSupportPlan(rm, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) == 0 {
		t.Fatalf("missing principal evidence lane: %+v", got)
	}
	if !strings.Contains(lane.Entries[0].Text, "CitationReq") ||
		!strings.Contains(lane.Entries[0].Location, "internal/orchestrator/orchestrator.go:6362") {
		t.Fatalf("model-authored assignment should be the leading scalar/count support entry, got %+v", lane.Entries[0])
	}
	if strings.Contains(answerSupportLaneText(lane), "BumpViolationsLogged") {
		t.Fatalf("deterministic concrete-value side evidence must not pollute model-authored scalar/count lane: %+v", lane.Entries)
	}
}

func TestBuildAnswerSupportPlan_GenericScalarDedupesSameTypedSurfaceRole(t *testing.T) {
	base := EvidenceItem{
		ID:              "assignment",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1903,
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		Subject:         "AnswerContract.CitationReq.Required",
		Object:          "false",
		Snippet:         "out.AnswerContract.CitationReq.Required = false",
		Summary:         "sets the citation requirement flag to false",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	countCarrier := base
	countCarrier.ID = "assignment-count-carrier"
	countCarrier.Summary = "4"
	countCarrier.Object = "4"
	countCarrier.LoadBearingSummary = true
	rm := RequestModel{
		Intent:        IntentReturnValue,
		AnswerSubject: AnswerSubject{Kind: SubjectNumeric, Confidence: 0.95},
		Predicates: SemanticPredicates{
			IsScalarAnswer:  true,
			IsCountQuestion: true,
		},
	}
	evidence := []EvidenceItem{base, countCarrier}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: evidence,
		FacetCoverage:   CompileFacetCoverage(rm, evidence),
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil {
		t.Fatalf("expected principal support lane, got %+v", got)
	}
	if len(lane.Entries) != 1 {
		t.Fatalf("same file:line + claim surface should collapse to one entry, got %+v", lane.Entries)
	}
}

func TestPrincipalSupportEvidenceItemsForFacetCuratesBroadCandidates(t *testing.T) {
	modelItem := EvidenceItem{
		ID:              "literal-model",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1903,
		AnchorKind:      AnchorAssignment,
		Subject:         "CitationReq.Required",
		Object:          "false",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	evidence := []EvidenceItem{modelItem}
	candidates := []string{modelItem.ID}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("noise-%02d", i)
		evidence = append(evidence, EvidenceItem{
			ID:              id,
			Kind:            EvidenceConcrete,
			Scope:           ScopeLine,
			Source:          "internal/tool/noise.go",
			LineStart:       i + 1,
			AnchorKind:      AnchorAssignment,
			AnchorSymbol:    fmt.Sprintf("helper%d", i),
			Producer:        "concrete_values",
			GroundingStatus: GroundingGrounded,
		})
		candidates = append(candidates, id)
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: evidence,
		FacetCoverage: &FacetCoverageContract{
			Family: QFGeneric,
			Required: []FacetRequirement{{
				Kind:            FacetResolvedLiteralOrSymbol,
				SourceCandidate: candidates,
			}},
		},
	}

	items := PrincipalSupportEvidenceItemsForFacet(QFGeneric, plan, FacetResolvedLiteralOrSymbol)
	if len(items) != 1 || items[0].ID != "literal-model" {
		t.Fatalf("facet-scoped principal evidence should keep model-authored answer fact only, got %+v", items)
	}
}

func TestPrincipalSupportEvidenceItemsForFacetKeepsDeterministicReturnFact(t *testing.T) {
	modelDefinition := EvidenceItem{
		ID:              "model-definition",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       31,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "Name",
		Subject:         "SubExplorer.Name",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	deterministicReturn := EvidenceItem{
		ID:              "terminal-return",
		Kind:            EvidenceConcrete,
		Scope:           ScopeLine,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       32,
		AnchorKind:      AnchorReturn,
		AnchorSymbol:    "Name",
		Subject:         "SubExplorer.Name",
		Predicate:       "returns",
		Object:          `"explorer"`,
		Producer:        "bridge_literal_terminal",
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{modelDefinition, deterministicReturn},
		FacetCoverage: &FacetCoverageContract{
			Family: QFRoleLookup,
			Required: []FacetRequirement{{
				Kind:            FacetResolvedLiteralOrSymbol,
				SourceCandidate: []string{modelDefinition.ID, deterministicReturn.ID},
			}},
		},
	}

	items := PrincipalSupportEvidenceItemsForFacet(QFRoleLookup, plan, FacetResolvedLiteralOrSymbol)
	if len(items) != 2 {
		t.Fatalf("principal support should keep model definition plus deterministic return fact, got %+v", items)
	}
	forms := map[ClaimForm]bool{}
	for _, item := range items {
		forms[ClaimFormOf(item)] = true
	}
	if !forms[ClaimDefinitionFact] || !forms[ClaimReturnFact] {
		t.Fatalf("expected definition_fact and return_fact support, got forms=%v items=%+v", forms, items)
	}
}

func TestBuildAnswerSupportPlan_EnumerationPrioritizesModelAuthoredSurfaceEvidence(t *testing.T) {
	imports := []EvidenceItem{
		{
			ID:              "import-logging",
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "internal/agent/explorer.go",
			LineStart:       19,
			AnchorKind:      AnchorImport,
			AnchorSymbol:    "logging",
			Producer:        "explorer.emit_evidence",
			SurfaceTerms:    []string{"github.com/hanchaoqun/codrax/internal/logging"},
			GroundingStatus: GroundingGrounded,
		},
		{
			ID:              "import-types",
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "internal/agent/explorer.go",
			LineStart:       26,
			AnchorKind:      AnchorImport,
			AnchorSymbol:    "types",
			Producer:        "explorer.emit_evidence",
			SurfaceTerms:    []string{"github.com/hanchaoqun/codrax/internal/types"},
			GroundingStatus: GroundingGrounded,
		},
	}
	var evidence []EvidenceItem
	var candidates []string
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("bulk-%02d", i)
		evidence = append(evidence, EvidenceItem{
			ID:              id,
			Kind:            EvidenceConcrete,
			Scope:           ScopeLine,
			Source:          "internal/agent/explorer.go",
			LineStart:       1 + i,
			AnchorKind:      AnchorAssignment,
			AnchorSymbol:    fmt.Sprintf("helper%d", i),
			Producer:        "concrete_values",
			GroundingStatus: GroundingGrounded,
		})
		candidates = append(candidates, id)
	}
	evidence = append(evidence, imports...)
	candidates = append(candidates, "import-logging", "import-types")
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: evidence,
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: candidates,
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentEnumerate}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) < 2 {
		t.Fatalf("expected principal evidence lane with import entries, got %+v", got)
	}
	first := lane.Entries[0].Text + "\n" + lane.Entries[1].Text
	for _, want := range []string{
		"github.com/hanchaoqun/codrax/internal/logging",
		"github.com/hanchaoqun/codrax/internal/types",
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("model-authored import surface terms should outrank bulk concrete values; missing %q in first entries:\n%s\nfull lane: %+v", want, first, lane.Entries)
		}
	}
	if strings.Contains(answerSupportLaneText(lane), "helper0") {
		t.Fatalf("model-authored enumeration evidence should exclude bulk concrete side evidence: %+v", lane.Entries)
	}
	if lane.Entries[0].ClaimForm != ClaimImportEdge || lane.Entries[0].LabelSurface != ClaimLabelSurfaceDisplayLabel {
		t.Fatalf("principal support entry should preserve typed import surface metadata, got %+v", lane.Entries[0])
	}
	if len(lane.Entries[0].SurfaceTerms) == 0 {
		t.Fatalf("principal support entry should preserve surface_terms for downstream member coverage, got %+v", lane.Entries[0])
	}
}

func TestBuildAnswerSupportPlan_EnumerationPrioritizesGroundedDefinitions(t *testing.T) {
	importItem := EvidenceItem{
		ID:              "import-helper",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/agent/explorer.go",
		LineStart:       12,
		AnchorKind:      AnchorImport,
		AnchorSymbol:    "helper",
		Producer:        "explorer.emit_evidence",
		SurfaceTerms:    []string{"github.com/hanchaoqun/codrax/internal/helper"},
		GroundingStatus: GroundingGrounded,
	}
	definitionItem := EvidenceItem{
		ID:              "definition-family",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/types/facet_plan.go",
		LineStart:       200,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "QFEnumeration",
		Producer:        "explorer.emit_evidence",
		Summary:         "QFEnumeration is the grounded definition for the requested member",
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{importItem, definitionItem},
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: []string{importItem.ID, definitionItem.ID},
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentEnumerate}, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) < 2 {
		t.Fatalf("expected definition and import evidence in principal lane, got %+v", got)
	}
	if !strings.Contains(lane.Entries[0].Text, "QFEnumeration") {
		t.Fatalf("grounded definition evidence must outrank adjacent import/support evidence, got entries=%+v", lane.Entries)
	}
}

func TestMissingPrincipalSupportMembers_EnumerationRequiresCitedPrincipalRows(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{
				{
					Text:         "agent imports criterion",
					Location:     "internal/agent/extractor.go:43",
					ClaimForm:    ClaimImportEdge,
					LabelSurface: ClaimLabelSurfaceDisplayLabel,
					Subject:      "agent",
					Object:       "github.com/hanchaoqun/codrax/internal/analysis/criterion",
					SurfaceTerms: []string{"agent", "github.com/hanchaoqun/codrax/internal/analysis/criterion"},
				},
				{
					Text:         "cmd imports criterion",
					Location:     "cmd/root.go:51",
					ClaimForm:    ClaimImportEdge,
					LabelSurface: ClaimLabelSurfaceDisplayLabel,
					Subject:      "cmd",
					Object:       "github.com/hanchaoqun/codrax/internal/analysis/criterion",
					SurfaceTerms: []string{"cmd", "github.com/hanchaoqun/codrax/internal/analysis/criterion"},
				},
			},
		}},
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{
			{File: "cmd/root.go", Line: 51},
		},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockTable,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "cmd",
				Text:        "imports criterion",
				CitationRef: 0,
			}},
		}},
	}

	missing := MissingPrincipalSupportMembers(doc, plan)
	if len(missing) != 1 || !strings.Contains(missing[0].Location, "internal/agent/extractor.go:43") {
		t.Fatalf("expected only the uncited agent member to be missing, got %+v", missing)
	}
	doc.Citations = append(doc.Citations, Citation{File: "internal/agent/extractor.go", Line: 43})
	doc.Blocks[0].Items = append(doc.Blocks[0].Items, AnswerBlockItem{
		Label:       "agent",
		Text:        "imports criterion",
		CitationRef: 1,
	})
	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("all cited principal rows should satisfy support-member coverage, got %+v", got)
	}
}

func TestMissingPrincipalSupportMembers_CaveatBlockTextCanExplainExclusion(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{{
				Text:         "helper returns false",
				Location:     "internal/agent/agent.go:1515",
				ClaimForm:    ClaimReturnFact,
				AnchorSymbol: "isReadFilePathMiss",
				SurfaceTerms: []string{"isReadFilePathMiss", "false"},
				Source:       "internal/agent/agent.go",
				LineStart:    1515,
			}},
		}},
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/agent/agent.go", Line: 1515}},
		Blocks: []AnswerBlock{
			{
				ID:          "items",
				Kind:        BlockOrderedList,
				SurfaceRole: SurfacePrincipal,
				FacetIDs:    []string{string(FacetEnumerationItem)},
				Items: []AnswerBlockItem{{
					Label:       "explorer",
					Text:        "the only principal member",
					CitationRef: -1,
				}},
			},
			{
				ID:   "excluded_helper",
				Kind: BlockCaveat,
				Text: "`isReadFilePathMiss` is a helper return fact, not a member of the requested agent set.",
				Items: []AnswerBlockItem{{
					ID:          "cite",
					CitationRef: 0,
				}},
			},
		},
	}
	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("caveat block text with matching citation should satisfy excluded support member, got %+v", got)
	}
}

func TestMissingPrincipalSupportMembers_DefinitionAnchorsCoalesceAndAcceptEquivalentCitation(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{
				{
					Text:         "CitationReq type declaration",
					Location:     "internal/types/analysis_ir.go:1229",
					ClaimForm:    ClaimDefinitionFact,
					AnchorSymbol: "CitationReq",
					SurfaceTerms: []string{"CitationReq"},
					Source:       "internal/types/analysis_ir.go",
					LineStart:    1229,
				},
				{
					Text:         "CitationReq.Required field declaration",
					Location:     "internal/types/analysis_ir.go:1232",
					ClaimForm:    ClaimDefinitionFact,
					AnchorSymbol: "CitationReq",
					SurfaceTerms: []string{"CitationReq"},
					Source:       "internal/types/analysis_ir.go",
					LineStart:    1232,
				},
			},
		}},
	}

	obligations := PrincipalSupportMemberObligations(plan)
	if len(obligations) != 1 {
		t.Fatalf("definition anchors for the same typed member should coalesce, got %+v", obligations)
	}
	if !strings.Contains(obligations[0].LocationHint(), "internal/types/analysis_ir.go:1232") {
		t.Fatalf("coalesced definition obligation should expose equivalent anchors, got %q", obligations[0].LocationHint())
	}

	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/types/analysis_ir.go", Line: 1232}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockBulletList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "CitationReq",
				Text:        "type definition",
				CitationRef: 0,
			}},
		}},
	}
	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("equivalent definition anchor should satisfy member coverage, got %+v", got)
	}

	doc.Citations[0].Line = 1234
	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("nearby line in the same typed definition span should satisfy member coverage, got %+v", got)
	}
}

func TestMissingPrincipalSupportMembers_AcceptsTypedEvidenceLineRangeCitation(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{{
				Text:         "ArkTS initializer range",
				Location:     "entry/src/main/ets/pages/index.ets:42",
				ClaimForm:    ClaimAssignmentFact,
				AnchorSymbol: "pageConfig",
				SurfaceTerms: []string{"pageConfig"},
				Source:       "entry/src/main/ets/pages/index.ets",
				LineStart:    42,
				LineEnd:      44,
			}},
		}},
	}

	obligations := PrincipalSupportMemberObligations(plan)
	if len(obligations) != 1 {
		t.Fatalf("range-backed principal member should produce one obligation, got %+v", obligations)
	}
	if !strings.Contains(obligations[0].LocationHint(), "entry/src/main/ets/pages/index.ets:44") {
		t.Fatalf("range-backed obligation should expose line_end as an accepted typed anchor, got %q", obligations[0].LocationHint())
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "entry/src/main/ets/pages/index.ets", Line: 43}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockTable,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "pageConfig",
				Text:        "initializer value",
				CitationRef: 0,
			}},
		}},
	}
	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("citation inside typed evidence line range should satisfy member coverage, got %+v", got)
	}
}

func TestMissingPrincipalSupportMembers_AcceptsSplitRelationMemberSurface(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{{
				Text:         "aggregator: Aggregate @ aggregator.go:132",
				Location:     "internal/analysis/aggregator/aggregator.go:132",
				ClaimForm:    ClaimDefinitionFact,
				AnchorSymbol: "Aggregate",
				SurfaceTerms: []string{"aggregator: Aggregate @ aggregator.go:132"},
				Source:       "internal/analysis/aggregator/aggregator.go",
				LineStart:    132,
			}},
		}},
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/analysis/aggregator/aggregator.go", Line: 132}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "aggregator",
				Text:        "Aggregate is the entry point.",
				CitationRef: 0,
			}},
		}},
	}

	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("split label/text relation member should satisfy support-member coverage, got %+v", got)
	}
}

func TestMissingPrincipalSupportMembers_AcceptsDecoratedFileMemberSplitAcrossItemSurface(t *testing.T) {
	member := "ActivityManagerService.java (前后台状态通知/mForegroundPackages系统级聚合)"
	plan := &AnswerSupportPlan{
		Family:                  QFEnumeration,
		PrincipalMemberCoverage: PrincipalMemberCoveragePolicyRequired,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{{
				Text:         member,
				Location:     "java/com/android/server/am/ActivityManagerService.java:1618",
				ClaimForm:    ClaimDefinitionFact,
				SurfaceTerms: []string{member},
				Source:       "java/com/android/server/am/ActivityManagerService.java",
				LineStart:    1618,
			}},
		}},
	}
	obligations := PrincipalSupportMemberObligations(plan)
	if len(obligations) != 1 {
		t.Fatalf("expected one obligation, got %+v", obligations)
	}
	surface := "| ActivityManagerService.java | 前后台状态通知/mForegroundPackages系统级聚合 |"
	if !AnswerTextMentionsSupportMember(surface, obligations[0]) {
		t.Fatalf("decorated file member split across table cells should be recognized")
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "java/com/android/server/am/ActivityManagerService.java", Line: 1618}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockTable,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "ActivityManagerService.java",
				Text:        "前后台状态通知/mForegroundPackages系统级聚合",
				CitationRef: 0,
			}},
		}},
	}

	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("split decorated file member should satisfy support-member coverage, got %+v", got)
	}
}

func TestMissingPrincipalSupportMembers_AcceptsModelRelationWithoutSymbolEndpoint(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{{
				Text:         "declarative → New (Classifier) @ internal/analysis/declarative/classifier.go:138",
				Location:     "internal/analysis/declarative/classifier.go:138",
				ClaimForm:    ClaimDefinitionFact,
				SurfaceTerms: []string{"declarative → New (Classifier) @ internal/analysis/declarative/classifier.go:138"},
				Source:       "internal/analysis/declarative/classifier.go",
				LineStart:    138,
			}},
		}},
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/analysis/declarative/classifier.go", Line: 138}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "declarative",
				Text:        "入口函数是 `New(Classifier)`。",
				CitationRef: 0,
			}},
		}},
	}

	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("split decorated relation member should satisfy support-member coverage, got %+v", got)
	}
}

func TestMissingPrincipalSupportMembers_AcceptsDecoratorListRelationMember(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{{
				Text:         "patcher → Engine (New + Submit/Apply) @ internal/analysis/patcher/patcher.go:83",
				Location:     "internal/analysis/patcher/patcher.go:83",
				ClaimForm:    ClaimDefinitionFact,
				SurfaceTerms: []string{"patcher → Engine (New + Submit/Apply) @ internal/analysis/patcher/patcher.go:83"},
				Source:       "internal/analysis/patcher/patcher.go",
				LineStart:    83,
			}},
		}},
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/analysis/patcher/patcher.go", Line: 83}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "Engine (New + Submit/Apply)",
				Text:        "patcher 子包入口类型。",
				CitationRef: 0,
			}},
		}},
	}

	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("decorator-list relation member should satisfy support-member coverage, got %+v", got)
	}
}

func TestBuildAnswerSupportPlan_GenericAggregateSupportRefMemberUsesLabelAndCitation(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateMemberSet,
			Label: "LoopController implementers",
			Value: "1",
			Members: []string{
				"explorerEvaluator (internal/agent/explorer.go:30)",
			},
		}},
	}
	support := BuildAnswerSupportPlan(RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{
			Required:    true,
			SourceQuote: "all implementers",
		},
	}, plan)
	obligations := PrincipalSupportMemberObligations(support)
	if len(obligations) != 1 {
		t.Fatalf("support-ref member should produce one principal obligation, got %+v", obligations)
	}
	if obligations[0].Label != "explorerEvaluator" {
		t.Fatalf("support-ref member should use the label as principal surface, got %+v", obligations[0])
	}
	if obligations[0].Location != "internal/agent/explorer.go:30" {
		t.Fatalf("support-ref member should carry the citation location, got %+v", obligations[0])
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/agent/explorer.go", Line: 30}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "explorerEvaluator",
				Text:        "implements LoopController.",
				CitationRef: 0,
			}},
		}},
	}
	if got := MissingPrincipalSupportMembers(doc, support); len(got) != 0 {
		t.Fatalf("label plus matching citation should satisfy support-ref member coverage, got %+v", got)
	}
}

func TestBuildAnswerSupportPlan_CompactAggregateSupportRefMemberUsesLabelAndCitation(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateMemberSet,
			Label: "LoopController implementers",
			Value: "1",
			Members: []string{
				"answerDocumentEvaluator@internal/agent/answer_document_evaluator.go:4544",
			},
		}},
	}
	support := BuildAnswerSupportPlan(RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{
			Required:    true,
			SourceQuote: "all implementers",
		},
	}, plan)
	obligations := PrincipalSupportMemberObligations(support)
	if len(obligations) != 1 {
		t.Fatalf("compact support-ref member should produce one principal obligation, got %+v", obligations)
	}
	if obligations[0].Label != "answerDocumentEvaluator" {
		t.Fatalf("compact support-ref member should use the label as principal surface, got %+v", obligations[0])
	}
	if obligations[0].Location != "internal/agent/answer_document_evaluator.go:4544" {
		t.Fatalf("compact support-ref member should carry the citation location, got %+v", obligations[0])
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/agent/answer_document_evaluator.go", Line: 4544}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockTable,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "answerDocumentEvaluator",
				Text:        "implements LoopController.",
				CitationRef: 0,
			}},
		}},
	}
	if got := MissingPrincipalSupportMembers(doc, support); len(got) != 0 {
		t.Fatalf("label plus matching citation should satisfy compact support-ref member coverage, got %+v", got)
	}
}

func TestBuildAnswerSupportPlan_GenericAggregateMemberSetUpgradesShortDisplayLocationFromSupportRef(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateMemberSet,
			Label: "public criterion symbols",
			Value: "2",
			Members: []string{
				"Kind @ grammar.go:26",
				"Eval @ eval.go:15",
			},
			SupportRefs: []string{
				"Member @ internal/analysis/criterion/grammar.go:26",
				"Member @ internal/analysis/criterion/eval.go:15",
			},
		}},
	}
	support := BuildAnswerSupportPlan(RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{
			Required:    true,
			SourceQuote: "public criterion symbols",
		},
	}, plan)
	obligations := PrincipalSupportMemberObligations(support)
	if len(obligations) != 2 {
		t.Fatalf("expected two principal obligations, got %+v", obligations)
	}
	if obligations[0].Label != "Kind" || obligations[0].Location != "internal/analysis/criterion/grammar.go:26" {
		t.Fatalf("short display path should upgrade to precise grammar.go support_ref, got %+v", obligations[0])
	}
	if obligations[1].Label != "Eval" || obligations[1].Location != "internal/analysis/criterion/eval.go:15" {
		t.Fatalf("short display path should upgrade to precise eval.go support_ref, got %+v", obligations[1])
	}

	doc := &AnswerDocumentV2{
		Citations: []Citation{
			{File: "internal/analysis/criterion/grammar.go", Line: 26},
			{File: "internal/analysis/criterion/eval.go", Line: 15},
		},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "Kind",
				Text:        "type Kind string.",
				CitationRef: 0,
			}, {
				Label:       "Eval",
				Text:        "func Eval.",
				CitationRef: 1,
			}},
		}},
	}
	if got := MissingPrincipalSupportMembers(doc, support); len(got) != 0 {
		t.Fatalf("full-path citations should satisfy obligations compiled from short display locations, got %+v", got)
	}
}

func TestBuildAnswerSupportPlan_RelationMemberShortDisplayLocationUsesTypedEvidencePath(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateMemberSet,
			Label: "internal/analysis 子包清单及入口函数",
			Value: "1",
			Members: []string{
				"aggregator: Aggregate (aggregator.go:132)",
			},
		}},
		SurfaceEvidence: []EvidenceItem{{
			Kind:            EvidenceDirect,
			Source:          "internal/analysis/aggregator/aggregator.go",
			LineStart:       132,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "Aggregate",
			Summary:         "aggregator package entry point",
			GroundingStatus: GroundingGrounded,
		}},
	}
	support := BuildAnswerSupportPlan(RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{
			Required:    true,
			SourceQuote: "all analysis subpackages",
		},
	}, plan)
	obligations := PrincipalSupportMemberObligations(support)
	if len(obligations) != 1 {
		t.Fatalf("expected one principal obligation, got %+v", obligations)
	}
	if obligations[0].Location != "internal/analysis/aggregator/aggregator.go:132" {
		t.Fatalf("short display location should upgrade from typed definition evidence, got %+v", obligations[0])
	}

	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/analysis/aggregator/aggregator.go", Line: 132}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "aggregator: Aggregate (aggregator.go:132)",
				Text:        "entry function.",
				CitationRef: 0,
			}},
		}},
	}
	if got := MissingPrincipalSupportMembers(doc, support); len(got) != 0 {
		t.Fatalf("full-path citation plus relation label should satisfy member coverage, got %+v", got)
	}
}

func TestBuildAnswerSupportPlan_CompleteCountFactMembersActAsPrincipalRows(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateTotalCount,
			Label:   "public functions",
			Value:   "2",
			Members: []string{"Eval @ eval.go:15", "EvalAll @ eval.go:36"},
			SupportRefs: []string{
				"Member @ internal/analysis/criterion/eval.go:15",
				"Member @ internal/analysis/criterion/eval.go:36",
			},
		}},
	}
	support := BuildAnswerSupportPlan(RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{
			Required:    true,
			SourceQuote: "all public functions",
		},
	}, plan)
	obligations := PrincipalSupportMemberObligations(support)
	if len(obligations) != 2 {
		t.Fatalf("complete count members should compile into principal obligations, got %+v", obligations)
	}
	if obligations[0].Label != "Eval" || obligations[0].Location != "internal/analysis/criterion/eval.go:15" {
		t.Fatalf("first count member should use precise support_ref, got %+v", obligations[0])
	}
	if obligations[1].Label != "EvalAll" || obligations[1].Location != "internal/analysis/criterion/eval.go:36" {
		t.Fatalf("second count member should use precise support_ref, got %+v", obligations[1])
	}
}

func TestMissingPrincipalSupportMembers_AssignmentStillRequiresExactCitationLine(t *testing.T) {
	plan := &AnswerSupportPlan{
		Family: QFEnumeration,
		Lanes: []AnswerSupportLane{{
			Kind: SupportLanePrincipalEvidence,
			Entries: []AnswerSupportEntry{{
				Text:         "CitationReq.Required is assigned",
				Location:     "internal/agent/analyzer.go:1903",
				ClaimForm:    ClaimAssignmentFact,
				AnchorSymbol: "CitationReq.Required",
				SurfaceTerms: []string{"CitationReq.Required"},
				Source:       "internal/agent/analyzer.go",
				LineStart:    1903,
			}},
		}},
	}
	doc := &AnswerDocumentV2{
		Citations: []Citation{{File: "internal/agent/analyzer.go", Line: 1904}},
		Blocks: []AnswerBlock{{
			ID:          "items",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "CitationReq.Required",
				Text:        "assignment site",
				CitationRef: 0,
			}},
		}},
	}
	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 1 {
		t.Fatalf("non-definition facts must still require exact cited line, got %+v", got)
	}
	doc.Citations[0].Line = 1903
	if got := MissingPrincipalSupportMembers(doc, plan); len(got) != 0 {
		t.Fatalf("exact assignment line should satisfy member coverage, got %+v", got)
	}
}

func TestBuildAnswerSupportPlan_EnumerationPrincipalLaneDoesNotSilentlyCapMembers(t *testing.T) {
	const n = 70
	evidence := make([]EvidenceItem, 0, n)
	candidates := make([]string, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("PublicEnum%02d", i)
		id := fmt.Sprintf("enum-%02d", i)
		evidence = append(evidence, EvidenceItem{
			ID:              id,
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "internal/types/generated_enums.go",
			LineStart:       10 + i,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    name,
			Producer:        "explorer.emit_evidence",
			GroundingStatus: GroundingGrounded,
		})
		candidates = append(candidates, id)
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: evidence,
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: candidates,
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentEnumerate}, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil {
		t.Fatalf("missing principal evidence lane: %+v", got)
	}
	if len(lane.Entries) != n {
		t.Fatalf("enumeration principal support lane must preserve every evidence-backed member; entries=%d want=%d", len(lane.Entries), n)
	}
	if !strings.Contains(lane.Entries[len(lane.Entries)-1].Text, "PublicEnum69") {
		t.Fatalf("tail principal member was capped away or reordered unexpectedly: %+v", lane.Entries[len(lane.Entries)-1])
	}
}

func TestBuildAnswerSupportPlan_FacetFamilyKeepsExternalObservationSoftUnlessFacetRequiresIt(t *testing.T) {
	plan := &AnswerSurfacePlan{
		ExternalObservationSeeds: []ExternalObservationSeed{
			{Kind: "error_type", Raw: "panic"},
		},
		SurfaceEvidence: []EvidenceItem{{
			ID:              "architecture-edge",
			Kind:            EvidenceRelationship,
			Scope:           ScopeLine,
			Source:          "internal/orchestrator/topology.go",
			LineStart:       62,
			AnchorKind:      AnchorCall,
			Subject:         "scheduler",
			Object:          "explorer",
			GroundingStatus: GroundingGrounded,
		}},
		FacetCoverage: &FacetCoverageContract{
			Family: QFArchitecture,
			Required: []FacetRequirement{{
				Kind:            FacetComponentRelation,
				SourceCandidate: []string{"architecture-edge"},
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentExplain, Scenario: ScenarioArchitectureExplain}, plan)
	if got == nil {
		t.Fatal("expected architecture support plan")
	}
	if observed := answerSupportLaneByKind(got, SupportLaneObservedArtifact); observed != nil {
		t.Fatalf("non-diagnostic architecture support plan must not promote external log observations into typed principal lanes: %+v", observed)
	}
	if principal := answerSupportLaneByKind(got, SupportLanePrincipalEvidence); principal == nil {
		t.Fatalf("expected principal architecture lane to remain, got %+v", got.Lanes)
	}
}

func TestBuildAnswerSupportPlan_ArchitectureCarriesRoleOutputEnrichment(t *testing.T) {
	principal := EvidenceItem{
		ID:              "stage-boundary",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/orchestrator/topology.go",
		LineStart:       24,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "pipelineTopology",
		Subject:         "pipelineTopology",
		GroundingStatus: GroundingGrounded,
		Producer:        "explorer.emit_evidence",
	}
	output := EvidenceItem{
		ID:                 "analyze-output",
		Kind:               EvidenceMechanism,
		Scope:              ScopeLine,
		Source:             "internal/agent/analyzer.go",
		LineStart:          36,
		AnchorKind:         AnchorDefinition,
		AnchorSymbol:       "analyzerEvaluator",
		Subject:            "StageAnalyze",
		Predicate:          "produces",
		Object:             "AnalysisIR / TaskGraph / EvidencePlan",
		Summary:            "StageAnalyze classifies the request and emits AnalysisIR with TaskGraph, EvidencePlan, and hypotheses for downstream stages.",
		LoadBearingSummary: true,
		GroundingStatus:    GroundingGrounded,
		Producer:           "explorer.emit_evidence",
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{principal, output},
		FacetCoverage: &FacetCoverageContract{
			Family: QFArchitecture,
			Required: []FacetRequirement{{
				Kind:            FacetComponentRelation,
				SourceCandidate: []string{principal.ID},
			}},
			Optional: []FacetRequirement{{
				Kind:            FacetNearestMechanism,
				SourceCandidate: []string{output.ID},
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentExplain, Scenario: ScenarioArchitectureExplain}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	lane := answerSupportLaneByKind(got, SupportLaneNearestMechanism)
	if lane == nil {
		t.Fatalf("expected architecture role/output enrichment lane, got %+v", got.Lanes)
	}
	if !strings.Contains(lane.Guidance, "produces") || !strings.Contains(lane.Guidance, "typed IRs") {
		t.Fatalf("guidance should describe role/output artifacts, got %q", lane.Guidance)
	}
	if len(lane.Entries) != 1 {
		t.Fatalf("enrichment entries=%d want 1: %+v", len(lane.Entries), lane.Entries)
	}
	if !strings.Contains(lane.Entries[0].Text, "AnalysisIR") || !strings.Contains(lane.Entries[0].Text, "TaskGraph") {
		t.Fatalf("role/output entry lost structured artifact detail: %+v", lane.Entries[0])
	}
}

func TestBuildAnswerSupportPlan_FacetPrincipalLaneAcceptsSchemaScopeEvidence(t *testing.T) {
	item := EvidenceItem{
		ID:              "file-layer",
		Kind:            EvidenceDirect,
		Scope:           ScopeFile,
		Source:          "codrax.yaml",
		FileRoleLabel:   FileRoleConfigCanonical,
		Subject:         "codrax.yaml",
		Predicate:       "is",
		Object:          "runtime config layer",
		Summary:         "codrax.yaml participates in config precedence without a single file:line anchor",
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{item},
		FacetCoverage: &FacetCoverageContract{
			Family: QFConfigPrecedence,
			Required: []FacetRequirement{{
				Kind:            FacetConfigPrecedenceRole,
				SourceCandidate: []string{item.ID},
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentConfigQuery}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("expected schema-scope principal entry, got %+v", got.Lanes)
	}
	entry := lane.Entries[0]
	if !strings.Contains(entry.Text, "codrax.yaml is runtime config layer") {
		t.Fatalf("file-scope structured fact should surface as principal text, got %+v", entry)
	}
	if entry.Location != "codrax.yaml" {
		t.Fatalf("file-scope location = %q, want codrax.yaml", entry.Location)
	}
}

func TestBuildAnswerSupportPlan_ConfigPrecedenceDropsConcreteNoiseWhenModelEvidenceExists(t *testing.T) {
	modelItem := EvidenceItem{
		ID:              "default-layer",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "cmd/root.go",
		LineStart:       71,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "defaultMaxSteps",
		Producer:        "explorer.emit_evidence",
		DiagramRole:     EvidenceDiagramRoleDefault,
		Summary:         "defaultMaxSteps defines the code default layer.",
		GroundingStatus: GroundingGrounded,
	}
	concreteNoise := EvidenceItem{
		ID:              "concrete-noise",
		Kind:            EvidenceConcrete,
		Scope:           ScopeLine,
		Source:          "internal/tool/repomap/topology/discover.go",
		LineStart:       59,
		AnchorKind:      AnchorAssignment,
		AnchorSymbol:    "Discover",
		Producer:        "concrete_values",
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{concreteNoise, modelItem},
		FacetCoverage: &FacetCoverageContract{
			Family: QFConfigPrecedence,
			Required: []FacetRequirement{{
				Kind:            FacetConfigPrecedenceRole,
				SourceCandidate: []string{"default-layer", "concrete-noise"},
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentConfigQuery}, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil {
		t.Fatalf("expected config principal lane, got %+v", got)
	}
	text := answerSupportLaneText(lane)
	if !strings.Contains(text, "defaultMaxSteps") {
		t.Fatalf("model-authored config evidence missing from lane:\n%s", text)
	}
	if strings.Contains(text, "Discover") {
		t.Fatalf("concrete-value noise should not pollute config precedence lane when model-authored evidence exists:\n%s", text)
	}
}

func TestBuildAnswerSupportPlan_ConfigPrecedenceExactAbsenceKeepsRequestedRolesPrincipal(t *testing.T) {
	target := syntheticConfigAbsenceKeyForTest()
	rm := RequestModel{
		RawRequest:    target + " 在默认值 / codrax.yaml / CLI flag 三层里各自是什么？",
		Intent:        IntentConfigQuery,
		Scenario:      ScenarioConfigTrace,
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
		AnalyzerHints: AnalyzerHints{
			Kind:              string(ReqConfigMapping),
			ExactTargets:      []string{target},
			ExactContextRoles: []EvidenceDiagramRole{EvidenceDiagramRoleDefault, EvidenceDiagramRoleConfig, EvidenceDiagramRoleOverride},
		},
	}
	contract := BuildExactResolutionContract(rm)
	if contract == nil {
		t.Fatal("expected exact-resolution contract")
	}
	plan := &AnswerSurfacePlan{
		ExactResolution: contract,
		PreferredExactResolution: &AnswerExactResolution{
			Status:      AnswerExactResolutionAbsent,
			ContextMode: AnswerExactResolutionContextGroundedOnly,
		},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "default-layer",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "internal/types/explore_budget.go",
				LineStart:       40,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "ExploreBudget",
				Summary:         "ExploreBudget is the same explore-budget family and has no xyz phantom field",
				ContextRole:     EvidenceContextRoleRelatedContext,
				DiagramRole:     EvidenceDiagramRoleDefault,
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
			{
				ID:                   "config-layer-absence",
				Kind:                 EvidenceAbsent,
				Scope:                ScopeNegative,
				Source:               "codrax.yaml.example",
				ContextRole:          EvidenceContextRoleAbsenceSupport,
				RequestedDiagramRole: EvidenceDiagramRoleConfig,
				NegativeQuery:        &NegativeQuery{File: "codrax.yaml.example", Pattern: target},
				NegativeScope:        NegativeScopeFile,
				SurfaceTerms:         []string{"codrax.yaml", target},
				Producer:             "explorer.emit_evidence",
				GroundingStatus:      GroundingGrounded,
				GroundingTier:        TierNegativeConfirmed,
			},
			{
				ID:              "skill-defaults-boundary",
				Kind:            EvidenceAbsent,
				Scope:           ScopeNegative,
				Source:          "internal/skill/defaults.go",
				ContextRole:     EvidenceContextRoleAbsenceSupport,
				Summary:         "exact key absent from skill defaults registry",
				NegativeQuery:   &NegativeQuery{File: "internal/skill/defaults.go", Pattern: target},
				NegativeScope:   NegativeScopeFile,
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierNegativeConfirmed,
			},
			{
				ID:              "override-missing-boundary",
				Kind:            EvidenceAbsent,
				Scope:           ScopeNegative,
				Source:          "cmd/root.go",
				ContextRole:     EvidenceContextRoleAbsenceSupport,
				Summary:         "exact key absent from CLI flag bindings",
				NegativeQuery:   &NegativeQuery{File: "cmd/root.go", Pattern: target},
				NegativeScope:   NegativeScopeFile,
				SurfaceTerms:    []string{"CLI flag"},
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierNegativeConfirmed,
			},
		},
	}
	plan.FacetCoverage = CompileFacetCoverage(rm, plan.SurfaceEvidence)

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil {
		t.Fatalf("expected principal config lane, got %+v", got)
	}
	text := answerSupportLaneText(lane)
	if !strings.Contains(text, "ExploreBudget") || !supportLaneHasSource(lane, "codrax.yaml.example") {
		t.Fatalf("principal lane should keep requested default/config role anchors, got entries=%+v text:\n%s", lane.Entries, text)
	}
	if supportLaneHasSource(lane, "internal/skill/defaults.go") || supportLaneHasSource(lane, "cmd/root.go") {
		t.Fatalf("boundary-only absence proofs must not become extra principal precedence rows: %+v", lane.Entries)
	}
	boundary := answerSupportLaneByKind(got, SupportLaneUncertaintyBound)
	if boundary == nil || !supportLaneHasSource(boundary, "internal/skill/defaults.go") ||
		!supportLaneHasSource(boundary, "cmd/root.go") {
		t.Fatalf("dropped boundary proofs should remain available in uncertainty lane, got %+v", got.Lanes)
	}
}

func TestBuildAnswerSupportPlan_FacetUncertaintyLaneAcceptsNegativeScopeAbsence(t *testing.T) {
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{{
			ID:              "absence-proof",
			Kind:            EvidenceAbsent,
			Scope:           ScopeNegative,
			Subject:         "legacy_config_key",
			Predicate:       "absent",
			NegativeQuery:   &NegativeQuery{File: "codrax.yaml", Pattern: "legacy_config_key"},
			NegativeScope:   NegativeScopeFile,
			ContextRole:     EvidenceContextRoleAbsenceSupport,
			GroundingStatus: GroundingGrounded,
		}},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentExplain, Scenario: ScenarioGeneric}, plan)
	if got == nil {
		t.Fatal("expected boundary-only support plan")
	}
	lane := answerSupportLaneByKind(got, SupportLaneUncertaintyBound)
	if lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("negative-scope absence should reach uncertainty lane, got %+v", got.Lanes)
	}
	if lane.Entries[0].Location != "codrax.yaml" {
		t.Fatalf("negative-scope location = %q, want codrax.yaml", lane.Entries[0].Location)
	}
	if !strings.Contains(lane.Entries[0].Text, "legacy_config_key absent") {
		t.Fatalf("negative-scope text should use structured fields, got %+v", lane.Entries[0])
	}
}

func TestBuildAnswerSupportPlan_FacetFamilyWithoutTypedCandidatesReturnsNil(t *testing.T) {
	plan := &AnswerSurfacePlan{
		SurfaceEvidence: []EvidenceItem{{
			ID:              "helper-only",
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1406,
			AnchorKind:      AnchorCall,
			Subject:         "analyzer",
			Object:          "BuildArtifactObservationProfile",
			GroundingStatus: GroundingGrounded,
		}},
		FacetCoverage: &FacetCoverageContract{
			Family: QFGeneric,
			Required: []FacetRequirement{{
				Kind: FacetCurrentCodePath,
			}},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{Intent: IntentExplain, Scenario: ScenarioGeneric}, plan)
	if got != nil {
		t.Fatalf("generic non-observation question without typed facet candidates should not create a principal hard lane: %+v", got)
	}
}

func TestBuildAnswerSupportPlan_ChangeImpactUsesAggregateMemberSetAsPrincipalLane(t *testing.T) {
	profile := &ChangeImpactProfile{
		IsChangeImpact:  true,
		Target:          "ShapeValue",
		TargetKind:      SubjectEnumValue,
		RequestedOutput: ImpactOutputFiles,
		Scope:           ImpactScopeProduction,
		Confidence:      0.9,
	}
	unrelated := EvidenceItem{
		ID:              "unrelated-definition",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       1083,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "ContractTermKind.IsValid",
		Subject:         "ContractTermKind.IsValid",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
	}
	plan := &AnswerSurfacePlan{
		ChangeImpactProfile: profile,
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateMemberSet,
			Label: "affected source members",
			Value: "3",
			Members: []string{
				"internal/agent/analyzer.go:1935-1949",
				"internal/types/answer_semantic_view_helpers.go:17-18",
				"internal/types/answer_semantic_view_helpers.go:122",
			},
		}},
		SurfaceEvidence: []EvidenceItem{unrelated},
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: []string{unrelated.ID},
			}},
		},
	}
	rm := RequestModel{
		Intent:              IntentEnumerate,
		AnalyzerHints:       AnalyzerHints{Kind: string(ReqEnumeration)},
		ChangeImpactProfile: profile,
		Predicates:          SemanticPredicates{IsCategoryEnumeration: true},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 3 {
		t.Fatalf("aggregate member_set should compile into three source members, got %+v", got)
	}
	if !strings.Contains(lane.Title, "Model-emitted") {
		t.Fatalf("principal lane should identify aggregate origin, got %q", lane.Title)
	}
	if strings.Contains(answerSupportLaneText(lane), "ContractTermKind") {
		t.Fatalf("facet helper must not replace the aggregate principal member set: %+v", lane.Entries)
	}
	for _, entry := range lane.Entries {
		if entry.ClaimForm != ClaimUnknown || entry.MemberSurface != PrincipalMemberSurfaceSourceLocation {
			t.Fatalf("aggregate source members should be non-symbol principal entries, got %+v", entry)
		}
		if entry.Producer != "explorer.emit_investigation_complete.aggregate_facts" {
			t.Fatalf("aggregate source member producer not preserved: %+v", entry)
		}
	}
	obligations := PrincipalSupportMemberObligations(got)
	if len(obligations) != 2 {
		t.Fatalf("requested_output=files should coalesce duplicate file sites into two file obligations, got %+v", obligations)
	}
	gotLabels := map[string]bool{}
	for _, ob := range obligations {
		gotLabels[ob.Label] = true
		if ob.ClaimForm != ClaimUnknown {
			t.Fatalf("aggregate source member should not masquerade as a symbol claim, got %+v", ob)
		}
	}
	for _, want := range []string{"internal/agent/analyzer.go", "internal/types/answer_semantic_view_helpers.go"} {
		if !gotLabels[want] {
			t.Fatalf("missing file obligation %q from %+v", want, obligations)
		}
	}
}

func TestBuildAnswerSupportPlan_ChangeImpactAggregateMemberInheritsTextReferenceSupport(t *testing.T) {
	profile := &ChangeImpactProfile{
		IsChangeImpact:    true,
		Target:            "ShapeValue",
		TargetKind:        SubjectEnumValue,
		RequestedOutput:   ImpactOutputFiles,
		Scope:             ImpactScopeAll,
		AffectedSiteKinds: []ImpactAffectedSiteKind{ImpactSiteDocumentation},
		Confidence:        0.9,
	}
	textRef := EvidenceItem{
		ID:              "shapevalue-comment",
		Kind:            EvidenceDirect,
		Scope:           ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       1235,
		AnchorKind:      AnchorTextReference,
		AnchorSymbol:    "ShapeValue",
		Subject:         "ShapeValue",
		Snippet:         "// ShapeStepList / ShapeValue / ShapeBoolean / ShapeConfigValue /",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
	}
	plan := &AnswerSurfacePlan{
		ChangeImpactProfile: profile,
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "affected source members",
			Members:     []string{"internal/types/analysis_ir.go"},
			SupportRefs: []string{"internal/types/analysis_ir.go:1235"},
		}},
		SurfaceEvidence: []EvidenceItem{textRef},
		FacetCoverage:   CompileFacetCoverage(RequestModel{ChangeImpactProfile: profile}, []EvidenceItem{textRef}),
	}
	rm := RequestModel{
		Intent:              IntentEnumerate,
		AnalyzerHints:       AnalyzerHints{Kind: string(ReqEnumeration)},
		ChangeImpactProfile: profile,
		Predicates:          SemanticPredicates{IsCategoryEnumeration: true},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("aggregate text-reference member should compile into principal lane, got %+v", got)
	}
	entry := lane.Entries[0]
	if entry.ClaimForm != ClaimTextReferenceFact || entry.AnchorKind != AnchorTextReference {
		t.Fatalf("aggregate member did not inherit text_reference support: %+v", entry)
	}
	if !strings.Contains(entry.Detail, "not_definition_call_or_assignment=true") {
		t.Fatalf("text-reference boundary detail missing: %+v", entry)
	}
}

func TestBuildAnswerSupportPlan_GenericEnumerationUsesAggregateMemberSetAsPrincipalLane(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateMemberSet,
			Label:       "enum type names",
			Value:       "3",
			Members:     []string{"Intent", "Scenario", "internal/types/enums.go:6"},
			SupportRefs: []string{"Intent @ internal/types/analysis_ir.go:653", "Scenario @ internal/types/analysis_ir.go:673"},
		}},
		SurfaceEvidence: []EvidenceItem{{
			ID:              "helper",
			Kind:            EvidenceDirect,
			Source:          "internal/types/helper.go",
			LineStart:       10,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "HelperContext",
			GroundingStatus: GroundingGrounded,
			GroundingTier:   TierLineText,
		}},
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind:            FacetEnumerationItem,
				SourceCandidate: []string{"helper"},
			}},
		},
	}
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"Intent", "Scenario", "HelperContext"},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all enum types"},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 3 {
		t.Fatalf("aggregate member_set should compile into the full principal lane, got %+v", got)
	}
	if strings.Contains(answerSupportLaneText(lane), "HelperContext") {
		t.Fatalf("facet helper must not replace aggregate principal members: %+v", lane.Entries)
	}
	if lane.Entries[0].Text != "Intent" || lane.Entries[1].Text != "Scenario" {
		t.Fatalf("aggregate member order not preserved: %+v", lane.Entries)
	}
	if lane.Entries[0].Location != "internal/types/analysis_ir.go:653" ||
		lane.Entries[0].MemberSurface != PrincipalMemberSurfaceSymbolLike {
		t.Fatalf("member-specific support_ref should cite the location without replacing the symbol member surface: %+v", lane.Entries[0])
	}
	if lane.Entries[2].Location != "internal/types/enums.go:6" ||
		lane.Entries[2].MemberSurface != PrincipalMemberSurfaceSourceLocation {
		t.Fatalf("file:line member should remain the principal source-location surface: %+v", lane.Entries[2])
	}
}

func TestBuildAnswerSupportPlan_GenericAggregateMemberSetGenericSupportRefCarriesTypedSnippetEvidence(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "default SubAgent names",
			Value:   "1",
			Members: []string{"explorer"},
			SupportRefs: []string{
				"Member @ internal/agent/subagent.go:63",
				"Member @ internal/agent/sub_explorer.go:31",
			},
		}},
		SurfaceEvidence: []EvidenceItem{{
			ID:              "register-default",
			Kind:            EvidenceDirect,
			Source:          "internal/agent/subagent.go",
			LineStart:       63,
			AnchorKind:      AnchorCall,
			AnchorSymbol:    "RegisterDefaultSubAgents",
			Snippet:         "func RegisterDefaultSubAgents(r *SubAgentRegistry, deps *Dependencies) {\n\tr.Register(NewSubExplorer(deps))\n}",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: GroundingGrounded,
			GroundingTier:   TierLineText,
		}, {
			ID:              "subexplorer-name",
			Kind:            EvidenceDirect,
			Source:          "internal/agent/sub_explorer.go",
			LineStart:       31,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "Name",
			Snippet:         "func (s *SubExplorer) Name() string {\n\treturn \"explorer\"\n}",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: GroundingGrounded,
			GroundingTier:   TierLineText,
		}},
	}
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqEnumeration),
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "default SubAgent names"},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("generic support_ref aggregate member should compile into one principal entry, got %+v", got)
	}
	entry := lane.Entries[0]
	if entry.Text != "explorer" || entry.Location != "internal/agent/sub_explorer.go:31" {
		t.Fatalf("member label and support_ref location should be preserved, got %+v", entry)
	}
	if entry.ClaimForm != ClaimDefinitionFact || entry.AnchorSymbol != "Name" {
		t.Fatalf("support_ref location should inherit same-line typed evidence metadata, got %+v", entry)
	}
	if entry.MemberSurface != PrincipalMemberSurfaceSymbolLike {
		t.Fatalf("member surface should stay on the answer label, not the helper function name: %+v", entry)
	}
}

func TestBuildAnswerSupportPlan_GenericAggregateMemberSetParallelSupportRefs(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "internal import packages",
			Value:   "2",
			Members: []string{"github.com/acme/project/internal/foo", "github.com/acme/project/internal/bar"},
			SupportRefs: []string{
				"internal/agent/explorer.go:13",
				"internal/agent/explorer.go:14",
			},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "import-foo",
				Kind:            EvidenceDirect,
				Source:          "internal/agent/explorer.go",
				LineStart:       13,
				AnchorKind:      AnchorImport,
				Object:          "github.com/acme/project/internal/foo",
				SurfaceTerms:    []string{"github.com/acme/project/internal/foo"},
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
			{
				ID:              "import-bar",
				Kind:            EvidenceDirect,
				Source:          "internal/agent/explorer.go",
				LineStart:       14,
				AnchorKind:      AnchorImport,
				Object:          "github.com/acme/project/internal/bar",
				SurfaceTerms:    []string{"github.com/acme/project/internal/bar"},
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
		},
		FacetCoverage: &FacetCoverageContract{
			Family: QFEnumeration,
			Required: []FacetRequirement{{
				Kind: FacetEnumerationItem,
			}},
		},
	}
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"internal/agent/explorer.go"},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all internal imports"},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 2 {
		t.Fatalf("parallel support refs should preserve the two aggregate members, got %+v", got)
	}
	if lane.Entries[0].Text != "github.com/acme/project/internal/foo" ||
		lane.Entries[0].Location != "internal/agent/explorer.go:13" ||
		lane.Entries[0].MemberSurface == PrincipalMemberSurfaceSourceLocation {
		t.Fatalf("first import member should keep package label and line 13 support, got %+v", lane.Entries[0])
	}
	if lane.Entries[1].Text != "github.com/acme/project/internal/bar" ||
		lane.Entries[1].Location != "internal/agent/explorer.go:14" ||
		lane.Entries[1].MemberSurface == PrincipalMemberSurfaceSourceLocation {
		t.Fatalf("second import member should keep package label and line 14 support, got %+v", lane.Entries[1])
	}
	obligations := PrincipalSupportMemberObligations(got)
	if len(obligations) != 2 {
		t.Fatalf("package members should become two obligations, got %+v", obligations)
	}
	if obligations[0].Label != "github.com/acme/project/internal/foo" ||
		obligations[1].Label != "github.com/acme/project/internal/bar" {
		t.Fatalf("obligations must use package labels, not container file labels: %+v", obligations)
	}
}

func TestBuildAnswerSupportPlan_AggregateImportMembersInheritDisplaySurface(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "internal import packages",
			Value:   "2",
			Members: []string{"github.com/acme/project/internal/foo", "github.com/acme/project/internal/bar"},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "import-foo",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "src/main.ts",
				LineStart:       3,
				AnchorKind:      AnchorImport,
				AnchorSymbol:    "github.com/acme/project/internal/foo",
				SurfaceTerms:    []string{"github.com/acme/project/internal/foo"},
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
			{
				ID:              "import-bar",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "src/main.ts",
				LineStart:       4,
				AnchorKind:      AnchorImport,
				AnchorSymbol:    "github.com/acme/project/internal/bar",
				SurfaceTerms:    []string{"github.com/acme/project/internal/bar"},
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
		},
	}
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"src/main.ts"},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all internal imports"},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 2 {
		t.Fatalf("aggregate import member_set should compile into two principal entries, got %+v", got)
	}
	for _, entry := range lane.Entries {
		if entry.ClaimForm != ClaimImportEdge || entry.LabelSurface != ClaimLabelSurfaceDisplayLabel {
			t.Fatalf("aggregate import member should inherit typed import claim metadata, got %+v", entry)
		}
		if entry.MemberSurface != PrincipalMemberSurfaceDisplayLabel {
			t.Fatalf("aggregate import path is a reference literal, not a symbol-definition member: %+v", entry)
		}
	}
	nonSymbol, symbolLike := AnswerSupportPlanPrincipalSurfaceCounts(got)
	if nonSymbol != 2 || symbolLike != 0 {
		t.Fatalf("principal surface counts should route import literals away from answer symbols, got nonSymbol=%d symbolLike=%d", nonSymbol, symbolLike)
	}
}

func TestBuildAnswerSupportPlan_GenericAggregateMemberSetReusesTypedEvidenceLocations(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "public enum members",
			Value:   "2",
			Members: []string{"Intent", "RouteKind"},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "go-intent",
				Kind:            EvidenceDirect,
				Source:          "internal/types/analysis_ir.go",
				LineStart:       653,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "Intent",
				Subject:         "Intent",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
			{
				ID:              "arkts-route",
				Kind:            EvidenceDirect,
				Source:          "entry/src/main/ets/router/RouteKind.ets",
				LineStart:       12,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "RouteKind",
				Subject:         "RouteKind",
				SurfaceTerms:    []string{"RouteKind"},
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
		},
	}
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"Intent", "RouteKind"},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all enum types"},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 2 {
		t.Fatalf("aggregate member_set should compile into two principal entries, got %+v", got)
	}
	if lane.Entries[0].Location != "internal/types/analysis_ir.go:653" ||
		lane.Entries[0].ClaimForm != ClaimDefinitionFact ||
		lane.Entries[0].AnchorSymbol != "Intent" {
		t.Fatalf("Go member should inherit typed evidence citation metadata, got %+v", lane.Entries[0])
	}
	if lane.Entries[1].Location != "entry/src/main/ets/router/RouteKind.ets:12" ||
		lane.Entries[1].ClaimForm != ClaimDefinitionFact ||
		lane.Entries[1].AnchorSymbol != "RouteKind" {
		t.Fatalf("ArkTS member should inherit typed evidence citation metadata, got %+v", lane.Entries[1])
	}
	obligations := PrincipalSupportMemberObligations(got)
	if len(obligations) != 2 {
		t.Fatalf("typed evidence-backed aggregate members should become cited obligations, got %+v", obligations)
	}
}

func TestBuildAnswerSupportPlan_GenericAggregateMemberSetPrefersAnchorSymbolOverCallSubject(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "analyze degraded paths",
			Value:   "1",
			Members: []string{"buildDegradedSemanticIR"},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "semantic-callsite",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "internal/orchestrator/orchestrator.go",
				LineStart:       2006,
				AnchorKind:      AnchorCall,
				AnchorSymbol:    "buildDegradedSemanticIR",
				Object:          "buildDegradedSemanticIR",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
			{
				ID:              "fallback-inside-semantic",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "internal/orchestrator/orchestrator.go",
				LineStart:       7612,
				AnchorKind:      AnchorCall,
				AnchorSymbol:    "buildDegradedFallbackIR",
				Subject:         "buildDegradedSemanticIR",
				Object:          "buildDegradedFallbackIR",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
		},
	}
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("aggregate member_set should compile into one principal entry, got %+v", got)
	}
	if lane.Entries[0].Location != "internal/orchestrator/orchestrator.go:2006" ||
		lane.Entries[0].AnchorSymbol != "buildDegradedSemanticIR" ||
		lane.Entries[0].ClaimForm != ClaimCallEdge {
		t.Fatalf("member should cite the line whose anchor_symbol matches the member, not a sibling call where it is only the subject: %+v", lane.Entries[0])
	}
}

func TestBuildAnswerSupportPlan_GenericAggregateMemberSetCoAnchorsSupportRefAndTypedDefinition(t *testing.T) {
	cases := []struct {
		name     string
		member   string
		defFile  string
		defLine  int
		implFile string
		implLine int
		symbol   string
	}{
		{
			name:     "go_same_file_method_proof",
			member:   "analyzerEvaluator (internal/agent/analyzer.go:887)",
			defFile:  "internal/agent/analyzer.go",
			defLine:  46,
			implFile: "internal/agent/analyzer.go",
			implLine: 887,
			symbol:   "analyzerEvaluator",
		},
		{
			name:     "arkts_same_file_lifecycle_proof",
			member:   "LifecycleObserver (entry/src/main/ets/lifecycle/Observer.ets:88)",
			defFile:  "entry/src/main/ets/lifecycle/Observer.ets",
			defLine:  12,
			implFile: "entry/src/main/ets/lifecycle/Observer.ets",
			implLine: 88,
			symbol:   "LifecycleObserver",
		},
		{
			name:     "cpp_header_definition_source_implementation",
			member:   "ParserObserver (src/parser/observer.cpp:73)",
			defFile:  "include/parser/observer.h",
			defLine:  15,
			implFile: "src/parser/observer.cpp",
			implLine: 73,
			symbol:   "ParserObserver",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := &AnswerSurfacePlan{
				StableAggregateFacts: []AnswerAggregateFact{{
					Kind:    AnswerAggregateMemberSet,
					Label:   "typed implementers",
					Value:   "1",
					Members: []string{tc.member},
				}},
				SurfaceEvidence: []EvidenceItem{{
					ID:              "definition-" + tc.name,
					Kind:            EvidenceDirect,
					Scope:           ScopeLine,
					Source:          tc.defFile,
					LineStart:       tc.defLine,
					AnchorKind:      AnchorDefinition,
					AnchorSymbol:    tc.symbol,
					Subject:         tc.symbol,
					Producer:        "explorer.emit_evidence",
					GroundingStatus: GroundingGrounded,
					GroundingTier:   TierLineText,
				}},
			}
			rm := RequestModel{
				Intent:     IntentEnumerate,
				Predicates: SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: AnalyzerHints{
					Kind:     string(ReqEnumeration),
					Entities: []string{tc.symbol},
				},
				CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all implementers"},
			}

			got := BuildAnswerSupportPlan(rm, plan)
			lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
			if lane == nil || len(lane.Entries) != 1 {
				t.Fatalf("aggregate member_set should compile into one principal entry, got %+v", got)
			}
			if lane.Entries[0].EvidenceID != "aggregate_fact:member_set:0:0" {
				t.Fatalf("aggregate member evidence id should preserve zero-based fact/member indexes, got %+v", lane.Entries[0])
			}
			wantImpl := tc.implFile + ":" + intString(tc.implLine)
			wantDef := tc.defFile + ":" + intString(tc.defLine)
			if !strings.EqualFold(lane.Entries[0].Location, wantImpl) {
				t.Fatalf("model support_ref should remain the primary aggregate anchor, got %+v", lane.Entries[0])
			}
			if !stringSliceContainsFold(lane.Entries[0].EquivalentLocations, wantDef) {
				t.Fatalf("typed definition should be retained as an equivalent anchor, got %+v", lane.Entries[0])
			}

			obligations := PrincipalSupportMemberObligations(got)
			if len(obligations) != 1 {
				t.Fatalf("co-anchored member should stay one obligation, got %+v", obligations)
			}
			for _, citation := range []Citation{
				{File: tc.implFile, Line: tc.implLine},
				{File: tc.defFile, Line: tc.defLine},
			} {
				doc := &AnswerDocumentV2{
					Citations: []Citation{citation},
					Blocks: []AnswerBlock{{
						ID:          "items",
						Kind:        BlockOrderedList,
						SurfaceRole: SurfacePrincipal,
						FacetIDs:    []string{string(FacetEnumerationItem)},
						Items: []AnswerBlockItem{{
							Label:       tc.symbol,
							Text:        "typed implementer",
							CitationRef: 0,
						}},
					}},
				}
				if missing := MissingPrincipalSupportMembers(doc, got); len(missing) != 0 {
					t.Fatalf("citation %+v should satisfy co-anchored member coverage, got %+v", citation, missing)
				}
			}
		})
	}
}

func TestBuildAnswerSupportPlan_GenericAggregateMemberSetDoesNotCoAnchorAmbiguousCrossFileDefinitions(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "typed implementers",
			Value:   "1",
			Members: []string{"ParserObserver (src/parser/observer.cpp:73)"},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "parser-observer-a",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "include/parser/observer.h",
				LineStart:       15,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "ParserObserver",
				Subject:         "ParserObserver",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
			{
				ID:              "parser-observer-b",
				Kind:            EvidenceDirect,
				Scope:           ScopeLine,
				Source:          "third_party/parser/observer.h",
				LineStart:       9,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "ParserObserver",
				Subject:         "ParserObserver",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
		},
	}
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"ParserObserver"},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all implementers"},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("aggregate member_set should compile into one principal entry, got %+v", got)
	}
	if len(lane.Entries[0].EquivalentLocations) != 0 {
		t.Fatalf("ambiguous cross-file same-name definitions must not become equivalent anchors, got %+v", lane.Entries[0])
	}
}

func TestBuildAnswerSupportPlan_GenericAggregateMemberSetPrefersRelationSlateOverLeftAxis(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{
			{
				Kind:    AnswerAggregateMemberSet,
				Label:   "internal/analysis subpackage directories",
				Value:   "2",
				Members: []string{"aggregator", "compiler"},
			},
			{
				Kind:    AnswerAggregateMemberSet,
				Label:   "subpackage entry functions",
				Value:   "2",
				Members: []string{"aggregator → Aggregate", "compiler → Compile"},
			},
		},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "axis-helper",
				Kind:            EvidenceDirect,
				Source:          "internal/agent/explorer.go",
				LineStart:       6440,
				AnchorKind:      AnchorAssignment,
				AnchorSymbol:    "filterPartialReadsByAuthoritativeFrames",
				SurfaceTerms:    []string{"aggregator"},
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
		},
	}
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true},
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqEnumeration),
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all subpackages and entry functions"},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 2 {
		t.Fatalf("relation member_set should be the only principal slate, got %+v", got)
	}
	if lane.Entries[0].Text != "aggregator → Aggregate" || lane.Entries[1].Text != "compiler → Compile" {
		t.Fatalf("principal entries should preserve relation members, got %+v", lane.Entries)
	}
	if strings.Contains(answerSupportLaneText(lane), "filterPartialReadsByAuthoritativeFrames") {
		t.Fatalf("left-axis helper evidence must not become principal via label reuse: %+v", lane.Entries)
	}
}

func TestBuildAnswerSupportPlan_GenericAggregateMemberSetRelationEvidenceRequiresLeftScope(t *testing.T) {
	plan := &AnswerSurfacePlan{
		StableAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateMemberSet,
			Label:   "scoped scoring helpers",
			Value:   "2",
			Members: []string{"priority.Score", "subject.Score"},
		}},
		SurfaceEvidence: []EvidenceItem{
			{
				ID:              "priority-score",
				Kind:            EvidenceDirect,
				Source:          "internal/analysis/priority/score.go",
				LineStart:       23,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "Score",
				Subject:         "priority.Score",
				SurfaceTerms:    []string{"Score", "priority"},
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
			{
				ID:              "subject-score",
				Kind:            EvidenceDirect,
				Source:          "internal/analysis/subject/score.go",
				LineStart:       34,
				AnchorKind:      AnchorDefinition,
				AnchorSymbol:    "Score",
				Subject:         "subject.Score",
				SurfaceTerms:    []string{"Score", "subject"},
				Producer:        "explorer.emit_evidence",
				GroundingStatus: GroundingGrounded,
				GroundingTier:   TierLineText,
			},
		},
	}
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true},
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqEnumeration),
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all scoped scoring helpers"},
	}

	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if lane == nil || len(lane.Entries) != 2 {
		t.Fatalf("relation member_set should compile into two principal entries, got %+v", got)
	}
	if lane.Entries[0].Location != "internal/analysis/priority/score.go:23" ||
		lane.Entries[1].Location != "internal/analysis/subject/score.go:34" {
		t.Fatalf("relation evidence must bind by left scope plus right endpoint, got %+v", lane.Entries)
	}
}

func answerSupportLaneByKind(plan *AnswerSupportPlan, kind AnswerSupportLaneKind) *AnswerSupportLane {
	if plan == nil {
		return nil
	}
	for i := range plan.Lanes {
		if plan.Lanes[i].Kind == kind {
			return &plan.Lanes[i]
		}
	}
	return nil
}

func answerSupportLaneText(lane *AnswerSupportLane) string {
	if lane == nil {
		return ""
	}
	var out []string
	for _, entry := range lane.Entries {
		out = append(out, entry.Text)
	}
	return strings.Join(out, "\n")
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
				Summary:      "ParseOutput invokes buildAnalysisIR as the observed analyzer handoff",
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

func TestCompileCurrentStatusVerdictSupportLanePreservesEntryDetail(t *testing.T) {
	lane := compileCurrentStatusVerdictSupportLane(&AnswerSupportPlan{
		Lanes: []AnswerSupportLane{{
			Kind:  SupportLaneCurrentCodePath,
			Title: "Current grounded code path",
			Entries: []AnswerSupportEntry{{
				Text:     "ParseOutput calls buildAnalysisIR",
				Detail:   "ParseOutput invokes buildAnalysisIR as the observed analyzer handoff",
				Location: "internal/agent/analyzer.go:743",
			}},
		}},
	})
	if len(lane.Entries) != 1 {
		t.Fatalf("verdict lane entries = %+v", lane.Entries)
	}
	if !strings.Contains(lane.Entries[0].Detail, "observed analyzer handoff") {
		t.Fatalf("current-status verdict lane should preserve source evidence detail, got %+v", lane.Entries)
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
				Summary:      "ParseOutput calls buildAnalysisIR as the observed frame transition",
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
				Summary:      "ParseOutput calls buildAnalysisIR as the observed frame transition",
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
	if len(pathLane.Entries) == 0 || !strings.Contains(pathLane.Entries[0].Detail, "observed frame transition") {
		t.Fatalf("path lane should preserve model-emitted evidence detail as enrichment, got %+v", pathLane.Entries)
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
				Summary:      "PrimaryEntities snapshots the original analyzer entity shortlist before deterministic augmentation",
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
	mechanismDetail := ""
	for _, entry := range mechanismLane.Entries {
		mechanismDetail += entry.Detail + "\n"
	}
	if !strings.Contains(mechanismDetail, "original analyzer entity shortlist") {
		t.Fatalf("mechanism lane should preserve structured evidence detail, got %+v", mechanismLane.Entries)
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
