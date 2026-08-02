# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T09:52:18Z
- sweep_start_ts: 20260802-025217
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_jsonl_filter_count | PASS | eval/results/data_jsonl_filter_count-20260802-025218 | log_regex,answer_regex | none | 63s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终纯数字 `2` 正确，两个必需材料均消费；但简单 4 行 JSONL 经历 missing-script repair，随后 `emit(count)` 被 Go Result 解码拒绝，再改 `emit_result(count)` 才完成。前者属于模型计划波动，后者是 runner 公共结果通道契约 gap。 |
| 1 | logtri_goroutine_dump | PASS | eval/results/logtri_goroutine_dump-20260802-025218 | log_attachment,answer_regex | log_triage | 114s | 19 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 附件只有一个 `fatal error` header；goroutine 15 是该显式错误栈，87/120 是并发线程快照。triager 却铸成 3 个 peer Errors，final 又断言三者同时 panic、访问同一 map 且 `writeSession` 缺锁。runner 的中英文关键词 oracle 未覆盖 error-occurrence/thread-snapshot 权限边界。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
