// Package userhint holds tiny shared extractors for operator-typed
// hints (PIB-5c @path pins; TTY-3 steering notes) so the REPL and the
// orchestrator consume ONE implementation instead of drifting copies.
package userhint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// pinnedFileTokenRE matches an @token up to the next whitespace. The
// leading token boundary keeps email-like text (a@b.c) from matching.
var pinnedFileTokenRE = regexp.MustCompile(`(?:^|\s)@([^\s@]+)`)

// ExtractPinnedFiles returns the repo-relative paths of every @token
// in text that resolves to an existing regular file inside repoRoot.
// Order preserved, duplicates removed, ../ and absolute escapes never
// pin, trailing sentence punctuation stripped before the stat.
func ExtractPinnedFiles(repoRoot, text string) []string {
	if strings.TrimSpace(repoRoot) == "" || !strings.Contains(text, "@") {
		return nil
	}
	seen := map[string]bool{}
	var pins []string
	for _, match := range pinnedFileTokenRE.FindAllStringSubmatch(text, -1) {
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
