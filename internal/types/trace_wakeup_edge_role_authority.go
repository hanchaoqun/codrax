package types

import (
	"fmt"
	"sort"
	"strings"
)

// TraceWakeupEdgeRoleAuthority is one exact trace_query wakeup edge with
// endpoint-owned priority and CPU fields. It is prompt context only: it does
// not create a causal seat, relation, rank, eliminable amount, or answer.
type TraceWakeupEdgeRoleAuthority struct {
	ArtifactLabel          string
	Scope                  string
	Waker                  string
	Wakee                  string
	WakeupTimestamp        string
	WakerPriority          string
	WakeePriority          string
	WakerPrioritySource    string
	WakeePrioritySource    string
	WakeePriorityAuthority string
	WakerCPU               string
	WakeeTargetCPU         string
	CPURelation            string
	SourceRecordID         string
}

// BuildTraceWakeupEdgeRoleAuthorities compiles target-bound endpoint roles
// from exact deterministic query observations. Matching is against the edge's
// wakee (Object), because ObservationRecord.Subject is the waker. Duplicate
// identical queries fold; conflicting rows for one edge identity fail closed.
func BuildTraceWakeupEdgeRoleAuthorities(ledger ObservationLedger, rm *RequestModel) []TraceWakeupEdgeRoleAuthority {
	if rm == nil || len(rm.RuntimeTargets) == 0 {
		return nil
	}
	type candidate struct {
		authority   TraceWakeupEdgeRoleAuthority
		fingerprint string
		conflict    bool
	}
	requestedWindow := ""
	if start, end, ok := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow(); ok {
		requestedWindow = fmt.Sprintf("%.6f..%.6f", start, end)
	}
	// Edges do not duplicate the query window on every row. Join their exact
	// result scope to a deterministic same-scope selected-window carrier. For
	// an explicit user window, an edge with no matching scope window is stale
	// or unqualified exploration context and therefore cannot enter the recap.
	scopeWindows := make(map[string]map[string]bool)
	if requestedWindow != "" {
		for _, record := range ledger.Records {
			if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
				!RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
				record.GroundingPolicy != ClaimGroundingHard {
				continue
			}
			window := traceObservationRichNoteValue(record.RichNotes, TraceNoteKeySelectedWindow)
			if window == "" && strings.TrimSpace(record.Predicate) == "wakeup_chain" {
				window = traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWindow)
			}
			if window == "" {
				continue
			}
			key := traceWakeupEdgeRoleScopeKey(record.SourceRef, traceWakeupEdgeRoleScope(record.ID))
			if scopeWindows[key] == nil {
				scopeWindows[key] = make(map[string]bool)
			}
			scopeWindows[key][window] = true
		}
	}
	byKey := make(map[string]candidate)
	order := make([]string, 0)
	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			record.GroundingPolicy != ClaimGroundingHard ||
			strings.TrimSpace(record.Predicate) != "wakeup_chain_edge" ||
			strings.TrimSpace(record.Subject) == "" ||
			strings.TrimSpace(record.Object) == "" {
			continue
		}
		wakeeRecord := record
		wakeeRecord.Subject = record.Object
		if !ObservationRecordMatchesUserRuntimeTarget(wakeeRecord, rm) {
			continue
		}
		if requestedWindow != "" {
			scopeKey := traceWakeupEdgeRoleScopeKey(record.SourceRef, traceWakeupEdgeRoleScope(record.ID))
			if !scopeWindows[scopeKey][requestedWindow] {
				continue
			}
		}
		authority := TraceWakeupEdgeRoleAuthority{
			ArtifactLabel:          traceTargetStateAuthorityArtifactLabel(record.SourceRef),
			Scope:                  traceWakeupEdgeRoleScope(record.ID),
			Waker:                  strings.TrimSpace(record.Subject),
			Wakee:                  strings.TrimSpace(record.Object),
			WakeupTimestamp:        traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeupTs),
			WakerPriority:          traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakerPriority),
			WakeePriority:          traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeePriority),
			WakerPrioritySource:    traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakerPrioritySource),
			WakeePrioritySource:    traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeePrioritySource),
			WakeePriorityAuthority: traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeePriorityAuthority),
			WakerCPU:               traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeupWakerCPU),
			WakeeTargetCPU:         traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeupWakeeTargetCPU),
			CPURelation:            traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeupCPURelation),
			SourceRecordID:         strings.TrimSpace(record.ID),
		}
		if authority.WakerPriority == "" && authority.WakeePriority == "" &&
			authority.WakerCPU == "" && authority.WakeeTargetCPU == "" {
			continue
		}
		key := strings.Join([]string{
			traceWakeupEdgeRoleArtifactIdentity(record.SourceRef), authority.Scope,
			authority.Waker, authority.Wakee, authority.WakeupTimestamp,
		}, "\x00")
		fingerprint := strings.Join([]string{
			authority.ArtifactLabel, authority.Scope, authority.Waker, authority.Wakee,
			authority.WakeupTimestamp, authority.WakerPriority, authority.WakeePriority,
			authority.WakerPrioritySource, authority.WakeePrioritySource,
			authority.WakeePriorityAuthority, authority.WakerCPU, authority.WakeeTargetCPU,
			authority.CPURelation,
		}, "\x00")
		if prior, ok := byKey[key]; ok {
			if prior.fingerprint != fingerprint {
				prior.conflict = true
				byKey[key] = prior
			}
			continue
		}
		byKey[key] = candidate{authority: authority, fingerprint: fingerprint}
		order = append(order, key)
	}
	out := make([]TraceWakeupEdgeRoleAuthority, 0, len(order))
	for _, key := range order {
		candidate := byKey[key]
		if !candidate.conflict {
			out = append(out, candidate.authority)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ArtifactLabel != out[j].ArtifactLabel {
			return out[i].ArtifactLabel < out[j].ArtifactLabel
		}
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		if out[i].WakeupTimestamp != out[j].WakeupTimestamp {
			return out[i].WakeupTimestamp < out[j].WakeupTimestamp
		}
		return out[i].Waker < out[j].Waker
	})
	return out
}

func traceWakeupEdgeRoleScope(id string) string {
	id = strings.TrimSpace(id)
	if before, _, ok := strings.Cut(id, "#wakeup_chain_edge:"); ok {
		return strings.TrimSpace(before)
	}
	if before, _, ok := strings.Cut(id, "#"); ok {
		return strings.TrimSpace(before)
	}
	return id
}

func traceWakeupEdgeRoleArtifactIdentity(ref ObservationSourceRef) string {
	for _, candidate := range []string{ref.CaptureIdentityPath, ref.Path, ref.ArtifactID, ref.PayloadRef} {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate
		}
	}
	return "unattributed"
}

func traceWakeupEdgeRoleScopeKey(ref ObservationSourceRef, scope string) string {
	return traceWakeupEdgeRoleArtifactIdentity(ref) + "\x00" + strings.TrimSpace(scope)
}
