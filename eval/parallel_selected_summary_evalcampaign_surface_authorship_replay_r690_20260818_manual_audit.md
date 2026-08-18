# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T14:44:02Z
- sweep_start_ts: 20260818-074400
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260818-074402 | answer_regex,answer_contains | none | 373s | 41 | read=20,repo_map=4,list=0,trace=0,source_lens=0 | midloop=15,inv=6/0,fin_reject=0,unavail=0,prune=1 | fail | B1082 正向：终稿零 `principal-support-surface-terms`，没有系统补表或跨 patch 重复块，模型仍自行输出完整段落/列表。人工仍不合格：配置文件被错写成 `.codrax/codrax.yaml`，优先级表把“运行时 Mermaid 开关”当成配置覆盖层并遗漏 CLI 精确 override，`KnownFields(true)` 的失败语义也写成 stderr warning；Mermaid 机制虽比 r689 更具体，但若干 outcome 解释仍混淆。末尾系统追加“系统降级披露：必答面硬转软 ×1”是内部合同词面，不是客户语言，记 B1083。Runner PASS 只证明弱 oracle 命中。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260818-074402 | answer_regex,answer_contains | none | 729s | 48 | read=32,repo_map=2,list=0,trace=0,source_lens=0 | midloop=19,inv=7/0,fin_reject=3,unavail=1,prune=4 | fail | B1082 正向：四轮 full/patch 均未追加 surface-term 系统表。四阶段表基本正确且保留输入/输出/状态载体，但 Mermaid 从首稿的 7 个业务参与者膨胀为 22 个节点，混入 `dispatchExploreWindowsParallelWithHintKind`、`recordFinalizeRepairDraft` 等无关内部函数，并增加四个孤立“未证关系边界”；读者难以看出一次请求的主时序。首稿未给可见箭头 typed anchors 的拒绝正确；深因是 Analyzer 把独立 table 的示例载体误铸成 diagram incident participants，后续 repair 又把整份 typed recipe 当绘图清单。记 B1084，不以放松关系证据门或系统代画解决。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Deterministic findings

1. `B1082-SURFACETERMSYSTEMAUTHORSHIP1` 获生产闭环。两案最终答案、full emit、三轮 patch 和 mutation
   日志均无 `principal-support-surface-terms`、其递增 ID 或“系统按已验证证据补充可见标签”；模型正文、
   table 和 diagram 仍能正常落盘。typed `surface_terms` 继续留在 prompt/advisory，系统没有接管结论。
2. 新确认 `B1083-DEGRADATIONFOOTERINTERNALLEXICON1=P1/easy-high-ROI`。最后一跳 footer 把 typed
   `facet_softened` 翻成“系统降级披露：必答面硬转软”，虽未泄漏 snake_case，却仍把内部合同分类直接
   暴露给客户。最优方案保留 typed 计数和披露，只改成客户可理解的证据边界句，例如“有 1 项原定内容
   因证据不足仅作为建议；具体原因见运行日志”；不删除披露、不修改模型答案、不扫描答案词面。
3. 新确认 `B1084-DIAGRAMSIBLINGSURFACEPARTICIPANT1=P1/design-required`。Analyzer 教学已明确“独立 table
   surface 的状态载体不是 diagram participant”，模型仍把 `AnalysisIR/EvidenceItems/AnswerDocument/BusContext`
   发成 `incident_required`，schema 因 identity/source_quote 字面合法而接受。participant gate 随后诚实要求
   四个 unproven boundary；typed recipe 修补又诱导模型复制十余条局部 call，造成图层表达过载。
4. B1084 不能通过放松 edge authority、按 case 删函数名、扫描原始请求/终稿关键词或系统重画来修。
   设计需让一个请求中的 diagram/table/list 各自拥有 typed presentation-surface ID，participant 必须绑定
   自己的 surface；sibling table dimension 不能进入 diagram incidence。repair context 只提供满足当前
   diagram surface 的最小候选集，完整 relation roster 继续留在证据池而非 copy-ready 绘图清单。
5. Combo 的精确六级 lookup 在 r689 曾正确、r690 又错，且本轮 Explorer/Finalizer 已看到相关实现；当前
   先判为模型成文波动与 dimension closure 仍弱，不为单案增加硬化答案。若跨异构回放持续，应补 unit-level
   mechanism closure 的 typed evidence selection，而非系统改文。
6. 两案均无固定 4ms 活跃流降级、畸形 JSON recovery、旧稿替换或空答案。本批没有 Trace 输入；显式窗、
   因果投影、自动补采、链上-only 主因、实际占用/业务线索与规则可消除量双轴均保持。
