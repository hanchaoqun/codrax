package hitraceconv

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
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
	pid := int(intField(ev, "common_pid", false))
	comm := ctx.cmdlines[pid]
	if pid == 0 {
		comm = "<idle>"
	} else if comm == "" {
		comm = "<...>"
	}
	tgidText := "-----"
	if tgid, ok := ctx.tgids[pid]; ok {
		tgidText = strconv.Itoa(tgid)
	}
	body, known := renderEventBody(ev, content, cpu)
	name := format.Name
	if name == "" {
		name = "unknown_event"
	}
	flags := traceFlagsToStr(intField(ev, "common_flags", false), intField(ev, "common_preempt_count", false))
	return fmt.Sprintf("%16s-%-6d (%5s) [%03d] %s %s: %s: %s",
		comm, pid, tgidText, cpu, flags, formatTimestamp(tsNS), name, body), known
}

func decodeEvent(format eventFormat, content []byte) decodedEvent {
	fields := make(map[string][]byte, len(format.Fields))
	for _, f := range format.Fields {
		if f.Offset < 0 || f.Size <= 0 || f.Offset+f.Size > len(content) {
			continue
		}
		fields[f.Name] = content[f.Offset : f.Offset+f.Size]
	}
	return decodedEvent{format: format, fields: fields}
}

func renderEventBody(ev decodedEvent, content []byte, cpu int) (string, bool) {
	if body, ok := renderOfficialOpenHarmonyBody(ev, content); ok {
		return body, true
	}
	switch ev.format.Name {
	case "sched_switch":
		if hasField(ev, "prev_comm[16]") {
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
			body := fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
				firstNonEmpty(strField(ev, "pname[16]"), idleName(cpu, intField(ev, "prev_tid", true))),
				intField(ev, "prev_tid", true),
				intField(ev, "pprio", true),
				harmonyPrevState(uint64(intField(ev, "pstate", false))),
				firstNonEmpty(strField(ev, "nname[16]"), idleName(cpu, intField(ev, "next_tid", true))),
				intField(ev, "next_tid", true),
				intField(ev, "nprio", true))
			if extras := schedSwitchHarmonyExtras(ev); extras != "" {
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
		return renderClockSetRate(ev, content), true
	case "block_rq_issue", "block_rq_complete":
		return renderBlockRequest(ev, content), true
	case "block_bio_remap":
		return renderBlockRemap(ev), true
	case "filemap_set_wb_err":
		return renderFilemapSetWBErr(ev), true
	case "mm_filemap_add_to_page_cache", "mm_filemap_delete_from_page_cache":
		return renderMMFilemapPageCache(ev), true
	case "binder_transaction":
		return renderKV(ev, "transaction", "dest_node", "dest_proc", "dest_thread", "reply", "flags", "code"), true
	case "binder_transaction_received":
		return renderKV(ev, "transaction"), true
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

func schedSwitchHarmonyExtras(ev decodedEvent) string {
	var parts []string
	if nextInfo := harmonySchedInfo(ev); nextInfo != "" {
		parts = append(parts, "next_info="+nextInfo)
	}
	if cg := firstNonEmpty(strFieldByCleanName(ev, "cg"), strFieldByCleanName(ev, "cgroup")); cg != "" {
		parts = append(parts, "cg="+cg)
	}
	return strings.Join(parts, " ")
}

func harmonySchedInfo(ev decodedEvent) string {
	_, ninfo, ok := fieldByCleanName(ev, "ninfo")
	if !ok {
		if s := strFieldByCleanName(ev, "next_info"); s != "" {
			return s
		}
		return ""
	}
	if len(ninfo) < 8 {
		if s := strings.TrimSpace(strFieldByCleanName(ev, "ninfo")); s != "" && isMostlyPrintable(s) {
			return s
		}
		return ""
	}
	affinityRaw := binary.LittleEndian.Uint32(ninfo[:4])
	affinity := strings.TrimLeft(fmt.Sprintf("%x", affinityRaw), "0")
	if affinity == "" {
		affinity = "0"
	}
	remaining := binary.LittleEndian.Uint32(ninfo[4:8])
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
	if !hasCleanField(ev, "cg") && !hasCleanField(ev, "cgroup") {
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

func renderClockSetRate(ev decodedEvent, content []byte) string {
	name := firstNonEmpty(
		dataLocStringByCleanName(ev, content, "name", "clk_name", "clock"),
		strField(ev, "name[16]"),
		strField(ev, "clk_name[32]"),
		strField(ev, "clock[32]"),
	)
	state := intField(ev, "state", false)
	cpuID := intField(ev, "cpu_id", true)
	if name != "" {
		return fmt.Sprintf("%s state=%d cpu_id=%d", name, state, cpuID)
	}
	return renderKV(ev, "state", "cpu_id")
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

func renderMMFilemapPageCache(ev decodedEvent) string {
	sDev := firstNonZero(intField(ev, "s_dev", false), intField(ev, "dev", false))
	index := intField(ev, "index", false)
	return fmt.Sprintf("dev %s ino 0x%x page=0x%x pfn=%d ofs=%d",
		devMajorMinor(sDev, ":"),
		firstNonZero(intField(ev, "i_ino", false), intField(ev, "ino", false)),
		intField(ev, "pg", false),
		intField(ev, "pfn", false),
		index<<12)
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
	us := (ns + 500) / 1000
	return fmt.Sprintf("%5d.%06d", us/1_000_000, us%1_000_000)
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
		if end := f.Offset + f.Size; end > limit {
			limit = end
		}
	}
	if limit <= 0 {
		return nil
	}
	content := make([]byte, limit)
	for _, f := range ev.format.Fields {
		if b := ev.fields[f.Name]; len(b) > 0 && f.Offset >= 0 && f.Offset+len(b) <= len(content) {
			copy(content[f.Offset:f.Offset+len(b)], b)
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
	if s := dataLocStringByCleanName(ev, content, "name"); s != "" {
		return s
	}
	if s := strField(ev, "name[32]"); s != "" {
		return s
	}
	pos := int(intField(ev, "name", false) & 0xffff)
	if pos >= 0 && pos < len(content) {
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
