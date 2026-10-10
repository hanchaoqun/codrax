package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmptyRepoRunRuntimeAttachmentPresence(t *testing.T) {
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "runtime.log", Data: []byte("runtime target\n")}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	emptyCatalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "empty.log", Data: []byte{}}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tracePath := filepath.Join(t.TempDir(), "capture.systrace")
	if err := os.WriteFile(tracePath, []byte("# trace text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: tracePath, PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		attach  func(*Orchestrator)
		present bool
		pre     types.AgentName
	}{
		{"log_catalog", func(o *Orchestrator) { o.SetAttachedLog(catalog.Preview(64)); o.SetAttachedLogCatalog(catalog) }, true, types.AgentLogTriager},
		{"empty_log_catalog", func(o *Orchestrator) { o.SetAttachedLogCatalog(emptyCatalog) }, true, ""},
		{"plain_log", func(o *Orchestrator) { o.SetAttachedLog("legacy runtime log\n") }, true, types.AgentLogTriager},
		{"trace_material", func(o *Orchestrator) { o.SetAttachedHitrace(material.Preview()); o.SetAttachedTraceMaterial(material) }, true, types.AgentPerfTriager},
		{"plain_trace", func(o *Orchestrator) { o.SetAttachedHitrace("# inline trace\n") }, true, types.AgentPerfTriager},
		{"none", func(*Orchestrator) {}, false, ""},
		{"flavor_hint_only", func(o *Orchestrator) { o.SetAttachedHitraceSource("hitrace") }, false, ""},
		{"cleared", func(o *Orchestrator) {
			o.SetAttachedLogCatalog(catalog)
			o.SetAttachedTraceMaterial(material)
			o.SetAttachedLog("")
			o.SetAttachedHitrace("")
		}, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := map[types.AgentName]int{}
			fns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){}
			for _, name := range []types.AgentName{types.AgentLogTriager, types.AgentPerfTriager, types.AgentAnalyzer} {
				name := name
				fns[name] = func(ac *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					calls[name]++
					if tc.name == "log_catalog" && ac.AttachedLogCatalog != catalog {
						t.Fatal("log material did not reach the dispatched agent")
					}
					if tc.name == "empty_log_catalog" && ac.AttachedLogCatalog != emptyCatalog {
						t.Fatal("zero-record log material was treated as no input")
					}
					if tc.name == "trace_material" && ac.AttachedTraceMaterial != material {
						t.Fatal("trace material did not reach the dispatched agent")
					}
					if name == types.AgentAnalyzer {
						return &agent.StageOutput{Error: "expected classifier stop"}, nil
					}
					return &agent.StageOutput{StageReport: "triaged"}, nil
				}
			}
			ar, sr, sar := buildRegistries(fns)
			sr.Register(&skill.Config{Name: "perf-triage-skill", Goal: "Inspect the runtime capture"})
			o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
			tc.attach(o)
			repo := t.TempDir()
			bus, runErr := o.Run("inspect the attached runtime capture", repo, "main")
			if tc.present {
				if calls[types.AgentAnalyzer] == 0 || tc.pre != "" && calls[tc.pre] != 1 {
					t.Fatalf("runtime-only request short-circuited: calls=%v error=%v", calls, runErr)
				}
			} else if runErr != nil || len(calls) != 0 || !bus.TaskState.IsTerminal || bus.Mutable.Result() != emptyRepoReadIntro(bus.Language, repo) {
				t.Fatalf("no-material empty repo behavior changed: calls=%v error=%v result=%q", calls, runErr, bus.Mutable.Result())
			}
		})
	}
}
