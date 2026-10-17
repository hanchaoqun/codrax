# HMC257 人工审计汇总

本审计区分有限交付、自然分支、整答与确定性回归。两例各一次，未追加重跑；原始过程及完整答案不修改。详细独立 A/B 审计、上下文专项和物理/当前集合收据保存于本地 `results/hmc_material_scope_20261016/verification-receipts-v2/`。正式运行/问题/oracle 见[批摘要](parallel_selected_summary_hmc_material_scope_20261016.md)。

## 裁定

| 合同 | A | B |
|---|---|---|
| 自然恢复：4 原计划本轮真实执行 | PASS | PASS |
| 原名包与最终真实尝试集合一致 | PASS | PASS |
| 完整原载荷、当前计数、退出后链接 | PASS | PASS |
| 显式报告安全回写 | 不适用 | PASS；与默认 MD 同字节 |
| 11.3 有限交付 | PASS | PASS |
| 页预览不污染独立集合完整性 | PASS，实际命中 534/564 | PASS，实际命中 534/564 |
| 排除源码与 required 位置矛盾修复 | 未命中；错误源码义务仍在 | 局部 PASS；下一次转为 observed_value |
| 原生正文/表格/关系同源复用 | FAIL | FAIL；正确附表不能代签正文 |
| 图修复／真实查询截断／泛化因果重试 | 未命中 | 未命中 |
| 完整答案 | **FAIL** | **FAIL** |

A/B 实际查询均为两个原件×线程101/202、resource_stack、[10,10.05)秒；3/1/1/0 事件、14 关联帧与独立 SQL 一致。只读结果 JSON 不增加 attempt，历史一次成功不计本轮成功；有范围 empty 仍是已成功执行的零匹配查询，不是未执行。

## 失败原证与责任

1. **未知单位被补成字节（A/B）**：原载体 heap_size 未声明单位，原生上下文/附表正确保留未知；调查 interpretation 和正文改成 B/字节。数值正确不授予单位。
2. **跨来源借身份（A）**：nested/202 原生为空且 observed name 未知，正文/list/table 借 main/202 的 background；不影响正确的 empty 执行状态，但身份事实不通过。
3. **深度被解释为调用顺序（A/B）**：后一个 Alloc 的 depth=[0,0,1]，同深度 malloc、buffer_cleanup 被串边；B 将 Free 的 [0,2] 错说成 depth0–1 缺失，实际仅1缺失。外部观察标签替换 call_edge 元数据不能消除正文关系主张。
4. **源帧完整被当成真实栈展开完整（A/B）**：source_frames_complete=true、omitted_frames=0 只说明返回源记录完整；unwind_completeness=unknown 仍未知。正确完整性附注不能抵销主文“调用链完整”。
5. **导航材料误升源码证据（A/B）**：A 引 checkpoint 的 artifact ID 行支持资源时间/值/栈；B 系统追加 checkpoint pid 字段为“关键源码锚点”。这不是客户源码事实，应贯穿证据角色/引用补充/显示修复，不能只靠用户重复“不要分析源码”。
6. **语义恢复扩大范围（B）**：一次精确位置冲突修复后，七轮 comparison 拒绝最终使 explicit-window 被改 full_artifact；系统误教全工件账户不可用。真实查询靠恢复计划保住原窗，不能据此签范围建模正确。

没有资源时长冒称线程等待、泄漏或响应根因；最终时间已纠正，不把过程错误算成终稿错误。两例没有图块，但正文箭头仍须关系证据，不能因无 Mermaid 就免审。

## 系统上下文核对

两例成文实际可见四个独立原生查询的 summary/members 共8选择、5事件/14关联帧、零成员省略，原始纳秒、每源 owner、未知单位、缺深度/重复深度均在。534/564 只是某个 payload 的逐字阅读页，不是采集/原生事件全集分母；新作用域教学自然生效。A 自由手抄、B 自动追加原生 summary 表均未统一正文；调查摘要仍重述错误单位和顺序，是下一共享事实交接缺口。未证明为随机模型波动。

v1 全仓另外暴露纯 Trace 预算压缩漏事实，不能用这两例资源上下文完整来抵销。预算恢复、源码纯场景保护和门禁登记用原测试复验；最终代码与原 live 二进制必须分版本记账。

## 原件与终稿索引

| 项目 | A SHA256 | B SHA256 |
|---|---|---|
| 详细日志 | `1c643df96e32ddbfe0f4cd936fd67b36cc5211a637f582b2d7c1464e7fe5d8f4` | `a6f4ddd4e89da4aa6af490ecf2211e1aadd388cc13e86fbdf668e49de2242afa` |
| 完整 Markdown | `ea3fad7fd180f9cb50ebffd05f5984ef7f4274da518b6df2af05930c46d9921e` | `0ec739d662fc5662f71bc89493100f124eacf404e056766c2a09cd0a9717a935` |
| 包 manifest | `16fe98a95a2577fae9d19f380642637e4c98d6d8c95f8c433d45705c46844d8c` | `4dde281b316cb91e1cca2549655435f04fa572238519405cf086807aa77740a1` |

A 默认终稿 `evidence-export/workspace/.codrax/output/20261016-185733.527-31828.md`；B 为 `report-writeback/workspace/.codrax/output/20261016-185840.496-31829.md`，与其 `final-report.md` 字节一致。完整两个 surface、实际 emit/patch、原生载荷、当前 index、独立 oracle 和命名包 report 均纳入独审，不只看截图或最终一段。

## 后续按 ROI

优先材料/导航/源码证据角色贯通和同源事实组件复用（16.4/16.5），保独立解释与链上业务线索；范围重试不能无依据保留伪 comparison、也不能扩大时间域。旧采样/只读登记/图验收继续轮换，不为同一问题追加第三例。11.3 按原有限导出/回写合同结算，不把所有正文质量并入它；完整答案仍为0/2，不以有限成功抹掉失败。
