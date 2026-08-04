# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T12:34:25Z
- sweep_start_ts: 20260804-053424
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260804-053425 | log_regex,typed_operation_terminal,answer_regex | none | 54s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 最终 typed 状态和 coverage receipt 均为 complete，但本轮只抓取首页；用户要求的是站内“用户使用手册”，首页已经给出 `user_guide.html` 链接。现有 receipt 证明“所取材料完整”，没有证明“所取材料就是目标材料”，typed oracle 也只检查任意 html_text receipt，因而发生确定性伪 PASS。 |
| 2 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260804-053425 | log_regex,answer_regex | none | 423s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 最终输出 `17,4,5`，应为按 targets.csv 顺序的 `17,0,5`。reconcile 只在 contribution groups 上自校验并签 pass；assemble 也按 contribution 的 group_key 排序，没有以 targets.csv 作完备参考集、补 GroupX=0。15 个 data rounds、3 次 repair、6 次 action failure 还暴露 schema 非进展循环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- operation：`complete` 与 coverage receipt 的语义本身正确，但缺少 receipt→source identity→goal material 的绑定；不得把“源完整”提升成“目标完成”。下一批先补系统记录派生的 source identity 观测面和 opt-in eval 合同，不从请求或答案词面硬推断目标。
- data：这是参考投影合同缺席，不是单一数字波动。输出域必须由声明的 reference set 决定，聚合贡献只能填充值；reconcile 需要披露 reference_total/emitted_total/missing/extra/order/zero_fill，禁止用输出自身回验自身。
- data 过程：重复 entity-resolution 没有增加所需字段仍继续运行。后续以 typed action/lineage/output-schema/unresolved-fields 进展签名治理，不按 action 名或模型 prose 做特例硬门。
