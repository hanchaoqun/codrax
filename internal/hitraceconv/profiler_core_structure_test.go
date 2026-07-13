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
	for _, authority := range []string{
		"func decodeProfilerCorePayloadWithTypedAuditContext(",
		"func renderProfilerFtraceCoreEventWithTypedAuditContext(",
		"func finalizeProfilerFtraceCoreEventWithTypedAuditContext(",
	} {
		if strings.Count(adapter, authority) != 1 {
			t.Fatalf("structured core Context authority is not unique %q", authority)
		}
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
	if strings.Count(audit, "renderProfilerFtraceEventBodyWithTypedAudit(event)") != 1 ||
		strings.Count(audit, "profilerFtraceEventIssueLabels(event.Field, issues)") != 1 ||
		strings.Contains(audit, "renderProfilerFtraceCoreEventWithTypedAudit(event)") {
		t.Fatal("direct compatibility path is not a single typed-call-to-label adapter")
	}

	typed := sourceBetween(t, profiler, "func renderProfilerFtraceEventBodyWithTypedAuditAndPairContext(", "const profilerFtraceGenericIssuesPerEvent")
	if strings.Count(typed, "renderProfilerFtraceCoreEventWithTypedAuditContext(ctx, event)") != 1 ||
		!strings.Contains(typed, "profiler_core_typed_renderer_unhandled") {
		t.Fatal("typed core path lost its single Context producer or governed unhandled guard")
	}
	if strings.Contains(typed, "profilerFtraceEventDegradationCorePayload") ||
		strings.Contains(typed, "renderProfilerFtraceEventBodyWithAudit(event)") ||
		strings.Contains(typed, "renderProfilerFtraceCoreEventWithTypedAudit(event)") {
		t.Fatal("production core still has a reverse legacy issue bridge")
	}
	compatCore := sourceBetween(t, adapter, "func renderProfilerFtraceCoreEventWithTypedAudit(", "func renderProfilerFtraceCoreEventWithTypedAuditContext(")
	if strings.Count(compatCore, "renderProfilerFtraceCoreEventWithTypedAuditContext(context.Background(), event)") != 1 {
		t.Fatal("core compatibility renderer is not a Background-only adapter over the Context authority")
	}
}
