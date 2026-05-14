package cmd

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeCompatArgs_RewritesLegacySingleDashLongFlags(t *testing.T) {
	got := normalizeCompatArgs([]string{
		"-repo", ".",
		"-branch=main",
		"-request", "trace analyzer",
		"-pipeline-max-steps", "50",
		"-chitchat-classifier=true",
	})
	want := []string{
		"--repo", ".",
		"--branch=main",
		"--request", "trace analyzer",
		"--pipeline-max-steps", "50",
		"--chitchat-classifier=true",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeCompatArgs mismatch:\n  got:  %#v\n  want: %#v", got, want)
	}
}

func TestNormalizeCompatArgs_LeavesShortFlagsAndPositionalsUntouched(t *testing.T) {
	in := []string{"-r", "task", "--request", "task2", "positional", "-x"}
	got := normalizeCompatArgs(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("short flags/positionals changed:\n  got:  %#v\n  want: %#v", got, in)
	}
}

func TestNormalizeCompatArgs_StopsRewritingAfterDoubleDash(t *testing.T) {
	in := []string{"-repo", ".", "--", "-request", "literal"}
	got := normalizeCompatArgs(in)
	want := []string{"--repo", ".", "--", "-request", "literal"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("double-dash stop mismatch:\n  got:  %#v\n  want: %#v", got, want)
	}
}

func TestLoopPolicyFromAgentSettingsPreservesQuotaBurnGuards(t *testing.T) {
	settings := types.DefaultAgentSettings()
	settings.LoopMinInjectInterval = 1
	settings.LoopMaxContinuations = 2
	settings.LoopMaxMidLoopInjects = 3
	settings.LoopIdleStopThreshold = 4

	got := loopPolicyFromAgentSettings(settings)
	defaults := agent.DefaultLoopPolicy()
	if got.MinInjectInterval != 1 ||
		got.MaxContinuations != 2 ||
		got.MaxMidLoopInjects != 3 ||
		got.IdleStopThreshold != 4 {
		t.Fatalf("settings-backed loop fields not applied: %+v", got)
	}
	if got.IdenticalToolCallAfterSuccessStreak != defaults.IdenticalToolCallAfterSuccessStreak ||
		got.IdenticalToolCallAfterFailureStreak != defaults.IdenticalToolCallAfterFailureStreak ||
		got.IdenticalErrorStreak != defaults.IdenticalErrorStreak ||
		got.MaxPerKeyInjects != defaults.MaxPerKeyInjects {
		t.Fatalf("quota-burn guards should inherit DefaultLoopPolicy values, got %+v defaults %+v", got, defaults)
	}
}
