package hitraceconv

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

func openProfilerSourceOrderRouteSink(t testing.TB) *traceDBRowSink {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "capture.sys")
	if err := os.WriteFile(source, []byte("source-order-route-fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(filepath.Join(root, "sort"), 128)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sink.tempDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := sink.openProfilerCapture(source); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sink.abortPairRowCensus()
		if err := sink.cleanup(); err != nil {
			t.Errorf("cleanup source-order route sink: %v", err)
		}
	})
	return sink
}

func TestProfilerSourceOrderProofCoversEveryProfilerPublisherLane(t *testing.T) {
	lanes := profilerSequenceAuthorityCases(t)
	generic := lanes[0]
	other := generic
	other.name = "other-text"
	other.publisher = profilerPairPublisherOtherText
	session := generic
	session.name = "session"
	session.publisher = profilerPairPublisherSession
	session.text = false
	lanes = append(lanes, other, session)

	roots := make(map[[sha256.Size]byte]string, len(lanes))
	for _, lane := range lanes {
		lane := lane
		t.Run(lane.name, func(t *testing.T) {
			sink := openProfilerSourceOrderRouteSink(t)
			beginProfilerSequencePublisher(t, sink, lane)
			seq := 0
			result := lane.run(context.Background(), true, &seq, sink)
			if result.err != nil || result.rows != 2 || seq != 2 || sink.stats.RowsAccepted != 2 ||
				sink.nextIngestOrdinal != 2 || sink.profilerSourceProof.count != 2 {
				t.Fatalf("%s publisher proof transaction drifted: rows=%d seq=%d accepted=%d ordinal=%d proof=%d err=%v",
					lane.name, result.rows, seq, sink.stats.RowsAccepted, sink.nextIngestOrdinal,
					sink.profilerSourceProof.count, result.err)
			}
			if lane.text {
				if err := sink.endProfilerTextMessage(result.rows); err != nil {
					t.Fatalf("end %s text message: %v", lane.name, err)
				}
			}
			_ = sink.endPairRowCensus()
			root, ok := sink.profilerSourceProof.terminalDigest()
			if !ok || root == [sha256.Size]byte{} {
				t.Fatalf("%s publisher lacks a terminal source proof: ok=%t root=%x proof=%+v",
					lane.name, ok, root, sink.profilerSourceProof)
			}
			var recomputed profilerSourceOrderProof
			recomputed.activate()
			defer recomputed.reset()
			pairLaneResolved := false
			for index, row := range sink.rows {
				ordinal := sink.rowIngestOrdinals[index]
				if err := recomputed.prepareStoredRowContext(context.Background(), row, ordinal); err != nil {
					t.Fatalf("recompute %s accepted row %d: %v", lane.name, index, err)
				}
				recomputed.commitPreparedRow(row.profilerProvenance())
				if profilerPairBudgetKind(row.provenance.PairKind) && row.provenance.LaneID != 0 {
					pairLaneResolved = true
				}
			}
			recomputedRoot, recomputedOK := recomputed.terminalDigest()
			if !pairLaneResolved || !recomputedOK || recomputed.count != sink.profilerSourceProof.count ||
				recomputedRoot != root {
				t.Fatalf("%s producer proof did not bind final stored provenance: lane_resolved=%t ok=%t count=%d/%d root=%x/%x rows=%+v",
					lane.name, pairLaneResolved, recomputedOK, recomputed.count, sink.profilerSourceProof.count,
					recomputedRoot, root, sink.rows)
			}
			if previous, duplicate := roots[root]; duplicate {
				t.Fatalf("publisher provenance did not separate %s from %s: root=%x", lane.name, previous, root)
			}
			roots[root] = lane.name
		})
	}
}
