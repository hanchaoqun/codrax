# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T05:51:07Z
- sweep_start_ts: 20260819-225106
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260819-225107 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 281s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式窗、四节点已证链、11ms 链上 IO 首席、三个 1ms runnable 调度项、20ms 目标睡眠症状、实际占用/规则可消除双轴、自动补齐与完整 E# 均在；背景未晋升。B1217 reader legend 生产生效。模型把 page wait 推成“来自磁盘/建议预取”超过证据，按模型波动软引导，不设正文硬门。后置系统校验附注仍泄漏 typed 事实/席位/confidence，另记 P2。 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-225107 | log_regex,write_apply,answer_regex,answer_contains | none | 835s | 26 | read=10,repo_map=3,list=2,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 确定性多仓路径合同冲突：活动 RepoRoot 已是 bindings-py，read_file/list_files 会把 bindings-py/fastlex/... 规范为 fastlex/... 并成功读取；emit_change_plan 未同源规范化 changes.path 与 project_test_observations.test_path，先报测试文件不存在，后报 patch_path_missing，无法进入 apply。B1214 在第三次同类拒绝后正确触发滚转，避免旧式提交风暴，但不能弥补路径身份缺口。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Machine result `1 PASS / 1 FAIL`; human result `1 pass / 1 fail`.
- Trace 正例证明 B1217 的生产读者面已显著缩短，且没有删除 Trace 因果投影或任何链上证据。模型对
  `fscache_page_wait_on_page_bit` 的磁盘/预取推断缺少设备、文件或块层证据；上下文已明确未证边界，暂按
  模型服从波动处理，只加强软教学，不扫描或替换模型答案。
- 写失败不是模型路径猜错：系统先让同一带仓库标签的路径在读取面成功，再在计划面以另一个根目录身份
  拒绝。应在进入计划去重、存在性、依赖、指纹与测试声明校验之前，共享 read-tool 的精确活动仓库标签
  规范化；真实同名子目录、新建目标和越界路径不得猜测改写。
- 两路活跃流均没有因 4ms、4m 或累计年龄降级。写路 835 秒后是结构校验失败，不是流式超时降级。
