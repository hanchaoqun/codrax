package orchestrator

import (
	"strings"
	"testing"
)

// TestSoftMessages_LocalizeOnLanguage pins that the forced-read and
// convergence-stall strings render in the user's preferred language
// and never leak internal jargon ("CGEC", "E2", "I4", counter names,
// node IDs). Operator-facing details live in logging.Info lines, not
// in these Reasoning strings.
func TestSoftMessages_LocalizeOnLanguage(t *testing.T) {
	cases := []struct {
		name string
		lang string
		zh   bool
	}{
		{"zh", "zh", true},
		{"zh-CN mixed case", "zh-CN", true},
		{"cn alias", "cn", true},
		{"chinese alias", "chinese", true},
		{"en defaults to english", "en", false},
		{"empty defaults to english", "", false},
		{"unknown defaults to english", "fr", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := softForcedReadMessage(c.lang, 3)
			if strings.HasPrefix(got, "⟳") == false {
				t.Errorf("forced-read must start with the ⟳ soft symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "补充") {
				t.Errorf("zh: forced-read must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Filling") {
				t.Errorf("en: forced-read must be English, got %q", got)
			}

			got = softConvergenceStallMessage(c.lang)
			if strings.HasPrefix(got, "–") == false {
				t.Errorf("stall must start with the – soft symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "线索") {
				t.Errorf("zh: stall must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Finalizing") {
				t.Errorf("en: stall must be English, got %q", got)
			}
		})
	}
}

// TestSoftMessages_NoInternalJargon pins that the user-facing strings
// carry no internal identifiers that would leak scheduler mechanics
// into the UI. The message-owner contract says those names (CGEC,
// E2, I4, enforcer counter names, repair kind enums, node IDs) stay
// in logging.* only.
func TestSoftMessages_NoInternalJargon(t *testing.T) {
	forbidden := []string{"CGEC", "E2", "I4", "enforcer", "forced_reads", "chains_demoted", "repair"}
	messages := []string{
		softForcedReadMessage("en", 3),
		softForcedReadMessage("zh", 3),
		softConvergenceStallMessage("en"),
		softConvergenceStallMessage("zh"),
	}
	for _, m := range messages {
		lower := strings.ToLower(m)
		for _, f := range forbidden {
			if strings.Contains(lower, strings.ToLower(f)) {
				t.Errorf("message %q leaks internal token %q", m, f)
			}
		}
	}
}
