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

**typed admission 与语法终态**：`ParserVersion` 升至 v22；完整 `C|` 载荷在 300-byte 展示副本截断前只解析一次并存入稀疏 side-table，Event name/value、搜索、inventory、duration-order、namespace/TID-TGID vote、delta 与 Top-N 全部消费同一 verdict。语法固定为 exact `C|owner` + opaque pipe-name + plain decimal value + optional HiTrace metadata；owner 仅 `0..MaxInt32`，scalar 内嵌空白拒绝，名称中的 `|` 与边缘空白保真，仅全空组件拒绝。尾 metadata 依上游 `ParseTagBits` 可达闭集解释为 output level + tag-bit indexes，COMMERCIAL bit 仅能配 level M；它是 provenance，不是 instance/track identity。同一 physical source、owner、exact name 的 metadata 漂移整 series fail-close，不能拆成两个貌似真实的 track。

**数值、边界与排序**：native int64 只有在 legacy float64 公共面可无损表达时发布；`2^53` 邻接碰撞、超域、非有限、科学计数、subnormal 与超过 15 位有效数字的 decimal 兼容形均保留 inventory 并抑制派生宣称。delta 与 Top-N 键直接基于原 token 的 `big.Rat`，最终公开时才舍入；整数 delta 不能无损表达时整 series 抑制。direct 与 action-restored carved 路径都在 TrimSpace 前守 1024-byte kernel marker 边界；没有 producer profile 时 `>=1024` 只入 inventory。side-table 内存计费、控制字符/bidi 单行转义、source/order/budget 与 metadata-change 均有机械 pin。

**标准 Donghu 复放与验证**：基准 `/Users/han/opt/donghu/donghu.ftrace`（SHA-256 `e15d3dfc7963739c648a3f4f40095cabff19716575949bf38ea02ef732672b25`）当前 27,843/27,843 events，0 unparsed/panic/rollback；counter 为 `653/653 valid identity`、`653/653 numeric`、52 logical series、0 issue。focused `-count=20`（冻结复核至 30）、race `-count=5`（冻结至 10）、两包测试、最新远端重放后的全仓测试与 `go vet ./...` 全绿，三路独立协议/对抗/冻结复核均 RELEASE。

**转换格式兼容红线（用户 2026-07-12 再确认）**：鸿蒙/东湖转换输出以该标准 Donghu 样本中实际存在的字段、字段顺序、标记、标量和转义为主要兼容基准；存在即可证明的内容必须 byte/semantic compatible。代码中来自其他标准 trace/profile 的合法能力可以保留，但必须由独立 typed producer/profile 选择，禁止串 profile，禁止为了“看齐”补造缺失 CPU0、page0、device、tag 或默认值，也禁止因 Donghu 样本未出现就删除其他标准来源已证明的格式。比较和回归必须按 profile 分层，不能用自由文本、文件名或单个样本缺席充当 hard gate。

**诚实开放的能力残口**：①OpenHarmony app-file 可产生完整 `>1024` 行，而 kernel marker 可能在 1024 截断；终态需要 converter/bundle typed producer provenance。②官方 CountTrace 支持完整 int64；若客户需要超过 float 精确域，新增 decimal-string/int64 wire，不能放宽现有 float 面。③action-lost `0xIP:` carved counter 与 opaque Begin 名称存在字节同形歧义，可能影响 namespace/TID vote；需保留原 action/provenance 后再消歧。④OH B 可带 customArgs，S 可带 category/args，并可叠 chain envelope；不能机械套用 C 的 final-metadata 算法。⑤Event/EventView JSON 当前只是输出 DTO；若未来成为 Index 反灌入口，必须显式序列化 typed counter verdict。B3-b2 raw complete-set anti-rescue已由`3d9555cdb`+`a856f1d45`关闭；下一 trace correctness 批按账本推进 remaining ftrace payload admission P0 → page/marker fidelity → R1b-C，block/storage request token identity仍等待两端生产 witness。

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

## §29.47 标准 Donghu profile 差分审计立案（2026-07-12，未收账）

**基准边界**：`donghu.ftrace` SHA `e15d3df…` 含27,843 events、14种event；sched_switch 4,670/4,670有next_info，blocked 438/438有delay，page-cache 2,907/2,907为page/pfn/ofs形。该文件证明 Donghu profile 中“存在什么”，不证明其他标准profile中“不允许什么”。CRLF、无systrace header与当前LF+生成header是容器差异；线程名会变化，禁止参加profile判定。hmtrace SQL的keyless `clock_set_rate: <name> <value>`是另一已证标准profile，不能被Donghu keyed形全局替换。

**P0 remaining ftrace payload admission**：direct RMQ 尚未让wakeup/blocked/CPU/print等剩余descriptor经过共同endpoint准入，missing/wrong-wire可被补成合法0，控制字符串可形成伪物理行；structured profiler的strict wire audit同样未覆盖binder/print/IRQ/CPU/wakeup/blocked/F2FS/MMC等全部descriptor，duplicate/walk错误仍有后值覆盖或整plugin失败的风险。B3-b完成后立即开独立P0：每个精确producer descriptor审core/optional的presence、wire、range、唯一性、UTF-8和单物理行，坏sibling局部抑制并计coverage，禁止默认CPU0/prio0/空串，禁止按事件名或线程名猜profile。

**P1两项**：①SQL raw page-cache把页index与byte offset混在一组alias，`index=1`会错误发布为offset 1；应拆Donghu filemap typed tuple并checked `index<<12`后复用共享renderer，其他标准byte-offset profile独立保留。②Golden print有8条真实尾空格marker与28条>300B payload；direct TrimSpace/clamp会造成跨引擎identity漂移。建立marker专用decoder，仅去NUL、保留合法边缘空白/pipe，同时严格守UTF-8、单物理行和容量，tracequery同步。P2为SQL header emitter identity projection披露，P3为删除/诚实化 `%lu/%d` 内核计数占位头。详细施工证据与优先级以trace gap施工账第24项为准。

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
20260712-100939.634-96728.md E14 行实锤新病形:行1 已细化「iowait」,行2 类别词仍发「D状态候选」、行尾仍挂裸「· D-state」——**类别词与尾 tag 按家族铸造,不消费已细化状态,同行三面三说法**。DSTATE-REFINE 工单(§29.39②)范围扩三臂:①原臂(合并车道词面精确门:全 iowait=0→D-state/全 1→iowait/混合或不全→合并形,E16 混合形主词面=诚实保留);②**类别词臂**:「D状态候选」族类别词消费细化态(iowait 行→「IO等待候选」族,已细化 D 行→「D状态候选」,混合→维持家族词);③**裸尾巴臂**:「· D-state」尾 tag 发射点归并(细化行/混合行两形都在);caller 等待对象族行2 披露与 delay= 增强候选原臂不变。witness=96728 E14/E16 追加入工单;排位不变(尾批收口推送后即开引擎口径批:V2-P0+IOFAM-SELF+DSTATE-REFINE)。

### §29.47.3 调度调整(2026-07-12,用户连续撞到已裁未落词面项):两更名提批
用户在 96728 连续指认「未接入树」仍在——已裁词面项排队过深的调度病。调整:从 UXG-3 拆出两个**裁定已终局的纯词面更名**提进下一批(引擎口径批同车,批名扩为 CAL-1):①「链上L#(未接入树)」→「链上L#(父节点未确认)」(§29.39①:三面同词 C6+zh-en 对+图例长句互指+与「深度未解析」可区分+pin lockstep);②「可运行等待反转」→「优先级反转·可运行等待」(§29.40 处置③:5 pin lockstep,D4 组合形原 token 括注保真)。CAL-1 批=V2-P0+IOFAM-SELF+DSTATE-REFINE 三臂+两更名;UXG-3 余项(D10 聚合行 ◦族词/D14 口径教词/D9 残余)排位不变。**调度教训落档:用户可见且裁定终局的词面项不积压——凡"一句话裁定+机械 lockstep"类,搭最近的批走**。

### §29.47.4 增补(用户 witness 2026-07-12):CAL-1 扩两件
①**IOFAM-SELF witness 升级+三❶违约实锤**:最新复放自因区五行 IO 平铺(io_latency 3.670/block_io inode 2.694+2.116/io_wait 对端 udk-irq 1.347+1.248)零层次表达,且 E1/E2/E3 **三行同戴 ❶**(家族成员各携 lead 席位=徽章单点权威违约,CR-2 P4 族合成病)。修后形=一席+成员分层 roster(席行持墙钟并集+❶ 唯一;成员按层次披露:调度等待/块层 inode/完成端),层次关系以口径词分层表达。
②**PACE-ROW(新件)**:pacing_idle 段从 ×N 睡眠折叠族独立成行(与"等依赖 sleep"语义异类,合折互相稀释;引擎已铸独立行,显示层不再折入)——标准行骨架 `◦ 自身·帧间空闲(等待下一帧) X.XXXms %`,行2「节拍吻合·上下文(不参与根因排序)」(用户提「帧间正常空闲」的"正常"语义由行2「节拍吻合」承载,typed 可证=mint 条件;主词面保持已落 R2'+图例形稳定);上下文行族不参赛不佩戴;ENG-2 注记存活臂在独立成行后退役为折叠兜底。

## §29.48 STAB-1+ENG-2 收账(2026-07-12;软车道零重写+系统校验附注落地;对抗复核 SHIP-WITH-FIXES+修复轮八项)
**批主体**(§29.47.1 裁定的工程化):S1 词表双源(标准内核事件名闭集+附件原文 token 一次性提取 8MiB/20000 双上界;2779/76278 两 witness 形突变红绿)/S2 最小 diff(strict 车道走既有 block-patch 咽喉,emit_answer_document_patch block 级 op 实证)/S3' 软车道零重写(PSG+lexicon 两条 bus-strict 促升臂删除,三类出厂路径全证无软 kind 重写口;operator pipeline_contract_strict_kinds=唯一 typed escape)+「系统校验附注」独立确定性块(display-attachment 家族,双出厂口,谦逊导语,typed 三面 finding 零内部词,零发现不渲染)+保留区软-only 退场+strict 仲裁看门(FallbackFinalizerOnly 一次性武装,FRCAP 恢复链完整)/ENG-2 第三折叠机(证据 span 对齐,「其中 15.758ms 帧间空闲(等待下一帧)」上树+图例+明细)。**并发接管范本**:新会话检测到旧尾批会话仍活跃→停写只读跟踪→静默后接管,保留其 S1/S2/ENG-2 成果验收+突变,旧 S3 按裁定改造 S3'。
**复核(SHIP-WITH-FIXES,4P2+5P3)→修复轮八项全收**:P2-1 附注提供者错标(系统自声块曾套「来自模型已生成」面板导语——SystemAuthored typed 分类器+两趟分组渲染+系统导语面板);P2-2 仲裁判据②收窄 strict-for-bus(信息面软 kind 不再否决补丁稿,防「第一稿带未修 strict 出厂」)+恢复路径残留 strict 披露入附注(P6 同族);P2-3 生产实证刷新(两次复放干净运行:词表补全后误报归零,附注零发现不渲染被生产实证);P2-4 typelabels 合成 pin;P3-2 注释勘正×3/P3-3 闭集勘正(binder_transaction_reply 与 cpu_capacity 移除=查无标准 tracepoint 实据;8MiB 截断尾防护)/P3-4 PacingIdleSummary 入 schema 指纹/P3-5 附注永不进模型 block 结构 pin/P3-1 双披露注释。architecture.md §流程 7 软车道例外落档。
**教训**:①仲裁类"更好"判据必须 typed 限定严重度域,字面"任何 kind"会让噪声信号驱动稿件选择并放走未修 strict;②系统自声内容的面板溯源导语也是承诺面;③干净运行是负向证据(附注不渲染)的生产实证形。
