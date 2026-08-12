# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T03:15:45Z
- sweep_start_ts: 20260811-201543
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260811-201545 | answer_regex | none | 153s | 24 | read=3,repo_map=5,list=1,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | All six visible call arrows are source-grounded and the answer correctly identifies walker as the file-discovery producer. However it calls `run -> collect_files` and the later file loop “two parallel branches” while the same sentence also says “先…再…”. Source shows a synchronous `let files = collect_files()` followed by the loop. The typed relation capsule already states fan-out does not prove concurrency/order, so this is model fluctuation over accurate context; do not add a prose scan or case-specific hard gate. |
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-201545 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 230s | 29 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit 7ms window, target five-state account, typed wakeup chain, cross-CPU topology, semantic VerifyClass seat, actual-vs-eliminable axes, scheduler-supply seat, off-chain background demotion, causal projection and supplement all survive. But the analyzer put the provisional claim “为什么…导致 5.8ms 阻塞” into sub_topics, and the system reissued it as a typed explore lane. Final prose then says the full 5ms sleep was completely determined by worker execution, despite also disclosing `direct blocker=unproven` and wake occurring before span end. This is model-planning text being over-authorized by system context, not a trace_query calculation gap. One Mermaid compatibility repair succeeded without answer loss. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit disposition

- `B598-RUNTIMESUBTOPICAUTHORITY1/P0-redline`: analyzer `sub_topics` are model planning labels, not runtime facts. For one named target in one user-anchored explicit window under `causal_diagnosis`, collapse them before persistence/task compilation; the deterministic Trace lane still expands all evidence dimensions.
- Rust fan-out wording remains a model-variance witness. The prompt already carries a precise topology boundary that fan-out proves neither parallelism nor temporal order. No second duplicate contract, prose scan, or fixture-specific validator is justified from this run.
- Trace root seats remain on-chain only. `VerifyClass` and target runnable delay are ranked; adjacent sleep and aggregated pressure stay context/background. The failure is the unsupported direct-blocking mechanism sentence, not loss of the dual root-cause dimensions.
- Neither case used malformed-JSON recovery, an old answer draft, or a system-authored answer. Both remained below four minutes.
