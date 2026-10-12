package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// A paired business interval owns its state account independently of the query
// view/target that discovered it. Reuse requires the entire native summary,
// exact physical span, owner, source generation, clock and enclosing window.
// This is not a population query and cannot carry roster/completion authority.
func qualifyRuntimeBusinessMeasurementIdentity(p *RuntimeMeasurementPublication, r ObservationRecord, original string, replay TraceQueryWindowReplayRef) {
	if p == nil || original == "" || r.Predicate != TraceBusinessSpanPredicate || len(p.Tables) != 1 ||
		r.Subject == "" || r.Object == "" || r.Span.EndTs <= r.Span.StartTs || r.Span.LineStart <= 0 || r.Span.LineEnd <= r.Span.LineStart {
		return
	}
	table := p.Tables[0]
	if table.View != RuntimeMeasurementSummary || table.MemberSet != nil || table.DisplayPopulation != nil || table.coverageScope != nil || table.nativeLogDisplayPopulation {
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
	canonical := *p
	canonical.ObservationID = ""
	canonical.Source.RawRef, canonical.Source.PayloadRef, canonical.Source.QueryScopeID = "", "", ""
	// Query target is a search selector, not this marker's owner. Owner and
	// exact paired coordinates remain in the key below; query window stays.
	canonical.Source.QueryTargetPID, canonical.Source.QueryTargetThread, canonical.Source.QueryTargetScope = 0, "", ""
	canonical.Tables = []RuntimeMeasurementTable{table.Clone()}
	canonical.Tables[0].ObservationID = ""
	raw, err := json.Marshal(struct {
		Original, Generation, Predicate, Subject, Object string
		Span                                             ObservationSpan
		Publication                                      RuntimeMeasurementPublication
	}{original, ref.identity.CacheToken(), r.Predicate, r.Subject, r.Object, r.Span, canonical})
	if err != nil {
		return
	}
	sum := sha256.Sum256(raw)
	p.Tables[0].nativeFactIdentity = "business:" + hex.EncodeToString(sum[:])
}
