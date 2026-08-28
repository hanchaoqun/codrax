# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T18:40:30Z
- sweep_start_ts: 20260822-114028
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-114030 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 精确窗、完整链、链上 IO 第一席、双账户、背景隔离及 Trace 因果投影齐全；活动流无固定耗时降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260822-114030 | answer_regex,answer_contains,mermaid_edge_count | none | 647s | 48 | read=24,repo_map=3,list=0,trace=0,source_lens=0 | midloop=21,inv=10/0,fin_reject=9,unavail=1,prune=0 | partial | B1356 生效，decorated BusContext 未被错误列为可删孤点；但局部范围字段与 lease 整块替换合同冲突造成 9 次拒绝，最终图的数据流表达明显缩水。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit findings

1. Trace 人工判定 pass。最终答案锁定 `2.000000..2.020000s` 与 `app-100`，完整保留
   `threadpool-400 -> network-300 -> cookie-200 -> app-100` 唤醒链和逐跳 CPU；11.000ms iowait 是已证链上第一席，三个
   1.000ms runnable/优先级候选保持独立。目标状态分区、实际占时与规则可消双账户、业务下钻、背景隔离及完整 `Trace 因果投影` 均在。
   finalizer 零拒绝、零 patch、零恢复旧稿；没有 4ms、4m、流年龄、首字节或上下文比例降级。模型探索 prose 曾把调用点扩写成锁/缓存机理，
   但最终结构保留证据边界；本轮不增加请求/模型/答案原文扫描硬门，也不由系统替模型改写结论。
2. Read 人工判定 partial。B1356 获得生产正证：`BusContext\nfile:line` 的第一行主身份在所有
   `optional_orphan_cleanups` 代次都受保护，模型没有再被诱导删除请求参与者。最终组件职责与顺序主链基本准确，Mermaid 语法合法；但图只保留三条
   stage precedence、`BusContext -> BuildAgentContext -> Mutable -> objective` 局部链，没有充分表达用户要求的 Mutable/BusContext 与四阶段间数据流。
3. `B1357-PATCHBASETRUTH1/P1` 为确定性重试教学错误。结构上已成功合并、但在终验被拒的 patch 草稿不会发布给用户，却会被
   `PendingAnswerDocumentPatchBase` 保留为下一轮 live patch base。旧提示仍说“原子事务未提交任何操作，下一轮重放全部操作”，导致模型重放旧 ref、旧
   edge/boundary 选择并与新代 delta 冲突。真正的执行前原子失败仍是零操作；两种失败语义必须分开。
4. `B1358-SCOPEEDITDEADEND1/P1` 为可执行能力缺口。validator 要求从 diagram block 删除陈腐
   `requested_relation_scope=partial_unproven`；同代 local relation lease 又正确禁止 `replace_blocks` 整块覆盖目标图，而 patch 协议没有局部编辑该字段的
   operation。模型依次经历字符串化畸形 JSON、只修 boundary 后仍失败、尝试不可用 full emit、整块替换触发新关系/边界失败、最后被 lease 拒绝；这不是
   单一 JSON 波动，而是两个各自合理的精确合同组合后没有交集。
5. 最优泛化修复不是放宽关系证据门或让系统替模型修图：以同一 typed request-spine mismatch 投影
   `diagram_relation_scope_edits[{block_id,action}]` 精确分支，仅允许模型选择 `set_partial_unproven` 或 `remove_scope`；执行器在 live base 上复验同一 mismatch，
   只克隆目标块并改变该 typed disclosure，Mermaid、edge anchor、节点、标签、布局和结论字节保持。无 mismatch 时字段从 schema 消失，lease-target 的
   whole replace 继续不可用。
