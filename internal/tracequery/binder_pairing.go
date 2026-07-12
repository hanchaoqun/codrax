package tracequery

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const binderPairingEndpointBudget = 65536

type binderPairingLane struct {
	cohort        pairingCohortState
	sendLines     []int
	orphanReceive bool
	lastTs        float64
	timestampSeen bool
}

type binderPairingObservation struct {
	event    Event
	verdict  PairingEndpointVerdict
	source   string
	key      string
	selected bool
}

type binderPairingAudit struct {
	pairs               map[int]Event
	matchedReceives     map[int]bool
	admittedSends       map[int]string
	admittedReceives    map[int]string
	sendSources         map[int]string
	receiveSources      map[int]string
	suppressedSends     map[int]bool
	precededSends       map[int]bool
	poisonedKeys        map[string]bool
	globalSources       map[string]bool
	familyGlobal        bool
	topologyIncomplete  bool
	unkeyableEndpoints  int
	unresolvedSources   int
	rejectedEmitters    int
	unknownEmitters     int
	ambiguousCohorts    int
	timeRollbacks       int
	rawEndpointFailures int
	endpointCount       int
	endpointLimit       int
	budgetExceeded      bool
	rawAuditCapped      bool
}

func newBinderPairingAudit() *binderPairingAudit {
	return &binderPairingAudit{
		pairs:            map[int]Event{},
		matchedReceives:  map[int]bool{},
		admittedSends:    map[int]string{},
		admittedReceives: map[int]string{},
		sendSources:      map[int]string{},
		receiveSources:   map[int]string{},
		suppressedSends:  map[int]bool{},
		precededSends:    map[int]bool{},
		poisonedKeys:     map[string]bool{},
		globalSources:    map[string]bool{},
	}
}

// auditBinderPairing audits the complete retained physical endpoint topology;
// q is only the publication gate for a matched pair. A begin before the window
// may still be open at the head, and a timestamp-regressed endpoint whose time
// lies outside q may still sit physically between two selected rows. Filtering
// either before replay can turn an overlapping/malformed cohort into a false
// one-to-one edge. Each source is therefore replayed by physical Line first;
// only pairs whose two endpoints are selected are published.
func auditBinderPairing(idx *Index, q Query) *binderPairingAudit {
	return auditBinderPairingWithBudget(idx, q, binderPairingEndpointBudget)
}

func auditBinderPairingWithBudget(idx *Index, q Query, endpointLimit int) *binderPairingAudit {
	audit := newBinderPairingAudit()
	audit.endpointLimit = endpointLimit
	if idx == nil {
		return audit
	}
	if endpointLimit <= 0 || endpointLimit > binderPairingEndpointBudget {
		audit.budgetExceeded = true
		audit.familyGlobal = true
		return audit
	}
	if !completePhysicalPairingTopology(idx) {
		audit.familyGlobal = true
		audit.topologyIncomplete = true
		return audit
	}
	// Parser-rejected Binder rows have no Event and therefore cannot be
	// rediscovered by the replay below. Consume the same bounded physical-row
	// ledger as the elapsed pairing families before admitting any endpoint: a
	// known transaction poisons only its exact source-scoped lane; an unknown
	// transaction poisons only that physical source. Unresolved provenance or a
	// capped witness ledger fails the Binder family closed.
	for _, failure := range idx.durationOrderFailures {
		if failure.Family != durationOrderBinder {
			continue
		}
		audit.rawEndpointFailures++
		if lane, _, laneOK := durationPairingFailureLane(idx, failure); laneOK {
			audit.poisonedKeys[lane] = true
			continue
		}
		if source, sourceOK := durationPairingFailureSource(idx, failure); sourceOK {
			audit.globalSources[source] = true
			audit.unkeyableEndpoints++
			continue
		}
		audit.unresolvedSources++
		audit.familyGlobal = true
	}
	if idx.durationOrderFailuresCapped[durationOrderBinder] {
		audit.rawAuditCapped = true
		audit.familyGlobal = true
	}
	observations := make([]binderPairingObservation, 0, 64)
	seenCoordinates := map[string]map[int]bool{}
	for _, ev := range idx.Events {
		verdict := binderEndpointVerdictForEvent(ev)
		if !verdict.Recognized || verdict.Family != PairingEndpointBinder {
			continue
		}
		audit.endpointCount++
		if audit.endpointCount > endpointLimit {
			audit.budgetExceeded = true
			audit.familyGlobal = true
			return audit
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			audit.unresolvedSources++
			audit.familyGlobal = true
			return audit
		}
		if ev.Line <= 0 {
			audit.unkeyableEndpoints++
			audit.globalSources[source] = true
			continue
		}
		lines := seenCoordinates[source]
		if lines == nil {
			lines = map[int]bool{}
			seenCoordinates[source] = lines
		}
		if lines[ev.Line] {
			audit.unkeyableEndpoints++
			audit.globalSources[source] = true
			continue
		}
		lines[ev.Line] = true
		key, keyOK := verdict.LaneKey(source)
		if !keyOK {
			audit.unkeyableEndpoints++
			audit.globalSources[source] = true
			continue
		}
		if !verdict.EmitterKnown {
			audit.unknownEmitters++
			audit.globalSources[source] = true
			continue
		}
		if !verdict.PayloadAdmitted || !verdict.EmitterAdmitted {
			audit.rejectedEmitters++
			audit.poisonedKeys[key] = true
			continue
		}
		observations = append(observations, binderPairingObservation{
			event: ev, verdict: verdict, source: source, key: key,
			selected: pairingEventInsideQuery(ev, q),
		})
	}

	sort.SliceStable(observations, func(i, j int) bool {
		if observations[i].source != observations[j].source {
			return observations[i].source < observations[j].source
		}
		if observations[i].event.Line != observations[j].event.Line {
			return observations[i].event.Line < observations[j].event.Line
		}
		return observations[i].event.Ts < observations[j].event.Ts
	})
	lanes := map[string]*binderPairingLane{}
	selectedEndpoints := map[int]bool{}
	for _, observation := range observations {
		if observation.selected {
			selectedEndpoints[observation.event.Line] = true
		}
	}
	for _, observation := range observations {
		ev, verdict, key := observation.event, observation.verdict, observation.key
		if audit.poisonedKeys[key] {
			continue
		}
		lane := lanes[key]
		if lane == nil {
			lane = &binderPairingLane{}
			lanes[key] = lane
		}
		if lane.timestampSeen && ev.Ts < lane.lastTs {
			audit.timeRollbacks++
			audit.poisonedKeys[key] = true
			for _, line := range lane.sendLines {
				audit.suppressedSends[line] = true
			}
			lane.cohort = pairingCohortState{}
			lane.sendLines = nil
			lane.orphanReceive = false
			continue
		}
		lane.lastTs = ev.Ts
		lane.timestampSeen = true
		switch verdict.Phase {
		case PairingEndpointStart:
			audit.admittedSends[ev.Line] = key
			audit.sendSources[ev.Line] = observation.source
			if lane.cohort.depth == 0 && lane.orphanReceive {
				audit.precededSends[ev.Line] = true
				lane.orphanReceive = false
			}
			if lane.cohort.depth == 0 {
				lane.sendLines = lane.sendLines[:0]
			}
			lane.sendLines = append(lane.sendLines, ev.Line)
			lane.cohort.observeStart(ev)
		case PairingEndpointDone:
			audit.admittedReceives[ev.Line] = key
			audit.receiveSources[ev.Line] = observation.source
			transition := lane.cohort.observeDone(ev)
			if transition.unpairedDone {
				lane.orphanReceive = true
			}
			if transition.pairReady {
				if ev.Ts < transition.pairStart.Ts {
					audit.timeRollbacks++
					audit.poisonedKeys[key] = true
					audit.suppressedSends[transition.pairStart.Line] = true
				} else {
					if selectedEndpoints[transition.pairStart.Line] && selectedEndpoints[ev.Line] {
						audit.pairs[transition.pairStart.Line] = ev
						audit.matchedReceives[ev.Line] = true
					}
				}
			}
			if transition.cohortClosed {
				if transition.ambiguous {
					audit.ambiguousCohorts++
					for _, line := range lane.sendLines {
						audit.suppressedSends[line] = true
					}
				}
				lane.sendLines = lane.sendLines[:0]
			}
		}
	}
	for _, lane := range lanes {
		transition := lane.cohort.finishEOF()
		if !transition.ambiguous {
			continue
		}
		audit.ambiguousCohorts++
		for _, line := range lane.sendLines {
			audit.suppressedSends[line] = true
		}
	}
	return audit
}

func (audit *binderPairingAudit) sendLane(line int) (string, bool) {
	if audit == nil || audit.familyGlobal || audit.suppressedSends[line] || audit.globalSources[audit.sendSources[line]] {
		return "", false
	}
	key, ok := audit.admittedSends[line]
	if !ok || audit.poisonedKeys[key] {
		return "", false
	}
	return key, true
}

func (audit *binderPairingAudit) receiveUsable(line int) bool {
	if audit == nil || audit.familyGlobal || audit.globalSources[audit.receiveSources[line]] {
		return false
	}
	key, ok := audit.admittedReceives[line]
	return ok && !audit.poisonedKeys[key]
}

func (audit *binderPairingAudit) matchForSend(line int) (Event, bool) {
	if _, ok := audit.sendLane(line); !ok {
		return Event{}, false
	}
	recv, ok := audit.pairs[line]
	if !ok || !audit.receiveUsable(recv.Line) {
		return Event{}, false
	}
	return recv, true
}

func (audit *binderPairingAudit) receivePrecededSend(line int) bool {
	return audit != nil && audit.precededSends[line]
}

func (audit *binderPairingAudit) caveats() []string {
	if audit == nil {
		return nil
	}
	var out []string
	if audit.topologyIncomplete {
		return []string{"binder_pairing_fail_closed=true; windowed_pairing_topology_incomplete=true; Binder edges were omitted because this cropped index has no complete physical endpoint topology proof; rebuild without windowed parsing or wait for a bounded pairing-topology sidecar"}
	}
	if audit.familyGlobal {
		out = append(out, fmt.Sprintf("binder_pairing_fail_closed=true; unresolved_sources=%d budget_exceeded=%t raw_audit_capped=%t raw_endpoint_failures=%d endpoints=%d limit=%d; binder edges were omitted because the complete physical endpoint set could not be assigned within one bounded physical-source authority", audit.unresolvedSources, audit.budgetExceeded, audit.rawAuditCapped, audit.rawEndpointFailures, audit.endpointCount, audit.endpointLimit))
		return out
	}
	if len(audit.globalSources) > 0 {
		out = append(out, fmt.Sprintf("binder_pairing_source_fail_closed=true; sources=%d unkeyable_endpoints=%d unknown_emitters=%d raw_endpoint_failures=%d; only affected physical-source Binder families were omitted", len(audit.globalSources), audit.unkeyableEndpoints, audit.unknownEmitters, audit.rawEndpointFailures))
	}
	if len(audit.poisonedKeys) > 0 {
		out = append(out, fmt.Sprintf("binder_pairing_exact_lane_quarantined=true; lanes=%d rejected_emitters=%d timestamp_rollbacks=%d raw_endpoint_failures=%d; malformed, idle, parser-rejected, or non-monotonic endpoints suppressed only their exact source-scoped transaction lanes", len(audit.poisonedKeys), audit.rejectedEmitters, audit.timeRollbacks, audit.rawEndpointFailures))
	}
	if audit.ambiguousCohorts > 0 {
		out = append(out, fmt.Sprintf("binder_pairing_ambiguous=true; cohorts=%d; overlapping equal transaction identities were withheld instead of reusing or FIFO-guessing receive endpoints", audit.ambiguousCohorts))
	}
	return out
}

func binderAuxPairingLane(idx *Index, ev Event) (string, bool) {
	if ev.BinderFields == nil || ev.BinderFields.TransactionID <= 0 {
		return "", false
	}
	transaction := strconv.Itoa(ev.BinderFields.TransactionID)
	if strings.TrimSpace(ev.FieldText) != "" {
		var ok bool
		transaction, ok = strictCoherentPairingAlias(ev.FieldText, "transaction", "debug_id", "transaction_id")
		if !ok {
			return "", false
		}
	}
	source, ok := tracePairingSourceIdentity(idx, ev)
	if !ok {
		return "", false
	}
	verdict := DecodePairingEndpoint(string(EventBinderTransaction), "transaction="+transaction, int64(ev.PID))
	if !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted {
		return "", false
	}
	return verdict.LaneKey(source)
}

func binderEndpointVerdictForEvent(ev Event) PairingEndpointVerdict {
	verdict := fingerprintPairingEvent(ev)
	if verdict.Recognized || (ev.Type != EventBinderTransaction && ev.Type != EventBinderReceived) {
		return verdict
	}
	if strings.TrimSpace(ev.Name) != "" {
		return verdict
	}
	// Compatibility for hand-built package/external Events. Production
	// ParseLine always sets Name and FieldText and cannot enter this lane.
	name := string(EventBinderTransaction)
	if ev.Type == EventBinderReceived {
		name = string(EventBinderReceived)
	}
	transaction := 0
	if ev.BinderFields != nil {
		transaction = ev.BinderFields.TransactionID
	}
	return DecodePairingEndpoint(name, fmt.Sprintf("transaction=%d", transaction), int64(ev.PID))
}
