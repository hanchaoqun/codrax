package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreparedLogOrchestratorRunAndPlainReplacement(t *testing.T) {
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "inline", Data: []byte(strings.Repeat("header\n", 100) + "tail-target\n")}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var seen []*loginput.Catalog
	fns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentLogTriager: func(ac *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			seen = append(seen, ac.AttachedLogCatalog)
			if ac.AttachedLogCatalog != nil {
				bus := types.ToolBusContext(ac, types.AgentLogTriager).ShallowClone()
				result, err := bus.AttachedLogCatalog.Query(context.Background(), loginput.Query{Contains: "tail-target"})
				if err != nil || result.Matched != 1 {
					t.Fatalf("catalog lost in real dispatch: %+v %v", result, err)
				}
			}
			return &agent.StageOutput{StageReport: "triaged"}, nil
		},
		types.AgentAnalyzer: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{Error: "expected classifier stop"}, nil
		},
	}
	ar, sr, sar := buildRegistries(fns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package sample\n"), 0600); err != nil {
		t.Fatal(err)
	}
	o.SetAttachedLog(catalog.Preview(64))
	o.SetAttachedLogCatalog(catalog)
	_, _ = o.Run("inspect runtime logs", repo, "main")
	if len(seen) != 1 || seen[0] != catalog {
		t.Fatalf("Run dropped catalog: %v", seen)
	}
	o.SetAttachedLog("replacement\n")
	if o.AttachedLogCatalog() != nil {
		t.Fatal("plain setter inherited old authority")
	}
	_, _ = o.Run("inspect runtime logs", repo, "main")
	if len(seen) != 2 || seen[1] != nil {
		t.Fatal("second Run reused stale catalog")
	}
	o.SetAttachedLog("")
	if o.AttachedLogCatalog() != nil || o.AttachedLog() != "" {
		t.Fatal("clear retained source")
	}
}
