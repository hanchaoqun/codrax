# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T08:47:31Z
- sweep_start_ts: 20260818-014730
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-014731 | answer_regex,answer_contains | none | 122s | 26 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | B1057 生产闭环：自动补发 7 条 parser-owned selected-definition body call，三个条件编译分支的 5 个平台 API 与 `cmd_sleep` 的 2 个调用全部有实现体行证据，最终表/图均保留。新 GAP B1060：同一模型关系载体把 typed skeleton 的 `n1..n7` 作为 from_node/to_node，渲染器在图外清单泄漏内部别名；图本身标签与拓扑正确。 |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260818-014731 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 193s | 32 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass | 显式 5.000..5.007s 窗形成完整 Trace 因果投影与自动补采：`worker-200 -> app-100` typed 唤醒边、目标五态 7ms 闭合、0.800ms runnable 调度供给席、5.000ms VerifyClass 真实语义 span 均保留。sleep 是症状，邻近/背景不进主因；正文分开“真实占时/新方向”与“现规则可消除”，系统附注只并置 typed 事实，未替模型改结论。两次成文结构修补未降级或丢答案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### sr_c_platform_fork

- B1057 的 C 预处理容器递归已获生产正证。日志自动补发 7 条 selected-definition body calls：`QueryPerformanceFrequency`、`QueryPerformanceCounter`、`mach_timebase_info`、`mach_absolute_time`、`clock_gettime`、`strtol`、`printf`；最终平台说明引用实际调用行，关系图保留全部 8 条真 call edge。
- 一次成文拒绝来自模型首稿把多个 API 合并成一个节点但 typed evidence 是逐 API 边；系统给出的逐 API skeleton 使模型局部替换图块，终图合法。这是正确的关系粒度修补，不是合同冲突，也没有删关系。
- 新确认 B1060：主列表已经携带完整 `from_identity/to_identity`，但可见 from_node/to_node 复用了 skeleton 的 `n1..n7`，渲染器机械发射成 `n1 -> n2`。根修只在同文档存在该 alias 对的真实可见箭头、且两端可见标签各自唯一时，显示模型写在图里的标签；不猜 alias 词形、不用 endpoint identity 代写业务名、不改 carrier/图/结论，歧义时保留原文。

### trace_query_frame_semantic_span_optimization

- 目标线程全窗状态账为 running 1.200ms + runnable 0.800ms + sleep 5.000ms = 7.000ms，D/IO 均为 0；`worker-200 -> app-100 @ 5.005000s` 是 typed 跨核唤醒边，0.800ms runnable 是唯一已计价主因席。
- 真实占用维度没有丢：worker running 4.000ms、`VerifyClass com.example.Foo` span 5.000ms 被列为链上关键路径/确定性优化候选，规则可消除为 0.000ms，答案明确提示这是新方向而不能用 0 覆盖真实墙钟。
- 5.000ms sleep 被明确标成等待唤醒症状；1.400ms worker sleep 只在邻近区，调度压力聚合只在背景区。模型没有把低优先级 waker 直接确权成优先级反转，只给出后续调查方向。
- `frame_causality=unproven/frame_evidence_status=absent` 仅作为边界限定，不摘除已证链上席；系统生成区只做五态、唤醒拓扑与席位对账，未覆盖模型的总结或修向。
