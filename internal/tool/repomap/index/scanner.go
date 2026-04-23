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
	"go.mod":         "build_config",
	"go.sum":         "build_config",
	"package.json":   "build_config",
	"tsconfig.json":  "build_config",
	"jsconfig.json":  "build_config",
	"pom.xml":        "build_config",
	"build.gradle":   "build_config",
	"Cargo.toml":     "build_config",
	"Cargo.lock":     "build_config",
	"CMakeLists.txt": "build_config",
	"Makefile":       "build_config",
	"Dockerfile":     "dockerfile",
	"docker-compose.yml": "dockerfile",
	"docker-compose.yaml": "dockerfile",
	".github":        "ci",
	".gitlab-ci.yml": "ci",
	"Jenkinsfile":    "ci",
}

// ScanFiles discovers source files in a repository.
// It uses `git ls-files` when available, falling back to filepath.Walk.
func ScanFiles(repoRoot string) ([]FileEntry, error) {
	entries, err := scanGit(repoRoot)
	if err != nil {
		return scanWalk(repoRoot)
	}
	return entries, nil
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
