package types

import (
	"fmt"
	"sort"
	"strings"
)

// ExploreLaneRole describes why a lane exists. It scopes exploration only; it
// is not an answer-authority signal.
type ExploreLaneRole string

const (
	ExploreLaneRoleUnknown      ExploreLaneRole = ""
	ExploreLaneRolePrincipal    ExploreLaneRole = "principal"
	ExploreLaneRoleSupport      ExploreLaneRole = "support"
	ExploreLaneRoleVerification ExploreLaneRole = "verification"
)

// ExploreLaneHandoffPolicy describes how the scheduler may treat exact
// overlap. It is advisory scheduling metadata, not a validator gate.
type ExploreLaneHandoffPolicy string

const (
	ExploreLaneHandoffUnknown ExploreLaneHandoffPolicy = ""
	ExploreLaneHandoffOwn     ExploreLaneHandoffPolicy = "own"
	ExploreLaneHandoffSupport ExploreLaneHandoffPolicy = "support"
	ExploreLaneHandoffVerify  ExploreLaneHandoffPolicy = "verify"
	ExploreLaneHandoffDelay   ExploreLaneHandoffPolicy = "delay"
)

// ExploreLanePlan is a typed, derived view over the current request's evidence
// lanes. It consumes existing analyzer/request contracts and does not inspect
// raw user text or model prose.
type ExploreLanePlan struct {
	Lanes []ExploreLane
}

// ExploreLane is one evidence lane that an explorer worker may own.
type ExploreLane struct {
	ID                     string
	Label                  string
	Origin                 AnswerEvidenceOrigin
	InvestigationUnitID    string
	InvestigationUnitLabel string
	FacetIDs               []string
	DimensionLabels        []string
	Role                   ExploreLaneRole
	Coupling               InvestigationCoupling
	HandoffPolicy          ExploreLaneHandoffPolicy
}

func (p ExploreLanePlan) Empty() bool {
	return len(p.Lanes) == 0
}

func (p ExploreLanePlan) LaneByID(id string) (ExploreLane, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ExploreLane{}, false
	}
	for _, lane := range p.Lanes {
		if lane.ID == id {
			return lane, true
		}
	}
	return ExploreLane{}, false
}

// Labels returns compact, declaration-ordered lane labels for UX.
func (p ExploreLanePlan) Labels() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(p.Lanes))
	for _, lane := range p.Lanes {
		label := strings.TrimSpace(lane.Label)
		if label == "" {
			label = string(lane.Origin)
		}
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, label)
	}
	return out
}

// OwnershipKey is the exact typed key used to detect duplicate principal
// ownership. It intentionally does not use fuzzy similarity.
func (l ExploreLane) OwnershipKey() string {
	parts := []string{
		string(l.Origin),
		strings.TrimSpace(l.InvestigationUnitID),
		joinNormalized(l.FacetIDs),
		joinNormalized(l.DimensionLabels),
	}
	return strings.Join(parts, "|")
}

// CompileExploreLanePlan derives evidence lanes from already-typed request
// contracts. It is intentionally conservative: ordinary single-origin,
// shared-context architecture explanations return an empty plan so existing
// scheduling remains unchanged.
func CompileExploreLanePlan(rm RequestModel, answerContract *AnswerContract, presentation AnswerPresentationContract) ExploreLanePlan {
	intent := CompileAnswerIntentContract(rm, answerContract)
	origins := cloneAnswerEvidenceOrigins(intent.Origins)
	if len(origins) == 0 {
		return ExploreLanePlan{}
	}
	investigation := CompileInvestigationPlan(rm, answerContract)
	dimByOrigin := dimensionsByExploreLaneOrigin(presentation.RequestedDimensions)
	shouldMaterialize := len(origins) > 1 ||
		len(investigation.Units) > 1 ||
		len(dimByOrigin) > 0 ||
		rm.HasRuntimeArtifactCurrentVerificationAnchor()
	if !shouldMaterialize {
		return ExploreLanePlan{}
	}

	units := investigation.Units
	if len(units) == 0 {
		units = []InvestigationUnit{{
			ID:       "request",
			Index:    1,
			Role:     InvestigationRolePrincipal,
			Coupling: investigation.Coupling,
		}}
	}
	mcpUnits := units
	if len(units) > 1 {
		// MCP resource/tool calls are structured external observations. When a
		// request has multiple sub-topics, current-source lanes may still need
		// per-topic ownership, but the same MCP resource should not be re-read
		// once per topic. Collapse only the MCP origin; all other origins keep
		// their ordinary unit fanout.
		mcpUnits = []InvestigationUnit{{
			ID:       "request",
			Index:    1,
			Role:     InvestigationRolePrincipal,
			Coupling: investigation.Coupling,
		}}
	}
	var lanes []ExploreLane
	for _, unit := range units {
		unitID := strings.TrimSpace(unit.ID)
		for _, origin := range origins {
			if origin == AnswerEvidenceOriginSystemInference {
				continue
			}
			dims := dimByOrigin[origin]
			if len(dims) == 0 && len(dimByOrigin) > 0 && !originShouldRemainWhenDimensionless(origin, dimByOrigin) {
				continue
			}
			lane := ExploreLane{
				Origin:                 origin,
				InvestigationUnitID:    unitID,
				InvestigationUnitLabel: strings.TrimSpace(unit.Label),
				Label:                  exploreLaneDefaultLabel(origin),
				DimensionLabels:        cloneStrings(dims),
				Role:                   ExploreLaneRolePrincipal,
				Coupling:               coalesceInvestigationCoupling(unit.Coupling, investigation.Coupling),
				HandoffPolicy:          ExploreLaneHandoffOwn,
			}
			lane.ID = stableExploreLaneID(lane, len(lanes)+1)
			lanes = append(lanes, lane)
		}
	}
	if containsExploreLaneOrigin(origins, AnswerEvidenceOriginMCPResource) {
		var filtered []ExploreLane
		for _, lane := range lanes {
			if lane.Origin != AnswerEvidenceOriginMCPResource {
				filtered = append(filtered, lane)
			}
		}
		lanes = filtered
		for _, unit := range mcpUnits {
			lane := ExploreLane{
				Origin:                 AnswerEvidenceOriginMCPResource,
				InvestigationUnitID:    strings.TrimSpace(unit.ID),
				InvestigationUnitLabel: strings.TrimSpace(unit.Label),
				Label:                  exploreLaneDefaultLabel(AnswerEvidenceOriginMCPResource),
				DimensionLabels:        cloneStrings(dimByOrigin[AnswerEvidenceOriginMCPResource]),
				Role:                   ExploreLaneRolePrincipal,
				Coupling:               coalesceInvestigationCoupling(unit.Coupling, investigation.Coupling),
				HandoffPolicy:          ExploreLaneHandoffOwn,
			}
			lane.ID = stableExploreLaneID(lane, len(lanes)+1)
			lanes = append(lanes, lane)
		}
	}
	return ExploreLanePlan{Lanes: dedupeExploreLanes(lanes)}
}

func containsExploreLaneOrigin(origins []AnswerEvidenceOrigin, want AnswerEvidenceOrigin) bool {
	for _, origin := range origins {
		if origin == want {
			return true
		}
	}
	return false
}

func dimensionsByExploreLaneOrigin(dims []RequestedAnswerDimension) map[AnswerEvidenceOrigin][]string {
	out := map[AnswerEvidenceOrigin][]string{}
	for _, dim := range dims {
		label := strings.TrimSpace(dim.Label)
		if label == "" {
			continue
		}
		for _, origin := range exploreLaneOriginsForDimension(dim.Role) {
			out[origin] = appendUniqueString(out[origin], label)
		}
	}
	return out
}

func exploreLaneOriginsForDimension(role RequestedAnswerDimensionRole) []AnswerEvidenceOrigin {
	switch role {
	case RequestedAnswerDimensionDiffClue:
		return []AnswerEvidenceOrigin{AnswerEvidenceOriginVCSDiff, AnswerEvidenceOriginVCSMetadata}
	case RequestedAnswerDimensionCurrentKeyCode:
		return []AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource}
	case RequestedAnswerDimensionEvidenceSource:
		return []AnswerEvidenceOrigin{
			AnswerEvidenceOriginVCSMetadata,
			AnswerEvidenceOriginVCSDiff,
			AnswerEvidenceOriginRuntimeArtifact,
			AnswerEvidenceOriginCommandMeasurement,
			AnswerEvidenceOriginRepoNegativeSearch,
			AnswerEvidenceOriginCrossRepoIndex,
			AnswerEvidenceOriginExternalDocument,
			AnswerEvidenceOriginWebPage,
			AnswerEvidenceOriginMCPResource,
			AnswerEvidenceOriginConnectorResource,
		}
	default:
		return nil
	}
}

func originShouldRemainWhenDimensionless(origin AnswerEvidenceOrigin, dimByOrigin map[AnswerEvidenceOrigin][]string) bool {
	if len(dimByOrigin) == 0 {
		return true
	}
	// Narrative dimensions such as "作用" or "影响" may not map to a single
	// origin. Preserve core origins so the lane planner does not erase a typed
	// request just because a display axis is cross-cutting.
	switch origin {
	case AnswerEvidenceOriginCurrentSource,
		AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginVCSDiff,
		AnswerEvidenceOriginRuntimeArtifact,
		AnswerEvidenceOriginCommandMeasurement:
		return true
	default:
		return len(dimByOrigin[origin]) > 0
	}
}

func stableExploreLaneID(lane ExploreLane, ordinal int) string {
	base := strings.Trim(strings.ToLower(lane.OwnershipKey()), "|")
	base = strings.NewReplacer("|", "_", " ", "_", "/", "_", ":", "_").Replace(base)
	base = strings.Trim(base, "_")
	if base == "" {
		base = fmt.Sprintf("lane_%d", ordinal)
	}
	return base
}

func exploreLaneDefaultLabel(origin AnswerEvidenceOrigin) string {
	switch origin {
	case AnswerEvidenceOriginCurrentSource:
		return "current_source"
	case AnswerEvidenceOriginVCSMetadata:
		return "vcs_history"
	case AnswerEvidenceOriginVCSDiff:
		return "vcs_diff"
	case AnswerEvidenceOriginRuntimeArtifact:
		return "runtime_artifact"
	case AnswerEvidenceOriginCommandMeasurement:
		return "command_measurement"
	case AnswerEvidenceOriginRepoNegativeSearch:
		return "negative_search"
	case AnswerEvidenceOriginCrossRepoIndex:
		return "cross_repo_index"
	case AnswerEvidenceOriginExternalDocument:
		return "external_document"
	case AnswerEvidenceOriginWebPage:
		return "web"
	case AnswerEvidenceOriginMCPResource:
		return "mcp"
	case AnswerEvidenceOriginConnectorResource:
		return "connector"
	default:
		return string(origin)
	}
}

func coalesceInvestigationCoupling(values ...InvestigationCoupling) InvestigationCoupling {
	for _, value := range values {
		if value != InvestigationCouplingUnknown {
			return value
		}
	}
	return InvestigationCouplingIndependent
}

func dedupeExploreLanes(in []ExploreLane) []ExploreLane {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]int{}
	out := make([]ExploreLane, 0, len(in))
	for _, lane := range in {
		key := lane.OwnershipKey()
		if key == "" {
			continue
		}
		if idx, ok := seen[key]; ok {
			out[idx].DimensionLabels = mergeStrings(out[idx].DimensionLabels, lane.DimensionLabels)
			out[idx].FacetIDs = mergeStrings(out[idx].FacetIDs, lane.FacetIDs)
			continue
		}
		lane.DimensionLabels = dedupeSortedStrings(lane.DimensionLabels)
		lane.FacetIDs = dedupeSortedStrings(lane.FacetIDs)
		seen[key] = len(out)
		out = append(out, lane)
	}
	return out
}

func joinNormalized(values []string) string {
	if len(values) == 0 {
		return ""
	}
	values = dedupeSortedStrings(values)
	return strings.Join(values, ",")
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return append([]string(nil), in...)
}

func appendUniqueString(in []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return in
	}
	key := strings.ToLower(value)
	for _, existing := range in {
		if strings.ToLower(strings.TrimSpace(existing)) == key {
			return in
		}
	}
	return append(in, value)
}

func mergeStrings(a, b []string) []string {
	out := cloneStrings(a)
	for _, value := range b {
		out = appendUniqueString(out, value)
	}
	return dedupeSortedStrings(out)
}

func dedupeSortedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]string{}
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; !ok {
			seen[key] = value
		}
	}
	out := make([]string, 0, len(seen))
	for _, value := range seen {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}
