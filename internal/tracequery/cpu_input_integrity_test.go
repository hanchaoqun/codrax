package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPUInputStrictScalarPresenceAndAdmission(t *testing.T) {
	intern := newStringInterner()
	cases := []struct {
		name    string
		cpuID   string
		present bool
		valid   bool
		cpu     int
	}{
		{name: "absent falls back", cpuID: "", present: false, valid: false, cpu: 3},
		{name: "zero", cpuID: "0", present: true, valid: true, cpu: 0},
		{name: "upper boundary", cpuID: "4095", present: true, valid: true, cpu: 4095},
		{name: "negative", cpuID: "-1", present: true, valid: false, cpu: -1},
		{name: "bad token", cpuID: "foo", present: true, valid: false, cpu: -1},
		{name: "above boundary", cpuID: "4096", present: true, valid: false, cpu: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := `idle-0 (0) [003] .... 1.000000: cpu_frequency: state=1200000`
			if tc.cpuID != "" {
				line += " cpu_id=" + tc.cpuID
			}
			ev, ok := ParseLine(1, line, intern)
			if !ok {
				t.Fatal("frequency row should remain searchable")
			}
			if ev.CPUForFieldPresent != tc.present || ev.CPUForFieldValid != tc.valid {
				t.Fatalf("presence/validity drift: %+v", ev)
			}
			if got := eventCPUForStats(ev); got != tc.cpu {
				t.Fatalf("eventCPUForStats=%d want %d", got, tc.cpu)
			}
			if got := isPerCPUFrequencySample(ev); got != (tc.cpu >= 0) {
				t.Fatalf("frequency admission=%t cpu=%d", got, tc.cpu)
			}
		})
	}

	limit := Event{Type: EventCPUFrequencyLimit, FrequencyMax: 1200000, CPU: 0, CPUForField: -1, CPUForFieldPresent: true}
	if _, ok := isPerCPULimitSample(limit); ok {
		t.Fatal("invalid explicit cpu_id must not fall back to header cpu0 for limits")
	}
}

func TestCPUInputHeaderAndPerfCPUAreGloballyBounded(t *testing.T) {
	intern := newStringInterner()
	invalidHeader := `app-20 (20) [4096] .... 1.000000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120`
	if _, ok := ParseLine(1, invalidHeader, intern); ok {
		t.Fatal("out-of-range row-header CPU must reject the whole attributed event")
	}
	failures := cpuInputValidationFailures(1, invalidHeader)
	if len(failures) != 1 || failures[0].Field != "header_cpu" {
		t.Fatalf("missing global header CPU witness: %+v", failures)
	}
	validHeader := `app-20 (20) [4095] .... 1.000000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120`
	if _, ok := ParseLine(1, validHeader, intern); !ok {
		t.Fatal("legal row-header CPU boundary rejected")
	}

	perf, ok := ParseLine(2, `perf-20 (20) [001] .... 1.100000: perf_sample: cpu=99999 pid=20 tid=20 period=1`, intern)
	if !ok || perf.PerfFields == nil {
		t.Fatal("perf inventory row should remain searchable")
	}
	if perf.CPU != -1 || perf.PerfFields.CPUKnown == nil || *perf.PerfFields.CPUKnown {
		t.Fatalf("invalid perf payload CPU gained attribution: %+v", perf)
	}
	if failures := cpuInputValidationFailures(2, `perf-20 (20) [001] .... 1.100000: perf_sample: cpu=99999 pid=20 tid=20 period=1`); len(failures) != 1 || failures[0].Field != "cpu" {
		t.Fatalf("missing perf CPU witness: %+v", failures)
	}
}

func TestPerfCPUExactMinusOneNoClaimDoesNotMintInvalidWitness(t *testing.T) {
	intern := newStringInterner()
	noClaim := `perf-20 (20) [001] .... 1.100000: perf_sample: cpu=-1 cpu_known=false pid=20 tid=20 period=1 sample_kind=unknown`
	ev, ok := ParseLine(1, noClaim, intern)
	if !ok || ev.PerfFields == nil || ev.CPU != -1 || ev.CPUInputInvalid || ev.PerfFields.CPUKnown == nil || *ev.PerfFields.CPUKnown {
		t.Fatalf("exact perf CPU no-claim was not normalized cleanly: %+v", ev)
	}
	if failures := cpuInputValidationFailures(1, noClaim); len(failures) != 0 {
		t.Fatalf("exact perf CPU no-claim minted invalid witness: %+v", failures)
	}
	path := filepath.Join(t.TempDir(), "perf_no_claim.systrace")
	if err := os.WriteFile(path, []byte(noClaim+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || len(idx.cpuInputIntegrityFailures) != 0 {
		t.Fatalf("index path converted legal perf no-claim into degradation: events=%+v failures=%+v", idx.Events, idx.cpuInputIntegrityFailures)
	}
	placeholder := `perf-20 (20) [000] .... 1.100000: perf_sample: cpu=0 cpu_known=false pid=20 tid=20 period=1 sample_kind=unknown`
	placeholderEvent, ok := ParseLine(2, placeholder, intern)
	if !ok || placeholderEvent.CPU != -1 || placeholderEvent.CPUInputInvalid || perfSampleHasKnownCPU(placeholderEvent) {
		t.Fatalf("cpu_known=false did not scrub a valid transport placeholder: %+v", placeholderEvent)
	}
	if failures := cpuInputValidationFailures(2, placeholder); len(failures) != 0 {
		t.Fatalf("valid numeric transport placeholder was reclassified as malformed CPU: %+v", failures)
	}

	for _, row := range []string{
		`perf-20 (20) [001] .... 1.100000: perf_sample: cpu=-1 cpu_known=true pid=20 tid=20 period=1`,
		`perf-20 (20) [001] .... 1.100000: perf_sample: cpu=-1 cpu_known=FALSE pid=20 tid=20 period=1`,
		`perf-20 (20) [001] .... 1.100000: perf_sample: cpu=-1 cpu_known=unknown pid=20 tid=20 period=1`,
		`perf-20 (20) [001] .... 1.100000: perf_sample: cpu=-2 cpu_known=false pid=20 tid=20 period=1`,
		`perf-20 (20) [001] .... 1.100000: perf_sample: cpu=bad cpu_known=false pid=20 tid=20 period=1`,
	} {
		failures := cpuInputValidationFailures(2, row)
		if len(failures) != 1 || failures[0].Field != "cpu" {
			t.Fatalf("non-canonical perf CPU no-claim bypassed validation: row=%q failures=%+v", row, failures)
		}
	}

	otherFamily := `idle-0 (0) [001] .... 1.100000: cpu_frequency: cpu_id=-1 cpu_known=false state=1200000`
	if failures := cpuInputValidationFailures(3, otherFamily); len(failures) != 1 || failures[0].Field != "cpu_id" {
		t.Fatalf("perf-only no-claim exception leaked to another family: %+v", failures)
	}
}

func TestCPURangeStrictRejectsCoercionAndBoundsExpansion(t *testing.T) {
	for _, raw := range []string{"foo", "-1", "3-1", "1--2", "0-4095", "4096"} {
		if got := parseCPURangeList(raw); len(got) != 0 {
			t.Fatalf("%q must fail closed, got %v", raw, got)
		}
	}
	if got := parseCPURangeList("4095"); len(got) != 1 || got[0] != 4095 {
		t.Fatalf("legal scalar boundary lost: %v", got)
	}
	if got := parseCPURangeList("0-1023"); len(got) != maxTraceCPUSetExpansion || got[0] != 0 || got[len(got)-1] != 1023 {
		t.Fatalf("legal expansion boundary lost: len=%d", len(got))
	}
	if topology := parseCoreTopology("small=foo;big=4-7"); len(topology) != 4 || topology[0] != "" || topology[4] != "big" {
		t.Fatalf("bad token must not become cpu0 while healthy entry survives: %v", topology)
	}
	if domains := parseClusterFreqDomains("small=3-1;big=4-7"); domains.byCPU[0] != "" || len(domains.byCPU) != 4 {
		t.Fatalf("frequency-domain parser admitted reversed range: %+v", domains)
	}
}

func TestCPUConstraintRosterAllOrNothing(t *testing.T) {
	intern := newStringInterner()
	bad, ok := ParseLine(1, `app-20 (20) [001] .... 1.000000: sched_setaffinity: comm=app pid=20 allowed_cpus=0-2,foo target_cpu=-1`, intern)
	if !ok || bad.ConstraintFields == nil {
		t.Fatal("constraint inventory row should remain searchable")
	}
	cf := bad.ConstraintFields
	if !cf.CPUPresent || cf.CPUValid || !cf.AllowedPresent || cf.AllowedValid || len(cf.Allowed) != 0 {
		t.Fatalf("invalid constraint leaked through presence/validity: %+v", cf)
	}

	good, ok := ParseLine(2, `app-20 (20) [001] .... 1.100000: sched_setaffinity: comm=app pid=20 allowed_cpus=0-2,4095 target_cpu=0`, intern)
	if !ok || !good.ConstraintFields.CPUValid || !good.ConstraintFields.AllowedValid {
		t.Fatalf("valid constraint rejected: %+v", good.ConstraintFields)
	}
	if got := good.ConstraintFields.Allowed; len(got) != 4 || got[0] != 0 || got[3] != 4095 {
		t.Fatalf("valid roster drifted: %v", got)
	}
}

func TestCPUInputMigrateValidationParity(t *testing.T) {
	intern := newStringInterner()
	for _, row := range []string{
		`app-20 (20) [001] .... 1.000000: sched_migrate_task: comm=app pid=20 prio=20 orig_cpu=-1 dest_cpu=2`,
		`app-20 (20) [001] .... 1.000000: sched_migrate_task: comm=app pid=20 prio=20 orig_cpu=1 dest_cpu=foo`,
		`app-20 (20) [001] .... 1.000000: sched_migrate_task: comm=app pid=20 prio=20 orig_cpu=1 dest_cpu=4096`,
	} {
		if failure := schedulerRowValidationFailure(1, row); failure == nil {
			t.Fatalf("scheduler validator accepted invalid migration: %s", row)
		}
		if _, ok := ParseLine(1, row, intern); ok {
			t.Fatalf("ParseLine admitted migration rejected by validator: %s", row)
		}
		if len(cpuInputValidationFailures(1, row)) == 0 {
			t.Fatalf("missing typed CPU witness: %s", row)
		}
	}
	valid := `app-20 (20) [001] .... 1.000000: sched_migrate_task: comm=app pid=20 prio=20 orig_cpu=0 dest_cpu=4095`
	if failure := schedulerRowValidationFailure(1, valid); failure != nil {
		t.Fatalf("valid boundary migration rejected: %s", failure.reason())
	}
	ev, ok := ParseLine(1, valid, intern)
	if !ok {
		t.Fatal("valid migration failed parse")
	}
	pid, cpu, _, migrated := schedMigrationTarget(ev)
	if !migrated || pid != 20 || cpu != 4095 {
		t.Fatalf("valid migration attribution drifted: pid=%d cpu=%d ok=%t", pid, cpu, migrated)
	}
}

func TestCPUInputFailureWitnessAndFrequencyCensusExclusion(t *testing.T) {
	trace := strings.Join([]string{
		`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=900000 cpu_id=foo`,
		`idle-0 (0) [000] .... 1.100000: cpu_frequency: state=1000000 cpu_id=-1`,
		`idle-0 (0) [001] .... 1.200000: cpu_frequency: state=1200000 cpu_id=1`,
		`idle-0 (0) [001] .... 1.300000: cpu_frequency: state=1400000 cpu_id=1`,
		`idle-0 (0) [000] .... 1.400000: cpu_frequency_limits: min=400000 max=800000 cpu_id=-1`,
	}, "\n") + "\n"
	path := filepath.Join(t.TempDir(), "invalid_cpu.systrace")
	if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.cpuInputIntegrityFailures) != 3 {
		t.Fatalf("typed failures=%+v", idx.cpuInputIntegrityFailures)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 2})
	if !containsSubstring(stats.Caveats, "cpu_input_integrity_degraded=true") {
		t.Fatalf("missing bounded integrity caveat: %v", stats.Caveats)
	}
	if len(stats.CPUFrequencyLimits) != 0 {
		t.Fatalf("invalid limit entered policy attribution: %+v", stats.CPUFrequencyLimits)
	}
	for _, cpu := range stats.CPU {
		if cpu.CPU == 0 && len(cpu.FrequencyResidency) > 0 {
			t.Fatalf("invalid cpu_id fell back into cpu0 residency: %+v", cpu)
		}
	}
	q := Query{EventTypes: []EventType{EventCPUFrequency}, Limit: 1}
	displayed := EventSearch(idx, q)
	census := ComputeCPUFrequencyCensus(idx, q, displayed)
	if census == nil || census.MatchedFrequencyRows != 2 || len(census.CPUs) != 1 || census.CPUs[0] != 1 {
		t.Fatalf("invalid frequency rows polluted census: %+v", census)
	}
}
