# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T00:41:38Z
- sweep_start_ts: 20260816-174137
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260816-174138 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 203s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | partial | 模型正文正确区分“策略上限存在”“目标绑定未确立”和“58.320ms 相对理想基准供给空间”；显式窗四态、完整 Trace 因果投影、链上-only 主因、业务占时与规则可消双轴均在。确定性“下一步”却把 frequency_thermal 写成“解除运行频点限制”，越过同页 typed authority，见 T599-2。 |
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-174138 | answer_regex | none | 239s | 32 | read=7,repo_map=2,list=2,trace=0,source_lens=1 | midloop=6,inv=1/0,fin_reject=3,unavail=0,prune=0 | fail | 三次可选图关系拒绝后，patch 删除图并保留 `hop1`；此前 patch 已把该 principal `principal_path_edge` 块的 `claim_uses`/`edge_anchors` 清空。pre-emit 明确发现但仅作 soft advisory，终稿仍把六项描述成完整链，并把 `_HAVE_NATIVE` 的导入文件错写成 `__init__.py`。见 T599-1/T599-3。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### T599-1（P0）：`principal_path_edge` 可在 patch 后失去同块关系所有权

1. 首稿的 `hop1` 同时带 directed `claim_uses` 与多条 `edge_anchors`；它们中的错误边被共享关系核正确拒绝。第二次 patch 替换 `hop1` 时保留
   `surface_role=principal`、`facet_ids=[current_code_path,principal_path_edge]` 和可见 hop，却省略全部 `claim_uses` 与
   `edge_anchors`。后续只删除可选图，没有再次替换 `hop1`。
2. 第四、第五轮 patch 的 pre-emit 日志都准确报出 `blocks[id="hop1"].claim_uses` 缺失，但该通用检查是 soft advisory；文档级
   required block/facet 又能被同类 sibling carrier 共同满足，post-contract 仅留下 3 条 soft violation，最终仍签绿。
3. 这是 typed 空集逃逸，不是答案关键词问题：一个块既声明“我是主路径边载体”，又没有任何机器可核验的定向 claim/endpoint owner。
   最优修复是把 `principal_path_edge` 与同块 directed claim + exact anchors 做 schema 合取；不从旧稿自动继承关系，不由系统造边。
4. 已施工：`QFCallChain + principal + ordered_list|bullet_list|table + principal_path_edge` 若没有 call/callback/register claim，或 claim
   没有 anchors，则走既有 typed relation hard lane 同轮拒绝。definition/support/Generic/Trace 全旁路；完整模型关系继续下放既有同向证据核。

### T599-2（P1）：频率修向的系统词面超过 typed 证据权限

1. 模型正文和 trace 权威块都正确写明：CPU0/4 的 policy ceiling 只证明策略上限存在；目标时间片未与上限绑定，也没有证明触顶或热节流。
   `58.320ms` 是相对既定理想频率基准的 supply-fold headroom。
2. 系统生成的“下一步”却写成“评估解除 ... 运行频点限制 / lifting ... running-frequency limits”，把“可提升算力供给”升级成“已有限制可解除”，
   与同页证据边界冲突。这不是模型结论，属于确定性补充的措辞过权。
3. 最优修复：保留 frequency_thermal 方向、subject、58.320ms 与升频/迁核动作，但改成“按既定理想基准折算的可提升空间”；禁用
   “解除限制 / lifting limits”。不改排序、数值、席位、因果投影或模型正文。

### T599-3（P1，待后批）：追加 citation 的文件/行/quote 未形成同源闭包

1. Explorer 的正确证据是 `bindings-py/fastlex/tokenizer.py:2` 导入 `_fastlex`；`bindings-py/fastlex/__init__.py` 只 re-export
   `FastTokenizer`。最终 patch 却追加 `{file=__init__.py,line=2,quote="import _fastlex"}`，并在可见 section 宣称导入发生在
   `__init__.py`。
2. 该 quote 与真实文件行不符，系统虽修了一处 item `citation_ref`，错误 citation 仍留在 citation pool 和可见 prose，最后只发通用
   “前后不一致” advisory。说明 patch append-citation 的 location/quote、item 绑定与可见声明没有在同一 exact source row 上闭合。
3. 后批应从 citation 载体完整性根修：当前源码 citation 的 file+line+quote 必须匹配同一 checkout 行；不匹配时拒绝该 citation 或要求模型改正，
   不能依据模型自报 quote 伪造源码权威，也不能扫描自由 prose 决定真假。

## 非回归与上下文审计

- Trace 的三次 `trace_query` 给出了完整四态、目标 CPU/频率桶、策略 ceiling 和 authority caveat，足以支撑模型的谨慎结论；问题只在系统 next-step
  模板。邻近/背景没有被选为主因，优先级反转、调度供给、算力供给、D/IO、确定性 JIT 与业务 span 线索均保留。
- Poly 的探索上下文本身含正确 import 行与核心 wrapper→core call evidence；错误不是“没给模型信息”，而是关系 patch 元数据丢失和 citation
  追加闭包不足。三个图拒绝均由精确 typed mismatch 触发，删除可选图本身合法；不应为保图放松关系权威。
- 两案均持续收到模型字节并正常完成；未出现 4ms、4s 或固定总年龄降级，也没有系统替换模型答案。
