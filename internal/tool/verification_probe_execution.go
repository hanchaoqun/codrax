package tool

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Roles are registered by the code which created these exact artifacts. No
// command text, random-name pattern, or model-provided alias is interpreted.
type verificationProbeInvocationRoles struct {
	Paths                 map[string]string
	PathListArgs          map[int]bool // indexes in exec.Cmd.Args, including argv[0]
	GeneratedSourceSHA256 string       // exact bytes created outside the planner probe
}

type verificationProbeInvocationPath struct {
	Literal string `json:"literal,omitempty"`
	Role    string `json:"role,omitempty"`
}

type verificationProbeInvocationArg struct {
	Value verificationProbeInvocationPath   `json:"value"`
	Paths []verificationProbeInvocationPath `json:"paths,omitempty"`
}

func verificationProbeExecutionReceipt(ctx *types.BusContext, probe types.VerificationProbe, cmd *exec.Cmd, timeout time.Duration, start time.Time, runErr error, roles verificationProbeInvocationRoles) *types.VerificationProbeExecutionReceipt {
	// An unknown start/wait failure is not a terminal process receipt. Check
	// the synchronized Wait result before touching ProcessState: a supervisor
	// kill-wait timeout may still have a live Wait goroutine.
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return nil
	}
	if ctx == nil || cmd == nil || cmd.ProcessState == nil || start.IsZero() {
		return nil
	}
	repoRoot, err := filepath.Abs(ctx.RepoRoot)
	if err != nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return nil
	}
	domain := repoRoot
	if ctx.MainRepoRoot != "" {
		domain, err = filepath.Abs(ctx.MainRepoRoot)
		if err != nil {
			return nil
		}
	}
	pathIdentity := func(path string) verificationProbeInvocationPath {
		if role, ok := roles.Paths[path]; ok {
			return verificationProbeInvocationPath{Role: role}
		}
		if filepath.IsAbs(path) {
			if rel, err := filepath.Rel(repoRoot, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return verificationProbeInvocationPath{Role: "repository/" + filepath.ToSlash(rel)}
			}
		}
		return verificationProbeInvocationPath{Literal: path}
	}
	args := make([]verificationProbeInvocationArg, len(cmd.Args))
	for i, arg := range cmd.Args {
		if roles.PathListArgs[i] {
			for _, path := range strings.Split(arg, string(os.PathListSeparator)) {
				args[i].Paths = append(args[i].Paths, pathIdentity(path))
			}
		} else {
			args[i].Value = pathIdentity(arg)
		}
	}
	invocation := struct {
		RepositoryRoot        string
		Executable            string
		Args                  []verificationProbeInvocationArg
		WorkingDir            verificationProbeInvocationPath
		TimeoutNS             int64
		GeneratedSourceSHA256 string
	}{domain, cmd.Path, args, pathIdentity(cmd.Dir), int64(timeout), roles.GeneratedSourceSHA256}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil
	}
	return &types.VerificationProbeExecutionReceipt{
		Version: types.VerificationProbeExecutionReceiptVersion, DefinitionSHA256: verificationProbeExecutionDigest(probe), InvocationSHA256: verificationProbeExecutionDigest(invocation),
		ExecutionID: hex.EncodeToString(nonce[:]), StartedAt: start, FinishedAt: time.Now(),
		RepositoryRoot: domain, Executable: cmd.Path, Args: append([]string(nil), cmd.Args...), WorkingDir: cmd.Dir,
	}
}

func verificationProbeExecutionDigest(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
