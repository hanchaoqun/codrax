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

// SourceInventoryRequestedField captures the answer fields the user asked the
// inventory to include. This is a typed request-shape carrier, not a rendering
// instruction; downstream may use it to avoid surfacing enum values when the
// user asked only for names and definition locations.
type SourceInventoryRequestedField string

const (
	SourceInventoryFieldName     SourceInventoryRequestedField = "name"
	SourceInventoryFieldLocation SourceInventoryRequestedField = "location"
	SourceInventoryFieldSummary  SourceInventoryRequestedField = "summary"
	SourceInventoryFieldValues   SourceInventoryRequestedField = "values"
	SourceInventoryFieldCount    SourceInventoryRequestedField = "count"
)

func AllSourceInventoryRequestedFields() []SourceInventoryRequestedField {
	return []SourceInventoryRequestedField{
		SourceInventoryFieldName,
		SourceInventoryFieldLocation,
		SourceInventoryFieldSummary,
		SourceInventoryFieldValues,
		SourceInventoryFieldCount,
	}
}

func (f SourceInventoryRequestedField) IsValid() bool {
	for _, declared := range AllSourceInventoryRequestedFields() {
		if f == declared {
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
	for _, role := range p.TargetRoles {
		if role == AnswerCandidateRoleUnknown || seen[role] {
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
