package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIOInflightPublicGroupingKeepsLayerDeviceAndOperation(t *testing.T) {
	body := "io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\n" +
		"io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 W 4096 () 123 + 8 [io]\n" +
		"io-40 (40) [003] .... 1.001000: block_rq_issue: 8,1 R 4096 () 123 + 8 [io]\n" +
		"io-40 (40) [003] .... 1.001000: block_bio_queue: 8,0 R 123 + 8 [io]\n" +
		"io-40 (40) [003] .... 1.001000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096\n" +
		"irq-2 (2) [003] .... 1.004000: block_rq_complete: 8,0 R () 123 + 8 [0]\n" +
		"irq-2 (2) [003] .... 1.004000: block_rq_complete: 8,0 W () 123 + 8 [0]\n" +
		"irq-2 (2) [003] .... 1.004000: block_rq_complete: 8,1 R () 123 + 8 [0]\n" +
		"irq-2 (2) [003] .... 1.004000: block_bio_complete: 8,0 R 123 + 8 [0]\n" +
		"io-40 (40) [003] .... 1.004000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096\n"
	idx := buildTraceIndex(t, "grouping.systrace", body)
	q := Query{View: "window_stats", PID: 99, TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true}
	result := Run(idx, q)
	if result.WindowStats == nil || result.RootCauseRank != nil {
		t.Fatal("window statistics must not synthesize a root-cause result")
	}
	wire := ioInflightPublicDecode(t, *result.WindowStats)
	if wire.QueryPID != 99 || len(wire.Groups) != 5 || wire.GroupCount != 5 {
		t.Fatalf("query target relabeled all-issuer population or merged domains: %+v", wire)
	}
	want := map[string]bool{"block|block_rq|8,0|R": true, "block|block_rq|8,0|W": true, "block|block_rq|8,1|R": true, "block|block_bio|8,0|R": true, "scsi|scsi_dispatch_cmd|12,80|read": true}
	for _, g := range wire.Groups {
		key := strings.Join([]string{g.Layer, g.EndpointFamily, g.Dev, g.Operation}, "|")
		if !want[key] || filepath.Base(g.SourcePath) != "grouping.systrace" {
			t.Fatalf("wrong domain/source: %+v", g)
		}
		delete(want, key)
		ioInflightPublicValues(t, g, 1, 1, 1, .3, 3, 3)
	}
	if len(want) != 0 {
		t.Fatalf("lost independent domains: %v", want)
	}
}

func TestIOInflightPublicPhysicalBundleSourcesStaySeparate(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct_captures_and_bundle_isolation", true: "endpoints_in_different_sources"}[split], func(t *testing.T) {
			dir := t.TempDir()
			a := ioInflightPublicRQ(ioInflightPublicPair{1.001, 1.003})
			b := ioInflightPublicRQ(ioInflightPublicPair{1.001, 1.005})
			if split {
				a = strings.SplitAfter(a, "\n")[0]
				b = strings.SplitAfter(b, "\n")[1]
			}
			for name, body := range map[string]string{"a.systrace": a, "b.systrace": b} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				if !split {
					direct, err := BuildIndex(context.Background(), filepath.Join(dir, name))
					if err != nil {
						t.Fatal(err)
					}
					stats := ComputeWindowStats(direct, Query{TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true})
					wire := ioInflightPublicDecode(t, stats)
					if len(wire.Groups) != 1 || filepath.Base(wire.Groups[0].SourcePath) != name {
						t.Fatalf("direct physical identity lost: %+v", wire)
					}
					ms := map[string]float64{"a.systrace": 2, "b.systrace": 4}[name]
					ioInflightPublicValues(t, wire.Groups[0], 1, 1, 1, ms/10, ms, ms)
				}
			}
			manifest := filepath.Join(dir, "capture.tracebundle.json")
			// The normal V2 receipt binds the actual bytes of both files; a
			// legacy manifest must not bypass source-universe admission.
			writeTraceBundleV2ForTest(t, manifest, []byte(`{"version":"test","systrace":"a.systrace","artifacts":[{"type":"systrace","path":"a.systrace"},{"type":"systrace","path":"b.systrace"}]}`))
			idx, err := BuildIndex(context.Background(), manifest)
			if err != nil || idx == nil || len(idx.TraceArtifacts) != 2 {
				t.Fatalf("real manifest did not select two physical sources: %+v %v", idx, err)
			}
			// Byte identity is not shared-capture proof. The existing source
			// admission isolates additional systraces before IO aggregation.
			for _, source := range idx.TraceArtifacts {
				if filepath.Base(source.SourcePath) == "b.systrace" && source.CausalCompatible {
					t.Fatal("additional systrace borrowed shared-capture authority")
				}
			}
			stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true})
			wire := ioInflightPublicDecode(t, stats)
			seen := map[string]bool{}
			for _, group := range wire.Groups {
				if split {
					if group.Values != nil || group.AcceptedPairCount != 0 {
						t.Fatalf("different physical source endpoints paired: %+v", group)
					}
					continue
				}
				name := filepath.Base(group.SourcePath)
				wantMS, exists := map[string]float64{"a.systrace": 2}[name]
				if !exists || seen[name] {
					t.Fatalf("wrong/repeated physical group: %+v", group)
				}
				seen[name] = true
				ioInflightPublicValues(t, group, 1, 1, 1, wantMS/10, wantMS, wantMS)
			}
			if !split && len(seen) != 1 {
				t.Fatalf("primary source was lost or isolated source entered shared count: %+v", wire)
			}
			if split {
				for _, c := range wire.Coverage {
					if c.AcceptedPairCount != 0 {
						t.Fatalf("cross-source endpoints gained accepted population: %+v", c)
					}
				}
			}
		})
	}
}

func TestIOInflightPublicGroupOverflowAndQueryOwnership(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 11; i++ {
		fmt.Fprintf(&body, "io-40 (40) [003] .... 1.001000: block_rq_issue: 8,%d R 4096 () 123 + 8 [io]\n", i)
	}
	for i := 0; i < 11; i++ {
		fmt.Fprintf(&body, "irq-2 (2) [003] .... 1.004000: block_rq_complete: 8,%d R () 123 + 8 [0]\n", i)
	}
	idx := buildTraceIndex(t, "overflow.systrace", body.String())
	before, _ := json.Marshal(idx.Events)
	q := Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true}
	result := Run(idx, q)
	if result.WindowStats == nil {
		t.Fatal("missing window stats")
	}
	wire := ioInflightPublicDecode(t, *result.WindowStats)
	if wire.GroupCount != 11 || len(wire.Groups) != 8 || wire.OmittedGroups != 3 {
		t.Fatalf("bounded output failed to disclose full group population: %+v", wire)
	}
	for _, g := range wire.Groups {
		ioInflightPublicValues(t, g, 1, 1, 1, .3, 3, 3)
	}
	for _, c := range wire.Coverage {
		if c.Family == "block" && c.AcceptedPairCount != 11 {
			t.Fatalf("Top-8 display became endpoint census: %+v", c)
		}
	}
	// Public snapshots cannot mutate shared pairing state or later queries.
	result.WindowStats.IOInFlight.Groups[0].Values.RequestMs = 999
	again := Run(idx, q)
	for _, g := range again.WindowStats.IOInFlight.Groups {
		ioInflightPublicValues(t, g, 1, 1, 1, .3, 3, 3)
	}
	after, _ := json.Marshal(idx.Events)
	if string(before) != string(after) {
		t.Fatal("read-only in-flight query rewrote parsed source events")
	}
}
