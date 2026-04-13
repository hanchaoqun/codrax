package repomap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileEntry is a file discovered during scanning.
type FileEntry struct {
	RelPath  string
	AbsPath  string
	Language string
	Size     int64
}

// excludedDirs are always skipped even if not in .gitignore.
var excludedDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	".git":         true,
	".venv":        true,
	"__pycache__":  true,
	".tox":         true,
	".mypy_cache":  true,
	"target":       true, // Rust/Maven
	".gradle":      true,
	".idea":        true,
	".vscode":      true,
}

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
	cmd := exec.Command("git", "-C", repoRoot, "ls-files", "--cached", "--others", "--exclude-standard")
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
		lang := DetectLanguage(line)
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
		lang := DetectLanguage(rel)
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
