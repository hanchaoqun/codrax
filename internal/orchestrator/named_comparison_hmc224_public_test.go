package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc224ComparisonScope(paths [2]string) *types.RuntimeArtifactScopeProfile {
	a, b, c, d := 1.0, 2.0, 4.0, 4.5
	first := types.RuntimeArtifactComparisonSide{Path: paths[0], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{TimeStart: &a, TimeEnd: &b, SourceQuote: fmt.Sprintf("`%s` from 1 to 2 seconds", paths[0])}}
	second := types.RuntimeArtifactComparisonSide{Path: paths[1], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{TimeStart: &c, TimeEnd: &d, SourceQuote: fmt.Sprintf("`%s` from 4 to 4.5 seconds", paths[1])}}
	return &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, Comparison: &types.RuntimeArtifactComparisonScope{Baseline: first, Current: second}, TimeWindows: []types.RuntimeArtifactTimeWindow{first.RuntimeArtifactTimeWindow, second.RuntimeArtifactTimeWindow}}
}

func TestHMC224NamedComparisonPreparationIsolationPublicRun(t *testing.T) {
	for _, mode := range []string{"bad_first", "bad_second", "missing_first", "missing_second", "independent_symlinks", "local_file_urls", "no_binding", "both_bad", "global_cancel", "symlink_alias", "hardlink_alias", "extra_input"} {
		t.Run(mode, func(t *testing.T) {
			repo := t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			good, _ := hmc223AcceptanceCopyCapture(t, "current.data")
			bad := filepath.Join(t.TempDir(), "empty.htrace")
			if err := os.WriteFile(bad, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(mode, "missing_") {
				bad = filepath.Join(t.TempDir(), "missing.data")
			}
			paths := [2]string{bad, good}
			if mode == "independent_symlinks" {
				for i, path := range paths {
					alias := filepath.Join(t.TempDir(), "capture.data")
					if err := os.Symlink(path, alias); err != nil {
						t.Fatal(err)
					}
					paths[i] = alias
				}
				bad = paths[0]
			}
			if mode == "local_file_urls" {
				paths = [2]string{"file://" + bad, "file://" + good}
				bad = paths[0]
			}
			if mode == "bad_second" || mode == "missing_second" {
				paths = [2]string{good, bad}
			}
			if mode == "both_bad" {
				paths[1] = filepath.Join(t.TempDir(), "other.htrace")
				if err := os.WriteFile(paths[1], nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "symlink_alias" || mode == "hardlink_alias" {
				paths[0], paths[1] = good, filepath.Join(t.TempDir(), "alias.data")
				var err error
				if mode == "symlink_alias" {
					err = os.Symlink(good, paths[1])
				} else {
					err = os.Link(good, paths[1])
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			scope := hmc224ComparisonScope(paths)
			if mode == "bad_second" || mode == "missing_second" {
				a, b, c, d := 4.0, 4.5, 1.0, 2.0
				scope.Comparison.Baseline.RuntimeArtifactTimeWindow = types.RuntimeArtifactTimeWindow{TimeStart: &a, TimeEnd: &b, SourceQuote: fmt.Sprintf("`%s` from 4 to 4.5 seconds", paths[0])}
				scope.Comparison.Current.RuntimeArtifactTimeWindow = types.RuntimeArtifactTimeWindow{TimeStart: &c, TimeEnd: &d, SourceQuote: fmt.Sprintf("`%s` from 1 to 2 seconds", paths[1])}
				scope.TimeWindows = []types.RuntimeArtifactTimeWindow{scope.Comparison.Baseline.RuntimeArtifactTimeWindow, scope.Comparison.Current.RuntimeArtifactTimeWindow}
			}
			request := fmt.Sprintf("Compare %s and %s.", scope.Comparison.Baseline.SourceQuote, scope.Comparison.Current.SourceQuote)
			if mode == "extra_input" {
				extra := filepath.Join(t.TempDir(), "third.htrace")
				if err := os.WriteFile(extra, nil, 0600); err != nil {
					t.Fatal(err)
				}
				request += " Also inspect `" + extra + "`."
			}
			args := map[string]any{}
			for i, side := range scope.Comparison.Sides() {
				args[[]string{"baseline", "current"}[i]] = map[string]any{"source": "path", "path": side.Path, "view": "measurements", "time_start": *side.TimeStart, "time_end": *side.TimeEnd}
			}
			params, _ := json.Marshal(map[string]any{"comparison": args})
			adapter := &hmc223AcceptanceAdapter{call: &llm.ToolCall{ID: "native-independent-pair", Name: "trace_query", Params: params}}
			if mode == "no_binding" {
				scope.Comparison = nil
			}
			bus, output, err := hmc223AcceptanceRun(t, repo, request, true, adapter, nil, mode == "global_cancel", scope)
			if mode != "bad_first" && mode != "bad_second" && mode != "independent_symlinks" && mode != "local_file_urls" && !strings.HasPrefix(mode, "missing_") {
				if err == nil || output != nil || adapter.visible != nil {
					t.Fatalf("atomic/cancelled input crossed exploration: err=%v output=%+v", err, output)
				}
				if mode == "both_bad" && (!strings.Contains(err.Error(), paths[0]) || !strings.Contains(err.Error(), paths[1])) {
					t.Fatalf("both original errors were not retained: %v", err)
				}
				return
			}
			if bus == nil || output == nil || !adapter.visible["trace_query"] {
				t.Fatalf("healthy side never reached real native tool: %v", err)
			}
			found := false
			for _, result := range output.ToolResults {
				report, ok := result.RuntimeMeasurementPair.Report()
				if !ok {
					continue
				}
				found = true
				for i, side := range report.Sides {
					if paths[i] == bad {
						if side.Status != "failed" || len(side.Publications) != 0 {
							t.Fatalf("bad side published facts: %+v", side)
						}
						continue
					}
					rows := 0
					for _, pub := range side.Publications {
						for _, table := range pub.Tables {
							if table.View == types.RuntimeMeasurementMembers {
								rows += len(table.Rows)
							}
						}
					}
					if side.Status != "available" || rows != 8 {
						t.Fatalf("healthy source did not retain all eight rows: %+v rows=%d", side, rows)
					}
				}
			}
			if !found {
				t.Fatal("real query produced no pair")
			}
			catalogs := bus.Mutable.TraceCatalogs()
			if len(catalogs) != 1 || len(catalogs[0].Snapshot().Artifacts) != 2 {
				t.Fatal("exact two-source navigation missing")
			}
			failed := 0
			for _, a := range catalogs[0].Snapshot().Artifacts {
				for _, issue := range a.Issues {
					if issue.Code == "preparation_failed" {
						failed++
					}
				}
			}
			if failed != 1 {
				t.Fatalf("original preparation failure lost: %d", failed)
			}
		})
	}
}
