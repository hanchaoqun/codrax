package types

// SourceInventoryTypeUnderlying is an optional structural facet for type
// inventories. It is intentionally language-neutral; language adapters decide
// whether they can prove the facet from parser data.
type SourceInventoryTypeUnderlying string

const (
	SourceInventoryTypeUnderlyingUnknown SourceInventoryTypeUnderlying = "unknown"
	SourceInventoryTypeUnderlyingString  SourceInventoryTypeUnderlying = "string"
	SourceInventoryTypeUnderlyingNumber  SourceInventoryTypeUnderlying = "number"
	SourceInventoryTypeUnderlyingBool    SourceInventoryTypeUnderlying = "bool"
	SourceInventoryTypeUnderlyingEnum    SourceInventoryTypeUnderlying = "enum"
)

func AllSourceInventoryTypeUnderlyings() []SourceInventoryTypeUnderlying {
	return []SourceInventoryTypeUnderlying{
		SourceInventoryTypeUnderlyingUnknown,
		SourceInventoryTypeUnderlyingString,
		SourceInventoryTypeUnderlyingNumber,
		SourceInventoryTypeUnderlyingBool,
		SourceInventoryTypeUnderlyingEnum,
	}
}

func (u SourceInventoryTypeUnderlying) IsValid() bool {
	for _, declared := range AllSourceInventoryTypeUnderlyings() {
		if u == declared {
			return true
		}
	}
	return false
}

// SourceInventoryProfile is the analyzer LLM's typed lane for bounded source
// inventories. It lets deterministic parser/graph code supply membership
// authority without inferring the user's requested role or structural facets
// from raw request text or model prose.
type SourceInventoryProfile struct {
	IsSourceInventory bool                            `json:"is_source_inventory"`
	TargetRoles       []AnswerCandidateRole           `json:"target_roles,omitempty"`
	TypeUnderlying    SourceInventoryTypeUnderlying   `json:"type_underlying,omitempty"`
	RequiresConstSet  bool                            `json:"requires_const_set,omitempty"`
	RequestedFields   []SourceInventoryRequestedField `json:"requested_fields,omitempty"`
	SourceQuotes      []string                        `json:"source_quotes,omitempty"`
	Confidence        float64                         `json:"confidence,omitempty"`
	Rationale         string                          `json:"rationale,omitempty"`
}

func (p *SourceInventoryProfile) Active() bool {
	return p != nil && p.IsSourceInventory && len(p.TargetRoles) > 0
}

func (p *SourceInventoryProfile) RequiresRole(role AnswerCandidateRole) bool {
	if !p.Active() || role == AnswerCandidateRoleUnknown {
		return false
	}
	for _, target := range p.TargetRoles {
		if target == role {
			return true
		}
	}
	return false
}

// PrincipalTargetRoles returns the roles that should become user-visible
// principal members. Some structural facets name additional roles only as a
// qualification rule. For example, a Go "type X string with const set"
// inventory uses constants to prove enum-likeness, but the requested principal
// member is still the exported type name.
func (p *SourceInventoryProfile) PrincipalTargetRoles() []AnswerCandidateRole {
	if !p.Active() {
		return nil
	}
	if p.RequiresConstSet &&
		p.TypeUnderlying != SourceInventoryTypeUnderlyingUnknown &&
		p.RequiresRole(AnswerCandidateRoleType) {
		return []AnswerCandidateRole{AnswerCandidateRoleType}
	}
	out := make([]AnswerCandidateRole, 0, len(p.TargetRoles))
	seen := map[AnswerCandidateRole]bool{}
	dropDisplayAttributes := p.hasStructuralPrincipalRole() && p.requestsPackageLikeDisplayField()
	for _, role := range p.TargetRoles {
		if role == AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		if dropDisplayAttributes && sourceInventoryDisplayAttributeRole(role) {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out
}

func (p *SourceInventoryProfile) RequiresPrincipalRole(role AnswerCandidateRole) bool {
	if role == AnswerCandidateRoleUnknown {
		return false
	}
	for _, target := range p.PrincipalTargetRoles() {
		if target == role {
			return true
		}
	}
	return false
}

// AnswerSubjectForSourceInventoryProfile derives the answer-subject lane from
// a typed source-inventory request. This is intentionally language-neutral: it
// consumes the structural candidate role emitted by the analyzer, not raw
// request words or model prose. Unknown/compound inventories stay passive.
func AnswerSubjectForSourceInventoryProfile(profile *SourceInventoryProfile) (AnswerSubject, bool) {
	if profile == nil || !profile.Active() {
		return AnswerSubject{}, false
	}
	roles := profile.PrincipalTargetRoles()
	if len(roles) != 1 {
		return AnswerSubject{}, false
	}
	var kind AnswerSubjectKind
	var axis string
	switch roles[0] {
	case AnswerCandidateRoleFunction, AnswerCandidateRoleMethod:
		kind = SubjectFunctionName
		axis = string(roles[0]) + " → name"
	case AnswerCandidateRoleType:
		kind = SubjectTypeName
		axis = "type → name"
	case AnswerCandidateRoleField:
		kind = SubjectStructField
		axis = "field → name"
	case AnswerCandidateRoleConfigKey:
		kind = SubjectConfigKey
		axis = "config → key"
	case AnswerCandidateRoleRoute:
		kind = SubjectHandlerRoute
		axis = "route → handler"
	case AnswerCandidateRoleFile:
		kind = SubjectFilePath
		axis = "file → path"
	case AnswerCandidateRoleConfigFile:
		kind = SubjectFilePath
		axis = "config_file → path"
	default:
		return AnswerSubject{}, false
	}
	return AnswerSubject{
		Kind:       kind,
		EntityAxes: []string{axis},
		Confidence: 0.62,
	}, true
}

// NormalizeSourceInventoryRequestedFieldsForAnswerSubject removes contradictory
// requested-field drift after the answer subject has been inferred. A string
// enum "type X string + const set" inventory asks for type identities unless
// the source-inventory profile survives as a non-type subject; in that type
// inventory shape, const/member values are a membership qualifier, not a
// requested visible field.
func NormalizeSourceInventoryRequestedFieldsForAnswerSubject(profile *SourceInventoryProfile, answerSubject AnswerSubject) bool {
	if profile == nil || !profile.Active() || !profile.RequestsField(SourceInventoryFieldValues) {
		return false
	}
	if answerSubject.Kind != SubjectTypeName {
		return false
	}
	if profile.TypeUnderlying != SourceInventoryTypeUnderlyingString || !profile.RequiresConstSet {
		return false
	}
	principalRoles := profile.PrincipalTargetRoles()
	if len(principalRoles) != 1 || principalRoles[0] != AnswerCandidateRoleType {
		return false
	}
	fields := make([]SourceInventoryRequestedField, 0, len(profile.RequestedFields))
	removed := false
	for _, field := range profile.RequestedFields {
		if field == SourceInventoryFieldValues {
			removed = true
			continue
		}
		fields = append(fields, field)
	}
	if !removed {
		return false
	}
	profile.RequestedFields = fields
	return true
}

func (p *SourceInventoryProfile) RequestsField(field SourceInventoryRequestedField) bool {
	if p == nil || field == "" {
		return false
	}
	for _, requested := range p.RequestedFields {
		if requested == field {
			return true
		}
	}
	return false
}

// RequestsSourceText reports whether this inventory asks for source-body
// semantics that typed row metadata alone cannot prove. Mechanical fields such
// as name, location, count, package, module, and namespace can ride on
// parser/source-inventory rows; summaries and values require source text.
func (p *SourceInventoryProfile) RequestsSourceText() bool {
	return p != nil && p.Active() &&
		(p.RequestsField(SourceInventoryFieldSummary) || p.RequestsField(SourceInventoryFieldValues))
}

func (p *SourceInventoryProfile) MechanicalRowsOnly() bool {
	return p != nil && p.Active() && !p.RequestsSourceText()
}
