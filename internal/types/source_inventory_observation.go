package types

import "strings"

// SourceInventoryCoverageState describes how a source-inventory row should be
// consumed. It is deliberately structural: callers should not infer semantic
// answer intent from these values.
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
	Active        bool                              `json:"active"`
	AdvisoryOnly  bool                              `json:"advisory_only,omitempty"`
	Complete      bool                              `json:"complete,omitempty"`
	Scopes        []string                          `json:"scopes,omitempty"`
	Provenance    []string                          `json:"provenance,omitempty"`
	Lens          []string                          `json:"lens,omitempty"`
	SourceClasses []SourceInventorySourceClassCount `json:"source_classes,omitempty"`
	Page          *SourceInventoryObservationPage   `json:"page,omitempty"`
	Execution     *SourceInventoryExecutionState    `json:"execution,omitempty"`
	Sets          []SourceInventoryObservationSet   `json:"sets,omitempty"`
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
}

// SourceInventorySourceClassCount is the source-class universe matrix attached
// to source-inventory observations. It is computed from repo path roles, not
// model prose, so absence gates can tell whether a zero result covered only
// production sources or also repo-owned auxiliary classes such as fixtures,
// generated files, vendored code, and third-party corpora.
type SourceInventorySourceClassCount struct {
	Role       SourcePathRole `json:"role,omitempty"`
	Count      int            `json:"count,omitempty"`
	Complete   bool           `json:"complete,omitempty"`
	Provenance []string       `json:"provenance,omitempty"`
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

// SourceInventoryObservationFromMutable returns the current durable
// source-inventory view, including Turn A handoff state. Consumers that need
// the complete typed inventory authority should use this instead of reading the
// live MutableState field alone.
func SourceInventoryObservationFromMutable(m *MutableState) SourceInventoryObservation {
	if m == nil {
		return SourceInventoryObservation{}
	}
	observation := m.SourceInventoryObservation()
	if ta := m.TurnAArtifacts(); ta != nil {
		observation = MergeSourceInventoryObservation(observation, ta.SourceInventoryObservation)
		if !observation.IsActive() && ta.SourceInventoryAdvisory.IsActive() {
			observation = SourceInventoryObservationFromAdvisory(ta.SourceInventoryAdvisory)
		}
	}
	return observation
}

// SourceInventoryExactAbsenceNeedsInventoryProof reports whether a
// source-inventory-lane exact absence answer is trying to stand on a generic
// negative query while a typed repo source-class universe says the candidate
// universe still exists and has not been closed by a complete empty
// source-inventory set. This is intentionally structural: it consumes the
// analyzer-emitted SourceInventoryProfile plus SourcePathRole-derived class
// counts, never user prose or model rationale.
func SourceInventoryExactAbsenceNeedsInventoryProof(profile *SourceInventoryProfile, observation SourceInventoryObservation) (string, bool) {
	if profile == nil || !profile.Active() {
		return "", false
	}
	classes := sourceInventoryPositiveSourceClasses(observation.SourceClasses)
	if len(classes) == 0 {
		return "", false
	}
	roles := profile.PrincipalTargetRoles()
	if len(roles) == 0 {
		return "", false
	}
	if sourceInventoryObservationHasCompleteZeroSetsForRoles(observation, roles) {
		return "", false
	}
	return "source_classes=" + SourceInventorySourceClassSummary(classes, 8) +
		"; principal_roles=" + sourceInventoryPrincipalRoleSummary(roles, 8), true
}

// SourceInventorySourceClassSummary renders a compact typed class-count matrix
// for repair hints and audit logs. It is deterministic display only; callers
// decide policy from the typed counts above.
func SourceInventorySourceClassSummary(classes []SourceInventorySourceClassCount, limit int) string {
	classes = sourceInventoryPositiveSourceClasses(classes)
	if len(classes) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(classes) {
		limit = len(classes)
	}
	parts := make([]string, 0, limit+1)
	for i, class := range classes {
		if i >= limit {
			parts = append(parts, "...")
			break
		}
		label := strings.TrimSpace(string(class.Role))
		if label == "" {
			label = string(SourcePathRoleUnknown)
		}
		suffix := ""
		if !class.Complete {
			suffix = "+"
		}
		parts = append(parts, label+":"+itoaSourceInventory(class.Count)+suffix)
	}
	return strings.Join(parts, ", ")
}

// SourceInventoryObservationSet is one role-bounded member set. Count is a
// machine-checkable invariant and is always normalized to len(Members) by the
// clone/merge/build helpers.
type SourceInventoryObservationSet struct {
	Role     AnswerCandidateRole                `json:"role,omitempty"`
	Complete bool                               `json:"complete,omitempty"`
	Count    int                                `json:"count,omitempty"`
	Members  []SourceInventoryObservationMember `json:"members,omitempty"`
}

// SourceInventoryObservationMember is a mechanical repo-lens row: a member,
// file, package, or symbol that the repo map / filesystem inventory can locate.
type SourceInventoryObservationMember struct {
	Name          string                                `json:"name,omitempty"`
	Key           string                                `json:"key,omitempty"`
	SupportRef    string                                `json:"support_ref,omitempty"`
	Note          string                                `json:"note,omitempty"`
	Provenance    []string                              `json:"provenance,omitempty"`
	Role          AnswerCandidateRole                   `json:"role,omitempty"`
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
	Role          AnswerCandidateRole          `json:"role,omitempty"`
	Exported      bool                         `json:"exported,omitempty"`
	File          string                       `json:"file,omitempty"`
	Line          int                          `json:"line,omitempty"`
	Language      string                       `json:"language,omitempty"`
	CoverageState SourceInventoryCoverageState `json:"coverage_state,omitempty"`
	Ambiguity     string                       `json:"ambiguity,omitempty"`
	Reason        string                       `json:"reason,omitempty"`
}

func SourceInventoryObservationFromAdvisory(advisory SourceInventoryAdvisory) SourceInventoryObservation {
	if !advisory.IsActive() {
		return SourceInventoryObservation{}
	}
	out := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: advisory.AdvisoryOnly,
		Complete:     advisory.Complete,
		Scopes:       append([]string(nil), advisory.Scopes...),
		Provenance:   append([]string(nil), advisory.Provenance...),
		Lens:         []string{"members", "symbols", "attributes", "count"},
		Sets:         make([]SourceInventoryObservationSet, 0, len(advisory.Sets)),
	}
	for _, set := range advisory.Sets {
		if len(set.Candidates) == 0 {
			if !set.Complete {
				out.Sets = append(out.Sets, SourceInventoryObservationSet{
					Role:     set.Role,
					Complete: false,
				})
			}
			continue
		}
		obsSet := SourceInventoryObservationSet{
			Role:     set.Role,
			Complete: set.Complete,
			Members:  make([]SourceInventoryObservationMember, 0, len(set.Candidates)),
		}
		for _, candidate := range set.Candidates {
			member := sourceInventoryObservationMemberFromAdvisory(candidate)
			if strings.TrimSpace(member.Name) == "" {
				continue
			}
			obsSet.Members = append(obsSet.Members, member)
		}
		obsSet.Count = len(obsSet.Members)
		if obsSet.Count == 0 {
			continue
		}
		out.Sets = append(out.Sets, obsSet)
	}
	return normalizeSourceInventoryObservation(out)
}

func sourceInventoryObservationMemberFromAdvisory(candidate SourceInventoryAdvisoryCandidate) SourceInventoryObservationMember {
	attrs := make([]SourceInventoryObservationAttribute, 0, len(candidate.Attributes))
	attributeAmbiguity := ""
	if len(candidate.Attributes) > 1 {
		attributeAmbiguity = "one_of_many_candidate_attributes"
	}
	for _, attr := range candidate.Attributes {
		item := SourceInventoryObservationAttribute{
			Name:          attr.Member,
			Key:           attr.Key,
			SupportRef:    attr.SupportRef,
			Note:          attr.Note,
			Role:          attr.Role,
			Exported:      attr.Exported,
			File:          attr.File,
			Line:          attr.Line,
			Language:      attr.Language,
			CoverageState: sourceInventoryObservationCoverage(attr.SupportRef, attr.File),
			Ambiguity:     attributeAmbiguity,
		}
		if attributeAmbiguity != "" {
			item.Reason = "Multiple graph-backed callable attributes are present under this member; the model must choose or disclose ambiguity."
			item.CoverageState = SourceInventoryCoverageAmbiguous
		}
		if strings.TrimSpace(item.Name) != "" {
			attrs = append(attrs, item)
		}
	}
	return SourceInventoryObservationMember{
		Name:          candidate.Member,
		Key:           candidate.Key,
		SupportRef:    candidate.SupportRef,
		Note:          candidate.Note,
		Role:          candidate.Role,
		Exported:      candidate.Exported,
		File:          candidate.File,
		Line:          candidate.Line,
		Language:      candidate.Language,
		CoverageState: sourceInventoryObservationCoverage(candidate.SupportRef, candidate.File),
		Attributes:    attrs,
	}
}

func sourceInventoryObservationCoverage(supportRef, file string) SourceInventoryCoverageState {
	if strings.TrimSpace(supportRef) != "" || strings.TrimSpace(file) != "" {
		return SourceInventoryCoverageObserved
	}
	return SourceInventoryCoverageNeedsRead
}

func CloneSourceInventoryObservation(in SourceInventoryObservation) SourceInventoryObservation {
	out := in
	out.Scopes = append([]string(nil), in.Scopes...)
	out.Provenance = append([]string(nil), in.Provenance...)
	out.Lens = append([]string(nil), in.Lens...)
	out.SourceClasses = cloneSourceInventorySourceClassCounts(in.SourceClasses)
	out.Page = cloneSourceInventoryObservationPage(in.Page)
	out.Execution = cloneSourceInventoryExecutionState(in.Execution)
	if in.Sets != nil {
		out.Sets = make([]SourceInventoryObservationSet, len(in.Sets))
		for i, set := range in.Sets {
			out.Sets[i] = set
			out.Sets[i].Members = cloneSourceInventoryObservationMembers(set.Members)
			out.Sets[i].Count = len(out.Sets[i].Members)
		}
	}
	return normalizeSourceInventoryObservation(out)
}

func MergeSourceInventoryObservation(prior, current SourceInventoryObservation) SourceInventoryObservation {
	if !prior.IsActive() {
		return CloneSourceInventoryObservation(current)
	}
	if !current.IsActive() {
		return CloneSourceInventoryObservation(prior)
	}
	merged := CloneSourceInventoryObservation(prior)
	merged.Active = true
	merged.AdvisoryOnly = prior.AdvisoryOnly && current.AdvisoryOnly
	merged.Complete = prior.Complete && current.Complete
	merged.Scopes = mergeSourceInventoryAdvisoryStrings(merged.Scopes, current.Scopes)
	merged.Provenance = mergeSourceInventoryAdvisoryStrings(merged.Provenance, current.Provenance)
	merged.Lens = mergeSourceInventoryAdvisoryStrings(merged.Lens, current.Lens)
	merged.SourceClasses = mergeSourceInventorySourceClassCounts(merged.SourceClasses, current.SourceClasses)
	if current.Page != nil {
		merged.Page = cloneSourceInventoryObservationPage(current.Page)
	}
	merged.Execution = mergeSourceInventoryExecutionState(merged.Execution, current.Execution)
	byRole := make(map[AnswerCandidateRole]int, len(merged.Sets))
	for i := range merged.Sets {
		byRole[merged.Sets[i].Role] = i
	}
	for _, set := range current.Sets {
		if idx, ok := byRole[set.Role]; ok {
			merged.Sets[idx].Complete = merged.Sets[idx].Complete && set.Complete
			merged.Sets[idx].Members = mergeSourceInventoryObservationMembers(merged.Sets[idx].Members, set.Members)
			merged.Sets[idx].Count = len(merged.Sets[idx].Members)
			continue
		}
		byRole[set.Role] = len(merged.Sets)
		cloned := SourceInventoryObservationSet{
			Role:     set.Role,
			Complete: set.Complete,
			Members:  cloneSourceInventoryObservationMembers(set.Members),
		}
		cloned.Count = len(cloned.Members)
		merged.Sets = append(merged.Sets, cloned)
	}
	return normalizeSourceInventoryObservation(merged)
}

func normalizeSourceInventoryObservation(in SourceInventoryObservation) SourceInventoryObservation {
	in.SourceClasses = normalizeSourceInventorySourceClassCounts(in.SourceClasses)
	if len(in.Sets) == 0 && len(in.SourceClasses) == 0 {
		return SourceInventoryObservation{}
	}
	in.Active = true
	for i := range in.Sets {
		in.Sets[i].Count = len(in.Sets[i].Members)
	}
	return in
}

func cloneSourceInventoryObservationPage(in *SourceInventoryObservationPage) *SourceInventoryObservationPage {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneSourceInventoryExecutionState(in *SourceInventoryExecutionState) *SourceInventoryExecutionState {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func mergeSourceInventoryExecutionState(existing, incoming *SourceInventoryExecutionState) *SourceInventoryExecutionState {
	if existing == nil {
		return cloneSourceInventoryExecutionState(incoming)
	}
	if incoming == nil {
		return cloneSourceInventoryExecutionState(existing)
	}
	return &SourceInventoryExecutionState{
		Budgeted:                 existing.Budgeted || incoming.Budgeted,
		CandidateBudgetTruncated: existing.CandidateBudgetTruncated || incoming.CandidateBudgetTruncated,
		AttributesDeferred:       existing.AttributesDeferred || incoming.AttributesDeferred,
	}
}

func cloneSourceInventorySourceClassCounts(in []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if in == nil {
		return nil
	}
	out := make([]SourceInventorySourceClassCount, len(in))
	for i, item := range in {
		out[i] = item
		out[i].Provenance = append([]string(nil), item.Provenance...)
	}
	return out
}

func mergeSourceInventorySourceClassCounts(existing, incoming []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if len(existing) == 0 {
		return cloneSourceInventorySourceClassCounts(incoming)
	}
	out := cloneSourceInventorySourceClassCounts(existing)
	byRole := make(map[SourcePathRole]int, len(out)+len(incoming))
	for i, item := range out {
		if item.Role != SourcePathRoleUnknown {
			byRole[item.Role] = i
		}
	}
	for _, item := range incoming {
		if item.Role == SourcePathRoleUnknown {
			continue
		}
		if idx, ok := byRole[item.Role]; ok {
			if item.Count > out[idx].Count {
				out[idx].Count = item.Count
			}
			out[idx].Complete = out[idx].Complete && item.Complete
			out[idx].Provenance = mergeSourceInventoryAdvisoryStrings(out[idx].Provenance, item.Provenance)
			continue
		}
		byRole[item.Role] = len(out)
		cloned := item
		cloned.Provenance = append([]string(nil), item.Provenance...)
		out = append(out, cloned)
	}
	return normalizeSourceInventorySourceClassCounts(out)
}

func normalizeSourceInventorySourceClassCounts(in []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	for _, item := range in {
		if item.Role == SourcePathRoleUnknown || item.Count <= 0 {
			continue
		}
		out = append(out, item)
	}
	return out
}

func sourceInventoryPositiveSourceClasses(in []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if len(in) == 0 {
		return nil
	}
	out := make([]SourceInventorySourceClassCount, 0, len(in))
	for _, class := range normalizeSourceInventorySourceClassCounts(cloneSourceInventorySourceClassCounts(in)) {
		if class.Count <= 0 || class.Role == SourcePathRoleUnknown {
			continue
		}
		out = append(out, class)
	}
	return out
}

func sourceInventoryObservationHasCompleteZeroSetsForRoles(observation SourceInventoryObservation, roles []AnswerCandidateRole) bool {
	if len(roles) == 0 {
		return false
	}
	required := map[AnswerCandidateRole]bool{}
	for _, role := range roles {
		if role == AnswerCandidateRoleUnknown {
			continue
		}
		required[role] = true
	}
	if len(required) == 0 {
		return false
	}
	covered := map[AnswerCandidateRole]bool{}
	for _, set := range observation.Sets {
		if !required[set.Role] {
			continue
		}
		if !set.Complete || set.Count != 0 || len(set.Members) != 0 {
			continue
		}
		covered[set.Role] = true
	}
	return len(covered) == len(required)
}

func sourceInventoryPrincipalRoleSummary(roles []AnswerCandidateRole, limit int) string {
	if len(roles) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(roles) {
		limit = len(roles)
	}
	parts := make([]string, 0, limit+1)
	seen := map[AnswerCandidateRole]bool{}
	for _, role := range roles {
		if role == AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		if len(parts) >= limit {
			parts = append(parts, "...")
			break
		}
		parts = append(parts, string(role))
	}
	return strings.Join(parts, ", ")
}

func itoaSourceInventory(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func cloneSourceInventoryObservationMembers(in []SourceInventoryObservationMember) []SourceInventoryObservationMember {
	if in == nil {
		return nil
	}
	out := make([]SourceInventoryObservationMember, len(in))
	for i, member := range in {
		out[i] = member
		out[i].Provenance = append([]string(nil), member.Provenance...)
		out[i].Attributes = append([]SourceInventoryObservationAttribute(nil), member.Attributes...)
	}
	return out
}

func mergeSourceInventoryObservationMembers(existing, incoming []SourceInventoryObservationMember) []SourceInventoryObservationMember {
	if len(existing) == 0 {
		return cloneSourceInventoryObservationMembers(incoming)
	}
	out := cloneSourceInventoryObservationMembers(existing)
	byKey := make(map[string]int, len(out)+len(incoming))
	for i, member := range out {
		if key := sourceInventoryObservationMemberKey(member); key != "" {
			byKey[key] = i
		}
	}
	for _, member := range incoming {
		key := sourceInventoryObservationMemberKey(member)
		if key == "" {
			continue
		}
		if idx, ok := byKey[key]; ok {
			out[idx].Attributes = mergeSourceInventoryObservationAttributes(out[idx].Attributes, member.Attributes)
			out[idx].Provenance = mergeSourceInventoryAdvisoryStrings(out[idx].Provenance, member.Provenance)
			if out[idx].CoverageState == SourceInventoryCoverageUnknown {
				out[idx].CoverageState = member.CoverageState
			}
			continue
		}
		byKey[key] = len(out)
		cloned := member
		cloned.Attributes = append([]SourceInventoryObservationAttribute(nil), member.Attributes...)
		out = append(out, cloned)
	}
	return out
}

func mergeSourceInventoryObservationAttributes(existing, incoming []SourceInventoryObservationAttribute) []SourceInventoryObservationAttribute {
	if len(existing) == 0 {
		return append([]SourceInventoryObservationAttribute(nil), incoming...)
	}
	out := append([]SourceInventoryObservationAttribute(nil), existing...)
	seen := make(map[string]bool, len(out)+len(incoming))
	for _, attr := range out {
		if key := sourceInventoryObservationAttributeKey(attr); key != "" {
			seen[key] = true
		}
	}
	for _, attr := range incoming {
		key := sourceInventoryObservationAttributeKey(attr)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, attr)
	}
	return out
}

func sourceInventoryObservationMemberKey(member SourceInventoryObservationMember) string {
	key := strings.TrimSpace(member.Key)
	if key == "" {
		key = strings.TrimSpace(member.Name)
	}
	if key == "" {
		return ""
	}
	return string(member.Role) + "\x00" + key + "\x00" + strings.TrimSpace(member.File) + "\x00" + strings.TrimSpace(member.SupportRef)
}

func sourceInventoryObservationAttributeKey(attr SourceInventoryObservationAttribute) string {
	key := strings.TrimSpace(attr.Key)
	if key == "" {
		key = strings.TrimSpace(attr.Name)
	}
	if key == "" {
		return ""
	}
	return string(attr.Role) + "\x00" + key + "\x00" + strings.TrimSpace(attr.File) + "\x00" + strings.TrimSpace(attr.SupportRef)
}
