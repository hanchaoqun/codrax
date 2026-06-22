package repl

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func (r *REPL) handleReadRunsCmd(line string) {
	if r.readRunSnapshotStore == nil {
		r.info(commandDisabled(r.language, "/read-runs", "read run snapshot store unavailable"))
		return
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/read-runs"))
	if rest == "" {
		rest = "list"
	}
	switch {
	case rest == "list":
		r.handleReadRunsList()
	case rest == "show" || strings.HasPrefix(rest, "show "):
		id := readRunCommandID(rest, "show")
		if id == "" {
			r.info("/read-runs show <run-id>")
			return
		}
		r.handleReadRunsShow(id)
	case rest == "resume" || strings.HasPrefix(rest, "resume "):
		id := readRunCommandID(rest, "resume")
		if id == "" {
			r.info("/read-runs resume <run-id>")
			return
		}
		r.handleReadRunsResume(id)
	case rest == "clear" || strings.HasPrefix(rest, "clear "):
		id := readRunCommandID(rest, "clear")
		if id == "" {
			r.info("/read-runs clear <run-id>")
			return
		}
		r.handleReadRunsClear(id)
	default:
		r.info("/read-runs <list|show <run-id>|resume <run-id>|clear <run-id>>")
	}
}

func (r *REPL) handleReadRunsList() {
	infos, err := r.readRunSnapshotStore.List()
	if err != nil {
		r.errorf("read-runs list: %v\n", err)
		return
	}
	if len(infos) == 0 {
		r.info(fmt.Sprintf("read-runs: no snapshots in %s", r.readRunSnapshotStore.RunDir()))
		return
	}
	r.renderBordered(readRunSnapshotListMarkdown(r.readRunSnapshotStore.RunDir(), infos))
}

func (r *REPL) handleReadRunsShow(id string) {
	snapshot, err := r.readRunSnapshotStore.Load(id)
	if err != nil {
		r.errorf("read-runs show: %v\n", err)
		return
	}
	if snapshot == nil {
		r.info(fmt.Sprintf("read-runs show: snapshot %q not found", id))
		return
	}
	var prior *types.ReadRunSnapshot
	if r.readRunSnapshotStore != nil {
		prior, _ = r.readRunSnapshotStore.LoadComparablePrior(*snapshot)
	}
	r.renderBordered(readRunSnapshotMarkdownWithReplayAudit(*snapshot, prior))
}

func (r *REPL) handleReadRunsResume(id string) {
	seedSetter, ok := r.runner.(readRunSnapshotSeedSetter)
	if !ok {
		r.info("read-runs resume: runner does not support typed read snapshot seeds")
		return
	}
	snapshot, err := r.readRunSnapshotStore.Load(id)
	if err != nil {
		r.errorf("read-runs resume: %v\n", err)
		return
	}
	if snapshot == nil {
		r.info(fmt.Sprintf("read-runs resume: snapshot %q not found", id))
		return
	}
	request := strings.TrimSpace(snapshot.Request)
	repoRoot := strings.TrimSpace(snapshot.RepoRoot)
	if request == "" {
		r.info(fmt.Sprintf("read-runs resume: snapshot %q has empty request", id))
		return
	}
	if repoRoot == "" {
		r.info(fmt.Sprintf("read-runs resume: snapshot %q has empty repo root", id))
		return
	}

	seedSetter.SetReadRunSnapshotSeed(snapshot)
	defer seedSetter.SetReadRunSnapshotSeed(nil)
	if mode, ok := r.runner.(modeSetter); ok {
		originalMode := r.currentMode
		mode.SetMode(types.ModeRead)
		defer mode.SetMode(originalMode)
	}
	if transcript, ok := r.runner.(outputTranscriptRequestSetter); ok {
		transcript.SetOutputTranscriptRequest(request)
		defer transcript.SetOutputTranscriptRequest("")
	}

	if r.renderer != nil {
		r.renderer.SetTotalStages(4)
		r.renderer.StartSpinnerWithCancelHint(spinnerCancelHint(r.language))
	}
	busCtx, runErr := r.runInFlightWrap(func() (*types.BusContext, error) {
		return r.runner.Run(request, repoRoot, r.branch)
	})
	if r.renderer != nil {
		armDockTerminalState(r.renderer, runErr)
		r.renderer.StopSpinner()
	}
	if runErr != nil {
		r.errorf("read-runs resume: %s\n", friendlyRunError(r.language, runErr))
		return
	}

	response := strings.TrimSpace(r.render(busCtx))
	if response == "" || response == "(no result)" {
		fmt.Fprintln(r.out, emptyResponseHint(r.language))
		return
	}
	r.renderBordered(response)
}

func (r *REPL) handleReadRunsClear(id string) {
	if err := r.readRunSnapshotStore.Clear(id); err != nil {
		r.errorf("read-runs clear: %v\n", err)
		return
	}
	r.success(fmt.Sprintf("read-runs cleared: %s", id))
}

func readRunCommandID(rest, command string) string {
	fields := strings.Fields(rest)
	if len(fields) >= 2 && fields[0] == command {
		return fields[1]
	}
	return ""
}

func readRunSnapshotListMarkdown(dir string, infos []ReadRunSnapshotInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Read run snapshots in `%s`\n\n", dir)
	for _, info := range infos {
		mod := ""
		if info.ModTime > 0 {
			mod = time.Unix(info.ModTime, 0).Format("2006-01-02 15:04:05")
		}
		hash := readRunShortHash(info.TaskGraphHash)
		request := readRunCompact(info.Request, 96)
		fmt.Fprintf(&b, "- `%s`", info.ID)
		if mod != "" {
			fmt.Fprintf(&b, " · %s", mod)
		}
		fmt.Fprintf(&b, " · nodes=%d statuses=%d reads=%d evidence=%d",
			info.TaskNodeCount, info.NodeStatusCount, info.ReadFileCount, info.AcceptedEvidence)
		if hash != "" {
			fmt.Fprintf(&b, " · graph=%s", hash)
		}
		if request != "" {
			fmt.Fprintf(&b, " · %s", request)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func readRunSnapshotMarkdown(snapshot types.ReadRunSnapshot) string {
	return readRunSnapshotMarkdownWithReplayAudit(snapshot, nil)
}

func readRunSnapshotMarkdownWithReplayAudit(snapshot types.ReadRunSnapshot, prior *types.ReadRunSnapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Read run `%s`\n\n", snapshot.RunID)
	fmt.Fprintf(&b, "- Schema: `%d`\n", snapshot.SchemaVersion)
	if !snapshot.CreatedAt.IsZero() {
		fmt.Fprintf(&b, "- Created: `%s`\n", snapshot.CreatedAt.UTC().Format(time.RFC3339))
	}
	if strings.TrimSpace(snapshot.RepoRoot) != "" {
		fmt.Fprintf(&b, "- Repo: `%s`\n", snapshot.RepoRoot)
	}
	if strings.TrimSpace(snapshot.Request) != "" {
		fmt.Fprintf(&b, "- Request: %s\n", readRunCompact(snapshot.Request, 160))
	}
	if strings.TrimSpace(snapshot.RequestHash) != "" {
		fmt.Fprintf(&b, "- Request fingerprint: `%s`\n", readRunShortHash(snapshot.RequestHash))
	}
	if repo := readRunRepoFingerprintSummary(snapshot.RepoFingerprint); repo != "" {
		fmt.Fprintf(&b, "- Repo fingerprint: %s\n", repo)
	}
	if env := readRunEnvironmentFingerprintSummary(snapshot.Environment); env != "" {
		fmt.Fprintf(&b, "- Environment fingerprint: %s\n", env)
	}
	if attachments := readRunAttachmentFingerprintSummary(snapshot.Attachments); attachments != "" {
		fmt.Fprintf(&b, "- Attachment fingerprints: %s\n", attachments)
	}
	fmt.Fprintf(&b, "- Task graph: hash=`%s` nodes=%d\n", readRunShortHash(snapshot.TaskGraphHash), snapshot.TaskNodeCount)
	if len(snapshot.NodeStatuses) > 0 {
		fmt.Fprintf(&b, "- Node statuses: %s\n", readRunNodeStatusSummary(snapshot.NodeStatuses))
	}
	if len(snapshot.ReadSet) > 0 {
		fmt.Fprintf(&b, "- Read files: %d (%s)\n", len(snapshot.ReadSet), readRunCompact(strings.Join(snapshot.ReadSet, ", "), 180))
	} else {
		b.WriteString("- Read files: 0\n")
	}
	if len(snapshot.AcceptedEvidence) > 0 {
		fmt.Fprintf(&b, "- Accepted evidence refs: %d\n", len(snapshot.AcceptedEvidence))
	} else {
		b.WriteString("- Accepted evidence refs: 0\n")
	}
	if snapshot.ProgressDecision.ReasonCode != "" {
		fmt.Fprintf(&b, "- Progress: reason=`%s` should_replan=%t\n",
			snapshot.ProgressDecision.ReasonCode, snapshot.ProgressDecision.ShouldReplan)
	}
	if active := readRunActiveStateSummary(snapshot.ActiveState); active != "" {
		fmt.Fprintf(&b, "- Active state: %s\n", active)
	}
	if snapshot.SourceInventory.IsActive() {
		fmt.Fprintf(&b, "- Source inventory: %s\n", readRunSourceInventorySummary(snapshot.SourceInventory))
	}
	if prior != nil {
		if audit := readRunReplayAuditSummary(*prior, snapshot); audit != "" {
			b.WriteString(audit)
		}
	}
	b.WriteString("\nAdvanced: `/read-runs resume ")
	b.WriteString(snapshot.RunID)
	b.WriteString("` · `/read-runs clear ")
	b.WriteString(snapshot.RunID)
	b.WriteString("`\n")
	return b.String()
}

func readRunReplayAuditSummary(prior, current types.ReadRunSnapshot) string {
	audit := types.BuildReadRunReplayAudit(&prior, &current, nil)
	if strings.TrimSpace(audit.BaseRunID) == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- Replay audit: baseline=`%s` request=%s graph=%s repo=%s\n",
		audit.BaseRunID, audit.RequestHashMatch, audit.TaskGraphHashMatch, audit.RepoFingerprintMatch)
	if status := readRunReplayAuditCountDeltaSummary(audit.NodeStatusDeltas, 5); status != "" {
		fmt.Fprintf(&b, "- Replay audit status delta: %s\n", status)
	}
	fmt.Fprintf(&b, "- Replay audit evidence/read/artifacts: evidence=%s read_set=%s artifacts=%s\n",
		readRunReplayAuditSetDeltaSummary(audit.AcceptedEvidenceDelta),
		readRunReplayAuditSetDeltaSummary(audit.ReadSetDelta),
		readRunReplayAuditSetDeltaSummary(audit.NodeArtifactDelta))
	src := audit.SourceInventoryDelta
	if src.ActiveBefore || src.ActiveAfter {
		fmt.Fprintf(&b, "- Replay audit source inventory: active=%t→%t members=%d→%d classes=%d→%d scopes=%s\n",
			src.ActiveBefore, src.ActiveAfter,
			src.MemberCountBefore, src.MemberCountAfter,
			src.SourceClassCountBefore, src.SourceClassCountAfter,
			readRunReplayAuditSetDeltaSummary(src.ScopeDelta))
	}
	if audit.ActiveStateBefore || audit.ActiveStateAfter {
		fmt.Fprintf(&b, "- Replay audit active state: %t→%t\n", audit.ActiveStateBefore, audit.ActiveStateAfter)
	}
	return b.String()
}

func readRunReplayAuditSetDeltaSummary(delta types.ReadRunReplayAuditSetDelta) string {
	delta = types.NormalizeReadRunReplayAudit(types.ReadRunReplayAudit{ReadSetDelta: delta}).ReadSetDelta
	return fmt.Sprintf("%d→%d +%d -%d", delta.BeforeCount, delta.AfterCount, delta.AddedCount, delta.RemovedCount)
}

func readRunReplayAuditCountDeltaSummary(items []types.ReadRunReplayAuditCountDelta, limit int) string {
	if limit <= 0 {
		limit = len(items)
	}
	var parts []string
	for _, item := range items {
		if item.Name == "" || item.Delta == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d→%d (%+d)", item.Name, item.Before, item.After, item.Delta))
		if len(parts) >= limit {
			break
		}
	}
	return strings.Join(parts, ", ")
}

func readRunActiveStateSummary(state types.ReadRunActiveState) string {
	state = types.NormalizeReadRunActiveState(state)
	if !state.IsActive() {
		return ""
	}
	var parts []string
	if state.TransientRetryPending {
		parts = append(parts, fmt.Sprintf("transient_retry=present reason=`%s` hint_hash=`%s` hint_bytes=%d",
			state.TransientRetryReasonCode, readRunShortHash(state.TransientRetryHintHash), state.TransientRetryHintBytes))
	}
	if state.ReadLoopNextAction.Active {
		parts = append(parts, fmt.Sprintf("next_action=`%s` reason=`%s`",
			state.ReadLoopNextAction.Action, state.ReadLoopNextAction.ReasonCode))
	}
	if state.ReadDispatchPolicy.IsActive() {
		parts = append(parts, fmt.Sprintf("policy=active tools=%d scopes=%d max_calls=%d",
			len(state.ReadDispatchPolicy.AllowedTools), len(state.ReadDispatchPolicy.ScopePaths), state.ReadDispatchPolicy.MaxToolCalls))
	}
	return strings.Join(parts, " · ")
}

func readRunNodeStatusSummary(statuses map[string]types.NodeExecStatus) string {
	counts := map[types.NodeExecStatus]int{}
	for _, status := range statuses {
		counts[types.NormalizeNodeExecStatus(status)]++
	}
	order := []types.NodeExecStatus{
		types.NodeExecPending,
		types.NodeExecRunning,
		types.NodeExecDone,
		types.NodeExecFailed,
		types.NodeExecRequeued,
	}
	var parts []string
	for _, status := range order {
		if count := counts[status]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", status, count))
		}
	}
	return strings.Join(parts, " ")
}

func readRunSourceInventorySummary(obs types.SourceInventoryObservation) string {
	var parts []string
	if obs.Complete {
		parts = append(parts, "complete=true")
	}
	if len(obs.SourceClasses) > 0 {
		var classParts []string
		for _, cls := range obs.SourceClasses {
			if cls.Role == "" {
				continue
			}
			classParts = append(classParts, fmt.Sprintf("%s=%d", cls.Role, cls.Count))
		}
		if len(classParts) > 0 {
			parts = append(parts, "classes: "+strings.Join(classParts, ", "))
		}
	}
	if len(obs.Sets) > 0 {
		parts = append(parts, fmt.Sprintf("sets=%d", len(obs.Sets)))
	}
	if len(parts) == 0 {
		return "active=true"
	}
	return strings.Join(parts, " · ")
}

func readRunRepoFingerprintSummary(fp types.ReadRunRepoFingerprint) string {
	fp = types.NormalizeReadRunRepoFingerprint(fp)
	if fp.Kind == "" {
		return ""
	}
	if !fp.Available {
		reason := strings.TrimSpace(fp.ReasonCode)
		if reason == "" {
			reason = "unavailable"
		}
		return fmt.Sprintf("kind=`%s` unavailable reason=`%s`", fp.Kind, reason)
	}
	parts := []string{fmt.Sprintf("kind=`%s` head=`%s`", fp.Kind, readRunShortHash(fp.Head))}
	if strings.TrimSpace(fp.StatusHash) != "" {
		parts = append(parts, fmt.Sprintf("status=`%s`", readRunShortHash(fp.StatusHash)))
	}
	return strings.Join(parts, " ")
}

func readRunAttachmentFingerprintSummary(items []types.ReadRunAttachmentFingerprint) string {
	items = types.NormalizeReadRunAttachmentFingerprints(items)
	if len(items) == 0 {
		return ""
	}
	var parts []string
	for _, item := range items {
		if item.Present {
			part := fmt.Sprintf("%s=%s/%dB", item.Kind, readRunShortHash(item.Hash), item.Bytes)
			if strings.TrimSpace(item.Source) != "" {
				part += " source=" + item.Source
			}
			parts = append(parts, part)
			continue
		}
		reason := strings.TrimSpace(item.ReasonCode)
		if reason == "" {
			reason = "unavailable"
		}
		parts = append(parts, fmt.Sprintf("%s=unavailable(%s)", item.Kind, reason))
	}
	return strings.Join(parts, " ")
}

func readRunEnvironmentFingerprintSummary(fp types.ReadRunEnvironmentFingerprint) string {
	fp = types.NormalizeReadRunEnvironmentFingerprint(fp)
	if fp.Kind == "" {
		return ""
	}
	if !fp.Available {
		reason := strings.TrimSpace(fp.ReasonCode)
		if reason == "" {
			reason = "unavailable"
		}
		return fmt.Sprintf("kind=`%s` unavailable reason=`%s`", fp.Kind, reason)
	}
	var parts []string
	identity := fmt.Sprintf("kind=`%s`", fp.Kind)
	if fp.CodraxVersion != "" {
		identity += " codrax=`" + fp.CodraxVersion + "`"
	}
	if fp.CodraxBuildTime != "" {
		identity += " build=`" + fp.CodraxBuildTime + "`"
	}
	if fp.GoVersion != "" {
		identity += " go=`" + fp.GoVersion + "`"
	}
	if fp.GOOS != "" || fp.GOARCH != "" {
		identity += " platform=`" + strings.Trim(strings.Join([]string{fp.GOOS, fp.GOARCH}, "/"), "/") + "`"
	}
	parts = append(parts, identity)
	if len(fp.Tools) > 0 {
		parts = append(parts, fmt.Sprintf("tools=%d", len(fp.Tools)))
	}
	if len(fp.Configs) > 0 {
		parts = append(parts, fmt.Sprintf("configs=%d", len(fp.Configs)))
	}
	if toolSummary := readRunToolFingerprintSummary(fp.Tools, 4); toolSummary != "" {
		parts = append(parts, "tool_hashes="+toolSummary)
	}
	if configSummary := readRunConfigFingerprintSummary(fp.Configs, 3); configSummary != "" {
		parts = append(parts, "config_hashes="+configSummary)
	}
	return strings.Join(parts, " ")
}

func readRunToolFingerprintSummary(items []types.ReadRunToolFingerprint, limit int) string {
	items = types.NormalizeReadRunToolFingerprints(items)
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	var parts []string
	for _, item := range items {
		if len(parts) >= limit {
			break
		}
		if item.Name == "" {
			continue
		}
		if !item.Available {
			reason := strings.TrimSpace(item.ReasonCode)
			if reason == "" {
				reason = "unavailable"
			}
			parts = append(parts, fmt.Sprintf("%s=unavailable(%s)", item.Name, reason))
			continue
		}
		hash := readRunShortHash(item.VersionHash)
		if hash == "" {
			hash = readRunShortHash(item.FileHash)
		}
		parts = append(parts, fmt.Sprintf("%s=%s", item.Name, hash))
	}
	return strings.Join(parts, ",")
}

func readRunConfigFingerprintSummary(items []types.ReadRunConfigFingerprint, limit int) string {
	items = types.NormalizeReadRunConfigFingerprints(items)
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	var parts []string
	for _, item := range items {
		if len(parts) >= limit {
			break
		}
		if item.Name == "" {
			continue
		}
		if !item.Available {
			reason := strings.TrimSpace(item.ReasonCode)
			if reason == "" {
				reason = "unavailable"
			}
			parts = append(parts, fmt.Sprintf("%s=unavailable(%s)", item.Name, reason))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", item.Name, readRunShortHash(item.Hash)))
	}
	return strings.Join(parts, ",")
}

func readRunShortHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func readRunCompact(text string, max int) string {
	text = oneLine(strings.TrimSpace(text))
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}
