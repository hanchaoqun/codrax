# HMC320：独立人工审计与有限合同裁定

持续main，基线a7705a69528472e962cae976d35dc2082c713eea；实现d92ca379098e767822bcb12ea2629b14b7692db0。本文与[批汇总](parallel_selected_summary_hmc_business_representative_20261030.md)共用447件不可改写原件。两例各一次、两独立审计代理及根复核；无第三模型或冻结后源码/测试/helper/输入修改。

## 1. 裁定

| 侧 | 进程 | 原机器/结构 | 原有限完整人工 | 稳定账本 |
| --- | --- | --- | --- | --- |
| A原业务树题 | exit0 | PASS | PASS，保非阻断措辞及共享教学欠项 | 仅04.2关闭 |
| B原代表选择题 | exit0 | 结构audit实际exit1/FAIL | FAIL | 11.2不关闭 |

79唯一由26交付/53开放变27交付/52开放；完整能力新增1、共享有界子能力2组。不是两例完全无欠项，更不是所有真实采集已验收。过去失败的原判定不改，新Donghu两例NOT_RUN；HTML仅源码审计。

完整独立报告：[A](results/hmc_business_representative_20261030_v1/sealed-v1/0135-codrax-hmc320-A-audit-v1.md)，SHA9b3ba2379187ec69d970ab8a9dc1b041f04ad1b7d707f9e79dd353c413730489；[B](results/hmc_business_representative_20261030_v1/sealed-v1/0155-codrax-hmc320-B-audit-v1.md)，SHA6f623d031854bf4d6fefdcec8edc64f2a3390378ae3cfae4315ef341a1ee4b05。根读取两份全文并独立核对实际事实/成文/代码与原任务，有限[裁定](results/hmc_business_representative_20261030_v1/sealed-v1/0421-codrax-hmc320-root-acceptance-v1.md)不由报告关键词、进程exit或机器正则自动生成。

## 2. A：业务树原合同闭环

原case SHA4ed47ac577b213ded9ffce3f2ed3962beea4710420bf90c0cc292744b21a3be3，15步/n1；原authored文本1481B/16行，SHA4d6956cb2befb4bad4124a122d2c6c3433fd9ff363887f6601cd8e113eba71aa。原问题/EXPECT未改；不增加必须计算可选self总和、Async CPU交集或特定Mermaid语法的门。

实际11次模型trace_query均成功，加1次确定性补采；9份去重native payload不是执行数。emit_analysis一拒一受理，调查完成三次两拒一受理，emit_answer_document一次受理、零patch。图标签soft advisory不是图失败。归档包含三份实际logged INIT、完整请求/结果、被拒稿及受理稿。

| 事实维度 | 原件与最终结果 |
| --- | --- |
| 同步树，business-700/TGID600 | OpenDocument→LoadIndex→DecodeRecord，两条contain；无call/wakeup/root-cause升权 |
| inclusive墙钟 | 10/5/1ms，父含子，不加为16ms |
| self墙钟 | 5/4/1ms，直接子区间并集扣除；合法互斥，不再称父self与子层重叠 |
| self状态 | 父running5；LoadIndex running3.5/runnable0.2/sleep0.3；子running1ms，分段精确 |
| AsyncPrefetch | S/F、business-700、cookie41、7.5ms，独立于同步树，不借名称归给900 |
| background-900同名实例 | inclusive=self5.5ms、调度不可计量；不合并、不中断已知self、不补零 |
| 线程全窗 | 12ms内running11.5/runnable0.2/sleep0.3；唤醒5.0033与上CPU5.0035分别保留 |

新分支均在实际INIT自然出现：INIT03 L453–465保四查询×三target节点＝12条发布，明确不是12物理实例；L467–473保三个不同query的background span→tree精确账户，每个self5.5/调度未知，独立16条预算，0省略，不升目标/因果/完成权限。未因此宣称所有伪造、过期和预算负控自然触发。

最终MD完整238行、21911B，SHAe139c1f92e027b0743715b83331734ce4e3b0fdd22627a87312b0b2de5ed54fc；HTML SHAe00eca52abd30afa95b17de6fff0706050233d7c08607398b514f541dd8743d7。正文/自写表/原生表分清inclusive与self；图三业务名、两contain及精确原生身份一致。8张HTML/MD表逐格一致，一处Mermaid源码；未做浏览器像素QA。附件原件1481B与预览1571B分开。

保留而不作为新04.2退出门的两处措辞：MD19说“这三个关系”而实际三节点两边；MD40 Async段说“线程实际占用了多少CPU”太宽，附近表/INIT实际指异步工作自身CPU归属未计量，business-700调度事实保留。不得据此强迫新增7ms Async投影，也不把有歧义的句子称理想答案。原包含/身份/self合同满足，警告仍挂18.2。

## 3. B：代表选择原规格失败，正确局部事实保留

原问题：按线程/窗分组每组两代表，比较10.000–10.050秒线程101/202的分配、释放和调用位置并概括进度。原50步/n1、四个SQLite只读副本、8规格/2组/4选/4未选。未注入view词或强制工具顺序，原结构检查不改。

实际discover明确用window_stats，原合同是resource_stack。原公开audit取同次成功status（log1153请求/1165成功）的immutable saved_index、selection_snapshot和SHA，实际08:36:43.281373–43.936131 exit1，错误为cluster specification or full membership differs。没有成功JSON不能改记NOT_TRIGGERED，也不从latest拼接新状态救回旧规格。

该返回index SHA5860ddc2e22213b142bde49b2eb65657e8ed2e0644e30bf55c7e9ead1cfb079e，selection SHAed6d25001e9aadfdaa4d9f9945a9485872009dfe76ae8b78de8f7a652f7e2ed7，status结果SHA4225076b585c21e2b6e05e6316a10f55a2cd8d197b29c7ebed58e2e69b9a4fe4，结构失败日志SHAc55624fed5cc6ec45609f3f3a04df1e7fca8dc189a3522da0546282f1e250f22。详细路径在独立B报告及归档索引。

23次真实工具声明均有对应结果，其中8次trace_query全ok，4个已选window_stats＋4个独立resource_stack。调查完成四次三拒一受理（未知item、数组kind、空表row0无合法原生地址），最后保独立advisory分析，不把无native_refs等于已验真。实际INIT只刊4解释中的2个，省略1436B；7个选中view中6个完整显示rows+notes，不说全部解释都到场。最终一次成文受理、零patch。

status早于最后一次资源查询：当时11记录/7attempt/6success1empty；当前最终12记录/8attempt/7success1empty、4未选，实际INIT02明确支持。最后多一查询合法，不能因整体FAIL制造历史覆盖当前或错误重试问题。

| 独立资源查询 | 事件/源帧carrier | 正确事实 |
| --- | --- | --- |
| session-a/101 | 3/8 | 两申请512+256B，显式释放512B；保重复深度、缺层、歧义符号 |
| session-d/101 | 1/3 | 申请512B；0匹配释放，不证明整个采集无释放 |
| session-c/202 | 1/3 | 申请128B；保Background/background身份 |
| session-d/202 | 0/0 | 真实成功空，不借另一采集线程名，不等于not_executed |

5事件/14carrier是四独立查询口径，非14调用或跨采集同一人口。深度不当执行关系，end_ns不当释放/CPU时长，无泄漏/因果升权。MD25零释放句可能指实际总体未知，因缺限定只记适用域歧义，不单独升级确定数值FAIL。

明确剩余范围错配：MD12把分组检查窗统一写[10,10.05)。resource_stack确为半开，但window_stats EventCounts使用ts>TimeEnd才剔除，含10.050s端点；A/C各19个resource_stack载体包含源event_row_id5及其三帧。不能由同数值端点统一不同view合同；不是资源查询漏行，不要求改合法旧window_stats端点。挂16.4/18.2。

最终MD250行/31102B，SHAaa22259f0f55cd2256c40d4f691a51662e49d67bd609d4a9a0da4c82b8b58a10；HTML SHAff913847c55acff3649baca3269c49c104385dfac5d82340104da874972f7c98。13张表逐格一致，主源码无Mermaid/svg/canvas/iframe/img，原题不要求图，不能以无图判失败。root-causes为空且contract未激活，无归档旁路；源码审计不是像素QA。B未触发本批业务树新分支，由A/确定性测试各自负责。

## 4. 共享欠项、原件与后续

确定共享教学冲突来自A实际INIT01 L55/L60的绝对NO CROSS-ROW DURATION SUMS，与INIT03 L452的同源互斥self许可矛盾。这是多场景高ROI规则适用域问题；A本次答对不等于系统矛盾消失。A重复timeline/端点标题未知与B跨view端点统一问题同挂16.4/18.2；不扫最终原文作硬门，不用表格正确抵任意正文错误。

本批4组剩余为11.2规格失败、共享求和教学、窗口/端点与重复展示、自由解释适用域/措辞。旧IO/采样/权限、Windowed自然覆盖、§310扩大race、18.5负控反馈债均原样保留。下一轮优先共享合同/教学精确化，再轮换已登记Donghu真实ftrace和只读登记；WAL零副作用前生产NO-GO。

原始模型请求有可证14.129秒重叠；A歧义/未闭合事件、B非法UTF8均保原字节，不合成响应或猜请求配对。冻结及全仓实际终态见批汇总。447件封存索引SHAdf3e105aa72ce3666e30c3bf4adc9b72c72938ec5ebc7dccdc48eb601c762e7b，旧21档6728件/450快照身份保真。文档/提交推送与[publication](results/hmc_business_representative_20261030_v1/publication-v1.json)只验证交付完整性，不代人工判定，也不回签任何失败或未触发。
