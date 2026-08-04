# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T15:34:07Z
- sweep_start_ts: 20260804-083406
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260804-083407 | dimension_substring,answer_contains | none | 53s | 19 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | All five typed rows are present with exact file, line, package and declaration family: one foreign func, three public classes and one extend. No finalizer retry. However the system appended false uncertainty (“部分项证据支持稍弱” / unexecuted drill-down) although the inventory was complete and all five citations were emitted. The soft principal-member checker produced four advisories because two `Cart` rows share a label; this re-confirms `EVAL-B59-INVROW1` as a presentation-authority gap, without invalidating the primary enumeration. |
| 1 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260804-083407 | log_regex,typed_operation_terminal,answer_regex | none | 80s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | True complete: round 1 fetched the full landing page and selected the typed `./user_guide.html` link; round 2 fetched exactly that source. The terminal binds a complete 20-page/118,802-visible-rune receipt to the 248,161-byte source identity and curl locator. The model then summarized the eight chapters; no malformed step, approval pause or lexical false positive remained. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- Runner/human primary correctness: 2/2 PASS.
- `EVAL-B84-OPSTRUCT1` is production-proven: the original witness completed in 80s and the malformed serialized container never reached shell.
- `EVAL-B59-INVROW1` remains open and is now repeatable without a finalizer retry: duplicate visible labels are not enough to represent two distinct principal rows; the resulting false uncertainty is a system-generated presentation problem, not a model conclusion problem.
