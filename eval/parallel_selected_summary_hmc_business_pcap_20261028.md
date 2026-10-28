# HMC303：解释行引用与真实PCAP只读入口候选

日期：2026-10-28。候选`codex/hmc299-wait-interval-candidate`，基线`960a14d31786875d1304292c4d7255bab7317426`；main保持`788588e365d40a48b30827ffd8db08dae4e2ae9d`。**两例完整验收及全仓均失败，本批不进入main。**

## 1. 对账与交付口径

79个唯一稳定任务＝25已交付＋54开放，18类。开放分布19部分实施/29待实施/3待验收/2验收中/1持续；06.1因已有候选实现由待实施改部分实施，不减少开放数。

- 完整能力交付：本批新增0，累计25。
- 已实施子能力：候选2组——可选原生行引用、经典PCAP有界只读准备/查询/三表；尚未获main交付资格。
- 本批剩余验收：PCAP完整编排准入、两项公共结构合同、图身份及原子修补、非空行引用自然覆盖与成文一致性。它们挂原稳定项，不重复扩充任务总数。

旧采样、IO、代表选择、权限自然覆盖及其它54项保持原退出条件；未触发不是通过。WAL未满足零副作用，生产NO-GO。

## 2. 双轨实现与参考取舍

解释轨：新增可选`native_refs={observation_id,view,row_indices}`，引用当前已选原生表的原始零基行；私有封印覆盖完整表/Notes/来源/查询代次，Finalizer再绑定，JSON不能恢复。动态schema有界发布完整分支；引用、文字、限制和状态共用原900/1800预算。引用只证明地址，不证明文字、算术、因果或图注；旧独立解释仍兼容且不自动授信。新合同公开RED是旧入口不接受新字段，不冒称已经消除旧错误句。原业务范围事实此前已到场，不能重写成缺取数事故。

PCAP轨：按magic识别经典PCAP2.4，持有只读descriptor并复验源代次；固定4KiB解析前缀、最多64成员预览，完整扫描计数与省略分开。保包序、源偏移、epoch精确值、captured/original两种长度和原生方向；重复包不去重，未知不猜，坏尾保partial原件但不签完整成功，合法空捕获保零与未知时间。`packet_inventory`仅接受source/path/limit，不继承Trace线程/时间窗；三个原生事实视图复用既有展示凭据，不授流会话、线程、跨源时间或根因资格。

参考仓`core/extensions/tcpdump_parser.py`/`tcpdump_extender.py`解决包记录统一查询，但迭代后全量list、二次rows、DROP输入DB及文件名offset不适合本项目。取其原生记录事实化意图，不移植这些策略。原协议依据另外核对[libpcap头](https://raw.githubusercontent.com/the-tcpdump-group/libpcap/master/pcap/pcap.h)与[SLL/SLL2头](https://raw.githubusercontent.com/the-tcpdump-group/libpcap/master/pcap/sll.h)。

真实参考原件3,499,182B，SHA `099ddab49c3c957b5e311d11e8f9ba2e2d7a380f0910bd8becf27a394fbac91a`；独立Ruby遍历及系统tcpdump核6101包、TCP4336/UDP1627/ICMP138，epoch `1789020987.035118–1789021016.416211`、观察跨度29.381093秒、captured/original各3,401,542B。不是硬件带宽、捕获无丢包或Trace同钟证明；tcpdump首条payload28B与记录76B不混用。

## 3. 冻结前确定性验证

| 验证 | 结果 |
|---|---|
| 原生引用末版相关 | 3包51顶层/216子例PASS |
| 原生引用race | 7顶层/15子例PASS |
| PCAP末版相关race | 6包47顶层/274子例PASS |
| Trace/原四根因/闭合睡眠/显式窗与自动补齐保护 | 4包230顶层/365子例PASS |
| 标准CLI构建 | 一次成功，UTC07:30:27.930218–07:30:33.002395 |

PCAP公开原入口RED为合法二进制NUL被文本入口拒绝；早期测试错误consumer及后续真实canonical凭证不一致分别留原失败，末版才通过，不把所有施工错误当产品RED。1GiB稀疏记录试验只证解析器前缀有界，不外推完整CLI峰值。原生引用错误自由文字负控仍可通过advisory路径，不伪称语义被自动验证。

CLI SHA `79604c32a8b76c50602addc79baacc9f21972b62a54f560ecb35a0b1c03a8b47`。冻结UTC07:31:27.673269：7434仓库输入、63外部输入、30仓外快照；SHA `f18f0aaf99ddfcc9992b9f300d5a5ff8ef68d9e7bed456bb1a4db898ee05eebc`。之后源码、测试、注释、helper均停写，未重建。旧901封存及18隔离快照保持不变；凭证内容未读/hash/复制。

## 4. 恰好两例真实并行，各一次

| 侧 | 入口 | UTC外层开始→结束 | 机器 / 完整人工 |
|---|---|---|---|
| A | 原OpenDocument自然题，已有构造ftrace；15步 | 07:32:59.283506→07:39:28.208789 | FAIL degraded / FAIL |
| B | 真实PCAP普通--htrace自然题；15步 | 07:32:59.286175→07:33:14.817394 | FAIL read_exit:1 / FAIL |

两外层runner exit0只表示生成了判定，不能当PASS。实际请求唯一可配对A19/B1，A另1歧义区间排除、不猜；日志证明至少5.103秒跨例请求重叠，非进程重叠。未追第三例。

A正确区分业务50ms/查询52ms、主线程5/7ms运行、worker业务8ms及31ms闭合睡眠/35ms请求驻留；正确原生事实保留，但新非空native_refs未触发（接受的7单元均空引用）。图混绑worker两原生lane，两个identity conflict、三业务角色未证边界；修补删边后遗漏孤立Storage处置，最终整图隔离、答案degraded。正文还称原四席缺#3，与原生表及自身附录冲突；不能按数字局部改善销账。

B原件准备和分类正确，完整编排的旧文本准入随后以NUL拒绝；0原生查询/0Finalizer/无答案。局部测试手建Bus绕过`Orchestrator.Run`，未覆盖该门。命名材料有同类静态风险但本例未触发。原件字节未变不代表CLI没有正常日志/缓存副作用。详见[逐面人工审计](parallel_selected_summary_hmc_business_pcap_20261028_manual_audit.md)。

## 5. 唯一全仓、封存及候选发布

唯一`go test -json ./... -count=1 -timeout=30m`，UTC07:32:59.404725–07:44:21.069454，exit1：96包PASS/2FAIL/12无测试；28393顶层/33357子例PASS、2顶层FAIL，42顶层/14子例SKIP。251982行均JSON，无重复终态或未结束。

1. `TestEventSideTablePromotionBan`：透明转发引入pcapinput.Result依赖，严格源码类型检查未覆盖；不是实际字段提升被证明违规。去掉不必要转发比放宽原断言更小。
2. `TestTraceCapabilitiesPublicCompleteDiscovery`：packet_inventory缺metric_refs。应补真实输出字段、单位/epoch/总体或预览合同，不借调度指标或豁免目录要求。

日志SHA `950a5a20f017c24127532ffb231dbaaff10a636fb2bee5bc805411b50ba8007b`。三正式运行冻结前后true；失败发现后没有补改或第二次全仓。

本地结果根`eval/results/hmc_business_pcap_20261028_v1`；268份原始失败、完整日志/查询/实际INIT/MD/HTML/旁路、两个独立审计、执行helper/二进制及源码非Go后缀快照按canonical来源路径＋SHA封存。索引SHA `cd26a25404a73a5b1511ffd096b0407c4938d107df522d8490eb403d1653784a`。大体积原件保持本地忽略目录，不强行加入Git。`sealed-index-v1.json`/`sealed-receipt-v1.json`签原件完整性，`publication-v1.json`签分笔代码/文档提交、冻结内容、候选远端与clean，不签main或验收通过。

候选代码提交`2cca253dbb37e0a39630cdd15136dbf356d74561`，五份文档另笔提交；最终普通推送与远端完整性以`publication-v1.json`为准。保存失败候选不代表批准合入main。

## 6. 下一ROI

1. 06.1：受信材料查询域贯通附件/命名准备→真实编排准入→原生工具，补跨Run公共回归，保真正坏源和代次边界。
2. 同能力内补专用PCAP输出与指标/时钟合同，移除透明转发，保持现有两项结构断言；不得填写Trace时间/线程或伪造event_count。
3. 缺陷轨公开复现业务角色—原生线程lane—事件实例精确映射及孤立主体原子修补；未知TGID不凭同TID/名称合并。再验非空行引用自然覆盖，不删合法legacy解释强求触发。
4. 原4席正文、同表体重复7次、上游重复3ms/CPU范围/优先级排除误述继续16.4/16.5/18；旧采样/IO/代表/权限及WAL不代销，保持轮换避免局部追绿。
