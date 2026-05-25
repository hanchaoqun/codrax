package repomap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestResolveRepoMapRootScopedRejectsParentEscape(t *testing.T) {
	repo := t.TempDir()

	if _, err := resolveRepoMapRootScoped("..", repo, repo); err == nil {
		t.Fatal("expected parent escape to be rejected")
	}
	if _, err := resolveRepoMapRootScoped(filepath.Join("src", "..", ".."), repo, repo); err == nil {
		t.Fatal("expected normalized parent escape to be rejected")
	}
}

func TestResolveRepoMapRootScopedRejectsAbsoluteOutside(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveRepoMapRootScoped(outside, repo, repo); err == nil {
		t.Fatal("expected absolute path outside the repo to be rejected")
	}
}

func TestResolveRepoMapRootScopedRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows builders")
	}
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "escape")); err != nil {
		t.Skipf("cannot create symlink on this filesystem: %v", err)
	}

	if _, err := resolveRepoMapRootScoped("escape", repo, repo); err == nil {
		t.Fatal("expected symlink escaping the repo to be rejected")
	}
}

func TestRepoMapSourceInventoryViewModelDrivenQuery(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("source inventory")
	graph := &Graph{
		Root:       repo,
		Files:      []*FileInfo{},
		FileIndex:  map[string]*FileInfo{},
		SymbolDefs: map[string][]*Symbol{},
	}
	addFile := func(fi *FileInfo) {
		graph.Files = append(graph.Files, fi)
		graph.FileIndex[fi.RelPath] = fi
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			if sym.File == "" {
				sym.File = fi.RelPath
			}
			graph.SymbolDefs[sym.Name] = append(graph.SymbolDefs[sym.Name], sym)
		}
	}
	addFile(&FileInfo{
		RelPath:  "src/alpha/a.py",
		Language: LangPython,
		Package:  "alpha",
		Symbols: []Symbol{{
			Name:     "run_alpha",
			Kind:     "function",
			File:     "src/alpha/a.py",
			Line:     7,
			Exported: true,
		}},
	})
	addFile(&FileInfo{
		RelPath:  "src/beta/B.java",
		Language: LangJava,
		Package:  "com.example.beta",
		Symbols: []Symbol{{
			Name:     "RunBeta",
			Kind:     "function",
			File:     "src/beta/B.java",
			Line:     11,
			Exported: true,
		}},
	})
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": ".",
		"view": "source_inventory",
		"scope": "src",
		"roles": ["function", "package"],
		"include_attributes": true,
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("repo_map source_inventory returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo_map source_inventory should succeed: %+v", res)
	}
	for _, want := range []string{
		"Repo Lens: Source Inventory",
		"count=2",
		"`run_alpha` @ src/alpha/a.py:7",
		"package/directory/module scope",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("source_inventory summary missing %q:\n%s", want, res.Summary)
		}
	}
	obs := mut.SourceInventoryObservation()
	if !obs.IsActive() || len(obs.Sets) != 2 || obs.Sets[0].Role != types.AnswerCandidateRoleFunction {
		t.Fatalf("model-driven source inventory observation not stored in role order: %+v", obs)
	}
}

func TestRepoMapSourceInventoryViewWorksBeforeAnalysisIREmission(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("source inventory before analysis")
	graph := BuildGraph(repo, []*FileInfo{{
		RelPath:  "src/alpha/a.py",
		Language: LangPython,
		Package:  "alpha",
		Symbols: []Symbol{{
			Name:     "run_alpha",
			Kind:     "function",
			File:     "src/alpha/a.py",
			Line:     7,
			Exported: true,
		}},
	}, {
		RelPath:  "src/beta/B.java",
		Language: LangJava,
		Package:  "com.example.beta",
		Symbols: []Symbol{{
			Name:     "RunBeta",
			Kind:     "function",
			File:     "src/beta/B.java",
			Line:     11,
			Exported: true,
		}},
	}})
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{RepoRoot: repo, Mutable: mut}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": ".",
		"view": "source_inventory",
		"scopes": ["src/alpha", "src/beta"],
		"roles": ["function"],
		"include_attributes": true,
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("repo_map source_inventory returned error before analysis IR: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo_map source_inventory should succeed before analysis IR: %+v", res)
	}
	for _, want := range []string{
		"Repo Lens: Source Inventory",
		"`run_alpha` @ src/alpha/a.py:7",
		"`RunBeta` @ src/beta/B.java:11",
		"repo_lens:roles",
		"repo_lens:scopes",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("pre-analysis source_inventory summary missing %q:\n%s", want, res.Summary)
		}
	}
	obs := mut.SourceInventoryObservation()
	if !obs.IsActive() || len(obs.Sets) != 1 || obs.Sets[0].Role != types.AnswerCandidateRoleFunction || obs.Sets[0].Count != 2 {
		t.Fatalf("pre-analysis model-driven observation not stored: %+v", obs)
	}
}

func TestRepoMapSourceInventoryViewAttributeRolesAttachFileLocalCandidates(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("source inventory attributes")
	graph := BuildGraph(repo, []*FileInfo{{
		RelPath:  "src/alpha/a.py",
		Language: LangPython,
		Package:  "alpha",
		Symbols: []Symbol{{
			Name:     "run_alpha",
			Kind:     "function",
			File:     "src/alpha/a.py",
			Line:     7,
			Exported: true,
		}},
	}, {
		RelPath:  "src/beta/b.py",
		Language: LangPython,
		Package:  "beta",
		Symbols: []Symbol{{
			Name:     "run_beta",
			Kind:     "function",
			File:     "src/beta/b.py",
			Line:     9,
			Exported: true,
		}},
	}})
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": ".",
		"view": "source_inventory",
		"scope": "src/alpha",
		"roles": ["file"],
		"attribute_roles": ["function"],
		"include_attributes": true,
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("repo_map source_inventory returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo_map source_inventory should succeed: %+v", res)
	}
	for _, want := range []string{
		"attribute_roles: `function`",
		"`src/alpha/a.py`",
		"attributes: function `run_alpha` @ src/alpha/a.py:7",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("source_inventory attribute view missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "run_beta") {
		t.Fatalf("source_inventory attribute view leaked sibling symbols:\n%s", res.Summary)
	}
	obs := mut.SourceInventoryObservation()
	if !obs.IsActive() || len(obs.Sets) != 1 || len(obs.Sets[0].Members) != 1 {
		t.Fatalf("unexpected observation: %+v", obs)
	}
	if attrs := obs.Sets[0].Members[0].Attributes; len(attrs) != 1 || attrs[0].Name != "run_alpha" {
		t.Fatalf("file-local attribute not preserved: %+v", attrs)
	}
}

func TestRepoMapSourceInventoryViewGroupsBroadSymbolRolesByScope(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("source inventory grouped symbols")
	graph := BuildGraph(repo, []*FileInfo{{
		RelPath:  "src/alpha/a.py",
		Language: LangPython,
		Package:  "alpha",
		Symbols: []Symbol{{
			Name:     "run",
			Kind:     "function",
			File:     "src/alpha/a.py",
			Line:     7,
			Exported: true,
		}, {
			Name:     "helper",
			Kind:     "function",
			File:     "src/alpha/a.py",
			Line:     20,
			Exported: false,
		}},
	}, {
		RelPath:  "src/beta/b.py",
		Language: LangPython,
		Package:  "beta",
		Symbols: []Symbol{{
			Name:     "run",
			Kind:     "function",
			File:     "src/beta/b.py",
			Line:     9,
			Exported: true,
		}},
	}})
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": ".",
		"view": "source_inventory",
		"scopes": ["src/beta", "src/alpha"],
		"roles": ["function"],
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("repo_map source_inventory returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo_map source_inventory should succeed: %+v", res)
	}
	for _, want := range []string{
		"## Cascaded Repo Lens Guide (advisory)",
		"scope_groups=2 candidate_files=2 candidate_items=3 ambiguous_groups=1",
		"`src/beta` — files=1 candidates=1 roles=function:1 languages=python:1 — next `repo_map {\"path\": \".\", \"view\": \"source_inventory\", \"scope\": \"src/beta\"",
		"## Scope-grouped Candidate View (advisory)",
		"`src/beta` — candidate_count=1 roles=function:1 languages=python:1",
		"function `run` @ src/beta/b.py:9",
		"`src/alpha` — candidate_count=2 roles=function:2 languages=python:2",
		"function `run` @ src/alpha/a.py:7",
		"function `helper` @ src/alpha/a.py:20",
		"## Candidate File Samples to Verify (advisory)",
		"`src/beta` — file_count=1 candidate_count=1 — files: `src/beta/b.py`",
		"candidates: function `run`@9",
		"`src/alpha` — file_count=1 candidate_count=2 — files: `src/alpha/a.py`",
		"candidates: function `run`@7, function `helper`@20",
		"count=3 complete=true",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("grouped source_inventory missing %q:\n%s", want, res.Summary)
		}
	}
	if beta := strings.Index(res.Summary, "`src/beta`"); beta < 0 {
		t.Fatalf("grouped source_inventory missing beta group:\n%s", res.Summary)
	} else if alpha := strings.Index(res.Summary, "`src/alpha`"); alpha < 0 || alpha < beta {
		t.Fatalf("grouped source_inventory did not preserve model-provided scope order:\n%s", res.Summary)
	}
	cascade := strings.Index(res.Summary, "## Cascaded Repo Lens Guide (advisory)")
	grouped := strings.Index(res.Summary, "## Scope-grouped Candidate View (advisory)")
	files := strings.Index(res.Summary, "## Candidate File Samples to Verify (advisory)")
	if cascade < 0 || grouped < 0 || files < 0 {
		t.Fatalf("source_inventory missing progressive navigation sections:\n%s", res.Summary)
	}
	if !(cascade < grouped && grouped < files) {
		t.Fatalf("source_inventory should render cascade guide, then grouped rows, then file samples:\n%s", res.Summary)
	}
	obs := mut.SourceInventoryObservation()
	if !obs.IsActive() || len(obs.Sets) != 1 || obs.Sets[0].Count != 3 {
		t.Fatalf("source inventory should preserve duplicate names at distinct locations: %+v", obs)
	}
}

func TestRepoMapSourceInventoryViewGroupsBroadSingleScopeByChild(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("source inventory grouped child scopes")
	graph := BuildGraph(repo, []*FileInfo{{
		RelPath:  "src/alpha/a.ts",
		Language: LangTypeScript,
		Package:  "alpha",
		Symbols: []Symbol{{
			Name:     "AlphaController",
			Kind:     "class",
			File:     "src/alpha/a.ts",
			Line:     3,
			Exported: true,
		}, {
			Name:     "handleAlpha",
			Kind:     "function",
			File:     "src/alpha/a.ts",
			Line:     12,
			Exported: true,
		}},
	}, {
		RelPath:  "src/beta/b.ts",
		Language: LangTypeScript,
		Package:  "beta",
		Symbols: []Symbol{{
			Name:     "handleBeta",
			Kind:     "function",
			File:     "src/beta/b.ts",
			Line:     6,
			Exported: true,
		}},
	}})
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": ".",
		"view": "source_inventory",
		"scope": "src",
		"roles": ["function", "type"],
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("repo_map source_inventory returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo_map source_inventory should succeed: %+v", res)
	}
	for _, want := range []string{
		"`src/alpha` — candidate_count=2 roles=function:1,type:1 languages=typescript:2",
		"type `AlphaController` @ src/alpha/a.ts:3",
		"function `handleAlpha` @ src/alpha/a.ts:12",
		"`src/beta` — candidate_count=1 roles=function:1 languages=typescript:1",
		"function `handleBeta` @ src/beta/b.ts:6",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("single-scope grouped source_inventory missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestRepoMapSourceInventoryViewSameRepoCrossLanguage(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("source inventory same-repo xlang")
	graph := BuildGraph(repo, []*FileInfo{
		{
			RelPath:  "src/ui/Index.ets",
			Language: LangArkTS,
			Package:  "ui",
			Symbols: []Symbol{
				{Name: "Index", Kind: "component", File: "src/ui/Index.ets", Line: 6, Exported: true},
				{Name: "build", Kind: "ui-entry", File: "src/ui/Index.ets", Line: 24, Parent: "Index", Exported: true},
			},
		},
		{
			RelPath:  "src/proto/user.proto",
			Language: LangProto,
			Package:  "user.v1",
			Symbols: []Symbol{
				{Name: "UserService", Kind: "service", File: "src/proto/user.proto", Line: 8, Exported: true},
				{Name: "GetUser", Kind: "rpc", File: "src/proto/user.proto", Line: 11, Parent: "UserService", Exported: true},
			},
		},
		{
			RelPath:  "src/cj/main.cj",
			Language: LangCangjie,
			Package:  "cj",
			Symbols: []Symbol{{
				Name: "operator +", Kind: "operator", File: "src/cj/main.cj", Line: 5, Exported: true,
			}},
		},
	})
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": ".",
		"view": "source_inventory",
		"scope": "src",
		"roles": ["function", "type"],
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("repo_map source_inventory returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo_map source_inventory should succeed: %+v", res)
	}
	for _, want := range []string{
		"count=3",
		"`build`",
		"@ src/ui/Index.ets:24",
		"language=arkts",
		"`GetUser`",
		"@ src/proto/user.proto:11",
		"language=proto",
		"`operator +`",
		"@ src/cj/main.cj:5",
		"language=cangjie",
		"`Index`",
		"@ src/ui/Index.ets:6",
		"`UserService`",
		"@ src/proto/user.proto:8",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("same-repo cross-language source inventory missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestRepoMapSourceInventoryViewConfigFilesAreNavigationRows(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("source inventory config files")
	graph := BuildGraph(repo, []*FileInfo{
		{
			RelPath:     "package.json",
			IsSpecial:   true,
			SpecialType: "build_config",
		},
		{
			RelPath:     "src/AndroidManifest.xml",
			IsSpecial:   true,
			SpecialType: "build_config",
		},
		{
			RelPath:  "src/main.ts",
			Language: LangTypeScript,
			Package:  "app",
			Symbols: []Symbol{{
				Name: "main", Kind: "function", File: "src/main.ts", Line: 3, Exported: true,
			}},
		},
	})
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": ".",
		"view": "source_inventory",
		"scopes": [".", "src"],
		"roles": ["config_file", "file"],
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("repo_map source_inventory returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo_map source_inventory should succeed: %+v", res)
	}
	for _, want := range []string{
		"configuration/manifest file",
		"`package.json`",
		"`src/AndroidManifest.xml`",
		"special_type=build_config",
		"`src/main.ts`",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("config source_inventory missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "config_key") {
		t.Fatalf("config file inventory must not pretend config keys were extracted:\n%s", res.Summary)
	}
}

func TestRepoMapSourceInventoryViewGroupsConfigKeysAndRoutes(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("source inventory config key route groups")
	graph := BuildGraph(repo, []*FileInfo{
		{
			RelPath:     "config/app.yaml",
			Language:    "yaml",
			IsSpecial:   true,
			SpecialType: "app_config",
			Symbols: []Symbol{{
				Name:     "feature.enabled",
				Kind:     "config_key",
				File:     "config/app.yaml",
				Line:     4,
				Exported: true,
			}, {
				Name:     "feature.timeout",
				Kind:     "config_key",
				File:     "config/app.yaml",
				Line:     5,
				Exported: true,
			}},
		},
		{
			RelPath:  "src/routes.ts",
			Language: LangTypeScript,
			Package:  "routes",
			Symbols: []Symbol{{
				Name:     "GET /healthz",
				Kind:     "route",
				File:     "src/routes.ts",
				Line:     11,
				Exported: true,
			}},
		},
	})
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": ".",
		"view": "source_inventory",
		"scopes": ["config", "src"],
		"roles": ["config_key", "route"],
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("repo_map source_inventory returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo_map source_inventory should succeed: %+v", res)
	}
	for _, want := range []string{
		"## Cascaded Repo Lens Guide (advisory)",
		"scope_groups=2 candidate_files=2 candidate_items=3 ambiguous_groups=1",
		"`config` — candidate_count=2 roles=config_key:2 languages=yaml:2",
		"config_key `feature.enabled` @ config/app.yaml:4",
		"config_key `feature.timeout` @ config/app.yaml:5",
		"`src` — candidate_count=1 roles=route:1 languages=typescript:1",
		"route `GET /healthz` @ src/routes.ts:11",
		"## Candidate File Samples to Verify (advisory)",
		"`config` — file_count=1 candidate_count=2 — files: `config/app.yaml`",
		"candidates: config_key `feature.enabled`@4, config_key `feature.timeout`@5",
		"`src` — file_count=1 candidate_count=1 — files: `src/routes.ts`",
		"candidates: route `GET /healthz`@11",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("config/route grouped source_inventory missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestRepoMapSourceInventoryViewPathDefaultsToScope(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "src", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src", "beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "alpha", "alpha.go"), []byte(`package alpha

func RunAlpha() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "beta", "beta.go"), []byte(`package beta

func RunBeta() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  types.NewMutableState("source inventory path default scope"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				Confidence:        0.9,
			},
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
			AnalyzerHints: types.AnalyzerHints{
				MentionedEntities: []string{"src/alpha", "src/beta"},
			},
		}},
	}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": "src/alpha",
		"view": "source_inventory",
		"roles": ["function"],
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("repo_map source_inventory returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo_map source_inventory should succeed: %+v", res)
	}
	if !strings.Contains(res.Summary, "`RunAlpha`") || strings.Contains(res.Summary, "RunBeta") {
		t.Fatalf("path should default source_inventory scope to src/alpha without explicit scope:\n%s", res.Summary)
	}
	obs := ctx.Mutable.SourceInventoryObservation()
	if !obs.IsActive() || len(obs.Sets) != 1 || obs.Sets[0].Count != 1 {
		t.Fatalf("path-default scoped observation should have one alpha function: %+v", obs)
	}
}

func TestResolveRepoMapRootScopedAllowsChildren(t *testing.T) {
	repo := t.TempDir()

	resolved, err := resolveRepoMapRootScoped(filepath.Join("src", "pkg"), repo, repo)
	if err != nil {
		t.Fatalf("expected child path to be allowed: %v", err)
	}
	want := filepath.Join(repo, "src", "pkg")
	if resolved != want {
		t.Fatalf("resolved path mismatch: got %q want %q", resolved, want)
	}
}

func TestResolveRepoMapRootScopedHonorsActiveSubRepoScope(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "repo-a")
	if err := os.Mkdir(active, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "repo-b"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveRepoMapRootScoped(filepath.Join("repo-a", "..", "repo-b"), root, active); err == nil {
		t.Fatal("expected escape from active sub-repo to be rejected")
	}
	if _, err := resolveRepoMapRootScoped(filepath.Join("repo-a", "src"), root, active); err != nil {
		t.Fatalf("expected path under active sub-repo to be allowed: %v", err)
	}
}

func TestRepoMapExecuteRejectsParentEscapeBeforeScan(t *testing.T) {
	repo := t.TempDir()
	ctx := &types.BusContext{RepoRoot: repo}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":".."}`))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected repo_map to refuse parent escape, got success: %+v", res)
	}
	if !strings.Contains(res.Summary, "outside the current repository scope") {
		t.Fatalf("unexpected refusal summary: %q", res.Summary)
	}
}

func TestRepoMapExecuteRejectsMissingPathBeforeScan(t *testing.T) {
	repo := t.TempDir()
	ctx := &types.BusContext{RepoRoot: repo}
	var events []types.RepoMapScanEvent
	SetScanNotifier(func(ev types.RepoMapScanEvent) {
		events = append(events, ev)
	})
	defer SetScanNotifier(nil)

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":"does/not/exist"}`))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected repo_map to refuse missing path, got success: %+v", res)
	}
	if !strings.Contains(res.Summary, "was not found") || strings.Contains(res.Summary, "scan failed") {
		t.Fatalf("missing-path summary should be gentle and pre-scan, got %q", res.Summary)
	}
	if len(events) != 0 {
		t.Fatalf("missing path must be rejected before scan notifier events, got %+v", events)
	}
}

func TestRepoMapExecuteRejectsFilePathBeforeScan(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: repo}
	var events []types.RepoMapScanEvent
	SetScanNotifier(func(ev types.RepoMapScanEvent) {
		events = append(events, ev)
	})
	defer SetScanNotifier(nil)

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":"main.go"}`))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected repo_map to refuse file path, got success: %+v", res)
	}
	if !strings.Contains(res.Summary, "points to a file") || strings.Contains(res.Summary, "scan failed") {
		t.Fatalf("file-path summary should be gentle and pre-scan, got %q", res.Summary)
	}
	if len(events) != 0 {
		t.Fatalf("file path must be rejected before scan notifier events, got %+v", events)
	}
}

func TestRepoMapExecuteReusesMutableSearchGraph(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("repo map reuse")
	mut.SetSearchGraph(BuildGraph(repo, []*FileInfo{{
		RelPath:  "virtual.go",
		Language: "go",
		Size:     12,
	}}))
	ctx := &types.BusContext{RepoRoot: repo, Mutable: mut}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":".","view":"overview","query":"virtual"}`))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected repo_map to reuse the in-memory graph instead of scanning the empty repo: %+v", res)
	}
}

func TestRepoMapExecuteMultiRepoHonorsEachExplicitSubRepoPath(t *testing.T) {
	parent := t.TempDir()
	baseRoot := filepath.Join(parent, "frameworks", "base")
	resschedRoot := filepath.Join(parent, "hm_z", "foundation", "resourceschedule", "ressched")
	if err := os.MkdirAll(baseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resschedRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mg := newTestMultiGraph(t, parent, baseRoot, resschedRoot)
	ctx := &types.BusContext{RepoRoot: parent, MultiGraph: mg}

	base, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":"frameworks/base","view":"overview"}`))
	if err != nil {
		t.Fatalf("base repo_map returned error: %v", err)
	}
	if !base.Success {
		t.Fatalf("base repo_map failed: %+v", base)
	}
	ressched, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":"hm_z/foundation/resourceschedule/ressched","view":"overview"}`))
	if err != nil {
		t.Fatalf("ressched repo_map returned error: %v", err)
	}
	if !ressched.Success {
		t.Fatalf("ressched repo_map failed: %+v", ressched)
	}

	if !strings.Contains(base.Summary, "- java: 1 files") {
		t.Fatalf("base overview should describe the base graph, got:\n%s", base.Summary)
	}
	if strings.Contains(base.Summary, "- cpp: 1 files") {
		t.Fatalf("base overview leaked the ressched graph:\n%s", base.Summary)
	}
	if !strings.Contains(ressched.Summary, "- cpp: 1 files") {
		t.Fatalf("ressched overview should describe the ressched graph, got:\n%s", ressched.Summary)
	}
	if strings.Contains(ressched.Summary, "- java: 1 files") {
		t.Fatalf("ressched overview leaked the base graph:\n%s", ressched.Summary)
	}
}

func TestRepoMapSourceInventoryViewMultiRepoHonorsActiveSubRepoScope(t *testing.T) {
	parent := t.TempDir()
	baseRoot := filepath.Join(parent, "frameworks", "base")
	resschedRoot := filepath.Join(parent, "hm_z", "foundation", "resourceschedule", "ressched")
	if err := os.MkdirAll(baseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resschedRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mg := newSourceInventoryTestMultiGraph(t, parent, baseRoot, resschedRoot)
	mut := types.NewMutableState("source inventory multirepo")
	ctx := &types.BusContext{
		RepoRoot:   parent,
		MultiGraph: mg,
		Mutable:    mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}

	base, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": "frameworks/base",
		"view": "source_inventory",
		"scope": "core",
		"roles": ["function", "method", "type"],
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("base source_inventory returned error: %v", err)
	}
	if !base.Success {
		t.Fatalf("base source_inventory failed: %+v", base)
	}
	for _, want := range []string{
		"`StartActivity`",
		"@ core/Foo.java:17",
		"`ActivityManager`",
		"@ core/Foo.java:8",
		"language=java",
	} {
		if !strings.Contains(base.Summary, want) {
			t.Fatalf("base source_inventory missing %q:\n%s", want, base.Summary)
		}
	}
	if strings.Contains(base.Summary, "RunSchedule") || strings.Contains(base.Summary, "Scheduler.cpp") {
		t.Fatalf("base source_inventory leaked ressched rows:\n%s", base.Summary)
	}

	ressched, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": "hm_z/foundation/resourceschedule/ressched",
		"view": "source_inventory",
		"scope": "services",
		"roles": ["function", "method", "type"],
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("ressched source_inventory returned error: %v", err)
	}
	if !ressched.Success {
		t.Fatalf("ressched source_inventory failed: %+v", ressched)
	}
	for _, want := range []string{
		"`RunSchedule`",
		"@ services/Scheduler.cpp:42",
		"`Scheduler`",
		"@ services/Scheduler.cpp:9",
		"language=cpp",
	} {
		if !strings.Contains(ressched.Summary, want) {
			t.Fatalf("ressched source_inventory missing %q:\n%s", want, ressched.Summary)
		}
	}
	if strings.Contains(ressched.Summary, "StartActivity") || strings.Contains(ressched.Summary, "Foo.java") {
		t.Fatalf("ressched source_inventory leaked base rows:\n%s", ressched.Summary)
	}
}

func TestRepoMapSourceInventoryViewMultiRepoConfigFilesStayScoped(t *testing.T) {
	parent := t.TempDir()
	baseRoot := filepath.Join(parent, "frameworks", "base")
	resschedRoot := filepath.Join(parent, "hm_z", "foundation", "resourceschedule", "ressched")
	if err := os.MkdirAll(baseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resschedRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := &types.BusContext{
		RepoRoot:   parent,
		MultiGraph: newSourceInventoryTestMultiGraph(t, parent, baseRoot, resschedRoot),
		Mutable:    types.NewMutableState("source inventory multirepo config"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}

	base, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": "frameworks/base",
		"view": "source_inventory",
		"scope": ".",
		"roles": ["config_file"],
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("base config source_inventory returned error: %v", err)
	}
	if !base.Success {
		t.Fatalf("base config source_inventory failed: %+v", base)
	}
	if !strings.Contains(base.Summary, "`AndroidManifest.xml`") ||
		strings.Contains(base.Summary, "CMakeLists.txt") {
		t.Fatalf("base config source_inventory should stay in base subrepo:\n%s", base.Summary)
	}

	ressched, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": "hm_z/foundation/resourceschedule/ressched",
		"view": "source_inventory",
		"scope": ".",
		"roles": ["config_file"],
		"include_counts": true
	}`))
	if err != nil {
		t.Fatalf("ressched config source_inventory returned error: %v", err)
	}
	if !ressched.Success {
		t.Fatalf("ressched config source_inventory failed: %+v", ressched)
	}
	if !strings.Contains(ressched.Summary, "`CMakeLists.txt`") ||
		strings.Contains(ressched.Summary, "AndroidManifest.xml") {
		t.Fatalf("ressched config source_inventory should stay in ressched subrepo:\n%s", ressched.Summary)
	}
}

func TestRepoMapSourceInventoryViewMultiRepoRejectsInactiveSubRepo(t *testing.T) {
	parent := t.TempDir()
	baseRoot := filepath.Join(parent, "frameworks", "base")
	resschedRoot := filepath.Join(parent, "hm_z", "foundation", "resourceschedule", "ressched")
	if err := os.MkdirAll(baseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resschedRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := &types.BusContext{
		RepoRoot:        parent,
		MultiGraph:      newSourceInventoryTestMultiGraph(t, parent, baseRoot, resschedRoot),
		Mutable:         types.NewMutableState("source inventory multirepo inactive"),
		PendingSubRepos: []string{"hm_z/foundation/resourceschedule/ressched"},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{
		"path": "hm_z/foundation/resourceschedule/ressched",
		"view": "source_inventory",
		"scope": "services",
		"roles": ["function"]
	}`))
	if err != nil {
		t.Fatalf("repo_map source_inventory returned unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("inactive subrepo source_inventory must be refused, got: %+v", res)
	}
	if !strings.Contains(res.Summary, "currently outside the active") ||
		!strings.Contains(res.Summary, "hm_z/foundation/resourceschedule/ressched") {
		t.Fatalf("inactive refusal should be active-set prose, got: %q", res.Summary)
	}
	if obs := ctx.Mutable.SourceInventoryObservation(); obs.IsActive() {
		t.Fatalf("inactive source_inventory refusal must not publish observations: %+v", obs)
	}
}

func TestGraphFromAgentContextOrLoadMultiRepoHonorsExplicitSubRepoRoot(t *testing.T) {
	parent := t.TempDir()
	baseRoot := filepath.Join(parent, "frameworks", "base")
	resschedRoot := filepath.Join(parent, "hm_z", "foundation", "resourceschedule", "ressched")
	if err := os.MkdirAll(baseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resschedRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mg := newTestMultiGraph(t, parent, baseRoot, resschedRoot)
	ctx := &types.AgentContext{RepoRoot: parent, MultiGraph: mg}

	graph, err := GraphFromAgentContextOrLoad(ctx, resschedRoot, "ressched")
	if err != nil {
		t.Fatalf("GraphFromAgentContextOrLoad returned error: %v", err)
	}
	if graph == nil {
		t.Fatal("GraphFromAgentContextOrLoad returned nil graph")
	}
	if !sameRepoMapRoot(graph.Root, resschedRoot) {
		t.Fatalf("explicit ressched root was routed to %q", graph.Root)
	}
	if graph.Metadata.Languages["cpp"] != 1 || graph.Metadata.Languages["java"] != 0 {
		t.Fatalf("explicit ressched root got wrong graph metadata: %+v", graph.Metadata.Languages)
	}
}

func TestGraphFromBusContextOrLoadSingleRepoHonorsSubdirRoot(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "src", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src", "beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "alpha", "alpha.go"), []byte("package alpha\nfunc RunAlpha() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "beta", "beta.go"), []byte("package beta\nfunc RunBeta() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	topo := &topology.RepoTopology{
		ParentRoot: repo,
		Repos: []topology.SubRepo{{
			Slug:      "single-00000000",
			RootAbs:   repo,
			RootRel:   ".",
			FileCount: 2,
		}},
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build: func(root, query string) (*Graph, error) {
			return BuildGraph(root, []*FileInfo{
				{RelPath: "src/alpha/alpha.go", Language: LangGo},
				{RelPath: "src/beta/beta.go", Language: LangGo},
			}), nil
		},
		Cap: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	graph, err := GraphFromBusContextOrLoad(
		&types.BusContext{RepoRoot: repo, MultiGraph: mg},
		filepath.Join(repo, "src", "alpha"),
		"",
	)
	if err != nil {
		t.Fatalf("GraphFromBusContextOrLoad returned error: %v", err)
	}
	if graph == nil {
		t.Fatal("GraphFromBusContextOrLoad returned nil graph")
	}
	if !sameRepoMapRoot(graph.Root, filepath.Join(repo, "src", "alpha")) {
		t.Fatalf("subdir root was ignored; graph.Root=%q", graph.Root)
	}
	if _, ok := graph.FileIndex["alpha.go"]; !ok {
		t.Fatalf("subdir graph missing alpha.go: keys=%v", graph.FileIndex)
	}
	if _, ok := graph.FileIndex["src/beta/beta.go"]; ok {
		t.Fatalf("single-repo subdir graph leaked beta file: keys=%v", graph.FileIndex)
	}
}

func TestGraphFromBusContextOrLoadProjectsSubdirFromMutableGraph(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "src", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src", "beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := BuildGraph(repo, []*FileInfo{{
		RelPath:  "src/alpha/alpha.go",
		Language: LangGo,
		Package:  "alpha",
		Symbols: []Symbol{{
			Name:     "RunAlpha",
			Kind:     "function",
			File:     "src/alpha/alpha.go",
			Line:     3,
			Exported: true,
		}},
		Relations: []Relation{{
			Kind: "call",
			From: "src/alpha/alpha.go:RunAlpha",
			To:   "src/beta/beta.go:RunBeta",
			File: "src/alpha/alpha.go",
			Line: 4,
			ToEP: RelationEndpoint{
				Name: "RunBeta",
				File: "src/beta/beta.go",
				Line: 3,
			},
		}},
	}, {
		RelPath:  "src/beta/beta.go",
		Language: LangGo,
		Package:  "beta",
		Symbols: []Symbol{{
			Name:     "RunBeta",
			Kind:     "function",
			File:     "src/beta/beta.go",
			Line:     3,
			Exported: true,
		}},
	}})
	mut := types.NewMutableState("scoped projection")
	mut.SetSearchGraph(base)
	ctx := &types.BusContext{RepoRoot: repo, Mutable: mut}

	graph, err := GraphFromBusContextOrLoad(ctx, filepath.Join(repo, "src", "alpha"), "RunAlpha")
	if err != nil {
		t.Fatalf("GraphFromBusContextOrLoad returned error: %v", err)
	}
	if graph == nil {
		t.Fatal("GraphFromBusContextOrLoad returned nil graph")
	}
	if !sameRepoMapRoot(graph.Root, filepath.Join(repo, "src", "alpha")) {
		t.Fatalf("projected graph root = %q", graph.Root)
	}
	if _, ok := graph.FileIndex["alpha.go"]; !ok {
		t.Fatalf("projected graph missing rebased alpha.go: keys=%v", graph.FileIndex)
	}
	if _, ok := graph.FileIndex["src/beta/beta.go"]; ok {
		t.Fatalf("projected graph leaked beta file: keys=%v", graph.FileIndex)
	}
	if _, ok := graph.SymbolDefs["RunAlpha"]; !ok {
		t.Fatalf("projected graph missing RunAlpha symbol: %+v", graph.SymbolDefs)
	}
	if strings.Contains(strings.Join(graph.ImportGraph["alpha.go"], ","), "beta") {
		t.Fatalf("projected graph leaked cross-scope import graph: %+v", graph.ImportGraph)
	}
	if base.FileIndex["src/alpha/alpha.go"].Symbols[0].File != "src/alpha/alpha.go" {
		t.Fatalf("projection mutated base graph symbol file: %+v", base.FileIndex["src/alpha/alpha.go"].Symbols[0])
	}
	cached, ok := mut.ScopedSearchGraph(scopedGraphProjectionCacheKey(repo, filepath.Join(repo, "src", "alpha"), "RunAlpha")).(*Graph)
	if !ok || cached == nil {
		t.Fatalf("projected graph was not cached")
	}
	if _, ok := cached.FileIndex["alpha.go"]; !ok {
		t.Fatalf("cached projection missing alpha.go: %+v", cached.FileIndex)
	}
}

func TestGraphFromBusContextOrLoadProjectsSubdirFromReadOnlySearchGraph(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "src", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src", "beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := BuildGraph(repo, []*FileInfo{{
		RelPath:  "src/alpha/alpha.go",
		Language: LangGo,
		Package:  "alpha",
		Symbols: []Symbol{{
			Name:     "RunAlpha",
			Kind:     "function",
			File:     "src/alpha/alpha.go",
			Line:     3,
			Exported: true,
		}},
	}, {
		RelPath:  "src/beta/beta.go",
		Language: LangGo,
		Package:  "beta",
		Symbols: []Symbol{{
			Name:     "RunBeta",
			Kind:     "function",
			File:     "src/beta/beta.go",
			Line:     3,
			Exported: true,
		}},
	}})
	ctx := &types.BusContext{RepoRoot: repo, SearchGraph: base}

	graph, err := GraphFromBusContextOrLoad(ctx, filepath.Join(repo, "src", "alpha"), "RunAlpha")
	if err != nil {
		t.Fatalf("GraphFromBusContextOrLoad returned error: %v", err)
	}
	if graph == nil {
		t.Fatal("GraphFromBusContextOrLoad returned nil graph")
	}
	if !sameRepoMapRoot(graph.Root, filepath.Join(repo, "src", "alpha")) {
		t.Fatalf("projected graph root = %q", graph.Root)
	}
	if _, ok := graph.FileIndex["alpha.go"]; !ok {
		t.Fatalf("projected graph missing rebased alpha.go: keys=%v", graph.FileIndex)
	}
	if _, ok := graph.FileIndex["src/beta/beta.go"]; ok {
		t.Fatalf("projected graph leaked sibling package: keys=%v", graph.FileIndex)
	}
}

func TestGraphFromBusContextOrLoadProjectsSubdirFromMutableSubRepoGraph(t *testing.T) {
	parent := t.TempDir()
	subRepoRoot := filepath.Join(parent, "CodeAgent")
	scopeRoot := filepath.Join(subRepoRoot, "packages", "cli", "src")
	if err := os.MkdirAll(scopeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	base := BuildGraph(subRepoRoot, []*FileInfo{{
		RelPath:  "packages/cli/src/auth.ts",
		Language: LangTypeScript,
		Package:  "cli",
		Symbols: []Symbol{{
			Name:     "validateNonInteractiveAuth",
			Kind:     "function",
			File:     "packages/cli/src/auth.ts",
			Line:     12,
			Exported: true,
		}},
	}, {
		RelPath:  "packages/server/src/server.ts",
		Language: LangTypeScript,
		Package:  "server",
		Symbols: []Symbol{{
			Name:     "startServer",
			Kind:     "function",
			File:     "packages/server/src/server.ts",
			Line:     7,
			Exported: true,
		}},
	}})
	mut := types.NewMutableState("subrepo scoped projection")
	mut.SetSearchGraph(base)
	ctx := &types.BusContext{RepoRoot: parent, Mutable: mut}

	graph, err := GraphFromBusContextOrLoad(ctx, scopeRoot, "validateNonInteractiveAuth W3 token")
	if err != nil {
		t.Fatalf("GraphFromBusContextOrLoad returned error: %v", err)
	}
	if graph == nil {
		t.Fatal("GraphFromBusContextOrLoad returned nil graph")
	}
	if !sameRepoMapRoot(graph.Root, scopeRoot) {
		t.Fatalf("projected graph root = %q, want %q", graph.Root, scopeRoot)
	}
	if _, ok := graph.FileIndex["auth.ts"]; !ok {
		t.Fatalf("projected graph missing rebased auth.ts: keys=%v", graph.FileIndex)
	}
	if _, ok := graph.FileIndex["packages/server/src/server.ts"]; ok {
		t.Fatalf("projected graph leaked sibling package: keys=%v", graph.FileIndex)
	}
	if _, ok := graph.SymbolDefs["validateNonInteractiveAuth"]; !ok {
		t.Fatalf("projected graph missing function symbol: %+v", graph.SymbolDefs)
	}
	key := scopedGraphProjectionCacheKey(subRepoRoot, scopeRoot, "validateNonInteractiveAuth W3 token")
	if cached, ok := mut.ScopedSearchGraph(key).(*Graph); !ok || cached == nil {
		t.Fatalf("subrepo-root projection was not cached under base graph root")
	}
}

func TestBuildOrLoadGraphWithinRejectsBeforeScanner(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := BuildOrLoadGraphWithin(outside, repo, ""); err == nil {
		t.Fatal("expected scoped graph loader to reject before scanning outside root")
	}
}

func newTestMultiGraph(t *testing.T, parent, baseRoot, resschedRoot string) *multigraph.MultiGraph {
	t.Helper()
	topo := &topology.RepoTopology{
		ParentRoot: parent,
		Repos: []topology.SubRepo{
			{
				Slug:      "base-10eb2a5e",
				RootAbs:   baseRoot,
				RootRel:   "frameworks/base",
				FileCount: 41530,
			},
			{
				Slug:      "ressched-c088d3ed",
				RootAbs:   resschedRoot,
				RootRel:   "hm_z/foundation/resourceschedule/ressched",
				FileCount: 259,
			},
		},
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build: func(root, query string) (*Graph, error) {
			switch {
			case sameRepoMapRoot(root, baseRoot):
				return BuildGraph(root, []*FileInfo{{
					RelPath:  "core/Foo.java",
					Language: "java",
					Size:     1,
				}}), nil
			case sameRepoMapRoot(root, resschedRoot):
				return BuildGraph(root, []*FileInfo{{
					RelPath:  "services/Scheduler.cpp",
					Language: "cpp",
					Size:     1,
				}}), nil
			default:
				t.Fatalf("unexpected graph root %q", root)
				return nil, nil
			}
		},
		Cap: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return mg
}

func newSourceInventoryTestMultiGraph(t *testing.T, parent, baseRoot, resschedRoot string) *multigraph.MultiGraph {
	t.Helper()
	topo := &topology.RepoTopology{
		ParentRoot: parent,
		Repos: []topology.SubRepo{
			{
				Slug:      "base-10eb2a5e",
				RootAbs:   baseRoot,
				RootRel:   "frameworks/base",
				FileCount: 41530,
			},
			{
				Slug:      "ressched-c088d3ed",
				RootAbs:   resschedRoot,
				RootRel:   "hm_z/foundation/resourceschedule/ressched",
				FileCount: 259,
			},
		},
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build: func(root, query string) (*Graph, error) {
			switch {
			case sameRepoMapRoot(root, baseRoot):
				return BuildGraph(root, []*FileInfo{
					{
						RelPath:  "core/Foo.java",
						Language: LangJava,
						Package:  "com.android.server",
						Symbols: []Symbol{
							{Name: "ActivityManager", Kind: "class", File: "core/Foo.java", Line: 8, Exported: true},
							{Name: "StartActivity", Kind: "method", File: "core/Foo.java", Line: 17, Parent: "ActivityManager", Exported: true},
						},
					},
					{
						RelPath:     "AndroidManifest.xml",
						IsSpecial:   true,
						SpecialType: "build_config",
					},
				}), nil
			case sameRepoMapRoot(root, resschedRoot):
				return BuildGraph(root, []*FileInfo{
					{
						RelPath:  "services/Scheduler.cpp",
						Language: LangCpp,
						Package:  "ressched",
						Symbols: []Symbol{
							{Name: "Scheduler", Kind: "class", File: "services/Scheduler.cpp", Line: 9, Exported: true},
							{Name: "RunSchedule", Kind: "function", File: "services/Scheduler.cpp", Line: 42, Exported: true},
						},
					},
					{
						RelPath:     "CMakeLists.txt",
						IsSpecial:   true,
						SpecialType: "build_config",
					},
				}), nil
			default:
				t.Fatalf("unexpected graph root %q", root)
				return nil, nil
			}
		},
		Cap: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return mg
}
