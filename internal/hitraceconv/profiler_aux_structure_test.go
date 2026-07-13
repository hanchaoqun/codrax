package hitraceconv

import (
	"reflect"
	"strings"
	"testing"
)

func TestProfilerStructuredAuxSchemaIsExact(t *testing.T) {
	want := map[int]map[int]int{
		1109: {1: 0, 2: 2},
		4009: {1: 0, 2: 0, 3: 0, 4: 0, 5: 0, 6: 0, 7: 0, 8: 0},
		4010: {1: 0, 2: 0, 3: 0, 4: 0, 5: 0},
		4011: {1: 0, 2: 0, 3: 0, 4: 0, 5: 0},
		4012: {1: 0, 2: 0, 3: 0, 4: 0, 5: 0},
		4015: {
			1: 0, 2: 0, 3: 2, 4: 0, 5: 0, 6: 0, 7: 2, 8: 0, 9: 0, 10: 0, 11: 2, 12: 0,
			13: 0, 14: 0, 15: 0, 16: 0, 17: 0, 18: 0, 19: 0, 20: 0, 21: 0, 22: 0, 23: 2,
		},
		4016: {
			1: 0, 2: 0, 3: 0, 4: 0, 5: 0, 6: 0, 7: 0, 8: 0, 9: 0, 10: 0, 11: 0, 12: 0,
			13: 0, 14: 0, 15: 0, 16: 0, 17: 0, 18: 0, 19: 0, 20: 0, 21: 0, 22: 0, 23: 0, 24: 0, 25: 2,
		},
	}
	if !reflect.DeepEqual(profilerStructuredAuxSchemas, want) {
		t.Fatalf("structured aux schema drifted: got=%v want=%v", profilerStructuredAuxSchemas, want)
	}
	fieldCount := 0
	for _, schema := range want {
		fieldCount += len(schema)
	}
	if fieldCount != 73 {
		t.Fatalf("structured aux field census=%d want=73", fieldCount)
	}
}

func TestProfilerAll36DescriptorsHaveOneStrictAdmissionOwner(t *testing.T) {
	owners := map[int]string{}
	add := func(owner string, fields ...int) {
		t.Helper()
		for _, field := range fields {
			if prior := owners[field]; prior != "" {
				t.Fatalf("descriptor field %d has multiple strict owners: %s and %s", field, prior, owner)
			}
			owners[field] = owner
		}
	}
	add("block", 202, 204, 205, 209, 210, 211, 212)
	for field := range profilerStructuredCoreSchemas {
		add("shared_core", field)
	}
	for field := range profilerStructuredAuxSchemas {
		add("aux", field)
	}
	add("existing_strict", 410, 1000, 1001, 2002, 2417)

	if len(owners) != 36 || len(profilerFtraceEventDescriptors) != 36 {
		t.Fatalf("descriptor/owner cardinality drifted: owners=%d descriptors=%d", len(owners), len(profilerFtraceEventDescriptors))
	}
	for field, descriptor := range profilerFtraceEventDescriptors {
		if owners[field] == "" {
			t.Fatalf("descriptor field %d (%+v) has no strict admission owner", field, descriptor)
		}
		if descriptor.Field != field {
			t.Fatalf("descriptor key/field mismatch: key=%d descriptor=%+v", field, descriptor)
		}
	}
	for field, owner := range owners {
		if _, ok := profilerFtraceEventDescriptors[field]; !ok {
			t.Fatalf("strict owner %s references unregistered descriptor field %d", owner, field)
		}
	}
}

func TestProfilerStructuredAuxUsesOneTypedAuthorityBeforeLegacy(t *testing.T) {
	adapter := mustReadRendererSource(t, "profiler_aux_payload.go")
	profiler := mustReadRendererSource(t, "profiler_ftrace_render.go")

	for _, token := range []string{
		"func decodeProfilerAuxPayload(",
		"func renderCanonicalProfilerAuxPayload(",
		"var profilerStructuredAuxSchemas",
	} {
		if strings.Count(adapter, token) != 1 {
			t.Fatalf("aux single authority lost %q", token)
		}
	}
	for _, forbidden := range []string{"protoUint(", "protoInt(", "protoString("} {
		if strings.Contains(adapter, forbidden) {
			t.Fatalf("strict aux adapter calls permissive legacy reader %q", forbidden)
		}
	}
	if !strings.Contains(adapter, "for field := 1; field <= maxField; field++") {
		t.Fatal("aux wire audit no longer reaches the MMC field23/25 tail")
	}

	legacy := sourceBetween(t, profiler, "func renderProfilerFtraceEventBody(", "func safeProfilerBlockedCaller(")
	for _, field := range []string{"1109", "4009", "4010", "4011", "4012", "4015", "4016"} {
		if strings.Contains(legacy, "case "+field+":") {
			t.Fatalf("legacy structured renderer retained a second aux authority for field %s", field)
		}
	}
	for _, forbidden := range []string{
		"func renderProfilerFtraceF2FS(",
		`protoString(event.Payload, 23)`,
		`protoString(event.Payload, 25)`,
	} {
		if strings.Contains(profiler, forbidden) {
			t.Fatalf("permissive aux implementation survived cutover: %q", forbidden)
		}
	}

	audit := sourceBetween(t, profiler, "func renderProfilerFtraceEventBodyWithAudit(", "func renderProfilerFtraceEventBodyWithTypedAudit(")
	coreAt := strings.Index(audit, "decodeProfilerCorePayload(event)")
	auxAt := strings.Index(audit, "decodeProfilerAuxPayload(event)")
	blockAt := strings.Index(audit, "blockRenderKindForProfilerField(event.Field)")
	genericAt := strings.Index(audit, "renderProfilerFtraceGenericEventWithTypedAudit(event)")
	if coreAt < 0 || auxAt < 0 || blockAt < 0 || genericAt < 0 || !(coreAt < auxAt && auxAt < blockAt && blockAt < genericAt) {
		t.Fatalf("typed renderer order drifted: core=%d aux=%d block=%d generic=%d", coreAt, auxAt, blockAt, genericAt)
	}
	auxArm := sourceBetween(t, audit, "decodeProfilerAuxPayload(event)", "if _, _, blockEvent")
	if !strings.Contains(auxArm, "case bodyRejected:") || !strings.Contains(auxArm, "return") ||
		!strings.Contains(auxArm, "renderCanonicalProfilerAuxPayload(auxPayload)") ||
		!strings.Contains(auxArm, "profilerCanonicalLineValid(event, auxPayload.Name, body)") ||
		!strings.Contains(auxArm, "auxPayload.Degradations") {
		t.Fatal("governed aux rejection/render/line-cap choke point can fall through")
	}
}

func TestProfilerAuxCanonicalRendererPinsTruthfulLabels(t *testing.T) {
	adapter := mustReadRendererSource(t, "direct_f2fs_payload.go")
	f2fs := sourceBetween(t, adapter, "func renderCanonicalF2FSPayload(", "func f2fsRW(")
	for _, required := range []string{"pino=0x%x", "cp_reason=%d", "pos=%d", "copied=%d", "FlagsPresent"} {
		if !strings.Contains(f2fs, required) {
			t.Fatalf("F2FS canonical truth field lost %q", required)
		}
	}
	for _, forbidden := range []string{"offset=", "rw=write"} {
		if strings.Contains(f2fs, forbidden) {
			t.Fatalf("F2FS canonical renderer reintroduced false semantic %q", forbidden)
		}
	}

	auxAdapter := mustReadRendererSource(t, "profiler_aux_payload.go")
	mmcDoneArm := sourceBetween(t, auxAdapter, "case profilerAuxMMCDone:", "default:")
	if !strings.Contains(mmcDoneArm, "renderCanonicalMMCPayload(") {
		t.Fatal("structured MMC no longer delegates to the source-neutral canonical renderer")
	}
	mmcDone := mustReadRendererSource(t, "direct_mmc_payload.go")
	for _, required := range []string{"struct mmc_request[0x%x]", "cmd_err=%d", "stop_err=%d", "sbc_err=%d", "data_err=%d"} {
		if !strings.Contains(mmcDone, required) {
			t.Fatalf("MMC canonical truth field lost %q", required)
		}
	}
	for _, forbidden := range []string{" ret="} {
		if strings.Contains(mmcDone, forbidden) {
			t.Fatalf("MMC canonical renderer exposed fabricated/lossy field %q", forbidden)
		}
	}
	for _, forbidden := range []string{"CmdResponseKnown: true", "StopResponseKnown: true", "SBCResponseKnown: true"} {
		if strings.Contains(mmcDoneArm, forbidden) {
			t.Fatalf("structured MMC promoted a lossy response carrier %q", forbidden)
		}
	}
}
