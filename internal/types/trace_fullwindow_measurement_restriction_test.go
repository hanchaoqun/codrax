package types

import "testing"

func TestFullWindowStateReferenceDoesNotBorrowAnotherEventSelection(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		root := rn12RunnablePrimary()
		local := rn12StateDrilldown(root.Subject, "runnable", "1200.000")
		foreign := rn12StateDrilldown(root.Subject, "runnable", "2500.000")
		foreign.ID += ":filtered"
		attach := func(r *ObservationRecord, name string, start, end int) {
			origin := schedulerRestrictionTestNode(name, start, end, 1).MeasurementOrigins[0]
			origin.MeasurementSources.Domains[0].TargetTID = 49706
			origin.MeasurementSources.Domains[0].WindowStartTs, origin.MeasurementSources.Domains[0].WindowEndTs = 100, 103
			r.SourceRef, r.ObservedAt, r.MeasurementSources = origin.SourceRef, origin.ObservedAt, origin.MeasurementSources
		}
		attach(&root, "root", 0, 0)
		attach(&local, "local", 0, 0)
		attach(&foreign, "foreign", 1, 400)
		records := []ObservationRecord{root, local, foreign}
		if reverse {
			records[1], records[2] = records[2], records[1]
		}
		projection := TraceCausalProjectionFromObservationRecords(records)
		node := traceProjectionFindNodeBySubject(projection.PrimaryRootCauses, root.Subject)
		if node == nil {
			t.Fatal("real compile lost primary")
		}
		if node.FullWindowStateMS != 1200 {
			t.Fatalf("reverse=%t: reference borrowed foreign selection: %v, want local 1200", reverse, node.FullWindowStateMS)
		}
		if node.ImpactMS != 635.981 || len(node.MeasurementOrigins) != 1 {
			t.Fatalf("reference changed source value or unioned donor into native origins: %+v", node)
		}
		// Unknown and incomplete donors must not fill a known row's scope.
		for _, unknown := range []bool{false, true} {
			if unknown {
				local.MeasurementSources = nil
			} else {
				local.MeasurementSources.HasUnknown = true
			}
			projection = TraceCausalProjectionFromObservationRecords([]ObservationRecord{root, local, foreign})
			node = traceProjectionFindNodeBySubject(projection.PrimaryRootCauses, root.Subject)
			if node == nil || node.FullWindowStateMS != 0 {
				t.Fatalf("unknown donor borrowed scope (unknown=%t): %+v", unknown, node)
			}
		}
	}
}
