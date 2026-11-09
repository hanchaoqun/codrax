package tool

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestHMC364CompanionSamplingTreesKeepPhysicalReferences(t *testing.T) {
	hmc176DisableExternalProviders(t)
	bus, preparer, conversions := hmc17NamedPathContext(t)
	original := filepath.Join(t.TempDir(), "capture.data")
	if err := os.WriteFile(original, hmc363DuplicateRawSamples(), 0600); err != nil {
		t.Fatal(err)
	}
	material, err := preparer.Prepare(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	bus.AttachedTraceMaterial, bus.AttachedHitrace = material, material.Preview()
	args := map[string]any{"source": "attached_trace", "view": "perf_sampling_tree", "pid": 42, "time_start": .99, "time_end": 1.01}
	direct := hmc363RawRecordPayload(t, hmc17NamedQuery(t, bus, args))
	if direct.Tree.SampleCount != 2 || len(direct.Tree.Cohorts) != 1 || direct.Tree.Cohorts[0].RecordSource == nil {
		t.Fatalf("dedicated view must have the original record reference: %+v", direct)
	}
	for _, view := range []string{"perf_stats", "perf_timeline", "window_stats", "trace_perf_bundle"} {
		t.Run(view, func(t *testing.T) {
			args["view"] = view
			for i := 0; i < 2; i++ {
				r := hmc17NamedQuery(t, bus, args)
				got := hmc363RawRecordPayload(t, r)
				if !reflect.DeepEqual(got, direct) {
					t.Fatalf("companion view did not publish the exact direct sample population and physical references: %+v", got)
				}
				decoded := false
				for _, observation := range r.Observations {
					if tree, ok := DecodeTracePerfSamplingTree(observation); ok && len(tree.Cohorts) == 1 && tree.Cohorts[0].RawRecordSource != nil {
						decoded = true
					}
				}
				if !decoded {
					t.Fatal("coordinates arrived in result JSON but not in the bounded native handoff")
				}
			}
		})
	}
	if conversions.Load() != 1 {
		t.Fatal("companion queries prepared the source again")
	}
	// The exact derived bundle and the same path-cache state cannot confer
	// original coordinates on a query without the immutable preparation.
	idx, err := tracequery.BuildIndex(context.Background(), direct.SourcePath)
	if err != nil {
		t.Fatal(err)
	}
	bare := tracequery.Run(idx, tracequery.Query{View: "perf_stats", PID: 42, TimeStart: .99, TimeEnd: 1.01})
	if p := bare.PerfSamplingTree; p == nil || p.SampleCount != 2 || p.Cohorts[0].RawRecordSource != nil || p.Cohorts[0].Nodes[0].References[0].RawRecord != nil {
		t.Fatal("unprepared companion borrowed an original reference or changed the population")
	}
	args["view"], args["pattern"] = "perf_stats", "unmatched"
	filtered := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, args))
	if filtered.PerfSamplingTree != nil && (filtered.PerfSamplingTree.Status != "unavailable" || len(filtered.PerfSamplingTree.Cohorts) != 0) {
		t.Fatal("attaching the original coordinate index dropped a query filter")
	}
}

func TestHMC364WholeOriginalAttachedCompanionRecordPositions(t *testing.T) {
	path := os.Getenv("CODRAX_HMC364_RAW_ORIGINAL")
	if path == "" {
		t.Skip("requires whole original reference capture")
	}
	const wantSHA = "da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632"
	sha, size := hmc176bRepresentativeDigest(t, path)
	if sha != wantSHA || size != 30062585 {
		t.Fatal("not the pinned whole original")
	}
	hmc176DisableExternalProviders(t)
	bus, preparer, conversions := hmc17NamedPathContext(t)
	material, err := preparer.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	bus.AttachedTraceMaterial, bus.AttachedHitrace = material, material.Preview()
	args := map[string]any{"source": "attached_trace", "view": "perf_sampling_tree", "pid": 1609,
		"time_start": 486489.4, "time_end": 486489.6}
	direct := hmc363RawRecordPayload(t, hmc17NamedQuery(t, bus, args))
	if direct.Tree.SampleCount != 164 || len(direct.Tree.Cohorts) != 1 || direct.Tree.Cohorts[0].TotalWeight != 259 {
		t.Fatal("wrong whole-input census")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, view := range []string{"perf_stats", "perf_timeline", "window_stats", "trace_perf_bundle"} {
		args["view"] = view
		got := hmc363RawRecordPayload(t, hmc17NamedQuery(t, bus, args))
		if !reflect.DeepEqual(got, direct) {
			t.Fatalf("%s differs from the direct tree for the same population", view)
		}
		c := got.Tree.Cohorts[0]
		if c.RecordSource == nil || c.RecordSource.Path != path || c.RecordSource.Bytes != size || c.RecordSource.SHA256 != sha {
			t.Fatalf("%s lost the original coordinate space", view)
		}
		count := 0
		for _, node := range c.Nodes {
			for _, ref := range node.Refs {
				var head [8]byte
				if ref.Record == nil {
					t.Fatalf("%s lacks an original record address for a retained ref", view)
				}
				if _, err := file.ReadAt(head[:], int64(ref.Record.Offset)); err != nil || binary.LittleEndian.Uint32(head[:]) != 9 || uint64(binary.LittleEndian.Uint16(head[6:])) != ref.Record.Bytes {
					t.Fatalf("%s has a coordinate that is not the observed binary record: %+v (%v)", view, ref.Record, err)
				}
				count++
			}
		}
		if count != 8 {
			t.Fatalf("%s lost a preview record: %d", view, count)
		}
	}
	if now, n := hmc176bRepresentativeDigest(t, path); now != sha || n != size || conversions.Load() != 1 {
		t.Fatal("original changed or a companion view started another conversion")
	}
}
