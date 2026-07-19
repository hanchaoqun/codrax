package tracequery

// evalcase_mt_bundle_pin_test.go — EVALCASE-DH batch, 多 trace guard pins on
// the REAL fixture pair (mining ledger evalcase_xa_cmp_mining.md §3, intake
// oracle MT-P2/MT-P3 re-collected at HEAD 1ada2c49f). The current promise
// face is pinned VERBATIM so any future systrace↔systrace affine lane must
// consciously rewrite these strings (设计上防未来行为漂移无声 — mining §4.3).
//
//	MT-2  双 systrace bundle — the second systrace child passes schema
//	      validation but is FORCE-isolated: clock_alignment=isolated with
//	      the verbatim shared-capture identity reason; zero of its events
//	      enter the shared causal timeline; the primary keeps identity
//	      authority. The arbitration arm is the CAPTURE IDENTITY proof —
//	      the two children carry the SAME time-domain label and are still
//	      not merged (a label is not an identity).
//	MT-3  隔离后按图索骥 — the isolation caveat carries the pointer words
//	      "query the artifact directly for per-domain analysis", and the
//	      isolated child stays fully queryable standalone.
//	MT-4  stream_scan 单工件拒 — the streaming view refuses the bundle with
//	      the verbatim single-physical-artifact error while the indexed
//	      composite views on the same bundle keep working.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const evalcaseMTIsolationReason = "additional systrace lacks explicit shared-capture identity proof; only one systrace causal authority is permitted"

func evalcaseWriteRealPairBundle(t *testing.T) (bundlePath string, donghuEvents int) {
	t.Helper()
	for _, p := range []string{evalcaseDonghuFixture, evalcaseTiebaFixture} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("real fixture not present: %v", err)
		}
	}
	dir := t.TempDir()
	copyIn := func(src, name string) {
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	copyIn(evalcaseDonghuFixture, "donghu.systrace")
	copyIn(evalcaseTiebaFixture, "xxx_all.systrace")
	bundlePath = filepath.Join(dir, "pair.tracebundle.json")
	// V2 schema binding (the mining MT-P2 oracle shape: schema
	// codrax.tracebundle/v2 with computed capture identity) — the legacy
	// schema admits exactly one systrace and never reaches the isolation arm.
	writeBundleProvenanceFixture(t, bundlePath, `{
  "version":"test",
  "systrace":"donghu.systrace",
  "artifacts":[
    {"type":"systrace","path":"donghu.systrace"},
    {"type":"systrace","path":"xxx_all.systrace"}
  ]
}`)
	primary, err := BuildIndex(context.Background(), evalcaseDonghuFixture)
	if err != nil {
		t.Fatal(err)
	}
	return bundlePath, len(primary.Events)
}

// MT-2 + MT-3.
func TestEvalcaseMT2SecondSystraceAuthorityIsolationRealPair(t *testing.T) {
	bundlePath, donghuEvents := evalcaseWriteRealPairBundle(t)
	idx, err := BuildIndex(context.Background(), bundlePath)
	if err != nil {
		t.Fatalf("bundle must pass schema validation (isolation is admission-side, not a parse error): %v", err)
	}
	if len(idx.TraceArtifacts) != 2 {
		t.Fatalf("MT-2: artifact ledger drifted: %+v", idx.TraceArtifacts)
	}
	primary, second := idx.TraceArtifacts[0], idx.TraceArtifacts[1]
	if !primary.CausalCompatible || primary.ClockAlignment != TraceClockAlignmentIdentity {
		t.Fatalf("MT-2: primary systrace lost identity authority: %+v", primary)
	}
	if second.CausalCompatible || second.ClockAlignment != TraceClockAlignmentIsolated {
		t.Fatalf("MT-2: second systrace was not isolated: %+v", second)
	}
	// Verbatim promise face (drift here = the affine-lane design changed).
	if second.IsolationReason != evalcaseMTIsolationReason {
		t.Fatalf("MT-2: isolation reason drifted:\n got %q\nwant %q", second.IsolationReason, evalcaseMTIsolationReason)
	}
	// The arbitration arm is capture identity, NOT the time-domain label:
	// both children carry the SAME label and are still not merged.
	if primary.TimeDomain != second.TimeDomain {
		t.Fatalf("MT-2: label-arm precondition drifted — the pin needs equal labels, got %q vs %q", primary.TimeDomain, second.TimeDomain)
	}
	// Zero isolated-child events in the shared timeline: the tieba capture
	// lives at 34579.x; the donghu clock is 13762.x. The composite must hold
	// exactly the primary's events and nothing in the tieba clock range.
	if len(idx.Events) != donghuEvents {
		t.Fatalf("MT-2: shared timeline event count drifted: %d want %d (isolated child must contribute zero)", len(idx.Events), donghuEvents)
	}
	for _, ev := range idx.Events {
		if ev.Ts >= 34579 && ev.Ts < 34580 {
			t.Fatalf("MT-2: isolated child event leaked into the shared timeline: %+v", ev)
		}
	}
	// MT-3 pointer words on the disclosure caveat.
	found := false
	for _, c := range idx.Caveats {
		if strings.Contains(c, "tracebundle_clock_domain_isolated") &&
			strings.Contains(c, "xxx_all.systrace") &&
			strings.Contains(c, evalcaseMTIsolationReason) &&
			strings.Contains(c, "query the artifact directly for per-domain analysis") {
			found = true
		}
	}
	if !found {
		t.Fatalf("MT-3: isolation disclosure caveat missing/drifted: %v", idx.Caveats)
	}
	// MT-3: the isolated child stays fully queryable standalone (the pointed
	// path works — XA-* cases all ride it).
	standalone, err := BuildIndex(context.Background(), evalcaseTiebaFixture)
	if err != nil || len(standalone.Events) == 0 {
		t.Fatalf("MT-3: isolated artifact must stay directly queryable: err=%v", err)
	}
}

// MT-4.
func TestEvalcaseMT4StreamScanBundleRefusalIndexedWorks(t *testing.T) {
	bundlePath, _ := evalcaseWriteRealPairBundle(t)
	_, err := StreamScan(context.Background(), bundlePath, "", func(Event) bool { return true })
	if err == nil {
		t.Fatal("MT-4: stream_scan must refuse a bundle universe")
	}
	want := "stream_scan requires a single physical artifact; " + canonicalTraceIndexPath(bundlePath) + " has a tracebundle or sibling artifact universe, so use an indexed composite view"
	if err.Error() != want {
		t.Fatalf("MT-4: refusal string drifted:\n got %q\nwant %q", err.Error(), want)
	}
	// The indexed composite view on the SAME bundle keeps working (the
	// refusal message's own pointer): a donghu-window stats query answers.
	idx, err := BuildIndex(context.Background(), bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	q := normalizeQuery(idx, Query{PID: 17267, TimeStart: 13762.9374, TimeEnd: 13762.9736})
	stats := ComputeWindowStats(idx, q)
	if len(stats.TopRunning) == 0 || stats.TopRunning[0].Thread.PID != 17267 {
		t.Fatalf("MT-4: indexed composite view broke on the bundle: %+v", stats.TopRunning)
	}
}
