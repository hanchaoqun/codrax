package hitraceconv

import (
	"os"
	"strings"
	"testing"
)

func TestDonghuRendererStructurePinsSingleAuthorities(t *testing.T) {
	profiler := mustReadRendererSource(t, "profiler_ftrace_render.go")
	render := mustReadRendererSource(t, "render.go")
	official := mustReadRendererSource(t, "official_render.go")
	descriptors := mustReadRendererSource(t, "profiler_container.go")
	filemap := mustReadRendererSource(t, "filemap_render.go")

	field410 := sourceBetween(t, profiler, "case 410:", "case 2002:")
	if strings.Contains(field410, `" cpu_id=`) || strings.Contains(field410, "protoScalarUint") {
		t.Fatalf("field410 ClkSetRate schema must not acquire a CPU dimension:\n%s", field410)
	}
	field2002 := sourceBetween(t, profiler, "case 2002:", "case 2417:")
	for _, token := range []string{"displayValue", "appendClockSetRateCPU"} {
		if !strings.Contains(field2002, token) {
			t.Fatalf("field2002 lost %q authority:\n%s", token, field2002)
		}
	}
	generic := sourceBetween(t, profiler, "func renderProfilerFtraceGenericEventWithTypedAuditContext(", "func profilerFtraceEventRenderCoverage(")
	if strings.Count(generic, "walkProfilerProtoFieldsContext(ctx, event.Payload") != 1 ||
		strings.Contains(generic, "walkProtoFields(event.Payload") {
		t.Fatal("generic 410/2002/2417 producer must parse its payload exactly once")
	}
	for _, required := range []string{
		"var fields [9]profilerFtraceGenericFieldState",
		"Issues [profilerFtraceGenericIssuesPerEvent]profilerFtraceEventIssue",
		"const profilerFtraceGenericIssuesPerEvent = 7",
	} {
		if !strings.Contains(profiler, required) {
			t.Fatalf("generic typed producer lost fixed structure %q", required)
		}
	}
	for _, forbidden := range []string{
		"protoScalarUint(", "protoScalarString(", "protoUint(", "protoInt(", "protoString(",
		"profilerFtraceEventIssueFromLegacy(", `fmt.Sprintf("core_field`, ".Error()",
	} {
		if strings.Contains(generic, forbidden) {
			t.Fatalf("generic typed producer reintroduced dynamic/rescan authority %q", forbidden)
		}
	}
	if !strings.Contains(descriptors, `{Field: 2002, Family: "clock", Name: "clock_set_rate"}`) ||
		!strings.Contains(descriptors, "profilerFtraceEventDescriptorList") ||
		!strings.Contains(descriptors, "for _, descriptor := range profilerFtraceEventDescriptorList") {
		t.Fatal("field2002 descriptor is not pinned")
	}
	if strings.Contains(profiler, "page=0x0") {
		t.Fatal("profiler renderer must not contain a fabricated page pointer literal")
	}
	consumer := sourceBetween(t, profiler, "func renderProfilerFtraceStructuredResultConsumerContext(", "func safeProfilerBlockedCaller(")
	for _, token := range []string{
		"renderProfilerFtraceEventBodyWithTypedAuditAndPairContext(ctx, event)",
		"profilerFtraceEventIssueLabels(event.Field, issues)",
		"degradationsByField",
		"degraded_",
		"traceDBCountSummary(counts)",
	} {
		if !strings.Contains(consumer, token) {
			t.Fatalf("profiler coverage/audit path lost %q", token)
		}
	}
	if strings.Contains(consumer, "renderProfilerFtraceEventBodyWithTypedAuditAndPair(event)") {
		t.Fatal("structured production consumer bypassed the Context typed authority")
	}

	if strings.Count(render, "func formatHarmonySchedInfo(") != 1 {
		t.Fatal("packed next_info must have exactly one text-format authority")
	}
	typed := sourceBetween(t, profiler, "func renderProfilerFtraceEventBodyWithTypedAuditAndPairContext(", "const profilerFtraceGenericIssuesPerEvent")
	if strings.Count(filemap, "func renderCanonicalFilemapPayload(") != 1 ||
		!strings.Contains(render, "decodeDirectFilemapPayload(ev)") ||
		strings.Count(typed, "renderProfilerFtraceFilemapEventWithTypedAuditContext(ctx, event)") != 1 ||
		strings.Contains(typed, "renderProfilerFtraceFilemapEventWithTypedAudit(event)") ||
		strings.Contains(render, `uniqueUintByCleanName(ev, "pg", "page")`) ||
		strings.Contains(filemap, "page=0x0") {
		t.Fatal("direct/profiler page-cache output must share one typed formatter without page-pointer fallback")
	}
	if strings.Count(render, "func appendClockSetRateCPU(") != 1 ||
		!strings.Contains(profiler, "appendClockSetRateCPU") {
		t.Fatal("direct/profiler clock CPU range must share one gate")
	}
	if !strings.Contains(official, "if hasCleanIntegerField(ev, \"expeller_type\")") ||
		!strings.Contains(official, "schedSwitchHarmonyExtras(ev, content)") {
		t.Fatal("official sched optional fields lost independent presence gates")
	}
	if strings.Count(render, "hasCleanStringField(ev") < 2 || strings.Count(render, "hasCleanHarmonyCommField(ev") < 2 ||
		!strings.Contains(render, "traceDBSinglePhysicalLine(value, false)") ||
		!strings.Contains(render, "traceDBSinglePhysicalLine(value, true)") {
		t.Fatal("direct sched core strings lost typed/control-character admission")
	}
	if strings.Count(render, "hasCleanPIDTIDField(ev") < 5 || !strings.Contains(render, "value <= math.MaxInt32") {
		t.Fatal("scheduler PID/TID identities lost the typed int32 admission gate")
	}
	if !strings.Contains(render, `safeOptionalCleanString(ev, content, "cg", "cgroup")`) ||
		!strings.Contains(render, "func strictClockName(") || !strings.Contains(render, "func numericFieldTypeAllowed(") {
		t.Fatal("clock core and optional cgroup lost single-line/UTF-8 admission")
	}
	for _, token := range []string{
		"func uniqueFieldByCleanNames(",
		"func hasCleanSignedInt32Field(",
		"strictDataLocString(raw, content)",
		"legacyClockOffsetTypeAllowed(lowerType)",
		"func traceDBSingleToken(",
	} {
		if !strings.Contains(render, token) {
			t.Fatalf("renderer lost strict authority %q", token)
		}
	}
	if !strings.Contains(filemap, "math.MaxInt64 >> 12") ||
		!strings.Contains(filemap, "profilerFtraceEventIssueFilemapIndexInvalid") {
		t.Fatal("profiler page offset overflow must fail closed with coverage")
	}
	if !strings.Contains(filemap, "math.MaxUint32") ||
		!strings.Contains(filemap, "profilerFtraceEventIssueFilemapDeviceInvalid") {
		t.Fatal("profiler dev_t must retain the pinned uint32 domain")
	}
}

func TestSystraceHeaderPIDColumnCanonicalAcrossEmitters(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	legacy := "%16s-%" + "-6d ("
	canonical := "%16s-%" + "-5d ("
	canonicalCount := 0
	perfHeaderClampCount := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		source := string(body)
		if strings.Contains(source, legacy) {
			t.Fatalf("%s reintroduced the non-canonical six-column PID formatter", name)
		}
		canonicalCount += strings.Count(source, canonical)
		perfHeaderClampCount += strings.Count(source, "perfTraceHeaderComm(comm)")
	}
	// Direct raw, shared structured/SQL, and four standalone perf emitters
	// deliberately share the Donghu-compatible PID column shape.
	if canonicalCount != 6 {
		t.Fatalf("canonical systrace PID formatter authorities=%d, want 6", canonicalCount)
	}
	if perfHeaderClampCount != 4 {
		t.Fatalf("standalone perf header comm clamps=%d, want 4", perfHeaderClampCount)
	}
}

func mustReadRendererSource(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func sourceBetween(t *testing.T, source, start, end string) string {
	t.Helper()
	startAt := strings.Index(source, start)
	if startAt < 0 {
		t.Fatalf("source missing start marker %q", start)
	}
	endAt := strings.Index(source[startAt+len(start):], end)
	if endAt < 0 {
		t.Fatalf("source missing end marker %q after %q", end, start)
	}
	return source[startAt : startAt+len(start)+endAt]
}
