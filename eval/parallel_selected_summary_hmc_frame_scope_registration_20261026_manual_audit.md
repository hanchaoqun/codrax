# HMC298 人工审计：帧范围隔离与只读登记

基线、实现和确定性证据见[批汇总](parallel_selected_summary_hmc_frame_scope_registration_20261026.md)。本页分开内容验收、自然分支触发和冻结一致性，不用一个绿色结果代替全部结论。

## A：真实Donghu帧窗口

原问题：只分析这份 trace，不分析代码。请分析 com.baidu.tieba 59566 主线程在 34579.472865s 到 34579.587805s 这一帧窗口内的卡顿原因。请分层说明直接阻塞/唤醒关系、链上主要原因、调度/资源背景，以及代表性时间窗。

使用原 `trace_query_donghu_real_frame_multicausal.case`，未塞入本批隔离规则；完整systrace SHA `f5d85dd9723d75c9121def4cbf20fc1ace8e2b6f35422cd794c9d8d68573d78d`。结果目录为 `results/hmc_frame_scope_registration_20261026_v1/A/trace_query_donghu_real_frame_multicausal-20261026-203241/`，principal、primary、transcript、answer-surfaces、完整日志和旁路均保原。

正确部分：114.940ms目标窗内Running26.946ms、Runnable3.636ms、S84.358ms、D/IO0；36次唤醒（CookieMonster34、Binder1、T7 1）和0.524ms Binder等待；ThreadPoolForeg60555→NetworkService60595→CookieMonster59843→target59566链上的优先级/供给信息仍在，没有被旧帧状态修复整体抹掉。原件中目标doFrame76795在34579.590312s，位于问题窗口之外；不能借它证明窗内帧边界。

完整答案判FAIL，三条可按原生行复核：

1. principal第56行将34579.475843–34579.484332绑为8.230ms等待。原生事件是475843–475857就绪0.014ms、475857–476102运行0.245ms、476102–484332睡眠8.230ms。不是小数舍入，而是开始点和状态错配。
2. principal第21行把4.262ms请求驻留称“设备驻留”。实际成文输入明确request_residence4.262ms、issuer_blocked4.256ms及量尺不可跨加；请求生命周期不等于设备实际服务或硬件等待。
3. principal第43行从总体就绪3.636ms少推出“很少被同核竞争直接挤掉”。状态占用不是抢占次数或机制；另一个局部CPU1区间确有1.743ms就绪、0.670ms已证低优先级重叠、1.073ms其余量，不能把局部凭证外推总体机制，也不能反称毫无竞争证据。

上下文责任边界：实际Finalizer INIT第1820–1822行给8.230ms为sleep_start_to_sched_wakeup；第2417行给精确连续端点，第2950行保独立CPU3运行0.245ms。原生时序表预览确实只有首1/169行，不能声称四行完整表已经投递；也不能说所需端点/状态全部缺失。IO量尺在INIT第2507行到场。CPU压力604.528 cpu·ms已经由现有原生摘要发布且有旧测试pin，本轮不把单位记作模型独创，若要改须独立审统计器语义。

上述是区间/量尺解释一致性问题，不据自由文本新增因果硬门，也不以整答失败否定正确主链、原生统计或Trace自动补齐。原始机器verdict、拒绝稿与恢复过程保原，机器结果不能代替人工逐项判断。

当前真实缺帧的局部边界自然命中且正确；历史另窗absent/flow→当前present/exact-contract的新冲突路径NOT_TRIGGERED，确定性公开回归不代签自然覆盖。实际7次模型原生查询＋2次自动补齐共9结果；无窗VSync包络只是独立检索，未被当作请求窗。最终HTML含同一错误正文与正确表，故非Markdown独有问题。完整MD另有系统因果投影/优化量text树，保链上、邻近、背景及不可相加边界；无模型Mermaid，HTML运行库中的SVG字样不计实际图，不签像素QA。三席root-causes为24.710、19.041、10.433ms，量尺和当前frame_unproven限定保留，不因旁路正确签整答。

## B：只读登记原有限合同

原问题：实现已修好，请运行保留的现有测试，确认返回值正确。不要再修改源码、测试、配置或依赖。

原 `TestNativeRegistrationLiveNaturalControllerTerminal`，BROOT `/tmp/codrax-hmc298-registration-v1`。自然父controller5次、planner2次、独立恢复子controller2次请求，均保实际日志；不是完整CLI。源码准备为operator公开工具流程，read exploration为脚本，执行器为public RunTests；恢复是明确设置的持久化边界，不冒充自然生产崩溃。

父planner实际读取2行value.py与4行原test_value.py，将既有assertion_ref登记到changes为空的计划。恢复前历史成功不当当前完成，恢复子PID93599真实发起新调用 `native:16d9f:18e2458993793cd8:1`；当前1条断言passed、2条命令，与旧三次invocation不同。readonly_checked/readonly均true，HEAD仍为保留源提交 `950567ec1f0c7328ce6df5961125b0965a959dae`，diff/status为空，源码和测试SHA未变。

预算仍max_batches5/used3/exploration1；源批保原验证，证明修复中间attempt仍unverified/verification_proof_incomplete，不改成历史passed；后继精确证明关闭依赖后整体verified/all_batches_verified。最终当前1测试/1通过/2命令，累计2贡献报告/2测试行/4命令分开。根目录final-report.json是ChangeReport兼容副本，真正WriteFinalReport为保留originals中的plan.final.json，不读错代。

父子各7原件/3条verify引用/2条final链接逐bytes/SHA匹配。五context-pack快照内容相同；活动merged首次保存ID归一化，后四artifact/reference map稳定，不能声称五次物理ID都相同。已清理的临时Store不伪称重新打开，只审现存快照记录。最终源码定位仍weak、materialization仍unavailable/apply_checkpoint_missing，不因本轮证明通过升强。

内容有限PASS，不代签完整自然源码创作、完整CLI、缺来源等负向、更宽18.4/18.5；54开放不减。子首次自由strategy有“三批已完成”措辞错误，原文保留，未据其升权；实际INIT、名册及最终三批状态正确。没有read Finalizer的MD/HTML/Mermaid产物，不签图形/像素QA。

## 冻结异常独立记FAIL

两例进程均exit0，但v1正式source_verified_after均false。唯一差异是冻结后测试注释+85字节；7377其余仓库输入、11外部输入及两个实际二进制未变。B离线checker因正式冻结前提失败而原样FAIL，没有追跑至绿。独立内容审计不修改该checker或原收据。

Go去注释AST相同及后继6顶层race通过只支持精确注释差异的后继验证，不倒签v1冻结一致，不改变A内容FAIL或未触发状态。早期实施报告误称注释在冻结前，独立drift报告用mtime与整文件重建纠正；两份原件均封存，不删除错误记录。

恰好两例各一次，实际模型重叠3.895秒；没有第三例、人工补查询或第二次全仓。原A/B独立审计、正式日志/失败收据、差异快照及AST证据全部进入封存索引，完整终态与发布见批汇总。

唯一全仓自身另有2个测试失败（95包PASS/2FAIL/12无测试），不是仅冻结检查失败：结构登记遗漏两种新消费者、系统补齐仍钉旧全局措辞。原168件封存后，仅迁移两测试，精确签名/SourceRef/索引核验及原跨工件负控保留，EN缺帧断言加强为局部范围。后继相关和race均3包100顶层/21子例PASS，生产及实际两模型二进制不变。原全仓exit1与冻结FAIL并列保留；后继不能代签最终全仓绿，也不改变A人工FAIL或自然未触发结论。
