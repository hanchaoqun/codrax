# r982 人工审计：工具错配降至一处，发现运行时工作与宿主调度量串账

- 基线：`main@fc82a04dd`
- 批次：`evalcampaign_capabilitypriority_r982_20260831`
- 并发：严格恰好 2
- 自动结果：2/2 PASS
- 人工结果：Trace FAIL；Cangjie 答案 PASS、过程 FAIL

| # | case | runner | sec | 人工 | 审计结论 |
|---:|---|---:|---:|---:|---|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | 240 | FAIL | 显式窗、6 次 typed query、链上排序、实际占时/规则可消双账户、业务线索、自动补齐与最终 `Trace 因果投影` 完整；B1497 reader receipt 仍正确显示 VerifyClass 0.285ms、宿主随后直接唤醒目标及三条未证边界。但模型正文再次泄漏 `relation_unproven`，并把宿主线程的 runnable 0.105ms 有效归因错误写成 VerifyClass 工作“计入”的有效归因；typed semantic row 实际是 0.000ms。确认 B1499，并新增 B1500：工作墙钟/工作有效归因/宿主调度贡献必须在上下文中明确分尺，系统不得替模型改答案。 |
| 2 | cangjie_repomap | PASS | 259 | PASS（答案）/FAIL（过程） | 最终 12 行、2/2/8 计数、符号/路径/package 全正确。不可用调用由 r980 的 6 次、r981 的 3 次降到 1 次；残余发生在后续 explorer dispatch：当前 schema 仅 completion，但 `BuildInitialInstruction` 先命中通用 self-loop retry，重新要求 repo_map。system 尾指令第二轮纠正成功，却仍浪费一轮并铸出错误 7-member closure。B1498 尚不能关闭。 |

## 结论与后续

1. `B1498c` 应统一初始提示分支顺序：调度器声明的 typed surface 必须先于通用 self-loop retry；覆盖 source-inventory landing/required-file verification、completion-only 和 read policy，不能再逐 case 补词。
2. `B1499-TRACEINTERNALTOKENLOCALIZATION1/P2` 与 `B1500-RUNTIMEWORKHOSTMETRICSEPARATION1/P1` 进入后续批。二者只允许改善模型上下文/软教学和 typed 尺度表达；禁止扫描、删除、替换或拒绝模型正文。
3. 本轮没有旧稿恢复或固定 4ms/4m、活动流年龄、轮次、首字节、上下文比例触发的答案降级。
