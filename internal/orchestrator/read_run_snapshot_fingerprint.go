package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

const readRunRepoFingerprintTimeout = 2 * time.Second

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
