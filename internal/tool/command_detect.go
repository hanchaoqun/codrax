package tool

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	shellOnce sync.Once
	shellPath string
	shellArgs []string
)

// NewShellCommandContext creates a command that executes through the
// platform-appropriate shell. Unix keeps the historical sh -c behavior;
// Windows prefers a runnable POSIX shell when Git provides one and falls
// back to cmd /C only when no such shell is available.
func NewShellCommandContext(ctx context.Context, command string) *exec.Cmd {
	path, prefix := shellSpec()
	args := append(append([]string(nil), prefix...), command)
	return exec.CommandContext(ctx, path, args...)
}

func shellSpec() (string, []string) {
	shellOnce.Do(func() {
		if runtime.GOOS != "windows" {
			shellPath = "sh"
			shellArgs = []string{"-c"}
			return
		}

		if path := firstRunnablePath("sh", []string{"-c", "exit 0"}, windowsExtraCommandCandidates("sh")...); path != "" {
			shellPath = path
			shellArgs = []string{"-c"}
			return
		}
		if path := firstRunnablePath("bash", []string{"-lc", "exit 0"}, windowsExtraCommandCandidates("bash")...); path != "" {
			shellPath = path
			shellArgs = []string{"-lc"}
			return
		}

		shellPath = "cmd"
		shellArgs = []string{"/C"}
	})
	return shellPath, append([]string(nil), shellArgs...)
}

func firstRunnablePath(name string, probeArgs []string, extras ...string) string {
	candidates := append(executableCandidates(name), extras...)
	for _, candidate := range dedupePaths(candidates) {
		if isRunnable(candidate, probeArgs...) {
			return candidate
		}
	}
	return ""
}

func executableCandidates(name string) []string {
	if name == "" {
		return nil
	}
	if filepath.IsAbs(name) || strings.ContainsRune(name, filepath.Separator) || strings.Contains(name, "/") {
		return []string{name}
	}

	var names []string
	names = append(names, name)
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		for _, ext := range windowsPathExts() {
			names = append(names, name+ext)
		}
	}

	var candidates []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		for _, candidate := range names {
			candidates = append(candidates, filepath.Join(dir, candidate))
		}
	}
	return candidates
}

func windowsExtraCommandCandidates(name string) []string {
	if runtime.GOOS != "windows" {
		return nil
	}

	var extras []string
	extras = append(extras, gitDerivedCandidates(name)...)
	extras = append(extras, wingetCandidates(name)...)

	if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
		extras = append(extras,
			filepath.Join(programFiles, "Git", "bin", name+".exe"),
			filepath.Join(programFiles, "Git", "usr", "bin", name+".exe"),
		)
	}
	if programFilesX86 := os.Getenv("ProgramFiles(x86)"); programFilesX86 != "" {
		extras = append(extras,
			filepath.Join(programFilesX86, "Git", "bin", name+".exe"),
			filepath.Join(programFilesX86, "Git", "usr", "bin", name+".exe"),
		)
	}
	return extras
}

func gitDerivedCandidates(name string) []string {
	gitPath, err := exec.LookPath("git")
	if err != nil || gitPath == "" {
		return nil
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(gitPath), ".."))
	return []string{
		filepath.Join(root, "bin", name+".exe"),
		filepath.Join(root, "usr", "bin", name+".exe"),
		filepath.Join(root, "mingw64", "bin", name+".exe"),
	}
}

func wingetCandidates(name string) []string {
	base := filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet")
	if base == "" {
		return nil
	}

	patterns := []string{
		filepath.Join(base, "Links", name+".exe"),
		filepath.Join(base, "Packages", "*", name+".exe"),
		filepath.Join(base, "Packages", "*", "*", name+".exe"),
	}

	var matches []string
	for _, pattern := range patterns {
		found, _ := filepath.Glob(pattern)
		matches = append(matches, found...)
	}
	return matches
}

func windowsPathExts() []string {
	raw := os.Getenv("PATHEXT")
	if raw == "" {
		raw = ".COM;.EXE;.BAT;.CMD"
	}
	parts := strings.Split(raw, ";")
	exts := make([]string, 0, len(parts))
	for _, ext := range parts {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		exts = append(exts, strings.ToLower(ext))
	}
	return exts
}

func dedupePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(path)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, path)
	}
	return out
}

func isRunnable(path string, probeArgs ...string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, probeArgs...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil && ctx.Err() == nil
}
