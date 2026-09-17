# r1085 人工审计：数值卡顿清单与显式窗 IO 因果链

- date: 2026-09-17T01:41:26Z
- sweep_start_ts: 20260916-184126
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

受测清洁提交 `363e83ce0`，两例各一次、并行2，无重跑。机器 1 PASS / 1 FAIL；人工分别 fail / partial。原答案、日志、机器裁决不改；本次发现不能被后续代码修复倒签成通过。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_jank_field_inventory | FAIL | eval/results/trace_query_jank_field_inventory-20260916-184126 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 191s | 31 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 查询正确3条；成文写4条、倒序，精确查询交接缺字段，见下文 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260916-184126 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 221s | 45 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 投影/窗口/11ms首席保留；正文机制过述，根因选择侧车unavailable |

## 卡顿字段清单：引擎正确，成文真实失败

原答案 `.codrax/output/20260916-184435.477-50116.md`：六个 >2^53 原生纳秒整数、三个头时间和20/40/70ms均保真。但L15/L25写总数4而正确3；L21–23排列2→4→7，反于用户降序要求。L31/L39把墙上时钟对齐列为必要条件也不准确：需要证明payload原生钟与调度事件钟的关系，同一已证trace时钟不需要转换UTC。未披露非法字段行被排除；对发出TID101、marker PID201与appid620的角色区分不足。没有具体根因宣称，未把背景封为主因。

日志 `run-1.logs/codrax-20260916-184128-000-50116.log` L1295实际使用 `appid eq 620 AND jank_frames gte 2`，L1305精确 `matched_total=3/emitted=3` 且完整。L1349 explorer甚至已提交正确3条、7/4/2聚合。最终上下文L1937–1938仅留长横幅截断；L2046–2052前五项全部来自旧宽查；L2057只剩后次查询scope，L2065一条截断fact，L2071省closure原因。数值条件、完整计数、typed字段/原生时钟边界和坏值披露未进入成文供给，构成独立系统gap（B1714）。不能恢复模型aggregate为确定性权威，也不能仅扩大top5解决。

L2172原始emit已经count4且升序；L2218仅修facet，原错误保留，不是renderer改坏。pre-triage四条陈述在L2030已被supersede，不能声称它直接导致终稿错数。模型仍可从已有三条原行自行得3，故系统交接缺口与模型算错/误排序同时存在，修交接不等于承诺模型永不出错。一次analyzer语义拒绝、一次路径修正、一次展示patch；无JSON降级、终稿成文硬拒为0。

## IO链：投影保留，但机器PASS不是完全正确

原答案 `.codrax/output/20260916-184505.937-50106.md`：显式2.000–2.020s、app Sleep20ms、threadpool IO11ms、三个runnable各1ms保留；窗外app0.020ms没有混入，嵌套等待没有累加，投影存在。优先级候选未提升为已证反转；Harmony解释来自附件入口提示 `flavor_hint_harmony_hitrace`，不是原文独立OS证明，也非凭空无来源。

正文L15将唤醒源叫直接阻塞原因；L40从caller推断文件缓存机制与具体优化；L23四节点称四跳（实际三边）。这些上限在上下文中已给出，属模型未遵循精确供给。唯一patch L2672填 `runtime_work_relation.conclusion` 为自由中文，L2675在合法枚举检查先被拒，整份patch未暂存；不得误报为已通过observation ID membership检查。原JSON侧车诚实为 `unavailable / valid_model_root_cause_selection_unavailable`，本次不能算程序化根因选择成功。

另留高ROI提示合同审计项：无semantic span时动态schema删除runtime_work_relation，但初始/补充教学仍要求选择receipt，而覆盖只认绑定receipt。需公共回归证实并补空候选闭环；不能为闭环将调度等待冒充语义工作，也不能放宽枚举。这次仅可选一轮，首稿仍保留，不是流式活跃超时或无答案。

## 证据封存与后续

7项原case/fixture/runner SHA与开跑前一致；两侧MD/HTML/JSON/日志/out共10项封存 `.codrax/tmp/20260916-r1085-original-artifacts.sha`。本批双Trace read，不冒称write/语言/图表新覆盖。B1714修复按query身份传递producer-owned清单与计数，预算省略单列，不改模型正文、原case/oracle或链上根因权限；未修复项留统一账本，后续异构pair再评，不追加本批第三例。
