package types

import (
	"strings"
	"testing"
)

func TestNormalizeCurrentSourceExplanationProfile_AnchoredSoftProfile(t *testing.T) {
	raw := "结合这段 trace 和当前源码解释调度链路，并说明现在实现是否还会触发"
	profile, warnings := NormalizeCurrentSourceExplanationProfile(raw, &CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes: []CurrentSourceExplanationMode{
			CurrentSourceExplanationTraceCurrentFlow,
			CurrentSourceExplanationMode("unknown_future_mode"),
			CurrentSourceExplanationTraceCurrentFlow,
		},
		SourceQuotes: []string{
			"当前源码解释",
			"现在实现是否还会触发",
			"prior turn only",
			"当前源码解释",
		},
		TargetTerms: []string{"调度链路", "trace", "调度链路"},
		Confidence:  1.2,
		Rationale:   "mixed trace/current source request",
	})
	if profile == nil || !profile.Active() {
		t.Fatalf("profile should survive normalization, got %+v warnings=%v", profile, warnings)
	}
	if got, want := profile.SourceQuotes, []string{"当前源码解释", "现在实现是否还会触发"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("source quotes=%v want %v", got, want)
	}
	if got, want := profile.Modes, []CurrentSourceExplanationMode{
		CurrentSourceExplanationTraceCurrentFlow,
		CurrentSourceExplanationOther,
	}; strings.Join(currentSourceExplanationModeStrings(got), "|") != strings.Join(currentSourceExplanationModeStrings(want), "|") {
		t.Fatalf("modes=%v want %v", got, want)
	}
	if got, want := profile.TargetTerms, []string{"调度链路", "trace"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("target terms=%v want %v", got, want)
	}
	if profile.Confidence != 1 {
		t.Fatalf("confidence=%v want clamped 1", profile.Confidence)
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, "\n"), "unanchored source_quote") {
		t.Fatalf("warnings should mention unanchored source quote, got %v", warnings)
	}
}

func TestNormalizeCurrentSourceExplanationProfile_DropsUnanchoredOptionalProfile(t *testing.T) {
	profile, warnings := NormalizeCurrentSourceExplanationProfile("只总结日志里发生了什么", &CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
		SourceQuotes:                        []string{"current source"},
		Confidence:                          0.8,
	})
	if profile != nil {
		t.Fatalf("unanchored profile must be dropped, got %+v", profile)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "ignored because no source_quote survived") {
		t.Fatalf("warnings=%v", warnings)
	}
}

func TestNormalizeCurrentSourceExplanationProfile_InactiveFalse(t *testing.T) {
	profile, warnings := NormalizeCurrentSourceExplanationProfile("结合当前代码", &CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: false,
		SourceQuotes:                        []string{"结合当前代码"},
		Confidence:                          0.9,
	})
	if profile != nil || len(warnings) != 0 {
		t.Fatalf("inactive false profile should be ignored cleanly, got profile=%+v warnings=%v", profile, warnings)
	}
}

func currentSourceExplanationModeStrings(modes []CurrentSourceExplanationMode) []string {
	out := make([]string, 0, len(modes))
	for _, mode := range modes {
		out = append(out, string(mode))
	}
	return out
}
