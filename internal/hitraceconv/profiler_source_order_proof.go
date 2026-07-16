package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

const (
	profilerSourceOrderProofVersion uint16 = 2

	profilerSourceOrderProofInitDomain     = "codrax/hitraceconv/profiler-source-order/init\x00"
	profilerSourceOrderProofLeafDomain     = "codrax/hitraceconv/profiler-source-order/leaf\x00"
	profilerSourceOrderProofStepDomain     = "codrax/hitraceconv/profiler-source-order/step\x00"
	profilerSourceOrderProofTerminalDomain = "codrax/hitraceconv/profiler-source-order/terminal\x00"

	profilerSourceOrderProofLeafPrefixBytes = len(profilerSourceOrderProofLeafDomain) + 2 + 8*4
	profilerSourceOrderProofStepBytes       = len(profilerSourceOrderProofStepDomain) +
		2 + sha256.Size + 8 + sha256.Size
)

type profilerSourceOrderProofWorkspace struct {
	scratch    [profilerContextByteCheckpointBytes]byte
	leafPrefix [profilerSourceOrderProofLeafPrefixBytes]byte
	provenance [13]byte
	leaf       [sha256.Size]byte
	step       [profilerSourceOrderProofStepBytes]byte
}

// profilerSourceOrderLeafBuilder recomputes canonical leaves from an
// authenticated sorter row without carrying source-order state. The final run
// is publication-sorted rather than ingest-sorted, so B-c must scatter these
// leaves by ingest ordinal before replaying the rolling proof.
type profilerSourceOrderLeafBuilder struct {
	hasher    hash.Hash
	workspace *profilerSourceOrderProofWorkspace
	scratch   []byte
}

func newProfilerSourceOrderLeafBuilder() *profilerSourceOrderLeafBuilder {
	workspace := new(profilerSourceOrderProofWorkspace)
	return &profilerSourceOrderLeafBuilder{
		hasher: sha256.New(), workspace: workspace, scratch: workspace.scratch[:],
	}
}

// profilerSourceOrderProof is the producer-side authority for the exact order
// in which an open Profiler capture accepts rows. It deliberately retains only
// a fixed state and one reusable 64 KiB hashing buffer. The authenticated
// sorter run remains ordered for publication; B-c reorders that run through a
// fixed sidecar and compares its independently recomputed terminal digest with
// this producer value.
type profilerSourceOrderProof struct {
	active        bool
	prepared      bool
	frozen        bool
	retired       bool
	count         uint64
	state         [sha256.Size]byte
	expectedCount uint64
	expectedRoot  [sha256.Size]byte
	hasher        hash.Hash
	workspace     *profilerSourceOrderProofWorkspace
	scratch       []byte
}

func profilerSourceOrderInitialState() [sha256.Size]byte {
	var input [len(profilerSourceOrderProofInitDomain) + 2]byte
	offset := copy(input[:], profilerSourceOrderProofInitDomain)
	binary.LittleEndian.PutUint16(input[offset:], profilerSourceOrderProofVersion)
	return sha256.Sum256(input[:])
}

func (proof *profilerSourceOrderProof) pristine() bool {
	if proof == nil {
		return true
	}
	return !proof.active && !proof.prepared && !proof.frozen && !proof.retired && proof.count == 0 &&
		proof.state == [sha256.Size]byte{} && proof.expectedCount == 0 &&
		proof.expectedRoot == [sha256.Size]byte{} && proof.hasher == nil &&
		proof.workspace == nil && proof.scratch == nil
}

func (proof *profilerSourceOrderProof) activate() {
	proof.active = true
	proof.prepared = false
	proof.frozen = false
	proof.retired = false
	proof.count = 0
	proof.state = profilerSourceOrderInitialState()
	proof.expectedCount = 0
	proof.expectedRoot = [sha256.Size]byte{}
	proof.hasher = sha256.New()
	proof.workspace = new(profilerSourceOrderProofWorkspace)
	proof.scratch = proof.workspace.scratch[:]
}

// reset is reserved for an unpublished verifier/builder instance. A live
// sink uses retire during cleanup so its fixed expected snapshot cannot be
// mistaken for a pristine, reusable capture.
func (proof *profilerSourceOrderProof) reset() {
	if proof == nil {
		return
	}
	*proof = profilerSourceOrderProof{}
}

func (proof *profilerSourceOrderProof) retire() {
	if proof == nil || !proof.active || proof.retired {
		return
	}
	if !proof.frozen {
		root, ok := proof.terminalDigest()
		if ok {
			proof.expectedCount = proof.count
			proof.expectedRoot = root
			proof.frozen = true
		}
	}
	proof.prepared = false
	proof.retired = true
	proof.hasher = nil
	proof.workspace = nil
	proof.scratch = nil
}

func (proof *profilerSourceOrderProof) validWorkspace() bool {
	return proof != nil && proof.workspace != nil &&
		len(proof.scratch) == profilerContextByteCheckpointBytes &&
		&proof.scratch[0] == &proof.workspace.scratch[0]
}

func (proof *profilerSourceOrderProof) validMutableState() bool {
	if proof == nil || !proof.active || proof.prepared || proof.retired || proof.hasher == nil ||
		!proof.validWorkspace() {
		return false
	}
	if proof.count == 0 && proof.state != profilerSourceOrderInitialState() {
		return false
	}
	if !proof.frozen {
		return proof.expectedCount == 0 && proof.expectedRoot == [sha256.Size]byte{}
	}
	root, ok := proof.terminalDigestUnchecked()
	return ok && proof.expectedCount == proof.count && proof.expectedRoot == root
}

// prepareRowContext hashes every variable-width input before addContext's
// final cancellation poll. The lane ID is not known until the no-fail commit
// tail, so the fixed 13-byte typed provenance encoding is appended by
// commitPreparedRow.
// An error never changes count or the committed rolling state.
func (proof *profilerSourceOrderProof) prepareRowContext(ctx context.Context, row renderedRow, ordinal uint64) error {
	return proof.prepareStoredScalarsContext(ctx, row.tsNS, row.seq, row.line, ordinal)
}

func (proof *profilerSourceOrderProof) prepareStoredRowContext(
	ctx context.Context,
	row traceDBStoredRow,
	ordinal uint64,
) error {
	return proof.prepareStoredScalarsContext(ctx, row.tsNS, row.seq, row.line, ordinal)
}

func (proof *profilerSourceOrderProof) prepareStoredScalarsContext(
	ctx context.Context,
	tsNS uint64,
	seq int,
	line string,
	ordinal uint64,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if proof == nil || !proof.active || proof.prepared || proof.frozen || proof.retired || proof.hasher == nil ||
		!proof.validWorkspace() || proof.count != ordinal {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_state_invalid"}
	}
	if err := prepareProfilerSourceOrderLeafContext(
		ctx, proof.hasher, proof.workspace, proof.scratch, tsNS, seq, line, ordinal,
	); err != nil {
		return err
	}
	proof.prepared = true
	return nil
}

func prepareProfilerSourceOrderLeafContext(
	ctx context.Context,
	hasher hash.Hash,
	workspace *profilerSourceOrderProofWorkspace,
	scratch []byte,
	tsNS uint64,
	seq int,
	line string,
	ordinal uint64,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if hasher == nil || workspace == nil || len(scratch) != profilerContextByteCheckpointBytes ||
		&scratch[0] != &workspace.scratch[0] {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_leaf_workspace_invalid"}
	}
	if seq < 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_sequence_invalid"}
	}

	hasher.Reset()
	prefix := &workspace.leafPrefix
	offset := copy(prefix[:], profilerSourceOrderProofLeafDomain)
	binary.LittleEndian.PutUint16(prefix[offset:], profilerSourceOrderProofVersion)
	offset += 2
	binary.LittleEndian.PutUint64(prefix[offset:], ordinal)
	offset += 8
	binary.LittleEndian.PutUint64(prefix[offset:], tsNS)
	offset += 8
	binary.LittleEndian.PutUint64(prefix[offset:], uint64(seq))
	offset += 8
	binary.LittleEndian.PutUint64(prefix[offset:], uint64(len(line)))
	_, _ = hasher.Write(prefix[:]) // crypto hashes never return a write error.

	processed := uint64(0)
	for start := 0; start < len(line); {
		end := min(start+len(scratch), len(line))
		if err := profilerByteContextCheckpoint(ctx, &processed, uint64(end-start)); err != nil {
			hasher.Reset()
			return err
		}
		chunk := scratch[:end-start]
		copy(chunk, line[start:end])
		_, _ = hasher.Write(chunk) // crypto hashes never return a write error.
		start = end
	}
	if err := ctx.Err(); err != nil {
		hasher.Reset()
		return err
	}
	return nil
}

func finishProfilerSourceOrderLeaf(
	hasher hash.Hash,
	workspace *profilerSourceOrderProofWorkspace,
	provenance profilerPairRowProvenance,
) [sha256.Size]byte {
	encodedProvenance := &workspace.provenance
	binary.LittleEndian.PutUint32(encodedProvenance[0:4], provenance.LaneID)
	binary.LittleEndian.PutUint32(encodedProvenance[4:8], provenance.TextMessageOrdinal)
	encodedProvenance[8] = byte(provenance.PairKind)
	encodedProvenance[9] = byte(provenance.EndpointSlot)
	encodedProvenance[10] = byte(provenance.PublisherSlot)
	encodedProvenance[11] = byte(provenance.Flags)
	encodedProvenance[12] = byte(provenance.TraceClass)
	_, _ = hasher.Write(encodedProvenance[:]) // crypto hashes never return a write error.
	hasher.Sum(workspace.leaf[:0])
	return workspace.leaf
}

func (builder *profilerSourceOrderLeafBuilder) leafContext(
	ctx context.Context,
	row traceDBStoredRow,
	ordinal uint64,
) ([sha256.Size]byte, error) {
	if builder == nil || builder.hasher == nil || builder.workspace == nil {
		return [sha256.Size]byte{}, &traceDBOutputInvariantError{Reason: "profiler_source_order_leaf_builder_invalid"}
	}
	if err := prepareProfilerSourceOrderLeafContext(
		ctx, builder.hasher, builder.workspace, builder.scratch,
		row.tsNS, row.seq, row.line, ordinal,
	); err != nil {
		return [sha256.Size]byte{}, err
	}
	leaf := finishProfilerSourceOrderLeaf(builder.hasher, builder.workspace, row.profilerProvenance())
	builder.hasher.Reset()
	return leaf, nil
}

func advanceProfilerSourceOrderState(
	state [sha256.Size]byte,
	ordinal uint64,
	leaf [sha256.Size]byte,
	step *[profilerSourceOrderProofStepBytes]byte,
) [sha256.Size]byte {
	offset := copy(step[:], profilerSourceOrderProofStepDomain)
	binary.LittleEndian.PutUint16(step[offset:], profilerSourceOrderProofVersion)
	offset += 2
	copy(step[offset:], state[:])
	offset += sha256.Size
	binary.LittleEndian.PutUint64(step[offset:], ordinal)
	offset += 8
	copy(step[offset:], leaf[:])
	return sha256.Sum256(step[:])
}

func terminalProfilerSourceOrderDigest(count uint64, state [sha256.Size]byte) [sha256.Size]byte {
	var input [len(profilerSourceOrderProofTerminalDomain) + 2 + 8 + sha256.Size]byte
	offset := copy(input[:], profilerSourceOrderProofTerminalDomain)
	binary.LittleEndian.PutUint16(input[offset:], profilerSourceOrderProofVersion)
	offset += 2
	binary.LittleEndian.PutUint64(input[offset:], count)
	offset += 8
	copy(input[offset:], state[:])
	return sha256.Sum256(input[:])
}

func (proof *profilerSourceOrderProof) abortPreparedRow() {
	if proof == nil || !proof.prepared {
		return
	}
	proof.prepared = false
	proof.hasher.Reset()
}

// commitPreparedRow performs only fixed-width, non-failing work. Callers must
// invoke it in the same no-fail tail that stores the row and ingest ordinal.
// The returned leaf is useful to B-c and to ABI tests; it is never retained.
func (proof *profilerSourceOrderProof) commitPreparedRow(provenance profilerPairRowProvenance) [sha256.Size]byte {
	workspace := proof.workspace
	leaf := finishProfilerSourceOrderLeaf(proof.hasher, workspace, provenance)
	proof.state = advanceProfilerSourceOrderState(proof.state, proof.count, leaf, &workspace.step)
	proof.count++
	proof.prepared = false
	proof.hasher.Reset()
	return leaf
}

func (proof *profilerSourceOrderProof) terminalDigest() ([sha256.Size]byte, bool) {
	if proof == nil || !proof.active || proof.prepared || proof.retired || proof.hasher == nil ||
		!proof.validWorkspace() ||
		(proof.count == 0 && proof.state != profilerSourceOrderInitialState()) {
		return [sha256.Size]byte{}, false
	}
	return proof.terminalDigestUnchecked()
}

func (proof *profilerSourceOrderProof) terminalDigestUnchecked() ([sha256.Size]byte, bool) {
	if proof == nil {
		return [sha256.Size]byte{}, false
	}
	return terminalProfilerSourceOrderDigest(proof.count, proof.state), true
}

func (proof *profilerSourceOrderProof) freezeExpected() error {
	if proof == nil || !proof.validMutableState() {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_state_invalid"}
	}
	if proof.frozen {
		return nil
	}
	root, ok := proof.terminalDigest()
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_terminal_invalid"}
	}
	proof.expectedCount = proof.count
	proof.expectedRoot = root
	proof.frozen = true
	return nil
}

func (s *traceDBRowSink) validateProfilerSourceOrderProof() error {
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_sink_missing"}
	}
	if s.captureLifecycle == profilerCaptureInactive {
		if !s.profilerSourceProof.pristine() {
			return &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_inactive_state_invalid"}
		}
		return nil
	}
	if !s.profilerSourceProof.validMutableState() || s.stats.RowsAccepted < 0 ||
		s.profilerSourceProof.count != s.nextIngestOrdinal ||
		s.profilerSourceProof.count != uint64(s.stats.RowsAccepted) {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_state_invalid"}
	}
	return nil
}

func (s *traceDBRowSink) expectedProfilerSourceOrderProof() (uint64, [sha256.Size]byte, error) {
	if err := s.validateProfilerSourceOrderProof(); err != nil {
		return 0, [sha256.Size]byte{}, err
	}
	if s.captureLifecycle == profilerCaptureInactive {
		return 0, [sha256.Size]byte{}, &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_inactive"}
	}
	if !s.profilerSourceProof.frozen {
		return 0, [sha256.Size]byte{}, &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_not_frozen"}
	}
	return s.profilerSourceProof.expectedCount, s.profilerSourceProof.expectedRoot, nil
}
