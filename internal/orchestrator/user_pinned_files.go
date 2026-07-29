package orchestrator

// AttachedLog returns the current attached-log payload. Read surface
// for the REPL's /log show handler. (Relocated from orchestrator.go
// with the PIB-5c setter to keep the god-file LOC ratchet honest.)
func (o *Orchestrator) AttachedLog() string {
	return o.attachedLog
}

// SetUserPinnedFiles stores the @path pins extracted from the current
// request (PIB-5c, ledger docs/design/pi_borrow_analysis_20260729.md
// §7.5). Per-turn lifetime: the REPL sets it fresh before every Run
// (nil clears), so stale pins never leak across turns. Lives in its
// own concern file so the orchestrator god-file LOC ratchet keeps its
// pressure.
func (o *Orchestrator) SetUserPinnedFiles(paths []string) {
	o.userPinnedFiles = paths
}
