package types

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// EvidenceKind classifies a structured evidence item produced by either
// the LLM investigation notes or the deterministic analysis layers.
type EvidenceKind string

const (
	EvidenceDirect       EvidenceKind = "direct"
	EvidenceConditional  EvidenceKind = "conditional"
	EvidenceRegistration EvidenceKind = "registration"
	EvidenceMechanism    EvidenceKind = "mechanism"
	EvidenceRelationship EvidenceKind = "relationship"
	EvidenceAbsent       EvidenceKind = "absent"
	EvidenceConcrete     EvidenceKind = "concrete_value"
	EvidenceDataflowPath EvidenceKind = "dataflow_path"
	EvidenceConflict     EvidenceKind = "conflict"
	EvidenceUnresolved   EvidenceKind = "unresolved"
	EvidenceTruncated    EvidenceKind = "analysis_truncated"
)

// allEvidenceKinds is the canonical, ordered list of every
// EvidenceKind value. The first six are LLM-emittable via
// emit_evidence; the last five are deterministic-only (written by
// mechanism_scan, dataflow/lower, the concrete-values extractor, and
// the analysis-truncation reporter) and are intentionally rejected
// if the LLM tries to emit them — see IsLLMEmittable below.
var allEvidenceKinds = []EvidenceKind{
	EvidenceDirect,
	EvidenceConditional,
	EvidenceRegistration,
	EvidenceMechanism,
	EvidenceRelationship,
	EvidenceAbsent,
	EvidenceConcrete,
	EvidenceDataflowPath,
	EvidenceConflict,
	EvidenceUnresolved,
	EvidenceTruncated,
}

// AllEvidenceKinds returns the canonical list of every EvidenceKind
// value in stable declaration order. Callers must not mutate the
// returned slice.
func AllEvidenceKinds() []EvidenceKind {
	out := make([]EvidenceKind, len(allEvidenceKinds))
	copy(out, allEvidenceKinds)
	return out
}

// IsLLMEmittable reports whether this EvidenceKind is one the LLM is
// allowed to produce through the emit_evidence tool. The kinds split
// into three families:
//
//  1. Investigation-shape (always emittable): direct / conditional /
//     registration / mechanism / relationship — the standard
//     evidence kinds for ScopeLine / ScopeLineRange / ScopeSection.
//
//  2. Negative-anchor (re-enabled 2026-05+): absent. ScopeNegative
//     gave EvidenceAbsent a typed home — the semantic contradiction
//     that motivated the original deprecation (no concrete file
//     anchor) is resolved by NegativeQuery + NegativeScope replacing
//     line_start as the verification target. Pairing rule enforced
//     by EvidenceItem.ValidateScope: kind=absent REQUIRES
//     scope=negative; emitting absent with any other scope is
//     rejected.
//
//  3. Deterministic-only: concrete_value / dataflow_path / conflict /
//     unresolved / analysis_truncated are written exclusively by Go
//     code (mechanism_scan, dataflow/lower, concrete-values
//     extractor, analysis-truncation reporter). Allowing the LLM to
//     emit them would let it launder unverified claims through a
//     channel whose semantic contract is "I ran a deterministic
//     check".
//
// The emit_evidence tool derives its schema enum and its accept-list
// from this predicate so the canonical list and the tool contract
// cannot drift apart.
func (k EvidenceKind) IsLLMEmittable() bool {
	switch k {
	case EvidenceDirect, EvidenceConditional, EvidenceRegistration,
		EvidenceMechanism, EvidenceRelationship, EvidenceAbsent:
		return true
	}
	return false
}

// LLMEmittableEvidenceKinds returns the subset of AllEvidenceKinds
// for which IsLLMEmittable is true, in canonical order. This is what
// the emit_evidence tool schema's enum is built from.
func LLMEmittableEvidenceKinds() []EvidenceKind {
	out := make([]EvidenceKind, 0, len(allEvidenceKinds))
	for _, k := range allEvidenceKinds {
		if k.IsLLMEmittable() {
			out = append(out, k)
		}
	}
	return out
}

// GroundingStatus is the post-validation verdict attached to each
// EvidenceItem by the grounding layer (internal/tool/ground). Three
// values replace the old binary "grounded vs /ungrounded-suffixed"
// model:
//
//   - Grounded: Tier 1 (line_text) or Tier 2 (symbol_table) accepted
//     the item's (Source, LineStart, AnchorSymbol) triple verbatim.
//   - Recovered: one of the Recovery tiers (R1-R5) found a nearby
//     match and rewrote LineStart and/or Source. The original LLM
//     claim was close enough that the system could repair it. The
//     item is still usable as a citation.
//   - Ungrounded: all tiers failed. The item is preserved as a
//     "lead" but is kept out of citation pools and rendered in a
//     dedicated "Unverified Leads" section. LineStart keeps the
//     LLM's original value so the note carries a human-readable
//     reference, but downstream must not emit it as a confirmed
//     file:line citation.
type GroundingStatus string

const (
	GroundingGrounded   GroundingStatus = "grounded"
	GroundingRecovered  GroundingStatus = "recovered"
	GroundingUngrounded GroundingStatus = "ungrounded"
)

// AnchorKind tells the grounder what KIND of code location the
// EvidenceItem.LineStart points at. Emitted by the LLM through the
// emit_evidence tool, required (not optional) so Tier 2 and the
// recovery tiers have a concrete dispatch key — otherwise the grounder
// would have to guess "is this the definition line? a call site? a
// return statement?" from Subject text, which was the source of the
// same-name-across-receivers misgrounding class.
//
// AnchorImport covers `import`/`use`/`require` statements whose
// AnchorSymbol is typically a package path or alias.
type AnchorKind string

const (
	AnchorDefinition AnchorKind = "definition"
	AnchorCall       AnchorKind = "call"
	AnchorCondition  AnchorKind = "condition"
	AnchorReturn     AnchorKind = "return"
	AnchorAssignment AnchorKind = "assignment"
	AnchorImport     AnchorKind = "import"
)

var allAnchorKinds = []AnchorKind{
	AnchorDefinition, AnchorCall, AnchorCondition,
	AnchorReturn, AnchorAssignment, AnchorImport,
}

// AllAnchorKinds returns the canonical list of every AnchorKind value
// in stable declaration order. Used by the emit_evidence schema to
// drive the required-enum list.
func AllAnchorKinds() []AnchorKind {
	out := make([]AnchorKind, len(allAnchorKinds))
	copy(out, allAnchorKinds)
	return out
}

// EvidenceContextRole classifies how an evidence item should be used
// when the user asked about an exact target. The LLM may recommend a
// role through emit_evidence, but the system validates and can
// downgrade the role before storing it on the item.
type EvidenceContextRole string

const (
	EvidenceContextRoleUnknown          EvidenceContextRole = ""
	EvidenceContextRoleDefining         EvidenceContextRole = "defining"
	EvidenceContextRoleAbsenceSupport   EvidenceContextRole = "absence_support"
	EvidenceContextRoleRelatedContext   EvidenceContextRole = "related_context"
	EvidenceContextRoleIllustrativeOnly EvidenceContextRole = "illustrative_only"
)

var allEvidenceContextRoles = []EvidenceContextRole{
	EvidenceContextRoleUnknown,
	EvidenceContextRoleDefining,
	EvidenceContextRoleAbsenceSupport,
	EvidenceContextRoleRelatedContext,
	EvidenceContextRoleIllustrativeOnly,
}

func AllEvidenceContextRoles() []EvidenceContextRole {
	out := make([]EvidenceContextRole, len(allEvidenceContextRoles))
	copy(out, allEvidenceContextRoles)
	return out
}

func (r EvidenceContextRole) IsValid() bool {
	for _, declared := range allEvidenceContextRoles {
		if r == declared {
			return true
		}
	}
	return false
}

// EvidenceDiagramRole classifies where an evidence item sits inside a
// config-precedence diagram. Like ContextRole, this is an
// LLM-recommended signal that the system validates structurally.
type EvidenceDiagramRole string

const (
	EvidenceDiagramRoleUnknown EvidenceDiagramRole = ""
	EvidenceDiagramRoleDefault EvidenceDiagramRole = "default"
	EvidenceDiagramRoleConfig  EvidenceDiagramRole = "config"
	// EvidenceDiagramRoleYAML is a deprecated alias kept so older
	// tests / saved traces continue to compile; the canonical role is
	// now `config`, which covers YAML/JSON/TOML/INI/... config-file
	// layers instead of only YAML.
	EvidenceDiagramRoleYAML     EvidenceDiagramRole = EvidenceDiagramRoleConfig
	EvidenceDiagramRoleRuntime  EvidenceDiagramRole = "runtime"
	EvidenceDiagramRoleOverride EvidenceDiagramRole = "override"
)

var allEvidenceDiagramRoles = []EvidenceDiagramRole{
	EvidenceDiagramRoleUnknown,
	EvidenceDiagramRoleDefault,
	EvidenceDiagramRoleConfig,
	EvidenceDiagramRoleRuntime,
	EvidenceDiagramRoleOverride,
}

func AllEvidenceDiagramRoles() []EvidenceDiagramRole {
	out := make([]EvidenceDiagramRole, len(allEvidenceDiagramRoles))
	copy(out, allEvidenceDiagramRoles)
	return out
}

func CanonicalEvidenceDiagramRole(raw string) EvidenceDiagramRole {
	role := EvidenceDiagramRole(strings.ToLower(strings.TrimSpace(raw)))
	switch role {
	case EvidenceDiagramRoleYAML:
		return EvidenceDiagramRoleConfig
	default:
		return role
	}
}

func NormalizeEvidenceDiagramRoles(in []EvidenceDiagramRole) []EvidenceDiagramRole {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[EvidenceDiagramRole]bool, len(in))
	var out []EvidenceDiagramRole
	for _, raw := range in {
		role := CanonicalEvidenceDiagramRole(string(raw))
		if role == EvidenceDiagramRoleUnknown || !role.IsValid() || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out
}

func JoinEvidenceDiagramRoles(roles []EvidenceDiagramRole) string {
	normalized := NormalizeEvidenceDiagramRoles(roles)
	if len(normalized) == 0 {
		return ""
	}
	parts := make([]string, 0, len(normalized))
	for _, role := range normalized {
		parts = append(parts, string(role))
	}
	return strings.Join(parts, ", ")
}

func (r EvidenceDiagramRole) IsValid() bool {
	for _, declared := range allEvidenceDiagramRoles {
		if r == declared {
			return true
		}
	}
	return false
}

// GroundingTier names the exact tier that produced a grounded or
// recovered verdict. Rendered in the per-item feedback the
// emit_evidence tool returns, so the LLM can tell "my line was right"
// from "my line was close and the system adjusted it".
type GroundingTier string

const (
	TierLineText         GroundingTier = "line_text"
	TierSymbolTable      GroundingTier = "symbol_table"
	TierFQNameSameFile   GroundingTier = "fqname_same_file"
	TierSnippetFuzzy     GroundingTier = "snippet_fuzzy"
	TierPackageSymbol    GroundingTier = "package_symbol"
	TierNearestCall      GroundingTier = "nearest_call"
	TierNearestCondition GroundingTier = "nearest_condition"
)

// EvidenceScope is the anchor-shape dimension. Required on every
// EvidenceItem; zero-value is invalid. Each scope maps to its own
// grounder path in internal/tool/ground; see scoped_evidence_anchors.md
// for the full design rationale.
type EvidenceScope string

const (
	// ScopeLine — anchors at one specific (file, line). The dominant
	// case for code-citing evidence.
	ScopeLine EvidenceScope = "line"
	// ScopeLineRange — anchors at a multi-line block (struct definition,
	// function body, comment block). Requires LineEnd > LineStart.
	ScopeLineRange EvidenceScope = "line_range"
	// ScopeSection — anchors at a named structural section (YAML
	// top-level group, Go const block). Requires SectionPath.
	ScopeSection EvidenceScope = "section"
	// ScopeFile — anchors a file's identity as a layer (e.g. codrax.yaml
	// as the canonical config layer). Requires FileRoleLabel; LineStart
	// must be 0.
	ScopeFile EvidenceScope = "file"
	// ScopeCrossfile — asserts a cross-file contract verified by query
	// (e.g. "no CLI flag for explore_* group"). Requires CrossfileQuery
	// AND CrossfileAssertion. The grounder re-runs the query.
	ScopeCrossfile EvidenceScope = "crossfile"
	// ScopeNegative — confirms an absence (e.g. "X is not in
	// codrax.yaml"). Requires NegativeQuery AND NegativeScope. Goes
	// hand-in-hand with EvidenceAbsent kind.
	ScopeNegative EvidenceScope = "negative"
)

var allEvidenceScopes = []EvidenceScope{
	ScopeLine, ScopeLineRange, ScopeSection,
	ScopeFile, ScopeCrossfile, ScopeNegative,
}

// AllEvidenceScopes returns the canonical list of every EvidenceScope
// value in stable declaration order. Used by the emit_evidence schema
// to drive the required-enum list.
func AllEvidenceScopes() []EvidenceScope {
	out := make([]EvidenceScope, len(allEvidenceScopes))
	copy(out, allEvidenceScopes)
	return out
}

// IsValid reports whether the scope is one of the six canonical
// values. The empty string is intentionally invalid — every
// EvidenceItem must carry an explicit scope.
func (s EvidenceScope) IsValid() bool {
	for _, declared := range allEvidenceScopes {
		if s == declared {
			return true
		}
	}
	return false
}

// IsLineShaped reports whether this scope produces a line-shaped
// anchor (i.e. has a meaningful LineStart and optional LineEnd).
// Used by Tier-1 floor computations and CGEC closure tracking.
//
// Schema-level scopes (File, Crossfile, Negative) are not line-shaped:
// they describe layer identity, cross-file contracts, or absences,
// not specific code locations.
//
// TRANSITIONAL (Stage 0–7): the empty zero-value scope is treated as
// line-shaped to preserve compatibility with EvidenceItem literals
// constructed before the scope migration completes. Stage 8 removes
// this fallback and makes empty-scope an error.
func (s EvidenceScope) IsLineShaped() bool {
	switch s {
	case ScopeLine, ScopeLineRange, ScopeSection:
		return true
	case "":
		// Transitional only — see doc comment above.
		return true
	}
	return false
}

// FileRoleLabel classifies the role a file plays as a "layer" in the
// answer. Required when EvidenceScope is ScopeFile. Open enum kept
// intentionally small in v1; new labels need explicit registry entry.
type FileRoleLabel string

const (
	// FileRoleConfigCanonical — the canonical user-facing config file
	// (e.g. codrax.yaml). Required path-shape: matches
	// LooksLikeConfigFilePath.
	FileRoleConfigCanonical FileRoleLabel = "config_canonical"
	// FileRoleCLIRegistration — the file where CLI flags are registered
	// (e.g. cmd/root.go). Required path-shape: must contain flag.*Var
	// or cobra.*Flag-like calls (validated heuristically).
	FileRoleCLIRegistration FileRoleLabel = "cli_registration"
	// FileRoleDefaultStruct — the file holding the default-value struct
	// (e.g. internal/types/config.go's DefaultExploreHeuristics).
	FileRoleDefaultStruct FileRoleLabel = "default_struct"
	// FileRoleManifest — package / project manifest file (go.mod,
	// package.json, Cargo.toml, etc.).
	FileRoleManifest FileRoleLabel = "manifest"
	// FileRoleSchema — schema-defining file (proto, openapi, etc.)
	FileRoleSchema FileRoleLabel = "schema"
)

var allFileRoleLabels = []FileRoleLabel{
	FileRoleConfigCanonical, FileRoleCLIRegistration,
	FileRoleDefaultStruct, FileRoleManifest, FileRoleSchema,
}

// AllFileRoleLabels returns the canonical list of every label.
func AllFileRoleLabels() []FileRoleLabel {
	out := make([]FileRoleLabel, len(allFileRoleLabels))
	copy(out, allFileRoleLabels)
	return out
}

// IsValid reports whether the label is one of the canonical 5 values.
func (l FileRoleLabel) IsValid() bool {
	for _, declared := range allFileRoleLabels {
		if l == declared {
			return true
		}
	}
	return false
}

// CrossfileAssertionKind enumerates how the grounder verifies a
// CrossfileQuery's result against the LLM's claim.
type CrossfileAssertionKind string

const (
	// CrossfileExists — assertion passes when at least 1 match across
	// query.Files.
	CrossfileExists CrossfileAssertionKind = "exists"
	// CrossfileForbidden — assertion passes when 0 matches across
	// query.Files. Used to ground "X never registered / referenced".
	CrossfileForbidden CrossfileAssertionKind = "forbidden"
	// CrossfileCountEq — assertion passes when match count exactly
	// equals CrossfileAssertion.Count.
	CrossfileCountEq CrossfileAssertionKind = "count_eq"
)

// CrossfileQuery captures the structured query the LLM is asserting
// about. Files is hard-capped at 5 entries by emit_evidence schema
// validation. Pattern is interpreted as a Go regex.
type CrossfileQuery struct {
	Files   []string `json:"files"`
	Pattern string   `json:"pattern"`
	Context string   `json:"context,omitempty"` // optional: limit to a section / function / type body
}

// CrossfileAssertion is the predicate the LLM commits to about the
// CrossfileQuery's result. The grounder re-runs the query and rejects
// the EvidenceItem if the assertion does not hold — preventing the
// LLM from making cross-file claims it cannot back up.
type CrossfileAssertion struct {
	Kind  CrossfileAssertionKind `json:"kind"`
	Count int                    `json:"count,omitempty"` // only used when Kind == CrossfileCountEq
}

// NegativeScope further qualifies WHERE the absence holds: across the
// whole file, within a line range, within a named section, or against
// a struct's field set.
type NegativeScope string

const (
	NegativeScopeFile         NegativeScope = "file"
	NegativeScopeRange        NegativeScope = "range"
	NegativeScopeSection      NegativeScope = "section"
	NegativeScopeStructFields NegativeScope = "struct_fields"
)

var allNegativeScopes = []NegativeScope{
	NegativeScopeFile, NegativeScopeRange,
	NegativeScopeSection, NegativeScopeStructFields,
}

// AllNegativeScopes returns the canonical list of every NegativeScope.
func AllNegativeScopes() []NegativeScope {
	out := make([]NegativeScope, len(allNegativeScopes))
	copy(out, allNegativeScopes)
	return out
}

// IsValid reports whether the scope is one of the canonical 4 values.
func (n NegativeScope) IsValid() bool {
	for _, declared := range allNegativeScopes {
		if n == declared {
			return true
		}
	}
	return false
}

// NegativeQuery captures the query whose ABSENCE of matches is the
// claim. The grounder runs the query and rejects the EvidenceItem if
// any match is found. Pair with NegativeScope to control where the
// query searches.
type NegativeQuery struct {
	File    string `json:"file"`
	Pattern string `json:"pattern"`
	Section string `json:"section,omitempty"` // when NegativeScope == NegativeScopeSection
}

// EvidenceItem is the normalized, structured representation of a
// single evidence statement that can be carried across agents/stages.
//
// Scope (REQUIRED) selects the anchor model: Line / LineRange /
// Section / File / Crossfile / Negative. Each scope routes through
// its own grounder path and renders differently in citations[].
// Anchor* fields apply to ScopeLine / ScopeLineRange (and partially
// Section). The five scope-specific field bundles
// (LineEnd / SectionPath / FileRoleLabel / Crossfile* / Negative*)
// are mutually exclusive — each scope reads exactly one bundle and
// ignores the rest. emit_evidence enforces this at schema-validation
// time; downstream consumers may panic if a mismatch sneaks through.
type EvidenceItem struct {
	ID          string       `json:"id"`
	Kind        EvidenceKind `json:"kind"`
	Subject     string       `json:"subject,omitempty"`
	Predicate   string       `json:"predicate,omitempty"`
	Object      string       `json:"object,omitempty"`
	Summary     string       `json:"summary,omitempty"`
	Condition   string       `json:"condition,omitempty"`
	Source      string       `json:"source,omitempty"`
	EvidenceRef string       `json:"evidence_ref,omitempty"`
	LineStart   int          `json:"line_start,omitempty"`
	LineEnd     int          `json:"line_end,omitempty"`
	DerivedFrom []string     `json:"derived_from,omitempty"`
	Confidence  float64      `json:"confidence,omitempty"`
	Producer    string       `json:"producer,omitempty"`

	// Role fields: the system-validated usage lanes for exact-target
	// and config-trace questions. Populated from optional emit_evidence
	// hints and/or structural validation after grounding.
	ContextRole          EvidenceContextRole `json:"context_role,omitempty"`
	DiagramRole          EvidenceDiagramRole `json:"diagram_role,omitempty"`
	RequestedDiagramRole EvidenceDiagramRole `json:"requested_diagram_role,omitempty"`

	// Anchor fields: required for ScopeLine / ScopeLineRange; let the
	// grounder dispatch Tier 2 and the recovery tiers without guessing.
	AnchorKind   AnchorKind `json:"anchor_kind,omitempty"`
	AnchorSymbol string     `json:"anchor_symbol,omitempty"`
	OwnerSymbol  string     `json:"owner_symbol,omitempty"`
	Snippet      string     `json:"snippet,omitempty"`

	// Grounding output: filled by the grounder. Downstream renderers
	// branch on GroundingStatus; Tier and Note are human-readable.
	GroundingStatus GroundingStatus `json:"grounding_status,omitempty"`
	GroundingTier   GroundingTier   `json:"grounding_tier,omitempty"`
	GroundingNote   string          `json:"grounding_note,omitempty"`

	// ====== Scope dimension (2026-05+) ======

	// Scope is REQUIRED on every EvidenceItem. Zero-value is invalid
	// and rejected by emit_evidence schema validation; downstream
	// consumers may panic if a mismatch sneaks through.
	Scope EvidenceScope `json:"scope"`

	// SectionPath — required when Scope == ScopeSection. Dot-separated
	// path inside the parsed file (e.g. "explore_*" for a YAML group,
	// "agents.env_recommender" for a nested config key,
	// "ExploreHeuristics" for a Go const block).
	SectionPath string `json:"section_path,omitempty"`

	// FileRoleLabel — required when Scope == ScopeFile. Names the
	// canonical role this file plays as a layer.
	FileRoleLabel FileRoleLabel `json:"file_role_label,omitempty"`

	// CrossfileQuery / CrossfileAssertion — required when
	// Scope == ScopeCrossfile. The grounder re-runs the query and
	// validates the assertion before accepting the item.
	CrossfileQuery     *CrossfileQuery     `json:"crossfile_query,omitempty"`
	CrossfileAssertion *CrossfileAssertion `json:"crossfile_assertion,omitempty"`

	// NegativeQuery / NegativeScope — required when
	// Scope == ScopeNegative. The grounder runs the query and rejects
	// the item if any match is found.
	NegativeQuery *NegativeQuery `json:"negative_query,omitempty"`
	NegativeScope NegativeScope  `json:"negative_scope,omitempty"`
}

// ValidateScope enforces the per-scope required-field invariants on
// an EvidenceItem. Returns nil when every required field for the
// item's Scope is set; returns a descriptive error otherwise.
//
// emit_evidence calls this at schema-validation time; downstream
// consumers (CGEC closure, citation builder, grounder dispatch) may
// also call it as a defense-in-depth check before processing.
func (e EvidenceItem) ValidateScope() error {
	if !e.Scope.IsValid() {
		return fmt.Errorf("evidence: scope is required and must be one of %v; got %q",
			AllEvidenceScopes(), e.Scope)
	}
	switch e.Scope {
	case ScopeLine:
		if e.Source == "" || e.LineStart <= 0 || e.AnchorKind == "" {
			return fmt.Errorf("evidence: scope=line requires source, line_start>0, anchor_kind")
		}
	case ScopeLineRange:
		if e.Source == "" || e.LineStart <= 0 || e.LineEnd <= e.LineStart {
			return fmt.Errorf("evidence: scope=line_range requires source, line_start>0, line_end>line_start")
		}
	case ScopeSection:
		if e.Source == "" || strings.TrimSpace(e.SectionPath) == "" {
			return fmt.Errorf("evidence: scope=section requires source and non-empty section_path")
		}
	case ScopeFile:
		if e.Source == "" || !e.FileRoleLabel.IsValid() {
			return fmt.Errorf("evidence: scope=file requires source and a valid file_role_label")
		}
		if e.LineStart != 0 {
			return fmt.Errorf("evidence: scope=file must have line_start=0 (file-identity anchor); got %d", e.LineStart)
		}
	case ScopeCrossfile:
		if e.CrossfileQuery == nil || e.CrossfileAssertion == nil {
			return fmt.Errorf("evidence: scope=crossfile requires crossfile_query and crossfile_assertion")
		}
		if len(e.CrossfileQuery.Files) == 0 || len(e.CrossfileQuery.Files) > 5 {
			return fmt.Errorf("evidence: scope=crossfile requires 1<=len(files)<=5; got %d", len(e.CrossfileQuery.Files))
		}
		if strings.TrimSpace(e.CrossfileQuery.Pattern) == "" {
			return fmt.Errorf("evidence: scope=crossfile requires non-empty pattern")
		}
	case ScopeNegative:
		if e.NegativeQuery == nil || !e.NegativeScope.IsValid() {
			return fmt.Errorf("evidence: scope=negative requires negative_query and a valid negative_scope")
		}
		if e.NegativeQuery.File == "" || strings.TrimSpace(e.NegativeQuery.Pattern) == "" {
			return fmt.Errorf("evidence: scope=negative requires negative_query.file and pattern")
		}
		if e.Kind != EvidenceAbsent {
			return fmt.Errorf("evidence: scope=negative requires kind=absent; got kind=%q", e.Kind)
		}
		if e.NegativeScope == NegativeScopeSection && strings.TrimSpace(e.NegativeQuery.Section) == "" {
			return fmt.Errorf("evidence: scope=negative with negative_scope=section requires negative_query.section")
		}
	}
	return nil
}

// EvidenceCountsTowardTier1Floor reports whether an evidence item
// should contribute to the Tier-1 proven-ratio denominator used by
// explorer completion and the orchestrator's pre-finalize gate.
//
// The floor is meant to protect citation-bearing answer anchors:
// facts the finalizer may actually need to cite. Auxiliary lanes that
// the system has already classified as non-defining context
// (illustrative examples, absence-support prose, unresolved exact-hit
// mentions) should not bloat the denominator and force an otherwise
// sufficient investigation to reopen just to repair evidence the
// pipeline itself says must remain context only.
//
// 2026-05+ scope axis: schema-level scopes (File / Crossfile /
// Negative) describe layer identity, cross-file contracts, or
// confirmed absences — none of these are line-anchored facts that
// read_file can verify per-line. They are real evidence for answer
// surface but should not bloat the Tier-1 denominator. ScopeLine /
// ScopeLineRange / ScopeSection remain line-shaped (per
// EvidenceScope.IsLineShaped) and continue to count.
func EvidenceCountsTowardTier1Floor(ev EvidenceItem) bool {
	if ev.Kind == EvidenceUnresolved {
		return false
	}
	switch ev.ContextRole {
	case EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleAbsenceSupport:
		return false
	}
	if !ev.Scope.IsLineShaped() {
		return false
	}
	return true
}

// ExactResolutionRequiredContextFiles returns the live same-scope file
// set for exact-absence related-context closure when the contract
// explicitly asked for grounded same-family/same-directory context.
// The helper lives in types so explorer completion, emit_* tools, and
// the orchestrator all consume the same authority instead of
// re-deriving required files independently.
func ExactResolutionRequiredContextFiles(contract *ExactResolutionContract, mutable *MutableState) []string {
	if contract == nil || mutable == nil || !contract.AllowAbsence {
		return nil
	}
	switch contract.RelatedContextPolicy {
	case ExactContextSameFamilyGrounded, ExactContextSameDirectoryGrounded:
		return mutable.ExactContextRequiredFiles()
	default:
		return nil
	}
}

// EvidenceCountsTowardTier1FloorInContext reports whether an evidence
// item should contribute to the Tier-1 denominator for the current
// completion context. The base rule excludes unresolved /
// illustrative-only / absence-support items. Exact-absence closures
// tighten further: nearby related_context items count only when they
// are strong enough to participate in closure as real same-scope
// proof, rather than being prose-only nearby context the answer may
// mention but need not cite.
func EvidenceCountsTowardTier1FloorInContext(
	ev EvidenceItem,
	contract *ExactResolutionContract,
	scenario Scenario,
	stableAbsent bool,
	requiredFiles []string,
	rm RequestModel,
) bool {
	if !EvidenceCountsTowardTier1Floor(ev) {
		return false
	}
	if ev.ContextRole == EvidenceContextRoleRelatedContext && IsScalarSourceLiteralLookup(rm) {
		// Scalar source-literal lookups already have a single answer-grade
		// anchor lane. Nearby related_context can still enrich summary
		// prose, but it should not bloat the Tier-1 denominator or block
		// closure after the defining literal is grounded.
		return false
	}
	if !stableAbsent || contract == nil || len(contract.Targets) == 0 {
		return true
	}
	if ev.ContextRole != EvidenceContextRoleRelatedContext {
		return true
	}
	return ExactResolutionRelatedContextProofAllowedInFiles(contract, scenario, true, ev, requiredFiles)
}

// FlowFindingDigest is the compact, stage-safe output of the dataflow
// engine. It intentionally omits the full graph and preserves only the
// user-facing path plus the evidence IDs needed for replay.
type FlowFindingDigest struct {
	ID                string   `json:"id"`
	Path              []string `json:"path,omitempty"`
	Conditions        []string `json:"conditions,omitempty"`
	Sources           []string `json:"sources,omitempty"`
	Sinks             []string `json:"sinks,omitempty"`
	Hops              []string `json:"hops,omitempty"`
	EvidenceIDs       []string `json:"evidence_ids,omitempty"`
	UnsupportedReason string   `json:"unsupported_reason,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
}

// StableEvidenceID returns a deterministic ID derived from the
// semantically relevant evidence fields. Callers should use this for
// dedupe/merge so that the same evidence coming from main/sub
// explorers or deterministic passes coalesces cleanly.
//
// 2026-05+ — signature takes the full EvidenceItem so scope-specific
// fields (SectionPath / FileRoleLabel / CrossfileQuery / NegativeQuery)
// participate in the hash. Without this, two distinct schema-level
// items (e.g. codrax.yaml as ConfigCanonical vs codrax.yaml as
// Schema) would collide on the old (kind, subject, source, ...)
// tuple and one would silently overwrite the other during dedupe.
func StableEvidenceID(item EvidenceItem) string {
	h := fnv.New64a()
	parts := []string{
		string(item.Kind),
		string(item.Scope),
		item.Subject,
		item.Predicate,
		item.Object,
		item.Condition,
		item.Source,
		fmt.Sprintf("%d:%d", item.LineStart, item.LineEnd),
	}
	switch item.Scope {
	case ScopeSection:
		parts = append(parts, "section="+item.SectionPath)
	case ScopeFile:
		parts = append(parts, "file_role="+string(item.FileRoleLabel))
	case ScopeCrossfile:
		if item.CrossfileQuery != nil {
			parts = append(parts,
				"xfiles="+strings.Join(item.CrossfileQuery.Files, ","),
				"xpat="+item.CrossfileQuery.Pattern,
				"xctx="+item.CrossfileQuery.Context)
		}
		if item.CrossfileAssertion != nil {
			parts = append(parts,
				"xassert="+string(item.CrossfileAssertion.Kind),
				fmt.Sprintf("xcount=%d", item.CrossfileAssertion.Count))
		}
	case ScopeNegative:
		if item.NegativeQuery != nil {
			parts = append(parts,
				"nfile="+item.NegativeQuery.File,
				"npat="+item.NegativeQuery.Pattern,
				"nsec="+item.NegativeQuery.Section)
		}
		parts = append(parts, "nscope="+string(item.NegativeScope))
	}
	_, _ = h.Write([]byte(strings.Join(parts, "\x1f")))
	return fmt.Sprintf("ev-%x", h.Sum64())
}

// IsCitable reports whether this evidence item carries an anchor
// downstream stages (extractor, finalizer) can trust as a citation
// target without re-grounding.
//
// 2026-05+ — semantics widened beyond line-only. The positive set:
//
//   - GroundingGrounded items of any scope (including schema-level
//     File / Crossfile / Negative scopes — their grounders verified
//     the layer / contract / absence claim end-to-end before stamping
//     Grounded).
//   - Legacy items with empty GroundingStatus (pre-session-5
//     deterministic concrete_value emitters) count as citable for
//     ScopeLine / ScopeLineRange / ScopeSection — they are
//     deterministic facts. Schema-level scopes with empty
//     GroundingStatus are NOT citable: those scopes are LLM-emit-only
//     and require the grounder to confirm.
//
// Recovered and Ungrounded items in any scope are NOT citable: the
// finalizer's strict gate may reject them.
func (e EvidenceItem) IsCitable() bool {
	switch e.GroundingStatus {
	case GroundingGrounded:
		return true
	case GroundingRecovered, GroundingUngrounded:
		return false
	}
	// Empty status — only line-shaped scopes get the legacy
	// deterministic-emitter pass.
	return e.Scope.IsLineShaped()
}

// DisplayLocation renders the "file:line" marker appropriate for a
// given strictness:
//
//   - strict=false: historical behaviour. Full "file:line" when Source
//     and LineStart are set; just "file" otherwise.
//   - strict=true: file:line ONLY when IsCitable; recovered/ungrounded
//     items show file alone (line suppressed). Used at the Turn B
//     / finalize boundary so downstream LLMs cannot pick up a
//     recovered line number that finalize-time grounding will later
//     reject.
//
// Empty source returns "".
func (e EvidenceItem) DisplayLocation(strict bool) string {
	if e.Source == "" {
		return ""
	}
	if strict && !e.IsCitable() {
		return e.Source
	}
	if e.LineStart > 0 {
		if e.LineEnd > e.LineStart {
			return fmt.Sprintf("%s:%d-%d", e.Source, e.LineStart, e.LineEnd)
		}
		return fmt.Sprintf("%s:%d", e.Source, e.LineStart)
	}
	return e.Source
}

// CompletenessClaim is the set-level authority the producer of an
// AnswerSymbol slate attaches to that slate. It answers "how
// authoritative is this list?" — a question that extractAnswerSymbols
// (the pre-P2.1 selection layer) could never answer and that the
// downstream rendering layer (context/builder.go §Answer Symbols)
// silently defaulted to "complete", producing the UNRESOLVED #1 bug
// where a partial LLM-derived allowlist was sold to the finalizer as
// a verified-complete answer. See memory/project_p2_1_session_1_shipped.md.
//
// The three claims form a strict authority ladder:
//
//   - CompletenessComplete: these are ALL the answers. The finalizer
//     runs its Translation mode prompt with "MUST NOT add or remove
//     symbols" directive. Safe only when the upstream producer has
//     structurally validated that no answers were missed.
//
//   - CompletenessLowerBound: these are confirmed present, but more
//     may exist. The finalizer runs a softened prompt: "MUST include
//     at least these names; MAY add more if evidence supports them."
//     This is the honest default when a partial allowlist is the best
//     we have — it preserves the floor without forbidding the ceiling.
//
//   - CompletenessUnknown: no authority at all. The finalizer drops
//     the answer-symbols section entirely and falls back to the
//     shape-based prompt (step_list / explanation / etc.). Zero-value
//     of the type so an un-set field naturally means "no claim".
//
// The enum is closed — the emit_answer_symbol schema in P2.1 P9 will
// reject any other string. CompletenessUnknown is the zero value so
// that legacy code paths that do not set the field degrade safely.
type CompletenessClaim string

const (
	// CompletenessUnknown is the zero value. It means the producer
	// either did not run a cardinality check at all, or ran one and
	// could not reach a definitive verdict. The rendering layer must
	// treat the answer-symbol slate as non-authoritative and fall back
	// to the shape-based prompt. Leaving the field at zero value is
	// always safe; it is the "fail closed" default for P2.1's
	// completeness contract.
	CompletenessUnknown CompletenessClaim = ""

	// CompletenessComplete asserts that the attached slate lists every
	// symbol that answers the question. The finalizer runs Translation
	// mode with "MUST NOT add or remove symbols". Producers MUST have
	// run a structural completeness check (Turn A's deterministic
	// extraction with a ≥1 baseline, or Turn B's emit_answer_symbol
	// claim validated against Turn A's TerminalEvidenceCount and the
	// AnalysisIR.AnswerContract.MustInclude cross-ref) before writing
	// this value. Writing it without the check is forbidden by the
	// P9 schema validator.
	CompletenessComplete CompletenessClaim = "complete"

	// CompletenessLowerBound asserts that the attached slate is a
	// proper floor on the true answer set — every listed symbol is
	// confirmed present, but the slate may be missing additional
	// symbols that also answer the question. The finalizer runs the
	// softened Translation prompt that preserves the floor without
	// forbidding the ceiling. This is the right claim when
	// investigation was bounded (read-file budget hit, grep recall
	// partial, etc.) but produced high-confidence matches.
	CompletenessLowerBound CompletenessClaim = "lower_bound"
)

// IsValid reports whether c is one of the three defined CompletenessClaim
// values. Used by the emit_answer_symbol schema validator and by
// applyStageOutput's merge rule so an invalid value cannot leak into
// BusContext.
func (c CompletenessClaim) IsValid() bool {
	switch c {
	case CompletenessUnknown, CompletenessComplete, CompletenessLowerBound:
		return true
	}
	return false
}

// AnswerSymbol is the structured, deterministic form of a single
// answer. Produced by Turn B's extractor via emit_answer_symbol,
// it is the bridge between the deterministic pipeline (which
// identifies the correct symbol) and the finalizer (which must
// render it as prose without adding or removing names).
type AnswerSymbol struct {
	Name      string           `json:"name"`
	File      string           `json:"file,omitempty"`
	Line      int              `json:"line,omitempty"`
	Chain     string           `json:"chain"`               // full chain text that yielded this symbol
	Kind      AnswerSymbolKind `json:"kind"`                // see answer_symbol_kind.go for the closed taxonomy
	Rationale string           `json:"rationale,omitempty"` // optional: why this terminal was picked
}

// AnswerChain is the typed ranked-and-scored answer-relevance envelope
// around an EvidenceItem. Produced by identifyAnswerChains (pure Go,
// deterministic, no LLM) from the explorer's structuredEvidence pool.
//
// Design rationale: identifyAnswerChains used to return []string where
// each string was a pre-flattened render of the underlying evidence
// (ev.Summary plus a " (file:line)" suffix). That violated the
// architecture principle "prose only at the LLM boundary" because the
// flattening happened in Go mid-pipeline, losing the structured fields
// for every downstream consumer. The typed form keeps the underlying
// EvidenceItem intact and defers rendering to context/builder.go's
// prompt assembly step (the single legal flatten point).
//
// Fields:
//   - Item: the underlying EvidenceItem (all structured fields —
//     Subject / Predicate / Object / Source / LineStart / Summary /
//     Kind / Confidence). Downstream consumers that need a specific
//     field access Item directly instead of parsing a display string.
//   - Score: the relevance score identifyAnswerChains computed. Kept
//     in the envelope because (a) it is committed API semantics
//     ("how well does this chain answer the question"), (b) debug
//     / eval tooling wants to see why the ranking came out this way,
//     (c) recomputing is expensive.
//   - StrictOK: whether the chain passed the L0-1 terminal + origin
//     predicates. The strict subset is used to compute β for Turn B's
//     cardinality validator. Non-strict chains are retained in the
//     slate as Ground Truth fallback but never contribute to β.
type AnswerChain struct {
	Item     EvidenceItem `json:"item"`
	Score    float64      `json:"score"`
	StrictOK bool         `json:"strict_ok"`
}

// AnswerChainItems flattens AnswerChains into their EvidenceItem
// payloads so downstream consumers can reason over a single evidence
// pool without reparsing chain envelopes.
func AnswerChainItems(chains []AnswerChain) []EvidenceItem {
	if len(chains) == 0 {
		return nil
	}
	out := make([]EvidenceItem, 0, len(chains))
	for _, chain := range chains {
		out = append(out, chain.Item)
	}
	return out
}

// ExactResolutionSurfaceEvidencePool merges the evidence channels the
// final answer stages consume today:
//   - explorer/tool-emitted evidence in MutableState
//   - deterministic / Turn-A evidence snapshots
//   - answer-chain envelopes compiled from that evidence
//
// The exact-resolution prompt builder, repair seeds, and final
// validators must agree on the same answer-grade evidence surface.
// Keeping that pool centralized prevents one stage from treating an
// anchor as available while another silently ignores it.
func ExactResolutionSurfaceEvidencePool(emitted, evidence []EvidenceItem, chains []AnswerChain) []EvidenceItem {
	merged := make(map[string]EvidenceItem)
	order := make([]string, 0, len(emitted)+len(evidence)+len(chains))
	appendItem := func(item EvidenceItem) {
		if item.ID == "" {
			item.ID = StableEvidenceID(item)
		}
		if existing, ok := merged[item.ID]; ok {
			merged[item.ID] = mergeExactResolutionSurfaceEvidence(existing, item)
			return
		}
		merged[item.ID] = item
		order = append(order, item.ID)
	}
	for _, group := range [][]EvidenceItem{emitted, evidence, AnswerChainItems(chains)} {
		for _, item := range group {
			appendItem(item)
		}
	}
	if len(order) == 0 {
		return nil
	}
	out := make([]EvidenceItem, 0, len(order))
	for _, id := range order {
		out = append(out, merged[id])
	}
	return out
}

func mergeExactResolutionSurfaceEvidence(dst, src EvidenceItem) EvidenceItem {
	if dst.Subject == "" {
		dst.Subject = src.Subject
	}
	if dst.Predicate == "" {
		dst.Predicate = src.Predicate
	}
	if dst.Object == "" {
		dst.Object = src.Object
	}
	if dst.Condition == "" {
		dst.Condition = src.Condition
	}
	if dst.AnchorSymbol == "" {
		dst.AnchorSymbol = src.AnchorSymbol
	}
	if dst.OwnerSymbol == "" {
		dst.OwnerSymbol = src.OwnerSymbol
	}
	if dst.AnchorKind == "" {
		dst.AnchorKind = src.AnchorKind
	}
	if dst.Snippet == "" {
		dst.Snippet = src.Snippet
	}
	if dst.Summary == "" {
		dst.Summary = src.Summary
	}
	if dst.Source == "" {
		dst.Source = src.Source
	}
	if dst.LineStart <= 0 {
		dst.LineStart = src.LineStart
	}
	if dst.LineEnd <= 0 {
		dst.LineEnd = src.LineEnd
	}
	if merged := mergeExactResolutionContextRole(dst.ContextRole, src.ContextRole); merged != EvidenceContextRoleUnknown || dst.ContextRole == EvidenceContextRoleUnknown {
		dst.ContextRole = merged
	}
	if dst.DiagramRole == EvidenceDiagramRoleUnknown {
		dst.DiagramRole = src.DiagramRole
	}
	if dst.RequestedDiagramRole == EvidenceDiagramRoleUnknown {
		dst.RequestedDiagramRole = src.RequestedDiagramRole
	}
	if dst.GroundingStatus == "" || dst.GroundingStatus == GroundingUngrounded {
		if src.GroundingStatus == GroundingGrounded || src.GroundingStatus == GroundingRecovered {
			dst.GroundingStatus = src.GroundingStatus
			dst.GroundingTier = src.GroundingTier
			dst.GroundingNote = src.GroundingNote
		}
	}
	if dst.Confidence < src.Confidence {
		dst.Confidence = src.Confidence
	}
	if dst.Producer == "" {
		dst.Producer = src.Producer
	}
	if dst.EvidenceRef == "" {
		dst.EvidenceRef = src.EvidenceRef
	}
	return dst
}

func mergeExactResolutionContextRole(dst, src EvidenceContextRole) EvidenceContextRole {
	if dst == EvidenceContextRoleUnknown {
		return src
	}
	if src == EvidenceContextRoleUnknown {
		return dst
	}
	if exactResolutionContextRoleStrictness(src) > exactResolutionContextRoleStrictness(dst) {
		return src
	}
	return dst
}

func exactResolutionContextRoleStrictness(role EvidenceContextRole) int {
	switch role {
	case EvidenceContextRoleIllustrativeOnly:
		return 4
	case EvidenceContextRoleAbsenceSupport:
		return 3
	case EvidenceContextRoleRelatedContext:
		return 2
	case EvidenceContextRoleDefining:
		return 1
	default:
		return 0
	}
}

// MergeAnswerChains combines multiple AnswerChain slices into one,
// preserving first-seen order and dropping duplicates by the chain
// identity tuple (Summary, Source, LineStart, Subject, Predicate,
// Object). The key matches identifyAnswerChains's own dedup key so a
// merged slice has the same cardinality as a freshly-produced one.
// Used by the orchestrator to deduplicate BusContext.AnswerChains on
// stage self-loops where the explorer's ParseOutput re-emits the
// full snapshot each run.
func MergeAnswerChains(groups ...[]AnswerChain) []AnswerChain {
	seen := make(map[string]struct{})
	var out []AnswerChain
	for _, g := range groups {
		for _, c := range g {
			key := fmt.Sprintf("%s|%s|%d|%s|%s|%s",
				c.Item.Summary, c.Item.Source, c.Item.LineStart,
				c.Item.Subject, c.Item.Predicate, c.Item.Object)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, c)
		}
	}
	return out
}

// MergeAnswerSymbols combines multiple AnswerSymbol slices into one,
// preserving first-seen order. Dedup key is a composite of Name +
// File + Line — identity fields the finalizer's translation-mode
// renderer uses to emit a unique bullet. Chain / Kind / Rationale
// are treated as metadata that may vary run-to-run without implying
// a different symbol.
func MergeAnswerSymbols(groups ...[]AnswerSymbol) []AnswerSymbol {
	seen := make(map[string]struct{})
	var out []AnswerSymbol
	for _, g := range groups {
		for _, s := range g {
			key := s.Name + "\x1f" + s.File + "\x1f" + itoa(s.Line)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// itoa is a tiny in-package int-to-string to avoid importing strconv
// in this file just for the dedup key builder.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
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

// StableFlowFindingID returns a deterministic ID for a compact
// dataflow finding based on its path/condition shape.
func StableFlowFindingID(path, conditions, sources, sinks []string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.Join([]string{
		strings.Join(path, "\x1e"),
		strings.Join(conditions, "\x1e"),
		strings.Join(sources, "\x1e"),
		strings.Join(sinks, "\x1e"),
	}, "\x1f")))
	return fmt.Sprintf("flow-%x", h.Sum64())
}
