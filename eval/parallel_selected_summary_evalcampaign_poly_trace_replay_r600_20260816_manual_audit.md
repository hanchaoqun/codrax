# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T01:14:57Z
- sweep_start_ts: 20260816-181456
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-181458 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 187s | 39 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Query/ledger 已有 running=157.248ms、runnable=5.604ms、sleep=70.338ms、D=0；终稿只保留 running 并把其余合成 75.942ms。根因是 typed target `.ugc.aweme.lite-17267 (17267)` 未匹配 subject `.ugc.aweme.lite-17267`，有限状态 authority 没进入 finalizer prompt。该题是 finite state + bounded effect verdict，不要求完整 Trace 因果投影；不能用补投影掩盖状态载体丢失。频率 policy ceiling 与 target binding 未证的边界保留正确。 |
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-181457 | answer_regex | none | 458s | 38 | read=2,repo_map=3,list=0,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=5,unavail=0,prune=0 | pass | B948 生产正证：无权/错向关系连续被拒，最终 principal list 自行提交 `_fastlex.tokenize_bytes -> py.tokenize_bytes` register 及完整 call/guard anchors 后通过；可选 Mermaid 因模型映射失败被自行撤回，系统未补边或代写。最终源码引用真实，r599 的空白/虚构 `__init__.py:2` 症状消失。五次成文拒绝、458s 仍属较高模型执行成本；“未读取 `_tokenize_slow` 函数体”与实际探索不符，记轻微模型措辞偏差。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

- `B948-PRINCIPALPATHFACETOWNER1`: production-proven. Typed owner gate rejected missing/wrong relation ownership and accepted the model's later complete relation rows; no system-authored relation or diagram.
- `B950-PATCHCITATIONROWCLOSURE1`: focused/full tests remain green; the previous false blank-row citation symptom did not recur in r600. This replay is symptom-negative rather than an exact production exercise of every drop branch.
- New `B951-TYPEDTARGETPARENSPID1`: confirmed. Parenthesized PID/TID is already inside the typed runtime target field, but the shared deterministic target matcher only accepted bracketed display suffixes. Exact target-state facts were consequently withheld from the finalizer even though they existed in `target_window_states` and explorer aggregates.
- B951 fix accepts typed trailing `[pid]` and `(pid)` forms, checks digits/range and name-tail consistency, and fails closed on conflicting IDs. It does not scan request/answer prose. Regression pins require the exact four-state authority and forbid manufacturing `Trace Causal Projection` for this finite question.
- Analyzer needed three attempts: it first widened a finite effect question to causal diagnosis, then chose bounded facts without the required target-effect dimension, and also placed a source-exclusion phrase in the artifact-citation field. These are soft JSON-teaching/model-burden findings; do not weaken typed validators or hard-fit this trace.
- No empty answer, fixed 4ms/4s/total-age degradation, system answer replacement, or background-to-root promotion occurred.
