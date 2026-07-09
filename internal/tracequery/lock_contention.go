package tracequery

import (
	"fmt"
	"math"
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
	// WaitObject is the lock-object name the "Lock contention on <subj> (owner
	// tid: N)" family prints as its subject (e.g. "thread suspend count lock",
	// "the InternTable lock"). It was previously discarded (§19 清点②); it is the
	// only description an ownerless contention row can carry, so it is preserved
	// verbatim and published as the row's wait_object.
	WaitObject string
	// OwnerHandoff is the verbatim "#A -->#B" hand-off chain elements (leading
	// '#' trimmed, order preserved) when the owner segment recorded MORE THAN
	// ONE holder — the runtime observed the lock CHANGE HANDS during the wait
	// (P0-E 锁车道修2, ledger §24.9-C F2, 2026-07-09). The FINAL element is
	// the current/last holder (unchanged parse below); the chain itself is a
	// PRECISE payload signal that whole-span single-holder attribution is
	// wrong, so it must reach the typed row instead of being discarded. nil
	// when the payload named a single owner.
	OwnerHandoff []string
	// OwnerAbsent is set when the payload printed an EXPLICIT ownerless sentinel
	// in the owner-tid slot — ART emits `owner tid: 0` (no-holder sentinel, §19
	// 语料 23/84) and `owner tid: 18446744073709551615` (uint64(-1) sentinel,
	// 7/84). These are NOT real thread ids: they must never become a Peer.PID or
	// an OwnerTidRaw audit number (the old strconv.Atoi silently clamped the
	// uint64-max form to MaxInt64 and printed the garbage 9223372036854775807).
	// A typed ownerless row keeps its BlockingKind + WaitObject and routes to the
	// payload-less wakeup-edge fallback, exactly like a span that carried no owner
	// slot at all.
	OwnerAbsent bool
}

// ownerlessTidSentinels are the two EXPLICIT no-holder values ART prints in the
// "owner tid: N" slot: 0 (unowned) and math.MaxUint64 (uint64(-1)). Parsed with
// ParseUint so the 20-digit uint64-max form is recognised exactly instead of
// being clamped by a signed Atoi.
func ownerTidIsSentinel(v uint64) bool {
	return v == 0 || v == math.MaxUint64
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
		// Same sentinel-safe parse as the "owner tid: N" form (pin④): never let a
		// uint64(-1) trailing tid clamp to MaxInt64.
		if v, err := strconv.ParseUint(owner[loc[2]:loc[3]], 10, 64); err == nil {
			if ownerTidIsSentinel(v) {
				info.OwnerAbsent = true
			} else if v <= math.MaxInt64 {
				info.Owner.PID = int(v)
			} else {
				info.OwnerAbsent = true
			}
		}
		owner = strings.TrimSpace(owner[:loc[0]])
	}
	// A "#A -->#B" hand-off chain names the FINAL holder last (§7.30.3 D1).
	// P0-E 锁车道修2 (§24.9-C F2): the chain itself is preserved as a typed
	// hand-over witness — it proves the holder changed during the wait.
	if strings.Contains(owner, "-->") {
		for _, part := range strings.Split(owner, "-->") {
			part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "#"))
			if part != "" {
				info.OwnerHandoff = append(info.OwnerHandoff, part)
			}
		}
		if len(info.OwnerHandoff) < 2 {
			info.OwnerHandoff = nil // degenerate split: no hand-over observed
		}
	}
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
		// ParseUint (not Atoi): the uint64(-1) sentinel is a 20-digit value that a
		// signed Atoi silently clamps to MaxInt64, producing the bogus
		// 9223372036854775807 "owner". 0 and math.MaxUint64 are EXPLICIT no-holder
		// sentinels → typed ownerless, never a Peer id (§19 S1/pin④).
		if v, err := strconv.ParseUint(body[loc[2]:loc[3]], 10, 64); err == nil {
			if ownerTidIsSentinel(v) {
				info.OwnerAbsent = true
			} else if v <= math.MaxInt64 {
				info.Owner.PID = int(v)
			} else {
				// A non-sentinel value that still overflows int (should not occur
				// for real tids) is treated as ownerless rather than a garbage id.
				info.OwnerAbsent = true
			}
		}
		subject = body[:loc[0]]
	}
	info.WaitObject = strings.TrimSpace(subject)
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
