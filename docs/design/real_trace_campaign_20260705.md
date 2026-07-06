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
| 1 | real_trace_f1_exclude_no_code | F1 | FAIL `banned:.codrax/blob`(20260705-190709) | 答案体"证据索引"导语原样渲染机器本地绝对路径 `` `/Users/…/.codrax/blob/20260705-190709-000-26634/attached_trace.txt` ``(run-1.out:115,在 ━━━ 后=banned 扫描域内)。**防线本身全部正常**:exclude 判定命中(`external_observation_policy=exclude/artifact_citation=external_only`,anchored quote "只分析这份 trace，不要读取或引用任何源码文件",logs:500-502);authority 行 lane=excluded/combined_proof_ready(logs:1060);引用池 0 条,CPD arm 剔除 4 条运行时引用并渲染注记(emit_answer_document_v2.go:830,run-1.out:138);CSP 无 still_present/not_enough_evidence 污染。命中面=(c) 展示层 render 注入,非 (a) 引用池、非 (b) 模型散文 | **产品 gap(展示层)**:v3 投影证据索引分组导语 `runtimeTraceProjEvidenceBlockParts`(answer_document_mutation_runtime_tree.go:4008)对 sole-artifact `sharedFile` 逐字输出 SupportRefs 携带的 blob 绝对路径;同一 roster 的 synthetic locator 已 basename 化(`runtimeTraceCausalProjectionPathTail(path,1)`,同文件 :3775),"系统补充"面也只显 `attached_trace.txt` — 导语是唯一裸吐全路径的显示面。CPD 闸只覆盖 citation pool/items ref,从未覆盖此 CMP 批新增导语。berlin golden 恰好 ref 本就是短名(`berlin.systrace`)故未暴露 | 修向选项(待裁定,不拍板):(a) 导语 sharedFile 过 `PathTail(…,1)` basename,与 E# 条目/系统补充同约定,一行改;(b) 经 runtime artifact registry 映射回原始附件显示名(`donghu_tieba_frame.systrace`),信息更友好但跨层。两案均为系统生成文案,不触"系统不代替 LLM 写答案"红线;trace_query 原始记录保留全路径,审计无损。附注:进度流 tool-call 回显(run-1.out:27)也含 blob 路径但在 banned 域外(scope_stdout 只取 ━━━ 后),暂不动 | OPEN(已归因) |
| 2 | real_trace_e2_cross_trace_asymmetry | E2 | FAIL `no_text_regex_match:(时基…)`(20260705-190358) | 行为链完好:两 trace 各查(window_stats×2+event_search×4×两轮,params 含 donghu_short_excerpt,logs:725-726/799-802/1300-1301/1379-1382);两 span 端点均取到(34579.450627..595184 / 2942.244845..245401);答案面已明确披露 "绝对时间基准相差巨大（≈34579s vs ≈2942s），无法在真实时间轴上直接对齐"(run-1.out:75)。复算:对 ━━━ 后 body 跑该 ERE = 0 hit——"无法…直接"被"在真实时间轴上"拆开,"相差巨大"不在字带内 | **oracle 过紧**(答案正确、正则漏配合法表述)。PROFILE §1 vs §2 数字复核支持答案:时基 34579.x/2942.x 区间不相交,答案陈述与 ground truth 一致。per-class 附注(非本 FAIL 根因):时基不可比披露今日完全靠模型散文——系统无"跨 trace span 不相交"typed 信号;CMP-6 对比引导只有同口径窗/归一化两行(runtimeTraceNextStepComparisonSteps),且本 run ledger 只编译出 1 个 active projection、is_cross_component=false,CMP-6/F3 行均未触发 | 改 oracle 单行(时基臂增列合法表述,如 `相差|不在同一|无法.{0,40}对齐|不能.{0,40}对齐`),概念带不降,已有 PROFILE 复核依据。可选加固(独立小批):两 per-trace selected_window 区间交集为空 → typed 对比提示(纯算术精确信号,挂对比显示面),把披露从"散文碰运气"升为系统面 | OPEN(已归因) |
| 3 | real_trace_e1_dual_window_normalized | E1 | FAIL `no_text_regex_match:(3\.x ms\|1[01]%)`(20260705-190231) | 答案 B 窗=0.912ms/3.0%(oracle 期望 3.414ms/11.4%);A 窗=0.987ms/33.0%(ground truth 0.000ms——A 窗首条 TEXT 正则因 "30.000 ms" 含 "0.000 ms" 子串假阳通过,A 侧错误未被检出)。行为链:模型对 B 窗跑了 thread_timeline(34579.475857..34579.505857)+event_search 多轮(logs:1324-2024);**工具 payload 已有精确答案**(trace-query-result-3d6f460b.json:48 intervals,running Σ=3.414ms/19 段、runnable 0.780/19、s_sleep 25.806/10,与 PROFILE §1.7 post30 完全一致),但渲染文本 12 条截断 "omitted 36 interval(s); see payload_ref" 且**无 per-state 合计行**;头部又教 "drill down…instead of reading this payload directly";read_file payload 被拒;模型退化为对截断 event_search 手工配对求和 → 0.912ms。A 窗 33% 来源:同一工具结果里时间线行 `running …472865..…472865 0.000ms`(clamp 后,正确)与 Evidence pack 行 `running for 0.987 ms`(未 clamp)自相矛盾且无 ⚠ 跨窗披露,模型采信后者 | **产品 gap(工具展示面),两个子 gap**:E1-a interval `Summary` 在 `makeIntervalWithWake`(tracequery/query.go:1205)用未 clamp 时长生成,`clampIntervals`(query.go:1240 起)重算 DurationMs 但不再生 Summary,`evidenceFromTimeline`(query.go:12973)原样入 Evidence pack——泄露 actual 时长冒充窗内值,违背 root_cause_rank 已确立的 projected_*/actual_*+⚠ 双口径纪律(typed 数据里 actual_* 字段其实都在,只是 Summary 面漏披露);E1-b thread_timeline 渲染文本(tool/trace_query.go:2918-2927)只列前 12 条无全量合计,>12 intervals 的窗口没有任何权威聚合可消费,诱导手工求和。**oracle 不改**(工具值与 PROFILE byte-一致,画像过硬;"查了值不同"不成立——是展示面把对的值藏住了) | 修向(确定性渲染改,无裁定张力):E1-a clamp 后再生 Summary 或双口径披露("窗口内 0.000ms(实际 0.987ms,越窗前)"),Evidence pack 同步;E1-b 渲染文本加全量 intervals 的 per-state totals 行(`totals running=3.414ms/19 …`,截断前计算)——两修任一即可救本案,双修互补。oracle 卫生附注(随修复批带走):e1 首条 TEXT 正则 `0(\.0+)? *ms` 会匹配 "30.000 ms" 子串,应加左边界(如 `(^\|[^0-9.])0\.0+ *ms`) | OPEN(已归因) |

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

## §5 战役收口(2026-07-05)

**终局:18/18 全绿**(首轮 15/18 → 三 FAIL 全归因全修复 → 复跑全过)。修复对照:

| 案 | 根因 | 修复 | commit |
|---|---|---|---|
| f1 | 证据索引导语裸吐 blob 全路径(全树唯一,防线全守住) | T11 basename 化 + pin | e24fb4dd(PTV4) |
| e2 | oracle 字带过紧(答案披露正确) | 臂扩合法措辞;RTC-2 #67 立案 typed 提示 | 1ffe777e |
| e1 | 四跑四形态:①端点精度(oracle 修:接受 ms 舍入)②clamp 双口径泄漏+timeline 无合计行(**真产品 gap**,R1 修:Summary 重生+state_totals+访问器单权威)③actual_* 误读加总(R1b 修:口径守护句)④A 臂合法措辞(oracle 臂扩) | R1=39eaecfc;R1b+oracle 臂=本 commit | 39eaecfc + 本批 |

e1 的四跑链条本身就是战役价值的浓缩:两次 oracle 精度校准(合法表达形态)、两次真 gap(双口径泄漏被修、误读车道被守护)。B 窗数值(3.414ms/11.4%)从"只存在于不可读 payload"变为"state_totals 权威块+守护句",最终 run 主线程双窗对比完整正确。弱模型视角漂移(第三跑进程级透镜)作为行为面残余记录,不立硬账(软引导域,同 DL-B 处置)。

开放项:客户回访验证(外部依赖)。~~RTC-2 #67~~ **已交付**(跨 trace span 不相交 typed 提示):纯算术精确信号 `types.TraceCausalProjectionTimeBasesDisjoint`(各分区 TimeBaseSpan=成员 impact 包络∪锚窗;与锚窗 F1 裁定的区别已在实现注释注明——本信号只判分区间时基可比性,永不作窗口锚),软引导双面落点=对比总览表尾注记行 + CMP-6 next-step 引导行(同一信号,两面锁步;单分区/交集非空/任一分区无时间证据=零发射,无任何硬门);pin=`trace_causal_projection_timebase_rtc2_test.go` + `answer_document_projection_timebase_rtc2_test.go`(zh+en verbatim 双面在/不在 + 判空反转与 fail-closed 反转双突变探针已验红)。

## §6 PTV5 批(#68 总装,2026-07-05 晚用户裁定集)

### §6.1 裁定原文(PTS/Q3,永久记录)

- **PTS(on-chain 完整性)裁定原文**:「凡 on-chain 项必须提及+进树,多则折叠+计数,审计全部丢弃/封顶点(span cap 8/桶入树/R3 边界),pin=合成超 cap 零静默丢弃。」
- **Q3 裁定原文**:「显示层一棵树+树头声明 N 窗+快照按窗分组(NEW-8 display 用途 per-record selected_window);对比门扩"单工件多锚窗"支复用 CMP-9 归一化(census=capture 语言不挪用);CMP-6 directive 加双窗对比→逐窗因果采样引导。B 方案(per-锚窗分区)明确不做留未来。」

### §6.2 PTS 丢弃/封顶点审计结论

| 点 | 位置 | 结论 | 处置 |
|---|---|---|---|
| 桶入树 cap | types/trace_causal_projection.go OnChainCauses limit=24 | **曾静默丢弃**(直接 on-chain 面) | 折叠+计数:`traceCausalProjectionLimitNodesOnChainFold`,溢出并入一条 subjectless fold 行(值=成员 MAX,墙钟不求和;roster+全量 evidence id 保留;`OnChainOverflowFold` typed 字段) |
| wire 家族 cap(32) | tool/trace_query.go wakeup_causal_impact 循环(即裁定所称 span cap 面;实测家族 cap=32,引擎 8 branch×深度可超) | **曾静默 break** | 溢出 on-chain 成员并入一条 fold 记录(folded_rows/folded_min_ms/folded_max_ms/folded_subjects 四个 NKR hard_consumer 键;编译端 re-materialize 成 fold 节点) |
| 引擎 aggregate top-8 | tracequery/query.go aggregateWakeupCausalImpacts | 派生视图截断(逐 hop CausalImpacts 完整保留,非 on-chain 项丢弃) | Caveat 计数披露(`aggregated_impacts kept top 8 of N`) |
| occurrence windows cap 8 | query.go:20/7786(字面 "span cap 8") | **本就是折叠+计数**:OccurrenceCount 保全量计数,windows 留 top-8 | 审计记录,无改动 |
| root_evidence/wakeup_chain_edge 家族 cap | tool/trace_query.go | root_evidence 记录无 relevance note→不入 OnChainCauses(仅 SupportingHops);edge 记录不建树节点(树干走 WakeupPath note) | 审计记录;不在"on-chain 项进树"面 |
| R3 边界 | aggregate.go traceCausalProjectionFoldUnknownBackground | 仅 BackgroundCauses(unknown-thread),on-chain 项不可达 | 审计记录,无改动 |
| Primary cap 10 | trace_causal_projection.go | on_chain primary 的 classified 副本仍入 OnChainCauses(桶重叠语义)→非树面丢弃 | 审计记录 + 代码注释 |

树面渲染:fold 行 `其余 N 项(链上折叠)(名单 等)`,新 mark `OnChainOverflowFold` + 图例口径组 entry;RN-3(a) fallback lead 与 ❶❷❸ badge 永不选 fold 行。pin=`TestTraceCausalProjectionOnChainOverflowFoldsWithCount`(合成 30>24,零静默丢弃逐 id 核对)+ under-cap 突变 + `TestPTV5OnChainFoldRowRendersAndNeverLeads` + emit fixture 超 cap(fold 记录 folded_* 四键)。

### §6.3 本批交付清单(八部分)

1. **P1-A C00**:主行 ms 回退口径可辨识 — `runtimeTraceProjNodeDisplayImpactSource` 四态来源;回退行内加 (a) 表口径词 MainRow tag(链上累计/有效归因/实际状态)+ 新图例 entry;占窗% 与 H8 >100% mark 只在窗口投影源发布(虚假触发面同修)。
2. **P1-B C13**:两态句括号删 wakeup_chain(gate 只读 root_cause_ 前缀),zh+EN,pin 负向禁回潮。
3. **PTS**:见 §6.2。
4. **Q1**:wakeup_causal_impact 补发 effective_impact_ms(镜像 rank lane 语义:periodic→VS-1 车道独家;inversion→R5d gated;普通→raw;NKR 键既有 hard_consumer 行,零新增);树行 `有效归因X` 常显 tag(gate=值>0,periodic/承自/effective-源三面防双打);表 (a) "—" 诚实性不变(值系上游发布,非显示层兜底)。
5. **Q2**:图例口径组新 `已归因/未归因` 覆盖行 entry(mark 于覆盖句渲染点);hop-only 形态信息行「目标睡眠 X 中 Y 已由链上解释」(X=sleep 族 hop 自身行 MAX,不求和;算术不动,attributed>X 禁猜跳过)。
6. **Q3**:`TraceCausalProjection.QueryWindows`(selected_window 单一 strict parser,±1ms 去重,display-only,锚窗车道不读);树头 `本报告数据来自 N 个查询窗` 行(≥2 窗);快照 tier 内按窗排序+`查询窗 a–b s ·` 标签前缀(CMP-4a tier 首序不动);对比门旁单工件多锚窗支(恰 1 投影+≥2 窗→CMP-9 归一化行+逐窗因果采样行);CMP-6 directive 第 3 行=双窗/多窗对比逐窗因果采样(cap=4 下 disjoint 对比形态 per-record 行让位,CMP-6 头条裁定使然)。census=capture 语言未挪用(工件身份 fold 未参与任何窗口分支)。
7. **Q4**:R1 absorb Object 空路径打通(空 survivor.Object←loser cause token;冲突面 影响点 lane 原样);priority_inversion_candidate 升 typed 节点字段(NKR display_only→hard_consumer+常量,producer 三处literal→常量,absorb OR,InversionRow=字段∨Object token);ActionCell runnable 词 调度/优先级→**调度等待**(scheduling wait,词表巧合消歧,负向 pin 禁"优先级"回潮)。
8. **44 条措辞**:confirmed 落地 30 条;死代码 `runtimeTraceCausalProjectionIntro` 删除(C27/C43)。

### §6.4 裁定冲突跳过清单(refuted 面为准)

| 项 | 冲突面 | 结论 |
|---|---|---|
| C04/C08/C23(覆盖行 on-chain→链上/分母口径词/百分比格式) | R09 + v3 设计稿:54 verbatim(`on-chain 已归因 <ms>/<pct>%,未归因残差 …`)+ 多处 golden pin | **跳过**;可辨识性由 Q2 图例覆盖行 entry 承担(不动句面) |
| C05(RN-3(a) 回退注两分支改写) | R10(flat 分支 CMP-7a)+ R11(rank=RN-3 裁定词) | **跳过** |
| C14/C32(▒ 图例 on-chain→链上) | R01 + v3 设计稿:77 verbatim | **跳过** |
| C17/C36 部分 | R02("top 片段"+source token=RN-12 账本 verbatim) | **部分落地**:仅 class 词走 D4 `可运行等待（runnable）`;top 片段+source 原样 |
| C19 部分 | R14("running 时间"=§7.2 CMP 设计原文) | **部分落地**:仅 同窗→各自同口径窗口内 |
| C28 部分 | R19(窗口名不统一为"分析窗口") | **部分落地**:定义句用本句原有"用户窗口" |

### §6.5 残余

- ~~引擎 aggregate top-8 的溢出对不进树(caveat 计数披露,数据未保留);如需进树须扩 WakeupCausalAggregate 合成 fold 成员,留未来裁定。~~ **已裁定并交付**(PTS-2 裁定①,#69 用户条件裁定 2026-07-06,评估+实施见 §7.1/§7.3)。
- Q3 B 方案(per-锚窗分区)按裁定明确不做。
- ~~comparison 4-item cap 下 disjoint 双 trace 形态 per-record 行被三固定行+RTC-2 行挤出(CMP-6 头条优先裁定的直接推论);如需并存须裁 cap 提升。~~ **已裁定并交付**(PTS-2 裁定②,#69 用户条件裁定 2026-07-06,评估+实施见 §7.2/§7.3;对比行优先序与总览表锁步不变)。

### §6.6 对抗复核批(2026-07-06,19 confirmed 全收)

**P1 簇一(fold 口径洗白+聚合前重复计数)**:① fold-cap 移到 `traceCausalProjectionAggregateForPresentation` **之后**(compile 内联,bucket 进聚合时不封顶)— 计数=R1/R4/V4/R2 合并后的真值(旗舰 donghu 重放实证的假"其余 16 项"重复行类根除);pin=`TestTraceCausalProjectionOnChainFoldCountsPostAggregationTruth`(26 distinct+4 R1 dup→fold 恰计 2,零静默丢弃跨 merge 保持)。② fold 值携带口径源:MAX 成员是窗口投影→ImpactMS 车道(可发 %);cumulative-only→仅 CumulativeImpactMS(ImpactMS=0→C00 管道印"链上累计"、零 %、(a) 表窗口投影列"—");pin=`TestTraceCausalProjectionOnChainFoldCarriesCaliberSource`+`TestPTV5CumulativeOnlyFoldRowCarriesCaliberWordNoShare`。

**P1 簇二(C00 tag 破 100 列)**:口径词宽度并入名字预算 reserve(`runtimeTraceProjRowMainReserve`,与 tag 发射同一源 `runtimeTraceProjRowFallbackCaliberWord`,零漂移;宽度 pass `consider` 同 reserve);floor 随 reserve 侵蚀(下限 8=省略行 roster 同款身份底线)。全宽度扫描 pin=`TestPTV5FallbackRowsHoldRowCapFullWidthSweep`:常见对(口径词+E#)全深度恒 ≤100;重型跨窗回退形态(口径词+⚠实际+E#(+N))zh ≤深度5 恒 ≤100、en ≤深度3,更深走量化 plateau(zh 104@6/108@7,en 101@4/105@5/109@6/113@7,**全面优于批前 ⚠+⊘+E# 三重合 plateau zh109/en118**,ceiling 只许降)。

**Med**:结论行 % 加 C00 同源门(仅窗口投影源发布 (占窗X%);链上累计/有效归因/实际状态源只印口径词——v3 golden/RN-3(a) pin 随改+负向禁 占窗 回潮);Q1 真镜像=引擎导出单一权威 `tracequery.WakeupCausalImpactEffectiveImpactMs`(rank lane 逐分支:periodic→VS-1 折算/gated>0 反转→R5d/其余→TotalMs→state 合计→blocking 兜底;gated=0 反转同 rank 落 TotalMs),tool 消费同一函数,pin=`TestWakeupCausalImpactEffectiveMirrorsRankLane`(8 形态双 lane 同输入同输出);Q3 快照门与渲染同读 ELIGIBLE 集合(CMP-4a 排除先行)+multi-window per-窗 floor≥1 槽(预算=max(2,窗数)),pin=`TestPTV5SnapshotPerWindowFloor`(A 窗双候选不再挤没 B 窗)。

**Low 全收**:QueryWindows cap-8 加 `QueryWindowsTruncated`(树头/next-step 渲染 "≥8 个查询窗",禁假精确数;dup 过 cap 不算截断,双 pin);±1ms 容差导出单一权威 `types.TraceCausalProjectionSameWindowToleranceS`(tool 快照分组消费,literal 清除,pin);badge fold 排除加 typed gate(Rank/Effective 非零合成→仍无 badge,"永不"兑现 pin);gofmt 两文件(runtime_tree.go enum 对齐+emit pin test);fold 行 (b) 块全名命名 fold lane(`其余 N 项(链上折叠)(名单)`,非"(未命名因果节点)",pin);Q1 双 carrier 合一(periodic∧Effective 回退源→C00 词让位 VS-1 tag,单 carrier pin);periodic∧inherited 实非互斥→`runtimeTraceProjEffectiveInherited` 加 `!PeriodicSource` typed guard(10× 启发式在 periodic 行禁用,pin+突变);Q3 4-item cap 挤出=已裁推论,维持 §6.5 记录不修。

**复验**:54 个真实存储 payload(8 个 2026-07-05 会话,3673 records)全链路重放(typed observations→ledger→compile→cluster render)= 零 >100 列 fence 行、零裸 % 口径行、零异常 fold;donghu 旗舰工件本机不存(../customlogs 缺),重复计数类由合成 R1-dup pin 覆盖,真机复验留待工件可达时回访。复核发现的两个非批内 zz_* 探针已清扫。

## §7 PTS-2 批(#69 用户条件裁定 2026-07-06:"如果没有风险(性能/内存)则动态扩充"——先评估后实施)

两条裁定均为条件裁定,协议=风险评估(数字化)先落本账本,再动手;任一评估显示非 O(1)/非有界增长则停在评估。

### §7.1 裁定①评估:引擎 aggregate top-8 溢出进树(§6.5 残余第 1 条)

探针:`tmp_probe_pts2_aggfold`(临时,按约用后删除;方法与 §5 附录 A 探针同款 —— `tracequery.BuildIndex` + `Run(view=wakeup_chain)`,分组复算与引擎同 key `%d/%s` 同过滤同 ≥2 门槛),工件 `eval/fixtures/real_traces/donghu_tieba_frame.systrace`(15,623 行),2 目标 PID(59566/59891)× 5 窗(jank/wide10/post30/legacy115/full)= 10 形态。

**实测 — 真实溢出组数量级:**

| 形态(pid×窗) | impacts | distinct 组 | eligible(≥2occ) | 发射 aggregates | 溢出(rank>8) | 查询耗时 | 查询分配 |
|---|---|---|---|---|---|---|---|
| 59566×jank | 2 | 1 | 0 | 0 | **0** | 19.8ms | 5.2MiB |
| 59566×wide10 | 13 | 3 | 3 | 3 | **0** | 27.6ms | 16.4MiB |
| 59566×post30 | 12 | 3 | 2 | 2 | **0** | 27.3ms | 16.1MiB |
| 59566×legacy115 | 24 | 5 | 5 | 5 | **0** | 43.5ms | 41.9MiB |
| 59566×full | 24 | 5 | 5 | 5 | **0** | 49.1ms | 52.4MiB |
| 59891×5 窗 | 1–7 | 0–5 | 0 | 0 | **0** | 18.6–47.1ms | 4.4–48.5MiB |

真实 trace 上 top-8 从不触发(eligible 最大 5)→ **≤8 组零发射面是常态**,fold 仅在重负载 trace 出现;≤8 组时字段=nil(omitempty 0 字节)、零 wire 记录、零树行,反噪音默认成立。

**实测 — 合成 fold 成员增量(合成 9/50/308 组 → 溢出 1/42/300,循环形状=实施同款:count+min/max+roster≤8+行/时间戳包络单遍):**

| 溢出组数 | 合成耗时/次 | 合成分配/次 | wire 记录增量(notes+summary) |
|---|---|---|---|
| 1 | 68ns | 64B | ≈326B |
| 42 | 686ns | 624B | ≈492B |
| 300 | 1.82µs | **624B(roster 满 8 后封顶,O(1))** | ≈497B(与组数无关) |

对比查询本体 19.6–49.1ms / 4.4–52.4MiB:增量 **<0.01% 耗时、<0.002% 内存**;时间 O(G) 且 G(全部组)已在聚合本体中付过,无新渐近项;内存 O(1)(合成成员 ≤624B + wire ≤0.5KiB + 树面恰 1 折叠行)。

**评估结论:无险 → 实施。**落点:`ChainResult.AggregatedImpactsFold`(typed 有界合成成员,不进 `AggregatedImpacts` 切片 —— 该切片被 root_cause_rank(query.go:8330)与 chainThreadRefs(:9122)直接消费,合成成员入列会污染排名与线程表)→ tool 端 `traceQueryWakeupCausalAggregateFoldRecord`(镜像 impact wire fold 构造)发射恰一条 `wakeup_causal_aggregate` fold 记录(**复用 NKR 折叠族四键 folded_rows/folded_min_ms/folded_max_ms/folded_subjects,零新键族**)→ 编译端走既有 `TraceCausalProjectionFromObservationRecords` folded_* re-materialize(MergedCount/口径/永不领衔/永不徽章/折叠行渲染全部现成)。fold 值=成员 MAX 永不求和(墙钟既有裁定);top-8 caveat 字节不变。

桶溢出退化面(复核 F1 裁定 2026-07-06 = **计数吸收,不做桶位豁免**):引擎 fold 行若落入编译端 on-chain 桶 fold(`traceCausalProjectionLimitNodesOnChainFold`)的溢出段,退化=不再单独渲染,但计数/证据无损——桶行 N 吸收其 FoldedRows(G 组计入,凭 typed `OnChainOverflowFold` 标记而非 1 项),roster 并入其 subjects(全局 cap 不变),证据 ID 仍逐记录吸收(+N 面不变);普通 ×N 聚合行仍计 1 不受影响。

### §7.2 裁定②评估:comparison next-step 4-item cap 动态扩(§6.5 残余第 3 条)

cap 扩=纯显示列表长度,无算法/无数据面变化:

- **行字节实测**:3 固定对比行 = 90/89/119B,RTC-2 行 = 147B,合计 445B —— disjoint 双 trace 形态现状恰满 cap=4,per-record 行 0 槽(cmp6 残余记录面)。per-record zh 行 40–120B(kind 映射 40–90B,动态 runnable 变体 ≤~120B)。
- **动态 cap 设计**:comparison 形态(gate 不变=≥2 active 投影,对比行⟺总览表锁步不破)cap = 已发射对比行数 + per-record 保底 2 槽(下限仍 4);**硬上界 = 4+2 = 6 行**。对比行全集永不截断(现 ≤4 行,本就 ≤cap;护未来家族增行)。
- **增量数字**:disjoint 双 trace 形态 4→≤6 行,+2 行 ≈ +240B ≈ +80 token;非 disjoint comparison 形态 4→≤5 行(3 固定+2 保底),+1 行 ≈ +120B;**非 comparison 形态 cap=4 字节不变**(动态项仅 comparison gate 内生效)。保底 N=2 取值依据:双 trace 形态两侧 trace 的 top per-record 引导各得一槽。
- **耗时/内存**:零新 compile(comparison gate 布尔与既有调用同源,提升复用),ledger 记录循环仍受 64 条编译上限约束,O(1)。

**评估结论:无险 → 实施。**优先序不变(对比行领衔),对比行⟺总览表锁步键(runtimeTraceProjComparisonShape)不动。

### §7.3 as-built 交付(评估落账后实施)

**裁定①(引擎 aggregate 溢出进树)落点:**

- 引擎:`internal/tracequery/types.go` 新 typed 有界类型 `WakeupCausalAggregateFold`(Groups+Min/MaxImpactMs+Subjects≤8+行/时间戳包络)+ `ChainResult.AggregatedImpactsFold`(nil=≤8 组零发射,omitempty;**刻意不进 `AggregatedImpacts` 切片**——该切片被 root_cause 排名与 chainThreadRefs 直接消费);`internal/tracequery/query.go` `aggregateWakeupCausalImpacts` trim 支新调 `foldWakeupCausalAggregateOverflow(out[8:])`(单遍,MAX 永不跨成员求和,top-8 caveat 字节不变)。
- tool:`internal/tool/trace_query.go` 新 `traceQueryWakeupCausalAggregateFoldRecord`(镜像 impact wire fold;恰一条 `wakeup_causal_aggregate` 记录,ClaimKey `wakeup_causal_aggregate:folded_overflow`,Value=成员 MAX,notes=**NKR 折叠族四键复用零新键族** + causality=on_wakeup_chain + chain_relevance=on_chain + selected_window F1 锚注)+ aggregates 循环后 nil-guard 发射点。
- 编译端:folded_* re-materialize(MergedCount/口径源/永不领衔/永不徽章/折叠行渲染)全部走 PTV5 既有管道;唯一复核修=F1 计数吸收(`traceCausalProjectionLimitNodesOnChainFold` 吞并带 typed `OnChainOverflowFold` 标记的成员时 N 累加其 MergedCount(G)而非 1,roster 并入其 subjects 仍受全局 cap,证据逐记录吸收不变;普通 ×N 聚合成员仍计 1)。
- 引擎 roster 去重(复核 F2):`foldWakeupCausalAggregateOverflow` 补 seen map(镜像 wire-cap fold)——同 PID 双态溢出计 2 组但占 1 roster 槽。
- emit pin fixture 扩:`trace_note_keys_emit_pin_test.go` fixture 挂 `AggregatedImpactsFold`(emit 面防 rot)。

**裁定① pins**(`internal/tracequery/tracequery_pts2_aggregate_fold_test.go` + `internal/tool/answer_document_projection_pts2_test.go`):

- `TestAggregateTopEightOverflowSynthesizesBoundedFoldMember`:12 组→8 保留+fold{Groups=4,range=成员 min–max 显示值,roster 全列,包络有效,caveat 字节不变}。
- `TestAggregateFoldRosterBoundedAtEightSubjects`:20 组→roster 恰 8(O(1) 内存兑现)。
- `TestAggregateAtOrUnderCapEmitsNoFoldMember`:5/8 组突变→field nil+零 caveat(≤8 零发射反噪音)。
- `TestPTS2AggregateFoldRecordEmitsTypedFoldLane`:恰一条 fold 记录,四键+双 on-chain 注+F1 锚注+Value=MAX。
- `TestPTS2AggregateFoldZeroEmissionWithoutEngineFold`:nil field→零记录。
- `TestPTS2EngineAggregateFoldRowReachesTreeWithCount`(端到端):records→compile→树 fence 含 `其余 3 项(链上折叠)(ovfa-500、`(引擎组数正确)+ fold 节点 OnChainOverflowFold + 永不领衔沿用。
- `TestAggregateFoldRosterDedupesSameThreadAcrossStates`(复核 F2):同 PID s_sleep+runnable 双态溢出→Groups=2、roster 恰 1 槽。
- `TestPTS2BucketFoldAbsorbsEngineFoldCountAndRoster`(复核 F1,types):引擎 fold(G=4)被桶 fold 吞并→桶行 N=2 普通成员+4=6,roster 并入 eng-a/eng-b(全局 cap 4),证据仍 3 条逐记录(+N 面不变)。
- `TestPTS2BucketFoldOrdinaryAggregateMemberStillCountsOne`(复核 F1 突变):普通 ×N 聚合成员(无 typed 标记)仍计 1。

**裁定②(next-step 动态 cap)落点:**

- `internal/tool/answer_document_mutation_runtime.go`:新 `runtimeTraceNextStepComparisonRecordFloor = 2`;comparison gate 布尔提升(`comparisonShape`,零重复 compile);对比行循环撤 cap break(对比行全集,护未来家族增行;现 ≤4 行无行为差);per-record 循环前算 `recordCap = len(out)+2`(仅 comparison 形态;放在所有前导 lane 之后=**强保底**,保底槽只有 per-record 行可消费),非 comparison 形态 recordCap=4 字节不变;中间 lane(unsampled/multi-window/flat-anchor)仍读 base cap 字节不变。
- 硬上界兑现:base(4)+floor(2)=6 行。

**裁定② pins**(`answer_document_mutation_runtime_cmp6_test.go`):

- `TestRuntimeTraceNextStepComparisonRowsCoexistWithRecordRows`(改写自被本裁定取代的旧挤出 pin):disjoint 双 trace+3 per-record 记录→恰 6 行=对比行全集(4)领衔+保底 2 槽 per-record(第 3 条不入=保底非无界),ID 连续。
- `TestRuntimeTraceNextStepNonComparisonCapByteIdentical`(突变):单工件 6 条 per-record→仍恰 4 行(动态项只在 comparison gate 内)。
- 既有 lockstep/领衔/RTC-2/EN/单工件 pins 全部原样通过(优先序与锁步键零改动)。
- 复核 F4/F5 免动;F5(动态 cap 涌现上界)由 coexist pin 的恰-6 断言充当先红机制看护(cap 公式任何再涨先打红该 pin),免独立 pin。

**测试结果**:`go test ./internal/tracequery/ ./internal/types/ ./internal/tool/ -count=1` 全绿;`go build ./...` OK;`go test ./... -count=1` 全仓全绿(2026-07-06)。复核批(F1 计数吸收/F2 roster 去重/F3 措辞)后三触及包重跑全绿。探针 `tmp_probe_pts2_aggfold` 按约用后删除,源码存档于 §7.4。

### §7.4 附录 — PTS-2 评估探针源码(已按约删除,此处为可复现存档)

复现:粘回仓根 `tmp_probe_pts2_aggfold/main.go`,`go run ./tmp_probe_pts2_aggfold` (从仓根)。

```go
// tmp_probe_pts2_aggfold — PTS-2 裁定① 风险评估探针 (2026-07-06, 临时,用后删除).
//
// 度量目标: 真实 trace 的 wakeup_chain 窗内 distinct (subject PID, dominant
// state) 聚合组数量级 —— 即引擎 aggregateWakeupCausalImpacts top-8 之外的溢出
// 组数,以及合成 fold 成员(count/min/max/subjects≤8)的增量内存与增量耗时。
// 复算逻辑与引擎同 key (fmt.Sprintf("%d/%s", PID, DominantState)) 同过滤
// (PID>0 ∧ ChainDepth>0 ∧ TotalMs>0 ∧ DominantState≠"") 同门槛 (OccurrenceCount≥2)。
package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type window struct {
	name       string
	start, end float64
}

// syntheticFoldCost bounds the fold-synthesis increment on a synthetic
// >8-group overflow (the real fixture never overflows): one linear pass over
// the ALREADY-materialized overflow aggregates building count + min/max +
// ≤8-label roster + line/ts envelope — the exact loop shape the engine fold
// will run. Measures ns/op and allocated bytes per synthesis.
func syntheticFoldCost() {
	for _, total := range []int{9, 50, 308} {
		aggs := make([]tracequery.WakeupCausalAggregate, total)
		for i := range aggs {
			aggs[i] = tracequery.WakeupCausalAggregate{
				Thread:           tracequery.ThreadRef{PID: 1000 + i, Comm: fmt.Sprintf("worker-thread-%03d", i)},
				DominantState:    "s_sleep",
				DominantImpactMs: float64(total-i) * 1.5,
				LineStart:        100 + i,
				LineEnd:          200 + i,
				FirstTs:          34579.45 + float64(i)*0.0001,
				LastTs:           34579.46 + float64(i)*0.0001,
				OccurrenceCount:  2,
			}
		}
		overflow := aggs[8:]
		const iters = 10000
		var m0, m1 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m0)
		t0 := time.Now()
		var sink int
		for it := 0; it < iters; it++ {
			groups := 0
			var minMs, maxMs float64
			var subjects []string
			lineStart, lineEnd := 0, 0
			var firstTs, lastTs float64
			for _, a := range overflow {
				groups++
				v := a.DominantImpactMs
				if minMs == 0 || (v > 0 && v < minMs) {
					minMs = v
				}
				if v > maxMs {
					maxMs = v
				}
				if len(subjects) < 8 {
					subjects = append(subjects, fmt.Sprintf("%s-%d", a.Thread.Comm, a.Thread.PID))
				}
				if a.LineStart > 0 && (lineStart <= 0 || a.LineStart < lineStart) {
					lineStart = a.LineStart
				}
				if a.LineEnd > lineEnd {
					lineEnd = a.LineEnd
				}
				if a.FirstTs > 0 && (firstTs == 0 || a.FirstTs < firstTs) {
					firstTs = a.FirstTs
				}
				if a.LastTs > lastTs {
					lastTs = a.LastTs
				}
			}
			sink += groups + len(subjects) + lineStart + lineEnd
		}
		dt := time.Since(t0)
		runtime.ReadMemStats(&m1)
		perOp := dt / iters
		allocPerOp := float64(m1.TotalAlloc-m0.TotalAlloc) / iters
		// Wire-record increment: the four folded_* notes + summary the tool
		// side would emit for this overflow.
		notes := fmt.Sprintf("causality=on_wakeup_chain; chain_relevance=on_chain; impact=%.3f; folded_rows=%d; folded_min_ms=%.3f; folded_max_ms=%.3f; folded_subjects=%s; selected_window=34579.472865-34579.475857",
			float64(total-8)*1.5, total-8, 1.5, float64(total-8)*1.5, strings.Join(func() []string {
				var s []string
				for i := 0; i < 8 && i < total-8; i++ {
					s = append(s, fmt.Sprintf("worker-thread-%03d-%d", 8+i, 1008+i))
				}
				return s
			}(), ","))
		summary := fmt.Sprintf("%d aggregated wakeup-causal pairs beyond the engine top-8 folded (max %.3fms); per-hop causal impact rows remain complete", total-8, float64(total-8)*1.5)
		fmt.Printf("synthetic groups=%3d overflow=%3d: fold synthesis %v/op, alloc %.0f B/op; wire record ≈ %d B notes + %d B summary (sink %d)\n",
			total, total-8, perOp, allocPerOp, len(notes), len(summary), sink)
	}
}

func main() {
	path := "eval/fixtures/real_traces/donghu_tieba_frame.systrace"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	t0 := time.Now()
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "BuildIndex:", err)
		os.Exit(1)
	}
	fmt.Printf("index built in %v\n", time.Since(t0))

	windows := []window{
		{"jank", 34579.472865, 34579.475857},
		{"wide10", 34579.472865, 34579.502785},
		{"post30", 34579.475857, 34579.505857},
		{"legacy115", 34579.472865, 34579.587805},
		{"full", 34579.450627, 34579.595184},
	}
	pids := []int{59566, 59891}

	syntheticFoldCost()

	for _, pid := range pids {
		for _, w := range windows {
			q := tracequery.Query{
				View:         "wakeup_chain",
				PID:          pid,
				TimeStart:    w.start,
				TimeEnd:      w.end,
				TimeStartSet: true,
				TimeEndSet:   true,
			}
			var m0, m1 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m0)
			tq0 := time.Now()
			res := tracequery.Run(idx, q)
			dt := time.Since(tq0)
			runtime.ReadMemStats(&m1)
			chain := res.WakeupChain
			if chain == nil {
				fmt.Printf("pid=%d win=%s: no chain result\n", pid, w.name)
				continue
			}
			// Replicate the engine grouping (same key/filter/threshold).
			type g struct {
				n              int
				minMs, maxMs   float64
				sumMs          float64
				subject, state string
			}
			groups := map[string]*g{}
			for _, impact := range chain.CausalImpacts {
				if impact.Thread.PID <= 0 || impact.ChainDepth <= 0 || impact.TotalMs <= 0 || strings.TrimSpace(impact.DominantState) == "" {
					continue
				}
				key := fmt.Sprintf("%d/%s", impact.Thread.PID, impact.DominantState)
				a := groups[key]
				if a == nil {
					a = &g{subject: fmt.Sprintf("%s-%d", impact.Thread.Comm, impact.Thread.PID), state: impact.DominantState}
					groups[key] = a
				}
				a.n++
				v := impact.DominantImpactMs
				if a.minMs == 0 || (v > 0 && v < a.minMs) {
					a.minMs = v
				}
				if v > a.maxMs {
					a.maxMs = v
				}
				a.sumMs += v
			}
			var eligible []*g
			for _, a := range groups {
				if a.n >= 2 {
					eligible = append(eligible, a)
				}
			}
			sort.Slice(eligible, func(i, j int) bool { return eligible[i].maxMs > eligible[j].maxMs })
			overflow := len(eligible) - 8
			if overflow < 0 {
				overflow = 0
			}
			var caveat string
			for _, c := range chain.Caveats {
				if strings.Contains(c, "aggregated_impacts kept top 8") {
					caveat = c
				}
			}
			fmt.Printf("pid=%d win=%-9s dur=%8v allocΔ=%8.2fMiB impacts=%4d edges=%4d distinct_groups=%3d eligible(≥2occ)=%3d emitted_aggregates=%d overflow(rank>8)=%d caveat=%q\n",
				pid, w.name, dt.Round(time.Microsecond), float64(m1.TotalAlloc-m0.TotalAlloc)/(1<<20),
				len(chain.CausalImpacts), len(chain.Edges), len(groups), len(eligible), len(chain.AggregatedImpacts), overflow, caveat)
			if overflow > 0 {
				// Size the synthetic fold member: subjects roster (≤8), min/max.
				subs := 0
				bytes := 0
				for i, a := range eligible[8:] {
					if i < 8 {
						subs++
						bytes += len(a.subject)
					}
				}
				fmt.Printf("    fold member would carry: groups=%d min=%.3fms max=%.3fms roster=%d subject label bytes=%d\n",
					overflow, eligible[len(eligible)-1].maxMs, eligible[8].maxMs, subs, bytes)
			}
		}
	}
}
```

## §8 PTV5 复跑三 FAIL 归因(2026-07-06 01:1x–01:44 批;只归因不修,依据=行为链+verbatim 证据)

**总判:三案均非 PTV5 回归**——关键工具面输出与 07-05 binary 字节相同(b3 `process_cpu_load ... threads=4` 行、c4 `freq_residency=...,+26` 折叠串两日 log 完全一致),涉事 gate(`explorer_trace_query_sufficient_runtime_evidence`)2026-06-28 088d1dcf 就在。07-05 的两个 PASS 本身都是意外通道(见各案),PTV5 复跑只是把行为方差重新掷了一次。

| 案 | 判据 | 定性 | 一句话根因 |
|---|---|---|---|
| b3(20260706-011737) | missing:NetworkService | 行为方差(view 选择)+ 工具面结构 gap 暴露 | 进程域 rollup 缺位:window_stats 的 pid 参数不进 rollup 域,`threads=4` 是全局 top-8 幸存者数伪装进程普查;07-05 PASS 靠 wakeup_chain 搭车 |
| c4(20260706-012258) | no_regex_match:807000 | 行为方差(无视 result_truncated 断言穷尽)+ 证据面双折叠 | event_search 40 行 chronological 截断+freq_residency 显示折 4 档,807000 两面皆不可见;07-05 PASS 靠 analyzer 漏判 exclude+thread 过滤零命中双重意外开出 shell 车道 |
| short_runnable(20260706-013200) | banned:still_present(wall=728s) | 行为方差(analyzer 单点分类失效),下游确定性放大 | analyzer 整个漏发 `external_observation_policy`(exclude 未 typed)+误发 `current_risk=true`→完成门锁死 current_source lane(×19)拖出 728s,finalizer decision 义务强制 enum,渲染器裸吐 `still_present` |

### §8.1 b3 行为链

- FAIL run 全 log **0 次** NetworkService(07-05 PASS log 42 次/答案 16 次)——token 从未到达模型。工件真值:tgid 59566 共 39 线程,`NetworkService-60595 (59566)` 127 行、42 个 running burst,PROFILE §1.4 #3 busy=13.135ms。
- 工具面链:window_stats(pid=59566) 事件准入无 pid 谓词(query.go:1410 仅行窗+时窗)→running 按 `(pid,comm,cpu)` 分桶(query.go:1519,跨核碎跑被稀释)→全局 top-8 截断(query.go:1583)→`thread_cpu_load`=全局 TopRunning∪RunnableTop cap12(query.go:1608/3604)→`process_cpu_load` 只 rollup 幸存者且渲染 `threads=%d`(query.go:3647-3650)。verbatim:`process_cpu_load process=com.baidu.tieba-59566 threads=4 running=46.411ms ... top_thread=CookieMonsterCl-59843`(两日字节相同→PTV5 per-窗快照 floor/fold 未涉此面)。模型照单全收:"进程共识别出 5 个线程"。
- 模型自救被斩:grep `\(59566\)` 与 exec_command `grep|sort -u`(可枚举全部 39 线程)先后被 `explorer_trace_query_sufficient_runtime_evidence` 硬拒(agent.go:6133-6167;本 run analyzer 正判 exclude→`explorerTraceQuerySourceFallbackHardBlocked`=true),回退 preview 头尾可见名单(NetworkService 全在被省略的 1.92MB 中段)。
- 07-05 PASS 复盘=运气:该 run 多调 `wakeup_chain(pid=59566)`,NetworkService 以链上中间 waker 身份搭车入答("经 NetworkService-60595/ThreadPoolForeg-60555 唤醒"),process 面同样只给 4 线程。oracle 不算过紧(#3 busy 理应出现在进程卷积答案)。
- 修向 **as-built(WSR 批 #70,2026-07-06 交付+核验修正轮)**:已增设 `process_domain_census` 车道 — pid/thread 场景下 window_stats 从 CMP-8 pre-truncation 全量 running 桶聚真进程普查:threads=时窗内真普查(成员观测与 running 侧同口径行窗+时窗谓词,核验 F1;纯 catalog 行窗口径会把窗外线程计入),top 线程跨核合并 running 降序 top-8 roster+PTS 折叠计数(其余 K 线程合计 X cpu·ms),同线程跨核可加和依据(时间线互斥)与跨线程 cpu·ms(CMP-3)注记三面同款;全局 TopRunning/ThreadCPULoad/ProcessCPULoad 面字节不变(typed DeepEqual+渲染行字节双 pin)。核验 F2 双修:sanitizeForBanner 200B 截断改 rune 安全(CJK 不再切出非法 UTF-8,通用机制修),census caveat 拆多条各<200B(zh/en 独立,关键从句渲染面存活 pin)。落点 internal/tracequery/cpu_occupancy.go::computeProcessDomainCensus + internal/tool/trace_query.go;pin=internal/tracequery/process_domain_census_test.go / internal/tool/trace_query_wsr_b3_census_test.go / builtin_test.go(CJK 边界)。b3 案复跑留主会话。
- typed-signal 卫生候选注记(核验 O2,本批不修):`topThreadDurations`(query.go)平局无确定性 tiebreak — RunnableTop/SleepTop/cpu_occupancy PerCPUTop 等 CPUPressure 系面等值行序随 map 迭代不稳定;候选=补 LineStart tiebreak,并入 typed-signal 卫生批。

### §8.2 c4 行为链

- FAIL run 全 log **0 次** 807000(07-05 log 19 次/答案 5 次)。真值:90 行 cpu_frequency,11 档 807000..2189000;807000 六行全在 ts 34579.5535/.5695(行 11623-12789)。
- 证据面双折叠:①event_search(event_types=cpu_frequency,无窗无 limit)→`sharedDefaultResultLimit=40` 截断,`matched_events=40`,包络止于 34579.5243——807000/965000 全部在截断之外;工具已给 caveat `event_search_limit_reached=true; returned rows are the first 40 chronological matches only, not an exhaustive result set` 且 typed refine hint 连正确救命调用都算好了(`preferred_params=...limit=90`),explorer 未消费,finalizer 只当 advisory。②window_stats `freq_residency=1090000kHz/1.396ms,2189000kHz/0.170ms,1224000kHz/0.510ms,1618000kHz/1.221ms,+26`(trace_query.go:4068 `i>=4` 显示折叠,时序前 4 档)——807000 residency 藏在 `+26`。模型以 40 行枚举 9 档,confidence=high 宣称"频率范围 1.09GHz~2.189GHz"。两折叠面 07-05 字节相同→非 PTV5 回归。
- 07-05 PASS 复盘=双重意外:①analyzer 漏判 exclude(`current_source_mode:"default"`,同句"只分析这份 trace，不分析代码"!)→shell 车道未锁;②event_search 继承 runtime_target `thread=com.baidu.tieba-59566` 过滤→`matched_events=0`(cpu_frequency 发射线程是 tppmgr-sched-in-5850)→trace_query 零 hard 观测→gate 双条件皆空→`grep|awk|sort -n|uniq` 打出全部 11 档。即 07-05 是"两个错误相乘=对",07-06 是"系统面全对(exclude 正判+trace-first 生效)但证据面折叠无人追补"。
- 修向:与再审计队列 typed-anchor-obligation 同族——answer-critical 维度(此处 boundary/range)遇 `result_truncated` 时 refine hint 应升义务消费一轮;或 window_stats 增 per-cpu 频点 census 聚合行(distinct states min..max,O(1) 行数)。

### §8.3 short_runnable 行为链

- ①发射面:finalizer emit_answer_document `decision` 块 `current_status_verdict:"still_present"`(模型手填,块 text 首字是"是。"不含 token)→渲染器 `renderV2BlockDecision`(render/answerdoc.go:342-377)把 raw enum 前缀进 prose:`**结论：** \`still_present\` — 是。...`。同段模型自陈"无法判断该优先级反转问题在最新代码中是否已修复"——被 finalizer prompt "**decision** (exactly 1) ... exactly one of `still_present`, `fixed`, or `not_enough_evidence`" 强制选边,自相矛盾(且 not_enough_evidence 同为 oracle banned;正确形态=该 run 根本不该有 decision 义务)。全 eval 库无任何正向期待 still_present 的案(occurrences 全是 EXPECT_NOT_CONTAINS)。
- ②exclude 判定这 run **未生效**:analyzer 单轮 emit,`external_observation_policy` 整字段缺失+`diagnostic_profile.current_risk=true, is_diagnostic=true`(TOOLRESULT 回显 `diagnostic_profile=true current_status_check=true`),直接违反 analyzer 教学(explicitly forbids→current_risk=false)。CSR(92a6b6a0)的 exclude-run 机器因无 typed exclude 可 key 而全程未触发,CSP/CPD 无涉——非回归,纯分类方差。对照 07-03 PASS(156s):同题 analyzer 正判 `current_source_mode:"exclude"`(source_quotes 全)+`current_risk:false`,0 次完成门阻塞、0 still_present;工件 ../../customlogs/xxx_all.systrace 在位,非工件问题。
- ③728s 与 still_present **同链同根**:current_risk=true→完成门要 current_source origin lane→`accepted investigation closure cannot auto-complete mixed-origin explore window; missing_origin_lanes=current_source` ×19,9 个 explore dispatch 窗(01:32:27→01:43:24),retry directive 反复推 repo_map lenses(explorer think 原话:"There's a mismatch here - the user wants trace analysis only"),multi-topic scaling 13→28 放大窗数;全程 LLM `attempt=1/6` 零重试、零流式退化、repair 指标全 0——**不是** 07-03 死等案那种 LLM 退化,是 lane 饥饿重放循环;最终 forced-finalize 放行(`grounding floor failed on forced-finalize path ... Missing repo_map lenses: task_map, file_map, relation_map`)。
- 定性:行为方差(analyzer 单点漏 typed exclude+误 current_risk),下游门全按 typed 输入确定性行事;两个结构放大器让方差变用户可见:(a) current-status verdict 义务只 key analyzer 的 `current_risk`,不校验"current_source lane 是否真产出过证据"——该精确信号就躺在完成门日志里(missing_origin_lanes);(b) `renderV2BlockDecision` 裸吐内部 enum 进中文 prose(同 3a6673ef de-jargon 方向未覆盖的面)。
- 修向(下批候选,均为精确信号硬门/软引导合规形态):finalize 侧 decision 义务对齐 origin-lane 台账(current_source lane 零证据→义务降级为 caveat,enum 不发射);渲染层 enum→措辞映射(仍存在/已修复/证据不足)。当场不修:非 CSR/PTV5 小回归,decision 车道语义横跨 diagnostic 家族须裁定,且账本纪律=本批只归因。
- **观察注记(2026-07-06,SPR 批顺带落档,断路器本体不在本批)**:完成门 lane 饥饿重放无断路器——`missing_origin_lanes=current_source` 同一 blocker 连续 ×19 重放、9 个 dispatch 窗、11 分钟,期间无任何"同 blocker 连续 N 次→升级/放行/改道"的确定性断路;最终靠 forced-finalize 兜底放行。同族先例=denial 断路器(2026-07-02 批,37 连拒案);若未来再现同形态,候选形态是对"连续同 blocker 重放计数"(精确信号)加断路,不是放宽 lane 义务本身。
- **§8.3 双修 as-built(SPR #72,2026-07-06,形态 (c) 渲染/门层降级)**:①decision 义务对齐 origin-lane 台账——`ComputeCurrentStatusVerdictDowngrade`(types/current_status_verdict_downgrade.go,单一 home)在 persist 链尾(`persistMergedAnswerDocument`)按"完成门同款 lane 谓词(`ObservationRecordHasCurrentSourceLineSpan`)零命中 ∧ 选边 verdict(still_present/fixed)或 Required 缺发"打 typed 降级章(`AnswerDocumentV2.CurrentStatusVerdictDowngrade`,系统内部字段非 LLM schema);渲染层 caveat 形态"未评估：本轮无源码证据(原始判定 `<enum>` 仅留档,未消费)",块上 verdict 原文不动=审计位;完成门不消费:`validateCurrentStatusVerdict` 见章即义务不可评估(不再索求也不再记 satisfied),pre-emit `preCheckCurrentStatusVerdict` 零证据时豁免索求(不逼选边);not_enough_evidence 与零证据自洽,不降级;有证据 run 全链字节不变。②渲染 enum→中文措辞映射——`decisionVerdictWordingZH`(render/decision_verdict_wording.go,单一 home)覆盖 current_status+error_granularity 全家族(仍然存在/已修复/证据不足/逐条拒绝/整批失败/部分成功/遇错即停/汇总报错),en 保 token 字节不变;pin=全家族往返+去 token+双向 map 无孤儿(types/current_status_verdict_downgrade_test.go、render/decision_verdict_wording_test.go、tool/answer_document_current_status_downgrade_test.go、orchestrator/contract_check_block_current_status_test.go;降级门/映射摘除即红)。模型 emit 面(prompt "exactly 1"、schema enum、analyzer 分类)全部未动——①的 analyzer 单点分类失效仍是上游敞口,本批只封两个确定性放大器。

## §9 两标本 13 项归因与 PTV6 系列 as-built(2026-07-06)

用户两标本(20260706-014044/014407 HTML)驱动的 13 项归因(全文见 session 归因 agent 输出,要点入库):
1-2/9 **critical_blocking 双投**(分类只看 predicate 不读 chain_relevance→同记录进 SupportingHops+background 双席;树侧硬编码唤醒边=偏离 v3 §5 规格;候选行稀释 ❶ 主因)→ PTV6-A 根修:hop 准入 on-chain 门+专用边词"链上·深度未解析"+▒ consumed 防御+hop cap 折叠计数;复核 P1=未声明 relevance 行零席位→缺省背景席(席位矩阵 pin 三形态)。
3 无主导态 vs D状态标签矛盾(两 lane 各自正确无人合流)→ C 批 TypeToken→状态族显示映射(io_latency=测量族刻意不入,边界 pin)。
4 **138% 幻影**(三条重叠 io_latency 近值求和 4.119ms>窗 2.992ms;V4 精确相等差 0.03ms 逃逸)→ B 批近似档(≤3%+同 subject/object/token+重叠+双侧真实身份;S3 反例证明包络重叠是嘈声信号不可作硬门)→ 1.383ms/46%。
5 ◇ 行挂链上口径词 → C 批裁定A:有效归因限链宇宙,stanza 用"累计(跨线程)/折算"。
6/13 近义六 lane 冗余 → C 批"全词一处+数据一处"+typed 身份去重(复核抓 substring 包含判据=噪音硬门,改 typed)。
7 已解析 peer 裸占 cause 词位 → C 批 ResolvedPeerText 关系形态。
8 自身行裸 s_sleep → C 批中文化。
10 UX 纵向密度(逐 tag 一行 by construction/类别样板词 31 次/60 行)→ D 批流式打包+类别词降维(无主导态→◦ 图例承载、候选影响→typed generic 臂抑制+图例 entry、候选根因 2 次免降维)+≈窗内并发语义词;行数账 46→27/23→15,节点普查不变。
11 **❶❷ 徽章复算**(父行与其成因分解行同值双席)→ A 批每主体一席。
12 关键词截断倒挂(优先级反转候选被截而样板词占行)→ C 批全词保障(截断→从属/主行首 tag 全词)。
批次:A/B=12f9cb7c、C=8f6e7a5c、D=本 commit;复核轮 12+12+6 findings 全 confirmed 全收。用户裁定沿革:A(常显限链宇宙)/B(删反转影响)/C(指路句族→trace 源坐标)、"打包非折叠"、PTS on-chain 完整性(必提及必进树,多则折叠+计数)。
