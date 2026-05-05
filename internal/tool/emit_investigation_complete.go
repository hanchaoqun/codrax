package tool

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/tool/multipath"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitInvestigationCompleteDowngradePrefix is the Summary prefix this
// tool writes when preCompleteContractCheck rejects a completion
// attempt. The tool returns Success=true (so the LLM sees the
// explanation in its tool-result history) while leaving
// MutableState.InvestigationComplete FALSE. Mid-loop observers that
// branch on "Success=true + emit_investigation_complete" must filter
// this prefix out — otherwise a soft keep-alive signal is mistaken
// for a terminal completion and the ReAct loop ends before the LLM
// gets to re-invest.
const EmitInvestigationCompleteDowngradePrefix = "emit_investigation_complete DOWNGRADED"

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
			"result_kind": {
				"type": "string",
				"enum": ["resolved", "absence"],
				"description": "Structured terminal disposition for this investigation. Use 'resolved' for ordinary positive/citable answers. Use 'absence' only when the honest terminal answer is zero / no-such-target / nothing found and you are also providing absence_justification."
			},
			"absence_justification": {
				"type": "string",
				"description": "OPTIONAL. Set this ONLY when the answer is an honest 'zero' / 'no X' / 'nothing found' that has no direct exact-target definition to cite (e.g. 'how many .py files?' answered 0, 'does handler X exist?' answered no). A single short sentence explaining why the answer is genuinely empty. Leave unset for every non-absence answer. Grounded related-context anchors are allowed when they remain clearly contextual (for example a nearby config family, call chain, or architecture edge) and do not define the missing exact target. This is a declarative claim, not a system override: the framework still audits that at least one investigation-class tool (grep / exec_command / list_files / read_file / repo_map) ran successfully before accepting the waiver."
			}
		},
		"required": ["reason", "confidence", "result_kind"]
	}`)
}

type emitInvestigationCompleteParams struct {
	Reason               string `json:"reason"`
	Confidence           string `json:"confidence"`
	ResultKind           string `json:"result_kind"`
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
	resultKind := strings.ToLower(strings.TrimSpace(p.ResultKind))
	if resultKind != "resolved" && resultKind != "absence" {
		return types.ToolResult{
			ToolName: t.Name(),
			Summary: fmt.Sprintf(
				"emit_investigation_complete rejected: result_kind=%q is not accepted. "+
					"Use 'resolved' for ordinary positive/citable answers or 'absence' for honest zero / no-such-target answers.",
				p.ResultKind),
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
	justification := strings.TrimSpace(p.AbsenceJustification)
	resultKind, justification = normalizeExactAbsenceCompletion(ctx, resultKind, reason, justification)

	// G1 (Plan E, 2026-05-02) — pre-complete coverage gate.
	// When the user's question carries a structural obligation
	// (CompletenessObligation.Required OR EnumerationBoundary
	// declared) AND result_kind=resolved, refuse the emit if the
	// closure's ScannedSet (files surfaced by grep / repo_map /
	// list_files) contains files NOT in ReadSet. The signal is
	// "the explorer surfaced N candidate files but only read K<N"
	// — exactly the s5a pathology where 8 LoopController
	// implementations were grep-known but only 3 read_file'd.
	//
	// Bypass conditions:
	//   - result_kind=absence: honest zero, scanning without reading
	//     is fine (you read enough to confirm absence)
	//   - no obligation declared: pre-Plan-E behaviour preserved
	//   - ScannedSet ⊆ ReadSet: every candidate covered, full pass
	//   - Tools failed to populate ScannedSet (test-mode shortcut):
	//     no candidate signal, gate skips
	// G1 (Plan E rollout 2026-05-02) was REVERTED 2026-05-02 after
	// the s1a-125430 trace surfaced a systemic over-firing problem.
	//
	// Architectural lesson: investigation-time enforcement of
	// "exhaustive coverage" needs a precise per-question candidate
	// set (e.g. files that grep matched the user's primary entity
	// during this dispatch). closure.ScannedSet is the breadth-scan
	// keyword ranker output, which surfaces files by import-count and
	// loose similarity — for s1a's "gate.Run 的 9 项检查" question the
	// ranker offered 12+ files including emit_answer_document.go and
	// agent.go (popular but unrelated). G1 forced the explorer to
	// read every one, looping into timeout.
	//
	// Replacement: enforce on the ANSWER side, not the investigation
	// side.
	//   - G2 (emit_answer_symbol) precisely rejects completeness=
	//     lower_bound under CompletenessObligation. The signal is
	//     question-precise (a single boolean flag) instead of
	//     ranker-noisy.
	//   - G3 (emit_answer_document) precisely rejects bucket-label
	//     omission. Single-bucket questions never trigger.
	//   - The explore-skill carries soft "COVERAGE BEFORE COMPLETION"
	//     guidance (E-7) — the LLM is encouraged to read every grep
	//     hit on completeness questions, but not structurally forced.
	//
	// Future option: a precise per-question candidate tracker
	// (closure-side "files with grep-matches against analyzer's
	// PrimaryEntities") could supersede the soft guidance with a
	// hard gate. Out of scope for this commit.

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
	// dispatch's Mutable buffer. Honest-zero absence claims skip
	// these floors, then pass through the dedicated absence validation
	// below; otherwise absent targets can be rejected for having no
	// positive evidence to cite.
	if justification == "" {
		contract := answerExactResolutionContract(ctx)
		evidence := ctx.Mutable.EmittedEvidence()
		if targets := completionPendingExactTargets(ctx, contract, evidence); len(targets) > 0 {
			if !evidenceHasAnyDefiningExactTargetProof(contract, evidence, targets) {
				label := "target"
				if contract != nil && strings.TrimSpace(contract.TargetLabel) != "" {
					label = strings.TrimSpace(contract.TargetLabel)
				}
				return types.ToolResult{
					ToolName: t.Name(),
					Summary: fmt.Sprintf(
						"emit_investigation_complete rejected: the primary exact %s still has no grounded defining proof, and the emitted evidence only supports nearby or contextual material. Do not complete a positive substitute chain as result_kind=resolved. Either find an explicit grounded defining anchor (or alias/parser mapping) that names the exact %s, or keep investigating until an honest exact-absence closure is ready and then re-call emit_investigation_complete with result_kind=\"absence\" plus absence_justification for the exact %s.",
						label, label, label,
					),
					Success:   false,
					Timestamp: time.Now(),
				}, nil
			}
		}
		policy := CurrentGroundingPolicy()
		if msg, ok := groundingGateReject(ctx, policy.GroundingFloor); !ok {
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   msg,
				Success:   false,
				Timestamp: time.Now(),
			}, nil
		}
		if msg, ok := tier1GateReject(ctx, policy.Tier1Floor, resultKind, justification); !ok {
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   msg,
				Success:   false,
				Timestamp: time.Now(),
			}, nil
		}
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
	if justification != "" {
		if resultKind != "absence" {
			return types.ToolResult{
				ToolName: t.Name(),
				Summary: "emit_investigation_complete rejected: absence_justification requires result_kind=absence. " +
					"For ordinary positive/citable answers, set result_kind=resolved and omit absence_justification.",
				Success:   false,
				Timestamp: time.Now(),
			}, nil
		}
		if evidence := ctx.Mutable.EmittedEvidence(); hasGroundedOrRecovered(evidence) && !allowsContextualEvidenceForAbsence(ctx, reason, justification, evidence) {
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
	if resultKind == "absence" && justification == "" {
		return types.ToolResult{
			ToolName: t.Name(),
			Summary: "emit_investigation_complete rejected: result_kind=absence requires absence_justification. " +
				"State in one short sentence what was searched and why the terminal answer is genuinely empty.",
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}
	if resultKind == "absence" && justification != "" {
		contract := exactResolutionContractForCompletion(ctx)
		requiredFiles := types.ExactResolutionRequiredContextFiles(contract, ctx.Mutable)
		if len(requiredFiles) > 0 {
			if ctx != nil && ctx.Mutable != nil {
				closure := ctx.Mutable.EvidenceClosure()
				refreshClosureReadSnapshot(ctx, closure)
				closure.DrainSatisfiedPendingReads()
			}
			evidence := ctx.Mutable.EmittedEvidence()
			scenario := types.ScenarioGeneric
			if ctx.AnalysisIR != nil {
				scenario = ctx.AnalysisIR.RequestModel.Scenario
			}
			if !evidenceHasGroundedRelatedContextProof(contract, scenario, evidence, requiredFiles) {
				repairKind, repairFiles := exactAbsenceContextRepairKind(ctx, contract, scenario, evidence, requiredFiles)
				configTraceMultiLayer := scenario == types.ScenarioConfigTrace &&
					contract != nil &&
					contract.TargetKind == types.SubjectConfigKey &&
					types.ConfigTraceNeedsMultiLayerClosure(requiredFiles)
				validatedRoleCount := 0
				if scenario == types.ScenarioConfigTrace && contract != nil && contract.TargetKind == types.SubjectConfigKey {
					validatedRoleCount = types.ConfigTraceValidatedDiagramRoleCount(contract, requiredFiles, evidence)
				}
				summary := fmt.Sprintf(
					"emit_investigation_complete rejected: this exact-absence answer still lacks a grounded production related-context anchor from the current same-scope candidate set. Read one of these repo_map-ranked files, emit at least one grounded related_context fact from it, then re-call emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...). Pending same-scope files: %s",
					strings.Join(repairFiles, ", "),
				)
				repairOrigin := "emit_investigation_complete.exact_absence_context"
				rationale := "exact-absence closure is still missing a grounded same-scope related-context anchor; read one of these files, emit related_context evidence, then retry emit_investigation_complete."
				hint := "Read one of the queued same-scope files, emit at least one grounded `related_context` fact from it, then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
				repairCode := "exact_absence_context_anchor"
				if scenario == types.ScenarioConfigTrace && contract != nil && contract.TargetKind == types.SubjectConfigKey {
					requestedRoles := types.ConfigTraceMissingRequestedDiagramRoles(contract, requiredFiles, evidence)
					requestedRoleText := types.JoinEvidenceDiagramRoles(requestedRoles)
					if configTraceMultiLayer && validatedRoleCount > 0 {
						if requestedRoleText != "" {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks grounded precedence coverage for the user-requested role(s) `%s` from the current same-scope candidate set. Read one of these repo_map-ranked files, then re-emit grounded related_context facts that carry explicit diagram_role_hint values so downstream answers can cite a real precedence chain across every requested role. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Pending same-scope files: %s",
								requestedRoleText,
								strings.Join(repairFiles, ", "),
							)
							hint = fmt.Sprintf("Read one of the queued same-scope files, emit grounded `related_context` facts with validated precedence `diagram_role_hint` values, and keep going until the requested roles `%s` each have a grounded anchor before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`.", requestedRoleText)
						} else {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks enough grounded precedence coverage from the current same-scope candidate set. Read one of these repo_map-ranked files, then re-emit grounded related_context facts that carry explicit diagram_role_hint values (`default`, `config`, `runtime`, or `override`) so downstream answers can cite a real multi-layer precedence chain. When the same-scope search already spans multiple files, preserve at least two validated precedence roles instead of closing on a single layer. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Pending same-scope files: %s",
								strings.Join(repairFiles, ", "),
							)
							hint = "Read one of the queued same-scope files, emit grounded `related_context` facts with validated precedence `diagram_role_hint` values, and when the current scope already spans multiple files keep going until you have at least two validated precedence roles before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
						}
						repairOrigin = "emit_investigation_complete.exact_absence_precedence"
						rationale = "exact-absence config-trace closure still lacks enough grounded precedence coverage; read one of these files, emit related_context evidence with validated diagram roles, then retry emit_investigation_complete."
						repairCode = "exact_absence_precedence_anchor"
					} else {
						if requestedRoleText != "" {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks a grounded precedence anchor for the user-requested role(s) `%s` from the current same-scope candidate set. Read one of these repo_map-ranked files, then re-emit at least one grounded related_context fact with an explicit diagram_role_hint so downstream answers can cite that requested role. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Pending same-scope files: %s",
								requestedRoleText,
								strings.Join(repairFiles, ", "),
							)
							hint = fmt.Sprintf("Read one of the queued same-scope files, emit at least one grounded `related_context` fact with a validated precedence `diagram_role_hint`, and keep going until the requested roles `%s` have grounded anchors before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`.", requestedRoleText)
						} else {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks a grounded precedence-capable lineage anchor from the current same-scope candidate set. Read one of these repo_map-ranked files, then re-emit at least one grounded related_context fact that carries an explicit diagram_role_hint (`default`, `config`, `runtime`, or `override`) so downstream answers can cite a real precedence anchor. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Pending same-scope files: %s",
								strings.Join(repairFiles, ", "),
							)
							hint = "Read one of the queued same-scope files, emit at least one grounded `related_context` fact with a validated precedence `diagram_role_hint`, then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
						}
						repairOrigin = "emit_investigation_complete.exact_absence_precedence"
						rationale = "exact-absence config-trace closure still lacks a grounded precedence-capable same-scope anchor; read one of these files, emit related_context evidence with a validated diagram role, then retry emit_investigation_complete."
						repairCode = "exact_absence_precedence_anchor"
					}
				}
				if repairKind == types.RepairEmitEvidence {
					summary = fmt.Sprintf(
						"emit_investigation_complete rejected: this exact-absence answer still lacks a grounded same-scope related-context anchor on the current answer surface, but the queued same-scope file(s) are already in read coverage. Do NOT widen scope with neighboring files yet. Re-emit at least one grounded `related_context` fact from these already-read file(s), then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`. Same-scope files: %s",
						strings.Join(repairFiles, ", "),
					)
					rationale = "exact-absence closure is still missing a grounded same-scope related-context anchor, but the relevant file scope has already been read; stay on those anchors, emit grounded related_context evidence, then retry emit_investigation_complete."
					hint = "Do NOT read neighboring files yet. Re-emit at least one grounded `related_context` fact from these already-read same-scope file(s), then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
					repairCode = "exact_absence_context_evidence"
				}
				if repairKind == types.RepairEmitEvidence && scenario == types.ScenarioConfigTrace && contract != nil && contract.TargetKind == types.SubjectConfigKey {
					requestedRoles := types.ConfigTraceMissingRequestedDiagramRoles(contract, requiredFiles, evidence)
					requestedRoleText := types.JoinEvidenceDiagramRoles(requestedRoles)
					if configTraceMultiLayer && validatedRoleCount > 0 {
						if requestedRoleText != "" {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks grounded precedence coverage for the user-requested role(s) `%s` on the current answer surface, but the queued same-scope file(s) are already in read coverage. Do NOT widen scope with neighboring files yet. Re-emit grounded `related_context` facts from these already-read file(s) with validated precedence `diagram_role_hint` values until each requested role has a grounded anchor, then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Same-scope files: %s",
								requestedRoleText,
								strings.Join(repairFiles, ", "),
							)
							hint = fmt.Sprintf("Do NOT read neighboring files yet. Re-emit grounded `related_context` facts from these already-read same-scope file(s) with validated precedence `diagram_role_hint` values until the requested roles `%s` each have grounded anchors before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`.", requestedRoleText)
						} else {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks enough grounded precedence coverage on the current answer surface, but the queued same-scope file(s) are already in read coverage. Do NOT widen scope with neighboring files yet. Re-emit grounded `related_context` facts from these already-read file(s) with validated precedence `diagram_role_hint` values (`default`, `config`, `runtime`, or `override`), and when the current same-scope span already covers multiple files preserve at least two validated roles before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Same-scope files: %s",
								strings.Join(repairFiles, ", "),
							)
							hint = "Do NOT read neighboring files yet. Re-emit grounded `related_context` facts from these already-read same-scope file(s) with validated precedence `diagram_role_hint` values, and when the current same-scope span already covers multiple files keep going until you have at least two validated roles before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
						}
						rationale = "exact-absence config-trace closure still lacks enough grounded precedence coverage, but the relevant file scope has already been read; stay on those anchors, re-emit related_context evidence with validated diagram roles, then retry emit_investigation_complete."
						repairCode = "exact_absence_precedence_evidence"
					} else {
						if requestedRoleText != "" {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks a grounded precedence anchor for the user-requested role(s) `%s` on the current answer surface, but the queued same-scope file(s) are already in read coverage. Do NOT widen scope with neighboring files yet. Re-emit grounded `related_context` facts from these already-read file(s) with validated precedence `diagram_role_hint` values until each requested role has a grounded anchor, then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Same-scope files: %s",
								requestedRoleText,
								strings.Join(repairFiles, ", "),
							)
							hint = fmt.Sprintf("Do NOT read neighboring files yet. Re-emit grounded `related_context` facts from these already-read same-scope file(s) with validated precedence `diagram_role_hint` values until the requested roles `%s` each have grounded anchors before retrying `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`.", requestedRoleText)
						} else {
							summary = fmt.Sprintf(
								"emit_investigation_complete rejected: this exact-absence config-trace answer still lacks a grounded precedence-capable lineage anchor on the current answer surface, but the queued same-scope file(s) are already in read coverage. Do NOT widen scope with neighboring files yet. Re-emit at least one grounded `related_context` fact from these already-read file(s) with a validated precedence `diagram_role_hint` (`default`, `config`, `runtime`, or `override`), then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`. `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Same-scope files: %s",
								strings.Join(repairFiles, ", "),
							)
							hint = "Do NOT read neighboring files yet. Re-emit at least one grounded `related_context` fact from these already-read same-scope file(s) with a validated precedence `diagram_role_hint`, then retry `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`."
						}
						rationale = "exact-absence config-trace closure still lacks a grounded precedence-capable same-scope anchor, but the relevant file scope has already been read; stay on those anchors, re-emit related_context evidence with a validated diagram role, then retry emit_investigation_complete."
						repairCode = "exact_absence_precedence_evidence"
					}
				}
				requiredDiagramRoles := types.JoinEvidenceDiagramRoles(types.ConfigTraceRequestedDiagramRoles(contract))
				if requiredDiagramRoles == "" {
					requiredDiagramRoles = "default,config,runtime,override"
				}
				repair := completionFileRepair(
					repairKind,
					repairCode,
					hint,
					repairFiles,
					map[string]string{
						"result_kind":            "absence",
						"context_role":           string(types.EvidenceContextRoleRelatedContext),
						"required_diagram_roles": requiredDiagramRoles,
						"repair_origin":          repairOrigin,
					},
				)
				queueCompletionRepairs(ctx, repairKind, repairFiles, rationale, repairOrigin)
				return types.ToolResult{
					ToolName:  t.Name(),
					Summary:   summary,
					Repair:    repair,
					Success:   false,
					Timestamp: time.Now(),
				}, nil
			}
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
				Kind:       types.ViolPreCompleteDowngrade,
				Detail:     "pre-complete simulator rejected emit_investigation_complete",
				ClusterKey: types.ComposeClusterKey("root", "CitationReq", "stage", "pre_complete"),
				Stage:      string(types.StageExplore),
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

	reason = normalizeLogSourceDriftCompletionReason(ctx, reason)
	ctx.Mutable.SetInvestigationComplete(reason)
	ctx.Mutable.SetInvestigationResultKind(resultKind)
	summary := fmt.Sprintf("Investigation marked complete (confidence=%s, result_kind=%s): %s", conf, resultKind, reason)
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
	refreshClosureReadSnapshot(ctx, closure)
	// Drain PendingReads the LLM has already satisfied via its own
	// read_file calls during this dispatch. Without this, an enforcer-
	// enqueued PendingRead (primary_anchor_unread, phase1_unread, chain
	// promotion, grounder reject) re-renders in every subsequent retry
	// hint until the next window boundary, falsely signalling "still
	// unread" and burning retries on a loop the LLM cannot exit.
	if drained := closure.DrainSatisfiedPendingReads(); drained > 0 {
		logging.Debug("[CGEC] drained %d satisfied PendingRead(s) after ReadSet refresh", drained)
	}

	// Session 12 phase1-unread gate. When the LLM calls complete on a
	// breadth-intent question (mechanism / call_chain / conditional)
	// while high-ranked pre-scan files remain unread, push those files
	// into PendingReads so the downstream PendingReads branch surfaces
	// them with a "Forced Read List" downgrade. This closes the gap
	// where R5 ghost-anchor promotion chased red-herring files because
	// the real answer-bearing file never produced a ghost anchor (the
	// LLM simply never read it, so no chain could reference it).
	//
	// requiresCrossFileCoverage further refines: a project-orientation
	// question (intent=explain, complexity=simple, no PrimaryEntities,
	// not cross-component) reads as "tour the repo, not investigate
	// a specific subsystem". The breadth-intent gates skip those
	// because there is no code-level cross-file flow to balance —
	// README + manifest + entry-point already cover the answer space.
	if justification == "" {
		raisePrimaryAnchorPendingRead(ctx, closure)
		raisePhase1UnreadPendingReads(ctx, closure)
		applyMultiPathAnchorChecks(ctx, closure)
	}
	if downgrade := explanationAnchorBackboneDowngrade(ctx); downgrade != "" {
		return downgrade
	}

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
				b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — evidence cites files the analyzer findings_validator flagged as unverified.\n\n")
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
		b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — pending forced reads block the closure.\n\n")
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
	if ctx.Mutable != nil {
		if bundle := ctx.Mutable.LogTriage(); bundle != nil && bundle.IsExternalSource() {
			// External-source logs (resolved_files=0) are answered from
			// the structured log semantics, not from repo file:line
			// anchors. Builder/front-end already teach downstream stages
			// to use summary prose and citation_ref=-1 where appropriate;
			// forcing a repo citation floor here only creates pointless
			// read-more loops against unrelated files.
			return ""
		}
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
		// Line-level check (post-2026-05-01 multi-path surgical-read
		// rewrite): use HasReadLine instead of HasRead so the gate
		// stays aligned with the finalizer-side grounder. Pre-fix:
		// HasRead returned true on any partial read, so a citation
		// pointing at a line OUTSIDE the surgical-read range was
		// counted as eligible by the gate but rejected by the
		// grounder downstream — the answer would ship with fewer
		// real citations than the gate believed it had cleared,
		// silently degrading shape compliance. HasReadLine has a
		// load-bearing backward-compat clause: when the file is in
		// readSet but no range records exist (legacy emitters that
		// never populated readRanges), it returns true so this
		// tightening does NOT regress callers that don't track
		// ranges.
		if len(readSet) > 0 && !closure.HasReadLine(e.Source, e.LineStart) {
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
		// kind of literal the answer should be) and the compiled
		// AnswerSemanticView's family classification. When the
		// subject points at a code symbol (function / type /
		// interface / handler route / return value) but the family
		// resolved to QFConfigPrecedence (config-key scalar
		// contract), the family routing under-fits the user's
		// subject. Emit a repair so the retry hint surfaces the
		// conflict; does NOT downgrade the current emit because
		// evidence IS present.
		if mismatch, fromFamily, toFamily := detectSubjectViewMismatch(ir); mismatch {
			closure.AddRepair(types.RepairDirective{
				Kind:      types.RepairSwapView,
				Subject:   fmt.Sprintf("from=%s,to=%s", fromFamily, toFamily),
				Rationale: fmt.Sprintf("answer subject=%s (source-code literal) but family=%s — the rendered answer should be %s instead", ir.RequestModel.AnswerSubject.Kind, fromFamily, toFamily),
				Origin:    "pre_complete.subject_view_mismatch",
			})
			logging.Info("[CGEC] B2b view_swap: origin=pre_complete.subject_view_mismatch from=%s to=%s", fromFamily, toFamily)
			// View mismatch detected at pre-complete boundary.
			// High-confidence (0.85) signal that the IR's family
			// classification disagrees with the subject.kind.
			closure.AppendViolation(types.Violation{
				Kind:       types.ViolViewSwap,
				Detail:     fmt.Sprintf("pre-complete B2b: AnswerSubject=%s vs family=%s (→ %s)", ir.RequestModel.AnswerSubject.Kind, fromFamily, toFamily),
				ClusterKey: types.ComposeClusterKey("subject", string(ir.RequestModel.AnswerSubject.Kind), "from", string(fromFamily), "to", string(toFamily), "root", "answer_subject"),
				Stage:      string(types.StageExplore),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_subject",
					Reason:     fmt.Sprintf("subject.kind=%s incompatible with family=%s at pre-complete", ir.RequestModel.AnswerSubject.Kind, fromFamily),
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
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — pre-complete citation preflight failed.\n\n")
	fmt.Fprintf(&b, "The answer contract requires ≥%d citation(s) but the current evidence buffer has only %d cite-eligible item(s) (Source non-empty AND in the read-files list).\n",
		min, eligible)
	b.WriteString("Continue the investigation: emit more file:line evidence anchored in files you actually read, or read additional files first.")
	return b.String()
}

func explanationAnchorBackboneDowngrade(ctx *types.BusContext) string {
	if ctx == nil || ctx.AnalysisIR == nil || !types.IRAllowsAnchorSkeleton(ctx.AnalysisIR) {
		return ""
	}
	plan := types.BuildAnswerSurfacePlanForBusContext(ctx)
	if plan == nil {
		return ""
	}
	anchors := plan.ExplanationAnchorBackbone
	missing := plan.ExplanationAnchorMissingTopics
	if len(missing) == 0 {
		return ""
	}
	if ctx.Mutable != nil {
		keywords := make([]string, 0, len(ctx.AnalysisIR.RequestModel.SubTopics))
		for _, topic := range ctx.AnalysisIR.RequestModel.SubTopics {
			keywords = append(keywords, topic.Entities...)
		}
		ctx.Mutable.EvidenceClosure().AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Keywords:  keywords,
			Rationale: fmt.Sprintf("multi-topic explanation still lacks one grounded anchor per sub-topic (%d/%d covered); read the exact owner/definition line for each missing sub-topic before re-calling emit_investigation_complete", len(anchors), len(ctx.AnalysisIR.RequestModel.SubTopics)),
			Origin:    "pre_complete.explanation_anchor_skeleton",
		})
	}
	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — multi-topic explanation still lacks one grounded anchor per sub-topic.\n\n")
	total := len(anchors) + len(missing)
	if total == 0 {
		total = len(ctx.AnalysisIR.RequestModel.SubTopics)
	}
	fmt.Fprintf(&b, "Current grounded anchor coverage: %d / %d sub-topics.\n", len(anchors), total)
	b.WriteString("Missing sub-topics:\n")
	for _, topic := range missing {
		fmt.Fprintf(&b, "  - %s\n", topic)
	}
	b.WriteString("\nRead the exact definition/owner line for the missing sub-topic(s), emit grounded evidence from those lines, then re-call emit_investigation_complete.")
	return b.String()
}

func capabilitySurfacePlan(ctx *types.BusContext) *types.AnswerSurfacePlan {
	plan := types.BuildAnswerSurfacePlanForBusContext(ctx)
	if plan == nil || plan.CapabilitySurface == nil {
		return nil
	}
	return plan
}

func capabilitySurfaceUnreadAuthorityFiles(plan *types.AnswerSurfacePlan, closure *types.EvidenceClosure, repoRoot string) []string {
	if plan == nil || closure == nil || len(plan.CapabilityAuthorityFiles) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(plan.CapabilityAuthorityFiles))
	var unread []string
	for _, raw := range plan.CapabilityAuthorityFiles {
		canon := phase1UnreadCanonPath(raw, repoRoot)
		if canon == "" || seen[canon] || closure.HasRead(canon) {
			continue
		}
		seen[canon] = true
		unread = append(unread, canon)
	}
	return unread
}

func refreshClosureReadSnapshot(ctx *types.BusContext, closure *types.EvidenceClosure) {
	if ctx == nil || ctx.Mutable == nil || closure == nil {
		return
	}
	gc := ground.BuildContext(ctx)
	if gc == nil || len(gc.LineIndex) == 0 {
		// Even with no ground LineIndex we still want to refresh the
		// per-file ranges in case the dispatch buffer carries reads
		// whose banners weren't ingested by ground (e.g. malformed
		// path canonicalisation). The coverage refresh below is its
		// own canonical walker; nil-safe on empty history.
		ground.RefreshClosureCoverage(ctx, closure)
		return
	}
	readSet := closure.ReadSet()
	if len(readSet) == 0 {
		readSet = make(map[string]bool, len(gc.LineIndex))
	}
	changed := false
	for file := range gc.LineIndex {
		if file == "" || readSet[file] {
			continue
		}
		readSet[file] = true
		changed = true
	}
	if changed {
		closure.SetReadSet(readSet)
	}
	// Sync per-file ranges + totals from the same live history. Every
	// downstream consumer of MergedReadLines / CoverageRatio /
	// HasFullyRead — including the multi-path symbol-anchored gate
	// (applyMultiPathAnchorChecks → multipath.EvaluateAnchor's
	// HasFullyRead bypass) and any other per-file coverage check —
	// would otherwise read a snapshot frozen at the END of the
	// previous dispatch's ParseOutput, so a mid-dispatch read_file
	// is invisible to the next emit_investigation_complete attempt.
	// Historical failure mode this fix targets: pre-2026-05-01 the
	// "covered 645 / 4368 lines (15%)" hint repeated across four
	// iterations while the LLM had read past the suggested range
	// three times.
	ground.RefreshClosureCoverage(ctx, closure)
}

func raisePrimaryAnchorPendingRead(ctx *types.BusContext, closure *types.EvidenceClosure) {
	if ctx == nil || ctx.Mutable == nil || closure == nil || ctx.AnalysisIR == nil {
		return
	}
	if capabilitySurfacePlan(ctx) != nil {
		return
	}
	// Orientation skip — a "what does this project do?" question
	// should not be blocked on "primary anchor unread"; the very
	// concept of a primary anchor implies a specific code subject
	// the user pinned to, which orientation questions explicitly
	// lack (Predicates clean + len(PrimaryEntities)==0). Mirrors
	// the same predicate applyMultiPathAnchorChecks uses.
	if types.IsProjectOrientationQuestion(ctx.AnalysisIR.RequestModel) {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(ctx.AnalysisIR.RequestModel.AnalyzerHints.Kind), "mechanism") {
		return
	}
	for _, ranked := range ctx.Mutable.Phase1Ranking() {
		if ranked.ExactEntityRank <= 0 {
			continue
		}
		canon := strings.TrimPrefix(strings.TrimSpace(ranked.Path), "./")
		if canon == "" {
			continue
		}
		if closure.HasRead(canon) {
			continue
		}
		closure.AddPendingRead(types.PendingRead{
			File:      canon,
			Rationale: "Exact entity anchor remains unread — mechanism answers must read the anchor implementation file before completion",
			Origin:    "pre_complete.primary_anchor",
		})
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Files:     []string{canon},
			Rationale: "Read the exact-entity anchor file before re-calling emit_investigation_complete",
			Origin:    "pre_complete.primary_anchor",
		})
		logging.Info("[CGEC] primary_anchor_unread: queued forced-read file=%s", canon)
		return
	}
}

// applyMultiPathAnchorChecks is the symbol-anchored verification the
// 2026-05-01 rewrite installed in place of raiseMultiPathCoverageParity.
// For each primary-anchor file (Phase1Ranking entry with
// ExactEntityRank > 0) it runs internal/tool/multipath.EvaluateAnchor
// and translates the resulting Decision into closure mutations:
//
//   - ActionSkip — no-op.
//   - ActionDemandSurgicalRead — AddPendingRead + AddRepair, both
//     carrying the engine's surgical Rationale that names the
//     uncovered symbol slivers and includes a copy-pasteable
//     read_file invocation.
//   - ActionAdvisoryHint — AddRepair with kind RepairExpandSearch,
//     intentionally NO PendingRead so emit_investigation_complete is
//     not blocked. The LLM may complete if it judges the file
//     unrelated.
//
// Cross-gate skips mirror the predecessor:
//   - capabilitySurfacePlan present → skip (capability surface owns
//     its own anchor set).
//   - IsProjectOrientationQuestion(RequestModel) → skip ("what does
//     this project do?" has no cross-component depth obligation).
//   - requiresCrossFileCoverage(kind, anchorCount) false → skip.
//   - Fewer than 2 primary anchors → skip (single-subject questions
//     have no parity target).
func applyMultiPathAnchorChecks(ctx *types.BusContext, closure *types.EvidenceClosure) {
	if ctx == nil || ctx.Mutable == nil || closure == nil || ctx.AnalysisIR == nil {
		return
	}
	if capabilitySurfacePlan(ctx) != nil {
		return
	}
	rm := ctx.AnalysisIR.RequestModel
	if types.IsProjectOrientationQuestion(rm) {
		return
	}
	kind := types.NormalizeRequirementKind(rm.AnalyzerHints.Kind)
	ranked := ctx.Mutable.Phase1Ranking()
	if len(ranked) == 0 {
		return
	}

	// Collect canonical anchor file list (dedup by canonical path).
	// Use ground.CanonicalRepoRelative for multi-platform safety
	// (POSIX absolute → repo-relative, Windows backslash → POSIX
	// slash, macOS HFS+/APFS case tolerance) so the anchor key
	// matches the keywordAnchorMap key downstream — which is also
	// canonicalised against the same repoRoot. Path mismatch here
	// would silently cause every keyword anchor lookup to miss.
	repoRoot := ctx.RepoRoot
	anchors := make([]string, 0, len(ranked))
	seen := make(map[string]bool, len(ranked))
	for _, rf := range ranked {
		if rf.ExactEntityRank <= 0 {
			continue
		}
		canon := canonicaliseAnchorPath(rf.Path, repoRoot)
		if canon == "" || seen[canon] {
			continue
		}
		seen[canon] = true
		anchors = append(anchors, canon)
	}
	if !requiresCrossFileCoverage(kind, len(anchors)) {
		return
	}
	if len(anchors) < 2 {
		return
	}

	// Resolve config + symbol oracle + keyword anchors once per check.
	limits := CurrentAnalysisLimits()
	cfg := multipath.Config{
		MinGroundedPerAnchor: limits.MultiPathMinGroundedPerAnchor,
		SymbolContextLines:   limits.MultiPathSymbolContextLines,
		KeywordContextLines:  limits.MultiPathKeywordContextLines,
		SmallFileThreshold:   limits.MultiPathSmallFileThreshold,
		RepoRoot:             repoRoot,
	}
	oracle := repomapSymbolOracle(ctx)
	entities := unionPrimaryAndDerivedEntities(rm.AnalyzerHints)
	evidence := ctx.Mutable.EmittedEvidence()
	keywordAnchorMap := multipath.ExtractKeywordAnchorsCapped(
		multiPathToolHistory(ctx), entities, repoRoot,
		limits.MultiPathMaxKeywordAnchorsPerFile,
	)

	for _, file := range anchors {
		dec := multipath.EvaluateAnchor(file, closure, evidence, oracle, entities, keywordAnchorMap[file], cfg)
		switch dec.Action {
		case multipath.ActionSkip:
			logging.Debug("[CGEC] multi_path_anchor: file=%s signal=%s action=skip reason=%q",
				file, dec.Signal, dec.Reason)
		case multipath.ActionDemandSurgicalRead:
			// Surgical Demand from the engine flows into the
			// PendingRead's LineRanges. runForcedReads (the Lazy
			// Auto-Read recovery in cgec_enforcers.go) honours these
			// to issue surgical read_file calls instead of pulling
			// the whole file when the LLM stalls — so the demand
			// stays bounded both in the primary path (LLM reads the
			// rationale-named ranges) and the recovery path
			// (orchestrator forced-reads the same ranges).
			closure.AddPendingRead(types.PendingRead{
				File:       file,
				Rationale:  dec.Rationale,
				Origin:     "pre_complete.multi_path_anchor",
				LineRanges: append([]types.LineRange(nil), dec.Demand...),
			})
			logging.Info("[CGEC] multi_path_anchor: file=%s signal=%s action=demand_surgical_read symbols=%v missing_ranges=%d",
				file, dec.Signal, dec.Symbols, len(dec.Demand))
		case multipath.ActionAdvisoryHint:
			closure.AddRepair(types.RepairDirective{
				Kind:      types.RepairExpandSearch,
				Files:     []string{file},
				Rationale: dec.Rationale,
				Origin:    "pre_complete.multi_path_anchor",
			})
			logging.Info("[CGEC] multi_path_anchor: file=%s signal=%s action=advisory_hint (non-blocking)",
				file, dec.Signal)
		}
	}
}

// canonicaliseAnchorPath collapses an anchor path to repo-relative
// POSIX form via ground.CanonicalRepoRelative when repoRoot is set,
// degrading to a slash-form cleanup when not. Mirrors the canonicaliser
// inside multipath / inside ground.coverage so every gate-related
// path key converges on the same shape across Linux / Windows / macOS.
func canonicaliseAnchorPath(p, repoRoot string) string {
	if p == "" {
		return ""
	}
	p = strings.TrimSpace(p)
	if repoRoot != "" {
		return ground.CanonicalRepoRelative(p, repoRoot)
	}
	cleaned := strings.ReplaceAll(p, "\\", "/")
	return strings.TrimPrefix(cleaned, "./")
}

// repomapSymbolOracle wraps the per-Run repomap Graph (when one is
// attached) into the multipath.SymbolOracle interface. Returns nil
// when no graph is available — the engine then disables Signal 1 and
// drops to Signal 2 / 3 cleanly.
func repomapSymbolOracle(ctx *types.BusContext) multipath.SymbolOracle {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	g, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if !ok || g == nil {
		return nil
	}
	return repomapOracleAdapter{graph: g}
}

type repomapOracleAdapter struct {
	graph *repotypes.Graph
}

func (a repomapOracleAdapter) SymbolsInFile(file string) []multipath.SymbolDef {
	if a.graph == nil {
		return nil
	}
	syms := a.graph.SymbolsInFile(file)
	if len(syms) == 0 {
		return nil
	}
	out := make([]multipath.SymbolDef, 0, len(syms))
	for _, s := range syms {
		out = append(out, multipath.SymbolDef{
			Name:    s.Name,
			Line:    s.Line,
			EndLine: s.EndLine,
		})
	}
	return out
}

// multiPathToolHistory aggregates the same three-source tool history
// ground.BuildContext consumes (TurnA artefacts → ctx.ToolResults →
// in-dispatch buffer). multipath.ExtractKeywordAnchors walks this
// to find grep matches the LLM produced earlier in the Run; passing
// the same composite history that everything else reads keeps the
// gate consistent with what the LLM was actually shown.
//
// nil-tolerant on every input branch.
func multiPathToolHistory(ctx *types.BusContext) []types.ToolResult {
	if ctx == nil {
		return nil
	}
	var history []types.ToolResult
	if ctx.Mutable != nil {
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil && len(ta.ToolResults) > 0 {
			history = append(history, ta.ToolResults...)
		}
	}
	if len(ctx.ToolResults) > 0 {
		history = append(history, ctx.ToolResults...)
	}
	if ctx.Mutable != nil {
		if disp := ctx.Mutable.DispatchToolResults(); len(disp) > 0 {
			history = append(history, disp...)
		}
	}
	return history
}

// unionPrimaryAndDerivedEntities merges the analyzer's PrimaryEntities
// + Entities + MentionedEntities + DerivedEntities into a single
// dedup'd lower-cased slice. The engine matches case-insensitively
// against this set when picking question-related symbols, so passing
// the union (rather than just PrimaryEntities) lets sub-topic
// derivations and log-augmented entities also count.
func unionPrimaryAndDerivedEntities(h types.AnalyzerHints) []string {
	seen := make(map[string]bool)
	out := make([]string, 0)
	push := func(list []string) {
		for _, e := range list {
			key := strings.ToLower(strings.TrimSpace(e))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, e)
		}
	}
	push(h.PrimaryEntities)
	push(h.MentionedEntities)
	push(h.Entities)
	push(h.DerivedEntities)
	return out
}

// nextReadOffset suggests the file offset (1-based line number) the
// LLM should pass to read_file to continue WITHOUT re-reading
// already-covered ranges. Strategy: take the largest End of any
// merged read range, return End+1. When the file has no recorded
// reads (fresh anchor) returns 1. The offset is a hint, not a
// constraint — the LLM may pass a different offset to reach a
// specific section. Helps the LLM avoid the "I already read 1-100
// so let me re-read 1-100" loop the production trace exposed.
func nextReadOffset(ranges []types.LineRange) int {
	maxEnd := 0
	for _, r := range ranges {
		if r.End > maxEnd {
			maxEnd = r.End
		}
	}
	if maxEnd <= 0 {
		return 1
	}
	return maxEnd + 1
}

// raisePhase1UnreadPendingReads is the session-12 CGEC gate that
// catches the "LLM declares complete while ignoring high-ranked pre-
// scan files" failure mode. The R5 ghost-anchor promotion covers the
// opposite half — files the LLM's chains REFERENCE but didn't read —
// and says nothing about files the LLM simply skipped. This gate
// closes that gap by treating the keyword-search ranking itself as
// evidence of "these files are likely answer-bearing; verify they
// were read before complete".
//
// Triggering conditions (all must hold):
//   - analysisLimits.Phase1UnreadTopK > 0 (gate enabled)
//   - ctx.AnalysisIR is non-nil (classification available)
//   - the declared RequirementKind structurally benefits from
//     cross-file coverage. mechanism / call_chain / conditional always
//     do; config_mapping joins them when multiple primary anchors are
//     present (for example code default + runtime overlay)
//   - Phase1Ranking has at least Phase1UnreadTopK entries
//   - top-K minus ReadSet has at least Phase1UnreadMinUnread entries
//
// When a repo graph and a concrete ReadSet focus are available, the
// gate narrows the unread list to hard anchors only. Generic graph
// adjacency remains a navigation signal for the explorer, but is too
// broad to become a completion blocker on its own (same-package
// interface/type references otherwise pull sibling implementations into
// the Forced Read List).
//
// When it fires: append PendingReads for each unread top-K file (with
// origin="phase1_unread" so trace inspection can tell this gate from
// chain-promotion D2) and raise a RepairExpandSearch directive. The
// downstream PendingReads branch of preCompleteContractCheck then
// renders the downgrade message.
func raisePhase1UnreadPendingReads(ctx *types.BusContext, closure *types.EvidenceClosure) {
	if ctx == nil || ctx.Mutable == nil || closure == nil {
		return
	}
	if ctx.AnalysisIR == nil {
		return
	}
	// Orientation skip — orientation questions don't carry a
	// breadth-intent obligation to read the keyword-search top-K.
	// Same predicate as raisePrimaryAnchorPendingRead +
	// applyMultiPathAnchorChecks for cross-gate consistency.
	if types.IsProjectOrientationQuestion(ctx.AnalysisIR.RequestModel) {
		return
	}
	// T2.1: once-per-pipeline latch. The first firing queues the
	// unread top-K files + raises a RepairExpandSearch directive;
	// the LLM has now been told. A second firing would list the same
	// (or a subset of the same) files without surfacing any new
	// information the first fire did not, while paying the cost of
	// an extra explorer redispatch. Stall soft/hard detection still
	// catches the "LLM keeps declaring complete without progress"
	// case via fingerprint convergence.
	if closure.Phase1UnreadFired() {
		return
	}
	if plan := capabilitySurfacePlan(ctx); plan != nil {
		unread := capabilitySurfaceUnreadAuthorityFiles(plan, closure, ctx.RepoRoot)
		if len(unread) == 0 {
			return
		}
		for _, file := range unread {
			closure.AddPendingRead(types.PendingRead{
				File:      file,
				Rationale: "Capability-surface authority file remains unread — inspect the canonical stage binding / skill contract / tool exposure source before declaring completion",
				Origin:    "phase1_unread",
			})
			logging.Info("[CGEC] phase1_unread: queued capability-authority file=%s", file)
		}
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Files:     unread,
			Rationale: fmt.Sprintf("%d capability-surface authority file(s) remain unread; inspect them before re-calling emit_investigation_complete", len(unread)),
			Origin:    "pre_complete.phase1_unread",
		})
		closure.MarkPhase1UnreadFired()
		return
	}
	limits := CurrentAnalysisLimits()
	if limits.Phase1UnreadTopK <= 0 {
		return
	}
	kind := types.NormalizeRequirementKind(ctx.AnalysisIR.RequestModel.AnalyzerHints.Kind)
	ranking := ctx.Mutable.Phase1Ranking()
	if len(ranking) == 0 {
		return
	}
	if !requiresCrossFileCoverage(kind, countPrimaryAnchorFiles(ranking)) {
		return
	}
	topK := limits.Phase1UnreadTopK
	if topK > len(ranking) {
		topK = len(ranking)
	}
	minUnread := limits.Phase1UnreadMinUnread
	if minUnread < 1 {
		minUnread = 1
	}
	filter := newPhase1UnreadFilter(ctx, closure)
	type unreadRankedFile struct {
		rank int
		file types.Phase1RankedFile
	}
	var unread []unreadRankedFile
	for i := 0; i < topK; i++ {
		f := ranking[i]
		canon := phase1UnreadCanonPath(f.Path, ctx.RepoRoot)
		if canon == "" {
			continue
		}
		if closure.HasRead(canon) {
			continue
		}
		if filter.enabled && !filter.hasMandatoryReadSignal(f, canon) {
			logging.Debug("[CGEC] phase1_unread: skip non-mandatory ranked file=%s after read focus", canon)
			continue
		}
		f.Path = canon
		unread = append(unread, unreadRankedFile{rank: i + 1, file: f})
	}
	if len(unread) < minUnread {
		return
	}

	files := make([]string, 0, len(unread))
	for _, item := range unread {
		f := item.file
		rationale := fmt.Sprintf(
			"Top-%d pre-scan ranked file (score=%.1f) remains unread — breadth-intent question (kind=%s) needs cross-component evidence",
			item.rank, f.Score, kind,
		)
		closure.AddPendingRead(types.PendingRead{
			File:      f.Path,
			Rationale: rationale,
			Origin:    "phase1_unread",
		})
		files = append(files, f.Path)
		logging.Info("[CGEC] phase1_unread: queued forced-read file=%s score=%.1f kind=%s", f.Path, f.Score, kind)
	}
	closure.AddRepair(types.RepairDirective{
		Kind:      types.RepairExpandSearch,
		Files:     files,
		Rationale: fmt.Sprintf("%d of the top-%d keyword-search ranked files were not read before emit_investigation_complete — open them before re-declaring complete", len(unread), topK),
		Origin:    "pre_complete.phase1_unread",
	})
	// T2.1: set the latch after the gate has produced its output, so
	// a nil/zero-queue early return above does NOT burn the latch.
	closure.MarkPhase1UnreadFired()
}

type phase1UnreadFilter struct {
	enabled   bool
	repoRoot  string
	graph     *repotypes.Graph
	readFiles []string
}

func newPhase1UnreadFilter(ctx *types.BusContext, closure *types.EvidenceClosure) phase1UnreadFilter {
	var out phase1UnreadFilter
	if ctx == nil || ctx.Mutable == nil || closure == nil {
		return out
	}
	out.repoRoot = ctx.RepoRoot
	if g, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph); ok && g != nil && len(g.FileIndex) > 0 {
		out.graph = g
	}
	for file := range closure.ReadSet() {
		if canon := phase1UnreadCanonPath(file, ctx.RepoRoot); canon != "" {
			out.readFiles = append(out.readFiles, canon)
		}
	}
	sort.Strings(out.readFiles)
	if out.graph == nil || len(out.readFiles) == 0 {
		return out
	}
	for _, file := range out.readFiles {
		if _, ok := out.graph.FileIndex[file]; ok {
			out.enabled = true
			break
		}
	}
	return out
}

func (f phase1UnreadFilter) hasMandatoryReadSignal(ranked types.Phase1RankedFile, file string) bool {
	file = phase1UnreadCanonPath(file, f.repoRoot)
	if file == "" {
		return false
	}
	return ranked.ExactEntityRank > 0
}

func phase1UnreadCanonPath(path, repoRoot string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = ground.CanonicalRepoRelative(path, repoRoot)
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

// requiresCrossFileCoverage reports whether the question kind should
// pay the cost of structural cross-file coverage guards. Pure breadth
// intents always qualify. Config-mapping questions are conditional:
// only multi-anchor cases need parity/unread protection.
func requiresCrossFileCoverage(k types.RequirementKind, primaryAnchors int) bool {
	switch k {
	case types.ReqMechanism, types.ReqCallChain, types.ReqConditional:
		return true
	case types.ReqConfigMapping:
		return primaryAnchors >= 2
	}
	return false
}

func countPrimaryAnchorFiles(ranked []types.Phase1RankedFile) int {
	if len(ranked) == 0 {
		return 0
	}
	seen := make(map[string]bool, len(ranked))
	count := 0
	for _, rf := range ranked {
		if rf.ExactEntityRank <= 0 {
			continue
		}
		path := strings.TrimPrefix(strings.TrimSpace(rf.Path), "./")
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		count++
	}
	return count
}

// detectSubjectViewMismatch returns true when the AnswerSubject is a
// source-code literal kind (function_name, type_name, interface,
// handler_route, return_value) but the compiled AnswerSemanticView's
// principal facet is the config-precedence family — i.e. the
// contract expects a config-key scalar but the subject points at a
// code symbol. The repair signal tells downstream that the family
// classification under-fits the user's actual subject.
//
// Returns (true, currentFamily, suggestedFamily) so the caller can
// emit a structured Repair with provenance metadata. fromFamily is
// always QFConfigPrecedence; toFamily is QFRoleLookup which is the
// closest scalar family for a code-symbol subject.
func detectSubjectViewMismatch(ir *types.AnalysisIR) (bool, types.QuestionFamily, types.QuestionFamily) {
	if ir == nil {
		return false, "", ""
	}
	subj := ir.RequestModel.AnswerSubject.Kind
	switch subj {
	case types.SubjectFunctionName, types.SubjectTypeName,
		types.SubjectInterface, types.SubjectHandlerRoute,
		types.SubjectReturnValue:
	default:
		return false, "", ""
	}
	family := types.ResolveQuestionFamily(ir.RequestModel)
	if family != types.QFConfigPrecedence {
		return false, "", ""
	}
	return true, family, types.QFRoleLookup
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
	total           int
	tier1           int
	tier2Grounded   int // GroundingGrounded but GroundingTier != TierLineText
	recovered       int
	ungrounded      int
	ungroundedItems []types.EvidenceItem
	tier2Items      []types.EvidenceItem
	recoveredItems  []types.EvidenceItem
}

// tallyEvidence classifies each item in the evidence buffer and
// returns the populated tally. Single source of truth for how the
// pipeline counts grounding outcomes.
func tallyEvidence(
	evidence []types.EvidenceItem,
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	stableAbsent bool,
	requiredFiles []string,
	rm types.RequestModel,
) evidenceTally {
	var t evidenceTally
	for _, e := range evidence {
		if !types.EvidenceCountsTowardTier1FloorInContext(e, contract, scenario, stableAbsent, requiredFiles, rm) {
			continue
		}
		t.total++
		switch e.GroundingStatus {
		case types.GroundingGrounded:
			if e.GroundingTier == types.TierLineText {
				t.tier1++
			} else {
				t.tier2Grounded++
				t.tier2Items = append(t.tier2Items, e)
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

type Tier1RepairTarget struct {
	File  string
	Lines []int
}

func toolRepairTargetsForFiles(kind types.RepairKind, files []string) []types.ToolRepairTarget {
	if len(files) == 0 {
		return nil
	}
	action := strings.TrimSpace(string(kind))
	if action == "" {
		action = string(types.RepairReadFile)
	}
	seen := make(map[string]bool, len(files))
	out := make([]types.ToolRepairTarget, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, types.ToolRepairTarget{
			File:   file,
			Action: action,
		})
	}
	return out
}

func queueCompletionRepairs(ctx *types.BusContext, kind types.RepairKind, files []string, rationale, origin string) {
	if ctx == nil || ctx.Mutable == nil || len(files) == 0 {
		return
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return
	}
	files = append([]string(nil), files...)
	for i := range files {
		files[i] = strings.TrimSpace(strings.ReplaceAll(files[i], `\`, `/`))
	}
	closure.AddRepair(types.RepairDirective{
		Kind:      kind,
		Files:     files,
		Rationale: rationale,
		Origin:    origin,
	})
}

func completionFileRepair(kind types.RepairKind, code, hint string, files []string, metadata map[string]string) *types.ToolRepair {
	targets := toolRepairTargetsForFiles(kind, files)
	if code == "" && hint == "" && len(targets) == 0 && len(metadata) == 0 {
		return nil
	}
	return &types.ToolRepair{
		Code:     code,
		Hint:     hint,
		Targets:  targets,
		Metadata: metadata,
	}
}

func exactAbsenceContextRepairKind(ctx *types.BusContext, contract *types.ExactResolutionContract, scenario types.Scenario, items []types.EvidenceItem, requiredFiles []string) (types.RepairKind, []string) {
	files := dedupeCanonicalFiles(requiredFiles)
	if len(files) == 0 {
		return types.RepairReadFile, nil
	}
	if ctx == nil || ctx.Mutable == nil {
		return types.RepairReadFile, files
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return types.RepairReadFile, files
	}
	refreshClosureReadSnapshot(ctx, closure)
	var unread []string
	var materialize []string
	for _, file := range files {
		if !closure.HasRead(file) {
			unread = append(unread, file)
			continue
		}
		if evidenceNeedsMaterializationFromFile(contract, scenario, items, file, files) || closure.HasFullyRead(file) {
			materialize = append(materialize, file)
		}
	}
	if len(materialize) > 0 {
		// Prefer staying on already-read same-scope anchors before
		// widening to unread siblings. This keeps exact-absence
		// closure repairs focused on the current grounded branch and
		// avoids repo_map-ranked detours when the missing proof can
		// still be materialized from files already in read coverage.
		return types.RepairEmitEvidence, materialize
	}
	if len(unread) > 0 {
		return types.RepairReadFile, unread
	}
	return types.RepairReadFile, files
}

func evidenceNeedsMaterializationFromFile(contract *types.ExactResolutionContract, scenario types.Scenario, items []types.EvidenceItem, file string, requiredFiles []string) bool {
	file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
	if contract == nil || file == "" || len(items) == 0 {
		return false
	}
	for _, item := range items {
		source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if source != file {
			continue
		}
		switch item.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered:
		default:
			continue
		}
		if types.ExactResolutionRelatedContextProofAllowedInFiles(contract, scenario, true, item, requiredFiles) {
			return true
		}
		if types.ExactResolutionAnswerContextAnchorAllowedInFiles(contract, scenario, true, item, requiredFiles) {
			return true
		}
		if types.ExactResolutionContextSurfaceRelevant(contract, item) {
			return true
		}
	}
	return false
}

func dedupeCanonicalFiles(files []string) []string {
	if len(files) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(files))
	out := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, file)
	}
	return out
}

func BuildTier1RepairTargets(repoRoot string, items []types.EvidenceItem) []Tier1RepairTarget {
	if len(items) == 0 {
		return nil
	}
	byFile := make(map[string]map[int]bool)
	for _, it := range items {
		file := ground.CanonicalRepoRelative(it.Source, repoRoot)
		if file == "" {
			continue
		}
		if byFile[file] == nil {
			byFile[file] = make(map[int]bool)
		}
		if it.LineStart > 0 {
			byFile[file][it.LineStart] = true
		}
	}
	if len(byFile) == 0 {
		return nil
	}
	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)
	out := make([]Tier1RepairTarget, 0, len(files))
	for _, file := range files {
		target := Tier1RepairTarget{File: file}
		for line := range byFile[file] {
			target.Lines = append(target.Lines, line)
		}
		sort.Ints(target.Lines)
		out = append(out, target)
	}
	return out
}

func buildTier1RepairTargets(ctx *types.BusContext, items []types.EvidenceItem) []Tier1RepairTarget {
	if ctx == nil {
		return nil
	}
	return BuildTier1RepairTargets(ctx.RepoRoot, items)
}

func Tier1LineList(lines []int, max int) string {
	if len(lines) == 0 {
		return ""
	}
	if max <= 0 || max > len(lines) {
		max = len(lines)
	}
	var parts []string
	for _, line := range lines[:max] {
		parts = append(parts, fmt.Sprintf("%d", line))
	}
	if len(lines) > max {
		parts = append(parts, fmt.Sprintf("+%d more", len(lines)-max))
	}
	return strings.Join(parts, ", ")
}

func tier1LineList(lines []int, max int) string {
	return Tier1LineList(lines, max)
}

func queueTier1ReadRepairs(ctx *types.BusContext, targets []Tier1RepairTarget) {
	if ctx == nil || ctx.Mutable == nil || len(targets) == 0 {
		return
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return
	}
	for _, target := range targets {
		rationale := "Line-text grounding floor: read_file this source to convert non-line_text evidence into grounded evidence."
		if lines := tier1LineList(target.Lines, 4); lines != "" {
			rationale = fmt.Sprintf("Line-text grounding floor: read_file %s near lines %s to convert non-line_text evidence into grounded evidence.", target.File, lines)
		}
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairReadFile,
			Files:     []string{target.File},
			Rationale: rationale,
			Origin:    "emit_investigation_complete.tier1_floor",
		})
	}
}

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
	t := tallyEvidence(evidence, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
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
	b.WriteString("  (A) call read_file on the source near the cited line so the line-text grounding pass can validate.\n")
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
func tier1GateReject(ctx *types.BusContext, floor float64, resultKind, justification string) (string, bool) {
	if floor <= 0 {
		return "", true
	}
	evidence := ctx.Mutable.EmittedEvidence()
	if len(evidence) == 0 {
		return "", true
	}
	contract := exactResolutionContractForCompletion(ctx)
	scenario := types.ScenarioGeneric
	if ctx != nil && ctx.AnalysisIR != nil {
		scenario = ctx.AnalysisIR.RequestModel.Scenario
	}
	stableAbsent := resultKind == "absence" && strings.TrimSpace(justification) != ""
	requiredFiles := types.ExactResolutionRequiredContextFiles(contract, ctx.Mutable)
	rm := types.RequestModel{}
	if ctx != nil && ctx.AnalysisIR != nil {
		rm = ctx.AnalysisIR.RequestModel
	}
	t := tallyEvidence(evidence, contract, scenario, stableAbsent, requiredFiles, rm)
	if t.total == 0 {
		return "", true
	}
	ratio := float64(t.tier1) / float64(t.total)
	if ratio >= floor {
		return "", true
	}
	targets := buildTier1RepairTargets(ctx, append(append([]types.EvidenceItem(nil), t.tier2Items...), t.recoveredItems...))
	queueTier1ReadRepairs(ctx, targets)
	var b strings.Builder
	fmt.Fprintf(&b,
		"emit_investigation_complete rejected: line-text-grounded ratio %.0f%% (%d grounded-via-line_text / %d total) < floor %.0f%%.\n\n",
		ratio*100, t.tier1, t.total, floor*100)
	b.WriteString("Citation grounding at answer-render time is stricter than at evidence-emit time: if the cited sources were never read via read_file, the rendered answer will drop those citations and the pipeline will reject the result.\n\n")
	if len(targets) > 0 {
		b.WriteString("Repair: structured read_file repairs have been queued for the sources below. Read them, emit grounded evidence, then retry completion.\n")
	} else {
		b.WriteString("Repair: call read_file on the sources below so the line-text grounding pass can re-validate them.\n")
	}
	maxList := 6
	for i, target := range targets {
		if i >= maxList {
			fmt.Fprintf(&b, "  ... and %d more non-line_text sources\n", len(targets)-maxList)
			break
		}
		lines := tier1LineList(target.Lines, 4)
		if lines == "" {
			fmt.Fprintf(&b, "  [%d] %s — read_file %s and re-emit grounded evidence\n", i+1, target.File, target.File)
			continue
		}
		fmt.Fprintf(&b, "  [%d] %s — read_file near lines %s, then re-emit grounded evidence\n",
			i+1, target.File, lines)
	}
	if len(targets) == 0 {
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
	}
	return b.String(), false
}

// hasGroundedOrRecovered reports whether the evidence buffer contains
// at least one item whose grounder verdict is grounded or recovered.
// Drives the absence-vs-grounded contradiction gate in Execute.
func hasGroundedOrRecovered(items []types.EvidenceItem) bool {
	return tallyEvidence(items, nil, types.ScenarioGeneric, false, nil, types.RequestModel{}).hasAny()
}

func allowsContextualEvidenceForAbsence(ctx *types.BusContext, reason, justification string, evidence []types.EvidenceItem) bool {
	if groundedEvidenceIsContextOnly(evidence) {
		return true
	}
	contract := exactResolutionContractForCompletion(ctx)
	if contract == nil || !contract.AllowAbsence {
		return false
	}
	scenario := types.ScenarioGeneric
	if ctx != nil && ctx.AnalysisIR != nil {
		scenario = ctx.AnalysisIR.RequestModel.Scenario
	}
	var requiredFiles []string
	if ctx != nil && ctx.Mutable != nil {
		requiredFiles = types.ExactResolutionRequiredContextFiles(contract, ctx.Mutable)
	}
	text := reason + "\n" + justification
	targets := exactAbsencePendingTargets(ctx)
	if len(targets) == 0 {
		targets = append(targets, contract.Targets...)
	}
	if types.ExactResolutionAbsenceClosureReady(contract, scenario, targets, evidence, requiredFiles) {
		return true
	}
	mentioned := false
	for _, target := range targets {
		if !types.ExactResolutionTextMentionsTarget(contract, text, target) {
			continue
		}
		mentioned = true
		if evidenceHasAnyDefiningExactTargetProof(contract, evidence, []string{target}) {
			continue
		}
		return true
	}
	if mentioned {
		return false
	}
	// Once the upstream exact-target lane has already established a
	// pending "not found" target, absence closure may still be valid
	// even if the reason/justification paraphrases the search outcome
	// without repeating the literal on every line. In that state,
	// supporting evidence is allowed as long as no defining proof for
	// the exact target was emitted.
	return len(targets) > 0 && !evidenceHasAnyDefiningExactTargetProof(contract, evidence, targets)
}

func groundedEvidenceIsContextOnly(items []types.EvidenceItem) bool {
	sawGrounded := false
	for _, item := range items {
		switch item.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered, "":
		default:
			continue
		}
		sawGrounded = true
		switch item.ContextRole {
		case types.EvidenceContextRoleAbsenceSupport,
			types.EvidenceContextRoleRelatedContext,
			types.EvidenceContextRoleIllustrativeOnly:
			continue
		default:
			return false
		}
	}
	return sawGrounded
}

func exactAbsencePendingTargets(ctx *types.BusContext) []string {
	contract := exactResolutionContractForCompletion(ctx)
	if contract == nil {
		return nil
	}
	unverified := unverifiedFindingsForCompletion(ctx)
	return types.ExactResolutionPendingTargets(contract, unverified)
}

func completionPendingExactTargets(ctx *types.BusContext, contract *types.ExactResolutionContract, evidence []types.EvidenceItem) []string {
	if contract == nil {
		return nil
	}
	if pending := exactAbsencePendingTargets(ctx); len(pending) > 0 {
		return pending
	}
	if ctx != nil && ctx.Mutable != nil &&
		strings.EqualFold(ctx.Mutable.StableInvestigationResultKind(), "absence") &&
		strings.TrimSpace(ctx.Mutable.StableAbsenceJustification()) != "" {
		return nil
	}
	if evidenceHasAnyDefiningExactTargetProof(contract, evidence, contract.Targets) {
		return nil
	}
	return append([]string(nil), contract.Targets...)
}

func unverifiedFindingsForCompletion(ctx *types.BusContext) []types.UnverifiedFinding {
	var out []types.UnverifiedFinding
	if ctx == nil {
		return nil
	}
	if ctx.Mutable != nil {
		out = append(out, ctx.Mutable.EvidenceClosure().UnverifiedFindings()...)
	}
	out = append(out, unverifiedFindingsFromStageReports(ctx.StageReports)...)
	return dedupeUnverifiedFindings(out)
}

func unverifiedFindingsFromStageReports(reports []types.StageReport) []types.UnverifiedFinding {
	var out []types.UnverifiedFinding
	for _, report := range reports {
		text := report.Findings
		for {
			start := strings.Index(text, "~~")
			if start < 0 {
				break
			}
			rest := text[start+2:]
			end := strings.Index(rest, "~~")
			if end < 0 {
				break
			}
			token := strings.TrimSpace(strings.Trim(rest[:end], "`\"' "))
			after := rest[end+2:]
			if token != "" && stageReportAnnotatedMissFollows(after) {
				out = append(out, types.UnverifiedFinding{
					Token:  token,
					Kind:   inferStageReportFindingKind(token),
					Reason: "unverified in prior stage report",
				})
			}
			text = after
		}
	}
	return out
}

func stageReportAnnotatedMissFollows(text string) bool {
	if len(text) > 120 {
		text = text[:120]
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "unverified") ||
		strings.Contains(lower, "repo graph") ||
		strings.Contains(text, "未验证") ||
		strings.Contains(text, "未在 repo") ||
		strings.Contains(text, "鏈獙")
}

func inferStageReportFindingKind(token string) string {
	if strings.ContainsAny(token, `/\`) {
		return "path"
	}
	return "symbol"
}

func dedupeUnverifiedFindings(in []types.UnverifiedFinding) []types.UnverifiedFinding {
	seen := make(map[string]bool)
	var out []types.UnverifiedFinding
	for _, f := range in {
		token := strings.TrimSpace(f.Token)
		if token == "" {
			continue
		}
		kind := strings.TrimSpace(f.Kind)
		if kind == "" {
			kind = "symbol"
		}
		key := kind + "\x00" + token
		if seen[key] {
			continue
		}
		seen[key] = true
		f.Token = token
		f.Kind = kind
		out = append(out, f)
	}
	return out
}

func exactResolutionContractForCompletion(ctx *types.BusContext) *types.ExactResolutionContract {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	if c := answerExactResolutionContract(ctx); c != nil {
		return c
	}
	return types.BuildExactResolutionContract(ctx.AnalysisIR.RequestModel)
}

func normalizeExactAbsenceCompletion(ctx *types.BusContext, resultKind, reason, justification string) (string, string) {
	if ctx == nil || ctx.Mutable == nil {
		return resultKind, justification
	}
	contract := exactResolutionContractForCompletion(ctx)
	if contract == nil || !contract.AllowAbsence || len(contract.Targets) == 0 {
		return resultKind, justification
	}
	if justification != "" && resultKind == "absence" {
		return resultKind, justification
	}
	scenario := types.ScenarioGeneric
	if ctx.AnalysisIR != nil {
		scenario = ctx.AnalysisIR.RequestModel.Scenario
	}
	requiredFiles := types.ExactResolutionRequiredContextFiles(contract, ctx.Mutable)
	evidence := ctx.Mutable.EmittedEvidence()
	if resultKind == "resolved" &&
		justification == "" &&
		strings.EqualFold(ctx.Mutable.StableInvestigationResultKind(), "absence") &&
		strings.TrimSpace(ctx.Mutable.StableAbsenceJustification()) != "" &&
		!evidenceHasAnyDefiningExactTargetProof(contract, evidence, contract.Targets) {
		logging.Info("[emit_investigation_complete] preserving prior exact-absence closure over resolved retry without new defining proof (targets=%v)", contract.Targets)
		return "absence", ctx.Mutable.StableAbsenceJustification()
	}
	if !types.ExactResolutionAbsenceClosureReady(contract, scenario, contract.Targets, evidence, requiredFiles) {
		return resultKind, justification
	}
	auto := types.ExactResolutionAutoAbsenceJustification(contract)
	if auto == "" {
		return resultKind, justification
	}
	if resultKind == "absence" && justification == "" {
		logging.Info("[emit_investigation_complete] synthesized exact-absence justification from structured evidence (targets=%v)", contract.Targets)
		return "absence", auto
	}
	if resultKind == "resolved" && justification == "" {
		logging.Info("[emit_investigation_complete] normalized result_kind resolved -> absence from structured exact-absence evidence (targets=%v)", contract.Targets)
		return "absence", auto
	}
	return resultKind, justification
}

func evidenceHasAnyDefiningExactTargetProof(contract *types.ExactResolutionContract, items []types.EvidenceItem, targets []string) bool {
	if contract == nil || len(items) == 0 {
		return false
	}
	for _, it := range items {
		switch it.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered, "":
		default:
			continue
		}
		if it.ContextRole == types.EvidenceContextRoleIllustrativeOnly ||
			it.ContextRole == types.EvidenceContextRoleAbsenceSupport ||
			it.ContextRole == types.EvidenceContextRoleRelatedContext {
			continue
		}
		if strings.TrimSpace(string(it.AnchorKind)) == "" {
			continue
		}
		if !evidenceProofAnchorsAnyListedExactTarget(contract, it, targets) {
			continue
		}
		if types.IsNegativeEvidencePredicate(it.Predicate) {
			continue
		}
		if !types.ExactResolutionSourceIsDefiningPrimaryProofLike(contract, it.Source) {
			continue
		}
		return true
	}
	return false
}

func evidenceProofAnchorsAnyListedExactTarget(contract *types.ExactResolutionContract, item types.EvidenceItem, targets []string) bool {
	for _, target := range targets {
		if types.ExactResolutionTextMentionsTarget(contract, item.AnchorSymbol, target) ||
			types.ExactResolutionTextMentionsTarget(contract, item.Object, target) {
			return true
		}
		if strings.TrimSpace(item.AnchorSymbol) == "" &&
			types.ExactResolutionTextMentionsTarget(contract, item.Subject, target) {
			return true
		}
	}
	return false
}

func evidenceDirectlyAnchorsAnyListedExactTarget(contract *types.ExactResolutionContract, item types.EvidenceItem, targets []string) bool {
	for _, target := range targets {
		if types.ExactResolutionTextMentionsTarget(contract, item.AnchorSymbol, target) ||
			types.ExactResolutionTextMentionsTarget(contract, item.Object, target) {
			return true
		}
		if strings.TrimSpace(item.AnchorSymbol) == "" &&
			types.ExactResolutionTextMentionsTarget(contract, item.Subject, target) {
			return true
		}
	}
	return false
}

func evidenceHasGroundedRelatedContextProof(contract *types.ExactResolutionContract, scenario types.Scenario, items []types.EvidenceItem, requiredFiles []string) bool {
	if contract == nil || len(items) == 0 {
		return false
	}
	for _, item := range items {
		if types.ExactResolutionRelatedContextProofAllowedInFiles(contract, scenario, true, item, requiredFiles) {
			return true
		}
	}
	if scenario == types.ScenarioConfigTrace && contract.TargetKind == types.SubjectConfigKey {
		return false
	}
	for _, item := range items {
		if types.ExactResolutionEvidenceCanSatisfyRelatedContext(contract, item, requiredFiles) {
			return true
		}
	}
	return false
}

func looksLikeHonestZeroClaim(reason, justification string) bool {
	text := strings.ToLower(strings.TrimSpace(reason + "\n" + justification))
	if text == "" {
		return false
	}
	if containsAnySubstr(text,
		"enough evidence", "sufficient evidence", "collected enough evidence", "found enough evidence",
		"fully traced", "fully track", "already found enough", "all necessary evidence",
		"充分的证据", "找到充分", "已经找到", "已收集", "完全追踪", "完整追踪", "证据均已") {
		return false
	}
	return containsAnySubstr(text,
		"no such", "does not exist", "doesn't exist", "not found", "nothing found",
		"found no", "not present",
		"none", "zero", "zero hit", "zero match", "0 hit", "0 match", "0 file", "0 files",
		"no handler", "no symbol", "no file", "no key", "no config key", "no exact", "absent",
		"不存在", "没有", "未找到", "找不到", "无此", "无该", "零", "0 个", "0个", "空结果")
}

func containsAnySubstr(text string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// G1 (Plan E rollout 2026-05-02) was implemented here as
// computePreCompleteCoverageGap and reverted the same day after the
// s1a regression — see the comment block above the resultKind check
// for the architectural lesson. Function deleted to keep the file
// honest about what's enforced and what's not.
