package index

import (
	"bufio"
	"io"
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

// ScanFilesProgress reports accepted file-discovery progress. Total
// repository file count is not known until discovery finishes, so the
// callback reports monotonic counted totals rather than a denominator.
type ScanFilesProgress func(files, parseable int, current string)

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
	return ScanFilesWithProgress(repoRoot, nil)
}

// ScanFilesWithProgress discovers source files and reports bounded
// progress while the inventory is still being built. It preserves
// ScanFiles semantics exactly; progress is telemetry only.
func ScanFilesWithProgress(repoRoot string, progress ScanFilesProgress) ([]FileEntry, error) {
	entries, err := scanGit(repoRoot, progress)
	if err != nil {
		entries, err = scanWalk(repoRoot, progress)
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
	arkTSDirCache := make(map[string]bool)
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
		if e.Language == types.LangTypeScript {
			dir := filepath.ToSlash(filepath.Dir(e.RelPath))
			isArkTS, ok := arkTSDirCache[dir]
			if !ok {
				isArkTS = types.IsArkTSProject(repoRoot, e.RelPath)
				arkTSDirCache[dir] = isArkTS
			}
			if isArkTS {
				e.Language = types.LangArkTS
			}
		}
		out = append(out, e)
	}
	return out
}

func scanGit(repoRoot string, progress ScanFilesProgress) ([]FileEntry, error) {
	cmd, cancel := tool.NewGitCommand(nil, "-C", repoRoot, "ls-files", "--cached", "--others", "--exclude-standard")
	defer cancel()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	lines, scanErr := scanGitLines(stdout)
	waitErr := cmd.Wait()
	if scanErr != nil {
		return nil, scanErr
	}
	if waitErr != nil {
		return nil, waitErr
	}
	return entriesFromGitLines(repoRoot, lines, progress), nil
}

func scanWalk(repoRoot string, progress ScanFilesProgress) ([]FileEntry, error) {
	var entries []FileEntry
	parseable := 0
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
			if tool.IsExcludedDirName(info.Name()) {
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
		if lang != "" {
			parseable++
		}
		entries = append(entries, FileEntry{
			RelPath:  rel,
			AbsPath:  path,
			Language: lang,
			Size:     info.Size(),
		})
		reportScanFilesProgress(progress, len(entries), parseable, rel)
		return nil
	})
	return entries, err
}

func scanGitLines(stdout io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := make([]string, 0, 1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func entriesFromGitLines(repoRoot string, lines []string, progress ScanFilesProgress) []FileEntry {
	entries := make([]FileEntry, 0, len(lines))
	parseable := 0
	for _, line := range lines {
		if tool.IsWindowsReservedDevicePath(line) {
			continue
		}
		if isExcludedPath(line) {
			continue
		}
		abs := filepath.Join(repoRoot, line)
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			continue
		}
		lang := types.DetectLanguage(line)
		if lang != "" {
			parseable++
		}
		entries = append(entries, FileEntry{
			RelPath:  line,
			AbsPath:  abs,
			Language: lang,
			Size:     info.Size(),
		})
		reportScanFilesProgress(progress, len(entries), parseable, line)
	}
	return entries
}

func reportScanFilesProgress(progress ScanFilesProgress, files, parseable int, current string) {
	if progress != nil {
		progress(files, parseable, current)
	}
}

func isExcludedPath(relPath string) bool {
	return tool.IsExcludedRelativePath(relPath)
}

// IsSpecialFile checks whether a filename is a notable project file.
func IsSpecialFile(name string) (bool, string) {
	base := filepath.Base(name)
	if t, ok := specialFiles[base]; ok {
		return true, t
	}
	return false, ""
}
