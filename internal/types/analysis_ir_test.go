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
		},
		TaskGraph: TaskGraph{
			Nodes: []TaskNode{
				{
					ID:              "n1",
					Type:            NodeProbe,
					Objective:       "locate ShouldStop definition",
					Inputs:          []string{"t2"},
					Outputs:         []string{"shouldstop_impl"},
					ExitArtifacts:   []string{"file_line_ref"},
					SuccessCriteria: []Criterion{{Kind: "symbol_present", Expr: "ShouldStop"}},
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
				{Kind: "all_hypotheses_decided"},
				{Kind: "contract_satisfied"},
			},
		},
		AnswerContract: AnswerContract{
			RequiredAnswerShape: ShapeExplanation,
			MustInclude:         []string{"t2"},
			MustExclude:         nil,
			CitationReq:         CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
			AcceptanceTests: []Criterion{
				{Kind: "contains_symbol", Expr: "ShouldStop"},
				{Kind: "citation_count_ge", Expr: "2"},
			},
			Language: "zh",
		},
		HypothesisSet: []Hypothesis{
			{
				ID:        "h1",
				Statement: "ShouldStop returns true when ERM is satisfied",
				RequiredEvidence: []Criterion{
					{Kind: "symbol_present", Expr: "ermSatisfied"},
				},
				FalsificationCondition: Criterion{Kind: "no_call_sites", Expr: "ermSatisfied"},
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

func TestAnalysisIR_VersionConstant(t *testing.T) {
	if AnalysisIRVersion != "v3" {
		t.Fatalf("unexpected AnalysisIRVersion: %q", AnalysisIRVersion)
	}
	ir := AnalysisIR{Version: AnalysisIRVersion}
	if ir.Version != "v3" {
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

	shapes := []AnswerShape{
		ShapeListOfSymbols, ShapeStepList, ShapeValue, ShapeBoolean,
		ShapeConfigValue, ShapeExplanation, ShapeNone,
	}
	seenShape := make(map[AnswerShape]bool, len(shapes))
	for _, s := range shapes {
		if seenShape[s] {
			t.Fatalf("duplicate AnswerShape constant: %q", s)
		}
		seenShape[s] = true
	}

	nodeTypes := []TaskNodeType{
		NodeProbe, NodeEvidence, NodeValidate, NodeReconcile, NodeFinalize,
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
