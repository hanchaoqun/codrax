# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T15:54:47Z
- sweep_start_ts: 20260828-085447
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-085448 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 215s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | B1383 production positive: the validated named target plus explicit window removed the discovery-probe dispatch, reducing r890's two explorer dispatches/eight trace queries to one explorer dispatch/four complementary views. The exact 2.000–2.020s window, threadpool-400→network-300→cookie-200→app-100 chain, 11.000ms on-chain IO first seat, three independent 1.000ms scheduling/priority candidates, actual-time/rule-eliminable accounts, background isolation, auto-supplement, and full Trace causal projection all survive. B1385 carrier is also positive: the old three global-unavailable/missing-waker residue rows are absent, while emitted meta/observations identify cookie-200 and the chain; the model's hidden reasoning still briefly misread the local schema, but that text never entered the typed downstream carrier or final answer. Human result remains partial because final prose overstates wake ordering as downstream-work completion/whole-chain control and reverses some sleep-before-wakeup descriptions. Typed context and deterministic appendix state the narrower authority, so retain this as model adherence noise rather than prose scanning, rejection, or system-written conclusions. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-085448 | answer_regex,answer_contains,mermaid_edge_count,typed_diagram_participant_coverage | none | 403s | 36 | read=22,repo_map=4,list=0,trace=0,source_lens=0 | midloop=14,inv=9/0,fin_reject=1,unavail=0,prune=0 | partial | The answer correctly explains the four read stages and the BusContext/Mutable carrier with grounded citations. Mermaid parses, all 6 required typed participants are incident-covered, and one joint participant+relation patch removes one unsupported return edge and replaces only the failed data-flow edge without system-authored nodes, labels, or layout. The resulting graph is evidence-backed but still visually technical: BuildAgentContext is a sink while the bus.Mutable→AgentContext.Mutable transfer is broadened to BusContext→AgentContext, so the reader must reconcile two nearby carrier paths from prose. This is usable but not yet an ideal business-level architecture view. The 22 reads, 503 accepted evidence rows, 14 mid-loop injections, and 9 completion calls are high process churn across two evidence units; no contradictory reject was found in this replay, so record it as a performance/complexity observation rather than infer a new hard gate from one case. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
