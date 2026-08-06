# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T22:14:16Z
- sweep_start_ts: 20260806-151414
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | logtri_rust | PASS | eval/results/logtri_rust-20260806-151416 | log_attachment,answer_contains | log_triage | 80s | 19 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 正确识别 Rust `Option::unwrap()` on `None` panic、`src/config.rs:42:28` 与日志调用栈，并明确当前仓栈帧未解析，未伪造源码根因；零成文 reject。 |
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260806-151416 | answer_regex | none | 214s | 21 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=4,unavail=0,prune=0 | fail | 文字链和 walker 的文件发现职责基本正确，但 `run -> walker::collect_files` 的第 20 行 typed 调用证据被 grounder 错误恢复到第 10 行，导致 diagram 连续 4 次硬拒后被删除；最终 `is_match` 又落在较弱的函数定义 citation。runner 的关键词 PASS 不能替代调用链完整性人工验收。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `logtri_rust` 人工通过，外部日志边界和有用信息提取正常。
- `sr_rust_cross_module_chain` 人工失败。根因不是模型凭空漏画：Explorer 已读取并精确发出 `src/main.rs:20` 的
  `run -> walker::collect_files`；公共 grounding 的 leaf 解析只支持 `.`，不支持 Rust/C++/Cangjie `::` 与 C/C++ `->`，于是精确证据先失配，
  再由 `nearest_call` 用 `subject=run` 错迁到第 10 行。下游 typed call-edge 池因此永久缺一边，模型无论怎样补 `edge_anchors` 都不能通过。
- 登记 `EVAL-B200-QUALCALL1=P1/confirmed`。修复应位于 typed symbol/call grounding 公共层，统一处理跨语言限定符；不增加用户/答案
  原文扫描，不放宽“调用箭头必须有同向 typed call-edge”硬门，也不由系统代画或删除模型内容。
