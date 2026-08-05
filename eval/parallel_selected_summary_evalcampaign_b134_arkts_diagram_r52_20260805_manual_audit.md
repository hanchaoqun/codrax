# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T21:39:43Z
- sweep_start_ts: 20260805-143942
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-143943 | answer_regex,answer_contains | none | 137s | 29 | read=2,repo_map=4,list=0,trace=0,source_lens=3 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Mermaid 正确呈现 analyze→explore→extract→finalize，四段职责与源码引用均准确且零成文重试。但 analyzer 的 `diagram_hint` 经兼容层只剩 `kind=flow`、漏掉 `required`，Finalizer 合同显示图为 optional/architecture；本轮模型碰巧仍画图。最终 `blocks[]` 是 JSON-encoded string，系统无损恢复；三条 edge anchor 多余 metadata 被有界规范化。模型已逐段且逐行引用四个 stage，系统仍重复追加整张阶段绑定表。 |
| 1 | arkts_repomap | PASS | eval/results/arkts_repomap-20260805-143943 | typed_inventory_rowset,answer_contains | none | 155s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 新 rowset 判定真实回放 PASS，4 个 @Entry + 2 个 @Builder 及位置完整；本轮模型直接使用 ordered-list 结构，零 reject/patch，未再出现“列 2/3/4”。两次 read_file 用于行为/源码确认，非不可用调用。答案有精确 surface-term 系统补充、quote 回填 ×4 和泛化矛盾/未钻取 caveat，事实不受影响但可见噪声偏高，记后续 presentation/context 审计。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
