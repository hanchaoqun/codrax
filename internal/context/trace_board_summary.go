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
	b.WriteString("The measured root-cause board below is the single authoritative ordering for this run. State root causes in THIS order; if your combined judgment deviates from it, keep the deviation explicit and say what it is based on — never reorder silently. Use these values verbatim (never sum rows together: they are per-thread wall-clock measurements), and describe each row's role with its own channel below — never demote an on-chain row to background noise.\n")
	writeRow := func(row traceBoardRow, channelWord string) {
		line := fmt.Sprintf("- #%d %s — %s · %s · channel=%s · confidence=%.2f", row.rank, channelWord, firstNonEmptyBoardField(row.subject, "(window-level)"), row.typeToken, row.channel, row.confidence)
		line += " · " + row.effectiveMS
		if row.caliber != "" {
			line += " · fold=" + row.caliber
		}
		if row.tier != "" {
			line += " · tier=" + row.tier
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
func traceBoardEffectiveValue(notes map[string]string) (value, word string) {
	if v := notes[types.TraceNoteKeyEffectiveImpactMS]; v != "" {
		return v, "effective attribution"
	}
	if v := notes[types.TraceNoteKeyCumulativeImpactMS]; v != "" {
		return v, "cumulative"
	}
	if v := notes[types.TraceNoteKeyImpactMS]; v != "" {
		return v, "window projection"
	}
	return "", ""
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
