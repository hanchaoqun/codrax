# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T17:55:13Z
- sweep_start_ts: 20260813-105512
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260813-105513 | answer_regex | none | 112s | 24 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 正文保留完整 Python/Rust 调用与 fallback。首稿三条图边均有 typed call 证据，但 `tokenize_bytes (Rust)` 的显示注释污染 endpoint identity，使 wrapper→core 真边被错拒；模型随后撤掉整图。这是展示/身份分层 GAP，不是该条边缺证据。 |
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260813-105513 | write_apply,answer_regex | none | 235s | 23 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | honest-unverified | B732-2 生产正证：无 `proof-probe-plan`、无 JavaScript probe、无第二个补证批；终局直接为 `production_verification_source_static_only`。自动 FAIL 是 eval 不把诚实未验证算绿，不能降低验证杆。源码与回归修复正确；一次 checker 驱动的测试格式 replan 属于 fixture 项目验证面，不是 runtime-loop 复发。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `B732-2` 已生产闭环：累计行为合同第二 bridge 与 source-static 第一入口现在共享 exact runtime
  opportunity；宿主无 Node 时不再铸造 mandatory JavaScript probe plan。
- 新确认 `B733`：节点显示标签中的紧凑语言/角色注释被并入 typed endpoint identity；精确真边
  `py.tokenize_bytes -> tokenize_bytes` 因 callee 显示为 `tokenize_bytes (Rust)` 被误拒并撤图。
- 根修只从 `exact_code_identity (single-token qualifier)` 产生候选，且候选仍须唯一出现在 citable
  typed evidence 中才生效。调用形 `resolve(json)`、第二代码身份、普通业务 prose 全部 fail-closed。
  该逻辑以跨语言身份语法为基准，不按案例名或语言白名单硬拟合。
