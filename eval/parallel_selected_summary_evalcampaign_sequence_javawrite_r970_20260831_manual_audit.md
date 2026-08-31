# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T12:02:45Z
- sweep_start_ts: 20260831-050243
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260831-050245 | write_plan,write_patch_oracle | none | 63s | 27 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确定位 Main.java:16，仅规划 retrun→return，状态 pending_approval 且未改源码。首个 plan 的 old_text 漏前导缩进，被当前字节诊断一次纠正；最终单行 patch、javac 与 greet 签名验收均正确。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260831-050245 | answer_regex,answer_contains | none | 364s | 32 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=4/0,fin_reject=4,unavail=0,prune=0 | partial | 最终事实与图正确：buildAnalysisIR→gate.RunWith←gate.Run，并明确不存在 buildAnalysisIR→gate.Run。首稿已带 principal_path_edge，故 B1486 分支未命中但无重复边回归。仍有 4 次确定性拒绝：模型已选 relation evidence 的列表缺隐藏 edge anchors，却先被迫整块重发；关键函数清单只缺 member_set 隐藏归属时又被要求整块重发，最后形成 malformed JSON string。确认 B1487 元数据修补面 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### B1487 — 可见答案已由模型完成，但隐藏关系/清单归属仍要求整块重写

- sequence 首稿已经正确选择两条 `call_edge` evidence，并在列表 item 与 Mermaid 中表达
  `buildAnalysisIR -> gate.RunWith <- gate.Run`；列表仅漏 `edge_anchors`。第一次修补要求模型复制完整 relation recipe，模型先漏
  endpoint identity，下一次又因 live lease 禁止 whole replace 被拒，第三次才通过 additions/attach ref。
- 关系修复接受后，关键中间函数列表已经完整可见，但 requested-dimension 检查只因缺隐藏 `member_set` facet 再触发一次整块替换。
  模型将数组错编码为含 `undefined` 的 JSON 字符串，补丁失败；系统保留此前已接受且用户可见内容完整的答案，所以没有丢答案或固定时长降级，
  但额外重试和 63k 上下文消耗可避免。
- 泛化修向：zero-anchor 列表只有在同一个 `evidence_id` 同时出现在模型 `claim_uses` 与可见 item 时，才发布 additions-only
  relation ref；模型仍选择 ref、local node 与 visible label，系统只追加隐藏 anchor。唯一 typed `enumeration_item` 清单缺
  `member_set` 时发布 `add_facet_id=member_set`；双清单、source-inventory row、混合 relation carrier 均不开放。
