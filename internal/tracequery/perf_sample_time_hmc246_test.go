package tracequery

import (
	"math"
	"testing"
)

func TestHMC246SamplingTimeAccumulator(t *testing.T) {
	var a perfSampleTimeAcc
	if a.summary() != nil {
		t.Fatal("no records became time zero")
	}
	a.add(math.NaN())
	a.add(math.Inf(1))
	b := a.summary()
	if b.KnownSampleCount != 0 || b.UnknownSampleCount != 2 || b.FirstTs != nil || b.LastTs != nil {
		t.Fatalf("unknown time became zero: %+v", b)
	}
	for _, ts := range []float64{2, 0, 1, 0} {
		a.add(ts)
	}
	b = a.summary()
	if b.KnownSampleCount != 4 || b.UnknownSampleCount != 2 || b.FirstTs == nil || b.LastTs == nil || *b.FirstTs != 0 || *b.LastTs != 2 {
		t.Fatalf("unordered/zero time not preserved: %+v", b)
	}
	*b.FirstTs = 99
	if *a.summary().FirstTs != 0 {
		t.Fatal("display mutation changed original accumulation")
	}
}
