package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func profilerFrameBlockLine(tsNS int64, event string) string {
	return traceDBFormatLine("worker", 40, 40, 2, tsNS, 0, 0, event)
}

func profilerFrameBlockTaskLine(task string, tsNS int64, event string) string {
	return strings.TrimLeft(traceDBFormatLine(task, 40, 40, 2, tsNS, 0, 0, event), " ")
}

func profilerRejectedFrameWithData(data []byte) []byte {
	return protoPayload(
		protoBytes(1, []byte("bytrace_plugin")),
		protoBytes(1, []byte("duplicate-name")),
		protoBytes(3, data),
	)
}

func convertProfilerBlockFrameFixture(t *testing.T, messages ...[]byte) (Result, string, string) {
	t.Helper()
	body := syntheticProfilerTraceFile(messages...)
	dir := t.TempDir()
	input := filepath.Join(dir, "input.htrace")
	output := filepath.Join(dir, "output.ftrace")
	if err := os.WriteFile(input, body, 0o600); err != nil {
		t.Fatal(err)
	}
	result, handled, err := tryConvertProfilerContainer(context.Background(), Options{InputPath: input},
		int64(len(body)), output, nil, nil, nil, nil, nil)
	if err != nil || !handled {
		t.Fatalf("profiler frame fixture conversion: handled=%t result=%+v err=%v", handled, result, err)
	}
	published, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	return result, string(published), output
}

func TestProfilerDroppedFrameExactBlockEndpointCannotBridgeSource(t *testing.T) {
	start := profilerFrameBlockLine(5_000_000_000, "block_rq_issue: 0,1 R 4 () 2 + 3 []")
	hole := profilerFrameBlockLine(5_001_000_000, "block_rq_complete: 0,1 R (READ) 2 + 3 [0]")
	collisionA := profilerFrameBlockTaskLine("A", 5_001_000_000, "block_rq_complete: 0,1 R (READ) 2 + 3 [0]")
	collisionB := profilerFrameBlockTaskLine("B", 5_001_000_000, "block_rq_complete: 0,1 R (READ) 2 + 3 [0]")
	collisionOne := profilerFrameBlockTaskLine("1", 5_001_000_000, "block_rq_complete: 0,1 R (READ) 2 + 3 [0]")
	dualAuthority := "    BUIp9vv-yBD.-99    (   40) [999] ....     5.001000: block_rq_issue: 0,1 R 4 () 2 + 3 []"
	badNonPairLead := strings.Replace(
		profilerFrameBlockTaskLine("lead", 5_000_500_000, "print: B|40|Lead"),
		"5.000500", "not-a-time", 1,
	)
	malformedScalarHole := strings.Replace(hole, "5.001000", "not-a-time", 1)
	structuredHole := profilerBlockTestStructuredMessage(209, profilerBlockTypedPayload(209, nil), 5_001_000_000, 40)
	done := profilerFrameBlockLine(5_002_000_000, "block_rq_complete: 0,1 R (READ) 2 + 3 [0]")
	print := profilerFrameBlockLine(5_003_000_000, "print: B|40|Frame")
	malformedPair := profilerTextPairAdmission(malformedScalarHole)
	malformedScan := scanProfilerStrictSystracePayload([]byte(malformedScalarHole+"\n"), nil)
	if malformedScalarHole == hole || !malformedPair.Governed || malformedPair.Admitted ||
		!malformedScan.rejected || !malformedScan.observed[pairRenderBlock] {
		t.Fatalf("malformed-scalar fixture lost exact physical Block provenance: pair=%+v scan=%+v row=%q",
			malformedPair, malformedScan, malformedScalarHole)
	}
	for _, collision := range []struct {
		name  string
		first byte
		line  string
	}{
		{name: "field8-fixed64-A", first: 0x41, line: collisionA},
		{name: "field8-bytes-B", first: 0x42, line: collisionB},
		{name: "field6-fixed64-1", first: 0x31, line: collisionOne},
	} {
		authority := decodeProfilerTracePluginResult([]byte(collision.line + "\n"))
		scan := scanProfilerStrictSystracePayload([]byte(collision.line+"\n"), nil)
		if len(collision.line) == 0 || collision.line[0] != collision.first ||
			authority.Disposition != profilerFtracePayloadMalformed ||
			authority.PairFamilies&pairCriticalFormatFamilyBlock != 0 ||
			!scan.observed[pairRenderBlock] || !profilerPayloadContainsExactBlockEndpoint([]byte(collision.line+"\n")) {
			t.Fatalf("protobuf-key/text collision lost physical Block authority: name=%s first=%x authority=%+v scan=%+v row=%q",
				collision.name, collision.line[0], authority, scan, collision.line)
		}
	}
	dualAuthorityProto := decodeProfilerTracePluginResult([]byte(dualAuthority))
	dualAuthorityScan := scanProfilerStrictSystracePayload([]byte(dualAuthority), nil)
	if dualAuthorityProto.Disposition != profilerFtracePayloadStructured ||
		dualAuthorityProto.PairFamilies&pairCriticalFormatFamilyBlock != 0 ||
		dualAuthorityScan.rejected || !dualAuthorityScan.originText ||
		!dualAuthorityScan.observed[pairRenderBlock] ||
		!profilerPayloadContainsExactBlockEndpoint([]byte(dualAuthority)) {
		t.Fatalf("whole strict text lost authority over protobuf collision: authority=%+v scan=%+v row=%q",
			dualAuthorityProto, dualAuthorityScan, dualAuthority)
	}
	badLeadPayload := []byte(badNonPairLead + "\n" + hole + "\n")
	badLeadScan := scanProfilerStrictSystracePayload(badLeadPayload, nil)
	if !badLeadScan.originText || !badLeadScan.rejected || !badLeadScan.observed[pairRenderBlock] ||
		!profilerPayloadContainsExactBlockEndpoint(badLeadPayload) {
		t.Fatalf("first physical nonpair malformed header lost later exact Block provenance: scan=%+v", badLeadScan)
	}

	for _, test := range []struct {
		name  string
		frame []byte
	}{
		{
			name:  "accepted noncanonical ftrace text",
			frame: syntheticProfilerPluginData("FTRACE-PLUGIN", []byte(hole+"\n")),
		},
		{
			name:  "accepted noncanonical ftrace structured",
			frame: syntheticProfilerPluginData("FTRACE-PLUGIN", structuredHole),
		},
		{
			name:  "rejected profiler plugin text",
			frame: profilerRejectedFrameWithData([]byte(hole + "\n")),
		},
		{
			name:  "rejected profiler plugin structured",
			frame: profilerRejectedFrameWithData(structuredHole),
		},
		{
			name:  "noncanonical exact header with malformed scalar",
			frame: syntheticProfilerPluginData("FTRACE-PLUGIN", []byte(malformedScalarHole+"\n")),
		},
		{
			name:  "rejected exact header with malformed sibling",
			frame: profilerRejectedFrameWithData([]byte(hole + "\nnot a physical trace row\n")),
		},
		{
			name:  "noncanonical protobuf-key collision task A",
			frame: syntheticProfilerPluginData("FTRACE-PLUGIN", []byte(collisionA+"\n")),
		},
		{
			name:  "rejected protobuf-key collision task B",
			frame: profilerRejectedFrameWithData([]byte(collisionB + "\n")),
		},
		{
			name:  "noncanonical whole-text structured collision",
			frame: syntheticProfilerPluginData("FTRACE-PLUGIN", []byte(dualAuthority)),
		},
		{
			name:  "rejected first nonpair malformed header",
			frame: profilerRejectedFrameWithData(badLeadPayload),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := syntheticProfilerTraceFile(
				syntheticProfilerPluginData("bytrace_plugin", []byte(start+"\n")),
				test.frame,
				syntheticProfilerPluginData("bytrace_plugin", []byte(done+"\n")),
				syntheticProfilerPluginData("bytrace_plugin", []byte(print+"\n")),
			)
			dir := t.TempDir()
			input := filepath.Join(dir, "input.htrace")
			output := filepath.Join(dir, "output.ftrace")
			if err := os.WriteFile(input, body, 0o600); err != nil {
				t.Fatal(err)
			}
			result, handled, err := tryConvertProfilerContainer(context.Background(), Options{InputPath: input},
				int64(len(body)), output, nil, nil, nil, nil, nil)
			if err != nil || !handled {
				t.Fatalf("dropped-frame conversion: handled=%t result=%+v err=%v", handled, result, err)
			}
			published, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if result.EventsWritten != 1 || strings.Contains(string(published), "block_rq_") ||
				!strings.Contains(string(published), "print: B|40|Frame") {
				t.Fatalf("dropped exact Block endpoint allowed cross-frame rescue: result=%+v\n%s", result, published)
			}
			barriers := 0
			for _, coverage := range result.TraceCoverage {
				if coverage.Family == "builtin_modern_ftrace:block" && coverage.Table == "__complete_capture_barrier__" {
					barriers++
					if coverage.RowsRead != 2 || coverage.RowsEmitted != 0 ||
						coverage.FieldSources["budget_failure"] != "none" {
						t.Fatalf("Block frame-hole barrier accounting drifted: %+v", coverage)
					}
				}
			}
			if barriers != 1 {
				t.Fatalf("Block frame-hole barrier count=%d coverage=%+v", barriers, result.TraceCoverage)
			}
			index, err := tracequery.BuildIndex(context.Background(), output)
			if err != nil {
				t.Fatal(err)
			}
			window := tracequery.ComputeWindowStats(index, tracequery.Query{})
			if len(window.IOLatencies) != 0 {
				t.Fatalf("dropped frame minted downstream Block duration: %+v", window.IOLatencies)
			}
		})
	}
}

func TestProfilerCanonicalWholeTextWinsStructuredClassifierCollision(t *testing.T) {
	start := profilerFrameBlockLine(5_000_000_000, "block_rq_issue: 0,1 R 4 () 2 + 3 []")
	collision := "    BUIp9vv-yBD.-99    (   40) [999] ....     5.001000: block_rq_issue: 0,1 R 4 () 2 + 3 []"
	done := profilerFrameBlockLine(5_002_000_000, "block_rq_complete: 0,1 R (READ) 2 + 3 [0]")
	print := profilerFrameBlockLine(5_003_000_000, "print: B|40|Frame")

	result, text, output := convertProfilerBlockFrameFixture(t,
		syntheticProfilerPluginData("bytrace_plugin", []byte(start+"\n")),
		syntheticProfilerPluginData("ftrace-plugin", []byte(collision)),
		syntheticProfilerPluginData("bytrace_plugin", []byte(done+"\n")),
		syntheticProfilerPluginData("bytrace_plugin", []byte(print+"\n")),
	)
	if result.EventsWritten != 4 || !strings.Contains(text, "BUIp9vv-yBD.-99") ||
		strings.Count(text, "block_rq_issue:") != 2 || strings.Count(text, "block_rq_complete:") != 1 {
		t.Fatalf("canonical strict text was lost to protobuf classification: result=%+v\n%s", result, text)
	}
	for _, coverage := range result.TraceCoverage {
		if coverage.Family == "builtin_modern_ftrace:block" && coverage.Table == "__complete_capture_barrier__" {
			t.Fatalf("complete canonical text invented a capture hole: %+v", coverage)
		}
	}
	index, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	window := tracequery.ComputeWindowStats(index, tracequery.Query{})
	if len(window.IOLatencies) != 0 {
		t.Fatalf("classifier-dropped middle start allowed a false cross-hole duration: %+v", window.IOLatencies)
	}
}

func TestProfilerMalformedStructuredEmbeddedHeaderCannotElectTextOrigin(t *testing.T) {
	start := profilerFrameBlockLine(5_000_000_000, "block_rq_issue: 0,1 R 4 () 2 + 3 []")
	embedded := profilerFrameBlockTaskLine("A", 5_001_000_000, "block_rq_issue: 0,1 R 4 () 2 + 3 []") + "\n"
	malformedStructured := append(protoBytes(5, append([]byte("\n"), []byte(embedded)...)), 0)
	done := profilerFrameBlockLine(5_002_000_000, "block_rq_complete: 0,1 R (READ) 2 + 3 [0]")
	print := profilerFrameBlockLine(5_003_000_000, "print: B|40|Frame")

	result, text, output := convertProfilerBlockFrameFixture(t,
		syntheticProfilerPluginData("bytrace_plugin", []byte(start+"\n")),
		syntheticProfilerPluginData("FTRACE-PLUGIN", malformedStructured),
		syntheticProfilerPluginData("bytrace_plugin", []byte(done+"\n")),
		syntheticProfilerPluginData("bytrace_plugin", []byte(print+"\n")),
	)
	if result.EventsWritten != 3 || strings.Count(text, "block_rq_issue:") != 1 ||
		strings.Count(text, "block_rq_complete:") != 1 || !strings.Contains(text, "print: B|40|Frame") {
		t.Fatalf("embedded structured header falsely closed Block publication: result=%+v\n%s", result, text)
	}
	for _, coverage := range result.TraceCoverage {
		if coverage.Family == "builtin_modern_ftrace:block" && coverage.Table == "__complete_capture_barrier__" {
			t.Fatalf("embedded structured header invented Block provenance: %+v", coverage)
		}
	}
	index, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	window := tracequery.ComputeWindowStats(index, tracequery.Query{})
	if len(window.IOLatencies) != 1 {
		t.Fatalf("embedded structured metadata changed the one real Block duration: %+v", window.IOLatencies)
	}
}

func TestProfilerDroppedFrameBlockProvenanceIsExactAndSourceBounded(t *testing.T) {
	exact := profilerFrameBlockLine(5_001_000_000, "block_rq_issue: 0,1 R 4 () 2 + 3 []") + "\n"
	collisionExact := profilerFrameBlockTaskLine("A", 5_001_000_000, "block_rq_issue: 0,1 R 4 () 2 + 3 []") + "\n"
	structuredEmbedded := protoBytes(5, append([]byte{0}, []byte(collisionExact)...))
	embeddedAuthority := decodeProfilerTracePluginResult(structuredEmbedded)
	if embeddedAuthority.Disposition != profilerFtracePayloadStructured ||
		embeddedAuthority.PairFamilies&pairCriticalFormatFamilyBlock != 0 ||
		profilerPayloadContainsExactBlockEndpoint(structuredEmbedded) {
		t.Fatalf("structured embedded header escaped typed-envelope precedence: authority=%+v", embeddedAuthority)
	}
	malformedStructuredEmbedded := append(
		protoBytes(5, append([]byte("\n"), []byte(collisionExact)...)),
		0,
	)
	malformedEmbeddedAuthority := decodeProfilerTracePluginResult(malformedStructuredEmbedded)
	malformedEmbeddedScan := scanProfilerStrictSystracePayload(malformedStructuredEmbedded, nil)
	if malformedEmbeddedAuthority.Disposition != profilerFtracePayloadMalformed ||
		malformedEmbeddedAuthority.PairFamilies&pairCriticalFormatFamilyBlock != 0 ||
		!malformedEmbeddedScan.observed[pairRenderBlock] || malformedEmbeddedScan.originText ||
		profilerPayloadContainsExactBlockEndpoint(malformedStructuredEmbedded) {
		t.Fatalf("malformed structured metadata promoted a later embedded header: authority=%+v scan=%+v",
			malformedEmbeddedAuthority, malformedEmbeddedScan)
	}
	anonymousThenExact := []byte("anonymous metadata\n" + collisionExact)
	anonymousAuthority := decodeProfilerTracePluginResult(anonymousThenExact)
	anonymousScan := scanProfilerStrictSystracePayload(anonymousThenExact, nil)
	if anonymousAuthority.Disposition != profilerFtracePayloadNotStructured ||
		!anonymousScan.observed[pairRenderBlock] || anonymousScan.originText ||
		profilerPayloadContainsExactBlockEndpoint(anonymousThenExact) {
		t.Fatalf("anonymous first fragment let a later exact header elect text: authority=%+v scan=%+v",
			anonymousAuthority, anonymousScan)
	}
	for _, drift := range []string{
		strings.Replace(collisionExact, "block_rq_issue:", "block_rq_issue", 1),
		strings.Replace(collisionExact, "block_rq_issue:", "block_rq_issue :", 1),
	} {
		payload := []byte(drift + collisionExact)
		scan := scanProfilerStrictSystracePayload(payload, nil)
		if !scan.originText || !scan.observed[pairRenderBlock] || !profilerPayloadContainsExactBlockEndpoint(payload) {
			t.Fatalf("physical delimiter drift failed to elect text origin for a later exact endpoint: scan=%+v", scan)
		}
	}
	oversizedPhysical := profilerFrameBlockTaskLine("lead", 5_000_000_000,
		"print: "+strings.Repeat("x", maxProfilerTextLineBytes)) + "\n" + collisionExact
	oversizedPhysicalScan := scanProfilerStrictSystracePayload([]byte(oversizedPhysical), nil)
	if !oversizedPhysicalScan.originText || !oversizedPhysicalScan.rejected ||
		!oversizedPhysicalScan.observed[pairRenderBlock] ||
		!profilerPayloadContainsExactBlockEndpoint([]byte(oversizedPhysical)) {
		t.Fatalf("oversized first nonpair physical header denied text origin: scan=%+v", oversizedPhysicalScan)
	}
	oversizedAnonymous := strings.Repeat("x", maxProfilerTextLineBytes+1) + "\n" + collisionExact
	oversizedAnonymousScan := scanProfilerStrictSystracePayload([]byte(oversizedAnonymous), nil)
	if oversizedAnonymousScan.originText || !oversizedAnonymousScan.observed[pairRenderBlock] ||
		profilerPayloadContainsExactBlockEndpoint([]byte(oversizedAnonymous)) {
		t.Fatalf("oversized anonymous prefix let a later exact header elect text: scan=%+v", oversizedAnonymousScan)
	}
	negatives := []struct {
		name string
		data []byte
	}{
		{
			name: "inventory endpoint",
			data: []byte(profilerFrameBlockLine(5_001_000_000, "block_rq_insert: 0,1 R 4 () 2 + 3 []") + "\n"),
		},
		{
			name: "near endpoint suffix",
			data: []byte(profilerFrameBlockLine(5_001_000_000, "block_rq_issue_extra: 0,1 R 4 () 2 + 3 []") + "\n"),
		},
		{
			name: "case drift",
			data: []byte(profilerFrameBlockLine(5_001_000_000, "Block_rq_issue: 0,1 R 4 () 2 + 3 []") + "\n"),
		},
		{
			name: "missing physical delimiter",
			data: []byte(profilerFrameBlockLine(5_001_000_000, "block_rq_issue 0,1 R 4 () 2 + 3 []") + "\n"),
		},
		{
			name: "shifted physical delimiter",
			data: []byte(profilerFrameBlockLine(5_001_000_000, "block_rq_issue : 0,1 R 4 () 2 + 3 []") + "\n"),
		},
		{
			name: "anonymous prose",
			data: []byte("metadata says block_rq_issue: but has no physical ftrace header\n"),
		},
		{
			name: "structured inventory endpoint",
			data: profilerBlockTestStructuredMessage(210, profilerBlockTypedPayload(210, nil), 5_001_000_000, 40),
		},
		{
			name: "structured metadata embeds exact header after nul",
			data: structuredEmbedded,
		},
		{
			name: "malformed structured metadata embeds later exact header",
			data: malformedStructuredEmbedded,
		},
		{
			name: "not-structured anonymous prefix before exact header",
			data: anonymousThenExact,
		},
	}
	for _, test := range negatives {
		for _, route := range []struct {
			name  string
			frame func([]byte) []byte
		}{
			{name: "noncanonical", frame: func(data []byte) []byte {
				return syntheticProfilerPluginData("FTRACE-PLUGIN", data)
			}},
			{name: "rejected", frame: profilerRejectedFrameWithData},
		} {
			t.Run(test.name+"/"+route.name, func(t *testing.T) {
				extracted, sink := extractSyntheticProfilerContainer(t, route.frame(test.data))
				defer sink.cleanup()
				if extracted.SourceFailClosed || sink.opaque[pairRenderBlock] || sink.poisoned[pairRenderBlock] ||
					sink.pairRows[pairRenderBlock] != 0 {
					t.Fatalf("non-endpoint bytes guessed Block provenance: extracted=%+v opaque=%v poisoned=%v rows=%v",
						extracted, sink.opaque, sink.poisoned, sink.pairRows)
				}
			})
		}
	}

	for _, test := range []struct {
		name  string
		frame []byte
	}{
		{
			name: "exact endpoint outside data field",
			frame: protoPayload(
				protoBytes(7, []byte(exact)),
			),
		},
		{
			name:  "exact endpoint after malformed outer key",
			frame: append([]byte{0}, protoBytes(3, []byte(exact))...),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			extracted, sink := extractSyntheticProfilerContainer(t, test.frame)
			defer sink.cleanup()
			if extracted.SourceFailClosed || sink.opaque[pairRenderBlock] || sink.poisoned[pairRenderBlock] {
				t.Fatalf("unproven outer bytes guessed Block provenance: extracted=%+v opaque=%v poisoned=%v",
					extracted, sink.opaque, sink.poisoned)
			}
		})
	}
}
