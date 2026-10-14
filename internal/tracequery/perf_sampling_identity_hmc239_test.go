package tracequery

import (
	"context"
	"reflect"
	"testing"
)

func TestSamplingIdentityCoverageDoesNotChangeRoleAdmissionHMC239(t *testing.T) {
	idx := buildTraceIndex(t, "role.perftrace", `
worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=3 event=cycles symbol=work source=raw
unknown-11 (10) [000] .... 1.2: perf_sample: pid=10 tid=11 thread_identity_known=false sample_weight=7 event=cycles symbol=other source=raw
`)
	q := Query{View: "perf_stats", PID: 11, TimeStart: 1, TimeEnd: 2}
	ordinary := computePerfContext(idx, q, 8)
	if ordinary == nil || ordinary.SampleCount != 0 || ordinary.ThreadIdentityCountExact == nil || *ordinary.ThreadIdentityCountExact {
		t.Fatalf("incomplete selected population became exact: %+v", ordinary)
	}
	// Causal/role joins have an independently proved member predicate. This
	// change does not broaden it to anonymous samples or suppress its facts.
	role := computePerfContextFilteredWithCaveats(idx, q, 8, func(_ Event, _ perfThreadKey, id PerfThreadIdentity, known bool) bool {
		return known && id.TID == 11
	}, nil)
	if role == nil || role.SampleCount != 1 || role.TotalPeriod != 3 || len(role.TopThreads) != 1 || role.TopThreads[0].Identity == nil || role.TopThreads[0].Identity.TID != 11 {
		t.Fatalf("population completeness rewrote independent role admission: %+v", role)
	}
}

func TestSamplingIdentityCoverageKeepsGenerationsHMC239(t *testing.T) {
	idx := buildTraceIndex(t, "generations.perftrace", `
worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=3 event=cycles symbol=work source=raw
other-99 (99) [000] .... 1.4: sched_wakeup_new: comm=worker pid=11 prio=120 target_cpu=0
worker-11 (10) [000] .... 1.6: perf_sample: pid=10 tid=11 sample_weight=5 event=cycles symbol=work source=raw
unknown-11 (10) [000] .... 2.2: perf_sample: pid=10 tid=11 thread_identity_known=false sample_weight=7 event=cycles symbol=other source=raw
`)
	q := Query{View: ViewPerfSamplingTree, PID: 11, TimeStart: 1, TimeEnd: 2}
	stream, err := StreamPerfSamplingTree(context.Background(), idx.Path, q)
	if err != nil {
		t.Fatal(err)
	}
	indexed := Run(idx, q)
	if !reflect.DeepEqual(stream.PerfSamplingTree, indexed.PerfSamplingTree) {
		t.Fatal("streamed and indexed identity coverage diverged")
	}
	p := stream.PerfSamplingTree
	if p.Status != "available" || p.SampleCount != 2 || p.CohortCount != 2 {
		t.Fatalf("outside-window unknown merged or suppressed task generations: %+v", p)
	}
	weights := map[int]int64{}
	for _, c := range p.Cohorts {
		if c.ThreadIdentity == nil || c.ThreadIdentity.TID != 11 {
			t.Fatalf("generation lost exact subject: %+v", c)
		}
		weights[c.ThreadIdentity.Generation] = c.TotalWeight
	}
	if !reflect.DeepEqual(weights, map[int]int64{1: 3, 2: 5}) {
		t.Fatalf("independent generation denominators changed: %v", weights)
	}
}
