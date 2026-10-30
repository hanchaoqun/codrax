# HMC317：完整索引开放尾边界与 IO 在途有限验收

执行日：2026-10-30 UTC（宿主本地2026-10-29）；目录日期只是批标识。基线 `d1721c62c650aa1babbe92f5f861496c5e117c37`，持续在 main 工作；实现提交 `221fa35b4f368e891c2fc2a0885318b598fe0d40`，紧邻文档提交及普通推送以本批 publication 收据为准。

## 结论与核账

**79唯一任务＝26已交付＋53开放，18类；本批完整能力新增0、产品有界子能力1组。** 状态为18部分/29待实施/3待验收/2验收中/1持续，未改稳定ID或勾选。两例真实模型评测机器2/2、完整人工0/2，08.3继续验收中。实现和回归通过不是完整答案通过。

已交付子能力：完整非Windowed索引的状态切换汇总，在有原有来源资格时按物理工件末端限制开放尾，三个视图共用；不改变请求窗口、闭合睡眠实例或原根因身份。当前仍有4组验收问题：窗口化源末端交接、IO共享标题端点、业务解释、IO解释；它们分别留原16.4/18.2/12.5/08.3等ID，不是新增4项任务。裁尾自然分支未触发另记覆盖欠项。

## 参考代码与设计取舍

实际对照 `/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main` 的7件文件并冻结其字节：sleep/thread_sleep_summary、sched/thread_sched、io/io_block、io/io_latency四份指标YAML，core/preprocess/thread_sched_ops.py、io_ops.py，以及config/skills/io_analysis.yaml。

- 调度实现先取原件 `trace_range()`，未闭合duration用源末端，再与查询窗求交；借鉴的是源覆盖与请求窗分离，不是用查询终点补物理事实。
- 睡眠的完整统计与top细节、每层分母分开；本项目保31ms闭合S态和5.5ms开放尾独立计量，不将EOF变成已观测唤醒。
- 参考IO采用20ms桶计数/平均等路径，设计用于展示局部请求活动。它不等于瞬时峰值或按时间加权的深度。本项目继续已有精确半开区间sweep、独立发起数、每层/设备/读写分组，不迁移桶阈值作硬件瓶颈或根因判断。

## 公开复现与限定修复

原业务fixture末事件1.051；宽查询0.990–1.055仍是合法65ms请求窗。原window_stats/wakeup_chain/root_cause_rank共用state_churn把worker最后开放睡眠算到1.055：40.5＝31闭合＋9.5开放，总51ms。公开1顶层/3子例RED后，完整索引修为36.5＝31＋5.5、总47ms；原查询窗不改。

复核追加真实2.000晚事件，冷/暖窗口化索引LastTs仍为1.051。初稿把其误当EOF，公开1顶层/2子例再次RED，错误裁短本来合法的40.5ms。这证明全文件单调资格与所选索引末行不是同一权威。终版7行仅在 `!idx.Windowed` 时接既有helper，既不把旧错误扩到窗口化churn，也不把窗口化真EOF说成已修。

新测试6顶层/22子例覆盖三消费视图、五活动状态与流式对照、冷/暖晚端负控、原查询/行范围、无end标志/无size/unknown-order兼容以及T/t/X/Z不补尾。另有windowed真EOF只读characterization，打印未闭环差异，不把错值固定为长期正确断言。旧断言未放宽。

后续建议已封存为只读设计：复用每源私有身份账本和锚缓存，完整EOF/合法事件/时钟及代次资格和值原子交接；冷/暖/缓存/多源分别验证。CoveredLastTs是前缀顺序状态，合并排序也非子源EOF，不能直接升权。此结构本批未实施。

## 冻结、回归与实际运行

03:50:17.196856Z冻结7513仓库输入、124外部输入、16仓外快照。SHA256 `285b8db9f70820b80db7fc3b4b8c8b6e66ae5818e74b5f5424d799e086ca6696`。一次标准CLI构建（SHA256 `8dc06f9033adf8394ab6d02066e79b75fc5076a87c6b1715891a3740a4dcc395`），没有testbin或替代构建。冻结后源码、测试、注释、helper全部停写，仅审计/文档和正式结果新增。

| 验证 | 实际结果 |
|---|---|
| 最终相关保护 | 40顶层/60子例PASS，0FAIL/0SKIP |
| 同矩阵 race | 40/60 PASS，PID29074，exit0 |
| 原4排序/合法小计合同 | 9/55 PASS |
| 因果与旧交接保护 | 20/137 PASS |
| IO有限合同基线 | 15/33 PASS |
| 唯一全仓 | 98包PASS、12无测试；28560顶层/33864子例PASS；43顶层/14子例按原条件SKIP；0FAIL、0重复终态、0未终态 |

全仓PID29425，03:50:29.296263Z父spawn前时间，04:01:55.855226Z实际wait结束，exit0、signal null、冻结输入前后核验true；日志SHA256 `0d61ff845f6169a78129f4792cf3b3817c124519acb79b9a9ba6b226dcf074f4`。没有用日志PASS片段代替进程终态，没有第二次全仓。

| 原自然案例 | 真实shell PID/终态UTC | 机器 | 完整人工 |
|---|---|---|---|
| A trace_business_io_sequence | 29426 / 03:54:21.655370Z / exit0 | PASS | FAIL |
| B trace_io_inflight_overview | 29427 / 03:55:01.952474Z / exit0 | PASS | FAIL |

两例均原QUESTION、原EXPECT、原15步/n1，以同一标准CLI并行运行；每侧11请求/响应唯一配对、无歧义，最大可证真实请求重叠48.942秒，不以barrier或进程重叠冒证。两份附件均为已有authored exported-text，不冒充新实机采集；oracle只供操作员审计，未注入模型工作区。原失败不重跑追绿。

## 人工审计摘要

A四次query全成功，三范围型查询均0.999–1.051；2次complete一拒一受理，1份答案拒后1次ensure_visible patch受理。图保三业务/原生身份与两唤醒方向，无重复主体或反向箭头，Trace投影/自动补齐保留。原生36.5＝31闭合＋5.5开放正确，但query没有超过EOF，故新裁尾分支自然NOT_TRIGGERED。

A完整FAIL：图把1.040000请求完成与1.040010唤醒合成一点，漏worker1.041010实际调度点；跨线程两observed_root的时间包含被升为同步等待业务完成；有限已确认运行/就绪总体被外推整机不饱和。同步等待解释确在实际INIT，与typed边界冲突；CPU排除句仅在原reason、实际预览已截断，不捏造其到场。合法31/35、2ms、47背景与平台优先级语义均保留，不新增错误门。私有生命周期自然路径仍NOT_PROVEN。

B一原生查询，5次complete＝3截断拒绝＋1降级＋1接受；1答案受理、0patch。四组24值、8成员13段、独立发起数与完整配对数、四原生表均正确。完整FAIL因浅深度被解释成无队列/非瓶颈和正常业务流程，以及将窗后完成当成未纳入配对的原因；实际carry-out仍已计入accepted，carry-in不属窗内starts。原数据与边界充足，不改正确统计器来迎合错解释。

B实际只展示2/8解释单元（index0/2），current_exact_rows仅证明地址，不认证正文；其余6单元未到场，不能把未显示的D状态误句倒推为成文污染。新发现的系统缺口独立记账：四原生表标题说端点未知，而同表notes明确半开；resource/sampling有受信端点桥，IO raw没有，须由真实producer密封交接而非扫描notes或全raw默认半开。

详见[本批人工审计](parallel_selected_summary_hmc_tail_io_inflight_20261029_manual_audit.md)。HTML仅源码语义核验，不冒充浏览器像素QA。

## 留存、优先级与发布

旧18档5607件及394快照来源身份逐字节核验，18历史污染身份是394的子集，未恢复快照到仓库。本批290件按规范来源路径＋SHA封存；同内容不同来源不丢，调用内联声明与blob不合计为执行次数，原非法UTF-8保字节和坐标。源快照以.go.txt留仓外。凭据内容未读、未哈希、未复制。

- [封存索引](results/hmc_tail_io_inflight_20261029_v1/sealed-index-v1.json)，SHA256 `51c758ace53b34d17b1cbef431d3abd663d4520a05a7216d8985a05a1ff92f24`。
- [封存收据](results/hmc_tail_io_inflight_20261029_v1/sealed-receipt-v1.json)、[发布收据](results/hmc_tail_io_inflight_20261029_v1/publication-v1.json)。后者验证紧邻代码/文档提交、main与远端一致、工作区清洁、冻结源与归档同代；不替代人工验收。
- [下一批窗口化覆盖设计](results/hmc_tail_io_inflight_20261029_v1/sealed-v1/0290-codrax-hmc317-windowed-design-v1.md)只为后续方案，不是本批实现。

ROI顺序：先每源完整覆盖→窗口化timeline/churn，以及原生端点→共享标题两个已证系统缺口；完整能力槽轮换04.2原业务树合同，08.3/03.2解释失败留账而不无限重跑；随后图局部/完整提交的精确身份关系导航、代表选择、只读登记负控及具备新增真实材料的17.6。WAL未零副作用前不接生产，旧采样/IO/权限欠项不漏记。closure=none，79/26/53保持。
