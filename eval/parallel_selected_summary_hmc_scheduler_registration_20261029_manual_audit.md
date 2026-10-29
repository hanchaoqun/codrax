# HMC307：两例逐面人工审计

固定两例各一次，机器2/2 PASS；人工为A完整FAIL、B限定负向PASS。审计区分原生数据、模型解释、图和系统补充，不用正确数字/表格抵消错误正文或缺失的所求图。原始失败、实际INIT、提交参数与输出逐字节保留；HTML源码检查不是浏览器像素QA。

## A：OpenDocument自然时序问题——完整FAIL

原问题、人工编写ftrace与独立oracle不变，模型问题未加入系统防护条款。普通CLI `--htrace`、15步、一次运行；原件2312B、预览封装2408B正确分开。最终恢复稿与同源HTML均须作为交付面验收，不能只看最后一次提交意图。

### 有限通过

- 原生状态/唤醒/IO查询保50ms业务窗与52ms查询窗；终稿开头明确业务内主线程运行5ms，查询状态表为7ms，没有把原总体删掉。
- 31ms完成闭合等待与35ms请求驻留在最终正文分开；worker与main两段各1ms有合法2ms小计，不跨方向直接求和。
- 背景backup写请求47ms不升为目标根因，设备/扇区/读写与TID80/TGID2保真。LoadDocumentIndex保业务线索但不凭打点授代码因果。
- 本次Harmony优先级口径来自宿主`--htrace`所选入口：`cmd/attached_input.go`、`cmd/root.go`将来源传入，未显式给platform的早期查询已采用同规则。不能因`.systrace`后缀或fixture无设备声明就指控模型自行切换平台；这也不证明夹具来自真实Harmony设备。

### 未通过

1. **所求时序图未交付。** 初稿声明的原生主体ID与Note/边的中文别名不一致；随后替换已有边却误用只适用于新增语句的placement_ref，修补未暂存。最终图未通过当前证据核验，被恢复流程隔离；答案仍出现“见下一块”而无可用时序图，且明确degraded/未通过最终结构化校验。保留安全隔离是正确边界，但不等于用户的图要求完成。
2. **同源闭合等待重复根因席。** 系统投影E5/E6均列document-worker的31ms提交线程IO等待，IO方向2席、总有效归因64ms；正文却说唯一席。不同结果文件的完整board和rank1深比较一致，解析目标/查询窗/请求/闭合实例相同，不是两次独立IO，也不只是模型重复一句话。尾注“可重叠不可相加”不能掩盖身份重复。
3. **业务段与调度段混用。** 正文写1.004500进入LoadDocumentIndex并运行至1.009010“共5.010ms”，该差值实际4.510ms；5.010ms来自更早1.004000调度运行起点。业务区间内运行8ms与整个查询内9.5ms均可保留，但端点必须对应自身量尺。
4. **解释冲突仍在。** 主线程44ms睡眠并非全部发生在读完成之前（读1.040结束，主线程睡到1.045）；“2ms比31ms小两个数量级”不正确。可给优先级建议，但不能把错误比例和端点写成事实。其它无证的确定性排除仍按完整独审留账，正确尾注不回签。

### 重复身份的证据链

实际payload `676ac386…`、`a7ace14d…`、`d54ca862…`：同source_path、2312B、mod_ns1789703368930181422；target app-main/PID100/TGID100；窗0.999–1.051；board fingerprint2609fdd5；worker200/TGID100；block_rq/dev12,80/opR/sector923339752/len64；issue/complete行9/11、1.005/1.040；闭合睡眠行10→wakeup行12、1.009010–1.040010。rank1完整对象SHA`6c4f9e5e4778705e15b8ae2500e55cf6bb1c88bcbf442b14d0f4d08f732f209c`相同。

成文前实际INIT已有两个candidate，relation_member_ref同为`#1@1ff28327`但query row_identity不同。前两次原查询用PID100，另一查询用thread=app-main。源码只读路径为`trace_query.go:1076`保原selector→`trace_io_projection_identity.go:98`纳入发布key→`trace_causal_projection.go:4039/6200`将完整发布key用作IO实例身份→`candidate_compiler.go:367`按含EvidenceID的candidateID去重。私有generation未序列化，不声称从JSON复原内部键；运行事实与静态路径相互支持。原四项保护只用PID查询，未覆盖此同代别名序列；冻结后没有补测试、改断言或实现修复。

### 上下文及未触发

实际成文INIT、拒绝稿/修补参数及完整payload已留存。新SchedulerContextScope在3份根因payload及实际成文INIT中自然触发，37.5cpu_ms明确归于完整查询窗，不能说本批范围交接未触发。调查过程有错误reason不等于它一定进入最终答案，最终裁定以上述实际表面为准；5次调查完成提交中3次empty_selection拒绝、2次完成，仍未证明原生选择器恢复闭环。A自然运行并未验收PCAP准备预算、packet脚本入口、未知执行类别等分支；确定性正反测试与自然触发分列。

本次效率提示为wall360s、估算最大context187215 tokens/94%、finalizer拒绝2次、investigation_complete拒绝3次。这是实际估计/拒绝记录，不是provider精确计费，更不是已证明由新scope导致；不能据此增加硬门或自动删原件。后续按共享重复内容、结构化导航与恢复交接审计，不按自由正文关键词拒绝。

## B：缺来源终态只读登记——限定负向PASS

原`TestNativeRegistrationLiveRestoredMissingTerminalReceipt`，真实planner＋操作员公共来源构造＋脚本verify/finish控制器＋独立恢复进程公共run_tests。测试14.04秒；外层03:51:16.712071–03:51:35.583174包含屏障开销，二者不混写。不是自然controller、完整CLI、模型源码创作或所有框架验收。

真实planner两轮，先完整读取2行value.py与4行test_value.py，再提交空changes和1条当前断言登记。登记受理只说明无需代码改动，不说明测试已通过。child PID65501不同于parent65360，新invocation与历史不同，由独立恢复后的公共执行器产生。

| 层级 | 当前事实 | 人工裁定 |
| --- | --- | --- |
| 来源批 | 真实apply/ref；没有controller verify attempt、VerifyRef或Completion | 来源终态仍缺失，未删除历史PASS制造负例 |
| 新proof批 | 新1条断言结果、2命令（语法检查＋现存unittest），verified/tests_passed | 有限验证通过 |
| 整体 | run_status complete，但completion unverified/missing_terminal_verify_verdict | 正确保留负向边界，调度结束不等于整体验证通过 |
| 历史计数 | 旧同一断言1结果/2命令，连同新代累计2/4 | 不是2个独立测试，也不是本轮4命令 |

真正WriteFinalReport中的verification计数与当前报告一致；根目录`final-report.json`只是ChangeReport兼容快照，不能替代真正终态。父/子各留4 typed原件、1 verify引用及1真实index.links；大小/SHA、plan/report身份、final指向的不可变报告全部逐项核对，0差异。来源旧报告保持原字节，不被本轮覆盖。

子审计HEAD与预期相同、git diff/status空，源码与测试SHA和留存字节一致；只读阶段没有修改源码、测试、配置或依赖。这不否认操作员前置阶段实际完成来源修复。正式冻结输入也保持不变。

仍未验收：context_pack_durability_verified=false、apply_checkpoint_missing、来源定位observed_only、自然controller/fullCLI、多框架与可视图。一般源码规划教学仍在INIT，本例未诱发错误，不据此说所有教学冲突已经消除；没有普通MD/HTML最终回答或像素审计。该有限负向PASS不关闭18.5持续项、不回签旧FAIL。

## 原始证据与结论

独立原始审计为`/private/tmp/codrax-hmc307-A-audit-v1.md`、`/private/tmp/codrax-hmc307-B-audit-v1.md`；材料、失败修补、INIT及正式日志列入[封存索引](results/hmc_scheduler_registration_20261029_v1/sealed-index-v1.json)。执行、全仓、冻结和发布见[批汇总](parallel_selected_summary_hmc_scheduler_registration_20261029.md)。本批完整能力新增0，54开放不减；不追加第三例、不因候选推送授main准入，WAL生产NO-GO保持。
