package tracequery

// §7.11 B-4 pins (docs/design/customer_dead_session_audit_20260703.md):
// non-mark print-family payloads chain through the EXISTING plugin detectors
// (isAbilityEvent / isXPowerEvent / isHiSystemEvent) plus the comm-anchored
// structural probe for the converter HiSysEvent shape
// ("{domain}/{ename}: {contents}", db2systrace.py:751-764; HiLog shape
// "[{level}][{tag}] {msg}", db2systrace.py:626-634). Ruling: NO standalone
// hilog event family — plain print prose stays EventUnknown and keeps its
// index slot / event_search behavior byte-identical.
//
// F1 hardening: the payload shape alone is ambiguous against real print
// prose ("UI/UX:", "GC/HEAP:", …), so the probe's PRIMARY criterion is
// comm == "<hisysevent>" — the machine comm token converters pin on
// re-emitted HiSysEvent rows (hitraceconv/streamerdb_export_extended.go:940;
// db2systrace.py:751-764) — plus a ≥2 length floor on both head segments.
// Any other comm falls back to the pre-B-4 EventUnknown path (fail-open).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyPrintPayloadPluginChainB4(t *testing.T) {
	const converterComm = "<hisysevent>"
	cases := []struct {
		name   string
		comm   string
		raw    string
		fields string
		want   EventType
	}{
		// Converter HiSysEvent shape (db2systrace.py:751-764), comm pinned to
		// the machine token by the converter — probe fires.
		{"hisysevent shape with contents", converterComm, "print", `BATTERY/POWER_KEY: {"UID":1,"STATE":1}`, EventHiSystemEvent},
		{"hisysevent shape empty contents", converterComm, "print", "BATTERY/SCREEN_ON:", EventHiSystemEvent},
		{"hisysevent shape on tracing_mark_write", converterComm, "tracing_mark_write", "AAFWK/APP_STARTUP_TYPE: pid=1301", EventHiSystemEvent},
		// F1 comm anchor: the SAME perfect converter shape on a real thread
		// comm must NOT be typed — fail-open to the pre-B-4 Unknown path.
		{"converter shape without converter comm", "com.demo.app", "print", "AAFWK/APP_STARTUP_TYPE: pid=1301", EventUnknown},
		// F1 false-positive counterexamples: real userspace print prose whose
		// head fits (or nearly fits) UPPER/UPPER: — all must stay Unknown.
		{"fp io prose", "storaged", "print", "I/O: read latency 5ms", EventUnknown},
		{"fp ab prose", "com.demo.app", "print", "A/B: experiment enabled", EventUnknown},
		{"fp uiux prose", "render_thread", "print", "UI/UX: jank detected on scroll", EventUnknown},
		{"fp gc prose", "com.demo.app", "print", "GC/HEAP: freed 2048kB", EventUnknown},
		{"fp net prose", "netmanager", "print", "NET/DNS: resolve 12ms", EventUnknown},
		{"fp txrx prose", "wifi_service", "print", "TX/RX: bytes 4096 1024", EventUnknown},
		{"fp cpugpu prose", "perfmonitor", "print", "CPU/GPU: load 80 40", EventUnknown},
		// F1 length floor: even under the comm anchor, 1-char segments are
		// not the HiSysEvent identifier shape.
		{"anchored comm short segments", converterComm, "print", "I/O: read done", EventUnknown},
		// Converter HiLog shape (db2systrace.py:626-634) typed only via the
		// existing plugin Contains chain — comm-independent, no hilog family.
		{"hilog tag hits xpower", "com.demo.app", "print", "[I][XPower] battery stats flushed", EventXPower},
		{"hilog tag hits ability chain", "com.demo.app", "print", "[E][AbilityManagerService] connect timeout", EventAbilityMonitor},
		{"hilog tag hits hisysevent chain", "com.demo.app", "print", "[W][HiSysEvent] queue overflow", EventHiSystemEvent},
		// Plain print prose stays Unknown (B-4 ruling: no hilog family).
		{"plain print text", "com.demo.app", "print", "hello world from app", EventUnknown},
		{"plain hilog line without plugin tag", "com.demo.app", "print", "[I][AceLayout] measure done", EventUnknown},
		{"plain tracing_mark_write text", "com.demo.app", "tracing_mark_write", "hello", EventUnknown},
		// Near-miss shapes must not be caught even WITH the comm anchor.
		{"lowercase pseudo domain", converterComm, "print", "http/1.1: ok", EventUnknown},
		{"lowercase ename", converterComm, "print", "AA/bb: x", EventUnknown},
		{"digit-leading domain", converterComm, "print", "2G/NET: x", EventUnknown},
		{"double slash", converterComm, "print", "AA/BB/CC: x", EventUnknown},
		{"no space after colon", converterComm, "print", "AA/BB:x", EventUnknown},
		// Mark payloads keep winning first — probe cannot shadow trace marks.
		{"plain mark payload", "com.demo.app", "print", "B|20|Choreographer#doFrame", EventTraceMark},
		{"address-carved mark payload", "com.demo.app", "print", "0xffffffc010123abc: B|20|bindApplication", EventTraceMark},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyEventType(tc.comm, tc.raw, tc.fields); got != tc.want {
				t.Fatalf("classifyEventType(%q, %q, %q) = %s, want %s", tc.comm, tc.raw, tc.fields, got, tc.want)
			}
		})
	}
}

func TestParseLineConverterHiSysEventPrintPositionalFieldsB4(t *testing.T) {
	intern := newStringInterner()
	// Converter fmt_line shape (db2systrace.py:45-48): no TGID parens, comm
	// pinned to '<hisysevent>' with tid 0 (db2systrace.py:751-764).
	line := `   <hisysevent>-0      [000] d...        2.500000: print: BATTERY/POWER_KEY: {"UID":1,"STATE":1}`
	ev, ok := ParseLine(7, line, intern)
	if !ok {
		t.Fatalf("ParseLine failed for converter hisysevent print row")
	}
	if ev.Type != EventHiSystemEvent {
		t.Fatalf("Type = %s, want %s", ev.Type, EventHiSystemEvent)
	}
	if ev.SubsystemKind != "hi_sysevent" {
		t.Fatalf("SubsystemKind = %q, want hi_sysevent", ev.SubsystemKind)
	}
	if ev.PluginFields.Domain != "BATTERY" || ev.PluginFields.EventName != "POWER_KEY" {
		t.Fatalf("positional plugin fields = domain %q ename %q, want BATTERY / POWER_KEY", ev.PluginFields.Domain, ev.PluginFields.EventName)
	}
}

// TestConverterHiSysEventPositionalHeadWinsOverScrapedKVB4 pins the F2
// ruling: inside the print-family gate there are no native plugin bytes to
// protect, so when the comm-anchored probe hits, the positional
// {domain}/{ename} head UNCONDITIONALLY overrides whatever the generic kv
// pass scraped out of the {contents} tail (name=/bundle= there is payload
// noise, not plugin identity). Before F2 the "fill only if empty" guard let
// that noise win.
func TestConverterHiSysEventPositionalHeadWinsOverScrapedKVB4(t *testing.T) {
	intern := newStringInterner()
	line := `   <hisysevent>-0      [000] d...        2.600000: print: AAFWK/APP_STARTUP_TYPE: pid=1301 name=com.noise.value bundle=com.noise.bundle`
	ev, ok := ParseLine(11, line, intern)
	if !ok || ev.Type != EventHiSystemEvent {
		t.Fatalf("ParseLine = %+v ok=%v, want converter hisysevent row", ev, ok)
	}
	if ev.PluginFields.Domain != "AAFWK" || ev.PluginFields.EventName != "APP_STARTUP_TYPE" {
		t.Fatalf("positional head must win over scraped kv noise: domain %q ename %q, want AAFWK / APP_STARTUP_TYPE", ev.PluginFields.Domain, ev.PluginFields.EventName)
	}
}

func TestParseLineNativePluginRowsUnaffectedByPrintPositionalFillB4(t *testing.T) {
	intern := newStringInterner()
	// Native (non-print) plugin raw types keep byte-identical fields: the
	// positional fill is gated on isPrintFamilyRaw.
	line := `      app-20   (   20) [001] .... 2.128300: hi_sysevent: domain=POWER eventname=THERMAL_REPORT type=STAT value=hot level=MINOR`
	ev, ok := ParseLine(9, line, intern)
	if !ok || ev.Type != EventHiSystemEvent {
		t.Fatalf("ParseLine = %+v ok=%v, want native hi_sysevent row", ev, ok)
	}
	if ev.PluginFields.Domain != "POWER" || ev.PluginFields.EventName != "THERMAL_REPORT" {
		t.Fatalf("native plugin fields changed: domain %q ename %q", ev.PluginFields.Domain, ev.PluginFields.EventName)
	}
}

// TestPlainPrintIndexAndEventSearchUnchangedB4 pins the B-4 no-regression
// half: a plain non-mark print row still parses as EventUnknown, keeps its
// index slot, stays out of ParsedKnown/EventCount, and stays reachable via
// both pattern search and EventTypes=[unknown] — exactly the pre-B-4 shape.
func TestPlainPrintIndexAndEventSearchUnchangedB4(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b4_plain_print.systrace")
	body := strings.Join([]string{
		`      app-20   (   20) [001] .... 2.100000: print: hello world from app`,
		`   <hisysevent>-0      [000] d...        2.110000: print: BATTERY/POWER_KEY: {"UID":1}`,
		`      app-20   (   20) [001] .... 2.120000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	// Plain print stays untyped; converter shape and wakeup are typed.
	if idx.ParsedKnown != 2 {
		t.Fatalf("ParsedKnown = %d, want 2 (typed hisysevent print + wakeup; plain print stays unknown)", idx.ParsedKnown)
	}
	events := EventSearch(idx, Query{Pattern: "hello world", Limit: 10})
	if len(events) != 1 || events[0].Type != EventUnknown {
		t.Fatalf("plain print must stay pattern-searchable as unknown: %+v", events)
	}
	if got := events[0].FieldText; got != "hello world from app" {
		t.Fatalf("plain print FieldText changed: %q", got)
	}
	events = EventSearch(idx, Query{EventTypes: []EventType{EventUnknown}, Limit: 10})
	if len(events) != 1 || !strings.Contains(events[0].FieldText, "hello world") {
		t.Fatalf("EventTypes=[unknown] must still return exactly the plain print row: %+v", events)
	}
	// New intended reachability: the converter row is now type-addressable.
	events = EventSearch(idx, Query{EventTypes: []EventType{EventHiSystemEvent}, Limit: 10})
	if len(events) != 1 || events[0].PluginFields.EventName != "POWER_KEY" {
		t.Fatalf("converter hisysevent print row must be reachable by type: %+v", events)
	}
}
