package types

import (
	"reflect"
	"testing"
)

func TestCompileAnswerIntentContract_HistoryScalarKeepsOriginSeparateFromShape(t *testing.T) {
	rm := RequestModel{
		Intent: IntentReturnValue,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
			IsScalarAnswer:  true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqHistory)},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginVCSMetadata},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputScalar},
	)
	if got.HasOutput(AnswerRequestedOutputEnumeration) || got.HasOutput(AnswerRequestedOutputMechanism) {
		t.Fatalf("history scalar must not grow richer answer outputs: %+v", got.RequestedOutputs)
	}
}

func TestCompileAnswerIntentContract_HistoryMultiTargetScalarBecomesPerTargetSet(t *testing.T) {
	rm := RequestModel{
		Intent: IntentReturnValue,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
			IsScalarAnswer:  true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind:            string(ReqHistory),
			PrimaryEntities: []string{"scalar", "definitely_absent_marker"},
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	if got.HasOutput(AnswerRequestedOutputScalar) {
		t.Fatalf("multi-target history existence query must not request scalar output: %+v", got.RequestedOutputs)
	}
	if !got.HasOutput(AnswerRequestedOutputEnumeration) {
		t.Fatalf("multi-target history query should keep a per-target answer surface: %+v", got.RequestedOutputs)
	}
}

func TestCompileAnswerIntentContract_HistoryMultiTargetCountStaysScalar(t *testing.T) {
	rm := RequestModel{
		Intent: IntentReturnValue,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
			IsScalarAnswer:  true,
			IsCountQuestion: true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind:            string(ReqHistory),
			PrimaryEntities: []string{"scalar", "other"},
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	if !got.HasOutput(AnswerRequestedOutputScalar) || !got.HasOutput(AnswerRequestedOutputCount) {
		t.Fatalf("true multi-target history count should remain scalar/count: %+v", got.RequestedOutputs)
	}
}

func TestCompileAnswerIntentContract_HistoryNarrativeDoesNotCollapseToScalar(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioGeneric,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqHistory)},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginVCSMetadata},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputMechanism},
	)
	if got.HasOutput(AnswerRequestedOutputScalar) || got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("pure history narrative should stay VCS narrative, got %+v", got)
	}
}

func TestCompileAnswerIntentContract_PureHistoryEnumerationStaysVCSOnly(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqHistory)},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginVCSMetadata},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputEnumeration},
	)
	if got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("pure commit-history enumeration must not require current-source origin: %+v", got)
	}
}

func TestCompileAnswerIntentContract_HistoryCurrentCodeMechanismKeepsTwoOrigins(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{
			AnswerEvidenceOriginCurrentSource,
			AnswerEvidenceOriginVCSMetadata,
			AnswerEvidenceOriginVCSDiff,
		},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputMechanism},
	)
}

func TestCompileAnswerIntentContract_HistoryDiagramKeepsDiagramAndDiff(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
		DiagramHint:   &DiagramHint{Kind: DiagramFlow, Required: true},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{
			AnswerEvidenceOriginCurrentSource,
			AnswerEvidenceOriginVCSMetadata,
			AnswerEvidenceOriginVCSDiff,
		},
		[]AnswerRequestedOutput{
			AnswerRequestedOutputSummary,
			AnswerRequestedOutputMechanism,
			AnswerRequestedOutputDiagram,
		},
	)
}

func TestCompileAnswerIntentContract_OptionalHistoryDiagramHintStaysAdvisory(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
		DiagramHint:   &DiagramHint{Kind: DiagramFlow, Required: false},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{
			AnswerEvidenceOriginCurrentSource,
			AnswerEvidenceOriginVCSMetadata,
		},
		[]AnswerRequestedOutput{
			AnswerRequestedOutputSummary,
			AnswerRequestedOutputMechanism,
		},
	)
}

func TestCompileAnswerIntentContract_CurrentSourceCountUsesMeasurementOrigin(t *testing.T) {
	rm := RequestModel{
		Intent: IntentReturnValue,
		Predicates: SemanticPredicates{
			IsScalarAnswer:  true,
			IsCountQuestion: true,
		},
		AnswerSubject: AnswerSubject{Kind: SubjectNumeric},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{
			AnswerEvidenceOriginCurrentSource,
			AnswerEvidenceOriginCommandMeasurement,
		},
		[]AnswerRequestedOutput{
			AnswerRequestedOutputSummary,
			AnswerRequestedOutputScalar,
			AnswerRequestedOutputCount,
		},
	)
}

func TestCompileAnswerIntentContract_ExternalRuntimeArtifactWithoutRequiredSourceUsesRuntimeOrigin(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{IsDiagnostic: true},
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "panic"}},
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginRuntimeArtifact},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputDiagnostic},
	)
}

func TestCompileAnswerIntentContract_ExternalRuntimeArtifactExplicitExcludeSuppressesCurrentSource(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{IsDiagnostic: true},
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "panic"}},
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode: ExternalObservationCurrentSourceExclude,
			ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
			SourceQuotes:      []string{"只分析日志"},
			Confidence:        0.9,
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginRuntimeArtifact},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputDiagnostic},
	)
	if got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("anchored external observation exclusion should suppress current-source origin: %+v", got.Origins)
	}
}

func TestCompileAnswerIntentContract_TraceArtifactWithoutRequiredSourceUsesRuntimeOrigin(t *testing.T) {
	rm := RequestModel{
		Intent: IntentTrace,
		PerfTrace: &PerfBundle{
			Janks: []PerfJank{{TriggerSpan: "main thread sleep", DurationMs: 135}},
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginRuntimeArtifact},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputTrace, AnswerRequestedOutputDiagnostic},
	)
}

func TestCompileAnswerIntentContract_TraceArtifactSourceScopeKeepsCurrentSource(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentRootCause,
		Scenario:      ScenarioPerformanceBottleneck,
		PredicateAxis: AxisImplement,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{IsDiagnostic: true},
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
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginRuntimeArtifact},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputDiagnostic},
	)
	if !rm.RequiresCurrentSourceForExternalObservation(nil) {
		t.Fatal("typed source scope on an external trace should require current-source evidence")
	}
}

func TestCompileAnswerIntentContract_DefaultTraceArtifactCurrentKeyCodeRequiresSource(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{IsDiagnostic: true},
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:       "state_churn",
				Subject:    "app-20",
				Summary:    "runtime trace span is fragmented",
				LineStart:  3,
				LineEnd:    23,
				DurationMs: 8,
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
			Confidence: 0.8,
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginRuntimeArtifact},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputDiagnostic},
	)
	if !rm.RequiresCurrentSourceForExternalObservation(nil) {
		t.Fatal("required current_key_code dimension plus typed source scope should require current-source evidence")
	}
}

func TestCompileAnswerIntentContract_DefaultTraceArtifactMetricDimensionStaysRuntimeOnly(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{IsDiagnostic: true},
		PerfTrace: &PerfBundle{
			Observations: []PerfObservation{{
				Kind:       "state_churn",
				Subject:    "app-20",
				Summary:    "runtime trace span is fragmented",
				LineStart:  3,
				LineEnd:    23,
				DurationMs: 8,
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
			Confidence: 0.8,
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginRuntimeArtifact},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputDiagnostic},
	)
	if rm.RequiresCurrentSourceForExternalObservation(nil) {
		t.Fatal("runtime metric dimension should not require current-source evidence")
	}
}

func TestCompileAnswerIntentContract_MCPResourceDefaultsToCurrentSourceUnlessExcluded(t *testing.T) {
	rm := RequestModel{
		RawRequest: "读取 mcp://fixture/trace/sleep-wakeup 并回答",
		Intent:     IntentExplain,
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginMCPResource},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputMechanism},
	)

	rm.ExternalObservationPolicy = &ExternalObservationPolicy{
		CurrentSourceMode: ExternalObservationCurrentSourceExclude,
		ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"只分析 MCP"},
		Confidence:        0.9,
	}
	got = CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginMCPResource},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputMechanism},
	)
}

func TestCompileAnswerIntentContract_ExternalRuntimeArtifactCurrentStatusWithoutAnchorSuppressesCurrentSource(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentRisk:         true,
			CurrentVersionCheck: true,
		},
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "fatal error"}},
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginRuntimeArtifact},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputDiagnostic},
	)
}

func TestCompileAnswerIntentContract_ExternalRuntimeArtifactCurrentStatusWithSourceProfileKeepsCurrentSource(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentRisk:         true,
			CurrentVersionCheck: true,
		},
		AnalyzerHints: AnalyzerHints{
			ExactTargets: []string{"writeSession"},
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationVerifyCurrentStatus},
			SourceQuotes:                        []string{"current checkout"},
			Confidence:                          0.95,
		},
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "fatal error"}},
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginRuntimeArtifact},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputDiagnostic},
	)
}

func TestCompileAnswerIntentContract_ExternalRuntimeArtifactRequiredFilesKeepCurrentSourceExplanation(t *testing.T) {
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
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginRuntimeArtifact},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputMechanism, AnswerRequestedOutputDiagnostic},
	)
}

func TestCompileAnswerIntentContract_ExternalRuntimeArtifactProfileKeepsCurrentSourceExplanation(t *testing.T) {
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
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginRuntimeArtifact},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputMechanism, AnswerRequestedOutputDiagnostic},
	)
}

func TestCompileAnswerIntentContract_HistoryScalarWithProfileKeepsCurrentSource(t *testing.T) {
	rm := RequestModel{
		Intent: IntentReturnValue,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
			IsScalarAnswer:  true,
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationLocateCurrentCode},
			SourceQuotes:                        []string{"结合当前代码"},
			Confidence:                          0.8,
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	if !got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("explicit current-source explanation profile should override scalar history default: %+v", got.Origins)
	}
	if !got.HasOrigin(AnswerEvidenceOriginVCSMetadata) {
		t.Fatalf("history provenance should remain present: %+v", got.Origins)
	}
}

func TestCompileAnswerIntentContract_CurrentSourceMechanismBaseline(t *testing.T) {
	rm := RequestModel{
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{},
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqMechanism),
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	assertAnswerIntentContract(t, got,
		[]AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource},
		[]AnswerRequestedOutput{AnswerRequestedOutputSummary, AnswerRequestedOutputMechanism},
	)
}

func TestCompileAnswerIntentContract_BucketedComparisonPreservesOutput(t *testing.T) {
	rm := RequestModel{
		RawRequest: "比较 A 和 B",
		Intent:     IntentExplain,
		Predicates: SemanticPredicates{
			IsCrossComponent: true,
		},
		Buckets: []QuestionBucket{
			{Label: "A", Index: 1},
			{Label: "B", Index: 2},
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	if !got.HasOutput(AnswerRequestedOutputComparison) {
		t.Fatalf("bucketed request should preserve comparison output: %+v", got.RequestedOutputs)
	}
	if !got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("ordinary comparison should use current-source origin: %+v", got.Origins)
	}
}

func TestCompileAnswerIntentContract_HistoryCommitComparisonStaysVCS(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup:  true,
			IsCrossComponent: true,
		},
		AnalyzerHints: AnalyzerHints{Kind: string(ReqHistory)},
		Buckets: []QuestionBucket{
			{Label: "commit A", Index: 1},
			{Label: "commit B", Index: 2},
		},
	}
	got := CompileAnswerIntentContract(rm, nil)
	if !reflect.DeepEqual(got.Origins, []AnswerEvidenceOrigin{AnswerEvidenceOriginVCSMetadata, AnswerEvidenceOriginVCSDiff}) {
		t.Fatalf("pure commit-history comparison origins mismatch: %+v", got.Origins)
	}
	if !got.HasOutput(AnswerRequestedOutputComparison) || got.HasOutput(AnswerRequestedOutputScalar) {
		t.Fatalf("pure commit-history comparison should keep comparison shape without scalar collapse: %+v", got.RequestedOutputs)
	}
	if got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("pure commit-history comparison must not require current-source origin: %+v", got.Origins)
	}
}

func TestCompileAnswerIntentContract_ExactResolutionAllowsAbsenceOutput(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		AnswerSubject: AnswerSubject{
			Kind: SubjectFunctionName,
		},
	}
	contract := &AnswerContract{
		ExactResolution: &ExactResolutionContract{
			TargetKind:   SubjectFunctionName,
			TargetLabel:  "symbol",
			Targets:      []string{"MissingSymbol"},
			AllowAbsence: true,
		},
	}
	got := CompileAnswerIntentContract(rm, contract)
	if !got.HasOutput(AnswerRequestedOutputAbsence) {
		t.Fatalf("exact-resolution contract that allows absence should preserve absence output: %+v", got.RequestedOutputs)
	}
	if !got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("exact-resolution source lookup should still require current-source origin: %+v", got.Origins)
	}
}

func TestCompileAnswerIntentContract_SourceInventoryAttributeDemand(t *testing.T) {
	perMemberTable := RequestModel{
		Intent:     IntentExplain,
		Predicates: SemanticPredicates{HasPerMemberTable: true},
	}
	if !CompileAnswerIntentContract(perMemberTable, nil).SourceInventoryAttributeDemand {
		t.Fatalf("per-member table request must demand source-inventory attribute columns")
	}

	summaryColumns := RequestModel{
		Intent: IntentEnumerate,
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldSummary},
		},
	}
	if !CompileAnswerIntentContract(summaryColumns, nil).SourceInventoryAttributeDemand {
		t.Fatalf("inventory profile requesting summary columns must demand attribute records")
	}

	identityOnly := RequestModel{
		Intent: IntentEnumerate,
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation},
		},
	}
	if CompileAnswerIntentContract(identityOnly, nil).SourceInventoryAttributeDemand {
		t.Fatalf("name/location identity fields ride on member records and must not demand attribute columns")
	}

	// Plain member enumerations demand members, not columns: the demand lane
	// must not widen to IsCategoryEnumeration / IntentEnumerate (that would
	// re-create the crowding 3fab4c14 fixed).
	plainEnumeration := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
	}
	if CompileAnswerIntentContract(plainEnumeration, nil).SourceInventoryAttributeDemand {
		t.Fatalf("plain member enumeration must not demand attribute columns")
	}

	inactiveProfile := RequestModel{
		Intent: IntentEnumerate,
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldSummary},
		},
	}
	if CompileAnswerIntentContract(inactiveProfile, nil).SourceInventoryAttributeDemand {
		t.Fatalf("inactive inventory profile (no target roles) must not demand attribute columns")
	}
}

func assertAnswerIntentContract(t *testing.T, got AnswerIntentContract, origins []AnswerEvidenceOrigin, outputs []AnswerRequestedOutput) {
	t.Helper()
	if !reflect.DeepEqual(got.Origins, origins) {
		t.Fatalf("origins mismatch\ngot:  %#v\nwant: %#v", got.Origins, origins)
	}
	if !reflect.DeepEqual(got.RequestedOutputs, outputs) {
		t.Fatalf("outputs mismatch\ngot:  %#v\nwant: %#v", got.RequestedOutputs, outputs)
	}
}

// §29.146 UPSTREAM-3 件2 pin: the current-source exclusion carve used to
// require a materialized triage bundle (rm.HasExternalOnlyRuntimeArtifact), so
// an attached trace above the perf_triage size gate (bundle == nil) bypassed
// the user's typed exclusion and minted a current_source origin. The carve now
// reads the deterministic Run-entry RuntimeArtifactPreflight carrier — the
// same single value source TOOLWIN admission and the LENSBURN guards consume —
// but ONLY when the corresponding bundle is absent.
func TestCompileAnswerIntentContractWithPreflight_LargeTraceExclusionCarveNotBypassed(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{IsDiagnostic: true},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
			ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			SourceQuotes:         []string{"只分析这份 trace，不分析代码"},
			Confidence:           0.95,
		},
		ArtifactObservationProfile: &ArtifactObservationProfile{
			Source:         "trace",
			SymptomSummary: "frame jank window",
		},
	}
	// A typed contract lane that would otherwise include current_source: this
	// isolates the carve as the deciding arm (the carve precedes the
	// contract-required check, exactly as it does for small-trace bundles).
	contract := &AnswerContract{
		CurrentStatusDiagnostic: &CurrentStatusDiagnosticContract{Required: true},
	}
	preflight := RuntimeArtifactPreflightProfile{
		Active: true,
		Artifacts: []RuntimeArtifactPreflightArtifact{{
			Kind:   "trace",
			Source: "attached_trace.systrace",
			Bytes:  1900 * 1024,
		}},
	}

	// Pre-fix shape (documented): the preflight-blind compile bypasses the
	// carve because no bundle was materialized for the 1.9MB trace.
	if !CompileAnswerIntentContract(rm, contract).HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatal("probe setup: the preflight-blind compile should still show the pre-fix bypass shape")
	}

	// 件2: with the Run-entry preflight carrier, the exclusion carve holds and
	// no current_source origin is minted.
	got := CompileAnswerIntentContractWithPreflight(rm, contract, preflight)
	if got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("typed exclusion + preflight trace artifact must carve the current_source origin, got %v", got.Origins)
	}
	if !got.HasOrigin(AnswerEvidenceOriginRuntimeArtifact) {
		t.Fatalf("runtime_artifact origin must survive the carve, got %v", got.Origins)
	}

	// Negative arm ① (bundle authority): a materialized bundle keeps FULL
	// authority over its lane. When the trace bundle resolved frames into
	// current-source files (IsExternalSource()==false), the preflight arm must
	// stay disabled and the origin is kept.
	withBundle := rm
	withBundle.PerfTrace = &PerfBundle{
		Observations:  []PerfObservation{{Kind: "state_churn", Subject: "app-20", Summary: "resolved trace", LineStart: 1}},
		ResolvedFiles: []string{"internal/render/frame.go"},
	}
	if !CompileAnswerIntentContractWithPreflight(withBundle, contract, preflight).HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatal("preflight arm must not override a materialized bundle that resolved current-source frames")
	}

	// Negative arm ② (no exclusion policy): preflight presence alone must not
	// carve anything — the default posture keeps source exploration open.
	noPolicy := rm
	noPolicy.ExternalObservationPolicy = nil
	if !CompileAnswerIntentContractWithPreflight(noPolicy, contract, preflight).HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatal("preflight artifact without a typed exclusion must keep the current_source origin")
	}

	// Zero-value preflight delegation: the legacy entry compiles identically
	// to WithPreflight(zero) — existing callers are byte-identical.
	if !reflect.DeepEqual(
		CompileAnswerIntentContract(rm, contract),
		CompileAnswerIntentContractWithPreflight(rm, contract, RuntimeArtifactPreflightProfile{})) {
		t.Fatal("zero-value preflight must compile byte-identically to the legacy entry")
	}
}

// §29.151 UPTAIL-1 件1 (P3-a) — single-listed TIGHTEN pin with its face
// matrix. The preflight carve arm now defers to the typed current-verification
// anchor so the contract face agrees with the lane face
// (CurrentSourceLaneDecision consults the anchor before the exclusion).
// Scope: preflight arm ONLY — the bundle arm keeps the ratified legacy
// carve-beats-anchor posture (UPSTREAM-3 P2 inversion pin, ratified
// §29.160/R-21), because a materialized bundle is the kind authority while
// the preflight profile is a mere presence census. §29.146 healthy-shape
// safety: the witness (frame_multicausal-20260719-030504) carries no precise
// anchor post-件3, so the conjunct cannot re-mint the origin there — the
// carve-holds arm of TestCompileAnswerIntentContractWithPreflight_
// LargeTraceExclusionCarveNotBypassed above is exactly that shape and stays
// green.
func TestCompileAnswerIntentContractWithPreflight_PreflightCarveDefersToVerificationAnchor(t *testing.T) {
	preflight := RuntimeArtifactPreflightProfile{
		Active: true,
		Artifacts: []RuntimeArtifactPreflightArtifact{{
			Kind:   "trace",
			Source: "attached_trace.systrace",
			Bytes:  1900 * 1024,
		}},
	}
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{IsDiagnostic: true},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
			ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			SourceQuotes:         []string{"只分析这份 trace，不分析代码"},
			Confidence:           0.95,
		},
		// The artifact path keeps HasExternalObservationArtifactReference on
		// (the anchor predicate's carrier gate); the code path is the PRECISE
		// current-source anchor that used to be contradicted by the carve.
		AnalyzerHints: AnalyzerHints{
			ExactTargets: []string{"attached_trace.systrace", "internal/render/frame.go"},
		},
		// Keeps the runtime_artifact origin populated so the empty-origin
		// current_source fallback can never mask what the carve decided.
		ArtifactObservationProfile: &ArtifactObservationProfile{
			Source:         "trace",
			SymptomSummary: "frame jank window",
		},
	}

	// Matrix row 1 — lane face: the precise anchor wins over the exclusion.
	if !rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		t.Fatal("probe setup: the precise code target must mint the verification anchor")
	}
	if got := rm.CurrentSourceLaneDecision(); got != CurrentSourceLaneRequired {
		t.Fatalf("lane face: anchor must win over exclusion, got %s", got)
	}

	// Matrix row 2 — contract face (the tighten): the preflight arm defers to
	// the anchor, so the current_source origin is kept and both faces agree.
	got := CompileAnswerIntentContractWithPreflight(rm, nil, preflight)
	if !got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("preflight carve must defer to the typed verification anchor, got %v", got.Origins)
	}

	// Matrix row 3 — anchor-free control: without the precise target the
	// carve holds (the §29.146 witness-family healthy shape is unchanged).
	anchorFree := rm
	anchorFree.AnalyzerHints = AnalyzerHints{ExactTargets: []string{"attached_trace.systrace"}}
	if anchorFree.HasRuntimeArtifactCurrentVerificationAnchor() {
		t.Fatal("probe setup: the anchor-free shape must not mint an anchor")
	}
	if CompileAnswerIntentContractWithPreflight(anchorFree, nil, preflight).HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatal("anchor-free exclusion + preflight carrier must keep carving current_source")
	}

	// Matrix row 4 — ratified legacy (bundle arm, pinned CURRENT STATE, not a
	// change): a materialized EXTERNAL bundle keeps carve-beats-anchor even
	// when a precise target mints the anchor (UPSTREAM-3 P2 inversion,
	// ratified §29.160/R-21). Evolving this row needs its own ruling.
	bundleArm := rm
	bundleArm.LogTriage = &LogBundle{
		Observations: []LogObservation{{Kind: LogObservationRetryCycle, Summary: "external log", LineStart: 3}},
	}
	if !bundleArm.HasExternalOnlyRuntimeArtifact() || !bundleArm.HasRuntimeArtifactCurrentVerificationAnchor() {
		t.Fatal("probe setup: bundle arm needs external bundle + minted anchor")
	}
	if CompileAnswerIntentContractWithPreflight(bundleArm, nil, preflight).HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatal("bundle arm keeps the ratified carve-beats-anchor posture; this pin guards it against silent drift")
	}
}

// §29.151 UPTAIL-1 件2 (P3-b) — cross-kind authority pin. A small trace
// bundle that RESOLVED frames into current-source files is the trace-kind
// authority (its ResolvedFiles arm mints the verification anchor); a large
// unbundled log's preflight arm must not override that authority and carve
// the current_source origin the bundle earned. Delivered by the 件1 anchor
// conjunct — no separate mechanism. The negative arm keeps the per-kind
// conjunction honest: an UNRESOLVED external trace bundle grants no
// cross-kind authority, so the carve still holds there.
func TestCompileAnswerIntentContractWithPreflight_CrossKindBundleAuthorityNotOverridden(t *testing.T) {
	preflight := RuntimeArtifactPreflightProfile{
		Active: true,
		Artifacts: []RuntimeArtifactPreflightArtifact{
			{Kind: "trace", Source: "small_trace.systrace", Bytes: 120 * 1024},
			{Kind: "log", Source: "huge_app.log", Bytes: 2100 * 1024},
		},
	}
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioPerformanceBottleneck,
		Predicates: SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: DiagnosticIntentProfile{IsDiagnostic: true},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
			ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			SourceQuotes:         []string{"只看运行数据"},
			Confidence:           0.9,
		},
		// The artifact path token keeps the anchor predicate's carrier gate
		// open (HasExternalObservationArtifactReference), exactly as real
		// attached-artifact runs record the attachment in analyzer hints.
		AnalyzerHints: AnalyzerHints{
			ExactTargets: []string{"small_trace.systrace"},
		},
		// Trace kind: materialized bundle, frames RESOLVED into the current
		// checkout — trace-kind authority says the source lane is real.
		PerfTrace: &PerfBundle{
			Observations:  []PerfObservation{{Kind: "state_churn", Subject: "app-20", Summary: "resolved trace", LineStart: 1}},
			ResolvedFiles: []string{"internal/render/frame.go"},
		},
		// Log kind: no bundle (LogTriage == nil) — only the preflight census
		// knows the 2.1MB log exists.
	}
	if rm.HasExternalOnlyRuntimeArtifact() {
		t.Fatal("probe setup: the resolved trace bundle must not read as external-only")
	}
	if !runtimeArtifactPreflightCarrierWithoutBundle(rm, preflight) {
		t.Fatal("probe setup: the unbundled log must activate the preflight carrier")
	}
	if !rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		t.Fatal("probe setup: the resolved trace bundle must mint the verification anchor (ResolvedFiles arm)")
	}
	if !CompileAnswerIntentContractWithPreflight(rm, nil, preflight).HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatal("the log kind's preflight arm must not override the resolved trace bundle's authority")
	}

	// Negative arm ① — an unresolved EXTERNAL trace bundle carries no
	// cross-kind authority: the carve holds (via the bundle arm and,
	// consistently, the anchor-free preflight arm).
	unresolved := rm
	unresolved.PerfTrace = &PerfBundle{
		Observations: []PerfObservation{{Kind: "state_churn", Subject: "app-20", Summary: "unresolved trace", LineStart: 1}},
	}
	if CompileAnswerIntentContractWithPreflight(unresolved, nil, preflight).HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatal("an unresolved external trace bundle must not shield the current_source origin from the exclusion carve")
	}

	// Negative arm ② — face-coherence pin for the no-reference corner
	// (current state, deliberate): when NO typed artifact reference keeps the
	// anchor predicate's carrier gate open, the resolved bundle cannot mint
	// the anchor, so the carve fires — and the LANE face answers Excluded
	// through the same gate. Both faces agree (no contradictory corner), so
	// this stays pinned as-is rather than growing a second carve predicate.
	noReference := rm
	noReference.AnalyzerHints = AnalyzerHints{}
	if noReference.HasRuntimeArtifactCurrentVerificationAnchor() {
		t.Fatal("probe setup: without an artifact reference the carrier gate must keep the anchor off")
	}
	if got := noReference.CurrentSourceLaneDecision(); got != CurrentSourceLaneExcluded {
		t.Fatalf("lane face must agree with the carve in the no-reference corner, got %s", got)
	}
	if CompileAnswerIntentContractWithPreflight(noReference, nil, preflight).HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatal("no-reference corner: both faces excluded — the carve holds coherently")
	}
}
