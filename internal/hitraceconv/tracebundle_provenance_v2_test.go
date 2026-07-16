package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func TestTraceBundleV2BindsOwnedCausalChildrenAndRelativePaths(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(input)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()

	systrace := filepath.Join(dir, "capture.systrace")
	perftrace := filepath.Join(dir, "capture.perftrace")
	systraceBody := []byte("# tracer: nop\n")
	perftraceBody := []byte("app-1 (1) [000] .... 1.000000: perf_sample: cpu=0 pid=1 tid=1 period=1 event=cycles symbol=App dso=lib.so source=test\n")
	writeOwnedSealedFixture(t, ledger, systrace, systraceBody)
	writeOwnedSealedFixture(t, ledger, perftrace, perftraceBody)

	bundleArtifact, err := writeTraceBundleWithAllCoverageAndLedger(
		context.Background(), input, systrace,
		[]Artifact{
			{Type: ArtifactSystrace, Path: systrace, Bytes: 1, Converter: "test"},
			{Type: ArtifactPerfTrace, Path: perftrace, Bytes: 1, Converter: "test", Perf: &PerfArtifactCapability{TimeDomain: "trace_seconds", TimeAlignment: "same_domain"}},
		}, nil, nil, nil, nil, nil, ledger,
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(bundleArtifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest traceBundleMetadata
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("decode tracebundle: %v\n%s", err, body)
	}
	if manifest.Schema != tracebundle.SchemaV2 || manifest.Systrace != "capture.systrace" || len(manifest.Artifacts) != 2 {
		t.Fatalf("schema/path contract mismatch: %+v", manifest)
	}
	wantBodies := map[string][]byte{"capture.systrace": systraceBody, "capture.perftrace": perftraceBody}
	members := make([]tracebundle.CaptureMember, 0, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		wantBody := wantBodies[artifact.Path]
		wantDigest := sha256.Sum256(wantBody)
		if artifact.Bytes != int64(len(wantBody)) || artifact.SHA256 != hex.EncodeToString(wantDigest[:]) {
			t.Fatalf("artifact proof mismatch: %+v", artifact)
		}
		if err := tracebundle.ValidateCapturePath(artifact.Path); err != nil {
			t.Fatalf("producer emitted noncanonical path %q: %v", artifact.Path, err)
		}
		members = append(members, tracebundle.CaptureMember{Type: artifact.Type, Path: artifact.Path, Bytes: artifact.Bytes, SHA256: artifact.SHA256})
	}
	wantCaptureID, err := tracebundle.CaptureID(members)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CaptureID != wantCaptureID {
		t.Fatalf("capture identity mismatch: got=%q want=%q", manifest.CaptureID, wantCaptureID)
	}
	if len(manifest.PerfClockAlignments) != 1 || manifest.PerfClockAlignments[0].ArtifactPath != "capture.perftrace" {
		t.Fatalf("clock alignment path was not rewritten with its artifact: %+v", manifest.PerfClockAlignments)
	}
}

func TestTraceBundleV2RejectsUnownedOrChangedCausalChild(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, *conversionFileLedger, string)
	}{
		{name: "unowned", setup: func(t *testing.T, _ *conversionFileLedger, path string) {
			if err := os.WriteFile(path, []byte("same"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "same_size_rewrite_after_seal", setup: func(t *testing.T, ledger *conversionFileLedger, path string) {
			writeOwnedSealedFixture(t, ledger, path, []byte("old!"))
			if err := os.WriteFile(path, []byte("new!"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
				t.Fatal(err)
			}
			ledger, err := newConversionFileLedger(input)
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			child := filepath.Join(dir, "capture.systrace")
			test.setup(t, ledger, child)
			_, err = writeTraceBundleWithAllCoverageAndLedger(context.Background(), input, child,
				[]Artifact{{Type: ArtifactSystrace, Path: child}}, nil, nil, nil, nil, nil, ledger)
			if err == nil {
				t.Fatal("unproven causal child was accepted")
			}
			bundlePath := filepath.Join(dir, "capture.tracebundle.json")
			if _, statErr := os.Lstat(bundlePath); !os.IsNotExist(statErr) {
				t.Fatalf("failed proof left a manifest publication: %v", statErr)
			}
		})
	}
}

func TestTraceBundleV2RejectsDuplicatePhysicalCausalChildren(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(input)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	first := filepath.Join(dir, "first.systrace")
	second := filepath.Join(dir, "second.perftrace")
	file, err := openOwnedConversionFile(first, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("same generation")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(first, second); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	firstInfo, err := os.Lstat(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.sealOwnedPath(first, firstInfo.Size()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.recordIdentity(second, info); err != nil {
		t.Fatal(err)
	}
	if err := ledger.sealOwnedPath(second, info.Size()); err != nil {
		t.Fatal(err)
	}
	_, err = writeTraceBundleWithAllCoverageAndLedger(context.Background(), input, first,
		[]Artifact{{Type: ArtifactSystrace, Path: first}, {Type: ArtifactPerfTrace, Path: second}},
		nil, nil, nil, nil, nil, ledger)
	if err == nil || !strings.Contains(err.Error(), "duplicate physical causal child") {
		t.Fatalf("physical duplicate was not rejected: %v", err)
	}
}

func TestTraceBundleV2RejectsPerftraceSuffixTypeConflicts(t *testing.T) {
	for _, test := range []struct {
		name         string
		artifactType string
		suffix       string
		wantError    string
	}{
		{name: "systrace", artifactType: ArtifactSystrace, suffix: ".perftrace", wantError: "type=systrace conflicts"},
		{name: "non_perf_mixed_case", artifactType: ArtifactTraceDB, suffix: ".PERFTRACE", wantError: "requires exact type=perftrace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
				t.Fatal(err)
			}
			ledger, err := newConversionFileLedger(input)
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			child := filepath.Join(dir, "capture"+test.suffix)
			_, err = writeTraceBundleWithAllCoverageAndLedger(context.Background(), input, child,
				[]Artifact{{Type: test.artifactType, Path: child}}, nil, nil, nil, nil, nil, ledger)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("producer accepted a type/path conflict: %v", err)
			}
			bundlePath := traceSidecarBase(input, child) + ".tracebundle.json"
			if _, statErr := os.Lstat(bundlePath); !os.IsNotExist(statErr) {
				t.Fatalf("type/path rejection left a manifest publication: %v", statErr)
			}
		})
	}
}

func TestTraceBundleV2BuilderKeepsCausalDescriptorHeld(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(input)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	child := filepath.Join(dir, "capture.systrace")
	writeOwnedSealedFixture(t, ledger, child, []byte("# tracer: nop\n"))

	_, _, heldChildren, err := buildTraceBundleV2Artifacts(context.Background(), filepath.Join(dir, "capture.tracebundle.json"),
		[]Artifact{{Type: ArtifactSystrace, Path: child}}, ledger)
	if err != nil {
		t.Fatal(err)
	}
	defer closeHeldSealedOwnedFiles(heldChildren)
	if len(heldChildren) != 1 || heldChildren[0] == nil || heldChildren[0].file == nil {
		t.Fatalf("builder released the causal descriptor before manifest publication: %+v", heldChildren)
	}
	moved := filepath.Join(dir, "moved.systrace")
	if err := os.Rename(child, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := heldChildren[0].file.Stat(); err != nil {
		t.Fatalf("held causal generation was not readable after path replacement: %v", err)
	}
	if err := heldChildren[0].Validate(context.Background()); err == nil {
		t.Fatal("held descriptor validation ignored that its public child path changed")
	}
}

func TestTraceBundleV2MetadataOnlyUsesEmptySetCaptureIdentity(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact, err := writeTraceBundleWithCoverage(input, "", []Artifact{{Type: ArtifactTraceDB, Path: filepath.Join(dir, "capture.db")}}, nil, nil, nil, []TraceDBCoverage{{Table: "stat"}})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(artifact.Path)
	body, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest traceBundleMetadata
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	want, err := tracebundle.CaptureID(nil)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != tracebundle.SchemaV2 || manifest.CaptureID != want || len(manifest.Artifacts) != 1 || manifest.Artifacts[0].Bytes != 0 {
		t.Fatalf("metadata-only v2 manifest malformed: %+v", manifest)
	}
	if !strings.Contains(string(body), `"bytes": 0`) {
		t.Fatalf("zero bytes must stay distinguishable from missing:\n%s", body)
	}
}

func writeOwnedSealedFixture(t *testing.T, ledger *conversionFileLedger, path string, body []byte) {
	t.Helper()
	file, err := openOwnedConversionFile(path, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(body); err != nil {
		t.Fatal(err)
	}
	if _, err := finishOwnedConversionFile(path, file, ledger, false); err != nil {
		t.Fatal(err)
	}
}
