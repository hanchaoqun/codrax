# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T09:53:48Z
- sweep_start_ts: 20260806-025346
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260806-025348 | typed_inventory_rowset,answer_contains | none | 114s | 21 | read=0,repo_map=1,list=1,trace=0,source_lens=1 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | 最终答案精确列出 4 个 `@Entry` 与 2 个 `@Builder`，成员、路径和行号均与 typed principal rows 一致。runner 假红：`@Entry` marker 先命中二级总标题，section 提取器把下面两个三级分组都纳入后按表行计成 6。另有真实 JSON/合同效率 gap：projected schema 允许 principal table 只用 Markdown `text`，但 source-inventory 后置门要求 row-local `items[]` sidecar，造成一次本可避免的成文拒绝；B171-S3 用 typed view 收窄 schema，不改可见答案。 |
| 2 | github_issue_gson_lazy_number | FAIL | eval/results/github_issue_gson_lazy_number-20260806-025348 | write_apply,write_patch_oracle | none | 123s | 20 | read=7,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain | 单文件补丁正确且未改测试/校验脚本；`make check` 仅执行 Python 源码形状检查，因此 changed Java production path 的 capability 诚实保持 `source_static`，控制器正确否决模型 `all_verified` 并以 `unverified/production_verification_source_static_only` 交付。本机 `/usr/bin/java`/`javac` 是无 JDK stub，无法补出行为通过证据。仍有通用 TestSurface gap：无 Maven/Gradle 的裸 Java `main` 测试没有 typed candidate，系统未显式尝试该仓库已有行为面；记录为 B171-JAVADIRECT1，不能把环境缺失拟合成代码失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
