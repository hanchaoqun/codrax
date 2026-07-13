package hitraceconv

import (
	"fmt"
	"testing"
)

func TestProfilerFtraceEventIssueLegacyBridgeLiteralGolden(t *testing.T) {
	tests := []struct {
		name       string
		eventField int
		source     profilerFtraceEventDegradationKind
		token      string
		wantLabel  string
		wantSource profilerFtraceEventDegradationKind
		wantSev    profilerFtraceEventIssueSeverity
		wantField  uint8
	}{
		{
			name: "event envelope hard reject", eventField: 0,
			source: profilerFtraceEventDegradationEnvelope, token: "envelope_event_malformed_wire",
			wantLabel: "envelope_event_malformed_wire", wantSource: profilerFtraceEventDegradationEnvelope,
			wantSev: profilerFtraceEventIssueHardReject, wantField: 0,
		},
		{
			name: "trace plugin envelope hard reject", eventField: profilerFtraceCPUDetailEnvelopeField,
			source: profilerFtraceEventDegradationEnvelope, token: "envelope_trace_plugin_malformed_wire",
			wantLabel: "envelope_trace_plugin_malformed_wire", wantSource: profilerFtraceEventDegradationEnvelope,
			wantSev: profilerFtraceEventIssueHardReject, wantField: 0,
		},
		{
			name: "nested common fields hard reject", eventField: 2003,
			source: profilerFtraceEventDegradationEnvelope, token: "envelope_common_fields_wrong_wire",
			wantLabel: "envelope_common_fields_wrong_wire", wantSource: profilerFtraceEventDegradationEnvelope,
			wantSev: profilerFtraceEventIssueHardReject, wantField: 50,
		},
		{
			name: "cross-field identity hard reject", eventField: 2003,
			source: profilerFtraceEventDegradationEnvelope, token: "envelope_identity_incomplete",
			wantLabel: "envelope_identity_incomplete", wantSource: profilerFtraceEventDegradationEnvelope,
			wantSev: profilerFtraceEventIssueHardReject, wantField: 0,
		},
		{
			name: "core scalar hard reject", eventField: 2420,
			source: profilerFtraceEventDegradationCorePayload, token: "core_field2_wrong_wire",
			wantLabel: "core_field2_wrong_wire", wantSource: profilerFtraceEventDegradationCorePayload,
			wantSev: profilerFtraceEventIssueHardReject, wantField: 2,
		},
		{
			name: "core display admitted", eventField: 2420,
			source: profilerFtraceEventDegradationCorePayload, token: "display_comm_wrong_wire",
			wantLabel: "display_comm_wrong_wire", wantSource: profilerFtraceEventDegradationCoreDisplay,
			wantSev: profilerFtraceEventIssueAdmittedDisplay, wantField: 1,
		},
		{
			name: "blocked caller display admitted", eventField: 4002,
			source: profilerFtraceEventDegradationCorePayload, token: "display_caller_str_invalid",
			wantLabel: "display_caller_str_invalid", wantSource: profilerFtraceEventDegradationCoreDisplay,
			wantSev: profilerFtraceEventIssueAdmittedDisplay, wantField: 4,
		},
		{
			name: "mmc display admitted", eventField: 4015,
			source: profilerFtraceEventDegradationAuxPayload, token: "drop_response_field7_out_of_source_profile",
			wantLabel: "drop_response_field7_out_of_source_profile", wantSource: profilerFtraceEventDegradationAuxDisplay,
			wantSev: profilerFtraceEventIssueAdmittedDisplay, wantField: 7,
		},
		{
			name: "block display admitted", eventField: 211,
			source: profilerFtraceEventDegradationBlockPayload, token: "cmd_unsafe_omitted",
			wantLabel: "cmd_unsafe_omitted", wantSource: profilerFtraceEventDegradationBlockDisplay,
			wantSev: profilerFtraceEventIssueAdmittedDisplay, wantField: 7,
		},
		{
			name: "generic cpu field audit admitted", eventField: 2002,
			source: profilerFtraceEventDegradationWireAudit, token: "cpu_id_wrong_wire",
			wantLabel: "cpu_id_wrong_wire", wantSource: profilerFtraceEventDegradationFieldAudit,
			wantSev: profilerFtraceEventIssueAdmittedDisplay, wantField: 3,
		},
		{
			name: "generic next-info field audit admitted", eventField: 2417,
			source: profilerFtraceEventDegradationWireAudit, token: "next_info_duplicate",
			wantLabel: "next_info_duplicate", wantSource: profilerFtraceEventDegradationFieldAudit,
			wantSev: profilerFtraceEventIssueAdmittedDisplay, wantField: 8,
		},
		{
			name: "generic clock scalar hard reject", eventField: 410,
			source: profilerFtraceEventDegradationWireAudit, token: "core_field1_wrong_wire",
			wantLabel: "core_field1_wrong_wire", wantSource: profilerFtraceEventDegradationWireAudit,
			wantSev: profilerFtraceEventIssueHardReject, wantField: 1,
		},
		{
			name: "filemap field hard reject", eventField: 1000,
			source: profilerFtraceEventDegradationFilemapPayload, token: "filemap_device_invalid",
			wantLabel: "filemap_device_invalid", wantSource: profilerFtraceEventDegradationFilemapPayload,
			wantSev: profilerFtraceEventIssueHardReject, wantField: 4,
		},
		{
			name: "unknown event hard reject", eventField: 987654,
			source: profilerFtraceEventDegradationUnmappedField, token: "unmapped structured ftrace event field",
			wantLabel: "unmapped structured ftrace event field", wantSource: profilerFtraceEventDegradationUnmappedField,
			wantSev: profilerFtraceEventIssueHardReject, wantField: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issue, ok := profilerFtraceEventIssueFromLegacy(test.eventField, test.source, test.token)
			if !ok {
				t.Fatalf("exact legacy token was rejected: field=%d source=%v token=%q", test.eventField, test.source, test.token)
			}
			if !issue.validFor(test.eventField) {
				t.Fatalf("bridge returned an issue invalid for its event: field=%d issue=%+v", test.eventField, issue)
			}
			if issue.Severity != test.wantSev {
				t.Fatalf("severity=%v want=%v issue=%+v", issue.Severity, test.wantSev, issue)
			}
			if issue.PayloadField != test.wantField {
				t.Fatalf("payload field=%d want=%d issue=%+v", issue.PayloadField, test.wantField, issue)
			}
			if got := issue.sourceClass(); got != test.wantSource {
				t.Fatalf("sourceClass=%v want=%v issue=%+v", got, test.wantSource, issue)
			}
			if got, labelOK := issue.label(test.eventField); !labelOK || got != test.wantLabel {
				t.Fatalf("label=(%q,%t) want=(%q,true) issue=%+v", got, labelOK, test.wantLabel, issue)
			}
		})
	}
}

func TestProfilerFtraceEventIssueLegacyBridgeRejectsNonExactTokens(t *testing.T) {
	tests := []struct {
		name       string
		eventField int
		source     profilerFtraceEventDegradationKind
		token      string
	}{
		{name: "empty", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: ""},
		{name: "leading space", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: " core_field2_wrong_wire"},
		{name: "trailing space", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "core_field2_wrong_wire "},
		{name: "embedded newline", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "core_field2_wrong_wire\n"},
		{name: "future token", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "future_core_reason"},
		{name: "extra suffix", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "core_field2_wrong_wire_extra"},
		{name: "leading zero field", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "core_field02_wrong_wire"},
		{name: "signed field", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "core_field+2_wrong_wire"},
		{name: "zero field", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "core_field0_wrong_wire"},
		{name: "uint8 overflow field", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "core_field256_wrong_wire"},
		{name: "schema foreign field", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "core_field9_wrong_wire"},
		{name: "wrong case", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "Core_field2_wrong_wire"},
		{name: "internal core descriptor", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "missing_core_descriptor"},
		{name: "internal core canonical payload", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "invalid_canonical_core_payload"},
		{name: "internal aux canonical payload", eventField: 4015, source: profilerFtraceEventDegradationAuxPayload, token: "invalid_canonical_aux_payload"},
		{name: "internal filemap canonical payload", eventField: 1000, source: profilerFtraceEventDegradationFilemapPayload, token: "invalid_canonical_filemap_payload"},
		{name: "internal missing typed issue", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "structured_renderer_missing_typed_reason"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if issue, ok := profilerFtraceEventIssueFromLegacy(test.eventField, test.source, test.token); ok {
				t.Fatalf("non-exact token admitted: field=%d source=%v token=%q issue=%+v", test.eventField, test.source, test.token, issue)
			}
		})
	}
}

func TestProfilerFtraceEventIssueLegacyBridgeKeepsEventAndSourceAuthority(t *testing.T) {
	tests := []struct {
		name       string
		eventField int
		source     profilerFtraceEventDegradationKind
		token      string
	}{
		// Field 410 and field 2002 have the same display name, but only 2002
		// owns the nested CPU-id wire dimension.
		{name: "field 410 cannot mint cpu id", eventField: 410, source: profilerFtraceEventDegradationWireAudit, token: "cpu_id_wrong_wire"},
		{name: "cpu token cannot claim core source", eventField: 2002, source: profilerFtraceEventDegradationCorePayload, token: "cpu_id_wrong_wire"},
		{name: "core token cannot claim wire source", eventField: 2420, source: profilerFtraceEventDegradationWireAudit, token: "core_field2_wrong_wire"},
		{name: "display token cannot bypass coarse core source", eventField: 2420, source: profilerFtraceEventDegradationCoreDisplay, token: "display_comm_wrong_wire"},
		{name: "mmc display token cannot bypass coarse aux source", eventField: 4015, source: profilerFtraceEventDegradationAuxDisplay, token: "drop_response_field3_out_of_source_profile"},
		{name: "block display token cannot bypass coarse block source", eventField: 211, source: profilerFtraceEventDegradationBlockDisplay, token: "comm_wrong_wire"},
		{name: "known event cannot mint unmapped issue", eventField: 2420, source: profilerFtraceEventDegradationUnmappedField, token: "unmapped structured ftrace event field"},
		{name: "reserved event envelope is not unknown", eventField: 0, source: profilerFtraceEventDegradationUnmappedField, token: "unmapped structured ftrace event field"},
		{name: "reserved cpu envelope is not unknown", eventField: profilerFtraceCPUDetailEnvelopeField, source: profilerFtraceEventDegradationUnmappedField, token: "unmapped structured ftrace event field"},
		{name: "unreserved negative field is not unknown", eventField: -3, source: profilerFtraceEventDegradationUnmappedField, token: "unmapped structured ftrace event field"},
		{name: "envelope field 1 is not unknown", eventField: 1, source: profilerFtraceEventDegradationUnmappedField, token: "unmapped structured ftrace event field"},
		{name: "reserved pre-oneof field 99 is not unknown", eventField: 99, source: profilerFtraceEventDegradationUnmappedField, token: "unmapped structured ftrace event field"},
		{name: "envelope issue cannot bind field 99", eventField: 99, source: profilerFtraceEventDegradationEnvelope, token: "envelope_timestamp_wrong_wire"},
		{name: "protobuf max plus one is not unknown", eventField: profilerFtraceUnknownEventAggregateField + 1, source: profilerFtraceEventDegradationUnmappedField, token: "unmapped structured ftrace event field"},
		{name: "envelope cannot bind protobuf max plus one", eventField: profilerFtraceUnknownEventAggregateField + 1, source: profilerFtraceEventDegradationEnvelope, token: "envelope_timestamp_wrong_wire"},
		{name: "oneof missing is reserved envelope only", eventField: 2003, source: profilerFtraceEventDegradationEnvelope, token: "envelope_oneof_missing"},
		{name: "oneof multiple is reserved envelope only", eventField: 2003, source: profilerFtraceEventDegradationEnvelope, token: "envelope_oneof_multiple"},
		{name: "oneof wrong wire is reserved envelope only", eventField: 2003, source: profilerFtraceEventDegradationEnvelope, token: "envelope_oneof_wrong_wire"},
		{name: "unknown event cannot mint known issue", eventField: 987654, source: profilerFtraceEventDegradationCorePayload, token: "core_field2_wrong_wire"},
		{name: "unknown event requires exact unmapped token", eventField: 987654, source: profilerFtraceEventDegradationUnmappedField, token: "unmapped_structured_ftrace_event_field"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if issue, ok := profilerFtraceEventIssueFromLegacy(test.eventField, test.source, test.token); ok {
				t.Fatalf("invalid event/source authority admitted: field=%d source=%v token=%q issue=%+v", test.eventField, test.source, test.token, issue)
			}
		})
	}
}

func TestProfilerFtraceEventIssueDisplayFieldWhitelists(t *testing.T) {
	tests := []struct {
		name       string
		eventField int
		source     profilerFtraceEventDegradationKind
		token      string
		want       bool
	}{
		{name: "mmc done response 3", eventField: 4015, source: profilerFtraceEventDegradationAuxPayload, token: "drop_response_field3_out_of_source_profile", want: true},
		{name: "mmc done response 7", eventField: 4015, source: profilerFtraceEventDegradationAuxPayload, token: "drop_response_field7_out_of_source_profile", want: true},
		{name: "mmc done response 11", eventField: 4015, source: profilerFtraceEventDegradationAuxPayload, token: "drop_response_field11_out_of_source_profile", want: true},
		{name: "mmc unlisted response 1", eventField: 4015, source: profilerFtraceEventDegradationAuxPayload, token: "drop_response_field1_out_of_source_profile", want: false},
		{name: "mmc start has no response drop", eventField: 4016, source: profilerFtraceEventDegradationAuxPayload, token: "drop_response_field3_out_of_source_profile", want: false},
		{name: "block bio queue comm", eventField: 204, source: profilerFtraceEventDegradationBlockPayload, token: "comm_wrong_wire", want: true},
		{name: "block rq complete cmd", eventField: 209, source: profilerFtraceEventDegradationBlockPayload, token: "cmd_duplicate", want: true},
		{name: "block rq insert comm", eventField: 210, source: profilerFtraceEventDegradationBlockPayload, token: "comm_unsafe_omitted", want: true},
		{name: "block rq insert cmd", eventField: 210, source: profilerFtraceEventDegradationBlockPayload, token: "cmd_wrong_wire", want: true},
		{name: "block rq issue comm", eventField: 211, source: profilerFtraceEventDegradationBlockPayload, token: "comm_duplicate", want: true},
		{name: "block rq issue cmd", eventField: 211, source: profilerFtraceEventDegradationBlockPayload, token: "cmd_unsafe_omitted", want: true},
		{name: "block bio complete has no comm", eventField: 202, source: profilerFtraceEventDegradationBlockPayload, token: "comm_wrong_wire", want: false},
		{name: "block bio queue has no cmd", eventField: 204, source: profilerFtraceEventDegradationBlockPayload, token: "cmd_wrong_wire", want: false},
		{name: "block rq complete has no comm", eventField: 209, source: profilerFtraceEventDegradationBlockPayload, token: "comm_wrong_wire", want: false},
		{name: "non block event cannot mint block display", eventField: 4015, source: profilerFtraceEventDegradationBlockPayload, token: "cmd_wrong_wire", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issue, ok := profilerFtraceEventIssueFromLegacy(test.eventField, test.source, test.token)
			if ok != test.want {
				t.Fatalf("admitted=%t want=%t field=%d source=%v token=%q issue=%+v", ok, test.want, test.eventField, test.source, test.token, issue)
			}
			if !ok {
				return
			}
			if issue.Severity != profilerFtraceEventIssueAdmittedDisplay {
				t.Fatalf("display whitelist minted severity=%v issue=%+v", issue.Severity, issue)
			}
			label, labelOK := issue.label(test.eventField)
			if !labelOK || label != test.token {
				t.Fatalf("display label=(%q,%t) want=(%q,true) issue=%+v", label, labelOK, test.token, issue)
			}
		})
	}
}

func TestProfilerFtraceEventIssueHardFieldWhitelists(t *testing.T) {
	tests := []struct {
		name       string
		eventField int
		source     profilerFtraceEventDegradationKind
		token      string
		want       bool
	}{
		{name: "binder field range", eventField: 113, source: profilerFtraceEventDegradationCorePayload, token: "core_field7_out_of_range", want: true},
		{name: "binder received range is semantic", eventField: 119, source: profilerFtraceEventDegradationCorePayload, token: "core_field1_out_of_range", want: false},
		{name: "wakeup success range", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "core_field4_out_of_range", want: true},
		{name: "wakeup pid range has semantic token", eventField: 2420, source: profilerFtraceEventDegradationCorePayload, token: "core_field2_out_of_range", want: false},
		{name: "f2fs dev range", eventField: 4010, source: profilerFtraceEventDegradationAuxPayload, token: "core_field1_out_of_range", want: true},
		{name: "f2fs inode never range token", eventField: 4010, source: profilerFtraceEventDegradationAuxPayload, token: "core_field2_out_of_range", want: false},
		{name: "clock name missing", eventField: 410, source: profilerFtraceEventDegradationWireAudit, token: "core_field1_missing_or_invalid", want: true},
		{name: "clock rate never missing token", eventField: 410, source: profilerFtraceEventDegradationWireAudit, token: "core_field2_missing_or_invalid", want: false},
		{name: "clock rate never range token", eventField: 410, source: profilerFtraceEventDegradationWireAudit, token: "core_field2_out_of_range", want: false},
		{name: "block rwbs semantic token", eventField: 204, source: profilerFtraceEventDegradationBlockPayload, token: "core_field4_missing_or_invalid", want: true},
		{name: "block dev never missing token", eventField: 204, source: profilerFtraceEventDegradationBlockPayload, token: "core_field1_missing_or_invalid", want: false},
		{name: "block rwbs never range token", eventField: 204, source: profilerFtraceEventDegradationBlockPayload, token: "core_field4_out_of_range", want: false},
		{name: "block dev range token", eventField: 204, source: profilerFtraceEventDegradationBlockPayload, token: "core_field1_out_of_range", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issue, ok := profilerFtraceEventIssueFromLegacy(test.eventField, test.source, test.token)
			if ok != test.want {
				t.Fatalf("admitted=%t want=%t field=%d source=%v token=%q issue=%+v",
					ok, test.want, test.eventField, test.source, test.token, issue)
			}
		})
	}
}

func TestProfilerFtraceEventIssueFixedPayloadFieldGolden(t *testing.T) {
	seen := [profilerFtraceEventIssueKindCount]bool{}
	check := func(eventField int, source profilerFtraceEventDegradationKind, payloadField uint8, tokens ...string) {
		t.Helper()
		for _, token := range tokens {
			issue, ok := profilerFtraceEventIssueFromLegacy(eventField, source, token)
			if !ok || issue.PayloadField != payloadField {
				t.Fatalf("fixed payload field drifted: event=%d source=%v token=%q issue=%+v ok=%t want_field=%d",
					eventField, source, token, issue, ok, payloadField)
			}
			seen[issue.Kind] = true
		}
	}

	check(0, profilerFtraceEventDegradationEnvelope, 0,
		"envelope_event_malformed_wire", "envelope_oneof_missing", "envelope_oneof_multiple", "envelope_oneof_wrong_wire")
	check(profilerFtraceCPUDetailEnvelopeField, profilerFtraceEventDegradationEnvelope, 0,
		"envelope_trace_plugin_malformed_wire", "envelope_cpu_detail_malformed_wire")
	check(2003, profilerFtraceEventDegradationEnvelope, 0, "envelope_identity_incomplete")
	check(2003, profilerFtraceEventDegradationEnvelope, 1,
		"envelope_cpu_duplicate", "envelope_cpu_wrong_wire", "envelope_cpu_out_of_range",
		"envelope_timestamp_duplicate", "envelope_timestamp_wrong_wire", "envelope_timestamp_out_of_range",
		"envelope_common_type_duplicate", "envelope_common_type_wrong_wire", "envelope_common_type_source_width")
	check(profilerFtraceCPUDetailEnvelopeField, profilerFtraceEventDegradationEnvelope, 2, "envelope_event_container_wrong_wire")
	check(2003, profilerFtraceEventDegradationEnvelope, 2,
		"envelope_tgid_duplicate", "envelope_tgid_wrong_wire", "envelope_tgid_out_of_range",
		"envelope_common_flags_duplicate", "envelope_common_flags_wrong_wire", "envelope_common_flags_source_width")
	check(profilerFtraceCPUDetailEnvelopeField, profilerFtraceEventDegradationEnvelope, 3, "envelope_overwrite_invalid")
	check(2003, profilerFtraceEventDegradationEnvelope, 3,
		"envelope_comm_duplicate", "envelope_comm_wrong_wire", "envelope_comm_invalid",
		"envelope_common_preempt_count_duplicate", "envelope_common_preempt_count_wrong_wire", "envelope_common_preempt_count_source_width")
	check(2003, profilerFtraceEventDegradationEnvelope, 4,
		"envelope_common_pid_duplicate", "envelope_common_pid_wrong_wire", "envelope_common_pid_out_of_range")
	check(2003, profilerFtraceEventDegradationEnvelope, 50,
		"envelope_common_fields_missing", "envelope_common_fields_duplicate", "envelope_common_fields_wrong_wire", "envelope_common_fields_malformed_wire")

	check(113, profilerFtraceEventDegradationCorePayload, 0,
		"core_payload_malformed_wire", "invalid_transaction_endpoint", "invalid_canonical_core_line")
	check(2004, profilerFtraceEventDegradationCorePayload, 0, "invalid_limits_profile", "invalid_limits_order")
	check(113, profilerFtraceEventDegradationCorePayload, 1, "invalid_transaction_id")
	check(1400, profilerFtraceEventDegradationCorePayload, 1, "missing_or_invalid_reason")
	check(1500, profilerFtraceEventDegradationCorePayload, 1, "missing_or_invalid_irq")
	check(1502, profilerFtraceEventDegradationCorePayload, 1, "missing_or_invalid_vec")
	check(2003, profilerFtraceEventDegradationCorePayload, 1, "missing_or_invalid_state")
	check(4002, profilerFtraceEventDegradationCorePayload, 1, "missing_or_invalid_pid")
	check(2420, profilerFtraceEventDegradationCorePayload, 1,
		"display_comm_wrong_wire", "display_comm_duplicate", "display_comm_invalid", "display_comm_unavailable", "display_comm_out_of_profile")
	check(1402, profilerFtraceEventDegradationCorePayload, 2, "missing_or_invalid_reason")
	check(1500, profilerFtraceEventDegradationCorePayload, 2, "missing_or_invalid_irq_name")
	check(1501, profilerFtraceEventDegradationCorePayload, 2, "missing_or_invalid_ret")
	check(2003, profilerFtraceEventDegradationCorePayload, 2, "missing_or_invalid_cpu_id")
	check(2420, profilerFtraceEventDegradationCorePayload, 2, "missing_or_invalid_pid")
	check(2004, profilerFtraceEventDegradationCorePayload, 3, "missing_or_invalid_cpu_id")
	check(2420, profilerFtraceEventDegradationCorePayload, 3, "missing_or_invalid_priority")
	check(4002, profilerFtraceEventDegradationCorePayload, 3, "missing_or_invalid_iowait")
	check(4002, profilerFtraceEventDegradationCorePayload, 4,
		"display_caller_str_wrong_wire", "display_caller_str_duplicate", "display_caller_str_invalid")
	check(113, profilerFtraceEventDegradationCorePayload, 5, "invalid_reply")
	check(2420, profilerFtraceEventDegradationCorePayload, 5, "missing_or_invalid_target_cpu")

	check(4015, profilerFtraceEventDegradationAuxPayload, 0,
		"aux_payload_malformed_wire", "invalid_canonical_aux_line")
	check(4009, profilerFtraceEventDegradationAuxPayload, 0, "invalid_f2fs_payload_range")
	check(4009, profilerFtraceEventDegradationAuxPayload, 1, "missing_or_invalid_f2fs_dev")
	check(1109, profilerFtraceEventDegradationAuxPayload, 2, "missing_or_invalid_print_buf")
	check(4009, profilerFtraceEventDegradationAuxPayload, 2, "missing_or_invalid_f2fs_ino")
	check(4015, profilerFtraceEventDegradationAuxPayload, 22, "missing_or_invalid_mmc_pointer")
	check(4015, profilerFtraceEventDegradationAuxPayload, 23, "missing_or_invalid_mmc_name")
	check(4016, profilerFtraceEventDegradationAuxPayload, 24, "missing_or_invalid_mmc_pointer")
	check(4016, profilerFtraceEventDegradationAuxPayload, 25, "missing_or_invalid_mmc_name")

	check(1000, profilerFtraceEventDegradationFilemapPayload, 1, "filemap_pfn_invalid")
	check(1000, profilerFtraceEventDegradationFilemapPayload, 2, "filemap_inode_invalid")
	check(1000, profilerFtraceEventDegradationFilemapPayload, 3, "filemap_index_invalid")
	check(1000, profilerFtraceEventDegradationFilemapPayload, 4, "filemap_device_invalid")
	check(1000, profilerFtraceEventDegradationFilemapPayload, 5, "filemap_order_invalid")
	check(1000, profilerFtraceEventDegradationFilemapPayload, 0, "invalid_canonical_filemap_line")

	blockDisplay := []string{"comm_malformed_wire", "comm_wrong_wire", "comm_duplicate", "comm_unsafe_omitted"}
	check(204, profilerFtraceEventDegradationBlockPayload, 5, blockDisplay...)
	check(210, profilerFtraceEventDegradationBlockPayload, 6, blockDisplay...)
	blockCommand := []string{"cmd_malformed_wire", "cmd_wrong_wire", "cmd_duplicate", "cmd_unsafe_omitted"}
	check(209, profilerFtraceEventDegradationBlockPayload, 6, blockCommand...)
	check(210, profilerFtraceEventDegradationBlockPayload, 7, blockCommand...)

	check(2002, profilerFtraceEventDegradationWireAudit, 3,
		"cpu_id_malformed_wire", "cpu_id_wrong_wire", "cpu_id_duplicate", "cpu_id_out_of_range")
	check(2417, profilerFtraceEventDegradationWireAudit, 8,
		"next_info_malformed_wire", "next_info_wrong_wire", "next_info_duplicate")
	check(9_999, profilerFtraceEventDegradationUnmappedField, 0, "unmapped structured ftrace event field")

	for kind := profilerFtraceEventIssueKind(0); kind < profilerFtraceEventIssueKindCount; kind++ {
		if !profilerFtraceEventIssueParameterizedKind(kind) && !seen[kind] {
			t.Fatalf("fixed issue kind %d lacks independent payload-field golden", kind)
		}
	}
}

func TestProfilerFtraceEventIssueVerdictConservation(t *testing.T) {
	display, ok := profilerFtraceEventIssueFromLegacy(
		2002, profilerFtraceEventDegradationWireAudit, "cpu_id_wrong_wire",
	)
	if !ok {
		t.Fatal("fixture: cpu field-audit issue was rejected")
	}
	hard, ok := profilerFtraceEventIssueFromLegacy(
		2420, profilerFtraceEventDegradationCorePayload, "core_field2_wrong_wire",
	)
	if !ok {
		t.Fatal("fixture: core hard-reject issue was rejected")
	}
	coreDisplay, ok := profilerFtraceEventIssueFromLegacy(
		2420, profilerFtraceEventDegradationCorePayload, "display_comm_wrong_wire",
	)
	if !ok {
		t.Fatal("fixture: core display issue was rejected")
	}
	unknown, ok := profilerFtraceEventIssueFromLegacy(
		987654, profilerFtraceEventDegradationUnmappedField, "unmapped structured ftrace event field",
	)
	if !ok {
		t.Fatal("fixture: unknown-event issue was rejected")
	}

	tests := []struct {
		name        string
		eventField  int
		publishable bool
		issues      []profilerFtraceEventIssue
		want        bool
	}{
		{name: "clean known row publishes", eventField: 2420, publishable: true, want: true},
		{name: "display issue requires published row", eventField: 2002, publishable: true, issues: []profilerFtraceEventIssue{display}, want: true},
		{name: "display issue cannot reject row", eventField: 2002, publishable: false, issues: []profilerFtraceEventIssue{display}, want: false},
		{name: "hard issue requires rejected row", eventField: 2420, publishable: false, issues: []profilerFtraceEventIssue{hard}, want: true},
		{name: "hard issue cannot publish row", eventField: 2420, publishable: true, issues: []profilerFtraceEventIssue{hard}, want: false},
		{name: "mixed verdict cannot publish", eventField: 2420, publishable: true, issues: []profilerFtraceEventIssue{hard, coreDisplay}, want: false},
		{name: "mixed verdict cannot reject", eventField: 2420, publishable: false, issues: []profilerFtraceEventIssue{hard, coreDisplay}, want: false},
		{name: "rejected row needs typed issue", eventField: 2420, publishable: false, want: false},
		{name: "unknown event requires rejection", eventField: 987654, publishable: false, issues: []profilerFtraceEventIssue{unknown}, want: true},
		{name: "unknown event cannot publish", eventField: 987654, publishable: true, issues: []profilerFtraceEventIssue{unknown}, want: false},
		{name: "event envelope cannot clean publish", eventField: 0, publishable: true, want: false},
		{name: "cpu detail envelope cannot clean publish", eventField: profilerFtraceCPUDetailEnvelopeField, publishable: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := profilerFtraceEventIssueVerdictValid(test.eventField, test.publishable, test.issues); got != test.want {
				t.Fatalf("verdict=%t want=%t field=%d publishable=%t issues=%+v", got, test.want, test.eventField, test.publishable, test.issues)
			}
		})
	}
}

func TestProfilerFtraceEventIssueCannotBeRelabeledOrSeverityFlipped(t *testing.T) {
	cpuDisplay, ok := profilerFtraceEventIssueFromLegacy(
		2002, profilerFtraceEventDegradationWireAudit, "cpu_id_wrong_wire",
	)
	if !ok {
		t.Fatal("fixture: cpu field-audit issue was rejected")
	}
	if cpuDisplay.validFor(410) {
		t.Fatal("field-2002 CPU issue became valid for same-name field 410")
	}
	if label, labelOK := cpuDisplay.label(410); labelOK || label != "" {
		t.Fatalf("field-2002 CPU issue relabeled for field 410: label=(%q,%t)", label, labelOK)
	}

	tamperedDisplay := cpuDisplay
	tamperedDisplay.Severity = profilerFtraceEventIssueHardReject
	if tamperedDisplay.validFor(2002) {
		t.Fatalf("display issue admitted after hard-reject severity flip: %+v", tamperedDisplay)
	}
	if label, labelOK := tamperedDisplay.label(2002); labelOK || label != "" {
		t.Fatalf("severity-flipped display issue produced label=(%q,%t)", label, labelOK)
	}

	hard, ok := profilerFtraceEventIssueFromLegacy(
		2420, profilerFtraceEventDegradationCorePayload, "core_field2_wrong_wire",
	)
	if !ok {
		t.Fatal("fixture: core hard-reject issue was rejected")
	}
	tamperedHard := hard
	tamperedHard.Severity = profilerFtraceEventIssueAdmittedDisplay
	if tamperedHard.validFor(2420) {
		t.Fatalf("hard issue admitted after display severity flip: %+v", tamperedHard)
	}
}

func TestProfilerFtraceEventTypedChokeFailsUnknownLegacyButDirectStaysCompatible(t *testing.T) {
	event := profilerFtraceEventRecord{
		Field:                2003,
		EnvelopeDegradations: []string{"future_envelope_reason"},
	}
	if _, _, ok, reasons := renderProfilerFtraceEventBodyWithAudit(event); ok ||
		len(reasons) != 1 || reasons[0] != "future_envelope_reason" {
		t.Fatalf("direct compatibility lane changed: ok=%t reasons=%v", ok, reasons)
	}
	_, _, ok, issues, err := renderProfilerFtraceEventBodyWithTypedAudit(event)
	if ok || len(issues) != 0 || err == nil {
		t.Fatalf("typed choke admitted unknown legacy issue: ok=%t issues=%+v err=%v", ok, issues, err)
	}
	invariant, typed := err.(*traceDBOutputInvariantError)
	if !typed || invariant.Reason != "profiler_event_legacy_issue_unmapped" {
		t.Fatalf("typed choke error=%T %v", err, err)
	}
}

func TestProfilerFtraceEventTypedChokeRejectsOutOfProtobufFieldDomain(t *testing.T) {
	event := profilerFtraceEventRecord{Field: profilerFtraceUnknownEventAggregateField + 1}
	_, _, ok, issues, err := renderProfilerFtraceEventBodyWithTypedAudit(event)
	if ok || len(issues) != 0 || err == nil {
		t.Fatalf("out-of-domain event field escaped typed choke: ok=%t issues=%+v err=%v", ok, issues, err)
	}
	invariant, typed := err.(*traceDBOutputInvariantError)
	if !typed || invariant.Reason != "profiler_event_field_domain_invalid" {
		t.Fatalf("out-of-domain error=%T %v", err, err)
	}
}

func TestProfilerFtraceEventUnknownRetainsEnvelopeAndUnmappedIssuesInFixedBucket(t *testing.T) {
	event := profilerFtraceEventRecord{
		Field:                9_999,
		EnvelopeDegradations: []string{"envelope_cpu_duplicate"},
	}
	if _, _, ok, reasons := renderProfilerFtraceEventBodyWithAudit(event); ok ||
		len(reasons) != 1 || reasons[0] != "envelope_cpu_duplicate" {
		t.Fatalf("direct unknown envelope compatibility drifted: ok=%t reasons=%v", ok, reasons)
	}
	_, _, ok, issues, err := renderProfilerFtraceEventBodyWithTypedAudit(event)
	if err != nil || ok || len(issues) != 2 ||
		issues[0].Kind != profilerFtraceEventIssueEnvelopeCPUDuplicate ||
		issues[1].Kind != profilerFtraceEventIssueUnmappedField {
		t.Fatalf("typed unknown did not retain envelope plus unmapped issues: ok=%t issues=%+v err=%v", ok, issues, err)
	}
	envelope, envelopeOK := profilerFtraceEventIssueFromLegacy(
		event.Field, profilerFtraceEventDegradationEnvelope, "envelope_cpu_duplicate",
	)
	if !envelopeOK || profilerFtraceEventIssueVerdictValid(event.Field, false, []profilerFtraceEventIssue{envelope}) ||
		!profilerFtraceEventIssueVerdictValid(event.Field, false, issues) {
		t.Fatalf("unknown slot envelope/unmapped conservation drifted: envelope=%+v ok=%t issues=%+v", envelope, envelopeOK, issues)
	}
	var batch profilerFtraceEventBatchCensus
	if !batch.observeRead(event.Field) || !batch.observeIssues(event.Field, false, issues) {
		t.Fatal("observe normalized unknown issue")
	}
	var ledger profilerFtraceEventDiagnosticLedger
	if !ledger.merge(batch) {
		t.Fatal("merge normalized unknown issue")
	}
	out := profilerContainerExtraction{}
	if !ledger.materialize(&out) {
		t.Fatal("materialize normalized unknown issue")
	}
	coverage, entries := profilerUnknownEventCoverage(out)
	if entries != 1 || coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
		coverage.FieldSources["degraded_unmapped_field_occurrences"] != "1" ||
		coverage.FieldSources["degraded_envelope_occurrences"] != "1" ||
		coverage.FieldSources["degraded_envelope_cpu_duplicate_occurrences"] != "1" {
		t.Fatalf("unknown fixed bucket issue drifted: entries=%d coverage=%+v", entries, coverage)
	}
}

func TestProfilerFtraceEventIssueFiniteUniverseRoundTripsAndFitsFixedCensus(t *testing.T) {
	eventFields := []int{profilerFtraceCPUDetailEnvelopeField, 0, 100, 9_999, profilerFtraceUnknownEventAggregateField}
	for _, descriptor := range profilerFtraceEventDescriptorList {
		eventFields = append(eventFields, descriptor.Field)
	}
	seenKinds := [profilerFtraceEventIssueKindCount]bool{}
	labels := map[string]profilerFtraceEventIssue{}
	total := 0
	maxPerEvent := 0
	for _, eventField := range eventFields {
		perEvent := 0
		for kind := profilerFtraceEventIssueKind(0); kind < profilerFtraceEventIssueKindCount; kind++ {
			for payloadField := 0; payloadField <= 255; payloadField++ {
				for severity := profilerFtraceEventIssueSeverity(0); severity < profilerFtraceEventIssueSeverityCount; severity++ {
					issue := profilerFtraceEventIssue{Kind: kind, PayloadField: uint8(payloadField), Severity: severity}
					if !issue.validFor(eventField) {
						continue
					}
					unknownSlot := profilerFtraceEventSlot(eventField) == profilerFtraceUnknownEventSlot
					if unknownSlot {
						if issue.Kind != profilerFtraceEventIssueUnmappedField &&
							issue.sourceClass() != profilerFtraceEventDegradationEnvelope {
							continue
						}
					} else {
						publishable := severity == profilerFtraceEventIssueAdmittedDisplay
						if !profilerFtraceEventIssueVerdictValid(eventField, publishable, []profilerFtraceEventIssue{issue}) {
							continue
						}
					}
					label, ok := issue.label(eventField)
					if !ok || label == "" {
						t.Fatalf("valid issue has no label: field=%d issue=%+v", eventField, issue)
					}
					legacySource := profilerFtraceEventIssueLegacySource(issue.sourceClass())
					roundTrip, ok := profilerFtraceEventIssueFromLegacy(eventField, legacySource, label)
					if !ok || roundTrip != issue {
						t.Fatalf("exact issue failed label/bridge round trip: field=%d source=%v label=%q issue=%+v round=%+v ok=%t",
							eventField, legacySource, label, issue, roundTrip, ok)
					}
					key := fmt.Sprintf("%d/%d/%s", eventField, legacySource, label)
					if previous, exists := labels[key]; exists && previous != issue {
						t.Fatalf("legacy label collision at %s: first=%+v second=%+v", key, previous, issue)
					}
					labels[key] = issue
					seenKinds[kind] = true
					perEvent++
					total++
				}
			}
		}
		if perEvent > profilerFtraceEventIssuesPerSlot {
			t.Fatalf("field %d legal issue universe=%d exceeds fixed census=%d", eventField, perEvent, profilerFtraceEventIssuesPerSlot)
		}
		if perEvent > maxPerEvent {
			maxPerEvent = perEvent
		}
	}
	for kind, seen := range seenKinds {
		if !seen {
			t.Fatalf("closed issue kind %d has no legal event tuple", kind)
		}
	}
	// Literal totals pin schema expansion and force an explicit census-capacity
	// review. Update only with the independent event/payload golden matrices.
	if total != 1_820 || maxPerEvent != 106 {
		t.Fatalf("finite issue universe drifted: total=%d want=1820 max_per_event=%d want=106", total, maxPerEvent)
	}
}

func profilerFtraceEventIssueLegacySource(class profilerFtraceEventDegradationKind) profilerFtraceEventDegradationKind {
	switch class {
	case profilerFtraceEventDegradationCoreDisplay:
		return profilerFtraceEventDegradationCorePayload
	case profilerFtraceEventDegradationAuxDisplay:
		return profilerFtraceEventDegradationAuxPayload
	case profilerFtraceEventDegradationBlockDisplay:
		return profilerFtraceEventDegradationBlockPayload
	case profilerFtraceEventDegradationFieldAudit:
		return profilerFtraceEventDegradationWireAudit
	default:
		return class
	}
}
