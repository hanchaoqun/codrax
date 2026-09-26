# Selected Eval Manual Audit — HiSys 保留与业务字段交接

- date: 2026-09-26T04:16:53Z
- sweep_start_ts: 20260925-211651
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

冻结快照 `12e2acfe4`，严格2并行×1，driver 1652正式exit0不代表两例通过。机器1/2，完整人工0/2；读完整主答案、执行日志、实际finalizer库存及独立fixture期望后判定。原机器verdict不改写，不追加第三次追绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_existing_sqlite_hisys_semantics | PASS | eval/results/trace_existing_sqlite_hisys_semantics-20260925-211653 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 154s | 37 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | FAIL | 六行字段交接成功；解释段推断未知域及完成流程，误称源metadata截断；指标fin_reject实际是一次emit接受后的advisory/patch，不是hard reject |
| 1 | trace_existing_sqlite_dictionary | FAIL | eval/results/trace_existing_sqlite_dictionary-20260925-211653 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 182s | 48 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 系统事件恢复5条；未知域仍被推测，4/5ms被写为约10ms/无结束；重叠库存重复占用32行预算，后端完成事件未送到库存上下文 |

## 六条系统事件新例

六条时间为2.004/2.012/2.022/2.034/2.046/2.060秒，表格均有，合法字典0解析为CAPTURE_DONE；NULL域、重复键未知名都未被删除；中文/斜线/引号名称及长内容末尾result=complete保留。日志2017的实际finalizer投递包含全部六条typed语义，1024字节内长contents完整可见，不只512字节raw预览。

完整答案仍FAIL：开头说“均来自相机管道和媒体管线”覆盖了未知域；仅凭事件标签和时间差把session=17描述成8ms完整流程，session=18的pending也被称为发起，没有区间配对证据。metadata完整供给却称“已被截断”，未区分展示省略与来源丢失。重复有序列表把重复键未解析也写成null；另泄露trace_query及临时转换路径。换行在表格中显示为空格不是此处根因，但不应称逐字原文。没有单位捏造或根因晋升。

日志2151第一次emit已接受；后续requested_dimensions软提示，2236一次add_facet_id member_set patch。机器fin_reject=1不可当成一次结构化硬拒绝。原答案留档，不为这些成文问题追加关键词门。

## 原字典例：人口恢复，结束事实被重复库存挤出

独立期望启动区间为1.010..1.018（8ms）、1.022..1.026（4ms）、1.030..1.034（4ms）、1.040..1.045（5ms）。答案前两条正确，后两条分别借下一阶段起点推为约10ms、谎称结束在窗外。系统事件5条现在全部存在，但1.050的未知域被猜为APP_LAUNCH，两个startup占位名未披露未知身份。

机器8ms正则误报仍保原判：表头已有ms而单元格只有8；5ms失败是真缺失，不以机器误报抹掉完整人工FAIL。

日志1254..2268有9次查询，多次重复AppStartup/hisysevent过滤。实际finalizer TraceEventInventories在3059、九份收据3067..3075：原trace_mark八行只显示首四行，省略四行；广域13行及另一13行库存也各只显示首四行。AppStartup过滤四条完整但都是B起点。32共享行按查询轮转被重复早期行占据，E1.034/E1.045没有进入该库存区块；原数据只有13条唯一事件，本可容纳。另summary只有top5，披露34项省略。此处有确定的系统证据投递缺陷，不能全部归为模型波动。

复核实际JSON：9份收据共56个查询成员（不是56条唯一事件），投递32个成员中只有12条不同来源/行/时间/类型组合，20个重复占位。12条里还含另一个0..1.08查询的0.950秒起点；该查询范围必须保留，不能为了当前窗口把它全局删掉。窗口内13条加该起点共14条仍可容纳在32行原帽内，显示共享有直接可量化收益。

下一高ROI修复归01.3/16.4：按可信来源/代次/物理行共享事件池，每份查询保过滤、membership、总量和完整性收据；不合并不同采集/时间轴，不扩大32行/128KiB预算，不按“最新查询”丢弃旧语义范围。耗时请求还需生产者的合法配对事实，不用下一起点代替结束。AppStartup未知名称逐行来源仍归04.3/17.7，不能靠扫描startup词面判断。

## 输出与证据收据

两例均没有读取源码、README或oracle；无repo_map/source_lens，纯Trace路径未回退源码义务。输出分别为`.codrax/output/20260925-211923.561-18384`（新例）及`20260925-211952.251-18385`（原例）。MD/HTML/answer-surfaces与131字节root-causes旁路均生成；旁路schema_version=2、root_causes=[]、status=unavailable、reason_code=trace_root_cause_contract_not_active，符合本次观察问题，无伪造根因。HTML仅文字/表格，未做截图视觉验收，不声称图形验收通过。600/300/600秒与活跃流保护未修改。

SHA-256（相对于表中result_dir）：

| 例 | 文件 | SHA-256 |
| --- | --- | --- |
| 原字典 | run-1.primary.md | 6a6176f8e46ecb72866f8487dbf0dbde3b65b65eb592eb6731da0d673a004e86 |
| 原字典 | run-1.logs.all.log | 9d199694432ab461b89664d10645383e5d28fc3deb3f0539797c7c3cbd43e5ed |
| 新语义 | run-1.primary.md | b094bfdae9e5b02e13ef63242268a953af2021d839ea67d2df577e3af867ce03 |
| 新语义 | run-1.logs.all.log | 91400609fbee5b680c2347277ee6da239c445332447f1988dcf9f6d416765a6b |
