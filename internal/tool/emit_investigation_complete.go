package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
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
			closure := ctx.Mutable.EvidenceClosure()
			closure.BumpPreCompleteDowngrades(1)
			// Session 11 F1: pre-complete downgrade is a compound
			// signal — something (missing reads, zero citations, shape
			// mismatch, unverified finds) blocked completion. The
			// downgrade message body carries the reason for the
			// operator; we record a ledger entry so F2 can aggregate
			// "closure blocked N times with same root" into a direct
			// IR patch request. Confidence 0.70 — the downgrade
			// doesn't pinpoint which IR field without more context,
			// so we leave CitationReq as the default blame and let
			// the paired Repair (always raised by preCompleteContractCheck)
			// carry the kind-specific detail.
			closure.AppendViolation(types.Violation{
				Kind:   types.ViolPreCompleteDowngrade,
				Detail: "pre-complete simulator rejected emit_investigation_complete",
				Stage:  string(types.StageExplore),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "CitationReq",
					Reason:     "closure snapshot fails preflight; evidence insufficient or citations outside ReadSet",
					Confidence: 0.70,
				},
			})
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

	// CGEC C2: check (e) — evidence Source falls on a file the
	// analyzer named but findings_validator could not verify. The
	// LLM's evidence pool is citing a file the framework has flagged
	// as "analyzer hallucination". Downgrade and push for grep
	// rediscovery so the LLM can disprove or confirm the file
	// actually exists with the expected symbol. Runs BEFORE the
	// PendingReads check so operator sees the root-cause signal
	// first (unverified findings are stronger evidence of bad
	// grounding than "LLM didn't read file X").
	if unverified := closure.UnverifiedFindings(); len(unverified) > 0 {
		unverifiedPaths := make(map[string]string)
		for _, u := range unverified {
			if u.Kind == "path" {
				unverifiedPaths[u.Token] = u.Reason
			}
		}
		if len(unverifiedPaths) > 0 {
			var hits []string
			for _, ev := range ctx.Mutable.EmittedEvidence() {
				if reason, bad := unverifiedPaths[ev.Source]; bad {
					hits = append(hits, fmt.Sprintf("%s (%s)", ev.Source, reason))
				}
			}
			if len(hits) > 0 {
				// Emit RepairExpandSearch so the retry hint surfaces
				// broaden-keyword guidance. The Rationale mentions
				// the unverified files explicitly so the LLM knows
				// which claims to disprove.
				var kws []string
				if ctx.AnalysisIR != nil {
					kws = append(kws, ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords...)
				}
				closure.AddRepair(types.RepairDirective{
					Kind:      types.RepairExpandSearch,
					Keywords:  kws,
					Rationale: fmt.Sprintf("evidence cites %d file(s) findings_validator flagged as unverified: %s — re-grep the repo to confirm the correct locations", len(hits), strings.Join(hits, "; ")),
					Origin:    "pre_complete.evidence_on_unverified_path",
				})
				logging.Info("[CGEC] C2 downgrade: evidence cites unverified path(s) count=%d", len(hits))
				var b strings.Builder
				b.WriteString("emit_investigation_complete DOWNGRADED — evidence cites files the analyzer findings_validator flagged as unverified.\n\n")
				b.WriteString("The following evidence sources were unable to be verified against the repo graph:\n")
				for _, h := range hits {
					b.WriteString("  - " + h + "\n")
				}
				b.WriteString("\nRe-run grep to confirm the correct file paths (the analyzer may have hallucinated them). After finding real evidence anchors, re-call emit_investigation_complete.")
				return b.String()
			}
		}
	}

	// Check (a): forced reads still outstanding.
	// CGEC D3: partition PendingRead entries by ScannedSet
	// membership. Files the explorer's pre-scan saw (or grep'd /
	// read during this run — IsScanned is lenient when ScannedSet
	// is empty) render as "Forced Read List" directing the LLM to
	// read them. Files NOT in ScannedSet render as "Suspicious
	// Anchors" — likely ghost paths the LLM should either verify
	// with grep OR ignore entirely. The two-section layout makes
	// the LLM's action different per bucket: READ the scanned
	// ones, INVESTIGATE (or reject) the unscanned ones.
	if len(pending) > 0 {
		var scanned, suspicious []types.PendingRead
		for _, p := range pending {
			if closure.IsScanned(p.File) {
				scanned = append(scanned, p)
			} else {
				suspicious = append(suspicious, p)
			}
		}
		var b strings.Builder
		b.WriteString("emit_investigation_complete DOWNGRADED — pending forced reads block the closure.\n\n")
		max := 6
		if len(scanned) > 0 {
			b.WriteString("## Forced Read List (scanned files the LLM has not read yet)\n")
			for i, p := range scanned {
				if i >= max {
					fmt.Fprintf(&b, "  ... and %d more\n", len(scanned)-max)
					break
				}
				fmt.Fprintf(&b, "  - %s — %s\n", p.File, p.Rationale)
			}
			b.WriteString("\n")
		}
		if len(suspicious) > 0 {
			b.WriteString("## Suspicious Anchors (files NOT in the explorer's ScannedSet — possibly hallucinated paths)\n")
			for i, p := range suspicious {
				if i >= max {
					fmt.Fprintf(&b, "  ... and %d more\n", len(suspicious)-max)
					break
				}
				fmt.Fprintf(&b, "  - %s — %s\n", p.File, p.Rationale)
			}
			b.WriteString("Either grep for the real path if this is a legitimate reference, or reject any chain / citation that depends on it.\n\n")
		}
		b.WriteString("Read the scanned files (if any) and/or verify the suspicious anchors, then re-call emit_investigation_complete. Marking complete now will drop every chain anchored in them.")
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
		// CGEC E3: check (f) — when the AnswerSubject has
		// reasonable confidence and the explorer's chain ranker
		// recorded SubjectMatch scores for chain terminals, assert
		// that at least ONE chain scored above the rebind floor
		// (0.4). If every chain scores below, the investigation is
		// producing evidence about the WRONG kind of token
		// (e.g. AgentName chains when question asks for SkillName).
		// Emit RepairRebindSubject so the retry prompt tells the
		// explorer to constrain its chain production to the right
		// subject kind. Confidence gate mirrors rankChainsBySubject's
		// existing G5 trigger (conf >= 0.5 + best < 0.4). Runs
		// BEFORE the subject/shape mismatch check — rebind is the
		// more fundamental fix (shape matters only if subject is
		// right).
		const (
			rebindFloor   = 0.4
			highConfFloor = 0.5
		)
		subj := ir.RequestModel.AnswerSubject
		matches := closure.AllSubjectMatches()
		if len(matches) > 0 && subj.Confidence >= highConfFloor && subj.Kind != types.SubjectUnknown {
			var bestScore float64
			for _, v := range matches {
				if v > bestScore {
					bestScore = v
				}
			}
			if bestScore < rebindFloor {
				closure.AddRepair(types.RepairDirective{
					Kind:      types.RepairRebindSubject,
					Subject:   string(subj.Kind),
					Rationale: fmt.Sprintf("pre-complete: %d chain(s) scored against expected subject %s; none above %.1f (best=%.2f)", len(matches), subj.Kind, rebindFloor, bestScore),
					Origin:    "pre_complete.subject_match_low",
				})
				logging.Info("[CGEC] E3 rebind_subject: origin=pre_complete.subject_match_low kind=%s best=%.2f floor=%.2f", subj.Kind, bestScore, rebindFloor)
			}
		}
		// CGEC B2b: eligible evidence is sufficient in quantity, but
		// check for a static mismatch between AnswerSubject (what
		// kind of literal the answer should be) and RequiredAnswerShape
		// (what shape the finalizer will emit). reconcileShape runs
		// at analyzer time but ONLY handles ShapeConfigValue →
		// ShapeValue for source-code literal subjects; other shape
		// mismatches (e.g. subject=SkillName + shape=ShapeExplanation)
		// slip through. Emit RepairSwapShape so the retry hint
		// surfaces the conflict explicitly — does NOT downgrade the
		// current emit_investigation_complete because the evidence
		// IS present; subsequent finalize + contract check will
		// decide whether to accept.
		if mismatch, fromShape, toShape := detectSubjectShapeMismatch(ir); mismatch {
			closure.AddRepair(types.RepairDirective{
				Kind:      types.RepairSwapShape,
				Subject:   fmt.Sprintf("from=%s,to=%s", fromShape, toShape),
				Rationale: fmt.Sprintf("AnswerSubject=%s (source-code literal) but RequiredAnswerShape=%s — finalizer should produce %s instead", ir.RequestModel.AnswerSubject.Kind, fromShape, toShape),
				Origin:    "pre_complete.subject_shape_mismatch",
			})
			logging.Info("[CGEC] B2b shape_swap: origin=pre_complete.subject_shape_mismatch from=%s to=%s", fromShape, toShape)
			// Session 11 F1: shape mismatch detected at pre-complete
			// boundary. High-confidence (0.85) signal that the IR's
			// answer_shape disagrees with the subject.kind — F2 can
			// aggregate with finalizer-side B2a events to push an IR
			// patch. Pre-complete sees this earlier than the finalizer
			// so the signal arrives in the ledger before the retry cycle.
			closure.AppendViolation(types.Violation{
				Kind:   types.ViolShapeSwap,
				Detail: fmt.Sprintf("pre-complete B2b: AnswerSubject=%s vs RequiredAnswerShape=%s (→ %s)", ir.RequestModel.AnswerSubject.Kind, fromShape, toShape),
				Stage:  string(types.StageExplore),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_shape",
					Reason:     fmt.Sprintf("subject.kind=%s incompatible with shape=%s at pre-complete", ir.RequestModel.AnswerSubject.Kind, fromShape),
					Confidence: 0.85,
				},
			})
		}
		return ""
	}
	// CGEC B1c: evidence short of MinCitations AND AnalysisIR.RequestModel
	// has keywords the LLM has been trying. The common root cause is
	// the keywords find too few files — tell the LLM to broaden the
	// grep coverage (stems / synonyms) before re-calling
	// emit_investigation_complete.
	var kws []string
	if ir.RequestModel.AnalyzerHints.Keywords != nil {
		kws = append(kws, ir.RequestModel.AnalyzerHints.Keywords...)
	}
	if eligible+1 < min && len(kws) > 0 {
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Keywords:  kws,
			Rationale: fmt.Sprintf("evidence buffer has only %d of %d required cite-eligible items — broaden grep coverage with stems / conceptual synonyms of the analyzer keywords above", eligible, min),
			Origin:    "pre_complete.citation_floor_low",
		})
		logging.Info("[CGEC] B1c expand_search: origin=pre_complete.citation_floor_low eligible=%d min=%d keywords=%d", eligible, min, len(kws))
	}
	var b strings.Builder
	b.WriteString("emit_investigation_complete DOWNGRADED — pre-complete citation preflight failed.\n\n")
	fmt.Fprintf(&b, "The AnswerContract requires ≥%d citation(s) but the current evidence buffer has only %d cite-eligible item(s) (Source non-empty AND in Turn A ReadSet).\n",
		min, eligible)
	b.WriteString("Continue the investigation: emit more file:line evidence anchored in files Turn A actually read, or read additional files first.")
	return b.String()
}

// detectSubjectShapeMismatch returns true when the AnswerSubject is
// a source-code literal kind (skill_name, agent_name, function_name,
// type_name, interface, handler_route, return_value) but the
// RequiredAnswerShape is one that cannot carry a single literal
// (ShapeExplanation / ShapeStepList / ShapeBoolean / ShapeListOfSymbols).
// ShapeValue / ShapeConfigValue are compatible targets; reconcileShape
// at analyzer time already handles ConfigValue→Value so we don't
// flag that case here.
//
// Returns (true, currentShape, recommendedShape) so the caller can
// emit a structured RepairSwapShape with "from=X,to=Y" subject.
func detectSubjectShapeMismatch(ir *types.AnalysisIR) (bool, types.AnswerShape, types.AnswerShape) {
	if ir == nil {
		return false, "", ""
	}
	subj := ir.RequestModel.AnswerSubject.Kind
	shape := ir.AnswerContract.RequiredAnswerShape
	switch subj {
	case types.SubjectFunctionName, types.SubjectTypeName,
		types.SubjectInterface, types.SubjectHandlerRoute,
		types.SubjectReturnValue:
	default:
		return false, "", ""
	}
	switch shape {
	case types.ShapeValue, types.ShapeConfigValue:
		return false, "", ""
	}
	return true, shape, types.ShapeValue
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
