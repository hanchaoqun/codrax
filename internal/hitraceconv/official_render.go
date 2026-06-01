package hitraceconv

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

func renderOfficialOpenHarmonyBody(ev decodedEvent, content []byte) (string, bool) {
	name := ev.format.Name
	switch {
	case name == "sched_switch" && hasCleanField(ev, "prev_comm"):
		return fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d expeller_type=%d",
			stringByCleanName(ev, content, "prev_comm"), intByCleanName(ev, "prev_pid", true), intByCleanName(ev, "prev_prio", true),
			linuxPrevState(uint64(intByCleanName(ev, "prev_state", true))), stringByCleanName(ev, content, "next_comm"),
			intByCleanName(ev, "next_pid", true), intByCleanName(ev, "next_prio", true), intByCleanName(ev, "expeller_type", false)), true
	case name == "sched_switch" && hasCleanField(ev, "pname"):
		body := fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
			stringByCleanName(ev, content, "pname"), intByCleanName(ev, "prev_tid", true), intByCleanName(ev, "pprio", true),
			harmonyPrevState(uint64(intByCleanName(ev, "pstate", false))), stringByCleanName(ev, content, "nname"),
			intByCleanName(ev, "next_tid", true), intByCleanName(ev, "nprio", true))
		if hasCleanField(ev, "ninfo") {
			body += " next_info=" + harmonySchedInfo(ev)
		}
		if cg := stringByCleanName(ev, content, "cg", "cgroup"); cg != "" {
			body += " cg=" + cg
		}
		return body, true
	case name == "sched_wakeup" || name == "sched_waking":
		return fmt.Sprintf("comm=%s pid=%d prio=%d target_cpu=%03d",
			firstNonEmpty(stringByCleanName(ev, content, "pname"), stringByCleanName(ev, content, "comm")),
			intByCleanName(ev, "pid", true), intByCleanName(ev, "prio", true), intByCleanName(ev, "target_cpu", true)), true
	case name == "sched_blocked_reason":
		if hasCleanField(ev, "func_name") {
			return fmt.Sprintf("pid=%d iowait=%d caller=%s+0x%x/0x%x[%s] delay=%d",
				intByCleanName(ev, "pid", true), intByCleanName(ev, "iowait", false), stringByCleanName(ev, content, "func_name"),
				intByCleanName(ev, "offset", false), intByCleanName(ev, "size", false), stringByCleanName(ev, content, "mod_name"),
				intByCleanName(ev, "delay", false)>>10), true
		}
		if hasCleanField(ev, "cnode_idx") {
			return fmt.Sprintf("pid=%d iowait=%d caller=0x%x cnode_idx=%d delay=%d",
				intByCleanName(ev, "pid", true), intByCleanName(ev, "iowait", false), intByCleanName(ev, "caller", false),
				intByCleanName(ev, "cnode_idx", false), intByCleanName(ev, "delay", false)>>10), true
		}
		return fmt.Sprintf("pid=%d iowait=%d caller=0x%x delay=%d",
			intByCleanName(ev, "pid", true), firstNonZero(intByCleanName(ev, "io_wait", false), intByCleanName(ev, "iowait", false)),
			intByCleanName(ev, "caller", false), intByCleanName(ev, "delay", false)>>10), true
	case name == "cpu_frequency" || name == "cpu_idle":
		return fmt.Sprintf("state=%d cpu_id=%d", intByCleanName(ev, "state", false), intByCleanName(ev, "cpu_id", false)), true
	case name == "cpu_frequency_limits":
		minFreq := firstNonZero(intByCleanName(ev, "min", false), intByCleanName(ev, "min_freq", false))
		maxFreq := firstNonZero(intByCleanName(ev, "max", false), intByCleanName(ev, "max_freq", false))
		return fmt.Sprintf("min=%d max=%d cpu_id=%d", minFreq, maxFreq, intByCleanName(ev, "cpu_id", false)), true
	case name == "clock_set_rate":
		return fmt.Sprintf("%s state=%d cpu_id=%d", stringByCleanName(ev, content, "name"), intByCleanName(ev, "state", false), intByCleanName(ev, "cpu_id", false)), true
	case name == "softirq_entry" || name == "softirq_exit" || (strings.Contains(name, "softirq") && hasCleanField(ev, "vec")):
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
	case name == "block_bio_remap":
		return renderBlockRemap(ev), true
	case name == "block_rq_issue" || name == "block_rq_insert" || name == "block_rq_complete":
		return renderBlockRequest(ev, content), true
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
	case strings.HasPrefix(name, "i2c_read"):
		return fmt.Sprintf("i2c-%d #%d a=%03x f=%04x l=%d", intByCleanName(ev, "adapter_nr", true),
			intByCleanName(ev, "msg_nr", false), intByCleanName(ev, "addr", false), intByCleanName(ev, "flags", false),
			intByCleanName(ev, "len", false)), true
	case strings.HasPrefix(name, "i2c_write") || strings.HasPrefix(name, "i2c_reply"):
		ln := int(intByCleanName(ev, "len", false))
		return fmt.Sprintf("i2c-%d #%d a=%03x f=%04x l=%d %s", intByCleanName(ev, "adapter_nr", true),
			intByCleanName(ev, "msg_nr", false), intByCleanName(ev, "addr", false), intByCleanName(ev, "flags", false),
			ln, dashHexWithOfficialTrailingBracket(dynamicBytesByCleanName(ev, content, "buf", ln), ln)), true
	case strings.HasPrefix(name, "i2c_result"):
		return fmt.Sprintf("i2c-%d n=%d ret=%d", intByCleanName(ev, "adapter_nr", true),
			intByCleanName(ev, "nr_msgs", false), intByCleanName(ev, "ret", true)), true
	case strings.HasPrefix(name, "smbus_read"):
		return fmt.Sprintf("i2c-%d a=%03x f=%04x c=%x %s", intByCleanName(ev, "adapter_nr", true),
			intByCleanName(ev, "addr", false), intByCleanName(ev, "flags", false), intByCleanName(ev, "command", false),
			smbusProtocol(intByCleanName(ev, "protocol", false))), true
	case strings.HasPrefix(name, "smbus_write") || strings.HasPrefix(name, "smbus_reply"):
		ln := int(intByCleanName(ev, "len", false))
		buf := stringByCleanName(ev, content, "buf")
		if buf == "" {
			buf = dashHexWithOfficialTrailingBracket(dynamicBytesByCleanName(ev, content, "buf", ln), ln)
		}
		return fmt.Sprintf("i2c-%d a=%03x f=%04x c=%x %s l=%d %s", intByCleanName(ev, "adapter_nr", true),
			intByCleanName(ev, "addr", false), intByCleanName(ev, "flags", false), intByCleanName(ev, "command", false),
			smbusProtocol(intByCleanName(ev, "protocol", false)), ln, buf), true
	case strings.HasPrefix(name, "smbus_result"):
		rw := "rd"
		if intByCleanName(ev, "read_write", false) == 0 {
			rw = "wr"
		}
		return fmt.Sprintf("i2c-%d a=%03x f=%04x c=%x %s %s res=%d", intByCleanName(ev, "adapter_nr", true),
			intByCleanName(ev, "addr", false), intByCleanName(ev, "flags", false), intByCleanName(ev, "command", false),
			smbusProtocol(intByCleanName(ev, "protocol", false)), rw, intByCleanName(ev, "res", true)), true
	case strings.HasPrefix(name, "regulator_set_voltage_complete"):
		return fmt.Sprintf("name=%s, val=%d", stringByCleanName(ev, content, "name"), intByCleanName(ev, "val", false)), true
	case strings.HasPrefix(name, "regulator_set_voltage"):
		return fmt.Sprintf("name=%s (%d-%d)", stringByCleanName(ev, content, "name"), intByCleanName(ev, "min", true), intByCleanName(ev, "max", true)), true
	case strings.HasPrefix(name, "regulator_"):
		return fmt.Sprintf("name=%s", stringByCleanName(ev, content, "name")), true
	case strings.HasPrefix(name, "dma_fence"):
		return fmt.Sprintf("driver=%s timeline=%s context=%d seqno=%d", stringByCleanName(ev, content, "driver"),
			stringByCleanName(ev, content, "timeline"), intByCleanName(ev, "context", false), intByCleanName(ev, "seqno", false)), true
	case strings.HasPrefix(name, "mmc_request_start"):
		return renderMMCRequestStart(ev, content), true
	case strings.HasPrefix(name, "mmc_request_done"):
		return renderMMCRequestDone(ev, content), true
	case strings.HasPrefix(name, "file_check_and_advance_wb_err"):
		return fmt.Sprintf("file=0x%x dev=%s ino=0x%x old=0x%x new=0x%x", intByCleanName(ev, "file", false),
			devByCleanName(ev, "s_dev", ":"), intByCleanName(ev, "i_ino", false), intByCleanName(ev, "old", false),
			intByCleanName(ev, "new", false)), true
	case strings.HasPrefix(name, "rss_stat"):
		return fmt.Sprintf("mm_id=%d curr=%d member=%d size=%dB", intByCleanName(ev, "mm_id", false),
			intByCleanName(ev, "curr", false), intByCleanName(ev, "member", true), intByCleanName(ev, "size", true)), true
	case strings.HasPrefix(name, "workqueue_execute"):
		return fmt.Sprintf("work struct 0x%x: function 0x%x", intByCleanName(ev, "work", false), intByCleanName(ev, "function", false)), true
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
	case name == "print":
		return fmt.Sprintf("0x%x: %s", intByCleanName(ev, "ip", false), stringFromOffset(content, 16)), true
	case name == "tracing_mark_write" && hasCleanField(ev, "start") && hasCleanField(ev, "pid"):
		if intByCleanName(ev, "start", false) == 1 {
			return fmt.Sprintf("B|%d|%s", intByCleanName(ev, "pid", false), stringByCleanName(ev, content, "name")), true
		}
		return fmt.Sprintf("E|%d|", intByCleanName(ev, "pid", false)), true
	case name == "tracing_mark_write":
		s := stringByCleanName(ev, content, "buffer", "buf", "str")
		if strings.HasPrefix(s, "E|") && strings.HasSuffix(s, "|") {
			s = strings.TrimSuffix(s, "|")
		}
		return s, true
	case name == "tracing_mark_write_xacct" || name == "xacct_tracing_mark_write":
		if intByCleanName(ev, "start", false) == 1 {
			return fmt.Sprintf("B|%d|%s", intByCleanName(ev, "pid", false), stringByCleanName(ev, content, "name")), true
		}
		return fmt.Sprintf("E|%d|", intByCleanName(ev, "pid", false)), true
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

func renderMMCRequestStart(ev decodedEvent, content []byte) string {
	return fmt.Sprintf("%s: start struct mmc_request[0x%x]: cmd_opcode=%d cmd_arg=0x%x cmd_flags=0x%x cmd_retries=%d stop_opcode=%d stop_arg=0x%x stop_flags=0x%x stop_retries=%d sbc_opcode=%d sbc_arg=0x%x sbc_flags=0x%x sbc_retires=%d blocks=%d block_size=%d blk_addr=%d data_flags=0x%x tag=%d can_retune=%d doing_retune=%d retune_now=%d need_retune=%d hold_retune=%d retune_period=%d",
		stringByCleanName(ev, content, "name"), intByCleanName(ev, "mrq", false), intByCleanName(ev, "cmd_opcode", false),
		intByCleanName(ev, "cmd_arg", false), intByCleanName(ev, "cmd_flags", false), intByCleanName(ev, "cmd_retries", false),
		intByCleanName(ev, "stop_opcode", false), intByCleanName(ev, "stop_arg", false), intByCleanName(ev, "stop_flags", false),
		intByCleanName(ev, "stop_retries", false), intByCleanName(ev, "sbc_opcode", false), intByCleanName(ev, "sbc_arg", false),
		intByCleanName(ev, "sbc_flags", false), intByCleanName(ev, "sbc_retries", false), intByCleanName(ev, "blocks", false),
		intByCleanName(ev, "blksz", false), intByCleanName(ev, "blk_addr", false), intByCleanName(ev, "data_flags", false),
		intByCleanName(ev, "tag", true), intByCleanName(ev, "can_retune", false), intByCleanName(ev, "doing_retune", false),
		intByCleanName(ev, "retune_now", false), intByCleanName(ev, "need_retune", false), intByCleanName(ev, "hold_retune", true),
		intByCleanName(ev, "retune_period", true))
}

func renderMMCRequestDone(ev decodedEvent, content []byte) string {
	cmdResp := fieldBytesByCleanName(ev, "cmd_resp")
	stopResp := fieldBytesByCleanName(ev, "stop_resp")
	sbcResp := fieldBytesByCleanName(ev, "sbc_resp")
	return fmt.Sprintf("%s: end struct mmc_request[0x%x]: cmd_opcode=%d cmd_err=%d cmd_resp=0x%x 0x%x 0x%x 0x%x cmd_retries=%d stop_opcode=%d stop_err=%d stop_resp=0x%x 0x%x 0x%x 0x%x stop_retries=%d sbc_opcode=%d sbc_err=%d sbc_resp=0x%x 0x%x 0x%x 0x%x sbc_retries=%d bytes_xfered=%d data_err=%d tag=%d can_retune=%d doing_retune=%d retune_now=%d need_retune=%d hold_retune=%d retune_period=%d",
		stringByCleanName(ev, content, "name"), intByCleanName(ev, "mrq", false), intByCleanName(ev, "cmd_opcode", false),
		intByCleanName(ev, "cmd_err", true), byteAt(cmdResp, 0), byteAt(cmdResp, 1), byteAt(cmdResp, 2), byteAt(cmdResp, 3),
		intByCleanName(ev, "cmd_retries", false), intByCleanName(ev, "stop_opcode", false), intByCleanName(ev, "stop_err", true),
		byteAt(stopResp, 0), byteAt(stopResp, 1), byteAt(stopResp, 2), byteAt(stopResp, 3), intByCleanName(ev, "stop_retries", false),
		intByCleanName(ev, "sbc_opcode", false), intByCleanName(ev, "sbc_err", true), byteAt(sbcResp, 0), byteAt(sbcResp, 1),
		byteAt(sbcResp, 2), byteAt(sbcResp, 3), intByCleanName(ev, "sbc_retries", false), intByCleanName(ev, "bytes_xfered", false),
		intByCleanName(ev, "data_err", true), intByCleanName(ev, "tag", true), intByCleanName(ev, "can_retune", false),
		intByCleanName(ev, "doing_retune", false), intByCleanName(ev, "retune_now", false), intByCleanName(ev, "need_retune", true),
		intByCleanName(ev, "hold_retune", true), intByCleanName(ev, "retune_period", false))
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
		if s := strFieldByCleanName(ev, want); s != "" {
			return s
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

func smbusProtocol(v int64) string {
	names := map[int64]string{0: "QUICK", 1: "BYTE", 2: "BYTE_DATA", 3: "WORD_DATA", 4: "PROC_CALL", 5: "BLOCK_DATA", 6: "I2C_BLOCK_BROKEN", 7: "BLOCK_PROC_CALL", 8: "I2C_BLOCK_DATA"}
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

func dashHexWithOfficialTrailingBracket(b []byte, ln int) string {
	if ln <= 0 {
		return "]"
	}
	if len(b) > ln {
		b = b[:ln]
	}
	parts := make([]string, 0, ln)
	for i := 0; i < ln; i++ {
		var v byte
		if i < len(b) {
			v = b[i]
		}
		parts = append(parts, hex.EncodeToString([]byte{v}))
	}
	return strings.Join(parts, "-") + "]"
}

func decimalByteArray(b []byte) string {
	parts := make([]string, 0, len(b))
	for _, v := range b {
		parts = append(parts, fmt.Sprintf("%d", v))
	}
	return strings.Join(parts, ", ")
}

func byteAt(b []byte, i int) byte {
	if i < 0 || i >= len(b) {
		return 0
	}
	return b[i]
}
