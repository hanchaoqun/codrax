package tool

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

type sourceInventoryCandidate struct {
	member     string
	key        string
	supportRef string
	note       string
	role       types.AnswerCandidateRole
	exported   bool
	file       string
	line       int
	language   string
	attributes []sourceInventoryCandidate
}

type sourceInventoryCandidateSet struct {
	role       types.AnswerCandidateRole
	candidates []sourceInventoryCandidate
	complete   bool
}

type sourceInventoryPackageBucket struct {
	dir       string
	files     int
	symbols   int
	languages map[string]int
	packages  map[string]int
}

type sourceInventoryObservationScopeGroup struct {
	Scope      string
	order      int
	Candidates []types.SourceInventoryObservationMember
	RoleCounts map[types.AnswerCandidateRole]int
	Languages  map[string]int
}

type sourceInventorySuggestedFileGroup struct {
	Scope          string
	order          int
	Files          []sourceInventorySuggestedFile
	candidateCount int
}

type sourceInventorySuggestedFile struct {
	File       string
	order      int
	RoleCounts map[types.AnswerCandidateRole]int
	Languages  map[string]int
	Candidates []sourceInventorySuggestedFileCandidate
	seen       map[string]bool
}

type sourceInventorySuggestedFileCandidate struct {
	Name string
	Role types.AnswerCandidateRole
	Line int
}

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

func publishSourceInventoryAdvisorySnapshot(ctx *types.BusContext, provenance string, advisoryOnly bool) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	advisory := buildSourceInventoryAdvisory(ctx, nil, nil)
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

// PublishSourceInventoryObservationFromLens builds and stores a model-driven
// source-inventory lens view. It is deliberately advisory-only: even when the
// model asks for a role/scope slice explicitly, the result is a repo-map
// observation checklist, not final answer text.
func PublishSourceInventoryObservationFromLens(ctx *types.BusContext, query types.SourceInventoryLensQuery) types.SourceInventoryObservation {
	if ctx == nil || ctx.Mutable == nil {
		return types.SourceInventoryObservation{}
	}
	advisory := buildSourceInventoryAdvisoryForLens(ctx, query, true, []string{"repo_lens:tool_query"})
	if !advisory.IsActive() {
		return types.SourceInventoryObservation{}
	}
	if current := ctx.Mutable.SourceInventoryAdvisory(); current.IsActive() {
		ctx.Mutable.SetSourceInventoryAdvisory(types.MergeSourceInventoryAdvisory(current, advisory))
	} else {
		ctx.Mutable.SetSourceInventoryAdvisory(advisory)
	}
	return types.SourceInventoryObservationFromAdvisory(advisory)
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
	b.WriteString(". To inspect it incrementally, consider `repo_map(view=\"source_inventory\")` before reading many files. This is navigation only, not final-answer evidence.\n")
	fmt.Fprintf(&b, "- broad member/attribute checklist: `%s`\n",
		sourceInventoryCascadeRepoMapCall(pathSurface, "", nil, expandRoles, attrRoles, 24, ""))
	if len(obs.ScopeGroups) > 0 {
		b.WriteString("- expand only the branch that matches the user's intent:\n")
		maxScopes := minInt(len(obs.ScopeGroups), 4)
		for i := 0; i < maxScopes; i++ {
			scope := obs.ScopeGroups[i]
			fmt.Fprintf(&b, "  - `%s`: `%s`\n", scope,
				sourceInventoryCascadeRepoMapCall(pathSurface, scope, nil, expandRoles, attrRoles, 24, ""))
		}
		if len(obs.ScopeGroups) > maxScopes {
			fmt.Fprintf(&b, "  - +%d more scope groups hidden here; use the same call shape with the chosen scope.\n", len(obs.ScopeGroups)-maxScopes)
		}
	} else {
		fmt.Fprintf(&b, "- expand a chosen scope: `%s`\n",
			sourceInventoryCascadeRepoMapCall(pathSurface, "<scope>", nil, expandRoles, attrRoles, 24, ""))
	}
	b.WriteString("- Adjust `roles` / `attribute_roles` to the structured role you need; verify chosen rows with `read_file` or `grep` before citing.")
	return strings.TrimSpace(b.String())
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

func renderSourceInventoryAdvisoryToolHint(advisory types.SourceInventoryAdvisory) string {
	if !advisory.IsActive() {
		return ""
	}
	const maxRows = 24
	var b strings.Builder
	b.WriteString("Structured source-inventory candidate checklist (advisory only, not final answer text):\n")
	if len(advisory.Scopes) > 0 {
		fmt.Fprintf(&b, "- scoped to: %s\n", strings.Join(advisory.Scopes, ", "))
	}
	b.WriteString("- reuse this checklist to avoid re-listing the same scope; verify/read unresolved rows before emitting evidence or aggregate_facts.\n")
	b.WriteString("- for a compact scoped member/attribute checklist, call repo_map with view=\"source_inventory\", roles=[...], and optional attribute_roles=[...] instead of reading every candidate file.\n")
	if cascadeView := renderSourceInventoryCascadeGuideView(
		types.SourceInventoryObservationFromAdvisory(advisory),
		types.SourceInventoryLensQuery{Scopes: append([]string(nil), advisory.Scopes...), IncludeAttributes: true, IncludeCounts: true},
		8,
	); cascadeView != "" {
		b.WriteByte('\n')
		b.WriteString(cascadeView)
		b.WriteString("\n\n")
	}
	emitted := 0
	total := 0
	for _, set := range advisory.Sets {
		total += len(set.Candidates)
	}
	for _, set := range advisory.Sets {
		if emitted >= maxRows || len(set.Candidates) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- %s candidates:", types.SourceInventoryAdvisoryRoleLabel(set.Role))
		for _, candidate := range set.Candidates {
			if emitted >= maxRows {
				break
			}
			member := strings.TrimSpace(candidate.Member)
			if member == "" {
				continue
			}
			if candidate.File != "" && candidate.Line > 0 {
				fmt.Fprintf(&b, " %s@%s:%d;", member, candidate.File, candidate.Line)
			} else if candidate.File != "" {
				fmt.Fprintf(&b, " %s@%s;", member, candidate.File)
			} else {
				fmt.Fprintf(&b, " %s;", member)
			}
			if attrs := renderSourceInventoryAdvisoryToolHintAttributes(candidate.Attributes); attrs != "" {
				fmt.Fprintf(&b, " {%s}", attrs)
			}
			emitted++
		}
		b.WriteByte('\n')
	}
	if total > emitted {
		fmt.Fprintf(&b, "- showing %d of %d candidates; keep using typed evidence for any hidden rows that matter.\n", emitted, total)
	}
	return strings.TrimSpace(b.String())
}

func renderSourceInventoryAdvisoryToolHintAttributes(attrs []types.SourceInventoryAdvisoryAttribute) string {
	if len(attrs) == 0 {
		return ""
	}
	const max = 3
	var parts []string
	for _, attr := range attrs {
		if len(parts) >= max {
			break
		}
		member := strings.TrimSpace(attr.Member)
		if member == "" {
			continue
		}
		loc := strings.TrimSpace(attr.File)
		if loc != "" && attr.Line > 0 {
			loc = loc + ":" + strconvItoa(attr.Line)
		}
		if loc != "" {
			parts = append(parts, member+"@"+loc)
		} else {
			parts = append(parts, member)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(attrs) > len(parts) {
		parts = append(parts, fmt.Sprintf("+%d", len(attrs)-len(parts)))
	}
	return "related callables: " + strings.Join(parts, ", ")
}

// RenderSourceInventoryCascadeGuideView exposes the same low-noise repo-lens
// navigation guide used by repo_map(view="source_inventory") tool results.
// It is intentionally advisory-only: callers may render it in prompts to help
// models choose narrower follow-up repo_map calls, but it must not be treated
// as evidence or as an answer-membership authority.
func RenderSourceInventoryCascadeGuideView(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery, maxGroups int) string {
	return renderSourceInventoryCascadeGuideView(observation, query, maxGroups)
}

func renderSourceInventoryCascadeGuideView(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery, maxGroups int) string {
	if !observation.IsActive() || sourceInventoryLensQueryOffset(query) > 0 || maxGroups <= 0 {
		return ""
	}
	groups := sourceInventorySuggestedFileGroups(observation, query)
	totalMembers := 0
	roleCounts := map[types.AnswerCandidateRole]int{}
	languageCounts := map[string]int{}
	attrRoleCounts := map[types.AnswerCandidateRole]int{}
	for _, set := range observation.Sets {
		totalMembers += len(set.Members)
		if set.Role != types.AnswerCandidateRoleUnknown && len(set.Members) > 0 {
			roleCounts[set.Role] += len(set.Members)
		}
		for _, member := range set.Members {
			if lang := strings.TrimSpace(member.Language); lang != "" {
				languageCounts[lang]++
			}
			for _, attr := range member.Attributes {
				if attr.Role != types.AnswerCandidateRoleUnknown {
					attrRoleCounts[attr.Role]++
				}
				if lang := strings.TrimSpace(attr.Language); lang != "" {
					languageCounts[lang]++
				}
			}
		}
	}
	totalFiles := 0
	totalCandidateItems := 0
	ambiguousGroups := 0
	for _, group := range groups {
		totalFiles += len(group.Files)
		totalCandidateItems += group.candidateCount
		if len(group.Files) != 1 || group.candidateCount != 1 {
			ambiguousGroups++
		}
	}

	roles := sourceInventoryLensQueryRoles(query)
	if len(roles) == 0 {
		roles = sourceInventoryObservationRoles(observation)
	}
	attributeRoles := sourceInventoryLensQueryAttributeRoles(query)
	if len(attributeRoles) == 0 {
		attributeRoles = sourceInventoryObservationAttributeRoles(observation)
	}
	var b strings.Builder
	b.WriteString("## Cascaded Repo Lens Guide (advisory)\n\n")
	b.WriteString("Use this first as a navigation summary. It helps you choose the next narrower `repo_map(view=\"source_inventory\")` call; it is not evidence and does not decide the final answer.\n\n")
	fmt.Fprintf(&b, "- summary: member_rows=%d", totalMembers)
	if len(groups) > 0 {
		fmt.Fprintf(&b, " scope_groups=%d candidate_files=%d candidate_items=%d ambiguous_groups=%d",
			len(groups), totalFiles, totalCandidateItems, ambiguousGroups)
	}
	if rolesText := renderSourceInventoryRoleCounts(roleCounts); rolesText != "" {
		fmt.Fprintf(&b, " roles=%s", rolesText)
	}
	if attrRolesText := renderSourceInventoryRoleCounts(attrRoleCounts); attrRolesText != "" {
		fmt.Fprintf(&b, " attribute_roles=%s", attrRolesText)
	}
	if languages := renderSourceInventoryLanguageCounts(languageCounts); languages != "" {
		fmt.Fprintf(&b, " languages=%s", languages)
	}
	b.WriteByte('\n')
	if len(groups) > 0 {
		fmt.Fprintf(&b, "- expand a scope before reading many files: `%s`\n",
			sourceInventoryCascadeRepoMapCall(sourceInventoryLensQueryPath(query), "<scope>", nil, roles, attributeRoles, 24, ""))
	}
	if totalMembers > 0 {
		fmt.Fprintf(&b, "- page the current checklist instead of widening blindly: `%s`\n",
			sourceInventoryCascadeRepoMapCall(sourceInventoryLensQueryPath(query), "", sourceInventoryGroupScopesFromQuery(query, observation), roles, attributeRoles, 24, "<next_cursor>"))
	}
	b.WriteString("- after you choose a candidate from a narrower lens, verify with `read_file` or `grep` before citing it as evidence.\n")
	if len(groups) == 0 {
		return strings.TrimSpace(b.String())
	}
	b.WriteString("\nSuggested cascade expansions (model chooses which ones match the user's intent):\n")
	rendered := 0
	for _, group := range groups {
		if rendered >= maxGroups {
			break
		}
		if len(group.Files) == 0 && group.candidateCount == 0 {
			continue
		}
		rendered++
		fmt.Fprintf(&b, "- `%s` — files=%d candidates=%d",
			group.Scope, len(group.Files), group.candidateCount)
		if rolesText := renderSourceInventoryFileGroupRoleCounts(group); rolesText != "" {
			fmt.Fprintf(&b, " roles=%s", rolesText)
		}
		if languages := renderSourceInventoryFileGroupLanguageCounts(group); languages != "" {
			fmt.Fprintf(&b, " languages=%s", languages)
		}
		fmt.Fprintf(&b, " — next `%s`\n",
			sourceInventoryCascadeRepoMapCall(sourceInventoryLensQueryPath(query), group.Scope, nil, roles, attributeRoles, 24, ""))
	}
	if len(groups) > rendered {
		fmt.Fprintf(&b, "\nshowing %d of %d expandable scope groups; use `cursor`/narrower `scope` to continue without reading every file.\n", rendered, len(groups))
	}
	return strings.TrimSpace(b.String())
}

func sourceInventoryObservationRoles(observation types.SourceInventoryObservation) []types.AnswerCandidateRole {
	seen := map[types.AnswerCandidateRole]bool{}
	var roles []types.AnswerCandidateRole
	for _, set := range observation.Sets {
		if set.Role == types.AnswerCandidateRoleUnknown || seen[set.Role] || len(set.Members) == 0 {
			continue
		}
		seen[set.Role] = true
		roles = append(roles, set.Role)
	}
	return roles
}

func sourceInventoryObservationAttributeRoles(observation types.SourceInventoryObservation) []types.AnswerCandidateRole {
	seen := map[types.AnswerCandidateRole]bool{}
	var roles []types.AnswerCandidateRole
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			for _, attr := range member.Attributes {
				if attr.Role == types.AnswerCandidateRoleUnknown || seen[attr.Role] {
					continue
				}
				seen[attr.Role] = true
				roles = append(roles, attr.Role)
			}
		}
	}
	return roles
}

func sourceInventoryLensQueryPath(query types.SourceInventoryLensQuery) string {
	p := strings.TrimSpace(strings.ReplaceAll(query.Path, `\`, `/`))
	if p == "" {
		return "<same repo path>"
	}
	return p
}

func sourceInventoryCascadeRepoMapCall(toolPath, scope string, scopes []string, roles, attributeRoles []types.AnswerCandidateRole, topN int, cursor string) string {
	parts := []string{
		`"path": ` + strconv.Quote(toolPath),
		`"view": "source_inventory"`,
	}
	scope = strings.TrimSpace(scope)
	if scope != "" {
		parts = append(parts, `"scope": `+strconv.Quote(scope))
	} else if scopesText := sourceInventoryStringJSONArray(scopes); scopesText != "" {
		parts = append(parts, `"scopes": `+scopesText)
	}
	if rolesText := sourceInventoryRoleJSONArray(roles); rolesText != "" {
		parts = append(parts, `"roles": `+rolesText)
	}
	if attrRolesText := sourceInventoryRoleJSONArray(attributeRoles); attrRolesText != "" {
		parts = append(parts, `"attribute_roles": `+attrRolesText)
	}
	parts = append(parts, `"include_counts": true`, `"include_attributes": true`)
	if topN > 0 {
		parts = append(parts, `"top_n": `+strconvItoa(topN))
	}
	if cursor = strings.TrimSpace(cursor); cursor != "" {
		parts = append(parts, `"cursor": `+strconv.Quote(cursor))
	}
	return "repo_map {" + strings.Join(parts, ", ") + "}"
}

func sourceInventoryStringJSONArray(items []string) string {
	if len(items) == 0 {
		return ""
	}
	seen := map[string]bool{}
	var parts []string
	for _, item := range items {
		item = strings.TrimSpace(strings.ReplaceAll(item, `\`, `/`))
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		parts = append(parts, strconv.Quote(item))
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func sourceInventoryRoleJSONArray(roles []types.AnswerCandidateRole) string {
	if len(roles) == 0 {
		return ""
	}
	seen := map[types.AnswerCandidateRole]bool{}
	var parts []string
	for _, role := range roles {
		if role == types.AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		parts = append(parts, strconv.Quote(string(role)))
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func renderSourceInventoryFileGroupRoleCounts(group sourceInventorySuggestedFileGroup) string {
	counts := map[types.AnswerCandidateRole]int{}
	for _, file := range group.Files {
		for role, count := range file.RoleCounts {
			counts[role] += count
		}
	}
	return renderSourceInventoryRoleCounts(counts)
}

func renderSourceInventoryFileGroupLanguageCounts(group sourceInventorySuggestedFileGroup) string {
	counts := map[string]int{}
	for _, file := range group.Files {
		for language, count := range file.Languages {
			counts[language] += count
		}
	}
	return renderSourceInventoryLanguageCounts(counts)
}

// RenderSourceInventoryObservationView renders a compact model-facing repo lens
// view. The underlying observation remains structured in MutableState; this
// markdown is only a readable checklist for the current tool result.
func RenderSourceInventoryObservationView(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) string {
	if !observation.IsActive() {
		return "Repo Lens: no source-inventory observation is available for the requested typed scope/role slice. Try a narrower `scope` or provide `roles` that match repo-map candidate roles."
	}
	cascadeView := renderSourceInventoryCascadeGuideView(observation, query, 16)
	groupedView := renderSourceInventoryGroupedObservationView(observation, query)
	suggestedFilesView := renderSourceInventoryCandidateFileSamplesView(observation, query)
	topN := query.TopN
	if topN <= 0 {
		topN = 60
	}
	if groupedView != "" && query.TopN <= 0 {
		topN = 24
	}
	offset := sourceInventoryLensQueryOffset(query)
	includeCounts := query.IncludeCounts
	includeAttributes := query.IncludeAttributes
	var b strings.Builder
	b.WriteString("# Repo Lens: Source Inventory\n\n")
	b.WriteString("This is a repo-map observation checklist for navigation and mechanical member/count checks. It is not final answer text; semantic conclusions still need grounded evidence.\n\n")
	if len(observation.Scopes) > 0 {
		fmt.Fprintf(&b, "- scopes: `%s`\n", strings.Join(observation.Scopes, "`, `"))
	}
	if len(observation.Provenance) > 0 {
		fmt.Fprintf(&b, "- provenance: `%s`\n", strings.Join(observation.Provenance, "`, `"))
	}
	if len(observation.Lens) > 0 {
		fmt.Fprintf(&b, "- lens: `%s`\n", strings.Join(observation.Lens, "`, `"))
	}
	if roles := sourceInventoryLensQueryAttributeRoles(query); len(roles) > 0 {
		labels := make([]string, 0, len(roles))
		for _, role := range roles {
			labels = append(labels, string(role))
		}
		fmt.Fprintf(&b, "- attribute_roles: `%s`\n", strings.Join(labels, "`, `"))
	}
	if includeCounts {
		totalMembers := 0
		for _, set := range observation.Sets {
			totalMembers += len(set.Members)
		}
		fmt.Fprintf(&b, "- total_visible_member_count: %d\n", totalMembers)
	}
	if offset > 0 {
		fmt.Fprintf(&b, "- page_offset: %d\n", offset)
	}
	b.WriteByte('\n')
	if cascadeView != "" {
		b.WriteString(cascadeView)
		b.WriteString("\n\n")
	}
	if groupedView != "" {
		b.WriteString(groupedView)
		b.WriteString("\n\n")
	}
	if suggestedFilesView != "" {
		b.WriteString(suggestedFilesView)
		b.WriteString("\n\n")
	}
	emitted := 0
	visited := 0
	total := 0
	for _, set := range observation.Sets {
		total += len(set.Members)
	}
	for _, set := range observation.Sets {
		if len(set.Members) == 0 || emitted >= topN {
			continue
		}
		setRowsRemaining := len(set.Members)
		if offset > visited {
			skipInSet := minInt(offset-visited, len(set.Members))
			setRowsRemaining -= skipInSet
		}
		if setRowsRemaining <= 0 {
			visited += len(set.Members)
			continue
		}
		if includeCounts {
			fmt.Fprintf(&b, "## %s (%s)\n\ncount=%d complete=%t\n\n",
				types.SourceInventoryAdvisoryRoleLabel(set.Role), set.Role, set.Count, set.Complete)
		} else {
			fmt.Fprintf(&b, "## %s (%s)\n\ncomplete=%t\n\n",
				types.SourceInventoryAdvisoryRoleLabel(set.Role), set.Role, set.Complete)
		}
		for _, member := range set.Members {
			if visited < offset {
				visited++
				continue
			}
			if emitted >= topN {
				break
			}
			line := renderSourceInventoryObservationMember(member, includeAttributes)
			visited++
			if line == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteByte('\n')
			emitted++
		}
		b.WriteByte('\n')
	}
	if total > emitted {
		end := offset + emitted
		if end > total {
			end = total
		}
		fmt.Fprintf(&b, "---\nshowing rows [%d,%d) of %d member rows in this tool result; the structured observation stored in run state preserves the full set.\n", offset, end, total)
		if end < total {
			fmt.Fprintf(&b, "next_cursor=%d\n", end)
		}
	}
	return strings.TrimSpace(b.String())
}

func renderSourceInventoryCandidateFileSamplesView(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) string {
	return renderSourceInventoryCandidateFileSamplesViewWithLimits(observation, query, 16, 3, 3)
}

func renderSourceInventoryCandidateFileSamplesViewWithLimits(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery, maxGroups, maxFilesPerGroup, maxItemsPerFile int) string {
	if !observation.IsActive() || sourceInventoryLensQueryOffset(query) > 0 {
		return ""
	}
	if maxGroups <= 0 || maxFilesPerGroup <= 0 || maxItemsPerFile <= 0 {
		return ""
	}
	groups := sourceInventorySuggestedFileGroups(observation, query)
	if len(groups) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Candidate File Samples to Verify (advisory)\n\n")
	b.WriteString("These are bounded examples from the same structured rows, useful after choosing a scope from the cascade guide. They are not a hard whitelist and not final answer evidence.\n\n")
	renderedGroups := 0
	for _, group := range groups {
		if renderedGroups >= maxGroups {
			break
		}
		if len(group.Files) == 0 {
			continue
		}
		renderedGroups++
		fmt.Fprintf(&b, "- `%s` — file_count=%d candidate_count=%d — files: ",
			group.Scope, len(group.Files), group.candidateCount)
		parts := make([]string, 0, minInt(len(group.Files), maxFilesPerGroup))
		for _, file := range group.Files {
			if len(parts) >= maxFilesPerGroup {
				break
			}
			if part := renderSourceInventorySuggestedFile(file, maxItemsPerFile); part != "" {
				parts = append(parts, part)
			}
		}
		if len(parts) == 0 {
			b.WriteString("(none)")
		} else {
			b.WriteString(strings.Join(parts, "; "))
		}
		if len(group.Files) > len(parts) {
			fmt.Fprintf(&b, " (+%d more files)", len(group.Files)-len(parts))
		}
		b.WriteByte('\n')
	}
	if len(groups) > renderedGroups {
		fmt.Fprintf(&b, "\nshowing %d of %d file groups; the structured observation stored in run state preserves all candidate files.\n", renderedGroups, len(groups))
	}
	return strings.TrimSpace(b.String())
}

func renderSourceInventorySuggestedFile(file sourceInventorySuggestedFile, maxItems int) string {
	if strings.TrimSpace(file.File) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("`")
	b.WriteString(file.File)
	b.WriteString("`")
	var meta []string
	if languages := renderSourceInventoryLanguageCounts(file.Languages); languages != "" {
		meta = append(meta, "languages="+languages)
	}
	if roles := renderSourceInventoryRoleCounts(file.RoleCounts); roles != "" {
		meta = append(meta, "roles="+roles)
	}
	candidates := renderSourceInventorySuggestedFileCandidates(file.Candidates, maxItems)
	if candidates != "" {
		meta = append(meta, "candidates: "+candidates)
	}
	if len(meta) > 0 {
		b.WriteString(" (")
		b.WriteString(strings.Join(meta, "; "))
		b.WriteString(")")
	}
	return b.String()
}

func renderSourceInventorySuggestedFileCandidates(candidates []sourceInventorySuggestedFileCandidate, maxItems int) string {
	if len(candidates) == 0 || maxItems <= 0 {
		return ""
	}
	var parts []string
	for _, candidate := range candidates {
		if len(parts) >= maxItems {
			break
		}
		name := strings.TrimSpace(candidate.Name)
		if name == "" {
			continue
		}
		role := strings.TrimSpace(string(candidate.Role))
		if role == "" {
			role = "candidate"
		}
		item := role + " `" + name + "`"
		if candidate.Line > 0 {
			item += "@" + strconvItoa(candidate.Line)
		}
		parts = append(parts, item)
	}
	if len(parts) == 0 {
		return ""
	}
	if len(candidates) > len(parts) {
		parts = append(parts, fmt.Sprintf("+%d", len(candidates)-len(parts)))
	}
	return strings.Join(parts, ", ")
}

func sourceInventorySuggestedFileGroups(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) []sourceInventorySuggestedFileGroup {
	if !observation.IsActive() {
		return nil
	}
	groupScopes := sourceInventoryGroupScopesFromQuery(query, observation)
	groupOrder := map[string]int{}
	for i, scope := range groupScopes {
		groupOrder[scope] = i
	}
	groupsByScope := map[string]*sourceInventorySuggestedFileGroup{}
	filesByScope := map[string]map[string]*sourceInventorySuggestedFile{}
	add := func(scope, file, language string, role types.AnswerCandidateRole, name string, line int) {
		file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
		if file == "" {
			return
		}
		scope = strings.Trim(strings.TrimSpace(strings.ReplaceAll(scope, `\`, `/`)), "/")
		if scope == "" {
			scope = "."
		}
		group := groupsByScope[scope]
		if group == nil {
			order := len(groupOrder) + len(groupsByScope)
			if explicit, ok := groupOrder[scope]; ok {
				order = explicit
			}
			group = &sourceInventorySuggestedFileGroup{Scope: scope, order: order}
			groupsByScope[scope] = group
			filesByScope[scope] = map[string]*sourceInventorySuggestedFile{}
		}
		files := filesByScope[scope]
		item := files[file]
		if item == nil {
			item = &sourceInventorySuggestedFile{
				File:       file,
				order:      len(files),
				RoleCounts: map[types.AnswerCandidateRole]int{},
				Languages:  map[string]int{},
				seen:       map[string]bool{},
			}
			files[file] = item
			group.Files = append(group.Files, *item)
		}
		key := sourceInventorySuggestedCandidateKey(role, name, line)
		if key == "" || item.seen[key] {
			return
		}
		item.seen[key] = true
		item.Candidates = append(item.Candidates, sourceInventorySuggestedFileCandidate{Name: name, Role: role, Line: line})
		if role != types.AnswerCandidateRoleUnknown {
			item.RoleCounts[role]++
		}
		if language = strings.TrimSpace(language); language != "" {
			item.Languages[language]++
		}
		group.candidateCount++
		group.Files[item.order] = *item
	}
	addMember := func(member types.SourceInventoryObservationMember) {
		file := sourceInventoryObservationMemberFile(member)
		if file != "" {
			scope := sourceInventorySuggestedFileScope(file, groupScopes, observation.Scopes)
			add(scope, file, member.Language, member.Role, member.Name, member.Line)
		}
		for _, attr := range member.Attributes {
			attrFile := strings.Trim(strings.TrimSpace(strings.ReplaceAll(attr.File, `\`, `/`)), "/")
			if attrFile == "" {
				continue
			}
			scope := sourceInventorySuggestedFileScope(attrFile, groupScopes, observation.Scopes)
			add(scope, attrFile, attr.Language, attr.Role, attr.Name, attr.Line)
		}
	}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			addMember(member)
		}
	}
	groups := make([]sourceInventorySuggestedFileGroup, 0, len(groupsByScope))
	for _, group := range groupsByScope {
		sort.SliceStable(group.Files, func(i, j int) bool {
			if group.Files[i].order != group.Files[j].order {
				return group.Files[i].order < group.Files[j].order
			}
			return group.Files[i].File < group.Files[j].File
		})
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].order != groups[j].order {
			return groups[i].order < groups[j].order
		}
		return groups[i].Scope < groups[j].Scope
	})
	return groups
}

func sourceInventorySuggestedCandidateKey(role types.AnswerCandidateRole, name string, line int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return string(role) + "\x00" + name + "\x00" + strconvItoa(line)
}

func sourceInventorySuggestedFileScope(file string, groupScopes []string, observationScopes []string) string {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	if file == "" {
		return ""
	}
	if len(groupScopes) > 1 {
		if scope := sourceInventoryBestMatchingScope(file, groupScopes); scope != "" {
			return scope
		}
	}
	baseScopes := groupScopes
	if len(baseScopes) == 0 {
		baseScopes = observationScopes
	}
	base := "."
	if len(baseScopes) == 1 {
		base = strings.Trim(strings.TrimSpace(strings.ReplaceAll(baseScopes[0], `\`, `/`)), "/")
		if base == "" {
			base = "."
		}
	}
	return sourceInventoryChildScopeForFile(file, base)
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

func sourceInventorySuggestedFilesForRequestedScope(groups []sourceInventorySuggestedFileGroup, requestedDir, requestedRel string) []sourceInventorySuggestedFile {
	requestedDir = strings.Trim(strings.TrimSpace(strings.ReplaceAll(requestedDir, `\`, `/`)), "/")
	if requestedDir == "" {
		requestedDir = "."
	}
	requestedRel = strings.Trim(strings.TrimSpace(strings.ReplaceAll(requestedRel, `\`, `/`)), "/")
	var files []sourceInventorySuggestedFile
	seen := map[string]bool{}
	addFile := func(file sourceInventorySuggestedFile) {
		if file.File == "" || file.File == requestedRel || seen[file.File] {
			return
		}
		seen[file.File] = true
		files = append(files, file)
	}
	for _, group := range groups {
		scope := strings.Trim(strings.TrimSpace(strings.ReplaceAll(group.Scope, `\`, `/`)), "/")
		if scope == "" {
			scope = "."
		}
		if scope != requestedDir {
			continue
		}
		for _, file := range group.Files {
			addFile(file)
		}
	}
	if len(files) > 0 {
		return files
	}
	for _, group := range groups {
		for _, file := range group.Files {
			if path.Dir(file.File) == requestedDir {
				addFile(file)
			}
		}
	}
	return files
}

func renderSourceInventoryGroupedObservationView(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) string {
	if !observation.IsActive() || sourceInventoryLensQueryOffset(query) > 0 {
		return ""
	}
	groups := sourceInventoryObservationScopeGroups(observation, query)
	if len(groups) < 2 {
		return ""
	}
	const (
		maxGroups          = 48
		maxCandidatesInRow = 5
	)
	var b strings.Builder
	b.WriteString("## Scope-grouped Candidate View (advisory)\n\n")
	b.WriteString("Use this navigation view to choose what to verify next. It groups the same structured rows by scope; it does not assert final answer membership or entry-point identity.\n\n")
	b.WriteString("If the downstream task needs one candidate per scope, treat groups with `candidate_count` other than 1 as ambiguous and verify the listed files before selecting or excluding a candidate.\n\n")
	renderedGroups := 0
	for _, group := range groups {
		if renderedGroups >= maxGroups {
			break
		}
		if len(group.Candidates) == 0 {
			continue
		}
		renderedGroups++
		fmt.Fprintf(&b, "- `%s` — candidate_count=%d", group.Scope, len(group.Candidates))
		if roles := renderSourceInventoryRoleCounts(group.RoleCounts); roles != "" {
			fmt.Fprintf(&b, " roles=%s", roles)
		}
		if languages := renderSourceInventoryLanguageCounts(group.Languages); languages != "" {
			fmt.Fprintf(&b, " languages=%s", languages)
		}
		b.WriteString(" — top_candidates: ")
		parts := make([]string, 0, maxCandidatesInRow)
		for _, candidate := range group.Candidates {
			if len(parts) >= maxCandidatesInRow {
				break
			}
			if part := renderSourceInventoryGroupedCandidate(candidate); part != "" {
				parts = append(parts, part)
			}
		}
		if len(parts) == 0 {
			b.WriteString("(none)")
		} else {
			b.WriteString(strings.Join(parts, "; "))
		}
		if len(group.Candidates) > len(parts) {
			fmt.Fprintf(&b, " (+%d more)", len(group.Candidates)-len(parts))
		}
		b.WriteByte('\n')
	}
	if len(groups) > renderedGroups {
		fmt.Fprintf(&b, "\nshowing %d of %d scope groups; the structured observation stored in run state preserves all grouped rows.\n", renderedGroups, len(groups))
	}
	return strings.TrimSpace(b.String())
}

func sourceInventoryObservationScopeGroups(observation types.SourceInventoryObservation, query types.SourceInventoryLensQuery) []sourceInventoryObservationScopeGroup {
	groupScopes := sourceInventoryGroupScopesFromQuery(query, observation)
	groupOrder := map[string]int{}
	for i, scope := range groupScopes {
		groupOrder[scope] = i
	}
	groupsByScope := map[string]*sourceInventoryObservationScopeGroup{}
	addCandidate := func(member types.SourceInventoryObservationMember) {
		if strings.TrimSpace(member.Name) == "" || !sourceInventoryObservationMemberGroupable(member) {
			return
		}
		scope := sourceInventoryObservationMemberGroupScope(member, groupScopes, observation.Scopes)
		if scope == "" {
			return
		}
		group := groupsByScope[scope]
		if group == nil {
			order := len(groupOrder) + len(groupsByScope)
			if explicit, ok := groupOrder[scope]; ok {
				order = explicit
			}
			group = &sourceInventoryObservationScopeGroup{
				Scope:      scope,
				order:      order,
				RoleCounts: map[types.AnswerCandidateRole]int{},
				Languages:  map[string]int{},
			}
			groupsByScope[scope] = group
		}
		group.Candidates = append(group.Candidates, member)
		group.RoleCounts[member.Role]++
		if lang := strings.TrimSpace(member.Language); lang != "" {
			group.Languages[lang]++
		}
		for _, attr := range member.Attributes {
			if lang := strings.TrimSpace(attr.Language); lang != "" {
				group.Languages[lang]++
			}
			if attr.Role != types.AnswerCandidateRoleUnknown {
				group.RoleCounts[attr.Role]++
			}
		}
	}
	for _, set := range observation.Sets {
		for _, member := range set.Members {
			addCandidate(member)
		}
	}
	groups := make([]sourceInventoryObservationScopeGroup, 0, len(groupsByScope))
	for _, group := range groupsByScope {
		sourceInventorySortObservationGroupCandidates(group.Candidates)
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].order != groups[j].order {
			return groups[i].order < groups[j].order
		}
		return groups[i].Scope < groups[j].Scope
	})
	return groups
}

func sourceInventoryObservationMemberGroupable(member types.SourceInventoryObservationMember) bool {
	switch member.Role {
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
		return true
	default:
		return len(member.Attributes) > 0
	}
}

func sourceInventoryGroupScopesFromQuery(query types.SourceInventoryLensQuery, observation types.SourceInventoryObservation) []string {
	seen := map[string]bool{}
	var scopes []string
	add := func(raw string) {
		scope := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
		if scope == "" {
			return
		}
		if scope == "." || scope == "./" || scope == "/" {
			scope = "."
		} else {
			scope = normalizeSourceInventoryScopeSurface(scope)
		}
		if scope == "" {
			scope = "."
		}
		if seen[scope] {
			return
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}
	for _, scope := range query.Scopes {
		add(scope)
	}
	if len(scopes) == 0 {
		for _, scope := range observation.Scopes {
			add(scope)
		}
	}
	return scopes
}

func sourceInventoryObservationMemberGroupScope(member types.SourceInventoryObservationMember, groupScopes []string, observationScopes []string) string {
	file := sourceInventoryObservationMemberFile(member)
	if len(groupScopes) > 1 {
		if scope := sourceInventoryBestMatchingScope(file, groupScopes); scope != "" {
			return scope
		}
	}
	baseScopes := groupScopes
	if len(baseScopes) == 0 {
		baseScopes = observationScopes
	}
	base := "."
	if len(baseScopes) == 1 {
		base = strings.Trim(strings.TrimSpace(strings.ReplaceAll(baseScopes[0], `\`, `/`)), "/")
		if base == "" {
			base = "."
		}
	}
	if file == "" {
		if base != "." {
			return base
		}
		return strings.TrimSpace(member.Name)
	}
	return sourceInventoryChildScopeForFile(file, base)
}

func sourceInventoryObservationMemberFile(member types.SourceInventoryObservationMember) string {
	file := strings.Trim(strings.TrimSpace(strings.ReplaceAll(member.File, `\`, `/`)), "/")
	if file != "" {
		return file
	}
	ref := strings.TrimSpace(member.SupportRef)
	if ref == "" {
		return ""
	}
	if _, loc, ok := aggregateMemberSupportRefParts(ref); ok {
		file, _ := aggregateLocationParts(loc)
		return strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	}
	return ""
}

func sourceInventoryBestMatchingScope(file string, scopes []string) string {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	best := ""
	for _, raw := range scopes {
		scope := strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)), "/")
		if scope == "" || scope == "." {
			if best == "" {
				best = "."
			}
			continue
		}
		if file == scope || strings.HasPrefix(file, scope+"/") {
			if len(scope) > len(best) {
				best = scope
			}
		}
	}
	return best
}

func sourceInventoryChildScopeForFile(file, base string) string {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	base = strings.Trim(strings.TrimSpace(strings.ReplaceAll(base, `\`, `/`)), "/")
	if file == "" {
		return ""
	}
	if base == "" || base == "." {
		parts := strings.Split(file, "/")
		if len(parts) <= 1 {
			return file
		}
		return parts[0]
	}
	if file == base {
		return base
	}
	if strings.HasPrefix(file, base+"/") {
		rest := strings.TrimPrefix(file, base+"/")
		parts := strings.Split(rest, "/")
		if len(parts) == 0 || parts[0] == "" {
			return base
		}
		return strings.Trim(base+"/"+parts[0], "/")
	}
	return path.Dir(file)
}

func sourceInventorySortObservationGroupCandidates(candidates []types.SourceInventoryObservationMember) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Exported != candidates[j].Exported {
			return candidates[i].Exported
		}
		if candidates[i].Role != candidates[j].Role {
			return string(candidates[i].Role) < string(candidates[j].Role)
		}
		if candidates[i].File != candidates[j].File {
			return candidates[i].File < candidates[j].File
		}
		if candidates[i].Line != candidates[j].Line {
			if candidates[i].Line == 0 {
				return false
			}
			if candidates[j].Line == 0 {
				return true
			}
			return candidates[i].Line < candidates[j].Line
		}
		return candidates[i].Name < candidates[j].Name
	})
}

func renderSourceInventoryGroupedCandidate(member types.SourceInventoryObservationMember) string {
	name := strings.TrimSpace(member.Name)
	if name == "" {
		return ""
	}
	role := strings.TrimSpace(string(member.Role))
	if role == "" {
		role = "candidate"
	}
	item := role + " `" + name + "`"
	loc := sourceInventoryObservationMemberFile(member)
	if loc != "" && member.Line > 0 {
		loc = loc + ":" + strconvItoa(member.Line)
	}
	if loc != "" {
		item += " @ " + loc
	}
	if len(member.Attributes) > 0 {
		if attrs := renderSourceInventoryObservationAttributes(member.Attributes); attrs != "" {
			item += " {" + attrs + "}"
		}
	}
	return item
}

func renderSourceInventoryRoleCounts(counts map[types.AnswerCandidateRole]int) string {
	if len(counts) == 0 {
		return ""
	}
	var keys []types.AnswerCandidateRole
	for role, count := range counts {
		if role == types.AnswerCandidateRoleUnknown || count <= 0 {
			continue
		}
		keys = append(keys, role)
	}
	sort.Slice(keys, func(i, j int) bool { return string(keys[i]) < string(keys[j]) })
	parts := make([]string, 0, len(keys))
	for _, role := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", role, counts[role]))
	}
	return strings.Join(parts, ",")
}

func renderSourceInventoryLanguageCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	var keys []string
	for lang, count := range counts {
		if strings.TrimSpace(lang) == "" || count <= 0 {
			continue
		}
		keys = append(keys, lang)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, lang := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", lang, counts[lang]))
	}
	return strings.Join(parts, ",")
}

func sourceInventoryLensQueryOffset(query types.SourceInventoryLensQuery) int {
	if query.Offset > 0 {
		return query.Offset
	}
	cursor := strings.TrimSpace(query.Cursor)
	if cursor == "" {
		return 0
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func renderSourceInventoryObservationMember(member types.SourceInventoryObservationMember, includeAttributes bool) string {
	name := strings.TrimSpace(member.Name)
	if name == "" {
		return ""
	}
	parts := []string{"`" + name + "`"}
	loc := strings.TrimSpace(member.File)
	if loc != "" && member.Line > 0 {
		loc = loc + ":" + strconvItoa(member.Line)
	}
	if loc != "" {
		parts = append(parts, "@ "+loc)
	}
	if member.Language != "" {
		parts = append(parts, "language="+member.Language)
	}
	if member.SupportRef != "" {
		parts = append(parts, "support_ref=`"+member.SupportRef+"`")
	}
	if member.CoverageState != "" {
		parts = append(parts, "coverage="+string(member.CoverageState))
	}
	if note := strings.TrimSpace(member.Note); note != "" {
		parts = append(parts, "note: "+note)
	}
	if includeAttributes {
		if attrs := renderSourceInventoryObservationAttributes(member.Attributes); attrs != "" {
			parts = append(parts, attrs)
		}
	}
	return strings.Join(parts, " — ")
}

func renderSourceInventoryObservationAttributes(attrs []types.SourceInventoryObservationAttribute) string {
	if len(attrs) == 0 {
		return ""
	}
	const max = 4
	var parts []string
	for _, attr := range attrs {
		if len(parts) >= max {
			break
		}
		name := strings.TrimSpace(attr.Name)
		if name == "" {
			continue
		}
		loc := strings.TrimSpace(attr.File)
		if loc != "" && attr.Line > 0 {
			loc = loc + ":" + strconvItoa(attr.Line)
		}
		role := strings.TrimSpace(string(attr.Role))
		if role == "" {
			role = "candidate"
		}
		item := role + " `" + name + "`"
		if loc != "" {
			item += " @ " + loc
		}
		if attr.Ambiguity != "" {
			item += " ambiguity=" + attr.Ambiguity
		}
		parts = append(parts, item)
	}
	if len(parts) == 0 {
		return ""
	}
	suffix := ""
	if len(attrs) > len(parts) {
		suffix = fmt.Sprintf(" (+%d more)", len(attrs)-len(parts))
	}
	return "attributes: " + strings.Join(parts, "; ") + suffix
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
		profile, advisoryOnly, provenance = sourceInventoryAdvisoryProfile(ctx, graph)
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
		return types.SourceInventoryAdvisory{}
	}
	sets := sourceInventoryCandidateSets(ctx, graph, scopes, profile, attributeRoles, explicitAttributeRoles)
	if len(sets) == 0 {
		return types.SourceInventoryAdvisory{}
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
		if len(set.candidates) == 0 {
			continue
		}
		if !set.complete {
			advisory.Complete = false
		}
		advisorySet := types.SourceInventoryAdvisorySet{
			Role:       set.role,
			Complete:   set.complete,
			Candidates: make([]types.SourceInventoryAdvisoryCandidate, 0, len(set.candidates)),
		}
		for _, candidate := range set.candidates {
			attrs := make([]types.SourceInventoryAdvisoryAttribute, 0, len(candidate.attributes))
			for _, attr := range candidate.attributes {
				attrs = append(attrs, types.SourceInventoryAdvisoryAttribute{
					Member:     attr.member,
					Key:        attr.key,
					SupportRef: attr.supportRef,
					Note:       sourceInventoryCandidateNote(attr),
					Role:       attr.role,
					Exported:   attr.exported,
					File:       attr.file,
					Line:       attr.line,
					Language:   attr.language,
				})
			}
			advisorySet.Candidates = append(advisorySet.Candidates, types.SourceInventoryAdvisoryCandidate{
				Member:     candidate.member,
				Key:        candidate.key,
				SupportRef: candidate.supportRef,
				Note:       sourceInventoryCandidateNote(candidate),
				Role:       candidate.role,
				Exported:   candidate.exported,
				File:       candidate.file,
				Line:       candidate.line,
				Language:   candidate.language,
				Attributes: attrs,
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

func sourceInventoryLensQueryScopes(ctx *types.BusContext, graph *repotypes.Graph, query types.SourceInventoryLensQuery) []string {
	if len(query.Scopes) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var scopes []string
	for _, raw := range query.Scopes {
		scope := sourceInventoryScopeForLensSurface(graph, raw)
		if scope == "" || seen[scope] {
			continue
		}
		// "." is a repo-lens selector for the active repo root, not an answer
		// evidence path. Keep the per-file source-scope checks at candidate
		// construction time so tests/docs/config rows are still filtered by the
		// typed source scope instead of by this synthetic selector.
		if scope != "." && ctx != nil && !aggregateEvidenceSourceInRequestedScope(ctx, scope) {
			continue
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}
	return sourceInventoryDedupeScopeAliases(scopes)
}

func sourceInventoryDedupeScopeAliases(scopes []string) []string {
	if len(scopes) <= 1 {
		return scopes
	}
	normalized := make([]string, 0, len(scopes))
	seen := map[string]bool{}
	for _, raw := range scopes {
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

func sourceInventoryAdvisoryProfile(ctx *types.BusContext, graph *repotypes.Graph) (*types.SourceInventoryProfile, bool, []string) {
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
	sets := sourceInventoryCandidateSets(ctx, graph, scopes, profile, nil, false)
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

func sourceInventoryCandidateSets(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool) map[types.AnswerCandidateRole]sourceInventoryCandidateSet {
	out := map[types.AnswerCandidateRole]sourceInventoryCandidateSet{}
	for _, role := range profile.PrincipalTargetRoles() {
		switch {
		case role == types.AnswerCandidateRoleFile:
			out[role] = sourceInventoryFileCandidates(ctx, graph, scopes, profile, attributeRoles, explicitAttributeRoles)
		case role == types.AnswerCandidateRoleConfigFile:
			out[role] = sourceInventoryConfigFileCandidates(ctx, graph, scopes, profile, attributeRoles, explicitAttributeRoles)
		case role == types.AnswerCandidateRolePackage:
			out[role] = sourceInventoryPackageCandidates(ctx, graph, scopes, profile, attributeRoles, explicitAttributeRoles)
		case role == types.AnswerCandidateRoleType &&
			profile.TypeUnderlying == types.SourceInventoryTypeUnderlyingString &&
			profile.RequiresConstSet:
			out[role] = sourceInventoryGoStringEnumCandidates(ctx, graph, scopes, profile)
		default:
			out[role] = sourceInventoryGraphCandidates(ctx, graph, scopes, profile, role)
		}
		if len(out[role].candidates) == 0 {
			delete(out, role)
		}
	}
	return out
}

func sourceInventoryFileCandidates(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: types.AnswerCandidateRoleFile, complete: sourceInventoryScopesHaveInventoryFiles(graph, scopes)}
	if graph == nil {
		return set
	}
	seen := map[string]bool{}
	for _, fi := range sourceInventoryScopedGraphFiles(graph, scopes, "") {
		if fi == nil || strings.TrimSpace(fi.RelPath) == "" || (strings.TrimSpace(fi.Language) == "" && !fi.IsSpecial) {
			continue
		}
		file := strings.Trim(strings.TrimSpace(strings.ReplaceAll(fi.RelPath, `\`, `/`)), "/")
		if file == "" || seen[file] || !aggregateEvidenceSourceInRequestedScope(ctx, file) {
			continue
		}
		seen[file] = true
		set.candidates = append(set.candidates, sourceInventoryCandidate{
			member:     file,
			key:        file,
			supportRef: file,
			note:       sourceInventoryFileCandidateNote(fi),
			role:       types.AnswerCandidateRoleFile,
			exported:   true,
			file:       file,
			language:   strings.TrimSpace(fi.Language),
			attributes: sourceInventoryFileAttributes(ctx, graph, file, attributeRoles, explicitAttributeRoles),
		})
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

func sourceInventoryConfigFileCandidates(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: types.AnswerCandidateRoleConfigFile, complete: sourceInventoryScopesHaveInventoryFiles(graph, scopes)}
	if graph == nil {
		return set
	}
	seen := map[string]bool{}
	for _, fi := range sourceInventoryScopedGraphFiles(graph, scopes, "") {
		if fi == nil || !fi.IsSpecial || strings.TrimSpace(fi.RelPath) == "" {
			continue
		}
		file := strings.Trim(strings.TrimSpace(strings.ReplaceAll(fi.RelPath, `\`, `/`)), "/")
		if file == "" || seen[file] || !aggregateEvidenceSourceInRequestedScope(ctx, file) {
			continue
		}
		seen[file] = true
		set.candidates = append(set.candidates, sourceInventoryCandidate{
			member:     file,
			key:        file,
			supportRef: file,
			note:       sourceInventoryConfigFileCandidateNote(fi),
			role:       types.AnswerCandidateRoleConfigFile,
			exported:   true,
			file:       file,
			language:   strings.TrimSpace(fi.Language),
			attributes: sourceInventoryFileAttributes(ctx, graph, file, attributeRoles, explicitAttributeRoles),
		})
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

func sourceInventoryPackageCandidates(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: types.AnswerCandidateRolePackage, complete: sourceInventoryScopesHaveInventoryFiles(graph, scopes)}
	if graph == nil {
		return set
	}
	buckets := map[string]*sourceInventoryPackageBucket{}
	for _, fi := range sourceInventoryScopedGraphFiles(graph, scopes, "") {
		if fi == nil || fi.IsSpecial || strings.TrimSpace(fi.RelPath) == "" || strings.TrimSpace(fi.Language) == "" {
			continue
		}
		if !aggregateEvidenceSourceInRequestedScope(ctx, fi.RelPath) {
			continue
		}
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
	for _, bucket := range buckets {
		member := sourceInventoryPackageBucketMember(bucket)
		key := strings.TrimSpace(bucket.dir)
		if member == "" || key == "" {
			continue
		}
		set.candidates = append(set.candidates, sourceInventoryCandidate{
			member:     member,
			key:        key,
			supportRef: key,
			note:       sourceInventoryPackageBucketNote(bucket),
			role:       types.AnswerCandidateRolePackage,
			exported:   true,
			file:       key,
			language:   sourceInventoryDominantMapKey(bucket.languages),
			attributes: sourceInventoryPackageBucketAttributes(ctx, graph, key, attributeRoles, explicitAttributeRoles),
		})
	}
	sourceInventorySortCandidates(set.candidates)
	return set
}

const (
	sourceInventoryMaxDefaultAttributes  = 4
	sourceInventoryMaxExplicitAttributes = 8
)

func sourceInventoryPackageBucketAttributes(ctx *types.BusContext, graph *repotypes.Graph, dir string, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool) []sourceInventoryCandidate {
	if graph == nil {
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
	for _, sym := range sourceInventoryGraphSymbols(graph) {
		if sym == nil || !sourceInventorySymbolInPackageDir(sym, dir) || !aggregateEvidenceSourceInRequestedScope(ctx, sym.File) {
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
		candidate := sourceInventoryCandidateForSymbol(sym, role, graph)
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

func sourceInventoryFileAttributes(ctx *types.BusContext, graph *repotypes.Graph, file string, attributeRoles []types.AnswerCandidateRole, explicitAttributeRoles bool) []sourceInventoryCandidate {
	if graph == nil || !explicitAttributeRoles {
		return nil
	}
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	if file == "" {
		return nil
	}
	limit := sourceInventoryAttributeLimit(true)
	seen := map[string]bool{}
	var out []sourceInventoryCandidate
	for _, sym := range sourceInventoryGraphSymbols(graph) {
		if sym == nil || strings.Trim(strings.ReplaceAll(sym.File, `\`, `/`), "/") != file || !aggregateEvidenceSourceInRequestedScope(ctx, sym.File) {
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
		candidate := sourceInventoryCandidateForSymbol(sym, role, graph)
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

func sourceInventoryGraphCandidates(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile, role types.AnswerCandidateRole) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: role, complete: sourceInventoryScopesHaveInventoryFiles(graph, scopes)}
	if graph == nil {
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
	for _, sym := range sourceInventoryGraphSymbols(graph) {
		if sym == nil || !sourceInventoryFileInScopes(sym.File, scopes) || !aggregateEvidenceSourceInRequestedScope(ctx, sym.File) {
			continue
		}
		candidateRole, ok := aggregateAnswerCandidateRoleForSymbol(sym)
		if !ok || candidateRole != role || answerDocumentSymbolMatchesExcludedRole(sym, excludedRoles) {
			continue
		}
		if !sourceInventorySymbolMatchesVisibility(sym, visibility) {
			continue
		}
		candidate := sourceInventoryCandidateForSymbol(sym, role, graph)
		key := sourceInventoryCandidateDedupeKey(candidate)
		if candidate.key == "" || key == "" || seen[key] {
			continue
		}
		seen[key] = true
		set.candidates = append(set.candidates, candidate)
	}
	sourceInventorySortCandidates(set.candidates)
	return set
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

func sourceInventoryGoStringEnumCandidates(ctx *types.BusContext, graph *repotypes.Graph, scopes []string, profile *types.SourceInventoryProfile) sourceInventoryCandidateSet {
	set := sourceInventoryCandidateSet{role: types.AnswerCandidateRoleType, complete: true}
	if graph == nil || ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		set.complete = false
		return set
	}
	files := sourceInventoryScopedGraphFiles(graph, scopes, "go")
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
		note := sourceInventoryCandidateNoteFromGraph(sym, sourceInventoryGraphLanguageForFile(graph, info.file))
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
			member:     name,
			key:        key,
			supportRef: name + ": " + aggregateSupportLocationKey(info.file, info.line),
			note:       note,
			role:       types.AnswerCandidateRoleType,
			exported:   exported,
			file:       info.file,
			line:       info.line,
			language:   "go",
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
	if graph == nil {
		return nil
	}
	var files []*repotypes.FileInfo
	for _, fi := range graph.Files {
		if fi == nil || strings.TrimSpace(fi.RelPath) == "" {
			continue
		}
		if language != "" && strings.TrimSpace(fi.Language) != language {
			continue
		}
		if sourceInventoryFileInScopes(fi.RelPath, scopes) {
			files = append(files, fi)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return files
}

func sourceInventoryFileInScopes(file string, scopes []string) bool {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	if file == "" {
		return false
	}
	for _, scope := range scopes {
		scope = strings.Trim(strings.TrimSpace(strings.ReplaceAll(scope, `\`, `/`)), "/")
		if scope == "" || scope == "." {
			return true
		}
		if file == scope || strings.HasPrefix(file, scope+"/") || aggregateReadFilePathMatchesQualifier(file, scope) {
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

func sourceInventoryGraphSymbols(graph *repotypes.Graph) []*repotypes.Symbol {
	if graph == nil {
		return nil
	}
	seen := map[*repotypes.Symbol]bool{}
	var out []*repotypes.Symbol
	if len(graph.SymbolByID) > 0 {
		for _, sym := range graph.SymbolByID {
			if sym != nil && !seen[sym] {
				seen[sym] = true
				out = append(out, sym)
			}
		}
	}
	for _, defs := range graph.SymbolDefs {
		for _, sym := range defs {
			if sym != nil && !seen[sym] {
				seen[sym] = true
				out = append(out, sym)
			}
		}
	}
	return out
}

func sourceInventoryCandidateForSymbol(sym *repotypes.Symbol, role types.AnswerCandidateRole, graph *repotypes.Graph) sourceInventoryCandidate {
	language := sourceInventoryGraphLanguageForFile(graph, sym.File)
	return sourceInventoryCandidate{
		member:     strings.TrimSpace(sym.Name),
		key:        aggregateMemberKey(sym.Name),
		supportRef: strings.TrimSpace(sym.Name) + ": " + aggregateSupportLocationKey(sym.File, sym.Line),
		note:       sourceInventoryCandidateNoteFromGraph(sym, language),
		role:       role,
		exported:   sym.Exported,
		file:       sym.File,
		line:       sym.Line,
		language:   language,
	}
}

func sourceInventoryCandidateNote(candidate sourceInventoryCandidate) string {
	return sourceInventoryCompactNote(candidate.note)
}

func sourceInventoryCandidateNoteFromGraph(sym *repotypes.Symbol, language string) string {
	if sym == nil {
		return ""
	}
	note := sourceInventoryCompactNote(sym.Doc)
	if !sourceInventoryGraphDocDescribesSymbol(note, sym.Name, language) {
		return ""
	}
	return note
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
