# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T05:57:49Z
- sweep_start_ts: 20260731-225748
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_git_two_diffs_current_code | PASS | eval/results/read_combo_git_two_diffs_current_code-20260731-225749 | answer_regex | none | 128s | 27 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 两次提交、diff 线索、当前关键代码、作用和影响均有当前源码锚点；B19-HIST1 补表未复发，上轮调用点/文件数误述也未复发。 |
| 1 | trace_query_donghu_real_frame_multicausal | FAIL | eval/results/trace_query_donghu_real_frame_multicausal-20260731-225749 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 172s | 43 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 根因排序、唤醒链、可消除量、投影、coverage 与补采均保留，19/7 矛盾未复发；但调查闭环已有 3 个代表窗，最终可见正文遗漏该维度，runner 正确失败。另有 frame_causality=unproven 与“该帧无法完成渲染/直接根因”确定措辞冲突，以及把不可相加席位写成“累计超过53ms”。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

- `EVAL-B19-SET1` 的精确集修复未影响显式窗 Trace 主合同；本轮模型没有再发
  count/roster 矛盾，稳定交接中也没有伪造的精确 D/IO 总数。
- `EVAL-B19-GREP1` 未复发，按模型波动保留 P3，不施工。
- 新 P1 `EVAL-B19-TWIN1`：typed ranked seats 已携带合法 occurrence windows，
  closure reason 也已列出 3 个窗口，但 finalizer 只发一个 summary，系统投影
  没有紧凑代表窗面，导致用户要求维度丢失。
- 新 P1 `EVAL-B19-FRAME1`：coverage block 正确声明
  `frame_causality=unproven`，但位置在长报告尾部，模型首段仍发布确定 frame
  结果/根因；不可扩大现有答案关键词 rewrite，应做 typed authority-first
  收敛。
- 新 P2 `EVAL-B19-ARITH2`：模型把重叠根因席写成“累计超过53ms”；系统虽加
  不可相加 caveat，却没有消除首段矛盾。与 FRAME1 一并研究结构化 principal
  claim authority，不对模型句子加数字/短语硬门。
