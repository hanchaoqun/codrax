package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Capture the real default analyzer's first adapter request. These fixtures
// exercise subject guidance, not a classifier oracle: the adapter stops before
// any model-authored classification or tool call can manufacture evidence.
func TestAnalyzerCapabilitySubjectActualInitialMessages(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name, zh, en string
			withSource   bool
			withTrace    bool
		}{
			{"host_empty_repo", "尚未提供 trace，请介绍分析工具能查询什么，以及各项所需数据和单位。", "No trace is supplied yet; describe the analysis tools' available queries, necessary data, and units.", false, false},
			{"host_unrelated_repo", "这个分析程序支持哪些查询？当前目标仓只是示例应用。", "Which queries does this analysis program support? The target repository is only an example app.", true, false},
			{"target_registration", "列出目标代码仓注册的处理器及其注册位置。", "List the handlers registered by the target repository and their registration locations.", true, false},
			{"mixed_with_capture", "介绍工具支持的等待分析，并分析所附 trace，说明目标仓中的处理器实现。", "Describe the tool's wait-analysis capabilities, analyze the attached trace, and explain the target repository's handler implementation.", true, true},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				root := t.TempDir()
				if tc.withSource {
					if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app\nfunc Handler() {}\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				request := tc.en
				if lang == "zh" {
					request = tc.zh
				}
				bus := &types.BusContext{RepoRoot: root, WorkDir: root, Language: lang, Mode: types.ModeRead,
					Mutable: types.NewMutableState(request)}
				if tc.withTrace {
					bus.AttachedHitrace = "app-42 (42) [000] .... 1.000000: sched_switch: prev_pid=42 prev_state=S ==> next_pid=0\n"
				}
				ctx := ctxbuilder.BuildAgentContext(bus, types.AgentAnalyzer, types.StageAnalyze)
				reg := toolpkg.NewRegistry()
				toolpkg.RegisterDefaults(reg)
				capture := &traceTeachingCaptureLLM{stop: errors.New("captured capability subject teaching")}
				analyzer := NewAnalyzerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
				_, err := analyzer.Execute(ctx, traceTeachingSkill(t, "analysis-skill"))
				if capture.calls != 1 || !errors.Is(err, capture.stop) {
					t.Fatalf("expected one actual initial request: calls=%d err=%v", capture.calls, err)
				}
				var prompt strings.Builder
				for _, message := range capture.messages {
					if message.Role == "system" || message.Role == "user" {
						prompt.WriteString(message.Content)
						prompt.WriteByte('\n')
					}
				}
				text := prompt.String()
				for _, want := range []string{
					"host program's available tools/capabilities from implementation inside the target repository",
					"classify directly; later exploration reads current tool documentation",
					"needs neither a trace/log attachment nor a target repository implementing those tools",
					"An empty or unrelated target repository cannot prove the host lacks a capability",
					"actual capture behavior still requires runtime data",
					"target-repository registration/implementation still require source evidence",
					"Keep these subjects separate in mixed requests",
					"do not infer current_source_mode=exclude from catalog discovery or missing attachments",
					"when their subject is target-repository source registration",
					"a target-repository registry/catalog/binding/default-registration member-set answer",
					"non-evidentiary classification context and may be shown to downstream agents",
					"Do not present unverified findings or provisional answers as established facts",
					"Structured fields remain the authority for deterministic planning",
					"current_source_mode=exclude only when the current request explicitly forbids",
					"current emit_analysis tool schema is the authority for JSON field names",
				} {
					if !strings.Contains(text, want) {
						t.Errorf("actual analyzer request missing subject/evidence boundary %q", want)
					}
				}
				if strings.Contains(text, "This text is captured but does not drive any agent") {
					t.Error("rationale propagation is still incorrectly denied")
				}
				if !strings.Contains(text, request) {
					t.Error("actual current request was lost")
				}
				// This fixture has no graph provider, so repo_map is unavailable;
				// the real schema assembly must preserve the other three tools.
				allowed := map[string]bool{"emit_analysis": true, "grep": true, "list_files": true}
				if len(capture.tools) != len(allowed) {
					t.Errorf("classification tool count changed: got %d, want %d", len(capture.tools), len(allowed))
				}
				for _, schema := range capture.tools {
					if !allowed[schema.Name] || !json.Valid(schema.Parameters) {
						t.Errorf("unexpected or invalid classification schema: %s", schema.Name)
					}
					delete(allowed, schema.Name)
				}
				if len(allowed) != 0 {
					t.Errorf("classification rights removed: %v", allowed)
				}
				if ctx.AnalysisIR != nil || ctx.Mutable.RequestModel() != nil || ctx.Mutable.TraceQueryRuntimeObservationCount() != 0 {
					t.Error("prompt teaching preclassified the request, closed the source lane, or invented runtime evidence")
				}
			})
		}
	}
}
