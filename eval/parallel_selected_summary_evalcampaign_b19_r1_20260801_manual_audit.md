# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T05:25:35Z
- sweep_start_ts: 20260731-222533
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | LAUNCH_FAIL |  | log_regex,trace_attachment,answer_regex,answer_contains | - | 0s | - | read=-,repo_map=-,list=-,trace=-,source_lens=- | midloop=-,inv=-/-,fin_reject=0,unavail=-,prune=- | not-run | Case 的 `../../customlogs/xxx_all.systrace` 按仓库 cwd 解析到不存在的 `/Users/han/customlogs`；仓内 `donghu_tieba_frame.systrace` 与目标样例 SHA-256 相同。属于 eval fixture 路径漂移，不能用于判断 Trace 产品能力。 |
| 2 | read_combo_git_two_diffs_current_code | FAIL | eval/results/read_combo_git_two_diffs_current_code-20260731-222535 | answer_regex | none | 120s | 23 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | 两次提交、diff 与当前链路主体正确；但 typed `diff_clue + current_key_code` 未被识别为 mixed history/current-code，代码文件 member_set 被强制补成“系统按已验证证据补充”表。另有独立事实错误：grep 的 50 matching lines（且 truncated）被模型叙述为 50 个文件；真实匹配为 30 行/7 文件，其中生产调用点 6 个。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
