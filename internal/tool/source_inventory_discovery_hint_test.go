package tool

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type sourceInventoryDiscoveryAliasGater struct {
	aliases map[string]string
}

func (g sourceInventoryDiscoveryAliasGater) ResolveActiveSetPath(_ *types.BusContext, _ string, llmPath string, _ func(string) bool) types.ActiveSetGateResult {
	if dst := g.aliases[llmPath]; dst != "" {
		return types.ActiveSetGateResult{Allowed: true, ResolvedPath: dst, AutoPrefixed: true}
	}
	return types.ActiveSetGateResult{Allowed: true, ResolvedPath: llmPath}
}

func (g sourceInventoryDiscoveryAliasGater) ResolveActiveSetCommand(_ *types.BusContext, _, _ string) types.ActiveSetGateResult {
	return types.ActiveSetGateResult{Allowed: true}
}

func sourceInventoryDiscoveryListFilesResult(path string, files []string) types.ToolResult {
	lines := append([]string{"[list_files: path=" + path + " recursive=false]"}, files...)
	return types.ToolResult{
		ToolName: "list_files",
		Success:  true,
		Summary:  strings.Join(lines, "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindListFiles,
			Path:           path,
			ResultCount:    len(files),
			CandidateFiles: files,
		},
	}
}

func sourceInventoryDiscoveryGrepFilesOnlyResult(path string, files []string) types.ToolResult {
	lines := append([]string{"[grep params: path=" + path + " files_only=true]"}, files...)
	return types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  strings.Join(lines, "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindGrep,
			Path:           path,
			FilesOnly:      true,
			ResultCount:    len(files),
			CandidateFiles: files,
		},
	}
}

func sourceInventoryDiscoveryRepoMapResult(path string, files []string) types.ToolResult {
	return types.ToolResult{
		ToolName: "repo_map",
		Success:  true,
		Summary: strings.Join([]string{
			"# File Map",
			"",
			"## rendered-summary-title-that-must-not-drive-discovery.go [go]",
		}, "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindRepoMap,
			Path:           path,
			ResultCount:    len(files),
			CandidateFiles: files,
		},
	}
}

func TestSourceInventoryDiscoveryHintFromListFilesBroadResultDedupe(t *testing.T) {
	ctx := &types.BusContext{Mutable: types.NewMutableState("plain unrelated objective")}
	result := sourceInventoryDiscoveryListFilesResult("internal/analysis", []string{
		"internal/analysis/aggregator",
		"internal/analysis/criterion",
		"internal/analysis/normalizer",
		"internal/analysis/subject",
		"internal/analysis/sourcemix",
		"internal/analysis/stopcond",
		"internal/analysis/types",
		"internal/analysis/trace",
	})

	hint := SourceInventoryDiscoveryHintFromToolObservation(ctx, result, json.RawMessage(`{"path":"internal/analysis"}`))
	for _, want := range []string{
		"Repo Lens discovery hint (advisory)",
		"choose one branch before source_inventory expansion",
		`"roles": ["function", "method", "type", "config_key", "route"]`,
		`"include_attributes": false`,
		`"scope": "aggregator"`,
		"after choosing a narrow scope/member",
		`"scope": "<chosen-scope>"`,
		`"attribute_roles": ["function", "method", "type", "config_key", "route"]`,
		`"view": "relation_map"`,
		`"sources": ["<symbol-or-file>"]`,
		`"relation_kinds": ["call", "import", "inheritance", "reference"]`,
		types.SourceInventoryMechanicalFactBoundary,
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("discovery hint missing %q:\n%s", want, hint)
		}
	}
	if strings.Contains(hint, `"roles": ["package", "file"]`) || strings.Contains(hint, "scope summary") {
		t.Fatalf("discovery hint should not lead with package/file summary calls that can be empty before analysis IR:\n%s", hint)
	}
	if strings.Contains(hint, `repo_map {"path": "internal/analysis", "view": "source_inventory", "roles"`) {
		t.Fatalf("discovery hint must not suggest root-wide fine-grained source_inventory expansion when branch scopes exist:\n%s", hint)
	}
	if again := SourceInventoryDiscoveryHintFromToolObservation(ctx, result, json.RawMessage(`{"path":"internal/analysis"}`)); again != "" {
		t.Fatalf("discovery hint should dedupe repeated broad tool result, got:\n%s", again)
	}
}

func TestSourceInventoryDiscoveryHintSuppressedDuringAnalyze(t *testing.T) {
	ctx := &types.BusContext{
		Mutable:       types.NewMutableState("plain objective"),
		PipelineStage: types.StageAnalyze,
	}
	result := sourceInventoryDiscoveryListFilesResult("internal/analysis", []string{
		"internal/analysis/aggregator",
		"internal/analysis/criterion",
		"internal/analysis/normalizer",
		"internal/analysis/subject",
		"internal/analysis/sourcemix",
		"internal/analysis/stopcond",
		"internal/analysis/types",
		"internal/analysis/trace",
	})

	if hint := SourceInventoryDiscoveryHintFromToolObservation(ctx, result, json.RawMessage(`{"path":"internal/analysis"}`)); hint != "" {
		t.Fatalf("analyze stage must not advertise deep source-inventory expansion:\n%s", hint)
	}
}

func TestSourceInventoryDiscoveryHintNormalizesActiveSetPath(t *testing.T) {
	ctx := &types.BusContext{
		Mutable: types.NewMutableState("plain objective"),
		MultiGraph: sourceInventoryDiscoveryAliasGater{aliases: map[string]string{
			"analysis": "subrepo/internal/analysis",
		}},
	}
	result := sourceInventoryDiscoveryRepoMapResult("analysis", []string{
		"subrepo/internal/analysis/aggregator/a.go",
		"subrepo/internal/analysis/criterion/b.go",
		"subrepo/internal/analysis/normalizer/c.go",
		"subrepo/internal/analysis/subject/d.go",
		"subrepo/internal/analysis/sourcemix/e.go",
		"subrepo/internal/analysis/stopcond/f.go",
		"subrepo/internal/analysis/types/g.go",
		"subrepo/internal/analysis/trace/h.go",
	})

	hint := SourceInventoryDiscoveryHintFromToolObservation(ctx, result, json.RawMessage(`{"path":"analysis","view":"file_map"}`))
	if !strings.Contains(hint, `repo_map {"path": "subrepo/internal/analysis", "view": "source_inventory"`) ||
		!strings.Contains(hint, `"scope": "aggregator"`) {
		t.Fatalf("discovery hint should use active-set-normalized path and scoped branches:\n%s", hint)
	}
}

func TestSourceInventoryDiscoveryHintNormalizesAbsolutePathInsideRepo(t *testing.T) {
	repoRoot := t.TempDir()
	absPath := filepath.Join(repoRoot, "internal", "analysis")
	ctx := &types.BusContext{Mutable: types.NewMutableState("plain objective"), RepoRoot: repoRoot}
	result := sourceInventoryDiscoveryListFilesResult(absPath, []string{
		filepath.Join(absPath, "aggregator"),
		filepath.Join(absPath, "criterion"),
		filepath.Join(absPath, "normalizer"),
		filepath.Join(absPath, "subject"),
		filepath.Join(absPath, "sourcemix"),
		filepath.Join(absPath, "stopcond"),
		filepath.Join(absPath, "types"),
		filepath.Join(absPath, "trace"),
	})

	hint := SourceInventoryDiscoveryHintFromToolObservation(ctx, result, json.RawMessage(`{"path":`+strconv.Quote(absPath)+`}`))
	if !strings.Contains(hint, `repo_map {"path": "internal/analysis", "view": "source_inventory"`) ||
		!strings.Contains(hint, `"scope": "aggregator"`) {
		t.Fatalf("absolute in-repo paths should normalize to repo-relative source-inventory calls:\n%s", hint)
	}
}

func TestSourceInventoryDiscoveryHintRejectsUnsafePath(t *testing.T) {
	ctx := &types.BusContext{Mutable: types.NewMutableState("plain objective")}
	result := sourceInventoryDiscoveryListFilesResult("../outside", []string{
		"../outside/a",
		"../outside/b",
		"../outside/c",
		"../outside/d",
		"../outside/e",
		"../outside/f",
		"../outside/g",
		"../outside/h",
	})

	if hint := SourceInventoryDiscoveryHintFromToolObservation(ctx, result, json.RawMessage(`{"path":"../outside"}`)); hint != "" {
		t.Fatalf("unsafe parent-escape path must not produce repo-lens hint:\n%s", hint)
	}
}

func TestSourceInventoryDiscoveryHintFromGrepFilesOnly(t *testing.T) {
	ctx := &types.BusContext{Mutable: types.NewMutableState("plain objective")}
	result := sourceInventoryDiscoveryGrepFilesOnlyResult("services", []string{
		"services/auth/login.py",
		"services/auth/routes.py",
		"services/billing/router.ts",
		"services/billing/config.yaml",
		"services/metrics/exporter.rs",
		"services/metrics/routes.rb",
		"services/search/indexer.java",
		"services/search/config.toml",
	})

	hint := SourceInventoryDiscoveryHintFromToolObservation(ctx, result, json.RawMessage(`{"path":"services","files_only":true}`))
	if !strings.Contains(hint, `repo_map {"path": "services", "view": "source_inventory"`) ||
		!strings.Contains(hint, `"scope": "auth"`) {
		t.Fatalf("grep files_only should produce scoped source-inventory discovery hint:\n%s", hint)
	}

	plainCtx := &types.BusContext{Mutable: types.NewMutableState("plain objective")}
	contentResult := result
	contentResult.PathDiscovery = &types.ToolPathDiscovery{
		Kind:        types.ToolPathDiscoveryKindGrep,
		Path:        "services",
		FilesOnly:   false,
		ResultCount: result.PathDiscovery.ResultCount,
	}
	if noHint := SourceInventoryDiscoveryHintFromToolObservation(plainCtx, contentResult, json.RawMessage(`{"path":"services","files_only":false}`)); noHint != "" {
		t.Fatalf("content grep must not produce broad source-inventory discovery hint:\n%s", noHint)
	}
}

func TestSourceInventoryDiscoveryHintFromRepoMapPreservesStructuredRoles(t *testing.T) {
	ctx := &types.BusContext{Mutable: types.NewMutableState("plain objective")}
	result := sourceInventoryDiscoveryRepoMapResult("src", []string{
		"src/api/routes.py",
		"src/api/config.yaml",
		"src/web/router.ts",
		"src/web/settings.json",
		"src/mobile/routes.kt",
		"src/mobile/app.conf",
		"src/admin/routes.rb",
		"src/admin/config.toml",
	})

	hint := SourceInventoryDiscoveryHintFromToolObservation(ctx, result, json.RawMessage(`{
		"path":"src",
		"view":"file_map",
		"roles":["route"],
		"attribute_roles":["config_key"]
	}`))
	for _, want := range []string{
		`"roles": ["route"]`,
		`"attribute_roles": ["config_key"]`,
		`"scope": "api"`,
		`"scope": "web"`,
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("repo_map discovery hint missing %q:\n%s", want, hint)
		}
	}

	sourceInventoryCtx := &types.BusContext{Mutable: types.NewMutableState("plain objective")}
	if noHint := SourceInventoryDiscoveryHintFromToolObservation(sourceInventoryCtx, result, json.RawMessage(`{"path":"src","view":"source_inventory"}`)); noHint != "" {
		t.Fatalf("source_inventory results should not recursively hint source_inventory:\n%s", noHint)
	}
}

func TestSourceInventoryDiscoveryHintFromRepoMapIgnoresSummaryOnlyRows(t *testing.T) {
	ctx := &types.BusContext{Mutable: types.NewMutableState("plain objective")}
	result := types.ToolResult{
		ToolName: "repo_map",
		Success:  true,
		Summary: strings.Join([]string{
			"# File Map",
			"",
			"## src/api/routes.py [python]",
			"## src/web/router.ts [typescript]",
			"## src/mobile/routes.kt [kotlin]",
			"## src/admin/routes.rb [ruby]",
			"## src/ops/routes.rs [rust]",
			"## src/cpp/router.cc [cpp]",
			"## src/java/Router.java [java]",
			"## src/kotlin/Router.kt [kotlin]",
		}, "\n"),
	}

	if hint := SourceInventoryDiscoveryHintFromToolObservation(ctx, result, json.RawMessage(`{"path":"src","view":"file_map"}`)); hint != "" {
		t.Fatalf("repo_map rendered summary rows must not produce discovery hint without typed path discovery:\n%s", hint)
	}
}

func TestSourceInventoryDiscoveryHintFromToolHistory(t *testing.T) {
	results := []types.ToolResult{
		{ToolName: "read_file", Success: true, Summary: "read ok"},
		{
			ToolName: "list_files",
			Success:  true,
			Summary: strings.Join([]string{
				"[list_files: path=pkg recursive=false]",
				"pkg/a",
				"pkg/b",
				"pkg/c",
				"pkg/d",
				"pkg/e",
				"pkg/f",
				"pkg/g",
				"pkg/h",
			}, "\n"),
			PathDiscovery: &types.ToolPathDiscovery{
				Kind:           types.ToolPathDiscoveryKindListFiles,
				Path:           "pkg",
				ResultCount:    8,
				CandidateFiles: []string{"pkg/a", "pkg/b", "pkg/c", "pkg/d", "pkg/e", "pkg/f", "pkg/g", "pkg/h"},
			},
		},
	}

	hint := SourceInventoryDiscoveryHintFromToolHistory(results)
	if !strings.Contains(hint, "Repo Lens discovery hint") ||
		!strings.Contains(hint, `repo_map {"path": "pkg", "view": "source_inventory"`) {
		t.Fatalf("history-based discovery hint missing expected repo lens call:\n%s", hint)
	}
}

func TestSourceInventoryDiscoveryHintFromToolHistoryIgnoresSummaryOnlyRows(t *testing.T) {
	results := []types.ToolResult{{
		ToolName: "list_files",
		Success:  true,
		Summary: strings.Join([]string{
			"[list_files: path=pkg recursive=false]",
			"pkg/a",
			"pkg/b",
			"pkg/c",
			"pkg/d",
			"pkg/e",
			"pkg/f",
			"pkg/g",
			"pkg/h",
		}, "\n"),
	}}

	if hint := SourceInventoryDiscoveryHintFromToolHistory(results); hint != "" {
		t.Fatalf("summary-only path rows must not produce source-inventory history hint:\n%s", hint)
	}
}

func TestSourceInventoryDiscoveryHintFromToolHistoryReadsRepoMapCarrierOnly(t *testing.T) {
	summaryOnly := types.ToolResult{
		ToolName: "repo_map",
		Success:  true,
		Summary: strings.Join([]string{
			"# File Map",
			"",
			"## pkg/a.go [go]",
			"## pkg/b.go [go]",
			"## pkg/c.go [go]",
			"## pkg/d.go [go]",
			"## pkg/e.go [go]",
			"## pkg/f.go [go]",
			"## pkg/g.go [go]",
			"## pkg/h.go [go]",
		}, "\n"),
	}
	if hint := SourceInventoryDiscoveryHintFromToolHistory([]types.ToolResult{summaryOnly}); hint != "" {
		t.Fatalf("summary-only repo_map history must not produce source-inventory hint:\n%s", hint)
	}

	typed := sourceInventoryDiscoveryRepoMapResult("pkg", []string{
		"pkg/a.go",
		"pkg/b.go",
		"pkg/c.go",
		"pkg/d.go",
		"pkg/e.go",
		"pkg/f.go",
		"pkg/g.go",
		"pkg/h.go",
	})
	hint := SourceInventoryDiscoveryHintFromToolHistory([]types.ToolResult{summaryOnly, typed})
	if !strings.Contains(hint, "Repo Lens discovery hint") ||
		!strings.Contains(hint, `repo_map {"path": "pkg", "view": "source_inventory"`) {
		t.Fatalf("typed repo_map carrier should produce history discovery hint:\n%s", hint)
	}
}
