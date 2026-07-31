package types

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// TraceIPCSyncRequest is one exact synchronous IPC request row. Its
// send/receive interval is transport timing, not target blocking wall clock.
type TraceIPCSyncRequest struct {
	TransactionID  int
	Peer           string
	SendTs         float64
	ReceiveTs      float64
	Flags          string
	FlagsKnown     bool
	Code           string
	CodeKnown      bool
	ReceiverSource string
	RecordID       string
}

// TraceIPCRequestCensusAuthority keeps IPC request counts and native request
// fields separate from target blocking-occurrence counts. It is an
// answer-writing authority only.
type TraceIPCRequestCensusAuthority struct {
	ArtifactLabel   string
	SelectedWindow  string
	Subject         string
	CoverageStatus  string
	TotalRequests   int
	SyncRequests    int
	OnewayRequests  int
	UnknownRequests int
	SyncRoster      []TraceIPCSyncRequest
}

type traceIPCRequestCensusKey struct {
	artifact string
	window   string
	subject  string
}

// BuildTraceIPCRequestCensusAuthorities consumes only deterministic typed
// ipc_request_census / ipc_request_edge records for explicit runtime targets.
// Counts must partition exactly. A complete census is downgraded when its
// synchronous-row roster is not complete, rather than filling fields from a
// blocking occurrence or narrative.
func BuildTraceIPCRequestCensusAuthorities(ledger ObservationLedger, rm *RequestModel) []TraceIPCRequestCensusAuthority {
	if rm == nil || len(rm.RuntimeTargets) == 0 {
		return nil
	}
	sets := map[traceIPCRequestCensusKey]ObservationRecord{}
	rows := map[traceIPCRequestCensusKey][]ObservationRecord{}
	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			record.GroundingPolicy != ClaimGroundingHard ||
			!ObservationRecordMatchesUserRuntimeTarget(record, rm) {
			continue
		}
		key, ok := traceIPCRequestCensusRecordKey(record)
		if !ok {
			continue
		}
		switch strings.TrimSpace(record.Predicate) {
		case "ipc_request_census":
			sets[key] = record
		case "ipc_request_edge":
			rows[key] = append(rows[key], record)
		}
	}

	keys := make([]traceIPCRequestCensusKey, 0, len(sets))
	for key := range sets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].artifact != keys[j].artifact {
			return keys[i].artifact < keys[j].artifact
		}
		if keys[i].window != keys[j].window {
			return keys[i].window < keys[j].window
		}
		return keys[i].subject < keys[j].subject
	})

	out := make([]TraceIPCRequestCensusAuthority, 0, len(keys))
	for _, key := range keys {
		set := sets[key]
		total, totalOK := traceIPCRequestCensusInt(set.Value)
		syncCount, syncOK := traceIPCRequestCensusNoteInt(set.RichNotes, TraceNoteKeyIPCSyncRequestCount)
		onewayCount, onewayOK := traceIPCRequestCensusNoteInt(set.RichNotes, TraceNoteKeyIPCOnewayRequestCount)
		unknownCount, unknownOK := traceIPCRequestCensusNoteInt(set.RichNotes, TraceNoteKeyIPCUnknownRequestCount)
		status := strings.TrimSpace(traceObservationRichNoteValue(set.RichNotes, TraceNoteKeyIPCRequestCensusStatus))
		if !totalOK || !syncOK || !onewayOK || !unknownOK || total < 0 ||
			syncCount < 0 || onewayCount < 0 || unknownCount < 0 ||
			syncCount+onewayCount+unknownCount != total ||
			(status != "complete" && status != "lower_bound_capacity_truncated") {
			continue
		}
		authority := TraceIPCRequestCensusAuthority{
			ArtifactLabel:   key.artifact,
			SelectedWindow:  key.window,
			Subject:         key.subject,
			CoverageStatus:  status,
			TotalRequests:   total,
			SyncRequests:    syncCount,
			OnewayRequests:  onewayCount,
			UnknownRequests: unknownCount,
		}
		seenTransactions := map[int]bool{}
		for _, row := range rows[key] {
			if !strings.EqualFold(strings.TrimSpace(traceObservationRichNoteValue(row.RichNotes, TraceNoteKeyIPCCallSemantics)), "sync_request") {
				continue
			}
			transactionID, ok := traceIPCRequestCensusNoteInt(row.RichNotes, TraceNoteKeyIPCTransactionID)
			if !ok || transactionID <= 0 || seenTransactions[transactionID] {
				continue
			}
			sendTs, receiveTs := row.Span.StartTs, row.Span.EndTs
			if sendTs <= 0 || receiveTs < sendTs || math.IsNaN(sendTs) || math.IsNaN(receiveTs) ||
				math.IsInf(sendTs, 0) || math.IsInf(receiveTs, 0) {
				continue
			}
			seenTransactions[transactionID] = true
			authority.SyncRoster = append(authority.SyncRoster, TraceIPCSyncRequest{
				TransactionID:  transactionID,
				Peer:           strings.TrimSpace(row.Object),
				SendTs:         sendTs,
				ReceiveTs:      receiveTs,
				Flags:          strings.TrimSpace(traceObservationRichNoteValue(row.RichNotes, TraceNoteKeyIPCFlags)),
				FlagsKnown:     traceObservationRichNoteBool(row.RichNotes, TraceNoteKeyIPCFlagsKnown),
				Code:           strings.TrimSpace(traceObservationRichNoteValue(row.RichNotes, TraceNoteKeyIPCCode)),
				CodeKnown:      traceObservationRichNoteBool(row.RichNotes, TraceNoteKeyIPCCodeKnown),
				ReceiverSource: strings.TrimSpace(traceObservationRichNoteValue(row.RichNotes, TraceNoteKeyIPCReceiverSource)),
				RecordID:       strings.TrimSpace(row.ID),
			})
		}
		sort.Slice(authority.SyncRoster, func(i, j int) bool {
			if authority.SyncRoster[i].SendTs != authority.SyncRoster[j].SendTs {
				return authority.SyncRoster[i].SendTs < authority.SyncRoster[j].SendTs
			}
			return authority.SyncRoster[i].TransactionID < authority.SyncRoster[j].TransactionID
		})
		if len(authority.SyncRoster) != authority.SyncRequests {
			authority.CoverageStatus = "counts_complete_sync_roster_incomplete"
			if status != "complete" {
				authority.CoverageStatus = "lower_bound_sync_roster_incomplete"
			}
		}
		out = append(out, authority)
	}
	return out
}

func traceIPCRequestCensusRecordKey(record ObservationRecord) (traceIPCRequestCensusKey, bool) {
	artifact := strings.TrimSpace(record.SourceRef.ArtifactID)
	if artifact == "" {
		artifact = strings.TrimSpace(record.SourceRef.Path)
	}
	window := strings.TrimSpace(traceObservationRichNoteValue(record.RichNotes, TraceNoteKeySelectedWindow))
	subject := strings.TrimSpace(record.Subject)
	if artifact == "" || window == "" || subject == "" {
		return traceIPCRequestCensusKey{}, false
	}
	return traceIPCRequestCensusKey{artifact: artifact, window: window, subject: subject}, true
}

func traceIPCRequestCensusInt(raw string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	return value, err == nil
}

func traceIPCRequestCensusNoteInt(notes []string, key string) (int, bool) {
	return traceIPCRequestCensusInt(traceObservationRichNoteValue(notes, key))
}
