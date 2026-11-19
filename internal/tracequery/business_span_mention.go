package tracequery

// business_span_mention.go — SPANVIS-1: the pure-advisory business-lens span
// mention face (user ruling 2026-07-19, 定形原则 verbatim: 「一个原则就是某个
// span过长,或者某类span高频聚合后过长,并且是链上(含自身)则需要提及,最好是
// 在因果树上和窗内可消除总览上提及。但不参与根因排序,只是从另外一个维度
// (业务视角)给用户指导,可能的优化方向。」).
//
// Admission = (family in-window wall-clock total clears the significance
// floor) ∧ (typed on-chain credential, incl. self). Every admission input is
// a PRECISE signal (typed thread identity, typed edge credentials, exact
// float sums); the face itself is soft output only — no seat, no ordinal, no
// bar, no conservation/census membership, no gate reads it.
//
// EVOLUTION (POOL2-1 件①, §29.160/§29.160.1 user ruling 2026-07-20 verbatim:
// 「这里的 "某个span过长…则需要提及" 指的是链上的(注意 自身也属于链上),如果
// 非链上,对优化方向和建议,以及优化收益都没有太大直接关系,属于噪音。」): the
// former THIRD admission gate — ≥1 member hidden below the bounded display
// view — is REMOVED. The mention duty binds to ON-CHAIN spans, self included
// (自身也属于链上 — the self arm passes exactly like chain_member/host once
// past the two remaining gates, no residual condition may suppress it); the
// bounded top-8 seat view is not the tree/◎ mention face, so "every member
// already reached the seat machinery" is no defense against omission. A
// fully-visible significant on-chain family now mentions with HiddenCount==0
// (the field stays informational). Non-chain stays zero-duty: gate ① keeps
// its strict typed on-chain credential unrelaxed. OmittedFamilies keeps
// counting admitted overflow only — its population honestly grows with the
// admission set.
//
//   - Inventory: WindowStats.traceSpanFullInventory — the FULL pre-bound span
//     inventory (§29.137 LOCKSPAN 调查 / §29.143 备案: the display top-8 cap
//     is a display budget; the seat/carve machinery keeps consuming the
//     bounded view byte-identically, and the full inventory feeds ONLY this
//     mention face). nil inventory → nil result (fail-open).
//   - Family key: (thread tid, verbatim span name) — 任意名聚族, no lexicon.
//   - On-chain (含自身), existing credential machinery only (禁自造第二套边
//     判定): self = chain.Target; chain member = wakeupChainThreadSet; host =
//     hostSemanticSpanEdgeAnchor R3 edge credential. Another thread's span
//     counts only for its part inside a wait of the target that thread's
//     own recorded wakeup ended (or inside its chain node windows), with its
//     raw wall shown beside it (batch-377 ruling R-377-1, the same bound as
//     the owner-approved dependency credit of 2026-11-18; it supersedes
//     SPANVIS P1-1 "wholly before the latest credential edge" and the
//     interim "member must lie wholly inside one wait": an on-chain business
//     clue crossing the wait boundary must not vanish). Relation-qualified
//     raw business occupancy, not a priced semantic-completion claim. A
//     thread with NO typed credential is never mentioned (硬纪律).
//   - Significance floor (微窗噪音门, user ruling 2026-07-19 verbatim: 「过于
//     微窗的不必关注,容易引入噪音,除非占用时长达到显著」): family total ≥
//     max(dust floor, window-share floor × window). Measured on the
//     donghu/tieba name census (scratchpad spanvis_floor_probe, 2026-07-19):
//     an absolute-only floor cannot both admit the donghu L1 形A monitor
//     family (0.303ms of a 4.5ms window = 6.7%) and reject same-magnitude
//     dust in the 233ms full window (0.3ms = 0.1%); a share-only floor
//     admits µs dust in sub-ms windows. The single-long shape needs no
//     second gate: a single long span is its own one-member family, so the
//     family-total floor subsumes 「单 span 过长」 exactly.
//   - Values are the raw in-window projections of the inventory members,
//     verbatim (原始值三问: 提及行值=原始段值; Σ/max/count are recomputable
//     from the same inventory member set — identity pinned).
//
// EVOLUTION (HMC377 E3, business clues on the chain; design document
// hmc377/design/business-clue-seats.md). Two wrong answers on real captures:
// a chain member that is on the chain for 1.648ms (one dependency window)
// published its whole 480-528ms spans as on-chain clues and took seven of
// the eight seats, and the analysed thread's own phases (makeApplication,
// the code-opening phase, a 50ms library-loading lock wait) were left out.
// The face now publishes in two tiers that never compete for one budget:
//   - own phases (basis self): the analysed thread's spans, each family
//     carrying the self time and the scheduler split of its instances from
//     the synchronous marker tree when every member has a closed tree
//     account; selected by 合计最长 ∪ 单次最长 ∪ 自身最长 (each TOP-K) —
//     inclusive time of nested phases always favours the enclosing wrapper,
//     the self account is where the thread's own time went;
//   - dependency threads (chain_member / host_wakeup_edge): another thread
//     contributes ONLY the part of each span inside its own typed dependency
//     windows — the merged chain node windows (chainAnchorWindowsByPID, the
//     windows the semantic seats intersect) united with the analysed
//     thread's waits that thread's own recorded wakeups ended — read from
//     the one carrier the host-edge seats price with
//     (ChainResult.directDependencyWindows, attachTargetWaitDependencyWindows;
//     integration ruling R-377-1: one edge rule, one window builder). A span
//     outside every window
//     contributes nothing, so a thread whose link is only temporal proximity
//     is never mentioned; this replaces the host-edge arm's former "wholly
//     before the latest credential edge" rule (a thread that delivered one
//     wakeup late in a long window no longer publishes every earlier span).
//     The raw in-window wall of the contributing members rides SpanWallMs.
//     The credential test itself (self / chain member / host edge) is
//     unchanged.
// Reading order is own tier first, then dependency tier, each by TotalMs.

import (
	"math"
	"sort"
	"strings"
)

// BusinessSpanMentionDustFloorMs is the absolute dust component of the
// significance floor: a family whose whole-window wall-clock total sits below
// 0.1ms is never significant, in ANY window (micro-window noise gate — the
// donghu L2 sentinel-owner lock families, ≤3µs × a handful of members, stay
// out even when a sub-ms window would make their window share look large).
const BusinessSpanMentionDustFloorMs = 0.1

// BusinessSpanMentionWindowShareFloor is the window-share component of the
// significance floor: 1% of the selected window. Measured floor (donghu L1
// 形A 6.7% / tieba 锁窗 suspend family 1.8% must pass; donghu 233ms full
// window's sub-2.33ms long tail must not).
const BusinessSpanMentionWindowShareFloor = 0.01

// BusinessSpanMentionFamilyCap is the per-criterion TOP-K of the dual-rule
// selection set (OMGCLEAN-1 件5 rider3, §29.175.5 user ruling 2026-07-20).
// EVOLUTION RECORD: the constant was the SPANTOP-1 single 「TotalMs TOP3」
// display cap over ALL admitted families (semantic classes included); the
// §29.175.5 content-boundary ruling makes the two dimensions mutually
// exclusive by content — typed semantic_class families leave ◈ entirely
// (they live on the seat/optimization-table faces) — and re-casts the cap as
// the selection rule over the NON-semantic business families:
// 单次最长 TOP-K ∪ 合计最长 TOP-K, deduped. Families admitted beyond the
// selection are counted honestly in OmittedFamilies (§29.175.3 提及义务:
// zero-crop obligation converges onto the selection set; the tail count
// discloses the rest).
//
// EVOLUTION RECORD (RULE3-1 件11, §29.185③ verbatim, 2026-07-21): K = 3 → 5.
// User ruling: 「"业务线索" 当前的TOP 3 需要扩展为 top 5,因为top 3 覆盖面
// 大概率被无关的 "Choreographer#doFrame " 占榜,没办法暴露真实需要客户关注的
// 信息。」 — the anchor span stays un-excluded (§29.175.5④ 维持: the user
// chose CAPACITY over anchor exclusion to solve the doFrame crowd-out), the
// dual-criterion union rule itself is unchanged, and every word face
// (◎ zone head / tree ◈ head / legend selection-rule word) moves in the
// same batch (词面单点).
//
// EVOLUTION RECORD (MENTION8-1, §29.203 user ruling verbatim, 2026-07-21):
// K = 5 → 8. 「另外 "业务线索" TOP 5 可能不太够,是否可以安全的扩充为TOP 8
// (如果有)?」 — a pure display-selection cap widening: the dual-criterion
// union rule (单次最长 TOP-K ∪ 合计最长 TOP-K 去重) is unchanged, the
// admission gates (on-chain ∧ dual-component significance floor) move zero
// bytes, ranking and value channels are untouched, fewer-than-K families
// render their actual count, and the honest tail count discloses admitted
// overflow exactly as before. Word faces (tree ◈ head selection-rule word /
// legend) move in the same batch (词面单点).
const BusinessSpanMentionFamilyCap = 8

// Closed-set on-chain basis tokens of the mention face (凭证词如实 — the row
// says how the thread's on-chain status was proven, at exactly the strength
// the credential supports).
const (
	// BusinessSpanMentionBasisSelf — the analysis target's own spans (自身=
	// 恒链上 by definition).
	BusinessSpanMentionBasisSelf = "self"
	// BusinessSpanMentionBasisChainMember — the thread is a node of the typed
	// wakeup chain universe toward the target.
	BusinessSpanMentionBasisChainMember = "chain_member"
	// BusinessSpanMentionBasisHostWakeupEdge — the host thread holds its OWN
	// in-window typed wakeup edge toward the target (R3 credential); only the
	// part of its spans inside the target waits that thread's own wakeups
	// ended counts, with the raw wall beside it (ruling R-377-1).
	BusinessSpanMentionBasisHostWakeupEdge = "host_wakeup_edge"
)

// BusinessSpanMention is one admitted (thread, verbatim span name) family on
// the advisory mention face. Own-phase and host-edge values are raw in-window
// projections summed over the admitted member set; a chain member's values
// are the parts of its members inside its own dependency windows (the raw
// in-window wall stays beside them in SpanWallMs). Never envelopes, never
// re-scaled.
type BusinessSpanMention struct {
	Thread ThreadRef `json:"thread"`
	// Name is the verbatim span name (typed family key — 任意名聚族).
	Name string `json:"name"`
	// Count / TotalMs / MaxSingleMs: admitted member count, Σ of their
	// counted durations, and the largest single counted member (双杠杆线索
	// typed fact trio: 次数多而单段小 vs 单段长). For a chain member the
	// counted duration of a member is its part inside the dependency windows.
	Count       int     `json:"count"`
	TotalMs     float64 `json:"total_ms"`
	MaxSingleMs float64 `json:"max_single_ms"`
	// SpanWallMs is the Σ of the same members' raw in-window walls. It equals
	// TotalMs for own phases and is at least TotalMs for another thread's
	// family, chain member or host wakeup edge alike (the part outside its
	// dependency windows is the difference — context, not an on-chain clue;
	// ruling R-377-1 shows it beside the counted part).
	SpanWallMs float64 `json:"span_wall_ms"`
	// StartLine/EndLine: the member line envelope (min start .. max end) —
	// the mention row's 行a..b pointer (no seat, so no evidence tag).
	StartLine int `json:"start_line,omitempty"`
	EndLine   int `json:"end_line,omitempty"`
	// OnChainBasis ∈ {self, chain_member, host_wakeup_edge} (closed set).
	OnChainBasis string `json:"on_chain_basis"`
	// HiddenCount is how many admitted members sit BELOW the bounded display
	// view — informational only (0..Count). POOL2-1 件① (§29.160① ruling):
	// no longer an admission input; a fully-visible significant on-chain
	// family mentions with HiddenCount==0.
	HiddenCount int `json:"hidden_count"`
	// Phase is the own-phase account from the synchronous marker tree (basis
	// self only): present iff every member is a closed tree instance whose
	// clipped inclusive wall equals the member's in-window duration. nil means
	// unavailable, never zero.
	Phase *BusinessSpanPhaseAccount `json:"phase,omitempty"`
}

// BusinessSpanPhaseAccount sums the marker tree accounts of one own-phase
// family's instances: SelfMs is inclusive minus the union of each instance's
// direct children (the tree's own definition), MaxSelfMs the largest single
// instance self, States the scheduler split of the inclusive extents (nil when
// any instance's owner states are unavailable).
type BusinessSpanPhaseAccount struct {
	SelfMs    float64                 `json:"self_ms"`
	MaxSelfMs float64                 `json:"max_single_self_ms"`
	States    *BusinessSpanStateSplit `json:"states,omitempty"`
}

// BusinessSpanStateSplit is the exclusive scheduler split of the summed
// inclusive extents: running + runnable + sleep + d_state + io_wait +
// unknown == the family's TotalMs (stopped/dead time is folded into unknown;
// sleep IO is a subset of sleep and is not carried).
type BusinessSpanStateSplit struct {
	RunningMs  float64 `json:"running_ms"`
	RunnableMs float64 `json:"runnable_ms"`
	SleepMs    float64 `json:"sleep_ms"`
	DStateMs   float64 `json:"d_state_ms"`
	IOWaitMs   float64 `json:"io_wait_ms"`
	UnknownMs  float64 `json:"unknown_ms"`
}

// BusinessSpanMentionIsOwnPhase reports the own-phase tier (basis self); every
// other admitted basis is the dependency-thread tier.
func BusinessSpanMentionIsOwnPhase(basis string) bool {
	return basis == BusinessSpanMentionBasisSelf
}

// BusinessSpanMentionResult is the advisory mention face: the own-phase tier
// then the dependency-thread tier, each selected and ordered on its own by
// TotalMs (desc, ties by StartLine — a deterministic READING order, not an
// ordinal: no rank badge, bar or tier ever attaches), plus the honest count of
// admitted families the selection left out.
type BusinessSpanMentionResult struct {
	Families        []BusinessSpanMention `json:"families"`
	OmittedFamilies int                   `json:"omitted_families,omitempty"`
}

// businessSpanMentionSameMsTol is the identity tolerance between a span's
// in-window duration and its marker tree instance's clipped inclusive wall:
// two computations over the same two timestamps agree to float dust or not at
// all (1µs, rspaAnchorIdentityTolMs precedent — own constant, own meaning).
const businessSpanMentionSameMsTol = 0.001

// computeBusinessSpanMentions builds the advisory mention face. Every
// fail-open exit returns nil (no inventory / no bounded time window / no
// admissible family → the face is simply absent; nothing downstream gates on
// it). The waits another thread's own recorded wakeups ended come from the
// chain's dependency-window carrier (targetWaitDependencyTimeWindowsByPID);
// an absent carrier means none is known and no other thread counts there.
func computeBusinessSpanMentions(q Query, chain ChainResult, chainThreads map[int]bool, stats WindowStats) *BusinessSpanMentionResult {
	inventory := stats.traceSpanFullInventory
	if len(inventory) == 0 {
		return nil
	}
	windowMs := (q.TimeEnd - q.TimeStart) * 1000
	if windowMs <= 0 {
		// No bounded window → no window-share floor (CMP-3 discipline: no
		// window, no estimate) → no mention face.
		return nil
	}
	floorMs := BusinessSpanMentionDustFloorMs
	if share := windowMs * BusinessSpanMentionWindowShareFloor; share > floorMs {
		floorMs = share
	}
	// Bounded-view membership: exact span identity (thread tid, name, line
	// extent) — the precise "already reached the seat machinery" signal.
	type spanIdentity struct {
		tid        int
		name       string
		start, end int
	}
	bounded := make(map[spanIdentity]bool, len(stats.TraceSpans))
	seatedSemanticFamilies := make(map[string]bool, len(stats.TraceSpans))
	for _, s := range stats.TraceSpans {
		seatedSemanticFamilies[semanticSeatFamilyKey(s)] = true
		bounded[spanIdentity{s.Thread.PID, s.Name, s.StartLine, s.EndLine}] = true
	}
	// The typed dependency windows of every thread other than the target:
	// the merged chain node windows [wait start, recorded wakeup] per member
	// (chainAnchorWindowsByPID) united with the analysed thread's waits that
	// thread's own wakeups ended directly — the same carrier, edge finder and
	// geometry that bound the host-edge seats (HMC377 E2, R-377-1). Both are
	// closed by a recorded wakeup; nothing else puts another thread's span on
	// the chain.
	dependencyWindows := chainAnchorWindowsByPID(chain)
	for pid, windows := range targetWaitDependencyTimeWindowsByPID(&chain) {
		if dependencyWindows == nil {
			dependencyWindows = map[int][]TimeWindow{}
		}
		dependencyWindows[pid] = mergeAnchorTimeWindows(append(append([]TimeWindow(nil), dependencyWindows[pid]...), windows...))
	}
	// The analysed thread's marker-tree accounts, indexed from the complete
	// node set — identical whether the thread was addressed by id or by name.
	var ownAccounts map[traceMarkerTreeInstanceKey]traceMarkerTreeInstanceAccount
	if stats.BusinessTree != nil && chain.Target.PID > 0 {
		ownAccounts = traceMarkerTreeTargetAccounts(stats.BusinessTree.allNodes, chain.Target.PID)
	}
	// Per-thread typed credential memo. "" = no credential (never mentioned).
	basisByTid := map[int]string{}
	threadBasis := func(thread ThreadRef) string {
		if basis, seen := basisByTid[thread.PID]; seen {
			return basis
		}
		basis := ""
		switch {
		case chain.Target.PID > 0 && thread.PID == chain.Target.PID:
			basis = BusinessSpanMentionBasisSelf
		case threadInSet(chainThreads, thread):
			basis = BusinessSpanMentionBasisChainMember
		default:
			if _, ok := hostSemanticSpanEdgeAnchor(&chain, thread); ok {
				basis = BusinessSpanMentionBasisHostWakeupEdge
			}
		}
		basisByTid[thread.PID] = basis
		return basis
	}
	type familyKey struct {
		tid  int
		name string
	}
	type familyAcc struct {
		mention    *BusinessSpanMention
		phase      BusinessSpanPhaseAccount
		states     BusinessSpanStateSplit
		phaseOK    bool
		statesOK   bool
		hasMembers bool
	}
	families := map[familyKey]*familyAcc{}
	for _, span := range inventory {
		if span.DurationMs <= 0 || span.Thread.PID <= 0 || span.Name == "" {
			continue
		}
		// OMGCLEAN-1 件5 rider3 (§29.175.5 内容互斥定谳): a typed
		// semantic_class hit is the PRECISE exclusion key — deterministic-
		// optimization families (VerifyClass / JIT / shader / GC / …) are the
		// value dimension: they hold seats and the optimization-point table,
		// never the ◈ name-dimension face (the dual-identity double-count
		// reading is eliminated by construction; the former ◈→席 mutual chip
		// of §29.175.2 is retired with it).
		//
		// The one exception is a family the platform vocabulary classifies
		// that lost every member at the seat bound: it holds no seat, so this
		// face is where its on-chain work stays visible.
		class := firstNonEmpty(strings.TrimSpace(span.SemanticClass), traceSpanSemanticClass(span.Name))
		omittedPlatformFamily := class != "" && semanticSpanMintedByPlatformVocabulary(span.Name) && !seatedSemanticFamilies[semanticSeatFamilyKey(span)]
		if class != "" && !omittedPlatformFamily {
			continue
		}
		basis := threadBasis(span.Thread)
		if basis == "" {
			continue
		}
		if omittedPlatformFamily && basis == BusinessSpanMentionBasisHostWakeupEdge && !semanticSpanAdmitsHostEdgeLane(span.Name) {
			continue
		}
		counted := span.DurationMs
		if basis != BusinessSpanMentionBasisSelf {
			// HMC377 E3: another thread is on the chain only inside its own
			// typed dependency windows (the waits its recorded wakeups
			// ended, directly or through the chain). Its span counts for the
			// part inside them — the typed interval intersection the
			// semantic seats already use — and a span outside every window
			// is temporal proximity and counts nothing. EVOLUTION: the
			// host-edge arm formerly admitted any member lying wholly before
			// the latest credential edge (R3 边前, whole-segment strict,
			// 返工轮 P1-1); a long window let a thread that delivered one
			// wakeup publish every earlier span. Post-edge time still never
			// counts (every ended wait closes at or before that edge), and
			// the raw wall rides SpanWallMs beside the counted part.
			counted = anchorWindowsOverlapMs(dependencyWindows[span.Thread.PID], span.StartTs, span.EndTs)
			if counted <= 0 {
				continue
			}
		}
		key := familyKey{span.Thread.PID, span.Name}
		acc := families[key]
		if acc == nil {
			acc = &familyAcc{mention: &BusinessSpanMention{Thread: span.Thread, Name: span.Name, OnChainBasis: basis,
				StartLine: span.StartLine, EndLine: span.EndLine}, phaseOK: basis == BusinessSpanMentionBasisSelf && ownAccounts != nil, statesOK: true}
			families[key] = acc
		}
		fam := acc.mention
		fam.Count++
		fam.TotalMs += counted
		fam.SpanWallMs += span.DurationMs
		if counted > fam.MaxSingleMs {
			fam.MaxSingleMs = counted
		}
		if span.StartLine > 0 && (fam.StartLine == 0 || span.StartLine < fam.StartLine) {
			fam.StartLine = span.StartLine
		}
		if span.EndLine > fam.EndLine {
			fam.EndLine = span.EndLine
		}
		if !bounded[spanIdentity{span.Thread.PID, span.Name, span.StartLine, span.EndLine}] {
			fam.HiddenCount++
		}
		if acc.phaseOK {
			account, ok := ownAccounts[traceMarkerTreeInstanceKey{source: span.SourcePath, tid: span.Thread.PID, line: span.StartLine}]
			if !ok || math.Abs(account.inclusiveMs-span.DurationMs) > businessSpanMentionSameMsTol {
				acc.phaseOK = false
			} else {
				acc.phase.SelfMs += account.selfMs
				if account.selfMs > acc.phase.MaxSelfMs {
					acc.phase.MaxSelfMs = account.selfMs
				}
				if account.states == nil {
					acc.statesOK = false
				} else {
					v := account.states
					acc.states.RunningMs += v.RunningMs
					acc.states.RunnableMs += v.RunnableMs
					acc.states.SleepMs += v.SleepMs
					acc.states.DStateMs += v.DStateMs
					acc.states.IOWaitMs += v.IOWaitMs
					acc.states.UnknownMs += account.unknownMs + v.StoppedMs + v.DeadMs
				}
			}
		}
		acc.hasMembers = true
	}
	var own, dependency []*BusinessSpanMention
	admittedCount := 0
	for _, acc := range families {
		fam := acc.mention
		if !acc.hasMembers || fam.TotalMs < floorMs {
			continue
		}
		// POOL2-1 件① (§29.160① ruling): the former third gate (≥1 member
		// below the bounded view) is removed — a fully-visible significant
		// on-chain family mentions too (HiddenCount stays informational).
		if acc.phaseOK {
			phase := acc.phase
			if acc.statesOK {
				states := acc.states
				phase.States = &states
			}
			fam.Phase = &phase
		}
		admittedCount++
		if BusinessSpanMentionIsOwnPhase(fam.OnChainBasis) {
			own = append(own, fam)
		} else {
			dependency = append(dependency, fam)
		}
	}
	if admittedCount == 0 {
		return nil
	}
	out := &BusinessSpanMentionResult{}
	selectedCount := 0
	for _, tier := range [][]*BusinessSpanMention{own, dependency} {
		for _, fam := range selectBusinessSpanMentionTier(tier) {
			out.Families = append(out.Families, *fam)
			selectedCount++
		}
	}
	out.OmittedFamilies = admittedCount - selectedCount
	return out
}

// selectBusinessSpanMentionTier applies the selection rule inside one tier
// and returns the selected families in reading order (TotalMs desc, ties by
// StartLine). OMGCLEAN-1 件5 rider3 (§29.175.5 选择规则): the union of the
// per-criterion TOP-K prefixes, deduped — 合计最长 (TotalMs) and 单次最长
// (MaxSingleMs). HMC377 E3 adds 自身最长 for the own-phase tier: nested own
// phases always rank an enclosing wrapper at least as high as the work inside
// it by inclusive time, so the self account (the marker tree's inclusive
// minus its direct children) is the criterion that keeps the work itself. A
// family without a tree account ranks by its inclusive total on that
// criterion (ranking key only; no self value is published for it).
func selectBusinessSpanMentionTier(tier []*BusinessSpanMention) []*BusinessSpanMention {
	if len(tier) == 0 {
		return nil
	}
	byKey := func(key func(*BusinessSpanMention) float64) []*BusinessSpanMention {
		sorted := append([]*BusinessSpanMention(nil), tier...)
		sort.Slice(sorted, func(i, j int) bool {
			a, b := key(sorted[i]), key(sorted[j])
			if a != b {
				return a > b
			}
			if sorted[i].StartLine != sorted[j].StartLine {
				return sorted[i].StartLine < sorted[j].StartLine
			}
			return sorted[i].Thread.PID < sorted[j].Thread.PID
		})
		return sorted
	}
	byTotal := byKey(func(m *BusinessSpanMention) float64 { return m.TotalMs })
	criteria := [][]*BusinessSpanMention{byTotal, byKey(func(m *BusinessSpanMention) float64 { return m.MaxSingleMs })}
	if BusinessSpanMentionIsOwnPhase(tier[0].OnChainBasis) {
		criteria = append(criteria, byKey(func(m *BusinessSpanMention) float64 {
			if m.Phase != nil {
				return m.Phase.SelfMs
			}
			return m.TotalMs
		}))
	}
	selected := map[*BusinessSpanMention]bool{}
	for _, ranked := range criteria {
		for i, fam := range ranked {
			if i >= BusinessSpanMentionFamilyCap {
				break
			}
			selected[fam] = true
		}
	}
	out := make([]*BusinessSpanMention, 0, len(selected))
	for _, fam := range byTotal {
		if selected[fam] {
			out = append(out, fam)
		}
	}
	return out
}
