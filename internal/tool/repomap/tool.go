package repomap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/render"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

// forceReclaimMinParseableFiles is the parse-count threshold above
// which fullScan returns parse-phase memory to the OS before building
// the graph. Below it the reclaim is skipped: small repos and REPL
// turns gain nothing and should not pay the FreeOSMemory latency.
const forceReclaimMinParseableFiles = 2000

// RepoMapV2 is the tree-sitter-powered repo map tool.
type RepoMapV2 struct {
	tool.ReadOnly
	tool.NavigationTool
}

type repoMapParams struct {
	Path              string                       `json:"path"`
	View              string                       `json:"view,omitempty"`        // overview, file_map, task_map, call_path, edit_impact, semantic_subgraph, relation_map, source_inventory, implementers
	Query             string                       `json:"query,omitempty"`       // for task_map / relation_map / source_inventory ranking hint; for implementers, the interface/trait/protocol name
	TargetFile        string                       `json:"target_file,omitempty"` // for edit_impact
	EntryPoint        string                       `json:"entry_point,omitempty"` // for call_path
	Scope             string                       `json:"scope,omitempty"`       // for source_inventory
	Scopes            []string                     `json:"scopes,omitempty"`      // for source_inventory
	Sources           []string                     `json:"sources,omitempty"`     // for relation_map
	RelationKinds     []string                     `json:"relation_kinds,omitempty"`
	Roles             []ctypes.AnswerCandidateRole `json:"roles,omitempty"` // for source_inventory
	AttributeRoles    []ctypes.AnswerCandidateRole `json:"attribute_roles,omitempty"`
	IncludeAttributes *bool                        `json:"include_attributes,omitempty"`
	IncludeCounts     *bool                        `json:"include_counts,omitempty"`
	TopN              int                          `json:"top_n,omitempty"` // max items
	Offset            int                          `json:"offset,omitempty"`
	Cursor            string                       `json:"cursor,omitempty"`
}

func (t *RepoMapV2) Name() string { return "repo_map" }
func (t *RepoMapV2) Description() string {
	return "Navigation index for the repository — shows which files, directories/modules/packages, symbols, routes, and config surfaces exist and where they are. " +
		"It provides verified navigation and candidate-universe facts such as existing scopes, files, symbols, languages, and counts. " +
		"It is not a semantic source citation: read or grep the selected files when you need to cite code behavior, implementation details, or exact source text. " +
		"Supports views: overview (module summary), file_map (symbols per file), " +
		"task_map (relevant subgraph for a query), call_path (dependency chain from entry point), " +
		"edit_impact (what changes to a file would affect), " +
		"semantic_subgraph (topological summary: linear chains, hub files, articulation-point bridges), " +
		"relation_map (advisory structural edges around model-chosen sources/scopes: calls, imports, inheritance, implements, references), " +
		"source_inventory (typed repo lens for scoped members/symbols/routes/config attributes/counts), " +
		"implementers (exhaustive list of concrete types implementing an interface/trait/protocol named in query). " +
		"Use source_inventory for scoped candidate-universe/member checklists, including broad lists when the question asks for them; reserve attribute_roles for narrowed scopes or selected members because they attach row-local details. " +
		"For top-level architecture or module overviews, start with overview/file_map/task_map and then inspect selected files. " +
		"Use semantic_subgraph for topology questions about hubs/bridges/chains, edit_impact for changed-file impact, and call_path for a concrete entry-point path trace. " +
		"When multiple scopes or broad symbol roles are requested, source_inventory also renders a scope-grouped advisory view to help choose the next files to verify. " +
		"For relation questions, call relation_map with sources when you already chose a symbol/file, or scopes when you want structural edges inside directories/modules; narrow relation_kinds when you know the edge family. " +
		"When the context or a tool refusal lists multiple active sub-repos, pass one active sub-repo path as path, then use scopes/sources relative to that selected sub-repo; compare repos by querying each active sub-repo explicitly."
}

func (t *RepoMapV2) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Root path of the repository to analyze. Normal single-repo run: use \".\" or a repo-relative directory. When the context or a tool refusal lists multiple active sub-repos, use one active sub-repo path/prefix; parent-wide \".\" may be refused to avoid scanning unrelated repositories."
    },
    "view": {
      "type": "string",
      "enum": ["overview", "file_map", "task_map", "call_path", "edit_impact", "semantic_subgraph", "relation_map", "source_inventory", "implementers"],
      "description": "Type of map to generate (default: overview). Use source_inventory for scoped member inventories and member→attribute candidate checklists; broad member lists are supported when the question asks for them, while attribute_roles are best after narrowing. Use relation_map for advisory structural edges around selected sources/scopes."
    },
    "query": {
      "type": "string",
      "description": "Search query for task_map / relation_map source discovery, or the interface/trait/protocol name for the implementers view. Use a short whitespace-separated set of exact code surfaces such as identifiers, file/module/package names, routes, or config keys; do not paste a natural-language sentence. The query matches file names, symbol names, and docstrings."
    },
    "target_file": {
      "type": "string",
      "description": "File path for edit_impact view, relative to the selected path/repository root. If path already names an active sub-repo, do not repeat the sub-repo prefix here."
    },
    "entry_point": {
      "type": "string",
      "description": "File path for call_path view (defaults to main/index file), relative to the selected path/repository root. If path already names an active sub-repo, do not repeat the sub-repo prefix here."
    },
    "scope": {
      "type": "string",
      "description": "For source_inventory / relation_map views: one scope to inspect relative to the selected path/repository root. If the context lists active sub-repos, set path to the chosen active sub-repo and keep scope relative to that sub-repo."
    },
    "scopes": {
      "type": "array",
      "items": {"type": "string"},
      "description": "For source_inventory / relation_map views: scopes to inspect relative to the selected path/repository root, preserving model-provided order. If the context lists active sub-repos, set path to the chosen active sub-repo and keep scopes relative to that sub-repo."
    },
    "sources": {
      "type": "array",
      "items": {"type": "string"},
      "description": "For relation_map view: model-chosen source symbols, files, or scopes to inspect. Use this after a source_inventory/task_map/file_map result surfaces a concrete source. Omit with query to let relation_map list matching source candidates. If path already names an active sub-repo, file/scope sources should be relative to that sub-repo."
    },
    "relation_kinds": {
      "type": "array",
      "items": {
        "type": "string",
        "enum": ["call", "calls", "called_by", "called-by", "import", "imports", "imported_by", "imported-by", "inheritance", "extends", "implements", "reference", "references", "type_usage"]
      },
      "description": "For relation_map view: structural relation families to show. Omit for a compact mix of calls, imports, inheritance/implements, and references."
    },
    "roles": {
      "type": "array",
      "items": {
        "type": "string",
        "enum": ["function", "method", "type", "constant", "variable", "field", "package", "file", "config_file", "config_key", "route", "import_path", "literal_value"]
      },
      "description": "For source_inventory view: candidate roles to list. Omit to use the current typed request roles."
    },
    "attribute_roles": {
      "type": "array",
      "items": {
        "type": "string",
        "enum": ["function", "method", "type", "constant", "variable", "field", "package", "file", "config_file", "config_key", "route", "import_path", "literal_value"]
      },
      "description": "For source_inventory view: row-local candidate roles to attach under each listed member, e.g. functions/methods/types/routes/config keys under a directory/module/package/file. Use only for bounded scopes; omit for top-level architecture/module overview. These are verified candidate-universe rows; verify selected rows with read_file/grep before using them as semantic source citations."
    },
    "include_attributes": {
      "type": "boolean",
      "description": "For source_inventory view: include bounded row-local symbol/callable attributes. Use false for broad member/count passes; use true only after narrowing when attribute_roles or row-local details are needed (default true for compatibility). If a requested attribute expansion is too broad, the tool may return a member/count checklist with a narrowing instruction instead of expanding row-local attributes."
    },
    "include_counts": {
      "type": "boolean",
      "description": "For source_inventory view: include machine-checkable count == len(members) summaries (default true)"
    },
    "top_n": {
      "type": "integer",
      "description": "Maximum number of files/items to include (default varies by view)"
    },
    "offset": {
      "type": "integer",
      "description": "For source_inventory view: zero-based row offset for paging large checklist outputs"
    },
    "cursor": {
      "type": "string",
      "description": "For source_inventory view: cursor returned by a previous source_inventory result; currently a numeric row offset"
    }
  },
  "required": ["path"]
}`)
}

// findFirstDenialFromCtx returns the first TypedDenial in s matching
// `tok`. Mirrors internal/tool's findFirstDenial; duplicated rather
// than imported to avoid the tool→tool/repomap↔tool cycle.
func findFirstDenialFromCtx(s *ctypes.TypedDenialSet, tok string, pathShaped bool) ctypes.TypedDenial {
	if s == nil || tok == "" {
		return ctypes.TypedDenial{}
	}
	for _, d := range s.Denials {
		if pathShaped {
			if d.Token == tok {
				return d
			}
		} else {
			if d.Token == tok {
				return d
			}
		}
	}
	return ctypes.TypedDenial{}
}

func (t *RepoMapV2) Execute(ctx *ctypes.BusContext, params json.RawMessage) (ctypes.ToolResult, error) {
	var p repoMapParams
	if err := json.Unmarshal(params, &p); err != nil {
		return ctypes.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("invalid params: %v", err),
			Timestamp: time.Now(),
		}, err
	}

	if p.View == "" {
		p.View = "overview"
	}

	// L1 negative-knowledge gate (R3 second-axis enforcement):
	// requested target_file / entry_point already shown absent in
	// the current repository. Refuse with the generic per-class
	// reason (no internal pipeline terminology, no fixture-fitted
	// examples).
	if ctx != nil {
		if p.TargetFile != "" && ctx.TypedDenials.IsPathDenied(p.TargetFile) {
			denial := findFirstDenialFromCtx(&ctx.TypedDenials, p.TargetFile, true)
			return ctypes.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   denial.HumanRefusalReason("repo_map"),
				Timestamp: time.Now(),
			}, nil
		}
		if p.EntryPoint != "" && ctx.TypedDenials.IsSymbolDenied(p.EntryPoint) {
			denial := findFirstDenialFromCtx(&ctx.TypedDenials, p.EntryPoint, false)
			return ctypes.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   denial.HumanRefusalReason("repo_map"),
				Timestamp: time.Now(),
			}, nil
		}
	}

	// L1 active-set hard gate (Phase 1.L1, 2026-05-08): in multi-repo
	// posture, refuse parent-wide scans, refuse paths that fall inside
	// inactive sub-repos, and refuse ambiguous bare paths that match
	// multiple active sub-repos. Reached via the
	// types.MultiRepoActiveSetGater interface so the gate's
	// implementation in multigraph stays single-source.
	allowedRoot := ""
	if ctx != nil && ctx.RepoRoot != "" {
		allowedRoot = ctx.RepoRoot
	}
	var paramAdvisories []string
	if ctx != nil {
		if gater, ok := ctx.MultiGraph.(ctypes.MultiRepoActiveSetGater); ok && gater != nil {
			// repo_map points at directories, not single files —
			// pass fileExists=nil so the gate forces the LLM to
			// specify a sub-repo prefix on bare paths.
			gate := gater.ResolveActiveSetPath(ctx, t.Name(), p.Path, nil)
			if !gate.Allowed {
				return ctypes.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   gate.RefusalProse,
					Timestamp: time.Now(),
				}, nil
			}
			p.Path = gate.ResolvedPath
			if ctx.RepoRoot != "" && gate.SubRepoRootRel != "" && gate.SubRepoRootRel != "." {
				allowedRoot = filepath.Join(ctx.RepoRoot, gate.SubRepoRootRel)
			}
			paramAdvisories = normalizeRepoMapLensParamsForSelectedSubRepo(&p, gate.SubRepoRootRel)
		}
	}

	// Resolve LLM-supplied path against ctx.RepoRoot. The LLM treats
	// the repo root as its own CWD ("." = "the repo I'm investigating"),
	// but the codrax process CWD is wherever the user invoked the
	// binary. Without resolution, `repo_map(path=".")` scans the codrax
	// process CWD instead of the user's --repo target — the LLM then
	// faithfully cites content from the wrong tree (Q2 glamour-vs-codrax
	// regression). Empty / "." / relative paths are rooted at
	// ctx.RepoRoot; absolute paths are accepted only when they remain
	// inside the current repository scope. This is a hard safety guard:
	// a model must not be able to make repo_map scan a parent directory
	// or an unrelated absolute path, because repo_map can recurse through
	// very large repositories.
	repoRoot, err := resolveRepoMapRootScoped(p.Path, "", allowedRoot)
	if ctx != nil && ctx.RepoRoot != "" {
		repoRoot, err = resolveRepoMapRootScoped(p.Path, ctx.RepoRoot, allowedRoot)
	}
	if err != nil {
		return ctypes.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   repoMapScopeRefusal(p.Path),
			Timestamp: time.Now(),
		}, nil
	}
	if summary, ok := repoMapPreflightScanRoot(p.Path, repoRoot); !ok {
		return ctypes.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   summary,
			Timestamp: time.Now(),
		}, nil
	}

	// Build, load, or reuse the graph. The scope check above runs
	// before cache selection and file discovery, so refused paths never
	// reach index.CacheDir / index.ScanFiles. Route through the context
	// facade so an analyzer prewarm stored on Mutable.SearchGraph or a
	// single-repo MultiGraph resident can satisfy this tool call without
	// triggering a second full scan.
	graph, err := GraphFromBusContextOrLoad(ctx, repoRoot, p.Query)
	if err != nil {
		return ctypes.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("scan failed: %v", err),
			Timestamp: time.Now(),
		}, nil
	}

	if p.View == "source_inventory" {
		if ctx != nil && ctx.Mutable != nil && graph != nil {
			ctx.Mutable.SetSearchGraph(graph)
		}
		includeAttributes := true
		if p.IncludeAttributes != nil {
			includeAttributes = *p.IncludeAttributes
		}
		includeCounts := true
		if p.IncludeCounts != nil {
			includeCounts = *p.IncludeCounts
		}
		scopes := sourceInventoryScopesForRepoMapParams(p, repoRoot, graph)
		observation := tool.PublishSourceInventoryObservationFromLens(ctx, ctypes.SourceInventoryLensQuery{
			Path:              p.Path,
			Scopes:            scopes,
			Roles:             append([]ctypes.AnswerCandidateRole(nil), p.Roles...),
			AttributeRoles:    append([]ctypes.AnswerCandidateRole(nil), p.AttributeRoles...),
			IncludeAttributes: includeAttributes,
			IncludeCounts:     includeCounts,
			TopN:              p.TopN,
			Offset:            p.Offset,
			Cursor:            p.Cursor,
			Query:             p.Query,
		})
		output := tool.RenderSourceInventoryObservationView(observation, ctypes.SourceInventoryLensQuery{
			Path:              p.Path,
			Scopes:            scopes,
			Roles:             append([]ctypes.AnswerCandidateRole(nil), p.Roles...),
			AttributeRoles:    append([]ctypes.AnswerCandidateRole(nil), p.AttributeRoles...),
			IncludeAttributes: includeAttributes,
			IncludeCounts:     includeCounts,
			TopN:              p.TopN,
			Offset:            p.Offset,
			Cursor:            p.Cursor,
		})
		output = prependRepoMapSourceInventoryFitAdvisory(ctx, p.Query, output)
		output = prependRepoMapParameterAdvisory(output, paramAdvisories)
		summary, ref := tool.StoreBlob(ctx, t.Name(), output)
		return ctypes.ToolResult{
			ToolName:  t.Name(),
			Success:   true,
			Summary:   summary,
			RawRef:    ref,
			Timestamp: time.Now(),
		}, nil
	}

	// Generate the requested view
	viewParams := ViewParams{
		Query:                   p.Query,
		TargetFile:              p.TargetFile,
		EntryPoint:              p.EntryPoint,
		Sources:                 append([]string(nil), p.Sources...),
		Scopes:                  repoMapRelationScopes(p),
		RelationKinds:           append([]string(nil), p.RelationKinds...),
		TopN:                    p.TopN,
		ShowSourceInventoryHint: repoMapOverviewSourceInventoryHintEnabled(ctx, p.View),
	}
	var viewProgress *repoMapScanProgress
	if repoMapViewProgressEnabled(graph, p.View) {
		viewProgress = newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanCacheHit, len(graph.FileIndex), 0)
		viewProgress.startPhase(ctypes.RepoMapScanPhaseViewRender, len(graph.FileIndex))
		viewParams.ViewProgress = func(step string, done, total int) {
			viewProgress.viewRendered(step, done, total)
		}
	}
	output := render.GenerateView(graph, p.View, viewParams)
	if viewProgress != nil {
		viewProgress.finish(true, nil)
	}
	output = prependRepoMapNavigationAdvisory(ctx, p.View, p.Query, output)
	output = prependRepoMapParameterAdvisory(output, paramAdvisories)

	summary, ref := tool.StoreBlob(ctx, t.Name(), output)
	return ctypes.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

func repoMapViewProgressEnabled(graph *Graph, view string) bool {
	if graph == nil {
		return false
	}
	if view == "semantic_subgraph" {
		return true
	}
	return len(graph.FileIndex) >= 50000
}

func repoMapOverviewSourceInventoryHintEnabled(ctx *ctypes.BusContext, view string) bool {
	if view != "" && view != "overview" {
		return false
	}
	if ctx == nil {
		return true
	}
	return ctx.PipelineStage != ctypes.StageAnalyze && ctx.ActiveAgent != ctypes.AgentAnalyzer
}

func prependRepoMapNavigationAdvisory(ctx *ctypes.BusContext, view, query, output string) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return output
	}
	policy := ctypes.CompileRepoMapNavigationPolicy(
		ctx.AnalysisIR.RequestModel,
		&ctx.AnalysisIR.AnswerContract,
		ctx.ExploreLanePlan,
	)
	if !repoMapNavigationAdvisoryEnabled(ctx, view, query, policy) {
		return output
	}
	hint := policy.RenderMarkdownHint("Repo Map Next-Step Hint", "Soft route hints from the structured analysis result. Broad repo_map output is still valid, but these hints are not read obligations; the next call is usually cheaper and clearer when it follows this route.")
	if strings.TrimSpace(hint) == "" {
		return output
	}
	return hint + output
}

func repoMapNavigationAdvisoryEnabled(ctx *ctypes.BusContext, view, query string, policy ctypes.RepoMapNavigationPolicy) bool {
	if ctx == nil || ctx.PipelineStage == ctypes.StageAnalyze || ctx.ActiveAgent == ctypes.AgentAnalyzer {
		return false
	}
	view = strings.TrimSpace(view)
	if view == "" || view == "overview" {
		return true
	}
	if view != "task_map" {
		return false
	}
	if strings.TrimSpace(query) == "" {
		return true
	}
	return policy.HasRoute(ctypes.RepoMapNavigationRouteRelationMap) ||
		policy.HasRoute(ctypes.RepoMapNavigationRouteCallPath)
}

func prependRepoMapSourceInventoryFitAdvisory(ctx *ctypes.BusContext, query, output string) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.PipelineStage == ctypes.StageAnalyze || ctx.ActiveAgent == ctypes.AgentAnalyzer {
		return output
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return output
	}
	policy := ctypes.CompileRepoMapNavigationPolicy(rm, &ctx.AnalysisIR.AnswerContract, ctx.ExploreLanePlan)
	if !policy.HasRoute(ctypes.RepoMapNavigationRouteRelationMap) && !policy.HasRoute(ctypes.RepoMapNavigationRouteCallPath) {
		return output
	}
	var b strings.Builder
	b.WriteString("## Repo Map View Fit Hint\n\n")
	b.WriteString("`source_inventory` is a member/attribute checklist lens. For this structural relation / call-flow request, treat this result as optional orientation only; if it is sparse or off-target, switch back to `repo_map(view=\"task_map\"")
	if strings.TrimSpace(query) != "" {
		b.WriteString(", query=")
		b.WriteString(fmt.Sprintf("%q", strings.TrimSpace(query)))
	}
	b.WriteString(")` and then `repo_map(view=\"relation_map\", sources=[...], scopes=[...])` around selected source files or symbols. This is a navigation hint, not proof and not a read obligation.\n\n")
	return b.String() + output
}

func normalizeRepoMapLensParamsForSelectedSubRepo(p *repoMapParams, subRepoRootRel string) []string {
	if p == nil {
		return nil
	}
	var advisories []string
	strip := func(field, value string) string {
		stripped := stripSelectedSubRepoPrefix(value, subRepoRootRel)
		if selectedSubRepoPrefixWasPresent(value, subRepoRootRel) && stripped != strings.TrimPrefix(strings.TrimSpace(strings.ReplaceAll(value, `\`, `/`)), "./") {
			advisories = append(advisories, fmt.Sprintf("%s: `%s` → `%s`", field, strings.TrimSpace(value), stripped))
		}
		return stripped
	}
	p.TargetFile = strip("target_file", p.TargetFile)
	p.EntryPoint = strip("entry_point", p.EntryPoint)
	p.Scope = strip("scope", p.Scope)
	for i := range p.Scopes {
		p.Scopes[i] = strip(fmt.Sprintf("scopes[%d]", i), p.Scopes[i])
	}
	for i := range p.Sources {
		p.Sources[i] = strip(fmt.Sprintf("sources[%d]", i), p.Sources[i])
	}
	return advisories
}

func stripSelectedSubRepoPrefix(value, subRepoRootRel string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, `\`, `/`))
	value = strings.TrimPrefix(value, "./")
	if value == "" {
		return value
	}
	prefix := strings.Trim(strings.TrimSpace(strings.ReplaceAll(subRepoRootRel, `\`, `/`)), "/")
	if prefix == "" || prefix == "." {
		return value
	}
	if value == prefix {
		return "."
	}
	if strings.HasPrefix(value, prefix+"/") {
		return strings.TrimPrefix(value, prefix+"/")
	}
	return value
}

func selectedSubRepoPrefixWasPresent(value, subRepoRootRel string) bool {
	value = strings.TrimSpace(strings.ReplaceAll(value, `\`, `/`))
	value = strings.TrimPrefix(value, "./")
	prefix := strings.Trim(strings.TrimSpace(strings.ReplaceAll(subRepoRootRel, `\`, `/`)), "/")
	if value == "" || prefix == "" || prefix == "." {
		return false
	}
	return value == prefix || strings.HasPrefix(value, prefix+"/")
}

func prependRepoMapParameterAdvisory(output string, advisories []string) string {
	if len(advisories) == 0 {
		return output
	}
	const maxAdvisories = 6
	var b strings.Builder
	b.WriteString("## Parameter advisory\n\n")
	b.WriteString("The call selected an active sub-repo as `path`, so repo_map interpreted the following parameters relative to that selected sub-repo. Next time, keep these parameters relative instead of repeating the sub-repo prefix:\n")
	limit := len(advisories)
	if limit > maxAdvisories {
		limit = maxAdvisories
	}
	for i := 0; i < limit; i++ {
		b.WriteString("- ")
		b.WriteString(advisories[i])
		b.WriteString("\n")
	}
	if extra := len(advisories) - limit; extra > 0 {
		fmt.Fprintf(&b, "- ... %d more parameter(s) normalized the same way\n", extra)
	}
	b.WriteString("\n")
	b.WriteString(output)
	return b.String()
}

func repoMapRelationScopes(p repoMapParams) []string {
	scopes := append([]string(nil), p.Scopes...)
	if strings.TrimSpace(p.Scope) != "" {
		scopes = append([]string{strings.TrimSpace(p.Scope)}, scopes...)
	}
	return scopes
}

func sourceInventoryScopesForRepoMapParams(p repoMapParams, resolvedRoot string, graph *Graph) []string {
	scopes := append([]string(nil), p.Scopes...)
	if strings.TrimSpace(p.Scope) != "" {
		scopes = append([]string{strings.TrimSpace(p.Scope)}, scopes...)
	}
	if len(scopes) > 0 {
		return scopes
	}
	if scope := sourceInventoryDefaultScopeForRepoMapPath(p.Path, resolvedRoot, graph); scope != "" {
		return []string{scope}
	}
	return nil
}

func sourceInventoryDefaultScopeForRepoMapPath(requestedPath, resolvedRoot string, graph *Graph) string {
	requested := strings.TrimSpace(strings.ReplaceAll(requestedPath, `\`, `/`))
	if requested == "" || requested == "." || requested == "./" || requested == "/" {
		return ""
	}
	if graph != nil && strings.TrimSpace(graph.Root) != "" && strings.TrimSpace(resolvedRoot) != "" {
		if sameRepoMapRoot(graph.Root, resolvedRoot) {
			return "."
		}
		if rel, ok := repoMapRelPathWithinRoot(graph.Root, resolvedRoot); ok {
			return rel
		}
	}
	if filepath.IsAbs(requested) {
		return ""
	}
	return strings.Trim(strings.TrimSpace(requested), "/")
}

func repoMapRelPathWithinRoot(root, target string) (string, bool) {
	rootAbs, errRoot := canonicalRepoMapPath(root)
	targetAbs, errTarget := canonicalRepoMapPath(target)
	if errRoot != nil || errTarget != nil || !repoMapPathWithinRoot(rootAbs, targetAbs) {
		return "", false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." {
		return ".", true
	}
	if strings.HasPrefix(rel, "../") || rel == ".." || filepath.IsAbs(rel) {
		return "", false
	}
	return strings.Trim(rel, "/"), true
}

func resolveRepoMapRootScoped(requestedPath, sessionRoot, allowedRoot string) (string, error) {
	repoRoot := requestedPath
	if sessionRoot != "" {
		switch {
		case repoRoot == "" || repoRoot == ".":
			repoRoot = sessionRoot
		case !filepath.IsAbs(repoRoot):
			repoRoot = filepath.Join(sessionRoot, repoRoot)
		}
	}
	if repoRoot == "" {
		repoRoot = "."
	}
	if allowedRoot != "" {
		if err := ensureRepoMapRootWithin(repoRoot, allowedRoot); err != nil {
			return "", err
		}
	}
	return repoRoot, nil
}

func ensureRepoMapRootWithin(targetRoot, allowedRoot string) error {
	target, err := canonicalRepoMapPath(targetRoot)
	if err != nil {
		return err
	}
	allowed, err := canonicalRepoMapPath(allowedRoot)
	if err != nil {
		return err
	}
	if !repoMapPathWithinRoot(allowed, target) {
		return fmt.Errorf("repo_map path %q resolves outside allowed root %q", targetRoot, allowedRoot)
	}
	return nil
}

func canonicalRepoMapPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if evaluated, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		evalAbs, absErr := filepath.Abs(evaluated)
		if absErr == nil {
			abs = filepath.Clean(evalAbs)
		}
		return abs, nil
	}
	return canonicalRepoMapExistingPrefix(abs), nil
}

func canonicalRepoMapExistingPrefix(abs string) string {
	cur := abs
	var suffix []string
	for {
		if evaluated, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				evaluated = filepath.Join(evaluated, suffix[i])
			}
			evalAbs, err := filepath.Abs(evaluated)
			if err != nil {
				return filepath.Clean(evaluated)
			}
			return filepath.Clean(evalAbs)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		suffix = append(suffix, filepath.Base(cur))
		cur = parent
	}
}

func repoMapPathWithinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func repoMapScopeRefusal(requestedPath string) string {
	display := strings.TrimSpace(requestedPath)
	if display == "" {
		display = "."
	}
	return fmt.Sprintf(
		"repo_map refused: path %q resolves outside the current repository scope. "+
			"repo_map can only scan the configured workspace or active sub-repo; pass `.` "+
			"or a path under the repository root.",
		display,
	)
}

func repoMapPreflightScanRoot(requestedPath, resolvedRoot string) (string, bool) {
	info, err := os.Stat(resolvedRoot)
	if err == nil {
		if info.IsDir() {
			return "", true
		}
		return fmt.Sprintf(
			"repo_map path %q points to a file, but repo_map indexes directories/repository scopes. Use the containing directory as `path`, or use `read_file` / targeted `grep` for that file.",
			repoMapDisplayPath(requestedPath),
		), false
	}
	display := repoMapDisplayPath(requestedPath)
	switch {
	case os.IsNotExist(err):
		return fmt.Sprintf(
			"repo_map path %q was not found. Use a repo-relative directory that exists; if the path is uncertain, call `list_files` on the nearest known parent or `repo_map` with `path=\".\"` for an overview.",
			display,
		), false
	case os.IsPermission(err):
		return fmt.Sprintf(
			"repo_map cannot access path %q because of filesystem permissions. Choose an accessible repo-relative directory, or use a narrower file/search tool if you already know the target.",
			display,
		), false
	default:
		return fmt.Sprintf(
			"repo_map cannot use path %q before indexing: %v. Choose an existing repo-relative directory, or verify the path with `list_files` first.",
			display, err,
		), false
	}
}

func repoMapDisplayPath(requestedPath string) string {
	display := strings.TrimSpace(strings.ReplaceAll(requestedPath, `\`, `/`))
	if display == "" {
		return "."
	}
	return display
}

// BuildOrLoadGraph builds or loads a cached repo graph, ranks files
// by the given query, and returns the result. This is the trusted
// low-level API for already-authorized roots (CLI/eval harnesses,
// topology discovery, tests). Model-influenced paths must use
// BuildOrLoadGraphWithin or a context-aware facade so workspace scope
// is checked before cache selection and file discovery.
func BuildOrLoadGraph(repoRoot, query string) (*Graph, error) {
	return buildOrLoadGraph(repoRoot, query)
}

// BuildOrLoadGraphWithin is the scoped graph loader for any caller
// whose repoRoot may come from an agent/tool parameter. It rejects
// parent traversal, absolute paths outside allowedRoot, and symlink
// escapes before index.CacheDir or index.ScanFiles can touch the
// filesystem.
func BuildOrLoadGraphWithin(repoRoot, allowedRoot, query string) (*Graph, error) {
	resolved, err := resolveRepoMapRootScoped(repoRoot, "", allowedRoot)
	if err != nil {
		return nil, err
	}
	return buildOrLoadGraph(resolved, query)
}

func buildOrLoadGraph(repoRoot, query string) (*Graph, error) {
	// Cap the Go runtime to the scan CPU budget for the whole scan —
	// parse, change detection, graph build, ranking and GC — so it
	// leaves the reserved cores free for interactive processes.
	// Restored on return; no-op when repomap_scan_reserve_cpus is 0.
	defer index.ApplyScanGOMAXPROCS()()

	cacheDir := index.CacheDir(repoRoot)

	// Scan files
	inventoryProgress := newRepoMapScanProgress(repoRoot, "", 0, 0)
	inventoryProgress.startPhase(ctypes.RepoMapScanPhaseFileScan, 0)
	entries, err := index.ScanFilesWithProgress(repoRoot, func(files, parseable int, current string) {
		inventoryProgress.filesScanned(files, parseable, current)
	})
	if err != nil {
		inventoryProgress.finish(false, err)
		return nil, fmt.Errorf("file scan: %w", err)
	}
	notifyRepoMapScan(ctypes.RepoMapScanEvent{
		RepoRoot:       repoRoot,
		Phase:          ctypes.RepoMapScanPhaseFileScan,
		Progress:       true,
		OK:             true,
		TotalFiles:     len(entries),
		ParseableFiles: countParseableEntries(entries),
		ParsedFiles:    len(entries),
		ElapsedMs:      time.Since(inventoryProgress.start).Milliseconds(),
	})

	if len(entries) == 0 {
		inventoryProgress.finish(false, fmt.Errorf("no source files found"))
		return nil, fmt.Errorf("no source files found in %s", repoRoot)
	}

	// No cache at all → full scan
	if index.NeedsFullRescan(cacheDir) {
		logging.Info("repo_map: full scan (%d files, no cache)", len(entries))
		progress := newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanFull, len(entries), len(entries))
		return fullScan(repoRoot, cacheDir, entries, query, progress)
	}

	// Detect which files changed
	changeProgress := newRepoMapScanProgress(repoRoot, "", len(entries), 0)
	changeProgress.startPhase(ctypes.RepoMapScanPhaseChangeScan, 0)
	changed := index.ChangedFilesWithProgress(repoRoot, cacheDir, entries, func(done, total int) {
		changeProgress.parsed(done, total)
	})

	// Nothing changed → load from cache directly
	if len(changed) == 0 {
		logging.Info("repo_map: cache hit (%d files, 0 changed)", len(entries))
		progress := newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanCacheHit, len(entries), 0)
		progress.startPhase(ctypes.RepoMapScanPhaseCacheLoad, countParseableEntries(entries))
		return loadFromCache(repoRoot, cacheDir, entries, query, progress)
	}

	// >30% changed → full rescan is faster than incremental
	if float64(len(changed))/float64(len(entries)) > 0.3 {
		logging.Info("repo_map: full rescan (%d files, %d changed >30%%)", len(entries), len(changed))
		progress := newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanFullRescan, len(entries), len(changed))
		return fullScan(repoRoot, cacheDir, entries, query, progress)
	}

	// Incremental: reparse only changed files, merge with cached data
	logging.Info("repo_map: incremental (%d files, %d changed)", len(entries), len(changed))
	progress := newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanIncremental, len(entries), len(changed))
	return incrementalScan(repoRoot, cacheDir, entries, changed, query, progress)
}

func countParseableEntries(entries []FileEntry) int {
	n := 0
	for _, e := range entries {
		if e.Language != "" {
			n++
		}
	}
	return n
}

func loadFromCache(repoRoot, cacheDir string, entries []FileEntry, query string, progress *repoMapScanProgress) (*Graph, error) {
	cached := index.LoadFileInfosWithProgress(cacheDir, func(loaded, total, chunksLoaded, chunksTotal int, current string) {
		progress.cacheLoaded(loaded, total, chunksLoaded, chunksTotal, current)
	})
	if cached == nil {
		// Cache corrupt or missing JSON → fall back to full scan
		if len(entries) == 0 {
			var err error
			if progress != nil {
				progress.startPhase(ctypes.RepoMapScanPhaseFileScan, 0)
			}
			entries, err = index.ScanFilesWithProgress(repoRoot, func(files, parseable int, current string) {
				progress.filesScanned(files, parseable, current)
			})
			if err != nil {
				return nil, fmt.Errorf("file scan: %w", err)
			}
		}
		if progress != nil {
			progress.mode = ctypes.RepoMapScanFull
			progress.changedFiles = len(entries)
		}
		return fullScan(repoRoot, cacheDir, entries, query, progress)
	}

	if progress != nil {
		progress.parseableFiles = countParseableFileInfos(cached)
		progress.setPhase(ctypes.RepoMapScanPhaseBuildGraph)
	}
	graph := index.BuildGraphWithProgress(repoRoot, cached, buildGraphProgressFn(progress))
	if progress != nil {
		progress.setPhase(ctypes.RepoMapScanPhaseRank)
	}
	retrieve.RankGraph(graph, query)
	if progress != nil {
		progress.finish(true, nil)
	}
	return graph, nil
}

func countParseableFileInfos(files []*FileInfo) int {
	n := 0
	for _, fi := range files {
		if fi != nil && fi.Language != "" {
			n++
		}
	}
	return n
}

func incrementalScan(repoRoot, cacheDir string, entries []FileEntry, changed []string, query string, progress *repoMapScanProgress) (*Graph, error) {
	if progress != nil {
		progress.startPhase(ctypes.RepoMapScanPhaseCacheLoad, countParseableEntries(entries))
	}
	cached := index.LoadFileInfosWithProgress(cacheDir, func(loaded, total, chunksLoaded, chunksTotal int, current string) {
		progress.cacheLoaded(loaded, total, chunksLoaded, chunksTotal, current)
	})
	if cached == nil {
		if progress != nil {
			progress.mode = ctypes.RepoMapScanFull
			progress.changedFiles = len(entries)
		}
		return fullScan(repoRoot, cacheDir, entries, query, progress)
	}

	// Build lookup of cached files by path
	cachedByPath := make(map[string]*FileInfo, len(cached))
	for _, fi := range cached {
		cachedByPath[fi.RelPath] = fi
	}

	// Build set of changed files for fast lookup
	changedSet := make(map[string]bool, len(changed))
	for _, c := range changed {
		changedSet[c] = true
	}

	// Split entries into changed (need reparse) and unchanged (keep cached)
	var toReparse []FileEntry
	currentPaths := make(map[string]bool, len(entries))
	for _, e := range entries {
		currentPaths[e.RelPath] = true
		if changedSet[e.RelPath] {
			toReparse = append(toReparse, e)
		}
	}

	// Parse only changed files in parallel
	var parseable []FileEntry
	var unparseable []FileEntry
	for _, e := range toReparse {
		if e.Language != "" {
			parseable = append(parseable, e)
		} else {
			unparseable = append(unparseable, e)
		}
	}

	progress.startScan(len(parseable))
	var scanErr error
	defer func() {
		progress.finish(scanErr == nil, scanErr)
	}()

	var onProgress func(done, total int)
	if progress != nil {
		onProgress = progress.parsed
	}
	activeFile := repoMapActiveFileReporter(progress)
	freshInfos, _ := index.ParseFilesWithProgressSinkAndActive(parseable, repoRoot, onProgress, nil, activeFile)
	for _, e := range unparseable {
		freshInfos = append(freshInfos, index.BasicFileInfo(e))
	}

	// Build fresh lookup
	freshByPath := make(map[string]*FileInfo, len(freshInfos))
	for _, fi := range freshInfos {
		freshByPath[fi.RelPath] = fi
	}

	// Merge: for each current file, use fresh if changed, cached otherwise
	merged := make([]*FileInfo, 0, len(entries))
	for _, e := range entries {
		if fi, ok := freshByPath[e.RelPath]; ok {
			merged = append(merged, fi)
		} else if fi, ok := cachedByPath[e.RelPath]; ok {
			merged = append(merged, fi)
		}
		// else: file disappeared between scan and merge, skip
	}

	// Build graph, rank, save
	progress.setPhase(ctypes.RepoMapScanPhaseBuildGraph)
	graph := index.BuildGraphWithProgress(repoRoot, merged, buildGraphProgressFn(progress))
	progress.setPhase(ctypes.RepoMapScanPhaseRank)
	retrieve.RankGraph(graph, query)
	progress.setPhase(ctypes.RepoMapScanPhaseCacheWrite)
	if err := index.SaveCacheWithProgress(cacheDir, graph, func(file string, written, total int64) {
		progress.cacheWriteFile(file, written, total)
	}); err != nil {
		logging.Warning("repo_map: cache save failed: %v", err)
	}
	return graph, nil
}

func fullScan(repoRoot, cacheDir string, entries []FileEntry, query string, progress *repoMapScanProgress) (*Graph, error) {
	// Filter to only parseable files (with known language)
	var parseable []FileEntry
	var unparseable []FileEntry
	for _, e := range entries {
		if e.Language != "" {
			parseable = append(parseable, e)
		} else {
			unparseable = append(unparseable, e)
		}
	}

	// Resume: reuse parse results from an earlier scan that was
	// interrupted (e.g. OOM-killed) before it could install its cache
	// manifest. index.ResumableFileInfos content-hash verifies every
	// reused record, so the resulting graph is byte-identical to a
	// full re-parse. Nil when resume is disabled or no orphan chunks
	// exist — degrading cleanly to a from-scratch full scan.
	resumed := index.ResumableFileInfos(cacheDir, parseable)
	var toParse []FileEntry
	var reused []*FileInfo
	if len(resumed) > 0 {
		reused = make([]*FileInfo, 0, len(resumed))
		toParse = make([]FileEntry, 0, len(parseable)-len(resumed))
		for _, e := range parseable {
			if fi := resumed[e.RelPath]; fi != nil {
				reused = append(reused, fi)
			} else {
				toParse = append(toParse, e)
			}
		}
		logging.Info("repo_map: resuming interrupted scan — reused %d of %d source files, %d to re-parse",
			len(reused), len(parseable), len(toParse))
	} else {
		toParse = parseable
	}

	// Parse the remaining files in parallel.
	progress.startScan(len(toParse))
	var scanErr error
	defer func() {
		progress.finish(scanErr == nil, scanErr)
	}()

	var onProgress func(done, total int)
	if progress != nil {
		onProgress = progress.parsed
	}
	activeFile := repoMapActiveFileReporter(progress)
	var cacheWriter *index.FileInfoCacheWriter
	var cacheSink func(*FileInfo) error
	if cacheDir != "" {
		if w, err := index.NewFileInfoCacheWriter(cacheDir, repoRoot); err != nil {
			logging.Warning("repo_map: cache stream setup failed: %v", err)
		} else {
			cacheWriter = w
			cacheSink = w.Append
		}
	}

	// Stream reused records into the new scan's cache first, so an
	// interruption of THIS scan still leaves them for the next resume:
	// progress converges across repeated interruptions instead of
	// restarting from zero.
	var cacheStreamErr error
	if cacheWriter != nil {
		for _, fi := range reused {
			if cacheStreamErr = cacheWriter.Append(fi); cacheStreamErr != nil {
				break
			}
		}
	}

	parsedInfos, parseStreamErr := index.ParseFilesWithProgressSinkAndActive(toParse, repoRoot, onProgress, cacheSink, activeFile)
	if cacheStreamErr == nil {
		cacheStreamErr = parseStreamErr
	}

	fileInfos := make([]*FileInfo, 0, len(parseable)+len(unparseable))
	fileInfos = append(fileInfos, reused...)
	fileInfos = append(fileInfos, parsedInfos...)

	// Add unparseable files with basic metadata
	for _, e := range unparseable {
		fi := index.BasicFileInfo(e)
		fileInfos = append(fileInfos, fi)
		if cacheWriter != nil && cacheStreamErr == nil {
			cacheStreamErr = cacheWriter.Append(fi)
		}
	}
	if cacheWriter != nil {
		if cacheStreamErr != nil {
			cacheWriter.Abort()
			logging.Warning("repo_map: streaming fileinfo cache failed: %v", cacheStreamErr)
		} else if err := cacheWriter.Close(); err != nil {
			cacheStreamErr = err
			cacheWriter.Abort()
			logging.Warning("repo_map: streaming fileinfo cache finalize failed: %v", err)
		}
	}

	// Return parse-phase memory to the OS before the graph-build and
	// ranking phases allocate on top of it: the file bytes and
	// tree-sitter ASTs from parsing are all dead here. Gated on a
	// meaningful parse count so small repos / REPL turns pay nothing.
	if len(toParse) >= forceReclaimMinParseableFiles {
		debug.FreeOSMemory()
		logging.Info("repo_map: reclaimed parse-phase memory before graph build (%d source files parsed)", len(toParse))
	}

	// Build graph
	progress.setPhase(ctypes.RepoMapScanPhaseBuildGraph)
	graph := index.BuildGraphWithProgress(repoRoot, fileInfos, buildGraphProgressFn(progress))

	// Rank
	progress.setPhase(ctypes.RepoMapScanPhaseRank)
	retrieve.RankGraph(graph, query)

	// Save cache (non-blocking — errors are tolerable)
	progress.setPhase(ctypes.RepoMapScanPhaseCacheWrite)
	cacheProgress := func(file string, written, total int64) {
		progress.cacheWriteFile(file, written, total)
	}
	var saveErr error
	if cacheStreamErr == nil && cacheWriter != nil {
		saveErr = index.SaveCacheWithoutFileInfosWithProgress(cacheDir, graph, cacheProgress)
	} else {
		saveErr = index.SaveCacheWithProgress(cacheDir, graph, cacheProgress)
	}
	if saveErr != nil {
		logging.Warning("repo_map: cache save failed: %v", saveErr)
	}

	return graph, nil
}

func repoMapActiveFileReporter(progress *repoMapScanProgress) func(index.FileEntry) {
	if progress == nil {
		return nil
	}
	return func(entry index.FileEntry) {
		progress.activeFile(entry.RelPath)
	}
}

// ToolDescription returns a short summary for status messages.
func ToolDescription(view, query string) string {
	switch view {
	case "task_map":
		return fmt.Sprintf("Generating task map for %q", query)
	case "source_inventory":
		return "Preparing source inventory lens"
	case "implementers":
		if query != "" {
			return fmt.Sprintf("Listing implementers of %q", query)
		}
		return "Listing interface implementers"
	case "relation_map":
		return "Preparing relation map lens"
	case "edit_impact":
		return "Analyzing edit impact"
	case "call_path":
		return "Tracing call paths"
	case "file_map":
		return "Generating file map"
	case "semantic_subgraph":
		return "Summarizing semantic subgraphs"
	default:
		return "Generating repository overview"
	}
}

// buildGraphProgressFn adapts the scan progress reporter into the
// relation-progress callback BuildGraphWithProgress expects, nil-safe
// when no progress sink is attached.
func buildGraphProgressFn(progress *repoMapScanProgress) func(done, total int) {
	if progress == nil {
		return nil
	}
	return progress.buildGraphRelations
}
