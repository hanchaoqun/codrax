# HMC305 两例逐面人工审计

日期2026-10-28（正式进程UTC为2026-10-29）。基线`8e2c14bb7ec63058c9dc4047fdae3cba833f87eb`，候选`codex/hmc299-wait-interval-candidate`。本报告不重跑模型、不改QUESTION/oracle、不改冻结源码/测试/helper；原始机器判定与人工结论分开。

## 1. 总结与范围

| 对象 | 机器 | 人工 | 可以签署 | 不可代签 |
|---|---|---|---|---|
| A 原只读登记缺来源终态 | PASS | 有限负向合同PASS | 当前新证明与来源终态缺失分离；恢复执行/当前计数/同代原件引用 | 自然controller、完整CLI、正向来源终态、所有登记欠项 |
| B 普通PCAP自然问题 | PASS | 完整答案FAIL | 真实只读取数、原件/查询/预览元数据、三张原生表 | 自由解释正确、协议会话/加密、进程归因、完整06.1 |
| 唯一全仓 | exit1 | 两类接缝FAIL | 旧8项schema失败本轮按原历史哈希PASS | 候选准入main |

不将不同范围的A限定PASS与B完整FAIL合并为“完整答案1/2”。稳定台账79唯一＝25已交付＋54开放，18类；完整能力新增0，候选子能力3组，main新增0。失败/未触发各在原稳定项下留账，不创建重复父项，不将机器PASS或封存视为人工通过。

## 2. 冻结、真实重叠与证据原件

- 冻结清单：`/private/tmp/codrax-hmc305-frozen-v1.json`，SHA256 `ae707a28c733d69f85cb146be177aa62cf507ebb29ad4a8452d14ac0005b5207`；7451仓库输入、59外部输入、26仓外快照。Go源码快照为仓外`.go.txt`，不增加全仓发现的包。
- 标准CLI：`/private/tmp/codrax-hmc305-cli-standard-v1`，SHA256 `91da74c03e2639792a0c3a47d72c83dc5c3a95ab7ea4586fae2f8680040873bf`；只构建一次。orchestrator测试可执行文件也只构建一次，编译不增加真实例数。
- 正式A：`/private/tmp/codrax-hmc305-live-A-v1-receipt.json`，UTC 01:19:06.885866–01:19:27.032117，exit0。日志SHA `cf29fae4e9984bf6bf12d725c528cd223b6cc5032e1c1197dcd1c7175f274621`。
- 正式B：`/private/tmp/codrax-hmc305-live-B-v1-receipt.json`，UTC 01:19:06.888508–01:22:47.277543，exit0。外层日志SHA `a1bee90942ded0274278abe076e38c7d09d567ed791ca865ad2d1334f9a28563`。
- 真实请求配对：`/private/tmp/codrax-hmc305-actual-request-overlap-v1.json`，A2/B15个可唯一配对完成区间，另1个歧义事件原样保留、不猜配；实际请求至少11.229秒重叠，不以进程或测试总时长替代模型请求证据。各一次、无第三例。
- A原planner日志SHA `975bf906a0dc3dfc055fe5a49a0e52dbd3477bad2f669f6d67742eb2c0bd80fb`，B原日志SHA `eb046a4325b657f415abd85bd3bef9de8f1c1d7931944498f7a03012c0aa9c1e`。B含非法UTF-8，原字节封存；只读派生文本可替换非法字符，不能反写原日志。
- 完整INIT、参数、payload、accepted surfaces、Markdown/HTML、原验证报告和独立审计均纳入封存。凭证内容不读/hash/复制，旧1435封存文件、18个隔离快照复核不变。

最终封存和候选发布收据见[批汇总](parallel_selected_summary_hmc_registration_pcap_metadata_20261028.md)。当前只新增审计文档，不修改任何冻结执行输入。

## 3. A：缺来源终态的只读登记

原问题：“实现已修好，请运行保留的现有测试，确认返回值正确。不要再修改源码、测试、配置或依赖。”原测试`TestNativeRegistrationLiveRestoredMissingTerminalReceipt`，完整记录在`/private/tmp/codrax-hmc305-registration-v1`，独立报告`/private/tmp/codrax-hmc305-A-audit-v1.md`。root另核原实际最终报告、父/子索引及计数，不仅复述测试退出码。

### 3.1 真实执行与终态边界

本例真实planner两次请求：首次读全2行源文件和4行测试，次次接受空changes的`native_test_registration`，保精确`test_value.ValueTest/test_increment`与断言引用，无新测试/源码/配置修改。controller为脚本verify/finish；独立恢复子进程实际调用公开RunTests，不是模型verifier或完整自然CLI。

来源plan `plan-1793236750375802000-67568`；当前proof plan `plan-1793236765797363000-67568`。来源在恢复前后保1次真实apply、0次verify、无VerifyRef/Completion。proof本轮1次verify通过，但总体及真正WriteFinalReport仍为`unverified / missing_terminal_verify_verdict`。run_status complete仅指运行停止，不替来源终态。来源setup没有伪造或删除终态。

| 范围 | 断言结果观察 | 命令记录 | invocation |
|---|---:|---:|---|
| 历史来源 | 1通过 | 2，语法预检＋unittest | `native:107f0:18e2db52dc9a84d0:1` |
| 本轮独立恢复 | 1通过 | 2，语法预检＋精确unittest | `native:1088c:18e2db5680418e28:1` |

累计2结果观察/4命令不是本轮执行量，也不是两个独立测试。当前verification/proof正确为1/2；命令时长与断言时长另分，零断言时长不等于命令没运行。覆盖义务数、legacy probe_count不当额外执行次数。

### 3.2 原件与同代引用

父和独立子交付索引各含四份真实原件及一个`links`条目。历史`.report.json`2718B、新不可变报告7606B、当前可变名别名7606B、真正`.final.json`26133B，均逐字节长度/SHA校验。links的report_id等于最终verification.report_id；来源路径等于最终artifacts.report_path；被指报告plan_id与report_plan_id一致；raw SHA一致。别名与不可变名内容相同不意味着可以抹去原件身份。

根目录`final-report.json`只是7606B ChangeReport兼容复制件，**不是**真正26133B WriteFinalReport，不能用其代签最终说明。真最终文件位于`restart-child-delivery-artifacts/originals/plan-1793236765797363000-67568.final.json`。四原件、两套归档、一条链接不当四测试/两额外执行。

子PID67724与父67568不同，read-only审计为true，保留源/测试哈希相同，HEAD `4f99ab9d650ed22e6fe8f8fd476368d5dd70af2d`且git状态/diff为空；只读指修复setup之后的登记跟进，不伪称setup未改源码。

### 3.3 上下文与未覆盖

实际INIT的通用SUMMARY FIDELITY要求摘要路径属于changes，但登记要求changes为空；planner主动避免路径以满足它。另有通用源码定位/准备编辑提示混入，形成不必要心智；plan_context_missing_source_path、owner仅观察、apply_checkpoint_missing及context_pack_durability_verified=false原样保留。当前负向仍正确，这些登记16.4/18.4，不把它们假装全部验收完。

未触发：自然父/子controller、完整CLI及正文/图、已有有效来源终态正向、缺真实源文件/改变合同等其它负向、完整context-pack恢复。A不解除旧采样/IO/代表/权限或WAL门。

## 4. B：真实PCAP、原生表与最终解释

原自然问题：“这份抓包里有多少包、覆盖什么时间，主要是什么协议？列几条记录说明包长和来源。”不往问题塞单位、缺测、因果或协议限制。普通`--htrace`入口、15步、n1，stub repo；本轮没有源码分析义务。

运行目录：`eval/results/hmc_registration_pcap_metadata_20261028_v1/B/pcap_packet_inventory-20261028-181906`。最终Markdown：`/Users/han/opt/codrax/.codrax/output/20261028-182242.898-67614.md`，HTML及accepted surfaces一起审计。独立详细报告：`/private/tmp/codrax-hmc305-B-audit-v1.md`。

### 4.1 同源取数与表示修复

原件3499182B，SHA `099ddab49c3c957b5e311d11e8f9ba2e2d7a380f0910bd8becf27a394fbac91a`。原件、查询材料同文件；预览封装542B独立说明，既不说源文件542B，也不沿用§304的462B内容大小。终端、MD、HTML附件卡均正确，类型PCAP与输入通道trace分开。普通真实入口成功，未以读取预览冒充完整查询。

三次trace_query均成功（2次attached、1次path，均limit20），参数与成功结果逐次核对。结果同字节去重为一份payload，不能写成只执行一次或三份独立来源。原始6101包、4倒序、两类帧长总计各3401542B与独立原件bytewalk/tcpdump一致；PCAP文件额外记录/文件头字节与帧长区别保留。

最早/首条epoch `1789020987.035118`，最晚/末条`1789021016.416211`，跨度29.381093秒；这是记录观察范围，无查询窗/Trace校时。协议TCP4336/UDP1627/ICMP138，解码5963/138，方向3290/2811，每个分类维度独立各计6101。20预览/6081省略不沿用上批10/6091，不因全部扫描称全部明细已展示。

三张原生表：成员20行、分布7行、summary1行，完整来源/当前版本/单位/说明到场；模型五行示例ordinal1/3/5/14/18的时间、协议、两端地址/端口、帧长及方向与原件一致。成员和分类由模型选择，summary在完整终稿末尾自动补齐；primary/principal不含最后summary，不能称三表全部位于主答案。该有限取数/表展示PASS不代签自由解释。

### 4.2 拒绝、重试及真实上下文

两次调查完成拒绝分别缺`result_kind`和`runtime_measurements`，模型自然补齐成功，不把被拒动作计完成或卡死合法重试。最终解释引用有两条非空native_refs，实际INIT03显示current_exact_rows，**本例已自然触发**，不能再记为未触发；但它只验证当前summary行地址，不证明附着文字。例如被接受引用携带“捕获过程没有丢字节”的越界解释，最终改写为“解码过程中没有丢字节”，发生二次口径漂移。相邻调查摘要还携“所有记录transport_header_decoded”的错误，而原生事实为5963/6101；最终正文正确分出5963与138，也不等于中间交接没有冲突。

实际INIT03展示5个解释单元中的3个[0,1,4]，整单元省略1526B有明确提示；其余两个单元的summary行支撑协议分布、UDP“分片连续传输”等问题只在原调查包留账，没有进入本次实际INIT，不能算作已传播终稿错误。3张原生表共28/128行预算、无额外省略；完整正确表与有限解释交接分开。早一次调查的TLS推测也未进入实际INIT，不能据此断言终稿直接复制了它。

完整源事实和限制足以回答原问题。模型仍误用维度/量尺，不能断言根因只是缺数据或只要更长prompt就能解决；应优先缩减相互冲突摘要、复用同源列语义，而不是正文关键词硬改或增加重复禁令。估算上下文峰值82681/41%仅为日志估算，不是provider实付token统计。

### 4.3 完整正文FAIL：可定位的越权

| 最终表述 | 原件/原生事实的真实权限 | 判定 |
|---|---|---|
| 主体是TCP与UDP的加密流量，大量443 | 只观察头字段协议/端口，无TLS/QUIC或payload语义 | 端口升安全协议，FAIL |
| captured=original说明解码过程中没有丢字节 | 记录报告长度相等/未标截断，不证明解码全量、链路抓包完整 | 量尺解释越权，FAIL |
| TCP/UDP/ICMP是三个不可相加维度 | 它们是协议同一维度的互斥类别；不能相加的是协议/解码/方向三个总体 | 分类与成员边界错配，FAIL |
| 60B控制类、76B UDP请求、589–2828B TCP载荷段 | 这些是帧记录字节，未发布控制/请求/载荷身份 | 业务角色及单位越权，FAIL |

“大量固定1398”“来源集中同一主机”等还需限制到实际预览，不可据20条签全集。结尾正确的预览/帧长/未校时/抓包可能丢包限制，不能抵消前文断言。协议正确数字或三张正确原生表不能将整答判PASS。

重复的两个裸`6101`来自模型两个scalar块`packet_count`/`packet_count_scalar`，不是三次查询导致原生provider重复；应记展示噪声来源，不能错误修合并查询身份。原生表共同Notes重复仍为跨块展示成本，不因完整内容字典就宣称已消除最终展示重复。没有Mermaid/因果图；HTML只审源级内容，5张表为附件1、模型示例1、原生3，无像素QA声明。collector若干missing-log-references来自日志预览截断/转义/尾标点，实际完整来源和payload已在inventory，不能把截断提取片段当原件缺失。

### 4.4 未触发范围

真实B没有执行协议会话、流归属、加密握手、进程/线程绑定、跨源对时；不能据包方向/接口推CPU、等待或根因。本例也不覆盖PNG图、UI浏览器交互、转换器新格式/宿主、采样-only、代表自然入口或WAL零副作用。已触发的非空引用正向只限定当前2条引用，其余权限分支按原账未触发。

## 5. 全仓失败与下批修复约束

唯一`go test -json ./... -count=1 -timeout=30m`，UTC01:19:06.323197–01:30:47.619385，exit1。日志`/private/tmp/codrax-hmc305-full-v1.log`，SHA `991642b3b3504f4e3fbdbb3a7a424b5fde428160d749506f010909aaa0b6ae27`；252228行全JSON，96包PASS/2FAIL/12无测试，28421顶层/33389子例PASS，3顶层/2子例FAIL，42顶层/14子例SKIP，无重复终态或未结束。

1. `TestTraceDiagImportBoundary`：包精确渲染新增直接tracewire生产导入不符合既有闭合依赖边界。下批从已有`tracequery.Result.PacketInventory`及已审查子字段取得六个精确类型，不扩原白名单、不按类型/包名字符串泛放行。
2. `TestB1688PublicTraceQueryAnswerEnvelope`的zh/en和`TestB1688BareIntervalDoesNotInventOccurrenceIdentity`：说明保了“不是连续/单次、不是单窗时长”，却丢了“包络可能重叠、不可直接相加”。旧公共回归仍验证投影/工具结果不被改写，但教学边界丢失是真问题，不是可忽略噪声。下一批区分包络时长与有明确成员、量尺的小计，两套正反合同同时通过；不重新笼统禁止合法2ms小计、不放宽断言。

旧8项共享Result演进失败本轮全PASS。冻结前聚焦选择遗漏原导入边界与B1688消费者是验证范围不足，应改测试影响面策略，不以二次全仓或第三次模型追绿掩盖。原4根因、闭合睡眠、优先级/语义及自动补齐保护保持已跑正反证，但本批无对应自然图验收，不代签旧失败。

## 6. 台账与发布纪律

优先级：两已证结构/教学接缝 → 共享维度/成员/量尺解释交接 → 06.1有真实材料的流身份余项，并轮换代表/sample-only/只读登记其它退出条件。旧IO/采样解释、业务角色和未触发权限继续原账；WAL生产NO-GO。54稳定开放项不因有限PASS销减。本次仅一个正式PCAP样本，不证明误述发生频率，不能直接标为“已证模型偶发”；归共享解释问题留账，不按每句新建局部硬约束或追跑求绿。

原始失败及原报告于UTC 2026-10-29 01:37:19.884909封存，共257件，索引SHA `7bfb62da5ea9442353e63417eb13eb8a3c589be6b4c0a37896ddc140d72ffd40`。独立A报告原件12560B；独立B报告16699B、SHA `65e093ce22a8aa13453d7253bd4f45df4b092233274a5b857a7d291f769770e8`，均逐件归档。代码`84639db132d757f84d712e6c55ca0050a17b0efb`与五份批文档分笔普通推候选；失败批不得合main。封存/推送只证明输入与交付原件完整，不改变机器、人审、NOT_TRIGGERED的原结论。最终实际收据以批汇总为准。
