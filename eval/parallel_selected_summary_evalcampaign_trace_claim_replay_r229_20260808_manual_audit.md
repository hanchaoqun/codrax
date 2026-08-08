# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T23:11:26Z
- sweep_start_ts: 20260808-161125
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-161126 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 163s | 42 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S37cz 已消除“锁持有者已证”强断言，答案也披露 holder/waiter 未提供；但又把 84.358ms sleep 解释为等待 vsync/Binder，把 CookieMonsterCl 称为“等待 vsync 的直接依赖项”并断言其延迟帧渲染，还用窗外 onVsync/RenderThread marker 构造窗内帧起点和同步关系。主因人口仍是 typed on-chain，失败发生在模型对 typed 席位和窗外预览的机理越权。 |
| 2 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-161126 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 242s | 35 | read=0,repo_map=0,list=0,trace=11,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | frame_timeline/frame_flow 与后续 event_search 被 app-20 目标持续收窄，11 次查询仍未形成 UI→RS→GPU 的完整 typed flow；模型只能回退消费 raw trace。最终把 PID/TID 20、300 错写成 CPU 20、CPU 300，并在 frame boundary/deadline 未提供、flow unproven 时以固定 16.67ms 宣判 janky frame。runner PASS 为假阳性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- runner `2/2 PASS`，人工 `0/2`；两个 oracle 都没有覆盖答案的字段身份、权限边界和跨线程完整性。
- Donghu 案证明 `EVAL-B384/B385` 仅部分生效：候选 claim envelope 阻止了“已证锁 holder/waiter”这一种强断言，但 62K 级 Finalizer 上下文仍让模型忽略同一尾部的精确边界，把协作式 sleep、vsync/Binder、帧延迟和窗外 marker 自行串成未证机理。不能继续靠增加关键词、同义句或答案硬门补丁；应把重复 Trace 上下文按 typed 决策席压缩并保持单源，模型仍负责结论。
- frame=77 案确认 `EVAL-B386-TRACECROSSTHREADSCOPE1=P0/HIGH`：跨线程 frame 视图把 request target 当成员过滤器，锚定 app-20 后无法看到 RS/GPU；后续跨线程探测又继承同一目标。最优方案是把“锚线程选择”和“frame 成员普查”分开：显式/继承 target 只选 anchor，`frame_timeline|frame_flow` 的成员集合必须覆盖同一选择窗内所有线程；不得关闭显式窗，也不得扩大 root-cause 席位人口。
- 新确认 `EVAL-B387-TRACESPANFIELDIDENTITY1=P1/HIGH`：模型把 task label 后缀/PID 当 CPU，说明模型输入的 span 身份没有强制分列 `comm/pid/tid/cpu`，且系统投影未携带 GPU lane。应由 typed span item 逐字段发布，不从展示名猜字段；GPU 仍只能是 frame timeline/背景席，除非有独立 typed causal edge，绝不进入链上主因。
- 新确认 `EVAL-B388-TRACEDEADLINECALIBER1=P1/HIGH`：`frame_flow_causality=unproven`、`frame_boundary_authority=not_provided` 时，模型仍以常量 16.67ms 判定 1/1 丢帧。deadline/refresh-rate 必须来自 typed trace authority；缺失时只能报告观测跨度与阶段耗时，不得把固定刷新率假设升级为丢帧结论。
- 新确认 `EVAL-B389-TRACEFINALCONTEXTCOMPLIANCE1=P1/HIGH`：B384/B385 的精确最终边界已在场但仍被弱模型忽略，且 Finalizer Trace 上下文约 62K tokens、多个区块重复同类口径。后续应按 typed carrier 做语义去重与 Trace 专用压缩，减少模型心智；不得扫描用户输入、thinking、summary 或最终答案，不得系统改写模型结论。
- 两案 deterministic 根因人口仍正确：主因只来自 typed on-chain；adjacent/background 和窗外 marker 只应作为额外排查/导航。后续修复必须保持显式时间窗、自动补采、根因排序、唤醒链、窗内可消除量及真实占时/规则可消双轴不变。
