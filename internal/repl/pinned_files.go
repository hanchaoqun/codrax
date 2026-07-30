package repl

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/userhint"
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

// pinnedFilesMsg confirms which @path tokens actually became pins.
func pinnedFilesMsg(lang string, pins []string) string {
	joined := strings.Join(pins, ", ")
	if isZh(lang) {
		return "已钉选必读文件: " + joined
	}
	return "Pinned must-read files: " + joined
}

// extractPinnedFiles delegates to the shared extractor (TTY-3 moved
// the implementation to internal/userhint so the orchestrator's
// steering lane consumes the same logic instead of a drifting copy).
func extractPinnedFiles(repoRoot, request string) []string {
	return userhint.ExtractPinnedFiles(repoRoot, request)
}

// followUpReplayMsg echoes which queued follow-up line is running now.
func followUpReplayMsg(lang, line string) string {
	if isZh(lang) {
		return "回放排队输入: " + line
	}
	return "Replaying queued input: " + line
}
