package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/pterm/pterm"
)

// TestRenderer_EmitterHonorsConfiguredOutput locks the renderer's
// live-output sink contract. The post-2026-04-30 liveBar refactor
// accidentally hard-coded os.Stdout throughout the event path,
// which polluted single-shot CLI stdout with progress ticks and
// agent reasoning. Supplying a bytes.Buffer here must capture the
// live output; otherwise the renderer regressed to writing outside
// its configured sink again.
func TestRenderer_EmitterHonorsConfiguredOutput(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)
	r.StartSpinner()
	defer r.StopSpinner()

	now := time.Now()
	emit := r.Emitter()
	emit(Event{
		Kind:      EventObjectiveStarted,
		Objective: "find the entry point",
		Timestamp: now,
	})
	emit(Event{
		Kind:      EventStageStart,
		Stage:     types.StageAnalyze,
		Agent:     types.AgentAnalyzer,
		Timestamp: now,
	})
	emit(Event{
		Kind:      EventAgentReasoning,
		Agent:     types.AgentAnalyzer,
		Iteration: 0,
		Reasoning: "Reasoning should surface through the configured writer.",
		Timestamp: now,
	})

	out := buf.String()
	if out == "" {
		t.Fatal("expected renderer to write live output to configured writer")
	}
	if !strings.Contains(out, "configured writer") {
		t.Fatalf("expected reasoning text in configured writer output, got %q", out)
	}
}

func TestRenderer_ReasoningDefaultsToFullText(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	longReasoning := strings.Repeat("detail ", reasoningMaxChars/len("detail ")+10) + "TAILMARKER"
	emit := r.Emitter()
	emit(Event{
		Kind:      EventAgentReasoning,
		Agent:     types.AgentAnalyzer,
		Iteration: 0,
		Reasoning: longReasoning,
		Timestamp: time.Now(),
	})

	out := buf.String()
	if !strings.Contains(out, "TAILMARKER") {
		t.Fatalf("default reasoning output should be untruncated, got %q", out)
	}
}

func TestRenderer_ReasoningTruncateOptIn(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)
	r.SetThinkingTruncate(true)

	longReasoning := strings.Repeat("detail ", reasoningMaxChars/len("detail ")+10) + "TAILMARKER"
	emit := r.Emitter()
	emit(Event{
		Kind:      EventAgentReasoning,
		Agent:     types.AgentAnalyzer,
		Iteration: 0,
		Reasoning: longReasoning,
		Timestamp: time.Now(),
	})

	out := buf.String()
	if strings.Contains(out, "TAILMARKER") {
		t.Fatalf("opt-in reasoning truncation should hide tail marker, got %q", out)
	}
	if !strings.Contains(out, "...") {
		t.Fatalf("opt-in reasoning truncation should add ellipsis, got %q", out)
	}
}

func TestFormatReasoningStylesTagAndBodySeparately(t *testing.T) {
	pterm.EnableColor()
	defer pterm.DisableColor()

	got := formatReasoning("analyzer", types.StageAnalyze, 0, "Readable detail.", false, "zh")
	expected := "  " + statusReasoningGlyph.Sprint("⋯") + " " + statusMeta.Sprint("分析 · 第 1 轮") + " " + statusReasoningBody.Sprint("Readable detail.")
	if got != expected {
		t.Fatalf("reasoning style should keep tag muted and body readable\nwant %q\ngot  %q", expected, got)
	}
	if wantGlyph := pterm.NewStyle(pterm.FgLightCyan).Sprint("⋯"); !strings.Contains(got, wantGlyph) {
		t.Fatalf("reasoning glyph should be bright but isolated\nwant glyph %q\ngot        %q", wantGlyph, got)
	}
	if wantBody := pterm.NewStyle(pterm.FgWhite, pterm.Fuzzy).Sprint("Readable detail."); !strings.Contains(got, wantBody) {
		t.Fatalf("reasoning body should use dim white to stay below answer prose while remaining readable\nwant body %q\ngot       %q", wantBody, got)
	}
	if plain := stripAnsiEscapes(got); plain != "  ⋯ 分析 · 第 1 轮 Readable detail." {
		t.Fatalf("styled reasoning changed visible text: %q", plain)
	}
}

func TestFormatReasoningFallsBackWithoutExposingUnknownAgent(t *testing.T) {
	got := formatReasoning("experimental_agent", "", 1, "Readable detail.", false, "zh")
	plain := stripAnsiEscapes(got)
	if want := "  ⋯ 模型 · 第 2 轮 Readable detail."; plain != want {
		t.Fatalf("unknown agent fallback changed\nwant %q\ngot  %q", want, plain)
	}
	if strings.Contains(plain, "experimental_agent") {
		t.Fatalf("unknown internal agent id leaked into user-facing scrollback: %q", plain)
	}
}

func TestFormatReasoningLocalizesOperationAndDataStages(t *testing.T) {
	cases := []struct {
		stage types.PipelineStage
		agent string
		body  string
		want  string
	}{
		{types.PipelineStage("operation"), "operation_planner", "生成下一批操作计划。", "  ⋯ 操作 · 第 1 轮 生成下一批操作计划。"},
		{types.PipelineStage("data"), "data_planner", "生成数据处理计划。", "  ⋯ 数据 · 第 1 轮 生成数据处理计划。"},
	}
	for _, tc := range cases {
		got := formatReasoning(tc.agent, tc.stage, 0, tc.body, false, "zh")
		plain := stripAnsiEscapes(got)
		if plain != tc.want {
			t.Fatalf("stage %q reasoning label changed\nwant %q\ngot  %q", tc.stage, tc.want, plain)
		}
	}
}

func TestFormatReasoningKeepsPlainMultilineAsCompactTrace(t *testing.T) {
	got := formatReasoning("analyzer", types.StageAnalyze, 0, "first line\nsecond line", false, "zh")
	plain := stripAnsiEscapes(got)
	if want := "  ⋯ 分析 · 第 1 轮 first line second line"; plain != want {
		t.Fatalf("plain multiline reasoning should retain legacy compact shape\nwant %q\ngot  %q", want, plain)
	}
}

func TestFormatReasoningPreservesBoxDiagramRows(t *testing.T) {
	got := formatReasoning("explorer", types.StageExplore, 20, strings.Join([]string{
		"我已经完成强制读取任务。",
		"调用流程",
		"┌───────┐",
		"│ Explorer │",
		"└───────┘",
	}, "\n"), false, "zh")
	plain := stripAnsiEscapes(got)
	for _, want := range []string{
		"⋯ 探索 · 第 21 轮 我已经完成强制读取任务。",
		"\n    调用流程",
		"\n    ┌───────┐",
		"\n    │ Explorer │",
		"\n    └───────┘",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("diagram reasoning should preserve row %q; got:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "调用流程 ┌") {
		t.Fatalf("diagram rows must not be collapsed into prose: %q", plain)
	}
}

func TestReasoningDisplayLinesPreservesFencedWideBoxRows(t *testing.T) {
	wideRow := "┌─────────────┐     ┌─────────────────┐     ┌─────────────────┐     ┌──────────────────┐"
	lines := reasoningDisplayLines(strings.Join([]string{
		"before",
		"```",
		wideRow,
		"│ Orchestrator│     │   Explorer Agent│     │  SubAgentRuntime│     │  SubExplorer(SubAgent) │",
		"```",
		"after",
	}, "\n"))

	var found bool
	for _, line := range lines {
		if stripAnsiEscapes(line.text) != wideRow {
			continue
		}
		found = true
		if !line.visual {
			t.Fatalf("fenced box-drawing row must be visual: %+v", line)
		}
	}
	if !found {
		t.Fatalf("fenced box-drawing row was split or dropped; got %+v", lines)
	}
}

func TestReasoningDisplayLinesPreservesUnfencedWideBoxRows(t *testing.T) {
	wideRow := "┌─────────────┐     ┌─────────────────┐     ┌─────────────────┐"
	lines := reasoningDisplayLines("diagram:\n" + wideRow + "\nend")

	var found bool
	for _, line := range lines {
		if stripAnsiEscapes(line.text) != wideRow {
			continue
		}
		found = true
		if !line.visual {
			t.Fatalf("box-drawing row must be visual: %+v", line)
		}
	}
	if !found {
		t.Fatalf("box-drawing row was split or dropped; got %+v", lines)
	}
}

func TestFormatReasoningSplitsCollapsedInlineBoxRows(t *testing.T) {
	got := formatReasoning("explorer", types.StageExplore, 1,
		"调用流程 ┌──┐ │ A │ └──┘", false, "zh")
	plain := stripAnsiEscapes(got)
	for _, want := range []string{
		"调用流程",
		"\n    ┌──┐",
		"\n    │ A │",
		"\n    └──┘",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("collapsed box row should be split around %q; got:\n%s", want, plain)
		}
	}
}

func TestFormatScrollbackBodyDisablesAutoWrapForVisualRows(t *testing.T) {
	body := formatScrollbackBody([]scrollbackLine{
		{text: "plain"},
		{text: "│ " + strings.Repeat("wide visual cell ", 3) + "│", visual: true},
	}, true)
	if !strings.Contains(body, "\x1b[?7l│ ") || !strings.Contains(body, "│\x1b[?7h") {
		t.Fatalf("visual scrollback rows should be bracketed with DECAWM controls: %q", body)
	}
	if strings.Contains(body, "\x1b[?7lplain") {
		t.Fatalf("plain rows must not receive visual auto-wrap controls: %q", body)
	}
}

func TestFormatAnswerDocumentDraftPreviewLinesExtractsReadableDraft(t *testing.T) {
	params := `{
		"blocks": [
			{"id":"summary","kind":"summary","text":"Explorer 调用 SubAgent 的流程如下："},
			{"id":"steps","kind":"ordered_list","items":[
				{"label":"Explorer","text":"提出子任务"},
				{"label":"SubAgentRuntime","text":"并行执行并归约"}
			]},
			{"id":"diagram","kind":"diagram","title":"流程图","diagram":{
				"language":"mermaid",
				"body":"flowchart TD\n    A --> B"
			}}
		]
	}`
	lines := formatAnswerDocumentDraftPreviewLines(params, "zh")
	if got := lines[0].text; !strings.Contains(got, statusObjective.Sprint("第一稿答案")) {
		t.Fatalf("draft preview header should use objective color, got %q", got)
	}
	plain := stripAnsiEscapes(formatScrollbackBody(lines, false))
	for _, want := range []string{
		"• 第一稿答案",
		"Explorer 调用 SubAgent 的流程如下：",
		"1. Explorer — 提出子任务",
		"2. SubAgentRuntime — 并行执行并归约",
		"流程图",
		"```mermaid",
		"flowchart TD",
		"    A --> B",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("draft preview missing %q; got:\n%s", want, plain)
		}
	}
	var sawDiagramBody bool
	for _, line := range lines {
		if strings.Contains(stripAnsiEscapes(line.text), "A --> B") {
			sawDiagramBody = true
			if !line.visual {
				t.Fatalf("diagram source row must be marked visual: %+v", line)
			}
		}
	}
	if !sawDiagramBody {
		t.Fatalf("expected diagram body row in preview: %+v", lines)
	}
}

func TestFormatAnswerDocumentDraftPreviewLinesParsesStringWrappedBlocks(t *testing.T) {
	params := `{"blocks":"[{\"id\":\"summary\",\"kind\":\"summary\",\"text\":\"wrapped draft\"}]"}`
	lines := formatAnswerDocumentDraftPreviewLines(params, "zh")
	plain := stripAnsiEscapes(formatScrollbackBody(lines, false))
	if !strings.Contains(plain, "wrapped draft") {
		t.Fatalf("string-wrapped blocks should still preview draft; got:\n%s", plain)
	}
}

func TestFormatAnswerDocumentDraftPreviewLinesRepairsStringWrappedBlocks(t *testing.T) {
	wrapped := `[
		{"id":"summary","kind":"summary","text":"探索器调用子智能体。"},
		{"id":"diagram","kind":"diagram","diagram":{
			"language":"mermaid",
			"body":"flowchart TD\n    A --> B"
		}]
	`
	encoded, err := json.Marshal(map[string]string{"blocks": wrapped})
	if err != nil {
		t.Fatal(err)
	}
	lines := formatAnswerDocumentDraftPreviewLines(string(encoded), "zh")
	plain := stripAnsiEscapes(formatScrollbackBody(lines, false))
	for _, want := range []string{
		"• 第一稿答案",
		"探索器调用子智能体。",
		"```mermaid",
		"flowchart TD",
		"    A --> B",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("draft preview missing %q; got:\n%s", want, plain)
		}
	}
}

func TestFormatAnswerDocumentDraftPreviewPreservesVisualRows(t *testing.T) {
	params := `{"blocks":[{"id":"summary","kind":"summary","text":"调用流程 ┌──┐ │ A │ └──┘\nname | role\n--- | ---\nexplorer | ` + strings.Repeat("collects evidence ", 5) + `"}]}`
	lines := formatAnswerDocumentDraftPreviewLines(params, "zh")
	plain := stripAnsiEscapes(formatScrollbackBody(lines, false))
	for _, want := range []string{
		"\n    ┌──┐",
		"\n    │ A │",
		"\n    └──┘",
		"name | role",
		"--- | ---",
		"explorer | collects evidence",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("draft preview should preserve visual row %q; got:\n%s", want, plain)
		}
	}
	body := formatScrollbackBody(lines, true)
	for _, want := range []string{
		"\x1b[?7l    " + statusReasoningBody.Sprint("│ A │") + "\x1b[?7h",
		"\x1b[?7l    " + statusReasoningBody.Sprint("name | role") + "\x1b[?7h",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("visual draft rows should disable terminal auto-wrap around %q; got:\n%q", want, body)
		}
	}
}

func TestRenderer_DropsFirstAnswerDraftPreviewWhenAccepted(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:           EventToolCallEnd,
		ToolName:       "emit_answer_document",
		ToolOK:         true,
		ToolParamsJSON: `{"blocks":[{"id":"summary","kind":"summary","text":"accepted draft"}]}`,
		Timestamp:      time.Now(),
	})
	emit(Event{
		Kind:            EventLivePreviewClear,
		PreviewRejected: false,
		Timestamp:       time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	if strings.Contains(out, "第一稿答案") || strings.Contains(out, "accepted draft") {
		t.Fatalf("accepted first draft should not duplicate final answer in scrollback; got:\n%s", out)
	}
}

func TestRenderer_SpinnerActiveTracksLiveDock(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	if r.SpinnerActive() {
		t.Fatal("fresh renderer must not report an active spinner")
	}
	r.dock = newDock(&buf)
	if !r.SpinnerActive() {
		t.Fatal("attached dock should report an active spinner")
	}
	r.StopSpinner()
	if r.SpinnerActive() {
		t.Fatal("StopSpinner should clear the live dock active flag")
	}
}

func TestRenderer_StartSpinnerClearsStaleDockWithoutTicker(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)
	r.dock = newDock(&buf)
	r.dock.paintDock([dockRowCount]string{"stale-1", "stale-2", "stale-3"})
	r.animStop = nil
	r.startTime = time.Now().Add(-30 * time.Second)
	r.activity = activityState{kind: activityRequesting}

	buf.Reset()
	r.StartSpinnerWithCancelHint("Ctrl+C")

	if r.dock != nil {
		t.Fatal("non-TTY stale dock must be cleared instead of leaving a static live region")
	}
	if r.activity.kind != activityWaitingPipeline {
		t.Fatalf("fresh spinner lifecycle should reset activity, got %v", r.activity.kind)
	}
	if r.startTime.IsZero() || time.Since(r.startTime) > time.Second {
		t.Fatalf("fresh spinner lifecycle should reset start time, got %v", r.startTime)
	}
	out := stripAnsiEscapes(buf.String())
	if strings.Contains(out, "stale-1") || strings.Contains(out, "stale-2") || strings.Contains(out, "stale-3") {
		t.Fatalf("stale dock rows should be cleared before the next spinner start, got %q", out)
	}
}

func TestRenderer_EmitsRejectedFirstAnswerDraftPreviewOnce(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:           EventToolCallEnd,
		ToolName:       "emit_answer_document",
		ToolOK:         true,
		ToolParamsJSON: `{"blocks":[{"id":"summary","kind":"summary","text":"first draft"}]}`,
		Timestamp:      time.Now(),
	})
	emit(Event{
		Kind:            EventLivePreviewClear,
		PreviewRejected: true,
		Timestamp:       time.Now(),
	})
	emit(Event{
		Kind:           EventToolCallEnd,
		ToolName:       "emit_answer_document",
		ToolOK:         true,
		ToolParamsJSON: `{"blocks":[{"id":"summary","kind":"summary","text":"second draft"}]}`,
		Timestamp:      time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	if strings.Count(out, "第一稿答案") != 1 {
		t.Fatalf("first draft preview should emit once; got:\n%s", out)
	}
	if !strings.Contains(out, "first draft") {
		t.Fatalf("first draft body missing; got:\n%s", out)
	}
	if strings.Contains(out, "second draft") {
		t.Fatalf("later drafts should not flood scrollback preview; got:\n%s", out)
	}
}

func TestRenderer_EmitsToolRejectedFirstAnswerDraftPreview(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:           EventToolCallEnd,
		ToolName:       "emit_answer_document",
		ToolOK:         false,
		ToolParamsJSON: `{"blocks":[{"id":"summary","kind":"summary","text":"rejected by tool"}]}`,
		Timestamp:      time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	if !strings.Contains(out, "第一稿答案") || !strings.Contains(out, "rejected by tool") {
		t.Fatalf("tool-rejected first draft should remain visible for diagnosis; got:\n%s", out)
	}
}

func TestFormatToolCallBatchSummarizesPureToolResponse(t *testing.T) {
	pterm.EnableColor()
	defer pterm.DisableColor()

	got := formatToolCallBatch("explorer", types.StageExplore, 19,
		[]string{"read_file"},
		1,
		"read_file",
		"internal/tool/apply_patch.go",
		"zh",
	)
	plain := stripAnsiEscapes(got)
	want := "  ⇢ 探索 · 第 20 轮 调用工具 read_file internal/tool/apply_patch.go"
	if plain != want {
		t.Fatalf("tool-only response line changed\nwant %q\ngot  %q", want, plain)
	}
	if wantGlyph := pterm.NewStyle(pterm.FgLightGreen).Sprint("⇢"); !strings.Contains(got, wantGlyph) {
		t.Fatalf("tool-call glyph should be bright but isolated\nwant glyph %q\ngot        %q", wantGlyph, got)
	}
}

func TestFormatToolCallBatchShowsRepoMapStructuredDetail(t *testing.T) {
	got := formatToolCallBatch("analyzer", types.StageAnalyze, 0,
		[]string{"repo_map"},
		1,
		"repo_map",
		"view=task_map path=. query=map update top_n=20",
		"zh",
	)
	plain := stripAnsiEscapes(got)
	want := "⇢ 分析 · 第 1 轮 调用工具 repo_map view=task_map path=. query=map update top_n=20"
	if !strings.Contains(plain, want) {
		t.Fatalf("repo_map tool-call detail missing\nwant contains %q\ngot           %q", want, plain)
	}
}

func TestFormatToolCallBatchShowsTraceQueryStructuredDetail(t *testing.T) {
	got := formatToolCallBatch("explorer", types.StageExplore, 2,
		[]string{"trace_query"},
		1,
		"trace_query",
		"view=wakeup_chain path=trace.systrace pid=36379 window=2942.124416..2942.260210 max_depth=6",
		"zh",
	)
	plain := stripAnsiEscapes(got)
	want := "⇢ 探索 · 第 3 轮 调用工具 trace_query view=wakeup_chain path=trace.systrace pid=36379 window=2942.124416..2942.260210 max_depth=6"
	if !strings.Contains(plain, want) {
		t.Fatalf("trace_query tool-call detail missing\nwant contains %q\ngot           %q", want, plain)
	}
}

func TestFormatToolCallBatchShowsMCPToolAsMCP(t *testing.T) {
	got := formatToolCallBatch("explorer", types.StageExplore, 0,
		[]string{"fixture__lookup_trace_fact"},
		1,
		"fixture__lookup_trace_fact",
		"target pid=4242 sleep wakeup",
		"zh",
	)
	plain := stripAnsiEscapes(got)
	want := "⇢ 探索 · 第 1 轮 调用 MCP fixture.lookup_trace_fact target pid=4242 sleep wakeup"
	if !strings.Contains(plain, want) {
		t.Fatalf("MCP tool-call detail missing\nwant contains %q\ngot           %q", want, plain)
	}
}

func TestFormatToolCallBatchShowsMCPResourceRead(t *testing.T) {
	got := formatToolCallBatch("explorer", types.StageExplore, 1,
		[]string{"mcp_read_resource"},
		1,
		"mcp_read_resource",
		"uri=mcp://fixture/trace/sleep-wakeup",
		"zh",
	)
	plain := stripAnsiEscapes(got)
	want := "⇢ 探索 · 第 2 轮 读取 MCP 资源 mcp://fixture/trace/sleep-wakeup"
	if !strings.Contains(plain, want) {
		t.Fatalf("MCP resource-read detail missing\nwant contains %q\ngot           %q", want, plain)
	}
}

func TestFormatToolCallBatchCompactsMCPBatch(t *testing.T) {
	got := formatToolCallBatch("explorer", types.StageExplore, 2,
		[]string{"mcp_read_resource", "fixture__lookup_trace_fact"},
		2,
		"mcp_read_resource",
		"",
		"en",
	)
	plain := stripAnsiEscapes(got)
	for _, want := range []string{"calling 2 MCP tools", "mcp_read_resource", "fixture.lookup_trace_fact"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("MCP batch line missing %q; got %q", want, plain)
		}
	}
}

func TestFormatToolCallBatchCompactsRepeatedTools(t *testing.T) {
	got := formatToolCallBatch("explorer", types.StageExplore, 2,
		[]string{"read_file", "read_file", "grep", "emit_evidence", "repo_map", "list_files"},
		6,
		"read_file",
		"",
		"en",
	)
	plain := stripAnsiEscapes(got)
	for _, want := range []string{"⇢ Explore · Round 3", "calling 6 tools", "read_file x2", "grep", "emit_evidence", "repo_map", "+1"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("tool batch line missing %q; got %q", want, plain)
		}
	}
}

func TestRenderer_ToolCallBatchUsesConfiguredOutput(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:          EventAgentToolCallBatch,
		Agent:         types.AgentExplorer,
		Stage:         types.StageExplore,
		Iteration:     3,
		ToolName:      "emit_evidence",
		ToolNames:     []string{"emit_evidence"},
		ToolCallCount: 1,
		Timestamp:     time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{"⇢ 探索 · 第 4 轮", "调用工具 emit_evidence"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tool batch event must leave visible output %q; got %q", want, out)
		}
	}
}

func TestRenderer_NonTTYProgressUsesREPLStagePhrases(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)
	r.SetLang("zh")

	emit := r.Emitter()
	now := time.Now()
	emit(Event{
		Kind:      EventStageStart,
		Stage:     types.StageExplore,
		Timestamp: now,
	})
	emit(Event{
		Kind:          EventTaskNodeStart,
		NodeKind:      "evidence",
		NodeObjective: "Collect evidence for each architectural component the user asked about.",
		Timestamp:     now.Add(time.Millisecond),
	})
	emit(Event{
		Kind:          EventTaskNodeStart,
		NodeKind:      "evidence",
		NodeObjective: "Collect evidence for a sibling topic.",
		Timestamp:     now.Add(1500 * time.Microsecond),
	})
	emit(Event{
		Kind:      EventTaskNodeEnd,
		NodeKind:  "validate",
		Timestamp: now.Add(2 * time.Millisecond),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{"→ 正在深入分析", "→ 正在收集证据", "✓ 已交叉验证证据"} {
		if !strings.Contains(out, want) {
			t.Fatalf("non-TTY progress should use REPL stage phrase %q; got %q", want, out)
		}
	}
	for _, banned := range []string{"→ explore", "Collect evidence", "✓ validate"} {
		if strings.Contains(out, banned) {
			t.Fatalf("non-TTY progress leaked internal label/objective %q; got %q", banned, out)
		}
	}
	if got := strings.Count(out, "→ 正在收集证据"); got != 1 {
		t.Fatalf("non-TTY progress should collapse consecutive duplicate node labels, got %d in %q", got, out)
	}
}

func TestRenderer_NonTTYAnalysisReadyPrintsAnswerDimensions(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)
	r.SetLang("zh")

	emit := r.Emitter()
	now := time.Now()
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: now,
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "唤醒链"},
			{ID: "n1_evidence_t1", Type: "evidence", Objective: "CPU 供给"},
		},
		AnswerDimensions: []AnswerDimensionInfo{
			{Index: 1, Label: "根因结论"},
			{Index: 2, Label: "证据支撑"},
		},
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{
		"分析拆分为 2 个调查单元",
		"分析拆分为 2 个答案维度",
		"根因结论",
		"证据支撑",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("analysis-ready output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderer_NonTTYAnalysisReadyPrintsAnswerDimensionsWithoutTaskNodes(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)
	r.SetLang("zh")

	emit := r.Emitter()
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: time.Now(),
		AnswerDimensions: []AnswerDimensionInfo{
			{Index: 1, Label: "根因结论"},
			{Index: 2, Label: "证据支撑"},
		},
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{
		"分析拆分为 2 个答案维度",
		"① 根因结论",
		"② 证据支撑",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("analysis-ready answer dimensions without task nodes missing %q:\n%s", want, out)
		}
	}
}

func TestRenderer_NonTTYSkipsPostAnalysisStageRowsOwnedByTaskNodes(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)
	r.SetLang("zh")
	r.analysisReady = true

	emit := r.Emitter()
	now := time.Now()
	emit(Event{
		Kind:      EventTaskNodeStart,
		NodeKind:  "finalize",
		Timestamp: now,
	})
	emit(Event{
		Kind:      EventStageStart,
		Stage:     types.StageFinalize,
		Timestamp: now.Add(time.Millisecond),
	})
	emit(Event{
		Kind:      EventTaskNodeEnd,
		NodeKind:  "finalize",
		Timestamp: now.Add(2 * time.Millisecond),
	})
	emit(Event{
		Kind:      EventStageEnd,
		Stage:     types.StageFinalize,
		Timestamp: now.Add(3 * time.Millisecond),
	})

	out := stripAnsiEscapes(buf.String())
	if got := strings.Count(out, "→ 正在撰写最终答案"); got != 1 {
		t.Fatalf("finalize start should render once from task node, got %d in %q", got, out)
	}
	if got := strings.Count(out, "✓ 已撰写最终答案"); got != 1 {
		t.Fatalf("finalize done should render once from task node, got %d in %q", got, out)
	}
}

func TestRenderer_MCPToolEndSurfacesObservationSummary(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:              EventToolCallEnd,
		Agent:             types.AgentExplorer,
		Stage:             types.StageExplore,
		ToolName:          "fixture__lookup_trace_fact",
		ToolOK:            true,
		ToolResultSummary: "server=fixture method=tools/call resource=mcp://fixture/trace/sleep-wakeup observations=2 lines=7,12",
		Timestamp:         time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{"MCP 返回 2 条外部观测", "资源 mcp://fixture/trace/sleep-wakeup", "行 7,12"} {
		if !strings.Contains(out, want) {
			t.Fatalf("MCP result summary missing %q; got %q", want, out)
		}
	}
	if !strings.Contains(out, "行 7,12\n") {
		t.Fatalf("MCP result summary must end with a newline before the next status row; got %q", out)
	}
}

func TestRenderer_MCPToolEndKeepsLargePayloadAsReference(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	largePayloadSnippet := strings.Repeat("VERY_LARGE_MCP_PAYLOAD ", 64)
	r.Emitter()(Event{
		Kind:              EventToolCallEnd,
		ToolName:          "fixture__large_table",
		ToolOK:            true,
		ToolResultSummary: "server=fixture method=tools/call resource=mcp://fixture/large/table observations=128 payload_ref=/tmp/codrax/blob/mcp-large.txt rowset_ref=/tmp/codrax/blob/mcp-large.jsonl " + largePayloadSnippet,
		Timestamp:         time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{"MCP 返回 128 条外部观测", "资源 mcp://fixture/large/table", "blob /tmp/codrax/blob/mcp-large.txt", "rows /tmp/codrax/blob/mcp-large.jsonl"} {
		if !strings.Contains(out, want) {
			t.Fatalf("MCP large-output summary missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "VERY_LARGE_MCP_PAYLOAD") || strings.Contains(out, "payload_ref=") || strings.Contains(out, "rowset_ref=") {
		t.Fatalf("MCP large-output summary must keep payload compact and reference-only, got %q", out)
	}
}

func TestRenderer_MCPToolEndSurfacesRepeatedObservationSummary(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	r.Emitter()(Event{
		Kind:              EventToolCallEnd,
		ToolName:          "fixture__lookup",
		ToolOK:            true,
		ToolResultSummary: "server=fixture method=tools/call resource=mcp://fixture/trace/sleep-wakeup observations=2 lines=7,12 repeated=true",
		Timestamp:         time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{"MCP 返回 2 条外部观测（与前次相同）", "资源 mcp://fixture/trace/sleep-wakeup", "行 7,12"} {
		if !strings.Contains(out, want) {
			t.Fatalf("MCP repeated result summary missing %q in %q", want, out)
		}
	}
}

func TestFormatEvidenceToolResultSummaryExternalObservationSkipZh(t *testing.T) {
	summary := "emit_evidence accepted 0 source evidence item(s); skipped 2 external observation item(s).\n\n" +
		"  - items[0]: pid=4242 @ mcp://fixture/trace/sleep-wakeup:7 is an external observation URI, not a current-source read_file anchor\n\n" +
		"MCP resources and other external observations are first-class evidence in the external observation lane.\n"
	got := stripAnsiEscapes(formatEvidenceToolResultSummary("emit_evidence", summary, "zh", 0))
	for _, want := range []string{"外部观测 2 条（未作为源码证据记录）", "未将其当作当前源码引用", "aggregate_facts"} {
		if !strings.Contains(got, want) {
			t.Fatalf("external observation skip summary missing %q; got %q", want, got)
		}
	}
	if strings.Contains(got, "证据 2 条") || strings.Contains(got, "未落地") {
		t.Fatalf("external observation skip must not render as ungrounded source evidence; got %q", got)
	}
}

func TestRenderer_UnavailableToolBatchDoesNotSayCallingTool(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:                 EventAgentToolCallBatch,
		Agent:                types.AgentFinalizer,
		Stage:                types.StageFinalize,
		Iteration:            1,
		ToolName:             "grep",
		ToolNames:            []string{"grep", "grep"},
		ToolCallCount:        2,
		ToolUnavailable:      true,
		UnavailableToolNames: []string{"grep"},
		Timestamp:            time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	if !strings.Contains(out, "尝试不可用工具 grep") {
		t.Fatalf("unavailable tool attempt should be explicit; got %q", out)
	}
	if strings.Contains(out, "调用工具 grep") || strings.Contains(out, "调用 2 个工具 grep") {
		t.Fatalf("unavailable tool attempt must not render as real tool execution; got %q", out)
	}
}

func TestRenderer_ParallelExplorerScrollbackShowsLaneOrdinal(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:            EventParallelDispatchStart,
		Agent:           types.AgentExplorer,
		Stage:           types.StageExplore,
		ParallelGroupID: "g",
		ParallelTotal:   2,
		Parallelism:     2,
		ParallelUnitIDs: []string{"u0", "u1"},
		Timestamp:       time.Now(),
	})
	emit(Event{
		Kind:            EventParallelDispatchUnitStart,
		Agent:           types.AgentExplorer,
		Stage:           types.StageExplore,
		ParallelGroupID: "g",
		ParallelUnitID:  "u1",
		Timestamp:       time.Now(),
	})
	emit(Event{
		Kind:            EventAgentReasoning,
		Agent:           types.AgentExplorer,
		Stage:           types.StageExplore,
		Iteration:       9,
		Reasoning:       "second lane reasoning",
		ParallelGroupID: "g",
		ParallelUnitID:  "u1",
		Timestamp:       time.Now(),
	})
	emit(Event{
		Kind:            EventAgentToolCallBatch,
		Agent:           types.AgentExplorer,
		Stage:           types.StageExplore,
		Iteration:       9,
		ToolName:        "read_file",
		ToolNames:       []string{"read_file"},
		ToolCallCount:   1,
		ParallelGroupID: "g",
		ParallelUnitID:  "u1",
		Timestamp:       time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{
		"⋯ 探索 · 第 2 路 · 第 10 轮 second lane reasoning",
		"⇢ 探索 · 第 2 路 · 第 10 轮 调用工具 read_file",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("parallel explorer scrollback missing %q; got %q", want, out)
		}
	}
	if strings.Contains(out, "探索 · 第 10 轮 second lane reasoning") {
		t.Fatalf("parallel explorer reasoning should not use ambiguous non-lane label: %q", out)
	}
}

func TestRenderer_ParallelExplorerScrollbackShowsUnitLaneLabel(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	now := time.Now()
	emit(Event{
		Kind:            EventParallelDispatchStart,
		Agent:           types.AgentExplorer,
		Stage:           types.StageExplore,
		ParallelGroupID: "g",
		ParallelTotal:   2,
		Parallelism:     2,
		ParallelUnitIDs: []string{"u0", "u1"},
		Timestamp:       now,
	})
	emit(Event{
		Kind:               EventParallelDispatchUnitStart,
		Agent:              types.AgentExplorer,
		Stage:              types.StageExplore,
		ParallelGroupID:    "g",
		ParallelUnitID:     "u1",
		ParallelLaneLabels: []string{"vcs_diff", "current_source"},
		Timestamp:          now.Add(time.Millisecond),
	})
	emit(Event{
		Kind:            EventAgentToolCallBatch,
		Agent:           types.AgentExplorer,
		Stage:           types.StageExplore,
		Iteration:       4,
		ToolName:        "git_diff",
		ToolNames:       []string{"git_diff"},
		ToolCallCount:   1,
		ParallelGroupID: "g",
		ParallelUnitID:  "u1",
		Timestamp:       now.Add(2 * time.Millisecond),
	})

	out := stripAnsiEscapes(buf.String())
	want := "⇢ 探索 · 第 2 路（历史差异、当前源码） · 第 5 轮 调用工具 git_diff"
	if !strings.Contains(out, want) {
		t.Fatalf("parallel explorer scrollback missing unit lane label %q; got %q", want, out)
	}
	if strings.Contains(out, "vcs_diff") || strings.Contains(out, "current_source") {
		t.Fatalf("scrollback should localize lane labels instead of raw enum names: %q", out)
	}
}

func TestRenderer_SerialReconcileExploreScrollbackShowsAggregateLabel(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:         EventAgentReasoning,
		Agent:        types.AgentExplorer,
		Stage:        types.StageExplore,
		Iteration:    4,
		Reasoning:    "reconcile findings",
		DispatchKind: string(types.NodeReconcile),
		Timestamp:    time.Now(),
	})
	emit(Event{
		Kind:          EventAgentToolCallBatch,
		Agent:         types.AgentExplorer,
		Stage:         types.StageExplore,
		Iteration:     4,
		ToolName:      "emit_investigation_complete",
		ToolNames:     []string{"emit_investigation_complete"},
		ToolCallCount: 1,
		DispatchKind:  string(types.NodeReconcile),
		Timestamp:     time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{
		"⋯ 探索 · 汇总 · 第 5 轮 reconcile findings",
		"⇢ 探索 · 汇总 · 第 5 轮 调用工具 emit_investigation_complete",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("serial reconcile scrollback missing %q; got %q", want, out)
		}
	}
	if strings.Contains(out, "探索 · 第 5 轮 reconcile findings") {
		t.Fatalf("serial reconcile should not use ambiguous bare explore label: %q", out)
	}
}

func TestFormatEvidenceToolResultSummaryZh(t *testing.T) {
	summary := `emit_evidence accepted 4 item(s)

  [1] mechanism NewSubExplorer @ internal/agent/sub_explorer.go:26 — 构造 explorer 子代理
      → grounded (tier=line_text)
  [2] mechanism toolConfidence @ internal/agent/sub_explorer.go:301 — 返回固定置信度 0.8
      → recovered (tier=line_text, you claimed line 312, adjusted to 301)
  [3] direct buildScopedSearchGraph @ internal/agent/sub_explorer.go:361
      → ungrounded: not in read window
  [4] direct extra @ internal/agent/sub_explorer.go:374 — 额外证据
      → grounded (tier=line_text)
`
	got := stripAnsiEscapes(formatEvidenceToolResultSummary("emit_evidence", summary, "zh", 7))
	for _, want := range []string{
		"• 证据 4 条（累计 7 条）",
		"1. 已落地 NewSubExplorer @ internal/agent/sub_explorer.go:26 — 构造 explorer 子代理",
		"2. 已校正 toolConfidence @ internal/agent/sub_explorer.go:301 — 返回固定置信度 0.8",
		"3. 未落地 buildScopedSearchGraph @ internal/agent/sub_explorer.go:361",
		"… 还有 1 条",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("evidence summary missing %q; got:\n%s", want, got)
		}
	}
}

func TestRenderer_EmitsEvidenceSummaryOnToolEnd(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:              EventToolCallEnd,
		ToolName:          "emit_evidence",
		ToolOK:            true,
		ToolResultSummary: "emit_evidence accepted 1 item(s)\n\n  [1] mechanism NewSubExplorer @ internal/agent/sub_explorer.go:26 — 构造 explorer 子代理\n      → grounded (tier=line_text)\n",
		EvidenceTotal:     3,
		Timestamp:         time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{"证据 1 条（累计 3 条）", "NewSubExplorer @ internal/agent/sub_explorer.go:26", "构造 explorer 子代理"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tool end should surface evidence summary %q; got %q", want, out)
		}
	}
}

func TestFormatAnalysisToolResultSummaryZh(t *testing.T) {
	params := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"question_kind": "mechanism",
		"predicate_axis": "call",
		"entities": ["SubExplorer", "NewSubExplorer", "RegisterDefaultSubAgents", "buildToolSchemas", "propose_sub_agents", "SubAgents"],
		"keywords": ["explorer", "SubExplorer", "sub_agent", "sequence", "流程图", "机制", "调用流程"],
		"answer_subject": {"kind": "function_name", "entity_axes": ["SubAgent -> mechanism"]},
		"diagram_hint": {"kind": "sequence"},
		"sub_topics": [
			{"summary": "explorer 调用入口", "entities": ["explorer"]},
			{"summary": "subagent 注册与调度", "entities": ["subagent"]}
		],
		"buckets": [
			{"index": 1, "label": "diff 线索"},
			{"index": 2, "label": "当前代码"}
		],
		"requested_answer_dimensions": {
			"is_dimensioned_answer": true,
			"dimensions": [
				{"index": 2, "label": "当前关键代码", "role": "current_key_code", "required": true},
				{"index": 1, "label": "diff 线索", "role": "diff_clue", "required": true}
			]
		},
		"current_source_explanation_profile": {
			"is_current_source_explanation_requested": true,
			"modes": ["compare_with_current_source"],
			"target_terms": ["SubExplorer"]
		},
		"required_files": [{"path": "internal/agent/sub_explorer.go"}]
	}`
	got := stripAnsiEscapes(formatStructuredToolResultSummary("emit_analysis", params, "", "zh", 0))
	for _, want := range []string{
		"• 分析结果",
		"意图 explain · 类型 mechanism · 场景 architecture_explain · 复杂度 moderate · 谓词轴 call",
		"答案主体 function_name",
		"实体 6 个：SubExplorer, NewSubExplorer, RegisterDefaultSubAgents, buildToolSchemas, propose_sub_agents, +1",
		"关键词 7 个：explorer, SubExplorer, sub_agent, sequence, 流程图, 机制, +1",
		"分析拆分为 2 个调查单元：",
		"① explorer 调用入口",
		"② subagent 注册与调度",
		"分析保留了 2 个用户分区：",
		"① diff 线索",
		"② 当前代码",
		"分析拆分为 2 个答案维度：",
		"① diff 线索",
		"② 当前关键代码",
		"源码关联 2 个：compare_with_current_source, SubExplorer",
		"图 sequence",
		"建议文件 1 个：internal/agent/sub_explorer.go",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("analysis summary missing %q; got:\n%s", want, got)
		}
	}
}

func TestFormatAnalysisToolResultSummaryUsesNormalizedRequiredFiles(t *testing.T) {
	params := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"question_kind": "mechanism",
		"entities": ["OAuthCredentials"],
		"keywords": ["OAuthCredentials"],
		"required_files": [{"path": "packages/core/src/mcp/token-storage/types.ts"}]
	}`
	summary := `analysis emitted: intent=explain scenario=architecture_explain complexity=moderate kw=1 ent=1 kind=mechanism required_files=["CodeAgent/packages/core/src/mcp/token-storage/types.ts"] | warn: required_files: 1 path(s) normalized to active repo-relative form`
	got := stripAnsiEscapes(formatStructuredToolResultSummary("emit_analysis", params, summary, "zh", 0))
	if !strings.Contains(got, "建议文件 1 个：CodeAgent/packages/core/src/mcp/token-storage/types.ts") {
		t.Fatalf("analysis summary should render normalized required_files; got:\n%s", got)
	}
	if strings.Contains(got, "建议文件 1 个：packages/core/src/mcp/token-storage/types.ts") {
		t.Fatalf("analysis summary should not render stale raw required_files; got:\n%s", got)
	}
}

func TestFormatAnalysisToolResultSummaryUsesResultSummaryTopicsAndDimensions(t *testing.T) {
	summary := `analysis emitted: intent=trace scenario=performance_bottleneck complexity=complex kw=2 ent=1 kind=root_cause sub_topics=["唤醒链","CPU 供给"] answer_dimensions=["根因结论","证据支撑"]`
	got := stripAnsiEscapes(formatStructuredToolResultSummary("emit_analysis", "", summary, "zh", 0))
	for _, want := range []string{
		"• 分析结果",
		"意图 trace · 类型 root_cause · 场景 performance_bottleneck · 复杂度 complex",
		"分析拆分为 2 个调查单元：",
		"① 唤醒链",
		"② CPU 供给",
		"分析拆分为 2 个答案维度：",
		"① 根因结论",
		"② 证据支撑",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("analysis result-summary fallback missing %q; got:\n%s", want, got)
		}
	}
}

func TestFormatAnswerDocumentToolResultSummaryRejectedZh(t *testing.T) {
	summary := "The answer document does not yet meet the structural contract for this question.\n\n" +
		"  1. Field: `blocks[].items[].label`\n" +
		"     Action: structured answer anchor label(s) must preserve: explorer\n" +
		"     Why: typed mechanism-anchor contract requires exact endpoint anchors.\n" +
		"  2. Field: `citations[]`\n" +
		"     Action: need at least 14 entries because citation_ref indexes up to 13\n"
	got := stripAnsiEscapes(formatStructuredToolResultSummary("emit_answer_document", "", summary, "zh", 0))
	for _, want := range []string{
		"• 成文校验未通过",
		"1. blocks[].items[].label: structured answer anchor label(s) must preserve: explorer",
		"2. citations[]: need at least 14 entries because citation_ref indexes up to 13",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("answer document reject summary missing %q; got:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "\n    1. blocks[].items[].label:") {
		t.Fatalf("reject summary should preserve numbered action indentation, got:\n%s", got)
	}
	if !strings.Contains(got, "\n    2. citations[]:") {
		t.Fatalf("reject summary should preserve later action indentation, got:\n%s", got)
	}
}

func TestFormatAnswerDocumentToolResultSummaryRejectedZhWrapsWithoutDroppingAction(t *testing.T) {
	longAction := "each symbol-like item label must cite the evidence line whose subject/object/anchor names that same label; candidate_citations=[internal/agent/sub_explorer.go:135, internal/agent/sub_explorer.go:139]"
	summary := "The answer document does not yet meet the structural contract for this question.\n\n" +
		"  1. Field: `blocks[].items[].citation_ref`\n" +
		"     Action: " + longAction + "\n"
	got := stripAnsiEscapes(formatStructuredToolResultSummary("emit_answer_document", "", summary, "zh", 0))
	if !strings.Contains(got, "candidate_citations=[internal/agent/sub_explorer.go:135, internal/agent/sub_explorer.go:139]") {
		t.Fatalf("wrapped reject summary must preserve full action; got:\n%s", got)
	}
	if strings.Contains(got, "…") {
		t.Fatalf("reject summary should wrap, not truncate: %q", got)
	}
	if !strings.Contains(got, "\n    1. blocks[].items[].citation_ref:") {
		t.Fatalf("wrapped reject summary should preserve first-line indentation, got:\n%s", got)
	}
	if !strings.Contains(got, "\n       candidate_citations=") {
		t.Fatalf("wrapped reject summary should use hanging indentation, got:\n%s", got)
	}
}

func TestFormatAnswerDocumentToolResultSummaryAcceptedZh(t *testing.T) {
	summary := "emit_answer_document accepted: replace_all blocks=4 citations=7"
	got := stripAnsiEscapes(formatStructuredToolResultSummary("emit_answer_document", "", summary, "zh", 0))
	for _, want := range []string{
		"• 答案草稿已写入：4 个区块 · 7 条引用",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("answer document accepted summary missing %q; got:\n%s", want, got)
		}
	}
}

func TestRenderer_EmitsAnalysisSummaryOnToolEnd(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:           EventToolCallEnd,
		ToolName:       "emit_analysis",
		ToolOK:         true,
		ToolParamsJSON: `{"intent":"explain","scenario":"architecture_explain","complexity":"moderate","question_kind":"mechanism","entities":["SubExplorer"],"keywords":["explorer"],"diagram_hint":{"kind":"sequence"}}`,
		Timestamp:      time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{"分析结果", "意图 explain", "实体 1 个：SubExplorer", "图 sequence"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tool end should surface analysis summary %q; got %q", want, out)
		}
	}
}

func TestRenderer_EmitsAnalysisSummaryTopicsAndDimensionsFromToolResult(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:              EventToolCallEnd,
		ToolName:          "emit_analysis",
		ToolOK:            true,
		ToolResultSummary: `analysis emitted: intent=trace scenario=performance_bottleneck complexity=complex kw=2 ent=1 kind=root_cause sub_topics=["唤醒链","CPU 供给"] answer_dimensions=["根因结论","证据支撑"]`,
		Timestamp:         time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{
		"分析拆分为 2 个调查单元：",
		"① 唤醒链",
		"② CPU 供给",
		"分析拆分为 2 个答案维度：",
		"① 根因结论",
		"② 证据支撑",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("tool end should surface analysis fallback %q; got %q", want, out)
		}
	}
}

func TestRenderer_EmitsAnswerDocumentFailureSummaryOnToolEnd(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:     EventToolCallEnd,
		ToolName: "emit_answer_document",
		ToolOK:   false,
		ToolResultSummary: "The answer document does not yet meet the structural contract for this question.\n\n" +
			"  1. Field: `blocks[].items[].label`\n" +
			"     Action: structured answer anchor label(s) must preserve: explorer\n",
		Timestamp: time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{"成文校验未通过", "blocks[].items[].label", "must preserve: explorer"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tool end should surface answer document failure %q; got %q", want, out)
		}
	}
}
