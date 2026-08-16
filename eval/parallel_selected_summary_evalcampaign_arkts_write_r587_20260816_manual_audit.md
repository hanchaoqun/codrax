# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T21:35:02Z
- sweep_start_ts: 20260816-143501
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | PASS | eval/results/arkts_repomap-20260816-143502 | typed_inventory_rowset,answer_contains | none | 113s | 26 | read=0,repo_map=1,list=0,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 最终答案逐项列出 4 个 `@Entry`（Index、ParentComponent、StyledPage、ListPage）和 2 个 `@Builder`（defaultHeader、GlobalCard），文件归属与 typed inventory rowset 一致。Analyzer 早期曾猜测没有 `.ets` 文件，但 Explorer 的 source inventory 在成文前纠正，未污染最终事实；零 finalizer reject、零系统代写。 |
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260816-143502 | write_apply,answer_regex | none | 172s | 25 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain | 一行生产修复正确，只接受 `true/error`，未改测试。`make check` 顶层命令及两个 Python source oracle 均通过；但 fixture 没有用 Node 执行 `tests/js-binding.test.ts`，故 `source_static` 与最终 `accept_unverified` 是正确安全边界，不能为 runner 变绿升级成 target behavior。独立上下文 GAP：ChangeReport 已保存 `make check` 的 executed-command identity，controller prompt 却只显示聚合 `1 result`，模型据此误称无法确认 `check_force_wasi.py` 是否执行。修向是透传 typed 顶层命令及聚合口径，不降低 capability。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

- 自动结果：`1/2 PASS`；人工结果：`1 pass / 0 fail / 1 uncertain`。
- Write 的自动 FAIL 不等于补丁失败。Python checker 会读取 TypeScript 实现与测试源码并用正则/动态 Python 逻辑校验形状，但没有执行 TypeScript 模块；因此真实 Node 行为仍未验证。
- 新立案 `B939-WRITECOMMANDACCOUNTINGCONTEXT1=P1`：顶层执行命令、退出码与 provenance 已在 `ChangeReport.ExecutedCommands`，却被 controller artifact section 漏投影；`total_results=1` 又是顶层 runner 计数，不是 Make 子命令计数。系统应向模型精确显示这两件事，禁止从聚合数反推某个子检查未运行；同时继续明确 source-static 不能证明 target behavior。
- 两案均未出现 malformed JSON、答案消失、Mermaid 语法问题、系统替换模型结论或活跃流固定 4ms 降级。Trace 本批未运行，既有显式窗/因果投影/自动补齐路径未改动。
