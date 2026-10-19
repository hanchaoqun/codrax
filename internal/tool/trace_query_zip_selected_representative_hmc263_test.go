package tool

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// A single real capture is copied into two operator-created ZIP members.
// Equal payload bytes do not make the member coordinates interchangeable, and
// this packaging is not evidence of two independently collected captures.
func TestHMC263ZIPSelectedRepresentativeCompletePublicQueries(t *testing.T) {
	source := strings.TrimSpace(os.Getenv("CODRAX_TEST_ZIP_PERF_SOURCE"))
	if source == "" {
		t.Skip("set CODRAX_TEST_ZIP_PERF_SOURCE to pinned reference test_brbe.data")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	sha, size := hmc176bRepresentativeDigest(t, source)
	if sha != "da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632" || size != 30062585 {
		t.Fatal("reference differs from independent oracle")
	}
	entries := hmc176bRepresentativeSourceEntries(t, filepath.Dir(source))
	t.Cleanup(func() {
		after, err := os.Stat(source)
		sum, n := hmc176bRepresentativeDigest(t, source)
		if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || sum != sha || n != size || !reflect.DeepEqual(entries, hmc176bRepresentativeSourceEntries(t, filepath.Dir(source))) {
			t.Error("reference changed")
		}
	})
	hmc176DisableExternalProviders(t)
	path := filepath.Join(t.TempDir(), "operator-multiple.zip")
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(out)
	names := []string{"captures/device capture.data", "control/device capture.data"}
	for _, name := range names {
		in, err := os.Open(source)
		if err != nil {
			t.Fatal(err)
		}
		member, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, copyErr := io.Copy(member, in)
		closeErr := in.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatalf("copy: %v %v", copyErr, closeErr)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	zipSHA, zipBytes := hmc176bRepresentativeDigest(t, path)
	bus, prep, conversions := hmc17NamedPathContext(t)
	if out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "perf_stats"}); out.Success || !strings.Contains(out.Summary, "multiple_trace_members") {
		t.Fatal("ambiguous real ZIP selected a first member")
	}
	paths := map[string]string{}
	for _, name := range names {
		args := map[string]any{"source": "path", "path": path, "archive_member": name, "view": "perf_stats"}
		full := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, args))
		if full.EventCount != 12000 || full.PerfStats == nil || full.PerfStats.SampleCount != 12000 {
			t.Fatal("full sample member lost")
		}
		args["pid"], args["time_start"], args["time_end"] = 1609, 486489.4, 486489.6
		hmc260CheckSelectedRepresentative(t, hmc17NamedPayload(t, hmc17NamedQuery(t, bus, args)))
		args["view"] = "perf_sampling_tree"
		tree := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, args)).PerfSamplingTree
		if tree == nil || tree.SampleCount != 164 || len(tree.Cohorts) != 1 {
			t.Fatalf("tree=%+v", tree)
		}
		c := tree.Cohorts[0]
		if c.TotalWeight != 259 || c.WeightUnit != "cycles" || c.Event != "hw-cpu-cycles" || c.NodeCount != 2 || len(c.Nodes) != 2 || (c.SampleKind != "" && c.SampleKind != "unknown") || c.ThreadIdentity == nil || c.ThreadIdentity.TID != 1609 || c.ThreadIdentity.TGID != 1514 {
			t.Fatalf("tree scope changed: %+v", c)
		}
		for _, node := range c.Nodes {
			if node.InclusiveSamples != 82 || node.ReferenceCount != 82 || len(node.References)+node.OmittedReferences != 82 {
				t.Fatal("bounded tree references replaced complete membership")
			}
		}
		m, err := prep.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: name})
		if err != nil {
			t.Fatal(err)
		}
		p := hmc176bReadBundle(t, m.QueryPath()).ArchiveProvenance
		if p == nil || p.Member != name || p.Selection != "explicit_member" || p.MemberSHA256 != sha || p.MemberBytes != size || p.ArchiveSHA256 != zipSHA {
			t.Fatalf("wrong original/member: %+v", p)
		}
		paths[name] = m.QueryPath()
	}
	if paths[names[0]] == paths[names[1]] || conversions.Load() != 3 || len(prep.PreparedMaterials()) != 2 {
		t.Fatal("real member identity/warm cache changed")
	}
	endSHA, endSize := hmc176bRepresentativeDigest(t, path)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0400 || endSHA != zipSHA || endSize != zipBytes {
		t.Fatal("operator ZIP changed")
	}
	t.Logf("operator-created ZIP, one independent real capture in two exact members; each full=12000; tid1609/tgid1514=164 samples/259 cycles, CPU0=82/85 CPU1=82/174; unknown kind; full tree=2 nodes/164 refs; preparations=1 rejected ambiguity+2 selected; source sha256=%s bytes=%d; ZIP sha256=%s bytes=%d", sha, size, zipSHA, zipBytes)
}
