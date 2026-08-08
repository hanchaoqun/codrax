# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T07:52:23Z
- sweep_start_ts: 20260808-005222
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260808-005224 | log_regex,answer_regex | none | 34s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | `instructions.md` 走 `planner_distilled`、`users.json` 走 `script_consumed`；最终字节形仅为 `{"ids":["u1","u3"]}`。一轮完成、`repair_rounds=0`、warnings=0，无畸形 JSON 恢复、额外解释或降级污染。该用例只证明严格 JSON 正常路径，不覆盖 malformed recovery。 |
| 1 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260808-005224 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 203s | 39 | read=0,repo_map=0,list=0,trace=4(+1 system supplement),source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | B334 生产正证：只把 threadpool-400 11ms 链上 IO wait 作为主因；logger-900 的 19.5/7.175ms 仍可见但 Chain total/Attribution 均为 `—`，正文明确其无 wakeup dependency、仅为背景，未再推断共享资源竞争，也未自加 30.5ms。一次 completion reject 来自模型把 `unit=confidence` 错拆为独立 aggregate object，schema 精确报 `kind/label` 缺失；下一轮修正通过。不是合同冲突；finalizer reject/repair 均为 0。B333 调用点机理口径仍作 P2 观察。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Human correctness: **2/2 PASS**。runner 与人工判断一致。
- `EVAL-B334-TRACEOFFCHAINRESOURCEINFERENCE1` 获得生产正证并关闭：模型没有再从共现、粗粒度 IO 类或 pressure score 推导共享资源竞争，也没有把链外背景量与链上量相加。
- `EVAL-B331-TRACEBACKGROUNDATTRIBUTIONDISPLAY1` 的显示权限继续稳定：链外数值保留为背景规模，但不得进入根因排序归因列。
- 本次 Trace 共执行 4 个模型请求视图；explore→extract 边界另由系统补采 1 个 `critical_blocking_calls` 视图。补采只增加 typed 证据，没有改变模型结论。
- 唯一 explorer reject 是 JSON 参数对象拼装错误，不是“成文校验未通过”，也不是两个合同要求同一字段既必带又必拒；finalizer `answer_contract_check` 的所有 section 均为 0 violation，repair budget used=0。
- JSON 正常路径保持单一、低心智教学；由于本用例没有畸形输入，安全修复与有用字符串降级披露能力仍需专门异构回放，不能由本批宣称闭环。
