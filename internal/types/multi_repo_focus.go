package types

// MultiRepoFocusSource records why the multi-repo active set was chosen.
// It is prompt/UX metadata, not evidence and not a source-code gate by
// itself. Hard routing decisions still come from explicit focus pins,
// exact topology validation, or the typed selector payload.
type MultiRepoFocusSource string

const (
	MultiRepoFocusSourceUnknown               MultiRepoFocusSource = ""
	MultiRepoFocusSourceUserPinned            MultiRepoFocusSource = "user_pinned"
	MultiRepoFocusSourceUserExplicitInRequest MultiRepoFocusSource = "user_explicit_in_request"
	MultiRepoFocusSourceModelRecommended      MultiRepoFocusSource = "model_recommended"
	MultiRepoFocusSourceFallbackPreview       MultiRepoFocusSource = "fallback_preview"
)

// MultiRepoFocusCandidate is one typed selector recommendation. RootRel
// must match a topology sub-repo root relative to the workspace root.
type MultiRepoFocusCandidate struct {
	RootRel    string               `json:"root_rel,omitempty"`
	Slug       string               `json:"slug,omitempty"`
	Confidence float64              `json:"confidence,omitempty"`
	Reason     string               `json:"reason,omitempty"`
	Source     MultiRepoFocusSource `json:"source,omitempty"`
}

// MultiRepoFocusDecision is the run-scoped active-set explanation.
// It is copied from BusContext to AgentContext so prompts and tools can
// present scope honestly without re-deriving routing internals.
type MultiRepoFocusDecision struct {
	Source     MultiRepoFocusSource      `json:"source,omitempty"`
	Confidence float64                   `json:"confidence,omitempty"`
	Candidates []MultiRepoFocusCandidate `json:"candidates,omitempty"`
	Fallback   bool                      `json:"fallback,omitempty"`
	Rationale  string                    `json:"rationale,omitempty"`
}

// MultiRepoFocusResolver is the narrow topology-facing contract used by
// read-only selection tools. It intentionally lives in types so generic
// tools do not import the concrete multigraph package (which would
// reintroduce the topology -> tool import cycle).
type MultiRepoFocusResolver interface {
	IsSingle() bool
	Cap() int
	MultiRepoSubRepoCount() int
	ResolveMultiRepoSubRepo(token string) (rootRel string, slug string, ok bool)
}
