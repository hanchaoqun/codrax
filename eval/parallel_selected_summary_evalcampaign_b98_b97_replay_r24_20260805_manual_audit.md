# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T08:02:02Z
- sweep_start_ts: 20260805-010200
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260805-010202 | answer_regex,answer_contains | none | 231s | 25 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=11,inv=3/0,fin_reject=2,unavail=0,prune=0 | fail | B97-A 生效：未读 exact sink 时不能关闭，Explorer 被定向到 gate.go 并证明 gate.Run 存在。最终仍把真实 `gate.Run -> RunWith` 写反为 `RunWith -> gate.Run`，且把 buildAnalysisIR 的同层直调步骤称为“线性调用链”；runner 只钉名称/图形而假绿。no-path capsule 缺少 sink-local 真实边，因为 Explorer 只发射 definition、没有发射已经读到的 wrapper call edge。 |
| 2 | qf_multi_member_set_count_caveat | FAIL | eval/results/qf_multi_member_set_count_caveat-20260805-010202 | answer_regex,answer_contains | none | 572s | 35 | read=6,repo_map=6,list=0,trace=0,source_lens=6 | midloop=5,inv=1/0,fin_reject=4,unavail=0,prune=0 | fail | B97-B 生效：bounded path 直接闭合，6 次 lens 即取得 3 type/5 production function/30 Kind constant。随后模型额外把 51 个 `_test.go` function 发成 principal aggregate；typed production row projection未把这个全在范围外的集合降级，硬校验强迫答案补测试项。修复提示又漏写 principal surface 元数据，模型四轮在 item label/text 间猜测，最终 degraded 出厂。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- `EVAL-B97-CALLENDPOINTPROOF1`: production admission positive, but not closed. Exact endpoint existence is now proven before waiver; endpoint-local topology is not guaranteed to enter the typed capsule.
- `EVAL-B97-REQUESTBOUNDARY1`: production positive and closed for the observed failure. The requested path remained bounded and the prior repo-wide debt cascade did not recur.
- New P0 `EVAL-B98-SCOPEAGGREGATE1`: a model-emitted principal member set whose row-local support is entirely outside the typed principal source scope survives alongside the canonical principal row set and becomes a hard final-answer obligation.
- New P0 `EVAL-B98-REPAIRSHAPE1`: the principal enumeration hard-repair hint names the roster but omits the exact structural carrier requirements (`surface_role=principal`, enumeration facet, member in item label/cell, compatible row-local citation), producing a deterministic four-retry loop.
- New P1 `EVAL-B98-ENDPOINTTOPOLOGY1`: no-path admission proves endpoint existence but does not require the already inspected endpoint-local call topology to be emitted. The finalizer therefore receives an incomplete capsule and can invert a wrapper edge. Any fix must operate on typed call edges/endpoint inspection state, not answer prose.
