# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T23:52:35Z
- sweep_start_ts: 20260819-165234
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-165235 | log_regex,write_apply,answer_regex,answer_contains | none | 642s | 26 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | B1198/B1199 生效，probe-only plan 绑定全部 required refs 且全绿；同 suite 的 pytest→unittest 成功降级仍被 capability unavailable 否决 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-165235 | answer_regex,answer_contains,mermaid_edge_count | none | 963s | 45 | read=43,repo_map=5,list=0,trace=0,source_lens=0 | midloop=25,inv=20/0,fin_reject=2,unavail=1,prune=1 | partial | 终图保留真实阶段顺序与 BusContext 参数边，但仍未表达 BusContext/Mutable 与各阶段的主要状态流；43 次读取、20 次无效调用过重 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### github_issue_tokenizers_newline_run_multirepo_py

- B1198 获生产正证：第二份 probe-only plan 的唯一探针显式绑定
  `existing-hi-test, five-newlines-collapse, newline-rank-not-merged-further,
  no-newline-rule-unaffected, outcome-1..4`，没有再把 planning-only ids 冒充证明权威。
- B1199 获生产正证：旧 cumulative-review batch 未闭合后，controller 完成窄探索；模型再次
  `finish` 时被精确收窄为 `plan_batch`，随后真实产生、应用并执行 probe-only ChangePlan。
  B1196 的 source plan id、target path 和 assertion-level observations 也贯穿第二份 plan。
- 探针、`make check`、两个 exact unittest assertions、changed-path target_behavior 和工作树审计
  全绿；补丁本身正确。Runner FAIL 来自新确认 B1200，而非实现失败。
- B1200 的精确矛盾：同一报告先记录 `pytest` 对
  `cwd=. / suite=tests/test_tokenizer.py` 为 `runner_missing`，随后 controller 以明确来源
  `runner_missing_escalation` 在同 runner、同 cwd、同 suite 执行 unittest 并通过。Proof obligations
  已全部 covered，ledger state 也是 verified，但 capability unavailable 计数仍为 1，严格 proof-only
  门因此错误产出 `verification_proof_incomplete`。
- 根修只将上述 exact typed target 的旧候选降为 advisory；不同 suite、空 suite、普通项目测试、
  失败/不可用 fallback 均不能消除 capability。没有读取命令输出、模型理由或任务/答案原文。

### qf_logic_view_read_pipeline

- B1197 保持正证：最终四条可见边分别是三条 parser/enum-owned 阶段 precedence 与一条
  `o.busCtx -> ctxbuilder.BuildAgentContext` argument_flow；没有 tuple 克隆、系统补边或答案代写。
- 相比 r747，阶段顺序恢复完整；但用户核心要求是四阶段与 `BusContext/Mutable` 的数据流，图中
  仍只有 BusContext→BuildAgentContext，Mutable 只能诚实标为 unproven boundary。Prose 描述了
  `AgentContext.Mutable` 别名与 StageOutput 回写，图却缺这些 parser-owned 关系，人工仍为 partial。
- Explorer 进行了 43 次 read、20 次 invalid tool call，Finalizer 两次关系拒绝，总耗时 963s。
  新记 B1201/P1：应从源码结构化补齐字段赋值/参数传递/回写关系，并改善 relation-target 取证
  recipe；不能为该用例放宽证据门或由系统画边。

## Cross-cutting invariants

- 两案分别持续 642s/963s，活跃字节流均未因 4ms、4m 或固定累计年龄降级；合法终止边界仍为
  cancel/deadline、无首字节、byte-stall、transport/decode failure。
- 本批未修改 Trace 查询、显式时窗、因果投影、自动补齐或答案正文。Trace 主因仍只来自 typed
  on-chain 证据；邻近/背景仅支撑额外排查，真实占用/业务线索与规则计价可消除量双轴保持。
- 无 JSON 畸形恢复、空答案、系统替写结论或图关系。
