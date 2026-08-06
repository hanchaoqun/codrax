# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T16:37:27Z
- sweep_start_ts: 20260806-093726
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260806-093727 | typed_inventory_rowset,answer_contains | none | 93s | 21 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Exact 4 `@Entry` types + 2 `@Builder` functions, with correct thirdparty scope, paths, lines, and exclusion of undecorated `EntryAbility`. S10 added no clean-path loop; finalizer accepted the first native JSON document. |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-093727 | typed_inventory_rowset,dimension_substring,answer_contains | none | 155s | 20 | read=0,repo_map=3,list=0,trace=0,source_lens=3 | midloop=2,inv=2/1,fin_reject=1,unavail=1,prune=0 | pass | Exact 2 extend + 2 foreign func + 8 public class rows, with package/path/citations aligned. First closure copied structural `type` total 10 into the `public class` family while listing 8 members; the exact `value == len(members)` contract rejected it and the model corrected to 8. A subsequent unavailable `repo_map` attempt was unnecessary tool-surface churn. First finalizer emit stringified `blocks`; recovery found only 2/4 structured blocks and correctly rejected lossy repair, then the native JSON retry succeeded without losing answer content. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
