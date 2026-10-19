package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func validTraceBundleArchiveProvenanceForTest() *traceBundleArchiveProvenance {
	return &traceBundleArchiveProvenance{
		Format: "zip", ArchiveBytes: 100, ArchiveSHA256: strings.Repeat("a", 64),
		Member: "nested/capture.sys", MemberBytes: 80, MemberSHA256: strings.Repeat("b", 64),
		Selection: "unique_candidate",
	}
}

func TestHMC262ArchiveOriginNamesDoNotCreateQueryCapability(t *testing.T) {
	emptyCapture, err := tracebundle.CaptureID(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range []string{"nested/capture.sys", "nested/capture.data", "采集/业务材料", "metadata.txt"} {
		t.Run(member, func(t *testing.T) {
			origin := validTraceBundleArchiveProvenanceForTest()
			origin.Member = member
			bundle := traceBundleFile{Schema: tracebundle.SchemaV2, CaptureID: emptyCapture, ArchiveProvenance: origin}
			if err := classifyTraceBundleSchema("capture.tracebundle.json", &bundle); err != nil {
				t.Fatalf("canonical origin name rejected: %v", err)
			}
			body, err := json.Marshal(bundle)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(t.TempDir(), "capture.tracebundle.json")
			if err := os.WriteFile(name, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if index, err := BuildIndex(context.Background(), name); err == nil || index != nil {
				t.Fatalf("bare origin JSON gained query capability: index=%+v err=%v", index, err)
			}
		})
	}
}

func TestTraceBundleArchiveProvenanceIsStrictV2NonCausalMetadata(t *testing.T) {
	emptyCapture, err := tracebundle.CaptureID(nil)
	if err != nil {
		t.Fatal(err)
	}
	bundle := traceBundleFile{
		Schema: tracebundle.SchemaV2, CaptureID: emptyCapture,
		ArchiveProvenance: validTraceBundleArchiveProvenanceForTest(),
	}
	if err := classifyTraceBundleSchema("capture.tracebundle.json", &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.schemaMode != traceBundleSchemaV2 || bundle.CaptureID != emptyCapture {
		t.Fatalf("archive metadata changed causal identity: %+v", bundle)
	}

	legacy := traceBundleFile{ArchiveProvenance: validTraceBundleArchiveProvenanceForTest()}
	if err := classifyTraceBundleSchema("legacy.tracebundle.json", &legacy); err == nil || !strings.Contains(err.Error(), "mixes V2 provenance fields") {
		t.Fatalf("legacy archive provenance was accepted: %v", err)
	}
}

func TestTraceBundleArchiveProvenanceRejectsEveryClosedTupleDrift(t *testing.T) {
	emptyCapture, err := tracebundle.CaptureID(nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*traceBundleArchiveProvenance)
	}{
		{name: "format", mutate: func(value *traceBundleArchiveProvenance) { value.Format = "ZIP" }},
		{name: "archive bytes", mutate: func(value *traceBundleArchiveProvenance) { value.ArchiveBytes = 0 }},
		{name: "member bytes", mutate: func(value *traceBundleArchiveProvenance) { value.MemberBytes = -1 }},
		{name: "archive sha", mutate: func(value *traceBundleArchiveProvenance) { value.ArchiveSHA256 = strings.Repeat("A", 64) }},
		{name: "member sha", mutate: func(value *traceBundleArchiveProvenance) { value.MemberSHA256 = "bad" }},
		{name: "member parent", mutate: func(value *traceBundleArchiveProvenance) { value.Member = "../capture.sys" }},
		{name: "member absolute", mutate: func(value *traceBundleArchiveProvenance) { value.Member = "/capture.data" }},
		{name: "selection", mutate: func(value *traceBundleArchiveProvenance) { value.Selection = "first" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provenance := validTraceBundleArchiveProvenanceForTest()
			test.mutate(provenance)
			bundle := traceBundleFile{Schema: tracebundle.SchemaV2, CaptureID: emptyCapture, ArchiveProvenance: provenance}
			if err := classifyTraceBundleSchema("capture.tracebundle.json", &bundle); err == nil || !strings.Contains(err.Error(), "archive_provenance") {
				t.Fatalf("invalid archive tuple accepted: %+v err=%v", provenance, err)
			}
		})
	}
}
