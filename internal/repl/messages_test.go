package repl

import (
	"strings"
	"testing"
)

// Locks the zh-as-default contract for every helper in messages.go:
// only an explicit "en" flips to English; everything else (empty,
// "zh", "fr", typos) stays zh.

func TestIsZh_DefaultsToZh(t *testing.T) {
	cases := []struct {
		lang string
		zh   bool
	}{
		{"", true},
		{"zh", true},
		{"ZH", true},
		{"fr", true},
		{"ja", true},
		{"off", true},
		{" zh ", true},
		{"en", false},
		{"EN", false},
		{"  en", false},
		{"english", true}, // strict — only "en" flips
	}
	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			if got := isZh(c.lang); got != c.zh {
				t.Errorf("isZh(%q) = %v; want %v", c.lang, got, c.zh)
			}
		})
	}
}

func TestApproveTitlePrompt_BothLangs(t *testing.T) {
	zh := approveTitlePrompt("zh", "plan-1", 3)
	if !strings.Contains(zh, "批准") || !strings.Contains(zh, "plan-1") || !strings.Contains(zh, "3 处") {
		t.Errorf("zh prompt missing key fragments; got %q", zh)
	}
	en := approveTitlePrompt("en", "plan-1", 3)
	if !strings.Contains(en, "Approve") || !strings.Contains(en, "plan-1") {
		t.Errorf("en prompt missing key fragments; got %q", en)
	}
	if zh == en {
		t.Errorf("zh and en prompts should differ; both = %q", zh)
	}
}

func TestApproveCancelled_BothLangs(t *testing.T) {
	if approveCancelled("zh") == approveCancelled("en") {
		t.Error("zh and en should differ")
	}
	if !strings.Contains(approveCancelled("zh"), "取消") {
		t.Errorf("zh missing 取消; got %q", approveCancelled("zh"))
	}
}

func TestApproveFailedNudge_BothLangs(t *testing.T) {
	zh := approveFailedNudge("zh")
	en := approveFailedNudge("en")
	if len(zh) != len(en) {
		t.Errorf("zh and en should have same line count; got %d vs %d", len(zh), len(en))
	}
	for _, line := range zh {
		if line == "" {
			t.Error("zh has an empty line")
		}
	}
	// Both languages must mention /mode plan as the recovery path.
	zhAll := strings.Join(zh, "\n")
	enAll := strings.Join(en, "\n")
	if !strings.Contains(zhAll, "/mode plan") {
		t.Errorf("zh nudge missing /mode plan recovery hint; got %q", zhAll)
	}
	if !strings.Contains(enAll, "/mode plan") {
		t.Errorf("en nudge missing /mode plan recovery hint; got %q", enAll)
	}
}

func TestRejectConfirmed_BothLangs(t *testing.T) {
	zhWith := rejectConfirmedWithReason("zh", "plan-X", "broken patch")
	enWith := rejectConfirmedWithReason("en", "plan-X", "broken patch")
	if !strings.Contains(zhWith, "已拒绝") || !strings.Contains(zhWith, "plan-X") || !strings.Contains(zhWith, "broken patch") {
		t.Errorf("zh-with-reason malformed; got %q", zhWith)
	}
	if !strings.Contains(enWith, "rejected") || !strings.Contains(enWith, "plan-X") {
		t.Errorf("en-with-reason malformed; got %q", enWith)
	}
	zhNo := rejectConfirmedNoReason("zh", "plan-Y")
	enNo := rejectConfirmedNoReason("en", "plan-Y")
	if zhNo == zhWith {
		t.Error("with-reason and no-reason zh should differ")
	}
	if enNo == enWith {
		t.Error("with-reason and no-reason en should differ")
	}
}

func TestNoPendingPlan_BothLangs(t *testing.T) {
	zh := noPendingPlan("zh")
	en := noPendingPlan("en")
	if zh == en {
		t.Error("zh and en should differ")
	}
	if !strings.Contains(zh, "/mode plan") || !strings.Contains(en, "/mode plan") {
		t.Errorf("both should reference /mode plan recovery; zh=%q en=%q", zh, en)
	}
}

func TestModeSwitched_BothLangs(t *testing.T) {
	if !strings.Contains(modeSwitched("zh", "plan"), "已切换") {
		t.Error("zh missing 已切换")
	}
	if !strings.Contains(modeSwitched("en", "plan"), "switched") {
		t.Error("en missing 'switched'")
	}
}

func TestPromptStickyTag_StateCombinations(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		hasLog      bool
		hasTrace    bool
		hasPlan     bool
		memPressure bool
		want        string
	}{
		{"empty", "", false, false, false, false, ""},
		{"read mode no attachments", "read", false, false, false, false, ""},
		{"plan mode only", "plan", false, false, false, false, "[mode:plan]"},
		{"log only", "read", true, false, false, false, "[log]"},
		{"trace only", "read", false, true, false, false, "[trace]"},
		{"pending plan only", "read", false, false, true, false, "[plan]"},
		{"memory pressure only", "read", false, false, false, true, "[mem!]"},
		{"plan+log", "plan", true, false, false, false, "[mode:plan][log]"},
		{"all on", "apply", true, true, true, true, "[mode:apply][log][trace][plan][mem!]"},
		{"case-insensitive read", "READ", false, false, false, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := promptStickyTag(c.mode, c.hasLog, c.hasTrace, c.hasPlan, c.memPressure)
			if got != c.want {
				t.Errorf("promptStickyTag(%q,%v,%v,%v,%v) = %q; want %q",
					c.mode, c.hasLog, c.hasTrace, c.hasPlan, c.memPressure, got, c.want)
			}
		})
	}
}

func TestMemoryPressureHint_BothLangs(t *testing.T) {
	zh := memoryPressureHint("zh", 30, 60)
	en := memoryPressureHint("en", 30, 60)
	if !strings.Contains(zh, "30") || !strings.Contains(zh, "60") {
		t.Errorf("zh hint should embed concrete counts; got %q", zh)
	}
	if !strings.Contains(zh, "/compact") || !strings.Contains(zh, "/clear") {
		t.Errorf("zh hint must surface both recovery commands; got %q", zh)
	}
	if !strings.Contains(en, "/compact") || !strings.Contains(en, "/clear") {
		t.Errorf("en hint must surface both recovery commands; got %q", en)
	}
}

func TestVerifyDispatching_BothLangs(t *testing.T) {
	zh := verifyDispatching("zh", "plan-V")
	en := verifyDispatching("en", "plan-V")
	if !strings.Contains(zh, "verify") || !strings.Contains(zh, "plan-V") {
		t.Errorf("zh malformed; got %q", zh)
	}
	if !strings.Contains(en, "verify") || !strings.Contains(en, "plan-V") {
		t.Errorf("en malformed; got %q", en)
	}
}
