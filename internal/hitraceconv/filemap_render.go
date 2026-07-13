package hitraceconv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// filemapRenderPayload is the sole value authority shared by direct RMQ,
// structured profiler and SQL-compatibility filemap rows. Page-cache events
// are instantaneous mutations, and writeback-error events are observations;
// no field in this payload is a duration endpoint.
type filemapRenderPayload struct {
	Kind         filemapRenderKind
	Name         string
	Dev          uint32
	Inode        uint64
	PFN          uint64
	Index        uint64
	Order        uint8
	OrderPresent bool
	File         uint64
	ErrSeq       uint32
	Old          uint32
	New          uint32
}

type filemapRenderKind uint8

const (
	filemapRenderUnknown filemapRenderKind = iota
	filemapRenderPageAdd
	filemapRenderPageDelete
	filemapRenderWritebackSet
	filemapRenderWritebackAdvance
)

type directFilemapFieldSpec struct {
	Name   string
	Type   string
	Offset int
	Size   int
	Signed bool
}

func directFilemapNameGoverned(name string) bool {
	_, ok := filemapRenderKindForName(name)
	return ok
}

func traceDBFilemapNameGoverned(name string) bool {
	switch name {
	case "mm_filemap_add_to_page_cache", "mm_filemap_delete_from_page_cache", "filemap_set_wb_err":
		return true
	default:
		return false
	}
}

func filemapRenderKindForName(name string) (filemapRenderKind, bool) {
	switch name {
	case "mm_filemap_add_to_page_cache":
		return filemapRenderPageAdd, true
	case "mm_filemap_delete_from_page_cache":
		return filemapRenderPageDelete, true
	case "filemap_set_wb_err":
		return filemapRenderWritebackSet, true
	case "file_check_and_advance_wb_err":
		return filemapRenderWritebackAdvance, true
	default:
		return filemapRenderUnknown, false
	}
}

func renderCanonicalFilemapPayload(payload filemapRenderPayload) (string, bool) {
	kind, governed := filemapRenderKindForName(payload.Name)
	if !governed || kind != payload.Kind {
		return "", false
	}
	dev := devMajorMinor(int64(payload.Dev), ":")
	switch kind {
	case filemapRenderPageAdd, filemapRenderPageDelete:
		if payload.Index > uint64(math.MaxInt64>>12) {
			return "", false
		}
		return fmt.Sprintf("dev %s ino 0x%x pfn=%d ofs=%d", dev, payload.Inode,
			payload.PFN, payload.Index<<12), true
	case filemapRenderWritebackSet:
		return fmt.Sprintf("dev=%s ino=0x%x errseq=0x%x", dev, payload.Inode, payload.ErrSeq), true
	case filemapRenderWritebackAdvance:
		if payload.File == 0 {
			return "", false
		}
		return fmt.Sprintf("file=0x%x dev=%s ino=0x%x old=0x%x new=0x%x", payload.File,
			dev, payload.Inode, payload.Old, payload.New), true
	default:
		return "", false
	}
}

func decodeDirectFilemapPayload(ev decodedEvent) (filemapRenderPayload, bodyAdmission, string) {
	kind, governed := filemapRenderKindForName(ev.format.Name)
	if !governed {
		return filemapRenderPayload{}, bodyUnsupported, ""
	}
	payload := filemapRenderPayload{Kind: kind, Name: ev.format.Name}
	fields, ok := directFilemapProfileFields(ev, kind)
	if !ok {
		return payload, bodyRejected, "missing_or_invalid_filemap_profile"
	}

	read := func(name string) uint64 {
		return directFilemapUint(fields[name])
	}
	switch kind {
	case filemapRenderPageAdd, filemapRenderPageDelete:
		payload.PFN = read("pfn")
		payload.Inode = read("i_ino")
		payload.Index = read("index")
		payload.Dev = uint32(read("s_dev"))
		if raw, present := fields["order"]; present {
			payload.Order = raw[0]
			payload.OrderPresent = true
		}
		if payload.Index > uint64(math.MaxInt64>>12) {
			return payload, bodyRejected, "filemap_index_out_of_range"
		}
	case filemapRenderWritebackSet:
		payload.Inode = read("i_ino")
		payload.Dev = uint32(read("s_dev"))
		payload.ErrSeq = uint32(read("errseq"))
	case filemapRenderWritebackAdvance:
		payload.File = read("file")
		payload.Inode = read("i_ino")
		payload.Dev = uint32(read("s_dev"))
		payload.Old = uint32(read("old"))
		payload.New = uint32(read("new"))
		if payload.File == 0 {
			return payload, bodyRejected, "filemap_file_pointer_zero"
		}
	default:
		return payload, bodyRejected, "invalid_filemap_kind"
	}
	return payload, bodyAdmitted, ""
}

func directFilemapProfileFields(ev decodedEvent, kind filemapRenderKind) (map[string][]byte, bool) {
	profiles := directFilemapProfiles(kind)
	var matched map[string][]byte
	for _, profile := range profiles {
		fields, ok := directFilemapExactFields(ev, profile)
		if !ok {
			continue
		}
		if matched != nil {
			return nil, false
		}
		matched = fields
	}
	return matched, matched != nil
}

func directFilemapProfiles(kind filemapRenderKind) [][]directFilemapFieldSpec {
	var profiles [][]directFilemapFieldSpec
	for _, wordSize := range []int{4, 8} {
		switch kind {
		case filemapRenderPageAdd, filemapRenderPageDelete:
			base := []directFilemapFieldSpec{
				{Name: "pfn", Type: "unsigned long", Offset: 8, Size: wordSize},
				{Name: "i_ino", Type: "unsigned long", Offset: 8 + wordSize, Size: wordSize},
				{Name: "index", Type: "unsigned long", Offset: 8 + 2*wordSize, Size: wordSize},
				{Name: "s_dev", Type: "dev_t", Offset: 8 + 3*wordSize, Size: 4},
			}
			profiles = append(profiles, base)
			modern := append([]directFilemapFieldSpec(nil), base...)
			modern = append(modern, directFilemapFieldSpec{
				Name: "order", Type: "unsigned char", Offset: 12 + 3*wordSize, Size: 1,
			})
			profiles = append(profiles, modern)
		case filemapRenderWritebackSet:
			profiles = append(profiles, []directFilemapFieldSpec{
				{Name: "i_ino", Type: "unsigned long", Offset: 8, Size: wordSize},
				{Name: "s_dev", Type: "dev_t", Offset: 8 + wordSize, Size: 4},
				{Name: "errseq", Type: "errseq_t", Offset: 12 + wordSize, Size: 4},
			})
		case filemapRenderWritebackAdvance:
			profiles = append(profiles, []directFilemapFieldSpec{
				{Name: "file", Type: "struct file *", Offset: 8, Size: wordSize},
				{Name: "i_ino", Type: "unsigned long", Offset: 8 + wordSize, Size: wordSize},
				{Name: "s_dev", Type: "dev_t", Offset: 8 + 2*wordSize, Size: 4},
				{Name: "old", Type: "errseq_t", Offset: 12 + 2*wordSize, Size: 4},
				{Name: "new", Type: "errseq_t", Offset: 16 + 2*wordSize, Size: 4},
			})
		}
	}
	return profiles
}

func directFilemapExactFields(ev decodedEvent, specs []directFilemapFieldSpec) (map[string][]byte, bool) {
	if len(specs) == 0 {
		return nil, false
	}
	expected := make(map[string]directFilemapFieldSpec, len(specs))
	for _, spec := range specs {
		expected[spec.Name] = spec
	}
	values := make(map[string][]byte, len(specs))
	common := make(map[string]bool, 4)
	for index, field := range ev.format.Fields {
		name := cleanFieldName(field.Name)
		if strings.HasPrefix(name, "common_") {
			spec, found := directFilemapCommonFieldSpec(name)
			if !found || common[name] || field.Name != spec.Name || field.Signed != spec.Signed ||
				normalizeFieldType(field.Type) != spec.Type || field.Offset != spec.Offset ||
				field.Size != spec.Size || !directDescriptorFieldIsolated(ev, index) {
				return nil, false
			}
			common[name] = true
			continue
		}
		spec, found := expected[name]
		if !found || values[name] != nil || field.Name != spec.Name || field.Signed != spec.Signed ||
			normalizeFieldType(field.Type) != spec.Type || field.Offset != spec.Offset ||
			field.Size != spec.Size || !directDescriptorFieldIsolated(ev, index) {
			return nil, false
		}
		raw, present := ev.fields[field.Name]
		if !present || len(raw) != field.Size {
			return nil, false
		}
		values[name] = raw
	}
	if len(common) != 4 || len(values) != len(specs) {
		return nil, false
	}
	return values, true
}

func directFilemapCommonFieldSpec(name string) (directFilemapFieldSpec, bool) {
	switch name {
	case "common_type":
		return directFilemapFieldSpec{Name: name, Type: "unsigned short", Offset: 0, Size: 2}, true
	case "common_flags":
		return directFilemapFieldSpec{Name: name, Type: "unsigned char", Offset: 2, Size: 1}, true
	case "common_preempt_count":
		return directFilemapFieldSpec{Name: name, Type: "unsigned char", Offset: 3, Size: 1}, true
	case "common_pid":
		return directFilemapFieldSpec{Name: name, Type: "int", Offset: 4, Size: 4, Signed: true}, true
	default:
		return directFilemapFieldSpec{}, false
	}
}

func directFilemapUint(raw []byte) uint64 {
	switch len(raw) {
	case 1:
		return uint64(raw[0])
	case 4:
		return uint64(binary.LittleEndian.Uint32(raw))
	case 8:
		return binary.LittleEndian.Uint64(raw)
	default:
		return 0
	}
}

func decodeTraceDBFilemapPayload(name string, args map[string]traceDBValue,
	invalidKeys map[string]bool,
) (filemapRenderPayload, bool) {
	if !traceDBFilemapNameGoverned(name) {
		return filemapRenderPayload{}, false
	}
	kind, _ := filemapRenderKindForName(name)
	payload := filemapRenderPayload{Kind: kind, Name: name}
	allowed := map[string]bool{}
	switch kind {
	case filemapRenderPageAdd, filemapRenderPageDelete:
		for _, key := range []string{"s_dev", "i_ino", "index", "pfn", "order"} {
			allowed[key] = true
		}
	case filemapRenderWritebackSet:
		for _, key := range []string{"s_dev", "i_ino", "errseq"} {
			allowed[key] = true
		}
	default:
		return payload, false
	}
	for key := range args {
		if !allowed[key] {
			return payload, false
		}
	}
	for key, invalid := range invalidKeys {
		if invalid || !allowed[key] {
			return payload, false
		}
	}
	read := func(key string, max uint64, required bool) (uint64, bool, bool) {
		value, present := args[key]
		if !present {
			return 0, false, !required
		}
		if !value.Valid || value.Datatype != 0 || value.Text == "" || value.Text != strings.TrimSpace(value.Text) {
			return 0, true, false
		}
		parsed, err := strconv.ParseUint(value.Text, 10, 64)
		if err != nil || parsed > max || strconv.FormatUint(parsed, 10) != value.Text {
			return 0, true, false
		}
		return parsed, true, true
	}
	dev, _, ok := read("s_dev", math.MaxUint32, true)
	if !ok {
		return payload, false
	}
	inode, _, ok := read("i_ino", math.MaxInt64, true)
	if !ok {
		return payload, false
	}
	payload.Dev, payload.Inode = uint32(dev), inode
	switch kind {
	case filemapRenderPageAdd, filemapRenderPageDelete:
		index, _, valid := read("index", uint64(math.MaxInt64>>12), true)
		if !valid {
			return payload, false
		}
		pfn, _, valid := read("pfn", math.MaxInt64, true)
		if !valid {
			return payload, false
		}
		order, present, valid := read("order", math.MaxUint8, false)
		if !valid {
			return payload, false
		}
		payload.Index, payload.PFN = index, pfn
		payload.Order, payload.OrderPresent = uint8(order), present
	case filemapRenderWritebackSet:
		errseq, _, valid := read("errseq", math.MaxUint32, true)
		if !valid {
			return payload, false
		}
		payload.ErrSeq = uint32(errseq)
	}
	return payload, true
}

const profilerFtraceFilemapIssuesPerEvent = 1

// profilerFtraceFilemapIssueSet preserves the filemap producer contract: one
// malformed page-cache event publishes exactly one dominant hard diagnostic.
// Keeping this a fixed checked container makes capacity and zero-tail
// invariants independent of slices supplied by callers or future adapters.
type profilerFtraceFilemapIssueSet struct {
	Count  uint8
	Issues [profilerFtraceFilemapIssuesPerEvent]profilerFtraceEventIssue
}

func (set *profilerFtraceFilemapIssueSet) validate(eventField int) error {
	if set == nil || int(set.Count) > len(set.Issues) {
		return &traceDBOutputInvariantError{Reason: "profiler_filemap_issue_count_invalid"}
	}
	if eventField != 1000 && eventField != 1001 {
		return &traceDBOutputInvariantError{Reason: "profiler_filemap_issue_schema_invalid"}
	}
	for index, issue := range set.Issues {
		if index >= int(set.Count) {
			if issue != (profilerFtraceEventIssue{}) {
				return &traceDBOutputInvariantError{Reason: "profiler_filemap_issue_count_invalid"}
			}
			continue
		}
		if issue.Kind < profilerFtraceEventIssueFilemapPayloadMalformedWire ||
			issue.Kind > profilerFtraceEventIssueFilemapOrderInvalid ||
			!issue.validFor(eventField) || issue.Severity != profilerFtraceEventIssueHardReject ||
			issue.Severity != issue.expectedSeverity() {
			return &traceDBOutputInvariantError{Reason: "profiler_filemap_issue_schema_invalid"}
		}
	}
	return nil
}

func (set *profilerFtraceFilemapIssueSet) add(eventField int, issue profilerFtraceEventIssue) error {
	if err := set.validate(eventField); err != nil {
		return err
	}
	if issue.Kind < profilerFtraceEventIssueFilemapPayloadMalformedWire ||
		issue.Kind > profilerFtraceEventIssueFilemapOrderInvalid ||
		!issue.validFor(eventField) || issue.Severity != profilerFtraceEventIssueHardReject ||
		issue.Severity != issue.expectedSeverity() {
		return &traceDBOutputInvariantError{Reason: "profiler_filemap_issue_schema_invalid"}
	}
	for index := 0; index < int(set.Count); index++ {
		if set.Issues[index] == issue {
			return &traceDBOutputInvariantError{Reason: "profiler_filemap_issue_duplicate"}
		}
	}
	if int(set.Count) == len(set.Issues) {
		return &traceDBOutputInvariantError{Reason: "profiler_filemap_issue_overflow"}
	}
	candidate := *set
	candidate.Issues[int(candidate.Count)] = issue
	candidate.Count++
	if err := candidate.validate(eventField); err != nil {
		return err
	}
	*set = candidate
	return nil
}

func (set *profilerFtraceFilemapIssueSet) addFixed(eventField int, kind profilerFtraceEventIssueKind) error {
	issue, ok := profilerFtraceEventFixedIssue(eventField, kind)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_filemap_issue_schema_invalid"}
	}
	return set.add(eventField, issue)
}

func (set *profilerFtraceFilemapIssueSet) checked(eventField int) ([]profilerFtraceEventIssue, error) {
	if err := set.validate(eventField); err != nil {
		return nil, err
	}
	return append([]profilerFtraceEventIssue(nil), set.Issues[:int(set.Count)]...), nil
}

type profilerFilemapProtoField struct {
	Count     uint8 // saturates at two: absent, singular, or duplicate
	WrongWire bool
	Malformed bool
	UintValue uint64
}

func profilerFilemapExpectedDescriptor(eventField int) (string, filemapRenderKind, bool) {
	switch eventField {
	case 1000:
		return "mm_filemap_add_to_page_cache", filemapRenderPageAdd, true
	case 1001:
		return "mm_filemap_delete_from_page_cache", filemapRenderPageDelete, true
	default:
		return "", filemapRenderUnknown, false
	}
}

// validateProfilerFilemapDescriptor keeps the generated event descriptor an
// internal invariant. Raw protobuf bytes can select neither a different event
// name nor a different page-cache semantic kind.
func validateProfilerFilemapDescriptor(eventField int, descriptor profilerFtraceEventDescriptor, present bool) (string, filemapRenderKind, error) {
	expectedName, expectedKind, governed := profilerFilemapExpectedDescriptor(eventField)
	if !governed {
		return "", filemapRenderUnknown,
			&traceDBOutputInvariantError{Reason: "profiler_filemap_descriptor_domain_invalid"}
	}
	if !present {
		return "", filemapRenderUnknown,
			&traceDBOutputInvariantError{Reason: "missing_filemap_descriptor"}
	}
	if descriptor.Field != eventField {
		return "", filemapRenderUnknown,
			&traceDBOutputInvariantError{Reason: "mismatched_filemap_descriptor_field"}
	}
	if descriptor.Family != "filemap" {
		return "", filemapRenderUnknown,
			&traceDBOutputInvariantError{Reason: "mismatched_filemap_descriptor_family"}
	}
	if descriptor.Name != expectedName {
		return "", filemapRenderUnknown,
			&traceDBOutputInvariantError{Reason: "mismatched_filemap_descriptor_name"}
	}
	kind, known := filemapRenderKindForName(descriptor.Name)
	if !known || kind != expectedKind {
		return "", filemapRenderUnknown,
			&traceDBOutputInvariantError{Reason: "invalid_filemap_descriptor_kind"}
	}
	return descriptor.Name, kind, nil
}

func profilerFilemapIssueKindForField(payloadField int) (profilerFtraceEventIssueKind, bool) {
	switch payloadField {
	case 1:
		return profilerFtraceEventIssueFilemapPFNInvalid, true
	case 2:
		return profilerFtraceEventIssueFilemapInodeInvalid, true
	case 3:
		return profilerFtraceEventIssueFilemapIndexInvalid, true
	case 4:
		return profilerFtraceEventIssueFilemapDeviceInvalid, true
	case 5:
		return profilerFtraceEventIssueFilemapOrderInvalid, true
	default:
		return profilerFtraceEventIssueKindCount, false
	}
}

// decodeProfilerFilemapPayloadWithTypedAudit is the single structured
// filemap parser. All five scalar endpoints are observed in one wire walk;
// diagnostics are selected afterward in schema order, independent of source
// order. A malformed known endpoint remains local, while a malformed key or
// unknown endpoint rejects the whole payload because no hard field identity
// can be proven for it.
func decodeProfilerFilemapPayloadWithTypedAudit(event profilerFtraceEventRecord) (
	filemapRenderPayload, bodyAdmission, profilerFtraceFilemapIssueSet, bool, error,
) {
	if _, _, governed := profilerFilemapExpectedDescriptor(event.Field); !governed {
		return filemapRenderPayload{}, bodyUnsupported, profilerFtraceFilemapIssueSet{}, false, nil
	}
	descriptor, present := profilerFtraceEventDescriptors[event.Field]
	name, kind, descriptorErr := validateProfilerFilemapDescriptor(event.Field, descriptor, present)
	if descriptorErr != nil {
		return filemapRenderPayload{}, bodyRejected, profilerFtraceFilemapIssueSet{}, true, descriptorErr
	}
	reject := func(issueKind profilerFtraceEventIssueKind) (
		filemapRenderPayload, bodyAdmission, profilerFtraceFilemapIssueSet, bool, error,
	) {
		var set profilerFtraceFilemapIssueSet
		err := set.addFixed(event.Field, issueKind)
		return filemapRenderPayload{}, bodyRejected, set, true, err
	}

	var fields [6]profilerFilemapProtoField
	walkErr := walkProtoFields(event.Payload, func(payloadField int, wire int, _ []byte, value uint64) error {
		if payloadField < 1 || payloadField > 5 {
			return nil
		}
		field := &fields[payloadField]
		if field.Count < 2 {
			field.Count++
		}
		if wire != 0 {
			field.WrongWire = true
			return nil
		}
		field.UintValue = value
		return nil
	})
	if walkErr != nil {
		var decodeErr *protoFieldDecodeError
		if !errors.As(walkErr, &decodeErr) {
			return filemapRenderPayload{}, bodyRejected, profilerFtraceFilemapIssueSet{}, true,
				&traceDBOutputInvariantError{Reason: "profiler_filemap_wire_error_untyped"}
		}
		_, knownEndpoint := profilerFilemapIssueKindForField(decodeErr.Field)
		localized := decodeErr.FieldKnown && knownEndpoint &&
			(decodeErr.Failure == protoFieldDecodeMalformedValue ||
				decodeErr.Failure == protoFieldDecodeUnsupportedWire)
		if localized {
			// Preserve schema-first precedence over complete hard failures
			// already observed before this structural endpoint. Fields beyond
			// the malformed boundary remain unknown and mint no facts.
			fields[decodeErr.Field].Malformed = true
		} else {
			return reject(profilerFtraceEventIssueFilemapPayloadMalformedWire)
		}
	}

	// Completed-scan structural failures dominate all numeric range checks.
	for payloadField := 1; payloadField <= 5; payloadField++ {
		field := fields[payloadField]
		if field.Malformed || field.WrongWire || field.Count > 1 {
			issueKind, _ := profilerFilemapIssueKindForField(payloadField)
			return reject(issueKind)
		}
	}
	limits := [6]uint64{
		0,
		math.MaxUint64,
		math.MaxUint64,
		uint64(math.MaxInt64 >> 12),
		math.MaxUint32,
		math.MaxUint8,
	}
	for payloadField := 1; payloadField <= 5; payloadField++ {
		if fields[payloadField].UintValue > limits[payloadField] {
			issueKind, _ := profilerFilemapIssueKindForField(payloadField)
			return reject(issueKind)
		}
	}

	payload := filemapRenderPayload{
		Kind: kind, Name: name,
		PFN: fields[1].UintValue, Inode: fields[2].UintValue,
		Index: fields[3].UintValue, Dev: uint32(fields[4].UintValue),
	}
	if fields[5].Count == 1 {
		payload.Order, payload.OrderPresent = uint8(fields[5].UintValue), true
	}
	return payload, bodyAdmitted, profilerFtraceFilemapIssueSet{}, true, nil
}

// renderProfilerFtraceFilemapEventWithTypedAudit is the single typed
// parse/render authority for structured page-cache events. Canonical render
// failures are internal invariants: every source-controlled value has already
// been reduced to a bounded numeric payload.
func renderProfilerFtraceFilemapEventWithTypedAudit(event profilerFtraceEventRecord) (
	name, body string, ok bool, issues []profilerFtraceEventIssue, handled bool, err error,
) {
	payload, admission, set, handled, err := decodeProfilerFilemapPayloadWithTypedAudit(event)
	if !handled || err != nil {
		return "", "", false, nil, handled, err
	}
	name, body, ok, issues, err = finalizeProfilerFtraceFilemapEventWithTypedAudit(event, payload, admission, set)
	return name, body, ok, issues, true, err
}

func finalizeProfilerFtraceFilemapEventWithTypedAudit(
	event profilerFtraceEventRecord,
	payload filemapRenderPayload,
	admission bodyAdmission,
	set profilerFtraceFilemapIssueSet,
) (name, body string, ok bool, issues []profilerFtraceEventIssue, err error) {
	// Set corruption dominates source verdicts and canonical rendering.
	issues, err = set.checked(event.Field)
	if err != nil {
		return "", "", false, nil, err
	}
	descriptor, present := profilerFtraceEventDescriptors[event.Field]
	expectedName, expectedKind, descriptorErr := validateProfilerFilemapDescriptor(event.Field, descriptor, present)
	if descriptorErr != nil {
		return "", "", false, nil, descriptorErr
	}
	switch admission {
	case bodyRejected:
		if payload != (filemapRenderPayload{}) || len(issues) != 1 ||
			issues[0].Severity != profilerFtraceEventIssueHardReject {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "profiler_filemap_rejected_verdict_invalid"}
		}
		return "", "", false, issues, nil
	case bodyAdmitted:
		if len(issues) != 0 || payload.Name != expectedName || payload.Kind != expectedKind {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "profiler_filemap_admitted_verdict_invalid"}
		}
		body, rendered := renderCanonicalFilemapPayload(payload)
		if !rendered {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "invalid_canonical_filemap_payload"}
		}
		if !profilerCanonicalLineValid(event, payload.Name, body) {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "invalid_canonical_filemap_line"}
		}
		return payload.Name, body, true, nil, nil
	default:
		return "", "", false, nil,
			&traceDBOutputInvariantError{Reason: "profiler_filemap_admission_invalid"}
	}
}

// decodeProfilerFilemapPayload is the legacy compatibility adapter. It only
// projects the typed verdict to the historical single-label surface; parsing
// and issue selection remain exclusively in the typed producer above.
func decodeProfilerFilemapPayload(event profilerFtraceEventRecord) (filemapRenderPayload, bodyAdmission, string) {
	payload, admission, set, handled, err := decodeProfilerFilemapPayloadWithTypedAudit(event)
	if !handled {
		return filemapRenderPayload{}, bodyUnsupported, ""
	}
	if err != nil {
		if reason, ok := traceDBOutputInvariantReason(err); ok {
			return filemapRenderPayload{}, bodyRejected, reason
		}
		return filemapRenderPayload{}, bodyRejected, "profiler_filemap_typed_audit_failed"
	}
	issues, err := set.checked(event.Field)
	if err != nil {
		return filemapRenderPayload{}, bodyRejected, "profiler_filemap_typed_issue_invalid"
	}
	labels, labelsOK := profilerFtraceEventIssueLabels(event.Field, issues)
	if !labelsOK {
		return filemapRenderPayload{}, bodyRejected, "profiler_filemap_typed_issue_invalid"
	}
	switch admission {
	case bodyRejected:
		if payload != (filemapRenderPayload{}) || len(labels) != 1 ||
			len(issues) != 1 || issues[0].Severity != profilerFtraceEventIssueHardReject {
			return filemapRenderPayload{}, bodyRejected, "profiler_filemap_typed_issue_invalid"
		}
		return filemapRenderPayload{}, bodyRejected, labels[0]
	case bodyAdmitted:
		if len(labels) != 0 {
			return filemapRenderPayload{}, bodyRejected, "profiler_filemap_typed_issue_invalid"
		}
		return payload, bodyAdmitted, ""
	default:
		return filemapRenderPayload{}, bodyRejected, "profiler_filemap_typed_admission_invalid"
	}
}
