package hitraceconv

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

func hmc287SourceRangeFields(t *testing.T, items []TraceDBCoverage) map[string]attachment.TraceSourceMetadataField {
	t.Helper()
	out := map[string]attachment.TraceSourceMetadataField{}
	for _, item := range items {
		for _, field := range item.SourceMetadata.Fields() {
			if field.Kind == "source_range" {
				if field.Origin != "trace_range" || out[field.Name].Name != "" {
					t.Fatalf("ambiguous range provenance: %+v", field)
				}
				out[field.Name] = field
			}
		}
	}
	return out
}

func TestHMC287SQLiteDeclaredRangeStrictStates(t *testing.T) {
	for _, test := range []struct {
		name, schema, rows string
		states             [3]string
		values             [3]string
	}{
		{"valid", "(start_ts,end_ts)", "(10000000000,10100000000)", [3]string{"observed", "observed", "observed"}, [3]string{"10000000000", "10100000000", "100000000"}},
		{"zero", "(start_ts,end_ts)", "(0,0)", [3]string{"observed", "observed", "observed"}, [3]string{"0", "0", "0"}},
		{"max_int", "(start_ts,end_ts)", "(0,9223372036854775807)", [3]string{"observed", "observed", "observed"}, [3]string{"0", "9223372036854775807", "9223372036854775807"}},
		{"null", "(start_ts,end_ts)", "(100,NULL)", [3]string{"observed", "missing", "missing"}, [3]string{"100", "", ""}},
		{"real", "(start_ts,end_ts)", "(1.0,100)", [3]string{"invalid", "observed", "invalid"}, [3]string{"1", "100", ""}},
		{"text", "(start_ts,end_ts)", "('1',100)", [3]string{"invalid", "observed", "invalid"}, [3]string{"", "100", ""}},
		{"blob", "(start_ts,end_ts)", "(zeroblob(2097152),100)", [3]string{"invalid", "observed", "invalid"}, [3]string{"", "100", ""}},
		{"negative", "(start_ts,end_ts)", "(-1,100)", [3]string{"invalid", "observed", "invalid"}, [3]string{"-1", "100", ""}},
		{"reversed", "(start_ts,end_ts)", "(100,99)", [3]string{"invalid", "invalid", "invalid"}, [3]string{"100", "99", ""}},
		{"duplicate", "(start_ts,end_ts)", "(1,100),(1,100)", [3]string{"invalid", "invalid", "invalid"}, [3]string{}},
		{"empty", "(start_ts,end_ts)", "", [3]string{"missing", "missing", "missing"}, [3]string{}},
		{"missing_end", "(start_ts)", "(100)", [3]string{"observed", "missing", "missing"}, [3]string{"100", "", ""}},
		{"absent", "", "", [3]string{"missing", "missing", "missing"}, [3]string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			sql := []string{"CREATE TABLE fixture_anchor (id INTEGER)"}
			if test.schema != "" {
				sql = append(sql, "CREATE TABLE trace_range "+test.schema)
				if test.rows != "" {
					sql = append(sql, "INSERT INTO trace_range VALUES "+test.rows)
				}
			}
			path := createTraceDBFixture(t, sql)
			db, err := openTraceDB(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.close()
			items, err := inspectTraceDBDiagnosticMetadata(t.Context(), db)
			if err != nil {
				t.Fatal(err)
			}
			fields := hmc287SourceRangeFields(t, items)
			if len(fields) != 3 {
				t.Fatalf("original declared range not handed off: %+v", fields)
			}
			for i, name := range []string{"start_ts", "end_ts", "duration_ns"} {
				f := fields[name]
				if f.State != test.states[i] || f.Value != test.values[i] {
					t.Fatalf("%s=%+v; want %s/%q", name, f, test.states[i], test.values[i])
				}
				if !strings.Contains(f.Detail, "ns") {
					t.Fatalf("exact unit lost: %+v", f)
				}
			}
			for _, item := range items {
				if item.Family != "metadata" || item.Table != "trace_range" || item.Role != "diagnostic_metadata" {
					continue
				}
				// The frozen v2 implementation copied observed fields by their
				// names. The literal-key form must preserve the exact same map,
				// including nil on early returns and empty on invalid endpoints.
				var previous map[string]string
				if item.Found && item.RowsRead == 1 {
					previous = map[string]string{}
					for _, field := range item.SourceMetadata.Fields() {
						if field.State == "observed" {
							previous[field.Name] = field.Value
						}
					}
				}
				if !reflect.DeepEqual(item.Metadata, previous) {
					t.Fatalf("literal projection differs from frozen field projection: got=%#v want=%#v", item.Metadata, previous)
				}
				// Stable full structural receipts permit a zero-model replay
				// against the sealed v2 source; only wall-clock timing is omitted.
				item.ElapsedUS = 0
				receipt, err := json.Marshal(item)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("HMC287_PARITY range/%s %x", test.name, sha256.Sum256(receipt))
			}
		})
	}
}

func TestHMC287ActualConversionSeparatesDeclaredRangeFromExcerpt(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_catalog_objects/captures/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	r, err := PrepareExistingTraceDB(t.Context(), Options{InputPath: path, OutputPath: filepath.Join(t.TempDir(), "capture.systrace")})
	if err != nil {
		t.Fatal(err)
	}
	fields := hmc287SourceRangeFields(t, r.TraceDBCoverage)
	if fields["duration_ns"].Value != "100000000" || fields["end_ts"].Value != "10100000000" {
		t.Fatalf("source range lost: %+v", fields)
	}
	if r.FirstTimestampSec != 10 || r.LastTimestampSec != 10.05 {
		t.Fatalf("declared range replaced converted event envelope: %v..%v", r.FirstTimestampSec, r.LastTimestampSec)
	}
	selected := r.SourceMetadataForQueryPath(r.OutputPath).Fields()
	found := false
	for _, f := range selected {
		found = found || f.Kind == "source_range" && f.Name == "duration_ns" && f.Value == "100000000"
	}
	if !found {
		t.Fatalf("query member lost its original range: %+v", selected)
	}
	if len(r.SourceMetadataForQueryPath("/unrelated").Fields()) != 0 {
		t.Fatal("unrelated member borrowed range")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var restored Result
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.SourceMetadata().Fields()) != 0 {
		t.Fatal("JSON diagnostic recreated preparation metadata")
	}
	body, err := os.ReadFile(r.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("HMC287_PARITY actual_conversion/systrace bytes=%d sha256=%x", len(body), sha256.Sum256(body))
}
