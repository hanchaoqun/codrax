package hitraceconv

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

func renderOfficialOpenHarmonyBody(ev decodedEvent, content []byte, cpu int) (string, bool) {
	name := ev.format.Name
	lowerName := strings.ToLower(name)
	switch {
	case name == "sched_switch" && hasCleanField(ev, "prev_comm"):
		if !standardSchedSwitchCorePresent(ev) {
			return "", false
		}
		body := fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
			stringByCleanName(ev, content, "prev_comm"), intByCleanName(ev, "prev_pid", true), intByCleanName(ev, "prev_prio", true),
			linuxPrevState(uint64(intByCleanName(ev, "prev_state", true))), stringByCleanName(ev, content, "next_comm"),
			intByCleanName(ev, "next_pid", true), intByCleanName(ev, "next_prio", true))
		// The standard and Harmony sched_switch layouts are both seen in
		// production. Optional extensions are independent format authorities:
		// a missing field is not the integer zero.
		if hasCleanIntegerField(ev, "expeller_type") {
			body += fmt.Sprintf(" expeller_type=%d", intByCleanName(ev, "expeller_type", false))
		}
		if extras := schedSwitchHarmonyExtras(ev, content); extras != "" {
			body += " " + extras
		}
		return body, true
	case name == "sched_switch" && hasCleanField(ev, "pname"):
		if !harmonySchedSwitchCorePresent(ev) {
			return "", false
		}
		body := fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
			firstNonEmpty(stringByCleanName(ev, content, "pname"), idleName(cpu, intByCleanName(ev, "prev_tid", true))),
			intByCleanName(ev, "prev_tid", true), intByCleanName(ev, "pprio", true),
			harmonyPrevState(uint64(intByCleanName(ev, "pstate", false))),
			firstNonEmpty(stringByCleanName(ev, content, "nname"), idleName(cpu, intByCleanName(ev, "next_tid", true))),
			intByCleanName(ev, "next_tid", true), intByCleanName(ev, "nprio", true))
		if extras := schedSwitchHarmonyExtras(ev, content); extras != "" {
			body += " " + extras
		}
		return body, true
	case strings.HasPrefix(name, "sched_stat_"):
		return renderSchedStat(ev, content), true
	case name == "clock_set_rate":
		return renderClockSetRate(ev, content)
	case strings.Contains(name, "softirq") && hasCleanField(ev, "vec"):
		// Preserve the pre-existing non-core vendor compatibility lane. The
		// three governed exact softirq names are intercepted by the typed core
		// decoder before this legacy renderer is reachable.
		vec := intByCleanName(ev, "vec", false)
		return fmt.Sprintf("vec=%d [action=%s]", vec, softirqAction(vec)), true
	case strings.HasPrefix(name, "ext4_da_write_begin"):
		return fmt.Sprintf("dev %s ino %d pos %d len %d flags %d",
			devByCleanName(ev, "dev", ","), intByCleanName(ev, "ino", false), intByCleanName(ev, "pos", true),
			intByCleanName(ev, "len", false), intByCleanName(ev, "flags", false)), true
	case strings.HasPrefix(name, "ext4_da_write_end"):
		return fmt.Sprintf("dev %s ino %d pos %d len %d copied %d",
			devByCleanName(ev, "dev", ","), intByCleanName(ev, "ino", false), intByCleanName(ev, "pos", true),
			intByCleanName(ev, "len", false), intByCleanName(ev, "copied", false)), true
	case strings.HasPrefix(name, "ext4_sync_file_enter"):
		return fmt.Sprintf("dev %s ino %d parent %d datasync %d ",
			devByCleanName(ev, "dev", ","), intByCleanName(ev, "ino", false), intByCleanName(ev, "parent", false),
			intByCleanName(ev, "datasync", true)), true
	case strings.HasPrefix(name, "ext4_sync_file_exit"):
		return fmt.Sprintf("dev %s ino %d ret %d", devByCleanName(ev, "dev", ","),
			intByCleanName(ev, "ino", false), intByCleanName(ev, "ret", true)), true
	case strings.HasPrefix(lowerName, "ext4_direct_io"):
		return renderExt4DirectIO(ev, content), true
	case name == "block_bio_queue" || name == "block_bio_complete" || name == "block_bio_remap" ||
		name == "block_rq_remap" || name == "block_rq_issue" || name == "block_rq_insert" || name == "block_rq_complete":
		return renderDirectBlockEvent(ev, content)
	case strings.HasPrefix(lowerName, "android_fs_dataread") || strings.HasPrefix(lowerName, "android_fs_datawrite"):
		return renderAndroidFSIO(ev, content), true
	case strings.HasPrefix(lowerName, "scsi_dispatch_cmd"):
		return renderSCSIDispatchCmd(ev, content), true
	case strings.HasPrefix(name, "ufshcd_command"):
		if hasCleanField(ev, "group_id") {
			opcode := intByCleanName(ev, "opcode", false)
			return fmt.Sprintf("%s: %s: tag: %d, DB: 0x%x, size: %d, IS: %d, LBA: %d, opcode: 0x%x (%s), group_id: 0x%x",
				stringByCleanName(ev, content, "str"), stringByCleanName(ev, content, "dev_name"), intByCleanName(ev, "tag", false),
				intByCleanName(ev, "doorbell", false), intByCleanName(ev, "transfer_len", true), intByCleanName(ev, "intr", false),
				intByCleanName(ev, "lba", false), opcode, ufsOpcodeName(opcode), intByCleanName(ev, "group_id", false)), true
		}
		return fmt.Sprintf("%s: %s: tag: %d, DB: 0x%x, size: %d, IS: %d, LBA: %d, opcode: 0x%x",
			stringByCleanName(ev, content, "str"), stringByCleanName(ev, content, "dev_name"), intByCleanName(ev, "tag", false),
			intByCleanName(ev, "doorbell", false), intByCleanName(ev, "transfer_len", true), intByCleanName(ev, "intr", false),
			intByCleanName(ev, "lba", false), intByCleanName(ev, "opcode", false)), true
	case strings.HasPrefix(name, "ufshcd_upiu"):
		return fmt.Sprintf("%s: %s: HDR:0x%s, CDB:0x%s",
			stringByCleanName(ev, content, "str"), stringByCleanName(ev, content, "dev_name"),
			littleEndianBytesHex(fieldBytesByCleanName(ev, "hdr")), littleEndianBytesHex(fieldBytesByCleanName(ev, "tsf"))), true
	case strings.HasPrefix(name, "ufshcd_uic_command"):
		return fmt.Sprintf("%s: %s: cmd: 0x%x, arg1: 0x%x, arg2: 0x%x, arg3: 0x%x",
			stringByCleanName(ev, content, "str"), stringByCleanName(ev, content, "dev_name"),
			intByCleanName(ev, "cmd", false), intByCleanName(ev, "arg1", false), intByCleanName(ev, "arg2", false), intByCleanName(ev, "arg3", false)), true
	case strings.HasPrefix(name, "ufshcd_clk_scaling"):
		return fmt.Sprintf("%s: %s %s from %d to %d Hz",
			stringByCleanName(ev, content, "dev_name"), stringByCleanName(ev, content, "state"), stringByCleanName(ev, content, "clk"),
			intByCleanName(ev, "prev_state", false), intByCleanName(ev, "curr_state", false)), true
	case strings.HasPrefix(name, "ufshcd_clk_gating"):
		return fmt.Sprintf("%s: gating state changed to %s", stringByCleanName(ev, content, "dev_name"),
			ufsClkGatingState(intByCleanName(ev, "state", true))), true
	case strings.HasPrefix(name, "ufshcd_auto_bkops_state"):
		return fmt.Sprintf("%s: auto bkops - %s", stringByCleanName(ev, content, "dev_name"), stringByCleanName(ev, content, "state")), true
	case strings.HasPrefix(name, "ufshcd_profile"):
		return fmt.Sprintf("%s: %s: took %d usecs, err %d", stringByCleanName(ev, content, "dev_name"),
			stringByCleanName(ev, content, "profile_info"), intByCleanName(ev, "time_us", true), intByCleanName(ev, "err", true)), true
	case strings.HasPrefix(name, "ufshcd_"):
		if hasCleanField(ev, "usecs") && hasCleanField(ev, "dev_state") && hasCleanField(ev, "link_state") {
			return fmt.Sprintf("%s: took %d usecs, dev_state: %s, link_state: %s, err %d",
				stringByCleanName(ev, content, "dev_name"), intByCleanName(ev, "usecs", true),
				ufsDevState(intByCleanName(ev, "dev_state", true)), ufsLinkState(intByCleanName(ev, "link_state", true)),
				intByCleanName(ev, "err", true)), true
		}
	case strings.HasPrefix(name, "regulator_set_voltage_complete"):
		return fmt.Sprintf("name=%s, val=%d", stringByCleanName(ev, content, "name"), intByCleanName(ev, "val", false)), true
	case strings.HasPrefix(name, "regulator_set_voltage"):
		return fmt.Sprintf("name=%s (%d-%d)", stringByCleanName(ev, content, "name"), intByCleanName(ev, "min", true), intByCleanName(ev, "max", true)), true
	case strings.HasPrefix(name, "regulator_"):
		return fmt.Sprintf("name=%s", stringByCleanName(ev, content, "name")), true
	case strings.HasPrefix(name, "dma_fence") && !directPairNameGoverned(name):
		return fmt.Sprintf("driver=%s timeline=%s context=%d seqno=%d", stringByCleanName(ev, content, "driver"),
			stringByCleanName(ev, content, "timeline"), intByCleanName(ev, "context", false), intByCleanName(ev, "seqno", false)), true
	case strings.HasPrefix(name, "rss_stat"):
		return fmt.Sprintf("mm_id=%d curr=%d member=%d size=%d", intByCleanName(ev, "mm_id", false),
			intByCleanName(ev, "curr", false), intByCleanName(ev, "member", true), intByCleanName(ev, "size", true)), true
	case strings.HasPrefix(name, "workqueue_execute") && !directPairNameGoverned(name):
		return fmt.Sprintf("work=0x%x function=0x%x", intByCleanName(ev, "work", false), intByCleanName(ev, "function", false)), true
	case strings.HasPrefix(name, "thermal_power_allocator_pid"):
		return fmt.Sprintf("thermal_zone_id=%d err=%d err_integral=%d p=%d i=%d d=%d output=%d",
			intByCleanName(ev, "tz_id", true), intByCleanName(ev, "err", true), intByCleanName(ev, "err_integral", true),
			intByCleanName(ev, "p", true), intByCleanName(ev, "i", true), intByCleanName(ev, "d", true), intByCleanName(ev, "output", true)), true
	case strings.HasPrefix(name, "thermal_power_allocator"):
		numActors := int(intByCleanName(ev, "num_actors", false))
		return fmt.Sprintf("thermal_zone_id=%d req_power={%s} total_req_power=%d granted_power={%s} total_granted_power=%d power_range=%d max_allocatable_power=%d current_temperature=%d delta_temperature=%d",
			intByCleanName(ev, "tz_id", true), decimalByteArray(dynamicBytesByCleanName(ev, content, "req_power", numActors*4)),
			intByCleanName(ev, "total_req_power", false), decimalByteArray(dynamicBytesByCleanName(ev, content, "granted_power", numActors*4)),
			intByCleanName(ev, "total_granted_power", false), intByCleanName(ev, "power_range", false),
			intByCleanName(ev, "max_allocatable_power", false), intByCleanName(ev, "current_temp", true), intByCleanName(ev, "delta_temp", true)), true
	case name == "phase_task_delta":
		return fmt.Sprintf("comm=%s tid=%d delta_exec=%d deltas={%s}", stringByCleanName(ev, content, "name"),
			intByCleanName(ev, "tid", false), intByCleanName(ev, "delta_exec", false), stringByCleanName(ev, content, "info")), true
	case strings.HasPrefix(name, "erofs_") || strings.HasPrefix(name, "z_erofs_"):
		if body, ok := renderEROFS(ev, content); ok {
			return body, true
		}
	}
	return "", false
}

func renderAndroidFSIO(ev decodedEvent, content []byte) string {
	var parts []string
	parts = appendStringKV(parts, "dev", traceIODev(ev, content))
	parts = appendHexCleanIntKV(parts, "ino", ev, false, "ino", "inode", "i_ino")
	parts = appendStringKV(parts, "entry_name", stringByCleanName(ev, content, "entry_name", "name", "file", "filename"))
	parts = appendCleanIntKV(parts, "offset", ev, true, "offset", "ofs", "pos", "off")
	parts = appendCleanIntKV(parts, "bytes", ev, false, "bytes", "len", "length", "size")
	parts = appendStringKV(parts, "rw", traceIORW(ev, content))
	parts = appendCleanIntKV(parts, "ret", ev, true, "ret", "res", "error", "err")
	parts = appendCleanIntKV(parts, "latency_us", ev, false, "latency_us", "duration_us", "time_us", "usecs")
	parts = appendCleanIntKV(parts, "i_size", ev, false, "i_size", "file_size")
	return strings.Join(parts, " ")
}

func renderSchedStat(ev decodedEvent, content []byte) string {
	comm := firstNonEmpty(stringByCleanName(ev, content, "comm"), stringByCleanName(ev, content, "pname"), stringByCleanName(ev, content, "name"))
	pid := intByCleanName(ev, "pid", true)
	switch ev.format.Name {
	case "sched_stat_runtime":
		return fmt.Sprintf("comm=%s pid=%d runtime=%d [ns] vruntime=%d [ns]",
			comm, pid, intByCleanName(ev, "runtime", false), intByCleanName(ev, "vruntime", false))
	default:
		return fmt.Sprintf("comm=%s pid=%d delay=%d [ns]", comm, pid, intByCleanName(ev, "delay", false))
	}
}

func renderExt4DirectIO(ev decodedEvent, content []byte) string {
	var parts []string
	parts = appendStringKV(parts, "dev", traceIODev(ev, content))
	parts = appendHexCleanIntKV(parts, "ino", ev, false, "ino", "inode", "i_ino")
	parts = appendCleanIntKV(parts, "offset", ev, true, "offset", "ofs", "pos", "off")
	parts = appendCleanIntKV(parts, "len", ev, false, "len", "length", "bytes", "size")
	parts = appendStringKV(parts, "rw", traceIORW(ev, content))
	parts = appendCleanIntKV(parts, "ret", ev, true, "ret", "res", "error", "err")
	return strings.Join(parts, " ")
}

func renderSCSIDispatchCmd(ev decodedEvent, content []byte) string {
	var parts []string
	parts = appendCleanIntKV(parts, "tag", ev, true, "tag")
	parts = appendStringKV(parts, "dev", traceIODev(ev, content))
	parts = appendCleanIntKV(parts, "lba", ev, false, "lba", "sector")
	parts = appendCleanIntKV(parts, "len", ev, false, "len", "length", "bytes", "transfer_len")
	opcode := firstNonEmpty(stringByCleanName(ev, content, "opcode", "op", "rw", "rwbs"), traceIOOpcodeName(firstCleanInt(ev, false, "opcode")))
	parts = appendStringKV(parts, "opcode", opcode)
	parts = appendCleanIntKV(parts, "ret", ev, true, "ret", "res", "error", "err")
	parts = appendCleanIntKV(parts, "latency_us", ev, false, "latency_us", "duration_us", "time_us", "usecs")
	return strings.Join(parts, " ")
}

func traceIODev(ev decodedEvent, content []byte) string {
	if s := stringByCleanName(ev, content, "dev_name", "devname"); s != "" {
		return s
	}
	for _, name := range []string{"dev", "s_dev", "dev_t"} {
		if hasCleanField(ev, name) {
			return devByCleanName(ev, name, ":")
		}
	}
	return ""
}

func traceIOOperationFromName(name string) string {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "dataread"):
		return "read"
	case strings.Contains(name, "datawrite"):
		return "write"
	case strings.Contains(name, "read"):
		return "read"
	case strings.Contains(name, "write"):
		return "write"
	case strings.Contains(name, "sync"):
		return "sync"
	default:
		return ""
	}
}

func traceIORW(ev decodedEvent, content []byte) string {
	for _, name := range []string{"rw", "rwbs", "op", "operation"} {
		f, _, ok := fieldByCleanName(ev, name)
		if !ok {
			continue
		}
		lowerType := strings.ToLower(f.Type)
		if lowerType == "" || strings.Contains(lowerType, "char") || strings.Contains(lowerType, "string") {
			if s := stringByCleanName(ev, content, name); s != "" {
				return s
			}
			continue
		}
		if cleanFieldName(f.Name) == "rw" {
			return traceIORWFromInt(intByCleanName(ev, "rw", true))
		}
	}
	return traceIOOperationFromName(ev.format.Name)
}

func traceIORWFromInt(v int64) string {
	switch v {
	case 0:
		return "read"
	case 1:
		return "write"
	default:
		return strconv.FormatInt(v, 10)
	}
}

func traceIOOpcodeName(v int64, ok bool) string {
	if !ok {
		return ""
	}
	if name := ufsOpcodeName(v); name != "" {
		return name
	}
	return fmt.Sprintf("0x%x", v)
}

func firstCleanInt(ev decodedEvent, signed bool, names ...string) (int64, bool) {
	for _, name := range names {
		if hasCleanField(ev, name) {
			return intByCleanName(ev, name, signed), true
		}
	}
	return 0, false
}

func appendStringKV(parts []string, key, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return parts
	}
	return append(parts, fmt.Sprintf("%s=%s", key, value))
}

func appendIntKV(parts []string, key string, value int64, ok bool) []string {
	if !ok {
		return parts
	}
	return append(parts, fmt.Sprintf("%s=%d", key, value))
}

func appendCleanIntKV(parts []string, key string, ev decodedEvent, signed bool, names ...string) []string {
	value, ok := firstCleanInt(ev, signed, names...)
	return appendIntKV(parts, key, value, ok)
}

func appendHexIntKV(parts []string, key string, value int64, ok bool) []string {
	if !ok {
		return parts
	}
	return append(parts, fmt.Sprintf("%s=0x%x", key, value))
}

func appendHexCleanIntKV(parts []string, key string, ev decodedEvent, signed bool, names ...string) []string {
	value, ok := firstCleanInt(ev, signed, names...)
	return appendHexIntKV(parts, key, value, ok)
}

func renderEROFS(ev decodedEvent, content []byte) (string, bool) {
	name := ev.format.Name
	dev := devByCleanName(ev, "dev", ",")
	devParen := fmt.Sprintf("(%s)", dev)
	switch {
	case strings.HasPrefix(name, "erofs_read_enter"):
		return fmt.Sprintf("dev:%s ino:%d offset:%d size:%d entry_name:%s", dev, intByCleanName(ev, "ino", false), intByCleanName(ev, "off", false), intByCleanName(ev, "size", false), stringByCleanName(ev, content, "name")), true
	case strings.HasPrefix(name, "erofs_read_exit"):
		return fmt.Sprintf("dev:%s ino:%d offset:%d size:%d res:%d", dev, intByCleanName(ev, "ino", false), intByCleanName(ev, "off", false), intByCleanName(ev, "size", false), intByCleanName(ev, "res", false)), true
	case strings.HasPrefix(name, "erofs_read_iter_enter"):
		return fmt.Sprintf("dev:%s ino:%d offset:%d size:%d", dev, intByCleanName(ev, "ino", false), intByCleanName(ev, "off", false), intByCleanName(ev, "size", false)), true
	case strings.HasPrefix(name, "erofs_read_iter_exit"):
		return fmt.Sprintf("dev:%s ino:%d offset:%d size:%d res:%d", dev, intByCleanName(ev, "ino", false), intByCleanName(ev, "off", false), intByCleanName(ev, "size", false), intByCleanName(ev, "res", false)), true
	case strings.HasPrefix(name, "erofs_readdir"):
		return fmt.Sprintf("dev:%s, ino:%d, start_pos:%d, end_pos:%d, err:%d", devParen, intByCleanName(ev, "index", false), intByCleanName(ev, "start_pos", false), intByCleanName(ev, "end_pos", false), intByCleanName(ev, "res", false)), true
	case strings.HasPrefix(name, "erofs_lookup_start"):
		return fmt.Sprintf("dev:%s, ino:%d, name:%s", devParen, intByCleanName(ev, "index", false), stringByCleanName(ev, content, "name")), true
	case strings.HasPrefix(name, "erofs_lookup_end"):
		return fmt.Sprintf("dev:%s, pino:%d, name:%s, ino:%d, err:%d", devParen, intByCleanName(ev, "index", false), stringByCleanName(ev, content, "name"), intByCleanName(ev, "cino", false), intByCleanName(ev, "res", false)), true
	case strings.HasPrefix(name, "erofs_getattr_enter") || strings.HasPrefix(name, "erofs_getattr_exit"):
		return fmt.Sprintf("dev:%s, ino:%d, mode:0x%x, size:%d, blocks:%d, linkcnt:%d", devParen, intByCleanName(ev, "index", false), intByCleanName(ev, "mode", false), intByCleanName(ev, "size", false), intByCleanName(ev, "blocks", false), intByCleanName(ev, "nlink", false)), true
	case strings.HasPrefix(name, "erofs_listxattr_enter"):
		return fmt.Sprintf("dev:%s, ino:%d, mode:0x%x, xattr_nid:%d, size:%d", devParen, intByCleanName(ev, "index", false), intByCleanName(ev, "mode", false), intByCleanName(ev, "xattr_nid", false), intByCleanName(ev, "size", false)), true
	case strings.HasPrefix(name, "erofs_listxattr_exit"):
		return fmt.Sprintf("dev:%s, ino:%d, mode:0x%x, size:%d, blocks:%d, linkcnt:%d, err:%d", devParen, intByCleanName(ev, "index", false), intByCleanName(ev, "mode", false), intByCleanName(ev, "size", false), intByCleanName(ev, "blocks", false), intByCleanName(ev, "nlink", false), intByCleanName(ev, "res", false)), true
	case strings.HasPrefix(name, "erofs_raw_access_readpages_start") || strings.HasPrefix(name, "z_erofs_vle_normalaccess_readpages_start"):
		return fmt.Sprintf("index:%d nr_pages:%d nid:%d", intByCleanName(ev, "index", false), intByCleanName(ev, "nr_pages", false), intByCleanName(ev, "nid", false)), true
	case strings.HasPrefix(name, "erofs_raw_access_readpages_end") || strings.HasPrefix(name, "erofs_read_raw_page_end") || strings.HasPrefix(name, "z_erofs_vle_normalaccess_readpage_end") || strings.HasPrefix(name, "z_erofs_vle_normalaccess_readpages_end"):
		return fmt.Sprintf("nid:%d res:%d", intByCleanName(ev, "nid", false), intByCleanName(ev, "res", false)), true
	case strings.HasPrefix(name, "erofs_read_raw_page_start") || strings.HasPrefix(name, "z_erofs_vle_normalaccess_readpage_start"):
		return fmt.Sprintf("index:%d nid:%d", intByCleanName(ev, "index", false), intByCleanName(ev, "nid", false)), true
	}
	return "", false
}

func intByCleanName(ev decodedEvent, want string, signed bool) int64 {
	for _, f := range ev.format.Fields {
		if cleanFieldName(f.Name) != want {
			continue
		}
		if b, ok := ev.fields[f.Name]; ok {
			return intFromBytes(b, signed || f.Signed)
		}
	}
	return 0
}

func stringByCleanName(ev decodedEvent, content []byte, names ...string) string {
	for _, want := range names {
		if s := dataLocStringByCleanName(ev, content, want); s != "" {
			return s
		}
		if f, _, ok := fieldByCleanName(ev, want); ok {
			lowerType := strings.ToLower(f.Type)
			if lowerType == "" || strings.Contains(lowerType, "char") || strings.Contains(lowerType, "string") {
				if s := strField(ev, f.Name); s != "" {
					return s
				}
			}
		}
		if off := intByCleanName(ev, want, false) & 0xffff; off > 0 {
			if s := stringFromOffset(content, int(off)); s != "" {
				return s
			}
		}
	}
	return ""
}

func fieldBytesByCleanName(ev decodedEvent, want string) []byte {
	for _, f := range ev.format.Fields {
		if cleanFieldName(f.Name) == want {
			return ev.fields[f.Name]
		}
	}
	return nil
}

func dynamicBytesByCleanName(ev decodedEvent, content []byte, want string, maxLen int) []byte {
	for _, f := range ev.format.Fields {
		if cleanFieldName(f.Name) != want {
			continue
		}
		if dataLocField(f) {
			loc := uint32(intField(ev, f.Name, false))
			off := int(loc & 0xffff)
			ln := int(loc >> 16)
			if maxLen > 0 && (ln == 0 || ln > maxLen) {
				ln = maxLen
			}
			if off >= 0 && off < len(content) {
				end := off + ln
				if end > len(content) {
					end = len(content)
				}
				if end > off {
					return content[off:end]
				}
			}
		}
		b := ev.fields[f.Name]
		if maxLen > 0 && len(b) > maxLen {
			b = b[:maxLen]
		}
		return b
	}
	return nil
}

func stringFromOffset(content []byte, off int) string {
	if off < 0 || off >= len(content) {
		return ""
	}
	b := content[off:]
	if i := bytesIndexNUL(b); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

func devByCleanName(ev decodedEvent, want string, sep string) string {
	return devMajorMinor(intByCleanName(ev, want, false), sep)
}

func softirqAction(vec int64) string {
	names := map[int64]string{0: "HI", 1: "TIMER", 2: "NET_TX", 3: "NET_RX", 4: "BLOCK", 5: "IRQ_POLL", 6: "TASKLET", 7: "SCHED", 8: "HRTIMER", 9: "RCU"}
	return names[vec]
}

func ufsOpcodeName(opcode int64) string {
	names := map[int64]string{0x8a: "WRITE_16", 0x2a: "WRITE_10", 0x88: "READ_16", 0x28: "READ_10", 0x35: "SYNC", 0x42: "UNMAP"}
	return names[opcode]
}

func ufsDevState(v int64) string {
	names := map[int64]string{1: "UFS_ACTIVE_PWR_MODE", 2: "UFS_SLEEP_PWR_MODE", 3: "UFS_POWERDOWN_PWR_MODE"}
	return names[v]
}

func ufsLinkState(v int64) string {
	names := map[int64]string{0: "UIC_LINK_OFF_STATE", 1: "UIC_LINK_ACTIVE_STATE", 2: "UIC_LINK_HIBERN8_STATE"}
	return names[v]
}

func ufsClkGatingState(v int64) string {
	names := map[int64]string{0: "CLKS_OFF", 1: "CLKS_ON", 2: "REQ_CLKS_OFF", 3: "REQ_CLKS_ON"}
	return names[v]
}

func littleEndianBytesHex(b []byte) string {
	if len(b) == 0 {
		return "0"
	}
	rev := make([]byte, len(b))
	for i := range b {
		rev[len(b)-1-i] = b[i]
	}
	n := new(big.Int).SetBytes(rev)
	return n.Text(16)
}

func decimalByteArray(b []byte) string {
	parts := make([]string, 0, len(b))
	for _, v := range b {
		parts = append(parts, fmt.Sprintf("%d", v))
	}
	return strings.Join(parts, ", ")
}
