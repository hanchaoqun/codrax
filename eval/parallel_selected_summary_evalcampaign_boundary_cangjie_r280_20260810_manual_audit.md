# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T23:07:51Z
- sweep_start_ts: 20260810-160749
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260810-160751 | typed_inventory_rowset,dimension_substring,answer_contains | none | 101s | 22 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | Typed lens and Principal Enumeration Rows contain all 12 correct rows. Finalizer then crossed the two extend row payloads/citations and lost exact identity for the duplicate native_add rows; one weaker aggregate citation repair moved the ffi citation to the bridge row. This is typed row-identity transport failure, not Cangjie extraction loss. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-160751 | answer_regex,answer_contains,mermaid_edge_count | none | 368s | 38 | read=20,repo_map=3,list=0,trace=0,source_lens=0 | midloop=13,inv=4/0,fin_reject=3,unavail=0,prune=1 | fail | B481 is a direct positive: finalizer rejects fell 12→3 and the four exact boundary rows/nodes were accepted. The final diagram still omits Explore/Extract stage precedence and leaves the four requested agents disconnected while prose claims a complete fixed flow, so the requested logic/data-flow view remains incomplete. B479 remains open. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- `B481-PARTBOUNDRECIPE1`: production-positive and mechanically effective, but not sufficient for answer completeness. It removes the boundary JSON/node-shape trial-and-error without authoring a relation.
- `B479-STAGECLOSEBOUND1`: still open. The shared stage authority exists and three precedence recipes are available, yet exploration/completion does not retain them as the principal requested stage component and the final draft can discard two edges.
- `B483-ENUMROWID1/P0`: all accepted source-inventory principal rows need one stable structured row identity through final authoring. Current guidance requires `source_inventory_row_id` only when the visible label is duplicated. A model can decorate duplicate labels or transpose two unique row payloads, thereby bypassing exact identity and allowing a weaker citation repair to rebind the row. The fix must use typed row IDs, family and source coordinates only; it must not scan item prose, user text, or model thinking, and must not rewrite the model's visible conclusion.
- Trace lanes were not selected in this batch. The proposed inventory fix is isolated from Trace/QFRootCauseTrace and cannot affect explicit windows, auto-supplement, causal projection, on-chain cause election, or off-chain background classification.
