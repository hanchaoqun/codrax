# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T04:18:37Z
- sweep_start_ts: 20260805-211834
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | hilog_arkts_panic | PASS | eval/results/hilog_arkts_panic-20260805-211837 | log_attachment,answer_contains | log_triage | 93s | 20 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 准确定位 `UserCard.build` / `UserCard.ets:42:15` 的 `undefined.name` 触发点、调用栈和 `sig=6`，并明确更深层 undefined 来源仍需代码。log triager 首个 `errors` carrier 是 JSON 字符串且内部有不配对 `]`，不能无损自动修；一次重发成功。Analyzer 首轮漏 `scenario`，精确拒绝后一次修复。最终 carrier 合法、答案未丢失，系统未改写模型结论。 |
| 1 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260805-211837 | log_regex,answer_regex | none | 127s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 当前 fixture 字节为 `10+7`，终稿严格单值 `17`，两条 contribution 与 reconcile=pass 同源。过程用了 6 批/2 次修复：先发了会被 rule-ledger prerequisite 拒绝的 terminal custom transform，随后又重复一次已执行的 derive_rules，再经 extract/qualify/contribute/reconcile/assemble 闭环；正确但心智与时延偏高。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
