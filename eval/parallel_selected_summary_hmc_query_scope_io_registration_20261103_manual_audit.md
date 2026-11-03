# HMC340 人工审计：原有限IO FAIL、只读缺来源终态限定PASS

原问题/检查器/oracle未变；OS、smoke、原生值、自然分支触发、完整人工及源码发布分开。全仓失败阻断候选源码；79/28/51不变，完整新增0、子能力交付0。

## A：四组正确数值不豁免解释越权

原trace_io_inflight_overview，完整构造fixture1839B，15steps/n1，1次attached_trace/window_stats/[1,1.01)且无PID筛选，8工具声明与8参数blob不能相加计数。三次completion提交，两次结构修补后第三次受理，未追加查询；1次答案发射/无patch，15块含12原生表。原smoke PASS；08.3原有限/完整人工FAIL。

| family/device/op | 峰 | 均 | 忙ms | request·ms | 完整对 | 发起 |
|---|---:|---:|---:|---:|---:|---:|
| RQ/8,0/R | 2 | 1.4 | 10 | 14 | 4 | 6 |
| BIO/8,0/R | 1 | 1 | 10 | 10 | 1 | 1 |
| RQ/8,0/W | 1 | .6 | 6 | 6 | 2 | 2 |
| RQ/8,1/R | 1 | .4 | 4 | 4 | 1 | 1 |

原独立oracle/nativeJSON与4summary、4members、4activity表同源。8配对成员保真实起止/裁窗和物理行：RQ首.998–1.004/窗内4ms、末1.008–1.012/窗内2ms；BIO1–1.01/10ms；写2+4/另一设备4ms。4组24值、独立issue/端点计数及单位正确，不是计算器故障。IO参考桶统计不替本项目积分。

实际正文错误：

- lead称只有RQ8,0读持续占用，其余含BIO大部分空闲，与BIO10/10ms及自己表矛盾；写6/10ms也不是多数空闲。
- 受限、partial配对的在途数升设备queue depth，并断言没有排队堆积、队列拥塞必单调上升，驻留耗时主要是服务慢而非排队，几十KB排除带宽压力。源无硬件队列/服务分解/完整捕获，不支持这些排除。
- “4次在窗口内形成完整配对”混完整配对资格与窗内端点；首起点窗外/末完成窗外，正确成员表不修复错误句子。
- 由同IRQ完成线程推串行接手是形态直接来源，没有依赖/服务/唤醒关系证明。
- 无scheduler捕获却从iowait/D-state0断线程未进D，缺证误升否定结论。末尾“不能证因果”的正确边界不能豁免前面正向排除。

实际Finalizer三INIT99301/5470/170963B完整保原；正确native量尺和边界足够。受理handoff5解释单元，只有坏unit0整段540B实际投递、其它4省9072B；current_exact_rows只校地址。unit0已有“无堆积/基本空闲”，因此共享解释冲突确实进入上下文；未投unit2的issue口径错不能冒直接污染。无新关键词硬门/系统代改结论。

MD31335B/SHA916267b7e8b2c90088a715f4a1a3a64545a78e03a58f0fe15431664782cc63ba，HTML2811737B/SHA6302759547fffd426087c030b813b835b2b9ada1c5ab7b343cd38d0d52d616ea。13表含附件格诊断一致、旁路MD摘要匹配；无图/patch/SVG，不冒浏览器像素QA。raw log822538B/SHAdca0a1ed7f731ff53461db12d83ae7b48b1afd3938b3e146cc5d4c85f73d2bbe非整体UTF-8，原样保留，scrub诊断非替代原件。[完整A审计](results/hmc_query_scope_io_registration_20261103_v1/sealed-v1/0160-codrax-hmc340-A-audit-v1.md)。新多query同payload分支自然NOT_TRIGGERED。

## B：新proof成功不替来源补终态

原TestNativeRegistrationLiveRestoredMissingTerminalReceipt，真实MiniMax planner＋脚本verify/finish＋public原生测试执行器，OS0/原harness PASS、限定人工PASS；非自然controller/fullCLI、无最终模型正文/HTML，context-pack耐久未验证。

source plan1793696815009504000-25132真实apply至157a42bc38ceba3165aa29c3eaa191cafb696328，只有apply、无source verify_ref/completion/伪造attempt。注册plan1793696821779365000-25132不新改源码。独立子PID25288/parent25132，newnative:62c8:18e47dc1a1427010:1与旧native:622c:18e47dc0024f91f0:1分开，HEAD/空diff/status及sourceSHA833556db828032d49efc4c58a380411c53465c917da89bf613a607a10cee2ba6、testSHAec7d5c37376fa9c583ba402a196e94c9fa3d1c12c734195ccd18e8e7386a7864均准确。现有test_increment真的断言increment(4)==5。

1断言、2命令（1syntax/1project）、4proof义务与4工件各是独立人口。proof verified/tests_passed，但workflow与实际WriteFinalReport整体仍unverified/missing_terminal_verify_verdict，风险保留；materialization unavailable/apply_checkpoint_missing亦未伪签。父子各4原件/1final→report，全部8工件实核size/hash，reportSHA358f3f4270b454df50fc2d807da55646c133a492f9325b503fd577796eb18fa6、canonicaldc02ba74691a4242fa5ddda25d67fe797cecbbcac064bc1d69aec47a36c04cb0、finalSHA1712deb0138f8cbe87ed0e2894cb688923f0aafe056f92d46df8a3eea01f3b8d匹配。[完整B审计](results/hmc_query_scope_io_registration_20261103_v1/sealed-v1/0182-codrax-hmc340-B-audit-v1.md)。不关闭持续18.5或复制旧PASS。

## 冻结失败与未覆盖

两例恰好各n1，精确请求A9/B2、0歧义，最大真实重叠3.074秒。唯一full97包PASS/1FAIL，3顶层/1子FAIL；事件库存整提示136274B、Extract引用描述及来源导航135574B三处原合同失败，候选不交付，不提高预算/降低断言或第三例追绿。419原件及root closure=none封存；旧IO/采样/资源解释、代表/权限/图、更多二进制/协议欠项原ID保留，WAL生产NO-GO。
