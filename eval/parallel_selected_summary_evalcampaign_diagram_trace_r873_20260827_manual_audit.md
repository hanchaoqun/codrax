# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T06:50:48Z
- sweep_start_ts: 20260827-235046
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260827-235048 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 183s | 37 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗内执行 6 次 bounded typed trace_query；保留 threadpool-400 → network-300 → cookie-200 → app-100 四跳链、11.000ms iowait 第一席、三个各 1.000ms 优先级候选、实际占时/规则可消双账户、链上业务下钻和完整「Trace 因果投影」。无 finalizer reject、无固定 4ms/4m 或活动流年龄降级。模型对 fscache 符号的业务下钻保持为待确认方向，没有把背景提升为已证主因。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260827-235048 | answer_regex,answer_contains,mermaid_edge_count | none | 436s | 46 | read=24,repo_map=2,list=1,trace=0,source_lens=0 | midloop=18,inv=9/0,fin_reject=3,unavail=1,prune=1 | partial | 正文阶段职责和四条 stage binding 正确，三条 typed precedence 保留；但最终图把 BusContext/Mutable 的核心载体复制表达成 bus → BuildAgentContext → Mutable → objective，未呈现源码已读且已接受的 bus.Mutable → Mutable 精确赋值。根因不是模型未探索：该行已读、已修为 parser-owned initializer 并进入证据池；系统因未保留 callable 参数 bus:*BusContext 的静态身份、又逐侧独立判歧义，未把这条直接关系放进 bounded first-pass candidate，弱的 Objective 局部操作占位。另有普通 BuildAgentContext 调用被模型标为 registration 且通过 grounding 的独立证据种类 gap，留待下一批。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

1. `B1357/B1358` 的可执行重试合同在本轮没有形成新的自冲突：第一稿一次同时收到 participant 与 relation delta，后续失败分别来自模型越权 whole-block replace 和残留 boundary，系统没有隐藏同代关系失败；但仍耗 3 次 finalizer reject，主要成本已经从 retry 路由转移到 typed candidate 质量。
2. 新确认 `B1359-PARAMFLOWIDENT1/P1`：调用参数的名称与静态类型没有进入 repomap 身份载体。于是 `bus.Mutable` 只能按词面同时匹配 BusContext 与 Mutable；旧逻辑逐侧独立消歧，把 `bus.Mutable -> Mutable` 整条边判为歧义，即使另一端已经唯一选中 Mutable。最优方案是 parser-owned 参数 binding、operation-local 身份 stamping、整条边联合唯一配对、直接双参与者关系优先排序；声明只提供身份，不创造关系。
3. 该方案必须跨当前全部 typed 语言统一：Go、Java、Python 注解、TypeScript、ArkTS、Kotlin、Rust、C、C++、Swift、仓颉；无静态类型的 JavaScript、Ruby、Lua 保持空载体并 fail-closed。提取输出改变必须整体 bump extractor version，防止暖缓存静默复用旧身份。
4. 新确认但与本批隔离的 `B1360-REGISTRATIONSHAPE1/P1`：`BuildAgentContext(o.busCtx, AgentExtractor, StageExtract)` 是普通构造调用/参数传递，模型却发成 registration；因为对象确实出现在可见行，现有 endpoint grounding 仍接受它。下一批应按 parser-owned source shape 校验 registration 是真实 registry/decorator/table binding，再决定它能否进入注册/运行时选择 authority；不能按函数名、自然语言或本样例硬过滤。
5. Trace 伴随用例人工通过，说明本轮问题与修复限定在源码图身份/候选编译层；显式时间窗、自动补采、链上根因、实际占时/可消量、背景隔离与因果投影没有被 read 图表修复影响。
