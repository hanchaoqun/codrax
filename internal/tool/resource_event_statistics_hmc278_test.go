package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func hmc278ManyResourceGroups(t *testing.T) tracequery.ResourceStackResult {
	t.Helper()
	var text strings.Builder
	null := tracewire.ResourceScalar{Status: "null"}
	for i := 0; i < 80; i++ {
		e := tracewire.ResourceEvent{SourceID: null, PID: 1, TID: 2, IPID: 1, ITID: 2, Thread: "worker", Operation: "AllocEvent", CallchainID: null, Address: null, Size: null, EndNS: null, StackStatus: "unresolved_callchain", Subtype: &tracewire.ResourceSubtype{ID: tracewire.ResourceScalar{Status: "known", Value: strconv.Itoa(i)}, Name: tracewire.ResourceText{Status: "known", Value: strings.Repeat("完整名称", 250)}}}
		line, ok := tracewire.FormatResourceStack(tracewire.ResourceStackRecord{TimestampNS: 1e9 + int64(i), EventRowID: int64(i), Event: &e})
		if !ok {
			t.Fatal("invalid fixture")
		}
		text.WriteString(line)
		text.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), "resource.systrace")
	if err := os.WriteFile(path, []byte(text.String()), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	p := tracequery.Run(idx, tracequery.Query{View: tracequery.ViewResourceStack, PID: 2, TimeStart: 1, TimeEnd: 2}).ResourceStack
	if !tracequery.ValidResourceStack(*p) {
		t.Fatal("invalid full fixture")
	}
	return *p
}

func TestHMC278ResourceStatisticsHandoffBudget(t *testing.T) {
	p := hmc278ManyResourceGroups(t)
	before, _ := json.Marshal(p)
	bounded := boundedResourceStackHandoff(p)
	data, _ := json.Marshal(bounded)
	if len(data) > resourceStackHandoffByteBudget || len(TraceResourceStackText(bounded, 8)) > resourceStackHandoffByteBudget {
		t.Fatalf("handoff overflow: %d", len(data))
	}
	if !tracequery.ValidResourceStack(bounded) || bounded.MatchedEvents != 80 || len(bounded.Events) == 0 {
		t.Fatal("new groups destroyed valid event preview or census")
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("handoff mutated original complete payload")
	}
}

func TestHMC278ResourceStatisticsOmittedGroupOwnerScope(t *testing.T) {
	p := hmc278ManyResourceGroups(t)
	// Last group has no retained event; it still must satisfy the exact query.
	for _, mutate := range []func(*tracequery.ResourceEventStatistic){
		func(g *tracequery.ResourceEventStatistic) { g.TID = 3 },
		func(g *tracequery.ResourceEventStatistic) { g.PID = 1 << 40 },
		func(g *tracequery.ResourceEventStatistic) { g.Thread = string([]byte{0xff}) },
		func(g *tracequery.ResourceEventStatistic) { g.Process = strings.Repeat("x", 4097) },
	} {
		copy := p
		stats := *p.EventStatistics
		stats.Groups = append([]tracequery.ResourceEventStatistic(nil), stats.Groups...)
		copy.EventStatistics = &stats
		found := false
		for i := range stats.Groups {
			if stats.Groups[i].Subtype.ID.Value == "79" {
				mutate(&stats.Groups[i])
				found = true
				break
			}
		}
		if !found {
			t.Fatal("missing omitted fixture group")
		}
		if tracequery.ValidResourceStack(copy) {
			t.Fatal("out-of-scope or invalid aggregate identity admitted")
		}
	}
}

func TestHMC278ResourceStatisticsEmptyPublicHandoff(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	_, _, p := nativeStackPublicQuery(t, path, map[string]any{"pid": 999, "time_start": 10, "time_end": 10.05})
	if p.Status != "available" || p.MatchedEvents != 0 || p.EventStatistics == nil || !p.EventStatistics.Complete || p.EventStatistics.EventCount != 0 || p.EventStatistics.TotalGroups != 0 {
		t.Fatalf("true zero vanished during handoff %+v", p)
	}
	for _, groups := range [][]tracequery.ResourceEventStatistic{nil, {}} {
		p.EventStatistics.Groups = groups
		if !tracequery.ValidResourceStack(p) {
			t.Fatal("empty slice representation changed zero-result semantics")
		}
	}
}
