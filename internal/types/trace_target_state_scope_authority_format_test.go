package types

import (
	"strings"
	"testing"
)

// trace_target_state_scope_authority_format_test.go — V3-1 pins
// (colleague_merge_audit §40.20 ①): the ONE prompt-face formatter of the
// target-state account and the ONE uninterruptible fold it publishes.

func formatTargetStateFixture() TraceTargetStateScopeAuthority {
	return TraceTargetStateScopeAuthority{
		ArtifactLabel: "customer.systrace", Subject: "app-100",
		WindowStartTs: 1, WindowEndTs: 1.010, WindowMS: 10,
		RunnableMS: 1, SleepMS: 8, DStateMS: 0.5, IOWaitMS: 0.5, SleepIOWaitMS: 0,
		TotalMS: 10, CoverageStatus: "complete",
	}
}

func TestFormatTargetStateAccountFoldsUninterruptibleAsDPlusIO(t *testing.T) {
	a := formatTargetStateFixture()
	if got := a.UninterruptibleWaitMS(); got != 1.0 {
		t.Fatalf("UninterruptibleWaitMS must fold D+IO = 1.000, got %.3f", got)
	}
	if got := TraceUninterruptibleWaitMS(4.039, 1.340); got != 4.039+1.340 {
		t.Fatalf("fold function drifted: %.3f", got)
	}
	projection := TraceCausalProjectionTargetStateAccount{DStateMS: 0.5, IOWaitMS: 0.5}
	if got := projection.UninterruptibleWaitMS(); got != 1.0 {
		t.Fatalf("projection account fold must delegate to the same function, got %.3f", got)
	}

	zh := FormatTargetStateAccount(a, "zh-CN")
	wantZH := "工件 customer.systrace；目标线程 app-100；窗口 1.000000–1.010000 秒；运行 0.000 毫秒，可运行但尚未获调度 1.000 毫秒，可中断睡眠 8.000 毫秒，不可中断等待 1.000 毫秒（其中调度器标记的 IO 等待 0.500 毫秒）；合计 10.000 毫秒；覆盖完整；未归账 0.000 毫秒。"
	if zh != wantZH {
		t.Fatalf("zh sentence drifted:\n got %s\nwant %s", zh, wantZH)
	}
	en := FormatTargetStateAccount(a, "en")
	wantEN := "Artifact customer.systrace; target thread app-100; window 1.000000–1.010000 seconds; running 0.000 ms, runnable but not yet scheduled 1.000 ms, interruptible sleep 8.000 ms, uninterruptible wait 1.000 ms (including 0.500 ms of scheduler-marked IO wait); total 10.000 ms; complete coverage; 0.000 ms unaccounted."
	if en != wantEN {
		t.Fatalf("en sentence drifted:\n got %s\nwant %s", en, wantEN)
	}
	for _, sentence := range []string{zh, en} {
		if strings.Contains(sentence, "\n") || strings.HasPrefix(sentence, "- ") {
			t.Fatalf("formatter must return one bare sentence (no bullet, no newline): %q", sentence)
		}
		for _, forbidden := range []string{"io_wait", "d_state", "sleep_io_wait", "partition-"} {
			if strings.Contains(sentence, forbidden) {
				t.Fatalf("formatter leaked machine token %q: %s", forbidden, sentence)
			}
		}
	}
}

func TestFormatTargetStateAccountDisclosesSleepSideIOMarkerInsideSleep(t *testing.T) {
	a := formatTargetStateFixture()
	a.SleepIOWaitMS = 3
	zh := FormatTargetStateAccount(a, "zh")
	if !strings.Contains(zh, "可中断睡眠 8.000 毫秒（其中带 IO 等待标记的可中断睡眠 3.000 毫秒，已含在睡眠内），不可中断等待 1.000 毫秒") {
		t.Fatalf("sleep-side IO marker must render inside the sleep term, never as an addend:\n%s", zh)
	}
	en := FormatTargetStateAccount(a, "en")
	if !strings.Contains(en, "interruptible sleep 8.000 ms (including 3.000 ms of interruptible sleep carrying an IO-wait marker, already inside the sleep term), uninterruptible wait 1.000 ms") {
		t.Fatalf("en sleep-side clause drifted:\n%s", en)
	}
	if got := a.SchedulerMarkedWaitMS(); got != 4.0 {
		t.Fatalf("SchedulerMarkedWaitMS = fold + sleep-side marker = 4.000, got %.3f", got)
	}
}

func TestFormatTargetStateAccountOmitsArtifactClauseWhenLabelUnknown(t *testing.T) {
	a := formatTargetStateFixture()
	a.ArtifactLabel = ""
	a.CoverageStatus = "partial_unaccounted"
	a.UnaccountedMS = 2.5
	zh := FormatTargetStateAccount(a, "zh")
	if strings.Contains(zh, "工件") || !strings.HasPrefix(zh, "目标线程 app-100；") {
		t.Fatalf("no artifact label → no artifact clause (and no synthetic partition name):\n%s", zh)
	}
	if !strings.HasSuffix(zh, "；部分覆盖，仍有未计入时间；未归账 2.500 毫秒。") {
		t.Fatalf("coverage word + unaccounted remainder must close the sentence:\n%s", zh)
	}
	if !strings.HasSuffix(FormatTargetStateAccount(a, "en"), "; partial coverage with unaccounted time; 2.500 ms unaccounted.") {
		t.Fatalf("en coverage tail drifted:\n%s", FormatTargetStateAccount(a, "en"))
	}
}

func TestFormatTargetStateAccountCaliberStatesTheFold(t *testing.T) {
	zh := FormatTargetStateAccountCaliber("zh")
	for _, want := range []string{"不可中断等待 = 非 IO 的 D 状态 + 调度器标记的 IO 等待", "不再相加", "未评估而不是零"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh caliber sentence missing %q:\n%s", want, zh)
		}
	}
	en := FormatTargetStateAccountCaliber("en")
	for _, want := range []string{
		"narrow scheduler-marked definition",
		"uninterruptible wait = non-IO D state + scheduler-marked IO wait",
		"never added again",
		"Target blocking closed by IO issue-to-completion evidence is a separate ruler",
		"unassessed rather than zero",
	} {
		if !strings.Contains(en, want) {
			t.Fatalf("en caliber sentence missing %q:\n%s", want, en)
		}
	}
}
