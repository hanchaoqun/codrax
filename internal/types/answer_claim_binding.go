package types

import (
	"fmt"
	"strings"
)

// AnswerClaimBinding is the origin-specific handoff consumed by answer-writing
// stages. It keeps the model-authored aggregate fact, evidence origin, visible
// output shape, and grounding policy in one deterministic record so downstream
// code does not independently reinterpret history/count/runtime/source facts.
type AnswerClaimBinding struct {
	ClaimID          string
	AggregateIndex   int
	AggregateKind    AnswerAggregateKind
	AggregateRole    AnswerAggregateRole
	Label            string
	Value            string
	TargetRef        string
	Origin           AnswerEvidenceOrigin
	RequestedOutputs []AnswerRequestedOutput
	SupportRefs      []string
	GroundingPolicy  ClaimGroundingPolicy
}

// CompileAnswerClaimBindingsFromAggregateFacts projects stable
// emit_investigation_complete.aggregate_facts into origin-specific claim
// bindings. It consumes typed aggregate dimensions and request model fields
// only; it does not inspect raw user prose or model free text.
func CompileAnswerClaimBindingsFromAggregateFacts(facts []AnswerAggregateFact, rm *RequestModel, answerContract *AnswerContract) []AnswerClaimBinding {
	if len(facts) == 0 {
		return nil
	}
	requestContract := AnswerIntentContract{}
	if rm != nil {
		var contract *AnswerContract
		if answerContract != nil {
			contract = answerContract
		}
		requestContract = CompileAnswerIntentContract(*rm, contract)
	}
	outputs := requestContract.RequestedOutputs
	if len(outputs) == 0 {
		outputs = []AnswerRequestedOutput{AnswerRequestedOutputSummary}
	}
	out := make([]AnswerClaimBinding, 0, len(facts))
	for idx, fact := range facts {
		role := AnswerAggregateFactRoleForRequest(fact, rm)
		origins := AnswerAggregateFactEvidenceOrigins(fact, rm)
		if len(origins) == 0 {
			origins = []AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource}
		}
		for _, origin := range origins {
			if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() {
				continue
			}
			out = append(out, AnswerClaimBinding{
				ClaimID:          answerClaimBindingID(idx, origin),
				AggregateIndex:   idx,
				AggregateKind:    fact.Kind,
				AggregateRole:    role,
				Label:            fact.Label,
				Value:            fact.Value,
				TargetRef:        answerClaimBindingTargetRef(fact),
				Origin:           origin,
				RequestedOutputs: cloneAnswerRequestedOutputs(outputs),
				SupportRefs:      cloneAnswerClaimBindingStrings(fact.SupportRefs),
				GroundingPolicy:  AnswerClaimBindingGroundingPolicy(origin, role),
			})
		}
	}
	return out
}

func AnswerClaimBindingGroundingPolicy(origin AnswerEvidenceOrigin, role AnswerAggregateRole) ClaimGroundingPolicy {
	principal := NormalizeAnswerAggregateRole(role).IsPrincipal()
	switch origin {
	case AnswerEvidenceOriginCurrentSource:
		if principal {
			return ClaimGroundingHard
		}
		return ClaimGroundingRepairable
	case AnswerEvidenceOriginRepoNegativeSearch,
		AnswerEvidenceOriginCommandMeasurement:
		if principal {
			return ClaimGroundingHard
		}
		return ClaimGroundingSoft
	case AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginVCSDiff,
		AnswerEvidenceOriginRuntimeArtifact,
		AnswerEvidenceOriginCrossRepoIndex:
		if principal {
			return ClaimGroundingRepairable
		}
		return ClaimGroundingSoft
	case AnswerEvidenceOriginSystemInference:
		if principal {
			return ClaimGroundingSoft
		}
		return ClaimGroundingDisplayOnly
	default:
		return ClaimGroundingSoft
	}
}

func answerClaimBindingID(index int, origin AnswerEvidenceOrigin) string {
	return fmt.Sprintf("aggregate_facts[%d]#%s", index, origin)
}

func answerClaimBindingTargetRef(fact AnswerAggregateFact) string {
	label := strings.TrimSpace(fact.Label)
	if label != "" {
		return label
	}
	if len(fact.Members) > 0 {
		return strings.TrimSpace(fact.Members[0])
	}
	return strings.TrimSpace(fact.Value)
}

func cloneAnswerRequestedOutputs(in []AnswerRequestedOutput) []AnswerRequestedOutput {
	if len(in) == 0 {
		return nil
	}
	out := make([]AnswerRequestedOutput, len(in))
	copy(out, in)
	return out
}

func cloneAnswerClaimBindingStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
