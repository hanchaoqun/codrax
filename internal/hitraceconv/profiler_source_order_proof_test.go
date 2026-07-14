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

func TestProfilerSourceOrderProofCanonicalABIV1Golden(t *testing.T) {
	const (
		wantInitDomain     = "codrax/hitraceconv/profiler-source-order/init\x00"
		wantLeafDomain     = "codrax/hitraceconv/profiler-source-order/leaf\x00"
		wantStepDomain     = "codrax/hitraceconv/profiler-source-order/step\x00"
		wantTerminalDomain = "codrax/hitraceconv/profiler-source-order/terminal\x00"
	)
	if profilerSourceOrderProofVersion != 1 ||
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
		"2c6c6806f23b4904616e23868ae1bcc1632221ecd4fc4dbf6474f845ac7904b7")
	wantTerminal0 := profilerSourceOrderGoldenDigest(t,
		"8b2a39d25e4ab15fb311e5a39ede0aec63229adc4ca4288c9fee08b228d4df5b")
	wantLeaf0 := profilerSourceOrderGoldenDigest(t,
		"9194c0b0d0c17fa23722219f1a7b777d4c766186eae764424a848ba581d79a6d")
	wantState1 := profilerSourceOrderGoldenDigest(t,
		"ba377150800ca178dcd55b27ade6ac875614b40988e6d333c3239f0b1ec58549")
	wantTerminal1 := profilerSourceOrderGoldenDigest(t,
		"efa4d2051e3e424c0f4d9c455b4cc205df5822636c99b7bcaf74ed805b3ef98e")
	wantLeaf1 := profilerSourceOrderGoldenDigest(t,
		"6268c0644bb76e6933cef35374b7c48257540056f1952b37ad0969184027ed9b")
	wantState2 := profilerSourceOrderGoldenDigest(t,
		"cdfa4b0ec2ee425646e73243fa668467bee25aacd476e10b7b6bf9575dffb571")
	wantTerminal2 := profilerSourceOrderGoldenDigest(t,
		"ad603e81f2126d5be37ad1d18e9daf230c33bec8fd618d5bde0bc139fb916c2d")

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
		"ad603e81f2126d5be37ad1d18e9daf230c33bec8fd618d5bde0bc139fb916c2d")
	wantBA := profilerSourceOrderGoldenDigest(t,
		"0691274d948bbb3a604d722b379febd477badd9b1ba31a4e115fb01eabb8925b")
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
