package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func profilerSourceOrderGoldenDigest(t testing.TB, encoded string) [sha256.Size]byte {
	t.Helper()
	raw, err := hex.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode independent source-order golden: %v", err)
	}
	if len(raw) != sha256.Size {
		t.Fatalf("source-order golden bytes=%d want=%d", len(raw), sha256.Size)
	}
	var digest [sha256.Size]byte
	copy(digest[:], raw)
	return digest
}

func profilerSourceOrderGoldenRows() (renderedRow, renderedRow) {
	first := renderedRow{
		tsNS:                       0x0102030405060708,
		seq:                        0x01020304,
		line:                       "A|\xce\xb1",
		pairKind:                   pairRenderF2FS,
		profilerEndpointSlot:       profilerPairEndpointF2FSWriteBegin,
		profilerPublisherSlot:      profilerPairPublisherOtherText,
		profilerProvenanceFlags:    profilerPairRowProvenanceText,
		profilerTraceClass:         profilerTraceClassTextKnown,
		profilerLaneID:             0x0a0b0c0d,
		profilerTextMessageOrdinal: 0x01020304,
	}
	second := renderedRow{
		tsNS:                    17,
		seq:                     7,
		line:                    "row-two",
		pairKind:                pairRenderBlock,
		profilerEndpointSlot:    profilerPairEndpointBlockRQIssue,
		profilerPublisherSlot:   profilerPairPublisherExactFtrace,
		profilerProvenanceFlags: profilerPairRowProvenanceStructured,
		profilerTraceClass:      profilerTraceClassStructuredKnown,
		profilerLaneID:          2,
	}
	return first, second
}

func profilerSourceOrderLeafAtOrdinal(t testing.TB, row renderedRow, ordinal uint64) [sha256.Size]byte {
	t.Helper()
	var proof profilerSourceOrderProof
	proof.activate()
	defer proof.reset()
	for prefixOrdinal := uint64(0); prefixOrdinal < ordinal; prefixOrdinal++ {
		prefix := renderedRow{
			tsNS: prefixOrdinal + 1,
			seq:  int(prefixOrdinal),
			line: "ordinal-prefix",
		}
		if err := proof.prepareRowContext(context.Background(), prefix, prefixOrdinal); err != nil {
			t.Fatalf("prepare source-order ordinal prefix %d: %v", prefixOrdinal, err)
		}
		proof.commitPreparedRow(prefix.profilerProvenance())
	}
	if err := proof.prepareRowContext(context.Background(), row, ordinal); err != nil {
		t.Fatalf("prepare source-order row at ordinal %d: %v", ordinal, err)
	}
	return proof.commitPreparedRow(row.profilerProvenance())
}

func profilerSourceOrderTerminalForRows(t testing.TB, rows ...renderedRow) [sha256.Size]byte {
	t.Helper()
	var proof profilerSourceOrderProof
	proof.activate()
	defer proof.reset()
	for index, row := range rows {
		ordinal := uint64(index)
		if err := proof.prepareRowContext(context.Background(), row, ordinal); err != nil {
			t.Fatalf("prepare source-order row %d: %v", index, err)
		}
		proof.commitPreparedRow(row.profilerProvenance())
	}
	digest, ok := proof.terminalDigest()
	if !ok {
		t.Fatalf("terminal source-order digest rejected for %d rows: %+v", len(rows), proof)
	}
	return digest
}

func TestProfilerSourceOrderProofCanonicalABIV2Golden(t *testing.T) {
	const (
		wantInitDomain     = "codrax/hitraceconv/profiler-source-order/init\x00"
		wantLeafDomain     = "codrax/hitraceconv/profiler-source-order/leaf\x00"
		wantStepDomain     = "codrax/hitraceconv/profiler-source-order/step\x00"
		wantTerminalDomain = "codrax/hitraceconv/profiler-source-order/terminal\x00"
	)
	if profilerSourceOrderProofVersion != 2 ||
		profilerSourceOrderProofInitDomain != wantInitDomain ||
		profilerSourceOrderProofLeafDomain != wantLeafDomain ||
		profilerSourceOrderProofStepDomain != wantStepDomain ||
		profilerSourceOrderProofTerminalDomain != wantTerminalDomain {
		t.Fatalf("source-order ABI domain/version drifted: version=%d init=%q leaf=%q step=%q terminal=%q",
			profilerSourceOrderProofVersion, profilerSourceOrderProofInitDomain,
			profilerSourceOrderProofLeafDomain, profilerSourceOrderProofStepDomain,
			profilerSourceOrderProofTerminalDomain)
	}

	wantState0 := profilerSourceOrderGoldenDigest(t,
		"6e9c6be2676bbf36ad74b3c4d1d43a7c6a4cb705ad57b23ac34caa9dba4d5783")
	wantTerminal0 := profilerSourceOrderGoldenDigest(t,
		"723f34a817d7cbd772f5b49d01388343f9cd4e19f767c60429a227a79a473170")
	wantLeaf0 := profilerSourceOrderGoldenDigest(t,
		"23cad388255677f71125842c92e951dc6d6e9411c329d65b054e7ba910308894")
	wantState1 := profilerSourceOrderGoldenDigest(t,
		"7ca80b18abffb965bebf088dc5f01331923433da98d270b39d001816d957ba57")
	wantTerminal1 := profilerSourceOrderGoldenDigest(t,
		"319094d3d29cf58a394cfd6f81660c86c341882a878402035415ba88f426bdee")
	wantLeaf1 := profilerSourceOrderGoldenDigest(t,
		"9e8f09e6c8e4492411753c0873ca6e0218cc147291929c3b998cd56d7b5505cc")
	wantState2 := profilerSourceOrderGoldenDigest(t,
		"6d4fbc6a030cf30eca76943635ce20a221bff5b930abf180128a273628368efb")
	wantTerminal2 := profilerSourceOrderGoldenDigest(t,
		"6c7ab574368ce4b2f5f14c2005a2a3bdda2a5a4da75b0921884440ec19e151b8")

	var proof profilerSourceOrderProof
	proof.activate()
	defer proof.reset()
	if proof.count != 0 || proof.state != wantState0 {
		t.Fatalf("source-order state0=(count=%d state=%x) want=(0 %x)",
			proof.count, proof.state, wantState0)
	}
	if got, ok := proof.terminalDigest(); !ok || got != wantTerminal0 {
		t.Fatalf("source-order terminal0=%x,%t want=%x,true", got, ok, wantTerminal0)
	}

	first, second := profilerSourceOrderGoldenRows()
	if err := proof.prepareRowContext(context.Background(), first, 0); err != nil {
		t.Fatal(err)
	}
	if got := proof.commitPreparedRow(first.profilerProvenance()); got != wantLeaf0 {
		t.Fatalf("source-order leaf0=%x want=%x", got, wantLeaf0)
	}
	if proof.count != 1 || proof.state != wantState1 || proof.prepared {
		t.Fatalf("source-order state1=(count=%d state=%x prepared=%t) want=(1 %x false)",
			proof.count, proof.state, proof.prepared, wantState1)
	}
	if got, ok := proof.terminalDigest(); !ok || got != wantTerminal1 {
		t.Fatalf("source-order terminal1=%x,%t want=%x,true", got, ok, wantTerminal1)
	}

	if err := proof.prepareRowContext(context.Background(), second, 1); err != nil {
		t.Fatal(err)
	}
	if got := proof.commitPreparedRow(second.profilerProvenance()); got != wantLeaf1 {
		t.Fatalf("source-order leaf1=%x want=%x", got, wantLeaf1)
	}
	if proof.count != 2 || proof.state != wantState2 || proof.prepared {
		t.Fatalf("source-order state2=(count=%d state=%x prepared=%t) want=(2 %x false)",
			proof.count, proof.state, proof.prepared, wantState2)
	}
	if got, ok := proof.terminalDigest(); !ok || got != wantTerminal2 {
		t.Fatalf("source-order terminal2=%x,%t want=%x,true", got, ok, wantTerminal2)
	}
}

func TestProfilerSourceOrderProofCanonicalFieldSensitivity(t *testing.T) {
	base, _ := profilerSourceOrderGoldenRows()
	baseLeaf := profilerSourceOrderLeafAtOrdinal(t, base, 0)
	tests := []struct {
		name    string
		ordinal uint64
		mutate  func(*renderedRow)
	}{
		{name: "ingest ordinal", ordinal: 1},
		{name: "timestamp", mutate: func(row *renderedRow) { row.tsNS++ }},
		{name: "sequence", mutate: func(row *renderedRow) { row.seq++ }},
		{name: "line bytes same length", mutate: func(row *renderedRow) { row.line = "A|\xce\xb2" }},
		{name: "line byte length", mutate: func(row *renderedRow) { row.line += "!" }},
		{name: "lane id", mutate: func(row *renderedRow) { row.profilerLaneID++ }},
		{name: "text message ordinal", mutate: func(row *renderedRow) { row.profilerTextMessageOrdinal++ }},
		{name: "pair kind", mutate: func(row *renderedRow) { row.pairKind++ }},
		{name: "endpoint slot", mutate: func(row *renderedRow) { row.profilerEndpointSlot++ }},
		{name: "publisher slot", mutate: func(row *renderedRow) { row.profilerPublisherSlot-- }},
		{name: "provenance flags", mutate: func(row *renderedRow) { row.profilerProvenanceFlags++ }},
		{name: "trace class", mutate: func(row *renderedRow) { row.profilerTraceClass++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			if got := profilerSourceOrderLeafAtOrdinal(t, candidate, test.ordinal); got == baseLeaf {
				t.Fatalf("canonical field mutation did not change leaf: row=%+v ordinal=%d leaf=%x",
					candidate, test.ordinal, got)
			}
		})
	}

	legacyMutations := []struct {
		name   string
		mutate func(*renderedRow)
	}{
		{name: "display lane", mutate: func(row *renderedRow) { row.pairLane = "legacy-lane" }},
		{name: "display table", mutate: func(row *renderedRow) { row.pairTable = "legacy-table" }},
		{name: "structured duplicate", mutate: func(row *renderedRow) { row.structuredPair = !row.structuredPair }},
		{name: "event field duplicate", mutate: func(row *renderedRow) { row.profilerEventField = 987654 }},
	}
	for _, test := range legacyMutations {
		t.Run("legacy excluded/"+test.name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate)
			if got := profilerSourceOrderLeafAtOrdinal(t, candidate, 0); got != baseLeaf {
				t.Fatalf("non-canonical legacy field entered source leaf: got=%x want=%x row=%+v",
					got, baseLeaf, candidate)
			}
		})
	}
}

func TestProfilerSourceOrderProofDistinguishesIngestFromTimestampOrder(t *testing.T) {
	first, second := profilerSourceOrderGoldenRows()
	if first.tsNS <= second.tsNS {
		t.Fatalf("golden rows no longer force timestamp order to reverse ingest order: first=%d second=%d",
			first.tsNS, second.tsNS)
	}
	wantAB := profilerSourceOrderGoldenDigest(t,
		"6c7ab574368ce4b2f5f14c2005a2a3bdda2a5a4da75b0921884440ec19e151b8")
	wantBA := profilerSourceOrderGoldenDigest(t,
		"c34de2bc2b9528a3d497649ea496b8c6225831046f574cbf2ee05258bdc2d7b6")
	ab := profilerSourceOrderTerminalForRows(t, first, second)
	ba := profilerSourceOrderTerminalForRows(t, second, first)
	if ab != wantAB || ba != wantBA || ab == ba {
		t.Fatalf("source-order terminal drifted or became commutative: AB=%x want=%x BA=%x want=%x",
			ab, wantAB, ba, wantBA)
	}

	publicationOrder := []renderedRow{first, second}
	sortRenderedRows(publicationOrder)
	if publicationOrder[0].line != second.line || publicationOrder[1].line != first.line {
		t.Fatalf("timestamp sorter did not reverse the source-order fixture: %+v", publicationOrder)
	}
	if got := profilerSourceOrderTerminalForRows(t, publicationOrder...); got != wantBA || got == ab {
		t.Fatalf("producer digest was derived from publication order: sorted=%x want=%x source=%x",
			got, wantBA, ab)
	}
}

func TestProfilerSourceOrderProofReusesWorkspaceWithoutPerRowAllocations(t *testing.T) {
	var proof profilerSourceOrderProof
	proof.activate()
	defer proof.reset()
	workspace := proof.workspace
	hasher := proof.hasher
	scratch := &proof.scratch[0]
	row := renderedRow{tsNS: 1, seq: 1, line: "allocation-pin"}
	ordinal := uint64(0)
	var proofErr error
	allocations := testing.AllocsPerRun(1_000, func() {
		if proofErr != nil {
			return
		}
		row.tsNS = ordinal + 1
		row.seq = int(ordinal)
		proofErr = proof.prepareRowContext(context.Background(), row, ordinal)
		if proofErr != nil {
			return
		}
		_ = proof.commitPreparedRow(row.profilerProvenance())
		ordinal++
	})
	if proofErr != nil {
		t.Fatal(proofErr)
	}
	if allocations != 0 || proof.workspace != workspace || proof.hasher != hasher ||
		&proof.scratch[0] != scratch || proof.count != ordinal {
		t.Fatalf("source proof per-row resources drifted: allocs=%.2f count=%d/%d workspace_same=%t hasher_same=%t scratch_same=%t",
			allocations, proof.count, ordinal, proof.workspace == workspace, proof.hasher == hasher,
			&proof.scratch[0] == scratch)
	}
}
