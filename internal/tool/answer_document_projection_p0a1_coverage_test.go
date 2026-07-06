package tool

// answer_document_projection_p0a1_coverage_test.go — P0-A1 显示批 pins for
// §15.D gap③ (real_trace_campaign_20260705.md, q6/q8 witness): a published
// target-symptom denominator must NEVER be silently demoted to the whole
// window when the depth-1 attribution overshoots it. The q6 shape — a
// 112.223ms resolved lock span 0.048ms past the target's 112.175ms sleep —
// rendered "on-chain 已归因 112.223ms/94%,未归因残差 6.838ms" against the
// 119.061ms window: a fabricated residual for a fully explained wait.
// Two overshoot regimes, forked on runtimeTraceProjSymptomOvershootJitterMS:
//   - jitter (≤0.5ms): full symptom coverage + verbatim raw-caliber
//     disclosure (min-clamp precedent);
//   - gross: both magnitudes published, no percentage, no residual — and
//     still never a whole-window recast.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// p0a1CoverageModel is the q6 witness shape: one target self sleep row
// (symptom denominator) and one data-bearing depth-1 chain row whose
// cumulative drives the attributed numerator.
func p0a1CoverageModel(symptomMS, attributedMS float64) runtimeTraceProjTreeModel {
	return runtimeTraceProjTreeModel{
		Target:   ".ugc.aweme.lite-16547",
		WindowMS: 119.061,
		SelfRows: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: ".ugc.aweme.lite-16547", ImpactMS: symptomMS, StateKind: "s_sleep"}},
		},
		TreeRows: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "NetworkKit_AssetsUtil_Operate_0-42067", CumulativeImpactMS: attributedMS}},
		},
	}
}

var p0a1Projection = types.TraceCausalProjection{WindowStartTs: 33872.289161, WindowEndTs: 33872.408222}

func TestRuntimeTraceProjCoverageJitterOvershootKeepsSymptomDenominator(t *testing.T) {
	// q6 witness numbers verbatim: lock span 112.223 vs target sleep 112.175
	// (excess 0.048ms — boundary jitter).
	model := p0a1CoverageModel(112.175, 112.223)
	line := runtimeTraceProjWindowLine(p0a1Projection, model, true)
	if !strings.Contains(line, "目标等待(sleep/D-state/runnable) 112.175ms 中 on-chain 已归因 112.175ms(100%),未归因 0.000ms(0%)") {
		t.Fatalf("jitter overshoot must keep the symptom denominator at full coverage:\n%s", line)
	}
	if !strings.Contains(line, "归因口径合计 112.223ms,略超目标等待 0.048ms") {
		t.Fatalf("the raw caliber total and excess must be disclosed verbatim:\n%s", line)
	}
	// The fabricated whole-window recast is gone: no 94%, no 6.838ms residual,
	// no whole-window residual CLAIM (the disclosure's negated "不计未归因残差"
	// mention is fine — the claim form carries a trailing value).
	for _, banned := range []string{"94%", "6.838", "未归因残差 "} {
		if strings.Contains(line, banned) {
			t.Fatalf("whole-window demotion artifact %q must be gone:\n%s", banned, line)
		}
	}
	en := runtimeTraceProjWindowLine(p0a1Projection, model, false)
	if !strings.Contains(en, "Of the target's 112.175ms wait time (sleep/D-state/runnable), on-chain attributed 112.175ms (100%), unattributed 0.000ms (0%)") {
		t.Fatalf("EN surface must fork the same way:\n%s", en)
	}
	if !strings.Contains(en, "The attribution caliber totals 112.223ms, 0.048ms past the target wait") {
		t.Fatalf("EN surface must disclose the raw caliber total:\n%s", en)
	}
	if strings.Contains(en, "unattributed residual 6.838ms") || strings.Contains(en, "94%") {
		t.Fatalf("EN whole-window demotion artifacts must be gone:\n%s", en)
	}
}

func TestRuntimeTraceProjCoverageGrossOvershootPublishesBothMagnitudesNoPercent(t *testing.T) {
	// Beyond the jitter allowance the F1 nesting premise is broken: per-ms
	// containment is unverified, so no percentage may be claimed — but the
	// denominator still never recasts to the whole window.
	model := p0a1CoverageModel(20.0, 60.0)
	line := runtimeTraceProjWindowLine(p0a1Projection, model, true)
	if !strings.Contains(line, "目标等待(sleep/D-state/runnable) 20.000ms;on-chain 归因口径合计 60.000ms,超出目标等待 40.000ms") {
		t.Fatalf("gross overshoot must publish both magnitudes:\n%s", line)
	}
	if !strings.Contains(line, "不给出覆盖百分比") {
		t.Fatalf("gross overshoot must say no coverage percentage is given:\n%s", line)
	}
	for _, banned := range []string{"(100%)", "未归因残差 ", "%)。"} {
		if strings.Contains(line, banned) {
			t.Fatalf("gross overshoot must not fabricate %q:\n%s", banned, line)
		}
	}
	// attributed (60) <= window (119.061): no cross-window pointer.
	if strings.Contains(line, "跨出窗口") {
		t.Fatalf("in-window overshoot must not claim the state crosses the window:\n%s", line)
	}
	en := runtimeTraceProjWindowLine(p0a1Projection, model, false)
	if !strings.Contains(en, "Target wait (sleep/D-state/runnable) 20.000ms; the on-chain attribution caliber totals 60.000ms, 40.000ms beyond the target wait") {
		t.Fatalf("EN gross overshoot must fork the same way:\n%s", en)
	}
	if strings.Contains(en, "unattributed residual ") || strings.Contains(en, "(100%)") {
		t.Fatalf("EN gross overshoot must not fabricate coverage:\n%s", en)
	}
}

func TestRuntimeTraceProjCoverageGrossOvershootCrossWindowKeepsWarnPointer(t *testing.T) {
	// attributed > whole window: the ⚠ cross-window pointer (previously the
	// third branch's only wording) survives on the symptom-denominator side.
	model := p0a1CoverageModel(20.0, 300.0)
	line := runtimeTraceProjWindowLine(p0a1Projection, model, true)
	if !strings.Contains(line, "超出目标等待 280.000ms") {
		t.Fatalf("cross-window gross overshoot still publishes both magnitudes:\n%s", line)
	}
	if !strings.Contains(line, "其实际状态跨出窗口,见 ⚠ 标记") {
		t.Fatalf("cross-window overshoot keeps the ⚠ pointer:\n%s", line)
	}
	en := runtimeTraceProjWindowLine(p0a1Projection, model, false)
	if !strings.Contains(en, "The underlying state crosses the window; see ⚠ marks.") {
		t.Fatalf("EN cross-window overshoot keeps the ⚠ pointer:\n%s", en)
	}
}

func TestRuntimeTraceProjCoverageOvershootJitterBoundary(t *testing.T) {
	// Exactly at the allowance → jitter regime; just past it → gross regime.
	at := runtimeTraceProjWindowLine(p0a1Projection, p0a1CoverageModel(100.0, 100.0+runtimeTraceProjSymptomOvershootJitterMS), true)
	if !strings.Contains(at, "已归因 100.000ms(100%)") || !strings.Contains(at, "略超目标等待 0.500ms") {
		t.Fatalf("excess == allowance stays in the jitter regime:\n%s", at)
	}
	past := runtimeTraceProjWindowLine(p0a1Projection, p0a1CoverageModel(100.0, 100.6), true)
	if !strings.Contains(past, "不给出覆盖百分比") || strings.Contains(past, "(100%)") {
		t.Fatalf("excess > allowance moves to the gross regime:\n%s", past)
	}
}

func TestRuntimeTraceProjCoverageNoSymptomStillFallsBackToWholeWindow(t *testing.T) {
	// No self-state symptom row → the pre-V2 whole-window wording is untouched
	// (§15.D gap③ changed ONLY the overshoot path).
	model := p0a1CoverageModel(0, 60.0)
	model.SelfRows = nil
	line := runtimeTraceProjWindowLine(p0a1Projection, model, true)
	if !strings.Contains(line, "未归因残差") || strings.Contains(line, "目标等待") {
		t.Fatalf("symptom-less shape keeps the whole-window fallback wording:\n%s", line)
	}
}
