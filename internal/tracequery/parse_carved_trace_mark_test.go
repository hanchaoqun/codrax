package tracequery

// §16 T-span engine root fix: a HarmonyOS print-mark converter (trace_streamer
// suspect) ate the leading action character (B|/E|/C|/S|/F|) of the standard
// hitrace print mark and left a "0x0:" address residue, so marks arrive as
// "print: 0x0: <pid>[|<name>][|I<tag>]". Before the fix the carved payload led
// with a bare pid, isDirectTraceMarkPayload rejected it (no B|/E| leader),
// EventTraceMark never fired, and span_window could not pair B/E — the customer
// could not locate bindApplication/setCoreSettings spans at all.
//
// The restoration lives in normalizeTraceMarkPayload → restoreCarvedTraceMarkPayload
// (parse.go). It fires ONLY under the precise variant signature — a prefix that
// LITERALLY begins with 0x/0X (F1: tracePrintPrefixLooksLikeAddress alone
// accepts any all-hex word like "cafe"/"1248", which is fine for the
// pass-through arm whose payload still carries the action letter, but a
// restoration arm on that loose gate would forge E|/B| marks out of arbitrary
// "word: number" prose and cascade-corrupt per-thread span stacks) AND a carved
// payload whose first field is pure digits (a pid, ≤8 digits) — so the standard
// "print: B|…" / "tracing_mark_write: B|…" / standard address-carved
// "0x<addr>: B|…" paths stay byte-identical. See docs/design
// /customer_dead_session_audit_20260703.md §16/§16.1.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestCarvedTraceMarkRestorationTruthTable is the B/E/C classification truth
// table for the carved variant, asserted at the normalize layer (the single
// point both isTraceMarkPayload and parseTraceMark consume).
func TestCarvedTraceMarkRestorationTruthTable(t *testing.T) {
	cases := []struct {
		name     string
		in       string // raw print-family fields (post "print: " split)
		wantNorm string // canonical restored form
	}{
		// Begin: pid followed by a name field, no tag.
		{"begin name only", "0x0: 15|setCoreSettings", "B|15|setCoreSettings"},
		{"begin bindApplication", "0x0: 15|bindApplication", "B|15|bindApplication"},
		// Begin: pid, name, trailing tag (isomorphic to native B|pid|name|<tag>).
		{"begin name with itag", "0x0: 48|H:validateDisplay|I38", "B|48|H:validateDisplay|I38"},
		{"begin hcodec name with itag", "0x0: 663|H:[hcodec][8_e]WaitFence for seq=2664897987|I33", "B|663|H:[hcodec][8_e]WaitFence for seq=2664897987|I33"},
		// Tag family is not closed to I: M<digits> rides the same shape
		// (undamaged reference capture: B|1727|H:RSUniRenderThread::Render()|M0538).
		{"begin name with mtag", "0x0: 1727|H:RSUniRenderThread::Render()|M0538", "B|1727|H:RSUniRenderThread::Render()|M0538"},
		// Name may carry spaces/colons/digits WITHIN a single |-field — the name
		// is not split on whitespace; the trailing embedded number is part of the
		// name, NOT a counter value.
		{"begin name with spaces and trailing number", "0x0: 481|Choreographer#doFrame 1255154", "B|481|Choreographer#doFrame 1255154"},
		{"begin spacey name with mtag", "0x0: 1727|H:...total ProcessedNodes: 118|M0538", "B|1727|H:...total ProcessedNodes: 118|M0538"},
		// End: bare pid, no field after it.
		{"end bare pid", "0x0: 15", "E|15"},
		// End: pid + only a tag, no name (isomorphic to native E|pid|<tag>).
		{"end itag only", "0x0: 48|I38", "E|48|I38"},
		{"end mtag only", "0x0: 1727|M0538", "E|1727|M0538"},
		// Counter/async safety: trailing INDEPENDENT |-field is a plain numeric
		// value/cookie (not a tag, not embedded in a name) → mapped to C| so it
		// can never forge a sync span.
		{"counter numeric tail", "0x0: 48|HeapAlloc|4096", "C|48|HeapAlloc|4096"},
		// P1-①: the real HarmonyOS 5-field counter puts the VALUE in the MIDDLE
		// and the tag LAST ("C|1252|H:VSync-rs|0|I38" carved). An independent
		// numeric field between name and tag = Counter, NOT Begin — judging only
		// the last field forged a fake 5ms VSync-rs sync span (measured pre-fix).
		{"counter 5-field value mid itag", "0x0: 1252|H:VSync-rs|0|I38", "C|1252|H:VSync-rs|0|I38"},
		{"counter 5-field m0538", "0x0: 1252|H:ACCUMULATED_BUFFER_COUNT|0|M0538", "C|1252|H:ACCUMULATED_BUFFER_COUNT|0|M0538"},
		{"counter 5-field spacey name m47", "0x0: 8091|H:Heap size (KB)|15872|M47", "C|8091|H:Heap size (KB)|15872|M47"},
		// 4-field counter with spacey name (real donghu "C|60194|Heap size (KB)|71214").
		{"counter 4-field spacey name", "0x0: 60194|Heap size (KB)|71214", "C|60194|Heap size (KB)|71214"},
		// DISCLOSED irreducible ambiguity: a value-less counter ("C|60194|Heap")
		// carves to "60194|Heap", byte-identical to a carved Begin — the
		// converter eats B|/C| uniformly. Default is Begin (donghu real corpus:
		// 60/60 counters carry a value field, zero value-less; 2-field Begins
		// dominate). See restoreCarvedTraceMarkPayload doc + ledger §16.1 for
		// the dangling-B residual risk.
		{"value-less counter ambiguity defaults to begin", "0x0: 60194|Heap", "B|60194|Heap"},
		// DISCLOSED tag-shape collision: a 2-field payload whose sole field is
		// literally tag-shaped ("V8") is judged End — "<pid>|<tag>" End rows are
		// pervasive in real captures, spans named exactly ^[A-Z][0-9]+$ are not.
		{"tag-shaped short name judged end", "0x0: 100|V8", "E|100|V8"},
		// Value domain: Inf/NaN parse as floats but are NAMES for the counter
		// test, not plain decimal literals — they must not flip Begin→Counter.
		{"inf tail is a name not a value", "0x0: 48|foo|Inf", "B|48|foo|Inf"},
		{"nan tail is a name not a value", "0x0: 48|foo|NaN", "B|48|foo|NaN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeTraceMarkPayload(tc.in)
			if got != tc.wantNorm {
				t.Fatalf("normalizeTraceMarkPayload(%q) = %q, want %q", tc.in, got, tc.wantNorm)
			}
			if !isTraceMarkPayload(tc.in) {
				t.Fatalf("isTraceMarkPayload(%q) = false, want true (restored variant must be a mark)", tc.in)
			}
		})
	}
}

// TestCarvedTraceMarkParsedFields pins the parseTraceMark output for the variant
// against the identically-parsing standard form, proving the restored canonical
// string re-enters the native parser byte-for-byte.
func TestCarvedTraceMarkParsedFields(t *testing.T) {
	cases := []struct {
		name              string
		variant, standard string
		wantAction        string
		wantSpanPID       int
		wantName          string
		wantValue         string
	}{
		{"begin name", "0x0: 15|setCoreSettings", "B|15|setCoreSettings", "B", 15, "setCoreSettings", ""},
		{"begin name itag", "0x0: 48|H:validateDisplay|I38", "B|48|H:validateDisplay|I38", "B", 48, "H:validateDisplay", "I38"},
		{"begin name mtag", "0x0: 1727|H:RSUniRenderThread::Render()|M0538", "B|1727|H:RSUniRenderThread::Render()|M0538", "B", 1727, "H:RSUniRenderThread::Render()", "M0538"},
		{"begin spacey name no tag", "0x0: 481|Choreographer#doFrame 1255154", "B|481|Choreographer#doFrame 1255154", "B", 481, "Choreographer#doFrame 1255154", ""},
		{"end bare", "0x0: 15", "E|15", "E", 15, "E|15", ""},
		{"end itag", "0x0: 48|I38", "E|48|I38", "E", 48, "I38", ""},
		{"end mtag", "0x0: 1727|M0538", "E|1727|M0538", "E", 1727, "M0538", ""},
		{"counter", "0x0: 48|HeapAlloc|4096", "C|48|HeapAlloc|4096", "C", 48, "HeapAlloc", "4096"},
		// 5-field counter: value lands in SpanValue exactly as the native form;
		// the trailing tag stays in the raw tail for literal search.
		{"counter 5-field", "0x0: 1252|H:VSync-rs|0|I38", "C|1252|H:VSync-rs|0|I38", "C", 1252, "H:VSync-rs", "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			va, vpid, vname, vval := parseTraceMark(tc.variant)
			sa, spid, sname, sval := parseTraceMark(tc.standard)
			// Variant parses to the intended fields.
			if va != tc.wantAction || vpid != tc.wantSpanPID || vname != tc.wantName || vval != tc.wantValue {
				t.Fatalf("variant %q => action=%q spanpid=%d name=%q val=%q; want %q/%d/%q/%q",
					tc.variant, va, vpid, vname, vval, tc.wantAction, tc.wantSpanPID, tc.wantName, tc.wantValue)
			}
			// And is byte-identical to the standard form's parse.
			if va != sa || vpid != spid || vname != sname || vval != sval {
				t.Fatalf("variant %q parse diverged from standard %q: %q/%d/%q/%q vs %q/%d/%q/%q",
					tc.variant, tc.standard, va, vpid, vname, vval, sa, spid, sname, sval)
			}
		})
	}
}

// TestCarvedTraceMarkGateIsolation is the gate red line: the restoration fires
// ONLY on the precise variant signature. Standard direct marks, standard
// address-carved marks (action letter still present), and non-mark prose after
// a 0x0: residue must all be untouched.
func TestCarvedTraceMarkGateIsolation(t *testing.T) {
	// Standard direct payloads: byte-identical through normalize, no restore.
	// The last group is the undamaged reference-capture corpus (action letter
	// intact, container-ns pid, print + tracing_mark_write, H:/M-tag, and a name
	// carrying spaces+digits) — the gate must NOT touch any of them.
	for _, s := range []string{
		"B|20|bindApplication",
		"E|20",
		"E",
		"C|31963|JNI Weak Global Refs|198",
		"E|1252|I39",
		"S|20|asyncName|cookie99",
		"F|20|asyncName|cookie99",
		// Reference fragment 1 (tracing_mark_write standard form, same logical flow):
		"B|14869|setCoreSettings",
		"E|14869",
		"B|14869|bindApplication",
		"B|1727|H:RSUniRenderThread::Render()|M0538",
		"E|1727|M0538",
		"E|499",
		// Reference fragment 2 (print standard form, no 0x0: residue):
		"B|29487|setCoreSettings",
		"E|29487",
		"B|29487|bindApplication",
		"E|481",
		"B|481|Choreographer#doFrame 1255154",
	} {
		if got := normalizeTraceMarkPayload(s); got != s {
			t.Fatalf("standard direct payload %q mutated to %q", s, got)
		}
	}
	// Standard address-carved mark: action letter is still present after the
	// address residue → the existing pass-through wins, restoration never runs.
	if got := normalizeTraceMarkPayload("0xffffffc010123abc: B|20|bindApplication"); got != "B|20|bindApplication" {
		t.Fatalf("standard address-carved mark mutated to %q", got)
	}
	// Prose after a 0x0: residue whose first field is NOT pure digits must NOT
	// be forced into a mark (fail-closed to the pre-§16 path).
	for _, prose := range []string{
		"0x0: hello world from converter",
		"0x0: setCoreSettings", // leads with a word, not a pid
		"0x0: B15|name",        // no pipe after the letter, first field not digits
	} {
		if isTraceMarkPayload(prose) {
			t.Fatalf("non-variant prose %q wrongly restored to a trace mark", prose)
		}
	}
	// A real address prefix but a hex-letter first field (not pure digits) also
	// stays out — only decimal container-ns pids are the variant.
	if isTraceMarkPayload("0x0: abc|name") {
		t.Fatalf("hex-leading payload wrongly restored")
	}
	// F1: the restoration arm requires the prefix to LITERALLY begin with
	// 0x/0X. tracePrintPrefixLooksLikeAddress alone also accepts bare all-hex
	// words ("cafe", "1248"); restoring on those would forge E|15 out of
	// "print: 1248: 15" and a forged E pops a REAL open B off the per-thread
	// span stack (ev.PID-keyed), cascading down the whole thread.
	for _, nonHex := range []string{
		"1248: 15",         // would forge E|15
		"1248: 15|name",    // would forge B|15|name
		"cafe: 99|open db", // would forge B|99|open db
	} {
		if isTraceMarkPayload(nonHex) {
			t.Fatalf("non-0x prefix %q wrongly restored to a trace mark", nonHex)
		}
		if got := normalizeTraceMarkPayload(nonHex); got != nonHex {
			t.Fatalf("non-0x prefix %q mutated to %q", nonHex, got)
		}
	}
	// Uppercase 0X is the same literal signature.
	if got := normalizeTraceMarkPayload("0X0: 15|setCoreSettings"); got != "B|15|setCoreSettings" {
		t.Fatalf("0X-prefixed variant not restored: %q", got)
	}
	// The PRE-EXISTING pass-through arm keeps its looser prefix acceptance:
	// an all-hex non-0x prefix with a DIRECT payload (action letter present —
	// the second precise factor) still carves, byte-identical to pre-§16.
	if got := normalizeTraceMarkPayload("cafe: B|20|x"); got != "B|20|x" {
		t.Fatalf("pre-existing pass-through arm changed: %q", got)
	}
	// pid magnitude sanity: Linux pid_max ≤ 2^22 (7 digits; cap 8). A 9+ digit
	// leading number is a datum (timestamp/hash), not a carved pid.
	if isTraceMarkPayload("0x0: 999999999") {
		t.Fatalf("9-digit leading number wrongly restored as pid End")
	}
	if !isTraceMarkPayload("0x0: 99999999") {
		t.Fatalf("8-digit pid must still restore")
	}
}

// TestCarvedTraceMarkStandardReferenceLinesByteStable pins the undamaged
// reference captures end-to-end through ParseLine: standard container-ns pid
// marks (action letter intact, both tracing_mark_write and print) must classify
// as EventTraceMark with the native action/name/tag — the §16 gate (0x0: +
// digit first field) never fires on them because the action letter is present.
func TestCarvedTraceMarkStandardReferenceLinesByteStable(t *testing.T) {
	intern := newStringInterner()
	cases := []struct {
		line       string
		wantAction string
		wantPID    int
		wantName   string
		wantValue  string
	}{
		// Fragment 1: tracing_mark_write standard form, container-ns pid.
		{`android.haitong-56023 (56023) [002] .... 1.000000: tracing_mark_write: B|14869|setCoreSettings`, "B", 14869, "setCoreSettings", ""},
		{`android.haitong-56023 (56023) [002] .... 1.000011: tracing_mark_write: E|14869`, "E", 14869, "E|14869", ""},
		{`android.haitong-56023 (56023) [002] .... 1.000018: tracing_mark_write: B|14869|bindApplication`, "B", 14869, "bindApplication", ""},
		{`   RSUniRenderThr-1727 ( 1727) [002] .... 1.000030: tracing_mark_write: B|1727|H:RSUniRenderThread::Render()|M0538`, "B", 1727, "H:RSUniRenderThread::Render()", "M0538"},
		{`   RSUniRenderThr-1727 ( 1727) [002] .... 1.000040: tracing_mark_write: E|1727|M0538`, "E", 1727, "M0538", ""},
		// Fragment 2: print standard form (no 0x0: residue), container-ns pid.
		{`e.intl:resident-13849 (13849) [003] .... 2.000000: print: B|29487|setCoreSettings`, "B", 29487, "setCoreSettings", ""},
		{`e.intl:resident-13849 (13849) [003] .... 2.000010: print: E|29487`, "E", 29487, "E|29487", ""},
		{`e.intl:resident-13849 (13849) [003] .... 2.000020: print: B|29487|bindApplication`, "B", 29487, "bindApplication", ""},
		{`      android.anim-10918 (10918) [004] .... 3.000000: print: B|481|Choreographer#doFrame 1255154`, "B", 481, "Choreographer#doFrame 1255154", ""},
	}
	for i, tc := range cases {
		ev, ok := ParseLine(i+1, tc.line, intern)
		if !ok || ev.Type != EventTraceMark {
			t.Fatalf("line %q => ok=%v type=%s, want EventTraceMark", tc.line, ok, ev.Type)
		}
		if ev.SpanAction != tc.wantAction || ev.SpanPID != tc.wantPID || ev.SpanName != tc.wantName || ev.SpanValue != tc.wantValue {
			t.Fatalf("line %q => action=%q pid=%d name=%q val=%q; want %q/%d/%q/%q",
				tc.line, ev.SpanAction, ev.SpanPID, ev.SpanName, ev.SpanValue, tc.wantAction, tc.wantPID, tc.wantName, tc.wantValue)
		}
	}
}

// TestCarvedTraceMarkSpaceyNameNotCounter pins correction #2: a carved Begin
// whose single name |-field embeds a trailing number ("Choreographer#doFrame
// 1255154") is NOT a counter — the numeric-tail counter rule fires only when an
// INDEPENDENT |-separated trailing field is purely numeric.
func TestCarvedTraceMarkSpaceyNameNotCounter(t *testing.T) {
	a, pid, name, val := parseTraceMark("0x0: 481|Choreographer#doFrame 1255154")
	if a != "B" || pid != 481 || name != "Choreographer#doFrame 1255154" || val != "" {
		t.Fatalf("spacey-name Begin misparsed: action=%q pid=%d name=%q val=%q", a, pid, name, val)
	}
	// The same shape must produce a Begin span (not a counter) end-to-end.
	idx := buildTraceIndex(t, "carved_spacey.systrace", strings.Join([]string{
		`  ui-481 ( 481) [000] .... 1.000000: print: 0x0: 481|Choreographer#doFrame 1255154`,
		`  ui-481 ( 481) [000] .... 1.002000: print: 0x0: 481`,
		"",
	}, "\n"))
	for _, ev := range idx.Events {
		if ev.Type == EventTraceMark && ev.SpanAction == "C" {
			t.Fatalf("spacey-name row wrongly classified as counter: %+v", ev)
		}
	}
	res := Run(idx, Query{View: "span_window", SpanName: "Choreographer", Limit: 4})
	if len(res.SpanWindows) != 1 {
		t.Fatalf("expected one Choreographer span, got %d caveats=%v", len(res.SpanWindows), res.Caveats)
	}
}

// TestCarvedTraceMarkCounterVariantNoFalseSpan pins the counter/async safety
// leg: a "<pid>|<name>|<numeric>" carved row is a C| counter, so it must NEVER
// enter the sync-span stack and forge a bindApplication-style window.
func TestCarvedTraceMarkCounterVariantNoFalseSpan(t *testing.T) {
	idx := buildTraceIndex(t, "carved_counter.systrace", strings.Join([]string{
		`  app-6565 ( 6565) [007] .... 1.000000: print: 0x0: 15|HeapAlloc|1024`,
		`  app-6565 ( 6565) [007] .... 1.001000: print: 0x0: 15|HeapAlloc|2048`,
		"",
	}, "\n"))
	// Both rows are trace marks (counters), but of action C.
	cMarks := 0
	for _, ev := range idx.Events {
		if ev.Type == EventTraceMark {
			if ev.SpanAction != "C" {
				t.Fatalf("counter-variant row got action %q, want C", ev.SpanAction)
			}
			cMarks++
		}
	}
	if cMarks != 2 {
		t.Fatalf("expected 2 counter marks, got %d", cMarks)
	}
	// No sync span may be forged from counter rows.
	res := Run(idx, Query{View: "span_window", SpanName: "HeapAlloc", Limit: 4})
	if len(res.SpanWindows) != 0 {
		t.Fatalf("counter-variant rows forged %d false span(s): %+v", len(res.SpanWindows), res.SpanWindows)
	}
}

// TestCarvedTraceMarkRealDonghuCounterCorpusNoFakeSpan replays the REAL donghu
// counter corpus (donghu_tieba_frame.systrace: 32 four-field + 28 five-field
// C| payloads, zero value-less) in carved form. Pre-fix, every 5-field payload
// (value mid, tag last) was misjudged Begin and — paired with a later carved
// End on the same thread — forged a fake sync span (measured: 5ms fake
// "H:VSync-rs" span). All must restore to C and produce zero span windows.
func TestCarvedTraceMarkRealDonghuCounterCorpusNoFakeSpan(t *testing.T) {
	// Carved forms of real donghu payloads: 5-field (I38/I35/M0538/M47) and
	// 4-field (numeric tail), covering all nine distinct counter names.
	carved := []string{
		"1252|H:VSync-rs|0|I38",
		"1252|H:ACCUMULATED_BUFFER_COUNT|0|M0538",
		"1252|H:DVSyncRateManagerPeriod|0|M0538",
		"1252|H:DVSyncRsPretime|0|M0538",
		"646|H:AudioSinkprimary|0|I35",
		"646|H:[100649]RendererInServer|0|I35",
		"8091|H:Heap size (KB)|15872|M47",
		"60194|Heap size (KB)|71214",
		"31963|JNI Weak Global Refs|198",
		"43417|VSYNC-app|1",
		"92|nRdy199|12428",
	}
	for _, p := range carved {
		in := "0x0: " + p
		got := normalizeTraceMarkPayload(in)
		if got != "C|"+p {
			t.Fatalf("real counter payload %q restored to %q, want %q", in, got, "C|"+p)
		}
	}
	// End-to-end: counter rows interleaved with carved End rows on the SAME
	// thread (the exact pre-fix forgery setup) must yield zero span windows.
	idx := buildTraceIndex(t, "carved_donghu_counters.systrace", strings.Join([]string{
		`  rs-1252 ( 1252) [000] .... 1.000000: print: 0x0: 1252|H:VSync-rs|0|I38`,
		`  rs-1252 ( 1252) [000] .... 1.005000: print: 0x0: 1252|I38`,
		`  audio-646 ( 646) [001] .... 1.010000: print: 0x0: 646|H:AudioSinkprimary|0|I35`,
		`  audio-646 ( 646) [001] .... 1.015000: print: 0x0: 646|I35`,
		`  gc-8091 ( 8091) [002] .... 1.020000: print: 0x0: 8091|H:Heap size (KB)|15872|M47`,
		"",
	}, "\n"))
	for _, ev := range idx.Events {
		if ev.Type == EventTraceMark && ev.SpanAction == "B" {
			t.Fatalf("real counter payload restored as Begin: %+v", ev)
		}
	}
	res := Run(idx, Query{View: "span_window", Limit: 8})
	if len(res.SpanWindows) != 0 {
		t.Fatalf("real counter corpus forged %d fake span(s): %+v", len(res.SpanWindows), res.SpanWindows)
	}
	// The counter values must stay consumable by the counter-delta path.
	for _, ev := range idx.Events {
		if ev.Type == EventTraceMark && ev.SpanAction == "C" && ev.SpanName == "H:VSync-rs" && ev.SpanValue != "0" {
			t.Fatalf("5-field counter value lost: %+v", ev)
		}
	}
}

// TestCarvedTraceMarkValueLessCounterAmbiguity pins the DISCLOSED irreducible
// ambiguity: a value-less counter ("C|60194|Heap") carves byte-identical to a
// carved Begin, so it restores to B by documented default (real corpora show
// counters virtually always carry a value: donghu 60/60). The safety property
// pinned here: the resulting dangling B alone is INERT — an unclosed B never
// produces a span window. (If the same thread later carries an unmatched E the
// dangling B can consume it — measured residual risk, documented in ledger
// §16.1; the classification side has no precise signal left to remove it.)
func TestCarvedTraceMarkValueLessCounterAmbiguity(t *testing.T) {
	if got := normalizeTraceMarkPayload("0x0: 60194|Heap"); got != "B|60194|Heap" {
		t.Fatalf("value-less ambiguous payload = %q, want documented default B|60194|Heap", got)
	}
	// Isolated dangling B (no E at all on the thread) → zero spans.
	idx := buildTraceIndex(t, "carved_valueless.systrace", strings.Join([]string{
		`  app-60194 (60194) [000] .... 1.000000: print: 0x0: 60194|Heap`,
		`  app-60194 (60194) [000] .... 1.100000: sched_wakeup: comm=app pid=60194 prio=20 target_cpu=000`,
		"",
	}, "\n"))
	res := Run(idx, Query{View: "span_window", Limit: 8})
	if len(res.SpanWindows) != 0 {
		t.Fatalf("isolated dangling B forged %d span(s): %+v", len(res.SpanWindows), res.SpanWindows)
	}
}

// TestCarvedTraceMarkSpanWindowLightsUp is the end-to-end payoff pin on the
// checked-in real-derived fixture: after restoration the same-thread B/E stack
// pairs and span_window(span_name=bindApplication) locates the closed window —
// the exact capability the carved trace denied the customer.
func TestCarvedTraceMarkSpanWindowLightsUp(t *testing.T) {
	path := filepath.Join("..", "..", "eval", "fixtures", "real_traces", "customer_carved_trace_mark.systrace")
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	// Every print row in the fixture is a carved mark; all must be typed.
	marks := 0
	for _, ev := range idx.Events {
		if ev.Type == EventTraceMark {
			marks++
		}
	}
	if marks == 0 {
		t.Fatalf("no EventTraceMark produced from carved fixture — restoration inert")
	}
	res := Run(idx, Query{View: "span_window", SpanName: "bindApplication", Limit: 4})
	if len(res.SpanWindows) != 1 {
		t.Fatalf("expected exactly one bindApplication window, got %d caveats=%v", len(res.SpanWindows), res.Caveats)
	}
	span := res.SpanWindows[0]
	if !strings.Contains(span.Name, "bindApplication") || span.Kind != "sync" {
		t.Fatalf("unexpected span: %+v", span)
	}
	if span.DurationMs <= 0 {
		t.Fatalf("bindApplication span has non-positive duration: %+v", span)
	}
	// event_search by trace_mark type must also reach the restored marks.
	events := EventSearch(idx, Query{EventTypes: []EventType{EventTraceMark}, Limit: 50})
	if len(events) == 0 {
		t.Fatalf("EventTypes=[trace_mark] returned nothing from carved fixture")
	}
	foundBind := false
	for _, ev := range events {
		if ev.SpanAction == "B" && strings.Contains(ev.SpanName, "bindApplication") {
			foundBind = true
		}
	}
	if !foundBind {
		t.Fatalf("restored bindApplication Begin mark not reachable via event_search")
	}
	// The fixture's carved counter rows (real donghu payload shapes) must be C
	// and must never surface as span windows.
	counters := 0
	for _, ev := range idx.Events {
		if ev.Type == EventTraceMark && ev.SpanAction == "C" {
			counters++
		}
	}
	if counters != 2 {
		t.Fatalf("expected 2 restored counter marks in fixture, got %d", counters)
	}
	for _, name := range []string{"VSync-rs", "VSYNC-app"} {
		res := Run(idx, Query{View: "span_window", SpanName: name, Limit: 4})
		if len(res.SpanWindows) != 0 {
			t.Fatalf("fixture counter %q forged %d fake span(s): %+v", name, len(res.SpanWindows), res.SpanWindows)
		}
	}
}
