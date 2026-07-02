package tracequery

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Deterministic parser for structured lock-contention trace-mark payloads
// (§7.30.3 D1). ART / OHOS runtimes emit fixed print formats when a thread
// blocks on a monitor or a runtime-internal lock:
//
//	monitor contention with owner <owner> (<tid>) at <sig>(<file:line>) waiters=<n> blocking from <sig>(<file:line>)
//	Lock contention on a monitor lock (owner tid: <tid>)
//	Lock contention on the thread list lock (owner tid: <tid>)
//	Lock contention on the InternTable lock (owner tid: <tid>)
//
// The <owner> field may itself be a "#Name -->#Name2" hand-off chain; the
// FINAL element is the current holder. These are fixed runtime-emitted payload
// shapes: parsing them is structured ftrace-payload parsing, never
// prose/user-intent keyword matching.

const (
	blockingKindMonitorContention = "monitor_contention"
	blockingKindLockContention    = "lock_contention"
)

const (
	lockContentionMonitorOwnerPrefix = "monitor contention with owner "
	lockContentionLockPrefix         = "Lock contention on "
)

var (
	lockContentionOwnerTidRe = regexp.MustCompile(`\(owner tid: ([0-9]+)\)`)
	lockContentionTrailTidRe = regexp.MustCompile(`\(([0-9]+)\)$`)
	lockContentionWaitersRe  = regexp.MustCompile(` waiters=([0-9]+)`)
)

type lockContentionInfo struct {
	Kind       string
	Owner      ThreadRef
	Waiters    int
	HolderSite string
}

// parseLockContentionPayload deterministically parses one trace-mark span
// name. ok is true whenever the payload matches one of the fixed contention
// formats — even when no owner could be extracted, so callers keep the typed
// blocking semantics and fall back to an ownerless contention row.
func parseLockContentionPayload(name string) (lockContentionInfo, bool) {
	name = strings.TrimSpace(name)
	switch {
	case strings.HasPrefix(name, lockContentionMonitorOwnerPrefix):
		return parseMonitorContentionOwnerPayload(strings.TrimPrefix(name, lockContentionMonitorOwnerPrefix)), true
	case strings.HasPrefix(name, lockContentionLockPrefix):
		return parseLockContentionOnPayload(strings.TrimPrefix(name, lockContentionLockPrefix)), true
	default:
		return lockContentionInfo{}, false
	}
}

// parseMonitorContentionOwnerPayload handles the ART "monitor contention with
// owner …" body: owner segment (optionally a "#A -->#B" hand-off chain with a
// trailing "(tid)"), an optional " at <sig>(<file:line>)" holder site, an
// optional " waiters=<n>" count, and an optional " blocking from …" suffix.
func parseMonitorContentionOwnerPayload(body string) lockContentionInfo {
	info := lockContentionInfo{Kind: blockingKindMonitorContention}
	ownerPart := body
	sitePart := ""
	if at := strings.Index(body, " at "); at >= 0 {
		ownerPart, sitePart = body[:at], body[at+len(" at "):]
	}
	if m := lockContentionWaitersRe.FindStringSubmatch(body); m != nil {
		info.Waiters, _ = strconv.Atoi(m[1])
	}
	owner := strings.TrimSpace(ownerPart)
	if loc := lockContentionTrailTidRe.FindStringSubmatchIndex(owner); loc != nil {
		info.Owner.PID, _ = strconv.Atoi(owner[loc[2]:loc[3]])
		owner = strings.TrimSpace(owner[:loc[0]])
	}
	// A "#A -->#B" hand-off chain names the FINAL holder last (§7.30.3 D1).
	if idx := strings.LastIndex(owner, "-->"); idx >= 0 {
		owner = strings.TrimSpace(owner[idx+len("-->"):])
	}
	info.Owner.Comm = strings.TrimSpace(strings.TrimPrefix(owner, "#"))
	if sitePart != "" {
		site := sitePart
		if cut := strings.Index(site, " waiters="); cut >= 0 {
			site = site[:cut]
		}
		if cut := strings.Index(site, " blocking from "); cut >= 0 {
			site = site[:cut]
		}
		info.HolderSite = strings.TrimSpace(site)
	}
	return info
}

// parseLockContentionOnPayload handles the ART "Lock contention on <lock>
// (owner tid: N)" family: a monitor lock keeps the monitor-contention kind;
// runtime-internal locks (thread list lock, InternTable lock, …) report the
// generic lock-contention kind. The owner tid may be absent.
func parseLockContentionOnPayload(body string) lockContentionInfo {
	info := lockContentionInfo{Kind: blockingKindLockContention}
	subject := body
	if loc := lockContentionOwnerTidRe.FindStringSubmatchIndex(body); loc != nil {
		info.Owner.PID, _ = strconv.Atoi(body[loc[2]:loc[3]])
		subject = body[:loc[0]]
	}
	if strings.TrimSpace(strings.ToLower(subject)) == "a monitor lock" {
		info.Kind = blockingKindMonitorContention
	}
	return info
}

// lockContentionSummarySuffix appends the typed contention semantics to a
// blocking-span summary: blocking kind, owner thread, holder site, waiters.
// Only fields the payload actually carried are emitted.
func lockContentionSummarySuffix(info lockContentionInfo) string {
	parts := []string{"blocking_kind=" + info.Kind}
	if info.Owner.Comm != "" || info.Owner.PID > 0 {
		parts = append(parts, "owner="+threadLabel(info.Owner))
	}
	if info.HolderSite != "" {
		parts = append(parts, "holder_site="+info.HolderSite)
	}
	if info.Waiters > 0 {
		parts = append(parts, fmt.Sprintf("waiters=%d", info.Waiters))
	}
	return " " + strings.Join(parts, " ")
}
