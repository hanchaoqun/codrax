# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T14:26:12Z
- sweep_start_ts: 20260805-072611
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260805-072612 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 193s | 39 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、因果投影、系统补齐和实际占用/规则可消除双轴均在；但 explorer 已在结构化 aggregate 中把 D-state 与 io_wait 跨 row 相加为 17.819ms，并把 wakeup path 升级成同步阻塞因果，finalizer 复用了该错误。runner oracle 未覆盖语义越证。 |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260805-072612 | answer_regex | none | 361s | 25 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=9,unavail=0,prune=0 | fail | 代码主链与 fallback 基本齐，但 9 次 diagram 拒绝/patch 后才删除可选图；每轮机械补发完整成员集，最终仍出现两份“跨语言调用链节点（6）”。若干成员引用错位，且 import/comment 被扩写为 `.so/.pyd` 装载行为。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap decisions

- `EVAL-B115-TRACEAGGAUTH1`（P1，confirmed）：模型探索期 aggregate 是 provisional synthesis，却与 deterministic `trace_query` 行共同进入事实合同；B114 尾界无法抵消前置结构化错误。最优解是在 answer-surface authority 编译点按 typed origin 分层：完整 Trace 投影存在时，不再把未独立绑定的 runtime/system-inference model aggregate 交给 finalizer 或 pre-emit 当事实；原始 payload 留在 Mutable/TurnAArtifacts 审计面。
- `EVAL-B115-ENUMACCUM1`（P1，confirmed）：被拒 draft 上的系统成员补充在每次 patch 前再次生成，形成 10 份候选块、日志和 citation advisory 线性膨胀；最终 dedupe 只清掉 9 份系统域，仍留下模型清单 + 系统清单各一份。
- `EVAL-B115-DIAGRAMCHURN1`（P1，confirmed）：可选 diagram 成为唯一硬拒后，相同 violation 连续 9 次仍只能 patch-first；这是 termination/repair-policy gap，不是“必带又必拒”的逻辑矛盾，因为模型随时可删除可选图，但系统没有让无进展 repair 尽早收敛。
- `EVAL-B107-ENDPOINTAMBIG1`（P1，confirmed second witness）：`FastTokenizer.tokenize -> tokenize_bytes` 在源码上明确存在，typed call edge 也已发射，但 owner-qualified endpoint 与裸 operation 在 diagram validator 中没有唯一对齐，导致 principal edge 被报 missing/unproven。需按 endpoint identity + operation 统一解析，不能按某语言/符号名特判。
