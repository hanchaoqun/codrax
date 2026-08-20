# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T12:18:58Z
- sweep_start_ts: 20260820-051856
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-051858 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 196s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000–2.020s 窗、完整四节点唤醒链、11.000ms 链上 iowait 首因、三个 runnable 调度席、实际占时/规则可消双轴、Trace 因果投影与自动补齐均保留；背景 IO 压力没有越权加冕。模型在后段业务线索 prose 中把 threadpool 的 11.000ms IO 等待误写成 14.000ms（14ms 属于 network sleep），与同页 typed 表矛盾；r770 无此错且上下文准确，暂判单轮模型波动，不加正文扫描硬门或系统代写。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260820-051858 | answer_regex,answer_contains | none | 197s | 36 | read=12,repo_map=2,list=0,trace=0,source_lens=0 | pass | B1235 生产接线正证：初始 Finalizer 教学明确要求每个 typed 成员自己的可见行携带 exact path/file:line；首稿即展示全部 12 个 production implementer 的精确位置，Mermaid 有 12 条模型编写的 implements 边，无抽象集合边界和 raw enum，唯一 patch 只补 member_set facet，零成文拒绝。另发现确定性系统补充把 LoopController 的注释行 515 标为“精确位置”，而模型和直接定义证据均为声明行 519；立案 B1236，不能归为模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

- Runner：2/2 PASS；人工：1 pass + 1 partial。两路均持续活跃，未因 4ms、4m、首字节、stall 或累计年龄触发降级。
- B1235 已获得生产闭环正证：展示教学、覆盖检查和关系证据门消费同一个 exact typed relation provider；系统没有创建成员、路径、表格、节点、边、标签或结论。
- B1236-ROLECOMMENTDECLARATIONLOCATIONALIAS1（P1，确定性）：`auto_pair_role_description` 指向声明前 doc comment，只承载职责/WHAT；源码定位层却将其提升为 owner/declaration anchor，使系统补充输出 `agent.go:515`，与真实声明 `agent.go:519` 冲突。修向是保留 EvidenceRef 上的职责证据，但禁止该 typed producer 铸造定位 anchor；不扫描用户输入、模型正文或语言关键词。
- Trace 的 11→14ms 错位属于模型在准确 typed 上下文上的单轮 prose 波动。继续用下一轮异构回放观察，不用系统改写模型答案，也不让邻近/背景证据进入链上根因席。
