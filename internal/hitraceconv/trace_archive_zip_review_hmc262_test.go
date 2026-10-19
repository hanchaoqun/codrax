package hitraceconv

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// The new contract admits existing leaf formats under ordinary names. These
// tests are independent of decoder-positive coverage: admission is not proof,
// and an explicit selection must not weaken archive-wide safety.
func TestHMC262ReviewZIPSelectionDoesNotGuessOrBypassSafety(t *testing.T) {
	body := traceArchiveTestBuiltinBody()
	for _, tc := range []struct {
		name     string
		members  []traceArchiveTestMember
		explicit string
		code     string
	}{
		{"legacy and ordinary candidate", []traceArchiveTestMember{{name: "old.sys", body: body}, {name: "new.data", body: body}}, "", traceArchiveCodeMultipleCandidates},
		{"two ordinary candidates", []traceArchiveTestMember{{name: "a.data", body: body}, {name: "目录/业务材料", body: body}}, "", traceArchiveCodeMultipleCandidates},
		{"explicit ordinary candidate", []traceArchiveTestMember{{name: "old.sys", body: body}, {name: "new.data", body: body}}, "new.data", ""},
		{"explicit unknown member", []traceArchiveTestMember{{name: "real.data", body: body}, {name: "notes", body: []byte("not a binary capture")}}, "notes", traceArchiveCodeExplicitMember},
		{"unselected traversal", []traceArchiveTestMember{{name: "real.data", body: body}, {name: "../notes", body: []byte("notes")}}, "real.data", traceArchiveCodeInvalidMember},
		{"unselected symlink", []traceArchiveTestMember{{name: "real.data", body: body}, {name: "link", body: []byte("real.data"), mode: os.ModeSymlink | 0o777}}, "real.data", traceArchiveCodeSpecialMember},
		{"unselected duplicate", []traceArchiveTestMember{{name: "real.data", body: body}, {name: "notes", body: []byte("one")}, {name: "notes", body: []byte("two")}}, "real.data", traceArchiveCodeDuplicateMember},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input, output := filepath.Join(dir, "capture.zip"), filepath.Join(dir, "out.systrace")
			traceArchiveTestZIP(t, input, tc.members...)
			result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, ArchiveMember: tc.explicit, TraceEngine: traceEngineBuiltin})
			if tc.code != "" {
				assertTraceArchiveCode(t, err, tc.code)
				assertNoTraceArchivePublication(t, output)
				return
			}
			if err != nil || QueryReadySystracePath(result) == "" || result.ArchiveProvenance == nil || result.ArchiveProvenance.Member != tc.explicit || result.ArchiveProvenance.Selection != "explicit_member" {
				t.Fatalf("explicit ordinary selection did not retain its exact receipt: result=%+v err=%v", result, err)
			}
		})
	}
}

type hmc262ZIPCountingReaderAt struct {
	reader io.ReaderAt
	start  int64
	end    int64
	read   int64
	onRead func()
}

func (r *hmc262ZIPCountingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := r.reader.ReadAt(p, off)
	if r.end > r.start {
		lo, hi := max(off, r.start), min(off+int64(n), r.end)
		if hi > lo {
			r.read += hi - lo
			if r.onRead != nil {
				r.onRead()
			}
		}
	}
	return n, err
}

// Empty stored DEFLATE blocks are valid and produce zero decoded bytes. They
// distinguish an actual compressed-input budget from a 64-byte output limit.
func hmc262ZIPRawDeflateMember(t *testing.T, emptyBlocks int) (*zip.Reader, *hmc262ZIPCountingReaderAt) {
	t.Helper()
	body := append([]byte("PERFILE2"), make([]byte, conversionInputProbeSize-8)...)
	compressed := bytes.Repeat([]byte{0, 0, 0, 0xff, 0xff}, emptyBlocks)
	compressed = append(compressed, 1, byte(len(body)), 0, ^byte(len(body)), 0xff)
	compressed = append(compressed, body...)
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	entry, err := w.CreateRaw(&zip.FileHeader{Name: "capture.data", Method: zip.Deflate, CRC32: crc32.ChecksumIEEE(body), CompressedSize64: uint64(len(compressed)), UncompressedSize64: uint64(len(body))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(compressed); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	counted := &hmc262ZIPCountingReaderAt{reader: bytes.NewReader(archive.Bytes())}
	z, err := zip.NewReader(counted, int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	// Validate the adversarial fixture independently before enabling counting.
	decoded, err := z.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(decoded)
	closeErr := decoded.Close()
	if err != nil || closeErr != nil || !bytes.Equal(got, body) {
		t.Fatalf("invalid DEFLATE fixture: got=%x err=%v close=%v", got, err, closeErr)
	}
	counted.start, err = z.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	counted.end = counted.start + int64(z.File[0].CompressedSize64)
	return z, counted
}

func TestHMC262ReviewZIPProbeLimitsActualCompressedConsumption(t *testing.T) {
	for _, tc := range []struct {
		name      string
		total     int64
		perMember int64
		limit     int64
	}{
		{"per member", 2048, 128, 128},
		{"total", 96, 2048, 96},
	} {
		t.Run(tc.name, func(t *testing.T) {
			z, counted := hmc262ZIPRawDeflateMember(t, 100)
			budget := &traceArchiveZIPProbeBudget{remaining: tc.total, perMember: tc.perMember}
			known, err := probeTraceArchiveZIPLeaf(context.Background(), z.File[0], budget)
			if known || err == nil {
				t.Fatalf("compressed-input limit disappeared behind output limit: known=%v err=%v", known, err)
			}
			assertTraceArchiveCode(t, err, traceArchiveCodeResourceLimit)
			if counted.read > tc.limit || counted.read <= 0 || budget.remaining != tc.total-counted.read {
				t.Fatalf("compressed-input accounting: actual=%d limit=%d remaining=%d", counted.read, tc.limit, budget.remaining)
			}
		})
	}
	t.Run("shared across members", func(t *testing.T) {
		dir := t.TempDir()
		body := append([]byte("PERFILE2"), make([]byte, conversionInputProbeSize-8)...)
		archive := traceArchiveTestZIP(t, filepath.Join(dir, "capture.zip"), traceArchiveTestMember{name: "a.data", body: body}, traceArchiveTestMember{name: "b.data", body: body})
		z, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			t.Fatal(err)
		}
		budget := &traceArchiveZIPProbeBudget{remaining: 96, perMember: 64}
		known, err := probeTraceArchiveZIPLeaf(context.Background(), z.File[0], budget)
		if err != nil || !known || budget.remaining != 32 {
			t.Fatalf("first member accounting: known=%v remaining=%d err=%v", known, budget.remaining, err)
		}
		known, err = probeTraceArchiveZIPLeaf(context.Background(), z.File[1], budget)
		if known || budget.remaining != 0 {
			t.Fatalf("member reset total budget: known=%v remaining=%d", known, budget.remaining)
		}
		assertTraceArchiveCode(t, err, traceArchiveCodeResourceLimit)
	})
	for _, during := range []bool{false, true} {
		name := "cancel before probe"
		if during {
			name = "cancel during compressed read"
		}
		t.Run(name, func(t *testing.T) {
			z, counted := hmc262ZIPRawDeflateMember(t, 100)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if during {
				counted.onRead = cancel
			} else {
				cancel()
			}
			known, err := probeTraceArchiveZIPLeaf(ctx, z.File[0], &traceArchiveZIPProbeBudget{remaining: 2048, perMember: 2048})
			if known || !errors.Is(err, context.Canceled) || (!during && counted.read != 0) {
				t.Fatalf("probe did not retain cancellation: known=%v read=%d err=%v", known, counted.read, err)
			}
		})
	}
}

func TestHMC262ReviewZIPExplicitSelectionOnlySpendsSelectedProbeBudget(t *testing.T) {
	dir := t.TempDir()
	body := append([]byte("PERFILE2"), make([]byte, conversionInputProbeSize-8)...)
	archive := traceArchiveTestZIP(t, filepath.Join(dir, "capture.zip"), traceArchiveTestMember{name: "notes", body: bytes.Repeat([]byte("x"), 64)}, traceArchiveTestMember{name: "capture.data", body: body})
	z, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	directory := traceArchiveZIPDirectory{entries: 2}
	budget := &traceArchiveZIPProbeBudget{remaining: 64, perMember: 64}
	member, selection, err := selectTraceArchiveZIPMemberWithBudget(context.Background(), z, directory, "capture.data", budget)
	if err != nil || member.name != "capture.data" || selection != "explicit_member" || budget.remaining != 0 {
		t.Fatalf("unselected classification consumed explicit budget: member=%q selection=%q remaining=%d err=%v", member.name, selection, budget.remaining, err)
	}
	_, _, err = selectTraceArchiveZIPMemberWithBudget(context.Background(), z, directory, "", &traceArchiveZIPProbeBudget{remaining: 64, perMember: 64})
	assertTraceArchiveCode(t, err, traceArchiveCodeResourceLimit)
}

func TestHMC262ReviewZIPProvenanceIsBoundToExtractedAuthority(t *testing.T) {
	for _, mutation := range []string{"member name", "member sha", "outer sha", "member size", "outer size", "selection", "other ledger", "synthetic route"} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.zip")
			archive := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.data", body: traceArchiveTestBuiltinBody()})
			authority, err := openConversionInputAuthority(input)
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			probe, err := authority.Probe()
			if err != nil {
				t.Fatal(err)
			}
			route, err := prepareTraceConversionInput(context.Background(), Options{InputPath: input, OutputPath: filepath.Join(dir, "out.systrace")}, authority, probe)
			if err != nil {
				t.Fatal(err)
			}
			defer route.Close()
			ledger, err := newConversionFileLedgerForAuthority(authority)
			if err != nil {
				t.Fatal(err)
			}
			if err := route.bindLedger(ledger); err != nil {
				t.Fatalf("valid held origin rejected: %v", err)
			}
			switch mutation {
			case "member name":
				route.provenance.Member = "other.data"
			case "member sha":
				route.provenance.MemberSHA256 = traceArchiveTestSHA([]byte("other"))
			case "outer sha":
				route.provenance.ArchiveSHA256 = traceArchiveTestSHA([]byte("other"))
			case "member size":
				route.provenance.MemberBytes++
			case "outer size":
				route.provenance.ArchiveBytes++
			case "selection":
				route.provenance.Selection = "explicit_member"
			case "other ledger":
				otherPath := filepath.Join(dir, "other.zip")
				if err := os.WriteFile(otherPath, archive, 0o600); err != nil {
					t.Fatal(err)
				}
				other, err := openConversionInputAuthority(otherPath)
				if err != nil {
					t.Fatal(err)
				}
				defer other.Close()
				ledger, err = newConversionFileLedgerForAuthority(other)
				if err != nil {
					t.Fatal(err)
				}
			case "synthetic route":
				route = &traceConversionInput{provenance: cloneTraceArchiveProvenance(route.provenance)}
			}
			if err := route.bindLedger(ledger); err == nil {
				t.Fatal("shape-valid provenance gained another source/selection's publication authority")
			}
		})
	}
}

func TestHMC262ReviewZIPUnknownOrdinaryMembersRemainOutsideClosedSet(t *testing.T) {
	for name, body := range map[string][]byte{
		"unknown proto": {0x0a, 0x04, 't', 'e', 's', 't'},
		"nested zip":    {'P', 'K', 3, 4, 0, 0, 0, 0},
		"nested gzip":   {0x1f, 0x8b, 8, 0, 0, 0, 0, 0},
		"perf text":     []byte("task 1 [000] 1.000: perf_sample: cpu=0\n"),
		"sqlite":        []byte("SQLite format 3\x00"),
	} {
		t.Run(name, func(t *testing.T) {
			for _, explicit := range []string{"", "capture.data"} {
				dir := t.TempDir()
				input, output := filepath.Join(dir, "capture.zip"), filepath.Join(dir, "out.systrace")
				traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.data", body: body})
				_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, ArchiveMember: explicit, TraceEngine: traceEngineBuiltin})
				code := traceArchiveCodeNoCandidate
				if explicit != "" {
					code = traceArchiveCodeExplicitMember
				}
				assertTraceArchiveCode(t, err, code)
				assertNoTraceArchivePublication(t, output)
			}
		})
	}
}

func TestHMC262ReviewZIPOrdinaryCandidateStillRequiresCompleteIntegrity(t *testing.T) {
	t.Run("CRC failure beyond selection prefix", func(t *testing.T) {
		dir := t.TempDir()
		input, output := filepath.Join(dir, "capture.zip"), filepath.Join(dir, "out.systrace")
		body := traceArchiveTestBuiltinBody()
		if len(body) <= conversionInputProbeSize+8 {
			t.Fatal("fixture must extend beyond the candidate probe")
		}
		archive := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.data", body: body, method: zip.Store})
		dataStart := 30 + int(binary.LittleEndian.Uint16(archive[26:28])) + int(binary.LittleEndian.Uint16(archive[28:30]))
		archive[dataStart+conversionInputProbeSize+8] ^= 0x80
		if err := os.WriteFile(input, archive, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		assertTraceArchiveCode(t, err, traceArchiveCodeMemberIntegrity)
		assertNoTraceArchivePublication(t, output)
	})
	t.Run("magic alone is not a ready receipt", func(t *testing.T) {
		dir := t.TempDir()
		input, output := filepath.Join(dir, "capture.zip"), filepath.Join(dir, "out.systrace")
		traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.data", body: []byte("PERFILE2")})
		result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin, PerfParser: "raw"})
		if QueryReadyPerfTracePath(result.Artifacts) != "" || QueryReadySystracePath(result) != "" {
			t.Fatalf("candidate magic incorrectly became a prepared capability: %+v, %v", result, err)
		}
		// Existing perf decoders may return a truthful inventory-only bundle
		// instead of a transaction error. That does not make the leaf queryable.
		if err != nil {
			assertNoTraceArchivePublication(t, output)
			return
		}
		if result.OutputPath != "" || len(result.ProviderDecisions) == 0 {
			t.Fatalf("inventory-only result hid unavailable decoding: %+v", result)
		}
		for _, decision := range result.ProviderDecisions {
			if decision.Succeeded || decision.TraceQueryReady {
				t.Fatalf("truncated header claimed successful decoding: %+v", decision)
			}
		}
		for _, artifact := range result.Artifacts {
			if artifact.Type != ArtifactTraceBundle {
				t.Fatalf("truncated header published a non-inventory artifact: %+v", artifact)
			}
		}
	})
}

func TestHMC262ReviewZIPOrdinaryMemberKeepsHeldSourceGeneration(t *testing.T) {
	for _, replaceMember := range []bool{false, true} {
		name := "outer archive"
		if replaceMember {
			name = "sealed member"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.zip")
			original := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "目录/业务材料", body: traceArchiveTestBuiltinBody()})
			authority, err := openConversionInputAuthority(input)
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			probe, err := authority.Probe()
			if err != nil {
				t.Fatal(err)
			}
			route, err := prepareTraceConversionInput(context.Background(), Options{InputPath: input, OutputPath: filepath.Join(dir, "out.systrace")}, authority, probe)
			if err != nil {
				t.Fatal(err)
			}
			defer route.Close()
			replacePath, replacement := input, original
			if replaceMember {
				replacePath, err = route.staging.ChildPath(traceArchiveMemberSnapshotLeaf)
				if err != nil {
					t.Fatal(err)
				}
				replacement = traceArchiveTestBuiltinBody()
			}
			if err := os.Rename(replacePath, replacePath+".original"); err != nil {
				t.Fatal(err)
			}
			// Even identical bytes at a new generation cannot inherit the held
			// original's publication authority.
			if err := os.WriteFile(replacePath, bytes.Clone(replacement), 0o600); err != nil {
				t.Fatal(err)
			}
			err = route.input.Validate(conversionInputStageBuiltinMetadata)
			var typed *ConversionInputError
			if !errors.As(err, &typed) || typed.Code != ConversionInputCodeGenerationChanged {
				t.Fatalf("replacement reused the old member authority: %T %v", err, err)
			}
		})
	}
}
