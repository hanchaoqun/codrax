package types

import (
	"math"
	"sort"
	"strings"
)

// RuntimeWorkRelationConclusion is the model-selected conclusion for one
// exact typed runtime-work observation.  The system publishes the available
// rows and evidence ceiling, but never selects this value from request or
// answer prose.
type RuntimeWorkRelationConclusion string

const (
	RuntimeWorkRelationConclusionUnknown                     RuntimeWorkRelationConclusion = ""
	RuntimeWorkRelationConclusionRelatedCausalityUnproven    RuntimeWorkRelationConclusion = "related_causality_unproven"
	RuntimeWorkRelationConclusionTargetSelfWorkObserved      RuntimeWorkRelationConclusion = "target_self_work_observed"
	RuntimeWorkRelationConclusionCausalContributionSupported RuntimeWorkRelationConclusion = "causal_contribution_supported"
	RuntimeWorkRelationConclusionRelationUnproven            RuntimeWorkRelationConclusion = "relation_unproven"
)

func (c RuntimeWorkRelationConclusion) IsValid() bool {
	switch c {
	case RuntimeWorkRelationConclusionRelatedCausalityUnproven,
		RuntimeWorkRelationConclusionTargetSelfWorkObserved,
		RuntimeWorkRelationConclusionCausalContributionSupported,
		RuntimeWorkRelationConclusionRelationUnproven:
		return true
	default:
		return false
	}
}

// RuntimeWorkRelationRow is one exact evidence-backed choice published to the
// finalizer.  Credential and Boundary are closed typed reader facts.  They are
// never inferred from the work name or model-authored prose.
type RuntimeWorkRelationRow struct {
	ObservationID      string
	WorkLabel          string
	Subject            string
	MeasuredDurationMS float64
	AllowedConclusions []RuntimeWorkRelationConclusion
	Credential         string
	Boundary           string
}

// RuntimeWorkRelationContract is active only when the analyzer explicitly
// declared the independent work/span-to-target subquestion and the ledger has
// at least one exact semantic-work row.
type RuntimeWorkRelationContract struct {
	Rows []RuntimeWorkRelationRow
}

func (c *RuntimeWorkRelationContract) Active() bool {
	return c != nil && len(c.Rows) > 0
}

// AnswerRuntimeWorkRelationReceipt is authored by the model on one visible
// principal block.  Only the exact row id and conclusion cross the wire.  The
// remaining facts are bound from the typed contract after validation so the
// renderer can display exact name/duration/credential/boundary without asking
// the model to recopy numbers or letting the system choose the conclusion.
type AnswerRuntimeWorkRelationReceipt struct {
	ObservationID string                        `json:"observation_id"`
	Conclusion    RuntimeWorkRelationConclusion `json:"conclusion"`
	BoundRow      RuntimeWorkRelationRow        `json:"-"`
}

func (r *AnswerRuntimeWorkRelationReceipt) IsBound() bool {
	return r != nil && strings.TrimSpace(r.BoundRow.ObservationID) != ""
}

func BindRuntimeWorkRelationReceipt(r *AnswerRuntimeWorkRelationReceipt, contract *RuntimeWorkRelationContract) bool {
	if r == nil || !contract.Active() || !r.Conclusion.IsValid() {
		return false
	}
	id := strings.TrimSpace(r.ObservationID)
	for _, row := range contract.Rows {
		if id == row.ObservationID && runtimeWorkRelationConclusionAllowed(r.Conclusion, row.AllowedConclusions) {
			r.ObservationID = id
			r.BoundRow = row
			return true
		}
	}
	return false
}

// BuildRuntimeWorkRelationContract compiles only exact typed semantic-work
// rows.  It does not inspect the request or answer prose.  requested comes
// from RuntimeQuestionProfile.RuntimeWorkRelationRequested.
func BuildRuntimeWorkRelationContract(input ObservationLedgerInput, requested bool) *RuntimeWorkRelationContract {
	if !requested {
		return nil
	}
	set := CompileTraceCausalProjectionSet(CompileObservationLedger(input))
	seen := map[string]bool{}
	var rows []RuntimeWorkRelationRow
	for _, projection := range set.Projections {
		for _, node := range projection.SemanticSpans {
			id := strings.TrimSpace(node.EvidenceID)
			label := strings.TrimSpace(node.SpanName)
			if id == "" || label == "" || seen[id] {
				continue
			}
			row := runtimeWorkRelationRowFromNode(id, label, node)
			if row.MeasuredDurationMS <= 0 || math.IsNaN(row.MeasuredDurationMS) || math.IsInf(row.MeasuredDurationMS, 0) {
				continue
			}
			seen[id] = true
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ObservationID < rows[j].ObservationID })
	return &RuntimeWorkRelationContract{Rows: rows}
}

func runtimeWorkRelationRowFromNode(id, label string, node TraceCausalProjectionNode) RuntimeWorkRelationRow {
	row := RuntimeWorkRelationRow{
		ObservationID: id,
		WorkLabel:     label,
		Subject:       strings.TrimSpace(node.Subject),
	}
	switch {
	case node.ActualImpactMS > 0:
		row.MeasuredDurationMS = node.ActualImpactMS
	case node.ImpactMS > 0:
		row.MeasuredDurationMS = node.ImpactMS
	case node.EndTs > node.StartTs:
		row.MeasuredDurationMS = (node.EndTs - node.StartTs) * 1000
	}
	switch strings.TrimSpace(node.OnChainBasis) {
	case TraceCausalOnChainBasisHostWakeupEdgeSpan:
		row.AllowedConclusions = []RuntimeWorkRelationConclusion{
			RuntimeWorkRelationConclusionRelatedCausalityUnproven,
			RuntimeWorkRelationConclusionRelationUnproven,
		}
		row.Credential = "host_direct_wakeup_edge"
		row.Boundary = "work_completion_target_wait_and_frame_causality_unproven"
	case TraceCausalOnChainBasisSemanticChainIntervalRelation:
		row.AllowedConclusions = []RuntimeWorkRelationConclusion{
			RuntimeWorkRelationConclusionRelatedCausalityUnproven,
			RuntimeWorkRelationConclusionRelationUnproven,
		}
		row.Credential = "typed_chain_interval_overlap"
		row.Boundary = "work_completion_target_wait_and_frame_causality_unproven"
	case TraceCausalOnChainBasisSelfDeterministicSpan:
		row.AllowedConclusions = []RuntimeWorkRelationConclusion{
			RuntimeWorkRelationConclusionTargetSelfWorkObserved,
			RuntimeWorkRelationConclusionRelationUnproven,
		}
		row.Credential = "target_self_execution"
		row.Boundary = "frame_or_deadline_causality_unproven"
	default:
		if node.EffectiveImpactPublished && node.EffectiveImpactMS > 0 {
			row.AllowedConclusions = []RuntimeWorkRelationConclusion{
				RuntimeWorkRelationConclusionCausalContributionSupported,
				RuntimeWorkRelationConclusionRelatedCausalityUnproven,
				RuntimeWorkRelationConclusionRelationUnproven,
			}
			row.Credential = "typed_chain_effective_attribution"
			row.Boundary = "bounded_by_typed_chain_evidence"
		} else {
			row.AllowedConclusions = []RuntimeWorkRelationConclusion{
				RuntimeWorkRelationConclusionRelationUnproven,
			}
			row.Credential = "none"
			row.Boundary = "work_to_target_relation_unproven"
		}
	}
	return row
}

func runtimeWorkRelationConclusionAllowed(got RuntimeWorkRelationConclusion, allowed []RuntimeWorkRelationConclusion) bool {
	for _, candidate := range allowed {
		if got == candidate {
			return true
		}
	}
	return false
}
