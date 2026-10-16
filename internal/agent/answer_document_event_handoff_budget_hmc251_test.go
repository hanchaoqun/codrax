package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceEventInventorySummaryBudgetReclaimsUnusedRowsHMC251(t *testing.T) {
	rows := []types.TraceEventSearchInventoryRow{{Line: 1, EventType: "trace_mark", Raw: "observed"}}
	budget := traceEventInventorySummaryBudget(8000, 2, rows)
	if budget <= 2000 || budget >= 4000 {
		t.Fatalf("summary allocation did not reclaim only unused row bytes: %d", budget)
	}
	if got := traceEventInventorySummaryBudget(8000, 2, nil); got != 4000 {
		t.Fatalf("empty rows reserved a fictitious member budget: %d", got)
	}
	if rows[0].Raw != "observed" {
		t.Fatal("allocation changed the accepted row")
	}
}

func TestTraceEventInventorySharedReferencesFitTotalBudgetHMC251(t *testing.T) {
	base := traceEventInventoryBudgetRecord(t)
	base.EventSearchInventory = types.CloneTraceEventSearchInventory(base.EventSearchInventory)
	first := base.EventSearchInventory.Rows[0]
	base.EventSearchInventory.Rows = nil
	for n := 0; n < 32; n++ {
		row := first
		row.Line, row.LocalLine = n+1, n+1
		base.EventSearchInventory.Rows = append(base.EventSearchInventory.Rows, row)
	}
	base.EventSearchInventory.Coverage.Emitted, base.EventSearchInventory.Coverage.MatchedTotal = 32, 32
	base.EventSearchInventory.RowsComplete, base.EventSearchInventory.HandoffRowsOmitted = true, 0
	for _, size := range []int{0, 100, 200, 300, 400, 600, 800, 1200, 1600, 2000, 2400, 2800, 3200} {
		t.Run(fmt.Sprintf("metadata_%d", size), func(t *testing.T) {
			var ledger types.ObservationLedger
			for n := 0; n < 32; n++ {
				r := base
				r.ID = fmt.Sprintf("shared-query-%02d", n)
				r.SourceRef.QueryScopeID = r.ID
				r.EventSearchInventory = types.CloneTraceEventSearchInventory(base.EventSearchInventory)
				r.EventSearchInventory.QueryScopeID = r.ID
				r.RichNotes = []string{strings.Repeat("x", size)}
				publication, ok := types.BuildTraceEventRuntimeMeasurementPublication(r)
				if !ok {
					t.Fatal("valid typed publication missing")
				}
				encoded, _ := json.Marshal(publication)
				r.RichNotes = append(r.RichNotes, types.TraceNoteKeyRuntimeMeasurement+"="+string(encoded))
				ledger.Records = append(ledger.Records, r)
			}
			before, _ := json.Marshal(ledger)
			prompt := renderAnswerDocTraceEventInventories(ledger)
			t.Logf("section bytes=%d", len(prompt))
			if len(prompt) > traceEventInventoryPromptByteLimit {
				t.Errorf("member references exceeded unchanged section budget: %d > %d", len(prompt), traceEventInventoryPromptByteLimit)
			}
			views := traceEventInventoryPromptViews(t, prompt)
			if len(views) != 32 {
				t.Fatalf("query roster truncated: %d", len(views))
			}
			for _, view := range views {
				if view.PromptRowsShown != 32 || view.PromptRowsOmitted != 0 || len(view.SelectorRowIDs) != 32 {
					t.Fatal("reference budgeting changed exact member selection")
				}
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("budget changed accepted facts")
			}
		})
	}
}

func TestTraceEventInventorySourceReferencesRequirePublishedExactEnvelopeHMC251(t *testing.T) {
	base := traceEventInventoryBudgetRecord(t)
	mirror := base
	mirror.ID, mirror.Predicate, mirror.EventSearchInventory = "point-or-independent-claim", "trace_mark", nil
	mirror.Summary, mirror.RichNotes = "independent explanation stays", []string{"independent boundary stays"}
	records := []types.ObservationRecord{base, mirror}
	before, _ := json.Marshal(records)
	if got := traceEventInventorySourceReferences(records, map[string]bool{base.ID: true}); got[mirror.ID] != base.ID {
		t.Fatal("exact source reference was not reused")
	}
	for _, published := range []map[string]bool{nil, {}, {base.ID: false}} {
		if got := traceEventInventorySourceReferences(records, published); len(got) != 0 {
			t.Fatal("unpublished or budget-omitted inventory supplied a source reference")
		}
	}
	for name, change := range map[string]func(*types.ObservationSourceRef){
		"path":      func(r *types.ObservationSourceRef) { r.Path += ".other" },
		"capture":   func(r *types.ObservationSourceRef) { r.CaptureIdentityPath += ".other" },
		"payload":   func(r *types.ObservationSourceRef) { r.PayloadRef += ".other" },
		"raw":       func(r *types.ObservationSourceRef) { r.RawRef += ".other" },
		"query":     func(r *types.ObservationSourceRef) { r.QueryScopeID += ".other" },
		"window":    func(r *types.ObservationSourceRef) { r.QueryWindowEndTs++ },
		"line":      func(r *types.ObservationSourceRef) { r.QueryLineEnd++ },
		"clock":     func(r *types.ObservationSourceRef) { r.TimeDomain += ".other" },
		"offset":    func(r *types.ObservationSourceRef) { value := 1.0; r.ClockOffsetSec = &value },
		"target":    func(r *types.ObservationSourceRef) { r.QueryTargetPID++ },
		"tool_call": func(r *types.ObservationSourceRef) { r.ToolCallID += ".other" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := mirror
			change(&changed.SourceRef)
			if got := traceEventInventorySourceReferences([]types.ObservationRecord{base, changed}, map[string]bool{base.ID: true}); got[changed.ID] != "" {
				t.Fatal("different exact source envelope borrowed a published reference")
			}
			if got := traceEventInventorySourceReferences([]types.ObservationRecord{base, mirror, changed}, map[string]bool{base.ID: true}); got[mirror.ID] != "" {
				t.Fatal("colliding observation IDs borrowed one source reference")
			}
		})
	}
	invalid := base
	invalid.EventSearchInventory = nil
	if got := traceEventInventorySourceReferences([]types.ObservationRecord{invalid, mirror}, map[string]bool{base.ID: true}); len(got) != 0 {
		t.Fatal("a publication label alone minted an inventory source")
	}
	after, _ := json.Marshal(records)
	if string(before) != string(after) {
		t.Fatal("display sharing mutated facts, explanation or authority")
	}
}

func TestTraceEventInventorySourceAliasesKeepRecordsAndExactTargetsHMC251(t *testing.T) {
	rows := []types.ObservationPromptRecord{
		{ID: "i", Claim: types.TraceEventSearchInventoryPredicate},
		{ID: "a", Source: "old truncated source", Summary: "independent explanation", Notes: []string{"independent note"}},
		{ID: "b", Summary: "another fact"},
		{ID: "c", Summary: "different query"},
		{ID: "d", Summary: "not published"},
	}
	before, _ := json.Marshal(rows)
	refs, roster := traceEventInventoryPromptSourceAliases(rows, map[string]string{"i": "i", "a": "i", "b": "i", "c": "j"})
	if !reflect.DeepEqual(refs, map[string]string{"a": "inventory_source_1", "b": "inventory_source_1", "c": "inventory_source_2"}) ||
		strings.Count(roster, "observation_id=\"i\"") != 1 || strings.Count(roster, "observation_id=\"j\"") != 1 {
		t.Fatalf("source reference roster changed exact targets: %v / %s", refs, roster)
	}
	after, _ := json.Marshal(rows)
	if string(before) != string(after) {
		t.Fatal("source reference changed the separate fact rows")
	}
}

func TestTraceEventInventorySourceSharingKeepsGenericFactsHMC251(t *testing.T) {
	ctx := traceEventInventoryPublicContext(traceEventInventoryPublicResults(t, 20, "2"))
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal([]any{ledger, types.CompileTraceCausalProjectionSet(ledger)})
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	if !strings.Contains(prompt, "inventory_source_1: complete source of inventory observation_id=") {
		t.Fatal("public finalizer did not share its exact published source")
	}
	rows := answerDocObservationPromptRecords(ctx, ledger.Records, answerDocObservationLedgerPromptLimit)
	checked := 0
	for _, row := range rows {
		if row.Claim == types.TraceEventSearchInventoryPredicate {
			continue
		}
		prefix := fmt.Sprintf("- `%s`: origin=`%s`; producer=`%s`; source=", row.ID, row.Origin, row.Producer)
		start := strings.Index(prompt, prefix)
		if start < 0 {
			t.Fatalf("separate generic fact disappeared: %s", row.ID)
		}
		line, _, _ := strings.Cut(prompt[start:], "\n")
		for _, field := range []string{
			fmt.Sprintf("role=`%s`; policy=`%s`", row.Role, row.GroundingPolicy),
			fmt.Sprintf("claim=%q", row.Claim),
			fmt.Sprintf("summary=%q", row.Summary),
		} {
			if !strings.Contains(line, field) {
				t.Fatalf("generic fact field changed: %s", field)
			}
		}
		if row.Span != "" && !strings.Contains(line, "span="+row.Span) {
			t.Fatal("point span changed")
		}
		if len(row.Notes) > 0 && !strings.Contains(line, "notes="+renderAnswerDocObservationNotes(row.Notes, len(row.Notes))) {
			t.Fatal("independent notes changed")
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("fixture had no generic facts")
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal([]any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger)})
	if string(before) != string(after) {
		t.Fatal("display reference changed ledger or causal projection")
	}
}
