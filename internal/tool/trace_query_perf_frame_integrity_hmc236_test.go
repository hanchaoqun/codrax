package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPerfFrameIntegrityPublicHandoffHMC236(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "perf_stats", "pid": 11, "time_start": 1, "time_end": 2})
	if !r.Success {
		t.Fatal(r.Summary)
	}
	row, ok := types.TraceNoteKeyLookup("perf_value_basis")
	if !ok || row.Carrier != types.TraceNoteCarrierDisplayOnly || row.Family != "perf" {
		t.Fatal("flat sample caliber must be registered without causal authority")
	}
	for _, want := range []string{"callchain_form=exact_frames", "callchain_form=opaque_flat_label", "frame_reason=legacy_frame_boundaries_unavailable", "callchain_known=2 callchain_unknown=1", "frame_statuses=", "value_basis=recorded_sample_fields"} {
		if !strings.Contains(r.Summary, want) {
			t.Errorf("model tool face lost %q", want)
		}
	}
	found, evidenceFound := 0, 0
	for _, observation := range r.Observations {
		if strings.Contains(observation.ID, "#evidence_fact:") && observation.Predicate == "perf_sample_top_symbol" && observation.Object == "/app/A.so" && strings.Contains(observation.Summary, "sample_weight=15 denominator=35") {
			evidenceFound++
			options := types.DefaultObservationPromptProjectionOptions(1)
			options.SummaryMaxLen = 140
			projected := types.ProjectObservationPromptRecords([]types.ObservationRecord{observation}, nil, nil, options)
			if len(projected) != 1 || !strings.Contains(projected[0].Summary, "sample_weight=15 denominator=35 value_basis=recorded_sample_fields_not_prefix_node") {
				t.Errorf("generic evidence compact prefix lost sampling caliber: %+v", projected)
			}
		}
		if observation.Predicate != "perf_sample_top_symbol" || observation.Object != "/app/A.so" || observation.Value != "15" {
			continue
		}
		found++
		projected := types.ProjectObservationPromptRecords([]types.ObservationRecord{observation}, nil, nil, types.DefaultObservationPromptProjectionOptions(1))
		if len(projected) != 1 || !strings.Contains(strings.Join(projected[0].Notes, " "), "recorded_sample_fields_not_prefix_node") {
			t.Errorf("finalizer compact face can upgrade flat 15 to path self weight: %+v", projected)
		}
	}
	if found == 0 {
		t.Fatal("flat observed A=15 must not be reduced to exact-frame A=10")
	}
	if evidenceFound == 0 {
		t.Fatal("public generic evidence path was not exercised")
	}
}

func TestPerfQualityCaptureIssueSurvivesPromptBudgetHMC236(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture-quality.perftrace")
	body := "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 cpu=0 cpu_known=true sample_kind=on_cpu sample_weight=99 event=cycles symbol=work dso=a.so callchain=\"root;work\" source=raw callchain_status=symbolized parser_caveats=\"lost_events_status=aggregate_overflow; lost_samples=5\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "perf_stats", "pid": 11, "time_start": 1, "time_end": 2})
	if !r.Success {
		t.Fatal(r.Summary)
	}
	for _, observation := range r.Observations {
		if observation.Predicate != "perf_sample_top_symbol" || observation.Value != "99" {
			continue
		}
		projected := types.ProjectObservationPromptRecords([]types.ObservationRecord{observation}, nil, nil, types.DefaultObservationPromptProjectionOptions(1))
		if len(projected) != 1 || !strings.Contains(strings.Join(projected[0].Notes, " "), "lost_events_status=aggregate_overflow") {
			t.Fatalf("generic frame teaching consumed the capture-quality note budget: %+v", projected)
		}
		if !strings.Contains(strings.Join(observation.RichNotes, " "), "not proof of exact frames or complete unwinding") {
			t.Fatal("reordering must retain native frame-boundary teaching")
		}
		return
	}
	t.Fatal("public sampling quality observation was not emitted")
}
