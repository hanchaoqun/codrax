# multirepo-polyglot — 跨语言多仓 workspace fixture

以真实开源项目形态为蓝本的缩减复现(非克隆):
- core-rs + bindings-py:Rust 核心 + pyo3 Python 绑定(蓝本:huggingface/tokenizers、pola-rs/polars 的 Rust/Python 分层)
- server-java + client-ts:Java 服务端路由表 + TS 客户端常量镜像(蓝本:grpc 生态多语言 SDK 的契约漂移场景)
- native-cpp:C++ 实现 + C ABI 头(蓝本:DaveGamble/cJSON、libgit2 的 C API 表面)

设计的跨语言事实(供 oracle 校验):
- Python `FastTokenizer.tokenize` → 原生模块 `_fastlex.tokenize_bytes`(pyo3 导出名)→ Rust `core-rs/src/lib.rs::tokenize_bytes`
- server-java Routes 有 5 条;client-ts API_ROUTES 只镜像了 4 条(缺 /v1/admin/reindex),且 /v1/search 的方法漂移(server POST vs client GET)
- native-cpp 的 C API 是 3 个 `fl_*` 函数,C++ 内部类 Lexer 不导出
