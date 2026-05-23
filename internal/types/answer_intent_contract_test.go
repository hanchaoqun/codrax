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
		DiagramHint:   &DiagramHint{Kind: DiagramFlow},
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

func TestCompileAnswerIntentContract_ExternalRuntimeArtifactDoesNotRequireCurrentSource(t *testing.T) {
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
	if got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("external-only runtime artifact should not imply current-source origin: %+v", got.Origins)
	}
}

func TestCompileAnswerIntentContract_ExternalRuntimeArtifactCurrentStatusWithoutAnchorStaysRuntimeOnly(t *testing.T) {
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
	if got.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("external-only runtime artifact without a current-source anchor should stay runtime-only: %+v", got.Origins)
	}
}

func TestCompileAnswerIntentContract_ExternalRuntimeArtifactCurrentStatusWithExactTargetKeepsCurrentSource(t *testing.T) {
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

func assertAnswerIntentContract(t *testing.T, got AnswerIntentContract, origins []AnswerEvidenceOrigin, outputs []AnswerRequestedOutput) {
	t.Helper()
	if !reflect.DeepEqual(got.Origins, origins) {
		t.Fatalf("origins mismatch\ngot:  %#v\nwant: %#v", got.Origins, origins)
	}
	if !reflect.DeepEqual(got.RequestedOutputs, outputs) {
		t.Fatalf("outputs mismatch\ngot:  %#v\nwant: %#v", got.RequestedOutputs, outputs)
	}
}
