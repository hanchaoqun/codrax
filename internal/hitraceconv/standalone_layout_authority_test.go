package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type p1b2StandaloneSHAProfile uint8

type p1b2StandaloneByteRange struct {
	start int64
	end   int64
}

type p1b2PayloadPoisonReaderAt struct {
	body         []byte
	payloads     []p1b2StandaloneByteRange
	payloadReads int
}

func (reader *p1b2PayloadPoisonReaderAt) ReadAt(dst []byte, off int64) (int, error) {
	end := off + int64(len(dst))
	for _, payload := range reader.payloads {
		if off < payload.end && end > payload.start {
			reader.payloadReads++
			return 0, fmt.Errorf("poison standalone payload read at [%d,%d)", off, end)
		}
	}
	if off < 0 || off >= int64(len(reader.body)) {
		return 0, io.EOF
	}
	n := copy(dst, reader.body[off:])
	if n != len(dst) {
		return n, io.EOF
	}
	return n, nil
}

const (
	p1b2StandalonePayloadSHA p1b2StandaloneSHAProfile = iota + 1
	p1b2StandaloneOfficialZeroSHA
	p1b2StandaloneWrongSHA
)

func p1b2StandaloneBlock(
	t testing.TB,
	dataType uint32,
	pluginName string,
	payload []byte,
	profile p1b2StandaloneSHAProfile,
) []byte {
	t.Helper()
	body := syntheticStandaloneProfilerBlock(dataType, pluginName, "1.0", payload)
	switch profile {
	case p1b2StandalonePayloadSHA:
		digest := sha256.Sum256(payload)
		copy(body[24:56], digest[:])
	case p1b2StandaloneOfficialZeroSHA:
		// TraceFileWriter::WriteStandalonePluginFile leaves this field at the
		// producer-default all-zero value. This is a distinct official lane,
		// not SHA256(empty).
		clear(body[24:56])
	case p1b2StandaloneWrongSHA:
		digest := sha256.Sum256(append(append([]byte(nil), payload...), 0xff))
		copy(body[24:56], digest[:])
	default:
		t.Fatalf("unknown standalone SHA profile %d", profile)
	}
	return body
}

func p1b2StandaloneInventory(t testing.TB, body []byte) standaloneSegmentInventory {
	t.Helper()
	input := newScriptedStandaloneInputView(
		filepath.Join(t.TempDir(), "standalone-layout.htrace"), body,
	)
	inventory, err := findStandaloneSegmentsFromInput(context.Background(), input)
	if err != nil {
		t.Fatalf("build standalone physical-layout inventory: %v", err)
	}
	return inventory
}

func p1b2StandaloneArtifacts(t testing.TB, body []byte) []Artifact {
	t.Helper()
	inventory := p1b2StandaloneInventory(t, body)
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanupErr := ledger.cleanup(); cleanupErr != nil {
			t.Errorf("cleanup standalone artifact ledger: %v", cleanupErr)
		}
	}()
	artifacts, _, _, err := extractStandaloneArtifactsWithOptionsAndLedger(
		context.Background(),
		Options{
			InputPath:          inventory.input.DisplayPath(),
			DisablePerfAdapter: true,
			PerfParser:         "raw",
		},
		inventory,
		filepath.Join(t.TempDir(), "standalone-layout.systrace"),
		standaloneExtractOptions{GeneratePerfTrace: true},
		ledger,
	)
	if err != nil {
		t.Fatalf("extract standalone artifacts: %v", err)
	}
	return artifacts
}

func p1b2AuthenticatedRoot(t testing.TB) []byte {
	t.Helper()
	return syntheticProfilerTraceFile(syntheticProfilerPluginData(
		"bytrace_plugin",
		[]byte("root-7 (7) [001] .... 1.000000: print: B|7|Root"),
	))
}

func TestStandaloneLayoutArbitraryPrefixCannotMintArtifact(t *testing.T) {
	block := p1b2StandaloneBlock(
		t, profilerDataTypeHiperf, "hiperf-plugin", syntheticRawPerfData(), p1b2StandalonePayloadSHA,
	)
	body := append([]byte("unframed-arbitrary-prefix"), block...)
	if inventory := p1b2StandaloneInventory(t, body); len(inventory.segments) != 0 {
		t.Fatalf("arbitrary-prefix magic minted standalone inventory: %+v", inventory.segments)
	}
	if artifacts := p1b2StandaloneArtifacts(t, body); len(artifacts) != 0 {
		t.Fatalf("arbitrary-prefix magic minted standalone artifacts: %+v", artifacts)
	}
}

func TestStandaloneLayoutPayloadMagicCannotMintNestedArtifact(t *testing.T) {
	inner := p1b2StandaloneBlock(
		t, profilerDataTypeHiperf, "hiperf-plugin", syntheticRawPerfData(), p1b2StandalonePayloadSHA,
	)
	outer := p1b2StandaloneBlock(
		t, profilerDataTypeStandalone, "hiebpf-plugin", inner, p1b2StandalonePayloadSHA,
	)
	inventory := p1b2StandaloneInventory(t, outer)
	if len(inventory.segments) != 1 || inventory.segments[0].Offset != 0 ||
		inventory.segments[0].Length != int64(len(outer)) ||
		inventory.segments[0].DataType != profilerDataTypeStandalone {
		t.Fatalf("payload magic escaped its outer standalone block: %+v", inventory.segments)
	}
	if artifacts := p1b2StandaloneArtifacts(t, outer); len(artifacts) != 0 {
		t.Fatalf("payload magic minted a nested HIPERF artifact: %+v", artifacts)
	}
}

func TestStandaloneLayoutOffsetZeroDirectRequiresExactPayloadSHA(t *testing.T) {
	payload := []byte("direct-standalone-payload")
	exact := p1b2StandaloneBlock(
		t, profilerDataTypeHiperf, "hiperf-plugin", payload, p1b2StandalonePayloadSHA,
	)
	inventory := p1b2StandaloneInventory(t, exact)
	if len(inventory.segments) != 1 || inventory.segments[0].Offset != 0 ||
		inventory.segments[0].Length != int64(len(exact)) ||
		inventory.segments[0].DataType != profilerDataTypeHiperf {
		t.Fatalf("exact direct standalone profile rejected: %+v", inventory.segments)
	}
	zero := p1b2StandaloneBlock(
		t, profilerDataTypeHiperf, "hiperf-plugin", payload, p1b2StandaloneOfficialZeroSHA,
	)
	if zeroInventory := p1b2StandaloneInventory(t, zero); len(zeroInventory.segments) != 0 {
		t.Fatalf("unanchored direct zero-SHA profile minted inventory: %+v", zeroInventory.segments)
	}
}

func TestStandaloneLayoutOffsetZeroDirectRejectsWrongSHA(t *testing.T) {
	body := p1b2StandaloneBlock(
		t, profilerDataTypeHiperf, "hiperf-plugin", []byte("sha-must-not-match"), p1b2StandaloneWrongSHA,
	)
	if inventory := p1b2StandaloneInventory(t, body); len(inventory.segments) != 0 {
		t.Fatalf("wrong standalone payload SHA minted inventory: %+v", inventory.segments)
	}
}

func TestStandaloneInventoryConsumerRejectsForgedDirectZeroSHA(t *testing.T) {
	body := p1b2StandaloneBlock(
		t, profilerDataTypeHiperf, "hiperf-plugin", syntheticRawPerfData(),
		p1b2StandalonePayloadSHA)
	inventory := p1b2StandaloneInventory(t, body)
	if len(inventory.segments) != 1 {
		t.Fatalf("exact-digest baseline did not produce one segment: %+v", inventory)
	}
	inventory.segments[0].Integrity = standaloneIntegrityOfficialZero
	if err := validateStandaloneInventoryProof(inventory); err == nil {
		t.Fatal("inventory consumer accepted forged direct official-zero writer profile")
	}
}

func TestStandaloneLayoutAuthenticatedRootAcceptsStrictContiguousChain(t *testing.T) {
	root := p1b2AuthenticatedRoot(t)
	first := p1b2StandaloneBlock(
		t, profilerDataTypeStandalone, "hiebpf-plugin", []byte("opaque"), p1b2StandalonePayloadSHA,
	)
	second := p1b2StandaloneBlock(
		t, profilerDataTypeHiperf, "hiperf-plugin", syntheticRawPerfData(), p1b2StandaloneOfficialZeroSHA,
	)
	body := append(append(append([]byte(nil), root...), first...), second...)
	inventory := p1b2StandaloneInventory(t, body)
	if len(inventory.segments) != 2 ||
		inventory.segments[0].Offset != int64(len(root)) ||
		inventory.segments[0].Length != int64(len(first)) ||
		inventory.segments[0].DataType != profilerDataTypeStandalone ||
		inventory.segments[1].Offset != int64(len(root)+len(first)) ||
		inventory.segments[1].Length != int64(len(second)) ||
		inventory.segments[1].DataType != profilerDataTypeHiperf {
		t.Fatalf("strict authenticated-root standalone chain drifted: %+v", inventory.segments)
	}
}

func TestStandaloneBadChainCannotStarveGoodRootOrRescueBadRoot(t *testing.T) {
	badTail := p1b2StandaloneBlock(
		t, profilerDataTypeHiperf, "hiperf-plugin", syntheticRawPerfData(),
		p1b2StandaloneWrongSHA)
	for _, test := range []struct {
		name         string
		mutateRoot   func([]byte)
		wantRootRows int
	}{
		{name: "good_root_bad_chain", wantRootRows: 1},
		{
			name: "bad_root_bad_chain",
			mutateRoot: func(root []byte) {
				root[24] ^= 0x01
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := append([]byte(nil), p1b2AuthenticatedRoot(t)...)
			if test.mutateRoot != nil {
				test.mutateRoot(root)
			}
			body := append(root, badTail...)
			dir := t.TempDir()
			input := filepath.Join(dir, "root-with-bad-chain.htrace")
			if err := os.WriteFile(input, body, 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := ConvertFile(context.Background(), Options{
				InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"),
				TraceEngine: traceEngineBuiltin,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.EventsWritten != test.wantRootRows ||
				!coverageTableHasSkipped(result.TraceCoverage, "__standalone_layout__",
					"standalone_payload_sha256_mismatch") {
				t.Fatalf("root/bad-chain independence drifted: %+v", result)
			}
			for _, artifact := range result.Artifacts {
				if artifact.Type == ArtifactPerfData || artifact.Type == ArtifactPerfTrace {
					t.Fatalf("bad standalone chain minted a perf child: %+v", artifact)
				}
			}
			if test.wantRootRows > 0 {
				if result.OutputPath == "" || !hasTraceDecision(result.TraceDecisions,
					traceProviderNameBuiltinModern, true) {
					t.Fatalf("good root was starved by a bad sibling chain: %+v", result)
				}
				return
			}
			if result.OutputPath != "" ||
				!hasTraceDecisionReason(result.TraceDecisions, traceProviderNameBuiltinModern,
					"profiler_source_integrity_fail_closed") ||
				!coverageTableHasSkipped(result.TraceCoverage, "__container_integrity_barrier__",
					"profiler_root_payload_sha256_mismatch") {
				t.Fatalf("bad sibling chain rescued or masked a bad root: %+v", result)
			}
		})
	}
}

func TestStandaloneLayoutStrictChainRejectsGapUnknownTypeAndBadSHA(t *testing.T) {
	root := p1b2AuthenticatedRoot(t)
	good := p1b2StandaloneBlock(
		t, profilerDataTypeStandalone, "hiebpf-plugin", []byte("good"), p1b2StandalonePayloadSHA,
	)
	goodPerf := p1b2StandaloneBlock(
		t, profilerDataTypeHiperf, "hiperf-plugin", syntheticRawPerfData(), p1b2StandalonePayloadSHA,
	)
	unknown := p1b2StandaloneBlock(
		t, 77, "unknown-plugin", []byte("unknown"), p1b2StandalonePayloadSHA,
	)
	badSHA := p1b2StandaloneBlock(
		t, profilerDataTypeStandalone, "hiebpf-plugin", []byte("bad-sha"), p1b2StandaloneWrongSHA,
	)
	for _, test := range []struct {
		name string
		body []byte
	}{
		{
			name: "root_end_gap",
			body: append(append(append([]byte(nil), root...), 0xa5), goodPerf...),
		},
		{
			name: "good_unknown_good",
			body: append(append(append(append([]byte(nil), root...), good...), unknown...), goodPerf...),
		},
		{
			name: "good_bad_sha_good",
			body: append(append(append(append([]byte(nil), root...), good...), badSHA...), goodPerf...),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if inventory := p1b2StandaloneInventory(t, test.body); len(inventory.segments) != 0 {
				t.Fatalf("invalid standalone chain retained prefix/suffix inventory: %+v", inventory.segments)
			}
		})
	}
}

func TestStandaloneLayoutPhysicalBlockBudgetCapAndCapPlusOne(t *testing.T) {
	const physicalBlockCap = 256
	build := func(count int) []byte {
		body := append([]byte(nil), p1b2AuthenticatedRoot(t)...)
		for index := 0; index < count; index++ {
			body = append(body, p1b2StandaloneBlock(
				t,
				profilerDataTypeStandalone,
				"hiebpf-plugin",
				[]byte(fmt.Sprintf("block-%03d", index)),
				p1b2StandalonePayloadSHA,
			)...)
		}
		return body
	}
	if inventory := p1b2StandaloneInventory(t, build(physicalBlockCap)); len(inventory.segments) != physicalBlockCap {
		t.Fatalf("physical standalone cap was not admitted atomically: got=%d want=%d", len(inventory.segments), physicalBlockCap)
	}
	if inventory := p1b2StandaloneInventory(t, build(physicalBlockCap+1)); len(inventory.segments) != 0 {
		t.Fatalf("physical standalone cap+1 was silently truncated/published: got=%d", len(inventory.segments))
	}
}

func TestStandaloneLayoutHiperfAdapterBudgetCapAndCapPlusOne(t *testing.T) {
	const hiperfAdapterCap = 64
	build := func(count int) []byte {
		body := append([]byte(nil), p1b2AuthenticatedRoot(t)...)
		for index := 0; index < count; index++ {
			payload := make([]byte, 8)
			binary.LittleEndian.PutUint64(payload, uint64(index+1))
			body = append(body, p1b2StandaloneBlock(
				t,
				profilerDataTypeHiperf,
				"hiperf-plugin",
				payload,
				p1b2StandalonePayloadSHA,
			)...)
		}
		return body
	}
	if inventory := p1b2StandaloneInventory(t, build(hiperfAdapterCap)); len(inventory.segments) != hiperfAdapterCap {
		t.Fatalf("HIPERF adapter cap was not admitted atomically: got=%d want=%d", len(inventory.segments), hiperfAdapterCap)
	}
	if inventory := p1b2StandaloneInventory(t, build(hiperfAdapterCap+1)); len(inventory.segments) != 0 {
		t.Fatalf("HIPERF adapter cap+1 was silently truncated/published: got=%d", len(inventory.segments))
	}
}

func TestStandaloneLayoutBudgetsCloseBeforeAnyPayloadHash(t *testing.T) {
	build := func(count int, dataType uint32, pluginName string) *p1b2PayloadPoisonReaderAt {
		reader := &p1b2PayloadPoisonReaderAt{}
		for index := 0; index < count; index++ {
			block := p1b2StandaloneBlock(
				t, dataType, pluginName, []byte{byte(index)}, p1b2StandalonePayloadSHA)
			start := int64(len(reader.body))
			reader.body = append(reader.body, block...)
			reader.payloads = append(reader.payloads, p1b2StandaloneByteRange{
				start: start + profilerStandalonePayloadBase,
				end:   start + int64(len(block)),
			})
		}
		return reader
	}
	for _, test := range []struct {
		name       string
		reader     *p1b2PayloadPoisonReaderAt
		wantReason string
	}{
		{
			name: "physical_cap_plus_one",
			reader: build(maxProfilerStandaloneBlocks+1,
				profilerDataTypeStandalone, "hiebpf-plugin"),
			wantReason: "standalone_block_budget_exceeded",
		},
		{
			name: "hiperf_cap_plus_one",
			reader: build(maxProfilerHiperfCandidates+1,
				profilerDataTypeHiperf, "hiperf-plugin"),
			wantReason: "standalone_hiperf_budget_exceeded",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			segments, failure, err := validateProfilerStandaloneChain(
				context.Background(), test.reader, int64(len(test.reader.body)), 0,
				false, standaloneLayoutDirectOffsetZero)
			if err != nil || failure != test.wantReason || len(segments) != 0 {
				t.Fatalf("pre-hash budget gate drifted: segments=%d failure=%q err=%v",
					len(segments), failure, err)
			}
			if test.reader.payloadReads != 0 {
				t.Fatalf("budget rejection touched %d payload range(s) before admission", test.reader.payloadReads)
			}
		})
	}
}

func TestStandaloneLayoutBudgetFailuresPublishResourceClass(t *testing.T) {
	build := func(count int, dataType uint32, pluginName string) []byte {
		var body []byte
		for index := 0; index < count; index++ {
			body = append(body, p1b2StandaloneBlock(
				t, dataType, pluginName, []byte{byte(index)}, p1b2StandalonePayloadSHA)...)
		}
		return body
	}
	for _, test := range []struct {
		name   string
		body   []byte
		reason string
	}{
		{
			name: "physical_cap_plus_one",
			body: build(maxProfilerStandaloneBlocks+1,
				profilerDataTypeStandalone, "hiebpf-plugin"),
			reason: "profiler_standalone_chain_standalone_block_budget_exceeded",
		},
		{
			name: "hiperf_cap_plus_one",
			body: build(maxProfilerHiperfCandidates+1,
				profilerDataTypeHiperf, "hiperf-plugin"),
			reason: "profiler_standalone_chain_standalone_hiperf_budget_exceeded",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "cap-plus-one.htrace")
			if err := os.WriteFile(input, test.body, 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := ConvertFile(context.Background(), Options{
				InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"),
				TraceEngine: traceEngineBuiltin,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.OutputPath != "" || result.EventsWritten != 0 ||
				!hasTraceDecisionReason(result.TraceDecisions, traceProviderNameBuiltinModern,
					"profiler_source_resource_fail_closed") ||
				!coverageTableHasSkipped(result.TraceCoverage,
					"__container_resource_barrier__", test.reason) ||
				coverageTableHasSkipped(result.TraceCoverage,
					"__container_integrity_barrier__", test.reason) {
				t.Fatalf("standalone budget failure class drifted: %+v", result)
			}
		})
	}
}

func TestStandaloneLayoutCapFollowedByInvalidTailRemainsIntegrityClass(t *testing.T) {
	var prefix []byte
	for index := 0; index < maxProfilerStandaloneBlocks; index++ {
		prefix = append(prefix, p1b2StandaloneBlock(
			t, profilerDataTypeStandalone, "hiebpf-plugin", []byte{byte(index)},
			p1b2StandalonePayloadSHA)...)
	}
	for _, test := range []struct {
		name       string
		tail       []byte
		wantReason string
	}{
		{name: "short_tail", tail: []byte{0xa5}, wantReason: "standalone_header_truncated"},
		{name: "bad_magic_header", tail: make([]byte, profilerTraceHeaderSize), wantReason: "standalone_magic_mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := append(append([]byte(nil), prefix...), test.tail...)
			inventory := p1b2StandaloneInventory(t, body)
			if len(inventory.segments) != 0 || inventory.standaloneChainError != test.wantReason {
				t.Fatalf("cap-following invalid tail was mislabeled: segments=%d failure=%q want=%q",
					len(inventory.segments), inventory.standaloneChainError, test.wantReason)
			}
			dir := t.TempDir()
			input := filepath.Join(dir, "cap-invalid-tail.htrace")
			if err := os.WriteFile(input, body, 0o600); err != nil {
				t.Fatal(err)
			}
			fullReason := "profiler_standalone_chain_" + test.wantReason
			result, err := ConvertFile(context.Background(), Options{
				InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"),
				TraceEngine: traceEngineBuiltin,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !hasTraceDecisionReason(result.TraceDecisions, traceProviderNameBuiltinModern,
				"profiler_source_integrity_fail_closed") ||
				!coverageTableHasSkipped(result.TraceCoverage,
					"__container_integrity_barrier__", fullReason) ||
				coverageTableHasSkipped(result.TraceCoverage,
					"__container_resource_barrier__", fullReason) {
				t.Fatalf("cap-following invalid tail failure class drifted: %+v", result)
			}
		})
	}
}
