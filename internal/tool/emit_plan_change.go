package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitPlanChange fills one file's body slot in the partial plan
// installed by emit_plan_skeleton. Each call is small and self-
// contained (one path + one new_content or one patch), so streaming
// truncation cannot bite the way it does on a single-shot
// emit_change_plan emission of a multi-file plan.
//
// When the LAST placeholder in the partial plan is filled, this
// tool runs the content-dependent validators (V1 deps closure /
// V2 dry-build / V4 summary fidelity / patch pre-flight) on the
// assembled plan. On success it promotes Mutable.PartialChangePlan
// to ChangePlan; on validator failure it leaves the partial state
// intact so the planner can retry just the failing path.
//
// See docblock on EmitPlanSkeleton for the design rationale.
type EmitPlanChange struct {
	ReadOnly
	NonEvidenceTool
}

type emitPlanChangeParams struct {
	Path       string                 `json:"path"`
	NewContent string                 `json:"new_content,omitempty"`
	Patch      string                 `json:"patch,omitempty"`
	Edits      []types.StructuredEdit `json:"edits,omitempty"`
}

const emitPlanChangeSchemaReminder = "REQUIRED schema: {path: string (must match a path in the skeleton), " +
	"new_content: string (full file body for kind=create/modify), " +
	"patch: string (unified diff for kind=patch), edits: optional structured line edits for kind=patch}. " +
	"For kind=delete, do NOT call emit_plan_change — delete kinds need no body. " +
	"Call emit_plan_skeleton FIRST; the skeleton sets the per-file kind, this tool fills the body."

func (t *EmitPlanChange) Name() string { return "emit_plan_change" }

func (t *EmitPlanChange) Description() string {
	return "Fill one file's body in the partial plan installed by emit_plan_skeleton. " +
		"For kind=create / kind=modify supply new_content; for kind=patch supply patch (unified diff) or edits (structured line edits). " +
		"The last call (when every non-delete slot is filled) runs the full validators and finalizes the plan."
}

func (t *EmitPlanChange) Parameters() json.RawMessage {
	return json.RawMessage(`{
	  "type": "object",
	  "additionalProperties": false,
	  "properties": {
	    "path":        {"type": "string", "description": "Repo-relative path matching one of the skeleton's changes[].path entries."},
	    "new_content": {"type": "string", "description": "Full body for kind=create / kind=modify."},
	    "patch":       {"type": "string", "description": "Unified diff for kind=patch. Use either patch OR edits, not both."},
	    "edits": {
	      "type": "array",
	      "description": "Optional structured line edits for kind=patch. The tool compiles these into a unified diff against current file bytes. Use either edits OR patch, not both.",
	      "items": {
	        "type": "object",
	        "additionalProperties": false,
	        "properties": {
	          "kind": {"type": "string", "enum": ["replace", "delete", "insert_before", "insert_after"]},
	          "start_line": {"type": "integer", "minimum": 1},
	          "end_line": {"type": "integer", "minimum": 1},
	          "content": {"type": "string"},
	          "old_text": {"type": "string"}
	        },
	        "required": ["kind", "start_line"]
	      }
	    }
	  },
	  "required": ["path"]
	}`)
}

func (t *EmitPlanChange) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_change requires a writable context",
			Timestamp: time.Now(),
		}, nil
	}

	// Truncated payload guard. emit_plan_change inputs can legitimately
	// be small (a tiny patch could be ~50 bytes), so the floor is
	// lower than emit_change_plan's. We pick 20 bytes as the floor:
	// `{"path":"x","new_content":"y"}` = 30 bytes, anything below 20
	// is structurally impossible.
	trimmed := strings.TrimSpace(string(params))
	if len(trimmed) < 20 || trimmed == "{}" || trimmed == "null" || trimmed == "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_change rejected: payload was empty or truncated (got " + fmt.Sprintf("%d", len(trimmed)) + " bytes). " + emitPlanChangeSchemaReminder,
			Timestamp: time.Now(),
		}, nil
	}

	params = applyStructuredPayloadCompat(t.Name(), params, t.Parameters())

	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p emitPlanChangeParams
	if err := dec.Decode(&p); err != nil {
		return failStrictDecodeWithErrorMessage(t.Name(), time.Now(), err, nil, params, "emit_plan_change rejected: ", ". "+emitPlanChangeSchemaReminder)
	}

	path := strings.TrimSpace(p.Path)
	if path == "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_change rejected: path is required and must be non-empty. " + emitPlanChangeSchemaReminder,
			Timestamp: time.Now(),
		}, nil
	}

	// Find the partial plan and the slot to fill. A failure here
	// usually means the planner skipped emit_plan_skeleton — surface
	// the protocol explicitly so the LLM doesn't keep retrying with
	// no skeleton in place.
	partial := ctx.Mutable.PartialChangePlan()
	if partial == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_change rejected: no skeleton in place. Call emit_plan_skeleton FIRST to install the plan metadata, then emit_plan_change once per non-delete file.",
			Timestamp: time.Now(),
		}, nil
	}
	slot := -1
	for i, c := range partial.Changes {
		if c.Path == path {
			slot = i
			break
		}
	}
	if slot < 0 {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("emit_plan_change rejected: path %q is not in the skeleton — it was not declared in emit_plan_skeleton.changes[]. Either correct the path or re-emit the skeleton with this file included.", path),
			Timestamp: time.Now(),
		}, nil
	}

	c := &partial.Changes[slot]
	kind := strings.TrimSpace(c.Kind)
	switch kind {
	case "create", "modify":
		if strings.TrimSpace(p.NewContent) == "" {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("emit_plan_change rejected: change %q (kind=%s) requires non-empty new_content", path, kind),
				Timestamp: time.Now(),
			}, nil
		}
		c.NewContent = p.NewContent
	case "patch":
		if strings.TrimSpace(p.Patch) == "" && len(p.Edits) == 0 {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("emit_plan_change rejected: change %q (kind=patch) requires non-empty patch (unified diff) or edits[]", path),
				Timestamp: time.Now(),
			}, nil
		}
		c.Patch = p.Patch
		c.Edits = append([]types.StructuredEdit(nil), p.Edits...)
	case "delete":
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("emit_plan_change rejected: change %q has kind=delete — delete kinds need no body, do not call emit_plan_change for them. The skeleton already declares the deletion.", path),
			Timestamp: time.Now(),
		}, nil
	default:
		// Should never happen — the skeleton validator rejects
		// unknown kinds before installing PartialChangePlan.
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("emit_plan_change internal error: skeleton change %q has unrecognized kind %q", path, kind),
			Timestamp: time.Now(),
		}, nil
	}

	// Write back the updated slot. Mutable storage is a pointer to
	// the plan, so the in-place edit above is already visible; this
	// SetPartialChangePlan call exists only to take the lock so the
	// in-progress write is serialised w.r.t. concurrent reads.
	ctx.Mutable.SetPartialChangePlan(partial)

	// Completeness check. We finalize when every non-delete slot
	// has its body filled. Delete kinds are completeness-trivial.
	missing := []string{}
	for _, ch := range partial.Changes {
		k := strings.TrimSpace(ch.Kind)
		switch k {
		case "create", "modify":
			if strings.TrimSpace(ch.NewContent) == "" {
				missing = append(missing, ch.Path)
			}
		case "patch":
			if strings.TrimSpace(ch.Patch) == "" && len(ch.Edits) == 0 {
				missing = append(missing, ch.Path)
			}
		}
	}

	if len(missing) > 0 {
		// Still incomplete — report progress and wait for more
		// emit_plan_change calls.
		filled := len(partial.Changes) - len(missing)
		// (deletes count as "filled" trivially, so this is the right
		//  number to surface to the LLM)
		summary := fmt.Sprintf(
			"[emit_plan_change: filled %s | %d/%d slots filled, %d remaining]\n"+
				"Continue with emit_plan_change for: %s",
			path, filled, len(partial.Changes), len(missing),
			strings.Join(missing, ", "))
		logging.Info("[emit_plan_change] plan=%s filled=%s pending=%d/%d remaining=%v",
			partial.ID, path, filled, len(partial.Changes), missing)
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   true,
			Summary:   summary,
			Timestamp: time.Now(),
		}, nil
	}

	// All bodies filled — run the content-dependent validators on
	// the assembled plan. On rejection we leave PartialChangePlan
	// intact so the planner can re-emit the offending file (single
	// emit_plan_change call to fix one path is much smaller than
	// re-running the entire emission).
	if rej := validatePlanFullContent(ctx, partial.Summary, partial.Changes); rej != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_change rejected during finalize: " + rej + " (PartialChangePlan retained — re-emit just the offending file via emit_plan_change to fix)",
			Timestamp: time.Now(),
		}, nil
	}

	// Promote to canonical ChangePlan and seed write closure. After
	// this point Mutable.ChangePlan is non-nil and the planner's
	// ShouldStop terminates the dispatch.
	wc := ctx.Mutable.WriteClosure()
	for _, fc := range partial.Changes {
		wc.EnqueuePendingApply(types.PendingApply{
			Path:      fc.Path,
			Rationale: fc.Rationale,
			Origin:    "emit_plan_change",
		})
	}
	ctx.Mutable.PromotePartialToChangePlan()

	summary := fmt.Sprintf(
		"[emit_plan_change: FINALIZED %s | id=%s changes=%d target_paths=%d acceptance_tests=%d]\n"+
			"All bodies filled and validators passed; plan is ready.",
		path, partial.ID, len(partial.Changes), len(partial.TargetPaths), len(partial.AcceptanceTests))
	logging.Info("[emit_plan_change] plan=%s FINALIZED last_path=%s changes=%d", partial.ID, path, len(partial.Changes))

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		Timestamp: time.Now(),
	}, nil
}
