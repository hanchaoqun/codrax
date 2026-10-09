package hitraceconv

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// Pin source storage independently of the production encoder. The legacy
// fixture's two CPU-registry rows have no generic filter owner; only row 3
// references measure_filter. REAL affinity and missing columns remain exact.
func sameInputWithoutMeasureExtension(t *testing.T, body string) string {
	t.Helper()
	const first = `{"row_id":"1","start_ns":{"storage_class":"integer","value":"2942126016000"},"duration_ns":{"storage_class":"absent","value":""},"value":{"storage_class":"real","value":"2.2e+06"},"filter_id":{"storage_class":"integer","value":"1"},"type":{"storage_class":"absent","value":""},"filter_status":"unknown"}`
	const second = `{"row_id":"2","start_ns":{"storage_class":"integer","value":"2942126116000"},"duration_ns":{"storage_class":"absent","value":""},"value":{"storage_class":"real","value":"1"},"filter_id":{"storage_class":"integer","value":"2"},"type":{"storage_class":"absent","value":""},"filter_status":"unknown"}`
	const third = `{"row_id":"3","start_ns":{"storage_class":"integer","value":"2942126216000"},"duration_ns":{"storage_class":"absent","value":""},"value":{"storage_class":"real","value":"400"},"filter_id":{"storage_class":"integer","value":"10"},"type":{"storage_class":"absent","value":""},"filter_status":"observed_unique","filter":{"id":{"storage_class":"integer","value":"10"},"name":{"storage_class":"text","value":"ddr_freq"},"type":{"storage_class":"text","value":"clock_rate_filter"},"source_arg_set_id":{"storage_class":"absent","value":""}}}`
	if strings.Count(body, tracewire.MeasureIntervalFamily) != 3 {
		t.Fatal("same-input raw measure extension must contain exactly three carriers")
	}
	for _, raw := range []string{first, second, third} {
		line := tracewire.MeasureIntervalFamily + " record=" + base64.RawURLEncoding.EncodeToString([]byte(raw)) + "\n"
		if strings.Count(body, line) != 1 {
			t.Fatalf("same-input raw measure source storage changed: want exactly one %s", raw)
		}
		body = strings.Replace(body, line, "", 1)
	}
	return body
}

// Reverse only the precisely audited extension before applying every old
// receipt/hash assertion. This does not accept an updated broad golden.
func sameInputBeforeMeasureExtension(t *testing.T, receipt sameInputAccountingReceipt) sameInputAccountingReceipt {
	t.Helper()
	if receipt.OutputBytes != 38816 || receipt.OutputSHA256 != "1655f13b64a536bf20616966e585f54cc073ae732f5e73627d2da6eacaa88108" ||
		receipt.EventsWritten != 38 || receipt.ArtifactRows != 38 || receipt.ArtifactKnown != 38 || receipt.ArtifactAuthority != 21 {
		t.Fatalf("same-input three-record measure extension changed output bytes or accounting: %+v", receipt)
	}
	receipt.OutputBytes, receipt.OutputSHA256 = 37193, "d9af65fe4c6c31bf9921bb11412d8614bd11818afe0ed1edad041e2e57969e5a"
	receipt.EventsWritten, receipt.ArtifactRows, receipt.ArtifactKnown, receipt.ArtifactAuthority = 35, 35, 35, 18
	var extension, previous []sameInputCoverageReceipt
	for _, row := range receipt.Coverage {
		if row.Family == "measurement" {
			extension = append(extension, row)
		} else {
			previous = append(previous, row)
		}
	}
	if !reflect.DeepEqual(extension, []sameInputCoverageReceipt{{Family: "measurement", Table: "measure", Role: "query_ready_export", Found: true, RowsRead: 3, RowsEmitted: 3}}) {
		t.Fatalf("same-input raw measure coverage extension drifted: %+v", extension)
	}
	receipt.Coverage = previous
	sorter := sameInputCoverageByKey(receipt.Coverage, "sorter", "__systrace_rows__", "systrace_text_output")
	if sorter == nil || sorter.RowsRead != 38 || sorter.RowsEmitted != 38 || !reflect.DeepEqual(sorter.Metrics, map[string]int64{"authenticated_tail_bytes": 34667, "authenticated_tail_rows": 17, "semantic_rows_sorted": 21}) {
		t.Fatalf("same-input raw measure sorter delta drifted: %+v", sorter)
	}
	sorter.RowsRead, sorter.RowsEmitted = 35, 35
	sorter.Metrics = map[string]int64{"authenticated_tail_bytes": 34667, "authenticated_tail_rows": 17, "semantic_rows_sorted": 18}
	receipt.TraceReceipt = append([]sameInputCoverageReceipt(nil), receipt.TraceReceipt...)
	validation := sameInputCoverageByKey(receipt.TraceReceipt, "trace_cross_validation", "tracequery_build_index", "tracequery_cross_validation")
	if validation == nil || validation.RowsRead != 49 || validation.RowsEmitted != 38 {
		t.Fatalf("same-input raw measure cross-validation delta drifted: %+v", validation)
	}
	validation.RowsRead, validation.RowsEmitted = 46, 35
	var extensionTypes, previousTypes []sameInputEventTypeReceipt
	for _, row := range receipt.EventTypes {
		if row.Type == "measure_interval" {
			extensionTypes = append(extensionTypes, row)
		} else {
			previousTypes = append(previousTypes, row)
		}
	}
	if !reflect.DeepEqual(extensionTypes, []sameInputEventTypeReceipt{{Type: "measure_interval", Count: 3}}) {
		t.Fatalf("same-input raw measure event extension drifted: %+v", extensionTypes)
	}
	receipt.EventTypes = previousTypes
	return receipt
}
