package hitraceconv

import (
	"archive/zip"
	"context"
	"errors"
)

const (
	inputRoutingZIPMaxShownMembers     = 64
	inputRoutingZIPMaxMemberNamesBytes = 16 << 10
)

// InputRoutingArchiveCandidates describes candidate discovery, not trace facts
// or an admitted member. CandidateCount covers the fully scanned safe directory;
// Members may be a bounded preview, but each shown name remains exact. A caller
// must never interpret a partial list as the complete choice set or choose its
// first item implicitly. Full preparation still validates the selected payload.
type InputRoutingArchiveCandidates struct {
	Members           []string `json:"members"`
	CandidateCount    int      `json:"candidate_count"`
	MembersComplete   bool     `json:"members_complete"`
	SelectionRequired bool     `json:"selection_required"`
}

func probeZIPRoutingCandidates(ctx context.Context, source conversionInputView, prefix []byte) (*InputRoutingArchiveCandidates, error) {
	if len(prefix) >= 4 && string(prefix[:4]) == "PK\x07\x08" {
		return nil, traceArchiveFailure(traceArchiveCodeMultiDisk, "", errors.New("spanned ZIP marker is unsupported"))
	}
	if source.Size() > traceArchiveZIPMaxArchiveBytes {
		return nil, traceArchiveFailure(traceArchiveCodeResourceLimit, "", errors.New("ZIP archive exceeds physical size limit"))
	}
	directory, err := preflightTraceArchiveZIP(ctx, source)
	if err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(source, source.Size())
	if err != nil {
		return nil, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", err)
	}
	budget := &traceArchiveZIPProbeBudget{remaining: traceArchiveZIPProbeTotalBytes, perMember: traceArchiveZIPProbeMemberBytes}
	candidates, err := discoverTraceArchiveZIPCandidates(ctx, reader, directory, "", budget)
	if err != nil {
		return nil, err
	}
	return boundedZIPRoutingCandidates(candidates), nil
}

func boundedZIPRoutingCandidates(candidates []traceArchiveZIPCandidate) *InputRoutingArchiveCandidates {
	result := &InputRoutingArchiveCandidates{
		Members: []string{}, CandidateCount: len(candidates),
		MembersComplete: true, SelectionRequired: len(candidates) > 1,
	}
	nameBytes := 0
	for _, candidate := range candidates {
		if len(result.Members) >= inputRoutingZIPMaxShownMembers || len(candidate.name) > inputRoutingZIPMaxMemberNamesBytes-nameBytes {
			result.MembersComplete = false
			continue
		}
		result.Members = append(result.Members, candidate.name)
		nameBytes += len(candidate.name)
	}
	return result
}

func zipRoutingProbeFailureReason(err error) string {
	var archiveError *TraceArchiveError
	if errors.As(err, &archiveError) {
		return archiveError.Code
	}
	return "archive_probe_unavailable"
}
