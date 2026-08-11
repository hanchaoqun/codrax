# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T20:54:19Z
- sweep_start_ts: 20260811-135418
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-135419 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 144s | 29 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 5.000..5.007s 窗、确定性补采和 Trace 因果投影均保留；正文同时给出 4.600ms 链上唤醒前占用与 0.800ms runnable 调度供给两个维度，并将帧因果、直接 blocker、holder/waiter 明确限定为未证。背景未被加冕为主因，B544 机理边界生效。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260811-135419 | answer_regex,answer_contains | none | 447s | 30 | read=28,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=7/1,fin_reject=3,unavail=0,prune=0 | fail | B568 已让精确 provider 关系进入证据门，反向 `LoopController -> implementer` 被正确拒绝；但合法 `classDiagram` 的 `<\|..` 边没有进入统一 ParseEdges。带正向 type_relation anchors 的类图被报 `typed_anchor_without_visible_edge`，模型删掉 anchors 后同一 12 边类图反而通过，形成关系校验绕过。接受后还整段附带无边第一稿，重复主体并暴露内部校验恢复过程。活动模型流持续 447s 后正常完成，没有按累计约 4 分钟降级或系统代答。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 2/2; human: 1/2.
- `B568` has a production-positive half: exact typed implementer authority is now available and reversed edges remain fail-closed. It is not closed because class-relation syntax is invisible to the common edge parser.
- New `B569-CLASSRELATIONEDGE1/P1-high`: parse valid Mermaid class relations into canonical semantic direction before body/anchor/evidence validation. UML heads determine direction (`Base <|-- Child` and `Base <|.. Impl` both mean `Child/Impl -> Base`; the right-headed forms keep left-to-right direction). Preserve the authored Mermaid bytes and relation enum; do not infer relation kind from labels or add edges.
- New `B570-FIRSTDRAFTDOMINANCE1/P2`: a later validated answer that structurally supersedes a rejected draft can still receive the whole rejected draft as an audit appendix. This is not a four-minute timeout fallback, but it makes a healthy final answer look degraded and duplicates user-visible content. Any suppression must be based on typed structured-carrier dominance, not prose similarity; until that is proven, keep the appendix behavior and treat B569 as the retry-reduction root fix.
- Trace invariants remain unchanged: explicit windows, deterministic supplements, on-chain-only main-cause election, dual occupancy/eliminable axes, and model ownership of conclusions all survived.
