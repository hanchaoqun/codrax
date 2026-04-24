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
	Path       string   `json:"path"`
	Kind       string   `json:"kind"`
	NewContent string   `json:"new_content,omitempty"`
	Patch      string   `json:"patch,omitempty"`
	Rationale  string   `json:"rationale"`
	DependsOn  []string `json:"depends_on,omitempty"`
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
          },
          "depends_on": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Repo-relative paths of OTHER changes in THIS plan that must apply before this one. Empty or omitted = no explicit ordering (declaration order wins). Use when a create or modify here relies on the output of another change (e.g. modify Y imports new-created X → Y depends_on [X]). Every entry MUST appear as another changes[].path; cycles are rejected."
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
	seenPaths := make(map[string]int, len(p.Changes))
	for i, c := range p.Changes {
		path := strings.TrimSpace(c.Path)
		if _, dup := seenPaths[path]; dup {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("emit_change_plan rejected: duplicate change for path %q (one-change-per-file constraint; combine into a single FileChange)", path),
				Timestamp: time.Now(),
			}, nil
		}
		seenPaths[path] = i
	}

	// 2) Unknown-depends_on check. Every DependsOn entry must
	//    appear as another change's Path. A dangling reference is a
	//    planner bug that would silently turn into an unapplied
	//    ordering constraint at apply time.
	for _, c := range p.Changes {
		path := strings.TrimSpace(c.Path)
		for _, dep := range c.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   fmt.Sprintf("emit_change_plan rejected: change %q has an empty depends_on entry", path),
					Timestamp: time.Now(),
				}, nil
			}
			if dep == path {
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   fmt.Sprintf("emit_change_plan rejected: change %q depends_on itself", path),
					Timestamp: time.Now(),
				}, nil
			}
			if _, ok := seenPaths[dep]; !ok {
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   fmt.Sprintf("emit_change_plan rejected: change %q depends_on %q but %q is not in changes[]", path, dep, dep),
					Timestamp: time.Now(),
				}, nil
			}
		}
	}

	// 3) Cycle detection. A cyclic DependsOn graph has no valid
	//    apply order — reject with the specific cycle path so the
	//    planner's retry (B1 fail-loud surface) can fix it.
	if cycle := detectDepsCycle(p.Changes); cycle != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("emit_change_plan rejected: depends_on cycle detected: %s", cycle),
			Timestamp: time.Now(),
		}, nil
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
	if GitAvailable() && strings.TrimSpace(ctx.RepoRoot) != "" {
		for _, c := range p.Changes {
			kind := strings.TrimSpace(c.Kind)
			if kind != "patch" {
				continue
			}
			if strings.TrimSpace(c.Patch) == "" {
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   fmt.Sprintf("emit_change_plan rejected: change %q has kind=patch but Patch is empty (unified-diff required)", strings.TrimSpace(c.Path)),
					Timestamp: time.Now(),
				}, nil
			}
			if err := CheckUnifiedDiff(ctx.RepoRoot, c.Patch); err != nil {
				msg := composePatchRejection(ctx.RepoRoot, strings.TrimSpace(c.Path), err.Error())
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   msg,
					Timestamp: time.Now(),
				}, nil
			}
		}
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
		Status:        types.PlanStatusPending,
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
		// Carry DependsOn through to the internal struct, trimming
		// each entry. Duplicates / unknowns / cycles are already
		// rejected above, so the slice here is clean.
		var deps []string
		if len(c.DependsOn) > 0 {
			deps = make([]string, 0, len(c.DependsOn))
			for _, d := range c.DependsOn {
				deps = append(deps, strings.TrimSpace(d))
			}
		}
		plan.Changes = append(plan.Changes, types.FileChange{
			Path:       path,
			Kind:       kind,
			NewContent: c.NewContent,
			Patch:      c.Patch,
			Rationale:  strings.TrimSpace(c.Rationale),
			DependsOn:  deps,
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
func detectDepsCycle(changes []emitChangePlanChange) string {
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
