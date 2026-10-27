package tool

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Use the unchanged public native fixture and request setup. Additional query
// measurements must remain available without becoming additional causes in the
// independently elected principal-window contract.
func TestHMC301PublicCandidateMeasurementScopes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ends     []float64
		captures int
	}{
		{"exact", []float64{2.020}, 1},
		{"wide_exact", []float64{2.0205, 2.020}, 1},
		{"two_wide_exact", []float64{2.0205, 2.0204, 2.020}, 1},
		{"reverse", []float64{2.020, 2.0204, 2.0205}, 1},
		{"same_names_two_captures", []float64{2.0205, 2.020}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := businessRefTestContext(t, selfRunningScopePublicTrace(t))
			path = selfRunningScopePublicPath(t, path)
			optimizationCaliberPublicRequest(ctx, path, "en", "app-100", 100, 2, 2.020, false)
			paths := []string{path}
			if tc.captures == 2 {
				_, second := businessRefTestContext(t, selfRunningScopePublicTrace(t))
				second = selfRunningScopePublicPath(t, second)
				paths = append(paths, second)
				ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts,
					types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: second, Carrier: "path"})
			}
			for _, source := range paths {
				for _, end := range tc.ends {
					selfRunningScopePublicQuery(t, ctx, source, end)
				}
			}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
			set := types.CompileTraceCausalProjectionSet(ledger)
			if len(set.Projections) != tc.captures {
				t.Fatalf("captures merged: %d", len(set.Projections))
			}
			before, err := json.Marshal(set)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range set.Projections {
				seen := map[float64]bool{}
				for _, row := range p.RankedSeats {
					if row.TypeToken != "io_wait" || row.Subject != "threadpool-400" {
						continue
					}
					if seen[row.QueryWindowEndTs] || row.QueryWindowStartTs != 2 || row.StartTs != 2.003 || row.EndTs != 2.014 || math.Abs(row.EffectiveImpactMS-11) > 1e-9 || len(row.MeasurementOrigins) == 0 {
						t.Fatalf("independent native IO measurement changed: %+v", row)
					}
					seen[row.QueryWindowEndTs] = true
				}
				for _, end := range tc.ends {
					if !seen[end] {
						t.Fatalf("raw rank measurement %.6f erased from %s: %+v", end, p.ArtifactPath, p.RankedSeats)
					}
				}
				if len(seen) != len(tc.ends) {
					t.Fatalf("unexpected native scopes: %v", seen)
				}
				values := map[float64]float64{}
				for _, end := range tc.ends {
					if end != 2.020 {
						values[end] = (end - 2.020020) * 1000
					}
				}
				selfRunningScopePublicOrigins(t, p, p.ArtifactPath, values)
			}
			contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{})
			if err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(set)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("candidate eligibility mutated original projection/measurement facts")
			}
			accepted := map[string]bool{}
			for _, id := range contract.AcceptedEvidenceIDs {
				accepted[id] = true
			}
			for _, record := range ledger.Records {
				if record.ID != "" && !accepted[record.ID] {
					t.Fatalf("original ledger evidence removed: %s", record.ID)
				}
			}
			contract.RootCauseReportEnabled = true
			selection := &types.TraceRootCauseReportV2{SchemaVersion: types.TraceRootCauseReportSchemaVersion}
			byArtifact := map[string]int{}
			for _, candidate := range contract.Candidates {
				if candidate.Decision.Token.FixDirection != "io_dependency" && candidate.Decision.Token.Token != "priority_inversion_candidate" {
					continue
				}
				byArtifact[candidate.Decision.ArtifactLabel]++
				selection.RootCauses = append(selection.RootCauses, &types.TraceRootCauseItemV2{CandidateID: candidate.Decision.CandidateID})
			}
			bound, err := tracefinding.BindRootCauseReportSelection(selection, contract)
			if err != nil || bound == nil || len(bound.RootCauses) != 4*tc.captures {
				dump, _ := json.Marshal(struct {
					Projection types.TraceCausalProjectionSet `json:"projection"`
					Contract   *types.TraceFindingContract    `json:"contract"`
				}{set, contract})
				t.Fatalf("native four causes per capture changed: %v; diagnostic=%s", err, dump)
			}
			if len(byArtifact) != tc.captures {
				t.Fatalf("same-name captures lost separate candidates: %v", byArtifact)
			}
			for label, count := range byArtifact {
				if label == "" || count != 4 {
					t.Fatalf("capture roster differs: %v", byArtifact)
				}
			}
			for _, cause := range bound.RootCauses {
				if cause.WindowScope == nil || cause.WindowScope.IsSupportingExploration() || cause.WindowScope.QueryWindowStartTs != 2 || cause.WindowScope.QueryWindowEndTs != 2.020 || cause.ImpactSeconds == nil || (math.Abs(*cause.ImpactSeconds-.011) > 1e-9 && math.Abs(*cause.ImpactSeconds-.001) > 1e-9) {
					t.Fatalf("selected cause lost its own window/value: %+v", cause)
				}
			}
		})
	}
}
