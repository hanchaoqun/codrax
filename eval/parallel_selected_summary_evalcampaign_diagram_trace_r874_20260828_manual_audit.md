# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T07:31:52Z
- sweep_start_ts: 20260828-003151
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-003153 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 175s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、3 次 bounded typed trace_query、四跳链、11.000ms iowait 第一席、三个独立 1.000ms 优先级候选、实际占时/规则可消双账户、业务下钻与完整 Trace 因果投影均在；成文零拒绝，无 4ms/4m/流年龄降级。模型正文有一句“占用 20ms 以外的全部可归因时段”措辞不严谨，但精确系统账未受影响，不据单次 prose 波动加硬门。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-003153 | answer_regex,answer_contains,mermaid_edge_count | none | 546s | 45 | read=20,repo_map=2,list=0,trace=0,source_lens=0 | midloop=20,inv=8/0,fin_reject=9,unavail=0,prune=0 | partial | B1359 生产正证：第一稿和最终图均保留 BusContext --共享状态写入--> Mutable，anchor 精确为 bus.Mutable -> AgentContext.Mutable，且同一候选同时覆盖两个参与者；旧 Objective 弱链不再占位。正文职责较完整，最终图仍保留四阶段顺序。但 9 次拒绝暴露新增边权限原子性 gap：补 stage spine 时未同轮披露既有同 tuple 非规范映射会冲突；append -> o.busCtx.ToolResults 的 addition_ref 还被允许映射到 Analyzer -> Mutable，直到下一代才因身份冲突拒绝。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

1. `B1359-PARAMFLOWIDENT1` 获生产正证并关核心：parser-owned 参数 `bus:*BusContext` 被 stamp 到 builder.go:59 的 initializer operation，first-pass candidate 同时给 BusContext/from 与 Mutable/to，模型第一稿即选择该边；跨语言参数身份与整边联合消歧方向正确。
2. 新 P0/P1 `B1361-DIAGRAMADDITIONATOMICITY1` 是 9 次 finalizer reject 的主要系统 gap。missing precedence spine 的 add recipe 与现存相同 typed tuple 的另一 reader-visible mapping 在下一 validator 才相撞，导致“先要求 add、再要求去重”；addition_ref 执行器又只复验 technical tuple/关系种类，没有在变更前复验模型 from_node/to_node 是否遵守 recipe 的 participant endpoint side，于是 `append -> o.busCtx.ToolResults` 被暂时挂到 Analyzer -> Mutable，下一轮才报 node/identity collision。
3. 最优方案是同代预检、同批披露、执行前复核：编译 add permission 时扫描 immutable base 中相同 typed tuple 的所有可见 occurrence；若新增会重用 tuple，则在同一 relation delta 发布冲突 occurrence 的精确 failure refs/允许动作，或不给不可原子执行的 add capability。执行 addition_ref 时用 recipe 自带 technical identities、participant endpoint side 与现有 participant node IDs 逐侧校验；不匹配必须在接触草稿前 fail-closed。系统不选择删除/保留/新增，模型仍在一份原子 capsule 内决定。
4. 普通调用误铸 registration 的 `B1360-REGISTRATIONSHAPE1` 本轮未复现，仍保留 P1，但排序降到 B1361 之后；不能为了 r873 一个 BuildAgentContext 样例做函数名/关键词过滤。
5. Trace 陪跑再次证明源码图改动没有影响显式窗、自动补采、链上主根因、双账户、业务线索和因果投影；模型正文一处算术措辞漂移按模型波动留观，系统精确事实区已足够纠偏。
