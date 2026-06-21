package types

// SourceInventoryAdvisory is a structured, language-neutral candidate carrier
// for scoped source inventories. It is not an answer and not a semantic source
// citation by itself; downstream agents may use it to avoid repeating broad
// repository enumeration and to preserve candidate/count invariants, while
// validators continue to require normal grounded evidence or accepted aggregate
// facts for user-visible semantic claims.
type SourceInventoryAdvisory struct {
	Active       bool                         `json:"active"`
	AdvisoryOnly bool                         `json:"advisory_only,omitempty"`
	Complete     bool                         `json:"complete,omitempty"`
	Scopes       []string                     `json:"scopes,omitempty"`
	Sets         []SourceInventoryAdvisorySet `json:"sets,omitempty"`
	Provenance   []string                     `json:"provenance,omitempty"`
}

func (a SourceInventoryAdvisory) IsActive() bool {
	return a.Active && len(a.Sets) > 0
}

// SourceInventoryAdvisorySet groups candidates for a single requested role.
// Total is the scanned candidate total or lower bound for the role; Candidates
// may hold only the current cursor page. Complete means the underlying
// graph/index had source files for the requested scopes; it is a boundedness
// signal, not a guarantee that the user wanted this set rendered verbatim.
type SourceInventoryAdvisorySet struct {
	Role       AnswerCandidateRole                `json:"role,omitempty"`
	Complete   bool                               `json:"complete,omitempty"`
	Total      int                                `json:"total,omitempty"`
	Candidates []SourceInventoryAdvisoryCandidate `json:"candidates,omitempty"`
}

// SourceInventoryAdvisoryCandidate is one graph-backed candidate row.
type SourceInventoryAdvisoryCandidate struct {
	Member       string                             `json:"member,omitempty"`
	Key          string                             `json:"key,omitempty"`
	SupportRef   string                             `json:"support_ref,omitempty"`
	Note         string                             `json:"note,omitempty"`
	SurfaceTerms []string                           `json:"surface_terms,omitempty"`
	Role         AnswerCandidateRole                `json:"role,omitempty"`
	Exported     bool                               `json:"exported,omitempty"`
	File         string                             `json:"file,omitempty"`
	Line         int                                `json:"line,omitempty"`
	Language     string                             `json:"language,omitempty"`
	Attributes   []SourceInventoryAdvisoryAttribute `json:"attributes,omitempty"`
}

// SourceInventoryAdvisoryAttribute is graph-backed context attached to a
// candidate member, for example possible callable/entry attributes for a
// package/directory candidate. Attributes are advisory context only: they are
// not independent answer rows and do not act as citations without normal
// grounded evidence.
type SourceInventoryAdvisoryAttribute struct {
	Member       string              `json:"member,omitempty"`
	Key          string              `json:"key,omitempty"`
	SupportRef   string              `json:"support_ref,omitempty"`
	Note         string              `json:"note,omitempty"`
	SurfaceTerms []string            `json:"surface_terms,omitempty"`
	Role         AnswerCandidateRole `json:"role,omitempty"`
	Exported     bool                `json:"exported,omitempty"`
	File         string              `json:"file,omitempty"`
	Line         int                 `json:"line,omitempty"`
	Language     string              `json:"language,omitempty"`
}
