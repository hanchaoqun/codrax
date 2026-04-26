package index

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// FileEntry is a file discovered during scanning.
type FileEntry struct {
	RelPath  string
	AbsPath  string
	Language string
	Size     int64
}

// excludedDirs delegates to the single authoritative any-level list
// in tool.ExcludeDirsAnyLevelSet (see internal/tool/search.go).
// Historically this was a separate map that drifted from GrepTool
// and keyword search; Phase 3 unifies the sources of truth so one
// list governs every scan and search.
//
// The any-level set excludes directories regardless of depth
// (node_modules, target, .git, ...) while ExcludeDirsRootOnlySet
// ("logs", "memory", "eval") is applied only at the top of a
// RelPath by isExcludedPath below, so a legitimate nested package
// such as `internal/memory/` is not accidentally dropped.
var excludedDirs = tool.ExcludeDirsAnyLevelSet

// specialFiles maps filenames to their special type.
var specialFiles = map[string]string{
	"go.mod":              "build_config",
	"go.sum":              "build_config",
	"package.json":        "build_config",
	"tsconfig.json":       "build_config",
	"jsconfig.json":       "build_config",
	"pom.xml":             "build_config",
	"build.gradle":        "build_config",
	"build.gradle.kts":    "build_config",
	"settings.gradle":     "build_config",
	"settings.gradle.kts": "build_config",
	"Cargo.toml":          "build_config",
	"Cargo.lock":          "build_config",
	"CMakeLists.txt":      "build_config",
	"Makefile":            "build_config",
	"Dockerfile":          "dockerfile",
	"docker-compose.yml":  "dockerfile",
	"docker-compose.yaml": "dockerfile",
	".github":             "ci",
	".gitlab-ci.yml":      "ci",
	"Jenkinsfile":         "ci",
	// HarmonyOS ArkTS / Cangjie build manifests. Flagging them as
	// build_config lets the analyzer hints and repomap rank treat
	// them as first-class project descriptors (same as package.json
	// / pom.xml for Java/Node), and the verifier's detectRunner
	// keys off the same filenames for runner dispatch.
	"oh-package.json5":    "build_config",
	"build-profile.json5": "build_config",
	"hvigorfile.ts":       "build_config",
	"cjpm.toml":           "build_config",
	// Android build descriptor — local.properties holds SDK paths
	// and is typically git-ignored, so we skip it here to avoid
	// surfacing operator credentials as "special files".
	// AndroidManifest.xml is flagged so Android projects surface a
	// recognisable "app entry" descriptor even when Gradle files
	// live only in sub-modules.
	"AndroidManifest.xml": "build_config",
}

// ScanFiles discovers source files in a repository.
// It uses `git ls-files` when available, falling back to filepath.Walk.
//
// Post-processes the raw entries to honour HarmonyOS red lines:
//   - L-ArkTS-2: `.ts` files inside an ArkTS project (oh-package.json5
//     present in any ancestor dir within repoRoot) are promoted to
//     LangArkTS; pure TS projects are not affected.
//   - L-Cangjie-1: `.cjo` Cangjie compiled artefacts are denied
//     at discovery time; they never enter the pipeline.
func ScanFiles(repoRoot string) ([]FileEntry, error) {
	entries, err := scanGit(repoRoot)
	if err != nil {
		entries, err = scanWalk(repoRoot)
		if err != nil {
			return nil, err
		}
	}
	return applyHarmonyOSPostProcess(repoRoot, entries), nil
}

// applyHarmonyOSPostProcess walks `entries` and applies two rules:
//  1. Drop .cjo / .cangjie-cache / target/cj-* compiled artefacts
//     (red line L-Cangjie-1).
//  2. Promote .ts files to LangArkTS when the project is an ArkTS
//     project (red line L-ArkTS-2). The detection is per-file so
//     sub-modules with their own oh-package.json5 get independent
//     classification; a .ts file outside any ArkTS module stays
//     LangTypeScript.
//
// The result is a new slice (input is not mutated) so callers can
// snapshot the pre-processed list for diagnostics.
func applyHarmonyOSPostProcess(repoRoot string, entries []FileEntry) []FileEntry {
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		relSlash := filepath.ToSlash(e.RelPath)
		ext := strings.ToLower(filepath.Ext(e.RelPath))
		if ext == ".cjo" {
			continue // denied compiled artefact
		}
		// Cangjie package manager's build cache - skip wholesale.
		if relSlash == ".cangjie-cache" ||
			strings.HasPrefix(relSlash, ".cangjie-cache/") ||
			strings.Contains(relSlash, "/.cangjie-cache/") {
			continue
		}
		if e.Language == types.LangTypeScript && types.IsArkTSProject(repoRoot, e.RelPath) {
			e.Language = types.LangArkTS
		}
		out = append(out, e)
	}
	return out
}

func scanGit(repoRoot string) ([]FileEntry, error) {
	cmd, cancel := tool.NewGitCommand(nil, "-C", repoRoot, "ls-files", "--cached", "--others", "--exclude-standard")
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	entries := make([]FileEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if tool.IsWindowsReservedDevicePath(line) {
			continue
		}
		// skip files inside excluded dirs
		if isExcludedPath(line) {
			continue
		}
		abs := filepath.Join(repoRoot, line)
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			continue
		}
		lang := types.DetectLanguage(line)
		entries = append(entries, FileEntry{
			RelPath:  line,
			AbsPath:  abs,
			Language: lang,
			Size:     info.Size(),
		})
	}
	return entries, nil
}

func scanWalk(repoRoot string) ([]FileEntry, error) {
	var entries []FileEntry
	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if tool.IsWindowsReservedDevicePath(info.Name()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if excludedDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		if isExcludedPath(rel) {
			return nil
		}
		lang := types.DetectLanguage(rel)
		entries = append(entries, FileEntry{
			RelPath:  rel,
			AbsPath:  path,
			Language: lang,
			Size:     info.Size(),
		})
		return nil
	})
	return entries, err
}

func isExcludedPath(relPath string) bool {
	if tool.IsWindowsReservedDevicePath(relPath) {
		return true
	}
	parts := strings.Split(relPath, string(os.PathSeparator))
	if len(parts) > 0 && tool.ExcludeDirsRootOnlySet[parts[0]] {
		return true
	}
	for _, p := range parts {
		if excludedDirs[p] {
			return true
		}
	}
	return false
}

// IsSpecialFile checks whether a filename is a notable project file.
func IsSpecialFile(name string) (bool, string) {
	base := filepath.Base(name)
	if t, ok := specialFiles[base]; ok {
		return true, t
	}
	return false, ""
}
