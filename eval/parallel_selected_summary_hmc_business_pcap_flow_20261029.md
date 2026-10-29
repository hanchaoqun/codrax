# HMC306：原边界闭环与双向端点原生统计

日期：2026-10-29（UTC批次日；本地2026-10-28）。基线`de71141b172f15224eaa4f9a15871d0ae325962a`，候选`codex/hmc299-wait-interval-candidate`，main保持`788588e365d40a48b30827ffd8db08dae4e2ae9d`。严格冻结后未改源码、测试、注释或helper；未追跑第三例、未二次全仓，不进main。

## 对账与交付

逐稳定ID重核79唯一＝25已交付＋54开放，18类；19部分实施/29待实施/3待验收/2验收中/1持续执行。

- 完整能力交付：本批新增0，累计25，main新增0。06.1仍开放，不把端点组合统计当协议会话、跨源时钟或所有入口完成。
- 已实施候选子能力：3组——精确DTO导入边界、定位包络与合法小计并存、PCAP双向端点完整聚合到原生表的链路。
- 剩余验收缺陷：新准备预算回归、目录与执行入口适用域、调度片段混带全窗量测、正文与同源表/图解释不一致分别留账。旧采样/IO/代表选择、权限未触发及WAL条件不变。

## 双轨实现及参考取舍

缺陷轨公开复现并闭环§305原导入边界与B1688说明失败。`tracediag`通过`tracequery.Result`及已审查的精确字段路径取得六个既有DTO类型，不新增生产`tracewire`导入，不扩白名单、不按类型名/包名猜测、不递归放行未来结构。共享定位说明恢复“包络可能重叠、包络时长不可相加”，同时保留明确成员/同量尺的合法小计；不改模型原文、原query、ledger或权限。旧断言未放宽。

能力轨新增`packet_flows`：复用held-generation PCAP扫描，按来源代次、link/interface已知性及值、VLAN栈、IP版本、协议和规范端点建立双向组合。端口0有效，IPv4-mapped IPv6不折叠，未知接口不变0；A/B不表示本机/远端或客户端/服务端，同端点单列。完整扫描后按captured帧字节排序再截预览，最多4096组内存和64组展示分别约束；每包最多4KiB前缀，1GiB稀疏包回归验证不按payload分配。预算超限仍计已跟踪组及排除数，但FlowCount未知、partial不签完整或全局Top N。坏尾、分片、未知端点、未支持协议保原因，不补零。

工具独立packet分派、精确参数域、来源前后复验、完整结果发表和原生summary/members/distribution同源；不继承Trace时间/线程/行筛选，不对齐epoch、不授因果/会话权限。两类帧字节与观察min/max独立展示，成员表不用长内部flow ID。新增六类DTO及`Result.PacketFlows`精确演进见证：新完整哈希`8a6e67555e8d211b42a85a74170fbb011a02b5d08e8472440d0d413e00abd6bb`，仅逆转新增字段后恢复§305的`068194140d8edb5f26727356721b8640eeb1f2a1cd2a2743f1b3ccedb194cd20`，再沿旧链复核；历史expected hashes/旧字段/tag/顺序不改。

参考仓`core/extensions/network_pcap_ops.py`和`preprocess/tcpdump_parser.py`以端点键聚包、按方向/协议支持网络排查；其有向四元组不保接口/VLAN观察域，DNS只按ID配对、SYN→SYNACK不足以证明完整握手，解析后仍累积list。本批保“可追溯原生包事实”的设计意图，改为有界流式、完整观察域身份与未知/partial分层，不照搬会话或应用语义推断。参考`query_engine.py`先保总体再降采样、`llm_contract.py`明确成员/字段绑定，落实为本仓已有原生发表和权限机制；不引入源DB重写、关键词硬门或文件名校时。WAL仍生产NO-GO。

## 冻结前验证

| 范围 | 结果与边界 |
|---|---|
| pcapinput/tracewire及race | 各2包54顶层/122子例PASS；独立真实503组oracle和公开前64组核对一致 |
| tracediag完整包及race | PASS；包括原导入边界、精确schema逆变换、真实低层结果渲染；不代签collect执行 |
| tool/tracequery/types/traceinput/skill完整5包 | 14106顶层/13517子例PASS，27顶层/14子例SKIP，零FAIL；日志SHA `f4f8929ad6dff913137d3b06cedbc9824e3b77f2fad6249e76edc42b8d1b6e0b` |
| 公共查询/原B1688/新小计及相关race | PASS；最大64组IPv6/8层VLAN/大数发表预算另测PASS |
| helpers | 原始NUL/非法UTF-8、歧义请求、不同payload类型、旧1692封存/18快照自测PASS；零模型 |

保留原始RED及施工期失败，不伪装全程绿。标准CLI只构建一次，SHA`8af5194280c7d3338e013ce00d806ad27dde68978a13af8e4801c19a3b97918e`。冻结7461仓库输入＋66外部＋31仓外快照，SHA`336c66fc84c0f272ee120fadce72bf37f1743db9574dcf57db21751a2650c743`；所有快照在仓外，Go源码副本用`.go.txt`，不污染包发现。凭证内容未读/hash/复制。冻结前完整相关包仍漏了outputdump的准备消费者，本次全仓失败不能称为仅测试噪声。

## 两例普通CLI，各一次

| 侧 | 自然问题/材料 | UTC起止 | 机器 | 完整人工 |
|---|---|---|---|---|
| A | 原OpenDocument时序问题；既有人工编写ftrace夹具，不称新设备采集 | 02:14:15.925688–02:21:03.496660 | PASS | FAIL |
| B | “这份抓包里主要有哪些端点在通信？按双向端点组合统计两边的包数和包长，列出最活跃的几组及覆盖时间。”；真实参考PCAP原字节 | 02:14:15.928604–02:19:10.670280 | PASS | FAIL |

日期均2026-10-29。两例均普通CLI、15步、共同ready barrier；实际可唯一配对请求A15/B24、1歧义不猜配，最大已证重叠56.206秒，非进程重叠推断。没有改问题加入系统约束/期望数值，没有预种生产准备、查询或目录。

B原件SHA`099ddab49c3c957b5e311d11e8f9ba2e2d7a380f0910bd8becf27a394fbac91a`、3499182B。独立Ruby逐包字节遍历＋离线tcpdump核6101记录、TCP4336/UDP1627/ICMP138；5963归组/138不归组，503组，captured/original帧字节各3401542B，原生epoch观察范围1789020987.035118–1789021016.416211、4倒序。自然4份flows结果limit40/1/15/20与4份inventory结果分别留原件；40唯一组的双方向、两类帧字节、截断、前缀受限和min/max全部与独立oracle一致。原件/查询3499182B与预览680B分开。正文仍把端点组合说成TCP连接、错述第13/15尾部、第8最早结束、第4后半起点和第12方向，且15组叙述与40组表不一致；全答FAIL。

A保住31ms提交线程闭合等待、35ms请求驻留、50ms业务与52ms查询、合法两成员2ms小计。图三唯一线程lane、两有证唤醒箭头与事件点顺序有限正确；业务角色未证关系边界不能据此擅删。正文仍倒置44ms与31ms包含关系、两处把后台W写成读。更重要的是，单个1ms`SchedulerLatencyItem`携带37.5ms其它CPU空闲，实际来自全查询窗；源码还同样携带全窗same-CPU busy/idle、高优先级/系统运行量，而另有真正片段overlap量。这是共享范围交接缺口，不仅是模型波动，不能只拟合37.5ms或修改正确原始统计。详见[逐面人工审计](parallel_selected_summary_hmc_business_pcap_flow_20261029_manual_audit.md)。

## 唯一全仓与新开放缺口

UTC 02:14:15.495551–02:25:31.734298，exit1：97包PASS/1FAIL/12无测试；28444顶层/33410子例PASS、1顶层/1子例FAIL、43顶层/14子例SKIP。252389行全JSON，无重复终态或未结束。SHA`ad5c080ff1213f676b6bc834f429c1f1de9bd30af97c3fbd9c533217fc79cb84`。运行前后及独立verify的7461＋66＋31输入均不变。

唯一失败`outputdump.TestHMC305PreparedAttachmentSeparatesOriginalQueryAndPreview/packet`：原测试640B预算，失败真实路径141B。同路径导航基线621B，本批759B，增长138B、超119B，错误为`packet_capture_preview_budget`，后续元数据断言尚未抵达。当前固定开销618B，仅剩22B给路径，是普遍可用性回归，不是坏PCAP或极端路径。下批应把完整来源/必需准备事实与精简导航置于不可拆最小envelope，重复教学按整单元预算加入；真放不下仍fail closed，不能截路径/改旧预算/放宽断言。§305原`TestTraceDiagImportBoundary`、B1688 zh/en及裸区间检查已全部PASS，不抵消此新失败。

另有静态确认的共享目录适用域缺口：CanonicalViewNames包含packet双view，脚本接受后却仍buildStepIndex→indexed Run；V2更早已进入Trace文本来源锁。只有工具入口实际PCAP分派。渲染测试低层结果→renderStepBody不覆盖脚本执行；文本源＋packet view可能返回invalid Result但被err=nil当步骤成功是静态风险，本批没有额外运行，不能写成已跑复现。下一批补精确入口能力/参数域，保全目录枚举及原导入边界；未实现入口先诚实拒绝，真实V1/V2适配另以公开矩阵验收。

## 下一ROI与未销账范围

1. **已证回归**：共享准备导航的最小必需内容与预算兼容，保640B原断言；覆盖所有准备/展示消费者，而不是仅加新视图测试。
2. **高影响事实交接**：SchedulerLatencyItem片段范围与全窗资源量尺显式分层，保既有精确overlap、原查询总体和因果资格；调查摘要、正文、表、图同源消费。
3. **完整能力轨**：共享目录的可枚举/可执行入口及参数域，再接真实PCAP脚本路径；独立保持05.2校时、06.2协议会话退出条件。轮换只读登记、sample-only/代表选择，17.6新材料到位提级。
4. **解释与覆盖债**：已有充分数字仍误述的自由正文留原16.4/16.5/18.4，不追第三例求绿；旧采样、IO、栈关系、角色/权限未触发均不回签。17.7 WAL零副作用未满足不接生产。

UTC2026-10-29 02:34:25.524707，原始失败、实际INIT、submitted参数、native payload、MD/HTML源、两独立逐例审计及入口/预算审计共328件按原字节封存，索引SHA`973347949f45a9fcd03af9827afda88f6b31ce183fd20d75cb51d1669b44502d`。旧1692件/18历史快照及本批7461＋66＋31冻结输入复核不变；封存按canonical realpath＋SHA去重，同字节不同来源保留。HTML源审计不是像素QA。

候选代码`d4ca25e4e35c93f89df31f3b97ae974d2d278a15`（25文件）与五份文档紧邻分笔提交、普通推送；最终远端同代/clean/main不动及两提交源码与冻结字节一致，以[封存索引](results/hmc_business_pcap_flow_20261029_v1/sealed-index-v1.json)、[原始终态](results/hmc_business_pcap_flow_20261029_v1/sealed-receipt-v1.json)、[发布收据](results/hmc_business_pcap_flow_20261029_v1/publication-v1.json)为准。封存/发布不授验收PASS，54项不减。
