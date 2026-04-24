package tool

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ApplyPatch is the apply-stage's structured write tool. B0 ships it
// as a STUB that always returns a clear "not yet implemented" error;
// the real unified-diff application + git-worktree commit lands in
// B2. The stub exists so:
//
//   1. The apply-stage agent (coder) has a wire-visible tool to
//      declare in its skill's ToolSuggestions — without it the agent
//      would have no legal emit path and the ReAct loop would wander.
//
//   2. The orchestrator's runApplyPhase can dispatch the coder stage
//      today and surface a clean fail-loud error rather than no-op
//      silently. Users who run `--mode=apply` before B2 see a clear
//      message instead of a mystery empty result.
//
// Red-line discipline (L3): Execute MUST NOT invoke ground.BuildContext
// or ground.GroundItem — apply is a write path, not a citation path.
// Enforced by write_mode_red_lines_test.go.
//
// Classified WriteCapable + NonEvidenceTool: IsWrite() returns true
// so future skill gating (Day 5+) can refuse this tool in read-only
// skills. NonEvidenceTool because the artifact is a code change, not
// a citable repo fact.
type ApplyPatch struct {
	WriteCapable
	NonEvidenceTool
}

// Name returns the tool's stable identifier.
func (t *ApplyPatch) Name() string { return "apply_patch" }

// Description is one sentence + the B0 stub disclaimer so operators
// reading tool logs see immediately that this is not yet functional.
func (t *ApplyPatch) Description() string {
	return "Apply a planned code change to the active git worktree. " +
		"B0 SKELETON STUB — returns 'not yet implemented'; real implementation ships in B2."
}

// Parameters mirrors the shape B2 will need so the schema is stable
// across B0→B2. Accepts a path + optional patch/content; Execute
// ignores everything and returns error.
func (t *ApplyPatch) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "path": {
      "type": "string",
      "description": "Repo-relative file path to modify (must be declared in plan.target_paths)."
    },
    "kind": {
      "type": "string",
      "enum": ["create", "modify", "delete", "patch"],
      "description": "Change type; matches ChangePlan.Changes[].Kind."
    },
    "new_content": {
      "type": "string",
      "description": "Full file body for kind=create|modify."
    },
    "patch": {
      "type": "string",
      "description": "Unified-diff payload for kind=patch."
    }
  },
  "required": ["path", "kind"]
}`)
}

// Execute is the B0 stub: always returns a clear error.
func (t *ApplyPatch) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	_ = ctx
	_ = params
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   false,
		Summary:   "[B0 skeleton] apply_patch not yet implemented — real patch application ships in B2. The apply stage is structurally wired but its write behavior is deliberately disabled in B0.",
		Timestamp: time.Now(),
	}, fmt.Errorf("apply_patch: B0 skeleton stub; implementation deferred to B2")
}
