# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T05:39:42Z
- sweep_start_ts: 20260804-223941
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260804-223942 | answer_regex,answer_contains | none | 496s | 26 | read=16,repo_map=1,list=0,trace=0,source_lens=0 | midloop=12,inv=5/1,fin_reject=2,unavail=0,prune=0 | fail | Runner false green. The answer correctly says `buildAnalysisIR` calls `gate.RunWith`, but then says `gate.Run` “may exist” although typed source proves `gate.Run` exists and calls `RunWith`. It omits that reverse/parallel wrapper edge from the diagram, and presents about 30 sibling calls as the requested chain. `n0_probe` had already accepted the exact no-path boundary; `n2_validate` and a third Explorer dispatch reread/re-emitted the same endpoint neighborhood. B94-B's old inline-code rejection did not recur, but this draft did not provide a positive inline-code production witness. |
| 1 | qf_multi_member_set_count_caveat | TIMEOUT | eval/results/qf_multi_member_set_count_caveat-20260804-223942 | answer_regex,answer_contains | none | 1500s | 56 | read=7,repo_map=34,list=0,trace=0,source_lens=34 | midloop=23,inv=21/10,fin_reject=0,unavail=0,prune=9 | fail | B94-A stopped the original incomplete roster from silently passing, but the exact typed projection is reached only after aggregate normalization. The model supplied 30 Kind members with stale value=24, so normalization rejected before typed parity could close it. Later sibling nodes lost the bounded request closure, reopened repo-root debt (`cmd`, fixtures, embedprobe, `internal/skill`), ran 34 lenses / 65 Explorer iterations / 9 history prunes, and timed out without an answer. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
