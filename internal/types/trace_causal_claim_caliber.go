package types

import (
	"fmt"
	"strings"
)

// TraceCausalClaimCaliber is the model-authored scope of the principal Trace
// synthesis. It describes how strongly the model is claiming causality; it
// never identifies or elects the cause itself.
type TraceCausalClaimCaliber string

const (
	TraceCausalClaimNoConclusion  TraceCausalClaimCaliber = "no_causal_conclusion"
	TraceCausalClaimBoundedWindow TraceCausalClaimCaliber = "bounded_window_candidate"
	TraceCausalClaimTypedChain    TraceCausalClaimCaliber = "typed_chain_cause"
	TraceCausalClaimTypedFrame    TraceCausalClaimCaliber = "typed_frame_cause"
)

func AllTraceCausalClaimCalibers() []TraceCausalClaimCaliber {
	return []TraceCausalClaimCaliber{
		TraceCausalClaimNoConclusion,
		TraceCausalClaimBoundedWindow,
		TraceCausalClaimTypedChain,
		TraceCausalClaimTypedFrame,
	}
}

func (c TraceCausalClaimCaliber) IsValid() bool {
	for _, candidate := range AllTraceCausalClaimCalibers() {
		if c == candidate {
			return true
		}
	}
	return false
}

func NormalizeTraceCausalClaimCaliber(raw string) (TraceCausalClaimCaliber, bool) {
	caliber := TraceCausalClaimCaliber(strings.TrimSpace(raw))
	return caliber, caliber.IsValid()
}

// TraceCausalClaimContract is projected only for a full typed Trace causal
// report with publication-grade rows. Allowed is the evidence ceiling for the
// model-authored principal summary. The system neither chooses a caliber nor
// derives one from answer prose.
type TraceCausalClaimContract struct {
	Allowed []TraceCausalClaimCaliber `json:"allowed,omitempty"`
	Ceiling TraceCausalClaimCaliber   `json:"ceiling,omitempty"`
}

func (c *TraceCausalClaimContract) Active() bool {
	return c != nil && len(c.Allowed) > 0 && c.Ceiling.IsValid()
}

func (c *TraceCausalClaimContract) Allows(caliber TraceCausalClaimCaliber) bool {
	if !c.Active() || !caliber.IsValid() {
		return false
	}
	for _, allowed := range c.Allowed {
		if allowed == caliber {
			return true
		}
	}
	return false
}

// BuildTraceCausalClaimContract derives a report-local causal ceiling from the
// exact producer observations consumed by the finally elected projection
// seats. An unrelated earlier trace query cannot lower a later seat because
// its observation IDs do not occur on that seat.
func BuildTraceCausalClaimContract(input ObservationLedgerInput) *TraceCausalClaimContract {
	ledger := CompileObservationLedger(input)
	set := CompileTraceCausalProjectionSet(ledger)
	if !RuntimeTraceReportMaterializationAllowed(input.RequestModel, set) ||
		!TraceCausalProjectionSetHasPublicationGradeRows(set) {
		return nil
	}

	authorities := traceCausalClaimAuthorityByObservationID(input)
	matchedLead := false
	typedChainAllowed := true
	frameRelevant := false
	typedFrameAllowed := true
	for _, projection := range set.Projections {
		lead := traceCausalClaimProjectionLead(projection)
		if lead == nil {
			continue
		}
		ids := append([]string{lead.EvidenceID}, lead.MergedEvidenceIDs...)
		seatMatched := false
		for _, id := range ids {
			authority, ok := authorities[strings.TrimSpace(id)]
			if !ok {
				continue
			}
			seatMatched = true
			matchedLead = true
			if authority.CausalConclusion != "bounded_by_typed_rows" {
				typedChainAllowed = false
			}
			status := strings.TrimSpace(authority.FrameEvidenceStatus)
			if status != "" {
				frameRelevant = true
				if status != "present" {
					typedFrameAllowed = false
					typedChainAllowed = false
				}
			}
			if strings.TrimSpace(authority.FrameFlowCausalConclusion) == "unproven" ||
				strings.TrimSpace(authority.CausalConclusion) == "unproven" {
				typedFrameAllowed = false
				if status != "" {
					typedChainAllowed = false
				}
			}
		}
		if !seatMatched {
			// A publication seat without a result-level authority can still be
			// described as a bounded candidate, but cannot mint a stronger
			// causal credential.
			typedChainAllowed = false
			typedFrameAllowed = false
		}
	}
	if !matchedLead {
		typedChainAllowed = false
		typedFrameAllowed = false
	}

	allowed := []TraceCausalClaimCaliber{
		TraceCausalClaimNoConclusion,
		TraceCausalClaimBoundedWindow,
	}
	ceiling := TraceCausalClaimBoundedWindow
	if typedChainAllowed {
		allowed = append(allowed, TraceCausalClaimTypedChain)
		ceiling = TraceCausalClaimTypedChain
	}
	if frameRelevant && typedChainAllowed && typedFrameAllowed {
		allowed = append(allowed, TraceCausalClaimTypedFrame)
		ceiling = TraceCausalClaimTypedFrame
	}
	return &TraceCausalClaimContract{Allowed: allowed, Ceiling: ceiling}
}

func traceCausalClaimProjectionLead(projection TraceCausalProjection) *TraceCausalProjectionNode {
	if projection.PrimaryRootCause != nil {
		return projection.PrimaryRootCause
	}
	if len(projection.PrimaryRootCauses) > 0 {
		return &projection.PrimaryRootCauses[0]
	}
	if len(projection.OnChainCauses) > 0 {
		return &projection.OnChainCauses[0]
	}
	if len(projection.AdjacentCauses) > 0 {
		return &projection.AdjacentCauses[0]
	}
	if len(projection.SupportingHops) > 0 {
		return &projection.SupportingHops[0]
	}
	return nil
}

func traceCausalClaimAuthorityByObservationID(input ObservationLedgerInput) map[string]TraceEvidenceAuthority {
	results := make([]ToolResult, 0, len(input.ToolResults)+len(input.SystemTraceSupplementResults))
	results = append(results, input.ToolResults...)
	results = append(results, input.SystemTraceSupplementResults...)
	seen := make(map[string]bool)
	out := make(map[string]TraceEvidenceAuthority)
	for i, result := range results {
		if !result.Success || !strings.EqualFold(strings.TrimSpace(result.ToolName), "trace_query") ||
			result.TraceEvidenceAuthority == nil {
			continue
		}
		for j, record := range result.Observations {
			if record.Origin == AnswerEvidenceOriginUnknown || !record.Origin.IsValid() ||
				record.Origin == AnswerEvidenceOriginCurrentSource {
				continue
			}
			id := strings.TrimSpace(record.ID)
			if id == "" {
				id = fmt.Sprintf("tool:%d#%s:typed:%d", i, strings.TrimSpace(result.ToolName), j)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			out[id] = *result.TraceEvidenceAuthority
		}
	}
	return out
}
