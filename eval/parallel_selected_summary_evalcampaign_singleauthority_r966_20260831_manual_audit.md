# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T10:33:16Z
- sweep_start_ts: 20260831-033315
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-033316 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 183s | 41 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail (model semantic category overclaim; system context/projection pass) | 显式 10ms 窗、目标四态、NetworkService→CookieMonster→目标的 typed 唤醒链、链上排序、实际占时/规则可消双账户、VerifyClass 0.285ms、背景隔离、自动补齐与最终 Trace 因果投影均完整。requested-dimension patch 由模型补答“确定性优化链上因果”，但把 NetworkService runnable 与 CookieMonster scheduler delay 称为确定性语义工作；真正 semantic span 的 completion→target-wait 仍未证、规则可消为 0。typed 上下文和系统投影已正确披露边界，按模型语义波动留档，不新增正文关键词硬门或系统结论改写。 |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260831-033316 | typed_inventory_rowset,dimension_substring,answer_contains | none | 287s | 29 | read=2,repo_map=4,list=0,trace=0,source_lens=3 | midloop=4,inv=4/0,fin_reject=2,unavail=5,prune=0 | pass (final answer; process system gap) | B1483 生产转正：最终答案精确保留 extend=2、foreign func=2、public class=8 共 12 条，符号、文件、package 均正确，无 Container/Vehicle/Machine、无 raw alias 重复。过程仍有新合同冲突：一次已接受的三段结构化列表已携带完整 `source_inventory_row_id`、可见 label/path/package，却因 replace_blocks 漏带冗余 `source_inventory_family` 被后置 requested-dimension 检查误报“符号名缺失”，诱导模型再加三张重复表并被 multiplicity 门拒绝；最终靠上一版已接受草稿恢复。B1484 改为由统一 typed source-inventory answer authority 判断完整可见行覆盖。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
