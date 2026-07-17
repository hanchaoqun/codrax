package hitraceconv

import (
	"crypto/sha256"
	"hash"
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
	if ledger == nil || lengthPrefix == nil || ledger.frames == math.MaxUint32 ||
		uint64(len(payload)) > math.MaxUint64-4 ||
		ledger.bodyBytes > math.MaxUint64-4-uint64(len(payload)) {
		return false
	}
	ledger.frames++
	if len(payload) == 0 {
		ledger.zeroFrames++
	}
	ledger.bodyBytes += 4 + uint64(len(payload))
	if ledger.bodyHasher != nil {
		_, _ = ledger.bodyHasher.Write(lengthPrefix[:])
		_, _ = ledger.bodyHasher.Write(payload)
	}
	return true
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
