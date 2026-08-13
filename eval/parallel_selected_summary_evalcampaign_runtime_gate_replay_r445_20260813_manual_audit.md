# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T17:25:22Z
- sweep_start_ts: 20260813-102521
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260813-102523 | answer_regex | none | 152s | 24 | read=2,repo_map=3,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 正文完整回答 Python→`_fastlex`→PyO3 wrapper→Rust core 与纯 Python fallback。首稿四条自绘伪 call 被精确拒绝；模型随后采用 typed skeleton，终稿保留三条有证据的业务关系边，Mermaid 合法可渲染。系统没有代画或改写结论。 |
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260813-102522 | write_apply,answer_regex | none | 235s | 23 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=2,prune=0 | honest-unverified | 生产修复与 false/0/空串回归正确，`make check` 仍只提供 source-static 证据。宿主无 Node，但累计行为合同 verify-only 结束后的第二 bridge 仍生成 4 个 JavaScript probe 的 `changes=[]` 计划，重复 source-static 验证后诚实未验证；没有静态洗绿，但存在不可执行补证循环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- B732 第一入口修复没有覆盖“累计行为合同 verify-only → probe planning”第二入口；该入口只消费了
  `verification_probe_required=true`，没有再次核验 typed target path 对应的 exact runtime 是否存在。
- 根修落在唯一 probe-planning bridge：所有非辅助 `ExpectedPaths` 都必须能映射到受支持的 inline
  runtime 且当前宿主确实可执行，才允许创建 mandatory probe plan；否则沿既有诚实未验证出口结束。
- 判据只读 controller-owned path、源码角色、语言族和 PATH/受支持 venv，不扫描用户问题、源码、命令、
  模型思考或答案。Read/Trace 路径不变；active stream 不能因 4ms 固定年龄降级。
