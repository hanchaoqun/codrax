package tracequery

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sourceRawVisibilityCarrierBody renders a valid carrier body wrapping a
// record of the given original event name (schema attached when asked).
func sourceRawVisibilityCarrierBody(originalName string, formatID int, withSchema bool) string {
	schema := []byte(fmt.Sprintf(`{"version":1,"id":%d,"name":%q,"fields":[],"print_fmt":""}`, formatID, originalName))
	digest := sha256.Sum256(schema)
	tokens := []string{
		sourceRawVisibilityWire,
		"semantic_authority=none",
		fmt.Sprintf("format_id=%d", formatID),
		"event_name_b64=" + base64.RawURLEncoding.EncodeToString([]byte(originalName)),
		"schema_sha256=" + hex.EncodeToString(digest[:]),
		"payload_b64=" + base64.RawURLEncoding.EncodeToString([]byte{0x3e, 0x81, 0x04, 0x02}),
	}
	if withSchema {
		tokens = append(tokens, "schema_b64="+base64.RawURLEncoding.EncodeToString(schema))
	}
	return strings.Join(tokens, " ")
}

// sourceRawVisibilityTestLine is the shipped carrier shape: the header is the
// reserved event name, the wrapped record's name travels in event_name_b64.
func sourceRawVisibilityTestLine(t *testing.T) string {
	t.Helper()
	return "worker-25827 (25827) [004] .... 32136.700490: " + SourceRawVisibilityEventName + ": " +
		sourceRawVisibilityCarrierBody("hmfs_writepage", 33086, true)
}

func TestSourceRawVisibilityCarrierIsExactAdvisoryOnly(t *testing.T) {
	line := sourceRawVisibilityTestLine(t)
	event, ok := ParseLine(1, line, nil)
	// EVOLUTION RECORD (colleague_merge_audit §40.13, V6-2): the fixture used to
	// carry the wrapped record's own name (`hmfs_writepage:`) in the header;
	// the converter now publishes every carrier under SourceRawVisibilityEventName.
	if !ok || event.Type != EventSourceRawVisibility || event.Name != SourceRawVisibilityEventName ||
		event.SubsystemKind != "" || event.PID != 25827 || event.CPU != 4 {
		t.Fatalf("valid visibility carrier did not remain exact advisory-only: event=%+v ok=%t", event, ok)
	}

	path := filepath.Join(t.TempDir(), "visibility.systrace")
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if index.ParsedKnown != 1 || len(index.Events) != 0 {
		t.Fatalf("ordinary index admitted visibility carrier: known=%d events=%+v", index.ParsedKnown, index.Events)
	}
	streamed, err := StreamEventSearch(context.Background(), path, Query{
		View: "event_search", EventTypes: []EventType{EventSourceRawVisibility}, Limit: 10,
	})
	if err != nil || len(streamed.Events) != 1 ||
		streamed.Events[0].Type != EventSourceRawVisibility ||
		streamed.Events[0].SubsystemKind != "" {
		t.Fatalf("explicit streaming search lost visibility advisory: events=%+v err=%v", streamed.Events, err)
	}
}

// TestSourceRawVisibilityLegacyOriginalNameHeaderStillParses pins that the
// parser is name-agnostic: artifacts converted before the reserved-name
// ruling (header = wrapped record's name) still classify as the advisory
// carrier and never regain name semantics. The reserved name is a producer
// contract enforced by the converter's emission census, not a parser gate.
func TestSourceRawVisibilityLegacyOriginalNameHeaderStillParses(t *testing.T) {
	line := "worker-25827 (25827) [004] .... 32136.700490: hmfs_writepage: " +
		sourceRawVisibilityCarrierBody("hmfs_writepage", 33086, true)
	event, ok := ParseLine(1, line, nil)
	if !ok || event.Type != EventSourceRawVisibility || event.Name != "hmfs_writepage" || event.SubsystemKind != "" {
		t.Fatalf("legacy original-name carrier lost its advisory classification: event=%+v ok=%t", event, ok)
	}
}

func TestSourceRawVisibilityMalformedClaimCannotGainNameSemantics(t *testing.T) {
	valid := sourceRawVisibilityTestLine(t)
	for name, line := range map[string]string{
		"wrong_authority": strings.Replace(valid, "semantic_authority=none", "semantic_authority=filesystem", 1),
		"bad_schema_hash": strings.Replace(valid, "schema_sha256=", "schema_sha256=00", 1),
		"bad_payload":     strings.Replace(valid, "payload_b64=PoEEAg", "payload_b64=%", 1),
		"extra_token":     valid + " invented=1",
	} {
		t.Run(name, func(t *testing.T) {
			event, ok := ParseLine(1, line, nil)
			if !ok || event.Type != EventUnknown || event.SubsystemKind != "" {
				t.Fatalf("malformed reserved carrier gained event-name semantics: event=%+v ok=%t", event, ok)
			}
		})
	}
}

// sourceRawVisibilityCarrierFixture writes two valid sched_switch rows around
// carriers that wrap sched_migrate_task and irq_handler_entry records. The
// header name is a parameter: reserved name (shipped) or the wrapped record's
// own name (the §40.13 defect shape).
func sourceRawVisibilityCarrierFixture(t *testing.T, name string, header func(original string) string) (string, int) {
	t.Helper()
	lines := []string{
		`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
	}
	carriers := 0
	for i := 0; i < 48; i++ {
		lines = append(lines, fmt.Sprintf("worker-25827 (25827) [001] .... 1.%06d: %s: %s",
			100000+i*10, header("sched_migrate_task"), sourceRawVisibilityCarrierBody("sched_migrate_task", 1201, i == 0)))
		carriers++
	}
	for i := 0; i < 2; i++ {
		lines = append(lines, fmt.Sprintf("worker-25827 (25827) [001] .... 1.%06d: %s: %s",
			101000+i*10, header("irq_handler_entry"), sourceRawVisibilityCarrierBody("irq_handler_entry", 1301, i == 0)))
		carriers++
	}
	lines = append(lines,
		`app-20 (20) [000] .... 1.200000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`)
	return writeSchedulerIntegrityTrace(t, name, lines...), carriers
}

// TestSourceRawVisibilityReservedNameCarriersAuditZero (§40.13 PIN 5): with
// carriers under the reserved name, the windowed index's pre-parse integrity
// prefilters (scheduler / cpu-input / duration-order / interrupt endpoint)
// mint ZERO witnesses, the carriers are dropped after parse, and both the
// indexed and the streaming scheduler faces stay published.
func TestSourceRawVisibilityReservedNameCarriersAuditZero(t *testing.T) {
	path, carriers := sourceRawVisibilityCarrierFixture(t, "reserved.systrace", func(string) string {
		return SourceRawVisibilityEventName
	})
	q := Query{PID: 20, TimeStart: 1.0, TimeEnd: 1.3}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1.0, TimeStartSet: true,
		TimeEnd: 1.3, TimeEndSet: true,
		AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Windowed {
		t.Fatal("fixture must exercise the windowed pre-parse audit lane")
	}
	if len(idx.schedulerRowIntegrityFailures) != 0 || len(idx.cpuInputIntegrityFailures) != 0 || len(idx.durationOrderFailures) != 0 {
		t.Fatalf("reserved-name carriers minted integrity witnesses: sched=%+v cpu=%+v duration=%+v",
			idx.schedulerRowIntegrityFailures, idx.cpuInputIntegrityFailures, idx.durationOrderFailures)
	}
	if len(idx.Events) != 2 || idx.ParsedKnown != 2+carriers {
		t.Fatalf("carriers were retained or not counted as known: events=%d known=%d carriers=%d", len(idx.Events), idx.ParsedKnown, carriers)
	}
	stats := ComputeWindowStats(idx, q)
	if len(stats.TopRunning) == 0 || containsSubstring(stats.Caveats, "scheduler_row_parse_incomplete") {
		t.Fatalf("indexed scheduler face poisoned by reserved-name carriers: %+v", stats)
	}
	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil || len(streamed.WindowStats.TopRunning) == 0 ||
		containsSubstring(streamed.Caveats, "scheduler_row_parse_incomplete") ||
		containsSubstring(streamed.Caveats, "stream_state_cluster_fail_closed=true") {
		t.Fatalf("streaming scheduler face poisoned by reserved-name carriers: %+v caveats=%v", streamed.WindowStats, streamed.Caveats)
	}
}

// TestSourceRawVisibilityOriginalNameCarriersAreAuditedAsMalformed is the
// documentation companion of the pin above: the same carriers published under
// the wrapped record's own name ARE audited as malformed semantic rows by the
// header-name-keyed prefilters. This is the mechanism the reserved name
// isolates; a future prefilter change that stops minting these witnesses must
// be a conscious edit here, not drift.
func TestSourceRawVisibilityOriginalNameCarriersAreAuditedAsMalformed(t *testing.T) {
	path, _ := sourceRawVisibilityCarrierFixture(t, "original.systrace", func(original string) string {
		return original
	})
	q := Query{PID: 20, TimeStart: 1.0, TimeEnd: 1.3}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1.0, TimeStartSet: true,
		TimeEnd: 1.3, TimeEndSet: true,
		AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.schedulerRowIntegrityFailures) == 0 ||
		!strings.Contains(idx.schedulerRowIntegrityFailures[0].reason(), "event=sched_migrate_task") {
		t.Fatalf("original-name sched_migrate_task carrier no longer audited by the scheduler prefilter: %+v", idx.schedulerRowIntegrityFailures)
	}
	interruptAudited := false
	for _, failure := range idx.durationOrderFailures {
		if failure.EventName == "irq_handler_entry" && failure.Issue == "endpoint_parse_incomplete" {
			interruptAudited = true
		}
	}
	if !interruptAudited {
		t.Fatalf("original-name irq_handler_entry carrier no longer audited by the interrupt prefilter: %+v", idx.durationOrderFailures)
	}
	stats := ComputeWindowStats(idx, q)
	if len(stats.TopRunning) != 0 || !containsSubstring(stats.Caveats, "scheduler_row_parse_incomplete") {
		t.Fatalf("original-name carriers stopped poisoning the scheduler face: %+v", stats)
	}
	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(streamed.Caveats, "scheduler_row_parse_incomplete") {
		t.Fatalf("streaming lane no longer discloses the original-name carrier audit: %v", streamed.Caveats)
	}
}
