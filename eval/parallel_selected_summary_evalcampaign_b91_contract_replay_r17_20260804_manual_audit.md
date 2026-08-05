# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T02:00:24Z
- sweep_start_ts: 20260804-190023
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260804-190025 | answer_regex,answer_contains | none | 563s | 28 | read=19,repo_map=2,list=0,trace=0,source_lens=0 | midloop=12,inv=5/0,fin_reject=6,unavail=0,prune=0 | fail | Degraded answer仍把两条汇入 `RunWith` 的并列边叙述为 `buildAnalysisIR` 到 `gate.Run` 的链。前几轮对 `RunWith -> Run`、伪 check 自环及无配对 reverse arrow 的拒绝均正确；但第 2/3/6 稿的真实 `gate.Run -> gate.RunWith` 也被拒。生产 evidence 已明确为 `owner=gate.Run anchor=RunWith role=production @ gate.go:135`，故这是 resolver/required-anchor 接线失配，不是证据缺席。no-path 成功摘要已改成 typed boundary，但仍把模型自由 `PrincipalSpanWaiver.Rationale` 追加到同一工具结果，形成错误解释侧漏。 |
| 2 | qf_multi_member_set_count_caveat | TIMEOUT | eval/results/qf_multi_member_set_count_caveat-20260804-190025 | answer_regex,answer_contains | none | 1201s | 35 | read=2,repo_map=16,list=0,trace=0,source_lens=16 | midloop=11,inv=7/1,fin_reject=2,unavail=0,prune=0 | fail | 目标 scope 在 selected-sub-repo 归一化后只以 `Scopes=[.]` 进入 observation，请求路径 carrier 再次无法铸造；16 次 source_inventory lens 被全仓 `candidate_budget_truncated` 债务牵引到无关目录。最终 typed roster 将 51 个 `_test.go` 测试入口与 5 个 production 函数合并为 function=56，成文门强制模型逐项发布，违背本 case 的公开生产 API 口径；第 2 次 patch 校验约耗 354s，最终超时。动态 production truth 为 type=3/function=5/Kind constant=30。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

1. `EVAL-B91-SCOPECOORD1`（P0/high ROI）：`SourceInventoryObservation.Scopes` 同时承担执行时 sub-repo 相对坐标与 repo-root
   请求边界，归一化把 `internal/analysis/criterion` 压成 `.` 后不可逆丢失身份。应增加 engine-derived repo-root query coordinate；
   producer 只把该 typed coordinate 与当前请求的 exact canonical path token 做合取，不能继续依赖模型 entity/quote。
2. `EVAL-B91-SOURCECLASS1`（P0）：inventory principal rowset 缺少 production/test 来源类别权限，导致有界 API 清单被测试入口污染，
   并由强制逐项发布合同放大成 6 分钟校验。应由 parser/index 给每行携带 source class，AnswerContract 只消费 analyzer 已声明的
   typed source-class universe；不得扫描用户或答案 prose 决定删测试。
3. `EVAL-B91-CALLEDGEWIRE1`（P0/red-line）：证据 owner 已是 exact `gate.Run`，正确限定边仍被拒，需核对 diagram required-edge
   endpoint 与 evidence resolver 是否使用同一 canonical symbol identity；错误反向边、自环继续 fail-closed。
4. `EVAL-B91-WAIVERRATIONALE1`（P1）：no-path 的自由 rationale 仅留审计态，不能与 typed endpoint boundary 一起进入工具结果/Finalizer。

上述结论均来自 typed observation/evidence/document 结构；没有把用户输入关键词或模型/最终答案原文作为 hard gate。Trace
`QFRootCauseTrace`、显式时间窗、因果投影和自动补齐均不在这些修改面内。
