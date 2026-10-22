package tracequery

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func hmc278ResourceEvent() tracewire.ResourceEvent {
	null := tracewire.ResourceScalar{Status: "null"}
	return tracewire.ResourceEvent{SourceID: null, PID: 10, TID: 11, IPID: 1, ITID: 2, Thread: "worker", Process: "gallery", Operation: "AllocEvent", CallchainID: null, Address: null, Size: tracewire.ResourceScalar{Status: "known", Value: "0"}, EndNS: null, StackStatus: "unresolved_callchain"}
}

func hmc278ResourceRows(t *testing.T, records []tracewire.ResourceStackRecord) string {
	t.Helper()
	var b strings.Builder
	for _, r := range records {
		line, ok := tracewire.FormatResourceStack(r)
		if !ok {
			t.Fatalf("invalid fixture %+v", r)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestHMC278ResourceStatisticsFullCensusNotPreviewOrStock(t *testing.T) {
	var records []tracewire.ResourceStackRecord
	add := func(ts int64, mutate func(*tracewire.ResourceEvent)) {
		e := hmc278ResourceEvent()
		if mutate != nil {
			mutate(&e)
		}
		records = append(records, tracewire.ResourceStackRecord{TimestampNS: ts, EventRowID: int64(len(records)), Event: &e})
	}
	for i := 0; i < 50; i++ {
		add(1_000_000_000+int64(i), nil)
	}
	for _, size := range []tracewire.ResourceScalar{{Status: "known", Value: "9223372036854775807"}, {Status: "known", Value: "9223372036854775807"}, {Status: "null"}, {Status: "unavailable"}, {Status: "invalid"}, {Status: "known", Value: "-1"}} {
		size := size
		add(1_100_000_000, func(e *tracewire.ResourceEvent) { e.Size = size })
	}
	add(1_200_000_000, func(e *tracewire.ResourceEvent) { e.Operation = "FreeEvent"; e.Size.Value = "17" })
	add(1_300_000_000, func(e *tracewire.ResourceEvent) { e.Operation = "FD_Open_Event"; e.Size.Value = "1" })
	add(1_400_000_000, func(e *tracewire.ResourceEvent) { e.ITID = 3 }) // same public tid, distinct source instance
	add(1_500_000_000, func(e *tracewire.ResourceEvent) {
		e.Subtype = &tracewire.ResourceSubtype{ID: tracewire.ResourceScalar{Status: "known", Value: "0"}, Name: tracewire.ResourceText{Status: "known", Value: ""}}
	})
	add(1_500_000_001, func(e *tracewire.ResourceEvent) {
		e.Subtype = &tracewire.ResourceSubtype{ID: tracewire.ResourceScalar{Status: "null"}, Name: tracewire.ResourceText{Status: "null"}}
	})
	add(900_000_000, func(e *tracewire.ResourceEvent) {
		e.EndNS = tracewire.ResourceScalar{Status: "known", Value: "1500000000"}
	}) // overlap is not event admission
	add(2_000_000_000, nil) // explicit right edge excluded
	idx := resourceStackTestIndex(t, hmc278ResourceRows(t, records))
	q := Query{View: ViewResourceStack, TimeStart: 1, TimeEnd: 2, Limit: 1}
	p := Run(idx, q).ResourceStack
	if !ValidResourceStack(*p) || p.MatchedEvents != 61 || p.OmittedEvents != 60 || len(p.Events) != 1 || p.EventStatistics.EventCount != 61 || len(p.EventStatistics.Groups) != 6 {
		t.Fatalf("incomplete census: %+v", p)
	}
	var main, fd, release *ResourceEventStatistic
	for i := range p.EventStatistics.Groups {
		g := &p.EventStatistics.Groups[i]
		if g.Operation == "AllocEvent" && g.ITID == 2 && g.Subtype.ID.Status == "unavailable" {
			main = g
		}
		if g.Operation == "FD_Open_Event" {
			fd = g
		}
		if g.Operation == "FreeEvent" {
			release = g
		}
	}
	if main == nil || main.EventCount != 56 || main.KnownSizeEvents != 52 || main.KnownSizeTotal != "18446744073709551614" || main.NullSizeEvents != 1 || main.UnavailableSizeEvents != 1 || main.InvalidSizeEvents != 1 || main.NegativeSizeEvents != 1 || main.SizeUnit != "bytes" || main.Process != "gallery" {
		t.Fatalf("quantity conflation: %+v", main)
	}
	if fd == nil || fd.SizeUnit != "unspecified" || fd.Action != "open" || fd.KnownSizeTotal != "1" || release == nil || release.EventCount != 1 || release.Action != "release" || release.KnownSizeTotal != "17" {
		t.Fatalf("family/endpoint conflation: fd=%+v release=%+v", fd, release)
	}
	streamed, err := StreamResourceStack(context.Background(), idx.Path, q)
	if err != nil || !reflect.DeepEqual(streamed.ResourceStack, p) {
		t.Fatalf("stream/index mismatch %v", err)
	}
	q.Limit = 40
	more := Run(idx, q).ResourceStack
	if !reflect.DeepEqual(more.EventStatistics, p.EventStatistics) {
		t.Fatal("preview changed complete aggregate")
	}
	q.PID, q.TargetScope = 10, TargetScopeProcess
	if got := Run(idx, q).ResourceStack; got.MatchedEvents != 61 {
		t.Fatalf("process owner filter: %+v", got)
	}
	q.PID = 999
	empty := Run(idx, q).ResourceStack
	if !ValidResourceStack(*empty) || empty.EventStatistics.EventCount != 0 || len(empty.EventStatistics.Groups) != 0 {
		t.Fatalf("true empty source result lost: %+v", empty)
	}
	// Retention failure is unavailable, never a partial aggregate or zero census.
	limited, err := streamResourceStack(context.Background(), idx.Path, q, 1, 32<<20)
	if err != nil || limited.ResourceStack.Status != "unavailable" || limited.ResourceStack.EventStatistics != nil {
		t.Fatalf("partial retained census escaped: %v %+v", err, limited.ResourceStack)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StreamResourceStack(ctx, idx.Path, q); err == nil {
		t.Fatal("canceled scan published")
	}
}

func TestHMC278ResourceStatisticsUnknownZeroAndValidator(t *testing.T) {
	e := hmc278ResourceEvent()
	e.Size = tracewire.ResourceScalar{Status: "null"}
	idx := resourceStackTestIndex(t, hmc278ResourceRows(t, []tracewire.ResourceStackRecord{{TimestampNS: 1e9, EventRowID: 1, Event: &e}}))
	p := Run(idx, Query{View: ViewResourceStack}).ResourceStack
	if !ValidResourceStack(*p) || p.EventStatistics.Groups[0].KnownSizeTotal != "" || p.EventStatistics.Groups[0].KnownSizeEvents != 0 || !p.Window.EndInclusive {
		t.Fatalf("unknown became zero %+v", p)
	}
	for _, mutate := range []func(*ResourceStackResult){
		func(p *ResourceStackResult) {
			p.Events[0].Source.Size = tracewire.ResourceScalar{Status: "known", Value: "17"}
		},
		func(p *ResourceStackResult) { p.EventStatistics.Groups[0].KnownSizeTotal = "0" },
		func(p *ResourceStackResult) { p.EventStatistics.Groups[0].SizeUnit = "bytes/s" },
		func(p *ResourceStackResult) { p.EventStatistics.Groups[0].ITID++ },
		func(p *ResourceStackResult) { p.EventStatistics.Groups[0].NullSizeEvents++ },
		func(p *ResourceStackResult) { p.EventStatistics.Complete = false },
		func(p *ResourceStackResult) { p.EventStatistics.Basis = "lifetime_overlap" },
		func(p *ResourceStackResult) {
			p.EventStatistics.Groups = append(p.EventStatistics.Groups, p.EventStatistics.Groups[0])
		},
	} {
		b, _ := json.Marshal(p)
		var copy ResourceStackResult
		if err := json.Unmarshal(b, &copy); err != nil {
			t.Fatal(err)
		}
		mutate(&copy)
		if ValidResourceStack(copy) {
			t.Fatalf("forged statistics admitted %+v", copy.EventStatistics)
		}
	}
	duplicate := *idx
	duplicate.Events = append(append([]Event(nil), idx.Events...), idx.Events[0])
	if got := Run(&duplicate, Query{View: ViewResourceStack}).ResourceStack; got.Status != "unavailable" || got.EventStatistics != nil {
		t.Fatal("duplicate physical identity counted")
	}
}

func TestHMC278ResourceStatisticsCancellationDuringAggregation(t *testing.T) {
	e := hmc278ResourceEvent()
	row := &ResourceStackEvent{Source: e}
	rows := make([]*ResourceStackEvent, 131072)
	for i := range rows {
		rows[i] = row
	}
	// Permit the initial sample; cancel at the next cooperative checkpoint.
	cancel := newRunCancelState(newRunCancelAfterN(1))
	if got := buildResourceEventStatistics(rows, cancel); got != nil || !cancel.fired() {
		t.Fatal("partial aggregate escaped mid-scan cancellation")
	}
}
