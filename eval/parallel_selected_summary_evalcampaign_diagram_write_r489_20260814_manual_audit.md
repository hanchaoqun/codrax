# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T13:33:02Z
- sweep_start_ts: 20260814-063300
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260814-063302 | log_regex,write_apply,write_patch_oracle | none | 168s | 24 | read=7,repo_map=5,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | API reference 被正确作为只读权威；sync/async 两个 `text_search` 同时从 `GET /v1/memories/search?...` 改为 `POST /v1/search` + JSON body，删除 `urlencode`，项目检查与目标行为探针共 2 项通过，最终 typed 状态 `complete/verified/all_batches_verified`。未改 API reference、未弱化测试。proof ledger 中仍保留一条历史 `source_localization_missing_path` 摘要，但最终 localization=`supported`、support_ratio=1、changed path covered；属于内部历史诊断噪声，不影响本轮交付，继续观察。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260814-063302 | answer_regex,answer_contains | none | 293s | 35 | read=9,repo_map=3,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=2,unavail=0,prune=0 | partial | 最终保留合法 `sequenceDiagram`、四阶段表和可核查引用，九条 typed recipe 均落到图中；两次成文拒绝来自首稿自造的 data-flow/precedence/reply 方向和第二稿把 `-->>` 用于 data_flow，第三稿按 typed recipe 收敛，未见合同互斥。正文仍把可写的 `BusContext` 称为“只读上下文容器”，并把 DAG 后续描述成简单串行，属于架构表述过度简化；机器 PASS 不能上调为人工 pass。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Machine: 2/2 PASS. Human: write pass, read partial.
- No new validator self-contradiction was found. The read case spent two repair rounds because the model initially ignored the supplied typed edge recipes; the same draft converged without weakening or conflicting contracts.
- The most important follow-up came from the independent customer REPL replay rather than these two evals: raw-prose finalizer fallback preserved only a short typed-fact list and omitted deterministic Trace report sections. That generalized lane is tracked as `B800-RAWDETERMINISTIC1`; it is not repaired by adding answer-text keyword gates or by system-authored conclusions.
