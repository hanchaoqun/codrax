# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T12:29:56Z
- sweep_start_ts: 20260805-052955
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260805-052956 | answer_regex | none | 191s | 21 | read=2,repo_map=3,list=0,trace=0,source_lens=1 | midloop=6,inv=1/0,fin_reject=3,unavail=0,prune=0 | pass | Final prose correctly distinguishes the Python facade, `_fastlex` native module, PyO3 wrapper at Rust line 40, public Rust core at line 10, and pure-Python fallback; no degraded output or role reversal. The diagram correctly uses `Note over` for the unproved Python↔PyO3 binding. Evidence handoff still contains only two call rows (native/fallback Python calls): the already-read wrapper→core call at line 42 was not emitted, and the bare sink `tokenize_bytes` lets completion stop at an ambiguous same-tail endpoint. The answer is factually correct, but its “complete chain” proof is incomplete; filed separately as EVAL-B107-ENDPOINTAMBIG1 rather than weakening the diagram gate. |
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260805-052956 | primary_answer | none | 411s | 30 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | fail | Explorer supplied all five exact typed calls and the capacity guard. The first diagram lacked edge_anchors and was correctly rejected; the next patch added all anchors and exact operation messages, but every edge was falsely rejected because grounding had canonicalized AnchorSymbol to qualified callees (`VisitService.schedule`) while the class-participant resolver compared it byte-for-byte with short message operation `schedule`. This contradiction triggered 7 rejects/patches (including three model JSON-string mistakes), removal of the structured diagram, and a system-preserved copy of the original model diagram. The prose path/capacity location is broadly correct, but the terminal claim says `AuditLog.record` writes to a database whereas the source only calls `System.out.println`; runner is a false green. Validator defect filed as EVAL-B107-DIAGRAMOP1; unsupported DB wording remains model-fluctuation replay watch because precise stdout evidence was present. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B106-CALLEDGEID1`: unit/full-package verified; B107 no longer shows duplicate-amendment loops or degraded polyglot output, but this replay emitted non-sparse Python rows and therefore is only a partial production witness for the sparse-carrier arm.
- `EVAL-B107-DIAGRAMOP1`: confirmed product gap and dominant retry cause. The hard gate consumed a different exact projection of the same grounded call anchor than the prompt taught, so legal class-participant diagrams were impossible after production normalization. Fix must project the operation tail from that same typed qualified anchor; owner and ambiguity checks stay fail-closed.
- `EVAL-B107-ENDPOINTAMBIG1`: confirmed follow-up gap. A semantic sink concretized to a bare same-tail symbol can terminate at a wrapper instead of the requested implementation; preserve as next-batch endpoint identity work, not a PyO3 special case.
- `EVAL-B107-JAVACLAIM1`: single model-fluctuation witness. Typed evidence explicitly says stdout, so do not add a prose scanner, answer normalizer, or system-authored replacement.
- No Trace case ran in B107, and no Trace explicit-window, causal-projection, auto-supplement, or two-axis root-cause code changed.
