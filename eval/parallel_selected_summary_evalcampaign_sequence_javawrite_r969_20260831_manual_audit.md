# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T11:36:53Z
- sweep_start_ts: 20260831-043652
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260831-043653 | write_plan,write_patch_oracle | none | 56s | 27 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确定位 Main.java:16，仅规划 retrun→return；状态 pending_approval，未改源码。首个未调用 Main 的 JavaCompiler 包装探针被 coupling gate 正确拒绝，第二稿覆盖 greet("Alice") 与空串分支后通过。语法级单行修复仍生成运行探针略重，记低优先级 soft-teaching 观察，不新增硬门。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260831-043653 | answer_regex,answer_contains | none | 357s | 33 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=6/4,fin_reject=3,unavail=0,prune=0 | partial | 最终事实与图方向正确：buildAnalysisIR→gate.RunWith←gate.Run，明确不存在 buildAnalysisIR→gate.Run 有向路径；Mermaid 可渲染。初稿已在两个块分别正确展示并引用两条边，但主路径 facet 门要求第二条边进入 principal_path_edge，而同代 relation lease 禁止向 principal-ordered-list 新增该边；模型最终重复展示 gate.Run→gate.RunWith 才通过，形成 B1486 合同/修补面 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### B1486 — relation 已存在但 facet 归属缺失时，修补面诱导重复关系

- 初稿已经有两个精确、同向、可引用的关系载体：`principal-ordered-list` 承载
  `buildAnalysisIR -> gate.RunWith`，`gate-run-boundary` 承载
  `gate.Run -> gate.RunWith`；时序图同时包含两条边。
- endpoint-boundary ownership 门只认声明 `principal_path_edge` 的块，所以诚实报告第二条边缺少归属；同轮图标签修补又安装了
  relation lease。提示没有提供“只给现有精确关系块补 facet”的无损操作，模型把第二条边复制进 principal block，随后被 lease 以
  `unlisted_relation_added` 拒绝。
- 下一轮模型恢复正确分块仍被 facet 门拒绝，最终再次复制第二条边才接受。因此最终结论正确，但产生可见重复关系和 3 次
  finalizer reject；这不是关系探索不足，也不是单纯模型波动。
- 泛化修向：仅当现有块的每个 typed item evidence id 与每个 edge anchor 都属于同一 endpoint-boundary 集合时，动态发布
  `block_field_edits_v1 {field:add_facet_id,value:principal_path_edge}`，由模型选择精确 block id。执行器只追加隐藏 membership，
  不新增/删除/复制/改向/改写关系、标签、正文、图或结论；混合旁支块和歧义块不发布该能力。
