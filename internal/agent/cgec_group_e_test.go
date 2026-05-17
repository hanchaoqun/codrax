package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// (TestInferAnswerSubject_CueMatch / CueOverridesLLM /
// LLMWinsWhenNoCueMatch removed in the Session 11 over-fitting
// audit: they validated a hard-coded "prose-pattern → kind"
// cue table that existed only to cater to a specific ZH/EN
// vocabulary observed in codrax eval traces. The cue table is
// gone; inference now flows through the generic question_kind
// fallback exercised by the test below.)

// TestInferAnswerSubject_QuestionKindFallback covers the
// question_kind → kind fallback: no codrax-specific cues fire,
// only the generic enum→enum mapping. Language-neutral.
func TestInferAnswerSubject_QuestionKindFallback(t *testing.T) {
	tests := []struct {
		kind     string
		wantSubj types.AnswerSubjectKind
	}{
		{"config_mapping", types.SubjectConfigKey},
		{"registration", types.SubjectGeneric},
		{"return_value", types.SubjectReturnValue},
		{"call_chain", types.SubjectFunctionName},
		{"enumeration", types.SubjectGeneric},
		{"mechanism", types.SubjectGeneric},
		{"conditional", types.SubjectGeneric},
	}
	for _, tt := range tests {
		rm := types.RequestModel{}
		rm.AnalyzerHints.Kind = tt.kind
		subj, reason := inferAnswerSubject(rm)
		if subj.Kind != tt.wantSubj {
			t.Errorf("kind=%s: got %s, want %s (reason=%s)", tt.kind, subj.Kind, tt.wantSubj, reason)
		}
	}
}

// TestInferAnswerSubject_HardFallback_NeverUnknown is the E1
// regression: after all cue and question_kind paths miss, the
// function MUST return SubjectGeneric with low confidence instead
// of the old SubjectUnknown zero value. This ensures every
// downstream subject-aware consumer (rankChainsBySubject,
// preCompleteContractCheck E3) gets a usable non-zero kind rather than
// silently deactivating.
func TestInferAnswerSubject_HardFallback_NeverUnknown(t *testing.T) {
	rm := types.RequestModel{}
	rm.AnalyzerHints.Kind = "unknown"
	subj, reason := inferAnswerSubject(rm)
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
	subj, _ := inferAnswerSubject(rm)
	if subj.Kind != types.SubjectGeneric {
		t.Errorf("empty question_kind + no cue must fallback to Generic, got %s", subj.Kind)
	}
}
