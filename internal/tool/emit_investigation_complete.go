package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitInvestigationComplete is the explorer's explicit completion
// signal. When the LLM has collected enough evidence to answer the
// user's question, it calls this tool to tell the system "move on
// to extraction and finalization". This replaces the implicit
// completion detection that relied on ShouldStop heuristics and
// soft-stop interception.
//
// The tool validates the declaration: confidence must be "high" or
// "medium" — a "low" confidence call is rejected so the LLM
// continues investigating instead of prematurely stopping.
//
// On success, the tool writes a flag on MutableState that the
// explorer's ShouldStop reads to terminate the ReAct loop, and that
// ParseOutput reads to set HasEnoughFacts.
type EmitInvestigationComplete struct {
	ReadOnly
	NonEvidenceTool
}

func (t *EmitInvestigationComplete) Name() string { return "emit_investigation_complete" }

func (t *EmitInvestigationComplete) Description() string {
	return "Signal that the investigation is complete and the system should " +
		"move to the extraction and finalization stages. Call this ONCE when " +
		"you have collected enough evidence to answer the user's question. " +
		"Do NOT call this if you still have files to read or hypotheses to verify. " +
		"Requires a reason and a confidence level (high or medium)."
}

func (t *EmitInvestigationComplete) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"reason": {
				"type": "string",
				"description": "Why you believe investigation is complete — e.g. 'all hypotheses have supporting evidence' or 'the answer chain is fully traced from entry to return value'."
			},
			"confidence": {
				"type": "string",
				"enum": ["high", "medium"],
				"description": "Your confidence that the collected evidence is sufficient. 'low' is not accepted — continue investigating instead."
			},
			"absence_justification": {
				"type": "string",
				"description": "OPTIONAL. Set this ONLY when the answer is an honest 'zero' / 'no X' / 'nothing found' that has no file:line to cite (e.g. 'how many .py files?' answered 0, 'does handler X exist?' answered no). A single short sentence explaining why the answer is genuinely empty. Leave unset for every non-absence answer. This is a declarative claim, not a system override: the framework still audits that at least one investigation-class tool (grep / exec_command / list_files / read_file / repo_map) ran successfully before accepting the waiver."
			}
		},
		"required": ["reason", "confidence"]
	}`)
}

type emitInvestigationCompleteParams struct {
	Reason               string `json:"reason"`
	Confidence           string `json:"confidence"`
	AbsenceJustification string `json:"absence_justification,omitempty"`
}

func (t *EmitInvestigationComplete) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   "emit_investigation_complete rejected: no mutable state (sub-agent context)",
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}

	var p emitInvestigationCompleteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   fmt.Sprintf("emit_investigation_complete: invalid params: %v", err),
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}

	conf := strings.ToLower(strings.TrimSpace(p.Confidence))
	if conf != "high" && conf != "medium" {
		return types.ToolResult{
			ToolName: t.Name(),
			Summary: fmt.Sprintf(
				"emit_investigation_complete rejected: confidence=%q is not accepted. "+
					"Only 'high' or 'medium' are valid. If you are unsure, continue "+
					"investigating — read more files, run more greps, collect more evidence.",
				p.Confidence),
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}

	reason := strings.TrimSpace(p.Reason)
	if reason == "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   "emit_investigation_complete rejected: reason is required",
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}

	// Grounding gates. Two independent floors evaluated in AND:
	//
	//   1. GroundingFloor — (grounded + recovered) / total. Blocks
	//      mostly-speculative investigations.
	//   2. Tier1Floor — grounded-via-TierLineText / total. Blocks
	//      "pure-recovery" investigations where the LLM never
	//      actually read a file; the recovery tiers filled every
	//      LineStart from the repomap graph but the finalizer
	//      grounder (stricter Tier 2) cannot re-cite the same
	//      anchors, leaving the pipeline in a loop.
	//
	// Items emitted before the gate apply cumulatively in this
	// dispatch's Mutable buffer.
	policy := CurrentGroundingPolicy()
	if msg, ok := groundingGateReject(ctx, policy.GroundingFloor); !ok {
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   msg,
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}
	if msg, ok := tier1GateReject(ctx, policy.Tier1Floor); !ok {
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   msg,
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}

	// Declarative absence claim. Stored on Mutable so the orchestrator
	// can waive citation-floor gates for honest-zero answers. The
	// audit (hasAnyInvestigationSuccess) still runs — an LLM cannot
	// escape by declaring absence with zero tool work.
	//
	// Absence-vs-grounded-evidence contradiction gate. The LLM has
	// previously learned to shortcut citation-floor gates by tacking
	// absence_justification onto every emit_investigation_complete
	// call. Reject the combination when the evidence buffer already
	// contains ≥1 grounded or recovered item — by definition that is
	// not a zero answer, and accepting the claim bypasses the finalize
	// citation gate for a question that DOES have file:line anchors.
	// The rejection message tells the LLM exactly what to do: drop
	// the field and re-emit. This runs BEFORE SetInvestigationComplete
	// so the LLM sees the error and corrects in the same dispatch.
	justification := strings.TrimSpace(p.AbsenceJustification)
	if justification != "" {
		if evidence := ctx.Mutable.EmittedEvidence(); hasGroundedOrRecovered(evidence) {
			return types.ToolResult{
				ToolName: t.Name(),
				Summary: "emit_investigation_complete rejected: absence_justification is reserved for honest-zero answers " +
					"(the question genuinely has nothing to cite — e.g. 'how many .py files?' → 0, 'does handler X exist?' → no). " +
					"This investigation already recorded grounded/recovered evidence items via emit_evidence, so the answer is NOT an absence. " +
					"Remove absence_justification and re-call emit_investigation_complete with just reason + confidence.",
				Success:   false,
				Timestamp: time.Now(),
			}, nil
		}
	}

	// CGEC E1: pre-complete contract simulation. Before flipping the
	// investigationComplete flag, simulate whether the finalizer's
	// AnswerContract would actually pass on the current evidence +
	// ReadSet snapshot. The two cheap predictive checks:
	//
	//   (a) PendingReads non-empty — the chain promotion enforcer
	//       or a previous emit_answer_document grounder reject
	//       queued forced reads. Completing now means the LLM
	//       skipped them; force a downgrade.
	//
	//   (b) Cite-eligible evidence count below the analyzer's
	//       MinCitations floor — the LLM hasn't gathered enough
	//       file:line anchors to satisfy citation_count_ge.
	//
	// Either failure causes the tool to return a downgraded result
	// (Success=true so the LLM sees the explanation but does NOT
	// flip investigationComplete). The explorer's ShouldStop sees
	// the flag still false and continues the loop.
	if downgrade := preCompleteContractCheck(ctx, justification); downgrade != "" {
		if ctx != nil && ctx.Mutable != nil {
			ctx.Mutable.EvidenceClosure().BumpPreCompleteDowngrades(1)
		}
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   downgrade,
			Success:   true, // soft signal so the loop continues without surfacing a tool error
			Timestamp: time.Now(),
		}, nil
	}

	ctx.Mutable.SetInvestigationComplete(reason)
	summary := fmt.Sprintf("Investigation marked complete (confidence=%s): %s", conf, reason)
	if justification != "" {
		ctx.Mutable.SetAbsenceJustification(justification)
		summary += fmt.Sprintf(" | absence_justification: %s", justification)
	}

	return types.ToolResult{
		ToolName:  t.Name(),
		Summary:   summary,
		Success:   true,
		Timestamp: time.Now(),
	}, nil
}

// investigationCompletePolicy holds the operator-configured policy
// from codrax.yaml's agent_investigation_complete_policy. cmd/root.go
// calls SetInvestigationCompletePolicy at startup. Mirrors the same
// SetXxx pattern as SetBlobLimits / SetAnalysisLimits / SetGroundingPolicy.
//
// Default is the empty string (effectively "soft" — preCompleteContractCheck
// gates fire normally). When set to "override", the pre-complete
// gates are skipped to honor the per-task scheduler's "skip all
// criteria" mode (see orchestrator.go:454-468).
var investigationCompletePolicy string

// SetInvestigationCompletePolicy is the configuration entrypoint
// from cmd/root.go. Pass the empty string to restore default
// behavior.
func SetInvestigationCompletePolicy(policy string) {
	investigationCompletePolicy = strings.TrimSpace(policy)
}

// CurrentInvestigationCompletePolicy returns the active policy
// string. Used by the pre-complete simulator and by tests that need
// to assert / restore the global.
func CurrentInvestigationCompletePolicy() string {
	return investigationCompletePolicy
}

// preCompleteContractCheck is the CGEC E1 simulator. Returns an
// empty string when the LLM may proceed to mark complete, or a
// human-readable downgrade message describing exactly what is
// missing. The caller treats a non-empty return as "do NOT call
// SetInvestigationComplete; surface this message in the tool
// result so the LLM sees what to do next".
//
// Honors the agent_investigation_complete_policy setting: when
// "override", the pre-complete check is skipped because the
// orchestrator's DAG scheduler will mark every explore-type node
// done immediately on the in-flight emit_investigation_complete,
// bypassing every criterion gate. Running pre-complete gates in
// "override" mode would contradict the operator's explicit
// "skip all criteria" policy.
//
// Two predictive checks (cheap, framework-side, no LLM in the loop):
//
//	(a) PendingReads — files queued by chain promotion or a previous
//	    finalizer grounder reject. If any are still outstanding the
//	    next finalize attempt will hit the same citation drop again.
//
//	(b) Citation-floor preflight — when AnswerContract.CitationReq.
//	    Required is true, the evidence buffer must contain at least
//	    one cite-eligible item whose Source is also in ReadSet.
//	    Else citation_count_ge will fail.
//
// Absence-justified investigations (justification non-empty) are
// exempted from check (b): the absence carve-out already waives
// MinCitations, and the LLM has explicitly told the system it cannot
// cite anything.
func preCompleteContractCheck(ctx *types.BusContext, justification string) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	// Honor agent_investigation_complete_policy=override. The DAG
	// scheduler will skip all criteria when this policy is set, so
	// running the pre-complete gates would contradict operator
	// intent. soft / strict / unset all leave the gates in force.
	if investigationCompletePolicy == "override" {
		return ""
	}
	closure := ctx.Mutable.EvidenceClosure()
	pending := closure.PendingReads()

	// Check (a): forced reads still outstanding.
	if len(pending) > 0 {
		var b strings.Builder
		b.WriteString("emit_investigation_complete DOWNGRADED — pending forced reads block the closure.\n\n")
		b.WriteString("The framework queued the following files (because chains anchored here or previous citations dropped) and the LLM has not read them yet:\n")
		max := 6
		for i, p := range pending {
			if i >= max {
				fmt.Fprintf(&b, "  ... and %d more\n", len(pending)-max)
				break
			}
			fmt.Fprintf(&b, "  - %s — %s\n", p.File, p.Rationale)
		}
		b.WriteString("\nRead these files via read_file and then re-call emit_investigation_complete. Marking complete now will drop every chain anchored in them.")
		return b.String()
	}

	// Check (b): citation-floor preflight. Requires AnalysisIR.
	if justification != "" {
		// Absence answer waives the floor by contract; bypass.
		return ""
	}
	ir := ctx.AnalysisIR
	if ir == nil {
		return ""
	}
	if !ir.AnswerContract.CitationReq.Required {
		return ""
	}
	min := ir.AnswerContract.CitationReq.MinCitations
	if min <= 0 {
		min = 1
	}
	readSet := closure.ReadSet()
	evidence := ctx.Mutable.EmittedEvidence()
	eligible := 0
	for _, e := range evidence {
		if e.Source == "" || e.LineStart <= 0 {
			continue
		}
		if len(readSet) > 0 && !readSet[e.Source] {
			continue
		}
		eligible++
	}
	if eligible >= min {
		return ""
	}
	var b strings.Builder
	b.WriteString("emit_investigation_complete DOWNGRADED — pre-complete citation preflight failed.\n\n")
	fmt.Fprintf(&b, "The AnswerContract requires ≥%d citation(s) but the current evidence buffer has only %d cite-eligible item(s) (Source non-empty AND in Turn A ReadSet).\n",
		min, eligible)
	b.WriteString("Continue the investigation: emit more file:line evidence anchored in files Turn A actually read, or read additional files first.")
	return b.String()
}

// evidenceTally breaks an evidence slice into grounding-status
// counts + the sub-slices a caller typically needs for rendering
// repair hints. Populated once by tallyEvidence and consumed by all
// three gate checks (groundingGateReject, tier1GateReject) + the
// hasGroundedOrRecovered predicate below, so the dispatch rules for
// GroundingStatus / GroundingTier live in one place.
//
// Legacy items with empty GroundingStatus (pre-session-5 concrete_value
// scans) count as Tier-1-grounded — they are deterministic facts, not
// LLM claims, and should NOT push either floor down.
type evidenceTally struct {
	total         int
	tier1         int
	tier2Grounded int // GroundingGrounded but GroundingTier != TierLineText
	recovered     int
	ungrounded    int
	ungroundedItems []types.EvidenceItem
	recoveredItems  []types.EvidenceItem
}

// tallyEvidence classifies each item in the evidence buffer and
// returns the populated tally. Single source of truth for how the
// pipeline counts grounding outcomes.
func tallyEvidence(evidence []types.EvidenceItem) evidenceTally {
	var t evidenceTally
	for _, e := range evidence {
		t.total++
		switch e.GroundingStatus {
		case types.GroundingGrounded:
			if e.GroundingTier == types.TierLineText {
				t.tier1++
			} else {
				t.tier2Grounded++
			}
		case types.GroundingRecovered:
			t.recovered++
			t.recoveredItems = append(t.recoveredItems, e)
		case types.GroundingUngrounded:
			t.ungrounded++
			t.ungroundedItems = append(t.ungroundedItems, e)
		default:
			t.tier1++
		}
	}
	return t
}

// acceptedTotal is (grounded + recovered) — the "at least it lined up
// somewhere" set used by the existing grounded-ratio gate.
func (t evidenceTally) acceptedTotal() int { return t.tier1 + t.tier2Grounded + t.recovered }

// hasAny reports whether at least one item is grounded or recovered.
// Powers the absence-vs-grounded contradiction gate.
func (t evidenceTally) hasAny() bool { return t.acceptedTotal() > 0 }

// groundingGateReject returns (message, ok). When ok=false, the
// returned message describes the gate miss and lists the ungrounded
// items with concrete repair options. When ok=true, the gate passed
// or was disabled (floor == 0).
func groundingGateReject(ctx *types.BusContext, floor float64) (string, bool) {
	if floor <= 0 {
		return "", true
	}
	evidence := ctx.Mutable.EmittedEvidence()
	if len(evidence) == 0 {
		// No emit_evidence calls at all — tool-only investigation is
		// still legitimate (exec_command one-shot, grep-only answer
		// for simple list questions). Accept.
		return "", true
	}
	t := tallyEvidence(evidence)
	if t.total == 0 {
		return "", true
	}
	ratio := float64(t.acceptedTotal()) / float64(t.total)
	if ratio >= floor {
		return "", true
	}
	leads := t.ungroundedItems
	grounded := t.tier1 + t.tier2Grounded
	recovered := t.recovered
	total := t.total
	var b strings.Builder
	fmt.Fprintf(&b,
		"emit_investigation_complete rejected: grounding ratio %.0f%% (%d grounded + %d recovered / %d total) < floor %.0f%%.\n\n",
		ratio*100, grounded, recovered, total, floor*100)
	b.WriteString("Ungrounded items cannot be emitted as citations:\n")
	maxList := 10
	for i, it := range leads {
		if i >= maxList {
			fmt.Fprintf(&b, "  ... and %d more\n", len(leads)-maxList)
			break
		}
		note := strings.TrimSpace(it.GroundingNote)
		if note == "" {
			note = "no tier accepted the citation"
		}
		anchor := it.AnchorSymbol
		if anchor == "" {
			anchor = "-"
		}
		fmt.Fprintf(&b, "  [%d] %s @ %s:%d (anchor_kind=%s, anchor_symbol=%s) — %s\n",
			i+1, it.Kind, it.Source, it.LineStart, it.AnchorKind, anchor, note)
	}
	b.WriteString("\nRepair options per item:\n")
	b.WriteString("  (A) call read_file on the source near the cited line so Tier 1 (line_text) can validate.\n")
	b.WriteString("  (B) re-emit with a different anchor_symbol — the identifier the grounder should find on that line.\n")
	b.WriteString("  (C) if you provided no snippet, add one (1-2 lines of actual code) so the snippet_fuzzy recovery tier can re-anchor.\n")
	b.WriteString("  (D) drop the item entirely if it was speculative — emit_evidence rejects of speculation do not hurt the investigation.\n")
	return b.String(), false
}

// tier1GateReject rejects emit_investigation_complete when the
// fraction of Tier-1-proven items (GroundingStatus=Grounded AND
// GroundingTier=TierLineText) falls below floor. Zero floor disables
// the gate (session-7 backward compat). Powers "don't let the LLM
// complete an investigation it never actually read" — the finalizer
// grounder's stricter Tier 2 would drop every citation anyway and
// stall the pipeline.
func tier1GateReject(ctx *types.BusContext, floor float64) (string, bool) {
	if floor <= 0 {
		return "", true
	}
	evidence := ctx.Mutable.EmittedEvidence()
	if len(evidence) == 0 {
		return "", true
	}
	t := tallyEvidence(evidence)
	if t.total == 0 {
		return "", true
	}
	ratio := float64(t.tier1) / float64(t.total)
	if ratio >= floor {
		return "", true
	}
	var b strings.Builder
	fmt.Fprintf(&b,
		"emit_investigation_complete rejected: Tier-1 proven ratio %.0f%% (%d grounded-via-line_text / %d total) < floor %.0f%%.\n\n",
		ratio*100, t.tier1, t.total, floor*100)
	b.WriteString("The pipeline's finalizer grounder enforces a STRICTER citation check than the evidence grounder: ")
	b.WriteString("the finalizer only accepts citations whose file:line falls inside a known symbol's body OR is corroborated by your read_file history. ")
	b.WriteString("An investigation that never read_file'd the sources it cited (pure repomap-graph recovery) will have its citations dropped at finalize time, leaving the pipeline in a retry loop.\n\n")
	b.WriteString("Repair: call read_file on the sources for the items below so Tier 1 (line_text) can re-ground them.\n")
	maxList := 6
	for i, it := range t.recoveredItems {
		if i >= maxList {
			fmt.Fprintf(&b, "  ... and %d more recovered-only items\n", len(t.recoveredItems)-maxList)
			break
		}
		anchor := it.AnchorSymbol
		if anchor == "" {
			anchor = "-"
		}
		fmt.Fprintf(&b, "  [%d] %s @ %s:%d (anchor_kind=%s, anchor_symbol=%s) — read_file %s near line %d to convert Recovered → Grounded\n",
			i+1, it.Kind, it.Source, it.LineStart, it.AnchorKind, anchor, it.Source, it.LineStart)
	}
	return b.String(), false
}

// hasGroundedOrRecovered reports whether the evidence buffer contains
// at least one item whose grounder verdict is grounded or recovered.
// Drives the absence-vs-grounded contradiction gate in Execute.
func hasGroundedOrRecovered(items []types.EvidenceItem) bool {
	return tallyEvidence(items).hasAny()
}
