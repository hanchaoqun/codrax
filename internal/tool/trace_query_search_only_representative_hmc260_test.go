package tool

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Opt-in real-capture acceptance of the already implemented Darwin namespace
// boundary. This is not a new decoder, a model evaluation, or BRBE acceptance.
func TestHMC260SearchOnlyRepresentativePerfSamples(t *testing.T) {
	source := strings.TrimSpace(os.Getenv("CODRAX_TEST_GZIP_PERF_SOURCE"))
	if source == "" {
		t.Skip("set CODRAX_TEST_GZIP_PERF_SOURCE to the pinned reference test_brbe.data capture")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("native search-only directory-entry witness targets Darwin")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(source)
	if err != nil || !original.Mode().IsRegular() {
		t.Fatalf("expected readable reference original: %v", err)
	}
	sha, size := hmc176bRepresentativeDigest(t, source)
	if sha != "da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632" || size != 30062585 {
		t.Fatal("reference capture differs from independently measured oracle")
	}
	entries := hmc176bRepresentativeSourceEntries(t, filepath.Dir(source))
	t.Cleanup(func() {
		after, err := os.Stat(source)
		actualSHA, actualSize := hmc176bRepresentativeDigest(t, source)
		if err != nil || !os.SameFile(original, after) || original.Mode() != after.Mode() || !original.ModTime().Equal(after.ModTime()) || actualSHA != sha || actualSize != size || !reflect.DeepEqual(entries, hmc176bRepresentativeSourceEntries(t, filepath.Dir(source))) {
			t.Error("reference original or its namespace changed")
		}
	})
	hmc176DisableExternalProviders(t)
	for _, wrapped := range []bool{false, true} {
		name := map[bool]string{false: "raw", true: "gzip"}[wrapped]
		t.Run(name, func(t *testing.T) {
			outer := filepath.Join(t.TempDir(), "search-only parent")
			inner := filepath.Join(outer, "原件 capture")
			if err := os.MkdirAll(inner, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(inner, "named capture.data")
			if wrapped {
				hmc176bGzipRepresentativeSource(t, source, path)
			} else {
				hmc260CopyRepresentative(t, source, path)
			}
			if err := os.Chmod(path, 0400); err != nil {
				t.Fatal(err)
			}
			copySHA, copySize := hmc176bRepresentativeDigest(t, path)
			for _, dir := range []string{inner, outer} {
				if err := os.Chmod(dir, 0100); err != nil {
					t.Fatal(err)
				}
				// Registered in this order so the ancestor is restored first.
				t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
				if _, err := os.ReadDir(dir); !os.IsPermission(err) {
					t.Fatalf("explicit native acceptance requires enforced listing denial: %v", err)
				}
			}
			bus, preparer, conversions := hmc17NamedPathContext(t)
			bus.AttachedHitrace = hmc176UnrelatedText
			query := func(view string, selected bool) tracequery.Result {
				params := map[string]any{"source": "path", "path": path, "view": view}
				if selected {
					params["time_start"], params["time_end"], params["pid"] = 486489.4, 486489.6, 1609
				}
				return hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			}
			full := query("perf_stats", false)
			if full.EventCount != 12000 || full.PerfStats == nil || full.PerfStats.SampleCount != 12000 {
				t.Fatalf("complete capture became preview: %+v", full.PerfStats)
			}
			// Cohorts have an independent display limit. Do not add the
			// visible cohort preview and call it the full physical population.
			quality := full.PerfStats.Quality
			if quality == nil || len(quality.WeightUnits) != 1 || quality.WeightUnits[0].SampleCount != 12000 || full.PerfStats.TotalPeriod != 0 || full.PerfStats.CohortCount <= len(full.PerfStats.Cohorts) {
				t.Fatal("global inventory lost its independent-cohort preview boundary")
			}
			material, err := preparer.Prepare(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			// Independently inspect every published member, never infer full
			// record weight from the intentionally bounded cohort projection.
			idx, err := tracequery.BuildIndex(context.Background(), material.QueryPath())
			if err != nil || len(idx.Events) != 12000 {
				t.Fatalf("complete prepared member census failed: %v", err)
			}
			var physicalWeight int64
			windowSamples := 0
			for _, event := range idx.Events {
				if event.Type != tracequery.EventPerfSample || event.PerfFields == nil || (event.PerfFields.SampleKind != "" && event.PerfFields.SampleKind != "unknown") {
					t.Fatal("sample-only member gained another event/state family")
				}
				physicalWeight += event.PerfFields.Period
				if event.Ts >= 486489.4 && event.Ts < 486489.6 {
					windowSamples++
				}
			}
			if physicalWeight != 43037682 || windowSamples != 1393 {
				t.Fatalf("complete member counts disagree with independent binary oracle: weight=%d window=%d", physicalWeight, windowSamples)
			}
			selected := query("perf_stats", true)
			hmc260CheckSelectedRepresentative(t, selected)
			tree := query("perf_sampling_tree", true).PerfSamplingTree
			if tree == nil || tree.Status != "available" || tree.SampleCount != 164 || len(tree.Cohorts) != 1 {
				t.Fatalf("complete selected tree unavailable: %+v", tree)
			}
			cohort := tree.Cohorts[0]
			if (cohort.SampleKind != "" && cohort.SampleKind != "unknown") || cohort.TotalWeight != 259 || cohort.NodeCount != 2 || len(cohort.Nodes) != 2 || cohort.ThreadIdentity == nil || cohort.ThreadIdentity.TID != 1609 || cohort.ThreadIdentity.TGID != 1514 {
				t.Fatalf("tree identity/state/denominator changed: %+v", cohort)
			}
			for _, node := range cohort.Nodes {
				if node.InclusiveSamples != 82 || node.ReferenceCount != 82 || node.OmittedReferences+len(node.References) != 82 {
					t.Fatalf("tree replaced complete members with preview: %+v", node)
				}
			}
			warm := query("perf_stats", true)
			hmc260CheckSelectedRepresentative(t, warm)
			material, err = preparer.Prepare(context.Background(), path)
			if err != nil || material.SourcePath() != path || material.QueryPath() == path || material.QueryPath() != selected.SourcePath || conversions.Load() != 1 || len(preparer.PreparedMaterials()) != 1 {
				t.Fatalf("source identity or warm reuse changed: conversions=%d err=%v", conversions.Load(), err)
			}
			if bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("named query replaced unrelated attachment")
			}
			actualSHA, actualSize := hmc176bRepresentativeDigest(t, path)
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0400 || actualSHA != copySHA || actualSize != copySize {
				t.Fatal("read-only selected original changed")
			}
			for _, dir := range []string{outer, inner} {
				if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0100 {
					t.Fatalf("production changed search-only directory permissions: %v", err)
				}
			}
			t.Logf("%s: original_sha256=%s bytes=%d; full=12000/43037682 target=164/259 CPU0=82/85 CPU1=82/174 unknown; two-level-search-only cold/warm preparations=1", name, copySHA, copySize)
		})
	}
}

func hmc260CheckSelectedRepresentative(t *testing.T, p tracequery.Result) {
	t.Helper()
	s := p.PerfStats
	if s == nil || s.SampleCount != 164 || len(s.Cohorts) != 1 || s.Cohorts[0].TotalPeriod != 259 || s.Quality == nil {
		t.Fatalf("selected native population changed: %+v", s)
	}
	if p.TimeStart != 486489.4 || p.TimeEnd != 486489.6 || p.WindowStats == nil || len(p.WindowStats.TopRunning) != 0 || len(p.WindowStats.RunnableTop) != 0 || p.WindowStats.CPUOccupancy != nil {
		t.Fatal("sample-only population gained scheduling authority or lost query window")
	}
	if !reflect.DeepEqual(s.Quality.SampleKinds, []tracequery.PerfValueCount{{Value: "unknown", SampleCount: 164, Period: 259, Percent: 100}}) {
		t.Fatalf("unknown recording type became execution state: %+v", s.Quality.SampleKinds)
	}
	if s.Quality.CPUKnownCount != 164 || s.Quality.CPUUnknownCount != 0 || len(s.Quality.RecordedCPUs) != 2 {
		t.Fatalf("recorded CPU inventory changed: %+v", s.Quality)
	}
	for _, cpu := range s.Quality.RecordedCPUs {
		want := map[string]int64{"cpu:0": 85, "cpu:1": 174}
		if cpu.SampleCount != 82 || want[cpu.Value] != cpu.Period {
			t.Fatalf("recorded CPU count/weight changed: %+v", cpu)
		}
	}
	if len(s.Cohorts[0].TopSymbols) != 2 {
		t.Fatalf("selected physical hotspots missing: %+v", s.Cohorts[0].TopSymbols)
	}
	for _, h := range s.Cohorts[0].TopSymbols {
		// Recorded CPU coordinates are inventory above, not execution-CPU
		// attribution. Unknown sample kind must leave execution CPUs empty.
		knownSymbol := strings.Contains(h.Symbol, "OnDataEvent_") && h.Period == 85
		fallbackIP := h.Symbol == "0xffffff800b829d24" && h.Period == 174 && h.SymbolizationStatus == "unsymbolized"
		if h.SampleCount != 82 || len(h.CPUs) != 0 || (!knownSymbol && !fallbackIP) {
			t.Fatalf("hotspot count/weight or execution authority changed: %+v", h)
		}
	}
}

func hmc260CopyRepresentative(t *testing.T, source, destination string) {
	t.Helper()
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
