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
