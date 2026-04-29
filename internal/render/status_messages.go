package render

import (
	"fmt"
	"regexp"
	"strings"
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

// stagePhrase returns the localized "primary" label for a normalized
// stage key. Stage keys are derived by friendlyPrimaryText from the
// taskRow's nodeKind / stage / agent fields. running == true picks
// the in-progress phrasing; running == false picks the completion
// phrasing (used when a row's endTime is non-zero).
//
// Unknown keys fall through to a generic "正在处理任务" /
// "Processing task" so an unmapped stage never surfaces an internal
// label like "ExtractorAgent(extract)".
func stagePhrase(key string, lang string, running bool) string {
	zh := isZh(lang)
	type pair struct{ run, done string }
	tableZh := map[string]pair{
		"analyze":   {"正在理解问题", "已完成问题分析"},
		"explore":   {"正在查找相关代码", "已完成代码查找"},
		"extract":   {"正在提取关键信息", "已完成关键信息提取"},
		"evidence":  {"正在收集关键证据", "已完成证据收集"},
		"validate":  {"正在验证结论可靠性", "已完成结论验证"},
		"reconcile": {"正在整理回答", "已完成回答整理"},
		"finalize":  {"正在生成最终答案", "已生成最终答案"},
		"plan":      {"正在制定改动计划", "已制定改动计划"},
		"apply":     {"正在应用改动", "已应用改动"},
		"verify":    {"正在验证改动", "已验证改动"},
	}
	tableEn := map[string]pair{
		"analyze":   {"Understanding the request", "Analysis complete"},
		"explore":   {"Searching relevant code", "Search complete"},
		"extract":   {"Extracting key information", "Extraction complete"},
		"evidence":  {"Collecting key evidence", "Evidence collected"},
		"validate":  {"Validating conclusion reliability", "Validation complete"},
		"reconcile": {"Organizing the answer", "Answer organized"},
		"finalize":  {"Generating final answer", "Final answer generated"},
		"plan":      {"Drafting change plan", "Change plan ready"},
		"apply":     {"Applying changes", "Changes applied"},
		"verify":    {"Verifying changes", "Changes verified"},
	}
	var p pair
	var ok bool
	if zh {
		p, ok = tableZh[key]
	} else {
		p, ok = tableEn[key]
	}
	if !ok {
		if zh {
			return tCommonProcessing(true /*zh*/, running)
		}
		return tCommonProcessing(false, running)
	}
	if running {
		return p.run
	}
	return p.done
}

func tCommonProcessing(zh bool, running bool) string {
	if zh {
		if running {
			return "正在处理任务"
		}
		return "已完成任务"
	}
	if running {
		return "Processing task"
	}
	return "Task complete"
}

// thinkingPhrase localizes a row.detail string that begins with
// "thinking" / "► thinking" / "thinking (round N)" / "thinking: …".
// Returns the localized form OR an empty string when the input is
// not a thinking-shaped detail (caller falls through to other
// detail formatters).
var thinkingRoundRe = regexp.MustCompile(`^(?:►\s*)?thinking\s*\(round\s*(\d+)\)\s*$`)
var thinkingTextRe = regexp.MustCompile(`^(?:►\s*)?thinking[:：]\s*(.*)$`)
var thinkingBareRe = regexp.MustCompile(`^(?:►\s*)?thinking\s*$`)

func thinkingPhrase(detail string, lang string) string {
	zh := isZh(lang)
	d := strings.TrimSpace(detail)
	if m := thinkingRoundRe.FindStringSubmatch(d); m != nil {
		if zh {
			return fmt.Sprintf("思考中 · 第 %s 轮", m[1])
		}
		return fmt.Sprintf("thinking · round %s", m[1])
	}
	if m := thinkingTextRe.FindStringSubmatch(d); m != nil {
		body := strings.TrimSpace(m[1])
		if zh {
			return "思考中：" + body
		}
		return "thinking: " + body
	}
	if thinkingBareRe.MatchString(d) {
		if zh {
			return "思考中"
		}
		return "thinking"
	}
	return ""
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
