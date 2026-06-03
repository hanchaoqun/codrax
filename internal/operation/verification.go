package operation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	VerificationVerified = "verified"
	VerificationFailed   = "failed"
	VerificationUnknown  = "unknown"
)

func verifyCommandStepOutcome(plan CommandOperationPlan, step CommandStep) OperationVerificationResult {
	if hint := strings.TrimSpace(step.VerifyHint); hint != "" {
		return verifyCommandHint(plan, step, hint)
	}
	switch baseProgram(step.Program) {
	case "mkdir", "touch", "cp", "mv", "tee":
		return verifyPathCandidates(plan, step, "path_exists", true)
	case "rm":
		return verifyPathCandidates(plan, step, "path_absent", false)
	}
	return OperationVerificationResult{}
}

func verifyCommandHint(plan CommandOperationPlan, step CommandStep, hint string) OperationVerificationResult {
	kind, value, ok := strings.Cut(hint, ":")
	if !ok {
		return OperationVerificationResult{
			Status:  VerificationUnknown,
			Kind:    "unsupported_hint",
			Summary: fmt.Sprintf("unsupported verify_hint %q; supported forms: path_exists:, file_exists:, dir_exists:, path_absent:", hint),
		}
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	value = strings.TrimSpace(value)
	switch kind {
	case "exists":
		kind = "path_exists"
	}
	path := resolveCommandVerificationPath(plan, step, value)
	switch kind {
	case "path_exists":
		return verifySinglePath(path, kind, func(info os.FileInfo) bool { return info != nil }, "exists")
	case "file_exists":
		return verifySinglePath(path, kind, func(info os.FileInfo) bool { return info != nil && !info.IsDir() }, "exists as a file")
	case "dir_exists":
		return verifySinglePath(path, kind, func(info os.FileInfo) bool { return info != nil && info.IsDir() }, "exists as a directory")
	case "path_absent":
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			return OperationVerificationResult{Status: VerificationVerified, Kind: kind, Path: path, Summary: fmt.Sprintf("%s is absent", path)}
		}
		if err != nil {
			return OperationVerificationResult{Status: VerificationUnknown, Kind: kind, Path: path, Summary: fmt.Sprintf("could not verify %s absence: %v", path, err)}
		}
		_ = info
		return OperationVerificationResult{Status: VerificationFailed, Kind: kind, Path: path, Summary: fmt.Sprintf("%s still exists", path)}
	default:
		return OperationVerificationResult{
			Status:  VerificationUnknown,
			Kind:    "unsupported_hint",
			Path:    path,
			Summary: fmt.Sprintf("unsupported verify_hint kind %q", kind),
		}
	}
}

func verifyPathCandidates(plan CommandOperationPlan, step CommandStep, kind string, shouldExist bool) OperationVerificationResult {
	paths := commandWritePathCandidates(step)
	if len(paths) == 0 {
		return OperationVerificationResult{}
	}
	var checked []string
	for _, value := range paths {
		path := resolveCommandVerificationPath(plan, step, value)
		checked = append(checked, path)
		_, err := os.Stat(path)
		if shouldExist {
			if err != nil {
				if os.IsNotExist(err) {
					return OperationVerificationResult{Status: VerificationFailed, Kind: kind, Path: path, Summary: fmt.Sprintf("%s does not exist after command", path)}
				}
				return OperationVerificationResult{Status: VerificationUnknown, Kind: kind, Path: path, Summary: fmt.Sprintf("could not verify %s: %v", path, err)}
			}
			continue
		}
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return OperationVerificationResult{Status: VerificationUnknown, Kind: kind, Path: path, Summary: fmt.Sprintf("could not verify %s absence: %v", path, err)}
		}
		return OperationVerificationResult{Status: VerificationFailed, Kind: kind, Path: path, Summary: fmt.Sprintf("%s still exists after command", path)}
	}
	if shouldExist {
		return OperationVerificationResult{Status: VerificationVerified, Kind: kind, Path: strings.Join(checked, ","), Summary: fmt.Sprintf("verified path outcome for %d target(s)", len(checked))}
	}
	return OperationVerificationResult{Status: VerificationVerified, Kind: kind, Path: strings.Join(checked, ","), Summary: fmt.Sprintf("verified absence for %d target(s)", len(checked))}
}

func verifySinglePath(path, kind string, predicate func(os.FileInfo) bool, expectation string) OperationVerificationResult {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return OperationVerificationResult{Status: VerificationFailed, Kind: kind, Path: path, Summary: fmt.Sprintf("%s does not %s", path, expectation)}
		}
		return OperationVerificationResult{Status: VerificationUnknown, Kind: kind, Path: path, Summary: fmt.Sprintf("could not verify %s: %v", path, err)}
	}
	if predicate(info) {
		return OperationVerificationResult{Status: VerificationVerified, Kind: kind, Path: path, Summary: fmt.Sprintf("%s %s", path, expectation)}
	}
	return OperationVerificationResult{Status: VerificationFailed, Kind: kind, Path: path, Summary: fmt.Sprintf("%s does not %s", path, expectation)}
}

func resolveCommandVerificationPath(plan CommandOperationPlan, step CommandStep, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	base := strings.TrimSpace(step.WorkDir)
	if base == "" {
		base = strings.TrimSpace(plan.WorkDir)
	}
	if base == "" {
		base = "."
	}
	if abs, err := filepath.Abs(filepath.Join(base, value)); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(filepath.Join(base, value))
}
