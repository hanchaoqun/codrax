# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T11:36:35Z
- sweep_start_ts: 20260805-043633
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260805-043635 | log_regex,typed_operation_terminal,answer_regex | none | 135s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B104 task-level material authority obtained a production positive replay: the terminal event is complete/complete with three recorded receipt/source members, and the visible answer no longer receives the contradictory “材料覆盖未完全验证” prefix. The answer is useful and grounded. Four operation rounds included one failed regex and later ls/head after complete material was already available; retain as an efficiency/model fluctuation witness, not a prose hard gate. |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260805-043635 | answer_regex | none | 230s | 21 | read=4,repo_map=5,list=0,trace=0,source_lens=1 | midloop=5,inv=2/0,fin_reject=3,unavail=0,prune=0 | fail | Prose/list correctly describes Python → `_fastlex` → pyo3 wrapper → Rust core and fallback, but the final diagram omits the wrapper→core→best_merge calls and falsely says they cannot be traced line by line although exact calls exist at `core-rs/src/lib.rs:42` and `:13`. Analyzer first selected call_chain, then the request-verbatim endpoint admission rejected discovered sink `tokenize_bytes` and forced a downgrade to mechanism; the all-language call-edge evidence guide therefore did not activate. Diagram hard gate correctly rejected unsupported arrows and must remain strict. Filed as EVAL-B105-ENDPOINTDISC1. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B104-OPLIST1` and `EVAL-B104-OPTASKCOV1`: closed by production replay.
- `EVAL-B105-ENDPOINTDISC1`: confirmed product gap. The generic fix preserves an ordered concrete endpoint resolved from a semantic current-request role as a non-authoritative investigation target; endpoint existence and every path edge remain fail-closed on grounded evidence.
- No Trace case ran in B105 and no Trace projection, explicit-window, auto-supplement, or double-axis code changed.
