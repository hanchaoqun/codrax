# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T06:24:56Z
- sweep_start_ts: 20260830-232454
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-232456 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 170s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B1474 的角色已在 schema 中，但分析器仍把“确定性工作跟目标的链上关系、入链证据和计时”只铸成 relation_path + causal_contributor_set，没有启用 runtime_work_relation。最终模型虽提到 VerifyClass 0.285ms，却把 NetworkService runnable、CookieMonster sleep/runnable 等调度状态统称为“链上确定性工作”，并把只有 host→target 唤醒锚、缺 semantic-completion/target-wait binding 的 VerifyClass 定谳成“均位于链上”。系统投影随后才正确披露关系仅有凭证、语义完成与延迟机理未证；投影不能替模型作答。显式窗、4 次 typed query、根因榜、双账户、自动补齐和 Trace 因果投影均保留。确认 B1475：分析器 typed 角色分流教学仍缺 relation_path 与 runtime_work_relation 的优先级/并列规则。 |
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260830-232456 | typed_inventory_rowset,dimension_substring,answer_contains | none | 1601s | 59 | read=11,repo_map=4,list=1,trace=0,source_lens=4 | midloop=7,inv=29/2,fin_reject=4,unavail=0,prune=0 | fail | 可见恢复稿最终列对 12 条、2/2/8，但流程不可接受：29 次 completion、7 次 midloop、4 次 finalizer reject、3 次 patch，1601s 后仍以 degraded_answer_checks_skipped 失败。核心矛盾是同一 Greeter/Version/Animal/Dog/Service 精确行被多次 lens 观察后，exact universe 裸 append 成 13 行；模型提交 8 条唯一声明时验证器固定报 covered=8/missing=5，甚至诱导模型重复和虚构 phantom 行。确认 B1476：按 typed declaration coordinate canonicalize 重复精确观测；同线不同身份和同名不同位置必须 fail-open 保留。另有 extend Cart 在可见草稿中存在却连续被 obligation roster 判 missing 的独立 B1477，需继续定位结构化 patch/row identity 链路。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
