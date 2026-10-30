# HMC315：两例独立人工审计摘要

与[批汇总](parallel_selected_summary_hmc_temporal_registration_20261029.md)同批、同冻结源。机器退出码、原评测判据、有限子合同与完整人工验收分开。审计不新增模型调用、查询或退出门，不以正确表格抵消正文错误。完整审计原件为 `/private/tmp/codrax-hmc315-A-audit-v1.md`、`B-audit-v1.md`、`integrity-v1.md` 与 `root-review-v1.md`（后三项同前缀），均按真实源路径收进本批sealed索引。

## 判定

| 对象 | 判定 | 依据和不能外推的范围 |
|---|---|---|
| 公开Extract/Finalize引用状态修复 | 有界实现PASS | 真实query→公开完成→实际两消费者；源变代/JSON/原子预算正反针及全仓通过，不是自然例通过 |
| A原Trace业务时序合同 | 机器FAIL，完整人工FAIL | 恢复稿没有所求时序图；文字仍有状态/依赖解释冲突 |
| A受信事实与恢复隔离 | 有限PASS | 原生时序表/Trace投影保留，未验图未泄露为已证图 |
| 新状态显示自然覆盖 | NOT_TRIGGERED | typed交接修正后曾受理，但后来legacy完成退役旧handoff；最终INIT无新状态 |
| B原缺来源终态负控 | 原测试PASS，限定人工PASS | proof verified、整体unverified，原来源未被倒签 |
| B自然controller/full CLI/所有框架 | 未覆盖 | 实际是真实planner＋scripted controller＋独立公共RunTests |

## A：输入、交接与最终各表面

原题与20行构造ftrace不变；这是真实模型/生产工具执行，不是新增设备实采。正式进程exit0不覆盖原 `FAIL degraded_answer_checks_skipped:1`。实际trace_query为16次且均ok=true，原生独立payload14份，两类计数分开；实际logged Finalizer INIT3份、最终MD与HTML源码均保留；未做浏览器像素QA。

最终MD：`.codrax/output/20261029-184110.225-57257.md`，44630字节，SHA256 `d53f3c1aaca642c728a2b642bfb7f0fcb469613e722978cd92074f311325f552`。HTML源码2886485字节，SHA256 `967a9f219cd117c4a0641146e9b78c019166fe879fc28a91c07648ee19527cf6`；源码提取表格与MD诊断一致，0 Mermaid，嵌入Markdown摘要一致不等于语义PASS。

- 完成 `1.040000` 与唤醒 `1.040010` 本轮分开；worker `[1.040010,1.041010)` 就绪1ms、随后原完整运行段至1.045500为4.490ms均在原生表。若止于唤醒main的1.045000则运行3.990ms，两个量尺不能互换。
- 主线程业务50ms的5运行/1就绪/44睡眠与查询52ms的7/1/44分开；worker业务40ms的8/1/31及原始全段统计不是相同分母。
- 31ms闭合睡眠与35ms请求驻留不相加；合法两段1ms小计2ms保留；47ms backup是背景。原4排序记录与去重后的3物理原因不混作缺项。
- 正文仍说“50ms里运行5ms，其余44ms全部睡眠”，漏掉1ms就绪；后文正确分区不能替前文销账。明确唤醒边可支持沿链排查，但不得把没有持有/阻塞凭证的关系说成已证直接机制；本轮相关自由解释继续留账，不追加文字扫描门。
- 无有效时序图交付，因此主体、两条唤醒边和上CPU点不能因正确原生表而判图合同PASS。恢复车道隔离未验图，保留受信事实是正确边界，不是完整问题已回答。

### 调查完成生命周期

两份nested `native_fact_handoff`确实提交；第一次引用不属于所选pair而拒绝，修正后的 `e78d6674` 参数受理。随后3个legacy完成受理；当前既有合同会在新完成h=nil时退役旧handoff，fork合入也只采用该fork当前完成决定。最终INIT没有 `native_reference_status`。没有逐fork私有状态快照，不冒称定位了唯一对象退役时刻。不能写“从未提交native_refs”“全部被拒”或把新显示分支算自然通过；原地址校验确已触发。

已受理交接如何在多生产者/多分支后仍精确进入适用消费者，是下一公开复现线索；现有退役规则有防旧权限复用意图，不能删规则或无条件恢复所有旧摘要。实际仍有正确原生时序事实进入最终上下文，不把所有文本错误归因于系统缺数，也不把未进入INIT的旧单元说成已重放。

### 图修补与预算

原完整草稿后有5次patch。前三次把同一个opaque participant_ref配到错误主体，未staged；实际日志有模型按枚举顺序猜主体映射的证据。离线按原JSON、既有结构顺序和Go HTML转义重算fingerprint/rp可复核：`rp1-774c…`指文档工作线程而非主线程。不能把这三次错误参数拒绝称为正确多行身份回归。

精确映射存在另一处typed delta，而当前schema分支仅枚举opaque ref；这是同源身份导航的泛化改进线索，不证明所有合法路线都被阻断。第四次修补增加未列入局部权限的边而回滚；第五次merged已staged但最终图合同仍失败，不能说5次全部未staged。HMC314显式native_participant重选自然覆盖仍不据此补签。

首次成文估计148151/200000 tokens；日志预算为592604/800000 bytes，末轮732076/800000（91.5%）触及保护后恢复。其值不是实际计费token，也不能单凭体积断定误述因果。下一步应把精确身份/关系修补交接与预算共同公开复现，不扩大scope、不移除校验或冻结后追改。

## B：只读登记缺来源终态

原 `TestNativeRegistrationLiveRestoredMissingTerminalReceipt` 和问题不变。实际2次planner请求、2次read_file＋1次emit_change_plan，完整读取原源码和原断言，changes=0。当前辅助参数索引漏记这类日志布局，原日志3调用仍完整，不能由空数组说零调用；旧采集欠项继续留18.4。

原来源准备有真实apply/Git坐标，但无verify attempt、VerifyRef或Completion，不是删除成功终态制造负控。登记后由独立新进程（parent57213、child57367）通过生产恢复入口与公共RunTests执行保留断言，登记阶段源码/测试/HEAD及diff/status不变。报告写入是验证产物，不声称整个文件系统零写入。

| 范围 | 结果行 | 命令 | 登记凭证 | controller验证 |
|---|---:|---:|---:|---:|
| 历史来源 | 1 | 2 | 0 | 0 |
| 本轮proof | 1 | 2 | 1 | 1 |

旧invocation `native:df7d:18e32a6a51cf4720:1` 与新 `native:e017:18e32a6da6948718:1` 不同。source的流程status=complete不是验证成功；proof verified，整体仍 `unverified / missing_terminal_verify_verdict`。有限负控PASS，不代签来源正向或自然controller。

真正WriteFinalReport位于父/子归档 `plan-1793323726749925000-57213.final.json`，不是根 `final-report.json` 兼容ChangeReport。其report_id与artifacts路径指本轮内容地址报告；当前1条结果/2命令，与历史分开。父/子各4工件/1attempt/1link，8份副本逐项bytes/SHA一致，同名四原件字节相同。

当前原始report SHA256 `e5c6d63d76c93fe3cc9e7150ee03a7d5b70d6012ae572c8ef2a2b5da2f2fd77f`；canonical内容SHA `519d0f9ac5fc67cc6ada9f5aae9861479384b10b918ea04e26300a15f35f83e9`，不混同。最终文件SHA `fd3d1adc2db45f9723383268a68b58ce414ebea5506f2bbdca20593bb19654c7`；旧原件保留，过期临时原路径由带来源与hash的副本审计，不假设永久存在。

context_pack_durability_verified=false及物化 `apply_checkpoint_missing` 如实披露，不扩大为所有恢复/持久化范围都已验收。本轮B没有模型最终MD/HTML/图，不冒充这些表面的自然能力验收。

## 完整性与余账

唯一全仓98包PASS，冻结7510/124/16一致；恰好2例各1次，实际最大请求重叠8.441秒。A的1个闭合无ID重叠组保留歧义，不能倒签13个逐请求时间。旧16档4949件、359快照来源身份未变（18历史污染为其中子集）。

完整能力新增0、有界子能力1组、53开放不变。新状态自然未触发、图交接/预算、正文状态/关系解释，以及旧代表选择、采样/IO/权限覆盖和扩大race欠项继续留原ID。WAL零副作用未满足前生产NO-GO；提交推送只证明同代代码与证据已交付，不改人工FAIL。
