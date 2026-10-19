package tool

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// The member is a real device-derived PERFILE2 capture. Its ZIP envelope is
// deliberately operator-created here, not represented as a device ZIP. No
// BRBE semantic decoding or scheduling/execution evidence is inferred.
func TestHMC262ZIPRepresentativeCompletePublicQueries(t *testing.T) {
	source := strings.TrimSpace(os.Getenv("CODRAX_TEST_ZIP_PERF_SOURCE"))
	if source == "" {
		t.Skip("set CODRAX_TEST_ZIP_PERF_SOURCE to the pinned reference test_brbe.data")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(source)
	if err != nil || !before.Mode().IsRegular() {
		t.Fatalf("reference is not a regular readable capture: %v", err)
	}
	sha, size := hmc176bRepresentativeDigest(t, source)
	if sha != "da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632" || size != 30062585 {
		t.Fatal("reference differs from pinned independent oracle")
	}
	entries := hmc176bRepresentativeSourceEntries(t, filepath.Dir(source))
	t.Cleanup(func() {
		after, err := os.Stat(source)
		afterSHA, afterSize := hmc176bRepresentativeDigest(t, source)
		if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || sha != afterSHA || size != afterSize || !reflect.DeepEqual(entries, hmc176bRepresentativeSourceEntries(t, filepath.Dir(source))) {
			t.Error("reference original or its namespace changed")
		}
	})
	hmc176DisableExternalProviders(t)
	const member = "captures/device capture.data"
	zipPath := filepath.Join(t.TempDir(), "人工包装 采集.zip")
	hmc262ZipRepresentative(t, source, zipPath, member)
	zipSHA, zipBytes := hmc176bRepresentativeDigest(t, zipPath)
	for _, tc := range []struct{ name, path string }{{"operator_zip", zipPath}, {"bare_reference", source}} {
		t.Run(tc.name, func(t *testing.T) {
			bus, preparer, conversions := hmc17NamedPathContext(t)
			bus.AttachedHitrace = hmc176UnrelatedText
			query := func(view string, selected bool) tracequery.Result {
				params := map[string]any{"source": "path", "path": tc.path, "view": view}
				if selected {
					params["pid"], params["time_start"], params["time_end"] = 1609, 486489.4, 486489.6
				}
				return hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			}
			full := query("perf_stats", false)
			if full.EventCount != 12000 || full.PerfStats == nil || full.PerfStats.SampleCount != 12000 || full.PerfStats.Quality == nil {
				t.Fatal("full capture replaced with bounded preview")
			}
			// Global count and visible cohort preview have different coverage;
			// never add displayed cohorts into an invented global denominator.
			if full.PerfStats.TotalPeriod != 0 || full.PerfStats.CohortCount <= len(full.PerfStats.Cohorts) || len(full.PerfStats.Quality.WeightUnits) != 1 || full.PerfStats.Quality.WeightUnits[0].SampleCount != 12000 {
				t.Fatal("independent-cohort completeness boundary changed")
			}
			material, err := preparer.Prepare(t.Context(), tc.path)
			if err != nil {
				t.Fatal(err)
			}
			idx, err := tracequery.BuildIndex(t.Context(), material.QueryPath())
			if err != nil || len(idx.Events) != 12000 {
				t.Fatalf("full member census failed: %v", err)
			}
			var recordWeight int64
			windowSamples := 0
			for _, event := range idx.Events {
				if event.Type != tracequery.EventPerfSample || event.PerfFields == nil || (event.PerfFields.SampleKind != "" && event.PerfFields.SampleKind != "unknown") {
					t.Fatal("sample-only capture acquired another event or execution state")
				}
				recordWeight += event.PerfFields.Period
				if event.Ts >= 486489.4 && event.Ts < 486489.6 {
					windowSamples++
				}
			}
			if recordWeight != 43037682 || windowSamples != 1393 {
				t.Fatalf("full original population changed: record_weight=%d window_samples=%d", recordWeight, windowSamples)
			}
			selected := query("perf_stats", true)
			hmc260CheckSelectedRepresentative(t, selected)
			tree := query("perf_sampling_tree", true).PerfSamplingTree
			if tree == nil || tree.Status != "available" || tree.SampleCount != 164 || len(tree.Cohorts) != 1 {
				t.Fatalf("exact target tree missing: %+v", tree)
			}
			c := tree.Cohorts[0]
			if c.TotalWeight != 259 || c.NodeCount != 2 || len(c.Nodes) != 2 || (c.SampleKind != "" && c.SampleKind != "unknown") || c.ThreadIdentity == nil || c.ThreadIdentity.TID != 1609 || c.ThreadIdentity.TGID != 1514 {
				t.Fatalf("tree denominator/state/identity changed: %+v", c)
			}
			scope := selected.PerfStats.Cohorts[0].Scope
			if scope == nil || scope.ID != c.ID || scope.SourcePath != c.SourcePath || scope.SourceGeneration != c.SourceGeneration || c.SourceGeneration == "" || c.Event != "hw-cpu-cycles" || c.WeightUnit != "cycles" || c.Clock != "perf_data" {
				t.Fatal("legacy statistics and lossless tree lost their shared source/population/unit")
			}
			for _, node := range c.Nodes {
				if node.InclusiveSamples != 82 || node.ReferenceCount != 82 || len(node.References)+node.OmittedReferences != 82 {
					t.Fatalf("tree preview replaced complete membership: %+v", node)
				}
			}
			hmc260CheckSelectedRepresentative(t, query("perf_stats", true))
			if conversions.Load() != 1 || len(preparer.PreparedMaterials()) != 1 || material.SourcePath() != tc.path || material.QueryPath() == tc.path || material.SelfContainedText() || bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("cold/warm source identity or attachment isolation changed")
			}
			if tc.name == "operator_zip" {
				bundle := hmc176bReadBundle(t, material.QueryPath())
				p := bundle.ArchiveProvenance
				if p == nil || p.Member != member || p.MemberSHA256 != sha || p.MemberBytes != size || p.ArchiveSHA256 != zipSHA || p.ArchiveBytes != zipBytes || p.Selection != "unique_candidate" || bundle.Systrace != "" || len(bundle.Artifacts) != 1 || bundle.Artifacts[0].Type != hitraceconv.ArtifactPerfTrace {
					t.Fatalf("ZIP/member identity or semantic capability wrong: %+v", bundle)
				}
				var receipt struct {
					SourcePath       string             `json:"source_path"`
					SourceGeneration string             `json:"source_generation"`
					Conversion       hitraceconv.Result `json:"conversion"`
				}
				body, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(body, &receipt); err != nil {
					t.Fatal(err)
				}
				if receipt.SourcePath != zipPath || receipt.SourceGeneration == "" || !reflect.DeepEqual(receipt.Conversion.ArchiveProvenance, p) {
					t.Fatal("prepared receipt lost source generation/archive binding")
				}
			}
			t.Logf("%s full records=12000 raw-record-weight=43037682 (not pooled cohort denominator); window records=1393; tid1609/tgid1514 samples=164 weight=259 CPU0=82/85 CPU1=82/174; unknown kind; tree nodes=2 full refs=82+82; cold/warm preparations=1", tc.name)
		})
	}
	finalSHA, finalBytes := hmc176bRepresentativeDigest(t, zipPath)
	info, err := os.Stat(zipPath)
	if err != nil || info.Mode().Perm() != 0400 || finalSHA != zipSHA || finalBytes != zipBytes {
		t.Fatal("read-only operator ZIP changed")
	}
	t.Logf("real member sha256=%s bytes=%d; operator ZIP sha256=%s bytes=%d; device_zip_capture=false", sha, size, zipSHA, zipBytes)
}

func hmc262ZipRepresentative(t *testing.T, source, target, name string) {
	t.Helper()
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	w := zip.NewWriter(out)
	m, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(m, in); err != nil {
		t.Fatal(err)
	}
	note, err := w.Create("README.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(note, "Operator-created transport around an unchanged device-derived sample capture."); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0400); err != nil {
		t.Fatal(err)
	}
}
