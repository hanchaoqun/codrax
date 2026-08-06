# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T14:37:16Z
- sweep_start_ts: 20260806-073715
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-073716 | typed_inventory_rowset,dimension_substring,answer_contains | none | 141s | 21 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 真正 declaration inventory 再次保持 principal，2 extend + 2 foreign func + 8 public class 的摘要、正文、路径、package 一致；无 JSON/成文重试。 |
| 1 | qf_architecture | PASS | eval/results/qf_architecture-20260806-073716 | answer_regex,answer_contains | none | 222s | 25 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=4,unavail=0,prune=0 | fail | S3 将 16 次 lens/13 次 completion 降至 1/1，模型主体准确给出 4 主 stage + 2 conditional pre-stage；但 authority ledger 已是 support_only/authority=false，ProjectSourceInventoryPrincipalRowSetAggregateFacts 仍系统铸造 38 个 principal rows，finalizer 被迫 4 次 patch，答案追加 25 个无关类型、write-mode 常量与 MissingPiece/TransportType。确认新的下游 authority leak。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
