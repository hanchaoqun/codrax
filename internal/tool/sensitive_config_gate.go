package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Sensitive configuration file gate — tool-layer face (SEC audit #26 +
// 复核收窄, 2026-07-10).
//
// Threat: read-mode tools (read_file / grep / exec_command) could slurp the
// LIVE LLM credential store (providers.yaml api_key lines) into LLM prompts,
// plaintext .codrax logs, and — via the deterministic citation quote
// backfill — the final user-facing answer. The write side already classifies
// providers.yaml as sensitive (types.changePlanSlicePathRequiresIsolation);
// this is the read-side counterpart.
//
// The AUTHORITY predicate lives in internal/types
// (types.IsSensitiveConfigFilePath) so the internal read points that bypass
// tools — the citation quote backfill and the ground package file reader —
// consult the same rules (ground cannot import tool). This file keeps the
// tool-facing re-exports plus the tool-specific arms: typed refusal, soft
// advisory for out-of-domain same-named files, broad-scan excludes, the
// NativeGrep skip, and the read-mode exec_command token arm.
//
// Deny/allow shape (precise signals for the hard gate; user-intent red
// line for the allow side):
//   - HARD deny: registered active credential path (rule 1) and credential
//     basenames INSIDE a registered anchor directory (rule 2).
//   - ALLOW + soft advisory: an analyzed repository's own file that merely
//     shares the credential naming convention (e.g. Grafana provisioning
//     providers.yaml) — hard-gating it would make a legitimate user file
//     unanalyzable for zero security gain.
//
// Refusal wording is deliberately generic ("configuration credentials
// file") with ZERO path echo: the resolved internal path (exeDir layout)
// must not leak into LLM-facing output either.

// SetSensitiveConfigFilePaths registers the resolved ACTIVE credential file
// paths with the shared authority (see types.SetSensitiveConfigFilePaths).
func SetSensitiveConfigFilePaths(paths []string) {
	types.SetSensitiveConfigFilePaths(paths)
}

// SensitiveConfigFilePaths returns the registered credential path keys
// (diagnostic/test use only — never for LLM-facing output).
func SensitiveConfigFilePaths() []string {
	return types.SensitiveConfigFilePaths()
}

// IsSensitiveConfigFilePath is the tool-layer face of the shared predicate:
// it applies the tool path normalisation (Windows POSIX spellings) before
// consulting the authority in types.
func IsSensitiveConfigFilePath(path string) bool {
	p := strings.TrimSpace(path)
	if p == "" {
		return false
	}
	return types.IsSensitiveConfigFilePath(normalizeToolAbsolutePath(p))
}

// sensitiveConfigSoftAdvisory returns a soft, LLM-facing advisory line for a
// file that is ALLOWED (outside the credential anchor domain) but shares the
// credential naming convention. Soft guidance only — never a gate signal.
// Empty string when no advisory applies.
func sensitiveConfigSoftAdvisory(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return ""
	}
	if !types.SensitiveCredentialBasename(filepath.Base(p)) {
		return ""
	}
	if IsSensitiveConfigFilePath(p) {
		return "" // hard-denied elsewhere; no advisory lane
	}
	return "[advisory] this file's name matches a credentials-file naming convention; treat its contents as potentially sensitive and never quote secret values (API keys, tokens, passwords) verbatim into notes or answers."
}

// sensitiveConfigScanExcludes computes the broad directory-scan exclusions
// for the shell grep backends: ONLY the concrete sensitive files (registered
// credential paths + credential-named files inside the registered anchor
// dirs) that actually live under scanRoot are excluded. An external
// repository's same-named file is NOT excluded (复核必修①: it must stay
// searchable). rgGlobs are root-anchored (`!/rel/path`, precise); GNU grep
// only supports basename excludes, so grepExcludes over-excludes same-named
// nested files ONLY in the self-analysis case where an anchor file is inside
// the scan tree (native backend and rg stay precise).
func sensitiveConfigScanExcludes(scanRoot string) (rgGlobs, grepExcludes []string) {
	scanRoot = strings.TrimSpace(scanRoot)
	if scanRoot == "" {
		return nil, nil
	}
	if abs, err := filepath.Abs(normalizeToolAbsolutePath(scanRoot)); err == nil {
		scanRoot = abs
	}
	scanRoot = filepath.Clean(scanRoot)

	candidates := map[string]bool{}
	for _, p := range types.SensitiveConfigFilePaths() {
		candidates[p] = true
	}
	for _, dir := range types.SensitiveConfigAnchorDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if types.SensitiveCredentialBasename(entry.Name()) {
				candidates[filepath.Join(dir, entry.Name())] = true
			}
		}
	}

	seenGlob := map[string]bool{}
	seenExclude := map[string]bool{}
	for candidate := range candidates {
		rel, err := filepath.Rel(scanRoot, candidate)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			continue // not under the scan root — nothing to exclude
		}
		if glob := "!/" + rel; !seenGlob[glob] {
			seenGlob[glob] = true
			rgGlobs = append(rgGlobs, glob)
		}
		if base := filepath.Base(candidate); base != "" && !seenExclude[base] {
			seenExclude[base] = true
			grepExcludes = append(grepExcludes, base)
		}
	}
	return rgGlobs, grepExcludes
}

const sensitiveConfigRefusalBody = "the target is a configuration credentials file and is excluded from analysis. Credential/settings files are never loaded into analysis context; continue with repository source files — configuration behavior is defined by the code that loads it."

// sensitiveConfigRefusal builds the typed refusal ToolResult shared by the
// read_file / grep / exec_command gates. Generic wording, zero path echo.
func sensitiveConfigRefusal(toolName string) types.ToolResult {
	return types.ToolResult{
		ToolName: toolName,
		Success:  false,
		Summary:  fmt.Sprintf("%s refused: %s", toolName, sensitiveConfigRefusalBody),
		Repair: &types.ToolRepair{
			Code:   "sensitive_config_file_denied",
			Hint:   "Do not retry this path. Read the configuration-loading source code (e.g. the config package) instead of the credential/settings file itself.",
			Fields: []string{"path"},
		},
		Timestamp: time.Now(),
	}
}

// execCommandNamesSensitiveConfigPath reports whether any word token of a
// read-mode shell command resolves (against the repo root) to an EXISTING
// sensitive config file — the exec_command arm of the SEC #26 gate. Without
// it, `exec_command: cat providers.yaml` bypasses the read_file/grep gates
// through the read-mode shell allowlist (cat/grep/rg/sed are all allowed).
// Glob tokens are expanded so `cat provi*.yaml` cannot dodge the check.
// Existence is required: a token that names no file leaks nothing, and this
// keeps prose-like tokens from tripping the gate. Recursive directory scans
// that never NAME the file (e.g. `grep -r <pattern> .`) are out of this
// arm's precise-signal reach and are handled by the grep tool lane instead.
func execCommandNamesSensitiveConfigPath(ctx *types.BusContext, command string) bool {
	tokens, err := lexShellCommand(command)
	if err != nil {
		return false // the read-mode validator already rejects unlexable commands
	}
	for _, tok := range tokens {
		if tok.kind != shellTokenWord {
			continue
		}
		word := strings.TrimSpace(tok.text)
		if word == "" || strings.HasPrefix(word, "-") {
			continue
		}
		resolved := resolveToolPath(ctx, word)
		if resolved == "" {
			continue
		}
		if strings.ContainsAny(word, "*?[") {
			matches, globErr := filepath.Glob(resolved)
			if globErr != nil {
				continue
			}
			for _, match := range matches {
				if info, statErr := os.Stat(match); statErr == nil && !info.IsDir() && IsSensitiveConfigFilePath(match) {
					return true
				}
			}
			continue
		}
		if info, statErr := os.Stat(resolved); statErr != nil || info.IsDir() {
			continue
		}
		if IsSensitiveConfigFilePath(resolved) {
			return true
		}
	}
	return false
}

// grepNativeShouldSkip builds the NativeGrep ShouldSkip callback: the
// sensitive-config file skip (always on) layered over the repo-relative
// directory exclusion policy (when a repo root is known).
func grepNativeShouldSkip(ctx *types.BusContext, dirFilter SearchDirFilter) func(path string, d os.DirEntry) bool {
	hasRepoRoot := ctx != nil && ctx.RepoRoot != ""
	return func(path string, d os.DirEntry) bool {
		if d != nil && !d.IsDir() && IsSensitiveConfigFilePath(path) {
			return true
		}
		if !hasRepoRoot {
			return false
		}
		rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, path)
		return ok && dirFilter.ExcludesRepoRelativePath(rel)
	}
}
