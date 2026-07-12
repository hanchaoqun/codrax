package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// bodyAdmission separates an unsupported renderer surface from a governed
// event whose physical payload failed admission. Only unsupported events may
// retain the legacy header-only compatibility row.
type bodyAdmission uint8

const (
	bodyUnsupported bodyAdmission = iota
	bodyAdmitted
	bodyRejected
)

type coreRenderKind uint8

const (
	coreRenderUnknown coreRenderKind = iota
	coreRenderWakeup
	coreRenderBlocked
	coreRenderCPU
	coreRenderBinder
	coreRenderInterrupt
)

type coreRenderPayload struct {
	Kind      coreRenderKind
	Name      string
	Wakeup    *coreWakeupPayload
	Blocked   *coreBlockedPayload
	CPU       *coreCPUPayload
	Binder    *coreBinderPayload
	Interrupt *coreInterruptPayload
}

type coreDecodeContext struct {
	PrintkFormats  map[uint64]string
	PrintkPoisoned map[uint64]bool
}

type coreWakeupPayload struct {
	Comm      string
	PID       int64
	Priority  int64
	TargetCPU int64
}

type coreBlockedPayload struct {
	PID              int64
	IOWait           uint64
	CallerRaw        uint64
	Caller           string
	CallerSymbolized bool
	CNodeIndex       uint64
	CNodeKnown       bool
	Delay            uint64
	DelayKnown       bool
}

type coreCPUPayload struct {
	State    uint64
	Min      uint64
	Max      uint64
	CPUID    uint64
	IsLimits bool
}

type coreBinderPayload struct {
	Transaction int64
	DestNode    int64
	DestProc    int64
	DestThread  int64
	Reply       int64
	Flags       uint64
	Code        uint64
	Received    bool
}

type coreInterruptPayload struct {
	IRQ        int64
	IRQName    string
	Ret        int64
	Vec        uint64
	TargetMask uint64
	Reason     string
}

func coreRenderKindForName(name string) (coreRenderKind, bool) {
	switch name {
	case "sched_wakeup", "sched_wakeup_new", "sched_waking":
		return coreRenderWakeup, true
	case "sched_blocked_reason":
		return coreRenderBlocked, true
	case "cpu_frequency", "cpu_frequency_limits", "cpu_idle":
		return coreRenderCPU, true
	case "binder_transaction", "binder_transaction_received":
		return coreRenderBinder, true
	case "irq_handler_entry", "irq_handler_exit",
		"softirq_entry", "softirq_exit", "softirq_raise",
		"ipi_entry", "ipi_exit", "ipi_raise":
		return coreRenderInterrupt, true
	default:
		return coreRenderUnknown, false
	}
}

// directCoreDescriptorLayoutValid prevents one physical byte range from
// becoming two independent typed facts. It checks the common envelope and the
// governed family's hard fields; display-only wake names and blocked symbols
// are included only when deciding whether those optional labels are safe to
// publish, so corrupt display metadata cannot erase an otherwise exact edge.
func directCoreDescriptorLayoutValid(ev decodedEvent, kind coreRenderKind, includeDisplay bool) bool {
	selected := make([]int, 0, len(ev.format.Fields))
	selectedSet := make(map[int]bool, len(ev.format.Fields))
	hasCommon := false
	for index, field := range ev.format.Fields {
		name := directCoreFieldBaseName(field.Name)
		if directCoreCommonFieldName(name) {
			hasCommon = true
			selected = append(selected, index)
			selectedSet[index] = true
			continue
		}
		if directCoreKindFieldName(kind, name, includeDisplay) {
			selected = append(selected, index)
			selectedSet[index] = true
		}
	}
	maxInt := int(^uint(0) >> 1)
	for position, fieldIndex := range selected {
		field := ev.format.Fields[fieldIndex]
		if field.Offset < 0 || field.Size <= 0 || field.Offset > maxInt-field.Size {
			return false
		}
		end := field.Offset + field.Size
		name := directCoreFieldBaseName(field.Name)
		// The RMQ event ID is selected independently from content[0:2]. A
		// descriptor body field must never reuse those bytes as payload.
		if hasCommon && name != "common_type" && field.Offset < 2 && end > 0 {
			return false
		}
		for prior := 0; prior < position; prior++ {
			other := ev.format.Fields[selected[prior]]
			otherEnd := other.Offset + other.Size
			if field.Offset < otherEnd && other.Offset < end {
				return false
			}
		}
		for otherIndex, other := range ev.format.Fields {
			if selectedSet[otherIndex] {
				continue
			}
			otherName := directCoreFieldBaseName(other.Name)
			if !includeDisplay && directCoreKindFieldName(kind, otherName, true) && !directCoreKindFieldName(kind, otherName, false) {
				// A malformed display-only field degrades that label through the
				// includeDisplay pass; it cannot erase an exact hard tuple.
				continue
			}
			if other.Offset < 0 || other.Size <= 0 || other.Offset > maxInt-other.Size {
				continue
			}
			otherEnd := other.Offset + other.Size
			if field.Offset < otherEnd && other.Offset < end {
				return false
			}
		}
	}
	return true
}

func directCoreCommonFieldName(name string) bool {
	switch name {
	case "common_type", "common_flags", "common_preempt_count", "common_pid":
		return true
	default:
		return false
	}
}

func directCoreKindFieldName(kind coreRenderKind, name string, includeDisplay bool) bool {
	switch kind {
	case coreRenderWakeup:
		if includeDisplay && (name == "comm" || name == "pname") {
			return true
		}
		switch name {
		case "pid", "prio", "target_cpu":
			return true
		}
	case coreRenderBlocked:
		if includeDisplay {
			switch name {
			case "func_name", "mod_name", "offset", "size":
				return true
			}
		}
		switch name {
		case "pid", "caller", "io_wait", "iowait", "delay", "cnode_idx":
			return true
		}
	case coreRenderCPU:
		switch name {
		case "state", "cpu_id", "min", "max", "min_freq", "max_freq":
			return true
		}
	case coreRenderBinder:
		switch name {
		case "transaction", "debug_id", "target_node", "dest_node", "to_proc", "dest_proc", "to_thread", "dest_thread", "reply", "flags", "code":
			return true
		}
	case coreRenderInterrupt:
		switch name {
		case "irq", "name", "ret", "vec", "target_cpus", "target_mask", "reason":
			return true
		}
	}
	return false
}

func decodeDirectCorePayload(ctx coreDecodeContext, ev decodedEvent, content []byte) (coreRenderPayload, bodyAdmission, string) {
	kind, governed := coreRenderKindForName(ev.format.Name)
	if !governed {
		return coreRenderPayload{}, bodyUnsupported, ""
	}
	if !directCoreDescriptorLayoutValid(ev, kind, false) {
		return coreRenderPayload{}, bodyRejected, "invalid_descriptor_layout"
	}
	payload := coreRenderPayload{Kind: kind, Name: ev.format.Name}
	switch kind {
	case coreRenderWakeup:
		comm, ok := "", false
		if directCoreDescriptorLayoutValid(ev, kind, true) {
			comm, ok = directCoreString(ev, content, true, false, "comm", "pname")
		}
		if !ok {
			// The target comm is display-only. Keep the typed wakeup edge when
			// its identity/priority/CPU tuple is exact, but never publish unsafe
			// bytes or let a rename change admission.
			comm = "<...>"
		}
		pid, ok := directCoreSigned(ev, directWidths(4), "pid")
		if !ok || pid < 0 || pid > math.MaxInt32 {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_pid"
		}
		priority, ok := directCoreSigned(ev, directWidths(4), "prio")
		if !ok || priority < math.MinInt32 || priority > math.MaxInt32 {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_priority"
		}
		targetCPU, ok := directCoreSigned(ev, directWidths(4), "target_cpu")
		if !ok || !validTraceDBCPUIndex(targetCPU) {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_target_cpu"
		}
		payload.Wakeup = &coreWakeupPayload{Comm: comm, PID: pid, Priority: priority, TargetCPU: targetCPU}
	case coreRenderBlocked:
		blocked, reason := decodeDirectBlockedPayload(ev, content)
		if reason != "" {
			return coreRenderPayload{}, bodyRejected, reason
		}
		payload.Blocked = &blocked
	case coreRenderCPU:
		cpuPayload, reason := decodeDirectCPUPayload(ev)
		if reason != "" {
			return coreRenderPayload{}, bodyRejected, reason
		}
		payload.CPU = &cpuPayload
	case coreRenderBinder:
		binder, reason := decodeDirectBinderPayload(ev)
		if reason != "" {
			return coreRenderPayload{}, bodyRejected, reason
		}
		payload.Binder = &binder
	case coreRenderInterrupt:
		interrupt, reason := decodeDirectInterruptPayload(ctx, ev, content)
		if reason != "" {
			return coreRenderPayload{}, bodyRejected, reason
		}
		payload.Interrupt = &interrupt
	default:
		return coreRenderPayload{}, bodyRejected, "invalid_core_kind"
	}
	return payload, bodyAdmitted, ""
}

func decodeDirectBlockedPayload(ev decodedEvent, content []byte) (coreBlockedPayload, string) {
	pid, ok := directCoreSigned(ev, directWidths(4), "pid")
	if !ok || pid < 0 || pid > math.MaxInt32 {
		return coreBlockedPayload{}, "missing_or_invalid_pid"
	}
	caller, ok := directCoreAddress(ev, directWidths(4, 8), "caller")
	if !ok {
		return coreBlockedPayload{}, "missing_or_invalid_caller"
	}
	ioWait, ok := directCoreUnsigned(ev, directWidths(1, 4), "io_wait", "iowait")
	if !ok || ioWait > 1 {
		return coreBlockedPayload{}, "missing_or_invalid_iowait"
	}
	out := coreBlockedPayload{PID: pid, IOWait: ioWait, CallerRaw: caller}
	if delay, present, valid := directCoreOptionalUnsigned(ev, directWidths(4, 8), "delay"); present {
		if !valid {
			return coreBlockedPayload{}, "invalid_delay"
		}
		out.Delay, out.DelayKnown = delay>>10, true
	}
	symbolFields := directCoreDeclaredCount(ev, "func_name", "mod_name", "offset", "size")
	cnodeFields := directCoreDeclaredCount(ev, "cnode_idx")
	if symbolFields > 0 && cnodeFields > 0 {
		return coreBlockedPayload{}, "ambiguous_caller_profile"
	}
	if symbolFields > 0 {
		// Symbolization is an optional display group. A partial/truncated or
		// unsafe group degrades to the independently admitted raw caller; it
		// must not fabricate zero offsets or erase the blocked observation.
		if symbolFields == 4 && directCoreDescriptorLayoutValid(ev, coreRenderBlocked, true) {
			function, functionOK := directCoreString(ev, content, false, false, "func_name")
			module, moduleOK := directCoreString(ev, content, false, true, "mod_name")
			offset, offsetOK := directCoreUnsigned(ev, directWidths(4, 8), "offset")
			size, sizeOK := directCoreUnsigned(ev, directWidths(4, 8), "size")
			candidate := fmt.Sprintf("%s+0x%x/0x%x[%s]", function, offset, size, module)
			if functionOK && moduleOK && offsetOK && sizeOK && traceDBSingleToken(candidate) {
				out.Caller, out.CallerSymbolized = candidate, true
			}
		}
	} else if cnodeFields > 0 {
		cnode, valid := directCoreUnsigned(ev, directWidths(4, 8), "cnode_idx")
		if !valid {
			return coreBlockedPayload{}, "invalid_cnode_idx"
		}
		out.CNodeIndex, out.CNodeKnown = cnode, true
	}
	return out, ""
}

func decodeDirectCPUPayload(ev decodedEvent) (coreCPUPayload, string) {
	cpuID, ok := directCoreUnsigned(ev, directWidths(4), "cpu_id")
	if !ok || cpuID > uint64(maxTraceDBCPUIndex) {
		return coreCPUPayload{}, "missing_or_invalid_cpu_id"
	}
	if ev.format.Name == "cpu_frequency_limits" {
		officialFields := directCoreDeclaredCount(ev, "min_freq", "max_freq")
		legacyFields := directCoreDeclaredCount(ev, "min", "max")
		if officialFields > 0 && legacyFields > 0 {
			return coreCPUPayload{}, "ambiguous_limits_profile"
		}
		var minFreq, maxFreq uint64
		var minOK, maxOK bool
		switch {
		case officialFields > 0:
			if officialFields != 2 {
				return coreCPUPayload{}, "incomplete_limits_profile"
			}
			minFreq, minOK = directCoreUnsigned(ev, directWidths(4), "min_freq")
			maxFreq, maxOK = directCoreUnsigned(ev, directWidths(4), "max_freq")
		case legacyFields > 0:
			if legacyFields != 2 {
				return coreCPUPayload{}, "incomplete_limits_profile"
			}
			minFreq, minOK = directCoreUnsigned(ev, directWidths(4), "min")
			maxFreq, maxOK = directCoreUnsigned(ev, directWidths(4), "max")
		default:
			return coreCPUPayload{}, "missing_limits_profile"
		}
		if !minOK || !maxOK {
			return coreCPUPayload{}, "invalid_limits_profile"
		}
		if minFreq > maxFreq {
			return coreCPUPayload{}, "invalid_limits_order"
		}
		return coreCPUPayload{Min: minFreq, Max: maxFreq, CPUID: cpuID, IsLimits: true}, ""
	}
	state, ok := directCoreUnsigned(ev, directWidths(4), "state")
	if !ok {
		return coreCPUPayload{}, "missing_or_invalid_state"
	}
	return coreCPUPayload{State: state, CPUID: cpuID}, ""
}

func decodeDirectBinderPayload(ev decodedEvent) (coreBinderPayload, string) {
	if ev.format.Name == "binder_transaction_received" {
		official := directCoreDeclaredCount(ev, "debug_id")
		legacy := directCoreDeclaredCount(ev, "transaction")
		if official+legacy != 1 {
			return coreBinderPayload{}, "ambiguous_or_missing_received_profile"
		}
		field := "debug_id"
		if legacy == 1 {
			field = "transaction"
		}
		transaction, ok := directCoreSigned(ev, directWidths(4), field)
		if !ok || transaction <= 0 {
			return coreBinderPayload{}, "missing_or_invalid_transaction"
		}
		return coreBinderPayload{Transaction: transaction, Received: true}, ""
	}

	officialFields := directCoreDeclaredCount(ev, "debug_id", "target_node", "to_proc", "to_thread")
	legacyFields := directCoreDeclaredCount(ev, "transaction", "dest_node", "dest_proc", "dest_thread")
	if officialFields > 0 && legacyFields > 0 {
		return coreBinderPayload{}, "mixed_transaction_profile"
	}
	var names [4]string
	switch {
	case officialFields > 0:
		if officialFields != 4 {
			return coreBinderPayload{}, "incomplete_official_transaction_profile"
		}
		names = [4]string{"debug_id", "target_node", "to_proc", "to_thread"}
	case legacyFields > 0:
		if legacyFields != 4 {
			return coreBinderPayload{}, "incomplete_legacy_transaction_profile"
		}
		names = [4]string{"transaction", "dest_node", "dest_proc", "dest_thread"}
	default:
		return coreBinderPayload{}, "missing_transaction_profile"
	}
	values := [4]int64{}
	for index, name := range names {
		value, ok := directCoreSigned(ev, directWidths(4), name)
		if !ok {
			return coreBinderPayload{}, "invalid_" + name
		}
		values[index] = value
	}
	if values[0] <= 0 {
		return coreBinderPayload{}, "invalid_transaction_id"
	}
	if values[1] < 0 || values[2] < 0 || values[3] < 0 {
		return coreBinderPayload{}, "invalid_transaction_endpoint"
	}
	reply, ok := directCoreSigned(ev, directWidths(4), "reply")
	if !ok || (reply != 0 && reply != 1) {
		return coreBinderPayload{}, "missing_or_invalid_reply"
	}
	flags, ok := directCoreUnsigned(ev, directWidths(4), "flags")
	if !ok {
		return coreBinderPayload{}, "missing_or_invalid_flags"
	}
	code, ok := directCoreUnsigned(ev, directWidths(4), "code")
	if !ok {
		return coreBinderPayload{}, "missing_or_invalid_code"
	}
	out := coreBinderPayload{
		Transaction: values[0],
		DestNode:    values[1],
		DestProc:    values[2],
		DestThread:  values[3],
		Reply:       reply,
		Flags:       flags,
		Code:        code,
	}
	return out, ""
}

func decodeDirectInterruptPayload(ctx coreDecodeContext, ev decodedEvent, content []byte) (coreInterruptPayload, string) {
	var out coreInterruptPayload
	switch ev.format.Name {
	case "irq_handler_entry":
		irq, ok := directCoreSigned(ev, directWidths(4), "irq")
		if !ok || irq < 0 {
			return coreInterruptPayload{}, "missing_or_invalid_irq"
		}
		name, ok := directCoreString(ev, content, false, false, "name")
		if !ok {
			return coreInterruptPayload{}, "missing_or_invalid_irq_name"
		}
		out.IRQ, out.IRQName = irq, name
	case "irq_handler_exit":
		irq, ok := directCoreSigned(ev, directWidths(4), "irq")
		if !ok || irq < 0 {
			return coreInterruptPayload{}, "missing_or_invalid_irq"
		}
		ret, ok := directCoreSigned(ev, directWidths(4), "ret")
		if !ok {
			return coreInterruptPayload{}, "missing_or_invalid_ret"
		}
		out.IRQ, out.Ret = irq, ret
	case "softirq_entry", "softirq_exit", "softirq_raise":
		vec, ok := directCoreUnsigned(ev, directWidths(4), "vec")
		if !ok || vec > 9 {
			return coreInterruptPayload{}, "missing_or_invalid_vec"
		}
		out.Vec = vec
	case "ipi_entry", "ipi_exit":
		reason, ok := directCoreIPIReason(ctx, ev, content)
		if !ok {
			return coreInterruptPayload{}, "missing_or_invalid_reason"
		}
		out.Reason = reason
	case "ipi_raise":
		mask, maskProfile, ok := directCoreIPIMask(ev, content)
		if !ok {
			return coreInterruptPayload{}, "missing_or_invalid_target_mask"
		}
		reason, reasonProfile, ok := directCoreIPIReasonWithProfile(ctx, ev, content)
		if !ok {
			return coreInterruptPayload{}, "missing_or_invalid_reason"
		}
		if maskProfile != reasonProfile {
			return coreInterruptPayload{}, "mixed_ipi_profile"
		}
		out.TargetMask, out.Reason = mask, reason
	default:
		return coreInterruptPayload{}, "invalid_interrupt_kind"
	}
	return out, ""
}

func renderCanonicalCorePayload(payload coreRenderPayload) (string, bool) {
	switch payload.Kind {
	case coreRenderWakeup:
		if payload.Wakeup == nil {
			return "", false
		}
		item := payload.Wakeup
		return fmt.Sprintf("comm=%s pid=%d prio=%d target_cpu=%03d", item.Comm, item.PID, item.Priority, item.TargetCPU), true
	case coreRenderBlocked:
		if payload.Blocked == nil {
			return "", false
		}
		item := payload.Blocked
		parts := []string{fmt.Sprintf("pid=%d", item.PID), fmt.Sprintf("iowait=%d", item.IOWait)}
		switch {
		case item.CallerSymbolized:
			parts = append(parts, "caller="+item.Caller)
		case item.CNodeKnown:
			parts = append(parts, fmt.Sprintf("caller=0x%x", item.CallerRaw), fmt.Sprintf("cnode_idx=%d", item.CNodeIndex))
		default:
			parts = append(parts, "caller=unknown", fmt.Sprintf("caller_raw=0x%x", item.CallerRaw), "caller_quality=opaque")
		}
		if item.DelayKnown {
			parts = append(parts, fmt.Sprintf("delay=%d", item.Delay))
		}
		return strings.Join(parts, " "), true
	case coreRenderCPU:
		if payload.CPU == nil {
			return "", false
		}
		item := payload.CPU
		if item.IsLimits {
			return fmt.Sprintf("min=%d max=%d cpu_id=%d", item.Min, item.Max, item.CPUID), true
		}
		return fmt.Sprintf("state=%d cpu_id=%d", item.State, item.CPUID), true
	case coreRenderBinder:
		if payload.Binder == nil {
			return "", false
		}
		item := payload.Binder
		if item.Received {
			return fmt.Sprintf("transaction=%d", item.Transaction), true
		}
		return fmt.Sprintf("transaction=%d dest_node=%d dest_proc=%d dest_thread=%d reply=%d flags=0x%x code=0x%x",
			item.Transaction, item.DestNode, item.DestProc, item.DestThread, item.Reply, item.Flags, item.Code), true
	case coreRenderInterrupt:
		if payload.Interrupt == nil {
			return "", false
		}
		item := payload.Interrupt
		switch payload.Name {
		case "irq_handler_entry":
			return fmt.Sprintf("irq=%d name=%s", item.IRQ, item.IRQName), true
		case "irq_handler_exit":
			ret := "unhandled"
			if item.Ret != 0 {
				ret = "handled"
			}
			return fmt.Sprintf("irq=%d ret=%s", item.IRQ, ret), true
		case "softirq_entry", "softirq_exit", "softirq_raise":
			if action := softirqAction(int64(item.Vec)); action != "" {
				return fmt.Sprintf("vec=%d [action=%s]", item.Vec, action), true
			}
			return fmt.Sprintf("vec=%d", item.Vec), true
		case "ipi_entry", "ipi_exit":
			return fmt.Sprintf("(%s)", item.Reason), true
		case "ipi_raise":
			// Pinned OpenHarmony default and 6.6.30 formatters use PRIu64
			// decimal for target_cpus; Donghu has no IPI row to override it.
			return fmt.Sprintf("target_mask=%d (%s)", item.TargetMask, item.Reason), true
		default:
			return "", false
		}
	default:
		return "", false
	}
}

type directWidthSet uint16

func directWidths(widths ...int) directWidthSet {
	var out directWidthSet
	for _, width := range widths {
		if width > 0 && width < 16 {
			out |= 1 << width
		}
	}
	return out
}

func (set directWidthSet) allows(width int) bool {
	return width > 0 && width < 16 && set&(1<<width) != 0
}

func directCoreSigned(ev decodedEvent, widths directWidthSet, names ...string) (int64, bool) {
	field, raw, ok := directCoreUniqueField(ev, names...)
	if !ok || !widths.allows(len(raw)) || field.Size != len(raw) || !directCoreSigned32TypeWidthAllowed(field, len(raw)) {
		return 0, false
	}
	return intFromBytes(raw, true), true
}

func directCoreUnsigned(ev decodedEvent, widths directWidthSet, names ...string) (uint64, bool) {
	field, raw, ok := directCoreUniqueField(ev, names...)
	if !ok || !widths.allows(len(raw)) || field.Size != len(raw) || !directCoreUnsignedTypeWidthAllowed(field, len(raw), widths) {
		return 0, false
	}
	return uintFromSupportedWidth(raw)
}

func directCoreAddress(ev decodedEvent, widths directWidthSet, names ...string) (uint64, bool) {
	field, raw, ok := directCoreUniqueField(ev, names...)
	if !ok || field.Signed || !widths.allows(len(raw)) || field.Size != len(raw) {
		return 0, false
	}
	if directCoreArrayDeclared(field) {
		return 0, false
	}
	typeName := normalizeFieldType(field.Type)
	pointerProfile := (typeName == "void *" || typeName == "void*") && (len(raw) == 4 || len(raw) == 8)
	wordProfile := directCoreUnsignedWordTypeWidthAllowed(field, len(raw))
	if !pointerProfile && !wordProfile {
		return 0, false
	}
	return uintFromSupportedWidth(raw)
}

func directCoreArrayDeclared(field eventField) bool {
	return strings.ContainsAny(field.Name, "[]")
}

func directCoreSigned32TypeWidthAllowed(field eventField, width int) bool {
	if !field.Signed || width != 4 || directCoreArrayDeclared(field) {
		return false
	}
	switch normalizeFieldType(field.Type) {
	case "int", "signed", "signed int", "pid_t", "int32_t", "s32", "__s32":
		return true
	default:
		return false
	}
}

func directCoreUnsigned32TypeWidthAllowed(field eventField, width int) bool {
	if field.Signed || width != 4 || directCoreArrayDeclared(field) {
		return false
	}
	switch normalizeFieldType(field.Type) {
	case "unsigned", "unsigned int", "uint32_t", "u32", "__u32":
		return true
	default:
		return false
	}
}

func directCoreUnsignedWordTypeWidthAllowed(field eventField, width int) bool {
	if field.Signed || directCoreArrayDeclared(field) {
		return false
	}
	switch normalizeFieldType(field.Type) {
	case "unsigned long", "unsigned long int", "long unsigned int":
		return width == 4 || width == 8
	case "unsigned long long", "unsigned long long int", "long long unsigned int", "uint64_t", "u64", "__u64":
		return width == 8
	default:
		return false
	}
}

func directCoreUnsignedTypeWidthAllowed(field eventField, width int, widths directWidthSet) bool {
	switch widths {
	case directWidths(4):
		return directCoreUnsigned32TypeWidthAllowed(field, width)
	case directWidths(1, 4):
		if width == 1 && !field.Signed && !directCoreArrayDeclared(field) {
			switch normalizeFieldType(field.Type) {
			case "bool", "_bool", "unsigned char", "uint8_t", "u8", "__u8":
				return true
			}
		}
		return directCoreUnsigned32TypeWidthAllowed(field, width)
	case directWidths(4, 8):
		return directCoreUnsigned32TypeWidthAllowed(field, width) || directCoreUnsignedWordTypeWidthAllowed(field, width)
	default:
		return false
	}
}

func directCoreOptionalUnsigned(ev decodedEvent, widths directWidthSet, names ...string) (uint64, bool, bool) {
	if directCoreDeclaredCount(ev, names...) == 0 {
		return 0, false, true
	}
	value, ok := directCoreUnsigned(ev, widths, names...)
	return value, true, ok
}

func directCoreDeclaredCount(ev decodedEvent, names ...string) int {
	count := 0
	for _, field := range ev.format.Fields {
		if cleanNameIn(directCoreFieldBaseName(field.Name), names) {
			count++
		}
	}
	return count
}

func directCoreFieldBaseName(name string) string {
	if index := strings.IndexByte(name, '['); index >= 0 {
		return name[:index]
	}
	return name
}

func directCoreUniqueField(ev decodedEvent, names ...string) (eventField, []byte, bool) {
	var selected eventField
	count := 0
	for _, field := range ev.format.Fields {
		if !cleanNameIn(directCoreFieldBaseName(field.Name), names) {
			continue
		}
		selected = field
		count++
	}
	if count != 1 {
		return eventField{}, nil, false
	}
	raw, ok := ev.fields[selected.Name]
	if !ok {
		return eventField{}, nil, false
	}
	return selected, raw, true
}

func directCoreString(ev decodedEvent, content []byte, allowInternalSpace, allowBlank bool, names ...string) (string, bool) {
	field, raw, ok := directCoreUniqueField(ev, names...)
	if !ok {
		return "", false
	}
	typeName := normalizeFieldType(field.Type)
	var source []byte
	switch typeName {
	case "__data_loc char[]":
		source, ok = directCoreDataLocSlice(ev, content, field, raw, "__data_loc char[]")
		if !ok || len(source) == 0 || source[len(source)-1] != 0 || bytesIndexNUL(source[:len(source)-1]) >= 0 {
			return "", false
		}
	case "char":
		if field.Signed || !directCoreArrayDeclared(field) || field.Size != len(raw) {
			return "", false
		}
		source = raw
	default:
		return "", false
	}
	if bytesIndexNUL(source) < 0 {
		return "", false
	}
	if nul := bytesIndexNUL(source); nul >= 0 {
		source = source[:nul]
	}
	value := string(source)
	if value != strings.TrimSpace(value) || !traceDBSinglePhysicalLine(value, allowBlank) {
		return "", false
	}
	if !allowInternalSpace && value != "" && !traceDBSingleToken(value) {
		return "", false
	}
	if allowInternalSpace && strings.ContainsAny(value, "=|") {
		return "", false
	}
	return value, true
}

func directCoreDataLocSlice(ev decodedEvent, content []byte, field eventField, raw []byte, exactType string) ([]byte, bool) {
	if field.Signed || field.Size != 4 || len(raw) != 4 || normalizeFieldType(field.Type) != exactType {
		return nil, false
	}
	fixedTail, ok := directCoreDescriptorFixedTail(ev)
	if !ok {
		return nil, false
	}
	location := binary.LittleEndian.Uint32(raw)
	offset := int(location & 0xffff)
	length := int(location >> 16)
	if offset < fixedTail || length <= 0 || offset > len(content) || length > len(content)-offset {
		return nil, false
	}
	return content[offset : offset+length], true
}

func directCoreDescriptorFixedTail(ev decodedEvent) (int, bool) {
	maxInt := int(^uint(0) >> 1)
	fixedTail := 0
	for _, field := range ev.format.Fields {
		if field.Offset < 0 || field.Size <= 0 || field.Offset > maxInt-field.Size {
			return 0, false
		}
		if end := field.Offset + field.Size; end > fixedTail {
			fixedTail = end
		}
	}
	return fixedTail, true
}

type directIPIProfile uint8

const (
	directIPIProfileUnknown directIPIProfile = iota
	directIPIProfileKernel
	directIPIProfileLegacyInline
)

func directCoreIPIReason(ctx coreDecodeContext, ev decodedEvent, content []byte) (string, bool) {
	reason, _, ok := directCoreIPIReasonWithProfile(ctx, ev, content)
	return reason, ok
}

func directCoreIPIReasonWithProfile(ctx coreDecodeContext, ev decodedEvent, content []byte) (string, directIPIProfile, bool) {
	field, raw, ok := directCoreUniqueField(ev, "reason")
	if !ok {
		return "", directIPIProfileUnknown, false
	}
	typeName := normalizeFieldType(field.Type)
	if typeName == "const char *" || typeName == "const char*" || typeName == "char *" || typeName == "char*" {
		if field.Signed || directCoreArrayDeclared(field) || (len(raw) != 4 && len(raw) != 8) || field.Size != len(raw) {
			return "", directIPIProfileUnknown, false
		}
		address, ok := uintFromSupportedWidth(raw)
		if !ok || address == 0 || ctx.PrintkPoisoned[address] {
			return "", directIPIProfileUnknown, false
		}
		reason, found := ctx.PrintkFormats[address]
		if !found || !validCoreIPIReason(reason) {
			return "", directIPIProfileUnknown, false
		}
		return reason, directIPIProfileKernel, true
	}
	reason, ok := directCoreString(ev, content, true, false, "reason")
	if !ok || !validCoreIPIReason(reason) {
		return "", directIPIProfileUnknown, false
	}
	return reason, directIPIProfileLegacyInline, true
}

func directCoreIPIMask(ev decodedEvent, content []byte) (uint64, directIPIProfile, bool) {
	field, raw, ok := directCoreUniqueField(ev, "target_cpus", "target_mask")
	if !ok {
		return 0, directIPIProfileUnknown, false
	}
	typeName := normalizeFieldType(field.Type)
	if typeName == "__data_loc unsigned long[]" {
		if directCoreFieldBaseName(field.Name) != "target_cpus" {
			return 0, directIPIProfileUnknown, false
		}
		dynamic, ok := directCoreDataLocSlice(ev, content, field, raw, "__data_loc unsigned long[]")
		if !ok || (len(dynamic) != 4 && len(dynamic) != 8) {
			return 0, directIPIProfileUnknown, false
		}
		return uint64(intFromBytes(dynamic, false)), directIPIProfileKernel, true
	}
	value, ok := directCoreUnsigned(ev, directWidths(4, 8), "target_cpus", "target_mask")
	if !ok {
		return 0, directIPIProfileUnknown, false
	}
	return value, directIPIProfileLegacyInline, true
}

func validCoreIPIReason(reason string) bool {
	if reason == "" || reason != strings.TrimSpace(reason) || !traceDBSinglePhysicalLine(reason, false) {
		return false
	}
	for _, r := range reason {
		switch r {
		case '\\', '"', '(', ')', '=', '|':
			return false
		}
	}
	return true
}
