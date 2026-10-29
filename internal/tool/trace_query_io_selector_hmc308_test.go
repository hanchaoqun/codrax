package tool

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Run the real native entry twice: the engine resolves both input spellings,
// rather than a test rewriting an observation's target after publication.
func TestHMC308PublicIOSelectorAliases(t *testing.T) {
	fixture, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "pid_then_name", true: "name_then_pid"}[reverse], func(t *testing.T) {
			ctx, path := businessRefTestContext(t, string(fixture))
			selectors := []map[string]any{{"pid": 100}, {"thread": "app-main"}}
			if reverse {
				selectors[0], selectors[1] = selectors[1], selectors[0]
			}
			var boards []*tracequery.RootCauseRankResult
			rows := map[string]types.ObservationRecord{}
			payloads := map[string][]byte{}
			publicationKeys := map[string]bool{}
			for _, selector := range selectors {
				params := map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "time_start": .999, "time_end": 1.051, "platform": "harmony", "trace_flavor": "harmony_hitrace"}
				for k, v := range selector {
					params[k] = v
				}
				result := businessRefTestQuery(t, ctx, params)
				if !result.Success {
					t.Fatal(result.Summary)
				}
				found := false
				for _, row := range result.Observations {
					if !strings.HasSuffix(row.ID, "#root_cause_rank:1") {
						continue
					}
					found = true
					data, err := os.ReadFile(row.SourceRef.PayloadRef)
					if err != nil {
						t.Fatal(err)
					}
					var native tracequery.Result
					if err := json.Unmarshal(data, &native); err != nil {
						t.Fatal(err)
					}
					boards = append(boards, native.RootCauseRank)
					payloads[row.SourceRef.PayloadRef] = data
					key, handled := types.TraceObservationIOPublicationKey(row)
					if !handled || key == "" || row.IORequestEndpoints == nil {
						t.Fatalf("public native source/request receipt missing: %+v", row)
					}
					if math.Abs((row.IORequestEndpoints.CompleteTs-row.IORequestEndpoints.IssueTs)*1000-35) > 1e-6 {
						t.Fatalf("original request residence changed: %+v", row.IORequestEndpoints)
					}
					publicationKeys[key] = true
					rows[row.ID] = row
					t.Logf("selector=%v id=%s source=%+v endpoints=%+v", selector, row.ID, row.SourceRef, row.IORequestEndpoints)
				}
				if !found {
					t.Fatal("native primary IO rank absent")
				}
			}
			if len(rows) != 2 || len(publicationKeys) != 2 || len(payloads) != 2 || len(boards) != 2 || !reflect.DeepEqual(boards[0], boards[1]) {
				t.Fatalf("fixture must retain distinct query publications and an identical native board: rows=%d keys=%d payloads=%d", len(rows), len(publicationKeys), len(payloads))
			}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
			publishedRefs := map[string]types.ObservationSourceRef{}
			for _, record := range ledger.Records {
				if _, ok := rows[record.ID]; ok {
					publishedRefs[record.ID] = record.SourceRef
				}
			}
			if len(publishedRefs) != len(rows) {
				t.Fatal("ledger lost an independent native query")
			}
			ledgerBefore, _ := json.Marshal(ledger)
			set := types.CompileTraceCausalProjectionSet(ledger)
			contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{})
			if err != nil {
				t.Fatal(err)
			}
			accepted := map[string]bool{}
			for _, id := range contract.AcceptedEvidenceIDs {
				accepted[id] = true
			}
			for id := range rows {
				if !accepted[id] {
					t.Errorf("independent query no longer addressable by the candidate contract: %s", id)
				}
			}
			var ioCandidates []types.TraceCauseDecision
			for _, candidate := range contract.Candidates {
				if candidate.Decision.Token.FixDirection == "io_dependency" {
					ioCandidates = append(ioCandidates, candidate.Decision)
				}
			}
			if len(ioCandidates) != 1 {
				t.Errorf("same native request/closed instance became %d IO candidates: %+v", len(ioCandidates), ioCandidates)
			}
			var seats []types.TraceCausalProjectionNode
			for _, projection := range set.Projections {
				for _, node := range projection.RankedSeats {
					if node.IORequestMeasurement && node.ResourceCompletionClosure {
						seats = append(seats, node)
					}
				}
				for _, zh := range []bool{false, true} {
					model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
					ioRows := 0
					for _, entry := range runtimeTraceProjElimBoard(model) {
						if entry.row.Node.IORequestMeasurement && entry.row.Node.ResourceCompletionClosure {
							ioRows++
							if math.Abs(entry.row.Node.EffectiveImpactMS-31) > 1e-6 {
								t.Errorf("rendered cause uses request residence or double-counted value: %+v", entry.row.Node)
							}
						}
					}
					if ioRows != 1 {
						t.Errorf("display board resurrected %d closed IO seats (zh=%v)", ioRows, zh)
					}
				}
			}
			if len(seats) != 1 {
				t.Errorf("same native closed wait became %d ranked seats: %+v", len(seats), seats)
			} else {
				seat := seats[0]
				if seat.StartTs != 1.009010 || seat.EndTs != 1.040010 || math.Abs(seat.EffectiveImpactMS-31) > 1e-6 {
					t.Errorf("closed wait was replaced by request residence: %+v", seat)
				}
				seen := map[string]bool{seat.EvidenceID: true}
				for _, id := range seat.MergedEvidenceIDs {
					seen[id] = true
				}
				for id := range rows {
					if !seen[id] {
						t.Errorf("fold lost independently addressable query row %s", id)
					}
				}
				// Ledger publication supplies ToolCallID; the origin contract
				// preserves repeated contributors, not a unique query count.
				// Every carried receipt must still be one of the exact originals.
				originIDs := map[string]bool{}
				for _, origin := range seat.MeasurementOrigins {
					found := false
					for id, ref := range publishedRefs {
						if reflect.DeepEqual(origin.SourceRef, ref) {
							originIDs[id], found = true, true
							break
						}
					}
					if !found {
						t.Errorf("fold introduced or canonicalized a query receipt: %+v", origin.SourceRef)
					}
				}
				if len(originIDs) != len(publishedRefs) {
					t.Errorf("fold lost an original query receipt: found=%v expected=%v", originIDs, publishedRefs)
				}
			}
			ledgerAfter, _ := json.Marshal(ledger)
			if !bytes.Equal(ledgerBefore, ledgerAfter) {
				t.Error("projection/candidate compilation mutated original ledger")
			}
			for path, before := range payloads {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Errorf("independent raw query changed: %s %v", path, err)
				}
			}
		})
	}
}
