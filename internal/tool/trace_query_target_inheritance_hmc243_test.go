package tool

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC243TypedTargetInheritancePublicMatrix(t *testing.T) {
	user := func(kind types.RuntimeTargetKind, pid int, name string) types.RuntimeTarget {
		return types.RuntimeTarget{Kind: kind, PID: pid, Thread: name, Source: "user_explicit"}
	}
	cursor := func(pid int, name string) types.RuntimeTarget {
		return types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: pid, Thread: name, Source: types.RuntimeTargetSourceExplicitToolCall}
	}
	thread, process := types.RuntimeTargetKindThread, types.RuntimeTargetKindProcess
	for _, tc := range []struct {
		name      string
		targets   []types.RuntimeTarget
		pid       int
		nameWant  string
		inherited bool
	}{
		{"named-user-bare-cursor", []types.RuntimeTarget{user(thread, 11, "document-worker-11"), cursor(11, "")}, 11, "document-worker-11", true},
		{"bare-cursor-named-user", []types.RuntimeTarget{cursor(11, ""), user(thread, 11, "document-worker-11")}, 11, "document-worker-11", true},
		{"equivalent-decorations", []types.RuntimeTarget{user(thread, 11, "document-worker-11"), cursor(11, "document-worker")}, 11, "document-worker-11", true},
		{"conflicting-names", []types.RuntimeTarget{user(thread, 11, "document-worker"), cursor(11, "other")}, 11, "", true},
		{"conflicting-name-not-restored", []types.RuntimeTarget{user(thread, 11, "document-worker"), cursor(11, "other"), cursor(11, "document-worker")}, 11, "", true},
		{"conflicting-decorated-id", []types.RuntimeTarget{user(thread, 11, "worker [12]"), cursor(11, "")}, 11, "", true},
		{"matching-bracket-literal-name-suffix", []types.RuntimeTarget{user(thread, 11, "worker-3 [11]"), cursor(11, "worker-3 (11)")}, 11, "worker-3 [11]", true},
		{"matching-bracket-literal-name", []types.RuntimeTarget{user(thread, 11, "worker-3 [11]"), cursor(11, "")}, 11, "worker-3 [11]", true},
		{"conflicting-explicit-carriers", []types.RuntimeTarget{user(thread, 11, "worker tid=11 [12]"), cursor(11, "")}, 11, "", true},
		{"different-ids", []types.RuntimeTarget{user(thread, 11, "document-worker"), cursor(12, "")}, 0, "", false},
		{"same-name-different-ids", []types.RuntimeTarget{user(thread, 11, "worker"), user(thread, 12, "worker")}, 0, "", false},
		{"thread-process-not-one-id", []types.RuntimeTarget{user(thread, 11, ""), user(process, 11, "")}, 0, "", false},
		{"process-not-thread", []types.RuntimeTarget{user(process, 11, "")}, 0, "", false},
		{"name-only", []types.RuntimeTarget{user(thread, 0, "document-worker"), cursor(0, "document-worker")}, 0, "document-worker", true},
		{"name-only-ambiguous", []types.RuntimeTarget{user(thread, 0, "document-worker"), cursor(0, "other")}, 0, "", false},
		{"name-only-not-numeric-proof", []types.RuntimeTarget{user(thread, 0, "document-worker"), cursor(11, "document-worker")}, 0, "", false},
		{"unknown-kind-not-thread-proof", []types.RuntimeTarget{user(types.RuntimeTargetKindUnknown, 11, ""), cursor(11, "")}, 0, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
			if err != nil {
				t.Fatal(err)
			}
			rm := types.RequestModel{RuntimeTargets: append([]types.RuntimeTarget(nil), tc.targets...)}
			ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("typed request"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
			ctx.Mutable.SetRequestModel(rm)
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 1, "time_end": 2})
			res, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !res.Success {
				t.Fatalf("query: %v %s", err, res.Summary)
			}
			if got := strings.Contains(res.Summary, "trace_query_target_inherited=true"); got != tc.inherited {
				t.Fatalf("inherited=%t want=%t: %s", got, tc.inherited, res.Summary)
			}
			if tc.inherited {
				target, ok := traceQuerySingleRuntimeTargetFromRequestModel(&rm)
				if !ok || target.PID != tc.pid || target.Thread != tc.nameWant || target.Source != "user_explicit" {
					t.Fatalf("selection/provenance: %+v ok=%t", target, ok)
				}
			}
			if !reflect.DeepEqual(ctx.AnalysisIR.RequestModel.RuntimeTargets, tc.targets) || !reflect.DeepEqual(ctx.Mutable.RequestModel().RuntimeTargets, tc.targets) {
				t.Fatal("default query rewrote original identity/cursor roster")
			}
		})
	}
}

func TestHMC243TypedTargetInheritanceExplicitAndSupplement(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, supplement := range []bool{false, true} {
		ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("typed request"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 11, Thread: "document-worker", Source: "user_explicit"}}}}}
		ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
		if supplement {
			ctx.Mutable.BeginSystemTraceSupplementExecution()
			defer ctx.Mutable.EndSystemTraceSupplementExecution()
		}
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 12, "time_start": 1, "time_end": 2})
		res, err := (&TraceQuery{}).Execute(ctx, params)
		if err != nil || !res.Success || strings.Contains(res.Summary, "trace_query_target_inherited=true") || !strings.Contains(res.Summary, "pid=12") {
			t.Fatalf("explicit selector changed supplement=%t: %v %s", supplement, err, res.Summary)
		}
		want := 2
		if supplement {
			want = 1
		}
		if targets := ctx.AnalysisIR.RequestModel.RuntimeTargets; len(targets) != want || targets[0].PID != 11 || targets[0].Source != "user_explicit" {
			t.Fatalf("supplement=%t cursor provenance mutated: %+v", supplement, targets)
		}
	}
}

func TestHMC243TypedTargetInheritanceRequestCopies(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	thread := types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 11, Source: "user_explicit"}
	process := thread
	process.Kind = types.RuntimeTargetKindProcess
	other := thread
	other.PID = 12
	invalid := thread
	invalid.PID = types.RuntimeTargetMaxPID + 1
	for _, tc := range []struct {
		name    string
		current []types.RuntimeTarget
		want    bool
	}{
		{"current-multiple-not-stale-single", []types.RuntimeTarget{thread, other}, false},
		{"current-cross-kind-not-stale-single", []types.RuntimeTarget{thread, process}, false},
		{"current-invalid-not-stale-single", []types.RuntimeTarget{invalid}, false},
		{"inactive-current-keeps-legacy-mutable", []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread}}, true},
		{"only-mutable", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("current request"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeTargets: tc.current}}}
			ctx.Mutable.SetRequestModel(types.RequestModel{RuntimeTargets: []types.RuntimeTarget{thread}})
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 1, "time_end": 2})
			res, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !res.Success {
				t.Fatalf("query: %v %s", err, res.Summary)
			}
			if got := strings.Contains(res.Summary, "trace_query_target_inherited=true"); got != tc.want {
				t.Errorf("current request inherited=%t want=%t", got, tc.want)
			}
		})
	}
}
