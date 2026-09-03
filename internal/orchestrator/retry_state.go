package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// retry_state.go — R14-c3 + c4 (post_shape_residual_audit.md, 2026-05-04).
// Populates types.RetryState from contract.Result + the latest
// finalizer emit before each retry dispatch. The agent layer's
// renderRetryState consumes this on the next BuildInitialInstruction
// to surface "Previous Emit / Active Violations / Required Changes /
// Hard Rule" sections to the LLM.
//
// Single-source-of-truth principle (R14): every retry-decision site
// in orchestrator.go calls populateRetryState BEFORE requeuing the
// finalizer. Producer Layer tagging happens here based on
// ViolationKind (kind → layer is a deterministic map; if a future
// producer needs finer-grained layer info, extend the map).

// populateRetryState writes types.RetryState to the bus mutable
// after a contract.Check failure, capturing:
//
//  1. The previous emit (PrevEmitJSON + PrevEmitSummary projection)
//     so the LLM sees what fields it already filled.
//  2. Every violation in the result, scored with severity / layer
//     / field path so the LLM can prioritise.
//  3. The retry attempt counter so renderers know we're in a retry.
//  4. (Phase 1-A2) The RepairPlan summary: LastPrimaryOwner,
//     OwnerStableAttempts (stability counter), LastPrimaryViolation.
//     Wired so the orchestrator can detect ping-pong / decide on
//     escalation without recomputing the plan.
//
// Called BEFORE state.requeue(fin.ID) so the next finalizer dispatch
// observes the populated state on its first BuildInitialInstruction
// pass.
func populateRetryState(mut *types.MutableState, res contract.Result, prevAttempt int) {
	if mut == nil {
		return
	}
	// A2: read previous state BEFORE building new RetryState so the
	// stability counter has access to the prior LastPrimaryOwner.
	prevState := mut.RetryState()

	rs := &types.RetryState{
		Attempt: prevAttempt + 1,
	}
	if doc := mut.AnswerDocumentV2(); doc != nil {
		rs.PrevEmitSummary = summarizeAnswerDocV2ForRetry(doc)
		// B4: surface patch lineage so retry summary observability
		// can audit "patch-extended vs fresh emit chain".
		rs.LastEmitFromPatch = mut.LastEmitFromPatch()
		// PrevEmitJSON: serialise the V2 doc as canonical JSON so
		// the LLM has the verbatim previous payload as a fallback
		// when summary alone is insufficient. Best-effort —
		// serialisation failure is non-fatal (summary still
		// renders).
		if raw, err := json.Marshal(doc); err == nil {
			rs.PrevEmitJSON = raw
			// Marker-stripping class root fix (audit 2026-07-10):
			// json.Marshal just stripped every json:"-"
			// SystemGeneratedKind marker from the snapshot. Capture
			// the live authority map alongside it so decode-side
			// consumers that rebuild a shipping document (patch base
			// / recovery draft / ParseOutput fallback) can
			// re-authenticate the genuine system blocks instead of
			// demoting them to model grade.
			rs.PrevEmitSystemBlockKinds = types.CaptureSystemGeneratedBlockKinds(doc)
		}
	}
	actionable := FilterActionableRootViolations(res.Violations)
	rs.ActiveViolations = scoreViolations(actionable)

	// A2: compute RepairPlan + populate stability fields.
	plan := BuildRepairPlan(actionable)
	rs.LastPrimaryOwner = string(plan.PrimaryOwner)
	rs.LastPrimaryViolation = deepestPrimaryKind(plan)
	if prevState != nil && prevState.LastPrimaryOwner == rs.LastPrimaryOwner && rs.LastPrimaryOwner != "" {
		rs.OwnerStableAttempts = prevState.OwnerStableAttempts + 1
	} else if rs.LastPrimaryOwner != "" {
		rs.OwnerStableAttempts = 1
	}

	mut.SetRetryState(rs)
}

// deepestPrimaryKind returns the Primary.Kind of the deepest cluster
// in plan.Clusters (the cluster that drives PrimaryOwner). Empty
// when the plan has no clusters. Helper for populateRetryState
// stability tracking.
func deepestPrimaryKind(plan RepairPlan) types.ViolationKind {
	if len(plan.Clusters) == 0 {
		return ""
	}
	// plan.Clusters is sorted deepest-first by sortClustersDeepestFirst,
	// so the first entry is the cluster whose Owner == PrimaryOwner.
	return plan.Clusters[0].Primary.Kind
}

// scoreViolations turns the contract.Check Violation slice into
// the structured ScoredViolation slice consumed by RetryState.
// Severity comes from types.DeriveSeverity (single source of truth);
// Layer is inferred from kind via inferViolationLayer; FieldPath is
// extracted from the violation Detail when present (e.g. block id).
func scoreViolations(in []contract.Violation) []types.ScoredViolation {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.ScoredViolation, 0, len(in))
	for _, v := range in {
		isStrict := !isSoftViolationKind(v.Kind)
		sv := types.ScoredViolation{
			Kind:      v.Kind,
			Detail:    v.Detail,
			Repair:    v.Repair,
			Severity:  types.DeriveSeverity(v.Kind, isStrict),
			Layer:     inferViolationLayer(v.Kind),
			BlockID:   extractBlockIDFromDetail(v.Detail),
			FieldPath: inferFieldPathFromKind(v.Kind, v.Detail),
		}
		out = append(out, sv)
	}
	return out
}

// inferViolationLayer maps each ViolationKind to the gating layer
// that produces it. Used by RetryState rendering to group active
// violations by Layer (gives the LLM cross-layer visibility — fixes
// R13 by ensuring the LLM sees scheduler-level + V2 oracle + contract
// check violations together).
//
// When a kind has multiple producers (rare), prefer the most
// frequent. Unknown kinds default to "contract_check".
//
// v3 B0 (2026-05-04): registry-first. When a spec exists for kind,
// the spec.Layer wins; otherwise fall through to the legacy switch.
// TestRegistryDerivesAllLegacyTables enforces byte-identical agreement
// for every kind in AllViolationKinds().
func inferViolationLayer(kind types.ViolationKind) string {
	if spec, ok := types.ViolKindSpecFor(kind); ok && spec.Layer != "" {
		return spec.Layer
	}
	// Unknown / unregistered kinds default to "contract_check" (the
	// legacy switch's own fallthrough).
	return "contract_check"
}

// extractBlockIDFromDetail tries to pull a block id out of the
// violation Detail string when present. Conservative — only
// matches the canonical block-id detail patterns that the validators
// emit; returns "" on no match.
func extractBlockIDFromDetail(detail string) string {
	for _, marker := range []string{`block id="`, `principal block "`} {
		idx := strings.Index(detail, marker)
		if idx < 0 {
			continue
		}
		rest := detail[idx+len(marker):]
		end := strings.Index(rest, `"`)
		if end < 0 {
			continue
		}
		return rest[:end]
	}
	return ""
}

// inferFieldPathFromKind suggests the V2 field path the LLM should
// edit to fix this violation. Conservative — only specific kinds
// have stable field paths; the rest fall back to the empty string
// (LLM uses Detail/Repair prose instead).
//
// Field path syntax: dotted-bracket form so the LLM can match
// against its emit JSON. Examples:
//
//	blocks[id="lifecycle"].claim_use
//	blocks[id="lifecycle"].facet_ids
//	citations[*].file
//	exact_resolution.status
func inferFieldPathFromKind(kind types.ViolationKind, detail string) string {
	blockID := extractBlockIDFromDetail(detail)
	switch kind {
	case types.ViolPrincipalClaimUseMissing:
		if blockID != "" {
			return fmt.Sprintf("blocks[id=%q].claim_use", blockID)
		}
		return "blocks[*].claim_use"
	case types.ViolFacetUncovered:
		// Detail names the missing facet but the FIX is to set
		// facet_ids on some block — we can't say which one
		// here, so the LLM picks via reverse-lookup hint from R7.
		return "blocks[*].facet_ids"
	case types.ViolBlockCoverageMissing:
		// Detail names the missing block kind. Field path is
		// "add a new block of that kind".
		return "blocks (add new block kind=...)"
	case types.ViolDiagramEdgeUnsupported, types.ViolDiagramCallEdgeUnproven:
		if blockID != "" {
			return fmt.Sprintf("blocks[id=%q].diagram", blockID)
		}
		return "blocks[kind=diagram].diagram"
	case types.ViolCallChainEndpointOmitted:
		return "blocks[].items[].label"
	case types.ViolUncertaintyBlockMissing:
		return "blocks (add new block kind=caveat)"
	case types.ViolCurrentStatusVerdictMissing:
		if blockID != "" {
			return fmt.Sprintf("blocks[id=%q].current_status_verdict", blockID)
		}
		return "blocks[kind=decision].current_status_verdict"
	case types.ViolLaneBlockKindMismatch:
		if blockID != "" {
			return fmt.Sprintf("blocks[id=%q].kind", blockID)
		}
		return "blocks[*].kind"
	case types.ViolClaimFormUnsupported:
		if blockID != "" {
			return fmt.Sprintf("blocks[id=%q].claim_use.claim_form", blockID)
		}
		return "blocks[*].claim_use.claim_form"
	case types.ViolAbsenceScopeExceeded:
		return "exact_resolution OR citations[*] (add scope=negative + negative_pattern)"
	case types.ViolMissingRequestedRoleUndisclosed:
		return "missing_requested_roles[]"
	case types.ViolCitation:
		return "citations[]"
	case types.ViolMustInclude, types.ViolMustExclude:
		return "answer text"
	case types.ViolDiagramIdentifier:
		if blockID != "" {
			return fmt.Sprintf("blocks[id=%q].diagram.body", blockID)
		}
		return "blocks[kind=diagram].diagram.body"
	}
	return ""
}

// summarizeAnswerDocV2ForRetry builds a RetryStateSummary from the
// previous V2 emit. Captures the typed fields R6/R6.1 found being
// silently dropped on retry (block-level claim_use existence +
// facet_ids verbatim + surface_role) so the LLM sees what it
// already filled and the Hard Rule rendering can demand
// preservation.
func summarizeAnswerDocV2ForRetry(doc *types.AnswerDocumentV2) types.RetryStateSummary {
	if doc == nil {
		return types.RetryStateSummary{}
	}
	out := types.RetryStateSummary{
		CitationsCount:         len(doc.Citations),
		HasExactResolution:     doc.ExactResolution != nil,
		BlockCompanionLineages: types.NormalizeAnswerBlockCompanionLineages(doc.BlockCompanionLineages),
	}
	if len(doc.Citations) > 0 {
		// Top-N (cap 8) verbatim file paths so the LLM can
		// match its citations[] back to prev emit.
		max := len(doc.Citations)
		if max > 8 {
			max = 8
		}
		seen := make(map[string]bool, max)
		for i := 0; i < len(doc.Citations) && len(out.CitationFiles) < max; i++ {
			f := strings.TrimSpace(doc.Citations[i].File)
			if f == "" || seen[f] {
				continue
			}
			seen[f] = true
			out.CitationFiles = append(out.CitationFiles, f)
		}
	}
	out.BlockSummaries = make([]types.RetryBlockSummary, 0, len(doc.Blocks))
	for _, b := range doc.Blocks {
		bs := types.RetryBlockSummary{
			ID:          b.ID,
			Kind:        b.Kind,
			SurfaceRole: b.SurfaceRole,
			FacetIDs:    append([]string(nil), b.FacetIDs...),
			HasItems:    len(b.Items) > 0,
			ItemCount:   len(b.Items),
		}
		// Block-level claim_use presence (R6.1 historical drop site).
		if len(b.ClaimUses) > 0 {
			bs.HasClaimUse = true
			// Pick first non-empty form for summary.
			for _, cu := range b.ClaimUses {
				if cu.ClaimForm != "" {
					bs.ClaimForm = cu.ClaimForm
					break
				}
			}
		}
		// Phase 1-B source-fix (V2 runtime eval followup, 2026-05-04):
		// edge anchors moved out of RenderedClaimUse into typed
		// AnswerBlock.EdgeAnchors. Count them directly from the
		// new top-level field; same retry-summary semantic preserved
		// (LLM sees how many edge anchors it filled, retry pressure
		// to keep them on rewrite).
		for i := range b.EdgeAnchors {
			if b.EdgeAnchors[i].HasEdgeAnchor() {
				bs.EdgeAnchoredClaimUses++
			}
		}
		// Item-level citation count (R6.1 sibling layer).
		for _, it := range b.Items {
			if it.CitationRef >= 0 {
				bs.ItemsWithCitation++
			}
		}
		// Text preview (head 400 + tail 200 when over 600 chars).
		bs.TextPreview = textHeadTail(b.Text, 400, 200)
		out.BlockSummaries = append(out.BlockSummaries, bs)
	}
	return out
}

// textHeadTail clips s to head + tail when over head+tail+marker
// length. Returns the full string when shorter. The marker
// "…<truncated N chars>…" surfaces the omission so the LLM
// doesn't quietly assume the prev text was just head+tail.
func textHeadTail(s string, head, tail int) string {
	r := []rune(s)
	if len(r) <= head+tail+10 {
		return s
	}
	dropped := len(r) - head - tail
	return string(r[:head]) + fmt.Sprintf("…<truncated %d chars>…", dropped) + string(r[len(r)-tail:])
}

// closeFinalizeRetryChain is the production reset half of the
// populateRetryState / ResetRetryState lifecycle pair (§40.14 V7-2).
// The finalize retry chain closes when the scheduler ACCEPTS an answer
// (contract pass, soft-only accept, arbitration-restored first draft,
// strict-review-disabled accept); from that point no finalizer
// re-dispatch will render the state and no hard arm may read it, so the
// carrier is cleared together with its paired RepairExecutionPlan.
//
// Deliberately NOT called at the fail-loud exit or the degraded
// transient-failure deliveries: those paths may still recover a draft
// from RetryState.PrevEmitJSON (finalizer_auto_repair). Deliberately NOT
// called at explore re-dispatch either: clearing there would lift the
// backtrack veto before the model speaks and would zero the owner
// ping-pong counter the next populate reads.
func (o *Orchestrator) closeFinalizeRetryChain() {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	o.busCtx.Mutable.ResetRetryState()
}

// acceptFinalizeNode is the single acceptance exit of the finalize
// retry chain: the evidence-utilization log line, the node bookkeeping,
// and the retry-chain close run together so every accepted answer
// closes the chain exactly once (the census in
// hard_arm_mutable_carrier_census_test.go requires the reset to have a
// production caller). The scheduler's four acceptance branches call
// this instead of repeating the triplet.
func (o *Orchestrator) acceptFinalizeNode(state *graphState, fin *types.TaskNode, finalize *agent.StageOutput) {
	logEvidenceUtilization(o, finalize)
	if state != nil && fin != nil {
		state.markDone(fin.ID)
		o.emitNodeEnd(fin.ID, true, "")
	}
	o.closeFinalizeRetryChain()
}
