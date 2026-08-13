package types

import "strings"

// DiagramParticipantBoundaryStatus is a model-authored decision about a
// schema-required diagram participant for which the requested directed
// relation is not proved in the final diagram. Independently proved local
// facts or no-arrow containment may still be shown; the boundary cannot mint
// an edge or promote them into the requested relation.
type DiagramParticipantBoundaryStatus string

const (
	DiagramParticipantBoundaryUnknown  DiagramParticipantBoundaryStatus = ""
	DiagramParticipantBoundaryUnproven DiagramParticipantBoundaryStatus = "unproven"
)

func (s DiagramParticipantBoundaryStatus) IsValid() bool {
	return s == DiagramParticipantBoundaryUnproven
}

// DiagramParticipantBoundary keeps an uncovered incident_required participant
// visible and explicitly bounds the requested directed relation without
// forcing the model to invent a bridge. Participant must resolve to one typed
// analyzer participant; validators never infer it from prose.
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
