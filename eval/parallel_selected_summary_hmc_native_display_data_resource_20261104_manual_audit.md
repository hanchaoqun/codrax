# HMC351 完整人工审计与未销账项

主审root operator，原两例各n1/15steps，无第三例/第二full。源码/测试/教学/helper/case/oracle冻结后停写；源码子能力1组，完整新增0、79/28/51不变。两个原smoke PASS，完整人工0/2；03.2有限FAIL，自然输出复用目标FAIL/私有资格触发未证，不误销账。冻结/测试/全51项ROI见[汇总](parallel_selected_summary_hmc_native_display_data_resource_20261104.md)。

## A 元宝：量测保真，完整端点和重复显示仍未闭环

[完整root手审](results/hmc_native_display_data_resource_20261104_v1/sealed-v1/0380-codrax-hmc351-A-audit-v1.md)、[128节点源对照](results/hmc_native_display_data_resource_20261104_v1/sealed-v1/0401-codrax-hmc351-A-native-source-audit-v1.json)、[2211时序显示对照](results/hmc_native_display_data_resource_20261104_v1/sealed-v1/0402-codrax-hmc351-A-timeline-display-audit-v1.json)保原。完整63659046B/412546行用户原件SHAbe59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c，未裁小片替代。新整数B/E oracle2316闭合/1未配E/0payload冲突/0EOF开放，独立标尺未喂模型，不授调度或因果权。Header TID/TGID46571、marker PID2227分开。

四查询共128原生业务行（非128唯一物理实例）的名称/完整实际端点和行/闭合/直子数量/裁窗inclusive/扣完整直子并集的self全部匹配。两activityStart59.695/6.770和111.487/3.938ms，登录页performCreate46.600/21.605和首页103.315/83.143ms均正确。最后四时序表217+836+322+836＝2211行，和预冻整件公开查询保留的原生行逐身份/状态/实际端点/裁窗值比较0差异；该对照不是独立调度oracle。不能把128行都称模型已完整看到。

三实际INIT98913/49606/327256B，共475775B；17原生JSON保留。实际INIT03中unit0一面称两实例闭合、一面说#2真实尾部可能略超窗，native_reference_status=no_native_row_address。最终正文据右边界重合断言#2真实尾部超窗；但原件183577行E|2227已经精确闭合于3733.795890，超到3733.863427的是父事务，不能从父借尾部。第一父事务3733.491746..3733.615757完整为124.011ms，答案把窗口内123.918当完整；第二父事务窗口内self为已测0却称未验证。对登录页等待的解释也混入更宽父实例11段/3.485ms，而其self D3.302ms等不能互换。

因此存在**实际受理解释与原生端点冲突**，不是猜未转发reason污染。其它提交中的self=total/下层完整覆盖、fusion_page_load错误解释未找到完整进入这些INIT，不声称全部坏单元被传。已有正确native事实和后来系统说明不等于正常正文已经绑定正确。

最终MD406934B/SHAa5d129ee20d08415bfdbc6a67b14228563ac193ba74347163784c46beac904bd，HTML3390890B/SHA97ef57a262b4fed27421d7b9f400f6a59bd9a5bc9079ab5d81b3cbbfd03007ec。11表含标题行数2/10/7/7/14/218/837/323/14/14/837，两个836行表完全相同，数据体SHA5e181f75c4b0be6bd4b0ca5f1f9d622919ce9b2065ee6e12c0f852393ce69549，未出现新“数据表N”引用。**自然复用目标FAIL，私有资格分支触发未证**；不能仅凭格式行相同放宽当前源/target/window/native精度资格。零模型完整双源接缝证明新有界能力，不签此自然分支或全输出预算PASS。

85工具声明/85参数全可解、11调查完成提交/1答案提交分别登记，不说所有尝试都受理；原日志9条非UTF8行保原。93缺引用候选含截断/ellipsis，不冒93真实原件不可读。HTML/MD源级表一致，无Mermaid/图或像素QA；不新增图门。

## B 原资源身份：表与合法差值正确，有限解释仍FAIL

[完整root手审](results/hmc_native_display_data_resource_20261104_v1/sealed-v1/0404-codrax-hmc351-B-audit-v1.md)保原。输入仅原2503B authored export-text events.systrace，SHA841d772de200860d1a13ddbae65912995fa39e9043438671ac514393c0af5dfb；不是raw binary/DB验收，expert父case/README/oracle未供模型。

0.0005..0.0055内5操作和5快照正确，窗外0.006 FD未混入。精确9007199254740993↔0x0020000000000001、0↔0x0000000000000000、−1↔0xffffffffffffffff、真实null、INT64_MIN↔0x8000000000000000均保留，子类7名称缓存|纹理、0真实空串、8真实null、99未发布、真实null subtype分开。数量大整数无浮点损失；未知栈键不升函数。Heap8192→4096→4128与Mmap4096→8192为独立累计序列，合法差值−4096/+32/+4096正确、不相加快照、不混10标记/5操作。

较旧失败改善：最终明确计数与操作数字吻合只是时序一致，不证释放/增长物理因果。仍以“多个字段本身是缺失或哨兵形态，不能当作真实资源身份”笼统涵盖已知大值/0/−1：原始身份值与已验证有效分配身份不是同一资格。后文不证明有效/成功/失败的正确边界，不能修复前句的混称。按既有有限合同，**03.2仍FAIL**；不添加图、完整栈、泄漏、实机二进制或WAL门。不能把正确表说成取数失败，也不以正确表签整答PASS。

MD21217B/SHA31db42c9a706f7edd3582403b8ee2bac349086f824a4e787acaeaabc74b5158e，HTML2814574B/SHAef7b04d49b3d791926de73c5bc339353bc81aefd1f154241d321678d150e1671。表含头2/6/6/3/2/2/2，资源值已在两surface手核；通用cell comparator因转义false仅为格式诊断，不冒语义坏值。无图/Mermaid或像素QA，资源栈unavailable诚实保留，非本有限任务的新阻塞条件。

## 新P0：数字与回退/缺测限定在共享交接中分家

[原生→实际INIT的root对照](results/hmc_native_display_data_resource_20261104_v1/sealed-v1/0406-codrax-hmc351-B-fallback-handoff-audit-v1.json)SHA6aa8739a2edd4155cd10e2a50c8c8004a47c9a769f495ffc70c093bf0f25184f。window_stats原件3619c6f0…和root_cause_rank7befe7e6…均有CPU1 frequency_known=false，caveat说明无cpu_frequency样本、运行时间回退权重1.0。数值ratio1，summary loss0.000；被省略的loss字段不是另一个实测零。

实际INIT115901+8426+172884＝297211B，只有第三体存在ratio1.000/loss0.000（字符118568/118587），三个体均未给原no-frequency/fallback限定或frequency_known。源码internal/tool/trace_query.go:14742的支持ObservationRecord搬Summary/七数值Notes，不搬per-CPU可用性或balance caveats。最终正常正文于是断言供给无挤占、低频损失0；后附Trace投影仍正确标“频点未采集、不可量”。后附正确边界不是成文前上下文充分的证据，完整原件路径也不是等价的限定投递。**这是已证系统缺口，不全归模型波动。** 文本定位仅审计读证，不新增关键词硬门。

B受理legacy reason只汇总资源/状态六事实，无供给排除结论；不误判它造成该误述。8工具声明/8参数全可解（非16调用）、5原生JSON、1完成/1答案分别登记，1条非UTF8日志保原。8缺引用候选含缩略路径，不冒8不可访问原件。

## 后继与不误销账

新P0挂01.3/16.4/08.5/13.2/18.4：按typed来源/query/CPU域，把实测/模型/回退/未知和覆盖、单位随数值贯通原生/ledger/finalizer/展示；保合法估算和链上算力根因，不把所有估算隐藏或禁算。源已冻，本批不边验边修。自然显示资格、跨块整组预算、已闭端点到解释继续挂原任务；不放宽旧断言求绿。

恰好两例，唯一请求A14/B8、8歧义组排除、12已证重叠对/最大19.213s。旧采样/IO及权限、代表/图、二进制/只读正负自然覆盖未在本pair验，不签通过；随后轮换有材料完整能力，不让充分证据后的偶发措辞长期独占。无稳定项关闭，[root决定](results/hmc_native_display_data_resource_20261104_v1/sealed-v1/0427-codrax-hmc351-acceptance-v1.json)closure=none。原4根因、闭合睡眠、Trace投影、自动补齐、链上业务/语义线索及合法运算/重试保持，WAL生产NO-GO。
