package types

import "testing"

func TestHMC368RunningWorkRefinementRequiresExactGenerationAndPartition(t *testing.T) {
	row := func(id, measurement string) ObservationRecord {
		out := cov4StateRecord(id, "target-61", "selected_window=100.000000..100.100000", 100)
		out.RichNotes = []string{"selected_window=100.000000..100.100000", "running=40.000",
			"runnable=10.000", "sleep=50.000", "total=100.000",
			"deterministic_running_scope_id=running_work:v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
		if measurement != "" {
			out.RichNotes = append(out.RichNotes, "deterministic_running_status=measured", "deterministic_running="+measurement)
		}
		out.SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "/capture/a.ftrace",
			PayloadRef: id + ".json", QueryScopeID: id, QueryWindowKnown: true, QueryWindowStartTs: 100,
			QueryWindowEndTs: 100.1, QueryLineRangeKnown: true}
		out.MeasurementSources = TraceSchedulerMeasurementSourcesFromDomain(&TraceSchedulerMeasurementDomain{
			Version: 1, Status: "constructed_partition", Method: "thread_timeline", TargetTID: 61,
			WindowStartTs: 100, WindowEndTs: 100.1, PartitionID: "scheduler_partition:v1:known"})
		return out
	}
	check := func(t *testing.T, wantID string, status TraceRunningWorkStatus, want float64, rows ...ObservationRecord) {
		t.Helper()
		p := TraceCausalProjection{WindowStartTs: 100, WindowEndTs: 100.1}
		cs := make([]traceCausalProjectionTargetStateCandidate, 0, len(rows))
		for _, r := range rows {
			c, ok := traceCausalProjectionTargetStateCandidateFromRecord(r)
			if !ok {
				t.Fatalf("invalid test record: %+v", r)
			}
			cs = append(cs, c)
		}
		traceCausalProjectionAttachTargetStateAccount(&p, cs)
		a := p.TargetStateAccount
		if a == nil || a.EvidenceID != wantID || a.DeterministicRunningStatus != status || a.DeterministicRunningMS != want ||
			a.RunningMS != 40 || a.RunnableMS != 10 || a.SleepMS != 50 || a.TotalMS != 100 {
			t.Fatalf("running work changed base, locator or value: %+v", a)
		}
	}
	base := row("base", "")
	base.RichNotes = append(base.RichNotes, "deterministic_running=0.000") // legacy ambiguous zero
	positive, zero := row("measured-positive", "13.75"), row("measured-zero", "0")
	check(t, "measured-positive", TraceRunningWorkMeasured, 13.75, base, positive, row("same-value", "13.75"))
	check(t, "measured-positive", TraceRunningWorkMeasured, 13.75, positive, base)
	check(t, "measured-zero", TraceRunningWorkMeasured, 0, base, zero)
	check(t, "base", TraceRunningWorkConflict, 0, base, zero, positive)
	check(t, "measured-positive", TraceRunningWorkConflict, 0, positive, zero, base)
	for name, modify := range map[string]func(*ObservationRecord){
		"another physical capture": func(r *ObservationRecord) { r.SourceRef.Path = "/capture/b.ftrace" },
		"another file generation": func(r *ObservationRecord) {
			r.RichNotes[5] = "deterministic_running_scope_id=running_work:v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		"different TID":                    func(r *ObservationRecord) { r.MeasurementSources.Domains[0].TargetTID++ },
		"equal totals different intervals": func(r *ObservationRecord) { r.MeasurementSources.Domains[0].PartitionID += "different" },
		"same rounded window distinct exact endpoint": func(r *ObservationRecord) {
			r.SourceRef.QueryWindowEndTs += 0.00000001
			r.MeasurementSources.Domains[0].WindowEndTs = r.SourceRef.QueryWindowEndTs
		},
		"unknown contributor":          func(r *ObservationRecord) { r.MeasurementSources.HasUnknown = true },
		"contradictory totals":         func(r *ObservationRecord) { r.RichNotes[1], r.RichNotes[2] = "running=39.000", "runnable=11.000" },
		"untrusted result":             func(r *ObservationRecord) { r.SourceRef.PayloadRef = "" },
		"missing value":                func(r *ObservationRecord) { r.RichNotes = r.RichNotes[:len(r.RichNotes)-1] },
		"non-finite value":             func(r *ObservationRecord) { r.RichNotes[len(r.RichNotes)-1] = "deterministic_running=NaN" },
		"intersection exceeds running": func(r *ObservationRecord) { r.RichNotes[len(r.RichNotes)-1] = "deterministic_running=41" },
		"conflicting fields":           func(r *ObservationRecord) { r.RichNotes = append(r.RichNotes, "deterministic_running=22") },
	} {
		t.Run(name, func(t *testing.T) {
			other := row("other", "20")
			modify(&other)
			check(t, "base", "", 0, base, other)
		})
	}
}
