package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hanchaoqun/design/internal/tool/patcher"
	"github.com/hanchaoqun/design/internal/types"
)

// ApplyPatch wraps the patcher package as a Tool. It replaces write_file
// with a safer, targeted API that supports four operation modes, preserves
// line endings, enforces workspace root safety, and returns a diff preview.
type ApplyPatch struct{}

type applyPatchParams struct {
	Path      string `json:"path"`
	Op        string `json:"op"`
	OldString string `json:"old_string,omitempty"`
	NewString string `json:"new_string,omitempty"`
	Content   string `json:"content,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

func (t *ApplyPatch) Name() string { return "apply_patch" }

func (t *ApplyPatch) Description() string {
	return "Apply a targeted change to a file. Modes: write (replace full content), edit (replace a unique exact-match substring), insert_before (insert new content immediately before an anchor substring), insert_after (insert new content immediately after an anchor substring). Preserves original line endings, confined to the workspace root, and returns a diff preview. Idempotent retries are detected and reported as already_applied."
}

func (t *ApplyPatch) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path":       {"type": "string",  "description": "File path relative to the workspace root"},
    "op":         {"type": "string",  "enum": ["write", "edit", "insert_before", "insert_after"], "description": "Operation to perform"},
    "old_string": {"type": "string",  "description": "Exact substring to match (required for edit/insert_before/insert_after). Must be unique in the file."},
    "new_string": {"type": "string",  "description": "Replacement content (edit) or inserted content (insert_before/insert_after)"},
    "content":    {"type": "string",  "description": "Full file content (required for write)"},
    "dry_run":    {"type": "boolean", "description": "If true, compute the result without writing to disk"}
  },
  "required": ["path", "op"]
}`)
}

func (t *ApplyPatch) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p applyPatchParams
	if err := json.Unmarshal(params, &p); err != nil {
		return failedResult(t.Name(), fmt.Sprintf("invalid params: %v", err)), err
	}

	root := ctx.RepoRoot
	if root == "" {
		root = "."
	}

	pp, err := patcher.New(root)
	if err != nil {
		return failedResult(t.Name(), err.Error()), err
	}

	req := patcher.Request{
		Path:      p.Path,
		Op:        patcher.Op(p.Op),
		OldString: p.OldString,
		NewString: p.NewString,
		Content:   p.Content,
		DryRun:    p.DryRun,
	}

	result, err := pp.Apply(context.Background(), req)
	if err != nil {
		return failedResult(t.Name(), err.Error()), err
	}

	success := result.Status == patcher.StatusApplied ||
		result.Status == patcher.StatusAlreadyApplied ||
		result.Status == patcher.StatusUnchanged

	summary := fmt.Sprintf("[%s] %s: %s", result.Status, p.Path, result.Message)
	if result.DiffPreview != "" {
		summary += "\n" + result.DiffPreview
	}

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   success,
		Summary:   summary,
		RawRef:    p.Path,
		Timestamp: time.Now(),
	}, nil
}

func failedResult(name, msg string) types.ToolResult {
	return types.ToolResult{
		ToolName:  name,
		Success:   false,
		Summary:   msg,
		Timestamp: time.Now(),
	}
}
