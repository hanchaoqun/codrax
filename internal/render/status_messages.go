package render

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// status_messages.go is the renderer's localization layer. Every
// user-facing status string in the spinner area passes through one
// of the friendly* helpers below so the surface text follows the
// configured lang code without any taskRow consumer needing to know
// zh from en.
//
// Design rules:
//   - Render package owns the strings. We intentionally do NOT
//     import internal/repl/messages.go to avoid a render → repl back
//     edge; the project-wide isZh logic is mirrored locally.
//   - All structural decisions (which stage label, which kind of
//     thinking summary, recoverable vs fatal classification) live
//     elsewhere; this file is a vocabulary.
//   - Light text-shaping (CJK ↔ ASCII spacing) lives in
//     normalizeTopicText so topic strings emitted by the analyzer
//     read naturally in either locale.

// isZh reports whether the given lang code maps to Chinese output.
// Empty defaults to zh (mirrors the project-wide isZh fallback used
// in cmd/root.go and internal/orchestrator). Any prefix matching
// "zh" is treated as Chinese; everything else falls through to en.
func isZh(lang string) bool {
	if lang == "" {
		return true
	}
	l := strings.ToLower(strings.TrimSpace(lang))
	return strings.HasPrefix(l, "zh")
}

// stagePhraseState is the lifecycle slot stagePhrase resolves
// against. Three values: running ("正在 ..."), done ("已 ..."),
// and pending ("待 ..." — queued, has NOT started yet). The third
// value is load-bearing because two rows reading "正在 X" and
// "正在 Y" side by side mislead the user into thinking both are
// executing concurrently — but the TaskGraph wires upstream stages
// as hard dependencies, so a row's pending state needs an
// unambiguous lexical form (not just a colour cue) the user reads
// as "this hasn't started yet".
type stagePhraseState int

const (
	stagePhraseRunning stagePhraseState = iota
	stagePhraseDone
	stagePhrasePending
)

// stagePhrase returns the localized "primary" label for a normalized
// stage key. Stage keys are derived by friendlyPrimaryText from the
// taskRow's nodeKind / stage / agent fields. The state argument
// picks one of three lifecycle phrasings: running / done / pending.
//
// Phrasing follows the actual problem-solving narrative the user
// experiences:
//
//   1. log_triage / perf_triage (conditional pre-stages) parse the
//      attached runtime log or HiTrace dump.
//   2. analyze (StageAnalyze) understands the user's request and
//      classifies it. After this completes the request is
//      "understood"; deep analysis happens NEXT.
//   3. explore (StageExplore) is the deep-analysis phase. The task
//      graph fans out into per-topic NodeEvidence rows, optionally
//      followed by a NodeValidate that cross-checks the evidence
//      and a NodeReconcile that merges findings into a coherent
//      story.
//   4. extract (StageExtract) pulls the structured facts the
//      finalizer needs out of the explore transcript.
//   5. finalize (StageFinalize) renders the final answer prose.
//
// Write mode (when --mode=plan|apply|verify):
//   1. analyze still runs as a classifier.
//   2. plan (StagePlan) drafts a ChangePlan.
//   3. apply (StageApply) applies patches inside a worktree.
//   4. verify (StageVerify) runs tests to confirm no regression.
//
// Unknown keys fall through to a generic "正在处理任务" /
// "Processing task" so an unmapped stage never surfaces an internal
// label like "ExtractorAgent(extract)".
func stagePhrase(key string, lang string, state stagePhraseState) string {
	zh := isZh(lang)
	type triple struct{ run, done, pending string }
	tableZh := map[string]triple{
		// Pre-stages
		"log_triage":  {"正在解析日志", "已解析日志", "待解析日志"},
		"perf_triage": {"正在解析性能数据", "已解析性能数据", "待解析性能数据"},
		// Read-mode core flow
		"analyze": {"正在理解问题", "已理解问题", "待理解问题"},
		// "explore" is the orchestrator-level stage AND the topic-
		// group parent label when multiple sub-topics fan out. As a
		// single status line it reads as "正在深入分析" — the broad
		// umbrella over the per-topic evidence work.
		"explore": {"正在深入分析", "已完成深入分析", "待深入分析"},
		// "evidence" is the NodeEvidence sub-step where the agent
		// actually reads code (read_file / grep / repo_map) and
		// emits structured evidence (emit_evidence /
		// emit_investigation_complete). Distinct label from
		// "explore" so the user can tell THIS step is where the
		// substantive code reading happens — collapsing it into
		// the parent stage label hid the most important phase.
		"evidence":  {"正在探索代码并收集证据", "已完成证据收集", "待探索代码并收集证据"},
		"validate":  {"正在校核分析结论", "已校核分析结论", "待校核分析结论"},
		"reconcile": {"正在整理结论", "已整理结论", "待整理结论"},
		"extract":   {"正在提取关键要点", "已提取关键要点", "待提取关键要点"},
		"finalize":  {"正在生成最终答案", "已生成最终答案", "待生成最终答案"},
		// Write-mode flow
		"plan":   {"正在设计改动方案", "已设计改动方案", "待设计改动方案"},
		"apply":  {"正在应用改动", "已应用改动", "待应用改动"},
		"verify": {"正在跑测试验证改动", "已通过测试验证改动", "待跑测试验证改动"},
	}
	tableEn := map[string]triple{
		"log_triage":  {"Parsing attached log", "Log parsed", "Awaiting log parse"},
		"perf_triage": {"Parsing performance trace", "Performance trace parsed", "Awaiting trace parse"},
		"analyze":     {"Understanding the request", "Request understood", "Awaiting request understanding"},
		"explore":     {"Investigating the problem", "Investigation complete", "Awaiting investigation"},
		"evidence":    {"Exploring code, collecting evidence", "Evidence collected", "Awaiting code exploration"},
		"validate":    {"Cross-checking findings", "Findings cross-checked", "Awaiting cross-check"},
		"reconcile":   {"Reconciling findings", "Findings reconciled", "Awaiting reconciliation"},
		"extract":     {"Extracting key findings", "Key findings extracted", "Awaiting key-finding extraction"},
		"finalize":    {"Generating final answer", "Final answer generated", "Awaiting final answer"},
		"plan":        {"Drafting change plan", "Change plan ready", "Awaiting change plan"},
		"apply":       {"Applying changes", "Changes applied", "Awaiting change apply"},
		"verify":      {"Running tests for verification", "Tests verified", "Awaiting verification"},
	}
	var t triple
	var ok bool
	if zh {
		t, ok = tableZh[key]
	} else {
		t, ok = tableEn[key]
	}
	if !ok {
		return tCommonProcessing(zh, state)
	}
	switch state {
	case stagePhraseDone:
		return t.done
	case stagePhrasePending:
		return t.pending
	default:
		return t.run
	}
}

func tCommonProcessing(zh bool, state stagePhraseState) string {
	if zh {
		switch state {
		case stagePhraseDone:
			return "已完成任务"
		case stagePhrasePending:
			return "待处理任务"
		default:
			return "正在处理任务"
		}
	}
	switch state {
	case stagePhraseDone:
		return "Task complete"
	case stagePhrasePending:
		return "Awaiting task"
	default:
		return "Processing task"
	}
}

// thinkingPhrase localizes a row.detail string that begins with
// "thinking" / "► thinking" / "thinking (round N)" / "thinking: …".
// Returns the localized form OR an empty string when the input is
// not a thinking-shaped detail (caller falls through to other
// detail formatters).
//
// Three distinct phases collapse into the "thinking" detail prefix
// in the event stream; the user asked to see them separately so the
// long quiet windows in front of the spinner read as a specific
// waiting mode rather than an undifferentiated stall:
//
//   1. "发送中" / "sending" — bare "thinking" within `sendingThreshold`
//      of detailStart. EventAgentThinking just fired; the HTTP
//      request body is still being dispatched OR the server hasn't
//      even acknowledged yet. Brief by design (typically <1s).
//
//   2. "等待响应" / "awaiting" — bare "thinking" beyond the threshold.
//      Request is fully in flight; we are waiting on the server /
//      model to start producing tokens. This is the slow window
//      (5-30s on thinking models) where the user previously had no
//      signal that "the request was sent and we're just waiting".
//
//   3. "回复中：…" / "replying: …" — "thinking: …" with a preview
//      tail. EventAgentContent fired with the first response
//      chunk; the model IS actively streaming back tokens.
//
// Phrasing is kept short (3-5 chars zh, ≤ 8 chars en) per the user's
// "尽量简短，不易太长，又能区分" callout. The distinction is the
// load-bearing UX, not the verbosity.
var thinkingRoundRe = regexp.MustCompile(`^(?:►\s*)?thinking\s*\(round\s*(\d+)\)\s*$`)
var thinkingTextRe = regexp.MustCompile(`^(?:►\s*)?thinking[:：]\s*(.*)$`)
var thinkingBareRe = regexp.MustCompile(`^(?:►\s*)?thinking\s*$`)

// sendingThreshold splits "sending" from "awaiting". Empirically,
// HTTP request dispatch + initial server handshake completes well
// under 1s on healthy networks; beyond that the latency is
// server-side queue or model think-time, not local IO.
const sendingThreshold = 1500 * time.Millisecond

func thinkingPhrase(detail string, lang string, now, detailStart time.Time) string {
	zh := isZh(lang)
	d := strings.TrimSpace(detail)
	// "thinking: …" — first content chunk arrived; model is replying.
	if m := thinkingTextRe.FindStringSubmatch(d); m != nil {
		body := strings.TrimSpace(m[1])
		if zh {
			return "回复中：" + body
		}
		return "replying: " + body
	}
	// "thinking (round N)" — bare-thinking with iteration counter.
	if m := thinkingRoundRe.FindStringSubmatch(d); m != nil {
		phase := bareThinkingPhase(now, detailStart, zh)
		if zh {
			return fmt.Sprintf("%s · 第 %s 轮", phase, m[1])
		}
		return fmt.Sprintf("%s · round %s", phase, m[1])
	}
	// "thinking" — bare; split into "sending" vs "awaiting" by elapsed.
	if thinkingBareRe.MatchString(d) {
		return bareThinkingPhase(now, detailStart, zh)
	}
	return ""
}

// bareThinkingPhase returns the localized "sending" vs "awaiting"
// phrase for a bare-thinking row, splitting on detailStart age. When
// detailStart is zero (test path, or row constructed without an
// event timestamp), default to "awaiting" so we don't claim "sending"
// for stale rows.
//
// NTP-jump guard: if the wall clock moves backward between the time
// detailStart was captured and `now` is read (rare but observed in
// systems whose monotonic clock disagrees with the real-time clock,
// or when the runner re-synchronises on boot), now.Sub(detailStart)
// can be negative — strictly less than sendingThreshold but not in
// the "fresh" semantic sense. We require a non-negative elapsed
// before claiming "sending" so a backward-jumped clock never lies
// about request freshness.
func bareThinkingPhase(now, detailStart time.Time, zh bool) string {
	if !detailStart.IsZero() {
		elapsed := now.Sub(detailStart)
		if elapsed >= 0 && elapsed < sendingThreshold {
			if zh {
				return "发送中"
			}
			return "sending"
		}
	}
	if zh {
		return "等待响应"
	}
	return "awaiting"
}

// toolDetailPhrase localizes a tool-call detail like "grep main.go" /
// "read internal/render/renderer.go" / "emit_answer_document". The
// argument tail (everything after the first whitespace) is preserved
// verbatim — file paths, package names, search patterns must NOT be
// translated.
//
// Returns "" when input doesn't look like a tool call, so the caller
// can fall through to the raw detail text.
func toolDetailPhrase(detail string, lang string) string {
	zh := isZh(lang)
	d := strings.TrimSpace(detail)
	if d == "" {
		return ""
	}
	// Split on first whitespace: tool name + tail (path/args).
	tool := d
	tail := ""
	if i := strings.IndexAny(d, " \t"); i > 0 {
		tool = d[:i]
		tail = strings.TrimSpace(d[i+1:])
	}
	type spec struct{ zh, en string }
	table := map[string]spec{
		"grep":                 {"正在搜索", "searching"},
		"search":               {"正在搜索", "searching"},
		"read":                 {"正在读取", "reading"},
		"read_file":            {"正在读取", "reading"},
		"list_files":           {"正在查看文件列表", "listing files"},
		"repo_map":             {"正在浏览仓库结构", "browsing repo structure"},
		"apply_patch":          {"正在应用改动", "applying changes"},
		"exec_command":         {"正在执行命令", "running command"},
		"run_tests":            {"正在运行测试", "running tests"},
		"emit_analysis":        {"正在生成分析结果", "generating analysis result"},
		"emit_answer_document": {"正在生成最终答案", "generating final answer"},
		"emit_change_plan":     {"正在生成改动计划", "generating change plan"},
		"emit_plan_skeleton":   {"正在生成计划骨架", "drafting plan skeleton"},
		"emit_plan_change":     {"正在补全计划文件", "filling plan file"},
		"emit_evidence":        {"正在记录证据", "recording evidence"},
		"emit_answer_symbol":   {"正在生成答案要点", "extracting answer symbols"},
		"emit_test_results":    {"正在汇总测试结果", "summarizing test results"},
	}
	if s, ok := table[tool]; ok {
		var verb string
		if zh {
			verb = s.zh
		} else {
			verb = s.en
		}
		if tail == "" {
			return verb
		}
		return verb + " " + tail
	}
	// Unknown tool — generic phrasing with the tool name kept as the
	// technical entity (NOT translated).
	if !looksLikeToolCall(tool) {
		return ""
	}
	if zh {
		if tail == "" {
			return "正在调用工具 " + tool
		}
		return "正在调用工具 " + tool + " " + tail
	}
	if tail == "" {
		return "calling tool " + tool
	}
	return "calling tool " + tool + " " + tail
}

// looksLikeToolCall is a conservative guard: the detail must look
// like an identifier / underscored token that could be a tool name,
// not a free-form sentence. Empty whitespace or punctuation-heavy
// text returns false so the caller falls through to raw display.
var toolNameLike = regexp.MustCompile(`^[a-z][a-z0-9_]+$`)

func looksLikeToolCall(s string) bool {
	return toolNameLike.MatchString(strings.ToLower(s))
}

// topicCountPhrase produces "识别到 N 个关注点" / "N focus areas
// found" — the secondary line below the parent stage when topic
// rows are aggregated.
func topicCountPhrase(n int, lang string) string {
	if isZh(lang) {
		return fmt.Sprintf("识别到 %d 个关注点", n)
	}
	if n == 1 {
		return "1 focus area found"
	}
	return fmt.Sprintf("%d focus areas found", n)
}

// topicLabelPhrase produces the per-row "关注点 N：" / "Focus N:"
// prefix shown next to the topic body text.
func topicLabelPhrase(idx int, lang string) string {
	if isZh(lang) {
		return fmt.Sprintf("关注点 %d：", idx)
	}
	return fmt.Sprintf("Focus %d: ", idx)
}

// topicOverflowPhrase covers the >5 topics case: first 5 are shown
// individually, the rest collapse into a single line.
func topicOverflowPhrase(extra int, lang string) string {
	if isZh(lang) {
		return fmt.Sprintf("另有 %d 个关注点，已合并到后续分析中", extra)
	}
	if extra == 1 {
		return "1 more focus area merged into the follow-up analysis"
	}
	return fmt.Sprintf("%d more focus areas merged into the follow-up analysis", extra)
}

// metaToolCountPhrase produces the dim "5 次工具调用" / "5 tool
// calls" trailer used on completed status lines.
func metaToolCountPhrase(n int, lang string) string {
	if isZh(lang) {
		return fmt.Sprintf("%d 次工具调用", n)
	}
	if n == 1 {
		return "1 tool call"
	}
	return fmt.Sprintf("%d tool calls", n)
}

// metaRoundPhrase produces "第 N 轮" / "round N" for the iteration
// trailer.
func metaRoundPhrase(n int, lang string) string {
	if isZh(lang) {
		return fmt.Sprintf("第 %d 轮", n)
	}
	return fmt.Sprintf("round %d", n)
}

// footerPhrase returns the spinner-area footer ("总耗时 1m18s" /
// "Total elapsed 1m18s"). The elapsed string is the caller's job
// (formatted via time.Duration.String).
func footerPhrase(elapsed string, lang string) string {
	if isZh(lang) {
		return "总耗时 " + elapsed
	}
	return "Total elapsed " + elapsed
}

// defaultCancelHint produces the bilingual cancel-hint surfaced in
// the footer when no caller-supplied hint is set. Used by the REPL
// after SetLang installs the user's locale.
func defaultCancelHint(lang string) string {
	if isZh(lang) {
		return "按 Ctrl+C 取消，连续按两次强制退出"
	}
	return "Press Ctrl+C to cancel, press twice to force quit"
}

// normalizeTopicText cleans up an analyzer-emitted objective string
// for display under "关注点 N：". Two transforms:
//
//   1. Collapse runs of whitespace to single spaces.
//   2. zh locale: insert a space between adjacent CJK and ASCII
//      letters/digits so "analyzers包" reads as "analyzers 包" —
//      the analyzer's prose generation skips the canonical CJK
//      typography rule and the result reads cramped.
func normalizeTopicText(s string, lang string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Collapse runs of any whitespace (spaces/tabs/newlines) to one.
	s = whitespaceRunRe.ReplaceAllString(s, " ")
	if !isZh(lang) {
		return s
	}
	// CJK ↔ ASCII boundary spacing. The two regexes inject a space
	// only at boundaries that have neither side already a space, so
	// idempotent on well-typed input.
	s = cjkAsciiBoundaryRe1.ReplaceAllString(s, "$1 $2")
	s = cjkAsciiBoundaryRe2.ReplaceAllString(s, "$1 $2")
	return s
}

var (
	whitespaceRunRe     = regexp.MustCompile(`\s+`)
	cjkAsciiBoundaryRe1 = regexp.MustCompile(`([\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}])([A-Za-z0-9])`)
	cjkAsciiBoundaryRe2 = regexp.MustCompile(`([A-Za-z0-9])([\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}])`)
)

// stripInternalTagPrefix removes the "[topic 1] " / "[validate] "
// style prefix that the legacy formatTaskLine emitted, in case a
// pre-localized string is fed back through the new pipeline. Used
// as a defensive last step before display.
var internalTagPrefixRe = regexp.MustCompile(`^\[\w[\w\s]*\]\s+`)

func stripInternalTagPrefix(s string) string {
	return internalTagPrefixRe.ReplaceAllString(s, "")
}
