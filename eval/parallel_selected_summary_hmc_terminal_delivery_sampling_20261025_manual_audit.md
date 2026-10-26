# HMC293 双例人工审计

本批恰好两例真实模型、各一次并行，不补查/重跑求绿。进程正常退出、机器smoke、局部修复命中和完整答案分别裁定；完整人工0/2，54开放不减。

## A：代表选择与终态交付

原题要求按线程101/202及共同窗口归类，每类两份代表。任务分类已接受两目标/共同窗/代表预算2，并实际送达Explorer。首次 `request_plan` 与 `representatives` 混形被原门拒绝；模型随后改用无pid的event_search/resource_stack两视图规格，8计划/2组/4选4未选只是数字相同，不是原请求矩阵。

普通status已经实际交付同代selection/index和SHA，修复分支自然命中；不再把未调representative_status误列为未覆盖。真实回执最终由原审计器一次检查，不取未返回文件拼接。最终正文仍以gallery/editor结果画像替原线程任务分组，并用箭头把原深度当调用关系。原生表保留精确字段及边界，不能抵消正文错误。

原始A输出：`/tmp/codrax-hmc293-representatives-v1/workspace/.codrax/output/20261025-232815.225-23858.md`；对应HTML、answer-surfaces和root-causes全部保留。19表可见单元格与Markdown一致，0实际图，未做像素QA。

最后实际status的index SHA `a6b9d986df9d320e9badd4a674e31c8964bc87ebac4b63f483fc1452453bbb53`、selection SHA `2ab4f0ff3c039162824bfe8071bd8dc3ef1bd29e8f41b24a1fbcfeb7c9c3a3a5`；当次8计划/6尝试/2未执行，原件后续物理代次和SHA未改。之后Finalizer当前执行视图为12登记/16尝试/全部已尝试，最终12/16有来源，不是回填旧status。本例没有再调最终status；原helper仅用真实三元组一次exit1，`cluster specification or full membership differs`。

本轮实际独立解释为2/6单元[0,1]，含把右边界计入的4事件及深度箭头。最终资源计数纠正为101的3事件（申请2/768B、释放1/512B），箭头仍保留；不能用未发布的[2..5]强归因，也不能沿用§292“错误箭头未投递”的结论否认本轮实际投递。下一共享审计应关注错误恢复导航和解释引用范围，保合法查询及已验证关系。

## B：完整sample-only二进制

完整真实原件经共享准备进入原生查询。目标164条记录、259cycles权重，两热点82/174、82/85准确；权重与记录比例分列，原件12000与窗口1393不是目标分母。未知采样类型、assumed时钟、ip_only及完整展开未知保持原生事实。

机器smoke PASS不代表完整解释通过：principal第18/33行把这些未知类型样本断言为“在CPU上运行/在CPU上的权重”，同段又说不能区分on/off CPU；第20/39行基于未证实的on-CPU定性，把补证指向off-CPU并否定更多同口径采样。HTML复现同样错误，正确表格和尾部边界不抵消。

本轮调查reason含相似误述，但实际Finalizer没有投递该reason/member_notes，没有独立解释单元；不能据提交参数认定传播。原生unknown/权重非时间等边界实际已送达；暂未发现直接强迫on-CPU解释的系统提示。归重复出现的解释欠项，不称已证明的偶发波动。

## 旧审计补记与范围

HMC292 A原人工报告“无interpretation_units”不准确：三次提交均4个，实际最后投递[0,2]，有箭头的[1]与member_notes未投递。旧原件不改，完整FAIL不撤回。漏一条质量note的独有事实缺失亦未证，严格6/7收据仍保留。

本批不签未实际生成的图、不以HTML文本一致冒充像素QA，不签缺材料的新设备/格式或WAL在线零副作用。独立原报告：A `/tmp/codrax-hmc293-A-manual-v1.md` SHA `ce28de86db7a12257f39f0ca293d2ebe9b5a030841152f65c55966acf04ada31`；B `/tmp/codrax-hmc293-B-manual-v1.md` SHA `2ef346ac951fb5e916346e0106c491181b57b27771253a84e6378e6997edafce`。两份extra-artifacts清单覆盖实际会话全部材料；封存索引及发布凭据见[批汇总](parallel_selected_summary_hmc_terminal_delivery_sampling_20261025.md)。
