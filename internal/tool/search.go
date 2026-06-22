package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// searchBackend holds the detected search command.
// Detected once at first use; cached for the process lifetime.
//
// Tri-state:
//   - "rg"     — ripgrep found and runnable
//   - "grep"   — GNU/BSD grep found and runnable (or blind-call fallback)
//   - "native" — neither found; use the Go-native scanner in nativegrep.go
//
// The "native" state guarantees codrax keeps working on minimal
// environments (distroless/scratch containers, stripped CI images,
// Windows without Git-for-Windows) at the cost of slower scans.
var (
	searchOnce    sync.Once
	searchCommand string // "rg" / "grep" / "native"
	searchPath    string // actual executable path used for the backend; "" when native
)

// SearchCommand returns the selected search backend identifier.
// The result is cached after the first call.
func SearchCommand() string {
	searchOnce.Do(func() {
		if path := firstRunnablePath("rg", []string{"--version"}, windowsExtraCommandCandidates("rg")...); path != "" {
			searchCommand = "rg"
			searchPath = path
			logging.Info("search backend: ripgrep (%s)", path)
			return
		}
		if path := firstRunnablePath("grep", []string{"--version"}, windowsExtraCommandCandidates("grep")...); path != "" {
			searchCommand = "grep"
			searchPath = path
			logging.Info("search backend: grep (%s; ripgrep not found — install ripgrep for faster scans)", path)
			return
		}
		searchCommand = "native"
		searchPath = ""
		logging.Warning("search backend: native Go scanner (neither ripgrep nor grep found on PATH — install ripgrep for faster scans; codrax falls back to a pure-Go regex walker)")
	})
	return searchCommand
}

// SearchExecutable returns the actual executable path selected for the
// current search backend. Returns "" when the backend is native —
// callers MUST check UseNativeGrep() before shelling out.
func SearchExecutable() string {
	SearchCommand()
	return searchPath
}

// UseRipgrep returns true if ripgrep was detected as available.
func UseRipgrep() bool {
	return SearchCommand() == "rg"
}

// UseNativeGrep returns true when neither rg nor grep is on PATH and
// the Go-native scanner must be used. Callers in this package and in
// internal/agent check this to avoid exec.CommandContext on an empty
// SearchExecutable(), which would fail to start.
func UseNativeGrep() bool {
	return SearchCommand() == "native"
}

// The directory exclusion policy is centralised here so grep,
// list_files, keyword_search, repomap, and explorer all reason about
// the same structural noise.
//
// Two overlapping categories, exposed separately so callers can
// pick the semantics they need:
//
//   - ExcludeDirsAnyLevel entries match at any directory depth.
//     They are dependency trees, VCS internals, build output, and
//     IDE state — directories that are structurally never part of
//     the codebase regardless of where they appear.
//
//   - ExcludeDirsRootOnly entries match only at the repo root.
//     They are runtime artifacts ("memory", "logs") and eval
//     fixtures ("eval") — names whose semantics are "this special
//     top-level folder codrax maintains", and which a nested
//     `internal/memory/` package must NOT be accidentally matched
//     against.
//
// New code should consume these slices through SearchDirFilter, which
// preserves root-only semantics and explicit-target overrides.
var ExcludeDirsAnyLevel = []string{
	".git", ".hg", ".svn",
	".codrax", // codrax's own per-repo state (logs / blob / worktrees / plans). Showed up in `list_files recursive=false` output and confused LLMs into thinking it was project state.
	"node_modules", "vendor", "__pycache__", ".tox", ".venv", "venv", ".mypy_cache", ".pytest_cache",
	// Vendored third-party trees in their other common spellings —
	// the same class as "vendor": copied-in dependency code (grammar
	// test corpora, forked libs) that is structurally never the
	// user's navigation target. Surfaced by the 2026-06-12 repomap_v3
	// refresh: tree-sitter grammar corpus sources under
	// internal/thirdparty/ outranked real extractors on path-token
	// matches. Deliberately NOT excluding generic "corpus" dirs — a
	// user repo can own a legitimate corpus directory.
	"thirdparty", "third_party", "third-party",
	".idea", ".vscode", ".vs",
	"target", "dist", "build", ".gradle", ".cargo",
	".next", ".nuxt", ".turbo", // common JS framework output dirs
	".pnpm-store", // alternative to node_modules
}

// ExcludeDirPatternsAnyLevel extends the exact-name directory list
// with wildcard/prefix patterns for transient cache roots that are
// created by tooling inside a repo checkout. These are infrastructure
// artifacts, not project content:
//   - .gotmp*   — per-run temp roots used by codrax/go tooling
//   - .gocache* — local Go build/module cache roots parked in-repo
//
// Kept central so shell-backed search and Go-native walkers apply the
// same policy instead of drifting.
var ExcludeDirPatternsAnyLevel = func() []string {
	out := make([]string, 0, len(ExcludeDirsAnyLevel)+2)
	out = append(out, ExcludeDirsAnyLevel...)
	out = append(out, ".gotmp*", ".gocache*")
	return out
}()

// ExcludeDirsRootOnly entries are matched only when they sit at
// position 0 of a relative path. See ExcludeDirsAnyLevel for
// rationale.
var ExcludeDirsRootOnly = []string{
	"logs", "memory", "eval",
}

// ExcludeDirsAnyLevelSet is the map form of ExcludeDirsAnyLevel,
// built once at package init for O(1) membership checks.
var ExcludeDirsAnyLevelSet = func() map[string]bool {
	m := make(map[string]bool, len(ExcludeDirsAnyLevel))
	for _, d := range ExcludeDirsAnyLevel {
		m[d] = true
	}
	return m
}()

// ExcludeDirsRootOnlySet is the map form of ExcludeDirsRootOnly.
var ExcludeDirsRootOnlySet = func() map[string]bool {
	m := make(map[string]bool, len(ExcludeDirsRootOnly))
	for _, d := range ExcludeDirsRootOnly {
		m[d] = true
	}
	return m
}()

var (
	searchRuntimeArtifactRootsMu sync.RWMutex
	searchRuntimeArtifactRoots   []string
)

// SetSearchRuntimeArtifactRoots installs repo-relative broad-search exclusion
// roots from runtime configuration. These are customer/workspace-specific
// generated/runtime/report directories, not a source taxonomy. Explicit tool
// targets under such a root are still honored by SearchDirFilter.
func SetSearchRuntimeArtifactRoots(roots []string) {
	normalized := normalizeSearchRuntimeArtifactRootSlice(roots)
	searchRuntimeArtifactRootsMu.Lock()
	defer searchRuntimeArtifactRootsMu.Unlock()
	searchRuntimeArtifactRoots = normalized
}

// SearchRuntimeArtifactRoots returns the currently configured repo-relative
// broad-search exclusion roots.
func SearchRuntimeArtifactRoots() []string {
	searchRuntimeArtifactRootsMu.RLock()
	defer searchRuntimeArtifactRootsMu.RUnlock()
	return append([]string(nil), searchRuntimeArtifactRoots...)
}

// DirNameMatchesExcludePattern reports whether name matches any exact
// or glob-style directory exclude pattern. Patterns are the same ones
// passed to grep/rg (`node_modules`, `.gotmp*`, ...), so shell-backed
// and Go-native scans stay aligned.
func DirNameMatchesExcludePattern(name string, patterns []string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if pattern == name {
			return true
		}
		if strings.ContainsAny(pattern, "*?[") {
			if matched, err := filepath.Match(pattern, name); err == nil && matched {
				return true
			}
		}
	}
	return false
}

// IsExcludedDirName reports whether a single directory basename should
// be excluded at any depth.
func IsExcludedDirName(name string) bool {
	return DirNameMatchesExcludePattern(name, ExcludeDirPatternsAnyLevel)
}

// IsExcludedRelativePath reports whether a repo-relative path should be
// skipped by repo search / scan layers. Root-only exclusions are only
// applied to the first segment; any-level exclusions apply to every
// directory segment.
func IsExcludedRelativePath(relPath string) bool {
	if IsWindowsReservedDevicePath(relPath) {
		return true
	}
	normalized := strings.TrimSpace(strings.ReplaceAll(relPath, `\`, `/`))
	if normalized == "" {
		return false
	}
	if configuredSearchRuntimeArtifactRootMatches(normalized) {
		return true
	}
	parts := strings.Split(normalized, "/")
	if len(parts) > 0 && ExcludeDirsRootOnlySet[parts[0]] {
		return true
	}
	for _, part := range parts {
		if IsExcludedDirName(part) {
			return true
		}
	}
	return false
}

// SearchDirFilter is the path-aware exclusion policy shared by
// repo-root walkers and shell-backed search tools.
//
// The key distinction from the old flat ExcludeDirs union is that
// root-only names keep their root-only semantics, and an explicitly
// targeted path inside an otherwise excluded subtree is allowed back
// in. This lets broad discovery skip structural noise while still
// honoring deliberate user/tool targets like `grep(path="dist")`.
type SearchDirFilter struct {
	targetDirRel           string
	anyLevelPatterns       []string
	rootOnlySet            map[string]bool
	includeAuxiliarySource bool
	runtimeArtifactRoots   map[string]bool
}

// SearchDirFilterOptions tunes the central search exclusion policy without
// creating a second source-class taxonomy. IncludeAuxiliarySource is used only
// by typed source-enumeration lanes that already have structured scope
// authority; it reopens repo-owned auxiliary source trees while keeping
// dependency/cache/runtime artifacts excluded.
type SearchDirFilterOptions struct {
	IncludeAuxiliarySource bool
	RuntimeArtifactRoots   []string
}

// NewSearchDirFilter builds a reusable exclusion policy for searches
// rooted at targetPath inside repoRoot.
//
// repoRoot may be empty (unit tests / ad-hoc temp dirs); in that case
// the policy falls back to any-level exclusions only. targetPath may
// name either a directory or a file. When it points inside an excluded
// subtree, that exact target subtree is exempted so explicit requests
// are not silently filtered away.
func NewSearchDirFilter(repoRoot, targetPath string) SearchDirFilter {
	return NewSearchDirFilterWithOptions(repoRoot, targetPath, SearchDirFilterOptions{})
}

// NewSearchDirFilterWithOptions builds a reusable exclusion policy for searches
// rooted at targetPath inside repoRoot, with optional typed source-scope
// inclusion.
func NewSearchDirFilterWithOptions(repoRoot, targetPath string, opts SearchDirFilterOptions) SearchDirFilter {
	filter := SearchDirFilter{
		anyLevelPatterns:       append([]string(nil), ExcludeDirPatternsAnyLevel...),
		rootOnlySet:            make(map[string]bool, len(ExcludeDirsRootOnly)),
		includeAuxiliarySource: opts.IncludeAuxiliarySource,
		runtimeArtifactRoots:   normalizeSearchRuntimeArtifactRoots(append(SearchRuntimeArtifactRoots(), opts.RuntimeArtifactRoots...)),
	}
	for _, name := range ExcludeDirsRootOnly {
		filter.rootOnlySet[name] = true
	}
	if filter.includeAuxiliarySource {
		filter.removeAuxiliarySourceExcludes()
	}

	targetDirRel := repoRelativeSearchDir(repoRoot, targetPath)
	if targetDirRel == "" {
		return filter
	}
	filter.targetDirRel = targetDirRel
	for root := range filter.runtimeArtifactRoots {
		if targetDirRel == root || strings.HasPrefix(targetDirRel, root+"/") {
			delete(filter.runtimeArtifactRoots, root)
		}
	}
	segments := strings.Split(targetDirRel, "/")
	if len(segments) > 0 {
		delete(filter.rootOnlySet, segments[0])
	}
	filtered := make([]string, 0, len(filter.anyLevelPatterns))
	for _, pattern := range filter.anyLevelPatterns {
		exempt := false
		for _, seg := range segments {
			if DirNameMatchesExcludePattern(seg, []string{pattern}) {
				exempt = true
				break
			}
		}
		if !exempt {
			filtered = append(filtered, pattern)
		}
	}
	filter.anyLevelPatterns = filtered
	return filter
}

func (f *SearchDirFilter) removeAuxiliarySourceExcludes() {
	filtered := make([]string, 0, len(f.anyLevelPatterns))
	for _, pattern := range f.anyLevelPatterns {
		switch strings.ToLower(strings.TrimSpace(pattern)) {
		case "thirdparty", "third_party", "third-party", "vendor":
			continue
		default:
			filtered = append(filtered, pattern)
		}
	}
	f.anyLevelPatterns = filtered
}

// AnyLevelPatterns returns the any-depth exclude patterns after
// applying explicit-target exemptions.
func (f SearchDirFilter) AnyLevelPatterns() []string {
	return append([]string(nil), f.anyLevelPatterns...)
}

// RipgrepGlobs renders the policy as ripgrep --glob exclusions.
// any-level patterns remain basename-style (`!node_modules/`);
// root-only exclusions are anchored to the repo root (`!/logs/**`).
func (f SearchDirFilter) RipgrepGlobs() []string {
	out := make([]string, 0, len(f.anyLevelPatterns)+len(f.rootOnlySet))
	for _, pattern := range f.anyLevelPatterns {
		out = append(out, "!"+pattern+"/")
	}
	for _, name := range ExcludeDirsRootOnly {
		if f.rootOnlySet[name] {
			out = append(out, "!/"+name+"/**")
		}
	}
	for rel := range f.runtimeArtifactRoots {
		out = append(out, "!/"+rel+"/**")
	}
	return out
}

// ExcludesRepoRelativePath reports whether relPath should be skipped
// under this filter's policy. relPath must be repo-relative.
func (f SearchDirFilter) ExcludesRepoRelativePath(relPath string) bool {
	if IsWindowsReservedDevicePath(relPath) {
		return true
	}
	normalized := strings.TrimSpace(strings.ReplaceAll(relPath, `\`, `/`))
	if normalized == "" || normalized == "." {
		return false
	}
	if f.targetDirRel != "" && (normalized == f.targetDirRel || strings.HasPrefix(normalized, f.targetDirRel+"/")) {
		return false
	}
	if searchRuntimeArtifactRootMatches(normalized, searchRuntimeArtifactRootMapKeys(f.runtimeArtifactRoots)) {
		return true
	}
	if f.includeAuxiliarySource {
		if searchAuxiliarySourceHardExcludedRel(normalized) {
			return true
		}
		if searchAuxiliarySourceRelAllowed(normalized) {
			return false
		}
		if searchAuxiliarySourceContainerRelAllowed(normalized) {
			return false
		}
	}
	parts := strings.Split(normalized, "/")
	if len(parts) > 0 && f.rootOnlySet[parts[0]] {
		return true
	}
	for _, part := range parts {
		if DirNameMatchesExcludePattern(part, f.anyLevelPatterns) {
			return true
		}
	}
	return false
}

func searchAuxiliarySourceHardExcludedRel(rel string) bool {
	rel = strings.Trim(strings.ToLower(strings.ReplaceAll(rel, `\`, `/`)), "/")
	if rel == "" {
		return false
	}
	if rel == ".codrax" || strings.HasPrefix(rel, ".codrax/") {
		return true
	}
	return false
}

func searchAuxiliarySourceRelAllowed(rel string) bool {
	rel = strings.Trim(strings.ToLower(strings.ReplaceAll(rel, `\`, `/`)), "/")
	if rel == "" {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		switch part {
		case "thirdparty", "third_party", "third-party", "vendor",
			"test", "tests", "__tests__", "testdata", "fixtures", "examples":
			return true
		}
	}
	return false
}

func searchAuxiliarySourceContainerRelAllowed(rel string) bool {
	rel = strings.Trim(strings.ToLower(strings.ReplaceAll(rel, `\`, `/`)), "/")
	switch rel {
	case "eval":
		return true
	default:
		return false
	}
}

func normalizeSearchRuntimeArtifactRoots(in []string) map[string]bool {
	out := map[string]bool{}
	for _, rel := range normalizeSearchRuntimeArtifactRootSlice(in) {
		out[rel] = true
	}
	return out
}

func normalizeSearchRuntimeArtifactRootSlice(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		rel := strings.Trim(strings.ToLower(strings.ReplaceAll(raw, `\`, `/`)), "/")
		if rel == "" || rel == "." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
			continue
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		out = append(out, rel)
	}
	return out
}

func searchRuntimeArtifactRootMatches(rel string, roots []string) bool {
	rel = strings.Trim(strings.ToLower(strings.ReplaceAll(rel, `\`, `/`)), "/")
	for _, root := range roots {
		root = strings.Trim(root, "/")
		if root == "" {
			continue
		}
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}

func configuredSearchRuntimeArtifactRootMatches(rel string) bool {
	searchRuntimeArtifactRootsMu.RLock()
	defer searchRuntimeArtifactRootsMu.RUnlock()
	return searchRuntimeArtifactRootMatches(rel, searchRuntimeArtifactRoots)
}

func searchRuntimeArtifactRootMapKeys(in map[string]bool) []string {
	out := make([]string, 0, len(in))
	for root := range in {
		out = append(out, root)
	}
	return out
}

func repoRelativePathWithinRoot(repoRoot, targetPath string) (string, bool) {
	repoRoot = strings.TrimSpace(repoRoot)
	targetPath = strings.TrimSpace(targetPath)
	if repoRoot == "" || targetPath == "" {
		return "", false
	}
	repoRoot = filepath.Clean(repoRoot)
	resolved := normalizeToolAbsolutePath(targetPath)
	switch {
	case resolved == "":
		resolved = targetPath
	case !toolPathIsAbs(resolved):
		resolved = filepath.Join(repoRoot, resolved)
	}
	if !toolPathIsAbs(resolved) {
		resolved = filepath.Join(repoRoot, resolved)
	}
	resolved = filepath.Clean(resolved)
	rel, err := filepath.Rel(repoRoot, resolved)
	if err != nil {
		return "", false
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	if rel == "." {
		return "", true
	}
	return strings.ReplaceAll(filepath.Clean(rel), `\`, `/`), true
}

func repoRelativeSearchDir(repoRoot, targetPath string) string {
	rel, ok := repoRelativePathWithinRoot(repoRoot, targetPath)
	if !ok || rel == "" {
		return ""
	}
	resolved := normalizeToolAbsolutePath(strings.TrimSpace(targetPath))
	switch {
	case resolved == "":
		resolved = targetPath
	case !toolPathIsAbs(resolved):
		resolved = filepath.Join(filepath.Clean(strings.TrimSpace(repoRoot)), resolved)
	}
	if !toolPathIsAbs(resolved) {
		resolved = filepath.Join(filepath.Clean(strings.TrimSpace(repoRoot)), resolved)
	}
	if info, err := os.Stat(resolved); err == nil && !info.IsDir() {
		rel = filepath.Dir(rel)
	} else if err != nil {
		rel = filepath.Dir(rel)
	}
	if rel == "." {
		return ""
	}
	return strings.ReplaceAll(filepath.Clean(rel), `\`, `/`)
}

// windowsReservedDeviceNames are DOS device basenames that behave like
// pseudo-files on Windows (`nul`, `con`, `prn`, `aux`, `com1`...).
// Repositories can contain these names when authored on other
// platforms, but ripgrep / grep / WalkDir on Windows will treat them
// as device handles rather than ordinary source files, causing noisy
// search failures before the pipeline even reaches grounded reads.
var windowsReservedDeviceNames = []string{
	"con", "prn", "aux", "nul",
	"com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
	"lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9",
}

var windowsReservedDeviceSet = func() map[string]bool {
	m := make(map[string]bool, len(windowsReservedDeviceNames))
	for _, name := range windowsReservedDeviceNames {
		m[name] = true
	}
	return m
}()

// IsWindowsReservedDevicePath reports whether name resolves to a
// Windows reserved device basename such as `nul` or `con`.
//
// The check is intentionally OS-gated: on Linux/macOS a file literally
// named `nul` is valid project content and must remain searchable. On
// Windows, however, these names break external search tools and native
// walkers alike, so codrax treats them as search/list noise files.
func IsWindowsReservedDevicePath(name string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	base := strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return false
	}
	if idx := strings.IndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	return windowsReservedDeviceSet[base]
}

// ReservedDeviceRipgrepGlobs returns ripgrep glob patterns that skip
// Windows reserved device basenames at repo root and nested paths.
func ReservedDeviceRipgrepGlobs() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	out := make([]string, 0, len(windowsReservedDeviceNames)*4)
	for _, name := range windowsReservedDeviceNames {
		out = append(out,
			name,
			name+".*",
			"**/"+name,
			"**/"+name+".*",
		)
	}
	return out
}

// ReservedDeviceGrepExcludes returns GNU grep basename exclude globs
// for Windows reserved device files.
func ReservedDeviceGrepExcludes() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	out := make([]string, 0, len(windowsReservedDeviceNames)*2)
	for _, name := range windowsReservedDeviceNames {
		out = append(out, name, name+".*")
	}
	return out
}
