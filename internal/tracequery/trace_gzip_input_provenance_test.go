package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func validTraceBundleGzipInputProvenanceForTest() *tracebundle.GzipInputProvenance {
	return &tracebundle.GzipInputProvenance{
		Profile:     tracebundle.GzipInputProfileV1,
		SourceBytes: 100, SourceSHA256: strings.Repeat("a", 64), SourceGeneration: "opaque-source-generation",
		DecodedFormat: "linux_perf_data", DecodedBytes: 120,
		DecodedSHA256: strings.Repeat("b", 64), DecodedGeneration: "opaque-decoded-generation",
	}
}

func TestTraceBundleGzipInputProvenanceIsV2NonCausalMetadata(t *testing.T) {
	emptyCapture, err := tracebundle.CaptureID(nil)
	if err != nil {
		t.Fatal(err)
	}
	bundle := traceBundleFile{
		Schema: tracebundle.SchemaV2, CaptureID: emptyCapture,
		GzipInputProvenance: validTraceBundleGzipInputProvenanceForTest(),
	}
	if err := classifyTraceBundleSchema("capture.tracebundle.json", &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.schemaMode != traceBundleSchemaV2 || bundle.CaptureID != emptyCapture || len(bundle.Artifacts) != 0 || len(bundle.PerfClockAlignments) != 0 {
		t.Fatalf("transport metadata minted causal authority: %+v", bundle)
	}
	legacy := traceBundleFile{GzipInputProvenance: validTraceBundleGzipInputProvenanceForTest()}
	if err := classifyTraceBundleSchema("legacy.tracebundle.json", &legacy); err == nil || !strings.Contains(err.Error(), "mixes V2 provenance fields") {
		t.Fatalf("legacy transport metadata was accepted: %v", err)
	}
}

func TestTraceBundleGzipInputProvenanceCannotReplaceStandalonePerfTransform(t *testing.T) {
	bundle := traceBundleFile{
		Schema:              tracebundle.SchemaV2,
		GzipInputProvenance: validTraceBundleGzipInputProvenanceForTest(),
		Artifacts: []traceBundleArtifact{{
			Type: "perftrace", Path: "capture.perftrace",
			Perf: &traceBundlePerfCapability{InputFormat: "gzip_perf_data", TraceQueryReady: true},
		}},
	}
	if err := classifyTraceBundleSchema("capture.tracebundle.json", &bundle); err == nil || !strings.Contains(err.Error(), "missing perf_input_transform") {
		t.Fatalf("outer transport receipt authorized an embedded standalone gzip transform: %v", err)
	}
}

func TestTraceBundleGzipInputProvenanceSnapshotRejectsMalformedWire(t *testing.T) {
	emptyCapture, err := tracebundle.CaptureID(nil)
	if err != nil {
		t.Fatal(err)
	}
	validJSON, err := json.Marshal(validTraceBundleGzipInputProvenanceForTest())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"profile", func(v map[string]any) { v["profile"] = "gzip_perf_data_v1" }},
		{"unknown decoded format", func(v map[string]any) { v["decoded_format"] = "gzip" }},
		{"text decoded format", func(v map[string]any) { v["decoded_format"] = "trace_text" }},
		{"padded decoded format", func(v map[string]any) { v["decoded_format"] = "linux_perf_data " }},
		{"source zero", func(v map[string]any) { v["source_bytes"] = 0 }},
		{"decoded negative", func(v map[string]any) { v["decoded_bytes"] = -1 }},
		{"source over limit", func(v map[string]any) { v["source_bytes"] = int64(64<<30) + 1 }},
		{"decoded over limit", func(v map[string]any) { v["decoded_bytes"] = int64(64<<30) + 1 }},
		{"ratio", func(v map[string]any) { v["decoded_bytes"] = 100001 }},
		{"uppercase sha", func(v map[string]any) { v["source_sha256"] = strings.Repeat("A", 64) }},
		{"malformed sha", func(v map[string]any) { v["decoded_sha256"] = "bad" }},
		{"empty generation", func(v map[string]any) { v["source_generation"] = "" }},
		{"control generation", func(v map[string]any) { v["decoded_generation"] = "gen\nunsafe" }},
		{"oversize generation", func(v map[string]any) { v["decoded_generation"] = strings.Repeat("g", 1025) }},
		{"noninteger bytes", func(v map[string]any) { v["source_bytes"] = 1.5 }},
		{"string bytes", func(v map[string]any) { v["decoded_bytes"] = "120" }},
		{"number generation", func(v map[string]any) { v["source_generation"] = 1 }},
	}
	for _, field := range []string{"profile", "source_bytes", "source_sha256", "source_generation", "decoded_format", "decoded_bytes", "decoded_sha256", "decoded_generation"} {
		field := field
		tests = append(tests, struct {
			name   string
			mutate func(map[string]any)
		}{"missing " + field, func(v map[string]any) { delete(v, field) }})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var receipt map[string]any
			if err := json.Unmarshal(validJSON, &receipt); err != nil {
				t.Fatal(err)
			}
			test.mutate(receipt)
			body, err := json.Marshal(map[string]any{
				"schema": tracebundle.SchemaV2, "capture_id": emptyCapture, "gzip_input_provenance": receipt,
			})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "capture.tracebundle.json")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			snapshot, _, err := openTraceBundleSnapshot(context.Background(), path)
			if snapshot != nil {
				_ = snapshot.Close()
			}
			if err == nil || snapshot != nil {
				t.Fatalf("malformed transport wire admitted: %s err=%v", body, err)
			}
		})
	}
}

func TestTraceBundleGzipInputProvenanceDoesNotReopenInputsOrChangeChildAuthority(t *testing.T) {
	dir := t.TempDir()
	childPath := filepath.Join(dir, "capture.systrace")
	bundlePath := filepath.Join(dir, "capture.tracebundle.json")
	consumerV2WriteFile(t, childPath, []byte(consumerV2WakeupRow(20, 10)))
	writeTraceBundleV2ForTest(t, bundlePath, consumerV2SingleSystraceManifest())
	baseline, err := BuildIndex(context.Background(), bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var bundle traceBundleFile
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatal(err)
	}
	captureID := bundle.CaptureID
	// The original no longer exists, and the receipt contains no decoded path.
	// Neither input is a prerequisite for querying the published children.
	bundle.InputPath = filepath.Join(dir, "deleted-original.gz")
	bundle.GzipInputProvenance = validTraceBundleGzipInputProvenanceForTest()
	write := func() []byte {
		t.Helper()
		body, err := json.Marshal(bundle)
		if err != nil {
			t.Fatal(err)
		}
		consumerV2WriteFile(t, bundlePath, body)
		return body
	}
	body = write()
	withReceipt, err := BuildIndex(context.Background(), bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(baseline.TraceArtifacts, withReceipt.TraceArtifacts) || !reflect.DeepEqual(baseline.Events, withReceipt.Events) {
		t.Fatalf("transport metadata changed child digest, clocks, or events: before=%+v after=%+v", baseline.TraceArtifacts, withReceipt.TraceArtifacts)
	}
	version, err := CaptureTraceSourceVersion(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	child, err := os.Stat(childPath)
	if err != nil {
		t.Fatal(err)
	}
	if version.SourceBytes() != int64(len(body))+child.Size() {
		t.Fatalf("transport metadata changed physical source membership: got=%d want=%d", version.SourceBytes(), int64(len(body))+child.Size())
	}
	bundle.GzipInputProvenance.SourceSHA256 = strings.Repeat("c", 64)
	write()
	if err := version.Validate(bundlePath); err == nil {
		t.Fatal("metadata mutation did not invalidate the held manifest generation")
	}
	changed, err := CaptureTraceSourceVersion(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Fingerprint() == version.Fingerprint() {
		t.Fatal("metadata mutation retained the physical source fingerprint")
	}
	if err := classifyTraceBundleSchema(bundlePath, &bundle); err != nil || bundle.CaptureID != captureID {
		t.Fatalf("metadata mutation altered capture identity: capture=%q err=%v", bundle.CaptureID, err)
	}
}
