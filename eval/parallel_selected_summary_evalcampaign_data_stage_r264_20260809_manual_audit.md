# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T05:02:33Z
- sweep_start_ts: 20260809-220230
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260809-220233 | answer_regex,answer_contains,mermaid_edge_count | none | 377s | 40 | read=13,repo_map=4,list=1,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=2,unavail=0,prune=1 | fail | Runner 只数任意 Mermaid edge。两次 typed relation 拒绝后，最终图仅余 runAnalyzePhase→dispatchStage→BuildAgentContext/ag.Execute 三条调用边；用户点名的 Analyzer→Explorer→Extractor→Finalizer 与 Mutable/BusContext 数据流全部从图中消失。正文还把 TaskGraph/EvidencePlan 等确定性 analysis 子包产物写成 Analyzer LLM 直接生成。 |
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260809-220233 | log_regex,answer_regex | none | 452s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B451 生效：不再自动执行 18 轮 auxiliary fallback，evaluator/continuation planner 获得控制权并正确推进到 active_with_canonical。compute_contributions 精确发现一条 canonical_label_status=unmatched；但 typed violation 只把 field/value/locator 拼为 `canonical_label_status=unmatched@line:5`，模型把展示定位符误当字段值，并交替使用 `{filter_field,filter_not_equals}` 与 parser 所需 `{field,op,value}`，6 次 repair 后 contributions 仍为 0。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

- `EVAL-B451-DATAFIRSTPRODUCER1`：production replay 关闭。自动派发不再用 auxiliary plan 饿死首个缺失 ledger producer。
- `EVAL-B454-DATAREPAIRFACTCARRIER1`（P0）：依赖失败把精确字段、值和位置压进一个 prose snippet，repair 消费面没有 typed field/value/locator 与 canonical parameter shape。最优修复是贯通 typed observed facts 和 repair params；仍由模型决定是否执行，不自动过滤业务记录。
- `EVAL-B452-STAGEDIAGRAMAUTH1`（P1）：再次确认。源码 stage/data-flow authority 没有形成 Finalizer 可消费的 typed relation recipes，硬门迫使模型删除用户要求的主图边。
- `EVAL-B453-ANALYZERPROVENANCE1`（P1）：再次确认。模型调用产物与确定性 analysis 派生产物在上下文中未分层，正文发生来源越权。
- 本轮没有扫描用户原文或模型答案作 hard gate，没有系统代写答案；Trace 显式时间窗、自动补齐、链上根因与因果投影未进入改动面。
