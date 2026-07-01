package types

import "testing"

// TestIsScalarSourceLiteralLookup_RoleLocateBlockedByLogFrames pins
// the 2026-05-02 fix: a panic / crash question whose RequestModel has
// IsRoleLocateLookup=true must NOT short-circuit to scalar lane when
// the user attached a multi-frame LogBundle. The bundle is objective
// evidence that the answer surface is a multi-step mechanism, not a
// single source literal. Without this guard, the scenario reconciler
// rewrites root_cause → generic, the system telegraphs "scalar
// source-literal" via WARN, and the LLM self-corrupts to a scalar
// answer across multi-emit retries.
func TestIsScalarSourceLiteralLookup_RoleLocateBlockedByLogFrames(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentRootCause,
		Complexity:    ComplexityModerate,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsRoleLocateLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: "mechanism"},
		LogTriage: &LogBundle{
			Errors: []LogError{{
				Type: "panic",
				Frames: []LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
					{File: "internal/agent/analyzer.go", Line: 320, Func: "ParseOutput"},
				},
			}},
		},
	}
	if IsScalarSourceLiteralLookup(rm) {
		t.Fatal("role-locate short-circuit must NOT fire when 2+ log frames attached — bundle is objective evidence of multi-step mechanism")
	}
}

func TestHasAttributeBearingEnumeration_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
		},
		EnumerationBoundary: &RequestedEnumerationBoundary{
			DeclaredCount: 25,
			SourceQuote:   "all packages",
		},
	}
	if !HasAttributeBearingEnumeration(rm) {
		t.Fatal("category enumeration + relational lookup + boundary should be attribute-bearing")
	}

	rm.EnumerationBoundary = nil
	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "all packages"}
	if !HasAttributeBearingEnumeration(rm) {
		t.Fatal("category enumeration + relational lookup + completeness obligation should be attribute-bearing")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.PredicateAxis = AxisDefine
	rm.AnalyzerHints.Entities = []string{"aggregator", "compiler"}
	if !HasAttributeBearingEnumeration(rm) {
		t.Fatal("category enumeration + predicate axis + exhaustive multi-member entity set should be attribute-bearing")
	}

	rm.PredicateAxis = AxisUnknown
	rm.SubTopics = []SubTopic{{Summary: "members"}, {Summary: "attributes"}}
	if !HasAttributeBearingEnumeration(rm) {
		t.Fatal("category enumeration + subtopic split + exhaustive multi-member entity set should be attribute-bearing")
	}

	rm.CompletenessObligation = nil
	rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{
		{Path: "a.go", Confidence: 0.9},
		{Path: "b.go", Confidence: 0.9},
	}
	if !HasAttributeBearingEnumeration(rm) {
		t.Fatal("category enumeration + subtopic split + required file hints for multiple members should be attribute-bearing")
	}

	rm.AnalyzerHints.RequiredFileHints = nil
	rm.SubTopics = nil
	if HasAttributeBearingEnumeration(rm) {
		t.Fatal("plain enumeration must not be treated as attribute-bearing")
	}
}

func TestHasObservationOnlyRuntimeArtifact_DefaultKeepsCurrentSourceLane(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "timeout"}},
		},
		AnalyzerHints: AnalyzerHints{
			RequiredFileHints: []RequiredFileHint{{
				Path:       "internal/llm/openai.go",
				Confidence: 0.9,
				Rationale:  "pre-scan matched the current-source mechanism requested by the user",
			}},
		},
	}
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("external runtime artifact must not become observation-only by default")
	}

	rm.AnalyzerHints.RequiredFileHints = nil
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("external runtime artifact without source anchors still defaults to mixed external/source analysis")
	}

	rm.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode: ExternalObservationCurrentSourceExclude,
		ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"只分析日志"},
		Confidence:        0.9,
	}
	if !rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("anchored typed source exclusion should mark runtime artifact observation-only")
	}
}

func TestHasRuntimeArtifactPathReference_ExternalObservationQuoteEmbeddedPath(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode: ExternalObservationCurrentSourceExclude,
			ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
			SourceQuotes: []string{
				"只分析 ../customlogs/xxx_all.systrace 这个 trace 文件，不分析代码",
			},
		},
	}
	if !rm.HasRuntimeArtifactPathReference() {
		t.Fatal("runtime artifact path embedded in typed source quote should be recognized")
	}
	if got := rm.RuntimeArtifactPathReferenceKind(); got != "trace" {
		t.Fatalf("runtime artifact path kind = %q, want trace", got)
	}
}

func TestHasObservationOnlyRuntimeArtifact_CurrentKeyCodeDimensionOpensCurrentSourceLane(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "timeout"}},
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Confidence:          0.9,
			Dimensions: []RequestedAnswerDimension{{
				Label:    "当前关键代码",
				Role:     RequestedAnswerDimensionCurrentKeyCode,
				Required: true,
				Index:    1,
			}},
		},
	}
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("explicit current_key_code dimension should keep mixed runtime/current-source lane open")
	}
	rm.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
		ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
		ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
		SourceQuotes:         []string{"不要把日志行当成当前源码引用"},
		Confidence:           0.9,
	}
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("current_key_code dimension must outrank accidental source exclusion")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("current_key_code dimension should require current source even with exclusion drift, got %s", got)
	}

	rm.ExternalObservationPolicy = nil
	rm.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionImpact
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("non-current-source presentation dimensions do not suppress default current-source analysis")
	}
}

func TestHasObservationOnlyRuntimeArtifact_CurrentSourceExplanationProfileOpensLane(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "timeout"}},
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{"当前源码解释"},
			Confidence:                          0.9,
		},
	}
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("typed current-source explanation profile should keep mixed runtime/current-source lane open")
	}
	rm.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
		ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
		ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
		SourceQuotes:         []string{"不要把日志行当成当前源码引用"},
		Confidence:           0.9,
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("current-source explanation profile should require current source even with exclusion drift, got %s", got)
	}

	rm.ExternalObservationPolicy = nil
	rm.CurrentSourceExplanationProfile.SourceQuotes = nil
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatal("inactive current-source explanation profile must not suppress default current-source lane")
	}
}

func TestCurrentSourceLaneDecision_ExternalArtifactCoordinatesDoNotRequireSource(t *testing.T) {
	rm := RequestModel{
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			Confidence:           0.9,
		},
		AnalyzerHints: AnalyzerHints{
			ExactTargets: []string{"mcp://fixture/trace/sleep-wakeup"},
			Entities:     []string{"pid=4242"},
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationOther},
			SourceQuotes:                        []string{"mcp://fixture/trace/sleep-wakeup", "line 7", "line 12"},
			Confidence:                          0.9,
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{
				{Label: "line 7", Role: RequestedAnswerDimensionCurrentKeyCode, SourceQuote: "line 7", Required: true, Index: 1},
				{Label: "第12行", Role: RequestedAnswerDimensionCurrentKeyCode, SourceQuote: "第12行", Required: true, Index: 2},
			},
			Confidence: 0.9,
		},
	}
	if !rm.HasExternalObservationArtifactReference() {
		t.Fatal("external artifact coordinates should be recognized without attached log/trace bundles")
	}
	if rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		t.Fatal("external artifact coordinates must not become current-source anchors")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneAllowedOptional {
		t.Fatalf("CurrentSourceLaneDecision=%s, want allowed_optional", got)
	}

	rm.CurrentSourceExplanationProfile.SourceQuotes = append(rm.CurrentSourceExplanationProfile.SourceQuotes, "当前关键代码")
	if !rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		t.Fatal("a separate typed current-source quote should still open the current-source lane")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("CurrentSourceLaneDecision=%s, want required after current-source quote", got)
	}
}

func TestCurrentSourceLaneDecision_RuntimeArtifactPathReferencesDoNotRequireSource(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
			Confidence:           0.9,
		},
		AnalyzerHints: AnalyzerHints{
			RequiredFileHints: []RequiredFileHint{{
				Path:       "eval/fixtures/runtime_path_panic.log",
				Confidence: 0.8,
				Rationale:  "user named a runtime log path",
			}},
			MentionedEntities: []string{"/tmp/app_window.systrace"},
		},
	}
	if !rm.HasRuntimeArtifactPathReference() {
		t.Fatal("explicit log/trace paths should be recognized as runtime artifact path references")
	}
	if !rm.HasExternalObservationArtifactReference() {
		t.Fatal("runtime artifact paths should also count as external observation references")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneAllowedOptional {
		t.Fatalf("runtime artifact path references should keep source optional, got %s", got)
	}
	if !rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("runtime artifact path references should satisfy no-required-current-source")
	}
	if got := rm.RuntimeArtifactPathReferenceKind(); got != "log" {
		t.Fatalf("first runtime artifact path kind = %q, want log", got)
	}

	rm.AnalyzerHints.RequiredFileHints = append(rm.AnalyzerHints.RequiredFileHints, RequiredFileHint{
		Path:       "internal/agent/analyzer.go",
		Confidence: 0.9,
		Rationale:  "user also asked for current implementation",
	})
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("separate current-source path should require source, got %s", got)
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("current-source path must clear runtime-artifact-only completion")
	}
}

func TestCurrentSourceLaneDecision_RuntimeArtifactDefaultOptional(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "timeout"}},
		},
		AnalyzerHints: AnalyzerHints{
			ExactTargets: []string{
				"record_trace_20260526174055.systrace",
				"Choreographer#doFrame 989634",
			},
		},
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneAllowedOptional {
		t.Fatalf("runtime artifact with unresolved trace targets should be source-optional, got %s", got)
	}
	if !rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("source-optional runtime artifact should satisfy the no-required-current-source contract")
	}

	rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{
		Path:       "internal/llm/openai.go",
		Confidence: 0.9,
	}}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("current-source required_file hint should require source lane, got %s", got)
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("current-source required_file hint must clear the no-required-current-source contract")
	}

	rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{
		Path:       "record_trace_20260526174055.systrace",
		Confidence: 0.9,
	}}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneAllowedOptional {
		t.Fatalf("runtime artifact required_file hint must not require source lane, got %s", got)
	}
	if !rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("runtime-artifact-only hint should keep the no-required-current-source contract")
	}

	rm.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode: ExternalObservationCurrentSourceExclude,
		ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"只分析 trace"},
		Confidence:        0.9,
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneExcluded {
		t.Fatalf("explicit exclusion should exclude source lane, got %s", got)
	}
	if !rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("explicit source exclusion should also satisfy the no-required-current-source contract")
	}
}

func TestCurrentSourceLaneDecision_ExplicitAllowRequiresMixedCurrentSourceLane(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		LogTriage: &LogBundle{
			Observations: []LogObservation{{
				Kind:       LogObservationRuntimeEvent,
				Summary:    "finalizer attempt failed after a stream timeout",
				Confidence: 0.95,
			}},
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceAllow,
			Confidence:           0.95,
		},
	}
	if !rm.HasExternalOnlyRuntimeArtifact() {
		t.Fatal("fixture should be an external runtime artifact")
	}
	if !rm.ExternalObservationAllowsCurrentSource() {
		t.Fatal("fixture should expose explicit mixed-lane allow policy")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("explicit allow should require the mixed current-source lane, got %s", got)
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("explicit allow must not collapse into runtime-artifact-only completion")
	}
	if rm.HasRuntimeArtifactObservationOnlySurface() {
		t.Fatal("explicit allow must not render observation-only start guidance")
	}
}

func TestHasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		AnalyzerHints: AnalyzerHints{
			ExactTargets: []string{"com.baidu.tieba-59566", "34579.472865s"},
		},
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("plain request without runtime bundle should not look like a runtime artifact")
	}
	if !rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext(true) {
		t.Fatal("attached trace context should satisfy runtime source-optional boundary")
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext(false) {
		t.Fatal("without attached trace context, missing runtime bundle must remain false")
	}

	rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{
		Path:       "internal/agent/analyzer.go",
		Confidence: 0.9,
	}}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext(true) {
		t.Fatal("current-source required_file hint must keep current-source lane required even with attached trace")
	}

	rm.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{
		Path:       ".codrax/blob/20260614-011115-000-51200/attached_trace.txt",
		Confidence: 0.9,
	}}
	if !rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext(true) {
		t.Fatal("attached trace blob path must stay in runtime-artifact lane, not current-source lane")
	}
}

func TestRuntimeArtifactContextActiveFromBusCoversTraceQueryAndLog(t *testing.T) {
	mut := NewMutableState("trace runtime observation")
	mut.AppendDispatchToolResult(traceQueryRuntimeObservationToolResultForTest())
	if !RuntimeArtifactContextActiveFromBus(&BusContext{Mutable: mut}) {
		t.Fatal("typed trace_query runtime observations should activate runtime-artifact context")
	}
	if !RuntimeArtifactContextActiveFromBus(&BusContext{Mutable: NewMutableState("log"), AttachedLog: "panic: boom"}) {
		t.Fatal("attached log should activate runtime-artifact context")
	}
	if !RuntimeArtifactContextActiveFromBus(&BusContext{
		Mutable: NewMutableState("path trace"),
		RuntimeArtifactPreflight: NormalizeRuntimeArtifactPreflightProfile(RuntimeArtifactPreflightProfile{
			Active:                   true,
			SourceNavigationOptional: true,
			Artifacts: []RuntimeArtifactPreflightArtifact{{
				Kind:    "trace",
				Source:  "/tmp/frame.systrace",
				Carrier: "request_path",
			}},
		}),
	}) {
		t.Fatal("runtime artifact preflight should activate runtime-artifact context")
	}
	if RuntimeArtifactContextActiveFromBus(&BusContext{Mutable: NewMutableState("plain")}) {
		t.Fatal("plain repo turn must not activate runtime-artifact context")
	}
}

func TestHasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContextCoversLogs(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		AnalyzerHints: AnalyzerHints{
			ExactTargets: []string{"panic: boom"},
		},
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("plain request without a runtime artifact should not satisfy runtime boundary")
	}
	if !rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(true) {
		t.Fatal("attached runtime log context should satisfy source-optional runtime boundary")
	}
	rm.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
		SourceQuotes:                        []string{"current checkout implementation"},
		Confidence:                          0.9,
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(true) {
		t.Fatal("typed current-source request must keep current-source lane required")
	}
}

func TestCurrentSourceLaneDecision_ExternalArtifactSourceScopeRequiresSource(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentRootCause,
		Scenario:      ScenarioPerformanceBottleneck,
		PredicateAxis: AxisImplement,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:       "trace_mark",
				Subject:    "DoFrame",
				Summary:    "runtime trace span is janky",
				LineStart:  5,
				LineEnd:    6,
				DurationMs: 86.111,
			}},
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceAllow,
			Confidence:           0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeProduction,
			Confidence:     0.8,
		},
	}
	if !rm.HasExternalOnlyRuntimeArtifact() {
		t.Fatal("fixture should be an external runtime artifact")
	}
	if !rm.HasTypedCurrentSourceScopeRequest() {
		t.Fatal("typed production source scope on an external artifact should request current-source evidence")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("source-scoped external artifact should require current source, got %s", got)
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("source-scoped external artifact must not be treated as source-optional")
	}

	rm.ExternalObservationPolicy.CurrentSourceMode = ExternalObservationCurrentSourceExclude
	rm.ExternalObservationPolicy.ExclusionKind = ExternalObservationSourceExclusionExplicitUserBoundary
	rm.ExternalObservationPolicy.SourceQuotes = []string{"只分析 trace"}
	if rm.HasTypedCurrentSourceScopeRequest() {
		t.Fatal("explicit typed source exclusion must override source-scope drift")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneExcluded {
		t.Fatalf("explicit source exclusion should still exclude current source, got %s", got)
	}
}

func TestCurrentSourceLaneDecision_DefaultExternalArtifactCurrentKeyCodeRequiresSource(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:      "state_churn",
				Subject:   "app-20",
				Summary:   "runtime trace state_churn metrics",
				LineStart: 3,
				LineEnd:   23,
			}},
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
			Confidence:           0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeProduction,
			Confidence:     0.8,
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{{
				Label:    "dominant_state",
				Role:     RequestedAnswerDimensionCurrentKeyCode,
				Required: true,
				Index:    1,
			}},
			Confidence: 0.9,
		},
	}
	if !rm.HasTypedCurrentSourceScopeRequest() {
		t.Fatal("required current_key_code dimension plus typed source scope should request current-source evidence")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("required current_key_code dimension should require current source, got %s", got)
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("required current_key_code dimension must not be treated as source-optional")
	}
}

func TestCurrentSourceLaneDecision_RuntimeArtifactSourceScopeQuoteRequiresSource(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		LogTriage: &LogBundle{
			Observations: []LogObservation{{
				Kind:      LogObservationRetryCycle,
				Subject:   "finalizer timeout",
				Summary:   "runtime log shows retry after timeout",
				LineStart: 2,
			}},
		},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeAll,
			SourceQuotes:   []string{"请结合当前源码解释"},
			Confidence:     0.8,
		},
	}
	if !rm.HasTypedCurrentSourceScopeRequest() {
		t.Fatal("runtime artifact plus typed source-scope quote should request current-source evidence")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("source-scoped runtime artifact should require current source, got %s", got)
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("source-scoped runtime artifact must not be treated as source-optional")
	}

	rm.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode: ExternalObservationCurrentSourceExclude,
		ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"只分析日志"},
		Confidence:        0.9,
	}
	if rm.HasTypedCurrentSourceScopeRequest() {
		t.Fatal("explicit source exclusion must override typed source-scope quote")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneExcluded {
		t.Fatalf("explicit source exclusion should exclude current source, got %s", got)
	}
}

func TestCurrentSourceLaneDecision_DefaultExternalArtifactMetricDimensionStaysOptional(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:      "state_churn",
				Subject:   "app-20",
				Summary:   "runtime trace state_churn metrics",
				LineStart: 3,
				LineEnd:   23,
			}},
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
			Confidence:           0.9,
		},
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeProduction,
			Confidence:     0.8,
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{{
				Label:    "dominant_state",
				Role:     RequestedAnswerDimensionStageWorkflow,
				Required: true,
				Index:    1,
			}},
			Confidence: 0.9,
		},
	}
	if rm.HasTypedCurrentSourceScopeRequest() {
		t.Fatal("runtime metric dimensions should not hard-require current source under default source mode")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneAllowedOptional {
		t.Fatalf("runtime metric dimensions should keep source optional, got %s", got)
	}
	if !rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("runtime metric dimensions should stay source-optional")
	}
}

func TestRuntimeArtifactReadSourceNavigationNotRequired_AttachedTraceObservationOnly(t *testing.T) {
	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent:   IntentTrace,
		Scenario: ScenarioPerformanceBottleneck,
		AnalyzerHints: AnalyzerHints{
			PrimaryEntities: []string{"app-20"},
		},
	}}
	if !RuntimeArtifactReadSourceNavigationNotRequired(ir, true) {
		t.Fatal("attached trace without current-source requirement should not require source navigation")
	}
	if !RuntimeArtifactRequestSourceNavigationNotRequired(ir.RequestModel, true) {
		t.Fatal("pre-IR request helper should match IR helper for attached trace without current-source requirement")
	}
}

func TestRuntimeArtifactReadSourceNavigationNotRequired_CurrentKeyCodeKeepsSourceRequired(t *testing.T) {
	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeProduction,
			Confidence:     0.8,
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{{
				Label:    "current implementation",
				Role:     RequestedAnswerDimensionCurrentKeyCode,
				Required: true,
				Index:    1,
			}},
			Confidence: 0.9,
		},
	}}
	if RuntimeArtifactReadSourceNavigationNotRequired(ir, true) {
		t.Fatal("required current_key_code dimension must keep source navigation required")
	}
	if RuntimeArtifactRequestSourceNavigationNotRequired(ir.RequestModel, true) {
		t.Fatal("pre-IR request helper must preserve required current-source navigation")
	}
}

func TestRuntimeArtifactReadSourceSupplementsNotRequired_SourceOptionalTrace(t *testing.T) {
	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent:   IntentTrace,
		Scenario: ScenarioPerformanceBottleneck,
		AnalyzerHints: AnalyzerHints{
			PrimaryEntities: []string{"app-20"},
		},
	}}
	if !RuntimeArtifactReadSourceSupplementsNotRequired(ir, true) {
		t.Fatal("source-optional attached trace should keep source audit supplements off the user answer surface")
	}
}

func TestRuntimeArtifactReadSourceSupplementsNotRequired_CurrentKeyCodeKeepsSupplements(t *testing.T) {
	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		SourceScopeProfile: &SourceScopeProfile{
			RequestedScope: SourceScopeProduction,
			Confidence:     0.8,
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{{
				Label:    "current implementation",
				Role:     RequestedAnswerDimensionCurrentKeyCode,
				Required: true,
				Index:    1,
			}},
			Confidence: 0.9,
		},
	}}
	if RuntimeArtifactReadSourceSupplementsNotRequired(ir, true) {
		t.Fatal("required current_key_code dimension should preserve source audit supplements")
	}
}

func TestRuntimeArtifactReadSourceSupplementsNotRequiredForBusContext_SoftMixedWithSourceProofKeepsSupplements(t *testing.T) {
	mut := NewMutableState("runtime mixed")
	mut.SetLogTriage(&LogBundle{Errors: []LogError{{Type: "timeout"}}})
	ctx := &BusContext{
		Mutable: mut,
		TurnRouteHint: TurnRouteHint{
			Route:           "repo",
			Source:          "mixed",
			NeedsRepoAccess: true,
			Confidence:      0.9,
		},
		AnalysisIR: &AnalysisIR{RequestModel: RequestModel{
			Intent:    IntentRootCause,
			Scenario:  ScenarioRootCause,
			LogTriage: mut.LogTriage(),
			ExternalObservationPolicy: &ExternalObservationPolicy{
				ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
				Confidence:           0.9,
			},
		}},
		EvidenceItems: []EvidenceItem{{
			ID:        "source-timeout-owner",
			Source:    "internal/llm/openai.go",
			LineStart: 472,
			Summary:   "current source defines the timeout retry boundary",
			Origin:    ClaimOriginCurrentRepo,
		}},
	}

	if RuntimeArtifactReadSourceSupplementsNotRequired(ctx.AnalysisIR, true) == false {
		t.Fatal("legacy request-trait helper should still classify the request as source-optional")
	}
	if RuntimeArtifactReadSourceSupplementsNotRequiredForBusContext(ctx) {
		t.Fatal("accepted current-source proof should preserve final source audit supplements")
	}
	if RuntimeArtifactReadSourceNavigationNotRequiredForBusContext(ctx) {
		t.Fatal("accepted current-source proof should preserve final source navigation follow-up")
	}
}

func TestRuntimeArtifactReadSourceAuditForBusContext_SoftObligationUsesAuthorityBeforeLegacySignal(t *testing.T) {
	mut := NewMutableState("runtime mixed soft obligation")
	mut.AppendDispatchToolResult(traceQueryRuntimeObservationToolResultForTest())
	ctx := &BusContext{
		Mutable: mut,
		TurnRouteHint: TurnRouteHint{
			Route:           "repo",
			Source:          "mixed",
			NeedsRepoAccess: true,
			Confidence:      0.9,
		},
		AnalysisIR: &AnalysisIR{RequestModel: RequestModel{
			Intent:   IntentRootCause,
			Scenario: ScenarioPerformanceBottleneck,
			ExternalObservationPolicy: &ExternalObservationPolicy{
				ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
				Confidence:           0.9,
			},
			CurrentSourceObligationSignals: []CurrentSourceObligationSignal{{
				Kind:  CurrentSourceObligationSignalDroppedRequestedDimension,
				Role:  RequestedAnswerDimensionFunctionOrPurpose,
				Index: 1,
			}},
		}},
	}

	if !ctx.AnalysisIR.RequestModel.HasCurrentSourceObligationSignal() {
		t.Fatal("test setup must expose the legacy typed obligation signal")
	}
	authority := BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, ObservationLedger{})
	if authority.CurrentSourceRequirement != RuntimeSourceRequirementSoft ||
		authority.CanHardBlockCompletion ||
		!authority.CanDowngradeToCaveat {
		t.Fatalf("soft dropped-dimension obligation should be caveatable, got %+v", authority)
	}
	if !RuntimeArtifactReadSourceSupplementsNotRequiredForBusContext(ctx) {
		t.Fatal("soft missing current-source obligation with accepted runtime proof should suppress audit supplements as caveat")
	}
	if !RuntimeArtifactReadSourceNavigationNotRequiredForBusContext(ctx) {
		t.Fatal("soft missing current-source obligation with accepted runtime proof should suppress navigation follow-up as caveat")
	}
}

func TestRuntimeArtifactReadSourceAuditForBusContext_PathAnchoredObligationKeepsAudit(t *testing.T) {
	mut := NewMutableState("runtime mixed precise obligation")
	mut.AppendDispatchToolResult(traceQueryRuntimeObservationToolResultForTest())
	ctx := &BusContext{
		Mutable: mut,
		TurnRouteHint: TurnRouteHint{
			Route:           "repo",
			Source:          "mixed",
			NeedsRepoAccess: true,
			Confidence:      0.9,
		},
		AnalysisIR: &AnalysisIR{RequestModel: RequestModel{
			Intent:   IntentTrace,
			Scenario: ScenarioPerformanceBottleneck,
			RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []RequestedAnswerDimension{{
					Label:       "current implementation",
					Role:        RequestedAnswerDimensionCurrentKeyCode,
					SourceQuote: "internal/tracequery/parse.go:42",
					Required:    true,
					Index:       1,
				}},
				Confidence: 0.9,
			},
		}},
	}

	authority := BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, ObservationLedger{})
	if authority.CurrentSourceRequirement != RuntimeSourceRequirementPrecise ||
		!authority.CanHardBlockCompletion ||
		authority.CanDowngradeToCaveat {
		t.Fatalf("path-anchored current-key-code obligation should remain precise, got %+v", authority)
	}
	if RuntimeArtifactReadSourceSupplementsNotRequiredForBusContext(ctx) {
		t.Fatal("precise current-source obligation should preserve audit supplements")
	}
	if RuntimeArtifactReadSourceNavigationNotRequiredForBusContext(ctx) {
		t.Fatal("precise current-source obligation should preserve navigation follow-up")
	}
}

func TestCurrentSourceLaneDecision_RuntimeExactTargetsRemainSourceOptional(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{Kind: "state_churn", Subject: "app-20"}},
		},
		AnalyzerHints: AnalyzerHints{
			ExactTargets: []string{"app-20", "11.0s-11.008s"},
		},
		DiagnosticProfile: DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentRisk:         true,
			CurrentVersionCheck: false,
			Confidence:          0.95,
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
			Confidence:           0.95,
		},
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneAllowedOptional {
		t.Fatalf("runtime exact targets should keep source optional, got %s", got)
	}
	if !rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("runtime exact targets alone must not hard-require current source")
	}
}

func TestCurrentSourceLaneDecision_RuntimeDiagnosticMechanismBridgeRequiresSource(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		LogTriage: &LogBundle{
			Observations: []LogObservation{{
				Kind:      LogObservationRetryCycle,
				Summary:   "finalizer retry after stream timeout",
				LineStart: 2,
			}},
		},
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
			IsCrossComponent:     true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentVersionCheck: true,
			Confidence:          0.9,
		},
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("typed diagnostic mechanism bridge should require source, got %s", got)
	}
	if !rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		t.Fatal("typed diagnostic mechanism bridge should be a current-source anchor")
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("typed diagnostic mechanism bridge must not be observation-only")
	}

	rm.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode: ExternalObservationCurrentSourceExclude,
		ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"只分析日志"},
		Confidence:        0.9,
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneExcluded {
		t.Fatalf("explicit typed source exclusion should override diagnostic bridge, got %s", got)
	}
}

func TestCurrentSourceLaneDecision_CurrentSourceProfileRequiresSource(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioRootCause,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "timeout"}},
		},
		AnalyzerHints: AnalyzerHints{
			ExactTargets: []string{"writeSession"},
		},
		DiagnosticProfile: DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentRisk:         true,
			CurrentVersionCheck: true,
			Confidence:          0.95,
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationVerifyCurrentStatus},
			SourceQuotes:                        []string{"current checkout"},
			Confidence:                          0.95,
		},
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("typed current-source profile should require source lane, got %s", got)
	}
}

func TestCurrentSourceLaneDecision_RuntimeMechanismDimensionRequiresSource(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:       "trace_mark",
				Subject:    "H:RenderService:DoFrame",
				Summary:    "runtime trace span is janky",
				LineStart:  5,
				LineEnd:    6,
				DurationMs: 86.111,
			}},
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
			Confidence:           0.9,
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{{
				Label:       "span 解析机制",
				Role:        RequestedAnswerDimensionFunctionOrPurpose,
				SourceQuote: "系统如何解析 trace span",
				Required:    true,
				Index:       1,
			}},
			Confidence: 0.9,
		},
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("required runtime mechanism dimension should require current source, got %s", got)
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("required runtime mechanism dimension must not collapse into runtime-observation-only completion")
	}

	rm.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionBoundary
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneAllowedOptional {
		t.Fatalf("runtime evidence-boundary dimension should remain source-optional, got %s", got)
	}
}

func TestCurrentSourceLaneDecision_DroppedRuntimeMechanismDimensionRequiresSource(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:       "trace_mark",
				Subject:    "H:RenderService:DoFrame",
				Summary:    "runtime trace span is janky",
				LineStart:  5,
				LineEnd:    6,
				DurationMs: 86.111,
			}},
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
			Confidence:           0.9,
		},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []RequestedAnswerDimension{{
				Label:    "evidence boundary",
				Role:     RequestedAnswerDimensionBoundary,
				Required: true,
				Index:    3,
			}},
			Confidence: 0.9,
		},
		CurrentSourceObligationSignals: []CurrentSourceObligationSignal{{
			Kind:  CurrentSourceObligationSignalDroppedRequestedDimension,
			Role:  RequestedAnswerDimensionCurrentKeyCode,
			Index: 2,
		}},
	}
	if !rm.HasCurrentSourceObligationSignal() {
		t.Fatal("dropped current-source dimension should expose typed obligation signal")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("dropped current-source obligation should require source lane, got %s", got)
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("dropped current-source obligation must not collapse into runtime-observation-only completion")
	}

	rm.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode: ExternalObservationCurrentSourceExclude,
		ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"explicit source exclusion"},
		Confidence:        0.9,
	}
	if rm.HasCurrentSourceObligationSignal() {
		t.Fatal("explicit typed current-source exclusion should suppress the obligation signal")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneExcluded {
		t.Fatalf("explicit current-source exclusion should still win, got %s", got)
	}
}

func TestHasBoundedCategoryEnumerationMembers_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"aggregator", "compiler"},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all subpackages"},
	}
	if !HasBoundedCategoryEnumerationMembers(rm) {
		t.Fatal("category enumeration + multi-member completeness should expose a bounded member lane")
	}

	rm.Predicates.IsRelationalLookup = true
	if HasBoundedCategoryEnumerationMembers(rm) {
		t.Fatal("relational lookup entities are not a plain principal-member lane")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.Predicates.IsCategoryEnumeration = false
	if HasBoundedCategoryEnumerationMembers(rm) {
		t.Fatal("non-enumeration request must not expose bounded enumeration members")
	}
}

func TestRequiresExhaustiveEnumerationMemberSetHandoff_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"Intent", "QuestionFamily", "Scenario"},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all public enum types"},
	}
	if !RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		t.Fatal("exhaustive category enumeration should require a structured member_set handoff")
	}

	rm.CompletenessObligation = nil
	rm.EnumerationBoundary = &RequestedEnumerationBoundary{DeclaredCount: 3, SourceQuote: "three public enum types"}
	if !RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		t.Fatal("declared-count enumeration should require a structured member_set handoff")
	}

	rm.EnumerationBoundary = nil
	rm.Predicates.IsRelationalLookup = true
	if !RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		t.Fatal("set-valued relational enumeration should require structured member_set handoff")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.EnumerationBoundary = nil
	rm.CompletenessObligation = nil
	rm.AnalyzerHints.Entities = []string{"aggregator", "compiler"}
	if !RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		t.Fatal("typed non-relation category member lane should require structured member_set handoff even without a count quote")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.Predicates.IsScalarAnswer = true
	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "all"}
	if RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		t.Fatal("scalar contradiction must keep the request out of exhaustive member-set handoff")
	}

	role := RequestModel{
		Intent: IntentReturnValue,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
			IsRoleLocateLookup:    true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqReturnValue)},
	}
	if RequiresExhaustiveEnumerationMemberSetHandoff(role) {
		t.Fatal("scalar role-locate relation lookup must not require member_set handoff")
	}

	arch := RequestModel{
		Intent:      IntentExplain,
		Scenario:    ScenarioArchitectureExplain,
		Complexity:  ComplexityComplex,
		Predicates:  SemanticPredicates{IsCategoryEnumeration: true, IsCrossComponent: true},
		DiagramHint: &DiagramHint{Kind: DiagramSequence},
		SubTopics:   []SubTopic{{Summary: "A"}, {Summary: "B"}},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"Analyzer", "Explorer", "Finalizer"},
		},
	}
	if RequiresExhaustiveEnumerationMemberSetHandoff(arch) {
		t.Fatal("architecture narrative component names are context, not an exhaustive principal member_set")
	}
}

func TestRequiresRelationMemberSetHandoff_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsRelationalLookup:    true,
			IsCategoryEnumeration: true,
		},
	}
	if !RequiresRelationMemberSetHandoff(rm) {
		t.Fatal("set-valued relation lookup should require a structured relation member_set handoff")
	}

	rm.Predicates.IsCategoryEnumeration = false
	rm.Predicates.IsCountQuestion = true
	if !RequiresRelationMemberSetHandoff(rm) {
		t.Fatal("relation count lookup should require a structured member_set so the count has a verified member basis")
	}

	rm.Predicates.IsCountQuestion = false
	rm.Predicates.IsRoleLocateLookup = true
	if RequiresRelationMemberSetHandoff(rm) {
		t.Fatal("scalar role-location relation must not require a member_set")
	}

	rm.Predicates.IsRoleLocateLookup = false
	rm.Intent = IntentExplain
	if RequiresRelationMemberSetHandoff(rm) {
		t.Fatal("mechanism-only relation explanation must not require a qualifying-member set")
	}
}

func TestRequiresSourceOperationSiteMemberSetHandoff_TypedBoundary(t *testing.T) {
	cgroup := RequestModel{
		Intent: IntentRootCause,
		Predicates: SemanticPredicates{
			HasPerMemberTable: true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqMechanism),
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction, AnswerCandidateRoleMethod},
		},
	}
	if !RequiresSourceOperationSiteMemberSetHandoff(cgroup) {
		t.Fatal("typed operation-site set request should require source operation-site member_set handoff")
	}

	callSites := RequestModel{
		Intent:        IntentEnumerate,
		PredicateAxis: AxisRegister,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqRegistration),
		},
	}
	if !RequiresSourceOperationSiteMemberSetHandoff(callSites) {
		t.Fatal("typed registration site set should require source operation-site handoff")
	}

	narrative := cgroup
	narrative.Predicates.HasPerMemberTable = false
	narrative.Intent = IntentExplain
	if RequiresSourceOperationSiteMemberSetHandoff(narrative) {
		t.Fatal("ordinary mechanism narrative without a typed set boundary must not require member_set handoff")
	}

	untypedSurface := cgroup
	untypedSurface.SourceInventoryProfile = nil
	untypedSurface.PredicateAxis = AxisUnknown
	untypedSurface.AnalyzerHints.Kind = string(ReqMechanism)
	if RequiresSourceOperationSiteMemberSetHandoff(untypedSurface) {
		t.Fatal("set boundary without typed operation-site surface must not require member_set handoff")
	}

	scalar := cgroup
	scalar.Predicates.IsScalarAnswer = true
	if RequiresSourceOperationSiteMemberSetHandoff(scalar) {
		t.Fatal("scalar lookup must not activate operation-site member_set handoff")
	}

	arch := cgroup
	arch.Intent = IntentExplain
	arch.Scenario = ScenarioArchitectureExplain
	arch.Complexity = ComplexityComplex
	arch.Predicates.IsCrossComponent = true
	if RequiresSourceOperationSiteMemberSetHandoff(arch) {
		t.Fatal("architecture narrative must not be rerouted by generic entry-point wording")
	}
}

func TestHistoryLookupPrefersVCSNarrativePrincipal_TypedBoundary(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioGeneric,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		SubTopics: []SubTopic{
			{Summary: "commit topic A", Entities: []string{"ScalarAnswer"}},
			{Summary: "commit topic B", Entities: []string{"AggregateScalar"}},
		},
	}
	if !HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("non-scalar history narrative should keep VCS metadata as the principal lane despite analyzer subtopics")
	}

	rm.Scenario = ScenarioArchitectureExplain
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqHistory)}
	if !HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("history evolution questions may be architecture_explain while still being VCS-principal")
	}
	rm.Scenario = ScenarioGeneric
	rm.AnalyzerHints = AnalyzerHints{}

	rm.Intent = IntentEnumerate
	if !HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("pure recent-N commit enumeration must remain VCS-principal and must not force current-source reads")
	}
	rm.Intent = IntentExplain
	rm.Predicates.IsCategoryEnumeration = true
	if !HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("pure commit category enumeration is still VCS metadata, not current source evidence")
	}
	rm.Predicates.IsCategoryEnumeration = false

	rm.Predicates.IsCrossComponent = true
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqHistory)}
	rm.Buckets = []QuestionBucket{{Label: "commit A", Index: 1}, {Label: "commit B", Index: 2}}
	if !HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("pure typed commit-history comparison should remain VCS-principal instead of forcing current-source reads")
	}
	rm.Buckets = nil
	rm.Predicates.IsCrossComponent = false
	rm.AnalyzerHints = AnalyzerHints{}

	currentSourceContract := &AnswerContract{Diagram: &DiagramContract{Required: true, RequiredKind: DiagramFlow}}
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, currentSourceContract) {
		t.Fatal("answer contract current_source origin must keep mixed history/current-source lane")
	}

	rm.Predicates.IsScalarAnswer = true
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("true scalar history lookup must not enter narrative-only lane")
	}
	rm.Predicates.IsScalarAnswer = false

	rm.DiagramHint = &DiagramHint{Kind: DiagramFlow}
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("history + diagram/code-flow request needs mixed history/current-source evidence")
	}
	rm.DiagramHint = nil

	contract := &AnswerContract{Diagram: &DiagramContract{Required: true, RequiredKind: DiagramFlow}}
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, contract) {
		t.Fatal("required diagram contract must keep current-source evidence principal")
	}

	rm.ChangeImpactProfile = &ChangeImpactProfile{IsChangeImpact: true}
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("history + change-impact request needs current-source principal evidence")
	}
	rm.ChangeImpactProfile = nil

	rm.DiagnosticProfile = DiagnosticIntentProfile{IsDiagnostic: true, CurrentVersionCheck: true}
	if HistoryLookupPrefersVCSNarrativePrincipal(rm, nil) {
		t.Fatal("history + current diagnostic check needs mixed evidence")
	}
}

func TestIsHistoryBackedCurrentCodeExplanation_TypedBoundary(t *testing.T) {
	rm := RequestModel{
		RawRequest:    "从历史提交里找一次相关改动，再结合当前代码解释现在链路怎么工作",
		Intent:        IntentExplain,
		Scenario:      ScenarioArchitectureExplain,
		Complexity:    ComplexityComplex,
		PredicateAxis: AxisDefine,
		Predicates: SemanticPredicates{
			IsHistoryLookup:  true,
			IsCrossComponent: true,
		},
		AnalyzerHints:       AnalyzerHints{Kind: string(ReqMechanism)},
		EnumerationBoundary: &RequestedEnumerationBoundary{DeclaredCount: 1, SourceQuote: "一次"},
	}
	if !IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("history + current-code mechanism explanation should keep a mixed narrative lane")
	}

	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqHistory)}
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("pure history evolution request must not be treated as current-code mixed analysis just because scenario=architecture_explain")
	}
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqMechanism)}

	rm.Intent = IntentTrace
	if !IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("history + current-code mechanism trace with define axis should still use mixed narrative lane")
	}
	rm.PredicateAxis = AxisCall
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqCallChain)}
	if !IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("history + current-code explanation misclassified as call_chain should still use mixed narrative lane when no explicit endpoints exist")
	}
	rm.AnalyzerHints.ExactTargets = []string{"A", "B"}
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("true call-chain trace with explicit endpoints must keep call-chain semantics")
	}
	rm.PredicateAxis = AxisDefine
	rm.AnalyzerHints = AnalyzerHints{Kind: string(ReqMechanism)}
	rm.Intent = IntentExplain

	rm.Intent = IntentEnumerate
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("explicit history enumeration/list request must not be treated as narrative-only")
	}
	rm.Intent = IntentExplain

	rm.Buckets = []QuestionBucket{{Label: "commit A", Index: 1}, {Label: "commit B", Index: 2}}
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("explicit bucketed history comparison should stay in comparison lane")
	}
	rm.Buckets = nil

	rm.Predicates.IsScalarAnswer = true
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("scalar history lookup must not become mixed current-code narrative")
	}
	rm.Predicates.IsScalarAnswer = false

	rm.Intent = IntentEnumerate
	rm.Predicates.IsCategoryEnumeration = true
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("explicit history enumeration/list request must not be treated as narrative-only")
	}
	rm.Intent = IntentExplain
	if !IsHistoryBackedCurrentCodeExplanation(rm) {
		t.Fatal("category-enumeration drift must not override history + current-code mechanism explanation")
	}
}

func TestIsCategoryEnumerationAnswerShape_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
	}
	if !IsCategoryEnumerationAnswerShape(rm) {
		t.Fatal("explicit category-enumeration predicate should mark set-valued answer shape")
	}

	rm = RequestModel{Intent: IntentEnumerate}
	if !IsCategoryEnumerationAnswerShape(rm) {
		t.Fatal("enumerate intent without scalar contradiction should mark set-valued answer shape")
	}

	rm.Predicates.IsScalarAnswer = true
	if IsCategoryEnumerationAnswerShape(rm) {
		t.Fatal("scalar answer predicate must keep enumerate intent out of set-valued answer shape")
	}

	rm.Predicates.IsScalarAnswer = false
	rm.Predicates.IsRoleLocateLookup = true
	if IsCategoryEnumerationAnswerShape(rm) {
		t.Fatal("role-locate predicate must keep enumerate intent out of set-valued answer shape")
	}

	rm.Predicates.IsRoleLocateLookup = false
	rm.Predicates.IsCountQuestion = true
	if IsCategoryEnumerationAnswerShape(rm) {
		t.Fatal("count predicate must keep enumerate intent out of set-valued answer shape")
	}
}

func TestCanUseAnalyzerEntitiesAsHardPrincipalMembers_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore"},
		},
	}
	if !CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("plain category enumeration entities should be eligible as hard principal members")
	}

	rm.CompletenessObligation = &CompletenessObligation{
		Required:    true,
		SourceQuote: "all members",
	}
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("exhaustive enumeration entities are search hints until exploration emits the verified member set")
	}
	rm.CompletenessObligation = nil

	rm.EnumerationBoundary = &RequestedEnumerationBoundary{
		DeclaredCount: 2,
		SourceQuote:   "2 members",
	}
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("declared-count enumeration entities must not seed hard members before exploration")
	}
	rm.EnumerationBoundary = nil

	rm.Predicates.IsRelationalLookup = true
	rm.EnumerationBoundary = &RequestedEnumerationBoundary{
		DeclaredCount: 2,
		SourceQuote:   "2 agents",
	}
	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "all agents"}
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("relation-shaped enumeration entities are mixed search hints, not a hard principal-member lane")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.Predicates.IsCategoryEnumeration = false
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("non-enumeration entities must not seed principal-member obligations")
	}

	rm.Predicates.IsCategoryEnumeration = true
	rm.AnalyzerHints.Entities = nil
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("empty entity list must not seed principal-member obligations")
	}
}

func TestHasPrincipalCategoryEnumerationMemberLane_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqEnumeration),
			Entities: []string{"aggregator", "compiler"},
		},
	}
	if !HasPrincipalCategoryEnumerationMemberLane(rm) {
		t.Fatal("plain typed category enumeration with multiple emitted members should expose a principal member lane")
	}

	rm.Predicates.IsRelationalLookup = true
	if HasPrincipalCategoryEnumerationMemberLane(rm) {
		t.Fatal("relation lookup entities are mixed targets/helpers, not a principal member lane")
	}

	rm.Predicates.IsRelationalLookup = false
	rm.Intent = IntentExplain
	rm.AnalyzerHints.Kind = string(ReqMechanism)
	if HasPrincipalCategoryEnumerationMemberLane(rm) {
		t.Fatal("non-enumerate/non-enumeration-kind category drift must not expose a hard member lane")
	}
}

func TestStructuralRelationScopeCandidates_UsesProvenanceLanes(t *testing.T) {
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"UserFacingInterface"},
			ExactTargets:      []string{"ExactFallback"},
			PrimaryEntities:   []string{"PrimaryInterface"},
			Entities:          []string{"UserFacingInterface", "ContextHelper", "SymbolResolver"},
			DerivedEntities:   []string{"ContextHelper", "SymbolResolver"},
		},
	}
	got := StructuralRelationScopeCandidates(rm)
	want := []string{"UserFacingInterface", "ExactFallback", "PrimaryInterface"}
	if len(got) != len(want) {
		t.Fatalf("candidates = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidates = %+v, want %+v", got, want)
		}
	}
}

func TestStructuralRelationScopeCandidates_LegacyEntitiesFallback(t *testing.T) {
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"Looper", "Looper", "Other"},
		},
	}
	got := StructuralRelationScopeCandidates(rm)
	want := []string{"Looper", "Other"}
	if len(got) != len(want) {
		t.Fatalf("legacy candidates = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("legacy candidates = %+v, want %+v", got, want)
		}
	}
}

func TestStructuralRelationCoverageScopeCandidates_NoBroadEntitiesFallback(t *testing.T) {
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"Agent", "SubAgent", "RuntimeHelper"},
		},
	}
	if got := StructuralRelationCoverageScopeCandidates(rm); len(got) != 0 {
		t.Fatalf("coverage candidates must ignore broad entities fallback, got %+v", got)
	}

	rm.AnalyzerHints.MentionedEntities = []string{"SubAgent"}
	rm.AnalyzerHints.ExactTargets = []string{"SubAgentRuntime.Run"}
	rm.AnalyzerHints.PrimaryEntities = []string{"Orchestrator"}
	got := StructuralRelationCoverageScopeCandidates(rm)
	want := []string{"SubAgent", "SubAgentRuntime.Run", "Orchestrator"}
	if len(got) != len(want) {
		t.Fatalf("coverage candidates = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("coverage candidates = %+v, want %+v", got, want)
		}
	}
}

func TestShouldSurfaceTypedRelationHints_ImplementAxisCoversDiagramMechanism(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Scenario:      ScenarioArchitectureExplain,
		PredicateAxis: AxisImplement,
		DiagramHint:   &DiagramHint{Kind: DiagramArchitecture},
	}
	if !ShouldSurfaceTypedRelationHints(rm) {
		t.Fatal("predicate_axis=implement should surface typed relation facts even outside enumeration")
	}
	rm.PredicateAxis = AxisUnknown
	if ShouldSurfaceTypedRelationHints(rm) {
		t.Fatal("plain architecture diagram without a typed relation axis must not surface relation hints")
	}
	rm.Predicates.IsRelationalLookup = true
	if !ShouldSurfaceTypedRelationHints(rm) {
		t.Fatal("relational lookup should continue surfacing typed relation facts")
	}
}

func TestShouldSurfaceTypedRelationHints_InterfaceDiagramShapeCoversAxisDrift(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Scenario:      ScenarioArchitectureExplain,
		PredicateAxis: AxisDefine,
		AnswerSubject: AnswerSubject{Kind: SubjectInterface},
		DiagramHint:   &DiagramHint{Kind: DiagramArchitecture},
	}
	if !ShouldSurfaceTypedRelationHints(rm) {
		t.Fatal("interface architecture diagram should surface typed relation facts even if predicate_axis drifted to define")
	}
	if !HasTypedRelationMemberSetShape(rm) {
		t.Fatal("interface diagram relation member sets must be protected from source-inventory rewrite")
	}
	rm.DiagramHint = nil
	if ShouldSurfaceTypedRelationHints(rm) {
		t.Fatal("interface subject alone is not enough to surface relation hints")
	}
}

func TestHasTypedRelationMemberSetShape_TypedOnly(t *testing.T) {
	rm := RequestModel{PredicateAxis: AxisImplement}
	if !HasTypedRelationMemberSetShape(rm) {
		t.Fatal("predicate_axis=implement should mark principal member sets as relation membership")
	}
	rm.PredicateAxis = AxisUnknown
	if HasTypedRelationMemberSetShape(rm) {
		t.Fatal("unknown axis without relational predicate must not mark source inventory as relation membership")
	}
	rm.Predicates.IsRelationalLookup = true
	if !HasTypedRelationMemberSetShape(rm) {
		t.Fatal("relational lookup predicate should mark principal member sets as relation membership")
	}
}

func TestSourceInventoryProfileConflictsWithRelationFlow_TypedBoundary(t *testing.T) {
	profile := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation, SourceInventoryFieldSummary},
	}
	rm := RequestModel{
		Intent:                 IntentTrace,
		PredicateAxis:          AxisCall,
		AnalyzerHints:          AnalyzerHints{Kind: string(ReqCallChain)},
		SourceInventoryProfile: profile,
	}
	if !SourceInventoryProfileConflictsWithRelationFlow(rm) {
		t.Fatal("call-chain trace should not carry source inventory as the principal answer shape")
	}

	rm.Predicates.IsCategoryEnumeration = true
	if SourceInventoryProfileConflictsWithRelationFlow(rm) {
		t.Fatal("true category enumeration should preserve source inventory even when relation-like axes are present")
	}
	rm.Predicates.IsCategoryEnumeration = false
	rm.Predicates.IsCountQuestion = true
	if SourceInventoryProfileConflictsWithRelationFlow(rm) {
		t.Fatal("true source-inventory count should preserve source inventory")
	}
	rm.Predicates.IsCountQuestion = false
	rm.Predicates.IsRelationalLookup = true
	if SourceInventoryProfileConflictsWithRelationFlow(rm) {
		t.Fatal("true relational member lookup should preserve source inventory for later relation handling")
	}
}

func TestSourceInventoryProfileConflictsWithRoleBinding_TypedBoundary(t *testing.T) {
	profile := &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
		RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation},
	}
	rm := RequestModel{
		Intent:        IntentEnumerate,
		PredicateAxis: AxisRegister,
		Predicates:    SemanticPredicates{IsCategoryEnumeration: true},
		AnswerRoleProfile: &AnswerRoleProfile{
			IsRoleBindingRequested: true,
			RequiredCandidateRoles: []AnswerCandidateRole{AnswerCandidateRoleType},
		},
		SourceInventoryProfile: profile,
	}
	if !SourceInventoryProfileConflictsWithRoleBinding(rm) {
		t.Fatal("registry/binding member-set answer must not use source inventory as completion authority")
	}
	if !SourceInventoryCompletionIsSupportOnly(rm) {
		t.Fatal("same registry/binding shape should make completion support-only")
	}

	rm.PredicateAxis = AxisDefine
	rm.AnswerRoleProfile = nil
	if SourceInventoryProfileConflictsWithRoleBinding(rm) {
		t.Fatal("plain source definition inventory should keep source inventory authority")
	}
	rm.PredicateAxis = AxisUnknown
	rm.AnswerRoleProfile = &AnswerRoleProfile{
		IsRoleBindingRequested: true,
		RequiredCandidateRoles: []AnswerCandidateRole{AnswerCandidateRoleType},
	}
	if SourceInventoryProfileConflictsWithRoleBinding(rm) {
		t.Fatal("precise source-inventory declaration lane should override noisy answer-role binding profile when predicate_axis is missing")
	}
	if !SourceInventoryPrincipalNavigationActive(rm) {
		t.Fatal("precise source-inventory declaration lane should remain the principal navigation surface")
	}
	rm.PredicateAxis = AxisRegister
	rm.AnswerRoleProfile = nil
	if !SourceInventoryProfileConflictsWithRoleBinding(rm) {
		t.Fatal("profile-aware check must not depend on answer_role_profile for register-axis category answers")
	}
	if !SourceInventoryLaneConflictsWithRoleBinding(rm) {
		t.Fatal("register-axis category enumeration must not depend on answer_role_profile to demote source inventory authority")
	}
	rm.Predicates = SemanticPredicates{}
	rm.CompletenessObligation = nil
	rm.Intent = IntentExplain
	if SourceInventoryLaneConflictsWithRoleBinding(rm) {
		t.Fatal("register axis alone without an enumerable answer shape must not drop source inventory authority")
	}
}

func TestSourceInventoryPrincipalAuthorityRequiresTypedPrecision(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCountQuestion:       true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqEnumeration)},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation, SourceInventoryFieldSummary},
			Confidence:        0.7,
		},
	}
	if SourceInventoryPrincipalAuthorityActive(rm) {
		t.Fatal("plain unscoped type/function inventory profile must stay advisory")
	}

	rm.SourceScopeProfile = &SourceScopeProfile{RequestedScope: SourceScopeAll}
	if !SourceInventoryPrincipalAuthorityActive(rm) {
		t.Fatal("explicit typed source scope should make source inventory principal")
	}

	rm.SourceScopeProfile = nil
	rm.SourceInventoryProfile.TypeUnderlying = SourceInventoryTypeUnderlyingString
	if !SourceInventoryPrincipalAuthorityActive(rm) {
		t.Fatal("structural type-underlying facet should make source inventory principal")
	}

	rm.SourceInventoryProfile.TypeUnderlying = SourceInventoryTypeUnderlyingUnknown
	rm.SourceInventoryProfile.RequiresConstSet = true
	if !SourceInventoryPrincipalAuthorityActive(rm) {
		t.Fatal("const-set facet should make source inventory principal")
	}
}

func TestSourceInventoryLaneConflictsWithRelationFlow_BeforeProfileSynthesis(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentTrace,
		PredicateAxis: AxisCall,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqCallChain)},
	}
	if !SourceInventoryLaneConflictsWithRelationFlow(rm) {
		t.Fatal("call-chain trace should block synthesized source inventory before a profile exists")
	}

	rm.Intent = IntentEnumerate
	rm.PredicateAxis = AxisDefine
	rm.AnalyzerHints.Kind = string(ReqEnumeration)
	rm.SourceScopeProfile = &SourceScopeProfile{
		RequestedScope: SourceScopeAll,
		Confidence:     0.9,
	}
	if SourceInventoryLaneConflictsWithRelationFlow(rm) {
		t.Fatal("source-scope-backed enumeration should allow synthesized source inventory")
	}
}

func TestArchitectureNarrativeExplanation_TypedBoundary(t *testing.T) {
	rm := RequestModel{
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Complexity: ComplexityComplex,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      true,
		},
		DiagramHint: &DiagramHint{Kind: DiagramSequence},
		SubTopics: []SubTopic{
			{Summary: "调度机制", Entities: []string{"Orchestrator", "PipelineStage"}},
			{Summary: "Agent职责", Entities: []string{"AnalyzerAgent", "ExplorerAgent"}},
			{Summary: "上下文传播", Entities: []string{"BusContext", "AgentContext"}},
		},
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"Orchestrator", "AnalyzerAgent", "ExplorerAgent", "AgentContext"},
		},
	}
	if !IsArchitectureNarrativeExplanation(rm) {
		t.Fatal("architecture view + diagram + subtopics should be recognized as narrative, not a member slate")
	}
	if CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) {
		t.Fatal("architecture narrative entities must remain soft hints even when category drift is present")
	}
	if HasBoundedCategoryEnumerationMembers(rm) {
		t.Fatal("architecture narrative must not expose a bounded principal-member lane")
	}

	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "all components"}
	if IsArchitectureNarrativeExplanation(rm) {
		t.Fatal("explicit completeness obligation should keep an architecture/member-list hybrid out of narrative-only lane")
	}
}

func TestArchitectureInventoryShape_TypedBoundary(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentEnumerate,
		Scenario: ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{
			IsScalarAnswer:        true,
			IsCountQuestion:       true,
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
			HasPerMemberTable:     true,
		},
		PredicateAxis: AxisCall,
	}
	if !IsArchitectureInventoryShape(rm) {
		t.Fatal("architecture count/list/role/relation hybrid should enter the compact inventory lane")
	}

	scalar := rm
	scalar.Predicates.IsCountQuestion = false
	if IsArchitectureInventoryShape(scalar) {
		t.Fatal("non-count scalar architecture lookup must not enter the inventory lane")
	}

	fieldValue := rm
	fieldValue.Predicates.IsScalarAnswer = false
	fieldValue.Predicates.IsCountQuestion = false
	fieldValue.FieldValueProfile = &FieldValueLookupProfile{
		IsFieldValueLookup: true,
		Target:             "Foo.Bar",
		Field:              "Bar",
		Literal:            "false",
	}
	if IsArchitectureInventoryShape(fieldValue) {
		t.Fatal("field-value lookup needs literal proof, not compact architecture inventory")
	}

	narrative := RequestModel{
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Complexity: ComplexityComplex,
		SubTopics: []SubTopic{
			{Summary: "control loop"},
			{Summary: "handoff"},
		},
		Predicates:  SemanticPredicates{IsCrossComponent: true},
		DiagramHint: &DiagramHint{Kind: DiagramArchitecture},
	}
	if !IsArchitectureInventoryShape(narrative) {
		t.Fatal("multi-subtopic architecture decomposition should have a compact inventory projection")
	}

	narrative.Predicates.IsDiagnosticQuestion = true
	if IsArchitectureInventoryShape(narrative) {
		t.Fatal("diagnostic architecture questions stay in diagnostic evidence lanes")
	}
}

func TestIsSingleTopicMechanismExplanation_TypedOnly(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		PredicateAxis: AxisCondition,
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqMechanism),
			Entities: []string{"emit_evidence", "anchor_kind", "EmitEvidence"},
		},
	}
	if !IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("single-topic mechanism explanation should be recognized from typed fields")
	}

	rm.Predicates.IsCrossComponent = true
	if !IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("bare cross-component breadth without subtopics should still stay in the single-topic mechanism lane")
	}

	rm.SubTopics = []SubTopic{
		{Summary: "first branch", Entities: []string{"First"}},
		{Summary: "second branch", Entities: []string{"Second"}},
	}
	if !IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("analyzer sub-topic decomposition alone must not turn one mechanism question into architecture/comparison")
	}

	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "cover every branch"}
	if !CompletenessObligationIsMechanismCoverageOnly(rm) {
		t.Fatal("typed mechanism completeness should be treated as coverage, not a principal member-set boundary")
	}
	if !IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("coverage-only completeness must not turn one mechanism explanation into an enumeration slate")
	}

	rm.Predicates.IsCategoryEnumeration = true
	if CompletenessObligationIsMechanismCoverageOnly(rm) {
		t.Fatal("category-enumeration completeness is a principal member-set boundary, not coverage-only")
	}
	if IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("category-enumeration shape must not stay in the lightweight mechanism lane")
	}
	rm.Predicates.IsCategoryEnumeration = false

	rm.Predicates.IsRoleLocateLookup = true
	rm.Predicates.IsScalarAnswer = true
	if IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("scalar role-locate lookup must not be stolen by the lightweight mechanism lane")
	}
	rm.Predicates.IsRoleLocateLookup = false
	rm.Predicates.IsScalarAnswer = false

	rm.Predicates.IsCrossComponent = false
	rm.SubTopics = nil
	rm.CompletenessObligation = nil
	rm.EnumerationBoundary = &RequestedEnumerationBoundary{
		DeclaredCount: 3,
		SourceQuote:   "3 steps",
	}
	if IsSingleTopicMechanismExplanation(rm) {
		t.Fatal("structural obligations should keep bounded mechanism questions out of the lightweight lane")
	}
}

func TestIsCodeIdentitySurface_CrossLanguage(t *testing.T) {
	accepted := []string{
		"aggregator",
		"findings_validator",
		"com.example.api",
		"react-dom",
		"@scope/pkg",
		"foo::bar",
		"packages/core",
		"ohos.ability",
	}
	for _, surface := range accepted {
		if !IsCodeIdentitySurface(surface) {
			t.Fatalf("expected %q to be accepted as a cross-language code identity", surface)
		}
	}

	rejected := []string{
		"",
		"two words",
		"entry point",
		"foo,bar",
		"needs?answer",
		// 2026-05-16 fix: CJK display labels were silently accepted
		// because Go's unicode.IsLetter is true for CJK ideographs. The
		// citation-alignment oracle then demanded these labels name a
		// symbol at the cited file:line, which no Chinese-only label
		// can satisfy. Pure-CJK and CJK-mixed labels are display prose
		// in this codebase.
		"引用锚定",   // citation grounding
		"自审查机制",  // self-review mechanism
		"质量门",    // quality gate
		"Foo 函数", // mixed: code symbol + CJK prose
		"Привет", // Cyrillic
		"αβγ",    // Greek
	}
	for _, surface := range rejected {
		if IsCodeIdentitySurface(surface) {
			t.Fatalf("expected %q to be rejected as prose / punctuation, not a code identity", surface)
		}
	}
}

// TestIsScalarSourceLiteralLookup_RoleLocateAllowedWithoutBundle is
// the negative-control: same RM minus the bundle → role-locate
// short-circuit is restored. Confirms the fix is gated by the
// artifact, not by a new universal blocker.
func TestIsScalarSourceLiteralLookup_RoleLocateAllowedWithoutBundle(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsRoleLocateLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: "mechanism"},
		// no LogTriage / PerfTrace
	}
	if !IsScalarSourceLiteralLookup(rm) {
		t.Fatal("role-locate short-circuit must still fire when no artifact bundle is attached (preserves single-literal locate behaviour)")
	}
}

func TestIsScalarSourceLiteralLookup_DiagnosticPredicateBlocksScalarLaneWithoutBundle(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentRootCause,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsRoleLocateLookup:   true,
			IsScalarAnswer:       true,
			IsDiagnosticQuestion: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: "mechanism"},
	}
	if IsScalarSourceLiteralLookup(rm) {
		t.Fatal("diagnostic questions must not enter the scalar source-literal lane, even without an attached artifact")
	}
}

// TestIsScalarSourceLiteralLookup_RoleLocateBlockedByPerfTrace pins
// the perf channel: HiTrace / atrace / systrace bundle with 2+ stalls
// or janks is also "multi-step mechanism" evidence and must block the
// scalar short-circuit, same as multi-frame log.
func TestIsScalarSourceLiteralLookup_RoleLocateBlockedByPerfTrace(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsRoleLocateLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: "mechanism"},
		PerfTrace: &PerfBundle{
			Janks: []PerfJank{
				{StartTsMs: 100, DurationMs: 25, Reason: "main-thread-blocked"},
				{StartTsMs: 200, DurationMs: 30, Reason: "gc-pause"},
			},
		},
	}
	if IsScalarSourceLiteralLookup(rm) {
		t.Fatal("role-locate short-circuit must NOT fire when 2+ perf janks attached")
	}
}

// TestIsScalarSourceLiteralLookup_SingleFrameDoesNotBlock pins the
// threshold: a 1-frame stack alone is NOT multi-step evidence (could
// be a single-line assertion failure). The role-locate short-circuit
// is preserved in this case so a "find the file that crashed" lookup
// still resolves to a literal.
func TestIsScalarSourceLiteralLookup_SingleFrameDoesNotBlock(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsRoleLocateLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: "mechanism"},
		LogTriage: &LogBundle{
			Errors: []LogError{{
				Type: "panic",
				Frames: []LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				},
			}},
		},
	}
	if !IsScalarSourceLiteralLookup(rm) {
		t.Fatal("single-frame log should NOT temper role-locate short-circuit (threshold is 2+)")
	}
}

func TestIsScalarSourceLiteralLookup_FallbackForUnnamedRoleLocate(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		AnalyzerHints: AnalyzerHints{
			Kind: "mechanism",
		},
	}
	if !IsScalarSourceLiteralLookup(rm) {
		t.Fatal("expected unnamed simple role-locate lookup to stay in scalar source-literal lane")
	}
	if !IsScalarRoleLocateLookup(rm) {
		t.Fatal("expected unnamed scalar fallback to be treated as role-locate lookup")
	}
}

func TestIsScalarSourceLiteralLookup_FallbackDoesNotFireWhenEntityAlreadyNamed(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		AnalyzerHints: AnalyzerHints{
			Kind:            "mechanism",
			PrimaryEntities: []string{"buildAnalysisIR"},
			Entities:        []string{"buildAnalysisIR"},
		},
	}
	if IsScalarSourceLiteralLookup(rm) {
		t.Fatal("named-entity mechanism question should not be forced into scalar source-literal fallback")
	}
	if IsScalarRoleLocateLookup(rm) {
		t.Fatal("named-entity mechanism question should not be treated as role-locate fallback")
	}
}

func TestIsScalarSourceLiteralLookup_ExplicitRoleLocateKeepsScalarLaneWithClueEntity(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexityModerate,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName},
		Predicates: SemanticPredicates{
			IsScalarAnswer:     true,
			IsRoleLocateLookup: true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind:            "mechanism",
			PrimaryEntities: []string{"AnalysisIR"},
			Entities:        []string{"AnalysisIR"},
		},
	}
	if !IsScalarSourceLiteralLookup(rm) {
		t.Fatal("explicit role-locate predicate should keep the request in scalar source-literal lane even when the user named a clue entity")
	}
	if !IsScalarRoleLocateLookup(rm) {
		t.Fatal("explicit role-locate predicate should classify as scalar role-locate lookup")
	}
}

func TestIsScalarSourceLiteralLookup_ExplicitRoleLocateAllowsConfigKey(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Complexity:    ComplexitySimple,
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
		Predicates: SemanticPredicates{
			IsScalarAnswer:     true,
			IsRoleLocateLookup: true,
		},
	}
	if !IsScalarSourceLiteralLookup(rm) {
		t.Fatal("config-key role-locate should stay in scalar lookup lane")
	}
}
