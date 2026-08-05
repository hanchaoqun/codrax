# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T05:02:51Z
- sweep_start_ts: 20260804-220248
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_multi_member_set_count_caveat | FAIL | eval/results/qf_multi_member_set_count_caveat-20260804-220251 | answer_regex,answer_contains | none | 205s | 23 | read=7,repo_map=5,list=0,trace=0,source_lens=5 | midloop=4,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | SCOPELINEAGE1 生产正证：相对 B93 的 16 次 lens、28 个 Explorer 轮、405s、42% context，降为 5 次 lens、8 轮、205s、23%，无关全仓 scope 扩散消失。但 typed observation 已明确 `constant complete=true count=30` 且 `len(members)=30`，模型因分页重叠与手工扣减只交付 24 项，completion/Finalizer 仍放行，最终漏 `KindExternalArtifactDecoded` 等 6 项。确认 EVAL-B94-LENSPARITY1：请求绑定的 executable complete lens 没有把精确成员等值接到 principal rowset/answer obligation。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260804-220251 | answer_regex,answer_contains | none | 613s | 34 | read=17,repo_map=2,list=0,trace=0,source_lens=0 | midloop=19,inv=11/5,fin_reject=7,unavail=0,prune=0 | fail | WAIVERWIRE1 生产正证：首个 `principal_span_waiver=no_directed_path` 被接受并发布 exact boundary，不再出现 string-tail 丢字段矛盾。证据池同时持有 13 条 grounded direct call 边及一条 grounded `buildAnalysisIR -> gate.RunWith`；但 Mermaid 合法 inline-code 标签如 ``participant buildIR as "`buildAnalysisIR`"`` 被 endpoint resolver 原样保留反引号，所有真边一起报 `call_edge_unproven`，7 次成文 reject 后无可用答案。确认 EVAL-B94-DIAGRAMCODEMARK1。34 个 Explorer 轮、4 次 dispatch、19 次 midloop 对同一 endpoint 重复读/结案，登记 EVAL-B94-CALLFANOUT1=P1 待独立归因；不能用放宽 call gate 或扫描答案 prose 消除。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
