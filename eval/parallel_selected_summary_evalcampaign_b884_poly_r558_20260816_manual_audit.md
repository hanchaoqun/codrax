# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T09:09:38Z
- sweep_start_ts: 20260816-020937
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-020939 | answer_regex | none | 238s | 26 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | Python `FastTokenizer.tokenize` → `_fastlex.tokenize_bytes` → pyo3 wrapper → Rust core, plus ImportError/`_tokenize_slow` fallback, are all present and line-grounded. The first optional sequence graph overclaimed the binding boundary as direct calls and was correctly rejected; the model removed it, which is acceptable because this request did not require a diagram. The final prose's shared-library/install wording is an inference rather than an independently cited build fact, but it does not change the requested chain/fallback result. No system-authored edge or relation appeared. |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-020939 | answer_regex,answer_contains | none | 330s | 40 | read=12,repo_map=1,list=0,trace=0,source_lens=0 | fail | B884 did not production-close. The first typed flow downgrade queued two `flow_navigation` pending reads, but the same dispatch never consumed them (`forced_read surgical` absent; terminal stats `pending_reads=2`). Worse, the returned hint still said to use broad repo_map/grep first, so the model spent two extra grep turns before manually reading the already-known production registration window at `cmd/root.go:4315/4319`. It then made six completion calls across 24 iterations and still disclosed `finalizer` as an unproven disconnected boundary. Literal values, table, registration rows and full-vs-patch behavior are useful, and B882 still publishes the dimension supplement exactly once, but the requested finalizer relationship is not actually closed; runner PASS is therefore insufficient. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
