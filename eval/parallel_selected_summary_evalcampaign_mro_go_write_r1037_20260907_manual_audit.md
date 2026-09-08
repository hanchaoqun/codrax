# r1037 Python MRO / Go 写入人工审计

- date: 2026-09-08T03:01:45Z
- sweep_start_ts: 20260907-200145
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

快照：已提交、干净构建 `3bbc749e1eaf`（B1617），两例恰好并行，无第三例。机器2/2 PASS；人工为Go交付通过、Python核心正确但有解释/引用残余，不算两例完整正确。此时SSH推送连接超时，不能将“已提交快照”写成“已推送快照”。原case、oracle、历史答案均未修改。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260907-200145 | write_apply,write_patch_oracle,answer_contains | none | 79s | 28 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass（交付） | 真实持久树只改1行，原测试逐字保留；摘要拼写方向误述另留观察 |
| 1 | sr_py_mro_order | PASS | eval/results/sr_py_mro_order-20260907-200145 | answer_regex,answer_contains | none | 118s | 28 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=5/0,fin_reject=1,unavail=0,prune=0 | partial | 核心顺序/职责/异常正确；C3、字典身份及Base引用不足 |

## 1. 选例与优先级

MRO上次08-05，检查跨类协作顺序、声明继承与真实调用区别、引用代次；Go上次08-30，作为低成本原生写入控制检查精准patch及持久化。Go只涉及main.go，已有POST_APPLY_FILE；不需要未修B1616b跨计划履约或B1561原生C++逐assertion能力。这个简单控制通过不代替复杂写验证。

## 2. Go 实际交付与证明

计划 `plan-1788836554336331000-10725`，实际持久树 `run-1.applied-tree`。逐字diff只有main.go:25的 `retrun`→`return`（1+/1-）；其余main.go不变，main_test.go、go.mod、README与原fixture逐字cmp一致。原测试覆盖空串、空白串、codrax三输入，没有删除、弱化或补写特判测试。

正式report记录一次真实 `go test -json ./...`，退出0、1052ms；原生TestGreet为assertion级结果，main.go为target_behavior covered，worktree audit clean。final.json的completion=verified、proof=strong、proof_ledger=verified、delivery=coherent，与实际单域交付一致。两个changed_file义务covered，convention仅advisory；不把覆盖计数理解为三条自然语言验收逐条独立执行。最终可见交付说明也披露此界限。

root另外从持久交付树执行 `go test -count=1 ./...` 通过（0.880s），CLI输出分别为 `Hello, codrax!`、空白输入 `Hello, world!`、多参数alpha/beta两行对应问候。验证不依赖仍存在的临时工作树，不修改原fixture。日志 `run-1.logs.all.log:1915` 是正式测试启动证据。

可见计划摘要把“retrun误写成return”方向说反，称单字符亦不精确，但拟议patch、执行和最终交付均正确；模型原始摘要就是如此，不能由系统扫描改写。仅作模型表达观察，不新增硬合同。本例没有replan，不作为B1616b生产正证。

## 3. Python 内容与关系

最终文件 `.codrax/output/20260907-200341.943-10720.md`。源码与原生Python实测：JsonPlugin没有自己的handle；TimestampMixin先复制原dict并写ingested_at，再到ValidationMixin，缺body立即ValueError、Base不运行；正常到BasePlugin再复制并置processed=True，原路返回且调用者dict不变。完整类MRO还含ABC/object；协作handle终点在Base。

最终核心顺序、两个mixin职责和异常都正确。md:11把C3线性化称为“从左到右、深度优先”不正确；md:13/18的“原始payload／或原始payload”对本次JsonPlugin调用含混，应是加戳副本。md:19用Base类docstring行13支持processed行为，真正实现18–20行，引用不够精确。上述错误在模型首稿/探索已存在，不是系统改写。日志3802给出同主体精确引用软提示，未制造额外拒绝。

请求没有要求必须画图。首稿画三条相邻owner call箭头，但已有工具事实只证明具体owner→super，没有绑定相邻owner的准确调用边。一次拒绝后模型在日志3794主动移除可选图；系统未删图，无图本身不判失败。现有通用cooperative-method roster教学已区分type_relation与call，本次模型只跑task_map/file_map，没有取完整roster；不能将声明基类顺序直接授权成调用边，也不据此新增MRO专门硬门。

## 4. JSON、上下文与重试

首稿blocks为JSON字符串，日志3723显示成功无损解析恢复；Mermaid格式亦有安全修复。原教学明确要求原生数组/正确嵌套，没有发现相反schema合同。一次成文拒绝同时包含3个不可引用recovered ID与缺乏typed支持的图边，下一patch成功；不是B1617旧越界ref复活的生产复现。本轮两例均无旧引用随池增长错绑迹象，但不把未触发反例称为B1617完整生产正证。

机器completion_rejects=0不能当成没有降级：日志2458/2487有两次DOWNGRADED，2520附近/2595经既有有界收敛排除仍无成员支持的aggregate，保其余grounded证据。精确support_ref行未匹配类成员身份，换成范围首行也不能证名；目前未证明同一合法carrier被同时必带必拒。

两例峰值上下文均28%，无预算耗尽/不可用工具/裁剪重试，不加预算。源码内容足以支持核心问题；模型解释错误不通过关键词门或模板答案纠正。

## 5. 记录而不追单题拟合

- P2候选：completion成员不匹配提示未给同源候选与精确失败原因，模型反复猜行；如落地应由同一个匹配器发布已有候选，不松门、不替模型选行。
- P2候选：recovered显示independently_proven（源码支持）与旁段不可引用（引用资格）是不同轴，非合同矛盾，但并置两轴可减少误读。不能把源码支持等同于可引用，更不能扫描正文作硬门。
- 高优先级仍是已真实复现B1618多capture混账，之后B1616b/B1561及异构交付域，不重复跑MRO直到模型换一种措辞。

本批没有Trace输入，不用runtime=none/trace_query=0宣称Trace回归。Trace隔离与活跃SSE由本轮已有生产/定向和整体验证覆盖；本例时长不是超过四分钟持续活跃流的证据，不引入默认4ms/4分钟未见正文降级。
