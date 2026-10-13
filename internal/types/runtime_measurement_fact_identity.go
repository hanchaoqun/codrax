package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
)

// The result envelope may change (for example, an inherited-window diagnostic)
// without changing any native facts. Reuse is restricted to a complete native
// population and the entire publication, never a matching row prefix. Source
// freshness is checked by the caller before supplying the private read receipt.
func qualifyRuntimeMeasurementFactIdentity(p *RuntimeMeasurementPublication, original string, replay TraceQueryWindowReplayRef) {
	if p == nil {
		return
	}
	for i := range p.Tables {
		p.Tables[i].nativeFactIdentity = ""
	}
	if original == "" {
		return
	}
	ref := replay.source
	if replay.preparedSource != nil {
		ref = replay.preparedSource.source
	} else if replay.nativeNavigation != nil {
		ref = replay.nativeNavigation.source
	}
	if ref.generation == nil || ref.path != p.Source.Path {
		return
	}
	complete := false
	for _, table := range p.Tables {
		// This reuse lane is display-only. Existing private completion scopes
		// and log-roster receipts keep their independent identity contracts.
		if table.MemberSet != nil || table.coverageScope != nil || table.nativeLogDisplayPopulation {
			return
		}
		complete = complete || table.HasCompleteNativeDisplayPopulation()
	}
	if !complete {
		return
	}
	// Remove only the exact result-envelope coordinate from producer-owned
	// identifiers. Retain the complete query hash, recursive/frame scope and
	// line scope; do not infer query identity from displayed values or notes.
	envelope := filepath.Base(p.Source.PayloadRef)
	query, ok := strings.CutPrefix(p.Source.QueryScopeID, envelope+":")
	observation, own := strings.CutPrefix(p.ObservationID, "trace_query:"+envelope)
	if !ok || !own || query == "" || observation == "" {
		return
	}
	canonical := *p
	canonical.ObservationID = observation
	canonical.Source.RawRef, canonical.Source.PayloadRef = "", ""
	canonical.Source.QueryScopeID = query
	canonical.Tables = make([]RuntimeMeasurementTable, len(p.Tables))
	for i, table := range p.Tables {
		canonical.Tables[i] = table.Clone()
		canonical.Tables[i].ObservationID = observation
		if population := canonical.Tables[i].DisplayPopulation; population != nil {
			if population.QueryScopeID != p.Source.QueryScopeID {
				return
			}
			population.QueryScopeID = query
		}
	}
	raw, err := json.Marshal(struct {
		Original, Generation string
		Publication          RuntimeMeasurementPublication
	}{original, ref.identity.CacheToken(), canonical})
	if err != nil {
		return
	}
	digest := sha256.Sum256(raw)
	key := hex.EncodeToString(digest[:])
	for i := range p.Tables {
		p.Tables[i].nativeFactIdentity = key
	}
}

// RuntimeMeasurementDisplayKey links exact aliases only for presentation. It
// does not alter selectors, source records, member coverage or replay rights.
// An unqualified table retains the original observation/view identity.
func RuntimeMeasurementDisplayKey(receipt AnswerRuntimeMeasurementReceipt, contract *RuntimeMeasurementContract) string {
	bound := BindRuntimeMeasurementReceipt(&receipt, contract)
	if bound && receipt.BoundTable.EventData != nil && receipt.EventSelection != nil {
		// A selected set/group cannot suppress the default full record table.
		// This is exact display identity only, never an enumeration witness.
		return receipt.ObservationID + "\x00" + string(receipt.View) + "\x00event-selection:" + runtimeEventSelectionKey(receipt.EventSelection)
	}
	if bound && receipt.BoundTable.nativeFactIdentity != "" {
		return "native-population:" + receipt.BoundTable.nativeFactIdentity + "\x00" + string(receipt.View)
	}
	return receipt.ObservationID + "\x00" + string(receipt.View)
}
