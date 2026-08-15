# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T14:53:26Z
- sweep_start_ts: 20260815-075325
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260815-075327 | write_apply,write_patch_oracle | none | 133s | 24 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B834 获生产正证：根目录 Python TestSurface 执行 `python3 -m unittest discover -v`，4/4 通过；四个 typed behavior probes 也通过，changed path=`covered/project_runner/target_behavior`，首次 verify 后 `all_verified`，零 replan，未再出现空模块 `ValueError`。 |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260815-075327 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 244s | 29 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B835 主目标获生产正证：下游只见 `candidate_* + authority=pretriage_model_extraction`，最终未把预检的同步 RPC/VerifyClass 当主因；typed CPU2→CPU1、0.800ms runnable、4.6ms 业务 span、binding/frame 因果未证、双轴与 Trace 因果投影均保留。残余一：覆盖边界把所有“等待型自身状态”笼统说成无排序席，与同页正确的 target-self runnable #1 席冲突（B836，确定性系统 gap）。残余二：Analyzer 把已有 nested 字段 `is_dimensioned_answer` 又发到顶层，PerfTriager 把 observations+residue 错包为 string，造成 3 轮分析/2 轮预检；属于 JSON 教学/typed 容错效率债（B837），但最终零成文拒绝/修补。正文的 “CPU 空闲” 可由 CPU1 的 idle→app 边界行与 typed CPU1 idle 区间支持，不过写成泛指 CPU 且该 lane coverage=partial，记录为措辞校准观察，不据此硬改答案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Machine result: `2 PASS / 0 FAIL`; human result: `1 pass / 1 partial`.
- B834 closes on production evidence. B835 closes its authority/context-pollution objective: pre-triage candidates no longer outrank deterministic trace rows, while business semantic spans and the causal projection survive.
- B836 is a typed-category wording contradiction, not a ranking defect: only target-self wait-on-counterpart symptom families must stay off the board; target-self runnable/D/IO and positive compute-supply rows remain decomposable candidates.
- B837 is a generalized structured-emission cost gap. Any repair must operate on schema-known duplicate/misnested fields or valid JSON carriers, never extract intent from request/model prose and never silently choose between contradictory typed values.
- Both streams remained active beyond 4ms/4s/4m and completed normally. No fixed-age fallback, stale-draft recovery, empty answer, or system-authored conclusion occurred.
