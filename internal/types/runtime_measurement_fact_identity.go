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
		p.Tables[i].nativeDisplayDataIdentity = ""
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
	query, observation, valid := RuntimeMeasurementPublicationDisplayCoordinates(*p)
	if !valid {
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

// RuntimeMeasurementPublicationDisplayCoordinates removes only a result's
// transport envelope for display comparison. Both consumers retain the full
// query hash, recursive/frame and line scopes. It supplies no source freshness,
// field equivalence, population, completion or causal authority; those gates
// remain the caller's responsibility. Never infer identity from display text.
func RuntimeMeasurementPublicationDisplayCoordinates(p RuntimeMeasurementPublication) (query, observation string, valid bool) {
	envelope := filepath.Base(p.Source.PayloadRef)
	query, queryOK := strings.CutPrefix(p.Source.QueryScopeID, envelope+":")
	observation, own := strings.CutPrefix(p.ObservationID, "trace_query:"+envelope)
	if !own {
		// Only the exact new full-digest grammar supplies a local suffix.
		// Legacy coordinates keep their existing parsing; aliases do not.
		coordinate, versioned := strings.CutPrefix(p.ObservationID, "trace_query:publication-")
		digest, suffix, separated := strings.Cut(coordinate, "#")
		decoded, err := hex.DecodeString(digest)
		if !versioned || !separated || len(digest) != 2*sha256.Size || err != nil || len(decoded) != sha256.Size || suffix == "" {
			return "", "", false
		}
		observation, own = "#"+suffix, true
	}
	return query, observation, queryOK && own && query != "" && observation != ""
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
