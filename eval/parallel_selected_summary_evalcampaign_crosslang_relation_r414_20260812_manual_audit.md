# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T01:24:30Z
- sweep_start_ts: 20260812-182428
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260812-182430 | primary_answer | none | 99s | 24 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 五条调用边、容量检查和修补后时序图均正确；但 `AuditLog.record` 的精确终点操作只有 `System.out.println`，答案仍称“完成审计落地”。typed tail 同时塞入配置/查询/审计三个叶子的 body calls，概念终点与叶候选未分型，确认 B690。第一次图拒绝是 class 标签 `VisitController` 未携方法而无法唯一匹配 typed endpoint，精确 skeleton 修补成功。 |
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260812-182430 | answer_regex,answer_contains | none | 104s | 24 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B688 生产正证：答案/图把 `run→fetchUser→send`、`send→dispatchOnce→fetch`、`sleep→setTimeout` 保持为连续链，把 `send` 的 maxAttempts/nextDelay/sleep 保持为同 caller 扇出，没有再伪造成 callee 链。路径别名与引用正确，零成文拒绝。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings and disposition

1. `sr_ts_workspace_chain` 是 B688 的跨语言生产正证。typed carrier 发布
   `grounded_direct_edge_count=7`、`connected_edge_transition_count=5` 和同 caller sibling group；
   模型正确消费，说明 direct edge / connected transition / sibling callsite 分型不是 Go case 拟合。
2. `sr_java_call_chain` 的 runner FAIL 与人工 FAIL 一致，但不是边缺失：五条 caller→callee 证据、
   容量检查分支和最终 diagram 都在。错误是把用户概念目标“审计落库”当成了已实现效果；精确
   terminal body 只证明 `AuditLog.record→System.out.println`。
3. 根因 B690（P1）：`discover_path` 的 typed call graph 有三个叶子——配置读取、容量查询、审计
   记录。生产者把所有叶子都命名为 Selected-Terminal，Finalizer 又 round-robin 注入其 body calls；
   虽然有“概念目标与实现分离”的软提示，载体的身份词却告诉模型它们都已选中，并把真正关键的
   stdout 行淹在 8 行细节里。这是 typed 上下文自相矛盾，不归为模型随机波动。
4. 修复采用通用候选分型：`discover_path/discover_terminal` 发布 Terminal-Candidate，而 exact sink /
   runtime-selected destination 才发布 Selected-Terminal。概念终点候选每个叶子最多一条精确 body
   operation；明确候选不自动等于业务目标，并要求模型比较精确操作后自行选终点。系统不判定
   stdout、数据库、网络、文件或任何业务效果，也不扫描或改写答案；模型继续拥有解释和结论。
5. 第一次 Java diagram 拒绝属于精确身份保护的合理工作：class-only actor 无法唯一覆盖
   method-qualified call edge，copy-ready 精确 endpoint skeleton 让模型一次修复。记录为低优先教学
   观察，不为它降低关系门，也不系统代画。
6. 本批不触碰 Trace 路径。显式时间窗、因果投影、自动补齐、链上-only 主因、实际占用/业务线索
   与规则可消除双轴保持。活跃流任一 Reader 字节持续续租；4ms 没有完整答案绝不触发降级。
