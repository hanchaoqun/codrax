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

	field410 := sourceBetween(t, profiler, "case 410:", "case 2002:")
	if strings.Contains(field410, `" cpu_id=`) || strings.Contains(field410, "protoScalarUint") {
		t.Fatalf("field410 ClkSetRate schema must not acquire a CPU dimension:\n%s", field410)
	}
	field2002 := sourceBetween(t, profiler, "case 2002:", "case 1000:")
	for _, token := range []string{"protoScalarUint(event.Payload, 3)", "appendClockSetRateCPU"} {
		if !strings.Contains(field2002, token) {
			t.Fatalf("field2002 lost %q authority:\n%s", token, field2002)
		}
	}
	if !strings.Contains(descriptors, `2002: {Field: 2002, Family: "clock", Name: "clock_set_rate"}`) {
		t.Fatal("field2002 descriptor is not pinned")
	}
	if strings.Contains(profiler, "page=0x0") {
		t.Fatal("profiler renderer must not contain a fabricated page pointer literal")
	}
	for _, token := range []string{
		"renderProfilerFtraceEventBodyWithAudit(event)",
		"degradationsByField",
		"degraded_",
		"traceDBCountSummary(counts)",
	} {
		if !strings.Contains(profiler, token) {
			t.Fatalf("profiler coverage/audit path lost %q", token)
		}
	}

	if strings.Count(render, "func formatHarmonySchedInfo(") != 1 {
		t.Fatal("packed next_info must have exactly one text-format authority")
	}
	if strings.Count(render, "func renderFilemapPageCacheBody(") != 1 ||
		!strings.Contains(profiler, "renderFilemapPageCacheBody(") ||
		!strings.Contains(render, `uniqueUintByCleanName(ev, "pg", "page")`) {
		t.Fatal("direct/profiler page-cache output must share one formatter")
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
		`uniqueUintByCleanName(ev, "s_dev", "dev")`,
		`uniqueUintByCleanName(ev, "i_ino", "ino")`,
		"func hasCleanSignedInt32Field(",
		"sDev > math.MaxUint32",
		"strictDataLocString(raw, content)",
		"legacyClockOffsetTypeAllowed(lowerType)",
		"func traceDBSingleToken(",
	} {
		if !strings.Contains(render, token) {
			t.Fatalf("renderer lost strict authority %q", token)
		}
	}
	if !strings.Contains(profiler, "core_field3_out_of_range") || !strings.Contains(profiler, "index > ^uint64(0)>>12") {
		t.Fatal("profiler page offset overflow must fail closed with coverage")
	}
	if !strings.Contains(profiler, "core_field4_out_of_range") || !strings.Contains(profiler, "sDev > math.MaxUint32") {
		t.Fatal("profiler dev_t must retain the pinned uint32 domain")
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
