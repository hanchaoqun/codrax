# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T13:38:17Z
- sweep_start_ts: 20260804-063816
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260804-063817 | log_regex,answer_regex | none | 33s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格输出 `2`；1 round、0 repair、0 failed action，完整消费 instructions.md 与 notes.txt。该简短半结构化任务使用有界 custom script，一次闭环，未触发新增合同。 |
| 2 | data_jsonl_filter_count | PASS | eval/results/data_jsonl_filter_count-20260804-063817 | log_regex,answer_regex | none | 96s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格输出 `2`；typed filter 产出 2 include/2 exclude，终态 decisions=6、rules=2、contributions=2、reconcile=pass。selection 参数合同无误伤。过程 8 rounds/1 repair/3 failures：首轮把内置 `jsonl_rows` 当模块 import，被 sandbox 正确拒绝；另两次为 rule/material rank 顺序恢复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
