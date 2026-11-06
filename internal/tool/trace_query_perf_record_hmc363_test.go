package tool

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Same producer sample/stream IDs, timestamp, TID, CPU, IP and weight; only
// the two physical record headers distinguish these samples.
func hmc363DuplicateRawSamples() []byte {
	base := hmc176PerfData(true)
	const data = 152
	original := base[data:]
	header := append([]byte(nil), base[:data]...)
	typ := binary.LittleEndian.Uint64(header[128:]) | 1<<6 | 1<<9
	binary.LittleEndian.PutUint64(header[128:], typ)
	record := make([]byte, 64)
	copy(record, original[:32])
	copy(record[48:], original[32:])
	binary.LittleEndian.PutUint64(record[32:], 40076)
	binary.LittleEndian.PutUint64(record[40:], 40076)
	binary.LittleEndian.PutUint16(record[6:], uint16(len(record)))
	binary.LittleEndian.PutUint64(header[48:], 2*uint64(len(record)))
	return append(append(header, record...), record...)
}

type hmc363RawRecordSpan struct {
	Offset uint64 `json:"header_offset"`
	Bytes  uint64 `json:"record_bytes"`
}

type hmc363RawRecordResult struct {
	SourcePath string `json:"source_path"`
	Tree       struct {
		SampleCount int `json:"sample_count"`
		Cohorts     []struct {
			TotalWeight  int `json:"total_weight"`
			RecordSource *struct {
				Path   string `json:"path"`
				SHA256 string `json:"sha256"`
				Bytes  int64  `json:"bytes"`
			} `json:"raw_record_source"`
			Nodes []struct {
				IP     string `json:"ip"`
				Weight int    `json:"inclusive_weight"`
				Count  int    `json:"reference_count"`
				Refs   []struct {
					Line   int                  `json:"line"`
					Record *hmc363RawRecordSpan `json:"raw_record"`
				} `json:"references"`
			} `json:"nodes"`
		} `json:"cohorts"`
	} `json:"perf_sampling_tree"`
}

func TestHMC363WholeRawBinaryRecordLocator(t *testing.T) {
	path := os.Getenv("CODRAX_HMC363_RAW_ORIGINAL")
	if path == "" {
		t.Skip("requires the pinned whole reference capture")
	}
	const wantSHA = "da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632"
	if sha, size := hmc176bRepresentativeDigest(t, path); sha != wantSHA || size != 30062585 {
		t.Fatal("not the whole reference capture")
	}
	hmc176DisableExternalProviders(t)
	bus, _, conversions := hmc17NamedPathContext(t)
	args := map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "pid": 1609,
		"time_start": 486489.4, "time_end": 486489.6, "sampling_depth": 8}
	got := hmc363RawRecordPayload(t, hmc17NamedQuery(t, bus, args))
	if got.Tree.SampleCount != 164 || len(got.Tree.Cohorts) != 1 || conversions.Load() != 1 {
		t.Fatalf("wrong raw sample population: samples=%d cohorts=%d conversions=%d", got.Tree.SampleCount, len(got.Tree.Cohorts), conversions.Load())
	}
	c := got.Tree.Cohorts[0]
	if c.TotalWeight != 259 || len(c.Nodes) != 2 || c.RecordSource == nil || c.RecordSource.Path != path || c.RecordSource.SHA256 != wantSHA || c.RecordSource.Bytes != 30062585 {
		t.Fatalf("lost original source, tree nodes, or exact weight: %+v", c.RecordSource)
	}
	// Independent physical walk from §362's sealed oracle. The original is
	// opened read-only and each displayed byte span is checked against its
	// PERF_RECORD_SAMPLE header, not the decoder's line numbering or sample IDs.
	want := map[string]struct {
		weight int
		spans  []hmc363RawRecordSpan
	}{
		"0x5a98ac9ed4":       {85, []hmc363RawRecordSpan{{2532255, 96}, {2532351, 864}, {2533215, 864}, {2534079, 864}}},
		"0xffffff800b829d24": {174, []hmc363RawRecordSpan{{3643901, 872}, {3644773, 872}, {3645645, 872}, {3646517, 872}}},
	}
	original, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	for _, node := range c.Nodes {
		w, ok := want[node.IP]
		if !ok || node.Weight != w.weight || node.Count != 82 || len(node.Refs) != len(w.spans) {
			t.Fatalf("wrong sampled member/weight/preview: %+v", node)
		}
		delete(want, node.IP)
		for i, r := range node.Refs {
			if r.Record == nil || *r.Record != w.spans[i] {
				t.Fatalf("wrong physical coordinate at %s[%d]: %+v", node.IP, i, r.Record)
			}
			var header [8]byte
			if _, err := original.ReadAt(header[:], int64(r.Record.Offset)); err != nil || binary.LittleEndian.Uint32(header[:]) != 9 || uint64(binary.LittleEndian.Uint16(header[6:])) != r.Record.Bytes {
				t.Fatalf("coordinate does not locate a raw sample header: %+v / %v", r.Record, err)
			}
		}
	}
	if sha, n := hmc176bRepresentativeDigest(t, path); sha != wantSHA || n != 30062585 || len(want) != 0 {
		t.Fatal("source changed or a whole member was lost")
	}
}

func hmc363RawRecordPayload(t *testing.T, r types.ToolResult) hmc363RawRecordResult {
	t.Helper()
	for _, o := range r.Observations {
		if o.Predicate != TracePerfSamplingTreePredicate || o.SourceRef.PayloadRef == "" {
			continue
		}
		var out hmc363RawRecordResult
		raw, err := os.ReadFile(o.SourceRef.PayloadRef)
		if err != nil || json.Unmarshal(raw, &out) != nil {
			t.Fatalf("native result: %v", err)
		}
		return out
	}
	t.Fatalf("native sampling tree not published: %s", r.Summary)
	return hmc363RawRecordResult{}
}

func TestHMC363PreparedRawBinaryRecordLocatorPublic(t *testing.T) {
	hmc176DisableExternalProviders(t)
	bus, preparer, conversions := hmc17NamedPathContext(t)
	original := filepath.Join(t.TempDir(), "capture.data")
	body := hmc363DuplicateRawSamples()
	if err := os.WriteFile(original, body, 0600); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"source": "path", "path": original, "view": "perf_sampling_tree", "pid": 42, "time_start": .99, "time_end": 1.01}
	got := hmc363RawRecordPayload(t, hmc17NamedQuery(t, bus, args))
	if got.Tree.SampleCount != 2 || len(got.Tree.Cohorts) != 1 || got.Tree.Cohorts[0].TotalWeight != 198 || len(got.Tree.Cohorts[0].Nodes) != 1 {
		t.Fatal("bad original raw sample population")
	}
	cohort := got.Tree.Cohorts[0]
	if cohort.RecordSource == nil {
		t.Fatal("prepared original record coordinates were dropped before the native query payload")
	}
	sha, n := hmc176bRepresentativeDigest(t, original)
	if cohort.RecordSource.Path != original || cohort.RecordSource.SHA256 != sha || cohort.RecordSource.Bytes != n || got.SourcePath == original || conversions.Load() != 1 {
		t.Fatalf("original and derived generations were conflated: %+v", cohort.RecordSource)
	}
	refs := cohort.Nodes[0].Refs
	if len(refs) != 2 || refs[0].Line == refs[1].Line {
		t.Fatal("physical records collapsed before reference publication")
	}
	for i, ref := range refs {
		if ref.Record == nil || *ref.Record != (hmc363RawRecordSpan{Offset: uint64(152 + i*64), Bytes: 64}) {
			t.Fatalf("not the original record header at %d: %+v", i, ref.Record)
		}
	}
	// A manifest and producer fields do not restore the prepared original's
	// authority. The same derived query, without preparation, keeps facts and
	// derived lines but cannot expose the raw record as a current original.
	bare, err := tracequery.StreamPerfSamplingTree(context.Background(), got.SourcePath, tracequery.Query{View: tracequery.ViewPerfSamplingTree, PID: 42, TimeStart: .99, TimeEnd: 1.01})
	if err != nil || bare.PerfSamplingTree == nil || bare.PerfSamplingTree.SampleCount != 2 {
		t.Fatalf("ordinary derived query changed: %v", err)
	}
	raw, _ := json.Marshal(bare)
	var unbound hmc363RawRecordResult
	if err := json.Unmarshal(raw, &unbound); err != nil || unbound.Tree.Cohorts[0].RecordSource != nil || unbound.Tree.Cohorts[0].Nodes[0].Refs[0].Record != nil {
		t.Fatal("unbound derived artifact granted original record location")
	}
	compressed := filepath.Join(t.TempDir(), "capture.data.gz")
	if err := os.WriteFile(compressed, hmc176Gzip(t, body), 0600); err != nil {
		t.Fatal(err)
	}
	args["path"] = compressed
	zipped := hmc363RawRecordPayload(t, hmc17NamedQuery(t, bus, args))
	if zipped.Tree.SampleCount != 2 || zipped.Tree.Cohorts[0].RecordSource != nil || zipped.Tree.Cohorts[0].Nodes[0].Refs[0].Record != nil {
		t.Fatal("decoded offset was re-labelled as an outer gzip offset")
	}
	material, err := preparer.Prepare(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	bound, ok := material.RawPerfRecordBinding(context.Background(), material.QueryPath())
	if !ok {
		t.Fatal("no direct physical source proof")
	}
	// Change the original generation without changing its bytes. A held,
	// still readable derived file cannot stand in for the changed original.
	replacement := filepath.Join(filepath.Dir(original), "replacement.data")
	if err := os.WriteFile(replacement, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, original); err != nil {
		t.Fatal(err)
	}
	staleQuery := tracequery.Query{View: tracequery.ViewPerfSamplingTree, PID: 42, TimeStart: .99, TimeEnd: 1.01}.WithRawPerfRecordBinding(&bound)
	if stale, err := tracequery.StreamPerfSamplingTree(context.Background(), material.QueryPath(), staleQuery); err == nil || !strings.Contains(err.Error(), "original perf record source changed") || stale.PerfSamplingTree != nil {
		t.Fatalf("changed original inherited positions: %v", err)
	}
}
