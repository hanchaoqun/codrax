package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// source_raw_lane_gate_behavior_test.go — colleague_merge_audit §40.53 (V6-4):
// the source-raw recovery lanes' pre-publication outcome is a closed three-way
// split — not applicable (envelope is not the official profile) / census
// incomplete (official envelope whose strict decode census did not close) /
// ready (lane-specific withheld_/complete_/published_ arms) — and no lane may
// wear a neighbouring label. These pins reference only identifiers that exist
// before the fix so they compile, and are red, on the pre-fix tree; the
// go/ast censuses that need the new declarations live in
// source_raw_lane_gate_census_test.go.

// traceDBSourceRawGateFixture builds one capture per gate arm and returns the
// scanned inventory. Arms: "official_truncated_segment" (official envelope,
// segment inventory incomplete), "official_page_layout_rejected" (official
// envelope, page/format profile not ready), "official_corrupt_envelope"
// (official envelope, census complete, visibility candidate envelope
// rejected), "legacy_envelope" (0x0ace).
func traceDBSourceRawGateFixture(t *testing.T, arm string) traceDBSourceNameInventory {
	t.Helper()
	var capture bytes.Buffer
	switch arm {
	case "official_truncated_segment":
		writeFileHeader(&capture, 2)
		body := capture.Bytes()
		binary.LittleEndian.PutUint16(body[0:2], traceStreamerRawTraceMagic)
		body[2] = 0
		capture.Reset()
		capture.Write(body)
		var header [segmentHdrSize]byte
		binary.LittleEndian.PutUint32(header[0:4], segmentRawTrace)
		binary.LittleEndian.PutUint32(header[4:8], 4096)
		capture.Write(header[:])
		capture.WriteByte(0)
	case "official_page_layout_rejected":
		writeFileHeader(&capture, 2)
		header := capture.Bytes()
		binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
		capture.Reset()
		capture.Write(header)
		writeSegment(&capture, segmentEventsFormat, []byte(strings.Join(
			syntheticFormatBlock("sched_switch", 90, []string{
				syntheticField("unsigned short", "common_type", 0, 2, false),
			}), "\n")))
		page := make([]byte, tracePageSize)
		binary.LittleEndian.PutUint64(page[8:16], tracePageSize)
		page[16] = 1
		writeSegment(&capture, segmentRawTrace, page)
	case "official_corrupt_envelope":
		raw, _ := traceDBRawVisibilityCapture(t, true)
		capture.Write(raw)
	case "legacy_envelope":
		format := traceDBRawVisibilityFormat("hmfs_writepage")
		content := traceDBRawVisibilityContent(format)
		writeFileHeader(&capture, 4)
		writeSegment(&capture, segmentCmdlines, []byte("25827 com.tencent.mm\n"))
		writeSegment(&capture, segmentEventsFormat, []byte(directPairFormatBlock(format.ID, format)))
		writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
			{EventID: uint16(format.ID), OffsetNS: 1000, Content: content},
		}))
	default:
		t.Fatalf("unknown gate fixture arm %q", arm)
	}
	path := filepath.Join(t.TempDir(), arm+".sys")
	if err := os.WriteFile(path, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { authority.Close() })
	inventory, err := scanTraceDBSourceNameInventory(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}

// TestTraceDBSourceRawVisibilityCensusIncompleteIsNotNotApplicable: an OFFICIAL
// capture whose strict decode census did not close was published as "not
// applicable: source profile absent" (Found even left false). It is a
// census-incomplete outcome, disclosed with the verbatim decode_state; the
// non-official envelope is the only not-applicable cause; the envelope-gap
// withheld arm and the published arm are unchanged.
func TestTraceDBSourceRawVisibilityCensusIncompleteIsNotNotApplicable(t *testing.T) {
	for _, test := range []struct {
		arm         string
		wantState   string
		wantReason  string
		wantFound   bool
		wantSkipped string
	}{
		{
			arm: "official_truncated_segment", wantState: "census_incomplete_source_raw_decode",
			wantReason: "withheld_segment_inventory_incomplete", wantFound: true,
			wantSkipped: "source raw visibility not evaluated: source raw decode census incomplete (withheld_segment_inventory_incomplete)",
		},
		{
			arm: "official_page_layout_rejected", wantState: "census_incomplete_source_raw_decode",
			wantReason: "withheld_profile_not_ready", wantFound: true,
			wantSkipped: "source raw visibility not evaluated: source raw decode census incomplete (withheld_profile_not_ready)",
		},
		{
			arm: "official_corrupt_envelope", wantState: "withheld_visibility_envelope_incomplete",
			wantFound:   true,
			wantSkipped: "source raw visibility withheld: candidate common-envelope census is incomplete",
		},
		{
			arm: "legacy_envelope", wantState: "not_applicable_source_profile",
			wantFound:   false,
			wantSkipped: "source raw visibility not applicable: strict official source raw profile absent",
		},
	} {
		t.Run(test.arm, func(t *testing.T) {
			inventory := traceDBSourceRawGateFixture(t, test.arm)
			sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			coverage, err := publishTraceDBSourceRawVisibility(context.Background(), &inventory, sink)
			if err != nil {
				t.Fatal(err)
			}
			if coverage.Metadata["publication_state"] != test.wantState ||
				coverage.Metadata["census_incomplete_reason"] != test.wantReason ||
				coverage.Found != test.wantFound || coverage.RowsEmitted != 0 ||
				sink.stats.RowsAccepted != 0 || coverage.Skipped != test.wantSkipped {
				t.Fatalf("visibility gate outcome drifted: %+v", coverage)
			}
			// The prose (everything before the verbatim decode_state token in
			// parentheses) must not borrow a neighbouring label.
			if prose, _, _ := strings.Cut(coverage.Skipped, " ("); test.wantState == "census_incomplete_source_raw_decode" &&
				(strings.Contains(prose, "not applicable") || strings.Contains(prose, "withheld")) {
				t.Fatalf("census-incomplete prose borrowed a neighbouring label: %q", coverage.Skipped)
			}
			if test.wantState == "not_applicable_source_profile" &&
				coverage.Metadata["not_applicable_reason"] != "not_applicable_non_official_profile" {
				t.Fatalf("not-applicable arm lost its decode_state disclosure: %+v", coverage.Metadata)
			}
			// The postvalidation reader accepts every minted zero-row state.
			if rows, err := traceDBSourceRawVisibilityPublishedRows([]TraceDBCoverage{coverage}); err != nil || rows != 0 {
				t.Fatalf("reader rejected a minted zero-row state %q: rows=%d err=%v", test.wantState, rows, err)
			}
		})
	}
}

// TestTraceDBSourceRawSiblingLanesShareTheGateSplit: block, exact, DMA wait,
// DMA lifecycle and marker sync consume the same classifier. DMA/marker used
// to mint `withheld_raw_decode_incomplete` for BOTH the non-official envelope
// and a genuinely incomplete census; block/exact folded census-incomplete into
// their family-withheld arm.
func TestTraceDBSourceRawSiblingLanesShareTheGateSplit(t *testing.T) {
	type laneRun func(t *testing.T, inventory *traceDBSourceNameInventory) []TraceDBCoverage
	lanes := map[string]laneRun{
		"block": func(t *testing.T, inventory *traceDBSourceNameInventory) []TraceDBCoverage {
			out, err := publishTraceDBRawBlockRecovery(context.Background(), inventory, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			return []TraceDBCoverage{out}
		},
		"exact": func(t *testing.T, inventory *traceDBSourceNameInventory) []TraceDBCoverage {
			items, err := publishTraceDBRawExactRecoveries(context.Background(), inventory, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			return items
		},
		"dma_wait": func(t *testing.T, inventory *traceDBSourceNameInventory) []TraceDBCoverage {
			out, err := publishTraceDBRawDMAWaitRecovery(context.Background(), inventory, nil, traceDBSchedulerAuthority{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			return []TraceDBCoverage{out}
		},
		"dma_lifecycle": func(t *testing.T, inventory *traceDBSourceNameInventory) []TraceDBCoverage {
			out, err := publishTraceDBRawDMALifecycleRecovery(context.Background(), inventory, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			return []TraceDBCoverage{out}
		},
		"marker_sync": func(t *testing.T, inventory *traceDBSourceNameInventory) []TraceDBCoverage {
			out, err := submitTraceDBRawMarkerSyncRecovery(context.Background(), inventory, traceDBSchedulerAuthority{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			return []TraceDBCoverage{out}
		},
	}
	for _, test := range []struct {
		arm        string
		wantState  string
		wantReason string
		wantFound  bool
		wantPhrase string
	}{
		{arm: "legacy_envelope", wantState: "not_applicable_source_profile", wantPhrase: " not applicable: strict official source raw profile absent"},
		{arm: "official_truncated_segment", wantState: "census_incomplete_source_raw_decode", wantReason: "withheld_segment_inventory_incomplete", wantFound: true, wantPhrase: " not evaluated: source raw decode census incomplete (withheld_segment_inventory_incomplete)"},
		{arm: "official_page_layout_rejected", wantState: "census_incomplete_source_raw_decode", wantReason: "withheld_profile_not_ready", wantFound: true, wantPhrase: " not evaluated: source raw decode census incomplete (withheld_profile_not_ready)"},
	} {
		inventory := traceDBSourceRawGateFixture(t, test.arm)
		for name, run := range lanes {
			t.Run(test.arm+"/"+name, func(t *testing.T) {
				for _, out := range run(t, &inventory) {
					if out.Metadata["publication_state"] != test.wantState ||
						out.Metadata["census_incomplete_reason"] != test.wantReason ||
						out.Found != test.wantFound || out.RowsEmitted != 0 ||
						!strings.HasSuffix(out.Skipped, test.wantPhrase) {
						t.Fatalf("%s lane mislabeled the %s arm: %+v", name, test.arm, out)
					}
				}
			})
		}
	}
}

// TestTraceDBSourceRawRetainedFamilyLanesNameTheRetentionWithdrawal: past the
// gate, the family predicate can only fail on this family's retained store
// being withdrawn by byte budget — the arm says so instead of the old blanket
// "raw decode incomplete".
func TestTraceDBSourceRawRetainedFamilyLanesNameTheRetentionWithdrawal(t *testing.T) {
	inventory := newTraceDBSourceNameInventory()
	inventory.RawDecode.Found = true
	inventory.RawDecode.Metadata["decode_state"] = "strict_target_ledger_complete_with_family_retention_withdrawal"
	for _, family := range []string{traceDBRawRetentionDMAWait, traceDBRawRetentionDMALifecycle, traceDBRawRetentionMarker} {
		inventory.RawDecode.Metadata["retention_"+family+"_state"] = "incomplete_byte_budget"
	}
	dmaWait, err := publishTraceDBRawDMAWaitRecovery(context.Background(), &inventory, nil, traceDBSchedulerAuthority{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dmaLifecycle, err := publishTraceDBRawDMALifecycleRecovery(context.Background(), &inventory, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	marker, err := submitTraceDBRawMarkerSyncRecovery(context.Background(), &inventory, traceDBSchedulerAuthority{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]TraceDBCoverage{"dma_wait": dmaWait, "dma_lifecycle": dmaLifecycle, "marker_sync": marker} {
		if out.Metadata["publication_state"] != "withheld_family_retention_budget_exceeded" ||
			!strings.HasSuffix(out.Skipped, " withheld: retained family record store exceeded its byte budget") ||
			!out.Found || out.RowsEmitted != 0 {
			t.Fatalf("%s lane did not name the retention withdrawal: %+v", name, out)
		}
	}
}

// TestTraceDBSourceRawLaneGateFailsLoudOnUnrecognizedShape: a decode_state
// outside the declared set, or a Found bit contradicting the state's gate
// kind, is not absorbed into a neighbouring label — the lane fails closed with
// a typed reason and no coverage state is minted (§40.50 ruling). A ready gate
// without the held replay generation is likewise loud.
func TestTraceDBSourceRawLaneGateFailsLoudOnUnrecognizedShape(t *testing.T) {
	for name, shape := range map[string]struct {
		found bool
		state string
		want  string
	}{
		"unknown_decode_state":            {found: true, state: "strict_target_ledger_partially_new", want: "source_raw_lane_gate_unresolved"},
		"not_applicable_but_found":        {found: true, state: "not_applicable_non_official_profile", want: "source_raw_lane_gate_unresolved"},
		"census_incomplete_but_not_found": {found: false, state: "withheld_profile_not_ready", want: "source_raw_lane_gate_unresolved"},
		"ready_without_replay":            {found: true, state: "strict_target_ledger_complete", want: "source_raw_visibility_replay_authority_missing"},
	} {
		t.Run(name, func(t *testing.T) {
			inventory := newTraceDBSourceNameInventory()
			inventory.RawDecode.Found = shape.found
			inventory.RawDecode.Metadata["decode_state"] = shape.state
			coverage, err := publishTraceDBSourceRawVisibility(context.Background(), &inventory, nil)
			var invariant *traceDBOutputInvariantError
			if !errors.As(err, &invariant) || invariant.Reason != shape.want {
				t.Fatalf("unrecognized gate shape was absorbed: err=%v coverage=%+v", err, coverage)
			}
			if coverage.Error == "" || coverage.Metadata["publication_state"] != "unavailable" {
				t.Fatalf("failed gate minted a publication state: %+v", coverage)
			}
			if _, readerErr := traceDBSourceRawVisibilityPublishedRows([]TraceDBCoverage{coverage}); readerErr == nil {
				t.Fatal("postvalidation accepted a lane that failed its gate")
			}
		})
	}
}

// TestTraceDBSemanticQualityDoesNotEvaluateNonReadyRawMarkerLanes extends the
// withheld pin to the two gate outcomes: a not-applicable or census-incomplete
// marker lane yields not_evaluated_<state> and mints no replacement metric.
func TestTraceDBSemanticQualityDoesNotEvaluateNonReadyRawMarkerLanes(t *testing.T) {
	for _, state := range []string{
		"not_applicable_source_profile",
		"census_incomplete_source_raw_decode",
		"withheld_family_retention_budget_exceeded",
	} {
		t.Run(state, func(t *testing.T) {
			quality := traceDBSemanticQualityCoverage([]TraceDBCoverage{
				{Family: "slice", Table: "callstack", Metrics: map[string]int64{"sync_spans_suppressed": 7}},
				{
					Family: "source_rawtrace_marker_sync", Table: "__raw_marker_sync__",
					Metadata: map[string]string{"publication_state": state},
				},
			})
			if got := quality.Metadata["raw_marker_replacement_closure"]; got != "not_evaluated_"+state {
				t.Fatalf("%s marker lane was presented as evaluated: %+v", state, quality)
			}
			if quality.Metrics["callstack_sync_spans_recovered_by_raw_marker"] != 0 {
				t.Fatalf("%s marker lane minted replacement evidence: %+v", state, quality)
			}
		})
	}
}
