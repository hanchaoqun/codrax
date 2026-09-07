package context

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1614RepositoryScopeSection(pc *types.PromptContext) string {
	for _, section := range pc.UserSections {
		if section.Title == SectionMultiRepoActiveSet {
			return section.Content
		}
	}
	return ""
}

func TestB1614WriteScopedRepositoryReachesAgentPrompts(t *testing.T) {
	for _, stage := range []struct {
		agent types.AgentName
		stage types.PipelineStage
	}{
		{types.AgentWriteAnalyzer, types.StageWriteAnalyze},
		{types.AgentPlanner, types.StagePlan},
		{types.AgentExplorer, types.StageExplore},
	} {
		for _, repoRoot := range []string{"/workspace/bindings-py", "/isolated/write-123"} {
			t.Run(string(stage.stage)+"/"+repoRoot, func(t *testing.T) {
				const request = "在 bindings-py 修复输入转换，保留原始问题中的路径。"
				snapshot := types.SubRepoSnapshot{RootRel: "bindings-py", RootAbs: "/workspace/bindings-py", PrimaryLangs: []string{"Python"}}
				beforeSnapshot := snapshot
				beforeSnapshot.PrimaryLangs = append([]string(nil), snapshot.PrimaryLangs...)
				bus := &types.BusContext{
					Mode: types.ModeApply, RepoRoot: repoRoot, MainRepoRoot: snapshot.RootAbs,
					ActiveSubRepo: &snapshot, SubRepos: []types.SubRepoSnapshot{snapshot, {RootRel: "client-ts", RootAbs: "/workspace/client-ts"}},
					Mutable: types.NewMutableState(request), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RawRequest: request}},
				}
				ac := BuildAgentContext(bus, stage.agent, stage.stage)
				pc := BuildPromptContext(ac, &skill.Config{Name: "scoped-write"})
				got := b1614RepositoryScopeSection(pc)
				for _, want := range []string{"already inside the selected repository", `"bindings-py"`, "relative to the current repository root", "isolated worktree", "do not prepend", "proposed change paths"} {
					if !strings.Contains(got, want) {
						t.Fatalf("typed child-scope prompt missing %q: %s", want, got)
					}
				}
				for _, wrong := range []string{"prefer the full sub-repo-prefixed form", "resolve it under each active sub-repo", "client-ts", "out of the active set"} {
					if strings.Contains(got, wrong) {
						t.Fatalf("child-scoped write inherited parent read-multi guidance %q: %s", wrong, got)
					}
				}
				if ac.RepoRoot != repoRoot || ac.MainRepoRoot != snapshot.RootAbs || bus.RepoRoot != repoRoot ||
					bus.MainRepoRoot != snapshot.RootAbs || bus.Mutable.Objective() != request || bus.AnalysisIR.RequestModel.RawRequest != request ||
					!reflect.DeepEqual(snapshot, beforeSnapshot) || ac.MultiGraph != nil || len(ac.PendingSubRepos) != 0 {
					t.Fatalf("prompt construction changed scope, raw request, or source snapshot: bus=%+v snapshot=%+v", bus, snapshot)
				}
			})
		}
	}
}

func TestB1614ChildScopeDoesNotReplaceReadOrUnscopedPrompt(t *testing.T) {
	for _, tt := range []struct {
		name       string
		mode       types.PipelineMode
		active     bool
		multi      bool
		wantParent bool
	}{
		{name: "read multi", mode: types.ModeRead, multi: true, wantParent: true},
		{name: "read multi with selected snapshot", mode: types.ModeRead, active: true, multi: true, wantParent: true},
		{name: "write missing selected snapshot", mode: types.ModeApply, multi: true, wantParent: true},
		{name: "write single repository", mode: types.ModeApply},
		{name: "read single repository", mode: types.ModeRead},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := types.SubRepoSnapshot{RootRel: "bindings-py", RootAbs: "/workspace/bindings-py"}
			bus := &types.BusContext{Mode: tt.mode, RepoRoot: "/workspace", Mutable: types.NewMutableState("already in bindings-py; use child-relative paths"), SubRepos: []types.SubRepoSnapshot{snapshot}}
			if tt.multi {
				bus.SubRepos = append(bus.SubRepos, types.SubRepoSnapshot{RootRel: "client-ts", RootAbs: "/workspace/client-ts"})
			}
			if tt.active {
				bus.ActiveSubRepo = &snapshot
			}
			ac := BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
			pc := BuildPromptContext(ac, &skill.Config{Name: "explore"})
			got := b1614RepositoryScopeSection(pc)
			if got != formatMultiRepoActiveSetAdvisory(ac) {
				t.Fatalf("non-child context must retain its existing multi-repo advisory: %s", got)
			}
			if strings.Contains(got, "already inside the selected repository") || strings.Contains(got, "do not prepend") {
				t.Fatalf("mode/snapshot cannot be inferred from request prose: %s", got)
			}
			if tt.wantParent != strings.Contains(got, "prefer the full sub-repo-prefixed form") {
				t.Fatalf("parent read-multi path convention changed: %s", got)
			}
		})
	}
}

func TestB1614ChildScopeUsesSelectedSnapshotWithoutTopologyInventory(t *testing.T) {
	for _, mode := range []types.PipelineMode{types.ModePlan, types.ModeApply, types.ModeVerify} {
		t.Run(string(mode), func(t *testing.T) {
			const path = " binding space\n```scope` "
			snapshot := types.SubRepoSnapshot{RootRel: path, RootAbs: "/workspace/child"}
			bus := &types.BusContext{Mode: mode, RepoRoot: snapshot.RootAbs, MainRepoRoot: snapshot.RootAbs, ActiveSubRepo: &snapshot, Mutable: types.NewMutableState("inspect local files")}
			pc := BuildPromptContext(BuildAgentContext(bus, types.AgentPlanner, types.StagePlan), &skill.Config{Name: "plan"})
			got := b1614RepositoryScopeSection(pc)
			quotedPath := strings.ReplaceAll(fmt.Sprintf("%q", path), "`", `\u0060`)
			if !strings.Contains(got, quotedPath) || strings.Contains(got, "\n```scope") {
				t.Fatalf("snapshot path must be preserved and quoted without injecting prompt structure: %q", got)
			}
			if snapshot.RootRel != path {
				t.Fatal("display quoting changed source identity")
			}
		})
	}
}
