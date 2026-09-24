package tracequery

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestIOInFlightMembersSourceCoordinatesAcrossIndexForms(t *testing.T) {
	// Exercise a real public perf+systrace V2 bundle and the lower native
	// path-list replay separately. V2 reserves its declared primary first;
	// path-list replay can reserve an isolated perf source before the primary.
	dir := t.TempDir()
	trace, perf := filepath.Join(dir, "capture.systrace"), filepath.Join(dir, "sample.perftrace")
	body := "# physical preamble\n" + ioInflightPublicRQ(ioInflightPublicPair{1.001, 1.003})
	writeBundleProvenanceFixture(t, trace, body)
	writeBundleProvenanceFixture(t, perf, "app-41 (41) [000] .... 1.002000: perf_sample: cpu=0 pid=41 tid=41 period=1 event=cpu-cycles symbol=App dso=libapp.so source=test\n"+body)
	q := Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.01}
	direct, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	directMember := Run(direct, q).WindowStats.IOInFlight.Groups[0].Members[0]
	check := func(idx *Index, expectRebased bool) {
		t.Helper()
		result := Run(idx, q)
		stats := ioInflightPublicDecode(t, *result.WindowStats)
		if len(stats.Groups) != 1 || len(stats.Groups[0].Members) != 1 {
			t.Fatalf("companion injected IO events or primary lost its witness: %+v", stats)
		}
		g := stats.Groups[0]
		ioInFlightAssertMemberPartition(t, g)
		m := g.Members[0]
		if m.ID != directMember.ID || m.IssueLocalLine != 2 || m.CompleteLocalLine != 3 || m.SourcePath != canonicalTraceIndexPath(trace) {
			t.Fatalf("identity depended on bundle placement: direct=%+v bundle=%+v", directMember, m)
		}
		if expectRebased && (m.IssueLine == m.IssueLocalLine || m.CompleteLine == m.CompleteLocalLine) {
			t.Fatalf("virtual line number masqueraded as physical source coordinate: %+v", m)
		}
		spans := result.ResolveArtifactSpans(m.IssueLine, m.IssueLine)
		if len(spans) != 1 || spans[0].SourcePath != m.SourcePath || spans[0].LocalLineStart != m.IssueLocalLine {
			t.Fatalf("public provenance cannot resolve exact issue witness: %+v %+v", m, spans)
		}
	}
	manifest := filepath.Join(dir, "public.tracebundle.json")
	writeTraceBundleV2ForTest(t, manifest, []byte(`{"version":"test","systrace":"capture.systrace","artifacts":[{"type":"perftrace","path":"sample.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}},{"type":"systrace","path":"capture.systrace"}],"perf_clock_alignments":[{"artifact_path":"sample.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":0,"slope":1,"calibrated":true}]}`))
	bundle, err := BuildIndex(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	check(bundle, false)
	for _, perfFirst := range []bool{false, true} {
		paths := []string{trace, perf}
		if perfFirst {
			paths = []string{perf, trace}
		}
		composite, err := parseTraceArtifactPathList(context.Background(), filepath.Join(dir, "native-path-list"), 0, 0, BuildOptions{}, paths)
		if err != nil {
			t.Fatal(err)
		}
		check(composite, perfFirst)
	}
	// Same endpoints in another physical file are separate instances even
	// though it can only be queried independently, not merged as a sibling.
	other := filepath.Join(dir, "other.systrace")
	writeBundleProvenanceFixture(t, other, body)
	otherIndex, err := BuildIndex(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	if Run(otherIndex, q).WindowStats.IOInFlight.Groups[0].Members[0].ID == directMember.ID {
		t.Fatal("identical request payloads in different physical files shared identity")
	}
}

func TestIOInFlightMembersPublicManifestKeepsSiblingIsolated(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "primary.systrace")
	sibling := filepath.Join(dir, "sibling.systrace")
	writeBundleProvenanceFixture(t, primary, ioInflightPublicRQ(ioInflightPublicPair{1.001, 1.003}))
	writeBundleProvenanceFixture(t, sibling, ioInflightPublicRQ(ioInflightPublicPair{1.001, 1.008}))
	manifest := filepath.Join(dir, "capture.tracebundle.json")
	writeTraceBundleV2ForTest(t, manifest, []byte(`{"version":"test","systrace":"primary.systrace","artifacts":[{"type":"systrace","path":"primary.systrace"},{"type":"systrace","path":"sibling.systrace"}]}`))
	idx, err := BuildIndex(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	result := Run(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.01})
	stats := ioInflightPublicDecode(t, *result.WindowStats)
	if len(stats.Groups) != 1 || len(stats.Groups[0].Members) != 1 || stats.Groups[0].Members[0].SourcePath != canonicalTraceIndexPath(primary) {
		t.Fatalf("member face widened existing source admission: %+v", stats)
	}
	ioInflightPublicValues(t, stats.Groups[0], 1, 1, 1, .2, 2, 2)
	for _, source := range idx.TraceArtifacts {
		if source.SourcePath == canonicalTraceIndexPath(sibling) && source.CausalCompatible {
			t.Fatal("sibling source gained causal compatibility")
		}
	}
}

func TestIOInFlightMembersPublicReusedPayloadIsDistinctInstance(t *testing.T) {
	body := ioInflightPublicRQ(ioInflightPublicPair{1.001, 1.003}) + ioInflightPublicRQ(ioInflightPublicPair{1.005, 1.007})
	// Both helpers emit the same device/op/sector/length/name. Successful
	// close-and-reopen cohorts are separate requests, not deduplicated data.
	if strings.Count(body, " 0 + 8 ") != 4 {
		t.Fatal("fixture did not reuse the request payload")
	}
	stats := ioInflightPublicStats(t, body, 1, 1.01)
	g := ioInflightPublicDecode(t, stats).Groups[0]
	ioInFlightAssertMemberPartition(t, g)
	if len(g.Members) != 2 || g.Members[0].ID == g.Members[1].ID || g.Members[0].IssueLocalLine != 1 || g.Members[1].IssueLocalLine != 3 {
		t.Fatalf("reused request payload replaced physical endpoint identity: %+v", g)
	}
	ioInflightPublicValues(t, g, 2, 2, 1, .4, 4, 4)
}
