package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitPlanSkeleton is the structural-safety-net counterpart to
// EmitChangePlan. The single-shot emit_change_plan path is preferred
// for small plans (1-2 files, modest content) because it lands the
// whole plan in one LLM round. But when a plan touches many files or
// each file's new_content is large, a single tool call can blow past
// the model's output ceiling, get truncated mid-stream, and never
// recover (every retry hits the same cap, the LLM never sees a
// non-truncated emission, and the dispatch loop dies).
//
// The structural fix is to split the emission into two layers:
//
//  1. emit_plan_skeleton(request, summary, changes_meta[]) emits the
//     plan skeleton — every file's path, kind, rationale, depends_on,
//     plus the request + summary + acceptance_tests — but NO file
//     bodies. This payload stays small (~1-2 KB) so it can never
//     truncate. Content-independent validators (graph integrity,
//     wiring closure, summary path-side consistency) run here.
//
//  2. emit_plan_change(path, new_content_or_patch) is called once per
//     non-delete change to fill in the body. Each call is small and
//     self-contained. When the LAST placeholder is filled, the tool
//     itself runs the content-dependent validators (V1 deps closure,
//     V2 dry-build, patch pre-check) on the assembled plan; on
//     success it promotes Mutable.PartialChangePlan → ChangePlan
//     using the SAME factory + validators as emit_change_plan, so the
//     output shape is byte-identical regardless of which path the
//     planner chose.
//
// This tool is purely additive — emit_change_plan stays for the
// happy path. The planner skill teaches both modes; the planner
// picks based on the size of the plan it intends to produce.
type EmitPlanSkeleton struct {
	ReadOnly
	NonEvidenceTool
}

// emitPlanSkeletonChange is the wire shape for the skeleton's
// changes_meta[] entries. No NewContent / Patch fields — those land
// via subsequent emit_plan_change calls.
type emitPlanSkeletonChange struct {
	Path      string   `json:"path"`
	Kind      string   `json:"kind"`
	Rationale string   `json:"rationale"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// emitPlanSkeletonParams is the wire shape for the skeleton tool.
// Mirrors emitChangePlanParams except changes carries metadata only.
type emitPlanSkeletonParams struct {
	Request            string                    `json:"request"`
	Summary            string                    `json:"summary"`
	Changes            []emitPlanSkeletonChange  `json:"changes"`
	AcceptanceTests    []string                  `json:"acceptance_tests,omitempty"`
	VerificationProbes []types.VerificationProbe `json:"verification_probes,omitempty"`
}

// emitPlanSkeletonSchemaReminder is the structural-emit twin of
// emitChangePlanSchemaReminder. Re-primed on every payload-shape
// rejection so a streaming-truncation retry has the schema to
// rebuild against.
const emitPlanSkeletonSchemaReminder = "REQUIRED schema: {request: string (1-3 sentences restating the user's ask), " +
	"summary: string (3-10 sentences explaining what + why), " +
	"changes: array of {path: string, kind: \"create\"|\"modify\"|\"delete\"|\"patch\", " +
	"rationale: string (1-3 sentences), depends_on: optional []string of OTHER paths in this plan}, " +
	"acceptance_tests: optional []string, verification_probes: optional typed bounded probes}. " +
	"Do NOT include new_content or patch here — those land via emit_plan_change once per file."

func (t *EmitPlanSkeleton) Name() string { return "emit_plan_skeleton" }

func (t *EmitPlanSkeleton) Description() string {
	return "Emit the metadata layer of a multi-round ChangePlan: request + summary + per-file (path/kind/rationale/depends_on). " +
		"Use this when a plan would be too large to fit a single emit_change_plan call. After the skeleton is accepted, " +
		"call emit_plan_change once per non-delete file to fill in the body; the last emit_plan_change runs the full validators and finalizes the plan."
}

func (t *EmitPlanSkeleton) Parameters() json.RawMessage {
	return json.RawMessage(`{
	  "type": "object",
	  "additionalProperties": false,
	  "properties": {
	    "request": {"type": "string", "description": "1-3 sentences restating the user's code-change ask."},
	    "summary": {"type": "string", "description": "3-10 sentences describing what the plan does and why."},
	    "changes": {
	      "type": "array",
	      "minItems": 1,
	      "items": {
	        "type": "object",
	        "additionalProperties": false,
	        "properties": {
	          "path": {"type": "string", "description": "Repository-relative path of the file."},
	          "kind": {"type": "string", "enum": ["create", "modify", "delete", "patch"]},
	          "rationale": {"type": "string", "description": "1-3 sentences explaining WHY this file needs this change."},
	          "depends_on": {
	            "type": "array",
	            "items": {"type": "string"},
	            "description": "Optional ordering constraints — repo-relative paths of OTHER changes in this plan."
	          }
	        },
	        "required": ["path", "kind", "rationale"]
	      }
	    },
	    "acceptance_tests": {
	      "type": "array",
	      "items": {"type": "string"},
	      "description": "Optional list of test assertions the verify stage must cover."
	    },
	    "verification_probes": {
	      "type": "array",
	      "description": "Optional typed fallback checks for verify environments where the project runner is unavailable or unparseable. Initial support: language=python inline code.",
	      "items": {
	        "type": "object",
	        "additionalProperties": false,
	        "properties": {
	          "id": {"type": "string"},
	          "language": {"type": "string", "enum": ["python"]},
	          "working_dir": {"type": "string"},
	          "code": {"type": "string"},
	          "timeout_seconds": {"type": "integer", "minimum": 1, "maximum": 30},
	          "expected_stdout": {"type": "array", "items": {"type": "string"}}
	        },
	        "required": ["language", "code"]
	      }
	    }
	  },
	  "required": ["request", "summary", "changes"]
	}`)
}

func (t *EmitPlanSkeleton) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_skeleton requires a writable context",
			Timestamp: time.Now(),
		}, nil
	}

	// Layer-3 truncated payload guard. Same threshold as
	// emit_change_plan — skeleton payloads should be at least summary
	// + one path + one kind + one rationale = several hundred bytes
	// minimum. Below 80 → truncation.
	trimmed := strings.TrimSpace(string(params))
	if len(trimmed) < emitMinPayloadBytes || trimmed == "{}" || trimmed == "null" || trimmed == "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_skeleton rejected: payload was empty or truncated (got " + fmt.Sprintf("%d", len(trimmed)) + " bytes). " + emitPlanSkeletonSchemaReminder,
			Timestamp: time.Now(),
		}, nil
	}
	params = applyStructuredPayloadCompat(t.Name(), params, t.Parameters())

	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p emitPlanSkeletonParams
	if err := dec.Decode(&p); err != nil {
		return failStrictDecodeWithErrorMessage(t.Name(), time.Now(), err, nil, params, "emit_plan_skeleton rejected: ", ". "+emitPlanSkeletonSchemaReminder)
	}

	if strings.TrimSpace(p.Summary) == "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_skeleton rejected: summary is required and must be non-empty. " + emitPlanSkeletonSchemaReminder,
			Timestamp: time.Now(),
		}, nil
	}
	if len(p.Changes) == 0 {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_skeleton rejected: changes[] cannot be empty — at least one FileChange metadata entry is required. " + emitPlanSkeletonSchemaReminder,
			Timestamp: time.Now(),
		}, nil
	}

	// Convert to canonical FileChange shape with empty content
	// placeholders. emit_plan_change will fill these in.
	fcs := make([]types.FileChange, 0, len(p.Changes))
	for _, c := range p.Changes {
		var deps []string
		if len(c.DependsOn) > 0 {
			deps = make([]string, 0, len(c.DependsOn))
			for _, d := range c.DependsOn {
				deps = append(deps, strings.TrimSpace(d))
			}
		}
		fcs = append(fcs, types.FileChange{
			Path:      strings.TrimSpace(c.Path),
			Kind:      strings.TrimSpace(c.Kind),
			Rationale: strings.TrimSpace(c.Rationale),
			DependsOn: deps,
			// NewContent / Patch left empty — placeholder.
		})
	}

	// Content-independent validators run NOW so the planner sees
	// structural mistakes (dup paths, dangling deps, illegal kind,
	// wiring fan-in gap, summary lying about a path) before
	// committing to per-file content emits.
	if rej := validatePlanGraphIntegrity(fcs); rej != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_skeleton rejected: " + rej,
			Timestamp: time.Now(),
		}, nil
	}
	// Method M: typed-signal hard gate on task.scope=micro →
	// kind=patch (see validatePlanScopeKindAlignment for full
	// rationale). Skeleton path catches the mismatch before any
	// per-file emit_plan_change is wasted.
	if rej := validatePlanScopeKindAlignment(ctx, fcs); rej != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_skeleton rejected: " + rej,
			Timestamp: time.Now(),
		}, nil
	}
	if cycle := detectDepsCycle(fcs); cycle != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("emit_plan_skeleton rejected: depends_on cycle detected: %s", cycle),
			Timestamp: time.Now(),
		}, nil
	}
	if rej := validatePlanWiringClosure(fcs); rej != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_skeleton rejected: " + rej,
			Timestamp: time.Now(),
		}, nil
	}
	// Note: V4 summary fidelity is partially runnable here (path
	// side) but its import-side check needs new_content. We DEFER
	// the V4 check entirely to emit_plan_change's finalize hop —
	// running half here and half there would split a single
	// rejection across two messages and confuse the planner. The
	// skeleton's value is structural plan shape, not summary
	// fidelity.

	probes, rej := normalizeVerificationProbes(p.VerificationProbes)
	if rej != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_plan_skeleton rejected: " + rej,
			Timestamp: time.Now(),
		}, nil
	}

	// Install partial plan. The factory builds the canonical struct
	// with placeholder bodies; emit_plan_change fills them.
	// ResetChangePlan FIRST clears any stale completed-or-partial
	// plan from a prior dispatch (it nils both slots), then we
	// install the fresh skeleton — order matters because Reset wipes
	// PartialChangePlan too.
	plan := newChangePlanFromChanges(strings.TrimSpace(p.Request), strings.TrimSpace(p.Summary), fcs, p.AcceptanceTests, probes)
	ctx.Mutable.ResetChangePlan()
	ctx.Mutable.SetPartialChangePlan(plan)

	pendingFiles := countNonDeleteChanges(plan.Changes)
	summary := fmt.Sprintf(
		"[emit_plan_skeleton: id=%s changes=%d pending_bodies=%d acceptance_tests=%d verification_probes=%d]\n"+
			"skeleton recorded — call emit_plan_change once per non-delete change to fill new_content / patch. "+
			"The last emit_plan_change runs the full validators (V1 deps / V2 dry-build / V4 fidelity / patch pre-check) "+
			"and finalizes the plan.",
		plan.ID, len(plan.Changes), pendingFiles, len(plan.AcceptanceTests), len(plan.VerificationProbes))
	logging.Info("[emit_plan_skeleton] plan=%s changes=%d pending_bodies=%d", plan.ID, len(plan.Changes), pendingFiles)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		Timestamp: time.Now(),
	}, nil
}

// countNonDeleteChanges returns how many entries need a body via
// emit_plan_change. delete kinds need no content; create/modify/patch do.
func countNonDeleteChanges(changes []types.FileChange) int {
	n := 0
	for _, c := range changes {
		if strings.TrimSpace(c.Kind) != "delete" {
			n++
		}
	}
	return n
}
