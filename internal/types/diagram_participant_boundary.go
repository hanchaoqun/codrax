package types

import "strings"

// DiagramParticipantBoundaryStatus is a model-authored decision about a
// schema-required diagram participant for which no evidence-backed incident
// relation is available in the final diagram. It is deliberately not a
// relation kind and cannot mint an edge.
type DiagramParticipantBoundaryStatus string

const (
	DiagramParticipantBoundaryUnknown  DiagramParticipantBoundaryStatus = ""
	DiagramParticipantBoundaryUnproven DiagramParticipantBoundaryStatus = "unproven"
)

func (s DiagramParticipantBoundaryStatus) IsValid() bool {
	return s == DiagramParticipantBoundaryUnproven
}

// DiagramParticipantBoundary keeps an uncovered incident_required participant
// visible and explicitly bounded without forcing the model to invent a
// relationship. Participant must resolve to one typed analyzer participant;
// validators never infer it from prose.
type DiagramParticipantBoundary struct {
	Participant string                           `json:"participant"`
	Status      DiagramParticipantBoundaryStatus `json:"status"`
}

func CloneDiagramParticipantBoundaries(in []DiagramParticipantBoundary) []DiagramParticipantBoundary {
	if len(in) == 0 {
		return nil
	}
	out := append([]DiagramParticipantBoundary(nil), in...)
	for i := range out {
		out[i].Participant = strings.TrimSpace(out[i].Participant)
	}
	return out
}
