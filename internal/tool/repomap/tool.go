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
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
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
      "x-codrax-enum-style-alias": true,
      "x-codrax-enum-aliases": {"filemap": "file_map", "taskmap": "task_map", "callpath": "call_path", "editimpact": "edit_impact", "semanticsubgraph": "semantic_subgraph", "relationmap": "relation_map", "sourceinventory": "source_inventory", "implementors": "implementers"},
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
      "x-codrax-split-string-array": true,
      "description": "For source_inventory / relation_map views: scopes to inspect relative to the selected path/repository root, preserving model-provided order. If the context lists active sub-repos, set path to the chosen active sub-repo and keep scopes relative to that sub-repo."
    },
    "sources": {
      "type": "array",
      "items": {"type": "string"},
      "x-codrax-split-string-array": true,
      "description": "For relation_map view: model-chosen source symbols, files, or scopes to inspect. Use this after a source_inventory/task_map/file_map result surfaces a concrete source. Omit with query to let relation_map list matching source candidates. If path already names an active sub-repo, file/scope sources should be relative to that sub-repo."
    },
    "relation_kinds": {
      "type": "array",
      "items": {
        "type": "string",
        "enum": ["call", "calls", "called_by", "called-by", "import", "imports", "imported_by", "imported-by", "inheritance", "extends", "implements", "reference", "references", "type_usage"],
        "x-codrax-enum-style-alias": true
      },
      "x-codrax-split-string-array": true,
      "description": "For relation_map view: structural relation families to show. Omit for a compact mix of calls, imports, inheritance/implements, and references."
    },
    "roles": {
      "type": "array",
      "items": {
        "type": "string",
        "enum": ["function", "method", "type", "constant", "variable", "field", "package", "file", "config_file", "config_key", "route", "import_path", "literal_value"],
        "x-codrax-enum-style-alias": true
      },
      "x-codrax-split-string-array": true,
      "description": "For source_inventory view: semantic candidate roles to list. Do not use file as the only primary role; file-path or file-family discovery belongs to list_files with recursive=true plus include/file_type filters. Use file only with attribute_roles for bounded file-local member expansion, or alongside semantic roles such as config_file when the file itself is a typed source-inventory member. Omit to use the current typed request roles."
    },
    "attribute_roles": {
      "type": "array",
      "items": {
        "type": "string",
        "enum": ["function", "method", "type", "constant", "variable", "field", "package", "file", "config_file", "config_key", "route", "import_path", "literal_value"],
        "x-codrax-enum-style-alias": true
      },
      "x-codrax-split-string-array": true,
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
      "description": "Maximum number of files/items to include (default varies by view and scales down on small repositories; an explicit value always wins)"
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

func (t *RepoMapV2) Execute(ctx *ctypes.BusContext, params json.RawMessage) (ctypes.ToolResult, error) {
	// Tool-boundary repair layer, mirroring trace_query's wiring:
	// the shared schema-driven compat pass runs first (deterministic
	// enum aliases, split string arrays, scalar carriers), then a
	// strict decode so an unknown field becomes a typed same-dispatch
	// retry hint instead of being silently dropped.
	params = tool.ApplyStructuredPayloadCompat(t.Name(), params, t.Parameters())
	p, err := decodeRepoMapParams(params)
	if err != nil {
		return tool.FailStrictDecodeWithError(t.Name(), time.Now(), err, nil, params)
	}

	if p.View == "" {
		p.View = "overview"
	}
	// A view still unknown after alias normalization is a typed
	// rejection naming the valid enum — never a silent fallback to
	// overview at the tool layer. (render keeps its own overview
	// fallback for trusted programmatic callers; model-driven calls
	// are gated here, before any filesystem or index work.)
	if !repoMapViewSupported(p.View) {
		return repoMapUnknownViewRejection(t.Name(), p.View), nil
	}
	if p.View == "source_inventory" {
		if summary, ok := repoMapSourceInventoryParamPreflight(p); !ok {
			return ctypes.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   summary,
				Timestamp: time.Now(),
			}, nil
		}
	}

	// L1 negative-knowledge gate (R3 second-axis enforcement):
	// requested target_file / entry_point already shown absent in
	// the current repository. Refuse with the generic per-class
	// reason (no internal pipeline terminology, no fixture-fitted
	// examples).
	if ctx != nil {
		if p.TargetFile != "" {
			if denial, denied := ctx.TypedDenials.FirstPathDenial(p.TargetFile); denied {
				return ctypes.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   denial.HumanRefusalReason("repo_map"),
					Timestamp: time.Now(),
				}, nil
			}
		}
		if p.EntryPoint != "" {
			if denial, denied := ctx.TypedDenials.FirstSymbolDenial(p.EntryPoint); denied {
				return ctypes.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   denial.HumanRefusalReason("repo_map"),
					Timestamp: time.Now(),
				}, nil
			}
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
	if p.View == "source_inventory" {
		if query, source := repoMapSourceInventoryDefaultQuery(ctx, p.Query); query != p.Query {
			p.Query = query
			paramAdvisories = append(paramAdvisories, "query: unset -> "+source)
		}
	}

	// Build, load, or reuse the graph. The scope check above runs
	// before cache selection and file discovery, so refused paths never
	// reach index.CacheDir / index.ScanFiles. Route through the context
	// facade so an analyzer prewarm stored on Mutable.SearchGraph or a
	// single-repo MultiGraph resident can satisfy this tool call without
	// triggering a second full scan.
	graph, err := GraphFromBusContextOrLoad(ctx, repoRoot, p.Query)
	if err != nil {
		// Operator detail goes to the log; the model-facing summary
		// stays neutral — raw scanner/filesystem error text is
		// internal detail the model cannot act on.
		logging.Warning("repo_map: index build failed for %q: %v", repoRoot, err)
		return ctypes.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   repoMapIndexBuildRefusal(p.Path),
			Timestamp: time.Now(),
		}, nil
	}

	if p.View == "source_inventory" {
		if ctx != nil && ctx.Mutable != nil && graph != nil {
			ctx.Mutable.SetSearchGraph(graph)
		}
		budgetAdvisories := repoMapSourceInventoryApplyBudgetGuard(&p, len(graph.FileIndex))
		includeAttributes := true
		if p.IncludeAttributes != nil {
			includeAttributes = *p.IncludeAttributes
		}
		includeCounts := true
		if p.IncludeCounts != nil {
			includeCounts = *p.IncludeCounts
		}
		scopes := sourceInventoryScopesForRepoMapParams(p, repoRoot, graph)
		graph, restoreGraph, auxProjectionApplied := sourceInventoryGraphWithAuxiliaryProjection(ctx, repoRoot, graph, p, scopes)
		if restoreGraph != nil {
			defer ctx.Mutable.SetSearchGraph(restoreGraph)
		}
		provenance := []string(nil)
		if auxProjectionApplied {
			provenance = append(provenance, "repo_lens:auxiliary_projection")
		}
		lensQuery := ctypes.SourceInventoryLensQuery{
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
			Provenance:        provenance,
			RepoFileCount:     len(graph.FileIndex),
		}
		narrowingAdvisories := []string(nil)
		observation, guardedQuery, guardAdvisories, guarded := repoMapSourceInventoryMaybeBoundBroadNavigationLens(ctx, graph, lensQuery)
		if guarded {
			lensQuery = guardedQuery
			narrowingAdvisories = append(narrowingAdvisories, guardAdvisories...)
		} else {
			observation = tool.PublishSourceInventoryObservationFromLens(ctx, lensQuery)
			observation, lensQuery, guardAdvisories = repoMapSourceInventoryMaybeNarrowNoRows(ctx, graph, observation, lensQuery)
			narrowingAdvisories = append(narrowingAdvisories, guardAdvisories...)
		}
		output := tool.RenderSourceInventoryObservationView(observation, lensQuery)
		output = prependRepoMapSourceInventoryFitAdvisory(ctx, p.Query, output)
		output = prependRepoMapParameterAdvisory(output, narrowingAdvisories)
		output = prependRepoMapBudgetGuardAdvisory(output, budgetAdvisories)
		output = prependRepoMapParameterAdvisory(output, paramAdvisories)
		output = appendRepoMapBudgetHint(graph, p.View, p.TopN, output)
		output = prependRepoMapIndexStatusBanner(graph, output)
		summary, ref := tool.StoreBlob(ctx, t.Name(), output)
		now := time.Now()
		observationCarrier := ctypes.CloneSourceInventoryObservation(observation)
		return ctypes.ToolResult{
			ToolName:        t.Name(),
			Success:         true,
			Summary:         summary,
			RawRef:          ref,
			Refinement:      repoMapSourceInventoryRefinement(observation, lensQuery),
			SourceInventory: &observationCarrier,
			Observations:    repoMapNavigationTypedObservation(p.View, ref, output, now),
			Timestamp:       now,
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
		DeprioritizeAuxiliary:   repoMapDeprioritizeAuxiliary(ctx, graph),
	}
	var viewProgress *repoMapScanProgress
	if repoMapViewProgressEnabled(graph, p.View) {
		viewProgress = newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanCacheHit, len(graph.FileIndex), 0)
		viewProgress.startPhase(ctypes.RepoMapScanPhaseViewRender, len(graph.FileIndex))
		viewParams.ViewProgress = func(step string, done, total int) {
			viewProgress.viewRendered(step, done, total)
		}
	}
	// relation_map keeps a typed side-channel next to the rendered prose:
	// the exact graph-backed rows the renderer printed are published as
	// observation rows on the ToolResult (trace_query's producer pattern),
	// so the navigation facts the explorer saw survive to later stages
	// without LLM re-emission or a gated per-stage graph re-probe. The
	// rendered TEXT is unchanged — RenderMarkdown over the same ViewData is
	// exactly what GenerateView produces — and every other view keeps the
	// untouched GenerateView call.
	var output string
	var relationRows []render.RelationMapRow
	viewData := render.GenerateViewData(graph, p.View, viewParams)
	if viewData != nil {
		relationRows = viewData.RelationRows
		output = render.RenderMarkdown(viewData)
	} else {
		output = render.GenerateView(graph, p.View, viewParams)
	}
	if viewProgress != nil {
		viewProgress.finish(true, nil)
	}
	output = prependRepoMapNavigationAdvisory(ctx, p.View, p.Query, output)
	output = prependRepoMapParameterAdvisory(output, paramAdvisories)
	output = appendRepoMapBudgetHint(graph, p.View, p.TopN, output)
	output = prependRepoMapIndexStatusBanner(graph, output)

	summary, ref := tool.StoreBlob(ctx, t.Name(), output)
	now := time.Now()
	observations := repoMapNavigationTypedObservation(p.View, ref, output, now)
	observations = append(observations, relationMapTypedObservations(relationRows, ref, output, now)...)
	return ctypes.ToolResult{
		ToolName:     t.Name(),
		Success:      true,
		Summary:      summary,
		RawRef:       ref,
		Refinement:   repoMapNavigationRefinement(graph, p, viewParams, viewData),
		Observations: observations,
		Timestamp:    now,
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
	if ctypes.IsTypedSourceEnumerationShape(rm) {
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

// decodeRepoMapParams strict-decodes tool params after the shared
// compat layer ran. DisallowUnknownFields turns a silently-dropped
// unknown field into a typed rejection upstream, the same posture as
// trace_query.
func decodeRepoMapParams(params json.RawMessage) (repoMapParams, error) {
	var p repoMapParams
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return repoMapParams{}, err
	}
	return p, nil
}

// repoMapSupportedViews mirrors the "view" enum in Parameters().
// TestRepoMapSupportedViewsMatchSchemaEnum pins the two lists equal so
// they cannot drift.
var repoMapSupportedViews = []string{
	"overview",
	"file_map",
	"task_map",
	"call_path",
	"edit_impact",
	"semantic_subgraph",
	"relation_map",
	"source_inventory",
	"implementers",
}

func repoMapViewSupported(view string) bool {
	for _, v := range repoMapSupportedViews {
		if view == v {
			return true
		}
	}
	return false
}

func repoMapSourceInventoryParamPreflight(p repoMapParams) (string, bool) {
	if len(p.Roles) != 1 || p.Roles[0] != ctypes.AnswerCandidateRoleFile {
		return "", true
	}
	if len(p.AttributeRoles) > 0 && !repoMapSourceInventoryBroadRootScope(p) {
		return "", true
	}
	return "repo_map refused: source_inventory with roles=[\"file\"] as the only primary role is a path-discovery request, not a semantic source-inventory lens. Use `list_files` with recursive=true plus include/file_type filters for file-path or file-family discovery; use source_inventory with semantic roles such as function, method, type, constant, variable, field, package, config_file, config_key, route, import_path, or literal_value when you need a member/count checklist.", false
}

const (
	sourceInventoryBroadRootBudgetFileThreshold = 500
	sourceInventoryBroadRootDefaultTopN         = 50
	sourceInventoryBroadRootMaxTopN             = 100
	sourceInventoryAuxProjectionMaxFiles        = 256
	sourceInventoryAuxProjectionMaxFileBytes    = 2 << 20
)

func sourceInventoryGraphWithAuxiliaryProjection(ctx *ctypes.BusContext, repoRoot string, graph *Graph, p repoMapParams, scopes []string) (*Graph, *Graph, bool) {
	if ctx == nil || ctx.Mutable == nil || graph == nil || !sourceInventoryShouldProjectAuxiliary(ctx, p, scopes) {
		return graph, nil, false
	}
	entries := sourceInventoryAuxiliaryProjectionEntries(repoRoot, graph, p, scopes)
	if len(entries) == 0 {
		return graph, nil, false
	}
	infos := index.ParseFiles(entries, repoRoot)
	if len(infos) == 0 {
		return graph, nil, false
	}
	seen := make(map[string]bool, len(graph.FileIndex)+len(infos))
	files := make([]*FileInfo, 0, len(graph.Files)+len(infos))
	for _, fi := range graph.Files {
		if fi == nil {
			continue
		}
		rel := strings.Trim(strings.ReplaceAll(fi.RelPath, `\`, `/`), "/")
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		files = append(files, fi)
	}
	for _, fi := range infos {
		if fi == nil {
			continue
		}
		rel := strings.Trim(strings.ReplaceAll(fi.RelPath, `\`, `/`), "/")
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		files = append(files, fi)
	}
	if len(files) == len(graph.Files) {
		return graph, nil, false
	}
	projected := index.BuildGraph(repoRoot, files)
	ctx.Mutable.SetSearchGraph(projected)
	return projected, graph, true
}

func sourceInventoryShouldProjectAuxiliary(ctx *ctypes.BusContext, p repoMapParams, scopes []string) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rootScoped := repoMapSourceInventoryBroadRootScope(p) || sourceInventoryScopesContainRoot(scopes)
	auxiliaryScoped := sourceInventoryScopesContainAuxiliarySourceClass(append(append([]string(nil), scopes...), p.Path))
	if !rootScoped && !auxiliaryScoped {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if tool.SourceInventoryHasExplicitAuxiliaryExclusion(rm) {
		return false
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		return true
	}
	if rm.SourceScopeProfile != nil && rm.SourceScopeProfile.AllowsAuxiliaryPrincipal() {
		return true
	}
	return ctypes.IsTypedSourceEnumerationShape(rm)
}

func sourceInventoryScopesContainAuxiliarySourceClass(scopes []string) bool {
	for _, raw := range scopes {
		scope := strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`), "/")
		if scope == "" || scope == "." {
			continue
		}
		if ctypes.SourcePathRoleIsAuxiliary(ctypes.ClassifySourcePathRole(scope)) {
			return true
		}
	}
	return false
}

const (
	repoMapSourceInventoryBroadNavigationFileThreshold = 500
	repoMapSourceInventoryBroadNavigationScanLimit     = 4096
	repoMapSourceInventoryBroadNavigationMaxRows       = 96
)

func repoMapSourceInventoryMaybeBoundBroadNavigationLens(ctx *ctypes.BusContext, graph *Graph, query ctypes.SourceInventoryLensQuery) (ctypes.SourceInventoryObservation, ctypes.SourceInventoryLensQuery, []string, bool) {
	if ctx == nil || ctx.Mutable == nil || graph == nil || len(graph.FileIndex) < repoMapSourceInventoryBroadNavigationFileThreshold {
		return ctypes.SourceInventoryObservation{}, query, nil, false
	}
	if !repoMapSourceInventoryQueryIsBroad(query) {
		return ctypes.SourceInventoryObservation{}, query, nil, false
	}
	roles := repoMapSourceInventoryEffectiveRoles(ctx, query)
	if len(roles) == 0 || !repoMapSourceInventoryOnlyNavigationRoles(roles) {
		return ctypes.SourceInventoryObservation{}, query, nil, false
	}
	topN := query.TopN
	if topN <= 0 {
		topN = rmtypes.DefaultTopN("source_inventory", rmtypes.RepoSizeTier(len(graph.FileIndex)))
	}
	if topN <= 0 {
		topN = 60
	}
	if topN > repoMapSourceInventoryBroadNavigationMaxRows {
		topN = repoMapSourceInventoryBroadNavigationMaxRows
	}
	guardedQuery := query
	guardedQuery.Roles = roles
	guardedQuery.TopN = topN
	guardedQuery.IncludeAttributes = false
	guardedQuery.Provenance = append(append([]string(nil), query.Provenance...),
		"repo_lens:broad_navigation_guard",
		"repo_lens:candidate_budget_truncated",
	)
	observation := repoMapSourceInventoryBroadNavigationObservation(graph, guardedQuery, topN)
	if !observation.IsActive() {
		observation = ctypes.SourceInventoryObservation{
			Active:       true,
			AdvisoryOnly: true,
			Complete:     false,
			Scopes:       repoMapSourceInventoryObservationScopes(guardedQuery),
			Provenance:   append([]string(nil), guardedQuery.Provenance...),
			Lens:         []string{"bounded_navigation_sample", "count"},
		}
	}
	observation = tool.AttachSourceInventorySourceClassUniverse(ctx, observation, guardedQuery)
	observation = tool.AttachSourceInventoryLensExecutionState(observation, guardedQuery)
	if current := ctx.Mutable.SourceInventoryObservation(); current.IsActive() {
		ctx.Mutable.SetSourceInventoryObservation(ctypes.MergeSourceInventoryObservation(current, observation))
	} else {
		ctx.Mutable.SetSourceInventoryObservation(observation)
	}
	return observation, guardedQuery, []string{
		"source_inventory_broad_navigation_guard: bounded navigation sample; rerun with narrower scope or use list_files for file enumeration",
	}, true
}

func repoMapSourceInventoryEffectiveRoles(ctx *ctypes.BusContext, query ctypes.SourceInventoryLensQuery) []ctypes.AnswerCandidateRole {
	if len(query.Roles) > 0 {
		return append([]ctypes.AnswerCandidateRole(nil), query.Roles...)
	}
	if ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.SourceInventoryProfile != nil &&
		ctx.AnalysisIR.RequestModel.SourceInventoryProfile.Active() {
		return ctx.AnalysisIR.RequestModel.SourceInventoryProfile.PrincipalTargetRoles()
	}
	return nil
}

func repoMapSourceInventoryOnlyNavigationRoles(roles []ctypes.AnswerCandidateRole) bool {
	if len(roles) == 0 {
		return false
	}
	for _, role := range roles {
		switch role {
		case ctypes.AnswerCandidateRoleFile,
			ctypes.AnswerCandidateRoleConfigFile,
			ctypes.AnswerCandidateRolePackage:
		default:
			return false
		}
	}
	return true
}

func repoMapSourceInventoryBroadNavigationObservation(graph *Graph, query ctypes.SourceInventoryLensQuery, topN int) ctypes.SourceInventoryObservation {
	scopes := repoMapSourceInventoryObservationScopes(query)
	roleOrder := repoMapSourceInventoryEffectiveRoles(nil, query)
	setByRole := map[ctypes.AnswerCandidateRole]*ctypes.SourceInventoryObservationSet{}
	seenByRole := map[ctypes.AnswerCandidateRole]map[string]bool{}
	add := func(role ctypes.AnswerCandidateRole, member ctypes.SourceInventoryObservationMember) {
		if setByRole[role] == nil {
			setByRole[role] = &ctypes.SourceInventoryObservationSet{Role: role, Complete: false}
		}
		if seenByRole[role] == nil {
			seenByRole[role] = map[string]bool{}
		}
		key := strings.TrimSpace(member.Key)
		if key == "" {
			key = strings.TrimSpace(member.File)
		}
		if key == "" || seenByRole[role][key] || len(setByRole[role].Members) >= topN {
			return
		}
		seenByRole[role][key] = true
		member.Role = role
		member.Provenance = []string{"repo_lens:broad_navigation_guard"}
		member.CoverageState = ctypes.SourceInventoryCoverageNeedsRead
		setByRole[role].Members = append(setByRole[role].Members, member)
		setByRole[role].Count = len(setByRole[role].Members)
	}
	scanned := 0
	for _, fi := range graph.Files {
		if fi == nil || strings.TrimSpace(fi.RelPath) == "" {
			continue
		}
		if scanned >= repoMapSourceInventoryBroadNavigationScanLimit || repoMapSourceInventoryAllSetsAtLimit(setByRole, roleOrder, topN) {
			break
		}
		scanned++
		rel := strings.Trim(strings.ReplaceAll(strings.TrimSpace(fi.RelPath), `\`, `/`), "/")
		if rel == "" || !repoMapSourceInventoryFileInAnyScope(rel, scopes) ||
			!repoMapSourceInventoryBroadNavigationMatchesQuery(rel, fi.Language, query.Query) {
			continue
		}
		for _, role := range roleOrder {
			switch role {
			case ctypes.AnswerCandidateRoleFile:
				add(role, ctypes.SourceInventoryObservationMember{
					Name:       rel,
					Key:        rel,
					SupportRef: rel,
					File:       rel,
					Language:   strings.TrimSpace(fi.Language),
				})
			case ctypes.AnswerCandidateRoleConfigFile:
				if ctypes.LooksLikeConfigFilePath(rel) {
					add(role, ctypes.SourceInventoryObservationMember{
						Name:       rel,
						Key:        rel,
						SupportRef: rel,
						File:       rel,
						Language:   strings.TrimSpace(fi.Language),
					})
				}
			case ctypes.AnswerCandidateRolePackage:
				if dir := repoMapSourceInventoryTopPackageScope(rel); dir != "" {
					add(role, ctypes.SourceInventoryObservationMember{
						Name:       dir,
						Key:        dir,
						SupportRef: dir,
						File:       dir,
						Language:   strings.TrimSpace(fi.Language),
					})
				}
			}
		}
	}
	sets := make([]ctypes.SourceInventoryObservationSet, 0, len(roleOrder))
	for _, role := range roleOrder {
		set := setByRole[role]
		if set == nil || len(set.Members) == 0 {
			continue
		}
		set.Count = len(set.Members)
		sets = append(sets, *set)
	}
	return ctypes.CloneSourceInventoryObservation(ctypes.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       scopes,
		Provenance:   append([]string(nil), query.Provenance...),
		Lens:         []string{"bounded_navigation_sample", "count"},
		Sets:         sets,
	})
}

func repoMapSourceInventoryObservationScopes(query ctypes.SourceInventoryLensQuery) []string {
	scopes := append([]string(nil), query.Scopes...)
	if len(scopes) == 0 {
		scopes = append(scopes, query.Path)
	}
	if len(scopes) == 0 {
		return []string{"."}
	}
	for i, scope := range scopes {
		scope = strings.Trim(strings.ReplaceAll(strings.TrimSpace(scope), `\`, `/`), "/")
		if scope == "" {
			scope = "."
		}
		scopes[i] = scope
	}
	return scopes
}

func repoMapSourceInventoryAllSetsAtLimit(sets map[ctypes.AnswerCandidateRole]*ctypes.SourceInventoryObservationSet, roles []ctypes.AnswerCandidateRole, topN int) bool {
	if len(roles) == 0 || topN <= 0 {
		return false
	}
	for _, role := range roles {
		set := sets[role]
		if set == nil || len(set.Members) < topN {
			return false
		}
	}
	return true
}

func repoMapSourceInventoryFileInAnyScope(file string, scopes []string) bool {
	file = strings.Trim(strings.ReplaceAll(strings.TrimSpace(file), `\`, `/`), "/")
	if file == "" {
		return false
	}
	for _, scope := range scopes {
		scope = strings.Trim(strings.ReplaceAll(strings.TrimSpace(scope), `\`, `/`), "/")
		if scope == "" || scope == "." || file == scope || strings.HasPrefix(file, scope+"/") {
			return true
		}
	}
	return false
}

func repoMapSourceInventoryBroadNavigationMatchesQuery(file, language, rawQuery string) bool {
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return true
	}
	haystack := strings.ToLower(file + " " + filepath.Base(file) + " " + strings.TrimSpace(language))
	for _, token := range strings.Fields(strings.ToLower(rawQuery)) {
		token = strings.Trim(token, "`'\"()[]{}.,;:")
		if token == "" {
			continue
		}
		if strings.Contains(haystack, token) {
			return true
		}
	}
	return false
}

func repoMapSourceInventoryTopPackageScope(file string) string {
	file = strings.Trim(strings.ReplaceAll(strings.TrimSpace(file), `\`, `/`), "/")
	if file == "" || !strings.Contains(file, "/") {
		return ""
	}
	return strings.Split(file, "/")[0]
}

func repoMapSourceInventoryMaybeNarrowNoRows(ctx *ctypes.BusContext, graph *Graph, observation ctypes.SourceInventoryObservation, query ctypes.SourceInventoryLensQuery) (ctypes.SourceInventoryObservation, ctypes.SourceInventoryLensQuery, []string) {
	if ctx == nil || ctx.AnalysisIR == nil || graph == nil {
		return observation, query, nil
	}
	if repoMapSourceInventoryObservationMemberCount(observation) > 0 {
		return observation, query, nil
	}
	if !repoMapSourceInventoryQueryIsBroad(query) {
		return observation, query, nil
	}
	scopes := repoMapSourceInventoryRequiredFileScopes(ctx, graph, query.Path)
	if len(scopes) == 0 {
		return observation, query, nil
	}
	narrowQuery := query
	narrowQuery.Scopes = scopes
	narrowQuery.Offset = 0
	narrowQuery.Cursor = ""
	narrowQuery.Provenance = append(append([]string(nil), query.Provenance...), "repo_lens:auto_narrow_required_files")
	narrowed := tool.PublishSourceInventoryObservationFromLens(ctx, narrowQuery)
	advisories := []string{"source_inventory_narrowing: broad no-row lens -> typed required_files/common_scope"}
	if repoMapSourceInventoryObservationMemberCount(narrowed) > 0 {
		return narrowed, narrowQuery, advisories
	}
	if strings.TrimSpace(query.Query) == "" {
		return narrowed, narrowQuery, advisories
	}
	relaxedQuery := narrowQuery
	relaxedQuery.Query = ""
	relaxedQuery.Provenance = append(append([]string(nil), query.Provenance...), "repo_lens:auto_narrow_required_files", "repo_lens:auto_narrow_query_relaxed")
	relaxed := tool.PublishSourceInventoryObservationFromLens(ctx, relaxedQuery)
	if repoMapSourceInventoryObservationMemberCount(relaxed) > 0 {
		advisories = append(advisories, "source_inventory_narrowing_query: relaxed broad query inside typed required_files/common_scope")
		return relaxed, relaxedQuery, advisories
	}
	return narrowed, narrowQuery, advisories
}

func repoMapSourceInventoryQueryIsBroad(query ctypes.SourceInventoryLensQuery) bool {
	scopes := append([]string(nil), query.Scopes...)
	if len(scopes) == 0 {
		scopes = append(scopes, query.Path)
	}
	if len(scopes) == 0 {
		return true
	}
	for _, raw := range scopes {
		scope := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
		if scope == "" || scope == "." || scope == "./" || scope == "/" {
			return true
		}
	}
	return false
}

func repoMapSourceInventoryRequiredFileScopes(ctx *ctypes.BusContext, graph *Graph, queryPath string) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	files := ctypes.BoundedSourceEnumerationScopeFiles(rm, ctx.AnalysisIR.EvidencePlan.RequiredFiles, ctx.RepoRoot)
	if len(files) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		scope := repoMapSourceInventoryResolveScope(graph, queryPath, raw)
		if scope == "" || scope == "." || seen[scope] {
			return
		}
		seen[scope] = true
		out = append(out, scope)
	}
	if common := ctypes.BoundedSourceEnumerationCommonScope(files); common != "" {
		add(common)
	}
	const maxFileScopes = 8
	for _, file := range files {
		if len(out) >= maxFileScopes+1 {
			break
		}
		add(file)
		dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(file)))
		if dir == "." {
			dir = ""
		}
		dir = strings.Trim(dir, "/")
		if dir != "" {
			add(dir)
		}
	}
	return out
}

func repoMapSourceInventoryResolveScope(graph *Graph, queryPath, raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	if raw == "" {
		return ""
	}
	base := strings.TrimSpace(strings.ReplaceAll(queryPath, `\`, `/`))
	if base != "" && base != "." && base != "./" && base != "/" {
		trimmed := strings.TrimPrefix(raw, strings.Trim(base, "/")+"/")
		if trimmed != raw {
			raw = trimmed
		}
	}
	raw = strings.Trim(raw, "/")
	if raw == "" || raw == "." {
		return ""
	}
	if graph != nil {
		if _, ok := graph.FileIndex[raw]; ok {
			return raw
		}
		for file := range graph.FileIndex {
			file = strings.Trim(file, "/")
			if file == raw || strings.HasPrefix(file, raw+"/") {
				return raw
			}
		}
	}
	return raw
}

func repoMapSourceInventoryObservationMemberCount(observation ctypes.SourceInventoryObservation) int {
	total := 0
	for _, set := range observation.Sets {
		total += len(set.Members)
	}
	return total
}

func repoMapSourceInventoryDefaultQuery(ctx *ctypes.BusContext, raw string) (string, string) {
	if strings.TrimSpace(raw) != "" || ctx == nil || ctx.AnalysisIR == nil {
		return raw, ""
	}
	profile := ctx.AnalysisIR.RequestModel.SourceInventoryProfile
	if profile == nil || !profile.Active() {
		return raw, ""
	}
	parts := make([]string, 0, len(profile.SourceQuotes))
	seen := map[string]bool{}
	usedQuotes := false
	usedEntities := false
	add := func(item string) {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			return
		}
		seen[item] = true
		parts = append(parts, item)
	}
	for _, quote := range profile.SourceQuotes {
		before := len(parts)
		add(quote)
		usedQuotes = usedQuotes || len(parts) > before
	}
	for _, entity := range ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities {
		before := len(parts)
		add(entity)
		usedEntities = usedEntities || len(parts) > before
	}
	if len(parts) == 0 {
		policy := ctypes.CompileRepoMapNavigationPolicy(ctx.AnalysisIR.RequestModel, &ctx.AnalysisIR.AnswerContract, ctx.ExploreLanePlan)
		if query := strings.TrimSpace(strings.Join(policy.QueryTerms, " ")); query != "" {
			return query, "navigation_policy.query_terms"
		}
		return raw, ""
	}
	switch {
	case usedQuotes && usedEntities:
		return strings.Join(parts, " "), "source_inventory_profile.source_quotes+analyzer_hints.entities"
	case usedEntities:
		return strings.Join(parts, " "), "analyzer_hints.entities"
	default:
		return strings.Join(parts, " "), "source_inventory_profile.source_quotes"
	}
}

func sourceInventoryScopesContainRoot(scopes []string) bool {
	if len(scopes) == 0 {
		return true
	}
	for _, scope := range scopes {
		if repoMapParamPathIsRoot(scope) {
			return true
		}
	}
	return false
}

func sourceInventoryAuxiliaryProjectionEntries(repoRoot string, graph *Graph, p repoMapParams, scopes []string) []index.FileEntry {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" || graph == nil {
		return nil
	}
	seen := make(map[string]bool, len(graph.FileIndex))
	for rel := range graph.FileIndex {
		seen[strings.Trim(strings.ReplaceAll(rel, `\`, `/`), "/")] = true
	}
	tracked, _ := tool.SourceInventoryTrackedSourceFilesForLens(repoRoot, ctypes.SourceInventoryLensQuery{
		Path:   p.Path,
		Scopes: append([]string(nil), scopes...),
	})
	entries := make([]index.FileEntry, 0, len(tracked))
	for _, file := range tracked {
		rel := strings.Trim(strings.ReplaceAll(file.Path, `\`, `/`), "/")
		if rel == "" || seen[rel] || !ctypes.SourcePathRoleIsAuxiliary(file.Role) {
			continue
		}
		if tool.IsWindowsReservedDevicePath(filepath.Base(rel)) {
			continue
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		info, statErr := os.Stat(abs)
		if statErr != nil || info.IsDir() || info.Size() > sourceInventoryAuxProjectionMaxFileBytes {
			continue
		}
		lang := strings.TrimSpace(file.Language)
		if lang == "" || !rmtypes.IsSupportedReadLanguage(lang) {
			continue
		}
		entries = append(entries, index.FileEntry{
			RelPath:  rel,
			AbsPath:  abs,
			Language: lang,
			Size:     info.Size(),
		})
		seen[rel] = true
		if len(entries) >= sourceInventoryAuxProjectionMaxFiles {
			break
		}
	}
	return entries
}

func repoMapSourceInventoryApplyBudgetGuard(p *repoMapParams, fileCount int) []string {
	if p == nil || fileCount < sourceInventoryBroadRootBudgetFileThreshold || !repoMapSourceInventoryBroadRootScope(*p) {
		return nil
	}
	var advisories []string
	if p.TopN <= 0 {
		p.TopN = sourceInventoryBroadRootDefaultTopN
		advisories = append(advisories, fmt.Sprintf("top_n: unset -> %d for broad root source_inventory over %d indexed files", p.TopN, fileCount))
	} else if p.TopN > sourceInventoryBroadRootMaxTopN {
		original := p.TopN
		p.TopN = sourceInventoryBroadRootMaxTopN
		advisories = append(advisories, fmt.Sprintf("top_n: %d -> %d for broad root source_inventory over %d indexed files", original, p.TopN, fileCount))
	}
	includeAttributes := p.IncludeAttributes == nil || *p.IncludeAttributes
	if includeAttributes && len(p.AttributeRoles) > 0 {
		p.AttributeRoles = nil
		v := false
		p.IncludeAttributes = &v
		advisories = append(advisories, "attribute_roles ignored and include_attributes disabled for broad root source_inventory; rerun with explicit narrow scope/scopes for row-local attributes")
	} else if includeAttributes {
		v := false
		p.IncludeAttributes = &v
		advisories = append(advisories, "include_attributes: true/default -> false for broad root source_inventory without explicit attribute_roles")
	}
	return advisories
}

func repoMapSourceInventoryBroadRootScope(p repoMapParams) bool {
	if !repoMapParamPathIsRoot(p.Path) {
		return false
	}
	if strings.TrimSpace(p.Scope) != "" {
		return repoMapParamPathIsRoot(p.Scope)
	}
	if len(p.Scopes) == 0 {
		return true
	}
	for _, scope := range p.Scopes {
		if repoMapParamPathIsRoot(scope) {
			return true
		}
	}
	return false
}

func repoMapParamPathIsRoot(raw string) bool {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	trimmed = strings.Trim(trimmed, "/")
	return trimmed == "" || trimmed == "."
}

func prependRepoMapBudgetGuardAdvisory(output string, advisories []string) string {
	if len(advisories) == 0 {
		return output
	}
	var b strings.Builder
	b.WriteString("## Source Inventory Budget Guard\n\n")
	b.WriteString("The source_inventory request targets the repository root on a large indexed graph, so repo_map normalized broad defaults before running the lens. Use explicit narrower `scope` / `scopes` or row-local `attribute_roles` when wider detail is required.\n")
	for _, advisory := range advisories {
		if strings.TrimSpace(advisory) == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(advisory)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(output)
	return b.String()
}

func repoMapSourceInventoryRefinement(observation ctypes.SourceInventoryObservation, query ctypes.SourceInventoryLensQuery) *ctypes.ToolRefinementHint {
	if !observation.IsActive() {
		return nil
	}
	candidateBudgetTruncated := sourceInventoryProvenanceContains(observation.Provenance, "repo_lens:candidate_budget_truncated")
	if observation.Execution != nil && observation.Execution.CandidateBudgetTruncated {
		candidateBudgetTruncated = true
	}
	nextCursor := ""
	pageIncomplete := false
	if observation.Page != nil {
		nextCursor = strings.TrimSpace(observation.Page.NextCursor)
		pageIncomplete = !observation.Page.Complete && nextCursor != ""
	}
	if !candidateBudgetTruncated && !pageIncomplete {
		return nil
	}
	reasonCode := "source_inventory_page_incomplete"
	if candidateBudgetTruncated {
		reasonCode = "source_inventory_candidate_budget_truncated"
	}
	hint := ctypes.ToolRefinementHint{
		ReasonCode:               reasonCode,
		CandidateBudgetTruncated: candidateBudgetTruncated,
		NextCursor:               nextCursor,
		TopSourceClasses:         repoMapSourceInventoryTopSourceClasses(observation.SourceClasses),
		PreferredNextTool:        "repo_map",
		PreferredParams: map[string]string{
			"view":               "source_inventory",
			"include_attributes": "false",
		},
	}
	if path := strings.TrimSpace(query.Path); path != "" && !repoMapParamPathIsRoot(path) {
		hint.PreferredParams["path"] = path
	}
	if roles := repoMapSourceInventoryRolesParam(query.Roles); roles != "" {
		hint.PreferredParams["roles"] = roles
	}
	if scopes := repoMapSourceInventoryScopesParam(query.Scopes); scopes != "" {
		if strings.Contains(scopes, ",") {
			hint.PreferredParams["scopes"] = scopes
		} else {
			hint.PreferredParams["scope"] = scopes
		}
	}
	if candidateBudgetTruncated && repoMapSourceInventoryQueryIsBroad(query) {
		hint.RequiredFields = []string{"scope"}
		delete(hint.PreferredParams, "scope")
		delete(hint.PreferredParams, "scopes")
	} else if nextCursor != "" {
		hint.PreferredParams["cursor"] = nextCursor
	}
	if query.TopN > 0 {
		hint.PreferredParams["top_n"] = fmt.Sprintf("%d", query.TopN)
	}
	return &hint
}

func sourceInventoryProvenanceContains(provenance []string, want string) bool {
	for _, value := range provenance {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func repoMapSourceInventoryTopSourceClasses(counts []ctypes.SourceInventorySourceClassCount) []ctypes.SourcePathRole {
	out := make([]ctypes.SourcePathRole, 0, len(counts))
	seen := map[ctypes.SourcePathRole]bool{}
	for _, count := range counts {
		if count.Role == "" || count.Count <= 0 || seen[count.Role] {
			continue
		}
		seen[count.Role] = true
		out = append(out, count.Role)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func repoMapSourceInventoryRolesParam(roles []ctypes.AnswerCandidateRole) string {
	parts := make([]string, 0, len(roles))
	seen := map[ctypes.AnswerCandidateRole]bool{}
	for _, role := range roles {
		if role == "" || seen[role] {
			continue
		}
		seen[role] = true
		parts = append(parts, string(role))
	}
	return strings.Join(parts, ",")
}

func repoMapSourceInventoryScopesParam(scopes []string) string {
	parts := make([]string, 0, len(scopes))
	seen := map[string]bool{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || repoMapParamPathIsRoot(scope) || seen[scope] {
			continue
		}
		seen[scope] = true
		parts = append(parts, scope)
	}
	return strings.Join(parts, ",")
}

// repoMapUnknownViewRejection is the typed refusal for a view value
// that survived deterministic alias normalization without matching the
// enum. It names every valid view so the retry can correct within the
// same dispatch.
func repoMapUnknownViewRejection(toolName, view string) ctypes.ToolResult {
	allowed := strings.Join(repoMapSupportedViews, ", ")
	return ctypes.ToolResult{
		ToolName: toolName,
		Success:  false,
		Summary: fmt.Sprintf(
			"repo_map does not support view %q. Valid views: %s. Re-call repo_map with one of these views, or omit view to get the default overview.",
			view, allowed),
		Repair: &ctypes.ToolRepair{
			Code:   "tool_param_invalid_enum_value",
			Fields: []string{"view"},
			Hint:   "Set view to one of the listed valid values. Common style drift (different casing or separators) is normalized automatically; anything else must match the enum exactly.",
			Metadata: map[string]string{
				"field":   "view",
				"value":   view,
				"allowed": allowed,
			},
		},
		Timestamp: time.Now(),
	}
}

// repoMapIndexBuildRefusal is the model-facing summary for an index
// build failure. The underlying error stays in the operator log only.
func repoMapIndexBuildRefusal(requestedPath string) string {
	return fmt.Sprintf(
		"repo_map could not build an index for path %q: the scan found no parseable source files or could not read the directory. Verify the directory with `list_files`, or pass a narrower repo-relative directory under the repository root.",
		repoMapDisplayPath(requestedPath))
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
		logging.Warning("repo_map: preflight stat failed for %q: %v", resolvedRoot, err)
		return fmt.Sprintf(
			"repo_map cannot use path %q before indexing because the path could not be inspected. Choose an existing repo-relative directory, or verify the path with `list_files` first.",
			display,
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

	// Version pre-check on the manifest header alone: after a schema
	// or extractor bump the loader would reject the cache anyway, so
	// skip the full repo hash pass and announce the rebuild up front.
	if reason := index.PeekCacheCompatibility(cacheDir); reason != index.CacheRejectNone {
		dir := rejectedCacheRebuildDir(cacheDir, reason)
		progress := newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanFull, len(entries), len(entries))
		return fullScanAfterRejectedCache(repoRoot, dir, entries, query, progress, reason)
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
	loaded := index.LoadFileInfosResult(cacheDir, func(loaded, total, chunksLoaded, chunksTotal int, current string) {
		progress.cacheLoaded(loaded, total, chunksLoaded, chunksTotal, current)
	})
	cached := loaded.Files
	if cached == nil {
		reason := loaded.RejectReason
		cacheDir = rejectedCacheRebuildDir(cacheDir, reason)
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
		return fullScanAfterRejectedCache(repoRoot, cacheDir, entries, query, progress, reason)
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
	graph.Metadata.IndexStatus = RepoMapIndexStatus{
		Source:         IndexSourceCacheHit,
		Freshness:      IndexFreshnessFresh,
		CacheWrittenAt: loaded.WrittenAt,
		RepoHead:       loaded.RepoHead,
		SchemaVersion:  loaded.SchemaVersion,
	}
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
	loaded := index.LoadFileInfosResult(cacheDir, func(loaded, total, chunksLoaded, chunksTotal int, current string) {
		progress.cacheLoaded(loaded, total, chunksLoaded, chunksTotal, current)
	})
	cached := loaded.Files
	if cached == nil {
		reason := loaded.RejectReason
		cacheDir = rejectedCacheRebuildDir(cacheDir, reason)
		if progress != nil {
			progress.mode = ctypes.RepoMapScanFull
			progress.changedFiles = len(entries)
		}
		return fullScanAfterRejectedCache(repoRoot, cacheDir, entries, query, progress, reason)
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
	graph.Metadata.IndexStatus = RepoMapIndexStatus{
		Source:         IndexSourceIncremental,
		Freshness:      IndexFreshnessFresh,
		CacheWrittenAt: loaded.WrittenAt,
		RepoHead:       loaded.RepoHead,
		SchemaVersion:  loaded.SchemaVersion,
	}
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
	graph.Metadata.IndexStatus = RepoMapIndexStatus{
		Source:    IndexSourceFullScan,
		Freshness: IndexFreshnessFresh,
	}

	// Save cache (non-blocking — errors are tolerable). cacheDir is
	// empty when a newer-schema cache must not be overwritten: the
	// scan stays in-memory only.
	if cacheDir != "" {
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
	}

	return graph, nil
}

// fullScanAfterRejectedCache is fullScan plus the rebuild-reason
// record: the banner tells the model WHY this call paid a full scan.
func fullScanAfterRejectedCache(repoRoot, cacheDir string, entries []FileEntry, query string, progress *repoMapScanProgress, reason index.CacheRejectReason) (*Graph, error) {
	g, err := fullScan(repoRoot, cacheDir, entries, query, progress)
	if g != nil && reason != index.CacheRejectNone && reason != index.CacheRejectMissing {
		g.Metadata.IndexStatus.RebuildReasons = []string{string(reason)}
	}
	return g, err
}

// rejectedCacheRebuildDir maps a cache rejection onto the rebuild
// posture: logs the typed reason (a bare nil used to be
// indistinguishable from corruption), and for a cache written by a
// NEWER binary returns "" so the fallback full scan stays in-memory
// and never downgrades the on-disk cache (mixed-version deployments
// would otherwise ping-pong full rescans; architecture.md §14.7).
func rejectedCacheRebuildDir(cacheDir string, reason index.CacheRejectReason) string {
	switch reason {
	case index.CacheRejectNone, index.CacheRejectMissing:
		return cacheDir
	case index.CacheRejectSchemaNewer:
		logging.Warning("repo_map: cache written by a newer codrax; scanning in-memory without overwriting (%s)", reason)
		return ""
	case index.CacheRejectSchemaVersion, index.CacheRejectExtractorVersion:
		logging.Info("repo_map: old cache schema is incompatible; rebuilding index (%s)", reason)
		return cacheDir
	default:
		logging.Warning("repo_map: cache rejected (%s); rebuilding index", reason)
		return cacheDir
	}
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

// prependRepoMapIndexStatusBanner prefixes the view output with a
// short (≤5 line) observation of how this graph was obtained, so the
// reading model can judge how much weight the map deserves. Wording
// rules: zero-value status renders "unknown" (never fabricates
// full_scan); fresh + zero-fallback output carries NO warning lines
// (anti-noise); the verify reminder reuses the existing contract
// vocabulary and appears only when the map is reused/unknown or
// fallback-parsed files exist. The banner is tool-result text — an
// allowed model-facing surface — and is navigation-level metadata,
// never citable evidence.
func prependRepoMapIndexStatusBanner(graph *Graph, output string) string {
	if graph == nil {
		return output
	}
	st := graph.Metadata.IndexStatus
	source := st.Source
	if source == "" {
		source = "unknown"
	}
	var b strings.Builder
	// Head line uses the bracketed k=v banner shape the observation
	// ledger already parses generically (toolResultBanners), so the
	// index status survives as a typed observation with zero new
	// ledger plumbing.
	fmt.Fprintf(&b, "[Repo Map Index: source=%s files=%d symbols=%d relations=%d",
		source, graph.Metadata.FileCount, graph.Metadata.SymbolCount, graph.Metadata.RelationCount)
	if graph.Metadata.FallbackFileCount > 0 {
		fmt.Fprintf(&b, " fallback_files=%d", graph.Metadata.FallbackFileCount)
	}
	if len(st.RebuildReasons) > 0 {
		fmt.Fprintf(&b, " rebuilt=%s", strings.Join(st.RebuildReasons, ","))
	}
	if st.CacheWrittenAt != "" {
		fmt.Fprintf(&b, " cache_written=%s", st.CacheWrittenAt)
	} else if !graph.Metadata.ScanTime.IsZero() {
		fmt.Fprintf(&b, " scanned=%s", graph.Metadata.ScanTime.Format(time.RFC3339))
	}
	b.WriteString("]\n")
	if len(st.RebuildReasons) > 0 {
		b.WriteString("- note: the previous index cache was incompatible and this call rebuilt it from a full scan\n")
	}
	if graph.Metadata.FallbackFileCount > 0 {
		fmt.Fprintf(&b, "- parse fallback: %d files; edges from fallback-parsed files may be incomplete\n",
			graph.Metadata.FallbackFileCount)
	}
	if graph.Metadata.FallbackFileCount > 0 || st.Freshness != IndexFreshnessFresh {
		b.WriteString("- verify selected files with read_file or targeted grep before citing implementation behavior\n")
	}
	b.WriteString("\n")
	return b.String() + output
}

// repoMapDeprioritizeAuxiliary decides whether ranked views softly
// downrank test/fixture/example/doc files for this call. Inputs are
// PRECISE typed signals only: the repo size tier (tiny/small repos
// are where auxiliary files crowd out the implementation) and the
// analyzer's typed SourceScopeProfile. A nil context, nil IR or nil
// profile leaves downranking OFF — when the typed intent lane is
// absent we never guess from prose (no-keyword-matching red line),
// and the rank layer's existing unconditional test multiplier keeps
// baseline noise control.
func repoMapDeprioritizeAuxiliary(ctx *ctypes.BusContext, graph *Graph) bool {
	if ctx == nil || ctx.AnalysisIR == nil || graph == nil {
		return false
	}
	profile := ctx.AnalysisIR.RequestModel.SourceScopeProfile
	if profile == nil || profile.AllowsAuxiliaryPrincipal() {
		return false
	}
	tier := GraphSizeTier(graph)
	return tier <= SizeTierSmall
}

// appendRepoMapBudgetHint adds a short narrowing note at the output
// tail for large repository scopes, teaching multiple narrow queries
// over broader single expansions (the CodeGraph-validated guidance
// shape). Suppressed when the relation_map broad-fallback advisory
// already rendered its own narrowing instruction (our deterministic
// marker, not prose matching), and entirely absent for tiny/small/
// medium scopes (anti-noise). An explicit user top_n far above the
// tier default earns one extra soft line — the value is always
// honored, never clamped.
// repoMapNavigationCallBudget is the soft per-scope guidance for how
// many NARROW repo_map/source_inventory follow-ups typically cover
// discovery at each repository size — the second half of the
// CodeGraph-validated budget mechanism (its A/B runs showed
// call-count steering text cuts broad-sweep fallbacks; the first
// half, size-tiered row budgets, landed with the P0.2 matrix).
// Tiny/small scopes get NO budget line (anti-noise ruling, design doc
// §5.4): one or two calls obviously suffice there and every extra
// hint line costs tokens on every call.
func repoMapNavigationCallBudget(tier SizeTier) int {
	switch tier {
	case SizeTierMedium:
		return 2
	case SizeTierLarge:
		return 3
	case SizeTierVeryLarge:
		return 4
	default:
		return 0
	}
}

// repoMapBudgetHintViews are the exploration-breadth views where the
// call-budget line earns its tokens. Targeted lookups (edit_impact,
// call_path, implementers) answer one precise question — budget prose
// there is noise.
var repoMapBudgetHintViews = map[string]bool{
	"overview":          true,
	"task_map":          true,
	"file_map":          true,
	"relation_map":      true,
	"source_inventory":  true,
	"semantic_subgraph": true,
}

func appendRepoMapBudgetHint(graph *Graph, view string, userTopN int, output string) string {
	if graph == nil {
		return output
	}
	tier := GraphSizeTier(graph)
	var lines []string
	// When relation_map's broad-fallback advisory already rendered its
	// own narrowing instruction (our deterministic marker), both the
	// budget line and the size hint stand down — at most one steering
	// voice per result.
	advisoryAlreadyRendered := strings.Contains(output, "mode=broad_fallback")
	if budget := repoMapNavigationCallBudget(tier); budget > 0 && repoMapBudgetHintViews[view] && !advisoryAlreadyRendered {
		lines = append(lines, fmt.Sprintf(
			"Navigation budget: ~%d narrow repo_map / source_inventory follow-ups (distinct scopes, sources, or roles) typically cover discovery in a repository of this size — prefer them over broad grep sweeps. Verifying selected files with read_file before citing stays required.",
			budget))
	}
	if tier >= SizeTierLarge && !advisoryAlreadyRendered {
		lines = append(lines,
			fmt.Sprintf("Repo Map Budget Hint: this repository scope is large (%d files indexed). Prefer follow-up relation_map / source_inventory calls with explicit sources or scopes over broader expansion.", len(graph.FileIndex)))
	}
	if tier >= SizeTierLarge {
		if def := DefaultTopN(view, tier); def > 0 && userTopN > 2*def {
			lines = append(lines,
				"Note: top_n is large for this repository scope; narrower follow-ups with scope/sources are usually cheaper.")
		}
	}
	if len(lines) == 0 {
		return output
	}
	return output + "\n\n" + strings.Join(lines, "\n") + "\n"
}
