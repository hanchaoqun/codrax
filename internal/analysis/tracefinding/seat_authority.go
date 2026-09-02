package tracefinding

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// SeatFrameCausalityIndex is THE single seat-level frame-causality authority
// (SIDECAR-Q1, user ruling 2026-09-02, colleague_merge_audit §40.28 ②): it
// binds trace_query's result-level frame authority to the exact producer
// observation IDs that can enter the compiled ledger, so a conclusion
// qualifier depends on the evidence the elected seat actually consumed —
// never on an unrelated exploratory query elsewhere in the run (the
// session-wide ANY aggregate the T3-1 ruling §7.3 rejected for the crown
// face; it survives only as an advisory prompt hint).
//
// Both public consumers read this one map: the Markdown crown face
// (answer_document_mutation_runtime.go) and the trace-finding contract that
// feeds the .root-causes.json sidecar. It deliberately mirrors
// compileProducerToolResultObservations' eligibility, fallback-ID and
// first-ID-wins rules. No request text, tool summary, or model prose
// participates.
type SeatFrameCausalityIndex map[string]bool

// BuildSeatFrameCausalityIndex compiles the index from the ledger input's
// tool results (producer + system trace supplements).
//
// Frame-status resolution (batch-one adversarial review + eval witness
// trace_query_frame_semantic_span_optimization, 2026-09-02): only the frame
// views (frame_root_cause_bundle / frame_flow / frame_timeline) EVALUATE frame
// evidence — root_cause_rank / wakeup_chain results carry an EMPTY
// FrameEvidenceStatus ("not evaluated", never "present"). Reading that empty
// status as proven made the qualifier depend on which result happened to
// supply a seat's canonical evidence ID (the same VerifyClass seat was
// 「（帧因果未证）」 on the crown face and "proven" on the sidecar in one
// run, then the reverse in the next). The seat-level rule is therefore:
//   - an EVALUATING typed-row result speaks for its own seats
//     (absent/unavailable/flow-unproven ⇒ frame_unproven; present ⇒ proven);
//   - a NON-evaluating typed-row result inherits the evaluated verdict of
//     the SAME ARTIFACT (scoped by the records' typed SourceRef.Path /
//     ArtifactID — a multi-artifact compare run never leaks one trace's
//     verdict onto another's seats): frame_unproven iff some typed-row
//     evaluator on that artifact found the frame evidence
//     absent/unavailable/unproven and none proved it;
//   - a zero-row result (the T3-1 exploratory probe class) never taints and
//     never proves — it is not an evaluator.
//
// An artifact with NO frame evaluator keeps the historical shape (no
// qualifier): the qualifier speaks about frame causality only where the run
// examined it.
func BuildSeatFrameCausalityIndex(input types.ObservationLedgerInput) SeatFrameCausalityIndex {
	results := make([]types.ToolResult, 0, len(input.ToolResults)+len(input.SystemTraceSupplementResults))
	results = append(results, input.ToolResults...)
	results = append(results, input.SystemTraceSupplementResults...)
	isTraceResult := func(result types.ToolResult) (*types.TraceEvidenceAuthority, bool) {
		if !result.Success || !strings.EqualFold(strings.TrimSpace(result.ToolName), "trace_query") {
			return nil, false
		}
		authority := result.TraceEvidenceAuthority
		if authority == nil || authority.TypedCausalRowCount <= 0 {
			return nil, false
		}
		return authority, true
	}
	frameVerdict := func(authority *types.TraceEvidenceAuthority) (evaluated, unproven bool) {
		// A typed frame-flow conclusion of "unproven" is an evaluation in
		// its own right, whatever the status word says.
		if authority.FrameFlowCausalConclusion == tracequery.FrameFlowCausalityUnproven {
			return true, true
		}
		switch strings.TrimSpace(authority.FrameEvidenceStatus) {
		case "absent", "unavailable":
			return true, true
		case "":
			return false, false
		default: // "present" with no withholding flow conclusion — proven
			return true, false
		}
	}
	artifactKey := func(result types.ToolResult) string {
		for _, record := range result.Observations {
			if path := strings.TrimSpace(record.SourceRef.Path); path != "" {
				return path
			}
			if id := strings.TrimSpace(record.SourceRef.ArtifactID); id != "" {
				return id
			}
		}
		return ""
	}
	artifactUnproven := map[string]bool{}
	artifactProven := map[string]bool{}
	for _, result := range results {
		authority, ok := isTraceResult(result)
		if !ok {
			continue
		}
		if evaluated, unproven := frameVerdict(authority); evaluated {
			key := artifactKey(result)
			if unproven {
				artifactUnproven[key] = true
			} else {
				artifactProven[key] = true
			}
		}
	}
	seen := make(map[string]bool)
	out := make(SeatFrameCausalityIndex)
	for i, result := range results {
		authority, isTypedTrace := isTraceResult(result)
		seatFrameUnproven := false
		if isTypedTrace {
			if evaluated, unproven := frameVerdict(authority); evaluated {
				seatFrameUnproven = unproven
			} else {
				key := artifactKey(result)
				seatFrameUnproven = artifactUnproven[key] && !artifactProven[key]
			}
		}
		if !result.Success {
			continue
		}
		for j, record := range result.Observations {
			if record.Origin == types.AnswerEvidenceOriginUnknown || !record.Origin.IsValid() ||
				record.Origin == types.AnswerEvidenceOriginCurrentSource {
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
			if seatFrameUnproven {
				out[id] = true
			}
		}
	}
	return out
}

// SeatFrameUnproven reports whether any of the seat's evidence IDs carries the
// frame-unproven authority. Empty index ⇒ false (no authority, no qualifier).
func (idx SeatFrameCausalityIndex) SeatFrameUnproven(evidenceIDs ...string) bool {
	if len(idx) == 0 {
		return false
	}
	for _, id := range evidenceIDs {
		if idx[strings.TrimSpace(id)] {
			return true
		}
	}
	return false
}
