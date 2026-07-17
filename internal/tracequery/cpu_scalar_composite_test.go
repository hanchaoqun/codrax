package tracequery

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func buildCPUScalarComposite(t *testing.T, namesAndBodies ...string) *Index {
	t.Helper()
	if len(namesAndBodies)%2 != 0 {
		t.Fatal("fixture names and bodies must be paired")
	}
	dir := t.TempDir()
	paths := make([]string, 0, len(namesAndBodies)/2)
	for i := 0; i < len(namesAndBodies); i += 2 {
		path := filepath.Join(dir, namesAndBodies[i])
		writeBundleProvenanceFixture(t, path, namesAndBodies[i+1])
		paths = append(paths, path)
	}
	idx, err := parseTraceArtifactPathList(context.Background(), filepath.Join(dir, "composite"), 0, 0, BuildOptions{}, paths)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestCPUScalarCompositeKnownPoisonCannotBeResurrectedBySibling(t *testing.T) {
	idx := buildCPUScalarComposite(t,
		"bad.ftrace", strings.Join([]string{
			`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=900000 cpu_id=0`,
			`idle-0 (0) [000] .... 1.100000: cpu_frequency: state=broken cpu_id=0`,
			`idle-0 (0) [001] .... 1.200000: cpu_frequency: state=1800000 cpu_id=1`,
			`idle-0 (0) [000] .... 1.300000: cpu_frequency_limits: min=300000 max=1000000 cpu_id=0`,
			`idle-0 (0) [000] .... 1.400000: cpu_frequency_limits: min=broken max=1000000 cpu_id=0`,
			`idle-0 (0) [001] .... 1.500000: cpu_frequency_limits: min=400000 max=2200000 cpu_id=1`,
		}, "\n")+"\n",
		"healthy.htrace", strings.Join([]string{
			`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
			`idle-0 (0) [000] .... 1.100000: cpu_frequency: state=1600000 cpu_id=0`,
			`idle-0 (0) [001] .... 1.200000: cpu_frequency: state=2400000 cpu_id=1`,
			`idle-0 (0) [000] .... 1.300000: cpu_frequency_limits: min=400000 max=1800000 cpu_id=0`,
			`idle-0 (0) [001] .... 1.400000: cpu_frequency_limits: min=400000 max=2600000 cpu_id=1`,
		}, "\n")+"\n",
	)

	if !idx.fullFreq.collected || !idx.fullFreq.freqUnsafe[0] || !idx.fullFreq.limitUnsafe[0] {
		t.Fatalf("composite lost child poison receipt: %+v", idx.fullFreq)
	}
	if got, ok := idx.fullFrequencyTimelines(); !ok || len(got[0]) != 0 || len(got[1]) == 0 {
		t.Fatalf("frequency sibling resurrected poisoned cpu0 or erased healthy cpu1: ok=%t curves=%v", ok, got)
	}
	if got, ok := idx.fullFrequencyLimitTimelines(); !ok || len(got[0]) != 0 || len(got[1]) == 0 {
		t.Fatalf("limit sibling resurrected poisoned cpu0 or erased healthy cpu1: ok=%t curves=%v", ok, got)
	}
	if got := indexFreqSampleTimelines(idx); len(got[0]) != 0 || len(got[1]) == 0 {
		t.Fatalf("hard frequency projection bypassed composite poison: %v", got)
	}
	cache := newChainQueryCache(idx, nil)
	cache.buildFreqLimitIndex()
	if len(cache.freqLimitByCPU[0]) != 0 || len(cache.freqLimitByCPU[1]) == 0 || cache.governedLimitMaxKHz(0, 1, 2) != 0 {
		t.Fatalf("policy ceiling bypassed composite poison: %v", cache.freqLimitByCPU)
	}
	if caveats := strings.Join(frequencyOrderIntegrityForGlobalDerivation(idx).globalCaveats(), "\n"); !strings.Contains(caveats, "bad.ftrace") ||
		!strings.Contains(caveats, "line=2") || !strings.Contains(caveats, "line=5") {
		t.Fatalf("composite poison lost its physical-source witness: %s", caveats)
	}
}

func TestCPUScalarCompositeUnknownOwnerPoisonsOnlyItsFamily(t *testing.T) {
	t.Run("frequency", func(t *testing.T) {
		idx := buildCPUScalarComposite(t,
			"bad.ftrace", strings.Join([]string{
				`idle-0 (0) [000] .... 1.000000: vendor_CPU_FREQ: state=broken`,
				`idle-0 (0) [000] .... 1.100000: cpu_frequency_limits: min=400000 max=1800000 cpu_id=0`,
			}, "\n")+"\n",
			"healthy.htrace", strings.Join([]string{
				`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
				`idle-0 (0) [001] .... 1.100000: cpu_frequency: state=2200000 cpu_id=1`,
				`idle-0 (0) [001] .... 1.200000: cpu_frequency_limits: min=400000 max=2600000 cpu_id=1`,
			}, "\n")+"\n",
		)

		if !idx.fullFreq.freqAll || idx.fullFreq.limitAll {
			t.Fatalf("unknown-owner poison crossed or missed its event family: %+v", idx.fullFreq)
		}
		if got, ok := idx.fullFrequencyTimelines(); !ok || len(got) != 0 {
			t.Fatalf("unknown-owner frequency poison did not close the family: ok=%t curves=%v", ok, got)
		}
		if got, ok := idx.fullFrequencyLimitTimelines(); !ok || len(got[0]) == 0 || len(got[1]) == 0 {
			t.Fatalf("frequency poison crossed into healthy limits family: ok=%t curves=%v", ok, got)
		}
	})

	t.Run("limits", func(t *testing.T) {
		idx := buildCPUScalarComposite(t,
			"bad.ftrace", strings.Join([]string{
				`idle-0 (0) [000] .... 1.000000: cpu_frequency_limits: min=broken max=1800000`,
				`idle-0 (0) [000] .... 1.100000: cpu_frequency: state=1200000 cpu_id=0`,
			}, "\n")+"\n",
			"healthy.htrace", strings.Join([]string{
				`idle-0 (0) [001] .... 1.000000: cpu_frequency: state=2200000 cpu_id=1`,
				`idle-0 (0) [000] .... 1.100000: cpu_frequency_limits: min=400000 max=1800000 cpu_id=0`,
				`idle-0 (0) [001] .... 1.200000: cpu_frequency_limits: min=400000 max=2600000 cpu_id=1`,
			}, "\n")+"\n",
		)

		if idx.fullFreq.freqAll || !idx.fullFreq.limitAll {
			t.Fatalf("unknown-owner poison crossed or missed its event family: %+v", idx.fullFreq)
		}
		if got, ok := idx.fullFrequencyLimitTimelines(); !ok || len(got) != 0 {
			t.Fatalf("unknown-owner limits poison did not close the family: ok=%t curves=%v", ok, got)
		}
		if got, ok := idx.fullFrequencyTimelines(); !ok || len(got[0]) == 0 || len(got[1]) == 0 {
			t.Fatalf("limits poison crossed into healthy frequency family: ok=%t curves=%v", ok, got)
		}
	})
}

func TestCPUScalarCompositePoisonSurvivesWarmWindowDerivation(t *testing.T) {
	idx := buildCPUScalarComposite(t,
		"bad.ftrace", strings.Join([]string{
			`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=900000 cpu_id=0`,
			`idle-0 (0) [000] .... 1.100000: cpu_frequency_limits: min=300000 max=1000000 cpu_id=0`,
			`idle-0 (0) [000] .... 9.000000: cpu_frequency: state=broken cpu_id=0`,
			`idle-0 (0) [000] .... 9.100000: cpu_frequency_limits: min=broken max=1000000 cpu_id=0`,
		}, "\n")+"\n",
		"healthy.htrace", strings.Join([]string{
			`idle-0 (0) [000] .... 1.200000: cpu_frequency: state=1600000 cpu_id=0`,
			`idle-0 (0) [000] .... 1.300000: cpu_frequency_limits: min=400000 max=1800000 cpu_id=0`,
		}, "\n")+"\n",
	)
	derived := deriveWindowedIndex(idx, BuildOptions{
		TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true,
	})
	// BuildIndex's warm-cache derive path replaces these copied full-index
	// ledgers with query-relevant witnesses. Model that final step explicitly:
	// both malformed transitions are outside 1..2 and therefore absent.
	derived.durationOrderFailures = nil
	derived.durationOrderFailuresCapped = nil
	// The malformed transitions are outside the derived query ledger. The
	// full-file poison receipt must therefore carry the global derivation
	// verdict on its own; a warm window cannot revive sibling samples.
	if integrity := frequencyOrderIntegrityForGlobalDerivation(derived); !integrity.frequencyUnsafe(0) || !integrity.limitUnsafe(0) {
		t.Fatalf("warm derive lost the full-file poison verdict: %+v", integrity)
	} else if caveats := strings.Join(integrity.globalCaveats(), "\n"); !strings.Contains(caveats, "duration_endpoint_parse_incomplete") ||
		!strings.Contains(caveats, "bad.ftrace") || !strings.Contains(caveats, "line=3") || !strings.Contains(caveats, "line=4") ||
		!strings.Contains(caveats, "authority=trace_global_derivation") {
		t.Fatalf("warm derive lost the full-file poison disclosure: %+v caveats=%s", integrity, caveats)
	}
	if got := indexFreqSampleTimelines(derived); len(got[0]) != 0 {
		t.Fatalf("warm derive resurrected poisoned frequency lane: %v", got)
	}
	cache := newChainQueryCache(derived, nil)
	cache.buildFreqLimitIndex()
	if len(cache.freqLimitByCPU[0]) != 0 || cache.governedLimitMaxKHz(0, 1, 2) != 0 {
		t.Fatalf("warm derive resurrected poisoned limits lane: %v", cache.freqLimitByCPU)
	}
}

func TestCPUScalarCausallyIsolatedChildCannotPoisonPrimary(t *testing.T) {
	idx := buildCPUScalarComposite(t,
		"healthy.systrace", `idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`+"\n",
		"isolated.systrace", `idle-0 (0) [000] .... 1.100000: cpu_frequency: state=broken cpu_id=0`+"\n",
	)
	if len(idx.TraceArtifacts) != 2 || idx.TraceArtifacts[1].CausalCompatible {
		t.Fatalf("fixture drift: second systrace must be causally isolated: %+v", idx.TraceArtifacts)
	}
	if got := indexFreqSampleTimelines(idx); len(got[0]) != 1 || got[0][0].khz != 1200000 {
		t.Fatalf("isolated child poisoned the primary frequency authority: %v", got)
	}
}

func TestCPUScalarCompositePoisonIsChildOrderIndependent(t *testing.T) {
	idx := buildCPUScalarComposite(t,
		"healthy.ftrace", strings.Join([]string{
			`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1600000 cpu_id=0`,
			`idle-0 (0) [000] .... 1.100000: cpu_frequency_limits: min=400000 max=1800000 cpu_id=0`,
		}, "\n")+"\n",
		"bad.htrace", strings.Join([]string{
			`idle-0 (0) [000] .... 1.200000: cpu_frequency: state=broken cpu_id=0`,
			`idle-0 (0) [000] .... 1.300000: cpu_frequency_limits: min=broken max=1800000 cpu_id=0`,
		}, "\n")+"\n",
	)
	if got := indexFreqSampleTimelines(idx); len(got[0]) != 0 {
		t.Fatalf("bad-last child failed to withdraw an earlier sibling frequency: %v", got)
	}
	cache := newChainQueryCache(idx, nil)
	cache.buildFreqLimitIndex()
	if len(cache.freqLimitByCPU[0]) != 0 {
		t.Fatalf("bad-last child failed to withdraw an earlier sibling limit: %v", cache.freqLimitByCPU)
	}
}

func TestCPUScalarCompositeRollbackPoisonCannotBeResurrected(t *testing.T) {
	idx := buildCPUScalarComposite(t,
		"rollback.ftrace", strings.Join([]string{
			`idle-0 (0) [000] .... 2.000000: cpu_frequency: state=1600000 cpu_id=0`,
			`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
			`idle-0 (0) [001] .... 2.100000: cpu_frequency: state=2200000 cpu_id=1`,
		}, "\n")+"\n",
		"healthy.htrace", strings.Join([]string{
			`idle-0 (0) [000] .... 1.100000: cpu_frequency: state=1800000 cpu_id=0`,
			`idle-0 (0) [001] .... 1.200000: cpu_frequency: state=2400000 cpu_id=1`,
		}, "\n")+"\n",
	)
	if !idx.fullFreq.freqUnsafe[0] {
		t.Fatalf("collector rollback did not enter the full-file poison receipt: %+v", idx.fullFreq)
	}
	if got := indexFreqSampleTimelines(idx); len(got[0]) != 0 || len(got[1]) == 0 {
		t.Fatalf("sibling resurrected rollback-poisoned cpu0 or erased cpu1: %v", got)
	}
}
