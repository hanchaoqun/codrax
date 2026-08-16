# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T23:25:22Z
- sweep_start_ts: 20260816-162520
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-162522 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 166s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式窗 13762.791708..13763.024898、3 次 typed trace_query、Trace 因果投影和自动补采完整；主因只来自链上席。实际占时/业务 span 与规则可消双轴分账，邻近/background 未晋升。首轮成文零拒绝，无 4ms/固定年龄降级。 |
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-162522 | answer_regex | none | 656s | 35 | read=2,repo_map=4,list=0,trace=0,source_lens=1 | midloop=7,inv=1/0,fin_reject=9,unavail=1,prune=0 | fail | Runner regex 假阳性。首个 accepted draft 已按 typed capsule 提交两条 exact registered-export bridge；pre-emit 接受，post-contract 却对同稿报 registration_edge_unproven，触发 235s 第二次 Finalizer。重试后一个 endpoint 被压成同一 Rust node，严格门耗尽后带 caveat 放行；终稿缺 summary、Rust 实现与 fallback 引用错绑、无图却声称“图示中…”。根因 B945 是验证 authority 接线不一致，不是模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- B944 的 selected-endpoint coverage 在生产中确实触发，并让模型补出两条非 call 注册桥；因此 typed receipt 与教学方向有效。
- B945（P0）是 pre-emit/post-contract 同稿异判：前者消费 dispatch-scoped semantic-handoff receipt，后者只看普通物理 source-edge evidence。它制造无意义跨阶段重试并使正确关系稿劣化。
- 本批施工让两层共享同一 receipt view；只过滤模型已写、identity/direction 精确匹配且 from_node/to_node 不同的 standalone registration anchor。Mermaid、普通 calls、partial identity 与 collapsed self-edge 继续原证据核 fail-closed。
- Runner PASS 只表示该 case 的 `answer_regex` 命中，不能覆盖 strict contract exhaustion；是否将该 telemetry 提升为通用 eval verdict 需跨 case 审计，当前不以单例新增全局硬门。
