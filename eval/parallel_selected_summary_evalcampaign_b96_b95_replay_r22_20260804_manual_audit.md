# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T06:26:27Z
- sweep_start_ts: 20260804-232625
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260804-232627 | answer_regex,answer_contains | none | 239s | 24 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=2/0,fin_reject=3,unavail=0,prune=0 | fail | B95-B production 生效：Explorer dispatch 3→1、read 16→4、iter 27→13；但答案仍把已证 `gate.Run -> RunWith` wrapper 写成“需进一步确认”，并把同层 fan-out 列表当主链。确认 ENDPOINTFOCUS1。三次 Finalizer reject 来自 participant/edge identity 修补，未再出现旧 code-mark 污染。 |
| 1 | qf_multi_member_set_count_caveat | FAIL | eval/results/qf_multi_member_set_count_caveat-20260804-232627 | answer_regex,answer_contains | none | 953s | 46 | read=3,repo_map=23,list=0,trace=0,source_lens=23 | midloop=17,inv=8/1,fin_reject=2,unavail=0,prune=9 | fail | 一次 Explorer dispatch 证明 B95-B 未重开 sibling；但 complete function lens 合并 production+test（56），默认 principal production rows 仅 5，exact parity 无法闭合，继而 23 次 lens/32 iter。终稿错误列 56 个 function（含 51 个测试函数），runner 动态生产函数值 5 未绑定。第三次成文前 normalize 单核高 CPU；sample 显示逐 item/citation 重复 `preEmitStableAggregateFacts -> answerSurfacePlan -> normalizeAggregateFactsForTypedExclusion -> 全图 exclusion census`，确认 CLASSPARTITION1 与 ANSWERPREEMITPERF1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
