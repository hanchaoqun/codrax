# HMC308：查询身份与根因实例分离、真实 PCAP 脚本执行

状态：候选冻结验证结束，完整人工验收未通过；54 开放项不减，不进入 main。

## 范围与任务数

基线 `cbc79bc436a46d79c025d94283acb6e339a84e5d`，候选 `codex/hmc299-wait-interval-candidate`，main 固定 `788588e365d40a48b30827ffd8db08dae4e2ae9d`。主代理与独立代理按稳定 ID 复核 **79 唯一＝25 已交付＋54 开放，18 类**；19 部分实施、29 待实施、3 待验收、2 验收中、1 持续执行。完整能力新增 **0**、候选子能力 **2 组**、main 新增 **0**；不得把子能力或机器 PASS 当作父项完成。01→18 类开放数依次为 2/1/1/5/2/3/4/6/3/2/1/5/3/5/3/2/2/4。

本批两轨：

1. 16.4/16.5/12.5/18.4：公开复现同一闭合 IO 实例因 PID/name 查询选择器不同，被重复投影为两个根因。原查询、窗口、量尺、payload 和引用各自保留；仅以受信解析目标、源代次、原始请求端点、闭合区间和同榜单依据合并投影。
2. 01.2/06.1/17.6：`packet_inventory`、`packet_flows` 的原生工具和 tracediag 共用 `StreamPacketQuery`。v1/v2 静态脚本可对单物理经典 PCAP 实际执行；Trace 坐标/发现/混域、indexed Run 和 Trace 恢复不借此取得 packet 权限。

参考仓实际核读 `core/query_engine.py`、`core/preprocess/network_pcap_ops.py`、`wakeup_chain_index.py`：复用显式执行分派、内部实例身份及完整统计与展示分层的意图，不照搬全量内存列表、零命中回退全量或名称推因果。原四根因、31ms 闭合睡眠、35ms 请求驻留、独立 RQ/BIO、Trace 因果投影、优先级关系、合法计算和自动补齐维持原断言；没有关键词硬门。WAL 仍生产 NO-GO。

## 公开红绿与相关验证

原始证据均使用 `/private/tmp/codrax-hmc308-` 新路径，旧失败与 2308 件历史封存不改。

| 验证 | 原始证据与范围 |
| --- | --- |
| IO 公开 RED | `io-selector-red-v1.log`：真实 `TraceQuery.Execute` PID→name/name→PID，两份原件/同榜单被投影为 2 个 IO 候选 |
| IO 公开 GREEN | `io-selector-green-v3.log`：两份原查询仍可寻址；1 个候选/1 个中英展示席，31/35ms 各自保真 |
| IO 保护与 race | `io-protections-v2.log`、`io-race-v1.log`：各 37 顶层/154 子例 PASS，零 FAIL/SKIP；包含原四根因、闭合等待、独立请求/量尺及自动补齐 |
| packet 入口 RED | `packet-entry-red-v1.log`：v1/v2 公开 Run 无实际适配；`execution-contract-red-v1.log` 保旧执行目录/严格类型依赖反例 |
| packet 最终 race | `packet-race-v3.jsonl`：4 包/55 顶层/108 子例 PASS，1 个旧真实 oracle 环境 SKIP、零 FAIL |
| packet 相邻包 | `diagnostic-packages-v2.jsonl`：tracediag/pcapinput 216 顶层/169 子例 PASS，1 SKIP；`shared-cmd-related-v1.jsonl`：23 顶层/42 子例 PASS |
| 根代理消费者 | `causal-consumers-v1.log`、`tool-types-v1.log`：tracefinding/context/agent 与 tool/types 的相关消费者通过；12 子例公开工具旧 JSON 包络精确对照通过 |

新测试前提错误不隐藏：`io-selector-green-v2.log`、`io-protections-v1.log` 原先错误假定来源贡献列表去重且无账本补入 ToolCallID；只将本批新增断言改为最终账本两种原始凭证精确覆盖、无第三源，未动旧合同。`diagnostic-packages-v1.jsonl` 发现新模板 1600 行预算违反旧 950 行约束，模板改为 450×2，旧断言不变。`packet-race-v2.jsonl` 的新换代夹具因 `/var` 与 `/private/var` 路径未规范化而提前拒绝，只修夹具规范路径，保留第一步后真实换代/零报告断言。全部原 FAIL 留存。

独立源码审查确认入口测试实际经过生产路径；发现的普通 `os.Open` 来源哈希风险已在冻结前消除，改为同代安全 regular-file 打开、64KiB 分块、取消及句柄/路径双重验。静态审查不代签模型效果。

## 构建、真实脚本与冻结

唯一标准构建 `go build -o /private/tmp/codrax-hmc308-cli-standard-v1 .` 成功，二进制 SHA256 `1aaf30e4b520d135c6d6c6dc4e4456d7ada670fcb6737288cab49051e5e5e22b`，构建前后输入相同。没有替代构建或测试二进制用于真实评测。

真实脚本命令为该二进制的 `--tracediag examples/tracediag/collect_packet_capture.yaml --trace /private/tmp/codrax-hmc308-operator-v1/tcpdump_cap_061627.pcap --out /private/tmp/codrax-hmc308-pcap-script-report-v1.txt`，exit 0；报告/日志/收据独立保留。原件 3,499,182B、SHA256 `099ddab49c3c957b5e311d11e8f9ba2e2d7a380f0910bd8becf27a394fbac91a`。这是 **零模型公开 CLI 执行**，不是第三例模型测试，也不证明模型自然选择了脚本；报告未注入 A/B。

独立脚本审计对照完整字节 oracle，2,464 项已发表字段核对及 30 组身份/排序均一致；6101 记录、5963 已归组/138 未归组、503 组正确，原件与副本字节未变。报告 615 行/87,328B，SHA256 `0f1c6281eacaac9b6dceef69cb388891b4b2abc94f68c106ab300b5576b1d902`。64 条记录预览完整发表，但流展示只完整 29 组、第 30 组局部，450/972 行、明确省略 522 行；不能把原生 64 组预览写成最终完整展示 64 组。未归组 138 计数在，原因/字节/时间明细被行帽截在后部，继续留展示欠项。只签实际 v1、classic-PCAP/SLL2/IPv4 静态脚本范围 PASS；v2/IPv6 等测试、自然脚本路由及 06.1 其余合同独立，不代签。

准备助手只复制原件与既有问题、独立 Ruby 字节 oracle 和系统 tcpdump 前 12 条；不预执行生产查询、不预填目录、不读/哈希/复制 provider 内容。首次零模型 preflight 因主代理未加 wrapper 执行位失败，原收据保留为 `preflight-attempt1-v1-receipt.json`；只补执行权限后正式 preflight 通过，不改 helper/source 或重建。

冻结 `frozen-v1.json` SHA256 `ecb92ba1d8973655c0a7a434b444b17e1456b7b6c9d8daf2d22672616d99a6e7`：**7479 仓库输入＋76 外部输入＋28 仓外快照**。所有源码/测试/注释/helper 自此停写。快照均在仓外，Go 副本为 `.txt`；旧 2308 件中的 210 个快照身份是子集，不另相加，旧 18 个污染路径没有恢复。

## 固定两例与唯一全仓

- A：原 `trace_business_io_sequence.case`，原合成 ftrace，OpenDocument 自然问题不变。合成材料不冒称设备实采。
- B：原 HMC306 `pcap_packet_flows.case`，仅附件路径改为当前批同字节副本，问题与 EXPECT 不变，真实 PCAP 内容不改。
- 两例均普通 CLI、15 steps、n=1，同一就绪 barrier；A/B 各 14/50 个可完整配对的模型请求区间，1 个歧义区间排除，实际请求已证重叠至少 **21.083 秒**，不凭进程并行推断。
- 唯一全仓为冻结后的 `go test -json ./... -count=1 -timeout=30m`，清除 live opt-ins。UTC 06:20:22.046929–06:31:58.245745，exit 0：**98 包 PASS/12 无测试，28488 顶层/33617 子例 PASS，43 顶层/14 子例 SKIP，零 FAIL**；254316 行均为 JSON，无重复终态/未结束。日志 SHA256 `bef771c47f579ea7599cec46da7bf4d63c5acf481b26d35add70d8d509a4a282`，冻结输入前后相同。

两例机器均 PASS，完整人工均 FAIL；实际执行、完整 INIT、原生 payload、成文、图、HTML 源与旁路由独立审计逐面核对，见[本批人工审计](parallel_selected_summary_hmc_business_pcap_script_20261029_manual_audit.md)。A 保住 50ms 业务/52ms 查询、31ms 闭合等待/35ms 请求驻留及合法 2ms 小计，但图修补产生 6 主体/3 孤立同名角色；真实只用 PID，不自然覆盖新 PID/name 去重分支。B 原生 6101 记录/503 组和逐方向数表正确，正文与自写表却把总体时间套给首组，并出现方向/尾部排序及主机/并行关系误述。正确原生表不能代签错误正文。HTML 源审计不是像素 QA；正式失败不重跑，不追加第三例求绿。

## 收尾与剩余

按 ROI 保留两轨，不重写已正确的统计器：

1. **高影响缺陷轨**：先公开复现共享证据边界把“无调度区间”硬写为“无匹配唤醒记录”的错误适用域；再处理调查摘要错误派生值与原生同源成员并列交接。B 实际单行 JSON 文本提取已把全局 min/max 绑首 member，并通过 Known Facts 到成文，不能仅称模型随机；保合法计算，不据命令名或问答关键词硬拒。
2. **完整能力轨**：轮换只读登记原有限终态/sample-only，或有实际新材料的二进制合同；06.1 仅完成有界单物理经典 PCAP 的静态脚本子能力，不新增会话/跨源校时等完成权。脚本原生预览与最终显示完整性、紧凑未归组分类优先于大成员列表的预算复用继续留账。
3. 图既有节点身份与修补声明的绑定、重复摘要 30/40/64 的预览/显示口径、PID/name 自然未触发及旧采样/IO/栈关系/代表/权限欠项仍属原稳定 ID，不另造重复任务。冻结后不追改；WAL 零副作用未满足仍生产 NO-GO。

## 封存与候选发布

UTC2026-10-29 06:44:42.768610，**386 件**原始证据封存，索引SHA256 `a0f360e22ca255fee0822bd5ab197bbb5e9312e82f30e23d7958daffd798285e`。旧2308件归档及7479＋76＋28冻结输入复验未变；本批封存按canonical realpath＋SHA去重，保不同来源身份，不以同字节删除来源。失败和未触发未改写。

- [封存索引](results/hmc_business_pcap_script_20261029_v1/sealed-index-v1.json)
- [封存与三进程终态收据](results/hmc_business_pcap_script_20261029_v1/sealed-receipt-v1.json)
- 候选代码 **`85bea6e2468dd31edd754dff15abd2470ca8329b`**，21文件，父提交为本批基线；五份文档单独紧邻提交。
- [最终发布完整性凭证](results/hmc_business_pcap_script_20261029_v1/publication-v1.json) 在普通推送成功后生成，记录两笔完整SHA、远端candidate一致、clean、两提交树与冻结7479文件逐一同字节、旧归档/本批386件未变及main固定。不以口头“已推送”代替该收据；凭证只证明候选发布，不代表完整人工通过。

本批剩余验收按共享原因5组，另1项PID/name自然覆盖未触发，具体见人工审计；不重复增加稳定任务数。任何发布均不改变main或旧verdict，54开放保持，WAL生产NO-GO。
