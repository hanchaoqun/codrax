package types

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Sensitive configuration file predicate (SEC audit #26 + 复核收窄,
// 2026-07-10). Lives in types (the shared base package) because BOTH the
// tool layer (read_file / grep / exec_command gates) and the internal
// read points that bypass tools — the citation quote backfill
// (internal/tool answer_document_citation_quote_normalize) and the
// grounding file reader (internal/tool/ground scope_dispatch) — must
// consult the SAME authority; ground cannot import tool (import cycle).
//
// Precise-signal HARD-gate rules (CLAUDE.md red line):
//
//  1. Exact resolved-path equality against the REGISTERED credential file
//     paths (the resolved providers_config). Denied wherever it lives,
//     whatever it is named.
//  2. Credential basename pattern (providers.yaml / providers.*.yaml)
//     ONLY inside a registered anchor directory (the directory of a
//     registered credential file — i.e. the exe/config anchor). This
//     covers unregistered fallback-slot siblings like
//     providers.deepseek.yaml next to the binary, while an EXTERNAL
//     analyzed repository's legitimately same-named file (e.g. Grafana
//     provisioning providers.yaml) stays fully readable/searchable —
//     hard-gating it would violate the user-intent red line for zero
//     security gain (rule 1 already covers codrax's real keys).
//
// The runtime settings file (codrax.yaml) is deliberately NOT registered:
// it carries runtime knobs, not credentials (CLAUDE.md), and refusing
// "show me my verify_mem_limit_mb" has zero security benefit.
var (
	sensitiveConfigMu sync.RWMutex
	// sensitiveConfigPaths: canonical keys of registered credential files
	// (cleaned + symlink-resolved spellings both present).
	sensitiveConfigPaths map[string]struct{}
	// sensitiveConfigAnchorDirs: canonical keys of the registered files'
	// parent directories (rule 2 domain), plus an ordered slice of the
	// original dir spellings for scan-exclude enumeration.
	sensitiveConfigAnchorDirs    map[string]struct{}
	sensitiveConfigAnchorDirList []string
)

// SetSensitiveConfigFilePaths registers the resolved ACTIVE credential file
// paths (providers_config after anchor resolution). Called from cmd/root.go
// once anchors are resolved; nil clears the registry (tests).
func SetSensitiveConfigFilePaths(paths []string) {
	registry := make(map[string]struct{}, len(paths)*2)
	anchors := make(map[string]struct{}, len(paths)*2)
	anchorList := make([]string, 0, len(paths))
	addAnchor := func(dir string) {
		key := sensitiveConfigPathKey(dir)
		if _, seen := anchors[key]; seen {
			return
		}
		anchors[key] = struct{}{}
		anchorList = append(anchorList, dir)
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		p = filepath.Clean(p)
		registry[sensitiveConfigPathKey(p)] = struct{}{}
		addAnchor(filepath.Dir(p))
		// Register the symlink-resolved spelling too, so a resolved read
		// path and a registered symlinked config path still collide.
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			registry[sensitiveConfigPathKey(resolved)] = struct{}{}
			addAnchor(filepath.Dir(resolved))
		}
	}
	sensitiveConfigMu.Lock()
	defer sensitiveConfigMu.Unlock()
	sensitiveConfigPaths = registry
	sensitiveConfigAnchorDirs = anchors
	sensitiveConfigAnchorDirList = anchorList
}

// SensitiveConfigFilePaths returns the registered credential path keys
// (diagnostic/test use only — never for LLM-facing output).
func SensitiveConfigFilePaths() []string {
	sensitiveConfigMu.RLock()
	defer sensitiveConfigMu.RUnlock()
	out := make([]string, 0, len(sensitiveConfigPaths))
	for p := range sensitiveConfigPaths {
		out = append(out, p)
	}
	return out
}

// SensitiveConfigAnchorDirs returns the rule-2 anchor directories (original
// spellings; used by the tool layer to compute precise broad-scan excludes).
func SensitiveConfigAnchorDirs() []string {
	sensitiveConfigMu.RLock()
	defer sensitiveConfigMu.RUnlock()
	return append([]string(nil), sensitiveConfigAnchorDirList...)
}

// sensitiveConfigPathKey canonicalises one absolute path for registry
// comparison. Windows AND darwin compare case-insensitively (both default
// to case-insensitive filesystems; a case-variant spelling must not bypass
// the gate).
func sensitiveConfigPathKey(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(p)
	}
	return p
}

func registeredSensitiveConfigPath(abs string) bool {
	sensitiveConfigMu.RLock()
	defer sensitiveConfigMu.RUnlock()
	if len(sensitiveConfigPaths) == 0 {
		return false
	}
	_, ok := sensitiveConfigPaths[sensitiveConfigPathKey(abs)]
	return ok
}

func sensitiveConfigDirIsAnchor(dir string) bool {
	sensitiveConfigMu.RLock()
	defer sensitiveConfigMu.RUnlock()
	if len(sensitiveConfigAnchorDirs) == 0 {
		return false
	}
	_, ok := sensitiveConfigAnchorDirs[sensitiveConfigPathKey(dir)]
	return ok
}

// SensitiveCredentialBasename reports whether base spells the providers
// credential store: exactly `providers.yaml`, or the fallback-slot pattern
// `providers.*.yaml` (e.g. providers.deepseek.yaml). Anchored glob — a
// precise signal, not a substring heuristic.
func SensitiveCredentialBasename(base string) bool {
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "providers.yaml" {
		return true
	}
	matched, err := filepath.Match("providers.*.yaml", base)
	return err == nil && matched
}

// IsSensitiveConfigFilePath is the gate predicate. path may be absolute or
// CWD-relative (it is resolved before comparison); symlinks are followed so
// an in-repo symlink pointing at the active config cannot bypass rule 1.
func IsSensitiveConfigFilePath(path string) bool {
	p := strings.TrimSpace(path)
	if p == "" {
		return false
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	p = filepath.Clean(p)
	candidates := []string{p}
	if resolved, err := filepath.EvalSymlinks(p); err == nil && sensitiveConfigPathKey(resolved) != sensitiveConfigPathKey(p) {
		candidates = append(candidates, resolved)
	}
	for _, candidate := range candidates {
		if registeredSensitiveConfigPath(candidate) {
			return true
		}
		if SensitiveCredentialBasename(filepath.Base(candidate)) && sensitiveConfigDirIsAnchor(filepath.Dir(candidate)) {
			return true
		}
	}
	return false
}
