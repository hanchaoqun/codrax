package tracequery

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func traceMarkCarryRequest(start, end float64, maxWindows int, families ...WindowDiscoveryFamily) WindowDiscoveryRequest {
	return WindowDiscoveryRequest{
		Strategy: WindowDiscoveryTraceMarkCarry, Families: families,
		TimeStart: start, TimeEnd: end, TimeStartSet: true, TimeEndSet: true,
		MaxWindows: maxWindows, MaxWindowMs: 50, PaddingMs: 5,
	}
}

func traceMarkCarryCandidatesByStatus(result WindowDiscoveryResult, status WindowDiscoveryPairingStatus) []WindowDiscoveryCandidate {
	var out []WindowDiscoveryCandidate
	for _, candidate := range result.Candidates {
		if candidate.PairingStatus == status {
			out = append(out, candidate)
		}
	}
	return out
}

func TestTraceMarkCarryDiscoveryFourCarryClassesAndSoftSemanticPriority(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, strings.Join([]string{
		traceMarkTestLine("writer", 10, .800, "S|200|through|1"),
		traceMarkTestLine("writer", 10, .900, "S|200|carry-in|2"),
		traceMarkTestLine("writer", 10, 1.010, "B|10|VerifyClass"),
		traceMarkTestLine("writer", 10, 1.020, "E"),
		traceMarkTestLine("writer", 10, 1.030, "F|200|carry-in|2"),
		traceMarkTestLine("writer", 10, 1.040, "G|300|render-track|carry-out|3"),
		traceMarkTestLine("writer", 11, 1.200, "H|300|render-track|3"),
		traceMarkTestLine("writer", 11, 1.300, "F|200|through|1"),
		traceMarkTestLine("writer", 11, 1.400, "S|200|outside|4"),
		traceMarkTestLine("writer", 11, 1.410, "F|200|outside|4"),
	}, "\n"))
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
		traceMarkCarryRequest(1, 1.1, 8,
			WindowDiscoveryFamilyTraceSync, WindowDiscoveryFamilyTraceAsync, WindowDiscoveryFamilyTraceTrack))
	if err != nil {
		t.Fatal(err)
	}
	if result.Strategy != WindowDiscoveryTraceMarkCarry || !result.Complete || !result.IdentityComplete {
		t.Fatalf("trace-mark discovery quality = %+v", result)
	}
	classes := map[WindowDiscoveryCarryClass]int{}
	semanticFound := false
	for _, candidate := range traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingCompleteExact) {
		classes[candidate.CarryClass]++
		if candidate.SemanticClass == "class_verification" {
			semanticFound = true
		}
		if candidate.StartEndpoint == nil || candidate.EndEndpoint == nil || candidate.StartEndpoint.SourcePath != canonicalTraceIndexPath(path) || candidate.EndEndpoint.SourcePath != canonicalTraceIndexPath(path) {
			t.Errorf("complete candidate lost physical endpoint provenance: %+v", candidate)
		}
	}
	wantClasses := map[WindowDiscoveryCarryClass]int{
		WindowDiscoveryCarryIn: 1, WindowDiscoveryCarryOut: 1,
		WindowDiscoveryCarryThrough: 1, WindowDiscoveryInsidePair: 1,
	}
	if !reflect.DeepEqual(classes, wantClasses) || !semanticFound {
		t.Fatalf("carry/semantic candidates: classes=%v semantic=%t candidates=%+v", classes, semanticFound, result.Candidates)
	}
	if len(result.Windows) != 7 {
		t.Fatalf("four selected pairs should use 2+2+2+1 atomic windows, got %+v", result.Windows)
	}
	for _, window := range result.Windows {
		if window.PairingStatus != WindowDiscoveryPairingCompleteExact || window.CarryClass == "" || window.StartEndpoint == nil || window.EndEndpoint == nil {
			t.Errorf("selected window lost typed pair/carry provenance: %+v", window)
		}
		if width := (window.EndTs - window.StartTs) * 1000; width <= 0 || width > 50+1e-6 {
			t.Errorf("selected marker window width %.6fms violates hard cap: %+v", width, window)
		}
	}
	for _, candidate := range result.Candidates {
		if candidate.Identity == "" || strings.Contains(candidate.Identity, canonicalTraceIndexPath(path)) {
			t.Errorf("candidate identity missing or leaked absolute source path: %+v", candidate)
		}
		if candidate.StartEndpoint != nil && candidate.StartEndpoint.Name == "outside" {
			t.Fatalf("pair outside the parent scope became a candidate: %+v", candidate)
		}
	}
}

func TestTraceMarkCarryDiscoverySoftSemanticSeatIsNotStarvedByGenericCarryThrough(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, strings.Join([]string{
		traceMarkTestLine("writer", 10, .800, "S|200|generic-monitor|1"),
		traceMarkTestLine("writer", 10, 1.010, "B|10|VerifyClass"),
		traceMarkTestLine("writer", 10, 1.020, "E"),
		traceMarkTestLine("writer", 10, 1.300, "F|200|generic-monitor|1"),
	}, "\n"))
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
		traceMarkCarryRequest(1, 1.1, 2, WindowDiscoveryFamilyTraceSync, WindowDiscoveryFamilyTraceAsync))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Windows) != 1 || result.Windows[0].SemanticClass != "class_verification" ||
		result.Windows[0].StartEndpoint == nil || result.Windows[0].StartEndpoint.Name != "VerifyClass" {
		t.Fatalf("generic carry-through consumed the whole fan-out budget ahead of a deterministic semantic witness: %+v", result)
	}
	if !strings.Contains(result.SelectionBasis, "semantic class then carry class") {
		t.Fatalf("soft selection basis did not disclose semantic-before-carry policy: %q", result.SelectionBasis)
	}
}

func TestTraceMarkCarryDiscoverySyncNestedLIFO(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, strings.Join([]string{
		traceMarkTestLine("app", 10, 2.000, "B|10|outer"),
		traceMarkTestLine("app", 10, 2.010, "B|10|inner"),
		traceMarkTestLine("app", 10, 2.020, "E"),
		traceMarkTestLine("app", 10, 2.030, "E"),
	}, "\n"))
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
		traceMarkCarryRequest(1.99, 2.04, 2, WindowDiscoveryFamilyTraceSync))
	if err != nil {
		t.Fatal(err)
	}
	pairs := traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingCompleteExact)
	if len(pairs) != 2 {
		t.Fatalf("nested B/E must yield two exact LIFO pairs: %+v", result.Candidates)
	}
	byName := map[string][2]int{}
	for _, pair := range pairs {
		byName[pair.StartEndpoint.Name] = [2]int{pair.StartEndpoint.Line, pair.EndEndpoint.Line}
	}
	if want := map[string][2]int{"inner": {2, 3}, "outer": {1, 4}}; !reflect.DeepEqual(byName, want) {
		t.Fatalf("nested sync pairing = %v want %v", byName, want)
	}
}

func TestTraceMarkCarryDiscoveryDuplicateCohortFailsClosedAndRecovers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		family WindowDiscoveryFamily
		lines  []string
	}{
		{
			name: "async", family: WindowDiscoveryFamilyTraceAsync,
			lines: []string{
				traceMarkTestLine("w", 10, 3.000, "S|200|same|7"),
				traceMarkTestLine("w", 11, 3.001, "S|200|same|7"),
				traceMarkTestLine("w", 12, 3.002, "F|200|same|7"),
				traceMarkTestLine("w", 13, 3.003, "F|200|same|7"),
				traceMarkTestLine("w", 14, 3.004, "S|200|same|7"),
				traceMarkTestLine("w", 15, 3.005, "F|200|same|7"),
			},
		},
		{
			name: "track", family: WindowDiscoveryFamilyTraceTrack,
			lines: []string{
				traceMarkTestLine("w", 10, 3.000, "G|200|track|first|7"),
				traceMarkTestLine("w", 11, 3.001, "G|200|track|second|7"),
				traceMarkTestLine("w", 12, 3.002, "H|200|track|7"),
				traceMarkTestLine("w", 13, 3.003, "H|200|track|7"),
				traceMarkTestLine("w", 14, 3.004, "G|200|track|recovered|7"),
				traceMarkTestLine("w", 15, 3.005, "H|200|track|7"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeWindowDiscoveryTrace(t, strings.Join(tc.lines, "\n"))
			result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
				traceMarkCarryRequest(2.99, 3.01, 2, tc.family))
			if err != nil {
				t.Fatal(err)
			}
			if len(traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingAmbiguousDuplicate)) != 1 ||
				len(traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingCompleteExact)) != 1 || len(result.Windows) != 1 {
				t.Fatalf("duplicate cohort/recovery verdict: %+v", result)
			}
			if result.Windows[0].StartEndpoint.Line != 5 || result.Windows[0].EndEndpoint.Line != 6 {
				t.Fatalf("ambiguous cohort minted duration or recovery was lost: %+v", result.Windows)
			}
		})
	}
}

func TestTraceMarkCarryDiscoverySeparatesPhysicalSourceAndPayloadGeneration(t *testing.T) {
	t.Run("physical_source", func(t *testing.T) {
		req, err := normalizeWindowDiscoveryRequest(traceMarkCarryRequest(3.9, 4.2, 2, WindowDiscoveryFamilyTraceAsync))
		if err != nil {
			t.Fatal(err)
		}
		d := newTraceMarkCarryDiscovery(req, "primary")
		start := mustParseTraceMarkCarryEvent(t, 1, 4.0, "S|200|work|1")
		wrongEnd := mustParseTraceMarkCarryEvent(t, 2, 4.1, "F|200|work|1")
		rightEnd := mustParseTraceMarkCarryEvent(t, 3, 4.2, "F|200|work|1")
		d.observe("source-a", start)
		d.observe("source-b", wrongEnd)
		d.observe("source-a", rightEnd)
		result := d.finalize(&Index{LineCount: 3, LastTs: 4.2}, TraceSourceVersion{})
		pairs := traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingCompleteExact)
		if len(pairs) != 1 || pairs[0].StartEndpoint.SourcePath != "source-a" || pairs[0].EndEndpoint.SourcePath != "source-a" {
			t.Fatalf("async endpoints crossed physical sources: %+v", result)
		}
	})

	t.Run("payload_owner_generation_recovery", func(t *testing.T) {
		path := writeWindowDiscoveryTrace(t, strings.Join([]string{
			traceMarkTestLine("w", 10, 5.000, "S|200|old|1"),
			` creator-20 (20) [000] .... 5.010000: sched_wakeup_new: comm=reused pid=200 prio=120 target_cpu=000`,
			traceMarkTestLine("w", 11, 5.020, "F|200|old|1"),
			traceMarkTestLine("w", 12, 5.030, "S|200|new|2"),
			traceMarkTestLine("w", 13, 5.040, "F|200|new|2"),
		}, "\n"))
		result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
			traceMarkCarryRequest(4.99, 5.05, 2, WindowDiscoveryFamilyTraceAsync))
		if err != nil {
			t.Fatal(err)
		}
		if len(traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingLifecycleCut)) != 1 || len(result.Windows) != 1 {
			t.Fatalf("generation cut/recovery = %+v", result)
		}
		if result.Windows[0].StartEndpoint.Name != "new" || result.Windows[0].StartEndpoint.Generation != 1 || result.Windows[0].EndEndpoint.Generation != 1 {
			t.Fatalf("new generation pair missing or crossed old generation: %+v", result.Windows)
		}
	})

	t.Run("sync_emitter_generation_recovery", func(t *testing.T) {
		path := writeWindowDiscoveryTrace(t, strings.Join([]string{
			traceMarkTestLine("reused", 10, 5.100, "B|10|old-sync"),
			` creator-20 (20) [000] .... 5.110000: sched_wakeup_new: comm=reused pid=10 prio=120 target_cpu=000`,
			traceMarkTestLine("reused", 10, 5.120, "E"),
			traceMarkTestLine("reused", 10, 5.130, "B|10|new-sync"),
			traceMarkTestLine("reused", 10, 5.140, "E"),
		}, "\n"))
		result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
			traceMarkCarryRequest(5.09, 5.15, 2, WindowDiscoveryFamilyTraceSync))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Windows) != 1 || result.Windows[0].StartEndpoint.Name != "new-sync" || result.Windows[0].StartEndpoint.Generation != 1 {
			t.Fatalf("sync endpoints crossed emitter generation or failed to recover: %+v", result)
		}
	})

	t.Run("track_payload_owner_generation_recovery", func(t *testing.T) {
		path := writeWindowDiscoveryTrace(t, strings.Join([]string{
			traceMarkTestLine("w", 10, 5.200, "G|300|track|old-track|1"),
			` creator-20 (20) [000] .... 5.210000: sched_wakeup_new: comm=reused pid=300 prio=120 target_cpu=000`,
			traceMarkTestLine("w", 11, 5.220, "H|300|track|1"),
			traceMarkTestLine("w", 12, 5.230, "G|300|track|new-track|2"),
			traceMarkTestLine("w", 13, 5.240, "H|300|track|2"),
		}, "\n"))
		result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
			traceMarkCarryRequest(5.19, 5.25, 2, WindowDiscoveryFamilyTraceTrack))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Windows) != 1 || result.Windows[0].StartEndpoint.Name != "new-track" || result.Windows[0].StartEndpoint.Generation != 1 {
			t.Fatalf("track endpoints crossed payload-owner generation or failed to recover: %+v", result)
		}
	})
}

func TestTraceMarkCarryDiscoveryPairAtomicBudgetAndTimestampRollback(t *testing.T) {
	t.Run("two_window_atomic_budget", func(t *testing.T) {
		path := writeWindowDiscoveryTrace(t, strings.Join([]string{
			traceMarkTestLine("w", 10, 5.900, "S|200|long|1"),
			traceMarkTestLine("w", 11, 6.200, "F|200|long|1"),
		}, "\n"))
		one, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
			traceMarkCarryRequest(6.0, 6.1, 1, WindowDiscoveryFamilyTraceAsync))
		if err != nil {
			t.Fatal(err)
		}
		if len(one.Windows) != 0 || len(one.Candidates) != 1 || one.Candidates[0].RequiredWindowCount != 2 || one.Candidates[0].SelectionReason != "generated_window_budget_exhausted_pair_atomic" {
			t.Fatalf("one-window budget published half a pair: %+v", one)
		}
		two, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
			traceMarkCarryRequest(6.0, 6.1, 2, WindowDiscoveryFamilyTraceAsync))
		if err != nil {
			t.Fatal(err)
		}
		if len(two.Windows) != 2 || two.Windows[0].IdentityFingerprint != two.Windows[1].IdentityFingerprint {
			t.Fatalf("two-window budget did not publish the pair atomically: %+v", two)
		}
	})

	t.Run("same_lane_timestamp_rollback", func(t *testing.T) {
		path := writeWindowDiscoveryTrace(t, strings.Join([]string{
			traceMarkTestLine("w", 10, 7.010, "S|200|rollback|1"),
			traceMarkTestLine("w", 11, 7.000, "F|200|rollback|1"),
		}, "\n"))
		result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
			traceMarkCarryRequest(6.99, 7.02, 2, WindowDiscoveryFamilyTraceAsync))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Windows) != 0 || result.IdentityComplete || len(traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingTimestampRollback)) != 1 {
			t.Fatalf("rollback lane did not fail closed: %+v", result)
		}
	})
}

func TestTraceMarkCarryDiscoveryMalformedSourceBudgetAndTOCTOUFailClosed(t *testing.T) {
	t.Run("malformed_family_scoped", func(t *testing.T) {
		path := writeWindowDiscoveryTrace(t, strings.Join([]string{
			traceMarkTestLine("w", 10, 8.000, "S|bad|malformed|1"),
			traceMarkTestLine("w", 10, 8.001, "S|200|valid-but-poisoned|2"),
			traceMarkTestLine("w", 10, 8.002, "F|200|valid-but-poisoned|2"),
			traceMarkTestLine("w", 10, 8.003, "G|300|track|healthy|3"),
			traceMarkTestLine("w", 10, 8.004, "H|300|track|3"),
		}, "\n"))
		result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
			traceMarkCarryRequest(7.99, 8.01, 2, WindowDiscoveryFamilyTraceAsync, WindowDiscoveryFamilyTraceTrack))
		if err != nil {
			t.Fatal(err)
		}
		if result.IdentityComplete || len(traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingMalformedEndpoint)) != 1 || len(result.Windows) != 1 || result.Windows[0].Family != WindowDiscoveryFamilyTraceTrack {
			t.Fatalf("malformed S must fail-close async only and preserve track: %+v", result)
		}
	})

	t.Run("unmaterialized_track_endpoint", func(t *testing.T) {
		path := writeWindowDiscoveryTrace(t, strings.Join([]string{
			` writer-10 (10) [9999] .... 8.100000: tracing_mark_write: G|300|track|unmaterialized|3`,
			traceMarkTestLine("w", 10, 8.101, "G|300|track|otherwise-valid|4"),
			traceMarkTestLine("w", 10, 8.102, "H|300|track|4"),
		}, "\n"))
		result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
			traceMarkCarryRequest(8.09, 8.11, 2, WindowDiscoveryFamilyTraceTrack))
		if err != nil {
			t.Fatal(err)
		}
		if result.IdentityComplete || len(result.Windows) != 0 || len(traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingMalformedEndpoint)) != 1 {
			t.Fatalf("unmaterialized G endpoint did not fail-close track discovery: %+v", result)
		}
	})

	t.Run("source_unresolved", func(t *testing.T) {
		req, err := normalizeWindowDiscoveryRequest(traceMarkCarryRequest(8.9, 9.1, 2, WindowDiscoveryFamilyTraceSync))
		if err != nil {
			t.Fatal(err)
		}
		d := newTraceMarkCarryDiscovery(req, "primary")
		d.observe("", mustParseTraceMarkCarryEvent(t, 1, 9, "B|10|unresolved"))
		result := d.finalize(&Index{LineCount: 1, LastTs: 9}, TraceSourceVersion{})
		if result.IdentityComplete || len(result.Windows) != 0 || len(traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingSourceUnresolved)) != 1 {
			t.Fatalf("source-unresolved marker did not fail closed: %+v", result)
		}
	})

	t.Run("endpoint_budget", func(t *testing.T) {
		path := writeWindowDiscoveryTrace(t, strings.Join([]string{
			traceMarkTestLine("w", 10, 10.000, "B|10|budget"),
			traceMarkTestLine("w", 10, 10.001, "E"),
		}, "\n"))
		req := traceMarkCarryRequest(9.99, 10.01, 2, WindowDiscoveryFamilyTraceSync)
		req.EndpointLimit = 1
		result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
		if err != nil {
			t.Fatal(err)
		}
		if result.Complete || result.IdentityComplete || !result.BudgetStopped || len(result.Windows) != 0 || !strings.Contains(strings.Join(result.Caveats, "\n"), "no candidate window was published") {
			t.Fatalf("endpoint budget did not fail closed: %+v", result)
		}
	})

	t.Run("source_lock_after_scan", func(t *testing.T) {
		path := writeWindowDiscoveryTrace(t, strings.Join([]string{
			traceMarkTestLine("w", 10, 11.000, "B|10|toctou"),
			traceMarkTestLine("w", 10, 11.001, "E"),
		}, "\n"))
		oldHook := windowDiscoveryAfterStreamScanHook
		windowDiscoveryAfterStreamScanHook = func() {
			if err := os.WriteFile(path, []byte("replacement generation\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		defer func() { windowDiscoveryAfterStreamScanHook = oldHook }()
		_, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
			traceMarkCarryRequest(10.99, 11.01, 2, WindowDiscoveryFamilyTraceSync))
		if err == nil || !strings.Contains(err.Error(), "source universe changed") {
			t.Fatalf("post-scan source mutation was published: %v", err)
		}
	})
}

func TestTraceMarkCarryDiscoveryCandidatePoolKeepsHealthyCrossFamilySeat(t *testing.T) {
	lines := make([]string, 0, windowDiscoveryCandidatePoolLimit+3)
	for i := 0; i <= windowDiscoveryCandidatePoolLimit; i++ {
		lines = append(lines, traceMarkTestLine("w", 10, 11.100+float64(i)/1_000_000, fmt.Sprintf("S|bad|malformed-%d|1", i)))
	}
	lines = append(lines,
		traceMarkTestLine("w", 10, 11.200, "G|300|track|healthy-after-malformed-storm|3"),
		traceMarkTestLine("w", 10, 11.201, "H|300|track|3"),
	)
	path := writeWindowDiscoveryTrace(t, strings.Join(lines, "\n"))
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto,
		traceMarkCarryRequest(11.09, 11.21, 2, WindowDiscoveryFamilyTraceAsync, WindowDiscoveryFamilyTraceTrack))
	if err != nil {
		t.Fatal(err)
	}
	if !result.CandidatePoolTruncated || result.RetainedCandidateCount != windowDiscoveryCandidatePoolLimit {
		t.Fatalf("malformed storm did not exercise bounded candidate pool: %+v", result)
	}
	if len(result.Windows) != 1 || result.Windows[0].Family != WindowDiscoveryFamilyTraceTrack || result.Windows[0].StartEndpoint == nil || result.Windows[0].StartEndpoint.Name != "healthy-after-malformed-storm" {
		t.Fatalf("one poisoned family crowded a healthy exact pair from another family: %+v", result)
	}
}

func TestTraceMarkCarryDiscoveryClosedRegistriesAndValidation(t *testing.T) {
	if got := WindowDiscoveryFamilyNames(WindowDiscoveryTraceMarkCarry); !reflect.DeepEqual(got, []string{"trace_sync", "trace_async", "trace_track"}) {
		t.Fatalf("trace_mark_carry family registry = %v", got)
	}
	if got := WindowDiscoveryPairingStatusNames(); len(got) != 9 {
		t.Fatalf("pairing status registry = %v", got)
	}
	if got := WindowDiscoveryCarryClassNames(); !reflect.DeepEqual(got, []string{"carry_in", "carry_out", "carry_through", "inside_pair"}) {
		t.Fatalf("carry registry = %v", got)
	}
	for name, req := range map[string]WindowDiscoveryRequest{
		"unbounded":        {Strategy: WindowDiscoveryTraceMarkCarry},
		"foreign family":   traceMarkCarryRequest(1, 2, 2, WindowDiscoveryFamilyBlock),
		"half line parent": {Strategy: WindowDiscoveryTraceMarkCarry, LineStart: 1},
	} {
		if _, err := normalizeWindowDiscoveryRequest(req); err == nil {
			t.Errorf("%s trace_mark_carry request must fail loud: %+v", name, req)
		}
	}
}

func mustParseTraceMarkCarryEvent(t *testing.T, line int, ts float64, payload string) Event {
	t.Helper()
	ev, ok := ParseLine(line, traceMarkTestLine("writer", 10, ts, payload), newStringInterner())
	if !ok || ev.Type != EventTraceMark || ev.SpanAction == "" {
		t.Fatalf("fixture marker %q failed parse: %+v", payload, ev)
	}
	return ev
}
