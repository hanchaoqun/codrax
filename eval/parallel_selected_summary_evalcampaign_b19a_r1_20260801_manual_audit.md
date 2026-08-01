# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T05:36:39Z
- sweep_start_ts: 20260731-223637
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260731-223639 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 153s | 33 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、根因排序、唤醒链、窗内可消除量、因果投影、coverage 与系统补采均保留；但 explorer 发出 member_set value=19、members=7（含“其他13个线程”占位），归一化层静默改成 7，最终误报“全窗口共有7个”。主 Trace 合同通过，精确枚举事实失败。 |
| 2 | read_combo_git_two_diffs_current_code | PASS | eval/results/read_combo_git_two_diffs_current_code-20260731-223639 | answer_regex | none | 193s | 27 | read=9,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=4/3,fin_reject=0,unavail=0,prune=0 | fail | B19-HIST1 目标通过：不再出现“系统按已验证证据补充缺失成员”主表。另有独立事实清册误差：答案称 5 文件/10 调用点，却把定义计入、漏掉 facet_plan 与一个 answer_intent 调用；并把若干调用点误写成不存在的函数名。先按 P3 重复后再决定通用修复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

- `EVAL-B19-HIST1` 的权限收敛修复有效；Git 用例的 supporting aggregate
  不再铸成系统 principal member table。
- Trace 用例的 `frame_causality=unproven`、`enumeration_status=incomplete`
  披露正确，且完整保留指定窗口的因果投影能力。
- 新 P1 `EVAL-B19-SET1` 是结构化精确集合同 gap：一个显式合法整数不能在
  与 `members[]` 不一致时被归一化层静默覆盖。应基于 kind/value/members
  fail-loud，不读原始用户输入或模型答案原文。
- Git 的调用点错误暂列 `EVAL-B19-GREP1` 同族 P3 观察：runner oracle 太宽，
  但单次模型探索/清册波动不足以支持新增硬门。
