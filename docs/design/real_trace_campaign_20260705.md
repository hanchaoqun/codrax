# 真实 trace 多维战役账本 — 第一阶段:画像 + 用例矩阵(2026-07-05)

素材:`eval/fixtures/real_traces/donghu_tieba_frame.systrace`(15,623 行,
com.baidu.tieba 59566 主线程 ~34579.472865–34579.475857s 卡顿)与
`eval/fixtures/real_traces/donghu_short_excerpt.systrace`(100 行摘录)。
两个 fixture 本体只读(README 红线),既有 `eval/cases/*.case` 未改动
(CSP agent 并发在跑)。

确定性画像(全部 ground truth 数字 + 推导命令):
`eval/fixtures/real_traces/PROFILE.md`。本账本逐案引用其小节号。

新用例:`eval/cases/real_traces/`(18 案,均只引用仓内 fixture 拷贝)。
本批产出后不跑批 —— 运行协议见 §3,主会话统一执行。

---

## 1. 维度矩阵

"支撑度"以 PROFILE 实测为准;不支撑的维度如实标 N/A,禁硬造。

| 维度 | 子项 | 判定 | 承载案 / 理由 |
|---|---|---|---|
| A 窗口 | A1 卡顿短窗 | 已有案覆盖 | `trace_query_donghu_real_short_runnable`(customlogs 路径)已覆盖 jank 窗直接依赖问法;不重复建案(任务红线) |
| | A2 加宽 10× 窗 | ✅ | `real_trace_a2_wide_window_ratio`(34579.472865–34579.502785,29.92ms) |
| | A3 整 trace 无窗 | ✅ | `real_trace_a3_whole_trace_overview` |
| | A4 越界窗(诚实降级) | ✅ | `real_trace_a4_out_of_range_window`(34580.0–34580.1,全在 last_ts=34579.595184 之后) |
| | A5 excerpt 退化窗 | ✅ | `real_trace_a5_excerpt_degenerate_window`(问 100ms,数据仅 0.556ms) |
| B 主体 | B1 点名线程 | 全批隐含 | 多数案以 comm+tid 点名(a2/b5/c2/d2/…);不设专案 |
| | B2 仅 tid | ✅ | `real_trace_b2_tid_only_waker`(裸 59843 → CookieMonsterCl 身份解析) |
| | B3 进程级 | ✅ | `real_trace_b3_process_level_rollup`(tgid 59566,39 线程) |
| | B4 不存在线程 | ✅ | `real_trace_b4_missing_thread_miss`(99999,grep 计数=0) |
| | B5 多主体 | ✅ | `real_trace_b5_multi_subject_render`(59566+59891 同窗对比) |
| C 状态 | C1 runnable 主导问法 | **N/A(改型)** | 主线程 runnable 在任何窗都不主导(full 5.529ms/144.6ms;jank 0.014ms;PROFILE §1.7)——按"runnable 主导"出题=诱导降 bar 或诱导编造;runnable 依赖问法已有案(A1 行);不另建 |
| | C2 D-state/io_wait | ✅ | `real_trace_c2_dstate_iowait`(3 条 iowait=1,verbatim caller) |
| | C3 周期源(有 vsync 才做) | ✅(限缩) | `real_trace_c3_vsync_periodic`——vsync 存在但窗内仅 ~2 个脉冲(PROFILE §1.9),只考 presence+period+驱动线程;**VS-1 式多周期 cadence 核算 N/A(样本量不支撑)**,VS-1 期内睡眠口径因此不在本案考面 |
| | C4 供给折算(有 freq 泳道才做) | ✅(限缩) | `real_trace_c4_freq_supply_evidence`——cpu_frequency 仅 cpu3-5 有样本;**cpu_frequency_limits 泳道 0 事件,VS-2b 治理时间线口径不可考**,只考"哪些核有采样+范围+诚实披露非全核" |
| D 因果 | D1 直接唤醒关系 | ✅(并入) | 并入 `real_trace_b2_tid_only_waker`(整 trace 聚合:34/48 次)与 `real_trace_g2_relative_path_inrepo`;jank 窗内直接唤醒已有案覆盖(A1 行) |
| | D2 链恢复(经由线程) | ✅ | `real_trace_d2_chain_via_networkservice`(main←CookieMonsterCl←NetworkService,31/32/34 次中继,PROFILE §1.6) |
| | D3 优先级反转有无 | 已有案覆盖 | 既有 short_runnable/multicausal oracle 已含 priority-inversion 分支;prio 事实(52 RT vs 20 CFS)已入 PROFILE §1.5 供审读,不再建专案(避免与在跑 CSP 案同窗同问重复) |
| | D4 背景压力/需求-供给措辞(§7.4 车道) | ✅ | `real_trace_d4_demand_vs_supply`(sleep 73.4% + idle 269.8ms 余量 + fmax 顶格 → 需求侧) |
| E 对比 | E1 同 trace 双窗(归一化/密度) | ✅ | `real_trace_e1_dual_window_normalized`(2.992ms vs 30ms,10× 窗长差,CMP 归一化教训) |
| | E2 跨 trace(单边未采样) | ✅ | `real_trace_e2_cross_trace_asymmetry`(freq/vsync 单边缺失 + 2942.x/34579.x 时基不可对齐) |
| F 诚实显示 | F1 exclude 语义(伪引用零渲染=CPD) | ✅ | `real_trace_f1_exclude_no_code`(负向断言 .codrax/blob) |
| | F2 覆盖行自解释 | ✅(并入) | 并入 `real_trace_a5_excerpt_degenerate_window`(实际覆盖端点必须出现) |
| | F3 截断披露 | **N/A** | 15,623 事件远低于任何 index/event 预算,两 fixture 都无法诚实触发截断面;伪造大 trace=改 fixture 红线,配置手术=硬造。留给未来更大素材 |
| | F4 selected_window 端点 | ✅(并入) | 并入 `real_trace_a2_wide_window_ratio`(typed 日志面 `selected_window=34579.4728…..34579.502…` 严格断言) |
| G 鲁棒 | G1 中英文变体 | ✅ | `real_trace_g1_english_dstate`(英文问法,同 C2 ground truth) |
| | G2 相对路径(仓内 fixture) | ✅ | `real_trace_g2_relative_path_inrepo`(问句内相对路径,无附件;README 指向仓内拷贝) |

计 18 新案;A-G 每维 ≥1 案;3 处 N/A(C1 改型、F3、VS-1/VS-2b 子口径)均有画像依据。

## 2. 逐案 oracle 依据

Oracle 设计红线(全批适用):断言只建在 PROFILE ground truth 数字与 typed
面(`phase=toolcall`、tool params、toolresult 的 `selected_window=` 行)上;
散文只允许"概念带"(状态词/诚实词的备选组),不做模糊断言;负向断言只用于
诚实面;数值断言一律用含真值的窄带(如 26.162ms→`2[5-9]…ms|8[5-9]…%`),
禁为过案放宽。本机 `grep` 是 ugrep 7.5.0,全部 ERE 已通过 ugrep 编译+正例
冒烟(有界重复 `{0,200}` 连用会触发 ugrep 复杂度上限——D2 链序断言已因此
改为 `.*` 间隔;后续新增 oracle 请复跑该冒烟)。

| # | 案 ID | 维度 | oracle 依据(→PROFILE) |
|---|---|---|---|
| 1 | real_trace_a2_wide_window_ratio | A2+F4 | §1.7 wide10:s_sleep 26.162ms=87.4% 主导,running 2.964ms。断言:两端点回显 + 睡眠主导概念带 + 数值窄带(25-29ms 或 85-89%)。**F4 严格 typed 面**:日志须现 `selected_window=34579.4728[0-9]*..34579.502[0-9]*`(问句给了 6 位小数端点,窗口保真是本维考点;若产品改写窗口即 FAIL=真 gap) |
| 2 | real_trace_a3_whole_trace_overview | A3 | §1.1 span 144.557ms/端点;§1.7 full running 50.524ms(busy 归因 51.462 → 带 50-51);§1.4 top:sysevent_store/hilogd。CONTAINS 三 token + 两条数值窄带 |
| 3 | real_trace_a4_out_of_range_window | A4 | §1.1 last_ts=34579.595184;请求窗全越界。断言:诚实词带 + 真实边界回显(34579.59/144.x ms)。**刻意不断言 tool 参数形状**(模型可从全窗查询诚实作答;精确信号=答案面诚实,工具探索形状=嘈声,只作软记录) |
| 4 | real_trace_a5_excerpt_degenerate_window | A5+F2 | §2.1 覆盖 0.556ms(2942.244845..2942.245401);§2.3 ColdPool#6 busy 0.367ms。断言:2942.24 回显 + 覆盖披露带 + 实际端点(TEXT) |
| 5 | real_trace_b2_tid_only_waker | B2+D1 | §1.4 59843=CookieMonsterCl tgid 59566;§1.6 唤醒 main 34 次(48 中最多)。断言:身份共现 + 唤醒关系 TEXT + 次数带(34/3x 次/最多) |
| 6 | real_trace_b3_process_level_rollup | B3 | §1.4 进程内 top:main 51.46 > Zeus 30.29 > NetworkService 13.14 > Cookie 8.49。断言:三 token + 主线程居首 TEXT + Zeus 量级带 |
| 7 | real_trace_b4_missing_thread_miss | B4 | §1.4 `grep -c '99999'`=0。断言:99999 回显 + 诚实 miss 词带。无法用正则禁"编造统计",依赖 miss 词带 + 复核时人工看 run-1.out(gap 表列预留) |
| 8 | real_trace_b5_multi_subject_render | B5 | §1.7:legacy115 窗 RenderThread 100% s_sleep(114.94ms),main running 24.99ms;RenderThread 只在 trace 尾被 main 唤醒(34579.590882/593245)。断言:SECTIONS 双主体 + RenderThread 全睡带 + main 活跃带 |
| 9 | real_trace_c2_dstate_iowait | C2 | §1.8:3 条 iowait=1,caller `sync_buffer_read_wi+0x60/0x11c`,ts 34579.4518/4531/4717;总量 0.488+0.147ms。断言:verbatim caller token(CONTAINS)+ 状态词带 + `34579.4(5|7)` 时间带 + 小量级诚实带 |
| 10 | real_trace_c3_vsync_periodic | C3 | §1.9:VSyncGenerator-1682;period:16552213ns≈16.55ms≈60Hz(verbatim)。断言:驱动线程 token + 周期数值带。不考 cadence(N/A 见矩阵) |
| 11 | real_trace_c4_freq_supply_evidence | C4 | §1.3:仅 cpu3-5 有采样,梯 807000..2189000;limits=0。断言:两端频点带 + 核 3 提及 + "非全核有采样"诚实 TEXT 带 |
| 12 | real_trace_d2_chain_via_networkservice | D2 | §1.6:NetworkService→CookieMonsterCl 31 次,CookieMonsterCl→main 34 次。断言:双中继 CONTAINS + 链序 TEXT(两向)+ 链概念带 |
| 13 | real_trace_d4_demand_vs_supply | D4 | §1.7 legacy115 sleep 84.358/114.94;§1.3 idle 269.8ms + fmax 2189000 观测顶格。断言:需求侧结论带 + 供给非瓶颈带 + 睡眠证据带(§7.4 需求/供给措辞车道的答案面投影) |
| 14 | real_trace_e1_dual_window_normalized | E1 | §1.7:jank running=0.000ms vs post30 running 3.414ms=11.4%;窗长差 10×。断言:三端点 SECTIONS + 归一化概念带 + "A 窗无 CPU"TEXT + B 窗数值带(3.x ms 或 10-11%)。CMP 教训(跨窗聚合必除窗长)的最小口径案 |
| 15 | real_trace_e2_cross_trace_asymmetry | E2 | §1 vs §2:144.557ms/0.556ms;34579.x/2942.x;excerpt 频率+vsync 双零。断言:双文件名 CONTAINS + 双 span 带 + 双时基 token + 单边未采样 TEXT + 时基不可对齐 TEXT;**typed 日志面**:params 必含 donghu_short_excerpt(证明两文件都真的查了) |
| 16 | real_trace_f1_exclude_no_code | F1 | §1.7 jank + §1.6 唤醒者。断言:CookieMonsterCl + 唤醒词 + 34579.4758 时刻;**负向**:`.codrax/blob`(附件 blob 路径伪引用零渲染=CPD 面)、still_present、not_enough_evidence(trace-only 答案禁源码状态措辞) |
| 17 | real_trace_g1_english_dstate | G1 | 同 #9 ground truth;英文问法,断言带中英双语状态词。验证语言变体不改变 typed 证据(caller token 必须仍出现) |
| 18 | real_trace_g2_relative_path_inrepo | G2 | §1.6 34/48 次(≈70.8%)。断言:CookieMonsterCl + 次数/占比带(34/3x 次/7x%);**typed 日志面**:params 必含 `real_traces/donghu_tieba_frame`(仓内相对路径被真实消费) |

已知张力(记录,不降 bar):#1 的 selected_window 严格面与 #15/#18 的
params 面是全批仅有的三处"工具面硬断言",都符合"精确信号才可作硬约束"
——三者断的都是 schema 化 typed 输出(结果 stanza 固定格式 / params JSON
回显),不是探索路径形状。若跑批时因模型改写端点而 FAIL,按窗口保真 gap
入表,不改 oracle。

## 3. 运行协议(主会话执行)

```bash
make   # 先构建 ./codrax(gate 先 make 教训)
PARALLEL=2 TIMEOUT=1800 \
EVAL_SELECTED_SUMMARY=eval/parallel_selected_summary_realtrace_batch1.md \
bash eval/parallel_selected.sh eval/cases/real_traces/*.case
```

- `parallel_selected.sh` 每案 N=1、快照二进制、自动生成
  `…_realtrace_batch1_manual_audit.md`;复跑单案用
  `bash eval/run.sh eval/cases/real_traces/<id>.case 3`(3 采样)。
- TIMEOUT=1800:1.9MB trace 案健康运行历史在 2-6 分钟;30 分钟上限只为
  吸收弱模型退化,不是预期时长。PARALLEL=2 避免与并发 CSP eval 争抢
  provider 限额。
- 子目录 **不在** `parallel_all.sh` 默认 glob(`eval/cases/*.case`)内:
  全量 sweep 需显式 `CASES_GLOB="eval/cases/real_traces/*.case"`,这是
  刻意的(不干扰在跑的 CSP 案,也不让 18 案悄悄进默认回归)。
- 本机 grep=ugrep:新增/修改 oracle 后跑 §2 开头的冒烟(ERE exit-2 检查)。
- summary 命名约定:`…_realtrace_batch<N>.md` 递增;gap 修复后的复跑用
  同名 batch+1,不覆写。

## 4. gap 记录表(跑批后填)

| # | 案 ID | 维度 | verdict | 现象(答案面/日志面) | 归因(产品 gap / oracle 过紧 / 素材极限) | 处置 | 状态 |
|---|---|---|---|---|---|---|---|
| | | | | | | | |

填表纪律:oracle 过紧必须以 PROFILE 数字论证后才允许改(改案=改单行断言,
禁降概念带下限);"LLM flake"禁用作归因;B4/A4 类诚实案 FAIL 一律先读
run-1.out 全文再归因。

## 5. 附录 A — 画像探针源码(已按约删除,此处为可复现存档)

复现方法:在仓根建 `tmp_probe_trace_profile/main.go` 粘贴以下内容,跑
PROFILE.md 头部记录的两条命令,用毕 `rm -rf tmp_probe_trace_profile`。

```go
// tmp_probe_trace_profile: TEMPORARY deterministic profiling probe for the
// real-trace campaign (2026-07-05). Parses a systrace fixture through
// internal/tracequery (the same parser the product uses) and dumps ground
// truth for PROFILE.md. DELETE AFTER USE — not part of the product.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type threadInfo struct {
	pid       int
	tgid      int
	comms     map[string]int
	busySec   float64
	switchIns int
}

func main() {
	pidFlag := flag.String("pids", "", "comma-separated target pids for state breakdown")
	winFlag := flag.String("windows", "", "semicolon-separated name:start:end windows for per-pid state breakdown")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: probe [-pids p1,p2] [-windows name:s:e;...] <trace>")
		os.Exit(2)
	}
	path := flag.Arg(0)
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "BuildIndex:", err)
		os.Exit(1)
	}

	fmt.Printf("== HEADER ==\n")
	fmt.Printf("path=%s size=%d\n", idx.Path, idx.Size)
	fmt.Printf("line_count=%d scanned=%d events=%d parsed_known=%d unparsed=%d panics=%d clock_regressions=%d\n",
		idx.LineCount, idx.ScannedLineCount, len(idx.Events), idx.ParsedKnown, idx.UnparsedLines, idx.ParseLinePanics, idx.ClockRegressions)
	fmt.Printf("first_ts=%.6f last_ts=%.6f span_ms=%.3f\n", idx.FirstTs, idx.LastTs, (idx.LastTs-idx.FirstTs)*1000)
	fmt.Printf("flavor=%s conf=%.2f signals=%v\n", idx.TraceFlavor, idx.FlavorConfidence, idx.FlavorSignals)
	for _, c := range idx.Caveats {
		fmt.Printf("caveat=%s\n", c)
	}

	// Event kind counts (typed) + raw name counts.
	typeCounts := map[tracequery.EventType]int{}
	nameCounts := map[string]int{}
	cpuCounts := map[int]int{}
	for _, ev := range idx.Events {
		typeCounts[ev.Type]++
		nameCounts[ev.Name]++
		cpuCounts[ev.CPU]++
	}
	fmt.Printf("\n== EVENT TYPE COUNTS (tracequery typed) ==\n")
	for _, kv := range sortedCounts(typeCounts) {
		fmt.Printf("%-28s %d\n", kv.k, kv.v)
	}
	fmt.Printf("\n== RAW EVENT NAME COUNTS (top 30) ==\n")
	for i, kv := range sortedCountsS(nameCounts) {
		if i >= 30 {
			break
		}
		fmt.Printf("%-32s %d\n", kv.k, kv.v)
	}
	fmt.Printf("\n== CPUS (events per cpu bracket) ==\n")
	cpus := make([]int, 0, len(cpuCounts))
	for c := range cpuCounts {
		cpus = append(cpus, c)
	}
	sort.Ints(cpus)
	for _, c := range cpus {
		fmt.Printf("cpu%d=%d ", c, cpuCounts[c])
	}
	fmt.Println()

	// cpu_frequency per governed cpu id; cluster shape from identical freq value sets.
	freqSets := map[int]map[int]int{}
	idleCPUs := map[int]int{}
	freqLimit := 0
	for _, ev := range idx.Events {
		switch ev.Type {
		case tracequery.EventCPUFrequency:
			cpu := ev.CPU
			if ev.CPUForFieldValid {
				cpu = ev.CPUForField
			}
			if freqSets[cpu] == nil {
				freqSets[cpu] = map[int]int{}
			}
			freqSets[cpu][ev.Frequency]++
		case tracequery.EventCPUIdle:
			cpu := ev.CPU
			if ev.CPUForFieldValid {
				cpu = ev.CPUForField
			}
			idleCPUs[cpu]++
		case tracequery.EventCPUFrequencyLimit:
			freqLimit++
		}
	}
	fmt.Printf("\n== FREQ / IDLE LANES ==\n")
	fmt.Printf("cpu_frequency_limits_events=%d\n", freqLimit)
	fkeys := make([]int, 0, len(freqSets))
	for c := range freqSets {
		fkeys = append(fkeys, c)
	}
	sort.Ints(fkeys)
	for _, c := range fkeys {
		vals := make([]int, 0, len(freqSets[c]))
		n := 0
		for v, k := range freqSets[c] {
			vals = append(vals, v)
			n += k
		}
		sort.Ints(vals)
		fmt.Printf("cpu_frequency cpu_id=%d samples=%d distinct=%d min=%d max=%d values=%v\n", c, n, len(vals), vals[0], vals[len(vals)-1], vals)
	}
	ikeys := make([]int, 0, len(idleCPUs))
	for c := range idleCPUs {
		ikeys = append(ikeys, c)
	}
	sort.Ints(ikeys)
	for _, c := range ikeys {
		fmt.Printf("cpu_idle cpu_id=%d samples=%d\n", c, idleCPUs[c])
	}

	// Thread inventory + per-cpu running attribution.
	threads := map[int]*threadInfo{}
	touch := func(pid, tgid int, comm string) {
		if pid <= 0 {
			return
		}
		t := threads[pid]
		if t == nil {
			t = &threadInfo{pid: pid, comms: map[string]int{}}
			threads[pid] = t
		}
		if tgid > 0 {
			t.tgid = tgid
		}
		if comm != "" {
			t.comms[comm]++
		}
	}
	type running struct {
		pid int
		ts  float64
	}
	cur := map[int]running{}
	idleSec := 0.0
	for _, ev := range idx.Events {
		touch(ev.PID, ev.TGID, ev.Comm)
		if ev.Type == tracequery.EventSchedSwitch {
			touch(ev.PrevPID, 0, ev.PrevComm)
			touch(ev.NextPID, 0, ev.NextComm)
			if r, ok := cur[ev.CPU]; ok {
				d := ev.Ts - r.ts
				if d > 0 {
					if r.pid > 0 {
						threads[r.pid].busySec += d
					} else {
						idleSec += d
					}
				}
			}
			cur[ev.CPU] = running{ev.NextPID, ev.Ts}
			if ev.NextPID > 0 {
				threads[ev.NextPID].switchIns++
			}
		}
		if ev.Type == tracequery.EventSchedWakeup || ev.Type == tracequery.EventSchedWaking {
			touch(ev.WakeePID, 0, ev.WakeeComm)
		}
	}
	fmt.Printf("\n== THREADS (total=%d; top 30 by attributed running time) ==\n", len(threads))
	fmt.Printf("(idle/pid0 attributed running: %.3f ms across all cpus)\n", idleSec*1000)
	all := make([]*threadInfo, 0, len(threads))
	for _, t := range threads {
		all = append(all, t)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].busySec > all[j].busySec })
	for i, t := range all {
		if i >= 30 {
			break
		}
		fmt.Printf("%-24s pid=%-6d tgid=%-6d busy_ms=%8.3f switch_ins=%d\n", mainComm(t.comms), t.pid, t.tgid, t.busySec*1000, t.switchIns)
	}

	// Threads of the target tgids (process-level subject ground truth).
	targetPIDs := parsePIDs(*pidFlag)
	tgidsOfTargets := map[int]bool{}
	for _, p := range targetPIDs {
		if t := threads[p]; t != nil && t.tgid > 0 {
			tgidsOfTargets[t.tgid] = true
		}
	}
	for tg := range tgidsOfTargets {
		fmt.Printf("\n== PROCESS tgid=%d THREADS (by busy) ==\n", tg)
		for _, t := range all {
			if t.tgid == tg {
				fmt.Printf("%-24s pid=%-6d busy_ms=%8.3f switch_ins=%d\n", mainComm(t.comms), t.pid, t.busySec*1000, t.switchIns)
			}
		}
	}

	// Priority distribution from sched_switch next_prio.
	prioCounts := map[int]int{}
	targetPrios := map[int]map[int]int{}
	for _, p := range targetPIDs {
		targetPrios[p] = map[int]int{}
	}
	for _, ev := range idx.Events {
		if ev.Type != tracequery.EventSchedSwitch {
			continue
		}
		prioCounts[ev.NextPrio]++
		if m, ok := targetPrios[ev.NextPID]; ok {
			m[ev.NextPrio]++
		}
	}
	fmt.Printf("\n== PRIORITY DISTRIBUTION (sched_switch next_prio, top 20) ==\n")
	for i, kv := range sortedCounts(prioCounts) {
		if i >= 20 {
			break
		}
		fmt.Printf("prio=%-6v switch_ins=%d\n", kv.k, kv.v)
	}
	for _, p := range targetPIDs {
		fmt.Printf("target pid=%d observed next_prio values: %v\n", p, keysOf(targetPrios[p]))
	}

	// Wakeup edges.
	wakeupTotal, wakingTotal := 0, 0
	pairCounts := map[string]int{}
	fmt.Printf("\n== WAKEUP EDGES TO TARGETS ==\n")
	for _, ev := range idx.Events {
		if ev.Type != tracequery.EventSchedWakeup && ev.Type != tracequery.EventSchedWaking {
			continue
		}
		if ev.Type == tracequery.EventSchedWakeup {
			wakeupTotal++
		} else {
			wakingTotal++
		}
		pairCounts[fmt.Sprintf("%s-%d -> %s-%d", ev.Comm, ev.PID, ev.WakeeComm, ev.WakeePID)]++
		for _, p := range targetPIDs {
			if ev.WakeePID == p {
				fmt.Printf("line=%-6d ts=%.6f %s waker=%s-%d(tgid=%d) wakee=%s-%d prio=%d target_cpu=%d\n",
					ev.Line, ev.Ts, ev.Type, ev.Comm, ev.PID, ev.TGID, ev.WakeeComm, ev.WakeePID, ev.WakeePrio, ev.TargetCPU)
			}
		}
	}
	fmt.Printf("\n== WAKEUP EDGE TOTALS ==\nsched_wakeup=%d sched_waking=%d\n", wakeupTotal, wakingTotal)
	fmt.Printf("top waker->wakee pairs:\n")
	for i, kv := range sortedCountsS(pairCounts) {
		if i >= 15 {
			break
		}
		fmt.Printf("%-64s %d\n", kv.k, kv.v)
	}

	// Binder samples.
	fmt.Printf("\n== BINDER SAMPLES (first 5 transactions) ==\n")
	nb := 0
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventBinderTransaction && nb < 5 {
			fmt.Printf("line=%d ts=%.6f %s-%d: %s\n", ev.Line, ev.Ts, ev.Comm, ev.PID, ev.FieldText)
			nb++
		}
	}

	// Vsync / periodic-source signals.
	fmt.Printf("\n== VSYNC / PERIODIC SOURCE SIGNALS ==\n")
	vsyncComm := map[string]int{}
	vsyncSpan := map[string]int{}
	spanNames := map[string]int{}
	for _, ev := range idx.Events {
		lcComm := strings.ToLower(ev.Comm)
		if strings.Contains(lcComm, "vsync") {
			vsyncComm[fmt.Sprintf("%s-%d", ev.Comm, ev.PID)]++
		}
		if ev.Type == tracequery.EventTraceMark {
			if ev.SpanName != "" {
				spanNames[ev.SpanName]++
				if strings.Contains(strings.ToLower(ev.SpanName), "vsync") {
					vsyncSpan[fmt.Sprintf("%s|%s", ev.SpanAction, ev.SpanName)]++
				}
			}
		}
	}
	fmt.Printf("threads with vsync in comm (event lines emitted by them):\n")
	for _, kv := range sortedCountsS(vsyncComm) {
		fmt.Printf("  %-28s %d\n", kv.k, kv.v)
	}
	fmt.Printf("trace_mark spans containing vsync:\n")
	for _, kv := range sortedCountsS(vsyncSpan) {
		fmt.Printf("  %-48s %d\n", kv.k, kv.v)
	}
	fmt.Printf("top 20 trace_mark span names overall:\n")
	for i, kv := range sortedCountsS(spanNames) {
		if i >= 20 {
			break
		}
		fmt.Printf("  %-48s %d\n", kv.k, kv.v)
	}

	// Blocked reasons for targets.
	fmt.Printf("\n== sched_blocked_reason FOR TARGETS ==\n")
	for _, ev := range idx.Events {
		if ev.Type != tracequery.EventSchedBlockedReason {
			continue
		}
		for _, p := range targetPIDs {
			if ev.WakeePID == p {
				fmt.Printf("line=%d ts=%.6f pid=%d iowait=%d reason=%s\n", ev.Line, ev.Ts, ev.WakeePID, ev.IOWait, ev.Reason)
			}
		}
	}

	// State breakdown per target pid per window (product-parity ThreadTimeline).
	windows := parseWindows(*winFlag, idx.FirstTs, idx.LastTs)
	for _, p := range targetPIDs {
		for _, w := range windows {
			q := tracequery.Query{PID: p, TimeStart: w.s, TimeEnd: w.e, TimeStartSet: true, TimeEndSet: true}
			tl := tracequery.ThreadTimeline(idx, q)
			agg := map[tracequery.ThreadState]float64{}
			frag := map[tracequery.ThreadState]int{}
			for _, iv := range tl.Intervals {
				agg[iv.State] += iv.DurationMs
				frag[iv.State]++
			}
			fmt.Printf("\n== STATE BREAKDOWN pid=%d window=%s [%.6f, %.6f] (%.3f ms) thread=%s tgid=%d ==\n",
				p, w.name, w.s, w.e, (w.e-w.s)*1000, tl.Thread.Comm, tl.Thread.TGID)
			total := 0.0
			for _, st := range []tracequery.ThreadState{tracequery.StateRunning, tracequery.StateRunnable, tracequery.StateSSleep, tracequery.StateDSleep, tracequery.StateIOWait, tracequery.StateStopped, tracequery.StateDead, tracequery.StateUnknown} {
				if agg[st] > 0 {
					fmt.Printf("%-10s %10.3f ms  fragments=%d\n", st, agg[st], frag[st])
					total += agg[st]
				}
			}
			fmt.Printf("covered_total=%.3f ms window=%.3f ms intervals=%d\n", total, (w.e-w.s)*1000, len(tl.Intervals))
			for _, c := range tl.Caveats {
				fmt.Printf("caveat=%s\n", c)
			}
			if w.verbose {
				for _, iv := range tl.Intervals {
					fmt.Printf("  iv %-10s %.6f -> %.6f dur=%8.3fms lines=%d..%d wake_line=%d prev_state=%s cpu=%d(known=%v)\n",
						iv.State, iv.StartTs, iv.EndTs, iv.DurationMs, iv.StartLine, iv.EndLine, iv.WakeupLine, iv.PrevStateRaw, iv.CPU, iv.CPUKnown)
				}
			}
		}
	}
}

type kvI struct {
	k interface{}
	v int
}

func sortedCounts[K comparable](m map[K]int) []kvI {
	out := make([]kvI, 0, len(m))
	for k, v := range m {
		out = append(out, kvI{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].v != out[j].v {
			return out[i].v > out[j].v
		}
		return fmt.Sprint(out[i].k) < fmt.Sprint(out[j].k)
	})
	return out
}

type kvS struct {
	k string
	v int
}

func sortedCountsS(m map[string]int) []kvS {
	out := make([]kvS, 0, len(m))
	for k, v := range m {
		out = append(out, kvS{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].v != out[j].v {
			return out[i].v > out[j].v
		}
		return out[i].k < out[j].k
	})
	return out
}

func mainComm(m map[string]int) string {
	best, n := "?", -1
	for k, v := range m {
		if v > n {
			best, n = k, v
		}
	}
	return best
}

func parsePIDs(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if v, err := strconv.Atoi(part); err == nil {
			out = append(out, v)
		}
	}
	return out
}

type window struct {
	name    string
	s, e    float64
	verbose bool
}

func parseWindows(s string, first, last float64) []window {
	out := []window{{name: "full", s: first, e: last}}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		bits := strings.Split(part, ":")
		if len(bits) < 3 {
			continue
		}
		ws, err1 := strconv.ParseFloat(bits[1], 64)
		we, err2 := strconv.ParseFloat(bits[2], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		verbose := len(bits) > 3 && bits[3] == "v"
		out = append(out, window{name: bits[0], s: ws, e: we, verbose: verbose})
	}
	return out
}

func keysOf(m map[int]int) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
```
