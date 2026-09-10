# 功能对照与验证依据

基线为上游 `eb5d0927739ab278cc8903e55e40160efee15c45` 的 `charles_mcp/tools/public_surface.py`。31 项 canonical 工具全部注册；新增 `delete_capture`，默认共 32 项。参数 Schema 以 `charles-mcp --tools` 为准。

| 上游工具 | Go 实现行为 | 主要验证依据 |
|---|---|---|
| start_live_capture | 开启观察、可选清空、记录录制所有权 | TestLivePaginationUpdatesResetAndOwnership |
| read_live_capture | 消费增量事件、分页不丢条、响应更新 | TestLivePaginationUpdatesResetAndOwnership |
| peek_live_capture | 不消费游标 | TestLivePaginationUpdatesResetAndOwnership |
| stop_live_capture | 最后快照、持久化、按所有权恢复录制 | TestLivePaginationUpdatesResetAndOwnership |
| query_live_capture_entries | 全量查询实时捕获，不消费增量游标 | TestLiveToolsControlAndWorkflowDispatch |
| list_recordings | 查找 XML/native/JSON 文件并分页 | TestHistoryDedupAndResetConfig |
| get_recording_snapshot | 导入或复用捕获，返回摘要页 | TestMCPProtocolToolsSchemasAndCalls |
| query_recorded_traffic | 完整结构化条件过滤 | TestQueryAndStats、MCP 协议测试 |
| analyze_recorded_traffic | 过滤后的分组和统计 | TestQueryAndStats、MCP 协议测试 |
| group_capture_analysis | 七类分组键 | Analyze、MCP 协议测试 |
| get_capture_analysis_stats | 错误、大小、状态/资源分布、时延 | TestQueryAndStats、MCP 协议测试 |
| get_traffic_entry_detail | Header、Cookie、重定向、正文预览与原始字节 | MCP 协议测试 |
| charles_status | 连通性、录制状态、实时会话列表 | TestLiveToolsControlAndWorkflowDispatch、实际 Charles --check |
| throttling | 预设及自定义弱网模式、关闭节流 | TestLiveToolsControlAndWorkflowDispatch |
| reset_environment | 明确的录制、清空、退出、备份与恢复操作 | TestHistoryDedupAndResetConfig、TestProfileReplacementRemovesStaleFiles |
| reverse_import_session | XML/native/JSON，原生 CLI 转换后备 | TestImportFormatsAndPreservation、TestNativeConversionFallbackAndErrors |
| reverse_list_captures | SQLite 历史列表、分页 | TestStorePersistenceAndCascade、MCP 协议测试 |
| reverse_query_entries | 与普通查询共用 Entry | TestQueryAndStats、MCP 协议测试 |
| reverse_get_entry_detail | 与普通详情共用 Entry | MCP 协议测试 |
| reverse_decode_entry_body | 压缩、字符集、JSON、form、multipart、动态 Protobuf | TestDecodeCompressionFormatsAndProtobuf |
| reverse_replay_entry | 真实发送、参数/Header/嵌套 JSON/form/text 变异、实验与差异持久化 | TestReplayActuallySendsMutationsAndStoresFailures、TestReplayFormAndExplicitProxy |
| reverse_discover_signature_candidates | 变化字段启发式排名、持久化发现 | TestSignatureAndNestedMutationPlan |
| reverse_list_findings | 发现、解码、实验、工作流记录查询 | Store.Artifacts、重放持久化测试 |
| reverse_start_live_analysis | 共用实时会话实现 | TestLivePaginationUpdatesResetAndOwnership |
| reverse_peek_live_entries | 共用无消费读取 | TestLivePaginationUpdatesResetAndOwnership |
| reverse_read_live_entries | 共用分页消费读取 | TestLivePaginationUpdatesResetAndOwnership |
| reverse_stop_live_analysis | 共用停止和录制恢复 | TestLivePaginationUpdatesResetAndOwnership |
| reverse_charles_recording_status | 共用状态检查 | Charles.Recording |
| reverse_analyze_live_login_flow | 登录候选评分、证据解码、变异计划 | TestWorkflowsArePassiveAndProduceExecutableRecipes |
| reverse_analyze_live_api_flow | API 候选评分、业务字段优先 | 同上 |
| reverse_analyze_live_signature_flow | 签名候选评分、签名字段优先 | 同上 |

## 重新设计的边界

- Capture 是一组记录；Entry 是请求/响应记录；Live Session 是观察过程与读取进度。
- 工具名保留，筛选参数归入 `query`；结果统一包装为 `result`。这不是旧参数的逐字兼容接口。
- `legacy_json` 导入在上游 reverse 中未实现，本版支持普通 JSON/chlsj，包括 `body.encoded=true`。
- 签名分析的嵌套字段路径与重放统一使用 JSON Pointer，避免生成无法正确修改嵌套对象的实验配方。
- 同内容导入复用捕获；重复实时快照仅更新变化记录，不保存每次完整快照文件。
- 会话超时会恢复服务开启的录制，持久化捕获由 `delete_capture` 显式删除。
- `reset_environment` 不做隐式文件删除；配置与 profile 备份/恢复有独立 action。恢复 profile 会替换旧集合并移除过时文件。
- `include_raw` 明确请求原始字节，`max_chars` 仅控制预览。文本导出标为 `text_only`，不会伪称原始字节保真。
- `--legacy-aliases` 额外启用查询别名、捕获列表别名和定时抓包；接口遵循本版文档。

## 自动验证范围

- `go test -race ./...`：Go 单元、SQLite、压缩与动态描述符、受控本机 HTTP、录制状态、MCP 内存连接及独立 stdio 子进程。
- `go vet ./...`：静态检查。
- `scripts/build-release.sh`：CGO_ENABLED=0，macOS/Windows/Linux × amd64/arm64 交叉构建。
- CI 配置在三平台运行测试；本地交叉构建不等同于已在 Windows/Linux 真机上连接 Charles。
- Charles Web Interface 的具体配置、SSL 证书和原生格式版本取决于用户安装。可使用 `--check` 检查本机控制接口，再进行实时测试。

## 本机联调记录

2026-09-10，macOS arm64：`--check` 成功连接用户运行中的 Charles；只读 XML 导出解析 13 条记录、JSON 导出解析 14 条记录。首次联调时原生导出超过当时的 128 MiB 上限，未声称该大型真实会话已导入成功。后续按用户要求将导出、会话输入及原生 ZIP 展开总大小上限提高为 4 GiB。离线原生归档、CLI 转换以及超限不误走转换的回归测试覆盖相应功能。没有清空会话、停止用户录制或向抓包目标发送请求。

可选真实检查：`CHARLES_MCP_LIVE_SMOKE=1 go test ./internal/core -run TestRealCharlesReadOnlySmoke -v`。该检查读取当前会话；大于输入上限时应失败并说明大小限制。不要将默认测试通过解释为任意大小会话均受支持。
