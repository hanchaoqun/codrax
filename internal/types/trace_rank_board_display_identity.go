package types

import "strings"

// TraceRankBoardDisplayIdentity describes one query's ordinal domain for
// prompt presentation. Complete means identity-complete, not row-enumeration
// complete. An empty Key must not merge unrelated records/nodes into a board;
// callers retain those rows separately and disclose the missing identity.
// This is not a wire carrier, rank selector, or evidence admission rule.
type TraceRankBoardDisplayIdentity struct {
	Key                    string
	ArtifactKey            string
	ArtifactLabel          string
	ArtifactPath           string
	BoardTarget            string
	BoardParamsFingerprint string
	WindowStartTs          float64
	WindowEndTs            float64
	Complete               bool
}

func TraceRankBoardDisplayIdentityFromRecord(record ObservationRecord) TraceRankBoardDisplayIdentity {
	start, end, _ := TraceCausalProjectionSelectedWindowNote(record.RichNotes)
	node := TraceCausalProjectionNode{
		RankBoardTarget:            traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyRankBoardTarget),
		RankBoardParamsFingerprint: traceCausalProjectionRichNoteValue(record.RichNotes, TraceNoteKeyRankBoardParams),
		QueryWindowStartTs:         start,
		QueryWindowEndTs:           end,
	}
	return traceRankBoardDisplayIdentity(record, node)
}

func TraceRankBoardDisplayIdentityFromNode(projection TraceCausalProjection, node TraceCausalProjectionNode) TraceRankBoardDisplayIdentity {
	// These are the partition compiler's typed capture/path or bare-ID lanes,
	// not a basename guessed from a node label or impact-window locator.
	record := ObservationRecord{SourceRef: ObservationSourceRef{
		CaptureIdentityPath: projection.ArtifactPath,
		ArtifactID:          projection.ArtifactLabel,
	}}
	return traceRankBoardDisplayIdentity(record, node)
}

func traceRankBoardDisplayIdentity(record ObservationRecord, node TraceCausalProjectionNode) TraceRankBoardDisplayIdentity {
	artifactKey, label, path := traceCausalProjectionArtifactIdentity(record)
	start, end := node.RankQueryWindowStartTs, node.RankQueryWindowEndTs
	if !traceCausalProjectionIntervalValid(start, end) {
		start, end = node.QueryWindowStartTs, node.QueryWindowEndTs
	}
	identity := TraceRankBoardDisplayIdentity{
		ArtifactKey: artifactKey, ArtifactLabel: label, ArtifactPath: path,
		BoardTarget:            strings.TrimSpace(node.RankBoardTarget),
		BoardParamsFingerprint: strings.TrimSpace(node.RankBoardParamsFingerprint),
		WindowStartTs:          start, WindowEndTs: end,
	}
	identity.Complete = artifactKey != "" && identity.BoardTarget != "" &&
		identity.BoardParamsFingerprint != "" && traceCausalProjectionIntervalValid(start, end)
	if identity.Complete {
		identity.Key = artifactKey + "\x00" + traceCausalProjectionRankBoardIdentityKey(node)
	}
	return identity
}
