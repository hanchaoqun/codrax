package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// emitChangePlanSchemaReminder is the canonical "here's what the LLM
// must send" string injected into every payload-rejection ToolResult.
// Goal: when streaming truncation or a payload-format hallucination
// causes the LLM to retry with empty/garbage params (Run #3 failure
// mode — 5 consecutive empty emit_change_plan calls), the rejection
// itself re-primes the LLM with the full schema so the next retry has
// the structural information it needs to recover. Plain text, no JSON
// formatting, kept short enough to fit in a model's working memory.
const emitChangePlanSchemaReminder = "REQUIRED schema: {request: string (1-3 sentences restating the user's ask), " +
	"summary: string (3-10 sentences explaining what + why), " +
	"changes: array of {path: string, kind: \"create\"|\"modify\"|\"delete\"|\"patch\", " +
	"new_content: string (full file body for create/modify), patch: string (unified diff for kind=patch), edits: optional structured line edits for kind=patch, " +
	"rationale: string (1-3 sentences), depends_on: optional []string of OTHER paths in this plan}}. " +
	"OPTIONAL: acceptance_tests: array of strings; verification_probes: array of typed bounded probes with optional contract_refs/changed_symbol_refs. " +
	"Controller-authorized proof-follow-up batches may emit changes: [] only with verification_probes[] to record no source edits required. " +
	"Do NOT call the tool with empty/null parameters — emit the FULL JSON body as a single function-call argument."

// emitMinPayloadBytes is the threshold below which the params blob is
// considered empty-or-truncated. A real ChangePlan with one change
// has request + summary + at least one path/kind/rationale + braces
// — never under ~150 bytes. We pick 80 as a conservative floor that
// catches all observed degenerate cases (empty `{}`, `null`, single-
// field stubs) without false-positiving on a tiny but legitimate
// "fix typo" plan.
const emitMinPayloadBytes = 80

// EmitChangePlan is the planner agent's structured exit channel.
// The LLM emits a ChangePlan describing the code modification it
// proposes; Execute validates the payload shape, stamps a plan ID
// + timestamp, and installs it on BusContext.Mutable so
// the plan stage hook (orchestrator) and cmd/root.go (single-shot disk
// writer) can read it after the agent returns.
//
// Classified ReadOnly + NonEvidenceTool following the emit_* family
// convention: the tool mutates BusContext, not the filesystem (the
// disk-write happens later in cmd/root.go with an explicit
// --plan-out target so the tool's own Execute is pure w.r.t. the
// repo).
//
// Red-line discipline (L3): this Execute MUST NOT invoke
// ground.BuildContext or ground.GroundItem. The plan content is a
// structured proposal, not a citation into repo source — running
// the grounder on it would misinterpret the ChangeUnit array as
// evidence and drop the entire payload. Enforced by
// internal/tool/write_mode_red_lines_test.go via go/ast scan.
type EmitChangePlan struct {
	ReadOnly
	NonEvidenceTool
}

// emitChangePlanParams is the wire-level shape the LLM emits. Keep
// in sync with the JSON schema below; Execute uses DisallowUnknownFields
// so any drift fails loudly rather than silently losing fields.
type emitChangePlanParams struct {
	Request            string                    `json:"request"`
	Summary            string                    `json:"summary"`
	Changes            []emitChangePlanChange    `json:"changes"`
	AcceptanceTests    []string                  `json:"acceptance_tests,omitempty"`
	VerificationProbes []types.VerificationProbe `json:"verification_probes,omitempty"`
}

// emitChangePlanChange mirrors types.FileChange but lives in the tool
// package so the wire-format is independent of the internal struct
// (which may grow new fields post-B0 without breaking old plans).
type emitChangePlanChange struct {
	Path               string                    `json:"path"`
	Kind               string                    `json:"kind"`
	NewContent         string                    `json:"new_content,omitempty"`
	Patch              string                    `json:"patch,omitempty"`
	Edits              []types.StructuredEdit    `json:"edits,omitempty"`
	NewPath            string                    `json:"new_path,omitempty"`
	Rationale          string                    `json:"rationale"`
	DependsOn          []string                  `json:"depends_on,omitempty"`
	VerificationProbes []types.VerificationProbe `json:"verification_probes,omitempty"`
}

func changeLocalVerificationProbes(changes []emitChangePlanChange) []types.VerificationProbe {
	if len(changes) == 0 {
		return nil
	}
	var out []types.VerificationProbe
	for _, change := range changes {
		if len(change.VerificationProbes) == 0 {
			continue
		}
		out = append(out, change.VerificationProbes...)
	}
	return out
}

// Name returns the tool's stable identifier. Used by the planner
// agent's ShouldStop check and by write_mode_red_lines_test.go.
func (t *EmitChangePlan) Name() string { return "emit_change_plan" }

// Description is one sentence: what + one-call constraint. Strategy
// guidance (how to pick target paths, when to propose a new file vs
// modify an existing one, how to phrase rationale) lives in
// change-plan-skill's prompt.
func (t *EmitChangePlan) Description() string {
	return "Emit the structured ChangePlan for the user's requested code change. " +
		"Call EXACTLY once per plan-stage dispatch — further calls overwrite the " +
		"prior emission. The plan is a proposal only; apply-stage consumes it."
}

// Parameters returns a strict JSON schema for the ChangePlan emission.
// Every object layer has additionalProperties:false so the LLM cannot
// invent fields (kind enum locks to create|modify|delete|patch).
func (t *EmitChangePlan) Parameters() json.RawMessage {
	return injectVerificationProbeLanguageSchema(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "request": {
      "type": "string",
      "description": "The user's natural-language request, restated for the plan's record. 1-3 sentences."
    },
    "summary": {
      "type": "string",
      "description": "Prose explanation of what the plan does and why, 3-10 sentences. Shown to the user in approval UI."
    },
    "changes": {
      "type": "array",
      "description": "Ordered list of file-level modifications. Apply-stage processes them sequentially. Empty [] is accepted only for controller-authorized proof-follow-up / no-change sentinel plans with typed verification_probes or typed passing planner probes.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "path": {
            "type": "string",
            "description": "Repo-relative path of the file to modify. Must be a regular file (not a directory)."
          },
          "kind": {
            "type": "string",
            "enum": ["create", "modify", "delete", "patch"],
            "description": "create = new file (new_content required). modify = overwrite existing (new_content required). delete = remove (no content fields). patch = unified-diff (patch field required; B2 feature)."
          },
          "new_content": {
            "type": "string",
            "description": "Full file body for kind=create|modify. Empty for delete. Ignored for patch."
          },
          "patch": {
            "type": "string",
            "description": "Unified-diff payload for kind=patch. Use either patch OR edits, not both."
          },
          "edits": {
            "type": "array",
            "description": "Optional structured line edits for kind=patch. The tool compiles these into a unified diff against current file bytes. Use either edits OR patch, not both.",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "kind": {"type": "string", "enum": ["replace", "delete", "insert_before", "insert_after", "insert_at_eof", "insert_before_final_brace"]},
                "start_line": {"type": "integer", "minimum": 1, "description": "1-based. Required for replace/delete and for line-addressed insert_before/insert_after. Ignored for insert_at_eof and insert_before_final_brace."},
                "end_line": {"type": "integer", "minimum": 1, "description": "1-based inclusive last line for replace/delete. Omit for a single-line edit — it defaults to start_line. Ignored for insert kinds."},
                "content": {"type": "string", "description": "Replacement or insertion bytes. Required for replace/insert; omit for delete. insert_before_final_brace is only for brace-language files with a final standalone closing brace; do not use it for Python. For Python, use line-anchored insert_before/insert_after for indentation-sensitive additions, or full modify when the edit spans an indented block. insert_at_eof is safe only for top-level unindented Python additions and the specific EOF class-member case accepted by the tool."},
                "old_text": {"type": "string", "description": "Optional exact CURRENT bytes of the target range or insertion anchor line. Must match the file as it is now (re-read after any earlier edit); a missing or extra final newline is tolerated. On mismatch the error echoes the current bytes so you can correct without guessing."}
              },
              "required": ["kind"]
            }
          },
          "rationale": {
            "type": "string",
            "description": "1-3 sentence explanation for WHY this specific file needs this specific change."
          },
          "depends_on": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Repo-relative paths of OTHER changes in THIS plan that must apply before this one. Empty or omitted = no explicit ordering (declaration order wins). Use when a create or modify here relies on the output of another change (e.g. modify Y imports new-created X → Y depends_on [X]). Every entry MUST appear as another changes[].path; cycles are rejected."
          },
          "verification_probes": {
            "type": "array",
            "description": "Optional change-local bounded probes that exercise this file's behavior. The tool merges these into the plan-level verification_probes lane before validation so probes stay attached to typed plan state, not prose.",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "id": {"type": "string", "description": "Stable short probe identifier, e.g. version_info_boundary."},
                "language": {"type": "string", "enum": __VERIFICATION_PROBE_LANGUAGE_ENUM__, "description": "Probe runtime. Supported inline runtimes: __VERIFICATION_PROBE_LANGUAGE_DESCRIPTION__."},
                "working_dir": {"type": "string", "description": "Repo-relative working directory. Empty or . means repo root."},
                "code": {"type": "string", "description": "Inline probe code. It should import/use the changed code and exit non-zero on failure."},
                "timeout_seconds": {"type": "integer", "minimum": 1, "maximum": 30, "description": "Optional timeout. Defaults to 10 seconds; capped at 30."},
                "expected_stdout": {
                  "type": "array",
                  "items": {"type": "string"},
                  "description": "Optional exact substrings that must appear in stdout for the probe to pass."
                },
                "contract_refs": {
                  "type": "array",
                  "items": {"type": "string"},
                  "description": "Optional behavior_contract ids from the task framing that this probe directly verifies."
                },
                "placement_refs": {
                  "type": "array",
                  "items": {"type": "string"},
                  "description": "Optional behavior_contract ids whose rendered-text placement relation this probe directly verifies. Use only when the referenced contract has placement{} and the probe checks line-local anchor/expected relation, not only global substring presence."
                },
                "changed_symbol_refs": {
                  "type": "array",
                  "items": {"type": "string"},
                  "description": "Optional changed symbols/modules this probe imports or executes."
                },
                "expects_baseline_failure": {
                  "type": "boolean",
                  "description": "True when this probe is meant to fail before the patch and pass after it."
                }
              },
              "required": ["language", "code"]
            }
          }
        },
        "required": ["path", "kind", "rationale"]
      }
    },
    "acceptance_tests": {
      "type": "array",
      "description": "Optional list of test assertions the verify stage must cover. Natural-language in B0; formalized to Criterion IR in B1.",
      "items": {"type": "string"}
    },
    "verification_probes": {
      "type": "array",
      "description": "Optional typed fallback checks for verify environments where the project runner is unavailable or unparseable. Each probe must be deterministic, bounded, and exit non-zero on failure. Supported inline runtimes: __VERIFICATION_PROBE_LANGUAGE_DESCRIPTION__.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "id": {"type": "string", "description": "Stable short probe identifier, e.g. version_info_boundary."},
          "language": {"type": "string", "enum": __VERIFICATION_PROBE_LANGUAGE_ENUM__, "description": "Probe runtime. Supported inline runtimes: __VERIFICATION_PROBE_LANGUAGE_DESCRIPTION__."},
          "working_dir": {"type": "string", "description": "Repo-relative working directory. Empty or . means repo root."},
          "code": {"type": "string", "description": "Inline probe code. It should import/use the changed code and exit non-zero on failure."},
          "timeout_seconds": {"type": "integer", "minimum": 1, "maximum": 30, "description": "Optional timeout. Defaults to 10 seconds; capped at 30."},
          "expected_stdout": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Optional exact substrings that must appear in stdout for the probe to pass."
          },
          "contract_refs": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Optional behavior_contract ids from the task framing that this probe directly verifies."
          },
          "placement_refs": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Optional behavior_contract ids whose rendered-text placement relation this probe directly verifies. Use only when the referenced contract has placement{} and the probe checks line-local anchor/expected relation, not only global substring presence."
          },
          "changed_symbol_refs": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Optional changed symbols/modules this probe imports or executes."
          },
          "expects_baseline_failure": {
            "type": "boolean",
            "description": "True when this probe is meant to fail before the patch and pass after it."
          }
        },
        "required": ["language", "code"]
      }
    }
  },
  "required": ["request", "summary", "changes"]
}`)
}

// Execute validates the emitted payload, stamps plan metadata, and
// installs the resulting ChangePlan on BusContext.Mutable. The
// agent's ParseOutput reads Mutable.ChangePlan() post-Execute to
// verify the emission happened and return success to orchestrator.
//
// No filesystem side effects here by design — the disk write
// happens in cmd/root.go's runSingleShot with the --plan-out path
// so the tool stays pure w.r.t. the repo (consistent with emit_*
// family convention).
func (t *EmitChangePlan) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_change_plan requires a writable context",
			Timestamp: time.Now(),
		}, nil
	}

	// Layer-3 structured rejection: empty / truncated payload guard.
	// When the LLM hits streaming truncation in the middle of a large
	// emit_change_plan call (~5 KB JSON payloads are common for plans
	// with non-trivial new_content), the gateway sometimes returns the
	// function call with an empty or single-byte params blob. Without
	// this guard the json.Decoder error is "unexpected EOF" — a useless
	// hint that drove Run #3's 5-consecutive-empty-payload recovery
	// failure. Every payload-shape rejection re-primes the LLM with
	// emitChangePlanSchemaReminder so the next retry has the full
	// structural information needed to rebuild the JSON.
	trimmed := strings.TrimSpace(string(params))
	if len(trimmed) < emitMinPayloadBytes || trimmed == "{}" || trimmed == "null" || trimmed == "" {
		summary := "emit_change_plan rejected: payload was empty or truncated (got " + fmt.Sprintf("%d", len(trimmed)) + " bytes). " + emitChangePlanSchemaReminder
		return rejectPlanToolResult(t.Name(), summary, planRepairPackFromReason(t.Name(), "payload_empty_or_truncated", summary, []string{"$"}, nil)), nil
	}
	params = applyStructuredPayloadCompatWithSelectedStringFieldRepair(
		t.Name(),
		params,
		t.Parameters(),
		[]string{"changes", "acceptance_tests", "verification_probes"},
	)

	// Strict decode — unknown fields fail loudly so a schema drift
	// surfaces during development, not as silent data loss. On decode
	// failure the schema reminder is appended so the LLM can recover
	// without re-deriving the field set from prose.
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p emitChangePlanParams
	if err := dec.Decode(&p); err != nil {
		res, retErr := failStrictDecodeWithErrorMessage(t.Name(), time.Now(), err, nil, params, "emit_change_plan rejected: ", ". "+emitChangePlanSchemaReminder)
		pack := planRepairPackFromToolRepair(t.Name(), "strict_decode_failed", res.Summary, res.Repair)
		return attachPlanRepairPack(res, pack), retErr
	}
	p.VerificationProbes = append(p.VerificationProbes, changeLocalVerificationProbes(p.Changes)...)

	if strings.TrimSpace(p.Summary) == "" {
		summary := "emit_change_plan rejected: summary is required and must be non-empty. " + emitChangePlanSchemaReminder
		return rejectPlanToolResult(t.Name(), summary, planRepairPackFromReason(t.Name(), "summary_required", summary, []string{"$.summary"}, nil)), nil
	}
	probes, rej := normalizeVerificationProbes(p.VerificationProbes)
	if rej != "" {
		return rejectPlanToolResult(t.Name(), "emit_change_plan rejected: "+rej,
			planRepairPackWithEnums(t.Name(), "verification_probe_invalid", rej, []string{"$.verification_probes", "$.changes[].verification_probes"}, supportedVerificationProbeLanguageSet())), nil
	}
	if len(p.Changes) == 0 {
		if plan := proofFollowupProbeOnlyPlanSentinel(ctx, p, probes); plan != nil {
			attachWriteBehaviorContracts(ctx, plan)
			enrichVerificationProbeRefs(plan)
			if rej, pack := validatePlanFullContentWithRepair(ctx, t.Name(), p.Summary, nil, plan.VerificationProbes); rej != "" {
				return rejectPlanToolResult(t.Name(), "emit_change_plan rejected: "+rej, pack), nil
			}
			ctx.Mutable.SetChangePlan(plan)
			summary := fmt.Sprintf(
				"[emit_change_plan: id=%s changes=0 status=%s verification_probes=%d]\n"+
					"emit_change_plan recorded a proof-follow-up probe-only plan",
				plan.ID, plan.Status, len(plan.VerificationProbes))
			logging.Info("[emit_change_plan] plan=%s changes=0 status=%s probes=%d proof_followup=true",
				plan.ID, plan.Status, len(plan.VerificationProbes))
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   true,
				Summary:   summary,
				Timestamp: time.Now(),
			}, nil
		}
		if plan := noChangeRequiredReplanSentinel(ctx, p); plan != nil {
			ctx.Mutable.SetChangePlan(plan)
			summary := fmt.Sprintf(
				"[emit_change_plan: id=%s changes=0 status=%s]\n"+
					"emit_change_plan recorded a no-change replan sentinel from a typed passing planner probe",
				plan.ID, plan.Status)
			logging.Info("[emit_change_plan] plan=%s changes=0 status=%s", plan.ID, plan.Status)
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   true,
				Summary:   summary,
				Timestamp: time.Now(),
			}, nil
		}
		summary := "emit_change_plan rejected: changes[] cannot be empty — at least one FileChange is required. " + emitChangePlanSchemaReminder
		if _, ok := activeProofFollowupWorkflowBatch(ctx.Mutable.WriteWorkflowRun()); ok {
			summary = "emit_change_plan rejected: this proof-follow-up batch has no source edit to apply; emit changes: [] only together with verification_probes[] that exercise the already-applied worktree. " + emitChangePlanSchemaReminder
		}
		return rejectPlanToolResult(t.Name(), summary, planRepairPackFromReason(t.Name(), "changes_empty", summary, []string{"$.changes"}, nil)), nil
	}

	// Convert wire-format emitChangePlanChange → canonical
	// types.FileChange once. Every validator below operates on
	// types.FileChange (so the structural emit_plan_skeleton +
	// emit_plan_change path can reuse them); keeping the conversion
	// at the top means the rest of Execute is a single-shape pipeline.
	fcs := emitChangesToFileChanges(p.Changes)

	// B1 Q1 validation — three checks on the changes[] + depends_on
	// graph. All three run BEFORE any Mutable mutation so a rejected
	// emit leaves no partial state.
	//
	// 1) Duplicate-path check. Session-33-Q1 decision: one change
	//    per file per plan. If a planner needs two semantic edits to
	//    the same file, they must be composed into one FileChange
	//    (kind=modify with full content, or kind=patch with combined
	//    diff). Rejecting duplicates keeps DependsOn unambiguously
	//    path-identified and removes a whole class of apply-stage
	//    ordering pathologies.
	if rej, pack := validatePlanGraphIntegrityWithRepair(t.Name(), fcs); rej != "" {
		return rejectPlanToolResult(t.Name(), "emit_change_plan rejected: "+rej, pack), nil
	}

	// Method M (2026-05-07 patch_go_typo r1 forensic): typed-signal
	// hard gate. When the analyzer classified the task as micro
	// scope (single-function / single-constant edit), the plan MUST
	// use kind=patch instead of overwriting the whole file with
	// kind=modify. The gate runs at structural-validate time so
	// the planner sees the rejection in the same dispatch and can
	// re-emit with the right kind.
	if rej := validatePlanScopeKindAlignment(ctx, fcs); rej != "" {
		return rejectPlanToolResult(t.Name(), "emit_change_plan rejected: "+rej,
			planRepairPackWithEnums(t.Name(), "scope_kind_alignment_failed", rej, []string{"$.changes[].kind", "$.changes[].patch", "$.changes[].edits"}, map[string][]string{
				"$.changes[].kind": {"patch"},
			})), nil
	}

	// 3) Cycle detection. A cyclic DependsOn graph has no valid
	//    apply order — reject with the specific cycle path so the
	//    planner's retry (B1 fail-loud surface) can fix it.
	if cycle := detectDepsCycle(fcs); cycle != "" {
		summary := fmt.Sprintf("emit_change_plan rejected: depends_on cycle detected: %s", cycle)
		return rejectPlanToolResult(t.Name(), summary, planRepairPackFromReason(t.Name(), "depends_on_cycle", summary, []string{"$.changes[].depends_on"}, nil)), nil
	}

	// 4) Patch pre-flight. For every kind=patch change, dry-run
	//    `git apply --check` against the current RepoRoot (pre-apply
	//    main repo HEAD). A malformed unified diff (wrong hunk counts,
	//    missing trailing newline, drifted context) fails this check
	//    verbatim with git's stderr — session-35 root cause of
	//    intermittent apply-phase blowups, where 2/3 of planner-
	//    generated diffs had structurally invalid hunk headers and
	//    the coder could not recover (apply_patch sources from Mutable,
	//    so retries re-read the same bad diff).
	//
	//    Pre-flight rejection returns a clear tool-result error,
	//    letting the planner's ShouldStop loop retry (cap 3) within
	//    the same dispatch so a single planner call gets multiple
	//    diff attempts before giving up. Happy-path cost is one git
	//    invocation per patch unit — cheap relative to the LLM dispatch.
	//
	//    Skipped silently when GitAvailable() is false (test harness /
	//    container without git). Skipped when RepoRoot is empty
	//    (unexpected in write mode, but belt-and-suspenders for
	//    tool-layer unit tests that don't set it).
	if rej, pack := validatePlanFullContentWithRepair(ctx, t.Name(), p.Summary, fcs, probes); rej != "" {
		return rejectPlanToolResult(t.Name(), "emit_change_plan rejected: "+rej, pack), nil
	}

	// 5) Line-structure advisory. Patch style is a noisy signal, so it must
	//    never reject a structurally valid plan. The advisory rides on the
	//    success summary for planner self-correction and audit visibility;
	//    hard acceptance stays governed by typed validators above.

	// Build the internal ChangePlan + populate target_paths from
	// the (already converted + already validated) changes slice.
	plan := newChangePlanFromChanges(strings.TrimSpace(p.Request), strings.TrimSpace(p.Summary), fcs, p.AcceptanceTests, probes)
	attachWriteBehaviorContracts(ctx, plan)
	enrichVerificationProbeRefs(plan)

	// Drain any per-language "unvalidated" reasons collected by the
	// dry-build helpers (commit 7 P1-E gap-fix) into the finalised
	// plan. /plan show renders these so the operator knows the
	// plan reached apply with one or more languages skipped due to
	// missing toolchains, instead of seeing "validated and passed"
	// when half the static gate didn't fire.
	plan.UnvalidatedReasons = ctx.Mutable.DrainUnvalidatedReasons()

	// Record a pending apply entry per file so downstream evaluators
	// (CritPlanReady) can observe the plan size without needing to
	// deep-inspect the ChangePlan itself.
	wc := ctx.Mutable.WriteClosure()
	for _, fc := range plan.Changes {
		wc.EnqueuePendingApply(types.PendingApply{
			Path:      fc.Path,
			Rationale: fc.Rationale,
			Origin:    "emit_change_plan",
		})
	}

	ctx.Mutable.SetChangePlan(plan)

	summary := fmt.Sprintf(
		"[emit_change_plan: id=%s changes=%d target_paths=%d acceptance_tests=%d verification_probes=%d]\n"+
			"emit_change_plan recorded",
		plan.ID, len(plan.Changes), len(plan.TargetPaths), len(plan.AcceptanceTests), len(plan.VerificationProbes))
	summary += patchStyleAdvisoryNote(plan.Changes)
	logging.Info("[emit_change_plan] plan=%s changes=%d paths=%d tests=%d probes=%d",
		plan.ID, len(plan.Changes), len(plan.TargetPaths), len(plan.AcceptanceTests), len(plan.VerificationProbes))

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		Timestamp: time.Now(),
	}, nil
}

func noChangeRequiredReplanSentinel(ctx *types.BusContext, p emitChangePlanParams) *types.ChangePlan {
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() || ctx.PipelineStage != types.StagePlan {
		return nil
	}
	handoff := ctx.Mutable.VerifyFailureHandoff()
	if handoff == nil || strings.TrimSpace(handoff.PlanID) == "" {
		return nil
	}
	qualification := writeflow.QualifyNoChangeReplanSentinel(writeflow.NoChangeReplanQualificationInput{
		VerifyFailureHandoff: handoff,
		PlannerProbeReports:  ctx.Mutable.PlanStageProbeReports(),
	})
	if !qualification.Allowed {
		return nil
	}
	request := strings.TrimSpace(p.Request)
	if request == "" {
		request = strings.TrimSpace(ctx.Mutable.Objective())
	}
	if request == "" {
		request = "replan after verification failure"
	}
	summary := strings.TrimSpace(p.Summary)
	if summary == "" {
		summary = "No additional file changes are required because the latest typed planner probe passed against the already-applied worktree."
	}
	return &types.ChangePlan{
		ID:              fmt.Sprintf("noop-%s-%d-%d", sanitizeNoChangePlanIDComponent(handoff.PlanID), time.Now().UnixNano(), os.Getpid()),
		Request:         request,
		Summary:         summary,
		AcceptanceTests: append([]string(nil), p.AcceptanceTests...),
		Status:          types.PlanStatusNoChangeRequired,
		CreatedAt:       time.Now(),
	}
}

func proofFollowupProbeOnlyPlanSentinel(ctx *types.BusContext, p emitChangePlanParams, probes []types.VerificationProbe) *types.ChangePlan {
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() || ctx.PipelineStage != types.StagePlan || len(probes) == 0 {
		return nil
	}
	batch, ok := activeProofFollowupWorkflowBatch(ctx.Mutable.WriteWorkflowRun())
	if !ok {
		return nil
	}
	request := strings.TrimSpace(p.Request)
	if request == "" {
		request = strings.TrimSpace(ctx.Mutable.Objective())
	}
	if request == "" {
		request = "close verification proof obligations"
	}
	summary := strings.TrimSpace(p.Summary)
	if summary == "" {
		summary = "No source edits are required for this proof-follow-up batch; the plan reruns typed verification probes against the already-applied worktree."
	}
	plan := newChangePlanFromChanges(request, summary, nil, p.AcceptanceTests, probes)
	plan.Status = types.PlanStatusNoChangeRequired
	plan.TargetPaths = append([]string(nil), batch.ExpectedPaths...)
	if len(plan.TargetPaths) == 0 {
		plan.TargetPaths = append([]string(nil), batch.ExploreTargets...)
	}
	plan.TargetPaths = dedupTrimEmitChangePlanStrings(plan.TargetPaths)
	return plan
}

func activeProofFollowupWorkflowBatch(run *types.WriteWorkflowRun) (writeflow.WriteBatchPlan, bool) {
	if run == nil {
		return writeflow.WriteBatchPlan{}, false
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" {
		return writeflow.WriteBatchPlan{}, false
	}
	progressAuthorized := false
	for _, event := range run.ProgressLedger {
		if strings.TrimSpace(event.ReasonCode) == "verification_proof_followup_requested" {
			progressAuthorized = true
			break
		}
	}
	if !progressAuthorized {
		return writeflow.WriteBatchPlan{}, false
	}
	for _, candidate := range run.Batches {
		if strings.TrimSpace(candidate.ID) != activeID {
			continue
		}
		purpose := strings.TrimSpace(candidate.Purpose)
		if purpose != "verification_proof_followup" && purpose != "impact_and_verification_proof_followup" {
			return writeflow.WriteBatchPlan{}, false
		}
		return writeflow.WriteBatchPlan{
			ID:              candidate.ID,
			Goal:            candidate.Goal,
			Purpose:         candidate.Purpose,
			ExpectedPaths:   append([]string(nil), candidate.ExpectedPaths...),
			SuccessCriteria: append([]string(nil), candidate.SuccessCriteria...),
		}, true
	}
	return writeflow.WriteBatchPlan{}, false
}

func dedupTrimEmitChangePlanStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(filepath.ToSlash(raw))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func sanitizeNoChangePlanIDComponent(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "prior"
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		}
		if b.Len() >= 48 {
			break
		}
	}
	if b.Len() == 0 {
		return "prior"
	}
	return b.String()
}

// isLegalChangeKind mirrors types.FileChange.Kind legal enum.
// Kept private to the tool package because the emit-side schema
// already enforces via JSON schema enum; this is Execute's runtime
// belt-and-suspenders (LLM can emit strictly-invalid JSON in rare
// corner cases).
func isLegalChangeKind(k string) bool {
	switch k {
	case "create", "modify", "delete", "patch", "rename":
		return true
	}
	return false
}

// detectDepsCycle returns a non-empty string describing a cycle in
// the DependsOn graph when one exists, or "" when the graph is a
// valid DAG. The returned string is the cycle path joined with
// " -> " (e.g. "a.go -> b.go -> a.go") so the rejection message
// names the specific cycle rather than a generic "cycle detected".
//
// Algorithm: classic DFS with three-color marking. For small plans
// (typical N ≤ 20 changes) the O(V+E) cost is negligible and the
// implementation stays readable. Self-edges and multi-step cycles
// are both detected by the same gray-encounter check.
//
// Callers have already validated that every DependsOn entry refers
// to a real change in the plan (see the "Unknown-depends_on" check
// above); detectDepsCycle assumes that precondition and does not
// re-verify. If the assumption is violated the worst case is an
// early return with "" (dep refers to a path not in the graph →
// the walk just stops at that branch) which is still safe.
func detectDepsCycle(changes []types.FileChange) string {
	// Build path -> deps adjacency map.
	deps := make(map[string][]string, len(changes))
	for _, c := range changes {
		path := strings.TrimSpace(c.Path)
		trimmed := make([]string, 0, len(c.DependsOn))
		for _, d := range c.DependsOn {
			trimmed = append(trimmed, strings.TrimSpace(d))
		}
		deps[path] = trimmed
	}

	const (
		white = 0 // unvisited
		gray  = 1 // on current DFS stack (cycle candidate if revisited)
		black = 2 // fully explored
	)
	color := make(map[string]int, len(deps))
	for p := range deps {
		color[p] = white
	}

	var stack []string // DFS path for cycle-path reconstruction
	var visit func(p string) string
	visit = func(p string) string {
		switch color[p] {
		case gray:
			// Cycle reached — find where p first appears on stack
			// so the returned path is exactly the cycle, no prefix.
			start := 0
			for i, s := range stack {
				if s == p {
					start = i
					break
				}
			}
			return strings.Join(append(append([]string(nil), stack[start:]...), p), " -> ")
		case black:
			return ""
		}
		color[p] = gray
		stack = append(stack, p)
		for _, dep := range deps[p] {
			// Skip deps not in graph — upstream validation already
			// rejected them, but be defensive: if called on a
			// malformed input we don't want to panic.
			if _, ok := color[dep]; !ok {
				continue
			}
			if cyc := visit(dep); cyc != "" {
				return cyc
			}
		}
		color[p] = black
		stack = stack[:len(stack)-1]
		return ""
	}

	for p := range deps {
		if color[p] == white {
			if cyc := visit(p); cyc != "" {
				return cyc
			}
		}
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────
// V1 deps-closure
// ─────────────────────────────────────────────────────────────────────

// validatePlanDepsClosure parses every Go new_content for its import
// list and verifies each non-stdlib import is satisfied by either
// (a) the existing repo's go.mod require block or (b) a require line
// added by a go.mod modify entry within the same plan.
//
// Returns "" on success or a human-readable rejection reason describing
// the first unsatisfied import (the planner's next retry only needs to
// know the first failure to fix; piling up all of them increases prompt
// noise without improving recovery rate).
//
// Degraded behaviour (returns ""):
//   - repoRoot empty, no go.mod readable: nothing to validate against.
//   - new_content fails go/parser: handled silently — V2 dry-build will
//     catch syntax errors with better stderr.
//
// "Project module path" (the value of the `module` line in go.mod) is
// treated as internal — imports starting with `<module-path>/...` need
// no go.mod entry.
func validatePlanDepsClosure(repoRoot string, changes []types.FileChange) string {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return ""
	}
	currentModulePath, currentRequires, err := readGoMod(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		// Repo isn't a Go module (or go.mod unreadable). Nothing to
		// validate; degrade silently — V2 dry-build will catch real
		// issues, and non-Go repos legitimately have no go.mod.
		return ""
	}
	// Overlay any go.mod modify entry from this plan.
	planRequires := unionStringSet(currentRequires, nil)
	planModulePath := currentModulePath
	for _, c := range changes {
		if filepath.ToSlash(strings.TrimSpace(c.Path)) != "go.mod" {
			continue
		}
		if c.Kind != "modify" && c.Kind != "create" {
			continue
		}
		if pm, prs, err := parseGoModBytes([]byte(c.NewContent)); err == nil {
			if pm != "" {
				planModulePath = pm
			}
			for r := range prs {
				planRequires[r] = struct{}{}
			}
		}
	}
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if c.Kind == "delete" || c.Kind == "patch" {
			// patch is validated separately by git apply --check;
			// delete has no content to inspect.
			continue
		}
		imports, err := parseGoImports(c.NewContent)
		if err != nil {
			// Syntax issue — V2 will surface it with `go vet`'s richer
			// diagnostics. Don't double-report here.
			continue
		}
		for _, imp := range imports {
			if isStdlibImport(imp) {
				continue
			}
			if planModulePath != "" && (imp == planModulePath || strings.HasPrefix(imp, planModulePath+"/")) {
				continue
			}
			// External import — must be in the (possibly overlaid)
			// require set.
			if !requireSatisfies(planRequires, imp) {
				return fmt.Sprintf("change %q imports %q which is not declared in go.mod. "+
					"Add a 'modify' entry for go.mod adding `require %s vX.Y.Z` (pick a recent stable version), "+
					"or remove the import from new_content. The deps-closure validator (V1) catches this so "+
					"the apply phase doesn't waste a turn failing `go build` on a missing dep.",
					c.Path, imp, requireRoot(imp))
			}
		}
	}
	return ""
}

// readGoMod parses an on-disk go.mod and returns (modulePath,
// requireSet, err). The requireSet contains the bare module paths
// (without version) of every direct + indirect require line.
func readGoMod(path string) (string, map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	return parseGoModBytes(data)
}

// parseGoModBytes is the shared parser used for both on-disk go.mod
// and plan-overlay go.mod content. Tolerant: extracts module path +
// require lines (single + block form), ignores everything else
// (replace, exclude, retract — irrelevant to our deps-presence check).
func parseGoModBytes(data []byte) (string, map[string]struct{}, error) {
	requires := make(map[string]struct{})
	var modulePath string
	scanner := newLineScanner(data)
	inRequireBlock := false
	for scanner.scan() {
		line := strings.TrimSpace(scanner.text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if !inRequireBlock {
			if strings.HasPrefix(line, "module ") {
				modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module"))
				modulePath = strings.Trim(modulePath, "\"")
				continue
			}
			if line == "require (" {
				inRequireBlock = true
				continue
			}
			if strings.HasPrefix(line, "require ") {
				if mod := requireBareModule(strings.TrimSpace(strings.TrimPrefix(line, "require"))); mod != "" {
					requires[mod] = struct{}{}
				}
				continue
			}
		} else {
			if line == ")" {
				inRequireBlock = false
				continue
			}
			// Strip trailing // indirect comment.
			if i := strings.Index(line, "//"); i >= 0 {
				line = strings.TrimSpace(line[:i])
			}
			if mod := requireBareModule(line); mod != "" {
				requires[mod] = struct{}{}
			}
		}
	}
	return modulePath, requires, nil
}

// requireBareModule extracts the module path from a "<module> <version>"
// line, dropping the version. Empty input or single-token input → "".
func requireBareModule(s string) string {
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

// requireSatisfies reports whether the given import path is rooted at
// any module already in the require set. e.g. requires={"github.com/
// pkg/sftp"} satisfies imp="github.com/pkg/sftp/internal/foo".
func requireSatisfies(requires map[string]struct{}, imp string) bool {
	if _, ok := requires[imp]; ok {
		return true
	}
	for mod := range requires {
		if strings.HasPrefix(imp, mod+"/") {
			return true
		}
	}
	return false
}

// requireRoot returns the most likely module-root of an import path
// for the rejection-message hint. e.g. "github.com/pkg/sftp/internal"
// → "github.com/pkg/sftp" (3-segment heuristic). Falls back to the
// full import for short paths.
func requireRoot(imp string) string {
	parts := strings.Split(imp, "/")
	if len(parts) >= 3 && (strings.Contains(parts[0], ".") || parts[0] == "golang.org") {
		return strings.Join(parts[:3], "/")
	}
	return imp
}

// parseGoImports returns the import paths of a Go source file. Comments
// and the `import "C"` cgo pseudo-import are skipped.
func parseGoImports(src string) ([]string, error) {
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "validate.go", src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		if imp.Path == nil {
			continue
		}
		val := strings.Trim(imp.Path.Value, "\"")
		if val == "" || val == "C" {
			continue
		}
		out = append(out, val)
	}
	return out, nil
}

// isStdlibImport reports whether a given import path is part of the
// Go standard library. The stdlib characteristic: NO dot in the first
// path segment (stdlib packages live in single-word top-level dirs).
// External packages live under domains like github.com/, golang.org/x/.
func isStdlibImport(imp string) bool {
	if imp == "" {
		return true
	}
	first := imp
	if i := strings.Index(imp, "/"); i >= 0 {
		first = imp[:i]
	}
	return !strings.Contains(first, ".")
}

// ─────────────────────────────────────────────────────────────────────
// V3 wiring-closure
// ─────────────────────────────────────────────────────────────────────

// validatePlanWiringClosure rejects plans that create a new file in a
// registered subsystem (mcp / skill / tool / agent) without also
// modifying the corresponding wiring file. Returns "" when every
// triggered anchor is satisfied; otherwise returns the rejection
// reason describing the first violation.
//
// "Modify" includes both kind=modify and kind=patch — both produce
// edits to the existing wiring file. Pure kind=create of the wiring
// file would itself be a wiring change but the anchors are designed
// for files that already exist (cmd/root.go, defaults.go) so creates
// of those files are themselves suspicious and reported.
func validatePlanWiringClosure(changes []types.FileChange) string {
	// Collect the set of paths that get modified or patched in this
	// plan — these are the candidate wiring touches.
	wired := make(map[string]struct{})
	for _, c := range changes {
		if c.Kind == "modify" || c.Kind == "patch" {
			wired[filepath.ToSlash(strings.TrimSpace(c.Path))] = struct{}{}
		}
	}
	for _, c := range changes {
		if c.Kind != "create" {
			continue
		}
		anchor := MatchWiringAnchor(c.Path)
		if anchor == nil {
			continue
		}
		if _, ok := wired[anchor.WiringFile]; ok {
			continue
		}
		return fmt.Sprintf("change %q creates a new file under %q but the plan does NOT include a modify/patch entry for %q. %s. "+
			"Add a 'modify' entry for %s containing the new registration call, then re-emit emit_change_plan.",
			c.Path, anchor.Dir, anchor.WiringFile, anchor.Reason, anchor.WiringFile)
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────
// V4 summary-changes consistency
// ─────────────────────────────────────────────────────────────────────

// pathTokenRe captures path-shaped tokens in plan summary prose.
// Examples it matches: internal/mcp/ssh.go, cmd/root.go, go.mod.
// Anchored on `.go` / `.md` / `.yaml` / `.json` / `.toml` / `.mod`
// suffixes plus a leading directory component to avoid false-positives
// on bare words like "summary.txt" appearing in mid-sentence.
var pathTokenRe = regexp.MustCompile(`[\w./_-]*\w+/[\w./_-]+\.(go|md|yaml|yml|json|toml|mod)\b`)

// importTokenRe captures Go import-path-shaped tokens in summary
// prose. Examples: github.com/pkg/sftp, golang.org/x/crypto/ssh.
// The leading "github.com/" / "golang.org/" / "go.uber.org/" pattern
// is a safe filter that avoids matching bare host names.
var importTokenRe = regexp.MustCompile(`(?:github\.com|golang\.org|go\.uber\.org|google\.golang\.org|gopkg\.in)/[\w./_-]+`)

// validatePlanSummaryConsistency cross-checks the prose summary against
// the structured changes[]:
//
//   - Every path-shaped token in summary MUST appear as a changes[].path
//     OR be an existing repo file (we don't require the summary to ONLY
//     name new paths — referencing existing files for context is OK).
//     The bug we're catching is summary saying "ssh.go" while the plan
//     creates "ssh_server.go" (Run #2's misleading inconsistency).
//   - Every import-path-shaped token in summary MUST be importable by
//     the new Go code (i.e. appear in the parsed imports of any
//     new_content) OR be in the existing repo's go.mod requires. Catches
//     Run #1's "uses golang.org/x/crypto/sftp" hallucinated package.
//
// Best-effort — when both checks succeed we return ""; on first
// violation we return the rejection reason naming the unmatched token.
func validatePlanSummaryConsistency(summary string, changes []types.FileChange) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	declaredPaths := make(map[string]struct{})
	allImports := make(map[string]struct{})
	for _, c := range changes {
		declaredPaths[filepath.ToSlash(strings.TrimSpace(c.Path))] = struct{}{}
		if strings.HasSuffix(c.Path, ".go") && c.Kind != "delete" && c.Kind != "patch" {
			if imps, err := parseGoImports(c.NewContent); err == nil {
				for _, imp := range imps {
					allImports[imp] = struct{}{}
				}
			}
		}
	}
	// Path-token check.
	for _, tok := range uniqueMatches(pathTokenRe, summary) {
		clean := filepath.ToSlash(tok)
		if _, ok := declaredPaths[clean]; ok {
			continue
		}
		// "Plan does not name an existing-file reference" is a softer
		// bar — we only reject when the summary names a *.go path
		// that is neither in changes[] NOR in the existing repo. This
		// avoids false positives on context references like
		// "internal/types/context.go" that the summary mentions but
		// doesn't change. For now: only enforce on unique match against
		// declaredPaths when both summary and changes claim the same
		// "new file" intent — heuristic: summary token contains a
		// new-ish word like "新增"/"create"/"new file" nearby. Keep
		// strict for now and tighten later if false-positive rate
		// observed in production.
		if !strings.HasSuffix(clean, ".go") && !strings.HasSuffix(clean, ".mod") {
			continue
		}
		// Skip well-known infrastructure files that summaries often
		// reference for context (these aren't "we're changing X" claims).
		if isCommonContextPath(clean) {
			continue
		}
		// Look for evidence the summary is CLAIMING this path is part
		// of the change (vs just mentioning context). Heuristic: the
		// path appears within 50 chars of a creation/modification verb.
		if !mentionedAsChangeTarget(summary, tok) {
			continue
		}
		// Final check: this is the smoking gun — summary asserts
		// "we're creating/modifying X" but changes[] contains no such
		// path. Reject.
		return fmt.Sprintf("summary claims to create/modify %q but no changes[] entry has that path. "+
			"Either add the path to changes[] or fix the summary to name the actual paths (%s). "+
			"V4 summary-consistency catches this lying so plan reviewers / approval UI don't see misleading prose.",
			tok, formatPathSet(declaredPaths))
	}
	// Import-token check — strict. An import-shaped token in summary
	// must either appear in the parsed imports of new_content OR in
	// the existing go.mod (when readable; we skip the latter check
	// because we don't have repoRoot here — V1 already caught
	// missing-from-go.mod cases. So here we verify summary's claimed
	// packages match the actual code).
	for _, tok := range uniqueMatches(importTokenRe, summary) {
		if _, ok := allImports[tok]; ok {
			continue
		}
		// Tolerate prefix relationship: summary says "golang.org/x/crypto"
		// (umbrella) but code imports "golang.org/x/crypto/ssh" (sub-
		// package) — accept.
		matched := false
		for imp := range allImports {
			if strings.HasPrefix(imp, tok+"/") || strings.HasPrefix(tok, imp+"/") {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		return fmt.Sprintf("summary mentions import path %q but no new_content actually imports it. "+
			"Either add an import in the relevant Go file or remove the false claim from summary "+
			"(actual imports across this plan: %s). V4 catches summary hallucination so the plan's "+
			"prose stays honest about what dependencies are being introduced.",
			tok, formatImportSet(allImports))
	}
	return ""
}

// mentionedAsChangeTarget reports whether the given path token appears
// near a verb that asserts the plan is changing (not just referencing)
// that path. Window is ±60 chars. Pure-Chinese cues are weighted equally
// with English cues since the user's prompt language may be either.
func mentionedAsChangeTarget(summary, token string) bool {
	cues := []string{
		"create", "creates", "creating", "modify", "modifies", "modifying",
		"add", "adds", "adding", "new file", "delete", "deletes",
		"新增", "新建", "创建", "修改", "更新", "添加", "删除",
	}
	idx := strings.Index(summary, token)
	if idx < 0 {
		return false
	}
	start := idx - 60
	if start < 0 {
		start = 0
	}
	end := idx + len(token) + 60
	if end > len(summary) {
		end = len(summary)
	}
	window := summary[start:end]
	for _, cue := range cues {
		if strings.Contains(window, cue) {
			return true
		}
	}
	return false
}

// isCommonContextPath returns true for paths that summaries routinely
// reference for orientation but are never themselves the target of a
// change (e.g. "internal/types/context.go" mentioned to explain types).
// Bypasses the V4 reject for those, eliminating noise.
func isCommonContextPath(path string) bool {
	commonContexts := []string{
		"internal/types/context.go",
		"internal/types/config.go",
		"internal/types/enums.go",
		"docs/architecture.md",
	}
	for _, c := range commonContexts {
		if path == c {
			return true
		}
	}
	return false
}

// uniqueMatches returns the regex's match list with duplicates removed,
// preserving insertion order.
func uniqueMatches(re *regexp.Regexp, src string) []string {
	matches := re.FindAllString(src, -1)
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

// formatPathSet returns a stable comma-separated list of paths for
// rejection-message embedding. Sorted for determinism.
func formatPathSet(set map[string]struct{}) string {
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// formatImportSet is the import-set sibling of formatPathSet. Returns
// "(none)" for empty input so the rejection message reads naturally.
func formatImportSet(set map[string]struct{}) string {
	if len(set) == 0 {
		return "(none)"
	}
	return formatPathSet(set)
}

// ─────────────────────────────────────────────────────────────────────
// V2 dry-build
// ─────────────────────────────────────────────────────────────────────

// validatePlanDryBuild stages every Go change into a scratch directory
// (an overlay of the main repo + the plan's modifications) and runs
// `go vet` on the impacted packages. Catches compile-level issues that
// V1's import-presence check can't detect: undefined identifiers (Run
// #2's `client.Connected()`), type mismatches, function signature
// drift, etc.
//
// Strategy: hardlink the repo into a scratch dir (cheap — just inodes,
// no data copy), then overlay the plan's new_content / modifications,
// then `go vet` the union of impacted package directories. Hardlinks
// keep the staging cost in single-digit ms even for large repos; the
// scratch dir is rm -rf'd on exit.
//
// Skipped silently when:
//   - ctx.RepoRoot is empty or has no go.mod (not a Go project).
//   - `go` binary is unavailable on PATH (CI / minimal containers).
//   - Plan touches zero .go files (nothing to vet).
//
// A non-zero `go vet` exit is treated as a hard rejection — its stderr
// is embedded in the rejection so the planner sees the line:col of the
// failure.
func validatePlanDryBuild(ctx *types.BusContext, changes []types.FileChange) string {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return ""
	}
	// Per-language fan-out. Each helper inspects `changes` for files
	// of its language, sets up the scratch overlay if there are any,
	// runs the language-specific syntax/semantic check, and returns
	// either "" (pass / not-applicable / skipped) or a rejection
	// string. Helpers run in declaration order — the first failure
	// wins so the planner sees ONE actionable error per emit. This
	// is symmetric to the prior Go-only design; each language fix
	// is additive.
	if rej := dryBuildGo(ctx, changes); rej != "" {
		return rej
	}
	if rej := dryBuildPython(ctx, changes); rej != "" {
		return rej
	}
	if rej := dryBuildNodeJS(ctx, changes); rej != "" {
		return rej
	}
	if rej := dryBuildRuby(ctx, changes); rej != "" {
		return rej
	}
	if rej := dryBuildRust(ctx, changes); rej != "" {
		return rej
	}
	if rej := dryBuildSwift(ctx, changes); rej != "" {
		return rej
	}
	if rej := dryBuildJava(ctx, changes); rej != "" {
		return rej
	}
	if rej := dryBuildKotlin(ctx, changes); rej != "" {
		return rej
	}
	return ""
}

// dryBuildGo runs `go vet ./<pkg>...` on an overlayed scratch dir to
// catch compile-level errors in plan-emit time. Skips when the repo
// has no go.mod, the `go` binary is missing, or no Go change is in
// the plan.
func dryBuildGo(ctx *types.BusContext, changes []types.FileChange) string {
	repoRoot := ctx.RepoRoot
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		return ""
	}
	if _, err := exec.LookPath("go"); err != nil {
		logging.Debug("[emit_change_plan] V2 Go dry-build skipped: go binary not on PATH")
		return ""
	}
	impactedPkgs := make(map[string]struct{})
	hasChange := false
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		hasChange = true
		dir := filepath.Dir(path)
		if dir == "" || dir == "." {
			impactedPkgs["."] = struct{}{}
		} else {
			impactedPkgs[dir] = struct{}{}
		}
	}
	if !hasChange {
		return ""
	}
	scratch, cleanup, ok := stageOverlay(ctx, changes, "go")
	if !ok {
		return ""
	}
	defer cleanup()

	pkgs := make([]string, 0, len(impactedPkgs))
	for p := range impactedPkgs {
		pkgs = append(pkgs, "./"+p)
	}
	sort.Strings(pkgs)
	args := append([]string{"vet"}, pkgs...)
	cmd := exec.Command("go", args...)
	cmd.Dir = scratch
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return formatDryBuildRejection("Go", "go vet "+strings.Join(pkgs, " "), out, len(out))
	}
	logging.Debug("[emit_change_plan] V2 Go dry-build PASS for packages: %v", pkgs)
	return ""
}

// dryBuildPython runs `python3 -m py_compile` on every changed .py
// file. py_compile catches all syntax errors AND the most common
// NameError/ImportError that AST validation can detect at parse time
// (note: full type-check would need mypy, an extra dep we don't
// require). Skips when no python3 binary is available or no .py
// change is present. Symmetric to the Go path.
func dryBuildPython(ctx *types.BusContext, changes []types.FileChange) string {
	pyRunner, ok := resolvePythonDryBuildRunner()
	if !ok {
		logging.Debug("[emit_change_plan] V2 Python dry-build skipped: no working python interpreter available")
		return ""
	}
	var pyChanges []string
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".py") {
			continue
		}
		if c.Kind == "delete" {
			continue
		}
		pyChanges = append(pyChanges, path)
	}
	if len(pyChanges) == 0 {
		return ""
	}
	scratch, cleanup, ok := stageOverlay(ctx, changes, "python")
	if !ok {
		return ""
	}
	defer cleanup()

	sort.Strings(pyChanges)
	args := append(append([]string{}, pyRunner.FixedArgs...), "-m", "py_compile")
	args = append(args, pyChanges...)
	cmd := exec.Command(pyRunner.ExePath, args...)
	cmd.Dir = scratch
	cmd.Env = pythonPreflightEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		displayArgs := append(append([]string{}, pyRunner.DisplayArgs...), "-m", "py_compile")
		displayArgs = append(displayArgs, pyChanges...)
		return formatDryBuildRejection("Python", strings.Join(displayArgs, " "), out, len(out))
	}
	if rej := validatePythonDiscardBindingUsage(ctx.RepoRoot, scratch, changes); rej != "" {
		return rej
	}
	logging.Debug("[emit_change_plan] V2 Python dry-build PASS for files: %v", pyChanges)
	return ""
}

type pythonDiscardBindingIntro struct {
	Path      string
	Name      string
	AddedLine string
}

func validatePythonDiscardBindingUsage(repoRoot, finalRoot string, changes []types.FileChange) string {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(finalRoot) == "" {
		return ""
	}
	intros := pythonDiscardBindingIntroductions(repoRoot, changes)
	if len(intros) == 0 {
		return ""
	}
	type key struct {
		path string
		name string
	}
	addedCounts := make(map[key]int)
	addedLines := make(map[key]string)
	for _, intro := range intros {
		k := key{path: intro.Path, name: intro.Name}
		addedCounts[k] += pythonIdentifierOccurrenceCount(intro.AddedLine, intro.Name)
		if addedLines[k] == "" {
			addedLines[k] = strings.TrimSpace(intro.AddedLine)
		}
	}
	for k, assignmentCount := range addedCounts {
		if assignmentCount <= 0 {
			assignmentCount = 1
		}
		data, err := os.ReadFile(filepath.Join(finalRoot, filepath.FromSlash(k.path)))
		if err != nil {
			continue
		}
		if pythonIdentifierOccurrenceCount(string(data), k.name) <= assignmentCount {
			return fmt.Sprintf("change %q replaces a Python discard binding `_` with %q but the planned file never reads %q. This is a dead edit; either use the named value in the relevant logic or keep `_` as the discard target. Added line: %s",
				k.path, k.name, k.name, addedLines[k])
		}
	}
	return ""
}

func pythonDiscardBindingIntroductions(repoRoot string, changes []types.FileChange) []pythonDiscardBindingIntro {
	var out []pythonDiscardBindingIntro
	for _, change := range changes {
		path := filepath.ToSlash(strings.TrimSpace(change.Path))
		if path == "" || !strings.HasSuffix(path, ".py") || strings.TrimSpace(change.Kind) != "patch" {
			continue
		}
		patch := strings.TrimSpace(change.Patch)
		if patch == "" && len(change.Edits) > 0 {
			compiled, err := compileStructuredEditsToPatch(repoRoot, &change)
			if err != nil {
				continue
			}
			patch = compiled
		}
		if patch == "" {
			continue
		}
		out = append(out, pythonDiscardBindingIntroductionsFromPatch(path, patch)...)
	}
	return out
}

func pythonDiscardBindingIntroductionsFromPatch(path, patch string) []pythonDiscardBindingIntro {
	var out []pythonDiscardBindingIntro
	var minusQueue []string
	for _, raw := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(raw, "@@"):
			minusQueue = nil
		case strings.HasPrefix(raw, "---") || strings.HasPrefix(raw, "+++"):
			continue
		case strings.HasPrefix(raw, "-"):
			minusQueue = append(minusQueue, strings.TrimPrefix(raw, "-"))
		case strings.HasPrefix(raw, "+"):
			added := strings.TrimPrefix(raw, "+")
			if len(minusQueue) == 0 {
				continue
			}
			removed := minusQueue[0]
			minusQueue = minusQueue[1:]
			for _, name := range pythonDiscardBindingNamesIntroduced(removed, added) {
				out = append(out, pythonDiscardBindingIntro{
					Path:      path,
					Name:      name,
					AddedLine: added,
				})
			}
		default:
			minusQueue = nil
		}
	}
	return out
}

func pythonDiscardBindingNamesIntroduced(removed, added string) []string {
	oldTarget, oldRHS, ok := splitPythonSimpleAssignment(removed)
	if !ok {
		return nil
	}
	newTarget, newRHS, ok := splitPythonSimpleAssignment(added)
	if !ok || strings.TrimSpace(oldRHS) != strings.TrimSpace(newRHS) {
		return nil
	}
	oldAtoms := pythonAssignmentTargetAtoms(oldTarget)
	newAtoms := pythonAssignmentTargetAtoms(newTarget)
	if len(oldAtoms) == 0 || len(oldAtoms) != len(newAtoms) {
		return nil
	}
	var out []string
	for i := range oldAtoms {
		if oldAtoms[i] == "_" && pythonDiscardReplacementIdentifier(newAtoms[i]) {
			out = append(out, newAtoms[i])
		}
	}
	return out
}

func splitPythonSimpleAssignment(line string) (target, rhs string, ok bool) {
	for i := 0; i < len(line); i++ {
		if line[i] != '=' {
			continue
		}
		prev := byte(0)
		next := byte(0)
		if i > 0 {
			prev = line[i-1]
		}
		if i+1 < len(line) {
			next = line[i+1]
		}
		if prev == '=' || prev == '!' || prev == '<' || prev == '>' || prev == ':' || next == '=' {
			continue
		}
		return line[:i], line[i+1:], true
	}
	return "", "", false
}

func pythonAssignmentTargetAtoms(target string) []string {
	target = strings.TrimSpace(target)
	if target == "" || strings.Contains(target, ".") {
		return nil
	}
	for {
		trimmed := strings.TrimSpace(target)
		if len(trimmed) >= 2 {
			first, last := trimmed[0], trimmed[len(trimmed)-1]
			if (first == '(' && last == ')') || (first == '[' && last == ']') {
				target = trimmed[1 : len(trimmed)-1]
				continue
			}
		}
		break
	}
	parts := strings.Split(target, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "*"))
		if idx := strings.Index(part, ":"); idx >= 0 {
			part = strings.TrimSpace(part[:idx])
		}
		if part == "" {
			return nil
		}
		out = append(out, part)
	}
	return out
}

func pythonDiscardReplacementIdentifier(name string) bool {
	if name == "" || name == "_" || strings.HasPrefix(name, "_") {
		return false
	}
	return regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(name)
}

func pythonIdentifierOccurrenceCount(content, name string) int {
	if name == "" {
		return 0
	}
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	return len(re.FindAllStringIndex(content, -1))
}

type pythonDryBuildRunner struct {
	ExePath     string
	FixedArgs   []string
	DisplayArgs []string
}

func resolvePythonDryBuildRunner() (pythonDryBuildRunner, bool) {
	candidates := []pythonDryBuildRunner{
		{DisplayArgs: []string{"python3"}},
		{DisplayArgs: []string{"python"}},
	}
	if runtime.GOOS == "windows" {
		candidates = []pythonDryBuildRunner{
			{DisplayArgs: []string{"py", "-3"}, FixedArgs: []string{"-3"}},
			{DisplayArgs: []string{"python"}},
			{DisplayArgs: []string{"python3"}},
		}
	}
	for _, candidate := range candidates {
		if len(candidate.DisplayArgs) == 0 {
			continue
		}
		exePath, err := exec.LookPath(candidate.DisplayArgs[0])
		if err != nil {
			continue
		}
		candidate.ExePath = exePath
		if probePythonDryBuildRunner(candidate) {
			return candidate, true
		}
	}
	return pythonDryBuildRunner{}, false
}

func probePythonDryBuildRunner(runner pythonDryBuildRunner) bool {
	if strings.TrimSpace(runner.ExePath) == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	args := append(append([]string{}, runner.FixedArgs...), "-c", "import sys")
	cmd := exec.CommandContext(ctx, runner.ExePath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		logging.Debug("[emit_change_plan] V2 Python dry-build probe failed for %q: %v (out=%q)", strings.Join(runner.DisplayArgs, " "), err, strings.TrimSpace(string(out)))
		return false
	}
	return true
}

// dryBuildNodeJS runs `node --check` on every changed .js / .mjs /
// .cjs file. node --check parses the file and reports SyntaxError
// without executing it. TypeScript files are skipped here because
// `tsc --noEmit` requires a tsconfig.json + tsc binary that varies
// per project; we leave TS to the project's own test runner.
//
// ESM detection: node --check runs in CommonJS mode by default,
// which rejects valid ES module syntax (`export`, top-level await,
// `import.meta`, etc.). When the project's package.json declares
// `"type": "module"` OR the source is .mjs, those .js files ARE ES
// modules and should be checked under module semantics. We rename
// to .mjs in the overlay scratch dir before invoking node --check
// so the LLM-generated valid ESM isn't falsely rejected. The
// original LLM-emitted file path is unchanged in the rejection
// message so the planner sees the right path on retry.
//
// Bug provenance: eval Batch I+J binary-js task — LLM emitted
// valid `export class Binary {...}` but V2 dry-build rejected with
// "SyntaxError: Unexpected token 'export'", killing the plan
// without retrying because the validator was wrong.
func dryBuildNodeJS(ctx *types.BusContext, changes []types.FileChange) string {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		logging.Debug("[emit_change_plan] V2 Node dry-build skipped: node binary not on PATH")
		return ""
	}
	var jsChanges []string
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !(strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".mjs") || strings.HasSuffix(path, ".cjs")) {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		jsChanges = append(jsChanges, path)
	}
	if len(jsChanges) == 0 {
		return ""
	}
	scratch, cleanup, ok := stageOverlay(ctx, changes, "node")
	if !ok {
		return ""
	}
	defer cleanup()

	// Detect ES module mode at TWO levels:
	//
	//  1. Project-level: package.json declares "type": "module".
	//     The simplest, most explicit signal; one flip of one flag.
	//
	//  2. Per-file content-level: the file ITSELF uses ESM syntax
	//     (top-level `import` / `export`). Catches the wide class of
	//     projects that compile ESM source to CJS at build time —
	//     Babel, webpack, jest with babel-jest, esbuild, parcel,
	//     vitest, etc. None of these set type:module on the manifest,
	//     yet their source files are valid ESM that `node --check`
	//     rejects under default CJS semantics.
	//
	// If either signal fires for a given file, we rename it to .mjs
	// in the scratch overlay so node treats it as ESM during the
	// check. The rename is per-file, so a CJS file in a Babel project
	// (e.g. `jest.config.cjs`) is still checked under CJS.
	//
	// Bug provenance: Batch L binary-js + space-age-js — both
	// projects use Babel/Jest with ESM source but no type:module,
	// so V2 dry-build false-positively rejected valid LLM output
	// with "Unexpected token 'export'". The per-file content scan
	// generalizes the .mjs rename without maintaining an opt-in
	// list of transformer toolchains.
	projectESM := nodePackageJSONIsModule(filepath.Join(scratch, "package.json"))

	// node --check is per-file; concatenate diagnostics from every
	// failing file so the planner sees the full picture in one shot.
	sort.Strings(jsChanges)
	var combinedOut []byte
	failed := false
	for _, p := range jsChanges {
		// In ESM mode, .js is parsed under module semantics. node
		// --check on a `.js` file with `type:module` package would
		// behave correctly... in theory. In practice node 18+
		// honours type:module for the file's parent package.json
		// IF the file is invoked relative to that package. Rename
		// to .mjs in the scratch dir to disambiguate — node always
		// treats .mjs as module regardless of package.json.
		//
		// Per-file ESM detection: the .mjs rename also fires when
		// the file's own contents use top-level import/export
		// (independent of project type). Covers Babel-transformed
		// projects whose source is ESM but manifest is CJS.
		checkPath := p
		fileESM := projectESM
		if !fileESM && strings.HasSuffix(p, ".js") {
			absSrc := filepath.Join(scratch, p)
			if data, err := os.ReadFile(absSrc); err == nil {
				fileESM = containsESMSyntax(data)
			}
		}
		esModule := fileESM
		if esModule && strings.HasSuffix(p, ".js") {
			renamed := p + ".__codrax_mjs.mjs"
			absSrc := filepath.Join(scratch, p)
			absDst := filepath.Join(scratch, renamed)
			if err := os.Rename(absSrc, absDst); err == nil {
				checkPath = renamed
				// Restore original name AFTER node finishes so
				// subsequent overlays see expected layout. Defer
				// in a closure so each iteration cleans its own.
				defer func(src, dst string) { _ = os.Rename(dst, src) }(absSrc, absDst)
			}
		}
		cmd := exec.Command(nodeBin, "--check", checkPath)
		cmd.Dir = scratch
		out, err := cmd.CombinedOutput()
		if err != nil {
			failed = true
			combinedOut = append(combinedOut, []byte("--- "+p+" ---\n")...)
			combinedOut = append(combinedOut, out...)
			combinedOut = append(combinedOut, '\n')
		}
	}
	if failed {
		return formatDryBuildRejection("Node.js", "node --check <files>", combinedOut, len(combinedOut))
	}
	logging.Debug("[emit_change_plan] V2 Node dry-build PASS for files: %v (project_esm=%v)", jsChanges, projectESM)
	return ""
}

// containsESMSyntax returns true when the given JavaScript source
// uses top-level ESM syntax (`import` / `export` keywords at line
// start, ignoring leading whitespace and shebang lines). Used by the
// V2 Node dry-build to auto-detect Babel-style projects whose source
// is ESM but whose package.json doesn't declare `type:module` — those
// projects depend on a build-step transformer (Babel, webpack, jest
// with babel-jest, esbuild, parcel, vitest, etc.) to compile ESM
// source to CJS at runtime, so `node --check` under default CJS
// semantics false-positively rejects the valid source.
//
// Detection is intentionally conservative: only top-of-line keywords
// match, so `// example: import x` in a comment doesn't trip it. The
// regex is anchored at line start (multiline mode) and requires a
// space after the keyword, ruling out identifiers like `importance`.
//
// False-positive cost (treating CJS as ESM): minimal — node accepts
// CJS-style `require` / `module.exports` under module semantics too,
// so the renamed .mjs check still passes. False-negative cost (ESM
// missed): the original Unexpected-token rejection fires, same as
// before this helper existed.
func containsESMSyntax(content []byte) bool {
	return esmSyntaxRegex.Match(content)
}

var esmSyntaxRegex = regexp.MustCompile(`(?m)^\s*(?:import|export)\s`)

// nodePackageJSONIsModule returns true when the package.json at the
// given path declares `"type": "module"`. Errors (file missing,
// malformed JSON, unexpected type) all degrade to false — CommonJS
// is the safer fallback because it accepts a strict subset of what
// ESM accepts (no `export` / `import` keywords without a wrapper).
func nodePackageJSONIsModule(pkgJSONPath string) bool {
	data, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		return false
	}
	var manifest struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return false
	}
	return manifest.Type == "module"
}

// dryBuildRuby runs `ruby -c` on every changed .rb file. -c is
// "syntax check only" and is part of the standard ruby distribution.
func dryBuildRuby(ctx *types.BusContext, changes []types.FileChange) string {
	rubyBin, err := exec.LookPath("ruby")
	if err != nil {
		logging.Debug("[emit_change_plan] V2 Ruby dry-build skipped: ruby binary not on PATH")
		return ""
	}
	var rbChanges []string
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".rb") {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		rbChanges = append(rbChanges, path)
	}
	if len(rbChanges) == 0 {
		return ""
	}
	scratch, cleanup, ok := stageOverlay(ctx, changes, "ruby")
	if !ok {
		return ""
	}
	defer cleanup()

	sort.Strings(rbChanges)
	var combinedOut []byte
	failed := false
	for _, p := range rbChanges {
		cmd := exec.Command(rubyBin, "-c", p)
		cmd.Dir = scratch
		out, err := cmd.CombinedOutput()
		if err != nil {
			failed = true
			combinedOut = append(combinedOut, []byte("--- "+p+" ---\n")...)
			combinedOut = append(combinedOut, out...)
			combinedOut = append(combinedOut, '\n')
		}
	}
	if failed {
		return formatDryBuildRejection("Ruby", "ruby -c <files>", combinedOut, len(combinedOut))
	}
	logging.Debug("[emit_change_plan] V2 Ruby dry-build PASS for files: %v", rbChanges)
	return ""
}

// dryBuildRust runs `cargo check --frozen --offline` on an overlayed
// scratch dir. cargo check parses + typechecks the entire crate
// without code generation, so it catches every compile-level error
// (undeclared symbols, type mismatches, lifetime issues, missing
// trait bounds) at plan-emit time without a multi-second build.
//
// Skipped silently when the repo has no Cargo.toml, the cargo
// binary is missing, or no .rs change is in the plan. --frozen
// blocks the dependency resolver from updating Cargo.lock; --offline
// blocks any network fetch — both keep the dry-build fast and
// hermetic on a CI box.
func dryBuildRust(ctx *types.BusContext, changes []types.FileChange) string {
	repoRoot := ctx.RepoRoot
	if _, err := os.Stat(filepath.Join(repoRoot, "Cargo.toml")); err != nil {
		return ""
	}
	if _, err := exec.LookPath("cargo"); err != nil {
		logging.Warning("[emit_change_plan] V2 Rust dry-build skipped: cargo binary not on PATH (plan unvalidated for Rust)")
		recordUnvalidated(ctx, "rust: cargo not in PATH")
		return ""
	}
	hasChange := false
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".rs") {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		hasChange = true
		break
	}
	if !hasChange {
		return ""
	}
	scratch, cleanup, ok := stageOverlay(ctx, changes, "rust")
	if !ok {
		return ""
	}
	defer cleanup()

	cmd := exec.Command("cargo", "check", "--frozen", "--offline", "--message-format=short")
	cmd.Dir = scratch
	out, err := cmd.CombinedOutput()
	if err != nil {
		return formatDryBuildRejection("Rust", "cargo check --frozen --offline", out, len(out))
	}
	logging.Debug("[emit_change_plan] V2 Rust dry-build PASS")
	return ""
}

// dryBuildSwift runs `swift build --skip-build` on an overlayed
// scratch dir. The flag instructs swiftpm to fully resolve and
// type-check the package without invoking the linker / code
// generator — fast and sufficient to catch every compile-level
// error at plan time.
//
// Skipped silently when the repo has no Package.swift, the swift
// binary is missing, or no .swift change is in the plan.
func dryBuildSwift(ctx *types.BusContext, changes []types.FileChange) string {
	repoRoot := ctx.RepoRoot
	if _, err := os.Stat(filepath.Join(repoRoot, "Package.swift")); err != nil {
		return ""
	}
	if _, err := exec.LookPath("swift"); err != nil {
		logging.Warning("[emit_change_plan] V2 Swift dry-build skipped: swift binary not on PATH (plan unvalidated for Swift)")
		recordUnvalidated(ctx, "swift: swift binary not in PATH")
		return ""
	}
	hasChange := false
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".swift") {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		hasChange = true
		break
	}
	if !hasChange {
		return ""
	}
	scratch, cleanup, ok := stageOverlay(ctx, changes, "swift")
	if !ok {
		return ""
	}
	defer cleanup()

	cmd := exec.Command("swift", "build", "--skip-build")
	cmd.Dir = scratch
	out, err := cmd.CombinedOutput()
	if err != nil {
		return formatDryBuildRejection("Swift", "swift build --skip-build", out, len(out))
	}
	logging.Debug("[emit_change_plan] V2 Swift dry-build PASS")
	return ""
}

// dryBuildJava runs the Maven or Gradle compile target (skipping
// tests) on an overlayed scratch dir. Detects the build system from
// the manifest layout: pom.xml → mvn; build.gradle / build.gradle.kts
// → ./gradlew if a wrapper script is present, else `gradle`.
//
// Skipped silently when neither manifest is present, no build tool
// binary is on PATH, or the plan touches zero .java files. Compile-
// only mode (-DskipTests=true / -x test) keeps the dry-build fast
// — full test runs belong to the verifier stage.
func dryBuildJava(ctx *types.BusContext, changes []types.FileChange) string {
	repoRoot := ctx.RepoRoot
	hasChange := false
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".java") {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		hasChange = true
		break
	}
	if !hasChange {
		return ""
	}

	spec, unavailable := javaCompileCommandSpec(repoRoot)
	if unavailable != "" {
		logging.Warning("[emit_change_plan] V2 Java dry-build skipped: %s (plan unvalidated for Java)", unavailable)
		recordUnvalidated(ctx, "java: "+unavailable)
		return ""
	}
	if spec.Name == "" {
		return ""
	}

	scratch, cleanup, ok := stageOverlay(ctx, changes, "java")
	if !ok {
		return ""
	}
	defer cleanup()

	cmd := exec.Command(spec.Name, spec.Args...)
	cmd.Dir = scratch
	out, err := cmd.CombinedOutput()
	if err != nil {
		return formatDryBuildRejection("Java", spec.Label, out, len(out))
	}
	logging.Debug("[emit_change_plan] V2 Java dry-build PASS via %s", spec.Label)
	return ""
}

type javaCompileCommand struct {
	Name  string
	Args  []string
	Label string
}

func javaCompileCommandSpec(repoRoot string) (javaCompileCommand, string) {
	pomPath := filepath.Join(repoRoot, "pom.xml")
	gradleKts := filepath.Join(repoRoot, "build.gradle.kts")
	gradleGroovy := filepath.Join(repoRoot, "build.gradle")
	switch {
	case fileExists(pomPath):
		if _, err := exec.LookPath("mvn"); err != nil {
			return javaCompileCommand{}, "java/maven: mvn not in PATH"
		}
		return javaCompileCommand{
			Name:  "mvn",
			Args:  []string{"-B", "-q", "compile", "-DskipTests=true", "-o"},
			Label: "mvn -B -q compile -DskipTests=true -o",
		}, ""
	case fileExists(gradleKts) || fileExists(gradleGroovy):
		wrapper := filepath.Join(repoRoot, "gradlew")
		if fileExists(wrapper) {
			return javaCompileCommand{
				Name:  wrapper,
				Args:  []string{"compileJava", "--offline", "-q", "-x", "test"},
				Label: "./gradlew compileJava --offline -x test",
			}, ""
		}
		if _, err := exec.LookPath("gradle"); err != nil {
			return javaCompileCommand{}, "java/gradle: neither ./gradlew nor system gradle available"
		}
		return javaCompileCommand{
			Name:  "gradle",
			Args:  []string{"compileJava", "--offline", "-q", "-x", "test"},
			Label: "gradle compileJava --offline -x test",
		}, ""
	default:
		return javaCompileCommand{}, ""
	}
}

// dryBuildKotlin runs `kotlinc -nowarn` on each changed .kt file in
// the overlayed scratch dir. The compiler typechecks each source
// against its own imports and signatures; without classpath
// resolution it can't catch cross-file reference errors, but it
// reliably catches per-file syntax / type-shape errors which is
// the bulk of LLM-emit defects.
//
// Skipped silently when no kotlinc is available or no .kt change
// is in the plan. Per-file invocation mirrors the Ruby path —
// keeps each error attributable to a specific source.
func dryBuildKotlin(ctx *types.BusContext, changes []types.FileChange) string {
	if _, err := exec.LookPath("kotlinc"); err != nil {
		// Even when no kotlinc is on PATH, only WARN when the plan
		// actually touches Kotlin files; otherwise the noise floor
		// from a Go-only repo would be unacceptable.
		for _, c := range changes {
			path := filepath.ToSlash(strings.TrimSpace(c.Path))
			if !strings.HasSuffix(path, ".kt") {
				continue
			}
			if c.Kind == "patch" || c.Kind == "delete" {
				continue
			}
			logging.Warning("[emit_change_plan] V2 Kotlin dry-build skipped: kotlinc binary not on PATH (plan unvalidated for Kotlin)")
			recordUnvalidated(ctx, "kotlin: kotlinc not in PATH")
			break
		}
		return ""
	}
	var ktChanges []string
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".kt") {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		ktChanges = append(ktChanges, path)
	}
	if len(ktChanges) == 0 {
		return ""
	}
	scratch, cleanup, ok := stageOverlay(ctx, changes, "kotlin")
	if !ok {
		return ""
	}
	defer cleanup()

	// Output dir captured to a sub-folder of scratch so cleanup
	// removes generated .class files together with the staging.
	outDir := filepath.Join(scratch, ".codrax-kotlinc-out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		logging.Warning("[emit_change_plan] V2 Kotlin dry-build skipped: mkdir outDir: %v", err)
		return ""
	}
	failed := false
	var combinedOut []byte
	for _, p := range ktChanges {
		cmd := exec.Command("kotlinc", "-d", outDir, "-nowarn", filepath.Join(scratch, filepath.FromSlash(p)))
		cmd.Dir = scratch
		out, err := cmd.CombinedOutput()
		if err != nil {
			failed = true
			combinedOut = append(combinedOut, []byte("--- "+p+" ---\n")...)
			combinedOut = append(combinedOut, out...)
			combinedOut = append(combinedOut, '\n')
		}
	}
	if failed {
		return formatDryBuildRejection("Kotlin", "kotlinc -d <tmp> -nowarn <files>", combinedOut, len(combinedOut))
	}
	logging.Debug("[emit_change_plan] V2 Kotlin dry-build PASS for files: %v", ktChanges)
	return ""
}

// recordUnvalidated appends a per-language unvalidated reason to
// MutableState's collector when the BusContext carries one. The
// collector is drained into ChangePlan.UnvalidatedReasons by
// emit_change_plan.Execute after validation succeeds. No-op when
// ctx or Mutable is nil (test fixtures, defensive).
func recordUnvalidated(ctx *types.BusContext, reason string) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	ctx.Mutable.RecordUnvalidatedReason(reason)
}

// fileExists is a tiny os.Stat wrapper used by the Java dry-build
// to detect manifest variants. Silent on every error path — a
// missing manifest is the common case.
func fileExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

// stageOverlay creates a hardlinked scratch dir of the repo and
// overlays the plan's create/modify changes onto it. Returns the
// scratch path + cleanup callback + ok flag. Shared by every
// per-language dry-build helper so the expensive part (hardlink the
// repo) happens at most once per call but the per-language wrappers
// stay independent. lang is logged-only (so a Python skip doesn't
// look like a Go skip in the trace).
func stageOverlay(ctx *types.BusContext, changes []types.FileChange, lang string) (string, func(), bool) {
	scratch, err := os.MkdirTemp(scratchBaseDir(ctx), "codrax-validate-"+lang+"-*")
	if err != nil {
		logging.Warning("[emit_change_plan] V2 %s dry-build skipped: mkdir temp: %v", lang, err)
		return "", func() {}, false
	}
	cleanup := func() { _ = os.RemoveAll(scratch) }
	if err := hardlinkTree(ctx.RepoRoot, scratch); err != nil {
		logging.Warning("[emit_change_plan] V2 %s dry-build skipped: hardlink tree: %v", lang, err)
		cleanup()
		return "", func() {}, false
	}
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if c.Kind == "patch" {
			if strings.TrimSpace(c.Patch) == "" {
				continue
			}
			if !GitAvailable() {
				recordUnvalidated(ctx, fmt.Sprintf("%s: patch dry-build skipped because git is not available", lang))
				logging.Debug("[emit_change_plan] V2 %s dry-build skipped: patch overlay requires git", lang)
				cleanup()
				return "", func() {}, false
			}
			if _, err := runUnifiedDiff(scratch, c.Patch, false); err != nil {
				logging.Warning("[emit_change_plan] V2 %s dry-build skipped: apply patch %s in scratch: %v", lang, path, err)
				cleanup()
				return "", func() {}, false
			}
			continue
		}
		dst := filepath.Join(scratch, filepath.FromSlash(path))
		if c.Kind == "delete" {
			_ = os.Remove(dst)
			continue
		}
		_ = os.Remove(dst)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			logging.Warning("[emit_change_plan] V2 %s dry-build skipped: mkdir %s: %v", lang, filepath.Dir(dst), err)
			cleanup()
			return "", func() {}, false
		}
		if err := os.WriteFile(dst, []byte(c.NewContent), 0o644); err != nil {
			logging.Warning("[emit_change_plan] V2 %s dry-build skipped: write %s: %v", lang, dst, err)
			cleanup()
			return "", func() {}, false
		}
	}
	return scratch, cleanup, true
}

// formatDryBuildRejection assembles the rejection text shown to the
// planner. lang names the language for the planner ("Go" / "Python");
// cmdDescr is the command form for diagnosis ("go vet ./..." etc).
// stderr is the captured combined output; truncated at 2 KB so the
// rejection stays prompt-friendly.
func formatDryBuildRejection(lang, cmdDescr string, stderr []byte, originalLen int) string {
	const maxStderrLen = 2000
	out := strings.TrimSpace(string(stderr))
	if len(out) > maxStderrLen {
		out = out[:maxStderrLen] + "\n... (truncated; full output had " + fmt.Sprintf("%d", originalLen) + " bytes)"
	}
	return fmt.Sprintf("dry-build failed: `%s` returned non-zero exit (%s). "+
		"The new_content for one or more files has compile / syntax / type errors — read the file:line references in the output below to identify which file and which line is broken, then re-emit emit_change_plan (or emit_plan_change for that one file in multi-round mode) with the corrected new_content. Do NOT re-emit unchanged content; the same error will recur. Full output:\n%s",
		cmdDescr, lang, out)
}

// ─────────────────────────────────────────────────────────────────────
// V5 lint — registry-driven, language-symmetric
// ─────────────────────────────────────────────────────────────────────

// validatePlanLint catches "code-smell" / unused-symbol / dead-code
// / style problems that V2's syntax + vet pass cannot see. The Batch
// E quality audit surfaced patterns this catches: unused imports,
// dead variables, unreachable returns, over-broad excepts.
//
// Implementation: a SINGLE generic loop walks lintRegistry. Each
// language is a data row — adding a new language is one struct
// literal, no Go function. Symmetric to run_tests' supportedRunner-
// Manifests registry. The previous per-language Go function pattern
// (lintPython / lintGoFmt) was the same anti-pattern we already
// removed from run_tests.go's detectRunner — code growing with
// every language is not generalisation, it's hardcoded fan-out.
//
// Severity policy (load-bearing):
//   - kind=create  → strict lint runs (new file, no excuse for
//     defects; LLM has full control of the bytes)
//   - kind=modify  → SKIPPED (pre-existing file likely had the same
//     pattern; rejecting would create churn; lint diff would be
//     more correct but is Phase 2)
//   - kind=patch / kind=delete → SKIPPED (no full file content)
//
// Operator opt-out: codrax.yaml :: pipeline_lint_enabled: false
// disables the whole family. Per-language opt-in is implicit:
// helpers short-circuit when the toolchain binary is missing.
//
// Toolchain availability is silent — a missing `ruff` simply skips
// the Python row; we don't error. Operators who want a specific
// language enforced must install its linter explicitly. INFO-level
// startup log lists which languages are active so operators can
// audit at a glance.
func validatePlanLint(ctx *types.BusContext, changes []types.FileChange) string {
	if !LintEnabled() {
		return ""
	}
	if ctx == nil {
		return ""
	}
	for _, lang := range lintRegistry {
		if rej := runLintForLang(ctx, changes, lang); rej != "" {
			return rej
		}
	}
	return ""
}

// LintLang declares one language's V5 lint rule. Registry-driven
// extension: adding a new language requires one entry, no new Go
// function. Field semantics are intentionally narrow so each row
// stays grep-readable; per-language quirks (severity filters,
// stdout-vs-stderr semantics) live in the BuildArgs / FailedFn
// closures.
type LintLang struct {
	// Name is the human-facing language label used in rejection
	// text + log. ("Python", "Go", "JavaScript", "Rust", "Ruby",
	// "Java", "Swift").
	Name string

	// Extensions is the file suffixes (with leading dot) this lang
	// claims. A change qualifies if its path ends in any of these.
	Extensions []string

	// Binary is the linter executable to LookPath. When missing,
	// the row is silently skipped.
	Binary string

	// Description is shown in startup log so operators can audit
	// what's active. ("ruff check (E,F families)", "gofmt -l",
	// "node --check", etc.)
	Description string

	// BuildArgs maps the (sorted) qualified file list to the
	// linter's argv. The scratch dir is set as the cmd's cwd by
	// runLintForLang, so paths can stay repo-relative.
	BuildArgs func(files []string) []string

	// FailedFn parses the linter's combined output + exec error to
	// decide whether this run constitutes a lint failure. Many
	// linters use exit code (=> err != nil); a few (gofmt -l) use
	// non-empty stdout instead. Returns (failed, rejection-msg).
	// Empty string = failed but no message (caller falls back to
	// formatLintRejection).
	FailedFn func(combined []byte, execErr error) (failed bool, msg string)
}

// lintRegistry enumerates every language V5 knows how to lint.
// Order matters: helpers run sequentially and the first failure
// wins so the planner sees ONE actionable rejection per emit. Order
// is alphabetical by language for predictability — change with care.
//
// Coverage status (matches run_tests.go runner whitelist size 12):
//
//	✓ C          — gcc -Wall -Wextra -Werror -fsyntax-only (system gcc)
//	✓ C++        — g++ -Wall -Wextra -Werror -fsyntax-only -std=c++17 (system g++)
//	✓ Go         — gofmt -l (bundled with go toolchain)
//	✓ Java       — javac -Xlint:all (bundled with JDK)
//	✓ JavaScript — node --check (bundled with node)
//	✓ Python     — ruff check --select=E,F (pip install ruff)
//	✓ Ruby       — ruby -wc (bundled with ruby)
//	✓ Rust       — rustc --edition=2021 --emit=metadata (bundled with rustup)
//	✓ Swift      — swift -frontend -typecheck (bundled with swift)
//	✓ TypeScript — tsc --noEmit --strict (single file; npm install -g typescript)
//	✗ ArkTS      — hvigor lint requires project-level oh-package.json5; deferred
//	               to a future "project-aware" V6 layer. Single-file lint is
//	               not viable for ArkTS by design (decorators reach into the
//	               project's bundle map).
//	✗ Cangjie    — cjpm check requires cjpm.toml + module resolution; same
//	               project-context constraint as ArkTS. Defer until upstream
//	               ships a single-file standalone checker (or until we add
//	               a V6 project-aware lint layer).
//	✗ CMake/Meson/Make — these are build-system declarative files, not
//	               "code" in the lint sense. cmake-lint exists but is a
//	               style nitpicker; meson has only a formatter. Make has
//	               no real linter. Adding any of these would generate
//	               noise without value.
//
// 10 of 12 covered with first-class V5 lint. The 2 deferred languages
// (ArkTS, Cangjie) need project-context awareness that doesn't fit
// the single-file overlay pattern V5 builds on; the 3 build-system
// formats are out of scope for "code lint." A future V6 layer can
// add project-aware lint for the project-context families.
var lintRegistry = []LintLang{
	{
		Name:        "C",
		Extensions:  []string{".c", ".h"},
		Binary:      "gcc",
		Description: "gcc -Wall -Wextra -Werror -fsyntax-only (parse + warning-as-error catch)",
		BuildArgs: func(files []string) []string {
			// -fsyntax-only: parse + analyse without codegen (fast).
			// -Werror: promote warnings (unused-var, dead-branch,
			// uninitialised-use) to hard fails so V5 routes them to
			// the planner. Per-file invocation; multi-file gcc would
			// drag in headers we don't want to require.
			return []string{"-Wall", "-Wextra", "-Werror", "-fsyntax-only", files[0]}
		},
		FailedFn: defaultExitCodeFailedFn,
	},
	{
		Name:        "C++",
		Extensions:  []string{".cc", ".cpp", ".cxx", ".hpp", ".hh"},
		Binary:      "g++",
		Description: "g++ -Wall -Wextra -Werror -fsyntax-only -std=c++17 (parse + warning-as-error catch)",
		BuildArgs: func(files []string) []string {
			return []string{"-Wall", "-Wextra", "-Werror", "-fsyntax-only", "-std=c++17", files[0]}
		},
		FailedFn: defaultExitCodeFailedFn,
	},
	{
		Name:        "Go",
		Extensions:  []string{".go"},
		Binary:      "gofmt",
		Description: "gofmt -l (formatting check; semantic covered by V2 go vet)",
		BuildArgs:   func(files []string) []string { return append([]string{"-l"}, files...) },
		FailedFn: func(out []byte, err error) (bool, string) {
			// gofmt -l prints offending file paths to stdout; exit
			// code is 0 even when files are dirty. A non-empty
			// listing = files need formatting.
			listing := strings.TrimSpace(string(out))
			if listing == "" {
				return false, ""
			}
			return true, "Files not gofmt-clean (format with `gofmt -s -w`):\n" + listing
		},
	},
	{
		Name:        "Java",
		Extensions:  []string{".java"},
		Binary:      "javac",
		Description: "javac -Xlint:all (annotations / unchecked / fallthrough; build-only flag set)",
		BuildArgs: func(files []string) []string {
			return append([]string{"-Xlint:all", "-Werror", "-d", os.TempDir(), "-implicit:none"}, files...)
		},
		FailedFn: defaultExitCodeFailedFn,
	},
	{
		Name:        "JavaScript",
		Extensions:  []string{".js", ".mjs", ".cjs"},
		Binary:      "node",
		Description: "node --check (parse + early-error catch; full eslint deferred — config-fragile)",
		BuildArgs: func(files []string) []string {
			// node --check is per-file; we run the first failing
			// file's diagnosis (or all-files single command if node
			// supports multi). To stay portable, batch one cmd:
			// node accepts `-e` for inline but for --check the form
			// is `node --check <file>` per file. Use first-file
			// invocation; FailedFn iterates remaining via a sentinel.
			return append([]string{"--check"}, files...)
		},
		FailedFn: defaultExitCodeFailedFn,
	},
	{
		Name:        "Python",
		Extensions:  []string{".py"},
		Binary:      "ruff",
		Description: "ruff check --select=E,F (pycodestyle errors + pyflakes unused-vars/imports/names)",
		BuildArgs: func(files []string) []string {
			return append([]string{"check", "--select=E,F", "--no-cache", "--output-format=concise"}, files...)
		},
		FailedFn: defaultExitCodeFailedFn,
	},
	{
		Name:        "Ruby",
		Extensions:  []string{".rb"},
		Binary:      "ruby",
		Description: "ruby -wc (-w warn + -c syntax check, bundled with ruby)",
		BuildArgs: func(files []string) []string {
			// ruby -wc only takes ONE file. Batched via -e is awkward.
			// Run on first file; for multi-file plans, we'd loop —
			// punted to FailedFn awareness via the harness pattern,
			// but for now first-file is most LLM-typical (single new
			// .rb file per plan).
			return []string{"-wc", files[0]}
		},
		FailedFn: defaultExitCodeFailedFn,
	},
	{
		Name:        "Rust",
		Extensions:  []string{".rs"},
		Binary:      "rustc",
		Description: "rustc --edition=2021 --crate-type=lib --emit=metadata -o /dev/null (parse + borrow check)",
		BuildArgs: func(files []string) []string {
			// Per-file rustc check; --emit=metadata avoids actual
			// codegen so it's fast. -o /dev/null suppresses output.
			// Multi-file rustc would need a Cargo project — out of
			// scope for V5.
			return []string{"--edition=2021", "--crate-type=lib", "--emit=metadata", "-o", "/dev/null", files[0]}
		},
		FailedFn: defaultExitCodeFailedFn,
	},
	{
		Name:        "Swift",
		Extensions:  []string{".swift"},
		Binary:      "swift",
		Description: "swift -frontend -typecheck (parse + type-check, bundled with swift toolchain)",
		BuildArgs: func(files []string) []string {
			return append([]string{"-frontend", "-typecheck"}, files...)
		},
		FailedFn: defaultExitCodeFailedFn,
	},
	{
		Name:        "TypeScript",
		Extensions:  []string{".ts", ".tsx"},
		Binary:      "tsc",
		Description: "tsc --noEmit --strict --target=es2020 (single-file type-check; npm install -g typescript)",
		BuildArgs: func(files []string) []string {
			// Single-file tsc invocation works without tsconfig.json
			// for files with no relative imports (the common new-
			// file case). --skipLibCheck avoids tsc trying to
			// type-check stdlib types in node_modules.
			return append([]string{"--noEmit", "--strict", "--target=es2020", "--skipLibCheck"}, files...)
		},
		FailedFn: defaultExitCodeFailedFn,
	},
}

// defaultExitCodeFailedFn is the canonical "non-zero exit = failure"
// predicate for linters whose only signal is the process exit code.
// The combined output is surfaced verbatim in the rejection message
// (no need for a custom message — the linter's own diagnostics are
// what the planner needs to read).
func defaultExitCodeFailedFn(out []byte, err error) (bool, string) {
	if err == nil {
		return false, ""
	}
	return true, ""
}

// runLintForLang dispatches one row of lintRegistry. Filters changes
// to this language's extensions, applies the kind=create severity
// gate, sets up the overlay, runs the configured argv, and routes
// failures through formatLintRejection. Symmetric to the dry-build
// fan-out in shape but generic in content.
func runLintForLang(ctx *types.BusContext, changes []types.FileChange, lang LintLang) string {
	bin, err := exec.LookPath(lang.Binary)
	if err != nil {
		logging.Debug("[emit_change_plan] V5 %s lint skipped: %s binary not on PATH", lang.Name, lang.Binary)
		return ""
	}
	var qualified []string
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !hasAnySuffix(path, lang.Extensions) {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		// Severity policy: strict lint only on NEW files; modify
		// gets no churn. See validatePlanLint docblock.
		if c.Kind != "create" {
			continue
		}
		qualified = append(qualified, path)
	}
	if len(qualified) == 0 {
		return ""
	}
	scratch, cleanup, ok := stageOverlay(ctx, changes, strings.ToLower(lang.Name)+"-lint")
	if !ok {
		return ""
	}
	defer cleanup()

	sort.Strings(qualified)
	cmd := exec.Command(bin, lang.BuildArgs(qualified)...)
	cmd.Dir = scratch
	out, execErr := cmd.CombinedOutput()
	failed, msg := lang.FailedFn(out, execErr)
	if !failed {
		logging.Debug("[emit_change_plan] V5 %s lint PASS for files: %v", lang.Name, qualified)
		return ""
	}
	body := []byte(msg)
	if msg == "" {
		body = out
	}
	cmdDescr := lang.Binary + " " + strings.Join(lang.BuildArgs(qualified), " ")
	return formatLintRejection(lang.Name, cmdDescr, body, len(body))
}

// hasAnySuffix returns true when path ends in any extension from
// the slice. Tiny helper; lifted for grep-discoverability.
func hasAnySuffix(path string, extensions []string) bool {
	for _, ext := range extensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// LintLanguagesEnabled returns the per-language activation status
// table for the startup capability log. Each entry: (Name,
// available, description). Available == true means the toolchain
// binary is on PATH at startup. Used by cmd/root.go to emit one
// INFO line per registered language so operators can audit the
// active V5 surface.
func LintLanguagesEnabled() []LintLangStatus {
	out := make([]LintLangStatus, 0, len(lintRegistry))
	for _, lang := range lintRegistry {
		_, err := exec.LookPath(lang.Binary)
		out = append(out, LintLangStatus{
			Name:        lang.Name,
			Binary:      lang.Binary,
			Available:   err == nil,
			Description: lang.Description,
		})
	}
	return out
}

// LintLangStatus is the per-language availability snapshot.
type LintLangStatus struct {
	Name        string
	Binary      string
	Available   bool
	Description string
}

// formatLintRejection mirrors formatDryBuildRejection but with a V5
// prefix so the planner can distinguish lint failures from
// build/syntax failures. Same truncation cap (2 KB).
func formatLintRejection(lang, cmdDescr string, stderr []byte, originalLen int) string {
	const maxStderrLen = 2000
	out := strings.TrimSpace(string(stderr))
	if len(out) > maxStderrLen {
		out = out[:maxStderrLen] + "\n... (truncated; full output had " + fmt.Sprintf("%d", originalLen) + " bytes)"
	}
	return fmt.Sprintf("V5 lint failed: `%s` reported %s code-smell issues. "+
		"These are not syntax errors — the code likely compiles — but the linter flagged dead variables, "+
		"unused imports, or other defects that indicate the new_content has unnecessary or buggy code. "+
		"Fix the issues and re-emit. Full output:\n%s",
		cmdDescr, lang, out)
}

// ─────────────────────────────────────────────────────────────────────
// V6 project-aware lint
// ─────────────────────────────────────────────────────────────────────

// validatePlanProjectLint covers languages whose linters require
// project-level context (manifest files + module resolution + bundle
// maps) and cannot be sensibly invoked on a single file. ArkTS and
// Cangjie are the canonical cases — both reach into the project's
// oh-package.json5 / cjpm.toml respectively to resolve imports and
// decorators, so V5's single-file overlay pattern returns "no
// project context" errors instead of useful diagnostics.
//
// V6's strategy: run the language's NATIVE project-level lint
// command from the overlayed project root. The overlay is the same
// hardlinked scratch dir V5 uses; we just chdir to project root and
// invoke the command as a project would.
//
// Trigger conditions (ALL must hold):
//  1. Plan touches a file with the language's extension
//     (e.g. .ets for ArkTS, .cj for Cangjie)
//  2. Project root contains the language's manifest file
//     (e.g. oh-package.json5, cjpm.toml) — proves this IS a
//     ${lang} project, not just a file with that extension
//  3. Language's lint binary is on PATH
//  4. SetLintEnabled(true) (same master switch as V5)
//
// Same severity policy as V5: kind=create only; modify skipped.
// Failure routes through formatLintRejection with a "V6 project
// lint" prefix so the planner can route on category.
func validatePlanProjectLint(ctx *types.BusContext, changes []types.FileChange) string {
	if !LintEnabled() {
		return ""
	}
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return ""
	}
	for _, lang := range projectLintRegistry {
		if rej := runProjectLintForLang(ctx, changes, lang); rej != "" {
			return rej
		}
	}
	return ""
}

// ProjectLintLang declares one language's V6 project-aware lint
// rule. Distinguished from LintLang by the ManifestFiles + project-
// relative invocation: V5 builds argv from a file list; V6 builds
// argv from the project root.
type ProjectLintLang struct {
	// Name is the human-facing language label. ("ArkTS", "Cangjie")
	Name string

	// Extensions is the file suffixes (with leading dot) this lang
	// claims. A change qualifies if its path ends in any of these.
	// V5 must NOT also claim these — V5 + V6 are mutually exclusive
	// per language to avoid double-linting.
	Extensions []string

	// ManifestFiles is the set of project-root files at least one
	// of which MUST exist for V6 to consider this a real ${lang}
	// project. Without a manifest the file extension alone is
	// ambiguous (.ts could be plain TS or ArkTS; the oh-package.json5
	// is the disambiguator the toolchain uses).
	ManifestFiles []string

	// Binary is the project-lint command. When missing, V6
	// silently skips (same convention as V5).
	Binary string

	// Description is shown in the V6 startup banner.
	Description string

	// BuildArgs returns the argv to pass to Binary, given the
	// (overlayed) project root. The cmd's cwd is set to that root
	// by runProjectLintForLang, so most commands need only their
	// flags ("lint", "check"...).
	BuildArgs func(projectRoot string) []string

	// FailedFn parses the linter's combined output + exec error to
	// decide whether this run is a failure. Same shape as V5's.
	FailedFn func(combined []byte, execErr error) (failed bool, msg string)
}

// projectLintRegistry enumerates every language V6 knows. Order
// matters: rows run sequentially; first failure wins.
//
// Coverage status (matches the 2 V5-deferred languages):
//
//	✓ ArkTS   — hvigor lint (HarmonyOS DevEco toolchain)
//	✓ Cangjie — cjpm check (Cangjie package manager / build tool)
//
// Both are silently inactive on hosts without the toolchain (the
// LookPath miss + missing-manifest checks both short-circuit). The
// startup banner emitted by tool.LogCapabilities surfaces which V6
// languages are active vs deferred.
var projectLintRegistry = []ProjectLintLang{
	{
		Name:          "ArkTS",
		Extensions:    []string{".ets"},
		ManifestFiles: []string{"oh-package.json5", "build-profile.json5"},
		Binary:        "hvigorw",
		Description:   "hvigor lint (ArkTS project-aware lint via HarmonyOS DevEco toolchain)",
		BuildArgs:     func(_ string) []string { return []string{"lint", "--no-incremental"} },
		FailedFn:      defaultExitCodeFailedFn,
	},
	{
		Name:          "Cangjie",
		Extensions:    []string{".cj"},
		ManifestFiles: []string{"cjpm.toml"},
		Binary:        "cjpm",
		Description:   "cjpm check (Cangjie project parse + type check)",
		BuildArgs:     func(_ string) []string { return []string{"check"} },
		FailedFn:      defaultExitCodeFailedFn,
	},
}

// runProjectLintForLang dispatches one row of projectLintRegistry.
// Walks the trigger conditions: extension match → manifest exists
// → binary on PATH → SetLintEnabled. On match, sets up the overlay
// + chdirs to project root (the overlay's repo root, since the
// manifest must already live at that level — sub-project lint is
// out of scope for V6).
func runProjectLintForLang(ctx *types.BusContext, changes []types.FileChange, lang ProjectLintLang) string {
	// 1. Any change in this language?
	hasLangChange := false
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !hasAnySuffix(path, lang.Extensions) {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		if c.Kind != "create" {
			continue
		}
		hasLangChange = true
		break
	}
	if !hasLangChange {
		return ""
	}
	// 2. Project manifest present in repo root?
	hasManifest := false
	for _, m := range lang.ManifestFiles {
		if _, err := os.Stat(filepath.Join(ctx.RepoRoot, m)); err == nil {
			hasManifest = true
			break
		}
	}
	if !hasManifest {
		logging.Debug("[emit_change_plan] V6 %s lint skipped: project manifest %v not found in %s",
			lang.Name, lang.ManifestFiles, ctx.RepoRoot)
		return ""
	}
	// 3. Binary on PATH?
	bin, err := exec.LookPath(lang.Binary)
	if err != nil {
		logging.Debug("[emit_change_plan] V6 %s lint skipped: %s binary not on PATH", lang.Name, lang.Binary)
		return ""
	}
	// 4. Stage the overlay + run.
	scratch, cleanup, ok := stageOverlay(ctx, changes, strings.ToLower(lang.Name)+"-projectlint")
	if !ok {
		return ""
	}
	defer cleanup()

	cmd := exec.Command(bin, lang.BuildArgs(scratch)...)
	cmd.Dir = scratch
	out, execErr := cmd.CombinedOutput()
	failed, msg := lang.FailedFn(out, execErr)
	if !failed {
		logging.Debug("[emit_change_plan] V6 %s project lint PASS", lang.Name)
		return ""
	}
	body := []byte(msg)
	if msg == "" {
		body = out
	}
	cmdDescr := lang.Binary + " " + strings.Join(lang.BuildArgs(scratch), " ")
	return formatProjectLintRejection(lang.Name, cmdDescr, body, len(body))
}

// ProjectLintLanguagesEnabled returns the per-language activation
// status table for V6, mirroring LintLanguagesEnabled but for the
// project-aware family. cmd/root.go uses this for the startup
// capability log.
func ProjectLintLanguagesEnabled() []LintLangStatus {
	out := make([]LintLangStatus, 0, len(projectLintRegistry))
	for _, lang := range projectLintRegistry {
		_, err := exec.LookPath(lang.Binary)
		out = append(out, LintLangStatus{
			Name:        lang.Name,
			Binary:      lang.Binary,
			Available:   err == nil,
			Description: lang.Description,
		})
	}
	return out
}

// formatProjectLintRejection mirrors formatLintRejection but uses
// "V6 project lint" prefix so the planner can distinguish single-
// file lint from project-aware lint failures.
func formatProjectLintRejection(lang, cmdDescr string, stderr []byte, originalLen int) string {
	const maxStderrLen = 2000
	out := strings.TrimSpace(string(stderr))
	if len(out) > maxStderrLen {
		out = out[:maxStderrLen] + "\n... (truncated; full output had " + fmt.Sprintf("%d", originalLen) + " bytes)"
	}
	return fmt.Sprintf("V6 project lint failed: `%s` reported %s project-level errors. "+
		"The project's native build/lint tool detected issues that single-file checking cannot see "+
		"(typically: bundle map mismatches, decorator misuse, or module resolution failures). "+
		"Fix the issues and re-emit. Full output:\n%s",
		cmdDescr, lang, out)
}

// scratchBaseDir returns the parent directory under which V2's
// scratch dir should be created. Prefers ctx.WorkDir (per-trace temp
// already in place) so temp files cluster with other codrax artifacts;
// falls back to os.TempDir() when WorkDir is unset (degraded contexts
// like unit tests).
func scratchBaseDir(ctx *types.BusContext) string {
	if ctx != nil && strings.TrimSpace(ctx.WorkDir) != "" {
		return ctx.WorkDir
	}
	return ""
}

// hardlinkTree mirrors src into dst by hardlinking every regular file
// (so disk + write-time overhead are negligible). Symlinks and special
// files are skipped — they're rare in Go projects and reproducing them
// adds complexity. Hidden directories (.git, .codrax) are skipped since
// they're irrelevant to `go vet` and would balloon the inode count.
func hardlinkTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Skip noise.
		base := filepath.Base(rel)
		if d.IsDir() {
			if base == ".git" || base == ".codrax" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks / sockets / fifos
		}
		dstPath := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		// Try hardlink first (fast); fall back to copy if cross-device.
		if err := os.Link(path, dstPath); err == nil {
			return nil
		}
		return copyFile(path, dstPath)
	})
}

// copyFile is the cross-device fallback for hardlinkTree.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// ─────────────────────────────────────────────────────────────────────
// helpers shared across validators
// ─────────────────────────────────────────────────────────────────────

// unionStringSet returns the union of a and b as a fresh set.
func unionStringSet(a, b map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		out[k] = struct{}{}
	}
	for k := range b {
		out[k] = struct{}{}
	}
	return out
}

// lineScanner is a minimal byte-buffer line iterator (avoids
// bufio.Scanner's 64KB default token limit problems on large go.mod).
type lineScanner struct {
	data []byte
	pos  int
	line string
}

func newLineScanner(data []byte) *lineScanner { return &lineScanner{data: data} }

func (s *lineScanner) scan() bool {
	if s.pos >= len(s.data) {
		return false
	}
	start := s.pos
	for s.pos < len(s.data) && s.data[s.pos] != '\n' {
		s.pos++
	}
	end := s.pos
	if s.pos < len(s.data) {
		s.pos++ // consume newline
	}
	if end > 0 && s.data[end-1] == '\r' {
		end--
	}
	s.line = string(s.data[start:end])
	return true
}

func (s *lineScanner) text() string { return s.line }
