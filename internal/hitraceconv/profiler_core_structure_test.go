package hitraceconv

import (
	"strings"
	"testing"
)

func TestProfilerStructuredCoreHasOneCanonicalRenderAuthority(t *testing.T) {
	core := mustReadRendererSource(t, "core_payload.go")
	direct := mustReadRendererSource(t, "render.go")
	profiler := mustReadRendererSource(t, "profiler_ftrace_render.go")
	adapter := mustReadRendererSource(t, "profiler_core_payload.go")

	if strings.Count(core, "func renderCanonicalCorePayload(") != 1 {
		t.Fatal("canonical core formatter must have exactly one definition")
	}
	if !strings.Contains(direct, "renderCanonicalCorePayload(payload)") ||
		!strings.Contains(profiler, "renderCanonicalCorePayload(corePayload)") {
		t.Fatal("direct and structured core paths must converge on the canonical formatter")
	}
	for _, forbidden := range []string{"protoUint(", "protoInt(", "protoString("} {
		if strings.Contains(adapter, forbidden) {
			t.Fatalf("structured core adapter calls permissive legacy reader %q", forbidden)
		}
	}

	legacy := sourceBetween(t, profiler, "func renderProfilerFtraceEventBody(", "func safeProfilerBlockedCaller(")
	for _, field := range []string{
		"113", "119", "1400", "1401", "1402", "1500", "1501", "1502", "1503", "1504",
		"2003", "2004", "2005", "2420", "2421", "2422", "4002",
	} {
		if strings.Contains(legacy, "case "+field+":") {
			t.Fatalf("legacy structured renderer retained a second core authority for field %s", field)
		}
	}

	audit := sourceBetween(t, profiler, "func renderProfilerFtraceEventBodyWithAudit(", "func profilerFtraceCoreWireAudit(")
	decodeAt := strings.Index(audit, "decodeProfilerCorePayload(event)")
	legacyAt := strings.Index(audit, "renderProfilerFtraceEventBody(event)")
	if decodeAt < 0 || legacyAt < 0 || decodeAt > legacyAt {
		t.Fatal("structured core typed admission must run before the legacy renderer")
	}
	if !strings.Contains(audit, "case bodyRejected:") ||
		!strings.Contains(sourceBetween(t, audit, "case bodyRejected:", "if _, _, blockEvent"), "return") {
		t.Fatal("a rejected governed core payload can still fall through to a compatibility renderer")
	}
}
