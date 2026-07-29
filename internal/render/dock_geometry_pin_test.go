package render

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// PIB-6 (ledger docs/design/pi_borrow_analysis_20260729.md §7.7,
// BARGRID-1 generalized to the terminal dock): emission-surface
// geometry must be locale-independent and every composed dock row must
// fit the column cap under the pinned width condition — with
// adversarial CJK / EAW-Ambiguous content, not just ASCII.

func adversarialDockStates() []dockRowState {
	return []dockRowState{
		{
			activity:   activityState{kind: activityReceiving},
			streamTail: strings.Repeat("汉字宽内容·↑↓…█▒", 40),
			frame:      "⠋",
			lang:       "zh",
			stageLabel: "正在理解问题（超长中文阶段标签与歧义宽度字符…·↑↓）",
			stageKey:   "explore",
			modelID:    "很长的模型标识符-MiniMax-M2.7-with-ambiguous-…·suffix",
			iteration:  12, toolCount: 9,
			contextTokensEstimate: 91000, contextWindowTokens: 100000,
			usageInputTotal: 1234567, usageOutputTotal: 89012,
			totalElapsed: "45m12s", cancelHint: "Ctrl+C 取消（连按 2 次强制退出）",
		},
		{
			activity: activityState{
				kind: activityRetrying, retryAttempt: 3, retryMaxAttempts: 6,
				retryRemainingSec: 128, retryCountdownActive: true,
			},
			frame: "⠙", lang: "zh",
			stageLabel: strings.Repeat("█▒", 100),
		},
	}
}

// TestDockRows_FitColumnCapWithAdversarialContent pins the geometry
// invariant: every composed row measures within the cap under the
// pinned condition.
func TestDockRows_FitColumnCapWithAdversarialContent(t *testing.T) {
	maxCols := previewLineMaxCols()
	for i, state := range adversarialDockStates() {
		rows := composeDockRows(state)
		for j, row := range rows {
			if w := dockWidthCond.StringWidth(stripAnsiEscapes(row)); w > maxCols {
				t.Errorf("state %d row %d overflows: width=%d cap=%d content=%q", i, j, w, maxCols, stripAnsiEscapes(row))
			}
		}
	}
}

// TestTruncByDisplayWidth_LocaleIndependent pins the BARGRID-1 lesson
// mechanically: flipping the process-global runewidth default
// condition (what a zh_CN locale does at startup) must NOT change the
// dock's truncation output — the emission surface uses the pinned
// condition, never the package default.
func TestTruncByDisplayWidth_LocaleIndependent(t *testing.T) {
	adversarial := "…·↑↓⠋█▒汉字 mixed ambiguous content …·↑↓⠋█▒ and more"
	baseline := truncByDisplayWidth(adversarial, 24)

	saved := runewidth.DefaultCondition.EastAsianWidth
	runewidth.DefaultCondition.EastAsianWidth = !saved
	defer func() { runewidth.DefaultCondition.EastAsianWidth = saved }()

	if flipped := truncByDisplayWidth(adversarial, 24); flipped != baseline {
		t.Fatalf("dock truncation depends on the process-global runewidth condition:\nbaseline=%q\nflipped =%q", baseline, flipped)
	}
}
