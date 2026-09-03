package types

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
)

// TraceTargetStateScopeAuthority is the wording boundary carried by one
// compiled target_window_states account. Every duration is scoped to the
// target thread's own wall-clock state partition; none is a CPU-wide
// utilization or saturation measurement.
type TraceTargetStateScopeAuthority struct {
	ArtifactLabel  string
	Subject        string
	WindowStartTs  float64
	WindowEndTs    float64
	WindowMS       float64
	RunningMS      float64
	RunnableMS     float64
	SleepMS        float64
	DStateMS       float64
	IOWaitMS       float64
	SleepIOWaitMS  float64
	TotalMS        float64
	UnaccountedMS  float64
	CoverageStatus string
	HeadCarryMS    float64
	HeadCarryState string
	TailOpenMS     float64
	TailOpenState  string
	EvidenceID     string
}

const traceTargetStateCoverageToleranceMS = 0.002

// TraceUninterruptibleWaitMS is the ONE place that says which engine lanes
// compose the published "uninterruptible wait" (不可中断等待 / D-state) figure:
// the non-IO D lane plus the scheduler-marked IO-wait lane. The two ledgers
// are mutually exclusive (tracequery DStateTop/IOWaitTop), so their same-
// thread sum is the complete D/IO blocking account; the sleep-side IO-wait
// overlay is NOT part of it (it lives inside SleepMS). V3-1 (colleague_merge_
// audit §40.20): every prompt face, the customer four-state line and the
// answer-side wall-clock audit fold through this function — a hand-written
// `DStateMS + IOWaitMS` outside internal/types is a census offender
// (internal/agent/target_state_account_render_census_test.go).
func TraceUninterruptibleWaitMS(dStateMS, ioWaitMS float64) float64 {
	return dStateMS + ioWaitMS
}

// UninterruptibleWaitMS returns the published uninterruptible-wait fold of
// this authority (see TraceUninterruptibleWaitMS).
func (a TraceTargetStateScopeAuthority) UninterruptibleWaitMS() float64 {
	return TraceUninterruptibleWaitMS(a.DStateMS, a.IOWaitMS)
}

// SchedulerMarkedWaitMS is the narrow scheduler-marked wait classifier's
// wall clock: the uninterruptible fold plus the interruptible sleep that
// carries an IO-wait marker. It is the "narrow finding" zero test and the
// value the complete wait-occurrence list pairs against; it is NOT a
// partition lane (SleepIOWaitMS is already inside SleepMS).
func (a TraceTargetStateScopeAuthority) SchedulerMarkedWaitMS() float64 {
	return a.UninterruptibleWaitMS() + a.SleepIOWaitMS
}

// FormatTargetStateAccount renders one target-state authority as ONE
// reader-facing sentence (no bullet, no newline), deterministic from the
// authority alone. It is the single prompt-face formatter of the account
// (V3-1): the observation-ledger scope section, the final reader decision
// card and the bounded-runtime reader handoff all print exactly this string,
// so the model can never read two calibers for the same account in one
// prompt. Shape (zh; en mirrors it word for word):
//
//	[工件 L；]目标线程 S；窗口 a–b 秒；运行 R 毫秒，可运行但尚未获调度 Q 毫秒，
//	可中断睡眠 P 毫秒[（其中带 IO 等待标记的可中断睡眠 X 毫秒，已含在睡眠内）]，
//	不可中断等待 U 毫秒（其中调度器标记的 IO 等待 I 毫秒）；合计 T 毫秒；
//	<coverage word>；未归账 N 毫秒。
//
// U is the UninterruptibleWaitMS fold and its 其中 clause is always printed
// (the IO share is a disclosure, never an addend); the sleep-side clause
// prints only when the overlay is non-zero; the artifact clause prints only
// when the label is known; the unaccounted remainder is always printed
// (0.000 is an honest complete-coverage statement). Lane words come from
// tracefence Table ⑧; wire lane keys never appear.
func FormatTargetStateAccount(a TraceTargetStateScopeAuthority, lang string) string {
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	word := func(lane string) string {
		w, _ := tracefence.StateLaneWord(lane, zh)
		return w
	}
	coverage := tracefence.StateCoverageWord(a.CoverageStatus, zh)
	label := strings.TrimSpace(a.ArtifactLabel)
	var b strings.Builder
	if zh {
		if label != "" {
			fmt.Fprintf(&b, "工件 %s；", label)
		}
		fmt.Fprintf(&b, "目标线程 %s；窗口 %.6f–%.6f 秒；", a.Subject, a.WindowStartTs, a.WindowEndTs)
		fmt.Fprintf(&b, "%s %.3f 毫秒，%s %.3f 毫秒，%s %.3f 毫秒",
			word(tracefence.StateLaneRunning), a.RunningMS,
			word(tracefence.StateLaneRunnable), a.RunnableMS,
			word(tracefence.StateLaneSleep), a.SleepMS)
		if a.SleepIOWaitMS > 0 {
			fmt.Fprintf(&b, "（其中%s %.3f 毫秒，已含在睡眠内）", word(tracefence.StateLaneSleepIOWait), a.SleepIOWaitMS)
		}
		fmt.Fprintf(&b, "，%s %.3f 毫秒（其中%s %s %.3f 毫秒）；合计 %.3f 毫秒；%s；未归账 %.3f 毫秒。",
			word(tracefence.StateLaneDState), a.UninterruptibleWaitMS(),
			tracefence.StateSchedulerMarkedQualifierZH, word(tracefence.StateLaneIOWait), a.IOWaitMS,
			a.TotalMS, coverage, a.UnaccountedMS)
		return b.String()
	}
	if label != "" {
		fmt.Fprintf(&b, "Artifact %s; ", label)
	}
	fmt.Fprintf(&b, "target thread %s; window %.6f–%.6f seconds; ", a.Subject, a.WindowStartTs, a.WindowEndTs)
	fmt.Fprintf(&b, "%s %.3f ms, %s %.3f ms, %s %.3f ms",
		word(tracefence.StateLaneRunning), a.RunningMS,
		word(tracefence.StateLaneRunnable), a.RunnableMS,
		word(tracefence.StateLaneSleep), a.SleepMS)
	if a.SleepIOWaitMS > 0 {
		fmt.Fprintf(&b, " (including %.3f ms of %s, already inside the sleep term)", a.SleepIOWaitMS, word(tracefence.StateLaneSleepIOWait))
	}
	fmt.Fprintf(&b, ", %s %.3f ms (including %.3f ms of %s %s); total %.3f ms; %s; %.3f ms unaccounted.",
		word(tracefence.StateLaneDState), a.UninterruptibleWaitMS(),
		a.IOWaitMS, tracefence.StateSchedulerMarkedQualifierEN, word(tracefence.StateLaneIOWait),
		a.TotalMS, coverage, a.UnaccountedMS)
	return b.String()
}

// FormatTargetStateAccountCaliber is the one-sentence definition that
// accompanies FormatTargetStateAccount on a prompt face: what the published
// uninterruptible figure folds, what it does not add, and what its zero does
// not prove. One source so the definition can never drift from the fold.
func FormatTargetStateAccountCaliber(lang string) string {
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	if zh {
		return "不可中断等待采用调度器标记的窄口径：不可中断等待 = 非 IO 的 D 状态 + 调度器标记的 IO 等待；睡眠内的 IO 等待标记已含在可中断睡眠中，不再相加。零值只表示没有匹配该窄口径，不能证明磁盘、文件系统、设备或其他 IO 活动/阻塞不存在。由 IO 发起到完成闭合的目标线程等待是另一把尺；若未单独发布，应表述为未评估而不是零。"
	}
	return "Uninterruptible wait here uses the narrow scheduler-marked definition: uninterruptible wait = non-IO D state + scheduler-marked IO wait; an IO-wait marker inside interruptible sleep stays inside the sleep term and is never added again. Zero means no match for that narrow definition; it does not prove the absence of disk, filesystem, device, or other IO activity/blocking. Target blocking closed by IO issue-to-completion evidence is a separate ruler; when it is not separately published, it is unassessed rather than zero."
}

// BuildTraceTargetStateScopeAuthorities compiles the target-thread scope
// authorities from the already-selected projection accounts. It deliberately
// consumes the compiled projection rather than all raw target-state records so
// explicit-window election and supplemental-window separation remain owned by
// the existing projection compiler.
func BuildTraceTargetStateScopeAuthorities(set TraceCausalProjectionSet) []TraceTargetStateScopeAuthority {
	out := make([]TraceTargetStateScopeAuthority, 0, len(set.Projections))
	seen := map[string]bool{}
	for _, projection := range set.Projections {
		account := projection.TargetStateAccount
		if account == nil || strings.TrimSpace(account.Subject) == "" || account.TotalMS <= 0 {
			continue
		}
		windowMS := 0.0
		if account.WindowEndTs > account.WindowStartTs {
			windowMS = (account.WindowEndTs - account.WindowStartTs) * 1000
		}
		// An account above its own selected window is impossible and cannot
		// become answer authority. Allow only µs-level representation drift.
		if windowMS > 0 && account.TotalMS > windowMS+traceTargetStateCoverageToleranceMS {
			continue
		}
		coverageStatus := "window_unknown"
		unaccountedMS := 0.0
		if windowMS > 0 {
			coverageStatus = "complete"
			if windowMS-account.TotalMS > traceTargetStateCoverageToleranceMS {
				coverageStatus = "partial_unaccounted"
				unaccountedMS = windowMS - account.TotalMS
			}
		}
		key := strings.Join([]string{
			strings.TrimSpace(projection.ArtifactPath),
			strings.TrimSpace(account.Subject),
			strings.TrimSpace(account.EvidenceID),
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, TraceTargetStateScopeAuthority{
			ArtifactLabel:  strings.TrimSpace(projection.ArtifactLabel),
			Subject:        strings.TrimSpace(account.Subject),
			WindowStartTs:  account.WindowStartTs,
			WindowEndTs:    account.WindowEndTs,
			WindowMS:       windowMS,
			RunningMS:      account.RunningMS,
			RunnableMS:     account.RunnableMS,
			SleepMS:        account.SleepMS,
			DStateMS:       account.DStateMS,
			IOWaitMS:       account.IOWaitMS,
			SleepIOWaitMS:  account.SleepIOWaitMS,
			TotalMS:        account.TotalMS,
			UnaccountedMS:  unaccountedMS,
			CoverageStatus: coverageStatus,
			HeadCarryMS:    account.HeadCarryMS,
			HeadCarryState: strings.TrimSpace(account.HeadCarryState),
			TailOpenMS:     account.TailOpenMS,
			TailOpenState:  strings.TrimSpace(account.TailOpenState),
			EvidenceID:     strings.TrimSpace(account.EvidenceID),
		})
	}
	return out
}

// BuildTraceTargetStateScopeAuthoritiesFromLedger preserves the finite
// explicit-window lane when no causal projection anchor is expected. The
// ordinary projection-derived authorities remain authoritative and return
// first. The fallback admits only a hard trace_query target_window_states row
// whose producer-owned selected window matches the analyzer-validated user
// window and whose subject matches a typed user target. It therefore exposes
// an already-observed state partition without manufacturing a causal
// projection, guessing a target, or borrowing an exploration window.
func BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger ObservationLedger) []TraceTargetStateScopeAuthority {
	set := CompileTraceCausalProjectionSet(ledger)
	if authorities := BuildTraceTargetStateScopeAuthorities(set); len(authorities) > 0 {
		return authorities
	}
	requestedStart, requestedEnd, ok := ledger.RuntimeArtifactScopeProfile.ExplicitTimeWindow()
	if !ok {
		return nil
	}
	typedTargets := make([]traceCausalProjectionAnchorEntity, 0, len(ledger.AnchorUserEntities))
	for _, entity := range traceCausalProjectionAnchorEntitiesFromLedger(ledger.AnchorUserEntities) {
		if entity.typedLane {
			typedTargets = append(typedTargets, entity)
		}
	}
	if len(typedTargets) == 0 {
		return nil
	}

	type selectedAccount struct {
		account TraceCausalProjectionTargetStateAccount
		path    string
		label   string
	}
	selected := map[string]selectedAccount{}
	order := make([]string, 0)
	for _, record := range ledger.Records {
		if !traceCausalProjectionTraceQueryRecord(record) ||
			strings.TrimSpace(record.Predicate) != "target_window_states" {
			continue
		}
		candidate, ok := traceCausalProjectionTargetStateCandidateFromRecord(record)
		if !ok || !TraceCausalProjectionPrincipalValueSameWindow(
			candidate.WindowStart, candidate.WindowEnd, requestedStart, requestedEnd,
		) {
			continue
		}
		matchedTarget := false
		for _, target := range typedTargets {
			if traceCausalProjectionAnchorLabelMatchesEntity(candidate.Account.Subject, target) {
				matchedTarget = true
				break
			}
		}
		if !matchedTarget {
			continue
		}
		artifactKey, label, path := traceCausalProjectionArtifactIdentity(record)
		if artifactKey == "" {
			continue
		}
		key := artifactKey + "\x00" + traceCausalProjectionCanonicalNode(candidate.Account.Subject)
		previous, exists := selected[key]
		if exists && previous.account.TotalMS >= candidate.Account.TotalMS {
			continue
		}
		if !exists {
			order = append(order, key)
		}
		selected[key] = selectedAccount{account: candidate.Account, path: path, label: label}
	}
	if len(selected) == 0 {
		return nil
	}
	projections := make([]TraceCausalProjection, 0, len(selected))
	for _, key := range order {
		item := selected[key]
		account := item.account
		projections = append(projections, TraceCausalProjection{
			ArtifactPath:       item.path,
			ArtifactLabel:      item.label,
			TargetStateAccount: &account,
		})
	}
	return BuildTraceTargetStateScopeAuthorities(TraceCausalProjectionSet{Projections: projections})
}

// TraceTargetWaitSummaryAuthority is the complete occurrence-level companion
// to TraceTargetStateScopeAuthority. It is compiled only when one deterministic
// trace_query result carries a complete aggregate record plus exactly the
// declared number of same-result occurrence rows. This avoids the prompt's
// bounded eight-row projection without parsing model prose or rebuilding
// scheduler intervals from neighboring events.
type TraceTargetWaitSummaryAuthority struct {
	ArtifactLabel          string
	Subject                string
	WindowStartTs          float64
	WindowEndTs            float64
	RequestedScopeRole     TraceTargetWaitRequestedScopeRole
	Count                  int
	WallClockMS            float64
	DStateOccurrences      int
	IOWaitOccurrences      int
	SleepIOWaitOccurrences int
	OtherWaitOccurrences   int
	Callers                []string
	Occurrences            []TargetWaitOccurrenceAuthorityRow
	RecordID               string
}

// TraceTargetWaitRequestedScopeRole distinguishes the account that answers
// the user's typed runtime-artifact scope from narrower exploration accounts.
// It is compiled from the quote-anchored RuntimeArtifactScopeProfile plus
// same-result deterministic scope coverage; timestamps alone never mint the
// full-artifact role.
type TraceTargetWaitRequestedScopeRole string

const (
	TraceTargetWaitScopeUnclassified          TraceTargetWaitRequestedScopeRole = ""
	TraceTargetWaitScopeRequestedPrincipal    TraceTargetWaitRequestedScopeRole = "requested_scope_principal"
	TraceTargetWaitScopeSupportingExploration TraceTargetWaitRequestedScopeRole = "supporting_exploration"
)

func (a TraceTargetWaitSummaryAuthority) IsRequestedScopePrincipal() bool {
	return a.RequestedScopeRole == TraceTargetWaitScopeRequestedPrincipal
}

// BuildTraceTargetWaitSummaryAuthorities returns complete, same-result wait
// summaries for typed user runtime targets. Repeated identical queries
// deduplicate; conflicting summaries for the same artifact/subject/window
// fail closed.
func BuildTraceTargetWaitSummaryAuthorities(ledger ObservationLedger, rm *RequestModel) []TraceTargetWaitSummaryAuthority {
	if rm == nil || len(rm.RuntimeTargets) == 0 {
		return nil
	}
	type candidate struct {
		key         string
		fingerprint string
		authority   TraceTargetWaitSummaryAuthority
	}
	var candidates []candidate
	for _, aggregate := range ledger.Records {
		if aggregate.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(aggregate.Producer) ||
			aggregate.GroundingPolicy != ClaimGroundingHard ||
			strings.TrimSpace(aggregate.Predicate) != "target_window_wait_occurrences" ||
			strings.TrimSpace(aggregate.Object) != "complete" ||
			!ObservationRecordMatchesUserRuntimeTarget(aggregate, rm) ||
			aggregate.ResultCount == nil ||
			aggregate.Span.EndTs <= aggregate.Span.StartTs {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(aggregate.Value))
		if err != nil || count <= 0 || count != *aggregate.ResultCount {
			continue
		}
		scopePrefix, ok := strings.CutSuffix(strings.TrimSpace(aggregate.ID), "#target_window_wait_occurrences")
		if !ok || scopePrefix == "" {
			continue
		}
		rowPrefix := scopePrefix + "#target_window_wait_occurrence:"
		rows := make(map[int]ObservationRecord, count)
		conflict := false
		for _, row := range ledger.Records {
			if !strings.HasPrefix(strings.TrimSpace(row.ID), rowPrefix) ||
				row.Origin != AnswerEvidenceOriginRuntimeArtifact ||
				!RuntimeObservationProducerIsDeterministicQuery(row.Producer) ||
				row.GroundingPolicy != ClaimGroundingHard ||
				strings.TrimSpace(row.Predicate) != "target_window_wait_occurrence" ||
				!strings.EqualFold(strings.TrimSpace(row.Subject), strings.TrimSpace(aggregate.Subject)) ||
				!traceTargetWaitSameResultSource(aggregate, row) {
				continue
			}
			ordinal, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(row.ID), rowPrefix))
			if err != nil || ordinal <= 0 || ordinal > count {
				conflict = true
				break
			}
			if prior, exists := rows[ordinal]; exists {
				if traceTargetWaitOccurrenceFingerprint(prior) != traceTargetWaitOccurrenceFingerprint(row) {
					conflict = true
				}
				continue
			}
			rows[ordinal] = row
		}
		if conflict || len(rows) != count {
			continue
		}
		authority := TraceTargetWaitSummaryAuthority{
			ArtifactLabel:      traceTargetStateAuthorityArtifactLabel(aggregate.SourceRef),
			Subject:            strings.TrimSpace(aggregate.Subject),
			WindowStartTs:      aggregate.Span.StartTs,
			WindowEndTs:        aggregate.Span.EndTs,
			RequestedScopeRole: traceTargetWaitRequestedScopeRole(aggregate, ledger, rm),
			Count:              count,
			RecordID:           strings.TrimSpace(aggregate.ID),
		}
		callers := map[string]bool{}
		for ordinal := 1; ordinal <= count; ordinal++ {
			row := rows[ordinal]
			duration, err := strconv.ParseFloat(strings.TrimSpace(row.Value), 64)
			if err != nil || duration < 0 || strings.TrimSpace(row.Unit) != "ms" ||
				row.Span.EndTs < row.Span.StartTs ||
				row.Span.StartTs < aggregate.Span.StartTs-0.000002 ||
				row.Span.EndTs > aggregate.Span.EndTs+0.000002 ||
				math.Abs((row.Span.EndTs-row.Span.StartTs)*1000-duration) > 0.002 {
				conflict = true
				break
			}
			fields, ok := traceTargetWaitOccurrenceObjectFields(row.Object)
			if !ok {
				conflict = true
				break
			}
			authority.Occurrences = append(authority.Occurrences, TargetWaitOccurrenceAuthorityRow{
				Ordinal:   ordinal,
				State:     fields["state"],
				StartTs:   row.Span.StartTs,
				EndTs:     row.Span.EndTs,
				DurationM: duration,
				IOWait:    fields["iowait"],
				Caller:    fields["caller"],
			})
			authority.WallClockMS += duration
			switch {
			case fields["state"] == "d_sleep":
				authority.DStateOccurrences++
			case fields["state"] == "io_wait":
				authority.IOWaitOccurrences++
			case fields["state"] == "s_sleep" && fields["iowait"] == "1":
				authority.SleepIOWaitOccurrences++
			default:
				authority.OtherWaitOccurrences++
			}
			if caller := strings.TrimSpace(fields["caller"]); caller != "" && caller != "unknown" {
				callers[caller] = true
			}
		}
		if conflict {
			continue
		}
		for caller := range callers {
			authority.Callers = append(authority.Callers, caller)
		}
		sort.Strings(authority.Callers)
		key := fmt.Sprintf("%s\x00%s\x00%.6f\x00%.6f",
			strings.ToLower(authority.ArtifactLabel),
			strings.ToLower(authority.Subject),
			authority.WindowStartTs,
			authority.WindowEndTs,
		)
		fingerprint := fmt.Sprintf("%d|%s|%d|%d|%d|%d|%s",
			authority.Count,
			strconv.FormatFloat(authority.WallClockMS, 'g', -1, 64),
			authority.DStateOccurrences,
			authority.IOWaitOccurrences,
			authority.SleepIOWaitOccurrences,
			authority.OtherWaitOccurrences,
			strings.Join(authority.Callers, "\x00"),
		)
		for _, occurrence := range authority.Occurrences {
			fingerprint += "|" + occurrence.CanonicalLine()
		}
		candidates = append(candidates, candidate{key: key, fingerprint: fingerprint, authority: authority})
	}
	byKey := map[string]TraceTargetWaitSummaryAuthority{}
	fingerprints := map[string]string{}
	conflicted := map[string]bool{}
	for _, candidate := range candidates {
		if prior, ok := fingerprints[candidate.key]; ok && prior != candidate.fingerprint {
			conflicted[candidate.key] = true
			continue
		}
		fingerprints[candidate.key] = candidate.fingerprint
		if prior, ok := byKey[candidate.key]; !ok ||
			traceTargetWaitRequestedScopeRolePriority(candidate.authority.RequestedScopeRole) <
				traceTargetWaitRequestedScopeRolePriority(prior.RequestedScopeRole) {
			byKey[candidate.key] = candidate.authority
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		if !conflicted[key] {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := byKey[keys[i]], byKey[keys[j]]
		lp := traceTargetWaitRequestedScopeRolePriority(left.RequestedScopeRole)
		rp := traceTargetWaitRequestedScopeRolePriority(right.RequestedScopeRole)
		if lp != rp {
			return lp < rp
		}
		return keys[i] < keys[j]
	})
	out := make([]TraceTargetWaitSummaryAuthority, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func traceTargetWaitRequestedScopeRole(
	aggregate ObservationRecord,
	ledger ObservationLedger,
	rm *RequestModel,
) TraceTargetWaitRequestedScopeRole {
	if rm == nil || rm.RuntimeArtifactScopeProfile == nil {
		return TraceTargetWaitScopeUnclassified
	}
	profile := rm.RuntimeArtifactScopeProfile
	if start, end, ok := profile.ExplicitTimeWindow(); ok {
		if math.Abs(aggregate.Span.StartTs-start) <= 0.000002 &&
			math.Abs(aggregate.Span.EndTs-end) <= 0.000002 {
			return TraceTargetWaitScopeRequestedPrincipal
		}
		return TraceTargetWaitScopeSupportingExploration
	}
	if !profile.FullArtifact() {
		return TraceTargetWaitScopeUnclassified
	}
	// The deterministic supplement executes with the analyzer's validated
	// requested scope. A supplement row under a full-artifact profile is
	// therefore a direct requested-scope account.
	if aggregate.SystemSupplement {
		return TraceTargetWaitScopeRequestedPrincipal
	}
	// Model-issued unbounded queries prove their physical artifact scope with a
	// sibling record from the SAME result. Pair by the producer-owned record
	// prefix; a coverage row from another query must not crown this account.
	scopePrefix, ok := strings.CutSuffix(
		strings.TrimSpace(aggregate.ID),
		"#target_window_wait_occurrences",
	)
	if ok && scopePrefix != "" {
		coverageID := scopePrefix + "#runtime_artifact_scope_coverage"
		for _, record := range ledger.Records {
			if strings.TrimSpace(record.ID) != coverageID ||
				record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
				record.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
				record.GroundingPolicy != ClaimGroundingHard ||
				!RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
				strings.TrimSpace(record.Predicate) != RuntimeArtifactScopeCoveragePredicate ||
				strings.TrimSpace(record.Object) != string(RuntimeArtifactScopeFullArtifact) ||
				strings.TrimSpace(record.Scope) != string(RuntimeArtifactScopeFullArtifact) ||
				!traceTargetWaitSameResultSource(aggregate, record) {
				continue
			}
			return TraceTargetWaitScopeRequestedPrincipal
		}
	}
	return TraceTargetWaitScopeSupportingExploration
}

func traceTargetWaitRequestedScopeRolePriority(role TraceTargetWaitRequestedScopeRole) int {
	switch role {
	case TraceTargetWaitScopeRequestedPrincipal:
		return 0
	case TraceTargetWaitScopeSupportingExploration:
		return 2
	default:
		return 1
	}
}

func traceTargetWaitSameResultSource(aggregate, row ObservationRecord) bool {
	a, b := aggregate.SourceRef, row.SourceRef
	if a.Kind != b.Kind ||
		strings.TrimSpace(a.ArtifactID) != strings.TrimSpace(b.ArtifactID) ||
		strings.TrimSpace(a.Path) != strings.TrimSpace(b.Path) ||
		strings.TrimSpace(a.PayloadRef) != strings.TrimSpace(b.PayloadRef) ||
		strings.TrimSpace(a.RawRef) != strings.TrimSpace(b.RawRef) {
		return false
	}
	aggregateAt := strings.TrimSpace(aggregate.ObservedAt)
	rowAt := strings.TrimSpace(row.ObservedAt)
	return aggregateAt == "" || rowAt == "" || aggregateAt == rowAt
}

func traceTargetWaitOccurrenceObjectFields(raw string) (map[string]string, bool) {
	fields := map[string]string{}
	for _, token := range strings.Split(strings.TrimSpace(raw), ";") {
		pair := strings.SplitN(strings.TrimSpace(token), "=", 2)
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" {
			return nil, false
		}
		fields[strings.TrimSpace(pair[0])] = strings.TrimSpace(pair[1])
	}
	state := fields["state"]
	iowait := fields["iowait"]
	if state == "" || (iowait != "0" && iowait != "1" && iowait != "unknown") {
		return nil, false
	}
	return fields, true
}

func traceTargetWaitOccurrenceFingerprint(record ObservationRecord) string {
	return fmt.Sprintf("%s|%s|%.9f|%.9f|%s|%s",
		strings.TrimSpace(record.Subject),
		strings.TrimSpace(record.Object),
		record.Span.StartTs,
		record.Span.EndTs,
		strings.TrimSpace(record.Value),
		strings.TrimSpace(record.Unit),
	)
}

func traceTargetStateAuthorityArtifactLabel(ref ObservationSourceRef) string {
	// Match the causal projection's typed artifact identity. Attached traces
	// carry Path=.../attached_trace.txt plus lane marker
	// ArtifactID=attached_trace; preferring the marker here made occurrence
	// rows from the same result fail to pair with the projection state card.
	if _, label, _ := traceCausalProjectionArtifactIdentity(ObservationRecord{SourceRef: ref}); label != "" {
		return label
	}
	return strings.TrimSpace(ref.ArtifactID)
}
