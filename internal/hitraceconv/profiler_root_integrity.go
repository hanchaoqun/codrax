package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"math"
)

const profilerTraceVersionV1 = uint32(0x00010000)

const (
	profilerRootWriterProfileEmpty       = "empty_n0_empty_sha256"
	profilerRootWriterProfileSequential  = "sequential_2n_body_sha256"
	profilerRootWriterProfileFDRandom    = "fd_random_n_empty_sha256"
	profilerRootWriterProfileEmptyDigest = "empty_digest_pending_frame_count"
)

// profilerRootIntegrityLedger is the single authority for the two root
// TraceFile writer profiles proven by the pinned OpenHarmony producer. It is
// deliberately capture-local and immutable after validate: semantic decoders
// may consume a frame only after its physical envelope has been accounted
// here, but they cannot influence the profile decision.
type profilerRootIntegrityLedger struct {
	profile    string
	frames     uint64
	zeroFrames uint64
	bodyBytes  uint64
	bodyHasher hash.Hash
}

// profilerRootProfileProof binds one exact root TraceFile physical envelope to
// the held conversion input generation. It is produced before standalone
// layout discovery, so a payload-local magic cannot invent a root terminus.
// Semantic decoding may consume this proof, but it cannot change the physical
// writer-profile verdict.
type profilerRootProfileProof struct {
	Header           profilerTraceHeader
	BodyEnd          int64
	Frames           uint64
	BodyBytes        uint64
	WriterProfile    string
	EnvelopeVerified bool
	ProfileVerified  bool
	Failure          string
	FailureOffset    int64
	FailureDeclared  uint64
	FailureAvailable uint64
	FailureLimit     uint64
}

func newProfilerRootIntegrityLedger(header profilerTraceHeader) profilerRootIntegrityLedger {
	empty := sha256.Sum256(nil)
	if header.SHA256 == empty {
		return profilerRootIntegrityLedger{profile: profilerRootWriterProfileEmptyDigest}
	}
	return profilerRootIntegrityLedger{
		profile:    profilerRootWriterProfileSequential,
		bodyHasher: sha256.New(),
	}
}

func (ledger *profilerRootIntegrityLedger) observeFrame(lengthPrefix *[4]byte, payload []byte) bool {
	if !ledger.beginFrame(lengthPrefix, uint64(len(payload))) {
		return false
	}
	ledger.observePayload(payload)
	return true
}

func (ledger *profilerRootIntegrityLedger) beginFrame(lengthPrefix *[4]byte, payloadBytes uint64) bool {
	if ledger == nil || lengthPrefix == nil || ledger.frames == math.MaxUint32 ||
		payloadBytes > math.MaxUint64-4 || ledger.bodyBytes > math.MaxUint64-4-payloadBytes {
		return false
	}
	ledger.frames++
	if payloadBytes == 0 {
		ledger.zeroFrames++
	}
	ledger.bodyBytes += 4 + payloadBytes
	if ledger.bodyHasher != nil {
		_, _ = ledger.bodyHasher.Write(lengthPrefix[:])
	}
	return true
}

func (ledger *profilerRootIntegrityLedger) observePayload(payload []byte) {
	if ledger != nil && ledger.bodyHasher != nil && len(payload) > 0 {
		_, _ = ledger.bodyHasher.Write(payload)
	}
}

// validateProfilerRootProfileEnvelope is the sole pre-publication producer of
// a typed root terminus. It walks the exact L/V envelope with checked offsets,
// streams the sequential-writer digest through fixed scratch, and preserves
// the existing 64 MiB frame gate before any payload read or allocation.
func validateProfilerRootProfileEnvelope(ctx context.Context, reader io.ReaderAt,
	header profilerTraceHeader, bodyEnd int64, maxFrameBytes uint64,
) (profilerRootProfileProof, error) {
	proof := profilerRootProfileProof{Header: header, BodyEnd: bodyEnd}
	if ctx == nil {
		ctx = context.Background()
	}
	if reader == nil || maxFrameBytes == 0 {
		proof.Failure = "profiler_root_profile_unknown"
		return proof, nil
	}
	if failure := profilerRootHeaderFailure(header, bodyEnd); failure != "" {
		proof.Failure = failure
		return proof, nil
	}
	integrity := newProfilerRootIntegrityLedger(header)
	const scratchBytes = 256 * 1024
	scratch := make([]byte, scratchBytes)
	off := int64(profilerTraceHeaderSize)
	var prefix [4]byte
	for off < bodyEnd {
		if err := ctx.Err(); err != nil {
			return profilerRootProfileProof{}, err
		}
		remaining := bodyEnd - off
		if remaining < int64(len(prefix)) {
			proof.Failure = "profiler_root_length_prefix_truncated"
			proof.FailureOffset = off
			proof.FailureAvailable = uint64(remaining)
			return proof, nil
		}
		if _, err := io.ReadFull(io.NewSectionReader(reader, off, int64(len(prefix))), prefix[:]); err != nil {
			return profilerRootProfileProof{}, err
		}
		payloadBytes := uint64(binary.LittleEndian.Uint32(prefix[:]))
		physicalPayloadBytes := uint64(remaining - int64(len(prefix)))
		if payloadBytes > physicalPayloadBytes {
			proof.Failure = "profiler_root_frame_truncated"
			proof.FailureOffset = off
			proof.FailureDeclared = payloadBytes
			proof.FailureAvailable = physicalPayloadBytes
			return proof, nil
		}
		if payloadBytes > maxFrameBytes {
			proof.Failure = "plugin_frame_size_budget_exceeded"
			proof.FailureOffset = off
			proof.FailureDeclared = payloadBytes
			proof.FailureAvailable = physicalPayloadBytes
			proof.FailureLimit = maxFrameBytes
			return proof, nil
		}
		if !integrity.beginFrame(&prefix, payloadBytes) {
			proof.Failure = "container_counter_overflow"
			return proof, nil
		}
		payloadOff := off + int64(len(prefix))
		left := payloadBytes
		for left > 0 {
			if err := ctx.Err(); err != nil {
				return profilerRootProfileProof{}, err
			}
			chunk := uint64(len(scratch))
			if left < chunk {
				chunk = left
			}
			part := scratch[:int(chunk)]
			if _, err := io.ReadFull(io.NewSectionReader(reader, payloadOff, int64(chunk)), part); err != nil {
				return profilerRootProfileProof{}, err
			}
			integrity.observePayload(part)
			payloadOff += int64(chunk)
			left -= chunk
		}
		off += int64(len(prefix)) + int64(payloadBytes)
	}
	proof.EnvelopeVerified = off == bodyEnd
	proof.Frames = integrity.frames
	proof.BodyBytes = integrity.bodyBytes
	if !proof.EnvelopeVerified {
		proof.Failure = "profiler_root_frame_envelope_incomplete"
		return proof, nil
	}
	if failure := integrity.validate(header, bodyEnd); failure != "" {
		proof.Failure = failure
		proof.WriterProfile = integrity.profile
		return proof, nil
	}
	proof.ProfileVerified = true
	proof.WriterProfile = integrity.profile
	return proof, nil
}

func profilerRootHeaderFailure(header profilerTraceHeader, traceBodySize int64) string {
	if header.DataType != profilerDataTypeProtobuf {
		return "profiler_root_data_type_unsupported"
	}
	if header.Version != profilerTraceVersionV1 {
		return "profiler_root_version_unsupported"
	}
	if traceBodySize < profilerTraceHeaderSize || header.Length != uint64(traceBodySize) {
		return "profiler_root_declared_length_mismatch"
	}
	return ""
}

func (ledger *profilerRootIntegrityLedger) validate(header profilerTraceHeader, traceBodySize int64) string {
	if ledger == nil || traceBodySize < profilerTraceHeaderSize ||
		ledger.bodyBytes != uint64(traceBodySize-profilerTraceHeaderSize) {
		return "profiler_root_frame_envelope_incomplete"
	}
	switch ledger.profile {
	case profilerRootWriterProfileEmptyDigest:
		if ledger.frames == 0 {
			ledger.profile = profilerRootWriterProfileEmpty
			if header.Segments != 0 {
				return "profiler_root_segments_mismatch"
			}
			break
		}
		ledger.profile = profilerRootWriterProfileFDRandom
		if ledger.zeroFrames != 0 {
			return "profiler_root_fd_random_zero_frame_forbidden"
		}
		if ledger.frames > math.MaxUint32 || header.Segments != uint32(ledger.frames) {
			return "profiler_root_segments_mismatch"
		}
	case profilerRootWriterProfileSequential:
		if ledger.frames > math.MaxUint32/2 || header.Segments != uint32(ledger.frames*2) {
			return "profiler_root_segments_mismatch"
		}
		var observed [sha256.Size]byte
		copy(observed[:], ledger.bodyHasher.Sum(nil))
		if observed != header.SHA256 {
			return "profiler_root_payload_sha256_mismatch"
		}
	default:
		return "profiler_root_profile_unknown"
	}
	return ""
}

func profilerRootIntegrityFailure(reason string) bool {
	switch reason {
	case "profiler_root_data_type_unsupported",
		"profiler_offset_zero_data_type_unsupported",
		"profiler_root_version_unsupported",
		"profiler_root_declared_length_mismatch",
		"profiler_root_frame_truncated",
		"profiler_root_length_prefix_truncated",
		"profiler_root_frame_envelope_incomplete",
		"profiler_root_segments_mismatch",
		"profiler_root_payload_sha256_mismatch",
		"profiler_root_fd_random_zero_frame_forbidden",
		"profiler_root_profile_unknown":
		return true
	default:
		return false
	}
}
