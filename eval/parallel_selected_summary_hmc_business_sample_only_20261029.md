# HMC309：缺测类型交接、完整可见行与 sample-only 轮换

状态：候选冻结验收未通过；54 开放项不减，不进入 main。

## 范围与计数

基线 `779fb97e72c83659f373365d3a6043ff6800e855`，候选 `codex/hmc299-wait-interval-candidate`，main 固定 `788588e365d40a48b30827ffd8db08dae4e2ae9d`。根代理和独立代理逐稳定 ID 核对 **79 唯一＝25 已交付＋54 开放，18 类**；19 部分实施、29 待实施、3 待验收、2 验收中、1 持续。01→18 类开放数为 2/1/1/5/2/3/4/6/3/2/1/5/3/5/3/2/2/4。

**完整能力新增 0；候选子能力 2 组；main 新增 0。** 子能力施工、机器 PASS 与历史完整交付数分别记账，不关闭父项。

- 缺陷轨（16.4/16.5/18.4）：原生缺测种类到成文的适用域交接；真实 read_file 展示预算、完整可见行和导航凭证。
- 能力轨（18.3）：轮换已有真实 PERFILE2 的原 sample-only 停顿问题，验收现有入口与最终解释。不是新增解析器、BRBE 解码或新设备材料。

参考仓实际核读 `core/context_index.py`、`wakeup_chain_index.py`、`sleep_ops.py`、`hiperf_binary_parser.py` 与 `config/indicators/sampling/hiperf/hot_function.yaml`：来源/步骤导航不等于结论授权；上游调度缺测不抹去已经记录的下游唤醒；采样 COUNT 与 period/SUM 是不同量尺。移植这些分层意图，不复制宽泛邻近匹配、全量列表或用名称猜因果。Trace 投影、自动补齐、原四根因合同、闭合睡眠、独立 RQ/BIO、链上优先级及合法计算均保留原断言，没有关键词硬门。WAL 仍生产 NO-GO。

## 公开复现与候选实现

1. **缺测域**：真实 TraceQuery→BuildInitialInstruction 公开 RED 证明 `trace_gap=no_sched_data` 被统一写成缺匹配唤醒，并错误标为完成身份未知。现在沿已有 typed `TraceGapKind` 分开无调度区间、无达到最小时长的区间及真正 missing_wakeup；未知保持未知。对齐缺测范围未由生产者发表时明确未发表，不拿查询窗补成缺测范围。独立已证完成身份、唤醒边和量测不撤销，来源不完整也不按同名同值合并。无新增 schema/查询权限。
2. **可见行**：真实 read_file 公开 RED 证明长单行 JSON 在 banner/gutter 计入后零完整行可见，却声称已读完并提示跳到下一行。候选只按生产者实际最终字节预算计算完整可见行，正常部分页使用原件绝对行号，保原始总行数、原件和 runtime 身份；不再用 blob 包装行号指示续读。零可见行候选返回独立预算失败，保存失败与文件不存在分开，合法其它行/有界结构化取值不封死。**此失败返回与旧源码版本预览合同冲突，全仓已证，尚未完成兼容修复。**

原件均以 `/private/tmp/codrax-hmc309-` 为前缀，公开 RED、初版失败、末版 GREEN 和命令收据完整留存：

| 验证 | 证据及结果 |
| --- | --- |
| 缺测公开 RED/GREEN | `gap-public-red-v1.log`、`gap-public-green-v1.log`，中英真实成文入口 |
| 缺测保护 | `gap-protections-v1.jsonl`：4 包、110 顶层/504 子例 PASS；`gap-race-v1.jsonl`：37 顶层/29 子例 PASS，零 FAIL/SKIP |
| 长行公开 RED | `read-long-line-red-v1.log`：源码/runtime、offset 0/1，4 子例原失败 |
| 读取末版相关 race | `read-final-related-v2.log`：4 包、107 顶层/116 子例 PASS，零 FAIL/SKIP；SHA256 `f97164a41fa5b4e1b7e2098713db6f9afc59f2de94ecc68b0cce4ada8a36ec9c` |

`gap-related-v1.log` 保留无凭证同名缺测错误去重的新反例失败；冻结前修实现，不降旧断言。`read-long-line-green-v2.log` 的新 256B 夹具误以为 banner 加一整行仍能容纳，改新夹具为 512B 后通过；原失败保留，不改变既有预算/旧断言。独立审查还补了误导后缀 `capture.go` 的受信 runtime 身份及 RawRef 保存失败的诚实提示。

## 输入、构建与冻结

A 保原 `trace_business_io_sequence.case` QUESTION/EXPECT/15 步和 2312B 人工编写 ftrace；不冒称设备实采。B 保原 `trace_binary_sample_only.case` QUESTION/EXPECT/15 步，仅附件改为本批完整同字节副本：参考仓 `tests/fixtures/hiperf_brbe_001/test_brbe.data`，**30,062,585B**，SHA256 `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`。完整原件 12000 条/43037682 权重、窗口 1393 条/2455 权重、目标 164 条/259 权重的既有独立 oracle 原封留存，历史绑定与当前副本绑定分开；没有重新运行旧构建 harness、预查询或预填生产目录。

唯一标准构建 `go build -o /private/tmp/codrax-hmc309-cli-standard-v1 .` 于 UTC 07:06:32.355640–07:06:43.419353 成功，输入前后相同，二进制 SHA256 `7282e0ff287a1b509a1550f33521a83fd9f7f4d0abe1a609db92a4a3ea860304`。

冻结 `frozen-v1.json` SHA256 `1eab4277158ac7375e55fe269815a50d52b576a2cb77e08aa0c4a1f44d55dc8e`：**7483 仓库输入＋115 外部输入＋14 仓外快照**。源码/测试/注释/helper 从此停写，仅新增审计输出及五份批次文档。旧 **2694 件**封存（含 **238 个快照身份**，不是另加 238 件）不变，18 个已隔离污染路径没有恢复；Go 快照用 `.go.txt`。provider 内容未读、哈希或复制。

## 固定两例与唯一全仓

两例同一 barrier、普通 CLI、n=1，无第三例或追跑。A/B 各 18/9 个可唯一配对的已结束实际模型请求区间，1 个歧义区间排除，实际请求至少重叠 **26.159 秒**；不以子进程并行冒充模型并行。

| 例 | 实际执行 | 机器 | 完整人工 |
| --- | --- | --- | --- |
| A 业务 IO 时序 | UTC 07:08:24.020108–07:13:41.891614，exit 0；9 次 trace_query | PASS | FAIL |
| B 原二进制 sample-only | UTC 07:08:24.022639–07:10:39.598991，exit 0；7 次 trace_query（6 成功、1 参数范围拒绝），exec_command 1 次执行前拒绝 | PASS | FAIL |

两例 `read_file=0`，本批长行修复 **NOT_TRIGGERED**，不得拿模型不报错当自然覆盖。A 新缺测类型进入真实 finalizer INIT 并改正相应文字；不以这一局部改善签整答。B 正确原生表及真实 INIT 已提供完整统计/各热点时间、CPU、未知采样状态、period/记录数区分，仍有自由正文解释越权。完整过程、拒绝/补丁、摘要、正文、表、图、HTML 源与旁路见[人工审计](parallel_selected_summary_hmc_business_sample_only_20261029_manual_audit.md)。HTML 源审计不是像素 QA。

唯一全仓 `go test -json ./... -count=1 -timeout=30m`，清除 live opt-ins，UTC **07:08:21.803999–07:19:43.444716**，exit **1**：**97 包 PASS、1 包 FAIL、12 无测试；28497 顶层/33633 子例 PASS，43/14 SKIP，1 顶层/1 子例 FAIL**。260477 行全 JSON，无重复终态/未结束；日志 SHA256 `b0907628fb0c8f24be0d88de80d0f8c48bd0101c09587e86762e8ebf4603ced8`，冻结输入前后相同。

唯一旧失败 `TestDispatchRepositoryReadVersionPublicPreview/one_truncated_line`：真实可读预览原合同要求成功且有非完成的 ReadCoverage，候选零完整行改为失败/无 coverage，公共辅助函数先拒绝。完整文件版本权限原本另有 `displayComplete` 等精确条件；必须兼容“读到原件/可导航”与“完整可见字节/完成权限”，不能通过改旧断言解冲突。最后相关命令名称筛选未匹配 `DispatchRepositoryReadVersion`，因此漏选该旧消费者；这是本批覆盖缺口，不归环境波动。冻结后未补改、未第二次全仓。

## 剩余验收与 ROI

本批按共享原因归 **5 组剩余验收缺陷**，另 **1 项长行自然入口未触发**，不重复增加稳定任务数：

1. **最高：读取预览兼容合同**。保零完整行无完成权、保原件可读/runtime 身份/合法结构化导航，同时恢复旧成功预览消费者；下一相关矩阵必须覆盖原 `DispatchRepositoryReadVersion` 全组。
2. **高：图主体与已验证关系覆盖交接**。A 首稿已有两条带真实实例锚点的唤醒关系，系统却要求三个业务角色声明未证；修补后保留合法箭头同时自动插入“当前证据未证明用户所请求的有向关系”，是系统驱动矛盾，不一概归模型。
3. **A 正文同源时间/榜单解释**。两个不相交 runnable 区间误称同一时刻；原榜单仍有 #3，自由正文将展示席省略解释为排序不完整。原生事实不缺，保留原失败，不词面硬改。
4. **B 采样时间与执行状态越权**。约 122.483ms 的两簇记录间隔被说成内核采样持续约 0.12s，真实内核记录包络仅约 1.514ms；未知类型仍被解释成持续停留/停顿位置。权重不是时长、采样记录不是完整执行状态；18.3 不销账。
5. **B 聚合完整性与事件预览混用**。以局部 event_search 预览截断推测还可能有第三热点，否定已完整 164/259、2 节点零省略的热点总体。各查询的完整性、预览和成文解释仍需同源分层。

完整能力轨优先回到 **06.1 已有真实 PCAP 的最后共享展示欠项**：原合同中的流式读取、接口/包序、IPv4/6、方向/流键、坏尾/重复/有界内存和时钟隔离已有实现与正反测试，不能反复说尚未实施，也不新增“每种边界都必须有实采”“自然选择脚本”“PCAPNG/会话/跨源校时”作为原父项退出条件。下一完整交付切片聚焦整组预算：紧凑未归组原因/字节/时间优先于大成员表，组不从中截断，原生预览量与最终显示量分开；复用已有真实字节 oracle/公共 CLI 证据，不连续追同一模型问题。06.2 会话语义和05.2校准独立；17.6真实新格式/工具/宿主材料到位再提级。只读登记原终态负向/正向轮换，旧 IO、代表、栈关系与权限未触发继续原账。WAL 未满足零副作用不接生产。

## 封存与候选发布

UTC **2026-10-29 07:31:10.613625**，两例原始失败、独立审计、唯一全仓失败和冻结输入共 **296 件**按 canonical realpath＋SHA256 封存，索引SHA256 **`22b49880fba69b042d2075774076b51e2842c030500cfd819c1fc2a7cee64e46`**。不删同字节不同来源身份；旧2694件及冻结7483＋115＋14输入复验未变。

候选代码 **`b5bdc7ee2e5520f0f436057513db22560aa0ad04`**，六文件，父提交为本批基线；五文档单独紧邻提交。普通推送后的完整性凭证记录两笔完整SHA、远端candidate一致、clean、两提交树与冻结输入同字节、旧/新封存未变及main固定。**不进入 main，不减54**；候选发布不表示全仓或完整人工通过。

- [封存索引](results/hmc_business_sample_only_20261029_v1/sealed-index-v1.json)
- [封存及实际进程终态](results/hmc_business_sample_only_20261029_v1/sealed-receipt-v1.json)
- [候选发布完整性凭证](results/hmc_business_sample_only_20261029_v1/publication-v1.json)：只证明两提交同冻结字节、普通远端一致、clean、旧/新证据未变及 main 固定，不代表全仓或人工通过。
