package tracequery

const ViewRenderingCandidates = "rendering_candidates"

const RenderingCandidatesTeaching = "rendering_candidates inventories same-source rendering framework and thread-role clues within the selected window. Multiple frameworks may coexist in one process; known TGID groups process observations, otherwise only the emitting TID is used. Signal counts are matching source-row occurrences, not unique threads or frames. Names are soft navigation, not frame boundaries, thread relationships, measured work or response causes. No match means unknown, not ArkUI. unsupported_definition identifies a clue without an implemented pipeline definition. Candidate, signal and example omissions are disclosed independently."

type RenderingCandidatesResult struct {
	Status            string                    `json:"status"`
	SourcePath        string                    `json:"source_path"`
	Window            RenderingCandidatesWindow `json:"window"`
	TargetPID         int                       `json:"target_pid,omitempty"`
	TargetThread      string                    `json:"target_thread,omitempty"`
	TargetScope       string                    `json:"target_scope,omitempty"`
	Candidates        []RenderingCandidate      `json:"candidates,omitempty"`
	TotalCandidates   int                       `json:"total_candidates"`
	OmittedCandidates int                       `json:"omitted_candidates"`
	Caveats           []string                  `json:"caveats,omitempty"`
}

type RenderingCandidatesWindow struct {
	StartTs      float64 `json:"start_ts"`
	EndTs        float64 `json:"end_ts"`
	EndInclusive bool    `json:"end_inclusive,omitempty"`
}

type RenderingCandidate struct {
	Framework        string                     `json:"framework"`
	DefinitionStatus string                     `json:"definition_status"`
	SourcePath       string                     `json:"source_path"`
	OwnerScope       string                     `json:"owner_scope"`
	OwnerID          int                        `json:"owner_id"`
	Signals          []RenderingCandidateSignal `json:"signals"`
	TotalSignals     int                        `json:"total_signals"`
	OmittedSignals   int                        `json:"omitted_signals"`
}

type RenderingCandidateSignal struct {
	Kind            string                          `json:"kind"`
	Pattern         string                          `json:"pattern"`
	RoleCandidate   string                          `json:"role_candidate"`
	Count           int                             `json:"count"`
	Examples        []RenderingCandidateObservation `json:"examples"`
	OmittedExamples int                             `json:"omitted_examples"`
}

type RenderingCandidateObservation struct {
	Line       int     `json:"line"`
	SourceLine int     `json:"source_line"`
	Ts         float64 `json:"ts"`
	Name       string  `json:"name"`
	TID        int     `json:"tid"`
	TGID       int     `json:"tgid"` // -1 means not recorded; never copied from a marker payload.
	Thread     string  `json:"thread"`
}
