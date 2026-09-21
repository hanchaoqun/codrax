# 309be8 固定双例人工审计（2026-09-21）

冻结构建 revision=`309be8dd13b5`，buildTime=`2026-09-21T12:36:03Z`。runner82068正式exit0，2并行×1，无第三例追绿；机器2/2、完整人工1/2。机器结果不回写为人工结果，旧FAIL不抵销。

| case | 机器 | 完整人工 | 耗时（runner/例内） | 结论 |
|---|---|---|---|---|
| patch_go_typo | PASS | PASS | 101/98秒 | 真实一行修复、原测试执行通过、原仓未改；不代表required合同绑定完成 |
| real_trace_c2_dstate_iowait | PASS | FAIL | 109/107秒 | 三段IO等待与0.635ms正确，但错误宣称没有进入D状态；确定系统教学/语义接缝另立P1 |

## Go：原生验证与证明权限分账

结果目录`eval/results/hmc_target_roster_recovery_crossmode_20260921/patch_go_typo-20260921-053645`。原仓仍为seed `700d76b26056bb7e1c979d27a0f8776460c4b0f9`，隔离工作树交付`4ade0185db5d52c0f9e28ef94390e2ef55ddb5e3`。只将main.go第25行retrun改为return，1增1删。main_test.go/go.mod/README.md与seed字节一致；测试SHA256=`7d92240e4daa7c9cbe0ad95b89e1d350e1dfe25eb891091c956f71a39265dfdf`。未改测试、未转整文件modify、无emit拒绝或修补重试。

真实工作树执行`go test -json ./...`退出0，唯一原生身份`codrax.example/patch_typo_fixture`/`TestGreet`通过；TestGreet内部循环覆盖空串、空白、codrax三个输入，不写成三个独立assertion。未另外执行go build/CLI。工具与controller末尾均收到当前PlanID、post_apply_verify、generated_at和完整suite/id。没有PTO/probe，三条behavior contract全部planning_only，最终hard=0/soft=0/planning=3；不是source-free放行，也不能代销B2–B6。最终交付明确尚在工作树、需用户合入。实际分析首轮not_applicable+省略targets通过，只覆盖非runtime兼容，未live命中空壳恢复。

## C2：机器漏检的状态口径冲突

结果目录`eval/results/hmc_target_roster_recovery_crossmode_20260921/real_trace_c2_dstate_iowait-20260921-053645`；主日志`run-1.logs/codrax-20260921-053657-000-87157.log`。首轮named_target（com.baidu.tieba-59566）、full_artifact、bounded_fact_set成功。三次探索查询后，系统windowless全工件window_stats补齐真实执行（日志1067–1068），没有被窄导航窗覆盖。最后输入1812/1817均提供完整三段0.635ms，最终首次成文通过、零修补。该有限清单问题没有强加因果投影；`.codrax/output/20260921-053832.299-87157.root-causes.json`为schema2空数组、trace_root_cause_contract_not_active，边界正确。

三段等待端点和时长均正确：34579.451701..34579.451839/0.138ms、34579.452934..34579.453081/0.147ms、34579.471372..34579.471722/0.350ms；kernel caller为sync_buffer_read_wi+0x60/0x11c[sysmgr.elf]。但正文用d_state_occurrences=0声称“未进入过不可中断等待（D状态）”。原始fixture `eval/fixtures/real_traces/donghu_tieba_frame.systrace`第90/225/2427行恰为这三次`prev_pid=59566 prev_state=D`，反证明确。

系统query将有iowait标记的D段归入独立io_wait桶，d_state=0只是另一个互斥桶为零，不能否定原生D。`internal/agent/answer_document_trace_principal_value_authority.go`现有教学却要求io_wait在d_state_occurrences=0时不得称D，实际最后输入1802投递、模型1906照此推理。探索阶段968已先行误解，所以不能把全部错误仅归于finalizer；应统一生产者到最后上下文的两层口径与原生状态来源。不得把所有io_wait改成D（S+IO与独立IO闭合等待需保留）、不得改计数/时长、不得加答案原文硬门。内部枚举泄漏另保留为表达债。

本轮窄恢复教学由公开RED/GREEN承担，live两例均未命中该修补分支。新D/IO矛盾列主账§95，先修系统误导，再跨异构验收；父账13/79、66开放不变。
