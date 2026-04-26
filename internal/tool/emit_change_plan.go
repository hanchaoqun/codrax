package tool

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
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
	"new_content: string (full file body for create/modify), patch: string (unified diff for kind=patch), " +
	"rationale: string (1-3 sentences), depends_on: optional []string of OTHER paths in this plan}}. " +
	"OPTIONAL: acceptance_tests: array of strings. " +
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
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_change_plan rejected: payload was empty or truncated (got " + fmt.Sprintf("%d", len(trimmed)) + " bytes). " + emitChangePlanSchemaReminder,
			Timestamp: time.Now(),
		}, nil
	}

	// Strict decode — unknown fields fail loudly so a schema drift
	// surfaces during development, not as silent data loss. On decode
	// failure the schema reminder is appended so the LLM can recover
	// without re-deriving the field set from prose.
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p emitChangePlanParams
	if err := dec.Decode(&p); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("emit_change_plan rejected: invalid params: %v. ", err) + emitChangePlanSchemaReminder,
			Timestamp: time.Now(),
		}, err
	}

	if strings.TrimSpace(p.Summary) == "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_change_plan rejected: summary is required and must be non-empty. " + emitChangePlanSchemaReminder,
			Timestamp: time.Now(),
		}, nil
	}
	if len(p.Changes) == 0 {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_change_plan rejected: changes[] cannot be empty — at least one FileChange is required. " + emitChangePlanSchemaReminder,
			Timestamp: time.Now(),
		}, nil
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
	if rej := validatePlanGraphIntegrity(fcs); rej != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_change_plan rejected: " + rej,
			Timestamp: time.Now(),
		}, nil
	}

	// 3) Cycle detection. A cyclic DependsOn graph has no valid
	//    apply order — reject with the specific cycle path so the
	//    planner's retry (B1 fail-loud surface) can fix it.
	if cycle := detectDepsCycle(fcs); cycle != "" {
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
	if rej := validatePlanFullContent(ctx, p.Summary, fcs); rej != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_change_plan rejected: " + rej,
			Timestamp: time.Now(),
		}, nil
	}

	// Build the internal ChangePlan + populate target_paths from
	// the (already converted + already validated) changes slice.
	plan := newChangePlanFromChanges(strings.TrimSpace(p.Request), strings.TrimSpace(p.Summary), fcs, p.AcceptanceTests)

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
	pyBin, err := exec.LookPath("python3")
	if err != nil {
		if pyBin, err = exec.LookPath("python"); err != nil {
			logging.Debug("[emit_change_plan] V2 Python dry-build skipped: python binary not on PATH")
			return ""
		}
	}
	var pyChanges []string
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".py") {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
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
	args := append([]string{"-m", "py_compile"}, pyChanges...)
	cmd := exec.Command(pyBin, args...)
	cmd.Dir = scratch
	out, err := cmd.CombinedOutput()
	if err != nil {
		return formatDryBuildRejection("Python", "python3 -m py_compile "+strings.Join(pyChanges, " "), out, len(out))
	}
	logging.Debug("[emit_change_plan] V2 Python dry-build PASS for files: %v", pyChanges)
	return ""
}

// dryBuildNodeJS runs `node --check` on every changed .js / .mjs /
// .cjs file. node --check parses the file and reports SyntaxError
// without executing it. TypeScript files are skipped here because
// `tsc --noEmit` requires a tsconfig.json + tsc binary that varies
// per project; we leave TS to the project's own test runner.
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

	// node --check is per-file; concatenate diagnostics from every
	// failing file so the planner sees the full picture in one shot.
	sort.Strings(jsChanges)
	var combinedOut []byte
	failed := false
	for _, p := range jsChanges {
		cmd := exec.Command(nodeBin, "--check", p)
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
	logging.Debug("[emit_change_plan] V2 Node dry-build PASS for files: %v", jsChanges)
	return ""
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
	return fmt.Sprintf("V2 dry-build failed: `%s` returned non-zero exit (%s). "+
		"This means the new_content has compile / syntax / type errors. Fix the issues and re-emit. Full output:\n%s",
		cmdDescr, lang, out)
}

// ─────────────────────────────────────────────────────────────────────
// V5 lint
// ─────────────────────────────────────────────────────────────────────

// validatePlanLint catches "code-smell" / unused-symbol / dead-code /
// style problems that V2's syntax+vet pass cannot see. The Batch E
// quality audit surfaced two patterns this catches:
//   - robot-name's reset() forgetting to remove the old name from
//     `_used_names` (would surface as "unused-attribute" in some
//     linters; ruff catches the F841 unused-variable family)
//   - generic dead branches / unreachable returns / over-broad excepts
//
// Per-language fan-out symmetric to validatePlanDryBuild. Each helper
// inspects `changes`, skips when its toolchain is missing, and runs
// the language-specific linter on a hardlinked overlay so the real
// repo is never touched. The check is opt-out via yaml — operators
// who want strict-tests-only mode set `pipeline_lint_enabled: false`.
//
// Severity policy: only ERROR-class diagnostics reject (not WARNINGs
// or stylistic nits). Linting was the lowest-priority validator and
// must be resilient to noisy projects — over-rejection is worse than
// under-rejection here. Each language helper picks its severity
// filter accordingly.
func validatePlanLint(ctx *types.BusContext, changes []types.FileChange) string {
	if !LintEnabled() {
		return ""
	}
	if rej := lintPython(ctx, changes); rej != "" {
		return rej
	}
	if rej := lintGoFmt(ctx, changes); rej != "" {
		return rej
	}
	return ""
}

// lintPython runs `ruff check --select=E,F` on every changed .py
// file. The E (pycodestyle errors) + F (pyflakes — unused-vars,
// unused-imports, undefined-names) families are the highest-signal
// for catching LLM-generated dead code without flooding rejections
// on style preferences. Skips when ruff is not installed.
func lintPython(ctx *types.BusContext, changes []types.FileChange) string {
	ruffBin, err := exec.LookPath("ruff")
	if err != nil {
		logging.Debug("[emit_change_plan] V5 Python lint skipped: ruff binary not on PATH")
		return ""
	}
	var pyChanges []string
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".py") {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		// Strict rules apply only to NEW files (kind=create) — for
		// modify, the pre-existing file likely already had the same
		// patterns and rejecting them would create churn. New files
		// have no excuse for unused variables / dead imports.
		if c.Kind != "create" {
			continue
		}
		pyChanges = append(pyChanges, path)
	}
	if len(pyChanges) == 0 {
		return ""
	}
	scratch, cleanup, ok := stageOverlay(ctx, changes, "python-lint")
	if !ok {
		return ""
	}
	defer cleanup()

	sort.Strings(pyChanges)
	args := append([]string{"check", "--select=E,F", "--no-cache", "--output-format=concise"}, pyChanges...)
	cmd := exec.Command(ruffBin, args...)
	cmd.Dir = scratch
	out, err := cmd.CombinedOutput()
	if err != nil {
		return formatLintRejection("Python", "ruff check --select=E,F", out, len(out))
	}
	logging.Debug("[emit_change_plan] V5 Python lint PASS for files: %v", pyChanges)
	return ""
}

// lintGoFmt runs `gofmt -l` on every changed .go file. Files with
// formatting deviations come back on stdout (one path per line);
// non-empty output = rejection. We don't enforce other Go lint rules
// here because `go vet` already covers semantic checks in V2 and
// fancier checks (staticcheck / golangci-lint) would need the
// operator to install + configure them. gofmt is part of the Go
// distribution, no extra install.
func lintGoFmt(ctx *types.BusContext, changes []types.FileChange) string {
	gofmtBin, err := exec.LookPath("gofmt")
	if err != nil {
		logging.Debug("[emit_change_plan] V5 Go gofmt skipped: gofmt binary not on PATH")
		return ""
	}
	var goChanges []string
	for _, c := range changes {
		path := filepath.ToSlash(strings.TrimSpace(c.Path))
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if c.Kind == "patch" || c.Kind == "delete" {
			continue
		}
		// Only enforce gofmt on NEW files — same rationale as ruff
		// (don't churn existing files into a different style).
		if c.Kind != "create" {
			continue
		}
		goChanges = append(goChanges, path)
	}
	if len(goChanges) == 0 {
		return ""
	}
	scratch, cleanup, ok := stageOverlay(ctx, changes, "go-lint")
	if !ok {
		return ""
	}
	defer cleanup()

	sort.Strings(goChanges)
	args := append([]string{"-l"}, goChanges...)
	cmd := exec.Command(gofmtBin, args...)
	cmd.Dir = scratch
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "" {
		// gofmt -l: non-empty output = files needing format. Treat
		// as rejection so the planner re-emits with correct format.
		// gofmt itself rarely errors; combine stderr+stdout for
		// completeness.
		listing := strings.TrimSpace(string(out))
		if listing == "" {
			return ""
		}
		return formatLintRejection("Go", "gofmt -l <files>",
			[]byte("Files not gofmt-clean (format with `gofmt -s -w`):\n"+listing), len(out))
	}
	logging.Debug("[emit_change_plan] V5 Go gofmt PASS for files: %v", goChanges)
	return ""
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
