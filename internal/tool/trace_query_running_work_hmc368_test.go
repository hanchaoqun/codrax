package tool

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC368PublicRunningWorkZeroAndGeneration(t *testing.T) {
	// Swap only the business marker. Both physical generations have an
	// identical native state partition, so a partition ID alone cannot
	// establish that their different work measurements are comparable.
	const original = `# tracer: nop
swapper-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=7 next_prio=120
worker-7 (7) [000] .... 1.004000: tracing_mark_write: B|7|LaunchStage Example
worker-7 (7) [000] .... 1.007000: tracing_mark_write: E|7
worker-7 (7) [000] .... 1.020000: sched_switch: prev_comm=worker prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120
`
	ctx, path := businessRefTestContext(t, original)
	query := func(view string) types.ObservationRecord {
		t.Helper()
		r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": view, "pid": 7,
			"time_start": 1.001, "time_end": 1.015, "target_scope": "thread"})
		for _, o := range r.Observations {
			if o.Predicate == "target_window_states" {
				return o
			}
		}
		t.Fatalf("%s missing exact account: %s", view, r.Summary)
		return types.ObservationRecord{}
	}
	zero, unknown := query("window_stats"), query("thread_timeline")
	z, zok := types.TraceCausalProjectionTargetStateAccountFromRecord(zero)
	u, uok := types.TraceCausalProjectionTargetStateAccountFromRecord(unknown)
	if !zok || !uok || z.DeterministicRunningStatus != types.TraceRunningWorkMeasured || z.DeterministicRunningMS != 0 ||
		u.DeterministicRunningStatus != types.TraceRunningWorkNotMeasured || u.DeterministicRunningMS != 0 ||
		!strings.Contains(zero.Summary, "deterministic_running=0.000ms") || !strings.Contains(unknown.Summary, "deterministic_running=not_measured") {
		t.Fatalf("no observed distinction between a native zero and no measurement: %+v / %+v", z, u)
	}
	for _, zh := range []bool{true, false} {
		m := runtimeTraceProjTreeModel{}
		zline, uline := runtimeTraceProjFourStateRunningLine(&z, m, zh), runtimeTraceProjFourStateRunningLine(&u, m, zh)
		if !strings.Contains(zline, "0.000ms") || zline == uline || strings.Contains(uline, "0.000ms") {
			t.Fatalf("rendered account conflated zero with unmeasured: %q / %q", zline, uline)
		}
		conflict := z
		conflict.DeterministicRunningStatus = types.TraceRunningWorkConflict
		if line := runtimeTraceProjFourStateRunningLine(&conflict, m, zh); line == "" || line == zline || line == uline || strings.Contains(line, "0.000ms") {
			t.Fatalf("rendered conflict minted a numeric subdivision: %q", line)
		}
	}
	changed := strings.Replace(original, "LaunchStage", "VerifyClass", 1)
	replacement := filepath.Join(filepath.Dir(path), "new-generation.tmp")
	if err := os.WriteFile(replacement, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	positive := query("window_stats")
	n, nok := types.TraceCausalProjectionTargetStateAccountFromRecord(positive)
	if !nok || n.DeterministicRunningStatus != types.TraceRunningWorkMeasured || math.Abs(n.DeterministicRunningMS-3) > 1e-8 ||
		unknown.MeasurementSources == nil || positive.MeasurementSources == nil ||
		unknown.MeasurementSources.Domains[0] != positive.MeasurementSources.Domains[0] {
		t.Fatalf("physical generations did not independently produce the expected measurements on the same partition: %+v", n)
	}
	scope := func(r types.ObservationRecord) string {
		for _, note := range r.RichNotes {
			if v, ok := strings.CutPrefix(note, types.TraceNoteKeyDeterministicRunningScopeID+"="); ok {
				return v
			}
		}
		return ""
	}
	if scope(zero) == "" || scope(zero) != scope(unknown) || scope(positive) == "" || scope(positive) == scope(unknown) {
		t.Fatal("scope did not match independent same-generation views or survived a physical replacement")
	}
}

func TestHMC368HoneybirdRunningWorkUsesSameNativePartition(t *testing.T) {
	path := os.Getenv("CODRAX_HMC365_HONEYBIRD")
	if path == "" {
		t.Skip("requires the complete Honeybird ftrace")
	}
	if h, n := hmc176bRepresentativeDigest(t, path); h != "21280a96618781b37be37f9289be9ba286d6a85d358e60ff72d9c73c827cf30e" || n != 106896120 {
		t.Fatal("not the original capture")
	}
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	var observations []types.ObservationRecord
	var missing, measured types.ObservationRecord
	for _, view := range []string{"thread_timeline", "critical_blocking_calls", "root_cause_rank", "window_stats", "wakeup_chain"} {
		result := hmc17NamedQuery(t, ctx, map[string]any{
			"source": "path", "path": path, "view": view, "pid": 64156, "target_scope": "thread",
			"time_start": 11343.248, "time_end": 11343.310,
		})
		found := false
		for _, row := range result.Observations {
			if row.Predicate != "target_window_states" {
				continue
			}
			found = true
			switch view {
			case "thread_timeline", "critical_blocking_calls":
				if strings.Contains(row.Summary, "deterministic_running=0.000ms") {
					t.Errorf("%s published unmeasured work as zero: %s", view, row.Summary)
				}
				missing = row
			default:
				if !strings.Contains(row.Summary, "deterministic_running=3.868ms") {
					t.Errorf("%s lost the measured running-work value: %s", view, row.Summary)
				}
				measured = row
			}
			break
		}
		if !found {
			t.Fatalf("%s did not publish its native partition", view)
		}
		observations = append(observations, result.Observations...)
	}
	if missing.MeasurementSources == nil || measured.MeasurementSources == nil ||
		len(missing.MeasurementSources.Domains) != 1 || len(measured.MeasurementSources.Domains) != 1 ||
		missing.MeasurementSources.Domains[0] != measured.MeasurementSources.Domains[0] {
		t.Fatalf("expected one exact partition behind the independent queries: %v / %v", missing.MeasurementSources, measured.MeasurementSources)
	}
	for i := 0; i < 2; i++ {
		got := types.TraceCausalProjectionFromObservationRecords(observations).TargetStateAccount
		if got == nil || math.Abs(got.DeterministicRunningMS-3.868) > 1e-6 ||
			math.Abs(got.RunningMS-15.218) > 1e-6 || math.Abs(got.RunnableMS-0.090) > 1e-6 ||
			math.Abs(got.SleepMS-46.692) > 1e-6 {
			t.Errorf("missing/refined selection changed the four-state partition or lost measured work: %+v", got)
		}
		for l, r := 0, len(observations)-1; l < r; l, r = l+1, r-1 {
			observations[l], observations[r] = observations[r], observations[l]
		}
	}
}
