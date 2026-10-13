# §233 双例人工审计

日期：2026-10-12；冻结代码`9a997c96819b`。所有结论对应本次原件，不回写历史FAIL。真实例恰好2个并行、各1次。相关/确定性测试不替代模型验收。

## A. 压缩SQLite卡顿清单

机器PASS，最终数值和表格PASS，**完整过程人工FAIL**。原件为`eval/results/hmc_request_identity_terminal_20261012/trace_gzip_sqlite_jank_inventory-20261012-194305/`；实际日志`run-1.logs/codrax-20261012-194316-000-8228.log`。

成功路径：没有通过后缀猜格式，实际完成gzip全量解包校验、自包含SQLite只读快照、完整systrace导出和tracebundle准备。输入仍是1100字节`capture.transport`，没有修改源数据库、fixture或oracle，未调用外部trace_streamer。原生查询实际返回两条marker，emission分别1.041/1.161秒，payload原始区间为1020000000–1040000000ns与1100000000–1160000000ns，appid均27599，帧数2/4，耗时20/60ms。原生字段按trace时钟解释，没有时区偏移或按刷新率重算。

原生查询`event_search`回执：查询窗1.0–1.2秒，parsed_events窗内8个，全捕获15个；matched_total=emitted=2、enumeration_complete=true、scope_complete=true。完整payload同时保`start_ts/end_ts`的原始marker文本与规范化`start_ts_ns/end_ts_ns/reported_duration_ns`。finalizer获得完整两个成员和正确单位；本次正文及表格未再把规范化起止字段叫原始字段，没有错误换算公式，没有把业务重叠当根因。最终`.md`、primary、principal及answer-surfaces一致；无图，root-causes旁路为空且`trace_root_cause_contract_not_active`，符合非因果清单任务。

未通过项：

1. analyzer两次`emit_analysis`均把预览看到的2条填入用户声明数量，`enumeration_boundary={declared_count:2,source_quote:"共两条"}`，而原问题是“共有几条”，没有这个用户引文。第一次因intent/question_kind不兼容被拒，第二次修分类后仍保错误声明。预览观察数量不能变成用户规定的总体或完成条件。
2. 唯一实际`trace_query`除用户要求的`jank_frames>=2`外，另加`appid=27599`过滤。用户没有指定appid，analyzer甚至给出no_named_target。当前fixture恰好只有这一个应用，两行数值虽全对，受额外条件限制的完整回执不能独自证明用户所问的全应用集合完整。已观察字段值不应自动成为选择条件。

上述是结构化交接与查询范围问题，不为它们扫描用户/模型正文作关键词硬门，也不追加本题专用prompt。归01.3/16.4/18.4/18.3原任务，后继应区分用户选择、数据观察与查询覆盖，并用阈值不同于条数/多appid的独立oracle验证；本轮不改问题追第三例。真实最终数值通过如实保留，不将其扩大为泛化完整性通过，也不以这次输出改善宣称已证明模型波动。

## B. 修后只读测试登记与恢复终态

机器PASS，**限定路径人工PASS**。原件为`eval/results/hmc_request_identity_terminal_20261012/native_registration_restored_terminal/`，保留source验证、登记前后计划、child原始plan/report/workflow/audit、父进程恢复终态及`receipt.json`。

真实planner完整读取`value.py`两行与`test_value.py`四行，使用所给精确assertion_ref登记`increment-result`，changes/probes为空。来源批先经实际controller验证，再保存登记状态并由独立child恢复原生验证，不沿用旧执行当新证明。child PID10182与parent8147不同；fresh invocation=`native:27c6:18ddf6a346c04fa0:1`，区别旧`native:1fd3:18ddf6a014a33148:1`和source controller invocation。child原生断言PASS、登记收据有效，source/proof/整体工作流verified，原因`all_batches_verified`。

`readonly_checked=true`、`readonly=true`，git diff/status为空，HEAD与期望均为`508a5504d3e5e6151ff975a73c5248d4e2992bcf`；源及测试SHA与导出文件一致。child report与恢复后final report结构相同，receipt.json确实存在；这次不是在终态落盘断言前失败。实际终态测试14.45s、包23.341s，正式exit0。此结果验证§232认证只读补证不再因合法源码观察误开owner义务；旧缺来源终态等负控由确定性测试保留，不给历史失败倒签。

独立发现仍开放：child内存workflow的7个context pack有7个唯一ID，父进程载入持久化workflow后仍7包但只6个唯一ID，planner_observation与merged两不同包碰撞；批次引用数2/2/2变4/4/3。源码`internal/repl/write_workflow_run_store.go`每次Save用run/batch/当前PackID重新加前缀并前截120字节，重复保存非幂等，长共同前缀丢区分尾部。包和终态目前均保留，不能谎称本次证明丢失或终态失败；按ID读取/外置文件覆盖风险须以Save→Load→Save公开反例修复，挂18.4/18.5后继P1，不在本批无限扩轨。独审原件和hash见结果目录`verification-receipts/codrax-hmc233-registration-contextpack-audit-v1.md`。

## 计数与后继

完整父项新增0，仍21实现已交付、58开放；新发现沿现有ID交叉登记，不重复加数。当前两例完整人工1/2，历史五稳定验收父项03.2/04.2/08.3/08.4/18.2继续开放，不与本次FAIL机械相加。子能力交付和全仓/发布收据以统一账本§233末版记录为准。
