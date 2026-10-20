package hitraceconv

import (
	"context"
	"errors"
	"io"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

const ArchiveTextTransportProfile = "zip_trace_text_v1"

// ArchiveTextTransportResult is byte transport only. Archive selection and
// complete text publication retain separate measured coordinates; neither is
// a semantic producer, event count, clock conversion or causal claim.
type ArchiveTextTransportResult struct {
	Profile           string                 `json:"profile"`
	SourcePath        string                 `json:"source_path"`
	SourceGeneration  string                 `json:"source_generation"`
	Archive           TraceArchiveProvenance `json:"archive"`
	DecodedPath       string                 `json:"decoded_path"`
	DecodedBytes      int64                  `json:"decoded_bytes"`
	DecodedSHA256     string                 `json:"decoded_sha256"`
	DecodedGeneration string                 `json:"decoded_generation"`
}

// ZIP namespace/size/CRC checks have already completed before this runs.
// Keep existing binary routing intact. Legacy suffix candidates do not make
// arbitrary UTF-8 a trace: text must also have the shared physical row prefix.
func admitArchiveTraceText(ctx context.Context, member *traceArchiveMemberInput) (bool, error) {
	prefix := make([]byte, min(member.Size(), int64(traceRoutingTextProbeBytes)))
	if _, err := io.ReadFull(io.NewSectionReader(member, 0, int64(len(prefix))), prefix); err != nil {
		return false, err
	}
	if !traceRoutingTextPrefix(prefix) {
		return false, nil
	}
	if err := attachment.ValidateTextReaderAtFull(ctx, attachment.KindTrace, member.DisplayPath(), member, member.Size(), attachment.TracePhysicalLineMaxBytes); err != nil {
		return false, traceArchiveFailure(traceArchiveCodeMemberIntegrity, member.member, err)
	}
	return true, member.Validate(conversionInputStageArchiveIntake)
}

func publishArchiveInputText(ctx context.Context, opts Options, route *traceConversionInput, ledger *conversionFileLedger) (ArchiveTextTransportResult, error) {
	if route == nil || !route.archiveText || route.archive == nil || route.member == nil || route.provenance == nil ||
		route.input != route.member || route.member.receipt != *route.provenance ||
		ledger == nil || ledger.archive == nil || *ledger.archive != *route.provenance {
		return ArchiveTextTransportResult{}, errors.New("archive text publication lacks its held member receipt")
	}
	receipt := route.member.receipt
	published, err := publishHeldTraceText(ctx, opts, route.member, receipt.MemberBytes, receipt.MemberSHA256, ledger)
	if err != nil {
		return ArchiveTextTransportResult{}, err
	}
	return ArchiveTextTransportResult{
		Profile: ArchiveTextTransportProfile, SourcePath: route.archive.DisplayPath(),
		SourceGeneration: route.archive.identity.CacheToken(), Archive: receipt,
		DecodedPath: published.path, DecodedBytes: published.bytes,
		DecodedSHA256: published.sha256, DecodedGeneration: published.generation,
	}, nil
}
