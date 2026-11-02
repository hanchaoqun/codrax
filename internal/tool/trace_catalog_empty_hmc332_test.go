package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC332CatalogPublicKnownZeroAcrossViews(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	declared := json.RawMessage(`{"queries":[{"view":"event_search","pid":101,"time_start":10,"time_end":10.05,"limit":1},{"view":"event_search","pid":999,"time_start":10,"time_end":10.05},{"view":"resource_stack","pid":999,"time_start":10,"time_end":10.05}]}`)
	discovery, err := (&TraceCatalog{}).Execute(bus, declared)
	if err != nil || !discovery.Success || len(bus.Mutable.TraceCatalogs()) != 1 {
		t.Fatalf("discovery: %v %s", err, discovery.Summary)
	}
	catalog := bus.Mutable.TraceCatalogs()[0]
	if len(catalog.Snapshot().Queries) != 9 {
		t.Fatalf("expected three exact plans for three physical inputs: %+v", catalog.Snapshot())
	}
	query := func(view string, pid, limit int) types.ToolResult {
		t.Helper()
		args := map[string]any{"source": "path", "path": filepath.Join(root, "capture.data"), "view": view, "pid": pid, "time_start": 10, "time_end": 10.05}
		if limit > 0 {
			args["limit"] = limit
		}
		raw, _ := json.Marshal(args)
		out, err := (&TraceQuery{}).Execute(bus, raw)
		if err != nil || !out.Success {
			t.Fatalf("%s pid=%d execution: %v %s", view, pid, err, out.Summary)
		}
		return out
	}
	positive := requireEventSearchInventory(t, query("event_search", 101, 1)).EventSearchInventory
	if positive.Coverage.MatchedTotal <= 1 || positive.Coverage.Emitted != 1 || positive.RowsComplete {
		t.Fatalf("fixture must have a nonempty census with a bounded preview: %+v", positive)
	}
	for attempt := 0; attempt < 2; attempt++ {
		zero := requireEventSearchInventory(t, query("event_search", 999, 0)).EventSearchInventory
		if zero.Coverage.MatchedTotal != 0 || zero.Coverage.Emitted != 0 || !zero.Coverage.ScopeComplete || !zero.Coverage.EnumerationComplete || !zero.RowsComplete {
			t.Fatalf("fixture did not prove a complete zero: %+v", zero)
		}
	}
	resource := query("resource_stack", 999, 0)
	foundResource := false
	for _, record := range resource.Observations {
		if stack, ok := DecodeTraceResourceStack(record); ok {
			foundResource = stack.Status == "available" && stack.MatchedEvents == 0
		}
	}
	if !foundResource {
		t.Fatal("resource-stack control did not prove a complete zero")
	}
	counts := map[tracecatalog.Outcome]int{}
	attempts := 0
	for _, q := range catalog.Snapshot().Queries {
		counts[q.Outcome]++
		attempts += len(q.Attempts)
		if len(q.Attempts) == 0 {
			if q.Outcome != tracecatalog.OutcomeNotExecuted {
				t.Fatalf("unexecuted plan became observed zero: %+v", q)
			}
			continue
		}
		want := tracecatalog.OutcomeSuccess
		if q.Object.ID == "999" {
			want = tracecatalog.OutcomeEmpty
		}
		if q.Outcome != want {
			t.Errorf("%s pid=%s: complete native result outcome=%s want=%s", q.View, q.Object.ID, q.Outcome, want)
		}
		for _, attempt := range q.Attempts {
			if attempt.Outcome != want || attempt.PayloadRef == "" || attempt.SourceRevision == "" || attempt.RawRef == "" {
				t.Errorf("attempt lost its actual result/status identity: %+v", attempt)
			}
		}
		if q.View == "event_search" && q.Object.ID == "999" && (len(q.Attempts) != 2 || q.Attempts[0].ID == q.Attempts[1].ID) {
			t.Fatalf("same-payload real retry was merged: %+v", q)
		}
	}
	if len(catalog.Snapshot().Queries) != 9 || attempts != 4 || counts[tracecatalog.OutcomeEmpty] != 2 || counts[tracecatalog.OutcomeSuccess] != 1 || counts[tracecatalog.OutcomeNotExecuted] != 6 {
		t.Fatalf("execution/zero/unexecuted census: queries=%d attempts=%d outcomes=%v", len(catalog.Snapshot().Queries), attempts, counts)
	}
}

func catalogZeroResultHMC332(t *testing.T, bus *types.BusContext, path, view string, pid, limit int) (json.RawMessage, types.ToolResult) {
	t.Helper()
	args := map[string]any{"source": "path", "path": path, "view": view, "pid": pid, "time_start": 10, "time_end": 10.05}
	if limit > 0 {
		args["limit"] = limit
	}
	raw, _ := json.Marshal(args)
	out, err := (&TraceQuery{}).Execute(bus, raw)
	if err != nil || !out.Success {
		t.Fatalf("native query failed: %v %s", err, out.Summary)
	}
	return raw, out
}

func TestHMC332CatalogNativeZeroRequiresCompleteExactView(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	path := filepath.Join(root, "capture.data")
	_, out := catalogZeroResultHMC332(t, bus, path, "event_search", 999, 0)
	zero := requireEventSearchInventory(t, out)
	_, positive := catalogZeroResultHMC332(t, bus, path, "event_search", 101, 1)
	nonzero := requireEventSearchInventory(t, positive)
	_, resource := catalogZeroResultHMC332(t, bus, path, "resource_stack", 999, 0)
	if !traceCatalogNativeQueryEmpty("event_search", out.Observations) || !traceCatalogNativeQueryEmpty("resource_stack", resource.Observations) {
		t.Fatal("complete native zero controls must be empty")
	}
	for _, mode := range []string{"scope_partial", "enumeration_partial", "missing_source", "wrong_scope_id", "empty_preview", "missing_inventory", "duplicate_inventory", "mixed_inventory", "unknown_view", "composite_view", "other_view_zero"} {
		t.Run(mode, func(t *testing.T) {
			r := zero
			r.EventSearchInventory = types.CloneTraceEventSearchInventory(zero.EventSearchInventory)
			view := "event_search"
			records := []types.ObservationRecord{r}
			switch mode {
			case "scope_partial":
				r.EventSearchInventory.Coverage.ScopeComplete, r.EventSearchInventory.RowsComplete = false, false
			case "enumeration_partial":
				r.EventSearchInventory.Coverage.EnumerationComplete, r.EventSearchInventory.RowsComplete = false, false
			case "missing_source":
				r.SourceRef.Path = ""
			case "wrong_scope_id":
				r.EventSearchInventory.QueryScopeID += "-other-query"
			case "empty_preview":
				r = nonzero
				r.EventSearchInventory = types.CloneTraceEventSearchInventory(nonzero.EventSearchInventory)
				r.EventSearchInventory.Rows = nil
				r.EventSearchInventory.HandoffRowsOmitted = r.EventSearchInventory.Coverage.Emitted
			case "missing_inventory":
				r.EventSearchInventory = nil
			case "duplicate_inventory":
				records = append(records, zero)
			case "mixed_inventory":
				records = append(records, nonzero)
			case "unknown_view":
				view = "unknown"
			case "composite_view":
				view = "window_stats"
			case "other_view_zero":
				records = resource.Observations
			}
			if mode != "other_view_zero" {
				records[0] = r
			}
			if traceCatalogNativeQueryEmpty(view, records) {
				t.Fatal("unverified, incomplete or different query was called a complete zero")
			}
		})
	}
}

func TestHMC332CatalogZeroKeepsTerminalPrecedence(t *testing.T) {
	for _, mode := range []string{"incomplete", "failure", "execution_error", "canceled", "stale"} {
		t.Run(mode, func(t *testing.T) {
			bus, root := catalogPublicFixture(t)
			_, err := (&TraceCatalog{}).Execute(bus, json.RawMessage(`{"queries":[{"view":"event_search","pid":999,"time_start":10,"time_end":10.05}]}`))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "capture.data")
			raw, out := catalogZeroResultHMC332(t, bus, path, "event_search", 999, 0)
			var p traceQueryParams
			if err := json.Unmarshal(raw, &p); err != nil {
				t.Fatal(err)
			}
			tickets := traceCatalogBeginQuery(bus, p, raw)
			if len(tickets) != 1 {
				t.Fatalf("expected one actual-source ticket: %d", len(tickets))
			}
			prior, ok := tickets[0].catalog.Query(tickets[0].queryID)
			if !ok || prior.Outcome != tracecatalog.OutcomeEmpty || len(prior.Attempts) != 1 {
				t.Fatalf("missing real prepared query control: %+v", prior)
			}
			// Reuse the real native preparation binding above. A nil-material
			// text binder is not a valid replacement for a binary SQL source.
			tickets[0].revision = prior.Attempts[0].SourceRevision
			tickets[0].executedParameters = prior.Attempts[0].ExecutedParameters
			want := tracecatalog.OutcomeSuccess
			var executeErr error
			switch mode {
			case "incomplete":
				out.EnumerationAuthority = &types.ToolEnumerationAuthority{Status: "incomplete"}
			case "failure":
				out.Success, want = false, tracecatalog.OutcomeFailure
			case "execution_error":
				executeErr, want = errors.New("native execution failed"), tracecatalog.OutcomeFailure
			case "canceled":
				executeErr, want = context.Canceled, tracecatalog.OutcomeCanceled
			case "stale":
				f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.Write([]byte("source-generation-change"))
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("fixture generation change: %v %v", err, closeErr)
				}
				want = tracecatalog.OutcomeStale
			}
			// Replay a typed completion boundary, not another native query.
			traceCatalogFinishQuery(bus, tickets, &out, executeErr)
			q, ok := tickets[0].catalog.Query(tickets[0].queryID)
			if !ok || q.Outcome != want {
				t.Fatalf("known zero overrode %s terminal: %+v", mode, q)
			}
			if mode == "incomplete" && !q.Attempts[len(q.Attempts)-1].Truncated {
				t.Fatal("incomplete execution lost its explicit truncated bit")
			}
		})
	}
}
