// Package context — log_triage_p2_test.go (2026-05-10).
//
// P2 audit follow-up to Fix-C (f01ae4b): the parallel-frame signal
// in renderPrimaryErrorSignal must NOT advertise a "concurrency /
// race" framing unless a deterministic corroborator (BugClassRace
// detector OR race-vocabulary keyword in the error type/message)
// agrees. Without the gate, recursion / fan-out / repeated
// exception wrapping / unrelated co-located errors would be
// framed as race conditions and bias the LLM toward incorrect
// concurrency theories.
package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === bundleHasRaceCorroboration ===

func TestBundleHasRaceCorroboration_NilBundle_False(t *testing.T) {
	if bundleHasRaceCorroboration(nil) {
		t.Error("nil bundle: expected false")
	}
}

func TestBundleHasRaceCorroboration_BugClassRaceDetected_True(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{
			BugClasses: []types.DetectedBugClass{
				{Class: types.BugClassRace, HumanEn: "data race"},
			},
		},
	}
	if !bundleHasRaceCorroboration(bundle) {
		t.Error("BugClassRace present: expected true")
	}
}

func TestBundleHasRaceCorroboration_NonRaceBugClass_False(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{
			BugClasses: []types.DetectedBugClass{
				{Class: "nil_deref", HumanEn: "nil pointer dereference"},
			},
		},
	}
	if bundleHasRaceCorroboration(bundle) {
		t.Error("non-race BugClass: expected false")
	}
}

func TestBundleHasRaceCorroboration_KeywordInMessage_True(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"concurrent_map_writes", "concurrent map writes", true},
		{"data_race", "data race detected at /path/to/foo.go", true},
		{"chinese_数据竞争", "数据竞争在第10行", true},
		{"race_condition", "race condition between goroutines", true},
		{"deadlock", "deadlock detected", true},
		{"chinese_死锁", "线程间死锁", true},
		{"concurrent_write", "concurrent write to slice", true},
		{"non_race_recursion", "maximum recursion depth exceeded", false},
		{"non_race_nil", "nil pointer dereference", false},
		{"non_race_panic", "runtime panic", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle := &types.LogBundle{
				Errors: []types.LogError{{Type: "fatal error", Message: tc.msg}},
			}
			if got := bundleHasRaceCorroboration(bundle); got != tc.want {
				t.Errorf("msg=%q got %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}

func TestBundleHasRaceCorroboration_KeywordInCauseChain_True(t *testing.T) {
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{
				Type:    "WrappingException",
				Message: "outer wrapper",
				Cause: &types.LogError{
					Type:    "RaceConditionError",
					Message: "data race detected",
				},
			},
		},
	}
	if !bundleHasRaceCorroboration(bundle) {
		t.Error("race keyword in cause chain: expected true")
	}
}

// === renderPrimaryErrorSignal — neutral vs corroborated framing ===

func TestRenderPrimaryErrorSignal_ParallelFrames_NoRaceCorroboration_NeutralFraming(t *testing.T) {
	// 3 frames at same coordinate but no race signal — should
	// frame as neutral co-occurrence, NOT concurrency.
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{Type: "panic", Message: "nil pointer dereference",
				Frames: []types.LogFrame{
					{File: "internal/foo.go", Line: 10},
					{File: "internal/foo.go", Line: 10},
					{File: "internal/foo.go", Line: 10},
				}},
		},
	}
	got := renderPrimaryErrorSignal(bundle)
	if !strings.Contains(got, "(neutral)") {
		t.Errorf("expected neutral framing without race corroboration; got %q", got)
	}
	// Must NOT advertise race / concurrency interpretation.
	if strings.Contains(got, "race-corroborated") {
		t.Errorf("non-race parallel frames should not be framed as race-corroborated; got %q", got)
	}
}

func TestRenderPrimaryErrorSignal_ParallelFrames_BugClassRace_CorroboratedFraming(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{
			BugClasses: []types.DetectedBugClass{
				{Class: types.BugClassRace, HumanEn: "data race"},
			},
		},
		Errors: []types.LogError{
			{Type: "fatal error", Message: "concurrent map writes",
				Frames: []types.LogFrame{
					{File: "internal/agent/analyzer.go", Line: 100},
					{File: "internal/agent/analyzer.go", Line: 100},
				}},
		},
	}
	got := renderPrimaryErrorSignal(bundle)
	if !strings.Contains(got, "race-corroborated") {
		t.Errorf("race-corroborated framing missing when BugClassRace fired; got %q", got)
	}
}

func TestRenderPrimaryErrorSignal_ParallelFrames_RaceKeyword_CorroboratedFraming(t *testing.T) {
	// No BugClass detected, but error message mentions race.
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{Type: "fatal error", Message: "concurrent write to map",
				Frames: []types.LogFrame{
					{File: "x.go", Line: 5},
					{File: "x.go", Line: 5},
				}},
		},
	}
	got := renderPrimaryErrorSignal(bundle)
	if !strings.Contains(got, "race-corroborated") {
		t.Errorf("race-keyword framing missing; got %q", got)
	}
}

func TestRenderPrimaryErrorSignal_ParallelFrames_RecursionPattern_NeutralFraming(t *testing.T) {
	// Recursion stack — same line N times, but no race signal.
	// Must be framed neutrally.
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{Type: "RecursionError", Message: "maximum recursion depth exceeded",
				Frames: []types.LogFrame{
					{File: "lib/recursive.py", Line: 42},
					{File: "lib/recursive.py", Line: 42},
					{File: "lib/recursive.py", Line: 42},
					{File: "lib/recursive.py", Line: 42},
				}},
		},
	}
	got := renderPrimaryErrorSignal(bundle)
	if strings.Contains(got, "race-corroborated") {
		t.Errorf("recursion → race framing leak; got %q", got)
	}
	if !strings.Contains(got, "non-race causes include recursion") {
		t.Errorf("expected explicit recursion mention in neutral framing; got %q", got)
	}
}
