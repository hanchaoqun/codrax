# HMC306：两例独立人工审计汇总

范围：冻结候选的两例普通CLI各一次。自动检查2/2 PASS，完整人工0/2；原生取数/局部图表通过单独记录，不拼成完整PASS。审计使用实际日志INIT、原工具参数与返回、完整native payload、最终MD/HTML源、独立oracle。HTML源不是像素QA；没有第三次模型运行或冻结后修补。

## A：OpenDocument业务Trace

正式目录`results/hmc_business_pcap_flow_20261029_v1/A/trace_business_io_sequence-20261028-191415`。原构造trace 2312B/20行，不冒称真实设备新采集。4个成功原生查询分别为event_search、window_stats、wakeup_chain、root_cause_rank；3个实际Finalizer INIT，提交失败原件也保留。主文SHA`8c929eb3f1c6e5069def7904d830c589ac367f2bb3f9bd062be73a080322a555`。

### 有限通过

- 业务1.000–1.050秒=50ms与查询0.999–1.051秒=52ms分开；主线程7ms运行/1ms可运行/44ms睡眠明确属于查询窗。
- worker闭合睡眠1.009010–1.040010=31ms，有独立完成闭合凭证；同一读请求1.005–1.040=35ms是发起至完成驻留。最终未再当同一种量尺或相加，保链上IO候选而不升设备服务时间。
- 主线程与worker两个1ms可运行段，同板明确成员#2/#4的合法小计2ms保持；未加重复语义#3。定位包络可能重叠/时长不可相加说明不再否认这个小计。
- 最终图3个唯一线程lane，显示短业务标签、长ID只作机器锚。两箭头storage-irq-80→worker@1.040010、worker→main@1.045000正确，睡眠归wakee、唤醒点不等于运行起点；TID80/TGID2分开。图源码无裸中文续行/重复actor；持续区间注释延伸到1.0455不等于点事件倒序。
- 主线程原生状态表四行running2/S44/runnable1/running5及端点一致；后台请求未进入根因席位。附件原件/查询2312B、预览2408B准确，未把预览当文件大小。

### 失败与系统原因

1. **唤醒链被强化为同步工作义务**：正文“主线程同步等文档工作线程”“LoadDocumentIndex整段压在关键路径”，并称异步化能移走整段。原生只证明worker自身31ms闭合等待、两唤醒边，不单独证明main等待完整40ms工作或确定收益。实际INIT明确direct blocking未提供；accepted investigation的reason却把“两级同步等待”作为advisory重传。应修跨阶段摘要与精确权限的交接，不删除合法链上候选或靠关键词改文。
2. **包含关系倒置**：正文44ms睡眠“几乎全部落在31ms里”。真实44ms包含31ms，31/44约70.45%；端点早已到场，后文正确免责声明不撤销首句。
3. **原生属性复用错误**：后台47ms请求原trace和payload均为W，accepted摘要也是写；正文两处称块读。保住background不等于属性也通过。
4. **单段/全窗混合量尺——优先修复的系统缺口**：`SchedulerLatencyItem`的1.045–1.046秒段携带sameCPU busy7/idle45、otherCPU idle37.5，实际都来自52ms查询窗。`query.go:6744`起从stats.CPU建表，6807附近累加其它CPU整窗IdleMs，6833附近写入单段item；HighPriorityRunningMs/SystemOrKernelRunningMs同样整窗，已有`*_OverlapMs`及competitors才是片段交集。最终因此说“同一时段37.5ms空闲”并推绑核倾向。数值真实但适用范围错误，不能只归模型波动，也不能针对37.5改文案；应保全窗背景、另以精确片段凭证支持局部竞争/供给。
5. **背景统计排除主因过强**：running峰值1/mean0.317具partial/窗头未完整分类/采集完整性未证限制；runnable累计2cpu·ms不是执行CPU时间16.5cpu·ms，不能据此断言“不是CPU”。worker120→main53实际higher_priority_waker且closed_range_stable，IRQ优先级未知；未发现已证低优先级席位被删，但只以“缺锁/等待者”排除反转理由不完整，不能推广成系统定义。

### 图边界和最后修补终态

三业务角色仍保`unproven_boundaries=3`，已证局部原生边与完整请求关系未证可并存。不能因图局部正确擅删保守边界或放宽角色身份门；正文直接下同步关系结论才是冲突。最后一次`runtime_work_relation`补丁把`facet_ids`数组写成`{"item":...}`、claim_uses也错套item，返回not staged/live retry base unchanged；随后force-stop保上一个已受理草稿，机器仍PASS。不能把提交意图当交付，也不反改机器判定。JSON教学和恢复自然覆盖继续留账。

本例原生与局部图表有限PASS，整体FAIL；没有证明只读登记、PCAP、旧采样/IO其它分支或WAL。

## B：真实PCAP双向端点自然入口

正式目录`results/hmc_business_pcap_flow_20261029_v1/B/pcap_packet_flows-20261028-191415`。来源为参考仓真实capture原字节；原件/查询3499182B、预览680B在最终附件分别披露。

实际9次trace_query：8成功、1参数拒绝；产生8份native payload（4份flows、4份inventory），不是8次全部调用。inventory的limit6101超限被拒后自然以64成功，合法修正未被阻断。flows自然调用limit40/1/15/20，不用inventory成功替代新view触发。最终选择40组原生members和summary，非只看15组预览。

独立Ruby字节oracle全503组，系统tcpdump只读前12补证。6101记录=4336 TCP+1627 UDP+138 ICMP；5963归组/138未归组，captured/original帧字节各3401542B，原生epoch min1789020987.035118/max1789021016.416211，4条倒序。4份flows覆盖40唯一组，双方向包数、两类帧字节、截断、4096B解码前缀受限、各方向min/max逐字段零差异。有限原生取数/表格PASS，不代签整个答案或脚本入口。

### 正文与原生表冲突

| 正文说法 | 同源证据与裁定 |
|---|---|
| 第13组是“另一条TCP连接” | 仅端点组合，完整连接/会话没有协议闭合凭证；解释越权。 |
| 第13/15组持续到接近窗末 | 各止于1789020996.771048、1789020993.231525，整体观察末点1016.416211；错误，不能称连续持续。 |
| 第8组最早结束 | 第8止1011.079394，11/13/15等更早；错误比较。 |
| 第2/3/4组只出现在后半 | 第4起0997.768791，距全体最早约10.734秒，小于29.381秒观察跨度的一半；混用不同组范围。 |
| 第12组本机发出主导 | 第12 A=1.81.2.91，B=7.250.117.156；A→B49676B、B→A3166B，方向总结反转。 |
| 下表是所展示15组完整统计 | 原生表实际40组/80方向行，正文选择与受信表选择不一致。 |

独立SLL2记录中2811条outgoing的源、3290条incoming的目的均为7.250.117.156；“本机”可作为捕获接口侧推断，本审计不把它单独定为事实错误，但没有由此获得设备/进程归属。A/B排序仍不能替代cooked方向证据；第12组正是必须保方向的反例。前15逐方向数值正确、全体/省略数和尾注正确，均不能抵消上述正文错述。

早期非空native_refs曾被受理，但较新闭合提交省略handoff，最终三份INIT没有证明继承该引用，不能用早期接受态代签最终交接。成文保8个来源组、128/128预览行预算，无来源组丢弃；完整选择器确实渲染40组80行、1040单元格。模型第一次及最后追加facet补丁把数组错写成item对象而未staged；最终沿用第二次已受理答案，其原生选择器换为40组但保留lead，因此15/40冲突仍在。应修局部修改的事实绑定与教学，不把合法数组说成schema拒绝。

本例没有图；HTML只做源级审计。10张表包含附件1、方向明细1、flow summary4和inventory summary4，同源摘要重复及长来源版本仍是展示噪音，不把不同查询预览数相加。自然选择发生不等于所有新权限分支已覆盖，不能补签未运行分支。完整人工FAIL，原统计器不需要因正文错述被重写。

## 全仓和共享入口的独立审计

唯一全仓97包PASS/1FAIL/12无测试。原导入边界与B1688已按原断言PASS；新失败是outputdump既有640B预览预算，被新增双view长导航从同路径621B撑到759B。失败路径141B，当前固定开销618B，只余22B给路径，是教学膨胀变成准备硬失败；不应增预算、截路径或把源可读说成坏源。

静态读取另确认目录/执行适用域问题：脚本共享全目录接纳packet双view，V1走文本索引，V2在runStep之前即用Trace来源锁；indexed Run没有PCAP分支。真正分派在工具入口，现有真实结果渲染测试没有执行脚本入口。文本源选择packet后invalid Result仍可能被err=nil计成功是静态风险，本批未新跑验证，不冒称公开RED。应通过共享精确入口枚举/参数域先给诚实拒绝，再独立实现V1/V2受信packet适配；不扩白名单或扫错误字符串。

## 留账与证据

本批不销54稳定项；有限通过只登记子片。下一批优先共享预算兼容和片段/整窗范围交接，能力轨补目录各入口能力；轮换只读登记与sample-only/代表。旧采样、IO、栈关系和权限欠项继续原账，WAL生产NO-GO。

独立审计原件：`/private/tmp/codrax-hmc306-A-audit-v1.md`、`/private/tmp/codrax-hmc306-B-audit-v1.md`、`/private/tmp/codrax-hmc306-entry-audit-v1.md`；正式封存索引保留其完整来源映射与SHA。详见[批汇总](parallel_selected_summary_hmc_business_pcap_flow_20261029.md)与[封存索引](results/hmc_business_pcap_flow_20261029_v1/sealed-index-v1.json)。原始失败不被此摘要覆盖。
