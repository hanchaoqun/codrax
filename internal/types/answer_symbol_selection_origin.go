package types

import "strings"

// AnswerSymbolSelectionOrigin is run-local producer provenance, not model
// input or a claim of completeness. Only accepted explicit symbol items can
// retain their own member obligation when no proved relation set is present.
type AnswerSymbolSelectionOrigin string

const (
	AnswerSymbolSelectionUnknown               AnswerSymbolSelectionOrigin = ""
	AnswerSymbolSelectionExplicitItems         AnswerSymbolSelectionOrigin = "explicit_items"
	AnswerSymbolSelectionInventoryMaterialized AnswerSymbolSelectionOrigin = "inventory_materialized"
)

// SetEmittedAnswerSymbolsWithOrigin atomically stores the accepted slate and
// its producer provenance. The legacy setter intentionally remains unknown.
func (m *MutableState) SetEmittedAnswerSymbolsWithOrigin(items []AnswerSymbol, claim CompletenessClaim, origin AnswerSymbolSelectionOrigin) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedAnswerSymbols = append([]AnswerSymbol(nil), items...)
	if !claim.IsValid() {
		claim = CompletenessUnknown
	}
	if len(items) == 0 || (origin != AnswerSymbolSelectionExplicitItems && origin != AnswerSymbolSelectionInventoryMaterialized) {
		origin = AnswerSymbolSelectionUnknown
	}
	m.emittedAnswerSymbolCompleteness = claim
	m.emittedAnswerSymbolOrigin = origin
	m.bumpAnswerSurfaceRevisionLocked()
}

// EmittedAnswerSymbolsWithOrigin returns one locked snapshot so a replacement
// cannot pair old members with a new source classification.
func (m *MutableState) EmittedAnswerSymbolsWithOrigin() ([]AnswerSymbol, CompletenessClaim, AnswerSymbolSelectionOrigin) {
	if m == nil {
		return nil, CompletenessUnknown, AnswerSymbolSelectionUnknown
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]AnswerSymbol(nil), m.emittedAnswerSymbols...), m.emittedAnswerSymbolCompleteness, m.emittedAnswerSymbolOrigin
}

// exactStepAnchorSubset preserves source identity, not display wording. A
// renamed, relocated, expanded, or source-less slate cannot inherit provenance.
func exactStepAnchorSubset(selected, accepted []StepSurfaceAnchor) bool {
	if len(selected) == 0 || len(accepted) == 0 {
		return false
	}
	type key struct {
		name, file string
		line, end  int
		kind       AnswerSymbolKind
	}
	keys := make(map[key]bool, len(accepted))
	for _, anchor := range accepted {
		if anchor.Name == "" || anchor.File == "" || anchor.Line <= 0 {
			continue
		}
		keys[key{anchor.Name, anchor.File, anchor.Line, anchor.LineEnd, anchor.Kind}] = true
	}
	for _, anchor := range selected {
		if !keys[key{anchor.Name, anchor.File, anchor.Line, anchor.LineEnd, anchor.Kind}] {
			return false
		}
	}
	return true
}

// An accepted symbol slate owns declaration members, not every nearby fact
// that mentions them. Other evidence stays in the supporting lanes.
func enumerationEvidenceMatchesAcceptedSymbolSlate(plan *AnswerSurfacePlan, item EvidenceItem) bool {
	if plan == nil || !plan.stepBackboneFromAcceptedSymbolSlate {
		return plan != nil && enumerationEvidenceMatchesStepBackbone(plan.StepBackbone, item)
	}
	if item.AnchorKind != AnchorDefinition || item.LineStart <= 0 {
		return false
	}
	name := strings.TrimSpace(item.AnchorSymbol)
	if name == "" {
		name = strings.TrimSpace(item.Subject)
	}
	for _, anchor := range plan.StepBackbone {
		if anchor.Line == item.LineStart && name != "" && name == strings.TrimSpace(anchor.Name) &&
			strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`)) == strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)) {
			return true
		}
	}
	return false
}
