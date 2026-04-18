package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestInferAnswerSubject_CueMatch covers the primary E2 bilingual
// cue path: "默认用哪个skill" in the question matches the
// SkillName cue and produces confidence=0.8.
func TestInferAnswerSubject_CueMatch(t *testing.T) {
	rm := types.RequestModel{}
	subj, reason := inferAnswerSubject(rm, "codrax 的 explorer agent 默认用哪个skill?")
	if subj.Kind != types.SubjectSkillName {
		t.Errorf("expected SkillName, got %s (reason=%s)", subj.Kind, reason)
	}
	if subj.Confidence != 0.8 {
		t.Errorf("expected confidence 0.8 for cue match, got %.2f", subj.Confidence)
	}
	if !strings.Contains(reason, "cue match") {
		t.Errorf("reason should mention cue match, got %q", reason)
	}
}

// TestInferAnswerSubject_LLMSuppliedWins asserts that when the LLM
// explicitly provided a Kind (rm.AnswerSubject.Kind != Unknown), the
// inference function returns it verbatim without running the cue
// table.
func TestInferAnswerSubject_LLMSuppliedWins(t *testing.T) {
	rm := types.RequestModel{}
	rm.AnswerSubject = types.AnswerSubject{
		Kind:       types.SubjectAgentName,
		Confidence: 0.9,
	}
	// Question has a skill cue but LLM declared AgentName — LLM wins.
	subj, reason := inferAnswerSubject(rm, "默认用哪个skill?")
	if subj.Kind != types.SubjectAgentName {
		t.Errorf("LLM-supplied kind must win, got %s (reason=%s)", subj.Kind, reason)
	}
	if reason != "" {
		t.Errorf("reason should be empty when LLM-supplied, got %q", reason)
	}
}

// TestInferAnswerSubject_QuestionKindFallback covers the tier-2
// fallback: when no cue matches, the question_kind field picks a
// subject with confidence 0.4 (or 0.3 for enumeration).
func TestInferAnswerSubject_QuestionKindFallback(t *testing.T) {
	tests := []struct {
		kind     string
		wantSubj types.AnswerSubjectKind
	}{
		{"config_mapping", types.SubjectConfigKey},
		{"registration", types.SubjectAgentName},
		{"return_value", types.SubjectReturnValue},
		{"call_chain", types.SubjectFunctionName},
		{"enumeration", types.SubjectGeneric},
		{"mechanism", types.SubjectGeneric},
		{"conditional", types.SubjectGeneric},
	}
	for _, tt := range tests {
		rm := types.RequestModel{}
		rm.AnalyzerHints.Kind = tt.kind
		subj, reason := inferAnswerSubject(rm, "totally neutral prose that matches nothing")
		if subj.Kind != tt.wantSubj {
			t.Errorf("kind=%s: got %s, want %s (reason=%s)", tt.kind, subj.Kind, tt.wantSubj, reason)
		}
	}
}

// TestInferAnswerSubject_HardFallback_NeverUnknown is the E1
// regression: after all cue and question_kind paths miss, the
// function MUST return SubjectGeneric with low confidence instead
// of the old SubjectUnknown zero value. This ensures every
// downstream subject-aware consumer (reconcileShape, rankChainsBySubject,
// preCompleteContractCheck E3) gets a usable non-zero kind rather
// than silently deactivating.
func TestInferAnswerSubject_HardFallback_NeverUnknown(t *testing.T) {
	rm := types.RequestModel{}
	rm.AnalyzerHints.Kind = "unknown"
	subj, reason := inferAnswerSubject(rm, "unmatched prose with no cue and no question_kind")
	if subj.Kind == types.SubjectUnknown {
		t.Errorf("hard fallback must NEVER return SubjectUnknown, got empty")
	}
	if subj.Kind != types.SubjectGeneric {
		t.Errorf("hard fallback kind=%s, want SubjectGeneric", subj.Kind)
	}
	if subj.Confidence >= 0.4 {
		t.Errorf("hard fallback confidence=%.2f, must be weakest (< 0.4)", subj.Confidence)
	}
	if !strings.Contains(reason, "hard fallback") {
		t.Errorf("reason should mark the hard fallback, got %q", reason)
	}
}

// TestInferAnswerSubject_EmptyQuestionKind also triggers the hard
// fallback. An empty AnalyzerHints.Kind (LLM forgot the field) must
// land on SubjectGeneric rather than the zero value.
func TestInferAnswerSubject_EmptyQuestionKind(t *testing.T) {
	rm := types.RequestModel{}
	subj, _ := inferAnswerSubject(rm, "plain text")
	if subj.Kind != types.SubjectGeneric {
		t.Errorf("empty question_kind + no cue must fallback to Generic, got %s", subj.Kind)
	}
}
