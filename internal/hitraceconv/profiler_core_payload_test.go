package hitraceconv

import (
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type profilerCoreTestValue struct {
	wire  int
	uint  uint64
	bytes []byte
}

type profilerCoreTestCase struct {
	field  int
	name   string
	values map[int]profilerCoreTestValue
	want   string
}

func profilerCoreVarint(value uint64) profilerCoreTestValue {
	return profilerCoreTestValue{wire: 0, uint: value}
}

func profilerCoreBytes(value string) profilerCoreTestValue {
	return profilerCoreTestValue{wire: 2, bytes: []byte(value)}
}

func profilerCoreTestCases() []profilerCoreTestCase {
	return []profilerCoreTestCase{
		{
			field: 113, name: "binder_transaction",
			values: map[int]profilerCoreTestValue{
				1: profilerCoreVarint(12_145_787), 2: profilerCoreVarint(12_138_790),
				3: profilerCoreVarint(1_864), 4: profilerCoreVarint(0), 5: profilerCoreVarint(0),
				6: profilerCoreVarint(0x0a), 7: profilerCoreVarint(0x10),
			},
			want: "transaction=12145787 dest_node=12138790 dest_proc=1864 dest_thread=0 reply=0 flags=0x10 code=0xa",
		},
		{
			field: 119, name: "binder_transaction_received",
			values: map[int]profilerCoreTestValue{1: profilerCoreVarint(12_145_787)},
			want:   "transaction=12145787",
		},
		{
			field: 1400, name: "ipi_entry",
			values: map[int]profilerCoreTestValue{1: profilerCoreBytes("Rescheduling interrupts")},
			want:   "(Rescheduling interrupts)",
		},
		{
			field: 1401, name: "ipi_exit",
			values: map[int]profilerCoreTestValue{1: profilerCoreBytes("Rescheduling interrupts")},
			want:   "(Rescheduling interrupts)",
		},
		{
			field: 1402, name: "ipi_raise",
			values: map[int]profilerCoreTestValue{1: profilerCoreVarint(16), 2: profilerCoreBytes("Rescheduling interrupts")},
			want:   "target_mask=16 (Rescheduling interrupts)",
		},
		{
			field: 1500, name: "irq_handler_entry",
			values: map[int]profilerCoreTestValue{1: profilerCoreVarint(17), 2: profilerCoreBytes("arch_timer")},
			want:   "irq=17 name=arch_timer",
		},
		{
			field: 1501, name: "irq_handler_exit",
			values: map[int]profilerCoreTestValue{1: profilerCoreVarint(17), 2: profilerCoreVarint(2)},
			want:   "irq=17 ret=handled",
		},
		{
			field: 1502, name: "softirq_entry",
			values: map[int]profilerCoreTestValue{1: profilerCoreVarint(0)},
			want:   "vec=0 [action=HI]",
		},
		{
			field: 1503, name: "softirq_exit",
			values: map[int]profilerCoreTestValue{1: profilerCoreVarint(5)},
			want:   "vec=5 [action=IRQ_POLL]",
		},
		{
			field: 1504, name: "softirq_raise",
			values: map[int]profilerCoreTestValue{1: profilerCoreVarint(9)},
			want:   "vec=9 [action=RCU]",
		},
		{
			field: 2003, name: "cpu_frequency",
			values: map[int]profilerCoreTestValue{1: profilerCoreVarint(840_000), 2: profilerCoreVarint(0)},
			want:   "state=840000 cpu_id=0",
		},
		{
			field: 2004, name: "cpu_frequency_limits",
			values: map[int]profilerCoreTestValue{1: profilerCoreVarint(418_000), 2: profilerCoreVarint(1_720_000), 3: profilerCoreVarint(0)},
			want:   "min=418000 max=1720000 cpu_id=0",
		},
		{
			field: 2005, name: "cpu_idle",
			values: map[int]profilerCoreTestValue{1: profilerCoreVarint(math.MaxUint32), 2: profilerCoreVarint(2)},
			want:   "state=4294967295 cpu_id=2",
		},
		{
			field: 2420, name: "sched_wakeup",
			values: map[int]profilerCoreTestValue{
				1: profilerCoreBytes("OS_IPC_14_45174"), 2: profilerCoreVarint(45_174),
				3: profilerCoreVarint(41), 4: profilerCoreVarint(0), 5: profilerCoreVarint(1),
			},
			want: "comm=OS_IPC_14_45174 pid=45174 prio=41 target_cpu=001",
		},
		{
			field: 2421, name: "sched_wakeup_new",
			values: map[int]profilerCoreTestValue{
				1: profilerCoreBytes("new-app"), 2: profilerCoreVarint(20),
				3: profilerCoreVarint(140), 4: profilerCoreVarint(0), 5: profilerCoreVarint(2),
			},
			want: "comm=new-app pid=20 prio=140 target_cpu=002",
		},
		{
			field: 2422, name: "sched_waking",
			values: map[int]profilerCoreTestValue{
				1: profilerCoreBytes("hm-app"), 2: profilerCoreVarint(21),
				3: profilerCoreVarint(159), 4: profilerCoreVarint(0), 5: profilerCoreVarint(3),
			},
			want: "comm=hm-app pid=21 prio=159 target_cpu=003",
		},
		{
			field: 4002, name: "sched_blocked_reason",
			values: map[int]profilerCoreTestValue{
				1: profilerCoreVarint(324), 2: profilerCoreVarint(0x1234), 3: profilerCoreVarint(0),
				4: profilerCoreBytes("kthread_worker_fn+0x14c/0x1ec[devhost.elf]"),
			},
			want: "pid=324 iowait=0 caller=kthread_worker_fn+0x14c/0x1ec[devhost.elf]",
		},
	}
}

func TestProfilerCorePayloadMatrixUsesCanonicalRenderer(t *testing.T) {
	for _, test := range profilerCoreTestCases() {
		t.Run(test.name, func(t *testing.T) {
			event := profilerFtraceEventRecord{Field: test.field, Payload: profilerCoreEncodeValues(test.values)}
			payload, admission, reason, degradations := decodeProfilerCorePayload(event)
			if admission != bodyAdmitted || reason != "" || len(degradations) != 0 {
				t.Fatalf("admission=%d reason=%q degradations=%v payload=%+v", admission, reason, degradations, payload)
			}
			if payload.Name != test.name {
				t.Fatalf("typed name=%q want=%q payload=%+v", payload.Name, test.name, payload)
			}
			body, ok := renderCanonicalCorePayload(payload)
			if !ok || body != test.want {
				t.Fatalf("canonical body: ok=%t got=%q want=%q payload=%+v", ok, body, test.want, payload)
			}
			name, rendered, known, gotDegradations := renderProfilerFtraceEventBodyWithAudit(event)
			if !known || name != test.name || rendered != test.want || len(gotDegradations) != 0 {
				t.Fatalf("structured canonical path: known=%t name=%q body=%q degradations=%v", known, name, rendered, gotDegradations)
			}
		})
	}
}

func TestProfilerCoreHardFieldsRejectWrongWireAndDuplicates(t *testing.T) {
	for _, test := range profilerCoreTestCases() {
		schema := profilerStructuredCoreSchemas[test.field]
		fields := make([]int, 0, len(schema))
		for field := range schema {
			if !profilerCoreDisplayField(test.field, field) {
				fields = append(fields, field)
			}
		}
		sort.Ints(fields)
		for _, field := range fields {
			value := test.values[field]
			for _, mutation := range []struct {
				name   string
				extra  []byte
				reason string
			}{
				{name: "wrong_wire", extra: profilerCoreWrongWire(field, value), reason: profilerCoreTestReason(field, "wrong_wire")},
				{name: "same_duplicate", extra: profilerCoreEncodeField(field, value), reason: profilerCoreTestReason(field, "duplicate")},
				{name: "conflicting_duplicate", extra: profilerCoreEncodeField(field, profilerCoreAlternateValue(value)), reason: profilerCoreTestReason(field, "duplicate")},
			} {
				t.Run(test.name+"/field"+coreTestItoa64(int64(field))+"/"+mutation.name, func(t *testing.T) {
					data := append(profilerCoreEncodeValues(test.values), mutation.extra...)
					payload, admission, reason, _ := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: test.field, Payload: data})
					if admission != bodyRejected || reason != mutation.reason || !reflect.DeepEqual(payload, coreRenderPayload{}) {
						t.Fatalf("admission=%d reason=%q want=%q partial=%+v", admission, reason, mutation.reason, payload)
					}
				})
			}
		}
	}
}

func TestProfilerCoreMalformedPayloadRejectsWithoutPartialAuthority(t *testing.T) {
	for _, test := range profilerCoreTestCases() {
		t.Run(test.name, func(t *testing.T) {
			data := append(profilerCoreEncodeValues(test.values), 0x80)
			payload, admission, reason, degradations := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: test.field, Payload: data})
			if admission != bodyRejected || reason != "core_payload_malformed_wire" ||
				!reflect.DeepEqual(payload, coreRenderPayload{}) || len(degradations) != 0 {
				t.Fatalf("malformed payload escaped: admission=%d reason=%q degradations=%v payload=%+v", admission, reason, degradations, payload)
			}
		})
	}
}

func TestProfilerCoreUnknownInnerFieldCannotChangeTypedFact(t *testing.T) {
	for _, test := range profilerCoreTestCases() {
		t.Run(test.name, func(t *testing.T) {
			base, admission, reason, baseDegradations := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: test.field, Payload: profilerCoreEncodeValues(test.values)})
			withUnknown := append(profilerCoreEncodeValues(test.values), protoVarint(99, math.MaxUint64)...)
			got, gotAdmission, gotReason, gotDegradations := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: test.field, Payload: withUnknown})
			if admission != bodyAdmitted || gotAdmission != bodyAdmitted || reason != "" || gotReason != "" ||
				!reflect.DeepEqual(got, base) || !reflect.DeepEqual(gotDegradations, baseDegradations) {
				t.Fatalf("unknown extension changed typed fact: base=%+v/%v got=%+v/%v", base, baseDegradations, got, gotDegradations)
			}
		})
	}
}

func TestProfilerCoreProto3AbsentAndExplicitZeroAreEquivalent(t *testing.T) {
	tests := []struct {
		name     string
		field    int
		omitted  []byte
		explicit []byte
	}{
		{name: "binder transaction defaults", field: 113,
			omitted:  protoPayload(protoVarint(1, 42), protoVarint(3, 100)),
			explicit: protoPayload(protoVarint(1, 42), protoVarint(2, 0), protoVarint(3, 100), protoVarint(4, 0), protoVarint(5, 0), protoVarint(6, 0), protoVarint(7, 0))},
		{name: "binder received", field: 119, omitted: protoVarint(1, 42), explicit: protoVarint(1, 42)},
		{name: "ipi entry", field: 1400, omitted: protoBytes(1, []byte("Timer broadcast interrupts")), explicit: protoBytes(1, []byte("Timer broadcast interrupts"))},
		{name: "ipi exit", field: 1401, omitted: protoBytes(1, []byte("Timer broadcast interrupts")), explicit: protoBytes(1, []byte("Timer broadcast interrupts"))},
		{name: "ipi mask zero", field: 1402,
			omitted:  protoBytes(2, []byte("Timer broadcast interrupts")),
			explicit: protoPayload(protoVarint(1, 0), protoBytes(2, []byte("Timer broadcast interrupts")))},
		{name: "irq number zero", field: 1500, omitted: protoBytes(2, []byte("timer")), explicit: protoPayload(protoVarint(1, 0), protoBytes(2, []byte("timer")))},
		{name: "irq exit zero", field: 1501, omitted: nil, explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0))},
		{name: "softirq entry zero", field: 1502, omitted: nil, explicit: protoVarint(1, 0)},
		{name: "softirq exit zero", field: 1503, omitted: nil, explicit: protoVarint(1, 0)},
		{name: "softirq raise zero", field: 1504, omitted: nil, explicit: protoVarint(1, 0)},
		{name: "frequency zeros", field: 2003, omitted: nil, explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0))},
		{name: "limits zeros", field: 2004, omitted: nil, explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0), protoVarint(3, 0))},
		{name: "idle zeros", field: 2005, omitted: nil, explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0))},
		{name: "wakeup zeros", field: 2420, omitted: nil,
			explicit: protoPayload(protoBytes(1, nil), protoVarint(2, 0), protoVarint(3, 0), protoVarint(4, 0), protoVarint(5, 0))},
		{name: "wakeup new zeros", field: 2421, omitted: nil,
			explicit: protoPayload(protoBytes(1, nil), protoVarint(2, 0), protoVarint(3, 0), protoVarint(4, 0), protoVarint(5, 0))},
		{name: "waking zeros", field: 2422, omitted: nil,
			explicit: protoPayload(protoBytes(1, nil), protoVarint(2, 0), protoVarint(3, 0), protoVarint(4, 0), protoVarint(5, 0))},
		{name: "blocked zeros", field: 4002, omitted: nil,
			explicit: protoPayload(protoVarint(1, 0), protoVarint(2, 0), protoVarint(3, 0), protoBytes(4, nil))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left, leftAdmission, leftReason, leftDegradations := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: test.field, Payload: test.omitted})
			right, rightAdmission, rightReason, rightDegradations := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: test.field, Payload: test.explicit})
			if leftAdmission != bodyAdmitted || rightAdmission != bodyAdmitted || leftReason != "" || rightReason != "" ||
				!reflect.DeepEqual(left, right) || !reflect.DeepEqual(leftDegradations, rightDegradations) {
				t.Fatalf("absent/default mismatch: omitted=%+v admission=%d reason=%q deg=%v explicit=%+v admission=%d reason=%q deg=%v",
					left, leftAdmission, leftReason, leftDegradations, right, rightAdmission, rightReason, rightDegradations)
			}
		})
	}
}

func TestProfilerCoreDisplayMetadataCannotChangeHardTuple(t *testing.T) {
	wakeHard := protoPayload(protoVarint(2, 20), protoVarint(3, 159), protoVarint(4, 0), protoVarint(5, 2))
	for _, test := range []struct {
		name    string
		display []byte
		reason  string
	}{
		{name: "absent", reason: "display_comm_unavailable"},
		{name: "explicit empty", display: protoBytes(1, nil), reason: "display_comm_unavailable"},
		{name: "wrong wire", display: protoVarint(1, 1), reason: "display_comm_wrong_wire"},
		{name: "same duplicate", display: protoPayload(protoBytes(1, []byte("worker")), protoBytes(1, []byte("worker"))), reason: "display_comm_duplicate"},
		{name: "conflicting duplicate", display: protoPayload(protoBytes(1, []byte("a")), protoBytes(1, []byte("b"))), reason: "display_comm_duplicate"},
		{name: "line injection", display: protoBytes(1, []byte("a\nb")), reason: "display_comm_invalid"},
		{name: "token injection", display: protoBytes(1, []byte("worker|pid=999")), reason: "display_comm_unavailable"},
		{name: "producer length overflow", display: protoBytes(1, []byte("1234567890123456")), reason: "display_comm_out_of_profile"},
	} {
		t.Run("wake/"+test.name, func(t *testing.T) {
			data := append(append([]byte(nil), test.display...), wakeHard...)
			payload, admission, reason, degradations := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: 2420, Payload: data})
			if admission != bodyAdmitted || reason != "" || payload.Wakeup == nil ||
				payload.Wakeup.Comm != "<...>" || payload.Wakeup.PID != 20 || payload.Wakeup.Priority != 159 || payload.Wakeup.TargetCPU != 2 ||
				len(degradations) != 1 || degradations[0] != test.reason {
				t.Fatalf("display-only wake metadata changed hard tuple: admission=%d reason=%q degradations=%v payload=%+v", admission, reason, degradations, payload)
			}
			body, _ := renderCanonicalCorePayload(payload)
			if body != "comm=<...> pid=20 prio=159 target_cpu=002" || strings.Contains(body, "999") {
				t.Fatalf("unsafe wake display escaped: %q", body)
			}
		})
	}

	blockedHard := protoPayload(protoVarint(1, 324), protoVarint(2, 0x1234), protoVarint(3, 1))
	for _, test := range []struct {
		name    string
		display []byte
		reason  string
	}{
		{name: "absent"},
		{name: "explicit empty", display: protoBytes(4, nil)},
		{name: "wrong wire", display: protoVarint(4, 1), reason: "display_caller_str_wrong_wire"},
		{name: "duplicate", display: protoPayload(protoBytes(4, []byte("worker_fn")), protoBytes(4, []byte("worker_fn"))), reason: "display_caller_str_duplicate"},
		{name: "line injection", display: protoBytes(4, []byte("a\nb")), reason: "display_caller_str_invalid"},
		{name: "token injection", display: protoBytes(4, []byte("worker|pid=999")), reason: "display_caller_str_invalid"},
		{name: "safe forged token", display: protoBytes(4, []byte("forged")), reason: "display_caller_str_invalid"},
		{name: "noncanonical hex", display: protoBytes(4, []byte("worker_fn+0x01/0x2[kernel]")), reason: "display_caller_str_invalid"},
	} {
		t.Run("blocked/"+test.name, func(t *testing.T) {
			data := append(append([]byte(nil), test.display...), blockedHard...)
			payload, admission, reason, degradations := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: 4002, Payload: data})
			if admission != bodyAdmitted || reason != "" || payload.Blocked == nil || payload.Blocked.CallerSymbolized ||
				payload.Blocked.PID != 324 || payload.Blocked.CallerRaw != 0x1234 || payload.Blocked.IOWait != 1 {
				t.Fatalf("display-only caller changed hard tuple: admission=%d reason=%q degradations=%v payload=%+v", admission, reason, degradations, payload)
			}
			if test.reason == "" && len(degradations) != 0 || test.reason != "" && (len(degradations) != 1 || degradations[0] != test.reason) {
				t.Fatalf("blocked degradation=%v want=%q", degradations, test.reason)
			}
			body, _ := renderCanonicalCorePayload(payload)
			if body != "pid=324 iowait=1 caller=unknown caller_raw=0x1234 caller_quality=opaque" || strings.Contains(body, "999") {
				t.Fatalf("unsafe blocked display escaped: %q", body)
			}
		})
	}
}

func TestProfilerCoreCanonicalLineCapRejectsOnlyTheGovernedRow(t *testing.T) {
	event := profilerFtraceEventRecord{
		Field:   1400,
		Payload: protoBytes(1, []byte(strings.Repeat("r", maxTraceDBSystraceLineBytes))),
	}
	name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(event)
	if known || name != "" || body != "" || len(degradations) != 1 || degradations[0] != "invalid_canonical_core_line" {
		t.Fatalf("oversized hard body escaped local rejection: known=%t name=%q body_len=%d degradations=%v", known, name, len(body), degradations)
	}
}

func TestProfilerCoreStrictSourceDomains(t *testing.T) {
	base := map[int]profilerCoreTestCase{}
	for _, test := range profilerCoreTestCases() {
		base[test.field] = test
	}
	invalid := []struct {
		name   string
		field  int
		change map[int]profilerCoreTestValue
		reason string
	}{
		{name: "binder transaction zero", field: 113, change: map[int]profilerCoreTestValue{1: profilerCoreVarint(0)}, reason: "invalid_transaction_id"},
		{name: "binder destination process zero", field: 113, change: map[int]profilerCoreTestValue{3: profilerCoreVarint(0)}, reason: "invalid_transaction_endpoint"},
		{name: "binder negative endpoint", field: 113, change: map[int]profilerCoreTestValue{2: profilerCoreVarint(math.MaxUint64)}, reason: "invalid_transaction_endpoint"},
		{name: "binder invalid reply", field: 113, change: map[int]profilerCoreTestValue{5: profilerCoreVarint(2)}, reason: "invalid_reply"},
		{name: "binder uint32 overflow", field: 113, change: map[int]profilerCoreTestValue{6: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, reason: "core_field6_out_of_range"},
		{name: "ipi reason injection", field: 1400, change: map[int]profilerCoreTestValue{1: profilerCoreBytes("bad(reason)")}, reason: "missing_or_invalid_reason"},
		{name: "ipi reason missing", field: 1400, change: map[int]profilerCoreTestValue{1: profilerCoreBytes("")}, reason: "missing_or_invalid_reason"},
		{name: "irq negative", field: 1500, change: map[int]profilerCoreTestValue{1: profilerCoreVarint(math.MaxUint64)}, reason: "missing_or_invalid_irq"},
		{name: "irq name injection", field: 1500, change: map[int]profilerCoreTestValue{2: profilerCoreBytes("timer name")}, reason: "missing_or_invalid_irq_name"},
		{name: "irq name missing", field: 1500, change: map[int]profilerCoreTestValue{2: profilerCoreBytes("")}, reason: "missing_or_invalid_irq_name"},
		{name: "softirq overflow", field: 1504, change: map[int]profilerCoreTestValue{1: profilerCoreVarint(10)}, reason: "missing_or_invalid_vec"},
		{name: "frequency source width", field: 2003, change: map[int]profilerCoreTestValue{1: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, reason: "missing_or_invalid_state"},
		{name: "frequency cpu range", field: 2003, change: map[int]profilerCoreTestValue{2: profilerCoreVarint(uint64(maxTraceDBCPUIndex + 1))}, reason: "missing_or_invalid_cpu_id"},
		{name: "limits order", field: 2004, change: map[int]profilerCoreTestValue{1: profilerCoreVarint(2_000_000), 2: profilerCoreVarint(1_000_000)}, reason: "invalid_limits_order"},
		{name: "wake negative pid", field: 2420, change: map[int]profilerCoreTestValue{2: profilerCoreVarint(math.MaxUint64)}, reason: "missing_or_invalid_pid"},
		{name: "wake priority source width", field: 2420, change: map[int]profilerCoreTestValue{3: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, reason: "missing_or_invalid_priority"},
		{name: "wake success source width", field: 2420, change: map[int]profilerCoreTestValue{4: profilerCoreVarint(uint64(math.MaxUint32) + 1)}, reason: "core_field4_out_of_range"},
		{name: "wake target cpu range", field: 2420, change: map[int]profilerCoreTestValue{5: profilerCoreVarint(uint64(maxTraceDBCPUIndex + 1))}, reason: "missing_or_invalid_target_cpu"},
		{name: "blocked iowait enum", field: 4002, change: map[int]profilerCoreTestValue{3: profilerCoreVarint(2)}, reason: "missing_or_invalid_iowait"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			values := profilerCoreCloneValues(base[test.field].values)
			for field, value := range test.change {
				values[field] = value
			}
			_, admission, reason, _ := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: test.field, Payload: profilerCoreEncodeValues(values)})
			if admission != bodyRejected || reason != test.reason {
				t.Fatalf("admission=%d reason=%q want=%q", admission, reason, test.reason)
			}
		})
	}

	for _, priority := range []uint64{140, 159, 301} {
		values := profilerCoreCloneValues(base[2420].values)
		values[3] = profilerCoreVarint(priority)
		payload, admission, reason, _ := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: 2420, Payload: profilerCoreEncodeValues(values)})
		if admission != bodyAdmitted || reason != "" || payload.Wakeup == nil || payload.Wakeup.Priority != int64(priority) {
			t.Fatalf("Harmony RT priority %d was rejected: admission=%d reason=%q payload=%+v", priority, admission, reason, payload)
		}
	}
	for name, encoded := range map[string]uint64{
		"low32":         math.MaxUint32,
		"sign_extended": math.MaxUint64,
	} {
		t.Run("negative priority "+name, func(t *testing.T) {
			values := profilerCoreCloneValues(base[2420].values)
			values[3] = profilerCoreVarint(encoded)
			payload, admission, reason, _ := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: 2420, Payload: profilerCoreEncodeValues(values)})
			if admission != bodyAdmitted || reason != "" || payload.Wakeup == nil || payload.Wakeup.Priority != -1 {
				t.Fatalf("negative int32 priority profile %s was rejected: admission=%d reason=%q payload=%+v", name, admission, reason, payload)
			}
			body, _ := renderCanonicalCorePayload(payload)
			if !strings.Contains(body, " prio=-1 ") {
				t.Fatalf("negative priority was not preserved in canonical output: %q", body)
			}
		})
	}

	t.Run("uint and CPU maxima", func(t *testing.T) {
		values := profilerCoreCloneValues(base[2003].values)
		values[1] = profilerCoreVarint(math.MaxUint32)
		values[2] = profilerCoreVarint(uint64(maxTraceDBCPUIndex))
		payload, admission, reason, _ := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: 2003, Payload: profilerCoreEncodeValues(values)})
		if admission != bodyAdmitted || reason != "" || payload.CPU == nil ||
			payload.CPU.State != math.MaxUint32 || payload.CPU.CPUID != uint64(maxTraceDBCPUIndex) {
			t.Fatalf("uint32/CPU maxima were rejected: admission=%d reason=%q payload=%+v", admission, reason, payload)
		}
	})

	t.Run("IPI uint64 mask maximum", func(t *testing.T) {
		values := profilerCoreCloneValues(base[1402].values)
		values[1] = profilerCoreVarint(math.MaxUint64)
		payload, admission, reason, _ := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: 1402, Payload: profilerCoreEncodeValues(values)})
		if admission != bodyAdmitted || reason != "" || payload.Interrupt == nil || payload.Interrupt.TargetMask != math.MaxUint64 {
			t.Fatalf("uint64 IPI mask maximum was rejected: admission=%d reason=%q payload=%+v", admission, reason, payload)
		}
	})

	t.Run("wakeup success nonzero is audited but ignored", func(t *testing.T) {
		zeroValues := profilerCoreCloneValues(base[2420].values)
		nonzeroValues := profilerCoreCloneValues(base[2420].values)
		zeroValues[4] = profilerCoreVarint(0)
		nonzeroValues[4] = profilerCoreVarint(1)
		zero, zeroAdmission, zeroReason, zeroDegradations := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: 2420, Payload: profilerCoreEncodeValues(zeroValues)})
		nonzero, nonzeroAdmission, nonzeroReason, nonzeroDegradations := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: 2420, Payload: profilerCoreEncodeValues(nonzeroValues)})
		if zeroAdmission != bodyAdmitted || nonzeroAdmission != bodyAdmitted || zeroReason != "" || nonzeroReason != "" ||
			!reflect.DeepEqual(zero, nonzero) || !reflect.DeepEqual(zeroDegradations, nonzeroDegradations) {
			t.Fatalf("success changed wake fact: zero=%+v/%d/%q/%v nonzero=%+v/%d/%q/%v",
				zero, zeroAdmission, zeroReason, zeroDegradations, nonzero, nonzeroAdmission, nonzeroReason, nonzeroDegradations)
		}
		zeroBody, _ := renderCanonicalCorePayload(zero)
		nonzeroBody, _ := renderCanonicalCorePayload(nonzero)
		if zeroBody != nonzeroBody {
			t.Fatalf("ignored success changed canonical output: zero=%q nonzero=%q", zeroBody, nonzeroBody)
		}
	})

	t.Run("blocked caller uint64 maximum", func(t *testing.T) {
		values := profilerCoreCloneValues(base[4002].values)
		values[2] = profilerCoreVarint(math.MaxUint64)
		values[4] = profilerCoreBytes("")
		payload, admission, reason, _ := decodeProfilerCorePayload(profilerFtraceEventRecord{Field: 4002, Payload: profilerCoreEncodeValues(values)})
		if admission != bodyAdmitted || reason != "" || payload.Blocked == nil || payload.Blocked.CallerRaw != math.MaxUint64 {
			t.Fatalf("uint64 caller maximum was rejected: admission=%d reason=%q payload=%+v", admission, reason, payload)
		}
		body, _ := renderCanonicalCorePayload(payload)
		if !strings.Contains(body, "caller_raw=0xffffffffffffffff") {
			t.Fatalf("uint64 caller maximum was truncated: %q", body)
		}
	})
}

func TestProfilerCoreRejectedRowKeepsLocalSiblings(t *testing.T) {
	badCPU := append(protoPayload(protoVarint(1, 840_000), protoVarint(2, 0)), 0x80)
	goodCPU := protoPayload(protoVarint(1, 1_550_000), protoVarint(2, 2))
	goodIPI := protoPayload(protoVarint(1, 16), protoBytes(2, []byte("Rescheduling interrupts")))
	detail := protoPayload(
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(1_000, 100, 100, "bad", 2003, badCPU),
		syntheticTracePluginFtraceEvent(2_000, 100, 100, "cpu", 2003, goodCPU),
		syntheticTracePluginFtraceEvent(3_000, 100, 100, "ipi", 1402, goodIPI),
	)
	structured := protoMessage(2, detail)
	sink, err := newTraceDBRowSink("", 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredRows(structured, &seq, sink)
	if err != nil || rows != 2 || seq != 2 || len(sink.rows) != 2 {
		t.Fatalf("local core rejection poisoned siblings: rows=%d seq=%d sink=%+v coverage=%+v err=%v", rows, seq, sink.rows, coverage, err)
	}
	joined := sink.rows[0].line + "\n" + sink.rows[1].line
	if !strings.Contains(joined, "cpu_frequency: state=1550000 cpu_id=2") ||
		!strings.Contains(joined, "ipi_raise: target_mask=16 (Rescheduling interrupts)") || strings.Contains(joined, "840000") {
		t.Fatalf("valid siblings missing or rejected row escaped:\n%s", joined)
	}
	cpuCoverage := coverageForTable(coverage, "cpu_frequency")
	if cpuCoverage == nil || cpuCoverage.RowsRead != 2 || cpuCoverage.RowsEmitted != 1 ||
		!strings.Contains(cpuCoverage.Skipped, "core_payload_malformed_wire=1") {
		t.Fatalf("CPU local-rejection coverage mismatch: %+v", cpuCoverage)
	}
	ipiCoverage := coverageForTable(coverage, "ipi_raise")
	if ipiCoverage == nil || ipiCoverage.RowsRead != 1 || ipiCoverage.RowsEmitted != 1 {
		t.Fatalf("IPI sibling coverage mismatch: %+v", ipiCoverage)
	}
}

func TestProfilerStructuredCoreSchemaAndDescriptorsAreClosed(t *testing.T) {
	wantCore := map[int]map[int]int{
		113: {1: 0, 2: 0, 3: 0, 4: 0, 5: 0, 6: 0, 7: 0},
		119: {1: 0}, 1400: {1: 2}, 1401: {1: 2}, 1402: {1: 0, 2: 2},
		1500: {1: 0, 2: 2}, 1501: {1: 0, 2: 0}, 1502: {1: 0}, 1503: {1: 0}, 1504: {1: 0},
		2003: {1: 0, 2: 0}, 2004: {1: 0, 2: 0, 3: 0}, 2005: {1: 0, 2: 0},
		2420: {1: 2, 2: 0, 3: 0, 4: 0, 5: 0},
		2421: {1: 2, 2: 0, 3: 0, 4: 0, 5: 0},
		2422: {1: 2, 2: 0, 3: 0, 4: 0, 5: 0},
		4002: {1: 0, 2: 0, 3: 0, 4: 2},
	}
	if !reflect.DeepEqual(profilerStructuredCoreSchemas, wantCore) {
		t.Fatalf("structured core schema drifted: got=%v want=%v", profilerStructuredCoreSchemas, wantCore)
	}

	wantDescriptors := map[int]profilerFtraceEventDescriptor{
		113: {113, "binder", "binder_transaction"}, 119: {119, "binder", "binder_transaction_received"},
		202: {202, "block", "block_bio_complete"}, 204: {204, "block", "block_bio_queue"},
		205: {205, "block", "block_bio_remap"}, 209: {209, "block", "block_rq_complete"},
		210: {210, "block", "block_rq_insert"}, 211: {211, "block", "block_rq_issue"}, 212: {212, "block", "block_rq_remap"},
		410: {410, "clock", "clock_set_rate"}, 1000: {1000, "filemap", "mm_filemap_add_to_page_cache"},
		1001: {1001, "filemap", "mm_filemap_delete_from_page_cache"}, 1109: {1109, "trace_marker", "print"},
		1400: {1400, "ipi", "ipi_entry"}, 1401: {1401, "ipi", "ipi_exit"}, 1402: {1402, "ipi", "ipi_raise"},
		1500: {1500, "irq", "irq_handler_entry"}, 1501: {1501, "irq", "irq_handler_exit"},
		1502: {1502, "irq", "softirq_entry"}, 1503: {1503, "irq", "softirq_exit"}, 1504: {1504, "irq", "softirq_raise"},
		2002: {2002, "clock", "clock_set_rate"}, 2003: {2003, "cpu", "cpu_frequency"},
		2004: {2004, "cpu", "cpu_frequency_limits"}, 2005: {2005, "cpu", "cpu_idle"},
		2417: {2417, "sched", "sched_switch"}, 2420: {2420, "sched", "sched_wakeup"},
		2421: {2421, "sched", "sched_wakeup_new"}, 2422: {2422, "sched", "sched_waking"},
		4002: {4002, "sched", "sched_blocked_reason"}, 4009: {4009, "f2fs", "f2fs_sync_file_enter"},
		4010: {4010, "f2fs", "f2fs_sync_file_exit"}, 4011: {4011, "f2fs", "f2fs_write_begin"},
		4012: {4012, "f2fs", "f2fs_write_end"}, 4015: {4015, "mmc", "mmc_request_done"},
		4016: {4016, "mmc", "mmc_request_start"},
	}
	if !reflect.DeepEqual(profilerFtraceEventDescriptors, wantDescriptors) {
		t.Fatalf("structured descriptor registry drifted: got=%v want=%v", profilerFtraceEventDescriptors, wantDescriptors)
	}
	for field := range wantCore {
		descriptor := wantDescriptors[field]
		if descriptor.Field != field {
			t.Fatalf("descriptor key/field mismatch: key=%d descriptor=%+v", field, descriptor)
		}
		if _, governed := coreRenderKindForName(descriptor.Name); !governed {
			t.Fatalf("structured core descriptor is not governed by canonical registry: field=%d descriptor=%+v", field, descriptor)
		}
	}
}

func profilerCoreEncodeValues(values map[int]profilerCoreTestValue) []byte {
	fields := make([]int, 0, len(values))
	for field := range values {
		fields = append(fields, field)
	}
	sort.Ints(fields)
	var out []byte
	for _, field := range fields {
		out = append(out, profilerCoreEncodeField(field, values[field])...)
	}
	return out
}

func profilerCoreEncodeField(field int, value profilerCoreTestValue) []byte {
	if value.wire == 2 {
		return protoBytes(field, value.bytes)
	}
	return protoVarint(field, value.uint)
}

func profilerCoreWrongWire(field int, value profilerCoreTestValue) []byte {
	if value.wire == 2 {
		return protoVarint(field, 1)
	}
	return protoBytes(field, []byte{1})
}

func profilerCoreAlternateValue(value profilerCoreTestValue) profilerCoreTestValue {
	if value.wire == 2 {
		return profilerCoreTestValue{wire: 2, bytes: append(append([]byte(nil), value.bytes...), []byte("-other")...)}
	}
	return profilerCoreTestValue{wire: 0, uint: value.uint + 1}
}

func profilerCoreCloneValues(values map[int]profilerCoreTestValue) map[int]profilerCoreTestValue {
	out := make(map[int]profilerCoreTestValue, len(values))
	for field, value := range values {
		value.bytes = append([]byte(nil), value.bytes...)
		out[field] = value
	}
	return out
}

func profilerCoreTestReason(field int, suffix string) string {
	return "core_field" + coreTestItoa64(int64(field)) + "_" + suffix
}
