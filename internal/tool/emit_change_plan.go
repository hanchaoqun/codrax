package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitChangePlan is the planner agent's structured exit channel.
// The LLM emits a ChangePlan describing the code modification it
// proposes; Execute validates the payload shape, stamps a plan ID
// + timestamp, and installs it on BusContext.Mutable so
// runPlanPhase (orchestrator) and cmd/root.go (single-shot disk
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
	Request         string                  `json:"request"`
	Summary         string                  `json:"summary"`
	Changes         []emitChangePlanChange  `json:"changes"`
	AcceptanceTests []string                `json:"acceptance_tests,omitempty"`
}

// emitChangePlanChange mirrors types.FileChange but lives in the tool
// package so the wire-format is independent of the internal struct
// (which may grow new fields post-B0 without breaking old plans).
type emitChangePlanChange struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	NewContent string `json:"new_content,omitempty"`
	Patch     string `json:"patch,omitempty"`
	Rationale string `json:"rationale"`
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
	return json.RawMessage(`{
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
      "description": "Ordered list of file-level modifications. Apply-stage processes them sequentially.",
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
            "description": "Unified-diff payload for kind=patch. B2 only; leave empty in B0."
          },
          "rationale": {
            "type": "string",
            "description": "1-3 sentence explanation for WHY this specific file needs this specific change."
          }
        },
        "required": ["path", "kind", "rationale"]
      }
    },
    "acceptance_tests": {
      "type": "array",
      "description": "Optional list of test assertions the verify stage must cover. Natural-language in B0; formalized to Criterion IR in B1.",
      "items": {"type": "string"}
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
			Summary:   "emit_change_plan requires BusContext.Mutable",
			Timestamp: time.Now(),
		}, nil
	}

	// Strict decode — unknown fields fail loudly so a schema drift
	// surfaces during development, not as silent data loss.
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p emitChangePlanParams
	if err := dec.Decode(&p); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("emit_change_plan rejected: invalid params: %v", err),
			Timestamp: time.Now(),
		}, err
	}

	if strings.TrimSpace(p.Summary) == "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_change_plan rejected: summary is required and must be non-empty",
			Timestamp: time.Now(),
		}, nil
	}
	if len(p.Changes) == 0 {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_change_plan rejected: changes[] cannot be empty — at least one FileChange is required",
			Timestamp: time.Now(),
		}, nil
	}

	// Build the internal ChangePlan + populate target_paths from
	// the changes array. Plan IDs embed trace + nano + pid so two
	// concurrent codrax processes never collide on the same plan
	// file name even when they run in the same clock-nano.
	now := time.Now()
	plan := &types.ChangePlan{
		ID:            fmt.Sprintf("plan-%d-%d", now.UnixNano(), os.Getpid()),
		TriggerTurnID: "", // populated by REPL layer in B0.6+ once /plan state wires up
		SessionID:     "", // same as above
		Request:       strings.TrimSpace(p.Request),
		Summary:       strings.TrimSpace(p.Summary),
		Status:        "pending_approval",
		CreatedAt:     now,
	}
	paths := make(map[string]struct{})
	for _, c := range p.Changes {
		path := strings.TrimSpace(c.Path)
		kind := strings.TrimSpace(c.Kind)
		if path == "" {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   "emit_change_plan rejected: one of the changes has an empty path",
				Timestamp: time.Now(),
			}, nil
		}
		if !isLegalChangeKind(kind) {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("emit_change_plan rejected: change %q has illegal kind %q (must be create|modify|delete|patch)", path, kind),
				Timestamp: time.Now(),
			}, nil
		}
		plan.Changes = append(plan.Changes, types.FileChange{
			Path:       path,
			Kind:       kind,
			NewContent: c.NewContent,
			Patch:      c.Patch,
			Rationale:  strings.TrimSpace(c.Rationale),
		})
		paths[path] = struct{}{}
	}
	for p := range paths {
		plan.TargetPaths = append(plan.TargetPaths, p)
	}
	if len(p.AcceptanceTests) > 0 {
		plan.AcceptanceTests = append([]string(nil), p.AcceptanceTests...)
	}

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
		"[emit_change_plan: id=%s changes=%d target_paths=%d acceptance_tests=%d]\n"+
			"emit_change_plan recorded",
		plan.ID, len(plan.Changes), len(plan.TargetPaths), len(plan.AcceptanceTests))
	logging.Info("[emit_change_plan] plan=%s changes=%d paths=%d tests=%d",
		plan.ID, len(plan.Changes), len(plan.TargetPaths), len(plan.AcceptanceTests))

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		Timestamp: time.Now(),
	}, nil
}

// isLegalChangeKind mirrors types.FileChange.Kind legal enum.
// Kept private to the tool package because the emit-side schema
// already enforces via JSON schema enum; this is Execute's runtime
// belt-and-suspenders (LLM can emit strictly-invalid JSON in rare
// corner cases).
func isLegalChangeKind(k string) bool {
	switch k {
	case "create", "modify", "delete", "patch":
		return true
	}
	return false
}
