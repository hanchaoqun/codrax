package types

import (
	"reflect"
	"testing"
)

func TestPerfMetaAuthorityPreservesIndependentVerifiedSemantics(t *testing.T) {
	b := &PerfBundle{Meta: PerfMeta{Signals: []string{"io-block", "render-miss"}},
		Janks:   []PerfJank{{VerdictAuthority: PerfObservationAuthorityDeterministicValidator}},
		Stalls:  []PerfStall{{Authority: PerfObservationAuthorityDeterministicValidator, DurationMs: PerfMainThreadStallMs}},
		Startup: &PerfStartup{Authority: PerfObservationAuthorityDeterministicValidator, Mode: "cold", AppLaunchMs: PerfStartupSlowColdMs + 1}}
	want := []string{"jank", "main-thread-stall", "cold-start-slow"}
	if b.HasAuthoritativeMeta() || !reflect.DeepEqual(b.AuthoritativeSignals(), want) {
		t.Fatalf("metadata authority suppressed or invented record facts: %v", b.AuthoritativeSignals())
	}
	b.Meta.Authority = PerfObservationAuthorityDeterministicValidator
	if !reflect.DeepEqual(b.AuthoritativeSignals(), append([]string{"io-block", "render-miss"}, want...)) {
		t.Fatal("verified metadata was lost")
	}
	var absent *PerfBundle
	if absent.HasAuthoritativeMeta() || len(absent.AuthoritativeSignals()) != 0 {
		t.Fatal("nil metadata authorized")
	}
}
