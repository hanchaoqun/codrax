package hitraceconv

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

const systraceHeader = `# tracer: nop
#
# entries-in-buffer/entries-written: %lu/%lu   #P:%d
#
#                                      _-----=> irqs-off
#                                     / _----=> need-resched
#                                    | / _---=> hardirq/softirq
#                                    || / _--=> preempt-depth
#                                    ||| /     delay
#           TASK-PID    TGID   CPU#  ||||    TIMESTAMP  FUNCTION
#              | |        |      |   ||||       |         |
`

type decodedEvent struct {
	format eventFormat
	fields map[string][]byte
}

type renderContext struct {
	cmdlines map[int]string
	tgids    map[int]int
}

func renderEventLine(ctx renderContext, tsNS uint64, cpu int, format eventFormat, content []byte) (string, bool) {
	ev := decodeEvent(format, content)
	prefix, envelopeOK := renderEventPrefix(ctx, tsNS, cpu, ev)
	if !envelopeOK {
		return "", false
	}
	body, known := renderEventBody(ev, content, cpu)
	name := format.Name
	if name == "" {
		name = "unknown_event"
	}
	return prefix + name + ": " + body, known
}

func renderEventHeaderLine(ctx renderContext, tsNS uint64, cpu int, format eventFormat, content []byte) string {
	prefix, ok := renderEventPrefix(ctx, tsNS, cpu, decodeEvent(format, content))
	if !ok {
		return ""
	}
	return prefix
}

func renderEventPrefix(ctx renderContext, tsNS uint64, cpu int, ev decodedEvent) (string, bool) {
	pid, flags, preemptCount, ok := decodeDirectFtraceCommonEnvelope(ev)
	if !ok || !validTraceDBCPUIndex(int64(cpu)) {
		return "", false
	}
	comm := ctx.cmdlines[pid]
	if pid == 0 {
		comm = "<idle>"
	} else if comm == "" || !traceDBSinglePhysicalLine(comm, true) {
		comm = "<...>"
	} else {
		// comm is display-only. Match the shared structured/SQL ftrace
		// formatter's kernel TASK_COMM_LEN-compatible 15-rune projection;
		// never let a long or renamed display value alter the typed PID/TGID.
		comm = traceDBCommName(comm, "<...>")
	}
	tgidText := "-----"
	if tgid, found := ctx.tgids[pid]; pid > 0 && found && tgid > 0 && int64(tgid) <= math.MaxInt32 {
		tgidText = strconv.Itoa(tgid)
	}
	return fmt.Sprintf("%16s-%-5d (%5s) [%03d] %s %s: ",
		comm, pid, tgidText, cpu, traceFlagsToStr(flags, preemptCount), formatTimestamp(tsNS)), true
}

// decodeDirectFtraceCommonEnvelope is the sole raw-prefix admission point.
// The RMQ content bytes are not protobuf: a missing, aliased, mistyped,
// duplicate or truncated common field has no default-value semantics and must
// never become a plausible CPU0/idle header.
func decodeDirectFtraceCommonEnvelope(ev decodedEvent) (pid int, flags, preemptCount int64, ok bool) {
	pidValue, pidOK := directFtraceCommonField(ev, "common_pid", 4, 4, true)
	flagsValue, flagsOK := directFtraceCommonField(ev, "common_flags", 2, 1, false)
	preemptValue, preemptOK := directFtraceCommonField(ev, "common_preempt_count", 3, 1, false)
	if !pidOK || !flagsOK || !preemptOK || pidValue < 0 || pidValue > math.MaxInt32 {
		return 0, 0, 0, false
	}
	return int(pidValue), flagsValue, preemptValue, true
}

func directFtraceCommonField(ev decodedEvent, name string, offset, size int, signed bool) (int64, bool) {
	field, raw, ok := uniqueFieldByCleanNames(ev, name)
	if !ok || field.Name != name || field.Offset != offset || field.Size != size || field.Signed != signed || len(raw) != size {
		return 0, false
	}
	lowerType := strings.ToLower(strings.Join(strings.Fields(field.Type), " "))
	switch name {
	case "common_pid":
		switch lowerType {
		case "int", "signed int", "int32_t", "__s32", "s32", "pid_t":
		default:
			return 0, false
		}
	case "common_flags", "common_preempt_count":
		switch lowerType {
		case "unsigned char", "uint8_t", "__u8", "u8":
		default:
			return 0, false
		}
	default:
		return 0, false
	}
	return intFromBytes(raw, signed), true
}

func decodeEvent(format eventFormat, content []byte) decodedEvent {
	fields := make(map[string][]byte, len(format.Fields))
	for _, f := range format.Fields {
		start, end, ok := eventFieldBounds(f.Offset, f.Size, len(content))
		if !ok {
			continue
		}
		fields[f.Name] = content[start:end]
	}
	return decodedEvent{format: format, fields: fields}
}

func eventFieldBounds(offset, size, limit int) (int, int, bool) {
	if offset < 0 || size <= 0 || offset > limit || size > limit-offset {
		return 0, 0, false
	}
	return offset, offset + size, true
}

func renderEventBody(ev decodedEvent, content []byte, cpu int) (string, bool) {
	if body, ok := renderOfficialOpenHarmonyBody(ev, content, cpu); ok {
		return body, true
	}
	switch ev.format.Name {
	case "sched_switch":
		if hasField(ev, "prev_comm[16]") {
			if !standardSchedSwitchCorePresent(ev) {
				return "", false
			}
			return fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
				strField(ev, "prev_comm[16]"),
				intField(ev, "prev_pid", true),
				intField(ev, "prev_prio", true),
				linuxPrevState(uint64(intField(ev, "prev_state", true))),
				strField(ev, "next_comm[16]"),
				intField(ev, "next_pid", true),
				intField(ev, "next_prio", true)), true
		}
		if hasAnyField(ev, "pname[16]", "prev_tid", "pprio", "pstate", "nname[16]", "next_tid", "nprio") {
			if !harmonySchedSwitchCorePresent(ev) {
				return "", false
			}
			body := fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
				firstNonEmpty(strField(ev, "pname[16]"), idleName(cpu, intField(ev, "prev_tid", true))),
				intField(ev, "prev_tid", true),
				intField(ev, "pprio", true),
				harmonyPrevState(uint64(intField(ev, "pstate", false))),
				firstNonEmpty(strField(ev, "nname[16]"), idleName(cpu, intField(ev, "next_tid", true))),
				intField(ev, "next_tid", true),
				intField(ev, "nprio", true))
			if extras := schedSwitchHarmonyExtras(ev, content); extras != "" {
				body += " " + extras
			}
			return body, true
		}
	case "sched_wakeup", "sched_waking":
		comm := firstNonEmpty(strField(ev, "comm[16]"), strField(ev, "pname[16]"))
		return fmt.Sprintf("comm=%s pid=%d prio=%d target_cpu=%03d",
			comm,
			intField(ev, "pid", true),
			intField(ev, "prio", true),
			intField(ev, "target_cpu", true)), true
	case "sched_blocked_reason":
		return fmt.Sprintf("pid=%d iowait=%d caller=%s delay=%d",
			intField(ev, "pid", true),
			intField(ev, "iowait", false),
			blockedCaller(ev),
			intField(ev, "delay", false)>>10), true
	case "cpu_idle":
		return renderKV(ev, "state", "cpu_id"), true
	case "cpu_frequency":
		return renderKV(ev, "state", "cpu_id"), true
	case "clock_set_rate":
		return renderClockSetRate(ev, content)
	case "block_rq_issue", "block_rq_complete":
		return renderBlockRequest(ev, content), true
	case "block_bio_remap":
		return renderBlockRemap(ev), true
	case "filemap_set_wb_err":
		return renderFilemapSetWBErr(ev), true
	case "mm_filemap_add_to_page_cache", "mm_filemap_delete_from_page_cache":
		return renderMMFilemapPageCache(ev)
	case "binder_transaction":
		return renderBinderTransaction(ev), true
	case "binder_transaction_received":
		return renderKV(ev, "transaction"), true
	case "binder_transaction_alloc_buf", "binder_alloc_buf":
		return renderKV(ev, "transaction", "debug_id", "data_size", "offsets_size", "extra_buffers_size"), true
	case "binder_transaction_reply", "binder_reply", "binder_transaction_lock", "binder_lock", "binder_transaction_locked", "binder_locked", "binder_transaction_unlock", "binder_unlock":
		return renderKV(ev, "transaction", "debug_id", "tag"), true
	case "irq_handler_entry":
		return fmt.Sprintf("irq=%d name=%s", intField(ev, "irq", true), irqName(ev, content)), true
	case "irq_handler_exit":
		ret := "unhandled"
		if intField(ev, "ret", true) != 0 {
			ret = "handled"
		}
		return fmt.Sprintf("irq=%d ret=%s", intField(ev, "irq", true), ret), true
	case "tracing_mark_write", "print":
		if payload := firstTracePayload(ev, content); payload != "" {
			return payload, true
		}
	}
	if len(ev.format.Fields) == 0 {
		return missingFormatPayload(ev.format.ID, content), false
	}
	return genericFields(ev, content), false
}

func schedSwitchHarmonyExtras(ev decodedEvent, content []byte) string {
	var parts []string
	if nextInfo := harmonySchedInfo(ev); nextInfo != "" {
		parts = append(parts, "next_info="+nextInfo)
	}
	if cg := safeOptionalCleanString(ev, content, "cg", "cgroup"); cg != "" {
		parts = append(parts, "cg="+cg)
	}
	return strings.Join(parts, " ")
}

func harmonySchedInfo(ev decodedEvent) string {
	field, rawField, ok := uniqueFieldByCleanNames(ev, "ninfo", "next_info")
	if !ok {
		return ""
	}
	if cleanFieldName(field.Name) == "next_info" {
		lowerType := strings.ToLower(field.Type)
		if strings.Contains(lowerType, "char") || strings.Contains(lowerType, "string") {
			return canonicalHarmonySchedInfoText(
				rawNULTerminatedField(ev, field.Name),
				!hasDeclaredCleanField(ev, "cg", "cgroup"),
			)
		}
	}
	// Both ninfo[8] and numeric next_info are packed uint64 fields. Reject
	// malformed widths rather than letting intFromBytes turn a short payload
	// into a plausible affinity/load tuple.
	if len(rawField) != 8 {
		return ""
	}
	raw := binary.LittleEndian.Uint64(rawField)
	if raw == ^uint64(0) {
		return ""
	}
	return formatHarmonySchedInfo(raw, !hasDeclaredCleanField(ev, "cg", "cgroup"))
}

func rawNULTerminatedField(ev decodedEvent, name string) string {
	raw := ev.fields[name]
	if i := bytesIndexNUL(raw); i >= 0 {
		raw = raw[:i]
	}
	return string(raw)
}

func canonicalHarmonySchedInfoText(raw string, includeCGID bool) string {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return ""
	}
	parts := strings.Split(raw, ",")
	wantParts := 5
	if includeCGID {
		wantParts = 6
	}
	if len(parts) != wantParts {
		return ""
	}
	for _, r := range parts[0] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return ""
		}
	}
	affinity, err := strconv.ParseUint(parts[0], 16, 32)
	if err != nil || strings.TrimSpace(parts[0]) != parts[0] || parts[0] == "" {
		return ""
	}
	limits := []uint64{2046, 3, 1, 7}
	if includeCGID {
		limits = append(limits, 31)
	}
	values := make([]uint64, len(limits))
	for i, limit := range limits {
		part := parts[i+1]
		for _, r := range part {
			if r < '0' || r > '9' {
				return ""
			}
		}
		value, err := strconv.ParseUint(part, 10, 32)
		if err != nil || part == "" || strings.TrimSpace(part) != part || value > limit {
			return ""
		}
		values[i] = value
	}
	canonical := fmt.Sprintf("%x,%d,%d,%d,%d", affinity, values[0], values[1], values[2], values[3])
	if includeCGID {
		canonical += fmt.Sprintf(",%d", values[4])
	}
	return canonical
}

// formatHarmonySchedInfo is the single text authority for the packed
// OpenHarmony/Donghu scheduler extension. The lower word is the affinity mask;
// the upper word carries load/group/restriction/expel/cgroup bit fields.
func formatHarmonySchedInfo(raw uint64, includeCGID bool) string {
	affinityRaw := uint32(raw)
	affinity := strings.TrimLeft(fmt.Sprintf("%x", affinityRaw), "0")
	if affinity == "" {
		affinity = "0"
	}
	remaining := uint32(raw >> 32)
	load := (remaining & ((1 << 10) - 1)) << 1
	group := (remaining >> 10) & ((1 << 2) - 1)
	restricted := (remaining >> 12) & 1
	expel := (remaining >> 13) & ((1 << 3) - 1)
	parts := []string{
		affinity,
		strconv.FormatUint(uint64(load), 10),
		strconv.FormatUint(uint64(group), 10),
		strconv.FormatUint(uint64(restricted), 10),
		strconv.FormatUint(uint64(expel), 10),
	}
	if includeCGID {
		cgid := (remaining >> 16) & ((1 << 5) - 1)
		parts = append(parts, strconv.FormatUint(uint64(cgid), 10))
	}
	return strings.Join(parts, ",")
}

func missingFormatPayload(eventID int, content []byte) string {
	const maxPayloadBytes = 32
	limit := len(content)
	truncated := false
	if limit > maxPayloadBytes {
		limit = maxPayloadBytes
		truncated = true
	}
	parts := []string{
		fmt.Sprintf("event_id=%d", eventID),
		fmt.Sprintf("payload_len=%d", len(content)),
	}
	if limit > 0 {
		parts = append(parts, "payload_hex="+hex.EncodeToString(content[:limit]))
	}
	if truncated {
		parts = append(parts, "payload_truncated=true")
	}
	return strings.Join(parts, " ")
}

func renderClockSetRate(ev decodedEvent, content []byte) (string, bool) {
	name, namePresent := strictClockName(ev, content)
	state, statePresent := uintByCleanName(ev, "state")
	if !namePresent || !statePresent {
		return "", false
	}
	parts := make([]string, 0, 3)
	parts = append(parts, name, fmt.Sprintf("state=%d", state))
	if cpuID, ok := uintByCleanName(ev, "cpu_id"); ok {
		parts = appendClockSetRateCPU(parts, cpuID)
	}
	return strings.Join(parts, " "), true
}

func strictClockName(ev decodedEvent, content []byte) (string, bool) {
	selected, raw, ok := uniqueFieldByCleanNames(ev, "name", "clk_name", "clock")
	if !ok {
		return "", false
	}
	lowerType := normalizeFieldType(selected.Type)
	if strings.Contains(lowerType, "__rel_loc") {
		// Relative locators need the declaration offset to resolve correctly;
		// treating their four wire bytes as an inline string would forge a name.
		return "", false
	}
	if dataLocField(selected) {
		return strictDataLocString(raw, content)
	}
	if strings.Contains(lowerType, "char") || strings.Contains(lowerType, "string") ||
		(lowerType == "" && (strings.Contains(selected.Name, "[") || len(raw) != 4)) {
		return strictNULTerminatedString(raw, false)
	}
	// Legacy OpenHarmony formats declare name as an unsigned 32-bit offset
	// rather than __data_loc. Width is part of that producer profile.
	if len(raw) != 4 || !legacyClockOffsetTypeAllowed(lowerType) {
		return "", false
	}
	offset := int(binary.LittleEndian.Uint32(raw) & 0xffff)
	if offset <= 0 || offset >= len(content) {
		return "", false
	}
	return strictNULTerminatedString(content[offset:], true)
}

func strictDataLocString(raw, content []byte) (string, bool) {
	if len(raw) != 4 {
		return "", false
	}
	loc := binary.LittleEndian.Uint32(raw)
	offset := int(loc & 0xffff)
	length := int(loc >> 16)
	if offset <= 0 || length <= 0 || offset > len(content) || length > len(content)-offset {
		return "", false
	}
	return strictNULTerminatedString(content[offset:offset+length], true)
}

func legacyClockOffsetTypeAllowed(lowerType string) bool {
	switch lowerType {
	case "", "unsigned int", "u32", "__u32":
		return true
	default:
		return false
	}
}

func normalizeFieldType(raw string) string {
	return strings.Join(strings.Fields(strings.ToLower(raw)), " ")
}

func strictNULTerminatedString(raw []byte, requireNUL bool) (string, bool) {
	nul := bytesIndexNUL(raw)
	if requireNUL && nul < 0 {
		return "", false
	}
	if nul >= 0 {
		i := nul
		raw = raw[:i]
	}
	value := string(raw)
	if !traceDBSingleToken(value) {
		return "", false
	}
	return value, true
}

// traceDBSingleToken protects the public key=value line grammar. Clock names
// and cgroup labels occupy one token; accepting whitespace, '=' or '|' would
// let source text mint sibling typed fields when tracequery reparses the row.
func traceDBSingleToken(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !traceDBSinglePhysicalLine(value, false) {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || r == '=' || r == '|' {
			return false
		}
	}
	return true
}

func renderBlockRequest(ev decodedEvent, content []byte) string {
	dev := blockDevText(ev, "dev", "dev_t")
	rwbs := firstNonEmpty(strField(ev, "rwbs[8]"), strField(ev, "rwbs[16]"), strField(ev, "cmd[16]"), "RW")
	sector := intField(ev, "sector", false)
	nr := firstNonZero(intField(ev, "nr_sector", false), intField(ev, "nr_bytes", false), intField(ev, "bytes", false))
	cmd := firstNonEmpty(dataLocStringByCleanName(ev, content, "cmd"), strField(ev, "cmd[16]"), strField(ev, "cmd[32]"))
	if ev.format.Name == "block_rq_complete" {
		return fmt.Sprintf("%s %s (%s) %d + %d [%d]", dev, rwbs, cmd, sector, nr, intField(ev, "error", true))
	}
	comm := firstNonEmpty(strField(ev, "comm[16]"), strField(ev, "comm[32]"))
	bytesValue := intField(ev, "bytes", false)
	return fmt.Sprintf("%s %s %d (%s) %d + %d [%s]", dev, rwbs, bytesValue, cmd, sector, nr, comm)
}

func renderBlockRemap(ev decodedEvent) string {
	dev := blockDevText(ev, "dev", "dev_t")
	sector := intField(ev, "sector", false)
	nr := firstNonZero(intField(ev, "nr_sector", false), intField(ev, "nr_bytes", false), intField(ev, "bytes", false))
	oldDev := blockDevText(ev, "old_dev", "from")
	oldSector := intField(ev, "old_sector", false)
	if rwbs := firstNonEmpty(strField(ev, "rwbs[8]"), strField(ev, "rwbs[16]")); rwbs != "" {
		return fmt.Sprintf("%s %s %d + %d <- (%s) %d", dev, rwbs, sector, nr, oldDev, oldSector)
	}
	return fmt.Sprintf("%s %d + %d <- (%s) %d", dev, sector, nr, oldDev, oldSector)
}

func renderFilemapSetWBErr(ev decodedEvent) string {
	sDev := firstNonZero(intField(ev, "s_dev", false), intField(ev, "dev", false))
	return fmt.Sprintf("dev=%s ino=0x%x errseq=0x%x",
		devMajorMinor(sDev, ":"),
		firstNonZero(intField(ev, "i_ino", false), intField(ev, "ino", false)),
		intField(ev, "errseq", false))
}

func renderMMFilemapPageCache(ev decodedEvent) (string, bool) {
	sDev, devPresent := uniqueUintByCleanName(ev, "s_dev", "dev")
	ino, inoPresent := uniqueUintByCleanName(ev, "i_ino", "ino")
	index, indexPresent := uintByCleanName(ev, "index")
	pfn, pfnPresent := uintByCleanName(ev, "pfn")
	if !devPresent || sDev > math.MaxUint32 || !inoPresent || !indexPresent || !pfnPresent || index > ^uint64(0)>>12 {
		return "", false
	}
	page, pagePresent := uniqueUintByCleanName(ev, "pg", "page")
	return renderFilemapPageCacheBody(
		devMajorMinor(int64(sDev), ":"),
		ino,
		page,
		pagePresent,
		pfn,
		index<<12,
	), true
}

func renderFilemapPageCacheBody(dev string, ino, page uint64, pagePresent bool, pfn, offset uint64) string {
	parts := []string{
		"dev " + dev,
		fmt.Sprintf("ino 0x%x", ino),
	}
	if pagePresent {
		// page is a kernel pointer-shaped uint64. Keep its full bit pattern;
		// formatting through int64 would render high addresses with a minus.
		parts = append(parts, fmt.Sprintf("page=0x%x", page))
	}
	parts = append(parts, fmt.Sprintf("pfn=%d", pfn), fmt.Sprintf("ofs=%d", offset))
	return strings.Join(parts, " ")
}

func uintByCleanName(ev decodedEvent, names ...string) (uint64, bool) {
	field, raw, ok := uniqueFieldByCleanNames(ev, names...)
	if !ok || !numericFieldTypeAllowed(field) {
		return 0, false
	}
	return uintFromSupportedWidth(raw)
}

func uniqueUintByCleanName(ev decodedEvent, names ...string) (uint64, bool) {
	return uintByCleanName(ev, names...)
}

func uintFromSupportedWidth(raw []byte) (uint64, bool) {
	switch len(raw) {
	case 1, 2, 4, 8:
		return uint64(intFromBytes(raw, false)), true
	default:
		return 0, false
	}
}

func appendClockSetRateCPU(parts []string, cpuID uint64) []string {
	if cpuID <= uint64(maxTraceDBCPUIndex) {
		return append(parts, fmt.Sprintf("cpu_id=%d", cpuID))
	}
	return parts
}

func standardSchedSwitchCorePresent(ev decodedEvent) bool {
	return hasCleanStringField(ev, "prev_comm") &&
		hasCleanPIDTIDField(ev, "prev_pid") &&
		hasCleanSignedInt32Field(ev, "prev_prio") &&
		hasCleanIntegerField(ev, "prev_state") &&
		hasCleanStringField(ev, "next_comm") &&
		hasCleanPIDTIDField(ev, "next_pid") &&
		hasCleanSignedInt32Field(ev, "next_prio")
}

func harmonySchedSwitchCorePresent(ev decodedEvent) bool {
	return hasCleanHarmonyCommField(ev, "pname", "prev_tid") &&
		hasCleanPIDTIDField(ev, "prev_tid") &&
		hasCleanSignedInt32Field(ev, "pprio") &&
		hasCleanIntegerField(ev, "pstate") &&
		hasCleanHarmonyCommField(ev, "nname", "next_tid") &&
		hasCleanPIDTIDField(ev, "next_tid") &&
		hasCleanSignedInt32Field(ev, "nprio")
}

func hasCleanHarmonyCommField(ev decodedEvent, name, tidName string) bool {
	field, _, ok := uniqueFieldByCleanNames(ev, name)
	if !ok || !hasCleanPIDTIDField(ev, tidName) {
		return false
	}
	lowerType := strings.ToLower(field.Type)
	if strings.Contains(lowerType, "__data_loc") || strings.Contains(lowerType, "__rel_loc") {
		return false
	}
	if lowerType != "" && !strings.Contains(lowerType, "char") && !strings.Contains(lowerType, "string") {
		return false
	}
	value := rawNULTerminatedField(ev, field.Name)
	if !traceDBSinglePhysicalLine(value, true) {
		return false
	}
	if strings.TrimSpace(value) != "" {
		return true
	}
	return intByCleanName(ev, tidName, true) == 0
}

func hasCleanIntegerField(ev decodedEvent, name string) bool {
	field, raw, ok := uniqueFieldByCleanNames(ev, name)
	if !ok || !numericFieldTypeAllowed(field) {
		return false
	}
	_, validWidth := uintFromSupportedWidth(raw)
	return validWidth
}

func numericFieldTypeAllowed(field eventField) bool {
	lowerType := strings.ToLower(field.Type)
	return lowerType == "" ||
		(!strings.Contains(lowerType, "char") &&
			!strings.Contains(lowerType, "string") &&
			!strings.Contains(lowerType, "__data_loc") &&
			!strings.Contains(lowerType, "__rel_loc"))
}

func hasCleanPIDTIDField(ev decodedEvent, name string) bool {
	value := intByCleanName(ev, name, true)
	return hasCleanIntegerField(ev, name) && value >= 0 && value <= math.MaxInt32
}

func hasCleanSignedInt32Field(ev decodedEvent, name string) bool {
	value := intByCleanName(ev, name, true)
	return hasCleanIntegerField(ev, name) && value >= math.MinInt32 && value <= math.MaxInt32
}

func hasCleanStringField(ev decodedEvent, name string) bool {
	field, _, ok := uniqueFieldByCleanNames(ev, name)
	if !ok {
		return false
	}
	lowerType := strings.ToLower(field.Type)
	if strings.Contains(lowerType, "__data_loc") || strings.Contains(lowerType, "__rel_loc") {
		return false
	}
	if lowerType != "" && !strings.Contains(lowerType, "char") && !strings.Contains(lowerType, "string") {
		return false
	}
	value := rawNULTerminatedField(ev, field.Name)
	return strings.TrimSpace(value) != "" && traceDBSinglePhysicalLine(value, false)
}

func safeOptionalCleanString(ev decodedEvent, content []byte, names ...string) string {
	field, raw, ok := uniqueFieldByCleanNames(ev, names...)
	if !ok {
		return ""
	}
	lowerType := strings.ToLower(field.Type)
	if strings.Contains(lowerType, "__rel_loc") {
		return ""
	}
	if dataLocField(field) {
		value, valid := strictDataLocString(raw, content)
		if valid {
			return value
		}
		return ""
	}
	if lowerType != "" && !strings.Contains(lowerType, "char") && !strings.Contains(lowerType, "string") {
		return ""
	}
	value, valid := strictNULTerminatedString(raw, false)
	if !valid {
		return ""
	}
	return value
}

func blockDevText(ev decodedEvent, names ...string) string {
	for _, name := range names {
		if hasField(ev, name) {
			return devMajorMinor(intField(ev, name, false), ",")
		}
	}
	if s := strField(ev, "devname[32]"); s != "" {
		return s
	}
	return "0,0"
}

func devMajorMinor(dev int64, sep string) string {
	if sep == "" {
		sep = ","
	}
	return fmt.Sprintf("%d%s%d", dev>>20, sep, dev&0xfffff)
}

func renderKV(ev decodedEvent, names ...string) string {
	var parts []string
	for _, name := range names {
		if !hasField(ev, name) {
			continue
		}
		if s := strField(ev, name); s != "" && fieldLooksString(name) {
			parts = append(parts, fmt.Sprintf("%s=%s", cleanFieldName(name), s))
		} else {
			parts = append(parts, fmt.Sprintf("%s=%d", cleanFieldName(name), intField(ev, name, false)))
		}
	}
	if len(parts) == 0 {
		return genericFields(ev, nil)
	}
	return strings.Join(parts, " ")
}

func renderBinderTransaction(ev decodedEvent) string {
	var parts []string
	for _, name := range []string{"transaction", "dest_node", "dest_proc", "dest_thread", "reply"} {
		if hasField(ev, name) {
			parts = append(parts, fmt.Sprintf("%s=%d", name, intField(ev, name, false)))
		}
	}
	if hasField(ev, "flags") {
		parts = append(parts, fmt.Sprintf("flags=0x%x", intField(ev, "flags", false)))
	}
	if hasField(ev, "code") {
		parts = append(parts, fmt.Sprintf("code=0x%x", intField(ev, "code", false)))
	}
	if len(parts) == 0 {
		return genericFields(ev, nil)
	}
	return strings.Join(parts, " ")
}

func genericFields(ev decodedEvent, content []byte) string {
	var parts []string
	for _, f := range ev.format.Fields {
		if strings.HasPrefix(f.Name, "common_") {
			continue
		}
		name := cleanFieldName(f.Name)
		if dataLocField(f) {
			if s := dataLocFieldString(ev, f, content); s != "" {
				parts = append(parts, fmt.Sprintf("%s=%s", name, s))
				continue
			}
		}
		if s := strField(ev, f.Name); s != "" && fieldShouldRenderAsString(f) {
			parts = append(parts, fmt.Sprintf("%s=%s", name, s))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", name, intField(ev, f.Name, f.Signed)))
	}
	if len(parts) == 0 {
		return "raw_event=unparsed"
	}
	return strings.Join(parts, " ")
}

func formatTimestamp(ns uint64) string {
	// Round without adding to ns directly: a valid near-MaxUint64 timestamp
	// must not wrap merely because the text renderer rounds nanoseconds.
	us := roundedTimestampUS(ns)
	return fmt.Sprintf("%5d.%06d", us/1_000_000, us%1_000_000)
}

func roundedTimestampUS(ns uint64) uint64 {
	us := ns / 1000
	if ns%1000 >= 500 {
		us++
	}
	return us
}

func traceFlagsToStr(flags int64, preemptCount int64) string {
	if flags == 0 && preemptCount == 0 {
		return "...."
	}
	var b strings.Builder
	if flags&0x01 != 0 {
		b.WriteByte('d')
	} else if flags&0x02 != 0 {
		b.WriteByte('X')
	} else {
		b.WriteByte('.')
	}
	needResched := flags&0x04 != 0
	preemptResched := flags&0x20 != 0
	switch {
	case needResched && preemptResched:
		b.WriteByte('N')
	case needResched:
		b.WriteByte('n')
	case preemptResched:
		b.WriteByte('p')
	default:
		b.WriteByte('.')
	}
	nmi := flags&0x40 != 0
	hardIRQ := flags&0x08 != 0
	softIRQ := flags&0x10 != 0
	switch {
	case nmi && hardIRQ:
		b.WriteByte('Z')
	case nmi:
		b.WriteByte('z')
	case hardIRQ && softIRQ:
		b.WriteByte('H')
	case hardIRQ:
		b.WriteByte('h')
	case softIRQ:
		b.WriteByte('s')
	default:
		b.WriteByte('.')
	}
	if preemptCount != 0 {
		const digits = "0123456789abcdef"
		b.WriteByte(digits[preemptCount&0x0f])
	} else {
		b.WriteByte('.')
	}
	return b.String()
}

func hasField(ev decodedEvent, name string) bool {
	_, ok := ev.fields[name]
	return ok
}

func hasAnyField(ev decodedEvent, names ...string) bool {
	for _, name := range names {
		if hasField(ev, name) {
			return true
		}
	}
	return false
}

func strField(ev decodedEvent, name string) string {
	b := ev.fields[name]
	if len(b) == 0 {
		return ""
	}
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

func strFieldByCleanName(ev decodedEvent, want string) string {
	if f, _, ok := fieldByCleanName(ev, want); ok {
		return strField(ev, f.Name)
	}
	return ""
}

func fieldByCleanName(ev decodedEvent, want string) (eventField, []byte, bool) {
	for _, f := range ev.format.Fields {
		if cleanFieldName(f.Name) != want {
			continue
		}
		b, ok := ev.fields[f.Name]
		if ok {
			return f, b, true
		}
		return f, nil, false
	}
	return eventField{}, nil, false
}

// uniqueFieldByCleanNames is the admission authority for compatibility aliases.
// It counts declarations, not only decoded values, so a truncated alias cannot
// be silently rescued by another physical field and duplicate authorities are
// rejected even when their bytes happen to agree.
func uniqueFieldByCleanNames(ev decodedEvent, wants ...string) (eventField, []byte, bool) {
	var selected eventField
	count := 0
	for _, field := range ev.format.Fields {
		if !cleanNameIn(cleanFieldName(field.Name), wants) {
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

func hasDeclaredCleanField(ev decodedEvent, names ...string) bool {
	for _, field := range ev.format.Fields {
		if cleanNameIn(cleanFieldName(field.Name), names) {
			return true
		}
	}
	return false
}

func cleanNameIn(name string, wants []string) bool {
	for _, want := range wants {
		if name == want {
			return true
		}
	}
	return false
}

func hasCleanField(ev decodedEvent, want string) bool {
	_, _, ok := fieldByCleanName(ev, want)
	return ok
}

func dataLocFieldString(ev decodedEvent, f eventField, content []byte) string {
	if !dataLocField(f) {
		return ""
	}
	if content == nil {
		content = eventContent(ev)
	}
	if content == nil {
		return ""
	}
	loc := uint32(intField(ev, f.Name, false))
	off := int(loc & 0xffff)
	ln := int(loc >> 16)
	if off < 0 || off >= len(content) || ln <= 0 {
		return ""
	}
	end := off + ln
	if end > len(content) {
		end = len(content)
	}
	if end <= off {
		return ""
	}
	b := content[off:end]
	if i := bytesIndexNUL(b); i >= 0 {
		b = b[:i]
	}
	s := strings.TrimSpace(string(b))
	if s == "" || !isMostlyPrintable(s) {
		return ""
	}
	return clampDynamicString(s, 300)
}

func dataLocStringByCleanName(ev decodedEvent, content []byte, names ...string) string {
	for _, want := range names {
		for _, f := range ev.format.Fields {
			if cleanFieldName(f.Name) == want {
				if s := dataLocFieldString(ev, f, content); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func dataLocField(f eventField) bool {
	return strings.Contains(strings.ToLower(f.Type), "__data_loc")
}

func eventContent(ev decodedEvent) []byte {
	limit := 0
	for _, f := range ev.format.Fields {
		raw, exists := ev.fields[f.Name]
		if !exists {
			continue
		}
		_, end, ok := eventFieldBounds(f.Offset, len(raw), tracePageSize)
		if ok && end > limit {
			limit = end
		}
	}
	if limit <= 0 {
		return nil
	}
	content := make([]byte, limit)
	for _, f := range ev.format.Fields {
		if b, exists := ev.fields[f.Name]; exists {
			start, end, ok := eventFieldBounds(f.Offset, len(b), len(content))
			if ok {
				copy(content[start:end], b)
			}
		}
	}
	return content
}

func bytesIndexNUL(b []byte) int {
	for i, v := range b {
		if v == 0 {
			return i
		}
	}
	return -1
}

func intField(ev decodedEvent, name string, signedDefault bool) int64 {
	b := ev.fields[name]
	if len(b) == 0 {
		return 0
	}
	signed := signedDefault
	for _, f := range ev.format.Fields {
		if f.Name == name {
			signed = f.Signed || signedDefault
			break
		}
	}
	return intFromBytes(b, signed)
}

func intFieldString(ev decodedEvent, name string, signed bool) string {
	if !hasField(ev, name) {
		return ""
	}
	return strconv.FormatInt(intField(ev, name, signed), 10)
}

func intFromBytes(b []byte, signed bool) int64 {
	var u uint64
	switch len(b) {
	case 1:
		u = uint64(b[0])
	case 2:
		u = uint64(binary.LittleEndian.Uint16(b))
	case 4:
		u = uint64(binary.LittleEndian.Uint32(b))
	default:
		if len(b) >= 8 {
			u = binary.LittleEndian.Uint64(b[:8])
		} else {
			for i := len(b) - 1; i >= 0; i-- {
				u = (u << 8) | uint64(b[i])
			}
		}
	}
	if !signed {
		return int64(u)
	}
	bits := uint(len(b) * 8)
	if bits == 0 || bits >= 64 {
		return int64(u)
	}
	sign := uint64(1) << (bits - 1)
	if u&sign == 0 {
		return int64(u)
	}
	mask := ^uint64(0) << bits
	return int64(u | mask)
}

func linuxPrevState(v uint64) string {
	flags := []struct {
		bit  uint64
		name string
	}{
		{0x1, "S"},
		{0x2, "D"},
		{0x4, "T"},
		{0x8, "t"},
		{0x10, "X"},
		{0x20, "Z"},
		{0x40, "P"},
		{0x80, "I"},
	}
	var parts []string
	for _, f := range flags {
		if v&f.bit != 0 {
			parts = append(parts, f.name)
		}
	}
	state := "R"
	if len(parts) > 0 {
		state = strings.Join(parts, "|")
	}
	if v&0x100 != 0 {
		state += "+"
	}
	return state
}

func harmonyPrevState(v uint64) string {
	switch v {
	case 0:
		return "R"
	case 1:
		return "S"
	case 2:
		return "D"
	case 0x10:
		return "X"
	case 0x100:
		return "R+"
	default:
		return "?"
	}
}

func blockedCaller(ev decodedEvent) string {
	funcName := strField(ev, "func_name[20]")
	modName := strField(ev, "mod_name[12]")
	if funcName != "" {
		return fmt.Sprintf("%s+0x%x/0x%x[%s]", funcName, intField(ev, "offset", false), intField(ev, "size", false), modName)
	}
	if hasField(ev, "caller") {
		return fmt.Sprintf("0x%x", intField(ev, "caller", false))
	}
	return ""
}

func irqName(ev decodedEvent, content []byte) string {
	if s := stringByCleanName(ev, content, "name"); s != "" {
		return s
	}
	if s := strField(ev, "name[32]"); s != "" {
		return s
	}
	pos := int(intField(ev, "name", false) & 0xffff)
	if pos > 0 && pos < len(content) {
		b := content[pos:]
		if i := strings.IndexByte(string(b), 0); i >= 0 {
			b = b[:i]
		}
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	return ""
}

func firstTracePayload(ev decodedEvent, content []byte) string {
	for _, name := range []string{"buf", "buf[256]", "str", "str[256]", "trace[256]"} {
		if s := strField(ev, name); s != "" {
			return s
		}
	}
	if s := dataLocStringByCleanName(ev, content, "buf", "str", "trace"); s != "" {
		return s
	}
	pos := int(intField(ev, "buf", false) & 0xffff)
	if pos > 0 && pos < len(content) {
		b := content[pos:]
		if i := strings.IndexByte(string(b), 0); i >= 0 {
			b = b[:i]
		}
		return strings.TrimSpace(string(b))
	}
	return genericFields(ev, content)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstNonZero(values ...int64) int64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func idleName(cpu int, pid int64) string {
	if pid == 0 {
		return fmt.Sprintf("tppmgr-idle-%d", cpu)
	}
	return ""
}

func cleanFieldName(name string) string {
	name = strings.TrimPrefix(name, "__data_loc_")
	if i := strings.IndexByte(name, '['); i >= 0 {
		return name[:i]
	}
	return name
}

func fieldShouldRenderAsString(f eventField) bool {
	lowerType := strings.ToLower(f.Type)
	if strings.Contains(lowerType, "char") || strings.Contains(lowerType, "string") {
		return true
	}
	return fieldLooksString(f.Name)
}

func fieldLooksString(name string) bool {
	switch strings.ToLower(cleanFieldName(name)) {
	case "comm", "prev_comm", "next_comm", "pname", "nname",
		"name", "buf", "str", "trace", "cmd", "rwbs",
		"devname", "dev_name", "clk_name", "clock",
		"func_name", "mod_name", "cg", "cgroup",
		"driver", "timeline", "profile_info":
		return true
	default:
		return false
	}
}

func isMostlyPrintable(s string) bool {
	if s == "" {
		return false
	}
	printable := 0
	for _, r := range s {
		if unicode.IsPrint(r) && r != unicode.ReplacementChar {
			printable++
		}
	}
	return printable*2 >= len([]rune(s))
}

func clampTaskName(s string) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= 32 {
		return s
	}
	r := []rune(s)
	return string(r[:32])
}

func clampDynamicString(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}
