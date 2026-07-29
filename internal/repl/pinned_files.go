package repl

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PIB-5c (pi borrow — pi's @-file references, ledger
// docs/design/pi_borrow_analysis_20260729.md §7.5): `@path/to/file.go`
// tokens in a request pin those files as deterministic must-read
// anchors for the read pipeline. Extraction is fs-validated — only
// tokens that resolve to an existing regular file under the repo root
// become pins; everything else stays plain text (an "@user" mention or
// a decorative @ never turns into a phantom read obligation).

// pinnedFilesSetter is the optional runner interface (same pattern as
// attachedLogSetter): test stubs without it silently skip propagation.
type pinnedFilesSetter interface {
	SetUserPinnedFiles(paths []string)
}

// pinnedFileTokenRE matches an @ token up to the next whitespace.
// Leading token boundary keeps email-like text (a@b.c) from matching.
var pinnedFileTokenRE = regexp.MustCompile(`(?:^|\s)@([^\s@]+)`)

// pinnedFilesMsg confirms which @path tokens actually became pins.
func pinnedFilesMsg(lang string, pins []string) string {
	joined := strings.Join(pins, ", ")
	if isZh(lang) {
		return "已钉选必读文件: " + joined
	}
	return "Pinned must-read files: " + joined
}

// extractPinnedFiles returns the repo-relative paths of every @token
// in the request that resolves to an existing regular file inside
// repoRoot. Order preserved, duplicates removed. Trailing punctuation
// that commonly rides sentence endings is stripped before the stat.
func extractPinnedFiles(repoRoot, request string) []string {
	if strings.TrimSpace(repoRoot) == "" || !strings.Contains(request, "@") {
		return nil
	}
	seen := map[string]bool{}
	var pins []string
	for _, match := range pinnedFileTokenRE.FindAllStringSubmatch(request, -1) {
		candidate := strings.TrimRight(match[1], ".,;:!?)]}\"'`")
		if candidate == "" || strings.Contains(candidate, "..") {
			continue
		}
		rel := filepath.ToSlash(filepath.Clean(candidate))
		if filepath.IsAbs(rel) || seen[rel] {
			continue
		}
		info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil || info.IsDir() {
			continue
		}
		seen[rel] = true
		pins = append(pins, rel)
	}
	return pins
}
