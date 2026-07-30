package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// sameInputAccountingReceipt is a test-only, path/time-independent projection
// of one complete SQL conversion. It proves deterministic accounting for the
// same immutable input without turning a synthetic DB into a real-capture
// parity oracle.
type sameInputAccountingReceipt struct {
	InputBytes        int64                       `json:"input_bytes"`
	InputSHA256       string                      `json:"input_sha256"`
	ChildInputBytes   int64                       `json:"child_input_bytes"`
	ChildInputSHA256  string                      `json:"child_input_sha256"`
	OutputBytes       int64                       `json:"output_bytes"`
	OutputSHA256      string                      `json:"output_sha256"`
	EventsWritten     int                         `json:"events_written"`
	ArtifactRows      int                         `json:"artifact_rows"`
	ArtifactKnown     int                         `json:"artifact_known"`
	ArtifactAuthority int                         `json:"artifact_authority"`
	ArtifactAdvisory  int                         `json:"artifact_advisory"`
	ArtifactReady     bool                        `json:"artifact_ready"`
	TypedTextRecords  int                         `json:"typed_text_records"`
	Coverage          []sameInputCoverageReceipt  `json:"coverage"`
	TraceReceipt      []sameInputCoverageReceipt  `json:"trace_receipt"`
	EventTypes        []sameInputEventTypeReceipt `json:"event_types"`
}

type sameInputCoverageReceipt struct {
	Family      string                   `json:"family"`
	Table       string                   `json:"table"`
	Role        string                   `json:"role"`
	Found       bool                     `json:"found"`
	RowsRead    int                      `json:"rows_read"`
	RowsEmitted int                      `json:"rows_emitted"`
	Skipped     string                   `json:"skipped"`
	Error       string                   `json:"error"`
	Metrics     map[string]int64         `json:"metrics,omitempty"`
	Capture     *sameInputCaptureReceipt `json:"capture,omitempty"`
}

type sameInputCaptureReceipt struct {
	State            string `json:"state"`
	RowsAccepted     int    `json:"rows_accepted"`
	Received         uint64 `json:"received"`
	DataLost         uint64 `json:"data_lost"`
	NotMatch         uint64 `json:"not_match"`
	NotSupported     uint64 `json:"not_supported"`
	InvalidData      uint64 `json:"invalid_data"`
	NonzeroIssueRows int    `json:"nonzero_issue_rows"`
}

type sameInputEventTypeReceipt struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

func TestSameInputTraceStreamerAccountingReceiptIsDeterministic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh; Windows snapshot parity is pinned by its platform lease tests")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "same-input.sys")
	inputBody := syntheticRootCauseMatrixSysBinary(t)
	if err := os.WriteFile(input, inputBody, 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, traceStreamerRootCauseMatrixDBStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)

	var receipts []sameInputAccountingReceipt
	for run := 1; run <= 2; run++ {
		childInput := filepath.Join(dir, "child-input-"+strconv.Itoa(run)+".sys")
		t.Setenv("TRACE_STREAMER_CONSUMED_INPUT", childInput)
		output := filepath.Join(dir, "same-input-"+strconv.Itoa(run)+".systrace")
		result, err := ConvertFile(context.Background(), Options{
			InputPath: input, OutputPath: output,
			TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: traceStreamer,
		})
		if err != nil {
			t.Fatalf("same-input conversion run %d: %v", run, err)
		}
		receipt := buildSameInputAccountingReceipt(t, input, childInput, output, result)
		if receipt.InputBytes != receipt.ChildInputBytes ||
			receipt.InputSHA256 != receipt.ChildInputSHA256 {
			t.Fatalf("trace_streamer child did not consume the held input generation: %+v", receipt)
		}
		receipts = append(receipts, receipt)
	}
	if !reflect.DeepEqual(receipts[0], receipts[1]) {
		left, _ := json.MarshalIndent(receipts[0], "", "  ")
		right, _ := json.MarshalIndent(receipts[1], "", "  ")
		t.Fatalf("identical input produced different accounting receipts:\nrun1=%s\nrun2=%s", left, right)
	}
	assertSameInputAccountingGolden(t, receipts[0])
}

func buildSameInputAccountingReceipt(
	t *testing.T,
	input, childInput, output string,
	result Result,
) sameInputAccountingReceipt {
	t.Helper()
	inputBytes, inputSHA := sameInputFileReceipt(t, input)
	childBytes, childSHA := sameInputFileReceipt(t, childInput)
	outputBytes, outputSHA := sameInputFileReceipt(t, output)
	if result.InputBytes != inputBytes || result.OutputBytes != outputBytes {
		t.Fatalf("Result byte accounting disagrees with physical files: result=%+v input=%d output=%d", result, inputBytes, outputBytes)
	}
	receipt := sameInputAccountingReceipt{
		InputBytes: inputBytes, InputSHA256: inputSHA,
		ChildInputBytes: childBytes, ChildInputSHA256: childSHA,
		OutputBytes: outputBytes, OutputSHA256: outputSHA,
		EventsWritten: result.EventsWritten,
		Coverage:      sameInputCoverageProjection(result.TraceDBCoverage),
		TraceReceipt:  sameInputCoverageProjection(result.TraceCoverage),
	}
	for _, artifact := range result.Artifacts {
		if artifact.Type != ArtifactSystrace || artifact.Path != output || artifact.Trace == nil {
			continue
		}
		if artifact.Bytes != outputBytes || artifact.SHA256 != outputSHA {
			t.Fatalf("systrace artifact receipt disagrees with physical output: artifact=%+v bytes=%d sha=%s", artifact, outputBytes, outputSHA)
		}
		receipt.ArtifactRows = artifact.Trace.Rows
		receipt.ArtifactKnown = artifact.Trace.Known
		receipt.ArtifactAuthority = artifact.Trace.AuthoritativeKnown
		receipt.ArtifactAdvisory = artifact.Trace.AdvisoryRows
		receipt.TypedTextRecords = artifact.Trace.AdvisoryRows
		receipt.ArtifactReady = artifact.Trace.TraceQueryReady
	}
	idx, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	eventCounts := make(map[string]int)
	for _, event := range idx.Events {
		eventCounts[string(event.Type)]++
	}
	for eventType, count := range eventCounts {
		receipt.EventTypes = append(receipt.EventTypes, sameInputEventTypeReceipt{Type: eventType, Count: count})
	}
	sort.Slice(receipt.EventTypes, func(i, j int) bool {
		return receipt.EventTypes[i].Type < receipt.EventTypes[j].Type
	})
	return receipt
}

func sameInputFileReceipt(t *testing.T, path string) (int64, string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return int64(len(body)), hex.EncodeToString(sum[:])
}

func sameInputCoverageProjection(items []TraceDBCoverage) []sameInputCoverageReceipt {
	out := make([]sameInputCoverageReceipt, 0, len(items))
	for _, item := range items {
		projected := sameInputCoverageReceipt{
			Family: item.Family, Table: item.Table, Role: item.Role,
			Found: item.Found, RowsRead: item.RowsRead, RowsEmitted: item.RowsEmitted,
			Skipped: item.Skipped, Error: item.Error,
		}
		if len(item.Metrics) != 0 {
			projected.Metrics = make(map[string]int64, len(item.Metrics))
			for key, value := range item.Metrics {
				projected.Metrics[key] = value
			}
		}
		if capture := item.CaptureCompleteness; capture != nil {
			projected.Capture = &sameInputCaptureReceipt{
				State: capture.State, RowsAccepted: capture.RowsAccepted,
				Received: capture.Received, DataLost: capture.DataLost,
				NotMatch: capture.NotMatch, NotSupported: capture.NotSupported,
				InvalidData: capture.InvalidData, NonzeroIssueRows: capture.NonzeroIssueRows,
			}
		}
		out = append(out, projected)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Family != out[j].Family {
			return out[i].Family < out[j].Family
		}
		if out[i].Table != out[j].Table {
			return out[i].Table < out[j].Table
		}
		return out[i].Role < out[j].Role
	})
	return out
}

func assertSameInputAccountingGolden(t *testing.T, receipt sameInputAccountingReceipt) {
	t.Helper()
	compact, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(compact)
	const (
		wantInputBytes  = 8442
		wantInputSHA    = "6294cbbff9509cc1458771f83f0c44d49a224eeead56b4a2e49aa8c64b0271ab"
		wantOutputBytes = 37140
		wantOutputSHA   = "427d8b8664897dba6641f271fb01ec29a3870c18b9417c26019da8ccb8388752"
		wantReceiptSHA  = "2dbdef5a78147a350af46637bbfc34c54d8bafd6d5b9f7c2c755133a7a123573"
		wantEvents      = 35
		wantAuthority   = 18
		wantAdvisory    = 17
	)
	gotReceiptSHA := hex.EncodeToString(sum[:])
	if receipt.InputBytes != wantInputBytes || receipt.ChildInputBytes != wantInputBytes ||
		receipt.InputSHA256 != wantInputSHA || receipt.ChildInputSHA256 != wantInputSHA ||
		receipt.OutputBytes != wantOutputBytes || receipt.OutputSHA256 != wantOutputSHA ||
		receipt.EventsWritten != wantEvents || receipt.ArtifactRows != wantEvents ||
		receipt.ArtifactKnown != wantEvents || receipt.ArtifactAuthority != wantAuthority ||
		receipt.ArtifactAdvisory != wantAdvisory || receipt.TypedTextRecords != wantAdvisory ||
		!receipt.ArtifactReady ||
		gotReceiptSHA != wantReceiptSHA {
		body, _ := json.MarshalIndent(receipt, "", "  ")
		t.Fatalf("same-input accounting golden drifted: digest=%s want=%s\n%s", gotReceiptSHA, wantReceiptSHA, body)
	}
	sorter := sameInputCoverageByKey(receipt.Coverage, "sorter", "__systrace_rows__", "systrace_text_output")
	crossValidation := sameInputCoverageByKey(receipt.TraceReceipt, "trace_cross_validation", "tracequery_build_index", "tracequery_cross_validation")
	rawAuthority := sameInputCoverageByKey(receipt.Coverage, "source_rawtrace_authority", "__source_segments__", "diagnostic_inventory")
	rawBlockedKey := sameInputCoverageByKey(receipt.Coverage, "source_rawtrace_blocked_key", "__raw_vs_db_blocked_key__", "diagnostic_deduplication")
	rawBlockedRecovery := sameInputCoverageByKey(receipt.Coverage, "source_rawtrace_blocked_recovery", "__raw_only_blocked_reason__", "query_ready_export")
	frameRoster := sameInputCoverageByKey(receipt.Coverage, "resolver.frame", "frame_slice", "resolver_index")
	frameCallstack := sameInputCoverageByKey(receipt.Coverage, "relation", "frame_slice_callstack", "query_ready_export")
	frameGPU := sameInputCoverageByKey(receipt.Coverage, "resource.relation", "gpu_slice", "query_ready_export")
	perfNAPI := sameInputCoverageByKey(receipt.Coverage, "perf.relation", "perf_napi_async", "query_ready_export")
	ebpfCallstack := sameInputCoverageByKey(receipt.Coverage, "resolver.ebpf", "ebpf_callstack", "resolver_index")
	ebpfFilesystem := sameInputCoverageByKey(receipt.Coverage, "ebpf.interval", "file_system_sample", "query_ready_export")
	ebpfPagedMemory := sameInputCoverageByKey(receipt.Coverage, "ebpf.interval", "paged_memory_sample", "query_ready_export")
	ebpfBIO := sameInputCoverageByKey(receipt.Coverage, "ebpf.interval", "bio_latency_sample", "query_ready_export")
	if sorter == nil || sorter.RowsRead != wantEvents || sorter.RowsEmitted != wantEvents ||
		crossValidation == nil || crossValidation.RowsEmitted != wantEvents ||
		rawAuthority == nil || rawAuthority.RowsRead != 4 || rawAuthority.RowsEmitted != 4 ||
		rawBlockedKey == nil || rawBlockedKey.RowsEmitted != 0 ||
		rawBlockedRecovery == nil || rawBlockedRecovery.RowsEmitted != 0 ||
		frameRoster == nil || frameCallstack == nil || frameGPU == nil || perfNAPI == nil ||
		ebpfCallstack == nil || ebpfFilesystem == nil || ebpfPagedMemory == nil || ebpfBIO == nil ||
		rawAuthority.Metrics["event_format_segments"] != 1 ||
		rawAuthority.Metrics["raw_trace_segments"] != 1 {
		t.Fatalf("same-input fixture did not produce a receipt-validated query-ready artifact: %+v", receipt)
	}
}

func sameInputCoverageByKey(items []sameInputCoverageReceipt, family, table, role string) *sameInputCoverageReceipt {
	for i := range items {
		if items[i].Family == family && items[i].Table == table && items[i].Role == role {
			return &items[i]
		}
	}
	return nil
}
