package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAppendSystemCaveats_DegradedTerminationVisibleBothLangs(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		mu := types.NewMutableState("degraded")
		mu.MarkTerminationFloorDegraded("ratio 10% < 50%")
		o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: lang}}
		got := o.appendSystemCaveatsToAnswer("answer body")
		if !strings.Contains(got, "answer body") {
			t.Fatalf("%s: answer body must be preserved", lang)
		}
		if lang == "zh" && !strings.Contains(got, "低于配置的最低标准") {
			t.Fatalf("zh: degraded caveat missing, got %q", got)
		}
		if lang == "en" && !strings.Contains(got, "below the configured floor") {
			t.Fatalf("en: degraded caveat missing, got %q", got)
		}
		// Internal diagnostic detail must NOT leak into the user answer.
		if strings.Contains(got, "ratio 10%") {
			t.Fatalf("%s: internal detail leaked, got %q", lang, got)
		}
	}
}

func TestAppendSystemCaveats_NoDegradationNoCaveat(t *testing.T) {
	mu := types.NewMutableState("clean")
	mu.SetTerminationProfile(types.TerminationProfile{Kind: types.TerminationStopCondition})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "en"}}
	got := o.appendSystemCaveatsToAnswer("answer body")
	if got != "answer body" {
		t.Fatalf("non-degraded termination must not add caveats, got %q", got)
	}
}
