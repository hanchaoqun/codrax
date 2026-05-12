package types

import "strings"

// ImpactScope describes the user-requested boundary for a change-impact
// answer. It is intentionally language-agnostic: Go production files,
// C/C++ headers, ArkTS pages, Cangjie packages, and generated bindings all
// flow through the same scope enum.
type ImpactScope string

const (
	ImpactScopeUnknown    ImpactScope = "unknown"
	ImpactScopeProduction ImpactScope = "production"
	ImpactScopeTest       ImpactScope = "test"
	ImpactScopeAll        ImpactScope = "all"
)

func AllImpactScopes() []ImpactScope {
	return []ImpactScope{
		ImpactScopeUnknown,
		ImpactScopeProduction,
		ImpactScopeTest,
		ImpactScopeAll,
	}
}

func (s ImpactScope) IsValid() bool {
	for _, declared := range AllImpactScopes() {
		if s == declared {
			return true
		}
	}
	return false
}

// ImpactRequestedOutput describes the principal member surface the user
// asked for. It guides prompts and member-surface checks; it is not a
// license for the system to invent missing answer members.
type ImpactRequestedOutput string

const (
	ImpactOutputUnknown ImpactRequestedOutput = "unknown"
	ImpactOutputFiles   ImpactRequestedOutput = "files"
	ImpactOutputSites   ImpactRequestedOutput = "sites"
	ImpactOutputSymbols ImpactRequestedOutput = "symbols"
	ImpactOutputSteps   ImpactRequestedOutput = "steps"
)

func AllImpactRequestedOutputs() []ImpactRequestedOutput {
	return []ImpactRequestedOutput{
		ImpactOutputUnknown,
		ImpactOutputFiles,
		ImpactOutputSites,
		ImpactOutputSymbols,
		ImpactOutputSteps,
	}
}

func (o ImpactRequestedOutput) IsValid() bool {
	for _, declared := range AllImpactRequestedOutputs() {
		if o == declared {
			return true
		}
	}
	return false
}

// ImpactAffectedSiteKind names the structural role a source location plays
// in a change-impact answer. The enum is deliberately cross-language: a
// C++ macro use, an ArkTS decorator-backed UI builder, and a Go struct
// literal all map to a role before final rendering.
type ImpactAffectedSiteKind string

const (
	ImpactSiteDefinition    ImpactAffectedSiteKind = "definition"
	ImpactSiteAssignment    ImpactAffectedSiteKind = "assignment"
	ImpactSiteRead          ImpactAffectedSiteKind = "read"
	ImpactSiteGuard         ImpactAffectedSiteKind = "guard"
	ImpactSiteCall          ImpactAffectedSiteKind = "call"
	ImpactSiteReturn        ImpactAffectedSiteKind = "return"
	ImpactSiteImport        ImpactAffectedSiteKind = "import"
	ImpactSiteConstruction  ImpactAffectedSiteKind = "construction"
	ImpactSiteValidation    ImpactAffectedSiteKind = "validation"
	ImpactSiteSerialization ImpactAffectedSiteKind = "serialization"
	ImpactSiteConfig        ImpactAffectedSiteKind = "config"
	ImpactSiteGenerated     ImpactAffectedSiteKind = "generated"
	ImpactSiteDocumentation ImpactAffectedSiteKind = "documentation"
	ImpactSiteBuild         ImpactAffectedSiteKind = "build"
)

func AllImpactAffectedSiteKinds() []ImpactAffectedSiteKind {
	return []ImpactAffectedSiteKind{
		ImpactSiteDefinition,
		ImpactSiteAssignment,
		ImpactSiteRead,
		ImpactSiteGuard,
		ImpactSiteCall,
		ImpactSiteReturn,
		ImpactSiteImport,
		ImpactSiteConstruction,
		ImpactSiteValidation,
		ImpactSiteSerialization,
		ImpactSiteConfig,
		ImpactSiteGenerated,
		ImpactSiteDocumentation,
		ImpactSiteBuild,
	}
}

func (k ImpactAffectedSiteKind) IsValid() bool {
	for _, declared := range AllImpactAffectedSiteKinds() {
		if k == declared {
			return true
		}
	}
	return false
}

// ChangeImpactProfile is the typed request lane for migration / affected-site
// questions. It says that the answer set is code locations affected by a
// hypothetical or requested change to a named target. It is authored by the
// analyzer LLM from the current request and consumed by deterministic planners;
// deterministic code must not infer this profile by keyword-scanning raw text.
type ChangeImpactProfile struct {
	IsChangeImpact    bool                     `json:"is_change_impact"`
	Target            string                   `json:"target,omitempty"`
	TargetKind        AnswerSubjectKind        `json:"target_kind,omitempty"`
	Scope             ImpactScope              `json:"scope,omitempty"`
	RequestedOutput   ImpactRequestedOutput    `json:"requested_output,omitempty"`
	AffectedSiteKinds []ImpactAffectedSiteKind `json:"affected_site_kinds,omitempty"`
	Confidence        float64                  `json:"confidence,omitempty"`
	Rationale         string                   `json:"rationale,omitempty"`
}

func (p *ChangeImpactProfile) Active() bool {
	return p != nil && p.IsChangeImpact
}

func (p *ChangeImpactProfile) SubjectCandidates() []string {
	if !p.Active() {
		return nil
	}
	var out []string
	if target := strings.TrimSpace(p.Target); target != "" {
		out = append(out, target)
	}
	return out
}

func (p *ChangeImpactProfile) RequestedBroadAffectedSites() bool {
	if !p.Active() {
		return false
	}
	switch p.RequestedOutput {
	case ImpactOutputFiles, ImpactOutputSites, ImpactOutputUnknown:
		return true
	default:
		return false
	}
}
