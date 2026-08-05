package types

// SourceInventoryCoverageState describes how a source-inventory row should be consumed.
// It is structural: callers must not infer semantic answer intent from it.
type SourceInventoryCoverageState string

const (
	SourceInventoryCoverageUnknown   SourceInventoryCoverageState = ""
	SourceInventoryCoverageObserved  SourceInventoryCoverageState = "observed"
	SourceInventoryCoverageAmbiguous SourceInventoryCoverageState = "ambiguous"
	SourceInventoryCoverageNeedsRead SourceInventoryCoverageState = "needs_read"
	SourceInventoryCoverageNoIndex   SourceInventoryCoverageState = "no_index"
)

// SourceInventoryObservation is the auditable companion to
// SourceInventoryAdvisory. It preserves the same candidate rows, while making
// count/list invariants and row support refs explicit for downstream ledgers and
// validators. It is still not final-answer text, does not decide semantic roles
// for the model, and does not replace source-text citations for implementation
// behavior claims.
type SourceInventoryObservation struct {
	Active          bool                              `json:"active"`
	AdvisoryOnly    bool                              `json:"advisory_only,omitempty"`
	Complete        bool                              `json:"complete,omitempty"`
	Scopes          []string                          `json:"scopes,omitempty"`
	QueryPathScopes []string                          `json:"query_path_scopes,omitempty"`
	Provenance      []string                          `json:"provenance,omitempty"`
	Lens            []string                          `json:"lens,omitempty"`
	SourceClasses   []SourceInventorySourceClassCount `json:"source_classes,omitempty"`
	RepoLanguages   []SourceInventoryLanguageCount    `json:"repo_languages,omitempty"`
	CompleteLenses  []SourceInventoryCompleteLens     `json:"complete_lenses,omitempty"`
	Page            *SourceInventoryObservationPage   `json:"page,omitempty"`
	Execution       *SourceInventoryExecutionState    `json:"execution,omitempty"`
	Sets            []SourceInventoryObservationSet   `json:"sets,omitempty"`
}

// SourceInventoryObservationPage is the typed pagination companion for a
// source-inventory lens result. It mirrors the model-facing cursor/offset view
// without requiring downstream gates or status cards to parse rendered
// markdown.
type SourceInventoryObservationPage struct {
	Offset     int    `json:"offset,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Total      int    `json:"total,omitempty"`
	Emitted    int    `json:"emitted,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	Complete   bool   `json:"complete,omitempty"`
}

// SourceInventoryExecutionState records deterministic execution budget state
// for a source-inventory observation. It is operational/coverage metadata, not
// semantic answer text.
type SourceInventoryExecutionState struct {
	Budgeted                 bool `json:"budgeted,omitempty"`
	CandidateBudgetTruncated bool `json:"candidate_budget_truncated,omitempty"`
	AttributesDeferred       bool `json:"attributes_deferred,omitempty"`
	// LensExecutedEmpty is the typed executed-empty lens fact (§29.122 LENSBURN
	// 病B): the source-inventory lens ran successfully and produced zero rows.
	// "Lens executed" is a first-class deterministic tool fact independent of
	// result emptiness; without this carrier an empty shelf is structurally
	// indistinguishable from "lens never ran" and completion gates burn rounds
	// re-demanding an already-executed lens. The flag is engine-minted on the
	// reconciler's empty path only — it is never decoded from model output.
	LensExecutedEmpty bool `json:"lens_executed_empty,omitempty"`
}

// SourceInventorySourceClassCount is the source-class universe matrix attached
// to source-inventory observations. It is computed from repo path roles, not
// model prose, so absence gates can tell whether a zero result covered only
// production sources or also repo-owned auxiliary classes such as fixtures,
// generated files, vendored code, and third-party corpora.
type SourceInventorySourceClassCount struct {
	Role       SourcePathRole                 `json:"role,omitempty"`
	Count      int                            `json:"count,omitempty"`
	Complete   bool                           `json:"complete,omitempty"`
	Samples    []string                       `json:"samples,omitempty"`
	Languages  []SourceInventoryLanguageCount `json:"languages,omitempty"`
	Provenance []string                       `json:"provenance,omitempty"`
}

// SourceInventoryLensQuery is the model/tool-facing query surface for an
// active repo lens. Empty fields mean "use the already typed request context".
// It is intentionally small and typed so callers can drive exploration without
// adding prose classifiers or bespoke inventory modes.
type SourceInventoryLensQuery struct {
	Path              string                `json:"path,omitempty"`
	Scopes            []string              `json:"scopes,omitempty"`
	Roles             []AnswerCandidateRole `json:"roles,omitempty"`
	AttributeRoles    []AnswerCandidateRole `json:"attribute_roles,omitempty"`
	IncludeAttributes bool                  `json:"include_attributes,omitempty"`
	IncludeCounts     bool                  `json:"include_counts,omitempty"`
	TopN              int                   `json:"top_n,omitempty"`
	Offset            int                   `json:"offset,omitempty"`
	Cursor            string                `json:"cursor,omitempty"`
	Query             string                `json:"query,omitempty"`
	Provenance        []string              `json:"provenance,omitempty"`

	// RepoFileCount is the served graph's indexed file count, filled
	// by the repo_map tool so the graph-less renderer can size its
	// default row budget by repo tier. Zero means "no tier info":
	// the renderer keeps its historical defaults.
	RepoFileCount int `json:"repo_file_count,omitempty"`
}

func (o SourceInventoryObservation) IsActive() bool {
	return o.Active && (len(o.Sets) > 0 || len(o.SourceClasses) > 0)
}

// LensExecutedEmpty reports the typed executed-empty lens carrier shape: an
// actively published observation whose lens execution succeeded with zero rows.
// It is deliberately mutually exclusive with IsActive(): as soon as any row or
// source-class content exists, IsActive() is the authority and the empty-shelf
// marker goes inert. This accessor is a SHAPE check only; execution-credential
// consumers must go through SourceInventoryLensExecuted, which additionally
// applies the provenance discipline (analyzer-stage lenses never count).
func (o SourceInventoryObservation) LensExecutedEmpty() bool {
	return o.Active && o.Execution != nil && o.Execution.LensExecutedEmpty &&
		len(o.Sets) == 0 && len(o.SourceClasses) == 0
}

// SourceInventoryObservationSet is one role-bounded member set. Count is a
// machine-checkable invariant and is always normalized to len(Members) by the
// clone/merge/build helpers. Total is the scanned member total or lower bound;
// Members may hold only the current cursor page.
type SourceInventoryObservationSet struct {
	Role     AnswerCandidateRole                `json:"role,omitempty"`
	Complete bool                               `json:"complete,omitempty"`
	Count    int                                `json:"count,omitempty"`
	Total    int                                `json:"total,omitempty"`
	Members  []SourceInventoryObservationMember `json:"members,omitempty"`
}

// SourceInventoryObservationMember is a mechanical repo-lens row: a member,
// file, package, or symbol that the repo map / filesystem inventory can locate.
type SourceInventoryObservationMember struct {
	Name         string              `json:"name,omitempty"`
	Key          string              `json:"key,omitempty"`
	SupportRef   string              `json:"support_ref,omitempty"`
	Note         string              `json:"note,omitempty"`
	SurfaceTerms []string            `json:"surface_terms,omitempty"`
	Provenance   []string            `json:"provenance,omitempty"`
	Role         AnswerCandidateRole `json:"role,omitempty"`
	// SourceClass is the engine-derived repository path lane for this exact
	// row. Older persisted observations may omit it; consumers retain a
	// canonical-path classifier fallback for backward compatibility.
	SourceClass   SourcePathRole                        `json:"source_class,omitempty"`
	Exported      bool                                  `json:"exported,omitempty"`
	File          string                                `json:"file,omitempty"`
	Line          int                                   `json:"line,omitempty"`
	Language      string                                `json:"language,omitempty"`
	CoverageState SourceInventoryCoverageState          `json:"coverage_state,omitempty"`
	Attributes    []SourceInventoryObservationAttribute `json:"attributes,omitempty"`
}

// SourceInventoryObservationAttribute is row-local context, for example
// candidate callable entrypoints under a package/directory member. Attributes
// remain advisory unless the model later cites or emits them as evidence.
type SourceInventoryObservationAttribute struct {
	Name          string                       `json:"name,omitempty"`
	Key           string                       `json:"key,omitempty"`
	SupportRef    string                       `json:"support_ref,omitempty"`
	Note          string                       `json:"note,omitempty"`
	SurfaceTerms  []string                     `json:"surface_terms,omitempty"`
	Role          AnswerCandidateRole          `json:"role,omitempty"`
	SourceClass   SourcePathRole               `json:"source_class,omitempty"`
	Exported      bool                         `json:"exported,omitempty"`
	File          string                       `json:"file,omitempty"`
	Line          int                          `json:"line,omitempty"`
	Language      string                       `json:"language,omitempty"`
	CoverageState SourceInventoryCoverageState `json:"coverage_state,omitempty"`
	Ambiguity     string                       `json:"ambiguity,omitempty"`
	Reason        string                       `json:"reason,omitempty"`
}
