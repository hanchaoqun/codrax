package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleTrace = `
      waker-10   (   10) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=10 next_prio=20
      waker-10   (   10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
      waker-10   (   10) [000] .... 1.060000: sched_switch: prev_comm=waker prev_pid=10 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
        app-20   (   20) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-10   (   10) [000] .... 1.180000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
        app-20   (   20) [001] .... 1.220000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
        app-20   (   20) [001] .... 1.260000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-10   (   10) [000] .... 1.300000: cpu_idle: state=4294967295 cpu_id=0
      waker-10   (   10) [000] .... 1.310000: clock_set_rate: pid_freq state=1200000 cpu_id=0
      waker-10   (   10) [000] .... 1.320000: block_rq_issue: 8,0 R 4096 () 123 + 8 [waker]
`

const resourceTrace = `
       main-20   (   20) [001] .... 2.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=main next_pid=20 next_prio=53
       main-20   (   20) [001] .... 2.010000: sched_switch: prev_comm=main prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=worker next_pid=30 next_prio=20
     worker-30   (   30) [001] .... 2.030000: sched_switch: prev_comm=worker prev_pid=30 prev_prio=20 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-10   (   10) [000] .... 2.040000: sched_blocked_reason: pid=30 iowait=1 caller=fscache_page_wait_on_page_bit
      waker-10   (   10) [000] .... 2.050000: cpu_idle: state=4294967295 cpu_id=0
      waker-10   (   10) [000] .... 2.060000: clock_set_rate: pid_freq state=1800000 cpu_id=0
      waker-10   (   10) [000] .... 2.070000: block_rq_issue: 8,0 R 4096 () 123 + 8 [worker]
      waker-10   (   10) [000] .... 2.080000: block_rq_complete: 8,0 R () 123 + 8 [0]
      waker-10   (   10) [000] .... 2.090000: binder_transaction: transaction=7 dest_node=0 dest_proc=40 dest_thread=41 reply=1 flags=0x0 code=0x1
      waker-10   (   10) [000] .... 2.095000: binder_transaction_received: transaction=7
      waker-10   (   10) [000] .... 2.100000: irq_handler_entry: irq=32 name=kirq
      waker-10   (   10) [000] .... 2.110000: mm_vmscan_direct_reclaim_begin: order=0 may_writepage=1
      waker-10   (   10) [000] .... 2.120000: print: B|20|Choreographer#doFrame
     worker-30   (   30) [001] .... 2.150000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=30 next_prio=20
       main-20   (   20) [001] .... 2.200000: sched_switch: prev_comm=worker prev_pid=30 prev_prio=20 prev_state=S ==> next_comm=main next_pid=20 next_prio=53
`

const ipcTrace = `
     client-20   (   20) [001] .... 3.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=20 next_prio=53
     client-20   (   20) [001] .... 3.010000: binder_transaction: transaction=42 dest_node=0 dest_proc=100 dest_thread=101 reply=1 flags=0x0 code=0x3
     client-20   (   20) [001] .... 3.010500: binder_transaction_alloc_buf: debug_id=42 data_size=128 offsets_size=16 extra_buffers_size=0
     client-20   (   20) [001] .... 3.011000: binder_transaction_lock: tag=binder_inner_lock
     client-20   (   20) [001] .... 3.011500: binder_transaction_unlock: tag=binder_inner_lock
 binder:100_1-101 (  100) [002] .... 3.012000: binder_transaction_received: transaction=42
     client-20   (   20) [001] .... 3.015000: sched_switch: prev_comm=client prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
 binder:100_1-101 (  100) [002] .... 3.020000: sched_wakeup: comm=client pid=20 prio=53 target_cpu=001
     client-20   (   20) [001] .... 3.030000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=20 next_prio=53
`

const frequencyTrace = `
 arch_disk_io_2-33642 (33566) [001] .... 2940.180000: cpu_frequency: state=1800000 cpu_id=11
 arch_disk_io_2-33642 (33566) [001] .... 2940.190402: cpu_frequency: state=2228000 cpu_id=11
 arch_disk_io_2-33642 (33566) [001] .... 2940.190402: clock_set_rate: heca_info state=87047 cpu_id=11
	com.tencent.mm-36379 (36379) [010] .... 2940.190402: mm_filemap_add_to_page_cache: dev 260:84 ino 0x60ffe page=0000000000000000 pfn=3062260 ofs=0
 arch_disk_io_2-33642 (33566) [001] .... 2940.190402: clock_set_rate: heca_ddr_freq state=3744 cpu_id=0
 binder:31963_1-37072 (37047) [001] .... 2941.665123: cpu_frequency: state=1600000 cpu_id=0
 binder:31963_1-37072 (37047) [001] .... 2941.675123: cpu_frequency: state=1800000 cpu_id=0
`

const harmonyTrace = `
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081774: block_rq_issue: 12,48 RS 4096 () 66637712 + 8 [OS_FFRT_0_0]
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081786: mm_filemap_add_to_page_cache: dev 260:84 ino 0x1 page=0000000000000000 pfn=2477336 ofs=1211162624
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081795: block_bio_remap: 12,48  66637568 + 8 <- (260,84) 14962432
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081798: block_rq_issue: 12,48 RS 4096 () 66637568 + 8 [OS_FFRT_0_0]
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081847: irq_handler_entry: irq=1507 name=kirq
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081851: sched_wakeup: comm=udk-irq-0 pid=73 prio=301 target_cpu=000
       udk-irq-0-73    (    2) [000] .... 928.081861: sched_switch: prev_comm=OS_FFRT_0_0 prev_pid=49634 prev_prio=20 prev_state=R+ ==> next_comm=udk-irq-0 next_pid=73 next_prio=301
       udk-irq-0-73    (    2) [000] .... 928.081873: block_rq_complete: 12,48 RS () 66637712 + 8 [0]
       udk-irq-0-73    (    2) [000] .... 928.081896: irq_handler_exit: irq=1507 ret=handled
     OS_FFRT_0_0-49634 (48679) [000] .... 928.081903: sched_switch: prev_comm=udk-irq-0 prev_pid=73 prev_prio=301 prev_state=S ==> next_comm=OS_FFRT_0_0 next_pid=49634 next_prio=20
`

const p1ResourceTrace = `
       main-20   (   20) [001] .... 4.000000: print: B|20|Choreographer#doFrame
       main-20   (   20) [001] .... 4.010000: print: C|20|JNI Weak Global Refs|198
       main-20   (   20) [001] .... 4.012000: cpu_frequency_limits: min=500000 max=1500000 cpu_id=1
       main-20   (   20) [001] .... 4.015000: irq_handler_entry: irq=32 name=kirq
       main-20   (   20) [001] .... 4.015400: irq_handler_exit: irq=32 ret=handled
       main-20   (   20) [001] .... 4.015700: irq_handler_entry: irq=32 name=kirq
       main-20   (   20) [001] .... 4.015800: softirq_entry: vec=3 action=NET_RX
       main-20   (   20) [001] .... 4.016000: mm_filemap_add_to_page_cache: dev 260:84 ino 0x1 page=0 pfn=1 ofs=0
       main-20   (   20) [001] .... 4.016500: ext4_sync_file_enter: dev 8,0 ino 42 parent 1 datasync 0
       main-20   (   20) [001] .... 4.016700: ufshcd_command: tag=1 opcode=0x28 doorbell=0x1
       main-20   (   20) [001] .... 4.017000: mm_vmscan_direct_reclaim_begin: order=0 may_writepage=1
       main-20   (   20) [001] .... 4.018000: thermal_power_allocator: actor=cpu power=300
       main-20   (   20) [001] .... 4.018500: workqueue_execute_start: work struct=0000000000000000 function=do_work
       main-20   (   20) [001] .... 4.019000: dma_fence_wait_start: driver=display timeline=present seqno=7
       main-20   (   20) [001] .... 4.020000: print: E|20
      other-20   (   21) [002] .... 4.025000: sched_wakeup: comm=main pid=20 prio=53 target_cpu=001
`

const schedulerLatencyTrace = `
        app-20   (   20) [001] .... 5.000000: cpu_frequency: state=1000000 cpu_id=1
      rival-30   (   30) [001] .... 5.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=rival next_pid=30 next_prio=80
      rival-30   (   30) [001] .... 5.090000: sched_switch: prev_comm=rival prev_pid=30 prev_prio=80 prev_state=R+ ==> next_comm=app next_pid=20 next_prio=53
        app-20   (   20) [001] .... 5.100000: cpu_frequency: state=2200000 cpu_id=1
        app-20   (   20) [001] .... 5.110000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=rival next_pid=30 next_prio=80
      rival-30   (   30) [001] .... 5.140000: sched_switch: prev_comm=rival prev_pid=30 prev_prio=80 prev_state=S ==> next_comm=app next_pid=20 next_prio=53
`

const blockingTrace = `
        app-20   (   20) [001] .... 6.000000: print: B|20|Choreographer#doFrame
        app-20   (   20) [001] .... 6.010000: print: B|20|Lock contention on InternTable lock
        app-20   (   20) [001] .... 6.040000: print: E|20
        app-20   (   20) [001] .... 6.050000: print: E|20
        app-20   (   20) [001] .... 6.060000: block_rq_issue: 8,0 R 4096 () 333 + 8 [app]
      irq-irq-9   (    2) [000] .... 6.080000: block_rq_complete: 8,0 R () 333 + 8 [0]
        app-20   (   20) [001] .... 6.090000: sched_blocked_reason: pid=20 iowait=1 caller=futex_wait_queue
`

const frameFlowTrace = `
         app-20   (   20) [001] .... 7.000000: print: B|20|Expected Timeline frame=77
         app-20   (   20) [001] .... 7.004000: print: E|20
         app-20   (   20) [001] .... 7.005000: print: B|20|Choreographer#doFrame frame=77
         app-20   (   20) [001] .... 7.016000: print: E|20
 RSUniRenderThre-2096 (1716) [000] .... 7.017000: print: B|1716|H:RenderFrame frame=77
 RSUniRenderThre-2096 (1716) [000] .... 7.030000: print: E|1716
         gpu-300   (  300) [002] .... 7.031000: print: B|300|GPU completion frame=77
         gpu-300   (  300) [002] .... 7.040000: print: E|300
`

const ebpfResourceTrace = `
        app-20   (   20) [001] .... 8.000000: bio_latency: op=R path=/data/app/base.db latency_us=2500 bytes=4096 callstack=BioRead>Submit
        app-20   (   20) [001] .... 8.010000: file_system: syscall=read path=/data/app/base.db duration_ms=3.5 bytes=1024 callstack=ReadFile
        app-20   (   20) [001] .... 8.020000: page_fault_user: operation=major address=0x1234 duration_us=150 size=4096 callstack=FaultHandler
`

const pluginResourceTrace = `
        app-20   (   20) [001] .... 9.000000: ability_monitor: domain=AAFWK event_name=AbilityStart metric=latency_ms value=12.5 category=foreground
     xpower-30   (   30) [002] .... 9.010000: xpower_cpu: component=CPU energy=8.2 usage=73 scene=foreground
       hisys-40   (   40) [003] .... 9.020000: hi_sysevent: domain=POWER eventname=THERMAL_REPORT type=STAT value=hot level=MINOR
`

const coreTopologyTrace = `
      freq-1   (    1) [000] .... 10.000000: cpu_frequency: state=800000 cpu_id=0
      freq-1   (    1) [000] .... 10.000000: cpu_frequency: state=2200000 cpu_id=4
        app-20 (   20) [004] .... 10.010000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=65535 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
        app-20 (   20) [004] .... 10.050000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=worker next_pid=30 next_prio=80
`

func TestParseLineSchedulerEvents(t *testing.T) {
	intern := newStringInterner()
	ev, ok := ParseLine(4, `        app-20   (   20) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`, intern)
	if !ok {
		t.Fatal("ParseLine did not parse sched_switch")
	}
	if ev.Type != EventSchedSwitch || ev.PrevPID != 20 || ev.PrevState != "S" || ev.NextPID != 0 || ev.CPU != 1 {
		t.Fatalf("unexpected event: %+v", ev)
	}
	ev, ok = ParseLine(6, `        app-20   (   20) [001] .... 1.120000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=worker next_pid=30 next_prio=80 next_info=rtq cg=top-app`, intern)
	if !ok || ev.Type != EventSchedSwitch || ev.NextInfo != "rtq" || ev.CGroup != "top-app" {
		t.Fatalf("sched_switch variant fields not preserved: %+v ok=%v", ev, ok)
	}
	wake, ok := ParseLine(5, `      waker-10   (   10) [000] .... 1.180000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`, intern)
	if !ok || wake.Type != EventSchedWakeup || wake.PID != 10 || wake.WakeePID != 20 || wake.TargetCPU != 1 {
		t.Fatalf("unexpected wake event: %+v ok=%v", wake, ok)
	}
}

func TestParseLineAcceptsPerfettoUnknownTGIDAndMissingTGID(t *testing.T) {
	intern := newStringInterner()
	ev, ok := ParseLine(1, `          <idle>-0     (-----) [006] ....    10.258854: sched_switch: prev_comm=swapper/6 prev_pid=0 prev_prio=120 prev_state=D ==> next_comm=foo next_pid=269 next_prio=130`, intern)
	if !ok || ev.Type != EventSchedSwitch || ev.TGID != 0 || ev.NextPID != 269 || ev.CPU != 6 {
		t.Fatalf("expected Perfetto (-----) TGID row to parse, got %+v ok=%v", ev, ok)
	}
	ev, ok = ParseLine(2, `          <idle>-0     [001] d..2  1234.000001: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=real_name next_pid=19999 next_prio=120`, intern)
	if !ok || ev.Type != EventSchedSwitch || ev.NextPID != 19999 || ev.CPU != 1 {
		t.Fatalf("expected no-TGID systrace row to parse, got %+v ok=%v", ev, ok)
	}
}

func TestParseLineSupportedResourceEvents(t *testing.T) {
	intern := newStringInterner()
	cases := []struct {
		name  string
		line  string
		want  EventType
		check func(Event) bool
	}{
		{
			name: "blocked reason",
			line: `      waker-10   (   10) [000] .... 2.040000: sched_blocked_reason: pid=30 iowait=1 caller=fscache_page_wait_on_page_bit`,
			want: EventSchedBlockedReason,
			check: func(ev Event) bool {
				return ev.WakeePID == 30 && ev.IOWait == 1 && ev.Reason == "fscache_page_wait_on_page_bit"
			},
		},
		{
			name:  "cpu idle",
			line:  `      waker-10   (   10) [000] .... 2.050000: cpu_idle: state=4294967295 cpu_id=0`,
			want:  EventCPUIdle,
			check: func(ev Event) bool { return ev.CPUForField == 0 && ev.CPUForFieldValid },
		},
		{
			name:  "cpu frequency",
			line:  `      waker-10   (   10) [000] .... 2.060000: clock_set_rate: pid_freq state=1800000 cpu_id=0`,
			want:  EventCPUFrequency,
			check: func(ev Event) bool { return ev.Frequency == 1800000 && ev.ClockName == "pid_freq" },
		},
		{
			name: "cpu frequency limit",
			line: `      waker-10   (   10) [000] .... 2.060500: cpu_frequency_limits: min=500000 max=1500000 cpu_id=6`,
			want: EventCPUFrequencyLimit,
			check: func(ev Event) bool {
				return ev.FrequencyMin == 500000 && ev.FrequencyMax == 1500000 && ev.CPUForField == 6 && ev.SubsystemKind == "cpu_frequency_limits"
			},
		},
		{
			name:  "cpu_frequency tracepoint",
			line:  `      waker-10   (   10) [000] .... 2.061000: cpu_frequency: state=1600000 cpu_id=0`,
			want:  EventCPUFrequency,
			check: func(ev Event) bool { return ev.Frequency == 1600000 && ev.ClockName == "cpu_frequency" },
		},
		{
			name:  "non CPU clock set rate",
			line:  `      waker-10   (   10) [000] .... 2.062000: clock_set_rate: heca_ddr_freq state=3744 cpu_id=0`,
			want:  EventClockSetRate,
			check: func(ev Event) bool { return ev.Frequency == 3744 && ev.ClockName == "heca_ddr_freq" },
		},
		{
			name: "block issue",
			line: `      waker-10   (   10) [000] .... 2.070000: block_rq_issue: 8,0 R 4096 () 123 + 8 [worker]`,
			want: EventBlockIssue,
			check: func(ev Event) bool {
				return ev.BlockDev == "8,0" && ev.BlockOp == "R" && ev.BlockSector == 123 && ev.BlockLen == 8
			},
		},
		{
			name: "block complete",
			line: `      waker-10   (   10) [000] .... 2.080000: block_rq_complete: 8,0 R () 123 + 8 [0]`,
			want: EventBlockComplete,
			check: func(ev Event) bool {
				return ev.BlockDev == "8,0" && ev.BlockOp == "R" && ev.BlockSector == 123 && ev.BlockLen == 8
			},
		},
		{
			name:  "block remap",
			line:  `      waker-10   (   10) [000] .... 2.085000: block_bio_remap: 12,48  66637568 + 8 <- (260,84) 14962432`,
			want:  EventBlockRemap,
			check: func(ev Event) bool { return ev.BlockDev == "12,48" && ev.BlockSector == 66637568 && ev.BlockLen == 8 },
		},
		{
			name: "binder",
			line: `      waker-10   (   10) [000] .... 2.090000: binder_transaction: transaction=7 dest_node=0 dest_proc=40 dest_thread=41 reply=1 flags=0x0 code=0x1`,
			want: EventBinderTransaction,
			check: func(ev Event) bool {
				return ev.PID == 10 && ev.BinderTransactionID == 7 && ev.BinderDestProc == 40 && ev.BinderDestThread == 41 && ev.BinderReply == 1 && ev.BinderFlags == "0x0" && ev.BinderCode == "0x1"
			},
		},
		{
			name:  "binder received",
			line:  `      waker-10   (   10) [000] .... 2.095000: binder_transaction_received: transaction=7`,
			want:  EventBinderReceived,
			check: func(ev Event) bool { return ev.FieldText == "transaction=7" && ev.BinderTransactionID == 7 },
		},
		{
			name: "binder alloc buf",
			line: `      waker-10   (   10) [000] .... 2.096000: binder_transaction_alloc_buf: debug_id=7 data_size=128 offsets_size=16 extra_buffers_size=4`,
			want: EventBinderAllocBuf,
			check: func(ev Event) bool {
				return ev.BinderTransactionID == 7 && ev.BinderDebugID == 7 && ev.BinderDataSize == 128 && ev.BinderOffsetsSize == 16 && ev.BinderExtraSize == 4
			},
		},
		{
			name:  "binder lock",
			line:  `      waker-10   (   10) [000] .... 2.097000: binder_transaction_lock: tag=binder_inner_lock`,
			want:  EventBinderLock,
			check: func(ev Event) bool { return ev.BinderLockTag == "binder_inner_lock" },
		},
		{
			name:  "irq",
			line:  `      waker-10   (   10) [000] .... 2.100000: irq_handler_entry: irq=32 name=kirq`,
			want:  EventIRQ,
			check: func(ev Event) bool { return ev.CPU == 0 && ev.IRQID == 32 && ev.IRQName == "kirq" },
		},
		{
			name:  "softirq",
			line:  `      waker-10   (   10) [000] .... 2.105000: softirq_entry: vec=3 action=NET_RX`,
			want:  EventSoftIRQ,
			check: func(ev Event) bool { return ev.IRQID == 3 && ev.IRQName == "NET_RX" && ev.SubsystemKind == "softirq" },
		},
		{
			name: "ebpf bio latency",
			line: `      app-20   (   20) [001] .... 2.106000: bio_latency: op=R path=/data/app/base.db latency_us=2500 bytes=4096 callstack=BioRead>Submit`,
			want: EventStorage,
			check: func(ev Event) bool {
				return ev.SubsystemKind == "ebpf_bio" && ev.ResourcePath == "/data/app/base.db" && ev.ResourceLatencyMs == 2.5 && ev.ResourceBytes == 4096
			},
		},
		{
			name: "ebpf filesystem",
			line: `      app-20   (   20) [001] .... 2.107000: file_system: syscall=read path=/data/app/base.db duration_ms=3.5 bytes=1024 callstack=ReadFile`,
			want: EventFilesystem,
			check: func(ev Event) bool {
				return ev.SubsystemKind == "ebpf_filesystem" && ev.ResourceOp == "read" && ev.ResourceLatencyMs == 3.5 && ev.ResourceCallstack == "ReadFile"
			},
		},
		{
			name: "ebpf page fault",
			line: `      app-20   (   20) [001] .... 2.108000: page_fault_user: operation=major address=0x1234 duration_us=150 size=4096 callstack=FaultHandler`,
			want: EventMemory,
			check: func(ev Event) bool {
				return ev.MemoryKind == "page_fault" && ev.ResourceOp == "major" && ev.ResourceAddress == "0x1234" && near(ev.ResourceLatencyMs, 0.150, 0.001)
			},
		},
		{
			name:  "trace mark",
			line:  `      waker-10   (   10) [000] .... 2.120000: print: B|20|Choreographer#doFrame`,
			want:  EventTraceMark,
			check: func(ev Event) bool { return ev.SpanAction == "B" && ev.SpanName == "Choreographer#doFrame" },
		},
		{
			name: "trace counter",
			line: `      waker-10   (   10) [000] .... 2.125000: print: C|31963|JNI Weak Global Refs|198`,
			want: EventTraceMark,
			check: func(ev Event) bool {
				return ev.SpanAction == "C" && ev.SpanName == "JNI Weak Global Refs" && ev.SpanValue == "198"
			},
		},
		{
			name:  "memory",
			line:  `      waker-10   (   10) [000] .... 2.110000: mm_vmscan_direct_reclaim_begin: order=0 may_writepage=1`,
			want:  EventMemory,
			check: func(ev Event) bool { return ev.MemoryKind == "reclaim" },
		},
		{
			name: "storage",
			line: `      waker-10   (   10) [000] .... 2.126000: ufshcd_command: tag=1 opcode=0x28 doorbell=0x1`,
			want: EventStorage,
			check: func(ev Event) bool {
				return ev.SubsystemKind == "storage_ufs" && strings.Contains(ev.FieldText, "opcode=0x28")
			},
		},
		{
			name:  "filesystem",
			line:  `      waker-10   (   10) [000] .... 2.127000: ext4_sync_file_enter: dev 8,0 ino 42 parent 1 datasync 0`,
			want:  EventFilesystem,
			check: func(ev Event) bool { return ev.SubsystemKind == "fs_ext4" },
		},
		{
			name:  "power",
			line:  `      waker-10   (   10) [000] .... 2.128000: thermal_power_allocator: actor=cpu power=300`,
			want:  EventPower,
			check: func(ev Event) bool { return ev.SubsystemKind == "thermal" },
		},
		{
			name: "ability monitor",
			line: `      app-20   (   20) [001] .... 2.128100: ability_monitor: domain=AAFWK event_name=AbilityStart metric=latency_ms value=12.5 category=foreground`,
			want: EventAbilityMonitor,
			check: func(ev Event) bool {
				return ev.SubsystemKind == "ability_monitor" && ev.PluginDomain == "AAFWK" && ev.PluginEventName == "AbilityStart" && ev.PluginMetric == "latency_ms" && ev.PluginValue == "12.5"
			},
		},
		{
			name: "xpower",
			line: `      app-20   (   20) [001] .... 2.128200: xpower_cpu: component=CPU energy=8.2 usage=73 scene=foreground`,
			want: EventXPower,
			check: func(ev Event) bool {
				return ev.SubsystemKind == "xpower" && ev.PluginMetric == "CPU" && ev.PluginValue == "73" && ev.PluginCategory == "foreground"
			},
		},
		{
			name: "hisysevent",
			line: `      app-20   (   20) [001] .... 2.128300: hi_sysevent: domain=POWER eventname=THERMAL_REPORT type=STAT value=hot level=MINOR`,
			want: EventHiSystemEvent,
			check: func(ev Event) bool {
				return ev.SubsystemKind == "hi_sysevent" && ev.PluginDomain == "POWER" && ev.PluginEventName == "THERMAL_REPORT" && ev.PluginMetric == "STAT" && ev.PluginValue == "hot" && ev.PluginCategory == "MINOR"
			},
		},
		{
			name:  "workqueue",
			line:  `      waker-10   (   10) [000] .... 2.129000: workqueue_execute_start: work struct=0 function=do_work`,
			want:  EventWorkqueue,
			check: func(ev Event) bool { return ev.SubsystemKind == "workqueue" },
		},
		{
			name:  "dma fence",
			line:  `      waker-10   (   10) [000] .... 2.129500: dma_fence_wait_start: driver=display timeline=present seqno=7`,
			want:  EventDMAFence,
			check: func(ev Event) bool { return ev.SubsystemKind == "dma_fence" },
		},
		{
			name:  "unknown ftrace row",
			line:  `      waker-10   (   10) [000] .... 2.130000: vendor_private_event: alpha=1`,
			want:  EventUnknown,
			check: func(ev Event) bool { return ev.FieldText == "alpha=1" },
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok := ParseLine(i+10, tc.line, intern)
			if !ok || ev.Type != tc.want || !tc.check(ev) {
				t.Fatalf("ParseLine() = %+v ok=%v, want type %s", ev, ok, tc.want)
			}
		})
	}
}

func TestUnknownEventsRetainFieldTextInEventSearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.systrace")
	body := strings.Join([]string{
		`      worker-10   (   10) [000] .... 2.130000: vendor_private_event: alpha=1 beta=needle`,
		`      worker-10   (   10) [000] .... 2.140000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=000`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(idx, Query{
		View:       "event_search",
		EventTypes: []EventType{EventUnknown},
		TimeStart:  2.12,
		TimeEnd:    2.135,
		Limit:      4,
	})
	if len(res.Events) != 1 {
		t.Fatalf("expected one unknown event, got %d: %+v", len(res.Events), res.Events)
	}
	if got := res.Events[0].FieldText; got != "alpha=1 beta=needle" {
		t.Fatalf("unknown event field text was not preserved: %q", got)
	}
	if !strings.Contains(res.Events[0].Raw, "vendor_private_event") {
		t.Fatalf("unknown event raw row missing: %+v", res.Events[0])
	}
}

func TestEventSearchPatternMatchesFrameIDsAndFields(t *testing.T) {
	idx := buildTraceIndex(t, "frame_pattern.systrace", `
      app-20 (20) [000] .... 9.000000: print: B|20|Choreographer#doFrame 1917295
      app-20 (20) [000] .... 9.001000: print: E|20
      app-20 (20) [000] .... 9.010000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
`)
	events := EventSearch(idx, Query{Pattern: "1917295", Limit: 10})
	if len(events) != 1 || !strings.Contains(events[0].SpanName, "1917295") {
		t.Fatalf("expected frame id pattern to match trace span event: %+v", events)
	}
	events = EventSearch(idx, Query{Pattern: "target_cpu=001", Limit: 10})
	if len(events) != 1 || events[0].Type != EventSchedWakeup {
		t.Fatalf("expected pattern to match raw field text for scheduler event: %+v", events)
	}
}

func TestExternalPerfettoSchedBlockedFixture(t *testing.T) {
	idx, err := BuildIndex(context.Background(), filepath.Join("testdata", "android_perfetto_sched_blocked.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	if idx.ParsedKnown != 6 {
		t.Fatalf("expected all public Perfetto rows to parse as known events, parsed=%d events=%d", idx.ParsedKnown, len(idx.Events))
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 20.0, TimeEnd: 21.2})
	if stats.BlockedReasonCount != 2 || stats.IOWaitBlockedCount != 1 {
		t.Fatalf("Perfetto blocked reason/iowait rows not summarized: %+v", stats)
	}
	tl := ThreadTimeline(idx, Query{PID: 2172, TimeStart: 21.05, TimeEnd: 21.13, MinDurationMs: 1})
	if !timelineHasState(tl, StateIOWait) {
		t.Fatalf("expected iowait timeline from public Perfetto fixture, got %+v", tl.Intervals)
	}
}

func TestExternalPerfettoCPUFrequencyLimitsFixture(t *testing.T) {
	idx, err := BuildIndex(context.Background(), filepath.Join("testdata", "android_perfetto_cpu_frequency_limits.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	res := Run(idx, Query{View: "event_search", TimeStart: 0.0, TimeEnd: 1.2, EventTypes: []EventType{EventCPUFrequency}, Limit: 20})
	if len(res.Events) != 12 {
		t.Fatalf("expected 12 cpu_frequency_limits rows to stay searchable as CPU-frequency events, got %d", len(res.Events))
	}
	for _, ev := range res.Events {
		if ev.Name != "cpu_frequency_limits" || ev.Type != EventCPUFrequencyLimit || !ev.CPUForFieldValid || ev.FrequencyMax == 0 {
			t.Fatalf("cpu_frequency_limits row lost name/cpu_id context: %+v", ev.Event)
		}
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.0, TimeEnd: 1.2})
	if len(stats.CPUFrequencyLimits) == 0 || stats.CPUFrequencyLimits[0].MaxFrequency != 1400000 {
		t.Fatalf("expected most restrictive frequency limit summary: %+v", stats.CPUFrequencyLimits)
	}
}

func TestExternalOpenHarmonyBytraceFixture(t *testing.T) {
	idx, err := BuildIndex(context.Background(), filepath.Join("testdata", "harmony_openharmony_bytrace_thread.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if idx.TraceFlavor != TraceFlavorHarmonyHitrace {
		t.Fatalf("OpenHarmony bytrace fixture should be flavor-classified as Harmony: flavor=%s signals=%v", idx.TraceFlavor, idx.FlavorSignals)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 168758.662877, TimeEnd: 168758.663329})
	if stats.EventCounts[EventSchedWakeup] == 0 || stats.EventCounts[EventSchedWaking] == 0 || len(stats.RunnableTop) == 0 || len(stats.TopRunning) == 0 {
		t.Fatalf("OpenHarmony fixture should produce wakeup and scheduler summaries: %+v", stats)
	}
	events := EventSearch(idx, Query{PID: 2716, TimeStart: 168758.662898, TimeEnd: 168758.663329, EventTypes: []EventType{EventSchedWakeup}, Limit: 10})
	found := false
	for _, ev := range events {
		if ev.PID == 1200 && ev.WakeePID == 2716 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected binder thread wakeup event from public OpenHarmony fixture, got %+v", events)
	}
}

func TestParseHarmonyHitraceSnippetKeepsSecondsAndPrioritySemantics(t *testing.T) {
	idx := buildTraceIndex(t, "harmony.systrace", harmonyTrace)
	if idx.FirstTs != 928.081774 {
		t.Fatalf("timestamp should stay in trace seconds, got %.6f", idx.FirstTs)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 928.081774, TimeEnd: 928.081903})
	if stats.BlockIssueCount != 2 || stats.BlockRemapCount != 1 || stats.BlockCompleteCount != 1 || stats.IRQCount != 2 || stats.MemoryEventCount != 1 {
		t.Fatalf("Harmony resource rows not summarized: %+v", stats)
	}
	foundCFS := false
	foundSystem := false
	for _, td := range stats.RunnableTop {
		if td.Thread.PID == 49634 && td.Priority == 20 && td.PriorityClass == "ohos_cfs" {
			foundCFS = true
		}
	}
	for _, td := range stats.TopRunning {
		if td.Thread.PID == 73 && td.Priority == 301 && td.PriorityClass == "system_or_kernel" {
			foundSystem = true
		}
	}
	if !foundCFS || !foundSystem {
		t.Fatalf("priority classes not preserved: running=%+v runnable=%+v", stats.TopRunning, stats.RunnableTop)
	}
	res := Run(idx, Query{View: "window_stats", TimeStart: 928.081774, TimeEnd: 928.081903})
	if res.TimeUnit != "seconds" || !strings.Contains(res.PrioritySemantics, "1-40=CFS") {
		t.Fatalf("result should describe trace time unit and Harmony priority semantics: %+v", res)
	}
}

func TestTraceFlavorDetectionAndPrioritySemantics(t *testing.T) {
	harmony := buildTraceIndex(t, "sample.htrace", harmonyTrace)
	harmonyRes := Run(harmony, Query{View: "window_stats", TimeStart: 928.081774, TimeEnd: 928.081903})
	if harmonyRes.TraceFlavor != string(TraceFlavorHarmonyHitrace) || !strings.Contains(harmonyRes.PrioritySemantics, "1-40=CFS") {
		t.Fatalf("Harmony trace should use Harmony priority semantics: %+v", harmonyRes)
	}

	android := buildTraceIndex(t, "sample.atrace", strings.Join([]string{
		` system_server-1000 (1000) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=system_server next_pid=1000 next_prio=98`,
		` SurfaceFlinger-2000 (2000) [001] .... 1.010000: sched_wakeup: comm=system_server pid=1000 prio=98 target_cpu=000`,
	}, "\n"))
	androidRes := Run(android, Query{View: "window_stats", TimeStart: 1.0, TimeEnd: 1.02})
	if androidRes.TraceFlavor != string(TraceFlavorAndroidAtrace) || strings.Contains(androidRes.PrioritySemantics, "1-40=CFS") {
		t.Fatalf("Android trace should not use Harmony priority semantics: %+v", androidRes)
	}
	if len(androidRes.WindowStats.TopRunning) == 0 || androidRes.WindowStats.TopRunning[0].PriorityClass != "android_raw_scheduler_prio" {
		t.Fatalf("Android priorities should stay raw: %+v", androidRes.WindowStats.TopRunning)
	}

	generic := buildTraceIndex(t, "sample.systrace", sampleTrace)
	genericRes := Run(generic, Query{View: "window_stats", TimeStart: 1.0, TimeEnd: 1.3})
	if genericRes.TraceFlavor != string(TraceFlavorGenericFtrace) || !strings.Contains(genericRes.PrioritySemantics, "raw scheduler priority") {
		t.Fatalf("ambiguous systrace should fall back to generic semantics: %+v", genericRes)
	}
}

func TestExplicitTraceFlavorOverrideWins(t *testing.T) {
	idx := buildTraceIndex(t, "sample.htrace", harmonyTrace)
	res := Run(idx, Query{
		View:                  "window_stats",
		TimeStart:             928.081774,
		TimeEnd:               928.081903,
		TraceFlavorHint:       TraceFlavorAndroidAtrace,
		TraceFlavorHintSource: "tool_param",
	})
	if res.TraceFlavor != string(TraceFlavorAndroidAtrace) || res.FlavorConfidence != 1.0 {
		t.Fatalf("explicit tool flavor should win: %+v", res)
	}
	if strings.Contains(res.PrioritySemantics, "1-40=CFS") {
		t.Fatalf("explicit Android flavor must not render Harmony semantics: %s", res.PrioritySemantics)
	}
	if !containsString(res.Caveats, "trace flavor was selected from explicit trace_query parameter") ||
		!containsSubstring(res.Caveats, "explicit trace flavor android_atrace conflicts with content-detected harmony_hitrace") {
		t.Fatalf("explicit conflict should be visible in caveats: %+v", res.Caveats)
	}
}

func TestTraceFlavorChineseAliases(t *testing.T) {
	for _, raw := range []string{"鸿蒙", "东湖", "OHOS", "Open Harmony"} {
		if got := NormalizeTraceFlavor(raw); got != TraceFlavorHarmonyHitrace {
			t.Fatalf("%q should normalize to harmony_hitrace, got %s", raw, got)
		}
	}
	for _, raw := range []string{"安卓", "Android"} {
		if got := NormalizeTraceFlavor(raw); got != TraceFlavorAndroidAtrace {
			t.Fatalf("%q should normalize to android_atrace, got %s", raw, got)
		}
	}
}

func TestDonghuPlatformKeepsHarmonySchedulerSemantics(t *testing.T) {
	if got := NormalizeTracePlatform("东湖"); got != TracePlatformDonghu {
		t.Fatalf("NormalizeTracePlatform(东湖)=%s, want donghu", got)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "donghu.systrace")
	trace := strings.Join([]string{
		`  com.tencent.mm-36379 (36379) [004] .... 2942.124416: sched_switch: prev_comm=com.tencent.mm prev_pid=36379 prev_prio=53 prev_state=S ==> next_comm=OS_FFRT_0_0 next_pid=49634 next_prio=20 next_info=rtq cg=top-app`,
		`     OS_FFRT_0_0-49634 (48679) [000] .... 2942.130000: sched_wakeup: comm=com.tencent.mm pid=36379 prio=53 target_cpu=004`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(idx, Query{
		View:              "event_search",
		TimeStart:         2942.12,
		TimeEnd:           2942.14,
		EventTypes:        []EventType{EventSchedSwitch, EventSchedWakeup},
		TracePlatformHint: TracePlatformDonghu,
		Limit:             8,
	})
	if res.Platform != string(TracePlatformDonghu) || res.TraceFlavor != string(TraceFlavorHarmonyHitrace) {
		t.Fatalf("donghu should resolve to platform=donghu and flavor=harmony_hitrace: %+v", res)
	}
	if !strings.Contains(res.PrioritySemantics, "1-40=CFS") || !strings.Contains(res.PrioritySemantics, "larger numeric value means higher priority") {
		t.Fatalf("donghu should keep Harmony priority semantics: %s", res.PrioritySemantics)
	}
	if res.FrameworkMode != "process_isolated_mixed" {
		t.Fatalf("donghu framework mode = %q", res.FrameworkMode)
	}
	if len(res.FrameworkSurfaces) < 2 {
		t.Fatalf("expected android and harmony framework surface hints: %+v", res.FrameworkSurfaces)
	}
	if len(res.Events) == 0 || res.Events[0].NextInfo != "rtq" || res.Events[0].CGroup != "top-app" {
		t.Fatalf("sched variant fields missing from event_search: %+v", res.Events)
	}
}

func TestAutoDonghuMixedHarmonyBaseCandidateKeepsHarmonySchedulerSemantics(t *testing.T) {
	trace := strings.Join([]string{
		`  com.tencent.mm-36379 (36379) [004] .... 2942.124416: sched_switch: prev_comm=com.tencent.mm prev_pid=36379 prev_prio=53 prev_state=S ==> next_comm=OS_FFRT_0_0 next_pid=49634 next_prio=20`,
		`     OS_FFRT_0_0-49634 (48679) [000] .... 2942.130000: sched_wakeup: comm=com.tencent.mm pid=36379 prio=53 target_cpu=004`,
		`  RSUniRenderThre-2096  ( 1716) [000] .... 2942.131000: print: B|1716|H:RenderFrame|M0538`,
		"",
	}, "\n")
	idx := buildTraceIndex(t, "mixed.systrace", trace)
	res := Run(idx, Query{View: "window_stats", TimeStart: 2942.12, TimeEnd: 2942.14})
	if res.Platform != string(TracePlatformDonghu) {
		t.Fatalf("auto mixed Harmony-base trace should resolve platform=donghu: %+v", res)
	}
	if res.PlatformCandidate != "mixed_harmony_base" || res.PlatformCandidateConfidence <= 0 {
		t.Fatalf("expected mixed_harmony_base platform candidate: %+v", res)
	}
	if res.TraceFlavor != string(TraceFlavorHarmonyHitrace) || !strings.Contains(res.PrioritySemantics, "1-40=CFS") {
		t.Fatalf("mixed Harmony-base trace should keep Harmony priority semantics: %+v", res)
	}
	if res.FrameworkMode != "process_isolated_mixed" || len(res.FrameworkSurfaces) < 2 {
		t.Fatalf("expected process-isolated mixed framework surfaces: mode=%s surfaces=%+v", res.FrameworkMode, res.FrameworkSurfaces)
	}
	if !containsSubstring(res.Caveats, "auto platform candidate mixed_harmony_base") {
		t.Fatalf("auto platform candidate should be visible in caveats: %+v", res.Caveats)
	}
}

func TestSpanWindowFindsUniqueTraceSpan(t *testing.T) {
	idx := buildTraceIndex(t, "span.systrace", p1ResourceTrace)
	res := Run(idx, Query{View: "span_window", SpanName: "Choreographer#doFrame", Limit: 4})
	if len(res.SpanWindows) != 1 {
		t.Fatalf("expected one span window, got %+v caveats=%+v", res.SpanWindows, res.Caveats)
	}
	span := res.SpanWindows[0]
	if span.StartLine == 0 || span.EndLine == 0 || !near(span.DurationMs, 20.0, 0.001) {
		t.Fatalf("unexpected span window: %+v", span)
	}
	if len(res.EvidencePack) == 0 || res.EvidencePack[0].Predicate != "trace_span_window" {
		t.Fatalf("span_window should produce evidence facts: %+v", res.EvidencePack)
	}
}

func TestSpanWindowMultipleMatchesSuggestsPatternNarrowing(t *testing.T) {
	idx := buildTraceIndex(t, "span_multi.systrace", `
app-20 (20) [001] .... 1.000000: print: B|20|Choreographer#doFrame 111
app-20 (20) [001] .... 1.010000: print: E|20
app-20 (20) [001] .... 2.000000: print: B|20|Choreographer#doFrame 222
app-20 (20) [001] .... 2.030000: print: E|20
`)
	res := Run(idx, Query{View: "span_window", SpanName: "Choreographer#doFrame", Limit: 4})
	if len(res.SpanWindows) != 2 {
		t.Fatalf("expected two span windows, got %+v", res.SpanWindows)
	}
	if !containsSubstring(res.Caveats, "event_search") || !containsSubstring(res.Caveats, "pattern=\"<frame id or exact label>\"") {
		t.Fatalf("multiple span caveat should teach event_search pattern narrowing: %+v", res.Caveats)
	}
}

func TestRootCauseRankTiersCandidates(t *testing.T) {
	idx := buildSampleIndex(t)
	res := Run(idx, Query{View: "root_cause_rank", PID: 20, TimeStart: 1.10, TimeEnd: 1.22, Limit: 5})
	if res.RootCauseRank == nil || len(res.RootCauseRank.Items) == 0 {
		t.Fatalf("expected ranked root-cause candidates, got %+v", res.RootCauseRank)
	}
	first := res.RootCauseRank.Items[0]
	if first.Rank != 1 || first.Tier != "primary" || first.ImpactMs <= 0 || first.Score <= 0 {
		t.Fatalf("bad primary rank item: %+v", first)
	}
	if len(res.EvidencePack) == 0 || !strings.HasPrefix(res.EvidencePack[0].Predicate, "root_cause_") {
		t.Fatalf("root_cause_rank should produce evidence facts: %+v", res.EvidencePack)
	}
}

func TestInteractionStatsCountsBidirectionalWakeups(t *testing.T) {
	idx := buildSampleIndex(t)
	res := Run(idx, Query{View: "interaction_stats", PID: 20, TimeStart: 1.0, TimeEnd: 1.3})
	if res.InteractionStats == nil || len(res.InteractionStats.Items) == 0 {
		t.Fatalf("expected interaction stats, got %+v", res.InteractionStats)
	}
	top := res.InteractionStats.Items[0]
	if top.Peer.PID != 10 || top.WakeupsToTarget != 2 || top.TotalInteractions != 2 {
		t.Fatalf("expected waker peer to wake target twice, got %+v", top)
	}
}

func TestSchedulerLatencyStatsQuantifiesRunnableWaitAndCompetition(t *testing.T) {
	idx := buildTraceIndex(t, "scheduler.systrace", schedulerLatencyTrace)
	res := Run(idx, Query{View: "scheduler_latency_stats", PID: 20, TimeStart: 5.0, TimeEnd: 5.15})
	if res.SchedulerLatency == nil || res.SchedulerLatency.Count != 2 {
		t.Fatalf("expected two runnable waits, got %+v", res.SchedulerLatency)
	}
	if !near(res.SchedulerLatency.MaxMs, 80, 0.001) || !near(res.SchedulerLatency.P95Ms, 80, 0.001) {
		t.Fatalf("bad latency percentiles: %+v", res.SchedulerLatency)
	}
	top := res.SchedulerLatency.Items[0]
	if top.Thread.PID != 20 || top.CPU != 1 || top.Frequency != 1000000 || len(top.SameCPUTopRunning) == 0 {
		t.Fatalf("scheduler latency item missing CPU/frequency/competition context: %+v", top)
	}
}

func TestWindowStatsComputeSupplyAndRootCauseRankUseSchedulerLatency(t *testing.T) {
	idx := buildTraceIndex(t, "scheduler.systrace", schedulerLatencyTrace)
	stats := ComputeWindowStats(idx, Query{PID: 20, TimeStart: 5.0, TimeEnd: 5.15})
	if len(stats.ComputeSupply) == 0 {
		t.Fatalf("expected compute supply summaries: %+v", stats)
	}
	foundPressure := false
	for _, supply := range stats.ComputeSupply {
		if supply.Thread.PID == 20 && strings.Contains(supply.Verdict, "cpu_pressure") {
			foundPressure = true
		}
	}
	if !foundPressure {
		t.Fatalf("expected CPU-pressure compute-supply signal: %+v", stats.ComputeSupply)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 20, TimeStart: 5.0, TimeEnd: 5.15})
	foundScheduler := false
	for _, item := range rank.Items {
		if item.Type == "scheduler_latency" || item.Type == "low_frequency" || item.Type == "compute_supply" {
			foundScheduler = true
		}
	}
	if !foundScheduler {
		t.Fatalf("root cause rank missing scheduler/compute supply evidence: %+v", rank.Items)
	}
}

func TestFramePipelineCriticalBlockingAndRecipeViews(t *testing.T) {
	idx := buildTraceIndex(t, "blocking.systrace", blockingTrace)
	frame := Run(idx, Query{View: "frame_window", PID: 20, TimeStart: 6.0, TimeEnd: 6.1})
	if frame.FramePipeline == nil || len(frame.FramePipeline.Items) == 0 || frame.FramePipeline.Items[0].Phase != "frame_schedule" {
		t.Fatalf("expected frame pipeline item: %+v", frame.FramePipeline)
	}
	blocking := Run(idx, Query{View: "critical_blocking_calls", PID: 20, TimeStart: 6.0, TimeEnd: 6.1})
	if blocking.CriticalBlocking == nil || len(blocking.CriticalBlocking.Items) == 0 {
		t.Fatalf("expected critical blocking candidates: %+v", blocking.CriticalBlocking)
	}
	foundLock := false
	for _, item := range blocking.CriticalBlocking.Items {
		if item.Type == "blocking_span" && strings.Contains(item.Summary, "Lock contention") {
			foundLock = true
		}
	}
	if !foundLock {
		t.Fatalf("expected lock/futex-like span in critical blocking candidates: %+v", blocking.CriticalBlocking.Items)
	}
	recipe := Run(idx, Query{View: "recipe", RecipeName: "jank", PID: 20, TimeStart: 6.0, TimeEnd: 6.1})
	if recipe.Recipe == nil || !containsString(recipe.Recipe.IncludedViews, "frame_window") || recipe.FramePipeline == nil || recipe.CriticalBlocking == nil {
		t.Fatalf("jank recipe should include frame and blocking views: %+v", recipe)
	}
	unbounded := Run(idx, Query{View: "recipe", RecipeName: "jank"})
	if unbounded.Recipe == nil || !containsString(unbounded.Recipe.IncludedViews, "frame_window") {
		t.Fatalf("unbounded jank recipe should still discover frame views: %+v", unbounded.Recipe)
	}
	if unbounded.WindowStats != nil || unbounded.RootCauseRank != nil || unbounded.CriticalBlocking != nil || unbounded.SchedulerLatency != nil {
		t.Fatalf("unbounded jank recipe should not expand expensive full-trace analysis: %+v", unbounded)
	}
	if !containsSubstring(unbounded.Caveats, "large recipe guard") || !containsSubstring(unbounded.Recipe.Caveats, "discovery mode") {
		t.Fatalf("unbounded jank recipe should explain discovery mode: result=%v recipe=%v", unbounded.Caveats, unbounded.Recipe.Caveats)
	}
}

func TestFrameTimelineAndFlowViews(t *testing.T) {
	idx := buildTraceIndex(t, "frame_flow.systrace", frameFlowTrace)
	timeline := Run(idx, Query{View: "frame_timeline", TimeStart: 7.0, TimeEnd: 7.05, Limit: 10})
	if timeline.FrameTimeline == nil || len(timeline.FrameTimeline.Items) < 3 {
		t.Fatalf("expected frame timeline items: %+v", timeline.FrameTimeline)
	}
	var foundExpected, foundUI, foundRS, foundGPU bool
	for _, item := range timeline.FrameTimeline.Items {
		switch item.Role {
		case "expected":
			foundExpected = true
		case "ui":
			foundUI = true
		case "render_service":
			foundRS = true
		case "gpu":
			foundGPU = true
		}
	}
	if !foundExpected || !foundUI || !foundRS || !foundGPU {
		t.Fatalf("missing frame timeline roles expected=%v ui=%v rs=%v gpu=%v items=%+v", foundExpected, foundUI, foundRS, foundGPU, timeline.FrameTimeline.Items)
	}
	flow := Run(idx, Query{View: "frame_flow", TimeStart: 7.0, TimeEnd: 7.05, Limit: 10})
	if flow.FrameTimeline == nil || len(flow.FrameTimeline.Flows) < 2 {
		t.Fatalf("expected frame flow edges: %+v", flow.FrameTimeline)
	}
	if len(flow.EvidencePack) == 0 {
		t.Fatalf("frame flow should produce evidence facts")
	}
}

func TestWindowStatsSummarizesSmartPerfEBPFResources(t *testing.T) {
	idx := buildTraceIndex(t, "ebpf.systrace", ebpfResourceTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 8.0, TimeEnd: 8.03})
	if len(stats.BIOResources) != 1 || stats.BIOResources[0].Path != "/data/app/base.db" || !near(stats.BIOResources[0].TotalLatencyMs, 2.5, 0.001) {
		t.Fatalf("expected BIO resource summary: %+v", stats.BIOResources)
	}
	if len(stats.FilesystemResources) != 1 || stats.FilesystemResources[0].Operation != "read" || stats.FilesystemResources[0].Bytes != 1024 {
		t.Fatalf("expected filesystem resource summary: %+v", stats.FilesystemResources)
	}
	if len(stats.PageFaultResources) != 1 || stats.PageFaultResources[0].Operation != "major" || stats.PageFaultResources[0].Address != "0x1234" {
		t.Fatalf("expected page fault resource summary: %+v", stats.PageFaultResources)
	}
}

func TestWindowStatsSummarizesSmartPerfPluginResources(t *testing.T) {
	idx := buildTraceIndex(t, "plugin.systrace", pluginResourceTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 9.0, TimeEnd: 9.03})
	if stats.AbilityEventCount != 1 || len(stats.AbilityEvents) != 1 || stats.AbilityEvents[0].Domain != "AAFWK" || stats.AbilityEvents[0].EventName != "AbilityStart" {
		t.Fatalf("expected ability monitor summary: count=%d items=%+v", stats.AbilityEventCount, stats.AbilityEvents)
	}
	if stats.XPowerEventCount != 1 || len(stats.XPowerEvents) != 1 || stats.XPowerEvents[0].Metric != "CPU" || stats.XPowerEvents[0].Value != "73" {
		t.Fatalf("expected XPower summary: count=%d items=%+v", stats.XPowerEventCount, stats.XPowerEvents)
	}
	if stats.HiSystemEventCount != 1 || len(stats.HiSystemEvents) != 1 || stats.HiSystemEvents[0].Domain != "POWER" || stats.HiSystemEvents[0].EventName != "THERMAL_REPORT" {
		t.Fatalf("expected HiSystemEvent summary: count=%d items=%+v", stats.HiSystemEventCount, stats.HiSystemEvents)
	}
}

func TestWindowStatsCoreTopologyAnnotatesComputeSupply(t *testing.T) {
	idx := buildTraceIndex(t, "core.systrace", coreTopologyTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 10.0, TimeEnd: 10.06, CoreTopology: "small=0-3,big=4-7"})
	foundCPU4 := false
	for _, cpu := range stats.CPU {
		if cpu.CPU == 4 {
			foundCPU4 = true
			if cpu.CoreClass != "big" {
				t.Fatalf("expected cpu4 big class: %+v", cpu)
			}
		}
	}
	if !foundCPU4 {
		t.Fatalf("missing cpu4 stats: %+v", stats.CPU)
	}
	if len(stats.CoreTopology) == 0 || stats.CoreTopology[len(stats.CoreTopology)-1].Class != "big" {
		t.Fatalf("expected core topology class summary: %+v", stats.CoreTopology)
	}
	if len(stats.ComputeSupply) == 0 || stats.ComputeSupply[0].CoreClass != "big" {
		t.Fatalf("expected compute supply to carry core class: %+v", stats.ComputeSupply)
	}
}

func TestThreadTimelineSplitsSleepAndRunnable(t *testing.T) {
	idx := buildSampleIndex(t)
	tl := ThreadTimeline(idx, Query{PID: 20, TimeStart: 1.09, TimeEnd: 1.24, MinDurationMs: 1})
	if len(tl.Intervals) < 2 {
		t.Fatalf("expected sleep+runnable intervals, got %+v", tl.Intervals)
	}
	if tl.Intervals[0].State != StateSSleep || tl.Intervals[1].State != StateRunnable {
		t.Fatalf("unexpected interval states: %+v", tl.Intervals)
	}
	if tl.Intervals[0].WakeupLine == 0 && tl.Intervals[1].WakeupLine == 0 {
		t.Fatalf("expected wakeup line to be preserved: %+v", tl.Intervals)
	}
}

func TestThreadTimelineClassifiesDState(t *testing.T) {
	idx := buildTraceIndex(t, "resource.systrace", resourceTrace)
	tl := ThreadTimeline(idx, Query{PID: 30, TimeStart: 2.02, TimeEnd: 2.16, MinDurationMs: 1})
	if !timelineHasState(tl, StateIOWait) {
		t.Fatalf("expected IO-wait interval enriched from sched_blocked_reason, got %+v", tl.Intervals)
	}
}

func TestWakeupChainFindsWakerAndRoot(t *testing.T) {
	idx := buildSampleIndex(t)
	chain := BuildWakeupChain(idx, Query{PID: 20, TimeStart: 1.10, TimeEnd: 1.22, MaxDepth: 4, MinDurationMs: 1})
	if len(chain.Edges) != 1 {
		t.Fatalf("expected one wakeup edge, got %+v", chain)
	}
	if chain.Edges[0].Waker.PID != 10 || chain.Edges[0].Wakee.PID != 20 || chain.Edges[0].WakeupLine == 0 {
		t.Fatalf("bad edge: %+v", chain.Edges[0])
	}
	if len(chain.RootEvidence) == 0 {
		t.Fatalf("expected root evidence: %+v", chain)
	}
}

func TestIPCGraphMatchesBinderSendAndReceive(t *testing.T) {
	idx := buildTraceIndex(t, "ipc.systrace", ipcTrace)
	ipc := BuildIPCGraph(idx, Query{PID: 20, TimeStart: 3.0, TimeEnd: 3.04, Limit: 10})
	if len(ipc.Edges) != 1 {
		t.Fatalf("expected one IPC edge, got %+v", ipc)
	}
	edge := ipc.Edges[0]
	if edge.TransactionID != 42 || edge.Sender.PID != 20 || edge.Receiver.PID != 101 || edge.Receiver.TGID != 100 || edge.SendLine == 0 || edge.ReceiveLine == 0 {
		t.Fatalf("bad IPC edge: %+v", edge)
	}
	if edge.Confidence < 0.9 || edge.LatencyMs <= 0 {
		t.Fatalf("expected matched receive confidence and latency: %+v", edge)
	}
	if len(ipc.BinderEvents) != 3 {
		t.Fatalf("expected binder auxiliary rows, got %+v", ipc.BinderEvents)
	}
	if !containsSubstring(edge.Caveats, "binder alloc buffer row") {
		t.Fatalf("edge should carry alloc buffer caveat: %+v", edge.Caveats)
	}
}

func TestWakeupChainCarriesRelevantIPCEdges(t *testing.T) {
	idx := buildTraceIndex(t, "ipc.systrace", ipcTrace)
	chain := BuildWakeupChain(idx, Query{PID: 20, TimeStart: 3.0, TimeEnd: 3.04, MaxDepth: 4, MinDurationMs: 1})
	if len(chain.IPCEdges) != 1 || chain.IPCEdges[0].TransactionID != 42 {
		t.Fatalf("expected wakeup chain to carry target IPC edge, got %+v", chain.IPCEdges)
	}
}

func TestWakeupChainReportsBinderWaitCandidate(t *testing.T) {
	idx := buildTraceIndex(t, "ipc.systrace", ipcTrace)
	chain := BuildWakeupChain(idx, Query{PID: 20, TimeStart: 3.0, TimeEnd: 3.04, MaxDepth: 4, MinDurationMs: 1})
	if len(chain.BinderWaits) != 1 {
		t.Fatalf("expected binder wait candidate, got %+v", chain.BinderWaits)
	}
	wait := chain.BinderWaits[0]
	if wait.TransactionID != 42 || wait.SendLine == 0 || wait.SleepLine == 0 || wait.Confidence <= 0 {
		t.Fatalf("bad binder wait: %+v", wait)
	}
	if !containsSubstring(wait.Caveats, "binder alloc buffer") || !containsSubstring(wait.Caveats, "binder_lock") {
		t.Fatalf("binder wait should carry auxiliary binder caveats: %+v", wait.Caveats)
	}
	foundRoot := false
	for _, root := range chain.RootEvidence {
		if root.Type == "binder_wait" {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Fatalf("binder wait should be carried as root evidence candidate: %+v", chain.RootEvidence)
	}
}

func TestWakeupChainReportsMissingWakeup(t *testing.T) {
	idx := buildSampleIndex(t)
	chain := BuildWakeupChain(idx, Query{PID: 10, TimeStart: 1.05, TimeEnd: 1.17, MaxDepth: 4, MinDurationMs: 1})
	if len(chain.RootEvidence) == 0 || chain.RootEvidence[0].Type != "missing_wakeup" {
		t.Fatalf("expected missing wakeup root, got %+v", chain.RootEvidence)
	}
}

func TestWakeupChainReportsDStateRoot(t *testing.T) {
	idx := buildTraceIndex(t, "resource.systrace", resourceTrace)
	chain := BuildWakeupChain(idx, Query{PID: 30, TimeStart: 2.03, TimeEnd: 2.15, MaxDepth: 4, MinDurationMs: 1})
	if len(chain.RootEvidence) == 0 || chain.RootEvidence[0].Type != "io_wait" || !strings.Contains(chain.RootEvidence[0].Summary, "fscache_page_wait_on_page_bit") {
		t.Fatalf("expected D-state root, got %+v", chain.RootEvidence)
	}
}

func TestWindowStatsComputesCPUAndResourceCounts(t *testing.T) {
	idx := buildSampleIndex(t)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.35})
	if stats.BlockIssueCount != 1 {
		t.Fatalf("BlockIssueCount=%d", stats.BlockIssueCount)
	}
	if len(stats.CPU) == 0 || len(stats.TopRunning) == 0 {
		t.Fatalf("expected cpu stats and top running threads: %+v", stats)
	}
}

func TestWindowStatsCountsRuntimeResourcesAndOffCPU(t *testing.T) {
	idx := buildTraceIndex(t, "resource.systrace", resourceTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2.0, TimeEnd: 2.2})
	if stats.BlockIssueCount != 1 || stats.BlockCompleteCount != 1 || stats.BinderCount != 2 || stats.BinderReceivedCount != 1 || stats.IRQCount != 1 || stats.MemoryEventCount != 1 || stats.BlockedReasonCount != 1 || stats.IOWaitBlockedCount != 1 {
		t.Fatalf("resource counts not preserved: %+v", stats)
	}
	if len(stats.BlockedReasons) == 0 || stats.BlockedReasons[0].Thread.PID != 30 || stats.BlockedReasons[0].IOWait != 1 || !strings.Contains(stats.BlockedReasons[0].Reason, "fscache") {
		t.Fatalf("blocked reason not summarized: %+v", stats.BlockedReasons)
	}
	if len(stats.RunnableTop) == 0 || stats.RunnableTop[0].Thread.PID != 20 {
		t.Fatalf("expected runnable top for main thread: %+v", stats.RunnableTop)
	}
	if stats.RunnableTop[0].CPU != 1 {
		t.Fatalf("expected runnable top to keep CPU context: %+v", stats.RunnableTop[0])
	}
	if len(stats.DStateTop) == 0 || stats.DStateTop[0].Thread.PID != 30 {
		t.Fatalf("expected D-state top for worker thread: %+v", stats.DStateTop)
	}
	if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].DurationMs <= 0 || stats.IOLatencies[0].IssueLine == 0 || stats.IOLatencies[0].CompleteLine == 0 {
		t.Fatalf("expected paired IO latency: %+v", stats.IOLatencies)
	}
	if len(stats.CPUPressure) == 0 {
		t.Fatalf("expected CPU pressure stats: %+v", stats.CPUPressure)
	}
	foundFreq := false
	for _, cpu := range stats.CPU {
		if cpu.CPU == 0 && cpu.Frequency == 1800000 {
			foundFreq = true
		}
	}
	if !foundFreq {
		t.Fatalf("expected cpu frequency to be summarized: %+v", stats.CPU)
	}
}

func TestWindowStatsComputesCPUFrequencyResidency(t *testing.T) {
	idx := buildTraceIndex(t, "freq.systrace", frequencyTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2940.185000, TimeEnd: 2941.680123})
	cpu11 := findCPUStats(stats, 11)
	if cpu11 == nil || cpu11.Frequency != 2228000 {
		t.Fatalf("expected CPU11 latest frequency from cpu_frequency row, got %+v", cpu11)
	}
	if len(cpu11.FrequencyResidency) != 2 {
		t.Fatalf("expected CPU11 residency before and after switch, got %+v", cpu11.FrequencyResidency)
	}
	if cpu11.FrequencyResidency[0].Frequency != 1800000 || !near(cpu11.FrequencyResidency[0].DurationMs, 5.402, 0.001) {
		t.Fatalf("bad pre-window carried residency: %+v", cpu11.FrequencyResidency[0])
	}
	if cpu11.FrequencyResidency[1].Frequency != 2228000 || cpu11.FrequencyResidency[1].DurationMs <= 0 {
		t.Fatalf("bad switched residency: %+v", cpu11.FrequencyResidency[1])
	}
	cpu0 := findCPUStats(stats, 0)
	if cpu0 == nil || cpu0.Frequency != 1800000 || len(cpu0.FrequencyResidency) != 2 {
		t.Fatalf("expected CPU0 residency from real cpu_frequency rows only, got %+v", cpu0)
	}
	for _, item := range cpu0.FrequencyResidency {
		if item.Frequency == 3744 {
			t.Fatalf("DDR clock_set_rate leaked into CPU frequency residency: %+v", cpu0.FrequencyResidency)
		}
	}
}

func TestWindowStatsComputesP1ResourceSummaries(t *testing.T) {
	idx := buildTraceIndex(t, "p1.systrace", p1ResourceTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 4.0, TimeEnd: 4.03})
	if len(stats.TraceSpans) != 1 || stats.TraceSpans[0].Name != "Choreographer#doFrame" || !near(stats.TraceSpans[0].DurationMs, 20.0, 0.001) {
		t.Fatalf("expected span duration: %+v", stats.TraceSpans)
	}
	if len(stats.TraceCounters) != 1 || stats.TraceCounters[0].Name != "JNI Weak Global Refs" || stats.TraceCounters[0].Value != "198" {
		t.Fatalf("expected trace counter: %+v", stats.TraceCounters)
	}
	if len(stats.IRQBursts) == 0 || stats.IRQBursts[0].IRQ != 32 || stats.IRQBursts[0].Count < 2 {
		t.Fatalf("expected IRQ burst: %+v", stats.IRQBursts)
	}
	if len(stats.MemoryKinds) != 2 {
		t.Fatalf("expected memory kinds: %+v", stats.MemoryKinds)
	}
	kinds := map[string]bool{}
	for _, item := range stats.MemoryKinds {
		kinds[item.Kind] = true
	}
	if !kinds["page_cache"] || !kinds["reclaim"] {
		t.Fatalf("expected page_cache and reclaim kinds: %+v", stats.MemoryKinds)
	}
	if len(stats.CPUFrequencyLimits) == 0 || stats.CPUFrequencyLimits[0].MaxFrequency != 1500000 {
		t.Fatalf("expected cpu frequency limit summary: %+v", stats.CPUFrequencyLimits)
	}
	if stats.SoftIRQCount != 1 || stats.StorageEventCount != 1 || stats.FilesystemEventCount != 1 || stats.PowerEventCount != 1 || stats.WorkqueueEventCount != 1 || stats.DMAFenceEventCount != 1 {
		t.Fatalf("expected subsystem counters, got %+v", stats)
	}
	subsystems := map[string]bool{}
	for _, item := range stats.SubsystemEvents {
		subsystems[item.Kind] = true
	}
	for _, want := range []string{"cpu_frequency_limits", "softirq", "storage_ufs", "fs_ext4", "thermal", "workqueue", "dma_fence"} {
		if !subsystems[want] {
			t.Fatalf("missing subsystem %s in %+v", want, stats.SubsystemEvents)
		}
	}
	if len(stats.ThreadDrifts) == 0 || stats.ThreadDrifts[0].PID != 20 {
		t.Fatalf("expected pid/name drift caveat: %+v", stats.ThreadDrifts)
	}
}

func TestWindowStatsHonorsLineWindow(t *testing.T) {
	idx := buildTraceIndex(t, "resource.systrace", resourceTrace)
	stats := ComputeWindowStats(idx, Query{TimeStart: 2.0, TimeEnd: 2.2, LineStart: 1, LineEnd: 4})
	if stats.BlockIssueCount != 0 || stats.BinderCount != 0 || stats.IRQCount != 0 || stats.MemoryEventCount != 0 {
		t.Fatalf("line window should exclude later resource rows: %+v", stats)
	}
}

func findCPUStats(stats WindowStats, cpu int) *CPUStats {
	for i := range stats.CPU {
		if stats.CPU[i].CPU == cpu {
			return &stats.CPU[i]
		}
	}
	return nil
}

func near(got, want, delta float64) bool {
	if got < want {
		return want-got <= delta
	}
	return got-want <= delta
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func containsSubstring(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

func buildSampleIndex(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "sample.systrace", sampleTrace)
}

func timelineHasState(tl TimelineResult, state ThreadState) bool {
	for _, it := range tl.Intervals {
		if it.State == state {
			return true
		}
	}
	return false
}

func buildTraceIndex(t *testing.T, name, content string) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.ParsedKnown == 0 {
		t.Fatal("expected known events")
	}
	return idx
}
