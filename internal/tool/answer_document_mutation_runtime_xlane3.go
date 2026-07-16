package tool

// XLANE-3 件3 (§29.104.2 定谳③ + §29.104.9 形③, real_trace_campaign_20260705.md,
// 2026-07-16): the cross-board same-thread same-state-family reconciliation.
//
// A multi-step report fuses several rank BOARDS (typed triple identity: query
// window × board target × params fingerprint) into one projection, and one
// physical thread's one state family can hold seats on several boards — the
// donghu 形③ witness: logd.writer's runnable-family board seats (调度延迟
// 40.071 全额 / runnable folds) on the tid-9163 board coexist with its ◇
// window-census seat on the tid-2955 board, Σ ≈ 2.35× the thread's physical
// wall clock. Every value stays untouched (值通道零修改): each seat gains ONE
// typed mutual-pointer sentence naming the peer board's seats, so the reader
// can never add the two boards' accounts (宁互指勿折叠 — true µs twins already
// fold through the ordinary R1 same-fact merge upstream of this pass).
//
// 修补轮 (2026-07-16): the board key derives from the ONE shared identity
// index (runtimeTraceProjRankBoardIndexFor — the former local key format was
// a second implementation; partial-fingerprint rows are now judged the same
// on every face), and the population keeps two EXPLICIT gates on top of it:
// a resolvable chip window (G4 完整板身份 — a windowless seat never pairs and
// never mints a phantom 0..0 board) and a non-empty typed board target
// (identity-less rows never pair, 宁漏勿假指).

import (
	"sort"
	"strings"
)

// runtimeTraceProjCrossBoardFamilyRefCap bounds the per-row peer-seat ref list
// (the remainder stays countable through CrossBoardFamilyMoreCount).
const runtimeTraceProjCrossBoardFamilyRefCap = 2

// runtimeTraceProjCrossBoardAnchorLabel names a peer board for the sentence:
// the board target label, plus the params fingerprint half exactly when the
// two boards share the target and differ only in knobs (the anchor must
// disambiguate as far as the identity does). 修补轮 件H① (2026-07-16): the
// params half is per-language (the zh 「·参数#」 leaked verbatim into EN
// sentences) — same word pair as the seat chip's params half.
func runtimeTraceProjCrossBoardAnchorLabel(row *runtimeTraceProjTreeRow, sameTargetDiffParams, zh bool) string {
	label := strings.TrimSpace(row.Node.RankBoardTarget)
	if sameTargetDiffParams {
		if fp := strings.TrimSpace(row.Node.RankBoardParamsFingerprint); fp != "" {
			if zh {
				label += "·参数#" + fp
			} else {
				label += " · params #" + fp
			}
		}
	}
	return label
}

// runtimeTraceProjStampCrossBoardFamilyNotes stamps the 件3 mutual pointers.
// Population per group: seat rows only (Rank>0 through the shared
// rank/confidence resolver), wall-clock rows only (两把尺), typed state
// family non-empty, evidence tag registered, and a COMPLETE typed board
// identity (resolvable chip window + non-empty board target; the board KEY
// itself is the shared triple index). Groups key on (canonical subject,
// state family); a group whose rows span ≥2 distinct board keys stamps every
// row with the OTHER boards' refs — both directions by construction. Rows
// already relating to a peer through the account-relation/value-mirror/
// non-additive arms or the cross-channel same-thread pointers skip THAT peer
// (one relation, one sentence — the double-tag lesson; 修补轮 件C: the
// cross-channel 「本线程另有链上/邻近/口径旁栏席」 pointer already names the
// peer, so the cross-board sentence must not re-hang the same ref).
func runtimeTraceProjStampCrossBoardFamilyNotes(model *runtimeTraceProjTreeModel, zh bool) {
	if model == nil {
		return
	}
	var population []*runtimeTraceProjTreeRow
	for _, row := range runtimeTraceProjSMR1AllRows(model) {
		if !row.HasData || strings.TrimSpace(row.EvidenceTag) == "" {
			continue
		}
		if rank, _ := runtimeTraceProjCauseRankConfidence(*row); rank <= 0 {
			continue
		}
		if row.SeatOrdinalStale || !runtimeTraceProjSMR1WallClockRow(row.Node) {
			continue
		}
		if runtimeTraceProjSMR1StateFamily(row.Node) == "" {
			continue
		}
		if runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) == "" {
			continue
		}
		// G4 完整板身份: window + target both required (the shared index
		// would otherwise inherit a single window cluster for a windowless
		// row — pairing is a stronger claim than grouping and keeps the
		// stricter gate).
		if _, _, ok := runtimeTraceProjRankChipWindow(row.Node); !ok {
			continue
		}
		if strings.TrimSpace(row.Node.RankBoardTarget) == "" {
			continue
		}
		population = append(population, row)
	}
	boardIDs := runtimeTraceProjStableRankBoardIDs(population)
	type member struct {
		row      *runtimeTraceProjTreeRow
		boardKey string
	}
	groups := map[string][]member{}
	var order []string
	for _, row := range population {
		family := runtimeTraceProjSMR1StateFamily(row.Node)
		subject := runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject)
		key := subject + "\x00" + family
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], member{row: row, boardKey: boardIDs[row]})
	}
	sort.Strings(order)
	for _, key := range order {
		members := groups[key]
		boards := map[string]bool{}
		targets := map[string]bool{}
		for _, m := range members {
			boards[m.boardKey] = true
			targets[strings.TrimSpace(m.row.Node.RankBoardTarget)] = true
		}
		if len(boards) < 2 {
			continue
		}
		// Same-target multi-board groups exist only through the params half —
		// the peer anchor label must then carry the fingerprint.
		sameTargetDiffParams := len(targets) < len(boards)
		for _, m := range members {
			var refs []string
			peerBoards := map[string]bool{}
			var peerOrder []string
			more := 0
			for _, peer := range members {
				if peer.boardKey == m.boardKey {
					continue
				}
				tag := strings.TrimSpace(peer.row.EvidenceTag)
				// One relation, one sentence: a peer this row already relates
				// to through another typed arm keeps that arm's sentence
				// (件C: the cross-channel pointers count — 「本线程另有邻近席
				// [E42]」 beside 「同线程同状态族账另见另板席 …[E42]」 was the
				// witnessed double-tag).
				if tag == m.row.AccountRelRef || tag == m.row.NonAdditiveRef ||
					tag == m.row.ValueMirrorRef || tag == m.row.CrossChannelChainRef ||
					tag == m.row.CrossChannelAdjacentRef || tag == m.row.CrossChannelCaliberRef {
					continue
				}
				if len(refs) < runtimeTraceProjCrossBoardFamilyRefCap {
					refs = append(refs, tag)
				} else {
					more++
				}
				if label := runtimeTraceProjCrossBoardAnchorLabel(peer.row, sameTargetDiffParams, zh); label != "" && !peerBoards[label] {
					peerBoards[label] = true
					peerOrder = append(peerOrder, label)
				}
			}
			if len(refs) == 0 {
				continue
			}
			m.row.CrossBoardFamilyRefs = refs
			m.row.CrossBoardFamilyMoreCount = more
			m.row.CrossBoardFamilyPeerBoards = peerOrder
		}
	}
}
