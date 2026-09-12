package tool

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

const pythonTargetSourceLimit = 2 << 20

type pythonTargetManifestFile struct {
	Path            string `json:"path"`
	SourceSHA256    string `json:"source_sha256"`
	AddedLines      []int  `json:"added_lines"`
	MappingComplete bool   `json:"mapping_complete"`
}

type pythonTargetObservation struct {
	Nonce         string                     `json:"nonce"`
	OutputPath    string                     `json:"output_path"`
	ExecutionRoot string                     `json:"execution_root"`
	Targets       []pythonTargetManifestFile `json:"targets"`
	receipt       types.VerificationProbeTargetExecutionReceipt
	effectHash    string
	ctx           *types.BusContext
}

// preparePythonTargetObservation uses only the current applied new-side line
// table. A path hint or old/hunk context is not a fallback execution target.
func preparePythonTargetObservation(ctx *types.BusContext, probe types.VerificationProbe) *pythonTargetObservation {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return nil
	}
	root, err := filepath.Abs(ctx.RepoRoot)
	if err != nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return nil
	}
	out := &pythonTargetObservation{ExecutionRoot: root, ctx: ctx, receipt: types.VerificationProbeTargetExecutionReceipt{
		Version: 1, PlanID: plan.ID, ProbeID: probe.ID, ExecutionRoot: root, Status: "unknown", ReasonCode: "patch_effect_unavailable",
	}}
	effect := plan.PatchEffect
	if effect == nil || effect.PlanID != plan.ID || effect.RecordID == "" || effect.DiffFingerprint == "" || effect.HeadRef == "" {
		return out
	}
	out.receipt.PatchEffectID, out.receipt.DiffFingerprint, out.receipt.HeadRef = effect.RecordID, effect.DiffFingerprint, effect.HeadRef
	out.effectHash = verificationProbeExecutionDigest(effect)
	prepareCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	commit, err := pythonTargetAppliedCommit(prepareCtx, root, effect)
	if err != nil {
		out.receipt.ReasonCode = "applied_source_unavailable"
		return out
	}
	out.receipt.SourceCommitSHA = commit
	paths := append([]string(nil), plan.TargetPaths...)
	sort.Strings(paths)
	seen := map[string]bool{}
	for _, path := range paths {
		if filepath.Ext(path) != ".py" || seen[path] {
			continue
		}
		seen[path] = true
		if filepath.ToSlash(filepath.Clean(path)) != path || filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, "../") || len(out.Targets) >= 32 {
			out.receipt.ReasonCode = "target_manifest_unsupported"
			return out
		}
		data, err := readPythonTargetSource(root, path)
		if err != nil {
			out.receipt.ReasonCode = "target_source_unavailable"
			return out
		}
		blob, err := pythonTargetCommitSource(prepareCtx, root, commit, path)
		if err != nil || !bytes.Equal(blob, data) {
			out.receipt.ReasonCode = "applied_source_mismatch"
			return out
		}
		item := pythonTargetManifestFile{Path: path, SourceSHA256: pythonTargetSHA(data)}
		var selected *types.PatchEffectFile
		for i := range effect.Files {
			if effect.Files[i].Path == path {
				if selected != nil {
					out.receipt.ReasonCode = "target_effect_ambiguous"
					return out
				}
				selected = &effect.Files[i]
			}
		}
		if selected != nil {
			item.AddedLines, item.MappingComplete = pythonTargetAddedLines(*selected, data)
		}
		out.Targets = append(out.Targets, item)
	}
	if len(out.Targets) == 0 {
		return out
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return out
	}
	file, err := os.CreateTemp("", "codrax-python-target-*.json")
	if err != nil {
		return out
	}
	out.OutputPath, out.Nonce = file.Name(), hex.EncodeToString(nonce[:])
	_ = file.Close()
	return out
}

func pythonTargetAddedLines(file types.PatchEffectFile, data []byte) ([]int, bool) {
	if file.Binary || len(file.Hunks) == 0 || file.AddedLines <= 0 {
		return nil, false
	}
	lines := strings.Split(string(data), "\n")
	seen := map[int]bool{}
	var out []int
	for _, hunk := range file.Hunks {
		if hunk.RemovedLines > 0 && hunk.AddedLines == 0 {
			return nil, false // deleted owner's identity is not known on the new side
		}
		if len(hunk.AddedLineNumbers) != hunk.AddedLines || len(hunk.AddedLineTexts) != hunk.AddedLines {
			return nil, false
		}
		hunkLines := map[int]bool{}
		for _, line := range hunk.AddedLineNumbers {
			if line <= 0 || line > len(lines) || seen[line] {
				return nil, false
			}
			seen[line] = true
			hunkLines[line] = true
			out = append(out, line)
		}
		textSeen := map[int]bool{}
		for _, line := range hunk.AddedLineTexts {
			if !hunkLines[line.Line] || textSeen[line.Line] || lines[line.Line-1] != line.Text {
				return nil, false
			}
			textSeen[line.Line] = true
		}
	}
	if len(out) != file.AddedLines {
		return nil, false
	}
	sort.Ints(out)
	return out, true
}

func (o *pythonTargetObservation) environment() string {
	if o == nil || o.OutputPath == "" {
		return "CODRAX_TARGET_MANIFEST="
	}
	encoded, err := json.Marshal(o)
	if err != nil {
		return "CODRAX_TARGET_MANIFEST="
	}
	return "CODRAX_TARGET_MANIFEST=" + base64.StdEncoding.EncodeToString(encoded)
}

func (o *pythonTargetObservation) cleanup() {
	if o != nil && o.OutputPath != "" {
		_ = os.Remove(o.OutputPath)
	}
}

func (o *pythonTargetObservation) finish(execution *types.VerificationProbeExecutionReceipt) {
	if o == nil || execution == nil {
		return
	}
	receipt := o.receipt
	receipt.InvocationBindingSHA256 = types.VerificationProbeTargetInvocationSHA256(execution)
	execution.TargetExecution = &receipt
	unknown := func(reason string) {
		receipt.Status, receipt.ReasonCode = "unknown", reason
		receipt.ManifestSHA256 = types.VerificationProbeTargetManifestSHA256(&receipt)
	}
	if o.OutputPath == "" {
		unknown(receipt.ReasonCode)
		return
	}
	data, err := readPythonTargetRegular(o.OutputPath, 8<<20)
	if err != nil {
		unknown("target_observation_unavailable")
		return
	}
	var result struct {
		Nonce      string                                         `json:"nonce"`
		Status     string                                         `json:"status"`
		ReasonCode string                                         `json:"reason_code"`
		Targets    []types.VerificationProbeTargetExecutionTarget `json:"targets"`
	}
	if json.Unmarshal(data, &result) != nil || result.Nonce != o.Nonce || len(result.Targets) != len(o.Targets) || (result.Status != "complete" && result.Status != "unknown") {
		unknown("target_observation_invalid")
		return
	}
	plan := o.ctx.Mutable.ChangePlan()
	if plan == nil || plan.ID != receipt.PlanID || plan.PatchEffect == nil || verificationProbeExecutionDigest(plan.PatchEffect) != o.effectHash {
		unknown("target_plan_changed")
		return
	}
	for i, target := range result.Targets {
		expected := o.Targets[i]
		if target.Path != expected.Path || target.SourceSHA256 != expected.SourceSHA256 || (target.MappingComplete && !expected.MappingComplete) {
			unknown("target_observation_binding_mismatch")
			return
		}
		current, err := readPythonTargetSource(o.ExecutionRoot, expected.Path)
		if err != nil || pythonTargetSHA(current) != expected.SourceSHA256 {
			unknown("target_source_changed")
			return
		}
		if target.MappingComplete {
			var mapped []int
			for _, owner := range target.Owners {
				mapped = append(mapped, owner.ChangedLines...)
			}
			sort.Ints(mapped)
			if len(mapped) != len(expected.AddedLines) {
				unknown("target_mapping_incomplete")
				return
			}
			for j := range mapped {
				if mapped[j] != expected.AddedLines[j] {
					unknown("target_mapping_incomplete")
					return
				}
			}
		}
	}
	receipt.Targets, receipt.Status, receipt.ReasonCode = result.Targets, result.Status, result.ReasonCode
	receipt.ManifestSHA256 = types.VerificationProbeTargetManifestSHA256(&receipt)
}

func pythonTargetSHA(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func pythonTargetAppliedCommit(ctx context.Context, root string, effect *types.PatchEffectRecord) (string, error) {
	if strings.HasPrefix(effect.HeadRef, "-") {
		return "", os.ErrInvalid
	}
	resolved, err := pythonTargetGit(ctx, root, 256, "rev-parse", "--verify", effect.HeadRef+"^{commit}")
	commit := strings.TrimSpace(string(resolved))
	if err != nil || (len(commit) != 40 && len(commit) != 64) {
		return "", os.ErrInvalid
	}
	if _, err := hex.DecodeString(commit); err != nil {
		return "", err
	}
	var args []string
	switch effect.Source {
	case "applied_commit":
		// These arguments are the production CaptureCommitPatch format. This
		// read uses a shared short deadline and byte cap, never a new diff form.
		args = []string{"-c", "core.quotePath=false", "show", "--format=medium", "--no-color", commit}
	case "workflow_cumulative_owned_diff":
		if effect.BaseRef == "" || strings.HasPrefix(effect.BaseRef, "-") {
			return "", os.ErrInvalid
		}
		args = []string{"-c", "core.quotePath=false", "diff", "--no-color", "--binary", effect.BaseRef, commit, "--"}
		for _, file := range effect.Files {
			path := file.Path
			if path == "" || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path || path == ".." || strings.HasPrefix(path, "../") {
				return "", os.ErrInvalid
			}
			args = append(args, path)
		}
		if len(effect.Files) == 0 {
			return "", os.ErrInvalid
		}
	default:
		return "", os.ErrInvalid
	}
	patch, err := pythonTargetGit(ctx, root, 16<<20, args...)
	// CaptureCommitPatch/CaptureRangePatchForPaths use runGitIn, whose
	// published diff bytes are TrimSpace-normalized. Only the diff follows
	// that existing representation; committed/current source remains exact.
	if err != nil || pythonTargetSHA([]byte(strings.TrimSpace(string(patch)))) != effect.DiffFingerprint {
		return "", os.ErrInvalid
	}
	return commit, nil
}

func pythonTargetGit(ctx context.Context, root string, limit int64, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	if int64(len(data)) > limit || readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, os.ErrInvalid
	}
	if err := command.Wait(); err != nil {
		return nil, err
	}
	return data, nil
}

func pythonTargetCommitSource(ctx context.Context, root, commit, path string) ([]byte, error) {
	entry, err := pythonTargetGit(ctx, root, 8192, "ls-tree", "-z", commit, "--", path)
	if err != nil || bytes.Count(entry, []byte{0}) != 1 {
		return nil, os.ErrInvalid
	}
	metadata, actualPath, ok := strings.Cut(strings.TrimSuffix(string(entry), "\x00"), "\t")
	fields := strings.Fields(metadata)
	if !ok || actualPath != path || len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
		return nil, os.ErrInvalid
	}
	return pythonTargetGit(ctx, root, pythonTargetSourceLimit, "cat-file", "blob", fields[2])
}

func readPythonTargetSource(root, relative string) ([]byte, error) {
	full := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return nil, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	if resolved != filepath.Join(resolvedRoot, filepath.FromSlash(relative)) {
		return nil, os.ErrPermission
	}
	return readPythonTargetRegular(full, pythonTargetSourceLimit)
}

// Stat checks are immutability checks only, never invocation authority.
func readPythonTargetRegular(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, os.ErrInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !os.SameFile(info, before) {
		return nil, os.ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, os.ErrInvalid
	}
	after, err := f.Stat()
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || !os.SameFile(before, after) {
		return nil, os.ErrInvalid
	}
	return data, nil
}
