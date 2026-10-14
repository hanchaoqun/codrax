package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Called only by the frozen roster gate, with its copying accessor. Later
// entity expansion/narrowing cannot erase a contradiction in the declaration.
// This refuses an ambiguous presentation join, not the native observations,
// and does not infer actor roles from request or answer prose.
func validateRuntimeParticipantBindingRoster(rm types.RequestModel, entities []string) string {
	if rm.DiagramHint == nil || !rm.DiagramHint.Required {
		return ""
	}
	for i, participant := range rm.DiagramHint.Participants {
		tid, eligible := requestedRuntimeParticipantThread(&rm, participant)
		if !eligible {
			continue
		}
		if pid, name, ok := threadidentity.Identity(participant.Identity); ok {
			if pid != tid {
				return fmt.Sprintf("diagram_hint.participants[%d] has an ambiguous native thread binding: its explicit thread coordinate %d differs from the selected thread %d; retain the requested coordinate and use a matching typed target quote", i, pid, tid)
			}
			if name == "" {
				// An exact coordinate is not a name alias; other quoted
				// actors cannot make that coordinate ambiguous.
				continue
			}
		}
		var members []string
		seen := map[string]bool{}
		add := func(entity string) {
			entity = strings.TrimSpace(entity)
			// The entire quoted phrase is a carrier, not an extra actor.
			// Separately declared identities can prove a composite span;
			// natural-language segmentation cannot.
			if entity == "" || entity == strings.TrimSpace(participant.SourceQuote) ||
				!entityNamedInQuote(participant.SourceQuote, entity) {
				return
			}
			key, selectorOnly := runtimeParticipantBindingIdentityKey(entity, tid)
			if !selectorOnly && key != "" && !seen[key] {
				seen[key] = true
				members = append(members, entity)
			}
		}
		for _, entity := range entities {
			add(entity)
		}
		// PID provenance does not authorize a paired display name. A name
		// explicitly present in this quote can only veto a conflicting join;
		// equality here never creates positive identity authority.
		for _, target := range rm.RuntimeTargets {
			if target.Source == "user_explicit" && target.Kind == types.RuntimeTargetKindThread && target.PID == tid {
				add(target.Thread)
			}
		}
		participantKey, _ := runtimeParticipantBindingIdentityKey(participant.Identity, tid)
		spanIdentity := participant.Identity == participant.SourceQuote
		if spanIdentity && len(members) <= 1 {
			continue
		}
		var conflicting []string
		for _, member := range members {
			key, _ := runtimeParticipantBindingIdentityKey(member, tid)
			if !spanIdentity && key == participantKey {
				continue
			}
			conflicting = append(conflicting, member)
		}
		if len(conflicting) != 0 {
			return fmt.Sprintf("diagram_hint.participants[%d] has an ambiguous native thread binding: source_quote for %q also contains distinct originally declared identities %v; narrow the participant and runtime target profile quotes to the exact target phrase, or retain separately declared actors as separate participant rows. One shared phrase with a numeric selector does not assign that thread to every named actor. Do not change roles or numeric targets merely to bypass this consistency check", i, participant.Identity, conflicting)
		}
	}
	return ""
}

// Compare complete typed identity carriers only. A bare 42 or tid=42 is the
// already selected coordinate, not a second actor beside its name. A named
// selector uses its existing canonical name; no free-form quote is parsed.
func runtimeParticipantBindingIdentityKey(identity string, tid int) (string, bool) {
	if pid, name, ok := threadidentity.Identity(identity); ok && pid == tid {
		if name == "" {
			return "", true
		}
		identity = name
	}
	return diagramParticipantIdentityAliasKey(identity), false
}
