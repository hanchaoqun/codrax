package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

// sampleAnalysisIR builds a fully populated AnalysisIR so the roundtrip
// test exercises every field, including optional slices/maps. Future
// fields added to the struct should extend this fixture so the roundtrip
// test catches accidental omissions from json marshalling.
func sampleAnalysisIR() AnalysisIR {
	return AnalysisIR{
		Version: AnalysisIRVersion,
		TraceID: "trace-abc-123",
		RequestModel: RequestModel{
			RawRequest: "解释 explorer 是如何决定停止的",
			Language:   "zh",
			Intent:     IntentExplain,
			Scenario:   ScenarioArchitectureExplain,
			Complexity: ComplexityModerate,
			TermGraph: TermGraph{
				Canonical: []CanonicalTerm{
					{ID: "t1", Surface: "explorer", Language: "en", Kind: TermSymbol, Confidence: 0.95},
					{ID: "t2", Surface: "ShouldStop", Language: "en", Kind: TermSymbol, Domain: "agent", Confidence: 1.0},
				},
				Aliases: []TermAlias{
					{Source: "探索器", Target: "t1", Relation: "translation", Confidence: 0.9},
				},
			},
			Ambiguities: []Ambiguity{
				{Clause: "停止", Options: []string{"soft-stop", "hard-stop"}, Resolution: "both"},
			},
			RiskMatrix: RiskMatrix{
				Security:      RiskLevel{Level: 0},
				DataIntegrity: RiskLevel{Level: 1, Evidence: []string{"read-only"}},
				Compatibility: RiskLevel{Level: 0},
				Performance:   RiskLevel{Level: 0},
				Ops:           RiskLevel{Level: 0},
				Compliance:    RiskLevel{Level: 0},
			},
			AnalyzerHints: AnalyzerHints{
				Keywords: []string{"explorer", "ShouldStop"},
				Entities: []string{"explorer", "ShouldStop"},
				Kind:     "call_chain",
			},
			AnswerSubject: AnswerSubject{Kind: SubjectFunctionName, Confidence: 0.8},
			PredicateAxis: AxisCall,
			DiagramHint:   &DiagramHint{Kind: DiagramCallDAG},
			ErrorGranularityProfile: &ErrorGranularityProfile{
				IsGranularityQuestion: true,
				SourceQuotes:          []string{"停止"},
				Confidence:            0.7,
				Rationale:             "roundtrip fixture exercises typed verdict lane",
			},
		},
		TaskGraph: TaskGraph{
			Nodes: []TaskNode{
				{
					ID:              "n1",
					Type:            NodeProbe,
					Objective:       "locate ShouldStop definition",
					Inputs:          []string{"t2"},
					Outputs:         []string{"shouldstop_impl"},
					SuccessCriteria: []Criterion{{Kind: CritSymbolPresent, Expr: "ShouldStop"}},
					SearchHints:     SearchHints{KeywordIDs: []string{"t2"}, EntityIDs: []string{"t2"}},
				},
				{
					ID:          "n2",
					Type:        NodeEvidence,
					Objective:   "collect stop conditions",
					Hypotheses:  []string{"h1"},
					SearchHints: SearchHints{KeywordIDs: []string{"t1", "t2"}},
					MaxRetries:  2,
				},
				{
					ID:          "n3",
					Type:        NodeFinalize,
					Objective:   "render answer",
					SearchHints: SearchHints{},
				},
			},
			Edges: []TaskEdge{
				{From: "n1", To: "n2", EdgeType: EdgeHardDependency},
				{From: "n2", To: "n3", EdgeType: EdgeHardDependency},
				{From: "n3", To: "n2", EdgeType: EdgeValidationFeedback, Guard: "contract_violation"},
			},
			ExecutionPolicy: ExecutionPolicy{
				MaxParallelism: 1,
				CriticalPath:   []string{"n1", "n2", "n3"},
				RetryBudget:    3,
			},
		},
		EvidencePlan: EvidencePlan{
			Budget: EvidenceBudget{MaxFiles: 40, MaxBytes: 200000, MaxReactIters: 20, MaxToolCalls: 30},
			SourceMix: map[string]int{
				"grep":    40,
				"repomap": 30,
				"read":    30,
			},
			StopConditions: []StopCondition{
				{Kind: CritAllHypothesesDecided},
				{Kind: CritContractSatisfied},
			},
		},
		AnswerContract: AnswerContract{
			Diagram: &DiagramContract{
				Required:       true,
				Minimum:        1,
				PreferredKinds: []DiagramKind{DiagramCallDAG, DiagramSequence},
				ScopeHint:      DiagramScopeOverall,
				Reasons:        []string{"trace_intent", "axis_call"},
			},
			ExactResolution: &ExactResolutionContract{
				TargetKind:              SubjectFunctionName,
				TargetLabel:             "symbol",
				Targets:                 []string{"ShouldStop"},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    ExactContextGroundedOnly,
				RelatedContextScopeHint: "grounded nearby context only",
			},
			MustInclude: []string{"t2"},
			MustExclude: nil,
			CitationReq: CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
			AcceptanceTests: []Criterion{
				{Kind: CritContainsSymbol, Expr: "ShouldStop"},
				{Kind: CritCitationCountGE, Expr: "2"},
			},
			Language: "zh",
		},
		HypothesisSet: []Hypothesis{
			{
				ID:        "h1",
				Statement: "ShouldStop returns true when ERM is satisfied",
				RequiredEvidence: []Criterion{
					{Kind: CritSymbolPresent, Expr: "ermSatisfied"},
				},
				FalsificationCondition: Criterion{Kind: CritNoCallSites, Expr: "ermSatisfied"},
				Priority:               80,
				Status:                 HypUnknown,
			},
		},
		QualityGate: GateReport{
			Passed:    true,
			Rejected:  false,
			Retryable: false,
			Checks: []GateCheck{
				{Name: "coverage", Passed: true, Score: 0.95, Threshold: 0.9},
				{Name: "dag_closure", Passed: true, Score: 1.0, Threshold: 1.0},
				{Name: "contract_complete", Passed: true, Score: 1.0, Threshold: 1.0},
			},
		},
	}
}

func TestAnalysisIR_JSONRoundtrip(t *testing.T) {
	original := sampleAnalysisIR()

	data, err := json.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded AnalysisIR
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("roundtrip mismatch:\noriginal=%#v\ndecoded =%#v", original, decoded)
	}
}

func TestRequestModel_DoesNotExposeLegacyTopLevelEntities(t *testing.T) {
	requestModelType := reflect.TypeOf(RequestModel{})
	if _, ok := requestModelType.FieldByName("Entities"); ok {
		t.Fatalf("RequestModel must not expose legacy top-level Entities; use AnalyzerHints.Entities with provenance lanes")
	}
	analyzerHintsType := reflect.TypeOf(AnalyzerHints{})
	if _, ok := analyzerHintsType.FieldByName("Entities"); !ok {
		t.Fatalf("AnalyzerHints.Entities is the canonical analyzer-emitted entity lane")
	}
}

func TestAnalysisIR_VersionConstant(t *testing.T) {
	// v14 adds error_granularity_profile so batch-vs-item / fail-fast /
	// partial-success requests require a typed decision verdict instead of
	// relying on answer prose synonyms.
	if AnalysisIRVersion != "v14" {
		t.Fatalf("unexpected AnalysisIRVersion: %q", AnalysisIRVersion)
	}
	ir := AnalysisIR{Version: AnalysisIRVersion}
	if ir.Version != "v14" {
		t.Fatalf("version not propagated: %q", ir.Version)
	}
}

func TestAnalysisIR_EnumDistinctness(t *testing.T) {
	// Catches accidental enum collisions (e.g. two constants sharing a
	// string). reflect.DeepEqual on the maps would miss this because
	// the duplicate would silently collapse; we test by explicit count.
	intents := []Intent{
		IntentExplain, IntentRootCause, IntentTrace, IntentEnumerate,
		IntentConfigQuery, IntentReturnValue, IntentUnknown,
	}
	seen := make(map[Intent]bool, len(intents))
	for _, i := range intents {
		if seen[i] {
			t.Fatalf("duplicate Intent constant: %q", i)
		}
		seen[i] = true
	}

	// AnswerShape constants retired in PR5 of the AnswerShape
	// terminal-retirement migration; their cross-uniqueness test is
	// gone with them. The replacement is the QuestionFamily enum
	// covered by answer_semantic_view_test.go.

	nodeTypes := []TaskNodeType{
		NodeProbe, NodeEvidence, NodeValidate, NodeReconcile, NodeExtract, NodeFinalize,
	}
	seenNode := make(map[TaskNodeType]bool, len(nodeTypes))
	for _, n := range nodeTypes {
		if seenNode[n] {
			t.Fatalf("duplicate TaskNodeType constant: %q", n)
		}
		seenNode[n] = true
	}
}

func TestAnalysisIR_EmptyMarshal(t *testing.T) {
	// An empty IR must round-trip cleanly — omitempty tags should not
	// cause NaN/nil-map panics.
	var ir AnalysisIR
	data, err := json.Marshal(&ir)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	var decoded AnalysisIR
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}
	if !reflect.DeepEqual(ir, decoded) {
		t.Fatalf("empty roundtrip mismatch: %#v vs %#v", ir, decoded)
	}
}
