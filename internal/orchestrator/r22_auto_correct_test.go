// Package orchestrator — r22_auto_correct_test.go (2026-05-10).
//
// Fix-D of the analyzer-failure remediation: when the gate rejects
// an AnalysisIR purely because of the R2.2 longform_scalar_subject
// shape contradiction AND the analyzer LLM has had a chance to
// revise on retry (attempt ≥ 1) AND keeps emitting the same scalar
// AnswerSubject.Kind, the orchestrator auto-clears the scalar
// declaration and re-runs the gate.
package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// === r22AutoCorrectShapeSubject ===

func TestR22AutoCorrect_NilIR_NoOp(t *testing.T) {
	if r22AutoCorrectShapeSubject(nil) {
		t.Error("nil IR: expected false; got true")
	}
}

func TestR22AutoCorrect_NotRejected_NoOp(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectStringLiteral
	ir.RequestModel.AnswerSubject.Confidence = 0.9
	ir.QualityGate.Rejected = false
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("not-rejected: expected false; got true")
	}
}

func TestR22AutoCorrect_RejectedButNotR22_NoOp(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectStringLiteral
	ir.RequestModel.AnswerSubject.Confidence = 0.9
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("non-R2.2 failure: expected false (don't broaden auto-correct surface)")
	}
}

func TestR22AutoCorrect_R22Fires_StringLiteralCleared(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectStringLiteral
	ir.RequestModel.AnswerSubject.Confidence = 0.9
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: family=root_cause_trace expects non-scalar payload but AnswerSubject.Kind=string_literal at confidence 0.90"},
	}
	if !r22AutoCorrectShapeSubject(ir) {
		t.Fatal("R2.2 + scalar string_literal: expected true (correction applied)")
	}
	if ir.RequestModel.AnswerSubject.Kind != types.SubjectUnknown {
		t.Errorf("kind should be cleared to Unknown; got %q", ir.RequestModel.AnswerSubject.Kind)
	}
	if ir.RequestModel.AnswerSubject.Confidence != 0 {
		t.Errorf("confidence should be cleared to 0; got %v", ir.RequestModel.AnswerSubject.Confidence)
	}
}

func TestR22AutoCorrect_R22Fires_NumericCleared(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectNumeric
	ir.RequestModel.AnswerSubject.Confidence = 0.7
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: family=architecture expects non-scalar payload but AnswerSubject.Kind=numeric at confidence 0.70"},
	}
	if !r22AutoCorrectShapeSubject(ir) {
		t.Fatal("R2.2 + scalar numeric: expected correction")
	}
}

func TestR22AutoCorrect_R22Fires_ReturnValueCleared(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectReturnValue
	ir.RequestModel.AnswerSubject.Confidence = 0.85
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: family=call_chain expects non-scalar payload but AnswerSubject.Kind=return_value at confidence 0.85"},
	}
	if !r22AutoCorrectShapeSubject(ir) {
		t.Fatal("R2.2 + scalar return_value: expected correction")
	}
}

func TestR22AutoCorrect_R22Fires_ButNonScalarKind_NoCorrection(t *testing.T) {
	// R2.2 wouldn't fire on a non-scalar kind in normal flow,
	// but defensively: if the IR somehow carries function_name
	// AND a R2.2 detail string (corrupted state), don't mutate
	// the kind — the caller's gate re-run will handle it.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectFunctionName
	ir.RequestModel.AnswerSubject.Confidence = 0.9
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: ..."},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("R2.2 + non-scalar kind: expected no correction (defensive)")
	}
}

func TestR22AutoCorrect_AlreadyUnknown_NoOp(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectUnknown
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: ..."},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("already Unknown: expected no correction (nothing to clear)")
	}
}

func TestR22AutoCorrect_R22ButPassedCheck_NoOp(t *testing.T) {
	// shape_subject_coherence appears as PASSED check — should
	// not trigger correction even though the Detail string
	// might mention R2.2 historically.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject.Kind = types.SubjectStringLiteral
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: true, Detail: "family=generic subject_kind=string_literal scalar_pred=true sub_topics=0"},
		{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("shape_subject_coherence passed: expected no correction (different failure mode)")
	}
}

// === Resolver-omission safety guards (2026-05-10 audit follow-up) ===

func TestR22AutoCorrect_R22PlusSubtopicFail_NoOp(t *testing.T) {
	// CRITICAL safety: when R2.2 fires alongside a resolver-backed
	// coherence failure (R1.4 / R1.5 in subtopic_coherence), the
	// auto-correct must REFUSE. The post-correction re-run uses a
	// nil resolver, which makes R1.4/R1.5 no-ops — without this
	// guard, the coherence failure would be silently bypassed.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject = types.AnswerSubject{Kind: types.SubjectStringLiteral, Confidence: 0.9}
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: family=root_cause_trace expects non-scalar payload but AnswerSubject.Kind=string_literal at confidence 0.90"},
		{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("R2.2 + R1.4: expected NO correction (would bypass resolver-backed R1.4 on re-run)")
	}
	// The IR's AnswerSubject must NOT have been mutated.
	if ir.RequestModel.AnswerSubject.Kind != types.SubjectStringLiteral {
		t.Errorf("expected scalar Kind preserved when correction refused; got %v", ir.RequestModel.AnswerSubject.Kind)
	}
}

func TestR22AutoCorrect_R22PlusContractFail_NoOp(t *testing.T) {
	// Same guard for non-coherence checks: contract_complete,
	// dag_closure, etc. Any other failed check blocks auto-correct
	// to keep the LLM-side retry path responsible for resolving
	// them. The fast-path re-run cannot validate non-R2.2 rules
	// without the original dispatch context.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject = types.AnswerSubject{Kind: types.SubjectNumeric, Confidence: 0.7}
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: ..."},
		{Name: "contract_complete", Passed: false, Detail: "missing exact-resolution contract"},
	}
	if r22AutoCorrectShapeSubject(ir) {
		t.Error("R2.2 + contract_complete fail: expected NO correction")
	}
}

func TestR22AutoCorrect_R22Solo_StillCorrects(t *testing.T) {
	// Sanity: when R2.2 is the SOLE failed check (other checks
	// either passed or aren't present), correction proceeds
	// normally — preserves the happy-path Fix-D contract.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnswerSubject = types.AnswerSubject{Kind: types.SubjectStringLiteral, Confidence: 0.9}
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: ..."},
		{Name: "coverage", Passed: true, Detail: "ok"},
		{Name: "dag_closure", Passed: true, Detail: "ok"},
	}
	if !r22AutoCorrectShapeSubject(ir) {
		t.Fatal("R2.2 sole-fail: expected correction to proceed")
	}
	if ir.RequestModel.AnswerSubject.Kind != types.SubjectUnknown {
		t.Errorf("kind should be cleared; got %v", ir.RequestModel.AnswerSubject.Kind)
	}
}

// === R1.4 single-axis enumeration auto-collapse (2026-05-11 eval audit) ===

func TestR14AutoCollapseEnumerationSubtopics_CollapsesSoleAxisFailure(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.Intent = types.IntentEnumerate
	ir.RequestModel.Predicates.IsCategoryEnumeration = true
	ir.RequestModel.AnalyzerHints.Entities = []string{"PipelineStage"}
	ir.RequestModel.AnalyzerHints.DerivedEntities = []string{"StageAnalyze"}
	ir.RequestModel.SubTopics = []types.SubTopic{
		{Summary: "analysis and exploration", Entities: []string{"StageAnalyze", "StageExplore"}},
		{Summary: "extraction and finalization", Entities: []string{"StageExtract", "StageFinalize"}},
	}
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: 2 sub-topics all sit in the same code area [types] under enumeration intent"},
		{Name: "coverage", Passed: true, Detail: "ok"},
	}

	if !r14AutoCollapseEnumerationSubtopics(ir) {
		t.Fatal("R1.4 sole-fail enumeration: expected deterministic collapse")
	}
	if len(ir.RequestModel.SubTopics) != 0 {
		t.Fatalf("SubTopics should be cleared; got %#v", ir.RequestModel.SubTopics)
	}
	for _, want := range []string{"PipelineStage", "StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"} {
		if !testStringSliceContains(ir.RequestModel.AnalyzerHints.Entities, want) {
			t.Fatalf("AnalyzerHints.Entities missing %q after collapse: %#v", want, ir.RequestModel.AnalyzerHints.Entities)
		}
	}
	for _, want := range []string{"StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"} {
		if !testStringSliceContains(ir.RequestModel.AnalyzerHints.DerivedEntities, want) {
			t.Fatalf("AnalyzerHints.DerivedEntities missing %q after collapse: %#v", want, ir.RequestModel.AnalyzerHints.DerivedEntities)
		}
	}
	if ir.QualityGate.Rejected || len(ir.QualityGate.Checks) != 0 {
		t.Fatalf("QualityGate should be cleared for caller re-run; got %#v", ir.QualityGate)
	}
}

func TestR14AutoCollapseEnumerationSubtopics_RequiresEnumerationTypedIntent(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.Intent = types.IntentExplain
	ir.RequestModel.SubTopics = []types.SubTopic{
		{Summary: "one", Entities: []string{"A"}},
		{Summary: "two", Entities: []string{"B"}},
	}
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
	}

	if r14AutoCollapseEnumerationSubtopics(ir) {
		t.Fatal("non-enumeration typed request: expected no collapse")
	}
	if len(ir.RequestModel.SubTopics) != 2 {
		t.Fatalf("SubTopics should be preserved; got %#v", ir.RequestModel.SubTopics)
	}
}

func TestR14AutoCollapseEnumerationSubtopics_RefusesMixedFailures(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.Intent = types.IntentEnumerate
	ir.RequestModel.Predicates.IsCategoryEnumeration = true
	ir.RequestModel.SubTopics = []types.SubTopic{
		{Summary: "one", Entities: []string{"A"}},
		{Summary: "two", Entities: []string{"B"}},
	}
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
		{Name: "contract_complete", Passed: false, Detail: "missing exact-resolution contract"},
	}

	if r14AutoCollapseEnumerationSubtopics(ir) {
		t.Fatal("R1.4 plus another failed check: expected no collapse")
	}
	if len(ir.RequestModel.SubTopics) != 2 {
		t.Fatalf("SubTopics should be preserved on mixed failure; got %#v", ir.RequestModel.SubTopics)
	}
}

func TestR14AutoCollapseEnumerationSubtopics_PreservesCrossComponentSubtopics(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.Intent = types.IntentEnumerate
	ir.RequestModel.Predicates.IsCategoryEnumeration = true
	ir.RequestModel.Predicates.IsCrossComponent = true
	ir.RequestModel.SubTopics = []types.SubTopic{
		{Summary: "repo a", Entities: []string{"A"}},
		{Summary: "repo b", Entities: []string{"B"}},
	}
	ir.QualityGate.Rejected = true
	ir.QualityGate.Checks = []types.GateCheck{
		{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
	}

	if r14AutoCollapseEnumerationSubtopics(ir) {
		t.Fatal("cross-component request: expected no collapse")
	}
}

func TestAutoCorrectAnalyzerStageOutput_R14ClearsErrorBeforeApply(t *testing.T) {
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mode:      types.ModeRead,
			Mutable:   types.NewMutableState("which values are registered"),
			TaskState: types.TaskState{LastError: "old analyzer gate error"},
		},
	}
	out := &agent.StageOutput{
		Error:        "analyzer quality gate rejected: subtopic_coherence",
		MissingPiece: types.MissingUnderstanding,
		RetryHint:    "retry analyzer",
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				SubTopics: []types.SubTopic{
					{Summary: "one", Entities: []string{"A"}},
					{Summary: "two", Entities: []string{"B"}},
				},
			},
			TaskGraph: types.TaskGraph{
				Nodes: []types.TaskNode{
					{ID: "n0_probe", Type: types.NodeProbe, Objective: "probe", Hypotheses: []string{"h0"}},
					{ID: "n1_evidence_t0", Type: types.NodeEvidence, Objective: "collect stale lane 0", Hypotheses: []string{"h0"}},
					{ID: "n1_evidence_t1", Type: types.NodeEvidence, Objective: "collect stale lane 1", Hypotheses: []string{"h0"}},
					{ID: "n1_evidence_t2", Type: types.NodeEvidence, Objective: "collect stale lane 2", Hypotheses: []string{"h0"}},
					{ID: "n2_finalize", Type: types.NodeFinalize, Objective: "render", Hypotheses: []string{"h0"}},
				},
				Edges: []types.TaskEdge{
					{From: "n0_probe", To: "n1_evidence_t0", EdgeType: types.EdgeHardDependency},
					{From: "n0_probe", To: "n1_evidence_t1", EdgeType: types.EdgeHardDependency},
					{From: "n0_probe", To: "n1_evidence_t2", EdgeType: types.EdgeHardDependency},
					{From: "n1_evidence_t0", To: "n2_finalize", EdgeType: types.EdgeHardDependency},
					{From: "n1_evidence_t1", To: "n2_finalize", EdgeType: types.EdgeHardDependency},
					{From: "n1_evidence_t2", To: "n2_finalize", EdgeType: types.EdgeHardDependency},
				},
				ExecutionPolicy: types.ExecutionPolicy{
					MaxParallelism: 1,
					RetryBudget:    1,
					CriticalPath:   []string{"n0_probe", "n1_evidence_t0", "n2_finalize"},
				},
			},
			EvidencePlan: types.EvidencePlan{
				Budget: types.EvidenceBudget{MaxFiles: 5, MaxReactIters: 8},
			},
			AnswerContract: types.AnswerContract{Language: "en"},
			QualityGate: types.GateReport{
				Rejected: true,
				Checks: []types.GateCheck{
					{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
				},
			},
		},
	}

	if !o.autoCorrectAnalyzerStageOutput(out, 0) {
		t.Fatal("R1.4 stage output: expected auto-correct to pass")
	}
	if out.Error != "" {
		t.Fatalf("stage error should be cleared before apply/render; got %q", out.Error)
	}
	if out.MissingPiece != types.MissingNone {
		t.Fatalf("missing piece = %q, want %q", out.MissingPiece, types.MissingNone)
	}
	if out.RetryHint != "" {
		t.Fatalf("retry hint should be cleared; got %q", out.RetryHint)
	}
	if o.busCtx.TaskState.LastError != "" {
		t.Fatalf("bus LastError should be cleared; got %q", o.busCtx.TaskState.LastError)
	}
	if out.AnalysisIR.QualityGate.Rejected {
		t.Fatalf("quality gate should pass after correction: %#v", out.AnalysisIR.QualityGate)
	}
	if got := testCountTaskNodes(out.AnalysisIR.TaskGraph, types.NodeEvidence); got != 1 {
		t.Fatalf("R1.4 correction should recompile away stale subtopic evidence lanes; got %d evidence nodes in %#v", got, out.AnalysisIR.TaskGraph.Nodes)
	}
	for _, n := range out.AnalysisIR.TaskGraph.Nodes {
		if n.ID == "n1_evidence_t0" || n.ID == "n1_evidence_t1" || n.ID == "n1_evidence_t2" {
			t.Fatalf("stale pre-correction evidence lane survived rebuild: %q", n.ID)
		}
	}
	if got := o.busCtx.Mutable.RequestModel(); got == nil || len(got.SubTopics) != 0 {
		t.Fatalf("mutable request model should reflect corrected subtopic collapse; got %#v", got)
	}
}

func testStringSliceContains(in []string, want string) bool {
	for _, got := range in {
		if got == want {
			return true
		}
	}
	return false
}

func testCountTaskNodes(graph types.TaskGraph, typ types.TaskNodeType) int {
	count := 0
	for _, n := range graph.Nodes {
		if n.Type == typ {
			count++
		}
	}
	return count
}
