# HMC355 — 两例完整人工审计与边界

2026-11-05，root独立人工审计。两真实原例各n1/15steps，机器2/2、完整人工0/2；完整能力新增0、子能力1、79唯一/29完整/50开放。进程退出、原case smoke、原生事实、自然分支和整答分别判断，详[主汇总/全50项ROI](parallel_selected_summary_hmc_context_row_reuse_io_binary_20261105.md)。

## A：原IO自然题，HMC-08.3仍验收中

原case SHA99390f376f8d1d20a09ca58f1607a761ec20f9c8266111f2c7a5246ac1923bbd，原authored events.systrace（非新用户实采），原QUESTION不加护栏；[逐项root审计](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-v1/0252-codrax-hmc355-A-audit-v1.md)。进程OS0、前后源true，原run-1.verdict PASS。

两个actual query（window_stats/event_search）发布2JSON；原请求窗[1,1.01)s，检索lookup扩展不是量测分母。四组按层/端点族/设备/操作独立，数值按峰/均/忙ms/面积request·ms/配对/发起：RQ8,0R=2/1.4/10/14/4/6，BIO8,0R=1/1/10/10/1/1，RQ8,0W=1/.6/6/6/2/2，RQ8,1R=1/.4/4/4/1/1。完整8成员、13段，0省略；全窗均值含idle，busy union，跨窗端点正确裁剪；family配对8/未配起点1/歧义1/抑制2与最终披露一致。独立原始整数oracle与expected一致。

最终331行Markdown全文及HTML main源/全部10表/原root-causes与answer-surfaces已审；表cell和MD digest一致。无SVG/canvas/img/mermaid图主体，不冒浏览器pixel QA或新图验收。四组24数与独立发起口径正确；RQ9端点事件小计合法、BIO1另列，不取消合法计算。

**原有限合同和整答FAIL：** 开头与结论用partial配对低在途深度断言设备无积压、请求快速返回，并以缺调度/D/IO-wait证据排除IO卡顿。无硬件队列/完整捕获/独立阻塞链支持；按组峰值不是设备全局峰。BIO/RQ不可加正确，但此处说成已证同一操作同样越权。尾部边界不能覆盖开头错误，“真实并发略高”及coverage1.00残留字节的解释也不成立。不是加新退出条件，也不为说明失败破坏原数值算法。

实际INIT99,301/6,405/175,527B，正确原生口径和非因果限定已给；默认0的D/iowait摘要在无调度材料时仍易误读，作为共享typed可用性跟进留账。原legacy reason无设备无排队断言，完整reason未逐字转发，不冒其直接污染。新完整行有6定义/0alias，因requested family为io_latency未走新的重复消费者；自然新引用NOT_TRIGGERED，不能写降本/幻觉改善。

## B：原真实sample-only二进制，事实PASS/整答FAIL

原case SHA09bae0831fa02a8b90f4605a0b6231fca82d90add25ec37144ec515de0269780，原30,062,585B真实PERFILE2，SHAda713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632；[逐项root审计](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-v1/0274-codrax-hmc355-B-audit-v1.md)。OS0/源前后true，原smoke PASS，实际模型看不到验收oracle。

原件→共享准备→原生工具被真实触发。7声明query与系统2补齐共9JSON，11实际声明与11参数blob不相加；12000全流/1393窗/164目标/259权重与独立reference parser一致。TID1609/TGID1514/generation1，CPU0/1各82记录，权重85/174；查询窗200ms与独立扩窗267、预览40/164和6/82、记录包络不混。

两个单帧root节点：内核0xffffff800b829d24未符号化、82/174；OnDataEvent_已符号化、82/85；原CPU坐标、独立分母、未知sample_kind/权重cycles/时钟assumed/缺调度与branch_stack跳过披露正确。没有多帧调用箭头、把权重当毫秒或running_threads=0当实测零。完整177行MD、全部5HTML表/sidecars保真，无浏览器像素/图渲染声明。两个独立query的总体质量块仍重复，不累计成328记录；原件raw与bundle/预览大小在附件头正确分开，正文称bundle原件的词汇债保留。

**整答FAIL：** 开头与后文仍称线程“先后跑过”两段代码。unknown样本状态只支持记录位置，不能确立on-CPU执行区间；之后正确的“不能判断on/off-CPU”与“没有根因”不使此正断言成立。Trace投影诚实链止/数据缺口，自动补齐继续存在，不反向改成邻近/背景根因。采样解释旧债留账，不追第三例。

## 真实拒绝、重试、投递与操作员配对纠正

必须按日志时间/调用ID/完整内容配对，不能按随机blob文件字母序：

1. log1102 call qxBmU5bBlzSSChEoWjEJ1BT4先提交6单元带native_refs，1105–1106明确拒绝；对应原params022ce0a5、result511356d5。坏引用/预览尾点单元未投INIT。
2. log1136 call amLBROQQT4DqTNqgyuLNcTOz后提交5单元无native_refs，1144–1145明确成功；对应params1c7af0f8、result83fb669a。自然重试成功，不是软停止或旧代次恢复。
3. actual INIT投成功后者units0/1/3，3/5整单元、1,322B省略。成功unit0自己说“两个CPU上跑过”同时注明unknown，所以该冲突真正进入成文；未投units2/4不冒直接污染。

操作审计曾误把按文件名字母序排列的第一个当时间第一，短暂报告疑似旧声明串代；已立即以以上原始日志与匹配result纠正，并在root审计/根决定保留。**没有已证串代产品bug，不虚增任务。** 只读扫描一次遇logger非法UTF-8字节，仅改检查用binary-read/display scrub，原件不改；不是目标测试失败。B新引用0定义/0alias，NOT_TRIGGERED。

## 封存、保护与下一步

唯一全仓98包28780顶层/34627子PASS、零FAIL；513件保原/closure=none：[seal](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-receipt-v1.json)、[完整索引](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-index-v1.json)、[根决定](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-v1/0296-codrax-hmc355-acceptance-v1.json)。两例A6/B8唯一模型请求、歧义0、10重叠对/最大29.446s。ARGV捕获不运行真实CLI，原runner缺答案FAIL保留，不误记实际模型。

已交付仅同次上下文精确完整行复用，公开正负绿但自然分支未触发；局部省599B未扣说明，不称全提示/token降幅。旧IO/采样/栈解释与新自然覆盖、完整输出/查询复用/预算、权限/图、只读B2–B6及更多真格式继续挂原ID。下一完整轨轮换只读/资源/真实二进制；缺陷轨看typed缺测/解释适用域和整组预算，不无限局部优化。保原4根因/闭合睡眠、Trace因果投影/自动补齐、链上业务、合法算术和重试；WAL生产NO-GO。
