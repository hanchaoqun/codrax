# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T10:44:24Z
- sweep_start_ts: 20260813-034423
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-034425 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 198s | 41 | read=2,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B718 改动未进入 Trace lane。五次查询均为显式用户窗；链上 running 65.912ms、D-state 36.757ms、反转/调度/供给/D/IO、业务 span 与双轴均保留，邻近单列不加冕，正文不跨席求和。完整 wakeup_chain 分支由系统补齐。 |
| 2 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260813-034425 | answer_regex,answer_contains,mermaid_edge_count | none | 617s | 40 | read=23,repo_map=2,list=1,trace=0,source_lens=0 | midloop=9,inv=6/0,fin_reject=6,unavail=0,prune=1 | fail | B718 第一层生效：prompt 已发布 requested_relation_spine_status=unproven 与 BusContext.Mutable no-arrow ownership recipe，模型也尝试 subgraph。第二层仍冲突：participant validator 发现 BusContext/Mutable 的任意局部 call 后仍判 typedEdgeAvailable，禁止 requested-relation boundary，强迫局部 call 冒充完整数据流。6 次拒绝耗尽后降级，恢复稿 Mermaid 被机械修复成 `] -->|` 损坏形。确认 B719。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

1. Trace 人工 PASS，B716 继续稳定，B717 未复现；B718/B719 施工必须继续对 Trace fail-closed。
2. `B718-REQUESTEDFLOWTOPOLOGYCOMPLETENESS1` 的 prompt/authority 层生产生效，但 validator 层未闭环。系统同时教“局部事实可与请求关系未证共存”和“只要存在任意局部 typed call 就必须删 boundary”，构成精确的矛盾合同。
3. 新立 `B719-LOCALINCIDENCEREQUESTSCOPECONFLATION1`（P0）：区分 request-scoped relation 与 local evidence relation。stage precedence 是本请求范围内真实子图；载体上的任意调用只是局部事实，不得消除缺失的跨参与者请求关系。
4. 泛化根修只使用 typed participant roster、checkout-verified stage precedence、结构化 Mermaid edges 与 typed anchors：若 stage provider 覆盖至少两个但未覆盖全部 incident participants，未覆盖载体可保留 local call + requested-relation unproven boundary；只有 typed cross-participant graph 真实连通全部请求参与者，boundary 才能消失。单参与者普通调用用例仍必须画已有 typed edge，证据杆不降低。
5. 用户点名的 `20260813-035440.459-11882.md` 正是本轮逻辑降级稿。其 Mermaid 含 `] -->|` 等非法行；日志显示先有 6 次合同拒绝，随后 `mermaid_source_repair_applied=7`，恢复稿却仍以 Mermaid fence 出厂。确认独立 `B720-DEGRADEDINVALIDMERMAIDSHIP1`（P0/L7）：降级恢复稿绕过正常最终结构校验后，机械修复既未保证语法有效，失败时也没有改为带 `# ⚠` 原因的 text fence。
6. B720 根修放在所有降级恢复出厂车道的统一 last-mile：仅对 rejected/text-recovered 文档执行最终 Mermaid dry-run；明显畸形或 parser-confirmed reject 时保留正文和原始图源码，但把 fence 改成 `text` 并写明“恢复稿 Mermaid 未通过语法校验”。合法 Mermaid 保持 source，终端库不支持的合法家族不误判。正常已验证答案零改动。
7. r431 活跃流在 198s/617s 内持续工作，未因 4ms 降级。逻辑用例的降级原因是 6 次确定性结构校验耗尽，不是活跃链接超过 4ms；修复目标是消除矛盾合同并让真正耗尽时安全出厂，而不是缩短首字节/byte-stall 权威。
