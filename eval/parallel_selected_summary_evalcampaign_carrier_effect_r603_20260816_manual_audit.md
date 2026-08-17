# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T02:18:55Z
- sweep_start_ts: 20260816-191853
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-191855 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 172s | 35 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | B953 is production-positive: after one precise schema retry Analyzer retained `bounded_effect_verdict`, so the finite question did not expand into a causal roster/projection. The final answer copied running=157.248ms, runnable=5.604ms, sleep=70.338ms, D/io=0 and total=233.190ms exactly, and kept the model-owned verdict `policy ceiling present; target binding unproven`. Runner FAIL is a narrow legacy word-order regex false negative. One model sentence says CPU4 has no same-window frequency record even though the typed matrix and system juxtaposition publish 558/640MHz; the missing fact is target-slice overlap/binding, not window frequency. Existing Finalizer guidance already states this exact distinction, so one occurrence remains model-deviation/watch rather than a new prose hard gate or system rewrite. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260816-191855 | answer_regex,answer_contains,mermaid_edge_count | none | 481s | 43 | read=11,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=6/0,fin_reject=7,unavail=0,prune=2 | partial | B954 reduced reads 22→11 and mid-loop hints 17→7, and the final diagram retained three exact stage-precedence edges plus three grounded local call edges. It still leaves `Mutable` and `BusContext` disconnected while prose overclaims full stage-to-carrier flow. Production inspection shows the new complete-argument navigator selected bare same-owner helpers such as `answerDocumentPatchBaseAvailable(ctx, e.mu)` before a qualified handoff such as `ctxbuilder.BuildAgentContext(o.busCtx, ...)`. Seven finalizer rejects then spent effort reconciling honest unproven boundaries. Confirmed B955: carrier navigation needs a parser-owned cross-owner handoff preference; it must still create no evidence/edge/answer. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap decision

- `B955-CARRIERHANDOFFRANK1` (P1, confirmed): a complete carrier argument is not automatically a useful component boundary. The v2 navigator gave every argument occurrence the same top rank, so lexical file order could prefer a bare same-owner helper over a differently-qualified receiving API.
- General fix: within the already typed, bounded carrier-argument candidate set, prefer parser-owned calls whose receiving endpoint has a distinct qualifier. Bare helper calls, `this`/`self`/`super`, the current owner, and a receiver containing the carrier binding stay local-use rank. This is a soft read-coordinate rank only; Explorer must still inspect and emit exact evidence, and no proved bridge means the requested relation remains unproven.
- Cross-language scope: the same punctuation argument parser and `SupportedReadLanguages` matrix cover Go, Java/Kotlin, Python, JS/TS/ArkTS, Cangjie, C/C++, Rust, Swift and the remaining supported readers. No language/case name table is introduced.
- Trace boundary: H4 confirms finite effect scope and exact state accounting. No full Trace causal projection is required for this finite verdict; causal-diagnosis cases retain projection and automatic supplementation unchanged. No request/model/final-prose scan and no system-authored conclusion/relationship/diagram is added.
