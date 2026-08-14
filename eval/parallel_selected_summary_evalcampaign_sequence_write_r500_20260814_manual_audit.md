# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T17:23:50Z
- sweep_start_ts: 20260814-102348
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260814-102350 | write_apply,write_patch_oracle,answer_contains | none | 95s | 24 | read=1,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | 精确只改 `main.c` 的 `retrun -> return`；`make test` 真实编译并运行两次且 applied commit/recovery ref 干净。稳定复现 B813：测试在 retained worktree 生成未跟踪二进制 `main`，终态却只披露 fully verified，未区分提交洁净与验证工作树副作用。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260814-102350 | answer_regex,answer_contains | none | 356s | 34 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=11,inv=6/1,fin_reject=1,unavail=0,prune=0 | partial | 最终事实和方向正确：不存在 `buildAnalysisIR -> gate.Run`，真实端点子图为 `buildAnalysisIR -> gate.RunWith <- gate.Run`；25 个内部调用均有源码引用。首稿图本来就只有两条边，本轮没有进入 B816 缺锚修补生产臂；唯一 reject 是 principal/support facet 归属，patch 保持图不变。仍重复 B689：22 轮探索、6 次 completion 才闭合 no-path，且正文称“25 个”后又把边界包装 `gate.Run` 列为第 26 项，属轻微模型计数/表述偏差，不加 prose 硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
