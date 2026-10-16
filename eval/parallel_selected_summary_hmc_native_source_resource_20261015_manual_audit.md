# HMC249 完整人工审计

日期：2026-10-15。固定两个自然问题各一次并行；CLI正式退出均0，**完整人工0/2**。本文件是审计结论，不替代原日志或真实工具收据。历史HMC248结果不回写，不追加第三例追绿。

原始证据根：`eval/results/hmc_native_source_resource_20261015/`（本地忽略）。A=`representatives`，B=`catalog-resume`。两个独立逐行报告归档于`verification-receipts/codrax-hmc249-{A,B}-manual-audit-v1.md`；根代理另读实际路由、原参数、最终上下文、终稿与sidecar进行交叉核对。

## 共同输入与边界

构造SQLite原件的root SHA为`a938230f8ec8bceaafd8038d16058057f11407e8ce1846ee43f4e307f6d42bf5`，nested为`f912c23433825e0f25188ffef226c296d3515a2550d6a9be56cbc87880e4f03e`。A四物理路径分别复制两份原件，不等于四次实机采集；B两原件及operator构造checkpoint不冒称自然中断。完整数据与独立SQLite oracle均保存，oracle不在模型workspace。

请求原窗为`[10.000,10.050)`。root101有3事件/8帧，root202有1事件/3帧，nested101有1事件/3帧，nested202为空。root101释放事件原深度0、2；最后分配事件原深度0、0、1；右界10.050不计。相同TID、栈键或字节内容不授跨源同一身份；帧深度不授执行方向，资源事件不授CPU/等待/泄漏或响应根因。

## A：代表对照——NOT_TRIGGERED，整答FAIL

正式收据：PID5924，01:35:56.426101Z–01:42:12.368205Z，exit0。日志`representatives/logs/codrax-20261015-183556-000-5924.log`；终稿stem为`representatives/workspace/.codrax/output/20261015-184212.289-5924`，保MD、HTML、answer-surfaces和root-causes。

1. 初始规范路由仍为mixed/current_source=required，理由把“仓库内目录”当源码证据来源。新来源定义确实投递，不称工具缺失。accepted analysis已不用source_location解释调用位置，但检查进度仍发function_or_purpose，关系义务也有过度声明。
2. discover实际返回零计划及引用当前`queries.items` schema的plan先行导航，不是内嵌完整schema；模型随后直接探查，没有representatives/representative_status。没有选择快照，结构smoke为NOT_TRIGGERED，不造快照或按mtime找代用品。
3. 实际查询用了10..10.06，扩大了用户右界。4次无selector的process_profile被拒；timeline/event_search等额外探查允许存在，但不能替代用户指定的代表计划。两次resource_stack带pattern，底层明确unavailable/requires_time_and_owner_selectors_only，不能只看工具ok=true当原生取数成功。新资源固定事实在A没有产生。
4. 完成门两次降级分别为member_set和grounding，external-only waiver已经接受；不是上批citation-floor拒绝的原样重演。读取SQLite等错误选择也早于这些降级，不能把所有错误推给引用门。
5. 最终将10.050右界事件纳入10.000–10.050报告；101漏session-b；202错列session-b/c有数据、session-a/d空，而实际是a/c有、b/d空。名字当进程名，同号/同栈键跨源推共享路径，调用位置没有完整同源源帧证明。
6. `end_ns=null`被推成窗口结束仍在使用、-1被称动态分配；原值10025000000被写成1002500000。“全部Alloc都走栈7”又与自己的10.040栈9冲突。检查进度被替换成事件活跃/生命周期叙述，未给真实选中/未选覆盖。
7. 正文、表格与持久化主答案同样错误。系统补充范围/因果边界不能替它免责。没有模型图，不代签图修复；缺层/同深度原生复用也未由本例触发。系统跨源表保持未证关系，但极长内部路径和原始编码引用仍是16.5可读性债。

结论：11.2仍部分实施；自然计划/选择未触发。上下文中工具原生状态、请求范围与模型综合必须分层；本轮不能归为仅供齐完整resource_stack后的偶发措辞。

## B：恢复资源——本轮入口NOT_TRIGGERED，整答FAIL

正式收据：PID5999，01:35:57.579221Z–01:40:23.353931Z，exit0。日志`catalog-resume/logs/codrax-20261015-183557-000-5999.log`；终稿stem为`catalog-resume/workspace/.codrax/output/20261015-184023.254-5999`，同样保全部答案面。

1. checkpoint137行在explorer INIT被完整自动预读；没有经过read_file导航发布。模型只discover并将历史root101成功当作可略过对象，没有resume或catalog_query_ref。本次未触发不抹去上批11.1真实有界PASS，也不将自由成文问题追加为恢复能力退出门。
2. 只真实执行root202、nested101、nested202三个resource_stack，窗口准确10..10.05，最终2success+1empty，root101没有当前查询。原生payload分别为`2390c5dd`、`5973f254`、`976a7e68`，源/目标没有碰撞。
3. 请求旧operator结果的read_file被明确拒绝，没有返回旧内容。模型随即将nested101复述成root101；不是旧payload本身错源，也不是持久化ID碰撞。后续调查读三个当前payload并纠正root101为未查，但没有补执行。
4. actual finalizer完整收到三组原生事件/源帧、六个summary/members选择项，预览共9行、0省略。空结果观测名保持未知；源单位/原深度/未知展开/非因果边界明确。两个非空事件共6源帧完整到场；不能声称未取的root101三事件/8帧也已供齐。
5. 模型的空`native_fact_handoff.runtime_measurements=[]`被正确拒绝，之后删掉可选交接。最终emit/patch未选runtime_measurement，固定事实答案复用NOT_TRIGGERED，不能说新共享渲染在真实答案已通过。
6. 终稿承认root101未查，却开头称请求窗口共2次Alloc、Free均无；这是已查子集冒充全集。把未查写成“未在本次查询范围”且混合失败/未执行，不能准确反映用户目标。表中root101借nested的editor-main，nested空202借root的background；同TID不能证明跨源名字。
7. 正文与表格给出相反调用箭头，原生仅给depth；size被直接写为字节，signed=-1被断言为无效标记，均超出当前载体声明。资源时间没有被写成CPU或等待耗时，这是正确保留的边界；但不抵销漏查/错源。
8. 完整MD、HTML主体、answer-surfaces与root-causes均已读。无模型图；root-causes=[]、unavailable/trace_root_cause_contract_not_active，没有新增链上根因。大量内部路径与schema/runtime_work_relation术语仍进入面向用户答案，记录16.5，不伪称已解决。

## 系统结论与下一工作范围

新代码的确定性链路已验证，真实自然效果没有被证实。当前至少有两个可复核高ROI接缝：完整自动预读材料没有共享read_file的恢复导航；已接受runtime-only域后，Requested results/evidence origins/旧警告仍残留source_explanation或current_source口径。后者不证明全部错误由它造成，前者也不应靠扫描问句硬触发resume。

下一缺陷轨从精确任务/当前执行集合与默认同源事实展示入手；保自由工具组合，失败、未查、空与历史分开，不把所有可见预览当全集。不要再按单句错词加硬门。新能力轨与旧失败解耦推进，实际供齐事实后的模型错述保原件，不允许无限挤占其它高ROI能力，也未证明其只是偶发波动。

唯一全仓正式收据、6971输入/二进制/原件最终复核与本地归档计数见[双例摘要](parallel_selected_summary_hmc_native_source_resource_20261015.md)和中央§249；没有额外模型重验或失败原件覆盖。
