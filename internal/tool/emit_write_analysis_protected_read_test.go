package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func protectedReadAnalysisParams(t *testing.T, target string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"task":        map[string]any{"kind": "bugfix", "scope": "micro", "summary": "preserve the observed baseline"},
		"risk":        map[string]any{"overall": "low"},
		"constraints": []map[string]string{{"kind": "preserve_regression_test", "target": target, "note": "keep the existing oracle unchanged"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func protectedReadBus(t *testing.T, path string) *types.BusContext {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, path), []byte("baseline one\nbaseline two\nbaseline three\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{RepoRoot: root, WorkDir: root, Mode: types.ModePlan, Mutable: types.NewMutableState("preserve the existing baseline")}
}

func protectedReadResult(t *testing.T, bus *types.BusContext, path string) types.ToolResult {
	t.Helper()
	p, err := json.Marshal(map[string]any{"path": path, "line_offset": 1, "limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	r, err := (&ReadFile{}).Execute(bus, p)
	if err != nil || !r.Success {
		t.Fatalf("read_file: err=%v result=%+v", err, r)
	}
	if r.ReadCoverage == nil || r.ReadCoverage.RawRef == "" {
		t.Fatalf("real read lacks coverage: %+v", r)
	}
	return r
}

func TestEmitWriteAnalysisProtectedReadAcceptsObservedBaselineWithoutNamingAuthority(t *testing.T) {
	for _, path := range []string{"test_repository.c", "checks/oracle.cj", "fixtures/expected.output", "fixtures/expected[1].output", "custom/SafetyHarness.ets"} {
		t.Run(path, func(t *testing.T) {
			if types.LooksLikeTestFilePath(path) {
				t.Fatalf("fixture must exercise the non-conventional lane: %q", path)
			}
			bus := protectedReadBus(t, path)
			r := protectedReadResult(t, bus, path)
			bus.Mutable.AppendDispatchToolResult(r)
			before := bus.Mutable.DispatchToolResults()
			result, err := (&EmitWriteAnalysis{}).Execute(bus, protectedReadAnalysisParams(t, "./"+path))
			if err != nil || !result.Success {
				t.Fatalf("observed exact baseline must remain protectable: err=%v summary=%s", err, result.Summary)
			}
			ir := bus.Mutable.WriteAnalysisIR()
			if len(ir.Request.Constraints) != 1 || ir.Request.Constraints[0].Target != path || ir.Request.Constraints[0].Note != "keep the existing oracle unchanged" {
				t.Fatalf("constraint changed: %+v", ir)
			}
			if len(bus.Mutable.DispatchToolResults()) != len(before) || bus.Mutable.ChangeReport() != nil || bus.Mutable.ChangePlan() != nil {
				t.Fatal("preserving bytes must not mint tool, plan, or verification authority")
			}
			if types.LooksLikeTestFilePath(path) {
				t.Fatal("read receipt must not change source-role classification")
			}
		})
	}
}

func TestEmitWriteAnalysisProtectedReadSpecialLiteralNameNeedsExactRead(t *testing.T) {
	for _, path := range []string{"tests/expected[1].py", "tests/literal*.py", "tests/literal?.py", "fixtures/{baseline}.output"} {
		t.Run(path, func(t *testing.T) {
			bus := protectedReadBus(t, path)
			result, err := (&EmitWriteAnalysis{}).Execute(bus, protectedReadAnalysisParams(t, path))
			if err != nil || result.Success {
				t.Fatalf("unobserved pattern-looking name must not inherit convention authority: %+v %v", result, err)
			}
			read := protectedReadResult(t, bus, path)
			bus.Mutable.AppendDispatchToolResult(read)
			result, err = (&EmitWriteAnalysis{}).Execute(bus, protectedReadAnalysisParams(t, path))
			if err != nil || !result.Success {
				t.Fatalf("exact literal read must not be confused with a glob: %+v %v", result, err)
			}
		})
	}
}

func TestEmitWriteAnalysisProtectedReadRejectsUnboundReads(t *testing.T) {
	for _, kind := range []string{"no_read", "failed", "summary_only", "other_tool", "other_path", "other_ref", "runtime", "history_only", "dispatch_reset", "other_repository", "no_dispatch_result", "invalid_range"} {
		t.Run(kind, func(t *testing.T) {
			path := "test_repository.c"
			bus := protectedReadBus(t, path)
			readBus := bus
			if kind == "other_repository" {
				readBus = protectedReadBus(t, path)
				readBus.Mutable = bus.Mutable
			}
			r := types.ToolResult{ToolName: "read_file", Success: true, Summary: "[test_repository.c: showing all 4 lines]"}
			if kind != "no_read" && kind != "summary_only" {
				r = protectedReadResult(t, readBus, path)
			}
			switch kind {
			case "failed":
				r.Success = false
			case "other_tool":
				r.ToolName = "grep"
			case "other_path":
				r.ReadCoverage.Path = "other/test_repository.c"
			case "other_ref":
				r.ReadCoverage.RawRef += "-other"
			case "runtime":
				r.RuntimeArtifactRead = &types.ToolRuntimeArtifactRead{RequestedPath: path}
			case "invalid_range":
				r.ReadCoverage.LineEnd = r.ReadCoverage.TotalLines + 1
			}
			if kind == "history_only" {
				bus.ToolResults = []types.ToolResult{r}
			} else if kind != "no_read" && kind != "no_dispatch_result" {
				bus.Mutable.AppendDispatchToolResult(r)
			}
			if kind == "dispatch_reset" {
				bus.Mutable.ResetDispatchToolResults()
				bus.Mutable.AppendDispatchToolResult(r)
			}
			result, err := (&EmitWriteAnalysis{}).Execute(bus, protectedReadAnalysisParams(t, path))
			if err != nil || result.Success || bus.Mutable.WriteAnalysisIR() != nil {
				t.Fatalf("unbound %s must not authorize fallback: err=%v result=%+v", kind, err, result)
			}
		})
	}
}

func TestEmitWriteAnalysisProtectedReadFreshDispatchRestoresObservedBaseline(t *testing.T) {
	bus := protectedReadBus(t, "test_repository.c")
	old := protectedReadResult(t, bus, "test_repository.c")
	bus.Mutable.AppendDispatchToolResult(old)
	bus.Mutable.ResetDispatchToolResults()
	fresh := protectedReadResult(t, bus, "test_repository.c")
	bus.Mutable.AppendDispatchToolResult(fresh)
	result, err := (&EmitWriteAnalysis{}).Execute(bus, protectedReadAnalysisParams(t, "test_repository.c"))
	if err != nil || !result.Success {
		t.Fatalf("fresh same-repository read must restore eligibility: %+v %v", result, err)
	}
}

type protectedReadSubRepoGater struct{ path, root string }

func (g protectedReadSubRepoGater) ResolveActiveSetPath(_ *types.BusContext, _, _ string, _ func(string) bool) types.ActiveSetGateResult {
	return types.ActiveSetGateResult{Allowed: true, ResolvedPath: g.path, SubRepoRootRel: g.root, AutoPrefixed: true}
}
func (protectedReadSubRepoGater) ResolveActiveSetCommand(_ *types.BusContext, _, _ string) types.ActiveSetGateResult {
	return types.ActiveSetGateResult{Allowed: true}
}

func TestEmitWriteAnalysisProtectedReadDoesNotPromoteResolvedSiblingOrSymlink(t *testing.T) {
	for _, kind := range []string{"resolved_subrepo", "outside_symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			path := "test_repository.c"
			bus := protectedReadBus(t, path)
			target := path
			var r types.ToolResult
			switch kind {
			case "resolved_subrepo":
				sibling := filepath.Join(bus.RepoRoot, "sibling")
				if err := os.Mkdir(sibling, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(sibling, path), []byte("another baseline\n"), 0644); err != nil {
					t.Fatal(err)
				}
				bus.MultiGraph = protectedReadSubRepoGater{path: "sibling/" + path, root: "sibling"}
				r = protectedReadResult(t, bus, path)
				if r.ReadCoverage.Path != "sibling/"+path {
					t.Fatalf("producer lost subrepo prefix: %+v", r.ReadCoverage)
				}
				// Neither the guessed bare spelling nor the prefixed path makes
				// a different physical repository a current-repository receipt.
				bus.Mutable.AppendDispatchToolResult(r)
				result, err := (&EmitWriteAnalysis{}).Execute(bus, protectedReadAnalysisParams(t, path))
				if err != nil || result.Success {
					t.Fatalf("bare alias authorized: %+v %v", result, err)
				}
				target = "sibling/" + path
			case "outside_symlink":
				other := protectedReadBus(t, path)
				target = "borrowed.c"
				if err := os.Symlink(filepath.Join(other.RepoRoot, path), filepath.Join(bus.RepoRoot, target)); err != nil {
					t.Fatal(err)
				}
				r = protectedReadResult(t, bus, target)
				bus.Mutable.AppendDispatchToolResult(r)
			case "directory":
				target = "tests/directory.py"
				if err := os.MkdirAll(filepath.Join(bus.RepoRoot, target), 0755); err != nil {
					t.Fatal(err)
				}
			}
			result, err := (&EmitWriteAnalysis{}).Execute(bus, protectedReadAnalysisParams(t, target))
			if err != nil || result.Success || bus.Mutable.WriteAnalysisIR() != nil {
				t.Fatalf("%s must not authorize a different/fileless baseline: %+v %v", kind, result, err)
			}
		})
	}
}

func TestEmitWriteAnalysisProtectedReadSchemaExplainsPreservationNotProof(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&EmitWriteAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	constraints := properties["constraints"].(map[string]any)
	fields := constraints["items"].(map[string]any)["properties"].(map[string]any)
	description := fields["target"].(map[string]any)["description"].(string)
	if !strings.Contains(description, types.WriteProtectedBaselineTargetTeaching) {
		t.Fatal("schema/skill baseline rule must share one source")
	}
	for _, want := range []string{"exact repo-relative file path", "current repository during this dispatch", "not test classification or execution proof"} {
		if !strings.Contains(description, want) {
			t.Fatalf("schema missing %q: %s", want, description)
		}
	}
}

func TestEmitWriteAnalysisProtectedReadRejectsNonFileTargets(t *testing.T) {
	for _, path := range []string{"../tests/oracle.py", "tests/*.py", "tests/oracle?.py", "tests/[a].py", "tests/../oracle.py", "tests/directory.py/"} {
		t.Run(path, func(t *testing.T) {
			bus := protectedReadBus(t, "test_repository.c")
			result, err := (&EmitWriteAnalysis{}).Execute(bus, protectedReadAnalysisParams(t, path))
			if err != nil || result.Success {
				t.Fatalf("non-file target accepted: %q %+v %v", path, result, err)
			}
			if !strings.Contains(result.Summary, "exact repo-relative") {
				t.Fatalf("lost exact-path repair: %s", result.Summary)
			}
		})
	}
}
