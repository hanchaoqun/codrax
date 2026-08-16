# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T02:46:59Z
- sweep_start_ts: 20260815-194657
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260815-194659 | log_regex,answer_regex | none | 38s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 严格输出 `{"ids":["u1","u3"]}`；一次数据工作流完成，无 JSON 修复、解释性尾文、重试、旧稿恢复或降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260815-194659 | answer_regex,answer_contains,mermaid_edge_count | none | 248s | 38 | read=7,repo_map=4,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=1,unavail=1,prune=1 | fail | B872 生产闭环且局部 typed 关系被保留；但用户要求的四阶段与 BusContext/Mutable 数据流仍未证，图诚实断开，正文却把该完整流写成既成事实。自动最小边数 oracle 未识别核心关系缺席和正文越界。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Detailed audit

### data_json_strict_ids

1. 数据车道只执行一批 `custom_transform`，直接读取已交付的规则材料与 `users.json`，按源顺序保留
   `active=true` 的 `u1`、`u3`。
2. 最终输出严格为 `{"ids":["u1","u3"]}`，没有 Markdown fence、解释前后缀、额外字段或顺序漂移。
3. `data_rounds=1`、`data_repair_rounds=0`、所有 strict-decode/string-recovery 指标为 0；本轮 JSON
   教学未形成自相矛盾合同，也没有增加模型心智或触发降级。

### qf_logic_view_read_pipeline

1. B872 获生产正证。r544 中已经接受 assignment-shaped exact value-transfer 行后，旧 initializer
   修补债仍被重放五次；r545 同一路径的 `emit_investigation_complete` 为 2 次调用、0 次拒绝，日志中
   不再出现 `unresolved schema-invalid evidence repair`，总耗时从 402s 降到 248s。修补债现在按同一
   源码赋值事实解除，而不是要求模型猜隐藏 schema 拼写。
2. Explorer 进一步取得两个真实局部关系：`Orchestrator.applyStageOutput ->
   appendStageOutputEvidenceToMutable` 的 call，以及 `o.busCtx.Mutable ->
   appendStageOutputEvidenceToMutable` 的 argument_flow；四个阶段的 precedence 也完整进入 typed recipe。
3. 第一稿图把 Analyzer/Explorer/Extractor/Finalizer 与 BusContext 字段全部直接连接，但没有对应 typed
   关系；validator 正确拒绝一次。模型 patch 后只保留三条 precedence 和上述两条局部 operation edge，
   并把 `BusContext` 显式列为 `unproven`，证明 fail-closed 图合同没有逼系统造边或代画完整数据流。
4. 最终正文仍声称“阶段之间通过 BusContext 传递数据”“StageOutput 由 applyStageOutput 写入 Mutable”
   以及 Explorer 通过 `o.busCtx.Mutable` 写共享区。当前证据只证明单个 EvidenceItems helper 的 call/
   argument handoff，不能证明每个阶段的生产/消费关系，更不能从 argument_flow 推导 callee 侧写入。
   因此自动 PASS 与最小边数不是人工正确性；中心问题仍未回答完整，且正文存在证据越界。
5. 最优下一步是 B871b：让 Explorer 沿生产代码中的 StageOutput 生产、apply/merge、下游消费链收集
   source-owned typed operation relations，并把“已证局部关系/未证高层 participant”分层交给模型。
   不得让系统补画关系、替换正文，亦不得扫描用户请求或模型答案关键词来判定应有结论。

## Disposition

- `B872-EVIDENCEVALIDATIONREPAIRSTALE1`: production-closed-r545；typed value-transfer 等价解除有效。
- `B871b-CARRIEROPERATIONCHAIN1`: P1，confirmed；局部 operation edge 在场，但请求的阶段↔载体关系
  证据链与模型上下文仍不完整。
- `B873-TRACERELATIONVOCABULARY1`: P1，confirmed；独立物理关系与分别计量在模型上下文中词义冲突，
  单独小批修复，不能用答案正文硬门。
- `JSON strict output`: production-pass-r545；无修复、无旧稿、无降级。
- `active-stream fixed-age degrade`: forbidden/not-observed；4ms/4s/4m 不能在字节流活跃时触发降级。
