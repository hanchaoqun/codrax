package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1633WriteFrequencyCapture(t *testing.T, root, name string, pid, frequency int) string {
	t.Helper()
	path := filepath.Join(root, name, "same.systrace")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`idle-0 (0) [007] .... 10.000000: cpu_frequency: state=%d cpu_id=7
idle-0 (0) [007] .... 10.001000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=%d next_prio=120
worker-%d (%d) [007] .... 10.002000: sched_switch: prev_comm=worker prev_pid=%d prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`, frequency, pid, pid, pid, pid)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func b1633ExecuteFrequency(t *testing.T, root, path string, pid, frequency int) types.ToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]any{"source": "path", "path": path,
		"view": "window_stats", "pid": pid, "time_start": 10.0, "time_end": 10.003})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: root, WorkDir: root}, params)
	if err != nil || !result.Success {
		t.Fatalf("real query failed: %v / %s", err, result.Summary)
	}
	found := false
	for _, record := range result.Observations {
		if record.Predicate == "running_time" && record.Subject == fmt.Sprintf("worker-%d", pid) && record.Unit == "ms" && len(record.RichNotes) > 0 {
			if record.Value != "1.000" || !strings.Contains(strings.Join(record.RichNotes, ";"), fmt.Sprintf("freq=%d", frequency)) || record.SourceRef.Path == "" || record.SourceRef.PayloadRef == "" || record.SourceRef.QueryScopeID == "" {
				t.Fatalf("producer premise failed before appendix check: %+v", record)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("actual query did not produce the expected frequency bucket")
	}
	return result
}

// Both inputs are real queries with correct original values. The appendix must
// not flatten their same-CPU facts into a source-less, thread-less inventory.
func TestB1633ActualQueriesKeepFrequencySourceInPublishedAppendix(t *testing.T) {
	root := t.TempDir()
	for _, sameTarget := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_target=%t", sameTarget), func(t *testing.T) {
			pidB := 52
			if sameTarget {
				pidB = 41
			}
			pathA := b1633WriteFrequencyCapture(t, root, fmt.Sprintf("a-%t", sameTarget), 41, 900000)
			pathB := b1633WriteFrequencyCapture(t, root, fmt.Sprintf("b-%t", sameTarget), pidB, 1500000)
			a := b1633ExecuteFrequency(t, root, pathA, 41, 900000)
			b := b1633ExecuteFrequency(t, root, pathB, pidB, 1500000)
			for _, lang := range []string{"zh", "en"} {
				for _, reverse := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/reverse=%t", lang, reverse), func(t *testing.T) {
						results := []types.ToolResult{a, b}
						if reverse {
							results[0], results[1] = results[1], results[0]
						}
						before, _ := json.Marshal(results)
						mut := types.NewMutableState("Compare the CPU observations.")
						for _, result := range results {
							mut.AppendDispatchToolResult(result)
						}
						doc := psgProseDoc("CPU7 frequency 900MHz: compare the supplied observations; this is model-owned prose.")
						mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
						docBefore, _ := json.Marshal(mut.AnswerDocumentV2())
						bus := &types.BusContext{RepoRoot: root, WorkDir: root, Mutable: mut, Language: lang}
						o := &Orchestrator{busCtx: bus}
						modelText := render.RenderAnswerDocument(doc, lang)
						out := &agent.StageOutput{FinalAnswer: modelText}
						o.attachSystemCrossCheckAppendix(out, "", nil)
						attachments := mut.AnswerDisplayAttachments()
						if len(attachments) != 1 {
							t.Fatalf("expected one published appendix: %+v", attachments)
						}
						body := attachments[0].Body
						for _, want := range []struct{ path, subject, frequency, other string }{
							{pathA, "worker-41", "900MHz", "1500MHz"},
							{pathB, fmt.Sprintf("worker-%d", pidB), "1500MHz", "900MHz"},
						} {
							matched := false
							for _, line := range strings.Split(body, "\n") {
								if strings.Contains(line, want.path) && strings.Contains(line, want.subject) && strings.Contains(line, want.frequency) && !strings.Contains(line, want.other) {
									matched = true
								}
							}
							if !matched {
								t.Errorf("actual published fact lost its own source/thread/value: %+v\n%s", want, body)
							}
						}
						after, _ := json.Marshal(results)
						docAfter, _ := json.Marshal(mut.AnswerDocumentV2())
						if string(before) != string(after) || string(docBefore) != string(docAfter) || !strings.Contains(out.FinalAnswer, modelText) {
							t.Fatal("appendix changed producer facts or model-owned answer")
						}
					})
				}
			}
		})
	}
}
