# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T00:09:14Z
- sweep_start_ts: 20260810-170913
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260810-170914 | answer_regex | none | 112s | 22 | read=2,repo_map=3,list=1,trace=0,source_lens=2 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | Native/fallback state producer、guard、两个 sink 与 PyO3 registration 均有 typed 证据；正文完整说明 `_fastlex`、wrapper→Rust core 与 ImportError fallback。首图的概念跨语言边被正确拒绝，patch 保留四条可引用的真实源码边。B482 获生产正证。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-170914 | answer_regex,answer_contains,mermaid_edge_count | none | 524s | 36 | read=43,repo_map=3,list=0,trace=0,source_lens=0 | midloop=26,inv=10/0,fin_reject=5,unavail=0,prune=0 | fail | stage authority 已让 completion 只剩 `[Mutable BusContext]`，但相同 typed 缺席集合因无关 closure 变化被重复追问；finalizer 首稿有三条 stage precedence，validator 因多行同阶段别名歧义全部拒绝，后续 canonical `extractor\nStageExtract` 又被 generic resolver 判成双身份，模型按 hard repair 删除后只剩 analyzer→explorer。正文仍声称完整流水线，图文矛盾；runner PASS 为假绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Human verdict: `1/2`。`mr_poly_binding_chain` 关闭 B482；`qf_logic_view_read_pipeline` 未关闭 B479。
- QF 的关系丢失不是模型波动。模型第一稿发射了全部三条阶段边；hard relation gate 的 endpoint resolver 把同一节点上的 `extractor` 与 `StageExtract` 视为两个不同身份，随后修复协议明确要求删掉“未证边”。
- QF 的 10 次 completion 中，前三次对完全相同的 `[Mutable BusContext]` 缺席集合重复 DOWNGRADED；原因是 lane blocker 混入新增 read/evidence/repair closure，而不是 participant 缺席集合发生变化。
- 最终可见的系统锚点补充没有直接篡改模型段落，但重复发布“系统保留内容/第一稿答案”并放大图文矛盾；在关系门修复前不能把这种 fallback 当质量闭环。
