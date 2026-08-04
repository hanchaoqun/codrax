# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T08:39:27Z
- sweep_start_ts: 20260804-013926
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260804-013927 | log_regex,answer_regex | none | 84s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | typed link inventory 从首页定位 user_guide.html；248161 bytes/118802 visible runes 被 20 个连续 material pages 完整覆盖，source/pages 均未截断并有 coverage receipt。最终 8 章摘要与材料相符，无假完整。 |
| 1 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260804-013927 | write_apply,answer_regex | none | 436s | 19 | read=19,repo_map=4,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 首版静态 probe 正确打红 n+1 overflow，replan 后静态 checker/make 通过；无 cargo/rustc 时最终诚实 unverified，且累计 scope 保留前后两计划的 3 个 Rust path。但终稿 nth_back 用 n>=current_length 而非 remaining=current_length-index；先 next() 后 nth_back(2) 会返回已消费元素。平面代码形合同与测试漏掉共享游标的跨方向状态序列。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

### operation：完整材料 receipt 与答案一致

首轮只抓首页后 evaluator 没有签绿，而是消费 typed href inventory 继续获取 `user_guide.html`。第二轮材料为 248161 bytes、
118802 visible runes、20 个连续分页，`source_truncated=false`、`pages_truncated=false` 并携带 coverage receipt；完成裁定与最终 8 章摘要
一致。一次字符串到单元素数组的兼容修复不改变命令、材料或结论。本例人工 PASS。

### write：终态安全，但补丁存在未被合同/测试捕获的状态机错误

第一计划的 Python probe 正确拒绝 `checked_sub(n + 1)`；第二计划改为分步 `checked_sub` 后，Python 静态 checker 与 `make check`
通过。系统没有把它们提升成 Rust 行为权限：cargo/rustc 不在环境，最终三个 Rust path 全部
`changed_path_verification_uncovered`，终态为 `unverified:verification_incomplete`。第一计划新增的 `tests/iterators.rs` 与第二计划改动的
`src/types/list.rs`、`src/types/tuple.rs` 同时存在于 cumulative verification scope，说明 write append-only 验证账没有被 replan 清空。

但人工状态机审计证明代码仍错误。`nth_back` 的有效剩余区间是 `[index, current_length)`；补丁只判断
`n >= current_length`。例如三元素迭代器先 `next()` 令 `index=1`，再 `nth_back(2)` 时剩余仅 2，应返回 None 并耗尽；补丁却得到
`target_index=0` 并返回已经消费的首元素。新增 Rust tests 只覆盖初始态的 past-end/usize::MAX，没有前向消费后反向跳过或反向消费后前向跳过。

写前 `behavior_contracts` 也揭示同根能力缺口：七条主合同是 `checked_add`、`checked_sub`、赋值代码形及单步
`nth(10)->next()`/`nth_back(10)->next_back()`，无法显式携带 setup→action→observation/postcondition 的状态序列和共享游标不变量。
因此这是泛化的 stateful API / protocol / lifecycle 测试设计 GAP，不是 PyO3 或 nth_back 单点特判。

施工方向：给 write behavior contract 增加可选 typed transition sequence（setup/action/observation/postcondition）；write analyzer 在轻量源码证据
显示共享可变状态或有序协议时形成序列合同，planner 用该合同设计至少一条非初始态/跨方向 probe。保持 soft guidance，不从用户或模型 prose
做硬门，不自动生成结论。Rust、C/C++、ArkTS、Cangjie 等缺少 bounded inline probe runtime 单独记能力矩阵债；当前继续 fail-closed，
不得把 Python/source-static checker 升格为这些语言的行为验证。
