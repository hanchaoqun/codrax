package hitraceconv

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// Assert logical names through the public parser, not a particular text wire.
func traceDBTestHasMarkerLabel(t *testing.T, body, label string) bool {
	t.Helper()
	path := filepath.Join(t.TempDir(), "markers.systrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	index, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range tracequery.Run(index, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventTraceMark}, Limit: 1000}).Events {
		if event.SpanName == label {
			return true
		}
	}
	return false
}

func TestStartupNameOriginMemoryAndSQLiteStageParity(t *testing.T) {
	ref := int64(0)
	origin, err := tracewire.EncodeMarkerNameOrigin(tracewire.MarkerNameOrigin{SourceTable: "app_startup", Name: tracewire.HiSysEventName{Status: "unresolved_reference", Reference: &ref}, Record: &tracewire.MarkerSourceRecord{RowID: 10, OwnerIPID: 1, StartNS: 1000000, EndNS: 2000000}})
	if err != nil {
		t.Fatal(err)
	}
	candidate := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerAppStartup, 10, 100, 100, 1000000, 2000000, "AppStartup:startup")
	candidate.NameOrigin = origin
	candidate.OwnerIPID, candidate.OwnerIPIDKnown = 1, true
	a := renderTraceDBSyncSpanStageCase(t, traceDBSyncSpanStageOptions{ResidentBytes: 1 << 20}, []traceDBSyncSpanCandidate{candidate}, nil, false)
	b := renderTraceDBSyncSpanStageCase(t, traceDBSyncSpanStageOptions{ResidentBytes: 1}, []traceDBSyncSpanCandidate{candidate}, nil, false)
	if a.body != b.body || !reflect.DeepEqual(a.report, b.report) {
		t.Fatal("spill lost source-name metadata or changed endpoint census")
	}
	if !traceDBTestHasMarkerLabel(t, b.body, "AppStartup:startup") {
		t.Fatal("spill output cannot roundtrip")
	}
	for _, mutate := range []func(*traceDBSyncSpanCandidate){
		func(c *traceDBSyncSpanCandidate) { c.StableID++ },
		func(c *traceDBSyncSpanCandidate) { c.OwnerIPID++ },
		func(c *traceDBSyncSpanCandidate) { c.OwnerIPIDKnown = false },
		func(c *traceDBSyncSpanCandidate) { c.Start++ },
		func(c *traceDBSyncSpanCandidate) { c.End++ },
	} {
		bad := candidate
		mutate(&bad)
		if validateTraceDBSyncSpanCandidate(bad) == nil {
			t.Fatal("source record was detached from the admitted interval")
		}
	}
}

func TestStartupNameOriginPublicConversionAndWindow(t *testing.T) {
	for _, tc := range []struct {
		name, ref, mutation, status, value string
		known                              bool
	}{
		{"zero", "0", "", "resolved", "ZERO", true},
		{"null", "NULL", "", "null_reference", "", false},
		{"text", "'0'", "", "invalid_reference_storage_class", "", false},
		{"real", "0.0", "", "invalid_reference_storage_class", "", false},
		{"blob", "X'30'", "", "invalid_reference_storage_class", "", false},
		{"missing", "0", "DELETE FROM data_dict WHERE id=0", "unresolved_reference", "", false},
		{"duplicate", "0", "INSERT INTO data_dict VALUES (0,'OTHER')", "unresolved_reference", "", false},
		{"empty", "0", "UPDATE data_dict SET data='' WHERE id=0", "resolved", "", true},
		{"opaque", "0", "UPDATE data_dict SET data='业务 | stage:α' WHERE id=0", "resolved", "业务 | stage:α", true},
		{"escaped_name_budget", "0", "UPDATE data_dict SET data='" + strings.Repeat(`"`, 3000) + "' WHERE id=0", "resolved", strings.Repeat(`"`, 3000), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mutations []string
			if tc.mutation != "" {
				mutations = append(mutations, tc.mutation)
			}
			_, converted := dictionaryReferencePublicConvert(t, [3]string{tc.ref, "9", "8"}, false, mutations...)
			index, err := tracequery.BuildIndex(context.Background(), converted.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			// event_search is an inclusive locator; do not silently change its
			// existing window contract while adding name provenance.
			got := tracequery.Run(index, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventTraceMark}, TimeStart: .002, TimeEnd: math.Nextafter(.003, 0), TimeStartSet: true, TimeEndSet: true})
			if len(got.Events) != 1 {
				t.Fatalf("narrow window must retain just the start: %+v", got.Events)
			}
			event := got.Events[0]
			if event.PluginFields == nil || event.MarkerNameOrigin == nil {
				t.Fatal("name origin lost in public conversion")
			}
			origin := event.MarkerNameOrigin
			if origin.Record == nil || origin.Record.StartNS != 2000000 || origin.Record.EndNS != 3000000 || origin.Record.OwnerIPID != 1 {
				t.Fatalf("lost original record: %+v", origin.Record)
			}
			if origin.SourceTable != "app_startup" || origin.Name.Status != tc.status || (origin.Name.Name != nil) != tc.known {
				t.Fatalf("wrong source name status: %+v", origin)
			}
			if tc.known && *origin.Name.Name != tc.value {
				t.Fatalf("name changed: %+v", origin)
			}
			if (tc.ref == "0") != (origin.Name.Reference != nil) {
				t.Fatalf("zero/null/storage class conflated: %+v", origin)
			}
			if origin.Name.Reference != nil && *origin.Name.Reference != 0 {
				t.Fatal("zero reference changed")
			}
			end := tracequery.Run(index, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventTraceMark}, TimeStart: .003, TimeEnd: .0031, TimeStartSet: true, TimeEndSet: true})
			if len(end.Events) != 1 || end.Events[0].SpanAction != "E" || end.Events[0].MarkerNameOrigin == nil || end.Events[0].MarkerNameOrigin.Name.Status != tc.status {
				t.Fatalf("end-only window lost source name: %+v", end.Events)
			}
			if !reflect.DeepEqual(end.Events[0].MarkerNameOrigin.Record, origin.Record) {
				t.Fatal("end-only query cannot identify the same source interval")
			}
		})
	}
}
