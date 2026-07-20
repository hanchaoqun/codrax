package context

// trace_board_summary.go — CR-1 件③ 单源摘要喂入 (§29.42.2① + §29.42.3
// precondition, docs/design/real_trace_campaign_20260705.md, 2026-07-12).
//
// The answer-rendering dispatch receives ONE concentrated, authoritative
// summary of the runtime root-cause board: the seated chain-channel rows in
// their engine seat order plus the adjacent-channel (◇) seats, one line each
// with the effective attribution value, its caliber word, the channel
// identity and the row confidence. 41006 witness: the model re-derived a
// third ordering and self-made aggregates ("约45ms") from scattered tool
// output; feeding the typed board removes the reason to invent either
// (§29.42.4: 喂好数据、教好方法、信任模型 — the summary is INPUT, never a
// gate). Volume is deliberately restrained: TOP chain seats + adjacent
// seats only, never a full observation dump.
//
// Sources are typed only: the observation-ledger records the deterministic
// runtime query published (rank / tier / chain_relevance / effective ms rich
// notes). No internal pipeline vocabulary reaches the prompt.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	// traceBoardChainRowCap / traceBoardAdjacentRowCap bound the summary
	// (体积预算克制 — TOP 榜行 + ◇ 榜行, 禁全量 dump).
	traceBoardChainRowCap    = 8
	traceBoardAdjacentRowCap = 4
)

// traceBoardRow is one seated board row parsed from a typed rank record.
type traceBoardRow struct {
	rank        int
	channel     string // "chain" | "adjacent"
	subject     string
	typeToken   string
	tier        string
	effectiveMS string // preformatted value + caliber source word
	caliber     string
	confidence  float64
	// tgid (CR-3 件③ P11, 2026-07-12; 冷读案8 裸线程名死指针): the seat's
	// typed process attribution — "" when the record published none.
	tgid string
	// representativeWindow is the row's FIRST typed occurrence window (CR-2
	// 组③ P7 / F-4): labeled as one occurrence so the model never pairs the
	// whole-window total with a single quoted window. "" when the record
	// published no occurrence windows (absence never guesses).
	representativeWindow string
	// fixDirection (FREQDIR-1 件1, §29.149, 2026-07-19; witness 95946: the
	// board's #1 seat spoke only the bare state word `running`, the model
	// absorbed it into the inversion narrative and its 修复方向 list dropped
	// the #1 direction 频率与热治理 58.320ms): the seat's typed repair-
	// direction DISPLAY word, resolved from the engine-stamped registry token
	// through the single word-face source (tracefence.FixDirectionWord — the
	// same mapping the report display consumes). "" when the record carries
	// no fix_direction note or an unregistered token: an unstamped seat never
	// wears a synthesized direction (absence stays absent, zero fabrication).
	fixDirection string
}

// formatTraceRootCauseBoardFromLedger renders the typed board summary, or ""
// when the run has no seated runtime rank rows (zero-emission anti-noise).
func formatTraceRootCauseBoardFromLedger(ledger types.ObservationLedger) string {
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return ""
	}
	var chain, adjacent []traceBoardRow
	for _, record := range ledger.Records {
		if record.Producer != "trace_query" || !strings.Contains(record.ID, "#root_cause_rank:") {
			continue
		}
		notes := traceBoardNoteMap(record.RichNotes)
		rank, _ := strconv.Atoi(notes[types.TraceNoteKeyRank])
		if rank <= 0 {
			// No board seat (context / symptom / data-gap / background rows)
			// — the board summary carries seats only.
			continue
		}
		row := traceBoardRow{
			rank:       rank,
			subject:    strings.TrimSpace(record.Subject),
			typeToken:  strings.TrimSpace(record.Object),
			tier:       notes[types.TraceNoteKeyTier],
			caliber:    notes[types.TraceNoteKeyMemberFoldCaliber],
			confidence: record.Confidence,
			tgid:       strings.TrimSpace(notes[types.TraceNoteKeyTGID]),
			representativeWindow: traceBoardFirstOccurrenceWindow(
				notes[types.TraceNoteKeyOccurrenceWindows]),
			fixDirection: traceBoardFixDirectionWord(notes[types.TraceNoteKeyFixDirection]),
		}
		value, valueWord := traceBoardEffectiveValue(notes)
		if value == "" {
			continue // a seat without a published magnitude teaches nothing
		}
		row.effectiveMS = value + "ms (" + valueWord + ")"
		switch notes[types.TraceNoteKeyChainRelevance] {
		case "adjacent":
			row.channel = "adjacent"
			adjacent = append(adjacent, row)
		default:
			// Seated rows default to the chain channel (the engine only
			// allocates ordinals on the chain and adjacent channels).
			row.channel = "chain"
			chain = append(chain, row)
		}
	}
	if len(chain) == 0 && len(adjacent) == 0 {
		return ""
	}
	sort.SliceStable(chain, func(i, j int) bool { return chain[i].rank < chain[j].rank })
	sort.SliceStable(adjacent, func(i, j int) bool { return adjacent[i].rank < adjacent[j].rank })
	var b strings.Builder
	b.WriteString("The measured root-cause board below is the single authoritative ordering for this run. State root causes in THIS order; if your combined judgment deviates from it, keep the deviation explicit and say what it is based on — never reorder silently. Use these values verbatim (never sum rows together: they are per-thread wall-clock measurements), and describe each row's role with its own channel below — never demote an on-chain row to background noise. When a row lists representative_window, that window is ONE occurrence among several — the row's value aggregates across the whole query window, so never present the value as the duration of that single window. When a row lists 修向=X, X is that seat's typed repair-direction word published as the 中文 face with its English face in parentheses — one closed registry vocabulary behind both faces, copied verbatim as a pair (this word IS the row's repair semantics, so never re-classify the seat under a different direction by its bare state word): seats sharing one 修向 form ONE repair lane, and that lane's maximum recoverable amount is the direction's LARGEST on-chain seat value — never the seats' sum; adjacent rows are conditional upper bounds and never join the lane maximum. A row without 修向 published no direction; never infer one.\n")
	writeRow := func(row traceBoardRow, channelWord string) {
		line := fmt.Sprintf("- #%d %s — %s · %s · channel=%s · confidence=%.2f", row.rank, channelWord, firstNonEmptyBoardField(row.subject, "(window-level)"), row.typeToken, row.channel, row.confidence)
		if row.tgid != "" {
			// CR-3 件③ P11: the seat's process attribution (tgid) — bare
			// thread names stay traceable to their process on the LLM face.
			line += " · tgid=" + row.tgid
		}
		line += " · " + row.effectiveMS
		if row.caliber != "" {
			line += " · fold=" + row.caliber
		}
		if row.tier != "" {
			line += " · tier=" + row.tier
		}
		if row.fixDirection != "" {
			// FREQDIR-1 件1 (§29.149): the typed repair-direction word,
			// verbatim from the single word-face source — the preamble
			// carries the lane rule (same-direction seats = one repair lane,
			// lane max = the direction's largest seat value).
			line += " · 修向=" + row.fixDirection
		}
		if row.representativeWindow != "" {
			// CR-2 组③ P7 / F-4: the first typed occurrence window, labeled as
			// one occurrence (the preamble carries the pairing rule) — the
			// witnessed misread paired the whole-window total with one window.
			line += " · representative_window=" + row.representativeWindow
		}
		b.WriteString(line + "\n")
	}
	if len(chain) > 0 {
		b.WriteString("On-chain seats (root-cause order):\n")
		for i, row := range chain {
			if i >= traceBoardChainRowCap {
				b.WriteString(fmt.Sprintf("- (+%d more seated rows; see the measured observations)\n", len(chain)-traceBoardChainRowCap))
				break
			}
			writeRow(row, "root-cause seat")
		}
	}
	if len(adjacent) > 0 {
		b.WriteString("Adjacent-impact seats (time-adjacent to the chain, not on it — an independent ordinal space, never merged with the on-chain order):\n")
		for i, row := range adjacent {
			if i >= traceBoardAdjacentRowCap {
				b.WriteString(fmt.Sprintf("- (+%d more adjacent rows; see the measured observations)\n", len(adjacent)-traceBoardAdjacentRowCap))
				break
			}
			writeRow(row, "adjacent seat")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// traceBoardEffectiveValue picks the row's published magnitude with its
// caliber source word, strongest first: effective attribution, cumulative,
// raw impact. Verbatim note values — never recomputed or rounded again.
// RANKDIS-M18 (§29.104.17 裁定② 2026-07-16, 复核件2 勘正): composite-score
// rows publish the *_score twin instead of the ms key (one row emits exactly
// one family). Under the CURRENT closed effective matrix no composite row is
// ever seated on this board (io_pressure zeroes to context_only,
// block_io_by_inode demotes to the ⌗ caliber side), so the score arms below
// are a defensive reservation — they go live only if a future ruling seats a
// composite row, and THAT ruling must also fork the board template's
// unconditional ms suit (row.effectiveMS = value + "ms (…)" above), which
// would otherwise re-dress the score as wall clock. Zero behavior today.
func traceBoardEffectiveValue(notes map[string]string) (value, word string) {
	for _, key := range []string{types.TraceNoteKeyEffectiveImpactMS, types.TraceNoteKeyEffectiveImpactScore} {
		if v := notes[key]; v != "" {
			return v, "effective attribution"
		}
	}
	for _, key := range []string{types.TraceNoteKeyCumulativeImpactMS, types.TraceNoteKeyCumulativeImpactScore} {
		if v := notes[key]; v != "" {
			return v, "cumulative"
		}
	}
	for _, key := range []string{types.TraceNoteKeyImpactMS, types.TraceNoteKeyImpactScore} {
		if v := notes[key]; v != "" {
			return v, "window projection"
		}
	}
	return "", ""
}

// traceBoardFixDirectionWord resolves the engine-stamped fix-direction
// registry token to its display word (FREQDIR-1 件1, §29.149). Single
// word-face source: tracefence.FixDirectionWord — the SAME mapping the
// report's 行2 word, ◎ section heads and legend consume, so the model-input
// face and the report face can never fork on a direction word. POOL2-1 件⑤
// (§29.160⑤ user ruling 2026-07-20 「按推荐的来」= EN 双面并列): the board
// row publishes BOTH language faces as one pair — 「<zh> (<en>)」 — with the
// EN half drawn from the SAME Table ⑦ mapping at zh=false (零二表: no second
// word table exists; an EN answer run copying the pair verbatim carries its
// own face). "" on an absent or unregistered token — absence stays absent
// (zero fabrication); a token resolved on only one face never publishes a
// half-pair.
func traceBoardFixDirectionWord(token string) string {
	zhWord, okZH := tracefence.FixDirectionWord(token, true)
	enWord, okEN := tracefence.FixDirectionWord(token, false)
	if !okZH || !okEN {
		return ""
	}
	return zhWord + " (" + enWord + ")"
}

// traceBoardFirstOccurrenceWindow extracts the FIRST occurrence window from
// the typed occurrence_windows note (windows separated by ';' or ','; each
// window is "start..end") through the shared strict window parser — a
// malformed head yields "" (never a fabricated window).
func traceBoardFirstOccurrenceWindow(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	head := raw
	if cut := strings.IndexAny(head, ";,"); cut >= 0 {
		head = head[:cut]
	}
	head = strings.TrimSpace(head)
	if _, _, ok := types.TraceCausalProjectionParseWindowValue(head); !ok {
		return ""
	}
	return head
}

// traceBoardNoteMap parses the record's k=v rich notes (first writer wins —
// the typed producers emit each key at most once).
func traceBoardNoteMap(notes []string) map[string]string {
	out := map[string]string{}
	for _, note := range notes {
		eq := strings.IndexByte(note, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(note[:eq])
		if _, exists := out[key]; exists {
			continue
		}
		out[key] = strings.TrimSpace(note[eq+1:])
	}
	return out
}

func firstNonEmptyBoardField(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
