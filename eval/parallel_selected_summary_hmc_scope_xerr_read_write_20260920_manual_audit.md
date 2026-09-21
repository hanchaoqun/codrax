# 5582 固定双例人工审计：卡顿清单与 Java 写计划

- date: 2026-09-21T06:35:33Z
- sweep_start_ts: 20260920-233533
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_scope_xerr_read_write_20260920

干净revision `5582d0fd03eb`，buildTime `2026-09-21T06:35:09Z`，snapshot `.codrax/tmp/codrax-selected-20260920-233533`；23:35:33–23:38:10恰好2并行×1，runner正式exit0。机器2/2，完整答案/计划人工0/2；正确数值和单行补丁分别保留，不因其它缺陷抹去，也不冒充整答通过。未改case/oracle或追加第三例追绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/hmc_scope_xerr_read_write_20260920/patch_java_typo-20260920-233533 | write_plan,write_patch_oracle | none | 99s | 28 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | FAIL | 单行patch正确；附带Java probe含非法import，通用类又会被系统误套main；另有5次planner拒绝 |
| 1 | trace_query_jank_field_inventory | PASS | eval/results/hmc_scope_xerr_read_write_20260920/trace_query_jank_field_inventory-20260920-233533 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 157s | 28 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 数值/成员/排序正确；marker PID被写成发射线程PID，内部状态词泄漏 |

## 1. 卡顿字段清单：数值通过，身份解释仍 FAIL

日志行号为本case的`run-1.logs.all.log`，正文为`run-1.primary.md`。

- 正文3–11：精确3条，7/4/2帧顺序和70/40/20ms正确；所有大于2^53的原始起止整数逐字保留，头行5.040/5.050/5.010正确。第11行排除同名子串和invalid数字，没有把缺测填零，也没有把B/E的10微秒生命周期当上报时长。
- 正文15将`writer-101`写成`tid=101,pid=201`。原始头`writer-101 (101)`提供发射TID/TGID=101，201是`B|201`内marker身份，不是该发射线程进程身份。appid与受阻线程不等同的大方向正确，不抵销这个身份串用。
- 正文17/25原样输出`time_domain_status=unverified`、`time_domain=trace_seconds`，虽附中文解释，仍未满足普通读者词汇目标。没有因此添加正文关键词硬门或系统改写模型句子。
- 实际成文输入2714已明确发射TID、marker PID、appid分离；2715–2717完整typed inventories分别保7/3/4条、各自查询条件与覆盖，字段为`emitter_tid=101/emitter_tgid=101/marker_pid=201`。2716精确筛选清单中3条大整数以字符串保真，缺字段和时钟边界都在，未被截断。2712要求普通语言而非内部状态token。因而本次身份/词汇错误不能记成事实未供给；三份清单和长来源重复属上下文成本观察，不是已证明错误原因，不反复加同义提示。
- 过程：初次analyzer把附加Trace事实查询误报not_applicable，756既有typed合同拒绝一次，792修成bounded_fact_set，保有限事实范围；不是bounded坐标清理新分支命中。三次trace_query中1913使用appid与jank_frames两个AND数值谓词，得到3条；另两次更宽查询均独立留存。预阶段的自由描述含错误成员/算术，最终2684明确由确定性查询替代5条预阶段导航观察。最终一次成文2872成功，零成文硬拒、零降级，无空答案或活跃流超时。
- 无Trace因果投影是正确范围：本例只请求字段清单及可行性边界，无调度/唤醒证据，也未请求链上根因选举。mandatory `.root-causes.json`确实生成：schema2、空数组、`trace_root_cause_contract_not_active`，不是文件丢失或生成失败；不构造根因满足presence指标。答案/HTML/收据均存在，无Mermaid图可据此签图解析能力。

输出`.codrax/output/20260920-233808.324-6233.*`。MD SHA `fb86488a0f81a6a0642fef8fd053f6e478b19f5e1b4e71d353483f607bc28d63`，root JSON SHA `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39`，primary/principal相同，SHA `42a075d17868d93db8595a08a19299150efdde4c252aad7b772f258398723253`，与收据绑定一致。runner157秒、进程wall154秒分列，不混口径。

## 2. Java plan-only：补丁通过，附带验证计划 FAIL

- `patch_java_typo.case` MODE=plan；runner通过`--mode=write --write-phase=plan`运行，未执行apply/verify。最终plan7–18只有`Main.java:16`一行retrun→return，统一diff精准；原fixture和eval repo字节相同，SHA均`94fa46db157256a710be1baa9f5a1f45cbb7d7ea00178378e92fa435338633e4`。plan94仍pending_approval。
- `run-1.out:60`为“提议的ChangePlan”，72–76为“验收测试”；“javac成功/Main.class/java输出”是目标，不是已经执行的收据，不能误判成虚报执行，也不能反过来作为已编译证据。计划SHA `dc6aa591969c31f25552e02919639a9ccfe31c706a25844a33de291802e295ef`。
- 实际6次emit、5拒绝：日志1090为同路径重复，1116/1142/1431/1463为外部命令包装器没有changed-class源耦合，1502最终接受；中间有一次重新派发。`finalizer_rejects=0`只计读成文，不代表planner零拒绝。既有实际提示已说明纯语法修复优先省略probe，source-level probe不是命令包装器；本例没有照做，不放宽原耦合门或把字符串命令名算源引用。
- 最终plan43含`import Main;`，默认包单名导入非法，属于模型错误；还含完整`public class ProbeMain`。现有生产Java源装载只认固定`CodraxVerificationProbe`，否则将剩余源嵌进main方法。任意合法完整主类因此也会被系统变坏，属于独立确定性系统缺口，不能与本例非法import混为一项。语法预检和执行都使用同一旧源函数；本机`/usr/bin/javac`仅launcher，无可用JRE，未获明确syntax诊断不等于语法通过。
- 后续最窄根修优先完整编译单元识别/文件名/主类装载共享，保snippet及原固定类，不改模型源、猜目标、清非法import或弱化验证权威。此片修复也不能倒签当前坏probe为可执行，不代销跨模式补证B2–B6。

## 3. 已交付与剩余项分账

本轮冻结§60/61两片全仓87包、13无测试包、零FAIL，已随`6d7e340bc`/`3f7f7a4c8`及文档`5582d0fd0`推送；其public/race验收和此轮答案验收独立。当前live未命中睡眠账目互指或bounded+坐标清理，不能用新机器PASS替代分支见证。旧业务/明确窗FAIL不变；新身份误述、内部词面及Java完整程序装载分别留账，13/79交付、66开放不变。
