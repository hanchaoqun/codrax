# r981 人工审计：运行时凭证闭环，工具能力教学仍有优先级冲突

- 基线：`main@4ad43dbde`
- 批次：`evalcampaign_capabilityalign_r981_20260831`
- 并发：严格恰好 2
- 自动结果：2/2 PASS
- 人工结果：Trace PASS；Cangjie 最终答案 PASS、过程 FAIL

| # | case | runner | sec | 人工 | 审计结论 |
|---:|---|---:|---:|---:|---|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | 188 | PASS | B1497 生产闭环：模型仍自主选择 `VerifyClass ...` 行及较保守关系结论；读者面完整保留 0.285ms、宿主线程随后直接唤醒目标的精确凭证，以及工作完成/目标等待/丢帧因果未证边界。显式 10ms 窗、7 次 typed trace query、链上根因排序、实际占时/规则可消双账户、业务线索、自动补齐与最终 `Trace 因果投影` 均保留，背景未进入根因排序。无旧稿降级或固定时间阈值降级。残余：模型正文仍复述 `chain_relevance=on_chain`、`confidence=0.91` 等内部 token，需后续只用软教学引导本地化，不能扫描或改写模型答案。 |
| 2 | cangjie_repomap | PASS | 245 | PASS（答案）/FAIL（过程） | 最终 12 行严格正确：extend=2、foreign func=2、public class=8，符号、路径、package 全一致。第一个 source-inventory explorer 的 completion-only 轮正确直接落地；后续独立 explorer 仍调用 3 次不可用 grep，并有 2 次 completion 拒绝。B1498 因此仅部分生产命中：user-role 工具面尾指令无法稳定压过 system-role 通用搜索教学。根修应把精确工具能力声明提升为同级、末位 system 指令，不增加正文硬门。 |

## 结论与后续

1. `B1497-RUNTIMEWORKCREDENTIALDISPLAY1` 可关闭；其读者事实展示与模型结论权属均获生产正证。
2. `B1498-EXPLORERTOOLCAPABILITYALIGN1` 保持开放并进入第二小批：当前 schema 是精确信号，允许在 system 优先级声明本轮工具能力；仍不得读取用户/模型正文或按语言/case 设例外。
3. 新记录 `B1499-TRACEINTERNALTOKENLOCALIZATION1/P2`：模型可见上下文中的 typed 字段名可被直接复制到答案。修向仅为跨 Trace typed rows 的软本地化教学；系统不得删除、替换或拒绝模型原文。
4. 本批没有发现固定 4ms/4m、活动流年龄、轮次、首字节或上下文比例触发的答案降级。
