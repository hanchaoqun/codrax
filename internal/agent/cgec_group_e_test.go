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

// TestInferAnswerSubject_CueOverridesLLM asserts that when the LLM
// classified the subject AND a curated cue pattern matches, the
// cue WINS. The curated cues are high-signal bilingual explicit
// patterns (e.g. "哪个 skill" → SkillName); the LLM's own
// AnswerSubject classification has been observed to mis-label
// "agent → skill" questions as SubjectConfigKey (post-Session-10
// 2026-04-18T14:25 log). A cue match at confidence 0.8 overrides
// the LLM and the reason string flags the override for the trace.
func TestInferAnswerSubject_CueOverridesLLM(t *testing.T) {
	rm := types.RequestModel{}
	rm.AnswerSubject = types.AnswerSubject{
		Kind:       types.SubjectConfigKey,
		Confidence: 0.9,
		EntityAxes: []string{"agent → skill"}, // LLM got axes right
	}
	subj, reason := inferAnswerSubject(rm, "codrax 的 explorer agent 默认用哪个skill?")
	if subj.Kind != types.SubjectSkillName {
		t.Errorf("cue must override LLM-supplied kind, got %s (reason=%s)", subj.Kind, reason)
	}
	if !strings.Contains(reason, "overriding LLM=") {
		t.Errorf("reason must flag the LLM override, got %q", reason)
	}
	// EntityAxes: LLM-supplied "agent → skill" preserved (better
	// than the static cue axes).
	if len(subj.EntityAxes) != 1 || subj.EntityAxes[0] != "agent → skill" {
		t.Errorf("LLM-supplied EntityAxes must survive cue override, got %v", subj.EntityAxes)
	}
}

// TestInferAnswerSubject_LLMWinsWhenNoCueMatch asserts that when
// no cue matches, the LLM-supplied kind is honoured verbatim —
// cue precedence only applies to the curated pattern set.
func TestInferAnswerSubject_LLMWinsWhenNoCueMatch(t *testing.T) {
	rm := types.RequestModel{}
	rm.AnswerSubject = types.AnswerSubject{
		Kind:       types.SubjectAgentName,
		Confidence: 0.9,
	}
	subj, reason := inferAnswerSubject(rm, "arbitrary prose that matches no cue")
	if subj.Kind != types.SubjectAgentName {
		t.Errorf("LLM-supplied kind must win when no cue matches, got %s (reason=%s)", subj.Kind, reason)
	}
	if reason != "" {
		t.Errorf("reason should be empty when LLM-supplied path fired, got %q", reason)
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
