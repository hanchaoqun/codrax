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

// readRunAutoResumeDisclosureMsg — visible line when the explicitly
// enabled auto-resume lane revives a prior read run.
func readRunAutoResumeDisclosureMsg(lang, runID string) string {
	if isZh(lang) {
		return "检测到匹配的读运行快照: " + runID + "(read_run_auto_resume 显式开启;校验通过则续跑,不通过将重新完整探索;/clear 可清除全部快照)"
	}
	return "Matching read-run snapshot found: " + runID + " (read_run_auto_resume explicitly enabled; resumes only if validation passes, otherwise a full fresh exploration runs; /clear wipes all snapshots)"
}

// readRunSnapshotsClearedMsg — /clear's snapshot arm confirmation.
func readRunSnapshotsClearedMsg(lang string, n int) string {
	if isZh(lang) {
		return "已清除 " + itoa(n) + " 个读运行快照(不会再自动复活)"
	}
	return "Cleared " + itoa(n) + " read-run snapshot(s) (no auto-revival possible)"
}
