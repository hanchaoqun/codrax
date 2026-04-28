package probe

import (
	"os/exec"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/types"
)

// pkgManagerSpec describes one package manager probe target.
// VersionArgs is the argv slice passed to the binary to extract
// its version; empty disables version probing for that manager.
type pkgManagerSpec struct {
	Name        string
	VersionArgs []string
}

// pkgManagerCatalog enumerates every package manager env_recommend
// knows how to recognise. Adding a new one only touches this
// catalog + the matching Recommender; the probe loop is generic.
//
// We probe broadly — apt / dnf / yum / brew etc. for OS-level,
// pip / pipx / poetry / hatch / uv / pixi for Python project,
// npm / pnpm / yarn / bun for Node, plus cargo / gem / bundler
// — to give the Recommender maximum signal. Probing is a single
// LookPath per manager; total cost ~50ms.
var pkgManagerCatalog = []pkgManagerSpec{
	// OS-level
	{Name: "apt", VersionArgs: []string{"--version"}},
	{Name: "apt-get", VersionArgs: []string{"--version"}},
	{Name: "dnf", VersionArgs: []string{"--version"}},
	{Name: "yum", VersionArgs: []string{"--version"}},
	{Name: "pacman", VersionArgs: []string{"--version"}},
	{Name: "zypper", VersionArgs: []string{"--version"}},
	{Name: "apk", VersionArgs: []string{"--version"}},
	{Name: "emerge", VersionArgs: nil},
	{Name: "xbps-install", VersionArgs: nil},
	{Name: "brew", VersionArgs: []string{"--version"}},
	{Name: "pkg", VersionArgs: nil}, // FreeBSD
	{Name: "winget", VersionArgs: []string{"--version"}},
	{Name: "scoop", VersionArgs: nil},
	{Name: "choco", VersionArgs: []string{"--version"}},
	// Python project-level
	{Name: "pip", VersionArgs: []string{"--version"}},
	{Name: "pip3", VersionArgs: []string{"--version"}},
	{Name: "pipx", VersionArgs: []string{"--version"}},
	{Name: "poetry", VersionArgs: []string{"--version"}},
	{Name: "hatch", VersionArgs: []string{"--version"}},
	{Name: "uv", VersionArgs: []string{"--version"}},
	{Name: "pixi", VersionArgs: []string{"--version"}},
	{Name: "conda", VersionArgs: []string{"--version"}},
	{Name: "mamba", VersionArgs: []string{"--version"}},
	// Node project-level
	{Name: "npm", VersionArgs: []string{"--version"}},
	{Name: "pnpm", VersionArgs: []string{"--version"}},
	{Name: "yarn", VersionArgs: []string{"--version"}},
	{Name: "bun", VersionArgs: []string{"--version"}},
	// Other ecosystems
	{Name: "cargo", VersionArgs: []string{"--version"}},
	{Name: "gem", VersionArgs: []string{"--version"}},
	{Name: "bundle", VersionArgs: []string{"--version"}},
	{Name: "go", VersionArgs: []string{"version"}},
	{Name: "swift", VersionArgs: []string{"--version"}},
	{Name: "rustup", VersionArgs: []string{"--version"}},
	{Name: "nvm", VersionArgs: nil},
	{Name: "pyenv", VersionArgs: []string{"--version"}},
	{Name: "rbenv", VersionArgs: []string{"--version"}},
	{Name: "asdf", VersionArgs: []string{"--version"}},
	{Name: "mise", VersionArgs: []string{"--version"}},
	// Build / test orchestration
	{Name: "make", VersionArgs: []string{"--version"}},
	{Name: "cmake", VersionArgs: []string{"--version"}},
	{Name: "meson", VersionArgs: []string{"--version"}},
	{Name: "ninja", VersionArgs: []string{"--version"}},
	{Name: "ctest", VersionArgs: []string{"--version"}},
	{Name: "bazel", VersionArgs: []string{"--version"}},
	{Name: "buck", VersionArgs: nil},
	{Name: "mvn", VersionArgs: []string{"--version"}},
	{Name: "gradle", VersionArgs: []string{"--version"}},
	// HarmonyOS / Cangjie ecosystems
	{Name: "hvigorw", VersionArgs: []string{"--version"}},
	{Name: "cjpm", VersionArgs: []string{"--version"}},
	// Git itself probes here so the rest of the system can ask
	// "is git on PATH?" via PkgManagers["git"]. (git's not really
	// a package manager but it earns inclusion as a top-priority
	// PATH tool.)
	{Name: "git", VersionArgs: []string{"--version"}},
}

// probePkgManagers walks the catalog in parallel (LookPath is
// cheap; per-spec ~5ms). Returns one entry per manager actually
// found. Versions are best-effort; failures yield empty Version.
func probePkgManagers() map[string]types.PkgManagerInfo {
	out := make(map[string]types.PkgManagerInfo, len(pkgManagerCatalog)/2)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, spec := range pkgManagerCatalog {
		wg.Add(1)
		go func(s pkgManagerSpec) {
			defer wg.Done()
			path, err := exec.LookPath(s.Name)
			if err != nil {
				return
			}
			info := types.PkgManagerInfo{Path: path}
			if len(s.VersionArgs) > 0 {
				if v, err := captureCmd(path, s.VersionArgs...); err == nil {
					info.Version = parseFirstVersionToken(v)
				}
			}
			mu.Lock()
			out[s.Name] = info
			mu.Unlock()
		}(spec)
	}
	wg.Wait()
	return out
}

// parseFirstVersionToken extracts the first dotted-numeric token
// from a `--version` style output line. Many tools emit
// "<name> X.Y.Z (build info)" so this is the canonical extractor.
// Empty when no numeric token found.
func parseFirstVersionToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Find the first run of [0-9.] characters of length >= 2 with
	// at least one '.'. Robust to "git version 2.43.0", "Python 3.12.0",
	// "npm 10.2.4", "cargo 1.74.0 (...)", etc.
	for i := 0; i < len(s); i++ {
		if isVersionStart(s[i]) {
			j := i
			for j < len(s) && isVersionRune(s[j]) {
				j++
			}
			tok := s[i:j]
			if strings.Contains(tok, ".") {
				return tok
			}
			i = j
		}
	}
	return ""
}

func isVersionStart(b byte) bool { return b >= '0' && b <= '9' }

func isVersionRune(b byte) bool {
	return (b >= '0' && b <= '9') || b == '.'
}
