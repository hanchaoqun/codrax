package tracequery

// rank_item_wire_m18.go — RANKDIS-M18 (§29.104.16.1 M18, user ruling
// §29.104.17 ② 2026-07-16): a composite-score rank row's value slots leave
// the ms-semantic JSON keys. B1241 further gives io_pressure its dedicated
// activity_index authority, because its background mixed-unit value is not a
// window projection or chain account. block_io_by_inode retains the generic
// composite-score family. Publishing either family under *_ms let a raw grep
// over the payload read a score as wall-clock milliseconds.
//
// Pure wire-key fork: the Go fields, every sort/tier/score lane and every
// non-composite row are byte-identical — the fork lives exclusively at JSON
// marshal time, gated on the registry wire arm CausalTokenCompositeValueWire
// (precise typed token membership, never a heuristic). Census (M18 batch,
// 2026-07-17) found zero in-repo json.Unmarshal readers of the rank-item
// payload, so the rename ships with no compatibility arm (user ruling: 零 Go
// 读者即干净改名零兼容臂).
//
// target_impact_ms / actual_impact_ms / actual_total_ms deliberately keep the
// ms suit on every row shape: they are physical wall-clock ledgers by their
// family definition (QH2-A 口径分离 — a composite row never mints them, and
// if one ever does the value is wall clock).

import "encoding/json"

// rootCauseRankItemWireAlias strips RootCauseRankItem's method set so the
// MarshalJSON below can delegate to encoding/json without recursing.
type rootCauseRankItemWireAlias RootCauseRankItem

// rootCauseRankItemCompositeWire is the composite-score wire shape: the four
// ms-semantic value slots are suppressed and re-published under the *_score
// keys. Suppression works by JSON-name shadowing — the depth-0 twins reuse
// the exact embedded tags and stay zero, so encoding/json elects them over
// the embedded fields and omitempty drops the key (a `json:"-"` field would
// be removed from conflict resolution entirely and the embedded ms key would
// leak back onto the wire).
type rootCauseRankItemCompositeWire struct {
	rootCauseRankItemWireAlias
	ImpactMs           float64 `json:"impact_ms,omitempty"`
	ProjectedImpactMs  float64 `json:"projected_impact_ms,omitempty"`
	CumulativeImpactMs float64 `json:"cumulative_impact_ms,omitempty"`
	EffectiveImpactMs  float64 `json:"effective_impact_ms,omitempty"`

	ImpactScore           float64 `json:"impact_score,omitempty"`
	ProjectedImpactScore  float64 `json:"projected_impact_score,omitempty"`
	CumulativeImpactScore float64 `json:"cumulative_impact_score,omitempty"`
	EffectiveImpactScore  float64 `json:"effective_impact_score,omitempty"`
}

// rootCauseRankItemIOPressureWire keeps the engine's private ranking/capping
// fields out of the public io_pressure context payload. The only published
// magnitude is the authoritative mixed-unit activity index. The underlying Go
// fields remain unchanged for engine compatibility; this is a wire authority
// split, not a ranking change.
type rootCauseRankItemIOPressureWire struct {
	rootCauseRankItemWireAlias
	ImpactMs           float64 `json:"impact_ms,omitempty"`
	ProjectedImpactMs  float64 `json:"projected_impact_ms,omitempty"`
	CumulativeImpactMs float64 `json:"cumulative_impact_ms,omitempty"`
	EffectiveImpactMs  float64 `json:"effective_impact_ms,omitempty"`
	Score              float64 `json:"score,omitempty"`

	ImpactScore           float64 `json:"impact_score,omitempty"`
	ProjectedImpactScore  float64 `json:"projected_impact_score,omitempty"`
	CumulativeImpactScore float64 `json:"cumulative_impact_score,omitempty"`
	EffectiveImpactScore  float64 `json:"effective_impact_score,omitempty"`
	ActivityIndex         float64 `json:"activity_index,omitempty"`
}

// MarshalJSON forks the value-slot key family on composite-score rows only.
// Non-members marshal through the plain alias — field set, order and bytes
// identical to the pre-M18 shape.
func (item RootCauseRankItem) MarshalJSON() ([]byte, error) {
	if !CausalTokenCompositeValueWire(item.Type) {
		return json.Marshal(rootCauseRankItemWireAlias(item))
	}
	if item.Type == "io_pressure" {
		return json.Marshal(rootCauseRankItemIOPressureWire{
			rootCauseRankItemWireAlias: rootCauseRankItemWireAlias(item),
			ActivityIndex:              item.CumulativeImpactMs,
		})
	}
	return json.Marshal(rootCauseRankItemCompositeWire{
		rootCauseRankItemWireAlias: rootCauseRankItemWireAlias(item),
		ImpactScore:                item.ImpactMs,
		ProjectedImpactScore:       item.ProjectedImpactMs,
		CumulativeImpactScore:      item.CumulativeImpactMs,
		EffectiveImpactScore:       item.EffectiveImpactMs,
	})
}
