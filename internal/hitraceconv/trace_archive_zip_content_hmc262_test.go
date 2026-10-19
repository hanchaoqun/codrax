package hitraceconv

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHMC262ZIPKnownLeafNamesReachExistingDecoders(t *testing.T) {
	for _, family := range []struct {
		name string
		body []byte
		perf bool
	}{
		{"perf", syntheticRawPerfData(), true},
		{"simpleperf", syntheticSimpleperfProtoStream(false, false), true},
		{"rmq", traceArchiveTestBuiltinBody(), false},
		{"profiler", syntheticProfilerTraceRoot(), false},
	} {
		for _, member := range []string{"capture.sys", "采集/capture.data", "采集/业务材料"} {
			t.Run(family.name+"/"+member, func(t *testing.T) {
				dir := t.TempDir()
				input := filepath.Join(dir, "capture.zip")
				before := traceArchiveTestZIP(t, input,
					traceArchiveTestMember{name: member, body: family.body, method: zip.Deflate},
					traceArchiveTestMember{name: "README.txt", body: []byte("a capture, not a format authority")},
				)
				opts := Options{
					InputPath: input, OutputPath: filepath.Join(dir, "result.systrace"),
					TraceEngine: traceEngineBuiltin,
				}
				if family.name == "perf" {
					opts.PerfParser = "raw"
				}
				result, err := ConvertFile(context.Background(), opts)
				if err != nil {
					t.Fatalf("normal member name rejected before its existing decoder: %v", err)
				}
				provenance := result.ArchiveProvenance
				if provenance == nil || provenance.Member != member || provenance.Selection != "unique_candidate" ||
					provenance.MemberSHA256 != traceArchiveTestSHA(family.body) || provenance.ArchiveSHA256 != traceArchiveTestSHA(before) {
					t.Fatalf("member/original provenance mismatch: %+v", provenance)
				}
				if family.perf {
					if QueryReadyPerfTracePath(result.Artifacts) == "" || QueryReadySystracePath(result) != "" {
						t.Fatalf("sample-only member changed capability: %+v", result)
					}
				} else if QueryReadySystracePath(result) == "" {
					t.Fatalf("trace member lost ready receipt: %+v", result)
				}
				after, err := os.ReadFile(input)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("source changed: %v", err)
				}
			})
		}
	}
}
