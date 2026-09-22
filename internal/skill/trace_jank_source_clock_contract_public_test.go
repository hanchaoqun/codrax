package skill_test

import (
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestJankSourceClockPublicSystemTeaching(t *testing.T) {
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	cfg, err := registry.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"zh", "en"} {
		for _, hasTrace := range []bool{false, true} {
			ac := &types.AgentContext{Stage: types.StageExplore, Language: language, Objective: "Inspect the selected runtime fields."}
			if hasTrace {
				ac.RuntimeArtifactPreflight = types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace", Carrier: "request_path"}}})
			}
			var system strings.Builder
			for _, message := range promptcontext.ToMessages(promptcontext.BuildPromptContext(ac, cfg)) {
				if message.Role == "system" {
					system.WriteString(message.Content)
				}
			}
			body := system.String()
			if got := strings.Count(body, skill.TraceJankQueryContract); got != 0 && !hasTrace || got != 1 && hasTrace {
				t.Fatalf("%s trace=%v field contract count=%d", language, hasTrace, got)
			}
			if hasTrace {
				for _, want := range []string{"same source Trace time axis", "(end_ts - start_ts) / 1,000,000", "not timezone, UTC or calendar alignment", "different captures", "Legacy unverified receipts", "Only proven chain evidence supplies root causes"} {
					if !strings.Contains(body, want) {
						t.Errorf("%s missing %q", language, want)
					}
				}
				if strings.Contains(body, "Never assume those clocks agree") {
					t.Errorf("%s contrary default remained", language)
				}
			}
		}
	}
}
