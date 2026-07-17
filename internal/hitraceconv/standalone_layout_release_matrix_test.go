package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneLayoutReleaseHeaderScalarMatrix(t *testing.T) {
	base := func() []byte {
		return p1b2StandaloneBlock(
			t,
			profilerDataTypeHiperf,
			"hiperf-plugin",
			[]byte("release-matrix-payload"),
			p1b2StandalonePayloadSHA,
		)
	}
	tests := []struct {
		name        string
		mutate      func([]byte)
		wantFailure string
	}{
		{
			name: "version_minus_one",
			mutate: func(body []byte) {
				binary.LittleEndian.PutUint32(body[16:20], profilerTraceVersionV1-1)
			},
			wantFailure: "standalone_version_unsupported",
		},
		{
			name: "version_plus_one",
			mutate: func(body []byte) {
				binary.LittleEndian.PutUint32(body[16:20], profilerTraceVersionV1+1)
			},
			wantFailure: "standalone_version_unsupported",
		},
		{
			name: "segments_one",
			mutate: func(body []byte) {
				binary.LittleEndian.PutUint32(body[20:24], 1)
			},
			wantFailure: "standalone_segments_nonzero",
		},
		{
			name: "segments_max_uint32",
			mutate: func(body []byte) {
				binary.LittleEndian.PutUint32(body[20:24], math.MaxUint32)
			},
			wantFailure: "standalone_segments_nonzero",
		},
		{
			name: "boottime_slot_dirty",
			mutate: func(body []byte) {
				body[60] = 1
			},
			wantFailure: "standalone_reserved_header_noncanonical",
		},
		{
			name: "monotonic_slot_dirty",
			mutate: func(body []byte) {
				body[84] = 1
			},
			wantFailure: "standalone_reserved_header_noncanonical",
		},
		{
			name: "duration_slot_dirty",
			mutate: func(body []byte) {
				body[profilerPluginVersionOffset+profilerPluginVersionSize] = 1
			},
			wantFailure: "standalone_reserved_header_noncanonical",
		},
		{
			name: "trailing_padding_dirty",
			mutate: func(body []byte) {
				body[profilerPluginVersionOffset+profilerPluginVersionSize+8] = 1
			},
			wantFailure: "standalone_reserved_header_noncanonical",
		},
		{
			name: "length_below_header",
			mutate: func(body []byte) {
				binary.LittleEndian.PutUint64(body[8:16], profilerTraceHeaderSize-1)
			},
			wantFailure: "standalone_declared_length_invalid",
		},
		{
			name: "length_greater_than_remaining",
			mutate: func(body []byte) {
				binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)+1))
			},
			wantFailure: "standalone_declared_length_invalid",
		},
		{
			name: "length_max_uint64",
			mutate: func(body []byte) {
				binary.LittleEndian.PutUint64(body[8:16], math.MaxUint64)
			},
			wantFailure: "standalone_declared_length_invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := base()
			test.mutate(body)
			inventory := p1b2StandaloneInventory(t, body)
			if len(inventory.segments) != 0 || inventory.standaloneChainError != test.wantFailure {
				t.Fatalf(
					"noncanonical standalone scalar was not rejected atomically: segments=%+v failure=%q want=%q",
					inventory.segments,
					inventory.standaloneChainError,
					test.wantFailure,
				)
			}
		})
	}
}

func TestStandaloneLayoutReleaseCStringMatrix(t *testing.T) {
	tests := []struct {
		name   string
		offset int
		size   int
		mutate func([]byte)
	}{
		{
			name:   "plugin_name_missing_nul",
			offset: profilerPluginNameOffset,
			size:   profilerPluginNameSize,
			mutate: func(field []byte) { copy(field, bytes.Repeat([]byte{'n'}, len(field))) },
		},
		{
			name:   "plugin_name_dirty_after_nul",
			offset: profilerPluginNameOffset,
			size:   profilerPluginNameSize,
			mutate: func(field []byte) { field[len("hiperf-plugin")+1] = 'x' },
		},
		{
			name:   "plugin_name_control_byte",
			offset: profilerPluginNameOffset,
			size:   profilerPluginNameSize,
			mutate: func(field []byte) { field[0] = '\n' },
		},
		{
			name:   "plugin_name_invalid_utf8",
			offset: profilerPluginNameOffset,
			size:   profilerPluginNameSize,
			mutate: func(field []byte) { field[0] = 0xff },
		},
		{
			name:   "plugin_version_missing_nul",
			offset: profilerPluginVersionOffset,
			size:   profilerPluginVersionSize,
			mutate: func(field []byte) { copy(field, bytes.Repeat([]byte{'1'}, len(field))) },
		},
		{
			name:   "plugin_version_dirty_after_nul",
			offset: profilerPluginVersionOffset,
			size:   profilerPluginVersionSize,
			mutate: func(field []byte) { field[len("1.0")+1] = 'x' },
		},
		{
			name:   "plugin_version_control_byte",
			offset: profilerPluginVersionOffset,
			size:   profilerPluginVersionSize,
			mutate: func(field []byte) { field[0] = '\t' },
		},
		{
			name:   "plugin_version_invalid_utf8",
			offset: profilerPluginVersionOffset,
			size:   profilerPluginVersionSize,
			mutate: func(field []byte) { field[0] = 0xff },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := p1b2StandaloneBlock(
				t,
				profilerDataTypeHiperf,
				"hiperf-plugin",
				[]byte("canonical-cstring-payload"),
				p1b2StandalonePayloadSHA,
			)
			field := body[test.offset : test.offset+test.size]
			test.mutate(field)
			inventory := p1b2StandaloneInventory(t, body)
			if len(inventory.segments) != 0 || inventory.standaloneChainError != "standalone_reserved_header_noncanonical" {
				t.Fatalf(
					"noncanonical standalone CString was not rejected atomically: segments=%+v failure=%q",
					inventory.segments,
					inventory.standaloneChainError,
				)
			}
		})
	}
}

func TestStandaloneLayoutReleaseTailAndInterruptedChainMatrix(t *testing.T) {
	good := func(payload string) []byte {
		return p1b2StandaloneBlock(
			t,
			profilerDataTypeStandalone,
			"hiebpf-plugin",
			[]byte(payload),
			p1b2StandalonePayloadSHA,
		)
	}
	for tailBytes := 1; tailBytes <= 3; tailBytes++ {
		t.Run(string(rune('0'+tailBytes))+"_byte_tail", func(t *testing.T) {
			body := append(good("valid-prefix"), bytes.Repeat([]byte{0xa5}, tailBytes)...)
			inventory := p1b2StandaloneInventory(t, body)
			if len(inventory.segments) != 0 || inventory.standaloneChainError != "standalone_header_truncated" {
				t.Fatalf(
					"short tail retained a partial standalone chain: segments=%+v failure=%q",
					inventory.segments,
					inventory.standaloneChainError,
				)
			}
		})
	}

	first := good("valid-first")
	interrupted := good("truncated-middle")[:3]
	last := good("valid-last")
	body := append(append(append([]byte(nil), first...), interrupted...), last...)
	inventory := p1b2StandaloneInventory(t, body)
	if len(inventory.segments) != 0 || inventory.standaloneChainError != "standalone_magic_mismatch" {
		t.Fatalf(
			"good/truncated-header/good chain was not rejected as one physical unit: segments=%+v failure=%q",
			inventory.segments,
			inventory.standaloneChainError,
		)
	}
}

func TestStandaloneLayoutReleasePayloadPositiveMatrix(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload []byte
	}{
		{name: "empty_payload", payload: nil},
		{name: "large_streamed_payload", payload: bytes.Repeat([]byte{0x6d}, 512*1024+17)},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := p1b2StandaloneBlock(
				t,
				profilerDataTypeHiperf,
				"hiperf-plugin",
				test.payload,
				p1b2StandalonePayloadSHA,
			)
			inventory := p1b2StandaloneInventory(t, body)
			if inventory.standaloneChainError != "" || len(inventory.segments) != 1 {
				t.Fatalf("valid standalone payload was rejected: segments=%+v failure=%q", inventory.segments, inventory.standaloneChainError)
			}
			segment := inventory.segments[0]
			if segment.Offset != 0 || segment.Length != int64(len(body)) || !segment.PerfEligible ||
				segment.Integrity != standaloneIntegrityPayloadSHA256 ||
				segment.Layout != standaloneLayoutDirectOffsetZero {
				t.Fatalf("valid standalone payload proof drifted: %+v", segment)
			}
		})
	}
}

func TestStandaloneLayoutReleaseUnknownHiperfPluginIsInventoryOnly(t *testing.T) {
	body := p1b2StandaloneBlock(
		t,
		profilerDataTypeHiperf,
		"future-perf-plugin",
		[]byte("unknown-plugin-payload"),
		p1b2StandalonePayloadSHA,
	)
	inventory := p1b2StandaloneInventory(t, body)
	if inventory.standaloneChainError != "" || len(inventory.segments) != 1 {
		t.Fatalf("unknown dtype=1 plugin lost physical inventory: segments=%+v failure=%q", inventory.segments, inventory.standaloneChainError)
	}
	segment := inventory.segments[0]
	if segment.DataType != profilerDataTypeHiperf || segment.PluginName != "future-perf-plugin" || segment.PerfEligible {
		t.Fatalf("unknown dtype=1 plugin was granted HIPERF capability: %+v", segment)
	}
	if artifacts := p1b2StandaloneArtifacts(t, body); len(artifacts) != 0 {
		t.Fatalf("unknown dtype=1 plugin minted perf artifacts: %+v", artifacts)
	}
}

func TestStandaloneLayoutReleaseStatusAndGenerateFalseParity(t *testing.T) {
	for _, test := range []struct {
		name         string
		pluginName   string
		wantPerf     bool
		wantCaveats  int
		wantCaveatIn string
	}{
		{
			name:         "typed_hiperf_plugin",
			pluginName:   "hiperf-plugin",
			wantPerf:     true,
			wantCaveats:  1,
			wantCaveatIn: "primary release-matrix source",
		},
		{
			name:       "unknown_dtype_one_plugin",
			pluginName: "future-perf-plugin",
			wantPerf:   false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := p1b2StandaloneBlock(
				t,
				profilerDataTypeHiperf,
				test.pluginName,
				[]byte("status-parity-payload"),
				p1b2StandalonePayloadSHA,
			)
			inputPath := filepath.Join(t.TempDir(), "status-parity.htrace")
			if err := os.WriteFile(inputPath, body, 0o600); err != nil {
				t.Fatal(err)
			}
			statusHasPerf, err := statusInputContainsStandalonePerfSidecar(context.Background(), inputPath)
			if err != nil {
				t.Fatalf("status standalone scan: %v", err)
			}
			inventory := p1b2StandaloneInventory(t, body)
			if statusHasPerf != test.wantPerf || inventory.hasHiperfData() != test.wantPerf {
				t.Fatalf(
					"status/inventory HIPERF capability parity drifted: status=%t inventory=%t want=%t segments=%+v",
					statusHasPerf,
					inventory.hasHiperfData(),
					test.wantPerf,
					inventory.segments,
				)
			}

			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if cleanupErr := ledger.cleanup(); cleanupErr != nil {
					t.Errorf("cleanup generate-false ledger: %v", cleanupErr)
				}
			}()
			artifacts, caveats, decisions, err := extractStandaloneArtifactsWithOptionsAndLedger(
				context.Background(),
				Options{InputPath: inventory.input.DisplayPath()},
				inventory,
				filepath.Join(t.TempDir(), "status-parity.systrace"),
				standaloneExtractOptions{
					GeneratePerfTrace: false,
					PrimaryPerfSource: "primary release-matrix source",
				},
				ledger,
			)
			if err != nil {
				t.Fatalf("generate-false extraction: %v", err)
			}
			if len(artifacts) != 0 || len(decisions) != 0 || len(caveats) != test.wantCaveats {
				t.Fatalf(
					"generate-false publication parity drifted: artifacts=%+v caveats=%+v decisions=%+v",
					artifacts,
					caveats,
					decisions,
				)
			}
			if test.wantCaveatIn != "" && !strings.Contains(caveats[0], test.wantCaveatIn) {
				t.Fatalf("generate-false caveat lost primary-source disclosure: %+v", caveats)
			}
		})
	}
}
