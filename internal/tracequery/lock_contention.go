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
	// lockContentionOwnerVocabRe is the LOCKNS-FIX 件3 unknown-morphology SOFT
	// screen (§29.104.12, 2026-07-16): a span name that speaks lock-owner
	// vocabulary (word-boundary `owner`) yet matches NO registered contention
	// morphology. NOISY signal by design — it drives ONLY the soft
	// "owner unresolved (morphology unregistered)" disclosure on the
	// payload-less blocking lane (§1 red line: 嘈声信号只作软引导); it never
	// admits a span, never mints a holder, never changes the row value (the
	// value rides the XERR1-FIX basis discipline unchanged — fail-open).
	lockContentionOwnerVocabRe = regexp.MustCompile(`(?i)\bowner\b`)
	// lockContentionOwnerTidKeyRe is the BLIND-2 GENERALIZED owner-tid key form
	// (§29.2 / §29.7-1 ruling, real_trace_campaign_20260705.md, 2026-07-09):
	// the literal `owner tid` key + a `:` or `=` separator (spaces tolerated) +
	// an integer, ANCHORED ON THE KEY ITSELF rather than any parenthesis or
	// prefix wording. Rationale (裁定原文): the owner-tid key is the carrying
	// signal — a keyed precise form, the §28.4 cpu_id keyed-rail analogy —
	// while the prefix text ("Lock contention on …", vendor variants) is
	// runtime free vocabulary; anchoring on the key covers the ART census form
	// and future vendor spellings without enumerating prefixes per flavor.
	lockContentionOwnerTidKeyRe = regexp.MustCompile(`\bowner tid\s*[:=]\s*([0-9]+)`)
)

type lockContentionInfo struct {
	Kind       string
	Owner      ThreadRef
	Waiters    int
	HolderSite string
	// Morphology names the registry row that parsed this payload (LOCKNS-FIX
	// 件3, §29.104.12): typed dispatch witness — the carve reads the row's
	// registered confidence through it. Stamped by parseLockContentionPayload.
	Morphology string
	// BlockingFromSite (BLOCKFROM, §27.4 G13 配套 / §28.1 收口批准, 2026-07-09):
	// the WAITER's own blocking call site — the "blocking from <sig>(<file:line>)"
	// tail segment of the ART monitor-contention payload (the method the blocked
	// thread was executing when it hit the lock). Counterpart of HolderSite
	// (the HOLDER's location); parsed verbatim with the same boundary strategy.
	// Empty when the payload carried no such segment — absence never invents a
	// wait point (the opendir_78 G13 witness: prose fabricated an
	// "enqueueMessage 消息队列锁" wait point while the span payload said
	// AssetManager.getResourceValue(AssetManager.java:761)).
	BlockingFromSite string
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

// lockContentionMorphology is ONE registered contention payload shape
// (LOCKNS-FIX 件3 morphology registry, §29.104.12, 2026-07-16): a typed
// (name, parser, confidence) row. Adding a new runtime print shape means
// adding a ROW, never a new code path — the registry is the single dispatch
// for every contention-payload consumer (parseLockContentionPayload).
type lockContentionMorphology struct {
	// Name is the typed morphology identifier (diagnostics / pins).
	Name string
	// Parse attempts this morphology on the trimmed span name; ok=false means
	// the shape did not match and the next registry row is consulted.
	Parse func(name string) (lockContentionInfo, bool)
	// Confidence is the payload-direct evidence grade a candidate parsed by
	// this morphology mints at the carve. All registered forms today ride the
	// structured-payload 0.72 grade (byte-identical to the pre-registry flat
	// value); a future weaker form declares its own grade here instead of
	// forking the carve.
	Confidence float64
}

// lockContentionMorphologyRegistry — registry order IS the legacy dispatch
// precedence (三臂平移, migration pinned byte-identical):
//  1. 形A prefix-anchored "monitor contention with owner …" (rich grammar:
//     holder_site / waiters / blocking-from / '-->' hand-off; 信息更全,先匹配).
//  2. 形B prefix-anchored "Lock contention on …" (subject + owner-tid key;
//     ownerless admission — that spelling is contention semantics even
//     without the key).
//  3. 形A embedded (LOCKNS-FIX 件2, §29.104.12 G2, 2026-07-16): the SAME rich
//     grammar located at a WORD-BOUNDARY inside a vendor-prefixed name
//     ("vendor_xyz: monitor contention with owner …"). Strictly additive —
//     names the prefix arms already match never reach this row — and
//     boundary-guarded ("premonitor contention …" never matches; 严禁自由
//     子串误伤, the XERR1 件4 word-boundary discipline).
//  4. BLIND-2 generalized owner-tid keyed arm (`owner tid[:=]<N>` anywhere;
//     §29.7-1 ruling — the key is the carrying signal, prefixes are free
//     vocabulary).
var lockContentionMorphologyRegistry = []lockContentionMorphology{
	{Name: "monitor_contention_owner", Confidence: 0.72, Parse: parseMonitorContentionOwnerPrefixed},
	{Name: "lock_contention_on", Confidence: 0.72, Parse: parseLockContentionOnPrefixed},
	{Name: "monitor_contention_owner_embedded", Confidence: 0.72, Parse: parseMonitorContentionOwnerEmbedded},
	{Name: "owner_tid_keyed", Confidence: 0.72, Parse: parseOwnerTidKeyedPayload},
}

// parseLockContentionPayload deterministically parses one trace-mark span
// name. ok is true whenever the payload matches one of the REGISTERED
// contention morphologies — even when no owner could be extracted, so callers
// keep the typed blocking semantics and fall back to an ownerless contention
// row. A name matching NO registry row is NOT a contention payload: it
// fail-opens to the payload-less blocking lane (no holder attribution is ever
// minted from an unregistered shape — LOCKNS-FIX 件3; the owner-vocabulary
// soft screen spanNameCarriesOwnerVocabulary words the disclosure).
func parseLockContentionPayload(name string) (lockContentionInfo, bool) {
	name = strings.TrimSpace(name)
	for _, m := range lockContentionMorphologyRegistry {
		if info, ok := m.Parse(name); ok {
			info.Morphology = m.Name
			return info, true
		}
	}
	return lockContentionInfo{}, false
}

// lockContentionMorphologyConfidence returns the registered payload-direct
// grade of a parsed morphology (0 = unregistered name, caller keeps its own
// base — fail-open).
func lockContentionMorphologyConfidence(morphology string) float64 {
	for _, m := range lockContentionMorphologyRegistry {
		if m.Name == morphology {
			return m.Confidence
		}
	}
	return 0
}

// parseMonitorContentionOwnerPrefixed is registry row 1: the prefix-anchored
// ART rich form (byte-identical to the pre-registry switch arm).
func parseMonitorContentionOwnerPrefixed(name string) (lockContentionInfo, bool) {
	if !strings.HasPrefix(name, lockContentionMonitorOwnerPrefix) {
		return lockContentionInfo{}, false
	}
	return parseMonitorContentionOwnerPayload(strings.TrimPrefix(name, lockContentionMonitorOwnerPrefix)), true
}

// parseLockContentionOnPrefixed is registry row 2: the prefix-anchored
// "Lock contention on …" family (byte-identical to the pre-registry arm).
func parseLockContentionOnPrefixed(name string) (lockContentionInfo, bool) {
	if !strings.HasPrefix(name, lockContentionLockPrefix) {
		return lockContentionInfo{}, false
	}
	return parseLockContentionOnPayload(strings.TrimPrefix(name, lockContentionLockPrefix)), true
}

// parseMonitorContentionOwnerEmbedded (LOCKNS-FIX 件2, §29.104.12 G2) locates
// the ART monitor-contention grammar INSIDE a vendor-prefixed span name and
// parses the body from the grammar on with the SAME parser as the prefix arm
// (one grammar, one parser — the vendor prefix is free vocabulary). Word
// boundary is mandatory: the grammar must start the name (registry row 1's
// case — unreachable here, kept for parser equivalence) or sit right after a
// non-alphanumeric byte, so "premonitor contention with owner …" never
// matches (free-substring hits forbidden — XERR1 件4 word-boundary
// discipline).
func parseMonitorContentionOwnerEmbedded(name string) (lockContentionInfo, bool) {
	for from := 0; ; {
		i := strings.Index(name[from:], lockContentionMonitorOwnerPrefix)
		if i < 0 {
			return lockContentionInfo{}, false
		}
		pos := from + i
		if pos == 0 || !isASCIIAlphanumeric(name[pos-1]) {
			return parseMonitorContentionOwnerPayload(name[pos+len(lockContentionMonitorOwnerPrefix):]), true
		}
		from = pos + 1
	}
}

// spanNameCarriesOwnerVocabulary reports whether a span name speaks
// lock-owner vocabulary (word-boundary `owner`, case-insensitive) — the
// LOCKNS-FIX 件3 unknown-morphology screen, consulted ONLY after every
// registered morphology missed. NOISY signal → SOFT disclosure only:
// "owner 未解析(形态未注册)" rides the payload-less row as a note; nothing
// is gated, no holder is minted, the value keeps the XERR1-FIX basis
// discipline (fail-open).
func spanNameCarriesOwnerVocabulary(name string) bool {
	return lockContentionOwnerVocabRe.MatchString(name)
}

// parseOwnerTidKeyedPayload is the BLIND-2 generalized arm: a span name
// carrying the `owner tid[:=]<N>` key form (census_report_a ⑦: 5600+
// "Lock contention on InternTable lock (owner tid: N)" ART rows plus future
// vendor prefixes) mints a lock-contention-family candidate. The owner tid is
// payload-direct (same sentinel discipline as the ART arms: 0 / uint64(-1) →
// typed ownerless, never a Peer id — census witness: 469 tid-0 rows); the
// FULL verbatim span name is preserved as the wait-object / holder-point
// description (专名如 InternTable 保英文 — the payload is never translated);
// holder_site / blocking_from / waiters stay empty — this form carries no
// such segments and absence never invents one.
func parseOwnerTidKeyedPayload(name string) (lockContentionInfo, bool) {
	loc := lockContentionOwnerTidKeyRe.FindStringSubmatchIndex(name)
	if loc == nil {
		return lockContentionInfo{}, false
	}
	info := lockContentionInfo{Kind: blockingKindLockContention}
	if v, err := strconv.ParseUint(name[loc[2]:loc[3]], 10, 64); err == nil {
		if ownerTidIsSentinel(v) {
			info.OwnerAbsent = true
		} else if v <= math.MaxInt64 {
			info.Owner.PID = int(v)
		} else {
			info.OwnerAbsent = true
		}
	}
	info.WaitObject = name
	return info, true
}

// spanNameCarriesOwnerTidKey reports whether a span name carries the BLIND-2
// generalized owner-tid key form — the admission-side twin of
// parseOwnerTidKeyedPayload (the key IS the carrying signal, so a vendor
// prefix without any blocking-vocabulary token still admits).
func spanNameCarriesOwnerTidKey(name string) bool {
	return lockContentionOwnerTidKeyRe.MatchString(name)
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
	// BLOCKFROM (§27.4 G13): the "blocking from <sig>(<file:line>)" tail names
	// the WAITER's own blocking call site. Same boundary strategy as the holder
	// site: the signature may contain parentheses/commas, so the value runs
	// verbatim to the end of the payload, cut only at a (tolerated, not
	// observed) trailing " waiters=" segment. No segment → field stays empty.
	if cut := strings.Index(body, " blocking from "); cut >= 0 {
		from := body[cut+len(" blocking from "):]
		if w := strings.Index(from, " waiters="); w >= 0 {
			from = from[:w]
		}
		info.BlockingFromSite = strings.TrimSpace(from)
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
	loc := lockContentionOwnerTidRe.FindStringSubmatchIndex(body)
	if loc == nil {
		// BLIND-2 (§29.7-1): the strict "(owner tid: N)" paren form missed —
		// consult the generalized key form (`owner tid[:=]N`, spaces
		// tolerated) so a separator/spacing variant of this known spelling
		// keeps its payload-direct owner instead of degrading to ownerless.
		// Strictly additive: payloads without the key stay byte-identical.
		if keyed := lockContentionOwnerTidKeyRe.FindStringSubmatchIndex(body); keyed != nil {
			loc = keyed
		}
	}
	if loc != nil {
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
	// The keyed fallback anchors INSIDE the parenthetical, so a wrapping "("
	// may remain on the subject tail; the strict paren form never leaves one
	// (its match starts at the "(") — the trim is a no-op there.
	info.WaitObject = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(subject), "("))
	// G12-ENG 复核 P3-1: the monitor reclassification reads the TRIMMED
	// subject — the keyed fallback anchors inside the parenthetical and used
	// to leave the wrapping "(" on the raw subject, so
	// "a monitor lock (owner tid= 5)" silently lost its monitor kind.
	// Strict-form behavior is byte-identical (its subject never carries the
	// paren, and WaitObject is exactly the old trimmed comparand).
	if strings.ToLower(info.WaitObject) == "a monitor lock" {
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
	if info.BlockingFromSite != "" {
		parts = append(parts, "blocking_from_site="+info.BlockingFromSite)
	}
	if info.Waiters > 0 {
		parts = append(parts, fmt.Sprintf("waiters=%d", info.Waiters))
	}
	return " " + strings.Join(parts, " ")
}
