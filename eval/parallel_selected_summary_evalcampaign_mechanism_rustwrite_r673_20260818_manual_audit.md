# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T08:34:27Z
- sweep_start_ts: 20260818-013426
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-013427 | answer_regex,answer_contains | none | 156s | 26 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/1,fin_reject=3,unavail=0,prune=0 | partial | B1055-v3 的 typed admission 生产生效，日志显示自动补发 2 条 parser-owned selected-body call；但两条都来自 `cmd_sleep`（`strtol/printf`）。三个 `#if/#elif/#else` 中同名 `monotonic_now_ns` 函数没有 repomap symbol，API calls 无法归属，平台表仍引用定义首行。另有 3 次成文拒绝/2 次 patch，答案正确但证据权限未闭环。 |
| 2 | github_issue_chrono_duration_min_symptom | FAIL | eval/results/github_issue_chrono_duration_min_symptom-20260818-013427 | write_apply,answer_regex | none | 388s | 25 | read=12,repo_map=1,list=2,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 高风险审批门正确暂停，未 apply、未验证。计划本身也不应获批：把 `i64::MAX % 1000` 手算错，`-milliseconds` 对 `i64::MIN` 溢出，所谓 floor 分支仍保留负 remainder，且额外改写 `checked_sub` 缺乏证明。机器 FAIL 不是应降门的误报；当前 apply eval 缺少显式高风险审批车道，无法继续验证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### sr_c_platform_fork

- 生产日志明确出现 `auto-paired 2 parser-owned selected-definition body call evidence item(s)`，证明 B1055-v3 的 `ExactTargets ∪ PrimaryEntities` 入口已闭环；新增事实是 `cmd_sleep -> strtol` 与 `cmd_sleep -> printf`。
- 平台 API 仍未补出。根因不在 admission：C/C++ extractor 只扫描 root 直属声明，tree-sitter 把条件编译分支封装为 `preproc_if/preproc_elif/preproc_else`，因此分支内部三个同名函数 symbol 全丢；全树 call walker 却仍看见 API calls，形成“有边、无 callable owner”的半张图。
- B1057 泛化修向：递归提取预处理容器中的声明，每个同名平台函数保留独立 line/end range；ordinary calls 只在完整 root 收集一次，避免递归重复。C 与 C++ 共用 extractor，二者版本必须同步 bump 以驱逐暖缓存。
- 最终平台表内容正确，但 API 说明仍引用定义首行；本轮只能关 v3 admission，不能关 selected-body 平台权限。下一生产回放必须看到三个分支 API call producer 行。

### github_issue_chrono_duration_min_symptom

- 写分析把新增公开 `try_milliseconds` 标为 public API/high risk，Auto Pilot 在 apply 前要求人工审批。这符合安全红线；runner 因 `worktree/report` 缺失判 FAIL，但不应通过降低风险等级或自动批准来追求绿灯。
- 计划存在实质错误：`i64::MAX % 1000` 应为 807，过程/计划出现 477 等互相矛盾数值；`-milliseconds` 在 `i64::MIN` 上溢出；floor 商与负 remainder 组合仍非规范化；对 `checked_sub` 的额外改动没有来自复现或 oracle 的证明。
- 泛化改进应是边界算术 soft guidance + 可执行 probe：优先语言原生 checked/euclidean division，禁止手算常量充当证明，计划前运行仓库 oracle/最小复现。不能让系统直接生成或替换算法，更不能自动批准高风险公开 API 变更。
- eval 基础设施另有缺口：`MODE=apply` 案没有 typed “预期需要审批”结果和显式隔离审批车道，导致安全暂停被统一记作普通 FAIL。该问题应在 runner 语义层解决，不能修改产品 approval gate。
