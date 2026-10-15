package tracequery

import (
	"math"
	"strconv"
	"testing"
)

func TestHMC245RecordedCPUCoordinateDoesNotGrantExecution(t *testing.T) {
	for _, kind := range []string{"", "unknown", "off_cpu", "on_cpu"} {
		for _, known := range []bool{false, true} {
			for _, cpu := range []int{-1, 0, 1, maxTraceCPUIndex + 1} {
				ev := Event{Type: EventPerfSample, CPU: cpu, PerfFields: &PerfFields{CPUKnown: &known, SampleKind: kind}}
				want := "unknown"
				if known && validTraceCPUIndex(cpu) {
					want = "cpu:" + strconv.Itoa(cpu)
				}
				if got := perfSampleRecordedCPULabel(ev); got != want {
					t.Fatalf("kind=%q known=%t cpu=%d: got %s want %s", kind, known, cpu, got, want)
				}
				if _, ok := perfSampleOnCPUExecutionCPU(ev); ok != (want != "unknown" && kind == "on_cpu") {
					t.Fatal("recorded CPU acquired execution semantics")
				}
			}
		}
	}
	if got := perfSampleRecordedCPULabel(Event{Type: EventPerfSample, CPU: 0, PerfFields: &PerfFields{}}); got != "unknown" {
		t.Fatal("missing typed CPU witness inferred zero")
	}
}

func TestHMC245RecordedCPUBoundedConservationAndOverflow(t *testing.T) {
	acc := newPerfQualityAcc()
	known := true
	for cpu := 0; cpu < 20; cpu++ {
		acc.add(Event{Type: EventPerfSample, CPU: cpu, PerfFields: &PerfFields{CPUKnown: &known, SampleKind: "unknown"}}, int64(cpu+1), "missing")
	}
	unknown := false
	acc.add(Event{Type: EventPerfSample, CPU: 0, PerfFields: &PerfFields{CPUKnown: &unknown}}, 1000, "missing")
	q := acc.summary(1210, true)
	if len(q.RecordedCPUs) != 18 || q.RecordedCPUOmittedCount != 4 || q.CPUKnownCount != 20 || q.CPUUnknownCount != 1 {
		t.Fatalf("unbounded or unknown folded: %+v", q)
	}
	count, weight := 0, int64(0)
	for _, item := range q.RecordedCPUs {
		count += item.SampleCount
		weight += item.Period
		if item.Value == "other_known_cpus" && (item.SampleCount != 4 || item.Period != 10) {
			t.Fatalf("wrong omitted known measure: %+v", item)
		}
		if item.Value == "unknown" && (item.SampleCount != 1 || item.Period != 1000) {
			t.Fatalf("unknown merged with known: %+v", item)
		}
	}
	if count != 21 || weight != 1210 {
		t.Fatalf("bounded projection changed totals: %d/%d", count, weight)
	}
	// Global cross-cohort and overflow views retain only sample counts. There
	// is no fake zero-weight claim: the existing typed status stays count-only.
	for _, weighted := range []bool{false} {
		q = acc.summary(0, weighted)
		if q.WeightStatus != "sample_count_only" {
			t.Fatal("cross-cohort weights acquired exact status")
		}
		for _, item := range q.RecordedCPUs {
			if item.Period != 0 || item.Percent != 0 {
				t.Fatal("unweighted census combined independent measures")
			}
		}
	}
	overflow := newPerfQualityAcc()
	for i := 0; i < 2; i++ {
		overflow.add(Event{Type: EventPerfSample, CPU: 0, PerfFields: &PerfFields{CPUKnown: &known}}, math.MaxInt64, "")
	}
	q = overflow.summary(0, false)
	if len(q.RecordedCPUs) != 1 || q.RecordedCPUs[0].Value != "cpu:0" || q.RecordedCPUs[0].SampleCount != 2 || q.RecordedCPUs[0].Period != 0 {
		t.Fatalf("overflow lost inventory or published invalid weight: %+v", q)
	}
}
