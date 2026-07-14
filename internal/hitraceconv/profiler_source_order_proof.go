package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

const (
	profilerSourceOrderProofVersion uint16 = 1

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
	provenance [12]byte
	leaf       [sha256.Size]byte
	step       [profilerSourceOrderProofStepBytes]byte
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
// tail, so the fixed 12-byte typed provenance is appended by commitPreparedRow.
// An error never changes count or the committed rolling state.
func (proof *profilerSourceOrderProof) prepareRowContext(ctx context.Context, row renderedRow, ordinal uint64) error {
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
	if row.seq < 0 {
		return &traceDBOutputInvariantError{Reason: "profiler_source_order_proof_sequence_invalid"}
	}

	proof.hasher.Reset()
	prefix := &proof.workspace.leafPrefix
	offset := copy(prefix[:], profilerSourceOrderProofLeafDomain)
	binary.LittleEndian.PutUint16(prefix[offset:], profilerSourceOrderProofVersion)
	offset += 2
	binary.LittleEndian.PutUint64(prefix[offset:], ordinal)
	offset += 8
	binary.LittleEndian.PutUint64(prefix[offset:], row.tsNS)
	offset += 8
	binary.LittleEndian.PutUint64(prefix[offset:], uint64(row.seq))
	offset += 8
	binary.LittleEndian.PutUint64(prefix[offset:], uint64(len(row.line)))
	_, _ = proof.hasher.Write(prefix[:]) // crypto hashes never return a write error.

	processed := uint64(0)
	for start := 0; start < len(row.line); {
		end := min(start+len(proof.scratch), len(row.line))
		if err := profilerByteContextCheckpoint(ctx, &processed, uint64(end-start)); err != nil {
			proof.hasher.Reset()
			return err
		}
		chunk := proof.scratch[:end-start]
		copy(chunk, row.line[start:end])
		_, _ = proof.hasher.Write(chunk) // crypto hashes never return a write error.
		start = end
	}
	if err := ctx.Err(); err != nil {
		proof.hasher.Reset()
		return err
	}
	proof.prepared = true
	return nil
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
	encodedProvenance := &workspace.provenance
	binary.LittleEndian.PutUint32(encodedProvenance[0:4], provenance.LaneID)
	binary.LittleEndian.PutUint32(encodedProvenance[4:8], provenance.TextMessageOrdinal)
	encodedProvenance[8] = byte(provenance.PairKind)
	encodedProvenance[9] = byte(provenance.EndpointSlot)
	encodedProvenance[10] = byte(provenance.PublisherSlot)
	encodedProvenance[11] = byte(provenance.Flags)
	_, _ = proof.hasher.Write(encodedProvenance[:]) // crypto hashes never return a write error.
	proof.hasher.Sum(workspace.leaf[:0])

	step := &workspace.step
	offset := copy(step[:], profilerSourceOrderProofStepDomain)
	binary.LittleEndian.PutUint16(step[offset:], profilerSourceOrderProofVersion)
	offset += 2
	copy(step[offset:], proof.state[:])
	offset += sha256.Size
	binary.LittleEndian.PutUint64(step[offset:], proof.count)
	offset += 8
	copy(step[offset:], workspace.leaf[:])
	proof.state = sha256.Sum256(step[:])
	proof.count++
	proof.prepared = false
	proof.hasher.Reset()
	return workspace.leaf
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
	var input [len(profilerSourceOrderProofTerminalDomain) + 2 + 8 + sha256.Size]byte
	offset := copy(input[:], profilerSourceOrderProofTerminalDomain)
	binary.LittleEndian.PutUint16(input[offset:], profilerSourceOrderProofVersion)
	offset += 2
	binary.LittleEndian.PutUint64(input[offset:], proof.count)
	offset += 8
	copy(input[offset:], proof.state[:])
	return sha256.Sum256(input[:]), true
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
