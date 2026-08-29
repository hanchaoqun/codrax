package types

import (
	"fmt"
	"strings"
)

// InvestigationRole describes how a unit should be treated by scheduling and
// display. It is not an answer-authority signal; evidence/finalizer contracts
// remain responsible for deciding what appears in the final answer.
type InvestigationRole string

const (
	InvestigationRoleUnknown      InvestigationRole = ""
	InvestigationRolePrincipal    InvestigationRole = "principal"
	InvestigationRoleSupport      InvestigationRole = "support"
	InvestigationRoleVerification InvestigationRole = "verification"
)

// InvestigationCoupling describes the relationship between units. It is a
// typed planning hint, not permission to override model-authored structure.
type InvestigationCoupling string

const (
	InvestigationCouplingUnknown       InvestigationCoupling = ""
	InvestigationCouplingIndependent   InvestigationCoupling = "independent"
	InvestigationCouplingSharedContext InvestigationCoupling = "shared_context"
	InvestigationCouplingSequential    InvestigationCoupling = "sequential"
	InvestigationCouplingComparative   InvestigationCoupling = "comparative"
)

// InvestigationAnswerPartition distinguishes analyzer work decomposition from
// user-requested answer sections.
type InvestigationAnswerPartition string

const (
	InvestigationAnswerPartitionNone                  InvestigationAnswerPartition = ""
	InvestigationAnswerPartitionAnalyzerDecomposition InvestigationAnswerPartition = "analyzer_decomposition"
	InvestigationAnswerPartitionUserBucket            InvestigationAnswerPartition = "user_bucket"
)

// InvestigationUnitSource records which typed IR surface produced a unit.
type InvestigationUnitSource string

const (
	InvestigationUnitSourceUnknown  InvestigationUnitSource = ""
	InvestigationUnitSourceSubTopic InvestigationUnitSource = "sub_topic"
	InvestigationUnitSourceBucket   InvestigationUnitSource = "bucket"
)

// InvestigationPlan is a derived, side-effect-free view over the analyzer IR.
// It lets UI and future schedulers distinguish user partitions from analyzer
// investigation decomposition without adding another hard gate.
type InvestigationPlan struct {
	Units          []InvestigationUnit
	Coupling       InvestigationCoupling
	HasUserBuckets bool
}

// InvestigationUnit is one displayable / schedulable work unit. The unit's
// text fields come from typed analyzer fields (SubTopics/Buckets); downstream
// code must not treat them as grounded evidence.
type InvestigationUnit struct {
	ID              string
	Index           int
	Label           string
	Summary         string
	Entities        []string
	Scopes          []string
	Role            InvestigationRole
	Coupling        InvestigationCoupling
	AnswerPartition InvestigationAnswerPartition
	EvidenceOrigins []AnswerEvidenceOrigin
	Source          InvestigationUnitSource
}

// CompileInvestigationPlan derives investigation units from existing typed
// request fields. It does not read model prose and does not mutate RequestModel.
func CompileInvestigationPlan(rm RequestModel, contract *AnswerContract) InvestigationPlan {
	intent := CompileAnswerIntentContract(rm, contract)
	origins := cloneAnswerEvidenceOrigins(intent.Origins)
	coupling := inferInvestigationCoupling(rm)
	buckets := rm.QuestionStructure().Buckets
	if len(buckets) >= 2 {
		units := make([]InvestigationUnit, 0, len(buckets))
		for i, bucket := range buckets {
			label := strings.TrimSpace(bucket.Label)
			if label == "" {
				label = fmt.Sprintf("bucket-%d", i+1)
			}
			units = append(units, InvestigationUnit{
				ID:              fmt.Sprintf("bucket-%d", i+1),
				Index:           i + 1,
				Label:           label,
				Summary:         label,
				Role:            InvestigationRolePrincipal,
				Coupling:        InvestigationCouplingComparative,
				AnswerPartition: InvestigationAnswerPartitionUserBucket,
				EvidenceOrigins: origins,
				Source:          InvestigationUnitSourceBucket,
			})
		}
		return InvestigationPlan{
			Units:          units,
			Coupling:       InvestigationCouplingComparative,
			HasUserBuckets: true,
		}
	}
	if len(rm.SubTopics) < 2 {
		return InvestigationPlan{Coupling: coupling}
	}
	units := make([]InvestigationUnit, 0, len(rm.SubTopics))
	for i, topic := range rm.SubTopics {
		summary := strings.TrimSpace(topic.Summary)
		label := summary
		if label == "" && len(topic.Entities) > 0 {
			label = strings.TrimSpace(topic.Entities[0])
		}
		if label == "" {
			label = fmt.Sprintf("unit-%d", i+1)
		}
		units = append(units, InvestigationUnit{
			ID:              fmt.Sprintf("subtopic-%d", i+1),
			Index:           i + 1,
			Label:           label,
			Summary:         summary,
			Entities:        append([]string(nil), topic.Entities...),
			Scopes:          append([]string(nil), topic.Scopes...),
			Role:            InvestigationRolePrincipal,
			Coupling:        coupling,
			AnswerPartition: InvestigationAnswerPartitionAnalyzerDecomposition,
			EvidenceOrigins: origins,
			Source:          InvestigationUnitSourceSubTopic,
		})
	}
	return InvestigationPlan{
		Units:    units,
		Coupling: coupling,
	}
}

func inferInvestigationCoupling(rm RequestModel) InvestigationCoupling {
	switch {
	case rm.Intent == IntentTrace || NormalizeRequirementKind(rm.AnalyzerHints.Kind) == ReqCallChain:
		return InvestigationCouplingSequential
	case len(rm.QuestionStructure().Buckets) >= 2:
		return InvestigationCouplingComparative
	case rm.HasRuntimeArtifactCurrentVerificationAnchor():
		return InvestigationCouplingSharedContext
	case rm.Predicates.IsCrossComponent:
		return InvestigationCouplingSharedContext
	default:
		return InvestigationCouplingIndependent
	}
}

func cloneAnswerEvidenceOrigins(in []AnswerEvidenceOrigin) []AnswerEvidenceOrigin {
	if len(in) == 0 {
		return nil
	}
	out := make([]AnswerEvidenceOrigin, 0, len(in))
	seen := map[AnswerEvidenceOrigin]bool{}
	for _, origin := range in {
		if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() || seen[origin] {
			continue
		}
		seen[origin] = true
		out = append(out, origin)
	}
	return out
}

// CompileExploreSubTopicGroups derives the scheduler's view of analyzer
// sub-topics without changing the analyzer IR or the answer partitions.
//
// A required relation diagram is one evidence objective even when the
// analyzer decomposes each requested participant into a separate sub-topic.
// Dispatching those participant facets as independent investigations makes
// every explorer rediscover the same relation surface. Group only the
// sub-topics whose typed entities are wholly contained by the required
// incident-participant roster; every other sub-topic remains independent.
//
// This is deliberately conservative and typed-only: a schema-valid required
// diagram and matching relation axis are mandatory; scoped/extra-entity
// sub-topics are not absorbed; and Trace/root-cause families retain their
// original fan-out so explicit windows, causal projection, and automatic
// evidence supplementation are unaffected. The function never reads
// RawRequest, sub-topic summaries, model prose, or answer text.
func CompileExploreSubTopicGroups(rm RequestModel) [][]int {
	groups := singletonExploreSubTopicGroups(len(rm.SubTopics))
	if len(rm.SubTopics) < 2 || !requiredDiagramRelationAxisAgrees(rm) ||
		ResolveQuestionFamily(rm) == QFRootCauseTrace || rm.Intent == IntentTrace {
		return groups
	}

	participantKeys := make(map[string]struct{}, len(rm.DiagramHint.Participants))
	for _, participant := range rm.DiagramHint.Participants {
		if participant.Role != DiagramParticipantIncidentRequired {
			continue
		}
		if key := exactInvestigationIdentityKey(participant.Identity); key != "" {
			participantKeys[key] = struct{}{}
		}
	}
	if len(participantKeys) < 2 {
		return groups
	}

	relationFacets := make([]int, 0, len(rm.SubTopics))
	for i, topic := range rm.SubTopics {
		if len(topic.Entities) == 0 || len(topic.Scopes) > 0 {
			continue
		}
		allParticipants := true
		matched := false
		for _, entity := range topic.Entities {
			key := exactInvestigationIdentityKey(entity)
			if key == "" {
				allParticipants = false
				break
			}
			if _, ok := participantKeys[key]; !ok {
				allParticipants = false
				break
			}
			matched = true
		}
		if allParticipants && matched {
			relationFacets = append(relationFacets, i)
		}
	}
	if len(relationFacets) < 2 {
		return groups
	}

	facetSet := make(map[int]struct{}, len(relationFacets))
	for _, idx := range relationFacets {
		facetSet[idx] = struct{}{}
	}
	groups = make([][]int, 0, len(rm.SubTopics)-len(relationFacets)+1)
	groupEmitted := false
	for i := range rm.SubTopics {
		if _, ok := facetSet[i]; ok {
			if !groupEmitted {
				groups = append(groups, append([]int(nil), relationFacets...))
				groupEmitted = true
			}
			continue
		}
		groups = append(groups, []int{i})
	}
	return groups
}

// ExploreSchedulingUnitCount is a scheduling count only;
// RequestModel.SubTopics and all answer obligations stay intact.
func ExploreSchedulingUnitCount(rm RequestModel) int {
	return len(CompileExploreSubTopicGroups(rm))
}

func singletonExploreSubTopicGroups(n int) [][]int {
	if n <= 0 {
		return nil
	}
	out := make([][]int, n)
	for i := 0; i < n; i++ {
		out[i] = []int{i}
	}
	return out
}

func requiredDiagramRelationAxisAgrees(rm RequestModel) bool {
	if rm.DiagramHint == nil || !rm.DiagramHint.Required ||
		rm.DiagramHint.Kind == DiagramNone || !rm.DiagramHint.Kind.IsValid() {
		return false
	}
	switch rm.DiagramHint.Kind {
	case DiagramFlow:
		return rm.PredicateAxis == AxisFlow
	case DiagramSequence:
		return rm.PredicateAxis == AxisFlow || rm.PredicateAxis == AxisCall
	case DiagramCallDAG:
		return rm.PredicateAxis == AxisCall
	case DiagramArchitecture:
		switch rm.PredicateAxis {
		case AxisFlow, AxisCall, AxisRegister, AxisConfigure, AxisImplement:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func exactInvestigationIdentityKey(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// InvestigationUnitForEvidenceNode returns the unit corresponding to an
// evidence node generated by expandEvidenceNodes. It understands the existing
// "_tN" suffix but otherwise leaves non-topic nodes unclassified.
func (p InvestigationPlan) InvestigationUnitForEvidenceNode(nodeID string) (InvestigationUnit, bool) {
	idx, ok := investigationTopicIndexFromNodeID(nodeID)
	if !ok {
		return InvestigationUnit{}, false
	}
	for _, unit := range p.Units {
		if unit.Index == idx+1 {
			return unit, true
		}
	}
	return InvestigationUnit{}, false
}

func investigationTopicIndexFromNodeID(id string) (int, bool) {
	pos := strings.LastIndex(id, "_t")
	if pos < 0 || pos+2 >= len(id) {
		return 0, false
	}
	n := 0
	for _, r := range id[pos+2:] {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}
