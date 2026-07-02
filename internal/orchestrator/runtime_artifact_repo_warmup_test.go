package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRun_AttachedRuntimeArtifactDefersSingleRepoGraphWarmup(t *testing.T) {
	repo := t.TempDir()
	if err := touchTestFile(repo, "main.go"); err != nil {
		t.Fatal(err)
	}

	var builds int
	topo := singleRepoWarmupTopology(repo)
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build: func(root, query string) (*rmtypes.Graph, error) {
			builds++
			return &rmtypes.Graph{
				Root:      root,
				Files:     []*rmtypes.FileInfo{},
				FileIndex: map[string]*rmtypes.FileInfo{},
				Metadata:  rmtypes.Metadata{FileCount: 0},
			}, nil
		},
		Cap: 1,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if ctx.AttachedHitrace == "" {
				t.Fatal("attached trace did not reach analyzer context")
			}
			ir := dagIR(types.AnswerContract{Language: "en"})
			return &agent.StageOutput{AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "runtime trace observation answer"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetMaxSteps(10)
	o.SetAttachedHitrace("sched_switch prev=app next=worker")
	o.SetTurnRouteHint(types.TurnRouteHint{
		Route:           "repo",
		Source:          "artifact",
		Operation:       "investigate",
		NeedsRepoAccess: true,
		Confidence:      0.9,
	})
	o.SetMultiGraphProvider(func() any { return mg })
	o.SetMultiRepoSnapshotProvider(func() ([]types.SubRepoSnapshot, []string) {
		return []types.SubRepoSnapshot{{
			Slug:         "root",
			RootAbs:      repo,
			RootRel:      ".",
			PrimaryLangs: []string{"go"},
			FileCount:    1,
		}}, nil
	})

	busCtx, err := o.Run("analyze attached trace", repo, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if builds != 0 {
		t.Fatalf("attached runtime artifact should defer run-entry graph warmup, build calls=%d", builds)
	}
	if busCtx == nil || busCtx.Mutable == nil || !strings.Contains(busCtx.Mutable.Result(), "runtime trace") {
		t.Fatalf("run did not complete through normal read path, result=%q", busCtx.Mutable.Result())
	}
}

func TestRun_RequestRuntimeArtifactPathDefersSingleRepoGraphWarmup(t *testing.T) {
	repo := t.TempDir()
	if err := touchTestFile(repo, "main.go"); err != nil {
		t.Fatal(err)
	}
	tracePath := filepath.Join(repo, "frame.systrace")
	if err := os.WriteFile(tracePath, []byte("sched_switch: prev_comm=app next_comm=worker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var builds int
	topo := singleRepoWarmupTopology(repo)
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build: func(root, query string) (*rmtypes.Graph, error) {
			builds++
			return &rmtypes.Graph{
				Root:      root,
				Files:     []*rmtypes.FileInfo{},
				FileIndex: map[string]*rmtypes.FileInfo{},
				Metadata:  rmtypes.Metadata{FileCount: 0},
			}, nil
		},
		Cap: 1,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if !ctx.RuntimeArtifactPreflight.SourceNavigationOptionalForAnalyze() {
				t.Fatalf("request trace path did not reach analyzer preflight: %+v", ctx.RuntimeArtifactPreflight)
			}
			ir := dagIR(types.AnswerContract{Language: "en"})
			return &agent.StageOutput{AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "runtime trace path answer"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetMaxSteps(10)
	o.SetMultiGraphProvider(func() any { return mg })
	o.SetMultiRepoSnapshotProvider(func() ([]types.SubRepoSnapshot, []string) {
		return []types.SubRepoSnapshot{{
			Slug:         "root",
			RootAbs:      repo,
			RootRel:      ".",
			PrimaryLangs: []string{"go"},
			FileCount:    1,
		}}, nil
	})

	busCtx, err := o.Run("analyze frame.systrace", repo, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if builds != 0 {
		t.Fatalf("request runtime artifact path should defer run-entry graph warmup, build calls=%d", builds)
	}
	if busCtx == nil || busCtx.Mutable == nil || !strings.Contains(busCtx.Mutable.Result(), "runtime trace path") {
		t.Fatalf("run did not complete through normal read path, result=%q", busCtx.Mutable.Result())
	}
}

func TestRuntimeArtifactPreflightProfileForRun_CompositeFtraceNamedInChineseRequest(t *testing.T) {
	repo := t.TempDir()
	traceRel := "record_trace_20260605224432@3279-299954687.sys.ftrace"
	tracePath := filepath.Join(repo, traceRel)
	if err := os.WriteFile(tracePath, []byte("sched_switch: prev_comm=UI prev_pid=19592 prev_state=S ==> next_comm=waker next_pid=3942\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	profile := runtimeArtifactPreflightProfileForRun(
		"分析东湖Trace:"+traceRel+"里面这一帧 Choreographer#doFrame 94410，不分析代码",
		repo,
		"",
		"",
	)
	if !profile.SourceNavigationOptionalForAnalyze() || !profile.HasTraceArtifact() {
		t.Fatalf("composite .sys.ftrace path should become typed trace preflight, got %+v", profile)
	}
	if len(profile.Artifacts) != 1 {
		t.Fatalf("preflight artifact count=%d, want 1: %+v", len(profile.Artifacts), profile.Artifacts)
	}
	got := profile.Artifacts[0]
	if got.Kind != "trace" || got.Source != traceRel || got.Carrier != "request_path" {
		t.Fatalf("preflight artifact drifted: %+v", got)
	}
	if got.Bytes <= 0 {
		t.Fatalf("preflight should preserve artifact size, got %+v", got)
	}

	// EXACT customer shape (trace_repl.log 2026-07-02): NO whitespace between
	// the glued CJK tail and the following ASCII term. The pure-CJK trailing
	// carve never reaches the extension here; preflight must still resolve
	// the artifact via the extension→CJK boundary carve.
	noSpace := runtimeArtifactPreflightProfileForRun(
		"分析东湖Trace:"+traceRel+"里面这一帧Choreographer#doFrame 94410，不分析代码",
		repo,
		"",
		"",
	)
	if !noSpace.SourceNavigationOptionalForAnalyze() || !noSpace.HasTraceArtifact() {
		t.Fatalf("no-space composite .sys.ftrace path should become typed trace preflight, got %+v", noSpace)
	}
	if len(noSpace.Artifacts) != 1 || noSpace.Artifacts[0].Source != traceRel || noSpace.Artifacts[0].Bytes <= 0 {
		t.Fatalf("no-space preflight artifact drifted: %+v", noSpace.Artifacts)
	}
	// The customer checkout holds nothing but the trace: the deterministic
	// census must recognize the runtime-artifact-only repository shape so
	// current-source floors know they are structurally unsatisfiable.
	if !noSpace.ZeroCurrentSourceRepo() {
		t.Fatalf("artifact-only repo should census as zero-current-source, got %+v", noSpace.RepoSourceCensus)
	}
}

func TestRepoSourceCensusForRun(t *testing.T) {
	t.Run("artifact_only", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.WriteFile(filepath.Join(repo, "a.sys.ftrace"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "b.log"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Hidden VCS/tool state must not count as current source.
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, ".git", "config"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		census := repoSourceCensusForRun(repo)
		if !census.ZeroCurrentSourceRepo() || census.ArtifactFiles != 2 || census.SourceFiles != 0 {
			t.Fatalf("artifact-only census drifted: %+v", census)
		}
	})
	t.Run("source_present", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.WriteFile(filepath.Join(repo, "a.sys.ftrace"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "src", "main.go"), []byte("package main"), 0o644); err != nil {
			t.Fatal(err)
		}
		census := repoSourceCensusForRun(repo)
		if census.ZeroCurrentSourceRepo() || census.SourceFiles == 0 {
			t.Fatalf("source repo must not census as zero-current-source: %+v", census)
		}
	})
	t.Run("empty_repo_inert", func(t *testing.T) {
		census := repoSourceCensusForRun(t.TempDir())
		if census.ZeroCurrentSourceRepo() {
			t.Fatalf("empty repo has no artifact either; census must stay inert: %+v", census)
		}
	})
	t.Run("missing_root_inert", func(t *testing.T) {
		census := repoSourceCensusForRun(filepath.Join(t.TempDir(), "does-not-exist"))
		if census.Completed || census.ZeroCurrentSourceRepo() {
			t.Fatalf("walk error must leave the census inert: %+v", census)
		}
	})
}

func TestRun_SourceTurnStillWarmsSingleRepoGraph(t *testing.T) {
	repo := t.TempDir()
	if err := touchTestFile(repo, "main.go"); err != nil {
		t.Fatal(err)
	}

	var builds int
	topo := singleRepoWarmupTopology(repo)
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build: func(root, query string) (*rmtypes.Graph, error) {
			builds++
			return &rmtypes.Graph{
				Root:      root,
				Files:     []*rmtypes.FileInfo{},
				FileIndex: map[string]*rmtypes.FileInfo{},
				Metadata:  rmtypes.Metadata{FileCount: 0},
			}, nil
		},
		Cap: 1,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{Error: "stop after entry warmup"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetMaxSteps(3)
	o.SetTurnRouteHint(types.TurnRouteHint{
		Route:           "repo",
		Source:          "repo",
		Operation:       "investigate",
		NeedsRepoAccess: true,
		Confidence:      0.9,
	})
	o.SetMultiGraphProvider(func() any { return mg })
	o.SetMultiRepoSnapshotProvider(func() ([]types.SubRepoSnapshot, []string) {
		return []types.SubRepoSnapshot{{
			Slug:         "root",
			RootAbs:      repo,
			RootRel:      ".",
			PrimaryLangs: []string{"go"},
			FileCount:    1,
		}}, nil
	})

	_, _ = o.Run("explain current source", repo, "main")
	if builds == 0 {
		t.Fatal("ordinary source turn should keep run-entry graph warmup")
	}
}

func singleRepoWarmupTopology(repo string) *topology.RepoTopology {
	return &topology.RepoTopology{
		ParentRoot: repo,
		ParentSlug: "root",
		Repos: []topology.SubRepo{{
			Slug:         "root",
			RootAbs:      repo,
			RootRel:      ".",
			PrimaryLangs: []string{"go"},
			FileCount:    1,
		}},
	}
}

func touchTestFile(root, rel string) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	return os.WriteFile(path, []byte("package main\n"), 0o644)
}
