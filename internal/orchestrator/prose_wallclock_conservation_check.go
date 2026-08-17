package orchestrator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

// prose_wallclock_conservation_check.go — CR-3 件① P6 wall-clock
// conservation gate (§29.42 four-gate P6; §29.50 遗留 B1/B2,
// docs/design/real_trace_campaign_20260705.md, 2026-07-12).
//
// Witnesses (two customer-facing P1s from the CR-2 cold read):
//   - tieba B1: prose stated the main thread ran 20.372ms and queued
//     46.364ms while the published sleep total was 76.599ms — the three
//     state durations sum to 143.34ms against a 114.94ms window, a
//     physically impossible partition (真值 running 26.946 / runnable
//     3.636); the starvation attribution flipped threads.
//   - CAL-1 F-8: prose stated 7.830ms of same-core queueing for the
//     target while the published full-window runnable total was 5.604ms —
//     a single dimension exceeding everything the engine published.
//
// Mechanics: one thread's scheduler states partition wall clock, so for
// any thread Σ(running + runnable + sleep + D-state) can never exceed the
// window, and no single state can exceed the window or its own published
// full-window total. The typed comparator is the target_window_states
// record family (§29.27② COV-4 — the focused thread's full-window state
// partition, admitted only when Σ balances the window), so the check mixes
// prose-claimed values with published truth and detects the impossible
// combination whichever side is wrong.
//
// 全批软纪律 (§29.42.4/§29.47.1): every arm is DISCLOSURE ONLY — findings
// render on the system cross-check appendix (the system's own surface),
// never a violation, never a retry round, never a body edit. The prose
// extraction is a noisy signal, so each arm asserts only the loose,
// conservative form (per-dimension MAX, named tolerance, absolute floors)
// — a missed finding is a sanctioned cost, a false positive is not.
const (
	// proseWallClockConservationEpsilonRel is the P6 named relative margin
	// (ε): Σ or a single value must exceed the comparator by MORE than this
	// fraction before anything is disclosed. Independent constant — never
	// borrowed from the approx-tolerance or window-length tolerances
	// (容差常量禁跨语义借用).
	proseWallClockConservationEpsilonRel = 0.05
	// proseWallClockSingleExcessFloorMS / proseWallClockSumExcessFloorMS /
	// proseWallClockWindowExcessFloorMS are the absolute noise floors:
	// honest last-digit rounding of a small published value ("3.64ms" →
	// "4ms") must never disclose. Each arm owns its named floor (修复轮 P4:
	// the single-value-over-window arm must not borrow the Σ arm's name —
	// 容差常量禁跨语义借用).
	proseWallClockSingleExcessFloorMS = 0.5
	proseWallClockSumExcessFloorMS    = 1.0
	proseWallClockWindowExcessFloorMS = 1.0
	// proseWallClockKeywordGapRunes bounds the rune distance between a
	// state keyword and the ms numeral it claims ("实际运行仅 20.372ms" —
	// gap 2). Larger gaps ("运行该任务耗时 96ms") are ambiguous wall-clock
	// phrasing and stay out of the claim set.
	proseWallClockKeywordGapRunes = 6
	// proseWallClockDesignatorGapRunes bounds a DESIGNATOR binding: a
	// singular target designator (目标自身/主线程…) claims a value only when
	// it PRECEDES the numeral within this many runes ("目标自身在 cpu=12
	// 竞争 7.830 ms"). 首放误报实证 (tieba 复放 2026-07-12): an anaphora
	// sentence ("该线程自身 runnable_wait 累计 20.342ms … 与主线程优先级关系
	// 形成…") carried a TRAILING 主线程 that nearest-binding grabbed —
	// 20.342 belonged to the anaphoric subject of the PREVIOUS sentence,
	// not the main thread. A designator after the value, or far before it,
	// never binds; name-tid tokens keep plain nearest-binding.
	proseWallClockDesignatorGapRunes = 24
	proseWallClockClaimCap           = 64
	proseWallClockFindingCap         = 8
)

// proseWallClockDimension is one scheduler-state dimension of the
// conservation account.
type proseWallClockDimension string

const (
	proseWallClockDimRunning  proseWallClockDimension = "running"
	proseWallClockDimRunnable proseWallClockDimension = "runnable"
	proseWallClockDimSleep    proseWallClockDimension = "sleep"
	proseWallClockDimDState   proseWallClockDimension = "d_state"
	// proseWallClockDimIOWait claims are ambiguous between the D-side and
	// sleep-side IO lanes, so they feed ONLY the single-dimension arm
	// (against the union comparator) and never the Σ arm (folding them
	// into a partition dimension could double-count sleep-side IO).
	proseWallClockDimIOWait proseWallClockDimension = "io_wait"
)

// proseWallClockConservationDims are the partition dimensions of the Σ arm.
var proseWallClockConservationDims = []proseWallClockDimension{
	proseWallClockDimRunning, proseWallClockDimRunnable,
	proseWallClockDimSleep, proseWallClockDimDState,
}

func (d proseWallClockDimension) labelZH() string {
	switch d {
	case proseWallClockDimRunning:
		return "运行(running)"
	case proseWallClockDimRunnable:
		return "就绪等待(runnable)"
	case proseWallClockDimSleep:
		return "睡眠(sleep)"
	case proseWallClockDimDState:
		return "D状态(D-state)"
	case proseWallClockDimIOWait:
		return "IO等待(io_wait)"
	}
	return string(d)
}

// proseWallClockKeyword maps one prose wording onto a state dimension.
// needsCPUContext gates ambiguous contention wording ("竞争") on the
// sentence also naming a CPU — the F-8 shape ("目标自身在 cpu=12 竞争
// 7.830 ms") — so lock-contention durations never enter the claim set.
type proseWallClockKeyword struct {
	word            string
	dim             proseWallClockDimension
	needsCPUContext bool
}

// proseWallClockKeywords is longest-match-first per position.
var proseWallClockKeywords = []proseWallClockKeyword{
	{word: "runnable_wait", dim: proseWallClockDimRunnable},
	{word: "runnable", dim: proseWallClockDimRunnable},
	{word: "可运行", dim: proseWallClockDimRunnable},
	{word: "就绪", dim: proseWallClockDimRunnable},
	{word: "等待调度", dim: proseWallClockDimRunnable},
	{word: "竞争", dim: proseWallClockDimRunnable, needsCPUContext: true},
	{word: "running", dim: proseWallClockDimRunning},
	{word: "运行", dim: proseWallClockDimRunning},
	{word: "s_sleep", dim: proseWallClockDimSleep},
	{word: "sleep", dim: proseWallClockDimSleep},
	{word: "睡眠", dim: proseWallClockDimSleep},
	{word: "深睡", dim: proseWallClockDimSleep},
	{word: "d_state_or_io_wait", dim: proseWallClockDimDState},
	{word: "d_state", dim: proseWallClockDimDState},
	{word: "d-state", dim: proseWallClockDimDState},
	{word: "D 状态", dim: proseWallClockDimDState},
	{word: "D状态", dim: proseWallClockDimDState},
	{word: "D 态", dim: proseWallClockDimDState},
	{word: "D态", dim: proseWallClockDimDState},
	{word: "不可中断", dim: proseWallClockDimDState},
	{word: "io_wait", dim: proseWallClockDimIOWait},
	{word: "iowait", dim: proseWallClockDimIOWait},
	{word: "IO等待", dim: proseWallClockDimIOWait},
	{word: "IO 等待", dim: proseWallClockDimIOWait},
	{word: "io等待", dim: proseWallClockDimIOWait},
}

// proseWallClockDesignators are singular target designators that bind a
// claim to THE account subject when the run has exactly one full-window
// account (ambiguity keeps them silent). 主线程 is NOT in this list — it
// designates a PROCESS ROLE, not the analysis target, so it binds only
// when the sole account subject is provably a main thread (修复轮 P3①:
// tid==tgid proven on the evidence face; a CompThread-target run must
// never absorb 主线程-worded claims).
var proseWallClockDesignators = []string{"目标线程", "目标自身", "关注线程"}

// proseWallClockMainThreadDesignator is the role-worded designator gated
// on the tid==tgid proof (see above).
const proseWallClockMainThreadDesignator = "主线程"

// proseWallClockAccount is one parsed target_window_states record — the
// typed full-window state partition of one thread.
type proseWallClockAccount struct {
	subject string
	tid     string
	dims    map[proseWallClockDimension]float64
	sleepIO float64
	// ioWait (修复轮 件4, 2026-07-13): the account's OWN io_wait lane — the
	// target_window_states partition is really FIVE states (donghu 常态
	// io_wait>0), so the partition-fact decomposition must list it or its Σ
	// self-verification breaks on every io_wait>0 account. Additive field:
	// the P6 conservation arms deliberately do not consume it (their Σ arm
	// keeps the four partition dims + union comparator design).
	ioWait   float64
	totalMS  float64
	windowMS float64
	// mainThread (修复轮 P3①): the subject is PROVEN a main thread — some
	// evidence record for the same tid publishes a typed tgid equal to it.
	// Gates the 主线程 designator; unprovable stays false (never guesses).
	mainThread bool
}

// proseWallClockClaim is one prose-claimed (thread, dimension, value).
type proseWallClockClaim struct {
	tid     string
	subject string // as the prose spelled the thread
	dim     proseWallClockDimension
	value   float64
	label   string // numeral as written + unit
	blockID string
}

// proseWallClockConservationFindings is the CR-3 P6 appendix provider.
// Information lane only: the caller renders these on the system
// cross-check appendix; nothing here raises a violation or retry.
func proseWallClockConservationFindings(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState) []proseScalarBindingFinding {
	if doc == nil || bus == nil || mut == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return nil
	}
	accounts := proseWallClockAccountsFromLedger(ledger)
	if len(accounts) == 0 {
		return nil
	}
	claims := proseWallClockScanClaims(doc, accounts)
	if len(claims) == 0 {
		return nil
	}
	maxWindow := 0.0
	for _, acc := range accounts {
		if acc.windowMS > maxWindow {
			maxWindow = acc.windowMS
		}
	}
	byTID := map[string]*proseWallClockAccount{}
	for i := range accounts {
		byTID[accounts[i].tid] = &accounts[i]
	}
	// Per (tid, dim) keep the MAX claim (the loose direction: smaller
	// restatements of the same dimension are sub-claims of the max).
	claimed := map[string]map[proseWallClockDimension]proseWallClockClaim{}
	order := []string{}
	for _, c := range claims {
		if _, ok := claimed[c.tid]; !ok {
			claimed[c.tid] = map[proseWallClockDimension]proseWallClockClaim{}
			order = append(order, c.tid)
		}
		if prev, ok := claimed[c.tid][c.dim]; !ok || c.value > prev.value {
			claimed[c.tid][c.dim] = c
		}
	}
	var out []proseScalarBindingFinding
	add := func(f proseScalarBindingFinding) {
		if len(out) < proseWallClockFindingCap {
			out = append(out, f)
		}
	}
	for _, tid := range order {
		dims := claimed[tid]
		acc := byTID[tid]
		window := maxWindow
		if acc != nil && acc.windowMS > 0 {
			window = acc.windowMS
		}
		subject := dims[proseWallClockFirstDim(dims)].subject
		// Arm B — a single state duration can never exceed the window.
		singleOver := map[proseWallClockDimension]bool{}
		if window > 0 {
			for dim, c := range dims {
				if c.value > window*(1+proseWallClockConservationEpsilonRel) &&
					c.value-window > proseWallClockWindowExcessFloorMS {
					singleOver[dim] = true
					add(proseScalarBindingFinding{
						entry: fmt.Sprintf("the prose states %s of %s time for thread %s, which exceeds the %.3fms analysis window — a caliber mix or numeric error is likely",
							c.label, string(dim), subject, window),
						entryZH: fmt.Sprintf("正文中线程 %s 的%s时长 %s 超过窗长 %.3fms，存在口径混用或数值错误的可能",
							subject, dim.labelZH(), c.label, window),
					})
				}
			}
		}
		// Arm C — a single dimension can never exceed its own published
		// full-window total (needs the thread's typed account).
		if acc != nil {
			for dim, c := range dims {
				if singleOver[dim] {
					continue
				}
				comparator, ok := proseWallClockDimComparator(acc, dim)
				if !ok {
					continue
				}
				if c.value > comparator*(1+proseWallClockConservationEpsilonRel) &&
					c.value-comparator > proseWallClockSingleExcessFloorMS {
					add(proseScalarBindingFinding{
						entry: fmt.Sprintf("the prose states %s of %s time for thread %s, which exceeds that thread's published full-window %s total of %.3fms",
							c.label, string(dim), subject, string(dim), comparator),
						entryZH: fmt.Sprintf("正文中线程 %s 的%s时长 %s 超过该线程该维度已发布总量 %.3fms",
							subject, dim.labelZH(), c.label, comparator),
					})
				}
			}
		}
		// Arm A — Σ over the partition dimensions (per-dimension MAX of
		// prose claim vs published truth) can never exceed the window.
		// 修复轮二放: when EVERY prose-claimed dimension already fired the
		// single-value arm, the Σ line restates the same fact — skip it
		// (one claim, one line).
		if window <= 0 {
			continue
		}
		allSingleOver := len(dims) > 0
		for dim := range dims {
			if !singleOver[dim] {
				allSingleOver = false
				break
			}
		}
		if allSingleOver {
			continue
		}
		sum := 0.0
		var parts []string
		var partsZH []string
		claimedAny := false
		for _, dim := range proseWallClockConservationDims {
			v := 0.0
			src := "published"
			if acc != nil {
				v = acc.dims[dim]
			}
			if c, ok := dims[dim]; ok {
				claimedAny = true
				if c.value > v {
					v = c.value
					src = "prose"
				}
			}
			if v <= 0 {
				continue
			}
			sum += v
			srcZH := "已发布"
			if src == "prose" {
				srcZH = "正文"
			}
			parts = append(parts, fmt.Sprintf("%s %.3fms (%s)", string(dim), v, src))
			partsZH = append(partsZH, fmt.Sprintf("%s %.3fms（%s）", dim.labelZH(), v, srcZH))
		}
		if !claimedAny {
			continue
		}
		if sum > window*(1+proseWallClockConservationEpsilonRel) &&
			sum-window > proseWallClockSumExcessFloorMS {
			add(proseScalarBindingFinding{
				entry: fmt.Sprintf("the state durations for thread %s sum to %.3fms (%s), which exceeds the %.3fms window — a caliber mix or numeric error is likely",
					subject, sum, strings.Join(parts, " + "), window),
				entryZH: fmt.Sprintf("线程 %s 的状态时长之和 %.3fms（%s）超过窗长 %.3fms，存在口径混用或数值错误",
					subject, sum, strings.Join(partsZH, "+"), window),
			})
		}
	}
	return out
}

func proseWallClockFirstDim(dims map[proseWallClockDimension]proseWallClockClaim) proseWallClockDimension {
	keys := make([]string, 0, len(dims))
	for k := range dims {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	return proseWallClockDimension(keys[0])
}

// proseWallClockDimComparator returns the published full-window total the
// dimension is compared against. IO-wait claims can refer to the exclusive
// io_wait state lane or the sleep-side IO attribution lane, so they use that
// typed union (loose); non-IO D-state is not part of the comparator.
func proseWallClockDimComparator(acc *proseWallClockAccount, dim proseWallClockDimension) (float64, bool) {
	if acc == nil {
		return 0, false
	}
	if dim == proseWallClockDimIOWait {
		return acc.ioWait + acc.sleepIO, true
	}
	v, ok := acc.dims[dim]
	return v, ok
}

// proseWallClockAccountsFromLedger returns the target-state accounts elected by
// the SAME typed scope authority as the trace causal projection.  An explorer
// may issue a wider probe before it reaches the user's exact window; choosing
// the largest raw target_window_states record here would let that exploratory
// window leak into the system appendix even though the final projection chose
// the exact window.  The shared authority owns that election.  The raw-record
// parser remains only as a legacy fallback for ledgers that predate selected
// window metadata and therefore cannot compile an authority.
func proseWallClockAccountsFromLedger(ledger types.ObservationLedger) []proseWallClockAccount {
	if authorities := types.BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger); len(authorities) > 0 {
		out := make([]proseWallClockAccount, 0, len(authorities))
		for _, authority := range authorities {
			subject := strings.TrimSpace(authority.Subject)
			tid := proseWallClockSubjectTID(subject)
			if subject == "" || tid == "" || authority.TotalMS <= 0 {
				continue
			}
			windowMS := authority.WindowMS
			if windowMS <= 0 {
				windowMS = authority.TotalMS
			}
			out = append(out, proseWallClockAccount{
				subject: subject,
				tid:     tid,
				dims: map[proseWallClockDimension]float64{
					proseWallClockDimRunning:  authority.RunningMS,
					proseWallClockDimRunnable: authority.RunnableMS,
					proseWallClockDimSleep:    authority.SleepMS,
					proseWallClockDimDState:   authority.DStateMS,
				},
				sleepIO:  authority.SleepIOWaitMS,
				ioWait:   authority.IOWaitMS,
				totalMS:  authority.TotalMS,
				windowMS: windowMS,
			})
		}
		proseWallClockMarkMainThreads(out, ledger)
		return out
	}

	var out []proseWallClockAccount
	for _, record := range ledger.Records {
		if strings.TrimSpace(record.Predicate) != "target_window_states" {
			continue
		}
		subject := strings.TrimSpace(record.Subject)
		if subject == "" {
			continue
		}
		tid := proseWallClockSubjectTID(subject)
		if tid == "" {
			continue
		}
		acc := proseWallClockAccount{
			subject: subject,
			tid:     tid,
			dims: map[proseWallClockDimension]float64{
				proseWallClockDimRunning:  proseWallClockNoteFloat(record.RichNotes, types.TraceNoteKeyRunning),
				proseWallClockDimRunnable: proseWallClockNoteFloat(record.RichNotes, types.TraceNoteKeyRunnable),
				proseWallClockDimSleep:    proseWallClockNoteFloat(record.RichNotes, types.TraceNoteKeySleep),
				proseWallClockDimDState:   proseWallClockNoteFloat(record.RichNotes, types.TraceNoteKeyDState),
			},
			sleepIO:  proseWallClockNoteFloat(record.RichNotes, types.TraceNoteKeySleepIOWait),
			ioWait:   proseWallClockNoteFloat(record.RichNotes, types.TraceNoteKeyIOWait),
			totalMS:  proseWallClockNoteFloat(record.RichNotes, types.TraceNoteKeyTotal),
			windowMS: proseWallClockNoteFloat(record.RichNotes, types.TraceNoteKeyWindowMS),
		}
		if acc.totalMS <= 0 {
			continue
		}
		if acc.windowMS <= 0 {
			acc.windowMS = acc.totalMS
		}
		// Legacy records have no selected-window authority. Keep, per thread,
		// the largest observed window as the old conservative comparator.
		replaced := false
		for i := range out {
			if out[i].tid == acc.tid {
				replaced = true
				if acc.windowMS > out[i].windowMS ||
					(acc.windowMS == out[i].windowMS && acc.totalMS > out[i].totalMS) {
					out[i] = acc
				}
				break
			}
		}
		if !replaced {
			out = append(out, acc)
		}
	}
	proseWallClockMarkMainThreads(out, ledger)
	return out
}

// proseWallClockMarkMainThreads proves main-threadness from the evidence face:
// a record whose subject carries the SAME tid and whose typed tgid note equals
// that tid (the CR-3 件③ rank-row tgid lane). Unprovable stays false.
func proseWallClockMarkMainThreads(out []proseWallClockAccount, ledger types.ObservationLedger) {
	for i := range out {
		for _, record := range ledger.Records {
			if proseWallClockSubjectTID(strings.TrimSpace(record.Subject)) != out[i].tid {
				continue
			}
			if tgid := strings.TrimSpace(proseWallClockNoteValue(record.RichNotes, types.TraceNoteKeyTGID)); tgid != "" {
				if tgid == out[i].tid {
					out[i].mainThread = true
				}
				break
			}
		}
	}
}

// proseWallClockNoteValue returns the raw value of a k=v rich note.
func proseWallClockNoteValue(notes []string, key string) string {
	prefix := key + "="
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if strings.HasPrefix(note, prefix) {
			return strings.TrimPrefix(note, prefix)
		}
	}
	return ""
}

func proseWallClockNoteFloat(notes []string, key string) float64 {
	prefix := key + "="
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if !strings.HasPrefix(note, prefix) {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(note, prefix))
		raw = strings.TrimSuffix(raw, "ms")
		if f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
			return f
		}
	}
	return 0
}

// proseWallClockSubjectTID extracts the numeric tid from a name-tid
// subject spelling (last-dash split, digits only). Deliberately NOT the
// prose extractor: engine subjects may start with characters the prose
// boundary guards reject (".ugc.aweme.lite-17267"), and the subject side
// is a typed engine field, not noisy prose.
func proseWallClockSubjectTID(subject string) string {
	dash := strings.LastIndexByte(subject, '-')
	if dash <= 0 || dash == len(subject)-1 {
		return ""
	}
	tid := subject[dash+1:]
	for i := 0; i < len(tid); i++ {
		if tid[i] < '0' || tid[i] > '9' {
			return ""
		}
	}
	return tid
}

// proseWallClockScanClaims walks the model-authored prose surfaces and
// extracts thread-bound state-duration claims. Sentence granularity: the
// thread binding and the keyword must live in the SAME sentence as the
// numeral (件⑤ block-level nearest-binding lesson).
func proseWallClockScanClaims(doc *types.AnswerDocumentV2, accounts []proseWallClockAccount) []proseWallClockClaim {
	var out []proseWallClockClaim
	soleAccountSubject := ""
	soleAccountTID := ""
	soleAccountMainThread := false
	if len(accounts) == 1 {
		soleAccountSubject = accounts[0].subject
		soleAccountTID = accounts[0].tid
		soleAccountMainThread = accounts[0].mainThread
	}
	scan := func(blockID, text string) {
		if text == "" || len(out) >= proseWallClockClaimCap {
			return
		}
		for _, span := range proseSentenceSpans(text) {
			sentence := text[span[0]:span[1]]
			toks := extractProseScalarTokens(blockID, sentence)
			if len(toks) == 0 {
				continue
			}
			threads := extractProseScalarThreadRefs(sentence)
			// Designator pseudo-refs (bound to THE sole account subject) —
			// collected separately: a designator binds a token only when it
			// PRECEDES it within the bounded gap (anaphora guard, see the
			// constant doc).
			type designatorRef struct {
				pos int
				end int
			}
			var designators []designatorRef
			if soleAccountTID != "" {
				words := proseWallClockDesignators
				if soleAccountMainThread {
					// 修复轮 P3①: the role word binds only under the
					// tid==tgid proof.
					words = append(append([]string{}, words...), proseWallClockMainThreadDesignator)
				}
				for _, word := range words {
					idx := 0
					for {
						rel := strings.Index(sentence[idx:], word)
						if rel < 0 {
							break
						}
						designators = append(designators, designatorRef{pos: idx + rel, end: idx + rel + len(word)})
						idx += rel + len(word)
					}
				}
			}
			if len(threads) == 0 && len(designators) == 0 {
				continue
			}
			cpuContext := proseWallClockSentenceHasCPUContext(sentence)
			for _, tok := range toks {
				if len(out) >= proseWallClockClaimCap {
					return
				}
				if tok.percent() || tok.Value <= 0 {
					continue
				}
				dim, ok := proseWallClockClaimDimension(sentence, tok.Pos, cpuContext)
				if !ok {
					continue
				}
				// 修复轮二放误报实证 (tieba 2026-07-12: 「CPU=0
				// runnable_wait=572.289ms,核心已被 sysevent_store-47924(…)
				// 占满」 — the CPU-queue total bound to the TRAILING thread
				// name): a subject claims a value it PRECEDES ("X-123 运行
				// 20ms"); a name after the value belongs to the next clause.
				// Same direction rule the designators already obey.
				candidates := make([]proseScalarThreadRef, 0, len(threads)+len(designators))
				for _, tref := range threads {
					if tref.Pos < tok.Pos {
						candidates = append(candidates, tref)
					}
				}
				for _, d := range designators {
					if d.end <= tok.Pos && len([]rune(sentence[d.end:tok.Pos])) <= proseWallClockDesignatorGapRunes {
						candidates = append(candidates, proseScalarThreadRef{
							Raw: soleAccountSubject, TID: soleAccountTID, Pos: d.pos,
						})
					}
				}
				if len(candidates) == 0 {
					continue
				}
				nearest := candidates[0]
				for _, tref := range candidates[1:] {
					if proseScalarPosDistance(tref.Pos, tok.Pos) < proseScalarPosDistance(nearest.Pos, tok.Pos) {
						nearest = tref
					}
				}
				if nearest.TID == "" {
					continue
				}
				out = append(out, proseWallClockClaim{
					tid:     nearest.TID,
					subject: nearest.Raw,
					dim:     dim,
					value:   tok.Value,
					label:   tok.label(),
					blockID: blockID,
				})
			}
		}
	}
	for _, blk := range doc.Blocks {
		if proseScalarScanExemptBlock(blk) {
			continue
		}
		if blk.Kind == types.BlockCaveat || blk.Kind == types.BlockDiagram {
			continue
		}
		scan(blk.ID, blk.Title)
		scan(blk.ID, blk.Text)
		for _, it := range blk.Items {
			scan(blk.ID, it.Label)
			scan(blk.ID, it.Text)
			for _, cell := range it.Cells {
				scan(blk.ID, cell)
			}
		}
	}
	return out
}

func proseWallClockSentenceHasCPUContext(sentence string) bool {
	lower := strings.ToLower(sentence)
	return strings.Contains(lower, "cpu") || strings.Contains(sentence, "核")
}

// proseWallClockClaimDimension resolves the state dimension claimed by the
// numeral at tokPos: the nearest keyword ending within
// proseWallClockKeywordGapRunes runes BEFORE the numeral.
func proseWallClockClaimDimension(sentence string, tokPos int, cpuContext bool) (proseWallClockDimension, bool) {
	if tokPos <= 0 || tokPos > len(sentence) {
		return "", false
	}
	prefix := sentence[:tokPos]
	lowerPrefix := strings.ToLower(prefix)
	bestEnd := -1
	var bestDim proseWallClockDimension
	for _, kw := range proseWallClockKeywords {
		if kw.needsCPUContext && !cpuContext {
			continue
		}
		needle := kw.word
		hay := prefix
		if needle == strings.ToLower(needle) && needle[0] < 0x80 {
			hay = lowerPrefix
		}
		idx := 0
		for {
			rel := strings.Index(hay[idx:], needle)
			if rel < 0 {
				break
			}
			start := idx + rel
			end := start + len(needle)
			idx = end
			// ASCII word boundaries for ASCII keywords.
			if needle[0] < 0x80 {
				if start > 0 {
					prev := hay[start-1]
					if prev == '_' || (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') || (prev >= '0' && prev <= '9') {
						continue
					}
				}
				if end < len(hay) {
					next := hay[end]
					if next == '_' || (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') {
						continue
					}
				}
			}
			if end > bestEnd {
				bestEnd = end
				bestDim = kw.dim
			}
		}
	}
	if bestEnd < 0 {
		return "", false
	}
	gap := prefix[bestEnd:]
	if len([]rune(gap)) > proseWallClockKeywordGapRunes {
		return "", false
	}
	return bestDim, true
}

// proseSentenceSpans splits a text unit into sentence byte spans. Shared
// by the P6 claim scan and the 件⑤ sentence-granularity binding arm.
// Terminators: 。！？!? ; ； and newline; an ASCII '.' terminates only when
// followed by whitespace and not preceded by a digit (decimals and
// name-tid tokens stay whole).
func proseSentenceSpans(text string) [][2]int {
	var spans [][2]int
	start := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		terminal := false
		switch r {
		case '。', '！', '？', '；', '\n', '!', '?', ';':
			terminal = true
		case '.':
			if i+1 < len(text) && (text[i+1] == ' ' || text[i+1] == '\t' || text[i+1] == '\n') &&
				(i == 0 || text[i-1] < '0' || text[i-1] > '9') {
				terminal = true
			}
		}
		i += size
		if terminal {
			if i > start {
				spans = append(spans, [2]int{start, i})
			}
			start = i
		}
	}
	if start < len(text) {
		spans = append(spans, [2]int{start, len(text)})
	}
	return spans
}
