# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T09:17:09Z
- sweep_start_ts: 20260821-021707
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-021709 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 221s | 37 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 系统面通过：显式 2.000..2.020s 窗、四节点三边唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时/规则可消双账户、背景隔离和完整 Trace 因果投影均在，且无固定 4ms/4m 降级。模型仍把 typed 调用点 fscache_page_wait_on_page_bit 扩写成“等待页面位图/内核级文件系统缓存 IO 完成”，而当前证据没有证明等待对象、具体后端或直接完成关系；保持 B1269/B1271 软教学项，不扫描、拒绝或改写正文。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260821-021709 | answer_regex,answer_contains | none | 914s | 66 | read=14,repo_map=2,list=0,trace=0,source_lens=0 | midloop=20,inv=6/0,fin_reject=20,unavail=0,prune=0 | fail | B1285 的 atomic target + unchanged 冲突已消失，首个局部 patch 正常执行；但同代关系修补未收敛。旧 generation failure_ref 被逐个以纯文本报 stale，当前完整 typed lease 未重发；alias normalizer 又把正文中显式 node id（analyze/explorer/...）按既有 participant label 改写成 An/Ex/... 的 anchor node，制造 body-only 与 anchor-only 双向失败。随后出现 carrier=unknown、无可执行 action、occurrence 超出当前正文等合同噪声，模型被逼回禁止的 whole replace。20 次拒绝后只恢复初稿，图保留大量无证调用边，结构化答案失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- `B1285` 仅获得部分生产正证：冗余 `unchanged_block_ids` 已由 atomic compiler 吸收，初始冲突不再出现；目标图 whole-block 能力也能在执行入口拒绝。但这不能视为局部关系修补闭环，因为模型最终仍在其他合同噪声推动下尝试 whole replace。
- `B1286-LIVERELATIONDELTAONATOMICERROR1/P0`：在 live relation lease 下，atomic edit 遇到 unknown/stale ref、非法 action 或失效 selector 时只返回首个纯文本错误，retry 看不到当前 generation 的完整 failures/allowed additions。模型只能逐项猜删，造成 O(n) 重试与陈旧 ref 串行泄漏。修复应从 live lease 重新发布 typed repair metadata，不解析错误文本、不替模型选择 action，也不静默丢操作。
- `B1287-EXACTNODEIDBEFORELABELALIAS1/P0`：`diagramNodeAliasIndex` 只收声明 id/label，没有把 Mermaid 正文边端点视为精确 node id；当模型新增 `analyze --> explorer`，而旧声明存在 `An as Analyzer`、`Ex as Explorer` 时，大小写折叠的 label alias 会覆盖新增 exact id，`normalizeDiagramEdgeAnchorMetadata` 只改 anchor 节点而不改 body，确定性制造 stale-anchor/missing-anchor 对。精确正文 node id 必须优先于显示 label alias；歧义 label 保守不解析。
- 本轮不是活跃流 4ms/4m 降级。read 的恢复旧稿只发生在 20 次结构化修补失败后；需要消除合同自造失败，不能通过缩短活动流、降低证据门或系统代写图/答案掩盖。
