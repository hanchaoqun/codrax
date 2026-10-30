package types

import (
	"bytes"
	"encoding/json"
)

// HasNativeMeasurementFacts grants only a pure display surface. Its receipt is
// minted after complete-publication conflict checks and current source/window
// binding, not from table shape, an observation ID or a serialized selector.
func (t RuntimeMeasurementTable) HasNativeMeasurementFacts() bool {
	return t.nativeMeasurementFactsDigest != "" && t.nativeMeasurementFactsDigest == runtimeSamplingDigest(t)
}

// Both the ordinary query and composed-query paths call this only after their
// private current-source validation. Composition must preserve the producer
// predicate: a generic display witness cannot replace a dedicated fact seal.
func qualifyCurrentNativeMeasurementDisplay(p *RuntimeMeasurementPublication, predicate, original string, replay TraceQueryWindowReplayRef, scope *RuntimeArtifactScopeProfile, cancelled bool) {
	if p == nil || !RuntimeMeasurementPredicateIsRegistered(predicate) {
		return
	}
	qualifyNativeAnswerDisplayPublication(p, original, replay, scope)
	p.nativeFixedDisplay = p.answerDisplayIdentity != "" && !cancelled && nativeMeasurementGenericDisplayPredicate(predicate)
}

func nativeMeasurementGenericDisplayPredicate(predicate string) bool {
	return RuntimeMeasurementPredicateIsRegistered(predicate) && !rawMeasurementPredicate(predicate) &&
		predicate != RuntimeSamplingMeasurementPredicate && predicate != RuntimeResourceMeasurementPredicate &&
		predicate != TraceEventSearchInventoryPredicate
}

func qualifyNativeMeasurementFacts(c *RuntimeMeasurementContract, publications []RuntimeMeasurementPublication) {
	eligible := make(map[string]bool)
	for _, p := range publications {
		if p.nativeFixedDisplay && p.answerDisplayIdentity != "" && c.answerDisplayIdentities[p.ObservationID] == p.answerDisplayIdentity {
			eligible[p.ObservationID] = true
		}
	}
	for i := range c.Tables {
		table := &c.Tables[i]
		// Existing producer-specific facts keep their own qualification. A fact
		// surface never borrows event selection, member or relationship authority.
		if eligible[table.ObservationID] && table.EventData == nil && table.IsValid() && !table.HasNativeFixedFacts() {
			table.nativeMeasurementFactsDigest = runtimeSamplingDigest(*table)
		}
	}
}

// Decode into a fresh value even when the caller reuses a previously bound
// table. JSON carries public data only; private current-dispatch receipts may
// survive Clone, but must never survive unmarshalling over an existing value.
func (t *RuntimeMeasurementTable) UnmarshalJSON(data []byte) error {
	type wire RuntimeMeasurementTable
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*t = RuntimeMeasurementTable(decoded)
	return nil
}

const RuntimeMeasurementFactDisplayTeaching = "Current source-bound measurements also support summary/table/diagram through the published runtime_measurement pair alone. The system reuses the complete producer rows and scope notes; omit competing text/items/columns/diagram and causal metadata. Keep each source, subject, layer, operation, population and window explicit. Derived arithmetic follows the shared scoped-arithmetic rule, with compatible units and proven population membership; a derived subtotal does not replace the original groups or imply an exhaustive total. A calculation does not establish physical correspondence or make overlapping request layers additive. Fact diagrams have no relationship edges. Interpretation belongs in a separate block; these measurements alone do not prove hardware saturation, execution state, thread waiting or a response cause."
