package hitraceconv

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func TestHMC264ZIPRoutingKnownFamiliesAndLegacyAreOnlyCandidates(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"perf.data", syntheticRawPerfData()},
		{"simpleperf.data", syntheticSimpleperfProtoStream(false, false)},
		{"rmq.data", traceArchiveTestBuiltinBody()},
		{"profiler.data", syntheticProfilerTraceRoot()},
		{"legacy.sys", []byte("# tracer: nop\n worker-42 ( 42) [000] .... 1.000000: tracing_mark_write: B|42|work\n")},
		{"unverified.sys", []byte("not a trace")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "资料.zip")
			traceArchiveTestZIP(t, input, traceArchiveTestMember{name: tc.name, body: tc.body, method: zip.Deflate})
			got := ProbeInputRoutingCapability(context.Background(), input, t.TempDir())
			if got.NativeReader != "trace_query" || got.Status != "archive_candidates" || got.ArchiveCandidates == nil || got.ArchiveCandidates.CandidateCount != 1 {
				t.Fatalf("existing family lost navigation: %+v", got)
			}
			if tc.name == "unverified.sys" {
				out := filepath.Join(dir, "rejected.systrace")
				result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: out, TraceEngine: traceEngineBuiltin})
				if err == nil || QueryReadySystracePath(result) != "" {
					t.Fatalf("candidate granted readiness: %+v, %v", result, err)
				}
				assertNoTraceArchivePublication(t, out)
			}
		})
	}
}

func TestHMC264ZIPRoutingRejectsUnsafeAndUnknownInventory(t *testing.T) {
	known := traceArchiveTestMember{name: "capture.data", body: traceArchiveTestBuiltinBody()}
	for _, tc := range []struct {
		name    string
		members []traceArchiveTestMember
		reason  string
	}{
		{"unknown", []traceArchiveTestMember{{name: "readme.data", body: []byte("not a trace")}}, "container_only_domain_unknown"},
		{"sqlite", []traceArchiveTestMember{{name: "database.data", body: append([]byte("SQLite format 3\x00"), make([]byte, 64)...)}}, "container_only_domain_unknown"},
		{"nested", []traceArchiveTestMember{{name: "nested.data", body: []byte("PK\x03\x04")}}, "container_only_domain_unknown"},
		{"gzip", []traceArchiveTestMember{{name: "nested.data", body: []byte{0x1f, 0x8b, 8, 0}}}, "container_only_domain_unknown"},
		{"traversal_after_known", []traceArchiveTestMember{known, {name: "../escape", body: []byte("x")}}, traceArchiveCodeInvalidMember},
		{"symlink_after_known", []traceArchiveTestMember{known, {name: "link", body: []byte("capture.data"), mode: os.ModeSymlink | 0777}}, traceArchiveCodeSpecialMember},
		{"duplicate", []traceArchiveTestMember{known, known}, traceArchiveCodeDuplicateMember},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := filepath.Join(t.TempDir(), "资料.zip")
			traceArchiveTestZIP(t, input, tc.members...)
			got := ProbeInputRoutingCapability(context.Background(), input, t.TempDir())
			if got.NativeReader != "" || got.Reason != tc.reason {
				t.Fatalf("unsafe/unknown archive gained navigation: %+v", got)
			}
			if tc.reason == "container_only_domain_unknown" {
				if got.ArchiveCandidates == nil || got.ArchiveCandidates.CandidateCount != 0 || !got.ArchiveCandidates.MembersComplete {
					t.Fatalf("unknown inventory misrepresented: %+v", got)
				}
			} else if got.ArchiveCandidates != nil {
				t.Fatalf("failed scan leaked partial candidates: %+v", got)
			}
		})
	}
}

func hmc264RawProbeZIP(t *testing.T, input string, members, emptyBlocks int) {
	t.Helper()
	body := append([]byte("PERFILE2"), make([]byte, conversionInputProbeSize-8)...)
	compressed := bytes.Repeat([]byte{0, 0, 0, 0xff, 0xff}, emptyBlocks)
	compressed = append(compressed, 1, byte(len(body)), 0, ^byte(len(body)), 0xff)
	compressed = append(compressed, body...)
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for i := 0; i < members; i++ {
		entry, err := writer.CreateRaw(&zip.FileHeader{Name: fmt.Sprintf("capture-%d.data", i), Method: zip.Deflate, CRC32: crc32.ChecksumIEEE(body), CompressedSize64: uint64(len(compressed)), UncompressedSize64: uint64(len(body))})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(compressed); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestHMC264ZIPRoutingCompressedBudgetsDiscardPartialInventory(t *testing.T) {
	for _, tc := range []struct {
		name            string
		members, blocks int
		rejected        bool
	}{
		{"bounded", 2, 100, false},
		{"per_member", 1, int(traceArchiveZIPProbeMemberBytes/5) + 1, true},
		{"shared_total", 10, 180000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := filepath.Join(t.TempDir(), "capture.zip")
			hmc264RawProbeZIP(t, input, tc.members, tc.blocks)
			got := ProbeInputRoutingCapability(context.Background(), input, t.TempDir())
			if tc.rejected {
				if got.NativeReader != "" || got.ArchiveCandidates != nil || got.Reason != traceArchiveCodeResourceLimit {
					t.Fatalf("budget bypass/partial capability: %+v", got)
				}
			} else if got.ArchiveCandidates == nil || got.ArchiveCandidates.CandidateCount != tc.members {
				t.Fatalf("bounded probe lost candidates: %+v", got)
			}
		})
	}
}

func TestHMC264ZIPRoutingDoesNotPretendFullCRCValidation(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.zip")
	body := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.data", body: traceArchiveTestBuiltinBody()})
	dataStart := 30 + int(binary.LittleEndian.Uint16(body[26:28])) + int(binary.LittleEndian.Uint16(body[28:30]))
	body[dataStart+conversionInputProbeSize+8] ^= 0x80
	if err := os.WriteFile(input, body, 0600); err != nil {
		t.Fatal(err)
	}
	got := ProbeInputRoutingCapability(context.Background(), input, t.TempDir())
	if got.Status != "archive_candidates" || got.NativeReader != "trace_query" {
		t.Fatalf("prefix navigation unexpectedly read full payload: %+v", got)
	}
	output := filepath.Join(dir, "out.systrace")
	_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	assertTraceArchiveCode(t, err, traceArchiveCodeMemberIntegrity)
	assertNoTraceArchivePublication(t, output)
}

func TestHMC264ZIPRoutingRejectsDirectoryAndTransportLimits(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		mutate     func([]byte)
	}{
		{"encrypted", traceArchiveCodeEncryptedMember, func(body []byte) {
			central := bytes.Index(body, []byte("PK\x01\x02"))
			binary.LittleEndian.PutUint16(body[6:8], binary.LittleEndian.Uint16(body[6:8])|1)
			binary.LittleEndian.PutUint16(body[central+8:central+10], binary.LittleEndian.Uint16(body[central+8:central+10])|1)
		}},
		{"unsupported_compression", traceArchiveCodeMemberIntegrity, func(body []byte) {
			central := bytes.Index(body, []byte("PK\x01\x02"))
			binary.LittleEndian.PutUint16(body[8:10], 99)
			binary.LittleEndian.PutUint16(body[central+10:central+12], 99)
		}},
		{"too_many_entries", traceArchiveCodeResourceLimit, func(body []byte) {
			end := bytes.LastIndex(body, []byte("PK\x05\x06"))
			binary.LittleEndian.PutUint16(body[end+8:end+10], traceArchiveZIPMaxEntries+1)
			binary.LittleEndian.PutUint16(body[end+10:end+12], traceArchiveZIPMaxEntries+1)
		}},
		{"too_large_directory", traceArchiveCodeResourceLimit, func(body []byte) {
			end := bytes.LastIndex(body, []byte("PK\x05\x06"))
			binary.LittleEndian.PutUint32(body[end+12:end+16], traceArchiveZIPMaxCentralDirectory+1)
		}},
		{"spanned", traceArchiveCodeMultiDisk, func(body []byte) { copy(body[:4], "PK\x07\x08") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := filepath.Join(t.TempDir(), "资料.zip")
			body := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.data", body: traceArchiveTestBuiltinBody()})
			tc.mutate(body)
			if err := os.WriteFile(input, body, 0600); err != nil {
				t.Fatal(err)
			}
			got := ProbeInputRoutingCapability(context.Background(), input, t.TempDir())
			if got.NativeReader != "" || got.ArchiveCandidates != nil || got.Reason != tc.code {
				t.Fatalf("directory/transport failure leaked navigation: %+v", got)
			}
		})
	}
}
