# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T00:41:40Z
- sweep_start_ts: 20260802-174138
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260802-174140 | write_plan,write_patch_oracle | none | 56s | 19 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确单行 patch；两次 typed 修复分别纠正不支持的 bash probe 与 old_text 缩进；plan-only 未修改仓库字节。 |
| 1 | github_issue_commons_lang_random_ascii_symptom | FAIL | eval/results/github_issue_commons_lang_random_ascii_symptom-20260802-174140 | write_apply,answer_regex | none | 319s | 19 | read=16,repo_map=3,list=1,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | PATHID1 已覆盖；三份计划均真实 apply 后执行 `make check` 并诚实失败。第三轮已读 checker，却把 `0x3B1` 错判为匹配 `0x(?:4e00|370|400)`，属模型理解波动；系统最终按预算 blocked，未伪造 verified。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### `patch_cpp_typo`

- 计划最终只修改 `main.cpp` 一行：`retrun` → `return`，与请求和字节证据一致。
- 首次计划中的 bash probe 被 typed schema 拒绝；第二次 `old_text` 的 tab/space 不一致被精确匹配拒绝；第三次收敛。这是可解释的规划纠错，不是 runner false green。
- 该 case 为 plan-only；fixture 仓库 `main.cpp` 无 diff，未越权 apply。
- 系统上下文足够：文件、行号、当前字节和修复反馈均精确，不需要新增硬规则。

### `github_issue_commons_lang_random_ascii_symptom`

- PATHID1 已真实生效：三轮计划、active slice 与 actual diff 均使用同一 canonical path，没有再次出现 `patch_effect_path_outside_plan_scope`。
- 三轮改动都进入隔离 worktree，并各自执行 repository-declared `make check`；结果均为 failed，Java probe 因 `javac` 缺失保持 unavailable，最终 changed paths 均为 `uncovered`。系统没有把静态 token 检查或失败结果提升成行为成功。
- 第三轮 planner 已读取 `tests/check_random_string_utils.py`，但声称 `0x3B1` 能匹配 `0x(?:4e00|370|400)`。这是对可见正则的直接误读；不能用 `0x3B1`、Java fixture 或答案关键词增加 case-specific hard gate。
- workflow 因全局步骤预算耗尽而 blocked，失败是诚实且可恢复的；本轮没有成功 proof，因此 CAPCAL1 尚未获得 live success-path 验收。

### 上下文闭包审计：`EVAL-B47-REPLANPROOF1b`

本轮没有触发错误终态，但冷读 durable plans 与构造代码发现一个独立 P1 通用缺口：恢复点之后，`stampCumulativeVerificationScope` 只携带直接 retained plan 的 ID 与直接路径；合同和探针已经递归，来源与路径却没有递归。三轮以上重规划若交替修改文件，更早仍应用的路径可能再次从 verify-only 上下文消失。

修复采用 controller-owned typed 传递闭包：从 retained durable plan 同时继承其 `SourcePlanIDs` 与 `TargetPaths`，再加入 retained plan 自身；当前 planner 提交的 cumulative scope 仍先清空重建，apply scope 仍只含当前计划。没有读取用户/模型原文，也没有扩大写权限。
