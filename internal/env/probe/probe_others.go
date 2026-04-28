package probe

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// probeRusts captures rustc + cargo + version. We only support the
// "first found" toolchain — rustup-managed multi-toolchain setups
// expose `cargo`/`rustc` as proxy shims, the active one is the
// only one Recommender cares about for "is rust available".
func probeRusts(facts *types.EnvFacts) []types.RustToolchain {
	rustc, err := exec.LookPath("rustc")
	if err != nil {
		return nil
	}
	t := types.RustToolchain{RustcPath: rustc, Origin: rustOrigin(rustc)}
	if cargo, err := exec.LookPath("cargo"); err == nil {
		t.CargoPath = cargo
	}
	if v, err := captureCmd(rustc, "--version"); err == nil {
		t.Version = parseFirstVersionToken(v)
	}
	return []types.RustToolchain{t}
}

func rustOrigin(rustcPath string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if strings.HasPrefix(rustcPath, filepath.Join(home, ".rustup")) ||
			strings.HasPrefix(rustcPath, filepath.Join(home, ".cargo")) {
			return "rustup"
		}
	}
	return "system"
}

// probeJavas captures javac + (mvn || gradle || gradle wrapper in
// repo). The Recommender uses HasGradleWrapper to prefer
// `./gradlew` over global gradle.
func probeJavas(facts *types.EnvFacts, repoRoot string) []types.JavaToolchain {
	javac, err := exec.LookPath("javac")
	if err != nil {
		// java without javac means JRE-only, can't compile —
		// Recommender treats as toolchain_missing later.
		if javaPath, jerr := exec.LookPath("java"); jerr == nil {
			t := types.JavaToolchain{}
			if v, err := captureCmd(javaPath, "-version"); err == nil {
				t.JavaVersion = parseFirstVersionToken(v)
			}
			return []types.JavaToolchain{t}
		}
		return nil
	}
	t := types.JavaToolchain{JavacPath: javac}
	if v, err := captureCmd(javac, "-version"); err == nil {
		t.JavaVersion = parseFirstVersionToken(v)
	}
	if mvn, err := exec.LookPath("mvn"); err == nil {
		t.MavenPath = mvn
	}
	if gradle, err := exec.LookPath("gradle"); err == nil {
		t.GradlePath = gradle
	}
	if repoRoot != "" {
		// Gradle wrapper lives at repo root as `gradlew` (Unix) or
		// `gradlew.bat` (Windows). Either presence flips the flag.
		if isExecutable(filepath.Join(repoRoot, "gradlew")) ||
			fileExists(filepath.Join(repoRoot, "gradlew.bat")) {
			t.HasGradleWrapper = true
		}
	}
	return []types.JavaToolchain{t}
}

// probeRubies captures ruby + bundler. Bundler is independent of
// the ruby install (operator has to `gem install bundler`); the
// probe records it separately.
func probeRubies(facts *types.EnvFacts) []types.RubyToolchain {
	ruby, err := exec.LookPath("ruby")
	if err != nil {
		return nil
	}
	t := types.RubyToolchain{RubyPath: ruby}
	if v, err := captureCmd(ruby, "--version"); err == nil {
		t.Version = parseFirstVersionToken(v)
	}
	if bundle, err := exec.LookPath("bundle"); err == nil {
		t.BundlerPath = bundle
	}
	return []types.RubyToolchain{t}
}

// probeNetwork is a TCP connect to 1.1.1.1:443 with a short
// timeout. We deliberately don't HTTP-GET — that requires DNS, TLS
// handshake, and may itself be intercepted by corporate proxies.
// A raw TCP connect to a well-known anycast IP is the fastest
// signal of "we have egress".
func probeNetwork(ctx context.Context) types.NetworkStatus {
	status := types.NetworkStatus{ProbedAt: time.Now()}
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		if os.Getenv(k) != "" {
			status.ProxySet = true
			break
		}
	}
	d := &net.Dialer{Timeout: 300 * time.Millisecond}
	c, err := d.DialContext(ctx, "tcp", "1.1.1.1:443")
	if err != nil {
		return status
	}
	_ = c.Close()
	status.Reachable = true
	return status
}

// probeProjectFiles scans the repo root one level deep for known
// project manifests. Goes one level only — monorepos with
// per-workspace manifests are surfaced via the top-level pointer
// (e.g. root pnpm-workspace.yaml) and Recommender knows to
// recurse. Returns a map keyed by canonical filename.
func probeProjectFiles(repoRoot string) map[string]string {
	if repoRoot == "" {
		return nil
	}
	out := map[string]string{}
	candidates := []string{
		"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt",
		"Pipfile", "Pipfile.lock", "pixi.toml", "noxfile.py", "tox.ini",
		"pytest.ini",
		"package.json", "package-lock.json", "pnpm-lock.yaml",
		"yarn.lock", "bun.lockb", "pnpm-workspace.yaml",
		"Cargo.toml", "Cargo.lock",
		"go.mod", "go.sum", "go.work",
		"Gemfile", "Gemfile.lock",
		"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle",
		"settings.gradle.kts",
		"Package.swift", "Package.resolved",
		"CMakeLists.txt", "meson.build",
		"Makefile", "GNUmakefile",
		"oh-package.json5", "build-profile.json5", "hvigorfile.ts",
		"cjpm.toml",
		"Dockerfile", "docker-compose.yaml", "docker-compose.yml",
		".python-version", ".node-version", ".nvmrc", ".tool-versions",
		".gitignore", "README.md",
	}
	for _, name := range candidates {
		full := filepath.Join(repoRoot, name)
		if fileExists(full) {
			out[name] = full
		}
	}
	return out
}

// probeGitRepoState classifies the repo state in three buckets the
// Recommender keys on. "git_missing" when git binary is absent;
// "ready" when both .git/ and a HEAD commit exist;
// "not_initialized" when no .git/; "no_commits" when .git/ exists
// but HEAD points at an unborn ref.
func probeGitRepoState(repoRoot string) string {
	if _, err := exec.LookPath("git"); err != nil {
		return "git_missing"
	}
	if repoRoot == "" {
		return "unknown"
	}
	if !fileExists(filepath.Join(repoRoot, ".git")) {
		// .git can be a directory (normal) or a file (worktree
		// pointer). fileExists returns true for either.
		return "not_initialized"
	}
	// Check HEAD points at a real commit.
	if err := runQuick("git", "-C", repoRoot, "rev-parse", "--verify", "HEAD"); err != nil {
		return "no_commits"
	}
	return "ready"
}
