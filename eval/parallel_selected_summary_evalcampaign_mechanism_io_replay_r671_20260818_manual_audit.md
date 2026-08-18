# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T08:06:42Z
- sweep_start_ts: 20260818-010640
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-010642 | answer_regex,answer_contains | none | 147s | 25 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | B1055 的首版 production replay 未闭环：分析器把两个机制答案维度都归为 `member_set`，而不是 `function_or_purpose`，所以 selected-body 自动配对没有触发。模型结论正确，但平台表只剩定义首行权限，Windows 被标为 `[illustrative]`，macOS/POSIX 两条操作说明无可复算正文引用；3 条池引用被清理。 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260818-010642 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 219s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 主结论与系统投影正确：目标 app-100 的 sleep 是症状，已证链 `threadpool-400 -> network-300 -> cookie-200 -> app-100` 保留，threadpool 的 11ms iowait 为链上最大单项；实际占用与可消除量、业务线索、背景 IO 均分层。模型把唤醒链叙述成线程依次“完成工作”，又把普通 sleep 推测为计时器/同步对象，超出 typed 证据上限；系统未代写或替换结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### sr_c_platform_fork

- `emit_analysis` 给出了 `question_kind=mechanism` 和精确目标 `monotonic_now_ns`，但“平台实现方式”和“调用的命令处理函数”都被归为 `member_set`。因此仅依赖 `function_or_purpose` 的 B1055 入口对生产 analyzer schema 漂移不稳健。
- Explorer 确实读取了 `src/clock.c` 与 `src/handlers.c` 的实现体，并正确识别三套平台 API 和 `cmd_sleep` 三处调用；问题发生在 typed evidence producer 未把已读 parser call 行提升为独立可引用事实，不是源码未读或模型事实完全错误。
- 最终答案的平台实现表仍由定义首行支撑，typed handoff 明示 `definition_site_only/executable_body=unproven`。这是 citation authority partial，而不是功能结论 pass。
- 泛化修向：对 `mechanism` 问题增加 typed `exact_targets` 兼容臂，仅允许模型已选择定义的规范化符号尾部与精确目标完全相等时补发 parser-authored body call；继续排除 related/absence/illustrative 定义。不得扫描用户原文、reasoning、summary 或答案文本。

### trace_query_wakeup_causal_io_chain

- 显式窗 `2.000..2.020s` 的 Trace 因果投影存在且未降级。链上 iowait 11ms、三个 1ms runnable 段和 20ms 目标 sleep 分开呈现，目标状态没有被误升为自身根因。
- 非链上的相邻 sleep 与全局 IO 压力仅留在背景/额外排查方向，没有参与主因加冕。系统自动补齐保持 evidence-only，没有删除、重写模型正文或关系。
- 模型的“依次完成工作”和“计时器、同步对象”等说法属于因果语义上限的软引导债：当前证据只证明调度/唤醒先后与状态，不证明具体完成语义或等待对象类别。不得为此增加答案 prose 关键词硬门；后续以 typed causal ceiling 的低心智教学和异构回放评估。
- 活跃流未出现固定 4ms 尚无完整答案即降级；本轮在完整结构化答案后正常发布。
