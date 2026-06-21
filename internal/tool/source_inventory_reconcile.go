package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/tool/sourceinventory"
	"github.com/hanchaoqun/codrax/internal/types"
)

type sourceInventoryCandidate struct {
	member       string
	key          string
	supportRef   string
	note         string
	surfaceTerms []string
	role         types.AnswerCandidateRole
	exported     bool
	file         string
	line         int
	language     string
	attributes   []sourceInventoryCandidate
}

type sourceInventoryCandidateSet struct {
	role       types.AnswerCandidateRole
	candidates []sourceInventoryCandidate
	total      int
	complete   bool
	truncated  bool
}

const sourceInventorySourceClassUniverseMaxFiles = 200000

type sourceInventoryPackageBucket struct {
	dir       string
	files     int
	symbols   int
	languages map[string]int
	packages  map[string]int
}

type sourceInventoryGraphSymbolIndex struct {
	graph  *repotypes.Graph
	all    []*repotypes.Symbol
	byFile map[string][]*repotypes.Symbol
	byDir  map[string][]*repotypes.Symbol
}

type sourceInventoryQueryFilter struct {
	Tokens    []string
	Languages map[string]bool
}

type sourceInventoryScopeFilter struct {
	ctx                          *types.BusContext
	repositoryWideTypedQueryLane bool
	sourceScopeProfileRequested  bool
	sourceInventoryProfileActive bool
	typedSourceEnumerationShape  bool
}

type sourceInventoryExecutionView = sourceinventory.ExecutionView

type sourceInventoryDiscoveryParams struct {
	Path           string                      `json:"path,omitempty"`
	View           string                      `json:"view,omitempty"`
	Scope          string                      `json:"scope,omitempty"`
	Scopes         []string                    `json:"scopes,omitempty"`
	Roles          []types.AnswerCandidateRole `json:"roles,omitempty"`
	AttributeRoles []types.AnswerCandidateRole `json:"attribute_roles,omitempty"`
	FilesOnly      bool                        `json:"files_only,omitempty"`
}

type sourceInventoryDiscoveryObservation struct {
	ToolName    string
	Path        string
	View        string
	Files       int
	ScopeGroups []string
	Roles       []types.AnswerCandidateRole
	AttrRoles   []types.AnswerCandidateRole
}

func publishSourceInventoryAdvisory(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	advisory := buildSourceInventoryAdvisory(ctx, facts, evidence)
	if advisory.IsActive() {
		ctx.Mutable.SetSourceInventoryAdvisory(advisory)
		return
	}
	// A model-driven repo lens can publish SourceInventoryObservation without the
	// current aggregate-fact pass producing a fresh request-derived advisory. Keep
	// that typed checklist alive for close/retry handoff; fresh run boundaries
	// still clear both carriers through MutableState.ClearSourceInventoryAdvisory.
	if ctx.Mutable.SourceInventoryObservation().IsActive() {
		return
	}
	ctx.Mutable.ClearSourceInventoryAdvisory()
}

// PublishSourceInventoryAdvisoryFromTypedRequest publishes a pre-completion
// source-inventory checklist from the already-typed request model and repo-map
// graph. It is deliberately advisory-only: this is retrieval/navigation
// context for explorer lanes, not answer text and not a completion signal.
func PublishSourceInventoryAdvisoryFromTypedRequest(ctx *types.BusContext) bool {
	return publishSourceInventoryAdvisorySnapshot(ctx, "pre_explore_typed_request", true)
}

// PublishSourceInventoryObservationFromTypedRequest executes the deterministic
// source-inventory lens for the current typed request when a pre-explore
// advisory is already active. This is a system projection over typed IR and
// graph artifacts, not model prose routing, and it reuses the same lens engine
// exposed through repo_map(view="source_inventory").
func PublishSourceInventoryObservationFromTypedRequest(ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	if sourceInventoryObservationHasRepoLensToolQuery(ctx.Mutable.SourceInventoryObservation()) {
		return false
	}
	advisory := ctx.Mutable.SourceInventoryAdvisory()
	if !advisory.IsActive() || !sourceInventoryAdvisoryIsExecutableTypedLane(ctx, advisory) {
		return false
	}
	query := sourceInventoryLensQueryFromAdvisory(ctx, advisory)
	if len(query.Roles) == 0 {
		return false
	}
	return publishAndPersistSourceInventoryObservationFromLens(ctx, query)
}

// PublishSourceInventoryAdvisoryFromToolObservation publishes the same
// advisory from a successful navigation observation. The returned string is a
// compact model-visible hint for the current ReAct dispatch. Empty means either
// no bounded source-inventory profile is active, no graph is available, or this
// active advisory lifecycle already attached the compact hint to a tool result.
func PublishSourceInventoryAdvisoryFromToolObservation(ctx *types.BusContext, result types.ToolResult) string {
	if ctx == nil || ctx.Mutable == nil || !result.Success || !sourceInventoryObservationTool(result.ToolName) {
		return ""
	}
	if !publishSourceInventoryAdvisorySnapshot(ctx, "tool:"+types.CanonicalToolName(result.ToolName), true) {
		return ""
	}
	if !ctx.Mutable.ClaimSourceInventoryAdvisoryHint() {
		return ""
	}
	return renderSourceInventoryAdvisoryToolHint(ctx.Mutable.SourceInventoryAdvisory())
}

func sourceInventoryAdvisoryIsExecutableTypedLane(ctx *types.BusContext, advisory types.SourceInventoryAdvisory) bool {
	if ctx == nil || ctx.AnalysisIR == nil || !advisory.IsActive() {
		return false
	}
	profile := ctx.AnalysisIR.RequestModel.SourceInventoryProfile
	if profile != nil && profile.Active() {
		return true
	}
	return sourceInventoryAdvisoryIsTypedQueryLane(ctx, advisory)
}

func sourceInventoryLensQueryFromAdvisory(ctx *types.BusContext, advisory types.SourceInventoryAdvisory) types.SourceInventoryLensQuery {
	query := types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        append([]string(nil), advisory.Scopes...),
		Roles:         sourceInventoryLensExecutionRolesFromAdvisory(advisory),
		IncludeCounts: true,
		TopN:          24,
		Query:         sourceInventoryAdvisorySnapshotQuery(ctx),
	}
	if len(query.Scopes) == 0 {
		query.Scopes = []string{"."}
	}
	return query
}

// PublishSourceInventoryObservationFromToolObservation stores exact mechanical
// candidate universes discovered by bounded navigation tools. This complements
// SourceInventoryAdvisory: advisory/lens rows help the model navigate, while a
// non-recursive list_files result is an auditable direct-child universe that
// downstream close gates can compare against model-authored member_set facts.
//
// The function is intentionally narrow and structural. It consumes only the
// tool name, list_files banner, result rows, repo path resolver, and filesystem
// metadata inside the active repository. It never inspects user prose or model
// prose and never turns rows into final answer members on its own.
func PublishSourceInventoryObservationFromToolObservation(ctx *types.BusContext, result types.ToolResult) bool {
	if ctx == nil || ctx.Mutable == nil || !result.Success {
		return false
	}
	observation := sourceInventoryObservationFromListFilesToolResult(ctx, result)
	if !observation.IsActive() {
		return false
	}
	if current := ctx.Mutable.SourceInventoryObservation(); current.IsActive() {
		observation = types.MergeSourceInventoryObservation(current, observation)
	}
	ctx.Mutable.SetSourceInventoryObservation(observation)
	return true
}

func publishSourceInventoryAdvisorySnapshot(ctx *types.BusContext, provenance string, advisoryOnly bool) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	query := types.SourceInventoryLensQuery{Query: sourceInventoryAdvisorySnapshotQuery(ctx)}
	advisory := buildSourceInventoryAdvisoryWithQuery(ctx, nil, query, advisoryOnly, nil)
	if !advisory.IsActive() {
		return false
	}
	if advisoryOnly {
		advisory.AdvisoryOnly = true
	}
	advisory.Provenance = sourceInventoryAdvisoryAppendProvenance(advisory.Provenance, provenance)
	if current := ctx.Mutable.SourceInventoryAdvisory(); current.IsActive() {
		advisory = types.MergeSourceInventoryAdvisory(current, advisory)
	}
	ctx.Mutable.SetSourceInventoryAdvisory(advisory)
	return true
}

func sourceInventoryAdvisorySnapshotQuery(ctx *types.BusContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	if profile := ctx.AnalysisIR.RequestModel.SourceInventoryProfile; profile != nil && profile.Active() {
		if query := strings.Join(trimStringSlice(profile.SourceQuotes), " "); strings.TrimSpace(query) != "" {
			return query
		}
		if profile.Confidence > 0 && profile.Confidence < 0.70 {
			return sourceInventoryAdvisoryQueryFromRequest(ctx)
		}
		return ""
	}
	return sourceInventoryAdvisoryQueryFromRequest(ctx)
}

func sourceInventoryAdvisoryQueryFromRequest(ctx *types.BusContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	policy := types.CompileRepoMapNavigationPolicy(ctx.AnalysisIR.RequestModel, &ctx.AnalysisIR.AnswerContract, ctx.ExploreLanePlan)
	return strings.Join(policy.QueryTerms, " ")
}

func sourceInventoryObservationFromListFilesToolResult(ctx *types.BusContext, result types.ToolResult) types.SourceInventoryObservation {
	if types.CanonicalToolName(result.ToolName) != "list_files" || !sourceInventoryListFilesResultIsDirect(result.Summary) {
		return types.SourceInventoryObservation{}
	}
	listPath := sourceInventoryDiscoveryPathFromBanner(result.Summary, "list_files")
	if listPath == "" {
		listPath = "."
	}
	listPath, ok := sourceInventoryDiscoveryNormalizeToolPath(ctx, "list_files", listPath, result.Summary, "list_files")
	if !ok {
		return types.SourceInventoryObservation{}
	}
	setsByRole := map[types.AnswerCandidateRole]*types.SourceInventoryObservationSet{}
	roleOrder := []types.AnswerCandidateRole{}
	add := func(member types.SourceInventoryObservationMember) {
		name := strings.TrimSpace(member.Name)
		if name == "" {
			return
		}
		if member.Role == types.AnswerCandidateRoleUnknown {
			return
		}
		set := setsByRole[member.Role]
		if set == nil {
			set = &types.SourceInventoryObservationSet{Role: member.Role, Complete: true}
			setsByRole[member.Role] = set
			roleOrder = append(roleOrder, member.Role)
		}
		set.Members = append(set.Members, member)
		set.Count = len(set.Members)
	}
	for _, line := range sourceInventoryDiscoveryResultLines(result.Summary) {
		if strings.HasPrefix(line, "[list_files:") {
			continue
		}
		rel := sourceInventoryDiscoverySafeResultPath(ctx, line)
		if rel == "" || !sourceInventoryDiscoveryResultPathExists(ctx, rel) {
			continue
		}
		member := sourceInventoryObservationMemberFromListFilesPath(ctx, rel)
		add(member)
	}
	if len(roleOrder) == 0 {
		return types.SourceInventoryObservation{}
	}
	out := types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{listPath},
		Provenance:   []string{"tool:list_files:direct"},
		Lens:         []string{"direct_children", "count"},
		Sets:         make([]types.SourceInventoryObservationSet, 0, len(roleOrder)),
	}
	for _, role := range roleOrder {
		set := setsByRole[role]
		if set == nil || len(set.Members) == 0 {
			continue
		}
		set.Count = len(set.Members)
		out.Sets = append(out.Sets, *set)
	}
	if len(out.Sets) == 0 {
		return types.SourceInventoryObservation{}
	}
	return types.CloneSourceInventoryObservation(out)
}

func sourceInventoryListFilesResultIsDirect(summary string) bool {
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[list_files:") {
			continue
		}
		for _, field := range strings.Fields(strings.TrimSuffix(strings.TrimPrefix(line, "[list_files:"), "]")) {
			key, value, ok := strings.Cut(field, "=")
			if ok && key == "recursive" && strings.EqualFold(strings.TrimSpace(value), "true") {
				return false
			}
		}
		return true
	}
	return false
}

func sourceInventoryObservationMemberFromListFilesPath(ctx *types.BusContext, rel string) types.SourceInventoryObservationMember {
	rel = strings.Trim(strings.TrimSpace(strings.ReplaceAll(rel, `\`, `/`)), "/")
	if rel == "" {
		return types.SourceInventoryObservationMember{}
	}
	role := types.AnswerCandidateRoleFile
	name := rel
	if sourceInventoryListFilesPathIsDir(ctx, rel) {
		role = types.AnswerCandidateRolePackage
		if base := path.Base(rel); base != "." && base != "/" && base != "" {
			name = base
		}
	} else if types.LooksLikeConfigFilePath(rel) {
		role = types.AnswerCandidateRoleConfigFile
	}
	return types.SourceInventoryObservationMember{
		Name:          name,
		Key:           rel,
		SupportRef:    rel,
		Provenance:    []string{"tool:list_files:direct"},
		Role:          role,
		File:          rel,
		CoverageState: types.SourceInventoryCoverageObserved,
	}
}

func sourceInventoryListFilesPathIsDir(ctx *types.BusContext, rel string) bool {
	if ctx == nil {
		return false
	}
	fsPath := resolveToolPath(ctx, rel)
	if strings.TrimSpace(fsPath) == "" {
		return false
	}
	info, err := os.Stat(fsPath)
	return err == nil && info.IsDir()
}

// PublishSourceInventoryObservationFromLens builds and stores a model-driven
// source-inventory lens view. It is deliberately advisory-only: even when the
// model asks for a role/scope slice explicitly, the result is a repo-map
// observation checklist, not final answer text.
func PublishSourceInventoryObservationFromLens(ctx *types.BusContext, query types.SourceInventoryLensQuery) types.SourceInventoryObservation {
	if ctx == nil || ctx.Mutable == nil {
		return types.SourceInventoryObservation{}
	}
	advisoryProvenance := append([]string{"repo_lens:tool_query"}, query.Provenance...)
	advisory := buildSourceInventoryAdvisoryForLens(ctx, query, true, advisoryProvenance)
	if !advisory.IsActive() {
		classOnly := sourceInventoryObservationWithSourceClassUniverse(ctx, types.SourceInventoryObservation{
			Active:       true,
			AdvisoryOnly: true,
			Complete:     true,
			Scopes:       sourceInventorySourceClassUniverseScopes(query),
			Provenance:   advisoryProvenance,
			Lens:         []string{"source_class_universe", "count"},
		}, query)
		if classOnly.IsActive() {
			renderObservation := types.CloneSourceInventoryObservation(classOnly)
			current := ctx.Mutable.SourceInventoryObservation()
			if current.IsActive() {
				ctx.Mutable.SetSourceInventoryObservation(types.MergeSourceInventoryObservation(current, classOnly))
			} else {
				ctx.Mutable.SetSourceInventoryObservation(classOnly)
			}
			return renderObservation
		}
		return types.SourceInventoryObservation{}
	}
	if current := ctx.Mutable.SourceInventoryAdvisory(); current.IsActive() {
		ctx.Mutable.SetSourceInventoryAdvisory(types.MergeSourceInventoryAdvisory(current, advisory))
	} else {
		ctx.Mutable.SetSourceInventoryAdvisory(advisory)
	}
	// Render only the current lens query. Exact/direct-child observations are
	// kept in MutableState for coverage checks, but mixing them back into the
	// visible repo_map result makes a narrow lens look like a broad union and
	// can mislead the model into treating navigation hints as the answer set.
	renderObservation := types.SourceInventoryObservationFromAdvisory(advisory)
	renderObservation = sourceInventoryObservationWithSourceClassUniverse(ctx, renderObservation, query)
	renderObservation = sourceInventoryObservationWithLensExecutionState(renderObservation, query)
	if exact := sourceInventoryObservationFromLensDirectChildren(ctx, query); exact.IsActive() {
		exact = sourceInventoryObservationWithSourceClassUniverse(ctx, exact, query)
		exact = sourceInventoryObservationWithLensExecutionState(exact, query)
		current := ctx.Mutable.SourceInventoryObservation()
		if current.IsActive() {
			exact = types.MergeSourceInventoryObservation(current, exact)
		}
		ctx.Mutable.SetSourceInventoryObservation(exact)
	}
	if renderObservation.IsActive() {
		return renderObservation
	}
	return ctx.Mutable.SourceInventoryObservation()
}

func sourceInventoryObservationWithSourceClassUniverse(ctx *types.BusContext, observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) types.SourceInventoryObservation {
	sourceClasses := sourceInventorySourceClassUniverseForLens(ctx, query)
	if len(sourceClasses) == 0 {
		return observation
	}
	if len(observation.Scopes) == 0 {
		observation.Scopes = sourceInventorySourceClassUniverseScopes(query)
	}
	observation.SourceClasses = sourceClasses
	observation.Lens = sourceInventoryAdvisoryAppendProvenance(observation.Lens, "source_class_universe", "count")
	observation.Provenance = sourceInventoryAdvisoryAppendProvenance(observation.Provenance, "source_class_universe:repo_tracked")
	observation.Complete = observation.Complete && sourceInventorySourceClassUniverseComplete(sourceClasses)
	if languages := sourceInventoryRepoLanguageCensus(ctx, observation.Scopes); len(languages) > 0 {
		observation.RepoLanguages = languages
		observation.Lens = sourceInventoryAdvisoryAppendProvenance(observation.Lens, "repo_languages")
	}
	return types.CloneSourceInventoryObservation(observation)
}

// AttachSourceInventorySourceClassUniverse adds the repo-owned source-class
// matrix to a source-inventory observation without running the full member
// reconciler. Fast-path repo_map guards use this to preserve exact-absence
// authority while still refusing or bounding over-broad navigation shapes.
func AttachSourceInventorySourceClassUniverse(ctx *types.BusContext, observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) types.SourceInventoryObservation {
	return sourceInventoryObservationWithSourceClassUniverse(ctx, observation, query)
}

// AttachSourceInventoryLensExecutionState adds typed pagination and deterministic
// budget metadata to a source-inventory observation. The information mirrors
// the current lens query and is intended for status cards, ledgers, and gates;
// renderers still use the query they are called with for visible pagination.
func AttachSourceInventoryLensExecutionState(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) types.SourceInventoryObservation {
	return sourceInventoryObservationWithLensExecutionState(observation, query)
}

func sourceInventoryObservationWithLensExecutionState(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) types.SourceInventoryObservation {
	if !observation.IsActive() {
		return observation
	}
	out := types.CloneSourceInventoryObservation(observation)
	budgetTruncated := sourceInventoryStringSliceContains(out.Provenance, "repo_lens:candidate_budget_truncated")
	attributesDeferred := sourceInventoryStringSliceContains(out.Provenance, "repo_lens:attributes_deferred_broad_scope")
	if budgetTruncated || attributesDeferred {
		out.Execution = &types.SourceInventoryExecutionState{
			Budgeted:                 budgetTruncated,
			CandidateBudgetTruncated: budgetTruncated,
			AttributesDeferred:       attributesDeferred,
		}
	}
	total := sourceInventoryObservationMemberTotal(out)
	if total == 0 {
		return out
	}
	page, ok := sourceinventory.Page(
		sourceInventoryLensQueryOffset(query),
		sourceInventoryObservationPageLimit(query),
		total,
		budgetTruncated,
	)
	if ok {
		out.Page = &page
	}
	return out
}

func sourceInventoryObservationMemberTotal(observation types.SourceInventoryObservation) int {
	total := 0
	for _, set := range observation.Sets {
		if set.Total > len(set.Members) {
			total += set.Total
			continue
		}
		total += len(set.Members)
	}
	return total
}

func sourceInventoryObservationPageLimit(query types.SourceInventoryLensQuery) int {
	return sourceinventory.PageLimit(query.TopN, query.RepoFileCount)
}

func sourceInventorySourceClassUniverseForLens(ctx *types.BusContext, query types.SourceInventoryLensQuery) []types.SourceInventorySourceClassCount {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return nil
	}
	scopes := sourceInventorySourceClassUniverseScopes(query)
	files, complete := sourceInventoryTrackedRepoFiles(ctx.RepoRoot)
	if len(files) == 0 {
		return nil
	}
	counts := map[types.SourcePathRole]int{}
	for _, rel := range files {
		rel = strings.Trim(strings.ReplaceAll(rel, `\`, `/`), "/")
		if rel == "" || !sourceInventoryFileInScopes(rel, scopes) {
			continue
		}
		lang := repotypes.DetectLanguage(rel)
		if lang == "" || !repotypes.IsSupportedReadLanguage(lang) {
			continue
		}
		role := types.ClassifySourcePathRole(rel)
		if role == types.SourcePathRoleUnknown {
			continue
		}
		counts[role]++
	}
	if len(counts) == 0 {
		return nil
	}
	order := []types.SourcePathRole{
		types.SourcePathRoleProduction,
		types.SourcePathRoleTest,
		types.SourcePathRoleFixture,
		types.SourcePathRoleExample,
		types.SourcePathRoleDocumentation,
		types.SourcePathRolePromptSupport,
		types.SourcePathRoleThirdParty,
		types.SourcePathRoleVendor,
		types.SourcePathRoleGenerated,
	}
	var out []types.SourceInventorySourceClassCount
	for _, role := range order {
		if count := counts[role]; count > 0 {
			out = append(out, types.SourceInventorySourceClassCount{
				Role:       role,
				Count:      count,
				Complete:   complete,
				Provenance: []string{"source_class_universe:git_tracked_or_filesystem"},
			})
		}
		delete(counts, role)
	}
	for role, count := range counts {
		if count <= 0 {
			continue
		}
		out = append(out, types.SourceInventorySourceClassCount{
			Role:       role,
			Count:      count,
			Complete:   complete,
			Provenance: []string{"source_class_universe:git_tracked_or_filesystem"},
		})
	}
	return out
}

func sourceInventorySourceClassUniverseScopes(query types.SourceInventoryLensQuery) []string {
	scopes := sourceInventoryLensDirectChildScopes(query)
	if len(scopes) == 0 {
		scopes = append([]string(nil), query.Scopes...)
	}
	if len(scopes) == 0 {
		scopes = []string{"."}
	}
	return dedupStringsPreserveOrder(scopes)
}

func sourceInventorySourceClassUniverseComplete(classes []types.SourceInventorySourceClassCount) bool {
	if len(classes) == 0 {
		return false
	}
	for _, class := range classes {
		if !class.Complete {
			return false
		}
	}
	return true
}

func sourceInventoryTrackedRepoFiles(repoRoot string) ([]string, bool) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil, false
	}
	if files, ok := sourceInventoryGitTrackedFiles(repoRoot); ok {
		return sourceInventoryLimitSourceClassFiles(files)
	}
	files, complete := sourceInventoryFilesystemFiles(repoRoot)
	return sourceInventoryLimitSourceClassFilesWithCompleteness(files, complete)
}

func sourceInventoryGitTrackedFiles(repoRoot string) ([]string, bool) {
	cmd, cancel := NewGitCommand(nil, "ls-files", "-z", "--")
	defer cancel()
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil, false
	}
	parts := bytes.Split(out, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		rel := strings.Trim(strings.ReplaceAll(string(part), `\`, `/`), "/")
		if rel == "" {
			continue
		}
		files = append(files, rel)
	}
	return files, len(files) > 0
}

func sourceInventoryFilesystemFiles(repoRoot string) ([]string, bool) {
	var files []string
	complete := true
	err := filepath.WalkDir(repoRoot, func(abs string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		name := strings.TrimSpace(d.Name())
		if name == "" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if abs == repoRoot {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, abs)
		if relErr != nil {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel = strings.Trim(strings.ReplaceAll(rel, `\`, `/`), "/")
		if rel == "" {
			return nil
		}
		if d.IsDir() {
			if sourceInventorySourceClassUniverseSkipDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, rel)
		if len(files) >= sourceInventorySourceClassUniverseMaxFiles {
			complete = false
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		complete = false
	}
	return files, complete
}

func sourceInventorySourceClassUniverseSkipDir(base string) bool {
	switch strings.ToLower(strings.TrimSpace(base)) {
	case ".git", ".hg", ".svn", ".codrax", "node_modules", ".tox", ".venv", "venv",
		".mypy_cache", ".pytest_cache", ".idea", ".vscode", ".vs", ".gradle", ".cargo",
		".next", ".nuxt", ".turbo", ".pnpm-store":
		return true
	default:
		return false
	}
}

func sourceInventoryLimitSourceClassFiles(files []string) ([]string, bool) {
	return sourceInventoryLimitSourceClassFilesWithCompleteness(files, true)
}

func sourceInventoryLimitSourceClassFilesWithCompleteness(files []string, complete bool) ([]string, bool) {
	if len(files) <= sourceInventorySourceClassUniverseMaxFiles {
		return files, complete
	}
	return files[:sourceInventorySourceClassUniverseMaxFiles], false
}

func sourceInventoryObservationFromLensDirectChildren(ctx *types.BusContext, query types.SourceInventoryLensQuery) types.SourceInventoryObservation {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return types.SourceInventoryObservation{}
	}
	roleAllowed := sourceInventoryLensDirectChildRoles(query.Roles)
	if len(roleAllowed) == 0 {
		return types.SourceInventoryObservation{}
	}
	scopes := sourceInventoryLensDirectChildScopes(query)
	if len(scopes) == 0 {
		return types.SourceInventoryObservation{}
	}
	setsByRole := map[types.AnswerCandidateRole]*types.SourceInventoryObservationSet{}
	var roleOrder []types.AnswerCandidateRole
	add := func(member types.SourceInventoryObservationMember) {
		if member.Role == types.AnswerCandidateRoleUnknown || !roleAllowed[member.Role] {
			return
		}
		if strings.TrimSpace(member.Name) == "" {
			return
		}
		set := setsByRole[member.Role]
		if set == nil {
			set = &types.SourceInventoryObservationSet{Role: member.Role, Complete: true}
			setsByRole[member.Role] = set
			roleOrder = append(roleOrder, member.Role)
		}
		set.Members = append(set.Members, member)
		set.Count = len(set.Members)
	}
	for _, scope := range scopes {
		scope, ok := sourceInventoryDiscoveryNormalizeToolPath(ctx, "repo_map", scope, "", "")
		if !ok {
			continue
		}
		fsPath := resolveToolPath(ctx, scope)
		entries, err := os.ReadDir(fsPath)
		if err != nil {
			continue
		}
		dirFilter := NewSearchDirFilter(ctx.RepoRoot, fsPath)
		for _, entry := range entries {
			name := strings.TrimSpace(entry.Name())
			if name == "" {
				continue
			}
			rel := path.Join(scope, name)
			if entry.IsDir() {
				if dirFilter.ExcludesRepoRelativePath(rel) {
					continue
				}
			} else if IsExcludedDirName(name) {
				continue
			}
			member := sourceInventoryObservationMemberFromListFilesPath(ctx, rel)
			if member.Role == types.AnswerCandidateRoleFile && types.LooksLikeConfigFilePath(rel) {
				member.Role = types.AnswerCandidateRoleConfigFile
			}
			member.Provenance = []string{sourceInventoryExactUniverseProvenanceRepoLensDirectChildren}
			member.Note = "exact direct child observed for source_inventory navigation"
			add(member)
		}
	}
	if len(roleOrder) == 0 {
		return types.SourceInventoryObservation{}
	}
	out := types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       scopes,
		Provenance:   []string{sourceInventoryExactUniverseProvenanceRepoLensDirectChildren},
		Lens:         []string{"direct_children", "count"},
		Sets:         make([]types.SourceInventoryObservationSet, 0, len(roleOrder)),
	}
	for _, role := range roleOrder {
		set := setsByRole[role]
		if set == nil || len(set.Members) == 0 {
			continue
		}
		set.Count = len(set.Members)
		out.Sets = append(out.Sets, *set)
	}
	return types.CloneSourceInventoryObservation(out)
}

func sourceInventoryLensDirectChildRoles(roles []types.AnswerCandidateRole) map[types.AnswerCandidateRole]bool {
	out := map[types.AnswerCandidateRole]bool{}
	for _, role := range roles {
		switch role {
		case types.AnswerCandidateRolePackage,
			types.AnswerCandidateRoleFile,
			types.AnswerCandidateRoleConfigFile:
			out[role] = true
		}
	}
	return out
}

func sourceInventoryLensDirectChildScopes(query types.SourceInventoryLensQuery) []string {
	base, ok := sourceInventoryLensSafeRelativePath(query.Path)
	if !ok {
		return nil
	}
	if base == "" || base == "." {
		base = "."
	}
	rawScopes := append([]string(nil), query.Scopes...)
	if len(rawScopes) == 0 {
		rawScopes = []string{"."}
	}
	var scopes []string
	for _, raw := range rawScopes {
		raw, ok = sourceInventoryLensSafeRelativePath(raw)
		if !ok {
			continue
		}
		if raw == "" || raw == "." {
			scopes = append(scopes, base)
			continue
		}
		if base != "." && !strings.HasPrefix(raw, base+"/") && raw != base {
			raw = path.Join(base, raw)
		}
		scopes = append(scopes, raw)
	}
	return dedupStringsPreserveOrder(scopes)
}

func sourceInventoryLensSafeRelativePath(raw string) (string, bool) {
	p := strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)), "/")
	if p == "" || p == "." {
		return ".", true
	}
	if toolPathIsAbs(p) {
		return "", false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", false
		}
	}
	return sourceInventoryDiscoverySafeToolPath(p)
}

func sourceInventoryObservationTool(name string) bool {
	switch types.CanonicalToolName(name) {
	case "repo_map", "list_files":
		return true
	default:
		return false
	}
}

// SourceInventoryDiscoveryHintFromToolObservation renders a small advisory
// entrypoint hint when a broad navigation tool result suggests that a typed
// source-inventory lens would be cheaper than more ad-hoc listing/reading. It
// uses only structured tool parameters plus the tool result shape; it never
// inspects the user's raw question or the model's prose.
func SourceInventoryDiscoveryHintFromToolObservation(ctx *types.BusContext, result types.ToolResult, params json.RawMessage) string {
	if ctx != nil && ctx.PipelineStage == types.StageAnalyze {
		return ""
	}
	obs, ok := sourceInventoryDiscoveryObservationFromTool(ctx, result, params)
	if !ok {
		return ""
	}
	if ctx != nil && ctx.Mutable != nil {
		if ctx.Mutable.SourceInventoryAdvisory().IsActive() || ctx.Mutable.SourceInventoryObservation().IsActive() {
			return ""
		}
		if !ctx.Mutable.ClaimSourceInventoryDiscoveryHint(sourceInventoryDiscoveryKey(obs)) {
			return ""
		}
	}
	return renderSourceInventoryDiscoveryHint(obs)
}

// SourceInventoryDiscoveryHintFromToolHistory is a fallback for mid-loop
// recovery hints where the original tool-call params are no longer available.
// It parses only structured banners/tool-result rows and returns one bounded
// suggestion for the latest broad list/grep result.
func SourceInventoryDiscoveryHintFromToolHistory(results []types.ToolResult) string {
	for i := len(results) - 1; i >= 0; i-- {
		obs, ok := sourceInventoryDiscoveryObservationFromHistory(results[i])
		if !ok {
			continue
		}
		return renderSourceInventoryDiscoveryHint(obs)
	}
	return ""
}

func sourceInventoryDiscoveryObservationFromTool(ctx *types.BusContext, result types.ToolResult, params json.RawMessage) (sourceInventoryDiscoveryObservation, bool) {
	if !result.Success {
		return sourceInventoryDiscoveryObservation{}, false
	}
	toolName := types.CanonicalToolName(result.ToolName)
	if toolName != "repo_map" && toolName != "list_files" && toolName != "grep" {
		return sourceInventoryDiscoveryObservation{}, false
	}
	var p sourceInventoryDiscoveryParams
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}
	roles := sourceInventoryNormalizeLensRoles(p.Roles)
	attrRoles := sourceInventoryNormalizeLensRoles(p.AttributeRoles)
	var obs sourceInventoryDiscoveryObservation
	switch toolName {
	case "repo_map":
		pathSurface, pathOK := sourceInventoryDiscoveryNormalizeToolPath(ctx, toolName, p.Path, result.Summary, "")
		if !pathOK {
			return sourceInventoryDiscoveryObservation{}, false
		}
		view := strings.TrimSpace(p.View)
		if view == "" {
			view = "overview"
		}
		if view == "source_inventory" {
			return sourceInventoryDiscoveryObservation{}, false
		}
		files, scopes := sourceInventoryDiscoveryRepoMapShape(ctx, result.Summary, pathSurface)
		obs = sourceInventoryDiscoveryObservation{
			ToolName:    toolName,
			Path:        pathSurface,
			View:        view,
			Files:       files,
			ScopeGroups: scopes,
			Roles:       roles,
			AttrRoles:   attrRoles,
		}
	case "list_files":
		pathSurface, pathOK := sourceInventoryDiscoveryNormalizeToolPath(ctx, toolName, p.Path, result.Summary, "list_files")
		if !pathOK {
			return sourceInventoryDiscoveryObservation{}, false
		}
		files, scopes := sourceInventoryDiscoveryListFilesShape(ctx, result.Summary, pathSurface)
		obs = sourceInventoryDiscoveryObservation{
			ToolName:    toolName,
			Path:        pathSurface,
			Files:       files,
			ScopeGroups: scopes,
			Roles:       roles,
			AttrRoles:   attrRoles,
		}
	case "grep":
		if !p.FilesOnly {
			return sourceInventoryDiscoveryObservation{}, false
		}
		pathSurface, pathOK := sourceInventoryDiscoveryNormalizeToolPath(ctx, toolName, p.Path, result.Summary, "grep params")
		if !pathOK {
			return sourceInventoryDiscoveryObservation{}, false
		}
		files, scopes := sourceInventoryDiscoveryGrepFilesShape(ctx, result.Summary, pathSurface)
		obs = sourceInventoryDiscoveryObservation{
			ToolName:    toolName,
			Path:        pathSurface,
			Files:       files,
			ScopeGroups: scopes,
			Roles:       roles,
			AttrRoles:   attrRoles,
		}
	}
	if !sourceInventoryDiscoveryObservationBroad(obs) {
		return sourceInventoryDiscoveryObservation{}, false
	}
	return obs, true
}

func sourceInventoryDiscoveryObservationFromHistory(result types.ToolResult) (sourceInventoryDiscoveryObservation, bool) {
	if !result.Success {
		return sourceInventoryDiscoveryObservation{}, false
	}
	toolName := types.CanonicalToolName(result.ToolName)
	switch toolName {
	case "list_files":
		pathSurface := sourceInventoryDiscoveryPathFromBanner(result.Summary, "list_files")
		if pathSurface == "" {
			pathSurface = "."
		}
		files, scopes := sourceInventoryDiscoveryListFilesShape(nil, result.Summary, pathSurface)
		obs := sourceInventoryDiscoveryObservation{ToolName: toolName, Path: pathSurface, Files: files, ScopeGroups: scopes}
		if sourceInventoryDiscoveryObservationBroad(obs) {
			return obs, true
		}
	case "grep":
		if !strings.Contains(result.Summary, "files_only=true") {
			return sourceInventoryDiscoveryObservation{}, false
		}
		pathSurface := sourceInventoryDiscoveryPathFromBanner(result.Summary, "grep params")
		if pathSurface == "" {
			pathSurface = "."
		}
		files, scopes := sourceInventoryDiscoveryGrepFilesShape(nil, result.Summary, pathSurface)
		obs := sourceInventoryDiscoveryObservation{ToolName: toolName, Path: pathSurface, Files: files, ScopeGroups: scopes}
		if sourceInventoryDiscoveryObservationBroad(obs) {
			return obs, true
		}
	}
	return sourceInventoryDiscoveryObservation{}, false
}

func sourceInventoryDiscoverySafePath(raw string) string {
	p := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	if p == "" {
		return ""
	}
	if toolPathIsAbs(p) {
		return ""
	}
	p = path.Clean(p)
	if p == "." {
		return "."
	}
	if p == ".." || strings.HasPrefix(p, "../") {
		return ""
	}
	return strings.Trim(p, "/")
}

func sourceInventoryDiscoverySafeToolPath(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return ".", true
	}
	p := sourceInventoryDiscoverySafePath(raw)
	if p == "" {
		return "", false
	}
	return p, true
}

func sourceInventoryDiscoveryNormalizeToolPath(ctx *types.BusContext, toolName, raw, summary, bannerName string) (string, bool) {
	if bannerName != "" {
		if bannerPath := sourceInventoryDiscoveryPathFromBanner(summary, bannerName); bannerPath != "" {
			raw = bannerPath
		}
	}
	if strings.TrimSpace(raw) == "" {
		raw = "."
	}
	if ctx != nil {
		if gater, ok := ctx.MultiGraph.(types.MultiRepoActiveSetGater); ok && gater != nil {
			gate := gater.ResolveActiveSetPath(ctx, toolName, raw, nil)
			if !gate.Allowed {
				return "", false
			}
			if strings.TrimSpace(gate.ResolvedPath) != "" {
				raw = gate.ResolvedPath
			}
		}
	}
	if toolPathIsAbs(raw) {
		if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
			return "", false
		}
		if rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, raw); ok {
			if rel == "" {
				return ".", true
			}
			return sourceInventoryDiscoverySafeToolPath(rel)
		}
		return "", false
	}
	pathSurface, ok := sourceInventoryDiscoverySafeToolPath(raw)
	if !ok {
		return "", false
	}
	if ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		fsPath := resolveToolPath(ctx, pathSurface)
		rel, within := repoRelativePathWithinRoot(ctx.RepoRoot, fsPath)
		if !within {
			return "", false
		}
		if rel == "" {
			return ".", true
		}
		return sourceInventoryDiscoverySafeToolPath(rel)
	}
	return pathSurface, true
}

func sourceInventoryDiscoveryPathFromBanner(summary, bannerName string) string {
	prefix := "[" + bannerName + ":"
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "]") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(line, prefix), "]")
		for _, field := range strings.Fields(body) {
			key, value, ok := strings.Cut(field, "=")
			if ok && key == "path" {
				return sourceInventoryDiscoverySafePath(value)
			}
		}
	}
	return ""
}

func sourceInventoryDiscoveryRepoMapShape(ctx *types.BusContext, summary, basePath string) (int, []string) {
	seenFiles := map[string]bool{}
	seenScopes := map[string]bool{}
	var scopes []string
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		header := strings.TrimSpace(strings.TrimPrefix(line, "## "))
		if header == "" || strings.HasPrefix(header, "Scope-grouped") || strings.HasPrefix(header, "Cascaded") {
			continue
		}
		file := sourceInventoryDiscoverySafeResultPath(ctx, strings.Trim(strings.Fields(header)[0], "`"))
		if file == "" || seenFiles[file] {
			continue
		}
		seenFiles[file] = true
		if scope := sourceInventoryDiscoveryChildScope(file, basePath); scope != "" && !seenScopes[scope] {
			seenScopes[scope] = true
			scopes = append(scopes, scope)
		}
	}
	return len(seenFiles), scopes
}

func sourceInventoryDiscoveryListFilesShape(ctx *types.BusContext, summary, basePath string) (int, []string) {
	seenFiles := map[string]bool{}
	seenScopes := map[string]bool{}
	var scopes []string
	for _, line := range sourceInventoryDiscoveryResultLines(summary) {
		if strings.HasPrefix(line, "[list_files:") {
			continue
		}
		file := sourceInventoryDiscoverySafeResultPath(ctx, line)
		if file == "" || seenFiles[file] {
			continue
		}
		seenFiles[file] = true
		if scope := sourceInventoryDiscoveryChildScope(file, basePath); scope != "" && !seenScopes[scope] {
			seenScopes[scope] = true
			scopes = append(scopes, scope)
		}
	}
	return len(seenFiles), scopes
}

func sourceInventoryDiscoveryGrepFilesShape(ctx *types.BusContext, summary, basePath string) (int, []string) {
	seenFiles := map[string]bool{}
	seenScopes := map[string]bool{}
	var scopes []string
	for _, line := range sourceInventoryDiscoveryResultLines(summary) {
		if strings.HasPrefix(line, "[") || strings.HasPrefix(line, "grep params") {
			continue
		}
		file := sourceInventoryDiscoverySafeResultPath(ctx, line)
		if file == "" || seenFiles[file] {
			continue
		}
		seenFiles[file] = true
		if scope := sourceInventoryDiscoveryChildScope(file, basePath); scope != "" && !seenScopes[scope] {
			seenScopes[scope] = true
			scopes = append(scopes, scope)
		}
	}
	return len(seenFiles), scopes
}

func sourceInventoryDiscoveryResultLines(summary string) []string {
	var lines []string
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Repo Lens discovery hint") {
			break
		}
		if line == "" || strings.HasPrefix(line, "...[") || strings.HasPrefix(line, "[[truncated") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func sourceInventoryDiscoverySafeResultPath(ctx *types.BusContext, raw string) string {
	p := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	if p == "" || strings.Contains(p, "\x00") || strings.HasPrefix(p, "[") {
		return ""
	}
	if idx := strings.Index(p, ":"); idx > 1 {
		before := p[:idx]
		after := p[idx+1:]
		if _, err := strconv.Atoi(strings.TrimSpace(strings.SplitN(after, ":", 2)[0])); err == nil {
			p = before
		}
	}
	p = strings.Trim(p, "`")
	if toolPathIsAbs(p) {
		if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
			return ""
		}
		rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, p)
		if !ok {
			return ""
		}
		if rel == "" {
			return "."
		}
		safe, ok := sourceInventoryDiscoverySafeToolPath(rel)
		if !ok {
			return ""
		}
		return safe
	}
	safe, ok := sourceInventoryDiscoverySafeToolPath(p)
	if !ok || safe == "." {
		return ""
	}
	if ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		fsPath := resolveToolPath(ctx, safe)
		rel, within := repoRelativePathWithinRoot(ctx.RepoRoot, fsPath)
		if !within || rel == "" {
			return ""
		}
		safe, ok = sourceInventoryDiscoverySafeToolPath(rel)
		if !ok || safe == "." {
			return ""
		}
	}
	return safe
}

func sourceInventoryDiscoveryResultPathExists(ctx *types.BusContext, rel string) bool {
	rel = strings.Trim(strings.TrimSpace(strings.ReplaceAll(rel, `\`, `/`)), "/")
	if rel == "" {
		return false
	}
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return true
	}
	fsPath := resolveToolPath(ctx, rel)
	if _, err := os.Stat(fsPath); err != nil {
		return false
	}
	return true
}

func sourceInventoryDiscoveryChildScope(file, basePath string) string {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	base := strings.Trim(strings.TrimSpace(strings.ReplaceAll(basePath, `\`, `/`)), "/")
	if file == "" {
		return ""
	}
	if base != "" && base != "." && strings.HasPrefix(file, base+"/") {
		file = strings.TrimPrefix(file, base+"/")
	}
	if file == "" || file == "." {
		return "."
	}
	parts := strings.Split(file, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return ""
	}
	return parts[0]
}

func sourceInventoryDiscoveryObservationBroad(obs sourceInventoryDiscoveryObservation) bool {
	if len(obs.ScopeGroups) >= 4 {
		return true
	}
	if obs.Files >= 8 {
		return true
	}
	return false
}

func sourceInventoryDiscoveryKey(obs sourceInventoryDiscoveryObservation) string {
	pathSurface := obs.Path
	if pathSurface == "" {
		pathSurface = "."
	}
	return strings.Join([]string{
		types.CanonicalToolName(obs.ToolName),
		pathSurface,
		obs.View,
		strings.Join(obs.ScopeGroups, ","),
	}, "\x00")
}

func renderSourceInventoryDiscoveryHint(obs sourceInventoryDiscoveryObservation) string {
	pathSurface := obs.Path
	if pathSurface == "" {
		pathSurface = "."
	}
	attrRoles := sourceInventoryDiscoveryAttributeRoles(obs)
	expandRoles := sourceInventoryDiscoveryExpandRoles(obs, attrRoles)
	var b strings.Builder
	b.WriteString("Repo Lens discovery hint (advisory): this navigation result is broad")
	if len(obs.ScopeGroups) > 0 || obs.Files > 0 {
		fmt.Fprintf(&b, " (scope_groups=%d candidate_files=%d)", len(obs.ScopeGroups), obs.Files)
	}
	b.WriteString(". To inspect it incrementally, consider `repo_map(view=\"source_inventory\")` before reading many files. These are verified navigation/count/candidate-universe facts, not semantic source citations.\n")
	if len(obs.ScopeGroups) > 0 {
		b.WriteString("- choose one branch before source_inventory expansion; do not expand all broad branches at once.\n")
	} else {
		fmt.Fprintf(&b, "- broad member/count checklist: `%s`\n",
			sourceInventoryCascadeRepoMapCall(pathSurface, "", nil, expandRoles, nil, false, 24, ""))
	}
	fmt.Fprintf(&b, "- structural relation lens by chosen source: `%s` when the next step is to inspect calls/imports/inheritance/references around a selected symbol or file.\n",
		sourceInventoryDiscoveryRelationMapCall(pathSurface, "", "<symbol-or-file>"))
	fmt.Fprintf(&b, "- structural relation lens by scope: `%s` when the next step is to inspect edges inside a chosen directory/module.\n",
		sourceInventoryDiscoveryRelationMapCall(pathSurface, "<scope>", ""))
	if len(obs.ScopeGroups) > 0 {
		b.WriteString("- expand only the branch that matches the user's intent:\n")
		maxScopes := minInt(len(obs.ScopeGroups), 4)
		for i := 0; i < maxScopes; i++ {
			scope := obs.ScopeGroups[i]
			fmt.Fprintf(&b, "  - `%s`: `%s`\n", scope,
				sourceInventoryCascadeRepoMapCall(pathSurface, scope, nil, expandRoles, nil, false, 24, ""))
		}
		if len(obs.ScopeGroups) > maxScopes {
			fmt.Fprintf(&b, "  - +%d more scope groups hidden here; use the same call shape with the chosen scope.\n", len(obs.ScopeGroups)-maxScopes)
		}
	} else {
		fmt.Fprintf(&b, "- expand a chosen scope: `%s`\n",
			sourceInventoryCascadeRepoMapCall(pathSurface, "<scope>", nil, expandRoles, nil, false, 24, ""))
	}
	if len(attrRoles) > 0 {
		fmt.Fprintf(&b, "- after choosing a narrow scope/member, add row-local attributes only if needed: `%s`\n",
			sourceInventoryCascadeRepoMapCall(pathSurface, "<chosen-scope>", nil, expandRoles, attrRoles, true, 24, ""))
	}
	b.WriteString("- Adjust `roles` to the structured member role you need; use `attribute_roles` only for row-local details after narrowing. Verify chosen rows with `read_file` or `grep` before citing.")
	return strings.TrimSpace(b.String())
}

func sourceInventoryDiscoveryRelationMapCall(pathSurface, scope, source string) string {
	if strings.TrimSpace(pathSurface) == "" {
		pathSurface = "."
	}
	fields := []string{
		`"path": ` + strconv.Quote(pathSurface),
		`"view": "relation_map"`,
		`"relation_kinds": ["call", "import", "inheritance", "reference"]`,
		`"top_n": 24`,
	}
	if strings.TrimSpace(source) != "" {
		fields = append(fields, `"sources": [`+strconv.Quote(source)+`]`)
	}
	if strings.TrimSpace(scope) != "" {
		fields = append(fields, `"scope": `+strconv.Quote(scope))
	}
	return "repo_map {" + strings.Join(fields, ", ") + "}"
}

func sourceInventoryDiscoveryAttributeRoles(obs sourceInventoryDiscoveryObservation) []types.AnswerCandidateRole {
	if roles := sourceInventoryNormalizeLensRoles(obs.AttrRoles); len(roles) > 0 {
		return roles
	}
	if roles := sourceInventoryNormalizeLensRoles(obs.Roles); len(roles) > 0 {
		return roles
	}
	return []types.AnswerCandidateRole{
		types.AnswerCandidateRoleFunction,
		types.AnswerCandidateRoleMethod,
		types.AnswerCandidateRoleType,
		types.AnswerCandidateRoleConfigKey,
		types.AnswerCandidateRoleRoute,
	}
}

func sourceInventoryDiscoveryExpandRoles(obs sourceInventoryDiscoveryObservation, attrRoles []types.AnswerCandidateRole) []types.AnswerCandidateRole {
	if roles := sourceInventoryNormalizeLensRoles(obs.Roles); len(roles) > 0 {
		return roles
	}
	if len(attrRoles) > 0 {
		return attrRoles
	}
	return []types.AnswerCandidateRole{
		types.AnswerCandidateRoleFunction,
		types.AnswerCandidateRoleMethod,
		types.AnswerCandidateRoleType,
		types.AnswerCandidateRoleConfigKey,
		types.AnswerCandidateRoleRoute,
	}
}

func sourceInventoryAdvisoryAppendProvenance(items []string, extra ...string) []string {
	out := append([]string(nil), items...)
	seen := make(map[string]bool, len(out)+len(extra))
	for _, item := range out {
		if key := strings.TrimSpace(item); key != "" {
			seen[key] = true
		}
	}
	for _, item := range extra {
		key := strings.TrimSpace(item)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func sourceInventoryStringSliceContains(items []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == want {
			return true
		}
	}
	return false
}

func sourceInventoryReadFilePathMissHint(ctx *types.BusContext, requested string) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	observation := ctx.Mutable.SourceInventoryObservation()
	if !observation.IsActive() {
		return ""
	}
	requestedRel, ok := sourceInventoryReadFileRequestedRel(ctx, requested)
	if !ok || requestedRel == "" {
		return ""
	}
	requestedDir := path.Dir(requestedRel)
	if requestedDir == "." && strings.Contains(requestedRel, "/") {
		requestedDir = path.Dir(strings.Trim(requestedRel, "/"))
	}
	groups := sourceInventorySuggestedFileGroups(observation, types.SourceInventoryLensQuery{})
	if len(groups) == 0 {
		return ""
	}
	files := sourceInventorySuggestedFilesForRequestedScope(groups, requestedDir, requestedRel)
	if len(files) == 0 {
		return ""
	}
	const (
		maxFiles = 5
		maxItems = 3
	)
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}
	parts := make([]string, 0, len(files))
	for _, file := range files {
		if part := renderSourceInventorySuggestedFile(file, maxItems); part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	scope := requestedDir
	if scope == "." || scope == "" {
		scope = "repo root"
	} else {
		scope = "`" + scope + "`"
	}
	return fmt.Sprintf("\n\nSource-inventory hint (advisory): the requested path is missing. Known candidate files in the same scope %s: %s. These are repo-map navigation suggestions, not a whitelist; read the relevant file before citing.", scope, strings.Join(parts, "; "))
}

func sourceInventoryReadFileRequestedRel(ctx *types.BusContext, requested string) (string, bool) {
	raw := strings.TrimSpace(strings.ReplaceAll(requested, `\`, `/`))
	if raw == "" {
		return "", false
	}
	if normalized, ok := normalizeWindowsPOSIXPath(raw); ok {
		raw = filepath.ToSlash(normalized)
	}
	if toolPathIsAbs(raw) {
		if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
			return "", false
		}
		root := filepath.Clean(ctx.RepoRoot)
		abs := filepath.Clean(raw)
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return "", false
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			return "", false
		}
		return rel, true
	}
	cleaned := path.Clean(raw)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	if ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		parts := strings.Split(cleaned, "/")
		repoLabel := filepath.Base(filepath.Clean(ctx.RepoRoot))
		if len(parts) > 1 && repoLabel != "" && parts[0] == repoLabel {
			cleaned = strings.Join(parts[1:], "/")
		}
	}
	if cleaned == "" || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return strings.Trim(cleaned, "/"), true
}

func buildSourceInventoryAdvisory(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) types.SourceInventoryAdvisory {
	return buildSourceInventoryAdvisoryWithQuery(ctx, facts, types.SourceInventoryLensQuery{}, false, nil)
}

func buildSourceInventoryAdvisoryForLens(ctx *types.BusContext, query types.SourceInventoryLensQuery, advisoryOnly bool, provenance []string) types.SourceInventoryAdvisory {
	return buildSourceInventoryAdvisoryWithQuery(ctx, nil, query, advisoryOnly, provenance)
}

func buildSourceInventoryAdvisoryWithQuery(ctx *types.BusContext, facts []types.AnswerAggregateFact, query types.SourceInventoryLensQuery, forceAdvisoryOnly bool, extraProvenance []string) types.SourceInventoryAdvisory {
	if ctx == nil || ctx.Mutable == nil {
		return types.SourceInventoryAdvisory{}
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil {
		return types.SourceInventoryAdvisory{}
	}
	var (
		profile      *types.SourceInventoryProfile
		advisoryOnly bool
		provenance   []string
	)
	if ctx.AnalysisIR != nil {
		profile, advisoryOnly, provenance = sourceInventoryAdvisoryProfile(ctx, graph, query.Query)
	}
	if forceAdvisoryOnly {
		advisoryOnly = true
	}
	provenance = sourceInventoryAdvisoryAppendProvenance(provenance, extraProvenance...)
	if roles := sourceInventoryLensQueryRoles(query); len(roles) > 0 {
		profile = sourceInventoryProfileWithLensRoles(profile, roles)
		advisoryOnly = true
		provenance = sourceInventoryAdvisoryAppendProvenance(provenance, "repo_lens:roles")
	}
	attributeRoles := sourceInventoryLensQueryAttributeRoles(query)
	explicitAttributeRoles := len(attributeRoles) > 0
	includeAttributes := query.IncludeAttributes || !forceAdvisoryOnly
	if forceAdvisoryOnly && sourceInventoryExplicitPackageProfile(ctx, profile) {
		includeAttributes = true
	}
	if explicitAttributeRoles {
		advisoryOnly = true
		provenance = sourceInventoryAdvisoryAppendProvenance(provenance, "repo_lens:attribute_roles")
	}
	if !profile.Active() {
		return types.SourceInventoryAdvisory{}
	}
	var scopes []string
	if ctx.AnalysisIR != nil {
		scopes = sourceInventoryScopes(ctx, graph, facts)
	}
	if lensScopes := sourceInventoryLensQueryScopes(ctx, graph, query); len(lensScopes) > 0 {
		scopes = lensScopes
		advisoryOnly = true
		provenance = sourceInventoryAdvisoryAppendProvenance(provenance, "repo_lens:scopes")
	}
	scopes = sourceInventoryDedupeScopeAliases(scopes)
	if len(scopes) == 0 {
		if sourceInventoryCanUseQueryRootScope(ctx, query.Query) {
			scopes = []string{"."}
			advisoryOnly = true
			provenance = sourceInventoryAdvisoryAppendProvenance(provenance, "request_traits:query_root_scope")
		}
	}
	if len(scopes) == 0 {
		return types.SourceInventoryAdvisory{}
	}
	candidateBudget := sourceInventoryExecBudgetForLens(ctx, query, forceAdvisoryOnly, graph)
	execView := newSourceInventoryExecutionViewWithBudget(graph, scopes, candidateBudget)
	if queryProfile, changed := sourceInventoryProfileWithQueryMatchedRoles(ctx, graph, scopes, profile, query.Query); changed {
		profile = queryProfile
		provenance = sourceInventoryAdvisoryAppendProvenance(provenance, "repo_lens:query_roles")
	}
	if includeAttributes && forceAdvisoryOnly && sourceInventoryShouldDeferAttributesForBroadToolLensView(execView) {
		includeAttributes = false
		provenance = sourceInventoryAdvisoryAppendProvenance(provenance, "repo_lens:attributes_deferred_broad_scope")
	}
	sets := sourceInventoryCandidateSets(ctx, graph, execView, scopes, profile, attributeRoles, explicitAttributeRoles, includeAttributes, query.Query, candidateBudget)
	if len(sets) == 0 {
		return types.SourceInventoryAdvisory{}
	}
	if sourceInventoryCandidateSetsTruncated(sets) {
		provenance = sourceInventoryAdvisoryAppendProvenance(provenance, "repo_lens:candidate_budget_truncated")
	}
	roles := sourceInventoryRoleOrder(profile, sets)
	advisory := types.SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: advisoryOnly,
		Complete:     true,
		Scopes:       append([]string(nil), scopes...),
		Provenance:   sourceInventoryAdvisoryProvenance(ctx, provenance),
	}
	for _, role := range roles {
		set := sets[role]
		if len(set.candidates) == 0 && !set.truncated {
			continue
		}
		if !set.complete {
			advisory.Complete = false
		}
		advisorySet := types.SourceInventoryAdvisorySet{
			Role:       set.role,
			Complete:   set.complete,
			Total:      set.total,
			Candidates: make([]types.SourceInventoryAdvisoryCandidate, 0, len(set.candidates)),
		}
		if advisorySet.Total < len(set.candidates) {
			advisorySet.Total = len(set.candidates)
		}
		for _, candidate := range set.candidates {
			attrs := make([]types.SourceInventoryAdvisoryAttribute, 0, len(candidate.attributes))
			for _, attr := range candidate.attributes {
				attrs = append(attrs, types.SourceInventoryAdvisoryAttribute{
					Member:       attr.member,
					Key:          attr.key,
					SupportRef:   attr.supportRef,
					Note:         sourceInventoryCandidateNote(attr),
					SurfaceTerms: sourceInventoryCandidateSurfaceTerms(attr),
					Role:         attr.role,
					Exported:     attr.exported,
					File:         attr.file,
					Line:         attr.line,
					Language:     attr.language,
				})
			}
			advisorySet.Candidates = append(advisorySet.Candidates, types.SourceInventoryAdvisoryCandidate{
				Member:       candidate.member,
				Key:          candidate.key,
				SupportRef:   candidate.supportRef,
				Note:         sourceInventoryCandidateNote(candidate),
				SurfaceTerms: sourceInventoryCandidateSurfaceTerms(candidate),
				Role:         candidate.role,
				Exported:     candidate.exported,
				File:         candidate.file,
				Line:         candidate.line,
				Language:     candidate.language,
				Attributes:   attrs,
			})
		}
		advisory.Sets = append(advisory.Sets, advisorySet)
	}
	if len(advisory.Sets) == 0 {
		return types.SourceInventoryAdvisory{}
	}
	return advisory
}

func sourceInventoryLensQueryRoles(query types.SourceInventoryLensQuery) []types.AnswerCandidateRole {
	return sourceInventoryNormalizeLensRoles(query.Roles)
}

func sourceInventoryLensQueryAttributeRoles(query types.SourceInventoryLensQuery) []types.AnswerCandidateRole {
	return sourceInventoryNormalizeLensRoles(query.AttributeRoles)
}

func sourceInventoryNormalizeLensRoles(rawRoles []types.AnswerCandidateRole) []types.AnswerCandidateRole {
	seen := map[types.AnswerCandidateRole]bool{}
	var roles []types.AnswerCandidateRole
	for _, raw := range rawRoles {
		role, ok := types.NormalizeAnswerCandidateRole(string(raw))
		if !ok || role == types.AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		switch role {
		case types.AnswerCandidateRoleFunction,
			types.AnswerCandidateRoleMethod,
			types.AnswerCandidateRoleType,
			types.AnswerCandidateRoleConstant,
			types.AnswerCandidateRoleVariable,
			types.AnswerCandidateRoleField,
			types.AnswerCandidateRolePackage,
			types.AnswerCandidateRoleFile,
			types.AnswerCandidateRoleConfigFile,
			types.AnswerCandidateRoleConfigKey,
			types.AnswerCandidateRoleRoute,
			types.AnswerCandidateRoleImportPath,
			types.AnswerCandidateRoleLiteralValue:
			seen[role] = true
			roles = append(roles, role)
		}
	}
	return roles
}

func sourceInventoryProfileWithLensRoles(profile *types.SourceInventoryProfile, roles []types.AnswerCandidateRole) *types.SourceInventoryProfile {
	if len(roles) == 0 {
		return profile
	}
	if profile == nil {
		return &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       append([]types.AnswerCandidateRole(nil), roles...),
			RequestedFields: []types.SourceInventoryRequestedField{
				types.SourceInventoryFieldName,
				types.SourceInventoryFieldLocation,
				types.SourceInventoryFieldSummary,
			},
			Confidence: 0.50,
		}
	}
	clone := *profile
	clone.IsSourceInventory = true
	clone.TargetRoles = append([]types.AnswerCandidateRole(nil), roles...)
	clone.RequestedFields = append([]types.SourceInventoryRequestedField(nil), profile.RequestedFields...)
	if len(clone.RequestedFields) == 0 {
		clone.RequestedFields = []types.SourceInventoryRequestedField{
			types.SourceInventoryFieldName,
			types.SourceInventoryFieldLocation,
			types.SourceInventoryFieldSummary,
		}
	}
	clone.SourceQuotes = append([]string(nil), profile.SourceQuotes...)
	if clone.Confidence <= 0 {
		clone.Confidence = 0.50
	}
	return &clone
}

func sourceInventoryExplicitPackageProfile(ctx *types.BusContext, profile *types.SourceInventoryProfile) bool {
	if ctx == nil || ctx.AnalysisIR == nil || profile == nil || !profile.Active() {
		return false
	}
	if ctx.AnalysisIR.RequestModel.SourceInventoryProfile == nil || !ctx.AnalysisIR.RequestModel.SourceInventoryProfile.Active() {
		return false
	}
	for _, role := range profile.PrincipalTargetRoles() {
		if role == types.AnswerCandidateRolePackage {
			return true
		}
	}
	return false
}

func sourceInventoryProfileWithQueryMatchedRoles(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile, rawQuery string) (*types.SourceInventoryProfile, bool) {
	if profile == nil || !profile.Active() || graph == nil || strings.TrimSpace(rawQuery) == "" {
		return profile, false
	}
	filter := sourceInventoryBuildQueryFilter(rawQuery)
	if !filter.Active() {
		return profile, false
	}
	seen := map[types.AnswerCandidateRole]bool{}
	var merged []types.AnswerCandidateRole
	for _, role := range profile.TargetRoles {
		if role == types.AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		merged = append(merged, role)
	}
	index := newSourceInventoryGraphSymbolIndex(graph)
	if index == nil {
		return profile, false
	}
	scopeFilter := newSourceInventoryScopeFilter(ctx)
	for _, sym := range index.all {
		if sym == nil ||
			!sourceInventoryFileInScopes(sym.File, scopes) ||
			!scopeFilter.SourceInRequestedScope(sym.File) ||
			!sourceInventorySymbolMatchesQuery(sym, graph, filter) {
			continue
		}
		role, ok := aggregateAnswerCandidateRoleForSymbol(sym)
		if !ok || role == types.AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		merged = append(merged, role)
	}
	if len(merged) == len(profile.TargetRoles) {
		return profile, false
	}
	clone := *profile
	clone.TargetRoles = merged
	clone.RequestedFields = append([]types.SourceInventoryRequestedField(nil), profile.RequestedFields...)
	clone.SourceQuotes = append([]string(nil), profile.SourceQuotes...)
	if clone.Confidence <= 0 || clone.Confidence > 0.75 {
		clone.Confidence = 0.75
	}
	return &clone, true
}

func sourceInventoryLensQueryScopes(ctx *types.BusContext, graph *repotypes.Graph, query types.SourceInventoryLensQuery) []string {
	if len(query.Scopes) == 0 {
		if scope := sourceInventoryScopeForLensSurfaceWithPath(graph, query.Path, query.Path); scope != "" {
			return []string{scope}
		}
		return nil
	}
	scopeFilter := newSourceInventoryScopeFilter(ctx)
	seen := map[string]bool{}
	var scopes []string
	for _, raw := range query.Scopes {
		scope := sourceInventoryScopeForLensSurfaceWithPath(graph, query.Path, raw)
		if scope == "" || seen[scope] {
			continue
		}
		// "." is a repo-lens selector for the active repo root, not an answer
		// evidence path. Keep the per-file source-scope checks at candidate
		// construction time so tests/docs/config rows are still filtered by the
		// typed source scope instead of by this synthetic selector.
		if scope != "." && ctx != nil && !scopeFilter.SourceInRequestedScope(scope) {
			continue
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}
	return sourceInventoryDedupeScopeAliases(scopes)
}

func sourceInventoryScopeForLensSurfaceWithPath(graph *repotypes.Graph, queryPath, raw string) string {
	base, okBase := sourceInventoryLensSafeRelativePath(queryPath)
	candidate, okCandidate := sourceInventoryLensSafeRelativePath(raw)
	if okBase && okCandidate && base != "" && base != "." && candidate != "" {
		switch {
		case candidate == base:
			return sourceInventoryScopeForLensSurface(graph, ".")
		case strings.HasPrefix(candidate, base+"/"):
			if scope := sourceInventoryScopeForLensSurface(graph, strings.TrimPrefix(candidate, base+"/")); scope != "" {
				return scope
			}
		}
	}
	if scope := sourceInventoryScopeForLensSurface(graph, raw); scope != "" {
		return scope
	}
	return ""
}

func sourceInventoryDedupeScopeAliases(scopes []string) []string {
	if len(scopes) <= 1 {
		return scopes
	}
	normalized := make([]string, 0, len(scopes))
	seen := map[string]bool{}
	for _, raw := range scopes {
		rawScope := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
		if rawScope == "." || rawScope == "./" || rawScope == "/" {
			if !seen["."] {
				seen["."] = true
				normalized = append(normalized, ".")
			}
			continue
		}
		scope := normalizeSourceInventoryScopeSurface(raw)
		if scope == "" {
			continue
		}
		if !seen[scope] {
			seen[scope] = true
			normalized = append(normalized, scope)
		}
	}
	if len(normalized) <= 1 {
		return normalized
	}
	hasLongAlias := map[string]bool{}
	for _, scope := range normalized {
		if !strings.Contains(scope, "/") {
			continue
		}
		base := path.Base(scope)
		if base != "." && base != "/" && base != "" {
			hasLongAlias[base] = true
		}
	}
	out := make([]string, 0, len(normalized))
	for _, scope := range normalized {
		if !strings.Contains(scope, "/") && hasLongAlias[scope] {
			continue
		}
		out = append(out, scope)
	}
	if len(out) == 0 {
		return normalized
	}
	return out
}

func sourceInventoryScopeForLensSurface(graph *repotypes.Graph, raw string) string {
	surface := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	if surface == "." || surface == "./" || surface == "/" {
		if graph != nil && len(graph.Files) > 0 {
			return "."
		}
	}
	return sourceInventoryScopeForSurface(graph, raw)
}

func sourceInventoryRoleOrder(profile *types.SourceInventoryProfile, sets map[types.AnswerCandidateRole]sourceInventoryCandidateSet) []types.AnswerCandidateRole {
	seen := map[types.AnswerCandidateRole]bool{}
	var roles []types.AnswerCandidateRole
	if profile != nil {
		for _, role := range profile.PrincipalTargetRoles() {
			if _, ok := sets[role]; ok && !seen[role] {
				seen[role] = true
				roles = append(roles, role)
			}
		}
	}
	var rest []types.AnswerCandidateRole
	for role := range sets {
		if !seen[role] {
			rest = append(rest, role)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return string(rest[i]) < string(rest[j]) })
	return append(roles, rest...)
}

func sourceInventoryAdvisoryProfile(ctx *types.BusContext, graph *repotypes.Graph, rawQuery string) (*types.SourceInventoryProfile, bool, []string) {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil, false, nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if profile := rm.SourceInventoryProfile; profile != nil && profile.Active() {
		clone := *profile
		clone.TargetRoles = append([]types.AnswerCandidateRole(nil), profile.TargetRoles...)
		clone.RequestedFields = append([]types.SourceInventoryRequestedField(nil), profile.RequestedFields...)
		clone.SourceQuotes = append([]string(nil), profile.SourceQuotes...)
		if profile.Confidence < 0.70 {
			return &clone, true, []string{"source_inventory_profile:low_confidence"}
		}
		return &clone, false, []string{"source_inventory_profile"}
	}
	var roleProfileRoles []types.AnswerCandidateRole
	if rm.AnswerRoleProfile != nil && rm.AnswerRoleProfile.Active() {
		roleProfileRoles = sourceInventoryAdvisoryPrincipalRolesFromRoleProfile(rm.AnswerRoleProfile)
		var provenance []string
		switch {
		case types.HasAttributeBearingEnumeration(rm):
			provenance = append(provenance, "request_traits:attribute_bearing_enumeration")
		case types.RequiresExhaustiveEnumerationMemberSetHandoff(rm):
			provenance = append(provenance, "request_traits:exhaustive_member_set")
		default:
			provenance = nil
		}
		if len(provenance) > 0 && len(roleProfileRoles) > 0 {
			provenance = append(provenance, "answer_role_profile")
			return &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       roleProfileRoles,
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
					types.SourceInventoryFieldSummary,
				},
				Confidence: 0.50,
			}, true, provenance
		}
	}
	if types.IsTypedSourceEnumerationShape(rm) && sourceInventoryBuildQueryFilter(rawQuery).Active() {
		roles := roleProfileRoles
		if len(roles) == 0 {
			roles = sourceInventoryDefaultQueryEnumerationRoles()
		}
		return &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       roles,
			RequestedFields: []types.SourceInventoryRequestedField{
				types.SourceInventoryFieldName,
				types.SourceInventoryFieldLocation,
				types.SourceInventoryFieldSummary,
			},
			Confidence: 0.45,
		}, true, []string{"request_traits:typed_source_enumeration_query"}
	}
	if types.HasBoundedSourceEnumerationScope(rm, ctx.AnalysisIR.EvidencePlan.RequiredFiles, ctx.RepoRoot) {
		roles := roleProfileRoles
		if len(roles) == 0 {
			roles = []types.AnswerCandidateRole{types.AnswerCandidateRolePackage}
		}
		return &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       roles,
			RequestedFields: []types.SourceInventoryRequestedField{
				types.SourceInventoryFieldName,
				types.SourceInventoryFieldLocation,
				types.SourceInventoryFieldSummary,
			},
			Confidence: 0.45,
		}, true, []string{"request_traits:bounded_source_enumeration_scope"}
	}
	if sourceInventoryCanUseScopeEntityFallback(rm) && len(sourceInventoryRequestedScopes(ctx, graph)) > 0 {
		roles := roleProfileRoles
		if len(roles) == 0 {
			roles = []types.AnswerCandidateRole{types.AnswerCandidateRolePackage}
		}
		return &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       roles,
			RequestedFields: []types.SourceInventoryRequestedField{
				types.SourceInventoryFieldName,
				types.SourceInventoryFieldLocation,
				types.SourceInventoryFieldSummary,
			},
			Confidence: 0.40,
		}, true, []string{"request_traits:source_scope_enumeration"}
	}
	return nil, false, nil
}

func sourceInventoryDefaultQueryEnumerationRoles() []types.AnswerCandidateRole {
	return []types.AnswerCandidateRole{
		types.AnswerCandidateRoleFunction,
		types.AnswerCandidateRoleMethod,
		types.AnswerCandidateRoleType,
		types.AnswerCandidateRoleConstant,
		types.AnswerCandidateRoleVariable,
		types.AnswerCandidateRoleField,
		types.AnswerCandidateRoleRoute,
		types.AnswerCandidateRoleConfigKey,
		types.AnswerCandidateRoleImportPath,
	}
}

func sourceInventoryCanUseQueryRootScope(ctx *types.BusContext, rawQuery string) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if !types.IsTypedSourceEnumerationShape(ctx.AnalysisIR.RequestModel) {
		return false
	}
	return sourceInventoryBuildQueryFilter(rawQuery).Active()
}

func sourceInventoryCanUseScopeEntityFallback(rm types.RequestModel) bool {
	if !types.IsTypedSourceEnumerationShape(rm) {
		return false
	}
	if rm.CompletenessObligation != nil && rm.CompletenessObligation.IsActive() {
		return false
	}
	return true
}

func sourceInventoryAdvisoryPrincipalRolesFromRoleProfile(profile *types.AnswerRoleProfile) []types.AnswerCandidateRole {
	if profile == nil || !profile.Active() {
		return nil
	}
	seen := map[types.AnswerCandidateRole]bool{}
	var roles []types.AnswerCandidateRole
	for _, role := range profile.RequiredCandidateRoles {
		if role == types.AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		switch role {
		case types.AnswerCandidateRoleFunction,
			types.AnswerCandidateRoleMethod,
			types.AnswerCandidateRoleType,
			types.AnswerCandidateRoleConstant,
			types.AnswerCandidateRoleVariable,
			types.AnswerCandidateRoleField,
			types.AnswerCandidateRolePackage,
			types.AnswerCandidateRoleFile,
			types.AnswerCandidateRoleConfigFile,
			types.AnswerCandidateRoleConfigKey,
			types.AnswerCandidateRoleRoute,
			types.AnswerCandidateRoleImportPath:
			seen[role] = true
			roles = append(roles, role)
		}
	}
	return roles
}

func sourceInventoryAdvisoryProvenance(ctx *types.BusContext, base []string) []string {
	out := append([]string(nil), base...)
	out = append(out, "repomap_graph")
	if sourceInventoryAdvisoryHasListFilesScope(ctx) {
		out = append(out, "tool:list_files")
	}
	return out
}

func sourceInventoryAdvisoryHasListFilesScope(ctx *types.BusContext) bool {
	for _, result := range aggregateSupportToolResults(ctx) {
		if result.Success && result.ToolName == "list_files" && sourceInventoryListFilesScope(result.Summary) != "" {
			return true
		}
	}
	return false
}

func reconcileCompletionAggregateFactsWithSourceInventory(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) []types.AnswerAggregateFact {
	if len(facts) == 0 || ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return facts
	}
	profile := ctx.AnalysisIR.RequestModel.SourceInventoryProfile
	if !profile.Active() || profile.Confidence < 0.70 {
		return facts
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil {
		return facts
	}
	scopes := sourceInventoryScopes(ctx, graph, facts)
	if len(scopes) == 0 {
		return facts
	}
	supportBudget := sourceInventoryExecBudgetForSupport(ctx, graph)
	sets := sourceInventoryCandidateSets(ctx, graph, newSourceInventoryExecutionViewWithBudget(graph, scopes, supportBudget), scopes, profile, nil, false, false, "", supportBudget)
	if len(sets) == 0 {
		return facts
	}
	families := newAggregateDeclarationFamilyLookup(ctx)
	out := cloneCompletionAggregateFacts(facts)
	changed := false
	for i := range out {
		if !types.AnswerAggregateFactCarriesCompleteMemberSet(out[i]) ||
			types.AnswerAggregateFactRoleForRequest(out[i], &ctx.AnalysisIR.RequestModel) != types.AnswerAggregateRolePrincipalAnswer ||
			len(out[i].Members) == 0 {
			continue
		}
		if sourceInventoryMustNotRewriteRelationMemberSet(ctx, out[i]) {
			continue
		}
		role, ok := sourceInventoryFactRole(ctx, out[i], evidence, profile, families)
		if !ok || !profile.RequiresPrincipalRole(role) {
			continue
		}
		set, ok := sets[role]
		if !ok || len(set.candidates) == 0 || !set.complete {
			continue
		}
		if !sourceInventoryMayRewriteMemberSet(profile, role) {
			continue
		}
		if !sourceInventoryShouldReplaceMemberSet(profile, role) {
			oldLen := len(out[i].Members)
			if sourceInventoryAppendMissingCandidates(&out[i], set.candidates) {
				reconcileAdjacentAggregateCountWithMemberSet(out, i, oldLen, len(out[i].Members))
				changed = true
			}
			continue
		}
		if sourceInventoryReplaceMemberSet(&out[i], set.candidates) {
			reconcileAdjacentAggregateCountWithMemberSet(out, i, len(facts[i].Members), len(out[i].Members))
			changed = true
		}
	}
	if !changed {
		return facts
	}
	return out
}

func sourceInventoryMustNotRewriteRelationMemberSet(ctx *types.BusContext, fact types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if !types.HasTypedRelationMemberSetShape(ctx.AnalysisIR.RequestModel) {
		return false
	}
	// In relation-shaped questions the model-authored member_set is the
	// relation carrier. Source inventory remains useful evidence, but replacing
	// or appending principal members from "all symbols in the scoped file" can
	// turn precise implementers/callees into unrelated neighboring definitions.
	return types.AnswerAggregateFactRoleForRequest(fact, &ctx.AnalysisIR.RequestModel) == types.AnswerAggregateRolePrincipalAnswer
}

func sourceInventoryFactRole(ctx *types.BusContext, fact types.AnswerAggregateFact, evidence []types.EvidenceItem, profile *types.SourceInventoryProfile, families *aggregateDeclarationFamilyLookup) (types.AnswerCandidateRole, bool) {
	if profile != nil {
		principalRoles := profile.PrincipalTargetRoles()
		if len(principalRoles) == 1 {
			return principalRoles[0], true
		}
	}
	p, ok := aggregateMemberSetDefinitionProfile(ctx, fact, evidence, families)
	if !ok {
		return types.AnswerCandidateRoleUnknown, false
	}
	return p.role, true
}

func sourceInventoryMayRewriteMemberSet(profile *types.SourceInventoryProfile, role types.AnswerCandidateRole) bool {
	if profile == nil {
		return false
	}
	// The only source-inventory rewrite that is precise enough to alter the
	// model-authored member set is the language-structural "public string enum
	// type backed by const set" shape. Generic function/method/type inventories
	// can be useful evidence, but automatically replacing or appending their
	// full graph candidate set can broaden requests such as "one entry function
	// per package" into "all functions in scope". That violates the model/user
	// intent boundary, so those candidates stay as support unless the model
	// itself emits them.
	return role == types.AnswerCandidateRoleType &&
		profile.TypeUnderlying == types.SourceInventoryTypeUnderlyingString &&
		profile.RequiresConstSet
}

func sourceInventoryShouldReplaceMemberSet(profile *types.SourceInventoryProfile, role types.AnswerCandidateRole) bool {
	if profile == nil {
		return false
	}
	if role == types.AnswerCandidateRoleType &&
		profile.TypeUnderlying == types.SourceInventoryTypeUnderlyingString &&
		profile.RequiresConstSet {
		return true
	}
	return false
}

func sourceInventoryReplaceMemberSet(fact *types.AnswerAggregateFact, candidates []sourceInventoryCandidate) bool {
	if fact == nil || len(candidates) == 0 {
		return false
	}
	existingByKey := sourceInventoryExistingMemberCarriers(*fact)
	members := make([]string, 0, len(candidates))
	refs := make([]string, 0, len(candidates))
	notes := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		existing := existingByKey[candidate.key]
		supportRef := candidate.supportRef
		if strings.TrimSpace(supportRef) == "" {
			supportRef = existing.supportRef
		}
		note := sourceInventoryCandidateNote(candidate)
		if strings.TrimSpace(existing.note) != "" {
			note = types.MergeEvidenceSummaries(existing.note, note)
		}
		members = append(members, candidate.member)
		refs = append(refs, supportRef)
		notes = append(notes, note)
	}
	notes = trimTrailingEmptyStrings(notes)
	if stringSlicesEqual(fact.Members, members) && stringSlicesEqual(fact.SupportRefs, refs) && stringSlicesEqual(fact.MemberNotes, notes) {
		return false
	}
	fact.Members = members
	fact.SupportRefs = refs
	fact.MemberNotes = notes
	fact.Value = strconvItoa(len(members))
	fact.Provenance = sourceInventoryAppendProvenance(fact.Provenance)
	return true
}

type sourceInventoryExistingMemberCarrier struct {
	note       string
	supportRef string
}

func sourceInventoryExistingMemberCarriers(fact types.AnswerAggregateFact) map[string]sourceInventoryExistingMemberCarrier {
	out := map[string]sourceInventoryExistingMemberCarrier{}
	for idx, member := range fact.Members {
		key := aggregateMemberKey(member)
		if key == "" {
			continue
		}
		existing := out[key]
		if idx < len(fact.MemberNotes) {
			existing.note = types.MergeEvidenceSummaries(existing.note, fact.MemberNotes[idx])
		}
		if strings.TrimSpace(existing.supportRef) == "" && idx < len(fact.SupportRefs) {
			existing.supportRef = strings.TrimSpace(fact.SupportRefs[idx])
		}
		out[key] = existing
	}
	return out
}

func sourceInventoryAppendMissingCandidates(fact *types.AnswerAggregateFact, candidates []sourceInventoryCandidate) bool {
	if fact == nil || len(candidates) == 0 {
		return false
	}
	existing := make(map[string]bool, len(fact.Members))
	for _, member := range fact.Members {
		if key := aggregateMemberKey(member); key != "" {
			existing[key] = true
		}
	}
	changed := false
	for _, candidate := range candidates {
		if candidate.key == "" || existing[candidate.key] {
			continue
		}
		fact.Members = append(fact.Members, candidate.member)
		fact.SupportRefs = append(fact.SupportRefs, candidate.supportRef)
		if note := sourceInventoryCandidateNote(candidate); note != "" {
			fact.MemberNotes = appendStringAtIndex(fact.MemberNotes, len(fact.Members)-1, note)
		}
		existing[candidate.key] = true
		changed = true
	}
	if changed {
		fact.Value = strconvItoa(len(fact.Members))
		fact.Provenance = sourceInventoryAppendProvenance(fact.Provenance)
	}
	return changed
}

func sourceInventoryAppendProvenance(current string) string {
	current = strings.TrimSpace(current)
	const marker = "system:source_inventory"
	if current == "" {
		return marker
	}
	for _, part := range strings.Split(current, ",") {
		if strings.EqualFold(strings.TrimSpace(part), marker) {
			return current
		}
	}
	return current + ", " + marker
}

func sourceInventoryMarkCandidateBudgetTruncated(set *sourceInventoryCandidateSet) {
	if set == nil {
		return
	}
	set.complete = false
	set.truncated = true
}

func sourceInventoryCandidateSetsTruncated(sets map[types.AnswerCandidateRole]sourceInventoryCandidateSet) bool {
	for _, set := range sets {
		if set.truncated {
			return true
		}
	}
	return false
}

func sourceInventoryCandidateSets(ctx *types.BusContext, graph *repotypes.Graph, view *sourceInventoryExecutionView, scopes []string, profile *types.SourceInventoryProfile, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool, includeAttributes bool, rawQuery string, budget sourceInventoryExecBudget) map[types.AnswerCandidateRole]sourceInventoryCandidateSet {
	out := map[types.AnswerCandidateRole]sourceInventoryCandidateSet{}
	if view == nil {
		view = newSourceInventoryExecutionView(graph, scopes)
	}
	scopeFilter := newSourceInventoryScopeFilter(ctx)
	queryFilter := sourceInventoryBuildQueryFilter(rawQuery)
	var symbolIndex *sourceInventoryGraphSymbolIndex
	getSymbolIndex := func() *sourceInventoryGraphSymbolIndex {
		if symbolIndex == nil {
			symbolIndex = newSourceInventoryGraphSymbolIndex(graph)
		}
		return symbolIndex
	}
	for _, role := range profile.PrincipalTargetRoles() {
		switch {
		case role == types.AnswerCandidateRoleFile:
			var attributeIndex *sourceInventoryGraphSymbolIndex
			if includeAttributes && explicitAttributeRoles {
				attributeIndex = getSymbolIndex()
			}
			out[role] = sourceInventoryFileCandidates(ctx, view, attributeIndex, scopeFilter, profile, attributeRoles, explicitAttributeRoles, queryFilter, budget)
		case role == types.AnswerCandidateRoleConfigFile:
			var attributeIndex *sourceInventoryGraphSymbolIndex
			if includeAttributes && explicitAttributeRoles {
				attributeIndex = getSymbolIndex()
			}
			out[role] = sourceInventoryConfigFileCandidates(ctx, view, attributeIndex, scopeFilter, profile, attributeRoles, explicitAttributeRoles, queryFilter, budget)
		case role == types.AnswerCandidateRolePackage:
			var attributeIndex *sourceInventoryGraphSymbolIndex
			if includeAttributes {
				attributeIndex = getSymbolIndex()
			}
			out[role] = sourceInventoryPackageCandidates(ctx, view, attributeIndex, scopeFilter, profile, attributeRoles, explicitAttributeRoles, queryFilter, budget)
		case role == types.AnswerCandidateRoleType &&
			profile.TypeUnderlying == types.SourceInventoryTypeUnderlyingString &&
			profile.RequiresConstSet:
			out[role] = sourceInventoryGoStringEnumCandidates(ctx, graph, scopes, profile, budget)
		default:
			out[role] = sourceInventoryGraphCandidates(ctx, graph, view, getSymbolIndex(), scopeFilter, scopes, profile, role, queryFilter, budget)
		}
		if len(out[role].candidates) == 0 && !out[role].truncated {
			delete(out, role)
		}
	}
	return out
}

func newSourceInventoryCandidateSet(role types.AnswerCandidateRole, view *sourceInventoryExecutionView) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: role, complete: sourceInventoryViewComplete(view)}
	if sourceInventoryViewBudgetTruncated(view) {
		sourceInventoryMarkCandidateBudgetTruncated(&set)
	}
	return set
}

const sourceInventoryBroadToolLensAttributeFileThreshold = 128

func sourceInventoryShouldDeferAttributesForBroadToolLensView(view *sourceInventoryExecutionView) bool {
	if view == nil {
		return false
	}
	count := 0
	for _, fi := range view.Files("") {
		if fi == nil || strings.TrimSpace(fi.RelPath) == "" {
			continue
		}
		count++
		if count > sourceInventoryBroadToolLensAttributeFileThreshold {
			return true
		}
	}
	return false
}

func sourceInventoryFileCandidates(ctx *types.BusContext, view *sourceInventoryExecutionView, symbolIndex *sourceInventoryGraphSymbolIndex, scopeFilter sourceInventoryScopeFilter, profile *types.SourceInventoryProfile, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool, queryFilter sourceInventoryQueryFilter, budget sourceInventoryExecBudget) sourceInventoryCandidateSet {
	set := newSourceInventoryCandidateSet(types.AnswerCandidateRoleFile, view)
	if view == nil {
		return set
	}
	seen := map[string]bool{}
	scanned := 0
	for _, fi := range view.Files("") {
		if fi == nil || strings.TrimSpace(fi.RelPath) == "" || (strings.TrimSpace(fi.Language) == "" && !fi.IsSpecial) {
			continue
		}
		file := strings.Trim(strings.TrimSpace(strings.ReplaceAll(fi.RelPath, `\`, `/`)), "/")
		if file == "" || seen[file] || !scopeFilter.SourceInRequestedScope(file) {
			continue
		}
		if budget.scanExceeded(scanned) {
			sourceInventoryMarkCandidateBudgetTruncated(&set)
			break
		}
		scanned++
		seen[file] = true
		candidate := sourceInventoryCandidate{
			member:     file,
			key:        file,
			supportRef: file,
			note:       sourceInventoryFileCandidateNote(fi),
			role:       types.AnswerCandidateRoleFile,
			exported:   true,
			file:       file,
			language:   strings.TrimSpace(fi.Language),
		}
		before := len(set.candidates)
		stop := budget.appendCandidate(&set, candidate, queryFilter)
		if len(set.candidates) > before {
			set.candidates[len(set.candidates)-1].attributes = sourceInventoryFileAttributes(ctx, symbolIndex, scopeFilter, file, attributeRoles, explicitAttributeRoles)
		}
		if stop {
			break
		}
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

func sourceInventoryConfigFileCandidates(ctx *types.BusContext, view *sourceInventoryExecutionView, symbolIndex *sourceInventoryGraphSymbolIndex, scopeFilter sourceInventoryScopeFilter, profile *types.SourceInventoryProfile, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool, queryFilter sourceInventoryQueryFilter, budget sourceInventoryExecBudget) sourceInventoryCandidateSet {
	set := newSourceInventoryCandidateSet(types.AnswerCandidateRoleConfigFile, view)
	if view == nil {
		return set
	}
	seen := map[string]bool{}
	scanned := 0
	for _, fi := range view.Files("") {
		if fi == nil || !fi.IsSpecial || strings.TrimSpace(fi.RelPath) == "" {
			continue
		}
		file := strings.Trim(strings.TrimSpace(strings.ReplaceAll(fi.RelPath, `\`, `/`)), "/")
		if file == "" || seen[file] || !scopeFilter.SourceInRequestedScope(file) {
			continue
		}
		if budget.scanExceeded(scanned) {
			sourceInventoryMarkCandidateBudgetTruncated(&set)
			break
		}
		scanned++
		seen[file] = true
		candidate := sourceInventoryCandidate{
			member:     file,
			key:        file,
			supportRef: file,
			note:       sourceInventoryConfigFileCandidateNote(fi),
			role:       types.AnswerCandidateRoleConfigFile,
			exported:   true,
			file:       file,
			language:   strings.TrimSpace(fi.Language),
		}
		before := len(set.candidates)
		stop := budget.appendCandidate(&set, candidate, queryFilter)
		if len(set.candidates) > before {
			set.candidates[len(set.candidates)-1].attributes = sourceInventoryFileAttributes(ctx, symbolIndex, scopeFilter, file, attributeRoles, explicitAttributeRoles)
		}
		if stop {
			break
		}
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

func sourceInventoryPackageCandidates(ctx *types.BusContext, view *sourceInventoryExecutionView, symbolIndex *sourceInventoryGraphSymbolIndex, scopeFilter sourceInventoryScopeFilter, profile *types.SourceInventoryProfile, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool, queryFilter sourceInventoryQueryFilter, budget sourceInventoryExecBudget) sourceInventoryCandidateSet {
	set := newSourceInventoryCandidateSet(types.AnswerCandidateRolePackage, view)
	if view == nil {
		return set
	}
	buckets := map[string]*sourceInventoryPackageBucket{}
	scanned := 0
	for _, fi := range view.Files("") {
		if fi == nil || fi.IsSpecial || strings.TrimSpace(fi.RelPath) == "" || strings.TrimSpace(fi.Language) == "" {
			continue
		}
		if !scopeFilter.SourceInRequestedScope(fi.RelPath) {
			continue
		}
		if budget.scanExceeded(scanned) {
			sourceInventoryMarkCandidateBudgetTruncated(&set)
			break
		}
		scanned++
		dir := strings.Trim(path.Dir(strings.ReplaceAll(fi.RelPath, `\`, `/`)), "/")
		if dir == "." || dir == "" {
			dir = strings.Trim(strings.TrimSpace(fi.Package), "/")
		}
		if dir == "" {
			continue
		}
		bucket := buckets[dir]
		if bucket == nil {
			bucket = &sourceInventoryPackageBucket{
				dir:       dir,
				languages: map[string]int{},
				packages:  map[string]int{},
			}
			buckets[dir] = bucket
		}
		bucket.files++
		bucket.symbols += len(fi.Symbols)
		if lang := strings.TrimSpace(fi.Language); lang != "" {
			bucket.languages[lang]++
		}
		if pkg := strings.TrimSpace(fi.Package); pkg != "" {
			bucket.packages[pkg]++
		}
	}
	bucketKeys := make([]string, 0, len(buckets))
	for key := range buckets {
		bucketKeys = append(bucketKeys, key)
	}
	sort.Strings(bucketKeys)
	for _, key := range bucketKeys {
		bucket := buckets[key]
		member := sourceInventoryPackageBucketMember(bucket)
		candidateKey := strings.TrimSpace(bucket.dir)
		if member == "" || candidateKey == "" {
			continue
		}
		candidate := sourceInventoryCandidate{
			member:     member,
			key:        candidateKey,
			supportRef: candidateKey,
			note:       sourceInventoryPackageBucketNote(bucket),
			role:       types.AnswerCandidateRolePackage,
			exported:   true,
			file:       candidateKey,
			language:   sourceInventoryDominantMapKey(bucket.languages),
		}
		before := len(set.candidates)
		stop := budget.appendCandidate(&set, candidate, queryFilter)
		if len(set.candidates) > before {
			set.candidates[len(set.candidates)-1].attributes = sourceInventoryPackageBucketAttributes(ctx, symbolIndex, scopeFilter, candidateKey, attributeRoles, explicitAttributeRoles)
		}
		if stop {
			break
		}
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

const (
	sourceInventoryMaxDefaultAttributes  = 4
	sourceInventoryMaxExplicitAttributes = 8
)

func sourceInventoryPackageBucketAttributes(ctx *types.BusContext, symbolIndex *sourceInventoryGraphSymbolIndex, scopeFilter sourceInventoryScopeFilter, dir string, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool) []sourceInventoryCandidate {
	if symbolIndex == nil {
		return nil
	}
	dir = strings.Trim(strings.TrimSpace(strings.ReplaceAll(dir, `\`, `/`)), "/")
	if dir == "" {
		return nil
	}
	if !explicitAttributeRoles {
		attributeRoles = []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleMethod}
	}
	limit := sourceInventoryAttributeLimit(explicitAttributeRoles)
	seen := map[string]bool{}
	var out []sourceInventoryCandidate
	for _, sym := range symbolIndex.symbolsForDir(dir) {
		if sym == nil || !scopeFilter.SourceInRequestedScope(sym.File) {
			continue
		}
		role, ok := sourceInventoryAttributeRole(sym, attributeRoles)
		if !ok {
			continue
		}
		if ctx != nil && ctx.AnalysisIR != nil &&
			!sourceInventorySymbolMatchesVisibility(sym, ctx.AnalysisIR.RequestModel.AnswerVisibilityProfile) {
			continue
		}
		candidate := sourceInventoryCandidateForSymbol(sym, role, symbolIndex.graph)
		key := string(role) + "\x00" + candidate.key + "\x00" + candidate.file + "\x00" + strconvItoa(candidate.line)
		if candidate.key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	sourceInventorySortCallableAttributes(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sourceInventoryFileAttributes(ctx *types.BusContext, symbolIndex *sourceInventoryGraphSymbolIndex, scopeFilter sourceInventoryScopeFilter, file string, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool) []sourceInventoryCandidate {
	if symbolIndex == nil || !explicitAttributeRoles {
		return nil
	}
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	if file == "" {
		return nil
	}
	limit := sourceInventoryAttributeLimit(true)
	seen := map[string]bool{}
	var out []sourceInventoryCandidate
	for _, sym := range symbolIndex.symbolsForFile(file) {
		if sym == nil || !scopeFilter.SourceInRequestedScope(sym.File) {
			continue
		}
		role, ok := sourceInventoryAttributeRole(sym, attributeRoles)
		if !ok {
			continue
		}
		if ctx != nil && ctx.AnalysisIR != nil &&
			!sourceInventorySymbolMatchesVisibility(sym, ctx.AnalysisIR.RequestModel.AnswerVisibilityProfile) {
			continue
		}
		candidate := sourceInventoryCandidateForSymbol(sym, role, symbolIndex.graph)
		key := string(role) + "\x00" + candidate.key + "\x00" + candidate.file + "\x00" + strconvItoa(candidate.line)
		if candidate.key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	sourceInventorySortCallableAttributes(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sourceInventoryAttributeLimit(explicit bool) int {
	if explicit {
		return sourceInventoryMaxExplicitAttributes
	}
	return sourceInventoryMaxDefaultAttributes
}

func sourceInventorySymbolInPackageDir(sym *repotypes.Symbol, dir string) bool {
	if sym == nil {
		return false
	}
	file := strings.Trim(strings.TrimSpace(strings.ReplaceAll(sym.File, `\`, `/`)), "/")
	if file == "" || dir == "" {
		return false
	}
	return file == dir || strings.HasPrefix(file, dir+"/")
}

func sourceInventoryCallableAttributeRole(sym *repotypes.Symbol) (types.AnswerCandidateRole, bool) {
	return sourceInventoryAttributeRole(sym, []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleMethod})
}

func sourceInventoryAttributeRole(sym *repotypes.Symbol, attributeRoles []types.AnswerCandidateRole) (types.AnswerCandidateRole, bool) {
	if sym == nil {
		return types.AnswerCandidateRoleUnknown, false
	}
	role, ok := aggregateAnswerCandidateRoleForSymbol(sym)
	if !ok {
		return types.AnswerCandidateRoleUnknown, false
	}
	if len(attributeRoles) == 0 {
		return types.AnswerCandidateRoleUnknown, false
	}
	for _, allowed := range attributeRoles {
		if role == allowed {
			return role, true
		}
	}
	return types.AnswerCandidateRoleUnknown, false
}

func sourceInventorySortCallableAttributes(candidates []sourceInventoryCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].exported != candidates[j].exported {
			return candidates[i].exported
		}
		if candidates[i].file != candidates[j].file {
			return candidates[i].file < candidates[j].file
		}
		if candidates[i].line != candidates[j].line {
			if candidates[i].line == 0 {
				return false
			}
			if candidates[j].line == 0 {
				return true
			}
			return candidates[i].line < candidates[j].line
		}
		return candidates[i].member < candidates[j].member
	})
}

func sourceInventoryPackageBucketMember(bucket *sourceInventoryPackageBucket) string {
	if bucket == nil {
		return ""
	}
	if pkg := sourceInventoryDominantMapKey(bucket.packages); pkg != "" {
		return pkg
	}
	return path.Base(strings.Trim(bucket.dir, "/"))
}

func sourceInventoryPackageBucketNote(bucket *sourceInventoryPackageBucket) string {
	if bucket == nil {
		return ""
	}
	parts := []string{"directory=" + bucket.dir}
	if lang := sourceInventoryDominantMapKey(bucket.languages); lang != "" {
		parts = append(parts, "language="+lang)
	}
	if bucket.files > 0 {
		parts = append(parts, fmt.Sprintf("files=%d", bucket.files))
	}
	if bucket.symbols > 0 {
		parts = append(parts, fmt.Sprintf("symbols=%d", bucket.symbols))
	}
	return strings.Join(parts, ", ")
}

func sourceInventoryDominantMapKey(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] == counts[keys[j]] {
			return keys[i] < keys[j]
		}
		return counts[keys[i]] > counts[keys[j]]
	})
	return keys[0]
}

func sourceInventoryFileCandidateNote(fi *repotypes.FileInfo) string {
	if fi == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if fi.IsSpecial {
		stype := strings.TrimSpace(fi.SpecialType)
		if stype == "" {
			stype = "special"
		}
		parts = append(parts, "special_type="+stype)
	}
	if language := strings.TrimSpace(fi.Language); language != "" {
		parts = append(parts, "language="+language)
	}
	if len(fi.Symbols) > 0 {
		parts = append(parts, fmt.Sprintf("symbols=%d", len(fi.Symbols)))
	}
	return strings.Join(parts, ", ")
}

func sourceInventoryConfigFileCandidateNote(fi *repotypes.FileInfo) string {
	if fi == nil {
		return ""
	}
	parts := []string{"configuration/manifest file"}
	if stype := strings.TrimSpace(fi.SpecialType); stype != "" {
		parts = append(parts, "special_type="+stype)
	}
	if language := strings.TrimSpace(fi.Language); language != "" {
		parts = append(parts, "language="+language)
	}
	return strings.Join(parts, ", ")
}

func sourceInventoryGraphCandidates(ctx *types.BusContext, graph *repotypes.Graph, view *sourceInventoryExecutionView, symbolIndex *sourceInventoryGraphSymbolIndex, scopeFilter sourceInventoryScopeFilter, scopes []string, profile *types.SourceInventoryProfile, role types.AnswerCandidateRole, queryFilter sourceInventoryQueryFilter, budget sourceInventoryExecBudget) sourceInventoryCandidateSet {
	set := newSourceInventoryCandidateSet(role, view)
	if graph == nil || symbolIndex == nil {
		return set
	}
	excludedRoles := map[types.AnswerCandidateRole]bool{}
	var visibility *types.AnswerVisibilityProfile
	if ctx != nil && ctx.AnalysisIR != nil {
		visibility = ctx.AnalysisIR.RequestModel.AnswerVisibilityProfile
		if policy := ctx.AnalysisIR.RequestModel.AnswerExclusionPolicy; policy != nil && policy.Active() {
			excludedRoles = answerDocumentEffectiveExcludedRoleSet(ctx, policy, nil, nil)
		}
	}
	seen := map[string]bool{}
	scanned := 0
	scoped := 0
	for _, sym := range symbolIndex.all {
		if sym == nil || !sourceInventoryViewContainsFile(view, sym.File, scopes) || !scopeFilter.SourceInRequestedScope(sym.File) {
			continue
		}
		scoped++
		if queryFilter.Active() && budget.queryScanExceeded(scoped) {
			sourceInventoryMarkCandidateBudgetTruncated(&set)
			break
		}
		if !queryFilter.Active() && budget.scanExceeded(scoped) {
			sourceInventoryMarkCandidateBudgetTruncated(&set)
			break
		}
		if queryFilter.Active() && !sourceInventorySymbolMatchesQuery(sym, graph, queryFilter) {
			continue
		}
		scanned++
		candidateRole, ok := aggregateAnswerCandidateRoleForSymbol(sym)
		if !ok || candidateRole != role || answerDocumentSymbolMatchesExcludedRole(sym, excludedRoles) {
			continue
		}
		if !sourceInventorySymbolMatchesVisibility(sym, visibility) {
			continue
		}
		candidate := sourceInventoryCandidateForSymbol(sym, role, graph)
		if candidate.key == "" || (queryFilter.Active() && !sourceInventoryCandidateMatchesQuery(candidate, queryFilter)) {
			continue
		}
		key := sourceInventoryCandidateDedupeKey(candidate)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if budget.appendCandidate(&set, candidate, queryFilter) {
			break
		}
	}
	if queryFilter.Active() && len(set.candidates) == 0 && budget.scanExceeded(scoped) {
		sourceInventoryMarkCandidateBudgetTruncated(&set)
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

// SourceInventoryHasExplicitAuxiliaryExclusion reports whether the analyzer
// preserved a user-stated exclusion of repo-owned auxiliary source classes.
// This is a typed policy check: it consumes only AnswerExclusionPolicy enums
// that already passed source_quote validation, never raw request prose or model
// rationale. A production SourceScopeProfile without this exclusion is treated
// as a ranking preference for source-inventory lenses, not as an absence-proof
// hard filter over fixture/corpus/example surfaces.
func SourceInventoryHasExplicitAuxiliaryExclusion(rm types.RequestModel) bool {
	policy := rm.AnswerExclusionPolicy
	if policy == nil || !policy.Active() {
		return false
	}
	for _, role := range policy.ExcludedCandidateRoles {
		switch role {
		case types.AnswerCandidateRoleTest,
			types.AnswerCandidateRoleDocumentation,
			types.AnswerCandidateRoleExample,
			types.AnswerCandidateRoleFixture,
			types.AnswerCandidateRoleGenerated:
			return true
		}
	}
	return false
}

func sourceInventoryFilterCandidateSetByQuery(set sourceInventoryCandidateSet, rawQuery string) sourceInventoryCandidateSet {
	filter := sourceInventoryBuildQueryFilter(rawQuery)
	if !filter.Active() || len(set.candidates) == 0 {
		return set
	}
	filtered := make([]sourceInventoryCandidate, 0, len(set.candidates))
	for _, candidate := range set.candidates {
		if sourceInventoryCandidateMatchesQuery(candidate, filter) {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == 0 {
		return set
	}
	set.candidates = filtered
	return set
}

func sourceInventoryCandidateMatchesQuery(candidate sourceInventoryCandidate, filter sourceInventoryQueryFilter) bool {
	if !filter.Active() {
		return true
	}
	if !sourceInventoryQueryLanguageMatches(candidate.language, filter) {
		return false
	}
	parts := []string{
		candidate.member,
		candidate.key,
		candidate.note,
		string(candidate.role),
	}
	for _, attr := range candidate.attributes {
		parts = append(parts, attr.member, attr.key, attr.note, string(attr.role))
	}
	if len(filter.Tokens) == 0 {
		return true
	}
	return sourceInventoryAnyQueryTokenMatches(parts, filter.Tokens)
}

func sourceInventorySymbolMatchesQuery(sym *repotypes.Symbol, graph *repotypes.Graph, filter sourceInventoryQueryFilter) bool {
	if sym == nil || !filter.Active() {
		return false
	}
	language := sourceInventoryGraphLanguageForFile(graph, sym.File)
	if !sourceInventoryQueryLanguageMatches(language, filter) {
		return false
	}
	parts := []string{
		sym.Name,
		sym.Kind,
		sym.Doc,
		sym.Parent,
		sym.Receiver,
		sym.Signature,
	}
	if graph != nil && graph.FileIndex != nil {
		if fi := graph.FileIndex[sym.File]; fi != nil {
			parts = append(parts, fi.Package)
		}
	}
	if len(filter.Tokens) == 0 {
		return true
	}
	return sourceInventoryAnyQueryTokenMatches(parts, filter.Tokens)
}

func sourceInventoryAnyQueryTokenMatches(parts []string, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	for _, part := range parts {
		normalized := sourceInventoryQueryNormalizeText(part)
		if normalized == "" {
			continue
		}
		for _, token := range tokens {
			if token != "" && strings.Contains(normalized, token) {
				return true
			}
		}
	}
	return false
}

func sourceInventoryBuildQueryFilter(raw string) sourceInventoryQueryFilter {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return sourceInventoryQueryFilter{}
	}
	seen := map[string]bool{}
	languages := map[string]bool{}
	var tokens []string
	add := func(token string) {
		token = strings.Trim(token, "_-")
		if len(token) < 2 || seen[token] {
			return
		}
		if repotypes.IsSupportedReadLanguage(token) {
			languages[token] = true
			seen[token] = true
			return
		}
		if sourceInventoryQueryTokenIsGeneric(token) {
			return
		}
		seen[token] = true
		tokens = append(tokens, token)
	}
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		add(cur.String())
		cur.Reset()
	}
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	if len(languages) == 0 {
		languages = nil
	}
	return sourceInventoryQueryFilter{Tokens: tokens, Languages: languages}
}

func sourceInventoryQueryTokenIsGeneric(token string) bool {
	switch token {
	case "function", "functions", "method", "methods", "type", "types",
		"file", "files", "path", "paths", "name", "names", "symbol", "symbols":
		return true
	}
	return false
}

func (f sourceInventoryQueryFilter) Active() bool {
	return len(f.Tokens) > 0 || len(f.Languages) > 0
}

func sourceInventoryQueryLanguageMatches(language string, filter sourceInventoryQueryFilter) bool {
	if len(filter.Languages) == 0 {
		return true
	}
	return filter.Languages[strings.ToLower(strings.TrimSpace(language))]
}

func sourceInventoryQueryNormalizeText(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := true
	for _, r := range raw {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-':
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func sourceInventoryCandidateDedupeKey(candidate sourceInventoryCandidate) string {
	key := strings.TrimSpace(candidate.key)
	if key == "" {
		key = aggregateMemberKey(candidate.member)
	}
	if key == "" {
		return ""
	}
	return strings.Join([]string{
		string(candidate.role),
		key,
		strings.TrimSpace(candidate.file),
		strconvItoa(candidate.line),
	}, "\x00")
}

func sourceInventoryGoStringEnumCandidates(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile, budget sourceInventoryExecBudget) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: types.AnswerCandidateRoleType, complete: true}
	if graph == nil || ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		set.complete = false
		return set
	}
	view := newSourceInventoryExecutionViewWithBudget(graph, scopes, budget)
	if sourceInventoryViewBudgetTruncated(view) {
		sourceInventoryMarkCandidateBudgetTruncated(&set)
	}
	files := view.Files("go")
	if len(files) == 0 {
		set.complete = false
		return set
	}
	type typeInfo struct {
		file string
		line int
		note string
	}
	stringTypes := map[string]typeInfo{}
	constBacked := map[string]bool{}
	for _, fi := range files {
		if fi == nil || strings.TrimSpace(fi.RelPath) == "" {
			continue
		}
		full := filepath.Join(ctx.RepoRoot, filepath.FromSlash(fi.RelPath))
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, full, nil, parser.ParseComments)
		if err != nil {
			set.complete = false
			continue
		}
		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			switch gen.Tok {
			case token.TYPE:
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || ts == nil || ts.Name == nil || !ts.Name.IsExported() {
						continue
					}
					if sourceInventoryGoExprSurface(ts.Type) != "string" {
						continue
					}
					pos := fset.Position(ts.Name.Pos())
					stringTypes[ts.Name.Name] = typeInfo{
						file: fi.RelPath,
						line: pos.Line,
						note: sourceInventoryGoCommentText(ts.Name.Name, ts.Doc, gen.Doc),
					}
				}
			case token.CONST:
				currentType := ""
				for _, spec := range gen.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok || vs == nil {
						continue
					}
					if vs.Type != nil {
						currentType = sourceInventoryGoExprSurface(vs.Type)
					}
					if _, ok := stringTypes[currentType]; !ok {
						continue
					}
					for _, name := range vs.Names {
						if name != nil {
							constBacked[currentType] = true
						}
					}
				}
			}
		}
	}
	var visibility *types.AnswerVisibilityProfile
	if ctx.AnalysisIR != nil {
		visibility = ctx.AnalysisIR.RequestModel.AnswerVisibilityProfile
	}
	seen := map[string]bool{}
	for name, info := range stringTypes {
		if !constBacked[name] {
			continue
		}
		sym := sourceInventoryGraphSymbolAt(graph, name, info.file, info.line)
		exported := ast.IsExported(name)
		if sym != nil {
			exported = sym.Exported
		}
		note, surfaceTerms := sourceInventoryCandidateNoteAndSurfaceTermsFromGraph(sym, sourceInventoryGraphLanguageForFile(graph, info.file))
		if note == "" {
			note = info.note
		}
		if !sourceInventoryVisibilityAllowsExported(exported, visibility) {
			continue
		}
		key := aggregateMemberKey(name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		set.candidates = append(set.candidates, sourceInventoryCandidate{
			member:       name,
			key:          key,
			supportRef:   name + ": " + aggregateSupportLocationKey(info.file, info.line),
			note:         note,
			surfaceTerms: surfaceTerms,
			role:         types.AnswerCandidateRoleType,
			exported:     exported,
			file:         info.file,
			line:         info.line,
			language:     "go",
		})
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

func sourceInventoryGoExprSurface(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if e.Sel != nil {
			return e.Sel.Name
		}
	}
	return ""
}

func sourceInventoryGoCommentText(symbolName string, groups ...*ast.CommentGroup) string {
	for _, group := range groups {
		if group == nil {
			continue
		}
		if note := sourceInventoryCompactNote(group.Text()); note != "" {
			if sourceInventoryCommentDescribesSymbol(note, symbolName) {
				return note
			}
		}
	}
	return ""
}

func sourceInventoryScopes(ctx *types.BusContext, graph *repotypes.Graph, facts []types.AnswerAggregateFact) []string {
	seen := map[string]bool{}
	var scopes []string
	add := func(raw string) {
		scope := sourceInventoryScopeForSurface(graph, raw)
		if scope == "" || seen[scope] {
			return
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}
	// Source-inventory recovery is allowed to fill holes inside the user's
	// requested package/path, but it must not promote analyzer-related context
	// files into principal inventory rows. Prefer path-like current-request
	// entity lanes; fall back to required file hints only when no request scope
	// was resolved. Subtopics and aggregate support refs remain legacy fallbacks.
	for _, scope := range sourceInventoryRequestedScopes(ctx, graph) {
		add(scope)
	}
	if len(scopes) > 0 {
		sort.Strings(scopes)
		return scopes
	}
	if ctx != nil && ctx.AnalysisIR != nil {
		for _, topic := range ctx.AnalysisIR.RequestModel.SubTopics {
			for _, entity := range topic.Entities {
				add(entity)
			}
		}
	}
	for _, fact := range facts {
		for _, ref := range fact.SupportRefs {
			_, loc, ok := aggregateMemberSupportRefParts(ref)
			if !ok {
				continue
			}
			file, _ := aggregateLocationParts(loc)
			add(file)
		}
	}
	sort.Strings(scopes)
	return scopes
}

func sourceInventoryRequestedScopes(ctx *types.BusContext, graph *repotypes.Graph) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	hints := ctx.AnalysisIR.RequestModel.AnalyzerHints
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		scope := sourceInventoryScopeForSurface(graph, raw)
		if scope == "" || seen[scope] {
			return
		}
		seen[scope] = true
		out = append(out, scope)
	}
	for _, group := range [][]string{
		hints.ExactTargets,
		hints.MentionedEntities,
		hints.PrimaryEntities,
		hints.Entities,
	} {
		for _, raw := range group {
			add(raw)
		}
	}
	if len(out) > 0 {
		sort.Strings(out)
		return out
	}
	for _, result := range aggregateSupportToolResults(ctx) {
		if !result.Success || result.ToolName != "list_files" {
			continue
		}
		add(sourceInventoryListFilesScope(result.Summary))
	}
	if len(out) > 0 {
		sort.Strings(out)
		return out
	}
	for _, file := range ctx.AnalysisIR.EvidencePlan.RequiredFiles {
		add(file)
	}
	for _, hint := range hints.RequiredFileHints {
		add(hint.Path)
	}
	sort.Strings(out)
	return out
}

func sourceInventoryListFilesScope(summary string) string {
	if strings.TrimSpace(summary) == "" {
		return ""
	}
	firstLine := summary
	if idx := strings.Index(firstLine, "\n"); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)
	if !strings.HasPrefix(firstLine, "[list_files: ") || !strings.HasSuffix(firstLine, "]") {
		return ""
	}
	const key = "path="
	idx := strings.Index(firstLine, key)
	if idx < 0 {
		return ""
	}
	value := strings.TrimSpace(firstLine[idx+len(key) : len(firstLine)-1])
	if split := strings.Index(value, " recursive="); split >= 0 {
		value = strings.TrimSpace(value[:split])
	}
	return value
}

func sourceInventoryScopeForSurface(graph *repotypes.Graph, raw string) string {
	surface := normalizeSourceInventoryScopeSurface(raw)
	if surface == "" {
		return ""
	}
	if graph != nil {
		if _, ok := graph.FileIndex[surface]; ok {
			return surface
		}
		if scope := sourceInventoryScopeFromAbsoluteSuffix(graph, surface); scope != "" {
			return scope
		}
		for file := range graph.FileIndex {
			file = strings.Trim(file, "/")
			if file == surface || strings.HasPrefix(file, surface+"/") {
				return surface
			}
			if aggregateReadFilePathMatchesQualifier(file, surface) {
				return file
			}
		}
	}
	return ""
}

func normalizeSourceInventoryScopeSurface(raw string) string {
	surface := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	if surface == "" {
		return ""
	}
	surface = filepath.ToSlash(filepath.Clean(surface))
	if surface == "." {
		return ""
	}
	surface = strings.TrimPrefix(surface, "./")
	return strings.Trim(surface, "/")
}

func sourceInventoryScopeFromAbsoluteSuffix(graph *repotypes.Graph, surface string) string {
	if graph == nil || len(graph.FileIndex) == 0 || surface == "" {
		return ""
	}
	best := ""
	consider := func(candidate string) {
		candidate = strings.Trim(candidate, "/")
		if candidate == "" {
			return
		}
		if surface == candidate || strings.HasSuffix(surface, "/"+candidate) {
			if len(candidate) > len(best) {
				best = candidate
			}
		}
	}
	for file := range graph.FileIndex {
		file = strings.Trim(file, "/")
		if file == "" {
			continue
		}
		consider(file)
		for dir := path.Dir(file); dir != "." && dir != "/" && dir != ""; dir = path.Dir(dir) {
			consider(dir)
		}
	}
	return best
}

func sourceInventoryScopedGraphFiles(graph *repotypes.Graph, scopes []string, language string) []*repotypes.FileInfo {
	return newSourceInventoryExecutionView(graph, scopes).Files(language)
}

func sourceInventoryFileInScopes(file string, scopes []string) bool {
	return sourceinventory.FileInScopes(file, scopes, aggregateReadFilePathMatchesQualifier)
}

func newSourceInventoryExecutionView(graph *repotypes.Graph, scopes []string) *sourceInventoryExecutionView {
	return newSourceInventoryExecutionViewWithBudget(graph, scopes, sourceInventoryInactiveExecBudget())
}

func newSourceInventoryExecutionViewWithBudget(graph *repotypes.Graph, scopes []string, budget sourceInventoryExecBudget) *sourceInventoryExecutionView {
	return sourceinventory.NewExecutionView(sourceinventory.ExecutionViewOptions{
		Graph:        graph,
		Scopes:       sourceInventoryDedupeScopeAliases(scopes),
		ScopeMatcher: aggregateReadFilePathMatchesQualifier,
		Budget:       budget.executionKernel(),
	})
}

func sourceInventoryViewComplete(view *sourceInventoryExecutionView) bool {
	return view != nil && view.Complete()
}

func sourceInventoryViewHasInventoryFiles(view *sourceInventoryExecutionView) bool {
	return view != nil && view.HasInventoryFiles()
}

func sourceInventoryViewBudgetTruncated(view *sourceInventoryExecutionView) bool {
	return view != nil && view.BudgetTruncated()
}

func sourceInventoryViewContainsFile(view *sourceInventoryExecutionView, file string, fallbackScopes []string) bool {
	rel := sourceinventory.NormalizeFileSurface(file)
	if rel == "" {
		return false
	}
	if view == nil {
		return sourceInventoryFileInScopes(rel, fallbackScopes)
	}
	return view.ContainsFile(rel)
}

func sourceInventoryNormalizeFileSurface(file string) string {
	return sourceinventory.NormalizeFileSurface(file)
}

func newSourceInventoryScopeFilter(ctx *types.BusContext) sourceInventoryScopeFilter {
	filter := sourceInventoryScopeFilter{ctx: ctx}
	if ctx == nil || ctx.AnalysisIR == nil {
		return filter
	}
	rm := ctx.AnalysisIR.RequestModel
	filter.sourceScopeProfileRequested = rm.SourceScopeProfile != nil && rm.SourceScopeProfile.RequestedScope != ""
	filter.sourceInventoryProfileActive = rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active()
	filter.typedSourceEnumerationShape = types.IsTypedSourceEnumerationShape(rm)
	if ctx.Mutable != nil && filter.typedSourceEnumerationShape {
		advisory := ctx.Mutable.SourceInventoryAdvisory()
		filter.repositoryWideTypedQueryLane = sourceInventoryAdvisoryUsesRepositoryRootQueryLane(ctx, advisory)
	}
	return filter
}

func (filter sourceInventoryScopeFilter) SourceInRequestedScope(source string) bool {
	if filter.ctx == nil || filter.ctx.AnalysisIR == nil {
		return true
	}
	rm := filter.ctx.AnalysisIR.RequestModel
	if filter.repositoryWideTypedQueryLane {
		return true
	}
	if filter.sourceInventoryProfileActive && !SourceInventoryHasExplicitAuxiliaryExclusion(rm) {
		return true
	}
	if filter.sourceScopeProfileRequested {
		return aggregateEvidenceSourceInRequestedScope(filter.ctx, source)
	}
	if filter.sourceInventoryProfileActive {
		return true
	}
	if filter.typedSourceEnumerationShape {
		return true
	}
	return aggregateEvidenceSourceInRequestedScope(filter.ctx, source)
}

func sourceInventorySourceInRequestedScope(ctx *types.BusContext, source string) bool {
	return newSourceInventoryScopeFilter(ctx).SourceInRequestedScope(source)
}

func sourceInventoryUsesRepositoryWideTypedQueryLane(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !types.IsTypedSourceEnumerationShape(rm) {
		return false
	}
	advisory := ctx.Mutable.SourceInventoryAdvisory()
	return sourceInventoryAdvisoryUsesRepositoryRootQueryLane(ctx, advisory)
}

func sourceInventoryAdvisoryUsesRepositoryRootQueryLane(ctx *types.BusContext, advisory types.SourceInventoryAdvisory) bool {
	if !sourceInventoryAdvisoryIsTypedQueryLane(ctx, advisory) {
		return false
	}
	for _, provenance := range advisory.Provenance {
		if strings.TrimSpace(provenance) == "request_traits:query_root_scope" {
			return true
		}
	}
	return false
}

func sourceInventoryScopesAllLanguage(graph *repotypes.Graph, scopes []string, language string) bool {
	files := sourceInventoryScopedGraphFiles(graph, scopes, "")
	if len(files) == 0 {
		return false
	}
	for _, fi := range files {
		if fi == nil || fi.IsSpecial {
			continue
		}
		if strings.TrimSpace(fi.Language) != language {
			return false
		}
	}
	return true
}

func sourceInventoryScopesHaveIndexedSourceFiles(graph *repotypes.Graph, scopes []string) bool {
	files := sourceInventoryScopedGraphFiles(graph, scopes, "")
	if len(files) == 0 {
		return false
	}
	for _, fi := range files {
		if fi == nil || fi.IsSpecial {
			continue
		}
		if strings.TrimSpace(fi.Language) != "" {
			return true
		}
	}
	return false
}

func sourceInventoryScopesHaveInventoryFiles(graph *repotypes.Graph, scopes []string) bool {
	files := sourceInventoryScopedGraphFiles(graph, scopes, "")
	if len(files) == 0 {
		return false
	}
	for _, fi := range files {
		if fi == nil {
			continue
		}
		if strings.TrimSpace(fi.Language) != "" || fi.IsSpecial {
			return true
		}
	}
	return false
}

func newSourceInventoryGraphSymbolIndex(graph *repotypes.Graph) *sourceInventoryGraphSymbolIndex {
	if graph == nil {
		return nil
	}
	index := &sourceInventoryGraphSymbolIndex{
		graph:  graph,
		byFile: map[string][]*repotypes.Symbol{},
		byDir:  map[string][]*repotypes.Symbol{},
	}
	seen := map[*repotypes.Symbol]bool{}
	if len(graph.SymbolByID) > 0 {
		for _, sym := range graph.SymbolByID {
			if sym != nil && !seen[sym] {
				seen[sym] = true
				index.add(sym)
			}
		}
	}
	for _, defs := range graph.SymbolDefs {
		for _, sym := range defs {
			if sym != nil && !seen[sym] {
				seen[sym] = true
				index.add(sym)
			}
		}
	}
	index.sort()
	return index
}

func (index *sourceInventoryGraphSymbolIndex) add(sym *repotypes.Symbol) {
	if index == nil || sym == nil {
		return
	}
	index.all = append(index.all, sym)
	file := sourceInventoryNormalizeGraphFile(sym.File)
	if file != "" {
		index.byFile[file] = append(index.byFile[file], sym)
		for _, dir := range sourceInventoryGraphFileDirs(file) {
			index.byDir[dir] = append(index.byDir[dir], sym)
		}
	}
}

func (index *sourceInventoryGraphSymbolIndex) symbolsForFile(file string) []*repotypes.Symbol {
	if index == nil {
		return nil
	}
	return index.byFile[sourceInventoryNormalizeGraphFile(file)]
}

func (index *sourceInventoryGraphSymbolIndex) symbolsForDir(dir string) []*repotypes.Symbol {
	if index == nil {
		return nil
	}
	return index.byDir[sourceInventoryNormalizeGraphFile(dir)]
}

func sourceInventoryNormalizeGraphFile(file string) string {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	for strings.HasPrefix(file, "./") {
		file = strings.TrimPrefix(file, "./")
	}
	return file
}

func sourceInventoryGraphFileDirs(file string) []string {
	file = sourceInventoryNormalizeGraphFile(file)
	if file == "" {
		return nil
	}
	dir := strings.Trim(path.Dir(file), "/")
	if dir == "." {
		dir = ""
	}
	if dir == "" {
		return []string{"."}
	}
	parts := strings.Split(dir, "/")
	out := make([]string, 0, len(parts)+1)
	out = append(out, ".")
	for i := range parts {
		out = append(out, strings.Join(parts[:i+1], "/"))
	}
	return out
}

func sourceInventoryGraphLanguageForFile(graph *repotypes.Graph, file string) string {
	if graph == nil || graph.FileIndex == nil {
		return ""
	}
	if fi := graph.FileIndex[file]; fi != nil {
		return strings.TrimSpace(fi.Language)
	}
	return ""
}

func sourceInventoryCompactNote(raw string) string {
	note := strings.Join(strings.Fields(sourceInventoryCleanCommentText(raw)), " ")
	if note == "" {
		return ""
	}
	const max = 240
	if len([]rune(note)) <= max {
		return note
	}
	runes := []rune(note)
	return strings.TrimSpace(string(runes[:max]))
}

func sourceInventoryCleanCommentText(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimSuffix(line, "*/"))
		for {
			switch {
			case strings.HasPrefix(line, "//"):
				line = strings.TrimSpace(strings.TrimPrefix(line, "//"))
			case strings.HasPrefix(line, "/*"):
				line = strings.TrimSpace(strings.TrimPrefix(line, "/*"))
			case strings.HasPrefix(line, "*/"):
				line = strings.TrimSpace(strings.TrimPrefix(line, "*/"))
			case strings.HasPrefix(line, "*"):
				line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
			default:
				goto donePrefix
			}
		}
	donePrefix:
		line = strings.TrimSpace(strings.TrimSuffix(line, "*/"))
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, " ")
}

func sourceInventoryCommentDescribesSymbol(note string, symbolName string) bool {
	note = strings.TrimSpace(note)
	symbolName = strings.TrimSpace(symbolName)
	if note == "" || symbolName == "" {
		return false
	}
	return sourceInventoryContainsSymbolToken(strings.ToLower(note), strings.ToLower(symbolName))
}

func sourceInventoryGraphDocDescribesSymbol(note string, symbolName string, language string) bool {
	note = strings.TrimSpace(note)
	if note == "" || sourceInventoryLooksStructuralDoc(note) {
		return false
	}
	if sourceInventoryCommentDescribesSymbol(note, symbolName) {
		return true
	}
	// Python docstrings are structurally inside the class/function node, so
	// they can be trusted as symbol docs even when the prose does not repeat
	// the symbol name. Adjacent comments in other languages remain stricter
	// until their extractors carry doc provenance separately.
	return language == repotypes.LangPython
}

func sourceInventoryLooksStructuralDoc(note string) bool {
	note = strings.TrimSpace(note)
	if note == "" {
		return false
	}
	lower := strings.ToLower(note)
	if strings.HasPrefix(note, "@") || strings.HasPrefix(lower, "extends ") {
		return true
	}
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return false
	}
	structural := map[string]bool{
		"public": true, "private": true, "protected": true, "internal": true,
		"open": true, "final": true, "abstract": true, "static": true,
		"const": true, "foreign": true, "operator": true, "func": true,
		"init": true, "main": true, "entry": true, "typealias": true,
		"extend": true,
	}
	for _, field := range fields {
		field = strings.Trim(field, " ,;:()[]{}")
		if field == "" {
			continue
		}
		if !structural[field] {
			return false
		}
	}
	return true
}

func sourceInventoryContainsSymbolToken(note string, symbol string) bool {
	start := 0
	for start < len(note) {
		idx := strings.Index(note[start:], symbol)
		if idx < 0 {
			return false
		}
		idx += start
		after := idx + len(symbol)
		if !sourceInventoryIdentifierRuneBefore(note, idx) && !sourceInventoryIdentifierRuneAt(note, after) {
			return true
		}
		start = after
	}
	return false
}

func sourceInventoryIdentifierRuneBefore(text string, idx int) bool {
	if idx <= 0 || idx > len(text) {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:idx])
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func sourceInventoryIdentifierRuneAt(text string, idx int) bool {
	if idx < 0 || idx >= len(text) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[idx:])
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func appendStringAtIndex(values []string, idx int, value string) []string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	for len(values) < idx {
		values = append(values, "")
	}
	if len(values) == idx {
		return append(values, value)
	}
	if values[idx] == "" {
		values[idx] = value
	}
	return values
}

func trimTrailingEmptyStrings(values []string) []string {
	last := -1
	for i, value := range values {
		if strings.TrimSpace(value) != "" {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	return append([]string(nil), values[:last+1]...)
}

func sourceInventorySymbolMatchesVisibility(sym *repotypes.Symbol, profile *types.AnswerVisibilityProfile) bool {
	if sym == nil {
		return false
	}
	return sourceInventoryVisibilityAllowsExported(sym.Exported, profile)
}

func sourceInventoryVisibilityAllowsExported(exported bool, profile *types.AnswerVisibilityProfile) bool {
	if profile == nil || !profile.Active() {
		return true
	}
	switch profile.SymbolVisibility {
	case types.AnswerSymbolVisibilityPublicExported:
		return exported
	case types.AnswerSymbolVisibilityPrivateOnly:
		return !exported
	case types.AnswerSymbolVisibilityAll:
		return true
	default:
		return true
	}
}

func sourceInventoryGraphSymbolAt(graph *repotypes.Graph, name, file string, line int) *repotypes.Symbol {
	if graph == nil {
		return nil
	}
	for _, sym := range graph.SymbolDefs[name] {
		if aggregateGraphSymbolMatchesLocation(sym, name, file, line) {
			return sym
		}
	}
	return nil
}

func sourceInventorySortCandidates(candidates []sourceInventoryCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].file != candidates[j].file {
			return candidates[i].file < candidates[j].file
		}
		if candidates[i].line != candidates[j].line {
			return candidates[i].line < candidates[j].line
		}
		return candidates[i].member < candidates[j].member
	})
}

func strconvItoa(n int) string {
	return strconv.Itoa(n)
}
