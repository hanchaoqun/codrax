# HMC325 两例人工审计：均FAIL

原问句、输入、步数和断言不变，每例只运行一次。完整能力新增0、子能力交付0、剩余52稳定项不减。全仓7顶层/38子例失败独立阻止批次通过；process exit0、机器、人工和完成权限不混算。详见[汇总](parallel_selected_summary_hmc_donghu_representative_native_handoff_20261030.md)。

后继注记（§327）：新代全仓通过和代码776c8683940a777ec99cfac94bc51ff2e5f42afa不回签本报告任何FAIL；§325人工与821原封存件均不变，不补旧代publication。§327 A/B完整人工均FAIL，B终态交付NOT_TRIGGERED，不能借§326 B PASS补签；新代552封存件与远端交付凭证只属于§327。见[§327独立审计](parallel_selected_summary_hmc_native_publication_compat_20261030_manual_audit.md)。

## A：真实105版元宝——FAIL，无最终答案

[完整逐原件独立审计](results/hmc_donghu_representative_native_handoff_20261030_v1/sealed-v1/0471-codrax-hmc325-A-audit-v1.md)，SHA `e9c0aae714e0f388a87c8020183532136408fd0935c15de86ca0799afdda5445`。真实完整ftrace63659046B，不是预览或合成输入；独立标尺为同SHA原件的历史oracle，未重跑或注入模型。

查询TID46571，3733.491839–3733.795890秒；header TID/TGID46571与marker payload PID2227分开。四完整账户依次activity1/login/activity2/home，总耗时59.695/46.600/111.487/103.315ms，自身耗时6.770/21.605/3.938/83.143ms，均在INIT03/06带精确owner/窗口/parent出现。self是实例墙钟减直接子区间并集，不是CPU时长或根因贡献。窄窗合法self0保独立scope，不覆盖完整账户。

全窗304.051ms＝Running174.935＋Runnable4.620＋S119.302＋D未标记桶3.610＋已标记IO1.584。D未标记不等于已证非IO；独立oracle没做IO细分也不能否定原生known flag/caller。login完整10等待/3.454ms而前8为2.522；activity2/home完整13等待/1.356而前8为.965；12个已标记IO共1.350ms，完整包络到3733.785715，不能截止前几段的.779678。全树和self分段都是有明确省略数的预览，总量在裁剪前计算；四账户完整不表示所有树节点/分段全部展示。

四次调查提交有两次拒绝/降级、两次受理。中间受理解释仍把IO全集套进预览时间、将另一线程41111的CheckIntervalIdle列为46571子阶段，另有75.3% Running说“几乎全CPU”、selector scope错配。最新6个解释单元实际仅unit0进最终INIT，1/6收据明确；不能声称其余错误都进入成文。主要事实正确不能给邻近线程关系增权。

本例无最终正文/表格/HTML/图。两次provider400/2013，smoke原判FAIL。每轮三INIT99152+118260+444807＝662219 bytes；messages662295+tools122507＝784802/800000，HARD后仍发请求。196438是token估计，不是实际provider计量；两轮第三INIT等长但SHA不同，不称完全同请求。大块包括event114516B、量测表69516B、状态65945B、业务账户35002B以及KnownFacts115004B。应统一整请求预算与精确同源复用，不以扩cap或事实截断冒完整。

三次read_file失败要求不存在的伪造6618后缀；另一次派生grep输出拒绝同时返回合法JSON分页导航。没有证据支持“正确原件不可访问”，不放松来源门。保存summary未含业务self、需长JSON导航的可用性另记后续，日志只展开前2000B不等于模型只收到2000B。

最终输出和图修补均未触发，不计PASS；本非因果问题没有强制新增唤醒链义务。完整事实进INIT只证明子能力进展，没有交付答案仍FAIL。

## B：代表选择自然入口——FAIL，已触发但规格错误

[完整逐原件独立审计](results/hmc_donghu_representative_native_handoff_20261030_v1/sealed-v1/0493-codrax-hmc325-B-audit-v1.md)，SHA `5c8246e1f01ca5faa0ee571f6399903bab08a7b2fec03286cb6842090c08acb9`。四SQLite拷贝来自两份夹具，分别保来源身份；同线程号/时间窗不证明跨设备同源。

原合同8计划/2组/每组2代表/4未选、thread101/202、[10.000,10.050)、resource_stack。实际选样形状正确，但view=window_stats。唯一原operator使用同次status返回精确index `0fb1f118…`与snapshot `ac4a5d52…`，exit1：cluster specification or full membership differs。不是未触发；附加资源查询不能回填改变选择身份。

四资源查询c101/c202/d101/d202分别3/1/1/0事件、8/3/3/0帧；c101分配512+256=768B、释放512B，c202分配128B/address8192，d101分配512B，d202是真正零匹配。未知/重复/缺口depth保原义；source_frames_complete不等于unwind_complete，全部真实展开质量unknown。全1位型signed−1是原始观测，不可擅称协议哨兵。原生五事件/十四帧和这些边界都在INIT及最终表中。

最终正文残留：把c202地址8192并入“其余均−1”；称协议定义哨兵；d101/window_stats一次成功说两次、c202/resource_stack两次成功说一次、c101两次都成功说“才成功”；四选中计划误称四份物理采集，实际只选c/d两份；按source depth列举被写成三层连续展开关系。事实充分仍误述，有限留账，不继续围绕一句话过拟合。

最后status snapshot15次尝试，之后3个真实query使INIT到18；这个代差合法。当前16查询记录＝8计划+8直接记录，12已尝试/4未尝试，18次尝试=16success+2empty，latest为11success/1empty/4未执行。未选a/b不写成量测零。上述进度错误是具体查询绑定错误，不是总数18非法。

七次调查提交五拒二受理；早期1280“分配总量”实际混加释放、释放malloc错误最终纠正为768/free，保原稿但不当最终残留。最新6解释只发1个且含旧释放/展开误述，与正确原生事实冲突。8/8选中视图行全发，7/8完整发布，不能写8/8全完整。

一份真实emit_answer_document受理；MD37531B、HTML2819371B，13表与原生数据对应，表格正确不能抵销正文。HTML为源/主文本/表审计，非浏览器像素QA；没有图/patch，不事后新增画图要求，也不计图验收通过。

## 保存和后继

原问题两例及原operator均未重跑。启动前空目录guard失败与恢复过程单列，不冒模型重试。实际完成请求A10/B27唯一配对，28歧义不强配，最大重叠14.512秒。821件失败证据已封存，后续修复不回签本代。优先修证据显式替换与总预算，代表规格及充分证据下解释越权继续留账；只读登记、sample-only、旧IO/权限覆盖未由本代关闭，WAL仍生产NO-GO。
