package tracequery

// business_span_mention.go — SPANVIS-1: the pure-advisory business-lens span
// mention face (user ruling 2026-07-19, 定形原则 verbatim: 「一个原则就是某个
// span过长,或者某类span高频聚合后过长,并且是链上(含自身)则需要提及,最好是
// 在因果树上和窗内可消除总览上提及。但不参与根因排序,只是从另外一个维度
// (业务视角)给用户指导,可能的优化方向。」).
//
// Admission = (family in-window wall-clock total clears the significance
// floor) ∧ (typed on-chain credential, incl. self) ∧ (at least one member is
// hidden below the bounded display view). Every admission input is a PRECISE
// signal (typed thread identity, typed edge credentials, exact float sums);
// the face itself is soft output only — no seat, no ordinal, no bar, no
// conservation/census membership, no gate reads it.
//
//   - Inventory: WindowStats.traceSpanFullInventory — the FULL pre-bound span
//     inventory (§29.137 LOCKSPAN 调查 / §29.143 备案: the display top-8 cap
//     is a display budget; the seat/carve machinery keeps consuming the
//     bounded view byte-identically, and the full inventory feeds ONLY this
//     mention face). nil inventory → nil result (fail-open).
//   - Family key: (thread tid, verbatim span name) — 任意名聚族, no lexicon.
//   - On-chain (含自身), existing credential machinery only (禁自造第二套边
//     判定): self = chain.Target; chain member = wakeupChainThreadSet; host =
//     hostSemanticSpanEdgeAnchor R3 edge credential, and only members lying
//     ENTIRELY before the latest credential edge count (边前=有效/边后=解除;
//     a boundary-crossing span is excluded whole — no bisected values on
//     this verbatim face). A thread with NO typed credential is never
//     mentioned (硬纪律).
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

import "sort"

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

// BusinessSpanMentionFamilyCap bounds the mention face to the top families by
// in-window total (SPANTOP-1 cap=3 家风). Families admitted beyond the cap
// are counted honestly in OmittedFamilies (件3 截断诚实披露) — only admitted
// (≥floor) families count there, so the disclosure itself stays noise-free
// (微窗裁定).
const BusinessSpanMentionFamilyCap = 3

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
	// in-window typed wakeup edge toward the target (R3 credential); only
	// pre-edge members counted.
	BusinessSpanMentionBasisHostWakeupEdge = "host_wakeup_edge"
)

// BusinessSpanMention is one admitted (thread, verbatim span name) family on
// the advisory mention face. Values are raw in-window projections summed over
// the admitted member set — never envelopes, never re-scaled.
type BusinessSpanMention struct {
	Thread ThreadRef `json:"thread"`
	// Name is the verbatim span name (typed family key — 任意名聚族).
	Name string `json:"name"`
	// Count / TotalMs / MaxSingleMs: admitted member count, Σ of their
	// in-window DurationMs, and the largest single member (双杠杆线索 typed
	// fact trio: 次数多而单段小 vs 单段长).
	Count       int     `json:"count"`
	TotalMs     float64 `json:"total_ms"`
	MaxSingleMs float64 `json:"max_single_ms"`
	// StartLine/EndLine: the member line envelope (min start .. max end) —
	// the mention row's 行a..b pointer (no seat, so no evidence tag).
	StartLine int `json:"start_line,omitempty"`
	EndLine   int `json:"end_line,omitempty"`
	// OnChainBasis ∈ {self, chain_member, host_wakeup_edge} (closed set).
	OnChainBasis string `json:"on_chain_basis"`
	// HiddenCount is how many admitted members sit BELOW the bounded display
	// view (≥1 by admission — a family whose every member already reached the
	// seat machinery's view is not re-listed here; XLANE-2 归口: 不再第三面
	// 抄成员).
	HiddenCount int `json:"hidden_count"`
}

// BusinessSpanMentionResult is the advisory mention face: at most
// BusinessSpanMentionFamilyCap families ordered by TotalMs (desc, ties by
// StartLine — a deterministic READING order, not an ordinal: no rank badge,
// bar or tier ever attaches), plus the honest count of admitted families the
// cap left out.
type BusinessSpanMentionResult struct {
	Families        []BusinessSpanMention `json:"families"`
	OmittedFamilies int                   `json:"omitted_families,omitempty"`
}

// computeBusinessSpanMentions builds the advisory mention face. Every
// fail-open exit returns nil (no inventory / no bounded time window / no
// admissible family → the face is simply absent; nothing downstream gates on
// it).
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
	for _, s := range stats.TraceSpans {
		bounded[spanIdentity{s.Thread.PID, s.Name, s.StartLine, s.EndLine}] = true
	}
	// Per-thread typed credential memo. "" = no credential (never mentioned).
	basisByTid := map[int]string{}
	boundaryByTid := map[int]float64{}
	threadBasis := func(thread ThreadRef) (string, float64) {
		if basis, seen := basisByTid[thread.PID]; seen {
			return basis, boundaryByTid[thread.PID]
		}
		basis, boundary := "", 0.0
		switch {
		case chain.Target.PID > 0 && thread.PID == chain.Target.PID:
			basis = BusinessSpanMentionBasisSelf
		case threadInSet(chainThreads, thread):
			basis = BusinessSpanMentionBasisChainMember
		default:
			if anchor, ok := hostSemanticSpanEdgeAnchor(&chain, thread); ok {
				basis = BusinessSpanMentionBasisHostWakeupEdge
				boundary = anchor.boundaryTs
			}
		}
		basisByTid[thread.PID] = basis
		boundaryByTid[thread.PID] = boundary
		return basis, boundary
	}
	type familyKey struct {
		tid  int
		name string
	}
	families := map[familyKey]*BusinessSpanMention{}
	for _, span := range inventory {
		if span.DurationMs <= 0 || span.Thread.PID <= 0 || span.Name == "" {
			continue
		}
		basis, boundary := threadBasis(span.Thread)
		if basis == "" {
			continue
		}
		if basis == BusinessSpanMentionBasisHostWakeupEdge && (span.StartTs >= boundary || span.EndTs > boundary) {
			// R3 semantics, whole-segment strict (返工轮 P1-1, 对抗官实证):
			// only members lying ENTIRELY before the credential edge carry it
			// (边前=有效 / 边后=解除). A boundary-crossing span is EXCLUDED
			// whole — bisecting it would mint a derived value, and this face
			// publishes ONLY verbatim inventory segments (原始值纪律; the
			// seat machinery's R3 bisection lane keeps that job). The old
			// StartTs-only gate let post-edge time leak into a row whose
			// credential word says 边前.
			continue
		}
		key := familyKey{span.Thread.PID, span.Name}
		fam := families[key]
		if fam == nil {
			fam = &BusinessSpanMention{Thread: span.Thread, Name: span.Name, OnChainBasis: basis,
				StartLine: span.StartLine, EndLine: span.EndLine}
			families[key] = fam
		}
		fam.Count++
		fam.TotalMs += span.DurationMs
		if span.DurationMs > fam.MaxSingleMs {
			fam.MaxSingleMs = span.DurationMs
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
	}
	admitted := make([]*BusinessSpanMention, 0, len(families))
	for _, fam := range families {
		if fam.TotalMs < floorMs {
			continue
		}
		if fam.HiddenCount == 0 {
			// Every member already reached the bounded view — the seat
			// machinery's faces list it; a mention row would be a pure copy.
			continue
		}
		admitted = append(admitted, fam)
	}
	if len(admitted) == 0 {
		return nil
	}
	sort.Slice(admitted, func(i, j int) bool {
		if admitted[i].TotalMs != admitted[j].TotalMs {
			return admitted[i].TotalMs > admitted[j].TotalMs
		}
		return admitted[i].StartLine < admitted[j].StartLine
	})
	kept := len(admitted)
	if kept > BusinessSpanMentionFamilyCap {
		kept = BusinessSpanMentionFamilyCap
	}
	out := &BusinessSpanMentionResult{Families: make([]BusinessSpanMention, 0, kept), OmittedFamilies: len(admitted) - kept}
	for _, fam := range admitted[:kept] {
		out.Families = append(out.Families, *fam)
	}
	return out
}
