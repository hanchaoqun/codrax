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
		!strings.Contains(adapter, "renderCanonicalCorePayload(payload)") {
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

	audit := sourceBetween(t, profiler, "func renderProfilerFtraceEventBodyWithAudit(", "func renderProfilerFtraceEventBodyWithTypedAudit(")
	decodeAt := strings.Index(audit, "renderProfilerFtraceCoreEventWithTypedAudit(event)")
	genericAt := strings.Index(audit, "renderProfilerFtraceGenericEventWithTypedAudit(event)")
	if decodeAt < 0 || genericAt < 0 || decodeAt > genericAt {
		t.Fatal("structured core typed admission must run before the generic typed renderer")
	}
	if !strings.Contains(audit, "if coreErr != nil") || !strings.Contains(audit, "profilerFtraceEventIssueLabels(event.Field, issues)") {
		t.Fatal("direct core compatibility path no longer fail-closes or uses the typed-to-label adapter")
	}

	typed := sourceBetween(t, profiler, "func renderProfilerFtraceEventBodyWithTypedAudit(", "const profilerFtraceGenericIssuesPerEvent")
	if strings.Count(typed, "renderProfilerFtraceCoreEventWithTypedAudit(event)") != 1 ||
		!strings.Contains(typed, "profiler_core_typed_renderer_unhandled") {
		t.Fatal("typed core path can fall through to the legacy bridge")
	}
	legacyBridge := sourceBetween(t, typed, "legacySource :=", "if profilerFtraceEventSlot(event.Field)")
	if strings.Contains(legacyBridge, "profilerStructuredCoreSchemas") ||
		strings.Contains(legacyBridge, "profilerFtraceEventDegradationCorePayload") {
		t.Fatal("production core still has a reverse legacy issue bridge")
	}
}
