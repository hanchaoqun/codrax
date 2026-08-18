# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T17:51:38Z
- sweep_start_ts: 20260818-105136
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260818-105138 | write_apply,write_patch_oracle,answer_contains | none | 102s | 25 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 单文件、单行 `retrun -> return`；实际运行 `go test -json ./...` 并通过，changed-path coverage 与工作树审计均为绿。成功工作树因 eval 的 keep-on-success oracle 配置保留，不是生产 L5 清理失效。仅有 dispatch=9 的轻微效率提示。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-105138 | answer_regex,answer_contains | none | 567s | 44 | read=11,repo_map=1,list=0,trace=0,source_lens=0 | midloop=12,inv=9/1,fin_reject=1,unavail=0,prune=0 | fail | “无 `buildAnalysisIR -> gate.Run` 有向路径”与最终图方向正确，但 Explorer 30 轮重复索取已经自动生成的 parser-owned 函数体证据；系统又把 `risk.Evaluate`、`hdp.Plan` 的引用分别错绑到后续行，并把列表中已经可见的 12+2 条关系锚再渲染一遍。摘要提到 `counterfactual.Expand`，结构列表却遗漏。Runner 的字符串 oracle 未覆盖这些事实错误与答案冗余。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### 1. Write 案

- 计划只有一个 `kind=patch` 变更，目标只有 `main.go`；实际 diff 仅修复一处拼写，`main_test.go` 未改。
- Verify 执行项目原生命令 `go test -json ./...`，exit=0，`changed_path_coverage main.go` 由
  project runner 覆盖，最终 controller 以 `all_verified` 收口。
- 日志中的 worktree preserved 来自 eval 为检查应用结果而显式启用的 keep-on-success，不代表正常产品路径
  违反“退出必清理”的红线。

### 2. Call-chain 案

- 源码真相是 `buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> gate.RunWith` 两条独立边；不存在
  `buildAnalysisIR -> gate.Run` 有向路径。最终摘要和 Mermaid 对此判断正确，未造假链。
- `requestedSubTopicCallableBodyDowngrade` 只认可 Explorer 手工 evidence，未消费系统已经生成、且带有
  `repomap_selected_callable_body_call + DerivedFrom + already-read` 权威的函数体内调用证据。模型因此在已经
  读完 `Compile`/`Amplify` 正文后仍重复补证，形成 30 个 Explorer 迭代、12 次 midloop 注入和 565 秒耗时。
- 调用规范化把源码中的 `risk.Evaluate`/`hdp.Plan` 压成短尾 `Evaluate`/`Plan`。成文机械修复无法用完整
  item endpoint 唯一匹配这两行，最终把二者错绑到 line 2539/2543；这是系统 repair 引入的错误。
- principal 列表项目已经逐条显示 `caller -> callee`，同块 `edge_anchors` 仍被 renderer 机械追加为第二套
  关系 bullet，造成模型答案被系统重复扩写。anchor 应继续承担 authority；只有关系没有任何可见结构行时，
  才需要 fallback 呈现。
- `counterfactual.Expand` 只留在摘要，没有进入结构化函数清单，属于模型成文遗漏。本轮不为该单例增加答案
  关键词硬门；先关闭上述三个确定性系统 gap 后再复放。

### 3. 不变量

- 本批没有修改 Trace 查询、显式时间窗、因果投影、自动补齐、根因排序或活跃流终止策略。
- Trace 主因仍只能来自 typed on-chain 证据；邻近/背景仍只作支持。系统不代写模型结论、关系或图。
- 活跃字节流不得因固定 4ms 内尚未形成 answer_document 而降级；本轮两案均未观察到该行为。
