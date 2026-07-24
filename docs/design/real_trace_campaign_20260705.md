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

**双账本权威声明(2026-07-11,§29.26 审计总收账批)**:本账本=trace 战役**权威账**(裁定原文、批次收账、验收句的唯一权威);`trace_analysis_open_gap_ledger_20260710.md`=trace correctness 子域的**施工账**(逐项状态/commit SHA/验证命令的唯一状态权威)。同一项的开放/结案状态以施工账为准,裁定原文与验收句以本账为准;两账头部互指(§29.24『剩余 gap』与施工账的三处状态矛盾已按此口径加勘注收敛)。**行号免责**:各节引用的 `.go:line` 均为该批收账时的工作树坐标,后续大改会漂移(实例:§29.6 P2-1 的 query.go:8424 现已指向无关代码)——引用旧节裸行号排批前先以符号名重定位;行号以该节收账 commit 为准,不逐节回改。

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
| 2 | real_trace_a3_whole_trace_overview | A3 | §1.1 span 144.557ms/端点;§1.7 full running 52.478ms(闭合对 51.462+窗尾 flush 1.016,§29.145 勘正;PROFREBASE-1 re-base,带 52±1——原稿 50.524/带 50-51 系 probe 时代 identity-bug 工件);§1.4 top:sysevent_store/hilogd。CONTAINS 三 token + 两条数值窄带 |
| 3 | real_trace_a4_out_of_range_window | A4 | §1.1 last_ts=34579.595184;请求窗全越界。断言:诚实词带 + 真实边界回显(34579.59/144.x ms)。**刻意不断言 tool 参数形状**(模型可从全窗查询诚实作答;精确信号=答案面诚实,工具探索形状=嘈声,只作软记录) |
| 4 | real_trace_a5_excerpt_degenerate_window | A5+F2 | §2.1 覆盖 0.556ms(2942.244845..2942.245401);§2.3 ColdPool#6 busy 0.367ms。断言:2942.24 回显 + 覆盖披露带 + 实际端点(TEXT) |
| 5 | real_trace_b2_tid_only_waker | B2+D1 | §1.4 59843=CookieMonsterCl tgid 59566;§1.6 唤醒 main 34 次(48 中最多)。断言:身份共现 + 唤醒关系 TEXT + 次数带(34/3x 次/最多) |
| 6 | real_trace_b3_process_level_rollup | B3 | §1.4 进程内 top:main 51.46 > Zeus 30.29 > NetworkService 13.14 > Cookie 8.49。断言:三 token + 主线程居首 TEXT + Zeus 量级带 |
| 7 | real_trace_b4_missing_thread_miss | B4 | §1.4 `grep -c '99999'`=0。断言:99999 回显 + 诚实 miss 词带。无法用正则禁"编造统计",依赖 miss 词带 + 复核时人工看 run-1.out(gap 表列预留) |
| 8 | real_trace_b5_multi_subject_render | B5 | §1.7:legacy115 窗 RenderThread 100% s_sleep(114.94ms),main running 26.95ms(PROFREBASE-1 re-base;原稿 24.99 系 probe 时代工件);RenderThread 只在 trace 尾被 main 唤醒(34579.590882/593245)。断言:SECTIONS 双主体 + RenderThread 全睡带 + main 活跃带 |
| 9 | real_trace_c2_dstate_iowait | C2 | §1.8:3 条 iowait=1,caller `sync_buffer_read_wi+0x60/0x11c`,ts 34579.4518/4531/4717;总量 io_wait 0.635ms=0.138+0.147+0.350(PROFREBASE-1 re-base;原稿 d_sleep 0.488+io_wait 0.147 拆分系 probe 时代 typing)。断言:verbatim caller token(CONTAINS)+ 状态词带 + `34579.4(5|7)` 时间带 + 小量级诚实带 |
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
(PROFREBASE-1 2026-07-20 适配注:当前引擎 `Event.Frequency` 已为 `int64`,
下述存档源码需在 `freqSets[cpu][ev.Frequency]++` 处加 `int(...)` 转换方可
编译;§29.150⑦ 重导出即以此单处机械适配运行,其余逐字节原样。)

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
- **§8.2 双方案评估(RFC #71,2026-07-06,先落评估再实施)**:

  | 维度 | (A) refine 消费义务(A1 lane 范式) | (B) cpu_frequency census 聚合行(确定性) |
  |---|---|---|
  | 根治面 | 通用:一切 `result_truncated` 维度(event_search/grep/read_file/repomap);但闭环仍依赖模型消费软义务——c4 正是"无视 advisory"的行为方差,软提醒只能压低不能归零 | 只治频点枚举/range/boundary 类;确定性:全梯直接进模型正在读的枚举面+observation 面,零消费成本 |
  | 义务判定信号(精确性) | 可行:registration=`Refinement.ResultTruncated`(typed bool,context.go:5715 注明 gate-eligible 精确旗);answer-critical=截断调用发布的 hard runtime observation 进 ledger(`SourceRef`/ToolCallID 关联;局部代理=该结果自带 hard observations 非空);消费证明=同 tool+view+filter 指纹的后续非截断调用;waive=emit_investigation_complete 新 typed 参数 | 无义务判定问题(不依赖模型行为) |
  | 实施复杂度 | **中,非小**:`dispatchToolResults` 每 dispatch 重置(context.go:241),跨 dispatch 义务需新持久 typed 状态(先例=`traceQueryRuntimeObservationCount` 存活计数);+指纹消费追踪+checkpoint 渲染+完成门 note+waive 参数(LLM-facing schema→R2' 6 处同步+prompt 红线 checklist);≈A1 lane 本体同量级 | **小**:StreamEventSearch 单 pass 本就数 `matchedTotal` 越过 limit(stream_search.go:130),census 同 pass O(distinct) 累加;索引孪生一次内存全扫;render 两行+residency 折叠后缀;WSR #70 process_domain_census 同构先例 |
  | 机制复用度 | compile/unconsumed/双软消费面与 A1 直接同构;但 anchor 的消费证明(路径∈读集)先天精确,truncation 的"消费"需指纹匹配设计裁定 | 与 WSR census 车道同构:pre-truncation 全量聚合、纯增量、全局面字节不变 |

  **选定:B 本批全量交付**(c4 类在两个折叠面上确定性根治);A 判定=信号面可行但体量非小(跨 dispatch 持久义务状态+waive 触 LLM-facing schema/R2'),按任务授权"选一个给依据"留给 typed-anchor-obligation 家族专项批,上表即该批的信号设计输入。
- **§8.2 B 修 as-built(RFC #71,2026-07-06,census 车道)**:两个折叠面各一手确定性修,均纯增量。①event_search 频点 census——`CPUFrequencyCensus`(internal/tracequery/cpu_frequency_census.go,单一 home):query 显式含 `event_types=[cpu_frequency]` 且时序显示截断真实隐藏了 matched 频点行时,发布 pre-truncation 全量档位聚合(distinct kHz 升序全集+每档行数+cpu_id 集+min..max 边界+lines 包络;`cpu_frequency_limits` 兼容匹配行单独计数不入梯);流式面(StreamEventSearch,event_search 主通道)在既有 matchedTotal 越限扫描的同一 pass O(distinct) 累加,索引孪生(Run default 分支)一次内存全扫同谓词,DeepEqual 孪生 pin 防漂移;banner 双行 `cpu_frequency_census(频点普查)`+`cpu_frequency_census_tiers khz×rows=`(共享 formatter `FormatCPUFrequencyCensusTiers`,>24 档防御折叠保两端点),census fact(`predicate=frequency_tier_census`)前置进 EvidencePack→evidence_fact hard observation 确定性上答案数值面;非截断/未点名频点家族结果全链字节不变。②window_stats freq_residency 折叠后缀——`+N`(时序段折叠)后追加 ` distinct_khz=N range=min..maxkHz`(全段列表聚合,≤4 段行字节不变 pin)。pin=internal/tracequery/cpu_frequency_census_test.go(90 行/11 档/807000×6 全在截断后合成形态、非截断 nil、孪生 parity、行窗谓词、折叠端点)+internal/tool/trace_query_rfc_c4_census_test.go(banner 12 断言、无 census 零字节、fold 披露、≤4 段字节 pin);3 路突变(census 累加摘除/后缀词汇漂移/端点丢弃折叠)全红后复原。A 面评估表见上,未实施。c4 案复跑留主会话。

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

## §10 客户回访反馈#1 逐项代码级归因(2026-07-06;只读归因批,未动代码)

标本=/Users/han/Downloads/cust_trace_q1.txt(595 行 CLI 全程;berlin.systrace 1104MiB;42591 滑动卡顿 3.3s 窗;MiniMax-M2.7;锚窗 6793224.900–6793225.050=151ms)。本节为主会话逐行初审的代码级归因确认。行号口径:归因期间他人 CFR 批在同一工作树并行改动(涉 internal/tracequery/query.go、supply_fold.go、types.go、cpu_occupancy.go、internal/tool/trace_query.go、internal/types/trace_note_keys.go 等),本节行号为归因时刻工作树快照,CFR 落地后可能漂移——复核时以符号名(函数/常量)定位为准;render/、agent/、tool/answer_document_* 等未被 CFR 触碰的文件与 main@7f9369af 一致。

### §10.1 逐项归因表

**A1 [P0,用户点名] binder peer hop 缺失** — 定性:引擎车道缺失+投影无跨进程边词。
- 产生链:binder_wait 行 `query.go:11760` 区(源=chain.BinderWaits←findBinderWaitsForChain ~:8246,按 TransactionID 配对 send/recv,不进对端);chain_relevance/edge_count 富化 enrichCriticalBlockingWithChainContext `query.go:11944` 区;peer= 富注 `internal/tool/trace_query.go:5613-5628`;投影仅消费 TraceNoteKeyPeer→BlockingPeer 文本(`internal/types/trace_causal_projection.go:1284-1285`)。
- 考证:①T3 transact join 形态:parse 层已抽 EventBinderTransaction/EventBinderReceived/EventBinderReply(`parse.go:2209-2222`),配对 `ipc.go:18-105` 产 IPCEdge{Send/ReceiveTs,Sender,Receiver,Interface,Flags,Oneway,SyncLike,LatencyMs}——**reply 事件已解析但配对止于 receive,服务端处理段(receive→reply)今日不构造**;补 reply join 即可得服务端处理段,增量非重构。②跨界续链现无任何车道:wakeup_chain 只锚单 target;via_thread(RN-14a,`query.go:7455-7524`,schema `tool/trace_query.go:129`)只判"在不在 target 唤醒路径上",不能以 peer 为子目标;root_cause_rank 不递归 peer。③"对端状态分解"半件已存在:buildCriticalBlockingPeerState(`query.go:11914`)对每个 peer 已解析行无条件建 ThreadStateBreakdown(见 A3)。④投影边词典仅 下钻/唤醒/链上·深度未解析,无跨进程 hop 表达。
- 修向三案:(1)引擎续链——on_chain binder_wait 且 peer 解析行,以 peer 为子目标在重叠窗跑 depth-capped 有界续链,发布 typed 子链观测(新 note 族 peer_chain_*:NKR 登记+R2' 六处同步+causal_token_registry 新行裁定),投影新边词 `─binder对端─` 下挂对端子树;(2)建议式下钻指令合成(软引导先行)——存在该形态时仿 RN-14a 既有合成先例(`tool/trace_query.go:262-281`、`emit_investigation_complete.go:3741-3780`)合成点名对端的具体 next/refine 指令,零引擎改动当轮多一跳,与 D1② 同件;(3)双做(推荐):2 本批止血,1 作引擎批交付,A2 reply join+A3 PeerState 为其数据底座。

**A2 [P0] blocking_span 对端未解析 26.287ms on-chain(E20)** — 定性:车道缺失(等待对象解析仅覆盖 lock-contention 结构化 payload)。
- 产生链:`query.go:11790-11816`,blocking_span 来自 stats.TraceSpans+isBlockingLikeText(span.Name);peer 仅当 parseLockContentionPayload(span.Name) 命中(§7.30.3 D1)才填,否则零值→显示"对端未解析";TraceSpanSummary(~:1655)本身无等待对象字段。
- 标本 E20=target 自身 ×3(8.485–9.169ms) span(:1013562-1016996 等):**span.Name 在产出点在手(Summary 里已打 %q)但既不用于对端推断、也不作为 wait_object 注发布**——投影 E20 行连 span 名都给不出。
- 修向:(a)最低成本:发布 wait_object=span.Name 注(NKR 登记),E20 行至少能说"等待对象=<span名>";(b)白名单形态启发解析:binder transact 包装 span→用 ipc join 同窗 IPCEdge 反查 peer;VSync 类 span→影响点=信号源;命中才填,不设嘈声硬门;(c)b 命中 binder 者汇入 A1 车道。

**A3 [P1] 对端睡眠 31.6~64.9ms 只在散文** — 定性:口径 gap(typed 车道已在,消费/审计面双缺)。
- 真相修正:对端睡眠**已是 typed**。散文来源=critical_blocking 视图文本 `tool/trace_query.go:3191-3197`(`peer_state ... sleep=%.3fms`);typed 富注 peer_state_sleep 等 8 键自 fa00f0d1(2026-06-14)即发布(`tool/trace_query.go:5631-5641`)。丢失点两处:①注册表 `types/trace_note_keys.go:437-444` 全族 carrier=display_only,投影编译零消费(只吃 peer=)→E14 行/明细块不显示对端睡眠;②系统补充 4-note cap(`agent/answer_document_evaluator.go:12563-12592`,AllowedNotePrefixes 表+`len(notes)>=4 break`)——binder_wait 前 4 键恰為 type/peer/chain_relevance/edge_count,peer_state_* 永远挤不进审计面(标本 566-569 行即此形态)。
- 修向:投影消费 peer_state_dominant/sleep(E14 类行加"对端睡眠Xms·主导态"注;carrier 升 soft_consumer+消费点+pin);系统补充按 type 定 note 优先序,binder_wait 行把 peer_state 两键排进前 4;A1 修向 1 落地则自然覆盖。

**B1 [P0] 锚选择(锚=VSyncGenerator 非用户线程)** — 定性:口径 gap——锚由"发布顺序先到先得"决定(嘈声信号),用户实体精确信号在手却只用于免责句。
- 产生链:锚=第一条 predicate==wakeup_chain 观测的 path 末元素:`types/trace_causal_projection.go:505-506`(`len(wakeupPath)==0` 先到先得)→`tool/answer_document_mutation_runtime_tree.go:632-634`(model.Target=path[len-1]);用户实体只进 disclaimer:tree.go:1905-1938(RootFocusAnchorOnly←runtimeTraceProjTargetMatchesUserEntities);锚窗=该记录 selected_window note(anchor_window carrier,`trace_note_keys.go:327`)。
- 本例机理:模型探索期对 VSyncGenerator 自身跑 wakeup_chain(正当下钻),其 path 记录先发布→锚=VSync;42591 的 binder/blocking 证据(E14 rank=1 conf=0.92)来自 critical_blocking/root_cause 记录、chain_depth 未解析→只能挂 depthless,结构上永远成不了根。42591 无以其为根的树,不是引擎没证据,是投影锚规则不看它。
- 裁定点+方案:depthless-heavy(Depth1Cumulative≈0)且用户线程持 rank 头名时锚归谁?(i)锚选择加精确前置:存在 target 匹配用户实体的 wakeup_chain path 记录时优先取之,否则维持先到先得(无匹配时零行为漂移);(ii)双根分区:锚树保留+用户关注线程另立分区(复用 CMP-A 多工件分区机制);(iii)维持现状仅强化 disclaimer/lead 连接句。建议 (i) 为主、depthless-heavy 时评估 (ii);须用户裁定。

**B2 [P0] 覆盖行 0.051/0%** — 定性:口径 gap——depthless-heavy 形态下"已归因"只认树位深度解析行,口径失真。
- 产生链:tree.go:4469 runtimeTraceProjDepth1Cumulative→:4880-4901 只收 Kind==Chain && Depth==depth && HasData(depth-1,H10 浅层回退);depthless 行(E14 4.577/E16 2.770/E17 3.227…有效归因非零)与锚自身 periodic 0.208 全不入分子——本例分子只剩 E13(L1 running 0.051)。分母:无 target symptom 行→整窗 151ms 分支(:4496-4503);hop-only 附句 :4513-4520 用 hopSleep=47.814(E1)。
- 修向两案:(a)分子扩展:depthless on-chain 行有效归因以"同主体 MAX、跨主体不加和"纪律并入(与 v3 墙钟禁Σ对齐,需裁定);(b)保守披露:覆盖行加"另有 N 项链上深度未解析行(单项最大 X ms,墙钟不可加和)未计入已归因"——纯增句零口径风险。建议先 (b) 上线、(a) 提裁定。

**B3 [P1] 结论行(42591 binder)与树根(VSync)分叉** — 定性:B1 的显示症状;两选择器各自正确。
- 产生链:lead 建模时 pin(tree.go:1070-1073)→runtimeTraceProjLeadSelect :4313-4323(primary 优先,空则 on-chain fallback :4335-4357:Chain|Depthless 行按 selection value 最大,fold 除外,:4379-4399)——depthless 可当 lead;root 见 B1;两者无互认。
- 修向:随 B1(i) 自然合流;短期 lead 行加连接句"(树根为分析锚点 X;主根因位于链上·深度未解析层)"。

**B4 [P1] 转运循环噪音+微值深链行(L31-33)胜出 vs E20 26ms 屈居 depthless** — 定性:显示卫生——树位形状由"谁被解析出深度"决定,与量级无关。
- 产生链:tree.go:738-751 depthAttach 按 ChainDepth 收编(L31-33 来自带 depth 注的 wakeup_causal_aggregate 记录);:880-913 未解析行入 depthless lane;TreeRows :928-940 插入序 DFS,**无 per-depth 量级 top-N/排序**;↺ 中转循环为引擎 path 结构如实展开。hop 榜没有"按量级排序后选 top-10"这回事——准入即深度解析存在性。
- 修向:(a)微值深链尾折叠:深度>K 且有效归因<ε 的尾臂并入既有"…省略N节点"段计数;(b)depthless lane 按有效归因降序。投影卫生批。

**B5 [P1] 同线程双行(E5/E6、E15/E16、E17/E18)** — 定性:行为域正确(R1 语义禁改,值语义不同:0.058=causal_impacts 投影 vs 4.115=running 全量)。
- 产生链:R1 键=subject+impact(3位)+line span(`types/trace_causal_projection_aggregate.go:114-133`);不同 token/不同值→不并,by design。
- 修向:展示层同(主体,窗)多行互引注("·同主体见E5"),纯显示不动聚合;投影卫生批低优先。

**B6 [P1] E26/E27 irq_activity 双行 ×5同值/×3同值不再合并** — 定性:行为域正确(V4 键=subject+object+token+值带,且须 line/time span 重叠,`aggregate.go:525-558`;E26=:1008657-1065394 窗1、E27=:1617861-1650214 锚窗,不重叠=两次独立 capture,合并反而伪装单一测量)。真正噪音=两行都不带窗身份。
- 修向:◇ 邻近行标各自 selected_window 短注(窗1/窗2);P2,投影卫生批。

**B7 [P2] "trace causal node" 占位名进量表** — 定性:显示卫生——量表 quick-cell 通道缺 fold 专名分支。
- 产生链:`tool/answer_document_mutation_runtime.go:2282-2291` subject/object 双空→字面量;树行/明细块 fold 有专名(tree.go:5169-5173),明细 fallback tree.go:5180 同字面量;runtime.go 量表通道无 OnChainOverflowFold 引用→E25 与 ×9 fold 行在量表裸占位。
- 修向:量表 cell 对 fold 行复用树侧专名 helper;顺带中文化该字面量。

**B8 [P2] E19 混窗 bar(窗1 值对 151ms 画 45%)** — 定性:口径 gap(轻)——值源窗≠锚窗时 bar 是伪投影。
- 产生链:bar=value/model.WindowMS 无条件(tree.go:2999-3014,caller :3573-3587);"另一查询窗"注 :5813-5821(FullWindowStateWindow* 有值即渲染)——注与 bar 互不通气。
- 修向:FullWindowStateWindow 有值且≠锚窗→bar 降级(不画或 ░ 虚底+跨窗记号),数值列保留;投影卫生批。

**C3 [P1] 空标题"一级根因(Primary)候选"/"二级根因(Tertiary)阻塞调用"** — 定性:显示卫生+prune 边界(run 不可复现,QCE §7.13 机制静态推演)。
- 产生链:渲染面 `render/answerdoc.go:214-217` renderV2BlockOrderedList **无条件先打 `**heading**`** 再看 items;可达清空路径三条:①模型只 emit 标题;②prunePrincipalEnumerationExtraneousItems(`tool/answer_document_principal_enum_compile.go:119-170`,out=Items[:0] 全删可达)——本 run"4 区块·0 引用",items 全部无 row 覆盖是现实形态;③item 级渲染抑制全空(:219-231 renderV2BlockItem 空串跳过)。结论:枚举 prune 在此形态**会**吞光成员留裸标题,且模型只 emit 标题同样可达,双因不排他。
- 修向:renderer 空列表块卫生门(可渲染 item==0→整块不渲染;len==0 为精确信号,作硬门合规);prune 删空时 WARN log 供远程归因;不做系统补写(红线:系统不可代替 LLM 写答案面)。

**C4 [P0,散文口径] raw 42.6ms 当一级根因,VS-1 折减 0.208ms 被散文无视** — 定性:行为域(锚点未强制消费 family,同 eval 两 FAIL 类)+审计面口径缺失;引擎数据不缺。
- 产生链(折减可见面):判据+计算 `query.go:22-62`/:7941 detectPeriodicWakeupSource;探索可见:aggregate Summary 显式带 `periodic_source=true detected_period=.. lateness=.. effective_impact=..`(`query.go:8272-8275`)与 rank 行 effective_impact(`tool/trace_query.go:2883,3520`);typed 富注(`tool/trace_query.go:5601` 区 traceQueryTypedPeriodicSourceRichNotes);投影消费(`types/trace_causal_projection.go:1273`→表 E1 0.208+图例)。
- 丢失面:①探索散文第 5 轮自己把 42.6 写成"聚合影响"(标本 :48),同视图文本内折减字段未被消费;②A→B 手递只传 Summary 文本(红线),extract 关键发现沿用散文口径;③系统补充 4-note cap 把 periodic 注记切在第 4 名外(`answer_document_evaluator.go:12563-12592`,标本 :554-555 备注止于 dominant_state)——finalizer/复核在审计面都看不到折减;④finalizer prompt 无"周期源折减优先于 raw 聚合"消费指令。
- 修向(软引导批):(a)观测/系统补充 note 优先序:periodic_source=true 行把 effective_impact/periodic 键排进前 4(typed 精确信号选序,非硬门);(b)explorer/finalizer skill prompt 加消费指令(过 prompt 红线 checklist);(c)远期并入 typed anchor obligation lane(eval 战役方向)。

**C1 [P2] 频点 kHz 裸出(668000→模型读成"668 kHz 严重低频")** — 定性:显示面诱导。
- 产生链:raw kHz 整数打印 10 处:`query.go:3052,3055,3884,3966,3969,8527,8772,13394,13826`(工作树行号)+`cpu_frequency_census.go:201`(%d..%dkHz)。
- 修向:统一 helper 双写"668000kHz(=668MHz)"(保 kHz 原文供既有 parser/pin,加人读括注);改前先清点消费面 pin(TestFallbackWarnLogFormatPinned 类字面量测试随改)。

**D1 [P2] next-step 模板噪音+binder 具体指令缺失** — 定性:模板噪音(出场门用嘈声代理"窗数")+车道缺失(per-peer 指令)。
- 产生链:`tool/answer_document_mutation_runtime.go:3775-3942` runtimeTraceNextStepItems;归一化/双窗对比两条来自 runtimeTraceNextStepMultiWindowSteps(:3855,门=查询窗≥2)——单场景 3 查询窗即中门,与"对比场景"无关;binder 只有 s_sleep 通用句(:4236"排查反复唤醒它的对端线程、binder等待、锁与条件变量等待"),无点名对端条目;RN-14a 合成先例已在(`tool/trace_query.go:281`、`emit_investigation_complete.go:3780`)。
- 修向:①归一化条目出场门收紧:窗数≥2 且(对比场景 or 答案面引用了跨窗数值),否则降为一条;②on_chain binder_wait+peer 解析时合成"对 <peer> 在重叠窗执行 view=wakeup_chain/critical_blocking"具体条目(=A1 修向 2 同件)。

### §10.2 修复分批建议

- **P0-E 引擎批(binder 对端车道)**:A1 修向1(peer 子目标有界续链+`─binder对端─` 边词)+A2(reply join 服务端处理段+wait_object 注+白名单对端启发)+A3(peer_state 消费升级)。前置:causal_token_registry 新行裁定(先读 §7.2.1/账本 §7.4/§7.5)+NKR 登记 peer_chain_*/wait_object+R2' 六处同步;测试=合成双进程 binder fixture+golden。
- **P0-A 锚定覆盖批**:B1(锚选择精确前置,i/ii 需用户裁定)+B2(先披露句 (b),分子扩展 (a) 提裁定)+B3(连接句,随 B1 合流)。
- **P1-H 投影卫生批**:B4(微值深链尾折叠+depthless 降序)、B5(同主体互引注)、B6(邻近行窗身份注)、B7(fold 专名进量表)、B8(混窗 bar 降级)、C3(空列表块卫生门+prune WARN)、C1(频点双写+pin 清点)。
- **SG 软引导批(可先行,全 prompt/合成面,过 prompt 红线 checklist)**:C4(a)(b)+D1①②+A1 修向2(建议式对端下钻合成)。

复核要点:①A3/C4 共因=系统补充 4-note cap 的无差别截断——修 note 优先序一处,两项同收;②本批全部修向遵守"精确信号硬门/嘈声信号软引导"红线(B1 前置用 typed 实体匹配、C3 用 len==0、A2 启发解析只软填不设门);③E20/E19/E14 等标本坐标已录 §10.1 供回归 fixture 取材。

## §11 客户回访反馈#2 逐项代码级归因(2026-07-06;只读归因批,未动代码)

标本=/Users/han/Downloads/cust_trace_q2.txt(809 行 CLI 全程;双 Android systrace 对比:7.0B30SP22_7315.systrace 389.6MiB vs 6.0B138_3900.sys.systrace 476.6MiB;bindApplication 1.793s vs 0.884s;MiniMax-M2.7;锚窗 7.0=3680.800–3681.001、6.0=8144.400–8144.600,各 201ms;两 trace 均 index_event_limit 触顶,标本 :44)。行号口径同 §10:CFR 批仍在工作树未提交(涉 internal/tracequery/query.go、ipc.go、parse.go、internal/tool/trace_query.go 等),这些文件行号为归因时刻快照,复核以符号名定位;`internal/tool/answer_document_*`、`internal/types/trace_causal_projection*` 与 main@7f9369af 一致。首个双工件对比 + 首个纯 Android(非 donghu)形态标本——N1/N3/N7/N8 全部只在这两个形态下暴露,q1 无法覆盖。

### §11.1 逐项归因表

**N1 [P0,NEW;同族=§10-B1/B3 但机制不同] 对比总览 lead="未定位到链上主根因(见背景压力段)"×2,树却满是 on-chain 行(7.0 E10 有效归因 104.127ms)** — 定性:lead 回退梯的空 rank 桶分支缺失(RN-3(a) 设计边界)+指路句双重失真。
- 产生链:总览 cell=runtimeTraceProjComparePrimaryCell(`answer_document_mutation_runtime.go:1247-1258`)调共享选择器 runtimeTraceProjLeadSelect(`answer_document_mutation_runtime_tree.go:4313-4324`),`primary==nil` 即打固定串 :1253-1257。选择器三分支:①primary 桶 rank 头名;②**`len(runtimeTraceCausalProjectionPrimaryRoots(projection))==0 → return nil,false`(:4317-4319)——q2 走的就是这支**;③on-chain fallback(:4320-4322,Chain|Depthless 按 selection value 最大,:4335-4358)只在"桶有行但全降背景"的矛盾形态才运行(注释 :4309-4310 明言空桶保留 legacy 无结论行为)。q2 探索期以 wakeup_chain/critical_blocking/window_stats 为主,rank 家族观测未进 primary 桶 → 空桶 → E10(104.127)等 depthless 行永远轮不上;q1-B3 是"lead 与根分叉"(桶非空),本条是"桶空即弃梯",机制不同。
- 指路句双重失真:①"见背景压力段"随 fallback 串无条件出场(:1253-1257 无背景段存在性检查);②同一行 背景压力 列="—":runtimeTraceProjCompareBackgroundPressureCell(:1324-1337)只认跨线程聚合行(runtimeTraceProjCrossThreadAggregateType),q2 背景段全是 per-thread critical_blocking d_state 行(7.0 E24-27)→ cell 落 "—" 而树内 ▒ 段确有内容——lead 指路的"背景压力段"在总览表自己的列上是空的。
- 修向(=用户"lead 回退梯"):runtimeTraceProjLeadSelect 补第四支——空桶∧model 存在 HasData 的 Chain|Depthless 行 → 走同一 on-chain fallback,fallback note 换措辞("无 rank 数据,按窗口内最大 on-chain 等待";:4366-4377 已有双措辞先例);条件全 typed(len==0 ∧ fallback!=nil,精确信号硬门合规)。"见背景压力段"指路 gate 在该工件 model.Background 非空(len 检查,精确);cell "—" 时指路句改指树内背景段或不指。单工件结论行同函数同修,B1(i) 精确前置与本条同一改动点,并入 P0-A。

**N2 [P0,NEW] 跨查询窗合并重复计数:E10 窗口投影 183.940ms=154.184(2.25s 窗)+29.756(201ms 窗),occurrence 3680.7995–3680.8192 两窗重叠段双算 ~15.2ms** — 定性:R2 求和合并无时间区间判据,跨窗同物理段叠加。
- 产生链:两次 wakeup_chain 查询(选定窗 3680.569–3682.819 与 3680.800–3681.001)各自发布 per-occurrence wakeup_causal 行,四实例 104.127/50.057/15.206/14.550(标本 :766-767 备注可逐字核对),SUM=183.940 与量表 ×4(14.550–104.127ms) 完全吻合;合并点=R2 traceCausalProjectionAggregateSameKind(`trace_causal_projection_aggregate.go:649-801`):键=`subject+"\x00"+object`(:670),≥3 员即无条件 `sum += display`(:696)→ `aggregate.ImpactMS = sum`(:742-743)。其中 15.206 的 occurrence [3680.7995–3680.8192] 完整落在 104.127 的 [3680.6909–3680.8192] 内——同一段 runnable 被两个查询窗各切一刀后按不同实例求和。
- 考证:①R1 same-fact 键(:118-134)=subject+impact(3位)+line span,对跨窗重叠形态结构性失效(impact 与 line span 均不同:117231-140719 vs 136600-140719);②occurrence_windows 只活在 note 字符串,聚合层 0 处消费(全文件无该词);③**但节点级时间区间在手**:node.StartTs/EndTs ← record.Span(`trace_causal_projection.go:1255-1260`),证据索引 E10 [3680.691–3681.079s] 即其外显——R2 只是不读。
- 修向:R2 成员预扫——同主体同 token 成员间时间区间重叠(StartTs/EndTs 相交)→ 重叠对做区间 union 归一(保 ×N 计数、值改 union 口径并标注),或保守案:拒并+各自带窗身份注(联动 q1-B6 窗身份注同件)。现有字段即够,无需新 typed 车道;墙钟纪律(union 非求和)与 v3 禁Σ裁定一致。建议独立正确性小批(数字翻倍属 P0 正确性,不混卫生批),改动集中 aggregate.go R2 一处+golden pin。
- **交付补记(2026-07-06,N2 批+F 复核吸收)**:union 口径已落地(区间代数单一权威 `trace_causal_projection_interval.go`+R2 消费点+树图例 `×N(a–b)union` 第四式/无损块"union 口径(K 窗重叠段不重复计),原始和 …ms 供对照"+窗来源 roster);精确门=跨窗身份(±1ms 归槽)∧区间相交,裸重叠/同窗/无窗身份一律保持 SUM(E9/E10 distinct-fact 裁定+PTV6 包络重叠否决)。
  - F-2 结构前提转 in-code 门:union 扣减假设"成员值≤自身 occurrence 区间墙钟";任一跨窗成员 value>区间长×(1+1e-9) 整组 fail-open 回 SUM(密度>1 的 cpu·ms 形态结构上必 SUM,pin=TestTraceCausalProjectionUnionDensityGateFailsOpenToSum)。
  - F-3 窗身份 ±1ms 容差沿用 F-2/PTV5 Q3 两既有消费者的浮点表示行为(同一 traceCausalProjectionFullWindowSameWindowToleranceS 常量),不另立案。
  - F-4 无窗身份成员桥接:不参与扣减、不进任何窗集,fail-open 回求和语义(合"精确信号硬门/嘈声信号软引导"红线);(a) 表 union 行不举 mergedSum 旗,gated"数值为总和"行永不注解 union 值(runtime.go 消费面本批未动,union 语义由树图例+无损块承载)。

**N3 [P0,NEW] 对比场景两侧锚窗不同相位:7.0 锚最热段(window_sweep 475+272 switches),6.0 锚死窗(target sleep 18.578ms,唯一链行 0.369ms)→ 总览行际对比空转** — 定性:锚各自"最后 selected_window 先到先得",对比形态无相位对齐无披露。
- 产生链:锚=traceCausalProjectionAnchorWindow(`trace_causal_projection.go:962-1003`):优先 frame_target_resolution,否则 fallback 循环内无条件覆写(:1000)=**最后一条**带 selected_window 的 wakeup_causal/root_cause 记录胜出(嘈声信号:发布顺序);家族白名单 :1039-1044,键白名单 `trace_note_keys.go:86-99`。逐 partition 独立编译(:409-631,:607-608 各自应用),CMP-A 无跨工件协调。7.0 末次 wakeup_chain 窗=3680.8–3681.0(标本 :118)、6.0 末次=8144.4–8144.6(:123,校验路顺手选的窗,恰为死窗)→ 总览 "on-chain 已归因 65.232 vs 0.369" 读作"7.0 有因 6.0 无因",实为选窗伪象。
- 既有挂点:总览披露行已有先例两条——时基不相交 :1213-1218(gate=TraceCausalProjectionTimeBasesDisjoint,`trace_causal_projection_partition.go:603-624`)与 F3 窗长不等 :1196-1204;投影窗列 :1163-1170。
- 修向:(a)披露先行(纯算术,零口径风险):双 partition 且各自 WindowStart/End 与 Span 包络的相对偏移差>阈值(或一侧锚窗 on-chain 已归因≈0 而另一侧>0)→ 总览补行"两侧锚窗相位不同(7.0 锚现象热段/6.0 锚静默段),行际数值不可直接对比";(b)对齐案(需裁定):对比形态下以主工件锚窗在其 Span 的相对偏移映射到另一侧选窗重锚——牵动锚语义,建议只作裁定议题。并入 P0-A(与 B1 同函数域)。

**N4 [P1,repeat-of-§10-B2 家族新形态,机制修正] 覆盖分子 65.232=depth-1 MAX,同深度唤醒边 E9(RpcSerialize 22.332)参赛但被 MAX 丢弃——非"边类型筛选"** — 定性:口径纪律(跨主体不加和→同深度取 MAX)的信息损失,非 bug;但披露缺失使读者以为 E9 漏收。
- 产生链(机制勘正):E9 走 depthAttach[1]→roots 车道(`answer_document_mutation_runtime_tree.go:868-874`),Kind=Chain、Depth=1、HasData=true——**与 E3 同池参赛**;分子=runtimeTraceProjChainDepthCumulative(model,1)(:4880-4902)对同深度行取 `if v > max` 单一 MAX(:4897-4899),65.232(E3)>22.332(E9)→ E9 值被弃。q1-B2 是"depthless 不入分子"(车道排除),本条是"同深度跨主体 MAX 丢弃"(纪律丢值),同族不同层。
- 考证:E3 睡眠段 [3682.481–3682.578] 与 E9 runnable 段 [3681.744–3681.785] **时间不相交**(证据索引可核)——本例 union 下界=87.564ms 完全合法;节点 StartTs/EndTs 在手(:1255-1260,同 N2)。
- 修向:随 q1-B2 同批两案——(a)分子升级:同深度跨主体行按时间区间 union 计下界(相交部分不重计,与墙钟纪律一致;精确信号=区间代数);(b)保守披露:覆盖行补"同层另有 N 项唤醒边(最大 X ms,跨主体不加和)未计入"句(B2(b) 同款零口径)。并入 P0-A,B2 修向文本需把"depthless"扩为"depthless+同深度跨主体"两形态。

**N5 [P1,NEW] 有效归因>窗口投影 7 处(E3 88.280>65.232、E5 63.838>37.067、E11 113.645>93.587、E13/E14/E15/E16/E17)无口径桥** — 定性:三口径(投影/有效/实际)并排,⚠ 只装在实际列,有效列裸奔。
- 产生链:值源=effective_impact_ms 富注(`trace_causal_projection.go:1273`)←引擎 rank-lane 单源(`tool/trace_query.go:5387` traceQueryCausalImpactEffectiveNoteValue→tracequery.WakeupCausalImpactEffectiveImpactMs,PTV5 Q1;CFR 快照行号)——反转候选=R5d gated composite、普通行=raw attribution,天然可超窗口投影(E14 53.553 甚至>实际 45.473:有效口径含链路/composite 成分,非纯状态时长);量表渲染 `answer_document_mutation_runtime_tree.go:5074-5095`:actual 列有 ⚠(:5093-5094,gate=runtimeTraceProjCrossWindow :4031-4040),effective 列无任何标注。
- 修向:effective cell 复用同款精确 gate——`EffectiveImpactMS > max(ImpactMS,CumulativeImpactMS)*1.001` → 缀注(如"⚠超窗口投影(排序口径)"),量表口径块补一行"有效归因为排序口径,可大于窗口投影";纯增注零口径风险。并入 P1-H。

**N6 [P1,NEW] 全词保障逃逸:6.0 树"◦ 14.227ms [E5]"裸值行(binder_wait 家族)+E3/E4/E5 同段同值三谓词三行** — 定性:词源三路全空无兜底+跨谓词同段发布无互指。
- 产生链(裸词):E5=wakeup_chain 车道 BinderWaits 发布(`tracequery/query.go:8291-8294` 区,Type="binder_wait";CFR 快照行号),peer=unknown(见 N8)→ 无 BlockingPeer 词;非状态谓词 → StateKind 空(`trace_causal_projection.go:1265-1271` 双路都不命中);无 TypeToken 状态词 → stateTag 组装(`answer_document_mutation_runtime_tree.go:3630-3678`)三路全空。PTV6-D 全词保障(runtimeTraceProjApplyCauseWordGuarantee :3164-3190 区)只救"截断",不救"本无";图例虽声明"无形态词行=候选影响类"(类别词不逐行重复),但该行连主行三要素都不齐(值+证据,无词)——PTV4 零省略原则在此形态破口。对照:E1(critical_blocking 家族)同为 peer 未解析却有"对端线程未解析"词,词有无取决于发布家族,非语义差异。
- 产生链(三行同段):E3(wakeup_causal [8144.586–8144.601])/E4(missing_wakeup,无 line span)/E5(binder_wait :134239-135855)同描述一段 14.227ms 等待;R1 键(`trace_causal_projection_aggregate.go:118-134`)=subject+impact+line span——E4 无 span 直接返 ""(:119-121),E3/E5 span 不同 → 三行并存,谓词根本不在键里,即使同 span 也是设计上不折(值语义不同,B5 家族裁定)。
- 修向:①词兜底(显示层):三词源全空的行渲染类别词"候选影响"或 peer 词(明细块 :5180 区已有该词,主行复用即可,零新语义);②同段互指(B5 同款展示层注):同主体、|值差|<ε、时间区间重叠的跨谓词行 → 从属行加"·同段见E3"互指注,不动 R1。并入 P1-H。

**N7 [P1,NEW;CMP-4 家族] 7.0 主线程全窗 state_churn 快照缺失/被截:快照行标"查询窗 3680.569–3682.819s"实际只有 128.327ms/3 段对齐窗观测;答案核心数字(sleep 1430ms/71.6%)审计面零佐证;6.0 侧却有全窗行——不对称** — 定性:快照资格 predicate-blind+行标签硬编码+tier 闸主体名失配,三因叠加。
- 产生链(假"state_churn"行):快照资格=runtimeTraceMetricSnapshotValues(`answer_document_mutation_runtime.go:3380-3405`)只认 9 个 churn 键齐全,**不看 Predicate**;wakeup_causal_impact 富注恰好全键发布(`tool/trace_query.go:5395-5403`+actual_* :5404-5408;CFR 快照行号)→ 目标行(main-6565,churn 口径=对齐 occurrence 窗:fragments=3/switches=2/sleep=128.327,与 :766 note target=128.327 逐字吻合)入池;行标签**硬编码** `subject+" state_churn"`(:3094-3098),家族失真;窗前缀取 selected_window note=整查询窗(:3106-3111)而值是对齐窗值 → "2.25s 查询窗只见 128ms";实际对齐窗 inline(:3577-3638,PTV6-C 裁定C)有披露,但主行"sleep 128.327ms(占该线程观测时长100%)"(H13 措辞)在此形态下仍读作装满。
- 产生链(全窗行不对称):tier=candidateTier(:3211-3221),chain tier 判据=subject canonical 名 ∈ chainSet(投影树主体名单,:3165-3193);7.0 全窗 window_stats churn 行主体名=**com.xs.fm.lite-6565**,而 7.0 树主体全用 **main-6565** 名(同 tid 双名,canonical 不同键)∧ 不匹配用户实体(实体=两文件名+bindApplication)→ rest tier → `hasChainCandidate→rest 全弃`闸(:2992-2994)丢弃;6.0 全窗行主体 com.xs.fm.lite-21538 **恰好是树内 E7 depthless 行主体** → chain tier → per-window floor(:3040-3051)存活。同线程双名失配与 W2 R面"根标签实体比对"同病家族。次要贡献:ledger 入口 cap 64(:2937)、churn 发布 cap traceQueryWidthTypedFamilyRowCap(`tool/trace_query.go:6086`)。
- 修向(=用户"答案核心对比数字必进快照",CMP-4 选题相关性):①答案强引用正向车道——答案面引用了某观测的数值(现有 :2965 coverage 检查是反向"已覆盖不再显示",需加正向"被引用但无佐证行→强制入选",按 subject+数值 variant 匹配,:3322-3374 变体匹配器可复用);②行标签改用 record.Predicate(:3096-3098 一行);③tier 判据主体名先做同 tid 归一(canonical 键加 tid 维度,精确信号);④churn 值口径与窗前缀矛盾时(值总和≪窗长)标"对齐窗观测"而非裸挂查询窗。并入 SG/快照批(与 §10-C4/A3 的 note 优先序修同域)。

**N8 [P1,NEW;A1 前置] binder_wait 对端全 unknown-thread(7.0×7+6.0×4):q1 HarmonyOS OS_IPC 可解析,q2 Android binder:8815_x 命名全失败** — 定性:Android 形态配对链双断:dest_thread=0 常态关死 endpoint 兜底,receive 行 join 又受索引预算截断;dest_proc 在手不用。
- 产生链(CFR 快照行号,符号定位):parse 覆盖面无缺——EventBinderTransaction/Received/Reply 事件名与 kv 抽取均覆盖 Android ftrace 格式(`parse.go:1732-1745`/`:2209-2222`,kvRE :25);配对 `ipc.go:94-117`:①chooseBinderReceive 按 TransactionID join 窗内 receive 行(:60-63 收集,窗外/未索引即空);②失败后 endpoint 兜底 **gate=`edge.DestThread > 0`**(:107)——Android BC_TRANSACTION 发进程池 dest_thread=0 是常态,兜底结构性死;③`edge.DestProc`(=8815,线程名 binder:8815_x 即其外显)在手但不参与兜底 → Receiver 零值;binder_wait 透传 wait.Peer=edge.Receiver(`query.go:8306-8386`,:8346),receiver-missing 已有 caveat(:8379-8381)但 peer 注仍打 unknown-thread(threadLabel 零值分支 `query.go:14264-14275`);两 trace 均 index_event_limit(标本 :44)→ receive 行缺索引为首要嫌疑。q1 donghu 成对=dest_thread 语义/receive 可用性差异,无平台闸(parse 层无平台条件)。
- 修向(P0-E 引擎批前置):①dest_proc>0 兜底——发布进程级 peer(形如"binder pool of pid 8815",复用 :107-111 信心分层先例 0.62→更低档,精确字段非猜测);②索引预算下 binder 事件族优先保留/receive 反查允许小幅越窗(send 后 ≤N ms);③reply join(§10-A2 同件)补服务端处理段。①落地后 A1 对端续链/A3 PeerState 在 Android 形态才有输入。
- 附 N11c 相关存量面:binder 事务计数 typed 面已存在——interaction_stats BinderToTarget/BinderFromTarget per-peer 计数(`tracequery/types.go:1911-1932`,BuildInteractionStats `query.go:10567-10665`,summary :10635-10636);ipc_graph 只发边不聚合计数。

**N9 [P2,NEW] 0.000ms 成因自环行:"└─成因─ ◦ 线程名未记录(tid 21564) 0.000ms 候选根因"(父行=同 tid running 0.369ms)** — 定性:成因行出场无零值门、无自指抑制。
- 产生链:trunk 建树时同主体次值节点全进 extra(`answer_document_mutation_runtime_tree.go:784-798`),extra 无条件发布为 Kind=Cause 行(:816-821)——无 `ImpactMS>0` 地板、无"与父行同主体且未携带父行没有的谓词/token 信息"抑制;链行的 `EffectiveImpactMS<=0 continue` 门(:1119 区)只作用于窗占比过滤环,不管建树。
- 修向:成因行出场门两条(均精确信号):①零值地板(ImpactMS<=0 且无独立 token/谓词信息 → 不发布);②自指抑制(同主体且展示词集为父行子集 → 折入父行)。并入 P1-H。

**N10 [P2,repeat-of-§10-B7/B4/D1] q2 证据补充(只补证据行,不另立案;§10 正文不动,witness 记此)** —
- §10-B7(占位名进量表)q2 witness:量表 E23 行"trace causal node [E23] | 52.332ms"(标本 :312)——树行有专名"其余 1 项(链上折叠)(main-6565)"(:261/:479-484)而量表 quick-cell 裸占位,与 B7 产生链(`answer_document_mutation_runtime.go:2282-2291` 区)完全同形。
- §10-B4(深链噪音)q2 witness:"…省略26节点"(:216)+L32/L33/L34 微值深链行 E6/E7/E8(28.230/13.201/12.896ms 挂深度 32-34)+"◦ main-6565 中转 ↺"×2(:210/:213)——插入序 DFS 无 per-depth 量级门(tree.go:738-751/:928-940 区)再证。
- §10-D1(next-step 模板噪音)q2 witness:下一步 6 条中 1/2/4 三条互重(:751-754:"同口径窗对比"/"归一化后对比"/"不可同轴对齐,以相对指标为准"三句同族;#4 还与总览披露行 :169 重复),#3(逐窗同口径因果采样)是唯一带增量信息的对比条目;q2 为真对比场景,runtimeTraceNextStepMultiWindowSteps 出场合规,问题在族内去重——D1① 收紧时按"同一语义家族只出最强一条"处理,并补 D1②/A1修向2 的对端点名条目(q2 形态:点名 binder:8815_1 对端在重叠窗跑 critical_blocking/wakeup_chain)。
- 附:N3 揭示的"锚窗死窗"与 D1 族修互补——若 #3 合成时点名"6.0 锚窗 on-chain≈0,建议改锚热段重跑",单条即可救活对比。

**N11 [P2,行为域记录] prose 三处(引擎数据面均在或可补,不立硬门)** —
- (a)"6.0 D-state 318ms……不阻碍主线程执行流水线"(标本 :153):逻辑硬伤——该 D-state 主体就是主线程自身(com.xs.fm.lite-21538),typed 面早已说清:E7 链上·深度未解析行 + 系统补充 :795-797 `chain_relevance=on_chain`;散文与 typed 面正面矛盾。行为域=锚点未强制消费 family(§10-C4 同族);系统辅助=L4 self_consistency BODY-vs-evidence 盲点的既有候选(MEMORY 已挂),不新立案;SG 批 finalizer 消费指令顺带覆盖("on_chain 主体=目标自身的 D-state 不得叙述为不阻塞目标")。
- (b)"6.0 频率数据为空说明测量精度不同"(:127):误读采样覆盖为精度差。typed 面已有半件:E6 行"频点数据不全,无法折算"(:588)是逐行 caveat;缺的是窗级"该窗无频点样本(采集面差异,非低频/非精度证据)"census 空窗 caveat——cpu_frequency_census(RFC #71,bd605684)聚合面加空窗分支即可,软引导不设门。挂 SG 批候选。
- (c)"20+ vs ~6 同步事务"关键对比数无 typed 佐证(:126-127→:157 进正文):发布面存在(N8 附:interaction_stats per-peer BinderToTarget/BinderFromTarget 计数),但 q2 只对 7.0 跑过 interaction_stats(标本 :62 区),6.0 侧无对应观测,"~6"为散文估算。行为域;系统辅助=对比场景 next-step 合成"双侧同窗 interaction_stats"条目(D1②/N10 同件),若双侧观测齐则计数自然可进快照/佐证面。

### §11.2 修复分批建议(与 §10.2 四批的合并关系)

- **P0-A 锚定覆盖批(扩)**:+N1(runtimeTraceProjLeadSelect 空桶第四支+fallback note 新措辞+"见背景压力段"存在性 gate)、+N3(总览锚相位披露行,纯算术;对齐重锚只作裁定议题)、+N4(B2 修向文本扩为"depthless+同深度跨主体"双形态:union 下界或披露句)。理由:N1 与 B1(i) 同函数域,N3 用 B1 同族 anchor 链,N4 与 B2 同分子函数——四项一批一轮 golden。
- **N2 独立正确性小批(建议置于 P1-H 之前,P0 性质)**:R2 跨窗重叠区间 union/拒并+窗身份注(联动 q1-B6)。不并 P1-H 的理由:数字翻倍是正确性缺陷非显示卫生,需单独 pin(双窗 fixture:重叠 occurrence 求和≠union);改动集中 `trace_causal_projection_aggregate.go` R2 一处。
- **P1-H 投影卫生批(扩)**:+N5(effective cell 复用 runtimeTraceProjCrossWindow 同款 gate 加注)、+N6(裸值行类别词兜底+同段跨谓词互指注,B5 互引注同族同机制)、+N9(成因行零值门+自指抑制)、+N10 witness(B4/B7/D1 各自条目按 §10 修向执行时带上 q2 fixture 坐标)。
- **P0-E 引擎批(前置扩)**:+N8——执行顺序上 N8① dest_proc 进程级兜底是 A1/A2/A3 在 Android 形态的先决条件(peer 全 unknown 时对端续链无输入);N8③ reply join 与 A2 同件;N8② binder 事件预算优先级为独立小项。
- **SG/快照批(扩)**:+N7(①答案强引用正向入选车道②行标签用 Predicate③tier 主体名 tid 归一④对齐窗口径标注——与 C4/A3 的 note 优先序修同域,可一批)、+N11(a=finalizer 消费指令一条、b=census 空窗 caveat、c=对比场景双侧 interaction_stats next-step 合成;全软引导,过 prompt 红线 checklist)。

复核要点:①N1/N3/N4 三项与 q1-B1/B2/B3 是同一"depthless-heavy 投影在头部面全灭"病灶的四个出口(结论行、总览 cell、覆盖行、锚),P0-A 一批修完后 q2 总览应给出 E10 lead+相位披露;②N2/N4/N6 共用同一底座事实——节点 StartTs/EndTs 已在手(`trace_causal_projection.go:1255-1260`)而三个消费点(R2 合并、分子、跨谓词互指)都不读区间,一次引入共享区间重叠 helper 三处受益;③N7 修向①的"答案引用→强制佐证"与 :2965 既有反向 coverage 检查共用变体匹配器(:3322-3374),不新造解析;④本节全部修向复核过"精确信号硬门/嘈声信号软引导"红线(N1 用 len==0、N2/N4 用区间代数、N5 用数值比较 gate、N7② 用 typed Predicate、N8① 用 dest_proc 字段;披露/互指/合成类全为软面);⑤q2 标本坐标(E10 双 aggregate 备注 :766-767、三谓词同段 :583-585、假 churn 快照行 :743-744、死窗锚 :551)供回归 fixture 取材;⑥两个跨 trace 形态(双工件+纯 Android)首次入库,P0-E/P0-A 的 golden 需各补一个 Android 双工件合成 fixture。


## §12 客户回访反馈#3 逐项代码级归因(q4 东湖 doFrame 3703298,2026-07-06)

标本=cust_trace_q4.txt(354 行,单 ftrace 15.8MiB,Choreographer#doFrame 3703298,窗口 33872.289161–33872.408222=119.061ms)。客户目录名"根因XX-UIsleepOpenDir"=ground truth,系统 E1 行已完整解析真根因(monitor_contention 112.223ms,持有者 NetworkKit_AssetsUtil_Operate_0-42067,持有点 AssetManager.list(AssetManager.java:1258))却被 lead/rank/散文三面全埋。归因=9-agent 工作流+9 对抗复核(8 upheld;Q4-K 复核遇 API 503,三承重主张由主会话抽验属实:头区仅 blocking=%d 计数 trace_query.go:3261-3263、blob 32KB head24K+tail4K 切中段 blob.go:44/50-53、补充面允许表无 blocking_kind 前缀 answer_document_evaluator.go:12563)。全文归因表见工作流产物(scratchpad/q4_attr_full.txt 落档时点),本节收敛要点。

### §12.1 归因表(产生链要点)

- **Q4-A[P0,NEW]** 锁证据 lead/rank 双面结构性排除:①rank 候选源全集(query.go:8441 buildRootCauseRankFrom)零锁车道,parseLockContentionPayload 唯一生产调用点=query.go:11868(critical_blocking builder),RootCauseRankItem(types.go:1777)无 BlockingKind/Peer/HolderSite 字段;锁 span 至多成泛型 trace_span 行(:8598)且不在 rootCauseTypeCanBeDirectOnChain 白名单(:10190-10203)→降 adjacent tier 永无头名;②lead 双门(tree.go:4313-4324):一级只读 primary rank 桶,二级 fallback 只准 Chain|Depthless — E1/E2 是 Kind=self,两门皆拒;结论量词取 CumulativeImpactMS=1.136(tree.go:4431-4437)。registry 定位:blocking_span RowToken=true(rank 收编合法零新登记),monitor_contention 保持 refinement 不升 token。
- **Q4-B[P0,NEW]** 承自门嘈声放行:attributeOnChainResourceItemsToWakeupDependency(query.go:10078-10123)material 判据 max(resourceMs=1.136, overlapMs≈112.1)≥max(16, target×0.35=39.26) 靠聚合窗重叠(嘈声)放行,EffectiveImpactMs 抬至 112.175(:10106-10108);on_chain tier 按 effective 排序(:9409-9414)→rank1;Score 承自后不重算(state_churn 有重算先例 :8677)→rank1 score 0.932<rank3 68.087 可见失谐。违反精确信号硬门红线。
- **Q4-C[P0,repeat-of-§10-A1+A3]** 锁持有者下钻缺失=跨线程阻塞对象续链车道的 lock-holder trigger 翼;§10.2 P0-E 原文"binder 对端"不自动覆盖 monitor_contention,批次范围需显式扩为三形态(binder peer/lock holder/blocking span object)。
- **Q4-D[P1,NEW]** 反转候选误报:分类器只看 waker/wakee prio 关系,无锁证据交叉核验;修向=同窗 monitor_contention(对端已解析,时段覆盖)存在时降级/加注(typed 精确判据)+建议式持有者下钻与 A1 修向2 共件。
- **Q4-E[P1,NEW]** 🎯 横幅反诬:锚真对(16547=用户点名线程),免责句拿帧号 3703298 当比对实体;与 §10-B1 同一 R2 比对链(tree.go:1905-1938)故障方向相反;analyzer 实体车道饥饿是 B1 修向 i 的共享前置(腿1 SG 先行)。
- **Q4-F[P1,NEW,R4-2 生产面回归]** E1/E2 同锁双行:comm 形态(112.223) vs pid=42067 形态(112.214)差 0.009ms<3% 带,peer 标签文本不同逃 V4 折叠;对端身份按 PID 归一后可折。
- **Q4-G[P2,NEW]** E5 机制构成句"+…+…共同作用"邀请加和误读(runnable 20.713 双现)— §7.10 红线(2)裁定落地面缺口;渲染措辞单源改+pin 随改。
- **Q4-H[P2,NEW]** E6 影响点名册 3/4 静默截断(漏 udk-irq-11-92 无"等")— PTS 家族逃过 §6.2 审计的 cap。
- **Q4-I[P2,NEW]** 快照选题面对 monitor_contention 对端实体零可见(锁持有者缺席快照);部分共因=§10-A3(peer_state display_only+4-note cap 掩埋)。
- **Q4-J[P2]** 系统补充混入整窗睡眠 idle 行 — Q2 批过滤未覆盖补充车道入口,修落引擎发布点共享守卫。
- **Q4-K[系统域裁定,NEW+repeat-of-§10-A3/C4/D1]** 散文不提锁=引擎/展示车道缺口非模型行为:bundle 组合文本>32KB 切 blob(head24K+tail4K),锁区块排序在 Window stats+frame timeline 之后确定性落中段盲区;头区仅 blocking=%d 计数零锁语义;Evidence pack 16 行 cap 追加序 chain→frame→rank→blocking 垫底双重出局;审计面允许表无 blocking_kind/holder_site 前缀(允许表级排除,比 4-cap 排序更硬)。行为互证:模型叙事与 head/tail 区逐一对应、中段 token 零。

### §12.2 分批合并(含 §10/§11 全景)

- **P0-E 引擎批**(范围显式扩):跨线程阻塞对象续链三形态(binder peer[HarmonyOS 已解析+Android dest_proc 兜底 §11-N8]/lock holder[Q4-C]/blocking span object[§10-A2])+rank 锁候选收编(Q4-A 修1,blocking_span RowToken 已 true)+承自门卫生(Q4-B:resourceMs 自身达标才 material,overlap 只辅助;Score 同步重算)+反转交叉核验(Q4-D)+bundle 头锁区块前置+pack 追加序(Q4-K 修1)+补充面允许表 blocking_kind/holder_site 前缀+per-type note 优先序(Q4-K 修3=§10-A3/C4 共因同座)。
- **P0-A 锚定覆盖批**:§10-B1(锚裁定 i)/B2(覆盖披露+分子)/B3+§11-N1(lead 空桶弃梯+指路句)/N3(相位披露)/N4+Q4-A 修2(目标态已解析对端从句)+Q4-E 腿2。与 RCX 设计合流。
- **N2 独立正确性小批**:跨查询窗重叠双算,区间 union(共享 helper 三收 N2/N4/N6)。
- **SG 软引导批(可先行)**:4-note cap 优先序(§10 共因)+C4 折减消费+D1/Q4-K 修5 per-holder next-step 合成+Q4-D 建议式下钻+Q4-E 腿1 实体软引导+§11-N7 快照佐证义务。
- **P1-H 卫生批**:§10-B4~B8/C3/C1+§11-N5/N6/N9+Q4-F(peer PID 归一折叠)/G/H/I/J。
- **RCX(裁定先行,#83)**:分层表达三件套设计稿;吸收 Q4-A/Q4-B 表达面修向;病灶总证=q1-B1/B2/B3+q2-N1/N3/N4+q4-A(depthless/self-heavy 形态头部面全灭家族)。

### §12.3 用户裁定(2026-07-06,五点全按建议放行)

1. **RCX 三件套放行**:①drill-debt 车道(typed DrillStatus{已下钻/未下钻·对端已识别/对端未解析},最大实测影响项未下钻时投影头部+bundle 头区强制披露+next-step 点名)②rank 记录分层化(Layer{目标态/直接阻塞/上游链/邻近/背景}+实测/承自分列+证据强度+DrillStatus;层内实测排序,禁跨层平铺;全局 headline 只准实测最大 on-chain 项;承自只作注记永不作硬排序键;score 从显示面删除或定义+承自后重算)③模型面根因骨架卡(bundle 头区+审计面高优先位,模型配散文不代写)。①③搭 P0-E,②搭 P0-A。
2. **lead tie-break**:实测优先 + 已解析对端行优先(承自值行让位,只得注记)。
3. **B1 锚归属=方案 i**:存在 target 匹配用户实体的 wakeup_chain path 记录时,锚优先给用户实体线程(精确 typed 信号);现状"第一条 path 先到先得"废止。
4. **B2 覆盖分子**:披露句(另有 N 项深度未解析…)立即上;分子扩展只并入 subject==目标且影响点已解析的行,墙钟重叠按 MAX/union 纪律。
5. **A1 token 车道**:新边词 `─binder对端─`/`─锁持有─` + `peer_chain_*` 注族,walk §7.2.1 registry 裁定流程 + NKR + R2' 六处同步;monitor_contention 保持 BlockingKind refinement 不升 row token;rank 锁候选收编走既有 blocking_span RowToken=true(零新登记)。

重提任何一点前先读本节;RCX 承自/排序纪律与 feedback_precise_signals_for_hard_gates 红线同源,永久生效。

## §13 回访#4(q5,2026-07-06):q4 同案远程 main 复跑 — 前后对照 + 两新 gap

标本=cust_trace_q5.txt(同 trace 同问题,客户基于含 SG=aa65c688 的 main 复跑)。**已生效**:🎯 横幅=‹用户关注线程›(analyzer 发出 16547 实体);系统补充 E1 行携 blocking_kind=monitor_contention+holder_site(SG 4-note 优先序实战生效);下一步第 1 条完整点名持有者+持有点(SG 合成车道生效,泛化让位)。**未生效即在途 backlog**:主根因行仍 block_io 1.136ms/覆盖 49%/E5 共同作用句/E6 名册 3/4/E1E2 双行/快照缺 42067/idle 行 — 归属 P0-E1(在途)/P0-A/P1-H 不变,q5 为新鲜 witness。

- **Q5-A[P0,NEW]** blob 逃生口被门挡死:探索第 2 轮模型 read_file `.codrax/blob/...`(bundle 82725B 截断的 payload_ref)被拒("need to use trace_query"),叠加 Q4-K 中段盲区 → 锁证据探索期结构性不可达,模型未见锁即完成调查 — 散文反转叙事的直接机制。归因待办:拒绝门产生链(O2 工件路径防线?evidence-lite?);修向=trace 结果 payload_ref 放行 read_file,或拒绝语合成窄视图重查指令(view=critical_blocking_calls)。
- **Q5-B[验证结论]** SG 散文处置义务单独不足(finalizer 可见 blocking_kind 仍反转头条)— 正面验证 P0-E1 修4(bundle 头区锁块)+RCX③ 骨架卡为必要件;不另立案。
- **Q5-D[NEW→P1-H 必做]** 同持有者双 next-step(comm 形态+pid=42067 形态):E1/E2 未归一穿透合成面,SG 去重键按标签文本分键 — peer PID 归一升必做,归一后 next-step 恰一条。

## §14 P0-E1 + Q5A as-built(2026-07-06,已推 main 1a05854f + 2ad764a6)

**P0-E1 引擎批(1a05854f)** — q4/q5 锁根因埋没引擎面闭环:
- rank 锁收编:root_cause_rank 消费已解析 monitor_contention 为 blocking_span head 行(主体=持有者,typed BlockingKind/BlockingPeer/HolderSite,resolved-peer-only 准入,泛型 trace_span carve-out);q4 112.223ms 锁行登 rank1。
- 承自门卫生(裁定§12.3-2):material 只读实测,承自值入 InheritedTargetBlockedMs 注记,Score 重算;1.136ms block_io 退位、score 失谐消。
- 反转 per-CLASS 交叉核验:candidate+runnable_wait 两类,锁观测覆盖等待区间→lock-wait-dominated 降级注+去 co-primary。
- **同锁折叠引擎源头**(collectBlockingSpanRows:kind+peer PID+区间重叠,富形态存活值取 max)— 一修收 Q4-F/Q5-D 全下游(E 行/rank/drill/next-step)。
- bundle 头区 top-blocking+largest-impact-undrilled(24KB 锚定);pack 序 chain→blocking→frame→rank。
- RCX① DrillStatus{drilled/undrilled_peer_known/peer_unknown} on blocking(binder_wait/io_latency/resolved lock)+rank。
- 复核 3 P2+4 P3 全吸收;drill 宇宙补全 own-conduct stats 全面;NKR display_only+golden/emit pin,causal_token_registry 零改。

**Q5A blob 逃生口批(2ad764a6)** — §13 Q5-A,**两轮对抗安全复核**后交付:
- 首轮四镜头复核发现**两个 P1**:(P1-1)注册来源③从 summary 文本抓 payload_ref= token → trace 受控文本可伪造仓外路径注册被 read_file/grep 读走(探针实证读到外部密件);(P1-2)blob read 经 buildLineIndex banner 解析落地成 current-source 可引用证据。
- 根修:①source③彻底删,只注册 typed RawRef/SourceRef(StoreBlob 自设=trace 内容不可达);②强制 .codrax/blob/ 前缀约束(canonicaliser 折叠路径穿越)+resolve 复验;③citation 四面同源单 matcher(read_file typed marker/buildLineIndex 只认 typed/emit_evidence 改道/extractor ResolveTraceQueryBlobRef);④第三段 probe-first 接 escape;⑤ObservedLineIndex .codrax 过滤。
- 二次安全复核用真实攻击串探针逐一证伪绕过(穿越/前缀段边界/typed marker 窗口/grep 旁路),**两 P1 闭合确认**;注入+grounding 攻击 pin 在。
- 教训入库:嘈声信号(summary 子串)绝不驱动硬 allow-gate;硬不变量不得依赖模型行为(软文案≠防线);安全批强制二次复核方可推。

## §15 回访#5(q6 东湖 doFrame 复跑,P0-E1 后)四维归因(2026-07-06)

标本=cust_trace_q6_opendir.txt。**P0-E1 已生效**:锁竞争 112.223ms 登 rank1(E3),散文正确=monitor+反转+持有点。用户点两问题+逐行。四维归因全 upheld(8-agent workflow)。

### §15.A on-chain running 有效归因未折算(用户问题1,P0)
E4 running 显裸 58.919ms 无折算,E7 反转节点显折算(供给缺口 17.702+running折算 16.697)。真因(复核修正):同一 WakeupCausalImpact 在 expandChain(query.go:13107)被双投影 —— ①入 res.CausalImpacts(:13147)②经 rootEvidenceFromCausalImpact(:13216)造 RootEvidence type=running,后者 rootCauseItem(query.go:8510 source=wakeup_chain)funnel **从不 set SupplyFoldBasis** → basis=nil → 无 fold_basis note → trace_causal_projection.go:1363 SupplyFoldComputed=false → E4 裸值(tree.go:3968,EffectiveImpactMs 走 raw TotalMs query.go:9667)。两折算除数不同(supply=大核 fmax→17.702 / weakCore=下游消费核 f_consumerMax→16.697,query.go:13277-13281/13392-13399)是 §7.10 红线(3)分车道设计非 bug,但披露缺口径说明。修向:RootEvidence running twin 补 fold 车道(引擎)/投影层 join 同 impact fold note 到 running 成因节点 + E7 机制构成句标注两折算除数口径(单源 answer_document_mutation_runtime_supplyfold.go:139)。

### §15.B 成因维度显示不清晰(用户问题2,P1 UX)
根因=引擎双发布:同一 running-dominant+反转候选节点发成两条墙钟重叠成因行(E4 running 58.919 全量 ⊃ E7 gated 37.410=runnable 20.713+running折算 16.697=PriorityInversionGatedMs query.go:13257);runnable 无独立有效归因行只作 E7 gated 子项双现(与 runnable-dominant 场景有独立 runnable_wait 行不对称)。显示层(P0-A)faithful 平铺状态维度成因行与判定维度成因行为同级 ├─成因─ 无轴区分。主根修=P0-E 引擎去双发布+成因状态轴平行化(续 §14 collectBlockingSpanRows 折叠家族);措辞修 §7.10/Q4-G 单源。

### §15.C 双向锁 bug(P0 回归 — P0-E1 rank 锁收编 1a05854f 引入)
三 P0 全 upheld:①同一物理锁 span 被双 lane 双向发布(waiter→holder + holder→waiter,行号区间完全相同);②E3 rank 锁行方向反转(holder-subject 行渲染成 waiter 行,"持有者"填被阻塞方 .ugc.aweme.lite);③next-step 第1条把 waiter(.ugc.aweme.lite-16547)当持有者点名下钻。用户可见错误引导。修=引擎去双向发布(critical_blocking builder)+ E3 方向 + next-step 持有者身份取值。**优先级最高,回归须速修**。

### §15.D 逐行(7 findings)
- gap③[P0 新根因,新立案 P0-A 覆盖批]:覆盖 94% 口径失真 —— 锁 span 112.223 略超 sleep 态 112.175(0.048ms)触发分母静默退整窗,把本应 ~100% 目标覆盖压成 94%,伪造 6.838ms 未归因残差。
- 其余[P1-H/SG]:E5 IO延迟名册 3/4 截断(§12 Q4-H repeat)/E7 机制构成 runnable 20.713 双现加和句(§12.3 Q4-G repeat)/下一步同持有点双现(§13 Q5-D repeat)/E11 VSyncGenerator peer_state 未展示(P0-E2a 已解析但未显)/◇邻近 E8/E9 页缓存抖动无 inode 区分标签/E5 ×6 与 (+11) 并列困惑(诚实澄清)。

### §15 批次归口
- **P0 回归速修批(双向锁)**:§15.C 引擎去双向发布 + 方向 + next-step 身份。
- **P0-E 引擎批(供给折算+成因去双发布)**:§15.A running twin fold 车道 + §15.B 去双发布/状态轴平行化。
- **P0-A 覆盖批**:§15.D gap③ 覆盖口径(锁 span 略超 sleep 的分母退整窗)。
- **P1-H/SG**:§15.D 其余 witness(多为 §12/§13 已立案 q6 复现)。

### §15.B/Q4-G 显示层交付(RCX²-成因维度平行化批,2026-07-07,纯显示层)
**已交付(措辞单源 answer_document_mutation_runtime_supplyfold.go;含 RCX² 复核 F1-F4 回炉)**:Q4-G 加和句式根治 —— 机制构成句去 `+…+…共同作用` 加和邀请,改「各口径独立、不可加和」leader + 无空格 `·` 并列(F3:zh 面与既有 tag 内部惯例同形,与 tag 间 ` · ` 分隔视觉区分,防邻 tag 误读第四口径;en 面保留其自身带空格惯例);**每个数配自己的尺子**(F1 修正:§15.A 两折算除数披露落在实际被折的数上 —— 供给折算缺口 `按大核满频折算` / 调度压力 runnable `就绪排队积压口径` / 反转构成内 running 分量 `按下游消费核折算`;runnable 分量 `全额`(producer 契约 counted in full);gated **总量只标 `gated 口径`** 不冒称折算,机制名去"折算"字=`优先级反转 X ms(gated 口径,内含 runnable A(全额)+ running 折算 B(按下游消费核折算))`)—— 唯一加和"+"锁在该节点自身分解括号内。F2:clause gated 总量与同行 `有效归因` tag **同源**(runtimeTraceProjInversionGatedTotalMS 消费 EffectiveImpactMS=引擎 rank-lane 单发布;两 %.3f 分量 note 复加可差 0.001=round3(a)+round3(b)≠round3(a+b);PeriodicSource/gated=0/未发布三角落回退分量和 —— 理想源 priority_inversion_gated note 现无投影消费者,接线需 internal/types 新字段+parse,超本显示批文件边界,留 P0-E 引擎批提升)。非 Triple 行独立 `影响构成` tag 经同一单源 composition text 顺带获得同款除数披露。三面(结论行/树尾 tag/明细表)单源同改。pin 随改(supplyfold_vs2_test.go despaced verbatim + `共同作用`/`acting together`/`优先级反转折算` 负向禁 + F4 无空格旧形 `下界)+` 一并入负向门 + F2 round3 分歧 fixture(20.713+16.697 显示复加=37.410 vs 引擎单源 37.409,同行两面同值断言)+ 总量冒称折算负向臂;d_round/ptv6c 影响构成 pin 随改)。措辞词面遵用户裁定"按 X 折算"(非"对");RN-16 车道 lint 绿(lane-lock 词全留原单源函数)。
**留 P0-E 引擎批(显示层不能凭空合成,no_system_backfill)**:
- **E4/E7 同段交叉注(pin② 退档)**:E4 裸值 running 行与 E7 gated 反转行是同一 running-dominant 节点被引擎双发布(§15.B),但**两节点间无 typed 同段身份**(无 twin/segment_id/cross-ref;R1/R2 merge 键因值/object/predicate 全异结构性不折)。显示层若凭 subject+反转候选+值包含(58.919⊃16.697)启发式交叉注,即"硬显示声明踩嘈声信号",违 `feedback_precise_signals_for_hard_gates` + `feedback_no_system_backfill`。**根治=P0-E 引擎去双发布**(那里天然同段身份)。本批显示层止血=E7 clause 已把反转构成内的 `running 折算` 分量明标为**折算口径视图**(`按下游消费核折算`,非独立第二段 running;gated 总量按 F1 只标 `gated 口径` 不冒称折算),读者可辨这是同段 running 的折算视图。
- **runnable 独立有效归因行(§15.B 显示半)**:running-dominant 时 runnable 只作 E7 gated 子项双现,无独立 runnable 成因行(与 runnable-dominant 场景有独立 runnable_wait 行不对称)。引擎未发独立 runnable 成因节点,显示层不能凭空造(no_system_backfill)。**留 P0-E 引擎批补发**;本批只确保 E7 里 runnable 口径标注清晰(`就绪排队积压口径` 已标)。
- **成因状态轴平行化(§15.B)**:状态维度成因行(E4 running)与判定维度成因行(E7 反转候选)同级 `├─成因─` 无轴区分 —— 属树结构/引擎双发布形态,**留 P0-E**(续 §14 collectBlockingSpanRows 折叠家族)。

## §16 SPL:q7 span 定位绕圈归因(2026-07-06,用户点名)

标本=cust_trace_q7_cmp.txt 探索期前 13 轮。**根因=引擎解析覆盖 gap**:客户 trace 的 bindApplication marker 是 `print: 0x0: 15|bindApplication`(数字前缀 print 变体,`N|name` 两字段),trace-mark 识别链(parse.go isDirectTraceMarkPayload :2724)只认 B|/E|/C|/S|/F| 前缀 → EventTraceMark 永不置位 → **span_window 结构性拿不到 span**(query.go:5874 第一行就滤 Type!=EventTraceMark),模型被迫 grep 绕圈是必然后果非能力问题。

三个叠加放大器:①**0 命中 hint 门控反了**(query.go:14728):"丢 event_types"建议只在 EventTypes 为空时发——恰好在模型设了过窄 event_types(第 5-9 轮空转)时被关掉;②thread=bindApplication(span 名当线程名)静默全 0 零诊断(resolveThread query.go:1361 PID=0 空 ThreadRef 下游静默);③无 span_locate recipe/对比黄金路径(BuildRecipe 6 recipe 全假设窗已选定)。

**修向三层**:
- **T-span 引擎根修(前置实测)**:`N|name` 变体进解析(parse.go:2706/2724/2666 扩展)。⚠ 语义歧义风险:15 可能是 depth/counter/async cookie,误当 sync B/E 配栈会产假 span 窗 — **必须先 ../customlogs 复放确证 begin/end 配对形态+N 语义+普遍性**(标准 atrace 是 B|pid|name,convert_test.go:585;此变体是 vendor 采集链形态,§7.11 平行审计未覆盖)。普遍则进 harmony flavor 封闭集,仅此一家降软识别。
- **D-diag 诊断引导(ROI 最高,先落)**:①反转 0 命中 hint 门控+跨类型反扫("pattern 在 N 个非所选类型事件里出现,去掉 event_types");②thread 未匹配但该词在 SpanName 集合的 typed caveat("这是 span 名,改用 span_window")。两条即可把 13 轮压到 4-5 轮。判据全精确(PID==0 布尔/verbatim 集合命中)。
- **R-recipe 软引导**:span_locate recipe(event_search 裸 pattern→span_window→取窗)+ span_window 教学补前置链与变体警示 + 对比场景黄金路径(两 trace 各 span_locate→各自归一化对比,与 CMP 除窗长教训合并)。

批次:D-diag 先行止血(等 P0-E2b 腾 query.go)→ T-span 待实测确证 → R-recipe 随 SG 车。

## §17 回访#6(q7 双trace对比复跑)四维归因(2026-07-06,全 upheld)

标本=cust_trace_q7_cmp.txt。**已生效确认**:对比总览 lead=binder 反转(N1 修复实战生效,空桶串 0 命中走 rank 头名支)、树根同源一致;CFR/CFR-2 判定=已生效且正确(6.0"频点数据不全"=该区段真无 cpu_frequency 采样,fail-open 诚实终态;7.0"running 折算 0.000"=反转折扣对 runnable 主导节点的正确结果 — 维度C 无代码修复项)。

- **A[P0×2+P1] 散文单侧跑偏**:finalizer 头条=AggregateOnly 背景聚合(supply_pressure 35027/pressure_density 50.04,registry :161-162 typed 定死跨线程 cpu·ms 非墙钟),typed 面(lead+树根)正确。三条 trace SG(C4/Q4K4/N7)无一禁此形态;explore-skill :126 聚合软引导钉在"chain"不约束 emit_investigation_complete.reason 交接散文(explorer.go:967→answer_surface_plan.go:1657 透传)。修=SG-2 双面:finalizer 新 SG"背景压力头条禁令"(存在 rank=1 on-chain 因时头条必须命名之;AggregateOnly 只作环境证据带单位;复用 SG-C4 句式)+ explore-skill 措辞扩到 reason 头条 + 散文头条对齐总览 lead cell。判据=Subject==AggregateOnly 枚举+chain_relevance==on_chain typed,全软引导。
- **B[P1×2]**:对比 cell 目标症状 7.0="—" vs 树面 252.265ms 两面矛盾(cell 缺树覆盖行已有的 hop-only 回退);用户核心 delta 909ms 无任何 typed 面分解(唯一 delta 陈述在散文且归因背景段)。→P0-A(hop-only 回退)+对比 SG(delta 分解引导+共同口径)。
- **D**:①still_present 降级文案矛盾(降级 caveat"未评估/未消费"拼接模型散文"仍然存在"断言)→SPR #72 收尾小批 P1;②覆盖分子 5%(q1-B2 同族 witness)→P0-A;③quick-cell "trace causal node"占位名(§12-B7 repeat)/背景聚合折叠不一致(supply_pressure ×4 合并但 io/cpu_pressure 各两条未合)/系统补充"缺失成员"块"符号名称"表头错配 trace 指标行→P1-H;排除项(E19-E22 pid 尾在/双口径为设计)如实记录。

### §17 批次归口
SG-2 软引导批(A 双面+B delta 引导+C N11(b)+§16 R-recipe 黄金路径合车)/BLK 双向锁 P0 回归速修(§15.C,先行)/P0-A(B hop-only+D② 覆盖分子+§15.D gap③ 分母)/SPR 收尾(D①)/P1-H(D③族)。

### §16.1 SPL 实测结论(2026-07-06,../customlogs + 转录)
客户两份大 systrace 不在本机(客户 Windows D:\)。本机可测=两份 donghu 真机 trace(0 变体,标准形完备)+8 转录引用行。**三问**:①配对形态**本机不可判**(零结束行样本);②N 语义最优假设=**容器 ns pid**(begin 字符 B| 在客户采集链丢失)——四实例全 anco 容器进程,06-01 真机证该 vendor 链容器 marker pid=容器 ns pid,15/96/51/11 每进程一值小整数(depth/async 排除),**但每进程仅 1 样本,N 恒定性未验**;③普遍性=**特定采集/导出链形态**(非 OS/vendor 普遍),仅这两 trace 有,且独有 `print: 0x0:` 地址前缀+缺 begin 共现,指向不同导出器(trace_streamer 嫌疑)。
**决策**:T-span 根修**暂挂**(配对未证+样本不足,过不了精确信号红线,误映射产假 span 窗);待客户侧抽样(实测 agent 给了 PowerShell/git-bash 最小指令:变体计数+单线程 marker 流+line 140711 邻域)确证配对+N 恒定后再定,普遍则进 harmony flavor 闭集。**D-diag 先落**(不依赖语义)。fixture:入库 donghu 不含变体,需客户 trace line 140711±200 截取新 fixture。

### §16.2 T-span 解锁(2026-07-06,客户 line 140711 邻域抽样)
客户回传邻域一锤定音:转换工具(trace_streamer 嫌疑)**吃掉了 hitrace 标准 print mark 的前导动作字符 B|/E|(及 C|/S|/F|)**,留 `0x0:` 地址残迹。还原真值表(标准→变体):`B|15|setCoreSettings`→`0x0: 15|setCoreSettings`(Begin,pid 后有 name)/`E|15`→裸 `0x0: 15`(End,pid 后无字段)/`E|48|I38`→`0x0: 48|I38`(End,pid 后只 I-tag)/`B|48|H:validateDisplay|I38`→`0x0: 48|H:...|I38`(Begin)。**N 恒定性验证通过**(15 跨 setCoreSettings/bindApplication 恒定=容器 ns pid;15|setCoreSettings→裸 15 是 5µs 闭合活样本 pair)。begin/end 判据=pid 后"有无 name 字段"+尾字段"I-tag vs 纯数值(counter/async 防误判)",全精确结构信号→**可作硬解析门**(gate:`0x0:` 前缀+地址剥离后首字段纯数字,标准 trace 无此签名不误触发)。T-span 根修**解锁开工**(a2659e6619,只碰 parse.go,与 BLK 零文件重叠);fixture=line 140711 邻域 27 行(scratchpad q7_line140711_neighborhood.txt)。普遍性=该客户转换链特定形态(非 OS/vendor 普遍),无条件识别倾向(签名精确)或闭集化待批内定。

## §18 回访#7/#8(q8 doFrame + q9 双trace对比)四维归因(2026-07-07)

标本=customlogs/{cust_trace_q8_opendir.txt,cust_trace_q9_cmp.txt}。用户点名:①数字格式重试频繁;②对比场景降级。8-agent 工作流,维度A/D upheld,B/C 复核修正后入账(修正内容如下,原归因误述不采)。

### §18.A 数字格式重试(NUM,P0,真实 blob 复现)
- **P0-repeat** decimal→scalar 安全网(completionAggregateFactDecimalCountShouldBeScalar emit_investigation_complete.go:1141)被场景门 allowDecimalCountScalar(:1130,只开 HasExternalOnlyRuntimeArtifact||IsScalarAnswer||Intent==Trace)挡死 — q9 intent=root_cause 对比场景+trace 落 repo → 门关 → 浮点值 count fact 裸硬拒外泄。修=精确信号无条件改判(ParseFloat 成功∧ParseUint 失败∧members==0 → scalar_value,浮点进 count kind 任何语义下都是错的,不需场景门)。
- **P0-new** 拒绝文案无字段路由:parseAggregateCountValue(answer_aggregate_fact.go:5124)只说"put units in unit and keep value numeric",不说"改 kind=scalar_value" → 模型 grouped→bucket→total 打转 5+
## §18 回访#7/#8(q8 doFrame + q9 双trace对比)四维归因(2026-07-06→07;含复核修正,B-1 证伪)

标本=customlogs/{cust_trace_q8_opendir.txt,cust_trace_q9_cmp.txt}(blob trace_query fn id jz0bwsq8/oduik768vaq9)。用户点名:①数字格式重试 ②对比降级。四维归因经对抗复核,**两项被证伪/修正,不原样入账**。

### §18.A 数字格式重试(用户①,P0,upheld+真实 blob 复现)
emit_investigation_complete aggregate_facts 浮点度量(supply_pressure 101084.884ms/density 77.70)被 count-kind(grouped_count/bucket_count)硬拒。两根因:
- **根因1(场景门关安全网)**:decimal→scalar 自动纠偏 completionAggregateFactDecimalCountShouldBeScalar(emit_investigation_complete.go:1141)被 allowDecimalCountScalar 门(:1130-1139,仅 HasExternalOnlyRuntimeArtifact||IsScalarAnswer||Intent==Trace 开)关掉;q9 intent=root_cause、is_scalar_answer=false、trace 落 in-repo → 门关 → 裸硬拒。真实 blob 复现:"grouped_count \"CPU 0 调度竞争\" has non-integer count value \"10.503ms\"" → 模型把 ms 挪进 reason 散文才过。**+ members 守卫(:1145 members>0 跳过)使带成员浮点 fact 也失效**。修向=放宽门(精确信号:ParseFloat 成功∧ParseUint 失败∧len(Members)==0 无条件改判 scalar)。
- **根因2(拒绝消息无字段路由)**:parseAggregateCountValue(answer_aggregate_fact.go:5124)浮点分支文案"non-integer count value; keep value numeric"不告诉模型改 kind=scalar_value → 模型 grouped→bucket→total_count 反复打转(blob round 4/5/6)。修向=Atoi 失败但 ParseFloat 成功时给字段路由导向。schema 描述(:100/:115)已正确(非 R2' 缺陷),缺的是运行期拒绝复述。

### §18.B 对比降级(用户②)— **B-1 证伪,真凶=B-2 chain_required**
- **B-1 member_set support_ref 门 = 证伪**:归因描述 pre-6f9b7987(2026-06-25)旧码;post-6f9b7987 runtime-only trace 线程名成员的 support_ref 豁免**可达且触发**(HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext synthetic clone,request_traits.go:1356/1515 → PerfBundle 无 ResolvedFiles → IsExternalSource → allowed_optional),两线程名+最坏路由实测门放行无降级。**不立修复项**。
- **B-2 chain_required 一次性降级门(CONFIRMED,P1 非 P0)**:top_sleep 行 chain_required=true(stateDrilldownNeedsWakeupChainForSource query.go:4955,FragmentCount>=4 窄窗)与 state_churn 兄弟行 chain_required=false 跨 call 并存;wakeupChainDrilldownPendingDowngrade(emit_investigation_complete.go:3695-3733)只扫 chain_required=true 不 reconcile 兄弟 false 行,降级文案只点线程名无窗口归因。MarkCompletionGateOneShot 一次性(烧一轮重试非死循环→P1)。修向=(thread,state) 对 reconcile 兄弟 state_churn false 行 + 降级文案补窗口归因。

### §18.C 对比总览退化(现象真,两处定性修正)
- lead 全降背景(Finding#1,既有机制 upheld 不修):对比场景所有 typed rank 根因(supply/cpu/io_pressure)IsAggregateMetric→runtimeTraceProjNodeDemotedToBackground → primary 桶空 → RN-3(a) OnChainFallback 退 runnable/sleep 症状,fallbackNote"rank 候选均降背景"(runtime_tree.go:4407)。aggregate-only 主导时诚实退化=正确。
- 散文头条 supply_pressure(P0,现象真+**交付链定性修正**):typed 对比 cell 已剥 AggregateMetric cause(runtime.go:1261),散文侧无等价门。**修正:非 reason 透传** — runtimeObservationOnly lane 下 model reason 被 answer_document_evaluator.go:3595 整段省略;真凶=finalizer 自由散文合成无 AggregateOnly-as-headline 软引导(defaults.go 三 SG 无此禁令)。修向=SG-2 finalizer 头条禁令(§17-A 同族,复用)。
- 目标症状"—"(P1,§17-B,**裁定张力修正**):cell msCell(TargetSymptomMS)排除 Role==CausalHop hop-view 行→hop-only 目标返 0→dash;树覆盖行用 HopOnlyTargetSleepMS 呈关系句。**修正:非"无裁定纯回退"** — TargetSymptomMS F1 裁定(runtime_tree.go:4608)故意排除 hop-view 墙钟防双计;把 hop-only sleep 塞症状时长 cell 与 F1 冲突,须先解"hop-only sleep 是否=症状时长"裁定张力(不可当纯确定性回退直接补)。

### §18.D 逐行(upheld witness)
- fanout 冗余(P1):对比场景 3 投调单元 fanout,第1/2路同标签并行各对两 trace 重复 trace_query x2 + checkpoint"继续上次调查"叠加 → emit 次数异常多。→LANE 收敛评估。
- q8 持有者 over-claim(P0):tid 42067(NetworkKit_AssetsUtil_Operate_0)phantom 不在 trace,散文把唤醒者 #RxComputationT 当持有者 + "属于 NetworkKit 组件在 RxJava 执行"组件归属推断当确证。→PROJ-HOLDER/SG(推断披露)。注:引擎面 wakeup-edge 回退持有者身份是 BLK/P0-E2a 已修车道,此标本 BLK 前;组件归属 over-claim 是散文层新项。
- repeat witness:supply_pressure 跨线程累计当对比主指标(§17-A)/covered 94% 分母退整窗(§15.D gap③)/E5 名册±N 并列(§12 Q4-H)/系统补充"符号名称"表头错配(§17-D)。

### §18 批次归口
NUM 数字格式批(§18.A 两根因,emit_investigation_complete.go+answer_aggregate_fact.go,先行 P0)/CHAIN-RECONCILE(§18.B B-2,chain_required reconcile+文案,P1)/SG-2(§18.C 散文头条禁令[§17-A 合并]+q8 持有者组件归属披露)/P0-A+RCX²(§18.C 目标症状需先解 F1 裁定张力/covered 分母/名册)/LANE(fanout 收敛评估)。

### §18.A.1 NUM 落地裁定补记(2026-07-07,防按原处方"纠偏"回退)
原处方(§18.A"无条件改判、不需场景门")落地时**保留一个例外**:IsCountQuestion==true(真整数计数问题)仍硬拒浮点 count 值 — 朝保守方向偏离,理由:①该 predicate 是既有硬门 typed carve-out 先例(gate/hard_gate.go:142);②analyzer 自洽门强制 is_count_question→is_scalar_answer,HEAD 旧门经 IsScalarAnswer 分支使计数硬拒**从未真实可达** — NUM 改后计数车道首次真实收紧、非计数车道放宽,是加固非放宽;③纵深防御:即使门失效,确定性计数证明门独立 DOWNGRADE 兜底。复核无 P0/P1/P2;两 nit 就地收(拒绝文案 kind 重复渲染/嵌单位数值"10.503ms"形态一跳路由)。

### §18.E 持有者解析梯子三级裁定(用户 2026-07-07,pin)

`(owner tid: XXX)` 匹配模式合理保留不泛化(再确认)。XXX 可能是容器外(跨 pidns)tid,解析梯子定为三级,逐级回落:
1. **payload 直解**(现有):XXX 在本 trace 线程表(tid-presence 四字段集)→ contention_payload;
2. **ns-span 打点推导**(新增,本裁定):查不到时先尽量从 trace 内 span 打点推导宿主身份 — 每条 trace_mark 携带 (SpanPID=容器 ns id ↔ 行头宿主 tid/tgid) 发射对(T-span 已分离存储),全 trace 扫描建 ns→宿主映射;owner tid 命中且 SpanPID→宿主映射**结构唯一**(精确信号,不涉 comm)→ 推导宿主身份(typed 来源=ns_span_derivation,置信介于直解与唤醒兜底之间)。**comm 只作软旁证**(用户 2026-07-07 修正:comm 动态可变+15 字符截断,同 BLK-2 lockOwnerCommMatches 教训):payload owner 线程名与推导宿主 comm 一致=加分/不一致=不否决(标"名不符,comm 可能已变"降档披露);绝不因 comm 不匹配拒绝 SpanPID 唯一命中的推导,也不靠 comm 匹配消解 SpanPID 多义(多义落第 3 级);映射多义/无命中 → 落第 3 级;
3. **最后唤醒兜底**(现有,语义收窄确认):借助**等锁 span 结束的那次唤醒**(closing wakeup=放锁唤醒,非窗内任意唤醒)推定(wakeup_edge,0.62)。
实施=LCK-OWNER 批(待 LCK 覆盖面归因回带可行性实测:映射密度/唯一性/现 resolveCounterpartViaWakeupEdge 是否锚定 closing wakeup)。

**§18.E 增补(用户 2026-07-07,线程级纠正)**:第②级产出**不是进程级封顶**,分形态升线程级(实测印证:xxx_all.systrace `owner tid: 62020` ≠ 发射 ns-pid 60194 ≠ ns-tgid → 62020 是容器**线程** ns-tid,行头宿主 tid=59566/com.baidu.tieba,进程内线程列表同 trace 在场):
- **②a span 自报 ns-tid**:若存在线程打印自身 ns-tid 的 span(文本 tid 字段+行头宿主 tid)→ (ns-tid↔宿主 tid) **线程级**映射样本;
- **②b 主线程特例(零成本精确)**:pidns 线程 1:1,owner ns-tid==ns-tgid ⇒ owner=该进程主线程 ⇒ 宿主 tid==tgid(host 主线程),线程级;
- **②c ns-tid→宿主 tid**:owner ns-tid≠tgid 时,在 ② 推导出的宿主进程(tgid)的线程集里对 ns-tid(需 ns-tid↔host-tid 映射源:②a 自报,或内核 pid 映射事件如有);对不上 → 进程级 + ③补线程。
- **②×③ 融合(新)**:③ closing-wakeup 的 waker 本就是**宿主线程**(sched_wakeup 行头 comm-tid,线程级无歧义);做交叉 — waker ∈ ② 推导进程 ⇒ 两级互证,waker 极可能=持有者线程本身(线程级+置信升档);waker ∉ 该进程 ⇒ 披露分歧(可能中介唤醒)。
结论:②的封顶是 **SpanPID 车道**(marker pid=ns 进程 id)的上限,非第②级全部;线程级由 ②a/②b/②c/②×③ 供给,③ 始终线程级兜底。LCK-2 实施按此分形态,披露标级(线程级/进程级)。

### §18.E.1 comm 交叉核验降级修正(用户 2026-07-07,pin)

comm 可经 prctl 动态改名(payload 记持锁时刻名,sched 面可为改名前后另
## §19 LCK 锁形态覆盖面归因(2026-07-07;§18.E 三级梯子可行性实测含)

**头条结论**:真实语料锁谱系窄 — futex/pthread/rwlock/条件变量/内核锁**文本形态全语料 0 例**(两条真实采集链均未开这些 tracepoint,按不写投机覆盖红线**不立案**,payload-less 车道兜着等 witness)。真洞在已认族内部与词表噪声:
- **S
## §19 LCK 锁形态覆盖面归因(2026-07-07,颠覆性结论)

**头条:真实语料里锁等待谱系比预想窄得多。** futex/pthread mutex/rwlock/条件变量/内核 mutex/rtmutex/binder node lock 的文本形态在**全语料(donghu 1.9MB=customlogs/xxx_all 同 trace + carved fixture + q1-q9 转录)0 实例** — tracepoint 两条真实采集链都没开。按"不写 speculative 覆盖"红线**无立案资格**,留 isBlockingLikeText payload-less 车道兜,等真实 witness 再议。原设想 LCK 覆盖面大批 = 大部分空。

**现状清点**:parseLockContentionPayload(lock_contention.go:51)恰认 2 前缀 — ①`monitor contention with owner <o>(tid) at <sig> waiters=N`(monitor_contention,提 owner comm+tid+holder_site+waiters)②`Lock contention on <subj> (owner tid: N)`(subj=="a monitor lock"→monitor_contention 否则 lock_contention;**subj 文本被丢弃未入 wait_object**)。BlockingKind 仅 2 值。语料分布:donghu `Lock contention on` ×84(suspend count 58/InternTable 25/ClassLinker 7/thread list 2),`monitor contention with owner` 仓内 0 例(只活客户标本 q4 pin/q7 转录)。

**真正的高频漏判=已认族内部哨兵死角(P0,客户案 100% 必经)**:
- `owner tid: 0`(ART 无主哨兵)**23/84** → resolveBlockingSpanRowCounterpart 两分支(query.go:12558 要 PID>0 / :12583 要 Kind=="")全跳,无 HolderSource/无 wakeup-edge 兜底/无 wait_object;
- `owner tid: 18446744073709551615`(uint64-1)**7/84** → lock_contention.go:108 strconv.Atoi 静默钳 MaxInt64 且吞错 → 打印"owner tid 9223372036854775807 不在 trace"垃圾数字,"无主"错述"幽灵线程";
- owner-bearing 54 行宿主侧全 phantom → E2a 兜底 100% 必经,非边角。

**词表噪声穿透 typed 车道(§1 红线实证)**:isBlockingLikeText(query.go:12717)10 自由子串;`io` 命中 animation/TimerIteration/全 Audio 家族(纯DSP)、`sync` 命中全 VSync、`lock` 命中 AudioRunningLock 记账 — 纯 CPU span 领 type=blocking_span conf 0.72 + E2a 0.62 推断对端。爆炸半径受 top-8-by-duration + rank BlockingKind≠"" 闸限,但 critical_blocking 面穿透实况。

**梯子②可行性(§18.E,comm 修正后)**:donghu 1680 发射对/17 SpanPID **结构唯一 17/17**(每 SpanPID 恰 1 宿主 tgid;ns 分歧 6 条含 60194→59566 673 样本);**SpanPID 是进程级**(60194 由 15 不同宿主 tid 发射)→ ②硬产出上限=宿主 tgid,线程级推不出。comm 可得率仓内 0%(全 tid-only)。42067 案=②进程级+③线程级**并列双披露**(③给 #RxComputationT 与 payload owner NetworkKit_* 名不符=裁定②降档披露标准案例,BLK-2 twin-port referent 分离已备显示面)。closing-wakeup:findWakeupForWithSelection(query.go:13764)覆写无 early-exit+尾 5µs 窗=**已是 closing 语义,无需收窄**,只缺 pin。TraceSpanSummary 不带 SpanPID(types.go:1689)→ ②接线补字段或 contention 族走 Thread.TGID,批内定。进程级 peer typed-pair 张力:不许塞 tgid 进 PID,首批走 peer 保 unresolved+进程级身份走 display note。

### §19 批次(重排)
- **LCK-1(P0 精修,升入回访收尾序列)**:S1 哨兵值收编(ParseUint,0 与 uint64-max→typed ownerless 不产垃圾数字;无主 contention 接 payload-less 兜底链,subj 文本入 wait_object[现被丢弃])+ F1 词表止噪(删 `io`/`sync` 加边界,IO 有 io_latency/blocked_reason/d_state 自有 typed 车道不需 span 名兜)+ closing-wakeup 取尾 pin。零 registry/NKR 变更,客户案直接受益。
- **LCK-2(梯子②,ns_span_derivation)**:Index sync.Once ns→宿主 tgid memo(第二 tgid→Ambiguous 硬拒落③,comm 不消歧);resolveBlockingSpanRowCounterpart 三级梯子;holder_source/peer_source 新值 ns_span_derivation(既有 string 键零新键,但 sourceIsInferred query.go:12102 单值比较改集合成员+drill_status 第三常量+skill 披露文案扩);置信 0.67(0.72/0.62 之间);comm 只软披露不进置信算术。进程级 peer typed-pair 首批走 display note。
- **LCK-3(软尾,低优先/待 witness)**:S4 blocked_reason caller 锁定性词(down/down_interruptible/rwsem_/rt_mutex_/__mutex_lock→定性词 IO 改锁,lane/token 不动)/S3 fence 等待(Waiting for Present Fence/WaitFence)/S2 futex span 对(FutexWait/FutexWake,与 closing-wakeup 同义佐证);S2/S3 上新 BlockingKind 才动 registry。
- pin:closing-wakeup 取尾/无主兜底接线/uint64-max typed ownerless(永不 MaxInt64)/ns 映射多义硬拒(comm 不参与)/词表止噪(Audio/animation 不 blocking-like,FutexWait/fence 仍准入)/SpanPID 进程级(carved 多线程单 tgid)。

## §20 RKC:root_cause_rank 折算口径与富信息消费审计(2026-07-07,五镜头全 upheld)

**头条(镜头A/E 双证,P0)**:running 孪生行(RootEvidence twin)整体绕过三代折算 — 同一 WakeupCausalImpact 铸 2~3 行(CausalImpacts 行+RootEvidence 孪生+aggregate),孪生 struct(types.go:2627)无 fold/窗口/状态字段,排序键=raw DominantImpactMs 58.919,同池压过自身 gated 反转行 37.410(+57% 通胀自我失效 R5d 校准),且一段双铸 co-primary、StartTs=0 逃逸 Q4-D 降级门等一切交叉核验。**fold 数值已在 Summary 散文(:13580-13585)= 零新解析即可 typed 化**。修=去双发布(一段一行,BLK §15.C 同型先例),归并行排序口径见裁定①。
- **A-Gap③(P1)**:非反转 running 同样双发布且两 raw 口径互异(TotalMs 含 sleep vs DominantImpactMs 仅 running),同线程最多 2N+1 行同池。
- **A-Gap④(P2)**:Score 三套硬编码权重(chain ×2.0/aggregate ×2.05/孪生 ×1.0·conf0.75),同段两行 score 与 effective 排序反向撕裂,均发布。§12.3-1② score 裁定只落了承自重算半边。
- **B/C 面**:VS-1 periodic 零值权威✓/runnable 全额契约一致✓/IO 实测(Q4-B 后)✓;RunnableTop 反转 retype 后 Score/registry 断言不重算(P2)。
- **D 面**:gated/LockDominated/P0-E1 排序三段基线全在位✓;blocking_span 无 typeWeight 条目→default 0.8 低于 generic trace_span 0.9(P2,裁定②);锁富信息(subject_state_*/subject_chain_*/waiters/wait_object)只经 twin-port 条件性到 rank(P2);gated note 无任何消费者確认(F2 记录成立)。
- **E 面**:aggregate 反转车道无 gated 口径(P1,R5d 只落 per-occurrence 半场);聚合指标行经嘈声窗口重叠升 adjacent tier(P2,§17 降背景只有 tool 半场,一行精确信号修);LockDominated 旗未入 note(P2→P0-A);DrillStatus 只 stamp 锁车道,binder/io rank 行不对称(P3);nearest_chain_* 四 note 发布即坠投影零解析(P3→P0-A);adjacent/background tier 内按 Score 排(P3 观察不立案)。

### §20 归批
P0-E 引擎批(主归口,最终 scope):去双发布+归并口径(裁定①)+孪生 fold typed 化+aggregate gated 补齐+聚合 adjacent 升级禁+retype Score 重算+Score 权重单源(裁定②)+gated note 接线+DrillStatus 对称+runnable 独立行+E4/E7 typed 同段身份(RCX² 退档)+SFD note 接线提升;P0-A 显示尾:LockDominated 旗 note+nearest_chain_* 消费。

### §20.1 用户裁定(2026-07-07,pin)
裁定①=**甲**:去双发布后,反转候选段的 rank 排序/Score 走 gated(runnable 全额+running 折算);raw 保留在 CumulativeImpactMs/状态字段+显示面三口径各带尺子;非反转 running 段仍 raw 参赛(§7.10 第四分支真实工作量)。丙(按 ideal/deficit 参赛)违 §7.10 红线永久排除。裁定②=blocking_span(已解析锁竞争)typeWeight=**1.35** 对齐 priority_inversion 家族(同一决定性证据类);未解析对端 blocking_span 仍低权。附带设计决定:§15.B 原"runnable 独立行"项**撤销** — 甲案下归并行的 gated 构成已含 runnable 全额分量,再发独立行=重造双计。

### §20.2 用户裁定(2026-07-07):非反转 running 归因口径=可消除缺口(推翻 §20.1甲 side-clause)
全局原则(用户 2026-07-07 前条)= 根因排序与因果投影树里"能算作影响的永远是折算后能消除的那部分(deficit),不是折算后应占用的(ideal),也不是 raw 墙钟。非反转 running 不是例外(§20.1甲 留 raw 参赛的 side-clause 与本原则矛盾,推翻)。
**更合理算法(用户委托设计,采纳)**:非反转 on-chain running 段的**归因影响**(排序键/EffectiveImpactMs/树有效归因)= `SupplyFoldDeficitMs`(可消除缺口),非 DominantImpactMs(raw)、非 ideal:
- deficit 可算:impact=deficit;deficit=0(满频满核真实工作量,§7.10 第四分支)→ 归因≈0 → 不作根因,仅链上上下文显示;
- raw 墙钟保留为**显示事实**(cumulative/树 bar 链上占用)不进归因(显示≠归因;目标端等待总量仍由链解释,每节点可归因贡献=自身可消除量);
- 频点缺失→§7.10 不伪造→deficit 折 0→行仍显示(raw+"未折算·墙钟上界"披露)但归因≈0,**不让未折算 raw 驱动排序**(嘈声信号红线)。
反转段已符合(gated 的 running 分量=GatedRunningDeficitMS by construction)。实施=P0-E 吸收轮(复核+deficit-vs-ideal 专项在途,镜头0 边界呈报正被本裁定解决):非反转 running effective 从 DominantImpactMs 改 SupplyFoldDeficitMs + 频点缺失 fallback + 未折算披露;pin=非反转满频 running(deficit=0)归因≈0/弱核 running 归因=deficit/频点缺失 raw 不驱动排序三形态。

### §18.E.1 LCK-2 设计补充:②×③ 融合的身份归一声明(用户洞察 2026-07-07,pin)
opendir 案散文"owner 与 holder 均指向同一 tid 42067"**未必是模型混淆** — comm 动态改名(已裁定 comm=软信号)下,NetworkKit_AssetsUtil_Operate_0(争锁时刻 comm)与 #RxComputationT(closing-wake 时刻 comm)可为同一物理线程。若梯子② 从 closing waker 自身的 span 打点推导出其容器 ns tid == payload owner tid(42067),则 owner==holder==同一线程是**两独立信号交叉印证的可推导事实**。
**LCK-2 新增产出=typed 身份归一声明**:②×③ 融合命中(waker 的 ns 身份推导值 == owner ns-tid)→ 发布 typed 归一("owner(ns tid X)=宿主线程 H,依据:ns-span 推导×closing-wakeup 交叉",置信高于单信号;comm 不符**预期内**=改名,软披露不否决);显示面随之呈现**单一身份**(不再 owner phantom + holder 两条),散文的归一断言从模型猜测升级为系统事实;gap#3(节点计数混乱)随归一自然消解。融合不命中(推导 ns-tid ≠ owner tid)→ 保持两身份 + 分歧披露(中介唤醒可能)。

## §21 回访 cmp_01(双trace对比,新构建)四维审计(2026-07-07,全 upheld)

**生效确认**:对比 cell 口径标注/披露句/逐分量尺子/锁 rank1 带持有点/runnable 成因行/RN-13 平铺披露/still_present 文案自洽(SPR witness)全部客户侧生效。glyph 全"?"判定=客户端 cp936 代码页 best-fit,非我方输出(ASCII fallback 评估 P1-H 候选)。

- **D-新P0 排队深度方向反转**:对比总览背景压力"≈平均排队深度"=×N 跨查询窗合并求和分子 ÷ 单锚窗分母 → 7.0=336.7 vs 6.0=449.3(6.0 更高)而工具真值 57.43 vs 43.49(7.0 更高)— 旗舰对比面与全篇散文结论相反。→CWD 批(分子分母窗基对齐)。
- **D-repeat P0 覆盖句窗基错配**:"目标睡眠 115.902ms">窗口 101ms 同段矛盾,94%=跨窗分子÷锚窗分母,伪造 6.534ms 残差。→CWD 批同族。
- **A① lead 语义回退缺失(P1)**:6.0 平铺 lane JIT 83.893ms 占窗83%(确定性优化点)却 lead="未定位到链上主根因";lead 三级梯队对 Kind=semantic 结构性排除。→LEAD-SEM(P0-A 显示,前置=A④ ⚠实际0.000 假标量修)。
- **A③ 双重缺口(P1)**:(a)对比 SG 无"两侧同口径链下钻"条款(reliable sequence 止于 root_cause_rank);(b)完成门 haveWakeupFamily **全局不分工件** — 7.0 的链豁免 6.0 欠账,one-shot 零消耗;投影层已有 per-partition WakeupChainRecommendedNotRun 旗未消费。→(a)SG-2b;(b)CHAIN-SCOPE(per-artifact 分桶或直接消费分区旗,one-shot 带工件后缀)。
- **C 17>16 cap(P1×2)**:①schema 未预告 cap+拒绝无合并路由(NUM 根因2 孪生);②**optional lane 潜伏 bug:cap 溢出静默清空全部 17 条 facts 且 emit 成功**(数据无声丢失,静态审出)。→NUM-2。
- **repeat witness**:E7/E8 同段双行 runnable-dominant 版(RCX² 退档三证)/N3 相位披露缺(6.0 死窗 100ms 无链 vs 7.0 有链 94%)/系统补充"符号名称"表头/pid=3900 零值占位行/E29 承自注无窗基/引用 4→1 attrition(QCE#61 族疑似)。

### §21 归批
**CWD**(P0,先行):排队深度分子分母窗基对齐+覆盖句窗基+承自注窗基(显示半)/跨窗聚合 overlap→MAX(引擎半);**EMIT-2**=NUM-2(cap 预告+路由+静默清空修+自愈评估)+CHAIN-SCOPE(同文件);**SG-2b**:对比同口径链下钻条款;**RNB+LEAD-SEM**(显示批):runnable 显示行+同段折叠(三证)+lead 语义回退+⚠0.000 前置;LCK-2 队列不变。

## §22 回访 huadong_01(滑动=q1 berlin 复跑,构建含 RCX²/SFD 不含 B1)四维审计(2026-07-07,全 upheld)

**用户两问答案**:"跟踪span"=类型词非名称(typelabels.go:94),真名 H:ReceiveVsync(引擎/工具面/投影三层都有名 node.SpanName,三显示面消费门锁死 semantic-only 全丢);原始值现仅证据索引 E21 行号区间可查,dump 40 行 cap 被 112 条 state_drilldown(order 18)整体挤出+span_name 不在准入表=双闸不可见。"trace_gap"=链下钻数据盲区诊断记号(query.go:13627),裸 token 泄漏+错标候选根因;**用户措辞裁定:显示词=数据盲区,行内披露=窗内无调度数据·链止**。

- **B/D-P0 CHAIN-PATH(B1-would-NOT-fix)**:path 记录多分支森林扁平化+nil-impact 中转 depth=0 使 path 终点错位到中段(VSyncGenerator);B1 选举谓词(path 末端==用户实体)本形态结构性不可满足——**q1 复跑验收判据作废,立 B1-b 批**(选举改沿 path 任意位置匹配用户实体或修 path 终点记录;用户实体在"省略26节点"折叠中段还需强制展开位 D-P2)。
- **A-P0 PTV7-SPN**:span 名五面丢失(树行 tree.go:2874 门 Kind==Semantic/明细表 runtime.go:2316/无损块 5473 同门;E21 predicate=root_cause_tertiary 永进不了门)+dump per-lane 无保底配额 P1+AuditDetail 无 span_name 且 72-rune 截断切 token P2+明细表折叠行 EN 兜底"trace causal node"(C39 漏面)P2+零值行卫生(trace_gap zh 词条+去候选根因 chip+去 0.000ms 条;明细表 — 反而诚实,树表两口径对齐)P3。顺带消化 §10-A2 wait_object 修向(a)。
- **B-P1 coverage"目标"指代**:锚 VSync 非用户 42591,与横幅自相矛盾;B-P2 分子 depth-1 同主体 max-only(E14+E15 不重叠两 occurrence,聚合已发布 5.335 分子只取 4.431)。
- **CWD 族两新 witness**:E19 跨查询窗 ×14 求和 63.831ms 对锚窗 101ms 画 63% bar(树行/时长条面);C7 window_stats 来源 rank 行无 actual_* note→无窗口基准标注→窗口1 行静默投影进窗口2 锚定树。→并入 CWD 验收。
- **C-P1 散文标量零守护(系统性)**:trace exclude-source 场景 0 引用→normalize/quote 守护家族整体旁路,无散文标量-证据面比对门;46.821/48.216(引擎 actual 字段真值但三层裁剪后不可核验)/1.59ms(疑 wakeup_chain 逐节点 running= 字段)/sector 明细/io"两窗各8次"双倍计数/"持有栅栏"(把线程名 Acquire Fence 读成栅栏对象,L4 BODY-vs-evidence 强 witness)。→设计项 PSG(散文标量 grounding)裁定候选+dump 配额扩容(PTV7-SPN F3)分摊。
- **D-P1 下一步针对性塌陷**:headline binder_wait rank1 undrilled 无 next-step 点名;RCX① drill-debt 用户面半场无交付痕迹。
- **D-P2/P3**:E1 合并行三数不可调和(actual 取单成员无注/0.166 vs 0.100+0.134 对不上账);E14/E15 同线程同状态 L1 两行违 ×N 合并承诺;"2/4 调查完成"阶段槽位与调查单元语义相撞;对齐窗只披露合计无 per-state 分解;弱模型自动选择报告体零披露(产品裁定候选)。
- **已立案复现登记**:E4/E5+E11/E13 同段双行(RNB 四证)。

### §22 归批
**B1-b**(P0,CWD 后最优先):path 任意位匹配选举+终点记录修+折叠强制展开;**PTV7-SPN**:span 名三面入行+审计摘要 span=+dump per-lane 配额+trace_gap 措辞(用户裁定)+C39 漏面+零值卫生;**CWD 验收扩**:E19/C7 两 witness;**NXT**(P1):undrilled headline 的 next-step 点名(drill-debt 用户面半场);**PSG** 裁定候选(散文标量 grounding,非本轮);E1/E14E15 合并行账目→PTV7-SPN 或 P1-H。

PTV7-SPN A2 评估(2026-07-07):§10-A2 修向(a)(发布 wait_object=span.Name)与 F1 helper **不共享**——(a) 是产端发射面(tracequery/query.go + tool 观测发射,需 NKR 新键 wait_object,属 P0-E 引擎批前置),F1 是显示端对既有 span_name 键的消费;本批红线不触发射逻辑,故只评估不实施,A2 留 P0-E。

§21 EMIT-2 复核补记(2026-07-07):principal 独超 16 保硬拒适用于 compat 门与非 optional lane;optional lane 例外=截 16 优于 legacy 整体清空,带披露 note,已 pin(emit2_test.go OptionalAggregateFactsCapOverflowNotWiped)。复核 SHIP 4/4 突变咬红。

## §21.1 CWD 批收账(2026-07-07,复核 SHIP-WITH-FIXES→已修出厂)
交付:引擎 ×N 第五式"跨窗取最大"(union>MAX>SUM 三态互斥,MergedSumMS 无损,typed 端点判定重叠;不相交/同窗/无窗身份字节保 SUM)+显示密度分母 7 分支同窗基(多窗合并无可解析基不出密度)+覆盖句矛盾硬门(hopSleep>WindowMS 必换披露形态,6.534 伪残差禁出厂)+承自注窗基+N3 不对称披露注。标本方向翻正 7.0=92.1>6.0=32.7(工具真值对齐,旧码 249.4 vs 453.8 反向)。复核 4/4 突变咬红;出厂前修复复核 F1=F3 注措辞矛盾(密度基≠投影窗时不再claims"投影窗长不等",改具名窗基,兼治行内基不可复算)。
**CWD-2 立账**:①E19 树行 %-面(§22 witness,跨窗合并行对锚窗 bar 63%——不相交窗合法 SUM,坏在 %-面;分支5 模板迁移,typed key=MergedCount>1∧多窗);②C7 产端半场(tracequery 给 window_stats 来源 rank 行发 selected_window/actual_* note,显示端 absence-never-guesses 已就位);③symptom 分母车道(tree.go:4654-4688)跨窗分子未设门;④chain 窗共识要求 ≥1 带窗 chain 行(防无窗分子挂未证实窗基);⑤混合形态 union 无区间成员全值入总量(N2 既有 fail-open,非本批引入);⑥重叠判定无下限(0.5ms 重叠即 MAX,保守可接受);⑦测试名 FailsOpenToSum 语义漂移(cosmetic)。

## §22.1 B1-b 批收账(2026-07-07,复核 SHIP-WITH-FIXES→已修出厂)
交付:选举谓词末端→**path 任意位置**匹配用户实体(comparator/三源/光标排除原样),命中处截断锚定(取最后出现;位置0不可当选);tie 三级裁定 pin=①typed Subject 命中>位置命中(链目标即用户实体为最强候选)②同类取匹配位置最深(截断后 trunk 最长;"影响最大"因 path 无 typed 标量弃用)③终极=发布序;产端 traceQueryWakeupChainPath 在 target 最后出现处截断(3 显示/LLM 面,rank 零波及,edge 逐条记录不截);F2=WakeupPathUserEntityHits typed 载体(截断后计算)+折叠段用户实体强制展开(有数据带量值/无数据"用户关注线程(中转)"具名行)。15 pin,复核 6/6 突变咬红,huadong 深层行 L32-34 实证保留,掉落后缀有数据节点保 depthless 席位已固化仓内 pin。
**已知良性残余(勿当回归)**:真上游 nil-impact 段序列化在 target 最后出现之后时被截(legacy=整树错根,截断=根正确+节点保席位,严格占优);全用户实体折叠段≥6节点↺时 cycle 注无处挂(纯显示极边角);仅位置命中当选时 trunk 中段仍可能含跨分支工件(P0-E 引擎根修前已知形态)。
**P0-E 续批立账**:CHAIN-PATH 引擎根修=按真实分支发布 path/ChainNode 真实递归 depth/树 attach 键加链域(假 L32/33/34 深度标签消除);验收含 ../customlogs huadong 案复放。

## §22.2 PTV7-SPN 批收账(2026-07-07,复核 SHIP)
交付:F1 span 名三面入行(共享 helper,SpanName 非空布尔;semantic 门只余宽度差;huadong E21 形态→`oney.hmn.berlin-42591 · H:ReceiveVsync(跟踪span)`)+F2 审计摘要 span= part+96-rune part 边界截断+F3 dump per-order 桶保底(10 桶×4=40 恰平衡,结构 pin TestSupplementQuotaOrderUniverseBalancesCap 复核后补钉:第 11 个 Order 值/抬 floor 必回此重裁措辞;order-0 默认桶入场即排除)+span_name= 准入+trace_span priority+F4 C39 明细表漏面+F5 trace_gap="数据盲区"/"窗内无调度数据·链止"/图例(用户措辞逐字)+Diagnostic lane 去 chip 去零值条。13+3 pin,复核 9 突变咬红(两批合计)。
**观察项留账**:F2 截断对自含" · "的 span 名可能切进值内(外观级);A2 wait_object 修向(a)=产端发射面,与 F1 显示消费不共享,归 P0-E 前置。

## §19.1 LCK-2 批收账(2026-07-07,复核 SHIP,含 skill 半场补交)
交付:梯子② ns-span 推导(ns_span_derivation.go:发射对建图+结构唯一硬门第二 host id 即 Ambiguous 硬拒、comm 永不消歧;②a 自报 ns-tid 线程级 fail-closed 提取器/②b 主线程特例需 tidPresent/②c 进程级降档显式披露 tgid 永不入 Peer.PID)+②×③ 融合归一 typed 声明(holder_ns_unification,0.70)+分歧披露不推翻+comm 软披露永不否决;①"(owner tid: N)"判定块零 hunk(用户红线);置信严格序 0.72>0.70>0.67>0.62;R2' 六处同步(holder_ns_unification/holder_host_process);skill SG-A2 追加两句(wakeup-edge 原句字节不动,9-substring pin)。12+2+1 pin。
**残余立账**:融合 0.70 为本批选值(账本仅 pin"高于单信号",如需改数值一点改);②a 真实语料 0 witness(xxx_all 84 个 tid 字段全 owner 他报,提取器故意收窄,真 witness 现身后扩形);②a 样本 vs ②b 结构推断优先序未 pin(观察>推断,可辩护);TraceSpanSummary 新增 span_pid JSON 键(omitempty);真实 62020 案复放落②进程级+③互证设计形态(§18.E 标本 xxx_all 实证)。

### §22.2.1 措辞裁定补记(用户 2026-07-07):trace 专用名词不翻译
"跟踪span"→"trace span"(typelabels case trace_span);原则=专用名词保持原文,不过度翻译(trace/span 均专名)。连锁:明细表 22-rune 预算下复合词 "H:ReceiveVsync(trace span)"=26 rune 超限→NodeSubjectCell 增超预算裸名臂(丢类型词括注保真名,截半复合词严格劣于裸名;无损块/树行 36 限仍带完整复合词),随 RNB 批 runtime.go 落地。此原则复查其他 zh 词条:数据盲区/调度压力(需求积压)等非专名翻译不受影响;workqueue/irq 类既有词条未裁定不动。
**§22.2.1 终裁(用户确认 2026-07-07)**:词条语言尺子=有业界通行中文标准译名且工程师日常用中文的保中文(中断/核间中断/工作队列/调度延迟等);专名/要 grep trace 原文的保英文(trace/span/DMA fence/JIT/cpuset/VSync/binder/调度态五词)。兜底已实证:无损块"类型"行=runtimeTraceCausalProjectionRawTypeToken(typelabels.go:423)单 helper 返原始 wire token 永不本地化,且第一分支与"有 zh 显示词"同门——凡被翻译显示的 token 原文必在类型行(supply_pressure 审计保真先例的结构性推广)。本轮仅 trace_span 改词,其余词条维持;后续新增 zh 词条按此尺子,重提先读本节。

## §21.2 RNB+LEAD-SEM 批收账(2026-07-07,复核 SHIP-WITH-FIXES→收尾四项已落,17fd6a80)
交付:R1 runnable 显式成因子行(⧖ 全额·就绪排队积压·gated 分量不重复计入排序,typed 非零门,display-only)+R2 同段双车道折叠(SFD join 键+annotation-only 转移+守卫五臂:跨窗镜像/×N/歧义/effective 等式/**cumulative 等式(复核 W-A 补闸)**;覆盖分子与 bar 尺折叠前后字节等价 pin;depthless 披露 peer-aware(W-B))+L1 ⚠跨窗无值记号(假 0.000 灭)+L2 lead 第4级语义回退(固定形文案/禁"主根因:"/LeadKey 不认领 pin/C00 份额抑制臂 pin)+L3 背景段非空校验。16+4 pin,7 突变咬红。
**重要预期修正(回访复放对照用)**:cmp_01 E7/E8 的 rank cum=47.503(含外围链 scope)≠chain cum=28.230——cum 等式闸下该对**双行保留是设计终态**(不同账目绝不折),勿当回归;折叠正例=opendir E6/E7(58.919==58.919)/huadong E4/E5(4.115==4.115)/E11/E13(2.770==2.770)。
**残余留账**:W-A 单侧零角(rank cum>0∧chain cum==0 可折,真实语料 cum 恒随 impact 发布无 witness);SelfRows 循环同款 peer-aware 未动(无 witness,同款一行改);非反转 running 双发布与 raw-vs-gated 旧形态设计性不折,归 P0-E 引擎去双发布。

## §22.3 NXT 批收账(2026-07-07,§22 D-P1 / RCX① §12.3-1① 用户面 next-step 半场)
交付:N1 undrilled-headline 点名行(runtime.go next-step lane;typed 双臂门=lead 树行 Kind depthless∧Edge chain_unresolved——trunked 限定,flat 形态归 flat header+RN-13(b)——∨typed UndrillableReason;lead=runtimeTraceProjLeadSelect 单源(与结论行/对比 primary cell 同面,永不点错节点),semantic lane 排除,聚合指标/on-chain fold/未解析主体不点名;三要素=subject(+cause 词+rank=N+多工件 ArtifactLabel)/为什么(深度未解析——与树边用户面同词)/怎么做(wakeup_chain / critical_blocking_calls,零内部名))+N2 floor 席位(裁定:普通形态=基 cap=4 内**置换**,点名行先于全部通用 lane,让位者=尾部通用模板(huadong 形→点名+2口径+1通用);对比家族(#69 全集发射不可置换)占满基 cap 时=**cap+1 扩展**;硬上限 4+1+2=7)。9 pin(verbatim 正向/drilled 负向(fixture 含 E17 形非 headline depthless 兄弟行,咬死 key-等值弃除与全放宽两类 mutant)/own-IO 边负向(Edge=own lead 不得冒称深度未解析,咬 Edge-only mutant)/flat 负向/floor/视图名+零行话/EN/missing_wakeup 臂/未解析主体让位)+5 突变咬红(N1 门禁用→4 pin 红;N2 floor 禁用→floor pin 红;门三类 mutant=弃 key 等值/全放宽/弃 Edge 判据→各自负向 pin 红,对抗复核收尾补钉)。
**N3 留账(一句话)**:RCX① 头部披露半场(投影头部/结论行对 depth-unresolved headline 的强制披露)在 tree.go=CWD-2 并行域,本批不动——结论行既有 ⊘ 臂仅覆盖 sleep∧Undrillable,深度未解析形态的头部披露留给 CWD-2/后续批;本批的 next-step 点名行即用户面披露的 next-step 半场。

## §23 DCS 动态编译 span 三义务审计+裁定(2026-07-07,用户规则符合性)
审计结论(HEAD e99dbf46):义务①窗口投影 rank typed 车道 HOLDS+两残口(H1 跨窗界 span 整条丢弃非裁剪、零告警;H2 观测通道原始时长对锚窗打 %——cmp_01 E2 span 全在锚窗外却显示 83%);义务②排序**零链窗口结构性破**(唯一根因:语义 span primary/co-primary 准入 100% 耦合唤醒链 on-chain 硬前置+零链时聚合 35% 窗帽不生效(28914ms 跨线程累计与墙钟同通道对赛)+rank 12 席无语义保留席(引擎 TraceSpans 16 席不对称);关键事实:83.893ms JIT 宿主=com.huawei.hwid 非目标进程,"目标自身准入"救不回);义务③ SPLIT(C4 系统块 HOLDS 含对比模式;散文提及 BROKEN=无 obligation 机制+skill 只有"禁提升为根因"半句无"以优化点身份提及"另半句)。
**🔴 用户裁定(2026-07-07):rank 准入=保留席+独立 tier**——rank 12 席内给语义编译 span 保留席,以独立 tier 词(deterministic_optimization)入榜参与排序,不与 primary 选举竞争;不碰 on-chain 硬前置与 §7.5 聚合裁定。义务③经保留席+提及义务闭环。
**DCS 修复批 scope**:E1=保留席+tier 词(tracequery rank builder+容量);E2=F2a 铸造 fall-through(链存在无重叠不降级泛型+重审 PID 门);E3=F2d fail-loud(有已分类语义 span 而 rank 0 语义行→typed caveat);E4=H1 跨窗界裁剪+caveat;E5=H2 显示重投影/标源窗(LEAD-SEM A④ 同车道);E6=F3a skill"以优化点(非根因)身份提及 top 项+占窗比"指令(过红线 checklist)+F3b 对比总览"确定性优化点"列+零链侧 lead 括注;附带=rank 内部退化链与观测通道 conf 口径分叉对齐。裁定点2(答案侧硬门"块存在⇒结论引用")=先软后硬,观察一轮回访再议。

### §23.1 DCS 裁定精化(用户 2026-07-07,取代 §23 首轮"保留席"粗粒度)
1. **rank 准入(保留席+deterministic_optimization tier)只给"窗内∧链上"的编译相关语义 span**(链上判据=既有链节点/impact 窗重叠谓词;窗内=窗口裁剪 impact>0)。
2. **非链上的窗内编译 span 不入链上 tier**——与窗口内背景影响(聚合行等)进入同一背景综合排序;口径警示:墙钟 span 与跨线程累计 cpu·ms 不可裸同通道对赛(§7.30 S1 先例),批内设计须给可辩护的排序基并逐行带口径词;零链聚合 35% 窗帽问题不动(毗邻 §7.5,未裁定)。
3. **提及义务只给链上编译 span**(无条件入正文提及,F3a/F3b scope 收窄到链上);非链上**不设提及义务,除非窗口内影响综合排序靠前**——"靠前"精确界=进入背景综合排序已发布行的 TOP 3(typed 榜位比较;默认值,用户可调)。
4. cmp_01 E2(83.893ms,宿主 com.huawei.hwid,零链窗)按此裁定=非链上道:入背景综合排序,提及与否取决于其榜位;显示层 LEAD-SEM 语义回退(lead 空手时点名最大语义 span)不受影响,保留。
**DCS 批 scope 修订**:E1=窗内∧链上保留席+tier 词;E1b=非链上窗内编译 span 入背景综合排序(可辩护排序基+口径词);E6=提及义务按链上/榜位双门(typed);E2/E3/E4/E5(fall-through/fail-loud/H1 裁剪/H2 显示)不变。
**EVOLUTION(§29.7-2,2026-07-10,SEM-LEAD 批实施,见 §29.22)**:本节①"不与 primary 选举竞争"半句已按用户裁定演化——on-chain 语义类行无条件全权参赛、可登顶(board/lead/❶❷❸ 全开,tier 词"确定性优化候选"身份保留);链上提及地板与非链上"背景综合排序+提及门 background_rank≤3"(本节②③)不变。

### §23.2 DCS 批 as-built(2026-07-08,全量交付)
**E1(保留席+tier)**:`RootCauseTierDeterministicOptimization`(types.go)= 窗内∧链上编译 span 的独立 tier;assignRootCauseRanksAndTiers 中该类行对 primary/secondary/tertiary **位次选举阶梯透明**(既不占位也不移位,负向 pin);**复核 F-4**:非链语义行同样阶梯透明(不占选举槽,tier=固定 tertiary 支撑带词)——零聚合退化板上非链 span 曾可占 slot0 冠 tier=primary,直接违 §23.1②;负向 pin+突变回放已咬红;co-primary 白名单删除 4 语义类(EVOLUTION RECORD 注,on-chain 硬前置一字未动);容量内保留席 `rootCauseSemanticOnChainReservedSeats=3`(truncateRootCauseRankItemsWithSemanticSeats:从尾部逐出最低非语义行,总数恒≤limit;引擎 16 席先例的 rank 面对称补齐)。tier 是 wire token:`tier=deterministic_optimization` 注 + `root_cause_deterministic_optimization` predicate(后者不匹配 root_cause_primary 前缀→显示 primary 桶天然排除,负向 pin);显示词 zh=确定优化/确定性优化点(与观测通道 typed 优先级词同族对齐;PTV8-RCR §24.1 将并入"确定性优化候选"类别词族)。
**E1b(背景综合排序基,本批设计决定)**:排序基=**占窗比**(每行以自身口径÷同一窗长:span=墙钟裁剪值/窗长;聚合=跨线程累计/窗长≈平均排队深度即显示密度归一;线程行=墙钟状态时长/窗长)——把各口径都化归"窗口当量倍数"这一唯一不伪造 ms 等价性的无量纲刻度,原始 Score(conf×权重通道)绝不跨口径对赛(S1 先例);同板窗长相消,故实现按 basis 值直接比较(placeNonChainSemanticSpanRows,**只重放语义行**,其余行相对位置字节稳定;无界窗=无基,Score 序原样,absence-never-guesses;adjacent 层不跨越)。零链聚合 35% 窗帽未动(§7.5)。**背景可见性席位=1**(`rootCauseSemanticBackgroundGuaranteedSeats`):witness 形(聚合+8×cpu_pressure+runnable 结构性填满 12 席)下任何纯排序基都救不回 0.83 窗当量的 span——无席则 §23.1 ③ 的榜位门在 witness 形永为死码;席位只保发布,榜位照占窗比实挣(cmp_01 形 e2e pin:background_rank>3→按裁定④不入散文,LEAD-SEM 显示回退仍点名)。榜位 typed 化:`BackgroundRank`(位次按全部非 on_chain 已发布行 1-based 计数——§23.1 二分道:adjacent+background+零链空白同板;**字段只盖在语义 span 行**,复核 F-2:与文本行/typed note 两输出面同门,非语义 trace 的 rank JSON payload 恢复字节稳定,control pin 已加)+ note key `background_rank`(registry causal_rank/soft_consumer,只在语义 span 行发)。
**E2(铸造 fall-through+PID 门)**:链存在无同线程节点/impact 窗重叠→窗口裁剪 typed 铸造(OnChain=false)不再降级泛型;零链分支 PID 门整体删除(cmp_01 E2=com.huawei.hwid 外进程 witness);**道别红线**=铸造期重叠谓词单点定道,enrich 的线程成员 on_chain 判定对铸造期非链语义行强制降 adjacent(rootCauseChainContextForItem 前置卫;huadong E21 adjacent 先例)。
**E3(fail-loud)**:semanticSpanRankFailLoudCaveat=精确计数比较(stats.TraceSpans 已分类数>0 ∧ 发布语义行==0→typed caveat 带双计数);只在 build 面发(enrich 只增行+席位保证使计数单调,不会由真变假)。
**E4(H1 全场修复,非半场)**:computeTraceMarks B/E+S/F 配对改全流(窗口过滤只保留 C| counter);铸后 clipTraceMarkSpanToQueryWindow 窗裁剪,TraceSpanSummary 新增 Actual{Start,End}Ts/ActualDurationMs(零值=未裁剪的精确信号),raw≡窗内不变量对全部消费方保持;线窗查询按行号重叠准入、时长不按行裁;不完整配对(语义词面 B/S 悬挂)→caveat 点名≤3(裸 E 无名不报);观测通道随发 actual_impact_ms/actual_window 注(显示 ⚠实际Xms 升级)。波及评估:全 76 包绿,唯一消费方 query.go stats 构建一处。
**E5(H2 显示源窗)**:产端=语义观测行加 selected_window note(=stats.Window,与 CWD-2 rank 行同键同格式;anchor 双门白名单不含 trace_semantic_span→不可能重锚);显示端=独立函数 runtimeTraceProjSemanticSourceWindowShareBaseMS(语义行∧typed 源窗∧源窗≠denom(±1ms F-2)→%基=源窗+行内「来自查询窗 X」tag+新 mark SemanticSourceWindowShare 图例条目;同窗/无源窗字节不变 control pin);占比词单源化 runtimeTraceProjSemanticSpanShareText(占窗N%/占其查询窗N% 二形),conclusion/compare cell/F3b 列三消费方同源。cmp_01 E2 形修正:83.893ms 83%→9%+来自查询窗注。tree.go 改动全部走独立函数(PTV8-RCR 摩擦最小化)。
**E6(提及义务双门)**:F3a=skill TRACE SEMANTIC SPAN ROOT CAUSES 条目重写(过红线 checklist;链上 tier 行→结论散文必须以优化点(非根因)身份提及最大项+占窗比;非链上 background_rank≤3 才提;与禁提升为根因指令互补,收口旧"unless"尾句);tool description 三句同步(co-primary 列表除名语义类)。F3b=对比总览"确定性优化点"列(≥1 工件有数据行才出列,防空列扩表;cell=top 语义 span+占比,选择器与 LEAD-SEM 同源 runtimeTraceProjSemanticTopSpan)+零链侧主根因 cell 括注"(存在确定性优化点: <名> Xms 占窗N%,见优化点列)"(仅 primary==nil 未定位分支;LEAD-SEM 语义回退 lane 早退不叠写,负向 pin)。
**验收**:make+go test ./... 全仓绿;gofmt 本批触碰文件零输出(仓内既有 79 文件为本地 gofmt 版本噪音,HEAD 即有,未动);pin 22+(engine 14 case+display 7+tool 3+skill 1,含既有 2 pin EVOLUTION RECORD 改写;复核收尾 +F-1 跨窗 contention 正负/+F-4 零聚合退化负向/+F-2 JSON 稳定 control);突变自查 6/6 咬红(席位禁用/道别谓词反转/E3 计数禁用/F3b 列禁用/E5 车道禁用/F-4 选举占位恢复)。**测试文件**:tracequery/dynamic_compile_span_dcs_test.go、tool/answer_document_mutation_runtime_dcs_test.go、tool/trace_query_dcs_test.go;revisit76 哨兵新 mark+fixture 照章注册。残余:①E6 散文义务是 prompt 软引导(裁定点2 答案侧硬门按 §23 先软后硬观察回访);②zh"确定性优化候选"类别词族终形随 PTV8-RCR(§24.1 已记);③E4 线窗查询的跨线窗对不裁时长(时长物理真值,已注释);④F-1 lock 车道 actual_* 搬运漏项**已修**(复核收尾:CriticalBlockingCandidate 三 actual 字段+carve/fold/rank item 搬运+cand/rank summary 双基披露(window-clipped; actual_span=…)+critical_blocking 观测 actual_impact_ms/actual_window 注;跨窗界 contention pin 正向+全窗内零值 control);⑤F-3 `background_rank` 的 soft_consumer 载体分类学随裁定点2 硬门议题一并复审(若加答案侧硬门则升 hard_consumer);⑥F-5 E5 源窗重基逻辑双实现(tree % 车道 runtimeTraceProjSemanticSourceWindowShareBaseMS 与占比词 runtimeTraceProjSemanticSpanShareText 的源窗判定各自成对)——PTV8-RCR 节点重构时合一。

## §24 回访 opendir_02(当日构建复跑,2026-07-07)+PTV8-RCR 显示重设计裁定
**生效确认**:锁主根因 rank1(E4 blocking_span 112.223ms/94% 覆盖+持有点)/身份归一(owner 42067 双名并列)/RNB runnable 子行+同段折叠/PTV7 dump 每序列保底("共65条,仅列40条:每序列保底后按序补足")/持有者 next-step 点名(条3)全部客户侧在场。
**用户裁定(UX 重设计,PTV8-RCR)**:①折叠方向反了——成因行身份=根因排序参赛身份:能进根因排序的都是"成因"行,下面再跟拆解;反转成因节点=「⚙ 优先级反转候选(runnable+running)」,节点下独立分行展示 runnable/running 各自原始时长与折算后时长;②纯 running 成因节点按大核满频折算影响时长入榜(引擎 §20.2 已是,显示对齐);③"同段rank行并入"中英混用不合格——两车道原生合一节点,徽章升头,该注整句消失;rank=N→根因排序#N;④"混乱无比"=全面 UX 复审令:所有"因果投影"展示描述(树/图例/量表/无损块/证据索引/指标快照/下一步/树头覆盖句)逐条再审,语言客户化、精简清晰。终形 mock 已确认(有效归因 37.410ms = runnable 20.713ms(全额)+running 折算 16.697ms(按下游消费核折算);机制构成长句退役由"="分解行+状态子行取代)。
**排批**:PTV8-UXA(全面措辞/排版审计,读侧先行)→PTV8-RCR(节点重构+措辞终形,含 gated 清除/"="形/同段注退役,吸收原措辞批)→DCS(§23.1)→P0-E。

### §24.1 PTV8-RCR 节点文法终稿(用户参考稿 2026-07-07+统一化收敛,已确认方向)
四行式统一文法(所有成因节点同构):行1=⚙ 状态构成+bar+窗口投影+%+⚠+[E#+E#];行2=成因类别·根因排序#N·置信;行3=有效归因 V = 状态(口径) a [+ 状态(口径) b];行4+=子行「状态 原始 raw → 计入 x(口径,机制词)」。恒等式 pin:Σ计入==有效归因==行3 右侧各项;行1 值==窗口投影(与 bar 同基)。退化规则:单状态且计入==原始→省略行3/子行(允许退化不允许变体)。口径词闭集三词:全额/折算,按下游消费核/折算,按大核满频,下界。用户例2 子行"供给折算缺口"句式并入口径括注(单一子行文法);7.702 笔误按恒等式纠为 17.702。类别词族:优先级反转候选/算力供给候选/IO阻塞候选/锁竞争·持锁…(与 rank tier 对齐,DCS 的 deterministic_optimization→"确定性优化候选"入族)。
**§24.1 补:口径词图例条目(用户问"下界"何意,2026-07-07)**——行内保留"下界",图例增:"`下界` = 保守最小值:频率数据缺失的片段计 0,折算未计大核单周期优势;真实可消除量只多不少"。随 PTV8-RCR 落;口径词闭集三词的每一词都须有对应图例条目(全额/按下游消费核/按大核满频,下界),自解释度普查并入 UXA 改造表消费。
**§24.2 事件类成因节点模板(用户问 IO 类如何设计,2026-07-07,四行文法同构变体)**:行1 词位二选一(状态构成|类型词,glyph 按既有记号),×N 上移行1,行尾形态词撤;行2 恒定=类别·根因排序#N·置信("候选根因"空话 chip 全树退役);行3="有效归因 V = 单次最大(a–b,共N次)"(口径词闭集扩四词:全额/折算,按下游消费核/折算,按大核满频,下界/单次最大(共N次),各配图例);子行=影响点清单(与状态拆解子行同位语义:支撑账目的分解明细);单次且计入==原始退化为两行(行3 并入行2 尾)。心智模型:行1 是什么多大/行2 身份榜位可信度/行3 算了多少怎么算/子行拆开看。
**§24.3 glyph 按影响形态映射(用户裁定 2026-07-07:成因 glyph 对应影响形态设计,不要亮色)**:闭集提案=⚙运行占用·算力供给/☾睡眠/⛓IO阻塞族(IO延迟等typed事件归族,不再挂◦)/⧖就绪排队/⊗锁竞争持锁/⇅优先级反转/✦确定性优化/↯中断活动族/◌数据盲区/◦无形态兜底;🎯(全树唯一彩色emoji)→◎;❶❷❸/⚠单色保留。三硬规则:①text-presentation禁VS16+等宽单格宽+三端一致(逐glyph渲染验证,v3红线);②glyph表与行2类别词族=同一typed表两列单源生成禁手工双同步;③GBK退化另案(P1-H ASCII fallback评估,cp936属客户端)。随PTV8-RCR落地。

## §24.4 PTV8-RCR-A 批收账(2026-07-08,复核 SHIP-WITH-FIXES→六项收尾已落)
交付:四行文法全量落地(成因节点=类别·根因排序#N·置信/有效归因"="分解(共享模板三面单源,复核 M7 实锤后归一)/原始→计入拆解子行/事件类 ×N+单次最大/退化形含 F4 单分量门)+恒等式机器 pin(Σ计入==有效归因==引擎 effective,不平衡拒渲绝不造数)+五项退役(同段rank行并入/机制构成长句(反转节点)/候选根因 chip 全树/用户面 gated/旧 runnable 子行)+glyph 影响形态闭集换装(单源两列表;树根=⊚ U+229A,复核查出 ◎ EAW-Ambiguous 后换字;单格 pin 含 EA 语境断言)+口径词四词图例强制(含"下界"解释条,FAIL-1 抑制臂补发)+hop 伪残差修(§15.D 抖动臂,opendir"6% 残差"灭)+E5 重基双实现合一。opendir_02 E4-E8 对照渲染=用户处方形态 verbatim。8/8 突变咬红。
**残余立账**:①⛓/❶/◇/▒ 四批前记号同为 EAW-Ambiguous(EA 终端双宽风险,重裁另立,P1-H 候选);②抑制形 mark 只在明细块面(bidirectional 清单不能收该形 fixture,探针页级化待议);③单分量 running(折算) 怪形保四行(负向 pin 钉界);④B 批承接=UXA 121 条词表横扫+F5 族 on-chain 混用+F6 类过期注释清扫。

## §24.5 PTV8-RCR-B 批收账(2026-07-08,UXA 121 条改造表全量消费)
**交付(词表三族全局替换 B1)**:窗族终词=分析窗/查询窗/用户请求窗/数据实际覆盖(关注窗口/分析窗口/投影窗(口)/锚窗/用户窗口/选定窗/实际对齐窗/观测跨度/聚焦子窗/重叠窗 全退役,树头"满格=窗口Xms"带数值句照 D-verify 保留);归因族=on-chain 中英混用全文退役(链上已归因/需结合链上证据/不计入链上已归因),"未归因残差"→"未归因","(链上)归因口径合计"→"各链上口径合计",有效归因循环定义重写(D#29 修正稿,不引已退役 gated 词);根因族=根因关注点TOP3→根因排序前三,总览列 工件/rank=1/on-chain已归因/投影窗→trace 文件/根因排序#1/链上已归因/分析窗,demoted 位置注 (rank=N)→(根因排序#N),候选影响→未分类(该行无具体状态/类型词)(候选影响 词全面退役,ptv6d 双向 pin 随迁);伴随=目标等待/目标睡眠→关注线程等待/睡眠(B#3-verify 成族),本轮/本批→本报告。
**B2 图例**:☾/◦数据行/❶❷❸/状态标签缩写/⊘链止(两面同词 sched_wakeup 括注)/⚠实际(分析窗)/链上L#(层数,与明细层级行一致)/时长条两分支拆条(新 mark BarScaleFallback)/×N取最大(canonical 墙钟跨线程不可加和,三面同词)/占窗>100%(≤3% 带宽保留)/整窗等待/◇(唤醒链同词)/▒/已归因未归因(互相包含解释)/累计(跨线程)+折算拆条(新 mark StanzaDiscount)/无类型词行 全部按改造表终稿;新增 `有效归因` 图例条(新 mark EffectiveAttributionTag,A#31);组头"(按出现频次)/(按树内出现顺序)"删(漏审B);记号组状态图标语义族先行再频次(layout-②);树读法头句 E#(+N) 教学前移+去"不是额外推测"(A#6)。
**B3 逐点**:blocking_span→持锁阻塞(typelabels,lead=持锁阻塞（blocking_span）,类型行保 token);missing_wakeup→无唤醒记录(UXA A#22-verify/D#7 收敛终词;无词自身行=无唤醒记录·⊘链止 A#30 补主语稿,有词行字节不变);影响点位禁裸调度态 token(精确状态词表命中,明细影响点行保全量 roster);原因名车道裸态 token 走 PTV7 alias 合并形 sleep（s_sleep）(B#14-verify);组合名后缀词界截断(runtimeTraceProjBoundaryTruncate:持锁阻塞… 保 A批 E4 形,s_s… 灭,首词装不下整词让位 A#24/漏审A);睡眠症状→查上游 改门=树内已渲染上游(含根)则不挂(A#25);持有点签名感知压缩 类.方法(文件:行) 保 40 格(A#29,块/审计面全签名无损);supplyfold 三句终稿(A#26/27/28,频点→CPU 频率);覆盖句全部 bullet 化+"。 "间隙灭(AL5/DL4);覆盖分子句→"未计入上句已归因数值"(A#3/D#17);本投影锚定其一→本因果树基于其中之一(A#5)+树头"(按查询窗分组)"括注删(D#16 半场:跨组件同门不可达,留账);关键指标(量表退役 B#1)+各列口径+10 行图例终稿+置信列(E# 单点 B#13)+双席/×N 行同词;明细(逐节点完整属性)+完整名称行删(S1)+因果位置单字段合并词表(B#16,自身行=关注线程自身 B#24)+关系全句化/影响点拆行(B#17,▸ 全退)+span 原文块末反引号(B#18)+×N 明细三式改写(B#19/20,自身 roster 抑制+共N列K)+E14 三面同名 其余 N 项合并(对端线程未解析)(B#22-verify,树面 roster 依 2026-07-03 客户裁定保留)+字节等价同名块合并 [E1] [E2](B#23)+层级链上臂 深度N→链上LN(B#27-verify);证据索引导语自解释+审计 token 七词图例句(C#1/2)+容量句修正稿(C#4-verify)+分组行 X–Y en-dash/详见尾并入定位/合成行分组态去 basename(C#5-8,coordinateTail 归一化耦合已修)+半角标点(DL3);指标快照 状态切换(state_churn) 标签(D#27 复用注册词)+两段分组(C#10 修正稿,主导保留/0% 括注删)+窗口基准 查询窗+数据实际覆盖 拆平(C#11/D#15,blob 面同改);下一步 条目级标签退役(C#12)+双窗对比句(C#14)+持有者句去工具语法保全签名(C#15 两 verify 交集)+反转/running/runnable 指引句+间距(C#16);系统补充 导语重写+单工件分组声明一次(C-L1,多工件保行内名)+配额句修正稿(C#20-verify)+claimKey 段级精确去重(C#18-verify)+type=/impact_ms= 重复注不占槽(C#19-verify 真修)+窗口基准=查询窗;对比总览三注出表格入正文(DL1)+闲置错配→就绪积压时核闲置(D#20-verify)+排队深度去重复(D#19)+唤醒链采样括注(D#18);平铺树头三句去——去工具语法(D#24);背景层两句(D#39);lead 回退括注 链上/根因排序候选(F5 收尾)。
**B4 排版**:非成因行注五段固定序(tag.Seg 精确发射位标注+稳定排序,MainRow/OwnLine 不动,D#36);从属行每行至多两注(AL6);折行禁则=闭括号标点不起行/开括号不收行,ASCII 括号/逗号独立成 atom+闭标点链循环下拉(DL2/复核 M6,字节拼接不变量保持);对比表注出格(DL1);DL3 半角统一仅落证据索引条目行("定位: %s; 审计: %s")与分组 详见 尾并入形,快照面本已半角、系统补充 blob 全角面未动(余点留账)。冲突消解补记:锚窗终词取 D-verify 分析窗(D#4/D#33 同族),A#32-verify 的"直接用查询窗"建议未采——该注对比的是两侧各自的锚定分析窗,非取数查询窗。
**B5 卫生**:F5 on-chain zh 混用全清(含 lead 回退括注两处);F6=死码 runtimeTraceCausalProjectionImpactMeaningCell 删除(唯一引用是 lint 白名单)+过期注释随手清。
**pin**:新文件 answer_document_projection_uxa_rcrb_test.go=三族"旧词禁出厂"全渲染面扫描负向 pin(6 fixture×20 禁词,含"重叠窗"整词)+图例终形 verbatim(19 条)+B3 重点正负(lead zh 标签/无唤醒记录两态/影响点抑制+块面全量/词界截断/睡眠指引两态/供给折算三句/五段序/≤2 注/折行禁则/分组定位/置信列/因果位置合并/关系全句/块合并/span 原文收尾);revisit76 哨兵照章(3 新 mark 注册+2 新 fixture,bar 探针因两分支共享 █ 改结构探针);既有 pin 全量演化随批(≈35 test 文件,EVOLUTION RECORD 引 UXA 条目)。
**留账(需裁定/另案)**:①C#15 持有点短签名提取(C-verify 逐字保留 vs D-verify 确定性截取,两 verify 相抵,本批只落交集=去工具语法+窗族);②D#30 top 片段/top_sleep(RN-12/R02 verbatim 禁动面,须用户重裁);③C#13 双窗嵌套出场门(触 #68 Q3 用户裁定,需带容差判据,先记后落);④D#16 完整同门化("(按查询窗分组)"承诺已删,快照分组门与树头共享谓词跨组件不可达,如需恢复承诺须建共享输入);⑤AL1 图例两层重构/AL3 深链缩进封顶/DL8 双席块多席合并/L2 表节点列去 (a–b)(排版重设计,非机械);⑥DL5/域A漏审C 记号 '?' 终端退化=P1-H ASCII fallback 另案;⑦E#(+N) 与 ×N 双计数对账注(B-L1 余项);⑧6 链上L#/链深# 两 verify 相抵,取 B#27 最小演化(明细并入树词 链上L#),链深# 未采;⑨C#21 尾部免责 finalizer 定式(软引导,可选未落)。
**复核收尾(SHIP-WITH-FIXES 七项,2026-07-08)**:①EN 用户请求窗行双转义字面 \n- 修+前缀 pin;②"本轮"漏网两处(RN-13(a) 平铺锚横幅/覆盖 caveat,后者顺带去"应按 trace_query 有界参数"工具语法)+负向 pin;③覆盖句 zh 斜杠形 X/N%→括号形 X(N%)(A#2 后半场,9 pin 随迁,EN 不动);④DL2 折行洞修(ASCII (/)/, 独立 atom+闭标点循环下拉),宽度全扫 pin+M6 单表突变咬红;⑤无唤醒记录 终词纠正(缺唤醒记录 为任务书笔误);⑥bar 两分支方向-B 探针重装(满格=窗口/满格=本报告最大);⑦留账⑦收账(E#(+N)/×N 两种口径互不换算从句)+两过期注释清+三 pin 缺口补(类型: missing_wakeup 无损行/持有点块面全签名/深缩进多注全存活)。

## §24.6 'top 片段'重裁(用户 2026-07-08,收 §24.5 留账②)
UXA D#30 终稿取代 RN-12 verbatim 冻结面:"top 片段"→"其中最大片段"(zh)/"its largest fragment"(en);裸 source token(top_sleep/state_drilldown)撤出散文,审计面(系统补充/证据索引)按 §22.2.1 兜底保留原文;source 在场仍作 note 出场门(typed 来历)。两发射形+7 pin 同批迁移(EVOLUTION RECORD 于 tree.go 发射点与测试头)。

## §25 PSG 散文标量 grounding(用户裁定 2026-07-08:方案 b+d 组合)
背景=§22 C-P1:trace exclude-source 场景 0 引用使 quote 守护家族整体旁路,散文数字/断言无门(46.821 真值不可核验/1.59 来源不明/"持有栅栏"纯编造/两窗各8次双倍计数四类实证)。
**裁定 scope**:b=答案侧确定性扫描散文 ms/% 标量,对证据面标量集合(aggregate_facts+系统补充观测值+投影数值)做成员检查,未命中→contract **软失败+重试提示**列出未命中数字(嘈声信号只驱软引导,红线合规;禁硬拒);d=容量侧扩留存(aggregate_facts 通道/dump 配额延伸,让引擎真值可核验);断言类编造(非数值)=skill 指令覆盖("散文数值必须证据面可查,查不到则引用视图+窗口或删数字"+"对象身份断言须有证据行支撑");裁定点2(优化点硬门)维持先软后硬待回访。

## §24.7 回访 opendir_78(含 PTV8 构建,2026-07-08)——文法半场性+三 gap 立案
**用户裁定(呈现逻辑统一令,重申)**:任何进入根因排序的行,无论挂在哪种树边(下钻/唤醒/成因)下,节点文法完全同构;状态构成词恒在行1 词位(链上行=glyph+线程名+·+状态构成);行2 身份/行3 有效归因"="/子行拆解,行序恒定无例外;同一(线程,类型)多实例合并后以合并量参赛(拆分参赛=弱化排序),真实区分键(如 inode)必须显示。
**现场三 gap**:①E4 链上行 `runnable+running · 链上L1` 漂到第6行(混合文法:新结构行+旧注车道);②E8 文法散架(行3=旧状态注,行4=旧 tag 形 `有效归因0.186ms` 无空格无"="无口径);③E5/E6 两条 块设备IO(inode) 全同属性并存未合并未显示区分键(1.136#3+0.462#8,合并 1.598 参赛榜位≥#3)。
**未点名疑点(自查上报)**:rank#1 持锁行方向翻转——同 span 原文(owner 42067@AssetManager.list)在 opendir_02=RxComputationT 持锁/目标等(waiters=1),opendir_78=目标持锁/LegoHandler 等(waiters=0);waiters=0 参赛 rank#1+目标自持锁对目标丢帧问题=相关性倒置;窗口微差(33872.279 vs .289 起点)下持有者归属不稳定,疑 DCS E4 跨窗界准入/lock carve/fold 胜出交互。→审计定唯一根因。
**§24.7.1 合并参赛规则(用户裁定 2026-07-08,gap③ 终裁)**:①真实区分键(inode 号等)显示层**不能丢**——树/表/明细面必须可见,**过多可折叠显示**(roster 截断带计数披露,"成员(共N,列M)"既有先例形,全量在明细/审计面);②同一(线程,类型)的多实例,**即便区分键不同(不同 inode)也合并为一个参赛者进根因排序**(合并量参赛,同线程求和合法)——此为通用规则,适用于全部可拆分参赛的 rank 类型族(block_io_by_inode/page_cache 等,io_latency 已合并为先例);显示形态=×N 行1 合并量+行2 身份(单榜位)+行3 单次最大或全额口径+子行/明细列区分键 roster(§24.2 影响点清单同位)。审计维度B 的产线图供实施;弱化排序普查结果全族同修。

## §24.8 回访 huadong_78(含 PTV8 构建,2026-07-08)——重要度分层省略总则+循环梯子病理
**用户裁定(排版总则)**:信息按重要程度分层省略——**重要信息永不省略**(可经下属子行/多行/换行承载);低重要度信息优先缩短(短图标即可传达的必须图标化/折叠);行宽压力下的省略顺序与重要度严格反序。witness="用户关注线程(中转) ↺"零量值拓扑占位行以完整长标签重复 7 次+逐级缩进吃掉 ~60 列,而 E7 载荷行(rank#9 反转,量值/身份/账目)被压进 ~20 字宽逐字竖裂("根因\n排序#9·置信高")——重要度完全倒挂。
**现场病理链**:B1-b F2 强制展开按"每次命中"执行 × 扁平化 ping-pong 循环(user⇄VSyncGenerator ×7 serialized)=14 行 60 列零信息梯子;根因=P0-E CHAIN-PATH 引擎病(扁平伪线性+假 L26/L27 深度)的显示面放大;显示侧可先做循环折叠(重复循环段折一行 ↺×N: A⇄B)+F2 展开一次规则+缩进封顶(AL3 留账项升级为 P0 witness)+注换行最小宽度地板。
**头部两疑点(审计定性)**:lead 主根因=目标自身 binder等待 4.577ms vs 树内 ❶rank#1 反转(散文称 18.643ms 聚合)背离;覆盖句"关注线程等待 0.011ms;各链上口径合计 4.431ms"配对可读性+68 条未计入。B1/B1-b 生效确认(⊚=用户线程)。

## §24.9 opendir_78 审计收账(2026-07-08,四维全 upheld)
**维度C 持锁翻转唯一根因(P0)**:lock fold 键**缺 waiter 身份**——两条物理不同的 contention span(目标等锁段+LegoHandler 等锁段)折成一行嵌合体,目标线程的受害行被整行吞除;P1=持有者按"最后释放者"整段归因(closing-wakeup 取尾把 115.944 全额记到接力唤醒方,payload 内 '-->' 移交链精确信号无守护);P1=方向盲区(目标自持锁=下游后果,现准入 rank#1+lead 加冕——**裁定点,勿直接改**,须与 §20.1②/Q4-A/§15.C 同席复审);P3=waiters=0 参赛合法(计的是发射时已排队的其他等待者,建议不设门);P2=推断级持有者 caveat 三面失显。→归 P0-E 引擎批(锁车道 fold 键+移交链守护同批)。
**维度D 两新 P0/P1**:D-1(P0)覆盖句分子消费 §20.1 反转覆写后的 cumulative→"已归因45%/未归因55%"对 ~97% 已解释等待伪造过半未归因(COV 批);D-2 "另有4条未计入"把症状自身 sleep 计入;D-4 animator 症状容器 span rank#12 以 95% bar 压过真因 45%;D-6 因果位置词表把 DLR lane 名当重要度(#3/#8/#11 全标"主根因(优先处理)",真因 E4 只标"链上(重点)"——叙事倒挂,措辞待裁);D-7 ❷徽章落 #11 行与图例"依有效归因前三"当面矛盾(badge 车道 vs rank 车道分叉);D-8 roster cap=3 静默截断;D-9 下一步自指循环;D-10 E8 ⚠实际2.641 与快照互斥(引擎 actual 口径,CSP#63 邻);D-11 表给离链行发布"链上累计"+单线程行贴跨线程口径词;D-12 索引行号尾巴位置。
**维度B gap③ 产生链**:引擎按 (dev,inode,PID) 铸行且 rank 全家无同(线程,类型)折叠——**7 族可拆分参赛**(io_latency 本标本即 6 席);R2 显示合并 min=3 放走 2 成员组且从不重排榜位;inode 只活在自由文本 Summary,全链无 typed 字段。→RCM 批(§24.7.1 规则实施:typed 键+引擎合并参赛+roster 显示,eval 敏感需代表性 sweep)。
**维度A 文法半场唯一根因**:行1 词位=未预留宽度的名字后缀(对照 C00 RowMainReserve 纪律),截断后由 guarantee/打包旧 lane 行尾重拼;§20.2 纯 running 缺口臂在结构化构建器缺失(第三口径词无生产者,E8 整类落回旧 tag);NoDeficit 措辞否认身旁精确归因数(阈下"无缺口"→"缺口仅 Vms 已计入");置信跨面分叉。→PTV8-RCR-C 显示批(与 huadong_78 审计的循环折叠/宽度预算合批)。
**归批图**:PTV8-RCR-C(显示:A 全部+D-5/6/7/11/12+huadong_78 显示面)/COV(D-1/D-2,P0)/RCM(B,§24.7.1)/P0-E(C 引擎+D-10)/裁定议程(方向盲区+D-6 措辞)。

## §25.1 PSG 批收账(2026-07-08,复核 SHIP-WITH-FIXES→收尾三项已落)
交付:ViolProseScalarUngrounded 软门(registry SoftByDefault 结构性禁硬拒+bus-scoped strict arm 一轮重试资格+双半场粘滞闩防活锁)/确定性 ms|% 标量提取(词边界+千分位 deny)对证据面标量集合成员检查(容差三臂+%重算三臂+两值和臂,宁松勿严)/未命中列表+改写路由进重试提示;P-d 评估=aggregate_facts 通道结构已够+dump floor4 洪泛保席(双实证 pin,不动策略);P-s skill 双句(数值可查性+线程名≠对象)。复核抓获**自 grounding 三洞**(模型自产 next_steps 保留块/runtime_trace_ lookalike 宽前缀/被拒草稿 attachments 回收——三 witness silent 实证)→收尾:证据喂入面收紧为系统确切拼写集(RuntimeTraceSystemBlockID 导出,免扫面松/喂证据面紧公理)+attachments 退出+三负向 pin+词边界 pin。11+5 pin,10 突变咬红。
**残余立账**:evidence_item Summary/TurnA facts=模型产但 §25 裁定明文纳入(sanctioned,如需收紧另裁);ledger 未展示 note 过门=b/d 分工设计;计数类("两窗各8次")只靠 P-s 软引导;比例/求和重算臂巧合掩护=宁松代价;aggregate actual_* note 白名单收编归 P1-H(审计 C1 已立案)。
**PSG-2 追加残余(第二臂绑定核对,2026-07-08)**:①值/窗长池不隔离——发布端点跨的派生窗长同时进成员 pool,散文可把窗长冒充量值引用而静默出厂(带异窗名的形仍被绑定臂抓;代码注释已 sanction 该松面);②时长区间伪窗——"0.5–2s" 类第二端点带秒单位的区间在无窗名词邻近时也解析为窗身份,可把正确陈述翻成 misbinding(软臂单轮有界;收紧候选=span 形加窗名词邻近门);③绑定核对的 text-unit 粒度=Text/Item 字段(段落级),超出"同句"scope——多段合一 Text 时句内窗/线程集合变宽,只向更松方向漂移(方向安全)。

## §24.10 回访 cmp_78_01(对比场景,含 DCS 构建,2026-07-08)——语义 span 族合计参赛裁定
**用户裁定**:同属一个语义类(VerifyClass/JIT/Shader=class_verification/jit_compile/shader_compile)的 span,细节(被 verify 的类名等)不同也**按投影合计进入排序**——§24.7.1 合并参赛规则扩展到 DCS 语义 span 族:合并键=(线程,语义类),参赛值=**窗口投影合计**(同线程求和墙钟合法;非单次最大——用户明示"投影合计"),roster=span 明细名单(可折叠,§24.7.1 修订);链上 tier 道与非链背景综合排序道同规。
**witness**:E27-E42 十六条 VerifyClass 同线程逐条成行(0.04-2.4ms,合计≈10.8ms 从未聚合),对比总览"确定性优化点"列只显示单条最大 2.424ms(占其查询窗3%)——family 量级被拆碎隐形;十六行纵向占版面同时违反 §24.8 重要度分层(与 huadong_78 梯子同族:低单值行海占版,合计信息缺席)。→并入 RCM 批(合并参赛)scope;显示=×16 一行+行3 合计口径+roster 折叠。

## §24.11 huadong_78 审计收账(2026-07-08,A/B/C upheld,D 复核流中断未验证)
**A(P0)循环梯子唯一根因**:foldSegments 段局部视角(per-hit split)无跨段 run-length 视图,重复二元组 (oney⇄VSync)×7 无合并通道;F2"每命中必展开"违反 §24.8(首次披露后 6 次=零信息占位);循环检测器只认 index-0 锚定全路径周期,mid-path 循环返回 (0,0) 整梯零 ×N 注;共享标签列 50-cell cap 击穿 20-cell 名字地板(深行身份蒸发成"◦ …",F2 自败);从属注 20-cell 竖裂(lead 无界增长,修=AL3 缩进封顶消源非抬地板);"用户关注线程(中转)"18 格长标签→「⊚中转」短记号(⊚已过 EAW)。→**PTV8-LAD 批**(run-length 循环折叠行「↺ 循环×7: A ⇄ B」+F2 首次一次+缩进封顶 12 级+名字 8-cell pid 尾地板+CJK 不可断 atom 表(根因排序#N/有效归因/下界))。引擎 ping-pong 本体=P0-E witness 登记(29 元素 path dump 入复放验收集)。
**B(P0 原则级)**:显示几何以深度为唯一布局输入与重要度解耦;名字预算可塌缩 1 cell("unreachable"注释被证伪);深缩进齐破 100-cell row cap 且静默;E9/E11/E5 无 % 列=typed 多窗抑制非宽度省略(防误归因澄清)。
**C**:C-1(P1 真bug)lead 选举与 ❶ 徽章读两套种群(primary 桶聚合前 cap=10+in-path class 排序把 rank#1 E9 逐出选举池,目标自身 binder 症状行捡漏加冕)→**LEAD 修**(选举种群=徽章同一 post-aggregation 榜);C-3(P1)覆盖句"等待0.011ms"=分母种群静默塌缩(仅 E2 入分母,binder/sleep 静默排除)+"各链上口径合计"名实不符(=depth-1 单行 MAX 非求和)→COV 批;C-2 E9/E11=§20 双发布 ×N 聚合形 witness(P0-E);C-4 置信跨面分叉(RCR-C 同 opendir_78);C-5 rank 序数跨榜碰撞(#1×2 等)chip 无榜身份=裁定点。
**D(未验证,PLAUSIBLE)**:F1 榜位命名空间碰撞/F2 树头四态行未入文法+binder 双发/F3 RN-12 跨窗伪包含(分子成员窗与合计窗不相交仍发布 59%)/F6 合并行三账复发/F7 E15/16 引擎聚合在而显示双发/F8 外窗行零窗披露——待 cmp_78_01 审计或复放交叉验证后定级。
**归批总图(更新)**:COV+LEAD(P0 小批先行)→PTV8-RCR-C(链上行文法)→PTV8-LAD(布局)→RCM(合并参赛 §24.7.1+§24.10)→P0-E(链+锁+双发布)→裁定呈报(方向盲区三 witness+序数榜身份+D-6 措辞)。

## §24.12 cmp_78_01 审计收账(2026-07-08,A/C/D upheld;B 流中断续跑中)
**维度A(RCM 语义族施工图,已交付)**:五跳全 per-span 定谳(铸造/rank/观测/投影桶(聚合豁免)/显示四面);**基准数更正 witness=7.124ms(同线程 ×14 并集合计,10.8 系跨线程误加弃用)**;三设计强制项:①family 载体禁复用 MergedCount/MergedMaxMS(lead 选择器把 ×N 一律折 MAX,合计当场失效)——新 typed 车道;②跨窗纪律=family 只在同 selected_window 内合并(RN-12 伪包含同型防双计),同线程嵌套/重叠段取区间并集(disjoint==求和,union<Σ 披露);③口径词闭集扩第五词"合计(共N段,同线程)"(图例必须紧邻 ×N取最大 条目说明"同线程可加;跨线程仍不可加和"消当面矛盾)。cap=16 恰被打满→family 合计只是下界(亦须披露);C4 块 family 分组;skill F3a 改 family 口径;E3 fail-loud 计数随 family。施工图全文在审计 output(RCM 批直接消费)。
**维度C 四新立案**:C5(P1)继承性有效归因缺口径词——8+6 条链上行 有效归因>窗口投影 无口径词,"承自归因"闸门=eff>10×cum 嘈声比值,1.1-1.8× 全漏(修=精确判据 eff>cum 即挂承自口径词);C6(P2)Depthless 行三面口径分叉(边"深度未解析" vs 注"链上L1" vs 明细"深度1(未接入链)");C7(P2)rank#1 lead 行明细自相矛盾(类型: binder_wait+影响形态: 未分类);C8(P2)数据盲区 rank#7 参赛零证据锚(trace_gap=Diagnostic lane 竟持榜位,无 E# 无索引)。C1=方向盲区第3/4 witness(**引擎原生 rank#1,LEAD 种群修不改本案**——裁定已熟);C2=树头四态行未入文法却持榜席+❶落#3/#2(D-F2 CONFIRMED);C3/C4=COV 新形态(已随批补充);C9=LAD 第二 witness(48 行语义行海,行2/行3 逐字重复×14);C10/C11 卫生捆。
**维度D**:P0=对比表症状 cell 双侧两口径两数量级(3.262 vs 470.071,旗舰面倒挂);P1=6.0 整窗分母把 493ms running 计成"未归因94%"(COV);P1=PSG witness(9 条唤醒延迟值 6/9 零成员资格——PSG 上线后将拦截);P1=语义明细"关系"字段把外宿主 span 伪绑定为关注线程(修=宿主如实)。
**归批增量**:RCR-C+=C5/C6/C7/C10/C11;RCM+=施工图+C8(Diagnostic lane 不参赛=分流遗漏);COV+=C3/C4 细化+D-P0 症状 cell 对比口径(需对称化或双侧同基披露);LAD+=C9;裁定呈报=方向盲区(4 witness 已熟)+❶徽章榜身份(C2/§24.11 C-5 同席)。

## §24.13 方向盲区+榜位身份双裁定(用户 2026-07-08)
**裁定一(症状降道)**:目标自身状态行(自身 binder 等待/自持锁 blocking_span/自身睡眠段,typed subject==分析目标)在"目标为何卡顿"问题中**不再竞争主根因/lead**——榜位照发但阶梯透明(deterministic_optimization 先例同型);lead 落到榜首**非自身**行(对端/上游);无非自身行时诚实回退("未定位到链上主根因")+症状披露。引擎(选举阶梯透明臂)+显示(lead/❶ 加冕)两层。四 witness=opendir_78 自持锁 rank#1/huadong_78 binder lead/cmp_78 双侧 binder 原生 rank#1。→**SYM 批**(LEAD 种群统一批落地后其上实施,先后依赖)。
**裁定二(榜位身份)**:❶❷❸ 徽章与 lead 消费同一 post-aggregation 榜(LEAD 批已在途);多查询窗多榜时序数 chip 带窗身份("根因排序#1·窗X")消同报告 #1×2 碰撞。→徽章半场并入 SYM/LEAD 落地面,窗标 chip 归 RCR-C 显示。

## §24.14 cmp_78_01 维度B/D 补账(续跑回收,2026-07-08,全 upheld)
**B 三 P0(对比面)**:B-1 症状 cell 双口径产生链=括注/回退闸门是存在性测试非主导性测试,主臂绕过 CWD 窗基共识(§24.12 D-P0 补链);B-2 主根因列双侧窗基 20× 不同源零披露(SG-2b 违反)+链上已归因列 MAX 冒名(COV C-4 对比面形);B-3 旗舰对比句 (值↔窗) 错绑("sleep 63.6%(1430ms)"把 2250ms 窗测量绑到 1800ms 窗名下)——**PSG 成员资格制拦不住的绑定类错误**。
**B/D P1-P2**:B-4/D-4 双侧锚窗构造不对称(6.0=用户宣称 884ms 的 1.86×,基线分母阶段纯度不对称零 reconcile;容差判据呈用户裁定);B-5/D-1 供给列 %窗基未点名(22× 误读);B-6 优化点列=两个 ~5% 覆盖切片对读+family 载体 7.124 已存在却落深度未解析折叠行(三面分裂);D-3 背景压力 cell 闭集类型闸吞 trace_span 背景行(总览"—" vs 树 stanza 91.940ms 当面矛盾);D-5 退役词"目标睡眠/目标症状时长"在 P0-A2 发射点幸存(负向 pin 20 禁词未含);D-6 系统补充 40 席被边界漂移孪生+整窗沉睡噪声占 12 席(段级去重缺 total= 漂移臂)。
**归批增量**:**COV-2**(对比 cell 层:B-1/B-2/B-5/D-1/D-3+§24.12 D-P0 症状 cell——四列复用背景列"窗基:"披露机制);**PSG-2**(第二臂=值↔窗/主体绑定核对,命中成员后核同句窗口线程 token 与证据行归属,仍软门);RCM+=B-6;RCR-C+=D-5;EMIT/P1-H+=D-6;**裁定待呈**:D-4 锚窗偏差披露容差判据。
**§24.14 补:D-4 容差裁定(用户 2026-07-08,按推荐默认)**——锚窗/分析窗长与用户宣称时长偏差 >±10% 即出 typed 披露句(形如"分析窗 1645ms,较你指定的 884ms 长 86%:窗口按数据边界对齐构造");≤±10% 静默;对比场景双侧各自判,任一侧披露则两侧同披露(对读基不同构必须可见)。判据=纯算术精确信号;归 COV-2 批。

## §24.15 COV+LEAD 批收账(2026-07-08,复核 SHIP-WITH-FIXES→收尾已落)
交付:L1=runtimeTraceProjRankBoard 共享榜(post-aggregation,lead 与 ❶ 同源,恒等按构造;huadong 形 lead=E9 反转行);C1=TargetImpactMS typed 通道(引擎"目标被阻塞分支墙钟"provenance 实证;note display_only→hard_consumer;五聚合车道 member-MAX 全 pin——复核抓四车道 pin 死角后表驱动补齐,MAX 置被吸收侧)+分子阶梯 typed-first+伪造残差禁出厂;C2=症状自计排除(闭集=TraceStateKindUniverse,自持锁照计);C3=分母 census 两臂 form-switch(crossBase=排除>0;非 crossBase=单项最大>入分母合计;全称括注 form fork);C4="合计"→"链上单项最大"五臂+EN。18 pin,8 突变咬红。opendir 形终态:"睡眠 115.353ms 中 112.175ms 已由链上解释"+计 1 条未计入。
**留账**:6.0 整窗分母 493ms running 计入未归因(COV-2)/D-P0 对比 cell(COV-2)/全榜 eff≤0 时 lead 有❶无(旧行为非回归)/overflow fold 丢 TargetImpactMS(有意保守,注释在)/lead"事实恒等"裁定注=fold 后 lead 消费合一节点面,SUM 永不出负向仍 pin。

## §24.16 SYM 批收账(2026-07-08,复核 SHIP-WITH-FIXES→三收尾已落)
交付(§24.13 裁定一实施):引擎=target_self_state tier(det-opt 同型 wire token)+SubjectIsAnalysisTarget typed 盖章(tid-first sameThreadRef 字面复用,absence never guesses,build+enrich 双点+enrich 全称 pin"目标 PID 行永不穿三级 tier")+选举跳过臂(不占槽/不 co-primary 促升/序数照发/语义臂优先);显示=统一榜过滤臂+RN-3(a) 回退车道滤臂(复核 F1:跳臂让位后自身行从回退道再加冕的 P0 通路,一行滤+witness pin)+fold tier 移植(禁折叠洗白)+全自身退化形诚实回退+症状披露(量程=lead 单实例口径,×N 取 MergedMaxMS/periodic 取折算——复核 F2 防 SUM 假标量)+词面(自身行=关注线程自身/自身状态,不再穿"主根因(优先处理)");对比 cell/LLM 面三处同步。21+2 pin,7/7 突变咬红。
**留账**:真 trace 复放随客户回访批验证 lead 终形;comm-lane 兜底=引擎既有语义未扩大;全自身形 LEAD-SEM 不可达=裁定内构造(pin 定格);窗身份 chip 归 RCR-C。

## §24.17 症状降道精化(用户裁定 2026-07-08,修订 §24.13 裁定一 scope)
**原则**:目标自身行分两族——①**等待症状族**(sleep 等唤醒/binder 等对端/自持锁阻塞他人):维持降道(§24.13/SYM 已落,根因在对端/上游/下游);②**自因可拆解族**(自身 runnable/running/IO/D-state):**以拆解后的成因身份进入根因排序参赛**——自身也是链上节点(depth 0,on-chain),这些状态的根因是系统性可动手项非对端:
- 自身 runnable → **调度压力候选**,影响时间**全额**;目标优先级低于 RT 且被抢占时补充披露"(优先级低于RT)"(typed 调度数据判定);
- 自身 running → **算力供给候选**("算力供给未拉满"),影响时间**折算后**(按大核满频,下界——§20.2 引擎语义已是,恢复其参赛资格即可);
- 自身 IO → **IO阻塞候选**,全额,带 IO 根因拆解(io_latency/block_io/inode 子行);
- 自身 D-state → **D状态候选**,全额,带 D 状态拆解(对端/设备)。
**实施(SYM-2 批)**:S1 跳过臂谓词收窄=subject==target ∧ 等待症状态族(sleep/binder_wait/blocking_span-自持);自因族不降道、正常占槽参赛、可加冕 lead("主根因: 关注线程调度压力(runnable 全额 X ms)"类形——actionable);board/RN-3(a)/fold 滤臂同步收窄;类别词/glyph 走 §24.3 既有族(⧖调度压力候选/⚙算力供给候选/⛓IO阻塞候选/D状态候选入族);四行文法照 §24.1/§24.2(行3 口径=全额/折算按大核满频,下界);SYM 全自身退化形回退保留(全为等待症状时)。witness 预期:cmp_78 双侧 lead 从"binder等待"变为该侧自因族或非自身榜首(binder 等对端仍降道)。排批=RCR-C 落地后立即(谓词收窄触 SYM 的 query.go+tree.go 滤臂)。

## §25.2 PSG-2 批收账(2026-07-08,复核 SHIP-WITH-FIXES→收尾已落)
交付:第二软臂=值↔窗/主体绑定核对(行粒度证据索引走既有四证据面零新通道;精确判据=三元组同证据行共现+全载行正向绑定且无一相合才断言;窗相合=端点等/载行子窗单向包含/窗长 1% 容差;线程按 tid 相等非名字拼写(MUT-2 pin 看守);百分比重算分母全窗且无一匹配才 raise;同 kind 同轮闩合流不加轮次)+skill 绑定句("数旁点名的窗/线程须等于发布行;跨窗先各自归一化")。cmp_78 B-3 旗舰形("1800ms 窗内 sleep 63.6%(1430ms)"实为 2250ms 窗)与 D-2 (b)(c) 形全 pin;(d) 形 tid any-match 放行=宁松设计内。8+2+1 pin,5 突变咬红。残余三行已入 §25.1 追加段。

## §26 CPU 算力折算裁定(用户 2026-07-08,CAP 批立案)
**规则**:running 折算必须计入核类算力差。厂商各核算力表为证据首选;**无算力表证据时采用默认比例粗算**(同频点):中核=小核×2.3,大核=中核×1.1(=小核×2.53),超大核=大核×1.2(=小核×3.036);恒序 超大核>大核>中核>小核。
**实施(CAP 批,引擎道)**:
1. **折算公式升级**:等效算力=频率×核类系数。VS-2 供给折算缺口:ideal=running×(f_actual×cap(实际核类))/(f_bigmax×cap(大核)),deficit=running−ideal——小核高频跑也会显出对大核的真实缺口;R5d 按下游消费核折算:1−(f_waker×cap_waker核类)/(f_consumerMax×cap_consumer核类);"已按大核满频(或接近)运行,无供给缺口"判词随新公式重判(小核满频≠无缺口)。
2. **核类识别**:优先 trace 内证据(cpu_capacity 类打点/厂商表工件);无则按既有簇结构(cpufreq policy 簇,引擎已识别大核簇 fmax)以簇 fmax 排序映射核类(2 簇=小+大;3 簇=小+中+大;4 簇=+超大),fail-loud:簇结构不可判则退回纯频率比旧算法+显式披露。
3. **披露纪律**:默认表粗算必须披露(口径词/图例:"按默认算力比粗算(非厂商实测表)");证据表在用时披露表来源。"下界"图例更新:核类优势已计(默认或实测),下界残余来源=频点缺失片段计 0(§24.1补 图例条目随改,原"折算未计大核单周期优势"半句退役)。
4. **波及面**:supply-fold 全家(缺口/verdict 判词/E8 类"无供给缺口")、gated running 分量、DCS 背景综合排序占窗当量(如涉)、既有数值 pin 大规模演化预期(EVOLUTION 引 §26);eval 敏感=代表性复放对照。
**排批**:引擎道 SYM-2 之后(CAP→RCM→P0-E);显示道不阻塞(COV-2/LAD 照跑)。

## §24.18 PTV8-RCR-C 批收账(2026-07-08,复核 SHIP-WITH-FIXES→三收尾已落)
交付六组:G1 §20.2 纯 running 缺口臂(第三口径词结构化生产者,恒等式守卫 Round3Equal(eff,deficit) fail-open,事件臂排他 pin)+G2 行1 词位预留席(RowNameFitted 唯一裁决,词整体回贴,MidTruncateKeepPid,B1/B1-b+ptv7_spn 22 pin 全绿,#12 guarantee 自灭,floor-clamp 角 pin)+G3 链上L# 收编行2(单门共享,hop 保旧)+G4 供给注两形(小缺口="接近大核满频,缺口仅 Vms(已计入)",永不否认身旁数字;"无法折算"co-repair)+G5 置信三面单源+G6 增量捆(C5 承自闸=精确 eff>cum/C6 三面"链上L#(未接入树)"/C7 禁"未分类"冒名+**binder_wait 迁族**(复核抓获 IPC 被当 IO:RCR-A 起误骑 ⛓ 族,迁独立行"binder等待候选"+◦ 兜底 glyph,COV census 成员资格保全)/C10 ◇▒ 重基/C11 卫生捆(同 tid 双名归一声明)/D-5 退役词/窗标 chip"根因排序#N·窗X")。18+2 test,9/9 突变咬红。E4/E8 终形=§24.9 钉定形 verbatim。
**留账**:IPC 族专属 glyph 候选待用户裁定(⇄ 类,需 EAW=N 单格核验,现 ◦ 借用);混合榜(1 有窗+1 无窗)#1 碰撞整体不发 chip(absence-never-guesses 保守);G4 两形+"下界"图例将按 §26 CAP 演化;C7 trace_span 无族行"未分类"与类型行微矛盾=措辞裁定候选;新 typed effective 生产者需同步补 C5 卫(闭集)。
**§24.14 补2(裁定,2026-07-08)**:D-4 ±10% 披露的单工件面 scope(分析窗 vs 用户宣称时长,非对比场景)归 **LAD 批**——树头披露行,复用 COV-2 同判据 helper;COV-2 只落对比面(批名即界)。

## §24.19 COV-2 批收账(2026-07-08,复核 SHIP-WITH-FIXES→四收尾已落)
交付:V1 症状 cell 主导性闸(census 同源,两臂口径词齐,CWD 共识接入,异臂注 typed enum)+V2 四列窗基披露(背景列"窗基:"机制复用;供给列"占其查询窗X%";"链上已归因(单项最大)"列名正名)+V3 背景 cell 闭集回退臂(禁伪"—";复核 F1 修=值车道镜像 census(MergedMax/periodic 跳过)+口径词按 DisplayImpactSource 门,Σ 冒"单项最大"与"非跨线程累计"错标双 witness pin)+V4 D-4 ±10% 双侧同披露(%.1f;单工件面归 LAD §24.14补2)+V5 cmp_78 终形对标本原文逐格对账(链上列修正 65.232/92.346+fixture EvidenceID 撞号 bug 顺带抓获)。注洪泛最小合并(同 base 元组合一行+D-4 折一行多分句,全触发 8→6,≤6 pin;完整分层归 LAD)。11+演化 pin,8 突变咬红。
**留账**:hop 臂"合计"vs union 口径(P3,需 tree.go 一行扩返回值,归后批);per-column row-exact 窗需新 tree.go 面(共识 lane 复用为 §21.1 刻意设计)。

## §24.20 SYM-2 批收账(2026-07-08,复核 SHIP 零 FAIL)
交付(§24.17 实施):降道谓词收窄=registry lane 推导闭集(WakeupChain∪LockContention 恰 5 token:sleep_wait/fragmented_sleep_wait/missing_wakeup/binder_wait/blocking_span;lane 活闸+token tripwire pin 双守护,新 token 入 lane 自动降道但必咬红强制对话);自因四态恢复参赛(runnable→调度压力候选·全额(§7.4 词表有意收敛,量纲注强制伴随)/running→算力供给候选·折算/IO→IO阻塞候选·全额/D-state→D状态候选 新族行(d_state_or_io_wait 按构造=D且无IO,不误标));"(优先级低于RT)"=RunnableBelowRTPreempted typed 字段(ohos_cfs 目标∧同CPU ohos_rt 竞争者∧R5g 位移重叠,Harmony-only,R2' 六处同步,6 负向 pin);盖章全量保留(身份事实)tier 只落等待族;三显示滤臂 key 在 tier 按构造同步。31+ pin,8/8 突变咬红(含复核自设两条均有既有看守)。cmp_78 修后形:自身 binder 仍不入榜,自身 runnable 加冕 lead+❶。
**留账**:①SYM-3 候选=树形自因行 board 席(trunked 形自因行入 SelfRows 不入 board,显示加冕被排除;修向=board 改 tier 滤+❶ self-stanza 渲染+LEAD pin 冲击评估);②全自身零缺口 running 平铺形加冕 0-eff lead(新可达,与 eff≤0 旧语义一致);③tool Description 5-token 枚举无 lane pin(lane 演化手工同步);④加冕句是否携拆解成因词=措辞裁定候选。

## §24.21 PTV8-LAD 批收账(2026-07-08,复核 SHIP 零 FAIL)
交付(§24.11 施工图+§24.8 总则):L1 run-length 循环折叠(k≤3 元组×≥2 贪心左起最大覆盖平局取小 k,通用非特化,异元组不折;旧 index-0 RepeatingPath 退役双墓碑)——huadong 梯子 14 行→5 行(纯 ping-pong=3;真实形保 2 次段外命中+中断跳=精确信号诚实),MAXWIDTH 135→100 对标本复放实证;L2 F2 首次一次(循环行整名承担披露义务,段外命中保强制展开,types 载体零动)——**§22.1 F2 条目修订**:"每命中必展开"演化为"段外命中展开+循环段内命中并入循环行 ×N 计数"(引 §24.11);L3 「⊚中转」短记号+CycleFold mark 哨兵;L4 缩进封顶 12 级(AncestorRails 单构建器双面同源)+8 格 pid 地板——竖裂消源;L5 不可断 atom 表(复合口径词+DL2 两洞闭合);L6 表尾注三级分层(typed 发射点定类,超 4 折入 _compare_notes 系统块,PSG 精确集收编,§24.19 留账销账);L7 D-4 单工件面披露行(SoloArtifact 精确门,§24.14补2 销账)。11 test+6/6 突变咬红。
**留账**:①带数据循环覆盖位的 stray 降档词面(未接入树 vs 深度可知,随 C6 词族收口);②循环行整名 3×长名+深缩进可越 100 行宽(有界,§24.8 序内,观察);③部分轮残余并入 ×N 注/段外后续命中并入邻折叠 roster=两措辞裁定候选(涉披露义务)。

## §24.22 RCM 批·引擎半场收账(2026-07-08,复核 SHIP-WITH-FIXES→四收尾已落)
交付(§24.7.1+§24.10 施工图):M1 语义族 fold(FoldSemanticSpanFamilies 单点;键=(线程,语义类,道别);值=同窗区间并集合计,disjoint==Σ,union<Σ typed 披露;SemanticSpanFamily 载体;rank/观测两消费方同函数同源(重叠形同源 pin 复核后补齐);单员族逐字退化零演化;roster typed 六字段+note keys R2';registry causalTokenFamilyFoldLanes 申报(8 generic+4 semantic,§7.5 supply_pressure 不入))+M2 通用族合并(键=(type,thread,道别,selected_window);口径梯四臂=count_sum/sum_disjoint/interval_union(值==区间长门)/max_overlap_fallback 诚实下界;inode/dev typed 六 mint 点零 Summary 反解;§7.5 35% 帽单次施加;effective 单公式禁 Σ clamp;状态标量按口径梯重导出(复核 F3))+M3 跨窗纪律(分行不二次求和)+M4 榜位对照(cmp ×14→7.0 登顶/opendir io 并集 2.547 反超 block 1.598/huadong 2.1 登顶=弱化排序双向同修)。32 pin,6 突变咬红。
**留账**:F5 roster 分隔符 " | " 遇含分隔符 span 名错拆(仅显示面);F7 enrich 帽后/build 未帽 effective 两纪律角落(均诚实);二次 fold hull 退化(今日不可达);**F6=RCM-2 置顶**:合批窗口期 family 合计值挂既有"累计(跨线程)"回退词(同线程合计被误标跨线程)+roster/×N 不可见——display 半场为 §24.7.1① 完整闭环硬前提;pre-sort 语义行整体前置(并列键才可见)。

## §24.23 RCM-2 批收账(2026-07-08,复核 SHIP-WITH-FIXES→四收尾已落)
交付(§24.7.1①/§24.10 显示闭环):D1 第五口径词"合计(共N段,同线程)"(口径梯四臂 typed 映射;unknown fail-open 零造词;图例 verbatim 紧邻 ×N取最大;"累计(跨线程)"对 family 行四路封禁——F6 过渡态灭)+D2 四行文法 family 形(恒等式 V==发布值 fail-open;×N 词位挤压保全;背景榜位#N typed 双态)+D3 三面单 helper(lead family 车道居 Merged 折价道之上,隔离负向)+D4 索引审计 token(member_* 前移 confidence 后,最坏前缀 pin)+D5 C4 分组+哨兵+三标本 dump 逐字命中裁定终形。复核四修:F-1 R2 展示聚合嵌合体(≥3 同类行整行继承 family 九字段→双 ×N 矛盾;无条件清除+端到端 chimera pin,fixture 以 family 为 group-first 种子防掩护)/F-2 明细 stanza 披露单源化(原 pin 固化过"重叠未拆+已并"同行矛盾)/F-3 token 前移/F-4 ✦ 榜位产线可铸(family 观测记录发 background_rank,POSITION 计全体 FIELD 只落多员族)。17+6 pin,6/6 突变咬红。
**留账**:观测通道榜位与 rank 视图 composite 板计非同源(各自如实);pre-sort 语义行前置=裁定候选;F5 roster 分隔符(引擎侧)。

## §26.1 CAP 批收账(2026-07-09,复核 SHIP-WITH-FIXES→三收尾已落;基准簇用户裁定=大核)
交付(§26 实施):C1 核类识别(cluster_freq_share 单一权威;显式拓扑 membership-only 类别按 fmax 序;四 fail-loud 臂含 fmax 平局禁掷币;系数表单源 小1.0/中2.3/大2.53/超大3.036)+C2 公式(VS-2 ratio=(f/fmax)×cap 比,同簇同源(复核 F1:跨簇拼积曾凭空铸 1.650/5.987ms,修=基准簇提名后 (fmax,cap) 恒同簇,降级取有治理数据的最高类簇同迁,宁 freq_only 勿拼积);R5d 系数=1−f×cap 积比,max-over-consumers 按积序,任一侧成员未知同退纯频率(复核 F2:waker 侧裸 1 曾激进铸 9.988 vs 真值 0);clamp 双保险负 deficit 不出厂)+C3 三态披露(default"按默认算力比粗算"/freq_only 显式/evidence 接线;"下界"图例更新原半句退役;降级基准判词如实"按小核/中核满频折算"+专图例)+C4 波及演化 13 处。**基准簇裁定(用户 2026-07-09)=大核类簇(§26 字面,cap 2.53;四簇形 prime 不作基准)**——与实现一致,改裁顶簇=coreCapabilityReferenceClass 一处切换(两 pin 预期演化已注)。数值对照:小核满频 0.990→6.378/同频点跨核类 R5d 0→11.490/大核满频 0 不变。26+ pin,10 突变咬红。
**留账**:降级形 affirmative 相对语义半句=措辞裁定候选;混窗聚合 ReferenceClass 取首成员(与 fmax 置零同界);evidence_table 通道待厂商表落地补词。

## §22.2 P0-E 收官批收账(2026-07-09,复核 SHIP-WITH-FIXES→四收尾已落;销 §22.1 立账+§24.9-C 锁三修)
**P1 链真分支**:ChainNode 真实递归 Depth+Branch(via-immune 续编,碰撞 pin);wakeup_chain 每分支一条真实 path 记录(ClaimKey wakeup_chain:path#N,扁平化只剩 legacy 回退臂);选举 branch 池独占开关(法本体共享,branch 池四臂自有 pin);树 attach 加链域(rel=Depth−rootDepth 重基,跨分支行=链上L#(未接入树) 诚实席,**ChainBranch==0 有损信号禁作硬门**——复核 REPRO 跨分支 aggregate 假挂已修+pin 翻转);假 L26/L27 深度标签消除,huadong 形=8 条真实分支+真深度 L1-L3,LAD 循环折叠自然 inert(实渲验证)。
**P2 锁三修**:fold 键补 waiter 身份(嵌合体禁折,同 waiter 双打印仍折+Waiters max);'-->' 移交链 typed 解析+披露(最后持有者非全段持有,保守整段+披露,不分段不造数);同锁自相矛盾守护(推断级持有者自身同锁排队 ≥50% 重叠→归因撤回,经既有 §12.3 typed 机制失直通道+1.35,零权重编辑;payload-direct rung① 豁免);推断级持有者 caveat 三面显达(树行括注/明细 持有者来历·移交链·撤回 三行/lead 括注,zh/en);waiters=0 不设门(裁定)。opendir 嵌合体形=两 waiter 各自成行+受害行保留+荒谬归因撤回。
**审计面**:系统补充当选分支 path 记录提桶首(typed WakeupPathBranch×artifact 身份匹配,fallback branch 号升序+留账)。
28+ pin(22+6 收尾),12 突变系+4 重放全咬红。
**留账**:raw trace 复放待原件(berlin.systrace+东湖 ftrace 入 /Users/han/opt/customlogs 即可本机复放);D-10(东湖 actual 口径)仍开;aggregate Path note 走查观察;conf 未压(取披露);移交链 tenure 分段=未来裁定;elected 记录 cap 存活非保证(fallback 注明)。

# §27 79 系回访审计(2026-07-09,两标本:huadong_79_01/opendir_79_01;26 批改动后首轮真实回放)

## §27.0 生效确认(正向 witness,勿当回归)
真实链深 L1–L5(假 L26/27 灭,梯子消失)/家族合并参赛(io_latency ×8/×6 interval_union+原始和披露,双场景)/症状降道(目标 sleep 零榜位)/周期源降道(VSync 投影 38.996→eff 0.166)/自因四态(目标 D-state #4 参赛;OS_FFRT running→算力供给候选 #3)/CAP 双公式+纯频率比披露+恒等式全平/移交链披露/自相矛盾撤回(LegoHandler 荒谬归因撤回生效)/多窗窗标+覆盖句拒伪百分比/边界抖动披露(112.223 vs 112.175 略超 0.048)/dump 配额/E# 并入。

## §27.1 已修:HTML 报告排版(6287e1cc,当日直付)
客户投诉字间距过紧难读。根因=preview/server.go renderHTMLPage CSS 字体栈无任何中文字体(Windows 中文回落宋体)。修=CJK 栈(PingFang SC/HarmonyOS Sans SC/微软雅黑/Noto Sans CJK SC)+行高 1.62→1.78+正文 letter-spacing .02em+pre/code 字距钉回 0(保 CJK 双宽网格 bar 对齐)+li 间距;pin=TestStandaloneHTMLPageCJKFontAndSpacing。

## §27.2 P1 引擎四案(三路只读核实,全部确认)
- **G1 跨车道重复显示**:io_latency rank 家族行与 critical_blocking 逐 peer 行同批事件双发布(opendir E3↔E6-E9 原始和 2.858 相等;huadong E10↔E13/E19-E22 原始和 15.156 相等,树内并列 5 行重复)。归因:三套折叠全落空——同段两车道折叠臂资格要求 inversion 候选∧MergedCount≤1∧精确行区间(tool/answer_document_mutation_runtime_tree.go:2242-2256,:2222);critical_blocking 不在 causalTokenFamilyFoldLanes(tracequery/causal_token_registry.go:295-305);NEW-3 令牌集不含 io_latency(tree.go:2005-2011)。修向=同(线程,type,窗)成员集跨车道对账(吸收或"已并入家族行"标注)。
- **G2 trace_gap 占席+同窗自相矛盾**:数据盲区行实占根因排序席位(#6-#12,query.go:8788-8808 无跳臂,assignRootCauseRanksAndTiers 仅语义/SYM 两跳臂 :11742-11792);"窗内无调度数据"判据整类排除 running(:15038)∧过滤<minDurationMs(:15035),而同(线程,窗)running 从 depth-0 例外通道(:8763-8772)另铸 rank#3——OS_FFRT_2_2-43037 同窗"#3 running 0.051ms"+"#6 窗内无调度数据"并存;dh-irq-bind E8 有数据+#12 盲区同窗并存。另双发布:个体 ◇ 盲区行与 on_chain 溢出折叠(types/trace_causal_projection.go:2441)不排他。修向=trace_gap 降道跳臂(非成因不占席)+判据措辞如实(或补 running 检查)+双发布去重。
- **G3 count_sum 家族三面不一致(恒等式引擎侧已破)**:页缓存抖动行 树 41.671(封顶发布值,排序同源)/明细成员 133.200+65.100=198.300(churn×0.3 伪 ms,query.go:8871,单段超整窗)/表 链上累计=有效归因=198.300。归因:rank_family_fold.go:621-622 Cumulative=未封顶计数和 与 :658 ImpactMs=backgroundImpactMs 封顶分叉,normalize 回退把 Effective 也填成 198.300(query.go:9858,:10149-10168);roster 串 "ms" 后缀硬编码(rank_family_fold.go:566);表三列直通无家族/邻近 gate(tree.go:8356,8403-8404)。修向=count 类 Cumulative/Effective 与发布值同源(或 count 类专列语义)+roster 单位按 Additivity 印计数。
- **G4 跨窗挂接假边**:attach 硬门只比 (ChainBranch,rel)(tree.go:1280,:1267-1293),branch 序号各查询窗自 1 编号(query.go:7683-7686)跨窗撞号→W2 hmfs L2 挂 W1 触控链 L1 下,明细"关系: 唤醒 OS_mmi_EventHdr-43103"为假边(真 path=hmfs→VSyncGenerator)。QueryWindow* 逐节点已携带但"no gate reads"(types/trace_causal_projection.go:199-209)。修向=挂接域补窗身份维(或 branch 全局编号)。P0-E 已知残洞第二形(第一形=ChainBranch==0 两义,§22.2)。

## §27.3 P2 显示五案(确认)
- **G5 occurrence 自因子行**:同线程 2 occurrence 被 buildTrunk main/extra 拆成"主行+├─成因─ 自身 sleep"(tree.go:1347-1394);×N 合并阈值≥3(trace_causal_projection_aggregate.go:45-49)+无引擎家族 note 双落空。修向=trunk 同(线程,状态) extra 并 ×N(阈值降 2)或引擎发家族 note。
- **G6 症状行有效归因泄漏**:指标表 8300-8409 循环无 IsTargetSelfStateRow gate,目标 sleep 行"有效归因"列印 6.357(列定义=计入排序;该行无榜位)。修向=症状行该列"—"(对齐 tree.go:7611-7635 既有约定)。
- **G7 自因分解行词值错配**:行1 状态词来自 Gated 分解(rcr.go:604-617),数值=单态 ImpactMS(tree.go:2773,:8403)——"runnable 4.115ms"实为 running 投影;sysmgr 窗口投影 2.770 漏 runnable 0.621。修向=双状态行行1 值取两态窗内和、词值同源。
- **G8 折行孤儿**:折行器任意原子边界可断(tree.go:5222-5258),"数值(口径)"未绑超原子→"(全额)"孤行;裸"小核"不在 atom 表(:4976-5014)逐 rune 拆;无"过宽退分离形"降级。修向=数值+括注超原子+口径 CJK 词补表。
- **G9 序数洞**:Rank=i+1 降道前预分配(query.go:11727),降道臂不回收,display 不重编号(tree.go:1915-1920)→多窗面 #6/#7/#12 独现、#1-#5 不可见。**裁定候选**:display 重编号 vs 保序数+树头披露降道席位(§24.13"阶梯透明"裁定的可见性延伸)。

## §27.4 P2 管线/成文五案
- **G13 成文主因与投影 ❶ 矛盾**:huadong LLM 全文主打 VSync 延迟链(引擎已周期源降道)而投影 ❶=hmfs IO;opendir 编造"enqueueMessage 消息队列锁"等待点(span 原文 blocking from=AssetManager.getResourceValue(AssetManager.java:761))。修向=skill 软引导(成文主根因须与投影 ❶ 同实体;等待点须引 span 原文)+blocking_from typed 化为"等待点"面(同 holder_site 待遇)+evaluator 软检;与裁定点2(提及硬门)同席裁定。
- **G14 PSG 残差五 witness**:153ms/inode 0x6a16/287.834ms/8处/4260次——皆探索期数值未入终面证据仍出正文(软门一轮闩放行属设计)。裁定候选=维持软引导积累 witness vs ms 标量硬化。
- **G15 verdict 降级正文残留**:"未评估:本轮无源码证据"标签下正文仍断言"该代码路径仍然存在…风险仍然存在"(opendir 决策块)。修向=降级时正文注入披露/裁剪断言。
- **G16 引用-断言错位**:opendir Hop3/4/5 引用整体错位一格(优先级反转 hop 挂 io_latency 引用)。软引导候选。
- **G17 移交链名义歧义**:披露"A --> B"未标注为线程名,LLM 读作"链路"。修向=披露词补"(线程)"。
- **G10 EN 撤回披露直出**:query.go:13634 witness 英文句直出 zh 明细(§22.2.1 词条尺子违规,P0-E 新引入)。修向=witness 本地化或降入证据索引审计字段。

## §27.5 P2/P3 卫生
- **G18 字节截断 mojibake(per-CLASS)**:emit_analysis.go:4987-4988 s[:120] 字节切 CJK→"事件��…"(调查单元三面同污染);同类 s[:n]+"…" 字节截断全仓约 10 处(log_triager.go:267/loop_policy.go:779/semantic_quality_reviewer.go:877/orchestrator.go:4091/extractor.go:307,361,883,3265/context/builder.go:4178/memory/store.go:1633/recall_memory.go:199)。修=共享 rune-safe truncate helper 一次收口,禁逐处修。
- **G11 目标自身 binder_wait 不可见**:4 条 root_cause_target_self_state binder_wait(最大 3.527ms)仅存系统补充,树自身状态区无(0.011ms D-state 反而在)——§24.8 抵触。修向=目标自因等待族入自身状态区(症状身份披露)。
- **G12 E23 双成员同值疑云**:hmfs_discard 与目标线程折叠双成员同值 14.272ms 到 μs 级,疑同段双归属;需原件复放核实(挂 D-10 复放清单)。
- **G19** 零值折叠行噪音"×9(0.000–0.000ms)取最大"。**G20** cpu_frequency_limit 双行同值未折(系统补充)。

# §28 TEX 需求裁定+texture 片段支撑数据审计(2026-07-09,用户令:79 系收口开工·商用标准)

## §28.1 用户裁定(2026-07-09)
- **TEX**:"Texture upload" 纳入语义 span 类,与 VerifyClass/Shader/JIT 完全同待遇——on-chain 累积折算参与根因竞争排序(同 §23.1 tier 车道)+提及义务(非链上背景综合排序,提及门 background_rank≤3)+§24.10 同(线程,语义类)窗口投影合计参赛。扩展点=traceSpanSemanticWorkClass 第五臂(query.go:12579)+registry semantic_class 车道第五 token(texture_upload);类名保英文(§22.2.1 专名尺子);"(15283) 512x194" 等 id/尺寸后缀归一化出类、原文留 roster 区分键。
- **收口批准**:§27 全清单按建议收口;G9 序数=引擎侧重编号(序数只分给携榜位显示身份的行,三面同源,单窗/每窗连续;§24.20"榜位照发"相应演化,EVOLUTION RECORD 记录);G14 PSG 维持软引导+skill 加"探索期数值未入终面证据禁引用"精确指令;G13 软引导+evaluator 软检+blocking_from typed 化。

## §28.2 texture 片段(trace_texture_upload.txt)支撑数据审计
- **T1 簇轨证据(CAP-2 裁定候选,先立案不动手)**:clock_set_rate m3_c0..c3_freq 四簇轨携 cpu_id=0/2/10/12 簇锚(非发射 CPU),值 417000/417000/1200000/2750000;另 thermal_inte1 state=2200000 cpu_id=2(热限轨)、m3_vote_delay、pid_task_freq。恰补两份 79 报告"簇结构不可判"缺口。**冲突点=§7.10 VS-2c 终局裁定**(clock 名=厂商自由词汇,禁喂 fmax 供给基,semantic_ruling_pins_test 结构 pin;isCPUFrequencyClockName 现行启发也不匹配 m3_cN_freq——无"cpu"子串)。cpu_id 为键控精确字段非名字启发,是否开"cpu_id 键控簇轨"evidence 接线=需用户裁定(重提先读 §7.10/§7.5)。
- **T2 B|pid≠发射线程 tgid**:span 标记 B|18998| 而发射线程 RenderThread-51342 tgid=50820(unyuan.app.chat)。span 归属应锚发射线程;若按标记 pid 归组则查询 pid=50820 时丢 span。TEX 批必须客户原文 pin 验证归属车道。
- **T3 形态覆盖**:纯 B|pid|name(无 H:/无 |I 尾)、E|pid 裸关、E|pid|I39 带载荷关、span 名含空格/括号/尺寸;嵌套子 span(AllocPages/MapMemory/Alloc Ioctl/GraphicsLoad 在 Texture upload 内)——同类内区间并集已防双计;**跨类嵌套**(子类若也成语义类)双计=裁定候选,本批只收 Texture upload 顶层。单次 76–118μs、高频形——家族合计参赛正是对症(×N 合计如 VerifyClass ×14 登顶先例)。
- **T4 调度扩展字段**:next_info 已有解析(NextInfoAffinity/AllowedCPUs/Load);prev_state=R+ 既有;prio=301(dh-irq 类)Donghu 优先级分类待核(误分类影响 R5g/反转判定)=TEX 批核验项。

## §28.3 Wave-3 批次计划(三道并行)
3.1 并行:**GAP-A**(tracequery 独占:G2 引擎跳臂+盲区判据 typed 化/G9 序数重编号/G3 引擎半场/G10 witness 本地化)+**GAP-B**(types 投影+tool 显示独占:G4 挂接窗维/G5 occurrence ×N/G6 症状列 gate/G7 词值同源/G8 折行原子/G11 自身等待可见/G17 移交链"(线程)")+**GAP-C**(skill+evaluator+verdict:G13/G14/G15/G16 软引导与软检+降级 caveat 附注,不代写正文)。
3.2 顺序:**TEX**(语义类第五臂+T2/T3/T4 验证,tracequery+显示)+G2/G3 显示半场+blocking_from typed 化+**HYG**(G18 rune-safe 共享 helper per-CLASS 收口约 10 站点/G19 零值折叠行/G20 同值背景双行折叠)。
CAP-2=等 T1 裁定。

## §28.4 CAP-2 裁定(用户确认 2026-07-09):键控簇轨复合门 evidence 接线
裁定形=**五重结构门全过才升级 evidence,任一失败整体回退默认表**(不部分猜测):①族形门(同前缀+索引位轨族成员≥2,名字只作族分组不承载绑定语义)/②异锚门(族内两两 cpu_id 不同;witness:thermal_inte1 cpu_id=2 与 m3_c1_freq 同锚碰撞,证明裸 cpu_id 不够)/③不变式门(全 trace 同轨 cpu_id 恒定,违者 fail-loud 整体回退)/④量纲门(state 落 CPU 频率合理区间)/⑤相容门(锚点集合⊆调度观测 CPU 集合)。绑定语义全部来自 cpu_id 键控字段+确定性检查。
残余假设诚实披露:成员归属=锚点连续分段**推定**(披露词单列"成员按锚点连续推定",与"簇拓扑=簇轨实测"分开);fmax=该轨 trace 内治理时间线最大值(沿 CMP-10)。
**§7.10 VS-2c=演化非推翻**:轨名单独绑定仍禁(isCPUFrequencyClockName 名字启发仍只软引导);新增"键控簇轨复合门"例外臂;semantic_ruling_pins_test 结构 pin 走 EVOLUTION RECORD 增臂不删原 pin。thermal 轨不入 fmax 基,另立"热限披露"候选(供给侧背景证据,未立批)。
预期效果:Donghu 类 trace CAP 从"簇结构不可判,按纯频率比折算"升级"按实测簇轨折算(成员按锚点连续推定)";门不过的 trace 行为不变,零回归面。验收=两原件复放对照。排期 Wave-3.3(GAP-A/TEX 落地后,tracequery 域)。

### §28.4 补充裁定(用户 2026-07-09):flavor 中立复用
Donghu 类与鸿蒙类 trace 同硬件平台——CAP-2 五重门**不设 flavor 门**,ftrace/hitrace 同一条接线(门是结构判定,与 flavor 无关);同平台鸿蒙 trace 发同样键控轨族即自然过门升级实测折算。边界:**跨 trace 拓扑移植不自动做**——无键控轨的 trace(如 hmtrace 无键位置形 cpu-cluster.N,无 cpu_id 锚)诚实回退默认表,不得借"同平台"移植他份 trace 实测拓扑(平台身份无 trace 内精确信号,absence never guesses);未来 trace 元数据携平台标识可再议移植臂。

## §28.5 cpu 片段(cust_trace_cpu.txt,关键字过滤,与 texture 片段同 trace 同平台)支撑数据审计(2026-07-09)
- **T5 标准 cpu_frequency 全 CPU 扫存在**(行8631-8942 形):per-CPU state 键控扫,共动分组直接可得成员——{0,1}{2..9}{10,11}{12,13} 四簇(与 m3 锚 0/2/10/12 完全相容,**锚点连续推定在本平台被实测验证**)。现行 cluster_freq_share.go 单一权威只认显式拓扑、不做共动推断→"簇结构不可判"的机理候选(CAP-2 首任务核实:79 两原件是窗内无 cpu_frequency 还是有而未推断)。**CAP-2 证据梯修订=两级**:Tier-1 per-CPU cpu_frequency 共动成员+cpu_frequency_limits 治理 fmax(cluster_ceilings.go VS-2b 梯已有消费根基);Tier-2 键控簇轨五门(cpu_frequency 缺位时,成员按锚点连续推定);两级并存时交叉验证(轨值须与锚 CPU cpu_frequency 相容,不符 fail-loud 弃 Tier-2)。
- **T6 thermal_inteN 是索引族——§28.4 早期假设纠正**:thermal_inte1/2/3 成族(①族形门旧排除论证**推翻**);inte1 携变动 cpu_id(2/3/4/5/7)→**③不变式门才是热轨判别主门**(witness pin 必做)。残余威胁:短窗只见 inte2(恒锚10)+inte3(恒锚12)可伪过①-⑤→**补第六道负向词汇筛(exclusion-only)**:名含 thermal/ddr/gpu/vote/delay/info/load 的轨排除出候选族——负向筛最坏=回退默认表,永不伪造,与 §7.10 方向相容。理论残洞(无害词汇+恒锚伪族)接受披露兜底(实测折算行审计注携轨族名可回溯)。
- **T7 thermal_inteN+limits=热限时间线**:inte1 2200000↔1850000/inte2 2295000↔1990000/inte3 2350000,与 cpu_frequency_limits max(cpu2=2200000/cpu10=2295000/cpu0 1750000→1550000 动态)互相印证=按簇热限治理时间线。**THERM 披露候选**:算力供给行披露"窗内该簇受热限压至 X"(成因语境,disclosure-only 零权重编辑),建议并入 CAP-2 批 C4 面。
- **T8 pid_freq 编码陷阱**:pid_freq 在 isCPUFrequencyClockName **显式白名单**(parse.go:2613),但本平台 state=10240923 等非纯 kHz 编码(10.24GHz 量纲荒谬),且单名多 cpu_id(发射 CPU 即 cpu_id);消费点 supply_fold.go:248 软车道——CAP-2 审计该车道并加量纲合理性筛(噪音源头消除);pid_task_freq 同。
- **T9 heca_info/heca_ddr_freq**:单名扫全 CPU 的遥测轨(state 非频率编码),被③不变式门+负向筛双重排除——门设计的活 witness(pin 素材)。
- **T10 GPU 轨族**:gpuload/gpufreq_info/gpufreq(167100000=167.1MHz)/gpu_state 治理通道——GPU 供给披露候选(texture/render 场景背景证据,未立批,等场景需求裁定)。
- **T11 双栈确认**:同 trace 并存 Android(android.display/systemui/wmshell)与 OHOS(sceneboard/OS_IPC/OS_FFRT/render_service)线程——与 opendir 案 AssetManager.java 形一致,平台背景,无新 gap。
- 附:本片段与 texture 片段同 trace(同线程同时间轴)——客户后续给全量原件时一次复放同时验收 TEX+CAP-2。

## §28.6 INODE 高频排序能力审计(2026-07-09,用户问询触发;三读+交叉核实,五断言全核)
**结论:今天问"哪些 inode 高频 IO"拿不到正确答案。**现有=根因语境三载体(file_io_by_inode count/bytes/latency 按总延迟排/page_cache_by_inode churn/block_io_by_inode join 面),全部键含 PID 分片、各 top-8 静默截断、count 仅第三优先;block_io_by_inode 还建在截断后输入上(二次聚合不可恢复全量);event_search+pattern 可查单 inode 原始行但无聚合。
**缺口 13 项**(引擎①-⑥/暴露⑦-⑨/路由显示⑩-⑬,详见审计工件):核心=①无 (dev,inode) 全窗跨线程载体(count/bytes 跨线程求和合法,墙钟红线只禁时长)②无频次优先口径④截断无披露**⑤hmfs_ 前缀不在 isFilesystemEvent 名单(parse.go:2355-2366)——本客户 HarmonyOS 平台 FS 层事件整体漏采,东湖案 inode 实际全靠 mm_filemap 侧漏进**⑥无 inode→路径注册表(entry_name 机会性;文件名仅 display-only note 车道,投影 typed 面只有 inode/dev)⑦无 inode=/dev= 过滤参⑩无"枚举/排序型 trace 问题选 view"教学(enumerate vs root_cause 路由分叉未收敛)⑪纯统计问下投影树/指标快照/观测补充三确定性面全缺位⑫QFEnumeration acceptableClaimForms 缺 external_observation(提示语误导非硬堵)。
**最小补齐路径(INODE 批候选,Wave-3.3 与 CAP-2 同道)**:1) query.go:1764 截断前对全量 fileIO/pageCache map 按 (dev,inode) 折叠新载体 WindowStats.TopIOInodes(count 优先排序+总组数披露;延迟只发 max 或按线程分列守墙钟红线);2) 暴露面 window_stats 文本段+typed observation+description(可选 inode= 过滤参);3) skill 视图矩阵加枚举型 IO 教学+QFEnumeration claim form 提示语修;4) parse.go:2360 名单加 hmfs_(顺手关口,独立可先行);5) 若入 rank/观测补充走 R2'+§7.2.1 红线。inode→路径注册表=独立候选(f2fs 类事件无名,收益依赖 android_fs/entry_name 覆盖率,先立不批)。

## §28.7 Wave-3.1 收账(2026-07-09,四批+HTML 字体批全部推 main;§27 立案 20 项中 17 项闭环)
**已推**:HTML 字体(6287e1cc)/GAP-A(1d450feb)/GAP-B(0960e41c)/GAP-C(cad2cdca)/HYG(f35d1fc0)。四批各经对抗复核(全 SHIP-WITH-FIXES)+收尾闭环;终态 make 绿+77 包全绿零 FAIL;每批突变自查+复核假 pin 抽查全咬红。
**交付对账(§27 → 状态)**:G1 跨车道对账=**未做**(3.2 队列,引擎工程量大);G2✅(data_gap tier+typed kind 两形,自相矛盾 typed 拆解端到端 pin);G3✅(count 三通道单源+计数当量三面同源,fence/明细两面对称);G4✅(挂接+trunk 消费双面窗/branch 域单一权威 helper);G5✅(×N 折叠+聚合视图排除双半,R2 同盲点捎带);G6✅;G7✅(词随值走);G8✅(超原子+簇词;>12cell 长括注=P3 留观);G9✅(引擎重编号;**复核纠偏:周期源保留席位**,恢复 VS-1 参赛形——§28.1 裁定文本的"榜位显示身份"以此为准);G10✅(zh witness;**留账:EN 报告面现携中文 witness,根修=witness typed 字段化两 lane 各自措辞**);G11✅(top-4 迁移+溢出 MergedMaxMS+分母跳迁移行);G12=待原件复放;G13-G16✅(四软引导+advisory 四重排除+降级披露);G17✅;G18✅(原语四件套+27 站点;**HYG-2 立案:域外 ≥30 站点,answer_reviewer.go:288 答案正文生截排最前**);G19/G20=3.2 队列。
**复核战果与新教训**:①跨批显示耦合三案同构(G9→SYM 披露句死亡/G11→COV 分母污染/G3→count 披露臂从不可达变必触发)——并行批复核必查跨面消费全谱;②fixture 漂移致 pin 假绿(SYM fixture 手工 Rank:1 为引擎不再产形)——**fixture 应取引擎实铸形**,新纪律条;③"display already suppresses"类前提必须实测,凭注释断言直接被证伪(P1-2);④复核自设突变再立功(窗门 End 比较删除存活→嵌套窗 fixture 补强)。
**3.2 队列(下批)**:TEX(语义类第五臂+B|pid 归属/E|pid|I39/嵌套/prio301 四验证)+G2/G3 显示半场(盲区措辞按 kind 分形+双发布去重+count 表列)+blocking_from typed 化+G1 跨车道对账+HYG-2+G19/G20+计数当量图例/atom(GAP-A P3-6)。3.3:CAP-2(两级证据梯+六门+THERM,flavor 中立)+INODE(§28.6,待用户点头)。

### §28.6 补充(用户裁定 2026-07-09):INODE 批排期确认
用户批准 INODE 批落地,排 Wave-3.3。施工图=§28.6 最小补齐路径四件套:①引擎 (dev,inode) 全窗跨线程载体 WindowStats.TopIOInodes(count 优先序+总组数披露;延迟 max/按线程分列守墙钟红线;在 query.go:1764 截断前对全量 map 折叠)②暴露面 window_stats 文本段+typed 观测+description(+可选 inode= 过滤参)③skill 视图矩阵枚举型 IO 教学+QFEnumeration claim form 提示语修④hmfs_ 前缀已由 TEX 批先行落地(§28.2-T3 顺手关口,销)。inode→路径注册表仍=独立候选不入本批。执行序:Wave-3.2 收尾提交后 → 3.2b G1 跨车道对账 → 3.3a INODE → 3.3b CAP-2+THERM(五门簇轨,flavor 中立)。

## §28.8 Wave-3.2 收账(2026-07-09,三批推 main;TEX/DISP-2/HYG-2 全经对抗复核+收尾)
**已推**:TEX+BLOCKFROM(f7738052)/DISP-2(cff25e37)/HYG-2(37fc46f3)。终态 make 绿+77 包全绿零 FAIL;三复核全 SHIP-WITH-FIXES 全闭环。
**交付对账**:TEX=texture_upload 第五语义类全链(引擎五通用点+客户原文 pin+H: 前缀 flavor parity 复核修+registry §7.2.1/R2' 全走+skill/config 五类词表[主会话])、T2 B|pid≠tgid 归属按发射线程 pin 固化、T4 prio301 保守三层链 pin、hmfs_ 前缀关口(§28.6④销)、blocking_from_site 等待点 typed 全链(引擎解析→registry hard_consumer→投影→"等待点:"明细行,opendir 原文整签名 pin);DISP-2=G2 显示半场(盲区措辞按 kind 分形+席位排他去重+G19 全零折叠行退役)、G3 显示半场(邻近行链上累计列 dash+count 端到端恒等 pin+计数当量图例/atom)、rcr 影响形态第五臂+**semantic-class universe 机械 pin(第六类漏改任何显示面即红)**;HYG-2=域外 49+ 站点 rune-safe 收口(answer_reviewer 答案正文/repomap 8 提取器+**extractorVersions 11 项 bump**(复核抓获:不 bump 已扫仓库修复静默不生效)/clampProfileSnippet LLM 提示面漏网)+G20 同值观测行折叠(fold key 全语义面+工件身份维,quota 包络无双减)。
**复核新教训**:⑤跨批 promotion 义务对置真空(两批各把 note key 升级留给对方,没人执行)——契约两端义务须在派单时指定唯一 owner;⑥"缓存版本协议"类仓库自身协议是复核必查面(extractorVersions);universe 覆盖 pin 是"跨面消费全谱"的机械化终态,凡枚举 switch 应配。
**留观立案**:DISP-2 P3-1 小桶双发布残形/P3-2 family ◇席 cum 推演盲点/邻近桶 cap=8 静默 limiter;TEX F5 banner 200B parity;G10-EN 面中文 witness 债(计数当量 EN 同族);GAP-B wave3.1 P3-3 长括注超原子。
**队列**:3.2b G1 跨车道对账(下批,引擎+显示单 agent)→3.3a INODE(§28.6 已批)→3.3b CAP-2+THERM。外部依赖:原件复放(G12/D-10/hmfs 实测/TEX-CAP 验收)+客户下一构建全场景回访。

## §28.9 Wave-3.2b G1 收账(2026-07-09,推 main;§27 立案 20 项全部闭环)
**已推**:G1 跨车道对账(见本节 commit)。§27 审计 20 项至此 **20/20 闭环**(G12 转待原件复放项)。
交付:引擎四维精确门(threadKey/裁定表恰 io_latency↔io_latency+universe pin/查询窗 typed 端点/区间∈成员并集**拒 hull**)+typed absorbed 标记(observations 照发不删,absorbed_by_rank_family/absorbed_into/rank_family_key 三键 hard_consumer+golden)+投影单收口点聚合前折叠(全渲染面自动继承,零逐面抑制)+家族明细「链上并入」注(E# 全引,守恒三面)+skill 不可加语义句。复核三修:P1 attach 位序(◇/▒ 桶填充前=披露死代码,复核 REPRO;移至全道填充后)+P2-a recon key 补 lane 维(与 fold key 同构,按构造对家族唯一)+P2-b ×N 合并保留首个非空 RankFamilyKey(仅 key 一字段,F-1 嵌合纪律完好)。30 pin,10 突变咬红(含 hull 嘈声维最强形)。
**留观立案**:P3-i 单行重复(MemberCount=1)scope 外/P3-ii cap 席位不回收(同既有账)/P3-iii 跨 result 双 tool-call 形不对账(注已声明;根修向=wire 携成员区间清单,需新裁定)/P3-iv ts=0 窗 stamp 跳过 recon 空转(合成 trace 才可达)/同 merge 组双 key 只留首(E# 兜底)。
**队列**:3.3a INODE(即开)→3.3b CAP-2+THERM。

## §28.11 验收模式裁定(用户确认 2026-07-09):客户不提供原件,复放转四层
客户不同意提供原始 trace 文件——"原件复放"依赖项全体转性:
1. **验收主路径=回访输出对账**(79 系已证可行):四场景回访清单落 docs/design/revisit_acceptance_pack_20260709.md,客户跑新构建贴全量输出转录(含系统补充块),逐格对账闭案。覆盖 P0-E/G1/盲区降道/INODE/TEX/词形排版全部验收。
2. **悬案转定向复查指令包**(同上文档):G12(E23 双成员同值 14.272 疑云)与 D-10(东湖 actual 口径互斥)各配精确自然语言命令,客户跑后贴几十行输出即可定案,不需文件。
3. **DIAG 自诊断批立案(3.3c,排 CAP-2 后)**:把"需原件"永久转化为"输出自答"——(a)跨线程折叠成员同值到 μs 时审计注携逐成员行区间+same_value_members typed 披露;(b)actual 口径两面互斥时 typed 一致性披露(不猜值,只标两面来源与差值);(c)hmfs 实测无需专项=INODE top_io_inode 段即探针(东湖回访输出该段有 FS 内容⟺hmfs_ 关口生效)。
4. **脱敏片段通道=候选**:客户已接受关键字片段粒度;必须看原始行时给脱敏指引(行号区间+名称哈希替换保结构);频繁则立 codrax sanitize 导出子命令候选(未立批)。

## §28.10 Wave-3.3a INODE 收账(2026-07-09,推 main;§28.6 施工图四件套闭环)
交付:引擎 WindowStats.TopIOInodes(截断前全量 (dev,inode) 折叠消 PID/op 维;Count=全 IO 族事件频次[实施裁定:只计 file_io 则东湖 mm_filemap 侧漏平台恒空榜],分解字段全保;延迟=单次 MAX+top-3 线程内求和,墙钟红线负向 pin;Count→Bytes→MaxLatency 序;top-10+TotalGroups+UnidentifiedEvents 双诚实披露;(unknown,unknown) 不折叠)+暴露三面(banner events= 词形[复核 P3-1 词值同源修]+typed 观测族 12 note+系统补充挂位 order 65/配额 40→44 再裁定)+路由教学(视图矩阵+TierB 两句+description,ATOMIC 过检)+QFEnumeration external_observation claim form(复核 P2-1 定性更正:提示语面+**软性 citation-role 对齐可达面扩展**,方向保护性,新可达面三臂 pin 固化)。负向 pin:统计枚举面不入投影树/rank。13 突变咬红(含"延迟臂偷偷改求和"与"折叠输入截断"两最强形);API 中断 SendMessage 唤醒无损续跑,缝隙自查零残片。
**留观/候选**:dev-unknown 同 inode 合并伪身份(窄可达)/claimKey 建议携 dev 维/threads 键族归属卫生/inode= typed 过滤参(可选项,如需立小批)/成员 TotalLatency 含 completion 行 latency 的 legacy 口径继承。
**队列**:3.3b CAP-2+THERM(即开,最后立案批)→3.3c DIAG(§28.11)。

## §28.12 TDIAG 裁定(用户 2026-07-09):内置 trace 诊断收集命令簇
客户同意提供 79 系式回访转录+命令结果(单结果≤1k 行,暂不脱敏)。裁定=codrax 内置**确定性诊断收集命令簇**(取代自然语言定向指令——NL 走 LLM 管线路由不确定/耗 token/需模型凭据;内置簇直连 tracequery 引擎,确定/可复现/免 LLM/行数硬界):
- CLI 形:tracediag 模式(flag 簇),输入=trace 路径+**收集脚本**(YAML,复用既有 yaml.v3 唯一依赖;步骤=label/view/pid/thread/window/line 区间/pattern/event_types/max_lines),输出=单文件文本报告(步骤头+每步行数帽默认 800 硬帽 1000+截断诚实披露+版本/flavor/窗口 provenance 头)。
- 工作流:我们按悬案作脚本(如 collect_g12.yaml/collect_d10.yaml 随仓库 examples 出货),客户 `codrax --tracediag <script> --trace <file> --out <txt>` 一条命令收全,回传单文件。
- 红线:纯读模式、零 LLM 调用、不触 L1 读管线字节恒等(独立 CLI 路径)、不修改 tracequery/tool(引擎只 import 消费,渲染在新包自建证据面——与系统补充同族的原文 token 风格)。
- 验收包(§28.11 文档)随批更新:定向指令包改为 tracediag 命令,NL 形降为无新构建时的回退。

### §28.12 补充裁定(用户 2026-07-09):TDIAG 增格式普查面(不留 trace 格式盲点)
诊断簇除个案取证外,增**格式盲点普查**步骤类(format census):从客户 trace 收"支撑后续扩展与优化方向"的观察面——全部聚合统计+有界样本,回传体量小:①事件名全谱(逐名计数+解析分类,**unknown 分类 top-N 名单=盲点清单**,hmfs_ 类漏采即此面自动暴露)②标记形普查(B|/E|/S|/F|/C| 计数、H: 前缀占比、|I 尾形——async span/counter 事件支持盲点)③clock_set_rate 轨谱(轨名×异 cpu_id 数×值域×计数=CAP 证据发现面)④调度域(prio 直方含>139、prev_state token 集、next_info 覆盖率)⑤FS/IO(前缀谱+ino/dev/entry_name kv 覆盖率)⑥电源事件覆盖(frequency/limits/idle per-CPU)⑦span 普查(top 名+语义 near-miss 名单+H: 占比)⑧行级(总行/不可解析行数+有界样本/时间轴健全)。全部 top-N 帽+总数披露(截断诚实)。若引擎缺计数 API:TDIAG 批先列缺口清单,CENSUS 半场随 CAP-2 落地后补(引擎域并发约束)。出货 collect_format_census.yaml。

## §28.13 TDIAG 收账(2026-07-09,推 main;§28.12+补充全量闭环)
交付:--tracediag 确定性收集模式(纯读/零 LLM/initApp 旁路 L1 零接触;strict YAML 全家桶 fail-loud;步帽 800 硬帽 1000+总帽 5000 两侧诚实披露;反射证据面渲染 map 全排序双跑 byte-equal pin;秒坐标定点 6 位/ms 对齐引擎 %.3f;--out temp+rename 失败不碰旧报告;冲突 flag fail-loud+日志 flag 披露)+八面格式普查(unknown 事件名盲点清单/标记形 S|F|C|/clock 轨谱/prio>139 直方/FS kv 覆盖率/电源覆盖/span 普查/行级健全;census 步 window/line scope 过滤)+四出货脚本(g12 五步 pattern 采 D 段起点行闭合判定点、d10 thread-only、acceptance、census;单文件预算≤950 机械 pin)。复核 P0=d10 pid+thread 同设采错线程(引擎 pid-first)——脚本修+装载层 fail-loud 三形堵death;P1=g12 对半拆行区间预设"不重合"伪证(改全区间+pattern 采样)+浮点科学计数法;P2=out 先建后验/确定性 pin 缺失/冲突 flag 静默。17 突变咬红。
**缺 API 清单(CENSUS 引擎半场,3.3c DIAG 并批候选)**:view 枚举器未导出/流式全 trace 事件迭代器(StreamScan)/语义 near-miss 判定未导出/Index.UnparsedSamples typed 化。**留观**:scope 语义分叉(census=window∧lines 交集 vs 引擎 lines 覆盖 window)/反射单行字节无界/摘要总帽虚报/padding 镜像已字面 pin。

## §28.14 Wave-3.3b CAP-2+THERM 收账(2026-07-09,推 main;§28.4/§28.5 裁定全量闭环,战役立案批全部完结)
交付:Tier-1=既有 CFR-2 共动推导补强为合格算力证据(样本下限 floor=2 capability 车道/类排序 fmax 治理梯 limits>rail>observed——witness:observed 序曾把中簇 1744000 误判大核)+Tier-2=cluster_rail_evidence.go 六门(⑥负向筛九词含 temp/tsens→①族形≥2→②键控异锚(hmtrace 位置形诚实回退=flavor 中立)→③全 trace 恒锚不变式→④kHz 量纲带→⑤锚集⊆调度 CPU 集;成员锚点连续推定;>1 过门族歧义整体回退;首簇前空洞不归属零猜测)+交叉验证(正向矛盾整弃 Tier-2/vacuous 不定罪)+细分(嵌套才切/横跨=structure_conflict 弃)。三级披露词 zh/EN verbatim+图例;typed 全链(fold_cluster_topology/gated_cluster_topology hard+fold_rail_basis 审计);explicit/legacy 字节保全负向 pin;THERM 热限披露(锚集⊆单簇归属+窗内 min+精确<fmax 门,零权重双 trace 恒等 pin);pid_freq 四假设量纲筛(10240923 源头消除;190091 类存活仅入"单位不明"caveat 软面);§7.10 增臂演化(原禁令逐字保全+例外臂三探针,idx3 空转已修)。19/19 突变咬红(含 thermal 伪族/heca_info/soc_temp 三形)。复核=零行为级缺陷。
**留观/裁定候选**:split 域未采样核 class 继承 vs 锚推定披露不一致(角落形)/THERM 同簇 limits 不一致取最紧(方向多报)/fold_rail_basis zh-only(G10-EN 债+1)/双过门族择一/floor 触发 Tier-2 补救/"limit/cap/budget" 负向词观察。
**战役状态**:§27 20/20+§28.1-§28.6 全部立案批完结;余 3.3c DIAG(§28.11-3+TDIAG 缺 API 引擎半场)=收官批。

# §29 首轮回传对账(2026-07-09,客户 tracediag 六报告:census a/c 成功+b/d 撞预算,g12 五步,d10 两步)

## §29.1 两悬案裁定
- **G12=同段双归属实锤(间接三证)**:①hmfs_discard-26-562 全区间 prev_state=D 切换**零命中**(其 IO 等待=S 态+sched_blocked_reason iowait=1,平台语义新发现);②oney 窗2 D 态合计仅 0.011ms(rank#4 行印证);③两成员 14.272ms 同值到 μs 而两线程物理上都不存在该量级 D/iowait 段——两条观测必为同一底层区间集的双端点重复计算。**立案 G12-ENG**(排 DIAG 后):critical_blocking d_state_or_io_wait 车道对 S+iowait 形的跨线程双归属修根;合成 S+iowait 形可本地复现,不再需客户数据。DIAG-A1 同值披露落地后回传输出可直接showcase。
- **D-10=两口径各真、来源不同,机制闭案**:timeline 双账本显示 per-interval actual_duration 仅窗裁剪区间携带(状态级 actual);79 引用的 actual_total=112.234=线程级观测合计(state_churn 合计),表面 59.050=running 单态 actual——非数据错误,是双口径并存无互斥披露。DIAG-A2 typed 披露即终态修。**TDIAG P0 修复实战验证**:thread-only 选择子正确解析 tid 16816(tgid=16547 双账本)。
## §29.2 格式盲点清单(census 四 trace)
- **BLIND-1** `print 引擎分类=unknown` 868/1090 条(两 ftrace 各 ~2-3% marker)——非 B/E/C/S/F 载荷形未识别;census 需补"未分类 trace_mark 载荷形直方"子面定位具体形。
- **BLIND-2 InternTable 锁风暴形**(xuewen 新场景 trace):"Lock contention on InternTable lock (owner tid: N)" ART 形 5600+ 条、数十 owner——**不在 monitor_contention 解析形内**("monitor contention with owner"形),锁车道新格式立案(xuewen 场景大概率是下一个 case)。
- **BLIND-3** C| counter 事件 1731/1323 条(占 marker ~4-8%)——counter 车道消费=候选;S|/F| async span 存在但稀少(≤8)。东湖 trace 出现 span 名 "20"/"10"(154/116 条)疑 B| 载荷退化形,留一探针。
- entry_name=0/72194——本平台 FS 事件全经 mm_filemap 无文件名,inode→路径 entry_name 途径死账(§28.6 候选降级)。
- cpu_frequency_limits 仅覆盖 {0,2,10}(prime 簇 12 无 limits)——CAP-2 治理梯 limits>rail 的 rail 臂在 prime 簇为唯一治理源,两级梯设计被生产数据印证;m3_cN 恒锚 {0}{2}{10}{12}+thermal_i2_cN 新索引热族/shell_frame_temp/ptr_budget_level/pi_boost_load 全部被六门+负向筛正确排除(真轨 zoo 生产验证)。
- prio 直方:301 系(万级)+负 prio(-1/-2)大量+X 态存在——现分类保守正确。
## §29.3 生产 witnesses(新机制客户产线实锤)
core_class=small/middle 入 window_stats(CAP-2 簇判定生效,"簇结构不可判"在东湖 trace 消失)/data_gap "no rank seat" 措辞/tracediag 全链(census 92-95 行、g12 零命中步诚实 caveats+流式 160 万行扫描、d10 双账本 350/1243 帽披露)。census b/d 撞 250K 索引预算=DIAG-B2 StreamScan 的活证(落地后全文件普查脱预算;过渡=census 加窗)。
## §29.4 账面更正(如实)
79 审计曾判 opendir 成文 "inode 0x6a16/约153ms" 为编造——**更正**:io_pressure 行 top_inode=0x6a16 d_state=157.402ms 为真实引擎 token(explore 期 window_stats 面),非幻觉;PSG-G14"终面证据外禁引用"裁定不变(该值未入终面),但性质=越面引用非编造。
## §29.5 队列
DIAG(在途)→G12-ENG+BLIND-2(引擎,DIAG 后)→BLIND-1 census 子面+BLIND-3 counter 候选(裁定池)。等待:四场景回访输出(验收对账主体未回传)。tracediag 观察:thread 过滤含发射者 tgid 命中(原始采集面可接受,读法注意);tppmgr data_gap 同线程双条(dedup 候选)。

## §29.6 Wave-3.3c DIAG 收账(2026-07-09,推 main;战役代码面收官批)
交付:A1 同值折叠成员披露(三跨线程取最大点全装:溢出折叠构造器/R3/wire causal-impact;tie=|v−max|<0.0005ms 帽4 零权重;typed+note same_value_members/same_value_lines(与 member_* 族避撞)+审计 token+图例;空 Subject 双侧对称滤)+A2 actual 口径互斥披露(单判定点三 lane,闭集 state_segment_vs_thread_total,>10% 门;明细「实际口径: 状态段/线程合计(两口径,来源不同)」;actual_total 升 hard)+TDIAG 四缺 API(CanonicalViewNames/StreamScan 零第二解析器全文件普查脱 250K 预算/语义类+near-miss 导出("不支持面"清单清空负向 pin)/Index.UnparsedSamples typed 帽5 rune-safe)。19+ pin,7 突变咬红。
**复核存档**:E23 归属排他性证明=wire 折叠 record Predicate 恒 "wakeup_causal_impact" 不可能产 critical_blocking E# → E23 根点=投影溢出折叠构造器,披露落正确链且双臂全装;A2 三 lane 源真(同源臂结构性静默非空转);B2 与索引路径逐语句同构。
**立案**:P2-1 第四跨线程取最大点(query.go:8424 引擎 top-8 aggregate 修剪折叠)无逐成员值/行区间→tie 不可披露——**并入 G12-ENG 批**(修法=fold 结构携帽4 tie roster,代码注释已指路);P3-4 census 累积 map 基数脱预算后随全文件增长(GB 高基数 span 名形,基数帽+披露候选);P3-5 样本子串钉底层数组(帽5 有界最坏 ~10MiB,strings.Clone 候选)。
**战役状态**:§27 20/20+§28 全裁定+§29 首轮对账全部完结;开放=G12-ENG+BLIND-2 引擎批(下批)/BLIND-1+BLIND-3 裁定池/四场景回访输出待回传。

## §29.7 用户双裁定(2026-07-09,792 回访审计中)
1. **锁形泛化=owner-tid 键形匹配**:锁竞争 span 识别不再枚举前缀词形("monitor contention with owner…"/"Lock contention on … lock (owner tid: N)"),泛化为**span 内 "(owner tid[:=] <N>)" 键形**——owner tid 括注是承载信号(键控精确形,类比 cpu_id 键控簇轨),前缀文本是运行时自由词汇。设计:泛化臂=span 名含 "owner tid" 字面+分隔符[:=]+整数→铸 monitor_contention 族候选(owner tid 直取=payload-direct;锁描述=span 原文 verbatim;无 at/blocking from 段则字段空不造);既有富文法臂("monitor contention with owner"携 holder_site/waiters/blocking from)保留为优先臂,泛化臂为兜底;ART 形及未来厂商形自动覆盖。并入 G12-ENG+BLIND-2 批(已转达)。
2. **语义类可加冕主根因(§23.1 演化)**:texture 场景的要义就是根因里点名 "Texture upload"(真实场景即凭此 span 找到优化点,类比 JIT/VerifyClass/Shader)。裁定:语义类行构成链上主导工作量时**应当**加冕主根因并点名语义类;§23.1 原"MUST mention as optimization point — never as the root cause"演化为"优化点身份保留,可为主根因"(EVOLUTION RECORD);792-textup 输出的"主根因: Texture upload"=正确行为予以追认。波及演化(SEM-LEAD 批,排 G12-ENG 后):skill TRACE SEMANTIC SPAN 条款措辞/工具 description "never as the root cause itself" 句/evaluator 头条选举语义排除臂(GAP-C P1-1 形)/LEAD-SEM 负向 pin——全部按裁定演化,不降 bar。**textup 其余三 gap 不在追认范围,照修**:E9/E13 语义族双席(语义 lane+rank lane 同 11 span 两行)、E13 行1 用最大成员名非类名、有效归因 214.561=score 乘子(2.10)泄漏为表值+semantic_multiplier/hidden_cost_boost 内部 token 直出正文(红线)。

### §29.7-2 精化(用户 2026-07-09):链上语义类=无条件全权参赛
去掉"构成主导工作量时"前置——**on-chain 语义类行必须能参赛且有机会登顶**(与其他成因同台,按有效归因竞争);即便未进 TOP N,仍必须作为优化点被提及(§23.1 链上提及义务=无条件地板,维持)。理由:这些是客户经验实战验证的确定性可优化点。非链上语义行维持背景综合排序+提及门 background_rank≤3 不变。
SEM-LEAD 批施工图定稿:①显示 board/lead/❶❷❸ 对 on-chain 语义行全开(runtimeTraceProjExcludeSemanticSpans 语义排除臂按裁定演化,tier 词"确定性优化候选"身份保留);②参赛值=家族真实合计(如 102.172),score 乘子(2.10)留引擎排序内部——同批修 214.561 表值泄漏+semantic_multiplier/hidden_cost_boost 内部 token 出正文;③E9/E13 语义族双席合一+行1 用类名"Texture upload ×N";④evaluator 头条选举语义排除臂(GAP-C P1-1)反向演化;⑤skill 条款/工具 description "never as the root cause itself" 句演化为"全权参赛+提及地板";⑥LEAD-SEM 负向 pin 按裁定演化(EVOLUTION RECORD 引本节)。

## §29.9 HTML 辅助信息 UX 裁定(用户 2026-07-09)
"树读法"/"各列口径"类辅助块过占重要信息空间(用户大部分时间不看,偶尔参考)。裁定=HTML 渲染层折叠:辅助块默认收起为一行可点开摘要(<details> 语义恰合"偶尔参考"),内容小号灰字;**只动 preview 包 HTML 面,Markdown 正文与终端渲染零改动**(与 §28.7 HTML 字体批同层先例)。识别=精确标记段("树读法:"/"各列口径:"等闭集)+紧随列表整体包裹,非标记段零影响。UXAUX 批实施。

### §29.9 修订(用户 2026-07-09):折叠块否决
<details> 折叠方案不采用。改为:HTML 渲染层把"树读法:"/"各列口径:"辅助块**移位至文末「阅读参考」附录区**(小号灰字渲染),原位置留一行小字指路("树读法与各列口径见文末「阅读参考」")——重要信息(覆盖句/树/表/明细)前移占据主版面,参考信息殿后随查。识别闭集/只动 HTML 层/Markdown 与终端零改动等约束不变。

# §29.8 四场景 792 回访总账(2026-07-09;构建=13:10:29Z 含至 CAP-2,不含 DIAG)
## 用户点名异常裁定
cmp"3/4 补齐校验信息后重启探索"=**正常设计内修复环**:Tier-1 证据闸(checkTier1Floor)拦下仅 window_stats 级弱证据答案→回探定向钻取(热点窗 rank/bundle),最终投影核心证据全部产自回探轮,一次收敛(↻ 仅一次)。**伴随 gap(P2,CSP#63 生产 witness)**:retry 指令对纯 trace 会话点名 repo_map lenses(结构性不可满足);三道抑制臂(zero_current_source_repo/runtime_observation_closure/TraceQueryRuntimeObservationCount>0)全未命中+authority keep 臂序前置——CSP#63 开批就绪。
## 验收横切(四场景)
修复实锤 20+:G1 链上并入(opendir 6 条 absorbed+E# token)/页缓存三面同值+计数当量/等待点行+撤回中文+移交链(线程)/盲区零榜位+判据二措辞/自因子行消失/孤行拆词消失/词值同源(行1)/binder_wait 自身席可见/G20 同值折叠/G19 零时长折叠注/top_io_inode(东湖)/TEX 全链+优化点表/CAP-2 Tier-1 共动分簇 Android systrace 判出+fail-loud 臂/CAP 恒等式全平/真实链深/零乱码零 EN witness/弱模型修复环一次收敛。不可判 4(huadong 窗口不同:io 家族/hmfs 边/全零折叠/E23 对照形未触发)。
## 新 gap 立案(792)
**P1**:G6-b 症状行泄漏链车道形(huadong 6.661/cmp 三处——tier 门只盖 rank 道,wakeup_causal_impact 道自身 sleep 行无 tier token 旁路;E16 承自链需保)。
**P2**:①拆解行"原始"分量取行值非引擎 raw(cmp E8/E10 同块两个"running 原始"自相矛盾;G7 拆解行漏面)②E7 rank 合并行三面不可对账(窗口投影恰=2×有效归因,occurrence 集不可调和;P0-E 双发布族新形)③序数洞+周期源榜位缺席=aggregate top-8 折叠吞携榜席成员(两场景实证打破验收判据,升格 ORD 批)④回探指令失配(CSP#63)⑤textup 覆盖句分母排除目标 sleep(单窗 CWD 守卫过火)⑥huadong 成文三错绑(编造线程名 dh-irq-bind-4-93/139.615 跨线程和张冠李戴/VSync 50.756×8 不可定位且矛盾)+cmp 旗舰句方向错位(供给 8109.844 vs 投影❶反转)——G13/G14 残差 witness 密度足,提请硬化裁定复审(cmp 转录=标本)⑦opendir 树层级扁平化(blocking_span 行无链身份被域门拒树位;富层级 vs 诚实席=裁定候选)⑧E22 ◇席窗标缺失(回归形)+区段行"累计(跨线程)"误挂单线程行(cmp E23 同族)。
**P3**:E19 跨窗折叠漏拒%/E7 ⚠消失回归/E1/E12 实际状态列=单次最大错配/树头"单项最大"29.298 实为和/背景榜位#1 双行无窗标/重叠窗语义族双发布无互斥注/opendir 反引号整段落 metric 块形+嵌套反引号破损/"阻塞等待"行形态词自相矛盾/自身区跨窗行无行级窗注/"system cpu_pressure"token 直出+错别字/E21 ×2 同值 81.301 疑云(DIAG A1 下构建自答)/D-4 时长宣称形不入 lane(裁定候选)/症状 span(bindApplication)持榜=SYM scope 裁定候选。
**流程**:huadong 转录尾截断(系统补充块缺失)→请客户补传。
## 批次队列(792 产出)
在途:G12-ENG+BLIND-2(owner-tid 泛化)/UXAUX(§29.9 修订形)。新开排期:DISP-3(G6-b+拆解行原始+窗标/词值/⚠/%/覆盖句分母/反引号块)→ORD(引擎:aggregate 折叠吞席修根+E7 2× 账目)→CSP#63(回探臂)→SEM-LEAD(§29.7-2)。裁定池:树层级(blocking_span 链身份)/D-4 时长形/症状 span scope/G13/G14 硬化复审。

## §29.10 三裁定(用户 2026-07-09)
1. **TRUNC(P1,立批)**:huadong_792 尾截断=系统无输出非客户剪切——答案尾段(保留内容段中部起+系统补充整块)未产出。排查面:终端渲染流帽/输出 dump 帽/W1 流式总上限对超长答案(多窗+保留段+系统补充=最长形)的误伤/REPL 打印链。witness=cust_trace_huadong_792.txt 行 926 "..." 截形。
2. **G13/G14 硬化=PSG-2H 设计(用户委托裁量,采纳)**:不作多轮硬拒门(37 连拒教训,livelock 红线)。三层:①检测=精确信号(数值 token/线程名 token 对终面证据拼写集的字面匹配——含新增线程名实体面,witness=dh-irq-bind-4-93 编造形);②命中→**恰一轮强制定向重写**(retry 指令逐个点名违规 token 与其应对面,单发帽);③二次仍违→**确定性 caveat 兜底放行**(typed 披露车道列"以下数值/实体未能定位于证据面:…",系统不改写正文,不再循环)。成本=仅违规时+1 成文轮;最坏形=披露放行,结构性无 livelock/无硬卡。评审复审语境的 witness 全表(79+792 两位数)为验收基准。
3. **多投影排版**:多份"Trace 因果投影"并存时,**投影树(含头/覆盖句/关键指标)依次全部优先显示,因果明细依次殿后**——当前树/明细交叉排版非最优。入 DISP-3 批(E# 交叉引用天然兼容)。

## §29.11 CAP-3 裁定(用户 2026-07-09):频率状态语义+拓扑 trace 全局化
用户指正采纳:①**cpu_frequency=独立状态泳道**——窗内频率值由窗前最近变化点携入(carry-in),"窗内无采样事件"≠数据缺失;核查 CAP 折算 lane 全路径(residency 面已 carry-in,fold/deficit 面"缺失计 0 下界"臂逐个核,把状态语义错当事件语义的臂修正);②**簇拓扑=trace 全局属性非窗属性**——共动推断基改为全 trace(同 Index 全事件流),判出一次全窗共用;floor 门作用于全局基(census 见 87+ 次扫,实际几乎恒可判);huadong"同 trace 异窗一判一不判"分叉即此机理,witness=huadong_792 两窗不可判 vs g12 窗判出。诚实边界不变:全 trace 零变频→默认表;某 CPU 窗尾前无任何变化点→该 CPU 真未知不猜;**跨 trace 移植禁令(§28.4 补充)不变**。CAP-3 批排引擎道(G12-ENG 落地后);collect_cap2.yaml 回传数据=该批验收基准(修后两窗应同判)。

### §29.11 补充(2026-07-09,cap2_report 回传定案):不可判=车道分叉非采样不足
客户 collect_cap2.yaml 四步全成:huadong 两原窗窗内 cpu_frequency 各 862/926 条,window_stats 面两窗均判出 core_class(small/big),驻留段窗起点状态携入正常——"采样不足"假设**排除**。同两窗 huadong_792 折算行印"簇结构不可判"=**折算 lane 与 window_stats lane 解析基分叉**(折算基疑在切片粒度上无携入取事件)。CAP-3 施工图精化:①折算 lane 的簇域/频点解析统一到与驻留面同一状态时间线(携入语义);②拓扑 trace 全局判定(§29.11 原文);③验收=同 trace 全窗折算词一致(cap2 四窗对照即基准)。
**cap2 顺带两观察(入 ORD 批语境)**:①同线程 runnable_wait 按 CPU 拆行各占席(#1#2#3 同一 OS_FFRT_3_45387,cpu=1/3/2)——runnable_wait 不在 §24.7.1 家族折叠车道=漏折候选;②aggregate 行+其两 occurrence 成员行三席并占(#4=occurrences=2 聚合 12.401,#5#6=两成员 6.236/6.165)——rank 车道聚合/成员双席形(GAP-B P1-1 修的是 trunk 显示半,rank 席位半未盖)。

## §29.12 裁定(用户 2026-07-09):finalizer 修复轮硬上限
finalizer 多轮修复=大量 token 交互反复且小模型输出不稳定(后轮可能劣于首轮),**必须次数上限控制**。设计(FRCAP,与 PSG-2H 合批):
1. **全清点**:枚举 finalize 期全部重试/修复环(evaluator retry-hint 环/oracle 拒稿重写/finalizer_auto_repair/contract 修复轮),逐环登记现行上限与来源(硬编码=违 ShouldStop 两段式红线,一并整改为 config 驱动)。
2. **硬上限**:codrax.yaml 新知 knob(指针型,两段式 cap 纪律),保守默认(如成文修复总轮数≤2);到顶即止,**永不无界**。
3. **最优稿回退**:到顶仍有违规→不再重试,按精确信号选最优稿出厂(硬违规数最少者;平手取**最早稿**——采纳"后轮未必更好"判断)+确定性 caveat 披露残余问题(与 PSG-2H 兜底同型:系统不改写正文,列明未定位项)。
4. 轮次遥测日志(每答案落"修复轮 N/上限 M"),回访转录可审计。
排期:TRUNC 批(同域 finalizer_auto_repair 等)复核收尾提交后立即开 FRCAP+PSG-2H 合批。

## §29.13 G12 勘察修正(2026-07-09,G12-ENG 批编译级复现定案;修正 §29.1)
**§29.1"同段双归属/双端点重复计算"结论作废**——引擎从未双计。E23 真相(逐字节复现产线形):14.272ms 唯一真身=目标线程自己的 5 条真实 binder 等待求和(1.035+2.215+2.918+3.527+4.577,R1 三副本合并+R2 ×5 聚合成单条 oney 自症状行,归属正确);hmfs"成员"=×4 全零时长 sched_blocked_reason 标记行(S+iowait 平台形留不下时长);**伪造点=显示层溢出折叠的 min-max 只统计有值成员而 ×N 计全员**,渲染成"×2(14.272–14.272)取最大"凭空给 hmfs 铸出第二个 14.272。三个引擎侧候选机理(互指/双端点/pid 错位)全部排除并负向 pin(S+iowait 永不入 D/iowait 时长车道)。修=显示层混合形如实"×2(有值1项 14.272ms,1项无时长值)取最大"+typed MergedValuelessCount+图例「无时长值」+审计 merged_valueless=N;DIAG-A1 tie roster 排除零值成员;发布数值零变动只降不升。§29.1 的 S+iowait 平台语义发现保留(已 pin)。**教训:审计推断"双归属"时未先区分成员的有值/无值——折叠行 min-max 与 ×N 分母不同源是伪造温床,同源 pin 已装。**(验收回注 2026-07-11:已于 §29.28① 关账——cust710 g12_report 生产二次确认:hmfs prev_state=D 匹配 0 行、oney 6 条真 D、hmfs 全 S+iowait。)

## §29.9 收账(2026-07-09,UXAUX 推 main;复核 SHIP 零 P0/P1/P2)
交付:HTML 层辅助块(树读法/各列口径,双冒号字形闭集)移文末「阅读参考」附录(小号灰字+上边线)+原位一行指路(相邻合并)+字节级去重(源字节区间键,结构变体不误合并)+**来源标签**(最近前置 heading 文本,多投影 4 块近似图例不再匿名;复核实测 cmp_792 两套图例 42vs40 条字节有差=按需生成活物,去重不命中系正确行为);Markdown/终端零字节改动;opendir 首屏省 ~48 行。复核收尾:fallback 护栏一致化(越界 panic 形 pin)+门语义注释如实(扁平文本恰等,包裹形搬移无损)。**留观/裁定池**:双 List 分裂形(发生器结构性不可达,留档)/EN 车道闭集扩展(zh-en 同词先例,下轮议程)/指路行锚点链接 nice-to-have。

## §29.14 TRUNC 收账(2026-07-10,推 main;P1 客户面尾丢失修根;复核=产品代码零缺陷)
双病灶(witness 逐字节吻合):①保留段 16000 rune 裸切帽(截形 "..."+"---" 三行对上代码)→帽 200000+显式双语披露("原文共X字符,此处仅显示前Y字符"),裸 "..." 根除;②挂第一稿附件/两处 auto-repair 的 FinalAnswer 覆写直调渲染丢全部 last-mile 系统补充块→单一咽喉重渲染(agent 导出 parseOutputV2 同管线),三覆写点收编+hedging 镜像同判。截断烙在组装层故 dump 同愈(终端+落盘两面)。复核实证:双重追加形结构性不可能(补充从 doc 重算/库内恒未 hedge/4 轮覆写 Count==1 零漂移 pin)+L1 恒等;收尾=短附件逐字节 pin(311 字节全段)+Count==1 负向 pin。
**立案**:P3-2 ApplyAuthorityHedging 非幂等+recovery lane 先 hedge 后入库(marker 叠加可达形,当前结构不可达;幂等化后批,代码注释已指路)。**留观**:结构 pin 文本扫描可绕形/首稿附件含自身补充块的双份观感(归 DISP 排版)/diagram 空 normalize 披露行。验收=客户同构建重跑 huadong:保留段显式披露+系统补充块回归。

## §29.15 G12-ENG+BLIND-2 收账(2026-07-10,推 main;复核=E23 定案独立确认)
交付:①E23 显示伪造修根(§29.13 定案):typed MergedValuelessCount+混合形词"×N(有值M项 a–b,K项无时长值)"三面同源(fence/(b) 明细/图例,复核抓获 CWD 臂/union 臂/R2 求和臂三漏面全补)+standalone 全零 R2 行"×N(全部无时长值)"禁 0.000 伪值+引擎负向 pin(S+iowait 不入时长车道/跨线程同值对绝迹/blocked_reason 恒零)+DIAG tie roster 排除零值成员(微值带守护 pin 补齐);②owner-tid 键形泛化臂(富文法臂优先;tid 0/max 哨兵无主;InternTable census verbatim+非 ART 合成形 pin;泛化回退保 monitor 分类顺修);③第四取最大点 tie roster(复用 same_value_members 既有 key 零新 key)。
**留观/裁定池**:fold 键无 wait-object 身份(不同锁描述可折一行,泛化臂扩大可达域)=裁定候选;自相矛盾守护对 payload-direct 误铸兜零=§29.7-1 设计内留档;strict 臂 TrimSuffix 边角构造形;wire folded_* 再物化 valueless 通道(结构性不可达,注释 pin 指路)。

## §29.16 CAP-3 收账(2026-07-10,推 main;勘察修正=分叉真根在 carve 边界非"切片无携入")
**勘察修正(修正 §29.11 补充的施工推断)**:折算 lane 早有携入语义(governedFreqSamples 头样携入+frequencyAt 全索引二分),拓扑基早读全 Index——"折算基在切片粒度上无携入"假设不成立。真实分叉:①**主根点=共动判据 freqTimelinesSameEmission 要求字节级全数组同一,对 carve 边界脆弱**(cluster_freq_share.go;垫头/垫尾 ts 闸/MaxEvents 预算切/window 面 TimeEnd 切四种刀)——刀落多行簇发射中间(成员行距 1–4µs)即裂簇成同 fmax 双胞胎→平局/>4 簇 fail-loud→freq_only;cap2 witness 因果链同构:huadong 两窗 event_count=250000 恰触预算 vs g12 对照窗 244629 未触。②次根点=window 面 windowFreqSampleTimelines 以窗裁剪样本推导拓扑。修:**freqTimelinesCoMove 新判据**(旧全数组同一快路径字节保全 ∨ 状态语义修剪形:变化点去重→头部未见证修剪[junction 携入态一致守卫]→中段严格 1:1 零容忍→尾部豁免[恰 1 条+全局样本流尾 15µs 锚+对齐变迁 floor])+indexFreqSampleTimelines 单点收集器(window 面共用,窗裁剪收集器退役)+Index once-memo(与 fold cache 共享单基)。
**复核(SHIP-WITH-FIXES)P1 实锤收**:初版 merge 地板 aligned≥1 把**入场公告对当共见证变迁**——两真异簇全 trace 钉同值+首样本 15µs 内共现+cadence 异→假并,域级实测**类倒置**(活跃小核簇加冕 big 静默出厂,踩 §26 类映射伪造红线)。修=floor 提 2(clusterFreqTrimmedMinAligned=至少 1 次真共见证变迁;尾臂专用常量收编)+a1'/a2-head split pin+域级 witness pin(停泊双胞不融合/活跃小核永不加冕/平局禁掷币);残余代价形(真同簇仅 1 次共见证变迁+异 cadence)发射学不可达(同簇成员由单 notifier 循环发射,cadence 结构性同一)。**P2 收(验收定位面)**:真机窗归因=推断非见证(straddle 两窗同中招 ~1e-3;备选机理=中段成员行真缺失/迟发>15µs 在新判据下依然诚实 split=残余形①)。修=freq_only 降级披露携带 split 点三要素(判定臂 token+违例 ts+cpu 对;判据单实现返回诊断零第二判定副本;**仅披露/审计,永不作门**)+collect_cap2.yaml +2 步(cpu_frequency 全量 census+carve 垫头边界邻域原文行,头注含判读指引)——客户复跑一次即可判"straddle 已愈 vs 残余形①在场"。突变 5/5 咬红(floor 回退 1=五面齐红)。
**教训**:①放宽判据的 merge 地板必须只数"真共见证变迁"——**入场公告=共享状态非共同行为**,首对齐对不作证据;②验收依赖推断归因时,**降级词面必须携带定位三要素**,否则复跑失败无从判型;③字节级全数组同一判据对任何 carve/预算边界天然脆弱,"同 trace 异窗一判一不判"即其指纹。
**留观/裁定池**:gated-only 行(无 SupplyFoldBasis 载体)不携 split audit(需 WakeupCausalImpact 新 json 字段=裁定候选);audit 进 rank 步 typed notes 需注册表 key+R2' 六处同步(留升级);中流真分歧残余形①=诚实 freq_only 留档。验收=客户复放 collect_cap2.yaml:修后 huadong 两窗折算词应与 g12 同判;若仍 freq_only,split_audit 判型。(验收回注 2026-07-11:已于 §29.28① 关账——cust710 两窗 core_class=small 全判出+全报告零"簇结构不可判/按纯频率比折算"词=两窗同判,straddle 已愈。)

## §29.17 FRCAP+PSG-2H 收账(2026-07-10,推 main;复核 P2 根修收编 CAVSTR 批销案)
**FRCAP(§29.12 落地)**:finalize 期修复/重试环 18 环全清点——成文修复总轮硬上限=既有 pipeline_finalize_repair_hard_cap 预占检查(默认 2,cross-stage 口径:retryUsed 计入 explore fact/Tier-1 floor/SC 重投;finalize-only 子计数=裁定候选,会放宽 cap 语义须用户裁定);三处硬编码迁 config(pipeline_max_repair_attempts_per_root/pipeline_same_error_class_retry_cap/agent_finalizer_empty_blocks_breaker_max_streak,默认字节恒等,两段式 cap 纪律);**最优稿回退**=到顶不再重试,稿账本(渲染稿+doc 深拷贝+hash,cap+1 有界)按硬违规数最少选稿、严格平手取最早稿("后轮未必更好"),换稿判定比 hash 非索引;轮次遥测 "finalize repair budget used=N cap=M (cross-stage)"(标签口径如实,回访转录可审计)。复核裁定:违规计数公平性通过(池内全 strict root,advisory/soft 前置滤除;severity 分层需先立 typed lane=裁定候选);换稿无死循环面(被选稿在其自身轮已过检)。
**PSG-2H(§29.10-2 落地)**:散文数值+线程名实体面(dh-irq-bind-4-93 编造形 witness)精确检测+pair-sum 跨线程审计(全正向发布要求)+恰一轮强制定向重写(逐 token 点名)+确定性 caveat 兜底放行(latch 单发,系统不改写正文)。复核误伤面 5/5 零误伤(裸名无 tid/截断/前缀别名 tid 豁免/x86-64·utf-8·top-10 形/verbatim+recase);P3b 边界噪声形(thread inventory 在场时 "base-64" 被软重写)=留观。
**复核(SHIP-WITH-FIXES)P1 收:cap 预占的 PSG 违规零披露出厂(承诺证伪)**——ViolProseScalarUngrounded 无 CaveatFamilyID→generic materializer 链收敛 nil→cap 分支零字追加,恰是层③"最坏形=披露放行"的反面。修=cap 分支 typed 精确门+复用 PSG 自有生成器(latch 无关,zh/en 现成件)+与 ship 出口 lane 寄存器同 key 结构性去重+结构布线 pin(P6 分支体内必须存在 typed 门+cap-preempt 调用)。
**复核 P2 收(根修,收编 CAVSTR 批)**:字符串 caveat 通道被 attachFirstDraftReference 重渲染覆没(触发面=strict review 默认开+cap 场景 rejectedForRewrite 恒真=P6-cap 主线本身;§29.14 只救了 doc 系"系统补充"块,string 系无人救)。修=**pending-caveat 寄存器**(answer_caveat_replay_register.go:有序+条目 key 幂等去重+每任务重置)+renderFinalAnswerWithLastMileSupplements 咽喉每次重渲染后按登记序重放——任何 FinalAnswer 覆写点(首稿附件/auto-repair/recovery/FRCAP 换稿)不再能杀死披露;P7 通道分离保持(严禁回写 doc.Caveats);登记面全收编(termination 三类/P6+FRCAP 残余披露/user+soft contract bullets/PSG 两 lane),materializer 拆分字节等价。突变:寄存器重放禁用→三 pin 红;cap 分支披露删除→结构 pin 红。
**教训**:①"最坏形=披露放行"类承诺必须对每个违规 kind 逐一验证 materializer 链非 nil——无 CaveatFamilyID 的 kind 在 generic 车道上是静默黑洞;②字符串面追加必须挂重渲染咽喉幂等重放,否则每个 FinalAnswer 覆写点都是静默清除器(TRUNC 只救 doc 系的教训半径)。
**留观/裁定池**:finalize-only 子计数(裁定候选);hedging 幂等化批(§29.14 已立案);P3b 实体面边界噪声形。

## §29.18 DISP-3 收账(2026-07-10,推 main;792 显示面九项+复核 P2 假 ⚠ 修根)
**交付九项**:①G6-b 链车道症状泄漏=表"有效归因"gate 增第二臂(Kind==Self∧IsSleepState→"—",legend 同步;刻意窄:E16 depthless/自因族保值,witness huadong:273/cmp:386 等六处);②**多投影排版(§29.10-3 裁定落地)**=按 typed block id 分组重排(总览→各投影 lead+关键指标依次→各明细+证据索引依次→partition caveat 殿后),纯块序重排零行内容字节改动,单投影字节恒等,UXAUX 标签/E# 交叉引用实测不破;③E22 ◇席窗标回归修根(回归点=0960e41c G5 ×N fold 零化行级窗)=typed RankQueryWindowStartTs/EndTs 随供 rank 序数的最小 rank 成员 verbatim 携带+chip 回退消费(absence 不猜;非 LLM-facing,R2' 不适用,MergedMaxWindow* 先例);④累计(跨线程)词值=单线程 fork 三臂(≈窗口投影免注/×N 且 eff==MergedMaxMS→行3 文法「单次最大(a–b,共N次)」/残余裸词;判据=Round3 精确等值,1µs 分歧走保守臂);⑤E19 跨窗折叠漏拒%=溢出折叠构造器补铸成员窗 roster(复用 F-2 slot builder 无第二实现),CWD-2 显示门即时消费(huadong:234 伪 24% 抑制);⑥E7 ⚠消失回归=双 scope 带保护(b8762441)收敛为 Round3 等值 carve-out+×N baseline 改 MergedMaxMS(79vs792 双腿 witness 恢复);⑦textup 覆盖句分母=单一准入权威(已准入状态行无 sleep 族成员时最大 sleep 态 hop 视图入分母,MAX 非 Σ;窗基三守卫保 crossBase pin;textup:60 census 拒答→100% 全称句);⑧render scalar 分发=literal 含反引号/换行/中文句末族/". "→平文段落(零系统词,真 scalar 字节恒等,opendir:57/59 嵌套破损灭;L7/L8 零触碰);⑨拆解行原始(P2①)=running 分量 RawMS 优先序反转(引擎 fold raw 领先与供给折算行同源,行值兜底;cmp E8 同块 1.392/2.681 自相矛盾灭,共享子行模板单点修复 fence+明细双面同愈)。
**复核(SHIP-WITH-FIXES)P2-1 收:×N 合并行 dual-scope 种子戴假 ⚠**——merged actual 从种子 verbatim 携带永不再派生,种子 pre-merge 链总量被 SUM 覆写销毁,行级等值 carve-out 对合并行结构性不可命中(REPRO=berlin E2 dual-scope 种子+成员合并→假 ⚠)。修=typed MergedActualDonorCumulativeMS(merge 权威种子段携带 SUM 覆写前原值,R2/G5 fold 共用单一 merge body)+合并行成员级等值 carve-out+**absence 臂保守(donor 缺失→直接抑制 ⚠,宁漏勿假)**;fence tag 与表 ⚠ 同谓词不分叉。P3-2 收:覆盖句 hop 准入 residue 披露句(仅 hop 臂 engaged 且同资格候选输给 MAX 时非零;复用 census 句式 verbatim 零新词,不改分母不改判)。
**教训**:①"只可能少标不可能误标"类方向论证必须对合并通道逐字段核——**verbatim 携带字段与 SUM 覆写字段共存的行,任何以行级字段做的等值/比较判定都要单独论证成员级可达性**;②回归考古双腿法(79 原件 vs 792 同形对照+git log -S 定位引入 commit)对"⚠/chip 消失"类回归高效(E22 回归点=0960e41c,E7=b8762441 过泛化+0960e41c 掩蔽叠加)。
**留观/裁定池**:P3-3 dash 后非合并自身 sleep 行 effective 全报告不可见(G6 rank-lane 先例同形,记账);P3-4 Round3 与 %.3f 打印浮点边界理论形;P3-5 tie-rank(同 Rank 异窗)chip 取先见成员窗=欠披露裁定候选;单线程区段行 cum-lane/C00 回退词理论形(无 witness 闭集缺词);合并行 原始(SUM)配 计入(seed) pre-existing 形(identity gate 兜底);P3-6 块 id 不识别 _detail_full(pre-existing,已立独立 chip)。留 ORD:E7 2× 账目(P2②)/序数洞+aggregate 折叠吞席(P2③)。

## §29.19 ORD 收账(2026-07-10,推 main;E7 恰 2×=聚合/成员双席双计恒等式实锤)
**交付五件(纯 internal/tracequery)**:①**aggregate/成员席位排他**(=E7 2× 账目根,P2②/cap2 观察②同因):引擎对每 (pid,dominant_state) 组同时铸 aggregate rank 行和逐 occurrence 行,显示 R2 求和把同一 occurrence 集加两遍——**恰=2× 是恒等式**(cap2:756-761 12.401=6.236+6.165 逐位;cmp:391 11.804=2×5.902);修=rankSeatAggregates 先解析席位 aggregate,成员以聚合建群同源 key(wakeupCausalAggregateGroupKey 单函数双侧消费永不漂移)从 rank 铸行抑制,view 车道无损。②**runnable_wait 全族入 same_thread_type 折叠车道**(裁定核实:§24.7.1"通用规则,适用于全部可拆分参赛的 rank 类型族",cpu 即区分键;registry 旧注释被 cap2 生产 witness 证伪;五 token 全族同修 per-CLASS);口径守护=memberSegmentsProducerDisjoint 精确信号(computeOffCPUStats 单 open-segment 状态机保证同线程段两两不相交→Σ 合法,复用 sum_disjoint 词;**前提门=idx.ClockRegressions==0**,逆序流退 MAX 保守)+mint-source 入 fold key(wakeup_chain RootEvidence 与 window_stats top 同 token 同线程禁跨车道 Σ,双计防线)。③**top-8 吞席修根**:席位分配读全量 census(chain.rankAggregateCensus 修剪前存留,空则字节退化旧行为);top-8 修剪降为纯显示容量;越 top-8 席以「链上·未接入树」独立行渲染(生产 witness 已证)。④**周期源旁路(勘察修正:真吞点=isIntermediateSleepAggregate 中间睡眠跳臂无周期源旁路,非账本原归因的 top-8 折叠)**:typed PeriodicSource 旁路,周期源以 VS-1 折减 eff(min(RunnableMs+迟到量,raw)) 参赛,非周期中间睡眠维持无席;huadong E12 VSync 零席修复。⑤序数按修后席位连续分配+端到端 contiguity pin。
**复核(SHIP-WITH-FIXES,五件零正确性缺陷;含 git restore 事故重建逐落点审计 PASS)收尾**:P2-1 **lane 准入类型全域披露**——五 token 折叠不止 window_stats 四铸点,wakeup_chain RootEvidence 行(无 typed ts→退成员 MAX+member_sum 披露 raw Σ)同受波及;witness=cmp:396 E11(main-6565 sleep ×6,29.298→单行 MAX 14.561+member_sum=29.298)——方向诚实且**顺销 §29.8 P3"树头单项最大实为和"**;E11 形引擎实铸 pin 补装。P3-1 时序门/P3-2 四铸点 proof 普查 pin(复核自设突变抓获三点静默降值无绊线)/P3-3 两注释改真/P3-4 账目确数更正(pin=10 test,突变=14 次记录)。
**教训**:①"恰 2×/恰 N×"类精确倍数=同集双加恒等式指纹,账目侦破先做精确算术分解;②类型全域准入(registry 车道)的波及面清单必须在批报告全列——铸点之外的同 token 行类都会改行为;③agent 突变自查 git restore 事故再犯(重建后经复核逐落点审计通过)——**突变恢复只允许 cp 副本,派单必须显式写明**;④"结构性 one-row-per-thread"类注释断言会被生产 witness 证伪,注释不是裁定依据。
**裁定项(需用户)**:①d_state_or_io_wait↔io_burst_episode 同段双席(huadong 窗2 洞#8 真身,同线程同区间同 1.062ms 两席;跨 type 席位语义变更需裁定,候选=G1 式裁定表扩对);②同 token 跨车道(RootEvidence vs window_stats top)双席仍在(本批只装禁并防线,G1-类对账候选,无 interior-hole witness);③榜自席与树「其余N项(链上折叠)」roster 并存措辞(PTS 显示语义遗留)。**留观**:ORD×DISP-3 吸收行可现"有效归因(gated 复合)>窗口投影(running 主导)"两把尺形(各自诚实,入复放核对)。
**复放核对清单(collect_cap2.yaml rank_fold_basis 步)**:runnable per-CPU→单行 sum_disjoint Σ12.537+roster cpu=1/3/2;#4#5#6→单 aggregate 席;E11→单行 MAX 14.561+member_sum=29.298;VSync 周期源低位入席;每窗序数连续(洞#8 待裁除外)。(验收回注 2026-07-11:已于 §29.28① 关账——cust710 cap2_report_02 rank_fold_basis 步逐字命中:runnable merged ×3 combined=12.537ms sum_disjoint、优先级反转 aggregated occurrences=2 单席、VSyncGenerator occurrences=8 周期源入席 tertiary #5、序数 #1..#12 连续。)

## §29.20 CSP#63 回探臂收账(2026-07-10,推 main;词面 fork 修,闸判定/臂序零变更)
**测绘边界(多轮立案对账)**:生产侧零排放/donghu authority 自开门/negative_search 定性/case-fold 残口=均已由 70d0c0c1 等前批修(20260706 post-fix eval 生产 run 实证 satisfied=false+citations=0);本批=**回探臂失配半(§29.8 P2④)**。witness=cmp_792:77-79"↻ 3/4 正在补齐校验信息":retry 指令对纯 trace 会话点名"Missing repo_map lenses"(结构性不可满足),同 prompt 工件段又禁 repo 广度搜索,模型逐字点出矛盾;探针 HEAD 逐位复现=污染源为模型 aggregate facts 经 plain-RM 终端 fallback 铸 current_source→satisfied=true→keep 臂前置短路三抑制臂。
**修=回探指令词面 fork(tier1_floor.go)**:traceObservationDrillRetryLensActive 五 typed 条件 AND(trace-query 面激活/确定性 trace 观测>0/TurnA 零真源码 read_file/无 current_repo origin evidence/**无 typed 精确 current-source 要求**)→trace 分支点名 window_sweep→heavy views 热窗钻取(per-trace 窗措辞)+repo_map/read_file/"Source localization" 词面整体剥离;源码分支字节保全;闸判定/臂序/proceed 零变更(修复环=设计终态)。三抑制臂逐臂归因落档:arm1=如实 fail-closed 不修;arm0/arm2=satisfied 污染类,冻结待裁。
**复核(SHIP-WITH-FIXES,零假 pin)两收**:P1-1 lens 混合 run 误激活(perf triage ResolvedFiles 形=健康权威态+真实精确源码要求,lens 却说"repository search is not required"与保活理由矛盾→预算耗尽 fail-loud)→增合取 readLocalizerTier1CurrentSourceRequired(同文件既有 typed 信号;**复核纠偏:census 纯工件条件是错修**——arm1 已抑制 followup 使 lens 永不可达);P2-1 勘察修正=blob/工件读生产路径本就进不了 ReadFiles(readFileTypedSourcePath 单一判定,工件读 ReadCoverage 恒 nil)→covenant pin 钉上游不变量+消费侧路径谓词防御双层。conscious-flip 前提 pin 装妥(plain-RM 根修落地时 witness pin 先红提醒重读账本)。pin 8 test+突变 10 组(含复核抽查 2)咬红。
**教训**:①LLM-facing retry 指令的词面 fork,激活判据必须枚举"该词面会撒谎的全部 run 形"——本批漏的恰是 typed 要求在场的混合 run(词面与闸的保活理由矛盾=指令自缢);②复核对任务书修法建议要独立验证可达性(census 条件在 arm1 之后结构性不可达)。
**裁定项(需用户,CSP#63 残根)**:plain-RM(required-lane)模型 aggregate facts→current_source 终端 fallback 仍是 satisfied 污染活源头(cmp 形即此;同时喂 keep 臂与完成门诸消费者;CSR #64 P6 pin 冻结字节稳)——根修需专批,裁定点=纯模型断言是否允许铸 current_source 级 observation。

## §29.21 裁定(用户 2026-07-10):纯模型断言不得铸 current_source 级 observation
**裁定原文**:认可建议——plain-RM(required-lane)模型 aggregate facts 经终端 fallback 铸 current_source 级 observation 的行为废止。**纯模型断言不得进 current-source 证明车道供 satisfied;记录无损保留 advisory 车道(继续喂显示/软引导)**。satisfied 从此只认确定性工具见证(read_file coverage/grep/trace_query typed 观测)。
**依据**:①精确信号硬门红线——CurrentSourceSatisfied 消费方全是硬门语义(keep 臂短路/完成门/authority 视图),模型 facts=最嘈信号;②双 witness 实锤——donghu 20260703(satisfied=true+4 blob 伪引用出厂)与 cmp_792(3 条模型 facts→satisfied=true→三抑制臂全灭→retry 指令自相矛盾),20260706 修后 run(satisfied=false+citations=0+如实披露)即正确终态;③先例同构——CPD 已裁 negative_search"保留 ledger 不得定性进证明车道",模型断言比负向搜索更弱不应享更高定性。
**施工图(CSP-RM 批)**:源头改类(answer_evidence_origin.go 终端 fallback→advisory/model_claim 级 kind),非下游豁免(keep 臂/完成门加豁免=遮羞,拒);波及面良性=keep 臂不再短路→arm2 三子臂(sufficiency/deterministic/count)复活=设计判定树首次可达,followup 只在 trace 证据不充分时触发且带 §29.20 trace 词面,cmp 有益回探保留;repo run 有真实见证不受影响,"模型嘴说未读"run 被压去真读=防幻觉方向。配套:CSR #64 P6 pin+§29.20 conscious-flip pin 按 EVOLUTION RECORD 演化(引本节);exclude run 零排放单咽喉与 ExcludesCurrentSource() typed escape 不动;批内必做 authority/完成门消费方全谱普查(证伪"合法依赖模型 facts 单独撑 satisfied"的 run 形或列外);验收=donghu_real eval 复跑+混合 repo run 回归+cmp 形复放(followup 走 arm2 子臂如实判)。

## §29.22 SEM-LEAD 收账(2026-07-10;§29.7-2 六件套全量交付)
**交付**:①**board/lead/❶❷❸ 语义全开**(runtimeTraceProjRankBoard 语义 kind 准入臂=on-chain typed relevance ∧ Rank>0;lead 空 primary 桶时 board[0]=链上语义行可加冕(纯语义板形);runtimeTraceProjExcludeSemanticSpans 语义排除臂 EVOLUTION RECORD=排除≠禁赛,✦ 行即唯一席位;§23.1① 演化见该节附注);②**乘子泄漏修根**(引擎:EffectiveImpactMs 发布=家族真实合计/窗口投影,乘子改走 RankSortBoostedEffectiveMs `json:"-"` 内部排序道(rootCauseEffectiveImpactMs 新臂+Score 同源),Summary 的 semantic_multiplier=/hidden_cost_boost= 内部 token 摘除——214.561 表值与 token 直出正文双泄漏同根灭);③**E9/E13 双席合一**(display 半场:runtimeTraceProjFoldSemanticRankLaneTwins,RNB R2 同构车道——join 键=canonical subject+typed SemanticClass+行号包络,守卫=值镜像等式/成员数镜像/跨窗 veto/歧义 fail-open,scope=**仅 on-chain 对**(非链 E10/E18 形维持两席=§23.1 后半不变);rank 席转移到 ✦ 行(行2 根因排序#N),E# 走 RankFoldPeers 既有词汇([E#+E#] 并入括号+明细"已并入本行,数值不重复计入"+W-B MAX 不变量));④**行1 类名**(三名面同修:tree 行1 generic 臂+关键指标表 node cell+明细完整名称——family∧SemanticClass typed 门→类词,成员名保留 roster/成员行);⑤**evaluator 反向演化**(traceProjectionHeadlineNode 语义排除臂收窄为非链 lane,rank-funnel 语义行同门(SemanticClass token);GAP-C P1-1 pin EVOLUTION+新正向 pin TestSemLeadHeadlineElectionSeatsOnChainSemanticRow);⑥**skill/description/pin 演化**(skill TRACE SEMANTIC SPAN 条款="全权参赛+可为主根因+类词点名+无条件提及地板",tool description 两句同改,"never as the root cause"负向 pin 双面(skill+description)防复活;LEAD-SEM 禁"主根因:"/禁 LeadKey 负向 pin scope 收窄为 tier-4 fallback lane(EVOLUTION RECORD 注))。
**与 ORD 关系(异因判定)**:E9/E13=跨观测通道双席(trace_semantic_span 通道+root_cause_* rank 通道两条 record),非 ORD 的引擎同通道 aggregate/occurrence 双铸——修在 display fold(RNB R2 同构),ORD 席位排他/全量 census/周期源旁路 pin 零回退(全仓绿复核)。
**pin**:engine 2(family 实铸=TestSemLeadFamilyPublishesRealTotalKeepsBoostInternal;单 span pin EVOLUTION=TestRootCauseRankPromotesOnChainSemanticRuntimeSpanWork)+tool e2e 1(BuildIndex→BuildWakeupChain/BuildRootCauseRank/ComputeWindowStats→traceQueryTypedObservations 全引擎实铸,断言=加冕/❶/单席/[E#+E#]/合计5.300/无11.130/无内部token/类词表面/roster 保留)+fold 守卫单元 7 臂+非链控制 1+evaluator 3 臂+skill/description 正负 pin。
**witness 吻合(cust_trace_textup_792.txt)**:行57 主根因加冕=追认保持;行112(E9 ✦ Texture upload ×11)+行136-138(E13 链上·未接入树 ❶ 最大成员名+有效归因214.561)=修后合一为单 ✦ 行(类名×N+根因排序#1+❶+有效归因102.172);行199/203 表双席+214.561 表值=修后单行+真实合计;行46/51 prose 的 semantic_multiplier/hidden_cost_boost=源头摘除。
**残留(如实)**:①非链双席形(E10/E18 shader 对=语义通道+adjacent rank 席两条 record 两个 E#)按裁定 scope 外维持现状——若将来裁定非链也合一,fold 的 on-chain 门一处放宽即可;②E13 witness 的行2 类别词"算力供给候选"(DominantState=running 驱动的 form 表)随双席消失自然退场,merged 行恒为"确定性优化候选"。

### §29.22.1 SEM-LEAD 复核收账(2026-07-10,SHIP-WITH-FIXES→三修复+DFULL 收编已落)
**P1-1(必修,序值倒挂)**:首铸把 rootCauseEffectiveImpactMs 排序通道切到内部 boost,而 board/❶❷❸ 按发布 eff 排序——real<primary 形(D/IO 8.100 vs texture real 5.300/boost 11.130)同页 ❶ 挂 根因排序#2、❷ 挂 #1 三面矛盾零披露。**主会话裁定=修向(a),依据 §7.30 S1(排序合成分数不得以 ms 硬事实发布——序数芯片即发布面)+嘈声信号只作软引导红线**。as-built:①on-chain 序数键改回发布 eff(rootCauseEffectiveImpactMs 新臂删除,EVOLUTION RECORD 注),序数≡board≡徽章≡图例;②boost 降级恰两个软面——Score 次级键(rootCauseRankScoreBasisMs,同 eff 平手 tie-break)+语义保留席分配信号(truncateRootCauseRankItemsWithSemanticSeats seatSignal:超额家族按 boost 择席,发布榜序/值零变);③语义行登顶=凭真实家族合计(textup 102.172 本就最大,照样 #1;e2e pin 锁);④RankSortBoostedEffectiveMs 字段注释改明两软面。pin:引擎 real<primary 关系不变量(序数序≡发布 eff 序+premise 断言 boost 本会跳位)+tie-break 单元双臂+保席分配+tool e2e ❶↔#1 同节点一致形(复核 witness 面)。突变 4/4 咬红(MP1 序数键回退 boosted=引擎+display 双红/MP3 tie-break 去 boost/MP4 席位信号去 boost/MP2 见下)。
**P2-1(必修,加冕臂零 pin)**:空 primary 桶语义加冕臂补 pin——纯语义板 e2e(引擎实铸:worker 无调度区间只有 marks+wakeup、app runnable 零长→全场零 root_cause_primary 记录,premise 在测内断言)→board[0] 语义行经 primary lane 加冕+❶;负对照=非语义 board[0] 维持 legacy nil+症状 rank=0+无席语义行 nil。突变 MP2(整臂删除)咬红。
**P3-1(书面)**:fold fail-open 残余 rank twin 可被 trunk main/extra 选中(域门放行时)呈 trunk行+✦行 两席形——各自账目诚实、类词臂全 seat kind 覆盖,显示冗余非双计;无生产 witness,注释落 runtimeTraceProjFoldSemanticRankLaneTwins。
**DFULL 收编(用户已批,pre-existing)**:runtimeTraceCausalProjectionFamilyBlockID 两 switch 补 "_detail_full"(builder 自 PTV8 明细重设计起即发射 idPrefix+"_detail_full" 而守卫不识)——修 F2b 幂等门(只剩 _detail_full 残块的文档不再二次发射)+PSG §25(b) 证据面(系统明细无损块数值入 grounding 池、其文本不再被当模型散文扫描)。方向论证:该块=系统铸造(SurfacePrincipal,引擎 typed 值渲染),纳入=修正系统面普查非放宽——伪造面边界与既有 "_detail" 同级(exact-spelling 守卫不松),数值可定位性正是 PSG 公理方向;两半均落。pin=family 守卫正负 case(_detail_full/_a2_detail_full 正;_detail_fullx/_detail_full_extra 负)+RuntimeTraceSystemBlockID 同断言+幂等 rerun 残块双 case;突变 MP5(两 switch 回退)咬红。


## §29.23 CSP-RM 收账(2026-07-10,推 main;§29.21 裁定落地+复核 P1 有益回探失而复得)
**交付**:双 fallback 改类——终端(answer_evidence_origin.go 模型 aggregate facts)+编译侧二次 fallback(observation_ledger.go 投影空集重铸,**勘察新发现=exclude 绕道**:原在零排放咽喉之上直铸 current_source,顺手关闭)→SystemInference origin+内部 kind model_claim(纯内部零 LLM-facing,R2' 不触);satisfied 只认确定性工具见证。消费方全谱普查(24+41+30 点):证伪"合法依赖模型 facts 单独撑 satisfied"的 run 形;唯一变严面=precise 要求∧零确定性见证→CanHardBlockCompletion 可武装(typed escape 双道原样,§1.6 合规);display allow-list 补入=advisory 记录无损喂显示的实现点。donghu_real eval 本地实跑 PASS(satisfied=false/source=0/citations=0,与 20260706 基线逐位一致)。
**复核(SHIP-WITH-FIXES)F-1 P1 收:首铸把 cmp 有益回探修没了**——satisfied 诚实变 false 后 arm0(用"源码导航债 advisory"理由杀 trace 钻取指令=语义错位)与 closure 子臂("1 条观测+insufficient 判闭环"名实不符)双双抑制 followup,弱首轮答案直接出厂,且被批自己的 Pin 1 以"充分抑制形"话术钉成期望行为(违 §29.21 验收句"cmp 有益回探保留")。修=**trace 钻取深度 typed 信号**(既有 TraceObservationCoverage 的 RootCauseRank 维 hard 确定性计数,零新信号零 R2' 面——witness 首轮恰零 rank 族行,逐位对应):arm0/arm2 双让位点 pending=词面 lens 激活∧深度==0→回探照发带 §29.20 trace 词面;钻取轮发布 rank 行→深度>0→gate 自灭=一次收敛(witness 逐位吻合,复用既有 retry budget 无需新 latch);fixture 保真修正(首轮证据级从 rank 级换 wakeup 级)。donghu 复跑零扰动(其首轮即有 rank 观测)。F-2 收:**第三绕道**=ungrounded evidence-item 铸 satisfied(模型 emit 伪 file:line 经 grounder 判 Ungrounded 仍进证明车道=donghu 伪引用形)——GroundingStatus 四态测绘后 Ungrounded→advisory;grounded(核验过)/recovered(确定性回捞,pin 反证)/empty(pass 未跑≠核验失败,absence never guesses)三态字节稳,过宽化突变三面 pin 防线咬红。F-5 producer 守卫 kind 侧同拒(走私行重定性)。突变累计 10 组咬红。
**教训**:①**批验收话术可掩盖行为反转**——"充分抑制形"pin 名字钉着 insufficient 断言,复核必须对照账本裁定原文逐句核验收句(两批同窗相同教训:SEM-LEAD"排序强度零变化"话术掩盖序值倒挂);②诚实化 authority 信号会让"以污染信号为前提偶然成立"的下游有益行为失效——修根必须同步给下游换上真正度量目标语义的 typed 信号(钻取深度),不能放任 arm 用错误理由做对的事。
**残口清单(裁定池,§29.21 scope 外)**:①explicit origin token(plain run 模型 emit `origin: current_source` 维度 token 仍可铸证明车道,字节稳 pin 在案);②line-44 required-lane 声明 SupportRefs 车道(模型声明 file:line 坐标进验证机器=边界形,防过宽 pin 在案);③binding 第四 fallback(answer_claim_binding.go 投影空集→current_source binding,仅 binding 面不喂 satisfied,修后仅 NegativeObservation/Unknown kind 可达)。留观:violation_root_cause Hard 早退微松/explorer typed-delta 进度启发式失配(软面)。
**验收**:donghu eval 20260710-090850 PASS;cmp 形=引擎实铸两方向 pin(首轮回探照发/钻取后抑制),原件仅客户机→实跑列客户复放项。

## §29.24 Trace correctness / 报告 UX 收口审计（2026-07-10）

**演进承接**：本批建立在 `ORD → DISP-3 → CSP-RM → SEM-LEAD` 的主线之上。远端 `SEM-LEAD` 已把链上语义工作提升为可加冕的正式参赛者，`CSP-RM` 已把模型 fallback 从 current-source 证明车道降为 advisory；本批不回退两项裁定，而是补齐其边界证明、物理时序与发布 UX。

**四个 correctness P0 关闭**：

1. **线程身份**：已知 TID 后禁止 comm/TGID/free-text 补命中；name-only 必须唯一；`sched_wakeup_new`、X/Z 后重现建立代际边界，creation 不进入普通 wake 因果。PID-keyed scheduler/resource 聚合跨代际 fail-close；优先级、CPU-near、派生 TGID/进程名只读同代样本。rename 只更新显示 metadata，不拆同一 TID 的累计。
2. **时间单调性**：time-end early stop 只认 EOF-complete monotonic proof；scheduler 按 exact CPU/TID lane 审计，坏必需字段不再默认为 0。非 scheduler 时长车道（span/counter、block/storage、IRQ、workqueue、DMA）及 cpu_frequency/frequency_limits 同样按物理 lane 审计；回拨 poison 穿过 cold window、warm cache 与 bundle canonical sort。频率 poison 精确到 CPU，并封堵 topology、donor、rail、ceiling、low-frequency 旁路；未配对事件不得用首尾 envelope 冒充时长。
3. **窗头 carry-in**：完整 prefix checkpoint 是唯一首段 authority；unknown/partial_unknown 明示，左边界旧 0ms carry 删除。`sched_migrate_task` 以 exact PID+dest_cpu 更新 Runnable carry 的 CPU 而不重启状态；恰在左边界及窗内迁核均按事件时刻在旧核/目标核间分段，WindowStats 与 SchedulerLatency 同判。
4. **bundle provenance**：每个事件/证据可逆映射 physical artifact+local line+clock domain；仅 same-domain 或 calibrated finite reversible affine map 入 canonical 因果域。隐式 sibling manifest 必须明确声明请求 systrace；额外 systrace 无 shared-capture identity 时隔离；source identity/cache/head/raw 读均做 TOCTOU 复核。Clock compatible 不再等同 capability compatible：perftrace 只允许 `trace_query_ready=true` 的 perf_sample，scheduler/CPU-control 永不入共享流；thread/cpu identity 只认 converter 闭集枚举，未证明坐标匿名化但保留 symbol/DSO 库存与剔除计数。

**裁定池落实**：A1 explicit origin token 在 ledger throat 降 advisory；A2 要求每个 source-shaped SupportRef 唯一绑定同轮 Grounded/Recovered evidence 或成功 tool typed carrier，裸 file:line、Summary 文本、短路径歧义和部分伪造均不能铸 `CurrentSourceSatisfied`。B4 采用精确裁定表：`d_state_or_io_wait` 吸收同 TID/同窗/同区间/同行号的 `io_burst_episode`，被吸收行仍以 typed supporting observation 和“链上并入”发布，合法 t=0 同样适用。B6 roster 仅在 subject 的可见 seat occurrence 唯一时追加“见榜位#N”；跨窗同 ordinal 两席也判歧义。C15 hedge marker/caveat 改 upsert/reconcile，多次 finalize 收敛。

**输入兼容与报告 UX**：`runtime_artifact:<16hex>` 误填 `source=path/path` 时只在当前 typed selection→唯一 stat-verified 物理 trace 的证明成立后自动改写；未知、错 kind、无 carrier、多映射 fail-close。工具结果头回写 `auto_resolved`、resolved source 和 canonical next call，prompt/schema 直接教 item.source/attached_trace。原 `20260710-104452.287-21978.html` 的树缺失并非“无因果”，而是三次 trace_query 都把逻辑 ID 当本地路径失败；新 eval 去掉“分层/依赖/确定性优化点”提示，要求模型主动发现，并用 tool success + 投影树作为硬验收。报告稳定排序为结论/投影树→确定性优化→行动/指标→明细/证据；系统块用 exact reserved ID + 不可由 JSON 铸造的 in-memory provenance marker。HTML 树按 rune 固定 0/1/2ch cell、跨平台 CJK fallback、小字号、独立滚动、窄屏/打印/焦点适配。

**剩余 gap（ROI 排序）**：

- **P1**：BLIND-3 结构化 C| counter 先做载荷 census，再建 typed schema；block/storage 同设备+扇区+操作的并发请求仍需生产 witness 与 typed request identity，避免 FIFO 在精确同键并发时交叉配对。（勘注 2026-07-11，§29.26：BLIND-3 已由 `5d91b433d` 结案，状态权威=施工账 `trace_analysis_open_gap_ledger_20260710.md` BLIND-3 行；block/storage request identity 仍 witness 触发（§29.28② W-8 维持）。本行保留为当日审计原文。）
- **P2**：非 scheduler malformed endpoint 的 schema-specific integrity poison 尚未覆盖全部族；CPU 数据族统一补负数/超拓扑范围 ID 校验；全局 TID-reuse fail-close 可收窄到实际参与 subject 以减少无关代际冲突导致的可用性损失；C11 EN 车道闭集机械化。（勘注 2026-07-11，§29.26：本行四项后续进展——统一 CPU ID 校验已结案 `5d91b433d`；Workqueue/DMA endpoint 半已结案 `d729f634f`（generic storage 仍部分修）；TID-reuse 已收窄至 perf+Workqueue/DMA+direct 六族（`b303c3fd5`/`6405c94cf`/`7af38bc23`，复合面仍开放）；C11 EN 闭集已结案 `5da3fbed0`。状态权威=施工账对应行。）
- **witness 触发留观**：A3、B5、B7、B9、C10、C12、C13、C16、BLIND-1。C12 继续坚持“无 typed 链身份不伪造树层级”。（勘注 2026-07-11，§29.26：B9 已由 `5d91b433d` 结案——`cust_trace_vc_710` 生产 witness，不再属等待池；BLIND-1 触发条件已满足（§29.28② W-6，G/H/N/I 已由 `61271e8f1` typed 实施，其余自由文本长尾继续触发式）；其余项状态权威=施工账。）
- **销池**：B8 维持 dash=无有效归因；C14 维持 cross-stage FRCAP 总成本上限。

## §29.25 裁定(用户 2026-07-10):HarmonyOS RT 优先级边界 41-159 确认+客户 witness 落档;审计处置委托
**裁定①(41-139→41-159 翻转追认,witness 补齐)**:用户回访附件三份支撑——`customlogs/format_census_berlin.txt`(berlin 1104MiB 全谱 census:prio 直方 **142×756604**(全 trace 最高频优先级之一)/157×3170/159×140/140×3212,prio>139 计 763186)+`customlogs/format_census.txt`(VerifyClass 案 record_trace_20260606:157×36/140×21)+`customlogs/cust_trace_vc_710.txt`(prio=53→ohos_rt 生产判定与两界兼容,20/53 CFS-RT 反转判定链完整)。真实工作负载大量落在 140-159 段,旧界 41-139 会把这些线程误归 system_or_kernel、错出高优先级压力账户。**41-159 为正确边界,远程批 flavor.go 翻转追认生效**;审计 P1-#4 的定性从"无证据链翻转"改为"witness 补齐后合法演化",剩余义务=flavor.go 与 stale-ban pin 补 EVOLUTION RECORD 引本节+witness 文件名。
**裁定②(处置委托)**:用户原话"其它的按合理的方向进行发展"——两轮全方位审计 finding 的处置(P1/P2 修复、EVOLUTION RECORD 欠账补记、双账本状态收敛、裁定池远程关闭项(B4/B6/B9/A1/A2)按方向一致性追认)按主会话判断执行;方向存疑项仍单独上呈。

## §29.26 Workqueue/DMA endpoint integrity + 语义标签演化（2026-07-10，`d729f634f` / `40fcf403b`）

**Workqueue/DMA correctness 收口**：elapsed-time 端点改为大小写敏感 exact 闭集。Workqueue 仅 `workqueue_execute_start/end`，硬身份=`PID + 可解析非零 work pointer + physical source`，function 只作 metadata（兼容旧内核 end 缺 function）；DMA 仅 `dma_fence_wait_start/end`，硬身份=`PID + driver + timeline + unsigned context + unsigned seqno + physical source`，`dma_fence_signaled` 明确为瞬时 inventory、不得关闭 wait。宽别名只可服务 inventory，不能满足 hard identity；tuple 采用 NUL 分隔，避免 driver/timeline 自由文本拼接碰撞。

**歧义与坏行策略**：同 typed key 两个未闭合 start 使整 cohort 进入 ambiguous，全部 duration withheld，depth 回零后才恢复；不再 FIFO 猜配。物理 raw 行在 Event admission 前按标准 event column 审计，缺字段、伪指针、坏 PID/CPU/时间戳及 parser reject 均 poison affected family；未知时间用“不可证明在窗外”的语义处理，不能折成 ts=0 后 fail-open。报告新增 unpaired/ambiguous/suppressed 计数与 bounded caveat。generic storage 的 typed request identity 仍需生产 raw token witness，不由本批猜 schema。

**中文 UX 标签演化**：用户裁定 `Texture upload → 纹理上传`、`JIT → JIT编译`。统一映射已覆盖主因 headline、语义 family 行、树 action/shape、确定性优化表“类别”列及中文引言；Markdown、用户面板与 HTML 共用同一 AnswerDocument 结果。原始 `SpanName`、成员 roster、`span 原文`、明细 `类型:` 和 evidence/wire token 继续逐字保留，避免本地化改写 trace 事实。旧节中英文 `Texture upload` 的历史 witness 引文只描述当时输出，不再作为当前中文显示裁定。

**TID-reuse 可用性续批（`6405c94cf`）**：在 d729 建立 exact endpoint 闭集后，Workqueue/DMA 的完整 numeric contributor PID set 可由同一 in-window event scan 精确枚举，因此各自改用 family-scoped lifecycle gate；无关 TID reuse 不再抹掉两族，一族贡献者复用不连坐另一族，贡献 PID 复用与 lifecycle-audit cap 仍 fail-close。通用 resource caveat 同步移除 Workqueue/DMA，避免“报告说已省略但行实际存在”。offCPU/scheduler latency/IO 复合面继续维持全局保守门，直到 typed context-completeness/派生完整性可贯通，禁止把缺失上下文伪装成 0。

## §29.27 EN trace 报告闭集收账（2026-07-10，`5da3fbed0`）

> **互指(2026-07-11)**:两轮全方位审计的总收账节因物理追加位于文末(§29.27.1 之后),编号沿用审计六 commit(`e63efd2b`/`4f627c9f`/`c6040084`/`84b1f076`/`588a8538`/`bff91dc5`)message 已固化的「**§29.26 审计总收账**」,与上文远端批「§29.26 Workqueue/DMA endpoint integrity」**同号异题**;其语义序位在本节之前。同理,文末「**§29.28 cust710 回传对账**」与下文「§29.28 TID-reuse direct scope」同号异题。

**用户词面统一**：英文 trace 系统补充从分散的 `Requested/Projection window`、`Target symptom/wait/sleep`、`Artifact`、`rank=N` 收敛为四个规范词族：`Analysis window`、`focused thread/focused-thread`、`Trace file`、`root-cause rank #N`。`projection.Window*` 是编译出的分析窗；真正的用户大窗继续由独立 `UserWindowStart/End` 发布为 `User-requested window`，两者不得混称。多 trace 总览、窗长归一化、时间基不相交、分区 caveat、折叠 roster、下一步与 evaluator 窗外注记同时收口，中英文下一步排名分别为 `根因排序#N` / `root-cause rank #N`。

**无损边界与机械保证**：不做全局替换。原始线程名、span 名、trace 文件名中即便含 `Target` / `Artifact` 仍逐字保留；`runtime_artifact` 等 wire token 与审计明细 `rank=N` 同样保留。新增 full-face closed-set fixture 同时验证规范词、退役系统词负向、`TargetRenderer` / `ArtifactWorker` raw pass-through、raw `rank=7`，并把同一 AnswerDocument 渲染为 Markdown 用户面板和 HTML 做词面 parity。三包回归：`go test ./internal/tool ./internal/agent ./internal/types -count=1`。

## §29.28 TID-reuse direct scope + SEM-DETAIL P1 收账（2026-07-10，`7af38bc23` / `3522a10e6`）

**direct resource/plugin identity scope**：`BIOResources`、`FilesystemResources`、`PageFaultResources`、`AbilityEvents`、`XPowerEvents`、`HiSystemEvents` 六族只消费同一次严格窗内扫描已接纳的 Event；admission 返回实际入桶 kind，正 `ev.PID` 即其完整 numeric identity dependency。各族独立 `threadIncarnationConflictForPIDSet`：无关复用不连坐、贡献者复用只压本族、非空集合遇 lifecycle audit cap 继续 fail-close、PID=0/空族不虚构冲突。global identity clean 是所有子集 clean 的快路径，避免正常窗重复扫描。FileIO/PageCache/storage 及 TopIOInodes/IOPressure/BlockIOByInode/IOBurst/root-rank 派生链未放宽，仍等 typed completeness 贯通。

**generic storage witness 裁定维持**：客户 VerifyClass 采集可见 58 issue + 59 complete（raw 步被截为 120/251），两端稳定公共字段仍只有当前已使用的 `dev/op/sector/len`；未见 `rq/request_id/req/mrq/bio/cookie/tag/cmd_tag/task_tag/unique_tag`。并行 outstanding 的 tuple 均不同，同粗键重叠为零；Berlin 只额外出现非端点 `block_bio_remap`。因此 request identity 仍属 witness-triggered，禁止把 issue bytes、提交线程、complete error 或 CPU/PID 猜成硬身份。

**SEM-DETAIL P1**：SEM-LEAD 已裁定 on-chain 语义工作全权参赛，但无损明细旧 arm 仍固定显示“优化项,非根因”，可与“主根因#1”同报告冲突。修为链上 `确定性优化点(链上参与根因排序)` / EN `on-chain root-cause ranking participant`；off-chain 保留原义。判定优先级与 projection parser 同构：显式 `chain_relevance=on_chain` 为真，`adjacent/background` 为假，只有 relevance 缺失才回退 `on_wakeup_chain/on_dependency_chain`，未知 token fail-close。Rank=0/TOP N 外仍表达“参与”而不虚构榜位，Markdown/HTML/EN 三面同判。

## §29.29 HTML 因果明细列尾收口（2026-07-10，`5e441d362`）

因果投影无损明细的宽屏/打印两栏布局已具备整体小字号、节点列表 `break-inside` 与窄屏退单栏，但 E# 三级标题此前可单独落在左列末尾、其属性块跳到右列。HTML CSS 为精确 wrapper 下的直接 `h3` 增加 `break-after: avoid-column` / `break-inside: avoid-column`，只约束列分页，不改变章节顺序、Markdown/终端或证据内容；standalone CSS 测试机械 pin。同期清理 finalizer recovery 路径中“hedging 尚非幂等”的过时注释，代码现状已由 C15 的 marker upsert + private caveat reconcile 保证重复渲染收敛。

## §29.30 tracediag 自动补采窗演化（2026-07-10，`3e90dfc48` / `29d121d3f` / `51e26d958` / `8070f10db` / `af1fce213`）

**触发**：生产回访的 `raw_io_pairing_rows` 在 151ms 父窗内出现 `120/251` 截断；要求客户继续手工猜第二个窗口既容易漏掉 carry-in，也会把“小窗无行”误读成“无并发”。裁定为零 LLM 的确定性自动发现，而不是让模型按关键词/热度猜窗。

**先修 correctness 边界**：审计发现 block 旧 carry-in 只保留窗前 start、跳过窗前 done，导致“窗前已闭同键 pair”可能残留成假 open；generic storage 则完全不重放前缀。`3e90dfc48` 抽出共享 cohort FSM 与 generic identity 单源 helper，block key 改 NUL typed tuple；time scope 完整重放可用前后缀，只按 interval 与查询域相交发布，窗前已闭 pair 不污染窗内，generic 精确 carry-in 可恢复，line scope 维持原精确行语义。

**发现引擎**：`29d121d3f` 新增 closed registry `pairing_integrity`，直接复用生产 block/storage admission、key 与 FSM。排序固定为 closed ambiguous → open ambiguous（只披露）→ completed-pair schema probe；同分按 collectible、单窗、max depth、endpoint count、物理行号，且 block/storage 在预算允许时各保留一席。candidate 端点 core 可原子拆为最多 8 个 `<=50ms` 窗；若所需切片超过预算则整 candidate 不选，禁止只发 3/4 个切片。端点数、活跃 lane、cohort roster 均有硬帽；TID generation、同 lane 时间回拨、坏 identity、EOF open 与 parser 盲点均 typed 披露。扫描使用完整物理顺序和显式 `TimeStartSet/EndSet`，ts=0 可表达。

**source/provenance**：导出 opaque `TraceSourceVersion`，复用 size+mtime+mode+dev+inode+ctime 的强身份并覆盖 bundle/sibling universe；`StreamScan` 同时补齐开读/读后 TOCTOU 校验并拒绝 composite 单文件旁路。发现前后、每个派生实例前后及发布前均验证同一 source universe；任何原地改写、恢复 mtime 或 atomic replace 都使整份 v2 缓冲报告不发布。

**tracediag v2**：`51e26d958` 保留 v1 静态路径，新增顶层 `discoveries` 与步骤 `windows_from.discovery` 的受限 typed 合约，不开放 JSONPath、模板字符串或任意前序 prose 解析。静态验证覆盖 closed strategy/family、全局 label、父窗、NaN/Inf、window/line 互斥、generated-window/expanded-step/report 最坏预算；动态实例不继承 defaults.window，使用 discovery 结果并标 `FrameWindowAutoDerived=true`。报告先显示发现结果和完整执行计划，再显示证据；`dependency_empty/failed` 明确阻断且不回退父窗。generated event_search 为 result/window/header 预留行，确保所有已返回 raw row 可见；若引擎 compaction，实例转失败并发布 matched/emitted，不能称 N/N witness。

**客户心智与扩展边界**：通用 `collect_open_gap_witness.yaml` 的目标 TID 与父窗分别由 `--trace-tid`、`--trace-window` 注入，无需复制或编辑 YAML，IO 步自动 fan-out；专用 `collect_io_pairing_witness.yaml` 无需 TID，可覆盖最多 8 个小窗。模板以 `inputs.tid/window=required` 声明输入，漏传时 fail-loud，不存在演示占位值静默运行。TID 只绑定声明 `pid_from: tid` 的目标视图；raw completion/trace-mark 等步骤不绑定，避免丢失 IRQ/kworker 对端。加载层使用 closed typed overrides 与 required-input schema，未来其它收集类型新增 discovery strategy 后仍投影统一 `DiscoveredWindow`；下游 `windows_from`、CLI 输入和 source/budget/provenance 合约不变。`window_sweep` 的 scheduler hotspot 不被冒充 pairing 完整窗，LLM 也不参与选窗。

**CLI 收口与实测**：`8070f10db` 增加 `--trace-window`、closed `ScriptOverrides` 与 v2 `inputs.window=required`；override 只写 `defaults.window`，显式 step/discovery 窗保持权威，报告记录 `source=cli_flag target=defaults.window`，静态报告预算预留该 provenance 行。漏 flag 在创建有效报告前明确报 `requires --trace-window`。以 `customlogs/a.systrace` 的 `2942.240..2942.260` 父窗实跑专用模板，确定性生成 `2942.240207..2942.250306`（10.099ms）子窗，原样发布 line 53 `block_rq_issue` 与 line 67 `block_rq_complete`，`complete=true`、`identity_complete=true`、source lock validated；`parse_complete=false` 仍原样披露为 caveat，不把未解析行包装成全文件完备。该实测证明“原模板不编辑 + 单父窗参数 → typed discovery → fan-out → 双端 witness”执行链路闭合。

**typed TID 输入**：`af1fce213` 增加 `--trace-tid`、`inputs.tid: required` 与闭集 `pid_from: tid`。TID 只注入显式绑定步骤，未绑定步骤继续服从原 YAML selector/default；v1、无声明、无消费者、显式 pid/thread 双 authority、非 ASCII 十进制或超出 `1..2147483647` 均 fail-loud。绑定值私有 provenance 保证重复 Validate 幂等，事后篡改 PID 会失败；v1 renderer 零漂移，v2 报告以规范整数记录 `source=cli_flag target=pid_from:tid`。真实 `a.systrace` 用 `--trace-tid 36644 --trace-window 2942.240..2942.260` 跑通通用八步包，共 452 行，四个目标步骤参数均为 PID 36644，四个 raw 步骤无 PID；IRQ 线程 TID 85 发出的 line 67 `block_rq_complete` 仍进入 raw IO witness，全部 8 个实例成功。漏 TID 明确报 `requires --trace-tid`，不发布伪报告。

### §29.27.1 补充裁定(用户 2026-07-11):徽章跟随席位+TOP5+三面记号一致
用户观察"大部分时候很少看到 ❶❷❸"经 witness 量化实锤:opendir_792/textup_792 中 ❷❸ 仅图例出现、正文零行佩戴——同树内 #1 行(锁竞争 lane)戴 ❶ 而 #2 行(下钻 lane ⇅,opendir:111)/#3 行(IO 家族折叠 ⛓,opendir:118)裸奔。根因=**徽章跟随行形非跟随席位**(各 lane 行构造器逐个实现,典型逐 SHAPE 病)。裁定:①**徽章单点权威**——凡 Rank∈TOP N 的行,无论 lane/行形/渲染面(树内/未接入树/下钻/背景段/语义 ✦ 行)一律佩戴,单一发射 helper;②**扩 TOP 5**(❹ U+2778/❺ U+2779 同 dingbat 区块同宽度类,不引入新 EAW 类;图例「❶..❺ = 根因排序前五(依有效归因)」);③**三面记号一致**——投影树/榜(明细表)/§29.27② 覆盖账归因行内联同组 ❶..❺+E#,同一单点词源。入 COV-4 批;验收=opendir/textup 复放正文 TOP5 席位行全佩戴+vc_710 覆盖账三面同记号。

## §29.26 审计总收账(2026-07-10→11;两轮全方位审计+Wave-1 五批+Wave-2 PERF+PROV;§29.25② 处置委托执行)

**编号注**:本节与上文远端批「§29.26 Workqueue/DMA endpoint integrity」**同号异题**——审计六 commit(`e63efd2b`/`4f627c9f`/`c6040084`/`84b1f076`/`588a8538`/`bff91dc5`)的 message 与代码内 EVOLUTION RECORD 均以「§29.26」指本节,编号随已推 commit 固化不可改,两节以标题区分;本节语义序位在 §29.27 之前,物理追加于文末(§29.27 头部有互指注)。

**① 方法与数字**:对远端 correctness 大批(`e920a5d8`/`5d91b433`/`78349788` 等 24+ 提交)与本地窗口做两轮全方位审计——第一轮 12 维 62 agents,第二轮对抗复核 6 维 52 agents;逐项 verdict 收敛后 **71 项确认=11 P1/27 P2/33 P3**(r1=35/r2=36;finding 以 #0–#70 编号,本节与 §29.24/§29.25 的引用即此编号)。处置依据=§29.25② 用户委托("其它的按合理的方向进行发展");方向存疑项明列于 ③ 追认清单与文末裁定池,供用户复核可翻转。

**② 六批收账(五个 W1 hash 为 rebase 后族;远端 24 提交 3-way 叠加消解)**:
- **W1-ENG `e63efd2b`**:block done 端点对称化(全流配对+铸对时窗口判定,跨窗假 io_latency 灭,#2)/threadIncarnationConflictForQuery 改 observeAll 修 masked-conflict/中断同 lane 嵌套=cohort ambiguous fail-close(#9)/census 冲突 fail-close+caveat/负 PID 拒(#47)/prose 行不铸 span poison+ts=0 漏网关闭(#4)/RT 41-159 EVOLUTION RECORD(§29.25① witness 补齐,#12)。复核 SHIP 零阻断;pin 6 新文件+突变 46/46 咬红。
- **W1-STREAM `4f627c9f`**:窗前 wakeup carry 铸 StateRunnable(饥饿线程双车道 parity,#48)/无头 wakeup 三面对齐(见③-6)/line+time 窗口语义统一到索引权威(#50)/解析质量披露 parity(#52)/scanned 量如实(#51)。复核 SHIP 零必修(三攻击形实测守住);突变 24+3 咬红。
- **W1-DISP `c6040084`**:交集口径追认+双口径披露(见③-1,#62)/twin-fold 值镜像改 typed 同源(复核 R1 根修=单员族观测复用 family fold 精确算法,分歧源函数删除)/§29.10-3 排版回裁(见③-2,#63)/HTML wrapper 精确闭集终止(#56)/单 span 值源统一/tier 退役追认+SemanticClass 显示身份臂(见③-3,#60/#66)/gc_pause registry pins(见③-4,#64)。复核 SHIP-WITH-FIXES 全收;突变 13+3 咬红。
- **W1-MARKER `84b1f076`**:SystemGeneratedKind 剥离类根修(#10/#57/#68/#69:CaptureSystemGeneratedBlockKinds 快照时刻捕权威 sidecar 永不过 JSON 边界+Reauthenticate 只还原捕获时刻实有;11 回灌车道穷举=5 接/4 advisory/2 严禁接)/假 ViolProseScalarUngrounded 杀有益 recovery 形修复/两终端 caveat appender 收编 CAVSTR 寄存器(#11)。复核 SHIP 零阻断;突变 9+4 咬红。
- **W1-SEC `588a8538`**:敏感配置三面门(read/grep/exec 注册凭据路径精确拒,basename 限锚域,外仓同名放行+软警示=用户意图红线,#26)/内部读取门谓词下沉 types 层(citation 回填/grounding 伪造引用不物化凭据)/tracediag 报告绝对路径脱敏 basename+sha256 对账(#27)/preview 默认 127.0.0.1(#28)/operation env 敏感变量 blocklist+日志 0600(#29)/berlin yaml S|/F| 锚定 async 车道(#30)。复核 SHIP-WITH-FIXES 全收;突变 29 咬红,零假 pin。
- **W2-PERF+PROV `bff91dc5`**:windowed 解析热环单扫 memo(#21 29× 回归修根,warm anchored 39.7ms=原基线 1.0×;非窗口吞吐恢复持平,#22)/audit tracker 有界化(#3/#23,预算域定案见⑤)/五族 witness LocalLine 双坐标(#36 虚拟行号×物理路径引用伪造面关)/raw reason 五值分文案(#37 clock_inverse 三重失实纠正)/one-systrace 因果 authority 下沉+trace_mark B/E 排序分歧双入口 fail-close(#40/#41)/direct .perftrace unattested 披露(#35)。复核 SHIP-WITH-FIXES 全收;26 pin+突变 59/59;全仓 TEST-EXIT=0 直验。

**③ 追认清单(§29.25② 委托下主会话裁决;明列供用户复核,任一项否决按 pin 反向回滚)**:
1. **交集口径(#62)**:链上语义家族发布 EffectiveImpactMs=成员∩链窗**交集并**(可加性红线方向),配双口径披露「链上计入 X(窗口投影合计 Y)」全闭集词素;全重叠形对 §29.22 witness 字节恒等,部分重叠正负 pin。
2. **DISP-3 排版回裁往返(#63)**:恢复 §29.10-3 裁定排版(各投影 lead+关键指标**成对依次**),远端"全部树先行分层"反转回裁;系统补充永不插入对间;64 帽禁删模型块+跨语言幂等。
3. **tier 退役+SemanticClass 身份(#60/#66)**:追认引擎退役 RootCauseTierDeterministicOptimization 铸造(链上语义行走 primary/secondary/tertiary 常规选举=SEM-LEAD 全权参赛的同向延伸);裁定过的"确定性优化候选"类词显示身份改由 typed SemanticClass 车道承载(身份 cells+正负 pin;#61 双写"候选候选"顺修)。
4. **gc_pause 第六类(#64)**:追认 causal_token_registry 第六语义类扩容(注释理由如实化+四维 pin);等待形 span×cpu_work lane 前瞻冲突形入本节裁定池。
5. **B4·B6·B9·A1·A2 远程关闭追认(#0/#14/#17/#33)**:五项裁定池项由远端批单方实施,经逐项对照裁定原文核验为**同向**(B4=本地候选 G1 式裁定表扩对,吸收行 typed supporting+「链上并入」发布;B6=可见 seat occurrence 唯一才「见榜位#N」;B9=§29.22 残留①预告的 on-chain 门放宽+生产 witness `cust_trace_vc_710`;A1/A2=§29.21"satisfied 只认确定性工具见证"的同向收紧)——按 §29.25② 追认关闭。§29.19 裁定项①③/§29.22 残留①/§29.23 残口①② 原文保留,以本条为处置注记;EVOLUTION RECORD 欠账(#13,远端批 ~25 处零新增+删 3 条 §23.1① 记录)以本节与 §29.25① 为账面补记,不再逐 pin 回填。
6. **无头 wakeup (b') 三面对齐(STREAM)**:窗内无头 wakeup(无前置 sched_switch 头)铸 runnable=typed 见证非猜测,索引/流式/churn 三面同判;partial_unknown 披露保留(churn 守卫 hunk 物理位于 W1-ENG commit 的 query.go)。

**④ 事故记录与新纪律(如实)**:
- **假绿管道**:批内验证管道曾以 `go test ... | grep ...` 形吞掉 go test 退出码,两次终验输出已印 FAIL 未读即推送(假绿出厂,复核环抓回)。
- **DISP 过期基线验证失实**:曾以"本树绿"宣称"main 绿"——验证跑在过期基线上,"本树绿"≠"main 绿"。
- **修复轮无复核环**:复核 finding 的修复轮未再过独立复核即收账。
- **render.go 冲突标记入 commit**:rebase 后残留 conflict marker 曾进 commit(amend 修复)。
**新纪律四条**:①终验直读完整输出尾+退出码,禁 grep 管道吞 exit code;②任何"绿"结论必附验证基线 commit;③修复轮必经主会话抽验;④`rebase --continue` 前必 grep 冲突标记。

**⑤ PERF 预算域定案(复核 F4)+残余**:durationOrderTrackerLaneBudget=**65536/族,域=全扫描区**(windowed 构建审计完整物理前缀+窗口)vs traceCounterSeriesBudget=**8192,域=窗内已裁剪消费事件集**——GiB 级 capture 合法携带 >8192 个全文件 counter 身份,故 tracker 预算=消费者 8×(~300B/lane,最坏 ~20MB/族);溢出 fail-close 永不静默(family capped+既有 order_audit_truncated poison 全消费者可见)。两域差异已注释成文于 `internal/tracequery/duration_order.go`。#24 memo witness 帽=1024(durationOrderEventScanWitnessCap,帽满如实披露,"假整族 fail-close"类灭)。#41 残余类=composite canonical sort 改 trace_mark B/E 栈配对语义形——已由双入口(BuildIndex/StreamScan)排序分歧 fail-close 收口。**比值型性能 pin=假防线教训**:cold/warm 比值断言随分子分母同伸缩恒真(29× 回归曾在其下全程绿)——重造为**绝对预算+同跑校准地板**(cold≈1× 全量解析语义地板,tripwire ≤4×,突变实证咬红)。

**⑥ 双修撞车三例与协调纪律**:同窗两会话(本地审计批×远端 correctness 批)三处撞车——①**block 配对**(W1-ENG done 端点对称化 × 远端 `3e90dfc48` 共享 cohort FSM 重构);②**semlead pin**(本地 SEM-LEAD 系 pin × 远端 `0d53b781` ranked localized semantic seat 演化);③**provenance 脱敏**(W1-SEC tracediag 绝对路径脱敏 × 远端 `7d574ec74` basename-only)。消解纪律=**pin-as-judge**:冲突不以先来后到,以账本裁定原文+对应 pin 为裁判——rebase 3-way 叠加后逐 pin 实跑判存留(实例:远端 traceMarkAsyncPairer 取代本批 #25a 优化;远端流式新校验接入本批 lineScan memo 净赚一次正则,benchmark 复测未稀释)。**协调纪律**:两会话以账本为唯一同步点——裁定/收账/状态变更必须先落账本,另一会话方可依赖;绕过账本的"对方会话已知"假设一律无效。

**裁定池新增(前瞻/留档,零行为变更;沿用 finding 编号)**:
- **gc_pause 等待形 span×cpu_work lane(前瞻)**:SYM(target_self_state)降道闭集派生自 registry Lane 列,gc_pause 现挂 cpu_work lane 故不受降道;若将来迁 wait lane,GC 暂停行将**静默**进入降道闭集——现状正确,纯前瞻;迁移前需先裁定该派生是否仍成立。
- **berlin 引擎 typed SpanAction 选择器(#30 尾)**:模板 async 车道现以锚定子串+exact `trace_mark_actions` 过滤双保险;引擎侧 S|/F| 车道判定是否整体迁 typed SpanAction 选择器=留档裁定。
- **模型中段 BlockCaveat 尾置语义(#59 邻)**:系统排序器把模型中段 caveat 块移文末——叙事位置是否属作者意图的一部分=裁定候选。
- **exec 递归扫出口遮蔽候选(SEC 留观)**:递归目录扫描的出口路径遮蔽形。
- **repl localOperationSkillEnv inherit(#29 邻)**:REPL 本地操作技能子进程 env 仍整继承(SEC 批 blocklist 只覆盖 operation executor 面)。
- **STREAM F2/F3(书面)**:F2=unparsed-ratio 披露分母稀释形;F3=流式扫尾成本。
- **ENG P4×3(书面)**:孤儿 exit 无披露/count=0 paired=1 读感/双重降解行逃逸。
- **MARKER P4×4(书面)**:白名单文件粒度/empty-ID 守卫无 pin/walk 只扫 internal//尾 \n。
- **远端 pairer lanes 全表扫(留档)**:遍历全 lanes 表的线性扫描形。
- **BLIND-1 升级**:触发条件已满足(§29.28② W-6)——从 witness 触发升为"条件已满足,验证远端 `61271e8f1` 覆盖后关账或补批"。

## §29.28 cust710 回传对账(2026-07-11;客户 build=0.1.20260710/14:06Z,含战役七批、不含 W1 审计批;witness=`customlogs/cust710/` 全套含 `*_02` 续片)

**编号注**:与上文远端批「§29.28 TID-reuse direct scope + SEM-DETAIL P1 收账」同号异题,见 §29.27 头互指注。

**① 三验收命中(关账)**:
- **ORD(§29.19)**:cap2_report_02 rank_fold_basis 步——runnable merged ×3 combined=12.537ms sum_disjoint 与 §29.19 复放预测**逐字同**;优先级反转 aggregated occurrences=2 单席;VSyncGenerator occurrences=8 周期源入席 tertiary #5;序数 #1..#12 连续。§29.19 复放核对清单关账(原节已加回注)。
- **CAP-3(§29.16)**:两窗 core_class=small 全判出+全报告零"簇结构不可判/按纯频率比折算"词=两窗同判——straddle 已愈,§29.16 验收关账(原节已加回注)。
- **G12/E23(§29.13)**:g12_report——hmfs prev_state=D 匹配 0 行+oney 6 条真 D+hmfs 全 S+iowait=§29.13 定案生产二次确认,验收关账(原节已加回注)。

**② 新 witness 立案**:
- **W-1 platform 标签同报告翻转**:open_gap_witness_02 步骤6/7=harmony vs 步骤8=donghu,同 trace 同 run——排查批候选。
- **W-2 blocked_reason caller=ASCII 污染假指针**:0x383435317c45000d="8451|E"、0x702e64676f6c6968="hilogd.p"(字节反转形)——支撑远端 `0e49700c` structured blocked reasons 方向(施工账头部互指段已注明)。
- **W-3 prio 301/65534/65535 大计数观察**:census 41-159 分带面工作正常(>159 入 system_or_kernel/raw 带,不入 RT 压力,与 §29.25① 一致)。
- **W-4 irq=0 合法+同 CPU 异 vector 嵌套实锤**:W1-ENG 中断 lane 键设计被生产验证;完整性门不得拒 irq=0。
- **W-6 BLIND-1 触发条件满足**:N|/I|/G|/H| 结构形(I| 带 ms 值、G|H| 带 cookie 对)=远端 `61271e8f1` ATRACE-TRACK 目标形——留作验证覆盖用样本;裁定池对应升级(§29.26 尾)。
- **W-7 open_gap instance5 `generated_window_compacted` typed fail**:fail-loud 行为正确;模板拆族/提帽改进建议归 remote 模板域。
- **W-8 B1 维持 witness-triggered**:io 两窗 pairs 零歧义+payload 空括号无 request token——立案条件不满足,维持等待。
- **W-9 d_state 202.000=整窗 carry 五行批量形**:carry-in clamp 行为正确;COV-4 覆盖账语境备注。

**③ 采集协议生产可用性**:客户命令列表显示已使用 `--trace-window`/`--trace-tid` 新 typed 输入跑通采集包,其中一步失败走了诚实 fail-loud(未发布伪报告)——§29.30 交付的"原模板不编辑+typed 输入→discovery→fan-out"链路生产可用性初证。

## §29.29 COV-4 收账(2026-07-11,推 main;§29.27 主文复归+四件套落地+复核三修)
**§29.27 主文复归(rebase 事故勘正)**:原 §29.27 裁定节(commit 7dec4f8d)在后续 rebase 中被远端同号节顶掉,仅 §29.27.1 存活。裁定原文复归于此(canonical):**裁定①(用户 2026-07-10)参赛口径=折算后有效影响**——runnable 全额/running 供给折算(§26)/语义类交集口径(§29.7-2)/周期源 VS-1 折减/IO·D 全额,同板同序数空间;**裁定②覆盖账=关注线程全窗四态墙钟分区**——running+runnable+sleep+D-state(Σ==窗恒等式,不平衡拒渲);**IO 点名为 sleep/D-state 内 typed 归因标签**(非第五加和项);running 段归因=语义 span∩running 交并+低频/小核双标互链不加和;running 残余="自身执行(无确定性可优化工作)";**禁止折算值进入墙钟百分比**(§7.30 S1 负面先例);归因行 E# 互链榜席。§29.27.1 码点笔误勘正:❹=U+2779、❺=U+277A(账本原文 U+2778 有误,代码实现自始正确)。
**COV-4 交付(四件套)**:①全窗四态账全链(引擎 target_window_state_account.go 单扫分区+deterministic_running 交集+bundle typed 发布+编译锚窗 F-2 准入禁猜+显示 Σ==窗恒等门拒渲字节恒等回退+图例分母披露);②同板查漏=引擎序数空间干净(单一 rankPos 统辖全部参赛者,running 序数键=折算 deficit);③徽章单点权威 runtimeTraceProjRowSeatBadgeOrdinal(=行发布席位序数,四行集合遍历,负向门与选举板同臂)+TOP5+图例「❶..❺=根因排序前五(依有效归因)」+HTML 面 ❹❺ token/色;逐 lane 实现与 PTV6 one-seat 去重退役(引擎 §29.19 排他+rank-twin fold 接防);④三面记号 token 单源(树/明细/覆盖账同 ❶..❺+E#)。
**复核(SHIP-WITH-FIXES)三修**:**A-1 P1=sleep 侧 IO refinement 缺失(G12 教训复发形)**——iowait 升级原只对 D 区间,S+iowait(东湖平台主形)在账面永无 IO 标签而图例已承诺;修=SleepIOWaitMs refinement overlay(复用同一 findBlockedReasonForWithSelection+wakeupMatchToleranceSec 5µs 单点容差;S 永不重分类,SleepMs/DStateTop/IOWaitTop 发布面零变,refinement 不加和),渲染「sleep 19%,其中 IO等待 X」镜像 D 项;B-1=Σ 门容差从借用的 0.5ms jitter(200× 舍入尺度,0.003-0.5ms 带内可见自相矛盾)收紧为 %.3f 打印串相等;C-1=seat token 改以 gated Badge 键 glyph(stale-Rank 症状行双面分叉闭合)。卫生:五 lane 加和/refinement 标签边界注释成文;Σ% 取整 ±1% 图例披露;确定性工作「(共N类,最大 Y 见 ❸[E#])」;fold 臂 pin 署名做实。突变累计 17 组咬红。
**教训**:①**图例是承诺面**——图例句写"sleep/D-state 内标签"而渲染器只有 D 侧数据道=承诺结构上不可能的能力,图例词面必须与数据道逐条对账;②容差常量禁跨语义借用(自述 never a hard gate 的措辞 fork 常量被借作渲染准入门=200× 错尺度);③refinement overlay(标注不加和)与 partition lane(加和)是两类信号,注释与 wire 必须写明归属。
**裁定项(需用户)**:SYM-2 树形 SelfRows 自因行不入 lead 选举板(平铺形可入板加冕=cmp_78 witness 形;徽章面本批已拉平,选举种群是否扩至 SelfRows=选举语义变更)。**留观**:同值双席若引擎再犯则显示层如实双 glyph(防线已全押引擎 §29.19);多窗 per-window 前五(裁定 verbatim);W-9 整窗 carry 形由 Σ 恒等门天然覆盖(carry 不全→拒渲)。**验收(客户新构建)**:vc_710 形四态账三行+sleep/D 双 IO 子句+三面同记号;textup 等待主导回归;opendir/textup TOP5 席位行全佩戴。

## §29.30 裁定(用户 2026-07-11):lead 选举种群=持席行(SYM-2 SelfRows 扩展,方案 a)
**裁定原文**:认可方案 (a)——lead 选举种群从 TreeRows-only 扩为**持席行**(TreeRows ∪ SelfRows,以席位为门),配自因加冕词面。依据:COV-4 徽章拉平后,自因行持席 #1 时"❶ 在一行、主根因在另一行"同页矛盾(序值倒挂教训的加冕版);同数据平铺可冕/树形不可冕=加冕身份随渲染形状漂移;running 主导帧(vc_710 类)加冕句指向"自身执行/确定性工作"才是可行动结论(方案 d 冠给次要外因=D-6 叙事倒挂类,拒)。
**四条设计约束**:①种群门=席位门(与徽章负向门同臂;target_self_state 症状降道 Rank=0 继续排除,症状永不加冕,§24.13 不动);②自因加冕词面=自因成因形(「主根因: 关注线程自身 running(确定性工作 X + 供给折算影响 Y,自身执行 Z)」类,词素取 COV-4 覆盖账闭集,不冒外因句式);③无特殊加成(同口径竞争,平手规则同既有);④pin 四向(平铺/树形 parity/自因冠词面/症状负对照/外因主导帧回归 vc_710 形冠仍归外因)。顺带解决 78 系遗留裁定候选"加冕句成因词"的自因半。批次=CLOSE-1(与 W-1 platform 翻转排查合批);cmp_78 平铺加冕 witness 形=收敛基准。

### §29.30.1 补充(远端同事要求,用户确认 2026-07-11):有效持席门四条件+单一共享实现
lead 实现额外守住:**EffectiveImpactMS>0 ∧ 非 context_only(6eb633a1 新 typed tier,链上语境转手行 rank=0/eff=0)∧ 非 target_self_state ∧ 非 overflow**;普通 sleep 与零 CAP running 不得因 SelfRows 扩展重新加冕。核心原则:**SelfRows 只是显示位置,不能成为语义特权;lead 与徽章必须复用同一"有效持席"门**(单一实现,收编既有 board 门第二份拷贝)——否则"❶ 在 A 行、主根因在 B 行"或零影响行被加冕的同页矛盾复发。波及:COV-4 zeroEff 佩戴形随门翻转(EVOLUTION RECORD 引本节;裸「#N」chip 保留);§24.15 全榜 eff≤0 形=lead 缺席+无 glyph 诚实形(旧裁定词面核对后对齐);TestSYM2ZeroDeficit 板尾参赛形=参赛显示保留、不冕不戴。

## §29.31 CLOSE-1 收账(2026-07-11,推 main;lead 选举扩展+platform 翻转修根;W-1 关账)
**编号注**:§29.30 存在同号异题(远端"tracediag 补采窗"节 vs 本地"lead 选举裁定"节,行 1978/2072)——两节以标题区分,本节引用的 §29.30/§29.30.1 均指 lead 选举裁定系。
**件①(§29.30+§29.30.1 落地)**:单一共享有效持席门 `runtimeTraceProjRowValidSeat`=HasData∧显示席位1..5(与行2同解析器,fold 领席一致)∧EffectiveImpactMS>0∧非context_only(6eb633a1 新 tier)∧非target_self_state∧非overflow∧**lane-kind 合法臂**(chain/cause/depthless 直通、self 挂 §24.17 四族闭集、semantic 挂 on_chain、background/adjacent 默认无席)——徽章权威与选举板同门消费,board kind switch 整体塌缩退役(第二份拷贝灭)。选举种群=TreeRows∪SelfRows 过门;自因冠词面 Tier A(running+四态账可证,parts 单源复用 Σ==窗门单实现)/Tier B(「关注线程自身 {state}」,D 族类别词回声抑制);V1 rankless 车道 typed 零 CAP 臂(SupplyFoldComputed∧deficit≤0 永不冕);自因身份三级 typed 推导(Target→账 Subject→印记一致 Subject,分歧不猜=flat 可达)。复核 F1 实锤收:徽章四集 vs 选举两集种群分裂(Background stale-Rank 佩 ❶ 而冠在链上行,探针复现)→lane 臂收进单门,EVOLUTION RECORD="佩戴=有效持席,lane 合法性是席位有效性组件"(§29.30.1 对 §29.27.1 的精化非反转)。pin P1-P9(平铺/树形 parity/自因冠词面/症状/context_only/普通 sleep/零 CAP running 负对照/vc_710 外因回归/❶==被冕行结构 pin/stanza lane 无席);突变 10 组咬红。
**裁定冲突处置记录(重要)**:"全榜 eff≤0=lead 缺席"一般化按字面会红 22 个既有 pin(含 #68 用户裁定 periodic 零冠"有效归因 0.000ms(期内节拍已折算)"/VS2 故意 pin/witness golden)——**根障碍=wire 上"发布 0"与"未发布"同为 float64 零无 typed 区分**(board 车道 eff>0 门使歧义结构性不可达,歧义只活在 V1 车道)。已交付=远端点名双形 typed 臂精确关闭;**后续批立案:typed effective-published 记号(R2' 六处同步)后做一般化拒冕**。
**件②(W-1 修根,§29.28② 关账)**:测绘定论=per-query 独立检测输入集差异(流式 event_search 把 idx.Events 重建为匹配子集后才做 framework surface 检测;witness=同报告步骤6/7 harmony vs 步骤8 donghu)。修=per-file 单一权威 platform_surfaces.go(typed vote,settled 双面齐后快路)挂 traceAnchorSet(write-once 与 flavor 同纪律),五扫描道过滤前投票;复核 F2 收:**铸造资格门=无 pattern 预滤∧无 time/line 窗∧扫描达 EOF**(event_types/action 过滤在 parse 后不收窄投票基;budget-denial 出口铸造移除;LRU 驱逐重铸只能来自完整基=翻转形关闭);F3 收:无合格 record 形 typed 披露 platform_detection_basis=partial,有 record 零披露。FrameworkSurfaces 显示 per-query 保留;flavor 车道零触碰。benchmark 零回归(A/B ×3,+0.85% 噪声内)。pin W1×5;突变 4 组。
**落账项(复核 F4-F7+观察)**:F4 席位∈1..TopN 收进选举门=行为变化(修前裸 Rank>0 无上限;顺带修掉修前"fold 领席行佩章不能登板"潜在分裂=正向收益);F5 stale 非四族自因形 flat/tree 加冕漂移残留(flat 无自因身份载体,结构性;"渲染形状漂移已杀"限定为四族形);F6 自因冠词面 parity=载体条件性(flat 无 stamp/account 载体时按 absence 不猜走外因句式=诚实);观察=vote 弃 PID>0 守卫(设计内放宽)/composite OR-merge 跨设备 bundle 奇异形(有信号披露)/IO 对(等待/阻塞候选)异词素保留。
**验收(客户新构建)**:同报告多 view platform 标签一致;自因主导帧(vc_710 类)冠句自因形;opendir/textup 徽章=有效持席形。

## §29.32 SQL Trace Streamer identity resolver R1a-A 收账（2026-07-11，`9d52377c2`）

**上游语义与裁定落地**：OpenHarmony SmartPerf `260b028b` 中 process `id/ipid`、thread `id/itid` 均来自同一 `CurrentRow()`；bundled SQL 与 hmtrace consumer 只因 alias parity 才等价。故 current profile 的 divergent alias 不是可翻译映射，而是命名空间不再可证，必须按相关 source/canonical cohort fail-close；legacy compatibility 仅由物理缺失 `id` 列证明，禁止按行失败后回退。

**交付**：shared resolver 对 `trace_range/process/thread` 硬字段统一执行 strict SQLite scalar 与范围审计，保留 internal ID 0、`2^32-2` 与合法 `t=0`，拒绝 `UINT32_MAX` sentinel、TEXT/REAL/BLOB coercion、负数和溢出。current/legacy source map 单点化；poison 同时封闭两个命名空间并在全表审计后重建二级索引，正反行序同判。`callstack.itid` 与非 NULL `callid` 的所有可达解释必须收敛。线程/进程名只作有界单行 display metadata，rename、缺名或不安全名称均不拆硬身份。

**消费面复核收口**：syscall、TaskPool、AppStartup、static-init、process-measure 不再把 malformed/poisoned/unresolved identity 降成 unknown 或 `(0,0)` 发布；合法 sibling 保留，TaskPool 两端身份先完整验证再发布。该收口只关闭 identity globalization，不宣称这些 producer 的 ts/dur scalar、CPU authority、stable row identity 或跨 exporter endpoint pairing 已完成。

**验证与剩余**：identity current/legacy/cross-alias 正反序、source map、二级索引、callstack 双身份、display-only 名称、ID 边界、traceStart singleton/t=0 与五类 consumer 均有机械 pin；验证为 `go test ./internal/hitraceconv -count=1`、`go vet ./internal/hitraceconv`、`go test ./... -count=1`、`git diff --check`。R1a-B 已由 §29.33 交付，blocked strict endpoint 已由 §29.34 交付；当前后续顺序为 R1b → R1c → R2。

## §29.33 SQL Trace Streamer scheduler identity R1a-B 收账（2026-07-11，`e61388803`）

**sched-start 与 active 六源**：`loadSchedStarts` 退役 `COALESCE(priority,120)` 和 SQL 预过滤，所有物理行先按 storage class 审计；同 `(itid,ts)` 只允许完全同值 coalesce，冲突/坏 sibling 形成 point/lane/global barrier，nearest poison 不得跳后。CPU 闭集为 `0..4095`；priority 是 strict signed-int32 且排除 `INT32_MAX` sentinel，`-1`、140、159、160 与 `INT32_MAX-1` 均按原值保留。active registry 的 callstack、sched_slice、thread_state、syscall、native_hook、frame_slice 六源复用 R1a-A canonical identity；CALLSTACK `itid/callid` profile 必须收敛，row-local malformed reference 不再靠 `DISTINCT/WHERE` 被静默吞掉，也不连坐合法 sibling 或 dormant main。

**signed identity 与 raw stable identity 分域**：instant/raw 的 internal ID 是 signed-int32 投影的 canonical uint32，`-1` 仍为 `UINT32_MAX` missing sentinel；`raw.id` 则是完整 uint32 event identity，故 `-1→4294967295` 合法。signed/canonical alias 进入同一 duplicate cohort 整体拒绝，同 timestamp 以 canonical uint32 排序，禁止 SQLite signed order 改写源序。

**wakeup 字段权威**：`sched_wakeup_new` 与 raw `sched_wakeup` 共用一个 canonical pairing kind，但输出事件名不改。缺 next-sched priority 不再丢失已证 wakeup edge；converter 写 `codrax_prio_source=unknown`，有 schedule-time inference 时写 `inferred_next_sched_slice`。`ParserVersion` 升至 v20；Event 用两个 authority bit 保持 688B，并在 EventView/WakeupEdge 显式披露来源。flavor、priority cache、scheduler-head carry、chain relation 与 inversion 等所有 hard consumer 对 inferred/unknown/untrusted 一律取 0；原生无 marker 的 exact wakeup priority 维持既有行为。tracediag RT histogram 同样只计 exact，并单列非确定性三类。

**兼容裁定**：旧 converter 产生、且未写 provenance marker 的 systrace 与原生 exact wakeup 在文本上不可逆区分。当前商用操作是重新转换旧制品；若要原位兼容，必须在 bundle 中保留 converter/version 这个精确信号后统一消费，禁止按路径、文件名或数值形态猜测。

**验证与下一批**：full uint32 边界、`-1/4294967295` alias duplicate、canonical order、140/159 round-trip、unknown edge、inferred/unknown chain 负向、native exact parity、wakeup_new、barrier/taint 与正反行序均有机械 pin。`go test ./internal/hitraceconv ./internal/tracequery ./internal/tool ./internal/tracediag -count=1`、四包 `go vet`、`go test ./... -count=1`、`git diff --check` 全绿，三路独立复审放行。blocked SQL strict scalar/stable endpoint 已由紧随其后的 §29.34 交付；剩余高 ROI 顺序调整为 R1b lifetime → R1c Running tid/pid → R2 single snapshot；TaskPool/raw active 补源与 sched_slice stable source order在精确信号可证时并入对应批。

## §29.34 SQL Trace Streamer blocked endpoint 收账（2026-07-11，`46d87930a`）

**上游权威与 projection 边界**：OpenHarmony SmartPerf TraceStreamer `260b028b` 的 `CpuFilter::InsertBlockedReasonEvent` 只把 `iowait/caller/optional delay` argset 附着到唯一 pending D/DK `thread_state`，并将状态改写为 D/DK 的 IO/NIO 分型；原 blocked 事件的 header CPU、header thread 与 timestamp 没有存进该表。故 converter 继续只发布可证明的投影：timestamp=`thread_state.ts`，header thread=该 state subject，header CPU=同 ITID 且 `sched_slice.ts+dur==thread_state.ts` 的唯一精确前驱；wire 明示 `original_timestamp_known=false`、`original_header_thread_known=false`，不把重建值冒充原 header。

**strict state/identity/args**：显式 `thread_state.id` 采用完整 uint32 stable identity，signed `-1` 与 canonical `4294967295` 是同一 ID，重复 cohort 整体拒绝；同 timestamp 按 canonical uint32 排序。缺 id 的已知 legacy schema只允许经证明的 SQLite hidden rowid，保留完整 signed-int64（含 -1/0）；`WITHOUT ROWID` 且无显式 ID 时 fail-close。itid/argset 采用 canonical uint32 internal 域并拒 `UINT32_MAX`，TID/TGID 必须在 public signed-int32 正域且与 shared thread/process resolver 精确一致。ts、可空 dur、state、consumed args 全部按 SQLite storage class 审计；`delay=4294967294` 是合法上界，`4294967295` sentinel 拒绝。blocked 不再维护第二套 arg JOIN：shared strict args/data_dict resolver 是唯一 `(argset,canonical_key)` 权威，相关 duplicate/poison 单调 fail-close，无关已知 key 不连坐。生产 S+iowait 兼容形、CPU0/4095、source/argset 0 与 `thread_state.dur=NULL` open-tail均保留。

**双席与 sched boundary 完整性**：同 `(itid,state-start)` 是一份 blocked 事实的 semantic cohort；有效+坏、双有效、closed-set near-token 无 args sibling 都不能让任一合法行逃逸。`sched_slice` 逐物理行 strict 扫描，不用 SQL WHERE 隐藏坏端点，也不把 `sched_slice.id` 误升为物理边界身份。已知 lane 的坏 timestamp 形成 lane taint；坏/NULL duration 形成方向性 lower-bound；未知 lane 的精确 end 形成 global point barrier，未知 end 形成 global lower-bound，未知 timestamp 形成 global taint。唯一 exact match 才可给 CPU；坏 CPU、多个 exact match、overflow 或 poisoned-nearest 均不发布。

**商用规模与验证**：full-table stable duplicate 检测改为 SQLite 内 canonical cohort probe，Go 堆只保留实际重复项；hidden rowid 路径零额外 audit。blocked adapter 仅对相关 argset 延迟建 item/map，避免 shared resolver 之外再制造 O(all argsets) 小对象。百万行 duplicate probe 本机约 0.13s。机械 pin 覆盖 INTEGER/TEXT/REAL/BLOB、full uint32 alias、canonical 正反序、hidden rowid/WITHOUT ROWID、CPU0/4095、argset/source 0、shared dict poison、TGID、NULL tail、point/lane/global barrier、delay 两侧边界、有效+坏/双有效/near-token cohort 与合法 sibling。`TestTraceDBBlocked* -count=10`、`go test -race ./internal/hitraceconv -run '^TestTraceDBBlocked'`、`go test ./internal/hitraceconv -count=1`、`go vet ./internal/hitraceconv`、`go test ./...`、`git diff --check` 全绿；上游 schema 审计、对抗审计和最终冻结审计均放行。下一批唯一 correctness 首项是 R1b lifetime，之后 R1c → R2。

## §29.35 SQL Trace Streamer lifetime R1b 权威裁定与施工检查点（2026-07-11）

**上游纠偏**：OpenHarmony SmartPerf TraceStreamer `260b028b` 证明当前代码把 `thread.start_ts` 当 hard lifetime 是错误的。线程创建/更新路径没有写 `Thread.startT_`，默认零通过 SQL 变成 NULL；`process.start_ts` 是路径依赖的首次观察时间；`thread.end_ts` 会被 `sched_process_exit`、稍后的 `sched_process_free` 以及同 TID+PID 复用后的退出反复覆盖。itid/ipid 是 canonical row identity，但退出不删除映射，故不是 generation identity。R1a-A 的 strict identity 仍成立，但 `loadStrictThreadIndex` 因 start NULL/坏 metadata 删除线程，以及 raw/perf/frame/callstack/native 用 latest/start gate 选代，均列为 R1b 必修纠偏。`trace_range` 只约束 capture，不补造线程/进程出生；thread/process name 永远只作显示。

**唯一 hard lifecycle 车道**：①strict `instant.name='sched_wakeup_new'` 的 `ts+ref(itid)` 是左闭 generation begin；上游 raw 表把该事件统一降名为普通 `sched_wakeup`，raw 名没有 creation 权威，且 creation 不作为普通 wake 因果发布。②exact `thread_state.state=X|Z @ ts`，以及 exact `sched_slice.end_state=X|Z @ checked(ts+dur)`，与严格更晚的首次可信 activity 组合证明 reuse cut；单独 X/Z 不凭空宣称新代，`sched_slice.ts_end` 不具权威。③`thread.end_ts` 最多在同一已证 generation、其后无 activity/rebirth 矛盾时作为外上界探针，不能自己切代。生命周期坏行复用 R1a-B：known itid+known ts=point poison，known itid+unknown ts=lane taint，unknown itid+known ts=global barrier，两者未知=global taint；`" X"/"x"` 等 near-token 不得被 trim 成 exact，也不得删除后跨洞。

**消费分类与边界**：thread-owned 执行/状态区间（sched/thread_state、sync callstack、syscall、frame、static-init 及可证 thread-owned IO sample）必须完整落在同一 generation；native-hook、BIO/block completion、network 等资源/请求生命周期只校验 origin emitter point，禁止拿资源 end 约束线程；async callstack 与 TaskPool 各端独立校验，任务允许跨线程；AppStartup/process measure 等只按可证明 process generation/point，唯一主线程 hard cut 才能向 exact IPID 投影。source duration 均为半开 `[start,end)`，generation begin 左闭；旧 source interval 可 `end<=cut`，但 B/E/S/F 的 closing line 若 `end==cut` 且 public TID/PID alias 被复用，因缺跨表同时间顺序证据必须拒绝。NULL duration 是 open/unknown，不能改写为零或 end_ts。

**小批与最低验收**：R1b-0 先把 identity 与 registration/lifetime tri-state metadata 解耦，current full schema 的 NULL/0/正数/TEXT/REAL/BLOB start 均不得改变 canonical identity/generation；R1b-A 建唯一 authority并接 scheduler/wakeup/blocked，覆盖 wakeup_new、X/Z 单/双源一致与冲突、NULL/overflow terminal、point/lane/global taint、正反序、exact boundary；R1b-B 迁移 callstack/frame/native/raw/perf，删除各自 latest-start/earliest-start 权威；R1b-C 迁移 syscall/TaskPool/AppStartup/static-init/process-measure并锁定上段分类。额外必须 pin：raw 普通 wakeup 不能切代、X/Z 同时间 activity 不切代、严格更晚 activity 才切、跨 cut interval 拒绝、新代 cut 点接受、native/block resource end 可越过 origin thread end、public-TID-only perf 多候选无 hard cut 时 fail-close、线程改名不改变任何 hard 结果。每批先账本、后代码、验证后立即提交推送；R1b 完成前不得把 R1c/R2 标成结案。

**R1b-0 收账（metadata=`3ad99b46e`；trace-start=`cc7dfec52`）**：identity 与非权威 metadata 已机械解耦。`StartTS`、`ByTID`、`ByTIDIncarnation` 全部退役；process/thread start 与 thread end 以 known/unknown/tainted 三态保存，缺列、NULL、0、正数、TEXT/REAL/BLOB/负数均不删除 canonical identity，重复 metadata 冲突也只污染 metadata。raw exact ITID 不再被 reused-TID 二次选举否决，TID/PID 只 cross-check；public TID 多候选 fail-close、exact PID 可唯一收窄且候选扫描零临时 slice。perf resolver 改为 missing/resolved/PID-conflict/ambiguous 四态，不能使用 trace identity 时明确 `resolution=perf_source_only` 与冲突原因。frame/callstack/native 退役 start-hint hard gate；metadata selector/helper 由 AST 函数级白名单限制在 identity 加载和 registration 展示。`TraceStartKnown` 现把合法 t=0 与 missing/空表/重复/冲突/NULL/坏类型分开；唯一 capture lower-bound helper 只在 known 时拒绝，未知 capture start 且无 thread hint 时跳过注册并计数披露，不再伪造 t=0。focused 10 轮、包级、race、vet、全仓测试、diff check及冻结复审通过。**未结案**：R1b-A 仍须建立事件型 cut并接 scheduler/wakeup/blocked。

**R1b-A 施工拆分（先落账后代码）**：A1a 只交付 immutable lifecycle kernel与纯机械边界测试；A1b 接入 callstack/sched_slice/thread_state/syscall/native_hook/frame_slice 六源 strict timestamped activity、wakeup_new creation 与 X/Z terminal collector；A2 才让 scheduler/wakeup/blocked 消费，并验证坏 sched 行整 CPU lane 抑制、Running 跨 cut taint、priority lookup 不跨 barrier、blocked 前驱恰在 cut 的 closed-boundary 拒绝。六源缺一不得宣称 activity authority 完整；各小批独立提交推送。

## §29.32 裁定(用户 2026-07-11):berlin.systrace=非标准旧转换产物;容错即可,禁过度拟合
**裁定原文**:berlin.systrace 由 codrax 之前内嵌的低版本转换逻辑产生,是非标准文件,不用过多关注;远端同事已修复转换工具。trace_query 仍要考虑一定容错,但**无需对非标准的 trace 文件做过多拟合优化**。
**波及清单**:①**W-2 重定性**——blocked_reason caller ASCII 污染(§29.28②)从"平台真相"改判"疑旧转换器工件"(文本字节写入指针字段=转换 bug 典型形);引擎 fail-close 结论不变(即正确容错),但不再基于污染 caller 建任何语义。②**S|/F| typed SpanAction 选择器裁定项销案**——属对非标准文件过度拟合;已落地的锚定 pattern 缓解(SEC #30)作为容错终态,标准 trace 出现同需求再议。③**验收改写**——"berlin 转换器修复后 sha256 对账复核"改为"新转换器产物为全新基线重采,不追旧产物 parity";"berlin 1GiB 首建实测"改为"任一 GiB 级标准 trace 实测"。④41-159 裁定依据重心=标准 trace witness(VerifyClass ftrace+vc_710;berlin 直方降为佐证),裁定本身不变。⑤HYG-C 批新增义务:扫描代码中是否存在只为旧转换器工件服务的特化臂(逐个核"通用容错 vs berlin 拟合"),拟合臂候选降级/移除报裁定。⑥既有行为修复(CAP-3/ORD/G12/完整性子系统)全部有引擎实铸 pin,与 berlin 文件解耦,零回退。

## §29.36 裁定包(用户 2026-07-11):树头 typed 记号+双序数通道+IO 家族+关键行减负+HTML 显示修正
**①树头统一形(用户明确同意)**:唤醒链不可上溯类 banner 统一为「⊘ <短结论>(<短因>)  满格=窗口Xms」——⊘ 复用既有"无匹配唤醒,链止"闭集语义,零新字形;渲染注记"以下各行按层级平铺"从头行移除(树读法图例已载,头行不重复);95072 witness 长句形(「睡眠区间在查询窗内无 sched_wakeup 记录,唤醒链无法上溯;…」)同规约为「⊘ 唤醒链无法上溯(窗内无唤醒记录)」。
**②双序数通道(用户裁定原话"如果是非链上的,应该都需要进入背景影响排序的另一个通道")**:根因排序#N 只发链上有效持席种群(=CLOSE-1 §29.30.1 lane 门种群,chip/glyph/冠/通道四面同一概念);邻近(◇)+背景(▒)行进独立**背景影响排序**通道(§23.1 非链语义类先例的一般化,提及门语义沿用);witness=4165 报告根因排序#1/#2 落在背景压力段而表注明言"不计入链上归因"=自相矛盾实锤。伴生修:⛓ glyph 与 ◇/▒ 段位归属统一 chain_relevance 单一来源(4165 的 ◇ 段内行戴 ⛓ =两来源分叉)。
**③IO 家族聚合(方向授权,批内细化)**:同线程同段跨 facet IO 行(页缓存抖动/块设备IO(inode)/IO延迟/IO突发/iowait)走 B4 裁定表全族扩对+**家族聚合席**(interval-union 墙钟持一席,成员行 absorbed 明细不占席,ORD 聚合/成员排他先例);witness=4165 同线程 5 席占半榜(0.6ms 物理等待)。
**④关键行减负**:参与性限定词(上下文·不参与根因排序等)统一行2限定词槽(限定词前、原因后;4165/140554 三变体+孤行+裸尾巴全规约);「深度未解析」类辅助词从 lane 前缀移行2(lane 前缀简化,释放关键行宽度);「×N同值」内联到行2名称后(与明细表同形,消灭孤行)。
**⑤HTML 显示修正**:glyph 视觉尺寸远小于字体(57823 witness)——CSS 字号/行高归一;深链 bar 列右漂(54476 witness:L3 行 bar 与上层不对齐)——bar 列全树固定,深层按「缩进封顶12」先例加紧名称截断。
**待确认项(本批不动)**:◦ 与形态词配对规则(建议:有影响形态词的行戴形态族 glyph,◦ 只留真正无类型词行——glyph 闭集变更待用户点头)。
**排批**:UXR-1 合批(②③=引擎参赛通道+①④⑤=显示),排 EPUB 落地后(同域 tree.go);eval runner 输出自动归档小项随 HYG-C。
### §29.36.1 待确认项确认(用户 2026-07-11,条件式同意经主会话论证成立)
**◦/形态词配对规则确认**:有影响形态词的行戴形态族 glyph(经 typed 形态表映射,严禁词面匹配);◦ 只留真正无类型词的行;形态词在场但闭集无对应 glyph 的形保持 ◦(其"无主导调度状态"宣称仍为真)。**调度态缺失事实不丢**:glyph 让位形态族后,该事实移入行2词面/明细承载(图例"具体影响形态见行内说明"对称改写)。论证要点:glyph 应承载已解析的最强语义,◦+形态词=混合信号(图标说不知道、文字说知道);与图例自身"无类型词的行=未识别影响类型"的暗示自洽。并入 UXR-1 批范围(§29.36⑤ 同批)。

### §29.36.5 EVOLUTION RECORD(2026-07-11 晚,v5 P0 批):⑤ 的 scale 实现路线退役
§29.36⑤「HTML glyph 尺寸/行高归一」的裁定目标(记号观感与行字号协调、bar 列全树固定)不变;**实现路线由 per-class transform scale(1.10/1.00/1.15)+overflow 补丁换轨为记号 2ch 信封纯算术**(v5 重-1,用户批准 2026-07-11):glyph+既有伴随空格并为 2ch inline-grid 槽,信封≥最大墨迹,scale 族整族退役(其注释~1.22x 与代码 1.15 的漂移即调参路线终局实锤,复核在案)。例外:❶..❺ 徽章 chip 按 T-6 保持 1ch+scale(.95) 至 P2a 恒4记号场批。bar 列固定臂保留并上移 fence 层(P2a)。禁以「归一已兑现」话术掩盖路线换轨事实——本 RECORD 即账面。

## §29.37 SQL Trace Streamer lifetime A1b-2 收账（2026-07-11，`7baf007f2`）

**唯一证据采集器**：creation、双 terminal 与 callstack/sched/thread-state/syscall/native/frame 六源 activity 已进入同一个 supplied-queryer collector；三类 completeness 共同控制 inferred restart，任一缺失都不得用局部较晚来源铸 cut。active registry 复用同一次物理扫描但保持原 admission/六条 coverage，不再有第二套 producer 或第三次数据扫描。数据 SQL 不做 COUNT 预扫或 WHERE/DISTINCT/COALESCE/CAST/ORDER BY 修复；sched/thread 各两遍是“先收全 terminal、后流式 activity”的必要边界，其余源各一遍。

**边界与规模**：exact/near、ref namespace、callstack 双 claim、idle0/public-TID0、高位 wire、known/unknown capture start、carry-in checked end、t=0、MaxInt64/overflow及四象限均已机械化。activity 流式复杂度为 O(rows log terminals)；可信 creation/terminal cohort 如实保留，malformed point 单独受 65536 预算约束，超限清点并升级 global taint。lifecycle authority summary在完整 tracebundle 中尾置，不改变既有 regular/extended coverage 顺序；tracequery 原 24 条预算不被挤占，并为该唯一 summary 额外保留一席。

**验证与剩余**：focused 多轮、两包 race/包级/vet、全仓测试、diff check与三路独立冻结全绿。A1b-2 关闭；A2 尚未让 scheduler/wakeup/blocked 消费该 index，R1b-B/C、R1c、R2 仍按既定顺序开放，禁止把本节写成 R1b 全结案。

### §29.36.2 补充裁定(用户提议 2026-07-11,主会话精化确认):三通道设计取代 §29.36② 两通道
**链上/邻近/背景各走各的通道**:①根因排序#N(链上)=有效持席门种群,可冕可戴(不变);②**邻近影响#N(◇)**=独立序数通道,同线程墙钟口径内排序,IO 家族聚合席(§29.36③)落此,提及门限量,不冕不戴;③**背景压力(▒)=无序数通道**——口径分组展示(跨线程聚合行与墙钟行分组,组内按量级固定序),§23.1 提及门只作内部筛选不打 chip。**精化理由**:▒ 段口径混杂(4165:跨线程 cpu·ms 聚合与墙钟同段),同一序数序列比较不可比数值=两把尺红线的序数版,故背景不发序数;◇=因果候选(可晋升上链,晋升边界=chain_relevance 单源)vs ▒=环境语境(永不晋升),认识论层级与可行动性均不同。配套不变量:序数 chip 词面必带通道名(根因排序#N/邻近影响#N),禁裸 #N;§29.36② 的"背景影响排序"单通道设计由本节取代。UXR-1 批范围随更新。

### §29.36.3 补充裁定(用户 2026-07-11):3+1 通道终形——提及义务显式化为第四通道
在 §29.36.2 三通道之上,**链上语义非 TOP N 的提及义务(§29.7-2 原文"即便没有进入 TOP N,也需要作为优化点被提及")显式化为第四通道**:①性质=保证型义务通道,非序数竞争——不排序、不占席、不冕不戴;②不变量=**凡 on_chain 语义行必渲染,结构上无静默消失路径**(入 TOP N→升通道1可冕可戴;未入→义务通道兜底:✦ 行保底+行2 typed 词面「优化点·未入根因排序前N」+类词点名(§28.1 五类+gc_pause)+E# 互链);③SEM-LEAD 既有提及地板 pin 升格为通道分类学显式成员(pin 演化带 EVOLUTION RECORD)。UXR-1 批范围最终定稿=3+1 通道(引擎)+§29.36①④⑤/§29.36.1(显示)。

### §29.36.4 补充裁定(用户 2026-07-11):词面精简两判据+核类词诚实门
witness=a4 报告(16011/2549)供给 clause「已按大核满频(或接近)运行,无供给缺口,running 为真实工作量(簇结构不可判,按纯频率比折算)」。裁定:①**推论链冗余判据**——A⟹B⟹C 互为推论的多段词面压成「证据+末端可行动结论+口径括注」单段(本例压缩形=「满频(或接近)运行·无供给折算(簇结构不可判,按频率比)」;图例已载展开语义,行内不重复);②**宣称-括注矛盾判据**——主句断言与口径括注互斥即词面诚实 bug:**簇结构不可判时禁用核类词**(「大核」宣称越权,纯频率比假设不得写成事实;簇可判时才允许核类词形);③UXR-1 增批义务=供给 clause 三臂族+状态词组合全模板扫描,两判据逐条过(已裁形如 zh-en 同词豁免);④同 witness 加重立案:「上下文·不参与根因排序」第四位置变体(主行数值列中间)+裸尾巴行2 再现,§29.36④ 范围内同修。

## §29.37 EPUB+EVAL sweep 收账(2026-07-11,推 main)
**EPUB 批(§29.31 立案关账:typed effective-published 记号+一般化拒冕)**:①记号=decode 单点存在性铸造(`EffectiveImpactPublished`,既有 effective_impact_ms note 存在性 typed 化,零 wire 新 key)——**R2' 六处同步不适用论证(定稿)**:零新 wire note key/投影从不序列化回灌(每轮从 ledger 重编译)/StateKind·Undrillable 先例同形/降级文本重解析 fail-open(published-0 正值过滤映射 unpublished,只保冠永不误拒);②传播两臂(R1 same-fact OR-单调/×N fold Σ-published,mixed 与 plain 分支均防御性去发布);③一般化拒冕臂(V1 车道:Published∧eff≤0∧!PeriodicSource→不冕,豁免全 typed);④21-pin 三分类全保留(#68 用户裁定原 pin 一字未动即证/VS2 20 pin 按构造保留/(c) 类=防御性,record 级 published-0⇒context_only∨periodic 完备性四 lane 直读核实);⑤"21 vs 22"差一破案=CLOSE-1 在其自身随后改写的 pin 种群上计数,当前基线无失踪 pin。复核(SHIP-WITH-FIXES,零假 pin)M1 收:**root_evidence 哨兵零被 R1 OR 臂移植为发布权威**(排位排除哨兵≠权威零,三元碰撞形假阳拒冕探针实证)→decode 铸造点 `root_evidence:` ClaimKey 家族豁免(emit 侧哨兵零触碰);L1 plain fold 防御性去发布;INFO twin-fold 值/记号分离点闭合(记号 OR 同行复制)。pin 9+突变 9 组咬红。
**EVAL 代表性回归 sweep(81 案,基线 48743718)**:**高置信通过——58 个 trace 案对本窗全部批次零可归因行为回归**;COV-4 四态账真实案精确闭合(114.940ms=窗长)/donghu 双案反转分析正确/fail-loud 契约无破口;唯一本窗相关 FAIL=cflow_perkind fixture 过期(FRCAP 新知识面,答案更新更对)。**既有缺陷收获(EVALFIX 立案)**:A.4 post-EOF 窗口捏造满窗 running(头快照播种越 EOF 窗,completeness 标 recovered 零披露,一答两面;复现脚本 collect_a4_eof.yaml 留档)=EVALFIX-A;A.2 make runner Suite:"make" 污染→scoped verify 执行 `make make`→write 永久 unverified(确定性)+A.3 workflow store 跨实例窜养(resume 门无 repo/goal 指纹,共享 .codrax 并发互认)=EVALFIX-B;A.1 census 消费缺口(b3 稳定 FAIL:引擎 typed process_domain_census 在场,模型只用钻取子集排榜;converted_inode 同类波动)="section 在场散文不消费"系统性主题=EVALFIX-C(答案侧硬门消费,精确 typed 信号)+subject 消歧软引导(storm/qf_arch 新词面遮蔽旧机制形);case 卫生三件(cflow_perkind oracle 扩词/state_churn·binder_aux 折行化)。波动类(data DL-B/read_combo typed anchor obligation)均既有裁定面,零新立案。

## §29.38 UXR-1 总收账(2026-07-11;§29.36 全裁定链落地+对抗复核+修复轮+UXG-0 推送门+六维 UX 审计+v5 设计裁定)

**批主体(基线 a4900959,含修复轮与 UXG-0)**:件①引擎 3+1 通道——wire 选型=零新 key((rank,chain_relevance) 二元组联合表意),assignRootCauseRanksAndTiers 拆 rankPos/adjacentPos 双计数器,▒ 零序数口径分组,✦ 提及义务通道显式化(SEM-LEAD 地板 pin 升格,EVOLUTION RECORD);IO facet 家族聚合席(rank_io_facet_family_uxr1.go:五 facet 闭集/TID+窗+lane 组键/墙钟 facet overlap-connected 同段证明/count facet roster-only/席值 interval-union)。件②显示九项:⊘ 统一树头三形/chip 带通道名禁裸#N/Row2 限定词槽/lane 前缀简化/×N 同值内联/glyph-形态词 typed 配对(blocking_span→⊗ Lock 族;⧗ U+29D7 非链 D-IO,⛓ 链上专属)/HTML glyph 尺寸/bar 列结构 pin/词面压缩(§29.36.4 两判据,2549 形逐字落地)。

**对抗复核=SHIP-WITH-FIXES(基线核实、越域零、假 pin 零、3 突变抽验全红)→修复轮八件全收**:
- P1-1 IO 家族联合值污染修根:MemberCount>1 行消费 familyMemberIntervals(hull 缝隙不再假证同段/计入墙钟);block_io_by_inode(值=复合分数、区间=inode 包络)从连通性与联合值双排除降 roster-only(「不计入墙钟合计」词复用);运行时不变量 union≤Σ(成员发布值);registry token 车道零触碰;4165 fixture 按引擎实铸重核(union=0.109 非包络 0.198)。**教训:interval-union 车道的输入必须有区间来历证明——hull 时间戳与 envelope 时间戳都是嘈声信号,不得直接进墙钟联合**。
- P1-2 周期源 pin 复原+失实 RECORD 勘正:fixture 补 CausalImpacts/ChainNodes 恒成对(引擎实铸形),Rank>0 断言恢复并兼任 §28.7 G9 保席看守 pin;"VS-1 in-period cadence demotion"确认引擎不存在,原 RECORD 失实定性落账。**裁定(主会话按用户委托,2026-07-11):周期源 sleep 聚合生产实形=◇ 邻近影响#N(§12.3-5 既有 typed 闭集:sleep_wait 不允直接 on_chain),维持不豁免回通道1**——G9 实质(席位+序数+可发现性)完整;豁免会重开"睡眠即因"语义口。cust710 §29.28 验收句演化:VSync 周期源 tertiary #5→◇ 邻近影响#2(映射表存档)。
- P2-2 trunk 名键捕获泄漏双层根修(用户裁定):捕获/attach 准入加 chain_relevance typed 臂+CLOSE-1 持席门 channel==chain 皮带(含 LeadPrimary rankless 加冕道);P2-3 陈旧工件 ◇ 序数换牌 fail-close(人口=typed 成员保留载体计数 max(1,MergedCount)+RankFoldPeers,渲染行裸计数=嘈声信号弃用;PTV6D/COV-4 两处批内 pin 依裁定改判落账);P2-4 ⧗ 入单格/EAW pin 循环("与 ⛓ 同宽度类"不实注勘正:⧗ Neutral 双语境宽1);P3 三件(cov4 头注勘正/blocking_span→⊗ 臂 pin/通道 chip 词单一词源两面同构造)。突变 11 组全红绿。
- 终形见证复跑 5/5 PASS 归档(a5/c3/a4/running_perf/donghu);cust710 cap2 复放缺件诚实跳过(berlin.systrace 1.16GB 仅客户侧),新旧序数映射表产出为客户重验收清单。

**六维 UX 审计(51 agent workflow;两波 15 CONFIRMED→11 缺陷+2 过程根因)+UXG-0 推送门批六项全收**:D1 fence 分类器 ⊘ 臂失联(六头加法扩集+F-2 假 pin 置换为引擎实铸跨包 pin ⊚×4+⊘×6;存档三形 89764/82377/87508 重渲染 classified=1)/D2 邻近序数臂+中性色 chip(trace-rank-adjacent,slate-blue 三主题;裁定=同样式中性色区分通道)/D3 ⊘ 入图标目录(⚠◇▒ 三字形显式裁定注释缓 UXG-1)/D5 徽章后补一格空格(❶⧖ 零间距灭;附带修根 off-by-one:空格挤占名列预算致 E4 持锁阻塞词丢失,在单一 fit 函数退还)/D4 图标格 overflow:visible+三档外溢预算核算(13px/12px/7.5pt;拒缩字号拒 2 列格;芯片保 hidden)/D11 ◇ primary-tier 明细「主根因」降级为 邻近(参考)。突变 8 组;L1 双 pin(fence 字节变化)复跑绿。**过程根因落账:①typed 权威在 markdown 字节边界死亡+preview 禁 import tool→五张闭集表手抄,同批 4 改 1 同步=机制上限非个体疏忽;②假 pin 拓扑=分类 pin 消费手写退役词面。收敛机制 M1-M7 立 UXG-1 批(叶子展示常量包单源+实铸 pin 政策+键集 tripwire+family 谓词强制+marks 签名机械化)**。UXG-2(D7 引擎 LineEnd clamp→D6 flat twin 车道→D8 ◌ 注记折叠去 bar)/UXG-3(D9 inversion 两 token family 单源+D10 聚合行 ◦+族词+D14 口径词教词)排批。五项通道裁定处置(用户委托):①邻近 chip 中性色✓已落 UXG-0;②聚合行 ◦+族词终局不造新字形;③「可运行等待反转」→「优先级反转·可运行等待」(UXG-3,5 pin lockstep);④口径括注贴值豁免+派生密度移行2;⑤◌ 行去 bar 化(UXG-2)。

**因果树 v5 设计稿 GREENLIT(PTV9-GRID-S,docs/design/causal_tree_v5_design_20260711.md)**:三方案竞标+三维评审,骨架=稳定性优先;架构终局=HTML 不脱离字符网格(装饰层);记号 2 格制+2ch 信封算术取代 scale 调参族(§29.36⑤ 挂 EVOLUTION RECORD:归一目标以信封兑现,实现路线换轨);恒 4 记号场/双行主行形/一段一席/确定性折叠+永不折叠白名单。F-重七条用户裁定 2026-07-11 全按稿内建议通过(重-5 取采纳臂);落地 P0→P2c 五批在本批推 main 后立项;P1 前置=§29.37 EPUB 已满足。

**mermaid HTML 截断修根(独立 commit)**:main letter-spacing .02em 泄漏进 mermaid htmlLabel(量宽容器在 main 外)→逐字符 0.32px 缺口,17/17 标签截断;修=.mermaid letter-spacing:normal+成对词面 pin(main tracking 在场哨兵);outputdump 与 preview 同一 renderHTMLPage 权威一处修两面。**tracediag snapshot v2 升级(独立 commit)**:collect_acceptance_snapshot.yaml v1→v2(inputs window/tid required+pid_from:tid),客户单行传参零编辑;budget pin 1002 红→992 绿。

**东湖标准 trace 本地验收(donghu.ftrace 3.5MB/233ms 切片,产物 customlogs/donghu_acceptance_20260711/)**:W-2 关账级证据=标准转换产物 caller 规范符号形(kthread_worker_fn+0x14c/0x1ec[devhost.elf]),§29.32 改判坐实;27845 行 unparsed=0(delay= 尾字段/[module] caller 新格式无损);W-8 干净基线=block 198/198 全配对零歧义(op=RCVHS schema_probe),精确同键并发形未现维持 witness 触发;BLIND-1 结构形不在;COV-4 partial 披露真数据工作(subject_checkpoint_missing)。**客户清单收缩至一条**(260M 原件容量点,若有)。增强候选立案:sched_blocked_reason delay= 字段语义消费(阻塞时长直出,免推断;不入本批)。

**开放**:v5 P0→P2c;UXG-1/2/3;EVALFIX-A/B/C;HYG-C;客户侧=260M 原件容量点+cap2 复放(映射表为清单)+新构建东湖 LLM 复放(本地先行);P1-2 裁定点用户可推翻(周期源 sleep 聚合是否豁免回通道1)。

## §29.39 裁定(用户 2026-07-11 晚):「未接入树」词面更名+后批工单池
**①「链上L#(未接入树)」→「链上L#(父节点未确认)」**(用户提议"未展开父节点"经辨析否决——该词把"关系未证"说成"关系已知未展示",伪造树位承诺;"父节点未确认"保住诚实换掉渲染行话)。执行=UXG-3:三面同词(§24.12 C6 记号)+zh-en 对+图例长句保留互指+pin lockstep;与「深度未解析」保持可区分(深度未解析=无层数;父节点未确认=有层数、挂点未证)。
**②后批工单池(均已裁定,待排)**:IOFAM-SELF(链上自因 lane IO 三口径一席:互证=donghu E4/E5/E6 同源实锤,block_io 复合分数在链上 lane 禁裸 ms,§29.36③ 家族折叠推广)+DSTATE-REFINE(D-state 词面消费 blocked_reason refinement 精确门:全 iowait=0→D-state/全 1→iowait/混合或不全→合并形;caller 等待对象族(dma_fence 等)入行2 披露候选;裸尾巴「· D-state」发射点归并;witness=donghu CompThread 12/12 iowait=0 dma_fence)+FIN-BIND(成文 skill 标量-主体绑定 directive+危险形示例,witness=41006 第一稿 168ms 误绑;prompt 红线 checklist+eval 复验)+delay= 字段语义消费(增强候选,与 DSTATE-REFINE 同数据源)。
**②-追加(用户 witness 41006 反转候选行,2026-07-11 晚)**:P2c 增两件——a) 恒等式家族层级化渲染(行3 恒等式/行4 拆解子行与缺口注缩进一级从属;图例 :995 教父子、现渲染拉平=结构不诚实;v5 B.1 F3+mockup :318 已含,升为 P2c 验收面);b) 零信息拆解子行门(逐分量精确判据:原始==计入∧口径==全额→不发该子行;=「退化全额折行2尾」现状规则的逐分量推广;全退化形/单分量形/折算 clamp 形/verbatim 口径词全部不动)。
**③信息契约豁免裁决规则(用户委托)**:影响扫读四问(分诊/身份/时间类别/因果角色)的信息不得豁免;仅核验用信息许留明细/证据索引;两头不沾的内部账豁免但逐条留案入豁免表,T1 census 引用豁免表(豁免表=承诺面)。

## §29.40 信息契约矩阵收账(2026-07-11 晚;18-agent workflow;13 遗漏+21 豁免全裁决)
**矩阵终报**=scratchpad/info_contract_report.md(全表+机械化规格);病理三形态:形A 铸而不读/形B wire→Node 断链"发布即坠"/形C 承诺面假指针(假注释:tree.go:251/3281/3493、trace_note_keys.go:430-437、rcr.go:859+tree.go:3682/3690)。
**13 遗漏批次编成**:UXG-1(三向 tripwire census+注册表 typed 列+假注释清理+OM-6 裁决)→ v5 P2c 搭载(OM-1① 迟到值明细格/OM-4 span 身份行/OM-2 备选)→ **IC-A P0-A 显示尾清账批**(OM-7 DrillStatus 树头披露/OM-8 锁主导降级注记=P1 误导向实锤/OM-9 邻近判据/OM-13 承接注记/OM-1② 恒等式完整修;campaign:1394/:1397 立账族收编宿主)→ **IC-L 锁族批**(OM-2/OM-10 ns 两键+假注释/OM-11 对端状态族+wait_object+peer_chain_* 伴生立案)→ **IC-E 证据索引通道批**(OM-3 边观测入册/OM-5 四态账 E#;需 v5 F10 条款增补)→ **IC-S 状态拆分批**(OM-12,§29.23 成员级论证先行)。每修复批合入必同 commit 翻契约表状态(known_gap→displayed)。
**14 条待确认豁免主会话裁决(用户委托,准绳=五问框架)**:维持 11(勘误 2026-07-12:原文误记 10,列举 11 项为准——W-2/3/4/5/7/9/10/13/18/19/21,W-7 与 W-9 带升级条件:回访/冷读再现对应追问即升 IC-A;W-18 附可达性探针;W-19 附 bundle 实证条件);收窄 3 全折 IC-A 顺手项(W-12 ⚠实际 行明细补实际段端点/W-14 audit 面补 source token/W-16 链上并入行加共N 计数)。豁免表进 UXG-1 T1 census 引用=承诺面。
**语料反查**:G-1 过程披露行族(预算触顶/降级/修复环透明度)=框架级缺口,立**裁定议题**非开工项;G-6 账债孤儿以 IC-A/IC-L 收编,落地时 campaign §17-E↔v5 G 节↔矩阵终报三向互挂。

### §29.40.1 裁定(用户 2026-07-11 晚):OM-8 反转×锁=并存披露,非降级替换
用户裁定原文要义:**锁持有与优先级反转可以并存**——有锁的反转可经"优先级传递"或"解耦锁"两路修复;无锁的反转只能优先级传递。故 Q4-D"锁持有覆盖整段时反转读法降注记"的 IC-A 落地语义修正为**并存事实披露**:行2/明细陈述两个事实(反转关系在场+锁持有覆盖整段;无锁形则反转词面独立成立),不以锁事实否定/替换反转词面;修复方向推理归正文/优化点面(合法建议面),显示面只陈述决定修复空间的事实(禁建议句红线不破)。主会话先前"用户被导向调度修理,实际该修锁"表述作废,以本节为准。PriorityInversionLockDominated 旗语义不变(锁覆盖整段的 typed 事实),仅显示词面按并存形铸。

## §29.41 v5 P0 收账(2026-07-11 晚;HTML 识别与装饰根修+对抗复核 SHIP-WITH-FIXES+修复轮五项)
**批主体**:①typed fence token(opener=```text trace-causal-projection,首 token 不变,内容行零字节;glamour 原版直测带/不带 token 输出字节相等)+新单源包 internal/tracefence/(树头闭集/token/尺度记号,tool 与 preview 双侧消费——六维审计"五表手抄"病根 fence 面根治);typed 等值硬门先行,旧嗅探降级 legacy fallback(pre-UXR-1 存档头 additive 保全),census 哨兵=引擎 10 头形剥 token 后 fallback 全真。②writeTraceProjectionGrid 五类 run(ASCII 整段/box-drawing per-rune rail/CJK 1-2ch/状态 glyph+伴随空格→2ch inline-grid 信封/█▒░ bar);scale 调参族退役换信封纯算术(§29.36.5 EVOLUTION RECORD);92300 重渲染 span -57%。③档1 装饰:.trace-line hover/区段头 sticky 横向臂/E# 页内锚链(k树↔k明细序数配对,计数不匹配整体退出零装饰=错链比无链更糟)。验收五条全过:⊘ 裸 pre 归零/真机 Chromium 几何 0 裁切/textContent==fence 字节 pin/glamour parity+L1 双 pin/内容行 golden diff 空。
**复核(SHIP-WITH-FIXES)最坏混排 20 行字节恒等+像素宽==整数列实测;修复轮五项**:F1 徽章 chip 恢复 scale(.95)(T-6 承诺兑现;复核抓到批把徽章一并退役=❶ 右弧多裁 0.5px;chip overflow:hidden 不属"调参终局"适用面;P2a 退役);F2 行盒 1.52 双向勘正(代码不动,注释+设计稿勘注;0.03 差不值重录 font pin);F4 slot-1 补 justify-content:center(Chromium place-items 对溢出网格项不居中实锤,真机 -0.78px/侧,⊘ 压「链」出血减半;字节层伴随空格留 P2a);F5 锚链只对已领 id 序数发链接(merged [E7][E9] 死链灭);F6 legacy 括号集实锤为双 ASCII 重复(全宽臂虚设),改 U+FF08+U+0028 双族+pin。F3=§29.36.5 RECORD(主会话)。突变累计 10 组(批 6+复核 4+修复轮 4 中 FM-4 首次 perl 假突变经 grep 证实后重做)。
**教训**:①CSS 布局角落(place-items 溢出项/IndexAny 重复字符)必须真机+hexdump 实证,目视与注释都会骗人;②承诺句("与 X 一致")必须逐属性核,checklist 亦是承诺面;③批内注释新写就能漂移(1.55 vs 1.52)——注释里的数字与代码同 pin 或不写数字。
**开放**:P2a 承接=徽章伴随空格+chip scale 退役+keep-mark 伴随空格+bar 列 fence 层;冷读扫荡(在飞)与 IC 系列批照 §29.40 编成。

## §29.42 41006 冷读扫荡收账(2026-07-12;90-agent workflow;81 证实/0 整案证伪/23 立案单元)
**最重两案(事实类,四门之外)**:
- **案1 BINDER-MISATTR(引擎级归因失实,P0 级)**:trace_query binder_wait 分类把 oneway 事务后的**帧间 pacing 空闲睡眠**错铸为 binder 同步阻塞——41006 报告 Rank1 叙事「binder 同步等待约45ms」trace 复算实际同步往返合计 ≈3.5ms(虚增 13×);15.758/14.302 两段实为帧周期睡眠,由 vsync 分发链(app-9511/DetectViewRect)唤醒终结;报告自家附录 wakeup 车道与正文互斥(附录才是对的);E2 ×7 折叠 7 段仅 1 段真属 binder 唤醒。**照此报告修(oneway 化建议)=修错方向**。根修=P9 binder 核销门(引擎层,精确信号:reply(T).ts<seg.start 拒归因/终结 waker.tgid==peer.tgid/段长≈帧周期∧waker∈vsync 链→改铸 pacing_idle 车道)。
- **案2 主因方向反转**:目标线程窗内 running=157.248ms(67.4%)=running 主导,报告结论「CPU 自身执行不弱,卡顿主要来自外部等待链」方向反转;「约168ms」任何口径不可复现且自标"请谨慎采信"仍撑判决;真实掉帧=13762.8095→.9374 约 128ms 无 doFrame。
**其余 21 案四门分组**(乙 精度不降 4:D-state 跨CPU折叠冒充单段/覆盖句双失真/span 冒充窗口/优化点校验环无声剥除;丙 增量 4:根因#1 机制现成标未解析(GPU fence,sched_blocked_reason 在手)/关键角色裸线程名无 tgid/下一步不可执行/报告 48.5% 逐字重复;丁 同源 6:E1/E2/E3 同段重贴标/同根因五量级零对账/rank-链双车道相邻双行/折叠行成员全已显/快照邻域冒充覆盖/keva 数值跨实体嫁接;戊 结构 7:双第一根因+排序键自违/徽章 TOP5 违约 #2 折叠吞席/⚠ 词面 11 行全假/供给缺口双计入规则并存/×N 实际状态列 seed 冒充/RT 数值误绑+词表外自造 token/块IO链归属词面反转 typed 车道)。12 处子主张复核收窄记录在案。
**机制终案(三关正交验证协议,固化为战役纪律)**:①T1 契约矩阵(机械,有据无显)+②四门 pass P1-P12(机械:P1 prose 数值溯源(实体,量纲,值)三元组集合成员/P2 词表门/P3 榜位单一权威/P4 徽章-图例闭合/P5 同段指纹收敛/P6 墙钟守恒/P7 口径标签 typed enum/P8 ×N 范围守真/P9 binder 核销/P10 blocked_reason 消费义务/P11 tgid 归属槽/P12 重复块门;19/23 案可机械拦截)+③**冷读关**(独立会话+原始 trace,显示批验收必做,验收句必引冷读结果禁修复自证;唯一能抓"引擎证据面本身失实"的方向——案1 各显示面对错误证据忠实且一致,内部验证全体沉默)。纯冷读残余=方向判决/分组键语义/可操作性/词面语用/叙事互斥。
**批次编成**:**CR-1(第一批,最高优先)**=P9 引擎根修+P1/P2 prose 溯源与词表+P3 榜位权威;CR-2=P4/P5/P7;CR-3=P6/P8/P10/P11/P12;案7 GPU fence 并 DSTATE-REFINE(同 blocked_reason 消费);案8 tgid 并 P11;案10 重复块并 P12。CR-1 插队至 UXG-1 之后、v5 P1 之前(分析正确性>显示工程)。

### §29.42.1 裁定方向(用户 2026-07-12):排序双视角并存
用户裁定方向原文要义:排序按两个视角并存更稳——「窗内可消除量(折算后有效归因,交集到窗)」与「有效归因(折算后)×置信档×类型权重」。主会话护栏(经用户本轮讨论确认的设计前提):双视角≠双序数芯片(冷读案17 双第一根因=事故形先例)——健康形=**主榜保持现行复合键**(根因排序#N/❶..❺ 佩戴唯一归属)+**「窗内可消除量 TOP」摘要/导航层**(纯可消除量降序,值+E# 指针回主榜行,零第二序数、零佩戴)。两把尺/佩戴单门/身份诚实零触碰。此方向作为 rank-order-v2 设计研究(在飞)的预设综合方向,终稿仍以真实板 mockup 验证后 NOT GREENLIT 呈裁定;生效前提=CR-1 归因正确性先修。

### §29.42.2 补充(用户 2026-07-12):模型作答面榜序一致性并入 CR-1
用户提问确认:模型可见的作答用根因顺序需一致优化。CR-1 工单扩三层:①**单源摘要喂入**——成文上下文携 typed 榜摘要(权威序+每行 有效归因/口径词/通道身份/置信档;双视角落地后两视角显式带标签),消除模型自造第三序/自造聚合数(案1 约45ms 形)的理由;②**教学先行**(FIN-BIND 家族):按 typed 榜序陈述禁重排/禁跨行求和/通道词按行 typed 通道(案23 链上行被叙成背景噪声形);③**P3 闸兜底**(prose 榜段实体序列==typed 序列+排序键单调,精确信号)。红线护栏:不采用"确定性回填 prose 榜段"臂(贴系统不代写红线)——榜的结构化列表本就是确定性渲染面,模型散文只做围绕综合并受序列闸约束。

### §29.42.3 修正(用户 2026-07-12):P3 榜位闸软化为两臂
用户裁定:P3 硬等式闸风险过高——对系统正确性要求太高,须给模型自由(模型可见信息面与判断力强于系统;案1 实证 typed 板自身可错,附录 wakeup 车道对而正文错=模型手中本有纠偏素材;37 连拒死锁史为硬卡反例)。修正:**P3a(硬,窄)**=只拦 prose 自身矛盾(同段双"第一/主因"最高级宣称、prose 自声明排序键对自给数值不单调——内部矛盾与谁对无关,精确信号);**P3b(软,单轮 latch)**=模型序≠板序时的**披露义务**(§29.20 conscious-flip 先例):静默偏离→一轮定向重写提示(对齐或显式说明综合判断依据),有披露的偏离直接放行;永不成拒绝环。单源摘要喂入(§29.42.2①)升格为该设计的前提——给自由的前提是喂最好的信息。原 §29.42.2③"P3 闸兜底"表述按本节作废。

### §29.42.4 裁定(用户 2026-07-12,最终形):排序一致性车道全线无硬拦
用户裁定原文要义:模型经 trace_query 拿到的 root rank+综合信息已很全面,客户反馈模型回答大概率相当不错;**一定不要硬拦**——排序问题本身是小,不要因系统自作主张颠覆回答正确的内容。落地:①P3 全臂(含 §29.42.3 P3a 内部矛盾臂)统一降为**至多一轮软提示**,第二稿维持即照发;②排序车道任何情况不硬拒/不进重试环/不阻断出厂,兜底=如实出厂+至多 advisory 日志(不注 caveat,系统不往答案塞话);③正确性投资方向=P9 引擎铸数修根+单源榜摘要喂入+FIN-BIND 教学——喂好数据、教好方法、信任模型;④P1/P2(纯捏造面)维持 prose_scalar 先例上限(单轮 latch 后照发),同样永不硬拦。元原则入 checklist:**系统与模型分歧永远走披露/软提示,硬门只留给"无论谁对都成立"且不阻断出厂的判据;答案出厂权属于模型**。

## §29.43 rank-v2 设计研究收账(2026-07-12;9-agent 三方案竞标;终稿=ELIM-1+V2-P0,NOT GREENLIT 待用户点头)
**终稿**(docs/design/rank_order_v2_design_20260712.md):骨架=分区演进案(三评审一致首位)。**主榜一根毛不动**;板首加「◎ 窗内可消除量总览」只读导航 stanza:链∪◇ 同尺 typed 持值行按发布 EffectiveImpactMs 纯降序 TOP5+◇ 最大保底+排除脚注+▒ 去向指针;零序数/零佩戴/不求和/不加冕/单源转录(值+身份词+口径注记+E#)。排序键定稿论证:拒 eff×conf(S1/§29.22.1 合成分数禁发布先例)、拒置信降档(mint 期类型常量=嘈声,禁作硬门);置信不进键是刻意的,弱证据霸板首的处置=修值(CR-1)非塞键。▒ 排除走第一性(跨线程行无定义好的窗内可消除量,真凶成分已供给折算升舱)非身份论证。伴生 **V2-P0 行级尺守卫**(count/复合分数行出序数入 ⌗ 口径旁栏——两把尺从通道级下沉行级,无论 ELIM-1 是否通过都该做;与 IOFAM-SELF 合并执行)。重裁清单:0 推翻/2 措辞补充(R5 图例句+R7 边界句)/3 新裁定(A=V2-P0/B=升舱换值 pin/C=tier 词对齐)/1 新立案(ADJ-MINT 铸造种群)。pin 量级:ELIM-1 引擎 0 文件、徽章 chip 0 迁移,≈统一榜方案 1/5 成本,半区回滚=撤一个 stanza。落地序:CR-1 硬前置 → v5 P1 后 ELIM-1 形制批;V2-P0 并行道。
**开放项主会话按用户既定委托取默认(可推翻)**:O-1 否(总览不转录徽章/序数,严格 §29.42.1);O-2 ◎ 记号+图例句照稿(tracefence 查重后);O-3 不带置信注记;O-4 新裁定 A/B/C 与 ADJ-MINT 立案批准;O-5 ✦-only 形取指针行(不虚构种群);R5/R7 措辞补充批准。**ELIM-1 主裁定(R16)用户 2026-07-12 裁定「按默认(最优)建议执行」=GREENLIT**;设计稿状态行已翻。

## §29.44 UXG-1 收账(2026-07-12;显示层机械化收敛+对抗复核 SHIP-WITH-FIXES+修复轮四项)
**批主体**:①五表单源化收尾(internal/tracefence/display_tables.go:状态记号目录 13 glyph/action token/序数通道短语+徽章族/章标题三对/auxfold 标记;preview 纯派生+tool 发射器派生;grep tripwire 首跑抓真实第二抄一处);②三向 census——T1 全字段契约表(Node112+Proj25+Account12+QW2+FoldPeer7,known_gap 恰=OM-1..13 机械断言,豁免表 21 条含理由+升级条件,幽灵行/翻账/越权四臂)、T2 armA NKR 载体真值 census+armB RankItem90 镜像、T3 图例哨兵+承诺 census;③M3 键集包含(删手写清单);④M4 family 谓词强制(inversion 双 token 收敛进单谓词+两族 grep tripwire);⑤假注释清理 7 处(逐处对码零假话)+auxfold 注释改指常量包。
**复核(SHIP-WITH-FIXES;12 组独立判别力探针全红,豁免表逐条对账)→修复轮四项**:F2 tool 侧表③残余手抄六处+同族四处全改 tracefence 派生(wrap-atom 折行边界表=字节形消费者最危险面,FM-1 突变实证改常量一字→发射面/wrap 面 12+ 同红)+散文残余走 per-file 计数 allowlist+承诺句改述;F3 census OM-11 恒真硬码销(改机械读 types 账本块提取引用,"恰13"不再钦定);F5 ActionWordENShort 入表+唯一红面 pin+preview 豁除注记(前缀腰斩风险);F4 计数盲区注记。**F1 宣称诚实化(收账口径)**:本批用户面漂移=markdown/fence 字节零变化;standalone HTML 恰 4 行漂移且全部为 <style> 内 CSS 注释声明性文字(零渲染效果)——tripwire 剥 Go 注释但 Go 字符串内 CSS 注释按字面命中被迫改写,该注释随 HTML 出厂;"零用户面字节变化"原表述按此收窄,不给 tripwire 开 CSS 注释豁免(注释手抄词正是该抓面)。
**机制意义**:自此"新增 typed 字段忘显示/图例承诺未兑现/词面第二抄/family 本地重枚举"四类病提交即红;13 条 OM=known_gap 挂账,修复批合入必翻表;豁免表=承诺面。**教训**:①"字节零变化"类宣称必须按面拆分陈述(md/fence/HTML 各自口径);②census 引用集禁钦定(恒真硬码=假验收);③tripwire 首跑抓到的每一处"注释里的词面"都是真分叉面。遗留:第六张手抄表(render↔preview trailing disclosure 标记)同模式后续收敛;M4 计数式盲区(报红必真、漏报可能)有第二张网。

## §29.45 Donghu / OpenHarmony counter right-parse 收账（2026-07-12，`539b9be05`）

**typed admission 与语法终态**：counter批当时将`ParserVersion`升至v22；direct marker右边界身份批`134762ac7`随后将现行缓存schema升至v23，v22只表示本段历史落点。完整 `C|` 载荷在 300-byte 展示副本截断前只解析一次并存入稀疏 side-table，Event name/value、搜索、inventory、duration-order、namespace/TID-TGID vote、delta 与 Top-N 全部消费同一 verdict。语法固定为 exact `C|owner` + opaque pipe-name + plain decimal value + optional HiTrace metadata；owner 仅 `0..MaxInt32`，scalar 内嵌空白拒绝，名称中的 `|` 与边缘空白保真，仅全空组件拒绝。尾 metadata 依上游 `ParseTagBits` 可达闭集解释为 output level + tag-bit indexes，COMMERCIAL bit 仅能配 level M；它是 provenance，不是 instance/track identity。同一 physical source、owner、exact name 的 metadata 漂移整 series fail-close，不能拆成两个貌似真实的 track。

**数值、边界与排序**：native int64 只有在 legacy float64 公共面可无损表达时发布；`2^53` 邻接碰撞、超域、非有限、科学计数、subnormal 与超过 15 位有效数字的 decimal 兼容形均保留 inventory 并抑制派生宣称。delta 与 Top-N 键直接基于原 token 的 `big.Rat`，最终公开时才舍入；整数 delta 不能无损表达时整 series 抑制。direct 与 action-restored carved 路径都在 TrimSpace 前守 1024-byte kernel marker 边界；没有 producer profile 时 `>=1024` 只入 inventory。side-table 内存计费、控制字符/bidi 单行转义、source/order/budget 与 metadata-change 均有机械 pin。

**标准 Donghu 复放与验证**：基准 `/Users/han/opt/donghu/donghu.ftrace`（SHA-256 `e15d3dfc7963739c648a3f4f40095cabff19716575949bf38ea02ef732672b25`）当前 27,843/27,843 events，0 unparsed/panic/rollback；counter 为 `653/653 valid identity`、`653/653 numeric`、52 logical series、0 issue。focused `-count=20`（冻结复核至 30）、race `-count=5`（冻结至 10）、两包测试、最新远端重放后的全仓测试与 `go vet ./...` 全绿，三路独立协议/对抗/冻结复核均 RELEASE。

**转换格式兼容红线（用户 2026-07-12 再确认）**：鸿蒙/东湖转换输出以该标准 Donghu 样本中实际存在的字段、字段顺序、标记、标量和转义为主要兼容基准；存在即可证明的内容必须 byte/semantic compatible。代码中来自其他标准 trace/profile 的合法能力可以保留，但必须由独立 typed producer/profile 选择，禁止串 profile，禁止为了“看齐”补造缺失 CPU0、page0、device、tag 或默认值，也禁止因 Donghu 样本未出现就删除其他标准来源已证明的格式。比较和回归必须按 profile 分层，不能用自由文本、文件名或单个样本缺席充当 hard gate。

**诚实开放的能力残口**：①OpenHarmony app-file 可产生完整 `>1024` 行，而 kernel marker 可能在 1024 截断；终态需要 converter/bundle typed producer provenance。②官方 CountTrace 支持完整 int64；若客户需要超过 float 精确域，新增 decimal-string/int64 wire，不能放宽现有 float 面。③action-lost `0xIP:` carved counter 与 opaque Begin 名称存在字节同形歧义，可能影响 namespace/TID vote；需保留原 action/provenance 后再消歧。④OH B 可带 customArgs，S 可带 category/args，并可叠 chain envelope；不能机械套用 C 的 final-metadata 算法。⑤Event/EventView JSON 当前只是输出 DTO；若未来成为 Index 反灌入口，必须显式序列化 typed counter verdict。B3-b2 raw complete-set anti-rescue已由`3d9555cdb`+`a856f1d45`关闭，direct marker authority/右边界身份又由`2f3c69a5f`+`134762ac7`关闭；下一trace correctness批按账本推进B pair-critical Workqueue/DMA→C I2C/SMBus→D page/writeback→E MMC/F2FS/EROFS→R1b-C，block/storage request token identity仍等待两端生产witness。

## §29.46 SQL raw B3-b 收账（2026-07-12，B3-b1/B3-b2 均已交付）

**问题不是基础 scalar 重做**：既有 `cbb276f3e/e61388803/8a69cd5e5` 已关闭 raw stable-ID、SQLite scalar、CPU0、argset 与 Running source-taint 基础边界；剩余 correctness 是 raw 仍消费 legacy Running、没有使用 scheduler 同实例 generation authority，以及逐行删坏 endpoint 后可能让前后合法 endpoint 在下游跨洞配对。故 B3-b 分两次独立推送，严禁用第一批完成冒充整项结案。

**B3-b1（先行）**：唯一 production caller显式传同一 `traceDBSchedulerAuthority` 与同一 lifecycle-filtered typed Running。subject闭集固定为 positive canonical thread、PID0 kernel thread与exact canonical idle；三类均先过 capture/lifecycle point。真正不存在任何canonical/rejected候选的source-only在冻结复核中勘正为coverage-only：没有独立typed inventory/profile前不发布任何标准或私有ftrace wire。`ObservedPublicTID/RejectedPublicTID`、PID conflict、歧义、point失败均不得降级；explicit CPU只能免 Running lookup，不能免 lifecycle。完成后删除 legacy raw Running lookup与相应结构豁免，并以身份/cut/poison、CPU四态、source-only零wire负对照和唯一调用图机械锁定。

### §29.46.1 B3-b1 收账（`40a254c3d`）

唯一production caller现把同一个scheduler authority和已构造的lifecycle-filtered typed Running直接传给raw exporter，旧`traceDBExtendedRunningCPUAt`及其taint字段已从生产树删除。capture lower-bound、canonical subject resolution和thread/process point admission支配两条CPU车道：合法显式CPU（包括CPU0）只免Running lookup，不能绕lifecycle；NULL/缺列CPU只消费typed Running，并把known、source-tainted、lifecycle-rejected、unknown四态分账，invalid explicit CPU禁止fallback。

public `0`仍是present claim：positive user的TID/PID 0冲突；PID0 kernel只接受TID缺失或exact positive TID、PID缺失或0，header投影为`TGID=TID`；idle只接受exact ITID0与零/缺失public claim。TID reuse由canonical candidates和PID精确收窄，identity audit拒绝过的candidate不能伪装成“从未观察”。source-only按pairing-capable、缺CPU、合法显式CPU、invalid CPU分别计coverage，但全部零wire；复核期间曾评估的“PID0 page-cache匿名载体/私有payload前缀”被判定会污染标准profile，已完全撤销且有结构/行为pin证明零残留。

验证完成：focused/结构`-count=20`、focused race`-count=5`、`go test ./internal/hitraceconv ./internal/tracequery -count=1`、`go vet ./...`、`go test ./... -count=1`、gofmt与`git diff --check`全绿；独立B3-b correctness复审为SHIP，独立Donghu profile复审为RELEASE。该提交只关闭subject/lifecycle/CPU车道；B3-b2五族完整集anti-rescue、anonymous raw inventory的独立typed profile与page-cache fidelity继续开放。

**B3-b2（随后）**：在任何 endpoint 落 sink 前完成 raw 全集审计，用有界 typed stage/freeze把 binder、workqueue、DMA、block与generic storage的坏 exact key局部化为 lane barrier，不可定位key升级family-global；五族key与tracequery单点同构或以跨包parity pin证明。valid→bad→valid不得跨洞铸IPC/时长，无关key与无关family必须存活。两批的格式验收继续遵循§29.45：标准 Donghu 已存在字段必须兼容；其他标准trace格式走独立typed profile，既不因样本缺席删除，也不串profile或补造默认值。

### §29.46.2 B3-b2 测绘校准（拆为 B3-b2a/B3-b2b）

只读调用图证明，下游五族配对键目前尚非可直接复用的单点权威：block/generic-storage helper存在零生产caller；workqueue work指针与DMA context/seqno只做数值合法性检查，却以原字符串入key，数值等价异形会分裂lane；Binder按全局transaction ID聚合，没有physical artifact namespace，且receive不被消费，两个同ID send可能复用一条receive。另一个族边界是exact idle：block/storage可保留，Binder/workqueue/DMA必须拒绝并形成barrier。直接在converter另写一份key会固化漂移，故禁止开工。

施工拆分为两个独立推送。**B3-b2a** 先在tracequery建立唯一typed endpoint fingerprint：source是artifact namespace而非payload字段；body key保持既定闭集，work指针及DMA无符号量规范化；Binder以`source+transaction_id`运行有序cohort，一条receive只消费一次，重叠同ID整cohort抑制，顺序复用可恢复；三族idle负门机械化。**B3-b2b** 再建立raw私有有界typed stage，直接消费该fingerprint并在scan后seal/freeze：坏exact key只污染该lane，key/owner不可定位升级family-global，所有 governed endpoint只能从唯一pass-2进入最终sink。generic storage继续使用`layer/base/dev/inode/op/header-TID`粗键，严禁用尚未获生产witness的tag/lba偷偷升级request identity。

### §29.46.3 B3-b2 收账（fingerprint `3d9555cdb`；SQL freeze `a856f1d45`）

B3-b2a 已建立五族唯一typed fingerprint并关闭Binder receive复用、numeric异形分lane、idle边与windowed topology误配；B3-b2b再让SQL raw无序单遍scan进入私有有界SQLite stage，按stable physical order完成lane/family freeze后才由唯一pass-2发布。exact坏key局部隔离，unknown key/owner升级family；WQ/DMA/storage跨canonical ITID未闭合cohort隔离，Binder/Block保留协议允许的跨emitter。duplicate stable cohort、timestamp rollback、same-ts稳定序与 `valid→bad→valid` 均有E2E pin。

stage硬界为4M physical rows、每family 1M lanes、4GiB temp；两个排序面均由私有索引承担并以query-plan禁止temp B-tree，源库查询机械禁止`ORDER BY`。CRC poison journal按精确字节计费；replay期间用journal实际大小动态收紧SQLite `max_page_count`，每次insert后复核实际组合占用，任何record/page/temp/sequence预算均在首行发布前fail-close。MMC/SCSI signed tag、独立MMC errors、work-only WQ、`fs_dev`、full-uint64 DMA与generic inode wire parity同步关闭。focused×20、race×5、真实SQLite FULL临界用例×50、两包与全仓test/vet、格式/diff检查全绿，两路独立终审RELEASE。

本节只关闭B3-b correctness。`loadArgsets` bounded spill、anonymous raw inventory、compact pairing-topology sidecar、generic block/storage request identity生产witness、remaining ftrace payload admission、page/marker fidelity、R1b-C、R2 snapshot与`ROW-SORT-BND`继续开放；下一最高ROI批为ftrace payload admission P0。

### §29.46.4 remaining ftrace payload admission P0-a 开账（2026-07-12）

先关闭structured当前支持descriptor的完整wire/duplicate/malformed矩阵，以及direct最关键的sched wake/blocked、CPU、Binder、IRQ/softirq body。实现必须是共享typed payload、两个source decoder、一个canonical renderer；known direct descriptor坏payload局部抑制，不能再降header-only，structured坏sibling不能杀整plugin。OpenHarmony `developtools_profiler@5bc8ef555d53a9fcf3d2d8c1e59595d39d949b01` 的default/6.6.30 proto与generated parser证明这些profile会无条件`set_*`，故仅对应proto3 scalar omission可解释为精确0；direct missing、任何wrong-wire/duplicate/malformed都无默认语义。payload CPU不继承header CPU，线程名display-only，140..159优先级继续合法。

P0-a明确不混direct marker能力、page-cache单位、storage/UFS/I2C/MMC/F2FS/workqueue等剩余direct descriptor；marker尾空格/长行/1024 provenance与page profile仍独立留账。验收固定为逐descriptor对抗fixture、合法0/坏sibling locality、direct↔structured parity、focused/race、包级/全仓test+vet、格式/diff检查和独立冻结，完成后立即推送，再开P0-b。

用户勘正：Berlin文件是旧转换器产物，只能作为异常触发和规模参考，不能充当格式/字段权威，也不能凭ASCII caller反推原始layout；转换格式继续以标准Donghu已存在内容为主，未出现族按各自独立typed上游schema兼容。descriptor-ID段内/跨段last-wins由当前代码直接证明，故仍先以exact equality关闭冲突ID整lane，后续重复不得救活；structured IPI 1400/1401/1402则依据OpenHarmony官方proto/parser补齐，不以Berlin数量作hard gate。`ProfilerPluginData` Name/Data及top-level ftrace detail也纳入wire/unique门，避免payload门前仍由container last-wins。

P0-a0 descriptor authority 已由 `6f9637068` 修复并推送：同段/跨段完整相等block幂等，任何同ID冲突或可辨畸形永久quarantine且不可被后续clean救活；duplicate ID/print/clean-field、bad field/signed/name、reserved-key空白和grammar注入均有机械pin。关联raw row仅计coverage、不进入missing/unknown或header-only发布；sibling locality与all-poison标准header/caveat均有E2E。focused×20、race×5、包级/全仓test+vet与两路独立RELEASE完成（压力复核focused×100/race×20）。该批未关闭body payload：下一小批仍为direct typed decoder + known-reject，再推进structured descriptor闭集。

P0-a1 direct core body 小批已冻结、代码未交付：closed family为wake/waking、blocked、CPU freq/limits/idle、Binder tx/received、IRQ/softirq/IPI。实现必须是source-neutral typed payload + direct decoder +唯一canonical renderer，并把production verdict拆成`unsupported/admitted/rejected`；known-family坏body只能局部拒绝并按闭集原因披露，不能再回落header-only。PID/CPU/source width/signedness/alias/string均从物理descriptor与raw bounds精确判定，CPU0和priority 140..159保真，payload CPU不继承header CPU，线程名仅display。Donghu实际存在族做golden，缺席族按OpenHarmony pinned schema，不以Berlin旧转换文本裁格式。本批不改structured/marker/page/storage；提交推送后再让structured decoder消费同一typed payload。

## §29.47 标准 Donghu profile 差分审计立案（2026-07-12，未收账）

**基准边界**：`donghu.ftrace` SHA `e15d3df…` 含27,843 events、14种event；sched_switch 4,670/4,670有next_info，blocked 438/438有delay，page-cache 2,907/2,907为page/pfn/ofs形。该文件证明 Donghu profile 中“存在什么”，不证明其他标准profile中“不允许什么”。CRLF、无systrace header与当前LF+生成header是容器差异；线程名会变化，禁止参加profile判定。hmtrace SQL的keyless `clock_set_rate: <name> <value>`是另一已证标准profile，不能被Donghu keyed形全局替换。

**P0 remaining ftrace payload admission**：direct RMQ 尚未让wakeup/blocked/CPU/print等剩余descriptor经过共同endpoint准入，missing/wrong-wire可被补成合法0，控制字符串可形成伪物理行；structured profiler的strict wire audit同样未覆盖binder/print/IRQ/CPU/wakeup/blocked/F2FS/MMC等全部descriptor，duplicate/walk错误仍有后值覆盖或整plugin失败的风险。B3-b完成后立即开独立P0：每个精确producer descriptor审core/optional的presence、wire、range、唯一性、UTF-8和单物理行，坏sibling局部抑制并计coverage，禁止默认CPU0/prio0/空串，禁止按事件名或线程名猜profile。

**P1两项状态勘正**：①SQL raw page-cache把页index与byte offset混在一组alias，`index=1`会错误发布为offset 1；仍应拆Donghu filemap typed tuple并checked `index<<12`后复用共享renderer，其他标准byte-offset profile独立保留。②Golden print的8条真实尾空格marker与28条>300B payload已由`2f3c69a5f`+`134762ac7`关闭：marker专用decoder/renderer保留合法边缘空白与pipe并守UTF-8、单物理行、容量，tracequery v23消费完整typed identity。故当前只剩①仍开放。P2为SQL header emitter identity projection披露，P3为删除/诚实化`%lu/%d`内核计数占位头。详细施工证据与优先级以trace gap施工账P0-a3为准。

### §29.43.1 增补(用户 witness 2026-07-12:总览折叠行类型不可见):折叠行词面必携类型
用户指认 ELIM-1 mockup 中「其余5项(链上折叠)·×5 取最大」类行**影响类型一眼不可见**——五问第③问(哪类时间=修理手册入口)在折叠行整体缺失,且该形坐总览高位。裁定(按既定委托):**折叠行(主榜 F8 族与 ◎ 总览转录同源)词面必携两个 typed 要素**:①**类型 census 短语**——成员类型去重计数(同质形=单类型词「其余5项(链上折叠·D-state)」;异质形=top2 类型+等「(D-state×3/binder×2)」),数据源=fold peer TypeWord(OM-6 字段,写而不读的死数据即本修的现成供给——OM-6 处置 A 臂词面落点由此定为折叠行名字场,取代原"明细(b) 链上并入行"落点);②**最大成员身份**——「取最大」值即最大成员的值,故最大成员的 线程·类型 可点名(宽度预算内优先 ①,② 进行2/明细)。落批:主榜折叠行词面=CR-1 戊组/P2c(OM-6 A 臂);◎ 总览零新铸自动继承(单源转录)。同门齐修:异质 census 词序按量级降序确定性;zh-en 同词;×N(a–b) 范围词保留。

### §29.42.5 资产登记(用户 2026-07-12):donghu.ftrace=eval 测试集金样本
用户指定:/Users/han/opt/donghu/donghu.ftrace(客户常用鸿蒙系统东湖 trace,3.5MB/27844 行/233ms 高密切片)内容非常丰富,可构造各种场景 eval 测试集。**立案 EVAL-DONGHU 批**(排 CR-1 之后——P9 改 binder 语义,oracle 须对修后正确语义建):候选场景族=①aweme 主线程卡顿(41006 形,binder 真等待 vs pacing 空闲双 oracle=P9 的正负两形)②CompThread D-state/dma_fence(非 IO D 态+等待对象,DSTATE-REFINE witness)③IO facet 家族(fscache 缺页读三口径,IOFAM-SELF witness)④runnable/供给(12CPU 全低频+热限压 1.53GHz,供给折算场景)⑤blocked_reason delay= 语义消费(增强候选)⑥⊘/◇/▒ 通道混布显示终形。oracle 全部 trace 逐行可复算(冷读三关第三关的常驻输入);已产出五件确定性采集产物在 customlogs/donghu_acceptance_20260711/。

## §29.47 CR-1 收账(2026-07-12;撞号注:commit message 引用 §29.45 为撞号前编号,以本节为准——远端同批已占 §29.45/§29.46;分析正确性批=P9 引擎修根+软门三臂+榜摘要喂入+FIN-BIND 教学;对抗复核 SHIP-WITH-FIXES+冷读关+修复轮九项)
**件① P9 binder 归因核销门**(binder_attribution.go):臂a reply 核销(reply<段起点禁归因,无 reply 行 fail-open)/臂b 对端一致(waker tgid≠peer 禁归因;reply 段内例外保归因+披露 caveat)/臂c pacing 改道(帧周期容差 2.0ms+合理带 4-50ms+frameScheduleEmitters 帧链门;帧源→pacing_idle「帧间空闲(等待下一帧)」,非帧周期源→periodic_idle「周期空闲」,两车道均 ContextOnly 不参赛;R2' 全同步)。donghu 复放:binder 从「约45ms」收敛恰 1.409ms 真事务;**复核全量复算洗清过度核销疑虑(五笔真等待 0.05ms 底限下 5/5 存活,「恰1」=既有 1.0ms 时长底限)**;修复轮补单臂独证 fixture(影子对灭)。
**件② ENG-1 running 合计截断修根(冷读发现的引擎病)**:computeThreadCPULoad 曾从已截断 top-8 切片求和(132.041=两核切片冒充全窗)——合计改全窗全核真和,帽只限展示行;donghu 实铸 157.248/5.604 恰合冷读全量状态机复算;第三次复放 wire 实证 132.041 归零。
**件③ 软门三臂全软实现**(§29.42.3/.4 逐字):P1 约-扩展(5% 相对容差+复合词表(节/制/预/合/契/隐)+范围记号;约-标记撤 pair-sum 豁免)/P2 词表门(registry∪run 证据∪工具输出∪问句)/P3a 双主根因+声明键单调/P3b 榜序静默偏离(board-head 臂修失火)——单轮 latch、第二稿维持照发、永不硬拒(TestProseLexiconBoardNeverHardRejects)。
**件④ 榜摘要喂入(成文+探索双阶段)+FIN-BIND 五 directive(七条 checklist 过检)**。**第三次复放结构性胜利:榜摘要进探索阶段后,模型第一稿即与 typed 榜逐席同序同值——84618 的双榜/自建榜/binder 误归因全部消失,软提示无需触发。「喂好数据、信任模型」路线实证**。
**冷读关(独立会话+trace 复算,84618→96236 两轮)**:第一轮判引擎面"接近可交付水位、历轮最干净",病灶收敛 prose 车道;第二轮(96236)三验收点=running 157.248 ✓/榜齐 ✓/pacing typed 车道在场、树面词面残口(下)。两 eval PASS(donghu+a5)。
**遗留跟批(STAB-1+ENG-2 残口,新 agent 接)**:①STAB-1 软修复轮稳定化(用户裁定,witness=2779 案:软提示含 3 个内核事件名误报+模型 replace_all 整篇重写致第二稿劣于第一稿)——S1 词表补附件原文 token 语料+标准内核事件词典;S2 最小 diff 修复义务(只动被点名 block);S3 **两稿仲裁:第二稿仅在严格更好(点名违规清除或披露保留∧零新违规∧无块/覆盖丢失)时取代第一稿,否则发第一稿**——"永远发最后一稿"的隐含假设换成"发更好的那稿";系统只选稿不改稿,strict 轮不受影响。②ENG-2 第三条折叠机(E1(+11) ×N 自身睡眠族吞 idle 注记,复现要点在案)。③F5-1 churn 覆盖声明(CR-2/P7)。
**流程教训(三连实锤)**:agent 停等后台任务未被唤回今日三次(eval 落盘后无回调)——**新纪律:主会话见 agent 停靠即核对其后台任务实际状态(ps+产物目录),超时未唤回即接管收口;已验证完毕的工作不压在停等 agent 手里,拆批先行收账**。本批即按此接管:CR-1 主体先收账推送,尾批另起。

### §29.47.1 裁定(用户 2026-07-12):软车道零重写+系统校验附注(取代单轮软提示与两稿仲裁)
witness 两案两中(2779:三内核事件名误报;76278:fscache_page_wait vs 引擎自身截断形 _o 近正确引用被判自造)——软机械发现触发整篇重写,第二稿两次劣于第一稿。用户裁定原文要义:**系统检测是机械性的,信号精确但在成稿阶段对用户问题整体把控不足,太容易误报;系统不能在非致命问题上硬卡,应该相信模型;系统自己检查出来的通过"补充体现"呈现**。落地(STAB-1 S3'):①soft-only **零修复轮**,第一稿直接出厂(§29.42.3 单轮软提示与 §29.47 两稿仲裁在软车道作废);②系统发现渲染「系统校验附注」独立确定性块(系统自声版面,谦逊导语+用户可读逐条,零发现不渲染;红线界定=禁替模型写散文≠禁系统在自己版面说话,因果树/明细表同族先例);③strict 车道(致命窄类)保留单轮修复但强制最小 diff block patch+仲裁看门(不严格更好→发第一稿+附注);④「第一稿保留区」软-only 情形退场。原则沉淀:**检测与执行分离——机械检查永远可以看,但只有"无论谁对都成立"的致命类才可以拦;其余一律转信息面**。

### §29.47.2 增补(用户 witness 2026-07-12,96728 案):DSTATE-REFINE 范围扩大
20260712-100939.634-96728.md E14 行实锤新病形:行1 已细化「iowait」,行2 类别词仍发「D状态候选」、行尾仍挂裸「· D-state」——**类别词与尾 tag 按家族铸造,不消费已细化状态,同行三面三说法**。DSTATE-REFINE 工单(§29.39②)范围扩三臂:①原臂(合并车道词面精确门:全 iowait=0→D-state/全 1→iowait/混合或不全→合并形,E16 混合形主词面=诚实保留);②**类别词臂**:「D状态候选」族类别词消费细化态(iowait 行→「IO等待候选」族,已细化 D 行→「D状态候选」,混合或记录不全→「D状态/IO候选」——用户补正 2026-07-12,45261 E9 witness:裸家族词对混合行名不副实,混合词形须与行1 合并主词面同构);③**裸尾巴臂**:「· D-state」尾 tag 发射点归并(细化行/混合行两形都在);caller 等待对象族行2 披露与 delay= 增强候选原臂不变。witness=96728 E14/E16 追加入工单;排位不变(尾批收口推送后即开引擎口径批:V2-P0+IOFAM-SELF+DSTATE-REFINE)。

### §29.47.3 调度调整(2026-07-12,用户连续撞到已裁未落词面项):两更名提批
用户在 96728 连续指认「未接入树」仍在——已裁词面项排队过深的调度病。调整:从 UXG-3 拆出两个**裁定已终局的纯词面更名**提进下一批(引擎口径批同车,批名扩为 CAL-1):①「链上L#(未接入树)」→「链上L#(父节点未确认)」(§29.39①:三面同词 C6+zh-en 对+图例长句互指+与「深度未解析」可区分+pin lockstep);②「可运行等待反转」→「优先级反转·可运行等待」(§29.40 处置③:5 pin lockstep,D4 组合形原 token 括注保真)。CAL-1 批=V2-P0+IOFAM-SELF+DSTATE-REFINE 三臂+两更名;UXG-3 余项(D10 聚合行 ◦族词/D14 口径教词/D9 残余)排位不变。**调度教训落档:用户可见且裁定终局的词面项不积压——凡"一句话裁定+机械 lockstep"类,搭最近的批走**。

### §29.47.4 增补(用户 witness 2026-07-12):CAL-1 扩两件
①**IOFAM-SELF witness 升级+三❶违约实锤**:最新复放自因区五行 IO 平铺(io_latency 3.670/block_io inode 2.694+2.116/io_wait 对端 udk-irq 1.347+1.248)零层次表达,且 E1/E2/E3 **三行同戴 ❶**(家族成员各携 lead 席位=徽章单点权威违约,CR-2 P4 族合成病)。修后形=一席+成员分层 roster(席行持墙钟并集+❶ 唯一;成员按层次披露:调度等待/块层 inode/完成端),层次关系以口径词分层表达。
②**PACE-ROW(新件)**:pacing_idle 段从 ×N 睡眠折叠族独立成行(与"等依赖 sleep"语义异类,合折互相稀释;引擎已铸独立行,显示层不再折入)——标准行骨架 `◦ 自身·帧间空闲(等待下一帧) X.XXXms %`,行2「节拍吻合·上下文(不参与根因排序)」(用户提「帧间正常空闲」的"正常"语义由行2「节拍吻合」承载,typed 可证=mint 条件;主词面保持已落 R2'+图例形稳定);上下文行族不参赛不佩戴;ENG-2 注记存活臂在独立成行后退役为折叠兜底。

## §29.48 STAB-1+ENG-2 收账(2026-07-12;软车道零重写+系统校验附注落地;对抗复核 SHIP-WITH-FIXES+修复轮八项)
**批主体**(§29.47.1 裁定的工程化):S1 词表双源(标准内核事件名闭集+附件原文 token 一次性提取 8MiB/20000 双上界;2779/76278 两 witness 形突变红绿)/S2 最小 diff(strict 车道走既有 block-patch 咽喉,emit_answer_document_patch block 级 op 实证)/S3' 软车道零重写(PSG+lexicon 两条 bus-strict 促升臂删除,三类出厂路径全证无软 kind 重写口;operator pipeline_contract_strict_kinds=唯一 typed escape)+「系统校验附注」独立确定性块(display-attachment 家族,双出厂口,谦逊导语,typed 三面 finding 零内部词,零发现不渲染)+保留区软-only 退场+strict 仲裁看门(FallbackFinalizerOnly 一次性武装,FRCAP 恢复链完整)/ENG-2 第三折叠机(证据 span 对齐,「其中 15.758ms 帧间空闲(等待下一帧)」上树+图例+明细)。**并发接管范本**:新会话检测到旧尾批会话仍活跃→停写只读跟踪→静默后接管,保留其 S1/S2/ENG-2 成果验收+突变,旧 S3 按裁定改造 S3'。
**复核(SHIP-WITH-FIXES,4P2+5P3)→修复轮八项全收**:P2-1 附注提供者错标(系统自声块曾套「来自模型已生成」面板导语——SystemAuthored typed 分类器+两趟分组渲染+系统导语面板);P2-2 仲裁判据②收窄 strict-for-bus(信息面软 kind 不再否决补丁稿,防「第一稿带未修 strict 出厂」)+恢复路径残留 strict 披露入附注(P6 同族);P2-3 生产实证刷新(两次复放干净运行:词表补全后误报归零,附注零发现不渲染被生产实证);P2-4 typelabels 合成 pin;P3-2 注释勘正×3/P3-3 闭集勘正(binder_transaction_reply 与 cpu_capacity 移除=查无标准 tracepoint 实据;8MiB 截断尾防护)/P3-4 PacingIdleSummary 入 schema 指纹/P3-5 附注永不进模型 block 结构 pin/P3-1 双披露注释。architecture.md §流程 7 软车道例外落档。
**教训**:①仲裁类"更好"判据必须 typed 限定严重度域,字面"任何 kind"会让噪声信号驱动稿件选择并放走未修 strict;②系统自声内容的面板溯源导语也是承诺面;③干净运行是负向证据(附注不渲染)的生产实证形。

### §29.47.5 增补(用户视觉反馈 2026-07-12):CAL-1 件⑥
①**徽章 2ch 信封提前兑现**(❶..❺ 小+右弧被裁="被遮盖"实感):T-6 原推迟至 v5 P2a,但 UXG-0 D5 的徽章后随空格已在字节里——preview run 分类并「徽章+空格」入 2ch 信封(彩底药丸 2ch/overflow:visible/墨迹完整),chip scale(.95)+hidden 退役,fence 零字节变化,EVOLUTION RECORD 指 T-6 提前理由;②**pacing 行专属记号+◦ 视觉升级**:候选机械审计(EAW 双语境宽1 禁 ambiguous/双面字体覆盖/13px 可辨/目录零撞)选型,走 tracefence 目录+图例+单格 pin+keyset census 机械管线;审计候选表入收账。

### §29.47.6 witness 追加+排程调整(用户 2026-07-12,14704 案)
20260712-131531.771-14704.md ⊘ 平铺树同段双席新形:同 running 段 54.599ms/55% 发两行(E1 候选车道:行1 带·running+行2 频率披露;E2 裸状态车道:行1 裸名+行2 尾 running)——冷读案 11 家族(candidate/raw-state/context 三车道同段)在 ⊘ 树的实锤,追加入 v5 P1 一段一席(B.2)与 CR-2 P5 同段指纹收敛门 witness。**排程调整**:用户反复撞到双席形→CR-2(P4 徽章-图例闭合/P5 同段收敛/P7 口径标签,含 churn 覆盖声明 F5-1)**提到 CAL-1 之后立即执行**;EVAL-DONGHU 拆两段=引擎语义 oracle(binder/D 态/IO 家族值,CAL-1 后语义已稳)先建,显示形 oracle 待 v5 P1 一段一席落地后建(防 oracle 反复重铸)。

### §29.47.7 立案(用户 witness 2026-07-12,13663 案):SRC-FALLBACK 诚实降级门
witness=20260712-131735.668-13663.md(CAL-1 施工中间态二进制产物,定性以其终态复放为准):trace 主导问题+零根因板观测时,模型回退 --repo 源码车道,答案变成引用仓库源码叙述机制(本案仓=codrax 自身故看似"泄内部",客户场景=其应用仓,但降级形同样是坏答案)+零因果投影。立案 **SRC-FALLBACK**:trace 主导问题(typed:attached_trace 在场∧问句 trace 词面)且 trace 车道零根因观测时——①答案面 fail-loud 诚实披露(「本次 trace 分析未产出根因板」+原因:观测为空/引擎错误/预算),不以源码机制叙述顶位;②探索面软引导强化(既有"source-owner tools only if trace leaves precise source question unresolved"在空板时失守——教学补"空板≠源码问题,空板=披露");③方向全软(§29.42.4:披露+教学,不硬拦)。**④判据收窄(用户告诫 2026-07-12:勿伤 trace+客户源码联合分析正常场景)**:病灶不是"源码车道被使用"——混合分析(trace 观测在场且锚定答案,源码解释被 trace 事实牵连的客户代码机制;current_source_explanation_profile 显式桥接形;perf 触发源码下钻)全部是正常形,零触碰。**触发条件=双 typed 精确信号同时成立**:(a) trace 主导问题(attached_trace 在场∧问句 trace 词面)∧ trace 车道零根因观测(空板 typed 信号);(b) 答案主张面(principal claims)仅由源码引用支撑。仅此形披露"未产出根因板";教学词面同样只教空板形("空板≠源码问题,空板=先披露"),不贬源码车道本身。归批:与 FIN-BIND 同族教学臂随 CR-2;披露臂=orchestrator 确定性(空板 typed 信号)。另:eval 复放用 --repo . 使源码车道可见 codrax 自身与 fixture——EVAL-DONGHU 建 case 时用中性小仓作 repo,消除测试装置噪声。

## §29.49 CAL-1 收账(2026-07-12;行级口径与词面诚实六件+对抗复核 SHIP-WITH-FIXES+冷读关双 witness+修复轮 11 项)
**六件**:①V2-P0(CausalTokenCaliberSideClass 共享臂;count/复合行出序数入 ⌗ 口径旁栏;capacity 独立子道);②IOFAM-SELF(NEW-3 显示折叠席五 facet;席位只许墙钟;分层 roster+复合「分数,非墙钟」;E# merged;三❶修根=被折成员不出行);③DSTATE-REFINE(offCPUDStateVerdict 三载体;wakeupMatchToleranceSec 第三消费修 1µs 尾随 marker 丢账——tieba E15 ×3 15.807→×4 15.317,复核独立逐段复现 µs 级;tri-form 类别词 IO等待候选/D状态候选/D状态·IO候选;裸尾 typed 归并;行2 等待对象 dma_fence 族);④两更名(父节点未确认/优先级反转·可运行等待,lockstep 全);⑤PACE-ROW(∿ U+223F 独立行+节拍吻合;EAW 机械审计选型);⑥徽章 2ch 信封(T-6 提前兑现,真机 0 裁剪)+◦ 1.2em 光学。
**冷读关里程碑**:两报告 30+ 项 trace 逐段复算(自写调度状态机)**数值本体零虚构 µs 级精确**——"数值无一错,错全在给数值穿的字";tieba 判可交付,donghu 确定性面可交付。
**修复轮 11 项**:P2-1 两诚实门补红测(假 pin 双消);P2-2 组级 veto→overlap 连通分量(扩员即回归教训:改成员集必审 veto 语义);P2-3 同段孪生 proof 传播+crown 第三面;P3-1 席值下界图例句;P3-2 fragmented 入尾冗余集;P3-3 EAW 勘正(「圆族全 ambiguous」言过,◉⦿⬤◘ 实宽1;选型三据如实)+canvas 双机械探针记录;P3-5 孤儿 --trace fail-loud CLI 守卫(131735 案根因=--trace 被静默忽略致零附件,SRC-FALLBACK §29.47.7 witness 前提修正:该案非空板形,真空板形立案仍有效待新 witness);F-7 计数席「=合计」谎言修(81.616=35% 离链窗帽合法值,typed clamp 判据+「计数当量(超上限截断)」词面);F-1 引擎修根(ThreadDuration 段真值载体;成员行「合计16.064ms(5段)」族+a–b=raw 段真 min-max——修理方向误导消:11×2-4ms 高频短等待非 16ms 长停顿)。
**F-2 附注覆盖诊断(CR-2 输入)**:133933 附注仅接住 2 词面项,prose 榜错嫁全漏。四缺口:①P3a 单调臂不解析「N. #N…」md 列表形;②榜成员身份无门(prose 席主体无 typed 席位不报);③P1 三元组缺主体-置信绑定(0.91 转贴);④prose 自造聚合(备注字段和 21.153 以席值形态出场)无披露义务。
**移交**:F-1 残口(E9 critical_blocking 孪生行行粒度范围)→CR-2 P5;F-7 伴生(⌗ 值入关键指标表列语义)→D14 族;冷读 F-3/F-4/F-5/F-6/F-8~F-14 原文存档 customlogs/codrax_output_archive_20260711/coldread_cal1_report.md 供 CR-2 分诊;IOFAM 席值 union 需引擎跨 lane ts inventory(立案);delay= 交叉校验未做;PACE-ROW % 槽(自身行族无 % 列,如需裁定);E15 账变披露(客户回访对数口径:5µs 容差修正,复核逐段复现)。

## §29.50 CR-2 收账(2026-07-12;承诺面闭合四组+对抗复核 SHIP-WITH-FIXES+冷读关+修复轮十件)
**四组**:①P4 持席行(Rank∈1..5)编译层折叠豁免(F-6 ❷洞关账:JankManager 16.687 独立成行,折叠计数诚实减除;TopN 常量与显示徽章恒等 pin);②P5 显示层同段收敛先行版(等值臂 root_evidence 裸行并入+[E1+E2]+「同段镜像已并入」;家族臂 E9 佩镜像记号+×N 明细段真值改述;⊂ 泛化臂无 typed inventory 不可证=诚实缓办 v5 P1);③P7 口径守真(ActualWindowStartTs/EndTs typed 字段+四态 scope enum——11 假⚠ 全改 episode 词零假⚠,真跨窗三重 pin 保护;F-5 (单次成员);F5-1 覆盖声明「实际状态段跨度(活动切片,非全窗事件覆盖)」;F-3 pacing 摘除句 typed 匹配;F-4 榜摘要 representative_window);④软检测四臂(P3a md 列表形/P3b 榜成员身份/P1 主体-置信/自造聚合披露)——生产复放附注实抓 2+1 自行加和+1 线程错绑,tieba 零发现零渲染。
**冷读关**:typed 面 30+ 数字 µs 级全中零虚造(连续第三轮);prose 面两 P1(r1 幻数 21.153 P0 建议/tieba 三元组假+恒等式内爆=饥饿归属翻转)=CR-3 P6 墙钟守恒门客户面活实例,优先级实证升高;**附注自身造假实锤**(分解式错配/幻数坐实)——教训:**披露面也是承诺面,系统自声版面的每个字同样要 typed 可证**。
**修复轮十件**:R-P2-1 置信绑定臂改纯附注(零重写合规——复核抓到"头注自述附注面、实际走改写轮"=验收话术掩盖行为反转教训活标本);R-P2-2 actual_window 升格 hard_consumer+census 反向臂(首跑再抓 background_rank/total 两低报键,三键齐升);C-1 附注自证义务(分解对逐侧核载体行实值/名-tid 单行提取(「嵌合」定案=真实引擎行,真病根=% token 单位盲匹配 ms 载体→百分比退出线程绑定臂;伴生修 %-复算臂升序池截断假披露)/不可证降级零分解式/成分含 pacing·跨主体附注);C-2 等值双席(tieba 对扩 value-mirror 臂+图例;donghu E13/E23 如实 fail-open=cum 不同账目绝不折 W-A 先例,pin+理由,终局 v5 P1;E8 (+1) 核毕诚实);R-P3-1 包含容差独立命名常量;R-P3-2 图例限定最小诚实形;R-P3-3 家族臂窗否决;P4×3 便宜项;D1 E39 悬空修根(优化点表 E# 仅已走位引用,未走位内联定位)。
**遗留**:B1/B2 prose P1→CR-3(P6 守恒门+FIN-BIND);块级绑定粒度+约-值载体归属=CR-3 附注卫生候选;A3 部分重叠席=v5 P1;notes-poor 覆盖头(CR-3/IC-A);F-9~F-14 分诊表照 CR-2 终报。

### §29.50.1 witness 追加(用户 2026-07-12,56643 案):部分重叠双席 runnable 变体
CookieMonsterCl-59843 双席:❶ 调度压力候选 runnable 全额 25.847 与 ❸ 优先级反转候选 19.933(链覆盖最大片段,19.933⊆25.847 物理重叠)。定性:**双假设分席=设计**(修理手册不同:供给 vs 反转;❸ 行2 从属披露已在);**跨席不可相加记号缺=已立案**(⊂ 部分重叠泛化臂,v5 P1 引擎一段一席射程;d_state↔io_burst 双席同族裁定池)。本 witness 追加 v5 P1 工单;过渡候选(v5 P1 前若需):高位席行2 加「与#N 席共享物理时间(不可相加)」反向指针——typed 判定可精确(同线程+同状态族+值包含+从属披露在场),随 CR-4 顺手评估。

### §29.50.2 witness 追加(用户 2026-07-12,56643 案):从属披露「最大片段」模板病
NetworkService-60595 两条链车道 runnable 行(7.843/6.754,两次独立发生=合法分行,和 14.597=引擎聚合席值)的从属披露均铸「链上仅覆盖其中最大片段 X」——「最大」按行铸,次行宣称为假且同页互斥。修(CR-3 修复轮追加):模板感知同线程链覆盖片段集合,唯真最大者保「最大片段」,余行改「另一链上片段」;单链行形字节不变。与 §29.50.1(CookieMonster 双假设子集重叠形=v5 P1 射程)对照:本形=同类不相交片段,分行合法、可相加,病仅在词面。

### §29.50.3 指令(用户 2026-07-12):同线程多行审计扩全状态族
「同线程多行」不仅针对 runnable——sleep/D/iowait/running/IO facet/语义 span/锁/binder 等全部状态与形态同样审计+修复。审计分类学(两 witness 定标):A=跨语义类子集重叠(CookieMonster 形:双假设分席合法,缺跨席不可相加指针);B=同类不相交片段(NetworkService 形:分行合法,词面模板病);C=同段不同账目(E13/E23 W-A 形:cum 不同绝不折,双行存续);D=纯重复(应折未折)。产出=每状态族×每行铸车道的关系矩阵+逐形处置(折/留+指针/留+改词/立案),修复批=SMR-1(排 CR-3 推送后,与 CR-4 实体绑定并批或紧邻)。

### §29.50.4 裁定(用户 2026-07-12):同线程·同状态·同影响根因片段合计参赛
用户裁定原文要义:同线程片段若同状态、同影响根因,就应合计参赛——真根因才能被揪出(拆分稀释排序)。对齐现状:引擎已按 (线程,类型) sum_disjoint 聚合参赛(§29.19 ORD 先例;NetworkService 14.597 聚合席/binder 按对端 ×7 合计均为现行);本裁定精确化聚合键=**(线程,状态族,影响根因身份)**——根因身份用 typed 判(对端/caller/等待对象/反转关系对):①同 cause 跨 type token 片段(如 D 与 iowait 同 dma_fence)应并席参赛,token 细分不得稀释真根因;②cause 身份不可证不并(absence never guesses,fail-open 保独立);③链车道逐次发生行保留为明细,带「合计参赛见席」指针(SMR-1 词面统一设计的一部分)。落批:SMR-1 处置矩阵以此为 B 型合并判据;引擎键演化随 v5 P1;两把尺/W-A(不同账目不折)红线不变。

## §29.51 CR-3 收账(2026-07-12;数账守恒与披露卫生七件+对抗复核 SHIP-WITH-FIXES+冷读八核点全 PASS+修复轮 14 件)
**七件**:①P6 墙钟守恒门三臂(全附注零硬拦;Σ 臂逐维 MAX 对 typed 全窗账/单值超窗/单维度超发布总量;ε=0.05 命名常量+绝对地板;witness=tieba 三元组/案11/F-8);②P10 blocked_reason 消费义务(「窗内存在 N 条…(caller=X,未核销)」两显示面;冷读案 7 机械关账);③P11 tgid 归属(单点收敛 stamp;明细行2「进程: tgid=G comm=P」+榜摘要;comm 不可解诚实);④P12 重复块门(单咽喉 K=8;观测块恰 1);⑤附注卫生三件(句级绑定+qualifier/约-载体归属/同线程对优先);⑥F-10 限压见证门(carry-in 不算见证;词面分叉「受热限压」vs「运行于X(限压原因未见证)」;donghu 1.53 见证 trace 直核为真);⑦FIN-BIND +3(IO 角色词/口径分离/空板禁静默源码回退)。5 note key R2' 三面+census 8 字段全同步。
**冷读里程碑**:八核点全 PASS,显示/证据面**连续第四轮零虚构**并判达客户交付水位(40+ 数值 µs 级直核);残余风险 #1 定名=**prose 实体绑定造假**(真值缝错主体:等待对象错栽×3(prose 与自家树打架)/名-tid 嵌合+持锁等锁角色倒置/CPU0-1090MHz 三成分无据)——附注防线只核标量不核实体绑定=CR-4 靶心。
**修复轮 14 件**:P10 计数修根(截断前全量累加 map,INODE 先例;tieba 19 条恰等 trace);P6 主线程主体校验(tid==tgid 可证才绑)+**前置律修根**(验收自抓:572.289 CPU 队列总量被尾随线程名误绑——主体只认值前);P12 恰8/9 边界+「同上略」自去重;巧合对三级抑制(验证对优先/纯池对降级零公式/跨主体 qualifier 只配验证对);Arm B 独立命名地板;F-10 khz<fmax 收紧(release-only 不穿热词);anaphora 双向 pin;P11 引擎孤儿 tgid pin;**DUP-CHIP(56643 witness)**:覆盖披露模板感知同线程片段集合——唯真最大保「最大片段」,余行「另一片段」,树表两面同源,同页互斥灭(tieba 16.812/14.597 复放实证);F-CR3-9 单成员 ⌗ 计数行表标签补「计数当量」。
**教训**:①精确计数词面禁在截断库存上二次聚合(INODE 教训第二次生效);②「最大/唯一」类最高级词面必须组感知铸造;③绑定方向律=主体只认值前(尾随名=噪声);④修复批的验收复放是最好的误报猎场(两轮自抓自修)。
**遗留**:CR-4 靶心=prose 实体绑定四臂(等待对象/持锁角色/名-tid 对/CPU·频率见证)+F-CR3-4/5/7/10/11 分诊+双席加法话术(d_state↔io_burst 裁定池);SMR 全状态族审计(§29.50.3 在跑)+合计参赛裁定(§29.50.4)→SMR-1;P11 三车道残围;target_window_states 发布率。

## §29.52 SMR 全状态族审计收账(2026-07-12;53-agent workflow,45 形确认/27 席位对判型 A5·B5·C4·D10)
**终报**=customlogs/codrax_output_archive_20260711/smr_audit_report.md(关系矩阵/工单草案/防重复清单/WO-A1 统一规格/优先级)。**头条伤害**:同线程三 rank 席跨席可加 33.1 vs 实 18.1(1.83×,9 报告存活);最坏五席 292.3 vs 105.8(2.76×)。**四条系统性结论**:①D 型病根收敛两个铸造口(双车道各自铸行+twin/mirror 臂指纹过严整体逃逸;remainder 池无归属检)——根修=去重先于合并/一段一席(v5 P1),显示层只做过渡 tag,**禁按词面/家族键造第二套合并**(S3-TPF 实证:家族键合并会把双发布 SUM 成 ×6 更坏);②A 型缺统一「不可相加」指针臂(§29.50.1 推广,typed 判定/词面/挂点三单源,v5 P1 退役);③C 型合规缺理由句,模板经复核校准三禁令(禁「同段」字面/禁方向暗示/禁量化重叠主张);④B 型病纯词面,统一入 §29.50.4③ 模板(合计席参赛+逐次行降明细带席指针)。
**SMR-1 批工单**(显示/词面层,红线=禁第二机制/精确信号才硬折/W-A 不折/零静默消失/两把尺):WO-A1 跨席指针统一臂/WO-D1 成员归属折叠(◌ 带值行首选无值披露臂)/WO-D2 branch 内同源聚合去重(§29.50.4 裁定要求方向)/WO-D3 critical 孪生双发(短期互指 tag,根修 v5 P1,禁家族键合并)/WO-D4 跨 path 双发(记号臂优先,折席必双列账目)/WO-C1 账目关系句/WO-B1 发生序短注+合计参赛指针(禁抹「父节点未确认」/改词保双席)/WO-G1 双◌ 合席+TraceGapKind 铺链车道(禁并一行双理由)/WO-G2 零值 marker 收敛(禁裸删席)/WO-N1 NEW-3 扩 lane(资格门按墙钟区间重叠禁行号包络)/WO-P1 正文榜类型词软引导/WO-T1 测试面补。
**独立立案**:CASE-1 G1 recon 第二裁定对({d_state_or_io_wait,io_wait}²,witness 三报告实锤,走 universe pin 权威通道+三工程缺口随案)/CASE-2 absorbed_into 窗口编码回归 witness/CASE-3 双 rank 席裁定臂(链投影 vs 全窗互斥账,过渡挂 WO-C1 句;伴生 ❹ 混合形合并词)/CASE-4 v5 P1 工单追加(自身席终局/跨 path 去重/⊂ 变体/引擎单席/过渡臂退役条件)/CASE-5 D14 族补账。**防重复**:6 配对已被既有机制覆盖只补测试面;C 型拒折行为正确勿动。

### §29.50.5 裁定精化(用户开放推翻,主会话建议采纳 2026-07-12):证明分区式合计参赛
§29.50.4 方向维持(拆分稀释真根因),精化三护栏:①**逐片段证明门**——每片段 typed 证明同根因(unanimous caller/同对端/同反转对)才并根因席;未证片段留通用 (线程,类型) 席作诚实余数(「D-state(原因未证)」形),绝不灌根因席(依据=P9 13× 教训:无证归因即假叙事机制);②**假设永不并**——根因身份=具体等待对象/对端,语义假设类(调度压力 vs 反转)保持分席+A 型指针(CookieMonster 形);③**实现位置=v5 P1 引擎铸造层**,显示层永不做第二套合并(SMR ×6 反例);过渡=SMR-1 B 型模板盖同 token 形,跨 token 合并等引擎版。C 型(不同账目)红线不动。否决替代案:纯逐片段参赛(稀释病+违 §29.19)/cause 只做汇总视角(榜面仍被 token 压位)。v5 P1 工单(CASE-4)以本节为聚合键规格。

### §29.52.1 裁定(用户 2026-07-12):区间符与 ×N 家族词面
witness=「· ×3(4.426–6.768ms) ·」:①「–」在算术密集报告中误读减号;②「×」读作乘法,且 ×N 语义超载实锤(实例合并形=N次 vs 取最大形=N线程,同记号两义)。两步处置:**WF-range(随 SMR-1 即落)**=值区间 en-dash「–」→ASCII「~」(~ 已是 prose 门 typed 区间语义;禁借「..」=已教时间窗记号);**WF-xn(独立词面窗口,已定向待排)**=×N 家族改词拆超载:实例形 ×N(…)→「N次(a~b)」/同值形→「N次同值」/跨线程取最大形→「N线程取最大(单项a~b)」——图例五式+wrap-atom 表+pin/golden 全 lockstep,排 SMR-1 推送后的词面批(可与 CR-4 同窗)。

### §29.52.2 witness 追加(用户 2026-07-12,76684 案):行1 形态词丢失回退
io_wait 族行行1 裸名(`ThreadPoolForeg-60555 █ 13.418ms`),iowait 掉行2——违 PTV4 行1 三要素;对照 96728 同族行(行1 `· iowait` 在场)=近两批回退(嫌疑 DSTATE fork/对端限定词走位)。修入 SMR-1(同域);**机械化候选**:行1 形态词在场 census(每条非折叠 cause/state 行的行1 必含 typed form 表词)——与既有三要素 per-form pin 互补的通用 tripwire,排 UXG census 族。

## §29.53 SMR-1 收账(2026-07-12→13;同线程多行处置十二 WO+四追加+对抗复核 SHIP-WITH-FIXES+冷读关+修复轮十五件)
**十二 WO**(§29.52 工单):A1 不可相加指针统一臂(三载体,§29.50.1 推广,v5 P1 退役条件入注)/D1 成员归属折叠(◌ 无值披露臂;42.131 幽灵 headline 灭)/D2·D4 branch 同源聚合(全等指纹硬折+eff 双列)/D3 critical 孪生(V4 lane 放宽+互指 tag,禁家族键合并)/C1 账目关系句(三禁令)/B1 发生段短注+合计参赛指针/G1 双◌+TraceGapKind 铺链/G2 零值 marker 并入/N1 墙钟区间连通/P1 正文榜词软引导/T1 测试面。**四追加**:WF-range(值区间 –→~ 全面,22+ pin lockstep;时间窗/真减号不动)/76684 行1 形态词回退修根(定位=unknown-thread 哨兵批;state 词入 keep-suffix)/「分数」→「综合评分」/「口径」→「等口径」。复放迭代链 12 轮自抓自修 7 件。
**复核 P1-1(教训级)**:N1 换轨墙钟后产线 IO 观测发射层不带 ts→整族折叠静默失活、S9 一席回退、复合分数裸 ms 回潮——**fixture 合成 ts 使测试面等价产线不等价**。修复轮:两发射点补 StartTs/EndTs(typed 现成)+产线负向 pin+S9 复验(29382:block_io 回 roster「综合评分,非墙钟」+「等口径」live)。**教训:换轨类改动 fixture 必须取产线真实发射形,合成字段=掩盖失活**。
**冷读关**:证据面 µs 级全中、已铸指针零指错、前批面零回归;未收敛四对漏铸→修复轮扩臂全落(E13↔E7/E8 和恒等互指/E9↔E14 显示臂待 CASE-1(b) 库存/E21·E26 镜像注+谦逊注/板级警示「席位合计 120.528 超窗 114.940:物理时间可重叠,不可直接相加」live);C1 误对修(同覆盖检);**新 P0=「全程 s_sleep/未发生 running」全称断言 vs trace running 157.2ms(67%)**——燃料修:四态账常态发布(目标锚定+有界窗恒发布);检测臂(全称状态断言 grounding)=CR-4;132.041 排查=无车道逃逸,模型跨口径自加(96.081 cpu·ms+35.960 per-CPU)=CR-4。
**修复轮十五件**其余:P2-2 折叠池跨口径穿透(typed 双 lane tag+wire-fold 池谦逊注 fallback)/P2-3 必要性否决/P3 群(type-lane 宽度/D3 负向/四守卫独立 pin/B1 窗键/G1 口径注)。
**交付判定(冷读)**:tieba 可交付(板级警示已补);donghu 退回(证据面准,prose 全称断言反转=CR-4 靶)。**遗留→CR-4**:全称状态断言门/R3 幽灵 PI 席(21.153/0.91)/「同进程组」定语/132.041 跨口径缝合;E9↔E14 产线落地+wire-fold Σ typed 化=CASE-1/3;B1 时间戳 en-dash=WF-xn。

### §29.53.1 SMR-1 四态账 resolved-target correctness 热修冻结（2026-07-13）

远端`4f57e9804`把四态账扩到所有target-anchored bounded non-bundle run后，对抗复核发现两条同一常态发布臂的确定性漏账。第一，name-only selector会由`targetWindowTimeline→ThreadTimeline→resolveThreadSelection`解析成唯一positive TID并写入`TimelineResult.Thread`，但caller随后仍把原始`ThreadRef{PID:q.PID,Comm:q.Thread}`传给`buildTargetWindowStateAccount`；当`q.PID=0`时，`SleepIOWaitMs`的blocked_reason matcher无法按`WakeePID`命中，`DeterministicRunningMs`也退化成comm匹配并在线程改名后漏掉同TID语义span。第二，常态臂以`q.TimeStart>0`判断窗口，错误排除显式`TimeStartSet=true`的`[0,x]`合法窗口。

- **唯一修复范围**：timeline是该臂已经执行且完成identity resolution的单点权威；builder必须消费同一次`tl.Thread`，禁止从原始selector重建第二身份。窗口资格复用既有`queryBoundedTimeStart/queryBoundedTimeEnd` typed helper并仍要求`End>Start`，显式零起点合法，未设置的隐式`0`不能借数值同形进入。bundle臂、timeline扫描、五状态partition、G12 single-attribution及SMR折叠/报告层均不改。
- **验收矩阵**：name-only唯一命中且线程中途rename时，常态账Thread保持positive TID，S+iowait marker仍进入`SleepIOWaitMs` refinement，semantic-span∩running仍按TID计入`DeterministicRunningMs`；同名多TID继续fail-close，PID selector与bundle arm字节/数值不变。`[0,x]`在start/end flags显式设置时发布；start值为0且缺start flag、end值为0且缺end flag、`End<=Start`均不发布，非零legacy值继续服从既有typed helper兼容语义。结构pin锁定常态臂builder的target实参只来自`tl.Thread`且窗口只读两typed helper；focused/shuffle/race、四相关包、全仓test/vet及独立复审RELEASE后单独提交推送。
- **不偷报**：本热修只关闭SMR-1新常态四态账的identity/window admission漏口；CR-4全称状态断言、P1-a2.2至a2.4、ROW-SORT-BND、P1-b及账本其它开放项不因本批结案。
- **交付收账（`d97b90c47`，已推送`main`）**：`Run`在`normalizeQuery`之前冻结调用方start/end boundedness，generic arm只在双端点确实由调用方提供且`End>Start`时铸账；同一次`targetWindowTimeline`产出的`tl.Thread`成为refinement唯一身份，不再把原始PID=0/name selector带回blocked-reason或semantic-span matcher。修前红fixture同时覆盖name-only→TID 61、`oldname→newname`改名、S+iowait≈10ms和`VerifyClass∩running`≈6ms；admission表覆盖显式`[0,x]`、缺start、缺end、相等、反向及legacy非零窗口。AST pin锁定pre-normalize单赋值、双boundedness+`End>Start`和builder六实参`idx,tl,ok,tl.Thread,window,res.WindowStats`，禁止第二身份/第二准入实现。focused shuffle`×20`、focused race、`tracequery/tracediag/tool/types`四包、`go test ./... -count=1`、`go vet ./...`、`git diff --check`全绿；标准Donghu本地golden相关真实trace测试随全仓通过，两路独立identity/admission复审均为**RELEASE**。本项关闭，后续恢复P1-a2.2。

### §29.53.2 裁定(用户 2026-07-13;撞号注:commit message 引用 §29.53.1 为撞号前编号,远端同日已占——以本节为准):系统禁以关键词匹配定性模型自然语言——附注全面改「事实并陈式」
用户裁定原文要义:系统不能通过关键词匹配模型自然语言后错误影响模型输出;模型信息更丰富、判断更准,系统误判不得影响模型。落地(CR-4 修复轮方向改造):①模型输出零影响已成立(STAB-1 软车道零修复轮+pin),本裁定进一步禁**系统对模型文本的定性**——附注删除一切「正文声称/表述为」指控式措辞与 prose 语义解析(角色/归因/全称断言声称提取全退役;十个误报向量的守卫打地鼠路线作废);②改为**在场触发+typed 事实陈列**:prose 出现线程/数值 token(纯在场检测)→附注列系统 typed 事实(「系统事实对照:…」),真事实多列无害=误报构造性不可能;③唯一保留判定=纯算术(「文中等式 A+B=C:实际和为 D」,数学非 NL 理解);④导语改定位「系统不判定正文正误,供交叉核验」。**原则沉淀:系统供证据,模型下判断,读者做裁决——系统对 NL 的唯一合法操作是 token 在场检测与算术,语义理解永远属于模型**。

## §29.54 CR-4+WF-xn 收账(2026-07-13;prose 实体事实并陈+词面家族改词;对抗复核 SHIP-WITH-FIXES 10 误报向量+冷读关+两轮修复轮,终态=§29.53.2 裁定全面落地;裁定节因撞号改号,commit message 引用为撞号前编号)
**演进史(本批内三形态)**:指控式六臂(等待对象/锁角色/CPU 频率/全称状态/跨口径/席位)→复核实测 10 误报向量(否定纠错句/被动句/同进程子串/祈使/假设句全反咬——正文写得越对咬得越狠)+冷读附注误报(1.193 目标自有值被打错绑)→**用户裁定 §29.53.1:系统禁以关键词定性模型自然语言→事实并陈式**:prose_fact_juxtaposition.go 取代六臂——在场触发(线程名/CPU+频率 token,零语义理解)+typed 事实陈列(席位/等待对象/锁角色/四态/tgid/频点),真事实多列无害=误报构造性不可能;唯一判定=纯算术假等式臂(「文中等式 15.758+5.395=20.816:实际和为 21.153」);PSG 绑定族措辞全面事实化(删「但正文将其表述为」尾句);导语「系统不判定正文正误」;banned 措辞 grep=0 入 pin;零输出影响 airtight pin;十误报向量句全 pin 零判定。四 witness(dma_fence 错栽/锁角色倒置/CPU0-1090/全程 s_sleep)全由事实行并陈覆盖。
**保留基建修复**(定位真):板族 render-shape 融合单元(两轮幽灵席隐身病根)/置信只认 typed record/loose-名邻近抑制/自加和句内点名对优先(2.162→0.808+1.354)/席位重复发布收敛。**引擎件**:wakeup target_cpu 退化普查(全 0 typed 判+地板 100+多核门)→window_stats caveat+事实行消费,witness 155/1697 live。**WF-xn**:五式改词(N次/N线程取最大/n=N en 形)+图例+tracefence 表⑥+数字左融合+时间区间 ~ 统一,引擎 ×N 零残留;golden 产线重取抓出真 drift(窗X–Ys chip 图例分叉)同修。两 eval PASS;双复放事实行 live。
**原则沉淀(§29.53.1)**:系统对 NL 的唯一合法操作=token 在场检测+算术;语义理解永远属于模型;系统供证据、模型下判断、读者做裁决。**遗留**:P6 守恒臂措辞事实化(一致性对齐候选,下词面窗);16.67ms 帧常数入自加和事实行(真话但噪,观察);席位事实行多窗双板窗标(候选);F-CR3-5/per-CPU idle/第N位 前批遗留照旧。

## §29.55 EVAL-DONGHU-A 收账(2026-07-13;金样本 eval 六 case 全 PASS,§29.42.5 关账)
**交付**:donghu.ftrace 入仓(sha 与原件逐字节一致)+six case(h1 binder 真/假归因/h2 dma_fence 纯 D tri-form/h3 IOFAM 一席/h4 供给+热见证(157.248 硬正+132.041 硬负)/h5 SMR 多行/h6 通道混布),oracle 双层(硬=trace 复算值+禁词 grep,注释带 trace 行号依据;软=verdict 判词),问句标准形零仓依赖;两次 oracle 中途修正均为对齐已实现语义非降杠(self/cause-row 车道形+io_wait facet 本 trace 合法缺席 census)。两天全部修复语义自此有常驻回归看守。
**新观察四件(立案/裁定候选)**:①**§29.53.2 首个生产战果**:模型两跑均把等待对象错栽 kthread_worker_fn(行上打印≠行所属混淆),事实并陈 appendix 两跑均正确给出 typed=dma_fence_default_w——读者可自行对照;错栽形=事实并陈族扩臂素材(「行上打印 vs 行所属」教学候选);②binder 同步等待普查疑漏一(txn 12145963 0.924ms 真同步未入 critical_blocking 车道,P9 头注自述窗内 ≈3.5ms)——车道口径差待裁定(立案 BINDER-CENSUS);③计数当量 ms 后缀两形不一致(树面 81.616ms vs 旁栏 81.616(非墙钟))——词面一裁(下词面窗);④「完成端到端」成员铸造随查询批次变异=条件面记录,恒铸与否待裁定。

### §29.55.1 需求(用户 2026-07-13):CLI 报告输出路径双 flag(OUT-1)
CLI 模式增两 flag 分别指定 markdown 报告与 html 报告输出路径(含文件名);默认 .codrax/output 落盘能力不丢;任一 flag 指定即按用户路径**额外**输出一份(两 flag 独立,可单给可双给)。

**追记(交付收账,2026-07-13)**:选型 `--report-md <path>`/`--report-html <path>`(--out 已被 tracediag 占用;compat 单横线兼容)。架构=单一内容源:`WriteResult` 唯一咽喉,同 BuildBody 产物写默认 dump+显式副本,html 同管线;`output_dump_enabled: false` 时 cmd 层占位 dir arm orchestrator 钩子+`SuppressDefaultDir` 抑制占位落盘+`Result{}` 消声(preview/paths 仍视为禁用),engine 域零改动,L1 直读绿。七 pin 验收(双 flag 独立+同给/默认 dump 不变/disabled+flag 仍写/父目录创建/覆写/md 字节==默认 dump/坏路径 WARN 不阻答案)+真机双 run。对抗复核 SHIP-WITH-FIXES:F1(P2)tracediag 车道静默吞新 flag(违背该车道自身 fail-loud 裁定)→修=`explicitReportFlagConflicts()` 单一值源并入 `traceDiagConflictingFlags`,红→绿 pin+真机与 --plan-out 同形文案;F2 --write-audit 车道同形修;F3 cmd 接线层零覆盖→`armExplicitReportOutputs` 窄接口拆分+四 pin+突变 3/3 击杀;F4 记录(help 未说 CWD 锚/无 ~ 展开与 --plan-out 一致/同路径双 flag 大小写不敏感盘穿门=病态输入)。文件:cmd/{root,report_out,tracediag}.go+internal/outputdump/{output_dump,explicit_report}.go+三测试文件。

### §29.55.2 立案(子 agent 上报,主会话采纳 2026-07-13):DET-1 选举决定性(并入 v5 P1)
block_io_by_inode/io_burst_episode 族同输入两跑翻面(storage_max 归属/caliber_side 成员选举 2.405↔2.694/top-8 在场性/证据主体)——根因=map 迭代序 tie-break(computeIOBurstEpisodes+inode 汇总选举),main HEAD 既有病(非 P1 引入)。**噪音从源头消除红线直踩**;并入 v5 P1 作 DET-1:帽/选举前确定性次序(typed 恒定平局键)+确定性 pin(donghu fixture 两趟 byte-identical)+突变红绿。**联动关账 §29.55④**:确定性修后金样本 h3「完成端到端」条件软 oracle 回升硬 oracle(恒铸与否按确定性真值定,悬案消解)。**勘注(PIN-1 B4 账实差勘正,2026-07-13)**:「回升硬 oracle」为过铸——实际落地形=**oracle 保软+条件恒等**(h3 case EVOLUTION RECORD 原文:"the oracle stays soft, but any run that carries the io-facet seat must now show the 完成端到端 member (恒在 given the seat)");词的铸造仍**结构性依赖 dispatch**(2026-07-13 派发扫描:rank/bundle 族视图下在场,window_stats-only 下缺席),悬案消解仅指 DET-1 后同派发下选举确定(2.405 形不可达)。判定逻辑未改,以 case 文件自述为准。

### §29.55.3 立案(用户 2026-07-13,报告 20260712-232349.054-94317):链上折叠行 ◦ 记号语义超载+对齐观感(并入 v5 P2a「折叠行类型词面」rider)
用户问:「◦ 其余 X 项(链上折叠)」的 ◦ 表述什么、是否链上、若是「影响未分类」如何在树上优雅表现;且 ◦ 视觉「缩紧半格」。核清三事实:①行是链上(OnChainOverflowFold=链上通道超逐行上限成员折叠,PTS #68 零静默丢弃;边词刻意省略=行名自报车道+roster 头名保护 pin);②◦ 非折叠专属记号——GlyphNeutral 默认臂(无主导态,成员状态可异质),同报内 ◦ 另佩 binder等待 borrow 义,一形三义+折叠行第四佩戴者=「影响未分类」误读风险实锤;③「缩紧半格」双成因=U+25E6 小环光学半格(2ch 信封内显空)+折叠行前缀 `├─ ` 比 `├─链上─ ` 短 ~5 格(记号列左移)。**处置(P2a rider 扩容)**:a) 折叠行铸专属记号从 ◦ 分裂(第三次适用 T4/∿ 分裂先例;候选 ⋯ U+22EF「还有更多」义,宽度纪律=EAW 确定+无 VS16+入 single-cell pin 环);b) 图例 `其余N项(链上折叠)` 条目挂新记号,◦ 图例适用面收窄;c) 对齐候选=边词省略时横线延长 `├──…─` 推记号至同列(视觉横线延伸非假边词,落地按 roster 预算 pin 核);成员异质性不上记号(恒4记号场),留行2+明细。
**处置更新(用户裁定形,2026-07-13,工件 20260713-022117 复问)**:用户确认并列关系后提议 **`链上─ ◌ 其余 2 项(折叠)`** 形——采纳,取代 a)/c):边词管车道(`链上─` 与兄弟行同列,并列自明;车道词对折叠成员为真,原省略理由针对唤醒关系边不适用)、行名管折叠(「链上」从行名去重,净宽 +1 格,PTS 计数+头名 pin 复核)、**记号位留给形态族**(实证:94317 折叠行 ◦=无主导形态,022117 折叠行 ◌=盲区族「窗内无调度数据」——记号继承合并节点影响形态,携真信息;专属 ⋯ 记号反而丢信息,a) 撤销)。lockstep 面:行名铸造双点+边词省略臂+图例条目(`其余N项(折叠)`)+tracefence 闭集表+fence 分类器折叠头+census/宽度 pin。仍并 v5 P2a rider。

### §29.55.4 立案(冷读关 2026-07-13,donghu 复放):prose 面双 P1——blocked_reason 证据未入模型面 + 互斥状态关系编造
冷读判 donghu typed/确定性面达交付水位(逐 µs 抽验全对/E# 端点精确/四态守恒/假⚠零/binder 核心答案四次复放字字稳定),但 prose 错在题眼维度,既有病非 v5 P1 回归:
- **F1(P1,立案 EVID-BR)等待对象四跑四答案**:题问「内核记录的等待对象」,prose 四次复放给出四套答案(dma_fence+kthread/unknown-thread/kthread pid=357/binder+mm_filemap),最新一份全错;真值 12 条 blocked_reason 全部 dma_fence_default_w;事实并陈附注每跑均正确给出 typed 值(§29.53.2 战果),但模型自己看不见——根因=blocked_reason typed 观测未进模型证据面+自身 D 行(stanza)不佩「等待对象」词面(链上席行已佩,CASE-1)。修向:blocked_reason 观测入 explore/extract 证据面+自身行词面补佩。
- **F2(P1,立案 FACT-REL)互斥状态包含关系编造**:prose 称「running 7.081 已包含在 D-state 36.757 内」等互斥调度状态间的包含关系,并自造数 162.852/45.315 断言可加合计 213.771,与 typed 四态账直接矛盾;附注算术臂抓到 213.771 假等式(工作中)。修向(§29.53.2 纪律内):不做 NL 关系判定,扩事实并陈臂——prose 同段出现同线程多状态值 token 时,附注列 typed 四态账行+「四态互斥分区(Σ=窗长)」typed 事实,读者自行对照。
- **P2 观察**:实体绑定漂移(keva「代为执行」6 笔 IO 实为主线程自发/sector 张冠李戴/binder 对端错挂)=CR-4 残余印证;prose 普查绝对化(39 vs 真值 50/「6 次 IO」vs 85 笔/8 区间 vs 65 段)=截断库存二次聚合教训 prose 面再现;附注一 FP(解释从句再提及值误报,事实并陈式下无害=真事实多列)。
- **冷读第二轮追加 witness(2026-07-13)**:FACT-REL 族新最重形=**claim-of-absence**(tieba prose 收尾「未证实的 IO 等待原因不存在/三类覆盖全部 D-sleep 段」直接否定同报 ❸ 原因未证 10.433 席,恰是所问维度;log thinking 示成因=手加五大段凑 17.613)——事实并陈臂扩向:prose 全称覆盖/不存在 token 在场时列 typed 席位清单(含未证席)供对照,仍零 NL 判定;EVID-BR 族追加=唤醒者清单系统性倒置(D 入口 switch 的 next_comm 当唤醒者、入口时刻当唤醒时刻,11/11 全错,真唤醒者 gpu-token-id4-2931)——wakeup 语义 typed 观测入证据面同批修。

### §29.55.5 v5 P1 修复轮二收账(件A 全量账铸席+件B 证明供体 dispatch 无关化,2026-07-13;delta 聚焦复核+冷读补验在飞,终局验收句随 §29.56)
**件A(ENG-1 补完,d/io 车道)**:`computeOffCPUStats` 回传 pre-cap `dstateCensus/iowaitCensus`(runnableCensus 同形);`rootCauseDIOStateFamilyItems` 席位改**全量账铸值**(成员选取复用 `topThreadDurations` 不设帽=单一值源;无帽外时与旧铸字节恒等,census nil 的 legacy 直构 fixture 按帽基 verbatim fail-open);`topThreadDurations` 平局链 typed 化(时长→行→pid→cpu,原 map 序=DET 同类洞顺手修根);`threadDurationCapOverflow` 帽外披露=被逐组各加一次(禁差减);wire 面 `top_d_state_overflow/top_io_wait_overflow`+per-lane caveat。tieba witness:席 4.739→**7.386**(fscache_page_wait_o)+hmfs_get_dnode 0.171 得席+hmfs_read 0.145+余数 10.433,Σ=**18.135** 全量基恒等(冷读复算逐 µs 同);帽披露 `groups=11/3.185ms`+`groups=15/17.306ms` 直验在场;donghu CompThread 四组全在榜→席位形字节保持。突变红实录恰在 4.739 vs 7.386。**病根钉子**:分区把 cause 词写上席面后,帽内下界值(4.739)成承诺面 falsity——精确值+cause 词的席只能铸在全量账上(INODE/ENG-1 教训第三次生效)。
**件B(证明供体 dispatch 无关化)**:per-group 证明经 `ThreadDuration` 导出访问器佩上 window_stats top 记录与 critical_blocking D/IO 候选(发射复用既有 note key,schema 不变);**R2 ×N 折叠证明=成员 AND**(一员未证→合并词存续=诚实词;R1 同事实 absorb 保持 OR);等待对象全员一致才留;D/IO 自身行内联「等待对象 X」。h2 oracle 回升硬形(refined regex 演化至引擎真形集,EVOLUTION RECORD 引 HEAD parity 实录;banned 断言照旧硬拒 /iowait 形)。病根=报告质量取决于模型碰巧 dispatch 哪个查询;h2 当日 0/8 全为此洞采样非回归(失败 dispatch 形干净 HEAD 逐字节同形实证)。
**终态 tally(A+B 在树,全仓绿)**:h1/h3/h4/h5/h6 终态 PASS;h2 011549-r2 **原生 PASS**+两趟演化 oracle 机械复核四维全中,banned 全趟零命中。工件:tieba `20260713-010930.971-54526.md`(7.386 形)/donghu `20260713-011017.358-54575.md`。**新观察(裁定候选)**:eval 自宿主污染类——`--repo .` 自指时模型可 grep 命中测试文件内 witness 值(批前既有,case 文件同含);修向候选=eval 指向空仓/排除 test 面。
**delta 聚焦复核(2026-07-13)**:SHIP-WITH-FIXES——核心全实测成立:单值源(席成员与 census 同一 map 本体,DStateTop=同 comparator 前缀拷贝)、nil-census 生产不可达(两 map 无条件分配+三 rank 车道全直出 ComputeWindowStats)、AND/OR 语义正确(runtime 探针证)、**h2 oracle 演化独立机械重放逐维核真非话术**(两回收趟四维全中/banned 是活防线曾真击发/011549-r2 零污染原生 PASS/可选组承重必要)、tieba witness 真跑非 skip、突变 M1 红数字与账本实录重现。修项(→修复轮三):F1(P1)AND 语义假 pin(负向全员未证 OR≡AND,补 mixed-member pin);F2(P2)parity 只断言 sum 不断言 roster 序(补成员序恒等);F3(P2)双供体互为掩体(补 window_stats-only dispatch pin);F5/F6(P3)RECORD 措辞点名 8 趟+平局链 pid==0 补 comm 键。**F4 自宿主污染实锤(裁定候选)**:011549-r1 模型 grep 命中 proof_donor_b_test.go 并引用 oracle 见证串+编造第 4 段,verdict 零改判(该趟本就 FAIL)但结构威胁真实(仓内 case+test 两处存 oracle 逐字串,未来 prose 逐字引用可四维全过);裁定建议=trace-only 案 --repo 指向密封 stub 仓(根修),短期 manual-audit 加 blob 外读取污染列。
**冷读补验(第二轮,2026-07-13)**:tieba 分区形确定性面 **PASS**——fscache 席 7.386 成员 17 段与真值普查全等(旧 4.739=仅 cpu=3 成员,帽逐出实锤)、余数席三成员恰=无 blocked_reason 段、Σ=18.135 逐 µs 恒等、席序严格按有效归因、全量账口径词零下界残留;上轮两差(Δ2.647/第三 caller 无席)全消。**流程病(立案 ARTIFACT-KEEP)**:两工件被 gold tally sweep 在复放与归档间隙清除(全盘无存),冷读从日志内嵌渲染稿重建验收;日志已抢救归档 `output_archive_v5p1_round2_artifacts/`;修向=eval/复放 harness 的 output 清理必须走归档不走删除。帽外披露核清=设计住 window_stats wire 面(有帽的榜面;席值已全量无帽),该复放未 dispatch 该 view 故零出现,非显示缺陷,正样本 wire 直验在案。**冷读新病处置**:R2-F1(P1)claim-of-absence——prose 收尾断言「未证实的原因不存在/三类覆盖全部段」直接否定同报 ❸ 原因未证 10.433 席=EVID-BR/FACT-REL 族新最重 witness(入 §29.55.4);R2-F2(P2)**批内修**——E4↔E25 W-A 关系句「物理时间重叠(不可相加)」为无条件模板,真值两席成员区间不相交(重叠宣称必须由 typed 成员区间相交推导,不相交时诚实说不相交;SMR「行级判定必须论证成员级可达性」直接适用)=修复轮三;R2-F3(P2)唤醒者清单系统性倒置(D 入口 switch next_comm 当唤醒者,11/11 全错)=EVID-BR 族 witness(入 §29.55.4);R2-F4(P2)donghu 该跑零 D 席=dispatch 结构依赖既有类(§29.55④ 已记)。

### §29.55.6 v5 P1 批 in-flight 披露两则(复核修项 R2,子 agent 记账 2026-07-13;撞号注:原记 §29.55.3 与折叠行立案节同号,改号 .6;终局收账随批终报)
a) **分区改变 LLM 中途可见文本**:§29.50.5 证明分区使分区线程的 rank 席位数+1..N(cause 席+余数),rank 截断普查行 total 随之位移(如 27 vs 25)——首发 dispatch 不受影响(先于任何工具结果),但模型**中途** dispatch 的概率位移非零(截断计数/席位词是模型可见文本)。如实披露:金样本 h2 今日多跑 dispatch 漂移不能排除此贡献,终态 tally 以终态树重跑为准(修项 R1)。
b) **CASE-1 gap(a) 工单偏离记录**:原工单=D/IO 链车道候选「补发 StartTs/EndTs」(published wire);实施=**引擎内部承载**(`CriticalBlockingCandidate.reconStartTs/reconEndTs` 非导出,recon 经 `criticalBlockingReconInterval` 独享消费)。偏离正当性:①发布 hull 端点后投影 span-overlap 折叠臂在 hull 噪声上误触发,金样本 h1 ∿ 帧间空闲席被顶(确定性双红实录:probe+eval);内化后双绿(probe PASS+h1 eval 2/2 PASS);②hull≠发生段——发布面若铸「发生段」词即假单段主张,与 CASE-1 自身「G1 明拒 hull」裁定自洽。recon 语义不减(吸收判据照工单)。

### §29.55.7 立案(复核修项 R3,P3,立案不实施 2026-07-13;撞号注:原记 §29.55.4 与 prose 双 P1 立案节同号,改号 .7):P10 残余披露差集化
v5 P1 件② 在余数席上整体抑制 P10 残余披露(`!unprovenRemainder` 门,防 sibling 已消费 marker 双说)——副作用:被分区线程**从未被任何席消费的其它符号 marker**(批前可披露形)随之失披露;tieba 60555 帽外 witness 同窗可见(19 条 blocked_reason,席面账 15.317<全量 18.135,差额无席面披露;修复轮二件A 后席面已回全量基,本案残余=符号级差集披露)。修向=P10 全窗累计器按语义符号分键,remainder 席披露「未消费差集」(count+symbols−sibling 已消费键);与 §29.55.5 件A 帽披露臂同域,可并批。

## §29.56 v5 P1 批总收账(2026-07-13;件②③④+DET-1+三修复轮;三关全过;件① 诚实未启动留实施地图)
**交付面**:件③ CASE-1(G1 recon 宇宙 {d_state_or_io_wait,io_wait}²+三工程缺口+可证臂=同源区间恒等∧µs 值恒等禁 union-containment+显示臂 donghu 4/4 链行吸收入 36.757 单席);件② §29.50.5 证明分区(per-cause 切片账/(线程,状态族,根因身份) 铸席/未证余数席 typed 词面/假设永不并/R2' 六处/证明族随折 OR 保全+R2 ×N 折叠=成员 AND);件④ CASE-2 witness+CASE-3 W-A 关系句(重叠/不相交由 typed 成员区间推导,修复轮三);DET-1(map 序选举修根+4 趟 byte-identical pin+平局链 typed 全序含 comm 键);**修复轮二件A=ENG-1 补完**(d/io 全量账铸席:tieba fscache 4.739→7.386+hmfs_get_dnode 得席,Σ=18.135 全量基恒等;帽外披露 wire 榜面;「帽基当全量」病第三次生效=INODE/ENG-1/席面)+**件B 证明供体 dispatch 无关化**(window_stats/critical 双面佩证明,h2 oracle 回升硬形)。
**三关**:对抗复核两轮(全批+delta)SHIP-WITH-FIXES 修项全落——delta 关抓两条**假 pin**(AND 语义/parity 序)+双供体互掩,修复轮三六件各带突变红实录(F3 前提经实测勘正=stats-only 树消费形结构性不存在,pin 落记录级发射面);冷读两轮=tieba 分区形确定性面 PASS(fscache 席 17 段成员与真值普查全等/Σ 逐 µs 恒等/席序口径词全对),R2-F2 关系句修后真机重出工件 `20260713-022117.180-79852.md` 冷读 witness 对现铸「物理时间不相交」;金样本终态 h1..h6 全 PASS(h2 011549-r2 原生 PASS,banned 全趟零命中)。**战役里程碑**:tieba ThreadPoolForeg 真根因首次带等待对象独立参赛(fscache_page_wait_o 7.386+hmfs_get_dnode 0.171+hmfs_read 0.145+诚实余数 10.433(原因未证))。
**教训入册**:①验收句必对账本/盘面原文核(子 agent 宣称 §29.55.5 已写实未写+R2/R3 撞号,主会话推送前抓获修账);②测试头注的突变自检宣称=承诺面,必实测(两条假 pin);③fixture 种子选举可掩蔽 veto 臂→鉴别 pin 落合并权威单点;④分区键教训(修复轮一):cause 织入 ledger 分组键→帽内条目膨胀逐出无关行,金样本 h1/h2 当场咬红——分区必须下沉切片账,键形保持;⑤eval 复放工件保留链(ARTIFACT-KEEP):harness 清 output 必须走归档。
**遗留队列**:件① B.2 一段一席引擎铸造(实施地图在批终报)/CASE-3 ❹ 混合形合并词/BINDER-CENSUS 披露臂(1ms 链睡眠底限披露)/wire-fold folded_sum note/§29.55.7 P10 差集/eval 自宿主污染裁定(F4=密封 stub 仓方向)/§29.55.4 EVID-BR+FACT-REL(prose 双 P1:blocked_reason 入证据面/claim-of-absence 事实并陈扩臂/唤醒者倒置)/h2 dispatch 敏感观察/单成员 D/IO 吸收扩围/§29.55.3 折叠行记号(P2a rider)。

## §29.57 EVID-1 批收账(2026-07-13;§29.55.4 EVID-BR+FACT-REL 修根;主批+修复轮六件+收尾四件;三关全过)
**病象(冷读实锤)**:「等待对象」题四跑四答案全错(typed 附注每跑均对=模型看不见 blocked_reason 证据)/claim-of-absence 否定同报未证席/唤醒者清单 11/11 倒置(D 入口 switch next_comm 当唤醒者)。
**件① EVID-BR(修根)**:`internal/context/trace_wait_evidence_summary.go`(CR-1 榜摘要模式)——观测 ledger 铸「Wait-Object & Wakeup Evidence」节 explore+finalize 双阶段喂入:等待对象 per-thread caller 事实(verbatim 值链)/pid 键 blocked_reason census/唤醒边 sched_wakeup 源直出。修复轮升级=**census 根修**:banner 解析(top-8 截断面二次聚合病根类+blob 预览脆弱)退役为 fallback,tracequery 从 `foldBlockedReasonFullByPID` 全量累加器铸 typed per-pid census note(per-caller 全枚举+Σms 宁缺勿假门+溢出显式);Event 捕获 vendor delay=(双 int32 槽,core 688B 回 ratchet 内,µs 单位真 trace 实测确认);R2' 六处+五结构 tripwire 真实咬红 deliberate 收账。帽=anchor 线程永不落帽+三帽溢出 pin+边帽首尾混采。
**件② FACT-REL(§29.53.2 纪律内)**:事实并陈扩臂——同线程多状态值 token 在场→五道分区事实(io_wait 第五道+平衡容差门自有常数,不平衡不铸恒等)+席位清单余数席永不过帽;回退嵌合体臂砍除(宁缺勿假)。**backfill 权属终态**:「系统按已验证证据补充」face 借壳病(复制模型假句模板,含英文走私面)两轮根修至 **typed-token 白名单铸造**(k=v/name-tid/符号/数值/封闭状态词表;任何语言句子文本零存活,叙述成员落中性指针)——披露面=承诺面,系统行零消费模型文本。
**三关**:对抗复核两轮(主批+delta)SHIP-WITH-FIXES 修项全落(LLM 串零内部词/L1 绿/假 pin 抽查 load-bearing;delta 关抓 Σms 部分覆盖 pin 缺口+英文走私面,收尾轮闭);冷读三轮:**三判据独立确认**——tieba 三 caller 全枚举与 19 条真值逐项全等且可见面逐字消费、借壳句零出现、回退臂砍除后全分区席复现+donghu D 席 3/3 漂移消退;donghu census 引用句(12 次/Σ39.157ms)复算全等;E1-F2 杜撰零复发;唤醒边抽 4 核 4。**修根战果**:等待对象实体层三跑同答案且正确(病基线四跑四答案→9/9 dma_fence 正确)、tieba claim-of-absence 消退(未证份额明报)、唤醒者倒置零出现。
**冷读终判与下批方向**:确定性面零新病;**残余全部收敛到「模型 prose 再演算」单一主战场**(喂入值全对,模型引用时自算错:开篇 18 次 +1 漂移/2.731=余数席 10.433 减三 cause 席 7.702 的跨口径错减/唤醒者汇总 8× 自计数 vs 自家清单 12 项)→ **PROSE-RC 立案**:feed 具名事实化(per-waker 计数事实+「余数 N 条未列」+余数值具名),让模型引用而非推导(census 臂同构复制)。
**残留立案**:E1-F2 口径杜撰观察续档/pid+1 计数漂移轻残/「围栏持有者」measured-waker 升格=软措辞守护候选/回退树自身分区席(排件① B.2)/census note 中途可见文本 +1(§29.55.6a 概率位移类)/未证份额「全窗 vs 已探段」口径裁定候选/per-subject census 门候选/fallback 车道 MAX 自认降级/「(见定位)」等词面族 zh-en 合审候选/h2 dispatch 类照旧。

## §29.58 PROSE-RC 批收账(2026-07-13;§29.57 立案的 feed 具名事实化;域=internal/context 两文件;三关全过)
**四臂交付**(EVID-1 三车道上扩):①per-waker 计数事实(ledger 边去重后全量聚合 ×N,count desc+(waker,wakee) 字典平局键,帽 8+帽外具名余数;口径标签=observed wakeup edges only,因引擎边发射受 typed 行帽,禁全量口径包装);②余数具名事实(verbatim 席值+互斥分区「禁互减」性质句+**收尾姊妹句「无内核记录 caller,禁挂任何已证 cause 名下」**);③census 总数导语(全量累加器 Count verbatim,caller-帽无关引擎侧核实;banner fallback 不铸总数);④余数成员性质句(N 成员共同构成未证份额;member_count=1 产线结构性不可达)。教学句全 CR-1「Use these values verbatim」同族(复核逐句核=证据喂入面车道,非 §29.53.2 定性面)。
**三关**:复核 SHIP-WITH-FIXES 七面全过(口径链核实到引擎端/L1 绿/突变 3/3+假 pin 2/2 判别)——F1 census fixture 引擎不可能算术(total 22 vs 自洽 21;帽外符号 count≤枚举最小值不变量被注释断言成假形,fixture 实铸形红线再犯)+F2 平局键 pin 无判别力,收尾轮修真(逆字典序 ts 插入 load-bearing);冷读=**再演算三形两消退一改道**:+1 计数(18次)与跨口径错减(2.731)独立确认消退;自计数在已喂入边忠实引用、未喂入实体照旧倒置造数(**PRC-F1 witness:「OS_IPC_14_34911 ×4」三重假**——raw 全文件该对唯一边方向相反且仅 1 条,fed 面无此行)。
**反效果教训(重要)**:模型遵守「禁互减」后**再演算冲动改道成绑定**——054419 把余数席 10.433 连 fold 属性整体搬进 fscache 名下(claim-of-absence 隐性变体+第三维缺答)、052947 段重绑+杜撰时段区间(首段起点+末段终点缝合)——**性质句族必须封闭所有改道方向**(减法→绑定→?),姊妹句已补绑定向;后续观察第三改道向。
**立案**:WAKE-CENSUS(引擎域,排 B.2 后):wakeup_chain 全量边集(pre-cap)铸 per-(waker,wakee) census note(blocked_reason_census 根修同构),context 端口径升 window-total,PRC-F1 类可灭;census 铸造 dispatch 依赖观察续档(候选=census 随 critical/timeline 结果同铸或计数类问题软引导);实体绑定 witness 两条移交 CR-4 族(余数挂名/三口径混称);context 包无 AST lint 门(现门只扫 tool+agent)=扩门候选。

### §29.58.1 立案(用户 2026-07-13,工件 20260713-062104):自身段三层级混同一视觉形(并入 v5 P2a rider)
用户问:`◦ 自身·binder`/`· 同段IO另有` 与 `☾ sleep`/`⛓ IO等待` 是否同级——「◦/· 像枚举行,又看不出上下级」。核实(md 同缩进 `│     `):自身段实混三层级——①状态行互为同级;②组成部分行(◦ binder 词面自报「为[E1]的组成部分·不可相加」=sleep 子集视图;∿ 帧间空闲候选)画成同级,包含关系只活在词面;③`·` 旁注(⌗ 口径旁栏语义)非行,与行同缩进仅靠 ◦/· 两个最相近小点区分;旁注首行缩进还与自身折行(更深)自相矛盾。**处置(沿折叠行裁定分工原则:结构管关系,词面管语义)**:a) 旁注行整体比宿主行深一级(与折行对齐);b) 组成部分行紧随父行+`↳` 从属连接符(v5 双行 ↳ 先例),binder 行归位 sleep 行下;c) ∿ 帧间空闲落地时先核 typed 关系(cadence-idle 是否 sleep 子集再分类)再定层级;词面「组成部分·不可相加」保留。lockstep 面照 §29.55.3(tracefence 闭集/fence 分类器/census/宽度 pin)。

### §29.58.2 点状记号全域普查(用户指令 2026-07-13「◦/· 非枚举像枚举,全域排查」;普查表 scratchpad/dot_glyph_census.md;处置并入 P2a rider)
**总盘**:live 点/圆族恰 4 形(⊚ 根/◌ 盲区/◦ 中性一形三义+折叠第四佩/· 行头从属+词内分隔双角色),∘∙⋅•●○◉◎ 零 live 铸造无漏网;图例 markdown `-` bullet 与树内记号零碰撞(健康)。
**新发现四条**:F1(最重,承诺面 falsity)——◦ 图例条目(tree.go:933)宣称「有形态词的行戴各自形态族记号」,binder 行有形态词(binder等待候选)却佩 ◦,且 BinderWait Mark 借用 IconNoDominant 致 binder-only 报告点亮「数据行」图例条目(062916 双行实锤)=图例说谎,专属字形落地即自愈;F2——◇/▒ 区段跨线程折叠行 `◦ 其余 N 项合并` 与链上折叠同病但 §29.55.3 裁定原文只点链上(区段折叠并入「记号位留形态族+边词/行名分工」同臂);F3——G11 段级旁注 `│ · 另有 N 条自身等待症状行`(tree.go:6027)独立铸造点,不在 §29.58.1 a) 臂点名内,随旁注深一级臂单点跟改;F4——确定性优化点表 `· 成员/· 其余` cell(runtime.go:4606-4618)候选 ↳ 与 v5 先例统一。
**binder 借用义专属字形裁定(默认最优,第四次分裂先例)**:首选 **⋈ U+22C8**(EAW=N 单格/无 VS16/Math Operators 块同 ∿ 覆盖论证/两方对接=IPC 会合语义/零光学碰撞;自动入 TestRCRImpactFormGlyphsSingleCellNoVS16 圈);次选 ⇌(transaction 最贴但与 ⇅ 箭头族亲缘暗示风险)。Mark 同步拆分(BinderWait 不再借 IconNoDominant)+图例条目随改。
**P2a rider 终清单(至此聚齐)**:折叠行用户裁定形(§29.55.3 链上+F2 区段扩围)/自身段三层级(§29.58.1:旁注深一级含 F3 G11+组成部分行 ↳ 归位+∿ typed 关系核)/binder ⋈ 分裂+Mark 拆分+F1 图例修真/F4 优化点表 ↳(裁定候选)/词面族 zh-en 合审(§29.57 残留)。

## §29.59 v5 P1 件① B.2 一段一席引擎铸造主批收账(2026-07-13;主批+修复轮五件;三关全过=对抗复核 SHIP-WITH-FIXES 修项全落+冷读纯改善零回归+tieba 84 行逐字节恒等独立核)

**勘正(F3 教训执行,动工前实测)**:①实施地图前提「引擎完全漏同段等值双席」部分证伪——HEAD cd2ca239 的 R1 同事实合并已收敛 14704 原型(候选行+裸 root_evidence 副本,同主体+%.3f 同值+同精确行区间;records 级产线发射形探针实证,固化为 pin)。引擎真实残口=无值裸副本(R1 键要求正值)/keeper 值仅在 eff·actual 车道/×N 席成员重发(值配成员非席)/两车道异行包络各自 R2 后的 ×N 孪生席。②显示层同段臂族全景大于地图(RNB R2/P5 两臂/WO-D2/WO-D3/ValueMirror/WO-A1/WO-C1/家族镜像/证明传播),逐臂按载体可证性处置,禁半形。

**交付面**:引擎铸造 `internal/types/trace_causal_projection_oneseat.go` 挂 AggregateForPresentation 聚合序三臂——**arm A**(R1 后)裸 root_evidence 同段孪生收敛=(主体,精确行区间)孪生键+显示链值 3dp 恒等(无值裸副本也收)+状态词一致门(显示门逐字镜像)+跨窗否决+1:1 歧义 fail-open+**keeper 诚实席门**(修复轮件1:◌/⊘链止/typed TraceGapKind/⌗ 非状态 token keeper 永不吸收,防诚实席被回填矛盾状态词;两把尺 keeper 侧同禁);keeper 空状态槽收编裸词仅限注册状态词(B.2「raw state 降入行2 状态槽」)。**arm B**(R2 后)成员重发收敛(25846 形)=µs 恒等对无损可导成员值(count2/count3+净Σ)+时长族一致(注册状态词族∪verbatim token 恒等,**注册表零拷贝**)+行包络必要性否决(P2-3)+跨窗否决;E# 并入,×N 计数与账目字节不动。**arm C**(R2 后)×N 孪生席收敛(42729 E9/E15 token 叉形+S8-TPF 同 token 跨区段)=段集指纹(主体+count+display+extrema 3dp+查询窗)**恰 2 席**(≥3 fail-open)+token 恒等或单侧缺失叉形(在场 token 须解析时长族=⌗ fail-open;双 object sentinel)+**W-A 否决**(双正 cum/eff 3dp 分歧不折)+rank 席 fail-open;fold=E# 并入+DuplicatePublications 既有披露车道,永不重加和(×3 恒 ×3)。源头铸造:`RootEvidence.DominantState` wire 字段(verbatim 抄 impact 孪生)+root_evidence 记录发射既有键 `dominant_state` note(missing_wakeup/trace_gap 诚实行不带)。

**过渡臂处置**:**CR-2 P5 等值臂退役**(per-carrier 引擎 pin 红→绿:valued=R1 既有覆盖/valueless+eff-only=arm A;显示等价=records→引擎→显示 E2E 单席+[E#(+N)] merged_ids 形,修复轮件3 补 valueless/eff-only E2E 变体,arm A 摘除态双红实录;「同段镜像已并入」interim tag 随臂退)。**P5 成员臂诚实保留**=legacy 车道防线(旧记录无 dominant_state note 且 Predicate 非注册状态词(d_state_or_io_wait 类)时 registry 查询是唯一族证明;修复轮件4 census fixture 换 legacy 因果 token 形=显示臂独占可达形,配引擎负向 pin 佐证边界;退役条件=legacy 载体消亡)。**WO-D3 保留**(witnessed 载体已被 arm C 收敛;余=引擎故意 W-A fail-open 对)。**WO-D2 保留**(56643 witnessed 对=eff 分歧,arm C W-A 否决 by design;退役条件=typed 双 eff wire 载体裁定)。**WO-A1/WO-C1 保留**(载体=嵌套账形≠段集恒等双席,不在 B.2 域;C1 退役条件照旧 CASE-3)。V4 单侧 token 放宽注释勘正(非死码,与 arm C=一学说两位置)。

**词面默认裁定(设计稿 重-5,用户可推翻)**:本批实装**零新词形**(merged `[E#(+N)]`+明细回查,方案2 形)——`双视图并席`※ chip 未铸;若用户按口味终裁选 chip,词面批随 P2a rider 补。

**pins(20 条 types 侧+3 条 tool E2E,修复轮后修真)**:勘正① R1 覆盖/arm A 三载体红→绿/arm B 25846 形红→绿+非成员值+诚实席不折/arm C token 叉形+同 token 跨区段红→绿+W-A cum·eff·rank·⌗ 四否决/分区席相容(§29.50.5)/修复轮五件=keeper 诚实席负向 pin(M1 突变红)+诚实 raw 排除判别 pin(M2 红)+跨窗否决判别 pin(M3 红,无值形专属——值形为 R1 车道)+armC ≥3 副本 fail-open pin(M4 红)+E2E 变体(M5 armA 摘除红)+legacy 载体边界 pin;突变实录五发五中,全部恢复后全绿。

**验收**:全仓 `go test ./...` 直读绿;金样本 h1/h2/h3/h5/h6 PASS+h4 首趟 dispatch 缺派发 FAIL(未派 wakeup_chain/rank→受热限压·全窗四态载体未铸)→同二进制复 roll PASS=§29.55.5 件B dispatch 病族非引擎门(纯净 main 同题趟 faces 在场佐证);tieba 复放投影 fence 与批前基线逐字节恒等(分区席/口径词/席序零漂移);donghu 复放两趟互为字节恒等(确定性面稳),对批前基线 diff 全部逐条归因 dispatch 派发集差异;.ugc.aweme running 行 [E22(+1)] 实证引擎单席产线生效。工件归档 `.codrax/output_archive_v5p1_b2_pre/`、`_post/`(ARTIFACT-KEEP)。

**披露与勘正**:①`dominant_state` note 为模型中途可见证据文本(§29.55.6a 概率位移类)——donghu/h4 dispatch 漂移经纯净 a460f903 二进制隔离对照归因主线侧,但本批贡献不能排零;②**dispatch 方差勘正(对抗复核 F2)**:初判「纯净 main 不派 rank」系单趟过度概括——复核 pristine 复现趟(工件 scratchpad/main_pristine/…065259)派发 rank 且 6 榜行在场,正确表述=**派发集方差**(同题同二进制趟间 rank/critical 派发不稳定);PROSE-RC 回访核查对象=派发率方差是否较 PROSE-RC 前变化,非固定漂移。

**遗留立案**:①【P2】D-state 裸 root_evidence 行 blocked_reason 行号覆写(query.go StateDSleep 臂 root.LineEnd=reason.Line)致与 impact 孪生行区间失配——引擎与显示双层皆盲;修向=行号独立 note,先取产线 witness。②【P2 观察】R1 值车道无诚实门:值形 undrillable raw 孪生(同值同区间)今日仍被 R1 折并转移 UndrillableReason(批前既有行为,件2① pin 刻意用无值形绕开);裁定候选=R1 增诚实门或维持(◌ 值形双席本身待 witness)。③【P3】P5 成员臂 legacy 车道消亡跟踪。④【P3】arm C 双正 eff 分歧对的 typed 双 eff wire 载体(WO-D2 终局退役前置)。⑤【既有队列】CASE-1 单成员 D/IO 吸收扩围/A1 嵌套账裁定(CASE-1 扩围/CASE-3)。

### §29.58.3 立案(用户 2026-07-13,工件 062104 witness):自身线程入 ◇ 邻近区段+跨通道同线程零互指(SELF-LANE,排 P2a 后显示批)
用户问:◇ 与链上同窗时,链上有同线程该线程还进 ◇ 是否不合理,尤其目标自身线程也入 ◇。核清三层:①◇/▒ 与链上同一查询窗,通道=因果证明等级逐段判定(chainContextForCandidate 按 (线程,区间) 对链节点窗重叠);②同线程跨通道并存=在案裁定保护(§23.1 DCS 道别红线:同线程身份非因果,无重叠段翻链上=伪造重叠;huadong E21 先例)——不可改判;**但跨通道同线程对零互指=SMR-1 关系句族漏网**(061053 ◇ 行无「本线程另有链上席 [E#]」指针,读者无从辨刻意分段 vs 重复);③**自身线程入 ◇=呈现层真病**(062104:trunk=.ugc.aweme 而 ◇ 内两行 .ugc.aweme 自己 E28/E29,同报自身段另有 E4)——段级车道诚实但「邻近」词面承诺=其它线程在旁竞争,主体非自己邻居;自身主账被拆两区块。(061053 同形实合理:该报 trunk=CompThread,.ugc 行是真邻居——判定必须以 trunk 身份为准。)**处置(SELF-LANE)**:a) 目标线程非链席不进 ◇,归位自身段(佩 E#/席位+「非链」限定词,不可相加纪律照旧);「邻近区段」词面只留非目标线程;b) 跨通道同线程互指句(「本线程另有链上席 [E#]」双向)入 SMR-1 关系句族;c) 落地按 P2a 件2 自身段新结构(旁注深一级/↳)之上实施,禁与 P2a 在飞批冲突,排其后。

## §29.60 P0 立案+用户裁定(2026-07-13,客户 witness /Users/han/opt/customlogs/endless_loop.txt):完成后系统硬绑定再探查致答案严重偏离——系统的判定完全要尊重模型
**用户裁定原文要义**:模型第一次调用 emit_investigation_complete 时已给出很合理的答案;后面被系统硬绑定去多次探查,导致答案严重偏离,非常严重——**系统的判定完全要尊重模型**。
**客户 witness 形**(donghu 970481 丢帧案,MiniMax-M2.7):一次会话 **18 次** emit_investigation_complete;循环=`✓已完成证据收集→交叉验证→↻ 2/4 正在补齐校验信息→重开收集→…`,截止仍在循环;首次完成答案=shadowhook-task-64305 runnable 等待(合理),被后续强制钻取拖偏。三机制叠加:①校验补齐 4 槽清单反复不满足→重开收集(livelock 家族);②校验 retry 指令强令「deeper runtime-trace drill-down…window_sweep 热点」——把模型拖离已得结论;③「emit_investigation_complete 多次结论冲突」指控臂=**系统比对模型 NL 结论**(§29.53.2 禁区家族)。另本地 witness(22164 日志):one-shot 唤醒链下钻降级臂,iter=4 降级点名外围 irq 线程(非用户目标),iter=6 核销通过——核销判据若 iter=4 已满足则该轮纯浪费。
**修复原则(本裁定+既有红线合成)**:模型的完成判定=终态,系统不得以任何再探查硬绑定推翻;校验/补齐类系统检测一律降软(§29.42.4 检测→披露:附注/下轮上下文提示,不阻断不重开);结论一致性指控臂废除(系统禁比对模型 NL 结论);完成门降级臂全面复审(typed 硬信号臂逐个对照「无论谁对都成立」标准,不满足即降软)。RCA 在飞(scratchpad/completion_gate_retrigger_rca.md),修向按本裁定收束后报批实施。

### §29.60.1 裁定收窄(用户 2026-07-13,勘正 §29.60 表述):非全局「完全尊重」,限定两条件
用户勘正原文要义:「系统的判定完全要尊重模型」过于武断——裁定**针对 emit_investigation_complete 这个完成信号**,且限定**非致命错误必须尊重模型的判定**。即:①适用面=完成门(模型宣告证据收集完成的判定权),非系统全部判定面;②致命/结构性错误(「无论谁对都成立」类:零工具见证、必需 typed 工件缺失、schema 无效等)系统仍可拦;③非致命类(链下钻建议/校验清单补齐/结论一致性/覆盖度建议等)必须尊重模型完成判定——降软为披露/下轮上下文提示,不得硬绑定重开收集。与 §29.42.4 框架合一:探索完成面与答案出厂面同标准。RCA 处置清单按此校准(每臂先判致命/非致命,再定废除/降软/保留)。

### §29.58.4 立案(WAKE-CENSUS 批件4 核验实锤,2026-07-13):WAKE-CENSUS-D——D 退出唤醒边结构性缺席
wakeup_chain 引擎唯一铸边点在 expandChain 的 `case StateSSleep:` 臂(findWakeup→res.Edges);`case StateDSleep, StateIOWait:` 臂只走 blocked_reason 不查 wakeup 不铸边不递归(findWakeupForWithSelection 本身状态无关,限制纯在 switch 臂)。donghu witness:窗内 raw sched_wakeup 29 条,12 条 D 退出全由 gpu-token-id4-2931,引擎 28 边零 gpu-token 对。后效:census 无法喂 D 退出计数,冷读残余漂移(+1 自数/×1 低报/时刻错绑)全部集中此车道。**修向裁定点**:a) D/IO 臂铸边是否递归 waker(blocked_reason 已占 D 因果车道,双重归因风险;D 唤醒者常为 IRQ/完成上下文);b) 边需 typed 种别(sleep-exit vs D-exit)——viaMonotonicHops 与 rank 车道都消费 res.Edges,新边会改 via 判定与排序参赛。**候选合并设计**:链无关的 window-total per-wakee sched_wakeup 直查 census(浅展开口径观察①同治)。

## §29.61 VerifyClass 语义席位调查结论+裁定(2026-07-13,用户问「语义 span 确定性优化权重是否偏低」;调查报告 scratchpad/verifyclass_weight_rca.md)
**核心发现:权重前提结构性不成立**。席序=通道优先(on_chain<adjacent<background)→发布 eff 降序→Score 仅同值 tie-break(SEM-LEAD 复核后 Score 不定序)。VerifyClass 低位=两条精确机制:①目标自身语义 span 发生在 running 段,与链窗(等待/影响段)结构永不重叠→mint 判 adjacent(道别红线:同线程身份不翻链上),◇ 通道 tier 恒 tertiary 不冕,bundle_top_cause 恒为链上行;②家族并集 13.006<26.392 同尺墙钟诚实排后。纸面推演全对账(23.067=26.392×0.76×1.15);反事实:置信抬 0.90/权重抬 1.35 席序零变化。**C4 兜底可靠**(每次答案落盘执行与轮数无关,首轮出厂形覆盖;仅两诚实降级形,无静默消失路径)。
**裁定(按调查建议默认处理)**:a) 置信档抬高**不做**(仅词面零席效;若做只按匹配臂分叉的精确形);c) 临时调类型权重**明确不做**(零席效+发布分数全网漂移=§29.7-2 乘子泄漏同病);b) **ELIM-1 提优先级为下一引擎批**(GREENLIT 已在案;总览 TOP5 链∪◇ 纯 eff 降序把 VerifyClass 13.006 抬到全板第 2 行佩「确定性优化·候选」身份词;设计稿明拒 eff×conf 键;本客户案(endless_loop/根因XX-VerifyClass)入 cust710 复放验收清单=ELIM-1 第一客户 witness);d) C4 表加占窗%列(8.75%,无折算合法百分比)随 ELIM-1 批+**ADJ-MINT 立案**(adjacent 语义行恒铸 rank item,否则总览对缺席行退化为指针)。
**开放终裁(SELF-SEM,用户裁定项)**:目标**自身**确定性语义 span 恒判 adjacent 是否复核——SYM-2 先例已裁自因可拆解族入链上选举;自身 running 段内的确定性语义 span 是目标关键路径的可直接消除子成分(无跨线程宣称,无伪造重叠问题——道别红线的保护 witness 是他进程 span,结构不同);精确信号具备(SubjectIsAnalysisTarget∧确定性语义类闭集)。若裁入通道 1:本案 VerifyClass=根因排序#2+❷,不动任何权重。因触碰 §23.1 道别红线边界,裁定先行不默认。

### §29.61.1 用户裁定(2026-07-13):SELF-SEM 采纳——目标自身确定性语义 span 窗内入链上通道
用户裁定原文要义:目标自身(的确定性语义 span)如果在用户提及的窗内,按道理应该算作链上——合理。**落地形**:①精确门=SubjectIsAnalysisTarget ∧ 确定性语义类闭集 ∧ span 在查询窗内(自身 running 段内);②入链上通道=参赛主席位家族(SYM-2 自因族先例延伸),词面佩「自身·确定性优化」族,**不铸唤醒边不宣称跨线程关系**(链上=通道/宇宙身份,非 wakeup 边宣称);③§23.1 道别红线对**非目标线程零改动**(其保护 witness=他进程 span,结构不同,红线原文不动);④与 SELF-LANE(§29.58.3)互补:自身确定性语义 span 升链上通道后自然离开 ◇ 显示区,残余非链自身行按 SELF-LANE 归位自身段。**witness 验收**:客户案(根因XX-VerifyClass)VerifyClass 预期=根因排序#2+❷ 徽章;随 ELIM-1 批实施(§29.61 b 已提优先级),cust710 复放清单同验。

## §29.62 WAKE-CENSUS 批收账(2026-07-13;§29.58 立案实施+修复轮五件;双关全过)
**交付**:引擎 `buildWakeupEdgeCensus` 在**全量边集 pre-cap** 铸 per-(waker,wakee) census(count+首末 ts+sched_wakeup 真方向 verbatim;挂接点在链展开全部路径之后=截断二次聚合结构不可犯;pair 帽 16+双溢出,target-wakee 对帽免疫引擎+tool 双面);tool 发射 `wakeup_edge_census` 观测+4 note key(R2' 全走,**第 7 处=tracediag ChainResult schema pin,首轮漏、修复轮补,教训:census 形引擎字段的 R2' 清单含 tracediag pin**);context 主源 census+full-inventory 口径词+方向 pin 句+**缺席性质句收窄至 census 口径**(bundle 混跑下「ZERO in this run」过强被复核抓获,修「was never measured with a per-pair count here」)+WC-F1 恒真标签(「absence=测量集范围,非内核行为,无需机制解释」);union 溢出多 scope 去数字化(可错定数类整体灭)。
**双关**:复核 SHIP-WITH-FIXES 五件全落(挂接点 5 路径核/免疫账/零位移/假 pin 双查真);冷读=**PRC-F1 三向封 3/3 消退**(造数/方向反/未喂入),R3B census 与 raw 逐对全等(×7 首末 ts 行级核真),缺席句误读零实例;新报仅 WC-F1(P3,已就地修)。**冷读终判:唤醒者题族残余 prose 病灶根供给源收敛到唯一缺口=D 退出边不在测量集(§29.58.4 WAKE-CENSUS-D)——落地后该题族有望首次 prose 全绿。**
**遗留**:P3-3 census 导语口径微调(可选,已被 measured…only+尾句围住);census 铸造 dispatch 依赖续档;WAKE-CENSUS-D 排 ELIM-1+SELF-SEM 合并引擎批。

## §29.63 P2a 显示 rider 五件套收账(2026-07-13;用户三裁定落地;复核 SHIP+冷读三问闭环)
**交付**:件1 折叠行裁定形 `├─链上─ <形态记号> 其余N项(折叠)`(边词省略臂退役+行名五铸造点 lockstep dedup+◇/▒ 区段扩围单一门+图例条改;**顺手修真批前欠账**:折叠行头名中截=承诺面欠账,落 typed 保护 floor(计数 stem+首成员+榜位指针永不截),donghu 头名首次整名+b6 指针复活);件2 自身段三层级(旁注深一级含 G11/binder 行 ↳ 归位 sleep 宿主下/**∿ typed 核验=不相交独立上下文,诚实保持同级**(ENG-2 铸造点+µs 论证:sleep 成员 1.337~14.302≠idle 15.758));件3 binder ⋈ 分裂(U+22C8+专属 Mark+图例单源生成+◦ 双佩豁免删除硬化+F1 图例修真);件4 优化点表 ↳;件5 词面修真((见定位)→(见定义位置)/(see Location) 等)。
**三关**:复核 SHIP 零必修(五表 lockstep 全数核/floor 截断数学+双 pin 真/∿ 结论独立复核成立/DOM 独立量测 3/3:折叠行记号与兄弟行 0.00px 同列/旁注恰深 2ch/信封同宽);冷读=**用户三问闭环**(三裁定形全在场+图例「点亮⟺佩戴」逐记号互证零单边+值面零回归+html 同源)。**h2 词形漂移信号裁决=非 P2a 回归**(090948 witness 实为平铺回退树:未 dispatch wakeup_chain/window_stats→自身段结构性不存在,线程名头形=HEAD 既有平铺臂;A/B 九文件隔离两树两形逐字节全同;「clean-HEAD PASS」=小 N dispatch 采样假信号,case 自档 clean HEAD 连续 8 跑同类)——**建议入裁定池:wakeup_chain/树席位面 dispatch 无关化**(§29.55.5 件B 同款扩展,金样本 h2 稳定的正解,勿显示层打补丁)。
**修向队列**:P2-1 双向图例 sweep 增 p2a_self_carve fixture(**已就地落地**);P3-1 链式组成部分递归/P3-2 keep-suffix floor 旁路/P3-3 手工形词面 fork(产线不可达)/P3-4 注释卫生(types 旧词形 4+2 处)。**呈现取舍记录**(用户可推翻):头名保护+5 格列宽联动(tag 降行2 零损);区段折叠记号右移 6 格(边词占位,区段兄弟无边词结构上无法同列)。

### §29.60.2 完成门修复批收账(2026-07-13;§29.60/.1 裁定落地;RCA+实施+复核修复轮;endless_loop P0 关账)
**RCA 关键正名**:①「结论冲突指控臂」系统中不存在(穷举确认)——witness 该句为模型对「反复清零+同指令重发」的自我归因;②「核销次序缺陷」假设证伪(核销先于降级,iter=4 时账内确实零 wakeup 观测);③endless_loop 唯一驱动器=预终结门 checkTier1Floor 失败→requeue 全节点+ResetInvestigationComplete 清零模型完成判定,多路拓扑放大成循环。
**修复(按 §29.60.1 二分)**:P0=预终结门两臂废 requeue 改披露(discloseTier1FloorGap→FloorDegraded→双语系统 caveat 咽喉;不再 requeue/清完成/烧预算);Reset 收窄 typed 载体三车道(零见证/FallbackBackToExplore/strict-SC,逐条论证致命性);R5 wakeup 臂/defining-proof/覆盖类 forced-reads 降软(检测逐字保留,typed escape 纪律);致命 7 项+C2+member_set 保硬。**复核修复轮五件**:披露词面按臂分形(followup 臂原共用「比例低于标准」文案=答案面假陈述,修为说真话双语文案);**披露死信接线**(三降软 lane 的 CompletionCaveats 原零用户面消费者——「检测→披露」披露端未通电,复核抓获;修入 appendSystemCaveatsToAnswer 咽喉 E2E pin)+gate note 槽 Set→Append;forced-reads 最高风险形回收(ExactTargets∧AxisCall 保血 citation 类);**回收车道恢复(裁定边界勘正)**:无完成信号退出(IsInvestigationComplete==false∧StableReason=="")保留一次有界 requeue——该处无模型判定可尊重,§29.60 不适用(CSP-RM §29.21 保活+cmp_792 witness);已完成路径零 requeue。
**验收**:修后 7/7 首 emit 出厂、0 降级轮、0 requeue(修前 donghu 4/4 requeue×4);R5 频率=5/7 模型自愿跟随核销,advisory 零实发;c3 tieba 复放 PASS 且答案行携带说真话新文案(件1 真机实锤);金样本 FAIL 双条 clean-HEAD 对照排除(h2=P2a 平铺形既档车道复核裁决非回归;h5=oracle 子串精度 ×39 撞 ×3,EVALFIX 立案);livelock 反向兜底核实(ShouldStop 两段 cap/stall/blocked-DAG 强制 finalize 全在)。工件归档 customlogs/completion_gate_rca_20260713/。**流程教训**:witness log 轮转吃 6/7 晨间日志——复放前必先归档(ARTIFACT-KEEP 第三次);RCA 先行+裁定收束+分级处置=完成门这类雷区的正确开刀式。**遗留**:客户侧 keep-alive 分支 debug 实证(P2 卫生)/h5 oracle 词边界(EVALFIX)/FallbackBackToExplore 质量 kind 二分审计候选。

## §29.64 RANK-U Stage 1 收账(2026-07-13;SELF-SEM §29.61.1+WAKE-CENSUS-D §29.58.4 2A;设计稿 ranku_design_20260713;三关+修复轮六件)
**SELF-SEM**:共享谓词四 typed 条件(链宇宙非空∧Target 解析∧sameThreadRef∧确定性语义六类闭集单源);Causality 新闭集 token `self_deterministic`+OnChainBasis wire 字段(零唤醒边宣称);enrich 保持臂(无条件覆写点会抹 mint 裁决——46ms 家族包络假重叠实测被拦);口径=窗投影并集 fail-closed(eff==union 恒等式=乘子泄漏防线,M2 突变实证 boost 泄漏会 2.388→6.209 连带翻 primary);§23.1 非目标线程零改动五形负向 pin;冕格全权(裁定④)pin 固化。**W1 witness**:donghu JIT 语义 span 通道翻转实录=邻近影响#1→`❷ ✦ …自身·确定性优化·根因排序#2`,eff 值不变;14 件真实工件宿主非目标 JIT 诚实留 ◇/▒(零误翻)。
**WAKE-CENSUS-D(2A)**:census 换源链无关 window-total raw sched_wakeup 直查(种群=target∪链节点线程;零触 res.Edges=八类边集消费者零波及,拒铸边理由存档);pair 行 sleep_exit/d_exit/other_exit typed split 双加恒等;强缺席句+种群性质句(冷读 RU-F1:单句把缺席外推全窗全域,raw 种群外 d_exit 配对 38 个——性质句含显式反例禁令);总数导语 anchor 臂 per-pair target_wakee typed 记号(会话级 anchor 授权退役——复核 F1 跨 scope 假定数 TOTAL 铸造面关死);出处门 fail-open。**W2 witness**:census≡raw 29 行逐对全等(gpu-token ×12=11 d_exit+1 other 诚实桶,idle-3 欠计修);**run6 唤醒者题 prose 全绿**(开头逐字引 TOTAL 29=17+11+1,题眼「D 唤醒源=gpu-token 唯一」首次三关全过=§29.62 冷读预言应验)。
**三关**:复核 SHIP-WITH-FIXES(突变 8/9 红;唯一存活 M5 由修复轮件3 双面判别 pin 关死);冷读双 witness 达验收;**h3 归因关账(件4,教训级)**:6 连绿→批内 2 连红,baseline 对照实锤=tool Description 中段插入句扰动 dispatch(批内跑零 rank dispatch)→修根=Description delta 归零(与 baseline 逐字节恒等,on_chain_basis 教学移数据处 Summary/图例),h3 复绿 2/2——**LLM-facing Description 面=dispatch 敏感面,新 note key 教学禁入 Description 中段(R2' 描述位刻意豁免需记录)**。tieba 零波及字节级独立确认。
**事故披露**:突变恢复误用 git checkout 整文件回滚(该教训第三犯),上下文全量重建复绿,cp 副本纪律实测恢复。**遗留**:gpu-token 第 12 次 wake=other 未分类(诚实桶,捕获首行无窗内时间线)/R13 pin 措辞随 Stage 2 rider/Stage 2=ELIM-1 ◎ 总览+stanza+rider(C4 %列+新裁定 B/C)。

### §29.58.5 立案(用户 2026-07-13,工件 20260713-133136):组成部分行双图标观感+dedup 行主行三要素缺状态词(并入 SELF-LANE 批)
用户三问:①`↳ ⋈ 自身·binder` 双图标——设计=连接符(结构)+形态记号分工,但两 2ch 信封并排读作双图标;**精化裁定**:组成部分行整体深一级缩进,↳ 落缩进位(从宿主分支义),行上单记号 ⋈(与旁注靠前缀字形区分);②◇ 内用户关注线程=§29.58.3 SELF-LANE 已立案(SELF-SEM 只覆盖确定性语义 span,IO facet 席无链证明依 §23.1 不上链,按 SELF-LANE 归位自身段);③**dedup 折叠行(N次同值)行1 缺状态词**——`主体+2次同值` 铸行1 而状态词落行2,违反 PTV4 主行三要素;修向=行1 补状态词(`主体 · IO等待 2次同值` 形),同区非 dedup 行行1 带状态的对照实锤。①③随 SELF-LANE 批(同域:◇ 自身行重构)一起交付。

### §29.61.2 用户裁定(2026-07-13,扩展 §29.61.1):SELF-ALL——◇ 内目标自身席不止显示问题,应参与链上根因排序
用户裁定原文要义:「◇ 内关注线程」不仅仅是显示问题,它应该参与链上根因排序。**落地形(SELF-SEM 推广)**:①精确门=SubjectIsAnalysisTarget ∧ typed 墙钟区间在窗内(阻塞态族/facet 席全类,不再限确定性语义闭集);②入链上通道=复用 Stage 1 机制(OnChainBasis 家族新 basis 值如 `self_blocking_wait`,零唤醒边宣称,enrich 保持臂同款);③**口径纪律不变**:非墙钟口径(综合评分/计数当量)照旧 ⌗ 口径旁栏不参赛(V2-P0);席位只许墙钟(IOFAM-SELF 分层 roster 照旧);④同线程同段双账(self facet vs 自身段/D 席)沿用既有 W-A/不可相加/互指机械;⑤§23.1 非目标线程零改动(负向 pin 同款)。**连带效应**:自身墙钟席升链后 ◇ 内目标行自然清空(§29.58.3 SELF-LANE 显示归位案大部收敛于此;残余=真无区间/非墙钟自身行的显示归位)。**排期**:RANK-U Stage 2(在飞,勿中途改其输入)落地后立即实施,与 SELF-LANE 显示残件+§29.58.5(双图标缩进/dedup 行三要素)合一批=SELF-ALL 批。witness 验收=133136 形复放:◇ 内 .ugc.aweme 席应转链上参赛佩序数。

### §29.61.3 用户裁定(2026-07-13):◎ 窗内可消除量总览置于投影树之前
用户裁定:总览表放投影树前面,视觉效果更好(先执摘后细节,与五问信息优先级一致)——覆盖 rank_order_v2 原稿「树 fence 后/明细表前」位置。已转发在飞 Stage 2 批(E#/榜位前向指针语义不变,装配序先核;分类器/census/宽度 pin 位置断言同步;md/html 同位;EVOLUTION RECORD 记裁定)。

### §29.65 pin 覆盖审计(用户问「之前问题是否都有测试看护」,2026-07-13;矩阵 scratchpad/pin_coverage_audit_20260713.md)
本窗 ~60 项修复逐条对账:绝大多数 (a) 已修+pin 实名核到 file:line;五类高危抽查健康(显示形 md pin 齐/冷读 prose 病五项 feed pin 全在/完成门行为翻转三向/census 族三层/修真假 pin 在位)。**回归口 7 条立案 PIN-1 批(排 Stage 2 后)**:B1(P2)折叠行头名保护 floor 无直接 pin(golden 间接覆盖,floor 数学+窄宽压力零用例);B2(P2)ENG-1 帽外披露 wire 文案无 pin(删 banner 行仍绿);B3(P3)Reset 三车道收窄无枚举 pin;B4(P3)h3 oracle 账实差勘注(账本称回升硬实为保软+条件恒等);B5(P3)tool Description 无 byte-golden(§29.64 dispatch 敏感面病根形可无声重现);B6 auxfold 旧词形(Stage 2 合入后复核);B7 ARTIFACT-KEEP 无 harness 机械看护(三犯)。**结构性折扣两条**:金样本单趟 FAIL 不可判回归(dispatch 方差,须 clean-HEAD A/B)+eval 不进 make test;DOM 几何一次性验收→机械化候选(记号同列/旁注深 2ch=rune-width 字符串几何断言零新依赖;html=md↔html fence 字节恒等 golden;信封宽扩 full-width sweep)随 PIN-1。

### §29.61.4 用户裁定(2026-07-13):「确定性优化点」卡片表 UX 简化
用户原话要义:卡片表格 UX 不好看,简洁一点,最好不要太多颜色,不要渐变。落地(随 RANK-U Stage 2 件E,同表顺手):渐变全灭;配色收敛单色系+至多一强调色(与因果树 fence 朴素风格对齐);装饰从简结构靠边框/留白/对齐;md 面不动纯 css 层;共享 css 类只收敛自有类勿动他卡。已转发在飞 Stage 2。

## §29.66 MMD-1 收账(2026-07-13;用户报 181931 工件 mermaid 失效;L8 模式系统侧修复)
**病灶实证**(嵌入 mermaid 11.12.0 逐段二分+38 字符阵列):唯一致死点=未引号子图标题内 em-dash U+2014(`subgraph 次因—优先级反转候选`)——裸标题走语句级受限词法流(仅 Unicode 字母+少数 ASCII;em/en dash/×/·/、/：/（）/弯引号/全角数字等全灭),与节点/边标签宽松模式不同;现有 shim 两门(多词/括号类)均漏单 token CJK+em-dash 形,裸节点 ID 同类已有白名单覆盖=子图标题是唯一缺口。**修复**(internal/mermaidcompat/normalize.go +53):新谓词只标记实证不可词法化形→并入 NeedsRepair 走既有引号改写(`subgraph subgraph_N ["标题"]`);被标记者今日必死=引号只救活不碰活图;ASCII/纯字母标题字节不变;**零 LLM 面改动(L8)**;错误框兜底照旧(负向 pin)。**验收**:witness 浏览器实渲染 SVG(11 节点/10 边/4 子图全量,console 零错,em-dash 标题几何非零);历史合法工件 fence 字节全同;29 形单测(15 雷区+14 合法)+HEAD 码红实证;全仓绿。

### §29.61.2a 用户裁定补充(2026-07-13):SELF-ALL 有效归因=与链上行同一阶梯,按状态族最合理折算
用户裁定原文要义:SELF-ALL 的自身席参赛时,running 状态按算力供给最高值折算影响(供给折算),runnable 状态按调度压力全额计影响,等等——按最合理的方式进行影响判断和根因排序。**落地形**:自身席入链后消费与既有链上席**完全相同的有效归因机械**,零特判——running=供给折算(按大核满频/实测频点共动分簇,含供给折算缺口披露,R5d-2/CR-3 既有阶梯);runnable=全额(调度压力);D/IO=墙钟合计(分区/证明机械照旧);确定性语义 span=窗投影并集原值(§29.61.1/R13 携值);非墙钟口径照旧 ⌗ 旁栏不参赛。行2 分解披露(原始→计入(口径))与链上行同形。SELF-ALL 批工单按此执行。

## §29.67 RANK-U Stage 2 收账(2026-07-13;ELIM-1 ◎ 总览+三用户裁定+R1 席位修根;复核 SHIP 零必修+冷读三裁定闭环;RANK-U 全案收官)
**交付**:件C ELIM-1 四臂准入(共享席臂纯抽取 M-E 八 pin 红实证/种群/通道链∪◇ ▒永不/口径值形;排序纯发布 eff 降序明拒 eff×conf);件D ◎ 窗内可消除量总览 typed fence(**用户裁定置投影树前 §29.61.3**;TOP5+◇最大保底+空链诚实行+排除脚注逐数如实+零序数零徽章零求和零%;锚配对普查排除 ◎ 借后随树配对,恒等式 pin;self 席「自身·确定性优化」候选词自动脱落=裁定⑥);件E C4 占窗%列(单值源÷分析窗+SemanticClass 门+多窗禁%)+R13 pin(census 恒等式+借道封口+self 携值合法臂)+新裁定 C 收窄形(▒ 恒 tertiary;完整 context 词试装被 tieba 对照抓获吞面回收,typed token 批候选,主会话追认);**C4 卡片 UX 简化(用户裁定 §29.61.4:单色+3px 左线唯一强调/透明底/零圆角/渐变 affirmative 禁令)**。**R1 席位修根(witness 揭示)**:absorb 丢被吸收侧 rank 席位对→同页「#2 vs 未入前3」矛盾+❷ 蒸发;修=席位对回填+SemanticSpans 领席统一,冷读四面一致确认(矛盾句族双清)。**收尾轮**:◎ 口径注记扩折算形(冷读活实例 3.175 折算席无口径词=值面承诺缺口;转录制五臂共享 composer,行3 字节恒等零新词)+M-A 注记如实分派+INDEX 渐变句两探针并陈勘正。
**三关**:复核 SHIP 零必修(F1/F2 措辞收尾轮落,F3 席位 tier 撕裂窗=产线不可达观察,F4 零序数 pin 词形域观察);冷读=三用户裁定+修根+witness 全闭环(◎ 区零徽章独立复核/排除脚注逐数/%复算/tieba 恰 4 意图 delta)。金样本按 binary 分列诚实(h2 0/2=批前 dispatch 家族,Description 源级恒等直证;h5=EVALFIX oracle 词边界实锤)。
**RANK-U 全案收官**:ELIM-1(GREENLIT→落地)+SELF-SEM(§29.61.1)+WAKE-CENSUS-D(§29.58.4 2A)三案齐;双客户 witness=VerifyClass 代理形(◎ 第 2 行+主榜 ❷ 并存)+唤醒者题 prose 全绿。**遗留**:新裁定 C 完整 context 词形(typed token 批候选,收窄形已追认)/◎ 退化窗注记 D-4 单源偏离记录/h2 dispatch 无关化裁定池照旧/launch.json ranku_s2 静态服务器条目保留(DOM 复核复用)。

## §29.68 PIN-1 收账(2026-07-13;§29.65 七回归口+DOM 机械化三件;纯看护批零行为改动;突变红实录逐件)
B1 折叠行头名 floor 直接 pin(数学六形+产线实铸压力形:同主体兄弟行中截而折叠行头名+榜位指针整体存活=压力真实性双见证);B2 帽外披露 wire 文案 verbatim pin(删行红);B3 Reset 车道闭集 pin(置位点恰 4+全仓恰 1 调用点+latch 守卫;第 5 软车道置位即红);B4 h3 账实差勘注(§29.55.2「回升硬」过铸勘正=保软+条件恒等,判定逻辑零改);**B5 tool Description byte-golden**(23,918B 产线实铸+UPDATE RITUAL 显式门;复现 §29.64 病根形 byte 2164 咬红,既有子串 pin 全盲=判别力实证);B6 auxfold 存档纪元勘注(标本字节不动,live 消费 grep 零);**B7 ARTIFACT-KEEP harness 修根**(eval runner cp -pn append-only 归档制先于每 case 起跑+Go 侧接线 pin 入 go test 面——三犯病根关账)。**DOM 机械化 3/3**(零新依赖,挂投影测试族):①折叠行记号列与兄弟 rune-width 恒等 sweep(zh 边词闭集恒 2 CJK 不变量);②旁注深恰 2ch 全 fence sweep;③md↔html `<pre>` textContent 逐字节恒等(P2a 形六 fence)——一次性 DOM 验收升级为常驻回归看护。残留:B5 golden 仅 trace_query(按证据扩)/EN 边词宽度不齐=设计现状/归档目录 append-only 手动清。

### §29.58.5a 用户裁定(2026-07-13,工件 225901 复问):组成部分行 ↳ 退役——深缩进已承载结构,冗余双编码
用户问 ☾ sleep 与 ↳ ⋈ binder 是否上下级、为何多一个 ↳。答:是(binder=E1 组成部分,词面明示);↳ 系归位时的从属连接符,深缩进(§29.58.5 用户裁定)落地后同一关系被双编码。**裁定(默认最优)**:↳ 从组成部分行退役,终形=深缩进+单记号 ⋈+「组成部分·不可相加」词面(结构管关系/记号管形态/词面管语义);与旁注行(· 前缀)区分保持;优化点表 ↳ cell 系另一语义不动;lockstep 全面回收(tracefence/图例/mark/census/宽度 pin/PIN-1 DOM sweep);记号演进史=◦→↳⋈→深缩进+⋈(EVOLUTION RECORD)。已并入在飞 SELF-ALL 批。

### §29.58.5b 用户终裁(2026-07-13,推翻 §29.58.5a):组成部分行 ↳ 保留
用户终裁:双编码 ↳ 保留——组成部分行终形=**深缩进+↳+⋈**(结构双编码为刻意设计,显式从属箭头与缩进并存)。§29.58.5a 退役裁定作废;在飞 SELF-ALL 批已撤销该指令(225901 工件形即终形)。记号演进终史:◦→↳⋈→深缩进+↳⋈(终形)。

## §29.69 SELF-ALL 批收账(2026-07-14;用户双裁定 §29.61.2/.2a+SELF-LANE §29.58.3+§29.58.5 全链;三关+修复轮六件)
**交付**:件① 自身墙钟席全类入链(registry 门=墙钟族∧非聚合∧CaliberSideClass==None∧Lane 排除,37 token 全表复核零新手抄表;basis=self_wall_clock_interval+causality=self_wall_clock;一谓词三消费;有效归因零特判=与链上同阶梯,恒等式 pin ×1.15 突变红;fold 键 basis 维禁混折);件② SELF-LANE 归位(◇ 只剩他线程;非墙钟 ⌗ 行佩「非链」;跨通道互指句双向入关系句族);件③ 组成部分行深缩进(终形=深缩进+↳+⋈,§29.58.5b 用户终裁)+dedup 行1 状态词。**连带修根**:⌗ 口径行 fold 豁免(h3 flake 真凶,h3 修后 2/2)。
**三关**:冷读=双裁定闭环 ✓(133136 六席形逐 µs 全等/阶梯复算 3.264 恒等/◎ 随动含上轮口径词建议落地);复核 SHIP-WITH-FIXES——引擎半场全核真(AS4 分区×self 补形 PASS 入正式 pin),显示半场两 P1 同族(升链人口+1 撞带帽折叠/普查):F1 佩序席被 compile 折叠吞没(#7 隐身序数出洞)→豁免扩 Rank>0 全席+IOFold 永不吸收持席行;F2 显示折叠缺 basis 维(不相交席折进「同段IO另有」=词面假,收编冷读残留2+SA-F1 共同病根)→显示折叠键补 basis 维分行+C1 关系句族自动双向互指;F3 ⌗ 计数值出普查(81.616 裸 ms+通道词失实)→双 typed 门;F4 互指两环同谓词;F5 零特判恒等式 pin。**修后 A/B 终验**:全席 1..12 可见+序数连续+#7 复见+「同段」词面消+h3 真机 PASS。
**勘正与披露**:F6 tieba 验收句勘正=引擎面字节全同,显示面恰 5 行意图 delta(件②b 互指句族在非目标双通道线程如实点火=裁定意图非回归);F7 Limit-12 帽尾位移(升链人口+1 使 binder 供给席 eff 0.933 跌出候选帽=合法位移非静默丢失,截断 caveat 如实计数)。
**新立案 SA-F2(VSync 发生器证据面,新题族)**:tieba VSync 题模型答回调消费方+124.14ms 回调间距;raw 内 VSyncGenerator-1682 有 43 处事件+权威周期 print(GenerateVsyncCount:1, period:16552213≈16.55ms)——发生器族证据未入模型面;修向候选=VSync/帧节拍发生器 census(EVID 族先例)。**credits 中断插曲**:双关 agent 中断,冷读死前报告已全,复核续命后带三探针完工,树零污染(守护巡检核销)。

### §29.61.4a 用户裁定(2026-07-14):优化点表成员行「成员」词退役+首列去加粗
用户裁定:「↳ 成员」的「成员」二字冗余(↳ 已表从属),其后内容不加粗。落地:member cell=`↳ <成员名>`(语言中立,zh/en 分叉消);rcm2 pin 随改;HTML `section.trace-action-optimization td:first-child` 加粗退役(th 保留);EVOLUTION RECORD 记裁定。主会话直接落地(三行级,全仓绿)。

### §29.61.4b 事故记录(2026-07-14,主会话自查):红树推送 15 分钟窗
§29.61.4a 落地时复合命令 `go test;git commit&&push` 的 `;` 未拦 test exit=1,红树 commit 89247838 被推出——红源=删「成员」词使 SEM-LEAD ④ 的 roster 行豁免键(靠「成员」字面识别)失锁,pin 按设计咬红但推送未被挡。15 分钟内修复(豁免键改从属连接符 2dd9ebd1)。**教训**:①词面 pin 的豁免键靠字面词识别时,删词类改动必先 grep 该词全部 pin 消费点(词面族 lockstep 纪律漏了 test 侧豁免键);②推送链的 test 门必须用独立步骤硬拦(先 test 取 exit 再单独 commit,禁复合 `;`)。

### §29.61.4c 用户裁定(2026-07-14):优化点卡左侧竖线换墨绿
用户问绿色竖线换黑或墨绿孰优。裁定=墨绿(黑丢「优化」语义残留且暗主题不可见):light `--action-border` #86efac→#14532d(两处含 print 块),dark #22c55e→#3a5f4b(柔和暗绿保可见);树内 action token(fg/bg)不受影响。注:落地时全仓扫撞上 SUPP-CORE 在飞半写态一次瞬态 build 红(归因排除,域零依赖),按并行批纪律以域内绿+build 净提交。

### §29.61.4d 事故补记(2026-07-14):推送门第二形态失效——管道吞退出码
§29.61.4c 落地时 `go test|tail` 后取 `$?`=tail 的 0,域内红(SUPP-CORE WIP 瞬态)未拦。事后 HEAD 快照严格验证 tool+preview 全绿=推送内容无恙(commit 只含 preview css+docs,红全在未提交 WIP)。**推送门终形纪律**:test 命令禁管道,`go test … > log 2>&1; EXIT=$?` 独立取码;并行批在飞时全仓门以 HEAD 快照(git archive)跑,不以活树跑。

## §29.70 EVAL-HYG 批收账(2026-07-14;F4 密封 stub 仓+EVALFIX h5 词边界;eval 域零产线码)
**件① F4 修根**:eval/fixtures/stub_repo/(中性 README+空 docs)+22 金样本 trace-only case 密封(FIXTURE 指向;逐 case 判定:e2/g2 混合形保留=repo 即案件主体);密封 PASS 趟日志普查=零仓内容读取(全部 read/grep/exec 目标 ∈ .codrax/blob;witness 串在自仓源码/case/PROFILE 逐字在而 stub scratch grep=0=通道关死)。**反证红利**:未密封对照趟模型整窗游走仓内一次 trace_query 未派=自宿主分心形——密封兼提质。h2 六败诚实归因 dispatch 族(两 PASS 趟携 wakeup_chain/rank,FAIL 趟全无;§29.56 在案,非本批)。域外同形 11 case 列册待裁(路径解析考点须留仓 8+donghu_real 族 3 随下批)。
**件② h5 词边界**:runner 层数字边界守卫(尾数字→`([^0-9]|$)` 前后文界定,×多字节 CJK 语境 \b 无效;ERE 转义;hex 短哈希雕除防身份前缀假 miss——u7g 存档实锤);四字面通道全走 helper,case oracle 零改(禁降 bar);19 断言(×3 三态真出现咬红/×39 不误咬/132.041 族/4次( 门);存档 A/B+80 token 全扫零翻转。**附加修根**:run.sh 同秒 OUTDIR 碰撞(原子 mkdir 认领+.N 后缀+e2e pin;3 污染目录标记+隔离重跑替证)。
**纪律**:全仓绿 witness(00:44);其后复跑红=SUPP-CORE 并发在飞(含其 orchestrator.go 触自身 LOC ratchet 9403>9395 的中间态信号,移交其批)。遗留:16 非 h 密封 case 随例行 sweep 覆盖;`7s`⊂`3.7s` 小数点邻接类=非数字族授权面,无实锤不动。

### §29.61.5 用户裁定(2026-07-14):补采双预算默认放宽
用户裁定:补采看护默认值翻倍+——`trace_supplement_max_duration` 5s→**20s**;窗跨度门 30s→**120s**。语义/披露/fail-open 纪律不变,仅默认值(codrax.yaml 可覆盖);随 SUPP-CORE 修复轮落。

### §29.61.6 立案(用户 2026-07-14):「未归因」图例补认识论定位句(词面批)
用户问「未归因」三读(不需要深挖/无需深挖正常/没有深挖未知)何义。核清:=树头覆盖句算术余量(等待时长−第一层已发布原因行覆盖),语义=**没有被归因(未知待查)**——已识别正常空闲(∿ 节拍)会被单独拆行,自身 running 残余禁叫未归因(在案裁定,tree.go:11746)。图例既有条目讲清了算法但未讲认识论地位——用户三选一困惑即 witness。**修向(词面批)**:图例条目补一句「未归因≠正常/无需解释:是尚未被已发布原因覆盖的部分(可能含未发现原因/未探查窗/未识别空闲,系统不判定),已识别的正常空闲另行单列」;zh-en 双语。

## §29.71 SUPP-CORE 收账(2026-07-14;DISPATCH-IND 批1+P1 双保险丝+修复轮六件;三关全过;dispatch 病族转折点)
**交付**:explore 完成被接受后、extract 前单点确定性补采——布尔 family 探测(与渲染同一 compiled ledger)→typed 参数推导(目标 user-source 恰一先+唯一正 pid 归一;窗口三档梯锚族→scoped-stats→全量,±1ms 共享容差永不 last-wins)→≤2 次直调 Execute(单一值源)→专用系统车道逐记录 SystemSupplement typed 出处→单行披露(三形:执行/超时部分跳过/超窗跳过);任一 typed 缺失=fail-open 跳过输出字节同 HEAD。**§29.60 完成门零重开/零 requeue/零 Reset(复核源码直证:零见证臂结构性不可被补采洗白);B5 byte-golden 零触碰;L1 直跑绿。P1 双保险丝(用户裁定默认 20s/120s)**:视图间 deadline(已完成 view 保留+尾注)+派生窗跨度门(整体跳过+缩窗指引,禁静默截窗禁猜)。产线脆性双修根(scoped-stats 中间档/pid 归一,013012 对照趟定谳)。
**三关**:复核 SHIP-WITH-FIXES(§29.60 相容首趟无懈可击;突变 6/6;树中途动用快照法);冷读=批1 达成(h2 纯 es 形铸满树逐值 raw 恒等/补采窗=用户窗/no-op 零披露/值面零回归)。修复轮六件:回探清槽单点门(五消费面归位+checkpoint pin)/reject 不计不披露(假出处宣称关死)/blob ref 注册/cursor 回写抑制双 recorder 门/审计面 origin=system_supplement 纯渲染 token(R2' 免)/SC-F2 双 tid 已修。**金样本:h2 历史 0/8 族→3/3 PASS(含案文自档 legitimately-fail 纯 event_search 形铸满树);h4 首趟 PASS;对照零回归,warm 亚秒级。**
**账面**:execution_failed 静默形+blocked-DAG 跳过形=不劣于 HEAD(记录);**立案 SUPP-CANCEL**(视图内取消=tracequery.Run ctx 线程化+协作取消采样点+typed canceled/partial+DET 义务,独立引擎批)。**遗留**:批2 SUPP-ORACLE(legitimately-fail 条款删除/h3 恒铸升硬/F6 满帽 E# pin)→批3 SUPP-FEED→批4 SA-F2+C-lite。

## §29.72 SUPP-ORACLE 收账(2026-07-14;DISPATCH-IND 批2;金样本 oracle 升硬;复放 5/5)
**件① h2 条款销亡**:「legitimately fail」dispatch 合法失败条款删除(生于 §29.55.5 件B,亡于 §29.71 SUPP-CORE;witness=014123 3/3 含纯 es 形+024052);机械 oracle 本已满硬零降杠;`(对端未解析)` 可选臂保留=两种引擎真形并集非软化;**金样本判读规矩更新入 case RECORD:自批2起 dispatch 形 FAIL=回归**(clean-HEAD A/B 归因臂对其他原因保留)。复放 2/2(模型自派臂+补采臂各一)。
**件② h3 恒铸升硬(先验后升)**:病根澄清=io_latency 成员住 critical 族,pre-SUPP 缺词=critical 缺席;补采使 rank+critical 双族恒达→恒铸双条件恒真;pre-harden 2/2 验证→EXPECT_CONTAINS += `完成端到端·IO延迟（io_latency）`(typed 组合形交付杠)→**post-harden 1/1=补采臂实趟过硬杠**(模型零 rank 派发,补采 1.007s 双 view,最强确认形);soft→hard 全史 EVOLUTION RECORD(§29.55④→DET-1→§29.68 B4→§29.71→批2)。
**件③ F6 满帽 E# pin**:全链引擎实铸(128 帽精确灌满+12 溢出裁+CapacityTruncated 点火+补采追加存活)四断言(序数连续/全表面引用可达/跨面同号同主体/双 compile byte 恒等)+h3 补采臂引擎 pin;突变 3/3 红(M3 首次假突变 grep 实证后重做=纪律在案)。混批树全程 clean-HEAD 快照二进制复放。

## §29.73 WF-2 词面批收账(2026-07-14;用户提前令;四件;复放产线直出终稿形)
**件①** 未归因图例认识论句(§29.61.6):zh/en 三要素句落图例(非正常义/可能构成/已识别另列;EN 初稿被 glossary lint 咬红改「no verdict is made」=内部词黑名单在岗);**件②** 补采披露终稿:「装配期」→「成文前」(内部词判定)/zh 面 view 双形「根因排序（root_cause_rank）」走 tracefence 单点(手抄字面被 UXG-1 F2 tripwire 首跑咬红=五表单源纪律在岗)/「窗长预算 vs 时长预算」刻意保留区分词;复放趟补采真实触发=终稿形产线直出+7 条 origin 记号;**件③** 计数当量 ms 后缀全域退役(§29.55 观察③ 一裁):统一「计数当量X(非墙钟)」,全铸造点 lockstep(喂入/roster/树行1/明细 note/◎ 脚注/图例)+新负向 sweep pin(`计数当量[0-9.]+ms` 出厂即红)+消费方 EN 连字符形随改;存档纪元标本不触;**件④** zh-en 抽审零不一致+一修(证据索引审计教学句补 origin 条目)。留裁定候选:综合评分类值带 ms(不在本裁射程)/G10-EN 照旧。全仓绿;donghu/tieba 复放 PASS md↔html 同源。

## §29.74 SUPP-FEED 收账(2026-07-14;DISPATCH-IND 批3;复放 4/4;喂入侧零缺口)
复放矩阵 4/4:R1 h2 病形=榜席/等待对象/窗全量 census 全为补采所得且只入 finalize prompt(explore 零占位=R1 裁定端到端);R2 补采榜行与 R1 逐值恒等(确定性佐证)+补采完成后 7ms finalizer 发车(免费受益直证);R3 唤醒者题眼全绿(gpu-token 唯一,§29.62 倒置零复发);R4 全家族自派=skip 3ms 零调用+喂入面基线恒等+零披露(阴性对照)。**喂入侧行为缺口零**(批1 接线完备);交付=3 条 E2E pin(builder 三接线点,突变 3/3 红)。**PROSE-RC 残余续档**:Σ 自加 41.157/TOTAL 未消费自数/**余数改道绑定复发新形**(1.899 绑 fscache+铸派生未证量 8.534=10.433−1.899——姊妹句升显式 member 禁令=PROSE-RC 续批素材)。

### §29.61.7 立案(用户/客户反馈 2026-07-14):树头 lead 段 E#/徽章引用 UX(UX-ANCHOR 批)
客户反馈:「Trace 因果投影」标题下、分析窗前后的 lead 段(主根因/四态账/running 分解/已归因未归因/链上行数句)中 `❷[E28]`/`E4(+1)` 类引用:①无内部锚点链接(树 fence 内 E# 有锚,lead 段在 fence 外未覆盖);②文字内不突出;③❷ 徽章沿用树内 pill 样式在正文中偏大。**处置(UX-ANCHOR 批)**:a) lead 段 E# 引用接入既有锚配对机械(同款 fail-closed:计数恒等破则降纯文本,禁悬空链接);b) 正文语境徽章样式分形(树内保 pill,正文用紧凑形——缩字号/去背景或轻背景,DOM 量测核不挤行高);c) 链接样式使引用在文字中可辨(链接色);md 面零字节变化(纯 html 渲染层);pin=锚可达+计数恒等+样式 verbatim+md 零变。

### §29.61.8 用户裁定(2026-07-14):「问题」节客户原稿 text 块 verbatim 呈现
用户裁定:报告「问题」节(客户输入回显)禁一切渲染,text 块最忠实。落地(并入 UX-ANCHOR 批件d):md=```text 围栏字节 verbatim(内容感知围栏长度防 ``` 碰撞);html 自然 <pre> 转义(注入面顺带关死,注入形 pin);逐字节恒等 pin(控制符混合 fixture);fence 分类器负向核。

### §29.61.9 用户裁定(2026-07-14):HTML 正文字体升级——Sarasa UI SC 前置+表格等宽数字
用户问正文默认字体推荐(卡片好看=mono 栈首选 Sarasa Mono SC)。裁定=正文栈前置 **Sarasa UI SC**(与卡片同设计族,装了更纱的环境观感统一;未装环境按原系统栈优雅回退,零外部字体自包含安全)+`td,th font-variant-numeric: tabular-nums`(数值列对齐)。letter-spacing 零触碰(mermaid 量宽教训)。

### §29.61.10 立案(用户 2026-07-14):链上席有效归因的因果边界排查(ATTR-BOUND)
用户指出:链上 runnable「调度压力候选」席可能把**无唤醒关系依赖的片段**也计入——多片 runnable 中仅前段直接/间接阻塞关注线程,后段虽 runnable 但无唤醒关系,全窗合计=误放大,污染根因排序与 ◎ 可消除量两面;要求排查其它状态是否同病,并设计合理计算。**病理框架**:时间窗重叠是因果贡献的嘈声代理;调度压力的因果语义=该线程 runnable 竞争 CPU 延迟了关注线程的唤醒→运行转换,故 runnable 片段仅在(a)链入唤醒边(直接/间接)或(b)与关注线程等待窗相交时可归因。**排查范围**:全状态族链上席值的计算口径普查(runnable/running/D-IO/sleep 各 lane:全窗合计?链窗交集?唤醒边锚定?)+witness 取证(donghu/tieba 找多片 runnable 链席含无唤醒关系片段的实例,量化多算量)+合理计算方案(先诊断后裁定)。只读排查先行,RSPA 批随裁定。

### §29.61.10a 用户裁定(2026-07-14):ATTR-BOUND 方案定向——链上席唤醒边锚定+调度压力语义降邻近
用户裁定:①**方案 A(唤醒边锚定,最严)为链上席合理口径**——链上 runnable 席值仅计直接/间接链入唤醒关系的片段;②**「调度压力语义」应入 ◇ 邻近区段而非链上**——纯时间重叠的 CPU 竞争不是严格链上因果(§23.1 推广:同线程身份非因果→时间重叠竞争亦非链上因果,是邻近级证据)。**落地含义**(排查按此收束方案):链上 runnable 席=唤醒边锚定段合计;无唤醒边的调度压力片段/线程→邻近通道席(调度压力候选词面随迁);ELIM-1 总览收链∪◇ 两通道=可消除量可见性不损,仅因果等级如实;其它状态族同框架核(D-IO 阻塞席的唤醒边/blocked_reason 锚定?running 供给折算的锚定基?)。实施=RSPA 批,排查回报边界与爆炸半径后开工。

## §29.75 批4 收账(2026-07-14;SA-F2 VSync 发生器 census+C-lite;双关+修复轮五件;SA-F2 题族收官)
**件①** vsync_generator_census(判据=线程名闭集∪GenerateVsyncCount 格式族,两 trace 普查先行;双口径 window_population/matched_rows;DET byte-identical;R2' 七处含 tracediag 双 hash+Description 刻意豁免恰合 §29.64);**产线修根**=tool 无目标 event_search 自动注入 runtime target 滤掉发生器行→census 准入剥离 pid/thread 臂(发生器按定义活在被查进程外;复核核实=剥离只放宽扫描集,准入仍逐行精确身份)。**件②** C-lite 无窗轻量预铸(45-58ms 单遍;冷预算同门;禁 claim-of-absence=零命中静默)。**修复轮五件**:indexed Run 接线 pin(F1 突变实锤零覆盖)/有窗趟 C-lite 武装(冷读残差:census family 缺席即武装,presence-oracle 掩蔽案例=「Eval PASS≠绿」再实例)/refresh 首见 wins/「卡顿」移出 exact 集(通用词误触发)/banner 前置序 pin+caliber 注。**三关**:复核 SHIP-WITH-FIXES(突变 3+2 全中);冷读=SA-F2 达交付水位(发生器+权威周期 16.55ms raw 全等,124.14 误读消退,修根双层对照);witness 复放模型 think 具名引用 census。**双立案销账**:§29.58 census dispatch 依赖续档+§29.62 浅展开口径观察——C-lite+census-absent 武装臂关闭。**新立案**:P3-4 skip-reason 裸字符串族 typed 闭集化(三面手抄,NKR 同构修向)/P3-5 C-lite 无墙钟 deadline(并 SUPP-CANCEL)/decoder-remap hint(模型臆造 ignore_case 参数被 strict-decode 拒)。

## §29.76 UX-ANCHOR 收账(2026-07-14;§29.61.7/.8 客户反馈四件;md 零字节纯 HTML 层)
**件a** lead 段 E# 锚点:新 markdown_trace_lead.go 挂锚配对 transformer 尾;lead 作用域=精确信号(投影 fence 向前回溯,H2∧SectionProjectionTitles 闭集终止;◎/aux 跳过);语法=fence 侧 token 复用+括号裸形,仅 claimed 序数铸链;**Text 节点 run 扫描**(goldmark 拆节点漏 token 已修);fail-closed=计数恒等破整体降纯文本。**顺带修根在产 bug**:`❹[E1](折算…)` 被 markdown 误解析假链接+口径括注吞进 href——按精确语法重建还原(真链接零触碰负向 pin)。**件b** 正文徽章紧凑形(0.85em/去粗/轻背景;树 pill 零改哨兵;DOM 证行高零撑=含徽章 li 恰 2×行盒与对照同值)。**件c** 链接可辨(--link+点划线 hover 实线)。**件d** 问题节 verbatim(§29.61.8):```text codrax-user-request 围栏+内容感知围栏长度防碰撞;HTML pre 转义=注入面关死(script 注入 pin);逐字节恒等 pin(控制符混合);typed 第二 token 关 mermaid 嗅探+分类器收紧(档案车道正负 pin)。全仓绿×2(工作树+HEAD 快照门);md 零字节 cmp 实证;真工件 47 锚零悬空点击实测。注:与批4 共享 answer_document_mutation_runtime.go(hunk 零交叠:披露函数族 vs title 发射点),合并收账双节分记。

### §29.61.10b 用户裁定(2026-07-14,推广 §29.61.10a 至全族):唤醒边=链上因果唯一凭证,邻近级证据责任归位——D/IO/running/span/binder 全状态族同原则排查
用户裁定原文要义:按同样语义思路排查其它状态(D/IO/running/span/binder 等),同一原则处理——「唤醒边=链上因果的唯一凭证」「邻近级证据责任归位」。**执行框架**:①每族链上席审定其**因果凭证类型**(唤醒边/binder 同步事务边/self-causality/blocked_reason 等待对象边——typed 有向依赖记录候选与唤醒边同级性逐一列举供用户终审;纯时间重叠恒为邻近级);②席值锚定=凭证关联片段合计(非全窗/非重叠窗);③无凭证片段/线程→◇ 邻近(词面随迁,ELIM-1 链∪◇ 收口=可见性不损);④每族出处置矩阵(凭证类型/现值口径/缺口/迁移形/爆炸半径)。ATTR-BOUND 排查扩为全族 ATTR-UNIFY;实施=RSPA 批(按矩阵与用户终审分件)。

### §29.61.10c 用户裁定(2026-07-14,凭证清单终审):typed 因果边有直接影响链上的边依赖证据者,统一入链上排序
用户裁定原文要义:内核记录的 typed 因果边,如有边的依赖证据能证明直接影响链上,统一到链上排序。**凭证判据定谳=证据面非类别面**:链上凭证=内核记录 typed 有向依赖边 ∧ 该边携带指向链的直接依赖证明(唤醒边天然满足;binder 同步事务边/blocked_reason 等待对象边等逐边核直接性,非按类别整族放行);纯时间重叠恒为邻近级。ATTR-UNIFY 矩阵按此判据分类每族边;RSPA 批实施。

### §29.61.8a 用户裁定(2026-07-14):问题 text 块自动换行防框边遮盖
用户裁定:html 问题 text 块加自动换行宽度处理,避免被框边遮盖。落地(主会话直落):tracefence 单源 `UserRequestInfoToken`(outputdump minter 归源+preview 消费);preview 渲染臂 `pre.user-request`(typed token 精确判,禁嗅探)+CSS `white-space: pre-wrap; overflow-wrap: anywhere`(字节 verbatim 不变,纯显示换行);**网格 fence 零波及**(投影树/◎ 保 no-wrap,负向 pin);既有两 pin 按 EVOLUTION RECORD 演进(转义性质不变仅类名)。

### §29.61.11 立案(用户 2026-07-14,工件 090607):优先级反转席的供给缺口成分禁压缩(INV-SUPPLY 批)
用户裁定:优先级反转问题中若部分成因(或窗内可消除部分)来自频点没跑满,答案必须提及,不能单压缩为「优先级反转」。witness=❶ CompThread 席:有效归因 7.081=runnable 全额 0.109+running 折算 6.972,而供给折算缺口 7.296 下界为主+热限压 1.53GHz——行3 真相已全,行2 类型词单形+prose 顺词压缩=丢频点成分。**设计(三层,零 NL 判定)**:①词面——typed 主导判据(缺口折算量/有效归因≥阈值,纯 typed 比较)→行2 复合词「优先级反转候选·供给缺口主导」(「·可运行等待」更名+tri-form 先例),◎ 转录同词;②喂入——席位构成 named-fact(反转全额 X+折算 Y(缺口 Z 下界,热限压 fmax) verbatim 值链,PROSE-RC 引用式);③ELIM 面——可消除构成分杠杆(调度修复 vs 频点/热策略)。阈值=精确信号纪律定值(实现时按双 trace witness 标定);排 RSPA 收账后(display 域让路)。

### §29.61.11a 用户裁定补充(2026-07-14):INV-SUPPLY 喂入层=必需(让模型看到构成)
用户确认:需要让模型看到。机制定形:行3 分解是显示面(模型写 prose 后才渲染,模型从未见)——病根与「等待对象四跑四答案」同构,解法同一=**席位构成 named-fact 进证据面**(CR-1 双阶段喂入):`席位构成(❶ 优先级反转候选): 反转等待(全额)Xms + running 折算 Yms(供给缺口 Zms 下界为主,热限压 fmax)——两因并提,引用勿推导`(EVID-1/PROSE-RC 引用式先例)。三层批(词面复合形+喂入+ELIM 分杠杆)=INV-SUPPLY,排 RSPA(display 域)与 PROSE-RC 续批(context 域)双收账后。

## §29.78 PROSE-RC 续批收账(2026-07-14;member 禁令姊妹句;三改道形 2/2 全零)
余数具名事实行第三姊妹句(zh/en:成员段=不可拆分未证整体,禁单段重绑已证 cause/禁份额−成员段/成员互减铸新未证量;§29.53.2 账目性质陈述+CR-1 祈使,ATOMIC 过);双语 verbatim pin+member-less 负向 pin+突变 2/2 红。**tieba 复放 2 趟三形全零**(段重绑/整席搬名/成员减法派生;R1 正文逐字消费新句;R4 的 1.899 归 fscache 叙事消失)。**复放保真实录**:R2 首发误用外部在飞脏二进制→击杀作废,git archive+本批两文件独立取码纯净重建重跑(混批复放纪律范本)。**第四改道向立案(PROSE-RC-4)**:嵌套-再减(hmfs 份额嵌进 10.433 席内再减出 10.117 新未证量+自算和 0.296 错;无显式等式绕过算术臂)——修向候选=余数事实行补「caller 份额在本值之外」词形+算术臂扩隐式加和形。席级错减附录块复现(§29.57 2.731 族)续档。**流程教训**:复放看守机制三次失灵(同 agent 两次)——守护巡检直读 verdict/工件文件为准,看守只作加速非依赖。

### §29.61.10d 三连裁定收官注(2026-07-14):ATTR-UNIFY→RSPA 全案落地
§29.61.10a(链上席唤醒边锚定+调度压力降邻近)/10b(全族 typed 因果边唯一凭证)/10c(证据面判据:有直接链依赖证据的内核 typed 边统一入链上排序)——经 ATTR-UNIFY 全族矩阵(witness:donghu ❶ 36.757 中 90.2% 无凭证)与 RSPA 实施批(§29.77)全案落地。因果树语义自此:链上排序只说有边证据的话;◇ 承载诚实余段;「同源二分:全窗=锚定+余段」为唯一可加关系形,全窗账永远可还原。

## §29.77 RSPA 收账(2026-07-14;三连裁定实施;战役最重批;从严双关+修复轮七件)
**引擎**:锚窗集(链 depth>0 节点窗 per-pid 并集,target 自因豁免);四+一精确门(锚基∧census∧anchored≤full∧µs 恒等臂 0.001ms∧producerDisjoint,失合 fail-open 保旧——keva witness 诚实双席);迁移通证两 pass 幂等(case A 窗席→◇/case B 剪切+克隆,锚定值零静默消失);卫星行随迁;M-IO 完成闭合凭证;runnable 帽基→census 限链成员(背景 top-8+溢出披露,census 迁移被冷读定谳=31.191 raw 真值);M-D 分区席持 ⛓+链席 mint 抑制;◇ 余段 side-row 车道;recon 锚定分解吸收臂;R2' 七处(4 wire+3+1 note key+双 schema hash)。**显示**:同源二分行2 双形/关系句 class(0) 可加还原形(唯一可加对)+class(0b) 镜像句(冷读给定句形:同段镜像·全窗账=[⛓]+[◇] 二分席之和,不可与二分席相加)/图例双条/成员(全窗账)/◇「无链上凭证」。**witness 补铸揪出三缺陷修根**:D1 ◇ 余段被 dedupe 同键吞并(键叉 typed 记号)/D2 same-fact 键 ChainAnchorFullMS 参键+survivor 交换(修复轮补 E# 记账)/D3 成员词面。
**从严双关**:冷读=引擎账面 9/9 恒等式精确+tieba 四席全等+Δ0.716 定谳(头部覆盖约定差,宁漏勿猜,恒等式对称)+显示 witness 复审三连裁定达形 ✓(冷读坚持显示 witness=D1 旗舰 bug 被揪的直接原因);复核 SHIP-WITH-FIXES(突变 6/7+假 pin 3/3 真;h6 旗舰形:◎ ◇33.159 居首+模型消费完美「间接阻塞 3.598ms」零复加零复辟,❶ 让位实现;h2 2/2 自身席零扰动 pin)。修复轮七件全落(镜像句/交换记账/donor census/caveat 限定/卫星与 D1-D3 pin/压席门 producerDisjoint 端到端)。
**立案六件**:①D2 交换形合成 fixture pin ②Run 双 sweep 性能(anchored memo 反哺)③io facet 部分重叠收窄(10c 后续)④两 pass byte 幂等 pin ⑤board sweep 放行臂 typed 化 ⑥side-row caveat 三类列举。**流程教训**:复放看守三失灵→守护直读 verdict 文件为准;显示批验收必须含当前树全链 witness(引擎 tally ≠ 显示验收)。

### §29.61.12 用户裁定(2026-07-14):◎ 总览两 UX——记号词距+链上整块前置
用户裁定:①「◇邻近」「⛓链上」记号与文字太挤——glyph 与通道词间加空格(`⛓ 链上`/`◇ 邻近`,md 权威面改,lockstep 词面族/census/宽度 pin);②链上优先级更高,**⛓ 整块排列在前、◇ 邻近块在后,块内各自值降序**——语义更贴切(RSPA 后 ◇ 余段可数值压 ⛓,纯值降序会让无凭证余段视觉盖过有凭证因果)。连带:表头「纯值降序」承诺句随改(如「⛓链上块先·块内值降序」);满格尺保持全区最大值(链上条短=诚实);◇最大保底逻辑随分块自然满足;ELIM pin 族/图例随改。并入在飞 INV-SUPPLY 批(同 elim.go 域)为件④。

## §29.79 INV-SUPPLY 批收账(2026-07-14;用户三裁定 §29.61.11/.11a/.12;三层+件④+收尾五件;双关全过)
**件①** 复合词:共享单源 `types.TraceSupplyGapDominant`(阈值 0.50 专属常量+双 trace 标定注:donghu 三席 64/72/103% 过线,缺口=下界可超归因;边界二进制精确 pin)→行2「优先级反转候选·供给缺口主导」+◎ 转录同字节+图例阈值句常量插值;tracefence 表③b 归源三表 lockstep。**件②** 席位构成 feed(CR-1 双阶段;verbatim 值链;热限压 witnessed 分叉词;帽4+具名溢出 pin;**反压缩双语导语**——复放 run-1 实锤 fact 达 finalize 仍被压缩→强化后 2/2 逐字引用;判据镜像不对称=ACCEPTED 注,双失败方向诚实)。**件③** ◎ 杠杆注「可消除构成: 调度修复 X+频点/热策略 Y」(行3 同一 balance-gated 构建器,拒渲不造数,零Σ零=)。**件④(§29.61.12)** ◎ 词距 `⛓ 链上`/`◇ 邻近`+链上整块前置(channel 主键+块内 eff 降序;表头承诺句随改+chainless/空板诚实分叉双向 pin;满格尺=全区最大,链条短=诚实)。
**双关**:冷读=三裁定闭环 ✓(两因并提 2/2 四值全对/复合词三席零漂移/杠杆注「知道拧哪个把手」/RSPA 残留③ 镜像句顺带确认落地);复核 SHIP-WITH-FIXES 七攻击面零错误行为(普查结论引擎侧独立证实:「类型词非供给族∧带 fold」第三族结构不可达,suffix 臂=恰反转族;tieba 纯 running 供给席 100% 恒等=同族赘语不戴)。收尾五件全 pin 级落(lint 域扩 ../context)。
**立案**:INV-P3-2 同席再发布分歧处置(MAX/整组替换/去数字化三选一裁定,现产线形结构无分歧);**词距外延候补(待用户裁)**:树面贴身记号族(「(⛓链上席)」/glyph-badge-词序列间距不一)是否统一加距——§29.61.12① 域为 ◎ 通道词,外延成批需裁定。观察续档:「全额→满额」模型 paraphrase(值零损,口径词回查落空,建议逐字引用=词面族素材)/mark70 计数当量词条-图例脱钩卫生候选。

## §29.80 PROSE-RC-4 收账(2026-07-14;第四改道向「嵌套-再减」双臂;复放 2/2 消退)
**臂①** 余数事实行第四姊妹句(OUTSIDE/净值性质,双语+实名 caller 符号列表帽5+溢出披露;四姊妹句族齐:no-caller/never-subtract/member-indivisible/outside-net);**臂② 隐式减法算术臂**(精确门四信号:X verbatim==余数发布值∧Y verbatim==已证份额发布值∧Z 非发布值(字符串精确+全发布数字集半 ulp 值级双门,过包容=安全方向)∧|X−ΣY−Z|≤0.002 自有容差;子集枚举帽 6/报告帽 2;纯事实附注零指控零输出影响)——未降级,5 负向 pin 收住误报面;突变 3+4 全红。**复放 2/2**(净室独立取码):10.117/0.296/2.731/8.534 族全零,第四句产线直出。**残余立案 PROSE-RC-5 候选**:减数位推广到任意发布值集(r1 五态账−余数跨口径 7.18/r2 blocking-span 错绑 5.5 两新形 witness;需 5-Q 误报审计)。

## §29.81 SUPP-CANCEL 收账(2026-07-14;视图内协作取消;§29.71 立案+批4 P3-5 关账;复核 SHIP-WITH-FIXES 核心不变量全实证)
**设计**:三案对比选 Query 载体(未导出 runCancel,零签名 churn,nil 载体结构性字节恒等;产线三 Run 位点逐 Run 新建=无跨 Run 复用);31 采样点(tick=64Ki 计数模+边界 sample);**取消语义=published⇒complete 不变量**(fired 粘滞+attach gate 整弃未完成面+先完成面保留;单出口 typed Result.ViewCancellation+恰一 caveat);三车道单一值源(模型=bus ctx 嵌套 min/补采=20s deadline 含 C-lite=批4 P3-5 关账/tracediag=结构性不可挂);补采按精确信号分类披露(execution_failed 静默裸丢形灭);ToolResult typed 镜像+R2' 七处。**DET 三层实录**:二进制级(tieba/donghu 模 generated_at 恒等——「0 diff」表述勘正为模头行恒等)+引擎级 armed pin 双真 trace+复核补验七 view 家族全恒等;病理 fixture(10 万行+1ms deadline)与真 trace(2ms 中断)双红绿。收尾=DiscardedFaces 门序反转(census 内 fire 形确定性 pin:64Ki 点算术强制落 census 内)+注释勘正。⑤ warm 双跑 pin 降标签(真防线=armed-vs-plain+二进制工件)。
**立案三条**:P3-B frame 族组装链零采样点(取消延迟上界随 index 放大;修向=组装主循环补 tick 零爆炸半径);P3-C stats overshoot 定量(2ms deadline 后 38~211ms 机器态波动,GB 级可放大秒~十秒;病根=pre-switch 探测+六边界间 untick 子段;修向=补桩+可选 16Ki mask);P3-D 补采零执行归因不对称(无条件时长预算词面误标用户中止形+parse 期分类竞态窗;修向=errors.Is 门镜像+canceled_by_caller 独立 reason+Execute 侧 typed 标志)。记账:evidence 行随 break 抑制形(面完整,契约不破)。h4 FAIL=CR-4 prose 族 A/B 双臂归因(非本批)。**用户资源看护四层闭环**:冷字节预算/视图间 deadline/窗跨门/视图内协作取消。

## §29.82 SUPP-HYG 收账(2026-07-14;§29.81 三立案+批4 P3-4 四件;overshoot 数量级压降)
**件①** frame 族补 tick(组装主循环;改前=2ms 过期 deadline 全程 NO-FIRE 跑完整视图,改后正常 fire 10ms;timeline/flow 显示帽下模点不可达=fired-carrier 短路 pin 如实注);**件②** stats untick 子段补桩(pre-switch 探测+六边界间七子段+**profile 实锤追加修**:resolveBlockedReasonThread 嵌套 O(rows×events) 解析器占 stats 构建>50%,穿透载体+补 tick)——**overshoot 复测:window_stats 225→6ms/critical 268→27/bundle 226→11**,观察延迟全视图≤10ms;残余=rank/bundle post-fire 尾 24-45ms 未桩子段(立案);**件③** 归因分叉(parse 期铸 typed TraceViewCancellation{View,Reason} 关竞态窗;errors.Is 镜像门=Deadline→时长预算/Canceled→**canceled_by_caller** 新 reason+zh/en ATOMIC 分叉,不再冒名预算不再误劝缩窗);**件④** skip-reason typed 闭集(12 成员常量表+注册表金样 pin+三面全转常量+NKR 同构 AST 三规则 pin,突变精确 file:line 红)。DET 双层复验(16 view 引擎级+二进制级模两头行恒等);全仓绿;h2/h6 回归 PASS。**立案**:rank/bundle post-fire 尾补桩(CANCEL-TAIL)。

## §29.83 RSPA-HYG 收账(2026-07-14;§29.77 立案六件+修复轮一件;全 pin/机械级;突变 9 红实测)
**件①** D2 交换形合成 fixture pin(`internal/types/trace_causal_projection_rspahyg_d2_test.go`):同 line-range 全值 twin(rank ⛓ 席 root_cause_secondary 记录+critical_blocking 未分解镜像行,chain_anchored/chain_anchor_full typed notes 产线发射形)→CompileTraceCausalProjection→survivor=⛓(EvidenceID=E-clip,发布值=锚定 3.598)∧ MergedEvidenceIDs 含被置换 E-full;负向臂=◇ 余段键叉永不并入全值身份。**pin 即揪出真缺陷并修根**:same-fact absorb 的 one-fact MAX 臂把 ⛓ survivor 的 CumulativeImpactMS 抬回被退役的全窗 claim(36.757)——补 `survivor.ChainAnchorFullMS == 0` 守卫(分解席值通道按构造自持,全账走 typed 字段;产线 donghu/tieba twins 因 line 不同从未触达此臂,合成补形 witness 实锤)。
**件②** Run 双 sweep 修根:root_cause_rank 混合视图 Run 原做 plain+anchored 两次 ComputeWindowStats——现 rank case 经 `getStatsForRank(chain)` 取 stats 面+anchored sweep 反哺共享 memo(导出面同构:pin 内 JSON byte 恒等+二进制 A/B 双 trace 复证);pin=Query 未导出 `statsSweepProbe` 精确计数(单 Run 恰 1,chainless 形同 1);**实测 donghu rank Run best-of-5 779.6ms→492.4ms(−37%)**。ThreadInput-only 形维持 HEAD 双 sweep(case 级 chain 门差异,收窄范围如实注);trace_perf_bundle/recipe 双 sweep 未动(取消面序不扰,续档候选)。
**件③** io facet 部分重叠收窄(裁定内值变化,EVOLUTION RECORD 在 query.go 判据臂):io_burst_episode/block_io_by_inode 宿主形凭证按 §29.61.10c 细化=区间⊆锚定窗(µs 容差,mint 期 typed containment 双 bit)成立才留链;部分重叠非 target 行降 ◇(值零触碰——D-IO 分区可加二分不读该 facet 综合/episode 口径,降道为机械相容收窄;剪值会铸既非测量值又非分区项的新值)。**witness=tieba ThreadPoolForeg-60555 block_io 包络 61.540ms 仅 24.568ms 在依赖窗内**,原全值留链现降 ◇(值 4.262 不变);donghu 17267 target-self 行豁免留链(负向 witness);io_burst_episode 无产线在场形=合成 unit fixture 补臂如实注。file_io/page_cache/workqueue/dma_fence 域外零触(立案③射程)。
**件④** reanchor 双过 byte 恒等专用 pin:六臂混合 fixture(case A 改写/case B 剪切+克隆/零凭证/D-IO 分账/卫星/fragmented)两过 DeepEqual+%#v byte 恒等;**突变实测如实注**:单删已迁移跳过守卫被下游车道排除+census 值恒等门吸收(纵深防御实录),复合突变(守卫+卫星标量恒等+车道门)红。
**件⑤** board sweep 放行臂 typed 化:「compacted∧<1ms」量级放行(嘈声信号进硬断言)换 typed 直接断言=counterpart 在 pre-truncation 池(RootCauseRankResult 未导出 `preTruncationItems`=build+enrich 两截断输入之并,两 cap 任一处死亡皆可核)∧ candidates Compaction 已披露。**typed 臂立即揪出旧放行掩盖的真形**:donghu udk-irq-12-92 的 ⛓ 剪切席(0.039ms)死于 build 截断 60→12 而 ◇ 余段活在 side lane——池并集后可核(账仍可由 ◇ 行自带分解字段还原,人口守恒经披露截断成立)。
**件⑥** side-row caveat 三类列举:「rank-0 diagnostic/target-self disclosure row(s)」句面(build+enrich 双 lane)补第三类=◇ chain-remainder seats(佩邻近序数非 rank-0);donghu 9/36+tieba 9/39 产线实发;旧二类句形负向 pin 关死。
**修复轮(件⑤ sweep dump 实锤)**:rspaRemainderSummary Sprintf 槽位 anchored/full 互换自 RSPA 批出厂——LLM-facing 英文账句读作「full-window account 0.039ms = 0.307ms anchored」(donghu 旗舰行同病:3.598/36.757 互换);typed 字段与显示面从未受染;修根+算术 pin。
**验收**:全仓 `go test ./...` 绿(独立后台趟 exit 0);DET 三针复验(DET1 typed 流 3 过恒等/RunCancel armed 双真 trace byte 恒等/deadline 红绿);**二进制 A/B**(git archive HEAD 净室快照,donghu+tieba×6 view Result JSON):真值 delta 恰三族=件③ 车道对(chain_relevance/causality ×2 面)+件⑥ 句面+修复轮 summary 槽位,余为环境噪声(mtime/路径)与同值行位置对换,rank Run 的 window_stats 面与 plain byte 恒等(件② 同构直证);突变 9 红全实测(M1 反哺/M2 backfeed/M3 判据臂/M5 池快照/M6 句面/M7a 交换臂/M7b id 记账/M7c cumulative 守卫/M8 槽位;M4 单点被纵深吸收如实注)。金样本 h3/h6 各 1 趟双 PASS(154203/154552 工件;h3 完成端到端·IO延迟 硬杠在场 ×2,target-self io 行豁免直证)。

## §29.84 LT-HYG 收账(2026-07-14;件①②③ 交付+件④ 持裁;h2/h6 各 1 趟 PASS)
**件① CANCEL-TAIL**:病根=完整性探针全索引重扫(binder 配对~11ms 最重/incarnation 多建造器各自重跑/调度序/head 精修/catalog)+bundle 组装链无阶段门+出口 caveat 重建全量 timeline——探针补 tick/早退门+组装 7 阶段门+出口门(附加诚实收益:不再从截断 timeline 铸 coverage 声明);**复测:rank 尾 27.1→2.9ms/bundle 44.2→2.5/pre-expired 入口 36.5→0.0**;DET armed 双层复验绿。**件② decoder-remap hint**:strict-decode unknown-field 拒绝文案追加 schema 反射字段清单(声明序禁手抄;仅无 relocate hint 时;nil-schema 字节恒等 pin;全体内建工具免费获益;B5 零字节)。**件③ mark70 再耦合**:◎ 脚注发射点点亮 mark 79/70(typed class 判),NEW-7 双向 sweep 自动看守,在库产线形 pin。
**件④ CASE-3 ❹ 裁定(主会话按委托默认)**:三案素材(A 词面臂 typed donor 记号/B 引擎重铸 eff:=Σ member eff/C 专词)——**裁定=B 根修+A 止血**:合并行 eff=种子单成员值继承(aggregate.go 明注 inherited VALUE stays untouched)=唯一零词面披露的误导面,「3次·折算 2.500」必被读作合计;Σ member eff 与 §29.50.4 合计参赛同向;**任一成员无 eff→清零回退(宁缺勿假禁混 cum 口径)**;◎ 榜序/加冕随动=诚实后果,金样本复扫入工单;A 的 MergedEffectiveDonorOnly typed 记号在 B 落地前的 pre-merged 种子形上仍需(provenance 唯一可知处);伴生多窗合并行窗标 chip 未披露同批。=CASE3-D4 批。**残留**:2ms 残尾 16Ki mask 续档/repomap 域清单跟进/perf_timeline lane tick/正常路径探针重复扫 perf 续档。

## §29.85 CASE3-D4 收账(2026-07-14;B 根修+A 收编+伴生窗标+件⑤⑥ UX 用户裁定;h2/h6/h5 各 1 趟 PASS)
**B 根修**(`traceCausalProjectionMergeSameKindMembers` plain 臂,"inherited VALUE stays untouched" 退役):全员有 eff→`eff:=Σ member eff`(published=成员 OR);任一成员 eff≤0→整行清零+un-publish(ABSENT 非 published-0,防误触拒冕臂);union/crossWindowMax 口径→清零(§21 CWD/§11-N2 墙钟跨窗不可加和,Σ 会重铸刚退役的重复计数且超发布值)。两合并入口共用单一权威。合成 witness 三面:树行 2.500→8.000(Σ 手算核对)/◎ 榜同步/disp3 E22 17.780=display Σ 等值折叠。**A 收编**:五 lane 审计(周期Σ/混合清零/plain新臂/R1同事实视图继承/R3 fresh-node/WO-G2 typed 四零门/wire-fold 引擎自发布)无「单成员值继承」残余形,MergedEffectiveDonorOnly 记号无落点,不实施。**伴生窗标**:多窗合并 rank 行 chip=`窗X~Ys(供席成员窗,成员跨K窗)`(单榜也披露);◎ 行追加 ` · 成员跨K窗`;typed 键复用 runtimeTraceProjMultiWindowMergedRow 零新信号;新 mark+图例双向 fixture。
**件⑤ ◇最大 记号退役(用户裁定 2026-07-14)**:值可见+满格 bar+表头「满格=本区TOP1」双载信号,记号冗余;§2.5 保底席位语义原样(pin 改断言席位+值+栅格);lead 场机制(leadWidth)整删,全板 flush 单栅格;EN "◇max" 同退。**件⑥ 构成注缩进反转修根(用户裁定)**:病根=注行 · 在 leadWidth+2 列而值数字起点 leadWidth+4 列,注行视觉成父节点;修=缩进常量 `runtimeTraceProjElimValueFieldWidth=12`(与 `%9.3fms ` 字段挂钩注释绑定),· 恰落 bar 起始列;h6 产线板复证两构成注 12 列+保底 33.159 行普通形在席。
**突变红绿**:M-D4 恢复继承值→7 pin 红;M-⑤ 恢复记号→1 红;M-⑥ 恢复旧缩进→1 红;恢复后全绿,sha 逐字节核对。**残留立案**:①周期臂 Σ under 跨窗口径理论双计(pre-existing,待裁);②「单次最大」等式面休眠词义收窄(待裁);③MergedCount pre-merged 成员 UNDERcount 卫生;④chip 窗不可解行2 无端点披露形(待裁);⑤多窗行成员缺 eff 清零失席=裁定后果;⑥h2 efficiency advisory pre-existing。

## §29.86 ELIM-CHAN 收账(2026-07-14;用户裁定:◎ 板 ⛓ 链上词墨绿单编码,HTML 渲染层)
用户裁定两步:①链上/邻近区域区分不明显→链上词加不抢眼颜色(墨绿)或加粗;②**二选一禁双编码**→主会话定向墨绿单编码(颜色零布局风险+与 --action-border 既有视觉语言同源)。实现:fence 字节零动(三端一致红线),HTML 层 token 臂——`isTraceElimOverviewFence` typed token 精确判定(ElimInfoToken),转义后 token 粒度注入 `<span class="elim-chain-word">`,token 闭集=`GlyphIOChain+" 链上"/" on-chain"`(glyph 走表① 单源,UXG-1 tripwire 实拦手写 ⛓;名词逐字转录自 runtimeTraceProjElimChannelWord);内层 markup 与未包裹形逐字节相同,textContent==fence 字节由 pin 看守。CSS `--elim-chain-fg` 三主题(light #14532d/dark #bbf7d0/print #14532d,独立槽位不随 rank 调色盘漂移),规则仅 color 零 font-weight。表头「⛓ 链上块先」同 token 一并着色;空链诚实行无 glyph 不命中(无行不佩戴)。pin 六测试(zh恰3/EN形/◇邻近负/转义/作用域负/CSS三声明);突变 M-E1 token 臂拔除→3 红、M-E2 作用域放宽→scope 红。金样本免(fence 字节零动+HTML 纯加性,依据在案)。**残留立案**:①token 词面单源化候选(提升进 tracefence,同 SeatChannel* 先例);②投影树面通道词着色同理受益(待裁不实施);③空链诚实行纯名词不着色(设计如此,期望变更需另裁)。

## §29.87 RSPA-HYG 残余收账(2026-07-14;四件全收;A/B 16/16 byte 恒等;金样本按纪律免趟)
**① 双 sweep 修根+词面延伸**:bundle/recipe 车道 case 入口预拉 `getStatsForRank(getChain())` 反哺共享 memo,chain-bearing Run 从 plain+anchored 两次 ComputeWindowStats 降为恰一次——**donghu 旗舰窗 bundle 830.6→520.3ms(−37%)/recipe 924.0→619.0ms(−33%)**;recipe 预拉置 discovery 守卫后,faceCanceled 检查序零改动,fired-lane 整弃语义如实注;词面 sweep 延伸核清:两车道与 root_cause_rank 共享全部 RSPA 发射点,35 槽 banner 逐一核序全对,未发现新病形,唯一防线缺口(⛓ anchored 姐妹句无算术 pin)补 pin。**② ThreadInput-only 形**:门分歧实锤(case 门漏 ThreadInput,可达形限退化 selector 如 "-");HEAD 后果=结构性饿死非双 sweep;修根=四处手抄门收敛单一 `rankChainGate` 谓词(精确信号单一定义),退化形照 bundle 既有形发布空链+诚实歧义 caveat。**③ io facet 域外逐边核(§29.61.10b/c 机械延伸)**:file_io_hot_inode/workqueue_activity/dma_fence_activity 三 facet 应锚定(mint 期 typed containment 双 bit+enrich 判据臂,件③ 同机制,合成 fixture 覆盖——双旗舰窗零产线 on-chain 在场,首个产线实锤待客户新 trace);page_cache_churn 应豁免(闭集结构性排除,计数分非墙钟,tieba/donghu 双产线 adjacent witness pin)。**④ perf_timeline lane tick**:`threadIncarnationConflictForPIDSet` 补门+tick,§29.84 件① 孪生同构,armed-unfired %#v 字节恒等臂。
**红绿**:突变 8/8 红(预拉×2/门回退/槽位对调/豁免闭集/判据臂/stamp/tick);二进制 A/B(净室 20a54e16 vs 工作树,donghu+tieba×8 view)模环境噪声后 16/16 byte 恒等真值 delta 恰零。**残留**:退化 selector 空链发布词面 UX 候选;三锚定 facet 产线 witness 待回访。

## §29.88 立案:客户 runnable.txt 双病(2026-07-14,用户高优先级;witness=/Users/han/opt/customlogs/runnable.txt,原始 ftrace 未入库待客户补交)
**用户裁定 R1(ELIM-SEM)**:◎ 窗内可消除量总览必须含**链上语义类席位**(既有裁定重申:链上语义类「确定性优化」==「语义优化span」,都有被答案提及的义务);同类可折叠合并(如 类校验 N次 一席),细节不在 ◎ 展开。witness:E29 类校验 9.586ms=根因排序#6 链上语义席,◎ 板零语义席。
**用户裁定 R2(CHAIN-CRED2)**:链上「有效归因」必须明确为**能够证明直接或间接(唤醒链依赖/binder/锁等,多跳 typed 边有关联)影响目标线程的部分**;虽在窗内但已解除影响后的部分不得计入链上;无唤醒关系依赖的部分归位 ◇ 邻近,防链上误判扩大。runnable 多片时只有前段直接/间接阻塞目标线程、后段无唤醒关系→后段不得计入。此前病例=优先级反转(RSPA 已修),本次=调度压力候选,要求**全状态族+全部有效归因车道排查残留**。
**witness 矛盾清单**:W1=E8 深度未解析链上席以 26.392ms 全额参赛 rank#1+◎ 首席,而同线程 E6 自证「窗内 runnable 合计 26.392,链上仅覆盖最大片段 8.606(33%)」,答案正文放大为「runnable等待26.392ms导致优先级反转」(实际 wakeup latency 8.870ms);W2=E22/E23/E24/E25/E27 同族(调度压力候选·链上·深度未解析)全窗合计全额参赛 #2-#5/#9;W3=E32 同源二分句「全窗9.272=锚定0.000+其余9.272」与 E9 互指(E9 自称⛓凭证锚定段 5.368 还原同一 9.272 全窗账)矛盾,且 E32 行值 10.643>全窗 9.272 无面上解释,E9+E10=10.682 与 9.272 关系不明;W4=◇ 区语义行(E34-E40)是否参赛 ◎ 待调查方案。

### §29.88.1 用户裁定 R3:非目标线程语义 span 的链上尺判据(2026-07-14,即答 ELIM-SEM 尺问题)
非目标线程的语义 span(witness 例:E30 Texture upload 位于 RenderThread-64334,链上L1)入链/入 ◎ 判据=**宿主线程对目标线程存在窗内 typed 唤醒边(直接或经链上多跳间接),且 span 段位于该唤醒边之前**(影响解除前)→ 该部分算链上席;无边、或位于边后(影响已解除)的部分→ ◇ 邻近。与 R2 同一锚定语义(边=凭证,边前=有效,边后=解除),即语义 span 与调度状态段同享 RSPA 锚定/二分原则,span 跨边时按边界二分。目标线程自身 span(E29 类校验)不涉边,照 SELF-SEM(§29.61.1)原样链上。

### §29.88.2 用户裁定 R4:「边=凭证,边前=有效,边后=解除」升格全状态族链上唯一精确判据(2026-07-14,终判通则)
「边=凭证,边前=有效,边后=解除」作为**链上席位的唯一精确证据语义**,不仅语义 span,全部状态族一体适用(running/D-state/runnable/IO/sleep/binder/语义 span 等):任何段要计入链上有效归因,必须能证明其位于一条 typed 因果边(唤醒/binder/锁等,直接或经链上多跳间接——**多跳间接亦可**,用户 2026-07-14 原话重申)之前且与该边关联(多跳形=凭证经中间跳传递:如 WifiHandlerThre→binder:500_1B→UI 三级链,各跳段以本跳向下游的边为凭证锚定,锚窗沿链传递推导,非只认直接唤醒目标者);凡出示不了该凭证的部分——无边、边后(影响已解除)、深度未解析给不出边——**一律归 ◇ 邻近**。链上=精确信号硬门,邻近=容纳其余,与「精确信号硬门/嘈声信号软引导」架构红线同构;§29.61.10c「逐边核非按类」在此升格为排他通则。
carve(既有裁定不受影响):目标线程**自身**席位(SELF-SEM §29.61.1 自身语义 span/SELF-ALL §29.61.2 自身四态墙钟席)——自身即目标,无「影响目标的边」概念,照旧链上;E4/E5 类自身行不涉本判据。

### §29.88.3 用户裁定 R5:running 折算基准统一为全域最大核最大频点(2026-07-14)
running 状态折算影响时(**含优先级反转场景**,全场景统一):一律按**最大核最大频点(全域)**为基准折算,取代现行「按下游消费核」基准及其「簇结构不可判→按纯频率比」回退链。操作注:全域最大核=治理时间线上有证据的真实核(CMP-10 判例:fmax 只取治理时间线、幽灵核排除),非 topology 幽灵;既有「下界」诚实词面族(频率数据缺失段计 0/核类算力差标注)在新基准下重新核词;涉及词面=「折算,按下游消费核」「按纯频率比折算」「按满频折算」全族随基准统一重审。witness 触点:E19 running 2.878→计入1.659(按下游消费核,簇结构不可判回退)、E14/E20 满频注、供给折算缺口族。并入 runnable.txt 修复批(R1/R3/R4/R5 同批)。

## §29.89 QH2-A 收账(2026-07-14;G10-EN 根修+综合评分 ms 两站点;h2 1 趟 PASS)
**件1 G10-EN**:withdraw witness typed 组件化——新 `TraceHolderSelfContradictionWitness{Holder,OwnerTid,QueuedMs,SpanMs,LineStart,LineEnd}`+`WitnessText(zh)` 单一措辞源;引擎 zh 串按构造字节恒等,Summary EN 句改嵌 WitnessText(false)(「EN 句逐字内嵌 zh 体」灭);wire `holder_self_contradiction_parts` 入 rank+critical_blocking;显示两 lane 各自措辞(zh verbatim pin,EN 全英文句),无组件 legacy 保 verbatim 回退。R2' 七处逐处勾账(tracediag RootCauseRankItem schema hash 8c729036→85fb376b 带 review 注)。族内 fold_rail_basis 论证豁免(display_only 零显示消费方,唯一面=证据索引审计 lane §22.2.1 保留裁定);typelabels.go:682 零触(§29.73 设计)。**件2 综合评分 ms**:站① 关键指标表 composite 行 cell=`X(综合评分,非墙钟)`(词面单源 helper 从树行1 mint 提取);站② rank 行 LLM 面 composite 正值槽去 ms 冠口径词(registry caliber class 精确门),零槽保 0.000ms,actual_* 无条件保墙钟 ms(双底账口径分离);`*_impact_ms` wire 名零动(留裁)。突变 M1-M4 红绿(sha 逐一比中);字节对照 diff 恰为两站点。**残留**:blocking detail key=value 面 verbatim zh witness(EN 组件已就位,收编一行改动候选);观测账 digest 面 composite `值=X ms`(Unit 字段族,与 wire 改名同留裁);count 行 cell 裸 ms 负向 pin 钉为不变(§29.73 件③ 射程外)。

### §29.88.4 用户裁定 R5a:限频/绑核降级检测的提及义务(2026-07-14,R5 补充)
在 R5(running 折算统一按全域最大核最大频点为基准)之上:**若发现限频(频点被治理/策略压低),或绑核(affinity/cpuset)限制了线程上更大核的可能性的情况,答案必须提及**(用户 2026-07-14 勘正原「小于超大核」措辞:判据=绑定排除了本可使用的更大核——允许核集的最大核类<全域可用最大核类时提及;绑定已含全域最大核类、或全域本无更大核可上,则不构成限制不提及)——与「优化点无条件入正文」同族的提及义务,非可选注记。操作面:①检测源=治理时间线(CMP-10 判例)与 cpu_affinity_or_cpuset 席(query.go:13578 族,RNB-1 正在按 R4 补臂);②提及形=席位注记/答案 caveat lane 走既有披露车道,值口径与 R5 折算基准同源(被限频线程的折算缺口按全域最大核最大频点算,限频事实即缺口成因披露);③绑核判定基准=允许核集最大核类对照全域可用最大核类(「限制了上更大核的可能性」判据,非静态超大核比较)。并入 RNB-4(R5 折算基准批)同批交付。

### §29.88.5 witness W5:donghu 产线活体同病(2026-07-14,用户指认 20260714-200729.050-98554)
仓内 fixture donghu.ftrace 旗舰窗(13762.791708~13763.024898)、目标=CompThread_0-2955 的产线报告(.codrax/output/20260714-200729.050-98554.md)呈 runnable.txt 同病全形:①根因榜 #1/#2/#4-#12 全「调度压力候选·链上·深度未解析」,#2 logd.writer-9163 runnable 47.678 全额/#4 低频运行 22.408 全额/#5 hilogcat CPU亲和 16.013 全额;②**B-2 车道产线实锤**:◎ 板 `logd.writer-9163 · CPU亲和/cpuset限制 47.678 [E29]` 与同线程 runnable 席同值 47.678 双席(同一物理时间两车道各全额,cpu_affinity 无 RSPA 臂 query.go:13578);③自相矛盾指纹:树头「链上已归因 14.750(13%)」vs #1 席 47.678=3.2×(runnable.txt 为 3.0×);④低频运行卫星全额=卫星车道 per-pid 决策亦 fail-open(T2)。**价值:与客户 runnable.txt 不同,本 witness 基于仓内 fixture 可无限复放——直接列 RNB-1 验收 A/B 主锚点。**

### §29.88.6 立案 AFF-EVID:CPU亲和/cpuset 席证据与描述面缺失(2026-07-14,用户指认,witness=W5 报告 E29/E31)
病形:亲和席树行仅 `CPU亲和/cpuset限制 · runnable` 裸词,明细仅类型/影响形态两行,证据索引定位跨近全 trace(E29=行1308–27460)——**裸断言席**:读者无从知道限制是什么、为何判定。病根=有料不上桌:铸造点(query.go:13585-13600)typed payload 在手——`AllowedCPUs`(允许核清单)/`CPUSet`(组名)/`Policy`(restricted=true 参与置信 0.64→0.72 分档)/`Summary`(已传入 rootCauseItem)——投影/显示链路未携带。修向:①约束描述入行3/明细(允许核集 vs 全域核集对照、cpuset 组名、判定依据),typed 字段走 R2' 七处;②证据定位收窄到判定所依的具体事件行(affinity 设定事件/受限 runnable 段),禁全窗 span 充数;③与 R5a(§29.88.4 绑核小于大核提及义务)同源:AllowedCPUs 对照全域最大核类即 R5a 判据输入,描述面与提及义务共用 typed 字段。归 RNB-2 显示批(与 W3 病①②③/ELIM-SEM 同批);RNB-1 的 R4 亲和臂(通道/值门)照常,两批不冲突。

### §29.88.7 用户裁定 R5b:折算场景「运行频点非最高」提及义务(2026-07-14,R5a 澄清+第三场景)
用户澄清:§29.88.4 的两条判据(提及=绑核限制上更大核可能性/不提及=绑定含全域最大核类或无更大核)**仅针对存在绑核情况**。另立第三种提及场景——**算力折算时,不管有没有绑核,running 实际运行的核未按照全域最大核最高频点运行(折算影响算法同 R5 基准),也必须提及「运行频点非最高」**。即三场景并立:①限频(治理/策略压低)→提及;②有绑核且排除了本可上的更大核→提及;③折算发现实际运行核/频点低于全域最大核最高频点→提及(该场景=R5 折算缺口的成因披露,凡按 R5 基准折算出非零缺口的 running 席,其「运行频点非最高/运行核非最大核」事实即席位披露义务,与既有供给缺口披露词面族合流重审)。归 RNB-4(R5 族批)。

### §29.88.8 东湖双 fixture 挖窗复测矩阵(2026-07-14,四路零 LLM 扫描;参数统一 MaxDepth=4 MinDurationMs=0.5 Limit=12 harmony_hitrace)
**结构事实**:donghu.ftrace 全文件跨度=旗舰窗(233.19ms,新维度=新目标线程/子窗);拓扑 14 核三簇,全域最大核=cpu12/13@2750000kHz(观测,非幽灵);tieba 旗舰窗外留头窗 22.238ms+尾窗 7.379ms(唯一 doFrame 在尾窗),6 核,cpu0-2 零频点样本,全域最大核=cpu3/4/5@2189000。
**R4 锚点(RNB-1 验收,SCAN-2)**:①donghu 目标 2955 全窗:logd.writer-9163 席 47.678 rank#1 无分解,census full=49.656/anchored=**0.018**/链Σ=0.018,dec 已铸而 :544 席位恒等 fail(5 组跨 CPU census,席=前3组折叠)——**修后链上理论值 0.018,47.66 归 ◇**;同窗 hilogcat-9503 健康二分(17.292=0.026+17.266)=同窗对照。②tieba 目标 59566 H1 窗(34579.450627..34579.522905):CookieMonsterCl-59843 席 22.277 无分解,census anch=14.079 vs 链Σ=13.363(0.716 分歧→:280 fail),同板链账行 impact=12.097=W1 双面矛盾同形;60595 depth-2 二分已发布=多跳回归对照。③tieba Q2 窗:59843 席 16.877(仅 cpu0 组)≠full 16.963(cpu1 尘埃 0.086)=INV-D S5 拓扑真机活体(最纯 T1)。④donghu H2 窗目标 19050:一窗 9 个 :544 失败=批量回归清单。⑤tieba 60555 D-IO 席(d 10.433+io 7.386,dio census 18.135/17.819,depth=3)=D-IO 臂+L3 多跳场。⑥卫星漏臂活体:tieba PRE 窗 59953 low_frequency 席 10.776 无分解(dec=true anchored=8.338)。tracediag 帽注:donghu 宽窗 rank view 1715 行超 1000 帽截断,验收用包内探针或窄窗;tieba 全窗完整。
**语义/R3 锚点(RNB-2/3,SCAN-3)**:两 fixture 语义 span 极稀(donghu 恰 2 条 jit 同线程 17284;tieba 恰 1 条 VerifyClass@61839)。①保底席触发窗:tieba 61839 目标窗 34579.470..34579.520,语义席 rank#8 恰被 TOP5 切(切点#5=1.338)=E29 同构;边界内对照窗(旗舰窗界)rank#5 在榜。②**R3 边锚定正负臂对**:正=窗 34579.490..34579.500(宿主 61839→目标 59566 裸边 34579.496810 行5639 在窗内,span 495841..496126 整体边前→修后应铸链上席);负=窗 34579.466..34579.4965(同 span 在窗,边全在窗外→必须留 ◇);窗界移 0.4ms 即翻道=边界 sentinel。③SELF-SEM 高位:donghu 17284 目标窗 13762.845..13762.900,jit 族折叠席 rank#2(2.388=1.781+0.607)。④R4 排他诱错窗:donghu 17267 窗 13762.890..13762.900,span2 横跨**他人**边(binder:496_9→17267)宿主自身无边→不得给席。⑤负结果:◇ 语义行 ≥3(W4-a 脚注形)两 fixture 无自然 witness,需客户 ftrace 或 SemanticSpanPattern 注入。
**R5 族锚点(RNB-4,SCAN-4)**:词面 typed 载体核清(gatedRunningDeficit→「按下游消费核」rcr.go:773;freq_only→「纯频率比」;refClass=big→「按大核满频」;ThermalCapWitnessed→热限词)。①A 锚点:donghu 目标 17597,keva-1 席 eff=3.399(gated 2.181+缺口 1.218)/keva-3 同行 gated 2.160 vs fold 2.286 两车道互异=R5 统一靶;链首 17267 running 席 eff=51.735=fold deficit(ideal=91.764)。②**B 锚点(R5a 判据缺口实锤)**:donghu 目标 9655 窗 13762.934161..:自身绑核席 27.601 on-chain,mask=ffb allowed=[0,1,3..11] **排除全域最大核 12/13,而 AllowedCoreClasses 面写 [small,middle,big] 无法表达该排除**(9-11 亦 big 类但非 2750 档)——「限制上更大核可能性」判据必须按核档而非核类。③tieba 全 trace 绑核/热限双无在场(如实负record;替代形=freq_only 词面族+cpu0-2 频点缺失「计 0」形,FusionSearch 窗 hilogd.pst 席 gated 0.000 vs fold 3.766 互异)。④donghu 热限三簇 typed 三形俱在(small→1530/middle→1880/big→2340,均<2750)+cpu0/cpu4 limits 事件密集窗。⑤帧窗底图(SCAN-1):donghu D 族窗(2955,dma_fence 11 段)/页缓存回收风暴窗(sysmgr-reclaim0,mm_filemap_delete 2193 行)/微片化 running+限频拍频窗(hmfs_txn,61 片+limits 8 行)/jank resync 记号窗(17267,13762.925..955);tieba 调度压力窗(sysevent_store,runnable 136 片 49%>running)/头窗 R5 主战场(59566 running 89%+三级 wifi 链)/尾窗真帧(doFrame+UI→RT 边)。

### §29.88.9 用户裁定 R6:核簇推导三规则(2026-07-14;witness=donghu.ftrace 地面真值)
**donghu.ftrace 地面真值(用户 2026-07-14 二次勘正)**:[0,1,2,3]=小核、[4,5,6,7,8,9,10,11]=中核、[12,13]=大核。现行引擎推导(§29.88.8 扫描实录:cpu4-8=middle、cpu9-11=big@2270000)与真值不符(**9/10/11 三核均误归 big**)=CLUSTER-DERIVE 病立案;按规则 2+3,4-11 若全域频点变化一致(或一致核间夹自然数编号核)应闭包为单一中核簇。**推导规则(用户裁定)**:
1. **首簇规则**:从 0 核开始到第一个有频点的核,同属一簇(前导无频点核归首簇);
2. **同簇判据=全域频点变化一致**:同簇的核在全域内的频点变化(曲线)完全一致;频点为 0 或无频点的样本不算「频点相同」(不得以零/缺样本充当一致证据);
3. **区间闭包**:全域内频点变化一致的几个核(排除频点 0/无频点者)构成一簇时,其间自然数编号所含的核(含频点为 0 或无频点的)全部并入该簇。
4. **全文件扫描(用户 2026-07-14 补充,同日二次扩面)**:推导输入=整份 trace 的频点样本,**不得只看头尾或查询窗局部**——频点变化在文件头尾缺失常见,局部窗口推导必错;实现注意性能效率(建议随 BuildIndex 单趟顺扫捎带收集 per-cpu 频点曲线,禁二次全文件重扫——**此禁令已被用户裁定 SUPERSEDED(2026-07-18,见 §29.129:可二次全文件重扫,控成本+复用,单次问答复用/跨问题重新计数**)。**适用面=簇成员推导+各簇最大频点+全域最大核最大频点(R5 折算基准)等一切频点派生量**——R5 基准若从窗局部取样,头尾缺失窗会低估 fmax 使折算缺口系统性偏小。
**消费面**:compute_supply_balance per_cpu 簇分类、refClass(big/middle/small)词面、供给折算 basis、R5 全域最大核判定(donghu=cpu12/13@2750000 不受影响,但簇词面/热限 per-cluster 词/R5a 核档比较全依赖正确簇)。归 RNB-4 批(R5 族),donghu 三簇真值立 pin fixture;tieba(cpu0-2 零频点样本)按规则 1+3 复核推导。

### §29.88.10 用户裁定 R7:◎ 板同线程同值双席与单分量构成注/复合词(2026-07-14,witness=20260714-230952.308-93419)
witness:◎ 板 JankManager-9655 双席同值 0.423(E31 优先级反转候选·供给缺口主导 + E32 runnable·合计共3段)+ E31 构成注「可消除构成: 调度修复 0.423ms」单分量。**裁定两条**:
1. **同线程同值双席=双算呈现,须收敛**:同一物理时间经反转席与 runnable 普查席两车道各发一席进 ◎,虽零求和但视觉双算;按 same-fact absorb 判例(§29.83 件①/树面「同段两车道已合并为一行」先例)收敛为一席(E# 互并),明细保双车道账。
2. **单分量构成注不显示+复合词不得出现**:构成注只有「调度修复」单分量(=行值自身,零信息)→ 不显示;且有效归因构成中**零供给分量(无 running 折算项)时,「供给缺口主导」复合后缀不得出现**——缺口是独立口径不计入有效归因,构成里没有供给分量却戴「供给缺口主导」词=词面自相矛盾;复合词判据在「缺口≥eff×50%」纯比较之上增加「eff 构成含 running 折算分量」构成性前提。
归 RNB-1 在飞批(同板同 witness 窗)。

### §29.88.11 用户裁定 R7a:◎ 板构成拆解移出 bar 区,独立拆解区按 [E#] 索引(2026-07-14)
用户裁定:「可消除构成」类子因素拆解行插在 bar 席行之间破坏栅格视觉,改为**bar 区下方独立拆解区**,逐条以 [E#] 前缀精确索引回席行与证据索引,不降低信息丰富度。定案形:bar 区=纯席行零行间子行;席行区之后、既有脚注区(不参与汇排/◇指针/▒)之前,新增 `· 构成拆解(按 [E#] 索引):` 区,条目=`[E#] 可消除构成: <原拆解字节原样>`,按板内席行序排列;区仅在存在 ≥1 条(R7 后=多分量)构成注时出现;zh/EN 双词面+图例句随迁;mark 点亮语义不变(词条-图例双向)。与 R7(§29.88.10 单分量退场)同批。此形为一切未来「席行从属拆解」的通用容器(不再允许行间子行回潮)。

## §29.90 RNB-1 旗舰批收账(2026-07-14→15;R2/R4 链上席重锚+双复核+回派修复+R7/R7a;h1..h6 全绿,h2 复扫 2/2)
**引擎重锚(方案 C=二分与权属解耦)**:门5 µs 恒等降级 case-A 权属资格(失合→案A' 迁 ◇+typed 双Σ披露 chain_anchor_ownership_divergent/chain_lane/census,armed tick=per-seat 字段);门3' 退役(链账缺席=case B census 自足二分);T1 席位恒等灭形→census 组账戳(mint `ledgerAnchorStamped/ledgerAnchoredRunnableMs`+fold 仅 sum_disjoint Σ 传播);B-3 在场判定补席发布值校验;M-D 抑制补 identityHolds;R4 三臂=inversion 改型/cpu_affinity/卫星无区间回退→整席 ◇ 降道(chain_credential_lane_demoted 值零动);B-4 零凭证 D/IO VIEW 行降道(bundle 路径;standalone plain-stats 保 legacy=标准 anchor-less 边界,代码注明);fold anchorForm 键防余段/降道席再 Σ。R2' 七处(tracediag hash 9125207f)。
**双复核(对抗+冷读,推送前双门)**:对抗官 23 (目标,窗) 组合扫描 after=0 病席、假 pin 猎杀 1 实锤(B-4 负向臂 vacuous conjunction)+1 缺 pin、R4 多跳/自身 carve/inversion 降道合规无需新裁定、零误降;冷读官裸读 diff+双锚窗渲染 witness+手算守恒全过,**D1 实锤**=余段/降道席被旧容量结构吞没(降道行无 side lane 全灭/邻近桶帽8+in-path 无条件优先吞 8 余段席/9 句悬空引用/◎ ◇ 注记假句)。**回派修复**:①demoted side lane(cap 8 镜像);②projection ◇/▒ 桶持值席值序先+in-path 退后+post-aggregation resort 同志愿(复核抓到 resort 复辟)+溢出铸折叠行零静默丢弃(PTV6D pin 演进实证旧帽本就静默丢 3 行);③句面 twin 可见性 sweep(ChainAnchorTwinInvisible 降级句)+引擎 post-truncation summary patch 幂等双 pass;④B-4 假 pin 修真(MD 复放红)。
**R7/R7a 件C**:C-1 ◎ 同线程同值双席收敛(精确门=同通道∧同主体∧eff µs 全等∧typed 同段,E# 互并,值分歧负向臂保双席;JankManager 活体在重构后窗已不在场,合成 fixture 承载);C-2 复合词构成性前提(GatedRunningDeficitMS>0 才可佩「供给缺口主导」)+单分量构成注退场;C-3 构成拆解迁 bar 区下独立区(`· 构成拆解(按 [E#] 索引):`,bar 区无行间子行负向 pin,件⑥ 12 列缩进形退役,INV-SUPPLY 双 pin 论证演进)。
**撞车事故与 S5/S6 演进(双出处)**:主会话 autostash 与复活 agent 实时写入冲突(query.go 单文件),手拼块(组账戳+scopes 分派)经 agent 按设计意图验证为正确;S5/S6 验收数字演进为线程总账形(97.5=27+70.5/75.5=25+50.5)——混车道拆分逃逸由 mint 源头线程总账聚合关闭+组账戳/fold 守卫双层保底。**范式**:主会话 docs-only 推送改走独立 detached worktree(已执行),严禁在飞树上 autostash。
**A/B 终录**:donghu2955=◎ 首席自身 D 36.757/锚定微席 0.018+0.026 在榜/◇ 49.638 满格/demoted 行佩「无链上凭证(整席降道)」独立在席/零悬空句/◎ ◇ 注记真值;tieba 负向恒等(60555 逐字节/17267 四行守卫值恒等);金样 h1..h6 全绿+h2 复扫 2/2(SUPP flake 未现)。突变总账 20 红(原批 9+D1 电池 7+R7 电池 4),sha 逐字节恢复。
**残留**:①W3 病①③+E7 继承源(R2 ×N merge eff 车道,需客户 ftrace)→RNB-2;②D2 尘埃锚定 pid 的 VIEW 行处置(有理由收窄,补裁定);③P4 卫星双账句「锚定」词潜在双值;④remainder/demoted side cap 8 溢出=caveat 计数+句面降级有界形(容量裁定候选);⑤U1(1.982 差额悬空感)/U2(双「窗内合计」无互解)/U3(微席榜面)观察,U3 已由 D1 修复大幅缓解。

### §29.90.1 立案 SUPP-TARGET:分析器变体分类不铸 runtime_targets→补采 fail-open 跳过(冷读官发现,h2 1 FAIL 复放)
病形:h2 某趟 analyzer 分类 intent=return_value/kind=conditional 且未铸 runtime_targets(entities 有 CompThread_0-2955,typed 车道空)→SUPP-CORE 补采 skip reason=no_typed_target→无树→三 hard oracle 全缺,答案以 blocked_reason delay 合计 39.157 顶替 36.757 墙钟账。在 SUPP-ORACLE 批2「dispatch-shaped FAIL 从此=回归」承诺射程内;修向=补采 entities-lane 回退或 analyzer 侧 runtime_targets pin。独立批,非 RNB-1 diff 所致(17 文件零触 analyzer/补采)。

### §29.88.12 R5 补强:两套 running 折算算法统一为单基准单算法(2026-07-15,用户 witness=20260714-235214.072-29116)
witness 实锤双算法同席并存:E10 行同一段 running 8.294ms,「计入 6.972ms(折算,**按下游消费核**,按实测频点共动分簇折算)」与「供给折算缺口 7.296ms(**按大核满频折算**,下界)」两套基准两个数并存,读者无从互推对账。**用户裁定**:统一为「最大核最高频(全域)」折算=单基准单算法(R5 §29.88.3 的显式扩面——不止取代 gated 车道的「按下游消费核」,supply-fold 车道的「按大核满频」也并入同一全域基准);同席只呈一套折算数,构成拆解/缺口与计入值同源可互推;热限(如 witness 的 1.53GHz)/降频/小核事实保留为成因披露(R5b 提及义务),不改变基准。fmax 取值循 R6 规则 4(全文件扫描)。归 RNB-4。

## §29.91 RNB-2 显示诚实批收账(2026-07-15;六件全交付;h2/h6 PASS,突变 M1-M11 红)
**件1 W3 病②**:smr1 case-A fallback 三缺全补——anchored≤0 不发指针/候选值校验门(µs,含 clipped 臂二次校验)/≥2 通过候选 fail-open(取最大 tiebreak 退役);mirror 臂捎带 MergedChainAnchorMemberAccounts 禁入。**件2 W3 病①**:R2 分组键补 anchorForm 叉键(余段/剪切/降道三形,循 RNB-1 rank fold 判例;E32 形 9.272+0.478+0.893 同键混并根治);merge body 取清账臂=清三元组五字段+「同源二分账留在各成员(种子成员账)」限定词;trunk ×2 fold 三形禁入 fail-open;Σ 重导出臂留待引擎成员级 disjoint 凭证(残留③)。E7 具体案复放待客户 ftrace(机制已修)。**件3 W3 病③**:anchored≤0 括注「(⛓链上席)」→「(无锚定段)」zh/EN+引擎姊妹句分叉,与 RNB-1 twin 臂正交(零值形胜出);残留=(0,tol] 尘埃括注留观。**件4 ELIM-SEM 方案A(用户 R1)**:◎ 链上语义保底席(◇最大对称臂,TOP5 无语义持席→追加最大一行;多类=单一最大席+计数披露);W4-a ◇ 语义计数脚注+E30 ⛓ 对称脚注;✦ 不可达定理补注;engine-real 三窗 witness——**锚点漂移如实注**:RNB-1 降道改盘后触发形在仓内双 fixture 无自然 witness(语义席现恰 #5 在席),保底臂由合成 pin 承载,真机触发 witness 待客户 ftrace。**件5 AFF-EVID(§29.88.6)**:CPUConstraint typed payload R2' 七处(tracediag hash→2cf7c2a4);ExcludedCPUs=允许集 vs 全域观测核对照(R5a 消费面预留,字段注写明);显示行3「CPU约束描述:允许核 0-1,3-11 · 排除全域观测核 2,12-13 · cpuset组 · 策略 · 判定依据」;证据收窄=决定性事件(W5 E29 行1308–27460 全窗充数→单行 1307-1307);「核类面无法表达排除」立案缺口由排除集词面直接闭合。**件6 P4**:「锚定」双值同句实锤可达(卫星行级账/多组 stamped 席账 vs pid census)→精确词面 fork「本席锚定X…pid全窗锚定账Σ」zh/EN,µs 相等对字节恒等负向臂。
**残留**:①件4 真机触发 witness 待客户 ftrace(或裁注入合成窗);②尘埃括注留观;③Σ 重导出臂待引擎凭证;④R5a 核档比较随 RNB-4 R6 落地;⑤明细面 E# 行区间显式词面=显示扩展候选。

## §29.92 立案 STREAM-WAIT:推理模型首字节等待不足+空流诊断贫瘠(2026-07-15,客户 witness=MiniMax-M2.7 双失败转录)
客户问「是不是等模型等的不够长」。定谳=两形分治:**形A「context deadline exceeded」(心跳 30s 后死)=是,等得不够长**——流式首字节看门狗默认 40s(providers.yaml `stream_first_byte_timeout_seconds` 未配→defaultStreamFirstByteTimeout=40s,openai.go:88-94),设计假设「典型首字节 100-500ms」对推理模型(MiniMax-M2.7 网关不流式吐 reasoning,首 chunk=全部思考完成后)不成立,analyzer 大 prompt 下 40s 必杀活请求。**形B「empty stream — provider closed before any chunk」=等待无用**——provider 侧 EOF(拒绝/过载/降载),但错误面无 HTTP 状态码/request-id/响应头(openai.go:1164 裸句),客户无从对质 provider;重试无退避连打同一网关。修向:①首字节默认提到推理模型安全档(≥180s)+SSE keep-alive 注释(:1029 现仅 log)计为活性重置看门狗;②形B 错误富化(状态码+request-id+安全截断 body,api_key 零泄漏)+重试抖动退避;③knob 文档面(providers.yaml 两超时+重试)+失败知会词面区分「服务端未开口(可调 stream_first_byte_timeout_seconds)」vs「服务端拒答(换模型/查网关)」;④心跳知会行带当前 deadline 值(用户可见「还会等多久」)。

### §29.93 用户裁定 R8+立案 ELIM-SELF:目标线程自身恒为链上;◎ 板自身可消除量全族排查(2026-07-15)
**用户裁定 R8**:不特殊说明,**用户(目标)线程自身恒为「链上」**——自身席位的通道语义定死,与 R4 的自身 carve(SELF-SEM/SELF-ALL)一致并升格为通则:任何面(◎ 板/树/榜/喂入)自身行不得落 ◇/▒ 通道;症状面排除(如 sleep 症状行「非可消除量」不参与汇排)是**可消除性**判定非通道判定,保留但词面必须说清是症状排除而非通道降级。
**立案 ELIM-SELF**:排查 ◎ 窗内可消除量总览是否漏收自身可消除量——已知:target_self_state 症状行排除有脚注(合理);自身 D-state/runnable 墙钟席在榜(witness 实证);**疑漏**:①自身 running 折算可消除量(目标自己低频/小核运行的供给缺口,SELF-ALL §29.61.2a 说 running=供给折算——引擎是否铸自身 running 席?铸了是否达 ◎?);②自身 IO 族;③自身语义席(RNB-2 保底席已覆盖链上语义,自身形复核);④elim.go:909/:1032 两处 IsTargetSelfStateRow 排除位的射程是否溢出症状面误伤可消除行。全族 census(引擎铸造 lane×◎ 准入)+witness 实证,漏收即修。

### §29.93.2 用户裁定 R9:折叠行行1 标签瘦身,成员预览下沉行2(2026-07-15,witness=「◦ 其余 3 项(折叠)(WifiHandlerThre-12073(见榜位#3)…」挤压 bar 失栅格)
折叠行(「其余 N 项(折叠)」族)行1 标签列被内联成员预览(含「见榜位#N」注)撑爆,bar 挤出栅格。裁定=按语义行形制(「语义─ ❶ ✦ 类校验 8次」短标签+成员行下沉)瘦身:**行1 只留 `◦ 其余 N 项(折叠)`**;成员预览+榜位指针下沉行2(`· 成员 <名>(见榜位#N) · 其余 M 项见明细`,复用既有成员行家族形),其余从属行(取最大口径句/同段镜像句)不动。信息零损只换行;适用全部折叠行发射面(链上─/背景─/◇ 区同族一体);「bar 栅格对齐」与 R7a「bar 区无行间子行」同族=显示栅格纪律。归下一显示批(与 ELIM-SELF 修复同批,避开在飞 RNB-4 的 tree 面)。

## §29.92.1 STREAM-WAIT 收账(2026-07-15;四件交付;假 provider 矩阵全绿;突变 M1-M3 红)
**件1 形A**:首字节默认 40s→**180s** 双席同步(「典型首字节 100-500ms」假设注释重写为诚实形=推理网关持有全部输出至思考完毕);SSE keep-alive/注释/空行计为活性重置两相看门狗(gotAnyChunk 空流判定语义零动),总墙钟帽 2×requestTimeout 钉死心跳永续;ResponseHeaderTimeout=firstByte 单 knob 三段分工注释落档。**件2 形B**:typed StreamEmptyError+哨兵(StatusCode/RequestID 白名单头/CommentOnly/BodyPrefix≤512B);三形词面(非200/200立即EOF/200注释后EOF+非SSE JSON);凭据红线结构化(redactCredential+双负向 pin:服务端回显 key 进 body→错误文本必不含)。**件3**:现状实锤=客户「2/4」计数为 L4 transient 车道零退避 requeue,且空流旧形不在任何重试白名单;修=ErrStreamEmpty 进 L1 白名单(零回调可证安全)+全抖动指数退避 uniform(0,min(15s,1s×2^n)](形A 已等满窗零额外退避)+知会行带退避时长(delay≤0→「立即重试」替代伪造「等 1s」)。**件4**:失败知会分形词面(未开口可调 knob/拒答带状态与 request-id);心跳行「已 30s / 首字节上限 3m0s」全链接通(Event.WaitDeadline+telemetry Reporter 三 adapter+agent 发射一行)。
**残留立案**:①**STREAM-WAIT-2(疑真凶/合谋)**:`internal/tool/analysis_limits.go:373` TerminalEmitOnlyRequestTimeoutSeconds=45——analyzer 终态 emit-only 请求 45s ctx 斩杀,与客户「心跳 30s 后 context deadline exceeded」更吻合,180s 看门狗救不了 ctx deadline;tool=在飞批领地,RNB-4 落地后即修(≥180s 推理档);②L4 requeue 零退避(orchestrator 小改)候补;③types/providers.go:181 注释宣称 20s 默认=双重失实,随①同批;④repl 终态词面分形候补。

### §29.93.1 ELIM-SELF 排查结论(2026-07-15;用户怀疑完全证实;修复依赖 RNB-4,排其后)
**核心**:◎ 板确漏目标自身可消除量,漏收面=**自身 running 供给折算缺口**,病根 100% 引擎铸造/容量层,◎ 显示四臂对自身席零误伤(F-4:两处 IsTargetSelfStateRow 排除位只射症状面,tier 铸造闭集结构上不可能误伤)。**Form-1 铸造缺席**:全引擎唯一自身 running 铸席通道=depth-0 impact 例外臂,而 interestingIntervals 显式跳过 running 段(query.go:20737)→常规窗零铸造;witness:donghu 17267 旗舰窗自身 running 157.248ms would-be 缺口 **58.320ms=◎ 首席 7.081 的 8.2 倍**,2955 窗 65.912,tieba 头窗 9.546——全部铸席 0;二阶伤害=compute_supply 卫星 running-dominant 抑制臂(:13974)前提「causal running 席已代表」对自身不成立→自身低频 running 全 lane 无出口。**Form-2 帽斩饿死**:退化窗(无跨线程链)自身链上席被通道盲排序+候选帽全灭出空板(tieba 头窗预截断池 82 行自身三席在 18/20/24 位,12 条 ▒ 背景行占满帽而它们全不是 ◎ 种群)——RNB-1 D1 同型死法,自身席未享侧车道保护。**修向(排 RNB-4 后,=ELIM-SELF-FIX 批)**:①新铸自身 running 缺口席(窗口投影+fold 机械,继承 RNB-4 新基准,basis=self_wall_clock_interval,R8 通道定死链上;删/收窄 :13974 抑制臂对自身适用;ORD 单席闭合防双算);②selfSide 有界侧车道(照抄 D1 demotedSide 模式,cap 4);③P-4 wire 面自身症状行 rel=adjacent 残口=R8 词面裁定级非 bug,随批收。V-2:全部 would-be 数值按现行基准,RNB-4 落地后验收锚点重算。R9(§29.93.2 折叠行瘦身)同批。

### §29.93.3 ELIM-SELF-FIX 验收补钉:自身全族收编 pin(2026-07-15,用户追问定案)
用户追问「除自身 running,runnable/IO/D 是否一并收编」——排查 F-3 已实锤三族+语义自身席在常规窗全数在榜(witness:2955 自身 D 36.757=⛓首席/自身 runnable 1.347/17267 io_latency 3.264/17284 jit 2.388),唯 running 缺铸造 lane(Form-1)。但 **Form-2 帽斩是全族共病**(tieba 头窗预截断池自身 runnable#20/io_wait#24/running#18 一起死)。ELIM-SELF-FIX 批验收补钉两条:①selfSide 侧车道=**全族保护**(四族+语义,非只 running);②**全族在榜恒等 pin**:任一自身族持值(eff>0)席在引擎预截断池在场时,◎ 板(或其保底/侧车道/脚注 lane)必有对应可见形——零静默消失不变式的自身特化,常规窗+退化窗双 fixture 钉死。sleep 症状面照旧脚注排除(非可消除量,R8 词面合规)。

## §29.94 RNB-4 旗舰批收账(2026-07-15;R6 核簇四规则+R5 单基准单算法+R5a/R5b 提及义务;双复核零 P0;金样 h1..h6 6/6)
**件1 CLUSTER-DERIVE 修根**:病根=旧 `inferCoreTopologyFromFrequency` 按排序位三分噪声推导(14 核 i≥9 切 big→9-11 误归;tieba 从全等 fmax 捏造三类词)——整函数墓碑退役,权威=R6 四规则(`deriveClusterFreqDomains` 首簇+区间闭包成员本体;向下继承臂退役=跨簇 gap 核诚实不判归;规则2=逐曲线 emission-identity 比对非 fmax 假实现,对抗官 MUT-R1 七 pin 红证);规则4=`full_freq_curves.go` BuildIndex 单趟顺扫全文件采集(窗门/剪枝/MaxEvents 前喂入,完整性精确门+anchor-cache 复用,70MB 实测零性能回归);donghu 真值 pin [0-3][4-11][12,13]+fmax 1720000/2270000/2750000@12/13,tieba=[0..5] 单簇零类词;对抗官独立 Python 探针直读原始 fixture 复推全中。**件2 单基准单算法**:`weakCoreDeficitMs`(按下游消费核)整函数删除,gated running 分量=同一 fold 缺口 by construction;基准=`supplyFoldGlobalMaxBasis` 全域最大核最高频点(全文件 ladder limit>observed>rail,降级走梯退役,freq_only=全域最高频×1);witness 席 6.972/7.296 两数→**7.405=0.109+7.296 单数**;keva-3 2.160→2.286 收敛;51.735/7.939 恒等经对抗官定谳=算法真换但该席两代同值(窗≈全文件+旧误归簇为含真顶核超集),历史窗分歧 pin 7.94→8.596 为证;冷读官 4 席守恒手算闭合(缺口=原始−ideal,计入==缺口)。**件3 提及义务**:R5a 按核档 typed 对(AllowedMaxTierKHz/GlobalMaxTierKHz,R2' 七处,tracediag hash 2cf7c2a4→1d78e2fd),9655 mask=ffb 正臂「绑核排除更大核档(2270000<2750000)」在席,tieba 双负臂+含顶档/档不可判禁铸;R5b「运行频点非最高」单词源骑乘全部非零缺口词面,零缺口禁词 pin。词面族退役 grep 零活体;簇词全局一致(无旧三分词残留);金样 6/6+h2/h4 冷读独立复跑 PASS;突变批 8+复核 5(R1/R4/R5 红,R2 合法吸收,R3=P1 洞)。
**双复核问题清单**:P1-1 完整性精确门无 pin(MUT-R3 存活绿;行为正确+人工核对,合成 seek/early-stop pin 下批补);P2-1「簇结构不可判,按纯频率比折算」capability 口径 clause 活体存续(语义自洽=披露核类差未计价非基准词,处置=账本本行说明留观);P2-2 单基准 pin nil-basis 跳过软点(改断言 basis 非 nil,下批);P2-3 51.735 锚点无 pin(补进 engine-real 族,下批);陈旧注释若干(P2 卫生)。**待裁两条**:①fmax 梯级 limit>observed 在整文件被限频压低时取被压顶棚(现继承 policy-authority 判例)还是观测峰;②「接近全域最大核最高频」词形只看 1.0ms 绝对地板不看相对份额,与「运行频点非最高」同句自我张力(前置病被 R5b 显性化)——建议相对份额守门。

### §29.94.1 事故记录:autostash 冲突标记被 add -A 静默入库(2026-07-15,已修复)
§29.94 收账时 autostash 与另一会话代码批(perf codec/parser self-audit,同触 parse.go/trace_query.go/render_key_first.go 等 7 文件)3-way 冲突,parse.go 残留冲突标记被收账复合命令的 `git add -A` 当「已解决」暂存,红树推上 main(3a419733 净室 EXIT=1)。修复=双边保留(对方 blockedReason capped 克隆行+RNB-4 fullFreq 传递行语义正交),全套门绿后推送;其余 6 文件为无标记干净 3-way 合并,净室复验。**纪律升级(第三次 git 汇流事故,终局范式)**:收账顺序改为「先 commit WIP → 再 pull --rebase(冲突会响亮停住,push 不会发生)→ 再 push」,禁止 autostash 收账;任何 add -A 前必须 `grep -rln '^<<<<<<< ' && git status | grep UU` 双零检查。

## §29.95 ELIM-SELF-FIX 收账(2026-07-15;Form-1/Form-2/R9/riders+双复核+修复轮;金样 h1..h6 6/6+clean-HEAD h6 对照;突变 M1-M6 全红)
**件1 Form-1**:自身 running 折算缺口席新铸(`rank_self_running_fold.go`:窗口投影+supplyFoldRunningIntervals 单一 R5 fold,basis=self_wall_clock_interval,R8 通道定死链上,deficit≤0 不铸;ORD 单席闭合=depth-0 例外臂同线程 running 禁铸+RootEvidence twin 保种;low_frequency 抑制臂收窄为精确 presence 位)。锚点(手算+对抗官独立复算 µs 全等):17267 旗舰窗 157.248/98.928/**58.320 领跑**(修前首席 7.405 的 8.2 倍收编)、2955 窗 65.912 领跑、tieba 头窗 9.365、h1 窗 9.546;新旧基准四窗同值经逐窗核实=诚实巧合。**件2 Form-2**:selfSide 侧车道 cap=4(全族谓词=主体+通道+eff 零 token 手抄),tieba 退化窗空板→全族四行 ⛓ 板;三侧车道独立帽互不侵占实测(8/8/4,sideTotal 诚实)。**件3**:全族在榜恒等 pin 双 fixture(引擎+显示双半场)。**件4 R9**:三发射面行1 瘦身+成员下沉行2+头名保护 floor 退役(构造保证)+bar 栅格 pin。**件5 riders**:RNB-4 完整性门 pin(windowed early-stop fail-closed)/nil-basis 断言/51.735 锚 engine-real pin/陈旧注释修(工单三处行号漂移如实纠错)。
**双复核**:对抗官独立复算全等+假 pin 猎杀三红+h6 FAIL 归因转录实证;冷读官渲染 witness 复做+守恒手算+叙事素读通过(「目标自己跑慢=最大可消除量」自洽,折算括注防混算)。**修复轮四件**:P1-1 ORD 闭合 pin(M5 红=got 2 双席形逐字吻合)/P2-3 R9 stanza+legacy 面 pin(M6 红)+EN 双括号臂/P2-2 clean-HEAD h6 A/B(净室 PASS;结构论证=FAIL 在 explore 首轮工具表装配点,本批 diff 零触调度车道,B5 字节恒等)/UX-1 裸 state 行修根(去重臂改键名值相等,nameIsStateWord 旗退役,runnable 字节恒等)。
**勘误**:§29.93.1「tieba 头窗 9.546」应为 9.365(9.546 属 h1 窗);批报「2955 ▒ 6→5=R8 涟漪」订正为 OS_FFRT_1_11-9427 被证据帽挤出(既有帽行为)。**裁定池(待用户)**:①selfSide cap4 vs §29.93.3 全族(≥5 族)零静默消失张力——主会话默认倾向 cap→6+结构 pin;②自身 page_cache_churn 通道=R8 vs §29.83 恒邻近冲突(现状 wire=adjacent+树面 ◇ 区+邻近席互指,板面=⌗ 旁栏)——推荐 (a) 计数当量自身行改口径旁栏词面(非通道语义)。**显示批候选**:SELF-stanza 面机制句补挂(最大席最寡言)/双 running 行互指注/家族限定词覆盖不齐/R9 行2 M 数口径。**环境纪律**:突变复核期禁并发取码构建(冷读官 06:59 二进制落在对抗官突变窗内,血统可疑经手算+pin 双源排除实害);LLM prose 面 1.53GHz 挂错簇=PROSE-RC 既有战场 witness 备查。

## §29.96 STREAM-WAIT-2 收账(2026-07-15;analyzer 终态 45s 真凶修+承诺面 sweep+L4 四车道退避;突变 M1/M2 红)
**件1(客户 deadline 真凶)**:`TerminalEmitOnlyRequestTimeoutSeconds` 默认 45→**180**(EVOLUTION RECORD:旧假设「emit-only=短结构化发射」对推理网关不成立——终态 emit 重新进入完整思考期,45s ctx 在 180s 首字节看门狗之前先杀=客户死状);httptest 1000× 压缩重放死/活双证(45 单位死=DeadlineExceeded 同形,当前默认活且 emit_analysis 完整到达);config 默认表+codrax.yaml.example 同步。**件2**:types/providers.go「20s default」谎言删除+承诺面全 sweep(providers.yaml.example 三处 40→180/user_guide 样例),锚写常量名禁写死数字。**件3**:L4 四车道接 `llm.NextRetryDelay` 同源退避(read_stage_retry 双车道+**orchestrator runAnalyzePhase=客户「2/4」行真发射点**+write controller plan 车道);deadline 类新增 →0 臂(形A 已烧满窗零额外罚等);知会行带时长(「等 3s」/「立即重试」与 L1 同舍入形);L5 force-finalize 顺手接通。**全链 <180s sweep**:核链唯一先斩点=件1(已修);诚实残留=stall 120s(思考块间静默>120s 且网关流式吐 reasoning 形,knob 可调)/repl 10s/60s 分类器档(60s 动之需重开 DR 裁定)/repl 终态词面分形候补。附带:orchestrator LOC ratchet 满格下净增 0 交付(退役 pre-Go-1.21 min shim 死重)。
**队列勘正**:R3(§29.88.1 非目标语义 span 边锚定)裁定已落账但**尚无批实施**(SCAN-3 哨兵对两窗 HEAD 均 0 语义席+fail-loud caveat=铸造缺席)——立 R3-IMPL 批(边前段铸链上席/无边留 ◇/跨边二分,验收=哨兵对),tieba 哨兵金样封案依赖其先行。

### §29.96.1 用户裁定 R5c:全域最大核最大频点=一切定义源取最大可能值(2026-07-15,终判 §29.94 待裁①)
全域(所有核)最大核最大频点的取值:**关注所有能定义核频点的信息源**(实测 cpu_frequency 样本、cpu_frequency_limits 限频顶棚、clock rail 等 typed 证据车道),**按最大可能的频点计算**——含限频顶棚大于所有实测频点的情况(核从未实际跑到限频顶棚,仍按顶棚算)。语义=各证据车道取 max(),取代 RNB-4 的 limit>observed>rail 优先级走梯:①limit>observed 形两者同解(取 limit);②observed>limit 形(整文件被压/limits 稀疏迟到)按 max()=取 observed 峰——即 §29.94 待裁①终判为「最大可能」而非「被压顶棚」。全文件扫描口径不变(R6 规则 4);幽灵核排除不变(无任何证据车道的核不参赛);throttled/热限事实照旧走披露车道不改基准(R5b 族)。消费面=supplyFoldGlobalMaxBasis 走梯改 max()+各簇 fmax 同法+来源注記词面(「(observed)」类括注按实际取值源改写);donghu/tieba 现真值 pin 预期不变(donghu 小簇 1720000=limit>observed 两法同解;顶簇 2750000 无 limits;tieba 无 limits)——实施批以 pin 复验证实。归 R5C 小批(R3-IMPL 收账后,同包避撞)。

### §29.96.2 裁定池 11 项终判:全部按主会话推荐执行(用户 2026-07-15;旧窗遗留议程维持不动)
①selfSide cap→6+合成 6 席结构 pin;②自身计数当量行口径旁栏化(树面撤 ◇ 区/wire 非通道 typed 位/互指词改口径旁栏行——R8 管因果通道、§29.83 管量纲,两全);③「接近最高频」双门(绝对<1.0ms ∧ 相对<15%);④D2 VIEW 行降道判据细化行级(行区间∩锚窗=∅;无区间沿 pid 级保守);⑤周期臂跨窗 Σ 维持(结构证明可加)+口径披露句+合成双计诱错 pin;⑥「单次最大」词改 wire-fold typed 来源位判定(数值巧合不触发);⑦微值锚定席折叠阈(链上剪切席<0.1ms 折叠一行,凭证语义保留);⑧composite wire 字段名不改名关账+digest Unit 字段发 caliber token;⑨多窗合并行 chip 不可解补「·多窗(端点见明细)」;⑩stall 120s 留观不动/REPL 单发路由 60s→120s(DR lane 结构不变只调值);⑪投影树通道词着色(复用 ELIM-CHAN token-臂;空链行不着色维持)。默认小件随批:SELF-stanza 机制句/双 running 互指注/家族限定词补齐/R9 行2 M 口径同源。执行分批:RNB-5A(tracequery 面:①⑤⑧каliber-token+R5c §29.96.1 fmax max()+refCap 死字段清理+⑩repl 值)、RNB-5B(显示面:②③④⑥⑦⑨⑪+默认小件),R3-IMPL 收账后按序开。

### §29.96.3 立案 STYLE-1:答案去 AI 味(用户 2026-07-15;安全优先/不硬卡/不伤精度)
用户诉求:模型回答人性化,去 AI 味,前提=不破坏答案精度与准确度、不硬卡、安全优先。**方案(全软引导,循「嘈声信号只作软引导」红线)**:①finalizer/答案 skill 加文风指引节(zh 主):工程师直述口吻、结论先行、短句直给;点名退场 AI 腔套词(「值得注意的是/综上所述/总的来说/让我们/深入探讨」空洞承接、总分总模板、重复复述问题、第一人称复数带读者、空泛评价句);②**事实车道围栏**(方案安全核):数值/口径词/图例词/E# 引用/typed named-fact 逐字引用纪律原样(PROSE-RC 族),风格自由度只作用于连接组织层——既有确定性防线(quote 回填/prose 回查/金样 oracle)全部不动,继续做事实面的硬门;③AI 腔词表 advisory lint(仅日志/eval 观察列,永不拦发布——嘈声信号禁硬门);④验收=金样 h1..h6 全扫(事实 oracle 不变)+双窗 before/after prose 冷读对照(人判风格,不自动化)。**明确不做(安全反面)**:系统事后改写模型文本(违 §29.42.4 答案权属+事实腐蚀风险)/风格硬门(嘈声硬卡必伤结构性好答案)/二段 LLM 重写(paraphrase=满额病同源)/采样温度调参(不可控)。prompt 改动过红线 checklist(BLOCKING)+删改指令三步走。

## §29.97 R3-IMPL 收账(2026-07-15;非目标语义 span 边锚定铸席+双复核+修复轮五件+refCap 死字段清理;金样 6/6+复核独立 h2/h5/h6)
**铸造 lane**(`rank_semantic_edge_anchor_r3.go`):凭证=①直接:WakeupEdgeCensus 宿主→目标裸边(target-wakee 帽免疫)②多跳:chain.Edges 宿主自身出边(凭证沿链传递,方向核 Waker==host);边界=最晚窗内凭证边;跨边二分(⛓ 席带 RSPA 双账+◇ 余段克隆复用同源二分三元组=二分句/侧车道/键叉/twin 全既有管线);无边/边后整段不给席(family mint fail-closed);SELF-SEM 防御性排除;causality=on_wakeup_chain(真边,区别 self 族);OverlapMs=0 不伪造重叠。R2' 七处(tracediag hash→cc99c4fd);顺手修真:语义余段权属词「(⛓链上席)」→「(✦链上席)」。**哨兵对四臂**(SCAN-3 原文):正臂铸 0.285(anchor 34579.496810 µs 精确,行5639)/负臂零席 caveat 原样/0.4ms 翻道双向 pin/诱错(跨他人边)零席。
**双复核零 P0/P1**:对抗官边几何原始 trace 全量枚举复核(8 边逐 µs 全等)+过度铸席猎杀(边后 span/入边方向/census belt 两层纵深)+手算「窗内投影」语义(席 1.000/余段 4.000 窗内截断非 14)+h5 首跑 FAIL 归因证据链无洞(比批更强的板输入恒等独立证明)+五突变全红;冷读官 R4 排他 grep 级验证(语义上链恰三臂无暗门)+哨兵渲染 witness 复做+SELF-SEM 回归零变。**修复轮五件**:「最近→最晚相关边」词面(+反向 pin)/zh 句去 EN 枚举(凭证=直接裸边/链上跳边,单源词表 fail-open 透传)/家族跨边合成 pin(pre+post≡union 分割恒等式+克隆恒等)/✦ 权属词 pin(state 负向)/退化窗 inWindow fail-closed(+M5 突变红)。**refCap 死字段清理**(任务卡):字段+写入点+coreCapabilityReferenceClass 常量+死断言全退,same-cluster 规则注释重锚 supplyFoldGlobalMaxBasis(唯一执行点)。
**残留**:跨边二分/多跳/◇语义≥3 真机 witness 待客户 ftrace(合成 pin 承载);语义余段席无 twin-unpublished 改写注(P3 观察,毗邻批残留③);◎ 板边锚定席资格 chip(RNB-5B 默认小件已列);N1 pin 按裁定 EVOLUTION(链上节点宿主 E21 adjacent 形近绝迹,负臂活体在钉);h5 prose ×N 词形敏感=既有 EVALFIX 族。

## §29.98 RNB-5A 收账(2026-07-15;终判①⑤⑧⑩+R5c;金样 h2/h4 PASS;突变 M1-M5 红)
**件1(①)**:selfSide cap 4→6+六席全族溢出结构 pin(突变回 4 实录恰两席静默消失=张力实锤后关账)。**件2(⑤)**:披露句「(跨窗周期合计:逐次折减相加,与行值去重口径分账)」typed 门发射(单窗零漂移负向 pin);**诱错实验坐实双计**——合成两窗重测同一 occurrence 形:值通道 union 正确去重(66.000,引擎自证同段),周期 Σ-有效车道全员相加 0.090(唯一合计 0.060)——「逐次折减不重叠」结构证明在此形上被违反;哨兵 pin 钉现行为+再裁指引,**新裁定项:是否给 Σ-有效车道加跨窗同段去重(主会话推荐=加,与值通道同源 union 口径)**。**件3(⑧)**:digest Unit 字段发 composite_score caliber token(registry 精确门恰一 token;显示 zh/EN 循 QH2-A 词面家族;coverage/prose 回查/跨包恒等 pin 三消费点同步;legacy 重解析 ms 谎不可达论证入档);wire 字段名零动=终判关账。**件4(R5c)**:三走梯(supplyFoldGlobalMaxBasis/cluster_ceilings/coreCapabilityClusterFmax)改各证据车道 max()(平手按权威序只定归因不改值);donghu/tieba 真值 pin 复验全绿零改(§29.96.1 预期兑现);反向形 pin(observed>limit 整文件被压→取 observed 峰,缺口 7.29→7.94 重算);throttled 披露改值无关事实门照旧披露。**件5(⑩)**:REPL 单发路由默认 60→120s(DR lane 结构零动;codrax.yaml.example 补录该 knob 此前缺席;默认值 pin 新增)。

## §29.99 RNB-5B 收账(2026-07-15;终判②③④⑥⑦⑨⑪+默认小件+双复核+修复轮;金样 h1..h6 6/6+h4/h5 终树复跑;突变 M1-M10 红)
**七终判件**:②自身计数当量旁栏化(wire self_caliber_side 精确门=SubjectIsAnalysisTarget∧Count 类;树面撤 ◇ 佩 ⌗ 词族;互指第三形「口径旁栏行」;桶帽折叠豁免;**产线修根**:17267 自身页缓存 81.616 计数当量曾在 ◇ 折叠行假冒墙钟 max「81.616ms 35%」出榜,修后独立 ⌗ 行+折叠行诚实缩 4 成员 max 2.388+症状分母 census 裸 ms 同堵);③「接近」双门(绝对<1.0∧相对<15% 专用常量对;66%/83%/32% 席褪词,3% 保留);④D2 行级凭证(hull 空交 sound=∅ 判定永不误伤;hull 有交保守留道=终判原文,keep-⛓ 逐段凭证化入裁定池);⑥「单次最大」typed 来源位(MergedWireFold 恰在 folded_rows 再物化铸;巧合负 pin;µs 恒等降一致性守卫+pin);⑦微席折叠阈 0.1ms(2955 三微席→0.060 合计行;跨线程「账目合计」词面划界+图例「账目相加非墙钟并集」句;被折席锚定值经 ◇ 孪生二分句逐席可达实测;N=1 不折);⑨多窗端点词「·多窗(端点见明细)」;⑪树面通道词着色(token 臂座次形恰识别,DOM 量测 14/14+8/8 精确同数,textContent 逐行恒等,双主题纯 color)。**默认小件**:SELF-stanza 机制句(58.320/65.912 席带 R5b+热限句)/家族限定词全员补齐/◎ 边锚定 chip;件b(双 running 互指)停素材面=ORD 闭合后无活体形;件d 核实已同源。
**双复核**(对抗+冷读,零 P0):对抗官 witness 逐字节独立重建+parser 死状复现+hull 几何 sound 论证+假 pin 猎杀 4 红;冷读官 DOM 量测+守恒手算+D2 字形偏离(⌗ 行掉 ⛓ 兜底=通道谎+不可中断误述)。**修复轮十件**:P1-1 census 漏计修(fold 行按 Depthless 成员数计,21 复原,MAX 只以单席参赛;M9 红=「want 21 got 18」病形原文)/P1-2 h5 终树复跑 PASS/D2 ⌗ 行首字形(默认裁定=专属 ⌗,◇ 道 ⧗ 状态谎同族同修,图例+双向 probe)/P2-3 composite 引擎级 ChainRelevance pin/P2-4 图例诚实射程/U1 阈值常量格式化/U2 图例补句/U4 全员一致态词存活/U3 折叠席位记忆「根因排序: #5~#7(折叠合一)」(全员持席才发宁缺勿假)/P3 注释勘正+µs 守卫 pin+计数族 ms 后缀两面清(行3 等式+表值格,QH2-A count 姊妹臂,负臂墙钟行字节恒等)。
**裁定池新增**:hull keep-⛓ 逐段凭证化(对抗官建议=候选行携逐段区间使 keep 侧也凭证化);「(或接近)」deficit==0 车道 hedge 词族(U8);(b) 块「单段 …ms」计数值后缀族发射器(卫生)。**残留**:B5 教学缺席(self_caliber_side LLM 面)/件⑦ 徽章后果素读已由席位记忆缓解。

## §29.100 STYLE-1 收账(2026-07-15;§29.96.3 方案全落;金样 6/6+oracle 零字节 diff;红线 ATOMIC 逐条勾账)
**件1+2**:answer-document-skill 加 Voice register 节(全引导式零硬门:结论先行/短句直给/点名 AI 腔套词退场/禁自我指涉;与既有 connective-tissue/结论先行规则显式回指对齐)+事实车道围栏(回指衔接形,不铸第二套 verbatim 规则:数值/口径词/图例词/[E#]/named-fact 逐字纪律原样)。**件3 advisory lint**:词表单点常量 AnswerStyleFillerPhrases(10 词,skill 例词同源渲染 SST pin);发射=recordTaskFinalize WARN 行(hits=0 零行)+eval style 观察列;全仓 grep 证明恰两处软面零 gate 零回喂;无 hasHan 门(走私面教训);LOC ratchet 连带按指引拆 record_task_finalize.go。**验收**:金样 6/6(oracle `git diff eval/cases`=0 字节=精度零损机械证明);双窗 before/after LLM 全流程对照(工件已交用户区 customlogs/style1_prose_ab_comparison_20260715.md):事实 token 零漂移(donghu 全数值可溯源/tieba 席位表 #1..#7 逐字恒等),**PROSE-RC 既有防线活体捕获**(after 趟模型自加得「理想值 34.8」被确定性尾注披露=事实硬网在岗证明);风格 n=1 冷读=导语更结论前置/短句密度升/tieba 转分节报告形,但「### 结论」收尾节残存=软引导不一击消除结构性 AI 形(如实);lint 四趟全零=该答案族 AI 味主要以结构形存在,词表保持纯观察定位正确。**残留**:REPL-local 面无 advisory/WARN 阳性无真机见证/其他 sweep 汇总列/「### 结论」收尾形+词表扩充=风格续批素材(观察数据驱动)。

## §29.101 SEAL-1 收账(2026-07-15;金样封案 h7/h8/h9,各 2 趟 6/6 PASS)
三窗封 sealed case 入 eval/cases/real_traces/:**h7** `real_trace_h7_self_seat_full_spectrum`(donghu 2955 全窗:自身 running 缺口 65.912 领跑+自身 D 36.757/dma_fence 次席+logd.writer 0.018/49.638 二分形+微席折叠行——按当前 HEAD 实况封 **5 席合计 0.094**(简报预期 3 席 0.060 为 RNB-5B 中间态形,case 头注 EVOLUTION 说明;被折五席逐席经 ◇ 孪生二分句可达实测);看护面=ELIM-SELF/R4 重锚/微席折叠合谱)。**h8** `real_trace_h8_semantic_edge_anchor_sentinel`(tieba R3 正臂窗:类校验 0.285 链上语义席+·边锚定 chip+凭证句「最晚相关边 34579.496810s,凭证=直接裸边」;两趟 E# 位移 E9→E10 实证禁序数纪律);**h9** `real_trace_h9_conversion_single_basis`(17597 窗:链首 51.735 单折算数+「按全域最大核最高频」词+keva-3 3.309=1.023+2.286 构成句+「按下游消费核」负 pin=0;「接近」正臂该窗无活体留 SOFT)。oracle 全部词边界形+值出处实跑行引用;私有二进制快照防并发重建。**残留**:h9 趟1 模型 prose 把原始 143.499 误称折算席并自行相加=PROSE-RC 既立案战场取证素材(typed 板面正确);「接近」正臂 pin 需另窗承载。金样族现 h1..h9 九案。

## §29.102 QH2-B 收账(2026-07-15;「全额→满额」口径词 paraphrase 修根,PROSE-RC 族;金样 h2×1+h9×3 PASS;突变 M1-M5 红)
**Table ③c 单点常量**:发布口径词闭集 {全额,折算,下界,原始,计入,单次最大}+未发布近义词表 {满额,足额}(扩词仅限 witnessed 近义,AST sweep 看守)。**件① 喂入绑定**:席位构成 lead 增口径词绑定导语(口径词与数值同为具名事实,连词带值整体照抄,禁未发布近义词替换;INV-SUPPLY 反压缩姊妹句纪律);裸喂审计=各事实面已带绑定词,缺口仅 lead 未立词面为引用单元,补齐。**件② 回查臂**(advisory appendix 专道,检测→披露禁硬拦):臂A=满额类未发布词贴数值直判(词表 membership 精确信号只驱软道);臂B=发布词与值全证据面配对全不合才披露且只陈证据侧事实;配对=贴身 leash+口径括注链(CaliberFull 语法)+最近 token 归属+方向连接词+否定守卫。**件③ 加法臂**:146.899 素材形此前双形皆逃逸(「席」无单位不进扫描/ulp 容差静默)——席-decimal token 扩展后合成 pin 生效,**终趟 160732 活体实锤:素材形产线复发,尾注确定性披露命中**;三值自加 pairwise 形=既有 PROSE-RC-5(§29.80)射程,不重复立案。**金样迭代硬化**:前两趟 appendix 各暴 1-2 边缘过报(隔句抢绑/括注链尾超 leash/方向误绑),逐一修根+负向 pin,终趟零误报+恰 1 真披露。ATOMIC 七条勾账齐。**残留**:EN 口径词面未巡(观察驱动)/披露不进 ship-time caveat(如需另裁)/箭头形安全方向漏检注明/三值自加=PROSE-RC-5 候选。

## §29.103 SUPP-TARGET 收账(2026-07-15;§29.90.1 h2 flake 修根;h2 4/4 PASS;突变 M1/M2 红;主动队列清空)
**诊断实锤**(FAIL 趟 20260714-221545 复放):①analyzer 变体两次 emit 均未铸 runtime_targets(entities 在,typed 空);②该趟模型 trace_query 全走 pattern/event_types,cursor 车道亦空;③双空→补采 skip no_typed_target→无树→三 hard oracle 全缺;④**prompt 面根因=analysis_contract 规则11 发射字段清单完全不含 runtime_targets**(486 行教学段在但离清单远,count/trace 快道均不提)。**A 确定性兜底**:补采第 3 级 entities 回退——触发门=双 RequestModel RuntimeTargets 原始计数为 0(有席但歧义=既有故意 skip,回退永不越权负向 pin);精确解析门(末位 dash/尾纯数字 1..7 位/pid 域/名部含字母灭区间形/共享 safe 门/唯一 pid 歧义即弃);溯源 meta.TargetSource=entities_fallback。**B 源头教学**:规则4 trace 快道+规则11 Optional 清单补 runtime_targets 教学(名-tid 形 MUST),pin 防漂移。ATOMIC 七条勾账齐;合成全链 pin(FAIL 趟同形→回退铸席+树+等待对象全铸/不可解析→照旧 skip 字节恒等);h2 4/4 PASS(病变体本轮未复现=flake 本性,A 生效路径合成 pin 承载)。**残留**:纯数字无名形靠 B 教学/答案面回退披露词面候选/真机 fallback witness 待自然 flake 趟。

## §29.104 裁定池五件终判:全部按主会话推荐方向落地(用户 2026-07-15;商用交付标准,惯例流程)
①**周期 Σ-有效跨窗同段去重=加**(复用值通道 union 同段证明基建,同一物理发生折减贡献只计一次,重测折减值不同取席位归属窗份;哨兵 pin 按新行为重钉;禁一刀切清零);②**self_caliber_side LLM 面教学=数据面注解先行**(工具结果文本就地注解「(口径旁栏:计数当量族,非因果通道)」,零 B5 风险;Description 正式教学句留至下次 Description 重基线 ritual);③**hull keep-⛓ 逐段凭证化=做**(候选行携段清单/census,keep 侧逐段∩锚窗核,至少一段真相交才保 ⛓,否则降 ◇+披露;新携带字段走 R2';段清单成本过高时退「(包络级凭证)」诚实注);④**「(或接近)」词汇分家**(零缺口/不可判车道弃用「接近」,改「缺口未检出/缺口 0(样本粒度内)」类自有词,图例随改);⑤**(b) 块计数值 ms 后缀=同法关账**(族发射器按口径 token 分叉,计数行「单段 计数当量X~Y(非墙钟)」,墙钟行字节恒等负向 pin)。**编队**:PERIODIC-DEDUP(①,引擎值面,双复核)→DISPLAY-HYG(②④⑤,显示词面合批,冷读复核)→HULL-CRED(③,wire+R2',双复核)。

### §29.104.1 立案 XLANE:跨车道同段物理时间双发链上全额席(2026-07-15,客户紧急反馈;witness=/Users/han/opt/customlogs/runnable2.txt)
客户指认 ◎ 板 E11(调度延迟 23.471 合计3段)与 E26(runnable 3次 17.635)疑双算,「链上影响没这么多」。**报告文本算术实锤**:E40(◇ 整席降道)自述该线程全窗 runnable=26.725;链上侧 E11(23.471 全额)+E26(17.635 全额)+E28(8.608)+E27(3.608)+E32(0.195)——仅前两席=41.106>全窗 1.5 倍;E11 三成员段(6.997/8.226/8.248)与 E15 runnable 段逐值相同=调度延迟席即 runnable 段换类型词再上榜;两席零互指零互斥(E11 只指 E40)。**语义同类形**:E34(类校验 8次 9.586)与 E35(4次 6.182)前三成员逐字相同(AwContents 2.762 等)=同 span 双语义族发席。**伴生词面 bug**:E29/E32(shadowhook 行)误佩「自身·墙钟席」(自身=目标专用)。排查任务=全部链上 eff 铸席车道 × 物理时间重叠互斥矩阵(调度延迟 vs runnable 普查 vs 反转席 vs 自身 running 缺口 vs 语义族 vs D/IO 普查 vs critical_blocking vs io facets),既有 W-A/同段互指/same-fact 机械的射程缺口定位,修向按「同段物理时间跨车道恰一全额席+其余席互指降口径」方向出素材。

### §29.104.2 XLANE 排查定谳与修复编队(2026-07-15;素材全文见排查报告,要点入账)
**定谳**:①witness 算术=shadowhook 链上 runnable 族 eff 合计 53.5ms=全窗物理 26.725 的 **2.0×**;六对榜位撞号(#1/#2/#5/#7/#12/#13 各×2)=报告内并存 ≥2 块 rank 板。②**根因=缺「同线程同状态族物理时间∩>0 ⇒ 恰一全额席」全局公理**:全部既有值面互斥(B4 same-fact/RSPA 二分/ORD 抑制/W-A/显示孪生折叠)都是「区间孪生/µs 全等」精确臂,而调度延迟卫星、链聚合、窗普查是同一物理时间的三种**不同切分**——永不孪生,全部 fail-open 成多全额席;RSPA 卫星臂全锚定路径 `:791 continue` 全额保链=锚定份被卫星与链席双代表(B4 头注早写明卫星 "must not mint a second seat",全锚定路径漏了)。③**横切最大杠杆**:同 artifact 全部查询步共熔一棵投影,引擎互斥全是单查询池内;显示多板识别只按窗端点→同窗异 target 第二板零 chip 静默撞号。④语义族:E34(8span)=E35(4span)∪E49(4span) 互补分割,跨步 lane 三分(SELF-SEM/重叠基/非链)+孪生键含行界→跨步必失配双发。⑤伴生 bug:E29/E32=target=shadowhook 步的合法自身席熔进主树后,词臂按 typed 字段无条件佩「自身·墙钟席」,不校验 node.Subject==树 target——跨步「自身」语义漂移。⑥红格矩阵入册(L2×L6/L7 本案、L2×L1、L6×L7 跨步、语义三 lane 跨步、L12×链席跨步、自身缺口席×自身语义 span(值面裁定候选,先互指披露)、L2×L3/宿主边锚 span×宿主席/lock holder=理论红格逐格立案)。
**修复编队(客户紧急,插队主线)**:XLANE-1=runnable 族(卫星全锚定→整席降道+互指正席;B4 增 wakeup_chain 吸收方向;正席序=反转候选>链聚合/逐次>卫星>窗普查;+④词面 bug rider:佩词校验 Subject==树 target+非 target 永不佩「自身·」负向 pin;witness=runnable2 复放;双复核)→XLANE-2=语义族(成员 span 行号集包含判定,子族降「为[E#]成员子集」互指;+自身缺口席×语义席互指披露,值面扣除留裁定池)→(DISPLAY-HYG/HULL-CRED 主线穿插)→XLANE-3=跨步熔合门根修(同线程同族跨板对账+板身份=(窗,target,参数指纹)+撞号修,独立大批)。待验证:客户步序列需 session log/复放;E15 承自行是否被 LLM 叙事误当第四份账。

## §29.105 PERIODIC-DEDUP 收账(2026-07-15;§29.104 终判①;双复核零 P0/P1+修复轮五件;金样 h1..h9 9/9)
**实现**:Σ-有效车道消费值通道 union 的同一份槽位判定(slots/slotOf 在 early-out 前无条件记录,零第二实现);同段身份=不同窗槽∧typed 区间重叠(共享严格谓词);归属窗两级 typed(RankQueryWindow 席位窗>种子成员窗,零启发);fail-open 车道=无窗身份/无区间永计入且不记账(禁连坐)、同槽重叠=独立事实双计、CWD 不可证保全 Σ(禁一刀切清零);单窗/不相交形字节恒等(diff 级证明);EPUB published=OR over counted(被跳副本 marker 不代发布);**迟到量随席去重=主会话追认合射程**(一物理迟到 tick 一份量,复核实测 0.013/0.017 双臂)。哨兵重钉 0.090→0.060。**双复核**:对抗官哨兵算术独立复算+三窗嵌套/链形批未测形实测符合+负向臂逐一突变证活(双重防线实录)+金样零波及升级 typed 喂入层机械证明(九案 periodic_source=true 零活体);冷读官五要素零偏离+witness 复做(七形)+守恒手算+h7/h9 独立复跑 PASS。**修复轮**:EPUB 负方向 pin(M-EPUB 红=published:true 病形)/迟到量 pin(M-LAT 红=0.022 三份全加病形)/三窗嵌套+链形 no-knockout 常驻 pin/CWD 不可证形句面加「已证」限定(零 R2' 方案,typed 分叉裁定指引留 emitter 头注)/VS-1 F6(a) 旧全称断言改历史形。**残留**:CWD 不可证形 Σ 双计残余(fail-open 设计,负向 pin 在岗,rank 行补 Span ts 自动进可证域);产线活体待客户新 trace。

### §29.104.3 立案 XERR1:span 包络冒充阻塞等待值(2026-07-15,客户紧急件二号;witness=/Users/han/opt/customlogs/cust_err1.txt)
客户指认「⊗ 自身·阻塞等待(对端 RenderThread-48660) 199.992ms [E1]」错误(整帧才 ~200ms)。**报告内在矛盾实锤**:E1 宣称阻塞等待=199.992ms=整个分析窗 100%,而同报告四态恒等账=running 108.432(54%)+sleep 84.832+D 3.879+runnable 2.849——运行了 54% 时间的线程不可能同时阻塞等待 100%。真相=E1 的值是 traversal span 的窗内包络投影(实际状态 215.640⚠=完整 span 长 215.575),引擎把 span 时长错标为「阻塞等待」并配「对端 RenderThread」「锁竞争·持锁」因果宣称(明细 E1 影响形态=锁竞争·持锁);模型 prose 放大为「被渲染线程阻塞 199.992ms 占 92.7%,剩余实际 UI 工作仅 15.6ms」(=215.575−199.992 的错误减法),与四态 running 108.432 直接冲突。排查任务:①E1 铸造 lane 定位(⊗ 字形+「阻塞等待(对端)」词面+锁竞争·持锁形——疑 lock_contention/blocking_span/critical_blocking 族),值=span 包络而非等待段的机制;②全 lane 排查「span/包络时长冒充等待值」同类形(binder ⋈ 行/blocking_span/holder 归因族);③修向=值收敛为 span 内目标线程实际非 running 等待段(∩sleep/D/blocked 段)或如实改词「span 包络(含运行)」并撤因果等待宣称;④**行级 sanity 不变式候选:自身阻塞等待值 ≤ 窗内非 running 合计**(四态账 typed 已有,纯比较精确信号)——铸造侧硬门或披露侧核查按红线分层设计;⑤客户 prose 放大面由 PROSE-RC 既有回查覆盖与否核实。原始 ftrace(record_trace_20260526170707@880,13.8MiB)未入库,复放待客户补交。

### §29.104.4 XERR1 排查定谳与修复编队(2026-07-15;素材全文见排查报告,要点入账)
**铸造链定谳(探针复现逐位吻合)**:①噪声筛 admit=`isBlockingLikeText` 自由 substring,token 表含 `sync`——客户 span 名「…re**sync**ed to 58563…」误中,vsync 豁免臂只认 `vsync` 字面不认 `resynced`;②值=span 包络窗投影(`blockingSpanCandidateFromTraceSpan` 直取 DurationMs,全链路无一处与调度状态段求交);③「对端 RenderThread」=span 窗内最后一条唤醒边的 waker(200ms 包络上近乎任意采样,置信 0.62);④明细「锁竞争·持锁」=等待方佩持有者词(family 词条单词条无分叉);⑤字符串 caveat 在 Summary 散文里,模型无视(旧教训再证:字符串 caveat 非咽喉)。**Sanity 缺席四处同处未比**:同一构建函数已 per-row 为 PEER 算完整四态(buildCriticalBlockingPeerState),对 waiter 自己跑同一函数即得真值——机器现成方向选错;覆盖分母机制早知该行非等待行(排除在等待分母外)而同树放它宣称阻塞整窗。**同病射程**:payload-less blocking_span=本尊(无 rank 席);BLIND-2 泛化臂(`owner tid=N` 任意 vendor span)可复现且**带 rank 席**;holder-subject rank 席=等待包络挂持有者(备案);binder ⋈/D/IO/blocked_reason 健康;E1 同 span 可经 trace_span(rank)+blocking_span 双道发布=XLANE 族形提请 XLANE-3。prose 错减法=PROSE-RC-5(§29.80)候选族,lane 定域排除非算术容差。
**修复编队 XERR1-FIX(排 XLANE-1 收账后,同 query.go 避撞)**:①值收敛根修=waiter 在 span∩窗的 Σ(sleep+D+iowait) 段(runnable 不入阻塞语义;复用 buildCriticalBlockingPeerState 模板换主体;包络留 actual_/披露;新 typed 字段 R2';排序分/fold/rank 锁车道/Summary 换径同步;与 sleep 自身行同段互指);②词面同批=BlockingKind=="" 行撤「阻塞等待(对端)」/⊗/「锁竞争·持锁」(改「span 包络(含运行)」+对端降格「最后唤醒者(推断)」;等待方佩持锁词 bug 顺手修);③sanity 不变式=铸造侧 typed 超预算 marker(值+预算随行,禁 clamp 禁硬拒)→显示词面分叉+⚠ 披露「span 包络 X > 窗内非 running Y:含 running Z,非阻塞等待段」;payload-typed(ART 真锁)首批只披露不改值;④噪声筛 `sync` token 误中形=vsync 豁免臂扩 `resynced`/词边界化(净化入口,与①独立双防);BLIND-2 泛化臂误 admit 面待客户 ftrace 定 scope。

### §29.104.5 立案 XCPU:runnable 成员 cpu=unknown 系引擎丢戳非数据无解(2026-07-15,用户指认 witness=20260715-201237.714-86978)
witness:R3 哨兵窗(34579.490..500,目标 59566)自身 runnable 席 E4=合计 0.183(成员 cpu=unknown 0.121+cpu=1 0.062)。**raw trace 手算定谳**:0.062=抢占段(496929 R+ 切出→496991 切回,cpu[001]);0.121=五段唤醒延迟段之和(496599→496622 等五段,23+14+12+38+34µs)——**五段收尾切入(next_pid=59566)全部在 cpu[001],CPU 完全可解**;引擎唤醒延迟段 lane 丢 CPU 戳(抢占段 lane 带戳),两成员修正后本应合并单成员 cpu=1 0.183。**修复陷阱**:wakeup 事件 target_cpu=000 与实际切入 cpu=001 不一致(唤醒后迁核)——归因必须取收尾切入事件 CPU,禁取 wakeup target_cpu(否则 unknown 换成错误的 0)。修向=唤醒延迟段铸造处补切入 CPU 戳(段收尾在窗内时);段真跨窗无切入→unknown 保持诚实;负向 pin=target_cpu≠切入 cpu 的迁核形必须取切入。与 AFF-EVID 同族(有料不上桌)。归 XERR1-FIX 同批 rider(同 tracequery 面,XLANE-1 后)。

### §29.104.6 立案 XGAP:answer_document 降级路径三 gap(2026-07-15,用户指认 witness=20260715-202022.323-89609)
witness=降级产物(页脚自认:重试耗尽渲染未校验草稿)。**内容错误**:①折算基准句编造(「×目标线程 running 份额比例」伪公式,真基准=全域最大核最高频 R5);②#4 席算术自破(2.181+1.419=3.600≠宣称 3.437;引擎真值 2.181+1.248=3.429);③引用错位三处(#3 指 keva-3 行/#4 指无关 0.225ms 行/#5 指 keva-1 行)=引用回填未跑;④模型末轮思考实录=与校验器纠缠两个成员标签逐字格式(`·` 分隔/runnable_wait 词形)烧光重试。**系统 gap**:G1=降级面丢确定性板块(◎/投影树/明细系统渲染不依赖模型,降级路径未渲染,未校验 prose 裸奔);G2=校验器重试死于标签格式(成员 verbatim 校验 vs 排版变体,四发全烧格式);G3=降级 prose 绕过事实面防线(机制句编造非标量回查不覆盖/错误加法降级路径未披露)。排查任务:①降级路径渲染链定位(为何确定性板块缺席;fallback 渲染器与正常 outputdump 的分工);②校验器该两条 member 校验的判定与教学句(模型为何四发解不开——格式教学歧义?循「LLM 分类错就是 prompt 歧义」红线);③降级路径上 PROSE-RC/引用回填/加法披露臂哪些跑了哪些没跑(如实矩阵);④修向:G1=降级面补渲确定性板块(typed 数据在手,零模型依赖)+G2=member 校验失败的 retry-hint 带 verbatim 模板(或校验放宽到规范化比对,按精确信号红线裁)+G3=降级 prose 过既有披露臂+降级页脚警示升格。

### §29.104.7 用户裁定 R10:answer_document 校验重试预算适当扩大(2026-07-15,XGAP 伴随裁定)
用户指示:校验循环耗尽若太少可适当扩大。执行形=**组合拳**(单独扩预算对「同一误解打转」形收益有限,witness 末轮思考实录为证):①重试预算默认适当上调(如 4→6,以 XGAP 排查定位的实际 knob 与现值为准;若有 codrax.yaml knob 同步默认表+example);②G2 retry-hint 修根同批——member verbatim 类校验失败时 hint 直接携带期望行逐字模板(校验要逐字,hint 就给逐字;循「LLM 分类错就是 prompt 歧义」红线);③G1 降级面补渲确定性板块=耗尽后的保底可信骨架。三件同入 XGAP-FIX 批。注意面:推理模型单发成本高(分钟级),预算上调幅度以「格式类小错可解」为度,不以预算掩盖 hint 缺陷(哨兵=若扩后仍频繁耗尽,回看 hint 质量)。

### §29.104.8 XGAP 排查定谳与 XGAP-FIX 编队(2026-07-15;素材全文见排查报告)
**G2 定谳(非教学缺陷单因,义务集自相矛盾)**:五拒=A/B/A/B 完美交替非同错——同 run 三次被接受的 emit_investigation_complete 铸了**两个「根因排序」member_set fact**(一无 role/unit,一 principal_answer/席位数;merge key 含 role+unit 分桶不折叠),同一 #1 席两值(143.499 全额 vs 51.735 折算)、同一 #3 席两词形(runnable vs runnable_wait)——**XLANE 双发病搬进 label 空间,verbatim 同时满足=报告自相矛盾,模型无解**;更痛:emit#4 带引擎真值(keva-1 3.429+正确折算基准)因装饰形 support_refs 不解析被整体 DOWNGRADE,好数据没进 handoff。F8-T4 审计注释(26b9c3ae)明文预言本案(同因 N-strike breaker 故意缓装等 witness——本 run 即 witness);agent 层同类断路器 5 拒即停兜住了预算,但兜法=放弃降级非收敛。校验臂本身=精确信号合法硬门(verbatim presence),不放宽。**G1 定谳**:确定性板块全挂 persist 咽喉(五拒未达 persist→从未装配);typed 观测账在手(fallback 渲出 88 条观测为证)只是投影 materializer 没被调;恢复支线未标 AnswerDegraded。**G3 定谳**:四防线(引用回填/PSG/加法披露/口径词回查)全部 docV2==nil **结构性跳过**(修复预算一分未花即出厂);且 **runtime-artifact 引用面无 quote 核验臂=健康路径独立结构缺口**(witness 引用「quote」是模型编的散文摘要,实际行是原始 ftrace 事件)。
**XGAP-FIX 编队(含 R10,排 XERR1-FIX 后)**:①G2 根修=completion 侧同 label member-set fact 仲裁(席位序数键对齐→后接受 emit supersede 先前版本,SEM-LEAD 序数键判例;消灭矛盾义务,校验臂零动);②G2 教学=hint 差集改全义务集+✓/✗ 在场标注+「逐字加入勿替换」指令句+F8-T4 同因 breaker(blockID+missing-member-hash,指纹排除 handler 自变值);③R10 预算=breaker 阈值/patch attempts 适当上调(以①为主②③为辅);④G1=ParseOutput 恢复支线调 tool 层导出 materializer(ToolBusContext 窄化,幂等守卫已有)+AnswerDegraded 打标(:10812);⑤G3=S3' 附录/PSG/lexicon 收降级车道输入(docV2==nil 时喂恢复 doc)+runtime-artifact 引用 quote 核验独立臂(检测→披露)+页脚警示升格。待验证:健康路径无「=」词形假加法句的等式臂命中;emit#3 伪基准句上游=PROSE-RC 战场。

### §29.104.9 XREPRO 本地同类形挖掘定谳(2026-07-15;双 fixture 四形,926 跑;工件 scratchpad/xrepro/out/)
**形③多步熔合撞号=本地全量复现(XLANE-3 验收基建成立)**:配方=donghu 同窗(13762.791708..13763.024898)步1 target 2955+步2 target 9163,双步 observation ledger→CompileTraceCausalProjectionSet→projections=1(同 artifact 共熔实锤);三病形 verbatim:①榜位撞号 9 组(chip 根因排序#1..#4 各×2);②「自身·墙钟席」跨步错佩活体(logd.writer 三行佩自身词于 2955 树);③同线程跨步席共存 ⛓Σ116.5 vs 全窗 49.656=**2.35×**(客户 2.0× 同病),且「锚定0.018(⛓链上席)」与「40.656 ⛓ 全额」两说同树并存。**形④XERR1=本地自然 witness 全链(XERR1-FIX 验收锚点成立)**:tieba span「H:Native a**sync** work complete…」被 sync substring 误中 admit(与客户 resynced 同病类);span 包络 29.843 vs 宿主非 running 16.572=**4.5× 超宣称**(修向真值 Σ(sleep+D+io)=6.637);端到端铸行=type=blocking_span dur=29.843 peer=唤醒边推断 rel=on_chain(§29.104.4 机制逐位对应);同窗同 span 又以 trace_span 双道发布(XLANE 族形);同窗 rank#1 带 cpu=unknown(sched_in_cpu_mismatch)=XCPU 同族在场=rider 验收窗。**形② P1 自然形**:包络缺口形 24 例(最佳=tieba 59843 全窗:envOv 94.915 vs occOv 16.589,三段全在缺口;donghu 45× 膨胀例)+部分相交 35~42% 档 5 例(极端 ≪10% 维持合成 pin);跨步非自身形(donghu 9163 三段落步1 occurrence 缺口+两段 µs 全等双发)。**形① 单查询卫星×链席共存=926 跑零自然命中**(分母 census:148 对共存中 144 自身豁免+4 已正确降 ◇),负结果可辩护,维持合成 pin+客户复放两级。**探针盲区教训(对在飞修复轮直接有效)**:rank 行面无 StartTs/EndTs 区间——互斥门必须回 ChainResult 本体(OccurrenceWindows/CausalImpacts.Window)取区间。**补采终判**:XLANE-3/XERR1-FIX 产线验收全锚本地 fixture;客户补采(v2_slim)降级为修后回放确认级,不阻塞修复批。

## §29.106 XLANE-1 收账(2026-07-15;客户紧急件一号:runnable 族跨车道双算修复+双复核+修复轮;金样 12 趟账;突变 M1-M8 红)
**机制(修后 runnable 族代表权)**:链聚合/逐次/反转候选=正席(同段物理时间唯一全额链上代表,B4 吸收方);卫星(调度延迟/低频)三分=区间孪生→B4 吸收(新 causal_scheduler_latency 对,区间集/行界/窗/TID 全等精确臂,lane 桥仅「件1 记号∧wakeup_chain 吸收方」放行)/链席**全覆盖**(Σoverlap≈卫星账目 µs 容差)→整席 ◇ 降道(typed ChainAnchorRepresentedByChainSeat,值零动,句面「锚定份由链席[E#]代表(整席降道)」与 R4 无凭证降道显式划界)/其余(链席缺席/不相交/部分相交/区间不可证/目标自身)→保链(唯一代表保护,四形+self-exempt 负向 pin);「自身·」佩词门 rider=canonical 主体≠树 target 零佩(E29/E32 病形 pin,平铺 fail-open)。R2' 七处(tracediag hash 6529e0a4)。
**双复核**:冷读零偏离(降道门十判定位精确信号纯度专项=纯;全链 witness 复做+守恒 8.443≤26.725);对抗官两 P1——**P1-1 包络击穿**(链席区间收集用聚合席 FirstTs..LastTs 包络,卫星段全在 occurrence 缺口也判相交=「hull/包络=嘈声禁进硬门」教训三犯)/**P1-2 词面超宣称**(门只验相交>0 而句称全额代表,1ms 相交 5ms 形实锤)。**修复轮**:P1-1=三级精确库存(familyMemberIntervals→OccurrenceWindows→单 occurrence 席;多成员无库存贡献零段=fail-open 保链;与 §29.104.9 XREPRO「rank 行面无区间必须回链本体」独立定谳吻合;缺口形对抗探针转常驻 pin=tieba 59843 自然形合成孪生);P1-2=门升覆盖证明(部分相交→保链;三面句面构造保真零改词;Σoverlap≈卫星值不变式 pin);P2 双静默面 pin(R1 幸存者 OR 双向/types anchorFormKey 叉);P3 名词退路 pin+终报 3603b64a 引文按文件面勘正(归因不变)。
**金样 12 趟**:h1..h8 PASS+h9 首趟 FAIL(=XGAP 降级 witness §29.104.6/.8,diff 与病灶车道零交集 grep 证明)+h9 复 roll PASS+修复轮 h7/h9 PASS;一趟瞬时 5 FAIL=10s 子进程超时(并行 eval 负载,隔离+全量复跑双绿归因)。**残留**:客户复放=修后回放确认级(§29.104.9);极端 ≪10% 部分相交档合成 pin 承载;XLANE-2/3、XERR1-FIX 按编队(本地锚已备)。

### §29.104.10 立案 LOCKNS:持锁 tid 识别双形态+容器化 ns-tid 泛化(2026-07-16,客户需求,排队分批交付)
客户给出 ART 两种持锁 span 文本形态(源码级):**形A monitor contention**=`monitor contention with owner <owner_name> (<owner_tid>) at <PrettyMethod>(<file>:<line>) waiters=<N>`;**形B mutex**=`Lock contention on <mutex_name> (owner tid: <tid>)`。两环境差:Android 非容器化=owner_tid 直接可定位宿主线程;**东湖场景=鸿蒙容器内跑 Android,span 里的 owner_tid 是容器命名空间 tid,须经 ns-tid 映射找宿主真实线程**;另有「其它未知形态」要求泛化可扩。现状基建线索:parseLockContentionPayload(lock_contention.go)已有 payload 解析(XERR1 排查记录其对 ART/OHOS 形覆盖)+BLIND-2 泛化臂 `owner tid[:=]N`;wire 面已有 HolderNsUnification 字段(既往批产物)——排查先行:①两形态现覆盖度(形A 的 method/file:line/waiters 富信息是否已提取);②ns-tid 映射机制现状(HolderNsUnification 语义/触发条件/东湖形是否已通);③泛化架构建议(形态注册表 vs 单一正则族;未知形态的 fail-open 词面);④与 XERR1-FIX 件2 词面(holder/waiter 分叉)与 holder-subject rank 席的衔接。排查后实施批(LOCKNS-FIX)入队列:暂列 XGAP-FIX 后、XLANE-2 前(客户需求优先于内部编队,顺序可按用户指示调)。

### §29.104.11 队列调序+LOCKNS/BLIND-2 本地普查(2026-07-16,用户裁定)
**调序(用户)**:XLANE-3 提前(客户近期频繁高优先级使用多步场景)——队列=XERR1-FIX 收账→XGAP-FIX→**XLANE-3**→LOCKNS-FIX→XLANE-2→DISPLAY-HYG→HULL-CRED。**本地普查(主会话 grep 实测)**:两 fixture 锁形态全齐——donghu 形B 19 条+形A monitor contention 4 条,tieba 形B 84 条;**形A 实际含第三段 `blocking from <method>(<file>:<line>)`(客户源码片段未含,泛化解析必须认);容器形活体在库**:span payload B|容器tgid|(37722/60194)与 trace 头宿主 tgid(17267/59566)不一致,owner tid(38414 等)在容器命名空间——ns 映射证据源=B|tgid| 前缀↔头部宿主 tgid 对应+owner 名宿主同进程对照;**哨兵值**:owner tid=18446744073709551615(uint64 -1)与 0 在场,解析必须识别哨兵。BLIND-2 scope 本地初判干净(全部 owner-key 行=ART 两形,零 vendor 杂形);客户侧降为**可选**微型普查(event_search 全文件 owner-key 行,<5KB),不阻塞任何批。

### §29.104.12 LOCKNS 排查定谳与 LOCKNS-FIX 编队(2026-07-16;素材全文见排查报告)
**现状(比预期乐观)**:①两 ART 形态**已全量解析**(形A 含 blocking-from 第三段/手递链/owner comm,形B 含哨兵纪律 OwnerAbsent);②**ns-tid 三级梯已建成**(LCK-2 ad5af294,§18.E:payload 直解 0.72→ns-span 发射对推导 0.67(进程级降档显式披露)→closing wakeup 0.62;②×③互证=HolderNsUnification 0.70);③**容器形当前能正确定位**(fixture 实测:donghu 形A owner 38414(容器)→宿主真线程 ransmitThread-18130,进程级+waker 互证)。**缺口四点**:G1=**形B 撞号错指向量**(容器 ns-tid 与宿主无关线程撞号时梯①直发 conf 0.72 零披露,rank 席主体换错误线程佩持锁词+1.35 权重;形A 靠 comm 碰撞检幸免,形B 无 comm)——修=梯①加 ns-divergence 门(SpanPID>0∧TGID>0∧SpanPID≠TGID 跳过直解入梯②,两整数精确信号;identity-ns trace 字节不变);**与既有 pin5/§18.E「梯①保留」裁定冲突,需用户裁定**(排查论证:ns-divergent 行 payload tid 由构造在容器命名空间,梯①对其永远语义错误,撞对也只是巧合);G2=形A 前缀变体(vendor 前缀)整形失效→payload-less 全丢(词边界化,additive);G3=哨兵/ownerless+无唤醒边行无披露(caveat 出 fb.OK 分支);G4=OM-10 已知缺口(HolderNsUnification 确定性显示面零消费,属 IC-L 批)。**BLIND-2 scope 关账**:本地全部 owner-key 行=ART 两形零 vendor 杂形;客户微型普查=可选。**LOCKNS-FIX 编队**(XLANE-3 后,同 query.go 区避撞 XERR1-FIX):件1 ns-divergence 门(待裁)/件2 形A 词边界化/件3 形态注册表化(三臂平移+未知形 fail-open 到 XERR1-FIX 包络词面+「owner 未解析」披露)/件4 哨兵披露;rider=OM-10 接线(可归 IC-L)+unresolved conf 残留核查。测试面:两形+容器形+哨兵+互证锚全本地(tieba 84 行 µs 级需小地板);合成必需=②a 自报形+撞号形+vendor 前缀形。

## §29.107 XERR1-FIX+XCPU rider 收账(2026-07-16;客户紧急件二号:span 包络冒充阻塞等待修根+cpu=unknown 迁移陷阱;双复核+修补轮八件)
**病(§29.104.3/.4/.5)**:客户 cust_err1.txt E1 `⊗ 自身·阻塞等待(对端 RenderThread-48660) 199.992ms`(帧长仅 ~200ms)=span 包络冒充阻塞等待值。三病根:①isBlockingLikeText 子串筛走私(re**sync**ed/a**sync** 命中 `sync`);②payload-less blocking_span 行值=span 窗包络,从不与 sched 状态求交;③对端=最后唤醒边 waker(0.62)佩「对端」因果词+明细「锁竞争·持锁」=等待方佩持有者词。XCPU rider:runnable 段 cpu 取 wakeup target_cpu=迁移陷阱(报告 20260715-201237 cpu=unknown 病,target=000 实跑 001)。
**修(四件+rider)**:件1 值收敛=waiter 在 span∩窗 Σ(sleep+D+iowait) 写 DurationMs(runnable/running 排除;D 与 iowait 互斥 lane 无双计),模板复用 buildCriticalBlockingThreadState 换主体零第二实现,收敛不可得→包络+typed basis=span_envelope;件2 词面 fork 单点 typed `blocking_value_basis`(wait_segments→**⊖ 阻塞等待候选**+对端降格「span 期间最后唤醒者(推断)」;span_envelope→**⊓ span包络(含运行)** 零阻塞宣称;basis 缺席→逐字节 fail-open;「锁竞争·持锁」等待方佩持有者词顺带修=BlockingSubjectIsHolder fork);件3 WaitBudgetExceeded typed marker+⚠ 句「span 包络 X > 窗内非 running Y:含 running Z,非阻塞等待段」(禁 clamp 禁硬拒,F-2 同基门=waiter 账目满覆盖 span 窗才判,容差常量独立禁跨语义借用);件4 词首边界化(resynced/async/vsync2 全族出局;synchronized/SyncFence/data_sync/`sync barrier` 保 admit;vsync 豁免臂保留过界形);互指对臂=同 canonical 主体∧sleep 状态族 registry lane∧墙钟行∧窗兼容∧区间包含证明,≥2 候选歧义整体跳过(宁漏勿假指)。XCPU=段收尾 sched_switch 切入 CPU(`sched_in_migrated`/`sched_in_stamped` 两 typed reason),负向 pin 双向禁取 wakeup target_cpu;真无收尾切入保持诚实 unknown;`sched_in_cpu_mismatch` 降 LEGACY-ONLY。R2' 七处全走。
**双复核**:冷读 PASS 零 P1——手工数字守恒全过(tieba 8091 窗 6.637=4.084+2.553+0.000/包络守恒 29.843=13.271+16.572/4.5× 超宣称/E1 合成 72=60+12(runnable 20 排除)/XCPU 0.183=0.121+0.062 五段切入全 cpu=1;自写解析器 raw tile 不经引擎);修前病形三要素逐位=客户 E1,修后四防线(件4 admit 拒→件1 值收敛→件2 词面 fork→件3 ⚠ 预算)下不可再现;h7/h2 金样复跑 PASS。对抗 PASS 一 P1——8 突变 M-R1..M-R8 全红+5 件假 pin 猎杀全活(holder-fork pin 基线复放 FAIL=真行为变更证明);件2 legacy 跨树逐字节 diff(4 形×zh/EN×fence+明细)fence 全恒等;h7 oracle 演化 typed 证据链核实(第六席 #tp-io-8-17460 二分恒等 0.477=0.011+0.466,pre-XCPU 两 PASS run 无 anchored 行=确系 XCPU 解锁);全套 83 包 EXIT=0。
**修补轮(八件,含复核清单全收)**:A(对抗 P1-1)fold take-MAX 合并行预算破同基修根=blockingSpanRow 新增 valueBudget{Start,End}Ts/valueBudgetUnknown 精确三字段,换值随值携带值胜出形区间,预算统一 window 于值胜出形区间,不可得禁判 fail-open(witness:rich 100.05..100.15/env100+tid-only 100.0..100.2/env200 全 sleep waiter,修前假 ⚠「含 running」对 running=0,修后 200≤200 零判;正向臂真超照判);payload-less 行结构上不可能被 fold(typed 门 BlockingKind!=""∧Peer.PID>0)=件1 无同病第二处,recompute 防线对齐。B(冷读 P2)⊖(basis=wait_segments)行豁免投影 R2 再测度 SUM 折叠(typed 门;该行已是 waiter 等待 lane 再测度,与同对端 wait 族相加必双计;witness:⊖6.637+⋈2.445+⋈1.639 修前 ×3 SUM 10.721 双计一席,修后 ⊖ 独立;无 basis 行 legacy 逐字节负 pin;types 侧镜像常量与引擎等值 pin)。C(对抗 P2-1)XCPU 切入戳窗界口径对齐=**行为保留**(切入事件可在窗外:段收尾切入定义该段运行 CPU,窗裁剪不改身份,与既有 verified 车道同口径),注释三处如实改写+表驱动 pin 钉窗外切入照戳(窗 1.0..1.1 切入 1.15→cpu=2);**本节裁定:§29.104.5 验收句「窗内切入」措辞过窄,以本节为准**。D(双复核同报)rcr.go Form 常量陈腐注释(riding ☾/◦ 残稿)勘正为专属 ⊖/⊓。E(对抗 P3-1)互指臂三负向分支补 pin(歧义跳/端点缺失跳/非包含跳)。F(冷读 P3-3)partial-coverage 已证下界披露=新 note key 对 blocking_wait_coverage_partial/blocking_wait_account_covered_ms,R2' 七处全走,明细「覆盖核查」行 zh/EN(端点缺失无数字退路句不造数),与件3 marker 互斥独立行 emit-pin。G(冷读 P3-4/5)Σ=0 形 pin(纯 running+runnable→诚实 0,件3 以真数照判)+holder-subject ⚠ 变体 pin(twin-port 预算三键随行+「(预算主体=等待方)」两面)——**pin 首跑抓出批交付真 bug:EN 面把 zh 附注连同 EN 附注一起拼出(if !zh 追加未分叉),修根 per-lane else 两处**。H 批遗留 zz_ 复核探针确认已清(开工即验,全仓零命中)。
**备案(字面偏离/话术勘正三件)**:①SpanEnvelopeMs 铸新字段而非复用 ActualDurationMs(裁定字面「包络留 actual_」)——ActualDurationMs 的「缺席=未截窗」是精确信号,物理 B/E 延展≠窗内包络投影,语义禁跨借用;判=比裁定字面更正确,准。②legacy 工件非严格字节恒等(对抗 P2-2):basis-less 行明细「影响形态」持锁→阻塞(zh/EN 各 2)=点名词 bug 修的正确外延;话术勘正为「⊗ 锁族/fence 字节恒等;FamilyWord 车道 deliberate 词修覆盖 legacy」。③h7 oracle 诚实演化补全(对抗 P2-4):0.094 五席→0.105 六席之外,同窗邻近席值面涟漪=logd.writer cpu_affinity_or_cpuset 9.000→48.519(5.4×)+新增 low_frequency 13.884/10.308 席=CPU 戳位解锁 CPU 特定车道的设计后果,非硬锚,随 EVOLUTION RECORD 补记。
**涟漪/观察**:XLANE-3 可见度上升(收敛后 ⊖ 6.637 与 trace_span ◦ 29.843 不再被 R1 同值吸收,同 span 双道并存,◦ 行诚实非链零宣称)=§29.104.9 案内涟漪不另立。词面观察不立案(客户复放若误读再动):「最后唤醒者(推断)」坐旧对端括号槽/⊖ 与 ⊗⊘ 近距/⚠ 冒号结构「含 running」挂靠首读歧义/「段落在」连读。冷读 P3 residual:sync 非 ASCII 折叠变长防御回退自由子串(soft-screen 安全方向,代码自述,不立案)。

## §29.104.12.1 LOCKNS G1 用户裁定(2026-07-16)
用户原文:「G1(最重,需你裁定):形B 的撞号错指向量 -> 按推荐的方案来。」落定:**梯①(payload 直解)加 ns-divergence 精确门**——SpanPID>0 ∧ TGID>0 ∧ 容器tgid≠宿主tgid → 跳过直解,入推导梯(②发射对推导/③收尾唤醒);§18.E「梯①保留」pin 相应重写。论证存档:ns-divergent 行 payload tid 由构造在容器命名空间,直解对宿主 tid 表**永远语义错误,撞对只是巧合**;identity 命名空间的 trace 字节不变,门是两整数精确比较。LOCKNS-FIX 编队解封(件1 本门+件2 形A vendor 前缀词边界+件3 形注册表/未知形 fail-open 至包络词面+owner-unresolved 披露+件4 哨兵/无主披露+riders OM-10 归 IC-L/unresolved 0.72 复核),队列位=XLANE-3 之后(§29.104.11)。

## §29.108 XGAP-FIX+R10 收账(2026-07-16;答案文档降级路径五件+预算;双复核+修补轮六件;义务集自矛盾修根)
**病(§29.104.6/.8)**:witness=用户降级报告 20260715-202022+h9 金样 FAIL run(201207)。同 run 三次被接受的 emit_investigation_complete 铸两个「根因排序」member_set fact(一无 role/unit,一 principal_answer/席位数;merge key 含 role+unit 分桶不折叠)→同一 #1 席两值(143.499 全额 vs 51.735 折算)/#3 席两词形→verbatim 义务自相矛盾,模型无解,五拒 A/B/A/B;emit#4 带引擎真值因装饰形 support_refs 被 DOWNGRADE;五拒未达 persist 咽喉→确定性板块全没装配;恢复支线无 AnswerDegraded;四防线 docV2==nil 结构性跳过;runtime-artifact 引用面无 quote 核验臂。
**修(五件+R10)**:①序数席 supersede=同 label member_set fact 仲裁(SupersedeOrdinalMemberSetFactsByLabel:归一化 label(刻意剔 role/unit/dims=witness 分桶病根)∧序数交集非空∧【修补轮第三精确臂】共享席主体逐一 verbatim 一致(§11-N7 canonical 线程 token 复用解析,零可比对/任一冲突→fail-open 保双)→后接受整 fact 胜;纯确定性记账,校验臂 verbatim presence 零动,完成门权属红线零触;六汇流点单函数接线=emit carry-forward/MergeExploreFork 双臂/surface_plan/observation_ledger 跨 turn 镜像双点;同桶席位占用守卫=先胜(方向差异 deliberate 双注互指));②hint 差集改全义务集+逐成员 ✓/✗+「逐字加入勿替换」指令句(roster cap40 诚实截断)+F8-T4 指纹断路器(指纹=sorted missing (label,member) sha256/8B,刻意剔 blockID=handler 自变值(blockerKey 自搅动教训),按指纹累计非连续,>3 force-stop 走快照恢复链,成功清零,异因不计不清);③R10=maxRetries 3→4/派生 hint 预算 12→16/finalizer 专属 IdenticalErrorStreak 3→4(真限流器=witness 实际死因,其余 agent 保 3)/新 knob agent_finalizer_member_set_breaker_max_strikes=3(非正回退默认);④降级面 materializer=MaterializeDeterministicAnswerSectionsForDegradedDoc(persist 咽喉七 materializer 同序+引用回填+quote 臂,ToolBusContext 窄化,幂等复用),ParseOutput 两恢复支线接线+typed AnswerDegraded/SkipAnswerChecks/DegradeReason(text-recovery lossless 保 SkipAnswerChecks=false=载体上照跑检查链);⑤四防线读 ShippedAnswerDocumentV2 面(降级快照载体,正规 emit 落地即清、永不晋升)+runtime-artifact 引用 quote 核验独立臂(空白折叠双向包含+省略号豁免,失配→披露永不硬拒,路径双锁拒任意绝对路径,64MiB 扫描墙 fail-open,健康路径 persist 咽喉同接线)+页脚降级升格 zh/EN。
**双复核**:冷读 PASS 零 P1——witness 两 fact 从 FAIL run 日志逐字节核出(pin 非合成形);唯一化方向=51.735 折算版胜出与 §29.88.12 单基准一致;降级路径端到端实渲(观测板/页脚双语/quote 臂/四防线 finding 全实证);**健康路径对基线 byte-identical**(persist 产物+全渲染);R2' 零触发核实。对抗 PASS 一 P1——13 突变 11 红 2 绿(绿=M2/M5 未 pin 接线→修补轮补钉);K1=**跨 target 义务误杀实锤**(双 target run 两合法「根因排序」板同 label 同佩 #1..#3,later 整块灭 earlier,义务+surface_plan 手递双消失,校验臂放行=义务硬删除挂嘈声 label 信号);K3 限流交错实测=witness A/B 形 class 门(streak4)第 5 拒先停、指纹臂价值=异 class 穿插+连续同指纹第 4 拒早停一轮;K5 健康路径 16,000 查零假披露(全半角标点变体会披露=软风险可接受);K6 同指纹风暴终态与修前持平、异因多烧恰 1 轮=R10 刻意余量。
**修补轮(六件)**:A(P1)=第三精确臂共享席主体一致(witness 形实测 #1/#2/#3 主体双方一致照 supersede 零改;跨 target 形修前误杀 2955 板/修后双保留+消费面义务双在场 pin;部分冲突/零可比 fail-open 保双 pin;false-keep 安全论证=矛盾义务兜底已由②断路器+④⑤降级车道接住;**与 XLANE-3 前置互补**=多步双板共存从义务面先保住,不撞 XLANE-3 展示面战场);B=M2 retained 臂 requeue 分歧窗 pin+M5 观测账跨 turn 镜像双点 pin(两处均实际可达);C=降级页脚内部 snake_case 板块名换用户可读名(就近映射表 10 token 全覆盖+漏映射 fail-open 通用词+映射完整性 pin,内部 token 零上用户面);D=quote 失配披露「模型转述」超宣称收敛为「与所引工件行原文不符(可能为转述或行号错位)」zh/EN;E=四处注释如实化(断路器 A/B 叙事/K2 双向仲裁互指/F8-T4 旧审计注 status/AnswerDegraded 两形态);F=64MiB 扫描墙负向 pin(尺寸探测点注入,cap 缩→墙外零披露+恢复→照披露)。
**备案**:①F8-T4 指纹剔除裁定字面的 blockID=有据收紧(义务文档级,含 blockID 让换块清零 breaker;「排除 handler 自变值」同句自洽);②断路器叙事勘正(见 K3 实测);③等式臂可达性核验=干净 A+B=C 形有防线,括号插入/倒装形为既有结构盲区,属 PROSE-RC 战场记录不修(emit#3 伪基准句上游同判);④patch-reject hint 的 answerDocPatchRejectTypedDetail 既有 360B 截断=后续候选,主承载面(tool result 错误文本)未截断且指令句置头;⑤advisory/prompt lane 三处裸 union(builder.go:2197 等)不铸义务,矛盾对可入 prompt 语境=观察不立案。

### §29.104.13 用户原则重申+SUPPREF-TOL 立案(2026-07-16)
**用户原文**:「系统硬拦的情况是否是致命错误?非致命错误尽量不要硬拦。尤其是模型结束调查和成文阶段。」——完成门权属模型(§29.60/.1)显式扩展覆盖**成文阶段**:emit_answer_document 硬拦保持 F5-T4 封闭白名单制(仅 complete-principal-member-set verbatim 在场核查(=模型自己在完成阶段断言过的义务,非系统意见)+用户显式要求的 diagram 块同轮修两条;未注册 kind 永不硬化;citation 漂移=修复/摘除/披露恒软);白名单硬臂必须有界(R10 预算→同错流水闸→同因指纹断路器→**降级出厂=确定性板块装配+页脚披露,永不空手永不无限拦**,§29.108 已压实)。完成阶段维持致命三类(零见证/工件缺失/schema)+absence_justification typed 逃生道;结论一致性指控臂=禁区不变。评审任何 gate 改动以此为 BLOCKING 检查项。
**SUPPREF-TOL 立案(盘点残留)**:装饰形 support_refs 触发 emit_investigation_complete DOWNGRADE 吞好数据——XGAP witness emit#4 带引擎真值(keva-1 3.429+正确折算基准)仅因 support_refs 带装饰不解析被整体降级,好数据未进 handoff(§29.104.8 定谳链一环,XGAP-FIX 未修此点)。修向=宽容解析(剥装饰→重试解析,仍失败才降级;精确信号不受损,解析成功面零变);小批,队列位=LOCKNS-FIX 后随行或并入就近批。边界维持现状备案:diagram 块 pre-emit 硬臂=用户显式要求面丢失,同轮局部修+预算封顶+post-emit 保软,可辩护。

### §29.104.14 立案 HEADLINE-ELIM:首因与最大可消除量倒挂时的诚实披露缺口(2026-07-16,用户指认)
**用户问题**:优先级反转可消除量 < 语义确定性 span 可消除量时,模型仍把优先级反转当首要原因,未诚实披露语义 span 为(可消除量口径的)主因——是否系统约束所致?**排查定谳(主会话只读盘点)**:是,两条约束叠加的合规行为+一个真实披露缺口。①榜序比较器(query.go sortRootCauseRankItems)第一键=chain_relevance 通道、第二键=发布 eff(Score/乘子仅同值 tie-break,SEM-LEAD §29.22.1 序数键==发布 eff)——R4「边=凭证」排他使无凭证语义 span 结构上不可能任链上#1;R3 边锚定形只有边前份额入链上席,剩余份进 ◇,而可消除量面看全 span。②skill 硬教学(PRIMARY-CAUSE ENTITY CONSISTENCY,defaults.go)=散文首因必须=榜首实体、禁止提升更低归因候选(分歧须显式声明)——模型行为=服从教学。**缺口**:✦ 提及义务+优化点无条件入正文保证了语义 span 不消失,但无任何义务要求首因句在「可消除量倒挂」时做对比披露。**修向(推荐,榜序零动/值通道零动)**:渲染层确定性互指披露臂——链上首因可消除量 < 某确定性语义 span 可消除量(两个已发布标量的精确比较)且该 span 非链上首位时,首因行/结论区强制携带对比句「按可消除量口径 [语义span] N ms 更大,但[无链上凭证进邻近/仅锚定 Y ms];首因按因果凭证口径为 [X](M ms)」zh/EN+typed note(R2');skill 侧同步软引导。**待 witness**:若用户报告中两行均链上且语义 eff 更大仍屈居=叠加序数真 bug,另查(折算基/合并行/二分份额)。队列位=DISPLAY-HYG 随行或独立小批。

### §29.104.14.1 HEADLINE-ELIM witness 定谳:形3=模型散文推翻榜首(2026-07-16,客户 witness=/Users/han/opt/customlogs/cust_span_runnable.txt)
**定谳(推翻 §29.104.14 的形1 假设)**:该 witness(donghu 970481 帧,17729.471..17729.623)里**引擎四确定性面全部正确**——根因排序#1=E26 类校验 9.586ms tier=primary(E26/E51 审计行),优先级反转 shadowhook E22 8.608ms 仅 #2 secondary;◎ 可消除量总览 TOP1 满格=类校验;投影头行「主根因:类校验 链上累计 9.586ms」;优化点表首行同。**是模型散文推翻榜首**:标题句「核心丢帧原因为 shadowhook-task…优先级反转阻塞」(8.870ms=窗 5.8%),并把 #1 primary 降为「可归为次要因素」,分歧理由=范畴论证(「并非由外部阻塞引起」=自身工作不算根因,违背系统同权参赛语义),9.586>8.608 数值对比零正视;诱因=客户前置分析「IdleHandler 被 shadowhook 阻塞」叙事锚定。**第二处散文-榜面矛盾**:散文「已运行在超大核最高频点…排除算力供给不足」vs E5 席「计入 4.843ms(折算,运行频点非最高)」;118.712% 已被校验附注标记不可复算。**放行机制**:ENTITY CONSISTENCY 纯软教学,分歧门两义务(显式声明+证据依据)未履行也无从查;校验附注只做标量复算/席位并置,无「标题实体 vs 榜首」并置臂(「主根因=类校验」与「核心原因=优先级反转」同纸并存零指认)。
**HEADLINE-ELIM 修向更新(遵 §29.104.13 非致命不硬拦)**:①skill 教学加硬度=推翻榜首必须显式「与根因排序分歧」声明+数值对比义务,禁范畴论证降级 #1(自身确定性工作同权参赛=系统语义,教学词面直书);②校验附注**确定性并置披露臂**=散文标题因实体与根因排序#1/投影主根因头失配→并置句「正文核心原因=X(M ms);本报告确定性主根因=Y(N ms);正文未声明分歧依据(如适用)」——散文实体提取=嘈声信号,只做披露 caveat 永不硬拦、永不改写正文;提取用 canonical 线程 token/语义类词 verbatim 命中,不可辨则整臂静默(宁漏勿假指);③rider=频点/算力宣称并置(「排除算力/供给不足」「已最高频」类宣称 vs typed 供给折算缺口席在场)。队列位=XLANE-3 收账后、LOCKNS-FIX 前(客户可见诚实面,小批;用户可重排)。witness 已存 customlogs/cust_span_runnable.txt。

## §29.109 XLANE-3 收账(2026-07-16;跨步熔合门根修=板身份三元组+撞号分域+跨板对账;双复核+修补轮十件;客户多步高频场景)
**病(§29.104.2 定谳③/§29.104.9 形③)**:同 artifact 全部查询步共熔一棵投影(projections==1),显示多板识别只按窗端点——同窗异 target 双步:①榜位撞号(根因排序#1/#2/#3 各×2 裸 chip 零披露);②同线程跨板席共存零互指(「锚定0.018 vs 40.071 全额」两说同树;客户 2.0×/XREPRO 2.35× 同病族);③跨板 Σ 活体(「各根因席位有效归因合计 355.562ms 超过窗长 233.190ms」=两板混加)。
**修(四件,值通道零动)**:件1 板身份=(窗,target,参数指纹)typed 三元组——engine RootCauseRankResult.BoardParamsFingerprint(sha256/8hex,归一化闭集,修补轮扩至 {MaxDepth,MaxBranches,MinDurationMs,Limit,CoreTopology,ViaThread,LineStart/End,TraceFlavor/Platform},窗/target/派生输入论证不入)+产端每 rank 观测行携 rank_board_target(result 级 typed)+rank_board_params_fingerprint,空值零落 absence 永不分裂;聚合 donor 三处随席;projections==1 保持(板在投影内三元组分域,多板机制零重造——探明现状=多板已可并存但身份只窗端点,修在身份键)。件2 chip 分域=窗×target×指纹三维普查,chip 半只在真歧义处佩(`·板锚 <comm-pid>` verbatim 零截断/`·参数#<fp>`),detail 同字节,板 ID 三级细分,三新 mark+GeneratedLegend;修后撞号=0,两板 #1 各佩各锚。件3 跨板互指(population=席位∧墙钟尺∧typed 状态族∧完整板身份,宁漏勿假指;双向句「同线程同状态族账另见另板席…各板独立成账、口径各异,不可跨板相加」cap2+诚实计余)+跨板 Σ 面按板分域(RankBoardEffSumMS 分域后 fused 两板 183.511/172.051 各<窗,355.562 病句诚实消失=183.511+172.051 手算闭环)。件4=XLANE-1「自身·」rider 多步双向纯 pin(fused 树 logd.writer 零佩/CompThread 照佩;单步佩回)。
**双复核**:冷读 PASS 零 P1——逐 hunk 零私货零缺失;值通道零动实锤(diff 无值写点+端到端数字多重集 pre/post 逐值全等+`git diff -w` 证纯 gofmt churn);单步两 target 逐字节;三病形修前复现/修后收敛;词面素读可懂;一 P2=微额折叠×板次序残口。对抗 SHIP-WITH-FIXES——**K1「全仓唯一跨板 Σ 面」宣称证伪**(P1-1 周期节拍 Σ 混板:56.229 全来自 9163 板却注解 2955 板残差;P1-2 覆盖分母 Σ 混板:参数分叉形 257.635>窗 233.190 物理不可能,17.815=跨板非孪生双计);**P1 互指臂四门+去重臂零负向 pin**(G1/G3 witness 突变证活);P2 族=core_topology 不入指纹(topo 分叉静默混同)/混合 legacy-new 继承返病(重印 355.562)/◎ 总览多板同尺宣称假/双挂;K4 单步 10 形(3 target×zh/EN+模型级)独立 byte-identical;K5 设计突变 8/8 红 ALL-SHA-MATCH;K6 R2' 七处+tracediag hash e75bc1d4 双向 load-bearing。
**修补轮(十件)**:A1 周期节拍 Σ=只计可证同板行(identity-less 不可证不计;fused 56.229 句消失=唯一诚实形,单步保留逐字节);A2 覆盖分母=具名板三元组分组最大板胜出,败板行流 C-3 census(参数分叉 257.635→239.820 手算钉值,跨板双计 17.815 灭;**偏差备案:239.820>窗=胜出单板自身既有 legacy 超窗形,单步 9163 同值逐字节=单步零改保护,非跨板双计,立案候选见下**);A3「另有 N 条」按板分域(49→37);B 四门负 pin 全补(G2 两把尺合成形=唯一防线 pin)+G5 排查=可达(µs 等值 cum 分歧跨板对→ValueMirror 让位,构造形 pin,臂保留);C 去重让位扩 cross-channel 三 ref(E11 双挂灭);D core_topology+五 knob 入指纹闭集(不入清单逐一论证;产线 topo 分叉指纹分裂 pin 655f120d vs 33e69cde;**备案:对抗官 r8 4.783→3.806 在当前构建不复现=topo 变体经 R1 合法折叠,值通道未触**);E 继承退役=identity-less 席自成无名板(混合形 355.562 病句灭,具名板佩 chip 无名板保裸 legacy;纯 legacy 全剥离逐字节保绿);F 微额折叠三口(折叠键并入板三元组永不混折/折叠行佩 uniform 板 chip 灭残余裸序数域/被折成员跨板反指合并行一句代表);G ◎ 总览多板头「尺=各板目标线程 窗内墙钟ms·跨板不可相加」+行佩板锚(单板恒等);H EN anchor label 语言分叉+键分隔符全换 \x00(注入撞板形 pin);板身份单值源(runtimeTraceProjCrossBoardKey 删除,Σ/chip/互指/覆盖/折叠全走 runtimeTraceProjRankBoardIndexFor,exact-float windowKey 第二实现同退役)。
**记录/立案**:①E11 三面自相矛盾(❶+链上 lane+根因排序席+「无链上凭证整席降道◇」句同行)=基线既有定谳⑤族,归 XLANE-2 战场 rider;②单板覆盖分母 legacy 超窗形(239.820>233.190,单步既有)=独立卫生立案候选(物理不可能等待宣称,与本批无关);③LLM prompt 面(trace_board_summary/prose_lexicon)多板形两 #1 并存=义务面已由 §29.108 修补A 接住,prompt 词锚板化=后续候选;④h 金样(单 target 窗形)确定性面 byte-identical 覆盖,LLM e2e 未复跑=下批随行验证。

## §29.110 HEADLINE-ELIM 收账(2026-07-16;标题因诚实面三件+合并复核+修补轮六件;witness=customlogs/cust_span_runnable.txt)
**病(§29.104.14/.14.1)**:引擎四确定性面正确排类校验 #1(9.586ms primary),模型散文推翻榜首(「核心丢帧原因为 shadowhook…优先级反转」8.608ms #2)并以范畴论证降 #1 为「次要因素」;第二矛盾=「排除算力供给不足」vs E5 折算缺口席 4.843ms;放行机制=ENTITY CONSISTENCY 纯软教学无从核查+校验附注无「标题因 vs 榜首」并置臂。
**修(三件,遵 §29.104.13 纯披露永不硬拦)**:件1 skill 教学扩展(旧句逐字保留+G13 pin 不改仍绿)——分歧必须数值对比并排引用两已发布值(「无数值对比的分歧不是已声明的分歧」)/禁范畴论证降榜首(causality=self_deterministic 同权参赛=typed 已发布 token,「必须正视该数值而非归类回避」)/请求内叙事=调查线索永非排序证据;prompt 红线 checklist 全勾(R3-R7+SST,witness 实体/数值 over-fit 负 pin 锁死)。件2 校验附注并置臂(internal/orchestrator/prose_headline_elim_check.go,挂 S3' findings 通道)——嘈声侧封闭标题词锚族+否定护栏+实体 verbatim(comm-pid 全拼/typed 类词 zh 经 TraceRootCauseTypeZHLabel 单源出口),多实体/冲突/不可辨整臂静默;精确侧 chain 通道 rank==1(多板经 rank_board_target↔唯一 target_window_states 账主解 target 板);失配→「正文核心/首要原因=X(席位值);本报告根因排序#1=Y(有效归因 N ms)。两者不一致;正文如为有意分歧,应显式声明分歧并给出数值依据(如适用)」zh/EN(CR-4 禁词避让显式处置=弃「正文将/正文称」句形,语义+值对保留)。件3 rider=封闭算力/频点宣称族 × typed supply_fold_deficit_ms>0 席→并置 finding。R2' 零触发(纯消费既有注册键,论证核验);B5 零改;两臂零 Violation/零重试/零改写。
**合并复核(对抗+冷读一体)**:有条件通过——确定性侧全扎实(突变 7/7 红 ALL-SHA-MATCH/witness 复刻独立重做逐字/prompt checklist 复核过+不过矫审=分歧门词面证据在/值权属全 typed);**假阳猎杀 FAIL=2P1+4P2**:P1-1 arm-B 否定绕过(「不能排除算力供给不足」FIRED 且从否定句截出肯定子串=主动歪曲);P1-2 arm-A 实体词否定绕过(正文**正确**否定反转「而非优先级反转」反被指认宣称反转,加重因子=件1 教学恰推顺从模型写这句形);P2=「之一」成员形/「用户认为」归属形/arm-B 无主体绑定(他线程真话被并置)/复合席词面超宣称(E11 eff 2.004 含全额成分整体佩「折算」词)。corpus 佐证:而非/并非 54 处=P1 形语言学完全自然。
**修补轮(六件,全部静默方向门)**:①claim 位点否定/虚拟前视(不能/难以/无法族+若/如果子句头判定——单字「若」禁作窗口 needle 防误中宛若);②实体词位点否定护栏(「A 而非 B」=仅 B 位点废,A 存活正常参赛,机制自然涌现);③「之一」后缀+EN "one of" 前缀双语言同语义规则+归属前缀(用户认为/前置分析认为)废锚;④arm-B 句粒度主体绑定(句内外来 tid≠缺口席 tid→废);⑤复合席恒引门控本值 supply_fold_deficit_ms(E11 形 2.004→1.911,措辞「供给折算缺口 X ms」;witness 4.843=纯折算席逐字不变);⑥锚 infix 禁结构助词(「多个核心被抢占的原因」静默,「核心丢帧原因」保命中)+尾句取 spec 原形「(如适用)」。13/13 pin 绿。
**备案**:①CR-4 句形冲突处置(账本原文句形 vs 任务模板「正文将」,取前者);②witness 标题句提取实走类词 zh 车道(comm 无 tid),批概要「comm-pid 全拼」话术过诺勘正;③M6 词典单源守卫=复核制非 pin 制(字节等同手抄副本字符串 pin 固有界);④G13 旧句保留偏差=一个英语连词迁移(ε,序列比对核验)。

### §29.104.15 立案 ELIM-GAP:◎ 可消除量总览静默丢席(2026-07-16,客户高优先级;witness=customlogs/cust_total_del.txt)+XERR1 客户回放验收 PASS
**客户指认**:重跑报告(cust_err1 同帧 58558 复放)答案与因果投影树正常,但 ◎ 窗内可消除量总览缺失根因排序#2 席=[GT]ColdPool#9-48667 runnable 8.211ms(全额有效归因)。**工件面确认**:E15(+2) 为合并行(source=wakeup_chain 聚合,明细 type 面「runnable」与「优先级反转候选」影响点注分离,带「窗内 runnable 合计 98.485,链上仅覆盖最大片段 8.211(8%)」覆盖注,同线程 ◇ 孪生 E28);按值 TOP5 非自身链上行=10.853/8.211/2.939/2.350/1.922,E15 应列第二 → 被 population 门排除非值切;尾注「语义类持席行另有 2 行未入榜(TOP5 值切)」只数语义行=非语义持榜席**静默消失零披露**(◎ 板=承诺面,比值切更糟)。伴随候选:①◇ 段非语义切席零披露(只列 E27,E29/E31/E30 消失);②E22(+2) 折叠行词面「窗内无调度数据·链止」含持榜位#12 成员;③E16/E18/E19 有效归因微大于窗口投影口径待归因;④头行「等待 71.029ms」vs 四态 91.560 口径待核。排查批在飞(净室快照,population 门 census+回归窗定位+15 榜席逐一去向表+泛化修向)。
**XERR1 客户回放验收 PASS(§29.107 残留项关账)**:同帧重跑 ⊗ 假阻塞席(199.992ms 冒充)消失,自身 sleep 8次 64.301 诚实席+覆盖注在场,客户评价「答案和因果投影树都很正常」——四防线产线实证,cust_err1 案客户侧闭环。

### §29.104.16 立案 RANKDIS:「rank」词汇多族复用致模型误读排名矛盾(2026-07-16,客户 witness=customlogs/cust_span_vs_prio.txt)
**客户问题**:模型校验轮 transcript 显示其把系统输出读成「排名不一致」反复 reconcile。**主会话发射面直查定谳**:根因板自身一致(#1 类校验 9.586/#2 反转 8.608),病=同 payload 多套序数面共用「rank」一词,模型 grep 原始 blob JSON 撞车:①state_drilldown 手递面逐行 `rank=1..N`(状态 Top-N 序数,trace_query.go:5013)——witness 第 6 轮模型原话「rank 1-8 全是与目标无关的 s_sleep 线程,rank 9=top_running 68.818」=把状态手递序数当根因板;②auto-window 候选结构 `Rank int json:"rank"`(trace_query.go:877)第三族;③根因板链上/邻近双通道共用 `rank` 键(XLANE-3 设计=(rank,chain_relevance) 联合辨义,typed 消费者无恙,裸 grep 见双 rank=1);④类校验族两套总账无 scope 词毗邻(window_stats 全窗跨线程族 14 spans/13.247ms vs 榜 #1 席=目标线程自身族 8段/9.586ms,模型两轮怀疑孰真)。**RANKDIS 修向(噪音从源头消除)**:①state_drilldown 序数词改名(state_rank,文本+JSON 双面,golden 随改);②auto-window 候选 rank 改名(window_rank)或加 scope 词;③根因板 raw 面自描述(rank 邻位通道词或 payload 头注「rank=各 section 内序数」);④语义族 summary 句加 scope 限定(「全窗跨线程 N 线程族;榜席仅计目标线程自身族」);词面/schema 改动走 B5/golden/R2' 纪律。队列位=ELIM-GAP-FIX 后(同为客户模型误解族,ELIM-GAP 用户标注优先级更高)。

### §29.104.16.1 RANKDIS-SWEEP 五族普查定谳与编队(2026-07-16;全文=docs/design/rankdis_sweep_20260716.md)
**普查(用户指令「排查给模型信息的用词是否还有其他不清晰场景」)**:五族并行审计(序数/值口径/scope/关系通道/JSON 字段名),46 finding 去重 **27 件=13 T1(witness 实锤)+5 T2+9 T3**,12+ 关键发射点汇总官只读复核零虚构。**T1 头部**:M1 裸 rank 键复用比 §29.104.16 立案更广(engine Summary/JSON/caveat/清单行/标题/AbsorbedItems 六处补漏);M2 wakeup_chain:path#N 序数字形=第四凶实锤(客户双 Rank#1 的另一半根源;#N=分支身份非重要性);M3 **GATED-CAL 新立案**=gated 合成值冒充族四面一根(「全额」假盖/窗口投影列违图例/裸「有效归因X」/◎ 裸 runnable,同根 query.go:15628-15632/12144-12153);M4 「链上累计」戴在自身线程头条→模型虚构跨线程 credential host;M5 值词库分裂簇(wire↔显示词无映射=gated_runnable/sum_disjoint 进客户正文;eff↔cum 四名一值→模型自铸「直达」;**Description 教的 projected_total_ms 键在 rank 行 JSON 不存在=幻键**);M6 trace_mark_category 总账无 scope 词(count=14 total=13.247 跨线程全窗+thread= 实为 top_thread→即客户五轮调和源);M7 同席双 token(candidate/runnable_wait);M8 反转 token 三家显示词分叉+◎ 强席显弱词;M9 「深度未解析」对 L1 行撒谎(C6 词族第四面);M10 置信档词=车道常量折词(板#1 中 vs 板#2 高,反向助推推翻板序);M15 TraceNoteKeyRank typed 键被状态板序借用(空 chain_relevance 默认链通道,现防线=prompt 去重巧合)。
**编队(五批切分,详表见 sweep 文档)**:①RANKDIS-EXT=A1(rank 键三面改名+双通道行内 rank_channel)+A2(path#N→branch=N)+A3(state_rank 专用键,R2' 全走)+B6+B11+C8;②GATED-CAL=B1(退化臂精确门 GatedRunningDeficitMS>0 禁盖「全额」+构成式+口径注记+◎ 注记臂)+B2(自身席头条改有效归因+族口径词);③值词库教学批=C1+C2+C3(defaults.go 教学模板+图例关系子句+Description 三处重写含幻键修正,单独批+金样 12 趟+prompt 红线 checklist);④反转词位单源批=A5+C4+C5(INV-SUPPLY §29.61.11 推广);⑤scope/卫生批=B3/B4/B5件/B7-B10/A4/A6/C6/C7(并入 DISPLAY-HYG)。**旁路两件**:M12(1.911 跨席误绑)喂 CR-4 验收;M13(Σ24.3 跨席求和)归 ELIM-GAP 核对。**待裁两件入裁定池**:M18 复合分数进 _ms 键族(io_pressure Score/block_io/rank_impact_ms,wire 键改名需裁;§7.30 S1 只关了文本面,代码注释自认留裁);C5 置信车道常量收敛(行为变更)。clean_faces 五族合并清单在册(防覆盖缺席);background_rank 独立键=修向正面样板。

## §29.111 LOCKNS-FIX 收账(2026-07-16;持锁 tid 识别泛化六件+G1 用户裁定落地;双复核+修补轮五件)
**病(§29.104.10/.12)**:客户两 ART 锁形+东湖容器化 ns-tid;排查=LCK-2 三级梯已建、两形全解析、容器形已正确,四缺口 G1-G4;G1=形B 撞号错指向量(容器 ns-tid 撞宿主无关线程时梯①直解 conf 0.72 零披露,rank 席主体换错人佩持锁词)。
**修(六件)**:件1 G1 门(§29.104.12.1 用户裁定原样落地)=query.go nsDivergent 两整数精确门(SpanPID>0∧TGID>0∧SpanPID≠TGID→跳梯①入推导梯②/③;identity-ns/任一整数缺失=fail-open 梯①逐字节;hostTidCollision 撞号见证仅词面零值面);§18.E pin5 重写=EVOLUTION RECORD 逐字引裁定原文(旧反向形转撞号 pin)。件2 形A vendor 前缀词边界(词首边界=前一字节非字母数字,additive 注册行;premonitor 负向)。件3 形态注册表化(四行 typed 表,三臂平移逐字节等价)+未知形 fail-open(\bowner\b 软筛只驱披露零门,OwnerKeyUnregistered note+「owner 未解析(形态未注册)」zh/EN)。件4 哨兵/无主披露全分支(else 臂补 caveat;明细 typed 五条件门;哨兵数字永不上句面)。件5 rider 核查=0.72 是 span-typing 等级非 holder 置信(推断 holder 全部封顶 ≤0.70,「载有推断 holder∧conf=0.72」不可构造——对抗官全赋值点排查证),pin 钉现状。件6 OM-10 rider=unification 半场就地关账(note 升格 hard_consumer 四面+Node 字段+互证括注「(发射对×收尾唤醒两道互证)」;process 半场显式留 IC-L)。R2' 两 key(blocking_owner_key_unregistered 新+holder_ns_unification 升格)七处全走。
**双复核**:对抗 SHIP 零 P0/P1/P2——独立撞号构造(全新值)三路验证/反向误杀零(identity-ns 跨树逐字节)/门键畸形 payload 全防(B|0| fail-open、负数超大数 invalid 无行、int31 合法分流无 panic)/262 行 fixture 普查 34 变更全=件4 设计内后缀/10/10 突变红 ALL-SHA-MATCH/XERR1 同区 26 pin 零回归。冷读 PASS 一 P2——逐 hunk 零私货(门恰=裁定两整数零私加维度);四组端到端(donghu 容器形修前后逐字节=38414→宿主 ransmitThread-18130@0.67 保持/tieba 哨兵形=恰 3 行后缀追加/撞号三路/vendor+未知形);泛化审=纯 identity-ns Android 形全链逐字节恒等+本地语料 owner-vocab 23 span 零误报;P2-F1=撞号形明细面残留 legacy「不在本 trace」假句与同板 Summary 撞号句矛盾(撞号见证只上 Summary 字符串无 typed note)。
**修补轮(五件)**:A(P2-F1+P3-F7 同族根修)=typed owner_tid_presence note(枚举 absent/present_collision/present_comm_mismatch,来源=引擎既有判定位 idx.tidPresent×lockOwnerCommCollides×nsDivergent 三分支穷尽,零新启发式)R2' 七处全走(rank hash 重钉 6aa91e82),明细面 presence-clause 分叉(collision→「在本 trace 中存在但为容器命名空间撞号,非持有者归因依据」/comm_mismatch→「在场但线程名不符」/absent+缺 note+未知值=legacy 逐字节 fail-open);B(P3-F6)=四处 "never the holder" 范畴断言软化为归因语义(a collision can only coincidentally match,对齐裁定「撞对只是巧合」);C(P3-F3)=件4 标签句首重复去重;D(P3-F4)=OM-11→OM-10 注释勘误两处;E(冷读残留引擎面半边,主会话回派追加)=comm-mismatch 形引擎 Summary presence 句同款分叉——梯③ caveat counterpartWakeupEdgeCaveat(query.go:20208)+梯② presence clause(ns_span_derivation.go:485 三臂 switch,collision 臂逐字节保件B 词面=判定位等价单点)按同一 typed 判定位分叉,「present in this trace but its thread name never matches the payload's owner comm — not a holder-attribution basis」;absent/空/未知值 legacy 句字节精确 pin;payload-less 形不变;pin=TestLocknsRepairPresenceCommMismatch/EngineCommMismatchWordFace/PresenceAbsentKeepsLegacySentence,LCK-2 pin1-10 全保绿。
**备案**:①对抗 P3 四观察=混形 dispatch 插位(富文法优先教义,语料零命中,dispatch pin 冻结)/TGID-less 采集车道门恒 fail-open(既有盲区非本批,与梯②同整数同盲区)/形A comm 碰撞旧词面债(已随修补轮 A/E 收)/件4 显示词轻张力(process 半场=IC-L 现状一致);②冷读 P3-F2=phantom-unresolved 两面不对称(明细多说=additive 披露方向正确,不修);③P3-F5=注册表 row3 插位备案同①。
**客户面**:东湖容器撞号错指向量灭(三路诚实处置+审计位保留);vendor 前缀形A 解析恢复;未知锁形 fail-open+披露;哨兵全分支披露;Android 非容器 trace 全链逐字节零影响(机械证据)。

## §29.112 ELIM-GAP-FIX 收账(2026-07-16;◎ 总览静默丢席修根四件;双复核双 SHIP;客户高优件三号)
**病(§29.104.15)**:客户 witness(cust_total_del.txt,cust_err1 同帧重跑)根因排序#2=E15(+2) 8.211ms 在 ◎ 完全缺席零披露。排查定谳:席位#2 骑 Node.Rank(R1 same-fact absorb 席位回填第四载体,§29.67)——徽章/选举/行2 全认,唯 ◎ 种群臂只认三臂;15 榜席 6 席静默消失(#2 门吃+5 席非语义 TOP5 值切零披露),◇ 8 席 7 席静默;自 ◎ 板诞生(0419a85f)即有,三历史点净室复现。
**修(四件)**:件A 种群臂第四臂 `Node.Rank>0`(与徽章/选举/成因节点门同一精确 typed 信号,§29.30.1 单门原则 ◎ 侧补全;载体闭合=构造性穷尽:Rank 唯一赋值点 takeOrdinal+显示侧 R1/SEM absorb 回填,第五载体不可构造——对抗官证)+**结构 pin 三重闭合**(载体闭合/披露闭合/闭合恒等式=渲染成员数+计数披露==全种群,静默消失=0 由算术保证,真渲染 fence 遍历非模型层,挂四形常驻);件B 值切计数披露逐通道泛化(「⛓/◇ 持席行另有 N 行未入榜(TOP5 值切),见明细」,精确信号=board 切片本身零新判据;切席语义行并入通道计数不双计,语义/◇-max fallback 席保留不计切,fallback 跳过=load-bearing 突变证);件C 折叠词面读 typed 真相(runtimeTraceProjFoldSeedGapMasked=OnChainOverflowFold∧¬MergedAllDataGap 三消费点:行内词/◌ 记号/明细影响形态;纯 gap 折叠+独立 gap 行逐字节负臂;另 census 四未声明消费点=两结构不可达+gapSeatDuplicate 行为差方向诚实备案);件D 口径词「(发生段账目)」(门恰=C5 两臂∧eff>cum,新 mark+词条 lockstep 双向,print-equal/eff<cum/承自 lane 负臂),兑现关键指标图例承诺。R2' 零触发(纯显示层,新 mark 走 NEW-7 纪律五处)。
**双复核(双 SHIP,零 P0/P1)**:对抗——四件证伪未遂;K1 误纳攻击不成立(通道归属由 stanza 权威独立判定与臂序无关;◇ Rank>0 行零 ⛓ 混入;多板形 R1 absorb 随迁板身份佩板锚,恒等式闭合);闭合恒等式活(MUT 假计数即红);10 突变全红 ALL-SHA-MATCH;跨树 engine-real byte-diff=◎ 面外含全部树面 byte 恒等。冷读——零私货零缺失;witness 15 席去向手推逐数吻合(⛓ 6+8=14/◇ 1+7=8,修前暗席=6 与立案相符);E15 修后入板第二 bar(8.211,bar 比例 0.76 合理);「排除≠消失」全称在 witness+四真实窗未找到反例;E22 混合折叠修后 ◦ 中性形+图例双向随动;tieba 真窗同病灶活体(8.049/7.100 入板)实锤入 pin。
**裁定池新增(对抗 P2,既有病非本批)**:XLANE-1 represented-by-chain-seat 降道卫星以 root_cause_<tier> predicate 全值 ◇ 条入 ◎ 板(引擎 RSPA 降道在 rank 分配前,ladder 无跳臂),与其树面句「同段物理时间已由链上席全额代表…不重复参赛」同报矛盾=C-1 §29.88.10 视觉双计族。推荐=显示层 ◎ 种群排除 ChainAnchorRepresentedByChainSeat 行+专用披露脚注「已由链上席代表(降道),不参与汇排 [E#]」(typed 精确门,值面/引擎序数零动,闭合恒等式随臂扩);因涉及「降道席该不该占 ◎ 条」语义请裁。
**P3/备案**:①EVOLUTION 注释超宣称勘正候选(Binder 13.898 修前已在板,真被救=8.049);②「(发生段账目)」图例「略高于」无 typed 上界背书;③持值行/持席行词库小分叉+计数句族未入 ◎ 词条;④TOP5 字面 vs 常量无恒等 pin(沿袭债);⑤披露闭合断言臂在四挂载形上休眠(产线行为正确,建议补 stale 口径持席形);⑥RootCauseFamilyObserved=false 前置条件边缘;⑦witness #13 席全面缺席=census 候选(非本批引入);⑧头行 71.029 未成行质量披露=候选;⑨rcr.go:1286 fail-open 预点既有行为——①-④⑨ 归 DISPLAY-HYG,⑤⑥⑦⑧ 留候选。客户侧同帧复放=外部回访项。

### §29.104.17 裁定池六件终判(2026-07-16,用户逐条批复)
**①XLANE-1 降道卫星入 ◎**(用户:「按推荐的来,按最优的做」):◎ 种群排除 ChainAnchorRepresentedByChainSeat 行(精确 typed 门)+专用披露脚注「已由链上席代表(降道):N 行 [E#…]」并入 §29.112 闭合恒等式;零条目+脚注形;值面与引擎序数零动。编队=并入 GATED-CAL 批(同 elim 面)。
**②M18 复合分数 _ms 槽**(用户:「批准第二级 wire 键改名,如果无需兼容旧键更优则无需考虑兼容,按最优的做」):复合分数(io_pressure Score/block_io 综合/rank_impact_ms 族)迁出 ms 语义键槽至专用 score 键;实施前先 census 全部读者——**零 Go 读者即干净改名零兼容臂**(噪音从源头消除最优形),有读者则读入兼容;伴随 typed value_kind 自描述视需要;R2'/golden 随改。独立小批 RANKDIS-M18。
**③C5 置信档词**(用户:「按建议的来」):先做图例披露句(「置信档=各证据车道数值阈值折词,不作跨行强度比较」),归值词库教学批(C2 图例族);车道常量统一标尺重标定=缓,观察客户复放后再议。
**④XLANE-2 值面扣除**(用户:「按推荐的来」):披露式拆分——自身缺口席行内列「其中 X ms 与语义席[E#]重叠」子句,主值零动(值通道零修改红线);硬扣除不做。并入 XLANE-2 批。
**⑤XERR1 payload-typed 值收敛扩展**(用户:「同意,按推荐的来」):真锁行(BlockingKind 在场)值收敛=waiter span∩窗 Σ(sleep+D+iowait),包络保 SpanEnvelopeMs+披露,F-2 同基门+fold 值胜出区间纪律沿用(§29.107 基建);**改值通道且改榜序=用户已准**;独立批 XERR1-EXT,配双复核。
**⑥ε-overlap 门**(用户:「按推荐的来」):**不加门**——部分相交保链 fail-open 维持,合成 pin 看守;客户复放出现自然 ≪10% 形再启用。零代码,账本记结即闭。
**队列更新**:RANKDIS-EXT 收账(§29.113,在飞)→GATED-CAL(sweep B1+B2+裁定①)→XERR1-EXT(裁定⑤)→RANKDIS-M18(裁定②)→值词库教学批(C1-C3+裁定③图例句)→反转词位批(A5+C4+C5)→SUPPREF-TOL→XLANE-2(E11 rider+裁定④)→DISPLAY-HYG(卫生 11 件+ELIM-GAP P3 群+裸 window= 残余)→HULL-CRED。

### §29.104.18 立案 DISPLAY-WRAP:fence 折行器盲折 chip 链(2026-07-16,用户指认;witness=.codrax/output/20260716-183011.023-26159.md)
**用户问题**:row-2 chip 链(「算力供给候选·自身·墙钟席·根因排序#1·窗…·板锚 …·置信高」)被宽度硬折,尾悬 `·`+「置信高」孤行——可否分行显示?**普查(同报告四形)**:①chip 链宽度盲折尾悬 `·`(×6,XLANE-3 窗+板锚 chip 加长后高发);②孤行短尾(置信高/置信中/E36(+1));③证据 E# 顿号清单折行孤引;④散文与数值拆行(「受热限压至 ␠\n1.88GHz」行尾空格+数值孤行)。病根=fence 折行器按字符宽度盲折,不认 chip/token 边界。
**修向(病族级)**:折行器 token 感知——①`·` chip 链只在 chip 边界断行,断点分隔符不悬尾(续行以 `·` 起或组式分行);②row-2 超宽时按语义组分行(类型/自身/席位组 | 窗·板锚 板身份组 | 置信·有效归因组)=用户所请「分行显示」形;③短尾防孤(末 chip 宽度不足并入前行或整组下移);④不可断对(词+数值/单位、E# 引用、顿号清单项)非断点。影响面=只动已超宽行,golden/B5 含长链 fixture 重滚需逐处论证;全部 pin 断言若跨断点需 survey。队列=§29.113 收账后优先小批(用户直接指认,先于 GATED-CAL)。

### §29.104.18.1 DISPLAY-WRAP 普查全形谱与编队更新(2026-07-16;全文=docs/design/display_ux_catalog_20260716.md)
**普查(用户指认「还有很多 UX 不平衡」)**:witness 报告 1014 行逐行冷读,33 形=A 组 7(影响理解)+B 组 14(费劲)+C 组 12(美观)。**结构性根因=B1**:单板单窗报告里窗/板锚 chip 全文同值重复 39/38 次——chip 不减肥,折行修不完;伴 B2(折算口径长句 30/26/14 次逐节点连拼)+B3(图例已有的规则句全文重复 ×5/×5/×4)。A 组头部:A1 CJK 词内断字(「为主/证据/对象/该簇」拆行,比立案形4 重,≥8 处);A2 树 chip「(全额)」与明细构成矛盾(E28=2.181 全额+1.248 折算佩全额单口径词)=**GATED-CAL M3 同族,移交该批**;A3 排名席有效归因无推导(疑盲折吞行尾);A4 同头三行不可区分(树行丢类型词);A6 计数当量违图例带 ms 后缀+裸值无口径前缀;A7 窗口边界三写法两精度舍入不一。
**DISPLAY-WRAP 批编队(更新)**:①折行器 token 感知(A1 禁 CJK 词内/括号内断+形1-4 chip 边界断+分组分行+C1 尾悬清理);②B1 单板单窗 chip 省略(板头/树头标注一次,行内省略;多板才逐行携带=XLANE-3 消歧本义);③B2/B3 节点内口径句去重+规则句体归图例、行内留 chip 词;④A3(若=盲折吞行尾同根修复)+A4 树行 head 携类型词+A6 计数当量词面(图例承诺违约=正确性)。**移交**:A2→GATED-CAL;A5/A7/B4-B14/C2-C12→DISPLAY-HYG 按 catalog 清扫(B4 图例裁剪/B6 表格成员行出表/B7 附录超长行/B11 三制混用等)。队列位不变=§29.113 收账后第一位。

## §29.113 RANKDIS-EXT 收账(2026-07-16;rank 序数词族源头收口六件;双复核+收尾三件;金样 h2/h3 4 趟 PASS)
**病(§29.104.16/.16.1)**:「rank」词在 payload 至少四族复用(root_cause 板双通道共键/state_drilldown rank=1..N/auto-window candidate/hotspot)+wakeup path#N 序数字形——客户模型把状态手递序数当根因板、把 path#1 读成排名第一,正文双 Rank#1(witness=cust_span_vs_prio 两件)。
**修(六件)**:A1 rank 键三面分叉=state_drilldown rank→drill_rank(JSON+tool 文本+engine Summary+ledger fallback 四面)/auto-window→window_rank(附带**意外真修**:旧清单行 `- rank=` 恰命中 ledger 段盲分发前缀=基线会铸幻影 root_cause 记录,改名顺手关死)/hotspot→density_rank/root_cause 板行内 rank_channel=chain|adjacent 自描述(RootCauseRankOrdinalChannelWord 纯委托序数分配器单源,rank=0 行零词字节保形,wire 零新键)/旧词面读入兼容臂 fail-open+pin;A2 ClaimKey wakeup_chain:path#N→branch=N(mint 单点,supplement order 双臂 fail-open 覆旧 path#N+裸键,handoff/projection 逐字透传自动同步;旧工件跨 turn 去重无破坏=ObservationRecord 零落盘回读 census 证)+Description 恰一句负向教学(branch 编号=分支身份非排名)+golden 重钉(字符级 diff=恰一句插入,EVOLUTION RECORD 在场)+prompt checklist 全勾;A3 state_drilldown 专用 state_rank typed 键(display_only)+投影 Node.Rank 解析 Predicate 门(HasPrefix root_cause,关闭「状态板序冒充席位」结构通道——原防线=prompt 去重巧合),R2' 七处全走;B6 total_scope=single_state|all_states(typed Source 封闭集单源分叉,构造点唯一六值全覆盖,未知省略不猜);B11 rank 行 row_window=+interaction 行 first_last=(零 Go 读入方 census 证,anchor_window typed lane 未触);C8 侦察修正 sweep 病描(root_cause 双 lane 本已同铸 position;真分叉=state_drilldown 对)→ledger lane 统一 position 铸+双 mint 点教学注释(malformed 同 rank 撞 ID 病同关)。
**双复核**:冷读全六件 PASS 零私货零缺失(端到端双树对照:drilldown 三面/ClaimKey 透传/双板 rank=1 各佩通道词/旧词面重放 identity 恒等/Description golden 单一 insert;词面素读过;裸 window= 残余 15 处 census→DISPLAY-HYG;4 P3=新句与既有 ranked 词面张力/rank_channel 无教学句/handoff ID 与 claim 两词并存/first_last 稍简,归教学批与 DISPLAY-HYG)。对抗 10/11 突变红 ALL-SHA-MATCH(唯一幸存 M5a=typed lane state_rank 铸点无 pin→收尾件1 已补);K1 兼容臂/K2 结构隔离(Predicate census=rank note 全仓仅两铸点全 root_cause 族,门无漏堵)/K3 golden 合法性/K5 投影渲染 blocks 字节恒等全过。
**收尾三件**:①typed lane state_rank 铸点 pin(RichNotes 含 state_rank= 且禁裸 rank=);②tracediag render_key_first.go R2' 第 7 处 adjudication 注×2(drill_rank/density_rank tag 改名,hash 不变零重钉,LOCKNS 先例);③金样 h2(dstate_dma_fence_triform)×2+h3(iofam_one_seat)×2 **全 PASS**(Description 新句效应验证)。
**备案**:①C8 侦察修正 sweep 病描(上);②账本 §29.104.16 修向①措辞漂移(state_rank(文本+JSON)→实施按后出 sweep 编队 drill_rank 文本/JSON+state_rank note lane);③window_discovery.go json:"rank" 零-LLM 面豁免;④环境=复核期同树并发写者一次瞬时 build 竞态(复跑即绿,「突变复核期禁并发构建」教训再证);⑤golden 变更恰 2 处(Description 一句+note-keys 一行)/B5 渲染 fixture 零重滚(投影面字节恒等实测)。

## §29.114 DISPLAY-WRAP 收账(2026-07-16;用户指认显示 UX 修根四件;双复核+补 pin 四件;witness=20260716-183011 报告+33 形 catalog)
**病(§29.104.18/.18.1)**:用户两次指认 row-2 chip 链宽度盲折(尾悬 ·/「置信高」孤行);普查 33 形,结构根因=B1 单板报告 chip 同值重复 39/38 次。
**修(四件)**:件② B1 修根——诊断推翻「门收窄」预案,真凶=**幻影无名板**(同段 lane-twin 折叠+SEM-LEAD 语义折叠两处显示级序数转移不携板身份→3 个 fold host 佩 Rank>0 空 target→XLANE-3 件E census 诚实铸无名第二板→单板翻多板佩 chip);修=序数转移必携板身份(XLANE-3 件1 纪律落两处,node.Rank<=0 收养臂内,已入座宿主不覆写),chip 佩戴门/census 零字节改;witness 板锚 40→0、窗 chip 38→0,真混合形具名席照佩(XLANE-3 pin 全族绿);「头部单点声明」半件停做=与 TestXLANE3SingleStepKeepsLegacyForm pin 冲突(⊚ 树头+分析窗行+◎ 尺行已三点单次声明)。件① 折行器 token 感知——CJK 词游程 atom(A1 词内断灭,「为主/证据/对象/该簇」整词)+`·` 入 openPunct 不悬尾+row-2 IdentityGroups 三组分行(类型席位组|窗·板锚 板身份组|置信归因尾组,组间断组内不断续行 · 起,join==原行字节恒等由构造)+不可断对(词值 ≤28/E# 引用 ≤24/顿号清单 ≤30,全部行宽二次封顶禁 rune 硬切)+短尾防孤+行尾空格 Trim(C1 全灭)。件③ 口径句节点内首现全拼+后现短词(封闭二短语表→「分簇口径同前」「按前述基准」,30/26→节点级恰一全拼;新 mark×2+图例双向;「受热限压至」节点内恒 ≤1 不入表)+三规则句体归图例行内 chip 化(整席降道×5/同源二分×4/覆盖集不同×4→0,值+E#+席角色全保,图例词条既有零补)。件④ A3=self stanza 补 Q1 有效归因 lane(E6 形 2.116 上树;fail-open 席口径词宁缺勿造)/A4=head 状态词覆写门收窄(三同头行分化=优先级反转·可运行等待/调度延迟/runnable,与表同名)/A6=计数当量词面(「计数当量X(非墙钟)」统一,单段 range 卸 ms 尾,墙钟族字节保形)。
**双复核(双 SHIP 零 P0/P1)**:冷读——33 形核销表(修复 9/自愈 1/移交 GATED-CAL 1=A2/移交 DISPLAY-HYG 22);数字守恒 diff 级证明(数值 token 多重集差恰三项全声明,E#/置信词多重集 SAME);修后素读=用户指认两形实测转好、指代回查全通、板身份三点可定位;golden/pin 重滚 33 hunks 逐处=设计内(rspaFenceContains 放宽=可接受但负向 pin 保 raw 更严)。对抗——19 突变 13 杀;件② 根修 M9/M11 双红+真无名板不误伤+XLANE-3/ELIM-GAP 全族独立复跑绿;4 P2=纯 pin 缺口(组分行发射位/EN 悬 ·/A3 ConsumedEffective 负臂/假 pin)→补 pin 四件全落(M1-M5 突变复验全红)。
**事故披露(第五起恢复纪律违规,行为面已封)**:补 pin 轮 M1 复验后 agent 误用 git checkout -- 回滚(违「突变恢复只用 cp 副本」三犯),tree.go 批改动被抹;从会话编辑历史 7 阶段脚本重放重建(每步唯一命中断言)+自述两处对齐修正 → **重建后 blob(0893fd93)≠冷读锚定(b52fb518)**;主会话独立三角验证=零突变残骸 grep(if false/false &&/witness_token 全 0)+DisplayWrap/XLANE3/ElimGap pin 电池绿+gofmt 净+全套绿,行为契约成立,偏差判=空白级,准入。教训重申:**突变恢复只用 cp 副本,git checkout 在飞树=禁**。
**残留/移交**:明细 dedup 不点 mark 图例缺席结构口+◎ 脚注 105 格(冷读 P2×2)、P3 六项(同批两档空格归一并存/new10 squash 弱化/footnote 双计数当量/`· runnable` 孤 tag 行/[E5]= 悬行尾/A3 计数族值形分叉)、gofmt 脏三文件(HEAD 既有)、「等待对象」双发(基线既有)→全部 DISPLAY-HYG;A2→GATED-CAL。

## §29.115 GATED-CAL 收账(2026-07-16;gated 合成值冒充族修根三件+裁定①落地;双复核同 P1 合流+修补轮四件)
**病(sweep M3/M4+catalog A2+§29.112 冷读 P2)**:gated 复合席(runnable 全额+running 折算)四面冒充单口径——树 chip「(全额)」假盖(A2 活体:3.429=2.181+1.248 佩全额)/行3 构成式缺席/表格列违图例承诺/◎ 裸 runnable 词;自身/语义席头条戴「链上累计」跨线程词(模型虚构 credential host 之源);XLANE-1 降道卫星以全值 ◇ 条占 ◎(与树面「不重复参赛」句同报矛盾)。
**修(三件)**:件1 单一 typed 门+单一词源四面同词——GatedCompositeSeat(inversion∧runnable>0∧deficit>0∧【修补轮】round3Equal(值,runnable+deficit))+GatedCompositeShortWord「构成,见明细」;退化臂禁盖「全额」(修补轮三臂:构成恒等→构成词/单分量恒等或 deficit-free→保「全额」/皆败→宁缺勿造);行3 构成式放宽(「running 原始未发布 → 计入 X(口径)」诚实缺席子行,零省略不造 0.000);表格 cell 注记+图例反转席限定;裸 tag 保底(C5 前提改读 ConsumedEffective 实际);◎ 注记臂+类词臂推广(=行2 同 composer,INV-SUPPLY 总览侧)。件2 头条 fork——ConclusionSelfSemanticSeat(SemanticClass∨self basis 封闭集)→「有效归因 X = 合计(共N段,同线程)」(恒等门=发布值==eff,gated 拒佩,semDual 保自车道,eff-less 入回退链);「链上累计」只留真下钻链席 verbatim。件3 裁定① 降道卫星出 ◎——ChainAnchorRepresentedByChainSeat 种群排除(门与脚注 census 同体不可分叉)+披露脚注「已由链上席代表(降道):N 行,见明细 [E#…]」zh/EN 并入闭合恒等式(elimGapAssertRepresentedLane);值面/引擎序数/树面席/互指句零动。tracequery 零改动=值通道零动 diff 级证明(三窗 E#/榜序/◎ bar 值多重集全等);零 golden 重滚;R2' 零触发;新 mark NEW-7 五处。
**双复核(双 SHIP-WITH-FIX,同一 P1 合流)**:冷读——六面 SHIP;A2 四面对照实证;多板 fused 恰一行 diff=头条声明面;数字守恒;P1=退化臂门(deficit 在场)对引擎现实不健全。对抗——K1 三窗 diff 级零动证明;K3 头条新词对 §29.110 标题臂零干扰(字形级不相交分析证明);K4 门脚注同体(只删脚注→恒等式独立红)+多板脚注=count-only 非 Σ 病;7/7 突变杀;K6 stash 事故核销(fsck 残骸=纯 gofmt hunk 零内容损失);**P1 活体=tieba E15 SharedPreferenc-61843(eff 8.049==gatedRunnable,deficit 0.073 发布未计入)——修前「(全额)」真话被首版门换成假构成词+悬空「见明细」指针**。
**修补轮(四件)**:A=退化臂三臂恒等式门(E15 保「全额」真话恢复/A2 照佩构成词/皆败宁缺;EVOLUTION RECORD 记首版反向新谎)+两 pin 重钉引擎实铸形(tieba E15 负臂+donghu A2 正臂,合成不可铸形废弃=fixture 实铸教训)+「deficit=0 假设」注释三处勘正(单行 mint 确 deficit=0,折叠载体可携未计入 deficit,安全性来自恒等式);B=GatedCompositeSeat 谓词恒等式臂(三消费面一次修,败则 ◎ 裸词/tag 宁缺);C=「佩构成词⇒拆解可达」结构 pin(全报告扫描绑定 E# 断言明细拆解在场,双引擎电池常驻=悬空指针病族关死);D=注释勘正+represented-lane EN 计数入恒等式。
**备案**:①stash 事故(批中误发 git stash 即刻 pop,fsck 核销零损失)=纪律六犯谱系,零 git 写红线重申;②P3 群→DISPLAY-HYG(多板脚注不分板计数/脚注与树面词族小分叉/◎ 尺随排除重标定/gated 席头条无构成词/◇▒ residual 裸 tag/deficit-only 裸 tag);③E18 车道声明内升级(短口径词→完整行3 方程,信息严格增);④h7 头条 74.915→65.912(自身席跨线程词卸下,与供给折算缺口自洽,oracle 零波及)。

## §29.116 XERR1-EXT 收账(2026-07-17;裁定⑤=payload-typed 真锁 blocking 值收敛扩展;双复核+修补轮三件;榜序变更用户已准)
**裁定⑤原文落地(§29.104.17「同意,按推荐的来」)**:件1 值收敛扩展=convergePayloadTypedBlockingRowValue(值=waiter 在 fold 值胜出区间 Σ(sleep+D+iowait),同 buildCriticalBlockingThreadState 零第二实现;包络保 SpanEnvelopeMs;basis 双 lane 铸 wait_segments/span_envelope;F-2 同基门+件F coverage 沿用;fail-open 两臂=零宽 span(tieba 3 真例,「theoretically unreachable」注勘正)/无时间线→保包络+span_envelope;Summary 换径单词源 helper,payload-less 臂重构字节恒等)。件2 词面联动=值口径/⚠/覆盖行按 basis 双 lane;锁词族四 fork 保留(kind fork 补 BlockingKind 精确门);holder-subject referent「(账目主体=等待方)」;payload 覆盖行无数字句(收敛区间不上 wire,不造分母);互指臂 payload 整跳(宁漏勿假指=包含证明会证错区间);twin-port 扩 6 既有注册键 zero-drop。件3 榜序验收=W4(tieba 59566)恰一席换径 0.141→0.066(三分量手算+对抗官独立 raw 解析器双闭合:0.141=running 0.049+sleep 0.066+runnable 0.026,被剔 runnable 恰在同板自席),15 席恒等;B4/R1 absorb 键含精确行区间+两面同读单一值源=锁步换径无解耦;§12.3 准入/1.35 权重=typed 对值无关;Score 无缓存。件4 witness 电池=cust_err1 复刻(200ms ART 锁→Σ100+⚠110/90,假和句「210>200」诚实消失)/tieba 84 条形B 三分类=**72 归 0(纯自旋)+9 真换径+3 零宽 fail-open**(批报告 75/6 数字勘正,对抗官引擎复跑+独立解析器双证 72/9)/donghu 形A 值 0.295→0.185 而 LOCKNS 推导链五面逐字节保持/payload-less 全族零回归(W1 dump 字节恒等)。R2' 零新键(值域扩展先例,tracediag 第 7 处正式条目)。
**双复核**:对抗 PASS 零 P0/P1——独立 Python 解析器直吃 raw trace 复算全部换径值逐值吻合;「收敛值>包络」结构不可达论证+104 真行扫描;7/7 突变红;6 窗基线对照除声明面字节恒等;LOCKNS/XERR1-FIX/LCK-2 全族零回归。冷读 SHIP-WITH-DISPOSITION——四要件+LOCKNS 零触全实证;产线三窗答案面字节恒等(µs 行预算下不上面=零 0.000 刷屏);**P1=默认参数车道锁席帽截静默消失**(0.066 跌 pool #14/帽 12,且同板可现「锁等待主导」降道注=矛盾头;=既有 D1「席死于帽」病形被批准改值暴露,锁席无保护道)。
**修补轮(三件;P1 处置=委托选定披露道,遵「排除≠消失」教义+§29.104.13)**:A=lockSeatRankFailLoudCaveat(仿 semantic 先例;精确 typed 门=pool 含 resolved 锁席(§12.3 同一 typed 对)∧板零锁席→「largest X ms at pool position N of M; see critical_blocking_calls」;build+enrich 双车道挂钩+哨兵去重一板一句;值/席位/帽零动;W4 默认参数形修后=席位恒等+恰一句披露;四臂 pin 含同板矛盾形蕴含证明);B=两消费侧权威注释勘正(trace_note_keys.go:751/trace_causal_projection.go:974「Absent on payload-typed」假注→值域扩展如实);C=span_envelope 行 rank 面头补包络 caveat(与 blocking 面对齐,legacy 零追加)。侧道保席(vs 披露)留候选=客户复放若要席再议。
**备案**:①payload ⚠ Summary 句族随 basis 换词(fail-open 臂保旧句);②twin 缺席边界=compaction 吞 twin 时 rank 行无值口径行(值仍诚实,与预算 trio 同既有几何);③件A caveat 单 EN 字符串=引擎 caveat 通道一致处置;④「值切」形(锁 span 掉出 top-8 span cap 根本未入 pool)不在披露射程→DISPLAY-HYG 候选;⑤µs tick 同刻 wakeup 账目 +0.018 保守高估=既有非本批;⑥top-N span 选择使 µs 锁 span 常不上产线报告(donghu 形A 窗字节恒等之因)。

## §29.117 RANKDIS-M18 收账(2026-07-17;裁定②=复合分数迁出 _ms 键槽;census 定策零兼容臂;合并复核+便宜修两件)
**裁定②原文落地(§29.104.17「批准第二级 wire 键改名,如果无需兼容旧键更优则无需考虑兼容,按最优的做」)**:步1 全读者 census 定策——display_only 键(rank_impact_ms/rank_impact note)零 Go 读者→**干净改名零兼容臂**(最优形);rank 行 JSON payload 零仓内 Unmarshal→per-row MarshalJSON fork(复合行 impact/cumulative/effective/projected 四族→*_score,target/actual 物理墙钟账保 ms,非复合行 alias 直通字节恒等);note 同名键有硬消费(projection/board/prose×2/evaluator)→**读者侧 union 同步**(一行恰发一族键,读者读并集,与 effective_impact FirstFloat 先例同形);跨系统(eval/tracediag/examples)零命中。步2 落地——单一值源 causalTokenCompositeValueWireRows(io_pressure 刻意不入 caliber-side class 排序臂=越权防,⌗ 降道留裁定议程);词面=「(composite score, not wall clock)」+Unit=composite_score(typed 自描述走终判⑧车道);新注册键 5+1 R2' 七处;explorer/evaluator 教学两句(prompt checklist 过);**tool Description 字节零动=deliberate**(§29.64 h3 教训:mid-Description note-key 教学句实锤翻红,教学移数据侧;C3 值词库教学批是 Description 正主)。步3 pin——注册表子集恒等+排序零动结构 pin(io_pressure class==None,突变 fatal 文案点名需独立裁定)+复合行四键 fork 正负+非复合行 marshal 字节恒等 alias 对照+词面正负。
**合并复核 PASS(零 P0/P1)**:census 独立复核零漏读者(tracediag reflect 渲染无泄漏=Summary-first 结构证明+真报告实测;退化 re-parse 车道=caliber 词破坏 ParseFloat→宁无值不假 ms,终判⑧ 同形文档化);四真窗双树 diff=非复合行三面零字节漂移;**w4 实锤=基线 146ms 窗发布 impact=635.077ms 假墙钟(正是裁定所杀客户病类),修后「635.077(composite score, not wall clock)」**;8 突变账(7 红+1 预判绿=死臂证据);union 双胞键并存确定性取 ms;:749 墙钟 union 刻意不并=结构不可达论证独立核实。
**便宜修两件**:①span= 回声词面(traceQueryRootCauseSpanCompact 无条件 %.3fms=QH2-A 漏网,复合行同一行自相矛盾)→走主槽同一 wire 臂单源(复合行回声穿 caliber 词,墙钟行字节恒等,三臂 pin);②三席位门 union 臂注释勘正(当前闭矩阵复合行恒 rank=0,臂=未来给席裁定防御性预留,板模板 ms 衣(trace_board_summary.go:90)预警点名)。
**备案**:①io_pressure ⌗ caliber-side 降道(tier/序数/fold 车道)无裁定=留裁定议程(注册表注释+子集 pin 留口);②P3-3 降道 re-parse 值损=诚实方向文档化;③手铸 ms-note fixture 不改=历史工件 union 验收面;④观察账本跨 turn 混合键=按记录隔离解析无翻覆。

### §29.104.19 用户常任委托升级(2026-07-17)
**用户原文**:「后续需要人工审核确认的暂时按照你的评估最优方案进行默认处理,我会在合适的时候审核,不要阻塞后续批的启动。」落定:裁定池新条目/复核 disposition/教学面改动等原需人工确认项=主会话按理论最优方案默认处置+账本完整留痕(条目标注「委托默认处置,待人工追认」),批启动零阻塞;用户回溯审核以账本节为准,追认或翻案均落账。既有红线不变(providers.yaml/api_key 纪律、致命三类完成门、值通道纪律、B5/R2'、突变恢复 cp、push 独立命令)。

## §29.118 值词库教学批收账(2026-07-17;sweep C1-C3+裁定③;双复核+修补轮五件;金样 14 趟=13 PASS+1 归因收口)
**修(四件)**:C1 skill 新 TRACE VALUE WORDS 指令(wire↔显示词映射:gated_runnable_ms→「runnable(全额)」/deficit→「running(折算)」/fold caliber 五词含截断臂「计数当量(超上限截断)」(修补轮补)/「有效归因」单点化+禁自铸「有效影响」「直达」;witness 三病=裸 gated_runnable、自铸直达、裸 sum_disjoint 逐一有禁句,冷读核销表在案)。C2 图例四句(①eff↔cum **放弃假全称**——读码定谳三车道双向都存在(折算 eff<cum/发生段账目与承自 eff>cum),改「同值=同一测量的两个名目/异值以行内口径词为准」;②L1 无链行「链上累计=自身投影,累计方向非直达声明」;③wire 挂钩行(projected_impact_ms 刻意不等同窗口投影);④裁定③置信档句「置信档=各车道数值阈值折词,不作跨行强度比较(不作为推翻榜位次序的依据)」双面落地——首版「构成」词被 UXG-0 承诺门实锤截获=承诺面门活防线)。C3 Description 三处(①hidden-cost→三口径现实;②**幻键 projected_total_ms 第三定谳**=它是 wakeup_chain 三结构真键+rank note 面 cumulative 双发回声,处置=教学诚实化 wire 零动(fork (a) 前提不成立/(b) 放大四名一值病),负向 lockstep pin 防幻键成真;③top-ranked state→drill_rank 词族)。golden 重钉两轮句级 diff 恰 3+1 句零外溢,EVOLUTION RECORD 全落。
**双复核(双 PASS,两 P1 合流)**:对抗——教学句逐条 TRUE 核(单源坐标逐一;C2① 攻击塌缩=WakeupCausalImpact 无 cumulative 字段;裁定③阈值 0.85/0.6 TRUE);9 突变账;K1 唯一证伪=count_sum 漏截断臂第五词(h1 金样自渲两次)。冷读——教学素读可执行性全过(行3 语境真机同形/禁词不与引用冲突/回查双向通);R7 漏网=defaults.go:126 旧句仍教幻键对「used for ranking」同文件活矛盾;lockstep 宣称过头(仅 fold+置信真对钉)。
**修补轮(五件)**:①:126 句诚实几何化(回声句与 C3 逐字同词族+排序键=effective 句+旧句负向 pin);②count_sum 截断分叉教学+clamped 真臂探针;③lockstep 单源化(行3 分量 runtimeTraceProjInversionComponents 组装对钉+列名单源,honest-scope 注释);④**双 LLM 面矛盾定谳=两句皆假**(closed-matrix 存在终端残余臂回填 cumulative)→双面统一「closed typed matrix, never generic default; only a residual row outside every arm without published effective falls back」+双面恒等 pin+三 stale 词形双面全禁;⑤fork 标签勘正(第三定谳)。
**金样电池**:首轮 11 趟(h1..h9+h2/h3 敏感对)全 PASS(规格「12 趟」交付口径=9+敏感对复趟,备案);修补轮 h2 PASS+h3 一次 FAIL——**归因=批前既有调度窗病**(explore 窗未提供 trace_query,模型正确尝试被打「不可用工具」,六锚必缺;零交集三证:分类/调度面零触+失败窗 Description 不在场+基线同形先例 h6-20260715-062137)+复 roll PASS 六锚全在。**立案 TOOLWIN(委托默认已立独立任务)**:attached-trace 运行的调查窗必须携带 trace 工具;witness=real_trace_h3-20260717-051341+real_trace_h6-20260715-062137 两 transcript。
**备案**:①C1 EN gated 词无 verbatim EN 形+M18 composite 键族 carve-out 缺=A7 议程随行;②「同值」以精确 float 判而读者按 %.3f 读=构造角;③reflect 负 pin 对 marshal 期铸键不可见(文案已诚实);④defaults.go:126 姊妹句首轮留置被冷读定为 P1=R7 论证被推翻的实录(教训:同文件姊妹句 R7 必逐句过,不许「已由 Description 承担」豁免);⑤内部注释 hidden-cost 残词=非 LLM 面卫生。

## §29.119 反转词位批收账(2026-07-17;sweep A5+C4+C5;INV-SUPPLY §29.61.11 推广收尾;合并复核 SHIP 零 P0-P2)
**修(三件)**:A5 反转词位单源——ImpactFormTokenFamily 双 token 归族(UXG-1 M4 单点判族零手拼)+唯一词源 composer(TypeToken→Object→Predicate registry 序,token 先于 flag)+六面接线(C7 影响形态 cell/「runnable调度候选」第三词删除(zh+EN 同灭)/表 cell node-aware(flag 行与值行同词)/行2 含 relocated state-tag 分叉(stateless 撤行尾词杜双词,stateful 保「· runnable」状态披露=裁定4)/◎ 类词臂从 composite 推广到全反转族席(witness 病=强席显弱词「runnable」)/crown 伴修(否则 crown/行2 新分叉——复核实渲证 load-bearing));witness 三面修后同词「优先级反转·可运行等待」,plain runnable 行「调度压力候选」字节恒定负臂。C4 absorbed 行自描述——**选型定谳(委托默认处置 §29.104.19,待人工追认)=不加 seat_status 新字段**(Tier 即 typed seat-status 枚举,第二字段=同一 typed 事实第二词家违单一值源;邻位从偶然变承诺=文本面同行序+JSON 面 tier 紧邻 type 行双 pin,字段夹塞即红;M7 witness 病 grep 修后 -B1 即见)+summary 尾词「priority-inversion (runnable-overlap) candidate」+skill 一句两 token 一族两通道教学(candidate=链上 gated 复合席/runnable_wait=同CPU runnable-overlap 发生段行/tier=absorbed 不占席;ATOMIC 7 条过)。C5 零代码留痕——裁定③图例句已落 §29.118,车道常量收敛按委托默认零动留裁定池。
**合并复核 SHIP**:三面同词端到端双树对照(值/E#/榜序/席位多重集全等,漂移恰声明词面);单源全仓 census(族词非测试铸点恰 1=typelabels.go:42,第三词活树零残留);C4 论证独立核成立(MarshalIndent 序列化+struct 序=邻位结构性成立);6/6 突变红;51/51 波及 pin 绿;B5 零重滚原因实证(◎ token 席 baseline 已同词+受改面无字节级 oracle);gatedcal 重钉 2 处=原 pin 钉的病形即本批所修,反向守卫升格。
**备案**:①C7 FamilyWord 反转臂产线不可达=防御纵深与 lock 臂同构;②A5 scan 射程 6 目录(feed 层逃扫风险,candidate 词经 tracefence 无虞);③perf-triage-skill:955 同题句自洽未升级=教学卫生候选;④E31 行1 名列宽截断=批前既有 DISPLAY-HYG 候选;⑤陈旧 worktree nice-leavitt 含旧词拷贝=环境注记。

### §29.104.20 两任务收编与队列更新(2026-07-17,用户批准收编)
**CSP63-FIX(排第一)**:blob 读污染 CurrentSourceSatisfied 根修——runtimeSourceAuthorityCurrentSourceRecord(runtime_source_answer_authority_view.go:482)守卫只认 RuntimeArtifactPathKind,引擎自产 trace-query-result-*.json blob 穿透铸 current-source 权威→trace-only run 的 runtime bypass 全扼→好数据整体 DOWNGRADE(h9 witness;SUPPREF-TOL 前瞻 pin TestSupprefTol_WitnessReplay_FaithfulStateStillDowngradedPendingCSP63 翻绿=验收信号,翻绿后改接 AcceptedExcludeLane 断言)。修向=blob-session 路径与 trace_query 发布 blob ref 不得铸 current-source(typed blob-ref registry(traceQueryBlobRefPathSegment)非文件名后缀匹配)+「blob 拼写 case-fold 残口」子项+donghu_real 复放(../customlogs);authority 爆炸半径 census(CanHardBlockCompletion/CurrentSourceSatisfied 全消费者,tool+orchestrator 两包)必须双复核。=CSP#63 记忆 🔴 头号候选终局战。
**TOOLWIN-FIX(排第二)**:attached-trace run 调查窗缺 trace_query——两 witness 跨分类标签(h3-20260717-051341 intent=explain/h6-20260715-062137 intent=root_cause),AttachedTrace 在场而 explore 窗只有 emit_evidence/emit_investigation_complete/repo_map,模型正确尝试被拒后诚实缺席答案=全锚必缺;生产同机器可达+金样电池持续暴露。修=确定性调度侧(internal/analysis 子包+loopkernel 窗工具集装配):trace 附件 run 的调查窗必须携带 trace 工具,或不能携带的窗不得为终窗;非 prompt 教学;回归 pin。
**队列(§29.120 收账后)**:CSP63-FIX→TOOLWIN-FIX→XLANE-2(裁定④+E11 rider)→DISPLAY-HYG(catalog 22 形+各批 P3 群)→HULL-CRED;尾段=裁定池人工追认清单汇总+客户回访件汇总(cust_total_del 修后复放/v2_slim/endless_loop/260M)。

## §29.120 SUPPREF-TOL 收账(2026-07-17;装饰形 support_refs 宽容解析;**战役窗首个 REJECT+返工闭环**;复审 PASS)
**立案落地(§29.104.13)与机制定谳超出假设**:witness(XGAP emit#4)完整拦链=三重:①装饰散文 ref(「attached_trace.txt: wakeup_chain path」emit#2 铸)不可解析;②其存在本身一票否决 bare-surface form repair(「有 ref 即拒」前置);③**blob 污染扼杀 runtime bypass**(explorer 分页读引擎自产 trace-query-result-*.json→穿透 :482 守卫铸 current-source→CurrentSourceSatisfied=true→bypass 全灭)=CSP#63 战场(chip task_7798395b,CSP63-FIX 已收编队列第一)。census 591 组装饰谱十家族(闭集=对称包裹/尾括号注/artifact 冒号散文注;中文行号/@ 形/序号前缀宁窄勿宽不剥)。
**修(终局形)**:剥装饰闭集(**逐层剥+每层解析即停**=复审过剥形收口)+重试包装(先既有精确解析失败才剥后重试同一解析器=解析器本体零改;五环接线;member 面/call-chain 阈值消费者刻意不接)+positional 车道**双轨**(精确解析保基线双向语义逐字节=换位 ref 修复车道记账前提(全 fall-through 二阶回归被全量套件实抓);剥后 strip-retry 臂 success-only 失败一律 fall-through 到 ref loop)+form-repair「有可消费 ref 才拒」(junk ref lossless 保留)+origins 预滤只在 bypass 实际保护成员时拦+schema 一句裸引用形软引导。
**REJECT 实录(复核制度活性证明)**:首轮合并复核 REJECT 两 P0——P0-A=新拦形(tolerant positional 双向 early-return 掐断 ref-loop 兜底,基线 A/B 实锤=权属红线破);P0-B=witness pin 建立在分析器明确拒铸的伪造状态(产线行 496 WARN ignore exclusion_kind,fixture 却铸 ExplicitUserBoundary;忠实状态复放 emit#4 仍 DOWNGRADE;M1 证 strip 机器对 h9 拯救零贡献)。**返工**:①双轨修形+A/B 探针转正 pin(双不变量防漂移);②诚实 witness 重定界=忠实产线终态 pin(今仍 DOWNGRADE+逐件分解+CSP#63 翻绿前瞻指引)+宣称降级三分解((a)剥装饰 resolve 改进=census 族3/4 h2/h6 引擎实发形端到端真 resolve;(b)form-repair 放宽合成 pin;(c)h9 完整拯救=CSP63-FIX 移交)+gate-1 臂保留有据(**309/1101 run 携被接受 explicit_user_exclusion 终态**=真产线车道,c4 政策 verbatim×h9 payload 合取 pin 穿臂落地 keva-1 3.429);③逐层剥 parse-stop+shadowed 一致性 pin+chip 重铸全路径+schema pin。复审 PASS(原探针三层回基线平价/换位记账前提保住/合取真形性核实/5 突变全抓)。
**教训(入红线记忆级)**:①witness fixture 必取产线真实状态链——伪造状态(分析器明确拒铸的形)上的 pin=把不可达行为钉成验收,是「fixture 取引擎实铸形」教训的状态面升级版;②「只 return true 哲学」不可盲移植——双向 early-return 可能是另一车道的记账前提,全量套件是二阶回归的真防线;③REJECT-返工-复审闭环首跑成功,复核官保留追问权。
**备案**:①FromSupportRef verbatim lane 子串宽松=origin 文法专项候选;②family4 注释 run-2 标注 P3;③strip-retry 死代码妆面;④.txt 核不入 location 文法维持设计。

### §29.104.21 用户核验 cpu=unknown(2026-07-17)+DISPLAY-HYG 候选一件
用户指认 20260717-052241 报告 E10 成员 cpu=unknown 0.259ms 是否准确。**主会话 raw trace 逐段核实=准确且设计内诚实**:wakeup@13763.024639 后全 trace 零收尾切入(trace 末行 ts=13763.024898 恰为窗终点,0.259ms 精确吻合)=采集在线程 runnable 排队中截止;§29.107 XCPU 规则下唯一诚实值(取 wakeup target_cpu=已修掉的迁移陷阱)。同席 cpu=13 0.337ms 成员=wake .023839→switch-in .024176@[013] 逐段核实。**DISPLAY-HYG 新候选**:成员行 cpu=unknown 佩 why 短词「(采集端截断,无收尾切入)」——typed reason(window_end_unverified 族)已在引擎,仅缺成员行词面消费。

### §29.104.22 立案 SCORE-DERIV:综合评分/计数当量推导透明性(2026-07-17,用户指认)
**用户问题**:报告中综合评分/计数当量的值如何计算,有无描述。**定谳=无读者可见推导**;真实公式(铸造点读出,用户报告值逐一精确对账):①block_io 综合评分(query.go:12041)=最大单事件块延迟+最大单事件存储延迟+文件IO字节/MiB+页缓存事件×0.2(E33 2.694);②io_pressure 综合评分(:11868)=max(块/存储最大延迟)+iowait阻塞次数×5+D态ms+iowait ms+页缓存事件×0.2+文件IO事件×0.1+字节/MiB×2;③计数当量(:14267)=churn 事件数×0.3(84.300=281×0.3/34.800=116×0.3/119.100=397×0.3);④超上限帽(:18014)=窗长×0.35(81.616=233.190×0.35 精确)。边界确认:全部权重只活 advisory/背景软引导车道,不参与汇排/序数/硬门(精确信号红线合规)。**修向(委托默认)**:①文档级必做=权重表+公式+帽落 docs 设计文档(架构附录或独立 score_derivation 文档),账本引用;②报告级择宜=明细/图例构成要素列举(倾向不印具体权重=「评分构成:块/存储延迟+IO 字节+页缓存事件,详见文档」形,避免启发式常量上客户面引质疑);归 DISPLAY-HYG 随行或独立小批。

### §29.104.22.1 SCORE-DERIV 用户裁定(2026-07-17)
**用户原文**:「报告里只需要在"阅读参考"里面提及即可,让客户大概知道公式即可,无需列举详细计算具体值,报告里可以体现公式,但是可以隐去权重常量的具体数值。」落定:①报告级=「阅读参考」(图例/各列口径区)新增公式形词条——综合评分(io_pressure)=最大单事件块/存储延迟+iowait 阻塞次数(加权)+D态/iowait 墙钟+页缓存事件(加权)+文件IO事件与字节(加权);综合评分(block_io)=最大块延迟+最大存储延迟+文件IO字节(加权)+页缓存事件(加权);计数当量=事件数×固定当量系数;超上限截断=按窗长固定比例设上限(原始和随行供对照);全部标注「非墙钟,不参与汇排」;**权重常量数值隐去**(只在代码+设计文档);行内不做逐值计算展示。②文档级照 §29.104.22=权重表+公式全量落设计文档。实施归 DISPLAY-HYG 批(图例词条+承诺面双向 pin+文档)。

## §29.121 CSP63-FIX 收账(2026-07-17;blob 读污染 CurrentSourceSatisfied 根修=CSP#63 多 session 🔴 头号悬案关账;双复核+修补轮四件)
**病与修**:引擎自产 blob(.codrax/blob/<session>/trace-query-result-*.json)被 explorer 分页 read_file 后穿透 :482 守卫铸 current-source 权威→trace-only run 的 runtime bypass 全扼→好数据整体 DOWNGRADE(h9 witness;三重拦链之三 §29.120)。修=单点守卫扩展(runtimeSourceAuthorityCurrentSourceRecord 第二 carve-out:IsCodraxBlobSessionPath=canonpath 折形→ToLower(case-fold 残口同治)→复用 typed traceQueryBlobRefPathSegment 结构判定,非文件名后缀;Q5-A 安全注册门保持大小写严格零触=deny 宽 allow 严双向偏安全);选点论证=CurrentSourceSatisfied 两条计数道单点分类,mint 时改 Origin 会波及全 ledger 消费者。
**authority 半径 census(双复核独立复算)**:四字段 17 文件直读点+派生字段族+helper 族——正确性证明形=**恒等式**(blob 记录对 snapshot 全字段零贡献,DeepEqual 双 pin)一次覆盖全部 snapshot 消费者;raw 侧信道三处(citation_normalize :103/:157=修前修后结局一致纯显示;answer_surface_plan.go:2104 镜像=修补轮已接线;orchestrator.go:7540=修补轮评估**不接**——共享谓词 7 消费者,carve 会把 mixed-contract 在场判 present→missing=收紧方向违权属,防分叉注释落位,**残口备案:逐流证明结局一致非行为缺陷,未来接线须配 pin**)。**翻转区 A/B 裁定(对抗官基线全谱实锤)**:precise 需求+仅 blob 读形修前 satisfied=true 假解锁(零仓库证据 combined_proof_ready)→修后硬拦=与同 run 无 blob 读基线逐点 DeepEqual 一致=**恢复既裁零见证硬门,非新拦**(§29.21+权属双红线论证)。
**witness**:h9 前瞻 pin 翻绿实录(旧 DOWNGRADE 臂在 origin-bypass-NOT-protecting 断言处翻转失败=验收信号)→改接 EmitFourLandsAfterCSP63(keva-1 3.429+折算基准入 handoff,**装饰面无损经 origin bypass 主车道**=非 form-repair 改写;AcceptedExcludeLane 断言同步升级=修复强制的接受面形变,repair 兜底车道仍由合成 pin 独立钉);混合 run 负臂=仓库源读照铸+快照逐字节;trace-only 无 blob 恒等;donghu 0703 野生污染态(satisfied=true:source=10+答案 10 处 blob 伪引用出厂)vs 修后(0717-092614 复放,修补轮全量二进制):PASS+全程 satisfied=false:source=0+零 blob/源码引用。
**修补轮(四件)**:A①姊妹镜像接线(source-optional surface plan 回落车道,blob 读不再压住 surface 处置=接受向)+A②:7540 不接线备案(上);B=blob offload 引用出书目(052241 md:966 活 witness:trace_query-*.txt ScopeFile 双旧车道皆漏;修=citationFileIsRuntimeArtifact 唯一共享识别权威接谓词,六消费面自动覆盖,移除+runtime_artifact 真话披露渡轮保留,CPD#58 typed-exclude 臂零触,mixed 裁定 pool 不动 rider pin);C=LAYERING NOTE 改指根修落点+保留臂真话理由;D=donghu 存证复放(上)。
**红线复核**:§29.21=分类修正非见证标准放宽(blob 读=tool 见证但对象为引擎自产物,其 runtime 功劳已由 typed 观测入账);完成门权属=零新拦(6+6 突变全中含 M6 证恒等臂独占载荷);R2' 不适用(内部谓词零 wire 面)。
**销账**:CSP#63 记忆 🔴 头号候选关账;SUPPREF-TOL (c) 分解闭环;CPD#58 显示臂保留理由更新。**备案**:symlink 字符串车道固有极限(与注册门同姿态)/case-fold 宽度注记(Linux 字面大写目录拒铸=deny 向噪声趋零)/donghu 04-05 时段 run 二进制存证缺口由 0926 复放补全。

### §29.104.18.2 DISPLAY-HYG 提序+◎ 脚注逐席分行(2026-07-17,用户第三次直接指认显示面)
**witness**=20260717-092738.844-28646 报告 ◎「不参与汇排(口径)」脚注一行塞两席+每席重复样板尾(·⌗口径旁栏·XX(非墙钟,不占序数))=§29.114 冷读 P2「◎ 区无折行治理」活体第二例。**修形(用户请分行)**:①样板词上提头行「不参与汇排(口径旁栏,非墙钟,不占序数):」;②≥2 席逐席一行(主体+口径短词+值+[E#]),单席保单行;③◎ 区接入 §29.114 折行器(token 感知)补宽度治理;同族=值切计数脚注/降道披露脚注/▒ 计数脚注若超宽同规则。**队列调整(委托默认,用户三次直接指认显示面)**:DISPLAY-HYG 提到 XLANE-2 之前——TOOLWIN-FIX(在飞)→**DISPLAY-HYG**(本件+SCORE-DERIV 两级+cpu=unknown why 词+catalog 22 形+各批 P3 群+CSP63 残口群+gofmt 三文件)→XLANE-2(④+E11)→HULL-CRED→尾段。

## §29.122 TOOLWIN-FIX 收账(2026-07-17;attached-trace 调查窗必携 trace 工具;合并复核 APPROVE+带修二进制复放最强形实证)
**病灶链(八环,复核比批多挖环0)**:环0=source_inventory_profile 在 trace-only run 由 synthesizeSourceInventoryProfileForTypedEnumeration 确定性合成(模型三次 emit 均未发射;h6 则为模型自发=双来源同窗形)→环1 lens probe 节点无条件入全部 5 scenario 模板图→环2 SourceInventoryProfileActive 窗优先(零标签消费)→环3 窗面硬滤三工具→环4 模型 iter=0 正确调 trace_query 被拒(reason=unavailable_tool_surface)→环5 lens 完成门死锁 4 轮断路器 force-complete→环6 accepted closure auto-complete 余节点=事实终窗→环7 六锚全缺诚实缺席答案。**机制措辞校正(冷读)**:iter=5 后 schema 实回 15 工具——非全窗结构盲,而是「首轮硬拒+三工具窄表拒收文案毒化锚定」;修形恰双治。
**修(修形A 三点,全 typed 零标签)**:①agent.go 探针面(lens/followup)经单一委托在 traceQueryToolVisible(与默认面同一 typed 谓词=单源不分叉)为真时加 trace_query;②validate 臂同门放行+拒收文案 available 列表如实;③read_loop_next_action.go 策略安装唯一咽喉 admit 臂(HasTraceArtifact∧未显式 Deny→AllowedTools 追加,budget cap 随装配);**显式 DeniedTools=刻意无 trace 窗选择退出**(landing-repair emit-only 修形窗尊重=pin;lane census 封闭恰 2 构造点)。修形B(非终窗保证)弃选=踩完成门权属+DL 战史双禁区且嘈声硬门。
**复核(APPROVE 零 P0/P1)**:病灶链逐环 log 行号对照+sibling PASS 差分=恰窗面;35 组 census 穿真实 compiler 全组同窗形=标签无关双向坐实;单源谓词 grep 恰 4 消费点;6/6 突变红;L1 双 pin+read e2e 全绿;**带修二进制 h3 复放(101009)=最强形 PASS**:analyzer 第三标签组合再合成 profile→lens 窗成形→iter=0 trace_query 成功入 transcript→六锚全在=修复在真实病灶路径端到端实证。
**立案(复核 P2 两件,同批处置候选=LENSBURN)**:①上游合成守卫盲区=HasObservationOnlyRuntimeArtifact 在大 trace 上结构性失明(perf_triage 尺寸跳过→RM 无 bundle→守卫恒 false→trace-only run 白铸 profile→lens 窗+义务烧轮;修向=守卫增读 Run-entry typed preflight 同载体);②lens 义务与空仓「no observation」结果互认失败恒烧 4 轮至断路器(复放 wall 216s 直接来源;窗面修后只烧轮不毁答案)。**P3 备案**:census 枚举值无完备性 tripwire/ReadRunActiveState 快照展示误差(行为无缺口)/机制措辞已按校正落账。

## §29.123 DISPLAY-HYG 第一轮收账(2026-07-17;用户直接指认四件;合并复核 PASS 零 P0/P1)
**修(四件)**:件1 ◎ ⌗ 脚注逐席分行+样板上提(§29.104.18.2 用户 witness 092738:206 格单行→头行「不参与汇排(口径旁栏,非墙钟,不占序数):」+逐席缩进一行(主体+单源值形+短⌗词+[E#])+溢出尾行「…等共N行」;单席字节恒等负臂;同族五脚注(值切/降道/▒/语义/构成拆解)同入宽度治理)。件2 ◎ 区宽度治理(§29.114 P2 关账)=◎ 注记行接同一 token 感知折行器(bar/成员/头行按发射位分类永不进折行=非字节嗅探;≤100 恒等由构造;§29.114 全纪律继承)。件3 SCORE-DERIV 两级(§29.104.22.1 用户裁定)=报告级四词条 on-demand(与值词面同 typed 谓词双向;「加权系数为固定常量(报告不列数值)」句)+文档级 docs/design/score_derivation_20260717.md 权重全量(九坐标复核零漂移)+权重负臂(全报告面 grep 零现,双包运行时字符串扫零泄漏);**委托处置(待人工追认)**=block_io 词条按发布值三项成文(裁定引文第四项(页缓存)源自 §29.104.22 把内部排序位点误作发布位点——照抄四项则客户对账必不合=承诺面 over-claim;复核独立读码维持)。件4 cpu=unknown why 词(§29.104.21)=恰 window_end_unverified reason 佩「(采集端截断,无收尾切入)」(typed 门;其它 unknown 保裸负臂;明细/树两面单源;witness E10 0.259ms 真 trace 复放逐字节;零 wire 面)。
**复核(PASS)**:witness 双复放素读(分行形与用户请求一致+信息无损);权重零泄漏猎杀(引擎 Summary 只发原始分量事实);委托处置独立读码维持;零动 diff 级证明(数值 token/E# 多重集 SAME);7/7 突变杀(含 reason 门判别性=负臂红正臂绿)。
**立案(复核 P2,批外既有,归 SCORE-DERIV 族)**:复合分数家族成员 roster 佩 ms 尾(rank_family_fold.go:1156 else 臂,真 trace 实锤「inode=0x14088d 2.694ms」而席位面已诚实——member_roster wire 供 LLM 消费=口径谎候选;§29.55 两形一裁未覆盖此面)。**P3 备案**:设计文档补「排序分影响截断成员资格」一句/计数当量词条 file_io 双产者完备性(文档已如实)/flag class-only 不对称/单席折行短孤尾。第二轮(catalog 22 形+P3 群+gofmt)待批。

## §29.124 DISPLAY-HYG 第二轮收账(2026-07-17;catalog+P3 群大清扫=清 23+4 件/跳过移交 17 件;合并复核 SHIP+便宜修四件)
**清件(23+便宜修 4)**:catalog=A5 全窗账 chip 双面单源/A7 窗分隔符 `~` 统一 12 面(核对面 `..` 保留)/B4 图例死条目 mark 门控(⊘⚠ 字形在场才渲词条)/B5b 实体名截断保尾部区分段(CompTh…ol_0-2955)/B9 占位符换成员预览名/B10 bar 0%→<1>(真 0 保 0%,0.5 舍入角 pin)/B11 kHz→GHz+`<` 空格/B12 E# 引用统一 [E#]/B13 明细块序导语/C6 双横线折一/C12 span原文 chip(SemanticClass 精确门);P3 群=「略」字撤(以两列实值差为准)/持值行词统一/TOP5 字面-常量恒等 pin/[E8]= 悬行尾折行规则/perf-triage 教学句升级/:103 :157 CSP63 残口注释化/score 文档补句/计数当量双产者词条/flag 不对称注/gofmt 三文件。**便宜修(复核两 P2+一 P3)**:C3 错位行**根修超预案**——复核机制(11 格补宽缺失)被字节几何复算推翻(zh 合计形 stem 恒 ≥11=字面照抄 no-op),真修=fence 级共享值槽(runtimeTraceProjTreeValueSlot 预算 max(11,最宽家族 stem),三臂同槽;无家族 fence 逐字节保形=全量套件即负臂);M-4b 假自检勘正+机制换轨(breakLine 拉锚臂废弃→原子化期裸 = 左融合 ≤20 格 cap,续行裸 = 开头 pin 先红后绿于复核指认 EN width 57 形,两 M-claim 实测红注记);空格两档统一(有效归因 %.3fms 三站点+源级 census pin+13 pin 锁步,rcrc 判别子升级次数恒等);rider 三注释(双单位形 4 面挂注=wire 键裁定 M18 先例暂不改/preview legacy-archive 勘正/B11 括号轴+值前空格余族未清事实收录)。
**复核(SHIP 零 P0/P1)**:清件抽验 12 件基线对照全 PASS(A7 恰 12 面/B10 十面翻转无第 11/B4 时序验证=表发射条件 ⊆ 树 mark 铸造);值零动 diff 级(三 fixture 双树数值 token/E# 序列/板行序 SAME,唯一 % 面差=声明 10 处);跳过清单 17/20 成立(结构级 B5a B6 B7 C7/裁定面 C8 C4/设计内 C9 C11/prompt 教学件 B14 C10/触恒等式 §29.115 群/B8 已被值词库图例实质覆盖);6+1 突变账。
**移交清单(下轮/专批)**:结构级四件(B5a chip 先压/B6 表成员出表/B7 附录 k=v 版式/C7 tag rail 安置)+裁定面两件(C8 标点两制/C4 折叠边词)+prompt 教学件两件(B14 块 id/C10 列 zh-en)+§29.115 多板脚注分板计数(触闭合恒等式需自带突变电池)+B11 括号轴+空格余族+双单位形收敛(wire 裁定)+§29.114 无锚 P3 残件+P3-5 P3-6 观察。

## §29.125 XLANE-2 收账(2026-07-17;语义族子集降道+裁定④披露+E11 rider;旗舰双复核双 SHIP-WITH-FIXES+修补轮七件)
**件1 语义族真子集降道互指(§29.104.2 定谳④)**:witness=runnable2 E34(类校验8次 9.586 榜#1)=E35(4次 6.376)∪E49(4次 3.210) 同物理 span 跨步三 lane 三发零互指。修=行号集 typed 全链:engine RankItem.MemberLineRanges(rank_family_fold mint,all-or-nothing/cap32,re-fold merged 清空 fail-open,重复区间整集不铸)→wire 新键 member_line_ranges(rank+span-family 双发射)→严格解码(count==member_count 整集弃)→判定在显示熔合点(answer_document_mutation_runtime_xlane2.go:真子集行号集包含∧同 canonical subject∧同板单值源 board index+named/unnamed 兼容臂+指纹盲点小门,歧义/缺席一律 skip=宁漏勿假指;A⊂B⊂C 单跳指最大超集)→词面「为[E#]成员子集(整席降道)」zh/EN+专属 mark+独立图例词条(generic 三词封闭集零动);◎ 种群排除+专用脚注(XLANE-1 裁定①先例形)+census==脚注计数恒等。值面 9.586/6.376/3.210 逐字保持(降道=口径变化非值变化)。
**件2 裁定④ self-gap×语义席重叠披露(用户「按推荐的来」)**:X 引擎算(running 库存∩语义成员区间逐席,序=overlap DESC,cap6 导出常量双包镜像 pin)→wire 新键 self_gap_semantic_overlaps→行内「其中 X ms 与语义席[E#]重叠」(多席「、」联;[E#] verbatim 包络匹配,歧义整 clause 跳)。主值零动、硬扣除不做、无重叠零字节(负 pin);手算 15.000=10+5/单 span 5.000;donghu 真 trace 57.248/98.928/58.320 既有 pin 恒等+零自身语义席=零披露诚实缺席。
**件3 E11 rider(§29.112 记录①)**:病根=R1 same-fact absorb 无条件 OR 继承降道双 marker——fused donghu 实锤 E11(❶ CPU亲和 根因排序#1)/E31/E32 三行同佩「无链上凭证(整席降道)」=❶+链上 lane+rank 席+降道句四面同行。修三点:(a) survivor 显式 on_chain 不继承降道(◇/▒/裸"" 保 OR=XLANE-1 P2-① 兼容 pin);(b) 链上面存 internal 记忆 AbsorbedWholeSeatDemotedView+anchorFormKey 第五臂保 fold-fork(裸删 marker→低频运行三重叠账 re-Σ 32.877 假和,复核 M14 独立复现证 load-bearing);(c) Rank 收养窄门只拒「降道 loser×链上 survivor」(初版全域通道门破坏 XLANE-3 fused 合法收养=静默丢席,实测回退;三正控 pin)。终证=baseline↔fixed fused byte-diff 恰三句谎言面消失+图例位次一移,全文值/序数/[E#]/互指 token 多重集恒等。
**双复核(双 SHIP-WITH-FIXES)**:对抗官=19 项突变假 pin 猎杀(隔离副本,活树只读)+两工件自产逐字节 BASE/FIXED_IDENTICAL+值通道 TOKENS_IDENTICAL 独立复算+X 区间手算独立+R2' 逐点;P1=compat 臂(产线 witness 形唯一通路)零 pin 覆盖=整臂杀死全套仍绿。冷读官=逐 hunk 零私货零缺失+裁定④原文逐字对+闭合恒等手算;P2=在席(Rank>0)子集席 ◎ 排除臂休眠。**修补轮七件(突变红/绿全实录)**:compat 正向两形(产线发射形 fixture=具名 rank 席×无名 span-family 席,照 trace_query:7033 独家发射构造+ids 互异 load-bearing 前置)+refuse 两分支(M1 整臂灭→红3)/歧义 clause 负臂(M2 first-match-wins→红1)/图例词条头逐字锁四形(M3 改一字→红1,方案B=词条头 HasPrefix 断言,probe 治不了 catalog 自证面)/在席子集席 ◎ 臂(bar 消失+脚注+§29.112 三闭合)/指纹盲点小门(无名行自身 fp 非空∧∉具名板指纹集→skip,per-seat 精确=无 fp 兄弟照降;M5 红1)/头注勘正(删「不可相加·」矛盾前缀)/mint 重复区间整集不铸(display 集合化塌重 8→7 假指风险;M7 红1)。
**R2'/勾稽**:两新键七处齐;golden 实测 392→394(+2;progress 393→395 为口径差,以实测为准);tracediag RootCauseRankItem schema hash review 注释重钉(CR-1 P9 先例);W-23 豁免 22→23(AbsorbedWholeSeatDemotedView internal carrier);tool Description 字节零动 deliberate(§29.113 先例)。**委托默认(待人工追认)**:「互指」实现=子集席单向指针+◎ 脚注,超集席无回指(XLANE-1「锚定份由链席[E#]代表」句族先例同形,E# 经明细块双向可解析;若裁「互指」须双向句对,补超集席「成员含子集席[E#…]」回指句=增量小改)。**移交/勘正**:确定性优化点表 E34/E35/E49 成员重复列示(件1 射程外第四表面)→DISPLAY-HYG 候选;progress 笔误勘正=donghu 57.248(非157.248)/E11 终态=窄门形(OrdinalChannelClass helper 已回退,全仓 grep 零命中);h 金样 LLM e2e 未复跑=确定性面 byte-identity 覆盖(§29.109 记录④同约定),客户 runnable2 复放=外部回访项。

## §29.126 HULL-CRED 收账(2026-07-17;§29.104 终判③ hull keep-⛓ 逐段凭证化;旗舰双复核=对抗 SHIP-WITH-FIXES+冷读 SHIP,零 P0/P1;便宜修三件)
**病与裁定**:§29.99 件④ D2 终判「hull 有交保守留道」=keep-⛓ 只凭 hull(reconStartTs/reconEndTs 包络)∩锚窗>0 保 ⛓——包络端点是嘈声,真段全在锚窗外时假凭证保链(修前病形基线实锤:worker 双段 [1.002,1.012]+[1.032,1.045] vs 锚窗 [1.020,1.030],hull∩=10ms 保 ⛓ 而逐段∩全空)。用户裁定③=逐段凭证化。
**修(四臂 typed verdict,值通道零修改)**:判定点=criticalBlockingDioRowDemotes(query.go);段收集在 addDurationCause close site(dioIntervals 真事件区间,hull 端点永不入清单;cap=CriticalBlockingCredentialSegmentCap=32 超弃全+dioIntervalsOverflow 闩死=源头 all-or-nothing);criticalBlockingDioRowCredentialVerdict 四臂:①∅ hull 交→legacy demote 字节零动(sound 臂,段清单在场也不换形,优先级 pin);②有交+清单有效(Σ 恒等 µs 容差)→逐段∩RSPA 锚窗,≥1 段严格正重叠(端点相触=0 不保)=keep+清单上 wire;③全不交→新降道形(既有 ◇ demoted bool+refinement 标记,claim 与 proof 同行=清单同发);④清单缺席(退化档/超 cap/pid census 保守)→keep+「(包络级凭证,见图例)」诚实注。wire 三 key(chain_credential_segments/_segment_disjoint/_envelope_level)+严格解码 9 病形拒收+镜像 cap;显示「无链上凭证(逐段核验,整席降道,见图例)」claim-gated-on-proof(disjoint∧len>0 否则旧 R4 字节);图例两词条 zh/EN(「包络端点是嘈声,不作凭证」)。旧 bool wrapper 保 RNB-5B pin 契约。
**双复核**:对抗官 9 突变(M1 hull-only 回退→红3=修前假凭证形实锤/M2 Σ 剥→红/M5 ∅ 优先剥→红2/M7 wrapper 分叉→红/M8ab cap 双向→红/M3 proof 门剥→红/M4ab 解码剥→红)+基线↔修后同 fixture 全链复放(23.000/7.000/66.000 值字节恒等,仅通道位移)+真相交零容差核(无假凭证复活道)+全仓消费者 census(新三元组仅词面/图例/解码消费,零 gate/score/sort);P2=源头闩零 pin(M6/M6b 存活,判定臂双重校验掩蔽=非正确性,残余=内存成本面)。冷读官 SHIP:逐 hunk 零私货零缺失+裁定③四臂逐字映射+偏离五条全裁(①pid census 佩注=中性偏正确②旧工件缺席零新词=更正确(absence never judges)③真交 keep 零新词=中性(清单本体即凭证)④复用 ◇ 机器=更正确⑤RSPA 原生锚窗=更正确且必要——∅-hull soundness 只对同一窗集成立,CMP-A selected_window 先例属跨 trace 对比投影车道不适用)+worker/helper/env 三形几何手算+golden #8→#10 读认。
**便宜修三件(突变红/绿实录)**:件1 源头闩直接单元 pin(cap+2=34 段→dioIntervals==nil∧overflow 闩死+对照组 2 段完整清单;M6 剥闩→红=post-overflow 部分重建抓获/M6b 剥 cap→红);件2 包络词×降道词互斥门(包络臂加 !LaneDemoted,与 proof 门对称;突变→红=矛盾词对同行实录);件3 发布切片单值源(verdict 连带返回已验证切片,call site 只消费返回值;∅ 臂零动,行为零变化)。
**备案**:keepEnvelope 臂吸收「在场但无效」清单而图例只述缺席两成因——产线不可达(校验器与 close site 同算式同序浮点逐位等),wire 零发射读者视角与缺席一致,词不超宣称,记词面漂移风险不动作。金样 h1..h9 LLM e2e 未复跑=确定性面既有投影 fixture 字节恒等+donghu/tieba 真 trace pin 全绿覆盖(§29.109 约定同形);客户复放=外部回访项。

## §29.127 尾段三件收账(2026-07-17;盘点/评估型,三并行只读)
**①委托默认追认清单**(docs/design/ratify_checklist_20260717.md):正式待追认 12 件(R-1 CASE3-D4 合并行 eff/R-2 XERR1-EXT 披露道/R-3 io_pressure ⌗ 留议程/R-4 TOOLWIN 收编/R-5 C4 Tier 选型/R-6 C5 常量零动/R-7 SUPPREF-TOL 返工三件/R-8 DISPLAY-HYG 提序/R-9 block_io 三项词条(字面偏离,改回有害,强烈维持)/R-10 双单位形 wire 键待裁(M18 模板已备)/R-11 XLANE-2 互指单向形/R-12 HULL-CRED 五处实现偏离)+附录 A 边缘备案 5(含 C8/C4 两件需裁定)+附录 B 早窗指针 7 组。**②客户回访 roster**(docs/design/customer_revisit_roster_20260717.md):13 项四组(A 修后复放 7=runnable2 最高优/cust_total_del/cust_span_runnable/cust_err1 已闭环仅余工件/cust_span_vs_prio/endless_loop/三场景总括;B 工件补交 3=tdkit 采集包已备;C 容量/低优 2;D 观察束+LOCKNS 微普查);通用前提=复放须 main≥4b90fd27f 新构建(客户现用 0.1.20260710)。**③LENSBURN 评估**(docs/design/lensburn_eval_20260717.md):病A=HasObservationOnlyRuntimeArtifact 只认 perf_triage bundle 而 >512KiB 大 trace 被尺寸门跳过 bundle 恒 nil→合成/撤销双守卫失明,真实客户 trace 全超线=失明常态;病B=lens 执行承认四臂全以 IsActive() 为前置,空仓 lens 零值 struct 永不被承认→完成门恒烧 3 轮拒绝+断路器 force-complete+调度侧同盲每轮重成形 lens 窗(复放 wall 216s 直接来源)。修向=A1 双守卫增读 Run-entry typed preflight 同载体(与 TOOLWIN admit 单值源)+A2 census 便宜臂;B1「lens executed-empty」一等 typed 事实(成功空 lens=执行凭证,精确工具见证放行硬拦,恰合 §29.104.13 非致命不硬拦)+B2 阈值仅保底。**立案 LENSBURN-FIX 中批**(委托默认排批,待人工追认;双复核;L1 双 pin+DeniedTools 刻意退出窗 pin 零动)。

## §29.128 LENSBURN-FIX 收账(2026-07-17;§29.122 立案/§29.127 评估落地;完成门敏感区旗舰双复核=对抗 SHIP-WITH-FIXES+冷读 SHIP;修补轮五件)
**病A 守卫失明(复明)**:HasObservationOnlyRuntimeArtifact 只认 perf_triage bundle,>512KiB 大 trace 被尺寸门跳过 bundle 恒 nil→合成(:1623)/撤销(:1619)双守卫失明,trace-only run 白铸 source_inventory_profile 成 lens 窗。修=A1 新 ctx-aware 谓词(bundle 臂逐字保留+preflight 臂**仅当对应 bundle 缺席**启用=PerfTrace==nil∧HasTraceArtifact(),消费 TOOLWIN 同载体 BusContext.RuntimeArtifactPreflight 单值源;ExcludesCurrentSource 合取+anchor 负臂逐字保留;无条件 OR 会夺小 trace bundle 权威——双向 pin)+A2 合成守卫独立 census 臂(ZeroCurrentSourceRepo 抑制**系统白铸**,守卫首臂先退=永不拦模型 emission,Completed=false 惰性负 pin)。
**病B 空仓 lens 烧轮(承认)**:lens 执行承认四臂以 IsActive() 为前置→空仓 lens 零值 struct 永不被承认→完成门每轮重发同一 downgrade 烧 3 轮+断路器 force-complete+调度 env 同盲每轮重成形 lens 窗(216s wall 直接来源)。修=「lens executed-empty」一等 typed 事实,**credential 存活链探根深于评估草图**:坍缩最深根在 normalizeSourceInventoryObservation 无条件归零+Merge 两早退臂+FromMutable advisory 替换臂三处丢失——六处修(types LensExecutedEmpty 字段+shape 访问器(与 IsActive 互斥)/lens_execution 承认成功空 lens+provenance 纪律(analyze 期/失败 lens 不冒充)/normalize 保留载体/Merge OR+早退 credential 保持/FromMutable 替换臂保持/reconcile:412 空仓铸点+持久化)+承认臂窄门共享谓词+调度 env 自动痊愈。B2 断路器阈值零动备案(根修后空仓 1 轮被承认,收缩无必要)。
**双复核**:对抗官 SHIP-WITH-FIXES——11 组突变+方向红线全 diff 核(零新增拒绝/降级/硬拦臂;A2 确证只抑制系统白铸;F5-T4 白名单/DeniedTools 刻意退出窗/散文 Summary 不作凭证三臂零动)+第七处丢失点全仓猎杀**P1 实锤:list_files 覆写路径**(Publish...FromToolObservation 非 IsActive 整体替换→空仓 lens 后一次成功 list_files 销毁 durable credential,闭包侧拒收非 IsActive=无第二 durable 家→跨轮烧轮复燃,探针实测红)+烧轮 pin 真链核(真产线 Publish→gap→downgrade 同函数)+空仓承认只放 lens 臂(不连带放其他义务,逐门论证)。冷读官 SHIP——方向表全放行/降软(唯一名义收紧点=drop-guard 扩面,判=spec 指定的第二盲守卫且驱动信号=模型自身 typed policy=执行模型意志非系统意志)+病A/病B 全链手推逐 file:line 闭环+A1 preflight 臂涵盖文件引用工件形=同病族良性放宽备案。
**修补轮五件(突变红/绿实录)**:件1 P1 修根=该 call site 无条件走 Merge(早退臂自带 ensure credential;零-current 臂逐字节等价)+收编探针为正式 pin(M-A 回退 replace→红=credential 灭实录);**同模式统一五点**(lens_marker/lens_observation/classOnly/repomap+超范围同族第五点 exact 直子臂,各点行为不变论证);件2 FromMutable 替换臂 pin(**可达性发现**:SetTurnAArtifacts 对 FromAdvisory-active 自动物化走 merge 臂,替换臂唯一可达形=advisory active 而投影 inert,该形下历史裸赋值替换成零观察——pin 按真可达形改写不留假 pin;M-B 回退→红);件3 page.go 死 OR 臂保留+注释如实(defensive, unreachable in present mint topology)+convergence 措辞改准;件4 承认臂合取防御深度注释(获非 FromMutable 输入源即承重,禁据 shape-only 全绿删除);件5 keep-alive import 占位删。
**R2'/L1/备案**:LensExecutedEmpty=internal carrier 豁免类(grep 全仓零 LLM 面,W-23 同族);L1 双 pin 复跑 PASS(A1 谓词突变不敏感=正常已记录);LOC ratchet 全 DELIBERATE;备案=graft provenance 在 durable-fallback 渲染形上窄可见(内容诚实,「模型可见面零变化」的已知窄例外)/preflight 臂涵盖引用工件=良性放宽/216s wall 塌缩=机制面闭环,实测复放=客户回访 roster A 组随行。

## §29.129 CLUSTER 簇判定专项+CLUSTER-FIX-1 收账(2026-07-18;三路审计+用户裁定+流式侧扫修根;旗舰双复核双 SHIP-WITH-FIXES+修补轮七件)
**立案(用户:「CPU 簇结构经常判断失败」,witness=/Users/han/opt/donghu/donghu.ftrace 东湖 14 核容器形)**:三路排查素材入库=docs/design/cluster_audit_trace_20260718.md(信号解剖+引擎实放)/cluster_audit_code_20260718.md(短板 S1-S11)/cluster_audit_refs_20260718.md(hmtrace/hiview 研究+补充信号 C1-C6)。**定谳**:donghu 全文件形 HEAD 判定干净(3 簇零降级);「经常失败」三成因=①裁剪采样基(窗扫失败率 10ms 98%/20ms 73%/50ms 12%;更劣静默形=行窗 2 域中簇错封 big、fmax 低估 2.15× 零降级词;HEAD 活入口=行窗/anchor-seek >128Ki 永不盖章/曲线溢出/MaxEvents>250k 硬拒;客户叠加=老 binary 早于 R6)②容器侧单簇 lane 采集形(xxx_all 仅 cpu3-5,结构性 freq_only)③Tier-2 rail 此机型族恒空转(零 CPU 簇时钟名,真簇频镜像躲 thermal_inte1/2 被排除词挡);边缘=15µs skew 界余量被实测推翻(相邻真实变频 14µs,值相等判据独撑,注释量测基待修订=裁定池)。参考器一句话:hmtrace=纯转换器零簇逻辑;hiview=sysfs 硬编码采集器(12 核反例实锤)。
**用户裁定(verbatim)**:「大 trace 全文件基丢失修根,补流式频点侧扫,可以二次全文件重扫,只要控制成本即可。扫描到的内容要能复用尽量复用,不要反复重复扫描即可。」+细化:「扫描到的内容要能复用尽量复用(单次问答中,跨问题可以重新计数)…单次问答中,跨问题需要重新计数。」——§29.88.9 条款4「禁二次全文件重扫」SUPERSEDED(原文已回注);复用边界=缓存只存原始扫描内容,派生簇计数每问重算。
**修(三件)**:件1 流式侧扫 freq_side_scan.go——单遍 O(1) 预筛稀疏 parse 喂同一 fullFreqCurveCollector(采集逐样本恒等 pin)、身份纪律=SameVersion 代际预检+EOF 后 validateTraceFileIdentityAfterRead、样本帽 1Mi 超帽即停读+判决入缓存+披露、进程内 sideScanCache(key=path+size+mtime+强身份(dev/inode/ctime)+parser 版本,LRU 4Mi 预算+条目上限 32,singleflight,hits/scans 计数器,不落盘=多实例安全)、composite 逐 artifact 源域+affine 映射+perf parity 拒;件2 三级精确串级 full_index→side_scan→window_carve(纯布尔)+limits 同基串级+basis token+caveat 车道(窗 carve 降级词照旧);件3 S4 剔核披露(判定零改)。**效果实测**:窗扫失败率 10ms 98%→0%/20ms 73%→0%/50ms 12%→0%;吞吐 ≈500MB/s(donghu 3.5MB=6.6ms/70MB=124ms/GB 级外推 1.8s、9MB<帽);病形钉双向(行窗 2 域错封→3 域正确);xxx_all 单簇负臂语义恒等(结构性诚实 freq_only 保持)。
**双复核(双 SHIP-WITH-FIXES 零 P0/P1)**:对抗官=修前病形四件纯净树独立复放逐值吻合+缓存投毒五形攻击(同 size+mtime 就地换写被强身份拒/超帽判决不粘代际/8 并发单飞/预算有界)+7 组评审突变全红+护栏四剥除全绿=P2(护栏零 in-tree pin);冷读官=387 行 diff 逐 hunk 零私货+用户裁定逐字对(LRU 逐出重扫裁为不违裁)+CAP-3 原文对照(侧扫零窗 crop pin 实证)+S2 四活入口手推(2 接住 1 披露 1 仍硬拒诚实列出)+P3 六件(FIFO 名不副实/失败重试残口/caveats 裸读/composite 正臂零 witness/anchor-seek 无专属 pin/账本 SUPERSEDED 义务)。
**修补轮(七件+1 真缺陷,8 突变红/绿实录)**:护栏 pin 收编四枚(代际换写拒/整文件替换拒/并发单飞恰1扫/预算有界;第四枚 mid-scan 后验=无确定性构造备案,helper 由 11 调用点覆盖);真 LRU(hit touch+move-to-back)+条目上限 32 双 pin;裁定边界 pin(reflect census freqSideScanArtifact=={curves,overflowed},新增派生字段即红+裁定注释);composite 三臂替代形(指令原形被 provenance 双红线封死;**发现并修真缺陷=超帽 union 被吞成 clock_unmappable 误标+环外帽检死代码→per-child 合并后先帽后 collected,MUT-7 红**);caveats 经 Once 路线否决(成本非零变)改单 goroutine 契约注+超宣称如实;perf 拒词面「(组合体或直查)」;失败判决按代际缓存(typed generationError,FIFO 32,open 失败蓄意不缓存;ctime 不可回拨=已亡代际判决永久正确)。-race 同集 ok。
**备案/移交**:S4 limits lane 剔核披露(后续批或永久接受待裁)/anchor-seek 未盖章形专属 pin(需大 fixture,推断覆盖已核)/15µs skew 注释量测基修订(裁定池)/S1 freq_only 成因 typed 五臂枚举+S3 rail 宇宙门+C1 burst 单次见证+C2 limits 锚+C4 掩码核数=CLUSTER-FIX-2/CLUSTER-SIG 候选批待排。**客户侧要点:复放须升级新构建(本节修复+R6 全文件基均晚于客户现用 0.1.20260710)。**

### §29.129.1 新裁定立案四件(2026-07-18,用户逐条批复;spec 落 scratchpad 待各批收账入正节)
①**LEVELMERGE-1**(「按你推荐的来」+方案 P「按推荐的来」):核实官改判(原疑三车道 HEAD 已灭:反转孪生折叠 5/5+gated 归零双层/missing_wakeup 恒0旁栏/binder 目标过滤;E28 重认定=链逐次席非 RootEvidence)→真残口=跨状态族 gated 复合重叠((pid,running) 反转席 gated 复合值计 runnable 份额与 (pid,runnable) 聚合席异组键,分支窗物理重叠同段双全额=E26+E28 Σ26.243>23.471 自洽机制,HEAD 无门零 fixture);批=件1 五层门回归 pin+件2 方案 P 区间分账(反转席保全额;聚合席拆 A 已归因份降道+B 残余参赛;A+B==修前值恒等;claim 资格=实际持席∧发布值>0;fail-open 退披露句)+件3 树面两向互指+构成行词面。**榜序变化已批**。
②**AXIOM-V2**(「同意」):用户语义定谳 verbatim=「不同的视角看同一个状态的确会有不同的净收益(折算后可消除的提升空间),用户最关心的就是能提升的空间,哪个最大,可以认为根因是他。排序根因是作为用户修复方向的指导。」→公理 v2=同线程同窗同板同口径**同修复方向**物理时间∩>0⇒恰一全额席;跨方向重叠=合法共存+「同段收益不叠加」互指;细化关系=细化席拥有重叠份(方案 P 形)。批三件=registry fix-direction 语义位(闭集,歧义 fail-open;改 registry 先读 §7.2.1+§7.4/§7.5)+跨方向不叠加互指+按方向分域守恒检查器(纯披露道,非硬拦)。
③**SPANTOP-1**(「同意」):客户诉求=链上 span 过长/过频在因果树列 top3。设计=聚合/语义族/链上 span 家族席下挂 ≤3 构成子行+余行(Σ(top3)+余==席值恒等;typed all-or-nothing 缺任一整块不发;口径词随席禁冒充等待;树面不铸「过于频繁」判词;cap=3;与「优化点表成员重复列示」备案一并归口=树 top3+明细全量两面互指)。
④**「链上」语义严守**(「这里都要注意"链上"的语义要严格遵守」):三批 spec 各加硬纪律区——链上判定=typed OnChain∧OnChainBasis∈闭集永不词面;凭证=逐段级(HULL-CRED);**分账后 B 行凭证按残余段重新成立不得自动继承 ⛓**;构成子行零链上宣称;检查器/披露种群=严格链上全额席(◇/gated 零/Self*/计数当量/包络级出圈);链上面与降道面不同行共存(E11 rider)。
**队列**:LEVELMERGE-1(§29.130)→SPANTOP-1(§29.131)→AXIOM-V2(§29.132)。

## §29.130 LEVELMERGE-1 收账+链上语义审计存档+新裁定族入案(2026-07-18;旗舰双复核=对抗 SHIP-WITH-FIXES+冷读 SHIP;修补轮八件)
**批三件(§29.129.1① 立案兑现)**:件1 五层门回归 pin 7 用例(反转孪生折叠 tieba 59566 真板 coexistence=0/gated 权威含零/missing_wakeup 零表+donghu Rank-0/binder 目标过滤+产线合成链/target_self_state 梯)——三条已灭车道钉死防复活。件2 方案 P 区间分账(rank_levelmerge_split.go):claim 资格=八合取精确链(反转型∧wakeup_chain∧GatedRunnableMs>0∧eff>0∧持席∧非Self∧on_chain∧真段库存;包络不 claim/at-cap=歧义→披露臂);A=min(|∪claim∩∪occ|,full) 纯测度,B=full−A;B 链上凭证按残余段机械重验不继承(失则 R4 typed 旗降 ◇+披露);A 行改铸 ◇ 零链上记号+demotedSide 路由;五 typed 字段 R2' 七处;两 pipeline 挂载+幂等。手算=A+B==15 恒等/跨席守恒 23→13/多 claim ∪ 非 Σ/clamp 边界/披露形主值零动。件3 两向互指(claim [E#] all-or-nothing+聚合席↔成员 ORD-A typed 谓词+同板门)+行2 四臂词面(构成/残余/裁定④句)+◎ A 行出种群+披露脚注+trunk fold 排除臂。
**双复核**:对抗官 19 突变假 pin 猎杀+值通道独立复算+四板真 trace 负控独立复放+「活体分裂不可构造」对抗验证=**结论背书且强化**(expandChain 单子展开+分支环禁 ⇒ 同 pid 因果窗跨分支恒不相交,HEAD 拓扑下正臂结构性休眠=防御性未来形);P1=PeriodicSource 聚合席入被拆种群(发布权威=VS-1 折减值 runnable+lateness 非 RunnableMs,split 抹 lateness 账 19→15 探针实锤,产线休眠故非 P0)。冷读官 SHIP:逐 hunk 零私货+三裁定链逐字+链上四纪律逐条+P3 三件。
**修补轮八件(16 突变=15 红+1 等价性证据)**:件1 P1 修=被拆种群排除 PeriodicSource(重叠测度真实在手→选披露臂非字节恒等=禁静默纪律;eff/runnable/lateness 全保+periodic 专句)+MemberFoldCaliber 皮带;件2 六加固 pin(trunk fold 臂真可折 fixture load-bearing/crossref 同板门/claim-ref+成员 all-or-nothing 两族/×N 清账+行2 臂门 FullMS>0→**ConstituentSeat bool**(清账后浮点归零不噬句,合并 Σ 行自解句;弃清 bool 案=连坐 ◎ census+fork+路由三面)/anchorFormKey 四形);件3 B 重验门探针收编 pin+假注释勘正(两可达形如实)+side-lane 选型=**复用 R4 ChainCredentialLaneDemoted typed 旗**(零 R2' 成本全链现成,语义精确同族;弃 remainderSeat 借旗=词面撒谎);件4 claim-ref stamp 同板门(与件3 对称,XLANE-3);件5 引擎披露词面「true overlap is at least this figure」统一;件6 C1/C2 针对性负臂;件7 文档勘正(clamp=无害皮带 M10 等价实证/Score 权重换轨 2.05→1.15 备案=RSPA 先例 tie-breaker 面)。
**链上语义审计存档(三命题)**:docs/design/onchain_mint_audit_20260718.md+onchain_segment_audit_20260718.md 入库。命题1(自身入链)=一致(残口=RootEvidence 自身行 legacy 词面→DISPLAY 候选);命题2=主体一致+两不一致(①无区间行伪造 overlapMs→ONCHAIN-FIX-1 已排队;②包络重叠残余→ONCHAIN-FIX-2/ENV)+五反向缺口(→3b-e/EDGES);命题3=锚窗∩非裸 proxy(窗右端点=真边,窗级等价「边前读法」),tieba 量化=proxy-only ≤2%/共享 worker 97.8%/反形漏计 20.7-26%。
**新裁定族(用户 2026-07-18,verbatim 入账)**:①「命题3:边前读法下是合法的」=97.8% 兼服形合法非偏差,FIX-4 兼服披露撤销转观察;②反向漏计定性问答=真遗漏需修复(漏因=遍历预算裁剪非语义判定);③「微窗 可以不展开了,暂时遗留」→后被 CHAIN-BUDGET 取代作废;④「MaxBranches 只选 top-N…是否可以在预算充足的情况下递归多个有价值的区间?(微窗无太多意义)」=CHAIN-BUDGET 立批:候选域=父节点 sleep 段集(用户:「值得探索的窗应该总是和关注线程的 sleep 状态重叠…上层父节点都应该处于sleep状态」=定义性非过滤,与 expandChain 机械一致;D 态=3b 射程同原则补全)+三层预算+「如果当前预算过紧…适当扩大一下预算」(实测论证三线量化够用即止)+「预算不足时,优先保证最优意思的那条。其次是 top 2,top k」=top-1 恒保贪心序(退化恒等 pin=天然回归保护)+逐跳真边硬门+预算参数入板指纹;⑤「需要裁定的部分,请先默认按照最优推荐进行实施,我后续抽空再追审核」=委托升级(ELIM-V2 不设预览门);⑥「这里都要注意"链上"的语义要严格遵守」=三批 spec 硬纪律区(已§29.129.1④)。**◎ 重设计定稿 ELIM-V2**(用户授权按最优排任务):方向分组制 A 胜出(C 口径红线一票否决/B=心算相加事故现场),节=修复方向+节序最大可消 desc+「方向间收益不可相加」结构性消解+小计三档阶梯+∩ 重叠对转录+守恒尾行;依赖 AXIOM-V2 三 typed 输入;spec=scratchpad/elim_v2_spec.md(收账时正文入 docs 随批)。
**队列**:SPANTOP-1(§29.131)→AXIOM-V2(§29.132)→ELIM-V2→ONCHAIN-FIX-1→FIX-2→CHAIN-BUDGET→3b-e;候选档=ONCHAIN-P3(含六开放问题)/ONCHAIN-ENV/EDGES 余项/FIX-4 观察/微窗作废。

## §29.131 SPANTOP-1 收账+EVALCASE-DH 立案+六问追认(2026-07-18;旗舰双复核=对抗 SHIP-WITH-FIXES+冷读 SHIP;便宜修五件)
**批(§29.129.1③ 客户诉求兑现)**:语义族席(含 semDual 链上形)行下挂 ≤3 构成 top3 子行+余行。wire 新 key member_wall_ms(R2' 七处:字段/双发射位/注册 causal_rank|hard_consumer/golden/严格解析 len==count 每项>0+aggregate ×N 清账/schema hash 重钉 e70e970b/display 消费);engine=MemberWallMsEntries all-or-nothing(空/超 cap32/任一非正→nil)+mint 盖章+re-fold 清账+语义 roster cap 8→32(EVOLUTION RECORD 标签);display=八重精确合取门(语义族∧sum_disjoint∧三载体 len==count∧desc∧µs(top1)==µs(MemberMaxMS)=§29.99件⑥复用∧Σµs==µs(行1)∧余行需E#),任一失败回落 legacy 字节恒等;词形「成员(span原文) 名 单段X.XXXms 行a..b」/「另有N段 合计X · 全清单见明细[E#]」zh/EN;B5 尾保留截断+截断撤 chip;树面零「过于频繁」判词;C4 优化点表成员重复列示归口(XLANE-2 备案兑现)。**E34 witness**=donghu_2955 JIT编译 2次 2.388ms 席:两子行 1.781+0.607=2.388ms 逐 µs 恒等+行号回源逐行核(B|/E| 时戳差=1781/607µs=原始段值 verbatim);tieba 两板 diff=0(无关席字节恒等)。「链上」硬纪律三条+原始值可见性三问全勾稽(子行值=typed 原始段值/口径词=单段闭集/恒等 pin+1µs 漂移臂)。
**双复核**:对抗官 22 突变(18 红+值通道零动=基线 worktree 四板全量 rank token 逐位 IDENTICAL+恶意名形反解不误+行号回源独立核+cap 8→32 演化对照);P2=C4 指针行「树面列前K项」在 fail-open 车道(periodic/gated/失衡→树面零成员行)作假指。冷读官 SHIP:473 行逐 hunk 零私货+偏离六条逐裁+第三面死绝 census 证实。
**便宜修五件**:件1 P2 修根=删「树面列前K项」半句改恒真中性形(「↳ 成员共N项:全清单见因果投影明细(本表不另列)」zh/EN,fail-open 形 pin+EN 词形 pin,M1 突变 4 pin 红);件2 三影子负臂全部去遮蔽成独红臂(零降档;唯 nil-wall 形数学不可独构=belt-and-braces 注);件3 semDual+spantop 组合 E2E pin(M5 挂钩删→独红);件4 注释卫生(MemberLineRanges 双车道如实/EVOLUTION 标签);件5 偏离补记两条(聚合 ×N 席出圈=范围收窄,LEVELMERGE ORD-A 两向互指已全量分解无需 top3,spec 纪律6 背书)。偏离共八条全裁(行a..b 词形/解码无 cap 双包镜像不适用/余行合成 witness/截断中切词 B5 同形/C4 无条件归口成立/roster cap 演化必要)。
**EVALCASE-DH 立案(用户授权:两客户 trace 构造场景 case 含对比/多 trace,eval+看护)**:挖掘完成=37 case 矩阵(donghu 17:帧形/锁形A容器双候选+形B哨兵/fscache 零D特异/热帽/簇三档+行窗错类/跨ns帧链;xa 10:ns 鬼 owner 全鬼/双哨兵/freq_only verdict/wakeup 全退化;对比 5:窗长归一 2.37×假象→1.47×/单边未采样/簇不对称/锁形/需求供给;多 trace 5=现状诚实拒绝串 guard pin:双 htrace 平铺硬拒/第二 systrace 权威隔离(共捕身份判定臂)/stream_scan 单工件拒)。关键身份:xxx_all.systrace md5==已入仓 tieba fixture=零新工件。踩点四件:两口径说明非 gap/**LOCKSPAN-SEAT 候选 gap 立案**(ART 锁 waiter span 未铸 blocking 席而 PresentFence 铸 conf0.72——pin 先钉现状,查实词表缺口再立批待裁)/fscache delay 单位锚 census。spec=scratchpad/evalcase_dh_spec.md,排净树错峰实施(冷读复核规格)。
**六问追认**:用户 verbatim「认可 当前判定的最优方案。」=六开放问题处置(Q1 结案零改/Q2 候选/Q3 CHAIN-BUDGET/Q4 FIX-1+2/Q5 双入口收敛+Q6 帽32 改已证下界并入 FIX-2)由委托默认升格已追认;ONCHAIN 族相关追认清单销项。
**队列**:AXIOM-V2(§29.132)→ELIM-V2→ONCHAIN-FIX-1→FIX-2(四件)→CHAIN-BUDGET→3b-e;EVALCASE-DH 错峰。

## §29.132 AXIOM-V2 收账(2026-07-18;公理 v2 机器面落地;旗舰双复核=对抗 SHIP-WITH-FIXES+冷读 SHIP;修补轮九件)
**公理 v2(用户确认原文入案)**:同线程·同窗·同板·同口径·同修复方向的物理时间∩>0⇒恰一全额席;跨方向重叠=合法共存全额+互指披露「同段收益不叠加」;细化关系=细化席拥有重叠份粗席残余参赛(方案P,§29.130 承担恰一子句)。语义定谳(用户 verbatim):「不同的视角看同一个状态的确会有不同的净收益(折算后可消除的提升空间),用户最关心的就是能提升的空间,哪个最大,可以认为根因是他。排序根因是作为用户修复方向的指导。」+「同意」。榜=修复方向指导;席值=折算后可消除空间。
**件1 registry fix-direction**:causalTokenFixDirections 53-token exhaustive 表(family-fold 先例槽位,6 列 golden 字节恒等,§7.2.1/§7.4/§7.5 红线勾稽)+闭集 7 向(调度供给/锁与优先级/IO与依赖/内存/频率与热治理/自身工作量/未定 fail-open)+FixDirection 透传+行2「修向 X」(unresolved 零佩戴)。归向要点(委托默认已实施):running 族→频率与热治理(§20.2 引擎不变量=竞争 running 席 eff 恒为折算缺口,代码+四板活体双证);gc_pause→内存(防假跨向对);missing_wakeup→未定(无边不认向);blocked_reason=Count 口径永不入种群仅词面。
**件2 跨方向互指**:严格链上全额席对(typed 种群谓词+支撑区间 basis 闭集+窗内裁剪+cap6 对称+both-or-neither prune+同板 gate+方向矛盾拒)→双向句+图例;wire CrossDirectionOverlaps 表;排序键定义句入阅读参考(键=折算后可消除量,跨方向可比不可相加;SCORE-DERIV 先例)。**活体 witness**:donghu 17267 E1(修向 频率与热治理)↔E8(IO与依赖)双向句+0.018ms(对抗官独立复核=真 typed 交集:running 恰有 18µs 整段落于 io_latency 单段内,非浮点尘);tieba_61839 0.285ms 嵌套形手算吻合。
**件3 方向分域守恒检查器(纯披露道)**:per-(pid,direction) Σ(union)>窗+1µs→typed finding+caveat 立案素材,永不拦发射(突变证:硬拦化→序数零动 pin 抓);载体缺席重叠对→undisclosed 记号。**护栏**:序数芯片唯席位家族 absence pin(5 板×2 语言;「修向」词禁佩序数突变实锤)——根因排序语义=保持一致零新 type 定谳落地。
**双复核**:对抗官 16 突变(9 红 7 存活→修补轮全补)+四板 rank dump diff=0 独立复算(值通道零动)+53 token 逐族对抗审(running 归向挑战成立)+registry 红线逐条;**P1=occurrence_windows 州混合包络入支撑闭集**(tieba 活体:occ 8.632ms 含 run/runnable/sleep 三态,union 21.42>eff 20.15=违自家包络出圈红线)。冷读官 SHIP:公理原文逐字+偏离逐裁+第七条未申报微偏离(OnChainBasis 空串=legacy 链上席入闭集=唯一可行读法,冷读背书)。
**修补轮九件(M1-M8 全红,两旧假 pin 实证)**:件1 P1 修=occurrence 臂 µs-identity 守卫(DominantImpactMs 与窗长 ±1µs 恒等才收,逐窗过滤;全拒→出种群 fail-open;四板 A/B=恰 tieba 两州混合反转席出圈零显示回归);件2-7 六假 pin 补齐(PID=0 分组负例/represented 第 14 出圈形/cap6 八席扇形/交集非嵌套形(真∩≠min)/越窗裁剪/unresolved wire 空值);件8 caveat 前缀分族(cross_direction_overlap: vs direction_conservation:);件9 素材落盘。
**偏离七条+追认项**:①Self* 豁免=Rank0 非竞争行读法(竞争 Self 席入种群,E26×E5 唯一自洽);②registry 槽位 family-fold 先例;③XLANE-2 并存句(词面折叠候选);④familyMemberIntervals 不入闭集(hull 包络出圈,io 碎片族覆盖→**ONCHAIN-ENV 衔接点名**);⑤归向委托默认三件;⑥**0.018ms 地板裁定候选**(真交集非噪;若设地板建议相对形禁绝对常数——裁定池);⑦OnChainBasis 空串入闭集(唯一可行读法)。∩ 值回源=partial(basis 侧钻取重建,XLANE-2 同形水位)——追认清单记。
**队列**:ELIM-V2(§29.133)→ONCHAIN-FIX-1→FIX-2(四件)→CHAIN-BUDGET→3b-e;EVALCASE-DH 错峰。

## §29.133 ELIM-V2 收账(2026-07-18;◎ 窗内可消除量总览方向分组重设计;旗舰双复核=双 SHIP-WITH-FIXES;修补轮八件)
**用户诉求(verbatim)**:「如下公理确认后,"◎ 窗内可消除量总览" 这个区域如何重新设计UX更清晰友好,请按照你探索后的最优建议进行排任务进行优化。」+委托升级「需要裁定的部分,请先默认按照最优推荐进行实施,我后续抽空再追审核。」设计终稿=方向分组制 A(双路探索评比:C 两层折叠制被口径零省略红线一票否决;B 平铺徽章制=心算相加事故现场否决;A 吸收 B 的 chip 两处)。
**结构落地**:节=修复方向(engine FixDirection verbatim,词表外→「方向未定/复合」恒尾),节序=节内最大可消 desc;节头「▸ 方向 · 最大可消X[· N席 · 小计Y(区间互斥)]」;防跨方向相加三层=区头「方向间收益不可相加」恒发+∩ chip 仅真 typed 重叠对(树行互指句同源 both-with-tree)+合并脚注「全句见树行互指」;小计三档(L1=typed 包络两两互斥充分条件(engine 包络⊇支撑段数学核)→Σ 恒等,L2 重叠→「合计不可直加」,L3 载体缺席/merged/跨板→零算术);◇ 块头「邻近(条件可消上界 · 不入方向守恒)」并存才发;守恒尾行(违例=typed finding 转录/pass=「各方向支撑区间并集皆 ≤ 窗X(检查器)」方向世代门,legacy 双态不发);⌗症状▒ 脚注原位字节回归;⛓ 先 ◇;◎ 节零序数(护栏①全兑现);闭合恒等式新四支(无跨方向总计 absence/L1 小计 fence 字节重构/档位 absence/bar 基准不变),elimgap walker 零改全绿。**件1b 根因修复(批中发现)**:FixDirection 穿 fold/absorb 丢失→三点回填(R1 absorb/语义 donor/rankFoldPeers),witness=17267 反转双席 未定→锁与优先级。真板对照全文入档(elimv2_out_pre|post):donghu_17267 修后=四方向节+∩[E1]∩[E8(+1)] 0.018ms+L1 小计 12.115=7.405+4.710 µs 恒等+守恒尾行。
**双复核**:对抗官 11 突变(6 红 5 幸存→修补轮全补)+存档不信任独立复渲逐字节+值通道多重集恒等+L1 数学核+闭合 walker 机械论证;P1=件1b 载荷回填点(rankFoldPeers)零 pin(剥除→witness 全面回归而全套仍绿)。冷读官=mock 兑现度+词面撞义+119ch 实测勘正。
**修补轮八件(9 突变全红)**:件1 P1=donghu.ftrace 17267 真板产线渲染 pin(剥除→「方向未定」回归原文);件2 两「惰性」回填点**全部构造成活体**(absorb 收养/语义 donor 收养产线链 fixture,非死代码=engine 只在 rank 记录盖 note,chain-view/display copy 天然裸);件3 L1 跨板门收紧(跨板尺/混合形→L3,负臂×2);件4 ◇ 块头 absence pin;件5 EN "section" 撞义修(board TOP1/board-wide maximum);件6 三小件(dedup 键并线程锚/NaN Inf 拒收臂 8→10/小计打印面取整根除 float dust=0.0045 见证);件7 守恒 pass 残余缺口=扫描扩到排除前种群(排除载体带违例→违例行发+pass 让位,pin);件8 落盘勘正(member 行最宽 119=∩ chip +11ch 行为无回归/结构行 ≤63/>120 零行;DOM 真浏览器量测缺口开放=复核官亦不可用)。
**偏离八条+委托默认(待追认)**:spec-wire 形不一致适配(L1 显示侧判据+pass 世代 proxy)/direction_conservation_excess 升 hard_consumer(NKR 四同步)/件1b 非列项根因修/DOM 静态 ch 网格代测/63ch 超 60 软标/◇-only 不发块头(新增委托点)/tieba 代理改形/守恒尾行恒发+◇ ·方向=X+单席节头不合并三默认落地。
**队列**:ONCHAIN-FIX-1(§29.134)→FIX-2(四件)→CHAIN-BUDGET→3b-e;EVALCASE-DH 错峰。

## §29.134 ONCHAIN-FIX-1 收账(2026-07-18;命题2 不一致①+命题1 残口;合并复核 SHIP 零修补轮;用户已追认族)
**件1 伪造 overlapMs 修诚实**:chainContextForCandidate 同 pid 无区间臂——keep on_chain 保留(fail-open 既裁),overlapMs 不再铸(修前=整节点窗墙钟伪造),新 typed 位 identityInheritance(准入时记录语义,复核突变①证与「降道后清位」替代四板逐字节等价=行为免费);两 stamp+HULL-CRED 四臂判定前统一退位(判定行说判定词);R2' 七处(chain_identity_inheritance 键+hash 重钉 fe96555d);词面「身份继承(链窗级,无区间凭证,见图例)」zh/EN+图例+强压弱四重 gate。**消费面全查表**(复核官自绘 grep 逐点对上+补核两处语义车道精确交集发射点=非伪造消费者):席值/序数/Score 全域零参与(Q4-B advisory-only),四真板 RANKVAL 逐字节恒等=值通道零动实证;**附带收益**=fall-through 降道行(trace_gap/missing_wakeup)wire 伪造 overlap 残留一并修愈;critical 面记号产线实锤(donghu_17267 板 2955×4+17439,target 自身 R8 豁免)。
**件2 RootEvidence 自身行词面**:on_wakeup_chain→SelfWallClock(ELIM-SELF-FIX 通道身份先例);消费面查出一处值通道并精确补偿=rankCausalThreadSet 加 self token 臂**限 Source 前缀 wakeup_chain**(不补=target 掉兜底因果集:scheduler 席失 causality 词面+非 target 行失 35% 窗帽,长等待最高 ≈2.86× 值膨胀——复核量化;不限=SELF-basis/症状行扩张,双向负臂 pin;症状翻转行靠 pass 顺序免疫=复核 P3-2 时序耦合观察入 checklist)。
**复核(合并官 SHIP,3 突变+四板 A/B)**:全 P3——P3-1 偏离② 措辞勘正(「interval-less 有凭证 pid 走 envelope 词」仅对入 dioDecisions 的 pid 成立,tieba 60555 本窗无条目佩身份词=行级机械诚实,全称句改如实);P3-2 补偿臂 pass 顺序耦合观察;P3-3 IOBurstEpisodeSummary 无记号=构造不可达且回归也是诚实零。
**队列**:ONCHAIN-FIX-2(§29.135,四件:包络泛化+Q5 双入口收敛+Q6 帽32 已证下界+两不一致余)→CHAIN-BUDGET→3b-e;EVALCASE-DH 错峰。

## §29.135 ONCHAIN-FIX-2 收账(2026-07-18;四件全落=包络泛化+Q5 双入口收敛+Q6 帽32 已证下界+io 碎片族衔接;旗舰双复核=对抗 SHIP-WITH-FIXES+冷读 SHIP;便宜修五件)
**件2(Q5 追认兑现)双入口判定机器收敛**:三产线入口(Run getBlocking 链先行/BuildCriticalBlockingCalls 链先行/bundle 既有)全喂锚定 stats,判定机器单点零动;自愈臂兜未来入口(便宜修加固=删 cachedChain==nil 前置闸,chain 已供+无锚 stats 第五形从 cachedChain 取锚零成本重扫,MUT-B 精确复现原 FORK);修前分叉甚至视图顺序依赖(rank 先跑翻转直连 verdict)已灭;**副产品=donghu 直连假⛓未分解镜像行退场**(Q5 修复目标形,信息保全经真板独立验证=双向对席句 9 处保留+36.757 拼回恒等,rspa_mirror pin 演化冻结收敛形)。
**件3(Q6 追认兑现)帽32 弃全→前缀已证下界**:源头前缀不可变+溢出闩;partial 臂逐段合法∧Σ≤账+tol+**len==cap 等式**(便宜修补=短闩非法形拒);前缀 ≥1 段∩→keep+typed truncated+词面「(凭证清单不完整,实际锚定不小于所证,见图例)」(避「下界」撞词=XERR1 第三语义占用,内部注释八处统一实词);**前缀全不交→keepEnvelope 禁判 disjoint(缺证≠证无)不发布不佩证**;hull∅ 臂零动(hull 含未采段=∅ 恒完备证明,复核数学核);cap=32 保持(实测 tieba max=12/donghu max=5 headroom 2.6×,悬崖已除上调收益机制性归零;67M 独立量测未获=诚实备案)。
**件4 io 碎片族衔接(AXIOM-V2 偏离④兑现)**:familyMemberIntervals(hull)维持出圈;真段载体 dioSegmentIntervals 下推(全有或全无四重门=sum_disjoint∧全 wholeTd∧零溢出∧Σ≈账)→AXIOM 支撑闭集新臂 dio_segment_intervals(方向互指/守恒种群受益)。
**件1 包络泛化**:26 类型处置清单成文(RSPA 9/语义 6/µs 恒等机械臂族/typed 对/构造性/闭表外空真);残余 hull-only keep→ChainCredentialEnvelopeLevel 复用(零新词);enrich 尾无条件重算(便宜修=assign 语义真 pin,粘滞突变红)。
**双复核**:对抗官=值通道零动四真板 RANK 面零 diff+镜像行退场信息保全独立验证+件3 数学核(词值关系=发布值恒全额且 anchoredMs 含超帽段故「不小于所证」如实)+cap 复测全等+dioIntervals 消费面全清点;P2×2=电池主张不实(M1 入口单独回退靠自愈兜底存活)+stamp 幂等零 pin(清除断言空转)。冷读官 SHIP=Q5/Q6 追认逐字兑现+26 类型 vs 审计表零漏+偏离逐裁。
**便宜修五件(6 突变)**:自愈加固+四五形收敛 pin(连锁=onchain_fix1 FailOpen 见证改真不可锚链 EVOLUTION);stamp assign 真 pin(MUT-C=旧 pin 绿新 pin 红双实锤);内部词面八处统一;len==cap+短闩非法 pin;落盘勘正(M1/E2E 措辞+偏离七条成文)。
**队列**:CHAIN-BUDGET(§29.136)→3b-e 逐件评估;EVALCASE-DH 错峰。

## §29.136 CHAIN-BUDGET 收账(2026-07-18;多 sleep 段预算递归;**战役第二例 REJECT→返工→复审 SHIP 闭环**;用户四裁定 verbatim 兑现)
**用户四裁定(verbatim,§29.129.1 后陆续裁,此处正节兑现)**:①「MaxBranches 只选 top-N 分支、depth≥1 只递归单个最有趣区间,是否可以在预算充足的情况下,可以递归 多个 有价值的区间?(微窗无太多意义)」=多区间递归取代微窗(微窗作废);②「值得探索的窗应该总是和关注线程的 sleep 状态重叠…上层父节点都是应该处于sleep状态」=候选域定义性钉死(节点窗 typed sleep 段集,唤醒边终结 sleep,running/runnable 不递归);③「如果当前预算过紧…可以考虑适当扩大一下预算」=实测论证扩容;④「预算不足时,优先保证 最优意思的那条。其次是 top 2,top k 等」=top-1 恒保贪心序。
**实现**:候选域=节点窗 sleep 段集+地板 1.0ms 导出常量+每节点帽(MaxBranches 泛化全深度);全局 frontier 贪心(值 desc→ts asc→注册序);top-1 与 extras 共用同一凭证臂(逐跳 sched_wakeup 硬门零松动,extras 无边不铸 missing_wakeup);耗尽 typed note 逐字+双 knob 词面;扩容=MaxBranches 8→16+MaxChainNodes=96(实测需求 12/13/max 73,O10 换代);SegmentOrdinal typed 面(legacy 恒缺省);树形=primary spine+side_chains+extra 边真路径 resolver;指纹 += max_chain_nodes(additive 不合板);视图/探索预算分离(LLM 面 ≤32 帽实证)。**验收**:tieba 反形漏计 5.436(20.7%)→0.888(3.4%)/5.173(26.0%)→1.304(6.6%)(pin⑨);RSPA 锚定随动 21.242→26.001/14.700→18.979=QUAD 逐位吻合,demoted 行发布值零动;成本真 A/B=+27.3%/+63.5%/+44.1%(独立复测吻合,预算硬界+96 过冲界实证)。
**REJECT(对抗官,P1×2)**:P1-1 贪心序假 pin(值序反转全量绿=fixture 预算从不中途耗尽序不可观测);P1-2 涟漪挤出真值行(新 depth-0 分支产零值 trace_gap 洪水+同 pid 双行,帽值盲排序挤出真披露行)。**返工**:P1-1=contest fixture 值序/时序反向设计(预算恰容一条,序首次可观测;M-D/M-E 双突变红=两不同幻胜者);P1-2 三层修根=铸点 per-(pid,gapKind) fold+同窗去重(持值席 pid 拒零值盲区行=§27.2 自相矛盾病形)+帽内先保真值行(发布 eff desc,零值恒后)——洪水受害全回归(tieba sleep_wait×2/17267 pacing_idle 15.758);**诚实披露**=OS_FFRT 19.984+donghu2955 四行(76.800/27.507/21.923/10.044)+keva 1.354 非洪水受害而是**有值 on_chain 新行合法帽亡**(逐席死因表:挤出者全为真边新席 eff>0=漏计回收本身),按 R-2 既裁(帽亡=披露道非保席)不越权保席,压缩 caveat 在岗,**备案三选项入追认清单**;via 回滚腐坏形 pin(幻影节点实录);P3 六件(双 knob/成本勘正/17267 自身 runnable 三席 Σ6.797 两把尺三问备案/注释限定/segment_ordinal 双文本面/无边候选观察归 FIX-3b)。**自纠件**:PerfBundleViewAlias 断言载体=零值矛盾病形原文→断言随语义演化(EVOLUTION+负断言);教训=包级 suite 禁管道 tail 取 EXIT(假绿实锤),重定向直读。**复审(单官对抗规格)=SHIP**:11/11 原 findings 销案(全部首手复放非转录采信;两幻胜者实录/逐席死因表逐行对账/QUAD 双闭合/跨进程 dump 字节恒等);诚实帽亡裁决独立背书;新 findings 全 P3(注释措辞/备案归集)。
**追认清单新增**:①帽亡真值行处置三选项(维持披露道/背景保席侧道独立裁定/MaxLimit 上调)②17267 自身 runnable 两把尺跨行恒等句(非便宜修)③cb_rework P3-A 注释第三对手序措辞。**队列**:EVALCASE-DH(§29.137 候选,净树错峰)+3b-e 逐件评估(无边候选披露归 3b)。

## §29.137 EVALCASE-DH 收账(2026-07-19;客户 trace 场景 eval 电池+长期看护;冷读 SHIP-WITH-FIXES+主会话两手修)
**用户授权(verbatim)**:「可以从 /Users/han/opt/donghu 的两份客户反馈的trace片段里,可以探索一下,抽取关键信息,构造各种场景,切含两个 trace比较场景,以及多个trace分析的场景,等的cases,进行各个细化场景的eval测试和看护。」
**交付**:27/37 case 落地(14 新文件,引擎零改;预期=当前 HEAD 现渲+独立手算双源,五窗手算逐位一致):引擎 pin 族=DH-F1 簇三档正例+行窗 2 域冒充负例(fmax 低估 2.148× 病形钉)/DH-J1 J2 帧形(跨簇分席手算)/DH-L1 形A 容器双候选+「no thread-level mapping material」诚实弃权词逐字+DH-L2 双哨兵 typed ownerless/DH-S1 envelope≠wait 词面/DH-T1 热帽抬 fmax+T2 Tier-2 空转诚实降级/DH-IO1 fscache 零 D 特异(census 锚,单位不锚=spec 踩点④)/DH-IO2 churn 风暴/XA-L2 四鬼 owner 全 host 缺席+L3 84 形B+30 哨兵 census/XA-F1 freq_only+R6 首簇 donor typed/XA-W1 退化 advisory(token 化 pin);CMP 机制 pin 五轴(窗长归一 2.37×假象→1.47×/单边未采样 donor 禁冒充/折算基分侧/锁形不对称归一反转/需求供给车道分离);多 trace 承诺面 guard 五(平铺硬拒串/第二 systrace 权威隔离+共捕身份臂/stream_scan 拒+indexed 通行——三串逐字 pin,改一字→红实证);tracediag 零 LLM 配置五(J3 双窗分化/IO2/R1/C2 多 target/XA-V1 微窗,e2e 全步成功)。跳过 10 全备案(金样三件 spec 自标缓/两件被吸收/三件机制既有覆盖;冷读裁 DH-B1 binder ns 命名陷阱+XA-R1 rank 席位形两条牵强→**小回访项立案**)。
**LOCKSPAN-SEAT 调查结论(踩点②)**:既非词表缺口也非地板——词表照 admit(7 条 contention span 全 admit)、配对完备;真机制=boundTraceMarkSpansWithInfo 的 **display top-8 时长界坐在 blocking carve 上游**(µs~亚 ms 锁 span 被 ≥3.4ms envelope span 挤出;semantic-work-class 有保留臂而锁形不在词类内)=「帽基当全量」族结构形,**证据价值倒挂**(PresentFence envelope 铸席、ART 锁证词无席;复核独立背书+两佐证:generic 截断零披露/INODE §28.6「never the top-8 slices」先例与修向 B 同向)。修向 A/B 素材落盘,现状三段 pin 钉死,**待裁项入裁定池**。
**复核(冷读 SHIP-WITH-FIXES)+主会话手修两件**:F1 P2=DH-IO1 曾 pin delay 值(违 spec 踩点④+收账句失实)→退 census 锚(caller+count),手算记录降观察注;F2 P3=XA-W1 全句 == pin advisory 显示面→token 化(marker+census+advisory 三 token);F3 显示耦合热点备案(J3 e2e 整行 pin+五报文 token=显示批 first-touch 名单);F4 跳过两条牵强→小回访项。复核实证=八组 raw 字节对账全中+承诺串三处改字母三红+-count=2 确定性。
**队列**:3b-e 评估官→追认清单滚动归集→全窗汇报;裁定池新增=LOCKSPAN-SEAT 修向 A/B。

## §29.138 收尾双件:追认清单滚动归集+3b-e 边种评估(2026-07-19;docs 节)
**追认清单更新**(ratify_checklist_20260717.md):R-13..R-20 新增八正式项(含 24 子决策,4 待裁:帽亡三选项/17267 两把尺/0.018ms 相对地板/LOCKSPAN-SEAT A/B)+R-13b(汇流勘定=毒化回执字段属原始扫描完整性事实准入缓存)+销项区(六问处置已追认销)+附录 A 增七行(含 15µs skew 量测基修订=裁定池);双基线声明+子项寻址规则。
**3b-e 评估**(docs/design/edge3be_eval_20260719.md,四旗舰窗+全 trace+67M 分窗实测):**3c 裸 census 边状态席=四件唯一立即实施**——R3 凭证臂(hostSemanticSpanEdgeAnchor)只挂语义 span,非链 host 状态席无门入 ⛓;witness 在库(SCAN-3 61839 判例=dio 3.550+runnable 0.370 边前份)+收益量化(tieba flag +17.9ms/trace 窗 +40.4ms/donghu2955 +10.7ms/17267 +0.3 天然负臂)=既有 ◇ 行换道+二分零新 ms 铸造;200-400 LOC 凭证函数零改复用;涟漪=tieba 13.959ms 行升 ⛓ 榜序显著重排(需 XLANE-2 式重叠披露)。**3b D/IO 闭合铸节点=缓办**(收益 0.118ms+IO 配对环依赖需共享 pre-pass 重构;CHAIN-BUDGET 已留接口位;「无边候选零披露」观察项四案全零无 witness 随行);**3e binder 多跳=缓办**(14200 条事务 nested sync=0 零 witness);**3d 锁二跳=需新裁定殿后**(depth-1 是 pinned 形状属性+真 trace 两跳=0,LOCKNS 域)。**队列重排(委托默认)**:spec 序 3b→3c→3d→3e 改为 **3c→(3b/3e 候选)→3d(待裁)**。副产物备案=blocking_span 消费 top-8 帽视图(LOCKSPAN-SEAT 姊妹口,今天 µs 级无实害)。裁定点三件(3c basis token 选型/3b 指纹闭集/3d 推翻 q1)=委托默认待追认。

## §29.139 ONCHAIN-3c 收账(2026-07-19;裸 census 边状态席=R3 凭证臂扩射程;旗舰双复核=对抗 SHIP-WITH-FIXES+冷读 SHIP;便宜修五件)
**批(§29.138 评估「四件唯一立即实施」兑现)**:非链 host 状态席(runnable/D-IO)持真实 typed census 边时入 ⛓——新 pass anchorBareCensusEdgeStateSeats(rank_state_edge_anchor.go ~470 行):凭证函数 hostSemanticSpanEdgeAnchor **零改复用**(对 main 零 diff=单一边判定权威,对抗官实证);逐段二分 semanticEdgeAnchorSplit 全模板;州分账双载体(D/IO 双通道);12 fail-closed 形逐字节 pin(宁漏勿假指);R4-mirror 门=gated 复合席禁沿边界拆(RSPA R4/§29.83 既裁),全边前→整席换道值零动/有边后份→整席不动。**basis 选型(委托默认)=sibling `host_wakeup_edge_pre_state`**(三理由复核独立裁全成立:计入值语义真差异/闭集文档钉死 pre_span 载体/R3 span pins 零波及 76 pin 绿实证);R2' 七处。mint 域扩 edge host(帽基当全量第五例,准入=examine 非 convert)。
**四板收益(勘正版)**:tieba flag=dio 3.550 全转+run 0.370⛓/0.075◇(SCAN-3 61839 判例正收,恒等式 0.445 手算);tieba trace=dio 3.750+run 3.126⛓+倒装整席换道 **19.358**(Chrome_IOThread 全边前;初报 19.438 含 RenderThread 0.080=链成员不可换道,复核勘正);donghu2955=换道 0.241(2614 partial 形 R4-mirror 拒=门正确);donghu17267=run 0.161⛓ 六 host 二分(µs 天然负臂)。**收益框架词勘正=「换道+新铸双形」**(活面七例中五例=mint 域扩新铸席;「零新 ms 铸造」只对值面守恒成立)。
**双复核**:对抗官 10 突变(9 红+M10 幸存→修补)+二分几何自写算术独立核+四板 A/B 全量 dump 逐席归因+值权属核(换道=census 真值重发布=RSPA 链席全账先例);P2=hull 清空臂空转 pin(fixture 从不设 hull)。冷读官 SHIP=R3 三纪律移植保真+sibling 三理由独立裁+12 形完备性+零私货。
**便宜修五件**:件1 P2=bisect fixture 非空 hull+清空断言真咬合+cross-type recon 假匹配负臂(M10 复放红);件2 发布面挤出三涟漪补账(RenderThread 链上席被 60560 挤出/HiPlayer 被 2931/keva ◇ 被五孪生=诚实帽机制账在池)+链上席挤出形 pin(60560 在榜∧59891 落池∧Total>Emitted 披露);件3 注释「never strands…will not examine」双处对齐+23088 拒转活 pin(倒装席留池普通形钉死);件4 XLANE3 fatalf E43→E44+括注位置微差备案;件5 落盘勘正(19.358/双形框架词/9644 池内重复观察)。
**裁定池新增**:partial 倒装复合席边前份拆分(LEVELMERGE 式拆分推广到 gated 复合席=收 23088 的 13.959 需此新裁定;素材=o3c_progress.md 偏离②)。**队列排空**:3b/3e=缓办候选档;3d=需推翻 q1 depth-1 裁定殿后;余=裁定池+追认清单+回访 roster 待用户。

## §29.140 eval 战役审计立案(2026-07-19;10 案×N=2=20 run:16 PASS/4 FAIL 零 flake 全归因;双官审计存档=docs/design/eval_audit_process/answer_20260719.md)
**用户指令(verbatim)**:「检查状态,并跑几组trace和读写模式的eval,审计过程和答案,挖掘系统GAP。」发射=trace 5+读 3+写 2(快照 binary 含 3c pin;driver 首轮 GNU timeout 秒败 127 教训=macOS 无 timeout,去掉重发)。
**FAIL 归因(确定性,零 flake)**:①zod_prefault ×2=**P0 写模式双 bug**:GAP-1 verify-failure handoff 把 runner 名注入 suite(黑名单挡 "make-test" 漏 "make")→run_tests 执行 `make make`→真断言失败洗成 UNAVAILABLE→write_report_failed;GAP-2 幽灵路径使 checkpoint git add 整批 abort(仅 WARN)→核心修复永失 durable applied ref(run-1 ref 复放 make check FAIL=交付面破损;run-2 ref 复放 PASS=假 FAIL 代码正确);伴生 G3=终答把这一切谎报「环境缺少测试运行器」并引导 /merge(词面谎)。②data_multifile run-2=**P1 DL 家族新亚种**:entity_resolutions 义务「Present(materialized)」与「satisfied(records>0)」双权威分叉→阶段机短路跳 normalize→emit 段门 [reconcile,assemble] 无 producer→repair 6 轮互相矛盾硬拒→terminal failed(正确答案 17,0,5 round-8 已算出被扣发;既有 no-deadlock pin 只查路由可达性漏此形;run-1 走 normalize 路径 PASS=同案掷币)。
**过程面(20 run)**:LENSBURN 修后成立(8/10 trace run 单 emit 零烧)+TOOLWIN 全绿(首 trace_query 全 ok)+STREAM-WAIT 后首个全绿窗(0 LLM 重试 0 >60s);例外=**GAP-4 mixed-origin 完成门对显式「不分析代码」的纯 trace 问重派 explorer ×2**(~180s/run,trace_query 21 vs 常态 3,orchestrator:7454=CSP#63 残口同族,soft lane 驱动硬重派后仍 caveat 放行)。write 生命周期 L5 零泄漏。
**答案面**:grounding 抽验=新引擎面全对(E20 同源二分 0.445 与 §29.139 账本逐字/◎ 方向分组 L1 恒等 1.675 手算过/凭证词逐字);**G4=c2 run-1 LLM 值错三连**(亲见 blocked_reason 行 2533 又丢弃/running 段 1.074 冒充 D/同案两 run 结论矛盾双 PASS);**G6=a3 覆盖缺口 1.954ms 被四态守恒静默折进 running 无披露**;G7=semantic run-2 标题「根因是优先级反转」与自贴投影(主根因=类校验,无反转席)矛盾=HEADLINE 家族;G9=段值冒充窗口总量;**观察=LLM 叙事层对本窗全部新词面零消费**(实渲全对但 narrative 0 引用,教学候选);G13=SPANTOP top3/∩ 互指零实渲(案单 coverage 缺口)。**PASS 掩盖四实锤**:a3 oracle 正则 `5[01] ?ms` 被 "0.350ms" 子串假阳(52.478 出 band 无声放行)/c2 值错三连漏/multicausal 时间戳靠 dump 命中/G7 矛盾句 PASS。
**修复编队(委托默认排队,待追认)**:**WRITEFIX-1**(P0×2+G3:suite 黑名单加 runner 名等值+make 车道 Suite 记真 target/checkpoint 按 AppliedSet 提交+commit 失败升 typed+eval EXPECT 改读 durable ref/「未验证」词面诚实)→**DATAGATE-1**(G10/GAP-3:义务 Present/satisfied 单一 typed 谓词双面共用(materialized=已偿,评估最优默认待追认)+门/hint 矛盾修+no-deadlock pin 补双权威同判臂)→**EVALGUARD**(G5 oracle 数字边界守卫(h5 先例)+a3 band 修+c2 EXPECT 补+G12 代表窗+G13 SPANTOP/∩ 案补+plan 步产物独立留档)→**ANSWERFACE-1**(G4 blocked_reason 教学/G6 覆盖缺口披露/G7 标题-投影一致性(HEADLINE 家族扩)/G8 双车道同形互指/G9 段值-总量教学/新词面叙事消费教学=prose lexicon)→**COMPLETE-2**(GAP-4:soft+downgradable lane 结构不可补时首轮放行,完成门权属合规)→SPANVIS-1(spec 六节已齐)。GAP-1/2 witness=eval/results/*20260719* 保留。

## §29.141 WRITEFIX-1 收账(2026-07-19;eval 审计 P0×2+G3 写模式修复;**战役第三例 REJECT→返工→复审 SHIP 闭环**)
**三件(§29.140 GAP-1/2/G3 兑现)**:件1 GAP-1 suite 污染=三修向全落(黑名单 runner 名等值(12 名单一来源+同步哨+解析器合成名族)/make 车道 Suite 记真 target(parseMakeOutput(target)源头诚实)/拒后空选择器回落 typed 检测非重试环)+source 标签修(verify_failure_handoff);件2 GAP-2 checkpoint=四层(worktree splitCommitPathsPresent 幽灵容缺(在盘∨tracked;tracked 删除照常)/AppliedSet 提交(跨 plan 累积自愈+空集回落 owned)/ApplyCheckpointRecord typed 披露非硬拦(完成门权属,M-K 双向钉)/eval durable-ref-first(materialize 优先+回落即红+fake binary 两病形臂));件3 G3 词面=reportUntriedRunnableCandidate 前置(有未试候选→「验证命令失败:<真实错误>」禁环境谎词)+多 ref 按序指引。**witness 复放**:zod run-1 applied-tree make check RED(交付面破损直接可见=旧基建读活 worktree 掩盖)/run-2 PASS(假败转正=新引擎 verify#2 真跑 make check)。LOC ratchet 拆 write_verify_render.go(318 行词面单点)。
**REJECT(对抗官,P1×2 双官同抓其一)**:P1-1=新诚实词面在 runner 真缺失形上**反向撒谎**(runner_missing 行被排除在「已尝试」外→候选当 untried→「验证环境并未缺失」+/verify 重试=GAP-5 镜像谎,正打在本批自修的诚实面上);P1-2=appliedRecoveryRefs 缺 per-Run 重置(REPL 跨轮列已 reject/merge 旧 ref=假指引)。**返工**:P1-1 修根=三车道 Outcome 分类(ran 族/环境拒绝族 disqualify(runner_missing+not_configured)/synthetic 保持/default 保守双向=不算跑过∧不铸环境完好词);P1-2=重置块+doc+真双 Run pin;P2×4(multi-ref union changed-paths overlay(兄弟全树覆盖回退病形 M9 红实锤)/broken-checkpoint 指引可达化(「/approve --retry」+willPreserve 分支=非 preserved 如实「字节不可恢复」)/REPL chip 拆臂/restore 面 disclosure 双臂);P3×4(ratchet 收 9260+hot 表/make -C 现状 pin/枚举注释/备案注)。9/9 突变 KILLED。
**复审(单官对抗规格)=SHIP**:原 findings 11/11 销案(三车道全 census+mixed 第四形构造探针=disqualify 恒胜且今日不可达/reset 块全字段 census 无同族漏/union 排序 committerdate 主序实证/「/approve --retry」命令面真存在/witness 两 run 结论逐字复放);新 findings 全 P3/P4——F-N1 电池洞(default→ran 突变逃逸)**主会话随收账补一臂**(make 候选+unknown outcome+命令错配→nil,pin 在案);F-N2/3/4=观察与 doc 备案。
**队列**:DATAGATE-1(§29.142)→EVALGUARD→ANSWERFACE-1→COMPLETE-2→SPANVIS-1。

## §29.142 DATAGATE-1 收账(2026-07-19;eval 审计 GAP-3 起**七轮 witness 复放剥洋葱**;合并复核 SHIP-WITH-FIXES 三 P1 修补;复放#7 双 PASS 17,0,5)
**剥洋葱七轮(每轮直读产线 verdict 归因,零 flake)**:#1 双 FAIL=义务集死锁(Present/satisfied 双权威分叉,正确答案已算出被扣发)→原三件修(单谓词 EntityResolutionObligationSatisfied 双面共用/五面同判枚举 4096 态/repair hint 与段门一致性);#2 死锁灭但答案双错(17,5 丢槽/17,4,5 非 target 组顶槽)→件A 参照集基数臂+件B 逐项参照接地臂(真值定谳=T2:GroupX 零映射必须 0);#3 run-2=round15 已算对 17,0,5 被 round16/17 劣化重投影覆写发布「0」→件C 答案权威粘滞门;#4 run-1「37」(模型 declare plain_single_line 绕激活)→件D 参照约束激活典藏化(typed census 推导,模型声明不可豁免)+run-1 验证器完美触发诚实 failed 实证;#5 双错误出厂(杂形「value=11;GroupA/value=17;…」+「0,0,0」零期望反转)→**发布咽喉图裁定**(RunDataTaskCLI→五答案出口→finalDataTaskAnswerForCLI→粘滞→完成门→candidacy→dataTaskAnswerMarkdown verbatim→stdout;**十 validator 全坐咽喉,无 LLM 复述面,#5 逃逸=validator 内部三洞非旁路**——主会话「外层复述」假设被裁否)+三修(杂形臂/零期望反转→inapplicable/audit-only role-starved 红)+件E1-3(绕过关死/repair 无 tool_call 硬化/贡献接地臂)+terminal json PublishedAnswer 审计面修;#6 双 PASS;合并复核对抗又打出三 P1(key-echo 混合形 17,GroupX,5 逃逸/totals 单行投毒关整门/粘滞直取收 inapplicable 违自述契约)+P2 咽喉调用点零 pin(删三行整包 SURVIVED)→修补轮六件(uniform echo/armA 独立+per-key/直取同契约/咽喉 pin/两 P3);#7 双 PASS。
**分型统计(修前 14 run)**:5 PASS/2 诚实 failed/7 错误出厂;**修后不变式=validator 红则永不 complete 出厂**(咽喉 pin+突变链看守)。突变累计 M1-M22+复核 V/A 系+修补轮,全 KILLED。
**裁定池新增**:E-2 机械代发边界=「系统可提案不可代答」(assemble_answer 入 fallback plan 候选可,绕 planner 直执不可;提案车道合成参数须过继 C1/C3 凭证)——feedback_no_system_backfill 红线边界备忘,留用户裁。**备案**:REPL 成功路径 PublishedAnswer 未接/empty_semantic+materialized 残口(pre-existing)/F5 反转 repl 侧独立 pin(dataquery 臂承重论证)。
**教训入库**:①witness 复放逐轮直读 verdict 是剥多层病的唯一可靠环(六层每层都被「pin 全绿」掩着);②新建 validator 必须对抗构造混合/投毒/反转形(修 A 谎易铸 ¬A 谎的第二例=grounding 门自身三逃逸口);③「fixture 链≠产线链」怀疑要用咽喉图证实/证伪(本例证伪=门都在咽喉,洞在门内)。
**队列**:EVALGUARD(§29.143)→ANSWERFACE-1→COMPLETE-2→SPANVIS-1。

## §29.143 EVALGUARD 收账+INTERSECT-REG 回归定谳(2026-07-19;冷读 SHIP+主会话两 P3 手修;h11 新案首战抓获真回归)
**五件(§29.140 G5/G12/G13+基建)**:件1 a3 oracle 数字边界(`5[01] ?ms` 被 "0.350ms" 子串假阳处死;band 按当前真值 52.478=50.524+1.954 重定 52±1);件2 c2 三真值锚(count=3 共现/Σ=0.635 边界/自身·D-state 诚实缺席 ban-token,回放=值错 run 正确翻红);件3 G12 代表窗落叙事面(邻近形 TEXT regex);件4 **两 coverage 新案**=h10 SPANTOP 子行(冒烟 PASS=top3 面产线 LLM 链全量实渲首证)+h11 ∩ 跨方向(KNOWN-RED 守卫);件5 plan artifacts 逐代留档(plan-1/plan-2,patch_go 实证 plan-2 独有键=历史被覆写吞掉的 apply 印章面)。**同型正则 census=58 案修**(裸短整数/时间戳吃裸 34/日期吃裸 20 等全族边界化);441 存档回放 0 误红+13 designed flips 逐条审计;LLM 冒烟 8 run(a3/h10/patch_go PASS;c2 两诚实红=G4 witness;multicausal#1 诚实红=G12 病灶本尊被新锚正确拒)。冷读 SHIP(4 P3):perfetto 首支边界不对称+N_DEFAULT 三态无常驻 pin=主会话手修(regex 对称化+runner_lib_test 三态合同);另两观察记录。
**INTERSECT-REG 回归定谳(h11 首战立功)**:∩ 对(§29.133 存档 [E1]∩[E8(+1)] 0.018ms)消失于 **1ada2c49f(CHAIN-BUDGET)**——sleep 探索放宽后 io_latency 单段行(single_segment_identity basis)被家族折叠吸并(5→6 members),familyMemberIntervals 被 §29.132 偏离④ 闭集按「per-(thread,cpu) hulls」理由整体排除→basis 缺席→席出种群→对不再探测(种群出局=静默臂零记号,六 commit 无人察觉)。**决定性证据**:HEAD dump=家族席 foldCaliber=interval_union、6 段齐全、union==发布值到 µs(真段非 hull!)、与 E1 支撑实测交集 0.230ms>0——载体在+物理重叠在+typed-但-理由不适用的排除=**回归臂非诚实消失**(无假值出厂,旗舰披露承诺面静默丢失)。修点素材就绪=偏离④ 收窄(familyMemberIntervals 精确准入:foldCaliber∈{interval_union,sum_disjoint}∧|union−ImpactMs|≤tol µs 恒等——hull census 族天然过不了=原保护不破;ONCHAIN-2 件4 同范式);h11 处置=修后按活体 0.230ms 形 re-base(0.018 是旧板伙伴不可复现);P3 观察=种群出局静默臂补 debug/caveat 软披露。**修复批 INTERSECT-FIX 插队**(§29.144)。
**队列**:INTERSECT-FIX(§29.144)→ANSWERFACE-1→COMPLETE-2→SPANVIS-1。

### §29.143.1 hotfix:eval 写模式 provider-args bash-3.2 空数组潜伏形(2026-07-19)
§29.143 净室首跑 bash 合同 FAIL 揪出:eval/run.sh 写模式 6 个调用站用裸 `"${CODRAX_PROVIDER_ARGS[@]}"`(读模式 4 站早用安全惯用形)——bash 3.2(macOS /bin/bash)空数组在 set -u 下 unbound;providers.yaml 在场时数组恒非空故 live 恒不触,净室 archive 无 secret 文件→空数组→run.sh 中断于 verdict 前。修=6 站统一 `${arr[@]+"${arr[@]}"}` 惯用形;live+净室双 ok。教训:①bash 合同必须入净室验证清单(§29.141/142 只跑了 go 面,此形潜伏两批);②数组展开新站必用安全形(shell 家风)。

## §29.144 INTERSECT-FIX 收账(2026-07-19;h11 day-one 回归全链闭环;合并复核 SHIP)
**全链**:h11 新案首战抓获(§29.143)→二分归因(消失于 CHAIN-BUDGET:io_latency 单段行被家族折叠吸并后 familyMemberIntervals 遭 §29.132 偏离④ 闭集按 hull 理由误伤;HEAD 决定性证据=interval_union 真段 union==发布值到 µs+物理交集 0.230ms)→**修=rootCauseItemDirectionSupport 新 basis 臂 family_member_segment_intervals**(准入三合取全精确:len>0∧foldCaliber∈{interval_union,sum_disjoint}∧|union−ImpactMs|≤tol µs 恒等 all-or-nothing)→h11 冒烟 PASS(「同段重叠 0.114ms」当届活体形)。种群出局静默臂补软披露 banner(六 commit 无人察觉教训;纯软面零硬门,两板 byte-identical 实证);h11 re-base 活体值形=回归 pin 常驻。
**合并复核 SHIP(0 P1/P2)**:两道对抗题数学裁定——①hull 族恰巧过恒等门=可构造但恰是无害形(超集性 I⊇S+恒等 ⟹ hull≡真段至多差 1µs 零测集=准入语义正确;偏离④「hull 不冒充段」实质保护成立);②sum_disjoint 臂更强成立(ImpactMs 独立铸出=恒等门真外部校验)。M1 剥臂→5 面红(回归复现)/M2 剥恒等→hull 混入红/M3 闭集混入拦截/M4 tol ±1µs 保守边界。四板 A/B=RANK 行零 diff,仅 ∩ 披露面新增。P3 备案=第三铸点注释(主会话随收账补)/interval_union 恒等部分自指(fold 期逐成员校验承重,批前约定)/61839 同 chip 双条显示疣→DISPLAY-HYG 候选。
**队列**:ANSWERFACE-1(§29.145)→COMPLETE-2→SPANVIS-1。

## §29.145 ANSWERFACE-1 收账(2026-07-19;§29.140 答案面 G4-G9+叙事消费+§29.144 P3-3 七件;合并复核 SHIP)
**件2 G6=重大前提修正**:审计原判「a3 覆盖缺口 1.954ms 被四态守恒静默折进 running」被推翻——detached worktree 旧引擎复放+复核官独立 python 逐行复算证实:52.478=闭合事件对 51.462+窗尾开区间 flush 1.016(逐 µs 恒等,全事件背书);1.954=**probe 时代 identity bug 工件**(旧引擎把 tid 61847 的 sched_wakeup_new/切入误归 59566,557907−555953=恰 1.954=陈腐 PROFILE §1.7 差值)。实现=typed 四字段(HeadCarry/TailOpen Ms+State,精确判据 recovered 前缀/EndLine==0 flush)+四态账行「,含未覆盖段 X 折入」词面单点+拒标不造数守卫+图例;legacy115 零 open 区间零折入(窗外关闭事件背书=语义正确)。a3 case 注释陈腐机制句主会话随收账勘正;**PROFILE §1.7 [probe] 行陈腐=ground-truth 文档裁定项留用户**。
**其余六件**:件1 G4 教学(BLOCKED-REASON CENSUS CONSUMPTION 条款,纯软);件3 G7=HEADLINE 扩(copula 绑定锚「根因是/为/在于」排≠标签+未发布反转因并置臂(封闭词族∧typed 反转席缺席∧board#1 可解),与既有改写层互补无双报,反转席在场静默);件4 G8=根修(SELF-TWIN fold 漏挂 chain-universe lane→同 matcher 扩,合并单席 [E+E],偏离「补互指→合并」有审计候选背书);件5 G9=FIN-BIND 口径分离第二方向句(段值永不冒充全窗总量;prose 意图=嘈声不加硬臂);件6 新词面叙事消费教学四车道(fix_direction/overlaps/member 族/凭证三态,零硬门);件7 同 chip 键去重(异值双留)。教学件 ATOMIC 7 红线复核独立勾稽全过。
**冒烟**:multicausal PASS(G9 病灶直接反转=正文明写 target_window_states 分区;census 口径消费在场);c2 诚实红(event_search-only 触顶不取 census=**G4 引擎车道候选立案**:blocked_reason↔D 段 typed 配对,超本批范围)。
**复核 SHIP(0 P1/P2,M1-M7 7/7 红)**;P3 四备案=a3 注释(已勘)/件4 fold 顺序依赖(逆序诚实回疣)/疑问尾「根因为什么」形观察/background 反转行不静默(词面字面为真)。**裁定项新增**:PROFILE §1.7 probe 行 re-base(ground-truth 文档,用户裁)。
**队列**:COMPLETE-2(§29.146)→SPANVIS-1(§29.147)。

## §29.146 COMPLETE-2 收账(2026-07-19;§29.140 GAP-4 mixed-origin 烧轮;合并复核 SHIP-WITH-FIXES+主会话两 pin 收编)
**修**:dropWaivedCurrentSourceOriginDebt post-filter——两臂 typed waiver(①ExcludesCurrentSource=analyzer 显式排除位(mode=exclude∧explicit_user_exclusion∧verbatim quote,与 emit 侧 bypass 同谓词同 reason 串)/②ZeroCurrentSourceRepo run-entry census)成立时**只**剔 current_source 债务,其它 lane 原样=纯放宽零新增拦截;两 auto-complete 门(explore window+reconcile node)共享一函数齐修;waiver INFO log。「downgradable 独立成臂」被否=复核 MUT-C 实证既有负臂+新 pin 双红(混源真需求重派保留)。ratchet split=accepted_closure_origin_debt.go 219 行(复核字节级验证纯搬运零语义)+收紧 9135/240。
**病链四环(witness 全证)**:trace 1.9MB 超帽无 bundle→排除 carve 被旁路→analyzer 误标 prose 维度 current_key_code+词面兜底铸锚(嘈声→硬门)→suppressor 提前 return→软债硬重派 ×2(3 emit/21 tq/318s,终局照样 caveat=等价结局烧轮)。**上游三件立独立候选**(lane 决策序 exclusion 后置/shouldIncludeCurrentSourceOrigin carve 被 bundle 缺席旁路/prose 兜底铸锚——各动权威面需自带 sweep,复核背书留候选)。
**验收(健形抽样对照,waiver 未实弹=诚实标注)**:witness 同参 3 run=1 emit/1 dispatch/0 blocks/tq 3-4/116-178s(修前 3 emit/21 tq/318s),case EXPECT 全过答案不降;waiver 判定臂由 witness-verbatim RequestModel 走产线函数确定性证明(复核对 log emit params 逐字段核验保真);修后 runs waiver log 全 0=复放全抽健形(analyzer 分类抽样性,未改题参)。
**复核 SHIP-WITH-FIXES(六突变全套)**:F-1 P2=「只剔 current_source」承重墙负臂(MUT-B 剔全 lane 三包全绿存活)→**主会话收编复核官探针**(TestAcceptedClosureWaiverKeepsRuntimeArtifactDebt=runtime 债在场+排除位→债保留拒 auto-complete);F-2 P3=reconcile 分叉不设防→**主会话补源级 census pin**(unfiltered helper 恰一产线调用点且在共享过滤入口内);F-4=落账话术勘正(「3→1」=健形抽样对照;123952 run=代表窗 regex 正交轴 FAIL 不计全绿)。
**队列**:SPANVIS-1(§29.147,末批)。

## §29.148 DATAGATE-2 收账(2026-07-19;用户急件=合并覆盖合同投毒;§29.147 SPANVIS-1 在飞预留,本节先行落账)
**病根(witness=.codrax/data-audit/20260719-094051-571898-358-terminal.json,replay data_multifile_reference_projection run-2)**:runner/model 例行注册 ID 与源文件名字面相同的 extract/reference 工件("targets.csv"/"labels.csv"/"observations.csv",112/235/196 处撞名),dataTaskWorkflowGeneratedArtifactPathSet 把 alias+artifact_path 无差别入集,strip 谓词(dataTaskCoverageMaterialIsGeneratedArtifact)只认集合命中→合并合同 required_materials **全塌缩为空**,workflow 级完成门失去从未被消费的 targets.csv 必需料义务(DATAGATE-1 E-1 已在 dataTaskReferencePathIsWorkflowMaterial 局部绕道过同一毒集=先例形)。
**修(用户指定修形=盘上存在优先凭证,先例同形)**:strip 谓词加前置臂——material.Path/ID 归一后**仓内相对路径且盘上实存**⇒永不作 generated 剔除(精确信号:os.Stat 单布尔);主会话复核自抓一真洞收紧=**凭证仅限相对路径**(dataTaskSourceFileExists 对绝对路径直接 stat 绕过 repoRoot 围栏,已物化生成工件的 blob 绝对路径会被误保护→abs 一律不授信,strip 现状保留)。repoRoot 全链穿线=~90 函数签名机械级联(guard/fallback/staging/completion/prompt 面全收,编译器引导 18 轮;evaluator/patch 五接口补齐 userLine,repoRoot 家风)。
**伴生防线**:可选能力接口(evaluator/patch/continuation/repair 十面)全为运行时类型断言匹配——签名漂移=实现者**静默脱接**(本批 stub mock 实锤:编译绿、eval=0 运行红)→llmDataTaskPlanner 十面编译期 var _ 断言单点钉死。
**pin(TestDataTaskWorkflowCoverageContractSourceNamedArtifactAliasDoesNotStripRealSourceMaterial)**:正臂=三源料+同名工件→合并合同全保留;负臂①=非盘上中间产物照剔;负臂②=物化绝对路径(盘上实存、仓外)照剔。双突变验证:去盘上臂→required=[] 塌缩形复现(与 witness 同形);去 abs 围栏→物化臂红。
**验收**:internal/repl 全套+dataquery/dataworkflow/cmd 邻包 count=1 全绿;测试呼叫点 240+ 处 ""(CWD 回退=旧行为保持,包目录无数据料文件)。委托默认待追认两点:①abs 不授信收紧;②接口静态断言。net:毒化关死,真源料义务回到完成门;残口=dataTaskReferencePathIsWorkflowMaterial 同 abs 洞(E-1 先例自带,射程=授予 standing 非剥夺义务,候选备案不随批)。

## §29.147 SPANVIS-1 收账(2026-07-19;◈ 业务 span 提及面=LOCKSPAN-SEAT 修向 C 定案落地;旗舰双复核双 SHIP-WITH-FIXES+返工轮全销)
**定形(用户原则 §29.131 verbatim 兑现)**:纯 advisory 提及面,零席位零序数零排序参与——树 fence 尾 ◈ 块(头行「◈ 业务span提示(不参与根因排序,业务视角):」+每族「· 主体 span名 单次最大X/N次 合计Y 行a..b 凭证:闭集词」)+◎ 旁栏脚注「◈ 业务优化线索(不占序数,业务视角)」,词面单点。准入=typed 链上(self=chain.Target/chain_member=wakeupChainThreadSet/host=R3 边凭证**整段边前**)∧族合计≥max(0.1ms,1%×窗)双分量地板(六窗名谱实测证绝对单值地板不可行)∧≥1 帽下隐藏段(偏离备案,追认项);帽3+「另有 N 个 span 族(≥显著地板)未列出」诚实截断。库存基=computeTraceMarksWithInventory 双返(bounded 席位机器 byte-identical;unknownEmitter/unresolvedPairing/durationFailure 三臂对全量库存同步 fail-closed wipe);wire=8 note key(business_span_*)R2' 全套+16 臂严格解析;双杠杆三层(typed 三数+阅读参考「次数多而单次小→业务流程/调用次数方向;单次长→单次运行时长方向」+prose lexicon (e) 条)。LOCKSPAN 现状 pin ①②③ 原样绿+④ 演化臂=修向 C 收账,**checklist R-20-a 标 RESOLVED**。
**双复核**:对抗官 SHIP-WITH-FIXES(P1-1 host-edge 跨边 span 整段入账=边后泄入边前记账,实证 40ms 形;P1-2 unresolvedPairing 库存 wipe 无测试牙=突变幸存实证;P2-1 ◎ pin 空转/P2-3 多窗累积/P3 ClaimKey);冷读官 SHIP-WITH-FIXES(四板重生成 byte-identical 证 witness 非陈腐;F2 ◈ 块无不可相加句(275.8ms>窗 233.19 嵌套实锤)/F3「未入统计」失实/F4 图例别名/F5 EN segment 撞词/F7 前既存计数当量行佩 ms+% 假单位=独立立案)。**返工轮全销**:P1-1=host 成员门整段边前(StartTs>=boundary||EndTs>boundary 拒,禁二分铸值)+跨边/恰界双负臂;P1-2=三测改走 WithInventory 断言库存空+durationFailure 臂新 pin;P2-1 pin 迁 elimBoardProjection 形零 SKIP 零 early-return;F2 单点「◈ 各族合计间不可相加(区间可重叠/嵌套)」zh/EN;F3「未列出」;F4/F5/P3 落;P2-3 落盘候选(跨窗 omitted 无诚实标量,anchor-window 准入设计=TargetStateAccount F-2 先例形)。
**件F ◎ 头部多行(用户裁定 2026-07-19 verbatim)**:「这一行太长了,换行看起来也不好看,可以考虑多行(佩戴同样的图标不缩进 是否更好看一些?),而不是堆积在一行,不好看」(witness=20260719-161405.439-17874.md:146)。落地=runtimeTraceProjElimHead 承诺段三条刻意短行每行同佩块 glyph 零缩进(①链上块先·节=修复方向 ②方向间收益不可相加·节内值降序 ③零序数·零佩戴·定位走[E#]·满格=本区TOP1),四臂(zh/EN×chain/adjacent)同规,glyph 自 channel-word 单点首 rune 派生;旧一行 pin 三处+preview fixture 刻意更新;新 pin TestElimHeadPromisesMultiLineGlyphWorn。
**突变**:M1-M25=24 RED+M11 等价备案+M15 首滚幸存(探针失牙真发现,修后 M15b RED);cp 串行 cmp 逐字节全复原。witness 四板(donghu L1 锁/tieba 锁/donghu 全窗/tieba 负板 0◈ 字节)+手算恒等(0.295+0.008=0.303/0.257+0.011=0.268 µs 恒等)。**偏离/追认项**:①单次最大(避 SPANTOP「单段」承诺面撞词);②第三门「≥1 帽下隐藏段」(冷读官 F1 场景=top-8 内超长 span 不提及,ruling letter 张力,追认);③行号形无 E#;④⑤⑥凭证词/优先级备案;⑦⛓ 佩标与席行同 glyph 混淆疑虑(按用户明示偏好执行)。**独立立案**:F7 计数当量行佩 ms+% 假单位(tieba 负板 L147/L383,前既存);P2-3 跨窗提及累积候选。三问答卷全勾稽(库存成员窗内投影 verbatim/闭集口径词/Σ==族成员 Σ µs pin)。

## §29.149 FREQDIR-1 立案(2026-07-19;用户问询=报告 20260719-124316.854-95946 答案正文缺 #1 方向「频率与热治理 58.320ms」;三路诊断定谳)
**病根=输入面缺口非模型读障**:fix_direction 分类词为显示层专属车道——引擎盖章(stampRootCauseFixDirections)+note 铸造(trace_query.go:7348)后唯一消费者=模型发射**之后**渲染的确定性投影(trace_causal_projection.go:3505);全 run 日志 grep fix_direction=0。四环:①LLM 可见权威板行(trace_board_summary.go:writeRow)只序列化裸状态词 running,零方向语义;②席位构成 named-fact 门 inversion==true(trace_wait_evidence_summary.go:453)——❷❸#8 反转席拿到热限压/缺口事实,#1 非反转席(58.320ms 主人)恰好没有,供给本质只剩英文 summary= 一句;③Known Facts 灌入 explorer 四方向竞争叙事(「修复方向一:优先级反转(最高优先)」);④模型 think 实录=逐行读板列出 #1 58.320 后写"Priority inversion is the dominant factor, with .ugc.aweme.lite-17267 at 58.320ms"=吸收实锤;教学 defaults.go:822「按 fix_direction 作答」指向不可见词面=不可执行,且无完备性义务(四条既有指令约束「怎么写」使漏整个方向成最便宜合规读法)。**门面无失灵**:prose-vs-board 全线附注披露(§29.42.4/§29.104.13 既裁);「最高优先」「为提升空间最大的方向」在 HEADLINE/copula/superlative 封闭锚词族外(宁漏勿假指设计边界);supply-adequacy 臂只抓假充足宣称;唯一结构盲区=**零漏向臂**(无任何 lane 审计 prose 漏报板 #1 方向)。
**修向五步(委托默认排批,待追认)**:①payload=板行追加 typed 方向词(闭集 verbatim 零铸造,未盖章席不合成)+可选 explorer 行面同步;②payload=席位构成 named-fact 扩至非反转 SupplyFoldDeficit 链上席(口径词嵌串防加和);③teaching=方向完备性义务+折算席以自身口径入清单不并总(依赖①);④appendix-only 漏向披露臂(修复方向枚举头封闭族∧板#1方向词缺席→一条附注,永不硬拦);⑤明确不建硬门(边界落档)。排队=SPANVIS-1 后首批(战场 internal/context+skill 已解锁)。随查旁获:P3-1「按归因幅度排列」跨块漏检观察备案。

### §29.149.1 FREQDIR-1 交付+件5 边界落档(2026-07-19;委托默认实施,待追认;旗舰双复核待排)
五件全落地。**件1** 板行方向词:词表单点迁移 tracefence.FixDirectionWord(Table ⑦,tool 侧 runtimeTraceProjFixDirectionWord 改委托,字节不变)→ trace_board_summary 席行 `· 修向=<词>`(zh 显示默认面,循 INV-SUPPLY 件② 先例)+preamble 车道句(同修向=一条修复车道,车道最大可消=该方向最大席值,未盖章席永不合成);95946 #1 行形 witness pin 逐字。**件2** 供给折算 named-fact 第二臂(trace_wait_evidence_summary):链上 rank 席∧**非反转(!inversion)**∧foldRan∧缺口>0(rank+subject 渲染前判重防畸形复播双叙事)→「供给折算(❶ 主体 类型): 供给折算缺口 X ms(运行频点非最高[,热限压 Y GHz])——独立折算口径,不与墙钟(全额)值相加、不计入四态合计」;口径词面对本臂种群=**非反转席恒真**(含 eff==deficit 形,E1)——反转席由 !inversion 整族出圈(返工 P1 勘正:其缺口=eff 的 running 计入分量,同源同值,§29.88.12 R5 已裁退「独立口径」词面;反转席供给叙事归席位构成臂独享,非主导/缺 gated-split 反转席双臂静默=诚实结果);**不用**立案稿建议的「不计入有效归因」;禁伪造 gated 拆分,零缺口/无 fold/邻近/无席/反转席全静默 pin。**件3** defaults.go (a) 条款扩:完备句(枚举 MUST 覆盖每个已发布方向值)与口径句(折算席以自身口径独立成条、永不并入跨方向和、不自行跨口径加冕,榜序发言)同对成文;ATOMIC 7 过;双臂 substring pin。**件4** HEADLINE-ELIM arm C(appendix-only):精确侧=链上#1 席佩 registry 可解方向 token;嘈声侧=模型单元含封闭头族(修复方向/提升空间/优化方向)∧同单元≥2 枚举行;方向词(zh/EN 面或裸 token)在场任意模型块→静默;最大可消=同板同方向席 eff 纯 MAX(◎ 节头公式);恰一条「typed 事实: 修向 X · 最大可消 Y ms(该方向最大席值,…)——正文未出现该方向词」(事实形,不预设清单存在——返工 P2-2a)。**件5 边界(既裁,防再议)**:方向完备/prose-板方向一致**永不硬拦**——「正文是否在枚举方向」为嘈声抽取,§29.42.4+§29.104.13 答案面权属模型;arm C 及未来同族永远 appendix-only(边界注记刻在 proseHeadlineDirectionOmissionFinding 头注)。**Rider(未做,备案)**:①explorer 行面 fix_direction token(trace_query.go:3829)未同步——已审:无确定性 parser 读 rank 行面(regex 解析仅 blocked_reason 行),风险仅在巨型 fmt 行+其字节形测试 pin 需专门 sweep;单独小批做(R2' 不涉,token=裸 wire 值不涉词表);②「zh/EN 按板语言」未实现语言切换:板 preamble 恒英文、词面循席位构成先例恒 zh 单面(EN answer run 的报告面词为 frequency & thermal,板词仍 频率与热治理)——如需双面或跟 answer 语言,须先裁词面单点如何跨包共享语言判定(requestedAnswerDocumentLanguage 在禁触 emit_*.go 内)。
**返工轮(双复核 冷读 SHIP+对抗 SHIP-WITH-FIXES=1P1+2P2+P3 补牙,同日全落地)**:P1=件2 谓词加 !inversion(见上勘正;对抗官探针形 eff=20=反转等待15+running折算5 与缺 gated-split 反转形均入静默 pin);P2-1=车道最大值语句通道限定——板 preamble「LARGEST **on-chain** seat value…adjacent rows are conditional upper bounds and never join the lane maximum」+件3 教学 duty 句「ON-CHAIN seated causes(邻近=条件上界不设方向最大)」,防模型算出超过 ◎ 节头(◎ 只折链上成员)的车道最大,双 pin 更新;P2-2=件4 三假阳硬化(a 附注句去预设=「正文未出现该方向词」;b 枚举行=水平线 `^-{2,}$` 出圈+bullet 符号后必空白;c 语义同指静默臂=枚举单元含该方向已发布 eff 数串(如 58.320)→静默,实质覆盖不指控),各配触发/静默双向 pin;P3-1=突变臂补齐(≥2→≥1 单 bullet fixture 红;max→min 同方向双席 fixture(58.320+12.100)红);P3-3=registry↔词表同步 pin(tracefence/fix_direction_word_test.go,test-only import tracequery 无生产环;registry 加第 7 方向而 Table ⑦ 缺→红,fail-open 臂同 pin)。**备案不动码**:P3-2 三种群边缘分歧(件4 maxEff 种群=prose-check 行集 vs ◎ 渲染种群 vs 板 feed 种群,聚合折叠边缘可分歧——件4 句自带「该方向最大席值」自定义基准,分歧只影响与 ◎ 面的字节一致性非真值,待观察);冷读 P3-1(preamble 旧措辞观察)按裁定备案,P2-1 重写已覆盖该句;P3-4(EN 双面候选)并入 rider ②。

## §29.150 用户裁定集(2026-07-19;待裁 dossier 七件逐裁+整批追认;各批携此节开工)
**①帽亡真值行(R-19-a)裁定 verbatim**:「一定要确保 优先保证 折算后提升最大的 链上的 TOP 内容不丢失,非链上的 和背景 次之,尽量减少噪音影响。请按最优方案推荐实施」→ 立 **CAPFIX-1** 批:帽存活跨道优先序=链上有值席按发布 eff desc 恒先(TOP 链上席在任何非链上/背景/零值行占位时永不帽亡)→非链上(◇)→背景(▒),零值恒末;残余帽亡走 R-2 式值披露 caveat 扩展(值+主体入句);排序语义零动(帽=存活选择非重排);旗舰双复核(板面涟漪)。
**②17267 两把尺(R-19-b)**:「按推荐来」=补「三席分账两把尺」跨行披露句小批(**RULER2-1**;新承诺面句族+幂等+pin,禁宣称跨尺合计=M3)。
**③µs 级互指地板(R-15-e)裁定 verbatim**:「极小交集 算作噪音,对用户影响极小 应该判为噪音是否更好?这样为后续减少注意力,和根因修复排序聚合更能确认方向。请按这个目标评估最优。」→ 立 **INTERFLOOR-1**:按此目标取最优=**相对形地板落地**(overlap 低于两席 eff 相对阈→降入既有 cross_direction_overlap_undisclosed typed 记号道,句面/∩ chip 不发;阈=导出常量按 witness 数据论证,**禁绝对 ms 常数**既裁红线;值通道零动);h11 案双臂 re-base(大交集正臂+降道负臂)。
**④partial 倒装拆分**:「请按推荐的 开 LEVELMERGE 式披露拆分批」→ 立 **PARTSPLIT-1**:gated 复合席边前份以披露句/分账行入 ⛓ 可见性(LEVELMERGE 披露拆分范式=拆分测度披露、发布权威整席零动、R4 整席不拆底线保持);目标收 tieba 23088 边前 13.939+donghu2955 9.618;23088 拒转 pin 演化为披露形;旗舰双复核。
**⑤3d 锁二跳**:「按推荐的来」=维持 depth-1 不动;候选留档,启用条件=客户交付真多级锁链 trace。
**⑥E-2**:「按推荐的来:开提案车道来」→ 立 **E2PROP-1**:validator 完整 assemble_answer 参数入 fallback plan 候选(模型仍可拒),合成参数过继 C1/C3 材料凭证同门受理(generated 别名拒),**禁绕 planner 直执**;witness 形(17,0,5 算对被扣发)确定性复证。
**⑦PROFILE §1.7**:「按推荐的来 推荐:批准 re-base」→ 立 **PROFREBASE-1**:当前引擎重跑探针流程更新 §1.7 [probe] 陈腐行与 1.954 句(机制注=§29.145 identity-bug 勘正),a3 band 预期零改需验证,探针用毕即删。
**整批追认**:「其它的也都按推荐的来。」= ratify_checklist R-1..R-20 全部子决策按建议处置追认落账(R-9/R-18-a/b 强烈维持确认;4 待裁=①②③⑬ 已由本节裁定;R-20-a 已 RESOLVED 修向 C);⑧15µs skew=修订注释量测基常数不动;⑨S4 limits=随 CLUSTER-FIX-2 顺带补同构 caveat;⑩C8=按区分制成文(prose 全角/fence 半角)消同句混用+pin;⑪C4=统一带树连接符形+pin;⑫R-10=按 M18 模板收敛(独立小批,排显示批后);R-21+ 候选族(3c basis sibling/3b 指纹闭集(3b 开批先裁)/编队排队/DATAGATE-2 abs 不授信+接口静态断言/FREQDIR-1 与 UPSTREAM-3 委托点)一并追认。
**队列(裁定后重排,trace 前置)**:在飞 FREQDIR-1+UPSTREAM-3 双复核;并行开 CAPFIX-1(①)/E2PROP-1(⑥)/PROFREBASE-1(⑦,均 worktree 隔离);后续序=PARTSPLIT-1(④)→INTERFLOOR-1(③)→RULER2-1(②)→G4→CLUSTER-FIX-2(携⑨)→显示小批(⑩⑪)→R-10 wire 批。

## §29.151 UPSTREAM-3 收账(2026-07-19;§29.146 上游三件=trace 附件读流烧轮根修;worktree 隔离实施;旗舰双复核=对抗 SHIP+冷读 SHIP 零阻断)
**件1 exclusion 车道先于债务铸造**:acceptedClosureRequiredOriginLanesBeforeDebtMint 尾接 withholdWaivedCurrentSourceOriginLaneBeforeDebtMint(与 post-filter 同谓词同 reason 串单值源);排除位成立时 current_source 义务不铸(原=铸后剔);post-filter 保留为不变量兜底+hit 可观测 log(常态结构性 0)。mixed 门保证(hasCurrent∧hasNonSource)使空集翻转结构性不可能(对抗官证);铸后剔≡不铸等价性双路径 pin(formatAnswerEvidenceOriginsForLog 序敏比较)。
**件2 carve 读 Run-entry preflight 载体**:CompileAnswerIntentContractWithPreflight 新入口(零值委托=字节恒等 pin);shouldIncludeCurrentSourceOrigin 排除 carve 增臂 runtimeArtifactPreflightCarrierWithoutBundle=(PerfTrace==nil∧HasTraceArtifact)∨(LogTriage==nil∧HasLogArtifact)——LENSBURN-A1 同构、TOOLWIN 同载体单值源;硬债/dispatch 链三点转接,20 软面留盲(逐点分类,对抗官独立复枚零漏硬消费者)。
**件3 prose 兜底铸锚灭**:current_key_code+function_or_purpose 两维词面臂改 dimensionHasPreciseCurrentSourceAnchor(权威分类器同谓词复用,单点);prose 谓词 dimensionHasCurrentSourceAnchor 删除;CurrentSourceExplanationProfile quote 臂蓄意保留 prose 容忍(typed 载体,GAP-4 混合负臂 pin 在岗)。
**复核**:冷读 SHIP(end-to-end 产线路径三链全证/零假 pin/P3×5=分裂合同面一致性债/tier1_floor 同族残口/死 helper/log 措辞/注释家风);对抗 SHIP(完成门权属纯放宽证明成立/等价性成立/4 突变全红;**P2 追认项=倒装 legacy pin:带锚显式排除的引用性边界形在混合请求且无 explanation profile 时,现交 trace-only 诚实降级答案(原强制源码分析)——方向合红线(词面不得硬门),行为翻转登记待追认**;P3-a 锚否定合取候选/P3-b 跨 kind 权威洞/P3-d current_version_check 同族幸存臂/P3-e godoc/P3-f log 噪)。**候选立案**:tier1_floor 裸维词面地板(冷读 P3-2)/死 helper requestModelHasRequiredCurrentKeyCodeDimension 清理/P3-a/P3-b/P3-d 三残口(件2/件3 同族收尾批候选)。ratchet 240→280 蓄意注记。门证=净室 make/83 包/bash 合同全 0。追认清单维护态新增:UPSTREAM-3 P2 pin 倒装+实施偏离 1-8。

## §29.152 E2PROP-1 收账(2026-07-19;§29.150⑥ 裁定落地=系统可提案不可代答;旗舰双复核=冷读 SHIP+对抗 SHIP-WITH-FIXES,返工全销)
**提案车道**:planner 退化(typed no_tool_call/no_plan_shape 族+单次有界 reprompt 二连空)∧ continuation 模型已咨询并失败 ∧ 确定性枚举空 → validator 修复提示的完整 assemble_answer 参数(活重算接地 guard+verbatim errText tie+四 typed 字段)合成 fallback plan 候选(typed provenance Source=validator_proposal),入既有候选车道零特权:同 protectPlan/准备/执行/validator 链/完成门/终答选择;拒绝全具名 WARNING 零静默;无完整参数/候选被拒=诚实 typed failed 原样。witness 复放 pin=17,0,5 算对被扣发形→提案→采纳→正确出厂(§29.142 病形关死)。
**返工全销**:P2=共享凭证门 abs 围栏(dataTaskReferencePathIsWorkflowMaterial 盘上臂加 !filepath.IsAbs,镜像 DATAGATE-2 先例=**§29.148 备案残口就此关账**;双臂 pin;既有消费者零 pin 依赖 abs 授信);P3-a 咨询后才提案(no_plan_shape 预检失败不提案,pin continueCalls==0);P3-b 终答选择抽 dataTaskCompletionAnswerSelection 单点(DL-C 单一权威,双点位 census pin);P3-c census 升传递闭包遍历(helper 间接绕行红,反例 trace 实录);P3-d REPL 侧 provenance pin;注释两处勘正。突变 M1-M11 全杀(cp 串行 cmp 复原)。
**复核 P3 备案**:zh 面板 raw label(先例维持)/Field 显示面 delta(DISPLAY-HYG 候选)/abs 不入 alias 集仍 fail-open 形(与门语义一致,落档);委托默认两点=P3-b helper 签名取 (records,current,result) 保字节等价、P3-a 依赖 typed/plain 错误不对称(pin+M8 臂看守)。**教训入账:多写者树上禁做合流操作——本批首次捡拣在 FREQDIR-1 在飞主树上冲突污染,改独立 worktree 后零冲突,合流一律独立树。**

## §29.153 FREQDIR-1 收账(2026-07-19;§29.149 立案五件全落=95946 缺席病根修;旗舰双复核=冷读 SHIP+对抗 SHIP-WITH-FIXES,返工全销)
**五件**:件1 板行方向词(writeRow 读 TraceNoteKeyFixDirection 佩「· 修向=X」+preamble 车道规则句,未盖章席零合成;词表迁 tracefence Table ⑦ 单点、tool 纯委托字节恒等);件2 named-fact 扩臂(非反转链上席 SupplyFoldDeficit>0 发「供给折算缺口 X ms(运行频点非最高,热限压 …)——独立折算口径,不与墙钟(全额)值相加、不计入四态合计;连口径词与数值整体照抄,勿推导」;spec 建议词面因 E1 席 eff==缺口 形会撒谎改取全席形恒真面=委托默认);件3 教学完备性义务+折算席自口径入清单不并总+禁自加冕(ATOMIC 7 全过);件4 附注漏向臂(封闭头族∧枚举形∧方向词全文缺席→恰一条 info 附注,§29.104.13 永不硬拦);件5 不建硬门边界双落档。读者测验(冷读官):四行为全过=方向入清单/带口径述值/禁并总/禁自加冕。
**返工全销**:P1=件2 谓词补 !inversion(反转席缺口=eff 计入分量「同源同值」,§29.88.12 R5 已裁退「独立口径」词面于反转行;漏入臂=复铸谎面,对抗官探针实锤 eff=20=反转15+折算5 形;修后双车道静默 pin+账本恒真句勘正);P2-1=车道最大值句限定 on-chain(邻近席=条件上界永不设车道最大;板 preamble+件3 duty 双改)  ;P2-2=附注臂三硬化(措辞去预设「正文未出现该方向词」/水平线不计枚举+bullet 后必空白/枚举单元含同方向已发布 eff 数串→语义同指静默);P3 补牙=≥2→≥1 与 max→min 两突变臂+registry↔词表同步 pin(第 7 方向缺词表→红)。
**备案**:三种群边缘分歧(件4 自带「该方向最大席值」基准句,分歧仅涉与 ◎ 字节一致非真值)/preamble 旧措辞(被 P2-1 覆盖)/EN 双面并入 rider ②(跨包语言决策裁定)/rider ①(explorer 行面 token 缓,零确定性 parser 消费实证)。LLM 全链复放留后续 eval 窗随行验证。

## §29.154 CAPFIX-1 收账(2026-07-19;§29.150① 用户裁定落地=帽存活跨道优先序;旗舰双复核=冷读 SHIP+对抗 SHIP-WITH-FIXES,返工全销)
**件1 跨道存活键**:selectRootCauseRankCapSurvivors(道优先 ⛓→◇→▒ 经 rootCauseOrdinalChannel 单值源+发布 eff desc+板位)取代 truncateRootCauseRankItemsStrict(墓碑+EVOLUTION);存活选择非重排(原板序重发射,序数铸造零动);**链上有值席在任何非链上/背景/零值行占位时永不帽亡=用户裁定不变量 pin**。分析性等价:链感知板 relevance 闭集使新键≡旧排序前缀(四旗舰板字节恒等,对抗官独立重渲复证);真行为翻转=chainless-sorted 板(XERR1 微板 §12.3 锁席 0.066 从池#14 入榜 ⛓=裁定精确目标形,e2e 见证)。**件2 残余帽亡带值披露**:「另有 N 行未入发布面(链上 a/邻近 b/背景 c),最大 <metric> <主体> <eff>ms(道)[;链上最大 …][;另 N 行自身侧道已发布]」zh/EN 单点;四板计数手核=不相交并集恰等。**件3 零值恒末** pin。**裁定意图勘验**:对抗官逐行再判道——§29.136 名单(OS_FFRT 19.984/76.800 族)今为 typed ▒ 背景(ONCHAIN-3c 再判道,同 rootCauseOrdinalChannel 单值源无二源漂移),帽亡=「背景次之」合规;真链上残亡(RenderThread 0.378/keva-3 1.354)仅败于全部 12 席更高 eff 链上席=非 TOP,带值点名。
**返工全销**:P2=carve 牙(mutant F 曾全套绿=谎计数无防;新合成 pin+真 trace verbatim「52/on_chain=2」双牙,复验三 pin 红);P3-1 gofmt 顺清;P3-2 fixture 再判形(chainFiller+背景填充退休形伴 pin);P3-3 平手取板序 pin(>→>= 翻转见证);冷读 P3-1 zh 补 metric 词对齐 EN;冷读 P3-2 自身侧道句内自解释(空亡也发,X→Y 算术恒自洽);P3-4/双最大域/prefix 射程=备案。**指纹**:limit 在闭集,存活键=引擎逻辑非 Query 键,无异板事件(论证落档)。**过程教训(本节汇流)**:cd 残留使捡拣误跑批 worktree(自捡自身冲突)——git 汇流命令一律显式 `-C <绝对路径>`,禁依赖 shell cwd。

## §29.155 CALSIDE-1 收账(2026-07-19;用户显示裁定=◎ 旁栏关键语义缺席;合并复核 SHIP 零 P0/P1)
**用户裁定 verbatim**:「"窗内可消除量总览" 里面 如下两条,看起来有些关键信息没展示,例如 "页缓存抖动","D-state / 不可中断等待" 等,这些信息相对关键,是否可以优雅的在这里展示,不然用户一眼看到数值,不知道是啥意思。」(witness=17874 报告 :164-165)。
**件1 ◎ 旁栏语义词**:⌗ 口径旁栏 footnote 行补席行同源类别词(runtimeTraceProjElimClassWord 同一 composer,零第二词表):「ThreadPoolForeg-60555 · 页缓存抖动 · 计数当量7.200(非墙钟)·⌗口径旁栏 [E32]」;类别缺席不合成(负臂 pin)。◎ 全 12 族 footnote sweep 逐族结论(1 补/6 本就有/4 不适用/◈ 不涉)。**件2 F7 假单位修**(§29.147 立案关账):计数当量树行值列去 ms 佩(「计数当量7.200(非墙钟)」),非墙钟两族(计数当量∪clamped 计数和族∪综合评分,typed 谓词)一律不佩窗 %、不画 bar(跨单位假象灭);飞行中新抓同族洞=无窗板 fallback 标尺可引计数当量为「最大X.XXXms」→ 排除臂同修(M7 突变红)。**件3 图例**:⌗ 词条补双承诺(类别词同源/非墙钟不佩 bar+%)zh/EN,双向 sweep 绿。
**复核 SHIP**:四板 diff 全属 ⌗ 族(donghu_L1 零 diff);复核官独立四突变全红(假 % 复现形逐字捕获);对齐量测=post 比 pre 更齐(81/85/86 vs 81/91/108)。**P2 立案跟进**:全非墙钟无窗板 fail-open 标尺句仍印假 ms+bar-less「满格=」宣称(前既存同严重度,F7 残形二号);P3 备案=突变日志尾形/witness 缺图例面/EN 折行孤立 ⌗/E# 列位既有。偏离 7 件委托默认(typed 谓词扩域/字节 pin 演化随 wrap/图例单点/E29-E30 行2 语序既有等)。

## §29.156 PARTSPLIT-1 收账(2026-07-19;§29.150④ 用户裁定落地=gated 复合席边前份 LEVELMERGE 式披露拆分;旗舰双复核=对抗 SHIP+冷读 SHIP-WITH-FIXES,主会话收编修单)
**定形兑现**:R4「整席不拆」底线零动摇(席值/车道/序数/Score 四板 strip-then-DeepEqual+复核官 Score 微扰突变证锋利)——R4-mirror 拒转臂演化为拒转+typed 记录(GatedCompositeEdge 四元组单点原子盖章);披露双面=席行行2 分账句+◎ 链区尾非席提及块(SPANVIS side-channel 同构,pool∪published 收割+SeatPublished 诚实位,cap-dead 席照披露);恒等基=**本席 runnable 账**(发布 eff=gated 0.796≠13.979,「席值恒等」会撒谎——委托默认,双复核官均判比 spec 更正确);准入全 typed(四元组∧via∈R3 闭集∧|X+Y−账|≤µs 级),缺则静默。wire=6 note key R2' 全套+all-or-nothing 解析+双 schema hash 重钉。**witness**:tieba flag 23088 边前 13.959+0.020==13.979(spec「13.939」为誊写误,产线值治理)+六 bonus 同族行(udk-irq×2/tppmgr/binder:227/2955-2614/trace-23088),七行 µs 恒等双复核官独立手算;四板 A/B=仅新披露行;§29.139 拒转 pin 演化(EVOLUTION,判「保意图且更强」)。M1-M6 全杀+复核官独立复突变。
**主会话收编修单**:P2 doc 注释错位复位(parser 插入撕裂 SPANVIS 注释);容差 0.002→0.003 带诚实算术注(引擎门 1µs 松+三次 %.3f 舍入 ≤1.5µs=最坏 2.5µs,原 2µs 会静默掉合法记录);parser 五 fail-closed 臂负臂电池(对抗官 P3-1 无牙实证→TestGatedCompositeEdgeShareParserFailClosedArms,身份臂突变红)。**备案**:无地板 0.007ms 行入 ◎(spec typed-only 既定,噪声裁定候选)/池序非值序(候选)/「R3/R4」规则号词面首上用户 fence(图例已定义,词面雷达观察)/convertStateSeat 陈旧四元组防御性清零候选/base 旧 dump 陈腐(base2 权威)。

## §29.157 INTERFLOOR-1 收账(2026-07-19;§29.150③ 用户裁定落地=跨方向互指 µs 级判噪相对地板;合并复核 SHIP)
**定形**:发射门 `overlap < 5%×min(两席发布 eff)` → 双侧对称降入既有 cross_direction_overlap_undisclosed typed 记号道(句面/∩ chip/◎ 脚注全静默=用户「减少注意力」目标;typed 记号+DEBUG advisory 行保审计;值/榜/席零动)。**RATIO=5% 导出常量**(委托默认待追认)带活体扫描论证:用户判噪形全在 1.09-4.99%(0.230ms 形恰 4.988% 距地板 0.55µs=界骑,收编 tripwire 注释宣示刻意);1-5% 带无直觉有意义活体形;12.33% 活体形(0.116/0.941)按裁定公式保发射=裁定池记项(降它需升阈或改式,裁定未授)。**禁绝对常数**=×1000 双侧缩放结构 pin(代数杀证:常数地板不可能同时过两臂)。h11 双臂重基:负臂 CJK 左界+ms 右界禁值形,正臂=确定性 advisory 行(上游种群灭→行灭→红=INTERSECT-REG 同级捕手,机制看护不弱化)。四板 A/B:三板字节恒等,17267 差=恰降道面;61839 差=仅图例地板句。M1-M5 全杀+复核官独立双突变。
**备案**:P3-1 降道对释放 cap6 名额(注释已披露,方向合裁定=噪声不再挤占显著披露;满帽活体零,commit 句「容量通道零动」略超——账本此句为准);P3-2 scan 文件标签误(post 态,比值治理独立复核全对);P3-3 恰 5.00% 保发射(`<` 语义,披露保守向)未 pin(浮点尘);全降板零提示=裁定意图。

## §29.158 RULER2-1 收账(2026-07-20;§29.150② 用户裁定落地=两把尺跨行披露句;R-19-b 销案;合并复核 SHIP 零 P0-P2)
**定形**:PARTSPLIT 范式(引擎判定/显示转录)——SelfRunnableTwoRulerAccounting 从发布板收割(typed 准入:runnable_wait∧self∧on_chain∧两尺俱占∧任一族席残破整记录静默);witness 句(donghu17267 ❺ lead 行下):「自身runnable账按两把尺记账:自身墙钟尺 2 席(3.956+1.193=5.149ms,同尺内可加)·唤醒边锚尺 1 席(1.648ms,另一把尺);跨尺不相加,无合计数」。**M3 禁混尺至上**:结构体无跨尺合计字段+全消费者 Σ census(唯一加法=同尺小计)+6.797 反向禁令 pin(复核官证独立承重:后缀注入形仅 ban pin 红);1+1 席形零虚构加法;三尺closed switch 不可构造。R2' 六键全套+strict parse 双尺恒等复验+schema 重钉。四板 A/B:三板字节恒等,17267 差=恰 3 行(图例+两行句)。M1-M4 全杀+复核官独立双突变五探针。
**偏离委托默认**:zh 词面「记账」避「分账」LEVELMERGE 探针词位(撞车实证);宿主=lead 席行2;族界=runnable_wait(scheduler_latency 3.120 typed 排除,Σ6.797 恰合 §29.136 备案);#14 序数=CAPFIX 后存活语义一致。**P3 备案(前既存面)**:#11 行「自身·墙钟席」佩词与句面「唤醒边锚尺」并置张力(各自诚实,词面裁定候选);#14 紧凑面无序数 chip 对照摩擦。checklist R-19-b 标 RESOLVED。

## §29.159 PROFREBASE-1 收账(2026-07-20;§29.150⑦ 用户裁定「批准 re-base」落地;主会话勘验直收=docs 域自带独立复算证据)
**§1.7 [probe] 全行重基于当前引擎**:full running 50.524→**52.478**(99 frag)/d_sleep 0.488+io 0.147→**io_wait 0.635=0.138+0.147+0.350**(逐区间验 prev_state=D∧iowait=1)/legacy115 24.992(67)→**26.946(66)** 覆盖精确 114.940 of 114.940;陈腐「~1.954 out-of-interval gap」句删,替以 §29.145 机制注(probe 时代 identity bug:tid 61847 误归 59566,差值恰 1.954)。**三方 µs 一致**:独立 python 逐行复算(51.462 闭合+1.016 窗尾 flush=52.478;legacy115 26.946 零 flush 零开区间=件3 同规)≡探针重跑(产品面 ThreadTimeline)≡a3 oracle 52±1 **band 零改**(case 头注按既定政策记录)。**被引面 sweep**:PROFILE 头部 re-base 记录+§1.8/a3/b3(52.478=50.524+1.954 折叠错误机制勘正为闭合+flush)/b5/c2 四 case+账本维度矩阵 2/8/9 行+boundary_fold 测试注;a2/d4/e1/f1/g1 引值未变核实不动;历史节(§29.140/§29.145/audit 文)保留为史录。探针流程一处机械适配(Frequency int64 cast)双点落档,「可复导出」承诺保持;探针删净。全套 83 包绿。
**备案**:d_sleep→io_wait 分型差=probe 时代引擎分型行为(Σ0.635 同值)非 identity bug,注文如实分立;§1.1 flavor 信号清单较现引擎少一枚 path_ext_systrace_ambiguous(值/置信零变)=PROFILE 卫生候选。**裁定队列七件全清**(§29.150①-⑦ 全落账)。

## §29.160 用户裁定集二(2026-07-20;裁定池新增六件逐裁+乙组整批追认;落地批=POOL2-1)
**①SPANVIS 第三门裁定 verbatim**:「这里的 "某个span过长…则需要提及" 指的是链上的,如果非链上,对优化方向和建议,以及优化收益都没有太大直接关系,属于噪音。」= 提及义务限**链上** span;非链上=噪音无义务(◈ 准入门1 本就 typed 链上,故语义蕴含:凡过门1+门2(显著)的链上族必须提及——**第三门「≥1 帽下隐藏段」对链上族撤除**(bounded top-8 视图非树/◎ 提及面,「席位面已见」辩护不成立);M4/M18 pin 刻意演化;OmittedFamilies 口径不变)。
**②INTERFLOOR 12.33%**:「按推荐的 推荐 A 维持 来」=公式维持(相对语义自洽:对小席主 12% 重叠可观),裁定池销;C'(第二相对条件)留观察候选。
**③PARTSPLIT 披露行**:「按推荐的来」=复用 SPANVIS 双分量地板 max(0.1ms,1%×窗)+行序改值降序;微真值降入 typed 记号(审计保留)。
**④CAPFIX 帽亡点名**:「按推荐的来」=每道 top-2 点名(76.800 个体可见)。
**⑤FREQDIR 方向词**:「按推荐的来」=EN 双面并列「修向=频率与热治理 (frequency & thermal)」(词表 EN 面现成,零语言决策依赖)。
**⑥RULER2 佩词张力**:「按推荐的来」=图例加正交声明(墙钟席=值口径轴;尺=归账车道轴,两轴独立),两既裁面零动。
**其余**:「其余的保持按默认推荐的来。」= 乙组⑦⑧⑨⑩整批追认(RATIO=5%/UPSTREAM-3 P2 倒装 pin/恒等基与词面委托族/PROFREBASE 分型注等)落账生效;丙组候选按推荐排批(CALSIDE P2 残形+UPSTREAM 三残口+tier1_floor+cap6 corner pin+词面卫生族→后续显示/收尾批吸收)。

### §29.160.1 裁定①补记(2026-07-20,用户重申 verbatim)
「这里的 "某个span过长…则需要提及" 指的是链上的(注意 自身也属于链上),如果非链上,对优化方向和建议,以及优化收益都没有太大直接关系,属于噪音。」= 提及义务覆盖**链上含自身**(◈ 准入门1 三臂 self=chain.Target 恒链上/chain_member/host 边凭证与此语义一致,自身臂不受第三门撤除影响之外的任何收窄);非链上=噪音零义务。POOL2-1 件① 以此全文为准。

## §29.161 G4-ENGINE 收账(2026-07-20;§29.145 立案=blocked_reason↔D 段 typed 配对引擎面;调研先行;旗舰双复核=双 SHIP-WITH-FIXES,返工全销)
**调研定谳(设计文档先行)**:c2 诚实红五环机械链全证——event_search 结构上不能铸配对面(census 只在 WindowStats 视图)/帽提示只导向更窄 event_search(烧圈)/**决定车道=SUPP-CORE 窗梯在 event_search-only 上静默跳**(pattern-only 无窗记录)/无 census 则 §29.145 件1 教学无物可绑;**吸收判定=未被吸收**(LENSBURN/TOOLWIN/COMPLETE-2/UPSTREAM-3 四批 file:line 锚证不触 census 在场性)。**第二缺口(探针发现)**:census count 序 top-16 pid 帽在 IO 忙 trace 上逐掉分析目标自身 count-3 行(donghu 实测 hisi_hcc×110 领榜目标缺席)。
**方案 A 两精确件**:A1=buildBlockedReasonCensus 目标 pid 准入(帽满时逐最低 count 尾行换入目标行;溢出计数守恒;目标缺席零合成;q.PID 精确信号);A2=SUPP 车道无窗回退(closed token 集 D 族命中∧census/rank 族缺席→无窗 root_cause_rank 经既有环执行;wire 零时间键;冷字节+SUPP-CANCEL 双熔断;禁猜=模型自定微窗不覆盖)。披露 fork=「全 trace 无时间窗——本次调查未确定统一分析时间窗」zh/EN,永不印 0..0;零新 wire note 键(meta 双字段循 CensusLite 先例)。c2 确定性链 pin=产线端到端(census total=3+sync_buffer_read_wi×3+自身 io_wait 0.635+caller note),EVALGUARD 三锚复活,案锚零改。
**返工全销**:P2-A 逐位受害者 pin 修真(真受害=pid13;逐头突变红)/P2-B dispatch 级 wire 捕获钩(绕 helper 突变红)/P2-C 连体词形词界判定(audio_wait/radio_wait 四形负臂;blocked_reason 蓄意保子串;CJK 界字节合法)/P3 五件全落(DeepEqual 升格/教学 selected_window 自名口径软句/伴跑 lane 标签如实/吸收 file:line 锚/无窗取消端到端 28LOC)。M1-M6+RM-A/B/C 全红。
**备案**:option C(event_search 帽提示词面)残余候选;model-pattern 第二触发面候选;pid=0 线程名 census 准入候选;**verification_probe_runtime_test 载荷敏感 flake 类**(全套并行下 10s 子进程界偶超,隔离复跑 7s 过=前既存类,如实申报入 commit body,bound 上调=卫生候选)。

## §29.162 POOL2-1 收账(2026-07-20;§29.160+§29.160.1 裁定集二五件落地;实施代理失速由主会话代 commit,合并复核升主验证=SHIP-WITH-FIXES,主会话收编)
**五件全落(复核官独立五板 A/B+十突变主验证)**:①第三门撤除(HiddenCount 准入臂灭;门1 typed 链上含自身三臂零收窄,§29.160.1 全文形入 EVOLUTION;自身族正臂 pin;非链上 50ms 仍零提及负臂;旗舰五板 ◈ 恰零变=三门在旗舰上从未咬合,单元 pin 为唯一守卫);②12.33% 零码销案核实;③PARTSPLIT 地板复用 SPANVIS 常量单点(零第三份)+值降序(tieba trace 微行退池、13.982 领行;池内 typed 记号审计留存实证);④帽亡每道 top-2(donghu2955=81.616+76.800 双点名;单亡道无次大负臂活体;平手同严格 > 规则);⑤方向词双面(Table ⑦ zh/false 同源,preamble 同步);⑥两轴正交句 zh/EN 双面 pin。五板 RANK 行字节恒等=值通道零动实证。
**主会话收编三修**:P2-1 发射侧 HiddenCount:0 无牙(复核官 M9 突变幸存实证:回退 traceQueryTypedCount 吞 0 成键缺席→strict parser 整记录丢弃=①裁定目标形静默消失)→新 pin TestSpanvisMentionObservationEmissionHiddenZero(M9 复验红);P3-1 note-key 注册表注释「int ≥ 1」勘正为 0..Count 合同;P3-2 harvest 头注措辞如实(pool-only 席进程内留存)。**备案**:无窗 PARTSPLIT 保尘埃准入=蓄意宁降不删偏离(pin 在岗);enrich 道同线程双行点名(逐行裁定合规,主体去重=未来裁定项);worktree 账本缺 §29.160.1 由本合流对齐。**过程记录**:实施代理两度失速于等待态,主会话按 §29.146 收编先例代跑门证代 commit,复核官升主验证角色并逐行佐证其五板归因。

## §29.163 CLUSTER-FIX-2 收账(2026-07-20;§29.129 移交五件+§29.150⑨ S4 caveat;实施代理失速主会话接管收尾;主验证合并复核=SHIP-WITH-FIXES,收编两修)
**六件全落(复核官逐件对审计底稿原文勘验+真 trace A/B 簇判定字节恒等)**:S1 freq_only typed 七臂 enum(审计五 token 细化,R2' 式全链同步+跨包 token 镜像 pin+donor 双生travel;主会话补登信息契约表 displayed,复核官 M9/M10 双向证登记强制且状态唯一正确);S3 rail 宇宙门(closed 三臂 CPU 归属集=底稿修向 verbatim+反循环排除收窄;旧宇宙拒绝形对照 pin;真 trace rail 采纳 NONE 双侧恒等);C1 burst 单次见证=**纯披露 token 细化非硬门**(D2 蓄意偏离 refs「可作硬门」草案:准入会反转 §28.5 复核 P1 floor 裁定——全策略齐停形满足全部字面条件=floor 所防的假并;**硬门准入=裁定池新项待用户**);C2 limits 锚=派生道内锚 mismatch 披露(S9 既判「裁定候选非急件」,判定零改 pin);C4 掩码宽度作核数见证(宽度=精确硬门/掩码值=嘈声只软,底稿 verbatim;3fff→0..13 pin);⑨S4 limits caveat(与 FIX-1 dropped_cpus 同构句族;delete 判定字节零动;:328 蓄意不记录注释更新为 EVOLUTION 真话)。15µs 常数零动 grep 证;十突变 9 杀。
**收编两修**:P2-1 图例「簇结构不可判」句与新单簇行同页矛盾(承诺面红线)→图例句泛化「簇结构不可判、或仅单簇有频点采样(单簇内频点等价)」zh/EN;P3-1 burst 贴界 pin(M6 幸存实证:常数换 100µs 字面全绿)→界内 1µs 余量/界外 5µs 双臂(浮点尘教训:恰界形表示敏感,余量臂为准),M6 复验红。
**裁定池新增**:C1 burst 硬门准入(refs 草案 vs §28.5 floor 裁定优先序,现=披露道;启用需用户显式推翻或界定并存形)。**备案**:P3-2 caveat lift 扫描广度(首中 roster 声明形,卫生候选);D1/D3/D4/D5 偏离委托默认(七臂细化/C2 披露向/反循环收窄/gated lane 零 reason twin=显示小批候选);过程=代理失速形二连(接管范式沿 §29.162)。

### §29.163.1 C1 burst 硬门准入裁定(2026-07-20,用户 verbatim:「同意 按推荐来,然后继续推进」)
= **A 维持**(单 burst 仅披露 token,floor 至上)+ **C 立为条件启用形**:对比见证升级(burst 须展示边界两侧异频点的分割证据;齐停无对比仍被 floor 拦=细化非推翻 §28.5)——启用判据=comove_floor_single_burst token 自然采集(客户回访/eval 窗)显示「floor 拦住且 burst 携对比」活体在场且量可观时开批,旗舰双复核;活体全为齐停形则 A 即终态。**裁定池清空**。

## §29.164 DISPHYG-3 收账(2026-07-20;§29.150⑩⑪ 两裁+丙组词面族五件;合并复核 SHIP-WITH-FIXES,主会话收编 P2-1)
**七件全落**:件1 C8 标点分制(系统铸造非 bullet prose 句顶层全角，；。/fence 体半角(census 实测恰零全角)/括注内与 fence 共享 token 保半角;witness「s,共」四板灭;10 旧 pin 刻意演化;残余非 bullet prose 导语/caveat 面 file:line 备案=后续卫生批);件2 C4 折叠边词统一带连接符(几何量测=180 bar 行列位逐行恒等零推挤);件3 F7 残二号关账(anchored typed 信号三板类分叉:无锚无窗=「不设占比标尺」诚实头/零持值=「本板无持值行」/混合有窗字节照旧;NoRulerMark 入 tracefence 闭集 2→4,preview 分类器自动覆盖);件4 板 preamble 口径感知句;件5 ⌗ 词 Seg 9→13(类别词先,census 取多数形;Seg sweep 零波及);件6 两把尺参与席补「根因排序#N」chip(#13=真池序三独立见证;唯一宿主门;裸 #N 禁令零打);件7 gated reason twin 全 R2' 链(同页矛盾灭:「簇结构不可判」post 零现)。M1-M7 终态全杀(M6 首轮幸存自抓补牙=批内诚实记录)。
**收编 P2-1(复核官抓幸存混用行)**:donghu_2955 供给折算结论句=fence 共享 composer 三难(转 lead 分叉/双转违 fence/不转留混用)——正解=**标点制参数化**(runtimeTraceProjProseClauseRegime 括注深度感知变换,lead 嵌入位深度0 ,;→，；,括注内保半角;composer 单点如 zh/EN 先例零分叉);pin 双层(helper 语义+lead 嵌入产线齿,后者首轮突变幸存自抓补齿=SupplyFoldComputed fixture 课);P3-1 preview fixture 漂移行随手对齐。**备案**:扫描器「」盲区(test-only 零产线门)/µs 等值双参与席边形(零活体)/legacy Suffix wrapper 无产线呼叫(存档族)/C8 残余 prose 面清单=卫生批候选。

## §29.165 R10WIRE-1 收账(2026-07-20;checklist R-10/§29.150⑫「按 M18 模板收敛」;双单位形四面收敛;worktree 隔离实施)
**步1 census 定策(落盘先于编辑,M18 程序)**:cpu_constraint_allowed_max_tier_khz/global_max_tier_khz 全读者 census——硬消费恰一对(发射 tool/trace_query.go:8433 ↔ 解码 types/trace_causal_projection.go:3414)+注册表/golden/契约 pin 全仓内;eval/tracediag/examples **零**键名/kHz 格式 oracle(频点 oracle 只钉别的值且 kHz|MHz|GHz 三形并纳);核档双值 kHz 面恰四=①wire note 键(*_khz 键名承诺,诚实)②③引擎 Summary k=v 自由文本(**单点铸** query.go:6741 renderCPUConstraintSummary,流向=「- cpu_constraint」文本行+observation detail=+rank 席 Summary 逐字拷贝=唯一与读者面 GHz 分叉的值文本面)④JSON payload 镜像(tracequery/types.go *_khz 字段,键名承诺,诚实);observation_ledger k=v 解析不消费 excludes_bigger_core_tier 字段。census 勘正:§29.124 rider 实落**一条**四面挂注(全仓 grep「双单位形/暂不改」恰 1,非 spec 句「四面各挂」四条)。
**步2 选型(委托默认,论证入库)**:a(四面全 kHz)=spec 自否(B11 读者面既裁 GHz);b(wire 值改 GHz+键改 *_ghz)=**否**——int→float64 wire 触 §29.42 零无发布/未发布陷阱(证明对负臂全靠 int>0)+精度有损(kHz 阶梯实测整数,%.2f 有损)+最大涟漪换负信息增益;c 字面(加 typed unit 字段)=**不需要**——*_khz 后缀就是 typed unit 自描述,第二字段=同一 typed 事实第二词家(§29.119 C4 seat_status 先例)。**采纳=c-minimal**:wire 车道(note 键+JSON 镜像)kHz 键值逐字节零动;唯一 kHz 自由文本面(单点 :6741)收敛 B11 约定 %.2fGHz(÷1e6);k=v `<` 不加空格=deliberate(k=v 行空白分词,B11 spaced `<` 属显示面文法)。终态:单位随自由文本走的面全 GHz(同一换算式),单位随键名走的面全 kHz(精确 int);R2' 七处/golden/读者 union=**零 delta**(零新键零改名,§29.116「R2' 零新键」先例);旧键零残留 pin 以「零改名」形满足+ghz 键负 pin 补钉。
**步3 落地+witness**:改动=1 格式串(query.go:6741)+「暂不改」注记销转 EVOLUTION RECORD(tree.go B11 站点);donghu 活体 A/B(JankManager-9655 mask=ffb 窗)=BEFORE「allowed_max_tier=2270000kHz<global_max_tier=2750000kHz」→AFTER「allowed_max_tier=2.27GHz<global_max_tier=2.75GHz」,其余字节(27.507ms/488.565ms/policy)逐字恒等=值语义零动活证。pin 四组:引擎合成双臂(GHz 形+全 summary 零 kHz 残留+对残缺三形负臂零任何单位词)+donghu 活体跨面恒等(wire int 2270000/2750000 精确保持∧Summary=同 int ÷1e6 %.2f 像,公式非字面)+tool 侧跨面恒等(fence GHz 词=node kHz int 公式像+fence 零 kHz)+wire 残留双负(*_khz 键值必逐字 int 禁小数禁 GHz;全键族禁 ghz 命名)。**突变账 4/4 红**(cp 恢复):M1 Summary 回 kHz→引擎双 pin 红;M2 显示面回 kHz→rnb2 字面 pin+恒等 pin 5 红;M3a wire 值改 GHz 浮点→round-trip+残留 pin 双红;M3b 键改 _ghz→golden 4 红+ghz 负 pin 红。
**偏离/备案**:①freq=/weighted_freq=/observed_max_freq=%dkHz 家族**刻意不动**——挂注词面借该族描述 k=v 面文法,非核档双值本体;各行值文本自带 kHz 词=单位诚实,收敛属独立值词面裁定(eval 频点 oracle 波及);②「四面各挂注释」census 勘正如上(实一条);③明细块 wire 字段镜像面=JSON *_khz 键名承诺形,与 wire 同车道同判;④gofmt 全仓预存 96 文件(复核官量测勘正)未格式化=批外既有,本批四触文件全 gofmt 洁净。

### §29.165.1 复核附注(2026-07-20)
R10WIRE-1 合并复核 SHIP 零 P0-P2;P3 三件已勘(gofmt 计数 94→96/M3b 红数 4→3 可复现形/0.00GHz 微档理论形=真实核档梯不可达,footnote 备案);复核官独立 census 复扫零漏消费者、%.2f 舍入边三值构造证两面同式、旧 kHz 文本形零读者、live A/B 唯 tier token 变。**既裁队列全清**(R-1..R-20+两轮裁定池+§29.150/§29.160 全部落地)。

## §29.166 UPTAIL-1 收账(2026-07-20;§29.151 候选五件收尾=UPSTREAM 家族权威面;旗舰双复核=双 SHIP 零 P0-P2)
**五件**:件1 P3-a 取收紧臂(a)——preflight carve 臂加 !anchor 合取(仅 preflight 臂;bundle/MCP 臂保已追认 carve-beats-anchor 姿态,反向齿 pin);对抗官全真值表证**收紧不可能铸 current_source 债**(carve 达阵必要条件 E=waiver 臂①同谓词,pre-mint withhold 恒接);§29.146 witness 族零锚形水位绿。件2 P3-b 跨 kind 权威 pin(小 bundle 已解 frames 不被大 log preflight 臂越权)+无引用角面自洽 备案(双官执行验证)。件3 P3-d 词面臂降 precise-anchor-only(严格子集证);**真混合载体裁定=agent pin 分叉演化**(bool+裸符号版本查询→诚实降道 allowed_optional;typed anchored profile/file:line 保全);analyzer 教学已指引 typed 载体。件4 tier1_floor 裸维地板收编同谓词单点(排除 carve 前置保持)。件5 死 helper 删+墓碑(零引用双复核 re-grep)。M1-M4+双官自加 M5/H2 反向齿全红;L1+read-e2e+GAP-4 族全绿。
**双官 P3 记账**:①收紧臂新保留格使 runtime_artifact 车道获 auto-complete 债压+sibling 等待(pre-batch mixed 门结构性关)——实际可达阻塞≈零(closure 受理见证要求结构性防+单次 trace_query 即偿,有界无环),**本行为 delta 正式入账**;②件3 翻转=独立行为翻转非 P2 涵盖(R-21 候选 P3-d+spec 授权在案),**追认清单独立行**;③matrix 缺 interlock 行(锚 pin 传递覆盖,备案)/reason 串软面未 pin(备案)/CurrentSourceObligationSignal 同族无精确锚检(候选,未来同族 sweep)。

## §29.167 TAILHYG-1 收账(2026-07-20;候选池三微件;主会话勘验直收=零行为批复核成本超收益)
件1 cap6 腾位 corner pin(§29.157 P3-1 销):满帽 8 对 fixture 证 demote 腾位→先前帽阻 keep 对确定性入槽 6+帽内守恒+末对诚实帽亡;突变(demote 不腾位)红。件2 PARTSPLIT 防御清零(§29.156 备案销):换道席四元组清零+不变量注(换道席永不携拒转记录;不可达论证=四元组唯 priority_inversion 臂铸而 converter 唯 runnable/D-IO 臂达);双臂 pin+突变红;◇ remainder clone 同论证覆盖备案。件3 PROFILE §1.1 第六信号行(§29.159 备案销:path_ext_systrace_ambiguous 零权重记号,flavor/置信零变,探针实测双 fixture 清单入注;oracle census 零消费者)。全套 83 包绿。**候选池余=C8 残余 prose 卫生批+caveat lift 广度+CurrentSourceObligationSignal 同族 sweep 三件。**

## §29.168 OBLSWEEP-1 收账(2026-07-20;§29.166 记账③候选销=CurrentSourceObligationSignal 同族精确锚 sweep;checklist R-22)
**勘验裁定(先勘后动,处置=阶梯②精确锚降级)**:生产者 census=恰一铸点(`CurrentSourceObligationSignalsFromRequestedDimensions`,唯一产线调用 emit_analysis.go parseRequestedAnswerDimensions→RequestModel 落袋)。铸造条件=Required(LLM bool)∧义务角色枚举(LLM 分类)∧**未通过请求原文 provenance 校验被丢弃**——载体非自证:武装条件与锚定反相关(信号恰在校验官拒绝的维度上点火),空 label 零面维度亦铸。丢弃车道由此比幸存车道(§29.146 件3/§29.151 件3+件4 已收编 precise-anchor-only)**更弱闸**=家族最坏面倒挂,对抗官 §29.166 观察实锤。载体刻意不携文本(防 prose 依赖)→消费侧无从补检,铸点=唯一单点;铸时认证使裸消费(臂④)转正。
**落地(纯放宽向,零收紧)**:铸点加合取 `requestedAnswerDimensionHasPreciseCurrentSourceAnchor`(=dimensionHasPreciseCurrentSourceAnchor 同体,提为包级单点谓词,§29.146/§29.151/本批三面同源);prose-only 丢弃维度留 advisory 车道(normalization warning 仍在),path 后缀/file:line 锚维度保铸(presentation 丢弃不失真义务)。消费者 census 四硬面+一软面逐点核:臂④ 验证锚(lane Required)/读源审计债双面(:1425/:1464)/tier1_floor 地板/completion landing 显式 runtime-origin——信号在每面均=加义务永非许可,减铸=每面纯放宽;RuntimeSourceAuthorityRequestCarrierActive=软 prompt 预算面(自注非硬闸)。**行为 delta 入账**:runtime-artifact 运行上 prose 转述维度不再翻 lane required(emit_analysis 产线 pin 由 prose 铸 2 信号改写为双臂:prose→0 信号+lane allowed_optional+summary 无计数;精确锚→2 信号+Required+计数照旧);「结合当前源码」类需求正规 typed 载体=current_source_mode:"allow"(§29.146 同款交换)。
**pin/突变**:pin 三组=铸点 prose 零铸(witness 族「链上主要原因」转述形+零面空 label+nil/幸存双 profile)+§29.146 witness 隔壁族回归(真产线链 normalize→mint→RM:丢弃 prose 维零信号∧锚不武装∧lane 保 allowed_optional;file:line 同形保铸→锚→Required)+emit_analysis 产线双臂。突变账 2/2 红(cp 恢复):MA 闸门中和→三新 pin 全红;MB 过降级恒不铸→保全臂 pin 全红(RecordsDroppedSourceRoles 精确锚化+两新 pin 保全臂+产线臂2)。L1 byte-identical+read-e2e 全绿;types/orchestrator/tool/agent 显式 EXIT=0;全套绿。gofmt:触文件全洁(全仓预存偏差批外)。备案:历史 session 序列化 IR 内既铸 prose 信号重放仍武装(消费面不变所致,铸点治理只管新铸;水位随重放退役自然清)。

## §29.169 P3MEASURE-1 立案裁定(2026-07-20;用户裁定链 verbatim;§29.168=OBLSWEEP-1 在飞预留)
**裁定链**:主会话提出 ONCHAIN-P3 两阶段修正案(阶段一=静默量测双不可见/阶段二=数据门披露)→用户以反事实推理勘验语义:「如果 Worker-200 早完成一点,早一点唤醒 UI-100,那么省出来的时间都应该算做窗内可消除的影响,按照这个逻辑推理下来,是否能支持决策?」→主会话确认该推理=席值公理的链上机械展开(工作控制型边上铁证成立;兼服形合法性同源),并定出推理失效两反例族(绝对时刻钉死边=定时器/vsync/周期源;晚到中继形)=P3 真偏差本体;量测口径升级为**边前移反事实有效性**(直接量「省出来是否真省」)→用户终裁 verbatim:**「按照这个决策,选择最优方案,开干」**。
**P3MEASURE-1 定形**:阶段一静默量测,每链上席记〈反事实有效份额+结构边见证份额〉双维,display-only 载体(模型/用户双不可见);反例族只认 typed 精确判据(①周期/绝对期限钉死=复用周期源折扣机器 typed 分类;②晚到中继=若无法 typed 精确化则本期不量、如实备案量测覆盖);校准 pin=审计 2%/0.1% 复现+周期折扣席一致性;值/榜/席/板指纹零动;阶段二维持数据门(中间带活体可观才议,词面须「见证下界」语义禁比值形,需届时新裁定)。

### §29.168.1 复核附注(2026-07-20)
OBLSWEEP-1 合并复核 SHIP 零 P0-P2:恰一铸点/反相关(信号恰在 provenance 拒绝维度武装,零面亦铸=MA 突变可视实锤)/严格子集/单点谓词/六消费面 census 全向放宽/端到端自构探针过/2 突变红/45 pin 族绿/cherry-pick 净。P3 两记(账本「四硬面+一软面」计法内部一致可留;types 端 pin 手装 RM 由 emit 端产线 pin 兜底=已充分)。候选池自此仅余在飞两批(C8PROSE/P3MEASURE)收账即排空。

## §29.170 C8PROSE-1 收账(2026-07-20;§29.164 备案残余 prose 面+§29.163 P3-2 caveat lift 广度;合并复核 SHIP-WITH-FIXES,主会话收编两修)
**件1 残余 prose 面 12+2 转**(逐面处置表在批报告):明细/证据索引/对比总览/分区 caveat/coverage/优化表/at-cap skip/census 「等」面+tree 跨文件双子(扫描臂活体抓获)全转 regime(深度0 全角,括注与 token roster 半角保,`:` 保);不改裁定=bullet 建制/⚠ 注记行车道/F2 既合规/next-step 列表车道,逐条论证。**件2 lift 面 census 单一权威**:scanResultSupplyFoldBases 走查全部已序列化 basis 面(含 AbsorbedItems=批自抓漏面);C2 锚 roster 改 sorted union(divergent 臂 pin 强制,memo 化论证=可达形等价);burst 保 OR、split-audit 保 first-hit+序 pin;tieba 活体 A/B(24+21 载体)复核官独立双窗复放字节恒等=判定零动。M1-M6 全红+复核官独立双突变。
**收编两修**:P2-1=五句 SUPP 披露 caveat(G4 无窗披露族)漏转→production 五句+两姊妹句+两 join 位全转,cancel/supplement 两测试文件 pin 刻意演化(EVOLUTION 头注);P3-1=第九已序列化 basis 面 nodes[].impact 靠未成文镜像不变量幸存→walker 补 node 面访问臂(产线幂等,结构性强制优于不变量信赖=批自家哲学)+注释勘正+census pin 第九面臂(node-only cpu8,掉臂突变红)。

## §29.171 P3MEASURE-1 收账(2026-07-20;§29.169 用户裁定「开干」落地=ONCHAIN-P3 阶段一反事实静默量测;旗舰双复核=冷读 SHIP+对抗 SHIP-WITH-FIXES,收编四修)
**口径兑现(§29.169 裁定链忠实,冷读官 UI-100/Worker-200 思想实验走码验证)**:每链上席双维——反事实有效份额(族①=复用 VS-1 detectPeriodicWakeupSource typed 判官逐窗定罪;无 typed 记录永不定罪=「absence never convicts」;族② 晚到中继无既有 typed 判据=本期诚实不量,p3m_coverage=families:[periodic_pinned] 逐席披露)+结构边见证份额(唯一硬化解析器+census 同源边库存+rspaIOCompletionClosureTolS 同源容差,零新常数零新扫描)。七处置闭集(self_ruled 零数字=禁重诉既裁;discounted 席扣发结构维防 >席值 谎;len==cap 拒二次聚合)。**校准=归档审计 µs 级复现**(legacy caps:0.419/1.97%+0.011/0.07%+18.641/97.78% 三席逐字对档,复核官独立复读归档表核验;HEAD caps 分子守恒+469µs 审计预算硬护栏)。**双不可见端到端**:四板+17874 形产线全链 A/B(用户 zh/en 面+模型 Summary/observation 面)字节恒等;supplement 车道双 pin;全仓 grep 闭合 pin。值/榜/席/板指纹零动(M6 Score 微扰红);7/7 突变;两机器同判 pin(berlin VS-1 同席 36.256→0.105 折扣处恰 invalid-in-full)。
**收编四修**:P2-1 覆盖披露诚实 pin(对抗官 M7 谎称量了族② 全绿实锤→literal pin+演化合同注,M7 复验红);P2-2 host-edge 臂精确判官(pid 粗 OR 过度定罪混 cadence 宿主=严格保守向但精确判官两行可得→HostWakeupEdgeAnchorTs 逐窗判,pid 级仅作无逐窗记录宿主回退;伴生混 cadence pin);P3-1 消费者缺席 pin walk 升仓根(cmd/入网,.git/.claude/.codrax 跳过);P3-3 ≤席值 doc 补 1µs 尘隙注。**备案**:冷读五 P3(prompt-notes 第二选择点靠排序屏障=候选补拒收 pin/note 文本载体通性旁路既有形一句备案/边库存无 inWindow 滤 ≤0.5ms 界外可及=文件头注候选/doc nit 已修)+tracediag 反射面=审计之家裁定(SelfGapSemanticOverlaps 先例,若用户读「任何渲染面」含 diag dump 则三行 skip 随裁)。**阶段二数据门就位**:量测自此在每份报告静默积累;中间带活体可观时按 §29.169 词面纪律(见证下界禁比值)议披露,消费者缺席 pin 的红=届时复审面。**候选池全清。**

## §29.172 DHMINE-1 收账(2026-07-20;用户提议=donghu 双 trace 换线程/窗构造场景替代回访采集;合并复核 SHIP 零 P0-P2,收编两 P3)
**C1 头条判定(复核官原始行独立重建背书)**:两 trace 全扫零对比 burst——donghu 182 burst 唯一多值形(ts 13762.858645,cpu12+13,{1675000→1200000})为**齐动共移过渡**(两核各自见证两值,无任何瞬间两核异值;且同派生簇无边界可言);tieba 30/0/0。**CONTRAST-1 合法保锁,A(仅披露)在本地数据上即稳态**(精确措辞:活体为齐动形非齐停形;判据=边界两侧异频点分割证据,§29.163.1)。census 冻结为 caliber-drift 看守 pin(带 §29.163.1 指针,禁静默重基)。
**采矿产出**:16 未用(线程,窗)板探针;病族候选表(A1×4/A2×2/A3A5×3/A7×2 组合;A6 诚实无引擎活体=整项维持外部);**11 确定性新案全绿**(两把尺守恒/板指纹分离/非目标席/逐道位移披露+top-2/反转对/M18 wire fork 双向/密度归一反转/双窗定界+C1/P3M/EPS 三看守);零引擎 gap。**搭车三采集**:P3MEASURE 125 席 248 窗旗零 periodic 定罪=「跑了且无罪开释」(复核官探针证判官真跑;阶段二数据门本地无中间带标本,静默继续积累);D1 七条=ε-overlap 活体丰富(0.099% 标本 pin,启用留用户裁)/io-facet 域外锚定零(原始行有 workqueue/dma_fence 但零链上锚定机会)/周期 CWD 零/C5 分布归档。
**roster 收缩**:A1/A2/A3/A5/A7 引擎病族形**本地已替代**(案号入注);外部最小集=LLM 显示/行为复放(A1 chip 互指面/A2 ◎ 面/A3 标题行为/A5 transcript/A6 整项/A7 对比总览+回探+data transcript)+B1-B3 工件+C1 260M+C2 berlin+D2 可选。**收编**:P3-1 count-pin 消息补 §29.163.1 指针;P3-3 测试头注会话期 scratchpad 引用换耐久指针(账本/roster);P3-2 A7a 算术臂冗余+P3-4 头注措辞=备案。

## §29.173 EMITBURN-1 立案(2026-07-20;客户反馈=emit_investigation_complete 连环硬拦四轮+「remove or fix」删条疑虑;双路诊断定谳)
**问1 是否正常**:拦截本身=合法(三族均属既裁致命类「schema 无效」可拦,feedback_completion_gate 红线);**连环四轮=结构性缺陷非模型失能**——验证管线每层首错即返零累积(JSON strict decode DisallowUnknownFields 首错/NormalizeAnswerAggregateFacts 首败 fact 即返 :209-213/fact 内 kind→label→dims→negative 臂→value 串行链/negative_observation 内 value→origin@4451→target@4455→scope→searched_at 逐臂/集合级校验首违即返)。witness 序列逐臂对号:round5=origin 臂,round6=target 臂(同函数相邻,修好前者才见后者;客户模型写的维度名 evidence 不在 canonical 表),round7-8=他 fact 形/decode 层。**2026-07-02 已诊断同病仅修 2/6 臂**(origin/target 带完整示例,注释自证;scope/searched_at/跨 fact/decode 层未修);理论烧轮上限 16 facts×4 臂≈60+;硬车道因 enumerate/scalar/义务问形非可选+零宣告负事实=承重缺席证而必然接通。
**问2 是否删信息**:分层——**结构安全**:引擎铸造全线(root_cause_rank 账本记录/权威板/wait-wake census/emitted evidence/completion reason)不经 aggregate_facts,数值骨架不可被模型删;**真可丢且永久**:被拒尝试零保留(仅 accepted emit 落 MutableState :2335-2338;carry-forward 只并已接受完成),第 4 次删除的模型自算标量/typed 零结果 negative_observation/member_set 清单从系统面消失,降级 reason 散文(下游显式降权);**最重发现:拒绝文案自己教删**——support_refs 臂「remove the member entirely」/基数臂「omit the fact」,仅 cap 臂说勿丢 principal,无任何「修而非删」总则——模型「故意删除」是循我方文案指路,非使坏。本例精确定损需客户回传该 run 完整 log(attempt3 vs 4 payload 对比);transcript 可见下游 Prior Stage Findings 数值齐全=骨架无损。
**EMITBURN-1 修向(委托默认排批)**:件1 错误累积=一次 emit 校验全 facts 全维度一条 reject 列全违规(脚手架现成:843-859 salvage 循环已逐 fact 收集 oneErr 仅用于丢弃未用于报告);件2 示例补全=scope/searched_at 臂补完整最小示例+inner decode 接 schema hint(2026-07-02 未竟半);件3 维度名 near-miss 提示(evidence→target 类,reject 点名改名不加别名防错用固化);件4 反删除教学=reject 总则「优先修正;确删条目须在 reason 列名说明」+「remove entirely」降为末选;件5 auto-repair 触发面(mixed repo+artifact 注 origin)评估件。纪律:全部=reject 文案/报告形层,零判定放宽零新硬臂;弱模型 witness 序列建 fixture 复放。

## §29.174 RUN2AUDIT-1 立案(2026-07-20;客户 runnable_2.txt 全片段三路审计=过程烧轮/◎ UX/断代重渲;判词=客户构建≈当前 main,投诉即现行设计)
**输入**:runnable_2.txt 515 行全片段(donghu tieba frame-76795,MiniMax-M2.7,墙钟 19m19s);三路并审=process(烧轮/误导)+ux(◎ 与全答案面)+rerender(断代+当前 main 同窗重渲对拍)。
**断代定谳(双字节 pin)**:`:494` 全角`；`=36cf7099c C8PROSE-1 独有形(前像半角)+`:129`「s，共」=§29.164——客户构建 ≥36cf7099c(2026-07-20 08:48)≤main 40b03a54d,§29.133..§29.170 全部显示批**均在其构建内**;缺席仅 P3MEASURE(设计双不可见)+DHMINE(fixture)。**重渲对拍**:产线路径 probe(donghu_tieba_frame carve,同窗 34579.450627..34579.595130 双板)form-for-form 复现客户 render;PARTSPLIT 块(13.982/0.020/14.002,34579.555890)/∩ 19.661/守恒 144.503/TOP5 clip ⛓14◇5 **字节恒等**;全部可见分歧=数据侧(437.7MiB 全 trace vs carve:VerifyClass 7.109 席/背景 9 行 census/E1 24.813↔23.994)或 typed fork 臂(≥50% sleep 词形/freq_only 单簇臂/⌗ 单席行内 vs 多席块形/◈ 合并序)。**判词:零「升级即消失」项,UX 投诉全部为现行设计上的真 gap**。
**Finding 名册(33 项)**:process F1-F9(F1 四连拒=§29.173 witness 精确对号;F2 **member_set 臂三再拒+跨子节课不转移**(9 emit 7 拒 2 收,同课 checkpoint 两教)=§29.173 外新缺口;F3 dock footer 撒谎「16 调用/2 轮/1 阶段」vs 真 31/~21/4=re-dispatch+resume 计数归零;F4 成文自矛盾「wakeup 62 次」×3 vs 分量 31+34=65(62 借自 state_churn 切换数);F5 首字节 40s cap 被破 7 次至 1m0s+ 无披露(客户 config 40s<推理模型地板+心跳静态 cap 面);F6 citations 5 提交→0 入册零披露(runtime-artifact 改道 E# 设计,delta 静默=gap);F7 拒后零保留→充分性宣告后仍 10/19 trace_query 重推演=EMITBURN 放大后果;F8 纯 trace 问 checkpoint route_tools 供 repo 码工具+reason=proof_weak 误导;F9 malformed JSON 单退单收=设计生效);ux UX-1..18(P1×7:**119.320ms vsync→doFrame 核心量探索期 6 次算出成文零出现**/51 行>100 cell 最恶 L115=1106/◎ 头承诺「节序降序」被块身违反 7.394 先于 16.684/全树最大单项 47.282 藏折叠而 0.021 具名/E48 66.093ms 主体名整列空/线程名截成「c…-59566」不可辨/导语等待总量 149.263>窗 144.503 且≠四态和 136.009 零披露;P2 族:三个#1 打架+E1 缺❶徽章/套话逐行复读(不可跨板相加×11/边锚定 3 行段×5/同窗值×26,树面 23% 行=套话)/21 记号 10+ 先用后释+「见图例」×9 而图例在截断尾/prose 泄 wire 词(tier=primary/dominant_state)/同组数 4-5 遍/「下一步」纯模板话(确定性模板 runtime.go:7088)/折行撕裂语义单元/树「成因」边词误导+快照双数无对账/◎ 38 行席行仅 7;P3:R3/R4 规则号上 fence=§29.156 备案未落);rerender ERA/RENDER/DATA/LEAD-1(cookie 锚四态行 probe 未铸(疑客户由 thread-state 查询喂入)+149.263 算术=全 trace 复放复核项,挂回访)。
**处置(委托默认排批)**:①EMITBURN-1 在飞扩容(F2:member_set 上牙入件2 范例集+checkpoint 课转移进下一子节 dispatch+fixture 扩 9 attempts;F7 可选=被拒 emit 字段作 salvage 草稿入 resume);②RUN2FIX-A=P1 显示六件(UX-3 节序承诺/UX-4 折叠带值披露/UX-5 空名 fail-loud/UX-6 截断反转保名/UX-7 导语对账尺注/UX-8① ❶ 对称);③RUN2FIX-B=过程面(F3 计数累计/F5 心跳有效死线+config 地板 WARN/F6 引用 delta 披露/F8 route_tools 滤+reason 词;F9 可选 fixture);④RUN2FIX-C=成文教学(UX-1 帧锚 named-fact+duty/UX-11 wire 词禁教学/UX-18+UX-2② 骨架与短句教学/F4 总-分对账 soft);⑤A2 排队(UX-2① 席行折行纪律/UX-10 图例前移/UX-13 下一步实例化/UX-14 断点白名单/UX-16 三件);⑥**裁定池新增四件(涉既裁承诺面,禁单方面动)**:UX-9 套话上收图例单点(行级留短记号)/UX-12② ◈ 双面合并/UX-17 ◎ 脚注族收敛统一形/记号瘦身(❶-❺ 与根因排序#N chip 双载裁其一)。红线:成文面全部软引导(§29.42.4/§29.104.13);F4 嘈声→soft only;显示批值通道/榜序数零动。

## §29.175 AUTOREPAIR/OMGCLEAN 立案(2026-07-20;用户三裁定+emit/answer 日志定损;三路工作流定谳)
**用户裁定 verbatim**:①「「schema 无效」"JSON"失败等,系统安全的能自动修复吗?避免浪费轮次让模型去修系统自己轻松做的事情?」②「"窗内可消除量总览" "方向未定/复合" 这个表述像是未完成分析似的,这种表述不对,既然都有可消除量了,为啥还是 向未定。另外请保持 "窗内可消除量总览" 的整洁,不宜放太多信息,排版要整洁,分区或逻辑清晰」③附件 emit_investigation_complete_log.txt=§29.173 请求的定损材料。
**定损定谳(客户疑虑洗清)**:校验节被拒(09:34:53)与被收(09:35:24)payload 字节级对比=reason/confidence/9 个 fact 全同,唯一 diff=fact[6] member_set→scalar_value 重打型+label 改名,4 节点链在 dimensions **原样保留;零删除**;首收→校验收之间 fact 更替=证据刷新(观测 1866→2915)非门驱动,掉的模型 facts(Chrome_IOThread 12.173 等)由确定性投影车道在用户面全数还原(§29.173 问2 结构安全判断实证)。**新病**:该 member_set 拒文(len=135 verbatim)「use scalar_value for prose-only summaries」教**降级**,与系统自身 retry hint(要 members[] 逐名)对拉,模型循文降级→typed 成员清单终未铸;malformed JSON=**单个缺 `}`**(11 open/10 close,s1 块未闭;实验性修复=错位处插一 `}` +全量 schema 复验成功)烧 75.4s 重试本可免;citations 5→0=改道合法(§29.21 trace 证据走 E# 非源码引用)但 delta 零披露;「62次(31+34)」双 emit 全门放行 ×3 出厂+姊妹「65次(31+32)」,62 借 state_churn 切换数,同 payload typed fact=65——prose 算术对账**恒软**(嘈声红线,已在 RUN2FIX-C 件4)。
**自修复安全矩阵(裁定①答案,矩阵全文=工作流归档)**:三层线——**Tier1 字节/结构层**(内容字节零动+修后全量复验):大部已 shipped(transport 四形/string-unwrap/Path A-D);新增=缺 `}` 数组元素间车道(parse-error offset 精确字节+容器栈条件);**Tier2 typed 语境蕴含的元数据回填**(带受理摘要披露,词面单点):大部已 shipped 但四处静默施修需补披露词;窄扩三件=dims 去重先于 cap/单位后缀数→scalar_value 拆分(IsCountQuestion 保留)/kind 枚举字面折叠;origin 回填=仅 fact 自带 artifact 锚 dims 时(EMITBURN 件5 已窄形落地);**Tier3 内容授权**(名/值/quote/scope/searched_at/epistemic 枚举/grounding refs)**永不代修**——伪造=proof-lane 红线,烧轮改由一次性全违规 reject+上牙范例消解;修复不得改动受理判定本身(完成门权属);维度名 near-miss 恒 Tier3(相似度=嘈声,只措辞提示不驱动改写);挂钩=三既有咽喉扩展零新 pass,internal/types 保持 ctx-free 单一校验权威+调用序结构 pin;逐车道三臂 pin(正/一位突变/不动点)。
**◎ 定谳(裁定②)**:「方向未定/复合」尾节三喂类=A 诚实未解析 token(fail-open 设计)/B 词表漏(结构性空)/C **carriage bug**——×N 同类合并 adopts Rank/窗/板身份却不 adopt FixDirection,方向已 stamp 的席被搁浅;runnable_2 尾节唯一喂席 E9(tieba 自身 running 折算 16.684)引擎已 stamp 频率与热治理(差分证据:未合并 E15 佩修向词,合并 E9 不佩)=**错归类非诚实设备**,修 carriage 后本报告尾节清空(顺带治愈 UX-3 witness 实例);无 typed「复合」判别子,词拆分不可 ground。处置=OMGCLEAN-1 七件:件1 改名「其他方向」/"other directions"(elim.go:601/603+图例 tree.go:1782;综合方向被否=对诊断 token 假指);件2 ×N 合并 FixDirection 空位过继(仅取 rank 供席+typed 全员一致才 adopt,冲突留尾=宁漏勿假指);件3 头部承诺行 3→2(③句上收图例);件4 未入榜/旁栏统一语法区(=UX-17);件5 ◈ 瘦身 头+最大族+树指针(=UX-12② 裁决);件6 PARTSPLIT 5→1 指针行(恒等凭证句走明细双面);件7 板锚 typed 一致上提节头+⌗ 样板句上收图例(**类别词禁移=§29.155 直接反转禁区**)。目标 38→24 行,值/榜/序数零动,排除≠消失(计数指针),§29.112 闭合恒等式保持。**涉既裁位移五件单列**(§29.147 件F 三行头/§29.150④+§29.156 边前份在◎/§29.104.15 分道计数/§29.133 件G 逐行板锚/§29.147+UX-12② ◈ 双面),逐件 EVOLUTION 落地引原裁原文,零静默。
**处置序**:EMITBURN 批已交付(worktree 7c4cb9d03,含 §29.174 扩容件A-D:member_set 上牙+课转移 typed 载体+9-attempt witness+件5 窄形自修复;件D salvage 草稿=FILED 拒实施,权威界论证)→合并复核在飞(复核点+=member_set 拒文降级教唆是否已由件A 治愈);AUTOREPAIR-1 批(EMITBURN 合流后,同文件);OMGCLEAN-1 批(RUN2FIX-A 合流后,同文件);RUN2FIX-B 件3 收 F5 披露词面建议(「N 条运行时工件引用已改道为观测出处(E#),不作为源码引用入册」形)。
**§29.175.1 裁定②补记(2026-07-20 用户 verbatim)**:「窗内可消除量总览 里面 各个区域区分 缩进无逻辑,导致无法区分上下级关系。这里一定要清晰明了,不宜太多套话,反复挂无意义的套话解释。窗内可消除量总览 一定要精简且关键。」→OMGCLEAN-1 增设缩进文法硬约束:**两级制**——0 列=区/节头(◎题/⛓承诺/▸方向节/◇/—对账—/—另账—),2 空格=内容行(席行/对账行/披露行),**禁第三级**(一切续行/子句要么行内压缩要么迁明细);席行套话剥离闭集(⛓链上 区位自明/构成,见明细→构成 短记/墙钟席 默认不标 仅折算标/板锚 typed 一致上提节头);UX 预览稿先行送用户过目,形定后才实施。
**§29.175.2 裁定②补记二(2026-07-20 用户问「链上的"确定性优化"和"业务线索"是两个维度,如何体现?」)**:定谳=两维度口径——**确定性优化=值维度**(typed semantic_class 类别聚合:类校验席 E32=WebViewContentsClientAdapter 7.109+LacUtils 0.285=7.394ms,占序数、入自身工作量方向、有效归因墙钟、参与守恒);**业务线索=名维度**(业务 span 原文名族聚合,不占序数、族间不可加、凭证标注;doFrame/traversal/draw 只有名维度身份)。同段墙钟可双身份,粒度互不包含(席按类别并 span,◈ 按名分族)。现状 gap=◈ 面零 E# 互指(business_span_mention.go 仅 R3 凭证语义),读者无从知 ◈ VerifyClass 7.109 即 E32 席成员,有二次计数误读。体现方案(入 OMGCLEAN-1 件5 rider):**同质互指 chip 单向 ◈→席**——◈ 族行凡与语义席同段(精确 typed 匹配键=宿主线程+span 名 verbatim+行锚重叠;不确定不佩=宁漏勿假指)佩 `=[E#]成员`;图例补「◈=业务名维度;与席同段时佩 =[E#],不与席值叠加」;席行/值通道零动(值维度权威面不佩名维度记号,席→span 实质已在明细 成员(span原文) 行);确定性优化点表既有 E# 指向保持。
**§29.175.3 裁定②补记三(2026-07-20 用户 verbatim)**:「链上的 "确定性优化" 是参与排序和提及义务,链上的 业务线索" 不参与排序,但是有提及义务」→两维度义务矩阵定谳:**确定性优化(值维度)=排序✓+提及✓**(语义席占序数入方向守恒——既有;优化点表无条件入正文=C4 既裁,保持);**业务线索(名维度)=排序✗+提及✓**(不占序数维持;提及从「可提及」升格为**义务**)。义务面=确定性板面:树 ◈ 全名册对**全部已准入族(typed on-chain∧双分量地板,既裁门)零裁剪逐族列出**——现状 BusinessSpanMentionFamilyCap=3 顶帽(SPANTOP-1)把地板上族裁成「另有 N 个 span 族(≥显著地板)未列出」计数行=义务违背,废除顶帽(地板=唯一噪音阀;若保安全上限须 CAPFIX 式带值带主体披露);完备 pin=admitted==rendered(census 同构);◎ 面保持瘦形(最大族+总族数+全名册指针,义务由 roster 面履行);prose 义务不对称维持=优化点入正文(C4)、业务线索 prose 零新义务(§29.42.4 权属,义务在板面不在模型)。入 OMGCLEAN-1 件5 rider2。
**§29.175.4 裁定②补记四(2026-07-20 用户问「边前/边后是唤醒边么?属于哪个维度?」)**:定谳=①是,typed 唤醒边(sched_wakeup;该例凭证形=R3 直接裸边 34579.555890s);边前份=最晚相关凭证边之前的状态段份额(因果有效,可构成对目标的延迟传导),边后份=边之后(唤醒已发生,因果解除),链上只计边前份。②维度归属:**不属于两个内容维度**——它是凭证/记账层,与值维度(确定性优化席)/名维度(业务线索)**正交且共用**(◈ 行「凭证:唤醒边凭证(边前)」同一套边凭证规则;SPANVIS host-edge 整段边前门同源);该披露行本身=对账披露非席(不占序数;该席整体未入榜账在候选池;R4拒转=不因边前份高拆子席入榜,席位铸造不被披露驱动),预览稿归「另账」区正确(对账区=已发布账互账,另账区=未发布/异口径账)。③词面升级(入 OMGCLEAN-1 件6):◎ 行「边前/边后」→「唤醒边前/边后」自明形,完整语义在图例。
**§29.175.5 裁定②补记五(2026-07-20 用户 verbatim)**:「链上的 "确定性优化" 是之前确定过的 verifyclass,JIT,shader,GC,等等固定几个。链上的 "业务线索" 是除了这些确定性优化的 业务自身span,的最长,最频繁(合计后最长),TOP 3?起到引导用户关注这些是否从业务角度可以减少时间占用的目的。」→内容边界定谳:**两维度按内容互斥**——确定性优化=typed semantic_class 闭集(VerifyClass/JIT/shader/GC/纹理上传/运行时编译…既定固定枚举),走席位+优化点表;**业务线索=semantic_class 之外的业务自身 span**(排除键=typed semantic_class 分类命中,精确信号),选择规则=单次最长 TOP3 ∪ 合计最长(最频繁加总)TOP3 去重(典型 3-5 行),面目的词=引导业务自查减时。**连锁修订**:①§29.175.2 互指 chip **退役**(维度按内容互斥后 VerifyClass 不再入 ◈,双身份二次计数问题从构造上消灭,无需互指);②§29.175.3 提及义务履行面收敛为该 TOP 选择集(选择集内零裁剪=义务;选择集外尾部保留「另有 N 族(≥显著地板)」计数披露;既裁双分量地板门与 on-chain 门零动);③cap 语义改造:BusinessSpanMentionFamilyCap 从「TotalMs TOP3 含语义类」→「非语义类 ∪ 双准则 TOP3」,选择规则成为承诺面词(写进 ◈ 头/图例);④锚 span(doFrame 类)不特判——业务自查减时对锚自身同样成立(traversal/draw=其子相,均正当),留观察。runnable_2 复放预期:VerifyClass 出 ◈(留 E32 席+优化点表),◈=doFrame 4.473/traversal 合计2.833/draw 合计1.803(+尾部计数)。

## §29.176 EMITBURN-1 收账(2026-07-20;§29.173 五件+§29.174 扩容件A-C 落地,件D FILED 拒实施;合并复核 SHIP 零 P0-P2)
**交付(017788084,12 文件 +954/−98,零校验语义变化)**:件1 错误累积=共享 violation sink 双模式(firstOnly 逐字节复放历史串行门;accumulate 走全臂),一条 reject 列全违规(cap10+计数披露,headline 恒=串行首错=构造保证);件2 scope/searched_at 补完整最小示例+inner decode 拒列合法字段表(schema 反射非手抄);件3 near-miss 改名提示(闭集识别名+缺臂双条件,不加 alias);件4 反删除总则全拒面+「remove entirely/omit the fact」全数降末选+通用 unknown-field hint 修先删后;件5 自修复窄形=仅 fact 自带 artifact_id/trace_window 锚维时注入 origin(混跑洗白臂三 pin 堵死,run 级注入拒);件A member_set 上牙(schema 教「逐名+value=len」拒前可见)+拒文降级教唆治愈(members[] 逐名先行,scalar_value 降格 prose-only 后置——§29.175 定损新病同批灭);件B 跨子节课转移(typed 载体 TaughtSchemaLessons,retry hint 逐字重放入后续 dispatch,dedupe 防双教);件C runnable_2 九态 witness 重放 pin(四拒+三拒+双收,受理侧零语义 pin)。件D salvage 草稿=FILED 拒实施(被拒内容跨受理边界=第二未验证载体,权威界论证)。
**复核(SHIP)**:六对抗臂全过(串行等价/headline 字节合同/课转移零内部词+插入序/窄形排除完备/降级教唆治愈实证/反删除三臂抽查);三突变 M1-M3 全红后 cp 恢复字节恒等;门=gofmt/make/五包 -count=1/L1 pin/全套 83 包 全 EXIT=0(门卫生自纠:突变窗禁并发构建重跑+管道尾吞 EXIT 重取)。**P3 备案三条**:SectionAnswerCoverageNotes 缺 canonicalUserSectionOrder(既有非本批)/cap 臂 merge 建议与反删除句轻微语感张力/纯语法错拒也附反删除句(无害冗余)。**烧轮预期**:同 payload 串行 4 轮→1 轮;member_set 同课两教→一教全程带。AUTOREPAIR-1(§29.175 矩阵)就绪可派。
**§29.175.6 裁定②补记六(2026-07-20 用户 verbatim)**:「当前 "窗内可消除量总览" 的逻辑还可以优化,区域清晰,最重要的 链上修复方向 可消除量(含链上确定性优化)> 业务线索(除了这些确定性优化的其它span)(单独区域或项目,不要在一行显示,可以多行 TOP 3?)> 邻近 > 背景 > 其它辅助信息等」→◎ 区域序定谳(取代 G5 五区序):**①⛓ 修复方向节区(含链上确定性优化=自身工作量语义席,现状归属确认)→②◈ 业务线索独立区(多行 TOP3 非单行压缩——§29.175.5 rider3 的「◎ 单行最大族+指针」形被本裁取代:◎ 区即选择集逐族多行(值·线程·span·次数,无 bar=名维度视觉区分),行号/凭证细节留树 ◈ 块,尾部计数指树)→③◇ 邻近→④▒ 背景(单行摘要+最大成员点名+指针)→⑤辅助区(∩ 对账/守恒/未入榜计数/唤醒边前份/⌗ 口径旁栏)**。OMGCLEAN-1 件4/件5 随改。

## §29.177 RUN2FIX-C 收账(2026-07-20;§29.174 处置④ 成文教学四件全软;合并复核 SHIP-WITH-FIXES 两现场修收编,零 P0/P1)
**交付(57c98f913,3 文件 +296/−3,全部 internal/skill 教学+pin)**:件1 用户点名端到端量 duty(机械图定谳:119.320ms 帧锚引擎零 typed 载体,只活探索期 thinking,checkpoint :41/:52 双丢,PSG-1 围栏下丢失原为「合规」——修=成文 duty 句「点名对象端到端量领句或如实披露缺哪个边界,禁编造禁邻近数顶替」+explore 侧携带句「定位到边界事件即时间戳 verbatim 入 emit 证据+完成 reason」,双向闭环;真 typed 帧锚事实=候选立案留 tracequery 战场);件2 读者词面 duty(射程勘定三因:STYLE-1=填充语词表不管 wire 词/TRACE VALUE WORDS 只挂 explore/板摘要 verbatim 喂 k=v+Fact lane 教逐字抄=泄漏结构成因——修=「数据字段原名非读者词,正文用板面已发布中文词,字段名仅作证据行引用键」+事实围栏 carve);件3 四层骨架(量化结论领句→自身分解→TOP 可消除≤5 行带 [E#]→其余指权威面,正文禁逐席复读+长墙拆短句);件4 总-分对账教学①(总数=分量精确和;往来数取 wakeup census 禁借 state_churn 切换数;witness 62=31+34 反例入文)+②contract 臂裁定不做(witness 三形全自由散文,精确 regex 零命中;CR-4 已裁 NL 解析退役;机械臂正道=引擎把双向 census 和作 typed named-fact 直喂,委托点)。
**复核(SHIP-WITH-FIXES 两修收编)**:P2-1 件3① 无条件量化开头=编造压力残口→header 交叉引用件1(量未立时诚实缺口句顶位,SST 惯例);P3-1 pin 不对称补 OnViolation==0 双断言;P3-2/P3-3 备案(件4×TVW capped 计数当量边缘稀有不 carve/件1 反例为行为描述形)。ATOMIC 7 条全过(R3 五段零内部词——per-pair census/state-churn 均为已发布面先例);四突变臂全红 cp 恢复恒等;门 gofmt/make/五包 全 EXIT=0。TierB 计数 pin 26→30 lockstep。
**§29.175.7 裁定②补记七(2026-07-20 用户 verbatim)**:「▒ 背景 也可以列top 3,多行展示」→▒ 区从单行摘要改多行 TOP3(窗内投影值降序,尾部「另有 N 行见背景段」计数);行形同 ◈ 紧凑制(值·线程·状态/span 名·短记,实际超窗值行内短形);**bar 视觉规则定形:bar=可消除量维度专属(⛓/◇ 佩),◈/▒ 名维度与背景语境不佩**。OMGCLEAN-1 件4 随改。
**§29.175.8 裁定②补记八(2026-07-20 用户「— 辅助 等UX也设计一下,当前看起来有点凌乱,信息逻辑不清晰」)**:辅助区定形=**两列文法**——标签列(固定宽,词面闭集:∩ 重叠对/守恒/未入榜/边前份/⌗ 口径旁栏,家族有既裁记号者记号随标签)+内容列(值在首段,说明压缩为一短句,完整语义图例);**组序=对账组先(重叠对/守恒=验证上区数值)、另账组后(未入榜/边前份/口径旁栏=上区之外的账)**,区头「对账与另账」宣告两组;条件族(represented/subset/self-symptom/semantic census 等)出现时同两列文法入另账组。OMGCLEAN-1 件4/件6 随改(件9 立)。
**§29.175.9 裁定②补记九(2026-07-20 用户 verbatim)**:「◇ 邻近 也同样,多行 TOP 3」→◇ 区=多行 TOP3(条件可消上界值降序)+尾部「另有 N 行见明细」;至此三个非方向区(◈/▒/◇)统一 TOP3 多行制;◇ 佩 bar(可消除量维度);**bar 归一随裁定形=各区 TOP1 满格**(五区制下全局归一使 ◇/小值区 bar 恒近空,承诺词「满格=本区TOP1」的「本区」明确为各区,词面与行为同批同改);辅助区「未入榜」◇ 计数口径随 TOP3 展示调整(展示外全额入明细计数)。OMGCLEAN-1 件4 随改。
**§29.175.10 裁定②补记十(2026-07-20 用户问「唤醒边前份…突然多出来这一行,是想说明什么?」+主会话推荐获默认采纳)**:定谳=该行为候选池最大「似该入榜而未入榜」账的审计预答(R4 边凭证不拆席),但独立行设计失败(主体 ◎ 内零出现+先机制后目的)——处置=**取消独立行,并入「未入榜」行作最大成员点名**:「未入榜 ⛓ N 行 · ◇ M 行 · 候选池最大 <主体> <值>ms(带边凭证,R4 不拆席),见明细」;完整边前/边后恒等式沉明细双面(§29.156 既有);演化 §29.150④/§29.156 的 ◎ 内披露块定位,EVOLUTION 落地引原裁。
**§29.175.11 裁定②补记十一(2026-07-20 用户 verbatim)**:「"∩ 重叠对","守恒" 等,如果是同级别的列表,前面 加 个圆点 是否更好?」→采纳:辅助区同级列表行一律 `· ` 起头(回归引擎脚注行既有习语);行首标记三分规则定形=**值行以右对齐值起头(⛓/◈/◇/▒ 各区)、辅助列表行以 · 起头、尾注行(另有 N…)缩进无点**。OMGCLEAN-1 件9 随改。
**§29.175.12 裁定②补记十二(2026-07-20 用户 verbatim)**:「"但 R4 规则(边=凭证,不可拆席转正)" 是内部术语,客户不感知,措辞需要优化。」→定谳=**用户可见面(◎/树/明细/图例)零规则编号零内部车道词**:R3/R4 等编号与「候选池」「拒转」「席值/车道/序数零动」类内部话语全数换白话闭集——R4拒转/整席不拆→「按口径不拆段入榜」;R3 边凭证→「直接唤醒边凭证」;候选池最大→「未入榜最大」;规则编号只存内部账本/代码/测试,图例用白话解释规则本身不引编号(=UX-15/§29.156 备案正式转裁定落地);词面单点+替换词表闭集。OMGCLEAN-1 件6/件9 随改+全面 sweep 臂。
**§29.175.13 裁定②补记十三(2026-07-20 用户 verbatim)**:「⌗ 口径旁栏…还带图标,感觉像是高亮提醒,需要降级,这行仅仅是非关键的辅助信息,都应该降噪显示」→定谳=辅助区标签列**图标策略**:仅保留功能性互指记号(∩——与席行 ∩[E#] chip 互指定位),类别装饰记号(⌗)在 ◎ 辅助行剥离,标签纯词「口径旁栏」;树/明细的 ⌗ 席行面不变(那里是口径旁栏本体);非关键辅助信息全线降噪原则入图例词条。OMGCLEAN-1 件9 随改(⌗ 剥离臂+∩ 保留臂)。
**§29.175.14 裁定②补记十四(2026-07-20 用户「未入榜行太长,是否可以拆成两行?」)**:采纳=拆两条同级列表行(不引入续行层级,守两级缩进制):「· 未入榜 ⛓ N 行 · ◇ M 行,见明细」+「· 未入榜最大 <主体> <值>ms · 有唤醒凭证,按口径不拆段入榜」;标签列闭集加「未入榜最大」;辅助行长度纪律=单行超预算优先拆同级行而非折行续行。OMGCLEAN-1 件9 随改。
**§29.175.15 裁定②补记十五(2026-07-20 用户问「调度延迟(5段)和 runnable 有区别吗?为啥两种表达形式?」)**:定谳=两级词制(typelabels.go 闭集单点):判词(scheduler_latency→调度延迟/cpu_pressure→CPU竞争压力/priority_inversion_runnable_wait 等=引擎已细分成因)vs 素状态词(runnable_wait→runnable/sleep_wait→sleep=成因未细分,状态原文诚实形,内核状态词 zh-en 同词既裁);区别真实但同板并排读者不可感知=UX gap。处置(OMGCLEAN-1 件11)=◎/榜面素状态席补短标「(成因未细分)」(词面单点,判词席零动),图例补两级词制双句;树面「深度未解析」限定词体系不动;pin=素状态席带标臂+判词席无标负臂。
**§29.175.16 裁定②补记十六(2026-07-20 用户「runnable 自身就有调度延迟的含义吧?只是没附加优先级反转等细化判词」)**:采纳,修正 §29.175.15 框架——registry 实证 runnable_wait 与 scheduler_latency 同车道(SchedulingDemand)同修向(SchedulingSupply)同折叠族=近同义,「判词 vs 素状态」两级制对 runnable 族不成立(对 sleep 族成立:sleep 无泛义诊断读法);真细化=另 token 另修向(priority_inversion_runnable_wait→锁与优先级/cpu_pressure)。处置(OMGCLEAN-1 件11 修订):◎ 诊断面 runnable 族词面统一「调度延迟」,未细分席带「(成因未细分)」;素状态词 runnable 保留树状态面/明细/state_churn(zh-en 同词裁定射程不变);registry token 零动仅显示词映射(supply_pressure 显示层分离先例,§7.2.1 红线合规);pin=◎ 调度供给节 runnable 席词面统一臂+树状态面 runnable 原词臂+sleep 不适用负臂。
**§29.175.17 裁定②补记十七(2026-07-20 用户「重新梳理选择最优判词,不要看起来像同一个事情两种表达」)**:判词文法定谳=**一族一主判词词根,细化用 ·限定 后缀永不换词根;素状态词全退 ◎ 诊断面**(树状态面/state_churn/明细保留)。全表:调度供给族=调度延迟/·碎片化/·CPU竞争(scheduler_latency+runnable_wait 统一,fragmented_runnable_wait/cpu_pressure 归根);锁与优先级族=优先级反转·* 维持(本就合文法);IO与依赖族=IO阻塞/·不可中断(原因未证)/·设备延迟(io_wait/d_state_or_io_wait/io_latency 归根,新铸词根待用户扫一眼);binder等待 维持;频率与热治理族=低频运行·折算(running 折算席,复用既有词根);自身工作量语义类维持。连带:§29.175.15/.16 的「(成因未细分)」括注撤销(文法自完成:裸主判词=未细分);全部为 typelabel ◎ 消费映射,registry 零动。OMGCLEAN-1 件11 终版随改。

## §29.178 RUN2FIX-B 收账(2026-07-20;§29.174 处置③ 过程面四件+件5 备案;合并复核 SHIP-WITH-FIXES,P1×1+P2×2 现场修收编,零 REJECT)
**交付(335187b87,16 文件 +1054/−95)**:件1 dock 计数三丢失机制修根(隐藏 probe 段整段蒸发=emitNodeStart 无行可记/resume 轮次绝对覆写回退/「1 阶段」只数非 node stage 行)——iterTotal 跨段累计+Renderer 级 stray 计数+footer runSummaryTotalsLocked(阶段=显示槽位去重与 ✓N/K 同源 stageSlotForKey)+本-timer 跨行聚合收窄同 nodeKind;witness footer 16 调用/2 轮/1 阶段→~25-29(ToolOK 基准)/~21 轮/4 阶段;旧假 pin(镜像循环不调产码)重写为真函数调用。件2 心跳注解形(恰界严格大于,40s/40s 旧形字节稳)+llm 地板 WARN(cap<180s∧推理族名单,仅一条 WARN 零行为);件3 引用 delta 披露(「N 条提交→M 条入册:运行时工件引用改走 E# 证据索引」;token 拼写避 citations= 贪婪正则;delta=0 字节稳;drop 四 call site 带因 DEBUG);件4 route_tools 过滤(typed 双腿门=HasTraceArtifact∧ExcludesCurrentSource)+reason 词 closure_incomplete(闭合已受理时 proof_weak 名不副实;自由字符串车道零 R2';loopkernel shadow 镜像面刻意保原词=不伪造内核状态)。件5 malformed fixture 备案成立(原始字节未落盘,fixture=猜形)→留证委托点已收编 AUTOREPAIR-1 件5。
**复核(SHIP-WITH-FIXES 三修收编)**:P1=AllowedTools 滤除打进执行面硬门(validateExplorerReadDispatchPolicyToolCall 对列外工具执行拒绝→trace-only 续派窗 read_file 被硬拦=封 §29.121 blob 钻取+§29.19 有益回探,违「全部=菜单/披露面」纪律)→Option C:执行面只 admit trace_query 不减,建议面(ToolSuggestions/PreferredTools/route_tools)全族滤除,policy_allowed_tools 披露如实;P2=心跳注解单机制断言失实(首字节早到的长生成同越 cap)→双机制词面「保活重置或首字节已到,未判超时」;P2 测试缺口=∧ 门第二腿无负臂→补 GateNeedsBothTypedLegs+MUT4 咬合。**P3 备案四条**:件3 comment 失实(persist 链可再铸 citation,对齐需触禁触文件→移交后续批)/工具计数 ToolOK 基准口径注/log_triage 段槽位=0 不计=承诺面一致化待追认/hitraceconv 并行负载环境病(隔离复跑×2 绿,另立卫生案候选)。四突变 MUT1-4 全咬 cp 恢复零差;门 gofmt/make/七包/全套 83 包 全 EXIT=0。

## §29.179 RUN2FIX-A 收账(2026-07-20;§29.174 处置② P1 显示六件;旗舰双复核=对抗+冷读双 SHIP-WITH-FIXES,修复轮 P1×1+P2×3+P3×7 全收编)
**交付(769976057,17 文件 +1070/−44)**:件1 节序承诺如实化(zh「方向未定/复合恒末,余按节内最大可消降序」/en 两行制;runnable_2 板活证=未定30.067>具名25.149 仍恒末;OMGCLEAN 改名时随词);件2 折叠行2 带值披露(typed MergedMaxSubject/StateKind 双 wire 键,宁漏勿假空清;R3 背景折叠同构;witness 47.282 藏匿→「成员最大 CookieMonsterCl-59843(见榜位#1) · sleep 47.282ms」;②折叠准入值优先+wire-fold 族(E19 类需新 note 键 R2')→**A2 委托**);件3 E48 空名根修(语义 span 臂空名回落 host+前门「(未命名因果节点)/(unnamed causal node)」防御;**pin-only 验证=carve 不复现客户空名,A/B 缺席挂全 trace 回访**);件4 截断反转(6-cell 名头地板+先截状态短语+B5 marker 门+RCM-2 D2 家族词恒整;三怪形 base 复现 final 全灭);件5 导语尺注(两把尺 |symptom−四态等待和|>0.5ms 同行括注;三旗舰实弹;149.263 carve 不可铸=LEAD-1 挂回访);件6 E1 ❶ 补洞(Self 反转席五条件臂,选举/加冕零动)+标题括注「(锚点板#1/主榜席;◎ 按具名节最大)」。
**双复核修复轮收编**:P1-F1=**§29.30.1 徽章单门 EVOLUTION 记录**——§29.174 用户令 ❶ 对称=badge 面例外臂(佩章≠加冕,选举单门零动,crown-follow 委托 A2);tree.go self-臂旧句「佩❶即重开裂缝」对 badge 面射程明文废止,引 §29.30.1 原文。P2-F3/CR-1=折叠归属裸 HasPrefix 假披露(app-951/9511 形错点名)→指针后缀剥离+canonical 全等+碰撞负臂;P2-F2=括注词面对齐触发器(「全板节最大」谎→「具名节最大」);P2-F4/CR-2=en dump 空心根修(投影读 AnswerContract.Language 非 lang 参数,测试 helper 注入;p3measure en A/B 升真 en 字节副产品);P3 七件(jitter 别名非 B-1 借用注/fold-twin 残余记/FirstSectionTop 两角落记/佩章 pin 种群收窄/两静默 pin 补演化注/en 占位词三面同改/CR-7 括注取数==◎首节头一致性 pin)。
**门**:双官+修复轮三跑 gofmt/make/五包/全套 83 包全 EXIT=0;突变 3+3+逐修复臂全咬 cp 恢复零差;五板 zh+en(真 en)A/B dump 归因=仅本批行族。**偏离备案**:types/trace_causal_projection{,_aggregate}.go=战场扩展(折叠构造器实际所在,禁触 answer_aggregate_fact.go 零触);CR-4 en 占位词三面同改超批内单点(词面单点纪律优先);F7(a) all-unresolved 角=已接受 wording corner 如实记。**A2 委托清单**:crown-follow/折叠准入值优先/wire-fold 族 max 披露/fold-twin badge 残余。

## §29.180 AUTOREPAIR-1 收账(2026-07-20;§29.175 用户裁定①落地=系统安全自修复分层;合并复核 SHIP-WITH-FIXES,P2-1 现场修收编,零 P0/P1)
**边界声明(随 commit 首段,矩阵 REDLINE 原文)**:系统可 (a) 重编码所给字节不改任何内容字节(Tier1,复验门控)、(b) 回填由模型 typed 字段或闭合 typed 运行门蕴含的元数据槽并在受理摘要单点披露(Tier2);系统永不作者化 claim 内容——名/非成员派生值/quote/scope·时间戳断言/epistemic 枚举/grounding refs(Tier3)——其烧轮只由一次性全违规拒+完整最小示例+确定性改名提示+反删除文案消解;任何修复不得改动 fatal/non-fatal 受理判定本身(完成门权属)。
**交付(c6c5765f2+39c6609fe,9+2 文件)**:件1 缺 `}` transport 车道(pattern 5:offset 精确定位+四条件容器栈判定+≤8 轮单调收敛+插入后全量 re-parse,复验不过返原字节=legacy reject 字节兼容;witness L272 3994 字节 byte-identical fixture 修复成功实测;八形攻击全过=合法 JSON/字符串 decoy/混合缺陷 abandon);件2 四静默 Tier2 施修补披露(「已补注 %s=%s(由 %s 推导)」词面三常量单点;err 路径组合合同=施修注+全违规清单并载);件3 dims 去重先于 cap/单位后缀拆分(IsCountQuestion+单位冲突双守卫;**attempt 4 载 119.320ms 从拒翻披露式受理=裁定行为**,carve-out 双突变臂)/kind 字面折叠(固定词表零相似度);件4 三结构 pin(agent pattern 序+tool normalize 先于校验门+全违规读修后残差);件5 malformed 有界留证(2KB+offset 邻域 256B,脱敏先于切片,2KB 跨界攻击零泄漏)。
**复核收编**:P2-1=数值前缀边界谓词两处手抄→types.AggregateValueNumericPrefix 单点(五表手抄教训贯彻)。**P3 备案三条**:①Unit:="matches" 回填仍静默(矩阵闭集未列,新增第四披露词面待裁);②differ 在 validator identity-dedup 掉重 fact 时 len 守卫全静默(方向安全零错注,稀有形欠披露);③fold+split 不复合(Grouped_Count+"10.5ms" 形仍烧一轮,窄安全)。**偏离采纳**:插 `}` 于逗号前=矩阵措辞字节等价;types 无 DEBUG log=ctx-free 纪律优先;result_count 注源 token=value 槽(typed 准确超矩阵枚举,披露侧扩展)。门三跑全 EXIT=0;突变 M1-M3 咬合 cp 零差。

## §29.181 用户裁定集三(2026-07-20;裁定池七件 dossier 逐裁,全按推荐)
**用户 verbatim**:「① 树面套话上收 --- 按最优方案(推荐)来。② 记号瘦身:序数双载 --- 最优方案(推荐=不追冠) 来。④ Unit:="matches" 静默回填要不要披露(小) --- 按 最优方案(推荐) 来。⑤ footer 阶段计数口径追认(小) --- 按最优方案 来。⑥ ε-overlap 披露启用 : 按最优方案(推荐) 来。⑦ tracediag 调试面豁免(极小,解释性) : 按 最优方案(推荐):来。」(②行括注携带③件推荐词「不追冠」=②③并裁;与委托默认一致。)
**裁定落位**:①树面套话上收=不变式句上收图例单点(跨板不可相加/边锚定段/同值窗一次声明),行级留短记号(⇄另板[E#]/边锚)+行内独有信息保留——涉既裁承诺面逐件 EVOLUTION;②序数单载=佩章席行2 不复读「根因排序#N」,未佩章有序数行(fold-twin 残余)保留词形兜底,图例补「❶..❺ 按板各发,该板 TOP5」;③**crown-follow 关案=不追冠**(维持 §29.30.1 选举单门,标题=选举权威+A 批括注桥接,徽章=板内值序;A2 委托项销,只补图例句);④Unit:="matches" 回填补披露注(「凡施修必披露」零例外,AUTOREPAIR P3-① 销);⑤footer 阶段计数=追认现行为(与 ✓ 梯同源同数,B 批 P3-③ 销);⑥ε-overlap 披露道启用(§29.150⑥ 钩子条件成熟=DHMINE 七活体):重叠<独立地板常数(禁借 INTERFLOOR)→席位保留值零动,佩「ε 重叠」披露记号+明细披露重叠值,非致命不硬拦;⑦tracediag 调试 dump 豁免于 P3MEASURE「任何渲染面不可见」射程(SelfGapSemanticOverlaps 先例,零代码,§29.171 备案销)。
**排批**:RULE3-1(候 OMGCLEAN-1 合流,战场 tree.go/图例/tracequery/emit 披露)=件①②③图例句⑥④;⑤⑦=纯落账即关。

## §29.182 用户裁定集四(2026-07-21;OMGCLEAN/B 复核带出四件逐裁)
**用户 verbatim**:「① 「已证非IO」的 D-state 席裸词残例 : 按 最优方案(推荐) 来。② EN 面维持族判词还是裸 wire token : 按 最优方案(推荐) 来。③ ∩ 重叠对出现两对时的行形 :允许多行,保持 渲染两条几乎同文的行,重复 但格式统一,UX更好看。④ 进度 ✓ 行「本」计时器的子题聚合口径(追认件) : 最优方案(推荐=追认维持) 来」
**裁定落位**:①已证非IO D 席铸独立判词**「不可中断等待·非IO已证」**(独立词根合文法=另一族病;榜面零裸态词,图例承诺变真;EN 同批)→RULE3-1 件7;②维持族判词 EN 化("priority inversion (candidate)"/"priority-inversion runnable wait"/"page-cache churn" 等,◎/树判词面用词,wire token 留证据引用键位)→RULE3-1 件8;③∩ 多对=**每对一行完整句,重复合法,格式统一优先**(「同上」省略形被否;OMGCLEAN 修复轮既定形即终形,零新工作);④「本」子题聚合口径追认维持(B 批 P3 偏离1 销;「本(含子题)」标注不加)。

## §29.183 同事审计核验定谳(2026-07-21;trace_analysis_current_implementation_audit_20260721.md 四路双向核验=代码+裁定账本)
**总判词**:代码事实栏高可信(15 项抽查 14 对 1 错:§4.2 条件3 把 census 层「wakeup 全无才回退 waking」误置于铸边车道——铸边实为两类型同池取最新 query.go:24256-24301);**系统性弱点在裁定侧**:方法论自称「不把历史方案当现行规则」,导致把 adjudicated design 误判为无主启发式——其 P0 三条中 G1/G4 全部、G2 半张会推翻用户既裁。
**逐条定谳**:G1 周期折算=确定性算法(固定常数纯算术零随机)非相似度启发式,既裁三层(VS-1 §7.8 客户口径/§29.27① 参赛口径/§29.19④ 保席+P3MEASURE §29.171 复用为族① typed 判据),降 advisory=禁重诉区→不修,采恰界 pin;G2 收窄为真残口=**◎ 面图例「已证可消除量」承诺覆盖既裁无区间凭证成员而 ◎ 席行不佩凭证级词**(树面已佩「身份继承(链窗级)」「(包络级凭证)」typed 词,◎ 面没有;准入收紧/降道方向=推翻 §29.104 终判③/§29.134/§29.61.2 禁区)→P1 词面修=◎ 席行补凭证级 chip(加法披露,不改 GREENLIT 基石 B 句),入 RULE3-1;G3=§29.169/.171 裁定原文重述→不修,三列命名法+exact-edge 白名单两增量入阶段二裁定议程;G4 near 折叠=精确 identity 键+双真身份门+披露道(审计漏报),软化重开 PTV6 批②#4 已裁 138% 幻影→不修,采 3.0% 恰界 pin;G5 via OnChain=实锤(成员判定 vs 单调路径分离,complete 只活散文),但翻转 OnChain=推翻 RN-14 判语→修=加法 typed path_complete+schema 第三态句(P2);G6=既裁保守+edge3be_eval 3b 已实测缓办(0.118ms 收益 vs 400-700 LOC)→不修在案;G7=2026-07-18 候选域钉死裁定→不修;G8 时间戳 0 sentinel=**真 bug 且面比审计更广**(11+ 处:WindowDurationMS/锚窗三解析器/IntervalValid 20 消费点/elim 算术梯/树面四门;[0,end] 合法 trace 丢锚+丢百分比+拒小计+假「未采集」词)→修=共享谓词「end>start∧start>=0」零 wire 改(P2,候 OMGCLEAN 合流);G9=引句非实际词面+漏三层既有诚实层,残口=守恒图例未声明种群范围→图例补种群句(P2,入 RULE3-1);G10=双位置空间为 §29.36.2/3+§6.4 既裁构造且拆词已实装(RANKDIS-EXT rank_channel/禁裸#N)→不修;G11/G12=注释漂移实锤→修(P3);G13=误读既裁(INTERFLOOR §29.150③ 落地+§29.160②/§29.162 追认;阈值只控披露已是现实)→残口=:78「待追认」注释未更新,单行修。**§12 八测**:直接采纳 0;随修采纳 3(G5 批)/6(G8 批)/7(守恒种群句);须裁 1/4;冗余 2/5/8(既有 pin:NearTierBandBoundary/P3MeasureAdvisoryOnlyConsumerAbsence/rank_channel_word)。**§14 对外口径**四句与既裁同向,缺名维度区域澄清句(§29.175.5/.6/.7 在飞)+时效注。
**排批(节奏)**:AUDITFIX-A(即派,engine 注释+纯测试,零显示冲突)=G11/G12/G13 注释三修+G1 带上沿恰界 pin+G4 3.0% 恰界 pin+query.go:23483 caveat 词面;AUDITFIX-B(即派,banner 面非 ◎)=G5 typed path_complete+schema 第三态+§12-3 测试;AUDITFIX-C(候 OMGCLEAN 合流,涉 ◎ golden)=G8 谓词统一+§12-6 timestamp-zero fixture;RULE3-1 增件=G2 ◎ 凭证级 chip(件9)+G9 守恒图例种群句(件10);裁定议程记档=G3 阶段二两增量/G6 3b 缓办已在案/G1、G4 重审需显式推翻。文档勘误清单回传同事(§4.2 条件3/§5.4「2/3 gap」→carve 后 interval/§8.2 semantic 切数已被通道线取代/「已证明可消除」字面实为「已证可消除量」+出处/引原裁 §/时效注)。

## §29.184 OMGCLEAN-1 收账(2026-07-21;§29.175 裁定②全族 .1-.17 落地=战役最大显示批;旗舰双复核双 SHIP-WITH-FIXES,修复轮 14 项全收编)
**交付(9847fa74b+ee65c1c5b,30+24 文件)**:◎ 五区制(⛓ 方向节→◈ 业务线索 TOP3 独立区→◇ TOP3→▒ TOP3→— 辅助 · 对账与另账 — 两列文法)、「其他方向」改名+件2 carriage 根修(×N 合并 FixDirection 空位过继=仅 rank 供席+typed 全员一致冲突留尾;五板尾节全清空,E18 30.067 迁入频率与热治理)、判词文法(调度延迟族统一/IO阻塞族/低频运行·折算,素状态词退 ◎)、两级缩进+行首三分制、bar=各区 TOP1 满格(承诺词同改)、零内部术语 sweep(R3/R4/候选池/拒转/§ 渲染字面 grep 闭合 pin)、板锚 typed 一致上提、涉既裁位移五件 EVOLUTION 全引原裁。**修复轮收编(双官 P1×4+P2×5+P3 若干)**:辅助区头定稿逐字/未入榜最大=AccountMS+行内恒等括注「(唤醒边前 X + 边后 Y)」(未发布席恒等式回归用户面;zh 尾「有唤醒凭证,」因 106>100 禁续行让位,词义由括注承载=偏离备案)/EN 辅助区续行清零(白名单臂删,禁续行 pin 真咬)/◈ 尾死指针改双面诚实「未列出」计数(§29.175.6 尾词 authority artifact EVOLUTION)/图例 § 自铸剔除+禁词表扩 §·拒转/∩·守恒行定稿逐字(双 ∩ 对=两同级完整句=§29.182③ 终形)/◈ 图例区头题对齐+区头单行/▒ 退化形合一行/件2 活体 pin(真产线 compile 路,no-op 过继块 tool 包必红)/strip-then-DeepEqual 双守卫/◈ 截断撞脸触发式区分头(B5b 既裁 pin 保持)/计数当量空格单源/EN 复数瑕。**门**:实施+复核+修复三轮 gofmt/make/五包/全套 83 包全 EXIT=0;十份 zh/en dump 三代对照全行归因;值通道零 diff。**行数**:39→35 非空行(G5 目标 24 被 §29.175.7/.9 TOP3 多行裁定合法取代)。**不动清单**:CR7 非IO D-state 词/EN 维持族 token(§29.182 已裁→RULE3-1 件7/件8)/车道降道白话扩案(RULE3-1 件6)。**B 批 P3 移交项**(citation comment 失实)本批未触=仍在案。

## §29.185 用户裁定集五(2026-07-21;七处既裁维持+防飘逸注释令+G3 榜语义定谳+◈ TOP5 扩容)
**用户 verbatim**:G1/G2/G4/G6+G7/G10/G13 各「按更优做法裁定,加注释或说明避免飘逸」;G3「"若 UI-100 醒后还要等下一拍 VSync," 可以不过度考虑,能优化的就是可以优化的尽量提醒用户去优化,按更优做法裁定,加注释或说明避免飘逸」;「"业务线索" 当前的TOP 3 需要扩展为 top 5,因为top 3 覆盖面大概率被无关的 "Choreographer#doFrame " 占榜,没办法暴露真实需要客户关注的信息。」
**裁定落位**:①七处既裁全部**维持并加防飘逸注释**(DRIFTGUARD sweep=RULE3-1 件12):八个争议位点(周期折算 query.go 判定区/凭证继承 fail-open 两臂/near 折叠判带/铸边唯一点+extra 候选域/tier·序数双空间/5% 相对地板/P3 stamp)注释补全裁定 § 引用+一句机理(为何非无主启发式/翻转两侧为何保守),使未来审计者不再误判;②**G3 榜语义定谳(重要 nuance)**:可消除榜=**优化归因提醒面**——「能优化的就是可以优化的,尽量提醒用户去优化」;反事实有效性(节拍吞噬类)**不作隐藏门**:阶段二披露时 invalid 只加注不得藏席,不过度考虑吞噬折扣;此语义写入 rank_p3_measure.go 与 elim 板头注,阶段二议程两增量(三列命名/白名单)在此语义下设计;③**◈ 业务线索 TOP3→TOP5**(RULE3-1 件11):双准则并集扩为 单次最长 TOP5 ∪ 合计最长 TOP5(锚 span 不特判维持=§29.175.5④,用户选择以扩容而非排除锚解决 doFrame 占榜),◎/树头词+图例「TOP3」→「TOP5」同批,尾部计数随动;§29.175.5 rider3/§29.175.6 的 3 值 EVOLUTION 引本节。

## §29.186 AUDITFIX-A/B 收账(2026-07-21;§29.183 排批①②;两微批主会话勘验直收+抽验绿)
**AUDITFIX-A(c13675385→摘入,5 文件 +100/−21,注释+纯测试零行为)**:G11 effective 闭表头注重写(删 TotalMs 回退叙述,引真实 pin 名 TestRootCauseEffectiveMatrixUsesTypedCalibers——任务指名 P0 名为文件名非函数名,偏离落账;同族失效内联注一并勘正);G12 gated reason twin 注改指 GatedCapabilityFreqOnlyReason(DISPHYG-3 件7 边界已闭);G13 :78「待追认」→引 §29.160② 追认原文;G1 带上沿恰界双臂 pin(恰 1.15p 含/+1ULP 不含,先探后钉);G4 3% 恰界双臂 pin(恰 3% 含/一 ULP 外不含+投影全链臂);铸边 caveat 词面 %s 随真实事件类型(全仓消费面查证零 pin 撞,sched_wakeup 臂字节不变;sched_waking 臂字面 pin=卫生候选备案)。
**AUDITFIX-B(793c7dca4→摘入 b78736bb0,5 文件 +113/−4)**:G5 via_thread 三态 typed 拆分——ChainViaThreadReport.PathComplete(json path_complete,NOT 臂恒 false 非 omitempty=缺席/假不歧义)由 viaMonotonicHops 完整性赋值;OnChain 成员语义零动(RN-14 判语二分保持);ON 臂 Summary 携 path_complete=%t typed token+截断散文 caveat 保留,NOT 臂字节零动(stanza pin 绿);schema 教第三态(「仍是 ON 判语,永非竞争判语」);§12-3 矛盾形 pin(OnChain=true∧PathComplete=false∧前缀 caveat)+完整路径正臂。主会话抽验:双批 pin 单跑绿、diff 范围恰好、RN-14 NOT 臂 grep 零动。两批全套 83 包各自 EXIT=0;合入后洁净室共验。

## §29.187 用户裁定集六(2026-07-21;凭证四字族定案+修向词「IO/内核/依赖」改名)
**用户 verbatim**:「同意,但 「不可中断等待·非IO已证」席 "▸ IO/内核/依赖" 是否更好」→①**入链凭证四字族获批**:·唤醒锚定/·目标自身/·交集证明/·成员继承(EN wakeup-anchored/target-self/interval-proven/member-inherited),每 ⛓ 席行恰佩其一,图例强→弱四行表+「词越靠后成色越保守」句;树面全词(边锚定(宿主→目标)/身份继承(链窗级)等)与图例词条同批随改同源单点;②**修向词改名「IO与依赖」→「IO/内核/依赖」**(EN "IO / kernel / dependency"):三腿名诚实覆盖 IO 等待/内核不可中断等待(非IO已证族)/依赖等待(binder 类),R3CR-P2-3 矛盾在名字层消解,零席位挪动、闭集成员数不变(纯改名=EVOLUTION 引 §29.153 修向闭集);co-move 全面 sweep=tracefence display 表/◎ 节头/树 修向 chip/图例词条/context 板摘要 LLM 面/skill 教学(FREQDIR 教训:新词面同批达 LLM 可见面)/既有词 pin 刻意演化;推荐 (a) 图例注形被本裁取代。两件均入 RULE3-1 修复轮。

## §29.188 RULE3-1 收账(2026-07-21;§29.181/.182/.183/.185/.187 五裁定集十四件全落;旗舰双复核双 SHIP-WITH-FIXES,修复轮十二项全收编)
**交付(21aa23e84+95bd32a02+主会话 skill co-move,56+35 文件)**:件1 树面套话上收(⇄另板短记/边锚定短行/全席同窗树头声明,witness 套话密度 21.4%→2.3%)/件2 序数单载(佩章行徽章即序数,fold-twin 兜底词形保留)/件3 crown 图例句/件4 ε-overlap 披露道(1% 独立常数,§29.104.17⑥ 钩子——**§29.181⑥ 笔误引 §29.150⑥ 在此勘正**;donghu 三活体 0.07%/0.15%/0.28% 佩记)/件5 matches 披露注/件6 白话扩案(整席不入链上榜)/件7 非IO已证判词(榜面零裸态词,射程=▸ 席行)/件8 维持族 EN 化(EN 判词全表镜像+AST 集合相等 pin)/件9→§29.187① **入链凭证四字族**(·唤醒锚定/·目标自身/·交集证明/·成员继承,每 ⛓ 席行恰佩其一,八常量单源,图例强→弱四行表)/件10 守恒种群句/件11 ◈ TOP5(§29.185③)/件12 DRIFTGUARD 八点+§29.187② **修向词「IO/内核/依赖」**(三腿名消解非IO已证矛盾,tracefence 单源全面随改+skill 教学面主会话随批=FREQDIR 纪律)。
**修复轮收编**:P1=EN snake_case 复合词车道漏网(supplyfold PTV6-C 旧纪律残留→改走 EN 词表,渲染面 grep pin 先红后绿证据归档)/chip 单源化+完备臂(剩余 on_chain 席佩 ·交集证明 精确 gate)/图例句收窄(值序=引擎发布口径)/EN lane 白话四处/hoist 判定 typed 化(SameWindowTolerance)/stale TOP3 注/AST 镜像 pin/突变红证据留档纪律立(mut_*_red.log)。**合流**:rank_direction_axiom.go 注释冲突按对抗官处方解(AUDITFIX-A 富段胜+DRIFTGUARD 块保留);runnable2 生成器已留档(修复轮偏离1 关)。
**裁定池新报三件**:①判词文法族标签「IO与依赖族」(§29.175.17 定稿字节)是否随 §29.187② 改「IO/内核/依赖族」(推荐=随改,纯连带);②标题口径括注恒挂形(crown≠❶ 板,冷读 P2-2(b));③◈ 凭证词「自身」与两把尺归账轴词(唤醒边锚尺)是否入四字族(轴词≠凭证 chip,推荐=不入,候追认)。**回访量测点**:客户板若 doFrame 族仍占满 TOP5,下一杠杆=锚族排除(本轮被裁定排除的方案,届时重议)。门:三轮 gofmt/make/五包/全套 83 包全 EXIT=0;十板值通道零 diff。

## §29.189 AUDITFIX-C 收账(2026-07-21;§29.183 G8 时间戳零谓词统一=同事审计最后真修项;合并复核 SHIP-WITH-FIXES,P1+P2×3 现场修收编)
**交付(6b50eb8b4+920ca75da,16+5 文件)**:共享存在谓词 TraceCausalProjectionWindowPresent(end>start∧start>=0)替换投影/答案层全部 start>0 窗门——60+ 谓词点(WindowDurationMS/锚窗三解析器/IntervalValid 20 消费点/elim 算术梯/树面 21+ 门/smr1/xerr1)+三处 0-as-unset 改显式采纳旗(×N 包络 fold envelopeStartSet/instant-marker start 臂/TimeBaseSpan);重基 [0,end] trace 恢复锚窗/within 记号/占窗%/方向小计;(0,0) 缺席对由谓词自护;异语义清单(引擎 Query sentinel/缺席对/行号解析器/HULL-CRED all-or-nothing 切分论证)。四旧 pin EVOLUTION;G8 双 fixture(types+产线 tool);四突变红;四旗舰 A/B 零 diff。
**复核收编(P1 实锤)**:line-anchored 查询形(LineStart>0 无时间窗)经 normalizeQuery 不对称回填发布 Window=(0,LastTs)+window_source=query_window→放宽后被采纳为真锚=**非重基 trace 伪造全前缀关注窗口**(合成 trace 复现:假 500011ms 分母+within 全真;基线为诚实「未采集」——放宽引入的静默伪造,宁漏勿假违例)→修=typed window_source=query_window_line_anchored_unbounded_start(锚门白名单外诚实缺席)+UI-unique 选帧 heal 车道 rider+显式 time_start=0 与真重基两合法车道验证保留+四 pin;P2×3=独立再扫漏网三点(runtime:2178/tree:15954/:19539)收编。**P3 残口移交裁定池⑥**:chain path/window_stats/state account 记录 Span 直拷 q 窗仍可携 (0,LastTs) 假 span 上证据索引窗标等显示面——完整根治=引擎结果窗 typed set-flag(与残口④ selected_window 同根,合并为一案)。同事审计 13 gap 至此全清账:真修五落(G5/G8/G11/G12/G13+恰界 pin)、词面两落(G2 chip/G9 种群句入 RULE3)、既裁七维持+DRIFTGUARD。

## §29.190 用户裁定集七(2026-07-21;裁定池四决策逐裁,全按推荐)
**用户 verbatim**:「① 判词文法族标签「IO与依赖族」连带改名 --- 按更优做法(推荐=随改)处理 ② 标题口径括注是否恒挂 ---- 按更优做法(推荐=恒挂) 处理 ③ 「轴词」是否并入凭证四字族 --- 按推荐的最优做法处理 ④(合并④⑤⑥)引擎结果窗 typed set-flag 案 --- 更优做法(推荐=立案一个中批) 处理」
**裁定落位**:①判词文法族标签→「IO/内核/依赖族」(EVOLUTION 引 §29.187②,纯连带)→A2 批扩容件8;②标题口径括注恒挂=crown≠❶ 亦触发(「(主榜席;❶ 按板内发布序)」形,与既有 crown≠◎首节 括注同词族单点)→A2 批扩容件9;③轴词不入族**追认关案**(异义异词:◈ 提及凭证/⛓ 记账凭证/尺口径轴三职各词,图例各自定义已足,零码);④**WINFLAG-1 中批立案**:引擎结果窗+记录 Span 全线携 typed start_set(TimeStartSet 解析旗传导至结果结构/note 键/消费端分流:真 0 渲染、unset 0 缺席;引擎侧 rank fold 同型 start>0 点改读旗),一次根治 selected_window 抑制/span 直拷假窗标/引擎折叠退化三残余;R2' 全链+板指纹 XLANE-3 论证;序=A2 之后。裁定池清空。

## §29.191 BADGEVIS 立案(2026-07-21;用户「❶..❺ 徽章大小是否可大一点且不被遮蔽?编号字有点不清晰」)
定谳双病根:①遮蔽=东亚歧义宽度类 bug(❶ U+2776 部分终端 1 格记宽字体画宽→后字压徽)→根治=显示格宽对该字形类 CJK 环境记 2 格+徽章后强制空格+几何 pin(字形无关,无条件修);②数字不清=衬线实心圈孔径小→字形三案(A 现状 ❶/B 推荐 ➊ 无衬线粗体 U+278A 族/C 空心 ① 孔径最大但视觉权重轻易混空心记号档;文本形不可行=[N] 撞 E# 标、裸 #N=§29.36.2 禁形)。默认=B+①根治,入 A2 件10(委托默认,用户可换 C);字形=单点常量,图例/pin 演化。
**§29.191.1 补记(2026-07-21 用户 verbatim)**:「按 B(推荐,无衬线实心) 的来。」——字形换族 ➊..➎(U+278A)由委托默认升为显式裁定,A2 件10(b) 照案执行,C 备选关闭。

## §29.192 用户裁定集八+CLUSTERDIAG 立案(2026-07-21)
**用户 verbatim**:「1. 自身折算席在 ◎ 恒显不入值切 落入一下。2. 另外,当前看到客户trace里在整个trace范围内 频点 信息都是全的,但是仍然得出 簇结构不可判 的情况,是因为当前核簇判定太严格了吗? 3. 窗内可消除量总览里 "- 辅助" 区域 "未入榜最大" 等子项的列表是否有TOP N限制,避免过多噪音?」
**裁定落位**:①**自身供给折算席 ◎ 恒显豁免值切**(§29.93 席存在(缺口>0)即入 ▸ 频率与热治理 节不受 TOP5 值切,CAPFIX「链上 TOP 不丢」同族纪律;席不存在(缺口 0/无数据)不合成)+伴修 SELFRUN-DISC(无频点数据时诚实缺席披露「运行频点未采集,自身降频折算不可量」,区分「无损失」与「量不了」)→A2 件11;②CLUSTERDIAG-1 立案:全频点 trace 仍判「簇结构不可判」——机械诊断核簇判定门(判据清单+客户 trace 逐门定位失败点+过严 vs 数据诚实不足分类;**放宽=值通道变化须用户裁定**,交诊断 dossier 不先动);③辅助区可增殖族统一 TOP N:∩ 重叠对/⌗ 口径旁栏/条件族(构成拆解等)=TOP3(按值降序)+尾部计数(与 ◈◇▒ 同文法);守恒/未入榜/未入榜最大=恒单行不涉→A2 件12。
**§29.192.1 补记(2026-07-21 用户「自身+优先级反转…没有比较不可能出现反转,表述需要优化调整。对吧?」)**:定谳=病根非「自身不可反转」而是**「反转必有对端,行面吞了对端」**——自身反转席的账=自身等待墙钟,判词比较对象=typed BlockingPeer(硬低优先级关系已证的另一线程),显示层不点名对端致「自己和自己反转」误读。修=自身反转席(◎+树)行内点名对端:判词限定形「优先级反转候选·对端 <线程>」(§29.175.17 ·限定文法;对端名过截断纪律 RUN2FIX-A 地板);BlockingPeer 未解析→维持现形不合成(宁漏勿假);非自身反转席(主体即对端侧)不涉。→A2 件13(允许移交下批出口)。
**§29.192.2 补记(2026-07-21 用户 verbatim)**:「"自身折算席 ◎ 恒显" 应该显示在 因果树上,窗内可消除量总览 里面凭排序上位即可,没必要强制在 窗内可消除量总览 里面 恒显。」→§29.192① 修正:恒显面=**因果树**(自身供给折算席存在(缺口>0)时树面恒显——树侧折叠/容量帽不得吞该席);**◎ 零豁免**(TOP5 值切照常,凭排序上位);A2 件11(a) 随改;SELFRUN-DISC 缺席披露(b)不变。
**§29.192.3 补记(2026-07-21 用户 verbatim)**:「自身反转席行内点名对端…避免过长,可仅在 因果树上 通过多行显示即可。」→§29.192.1 修正:对端点名面=**仅因果树,行2 多行形**(自身反转席树面行2 补「· 对端 <线程>(低优先级已证)」独立限定行,来源仍=typed BlockingPeer 已解析才合成);**◎ 席行零动**(保持紧凑现形);A2 件13 随改。
**§29.192.4 补记(2026-07-21 用户 CLUSTERDIAG 设计方向 verbatim)**:「核簇判定 是否可以在全trace范围内流式的 按信息增量逐步的去推断,而不是非要找到特定的某个点全部得出结论?因为整个trace范围内,频点信息 "cpu_frequency" (重点),热限频信息等 非常丰富,另外还有一点 同簇的cpu频点变化数值完全一致 且 前后时间偏差极小 这个规律也可以通过信息增量的方式 通过变化规律来判定 也可以作为一个关键证据之一。」→设计输入定档:①推断形=全 trace 流式增量(证据累积置信单调,不依赖单一决胜点);②证据源=cpu_frequency(主)+热限频(thermal limit)等;③关键规律=同簇齐动共移(变化值完全一致∧前后偏差≤微小容差)——**精确信号族**(逐事件确定性谓词,合红线,可入硬口径);与既有 C1 齐动共移判据(§29.163.1/§29.172)同源可复用。CLUSTERDIAG-1 dossier 义务扩:诊断现状之外,评估该流式增量共移设计(机制草图/与 clusterFreqDeriveMaxSkewSec 及 C1 census 的关系/冲突核处理(某核两簇间迁移或共移对象漂移)/值通道影响面/噪音风险);启用仍候 dossier 后用户终裁(值通道)。

## §29.193 CLUSTERSTREAM-1 授权立案(2026-07-21;用户全权委托)
**用户 verbatim**:「无需等我裁定,你先按默认最优的形态裁定去默认实现,后面我有空了再看。流式判定的成果在单次问题对话范围内 能复用尽量复用,避免反复触发判定即可。」
**默认最优形态定档(委托裁定,追审面)**:①推断=全 trace 流式增量共移(精确谓词:同值∧偏差≤容差 的 pairwise 见证累积→并查集成簇;频点值集分层作第二独立信号互证;反例见证记账,置信单调;donghu.ftrace 基准=181 见证/零反例三簇);②证据源=cpu_frequency(ftrace flavor)+C| counter(systrace flavor)双 flavor 摄入(CLUSTERDIAG 若定位 flavor 缺口则同批修)+热限频辅证;③**复用=trace 级 Index 挂载单次推导**(惰性首用一次,同 trace 全部查询/全对话复用,确定性=同 trace 同簇——超额满足「单次问题对话内复用」);④值通道=判簇成功席位折算从纯频率比升级为弱核算力比例(可证算力信息时),「簇结构不可判」臂收窄+S1 七臂 enum 词随动,**值 diff 逐席归因落账供用户追审**;⑤板指纹按 XLANE-3 论证;⑥宁漏勿假指:见证不足/冲突核→维持不可判臂(阈值=dossier 定,默认保守)。序=CLUSTERDIAG dossier 到即开 CLUSTERSTREAM-1 实施批(旗舰双复核制)。
**§29.193.1 CLUSTERDIAG-1 dossier 收账+CLUSTERSTREAM-1 开工(2026-07-21)**:诊断定谳=**全序列身份判据单点否决结构**(同簇要求两核变化曲线全流逐点一致,中段任一不配对(丢行/一次>15µs 偏斜)即永久分簇,与其余成百上千共移见证无关)——舰队三实锤:case1=2517 行频点被 `cpu0↔cpu1 @17729.521567 mid_alignment_mismatch` 单点判死;case2=co_witness_floor(window_carve 老基,HEAD 侧扫预期治愈);test_trace_02 臂收窄三候选(comove_floor 族/fmax_tie/overflow,词面六臂折叠一句不可分辨=自诊断缺口)。donghu.ftrace 主会话实测=181 见证/零反例三簇/偏斜中位 2µs。**实施形=dossier §5 草图**(§29.193 授权):流式 pairwise pro/con 见证累积(复用 side_scan 单遍有界基建)+公告不铸见证守卫(§28.5 P1 停放假并结构排除)+con 一票否决+定地板 2(同 clusterFreqTrimmedMinAligned 常数论证)+sameEmission 快路保留(停放核并簇零回归)+trimmed 全序列判据族退役(mid/head/tail 臂消亡);词面=七臂逐臂 fork(§3.3)+split 败因因子(kHz vs 偏斜)披露;值通道=judged 升级全清单(§3.2 1-6:弱簇 capRatio<1 缺口上升方向真值/板指纹涉动/核类词激活/◎ 无缺口枝迁移与 §29.192.2 联动),逐席 diff 归因落账供追审;复用=Index 级惰性单推导(既有挂载形);诚实残留臂保留(single_cluster/no_domains/真 tie/真 >4 域/稀疏地板——全程停放仅公告形流式同样判不出,治它=C1 burst 硬门+limits 锚,维持独立裁定点不入本批);test_trace_02 秒级判别补采(tracediag event_search cpu_frequency+split 邻域 ±50ms 原文行)入回访清单。合流序=A2 之后(tree.go 词面撞面)。

## §29.194 A2 收账(2026-07-21;队列尾十三件=§29.174⑤+§29.179 委托+§29.190-192 裁定族;旗舰双复核双 SHIP-WITH-FIXES,修复轮 F1-F8+冷读全系收编)
**交付(19164d4fd+c24a58d03,90+4 文件)**:件1 下一步实例化(三模板话退役→◎ 修向节确定性合成「<修向>→<动作> <主体>(<值>ms 可消)」,共享 roster 权威抽取 fence 字节恒等);件2 树头迷你图例(28+1 项闭表只发实际用到字形,「先用后释」清零);件3 席行折行(⤷ 续行记号+断点白名单增强:⌗ openPunct/枚举融合/×乘子;树席行≤100 census 达成);件4 UX-16 三件(成因─→构成─ 四面/快照观测窗注/平铺值降序+树头承诺);件5②wire-fold max 双键 R2' 七处全同步([E31] 47.282 成员最大句实拍);件6 citation 净增披露(citations_minted_persist token);**件7 hitraceconv flake 根修**(O_TRUNC 空文件窗×10ms 轮询竞态确定性定谳→temp+rename 原子发布,非隔离非重试);件8 族标签「IO/内核/依赖族」;件9 括注 badge 臂(crown≠➊,合成 pin 三臂);件10 BADGEVIS(➊..➎ U+278A 恒 1 格族换+遮蔽根治);件11(a) 自身折算席树面恒显(wire 帽 typed 三元豁免,◎ 零豁免=§29.192.2);件12 辅助区可增殖族 TOP3(∩/构成拆解;⌗ 帽既 3 对齐尾词);件13 自身反转对端行2(§29.192.3)。
**修复轮收编**:P2-F1=「(低优先级已证)」括注 typed 前提被证伪(candidate 无 caliber 门+BlockingPeer=锁对端)→**词降「· 对端 <线程>」**(§29.192.3 原词括注按宁漏勿假降,恢复条件=wire 铸硬关系对端载体,坐标留注——**用户追审点**);P2-F2=EN 长括注撕裂→值+ASCII 括注整组融合(fusionCap 60 封顶;CJK scope 排除=既有 CJK pin 实红抓获初版,真信号非 flake);F3 EN 头注行 101→100;F4 迷你图例补 ↳+glyph-lead 结构断言;F5 五注释 ⤷ 改齐;F6 徽章 EAW 注改真;F7 minted 净增注降格+对冲搅动 QCE GAP-B 同族备案;F8 self 主行并入 fold。**评估不实施/移交**:件5① 折叠准入值优先(引擎切帽+E# 全局重号=独立立批候裁);件11(b) SELFRUN-DISC(typed 缺席载体需引擎全链新铸,坐标已列,候独立小批);件5③ fold-twin 徽章维持保守(DECISION RECORD:resolver 序数=嘈声禁入精确徽章道)。**候裁小件三**(A2R-3/5/8):badge+[E#] 复合 token 空格形/⌗ 混池定序词面张力/◎ 面 157-160 宽存量行(UX-2 的 ◎ 版)。**勘误**:原批报告「fourteen arms/六包全绿」→实 15 臂/六包=5 绿+tool 复绿。门:三轮 gofmt/make/六包/全套 83 包全 EXIT=0;十份三代 dump 值多重集恒等,3 份 zh 字节恒等。回访清单+:件9/11a/12/13 四面 fixture-only 水位首出场读感+case2 co_witness_floor 治愈期望+公告计数旧并簇形复查(CLUSTERSTREAM 对抗官 P3-3)。

## §29.195 CLUSTERSTREAM-1 收账(2026-07-21;§29.192.4/§29.193 用户设计+全权委托落地;旗舰双复核=对抗 SHIP+冷读 SHIP-WITH-FIXES 修复轮全收编)
**交付(56d1565ae+5e5111a5c,14+8 文件)**:核簇判定从「全序列身份单点否决」改为**流式 pairwise pro/con 见证累积**——值键贪心配对(fuzz 20k 次证=逐值类最大匹配,「缺席≠异值」由构造保证=假 con 结构性不可能)+定地板 2(同 clusterFreqTrimmedMinAligned 常数改名 EVOLUTION)+con 同窗异值一票否决(union 全 pairwise 查,分量内无 con 不变式 fuzz 证)+公告不铸见证(§28.5 P1 假并结构排除)+sameEmission 快路保留(停放核零回归);trimmed head/mid/tail 族退役(case1 单点否决病消亡);Index 级惰性单推导(同 trace 全查询复用=§29.193③);split_audit 演化=transition_conflict(两侧 kHz+偏斜因子)/co_witness_floor(见证数);七臂词面逐臂 fork(§3.3 词形)+witness 225 并注单折算动词;值通道=judged 弱簇 capRatio 既有机械零新码,**本地五窗零值翻转**(donghu 已判面字节恒等=渐进单向实证;升级在客户全量侧显形);donghu 基准 pin(三簇/con=0/pairwise 25/55/63)。**修复轮收编**:P2-F1=veto 分簇形空审计(判簇全 pairwise vs 审计代表对粒度不同构)→derive 侧记录 con 边+审计直发因子(冷读原构造 {0,1}|{2,3} 仅 con(1,3) 证不可实现,可实现形=con 触代表×非代表,已 pin);P2-F2=图例枚举补第五臂+closed-set↔图例完备性 pin(EN 枚举同批换行词 verbatim);F3 下界除外键双词;F4 成因短语入不可断词表(名册派生);F5 稠密窗成本注落档(不加步进帽=§29.129 既裁③);F6 两注释漂移。
**追审备案四条(复核官+修复轮)**:①§29.193①「频点值集分层第二信号」被 §29.193.1/§5 实施形合法取代未实装(候选未来独立裁定);②S1 单簇字节冻结现仅存 CLAUSE 面,suffix 面按并注取短形(非静默漂移);③「渐进单向零回归」=语料域承诺非结构承诺——cap3 真值表三翻转全在 §29.193.1 定档内(mid 治愈/公告双向不铸/稀疏诚实拒),客户全量复放须见 case2 co_witness_floor 痊愈+曾以公告计数并簇的旧 trace 复查(回访清单+);④残口=同节点 gated/fold reason 孪生分叉(E19/E26 形,DISPHYG-3 同族,候独立立案)。EN 产线见证=单测 pin 承载(harness 语言开关候补跑)。门:实施+双官+修复三轮全 EXIT=0(83 包);对抗官含洁净室基线独立重建 A/B 复现。

## §29.196 用户裁定集九(2026-07-21;七件+三批处置逐裁)
**用户 verbatim**:「① 「(低优先级已证)」 --- 就写通过边链接的对端即可,唤醒关系不一定都是 锁,降到 「· 对端 LockHolder-7」即可。② badge+[E#] 复合 token 空格形(A2R-3) -- 按最优推荐来。③ ⌗ 计数当量行在平铺列的定序(A2R-5) -- 按 最优裁断(推荐) 来。④ ◎ 面存量宽行(A2R-8,UX-2 的 ◎ 版) -- 按 最优裁断(推荐) 来。⑤ 折叠准入值优先(件5① 评估结论候裁) -- 按 最优裁断(推荐=不立批) 来。⑥ gated/fold reason 孪生分叉(CLUSTERSTREAM 残口) -- 按 最优裁断(推荐=立小批) 来。⑦ 频点值集分层第二信号(远期记档) -- 按 最优裁断 来。其它三个批类问题,同意你的推荐裁定。」
**裁定落位**:①对端词形追认=「· 对端 <线程>」终形(对端语义=**边链接的对端**,不限锁——A2 修复轮降词获批,「已证」括注永久退役,恢复条件注留档但非承诺);②③④⑥→**SMALL3-1 小批**(②指针复合 token 补空格+pin;③⌗ 行恒沉平铺列尾+树头承诺句加「(计数当量行恒末)」半句;④◎ fence 宽行纪律=超预算拆同级/⤷ 按行族定+golden 演化;⑥fold reason 键上 wire R2' 七处同步+同臂词面,消同节点 reason 分叉);⑤折叠准入值优先=**不立批定案**(条件批:客户复放实锤「重要行被折且披露不足」再启);⑦值集分层=记档关案(未来冲突仲裁增强候选);三批处置追认=WINFLAG-1/SELFRUN-DISC 在飞、折叠值优先不做。裁定池清空。

## §29.197 客户回访反馈四件立案(2026-07-21;cust_report_xx.txt;用户判「核簇判定不出=客户信心最高风险」)
**用户 verbatim**:「1. 核簇判别客户反馈还是失败的,当前核簇判定不出来已经成为影响客户信心的最高风险点。2. "- 辅助" 本身是低优先级信息 "未入榜最大" 列的太多,因为其 不占序数 只显示TOP 5 个,其它折叠减少噪音。3. (折算,按全域最高频,簇最高频并列,核类排序不可判,按频率比) 和 (运行频点非最高,已计入有效归因,簇最高频并列,核类排序不可判,按频率比) 反复出现,且内容信息冗长繁琐。4. 当前采集簇信息里面里面提到的 collect_clusterdiag.yaml 文件不存在」
**事实定谳(报告实读)**:①CLUSTERSTREAM 共移**判簇成功**,卡点=下一环 **fmax_tie**(簇最高频并列→核类排序拒判→capRatio=1);「不可判」词×13 逐行复读放大观感=客户读作整体失败;②未入榜最大 ×21 行=复合席(优先级反转 runnable 族携唤醒边界)逐席铸行增殖;④指引 N1 文件名失配(guide 写 collect_clusterdiag.yaml,库内 collect_clusterdiag_customer.yaml)+客户构建早于该文件推送。
**处置**:①**CLUSTERTIE-1 立案(最高优先)**=fmax 并列破局精确信号链:(a) cpu_frequency_limits 分簇锚(热限频把两簇压同频=observed tie 的最可能病理,limits 才是真 fmax——引擎三车道 max 已含 limits,须查客户形为何仍 tie);(b) 去热限窗观测法(排除 limits 生效窗后的分簇 observed max=精确时间条件信号);(c) §29.193① 值集分层第二信号**触发启用**(用户⑦裁定的「判簇歧义再启」条件已命中)——三链全断=诚实 tie 保留;破局成功→capRatio 升级(§29.193 值通道授权延续,逐席 diff 追审);数据门=客户 N1 采集(件④修后);②未入榜最大族 TOP5(值降序)+「另有 N 项见明细」折叠→SMALL3-1 扩容;③折算括注上收=同因短语板级/图例一次声明,行面留「(折算)」「按频率比」短记(RULE3 件1 同构)→CLUSTERTIE-1 显示半;④指引文件名修正+构建前提加粗。

## §29.198 SELFRUN-DISC 收账(2026-07-21;§29.192①(b) 落地=「量不了」与「无损失」分家;合并复核 SHIP 零现场修)
**交付(677688387→摘入,16 文件 +742/−16)**:typed 缺席载体 SelfRunningFoldUnmeasuredDisclosure(判据=deficit≤0∧KnownMs==0∧UnknownMs>0,恒等 running==unknown by construction 同切片同源;NON-SEAT 侧通道,两把尺家族同构,零 gate/rank 读方=census 亲验);R2' 双 wire 键七处全同步(+info_contract census+tracediag hash 重钉两既有协议位);◎ 另账行「折算不可量 <主体> 窗内 running Xms:运行频点未采集,自身降频折算不可量」双 call site(主装配+空板形——无频点恰最可能空板);部分已知形结构排斥(b3 既有族管辖,双披露不可达=同页共存仅异主体诚实形);六突变先红后绿;四旗舰字节恒等(复核官独立基线双树亲验)。**复核 P3 观察**:同主体 depth-0 退化窗共存形两探针未复现候客户复放;长主体形 96 格折行=SMALL3-1④ 既立案射程;zh 句与裁定原文字节恒等。**合流序处方**:本批先落,SMALL3-1⑥/WINFLAG-1 基于本批重钉 trace_note_keys golden 与 tracediag hash。
**§29.198.1 违纪自查与修复(2026-07-21)**:collect_clusterdiag_customer.yaml 直推(0b4bb71cc)未跑套件——examples/tracediag/=shipped-scripts pin 覆盖面(v2 脚本须在 loadShippedScript 开关表登记 overrides),两 pin 红上 main 三个 push 未察觉;且 §29.198 推送复合命令未以 TEST_EXIT 值阻断(打印后无条件 push)。修=pin 表登记(本 commit)+tracediag 套件绿。**纪律双补**:①examples/ 一律非 docs 车道,任何新脚本随 tracediag 套件门;②推送前必须显式读洁净室三 EXIT 值判绿后单独执行 push,禁与输出打印同链无条件推。

## §29.199 WINFLAG-1 收账(2026-07-21;§29.190④ 中批=结果窗 typed set-flag;合并复核 SHIP 零 P1/P2)
**交付(14b13a5e0→摘入,17 文件)**:载体=TimeWindow.StartSet bool json:"-"(零 wire 字节零新 note 键,window_source 枚举案落选=侵入小者当选);派生单点三臂(TimeStartSet∪TimeStart>0∪normalizeQuery 全 trace 回填 provenance 位——LineStart>0 阻断分支不 stamp,复核官亲验后归一化再赋值全站点 sweep 零假阳性);13 消费点接旗((a) selected_window 解抑制=真重基 0..X 发声/(b) 四铸点 unset 形改 (0,0) 诚实缺席(证据索引假整前缀标就死)/(c) 引擎八点 rank fold/self-gap/demote/stream);recon 剥旗(值身份≠旗身份,全仓比窗 sweep 无第二点);板指纹三声称全实证(闭集不含/json:"-"/reflect 面旗盲);G8 fixture 演化;8/8 突变红;五报告+真重基第六臂 A/B 内容零 diff。**复核 P3 收录**:遗留①②(legacy path/frame window= 富注仍铸 0-start)三消费链逐一闭死=不可达投影(分类结构性保护;若未来 wakeup_chain 记录 node 化会复活——单点接旗方向留档为加固候选);流式车道回填臂结构不可达(正臂=显式 0..X 形);stats-scalar 回退 twin 无旗可载维持抑制(宁漏);复核官实弹 rebase 探针零冲突全绿。环境 flake 一例(pytest 并发预算超时)已定性非本批。

## §29.200 客户核簇 N1 回传定谳(2026-07-21;cluster_report.txt=新构建 0.1.20260722+grep_result.txt 原始行;CLUSTERTIE-1 现场重定向)
**事实**:①客户已在含 CLUSTERSTREAM 的新构建上跑 N1,仍 freq_only,split_audit=「cpu0↔cpu1 @925.310393 判定臂=co_witness_floor(共见证变迁不足:共见证=0(<2))」——新臂词面与因子工作正常;②原始行实锤病理=**恒值周期全量公告形**:每 ~1ms 全部 12 核由当值线程重发一次频点,值全程恒定(cpu0-3=1600000/cpu4-9=2151000/cpu10-11=2500000),**零变迁**→流式见证 pro=0(公告不铸见证守卫按设计工作)→co_witness_floor 诚实;③三簇结构在数据里**以值分组形式每毫秒自证一次**(三组值互斥,快照级分区恒定);④三组 fmax 1600/2151/2500 全异——判簇成功后**无 tie**,此前 cust_report 的「簇最高频并列」大概率=co_witness_floor 碎片间同值并列(同真簇碎片各持 1600000 互 tie),非真簇 tie。
**gap 定谳**:恒值公告形=CLUSTERSTREAM 已知诚实残留臂(全程停放仅公告),但客户舰队采集**常态即此形**(周期重发是采集器行为)——「稀疏证据」假设不成立,必须破局。**疑点并查**:sameEmission 快路(为停放核保留)为何未并 cpu0-3(去重后各核应为单变化点等值 2µs 偏斜——须实证何处失配:全文件首值差异/长度差/0 值离线记号混入)。
**新精确信号(CLUSTERTIE-1 重定向首环)**:**公告快照分区一致性**——每次全量公告 burst(≤N µs 内覆盖全核)内按值分组,若分区在全部 burst 上恒定且组间值互异,则分区=簇结构(组间分离由值互异**每 burst 直接证明**,组内合并由全部 burst 一致背书);§28.5 毒形(两真簇恒同值停放)在该信号下同组不可分——处置=同值组合并须额外证据(limits 异/任一 burst 值分化)否则**组内honest 合并但 fmax 同值时核类排序仍禁掷币**(该毒形下 capRatio 恒 1 无损,诚实无害);固定容差固定判据零自适应。数据基准=客户 grep 原始行(200+ burst 快照,分区零漂移)。

## §29.201 SMALL3-1 收账(2026-07-21;§29.196②③④⑥+§29.197② 五件;合并复核 SHIP 零现场修)
**交付(b0c98fd11→摘入,13 文件 +641/−53)**:件1 指针复合 token 空格(两铸点+三 pin 演化);件2 ⌗ 恒沉平铺列尾(共享谓词单点三消费,活体 witness donghu_17284_selfsem 沉尾实拍,条件承诺句「(计数当量行恒末)」仅 ⌗ 在板才渲染=词条-图例双向);件3 ◎ 封口折行(59 条超预算行→0,bar 行整 atom 断两侧几何安全,NoteWrapNeverTouchesBarRows 单行承诺 SUPERSEDED 诚实演化+新普查臂先红);件4 fold reason 孪生治愈(wakeup_aggregate_overlap 提携一对拷贝,E19 三面「簇结构不可判→仅单簇有频点采样」实拍,judged 零 reason 负臂,零新 wire key);件5 未入榜最大 TOP5(AccountMS 稳定降序+尾计数,witness 21 行形→5+尾)。复核亲验:12 dump 值多重集逐份 byte-identical;gofmt 误递归回滚核实为真;突变三臂红。**P2-lite 跟进立案**:wrap 设备在 CJK|ASCII 无空格边界拆「标签=值」chip(活体 3/59:「·方向=IO/内核/依赖」被撕/EN word+value 对),违 ⤷ 图例承诺面——修法=「X= 」类 chip 融合臂或图例词面演化,候办小件;P3 观察三条(全 ⌗ 列句空转不可达/EN 恒末句省字分句/新 const 顶 doc)记录。合流预判兑现:与 main(SELFRUN/WINFLAG)hunk 零重叠。

## §29.202 ISPGAP-1 立案(2026-07-21;客户 runnable 回放 cust_runnable2_cli.txt;用户「isplogcat-1225 没看到链上唤醒或依赖关系,如何上链的?是否系统 gap?」)
**事实定谳(报告实读)**:isplogcat-1225(日志守护线程)从原 runnable_2 报告的 ▒ 背景压力([E40] 整窗等待,不参与根因排序)跃升为**⛓ 链上 ➊ 主根因**,144.504ms=100% 整窗全额,标题「主根因: isplogcat-1225 D-state/iowait 链上累计 144.504ms」——客户面灾难级误归因。**三无席特征**:①行2 通道词=裸「链上」(全板其它链上行均带 L1/L2 层级);②「深度未解析」+「影响点 对端线程未解析」=零唤醒边零 blocked-reason 对端;③行2 无板锚(其它席有);④◎ 行为全板唯一无凭证 chip 的 ⛓ 行=「每⛓席恰佩其一」完备臂被穿透(第二 gap:chip 完备 gate 读的 typed 位未覆盖该席的准入车道)。
**主嫌疑(候实证)**:WINFLAG-1 (c) 组八点把 StartSet 旗接入 rank fold 家族区间判定——查询窗显式设置(旗=true)时 `rankFoldStartUsable(s,e,true)=s>=0∧e>s`,**若旗被误用于判定成员/席位支撑区间的存在性**(成员区间 0=引擎未设置哨兵,非真 0),未解析对端的整窗 D 行以 (0,end) 伪区间过门→包络∩锚窗恒有交→keep-⛓ 全额入链;旗语义=查询窗 provenance,永不该判成员区间。次嫌疑=G8 投影层放宽/其它 keep 臂。**处置=ISPGAP-1 紧急批**:①本地复现(xxx_all.systrace 同窗同参)+A/B 定位引入 commit(bcada9a64→a94fab050 六合流逐一);②准入车道逐臂定谳(哪个 keep 臂放行/为何零 chip);③修=旗射程收窄(成员区间判定回 0=unset 哨兵)或该 keep 臂补边/对端凭证门;背景整窗 D 行回 ▒;④chip 完备臂堵洞(该车道 typed 位入 gate 或拒入 ⛓);⑤pin=isplogcat 形回背景正臂+完备臂穿透负臂+引入 commit 的回归 pin。

## §29.203 用户裁定(2026-07-21):◈ 业务线索 TOP5→TOP8
**用户 verbatim**:「另外 "业务线索" TOP 5 可能不太够,是否可以安全的扩充为TOP 8(如果有)?」→采纳:选择规则=单次最长 TOP8 ∪ 合计最长 TOP8 去重(BusinessSpanMentionFamilyCap 5→8,EVOLUTION 引 §29.185③);安全论证=纯显示选择帽(排序✗值通道✗),准入门(on-chain∧双分量地板)零动,不足 8 显实际数+尾部计数照旧;◎/树头/图例「TOP5」词面同批随改 zh/EN;既有 TOP5 pin 刻意演化。→MENTION8-1 微批。

## §29.204 CHAINGUARD-1 立案(2026-07-21;用户「排查是否还有其它误上链场景,一定要看护好避免回归」)
处置=三路只读审计(与 ISPGAP-1 并行零冲突):①**链上准入车道全普查**(凡能置/保 chainRelevance=on_chain 的路径逐一列册:边递归铸节/R8 自身/身份继承 fail-open/包络 hull keep/聚合过继/level merge/RSPA 锚定拆分/D-IO 车道等,各配「凭证门+chip 映射+既有 pin」三列);②**逐车道对抗构造**(无边/无对端/整窗/零区间/unset-0 字段形逐车道探针实跑,找 isplogcat 同类可达形);③**看护设计**=引擎级结构不变式:「凡 on_chain∧rank>0 席必携至少一枚 typed 凭证(边锚∨自身∨继承戳∨包络戳),零凭证=fail-loud 禁入链」+chip 完备臂升引擎 census 同源(显示层 chip 穿透即 §29.202 第二 gap 的根治形);审计产出喂 ISPGAP-1 修复轮或后继 CHAINGUARD 实施批(序=ISPGAP 合流后)。

## §29.205 MENTION8-1 收账(2026-07-21;§29.203 ◈ TOP5→TOP8;微批勘验直收+抽验绿)
交付(8c8a7cd0c→摘入,6 文件 +60/−36):cap 5→8(EVOLUTION 链 §29.185③→§29.203,选择逻辑零动)+词面三面(选择规则词/◎ 图例 TOP8 行/注释)+pin 演化(cap 臂/8+8 并集十族 fixture/词面 zh/EN)。八 dump A/B 归因=仅 ◈ 行数(donghu +3/+4 族)与词面;≤5 族板实际数渲染零动、无 ◈ 板缺席照旧静默。抽验:pin 单跑绿+diff 范围恰好。历史引文残留两处刻意保留(§29.175.5 verbatim 引文/RULE3 索引注)。
**§29.204.1 CHAINGUARD 审计收账(2026-07-21;三路产出)**:①**真机制定谳(探针产线复现客户全征)**:三无席结构病理=「三层 fail-open 对空 chain_relevance 三个答案」——引擎序数道空→chain 通道发 Rank(chainless 板既裁设计)/显示通道分类器空→chain/chip 完备臂却要显式 on_chain ⇒ 空 relevance 席=⛓+序数+加冕+零凭证词;触发面=**无链板**(无目标 rank/span 未解析 frame bundle,树头自认「⊘ 唤醒链未下钻」——客户回放正是此形),isplogcat 形整窗 D/runnable 皆可加冕 ➊ 100% 全额(probes F1,三代码点定位);WINFLAG 旗嫌疑排除(门在裸 end>start 与空 relevance,非旗)。②**伴生四洞**:F2 runnable 车道缺 D/IO 的同源二分纪律(身份继承 1ms 锚交全额 36ms)【§29.211 加注 2026-07-22:36 全冠形在探针基线 a94fab050 与 d849553fc 两基线经产线 BuildRootCauseRank 均不可复现(同拓扑实产二分形 2.000/35.000);RSPA runnable 臂(RNB-1 T1,§29.88)2026-07-14 已 ship 早于本探针窗,判=**未经产线复现的静读外推形**(嫌疑=审计员按 D/IO 纪律静读外推或手喂席集/anchor-less 统计板观察);重开条件=任何活体 36 全冠观察】;F3 同账双席(成员行×churn 行无互斥申报双计);F4 邻近余段双席;F5 合账 Limit 截断形静默丢账。③**车道册**:A 构造边/E 自身/F 宿主边=fail-closed 无需修;B RootEvidence 无区间继承/C 包络+RSPA fail-open 零词/D 身份继承(既裁,typed 完备)/G 裸成员卫星臂=看护对象。④**CHAINGUARD-1 spec 定稿**:census 不变式落 assignRootCauseRanksAndTiers 双尾(铸序即普查)+投影/加冕消费端读随行 census note 第二席门(闭跨 query 合并洞)+chip 升引擎同源 enum(四值+none,完备 pin 升全席扫描)+单一发射 helper(R2');序=ISPGAP-1 合流后。处置:ISPGAP-1 重定向修 F1 主路+F5 伴随;F2/F3/F4 入 CHAINGUARD-1 实施批。

## §29.206 CLUSTERTIE-1 收账(2026-07-21;§29.197①/§29.200 客户信心最高风险件;旗舰双复核 SHIP-WITH-FIXES,修复轮 F1+P3 全收编)
**交付(de1d8cb3e+30c46aa0c+主会话 F5 175682451,27+7+1 文件)**:件A **公告快照分区信号**(deriveAnnounceSnapshotPartition,零新常数=既有 15µs 链距+地板 2):恒值周期全量公告形(客户舰队常态,零变迁)按「burst 内值分组分区恒定∧组间值互异」判簇——合成客户形(200 sweep+单丢行)判 {0-3}{4-9}{10,11} 三簇+真文件端到端 pin(fold basis big/2500000/2.53);§28.5 毒形=单值组诚实合并+single_cluster 词;漂移全信号 fail-open;丢行=full-burst 精确判(骑跨/partial 恒 skip 不 veto);件B 定谳=sameEmission 快路非 bug(全有或全无前提被舰队单丢行击破,分区信号即补位;客户全文件失配臂三候选判别入回访 N1 三命令);件C 三链 tie 兜底(limits_rank/去热限窗观测/值集 max 序,零自适应,≥3 并列保守,禁计数比较);件D 词面上收(13 处成因复读→树头「本板成因」板级一次声明+行短形,单行/混因/无因三负臂);cust_report「簇最高频并列」重解=co_witness_floor 碎片同值并列,判簇修好即消(end-to-end pin freqOnlyReason=="")。**修复轮收编**:P1=stale 破局审计串上 freq_only 判决(两并列对第二对 undecided 形,活体探针 CONFIRMED)→源头清零+负臂 pin 先红后绿;tracediag hash 注精度;F2(快照间亚周期游移假并,客户恒值形不可达)/F3(分区拒并因静默)/F4(上收普查 wire-vs-render)三备案 fix_direction verbatim 落码注→CHAINGUARD/后续小件;F5 DET1 墙钟恒等 flake=主会话根修(ObservedAt 出恒等面,墙钟禁入恒等 pin 族)。合流:examples_pin_test 同修双版按 MERGE-1 取 worktree(名册净防护收);MERGE-2 tree.go auto-union 后四族 pin 复跑绿。**客户端预期**:分区判三簇→fmax 1600/2151/2500 无 tie→judged fork 词+核类词激活→E4 类弱簇席缺口 ×1.56 向真值;「核类排序不可判」词族在其舰队形消亡。
**§29.207 ISPGAP-1 复核 REJECT+F-B 委托裁定(2026-07-21)**:复核定谳=核心三门治愈成立(双入口回 ▒/构造性豁免宽窄正确/突变全咬/二分取代论证成立/第三入口零新增),但 P1×2 退回:F-A evidence_fact 零值镜像记录被新门送入 ▒(0.000ms 噪声席+E# 重编)且同包络同主体致 AXIOM-V2 互指句 both-or-neither 全灭(b1/b3/b6 丢 ×3/×2/×3);F-B 多调用全并集形同线程同型跨记录**求和** 52.5+150+0=202.5ms=135%>100% 单行。**F-B 调和形委托裁定(§7.5 R2 既裁口径同构)**:▒/背景折叠对同线程同型跨记录,窗口重叠(同物理时间)=取区间并集,不可精确并=取大作下界,**禁求和**;×N 计数保留+「同段镜像·不可相加」话术随行;base 双席病形(⛓150+▒52.5)已由三门修消,调和后单席 ≤ 窗。F-C(空冕静默)同批修=诚实臂条件扩「有 rank 数据但 primary 空」→「窗口内未定位到链上主根因,见背景压力段」;F-D 注释同批;F-E 记档 CHAINGUARD。退回定向补修→复核官复验。

## §29.208 ISPGAP-1 收账(2026-07-21;§29.202 客户面灾难级误归因;合并复核 REJECT→定向补修→复验 SHIP=战役首个全流程 REJECT 闭环)
**交付(3af4bd0d8+3b82ecdfa,8+8 文件)**:三无席病根修=空 chain_relevance 三层 fail-open 复合的三个显示消费端各补门(types 编译 primary 车道 hop-lane #1a 同形门/树 depthless 分流拒 rank-lane 空 token/显示通道分类器空∧rank-lane→background),引擎 chainless 序数道零动(发 Rank 合法,病在显示读空为 ⛓);客户 isplogcat 整窗 D 144.504ms 形双入口(untargeted-rank/unresolved-bundle)+整窗 runnable 孪生端到端回 ▒ 背景;WINFLAG 旗嫌疑死臂论证排除(q.TimeStart>0 结构性)。**REJECT 轮(复核官六块有链板 A/B 亲刻抓获)**:F-A 零值 evidence_fact 镜像被新门送 ▒(0.000ms 噪声席+E# 重编+AXIOM-V2 互指句 both-or-neither 全灭)→镜像 verbatim 前缀豁免(仅 classified 空 token 不入 primary slice,带镜像 A/B pin+互指句在场 pin);F-B 多调用全并集同线程同型求和 202.5=135%>100%→**§29.207 委托裁定落地=同段镜像口径(×N 第六式)**:重叠账区间并集核销/不可精确并取大下界/禁求和/MergedSameSegmentMirror typed 位+原始 Σ 随行+「N次同段镜像(单项a~b,不可相加)」词;F-C 空冕静默→诚实臂扩「有 rank 数据∧primary 空」发「窗口内未定位到链上主根因,见背景压力段」;F-D trunk-admit 注释失真修。**复验 SHIP**:b1-b6 复核官自有 probe 全 byte-identical/u2 单席 150=100%≤窗/四孤立突变全咬/merge-tree 对 main 零冲突。**P3 记档三**:▒ 成对臂端点相接负 pin(CHAINGUARD 补)/Role="" 缺省开(F-E)/带 relevance 镜像假想形 census 兜。F2/F3/F4 伴生洞=CHAINGUARD-1 承接。

## §29.209 用户裁定(2026-07-22):RUNSPLIT-1 立批=runnable 同源二分推广
**用户 verbatim**:「按照 把既有二分机械推广到链上 runnable 席 的推荐来排」→采纳三件形:①锚定份计值(ledgerAnchoredRunnableMs typed 台账,⛓ 席值=链锚窗内份额,按真值参排);②余段落 ◇(条件可消上界不入守恒,与 ⛓ 席互指+「合计还原全窗账」对账句,D/IO 同源二分全同构);③无台账席两级回退=维持全额+census「成员继承」词面诚实,禁发明估算份额,待台账覆盖收敛。值通道大件:旗舰板序预期重排(向真值收紧下界),逐席值 diff 追审表随批;序=CHAINGUARD-1 合流后(RUNSPLIT-1,旗舰双复核制,挂点=RNB-1 T1 runnable 台账)。

## §29.210 CHAINGUARD-1 收账(2026-07-22;§29.204 结构看护终局批;旗舰双复核=对抗 SHIP(八板 48/48 亲刻恒等)+冷读 SHIP-WITH-FIXES,小修轮六件收编)
**交付(30306ba7f+8812db633)**:①**census 不变式**=引擎单点 censusChainSeatCredential(七档接戳表 LANE-A..G→四凭证档+none 闭集 enum),落两处 assignRootCauseRanksAndTiers 尾(铸序即普查,后铸绕不过=PostNormalizeMintCannotEscape pin);none 席=降 ▒ 背景+审计 caveat(§29.104.13 非致命不硬拦;值零动序数重排);伪区间 (0,end) 不算戳(rankFoldStartUsable 单点=ISPGAP 主嫌疑独立第二网);R8 self 臂实施中被真迹测试抓获后补(网先抓了自己一个漏);②投影第二席门(census note 单发射 helper R2' 七处+board/badge/crown 三面同门读 none 拦,闭跨 query 合并洞;无 note 旧工件渐进兼容);③chip 引擎同源(五值显式映射含 target_self/wakeup_anchored 补臂+foreignSelf 白名单;P4 全席扫描双形+merged 代表行圈网=E10(+1) witness 载体形入网);④件4 F3/F4 同账吸收臂(churn 双席六维门吸收,两 legacy 双席 pin EVOLUTION);⑤件5 三 P3 pin(端点相接负 pin/Role 空定死/RootEvidence 节点窗铸+升档 e2e)。舰队 none 清册=0(ISPGAP 修后已净,网为看护非清污)。**小修轮**:re-verify 注释改构造论证/接戳表三漂移/chip 五值契约+四 pin/merged 扫描臂/caveat 去重记档/**(c)(d) 撤销裁定**=none 席显示词+tracediag strict 臂撤销(caveat+wire+第二席门+pin 四层已足,未来 none>0 活体再启)。**S8 偏离裁定 ACCEPT**:wakeup_chain 构造席 census 归 interval_proven 档(词面字节恒等硬目标;「唤醒锚定」专属 R3 宿主边机制词,禁错借)。**记档候办**:caveat 席名集合并入形/census=target_self 无 basis 兜底形 AST 看护。F2=RUNSPLIT-1(§29.209 已裁已排)。至此「误上链」双层同源看护(引擎 census+显示门)成立,§29.202 三无席形结构性不可能。

## §29.211 RUNSPLIT-1 收账(2026-07-22;§29.209 终批=开发管线收官;旗舰双复核=对抗+冷读双席 SHIP-WITH-FIXES 且收窄裁定双席独立判合规;修复轮四件全收编)
**交付(034f4f120+d4d079da8,3+2 文件)**:本批走 spec 授权的**「评估后收窄实施+落论证」出口**——机械图实测证明件1/件2 语义**已由在役 RSPA 机械承担**(RNB-1 T1 台账挂点 query.go 成员块铸 ledgerAnchorStamped/ledgerAnchoredRunnableMs → reanchorOnChainStateSeats runnable 臂消费:⛓ 席值=锚定份、eff/Score/序数随真值、census 凭证=interval_proven(ChainAnchoredMs>0)、◇ 余段孪生 adjacent 道不入方向守恒、与 D/IO 车道共用 rspaAnchoredSummary/rspaRemainderSummary 词面单点=禁二抄天然满足),且 §29.209 点名的「身份继承/包络准入」链上 runnable 冠席**全舰队四板实测种群为零**、包络形对 runnable 族结构性不可达(rank 席 ChainCredentialEnvelopeLevel 唯一铸点经 rootCauseHullKeepIsEnvelopeTier,query.go:18428 rspaReanchorOwnedType 硬排;交付声明误引 :18417,勘误于此)。故件1/件2 落地形=**F2 探针形回归 pin 族**(TestRUNSPLITProbeF2FormBisectsToAnchoredShareAndRemainder 端到端:36 全冠绝迹/锚窗上限/守恒豁免/合计还原/双面词面共享/RNB 病形谓词),件3=唯一行为变更:**runnable 台账回退披露** rspaRunnableLedgerFallbackCaveat(哨兵 runnable_ledger_fallback:;射程=armed 板∧无基词成员席∧保全额∧台账不可用三臂(无决策/无戳失配/anchored>full);双发布位+哨兵去重;值/道/序数零动、禁估算;anchor-less 板静默=§29.61.10 既裁边界)。**双复核判决性检验**:对抗席把新 pin 剥离件3臂搬到基线 d849553fc 独立 worktree 实跑 PASS(件1/件2 批前在役的判决性证明);冷读席四板(donghu-17267/tieba-59566/tieba-61839/carve-59843)双基线全席 16 通道+caveats 逐字节 diff=空,零值变动=结构性保证(caveat 纯读)——**逐席值 diff 追审表=全零表**,§29.209「板序重排预期」在收窄下退化为零 diff 是结果而非回避。**修复轮(439ac1acc→d4d079da8)四件**:①对抗 F1 披露人口洞=inversion 原地重铸席(priority_inversion_runnable_wait,Source 仍 window_stats、台账戳先于重铸落)在 armed 无决策板保全额时 caveat switch default 静默、同一物理账重铸前后待遇不一致→**首选落地=入人口配诚实子句**(「N priority-inversion seat(s) among them already rank by their directly measured same-CPU overlap…」,零内部术语;有决策重铸席经 ChainCredentialLaneDemoted 顶闸排除+降道确发生负 pin);②冷读 F1 接线零 pin=删 query.go 两挂点后全套件照绿(M4 实锤)→e2e 发布 pin(时钟回退板产线 BuildRootCauseRank,rank.Caveats 恰一条哨兵句名席 runnable_wait worker-200;M4 复跑独家红 got 0+M4b 剥 enrich 去重红 got 2);③对抗 F2「documented fail-open keep」内部词出厂→直述形「values kept unchanged, no split estimated」+词面负 pin;④census F-4 同款去重限度候办词写上 const 注释(席名集合并入=升级路径)。五突变红全亲证。**F2 悬疑定谳**:36 全冠形两基线产线不可复现(冷读 C6 探针),§29.204.1 原记载加注=未经产线复现的静读外推形,重开条件=活体观察;身份继承形零人口=经验断言非结构证明(对抗 F3),安全网=该形若铸出有台账则二分/无台账则入件3披露,无静默逃逸。**授权链落账(对抗 F6,列用户追审面)**:收窄出口条款(「若台账射程实测远窄于席族…允许评估后收窄实施+落论证」)不在 runsplit1_spec.md 字面内,系编排方派单时按用户 standing 委托(「需要裁定的部分默认按最优推荐实施,后续追审」)附加;本节显式落账该授权链。**记档**:披露种群=截断后已发布板口径(帽死席不入计数,防误读为全池);build/enrich 席集相异被吞名限度已入候办族;复核过程纪律=交付 worktree 曾被实施侧突变循环残留污染(冷读 F6),**旗舰复核一律 git archive 不可变快照先行**入战役纪律。RUNSPLIT-1 落地后开发管线全收官,战役转纯外部等待(N1-N5/LEAD-1/C1/阶段二)。

## §29.212 同事审计 v2 核验(2026-07-22;「trace 分析规则与读写模式系统缺口审计」基线 ca644b94b;七域并行行级核验+账本双向核账;核验报告回传同目录 trace_and_read_write_mode_audit_verification_20260722.md)
**总判**:描述面约 85% 行级准确(状态全集/唤醒链六条件其五/预算/census/周期公式/能力表/supply fold/双闭表/凭证五值/排序五级/容量全表/投影 fold 十三步/五区结构/P3 十条/write 状态机与审批全对);判定面系统性问题=**未对 §29.183 前次逐条定谳双向核账**——G1/G2/G3/G4/G5/G6/G7/G9/G10/G13 十条重诉既裁(各 DRIFTGUARD 注释亲刻在案),其中两处**重复了 §29.183 勘误清单已回传的同款错误**(§5.2 条件3/4 铸边 wakeup/waking 优先级错置=census 单尺规则误置铸边车道;§6.5 2/3 分母=carve 后 interval 非全部 gap)。G8 已修复确认(WINFLAG 全链);G14=§29.210/§29.211 已候办同族(席名集合并);G11/G12=AUDITFIX-A 已收账无新 witness;G15/G16=既裁维持(§29.129 件3/§29.163 C2-S9,「披露不改判定」;§29.206 F2-F4 系公告分区条目不覆盖)。**确认真 gap 六项**:①**RW1 write 续跑无仓库身份门(全案实证,本审计最有价值)**——WriteWorkflowRun envelope 零身份字段(write_workflow_run.go:11-24)+FindActiveRun 按 ModTime 零比较(store:317-338)+无比较续跑(scheduler:636-666)+store 锚定 CWD 而 --repo 不锚定(root.go:2114-2126)⇒ 同 CWD 跨 repo 续跑场景成立,修法与 auto-resume 既定设计兼容;②③**RW2/RW3 只读 shell 护栏合同洞**——awk 程序体无验证臂(system() 静态可达)/git branch 参数不验/sed e 不验/路径校验仅 cd+GIT_DIR+git 三处(cat /etc/passwd 可达),违背护栏自身「must be read-only/stay inside repository」承诺并击穿 L6「worktree 含 blast radius」前提;定位分层=LLM 失误护栏非安全沙箱(仓内从未宣称),收窄修无争议、「移除 awk」与 skill/defaults.go:126+builtin.go:505 成文指引冲突不采;④**RW7 持久化静默降级**(Save 失败仅 Warning,scheduler:3738-3762;缓解=applied ref 独立 durable 通道,定级中);⑤⑥**文档漂移批**:RW8(architecture.md:56/:3093「重试耗尽终止 Run」与非 stream degrade-and-continue 现实相悖+orchestrator.go:2212 陈旧注释)/RW10 半条(线性 plan-apply-verify 图四处残留确凿;「read 0 字节副作用」不存在——:96 系 L1 写机械零字节语义;改列 byte-preserved 措辞三处与 CLAUDE.md L1 行为等价定义漂移)/G17(supply_fold.go:32-38 头注仍述已退役 resolveCoreTopology/频率 tier 推断供能力类,与 core_capability.go:20-23 单源矛盾,唯一 trace 域真 gap)。**非 gap**:RW4(baseline 缺席即 satisfied=代码内成文蓄意裁量+Detail 已披露)/RW6(auto-init 三档显式授权语义下顶层句自洽,§8.12/§8.13 例外已成文,仅缺脚注)/RW9(verdict 必读=成文契约)。**需用户裁定一项**:P0-1 后半=exec observation 从护栏升级 OS 级 sandbox 的产品定位(涉 L6 表述+awk/sed 取舍)。P1/P2/P3-1/P3-2 全部触既裁不立案(重开需显式推翻:§29.183 G2 基石 B 句/VS-1 §7.8 周期四层/PTV6 批②#4 V4/§29.129/§29.163/候选域钉死)。§19 对外口径两处修正(▒ 背景不入可消除种群=基石 C;补 ◈ 名维度;write 段护栏定位句)。待办登记:RW1 立案/RW2-RW3 护栏收窄批/RW7/文档清理批(RW8+RW10+G17+L1 措辞+RW6 脚注+L6 随修)——均候用户排期;方法论提示三条随报告回传(勘误清单为增量基线/G-编号先对 §29.183 核账/RW1 单独成案勿混重诉序列)。

## §29.213 用户裁定(2026-07-22):§29.212 全案按推荐最优方案实施
**用户 verbatim**:「都按照你推荐的最优方案来。」→ 裁定落地:①**shell 定位=方案 A**:exec observation 维持「LLM 失误护栏」定位,不升级 OS 级 sandbox,不移除 awk(与 skill/defaults.go:126+builtin.go:505 成文指引一致);护栏收窄修使其兑现自身「read-only/仓内」合同;L6 措辞随修注明护栏定位(「worktree contains blast radius」前提在洞修复后重新成立)。②**排期总序**(依序推进,每批旗舰双复核+终局范式):**WFID-1**(RW1 write 续跑身份门:信封四元组 canonical repo root/repo 指纹/base SHA+branch/goal hash,逐项精确匹配,不匹配 fail closed 显式选择,老文件视为不匹配,A→B 对抗测试;复用读侧 canonicaliser/指纹单点=path canonicalization 红线)→**SHELLGUARD-1**(RW2/RW3 收窄:awk 程序体拒 system()/getline|cmd/输出重定向、sed 程序体拒 e/w/W、git branch 限只读形、path operand canonicalize 仓内校验+scratchpad/runtime 白名单 typed escape lane §1.6、L6 措辞)→**DOCSWEEP-1**(零行为文档批:RW8 三态状态机两处+orchestrator.go:2212 陈旧注释、RW10 线性图残留四处、L1 byte-preserved 措辞三处对齐行为等价定义、RW6 顶层句例外脚注、G17 supply_fold.go:32-38 头注重写)→**PERSIST-1**(RW7:首次 mutation 前 checkpoint 或 persistence_degraded typed 终态+注入测试,定级中)→**DISPFIX-1**(显示小件:census/RUNSPLIT caveat 席名集合并入形+wrap「标签=值」chip 融合臂)→**PARTDISC-1**(CLUSTERTIE F3 披露扩臂:partition_below_floor/partition_drift/partition_limits_veto 因子并报,零 gate)→**CENSAME-1**(CLUSTERTIE F4 普查谓词与渲染点同源+零渲染行负臂 pin;CHAINGUARD P4 R8 兜底臂 AST/结构看护)。③**F2 亚周期游移=维持记档等活体**(备案 fix_direction 在案,活体出现再修)。RW4/RW6/RW9 维持非 gap 不动;P1/P2/P3-1/P3-2 维持既裁不立。

## §29.214 WFID-1 收账(2026-07-22;§29.213 排期件1=RW1 write 续跑仓库身份门;旗舰双复核=对抗 SHIP+冷读 SHIP-WITH-FIXES,修复轮五件全收编)
**交付(9a6885d50+385717dd6,10+12 文件)**:①**五元身份信封** WriteWorkflowRepoIdentity(canonical repo root/repo fingerprint/base HEAD SHA/base branch/goal hash+schema 标记),铸点=seedWriteWorkflowRun 单点(goal 终值后恰一次,含 imported-plan 车道);②**单点匹配门** MatchWriteWorkflowRepoIdentity(逐字段精确相等,首个不匹配给 typed reason;legacy nil/schema-0→workflow_identity_missing 视为不匹配),双消费点(store FindActiveRunMatching+loadOrSeed 重跑同判定防旁路);不匹配 fail closed+/workflow resume|show|clear 指引,显式 resume=一次性令牌→收养重戳身份;③**canonicaliser/指纹复用读侧单点**(canonicalReadRunSnapshotRepoRoot+readRunCurrentRepoFingerprint,BaseHeadSHA=指纹自身 Head 零二次 git 探针,禁第二套红线兑现);④六突变红 pin 族(A→B 对抗/base 漂移-指纹后备在场仍判别专臂/goal 变更/legacy 三 pin+显式逃逸臂/happy path 过闭保护 M5)。**偏离裁量(复核双席核准)**:StatusHash 刻意不入匹配臂(CWD 锚定 store 自扰 git status=脏树噪声,精确信号红线教科书应用;BaseHeadSHA 抓 commit 移动,StatusHash 仍存档审计,配 drift-alone-still-matches pin+活体亲测净→脏仍续/commit 移动同 hash 仍拒)。**修复轮(f7aa57c32→385717dd6)五件**:FIX-1 GoalHash 改确定性来源——冷读 CR-2 抓获 LLM 生成 Summary 进哈希(嘈声信号进硬门,自家红线之违),且任务书候选 RawRequest 亲读证伪同为 LLM echo(emit_write_analysis.go:124-128「the LLM echoes」),终选 bus Objective(系统直存用户原文,StripConversationPrefix 共享单点剥前缀),同请求异摘要 pin 亲证不误拒;FIX-2 令牌收紧=根绑定+事件语义存活期(零墙钟零 TTL):铸牌记 canonical root、消费点单谓词(令牌在场∧root 精确相等),loadOrSeed 四返回臂(匹配续/收养/fresh seed/import)后置 sweep 清其它 run 残牌——三残留窗处置:crash 窗保持(两侧一致良性)/import 跳 finder 窗 sweep 清/更新 run 抢通窗 sweep except 清,前 commit「never linger」过强声明由本 commit 如实修正;FIX-3 裸 /workflow resume 补门(对抗 R1:身份盲 ModTime 选 run 静默预授权)——失配则不铸牌+失配判词+「跨上下文收养请用 <id> 形」指引,带 ID 显式形维持;FIX-4 收养时 run.Goal 同步刷新(CR-3 显示/哈希脱钩);FIX-5 docs 措辞补 StatusHash 投影括注(CR-4)。七突变红全录(哈希源回退/谓词退化三层同红/两 sweep 删除/裸门删除/goal 刷新删除)。**记档候办(随批落账)**:对抗 R3=banner 与 /approve 自动绑定仍身份盲(显示/行为错位,后续 sweep 件);冷读 CR-5=非 git 仓五元退化三元(root 臂兜底无实洞,防未来误设);CR-6=store 读错 fail-open 续种(批前语义保留);对抗 R4=BaseHeadSHA-先于-指纹臂序 load-bearing(防重排)。门:gofmt/make/三包 -count=1/全套 ./... -p 4 全 EXIT=0。全程三次 503 服务中断(实施两次+复核一轮双席),续跑/接管/重拉三式收复,零现场损失。

## §29.215 SHELLGUARD-1 收账(2026-07-22;§29.213 排期件2=RW2/RW3 只读 shell 护栏收窄;旗舰双复核=对抗 SHIP-WITH-FIXES(1 blocker)+冷读 SHIP,修复轮六件+聚焦复验 SHIP)
**定位裁定落地(§29.213)**:exec_command 只读面=**LLM 失误护栏,非对抗性安全沙箱**;不移除 awk/sed(skill/defaults.go:126 grep/awk 回退车道+builtin.go:505 工具描述推荐 awk 保行号成文指引存活),护栏收窄使其兑现自身「read-only/仓内」承诺,L6 措辞随批注明(worktree blast-radius 前提在程序体语义验证+路径仓内校验后成立)。**交付(cb6f50d1f+8017448eb,主批 5 文件 +1538/−20+修复轮 2 文件)**:件1 awk 程序体验证(拒 system(/getline/单管道/print 重定向>;经典 awk lexer 除法vs正则判定+字符串/注释跳过);件2 sed(拒 e/w/W 命令+s///e/w flag,r/R 文件名汇入路径臂,未知字母 fail-open);件3 git branch 只读形(变更 flag/位置参数拒);件4 逐命令路径 operand 模型 readOnlyPathArgvModels(grep/rg PATTERN 绝不校验,operand canonicalize 仓内+EvalSymlinks 防 symlink 逃逸)+typed escape lane(§1.6:temp 目录族+.codrax runtime anchor+attached-artifact 绝对 Source 目录);件5 L6 措辞。write 模式经 decideWriteModeExecPermissionInScope 复用全套(worktree=command root)。**双复核 blocker→修复**:对抗席逮到 blocker=git 非 branch 臂零 per-subcommand 选项模型 `return nil`,`git diff/log/show --output=<file>` 写任意文件(绝对仓外/`../` 越界实证写成)——件4 给 cat/grep/sort 精细建模唯独 git 漏;同根带出执行向量 `git grep -O/--open-files-in-pager`(每匹配文件跑 pager)、`git -c core.fsmonitor=`(注入执行)。冷读席 SHIP+抓误伤:`/dev/null` 等设备节点作 operand 被误拒(重定向形已豁免 operand 形没跟上)、`yq --split-exp` 写向量漏+错槽、`date -f` GNU 读仓外。**修复轮六件(8017448eb)**:①git 建 per-subcommand 选项模型 validateReadOnlyGitSubcommandWriteOptions=diff/log/show 拒 --output(inline+空格,-O<orderfile> 只读形保留);②grep 拒 -O/--open-files-in-pager;③全局 -c validateReadOnlyGitGlobalConfigInjection=terminal-component exec-config denylist(command/cmd/textconv/clean/smudge/process/pager/editor/askpass/sshcommand/fsmonitor/hookspath/gitproxy/helper/program/external/packobjectshook)+--config-env,用 denylist 而非 whitelist 以免误伤合法 -c user.name/core.quotepath/color.ui(误伤零回归纪律);④设备节点 operand 豁免 isReadOnlyDeviceNode(精确 /dev/{null,zero,full,random,urandom,tty,stdin,stdout,stderr}+/dev/fd/ 前缀,/dev/nullish 仍拒);⑤yq -s/--split-exp 拒+错槽修;⑥date -f/--file/-r 路由路径臂。六突变红全录(git --output/grep -O/-c fsmonitor/设备节点两层/yq split/date -f)。**聚焦对抗复验 SHIP**(安全批+blocker 修必验):V1 修的向量 13/13 DENY;V2 未建模子命令扫查=**无可达漏网**(--output 仅 diff/log/show 写文件其余拒为未知选项无文件写,粘连 -c<key>=<val> git 自身拒 rc=129,守卫精确 -c 匹配无旁路);V3 -c denylist 覆盖所有可达 exec-config(实证 core.fsmonitor 在 git status/diff.external 在 git diff 真执行且均被拒);V4 只读形 30/30 ALLOW 零回归(含 pre-existing git -C//--git-dir 逃逸门保留);V5 门 EXIT=0。**记档候办(INFO 级不可达)**:`git -c pager.<子命令>=<cmd>` 不在 denylist(terminal component=子命令名),但 exec_command 恒把 stdout 捕获进非 tty buffer(builtin.go:617),git 只在 stdout=TTY 时启 pager,实证连已列的 core.pager 在管道下都不触发→不可达;对称加固=拒 -c key first section component==pager(顺带修修复轮注释「git 执行的 command 变量」完整性略过强措辞),随后续批或 DISPFIX 顺手。门:gofmt/make/go test ./internal/tool/ -count=1/全套 ./... -p 4 全 EXIT=0。全程一次 model 切换(/model claude-fable-5)无损。

## §29.216 DOCSWEEP-1 收账(2026-07-22;§29.213 排期件3=RW8/RW10/L1/RW6/G17 零行为文档批)
**方法**:五项并行**地面真相核实**(每项亲读现行代码建真相+产出逐字替换文本,匹配周边文风/语言)→单点严格锚点替换(16 锚全部唯一命中,否则中止)→gofmt/build 核。**交付(单 commit,16 编辑:architecture.md ×13、orchestrator.go 注释、CLAUDE.md、supply_fold.go 头注)**:①**RW8** analyzer 失败三态精化——§1.3(:56)与速记表(:3093)「重试耗尽则终止 Run」笼统化改为按错误类型分两路:stream 级传输错误(EOF/流卡死/首字节超时/空流/deadline/网络抖动,见 llm.IsStreamLevelRetryable,HTTP 429/5xx 归 L1 不在此列)硬失败置 LastError 跳 Phase 2;其余(missing-emit/质量门拒绝)装降级 IR 续跑(曾发射走 buildDegradedSemanticIR 保 RequestModel/hints/predicates,从未发射走 buildDegradedFallbackIR 单 finalize)带 SoftAnalyzerError+QualityGate 诊断+operator 日志三处披露(非「静默零值 IR」);orchestrator.go:2212 陈旧注释「terminates without entering phase 2」改为两分支按错误类分述(该注释只对正下方 stream 臂成立)。CLAUDE.md:9「stage errors and retries」核准确不改。②**RW10** 线性三节点图残留六处对齐 controller 动态 DAG:§2.3 标题「线性 3 节点图」→「controller 动态 DAG」、概览图节点「plan/apply/verify 直分派」→「controller-first 动态 DAG」、阶段表引句+SC 表引句注明 plan/apply/verify 由 controller 按 batch typed state 动态分派非固定线性、SC 表头「节点」→「阶段」、verify 行 Terminal 列 ✅ 删除(stage_binding.go 仅 StageFinalize.Terminal=true,StageVerify=false;controller finish/block 收敛非 verify 终点)。③**L1** byte-preserved 措辞四处→行为等价口径(核 TestRunMode_ReadByteIdentical 实断言 Mode=""与ModeRead 的 BusContext 输出等价而非源码字节;runReadSchedulerLoop 可为读特性演进,不变量=写机械在场与否不扰动读行为)。④**RW6** 顶层「HEAD/merge 永不自动变化」句加显式授权例外脚注(ff /merge §8.12 + 授权 auto-init §8.13),CLAUDE.md 英文/architecture.md 中文各按语言,不改强度。⑤**G17**(trace 域唯一真 gap)supply_fold.go:32 头注「CMP-C reuse: resolveCoreTopology + frequency-tier inference」改为准确单一来源:capability 类经 coreCapability(q.CoreTopology)→resolveCoreCapabilityEvidence over resolveClusterFreqDomains,NOT resolveCoreTopology(后者今仅服务 window-stats 面,query.go 唯一调用方);优先梯 explicit CoreTopology>Tier-1 共振>Tier-2 六门 keyed rail>freq_only;退役的 positional-thirds inference(inferCoreTopologyFromFrequency,§29.88.9 R6 tombstone)是嘈声信号永不喂 capability 类;加防飘逸提示。引用符号全部亲证存在(coreCapability@694/supplyFoldGlobalMaxBasis@599/presentClassesByRankDesc@1016/inferCoreTopologyFromFrequency thirds-based@21/R6 §29.88.9@1027)。**零行为**:纯文档+注释,gofmt 净、build OK。L6 架构文档面(§29.213 定位句)在 SHELLGUARD-1 批已落,本批不重复。

## §29.217 PERSIST-1 收账(2026-07-22;§29.213 排期件4=RW7 写工作流持久化静默降级;旗舰双复核=对抗 SHIP+冷读 SHIP-WITH-FIXES,修复轮三件全收编)
**病根(§29.212 RW7)**:persistWriteWorkflowRun 两处静默降级(store==nil 直接 return 续内存;Save err 仅 Warning 续)——「durable dynamic DAG」宣称能力静默退化,crash 后 run JSON 与 refs/codrax/applied ref+change report 脱节。裁定 MEDIUM(§29.213,applied ref 落主仓 git 独立 durable 通道兜底):mutation 前 checkpoint 或 persistence_degraded typed 终态+注入测试。**交付(4a79479fd+8a2b34702,主批 8 文件 +515/−18+修复轮)**:件1 WriteWorkflowRun 增 PersistenceDegraded bool+PersistenceDegradedReason string(typed 常量 no_durable_store/save_failed;修复轮补 persistence_degraded 兜底),set 于 persistWriteWorkflowRun 两降级点,sticky 镜像 caller run 指针+下次成功 Save flush-to-disk,NormalizeWriteWorkflowRun 双向配对(flag off 清 reason/flag on 空 reason backfill);件2 pre-apply checkpoint=复用既有 markWorkflowRunActiveSliceApplying+persistWriteWorkflowRun seam(严在 runControllerApplyPlan 不可逆 worktree patch+applied ref 创建前,对抗 A2 亲证时序),插 discloseDegradedPreApplyCheckpoint 一次性 pre_apply_checkpoint_degraded 进度项(per-batch dedup,不硬 block);件3 status card+completion 披露(WriteWorkflowNextActionView.PersistenceDegraded 单 typed 源,双语客户面词面)。**双复核**:对抗席 SHIP(静默残余搜捕清零=50+ persist 全经 gated path,三旁路 Save 均验证在 durable-DAG 前进路径外;三值 verdict 字节未动;A6 诚实 note=降级 flag 在 nil-store/持续失败两主模式下自身也只内存 crash 仍丢,但正是裁定 MEDIUM 之因 applied ref 兜底,文档诚实标注)。冷读席 SHIP-WITH-FIXES 逮 major=**完成面漏披露**(五 ActionFinish/completion helper 的 SetResult「write workflow complete」结果串都不读 run.PersistenceDegraded,降级 run 完成显示无限定,只 /workflow show 才见 caveat,违 spec 件3 completion 旁注)。**修复轮三件(8a2b34702)**:FIX-1=五完成终点(ActionFinish/两 interrupted-followup/appliedPendingVerify/budgetExhausted)统穿新单点 seam setWriteWorkflowCompletionResult,降级时经 writeWorkflowPersistenceDegradedCompletionCaveat(复用 writeWorkflowPersistenceDegradedLine 词面)追加 caveat,覆盖 CLI 单发(SetResult 共享 bus 结果 CLI+REPL 同源),真产线终点 pin+healthy 字节不变负臂;FIX-2=framing 精确化(discloseDegradedPreApplyCheckpoint 注释补「披露记录 persist 非 checkpoint 保证 Save,dedup≤1/batch,失败无害因 applied ref durable」;commit message 诚实表述交付边界=进程内实时披露+瞬时恢复 flush+applied-ref 重定向,非 crash 后 flag 持久性);FIX-3=Normalize 对称 backfill。五主批突变红(M1-M5)+修复轮三突变红全录。**记档候办**:无(A6/F2/F3 均已修或裁定内诚实)。门:gofmt/make/三包 -count=1/全套 ./... -p 4 全 EXIT=0。

## §29.218 DISPFIX-1 收账(2026-07-22;§29.213 排期件5=显示/加固三小件;旗舰双复核=对抗+冷读双席 SHIP(零 fix_required),小修复轮三件收编)
**交付(cb6a8a553+a897315e9,主批 10 文件 +513/−71+小修复轮 3 文件 test/注释)**:**件1 caveat 席名集合并入形**(G14/§29.210+§29.211 候办)——census(chain_credential_census)与 runnable-fallback(rank_chain_anchor_rspa)两 caveat family 从「哨兵前缀去重整句丢弃」升级为「席名集合并入」:共享累加器(两 unexported RootCauseRankResult 字段 censusDemotedLabels/runnableFallbackLabels 把 build 车道 demote 席集带到 enrich 车道)+ 共享 helper mergeStableDedupLabels(first-seen 序 verbatim 去重)/replaceOrAppendCaveatByPrefix(哨兵前缀原地换),每车道从 union(carried,本车道集)渲染一次;避开被禁的「解析既有句正则重建」次选形(禁整行正则红线);真舰队 none/fallback=0 永不活体触发,单车道形字节不变。**件2 wrap 标签=值 chip 融合**(SMALL3 P2-lite/§29.201 候办)——诊断确认拆点来自 codrax 自身 atomizer(非 viewer 层,活体复现「·方向=IO/内核/依赖」在 width 40-80 被撕),runtimeTraceProjWrapDisplay 增精确融合 pass:CJK 标签 atom + 紧邻 `=<值>` atom 融为一原子(fusionCap 28 ≤width 不强拆,bare `=` 留给 DISPLAY-HYG),EN 形单 ASCII atom 合法空格断不受影响,图例无需改(承诺变真)。**件3 SHELLGUARD pager.<sub> 对称加固**(§29.215 INFO 候办)——gitConfigKeyIsExecCapable 增 first-section-component==pager 臂(整臂拒含 pager.<sub>=false 禁用形,依据:stdout 恒非 tty pager 永不启=无损 belt-and-suspenders,镜像既有 core.pager 处置,parts[0] 精确单 token vs 值解析噪);头注完整性措辞修正(pager.<sub>=false 是「持命令」声明的反例)。**双复核双席 SHIP 零 fix_required**;对抗席三 note(件1 显示标签去重不同席同标签不可达合并/merge.<driver>.driver 既存不可达越界项/pager 臂正确),冷读席正面核实三件全扎实(累加器时序/融合无过融合/pager 唯一 section-level exec)+两 note。**小修复轮(a897315e9,循 §29.211 RUNSPLIT-1 M4 教训「唯一行为变更发布接线必配 e2e 正向 pin」)**:冷读 F1=census family 发布接线**无 e2e 看护**(删 query.go 两 census merge 挂点全套件仍绿,对比 runnable 有 TestRUNSPLITFallbackDisclosurePublishedEndToEnd)。修=补 TestChainguardCensusCaveatPublishedThroughEnrichLane——census=none 经可达路径构造(compute_supply/cpu_constraint 卫星跳过 credentialing pass→非目标成员 low_frequency 无凭证骑链上通道→census=none),驱动产线 enrichRootCauseRankWithScheduler 断言哨兵句恰一次落 rank.Caveats(fixture 自检先证席真 demote 防空过);**删双挂点→pin 红(got 0)/删单 build 挂点→绿=build-lane census=none 结构性不可达(build 车道全 mint 皆 construction credentialed),如实记档为防御镜像非假 pin**——诚实处置。FIX-2 两 has* predicate(hasChainCredentialCensusCaveat/hasRunnableLedgerFallbackCaveat)确认迁移后产线已死仅测试用,注释改诚实(「no production role, retained as test-only presence helper」)。FIX-3 mergeStableDedupLabels 显示标签去重记档。四主批突变红(M1 census/M1 runnable/M2 chip/M3 pager)+小修复轮删挂点红全录。门:gofmt/make/两包 -count=1/全套 ./... -p 4 全 EXIT=0。全程一次进程死于小修复轮中途(无成果,清探针重跑收复)。**至此 §29.213 排期 5/7 收官(件6 PARTDISC-1/件7 CENSAME-1 待续)**。

## §29.219 PARTDISC-1 收账(2026-07-24;§29.213 排期件6=CLUSTERTIE F3 分区拒并因披露扩臂)
**交付性质=纯披露、零 gate**:`announceSnapshotPartition` 旁挂只读 `announcePartitionAudit`，derive 侧在三类精确信号上记录 `partition_below_floor`（恰一完整快照）、`partition_drift`（漂移前快照数+漂移 burst 首时间戳）、`partition_limits_veto`（成员与正 limits 上界，成员组按最小 CPU、上界升序）；`sameGroup` 仍只读 `fired/groupByCPU`，所有 merge/veto/频点/fmax/序数均未读取审计字段。审计随 `clusterFreqDomains` 进入两个既有 fragmentation freq_only split-audit 铸点，与 witness-lane 句以 `;` 并报；分区未形成完整快照、成功判簇、非 derived lane 保持零新字节。rail refinement 携带全局、标签无关的 partition audit，同时继续不携带端点标签已失义的 conVetoes。F2 `partition_value_set_veto` 仅预留闭集名位，未铸、未改变判簇，继续等活体。**看护**:Run→SupplyFoldBasis→Result.Caveats 漂移形先红（基线仅 `co_witness_floor`）后绿；三因子逐臂与旧 verdict 同值、跑过拒绝 vs 从未形成快照字节区分、零快照/成功负臂、双 limits 组确定顺序、rail 携带全落 pin；原 announcement/cap3/cluster-stream/rail 族全绿，`go test ./internal/tracequery -count=1` EXIT=0。回访 N1 增三 token 判读说明。**状态**:§29.213 已完成 6/7，余件7 CENSAME-1。

## §29.220 CENSAME-1 收账(2026-07-24;§29.213 排期件7=CLUSTERTIE F4 同源普查+CHAINGUARD 结构看护)
**子件① F4**:板头 freq_only 同因上收不再按 raw source token 普查，而由 `runtimeTraceProjFreqOnlyRowRenderedCauses` 复用真实渲染门：supply 调 `runtimeTraceProjSupplyFoldVerdictFor`，排除 None、UnknownBasis 零缺口及 §24② cause-node inversion 机制句压制；gated 以 `GatedRunningDeficitMS>0 ∧ inversion` 后调用纯 builder 薄封 `runtimeTraceProjInversionComponentsOK`，零第二公式、零 marks 副作用。一行双车道原因分别入集合、行数只计一次，混因继续 fail-open。先红 fixture 实证两类悬空承诺：UnknownBasis 零缺口行只说「频率数据不全」；gated 分量 2+1 与有效归因 5 打印恒等不平；修前 zh/en 均有板头承诺而零行后缀，修后承诺撤回。N2 输出级不变式钉“承诺在场⇒至少两行后缀”；N3 既有正臂字节看护全绿；N4 谓词真值表含非 cause-node inversion Triple 正臂；N5 直接走生产 ×N merger，清 fold 但保 source/reason 的可达形不再误入普查。**子件② CHAINGUARD P4/R8**:新增 AST 读集闭包（接戳函数十字段集合精确相等）+ `RootCauseRankItem` 反射处置清册（`Chain* / OnChain* / Causality / SubjectIsAnalysisTarget / OverlapMs / Source` 必须三选一：probe/output/exempt 且豁免带理由）；临时新增未分类 `ChainCredentialStructureProbe` 与临时偷读 `ProcessComm` 两突变均独家红，补丁撤回后绿。纯测试看护，生产 census、链道、席值、序数、wire 零变。**门**:`go test ./internal/tool ./internal/tracequery -count=1` EXIT=0（tool 164.803s，tracequery 65.337s），`git diff --check` 绿。F2 亚周期游移继续等活体，本批无值集硬门。**状态**:§29.213 七批全部收官。

## §29.221 CPU-BUSY-0 / NO-WINDOW 客户回访收账（2026-07-24）

**production witness 与裁定**：`cpu_busy_zero.txt` 证明 lifecycle fail-close 后的 unavailable 被 frequency-only/streaming 面渲染成 numeric zero；`no_window.txt` 证明唯一用户外窗包含多个合法子窗时，旧 exact-equality window reconciliation 会误判 inconsistent、补采退到 whole trace，随后 `context_only` IO 背景行又使 causal/enumeration authority 因 cluster 非空而消失。两份客户原件只作外部 witness，不入仓；仓内只保留合成最小回归。`4340` 精确等于 `868×5`，是 count-only 活动指数，没有绝对高低档。

**Batch 0（`4c2149c47`）**：完成附件逐段复算、现行代码冷读和同事矩阵复核，冻结 `NW-01..NW-05`。确认真正上游根因是 nested window reconciliation、frame family 补采缺席与 nonempty cluster authority swallow；不是 incarnation guard 本身，也不是调高 IO rank 权重。

**Batch 1（`382e6baba`）**：只在唯一显式 recorded window 包住全部其他 anchor window 时选回外窗；互不包含仍 fail-open，不做 union/majority/last-wins。typed frame request 无 present evidence 时有界执行 `frame_root_cause_bundle`，analyzer process/thread kind 精确穿透，非法非空 scope 拒绝。coverage/authority 与 projection cluster 独立，普通与 system supplement 结果合并；非空背景行不能再吞掉 `frame_causality=unproven`、enumeration 或 lifecycle remedy。

**Batch 2（`c1c7eba13`）**：CPU interval 数学与 identity lane 解耦；streaming census 发布 `evaluated/not_evaluated`；core class 只累加 measured/partial CPU，frequency-only 保留频率但显示 busy/idle unavailable，mixed 显示 partial，实测零仍是 measured zero。incarnation 对线程/进程身份相关聚合继续 fail-close。

**Batch 3（`9c5dec781`）**：count-only activity index 三面披露 exact breakdown、合法比较域与 `absolute_level=not_defined`；`868×5=4340` 有 zh/en 正臂，不改 `context_only/pressure_unproven`，不新增 rank、retry 或 hard gate。补齐 wall-clock/latency corroborated 正臂、多候选 selector roster、process 多 UI fail-close、scheduler state/priority 矩阵、arithmetic persist EN 接线。通用 score 图例与 typed 例外的旧文案矛盾同步消除。

**同事矩阵终判**：CPU-AVAIL 主/次面与 CAUSAL/COMPACTION authority 发布已 covered；SCHED-SEMANTICS、SELECTOR-MISMATCH、FRAME-SCOPE、ARITH-RELATION 的生产行为此前基本在场，本轮补齐缺失 pins；IO-CALIBER 保持 noisy-signal soft guidance 红线；ROLE-PROOF、LIFECYCLE-REMEDY、FREQ-AUTHORITY、LINKIFY 不重开。P2 §663 只关闭 CPU/availability 子面，FileIO/PageCache/storage contributor completeness 与复合派生物仍开放。

**过程债复核**：`PARTDISC-1@e44c245fe` 与 `CENSAME-1@583ce9261` 的生产目标 covered；前者 F2 第四名位仍零消费，后者 identity 空 cause-node 仍有方向无害的窄 fail-open。两者真实 zh/en board diff 留痕和更强逐值断言仍是过程证据债，不误写成生产 gap。`fbf0920f3` 只同步 tracediag schema adjudication/hash/e2e 期望，无 rank/gate/priority/dedup/truncation 行为变化，复核通过。

**诚实残余**：trace 不含可证明 frame/deadline/唯一 UI member 时，正确终态仍是 `frame_causality=unproven`；多个 process UI candidate 不猜选；count-only score 不跨窗长/采集配置比较；模型自由正文可能保留过强措辞，但系统投影必须并行发布 typed 结论上限，按裁定不增加硬阻断、正文重写或额外模型重试。

**验证回执**：Batch 1 tool 全包 `162.485s`；Batch 2 tracequery/tool `66.041s/162.828s`；Batch 3 tracequery/tool `64.271s/161.000s`。最终 `go test ./... -p 4` EXIT=0（tool `172.516s`、tracequery `69.763s`、tracediag `5.980s`），`git diff --check` 通过。三份账本已互相引用同一批次、同一诚实残余和同一 §663 状态边界。

## §29.222 NO-WINDOW 批独立核验 + 残余修复批收账（2026-07-24）

**独立核验**（基线 `3cda2c78b`，8 席对抗：NW-01/NW-02/NW-03/Batch2/Batch3/linkify 裁决/Batch4 台账/最优性批评官，全席对码亲证）：§29.221 四批的客户实际形修复全部复证成立（嵌套窗选举 verbatim 客户坐标 pin、frame bundle 补采、非空投影 authority 发布、`868×5=4340` 分解式、CPU/core_class 可用性面）；同时抓获两条 P1 未修与若干 P2/P3 残余，随批全部修复如下（每件先红后绿或谓词双腿 pin）：

- **ARITHDUP（P1）**：算术附注零去重——同一断言以不同词面精度重复（`18.76%`/`18.760%` 同值同渲染串）逐匹配各发一注，`no_window.txt`:189 同句两遍在 HEAD 必复现。修=relation 级值对去重（首现 token 保留容差权威）+ note 级 byte 去重第二网（`answer_document_mutation_runtime_arithmetic.go`）。
- **LINKFW（P1，勘正 §11.3 LINKIFY 行）**：`不据此重开已修生产面` 的断代说辞被独立核验证伪——`artifact_literal.go` 两张标点表缺全角括号 U+FF08/FF09 与中文引号/书名号族，客户 181 行精确形（全角括号直贴 artifact 名）在 HEAD 终端+HTML 双面仍铸 `mailto:trace_...`（HTML 面红测试逐字复现）。修=两表补全角括号/引号/书名号/顿号族；四种 CJK 包裹形双面 pin；显示形按 R4 备案接受尾段 code span（`Other_` 前缀留 text），普通 email/URL 零回归。
- **SUPPFRAME（P2）**：`trace_query_supplement.go` 窗派生失败 + D-state family 命中时无条件覆写 views 为无窗 `root_cause_rank`，把已选的 `frame_root_cause_bundle` 顶替回「通用背景补采冒充帧调查」的被判死旧形。修=frame bundle 已选时 typed skip（帧调查必须有窗），census-lite 车道不受影响。
- **GUARDREG（P2，前提修订）**：核验席「被 guard 拒绝的探针窗可赢选举」主张经代码亲证**不可达**——heavy-view guard 只拦零时间/行界调用（`traceQueryHasBoundedTraceScope`），窗登记要求双时间界显式，两门结构性互斥。裁定=不搬登记点；交付=互斥不变式两腿 pin + 生产点注释 + 「显式近全 trace anchor 窗当选」形如实冻结 pin（该形真实可达：窗是模型显式调查过的精确参数窗，选举合法当选但偏大；根修=typed 用户窗 lane，见台账立案）。
- **PARTEMPTY（P2）**：`runtimeTraceProjPartitionCaveatBlock` 只在非空 cluster 分支追加——空投影 + 无归属观测/分区截断时边界披露静默消失（NW-03 同型第三处「只在某形发布」窄门）。修=空分支边界块装配抽 helper（分区前、coverage 后，镜像非空序）+ 四臂 unit pin。
- **CURSORKIND（P2）**：`traceQueryRecordExplicitRuntimeTarget` 对 pid-only 调用默认铸 `Kind=process`（schema 明文裸 pid=exact TID），Batch1 起 Kind 承载 scope 进 frame bundle=静默升级侧信道；且 cursor 记录读的是 user 目标回填后的参数——「继承的 pid」被误录为模型游标（此前仅被 kind 撞车判重掩盖）。修=Kind 仅显式 `target_scope=process` 时铸 process + cursor 车道读回填前参数快照;四形 pin。
- **SUPPLYAVAIL（P2）**：`ComputeSupplySummary` 无条件拷贝 unavailable CPU 的零值 busy/idle 且渲染面无条件打数值——`busy=0.000ms` 伪装第三面。修=新增 `CPUBusyIdleStatus/Reason` 镜像源 CPU 权威 + 渲染 unavailable/partial 臂（空 status=legacy 字节恒等）；引擎/渲染双 pin。
- **P3 三小件**：①NW-04 接应——count-only IO 指数在场时 next-step lane 追加一条 typed 建议行（同窗补跑 `critical_blocking_calls`/存储延迟面把计数升级为墙钟证据），precise trigger=`io_pressure_evidence_quality=activity_marker_only` note，尾位不挤因果行；②NW-05 软臂——成文 prompt 增 `Runtime causal ceiling hint`，precise trigger=typed `causal_conclusion=unproven` authority（软效果 only，零硬拦/零重试/零改写），正负双 pin；③measured 行空 `busy_idle_reason=` token 省略（per-CPU/core_class 双面）。

**立案移交（记档待批，见 open gap ledger 同日追加）**：typed 用户时间窗 lane（RequestModel 零时间窗字段，与 target 推导 R2 user-first 不对称；选举对「显式近全 trace anchor 窗」形失准的根修；需 R2' 全同步面）；时间戳存在性对账 advisory（`no_window.txt`:88 抢占序列抄错时间戳 69326.849930 无任何确定性网）；42.668ms 误归 timerfd_read（真实阻塞 ≈0.1ms）=已知 L4 BODY-vs-evidence 盲点的产线实证，挂 witness 不另立架构议题。

**勘正**：§29.221/§663 把「CPU interval 数学与 identity lane 解耦」记在 `c1c7eba13`——git 实证该解耦（identity-independent 注释+caveat+per-CPU status）落在 `3bcfa33af`（2026-07-23）；`c1c7eba13` 交付的是剩余面（core-class 聚合+streaming census）。实质状态不变。

**验证回执**：每件先红后绿证据在案（ARITHDUP/LINKFW/SUPPFRAME/CURSORKIND 红输出留存本节工作账）；`go build ./...` 绿；`go test ./internal/...` 全量 EXIT=0（含 render/preview/markdownext/tool/tracequery/agent 全部新旧 pin）。

## §29.223 A/B/C 遗留批 ROI 甄选 + 高 ROI 批收账（2026-07-24）

**甄选裁定（逐项 ROI/可达性代码亲证后定）**：做=ARITH-DENOM（NW-WIN-TYPED 拆件）/LIFEMULTI（客户自身双边界形）/TESTS-2（absent 臂+NP3 逐值）/WORKTREE-CLEAN；**不做并落理由**=NW-WIN-TYPED 整批（防护形当前为理论形，客户实形已被选举覆盖+冻结 pin 会显形，挂 D9 复放证据决策）、NW-TS-RECON 实施（existence-join 全集不完整=typed observations 为 capped 子集，模型合法引用的 raw 行 ts 会成批误报；near-miss 变体在密集调度区同样受邻近事件噪扰——设计约束落台账，等下一个活体再裁）、TS-JOIN/42.668ms 形（typed 行携带窗形值 `actual_window=a..b` 而非单事件 ts+时长对，join 语义需独立设计轮，维持挂 L4 witness）、linkify 尖括号形（goldmark 显式 autolink parser 优先级 300 先于本扩展，理论臂低危留档）、§663 IO completeness（同事活跃域大批）。

**交付**：
- **ARITH-DENOM**：多窗 census 下 per-relation 分母判别（算术自洽唯一性=精确信号，advisory 车道）——恰一窗自洽→按该窗复算并披露选举（`分母=N 个 typed 窗长中唯一算术自洽的 Xms`，complete 时静默与单窗 checked 臂对称）；零窗自洽→`与全部 N 个 typed 窗长均不自洽`+最近窗重算 mismatch 注（**客户 18.76% 实形恰落此臂**：真值 18.766%，差 0.006>容差 0.005——修后客户复跑将看到真复算而非「无法唯一定位」）；多窗自洽→维持 unverified 词面。零 schema，zh/en，三臂+对称静默四 pin。
- **LIFEMULTI**：`threadIncarnationConflictsForQuery` bounded 多冲突收集（cap=4，preserved-audit 车道；空时回落单冲突函数保全 legacy 扫描车道）+`traceLifecycleSuppressionsForQuery` 逐冲突铸 suppression——客户窗的 50173/50174 双边界修后全披露（authority LifecycleBoundaries 随行）。双边界客户形+上限 roster 双 pin；单冲突门消费者（schedulerDurationsSafe 等）零动。
- **TESTS-2**：①absent 臂单元封口（frame 类视图+零帧+无 withdrawal→`absent`+`unproven`，与 withdrawal→`unavailable` 二臂机械区分，§9 fixture 7 字面）；②NP3 逐值加强=三拒并臂判簇结果以完整分区快照钉死（`announcePartitionClasses` 规范串，「一对不等」升级「全员归属恒等」，同事过程债清一件）。
- **WORKTREE-CLEAN**：39 个历史 agent worktree 逐个核净后清理（脏者上报不动），零仓库内容变更。

**验证回执**：ARITH-DENOM 期望值两轮校准后全绿（机制一次成型，18.768→18.766/50.001→50.000 为手算误差非代码病）；`go test ./internal/...` 全量 EXIT=0；`git diff --check` 绿。
