package tool

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This ephemeral carrier is populated only after the native relation provider
// has validated the event, source, query, clock and requested window. It is not
// serializable authority and never changes a relation's instance identities.
type runtimeDiagramThreadEndpoints struct {
	source   types.ObservationSourceRef
	from, to types.RuntimeWakeupThread
}

// Reconcile an exact accepted request phrase with its independently validated
// numeric selector. Identity membership reuses the analyzer's exact lexical
// provenance predicate on that typed quote, never a scan of raw question or
// answer text. A short accepted display identity need not equal its whole
// quote. Co-listed participant identities remain ambiguous; no role/name
// similarity or native comm matching supplies a mapping.
func requestedRuntimeParticipantThread(rm *types.RequestModel, p types.DiagramParticipantHint) (int, bool) {
	if rm == nil || rm.DiagramHint == nil || rm.RuntimeTargetProfile == nil || rm.RuntimeTargetProfile.Declaration != types.RuntimeTargetDeclarationNamedTarget ||
		p.Role != types.DiagramParticipantIncidentRequired || strings.TrimSpace(p.Identity) == "" ||
		p.SourceQuote != rm.RuntimeTargetProfile.SourceQuote || !entityNamedInQuote(p.SourceQuote, p.Identity) {
		return 0, false
	}
	// A shared source span can co-list several actors but only one numeric
	// selector. Exact quote equality cannot assign that selector to all of
	// them. Inspect only the accepted typed roster, not natural-language roles.
	for _, other := range rm.DiagramHint.Participants {
		if other.Identity != p.Identity && entityNamedInQuote(p.SourceQuote, other.Identity) {
			return 0, false
		}
	}
	var selected types.RuntimeTarget
	count := 0
	for _, target := range rm.RuntimeTargets {
		if target.Source != "user_explicit" || !runtimeTargetIdentityInQuote(target, p.SourceQuote) {
			continue
		}
		selected, count = target, count+1
	}
	return selected.PID, count == 1 && selected.Kind == types.RuntimeTargetKindThread && selected.PID > 0 && selected.PID <= types.RuntimeTargetMaxPID
}

func bindRequestedRuntimeParticipants(rows []RuntimeDiagramRelation, rm *types.RequestModel) []RuntimeDiagramRelation {
	if rm == nil || rm.DiagramHint == nil || !rm.DiagramHint.Required {
		return rows
	}
	for _, participant := range rm.DiagramHint.Participants {
		tid, ok := requestedRuntimeParticipantThread(rm, participant)
		if !ok {
			continue
		}
		// Do not collapse different observed process/name precision into a
		// single selected thread within one producer scope. Query/payload still
		// separate independently verified observations of the same request.
		identities := map[string]types.RuntimeWakeupThread{}
		ambiguous := map[string]bool{}
		for _, row := range rows {
			endpoints := row.threadEndpoints
			if endpoints == nil {
				continue
			}
			key := requestedRuntimeParticipantScope(endpoints.source)
			for _, thread := range []types.RuntimeWakeupThread{endpoints.from, endpoints.to} {
				if thread.PID != tid {
					continue
				}
				if before, exists := identities[key]; exists && before != thread {
					ambiguous[key] = true
				}
				identities[key] = thread
			}
		}
		for i := range rows {
			endpoints := rows[i].threadEndpoints
			if endpoints == nil || ambiguous[requestedRuntimeParticipantScope(endpoints.source)] {
				continue
			}
			if endpoints.from.PID == tid {
				rows[i].requestedFrom = append(rows[i].requestedFrom, participant.Identity)
			}
			if endpoints.to.PID == tid {
				rows[i].requestedTo = append(rows[i].requestedTo, participant.Identity)
			}
		}
	}
	return rows
}

func requestedRuntimeParticipantScope(ref types.ObservationSourceRef) string {
	data, _ := json.Marshal(ref)
	return string(data)
}

// Only current provider endpoints are additional presentation surfaces. Exact
// event identities continue to gate every visible edge and local repair.
func requestedRuntimeParticipantSurfaces(identity string, rows []RuntimeDiagramRelation) []string {
	out := []string{identity}
	add := func(values []string, node, instance string) {
		for _, value := range values {
			if value == identity {
				out = append(out, node, instance)
				return
			}
		}
	}
	for _, row := range rows {
		add(row.requestedFrom, row.FromNode, row.FromIdentity)
		add(row.requestedTo, row.ToNode, row.ToIdentity)
	}
	return out
}

func requestedRuntimeParticipantNode(identity, node string, rows []RuntimeDiagramRelation) bool {
	for _, row := range rows {
		for _, side := range []struct {
			node         string
			participants []string
		}{{row.FromNode, row.requestedFrom}, {row.ToNode, row.requestedTo}} {
			if side.node != node {
				continue
			}
			for _, participant := range side.participants {
				if participant == identity {
					return true
				}
			}
		}
	}
	return false
}
