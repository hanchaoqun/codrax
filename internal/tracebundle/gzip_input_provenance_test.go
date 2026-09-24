package tracebundle

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func validGzipInputProvenanceForTest() *GzipInputProvenance {
	return &GzipInputProvenance{
		Profile:     GzipInputProfileV1,
		SourceBytes: 100, SourceSHA256: strings.Repeat("a", 64), SourceGeneration: "opaque-source-generation",
		DecodedFormat: "linux_perf_data", DecodedBytes: 120,
		DecodedSHA256: strings.Repeat("b", 64), DecodedGeneration: "opaque-decoded-generation",
	}
}

func TestGzipInputProvenanceAcceptsClosedFormatsAndInclusiveLimits(t *testing.T) {
	if err := ValidateGzipInputProvenance(nil); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"harmony_rmq", "openharmony_profiler", "linux_perf_data", "simpleperf_report_sample_proto", "openharmony_raw", "sqlite"} {
		t.Run(format, func(t *testing.T) {
			value := validGzipInputProvenanceForTest()
			value.DecodedFormat = format
			if err := ValidateGzipInputProvenance(value); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, limits := range []struct{ source, decoded int64 }{
		{1, 1}, {1, 1000}, {64 << 30, 64 << 30}, {64 << 30, 1},
	} {
		value := validGzipInputProvenanceForTest()
		value.SourceBytes, value.DecodedBytes = limits.source, limits.decoded
		value.SourceGeneration = strings.Repeat("x", gzipInputGenerationMaxBytes)
		value.DecodedGeneration = "generation-vNext:opaque"
		if err := ValidateGzipInputProvenance(value); err != nil {
			t.Fatalf("inclusive limit rejected: %+v: %v", limits, err)
		}
	}
}

func TestGzipInputProvenanceRejectsClosedTupleDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*GzipInputProvenance)
	}{
		{"profile missing", func(v *GzipInputProvenance) { v.Profile = "" }},
		{"profile padded", func(v *GzipInputProvenance) { v.Profile += " " }},
		{"standalone profile", func(v *GzipInputProvenance) { v.Profile = "gzip_perf_data_v1" }},
		{"source zero", func(v *GzipInputProvenance) { v.SourceBytes = 0 }},
		{"source negative", func(v *GzipInputProvenance) { v.SourceBytes = -1 }},
		{"source over limit", func(v *GzipInputProvenance) { v.SourceBytes = (64 << 30) + 1 }},
		{"source overflow", func(v *GzipInputProvenance) { v.SourceBytes = math.MaxInt64 }},
		{"decoded zero", func(v *GzipInputProvenance) { v.DecodedBytes = 0 }},
		{"decoded negative", func(v *GzipInputProvenance) { v.DecodedBytes = -1 }},
		{"decoded over limit", func(v *GzipInputProvenance) { v.DecodedBytes = (64 << 30) + 1 }},
		{"decoded overflow", func(v *GzipInputProvenance) { v.DecodedBytes = math.MaxInt64 }},
		{"ratio", func(v *GzipInputProvenance) { v.DecodedBytes = v.SourceBytes*1000 + 1 }},
		{"source sha missing", func(v *GzipInputProvenance) { v.SourceSHA256 = "" }},
		{"source sha uppercase", func(v *GzipInputProvenance) { v.SourceSHA256 = strings.Repeat("A", 64) }},
		{"decoded sha nonhex", func(v *GzipInputProvenance) { v.DecodedSHA256 = strings.Repeat("g", 64) }},
		{"decoded sha prefix", func(v *GzipInputProvenance) { v.DecodedSHA256 = "sha256:" + v.DecodedSHA256 }},
		{"source generation missing", func(v *GzipInputProvenance) { v.SourceGeneration = "" }},
		{"source generation padded", func(v *GzipInputProvenance) { v.SourceGeneration += " " }},
		{"source generation control", func(v *GzipInputProvenance) { v.SourceGeneration = "source\x00token" }},
		{"source generation large", func(v *GzipInputProvenance) { v.SourceGeneration = strings.Repeat("x", gzipInputGenerationMaxBytes+1) }},
		{"decoded generation missing", func(v *GzipInputProvenance) { v.DecodedGeneration = "" }},
		{"decoded generation whitespace", func(v *GzipInputProvenance) { v.DecodedGeneration = "decoded\u00a0token" }},
		{"decoded generation invalid UTF8", func(v *GzipInputProvenance) { v.DecodedGeneration = "decoded\xff" }},
		{"decoded generation large", func(v *GzipInputProvenance) { v.DecodedGeneration = strings.Repeat("x", gzipInputGenerationMaxBytes+1) }},
	}
	for _, format := range []string{"", "Linux_perf_data", "linux_perf_data ", "trace_text", "gzip", "zip", "SQLite", "sqlite ", "gzip_perf_data"} {
		format := format
		tests = append(tests, struct {
			name   string
			mutate func(*GzipInputProvenance)
		}{"unsupported format " + format, func(v *GzipInputProvenance) { v.DecodedFormat = format }})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validGzipInputProvenanceForTest()
			test.mutate(value)
			if err := ValidateGzipInputProvenance(value); err == nil {
				t.Fatalf("invalid tuple accepted: %+v", value)
			}
		})
	}
}

func TestGzipInputProvenanceWireShapeAndClone(t *testing.T) {
	if CloneGzipInputProvenance(nil) != nil {
		t.Fatal("nil clone is not nil")
	}
	value := validGzipInputProvenanceForTest()
	clone := CloneGzipInputProvenance(value)
	if clone == value || !reflect.DeepEqual(clone, value) {
		t.Fatal("clone did not detach an equal receipt")
	}
	clone.SourceSHA256 = strings.Repeat("c", 64)
	if clone.SourceSHA256 == value.SourceSHA256 {
		t.Fatal("clone mutation changed the original")
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	want := []string{"profile", "source_bytes", "source_sha256", "source_generation", "decoded_format", "decoded_bytes", "decoded_sha256", "decoded_generation"}
	if len(fields) != len(want) {
		t.Fatalf("transport receipt gained extra wire fields: %s", body)
	}
	for _, field := range want {
		if _, ok := fields[field]; !ok {
			t.Fatalf("transport receipt lost %q: %s", field, body)
		}
	}
}
