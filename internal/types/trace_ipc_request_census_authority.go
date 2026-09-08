package types

import (
	"encoding/json"
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
	// ArtifactKey is an exact capture identity, never a human-facing label.
	ArtifactKey   string
	ArtifactLabel string
	// SourceRecordID identifies the census whose own result rows were used.
	SourceRecordID  string
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

type traceIPCResultSource struct {
	kind                         ObservationSourceKind
	producer, path, payload, raw string
}

type traceIPCRequestCohortKey struct {
	traceIPCRequestCensusKey
	source traceIPCResultSource
}

type traceIPCRequestCounts struct {
	total, sync, oneway, unknown int
	status                       string
}

func traceIPCRequestCountsFromRecord(set ObservationRecord) (traceIPCRequestCounts, bool) {
	total, totalOK := traceIPCRequestCensusInt(set.Value)
	syncCount, syncOK := traceIPCRequestCensusNoteInt(set.RichNotes, TraceNoteKeyIPCSyncRequestCount)
	onewayCount, onewayOK := traceIPCRequestCensusNoteInt(set.RichNotes, TraceNoteKeyIPCOnewayRequestCount)
	unknownCount, unknownOK := traceIPCRequestCensusNoteInt(set.RichNotes, TraceNoteKeyIPCUnknownRequestCount)
	status := strings.TrimSpace(traceObservationRichNoteValue(set.RichNotes, TraceNoteKeyIPCRequestCensusStatus))
	valid := totalOK && syncOK && onewayOK && unknownOK && total >= 0 && syncCount >= 0 && onewayCount >= 0 && unknownCount >= 0 &&
		syncCount+onewayCount+unknownCount == total && (status == "complete" || status == "lower_bound_capacity_truncated")
	return traceIPCRequestCounts{total, syncCount, onewayCount, unknownCount, status}, valid
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
	var sets []ObservationRecord
	rows := map[traceIPCRequestCohortKey][]ObservationRecord{}
	artifacts := map[string]traceRuntimeAuthorityArtifact{}
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
			sets = append(sets, record)
			artifacts[key.artifact] = traceRuntimeAuthorityArtifactFromRecord(record)
		case "ipc_request_edge":
			if source, ok := traceIPCResultSourceFromRecord(record); ok {
				cohort := traceIPCRequestCohortKey{traceIPCRequestCensusKey: key, source: source}
				rows[cohort] = append(rows[cohort], record)
			}
		}
	}

	labels := traceRuntimeAuthorityArtifactLabels(artifacts)
	// One result/target/window has one count partition. Contradictory copies
	// of that exact census cannot elect a larger or later partition.
	cohortCounts := map[traceIPCRequestCohortKey]traceIPCRequestCounts{}
	conflictedCohorts := map[traceIPCRequestCohortKey]bool{}
	for _, set := range sets {
		key, _ := traceIPCRequestCensusRecordKey(set)
		source, sourceKnown := traceIPCResultSourceFromRecord(set)
		counts, valid := traceIPCRequestCountsFromRecord(set)
		if !sourceKnown || !valid {
			continue
		}
		cohort := traceIPCRequestCohortKey{traceIPCRequestCensusKey: key, source: source}
		if previous, exists := cohortCounts[cohort]; exists && previous != counts {
			conflictedCohorts[cohort] = true
		}
		cohortCounts[cohort] = counts
	}
	var out []TraceIPCRequestCensusAuthority
	// Identical result facts may corroborate each other, but a different
	// result's roster must never fill a census. Keep different cohorts visible.
	byFacts := map[string]int{}
	for _, set := range sets {
		key, _ := traceIPCRequestCensusRecordKey(set)
		counts, valid := traceIPCRequestCountsFromRecord(set)
		source, sourceKnown := traceIPCResultSourceFromRecord(set)
		cohort := traceIPCRequestCohortKey{traceIPCRequestCensusKey: key, source: source}
		if !valid || sourceKnown && conflictedCohorts[cohort] {
			continue
		}
		authority := TraceIPCRequestCensusAuthority{
			ArtifactKey:     key.artifact,
			ArtifactLabel:   labels[key.artifact],
			SourceRecordID:  strings.TrimSpace(set.ID),
			SelectedWindow:  key.window,
			Subject:         key.subject,
			CoverageStatus:  counts.status,
			TotalRequests:   counts.total,
			SyncRequests:    counts.sync,
			OnewayRequests:  counts.oneway,
			UnknownRequests: counts.unknown,
		}
		type occurrenceKey struct {
			transactionID, lineStart int
			start                    float64
		}
		type occurrence struct {
			key     occurrenceKey
			row     TraceIPCSyncRequest
			lineEnd int
		}
		byOccurrence := map[occurrenceKey]occurrence{}
		conflicts := map[occurrenceKey]bool{}
		for _, row := range rows[cohort] {
			if !sourceKnown || strings.TrimSpace(set.ObservedAt) != "" && strings.TrimSpace(row.ObservedAt) != "" && set.ObservedAt != row.ObservedAt {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(traceObservationRichNoteValue(row.RichNotes, TraceNoteKeyIPCCallSemantics)), "sync_request") {
				continue
			}
			transactionID, ok := traceIPCRequestCensusNoteInt(row.RichNotes, TraceNoteKeyIPCTransactionID)
			if !ok || transactionID <= 0 {
				continue
			}
			sendTs, receiveTs := row.Span.StartTs, row.Span.EndTs
			if sendTs <= 0 || receiveTs < sendTs || math.IsNaN(sendTs) || math.IsNaN(receiveTs) ||
				math.IsInf(sendTs, 0) || math.IsInf(receiveTs, 0) {
				continue
			}
			request := TraceIPCSyncRequest{
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
			}
			// A send identifies the request. Its receive endpoint is a paired
			// fact, not permission to count a second request on disagreement.
			occKey := occurrenceKey{transactionID, row.Span.LineStart, sendTs}
			if previous, exists := byOccurrence[occKey]; exists {
				before, after := previous.row, request
				before.RecordID, after.RecordID = "", ""
				if before != after || previous.lineEnd != row.Span.LineEnd {
					conflicts[occKey] = true
					continue
				}
				if previous.row.RecordID < request.RecordID {
					request = previous.row
				}
			}
			byOccurrence[occKey] = occurrence{occKey, request, row.Span.LineEnd}
		}
		var occurrences []occurrence
		for key, item := range byOccurrence {
			if !conflicts[key] {
				occurrences = append(occurrences, item)
			}
		}
		sort.Slice(occurrences, func(i, j int) bool {
			a, b := occurrences[i], occurrences[j]
			if a.row.SendTs != b.row.SendTs {
				return a.row.SendTs < b.row.SendTs
			}
			if a.row.ReceiveTs != b.row.ReceiveTs {
				return a.row.ReceiveTs < b.row.ReceiveTs
			}
			if a.key.lineStart != b.key.lineStart {
				return a.key.lineStart < b.key.lineStart
			}
			if a.lineEnd != b.lineEnd {
				return a.lineEnd < b.lineEnd
			}
			return a.row.TransactionID < b.row.TransactionID
		})
		var coordinates [][2]int
		for _, occurrence := range occurrences {
			authority.SyncRoster = append(authority.SyncRoster, occurrence.row)
			coordinates = append(coordinates, [2]int{occurrence.key.lineStart, occurrence.lineEnd})
		}
		if !sourceKnown || len(conflicts) > 0 || len(authority.SyncRoster) != authority.SyncRequests {
			authority.CoverageStatus = "counts_complete_sync_roster_incomplete"
			if counts.status != "complete" {
				authority.CoverageStatus = "lower_bound_sync_roster_incomplete"
			}
		}
		facts := authority
		facts.SourceRecordID = ""
		facts.SyncRoster = append([]TraceIPCSyncRequest(nil), authority.SyncRoster...)
		for i := range facts.SyncRoster {
			facts.SyncRoster[i].RecordID = ""
		}
		encoded, err := json.Marshal(struct {
			Authority   TraceIPCRequestCensusAuthority
			Coordinates [][2]int
		}{facts, coordinates})
		if err != nil {
			continue
		}
		fingerprint := string(encoded)
		if index, exists := byFacts[fingerprint]; exists {
			if traceIPCRequestAuthoritySourceLess(authority, out[index]) {
				out[index] = authority
			}
			continue
		}
		byFacts[fingerprint] = len(out)
		out = append(out, authority)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ArtifactKey != b.ArtifactKey {
			return a.ArtifactKey < b.ArtifactKey
		}
		if a.SelectedWindow != b.SelectedWindow {
			return a.SelectedWindow < b.SelectedWindow
		}
		if a.Subject != b.Subject {
			return a.Subject < b.Subject
		}
		return traceIPCRequestAuthoritySourceLess(a, b)
	})
	return out
}

func traceIPCRequestAuthoritySourceLess(a, b TraceIPCRequestCensusAuthority) bool {
	if a.SourceRecordID != b.SourceRecordID {
		return a.SourceRecordID < b.SourceRecordID
	}
	// Preserve stable output even for legacy duplicated/missing record IDs;
	// this tie-break only selects a witness for already identical typed facts.
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) < string(right)
}

// A timestamp or path alone is not a result receipt. Both references are
// copied by the real producer onto its census and request rows; keeping the
// exact carrier path also prevents raw/original aliases from lending rows.
func traceIPCResultSourceFromRecord(record ObservationRecord) (traceIPCResultSource, bool) {
	source := traceIPCResultSource{
		kind: record.SourceRef.Kind, producer: strings.TrimSpace(record.Producer),
		path:    strings.TrimSpace(record.SourceRef.Path),
		payload: strings.TrimSpace(record.SourceRef.PayloadRef), raw: strings.TrimSpace(record.SourceRef.RawRef),
	}
	return source, source.payload != "" || source.raw != ""
}

func traceIPCRequestCensusRecordKey(record ObservationRecord) (traceIPCRequestCensusKey, bool) {
	artifact := TraceCausalProjectionRecordArtifactIdentity(record)
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
