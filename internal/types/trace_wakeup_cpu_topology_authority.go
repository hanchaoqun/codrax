package types

import (
	"strconv"
	"strings"
)

// TraceWakeupCPUTopologyAuthorityLimit bounds the exact wakeup-placement rows
// carried into the final answer prompt and the post-answer typed-fact appendix.
// Both consumers intentionally share this compiler so they cannot disagree
// about which edge/CPU tuples are authoritative.
const TraceWakeupCPUTopologyAuthorityLimit = 3

type TraceWakeupCPUTopologyRelation string

const (
	TraceWakeupCPUTopologySameCPU  TraceWakeupCPUTopologyRelation = "same_cpu"
	TraceWakeupCPUTopologyCrossCPU TraceWakeupCPUTopologyRelation = "cross_cpu"
)

// TraceWakeupCPUTopologyAuthority is an exact producer-owned placement tuple.
// It proves where the wakeup event ran and which CPU received the wakee. It
// does not by itself prove runnable overlap, preemption, direct competition, or
// a semantic completion/wait binding.
type TraceWakeupCPUTopologyAuthority struct {
	Waker          string
	Wakee          string
	WakerCPU       int
	WakeeTargetCPU int
	Relation       TraceWakeupCPUTopologyRelation
}

// BuildTraceWakeupCPUTopologyAuthorities compiles only hard, deterministic
// trace_query edge rows. Missing, unknown, or internally inconsistent CPU
// relations remain unpublished/unknown rather than being guessed from prose.
func BuildTraceWakeupCPUTopologyAuthorities(ledger ObservationLedger) []TraceWakeupCPUTopologyAuthority {
	seen := make(map[string]bool)
	out := make([]TraceWakeupCPUTopologyAuthority, 0, TraceWakeupCPUTopologyAuthorityLimit)
	for _, record := range ledger.Records {
		if len(out) >= TraceWakeupCPUTopologyAuthorityLimit {
			break
		}
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			record.GroundingPolicy != ClaimGroundingHard ||
			strings.TrimSpace(record.Predicate) != "wakeup_chain_edge" {
			continue
		}
		waker := strings.TrimSpace(record.Subject)
		wakee := strings.TrimSpace(record.Object)
		wakerCPU, wakerOK := traceWakeupCPUTopologyNoteInt(record.RichNotes, TraceNoteKeyWakeupWakerCPU)
		wakeeCPU, wakeeOK := traceWakeupCPUTopologyNoteInt(record.RichNotes, TraceNoteKeyWakeupWakeeTargetCPU)
		relation := TraceWakeupCPUTopologyRelation(strings.TrimSpace(traceWakeupCPUTopologyNoteValue(record.RichNotes, TraceNoteKeyWakeupCPURelation)))
		if waker == "" || wakee == "" || !wakerOK || !wakeeOK || wakerCPU < 0 || wakeeCPU < 0 {
			continue
		}
		switch relation {
		case TraceWakeupCPUTopologySameCPU:
			if wakerCPU != wakeeCPU {
				continue
			}
		case TraceWakeupCPUTopologyCrossCPU:
			if wakerCPU == wakeeCPU {
				continue
			}
		default:
			continue
		}
		key := strings.Join([]string{
			waker, wakee, strconv.Itoa(wakerCPU), strconv.Itoa(wakeeCPU), string(relation),
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, TraceWakeupCPUTopologyAuthority{
			Waker: waker, Wakee: wakee, WakerCPU: wakerCPU,
			WakeeTargetCPU: wakeeCPU, Relation: relation,
		})
	}
	return out
}

func traceWakeupCPUTopologyNoteInt(notes []string, key string) (int, bool) {
	raw := traceWakeupCPUTopologyNoteValue(notes, key)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil
}

func traceWakeupCPUTopologyNoteValue(notes []string, key string) string {
	prefix := strings.TrimSpace(key) + "="
	if prefix == "=" {
		return ""
	}
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if strings.HasPrefix(note, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(note, prefix))
		}
	}
	return ""
}
