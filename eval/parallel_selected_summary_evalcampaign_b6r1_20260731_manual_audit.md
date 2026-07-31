# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T16:31:20Z
- sweep_start_ts: 20260731-093119
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260731-093120 | write_plan,write_patch_oracle | none | 97s | 18 | read=3,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | ChangePlan 仅含 Main.java 第16行 `retrun→return`；kind=patch、一个 replace edit、一个 slice，`git apply --check --recount` 通过。两次 plan repair 均因模型把 Python subprocess 代码误标为 Java probe；最终改用 Python 调 javac 后通过，属于低优先级过程波动。 |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260731-093120 | answer_regex,answer_contains | none | 138s | 22 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=5 | partial | 默认值50、YAML `*int` 映射、两阶段合并和 code<YAML<CLI 均正确；但 6 条已提交引用只有默认值一条留存。accepted positional `support_refs` 因一个 `.yaml.example` 路径不可解析而使三行全部失去 location，完整 Markdown 表的 typed 行又未计入 citation 使用集合，5条证据被清理。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual Findings

### patch_java_typo

- 计划范围正确：只修改 `Main.java`，只替换第 16 行，未夹带格式化、测试文件或相邻逻辑变化。
- 原始 patch 的 hunk 可应用；校验时必须使用 `jq -jr`，避免 `jq -r` 在已有终止换行后再追加一个空行而制造假失败。
- `verification_probes` 前两次失败不是产品计划语义错误，而是模型将 Python `subprocess.run(...)` 代码声明为 `language=java`。repair pack 正确 fail-closed，第三次以 Python probe 成功。记录为 `EVAL-B6-W1/P2`，等待跨语言写计划复现后再优化软指导。

### qf_config_precedence

- 事实核对：
  - `cmd/root.go:88` 定义 `defaultMaxSteps=50`；
  - `internal/config/runtime.go:365` 定义 `PipelineMaxSteps *int yaml:"pipeline_max_steps"`；
  - `cmd/root.go:2565-2566` 在 YAML 非 nil 时更新 merged 值；
  - `cmd/root.go:2664-2665` 仅在 CLI flag 未显式变化时回填 merged 值；
  - `codrax.yaml.example:20` 与 `:485` 分别给出优先级和默认值说明。
- 第一次 evidence batch 中 `cmd/root.go:649` 未读而被拒绝是正确 fail-closed；`anchor_kind=call` 用于字段条件导致 2565 行失败，改为 condition 后成功，也不是产品 GAP。
- 完成调用对 decorated member 无 support_refs 的拒绝正确，第二次采用 positional refs 后闭包成立。
- 真 GAP 是最终 citation wiring：
  1. positional support refs 本来是一成员一槽，但解析器先丢弃不可识别的 `.yaml.example`，再用压缩后的 bare refs 数量判断是否可按位置绑定；一个坏槽导致其他有效槽也全部丢失；
  2. 模型已提交完整 Markdown 表和 6 条 citations，但 table text 没有 item-level `citation_ref`，unused-prune 将 5 条删掉；
  3. 现有源码定位补充只承载 localization owner，不等价于本题三层机制证据链。

## Batch Q Decision

- `EVAL-B6-C1/P1`：positional support_refs 必须逐槽独立解析，单槽未知格式不能毒化兄弟槽。
- `EVAL-B6-C2/P1`：支持常见声明式配置模板路径 `*.yaml|yml|json|toml|ini|xml.(example|sample|template|dist|default)`；不把 `source.go.example` 或任意 `.example` 放宽为源码。
- `EVAL-B6-C3/P1`：unused citation 清理时，把“accepted typed enumeration row 已由同一 Markdown 表完整呈现”计为该 row citation 已使用；保持表格 `Items` 为空，不改表格文字、不生成新成员、不用用户输入或答案关键词铸权。
- 不触碰 Trace 请求分类、显式时间窗、causal projection materialization、根因排序、唤醒链、窗内可消除量或自动补采。
