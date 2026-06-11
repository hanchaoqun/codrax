# 残余 gap 修复(批 6 E 层收口,2026-06-11)

## 已修(本 commit)

### R1. idempotency 毒化粒度(data-planner 专项确认根因)
blocked plan 的**全部** action key 被毒化(`blockedIdempotencyKeysFromRecords`),正确的兄弟动作在修复 plan 里重提交被拒"repeats a previously blocked or failed workflow edge"——live 实证:正确的 Decimal 乘法脚本仅因与一个被拒动作同 plan 而被拒,逼 planner 走硬编码常数。**修**:violation 带 typed `ActionID` 归因时只毒化被点名动作;无归因(plan 形态错误)或归因不命中(stale ID)保持整 plan 毒化。双向测试钉死。验证:data_join PASS。

### R2. analyzer Rule 6 退化升级的 §1.6 typed escape(批 6 E2)
typo 对(retrun/return)+文件+函数结构性凑满 "2+ entities",mechanism 问题被硬升 complex,无视模型 simple@0.98 声明;读侧预算与写侧 planner soft cap(6→10)都被污染。**修**:`complexityEscalationConfidenceCeiling=0.9`——模型以 ≥0.9 置信声明 simple 时,Rule 6(噪声计数启发式)让位;低置信/moderate 声明照常升级;精确结构规则(sub-topic floor / is_cross_component)不受上限影响,typed is_cross_component=true 依旧无条件升级。四向测试钉死。验证:patch_go_typo PASS,本轮无 complexity 升级日志。

## 评估后缓修(落账)

- **E7 auto_low_risk 标签**:`ApprovalMode==ApprovalAutoLowRisk` 是 5+ 处自动执行门的行为谓词,改值需动门,LOW 项不值回归风险。正确修法在 display 层(messages.go 按 ReasonCode+risk 渲染),待 operation lane 下次迭代。
- **E6 Retry Directive 首发误标**:TaskState.RetryHint 契约明确"仅 self-loop 时设置,前向转换清除"——真因是某 scheduler producer 违反契约把 DAG node objective 塞进首发 RetryHint,需 scheduler 取证定位 producer,LOW 项独立小任务。
- **E3 write_controller 确定性转换**、**E4 op_* 计数器**、**E5 prompt 膨胀**、**E8/E9**:照批 6 账本,专项。

## data-planner 专项形态清单(更新至 6 种)
mapping 循环(已修 guard)/ custom_transform 死端 / 空 input_paths / 幻影字段(算术 op 已补)/ deferred-rank 循环 / **预算耗尽 reconcile missing(新)**。结构层修复后纯 planner 路径选择问题,idempotency 粒度修复进一步减少误拒;专项=prompt 引导强化 + 典型路径 scaffold。
