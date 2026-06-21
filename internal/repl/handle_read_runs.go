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
	case rest == "clear" || strings.HasPrefix(rest, "clear "):
		id := readRunCommandID(rest, "clear")
		if id == "" {
			r.info("/read-runs clear <run-id>")
			return
		}
		r.handleReadRunsClear(id)
	default:
		r.info("/read-runs <list|show <run-id>|clear <run-id>>")
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
	r.renderBordered(readRunSnapshotMarkdown(*snapshot))
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
	if snapshot.SourceInventory.IsActive() {
		fmt.Fprintf(&b, "- Source inventory: %s\n", readRunSourceInventorySummary(snapshot.SourceInventory))
	}
	b.WriteString("\nAdvanced: `/read-runs clear ")
	b.WriteString(snapshot.RunID)
	b.WriteString("`\n")
	return b.String()
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
