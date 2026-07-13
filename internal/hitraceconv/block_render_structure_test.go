package hitraceconv

import (
	"strings"
	"testing"
)

func TestBlockRendererUsesOneTypedAuthorityWithoutDefaultFallbacks(t *testing.T) {
	block := mustReadRendererSource(t, "block_render.go")
	render := mustReadRendererSource(t, "render.go")
	official := mustReadRendererSource(t, "official_render.go")
	profiler := mustReadRendererSource(t, "profiler_ftrace_render.go")
	streamer := mustReadRendererSource(t, "streamerdb_export_raw_ftrace.go")

	for _, token := range []string{
		"type blockRenderPayload struct",
		"func renderCanonicalBlockPayload(",
		"func decodeDirectBlockPayload(",
		"func decodeProfilerBlockPayloadWithTypedAudit(",
		"func renderProfilerFtraceBlockEventWithTypedAudit(",
		"func decodeTraceDBBlockPayload(",
	} {
		if strings.Count(block, token) != 1 {
			t.Fatalf("block single authority lost %q", token)
		}
	}
	if strings.Count(block, "walkProtoFields(event.Payload") != 1 ||
		!strings.Contains(block, "var fields [8]profilerFtraceBlockFieldState") ||
		!strings.Contains(block, "Issues [profilerFtraceBlockIssuesPerEvent]profilerFtraceEventIssue") {
		t.Fatal("structured block fields no longer flow through one fixed-state typed audit")
	}
	if !strings.Contains(render, "return renderDirectBlockEvent(ev, content)") ||
		!strings.Contains(official, "return renderDirectBlockEvent(ev, content)") ||
		strings.Count(profiler, "renderProfilerFtraceBlockEventWithTypedAudit(event)") != 1 ||
		!strings.Contains(streamer, "return renderTraceDBBlockEvent(name, args, invalidKeys)") {
		t.Fatal("a direct, structured, or SQL block lane bypasses the shared typed authority")
	}
	for _, forbidden := range []string{
		`firstNonEmpty(protoString(event.Payload, 5), "RW")`,
		`return "0,0"`,
		"func blockDevText(",
		"func renderBlockRequest(",
		"func renderBlockRemap(",
		"func traceDBRenderRawBlockRequest(",
		"func traceDBRenderRawBlockRemap(",
		`traceDBRawArg(args, "RW"`,
		`"nr_sector", "nr_sectors", "sectors", "len", "length", "bytes", "nr_bytes"`,
		"func decodeProfilerBlockPayload(",
		"func renderProfilerBlockEvent(",
		"protoScalarUint(",
		"protoScalarString(",
		`fmt.Sprintf("core_field`,
		"profilerFtraceEventIssueFromLegacy(",
	} {
		if strings.Contains(block+render+official+profiler+streamer, forbidden) {
			t.Fatalf("block renderer reintroduced a fabricated fallback/second implementation: %q", forbidden)
		}
	}
}
