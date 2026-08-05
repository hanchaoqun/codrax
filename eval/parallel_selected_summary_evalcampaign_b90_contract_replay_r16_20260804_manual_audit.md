# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T01:30:58Z
- sweep_start_ts: 20260804-183056
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_multi_member_set_count_caveat | FAIL | eval/results/qf_multi_member_set_count_caveat-20260804-183058 | answer_regex,answer_contains | none | 329s | 40 | read=3,repo_map=10,list=0,trace=0,source_lens=10 | midloop=10,inv=5/0,fin_reject=0,unavail=0,prune=1 | fail | 用户目录没有固化进 typed requested-path carrier；completion 四次要求 `cmd`、fixtures、`internal/skill` 等无关范围。最终虽正确列出 3 个 type 和 30 个 Kind constant，却只列 4 个 production 函数，漏掉 checkout 中 `SetExternalArtifactFloor`，动态真值为 5。生产回放否定 §93.2 的闭环声称。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260804-183058 | answer_regex,answer_contains | none | 497s | 27 | read=27,repo_map=0,list=0,trace=0,source_lens=0 | midloop=22,inv=12/4,fin_reject=2,unavail=0,prune=0 | fail | 新 resolver 已在第三稿接受 `gate.Run -> gate.RunWith` 限定边，证明限定身份合同修复有效；前两次拒绝分别针对模型画出的反向边和漏掉的 principal edge，均为正确拒绝。但最终 prose 仍把 `buildAnalysisIR -> gate.RunWith` 与反向 wrapper 边拼成“间接到达 gate.Run”，与 typed `no_directed_path` 及最终图矛盾。runner 的 token oracle 未覆盖方向真值。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B89-CALLEDGEQUAL1` 的限定 owner identity join 已获生产正证，不再出现“正确限定边自身不可满足”的六连拒；本轮 2 次拒绝来自模型先后提交的真实错误图，不应降杆。
- 新 P0 `EVAL-B90-CALLBOUNDCTX1`：系统已铸出 `principal_span_waiver=no_directed_path`，却继续把模型自报的“完整调用链” member_set 和错误 summary 作为 principal/finalizer 上下文。模型在同页正确图之外仍拼出不存在的 source→sink 路径。修复必须清理 typed context authority，不能扫描或改写模型最终 prose。
- `EVAL-B88-SCOPEPROV1-R2`：真实 analyzer 本轮没有把目录写进 `MentionedEntities` 或 `SourceScopeProfile.SourceQuotes`；唯一稳定精确信号是 analyzer-stage source_inventory tool scope 与当前请求中的同一 canonical path literal。应使用 lexical path identity（非关键词/语义扫描）铸造 typed boundary，并保持 wrong-stage/root/unmatched fail-closed。
- 新 P1 `EVAL-B90-INVENTORYFRESH1`：目录边界缺失使 production roster 被全仓 census 淹没，模型从旧的 4 函数印象推断而漏掉新导出函数。先修边界并让 requested-universe rowset 直接提供完整 production roster；若 scoped lens 已 complete 仍遗漏，再单独修 row publication，不能对函数名做 case 拟合。
