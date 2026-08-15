# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T20:31:46Z
- sweep_start_ts: 20260815-133145
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260815-133146 | write_plan,write_patch_oracle | none | 85s | 24 | read=1,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 计划仅修改 `main.cpp:19`，统一 diff 与 structured replace 都是 `retrun -> return`，没有第二文件、无关 probe、计划拒绝或重试。plan-only 没有实际编译，故“编译通过/行为正确”是待 apply/verify 的验收合同，不冒充已验证结果。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260815-133146 | answer_regex,answer_contains | none | 255s | 30 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | `no_directed_path` 判定正确：`buildAnalysisIR -> gate.RunWith <- gate.Run` 是共享被调方边界，不存在 `buildAnalysisIR -> gate.Run` 有向路径。首稿 Mermaid 还画出 7 条真实、已有 citation 的 `buildAnalysisIR` 本地调用边，却因漏 `edge_anchors` 被正确拒绝；generic repair 将“补锚”和“删边”并列，模型选择删除全部 7 条，终稿语法合法但视觉关系缩水，正文列表仍保留这些函数。确认 B853：精确有证据的缺锚图在同轮伴随列表/端点错误时，metadata-only 保图车道被旁路。Mermaid 兼容层正确补 participant 引号，未降级为 text。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- 写模式 C++ 计划通过，验证所有权未越界；本轮不因 plan-only 把未执行的编译测试签成已通过。
- 图案 runner PASS 不能掩盖关系缩水。校验器拒绝缺锚是正确的，gap 在 repair routing：同轮非图 companion violations 让“已证边仅缺 metadata”的无损车道失效。
- B853 采用通用 typed 修复：只要 diagram relation failure closed set 全部是 `missing_grounded_call_anchor` 且 producer 已给完整 anchor patch JSON，就先要求模型保持 Mermaid body/关系/文字字节不变并只补元数据；其他字段下一轮继续校验。混有任何 unsupported relation issue 仍不进入该车道。
- 两案均无旧稿恢复、空答案或 fixed-age active-stream 降级；时序图案持续 255s 后正常结束。
