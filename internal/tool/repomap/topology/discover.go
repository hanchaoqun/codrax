package topology

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// Options controls the BFS walk in Discover. Zero values fall back to
// defaults documented on each field.
type Options struct {
	// Depth caps the BFS depth from parentAbs. Zero → 4.
	Depth int

	// MinFiles drops sub-repos whose Tier-1 file count is below
	// this threshold. Zero → 1 (only purely-empty roots dropped).
	MinFiles int

	// RuntimeAnchor lets Discover skip codrax's own per-session
	// worktree directories under <runtime-anchor>/worktrees/.
	// Empty → no skip.
	RuntimeAnchor string
}

// Discover walks parentAbs and returns a populated *RepoTopology.
//
// Algorithm (design §3.3.1):
//  1. parentAbs/.git present → degenerate single-repo path, return one
//     SubRepo whose RootRel = ".". No BFS occurs.
//  2. else BFS down to Options.Depth, pruning at IsExcludedDirName and
//     at codrax-self worktree dirs. Each directory whose own .git
//     entry exists becomes a SubRepo (and is NOT descended further).
//  3. For each SubRepo: probe Tier-1 PrimaryLangs + FileCount via
//     `git ls-files`, hash the manifest fingerprint.
//  4. Filter by MinFiles, sort by RootRel.
//
// parentAbs MUST be the result of filepath.Abs + filepath.EvalSymlinks
// (cmd/root.go does this for flagRepo). Returns an error only on a
// truly empty parentAbs; an empty BFS result is a legitimate outcome
// (returned with Repos == nil, callers fail-loud with their own
// "no git repository found" message).
func Discover(parentAbs string, opts Options) (*RepoTopology, error) {
	if parentAbs == "" {
		return nil, fmt.Errorf("topology.Discover: empty parentAbs")
	}
	parentSlug := index.CacheDirSlug(parentAbs)

	// Step 1 — single-repo degenerate fast path.
	if mode := gitMode(parentAbs); mode != "" {
		sr := buildSubRepo(parentAbs, parentAbs, ".", mode)
		return &RepoTopology{
			ParentRoot:   parentAbs,
			ParentSlug:   parentSlug,
			Repos:        []SubRepo{sr},
			DiscoveredAt: time.Now(),
		}, nil
	}

	// Step 2 — BFS.
	depth := opts.Depth
	if depth <= 0 {
		depth = 4
	}
	minFiles := opts.MinFiles
	if minFiles <= 0 {
		minFiles = 1
	}
	worktreePrefix := ""
	if opts.RuntimeAnchor != "" {
		worktreePrefix = filepath.Join(opts.RuntimeAnchor, "worktrees") + string(os.PathSeparator)
	}

	type walkItem struct {
		abs   string
		depth int
	}
	var subRepoRoots []string
	queue := []walkItem{{abs: parentAbs, depth: 0}}
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= depth {
			continue
		}
		if worktreePrefix != "" && strings.HasPrefix(item.abs+string(os.PathSeparator), worktreePrefix) {
			continue
		}
		entries, err := os.ReadDir(item.abs)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if tool.IsExcludedDirName(name) {
				continue
			}
			child := filepath.Join(item.abs, name)
			if mode := gitMode(child); mode != "" {
				subRepoRoots = append(subRepoRoots, child)
				continue // prune: don't descend into a sub-repo
			}
			queue = append(queue, walkItem{abs: child, depth: item.depth + 1})
		}
	}

	// Step 3 — build SubRepo entries.
	repos := make([]SubRepo, 0, len(subRepoRoots))
	for _, root := range subRepoRoots {
		rel, err := filepath.Rel(parentAbs, root)
		if err != nil {
			continue
		}
		sr := buildSubRepo(parentAbs, root, filepath.ToSlash(rel), gitMode(root))
		if sr.FileCount < minFiles {
			continue
		}
		repos = append(repos, sr)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].RootRel < repos[j].RootRel })

	return &RepoTopology{
		ParentRoot:   parentAbs,
		ParentSlug:   parentSlug,
		Repos:        repos,
		DiscoveredAt: time.Now(),
	}, nil
}

// gitMode returns "dir" if path/.git is a directory, "file" if it's a
// regular file (submodule / git-worktree pointer), and "" otherwise.
func gitMode(path string) string {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return "dir"
	}
	return "file"
}

func buildSubRepo(parentAbs, root, rootRel, mode string) SubRepo {
	sr := SubRepo{
		Slug:    index.CacheDirSlug(root),
		RootAbs: root,
		RootRel: rootRel,
		GitMode: mode,
	}
	sr.PrimaryLangs, sr.FileCount = probeTier1(root)
	sr.PrimaryLangsTier = 1
	sr.ManifestFingerprint = manifestFingerprint(root)
	return sr
}

// probeTier1 cheaply samples files via `git ls-files`. Returns
// (PrimaryLangs top-3 by extension count with special-file overrides,
// total file count). On error or empty repo it returns (nil, 0).
func probeTier1(root string) ([]string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd, cmdCancel := tool.NewGitCommand(ctx, "-C", root, "ls-files", "--cached", "--others", "--exclude-standard")
	defer cmdCancel()
	out, err := cmd.Output()
	if err != nil {
		return nil, 0
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, 0
	}
	lines := strings.Split(raw, "\n")

	langCounts := make(map[string]int, 8)
	for _, line := range lines {
		if line == "" {
			continue
		}
		if lang := types.DetectLanguage(line); lang != "" {
			langCounts[lang]++
		}
	}

	type kv struct {
		lang  string
		count int
	}
	sorted := make([]kv, 0, len(langCounts))
	for k, v := range langCounts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].lang < sorted[j].lang
	})

	top := make([]string, 0, 3)
	for _, kv := range sorted {
		if len(top) >= 3 {
			break
		}
		top = append(top, kv.lang)
	}

	// Special-file overrides — guaranteed presence beats extension counts.
	specials := []struct {
		file string
		lang string
	}{
		{"oh-package.json5", types.LangArkTS},
		{"cjpm.toml", types.LangCangjie},
		{"Cargo.toml", types.LangRust},
	}
	for _, s := range specials {
		if _, err := os.Stat(filepath.Join(root, s.file)); err == nil {
			top = promoteToFront(top, s.lang)
		}
	}
	// Gradle: prefer Kotlin if .kt > .java in this repo, else Java.
	if hasGradle(root) {
		prefer := types.LangJava
		if langCounts[types.LangKotlin] > langCounts[types.LangJava] {
			prefer = types.LangKotlin
		}
		top = promoteToFront(top, prefer)
	}

	return top, len(lines)
}

func hasGradle(root string) bool {
	for _, name := range []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

// promoteToFront moves lang to the head of xs (deduping); cap len at 3.
func promoteToFront(xs []string, lang string) []string {
	if lang == "" {
		return xs
	}
	out := []string{lang}
	for _, v := range xs {
		if v == lang {
			continue
		}
		out = append(out, v)
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// manifestFingerprint hashes size + mtime of recognised manifest files
// at root. Used to detect topology drift between runs.
func manifestFingerprint(root string) string {
	files := []string{
		"go.mod", "go.sum",
		"package.json", "pnpm-lock.yaml", "yarn.lock",
		"tsconfig.json",
		"Cargo.toml", "Cargo.lock",
		"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt",
		"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts",
		"oh-package.json5",
		"cjpm.toml",
		"Gemfile", "Gemfile.lock",
		"Package.swift",
		"Makefile", "CMakeLists.txt", "meson.build",
		"composer.json",
	}
	h := sha256.New()
	for _, name := range files {
		if info, err := os.Stat(filepath.Join(root, name)); err == nil {
			fmt.Fprintf(h, "%s|%d|%s\n", name, info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano))
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}
