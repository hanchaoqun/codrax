package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMMCRightEdgeAdmissionSurvivesWarmIndexCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mmc-cache.systrace")
	body := "io-40 (40) [003] .... 1.000000: mmc_request_start: " + mmcExactStartBody + "\n" +
		"io-40 (40) [003] .... 1.002000: mmc_request_done: " + mmcDirectDoneBody + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		idx, err := BuildIndex(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		stats := ComputeWindowStats(idx, Query{})
		row := storageLatencyRow(stats.StorageLatencyByLayer, "mmc", "mmc_request")
		if row == nil || row.PairedCount != 1 {
			t.Fatalf("cache pass %d lost full-right-edge MMC admission: rows=%+v caveats=%v", pass, stats.StorageLatencyByLayer, stats.Caveats)
		}
	}
}

const mmcExactStartBody = "mmc0: start struct mmc_request[0x1234]: cmd_opcode=17 cmd_arg=0x123 cmd_flags=0x456 cmd_retries=1 stop_opcode=18 stop_arg=0x234 stop_flags=0x567 stop_retries=2 sbc_opcode=19 sbc_arg=0x345 sbc_flags=0x678 sbc_retires=3 blocks=8 block_size=512 blk_addr=10 data_flags=0x9 tag=-1 can_retune=1 doing_retune=0 retune_now=1 need_retune=-2 hold_retune=-3 retune_period=4"

const mmcDirectDoneBody = "mmc0: end struct mmc_request[0x1234]: cmd_opcode=17 cmd_err=-5 cmd_resp=0x11223344 0x89abcdef 0x1 0xffffffff cmd_retries=1 stop_opcode=18 stop_err=-6 stop_resp=0x55667788 0x10203040 0xa5a5a5a5 0x0 stop_retries=2 sbc_opcode=19 sbc_err=-7 sbc_resp=0xdeadbeef 0x80000000 0x7fffffff 0x12345678 sbc_retries=3 bytes_xfered=4096 data_err=-8 tag=-1 can_retune=1 doing_retune=0 retune_now=1 need_retune=-9 hold_retune=-10 retune_period=11"

const mmcStructuredDoneBody = "mmc0: end struct mmc_request[0x1234]: cmd_opcode=17 cmd_err=-5 cmd_retries=1 stop_opcode=18 stop_err=-6 stop_retries=2 sbc_opcode=19 sbc_err=-7 sbc_retries=3 bytes_xfered=4096 data_err=-8 tag=-1 can_retune=1 doing_retune=0 retune_now=1 need_retune=-9 hold_retune=-10 retune_period=11"

const mmcCompactStartWire = "mmc0 tag=-1 opcode=17 blocks=8 block_size=512 blk_addr=10"
const mmcCompactDoneWire = "mmc0 tag=-1 opcode=17 bytes_xfered=4096 ret=-5 cmd_err=-6 data_err=-7"

func TestMMCExactWireProfilesRemainSeparateAndPair(t *testing.T) {
	t.Parallel()
	profiles := []struct {
		name      string
		startBody string
		doneBody  string
	}{
		{name: "direct full", startBody: mmcExactStartBody, doneBody: mmcDirectDoneBody},
		{name: "structured no response", startBody: mmcExactStartBody, doneBody: mmcStructuredDoneBody},
		{name: "SQL compact", startBody: mmcCompactStartWire, doneBody: mmcCompactDoneWire},
	}
	for _, tc := range profiles {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start := DecodePairingEndpoint("mmc_request_start", tc.startBody, 40)
			done := DecodePairingEndpoint("mmc_request_done", tc.doneBody, 40)
			if !start.Recognized || !start.KeyKnown || !start.PayloadAdmitted ||
				!done.Recognized || !done.KeyKnown || !done.PayloadAdmitted ||
				start.SemanticKey == "" || start.SemanticKey != done.SemanticKey {
				t.Fatalf("closed MMC profile did not share one admitted coarse lane: start=%+v done=%+v", start, done)
			}

			idx := buildTraceIndex(t, "mmc-"+strings.ReplaceAll(tc.name, " ", "-")+".systrace",
				"io-40 (40) [003] .... 1.000000: mmc_request_start: "+tc.startBody+"\n"+
					"io-40 (40) [003] .... 1.002000: mmc_request_done: "+tc.doneBody+"\n")
			stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.1})
			row := storageLatencyRow(stats.StorageLatencyByLayer, "mmc", "mmc_request")
			if row == nil || row.PairedCount != 1 || !near(row.MaxLatencyMs, 2, .001) {
				t.Fatalf("profile lost parser-clamp-safe endpoint admission: rows=%+v caveats=%v field_text=%q", stats.StorageLatencyByLayer, stats.Caveats, idx.Events[0].FieldText)
			}
			if tc.name != "SQL compact" && len(idx.Events[0].FieldText) != 300 {
				t.Fatalf("fixture no longer proves full-right-edge authority beyond display clamp: len=%d", len(idx.Events[0].FieldText))
			}
		})
	}
}

func TestMMCTypedIdentityCannotSubstituteForClosedPayloadAdmission(t *testing.T) {
	t.Parallel()
	input := PairingEndpointTypedInput{
		Name: "mmc_request_start", HeaderTID: 40,
		StorageIdentityKnown: true, StorageDevice: "mmc0", StorageOperation: "17",
	}
	identityOnly := FingerprintPairingEndpoint(input)
	if !identityOnly.Recognized || !identityOnly.KeyKnown || identityOnly.PayloadAdmitted || identityOnly.SemanticKey == "" {
		t.Fatalf("typed MMC identity substituted for an unknown closed-body verdict: %+v", identityOnly)
	}
	input.StoragePayloadAdmissionKnown = true
	input.StoragePayloadAdmitted = true
	admitted := FingerprintPairingEndpoint(input)
	if !admitted.KeyKnown || !admitted.PayloadAdmitted || admitted.SemanticKey != identityOnly.SemanticKey {
		t.Fatalf("explicit MMC closed-body verdict did not admit the same lane: identity=%+v admitted=%+v", identityOnly, admitted)
	}
}

func TestMMCClosedBodyProfilesRejectCrossProfileAndMalformedShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		event string
		body  string
	}{
		{name: "legacy kv is not a fourth profile", event: "mmc_request_start", body: "dev=mmcblk0 op=read"},
		{name: "direct extra key", event: "mmc_request_start", body: mmcExactStartBody + " latency_us=2"},
		{name: "direct doubled whitespace", event: "mmc_request_start", body: strings.Replace(mmcExactStartBody, " cmd_arg=", "  cmd_arg=", 1)},
		{name: "direct tab whitespace", event: "mmc_request_start", body: strings.Replace(mmcExactStartBody, " cmd_arg=", "\tcmd_arg=", 1)},
		{name: "direct wrong phase prose", event: "mmc_request_start", body: strings.Replace(mmcExactStartBody, " start struct ", " end struct ", 1)},
		{name: "zero request pointer", event: "mmc_request_start", body: strings.Replace(mmcExactStartBody, "[0x1234]", "[0x0]", 1)},
		{name: "uppercase request pointer", event: "mmc_request_start", body: strings.Replace(mmcExactStartBody, "[0x1234]", "[0X1234]", 1)},
		{name: "noncanonical decimal", event: "mmc_request_start", body: strings.Replace(mmcExactStartBody, "cmd_opcode=17", "cmd_opcode=017", 1)},
		{name: "u32 overflow", event: "mmc_request_start", body: strings.Replace(mmcExactStartBody, "blocks=8", "blocks=4294967296", 1)},
		{name: "direct response short", event: "mmc_request_done", body: strings.Replace(mmcDirectDoneBody, "cmd_resp=0x11223344 0x89abcdef 0x1 0xffffffff", "cmd_resp=0x11223344 0x89abcdef 0x1", 1)},
		{name: "direct response uppercase", event: "mmc_request_done", body: strings.Replace(mmcDirectDoneBody, "0xdeadbeef", "0xDEADBEEF", 1)},
		{name: "structured reordered field", event: "mmc_request_done", body: strings.Replace(mmcStructuredDoneBody, "cmd_err=-5 cmd_retries=1", "cmd_retries=1 cmd_err=-5", 1)},
		{name: "structured partial response", event: "mmc_request_done", body: strings.Replace(mmcStructuredDoneBody, "cmd_retries=1", "cmd_resp=0x1 0x2 0x3 0x4 cmd_retries=1", 1)},
		{name: "compact start missing field", event: "mmc_request_start", body: "mmc0 tag=-1 opcode=17 blocks=8 block_size=512"},
		{name: "compact start alias", event: "mmc_request_start", body: "mmc0 tag=-1 cmd_opcode=17 blocks=8 block_size=512 blk_addr=10"},
		{name: "compact done needs error", event: "mmc_request_done", body: "mmc0 tag=-1 opcode=17 bytes_xfered=4096"},
		{name: "compact done optional order", event: "mmc_request_done", body: "mmc0 tag=-1 opcode=17 bytes_xfered=4096 data_err=-7 ret=-5"},
		{name: "quoted device", event: "mmc_request_start", body: "\"mmc0\" tag=-1 opcode=17 blocks=8 block_size=512 blk_addr=10"},
		{name: "trailing comma device", event: "mmc_request_start", body: "mmc0, tag=-1 opcode=17 blocks=8 block_size=512 blk_addr=10"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DecodePairingEndpoint(tc.event, tc.body, 40)
			if !got.Recognized || got.PayloadAdmitted {
				t.Fatalf("malformed/unregistered MMC shape gained payload authority: %+v body=%q", got, tc.body)
			}
		})
	}
}

func TestBoundedEventNameProbeRequiresTerminatedHeaderToken(t *testing.T) {
	line := "io-40 (40) [003] .... 1.000000: mmc_request_start: " + mmcExactStartBody
	if got, ok := ProbeEventNamePrefix(line); !ok || got != "mmc_request_start" {
		t.Fatalf("complete exact header was not recognized: name=%q ok=%t", got, ok)
	}
	cut := strings.Index(line, "mmc_request_start") + len("mmc_request_sta")
	if got, ok := ProbeEventNamePrefix(line[:cut]); ok || got != "" {
		t.Fatalf("unterminated bounded prefix gained event authority: name=%q ok=%t", got, ok)
	}
	other := "io-40 (40) [003] .... 1.000000: print: " + strings.Repeat("x", 8192)
	if got, ok := ProbeEventNamePrefix(other[:4096]); !ok || got != "print" {
		t.Fatalf("bounded non-MMC header could not be excluded precisely: name=%q ok=%t", got, ok)
	}
}

func TestMMCExactNamesAndMalformedBodiesStayInventoryOnly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, start, done string
	}{
		{name: "suffix", start: "mmc_request_start_extra", done: "mmc_request_done_extra"},
		{name: "case", start: "MMC_request_start", done: "MMC_request_done"},
	} {
		if got := DecodePairingEndpoint(tc.start, mmcExactStartBody, 40); got.Recognized || got.KeyKnown || got.PayloadAdmitted {
			t.Fatalf("%s drift entered exact endpoint registry: %+v", tc.name, got)
		}
		idx := buildTraceIndex(t, "mmc-near-"+tc.name+".systrace",
			"io-40 (40) [003] .... 1.000000: "+tc.start+": "+mmcExactStartBody+"\n"+
				"io-40 (40) [003] .... 1.002000: "+tc.done+": "+mmcDirectDoneBody+"\n")
		assertMMCInventoryOnly(t, idx, 0)
	}

	for _, name := range []string{" mmc_request_start", "mmc_request_start ", "MMC_request_start"} {
		if got := DecodePairingEndpoint(name, mmcExactStartBody, 40); got.Recognized || got.KeyKnown || got.PayloadAdmitted {
			t.Fatalf("non-byte-exact public name entered MMC authority: name=%q verdict=%+v", name, got)
		}
	}

	malformed := strings.Replace(mmcExactStartBody, "retune_period=4", "retune_period=4 extra=1", 1)
	idx := buildTraceIndex(t, "mmc-malformed-exact.systrace",
		"io-40 (40) [003] .... 1.000000: mmc_request_start: "+malformed+"\n"+
			"io-40 (40) [003] .... 1.002000: mmc_request_done: "+mmcDirectDoneBody+"\n")
	assertMMCInventoryOnly(t, idx, 1)
}

func assertMMCInventoryOnly(t *testing.T, idx *Index, validSemanticRows int) {
	t.Helper()
	q := Query{TimeStart: .9, TimeEnd: 1.1, Limit: 20}
	stats := ComputeWindowStats(idx, q)
	if stats.StorageEventCount != 2 || stats.EventCounts[EventStorage] != 2 {
		t.Fatalf("MMC rejected rows lost raw inventory: count=%d events=%v", stats.StorageEventCount, stats.EventCounts)
	}
	if len(stats.StorageLatencyByLayer) != 0 || stats.IOPressureSummary != nil || len(stats.IOBurstEpisodes) != 0 ||
		len(stats.SubsystemEvents) != validSemanticRows {
		t.Fatalf("inventory-only MMC rows leaked semantic IO/evidence carriers: storage=%+v pressure=%+v bursts=%+v subsystem=%+v caveats=%v",
			stats.StorageLatencyByLayer, stats.IOPressureSummary, stats.IOBurstEpisodes, stats.SubsystemEvents, stats.Caveats)
	}
	if validSemanticRows == 0 {
		for _, fact := range evidenceFromStats(stats) {
			if strings.Contains(strings.ToLower(fact.Summary+" "+fact.Subject+" "+fact.Object), "mmc") ||
				fact.Predicate == "storage_latency_by_layer" {
				t.Fatalf("inventory-only MMC row leaked evidence: %+v", fact)
			}
		}
	} else if len(stats.SubsystemEvents) != 1 || stats.SubsystemEvents[0].Line != 2 {
		t.Fatalf("malformed MMC row entered subsystem evidence instead of only its valid sibling: %+v", stats.SubsystemEvents)
	}
	for _, item := range BuildRootCauseRank(idx, q).Items {
		if strings.Contains(strings.ToLower(item.Type+" "+item.Source+" "+item.Summary), "mmc") ||
			strings.Contains(item.Type, "io_") {
			t.Fatalf("inventory-only MMC row leaked root-rank seat: %+v", item)
		}
	}
	for _, result := range []Result{
		Run(idx, Query{View: "event_search", TimeStart: .9, TimeEnd: 1.1, Limit: 20}),
		{EvidencePack: evidenceFromEvents(EventSearch(idx, q))},
	} {
		mmcFacts := 0
		for _, fact := range result.EvidencePack {
			if strings.Contains(strings.ToLower(fact.Summary+" "+fact.Subject+" "+fact.Object), "mmc") {
				mmcFacts++
			}
		}
		if mmcFacts != validSemanticRows {
			t.Fatalf("inventory-only MMC rows leaked generic event-search evidence: got=%d want=%d facts=%+v", mmcFacts, validSemanticRows, result.EvidencePack)
		}
	}
}
