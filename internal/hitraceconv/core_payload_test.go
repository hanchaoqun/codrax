package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectCoreCanonicalPayloadMatrix(t *testing.T) {
	for _, test := range []struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{
		wakeCoreCase("sched_wakeup", 0, 140, 0, "app", "comm"),
		wakeCoreCase("sched_wakeup_new", 20, 159, 2, "new-app", "comm"),
		wakeCoreCase("sched_waking", 21, 160, 3, "hm-app", "pname"),
		wakeCoreCase16("sched_wakeup_new", 22, 159, 4, "new16-app", "comm"),
		cpuCoreCase("cpu_frequency", 0, 0),
		cpuCoreCase("cpu_idle", ^uint32(0), 2),
		cpuLimitsCoreCase("min_freq", "max_freq", 0, 0, 0),
		cpuLimitsCoreCase("min", "max", 418000, 1720000, 3),
		blockedCanonicalCoreCase(),
		binderTransactionCoreCase(true),
		binderTransactionCoreCase(false),
		binderReceivedCoreCase(true),
		binderReceivedCoreCase(false),
		irqEntryDataLocCoreCase(),
		irqExitCoreCase(2),
		softIRQCoreCase("softirq_entry", 0),
		softIRQCoreCase("softirq_exit", 5),
		softIRQCoreCase("softirq_raise", 9),
		ipiPointerCoreCase("ipi_entry"),
		ipiPointerCoreCase("ipi_exit"),
		ipiRaisePointerCoreCase(),
		ipiRaisePointerZeroCoreCase(),
	} {
		t.Run(test.name, func(t *testing.T) {
			event := decodeEvent(test.format, test.content)
			payload, admission, reason := decodeDirectCorePayload(test.ctx, event, test.content)
			if admission != bodyAdmitted || reason != "" {
				t.Fatalf("admission=%d reason=%q payload=%+v", admission, reason, payload)
			}
			body, ok := renderCanonicalCorePayload(payload)
			if !ok || body != test.want {
				t.Fatalf("canonical body: ok=%v got=%q want=%q payload=%+v", ok, body, test.want, payload)
			}
		})
	}
}

func wakeCoreCase16(name string, pid, priority, cpu int64, comm, commField string) struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	format := eventFormat{Name: name, Fields: []eventField{
		{Type: "char", Name: commField + "[16]", Offset: 0, Size: 16},
		{Type: "int", Name: "pid", Offset: 16, Size: 4, Signed: true},
		{Type: "int", Name: "prio", Offset: 20, Size: 2, Signed: true},
		{Type: "int", Name: "target_cpu", Offset: 22, Size: 4, Signed: true},
	}}
	content := make([]byte, 26)
	copy(content[:16], []byte(comm))
	binary.LittleEndian.PutUint32(content[16:20], uint32(pid))
	binary.LittleEndian.PutUint16(content[20:22], uint16(priority))
	binary.LittleEndian.PutUint32(content[22:26], uint32(cpu))
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: name, format: format, content: content, want: "comm=" + comm + " pid=" + coreTestItoa64(pid) + " prio=" + coreTestItoa64(priority) + " target_cpu=" + coreTestThreeDigits(cpu)}
}

func TestDirectCoreProfileAndBoundaryAdmission(t *testing.T) {
	t.Run("wakeup 16-bit priority remains signed and exact", func(t *testing.T) {
		test := wakeCoreCase16("sched_wakeup_new", 20, 159, 2, "app", "comm")
		test.format.Fields[2].Signed = false
		payload, admission, reason := decodeDirectCorePayload(
			coreDecodeContext{}, decodeEvent(test.format, test.content), test.content)
		if admission != bodyRejected || reason != "missing_or_invalid_priority" || payload.Wakeup != nil {
			t.Fatalf("unsigned 16-bit priority escaped exact profile: admission=%d reason=%q payload=%+v",
				admission, reason, payload)
		}
	})

	t.Run("Harmony packed wakeup target CPU is exact signed s8", func(t *testing.T) {
		test := wakeCoreCase16("sched_wakeup_new", 20, 159, 2, "app", "comm")
		test.format.Fields[3] = eventField{
			Type: "s8", Name: "target_cpu", Offset: 22, Size: 1, Signed: true,
		}
		test.content = test.content[:23]
		test.content[22] = 2
		payload, admission, reason := decodeDirectCorePayload(
			coreDecodeContext{}, decodeEvent(test.format, test.content), test.content)
		if admission != bodyAdmitted || reason != "" || payload.Wakeup == nil ||
			payload.Wakeup.TargetCPU != 2 {
			t.Fatalf("packed wakeup target CPU rejected: admission=%d reason=%q payload=%+v",
				admission, reason, payload)
		}
	})

	t.Run("wakeup display corruption degrades without changing hard tuple", func(t *testing.T) {
		test := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
		copy(test.content[:16], []byte("bad\nname"))
		payload, admission, reason := decodeDirectCorePayload(coreDecodeContext{}, decodeEvent(test.format, test.content), test.content)
		if admission != bodyAdmitted || reason != "" || payload.Wakeup == nil || payload.Wakeup.Comm != "<...>" ||
			payload.Wakeup.PID != 20 || payload.Wakeup.Priority != 140 || payload.Wakeup.TargetCPU != 0 {
			t.Fatalf("display-only degradation changed hard wake tuple: admission=%d reason=%q payload=%+v", admission, reason, payload)
		}
	})

	t.Run("wakeup display overlap cannot erase hard tuple", func(t *testing.T) {
		test := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
		test.format.Fields[0].Offset = 16
		payload, admission, reason := decodeDirectCorePayload(coreDecodeContext{}, decodeEvent(test.format, test.content), test.content)
		if admission != bodyAdmitted || reason != "" || payload.Wakeup == nil || payload.Wakeup.Comm != "<...>" ||
			payload.Wakeup.PID != 20 || payload.Wakeup.Priority != 140 || payload.Wakeup.TargetCPU != 0 {
			t.Fatalf("display overlap changed hard tuple: admission=%d reason=%q payload=%+v", admission, reason, payload)
		}
	})

	t.Run("blocked symbol failure degrades to raw caller", func(t *testing.T) {
		format, content := blockedCoreFixture(true)
		for index := 37; index < 53; index++ {
			content[index] = 0
		}
		copy(content[37:53], []byte("bad\nfunction"))
		payload, admission, reason := decodeDirectCorePayload(coreDecodeContext{}, decodeEvent(format, content), content)
		if admission != bodyAdmitted || reason != "" || payload.Blocked == nil || payload.Blocked.CallerSymbolized {
			t.Fatalf("optional symbol failure rejected or forged: admission=%d reason=%q payload=%+v", admission, reason, payload)
		}
		body, _ := renderCanonicalCorePayload(payload)
		if body != "pid=324 iowait=0 caller=unknown caller_raw=0x1234 caller_quality=opaque delay=7" {
			t.Fatalf("raw blocked fallback mismatch: %q", body)
		}
	})

	for _, test := range []struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{
		invalidWakeMissingTarget(),
		invalidWakeWrongTargetSign(),
		invalidWakeTruncatedTarget(),
		invalidCPUOutOfRange(),
		invalidCPULimitsMixedProfile(),
		invalidCPULimitsOrder(),
		invalidBinderMixedProfile(),
		invalidBinderReply(),
		invalidIRQNumericName(),
		invalidSoftIRQVector(),
		invalidIPIMissingPrintk(),
		invalidIPIConflictedPrintk(),
		invalidIPIOversizeMask(),
		invalidIPIMixedProfile(),
	} {
		t.Run(test.name, func(t *testing.T) {
			_, admission, reason := decodeDirectCorePayload(test.ctx, decodeEvent(test.format, test.content), test.content)
			if admission != bodyRejected || reason != test.reason {
				t.Fatalf("admission=%d reason=%q want=%q", admission, reason, test.reason)
			}
		})
	}

	vendor := eventFormat{Name: "vendor_event", Fields: []eventField{{Name: "value", Offset: 0, Size: 4}}}
	if _, admission, reason := decodeDirectCorePayload(coreDecodeContext{}, decodeEvent(vendor, make([]byte, 4)), make([]byte, 4)); admission != bodyUnsupported || reason != "" {
		t.Fatalf("unsupported negative control changed: admission=%d reason=%q", admission, reason)
	}
}

func TestDirectCoreStrictPhysicalProfilesAndLayouts(t *testing.T) {
	t.Run("wakeup dual display aliases only degrade display", func(t *testing.T) {
		test := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
		test.format.Fields = append(test.format.Fields, eventField{Type: "char", Name: "pname[16]", Offset: 28, Size: 16})
		test.content = append(test.content, make([]byte, 16)...)
		copy(test.content[28:], []byte("renamed"))
		payload, admission, reason := decodeDirectCorePayload(coreDecodeContext{}, decodeEvent(test.format, test.content), test.content)
		if admission != bodyAdmitted || reason != "" || payload.Wakeup == nil || payload.Wakeup.Comm != "<...>" ||
			payload.Wakeup.PID != 20 || payload.Wakeup.Priority != 140 || payload.Wakeup.TargetCPU != 0 {
			t.Fatalf("display alias changed hard tuple: admission=%d reason=%q payload=%+v", admission, reason, payload)
		}
	})

	for _, test := range []struct {
		name   string
		format eventFormat
		body   []byte
		ctx    coreDecodeContext
		reason string
	}{
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
			item.format.Fields[1].Type = "void *"
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"wake_pointer_pid", item.format, item.content, coreDecodeContext{}, "missing_or_invalid_pid"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
			item.format.Fields[1].Name = "pid[1]"
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"wake_array_pid", item.format, item.content, coreDecodeContext{}, "missing_or_invalid_pid"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
			item.format.Fields[2].Offset = item.format.Fields[1].Offset
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"wake_hard_field_overlap", item.format, item.content, coreDecodeContext{}, "invalid_descriptor_layout"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
			item.format.Fields = append(item.format.Fields, eventField{Type: "unsigned int", Name: "vendor_shadow", Offset: 16, Size: 4})
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"wake_unknown_field_overlaps_pid", item.format, item.content, coreDecodeContext{}, "invalid_descriptor_layout"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
			item.format.Fields[1].Name = "__data_loc_pid"
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"wake_prefixed_pid_is_not_canonical", item.format, item.content, coreDecodeContext{}, "missing_or_invalid_pid"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			format := eventFormat{ID: 83, Name: "sched_wakeup", Fields: []eventField{
				{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
				{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
				{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
				{Type: "int", Name: "pid", Offset: 0, Size: 4, Signed: true},
				{Type: "int", Name: "prio", Offset: 8, Size: 4, Signed: true},
				{Type: "int", Name: "target_cpu", Offset: 12, Size: 4, Signed: true},
			}}
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"wake_body_reuses_implicit_event_id", format, make([]byte, 16), coreDecodeContext{}, "invalid_descriptor_layout"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := cpuCoreCase("cpu_frequency", 1, 0)
			item.format.Fields[0].Type = "unsigned long"
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"cpu_state_wrong_physical_type", item.format, item.content, coreDecodeContext{}, "missing_or_invalid_state"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := irqEntryDataLocCoreCase()
			item.format.Fields[1].Type = "__data_loc unsigned long[]"
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"irq_wrong_dynamic_type", item.format, item.content, coreDecodeContext{}, "missing_or_invalid_irq_name"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := irqEntryDataLocCoreCase()
			item.format.Fields[1].Signed = true
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"irq_signed_dynamic_locator", item.format, item.content, coreDecodeContext{}, "missing_or_invalid_irq_name"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := irqEntryDataLocCoreCase()
			binary.LittleEndian.PutUint32(item.content[4:8], uint32((4<<16)|4))
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"irq_dynamic_points_into_fixed_fields", item.format, item.content, coreDecodeContext{}, "missing_or_invalid_irq_name"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			content := make([]byte, 5)
			binary.LittleEndian.PutUint32(content[:4], 17)
			content[4] = 'x'
			format := eventFormat{Name: "irq_handler_entry", Fields: []eventField{
				{Type: "int", Name: "irq", Offset: 0, Size: 4, Signed: true},
				{Type: "unsigned char", Name: "name", Offset: 4, Size: 1},
			}}
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"irq_scalar_char_is_not_string", format, content, coreDecodeContext{}, "missing_or_invalid_irq_name"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := ipiPointerCoreCase("ipi_entry")
			item.format.Fields[0].Type = "char **"
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"ipi_double_pointer_reason", item.format, item.content, item.ctx, "missing_or_invalid_reason"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := ipiRaisePointerCoreCase()
			item.format.Fields[0].Type = "__data_loc unsigned long long[]"
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"ipi_mask_substring_profile", item.format, item.content, item.ctx, "missing_or_invalid_target_mask"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := ipiPointerCoreCase("ipi_entry")
			item.ctx.PrintkFormats[0x1234] = `forged"reason`
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"ipi_reason_quote_injection", item.format, item.content, item.ctx, "missing_or_invalid_reason"}
		}(),
		func() struct {
			name   string
			format eventFormat
			body   []byte
			ctx    coreDecodeContext
			reason string
		} {
			item := binderTransactionCoreCase(true)
			binary.LittleEndian.PutUint32(item.content[4:8], ^uint32(0))
			return struct {
				name   string
				format eventFormat
				body   []byte
				ctx    coreDecodeContext
				reason string
			}{"binder_negative_endpoint", item.format, item.content, coreDecodeContext{}, "invalid_transaction_endpoint"}
		}(),
	} {
		t.Run(test.name, func(t *testing.T) {
			_, admission, reason := decodeDirectCorePayload(test.ctx, decodeEvent(test.format, test.body), test.body)
			if admission != bodyRejected || reason != test.reason {
				t.Fatalf("admission=%d reason=%q want=%q", admission, reason, test.reason)
			}
		})
	}

	for _, name := range []string{
		"sched_wakeup", "sched_wakeup_new", "sched_waking", "sched_blocked_reason",
		"cpu_frequency", "cpu_frequency_limits", "cpu_idle",
		"binder_transaction", "binder_transaction_received",
		"irq_handler_entry", "irq_handler_exit", "softirq_entry", "softirq_exit", "softirq_raise",
		"ipi_entry", "ipi_exit", "ipi_raise",
	} {
		_, admission, reason := decodeDirectCorePayload(coreDecodeContext{}, decodeEvent(eventFormat{Name: name}, nil), nil)
		if admission != bodyRejected || reason == "" {
			t.Fatalf("closed family %s escaped empty-payload rejection: admission=%d reason=%q", name, admission, reason)
		}
	}
}

func TestDirectCoreHeaderCPUAndPayloadCPUStayIndependent(t *testing.T) {
	t.Run("wakeup target CPU", func(t *testing.T) {
		format := eventFormat{ID: 80, Name: "sched_wakeup", Fields: []eventField{
			{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
			{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
			{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
			{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "char", Name: "comm[16]", Offset: 8, Size: 16},
			{Type: "int", Name: "pid", Offset: 24, Size: 4, Signed: true},
			{Type: "int", Name: "prio", Offset: 28, Size: 4, Signed: true},
			{Type: "int", Name: "target_cpu", Offset: 32, Size: 4, Signed: true},
		}}
		content := make([]byte, 36)
		binary.LittleEndian.PutUint16(content[:2], 80)
		binary.LittleEndian.PutUint32(content[4:8], 10)
		copy(content[8:24], []byte("app\x00"))
		binary.LittleEndian.PutUint32(content[24:28], 20)
		binary.LittleEndian.PutUint32(content[28:32], 140)
		line, known := renderEventLine(renderContext{}, 1_000, 4, format, content)
		if !known || !strings.Contains(line, "[004]") || !strings.Contains(line, "target_cpu=000") {
			t.Fatalf("header and wake target CPU were merged: known=%v line=%q", known, line)
		}
	})

	t.Run("frequency payload CPU", func(t *testing.T) {
		format := eventFormat{ID: 81, Name: "cpu_frequency", Fields: []eventField{
			{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
			{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
			{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
			{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "unsigned int", Name: "state", Offset: 8, Size: 4},
			{Type: "unsigned int", Name: "cpu_id", Offset: 12, Size: 4},
		}}
		content := make([]byte, 16)
		binary.LittleEndian.PutUint16(content[:2], 81)
		binary.LittleEndian.PutUint32(content[4:8], 10)
		binary.LittleEndian.PutUint32(content[8:12], 840000)
		binary.LittleEndian.PutUint32(content[12:16], 2)
		line, known := renderEventLine(renderContext{}, 1_000, 4, format, content)
		if !known || !strings.Contains(line, "[004]") || !strings.Contains(line, "cpu_id=2") {
			t.Fatalf("header and frequency payload CPU were merged: known=%v line=%q", known, line)
		}
	})
}

func TestDirectCoreCompleteLineLimitRejectsWithoutTruncation(t *testing.T) {
	const address = 0x1234
	format := eventFormat{ID: 82, Name: "ipi_entry", Fields: []eventField{
		{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
		{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
		{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
		{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
		{Type: "const char *", Name: "reason", Offset: 8, Size: 8},
	}}
	content := make([]byte, 16)
	binary.LittleEndian.PutUint16(content[:2], 82)
	binary.LittleEndian.PutUint32(content[4:8], 100)
	binary.LittleEndian.PutUint64(content[8:16], address)
	ctx := renderContext{printkFormats: map[uint64]string{address: strings.Repeat("x", maxTraceDBSystraceLineBytes)}}
	line, admission, reason, envelopeOK := renderEventLineDecision(ctx, 1_000, 0, format, content)
	if !envelopeOK || admission != bodyRejected || reason != "invalid_rendered_line" || len(line) <= maxTraceDBSystraceLineBytes {
		t.Fatalf("overlong core line did not fail closed: envelope=%v admission=%d reason=%q len=%d", envelopeOK, admission, reason, len(line))
	}
	if wrapped, known := renderEventLine(ctx, 1_000, 0, format, content); known || wrapped != "" {
		t.Fatalf("legacy wrapper exposed a rejected governed line: known=%v line_len=%d", known, len(wrapped))
	}
}

func TestDirectCoreNonClosedVendorCompatibilityIsPreserved(t *testing.T) {
	format := eventFormat{Name: "vendor_softirq_latency", Fields: []eventField{{Type: "unsigned int", Name: "vec", Offset: 0, Size: 4}}}
	content := make([]byte, 4)
	binary.LittleEndian.PutUint32(content, 5)
	body, admission, reason := renderEventBodyDecision(coreDecodeContext{}, decodeEvent(format, content), content, 0)
	if admission != bodyAdmitted || reason != "" || body != "vec=5 [action=IRQ_POLL]" {
		t.Fatalf("non-closed vendor compatibility changed: admission=%d reason=%q body=%q", admission, reason, body)
	}
}

func TestBuiltinSysKnownCoreRejectNeverFallsBackHeaderOnly(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "core-reject.sys")
	output := filepath.Join(dir, "core-reject.systrace")
	bad := syntheticWakeupContent(10)[:32]
	good := syntheticWakeupContent(10)
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(syntheticEventFormat()))
	writeSegment(&capture, segmentCmdlines, []byte("36379 app\n"))
	writeSegment(&capture, segmentTGIDs, []byte("36379 36379\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 10, OffsetNS: 0, Content: bad},
		{EventID: 10, OffsetNS: 1_000, Content: good},
	}))
	if err := os.WriteFile(input, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 1 || result.UnknownEventCount != 0 || result.MissingFormatCount != 0 ||
		strings.Count(string(body), "sched_wakeup:") != 1 {
		t.Fatalf("known reject escaped or sibling was lost: result=%+v\n%s", result, body)
	}
	caveats := strings.Join(result.Caveats, "\n")
	if !strings.Contains(caveats, "1 governed direct ftrace event row") ||
		!strings.Contains(caveats, "sched_wakeup_missing_or_invalid_target_cpu=1") {
		t.Fatalf("known reject coverage missing: %s", caveats)
	}
}

func wakeCoreCase(name string, pid, priority, cpu int64, comm, commField string) struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	format := eventFormat{Name: name, Fields: []eventField{
		{Type: "char", Name: commField + "[16]", Offset: 0, Size: 16},
		{Type: "int", Name: "pid", Offset: 16, Size: 4, Signed: true},
		{Type: "int", Name: "prio", Offset: 20, Size: 4, Signed: true},
		{Type: "int", Name: "target_cpu", Offset: 24, Size: 4, Signed: true},
	}}
	content := make([]byte, 28)
	copy(content[:16], []byte(comm))
	binary.LittleEndian.PutUint32(content[16:20], uint32(pid))
	binary.LittleEndian.PutUint32(content[20:24], uint32(priority))
	binary.LittleEndian.PutUint32(content[24:28], uint32(cpu))
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: name, format: format, content: content, want: "comm=" + comm + " pid=" + coreTestItoa64(pid) + " prio=" + coreTestItoa64(priority) + " target_cpu=" + coreTestThreeDigits(cpu)}
}

func cpuCoreCase(name string, state, cpu uint32) struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	format := eventFormat{Name: name, Fields: []eventField{
		{Type: "unsigned int", Name: "state", Offset: 0, Size: 4},
		{Type: "unsigned int", Name: "cpu_id", Offset: 4, Size: 4},
	}}
	content := make([]byte, 8)
	binary.LittleEndian.PutUint32(content[:4], state)
	binary.LittleEndian.PutUint32(content[4:8], cpu)
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: name, format: format, content: content, want: "state=" + coreTestUtoa64(uint64(state)) + " cpu_id=" + coreTestUtoa64(uint64(cpu))}
}

func cpuLimitsCoreCase(minName, maxName string, min, max, cpu uint32) struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	format := eventFormat{Name: "cpu_frequency_limits", Fields: []eventField{
		{Type: "unsigned int", Name: minName, Offset: 0, Size: 4},
		{Type: "unsigned int", Name: maxName, Offset: 4, Size: 4},
		{Type: "unsigned int", Name: "cpu_id", Offset: 8, Size: 4},
	}}
	content := make([]byte, 12)
	binary.LittleEndian.PutUint32(content[:4], min)
	binary.LittleEndian.PutUint32(content[4:8], max)
	binary.LittleEndian.PutUint32(content[8:12], cpu)
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: minName + "_profile", format: format, content: content, want: "min=" + coreTestUtoa64(uint64(min)) + " max=" + coreTestUtoa64(uint64(max)) + " cpu_id=" + coreTestUtoa64(uint64(cpu))}
}

func binderTransactionCoreCase(official bool) struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	names := []string{"transaction", "dest_node", "dest_proc", "dest_thread"}
	if official {
		names = []string{"debug_id", "target_node", "to_proc", "to_thread"}
	}
	fields := make([]eventField, 0, 7)
	for index, name := range names {
		fields = append(fields, eventField{Type: "int", Name: name, Offset: index * 4, Size: 4, Signed: true})
	}
	fields = append(fields,
		eventField{Type: "int", Name: "reply", Offset: 16, Size: 4, Signed: true},
		eventField{Type: "unsigned int", Name: "code", Offset: 20, Size: 4},
		eventField{Type: "unsigned int", Name: "flags", Offset: 24, Size: 4})
	content := make([]byte, 28)
	for index, value := range []uint32{42, 0, 500, 0, 0, 0xa, 0x10} {
		binary.LittleEndian.PutUint32(content[index*4:index*4+4], value)
	}
	profile := "legacy"
	if official {
		profile = "official"
	}
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: "binder_transaction_" + profile, format: eventFormat{Name: "binder_transaction", Fields: fields}, content: content,
		want: "transaction=42 dest_node=0 dest_proc=500 dest_thread=0 reply=0 flags=0x10 code=0xa"}
}

func binderReceivedCoreCase(official bool) struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	name := "transaction"
	if official {
		name = "debug_id"
	}
	content := make([]byte, 4)
	binary.LittleEndian.PutUint32(content, 42)
	profile := "legacy"
	if official {
		profile = "official"
	}
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: "binder_received_" + profile, format: eventFormat{Name: "binder_transaction_received", Fields: []eventField{{Type: "int", Name: name, Offset: 0, Size: 4, Signed: true}}}, content: content, want: "transaction=42"}
}

func irqEntryDataLocCoreCase() struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	payload := []byte("arch_timer\x00")
	content := make([]byte, 8+len(payload))
	binary.LittleEndian.PutUint32(content[:4], 17)
	binary.LittleEndian.PutUint32(content[4:8], uint32((len(payload)<<16)|8))
	copy(content[8:], payload)
	format := eventFormat{Name: "irq_handler_entry", Fields: []eventField{
		{Type: "int", Name: "irq", Offset: 0, Size: 4, Signed: true},
		{Type: "__data_loc char[]", Name: "name", Offset: 4, Size: 4},
	}}
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: "irq_entry_data_loc", format: format, content: content, want: "irq=17 name=arch_timer"}
}

func irqExitCoreCase(ret int64) struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	content := make([]byte, 8)
	binary.LittleEndian.PutUint32(content[:4], 17)
	binary.LittleEndian.PutUint32(content[4:8], uint32(ret))
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: "irq_exit_ret_nonzero", format: eventFormat{Name: "irq_handler_exit", Fields: []eventField{
		{Type: "int", Name: "irq", Offset: 0, Size: 4, Signed: true},
		{Type: "int", Name: "ret", Offset: 4, Size: 4, Signed: true},
	}}, content: content, want: "irq=17 ret=handled"}
}

func softIRQCoreCase(name string, vec uint32) struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	content := make([]byte, 4)
	binary.LittleEndian.PutUint32(content, vec)
	action := softirqAction(int64(vec))
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: name, format: eventFormat{Name: name, Fields: []eventField{{Type: "unsigned int", Name: "vec", Offset: 0, Size: 4}}}, content: content,
		want: "vec=" + coreTestUtoa64(uint64(vec)) + " [action=" + action + "]"}
}

func ipiPointerCoreCase(name string) struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	const address = 0x1234
	content := make([]byte, 8)
	binary.LittleEndian.PutUint64(content, address)
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: name + "_printk_pointer", format: eventFormat{Name: name, Fields: []eventField{{Type: "const char *", Name: "reason", Offset: 0, Size: 8}}}, content: content,
		ctx: coreDecodeContext{PrintkFormats: map[uint64]string{address: "Rescheduling interrupts"}}, want: "(Rescheduling interrupts)"}
}

func ipiRaisePointerCoreCase() struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	const address = 0x1234
	content := make([]byte, 24)
	binary.LittleEndian.PutUint32(content[:4], uint32((8<<16)|16))
	binary.LittleEndian.PutUint64(content[8:16], address)
	binary.LittleEndian.PutUint64(content[16:24], 0x10)
	format := eventFormat{Name: "ipi_raise", Fields: []eventField{
		{Type: "__data_loc unsigned long[]", Name: "target_cpus", Offset: 0, Size: 4},
		{Type: "const char *", Name: "reason", Offset: 8, Size: 8},
	}}
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: "ipi_raise_kernel_profile", format: format, content: content,
		ctx: coreDecodeContext{PrintkFormats: map[uint64]string{address: "Rescheduling interrupts"}}, want: "target_mask=16 (Rescheduling interrupts)"}
}

func ipiRaisePointerZeroCoreCase() struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	test := ipiRaisePointerCoreCase()
	binary.LittleEndian.PutUint64(test.content[16:24], 0)
	test.name = "ipi_raise_zero_mask"
	test.want = "target_mask=0 (Rescheduling interrupts)"
	return test
}

func blockedCanonicalCoreCase() struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	want    string
} {
	format, content := blockedCoreFixture(true)
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		want    string
	}{name: "blocked_symbol_delay", format: format, content: content,
		want: "pid=324 iowait=0 caller=worker_fn+0x10/0x20[kernel] delay=7"}
}

func blockedCoreFixture(symbol bool) (eventFormat, []byte) {
	fields := []eventField{
		{Type: "int", Name: "pid", Offset: 0, Size: 4, Signed: true},
		{Type: "void *", Name: "caller", Offset: 4, Size: 8},
		{Type: "bool", Name: "io_wait", Offset: 12, Size: 1},
		{Type: "unsigned long", Name: "delay", Offset: 13, Size: 8},
	}
	content := make([]byte, 61)
	binary.LittleEndian.PutUint32(content[:4], 324)
	binary.LittleEndian.PutUint64(content[4:12], 0x1234)
	binary.LittleEndian.PutUint64(content[13:21], 7<<10)
	if symbol {
		fields = append(fields,
			eventField{Type: "unsigned long", Name: "offset", Offset: 21, Size: 8},
			eventField{Type: "unsigned long", Name: "size", Offset: 29, Size: 8},
			eventField{Type: "char", Name: "func_name[16]", Offset: 37, Size: 16},
			eventField{Type: "char", Name: "mod_name[8]", Offset: 53, Size: 8})
		binary.LittleEndian.PutUint64(content[21:29], 0x10)
		binary.LittleEndian.PutUint64(content[29:37], 0x20)
		copy(content[37:53], []byte("worker_fn"))
		copy(content[53:61], []byte("kernel"))
	}
	return eventFormat{Name: "sched_blocked_reason", Fields: fields}, content
}

func invalidWakeMissingTarget() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
	test.format.Fields = test.format.Fields[:3]
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"wake_missing_target", test.format, test.content, coreDecodeContext{}, "missing_or_invalid_target_cpu"}
}

func invalidWakeWrongTargetSign() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
	test.format.Fields[3].Signed = false
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"wake_wrong_target_sign", test.format, test.content, coreDecodeContext{}, "missing_or_invalid_target_cpu"}
}

func invalidWakeTruncatedTarget() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := wakeCoreCase("sched_wakeup", 20, 140, 0, "app", "comm")
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"wake_truncated_target", test.format, test.content[:24], coreDecodeContext{}, "missing_or_invalid_target_cpu"}
}

func invalidCPUOutOfRange() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := cpuCoreCase("cpu_frequency", 1, 4096)
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"cpu_out_of_range", test.format, test.content, coreDecodeContext{}, "missing_or_invalid_cpu_id"}
}

func invalidCPULimitsMixedProfile() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := cpuLimitsCoreCase("min_freq", "max_freq", 1, 2, 0)
	test.format.Fields = append(test.format.Fields, eventField{Type: "unsigned int", Name: "min", Offset: 12, Size: 4})
	test.content = append(test.content, make([]byte, 4)...)
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"cpu_limits_mixed_profile", test.format, test.content, coreDecodeContext{}, "ambiguous_limits_profile"}
}

func invalidCPULimitsOrder() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := cpuLimitsCoreCase("min_freq", "max_freq", 2, 1, 0)
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"cpu_limits_order", test.format, test.content, coreDecodeContext{}, "invalid_limits_order"}
}

func invalidBinderMixedProfile() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := binderTransactionCoreCase(true)
	test.format.Fields = append(test.format.Fields, eventField{Type: "int", Name: "transaction", Offset: 28, Size: 4, Signed: true})
	test.content = append(test.content, make([]byte, 4)...)
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"binder_mixed_profile", test.format, test.content, coreDecodeContext{}, "mixed_transaction_profile"}
}

func invalidBinderReply() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := binderTransactionCoreCase(true)
	binary.LittleEndian.PutUint32(test.content[16:20], 2)
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"binder_reply", test.format, test.content, coreDecodeContext{}, "missing_or_invalid_reply"}
}

func invalidIRQNumericName() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	content := make([]byte, 8)
	binary.LittleEndian.PutUint32(content[:4], 17)
	binary.LittleEndian.PutUint32(content[4:8], 8)
	format := eventFormat{Name: "irq_handler_entry", Fields: []eventField{
		{Type: "int", Name: "irq", Offset: 0, Size: 4, Signed: true},
		{Type: "unsigned int", Name: "name", Offset: 4, Size: 4},
	}}
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"irq_numeric_name", format, content, coreDecodeContext{}, "missing_or_invalid_irq_name"}
}

func invalidSoftIRQVector() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := softIRQCoreCase("softirq_entry", 10)
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"softirq_vec10", test.format, test.content, coreDecodeContext{}, "missing_or_invalid_vec"}
}

func invalidIPIMissingPrintk() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := ipiPointerCoreCase("ipi_entry")
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"ipi_missing_printk", test.format, test.content, coreDecodeContext{}, "missing_or_invalid_reason"}
}

func invalidIPIConflictedPrintk() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := ipiPointerCoreCase("ipi_entry")
	test.ctx.PrintkPoisoned = map[uint64]bool{0x1234: true}
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"ipi_conflicted_printk", test.format, test.content, test.ctx, "missing_or_invalid_reason"}
}

func invalidIPIOversizeMask() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	test := ipiRaisePointerCoreCase()
	test.content = append(test.content, make([]byte, 8)...)
	binary.LittleEndian.PutUint32(test.content[:4], uint32((16<<16)|16))
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"ipi_oversize_mask", test.format, test.content, test.ctx, "missing_or_invalid_target_mask"}
}

func invalidIPIMixedProfile() (out struct {
	name    string
	format  eventFormat
	content []byte
	ctx     coreDecodeContext
	reason  string
}) {
	const address = 0x1234
	content := make([]byte, 16)
	binary.LittleEndian.PutUint64(content[:8], 0x10)
	binary.LittleEndian.PutUint64(content[8:16], address)
	format := eventFormat{Name: "ipi_raise", Fields: []eventField{
		{Type: "unsigned long", Name: "target_cpus", Offset: 0, Size: 8},
		{Type: "const char *", Name: "reason", Offset: 8, Size: 8},
	}}
	return struct {
		name    string
		format  eventFormat
		content []byte
		ctx     coreDecodeContext
		reason  string
	}{"ipi_mixed_profile", format, content, coreDecodeContext{PrintkFormats: map[uint64]string{address: "Rescheduling interrupts"}}, "mixed_ipi_profile"}
}

func coreTestItoa64(value int64) string { return coreTestUtoa64(uint64(value)) }

func coreTestUtoa64(value uint64) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	index := len(buf)
	for value > 0 {
		index--
		buf[index] = digits[value%10]
		value /= 10
	}
	return string(buf[index:])
}

func coreTestThreeDigits(value int64) string {
	text := coreTestItoa64(value)
	for len(text) < 3 {
		text = "0" + text
	}
	return text
}
