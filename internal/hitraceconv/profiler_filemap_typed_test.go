package hitraceconv

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func profilerFilemapTestEvent(eventField int, payload []byte) profilerFtraceEventRecord {
	return profilerFtraceEventRecord{
		CPU: 0, TSNS: 1, TGID: 7, PID: 7, HeaderOwnerKnown: true,
		Comm: "filemap", Field: eventField, Payload: payload,
	}
}

func profilerFilemapTestProtoKey(field, wire int) []byte {
	var raw [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(raw[:], uint64(field<<3|wire))
	return append([]byte(nil), raw[:n]...)
}

func profilerFilemapTestProtoVarint(field int, value uint64) []byte {
	out := profilerFilemapTestProtoKey(field, 0)
	var raw [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(raw[:], value)
	return append(out, raw[:n]...)
}

func profilerFilemapTestProtoBytes(field int, value []byte) []byte {
	out := profilerFilemapTestProtoKey(field, 2)
	var raw [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(raw[:], uint64(len(value)))
	out = append(out, raw[:n]...)
	return append(out, value...)
}

func profilerFilemapTestPayload(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func profilerFilemapTestBasePayload() []byte {
	return profilerFilemapTestPayload(
		profilerFilemapTestProtoVarint(1, 1),
		profilerFilemapTestProtoVarint(2, 2),
		profilerFilemapTestProtoVarint(3, 3),
		profilerFilemapTestProtoVarint(4, 4),
		profilerFilemapTestProtoVarint(5, 5),
	)
}

func profilerFilemapTestIssue(t *testing.T, eventField int, kind profilerFtraceEventIssueKind) profilerFtraceEventIssue {
	t.Helper()
	issue, ok := profilerFtraceEventFixedIssue(eventField, kind)
	if !ok {
		t.Fatalf("event %d kind %d is not a legal fixed filemap issue", eventField, kind)
	}
	return issue
}

func profilerFilemapTestInvariantReason(t *testing.T, err error) string {
	t.Helper()
	var invariant *traceDBOutputInvariantError
	if !errors.As(err, &invariant) {
		t.Fatalf("expected traceDBOutputInvariantError, got %T: %v", err, err)
	}
	return invariant.Reason
}

func profilerFilemapAssertRejected(t *testing.T, event profilerFtraceEventRecord, kind profilerFtraceEventIssueKind, label string) {
	t.Helper()
	name, body, ok, issues, handled, err := renderProfilerFtraceFilemapEventWithTypedAudit(event)
	want := profilerFilemapTestIssue(t, event.Field, kind)
	if err != nil || !handled || ok || name != "" || body != "" || !reflect.DeepEqual(issues, []profilerFtraceEventIssue{want}) {
		t.Fatalf("typed filemap rejection drifted: name=%q body=%q ok=%v handled=%v issues=%+v want=%+v err=%v",
			name, body, ok, handled, issues, want, err)
	}
	payload, admission, reason := decodeProfilerFilemapPayload(event)
	if payload != (filemapRenderPayload{}) || admission != bodyRejected || reason != label {
		t.Fatalf("compat filemap rejection drifted: payload=%+v admission=%d reason=%q want=%q",
			payload, admission, reason, label)
	}
}

func TestProfilerFilemapTypedExactLegalTupleCensus(t *testing.T) {
	if profilerFtraceFilemapIssuesPerEvent != 1 {
		t.Fatalf("filemap issue capacity=%d want=1", profilerFtraceFilemapIssuesPerEvent)
	}
	kinds := []struct {
		kind         profilerFtraceEventIssueKind
		payloadField uint8
		label        string
	}{
		{profilerFtraceEventIssueFilemapPayloadMalformedWire, 0, "filemap_payload_malformed_wire"},
		{profilerFtraceEventIssueFilemapPFNInvalid, 1, "filemap_pfn_invalid"},
		{profilerFtraceEventIssueFilemapInodeInvalid, 2, "filemap_inode_invalid"},
		{profilerFtraceEventIssueFilemapIndexInvalid, 3, "filemap_index_invalid"},
		{profilerFtraceEventIssueFilemapDeviceInvalid, 4, "filemap_device_invalid"},
		{profilerFtraceEventIssueFilemapOrderInvalid, 5, "filemap_order_invalid"},
	}
	type tuple struct {
		eventField   int
		kind         profilerFtraceEventIssueKind
		payloadField uint8
		severity     profilerFtraceEventIssueSeverity
	}
	expected := make(map[tuple]bool)
	for _, eventField := range []int{1000, 1001} {
		for _, item := range kinds {
			issue := profilerFilemapTestIssue(t, eventField, item.kind)
			if issue.PayloadField != item.payloadField || issue.Severity != profilerFtraceEventIssueHardReject ||
				issue.expectedSeverity() != profilerFtraceEventIssueHardReject {
				t.Fatalf("event %d kind %d tuple drifted: %+v", eventField, item.kind, issue)
			}
			label, ok := issue.label(eventField)
			if !ok || label != item.label {
				t.Fatalf("event %d kind %d label=%q ok=%v want=%q", eventField, item.kind, label, ok, item.label)
			}
			expected[tuple{eventField, issue.Kind, issue.PayloadField, issue.Severity}] = true
		}
	}
	actual := make(map[tuple]bool)
	for _, eventField := range []int{1000, 1001} {
		for kind := profilerFtraceEventIssueFilemapPayloadMalformedWire; kind <= profilerFtraceEventIssueFilemapOrderInvalid; kind++ {
			for payloadField := 0; payloadField <= math.MaxUint8; payloadField++ {
				for severity := profilerFtraceEventIssueHardReject; severity < profilerFtraceEventIssueSeverityCount; severity++ {
					issue := profilerFtraceEventIssue{
						Kind: kind, PayloadField: uint8(payloadField), Severity: severity,
					}
					if issue.validFor(eventField) {
						actual[tuple{eventField, kind, uint8(payloadField), severity}] = true
					}
				}
			}
		}
	}
	if len(expected) != 12 || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("filemap legal tuple closure drifted: actual=%+v expected=%+v", actual, expected)
	}
	for _, eventField := range []int{999, 1002} {
		for _, item := range kinds {
			if _, ok := profilerFtraceEventFixedIssue(eventField, item.kind); ok {
				t.Fatalf("foreign event %d accepted filemap kind %d", eventField, item.kind)
			}
		}
	}
}

func TestProfilerFilemapTypedKnownEndpointRawShapeMatrix(t *testing.T) {
	kinds := [...]profilerFtraceEventIssueKind{
		0,
		profilerFtraceEventIssueFilemapPFNInvalid,
		profilerFtraceEventIssueFilemapInodeInvalid,
		profilerFtraceEventIssueFilemapIndexInvalid,
		profilerFtraceEventIssueFilemapDeviceInvalid,
		profilerFtraceEventIssueFilemapOrderInvalid,
	}
	labels := [...]string{"", "filemap_pfn_invalid", "filemap_inode_invalid", "filemap_index_invalid", "filemap_device_invalid", "filemap_order_invalid"}
	shapes := 0
	for _, eventField := range []int{1000, 1001} {
		for payloadField := 1; payloadField <= 5; payloadField++ {
			fixtures := [][]byte{
				profilerFilemapTestProtoBytes(payloadField, []byte{1}),
				profilerFilemapTestPayload(
					profilerFilemapTestProtoVarint(payloadField, 1),
					profilerFilemapTestProtoVarint(payloadField, 2),
				),
				append(profilerFilemapTestProtoKey(payloadField, 0), 0x80),
			}
			for _, fixture := range fixtures {
				profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField, fixture), kinds[payloadField], labels[payloadField])
				shapes++
			}
		}
	}
	if shapes != 30 {
		t.Fatalf("known endpoint raw shape census=%d want=30", shapes)
	}
}

func TestProfilerFilemapTypedMalformedProvenanceAndUnknownExtensions(t *testing.T) {
	base := profilerFilemapTestBasePayload()
	for _, eventField := range []int{1000, 1001} {
		event := profilerFilemapTestEvent(eventField, base)
		name, body, ok, issues, handled, err := renderProfilerFtraceFilemapEventWithTypedAudit(event)
		wantName := "mm_filemap_add_to_page_cache"
		if eventField == 1001 {
			wantName = "mm_filemap_delete_from_page_cache"
		}
		if err != nil || !handled || !ok || name != wantName ||
			body != "dev 0:4 ino 0x2 pfn=1 ofs=12288" || len(issues) != 0 {
			t.Fatalf("valid event %d drifted: name=%q body=%q ok=%v handled=%v issues=%+v err=%v",
				eventField, name, body, ok, handled, issues, err)
		}
		withUnknown := profilerFilemapTestPayload(base, profilerFilemapTestProtoVarint(99, 7))
		unknownName, unknownBody, unknownOK, unknownIssues, unknownHandled, unknownErr :=
			renderProfilerFtraceFilemapEventWithTypedAudit(profilerFilemapTestEvent(eventField, withUnknown))
		if unknownErr != nil || !unknownHandled || !unknownOK || unknownName != name || unknownBody != body || len(unknownIssues) != 0 {
			t.Fatalf("complete unknown extension changed event %d: name=%q body=%q ok=%v handled=%v issues=%+v err=%v",
				eventField, unknownName, unknownBody, unknownOK, unknownHandled, unknownIssues, unknownErr)
		}

		wholeFixtures := [][]byte{
			profilerFilemapTestPayload(base, []byte{0x80}),
			[]byte{0},
			profilerFilemapTestPayload(base, append(profilerFilemapTestProtoKey(99, 0), 0x80)),
			profilerFilemapTestPayload(base, profilerFilemapTestProtoKey(99, 7)),
		}
		for _, fixture := range wholeFixtures {
			profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField, fixture),
				profilerFtraceEventIssueFilemapPayloadMalformedWire, "filemap_payload_malformed_wire")
		}

		// Regression: the old five-scan decoder collapsed this exact f5 failure
		// into PFN-invalid because its first scalar scan observed the tail error.
		knownF5 := profilerFilemapTestPayload(
			profilerFilemapTestProtoVarint(1, 1),
			profilerFilemapTestProtoVarint(2, 2),
			profilerFilemapTestProtoVarint(3, 3),
			profilerFilemapTestProtoVarint(4, 4),
			append(profilerFilemapTestProtoKey(5, 0), 0x80),
		)
		profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField, knownF5),
			profilerFtraceEventIssueFilemapOrderInvalid, "filemap_order_invalid")
	}
}

func TestProfilerFilemapTypedContainerCoverageAndSiblingLocality(t *testing.T) {
	validAdd := profilerFilemapTestBasePayload()
	validDelete := profilerFilemapTestPayload(
		profilerFilemapTestProtoVarint(1, 6),
		profilerFilemapTestProtoVarint(2, 7),
		profilerFilemapTestProtoVarint(3, 8),
		profilerFilemapTestProtoVarint(4, 9),
	)
	badWhole := profilerFilemapTestPayload(validAdd, []byte{0x80})
	badOrder := profilerFilemapTestPayload(
		profilerFilemapTestProtoVarint(1, 6),
		profilerFilemapTestProtoVarint(2, 7),
		profilerFilemapTestProtoVarint(3, 8),
		profilerFilemapTestProtoVarint(4, 9),
		append(profilerFilemapTestProtoKey(5, 0), 0x80),
	)
	structured := protoMessage(2,
		profilerFilemapTestProtoVarint(1, 3),
		syntheticTracePluginFtraceEvent(1_000, 10, 10, "bad-add", 1000, badWhole),
		syntheticTracePluginFtraceEvent(2_000, 10, 10, "good-add", 1000, validAdd),
		syntheticTracePluginFtraceEvent(3_000, 10, 10, "bad-delete", 1001, badOrder),
		syntheticTracePluginFtraceEvent(4_000, 10, 10, "good-delete", 1001, validDelete),
	)
	sink, err := newTraceDBRowSink("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredRows(structured, &seq, sink)
	if err != nil || rows != 2 || sink.publishableRows() != 2 {
		t.Fatalf("filemap container locality drifted: rows=%d publishable=%d err=%v coverage=%+v",
			rows, sink.publishableRows(), err, coverage)
	}
	add := coverageForTable(coverage, "mm_filemap_add_to_page_cache")
	if add == nil || add.RowsRead != 2 || add.RowsEmitted != 1 ||
		add.FieldSources["degraded_filemap_payload_malformed_wire_rows"] != "1" ||
		add.Skipped != "filemap_payload_malformed_wire=1" {
		t.Fatalf("filemap whole-malformed coverage drifted: %+v", add)
	}
	deleted := coverageForTable(coverage, "mm_filemap_delete_from_page_cache")
	if deleted == nil || deleted.RowsRead != 2 || deleted.RowsEmitted != 1 ||
		deleted.FieldSources["degraded_filemap_order_invalid_rows"] != "1" ||
		deleted.Skipped != "filemap_order_invalid=1" {
		t.Fatalf("filemap localized-malformed coverage drifted: %+v", deleted)
	}
}

func TestProfilerFilemapTypedDominanceAndSchemaOrder(t *testing.T) {
	for _, eventField := range []int{1000, 1001} {
		// Physical f2 appears first, but completed-scan selection is schema-first
		// and therefore the later duplicate f1 wins deterministically.
		schemaFirst := profilerFilemapTestPayload(
			profilerFilemapTestProtoBytes(2, []byte{1}),
			profilerFilemapTestProtoVarint(1, 1),
			profilerFilemapTestProtoVarint(1, 2),
		)
		profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField, schemaFirst),
			profilerFtraceEventIssueFilemapPFNInvalid, "filemap_pfn_invalid")

		// Any completed-scan wire failure dominates numeric range failures.
		wireBeforeRange := profilerFilemapTestPayload(
			profilerFilemapTestProtoVarint(3, uint64(math.MaxInt64>>12)+1),
			profilerFilemapTestProtoBytes(5, []byte{1}),
		)
		profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField, wireBeforeRange),
			profilerFtraceEventIssueFilemapOrderInvalid, "filemap_order_invalid")

		// Range selection is also schema-first, independent of wire order.
		rangeOrder := profilerFilemapTestPayload(
			profilerFilemapTestProtoVarint(5, math.MaxUint8+1),
			profilerFilemapTestProtoVarint(4, uint64(math.MaxUint32)+1),
			profilerFilemapTestProtoVarint(3, uint64(math.MaxInt64>>12)+1),
		)
		profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField, rangeOrder),
			profilerFtraceEventIssueFilemapIndexInvalid, "filemap_index_invalid")

		// An unlocalized structural tail dominates all completed prefix facts.
		unknownTail := profilerFilemapTestPayload(
			profilerFilemapTestProtoBytes(1, []byte{1}),
			append(profilerFilemapTestProtoKey(99, 0), 0x80),
		)
		profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField, unknownTail),
			profilerFtraceEventIssueFilemapPayloadMalformedWire, "filemap_payload_malformed_wire")

		// A localized structural tail joins the already observed hard audit;
		// it must not override a lower schema endpoint solely because it ended
		// the physical walk.
		knownTail := profilerFilemapTestPayload(
			profilerFilemapTestProtoBytes(1, []byte{1}),
			append(profilerFilemapTestProtoKey(5, 0), 0x80),
		)
		profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField, knownTail),
			profilerFtraceEventIssueFilemapPFNInvalid, "filemap_pfn_invalid")

		// The converse cannot infer through an earlier malformed boundary: f1
		// bytes after unsupported f5 were never observed as a protobuf field.
		boundaryFirst := profilerFilemapTestPayload(
			profilerFilemapTestProtoKey(5, 7),
			profilerFilemapTestProtoBytes(1, []byte{1}),
		)
		profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField, boundaryFirst),
			profilerFtraceEventIssueFilemapOrderInvalid, "filemap_order_invalid")
	}
}

func TestProfilerFilemapTypedProtoDefaultsAndBounds(t *testing.T) {
	for _, eventField := range []int{1000, 1001} {
		name, body, ok, issues, handled, err := renderProfilerFtraceFilemapEventWithTypedAudit(
			profilerFilemapTestEvent(eventField, nil))
		if err != nil || !handled || !ok || name == "" || body != "dev 0:0 ino 0x0 pfn=0 ofs=0" || len(issues) != 0 {
			t.Fatalf("proto3 defaults event %d drifted: name=%q body=%q ok=%v handled=%v issues=%+v err=%v",
				eventField, name, body, ok, handled, issues, err)
		}
		maxPayload := profilerFilemapTestPayload(
			profilerFilemapTestProtoVarint(1, math.MaxUint64),
			profilerFilemapTestProtoVarint(2, math.MaxUint64),
			profilerFilemapTestProtoVarint(3, uint64(math.MaxInt64>>12)),
			profilerFilemapTestProtoVarint(4, math.MaxUint32),
			profilerFilemapTestProtoVarint(5, math.MaxUint8),
		)
		maxDecoded, admission, set, handled, err := decodeProfilerFilemapPayloadWithTypedAudit(
			profilerFilemapTestEvent(eventField, maxPayload))
		if err != nil || !handled || admission != bodyAdmitted || set.Count != 0 ||
			maxDecoded.PFN != math.MaxUint64 || maxDecoded.Inode != math.MaxUint64 ||
			maxDecoded.Index != uint64(math.MaxInt64>>12) || maxDecoded.Dev != math.MaxUint32 ||
			!maxDecoded.OrderPresent || maxDecoded.Order != math.MaxUint8 {
			t.Fatalf("exact maxima event %d rejected: payload=%+v admission=%d set=%+v handled=%v err=%v",
				eventField, maxDecoded, admission, set, handled, err)
		}
		zeroOrder, admission, _, _, err := decodeProfilerFilemapPayloadWithTypedAudit(
			profilerFilemapTestEvent(eventField, profilerFilemapTestProtoVarint(5, 0)))
		if err != nil || admission != bodyAdmitted || !zeroOrder.OrderPresent || zeroOrder.Order != 0 {
			t.Fatalf("explicit zero order event %d lost presence: payload=%+v admission=%d err=%v",
				eventField, zeroOrder, admission, err)
		}
		profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField,
			profilerFilemapTestProtoVarint(3, uint64(math.MaxInt64>>12)+1)),
			profilerFtraceEventIssueFilemapIndexInvalid, "filemap_index_invalid")
		profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField,
			profilerFilemapTestProtoVarint(4, uint64(math.MaxUint32)+1)),
			profilerFtraceEventIssueFilemapDeviceInvalid, "filemap_device_invalid")
		profilerFilemapAssertRejected(t, profilerFilemapTestEvent(eventField,
			profilerFilemapTestProtoVarint(5, math.MaxUint8+1)),
			profilerFtraceEventIssueFilemapOrderInvalid, "filemap_order_invalid")
	}
}

func TestProfilerFilemapTypedIssueSetCapacityAndCorruption(t *testing.T) {
	first := profilerFilemapTestIssue(t, 1000, profilerFtraceEventIssueFilemapPFNInvalid)
	second := profilerFilemapTestIssue(t, 1000, profilerFtraceEventIssueFilemapInodeInvalid)
	var set profilerFtraceFilemapIssueSet
	if err := set.add(1000, first); err != nil {
		t.Fatal(err)
	}
	snapshot := set
	if reason := profilerFilemapTestInvariantReason(t, set.add(1000, second)); reason != "profiler_filemap_issue_overflow" {
		t.Fatalf("overflow reason=%q", reason)
	}
	if set != snapshot {
		t.Fatalf("overflow partially mutated set: got=%+v want=%+v", set, snapshot)
	}
	if reason := profilerFilemapTestInvariantReason(t, set.add(1000, first)); reason != "profiler_filemap_issue_duplicate" {
		t.Fatalf("duplicate reason=%q", reason)
	}

	corrupt := []struct {
		name   string
		set    profilerFtraceFilemapIssueSet
		event  int
		reason string
	}{
		{"count overflow", profilerFtraceFilemapIssueSet{Count: 2}, 1000, "profiler_filemap_issue_count_invalid"},
		{"nonzero tail", profilerFtraceFilemapIssueSet{Issues: [1]profilerFtraceEventIssue{first}}, 1000, "profiler_filemap_issue_count_invalid"},
		{"foreign event", profilerFtraceFilemapIssueSet{}, 1002, "profiler_filemap_issue_schema_invalid"},
		{"wrong severity", profilerFtraceFilemapIssueSet{Count: 1, Issues: [1]profilerFtraceEventIssue{{
			Kind: first.Kind, PayloadField: first.PayloadField, Severity: profilerFtraceEventIssueAdmittedDisplay,
		}}}, 1000, "profiler_filemap_issue_schema_invalid"},
		{"wrong endpoint", profilerFtraceFilemapIssueSet{Count: 1, Issues: [1]profilerFtraceEventIssue{{
			Kind: first.Kind, PayloadField: 2, Severity: profilerFtraceEventIssueHardReject,
		}}}, 1000, "profiler_filemap_issue_schema_invalid"},
		{"foreign kind", profilerFtraceFilemapIssueSet{Count: 1, Issues: [1]profilerFtraceEventIssue{{
			Kind: profilerFtraceEventIssueCorePayloadMalformedWire, Severity: profilerFtraceEventIssueHardReject,
		}}}, 1000, "profiler_filemap_issue_schema_invalid"},
	}
	for _, test := range corrupt {
		t.Run(test.name, func(t *testing.T) {
			if reason := profilerFilemapTestInvariantReason(t, test.set.validate(test.event)); reason != test.reason {
				t.Fatalf("reason=%q want=%q", reason, test.reason)
			}
		})
	}
	checked, err := snapshot.checked(1000)
	if err != nil || len(checked) != 1 {
		t.Fatalf("checked set failed: issues=%+v err=%v", checked, err)
	}
	checked[0] = second
	if snapshot.Issues[0] != first {
		t.Fatal("checked returned an alias of the fixed issue set")
	}
}

func TestProfilerFilemapTypedDescriptorAndCanonicalAreInternalInvariants(t *testing.T) {
	validDescriptor := profilerFtraceEventDescriptor{Field: 1000, Family: "filemap", Name: "mm_filemap_add_to_page_cache"}
	if name, kind, err := validateProfilerFilemapDescriptor(1000, validDescriptor, true); err != nil ||
		name != validDescriptor.Name || kind != filemapRenderPageAdd {
		t.Fatalf("valid descriptor rejected: name=%q kind=%d err=%v", name, kind, err)
	}
	descriptorCases := []struct {
		name       string
		eventField int
		descriptor profilerFtraceEventDescriptor
		present    bool
		reason     string
	}{
		{"domain", 999, validDescriptor, true, "profiler_filemap_descriptor_domain_invalid"},
		{"missing", 1000, profilerFtraceEventDescriptor{}, false, "missing_filemap_descriptor"},
		{"field", 1000, profilerFtraceEventDescriptor{Field: 1001, Family: "filemap", Name: validDescriptor.Name}, true, "mismatched_filemap_descriptor_field"},
		{"family", 1000, profilerFtraceEventDescriptor{Field: 1000, Family: "mm", Name: validDescriptor.Name}, true, "mismatched_filemap_descriptor_family"},
		{"name", 1000, profilerFtraceEventDescriptor{Field: 1000, Family: "filemap", Name: "mm_filemap_delete_from_page_cache"}, true, "mismatched_filemap_descriptor_name"},
	}
	for _, test := range descriptorCases {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := validateProfilerFilemapDescriptor(test.eventField, test.descriptor, test.present)
			if reason := profilerFilemapTestInvariantReason(t, err); reason != test.reason {
				t.Fatalf("reason=%q want=%q", reason, test.reason)
			}
		})
	}

	originalDescriptor, originalPresent := profilerFtraceEventDescriptors[1000]
	defer func() {
		if originalPresent {
			profilerFtraceEventDescriptors[1000] = originalDescriptor
		} else {
			delete(profilerFtraceEventDescriptors, 1000)
		}
	}()
	registryCases := []struct {
		name       string
		descriptor profilerFtraceEventDescriptor
		present    bool
		reason     string
	}{
		{"missing registry", profilerFtraceEventDescriptor{}, false, "missing_filemap_descriptor"},
		{"field registry", profilerFtraceEventDescriptor{Field: 1001, Family: "filemap", Name: validDescriptor.Name}, true, "mismatched_filemap_descriptor_field"},
		{"family registry", profilerFtraceEventDescriptor{Field: 1000, Family: "mm", Name: validDescriptor.Name}, true, "mismatched_filemap_descriptor_family"},
		{"name registry", profilerFtraceEventDescriptor{Field: 1000, Family: "filemap", Name: "mm_filemap_delete_from_page_cache"}, true, "mismatched_filemap_descriptor_name"},
	}
	for _, test := range registryCases {
		t.Run(test.name, func(t *testing.T) {
			if test.present {
				profilerFtraceEventDescriptors[1000] = test.descriptor
			} else {
				delete(profilerFtraceEventDescriptors, 1000)
			}
			event := profilerFilemapTestEvent(1000, nil)
			_, _, ok, issues, handled, err := renderProfilerFtraceFilemapEventWithTypedAudit(event)
			if !handled || ok || len(issues) != 0 || profilerFilemapTestInvariantReason(t, err) != test.reason {
				t.Fatalf("typed producer descriptor verdict drifted: ok=%v handled=%v issues=%+v err=%v",
					ok, handled, issues, err)
			}
			payload := filemapRenderPayload{Kind: filemapRenderPageAdd, Name: validDescriptor.Name}
			_, _, ok, issues, err = finalizeProfilerFtraceFilemapEventWithTypedAudit(
				event, payload, bodyAdmitted, profilerFtraceFilemapIssueSet{})
			if ok || len(issues) != 0 || profilerFilemapTestInvariantReason(t, err) != test.reason {
				t.Fatalf("typed finalizer descriptor verdict drifted: ok=%v issues=%+v err=%v", ok, issues, err)
			}
			profilerFtraceEventDescriptors[1000] = originalDescriptor
		})
	}

	event := profilerFilemapTestEvent(1000, nil)
	validPayload := filemapRenderPayload{Kind: filemapRenderPageAdd, Name: "mm_filemap_add_to_page_cache"}
	if name, body, ok, issues, err := finalizeProfilerFtraceFilemapEventWithTypedAudit(
		event, validPayload, bodyAdmitted, profilerFtraceFilemapIssueSet{}); err != nil ||
		!ok || name != validPayload.Name || body == "" || len(issues) != 0 {
		t.Fatalf("valid finalizer rejected: name=%q body=%q ok=%v issues=%+v err=%v", name, body, ok, issues, err)
	}
	corruptPayload := validPayload
	corruptPayload.Index = uint64(math.MaxInt64>>12) + 1
	_, _, _, issues, err := finalizeProfilerFtraceFilemapEventWithTypedAudit(
		event, corruptPayload, bodyAdmitted, profilerFtraceFilemapIssueSet{})
	if len(issues) != 0 || profilerFilemapTestInvariantReason(t, err) != "invalid_canonical_filemap_payload" {
		t.Fatalf("canonical payload corruption became source issue: issues=%+v err=%v", issues, err)
	}
	lineEvent := event
	lineEvent.TSNS = math.MaxUint64
	_, _, _, issues, err = finalizeProfilerFtraceFilemapEventWithTypedAudit(
		lineEvent, validPayload, bodyAdmitted, profilerFtraceFilemapIssueSet{})
	if len(issues) != 0 || profilerFilemapTestInvariantReason(t, err) != "invalid_canonical_filemap_line" {
		t.Fatalf("canonical line corruption became source issue: issues=%+v err=%v", issues, err)
	}

	hard := profilerFtraceFilemapIssueSet{}
	if err := hard.addFixed(1000, profilerFtraceEventIssueFilemapPFNInvalid); err != nil {
		t.Fatal(err)
	}
	_, _, ok, issues, err := finalizeProfilerFtraceFilemapEventWithTypedAudit(
		event, filemapRenderPayload{}, bodyRejected, hard)
	if err != nil || ok || len(issues) != 1 || issues[0] != hard.Issues[0] {
		t.Fatalf("valid rejected verdict drifted: ok=%v issues=%+v err=%v", ok, issues, err)
	}
	_, _, _, _, err = finalizeProfilerFtraceFilemapEventWithTypedAudit(event, validPayload, bodyRejected, hard)
	if reason := profilerFilemapTestInvariantReason(t, err); reason != "profiler_filemap_rejected_verdict_invalid" {
		t.Fatalf("rejected verdict reason=%q", reason)
	}
	_, _, _, _, err = finalizeProfilerFtraceFilemapEventWithTypedAudit(event, validPayload, bodyAdmitted, hard)
	if reason := profilerFilemapTestInvariantReason(t, err); reason != "profiler_filemap_admitted_verdict_invalid" {
		t.Fatalf("admitted verdict reason=%q", reason)
	}
	badSet := profilerFtraceFilemapIssueSet{Count: 2}
	_, _, _, issues, err = finalizeProfilerFtraceFilemapEventWithTypedAudit(event, corruptPayload, bodyAdmitted, badSet)
	if len(issues) != 0 || profilerFilemapTestInvariantReason(t, err) != "profiler_filemap_issue_count_invalid" {
		t.Fatalf("set corruption did not dominate canonical corruption: issues=%+v err=%v", issues, err)
	}
}

func TestProfilerFilemapTypedUnsupportedAndSingleWalkStructure(t *testing.T) {
	if _, _, _, issues, handled, err := renderProfilerFtraceFilemapEventWithTypedAudit(
		profilerFilemapTestEvent(1002, nil)); err != nil || handled || len(issues) != 0 {
		t.Fatalf("unsupported filemap event handled=%v issues=%+v err=%v", handled, issues, err)
	}
	source, err := os.ReadFile("filemap_render.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	decodeCompatStart := strings.Index(text, "func decodeProfilerFilemapPayloadWithTypedAudit(")
	decodeStart := strings.Index(text, "func decodeProfilerFilemapPayloadWithTypedAuditContext(")
	renderCompatStart := strings.Index(text, "func renderProfilerFtraceFilemapEventWithTypedAudit(")
	renderStart := strings.Index(text, "func renderProfilerFtraceFilemapEventWithTypedAuditContext(")
	finalizeStart := strings.Index(text, "func finalizeProfilerFtraceFilemapEventWithTypedAudit(")
	adapterStart := strings.Index(text, "func decodeProfilerFilemapPayload(event")
	if decodeCompatStart < 0 || decodeStart <= decodeCompatStart || renderCompatStart <= decodeStart ||
		renderStart <= renderCompatStart || finalizeStart <= renderStart || adapterStart <= finalizeStart {
		t.Fatal("filemap typed producer boundaries missing")
	}
	decodeCompat := text[decodeCompatStart:decodeStart]
	decode := text[decodeStart:renderCompatStart]
	renderCompat := text[renderCompatStart:renderStart]
	render := text[renderStart:finalizeStart]
	adapter := text[adapterStart:]
	if strings.Count(decodeCompat, "decodeProfilerFilemapPayloadWithTypedAuditContext(context.Background(), event)") != 1 {
		t.Fatal("filemap decode compatibility entry is not a Background-only Context adapter")
	}
	if strings.Count(decode, "walkProfilerProtoFieldsContext(ctx, event.Payload") != 1 ||
		strings.Contains(decode, "walkProtoFields(event.Payload") ||
		!strings.Contains(decode, "var fields [6]profilerFilemapProtoField") ||
		strings.Contains(decode, "protoScalarUint(") {
		t.Fatal("filemap typed producer is not one fixed-[6] wire walk")
	}
	if strings.Count(renderCompat, "renderProfilerFtraceFilemapEventWithTypedAuditContext(context.Background(), event)") != 1 {
		t.Fatal("filemap render compatibility entry is not a Background-only Context adapter")
	}
	if strings.Count(render, "decodeProfilerFilemapPayloadWithTypedAuditContext(ctx, event)") != 1 ||
		!strings.Contains(render, "finalizeProfilerFtraceFilemapEventWithTypedAuditContext(") ||
		strings.Contains(render, "decodeProfilerFilemapPayloadWithTypedAudit(event)") ||
		strings.Contains(render, "finalizeProfilerFtraceFilemapEventWithTypedAudit(event") {
		t.Fatal("filemap typed renderer is not the sole parse/finalize authority")
	}
	if strings.Count(adapter, "decodeProfilerFilemapPayloadWithTypedAudit(event)") != 1 ||
		strings.Contains(adapter, "walkProtoFields(") || strings.Contains(adapter, "protoScalarUint(") {
		t.Fatal("filemap compatibility adapter regained parsing authority")
	}
	if strings.Contains(text, "FilemapInvalidCanonicalLine") {
		t.Fatal("source-unreachable filemap canonical issue returned to producer")
	}

	entrySource, err := os.ReadFile("profiler_ftrace_render.go")
	if err != nil {
		t.Fatal(err)
	}
	entry := string(entrySource)
	compatStart := strings.Index(entry, "func renderProfilerFtraceEventBodyWithAudit(")
	typedStart := strings.Index(entry, "func renderProfilerFtraceEventBodyWithTypedAuditAndPairContext(")
	genericStart := strings.Index(entry, "const profilerFtraceGenericIssuesPerEvent")
	if compatStart < 0 || typedStart <= compatStart || genericStart <= typedStart {
		t.Fatal("profiler typed/compat entry boundaries missing")
	}
	compatEntry := entry[compatStart:typedStart]
	typedEntry := entry[typedStart:genericStart]
	if strings.Count(compatEntry, "renderProfilerFtraceEventBodyWithTypedAudit(event)") != 1 ||
		strings.Count(compatEntry, "profilerFtraceEventIssueLabels(event.Field, issues)") != 1 ||
		strings.Contains(compatEntry, "renderProfilerFtraceFilemapEventWithTypedAudit(event)") {
		t.Fatal("compat entry is not one typed-call-to-label adapter")
	}
	filemapAt := strings.Index(typedEntry, "renderProfilerFtraceFilemapEventWithTypedAuditContext(ctx, event)")
	genericAt := strings.Index(typedEntry, "renderProfilerFtraceGenericEventWithTypedAuditContext(ctx, event)")
	if strings.Count(typedEntry, "renderProfilerFtraceFilemapEventWithTypedAuditContext(ctx, event)") != 1 ||
		filemapAt < 0 || genericAt < 0 || filemapAt >= genericAt {
		t.Fatal("typed entry lost direct filemap typed precedence")
	}
	if strings.Contains(typedEntry[:genericAt], "decodeProfilerFilemapPayload(event)") ||
		strings.Contains(typedEntry[:genericAt], "renderProfilerFtraceFilemapEventWithTypedAudit(event)") {
		t.Fatal("typed entry regained legacy filemap parser authority")
	}
	legacyAt := strings.Index(typedEntry, "renderProfilerFtraceEventBodyWithAudit(event)")
	if legacyAt >= 0 ||
		strings.Contains(typedEntry, "profilerFtraceEventDegradationFilemapPayload") {
		t.Fatal("typed filemap arm returned to the reverse bridge")
	}
}
