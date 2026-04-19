package cmd

import (
	"reflect"
	"testing"
)

func TestNormalizeCompatArgs_RewritesLegacySingleDashLongFlags(t *testing.T) {
	got := normalizeCompatArgs([]string{
		"-repo", ".",
		"-branch=main",
		"-request", "trace analyzer",
		"-pipeline-max-steps", "50",
	})
	want := []string{
		"--repo", ".",
		"--branch=main",
		"--request", "trace analyzer",
		"--pipeline-max-steps", "50",
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
