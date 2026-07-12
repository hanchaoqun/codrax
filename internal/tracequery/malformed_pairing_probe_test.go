package tracequery

import "testing"

func TestProbeMalformedPairingEndpointUsesExactRawGrammar(t *testing.T) {
	for _, timestamp := range []string{"NaN", "1.2.3", "1e3"} {
		line := "io-40 (40) [003] .... " + timestamp + ": f2fs_write_end: " + f2fsWriteEndBody
		got, ok := ProbeMalformedPairingEndpoint(line)
		if !ok || got.Name != "f2fs_write_end" || !got.KeyKnown || got.SemanticKey == "" {
			t.Fatalf("malformed timestamp %q lost exact F2FS provenance: ok=%t probe=%+v", timestamp, ok, got)
		}
	}

	mmc, ok := ProbeMalformedPairingEndpoint("io-40 (40) [003] .... NaN: mmc_request_start: " + mmcExactStartBody)
	if !ok || mmc.Name != "mmc_request_start" || !mmc.KeyKnown || mmc.SemanticKey == "" {
		t.Fatalf("malformed timestamp lost exact MMC provenance: ok=%t probe=%+v", ok, mmc)
	}
	missingPID, ok := ProbeMalformedPairingEndpoint("BROKEN [003] .... NaN: f2fs_write_end: " + f2fsWriteEndBody)
	if !ok || missingPID.Name != "f2fs_write_end" || missingPID.KeyKnown || missingPID.SemanticKey != "" {
		t.Fatalf("missing-PID header did not retain family-only F2FS provenance: ok=%t probe=%+v", ok, missingPID)
	}
}

func TestProbeMalformedPairingEndpointPreservesNumericSuffixCommOwner(t *testing.T) {
	for _, test := range []struct {
		comm string
		pid  string
		tgid string
	}{
		{comm: "worker-pool-12", pid: "40", tgid: "400"},
		{comm: "name-with-99-digits", pid: "42", tgid: "402"},
		{comm: "foo-7 [bad", pid: "43", tgid: "403"},
		{comm: "x-1 [2] . 3: e:", pid: "44", tgid: "404"},
	} {
		canonical := test.comm + "-" + test.pid + " (" + test.tgid + ") [003] .... 1.000000: f2fs_write_end: " + f2fsWriteEndBody
		ev, ok := ParseLine(1, canonical, nil)
		if !ok || ev.Comm != test.comm {
			t.Fatalf("canonical owner fixture did not parse: ok=%t event=%+v", ok, ev)
		}
		want := FingerprintPairingEvent(ev)
		malformed := test.comm + "-" + test.pid + " (" + test.tgid + ") [003] .... NaN: f2fs_write_end: " + f2fsWriteEndBody
		got, probed := ProbeMalformedPairingEndpoint(malformed)
		if !probed || !got.KeyKnown || got.SemanticKey != want.SemanticKey {
			t.Fatalf("numeric-suffix comm changed malformed endpoint owner: probed=%t got=%+v want=%+v", probed, got, want)
		}
	}
}

func TestMalformedPairingEndpointElectsLaterInvalidPIDOverCommPseudoHeader(t *testing.T) {
	line := "x-1 [2] . 3: e:-BROKEN (404) [003] .... NaN: f2fs_write_end: " + f2fsWriteEndBody
	if ev, parsed := ParseLine(1, line, nil); parsed {
		t.Fatalf("comm-internal canonical pseudo header displaced later invalid-PID header: event=%+v", ev)
	}
	header, ok := ProbePhysicalFtraceHeader(line)
	if !ok || header.EventName != "f2fs_write_end" || header.TimestampKnown {
		t.Fatalf("later invalid-PID physical header provenance drifted: ok=%t header=%+v", ok, header)
	}
	probe, ok := ProbeMalformedPairingEndpoint(line)
	if !ok || probe.Name != "f2fs_write_end" || probe.KeyKnown || probe.SemanticKey != "" {
		t.Fatalf("later invalid-PID endpoint did not retain family-only authority: ok=%t probe=%+v", ok, probe)
	}
}

func TestProbeMalformedPairingEndpointRejectsParseableProseAndNearNames(t *testing.T) {
	negative := []string{
		"io-40 (40) [003] .... 1.000000: f2fs_write_end: " + f2fsWriteEndBody,
		"customer prose quotes NaN and f2fs_write_end: " + f2fsWriteEndBody,
		"io-40 (40) [003] .... NaN: print: f2fs_write_end: " + f2fsWriteEndBody,
		"io-40 (40) [003] .... NaN: f2fs_write_exit: " + f2fsWriteEndBody,
		"io-40 (40) [003] .... NaN: F2FS_write_end: " + f2fsWriteEndBody,
		"io-40 (40) [003] .... NaN: mmc_request_finish: " + mmcDirectDoneBody,
	}
	for _, line := range negative {
		if got, ok := ProbeMalformedPairingEndpoint(line); ok {
			t.Fatalf("non-malformed or non-exact row gained raw endpoint authority: probe=%+v line=%q", got, line)
		}
	}
}

func TestProbeMalformedPairingEndpointRejectsNestedFullHeadersInsidePrintProse(t *testing.T) {
	outer := func(nested string) string {
		return "outer-7 (7) [001] .... 1.000000: print: customer prose embeds " + nested
	}
	negative := []string{
		outer("inner-40 (40) [003] .... NaN: f2fs_write_end: " + f2fsWriteEndBody),
		outer("inner-40 (40) [003] .... 1.2.3: mmc_request_start: " + mmcExactStartBody),
	}
	for _, line := range negative {
		ev, parsed := ParseLine(1, line, nil)
		if !parsed || ev.Name != "print" {
			t.Fatalf("negative fixture must exercise a real top-level print row: parsed=%t event=%+v line=%q", parsed, ev, line)
		}
		if got, ok := ProbeMalformedPairingEndpoint(line); ok {
			t.Fatalf("nested full header in print prose gained hard endpoint authority: probe=%+v line=%q", got, line)
		}
	}
}

func TestFtraceHeaderAuthoritySelectsLeftmostCompleteHeader(t *testing.T) {
	outer := func(nested string) string {
		return "outer-worker-7 (70) [001] .... 1.000000: print: customer prose embeds " + nested
	}
	lines := []string{
		outer("inner-40 (40) [003] .... 2.000000: f2fs_write_end: " + f2fsWriteEndBody),
		outer("inner-40 (40) [003] .... 2.000000: mmc_request_start: " + mmcExactStartBody),
	}
	for _, line := range lines {
		ev, parsed := ParseLine(1, line, nil)
		if !parsed || ev.Name != "print" || ev.Comm != "outer-worker" || ev.PID != 7 || ev.TGID != 70 || ev.CPU != 1 || ev.Ts != 1 {
			t.Fatalf("nested header displaced the leftmost physical header: parsed=%t event=%+v line=%q", parsed, ev, line)
		}
		if ts, ok := ParseLineTimestampNS(line); !ok || ts != 1_000_000_000 {
			t.Fatalf("timestamp probe selected nested header: ts=%d ok=%t line=%q", ts, ok, line)
		}
		if name, ok := ProbeEventNamePrefix(line); !ok || name != "print" {
			t.Fatalf("event-name probe selected nested header: name=%q ok=%t line=%q", name, ok, line)
		}
		if got, ok := ProbeMalformedPairingEndpoint(line); ok {
			t.Fatalf("nested valid header in print prose gained hard endpoint authority: probe=%+v line=%q", got, line)
		}
	}
}

func TestFtraceHeaderAuthoritySelectsBoundedMiddleHeader(t *testing.T) {
	const comm = "x-1 [2] . 3: e:"
	valid := comm + "-44 (404) [001] .... 1.000000: print: prose embeds " +
		"inner-40 (40) [003] .... 2.000000: f2fs_write_end: " + f2fsWriteEndBody
	ev, parsed := ParseLine(1, valid, nil)
	if !parsed || ev.Comm != comm || ev.PID != 44 || ev.TGID != 404 || ev.Name != "print" || ev.Ts != 1 {
		t.Fatalf("bounded middle outer header lost election: parsed=%t event=%+v", parsed, ev)
	}
	if got, ok := ProbeMalformedPairingEndpoint(valid); ok {
		t.Fatalf("nested endpoint behind bounded middle print gained authority: probe=%+v", got)
	}

	malformed := comm + "-44 (404) [001] .... NaN: print: prose embeds " +
		"inner-40 (40) [003] .... 2.000000: f2fs_write_end: " + f2fsWriteEndBody
	if ev, parsed := ParseLine(1, malformed, nil); parsed {
		t.Fatalf("malformed bounded middle outer promoted a pseudo header: event=%+v", ev)
	}
	header, ok := ProbePhysicalFtraceHeader(malformed)
	if !ok || header.EventName != "print" || header.TimestampKnown {
		t.Fatalf("malformed bounded middle outer header provenance drifted: ok=%t header=%+v", ok, header)
	}
	if got, ok := ProbeMalformedPairingEndpoint(malformed); ok {
		t.Fatalf("nested endpoint behind malformed bounded middle print gained authority: probe=%+v", got)
	}
}

func TestFtraceHeaderAuthorityIgnoresPayloadBracketRoster(t *testing.T) {
	line := "this-is-a-very-long-worker-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]"
	ev, parsed := ParseLine(1, line, nil)
	if !parsed || ev.Comm != "this-is-a-very-long-worker" || ev.PID != 40 || ev.Name != "block_rq_issue" || ev.Ts != 1 {
		t.Fatalf("payload bracket roster displaced the physical header: parsed=%t event=%+v", parsed, ev)
	}
	if ts, ok := ParseLineTimestampNS(line); !ok || ts != 1_000_000_000 {
		t.Fatalf("payload bracket roster displaced timestamp authority: ts=%d ok=%t", ts, ok)
	}
}

func TestFtraceHeaderAuthorityPreservesRealCommAndTGIDShapes(t *testing.T) {
	tests := []struct {
		line string
		comm string
		pid  int
		tgid int
	}{
		{line: "worker-pool-12-40 (400) [003] .... 1.000000: print: keep", comm: "worker-pool-12", pid: 40, tgid: 400},
		{line: "render thread 2-41 (401) [002] .... 1.000000: print: keep", comm: "render thread 2", pid: 41, tgid: 401},
		{line: "name-with-99-digits-42 [001] .... 1.000000: print: keep", comm: "name-with-99-digits", pid: 42},
		{line: "foo]bar-43 (403) [001] .... 1.000000: print: keep", comm: "foo]bar", pid: 43, tgid: 403},
		{line: "x-1 [2] . 3: e:-44 (404) [001] .... 1.000000: print: keep", comm: "x-1 [2] . 3: e:", pid: 44, tgid: 404},
	}
	for _, test := range tests {
		ev, parsed := ParseLine(1, test.line, nil)
		if !parsed || ev.Name != "print" || ev.Comm != test.comm || ev.PID != test.pid || ev.TGID != test.tgid {
			t.Fatalf("leftmost header grammar changed a real comm/TGID shape: parsed=%t event=%+v want_comm=%q want_pid=%d want_tgid=%d",
				parsed, ev, test.comm, test.pid, test.tgid)
		}
		if ts, ok := ParseLineTimestampNS(test.line); !ok || ts != 1_000_000_000 {
			t.Fatalf("real comm/TGID timestamp drifted: ts=%d ok=%t line=%q", ts, ok, test.line)
		}
		if name, ok := ProbeEventNamePrefix(test.line); !ok || name != "print" {
			t.Fatalf("real comm/TGID event probe drifted: name=%q ok=%t line=%q", name, ok, test.line)
		}
	}
}

func TestFtraceHeaderAuthorityDoesNotPromoteNestedHeaderBehindMalformedOuter(t *testing.T) {
	tests := []string{
		"outer-7 (7) [001] .... NaN: print: customer prose embeds " +
			"inner-40 (40) [003] .... 2.000000: f2fs_write_end: " + f2fsWriteEndBody,
		"outer-7 (7) [bad] .... 1.000000: print: customer prose embeds " +
			"inner-40 (40) [003] .... 2.000000: mmc_request_start: " + mmcExactStartBody,
		"outer-7 (7) [bad .... NaN: print: customer prose embeds " +
			"inner-40 (40) [003] .... 2.000000: f2fs_write_end: " + f2fsWriteEndBody,
		"outer-7 (7) [bad] .... NaN: vendor/foo: customer prose embeds " +
			"inner-40 (40) [003] .... 2.000000: mmc_request_start: " + mmcExactStartBody,
		"outer-7 (7) [bad] .... NaN: print-event: customer prose embeds " +
			"inner-40 (40) [003] .... 2.000000: f2fs_write_end: " + f2fsWriteEndBody,
		"BROKEN [003] .... NaN: print: customer prose embeds " +
			"inner-40 (40) [003] .... 2.000000: f2fs_write_end: " + f2fsWriteEndBody,
	}
	for _, line := range tests {
		if ev, parsed := ParseLine(1, line, nil); parsed {
			t.Fatalf("nested endpoint behind malformed outer header was promoted: event=%+v line=%q", ev, line)
		}
		if ts, ok := ParseLineTimestampNS(line); ok || ts != 0 {
			t.Fatalf("nested timestamp behind malformed outer header was promoted: ts=%d ok=%t line=%q", ts, ok, line)
		}
		if name, ok := ProbeEventNamePrefix(line); ok || name != "" {
			t.Fatalf("nested event behind malformed outer header was promoted: name=%q ok=%t line=%q", name, ok, line)
		}
		if got, ok := ProbeMalformedPairingEndpoint(line); ok {
			t.Fatalf("nested endpoint behind malformed outer print caused pair poison: probe=%+v line=%q", got, line)
		}
	}
}

func TestProbePhysicalFtraceHeaderRequiresFullEnvelope(t *testing.T) {
	for _, line := range []string{
		"customer 1.001000: f2fs_write_end: " + f2fsWriteEndBody,
		"prose 1.001000: mmc_request_start: " + mmcExactStartBody,
	} {
		if got, ok := ProbePhysicalFtraceHeader(line); ok {
			t.Fatalf("timestamp:event prose gained physical-header authority: probe=%+v line=%q", got, line)
		}
	}
	outer := "outer-7 (7) [bad] .... NaN: vendor/foo: customer prose embeds " +
		"inner-40 (40) [003] .... 2.000000: f2fs_write_end: " + f2fsWriteEndBody
	got, ok := ProbePhysicalFtraceHeader(outer)
	if !ok || got.EventName != "vendor/foo" || got.TimestampKnown {
		t.Fatalf("leftmost malformed outer header was not retained: ok=%t probe=%+v", ok, got)
	}
}
