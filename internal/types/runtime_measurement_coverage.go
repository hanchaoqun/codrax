package types

import (
	"encoding/json"
	"io"
	"strings"
)

// RuntimeMeasurementMemberSet describes the exact rows of one producer-owned
// population. Complete means no rows of THAT population were omitted, not
// that a capture, dependency investigation, or requested answer is complete.
// The model still chooses the population and its member_set presentation role.
type RuntimeMeasurementMemberSet struct {
	PopulationID string   `json:"population_id"`
	RowIDs       []string `json:"row_ids"`
	TotalRows    int      `json:"total_rows"`
	Complete     bool     `json:"complete"`
}

func (m *RuntimeMeasurementMemberSet) valid(rows int) bool {
	if m == nil || m.PopulationID == "" || m.TotalRows < rows || len(m.RowIDs) != rows || (m.Complete && m.TotalRows != rows) {
		return false
	}
	seen := map[string]bool{}
	for _, id := range m.RowIDs {
		if strings.TrimSpace(id) == "" || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func (m *RuntimeMeasurementMemberSet) clone() *RuntimeMeasurementMemberSet {
	if m == nil {
		return nil
	}
	out := *m
	out.RowIDs = append([]string(nil), m.RowIDs...)
	return &out
}

// This private display scope is installed only while admitting a publication.
// It cannot be minted by model JSON, a fabricated BoundTable, or table headings.
type runtimeMeasurementCoverageScope struct {
	path                   string
	windowStart, windowEnd float64
	windowKnown            bool
	targetPID              int
	targetThread           string
}

// CoversMemberSet checks exact data availability, not semantic relevance. It
// deliberately requires the model-owned member_set marker/selection at its
// callsite. A summary, timeline, partial table, or unverified range cannot
// replace an exhaustive member handoff. Full-artifact coverage is not inferred
// from a query's event envelope.
func (t RuntimeMeasurementTable) CoversMemberSet(rm *RequestModel) bool {
	if rm == nil || !t.IsValid() || !t.MemberSet.valid(len(t.Rows)) || !t.MemberSet.Complete || len(t.Rows) == 0 ||
		(t.View != RuntimeMeasurementMembers && t.View != RuntimeMeasurementDistribution) || t.coverageScope == nil {
		return false
	}
	s := t.coverageScope
	if s.path == "" || !s.windowKnown || s.targetPID != 0 || s.targetThread != "" || rm.RuntimeTargetProfile.NamedTarget() {
		return false
	}
	for _, target := range rm.RuntimeTargets {
		if target.Source == "user_explicit" && target.Active() {
			return false
		}
	}
	_, matched := rm.RuntimeArtifactScopeProfile.MatchExplicitTimeWindow(s.windowStart, s.windowEnd)
	return matched
}

// RuntimeMeasurementMemberSetDomain excludes independent source obligations.
// Merely reading a README or having an optional source lane cannot create one.
func RuntimeMeasurementMemberSetDomain(rm *RequestModel, authority RuntimeSourceAnswerAuthoritySnapshot) bool {
	if rm == nil || !authority.UnboundExplanationUsesExternalObservationDomain() || rm.SourceInventoryProfile.Active() {
		return false
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if CanonicalRequestedDimensionSource(hint.Path) != "" && len(hint.RequestedDimensionIndices) > 0 {
			return false
		}
	}
	for _, target := range rm.AnalyzerHints.ExactTargets {
		if textHasPreciseCurrentSourceAnchor(target) {
			return false
		}
	}
	return true
}

// RuntimeMeasurementMemberSetSelections validates model-selected populations;
// it never elects a table merely because one exists. Duplicate selectors are
// one seat and cannot manufacture coverage for independent requested rosters.
func RuntimeMeasurementMemberSetSelections(selected []AnswerRuntimeMeasurementReceipt, contract *RuntimeMeasurementContract, rm *RequestModel) ([]AnswerRuntimeMeasurementReceipt, bool) {
	seen := map[string]bool{}
	windows := map[int]bool{}
	var out []AnswerRuntimeMeasurementReceipt
	for _, receipt := range selected {
		if !BindRuntimeMeasurementReceipt(&receipt, contract) || !receipt.BoundTable.CoversMemberSet(rm) {
			return nil, false
		}
		key := receipt.ObservationID + "\x00" + string(receipt.View)
		if !seen[key] {
			seen[key] = true
			out = append(out, receipt)
			s := receipt.BoundTable.coverageScope
			index, _ := rm.RuntimeArtifactScopeProfile.MatchExplicitTimeWindow(s.windowStart, s.windowEnd)
			windows[index] = true
		}
	}
	return out, len(out) > 0 && len(windows) == len(rm.RuntimeArtifactScopeProfile.ExplicitTimeWindows())
}

// Ambiguous JSON member ownership must not gain new completion authority via
// encoding/json's last-key-wins (including case-folded field aliases). String
// contents are tokens, not scanned as JSON or interpreted as instructions.
func runtimeMeasurementUniqueKeys(raw string) bool {
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 128 {
			return false
		}
		token, err := d.Token()
		if err != nil {
			return false
		}
		delim, compound := token.(json.Delim)
		if !compound {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return false
				}
				name, ok := key.(string)
				if !ok {
					return false
				}
				name = strings.ToLower(name)
				if seen[name] {
					return false
				}
				seen[name] = true
				if !value(depth + 1) {
					return false
				}
			}
		case '[':
			for d.More() {
				if !value(depth + 1) {
					return false
				}
			}
		default:
			return false
		}
		_, err = d.Token()
		return err == nil
	}
	if !value(0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

// One short source for completion schema, query-facing guidance and finalizer
// handoff. Selecting a table never copies values into model-owned aggregates.
const RuntimeMeasurementMemberSetTeaching = "For a requested runtime member set, select its complete published observation_id/view in runtime_measurement_member_sets instead of retyping members into aggregate_facts. Only complete member/distribution rows with matching source, target and exact requested window qualify; summary/timeline, omitted rows and unrelated/source-code rosters do not. The same selector can be used on an answer table with facet_ids:[\"member_set\"]. Interpretation and selection of the requested population remain yours; measurements never prove a root cause."
