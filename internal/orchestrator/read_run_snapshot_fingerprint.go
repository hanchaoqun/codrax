package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const readRunRepoFingerprintTimeout = 2 * time.Second

const (
	readRunEnvironmentFingerprintTimeout       = 750 * time.Millisecond
	readRunEnvironmentToolFileHashMaxBytes     = 64 * 1024 * 1024
	readRunFingerprintReasonVersionUnavailable = "version_unavailable"
	readRunFingerprintReasonFileTooLarge       = "file_too_large"
)

func readRunCurrentRepoFingerprint(repoRoot string) types.ReadRunRepoFingerprint {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return types.ReadRunRepoFingerprint{
			Kind:       types.ReadRunRepoFingerprintKindGitHead,
			Available:  false,
			ReasonCode: types.ReadRunFingerprintReasonUnavailable,
		}
	}
	head, err := readRunGitOutput(repoRoot, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(head) == "" {
		return types.ReadRunRepoFingerprint{
			Kind:       types.ReadRunRepoFingerprintKindGitHead,
			Available:  false,
			ReasonCode: types.ReadRunFingerprintReasonNotGit,
		}
	}
	status, _ := readRunGitOutput(repoRoot, "status", "--short", "--untracked-files=all")
	return types.NormalizeReadRunRepoFingerprint(types.ReadRunRepoFingerprint{
		Kind:       types.ReadRunRepoFingerprintKindGitHead,
		Available:  true,
		Head:       strings.TrimSpace(head),
		StatusHash: readRunRepoStatusHash(status),
	})
}

func readRunGitOutput(repoRoot string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), readRunRepoFingerprintTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoRoot}, args...)...)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func readRunRepoStatusHash(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(status))
	return hex.EncodeToString(sum[:])
}

func (o *Orchestrator) readRunCurrentEnvironmentFingerprint() types.ReadRunEnvironmentFingerprint {
	version := ""
	buildTime := ""
	if o != nil {
		version = strings.TrimSpace(o.readRunCodraxVersion)
		buildTime = strings.TrimSpace(o.readRunCodraxBuild)
	}
	out := types.ReadRunEnvironmentFingerprint{
		Kind:            types.ReadRunEnvironmentKindCodrax,
		Available:       true,
		CodraxVersion:   version,
		CodraxBuildTime: buildTime,
		GoVersion:       runtime.Version(),
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
		Tools: []types.ReadRunToolFingerprint{
			readRunToolFingerprint("git"),
			readRunToolFingerprint("rg"),
			readRunToolFingerprint("grep"),
			readRunToolFingerprint("go"),
		},
		Configs: []types.ReadRunConfigFingerprint{
			types.ReadRunConfigFingerprintFromStringSlice(
				types.ReadRunConfigFingerprintSearchExcludeRoots,
				tool.SearchRuntimeArtifactRoots(),
			),
		},
	}
	return types.NormalizeReadRunEnvironmentFingerprint(out)
}

func readRunToolFingerprint(name string) types.ReadRunToolFingerprint {
	name = strings.TrimSpace(name)
	if name == "" {
		return types.ReadRunToolFingerprint{}
	}
	executable, err := exec.LookPath(name)
	if err != nil || strings.TrimSpace(executable) == "" {
		return types.NormalizeReadRunToolFingerprint(types.ReadRunToolFingerprint{
			Name:       name,
			Available:  false,
			ReasonCode: types.ReadRunFingerprintReasonNotFound,
		})
	}
	executable = readRunCanonicalExecutablePath(executable)
	versionHash, versionReason := readRunToolVersionHash(name, executable)
	fileHash, fileReason := readRunToolFileHash(executable)
	reason := ""
	if versionHash == "" {
		reason = versionReason
	}
	if reason == "" && fileHash == "" {
		reason = fileReason
	}
	return types.NormalizeReadRunToolFingerprint(types.ReadRunToolFingerprint{
		Name:        name,
		Available:   true,
		Executable:  executable,
		VersionHash: versionHash,
		FileHash:    fileHash,
		ReasonCode:  reason,
	})
}

func readRunToolVersionHash(name, executable string) (string, string) {
	args := []string{"--version"}
	if name == "go" {
		args = []string{"version"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), readRunEnvironmentFingerprintTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", readRunFingerprintReasonVersionUnavailable
	}
	text := strings.TrimSpace(string(out))
	if text == "" || err != nil && text == "" {
		return "", readRunFingerprintReasonVersionUnavailable
	}
	sum := sha256.Sum256([]byte(name + "\x00" + text))
	return hex.EncodeToString(sum[:]), ""
}

func readRunToolFileHash(executable string) (string, string) {
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return "", types.ReadRunFingerprintReasonUnavailable
	}
	info, err := os.Stat(executable)
	if err != nil || info.IsDir() {
		return "", types.ReadRunFingerprintReasonUnavailable
	}
	if info.Size() > readRunEnvironmentToolFileHashMaxBytes {
		return "", readRunFingerprintReasonFileTooLarge
	}
	f, err := os.Open(executable)
	if err != nil {
		return "", types.ReadRunFingerprintReasonUnavailable
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, readRunEnvironmentToolFileHashMaxBytes+1)); err != nil {
		return "", types.ReadRunFingerprintReasonUnavailable
	}
	return hex.EncodeToString(h.Sum(nil)), ""
}

func readRunCanonicalExecutablePath(executable string) string {
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return ""
	}
	if abs, err := filepath.Abs(executable); err == nil {
		executable = abs
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	return filepath.Clean(executable)
}
