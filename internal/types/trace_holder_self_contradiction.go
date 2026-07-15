package types

import "fmt"

// TraceHolderSelfContradictionWitness (G10-EN 根修, QH2-A 2026-07-14; ledger
// real_trace_campaign_20260705.md §27.4-G10 → §28.7 留账「EN 报告面现携中文
// witness,根修=witness typed 字段化两 lane 各自措辞」): the typed COMPONENTS
// of the same-lock self-contradiction demotion witness (P0-E 锁车道修2,
// §24.9-C F2). The engine used to mint the witness as ONE zh sentence that
// rode verbatim into the EN report faces (the EN detail stanza and the EN
// engine summary each concatenated the zh body). The components now travel
// typed and every lane words its own sentence from them through WitnessText —
// the zh face stays byte-identical to the legacy mint BY CONSTRUCTION (single
// wording source), the EN face is a full English sentence.
//
// Exactly the components of the legacy sentence, no more (禁多铸): the
// inferred-holder thread label, the payload owner tid both parties queued on,
// the holder's own same-owner queued overlap, the attributed span, and the
// contradicting span's line range. §22.2.1 词条尺子: payload/tid stay
// untranslated on both lanes; number and line formats byte-preserved
// (%.3fms, 行 %d-%d / lines %d-%d).
type TraceHolderSelfContradictionWitness struct {
	// Holder is the withdrawn inferred holder's thread label (comm-pid).
	Holder string `json:"holder"`
	// OwnerTid is the payload owner tid the holder itself was queued on.
	OwnerTid int `json:"owner_tid"`
	// QueuedMs is the holder's own same-owner contention overlap with the
	// attributed span (wall-clock ms).
	QueuedMs float64 `json:"queued_ms"`
	// SpanMs is the attributed span's duration (wall-clock ms).
	SpanMs float64 `json:"span_ms"`
	// LineStart / LineEnd locate the holder's contradicting contention span.
	LineStart int `json:"line_start"`
	LineEnd   int `json:"line_end"`
}

// WitnessText words the witness for one report lane. The zh form is the
// byte-frozen legacy engine mint (TestLockHolderSelfContradictionWitnessIsChineseG10
// pins it); the EN form carries the same facts as a full English sentence.
func (w TraceHolderSelfContradictionWitness) WitnessText(zh bool) string {
	if zh {
		return fmt.Sprintf("推断持有者 %s 自身在同一 payload 持有者 tid %d 上排队 %.3fms(本段共 %.3fms;行 %d-%d)",
			w.Holder, w.OwnerTid, w.QueuedMs, w.SpanMs, w.LineStart, w.LineEnd)
	}
	return fmt.Sprintf("inferred holder %s was itself queued on the same payload owner tid %d for %.3fms (of the %.3fms span; lines %d-%d)",
		w.Holder, w.OwnerTid, w.QueuedMs, w.SpanMs, w.LineStart, w.LineEnd)
}
