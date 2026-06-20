package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
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
			if strings.HasPrefix(got, "›") == false {
				t.Errorf("forced-read must start with the › progress symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "补充") {
				t.Errorf("zh: forced-read must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Filling") {
				t.Errorf("en: forced-read must be English, got %q", got)
			}

			got = softConvergenceStallMessage(c.lang)
			if strings.HasPrefix(got, "·") == false {
				t.Errorf("stall must start with the · info-class symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "证据") {
				t.Errorf("zh: stall must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "evidence") {
				t.Errorf("en: stall must be English, got %q", got)
			}

			got = softRetryHintMessage(c.lang)
			if strings.HasPrefix(got, "⟳") == false {
				t.Errorf("retry-hint must start with the ⟳ soft symbol, got %q", got)
			}
			// P7 follow-up (2026-05-17): keep retry copy
			// user-facing, but avoid the vague "上下文信息"
			// phrase and avoid implying every retry is evidence
			// collection; some are answer-shape validation retries.
			if c.zh && !strings.Contains(got, "验证") {
				t.Errorf("zh: retry-hint must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Validation") {
				t.Errorf("en: retry-hint must be English, got %q", got)
			}

			got = softInvestigationReadyMessage(c.lang)
			if strings.HasPrefix(got, "›") == false {
				t.Errorf("investigation-ready must start with the › progress symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "调查") {
				t.Errorf("zh: investigation-ready must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Investigation") {
				t.Errorf("en: investigation-ready must be English, got %q", got)
			}

			got = softAdvisoryDebtSkippedMessage(c.lang)
			if strings.HasPrefix(got, "›") == false {
				t.Errorf("advisory-debt-skipped must start with the › progress symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "跳过") {
				t.Errorf("zh: advisory-debt-skipped must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Skipping") {
				t.Errorf("en: advisory-debt-skipped must be English, got %q", got)
			}

			got = softAnswerCheckRetryMessage(c.lang)
			if strings.HasPrefix(got, "⟳") == false {
				t.Errorf("answer-check must start with the ⟳ soft symbol, got %q", got)
			}
			// P7 wording polish: "再跑一轮" → "正在重新生成";
			// "Answer needs another pass" → "Refining the answer".
			if c.zh && !strings.Contains(got, "答案") {
				t.Errorf("zh: answer-check must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "answer") {
				t.Errorf("en: answer-check must be English, got %q", got)
			}

			got = softFinalizingMessage(c.lang)
			if strings.HasPrefix(got, "›") == false {
				t.Errorf("finalizing must start with the › progress symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "生成") {
				t.Errorf("zh: finalizing must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Composing") {
				t.Errorf("en: finalizing must be English, got %q", got)
			}
		})
	}
}

// TestSoftRetryHintMessage_DropsInternalPromptMarkup pins the
// session-22 customer-facing fix: the retry-hint event must not
// leak the LLM-directed RetryHint body's internal prompt markup —
// customers reported thinking-feed lines like "## Evidence Gaps",
// "[MISSING]", "(entities: MidLoopHint)" that they could not
// interpret. After the fix, the event carries only the localized
// generic cue; the full body lives in the debug log.
func TestSoftRetryHintMessage_DropsInternalPromptMarkup(t *testing.T) {
	forbidden := []string{
		"## Evidence Gaps",
		"[MISSING]",
		"(entities:",
		"not yet satisfied",
		"return_value",
		"Previous attempt",
	}
	for _, lang := range []string{"en", "zh"} {
		m := softRetryHintMessage(lang)
		for _, f := range forbidden {
			if strings.Contains(m, f) {
				t.Errorf("lang=%q: message %q leaks internal prompt markup %q", lang, m, f)
			}
		}
	}
}

func TestSoftTransportRetryHintForStage_SeparatesTransportFromSemanticRetry(t *testing.T) {
	zh := softTransportRetryHintForStage("zh", types.StageAnalyze)
	if !strings.Contains(zh, "连接/流式响应异常") ||
		!strings.Contains(zh, "理解问题") ||
		strings.Contains(zh, "验证还不够稳") ||
		strings.Contains(zh, "模型响应出错") {
		t.Fatalf("analyze transport retry must describe transport retry, got %q", zh)
	}
	en := softTransportRetryHintForStage("en", types.StageAnalyze)
	if !strings.Contains(en, "Connection/stream issue") ||
		!strings.Contains(en, "understanding the request") ||
		strings.Contains(en, "Validation needs") ||
		strings.Contains(en, "Model response error") {
		t.Fatalf("analyze transport retry must describe transport retry, got %q", en)
	}
	finalizeZH := softTransportRetryHintForStage("zh", types.StageFinalize)
	if !strings.Contains(finalizeZH, "撰写最终答案") || strings.Contains(finalizeZH, "重写") {
		t.Fatalf("finalize transport retry must not read like semantic rewrite, got %q", finalizeZH)
	}
}

// TestSoftMessages_NoInternalJargon pins that the user-facing strings
// carry no internal identifiers that would leak scheduler mechanics
// into the UI. The message-owner contract says those names (CGEC,
// E2, I4, enforcer counter names, repair kind enums, node IDs) stay
// in logging.* only.
func TestSoftMessages_NoInternalJargon(t *testing.T) {
	forbidden := []string{
		// CGEC enforcer naming (previous contract).
		"CGEC", "E2", "I4", "enforcer", "forced_reads", "chains_demoted", "repair",
		// Scheduler / criterion internals newly surfaced in session 22
		// when we folded four more events into the friendly surface.
		"success criteria", "SuccessCriteria", "envShape", "hypProgress",
		"validate node", "node_id", "criterion",
		// Raw RetryHint prompt markup (would leak if a site accidentally
		// reverts to string-concatenating RetryHint bodies).
		"## Evidence Gaps", "[MISSING]", "(entities:",
		// Answer-contract violation vocabulary rendered by
		// renderViolations — must not land in the user-facing event.
		"answer contract", "AnswerContract", "violation", "pendingViolation",
	}
	messages := []string{
		softForcedReadMessage("en", 3),
		softForcedReadMessage("zh", 3),
		softConvergenceStallMessage("en"),
		softConvergenceStallMessage("zh"),
		softRetryHintMessage("en"),
		softRetryHintMessage("zh"),
		softInvestigationReadyMessage("en"),
		softInvestigationReadyMessage("zh"),
		softAdvisoryDebtSkippedMessage("en"),
		softAdvisoryDebtSkippedMessage("zh"),
		softAnswerCheckRetryMessage("en"),
		softAnswerCheckRetryMessage("zh"),
		softFinalizingMessage("en"),
		softFinalizingMessage("zh"),
		// Audit followup (2026-05-02) — Block 1+2+3 dock surfaces.
		softFallbackTargetMessage("en", FallbackFinalizerOnly),
		softFallbackTargetMessage("zh", FallbackFinalizerOnly),
		softFallbackTargetMessage("en", FallbackBackToExtract),
		softFallbackTargetMessage("zh", FallbackBackToExtract),
		softFallbackTargetMessage("en", FallbackBackToExplore),
		softFallbackTargetMessage("zh", FallbackBackToExplore),
		softFallbackTargetMessage("en", FallbackFailLoud),
		softFallbackTargetMessage("zh", FallbackFailLoud),
		softUpstreamFallbackCapMessage("en", 2, 2),
		softUpstreamFallbackCapMessage("zh", 2, 2),
		softYieldKillMessage("en"),
		softYieldKillMessage("zh"),
		softPlanCriticReviewMessage("en", 3),
		softPlanCriticReviewMessage("zh", 0),
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

// TestSoftFallbackTargetMessage_DistinctPerTarget pins the audit
// followup contract (2026-05-02): each fallback target produces a
// DIFFERENT user-visible line so the dock conveys which layer is
// being re-run. Pre-this-fix every target rendered the same generic
// "answer needs another pass", hiding scope.
func TestSoftFallbackTargetMessage_DistinctPerTarget(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		seen := map[string]FallbackTarget{}
		for _, tgt := range []FallbackTarget{
			FallbackFailLoud, FallbackFinalizerOnly,
			FallbackBackToExtract, FallbackBackToExplore,
		} {
			msg := softFallbackTargetMessage(lang, tgt)
			if msg == "" {
				t.Errorf("lang=%s target=%s produced empty message", lang, tgt)
				continue
			}
			if prev, dup := seen[msg]; dup {
				t.Errorf("lang=%s targets %s and %s share message %q (must be distinct)", lang, prev, tgt, msg)
			}
			seen[msg] = tgt
		}
	}
}

func TestSoftFallbackTargetMessage_TaxonomyIsSpecific(t *testing.T) {
	targets := []FallbackTarget{
		FallbackFailLoud, FallbackFinalizerOnly,
		FallbackBackToExtract, FallbackBackToExplore,
	}
	for _, lang := range []string{"en", "zh"} {
		for _, tgt := range targets {
			msg := softFallbackTargetMessage(lang, tgt)
			if strings.Contains(msg, string(tgt)) {
				t.Errorf("lang=%s target=%s leaked enum in %q", lang, tgt, msg)
			}
			for _, banned := range []string{
				"transport", "Transport", "stream", "流式响应", "连接",
				"答案待完善", "再跑一轮", "模型响应出错",
			} {
				if strings.Contains(msg, banned) {
					t.Errorf("lang=%s target=%s message %q uses over-broad/transport wording %q", lang, tgt, msg, banned)
				}
			}
		}
	}

	zhFinal := softFallbackTargetMessage("zh", FallbackFinalizerOnly)
	zhExtract := softFallbackTargetMessage("zh", FallbackBackToExtract)
	zhExplore := softFallbackTargetMessage("zh", FallbackBackToExplore)
	zhFail := softFallbackTargetMessage("zh", FallbackFailLoud)
	if !strings.Contains(zhFinal, "最终答案") || !strings.Contains(zhFinal, "重写") {
		t.Fatalf("finalizer-only fallback should name final-answer rewrite, got %q", zhFinal)
	}
	if !strings.Contains(zhExtract, "答案结构") {
		t.Fatalf("extract fallback should name answer-structure rebuild, got %q", zhExtract)
	}
	if !strings.Contains(zhExplore, "证据阶段") || !strings.Contains(zhExplore, "上下文") {
		t.Fatalf("explore fallback should name evidence-context collection, got %q", zhExplore)
	}
	if !strings.Contains(zhFail, "保留") || !strings.Contains(zhFail, "边界") {
		t.Fatalf("fail-loud fallback should name accept-with-boundary behavior, got %q", zhFail)
	}
}

// TestSoftPlanCriticReviewMessage_HonoursCount pins the per-count
// branch: 0 risks renders cleanly without a count, ≥1 renders the
// count plus a /plan show pointer so the user knows where to read
// the full critique.
func TestSoftPlanCriticReviewMessage_HonoursCount(t *testing.T) {
	zero := softPlanCriticReviewMessage("en", 0)
	three := softPlanCriticReviewMessage("en", 3)
	if zero == three {
		t.Errorf("zero-risk and 3-risk messages must differ")
	}
	if !strings.Contains(three, "3 risk") {
		t.Errorf("3-risk message must surface the count: %q", three)
	}
	if !strings.Contains(three, "/plan show") {
		t.Errorf("≥1 risk message must reference /plan show as the read-more pointer: %q", three)
	}
	if strings.Contains(zero, "/plan show") {
		t.Errorf("0-risk message must NOT reference /plan show (nothing to read): %q", zero)
	}
}

// TestSoftMessages_NoVisualShockSymbols pins the operator-feedback
// red line (2026-05-02): triangles / heavy alert glyphs are too
// strong for the dock cadence — soft '·' / '⟳' / '–' / '›' only.
func TestSoftMessages_NoVisualShockSymbols(t *testing.T) {
	visualShock := []string{"⚠", "❗", "‼", "⛔", "🚨", "🔴", "✘", "✗", "❌"}
	messages := []string{
		softFallbackTargetMessage("en", FallbackFailLoud),
		softFallbackTargetMessage("zh", FallbackFailLoud),
		softFallbackTargetMessage("en", FallbackFinalizerOnly),
		softFallbackTargetMessage("en", FallbackBackToExtract),
		softFallbackTargetMessage("en", FallbackBackToExplore),
		softUpstreamFallbackCapMessage("en", 2, 2),
		softUpstreamFallbackCapMessage("zh", 2, 2),
		softYieldKillMessage("en"),
		softYieldKillMessage("zh"),
		softPlanCriticReviewMessage("en", 3),
		softPlanCriticReviewMessage("en", 0),
	}
	for _, m := range messages {
		for _, glyph := range visualShock {
			if strings.Contains(m, glyph) {
				t.Errorf("message %q contains visual-shock glyph %q (use soft '·' / '⟳' / '–' / '›' only)", m, glyph)
			}
		}
	}
}

// =====================================================================
// Phase 4 / 6 (2026-05-08) multi-repo scan progress messages
// =====================================================================

// TestMultiRepoScanMessages_NoticeStyle pins the scan-progress
// notice strings to the canonical glyph semantics (operator
// feedback, 2026-05-08 audit):
//
//	⟳ <prose>   RETRY only          (planner regenerate, apply re-run, …)
//	· <prose>   in-progress         (canonical glyphPending)
//	✓ <prose>   milestone complete  (canonical glyphSuccess)
//	✗ <prose>   failure             (canonical glyphFatal)
//
// First-time scan is in-progress (NOT a retry) → · for start, ✓
// for ok end, ✗ for fail. ⟳ MUST NOT appear here — that would
// false-signal "we are retrying the scan" to the operator.
//
// Localisation parity check across zh / en.
func TestMultiRepoScanMessages_NoticeStyle(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		start := multiRepoScanStartMessage(lang, "repo-x")
		endOK := multiRepoScanEndMessage(lang, "repo-x", 1234, true)
		endFail := multiRepoScanEndMessage(lang, "repo-x", 1234, false)
		// Glyph contract.
		if !strings.HasPrefix(start, "· ") {
			t.Errorf("%s: scan-start %q must start with \"· \" (in-progress glyph, NOT ⟳ which is retry-only)", lang, start)
		}
		if !strings.HasPrefix(endOK, "✓ ") {
			t.Errorf("%s: scan-end-ok %q must start with \"✓ \" (canonical success rune)", lang, endOK)
		}
		if !strings.HasPrefix(endFail, "✗ ") {
			t.Errorf("%s: scan-end-fail %q must start with \"✗ \" (canonical fatal rune)", lang, endFail)
		}
		// Forbid the retry glyph anywhere in the scan messages —
		// scans are first-time data loads, not retries.
		for _, m := range []string{start, endOK, endFail} {
			if strings.Contains(m, "⟳") {
				t.Errorf("%s: %q leaks the retry glyph ⟳; first-time scans must not signal retry to the operator", lang, m)
			}
		}
		// Sub-repo path wrapped in backticks (consistent with
		// abandonForcedReadMessage and other path-bearing notices).
		for _, m := range []string{start, endOK, endFail} {
			if !strings.Contains(m, "`repo-x`") {
				t.Errorf("%s: %q must wrap the sub-repo name in backticks", lang, m)
			}
		}
		// Forbid the banner-style "multi-repo:" topic prefix — the
		// soft-notice family does not use a topic prefix; the
		// glyph alone classifies the row.
		for _, m := range []string{start, endOK, endFail} {
			if strings.Contains(m, "multi-repo:") {
				t.Errorf("%s: %q leaks the banner-style \"multi-repo:\" prefix; soft-notice family does not use a topic prefix", lang, m)
			}
		}
		// Localisation parity — zh / en messages MUST differ once
		// the leading glyph is removed (otherwise the lang switch
		// is a no-op and a zh user reads English).
		strip := func(s string) string {
			runes := []rune(s)
			if len(runes) <= 2 {
				return s
			}
			return strings.TrimSpace(string(runes[1:]))
		}
		if lang == "zh" {
			zhStart := strip(start)
			enStart := strip(multiRepoScanStartMessage("en", "repo-x"))
			if zhStart == enStart {
				t.Errorf("zh start localisation is identical to en (no-op switch): %q vs %q", zhStart, enStart)
			}
		}
	}
}

func TestMultiRepoScanMessages_ElapsedFormat(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{0, "<1ms"},
		{500, "500ms"},
		{1500, "1.5s"},
		{12345, "12.3s"},
	}
	for _, c := range cases {
		if got := formatScanElapsed(c.ms); got != c.want {
			t.Errorf("formatScanElapsed(%d) = %q, want %q", c.ms, got, c.want)
		}
	}
}

func TestRepoMapScanMessagesCarryFileCounts(t *testing.T) {
	ev := types.RepoMapScanEvent{
		RepoRoot:       "/work/linux",
		Mode:           types.RepoMapScanFull,
		TotalFiles:     93459,
		ParseableFiles: 12000,
		ParsedFiles:    4000,
	}
	start := repoMapScanMessage("zh", ev)
	if !strings.HasPrefix(start, "· ") || !strings.Contains(start, "`linux`") || !strings.Contains(start, "93459 个文件") || !strings.Contains(start, "12000 个源文件") {
		t.Fatalf("scan start should carry repo label and counts, got %q", start)
	}
	ev.Progress = true
	ev.Phase = types.RepoMapScanPhaseParse
	progress := repoMapScanMessage("zh", ev)
	if !strings.Contains(progress, "已解析 4000/12000") || strings.Contains(progress, "⟳") {
		t.Fatalf("scan progress should carry parsed/total without retry glyph, got %q", progress)
	}
	ev.Phase = types.RepoMapScanPhaseCacheWrite
	cacheWrite := repoMapScanMessage("zh", ev)
	if !strings.Contains(cacheWrite, "正在写入索引缓存") || !strings.Contains(cacheWrite, "源文件已解析 12000/12000") {
		t.Fatalf("cache-write progress should switch wording after parsing, got %q", cacheWrite)
	}
	ev.CurrentFile = "relations.md"
	ev.BytesWritten = 12 * 1024 * 1024
	ev.BytesTotal = 120 * 1024 * 1024
	cacheWriteBytes := repoMapScanMessage("zh", ev)
	if !strings.Contains(cacheWriteBytes, "relations.md") ||
		!strings.Contains(cacheWriteBytes, "12.0 MB/120.0 MB") ||
		!strings.Contains(cacheWriteBytes, "10%") {
		t.Fatalf("cache-write byte progress should show active sidecar and bytes, got %q", cacheWriteBytes)
	}
	ev.Phase = types.RepoMapScanPhaseParse
	ev.CurrentFile = "drivers/gpu/big_header.h"
	ev.BytesWritten = 0
	ev.BytesTotal = 0
	activeFile := repoMapScanMessage("zh", ev)
	if !strings.Contains(activeFile, "正在解析 drivers/gpu/big_header.h") ||
		!strings.Contains(activeFile, "已完成 4000/12000") {
		t.Fatalf("parse active-file progress should explain long tail work, got %q", activeFile)
	}
	ev.Progress = false
	ev.Phase = ""
	ev.CurrentFile = ""
	ev.Finished = true
	ev.OK = true
	ev.ElapsedMs = 1234
	done := repoMapScanMessage("en", ev)
	if !strings.HasPrefix(done, "✓ ") || !strings.Contains(done, "repo index `linux` ready") || !strings.Contains(done, "1.2s") {
		t.Fatalf("scan end should carry success glyph, repo label, elapsed time; got %q", done)
	}
}

func TestRepoMapScanMessagesShowInventoryAndChangePhases(t *testing.T) {
	inventory := types.RepoMapScanEvent{
		RepoRoot: "/work/linux",
		Phase:    types.RepoMapScanPhaseFileScan,
		Started:  true,
	}
	got := repoMapScanMessage("zh", inventory)
	if !strings.Contains(got, "正在统计仓库索引 `linux` 文件") || strings.Contains(got, "0 个文件") {
		t.Fatalf("inventory phase should be immediate and not claim zero files, got %q", got)
	}
	inventory.Started = false
	inventory.Progress = true
	inventory.TotalFiles = 1200
	inventory.ParseableFiles = 700
	inventory.ParsedFiles = 1200
	inventory.CurrentFile = "kernel/bpf/syscall.c"
	got = repoMapScanMessage("zh", inventory)
	if !strings.Contains(got, "正在统计文件") ||
		!strings.Contains(got, "正在检查 kernel/bpf/syscall.c") ||
		!strings.Contains(got, "已统计 1200 个文件，700 个源文件") {
		t.Fatalf("inventory progress should report counted files and current file, got %q", got)
	}

	change := types.RepoMapScanEvent{
		RepoRoot:    "/work/linux",
		Phase:       types.RepoMapScanPhaseChangeScan,
		Progress:    true,
		TotalFiles:  93459,
		ParsedFiles: 4000,
	}
	got = repoMapScanMessage("zh", change)
	if !strings.Contains(got, "正在校验缓存差异") || !strings.Contains(got, "已校验 4000/93459 个文件") {
		t.Fatalf("change-check phase should report checked file counts, got %q", got)
	}
	change.Progress = false
	change.Started = true
	change.ParsedFiles = 0
	got = repoMapScanMessage("zh", change)
	if !strings.Contains(got, "正在校验缓存差异") || !strings.Contains(got, "准备校验 93459 个文件") ||
		strings.Contains(got, "已校验 0/") {
		t.Fatalf("change-check start should not look like stale zero progress, got %q", got)
	}
	got = repoMapScanMessage("en", change)
	if !strings.Contains(got, "checking cache differences") || !strings.Contains(got, "preparing to check 93459 files") ||
		strings.Contains(got, "checked 0/") {
		t.Fatalf("change-check start should not look like stale zero progress in English, got %q", got)
	}

	cacheLoad := types.RepoMapScanEvent{
		RepoRoot:    "/work/linux",
		Mode:        types.RepoMapScanCacheHit,
		Phase:       types.RepoMapScanPhaseCacheLoad,
		Started:     true,
		TotalFiles:  93468,
		ParsedFiles: 0,
	}
	got = repoMapScanMessage("zh", cacheLoad)
	if !strings.Contains(got, "正在读取仓库索引 `linux` 缓存") ||
		!strings.Contains(got, "准备读取 93468 条索引记录") ||
		strings.Contains(got, "已读取 0/") {
		t.Fatalf("cache-load start should show preparation, not a misleading 0/total count, got %q", got)
	}
	cacheLoad.Started = false
	cacheLoad.Progress = true
	cacheLoad.CacheRecordsLoaded = 4096
	cacheLoad.CacheRecordsTotal = 93468
	cacheLoad.CacheChunksLoaded = 4
	cacheLoad.CacheChunksTotal = 92
	got = repoMapScanMessage("zh", cacheLoad)
	if !strings.Contains(got, "正在读取缓存") ||
		!strings.Contains(got, "已读取 4096/93468 条索引记录") ||
		!strings.Contains(got, "缓存分块 4/92") {
		t.Fatalf("cache-load progress should report record and chunk counts, got %q", got)
	}
	cacheLoad.Phase = types.RepoMapScanPhaseBuildGraph
	cacheLoad.ParseableFiles = 63891
	got = repoMapScanMessage("zh", cacheLoad)
	if !strings.Contains(got, "正在构建符号关系图") ||
		!strings.Contains(got, "缓存已读取，已加载 63891 个源文件") ||
		strings.Contains(got, "缓存命中，93468 个文件") {
		t.Fatalf("cache-hit build phase should show loaded source count, got %q", got)
	}

	viewRender := types.RepoMapScanEvent{
		RepoRoot:       "/work/linux",
		Phase:          types.RepoMapScanPhaseViewRender,
		Progress:       true,
		OK:             true,
		TotalFiles:     93468,
		ParseableFiles: 63891,
		ViewStep:       "bridges",
		ViewStepsDone:  3,
		ViewStepsTotal: 3,
	}
	got = repoMapScanMessage("zh", viewRender)
	if !strings.Contains(got, "正在生成 repo_map 视图") ||
		!strings.Contains(got, "正在生成 bridges") ||
		!strings.Contains(got, "步骤 3/3") {
		t.Fatalf("view render progress should show repo_map view work, got %q", got)
	}
	viewRender.Progress = false
	viewRender.Finished = true
	viewRender.ElapsedMs = 620
	got = repoMapScanMessage("zh", viewRender)
	if !strings.Contains(got, "repo_map 视图 `linux` 已生成") {
		t.Fatalf("view render finish should not say scan ready, got %q", got)
	}
}

func TestRepoMapScanMessagesShowWaitPhase(t *testing.T) {
	ev := types.RepoMapScanEvent{
		RepoRoot:       "/work/mono/services/api",
		DisplayName:    "services/api",
		SubRepoRootRel: "services/api",
		Phase:          types.RepoMapScanPhaseWait,
		Started:        true,
		TotalFiles:     259,
	}
	got := repoMapScanMessage("zh", ev)
	if !strings.Contains(got, "子仓索引 `services/api`") || !strings.Contains(got, "正在等待已有索引构建") {
		t.Fatalf("wait phase should explain in-flight repo_map build, got %q", got)
	}
	got = repoMapScanMessage("en", ev)
	if !strings.Contains(got, "sub-repo index `services/api`") || !strings.Contains(got, "waiting for the in-progress index build") {
		t.Fatalf("wait phase should be localized in English too, got %q", got)
	}
}

func TestRepoMapScanMessagesUseSubRepoLabelWhenPresent(t *testing.T) {
	ev := types.RepoMapScanEvent{
		RepoRoot:       "/work/mono/services/api",
		DisplayName:    "services/api",
		SubRepoRootRel: "services/api",
		Mode:           types.RepoMapScanCacheHit,
		Finished:       true,
		OK:             true,
		TotalFiles:     42,
	}
	got := repoMapScanMessage("zh", ev)
	if !strings.Contains(got, "子仓索引 `services/api`") || !strings.Contains(got, "缓存命中") {
		t.Fatalf("multi-repo scan should use RootRel label and cache-hit wording, got %q", got)
	}
}

func TestRepoMapScanCounts_BuildGraphRelationProgress(t *testing.T) {
	ev := types.RepoMapScanEvent{
		Phase: types.RepoMapScanPhaseBuildGraph, Mode: types.RepoMapScanFull,
		TotalFiles: 1417, ParseableFiles: 430, ViewStepsDone: 600, ViewStepsTotal: 1417,
	}
	zh := repoMapScanCountsZH(ev, true)
	if !strings.Contains(zh, "600/1417") || !strings.Contains(zh, "构建符号图") {
		t.Fatalf("zh build_graph relation progress missing: %q", zh)
	}
	en := repoMapScanCountsEN(ev, true)
	if !strings.Contains(en, "600/1417") || !strings.Contains(en, "symbol graph") {
		t.Fatalf("en build_graph relation progress missing: %q", en)
	}
	// Without relation totals it falls back to the parsed-count line.
	evNoSteps := ev
	evNoSteps.ViewStepsTotal = 0
	if got := repoMapScanCountsZH(evNoSteps, true); strings.Contains(got, "构建符号图") {
		t.Fatalf("no relation total must fall back, got: %q", got)
	}
}
