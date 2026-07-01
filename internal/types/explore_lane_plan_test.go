package types

import "testing"

func TestCompileExploreLanePlan_MixedVCSDiffCurrentSource(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes: []CurrentSourceExplanationMode{
				CurrentSourceExplanationLocateCurrentCode,
			},
			Confidence:   0.9,
			SourceQuotes: []string{"结合源码"},
		},
	}
	presentation := AnswerPresentationContract{
		RequestedDimensions: []RequestedAnswerDimension{
			{Label: "diff 线索", Role: RequestedAnswerDimensionDiffClue, Index: 1},
			{Label: "当前关键代码", Role: RequestedAnswerDimensionCurrentKeyCode, Index: 2},
			{Label: "作用和影响", Role: RequestedAnswerDimensionImpact, Index: 3},
		},
	}
	got := CompileExploreLanePlan(rm, nil, presentation)
	assertExploreLaneOrigin(t, got, AnswerEvidenceOriginVCSMetadata, "diff 线索")
	assertExploreLaneOrigin(t, got, AnswerEvidenceOriginVCSDiff, "diff 线索")
	assertExploreLaneOrigin(t, got, AnswerEvidenceOriginCurrentSource, "当前关键代码")
	if len(got.Lanes) != 3 {
		t.Fatalf("lanes=%d want 3: %+v", len(got.Lanes), got.Lanes)
	}
}

func TestCompileExploreLanePlan_OrdinaryArchitectureUnchanged(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
	}
	got := CompileExploreLanePlan(rm, nil, AnswerPresentationContract{})
	if !got.Empty() {
		t.Fatalf("ordinary single-origin architecture request should not materialize lanes: %+v", got.Lanes)
	}
}

func TestCompileExploreLanePlan_ExternalObservationExcludeSuppressesCurrentSourceDimensions(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		PerfTrace: &PerfBundle{Observations: []PerfObservation{{
			Kind:    "frame_jank",
			Subject: "app-100",
			Summary: "runtime trace identifies a jank window",
		}}},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
			ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
			SourceQuotes:         []string{"只分析 trace"},
			Confidence:           0.95,
		},
		SubTopics: []SubTopic{
			{Summary: "target thread state"},
			{Summary: "wakeup chain"},
		},
	}
	presentation := AnswerPresentationContract{
		RequestedDimensions: []RequestedAnswerDimension{
			{Label: "直接阻塞/唤醒关系", Role: RequestedAnswerDimensionCurrentKeyCode, Index: 1},
			{Label: "调度/资源背景", Role: RequestedAnswerDimensionEvidenceSource, Index: 2},
		},
	}

	got := CompileExploreLanePlan(rm, nil, presentation)
	if current := countExploreLaneOrigin(got, AnswerEvidenceOriginCurrentSource); current != 0 {
		t.Fatalf("excluded current-source lane must not be recreated from display dimensions, current_source=%d lanes=%+v", current, got.Lanes)
	}
	assertExploreLaneOrigin(t, got, AnswerEvidenceOriginRuntimeArtifact, "调度/资源背景")
}

func TestCompileExploreLanePlan_UserBucketsPartitionLanes(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsCrossComponent: true,
		},
		Buckets: []QuestionBucket{
			{Label: "codrax", Index: 1},
			{Label: "opencode", Index: 2},
		},
	}
	got := CompileExploreLanePlan(rm, nil, AnswerPresentationContract{})
	if len(got.Lanes) != 2 {
		t.Fatalf("bucketed request should produce one current-source lane per bucket, got %+v", got.Lanes)
	}
	if got.Lanes[0].InvestigationUnitID != "bucket-1" || got.Lanes[1].InvestigationUnitID != "bucket-2" {
		t.Fatalf("bucket unit ids not preserved: %+v", got.Lanes)
	}
	if got.Lanes[0].OwnershipKey() == got.Lanes[1].OwnershipKey() {
		t.Fatalf("bucket lanes must not overlap: %+v", got.Lanes)
	}
}

func TestCompileExploreLanePlan_CommandMeasurementAndCurrentSource(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsCountQuestion: true,
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes: []CurrentSourceExplanationMode{
				CurrentSourceExplanationExplainCurrentMechanism,
			},
			Confidence:   0.9,
			SourceQuotes: []string{"结合源码"},
		},
	}
	got := CompileExploreLanePlan(rm, nil, AnswerPresentationContract{})
	assertExploreLaneOrigin(t, got, AnswerEvidenceOriginCommandMeasurement, "")
	assertExploreLaneOrigin(t, got, AnswerEvidenceOriginCurrentSource, "")
	if len(got.Lanes) != 2 {
		t.Fatalf("lanes=%d want command+source: %+v", len(got.Lanes), got.Lanes)
	}
}

func TestCompileExploreLanePlan_MCPResourceCollapsesOnlyMCPLane(t *testing.T) {
	rm := RequestModel{
		RawRequest: "请使用 MCP fixture 工具查询 mcp://fixture/trace/sleep-wakeup 的 line 7 和 line 12",
		Intent:     IntentExplain,
		SubTopics: []SubTopic{
			{Summary: "line 7"},
			{Summary: "line 12"},
		},
	}
	got := CompileExploreLanePlan(rm, nil, AnswerPresentationContract{})
	if current := countExploreLaneOrigin(got, AnswerEvidenceOriginCurrentSource); current != 2 {
		t.Fatalf("default MCP resource request should keep per-topic source lanes, got current_source=%d lanes=%+v", current, got.Lanes)
	}
	if mcp := countExploreLaneOrigin(got, AnswerEvidenceOriginMCPResource); mcp != 1 {
		t.Fatalf("MCP resource origin should collapse duplicate per-topic reads, got mcp=%d lanes=%+v", mcp, got.Lanes)
	}
	if len(got.Lanes) != 3 {
		t.Fatalf("lanes=%d want 2 source lanes + 1 MCP lane: %+v", len(got.Lanes), got.Lanes)
	}
}

func TestCompileExploreLanePlan_MCPAndSourceKeepsBothOrigins(t *testing.T) {
	rm := RequestModel{
		RawRequest: "请使用 MCP fixture 工具查询 mcp://fixture/trace/sleep-wakeup，并结合源码解释实现",
		Intent:     IntentExplain,
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes: []CurrentSourceExplanationMode{
				CurrentSourceExplanationExplainCurrentMechanism,
			},
			Confidence:   0.9,
			SourceQuotes: []string{"结合源码"},
		},
	}
	got := CompileExploreLanePlan(rm, nil, AnswerPresentationContract{})
	assertExploreLaneOrigin(t, got, AnswerEvidenceOriginMCPResource, "")
	assertExploreLaneOrigin(t, got, AnswerEvidenceOriginCurrentSource, "")
}

func assertExploreLaneOrigin(t *testing.T, plan ExploreLanePlan, origin AnswerEvidenceOrigin, wantDim string) {
	t.Helper()
	for _, lane := range plan.Lanes {
		if lane.Origin != origin {
			continue
		}
		if wantDim != "" && !containsExploreLaneTestString(lane.DimensionLabels, wantDim) {
			t.Fatalf("lane %s missing dimension %q: %+v", origin, wantDim, lane)
		}
		if lane.HandoffPolicy != ExploreLaneHandoffOwn || lane.Role != ExploreLaneRolePrincipal {
			t.Fatalf("lane %s policy/role wrong: %+v", origin, lane)
		}
		return
	}
	t.Fatalf("missing lane origin %s in %+v", origin, plan.Lanes)
}

func countExploreLaneOrigin(plan ExploreLanePlan, origin AnswerEvidenceOrigin) int {
	count := 0
	for _, lane := range plan.Lanes {
		if lane.Origin == origin {
			count++
		}
	}
	return count
}

func containsExploreLaneTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
