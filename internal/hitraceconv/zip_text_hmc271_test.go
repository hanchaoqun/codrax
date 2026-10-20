package hitraceconv

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func hmc271ZIPTextBody() []byte {
	return []byte("# tracer: nop\n worker-42 (42) [000] .... 1.000000: tracing_mark_write: B|42|early\n" +
		strings.Repeat("# complete member, not a prefix\n", 3000) +
		" worker-42 (42) [000] .... 2.000000: tracing_mark_write: B|42|tail-work\n" +
		" worker-42 (42) [000] .... 2.040000: tracing_mark_write: E|42\n")
}

func TestHMC271ZIPTextPublicCandidateAndCompletePreparation(t *testing.T) {
	for _, method := range []uint16{zip.Store, zip.Deflate} {
		for _, member := range []string{"session/capture.data", "资料/业务材料", "capture.sys"} {
			t.Run(member+"/"+map[uint16]string{zip.Store: "store", zip.Deflate: "deflate"}[method], func(t *testing.T) {
				dir := t.TempDir()
				input, output := filepath.Join(dir, "资料.data"), filepath.Join(dir, "complete.systrace")
				body := hmc271ZIPTextBody()
				wire := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: member, body: body, method: method})
				probe := ProbeInputRoutingCapability(t.Context(), input, filepath.Join(dir, "runtime"))
				if probe.NativeReader != "trace_query" || probe.ArchiveCandidates == nil || probe.ArchiveCandidates.CandidateCount != 1 ||
					len(probe.ArchiveCandidates.Members) != 1 || probe.ArchiveCandidates.Members[0] != member {
					t.Fatalf("ordinary named text member lost candidate navigation: %+v", probe)
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatal("candidate produced a ready output", err)
				}
				result, err := PrepareFile(t.Context(), Options{InputPath: input, OutputPath: output, RuntimeAnchor: filepath.Join(dir, "runtime")})
				if err != nil || result.OutputBytes != int64(len(body)) || result.ArchiveProvenance == nil ||
					result.ArchiveProvenance.Member != member || result.ArchiveProvenance.ArchiveSHA256 != traceArchiveTestSHA(wire) ||
					result.ArchiveProvenance.MemberSHA256 != traceArchiveTestSHA(body) {
					t.Fatalf("complete text transport failed: %+v %v", result, err)
				}
				if result.EventsWritten != 0 || len(result.Artifacts) != 0 || result.BundlePath != "" || QueryReadySystracePath(result) != "" ||
					len(result.TraceCoverage) != 0 || len(result.TraceDecisions) != 0 {
					t.Fatal("byte transport acquired semantic conversion authority")
				}
				transport := result.ArchiveTextTransport
				if transport == nil || transport.Profile != ArchiveTextTransportProfile || transport.Archive != *result.ArchiveProvenance ||
					transport.SourcePath != input || transport.SourceGeneration == "" || transport.DecodedGeneration == "" ||
					transport.DecodedBytes != int64(len(body)) || transport.DecodedSHA256 != traceArchiveTestSHA(body) ||
					result.TextTransport != nil || result.GzipInputProvenance != nil {
					t.Fatalf("independent transport receipt missing or mixed: %+v", transport)
				}
				decoded, err := os.ReadFile(output)
				if err != nil || !bytes.Equal(decoded, body) {
					t.Fatal("complete member changed", err)
				}
				idx, err := tracequery.BuildIndex(t.Context(), output)
				if err != nil || len(idx.Events) != 3 || idx.Events[1].Ts != 2 || idx.Events[1].Line < 3000 {
					t.Fatalf("native query lost the tail: %+v %v", idx, err)
				}
				current, err := os.ReadFile(input)
				if err != nil || !bytes.Equal(current, wire) {
					t.Fatal("original changed", err)
				}
			})
		}
	}
}

func TestHMC271ZIPTextNavigationIsBoundedPhysicalOriginOnly(t *testing.T) {
	header := " worker-42 (42) [000] .... 1.000000: future_device_event: "
	for _, tc := range []struct {
		name, body string
		candidate  bool
	}{
		{"crlf", "\r\n # comments\r\n" + header + "value\r\n", true},
		{"long_payload", header + strings.Repeat("x", traceRoutingTextProbeBytes*4), true},
		{"header_only", "# tracer: nop\n", false},
		{"prose", "here is an example\n" + header, false},
		{"binary_prefix", "\x00\xff\n" + header, false},
		{"past_budget", "#" + strings.Repeat("x", traceRoutingTextProbeBytes) + "\n" + header, false},
		{"partial_header", "#" + strings.Repeat("x", traceRoutingTextProbeBytes-15) + "\n" + header, false},
		{"sqlite", "SQLite format 3\x00" + header, false},
		{"nested_gzip", "\x1f\x8b\x08\x00" + header, false},
		{"nested_zip", "PK\x03\x04" + header, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "资料.bin")
			wire := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.data", body: []byte(tc.body)})
			out := ProbeInputRoutingCapability(t.Context(), input, dir)
			if (out.NativeReader == "trace_query") != tc.candidate || out.ArchiveCandidates == nil {
				t.Fatalf("physical candidate mismatch: %+v", out)
			}
			got, err := os.ReadFile(input)
			if err != nil || !bytes.Equal(got, wire) {
				t.Fatal("navigation changed source", err)
			}
		})
	}
}

func TestHMC271ZIPTextFullIntegrityAfterCandidate(t *testing.T) {
	for _, mutation := range []string{"crc", "declared_size", "late_nul", "late_utf8", "long_physical_line"} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			input, output := filepath.Join(dir, "capture.zip"), filepath.Join(dir, "out.systrace")
			body := hmc271ZIPTextBody()
			switch mutation {
			case "late_nul":
				body = append(body, 0)
			case "late_utf8":
				body = append(body, 0xff)
			case "long_physical_line":
				body = append(body, []byte(strings.Repeat("x", attachment.TracePhysicalLineMaxBytes+1))...)
			}
			wire := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "session/capture.data", body: body})
			central := bytes.Index(wire, []byte("PK\x01\x02"))
			switch mutation {
			case "crc":
				wire[central+16] ^= 0xff
			case "declared_size":
				binary.LittleEndian.PutUint32(wire[central+24:central+28], uint32(len(body)-1))
			}
			write := os.WriteFile(input, wire, 0600)
			if write != nil {
				t.Fatal(write)
			}
			probe := ProbeInputRoutingCapability(t.Context(), input, dir)
			if probe.NativeReader != "trace_query" {
				t.Fatalf("prefix unexpectedly consumed full source: %+v", probe)
			}
			result, err := PrepareFile(t.Context(), Options{InputPath: input, OutputPath: output, RuntimeAnchor: filepath.Join(dir, "runtime")})
			if err == nil || result.ArchiveTextTransport != nil || result.OutputPath != "" {
				t.Fatalf("bad full member became ready: %+v %v", result, err)
			}
			assertNoTraceArchivePublication(t, output)
			got, err := os.ReadFile(input)
			if err != nil || !bytes.Equal(got, wire) {
				t.Fatal("failure changed original", err)
			}
		})
	}
}

func TestHMC271ZIPTextPublicationKeepsSourceAndOptionBoundaries(t *testing.T) {
	for _, mutation := range []string{"existing_output", "source_output", "explicit_db", "explicit_streamer", "cancelled", "source_replaced"} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			input, output := filepath.Join(dir, "capture.zip"), filepath.Join(dir, "out.systrace")
			wire := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "session/capture.data", body: hmc271ZIPTextBody()})
			opts := Options{InputPath: input, OutputPath: output, RuntimeAnchor: filepath.Join(dir, "runtime")}
			ctx := &hmc264RoutingContext{Context: t.Context()}
			switch mutation {
			case "existing_output":
				if err := os.WriteFile(output, []byte("prior output"), 0600); err != nil {
					t.Fatal(err)
				}
			case "source_output":
				opts.OutputPath = input
			case "explicit_db":
				opts.TraceDBOutputPath = filepath.Join(dir, "retained.db")
			case "explicit_streamer":
				opts.TraceEngine = "trace_streamer"
			case "cancelled":
				ctx.cancelled = true
			case "source_replaced":
				ctx.at = 12
				ctx.action = func() {
					if err := os.WriteFile(input+".new", wire, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(input+".new", input); err != nil {
						t.Fatal(err)
					}
				}
			}
			result, err := PrepareFile(ctx, opts)
			if err == nil || result.ArchiveTextTransport != nil || result.OutputPath != "" {
				t.Fatalf("publication boundary bypassed: %+v %v", result, err)
			}
			if mutation == "source_replaced" && ctx.checks < ctx.at {
				t.Fatal("source replacement control did not execute")
			}
			if mutation == "existing_output" {
				got, err := os.ReadFile(output)
				if err != nil || string(got) != "prior output" {
					t.Fatal("prior output changed", err)
				}
			} else if mutation != "source_output" {
				assertNoTraceArchivePublication(t, output)
			}
			got, err := os.ReadFile(input)
			if err != nil || !bytes.Equal(got, wire) {
				t.Fatal("original bytes changed", err)
			}
		})
	}
}

func TestHMC271ZIPTextExplicitSelectionDoesNotGuess(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "captures.zip")
	body := hmc271ZIPTextBody()
	traceArchiveTestZIP(t, input,
		traceArchiveTestMember{name: "A.data", body: body},
		traceArchiveTestMember{name: "B.data", body: body})
	opts := Options{InputPath: input, OutputPath: filepath.Join(dir, "result.systrace"), RuntimeAnchor: filepath.Join(dir, "runtime")}
	_, err := PrepareFile(context.Background(), opts)
	assertTraceArchiveCode(t, err, traceArchiveCodeMultipleCandidates)
	assertNoTraceArchivePublication(t, opts.OutputPath)
	opts.ArchiveMember = "B.data"
	result, err := PrepareFile(t.Context(), opts)
	if err != nil || result.ArchiveProvenance == nil || result.ArchiveProvenance.Member != "B.data" || result.ArchiveProvenance.Selection != "explicit_member" {
		t.Fatalf("exact text member selection failed: %+v %v", result, err)
	}
}
