package hitraceconv

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

func TestHMC229RawPerfMetadataActualConversionAndJSONBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.data")
	if err := os.WriteFile(path, syntheticRawPerfDataWithFeatures(), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := ConvertFile(t.Context(), Options{InputPath: path, OutputPath: filepath.Join(dir, "out.systrace"), PerfParser: "raw"})
	if err != nil {
		t.Fatal(err)
	}
	fields := r.SourceMetadata().Fields()
	want := map[string]string{"Hostname": "device-01", "CPUDesc": "arm big.LITTLE", "CPUID": "0x41", "WorkloadCmd": "com.example.app"}
	if len(fields) != 4 {
		t.Fatalf("metadata lost in actual conversion: %+v", fields)
	}
	for _, f := range fields {
		if f.State != "observed" || f.Value != want[f.Name] {
			t.Fatalf("wrong original field: %+v", f)
		}
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"WorkloadCmd"`) {
		t.Fatal("diagnostic metadata missing from receipt")
	}
	var restored Result
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.SourceMetadata().Fields()) != 0 {
		t.Fatal("JSON restored trusted metadata")
	}
}

func TestHMC229RawMetadataStrictStatesAndOriginalWhitespace(t *testing.T) {
	field := attachment.TraceSourceMetadataField{Origin: "perf_feature", Kind: "workload", Name: "WorkloadCmd"}
	for _, test := range []struct {
		name         string
		raw          []byte
		state, value string
	}{
		{"observed", rawPerfFeatureString("  app --arg=x  "), "observed", "  app --arg=x  "},
		{"empty", rawPerfFeatureString(""), "observed", ""},
		{"invalid", []byte{20, 0, 0, 0, 'x'}, "invalid", ""},
		{"invalid_utf8", []byte{1, 0, 0, 0, 255}, "invalid", ""},
		{"truncated", rawPerfFeatureString(strings.Repeat("x", 1025)), "truncated", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := decodeRawPerfMetadataField(field, test.raw)
			if got.State != test.state || got.Value != test.value {
				t.Fatalf("%+v", got)
			}
		})
	}
	missing := rawPerfSourceMetadata(nil)
	for _, f := range missing {
		if f.State != "missing" {
			t.Fatal(f)
		}
	}
	// Malformed declaration must not inherit the permissive legacy caveat's
	// clamped string, even though the rest of this valid capture is readable.
	raw := syntheticRawPerfDataWithFeatures()
	desc := int(binary.LittleEndian.Uint64(raw[40:48]) + binary.LittleEndian.Uint64(raw[48:56]))
	pos := int(binary.LittleEndian.Uint64(raw[desc+16 : desc+24])) // hostname is second feature
	binary.LittleEndian.PutUint32(raw[pos:pos+4], 9000)
	path := filepath.Join(t.TempDir(), "capture.data")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := readRawPerfData(t.Context(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Features.SourceMetadata[0].State != "invalid" || parsed.Features.SourceMetadata[0].Value != "" {
		t.Fatal(parsed.Features.SourceMetadata)
	}
}

func TestHMC229SQLiteMetadataStatesAndGenericKeys(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE device_info (physical_width, physical_height, physical_frame_rate)", "INSERT INTO device_info VALUES (1080,NULL,60.0)",
		"CREATE TABLE meta(name,value)", "INSERT INTO meta VALUES ('workload_cmd','  parser only  ')", "INSERT INTO meta VALUES ('revision','a')", "INSERT INTO meta VALUES ('revision','b')",
		"INSERT INTO meta VALUES ('large', CAST(zeroblob(1025) AS TEXT))",
	})
	db, err := openTraceDB(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()
	items, err := inspectTraceDBDiagnosticMetadata(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	device := items[0].SourceMetadata.Fields()
	if len(device) != 3 || device[0].State != "observed" || device[1].State != "missing" || device[2].State != "invalid" {
		t.Fatal(device)
	}
	fields := items[1].SourceMetadata.Fields()
	states := map[string]string{}
	for _, f := range fields {
		if f.Kind != "parser" {
			t.Fatal("generic key promoted", f)
		}
		states[f.Name] = f.State
		if f.Name == "workload_cmd" && f.Value != "  parser only  " {
			t.Fatal(f)
		}
	}
	if states["revision"] != "invalid" || states["large"] != "truncated" || states["workload_cmd"] != "observed" {
		t.Fatal(fields)
	}
}

func TestHMC229SelectedMemberDoesNotPoolCaptureMetadata(t *testing.T) {
	a := attachment.NewTraceSourceMetadata([]attachment.TraceSourceMetadataField{{Origin: "perf_feature", Kind: "device", Name: "Hostname", State: "observed", Value: "host A"}})
	b := attachment.NewTraceSourceMetadata([]attachment.TraceSourceMetadataField{{Origin: "perf_feature", Kind: "device", Name: "Hostname", State: "observed", Value: "host B"}})
	r := Result{BundlePath: "/bundle", Artifacts: []Artifact{{Type: ArtifactPerfTrace, Path: "/a", SourceMetadata: a}, {Type: ArtifactPerfTrace, Path: "/b", SourceMetadata: b}}}
	for path, want := range map[string]string{"/a": "host A", "/b": "host B"} {
		fields := r.SourceMetadataForQueryPath(path).Fields()
		if len(fields) != 1 || fields[0].Value != want {
			t.Fatal(path, fields)
		}
	}
	fields := r.SourceMetadataForQueryPath("/bundle").Fields()
	if len(fields) != 1 || fields[0].Name != "device_workload_attribution" || fields[0].State != "invalid" || fields[0].Value != "" {
		t.Fatal("ambiguous capture pooled member identity", fields)
	}
	if len(r.SourceMetadataForQueryPath("/other").Fields()) != 0 {
		t.Fatal("unselected source borrowed metadata")
	}
}

func TestHMC229DeviceMalformedPayloadIsNotReadAsDiagnosticValue(t *testing.T) {
	path := createTraceDBFixture(t, []string{"CREATE TABLE device_info(physical_width,physical_height,physical_frame_rate)", "INSERT INTO device_info VALUES(CAST(zeroblob(2097152) AS TEXT),zeroblob(2097152),120)"})
	db, err := openTraceDB(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.close()
	coverage, err := inspectTraceDBDeviceInfoMetadata(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	fields := coverage.SourceMetadata.Fields()
	for _, field := range fields[:2] {
		if field.State != "invalid" || field.Value != "" {
			t.Fatal("unbounded/partial diagnostic value", field)
		}
	}
	if fields[2].Value != "120" || fields[2].State != "observed" {
		t.Fatal("bad field poisoned sibling", fields)
	}
}
